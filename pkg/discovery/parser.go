package discovery

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const (
	LabelEnable              = "toron.enable"
	LabelHost                = "toron.host"
	LabelPrefix              = "toron.prefix"
	LabelMethod              = "toron.method"
	LabelHeaders             = "toron.headers"
	LabelHeaderPrefix        = "toron.header."
	LabelPort                = "toron.port"
	LabelWeight              = "toron.weight"
	LabelHealthCheck         = "toron.health_check"
	LabelHealthCheckInterval = "toron.health_check_interval"
	LabelStripPrefix         = "toron.strip_prefix"
	LabelRewriteRedirects    = "toron.rewrite_redirects"
	LabelRewriteCookiePath   = "toron.rewrite_cookie_path"
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

	// Method Extraction (REQ-095-AC-02)
	method := ""
	if mVal, ok := container.Labels[LabelMethod]; ok {
		method = strings.ToUpper(strings.TrimSpace(mVal))
	}

	// Headers Extraction (REQ-095-AC-02)
	var headers map[string]string

	// 1. Grouped headers label: toron.headers (CSV or JSON)
	if rawHeaders, ok := container.Labels[LabelHeaders]; ok {
		raw := strings.TrimSpace(rawHeaders)
		if raw != "" {
			if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
				var jsonMap map[string]string
				if err := json.Unmarshal([]byte(raw), &jsonMap); err == nil {
					if len(jsonMap) > 0 {
						headers = make(map[string]string, len(jsonMap))
						for k, v := range jsonMap {
							kTrim := strings.TrimSpace(k)
							if kTrim != "" {
								headers[kTrim] = strings.TrimSpace(v)
							}
						}
					}
				} else {
					// Fallback to comma-delimited parsing without panic
					tokens := strings.Split(raw, ",")
					for _, tok := range tokens {
						tok = strings.TrimSpace(tok)
						if tok == "" {
							continue
						}
						parts := strings.SplitN(tok, "=", 2)
						if len(parts) == 2 {
							kTrim := strings.TrimSpace(parts[0])
							if kTrim != "" {
								if headers == nil {
									headers = make(map[string]string)
								}
								headers[kTrim] = strings.TrimSpace(parts[1])
							}
						}
					}
				}
			} else {
				// Comma-delimited key=value
				tokens := strings.Split(raw, ",")
				for _, tok := range tokens {
					tok = strings.TrimSpace(tok)
					if tok == "" {
						continue
					}
					parts := strings.SplitN(tok, "=", 2)
					if len(parts) == 2 {
						kTrim := strings.TrimSpace(parts[0])
						if kTrim != "" {
							if headers == nil {
								headers = make(map[string]string)
							}
							headers[kTrim] = strings.TrimSpace(parts[1])
						}
					}
				}
			}
		}
	}

	// 2. Individual header labels: toron.header.<Name> (merge additively & override grouped headers)
	for k, v := range container.Labels {
		if strings.HasPrefix(k, LabelHeaderPrefix) {
			hName := strings.TrimSpace(k[len(LabelHeaderPrefix):])
			if hName != "" {
				if headers == nil {
					headers = make(map[string]string)
				}
				headers[hName] = strings.TrimSpace(v)
			}
		}
	}

	if len(headers) == 0 {
		headers = nil
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

	var healthCheckInterval time.Duration
	if hciStr := strings.TrimSpace(container.Labels[LabelHealthCheckInterval]); hciStr != "" {
		if d, err := time.ParseDuration(hciStr); err == nil && d > 0 {
			healthCheckInterval = d
		}
	}

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
		ContainerID:         container.ID,
		ContainerName:       cName,
		Host:                host,
		Prefix:              prefix,
		Method:              method,
		Headers:             headers,
		TargetIP:            ip,
		TargetPort:          targetPort,
		Weight:              weight,
		HealthCheckPath:     healthCheck,
		HealthCheckInterval: healthCheckInterval,
		StripPrefix:         stripPrefix,
		RewriteRedirects:    rewriteRedirects,
		RewriteCookiePath:   rewriteCookiePath,
	}, true
}
