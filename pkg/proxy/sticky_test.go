package proxy

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"toron/pkg/httpparser"
)

// TestSticky_PreserveMobileRoamingAffinity verifies that sticky.go continues to inspect
// client-level headers (X-Forwarded-For) during IP-hash load balancing without regression,
// ensuring that mobile clients experiencing physical IP churn during cellular tower handoffs
// or CGNAT transitions maintain unbroken session affinity (REQ-030 / TC-092-09).
func TestSticky_PreserveMobileRoamingAffinity(t *testing.T) {
	targets := make([]*UpstreamTarget, 3)
	for i := 0; i < 3; i++ {
		u, _ := url.Parse(fmt.Sprintf("http://backend-%d.internal:808%d", i+1, i+1))
		targets[i] = NewUpstreamTarget(u, "", 3, 10*time.Second)
	}

	balancer, err := NewIPHashBalancer(targets)
	if err != nil {
		t.Fatalf("failed to create IPHashBalancer: %v", err)
	}
	defer balancer.Stop()

	// Mobile client session identifier injected by mobile carrier gateway
	clientCarrierIP := "203.0.113.77"

	// Hop 1 (Cell Tower 1)
	req1, _ := httpparser.NewRequest("GET", "/app/session", "HTTP/2.0")
	req1.RemoteAddr = "198.51.100.10:41000"
	req1.Header.Set("X-Forwarded-For", clientCarrierIP)

	target1, err := balancer.Next(req1)
	if err != nil {
		t.Fatalf("hop 1 failed to select target: %v", err)
	}

	// Hop 2 (Cell Tower 2 - Handover)
	req2, _ := httpparser.NewRequest("GET", "/app/session", "HTTP/2.0")
	req2.RemoteAddr = "198.51.100.25:42000" // Physical IP changed due to tower handover
	req2.Header.Set("X-Forwarded-For", clientCarrierIP)

	target2, err := balancer.Next(req2)
	if err != nil {
		t.Fatalf("hop 2 failed to select target: %v", err)
	}

	// Hop 3 (Carrier CGNAT shift)
	req3, _ := httpparser.NewRequest("GET", "/app/session", "HTTP/2.0")
	req3.RemoteAddr = "198.51.100.80:43000" // Physical IP shifted again
	req3.Header.Set("X-Forwarded-For", clientCarrierIP)

	target3, err := balancer.Next(req3)
	if err != nil {
		t.Fatalf("hop 3 failed to select target: %v", err)
	}

	// Verification 1: extractClientIP returns the consistent client identifier
	if ip1 := extractClientIP(req1); ip1 != clientCarrierIP {
		t.Errorf("extractClientIP(req1) = %q, want %q", ip1, clientCarrierIP)
	}
	if ip2 := extractClientIP(req2); ip2 != clientCarrierIP {
		t.Errorf("extractClientIP(req2) = %q, want %q", ip2, clientCarrierIP)
	}
	if ip3 := extractClientIP(req3); ip3 != clientCarrierIP {
		t.Errorf("extractClientIP(req3) = %q, want %q", ip3, clientCarrierIP)
	}

	// Verification 2: All three hops route to the exact same backend target
	if target1.URL.String() != target2.URL.String() {
		t.Fatalf("session affinity broken between hop 1 (%s) and hop 2 (%s)", target1.URL, target2.URL)
	}
	if target2.URL.String() != target3.URL.String() {
		t.Fatalf("session affinity broken between hop 2 (%s) and hop 3 (%s)", target2.URL, target3.URL)
	}
}
