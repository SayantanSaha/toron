package waf

import (
	"net"
	"testing"

	"toron/pkg/httpparser"
)

func TestIPAccessList_DeniedCIDR(t *testing.T) {
	acl, err := NewIPAccessList(nil, []string{"198.51.100.0/24", "203.0.113.5"})
	if err != nil {
		t.Fatalf("unexpected error initializing IPAccessList: %v", err)
	}

	// Blocked by subnet
	allowed, reason := acl.CheckIP(net.ParseIP("198.51.100.42"))
	if allowed {
		t.Errorf("expected 198.51.100.42 to be denied, but was allowed")
	}
	if reason == "" {
		t.Errorf("expected non-empty reason when denied")
	}

	// Blocked by exact IP
	allowed, _ = acl.CheckIP(net.ParseIP("203.0.113.5"))
	if allowed {
		t.Errorf("expected 203.0.113.5 to be denied, but was allowed")
	}

	// Allowed benign IP
	allowed, _ = acl.CheckIP(net.ParseIP("127.0.0.1"))
	if !allowed {
		t.Errorf("expected 127.0.0.1 to be allowed")
	}
}

func TestIPAccessList_AllowedCIDR(t *testing.T) {
	acl, err := NewIPAccessList([]string{"10.0.0.0/8", "192.168.1.100"}, nil)
	if err != nil {
		t.Fatalf("unexpected error initializing IPAccessList: %v", err)
	}

	// Allowed by subnet
	allowed, _ := acl.CheckIP(net.ParseIP("10.254.1.5"))
	if !allowed {
		t.Errorf("expected 10.254.1.5 to be allowed")
	}

	// Allowed by exact IP
	allowed, _ = acl.CheckIP(net.ParseIP("192.168.1.100"))
	if !allowed {
		t.Errorf("expected 192.168.1.100 to be allowed")
	}

	// Denied (not in allowlist)
	allowed, reason := acl.CheckIP(net.ParseIP("192.168.1.101"))
	if allowed {
		t.Errorf("expected 192.168.1.101 to be denied")
	}
	if reason == "" {
		t.Errorf("expected non-empty reason when denied")
	}
}

func TestIPAccessList_IPv6Support(t *testing.T) {
	acl, err := NewIPAccessList([]string{"2001:db8::/32"}, []string{"2001:db8:ffff::/48"})
	if err != nil {
		t.Fatalf("unexpected error initializing IPAccessList with IPv6: %v", err)
	}

	// Blocked by IPv6 denylist
	allowed, _ := acl.CheckIP(net.ParseIP("2001:db8:ffff::1"))
	if allowed {
		t.Errorf("expected 2001:db8:ffff::1 to be denied")
	}

	// Allowed by IPv6 allowlist
	allowed, _ = acl.CheckIP(net.ParseIP("2001:db8:1234::1"))
	if !allowed {
		t.Errorf("expected 2001:db8:1234::1 to be allowed")
	}

	// Denied (outside allowlist)
	allowed, _ = acl.CheckIP(net.ParseIP("2607:f8b0:4005:805::200e"))
	if allowed {
		t.Errorf("expected non-matching IPv6 to be denied")
	}
}

func TestIPAccessList_InvalidCIDR(t *testing.T) {
	_, err := NewIPAccessList([]string{"invalid-cidr/99"}, nil)
	if err == nil {
		t.Errorf("expected error with invalid CIDR")
	}

	_, err = NewIPAccessList(nil, []string{"999.999.999.999"})
	if err == nil {
		t.Errorf("expected error with invalid IP")
	}
}

func TestIPAccessList_ExtractClientIP(t *testing.T) {
	// From X-Forwarded-For
	req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	req.Header.Set("X-Forwarded-For", "203.0.113.195, 10.0.0.1")
	ip := ExtractClientIP(req)
	if ip == nil || ip.String() != "203.0.113.195" {
		t.Errorf("expected client IP 203.0.113.195, got %v", ip)
	}

	// From X-Real-IP
	req2, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	req2.Header.Set("X-Real-IP", "198.51.100.7")
	ip2 := ExtractClientIP(req2)
	if ip2 == nil || ip2.String() != "198.51.100.7" {
		t.Errorf("expected client IP 198.51.100.7, got %v", ip2)
	}
}
