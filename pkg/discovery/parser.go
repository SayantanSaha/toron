package discovery

import (
	"strconv"
	"strings"
)

const (
	LabelEnable      = "toron.enable"
	LabelHost        = "toron.host"
	LabelPrefix      = "toron.prefix"
	LabelPort        = "toron.port"
	LabelWeight      = "toron.weight"
	LabelHealthCheck = "toron.health_check"
)

// ParseContainerLabels extracts Toron route configuration from container labels or annotations.
// Returns (route, true) if toron.enable is true and valid port information is present; otherwise (nil, false).
func ParseContainerLabels(container Container, defaultWeight int) (*DiscoveredRoute, bool) {
	if container.Labels == nil {
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
		ContainerID:     container.ID,
		ContainerName:   cName,
		Host:            host,
		Prefix:          prefix,
		TargetIP:        ip,
		TargetPort:      targetPort,
		Weight:          weight,
		HealthCheckPath: healthCheck,
	}, true
}
