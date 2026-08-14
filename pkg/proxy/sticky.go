package proxy

import (
	"fmt"
	"net"
	"strings"
	"sync/atomic"

	"toron/pkg/httpparser"
)

// IPHashBalancer maps requests from the same client IP address consistently to a healthy target.
type IPHashBalancer struct {
	targets []*UpstreamTarget
}

// NewIPHashBalancer creates a load balancer that hashes client IP addresses.
func NewIPHashBalancer(targets []*UpstreamTarget) (*IPHashBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	copied := make([]*UpstreamTarget, len(targets))
	copy(copied, targets)
	return &IPHashBalancer{targets: copied}, nil
}

func (b *IPHashBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	var healthy []*UpstreamTarget
	for _, t := range b.targets {
		state := t.GetState()
		if state == StateClosed || state == StateHalfOpen {
			healthy = append(healthy, t)
		}
	}
	if len(healthy) == 0 {
		return nil, ErrNoHealthyUpstreamAvailable
	}

	clientIP := extractClientIP(req)
	h := fnv32Hash(clientIP)
	idx := int(h % uint32(len(healthy)))
	return healthy[idx], nil
}

func (b *IPHashBalancer) Algorithm() Algorithm       { return AlgorithmIPHash }
func (b *IPHashBalancer) Targets() []*UpstreamTarget { return b.targets }
func (b *IPHashBalancer) Stop() {
	for _, t := range b.targets {
		t.StopActiveHealthCheck()
	}
}

// StickyCookieBalancer implements cookie-based session affinity.
type StickyCookieBalancer struct {
	targets    []*UpstreamTarget
	cookieName string
	counter    uint64
	targetMap  map[string]*UpstreamTarget
}

// TargetHash computes a hash string identifier for a target URL.
func TargetHash(targetURL string) string {
	h := fnv32Hash(targetURL)
	return fmt.Sprintf("%08x", h)
}

// NewStickyCookieBalancer creates a load balancer that binds client sessions using HTTP cookies.
func NewStickyCookieBalancer(targets []*UpstreamTarget, cookieName string) (*StickyCookieBalancer, error) {
	if len(targets) == 0 {
		return nil, ErrNoTargetsAvailable
	}
	if strings.TrimSpace(cookieName) == "" {
		cookieName = "TORON_STICKY"
	}
	copied := make([]*UpstreamTarget, len(targets))
	copy(copied, targets)

	tMap := make(map[string]*UpstreamTarget)
	for _, t := range copied {
		hashKey := TargetHash(t.URL.String())
		tMap[hashKey] = t
	}

	return &StickyCookieBalancer{
		targets:    copied,
		cookieName: strings.TrimSpace(cookieName),
		targetMap:  tMap,
	}, nil
}

func (b *StickyCookieBalancer) CookieName() string {
	return b.cookieName
}

func (b *StickyCookieBalancer) Next(req *httpparser.Request) (*UpstreamTarget, error) {
	if req != nil {
		cookieHeader := req.Header.Get("Cookie")
		if cookieHeader != "" {
			cookies := strings.Split(cookieHeader, ";")
			for _, c := range cookies {
				kv := strings.SplitN(strings.TrimSpace(c), "=", 2)
				if len(kv) == 2 && strings.TrimSpace(kv[0]) == b.cookieName {
					hashVal := strings.TrimSpace(kv[1])
					if target, exists := b.targetMap[hashVal]; exists {
						state := target.GetState()
						if state == StateClosed || state == StateHalfOpen {
							return target, nil
						}
					}
				}
			}
		}
	}

	// Fallback to round-robin healthy target
	n := len(b.targets)
	startIdx := int(atomic.AddUint64(&b.counter, 1) - 1)
	for i := 0; i < n; i++ {
		target := b.targets[(startIdx+i)%n]
		state := target.GetState()
		if state == StateClosed || state == StateHalfOpen {
			return target, nil
		}
	}

	return nil, ErrNoHealthyUpstreamAvailable
}

func (b *StickyCookieBalancer) Algorithm() Algorithm       { return AlgorithmStickyCookie }
func (b *StickyCookieBalancer) Targets() []*UpstreamTarget { return b.targets }
func (b *StickyCookieBalancer) Stop() {
	for _, t := range b.targets {
		t.StopActiveHealthCheck()
	}
}

func extractClientIP(req *httpparser.Request) string {
	if req == nil {
		return "127.0.0.1"
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if req.RawConn != nil {
		if remoteAddr := req.RawConn.RemoteAddr(); remoteAddr != nil {
			host, _, err := net.SplitHostPort(remoteAddr.String())
			if err == nil {
				return host
			}
			return remoteAddr.String()
		}
	}
	return "127.0.0.1"
}

func fnv32Hash(key string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return hash
}
