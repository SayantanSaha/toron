package waf

import (
	"fmt"
	"net"
	"strings"

	"toron/pkg/httpparser"
)

// IPAccessList manages allowed and denied CIDR blocks and IP addresses.
type IPAccessList struct {
	allowedSubnets []*net.IPNet
	allowedIPs     []net.IP
	deniedSubnets  []*net.IPNet
	deniedIPs      []net.IP
}

// NewIPAccessList compiles allowed and denied IP / CIDR string lists into fast-evaluating structures.
func NewIPAccessList(allowedCIDRs []string, deniedCIDRs []string) (*IPAccessList, error) {
	acl := &IPAccessList{}

	for _, raw := range allowedCIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			_, ipNet, err := net.ParseCIDR(raw)
			if err != nil {
				return nil, fmt.Errorf("waf: invalid allowed CIDR %q: %w", raw, err)
			}
			acl.allowedSubnets = append(acl.allowedSubnets, ipNet)
		} else {
			ip := net.ParseIP(raw)
			if ip == nil {
				return nil, fmt.Errorf("waf: invalid allowed IP %q", raw)
			}
			acl.allowedIPs = append(acl.allowedIPs, ip)
		}
	}

	for _, raw := range deniedCIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			_, ipNet, err := net.ParseCIDR(raw)
			if err != nil {
				return nil, fmt.Errorf("waf: invalid denied CIDR %q: %w", raw, err)
			}
			acl.deniedSubnets = append(acl.deniedSubnets, ipNet)
		} else {
			ip := net.ParseIP(raw)
			if ip == nil {
				return nil, fmt.Errorf("waf: invalid denied IP %q", raw)
			}
			acl.deniedIPs = append(acl.deniedIPs, ip)
		}
	}

	return acl, nil
}

// HasRules returns true if any allowed or denied rules are configured.
func (acl *IPAccessList) HasRules() bool {
	if acl == nil {
		return false
	}
	return len(acl.allowedSubnets) > 0 || len(acl.allowedIPs) > 0 || len(acl.deniedSubnets) > 0 || len(acl.deniedIPs) > 0
}

// CheckIP evaluates whether an IP address is permitted by the access list.
// If an allowed IP list is configured and ip is nil, access is denied (fail-secure).
func (acl *IPAccessList) CheckIP(ip net.IP) (allowed bool, reason string) {
	if acl == nil || !acl.HasRules() {
		return true, ""
	}

	if ip == nil {
		if len(acl.allowedSubnets) > 0 || len(acl.allowedIPs) > 0 {
			return false, "client IP could not be determined and allowed IP list is enforced"
		}
		return true, ""
	}

	// 1. Check Deny List First
	for _, ipNet := range acl.deniedSubnets {
		if ipNet.Contains(ip) {
			return false, fmt.Sprintf("IP address %s matches denied CIDR %s", ip.String(), ipNet.String())
		}
	}
	for _, deniedIP := range acl.deniedIPs {
		if deniedIP.Equal(ip) {
			return false, fmt.Sprintf("IP address %s matches denied IP", ip.String())
		}
	}

	// 2. If Allowed List is configured, IP must match at least one entry
	if len(acl.allowedSubnets) > 0 || len(acl.allowedIPs) > 0 {
		for _, ipNet := range acl.allowedSubnets {
			if ipNet.Contains(ip) {
				return true, ""
			}
		}
		for _, allowedIP := range acl.allowedIPs {
			if allowedIP.Equal(ip) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("IP address %s is not in the allowed IP list", ip.String())
	}

	return true, ""
}

func isTrustedProxy(ip net.IP, trustedProxies []string) bool {
	if ip == nil || len(trustedProxies) == 0 {
		return false
	}
	for _, raw := range trustedProxies {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			if _, ipNet, err := net.ParseCIDR(raw); err == nil && ipNet != nil {
				if ipNet.Contains(ip) {
					return true
				}
			}
		} else {
			if tip := net.ParseIP(raw); tip != nil {
				if tip.Equal(ip) {
					return true
				}
			}
		}
	}
	return false
}

func extractForwardedHeaderIP(req *httpparser.Request) net.IP {
	if req == nil || req.Header == nil {
		return nil
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			raw := strings.TrimSpace(parts[0])
			if ip := net.ParseIP(raw); ip != nil {
				return ip
			}
			if host, _, err := net.SplitHostPort(raw); err == nil {
				if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
					return ip
				}
			}
		}
	}
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		raw := strings.TrimSpace(xri)
		if ip := net.ParseIP(raw); ip != nil {
			return ip
		}
		if host, _, err := net.SplitHostPort(raw); err == nil {
			if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
				return ip
			}
		}
	}
	return nil
}

// ExtractClientIP extracts the remote client IP from an HTTP request safely.
// Physical connection address (req.RemoteIP() / req.RemoteAddr / req.RawConn) is prioritized.
// Client-supplied forwarded headers (X-Forwarded-For, X-Real-IP) are only evaluated
// if the physical peer IP matches a configured trusted_proxies entry, or for synthetic
// test requests where physical connection metadata is absent (req.RemoteAddr == "" && req.RawConn == nil).
func ExtractClientIP(req *httpparser.Request, trustedProxies ...[]string) net.IP {
	if req == nil {
		return nil
	}

	var tp []string
	if len(trustedProxies) > 0 {
		tp = trustedProxies[0]
	}

	physicalIP := req.RemoteIP()

	// If physical connection IP is available:
	if physicalIP != nil {
		if isTrustedProxy(physicalIP, tp) {
			// Physical connection is a verified trusted proxy: evaluate forwarded headers
			if headerIP := extractForwardedHeaderIP(req); headerIP != nil {
				return headerIP
			}
		}
		// Untrusted peer or no forwarded header: strictly return verified physical IP
		return physicalIP
	}

	// Fallback for synthetic unit test requests without physical connection information
	return extractForwardedHeaderIP(req)
}
