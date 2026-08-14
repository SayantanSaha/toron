package router

import (
	"net/http"
	"testing"

	"toron/pkg/httpparser"
)

func TestSecurityHeaders_Injection(t *testing.T) {
	cfg := SecurityHeadersConfig{
		Enabled:            true,
		HSTS:               "max-age=31536000; includeSubDomains; preload",
		ContentTypeOptions: "nosniff",
		FrameOptions:       "DENY",
		ReferrerPolicy:     "strict-origin-when-cross-origin",
		CSP:                "default-src 'self'; script-src 'self'",
		PermissionsPolicy:  "camera=(), geolocation=()",
	}

	middleware := NewSecurityHeadersMiddleware(cfg)
	handler := middleware(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("hello security")
	})

	req, _ := httpparser.NewRequest("GET", "/secure", "HTTP/1.1")
	res := httpparser.NewResponse()
	handler(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if hsts := res.Header.Get("Strict-Transport-Security"); hsts != cfg.HSTS {
		t.Errorf("expected HSTS %q, got %q", cfg.HSTS, hsts)
	}
	if cto := res.Header.Get("X-Content-Type-Options"); cto != "nosniff" {
		t.Errorf("expected X-Content-Type-Options 'nosniff', got %q", cto)
	}
	if fo := res.Header.Get("X-Frame-Options"); fo != "DENY" {
		t.Errorf("expected X-Frame-Options 'DENY', got %q", fo)
	}
	if rp := res.Header.Get("Referrer-Policy"); rp != "strict-origin-when-cross-origin" {
		t.Errorf("expected Referrer-Policy 'strict-origin-when-cross-origin', got %q", rp)
	}
	if csp := res.Header.Get("Content-Security-Policy"); csp != cfg.CSP {
		t.Errorf("expected CSP %q, got %q", cfg.CSP, csp)
	}
	if pp := res.Header.Get("Permissions-Policy"); pp != cfg.PermissionsPolicy {
		t.Errorf("expected Permissions-Policy %q, got %q", cfg.PermissionsPolicy, pp)
	}
}

func TestSecurityHeaders_DefaultConfig(t *testing.T) {
	cfg := DefaultSecurityHeadersConfig()
	if !cfg.Enabled {
		t.Errorf("expected DefaultSecurityHeadersConfig to be enabled")
	}
	if cfg.ContentTypeOptions != "nosniff" {
		t.Errorf("expected nosniff, got %q", cfg.ContentTypeOptions)
	}
	if cfg.FrameOptions != "DENY" {
		t.Errorf("expected DENY, got %q", cfg.FrameOptions)
	}
}
