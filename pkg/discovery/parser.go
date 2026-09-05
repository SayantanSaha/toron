package discovery

import (
	"strconv"
	"strings"
)

const (
	LabelEnable            = "toron.enable"
	LabelHost              = "toron.host"
	LabelPrefix            = "toron.prefix"
	LabelPort              = "toron.port"
	LabelWeight            = "toron.weight"
	LabelHealthCheck       = "toron.health_check"
	LabelStripPrefix       = "toron.strip_prefix"
	LabelRewriteRedirects  = "toron.rewrite_redirects"
	LabelRewriteCookiePath = "toron.rewrite_cookie_path"
)

// ParseContainerLabels extracts Toron route configuration from container labels or annotations.
// Returns (route, true) if toron.enable is true and valid port information is present; otherwise (nil, false).
func ParseContainerLabels(container Container, defaultWeight int) (*DiscoveredRoute, bool) {
	// Filter out non-running containers (e.g. exited, stopped, created, dead)
	if container.State != "" && container.State != "running" {
		return nil, false
	}

	enableVal := strings.ToLower(strings.TrimSpace(container.Labels[LabelEnable]))
	if enableVal != "true" && enableVal != "1" && enableVal != "yes" {
		return nil, false
	}

	host := strings.TrimSpace(container.Labels[LabelHost])
	prefix := strings.TrimSpace(container.Labels[LabelPrefix])

	if prefix != "" && !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	// Security (TASK-079): Disallow unhosted root ("" or "/") and reserved administrative prefixes
	if (prefix == "" || prefix == "/") && host == "" {
		return nil, false
	}
	if prefix == "/internal" || strings.HasPrefix(prefix, "/internal/") ||
		prefix == "/api/status" || strings.HasPrefix(prefix, "/api/status/") {
		return nil, false
	}

	targetPort := 0
	if portStr := strings.TrimSpace(container.Labels[LabelPort]); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			targetPort = p
		}
	}

	// Fallback to first exposed container port if toron.port is not explicitly set
	if targetPort == 0 && len(container.Ports) > 0 {
		for _, pm := range container.Ports {
			if pm.PrivatePort > 0 {
				targetPort = pm.PrivatePort
				break
			}
		}
	}

	if targetPort == 0 {
		targetPort = 80 // Default HTTP port fallback
	}

	weight := defaultWeight
	if weight <= 0 {
		weight = 1
	}
	if weightStr := strings.TrimSpace(container.Labels[LabelWeight]); weightStr != "" {
		if w, err := strconv.Atoi(weightStr); err == nil && w > 0 {
			weight = w
		}
	}

	healthCheck := strings.TrimSpace(container.Labels[LabelHealthCheck])

	var stripPrefix *bool
	if spStr, ok := container.Labels[LabelStripPrefix]; ok {
		val := strings.ToLower(strings.TrimSpace(spStr))
		b := val == "true" || val == "1" || val == "yes"
		stripPrefix = &b
	}

	var rewriteRedirects *bool
	if rrStr, ok := container.Labels[LabelRewriteRedirects]; ok {
		val := strings.ToLower(strings.TrimSpace(rrStr))
		b := val == "true" || val == "1" || val == "yes"
		rewriteRedirects = &b
	}

	var rewriteCookiePath *bool
	if rcStr, ok := container.Labels[LabelRewriteCookiePath]; ok {
		val := strings.ToLower(strings.TrimSpace(rcStr))
		b := val == "true" || val == "1" || val == "yes"
		rewriteCookiePath = &b
	}

	cName := ""
	if len(container.Names) > 0 {
		cName = strings.TrimPrefix(container.Names[0], "/")
	} else if len(container.ID) >= 12 {
		cName = container.ID[:12]
	} else {
		cName = container.ID
	}

	ip := container.IPAddress
	if ip == "" {
		ip = "127.0.0.1"
	}

	// For Docker Desktop on macOS/Windows or host-mode discovery, map target to 127.0.0.1:PublicPort
	for _, pm := range container.Ports {
		if pm.PublicPort > 0 {
			if targetPort == pm.PrivatePort || targetPort == 0 || targetPort == 80 {
				targetPort = pm.PublicPort
				ip = "127.0.0.1"
				break
			}
		}
	}

	return &DiscoveredRoute{
		ContainerID:       container.ID,
		ContainerName:     cName,
		Host:              host,
		Prefix:            prefix,
		TargetIP:          ip,
		TargetPort:        targetPort,
		Weight:            weight,
		HealthCheckPath:   healthCheck,
		StripPrefix:       stripPrefix,
		RewriteRedirects:  rewriteRedirects,
		RewriteCookiePath: rewriteCookiePath,
	}, true
}
