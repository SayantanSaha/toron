package router

import (
	"net/http"
	"testing"

	"toron/pkg/httpparser"
)

func TestCORS_PreflightAllowed(t *testing.T) {
	cfg := CORSConfig{
		Enabled:          true,
		AllowOrigins:     []string{"https://app.example.com"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"X-Cache"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	middleware := NewCORSMiddleware(cfg)
	handlerCalled := false
	handler := middleware(func(req *httpparser.Request, res *httpparser.Response) {
		handlerCalled = true
	})

	req, _ := httpparser.NewRequest("OPTIONS", "/api/data", "HTTP/1.1")
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	res := httpparser.NewResponse()
	handler(req, res)

	if handlerCalled {
		t.Errorf("preflight request should not invoke downstream handler")
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", res.StatusCode)
	}
	if origin := res.Header.Get("Access-Control-Allow-Origin"); origin != "https://app.example.com" {
		t.Errorf("expected Access-Control-Allow-Origin 'https://app.example.com', got %q", origin)
	}
	if creds := res.Header.Get("Access-Control-Allow-Credentials"); creds != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials 'true', got %q", creds)
	}
	if maxAge := res.Header.Get("Access-Control-Max-Age"); maxAge != "3600" {
		t.Errorf("expected Access-Control-Max-Age '3600', got %q", maxAge)
	}
	if expose := res.Header.Get("Access-Control-Expose-Headers"); expose != "X-Cache" {
		t.Errorf("expected Access-Control-Expose-Headers 'X-Cache', got %q", expose)
	}
}

func TestCORS_PreflightDisallowed(t *testing.T) {
	cfg := CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"https://trusted.com"},
	}

	middleware := NewCORSMiddleware(cfg)
	handler := middleware(func(req *httpparser.Request, res *httpparser.Response) {})

	req, _ := httpparser.NewRequest("OPTIONS", "/api/data", "HTTP/1.1")
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	res := httpparser.NewResponse()
	handler(req, res)

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for disallowed preflight origin, got %d", res.StatusCode)
	}
}

func TestCORS_WildcardOrigin(t *testing.T) {
	cfg := CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"*"},
	}

	middleware := NewCORSMiddleware(cfg)
	handler := middleware(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	req, _ := httpparser.NewRequest("GET", "/api/public", "HTTP/1.1")
	req.Header.Set("Origin", "https://anywhere.org")

	res := httpparser.NewResponse()
	handler(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if origin := res.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin '*', got %q", origin)
	}
}

func TestCORS_SubdomainWildcard(t *testing.T) {
	cfg := CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"https://*.toron.dev"},
	}

	middleware := NewCORSMiddleware(cfg)
	handler := middleware(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
	req.Header.Set("Origin", "https://sub.toron.dev")

	res := httpparser.NewResponse()
	handler(req, res)

	if origin := res.Header.Get("Access-Control-Allow-Origin"); origin != "https://sub.toron.dev" {
		t.Errorf("expected Access-Control-Allow-Origin 'https://sub.toron.dev', got %q", origin)
	}
}

func TestCORS_ActualRequestWithoutOrigin(t *testing.T) {
	cfg := CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"https://app.com"},
	}

	middleware := NewCORSMiddleware(cfg)
	handlerCalled := false
	handler := middleware(func(req *httpparser.Request, res *httpparser.Response) {
		handlerCalled = true
		res.SetStatus(http.StatusOK)
	})

	req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
	res := httpparser.NewResponse()
	handler(req, res)

	if !handlerCalled {
		t.Fatalf("expected handler to be called when no Origin header is present")
	}
	if origin := res.Header.Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("expected empty CORS header, got %q", origin)
	}
}
