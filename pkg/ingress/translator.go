package ingress

import (
	"fmt"
	"strings"

	"toron/pkg/discovery"
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

	var routes []*discovery.DiscoveredRoute

	for _, rule := range ing.Spec.Rules {
		host := strings.TrimSpace(rule.Host)
		if rule.HTTP == nil {
			continue
		}

		for _, pathRule := range rule.HTTP.Paths {
			prefix := strings.TrimSpace(pathRule.Path)
			if prefix != "" && !strings.HasPrefix(prefix, "/") {
				prefix = "/" + prefix
			}

			if pathRule.Backend.Service == nil {
				continue
			}

			svcName := pathRule.Backend.Service.Name
			svcPort := pathRule.Backend.Service.Port.Number

			// Lookup endpoints IPs for service
			epKey := fmt.Sprintf("%s/%s", ing.Metadata.Namespace, svcName)
			ep, epFound := endpointsMap[epKey]

			var targetIPs []string
			var targetPort int = svcPort

			if epFound && ep != nil {
				for _, subset := range ep.Subsets {
					for _, addr := range subset.Addresses {
						if addr.IP != "" {
							targetIPs = append(targetIPs, addr.IP)
						}
					}
					if targetPort == 0 && len(subset.Ports) > 0 {
						targetPort = subset.Ports[0].Port
					}
				}
			}

			if targetPort == 0 {
				targetPort = 80
			}

			if len(targetIPs) == 0 {
				// Fallback to K8s service cluster domain DNS / IP
				targetIPs = []string{fmt.Sprintf("%s.%s.svc.cluster.local", svcName, ing.Metadata.Namespace)}
			}

			for idx, ip := range targetIPs {
				r := &discovery.DiscoveredRoute{
					ContainerID:   fmt.Sprintf("k8s-%s-%s-%s-%d", ing.Metadata.Namespace, ing.Metadata.Name, svcName, idx),
					ContainerName: fmt.Sprintf("%s/%s", ing.Metadata.Namespace, svcName),
					Host:          host,
					Prefix:        prefix,
					TargetIP:      ip,
					TargetPort:    targetPort,
					Weight:        1,
				}
				routes = append(routes, r)
			}
		}
	}

	return routes, len(routes) > 0
}
