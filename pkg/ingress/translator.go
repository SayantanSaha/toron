package ingress

import (
	"encoding/json"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"toron/pkg/discovery"
)

const (
	AnnotationMethod               = "toron.io/method"
	AnnotationHeaders              = "toron.io/headers"
	AnnotationHeaderPrefix         = "toron.io/header."
	AnnotationNginxCanary          = "nginx.ingress.kubernetes.io/canary"
	AnnotationNginxCanaryHeader    = "nginx.ingress.kubernetes.io/canary-by-header"
	AnnotationNginxCanaryHeaderVal = "nginx.ingress.kubernetes.io/canary-by-header-value"
	AnnotationHealthCheck          = "toron.io/health-check"
	AnnotationHealthCheckInterval  = "toron.io/health-check-interval"
)

// TranslateIngress converts a Kubernetes Ingress object into a list of Toron DiscoveredRoute items.
func TranslateIngress(ing Ingress, targetIngressClass string, endpointsMap map[string]*Endpoints) ([]*discovery.DiscoveredRoute, bool) {
	if targetIngressClass == "" {
		targetIngressClass = "toron"
	}

	// Filter by IngressClassName or legacy annotation
	classMatch := false
	if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName == targetIngressClass {
		classMatch = true
	} else if annClass, ok := ing.Metadata.Annotations["kubernetes.io/ingress.class"]; ok && annClass == targetIngressClass {
		classMatch = true
	} else if ing.Spec.IngressClassName == nil && len(ing.Metadata.Annotations) == 0 && targetIngressClass == "toron" {
		// Accept default if unspecified
		classMatch = true
	}

	if !classMatch {
		return nil, false
	}

	// Method constraint parsing (REQ-096-AC-01)
	var method string
	if rawMethod, ok := ing.Metadata.Annotations[AnnotationMethod]; ok {
		method = strings.ToUpper(strings.TrimSpace(rawMethod))
	}

	// Headers constraint parsing (REQ-096-AC-01)
	headers := make(map[string]string)

	// 1. Grouped headers annotation: toron.io/headers (JSON or CSV)
	if rawHeaders, ok := ing.Metadata.Annotations[AnnotationHeaders]; ok {
		trimmed := strings.TrimSpace(rawHeaders)
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			var jsonMap map[string]string
			if err := json.Unmarshal([]byte(trimmed), &jsonMap); err == nil {
				for k, v := range jsonMap {
					kTrim := strings.TrimSpace(k)
					if kTrim != "" {
						headers[kTrim] = strings.TrimSpace(v)
					}
				}
			}
		} else if trimmed != "" {
			for _, pair := range strings.Split(trimmed, ",") {
				pair = strings.TrimSpace(pair)
				if pair == "" {
					continue
				}
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) == 2 {
					kTrim := strings.TrimSpace(parts[0])
					if kTrim != "" {
						headers[kTrim] = strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}

	// 2. Industry-Standard NGINX Canary Compatibility (nginx.ingress.kubernetes.io/canary*)
	if strings.ToLower(strings.TrimSpace(ing.Metadata.Annotations[AnnotationNginxCanary])) == "true" {
		canaryHdr := strings.TrimSpace(ing.Metadata.Annotations[AnnotationNginxCanaryHeader])
		if canaryHdr != "" {
			canaryVal := strings.TrimSpace(ing.Metadata.Annotations[AnnotationNginxCanaryHeaderVal])
			if canaryVal == "" {
				canaryVal = "always"
			}
			headers[canaryHdr] = canaryVal
		}
	}

	// 3. Individual header annotations (toron.io/header.<Name>) override grouped/canary headers
	for k, v := range ing.Metadata.Annotations {
		if strings.HasPrefix(k, AnnotationHeaderPrefix) {
			hdrKey := strings.TrimSpace(k[len(AnnotationHeaderPrefix):])
			if hdrKey != "" {
				headers[hdrKey] = strings.TrimSpace(v)
			}
		}
	}

	if len(headers) == 0 {
		headers = nil
	}

	// Health check path & interval
	hcPath := strings.TrimSpace(ing.Metadata.Annotations[AnnotationHealthCheck])
	if hcPath == "" {
		hcPath = strings.TrimSpace(ing.Metadata.Annotations["toron.io/health_check"])
	}
	var hcInterval time.Duration
	hcIntervalStr := strings.TrimSpace(ing.Metadata.Annotations[AnnotationHealthCheckInterval])
	if hcIntervalStr == "" {
		hcIntervalStr = strings.TrimSpace(ing.Metadata.Annotations["toron.io/health_check_interval"])
	}
	if hcIntervalStr != "" {
		if d, err := time.ParseDuration(hcIntervalStr); err == nil && d > 0 {
			hcInterval = d
		}
	}

	var routes []*discovery.DiscoveredRoute

	for _, rule := range ing.Spec.Rules {
		host := strings.TrimSpace(rule.Host)
		if rule.HTTP == nil {
			continue
		}

		for _, pathRule := range rule.HTTP.Paths {
			rawPrefix := strings.TrimSpace(pathRule.Path)
			if rawPrefix != "" && !strings.HasPrefix(rawPrefix, "/") {
				rawPrefix = "/" + rawPrefix
			}
			cleanPrefix := path.Clean(rawPrefix)
			if cleanPrefix == "." || cleanPrefix == "" {
				cleanPrefix = "/"
			}

			// Security (TASK-087 & TASK-088): Disallow unhosted root ("" or "/") and reserved administrative prefixes
			if (cleanPrefix == "/" || cleanPrefix == "") && host == "" {
				log.Printf("[INGRESS] Security rejection in %s/%s: unhosted root path %q is prohibited",
					ing.Metadata.Namespace, ing.Metadata.Name, pathRule.Path)
				continue
			}

			if cleanPrefix == "/internal" || strings.HasPrefix(cleanPrefix, "/internal/") ||
				cleanPrefix == "/api/status" || strings.HasPrefix(cleanPrefix, "/api/status/") {
				log.Printf("[INGRESS] Security rejection in %s/%s: attempted to shadow protected system path %q (host: %q)",
					ing.Metadata.Namespace, ing.Metadata.Name, cleanPrefix, host)
				continue
			}

			if host == "" && (cleanPrefix == "/health" || strings.HasPrefix(cleanPrefix, "/health/") ||
				cleanPrefix == "/metrics" || strings.HasPrefix(cleanPrefix, "/metrics/")) {
				log.Printf("[INGRESS] Security rejection in %s/%s: unhosted shadowing of probe/telemetry endpoint %q is prohibited",
					ing.Metadata.Namespace, ing.Metadata.Name, cleanPrefix)
				continue
			}

			prefix := cleanPrefix

			if pathRule.Backend.Service == nil {
				continue
			}

			svcName := pathRule.Backend.Service.Name
			svcPort := pathRule.Backend.Service.Port.Number

			// Lookup endpoints IPs for service
			epKey := fmt.Sprintf("%s/%s", ing.Metadata.Namespace, svcName)
			ep, epFound := endpointsMap[epKey]

			type targetEndpoint struct {
				ip   string
				port int
			}
			var targets []targetEndpoint

			if epFound && ep != nil {
				for _, subset := range ep.Subsets {
					p := svcPort
					if len(subset.Ports) > 0 && subset.Ports[0].Port > 0 {
						p = subset.Ports[0].Port
					} else if p == 0 {
						p = 80
					}
					for _, addr := range subset.Addresses {
						if addr.IP != "" {
							targets = append(targets, targetEndpoint{ip: addr.IP, port: p})
						}
					}
				}
			}

			if len(targets) == 0 {
				p := svcPort
				if p == 0 {
					p = 80
				}
				// Fallback to K8s service cluster domain DNS / IP
				targets = append(targets, targetEndpoint{
					ip:   fmt.Sprintf("%s.%s.svc.cluster.local", svcName, ing.Metadata.Namespace),
					port: p,
				})
			}

			for idx, tgt := range targets {
				var routeHeaders map[string]string
				if len(headers) > 0 {
					routeHeaders = make(map[string]string, len(headers))
					for hk, hv := range headers {
						routeHeaders[hk] = hv
					}
				}

				r := &discovery.DiscoveredRoute{
					ContainerID:         fmt.Sprintf("k8s-%s-%s-%s-%d", ing.Metadata.Namespace, ing.Metadata.Name, svcName, idx),
					ContainerName:       fmt.Sprintf("%s/%s", ing.Metadata.Namespace, svcName),
					Host:                host,
					Prefix:              prefix,
					Method:              method,
					Headers:             routeHeaders,
					TargetIP:            tgt.ip,
					TargetPort:          tgt.port,
					Weight:              1,
					HealthCheckPath:     hcPath,
					HealthCheckInterval: hcInterval,
				}
				routes = append(routes, r)
			}
		}
	}

	return routes, len(routes) > 0
}
