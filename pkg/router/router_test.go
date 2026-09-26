package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/logging"
	"toron/pkg/proxy"
	"toron/pkg/router"
)

func TestRouter_MatchingAndMiddleware(t *testing.T) {
	r := router.New()

	var middlewareExecuted bool
	mw := func(next router.HandlerFunc) router.HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			middlewareExecuted = true
			next(req, res)
		}
	}
	r.Use(mw)

	var handlerExecuted bool
	r.GET("/api/test", func(req *httpparser.Request, res *httpparser.Response) {
		handlerExecuted = true
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("hello route")
	})

	// Test 1: Match route
	req, _ := httpparser.NewRequest("GET", "/api/test", "HTTP/1.1")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if !middlewareExecuted {
		t.Error("expected middleware to be executed")
	}
	if !handlerExecuted {
		t.Error("expected handler to be executed")
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}

	// Test 2: Method Not Allowed
	req405, _ := httpparser.NewRequest("POST", "/api/test", "HTTP/1.1")
	res405 := httpparser.NewResponse()

	r.ServeHTTP(req405, res405)
	if res405.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", res405.StatusCode)
	}

	// Test 3: Not Found
	req404, _ := httpparser.NewRequest("GET", "/nonexistent", "HTTP/1.1")
	res404 := httpparser.NewResponse()

	r.ServeHTTP(req404, res404)
	if res404.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", res404.StatusCode)
	}
}

func TestRouter_HeaderBasedRouting(t *testing.T) {
	r := router.New()

	// Register header-conditional route (API v2)
	r.GETHeader("/api/data", "X-Version", "v2", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("v2 API")
	})

	// Register default path route (API v1)
	r.GET("/api/data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("v1 API")
	})

	// Test 1: Request with X-Version: v2
	reqV2, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
	reqV2.Header.Set("X-Version", "v2")
	resV2 := httpparser.NewResponse()

	r.ServeHTTP(reqV2, resV2)

	if resV2.Body.String() != "v2 API" {
		t.Errorf("expected 'v2 API', got %q", resV2.Body.String())
	}

	// Test 2: Request without header -> falls back to default v1 API
	reqV1, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
	resV1 := httpparser.NewResponse()

	r.ServeHTTP(reqV1, resV1)

	if resV1.Body.String() != "v1 API" {
		t.Errorf("expected 'v1 API', got %q", resV1.Body.String())
	}
}

func TestRouter_RecoveryMiddleware(t *testing.T) {
	r := router.New()
	r.Use(router.RecoveryMiddleware())

	r.GET("/panic", func(req *httpparser.Request, res *httpparser.Response) {
		panic("test panic")
	})

	req, _ := httpparser.NewRequest("GET", "/panic", "HTTP/1.1")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", res.StatusCode)
	}
}

func TestRouter_StaticFileServing(t *testing.T) {
	tmpDir := t.TempDir()

	// Write index.html and style.css in tmpDir
	htmlContent := "<html><body>Hello Static Website</body></html>"
	cssContent := "body { color: red; }"

	_ = os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(htmlContent), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "style.css"), []byte(cssContent), 0644)

	r := router.New()
	r.Static("/static", tmpDir)

	// Test 1: GET /static/ (should serve index.html)
	req1, _ := httpparser.NewRequest("GET", "/static/", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for /static/, got %d", res1.StatusCode)
	}
	if res1.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected text/html content type, got %q", res1.Header.Get("Content-Type"))
	}
	if res1.Body.String() != htmlContent {
		t.Errorf("expected body %q, got %q", htmlContent, res1.Body.String())
	}

	// Test 2: GET /static/style.css
	req2, _ := httpparser.NewRequest("GET", "/static/style.css", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for style.css, got %d", res2.StatusCode)
	}
	if res2.Body.String() != cssContent {
		t.Errorf("expected css body, got %q", res2.Body.String())
	}

	// Test 3: Path Traversal Attempt
	req3, _ := httpparser.NewRequest("GET", "/static/../../etc/passwd", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)

	if res3.StatusCode != http.StatusForbidden && res3.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 403 Forbidden or 404 for traversal, got %d", res3.StatusCode)
	}

	// Test 4: Trailing slash redirect for GET /static
	req4, _ := httpparser.NewRequest("GET", "/static", "HTTP/1.1")
	res4 := httpparser.NewResponse()
	r.ServeHTTP(req4, res4)

	if res4.StatusCode != http.StatusFound {
		t.Errorf("expected status 302 Found for /static, got %d", res4.StatusCode)
	}
	if res4.Header.Get("Location") != "/static/" {
		t.Errorf("expected Location /static/, got %q", res4.Header.Get("Location"))
	}
}

func TestRouter_ProxyBalancer(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-1"))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer server2.Close()

	r := router.New()
	err := r.ProxyBalancer("/api", []string{server1.URL, server2.URL}, proxy.AlgorithmRoundRobin)
	if err != nil {
		t.Fatalf("failed to configure ProxyBalancer: %v", err)
	}

	// Request 1 -> server1
	req1, _ := httpparser.NewRequest("GET", "/api/users", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if body1 := res1.BodyString(); body1 != "backend-1" {
		t.Errorf("request 1: expected backend-1, got %q", body1)
	}

	// Request 2 -> server2
	req2, _ := httpparser.NewRequest("GET", "/api/users", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if body2 := res2.BodyString(); body2 != "backend-2" {
		t.Errorf("expected backend-2 response, got %q", body2)
	}
}

func TestRouter_DomainHostRouting(t *testing.T) {
	r := router.New()

	r.GETHost("api.toron.local", "/v1/data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("api-domain-matched")
	})

	r.GETHost("admin.toron.local", "/v1/data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("admin-domain-matched")
	})

	// 1. Match api.toron.local
	req1, _ := httpparser.NewRequest("GET", "/v1/data", "HTTP/1.1")
	req1.Header.Set("Host", "api.toron.local:8080")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK || res1.Body.String() != "api-domain-matched" {
		t.Errorf("expected api-domain-matched, got status %d body %q", res1.StatusCode, res1.Body.String())
	}

	// 2. Match admin.toron.local
	req2, _ := httpparser.NewRequest("GET", "/v1/data", "HTTP/1.1")
	req2.Header.Set("Host", "admin.toron.local")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusOK || res2.Body.String() != "admin-domain-matched" {
		t.Errorf("expected admin-domain-matched, got status %d body %q", res2.StatusCode, res2.Body.String())
	}
}

func TestRouter_RoutePrefixStaticWithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	htmlContent := "<html><body>Domain Match Static Page</body></html>"
	_ = os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(htmlContent), 0644)

	r := router.New()

	// Register static route with host and header matching via RoutePrefix
	err := r.RoutePrefix(router.RouteTypeStatic, "docs.toron.local", "/docs", map[string]string{"X-UI": "v2"}, tmpDir, proxy.ProxyOptions{})
	if err != nil {
		t.Fatalf("failed to register static route via RoutePrefix: %v", err)
	}

	// 1. Matching request with correct host and header
	req, _ := httpparser.NewRequest("GET", "/docs/", "HTTP/1.1")
	req.Header.Set("Host", "docs.toron.local")
	req.Header.Set("X-UI", "v2")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.Body.String() != htmlContent {
		t.Errorf("expected body %q, got %q", htmlContent, res.Body.String())
	}

	// 2. Request with wrong host -> 404 Not Found
	reqWrongHost, _ := httpparser.NewRequest("GET", "/docs/", "HTTP/1.1")
	reqWrongHost.Header.Set("Host", "other.toron.local")
	reqWrongHost.Header.Set("X-UI", "v2")
	resWrongHost := httpparser.NewResponse()

	r.ServeHTTP(reqWrongHost, resWrongHost)

	if resWrongHost.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-matching host, got %d", resWrongHost.StatusCode)
	}
}

func TestRouter_Reset(t *testing.T) {
	r := router.New()
	r.GET("/api/v1", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("v1")
	})

	req1, _ := httpparser.NewRequest("GET", "/api/v1", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK before reset, got %d", res1.StatusCode)
	}

	r.Reset()

	req2, _ := httpparser.NewRequest("GET", "/api/v1", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found after reset, got %d", res2.StatusCode)
	}
}

func setupSPAFixture(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// index.html
	_ = os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<!DOCTYPE html><html><body>Kite SPA Root</body></html>"), 0644)
	// app.html
	_ = os.WriteFile(filepath.Join(tmpDir, "app.html"), []byte("<!DOCTYPE html><html><body>Custom Fallback App</body></html>"), 0644)
	// assets/style.css
	_ = os.MkdirAll(filepath.Join(tmpDir, "assets"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "assets", "style.css"), []byte("body { margin: 0; background: #fafafa; }"), 0644)
	// dashboard/index.html
	_ = os.MkdirAll(filepath.Join(tmpDir, "dashboard"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "dashboard", "index.html"), []byte("<!DOCTYPE html><html><body>Dashboard Subdir</body></html>"), 0644)

	return tmpDir
}

func TestRouter_SPA_PhysicalFileServing(t *testing.T) {
	tmpDir := setupSPAFixture(t)
	r := router.New()
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/kite", nil, tmpDir, proxy.ProxyOptions{SPA: true, Fallback: "index.html"})
	if err != nil {
		t.Fatalf("failed to register route: %v", err)
	}

	// 1. GET /kite/assets/style.css
	req1, _ := httpparser.NewRequest("GET", "/kite/assets/style.css", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for style.css, got %d", res1.StatusCode)
	}
	if !strings.Contains(res1.Header.Get("Content-Type"), "text/css") {
		t.Errorf("expected text/css, got %q", res1.Header.Get("Content-Type"))
	}
	if res1.Body.String() != "body { margin: 0; background: #fafafa; }" {
		t.Errorf("unexpected body: %q", res1.Body.String())
	}

	// 2. GET /kite/
	req2, _ := httpparser.NewRequest("GET", "/kite/", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /kite/, got %d", res2.StatusCode)
	}
	if !strings.Contains(res2.Body.String(), "Kite SPA Root") {
		t.Errorf("expected index.html content, got %q", res2.Body.String())
	}

	// 3. GET /kite/dashboard/
	req3, _ := httpparser.NewRequest("GET", "/kite/dashboard/", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /kite/dashboard/, got %d", res3.StatusCode)
	}
	if !strings.Contains(res3.Body.String(), "Dashboard Subdir") {
		t.Errorf("expected dashboard index.html, got %q", res3.Body.String())
	}
}

func TestRouter_SPA_NavigationFallback(t *testing.T) {
	tmpDir := setupSPAFixture(t)
	r := router.New()
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/kite", nil, tmpDir, proxy.ProxyOptions{SPA: true, Fallback: "index.html"})
	if err != nil {
		t.Fatalf("failed to register route: %v", err)
	}

	testPaths := []string{
		"/kite/signin",
		"/kite/devops",
		"/kite/deep/nested/route",
	}

	for _, path := range testPaths {
		req, _ := httpparser.NewRequest("GET", path, "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for path %q, got %d", path, res.StatusCode)
		}
		if res.Header.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Errorf("expected text/html; charset=utf-8 for %q, got %q", path, res.Header.Get("Content-Type"))
		}
		if !strings.Contains(res.Body.String(), "Kite SPA Root") {
			t.Errorf("expected fallback body for %q, got %q", path, res.Body.String())
		}
	}

	// HEAD method test
	headReq, _ := httpparser.NewRequest("HEAD", "/kite/devops", "HTTP/1.1")
	headRes := httpparser.NewResponse()
	r.ServeHTTP(headReq, headRes)
	if headRes.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for HEAD /kite/devops, got %d", headRes.StatusCode)
	}
	if headRes.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected text/html; charset=utf-8 for HEAD, got %q", headRes.Header.Get("Content-Type"))
	}
	if headRes.Body.Len() != 0 {
		t.Errorf("expected empty body for HEAD, got %q", headRes.Body.String())
	}
}

func TestRouter_SPA_AssetProtection404(t *testing.T) {
	tmpDir := setupSPAFixture(t)
	r := router.New()
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/kite", nil, tmpDir, proxy.ProxyOptions{SPA: true})
	if err != nil {
		t.Fatalf("failed to register route: %v", err)
	}

	assetPaths := []string{
		"/kite/assets/missing.js",
		"/kite/logo.png",
		"/kite/styles/main.css",
		"/kite/data/config.json",
	}

	for _, path := range assetPaths {
		req, _ := httpparser.NewRequest("GET", path, "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for missing asset %q, got %d", path, res.StatusCode)
		}
		if strings.Contains(res.Body.String(), "Kite SPA Root") {
			t.Errorf("expected asset %q to not mask 404 with fallback HTML", path)
		}
	}
}

func TestRouter_SPA_CustomFallback(t *testing.T) {
	tmpDir := setupSPAFixture(t)
	r := router.New()

	// Explicit SPA with custom fallback
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/custom", nil, tmpDir, proxy.ProxyOptions{SPA: true, Fallback: "app.html"})
	if err != nil {
		t.Fatalf("failed to register /custom route: %v", err)
	}

	// Implicit SPA with custom fallback (fallback != "")
	err = r.RoutePrefix(router.RouteTypeStatic, "", "/portal", nil, tmpDir, proxy.ProxyOptions{Fallback: "app.html"})
	if err != nil {
		t.Fatalf("failed to register /portal route: %v", err)
	}

	customPaths := []string{
		"/custom/settings",
		"/custom/user/profile",
		"/portal/analytics",
	}

	for _, path := range customPaths {
		req, _ := httpparser.NewRequest("GET", path, "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for %q, got %d", path, res.StatusCode)
		}
		if res.Header.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Errorf("expected text/html; charset=utf-8 for %q, got %q", path, res.Header.Get("Content-Type"))
		}
		if !strings.Contains(res.Body.String(), "Custom Fallback App") {
			t.Errorf("expected custom fallback body for %q, got %q", path, res.Body.String())
		}
	}
}

func TestRouter_Static_NonSPABackwardCompatibility(t *testing.T) {
	tmpDir := setupSPAFixture(t)
	r := router.New()
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/legacy", nil, tmpDir, proxy.ProxyOptions{SPA: false, Fallback: ""})
	if err != nil {
		t.Fatalf("failed to register route: %v", err)
	}

	// Virtual route without extension should return 404
	req1, _ := httpparser.NewRequest("GET", "/legacy/missing-page", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-SPA missing page, got %d", res1.StatusCode)
	}

	// Asset with extension should return 404
	req2, _ := httpparser.NewRequest("GET", "/legacy/missing-script.js", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-SPA missing script, got %d", res2.StatusCode)
	}
}

func TestRouter_SPA_SecurityPathTraversal(t *testing.T) {
	tmpDir := setupSPAFixture(t)
	r := router.New()
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/kite", nil, tmpDir, proxy.ProxyOptions{SPA: true})
	if err != nil {
		t.Fatalf("failed to register route: %v", err)
	}

	traversalPaths := []string{
		"/kite/../",
		"/kite/../../../etc/passwd",
		"/kite/..%2f..%2fetc/passwd",
	}

	for _, path := range traversalPaths {
		req, _ := httpparser.NewRequest("GET", path, "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusNotFound {
			t.Errorf("expected 403 or 404 for traversal path %q, got %d", path, res.StatusCode)
		}
	}

	// Malicious fallback configuration
	r2 := router.New()
	err = r2.RoutePrefix(router.RouteTypeStatic, "", "/malicious", nil, tmpDir, proxy.ProxyOptions{
		SPA:      true,
		Fallback: "../../../../etc/passwd",
	})
	if err != nil {
		t.Fatalf("failed to register malicious route: %v", err)
	}

	reqMal, _ := httpparser.NewRequest("GET", "/malicious/route", "HTTP/1.1")
	resMal := httpparser.NewResponse()
	r2.ServeHTTP(reqMal, resMal)

	if resMal.StatusCode != http.StatusNotFound && resMal.StatusCode != http.StatusForbidden {
		t.Errorf("expected 404 or 403 for malicious fallback escape, got %d", resMal.StatusCode)
	}
	if strings.Contains(resMal.Body.String(), "root:") {
		t.Errorf("critical security breach: etc/passwd content leaked!")
	}
}

func TestRouter_ShouldRedirectHTTP_RouteOverride(t *testing.T) {
	r := router.New()
	tmpDir := t.TempDir()

	falseVal := false
	trueVal := true

	// Route 1: static challenge with redirect_http = false
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/.well-known/acme-challenge", nil, tmpDir, proxy.ProxyOptions{
		RedirectHTTP: &falseVal,
	})
	if err != nil {
		t.Fatalf("failed to register exempt route: %v", err)
	}

	// Route 2: static app with default redirect (nil)
	err = r.RoutePrefix(router.RouteTypeStatic, "", "/app", nil, tmpDir, proxy.ProxyOptions{})
	if err != nil {
		t.Fatalf("failed to register default route: %v", err)
	}

	// Route 3: upstream route with explicit redirect_http = true
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	err = r.RoutePrefix(router.RouteTypeUpstream, "", "/api", nil, "", proxy.ProxyOptions{
		Targets:      []string{backend.URL},
		RedirectHTTP: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to register upstream route: %v", err)
	}

	// Test 1: Request matching exempt route -> should NOT redirect
	reqExempt, _ := httpparser.NewRequest("GET", "/.well-known/acme-challenge/test-token", "HTTP/1.1")
	reqExempt.Header.Set("Host", "example.com")
	if r.ShouldRedirectHTTP(reqExempt) {
		t.Errorf("expected ShouldRedirectHTTP false for /.well-known/acme-challenge, got true")
	}

	// Test 2: Request matching default route (/app) -> should redirect
	reqApp, _ := httpparser.NewRequest("GET", "/app/dashboard", "HTTP/1.1")
	reqApp.Header.Set("Host", "example.com")
	if !r.ShouldRedirectHTTP(reqApp) {
		t.Errorf("expected ShouldRedirectHTTP true for /app/dashboard, got false")
	}

	// Test 3: Request matching explicit true route (/api) -> should redirect
	reqAPI, _ := httpparser.NewRequest("GET", "/api/v1/users", "HTTP/1.1")
	reqAPI.Header.Set("Host", "example.com")
	if !r.ShouldRedirectHTTP(reqAPI) {
		t.Errorf("expected ShouldRedirectHTTP true for /api/v1/users, got false")
	}

	// Test 4: Unmatched path -> should redirect
	reqUnmatched, _ := httpparser.NewRequest("GET", "/unknown", "HTTP/1.1")
	reqUnmatched.Header.Set("Host", "example.com")
	if !r.ShouldRedirectHTTP(reqUnmatched) {
		t.Errorf("expected ShouldRedirectHTTP true for unmatched route, got false")
	}
}

func TestRouter_AccessLoggerMiddleware_RouteOverrides(t *testing.T) {
	tempDir := t.TempDir()
	defaultAccessLog := filepath.Join(tempDir, "default_access.log")
	customAccessLog := filepath.Join(tempDir, "custom_api_access.log")

	mgr, err := logging.NewLogManager(logging.Config{
		Format:    "text",
		AccessLog: defaultAccessLog,
	})
	if err != nil {
		t.Fatalf("failed to create LogManager: %v", err)
	}
	defer mgr.Close()

	r := router.New()
	r.Use(router.AccessLoggerMiddleware(mgr, r))

	// Route 1: Default access logging (no override)
	r.GET("/hello", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(200)
		_, _ = res.WriteString("hello")
	})

	// Route 2: Custom access log override
	staticDir := filepath.Join(tempDir, "static")
	_ = os.MkdirAll(staticDir, 0755)
	_ = os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("index content"), 0644)
	err = r.RoutePrefix(router.RouteTypeStatic, "", "/custom", nil, staticDir, proxy.ProxyOptions{
		AccessLog: customAccessLog,
	})
	if err != nil {
		t.Fatalf("failed to register custom static route: %v", err)
	}

	// Route 3: Silenced access log ("off")
	err = r.RoutePrefix(router.RouteTypeStatic, "", "/silent", nil, staticDir, proxy.ProxyOptions{
		AccessLog: "off",
	})
	if err != nil {
		t.Fatalf("failed to register silent route: %v", err)
	}

	// Request 1: hits /hello (default access log)
	req1, _ := httpparser.NewRequest("GET", "/hello", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res1.StatusCode)
	}

	// Request 2: hits /custom/index.html (custom access log)
	req2, _ := httpparser.NewRequest("GET", "/custom/index.html", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res2.StatusCode)
	}

	// Request 3: hits /silent/index.html (silenced)
	req3, _ := httpparser.NewRequest("GET", "/silent/index.html", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res3.StatusCode)
	}

	// Verify default access log contains /hello, and does not contain /custom or /silent
	defContent, err := os.ReadFile(defaultAccessLog)
	if err != nil {
		t.Fatalf("failed to read default access log: %v", err)
	}
	if !strings.Contains(string(defContent), "/hello") {
		t.Errorf("default access log missing /hello: %s", string(defContent))
	}
	if strings.Contains(string(defContent), "/custom") {
		t.Errorf("default access log should NOT contain /custom")
	}
	if strings.Contains(string(defContent), "/silent") {
		t.Errorf("default access log should NOT contain /silent")
	}

	// Verify custom access log contains /custom/index.html
	customContent, err := os.ReadFile(customAccessLog)
	if err != nil {
		t.Fatalf("failed to read custom access log: %v", err)
	}
	if !strings.Contains(string(customContent), "/custom/index.html") {
		t.Errorf("custom access log missing /custom/index.html: %s", string(customContent))
	}
}

func TestRouter_PathCanonicalizationAndTraversalGuards(t *testing.T) {
	r := router.New()

	publicCalled := false
	internalCalled := false
	rootCalled := false

	r.GET("/public", func(req *httpparser.Request, res *httpparser.Response) {
		publicCalled = true
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("public")
	})

	r.GET("/internal", func(req *httpparser.Request, res *httpparser.Response) {
		internalCalled = true
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("internal")
	})

	r.GET("/", func(req *httpparser.Request, res *httpparser.Response) {
		rootCalled = true
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("root")
	})

	// Test 1: Dot segment traversal (/public/../internal -> /internal) must NOT match /public
	req1, _ := httpparser.NewRequest("GET", "/public/../internal", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if publicCalled {
		t.Errorf("expected /public handler NOT to be called for /public/../internal")
	}
	if !internalCalled {
		t.Errorf("expected /internal handler to be called for /public/../internal, got status %d", res1.StatusCode)
	}

	// Test 2: Redundant slashes (//internal -> /internal)
	internalCalled = false
	req2, _ := httpparser.NewRequest("GET", "//internal", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if !internalCalled {
		t.Errorf("expected /internal handler to be called for //internal, got status %d", res2.StatusCode)
	}

	// Test 3: Traversal above root (/.. or /../../ -> /)
	req3, _ := httpparser.NewRequest("GET", "/../../", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if !rootCalled {
		t.Errorf("expected / handler to be called for /../../, got status %d", res3.StatusCode)
	}
}

// TestRouter_HandlePrefix_MethodAndMatcher verifies that Router.HandlePrefix and Router.HandlePrefixWithMatcher
// enforce HTTP methods, evaluate custom matchers, return 405 on method mismatch, and fall through cleanly (SEC-30).
func TestRouter_HandlePrefix_MethodAndMatcher(t *testing.T) {
	r := router.New()

	// Register prefix handler with matcher
	r.HandlePrefixWithMatcher("GET", "", "/v1/items", nil, func(p string) bool {
		return strings.HasPrefix(p, "/v1/items/") && !strings.Contains(p, "forbidden")
	}, func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("item matched")
	})

	// 1. Valid matching request
	req1, _ := httpparser.NewRequest("GET", "/v1/items/item-123", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != http.StatusOK || res1.Body.String() != "item matched" {
		t.Errorf("req1: want 200 item matched, got %d %q", res1.StatusCode, res1.Body.String())
	}

	// 2. Method mismatch on matching prefix -> 405 Method Not Allowed
	req2, _ := httpparser.NewRequest("POST", "/v1/items/item-123", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("req2: want 405, got %d", res2.StatusCode)
	}

	// 3. Matcher returns false (contains forbidden) -> 404 Not Found
	req3, _ := httpparser.NewRequest("GET", "/v1/items/forbidden-item", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.StatusCode != http.StatusNotFound {
		t.Errorf("req3: want 404, got %d", res3.StatusCode)
	}

	// 4. Unrelated path -> 404 Not Found
	req4, _ := httpparser.NewRequest("GET", "/v1/other", "HTTP/1.1")
	res4 := httpparser.NewResponse()
	r.ServeHTTP(req4, res4)
	if res4.StatusCode != http.StatusNotFound {
		t.Errorf("req4: want 404, got %d", res4.StatusCode)
	}
}

func TestRouter_ReplacePrefixRoutesBySource(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-1 payload"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-2 payload"))
	}))
	defer backend2.Close()

	tempDir := t.TempDir()
	staticFile := filepath.Join(tempDir, "index.html")
	if err := os.WriteFile(staticFile, []byte("static page content"), 0644); err != nil {
		t.Fatalf("failed to write test static file: %v", err)
	}

	r := router.New()

	// Scenario 1.1: Source Isolation
	// Register Route 1 (src: "config") and Route 2 (src: "k8s-ingress")
	err := r.RoutePrefix(router.RouteTypeStatic, "", "/static", nil, tempDir, proxy.ProxyOptions{})
	if err != nil {
		t.Fatalf("RoutePrefix static failed: %v", err)
	}
	err = r.RoutePrefixWithSource("k8s-ingress", router.RouteTypeUpstream, "", "/v1", nil, "", proxy.ProxyOptions{
		Targets: []string{backend1.URL},
	})
	if err != nil {
		t.Fatalf("RoutePrefixWithSource k8s-ingress failed: %v", err)
	}

	// Replace "k8s-ingress" routes with new /v2 spec
	newSpecs := []router.PrefixRouteSpec{
		{
			TargetType: router.RouteTypeUpstream,
			Prefix:     "/v2",
			Opts: proxy.ProxyOptions{
				Targets: []string{backend2.URL},
			},
		},
	}
	err = r.ReplacePrefixRoutesBySource("k8s-ingress", newSpecs)
	if err != nil {
		t.Fatalf("ReplacePrefixRoutesBySource failed: %v", err)
	}

	snaps := r.GetPrefixRoutes()
	if len(snaps) != 2 {
		t.Fatalf("expected 2 routes in snapshot, got %d", len(snaps))
	}
	if snaps[0].Source != "config" || snaps[0].Prefix != "/static" {
		t.Errorf("snaps[0] = %+v, want source=config prefix=/static", snaps[0])
	}
	if snaps[1].Source != "k8s-ingress" || snaps[1].Prefix != "/v2" {
		t.Errorf("snaps[1] = %+v, want source=k8s-ingress prefix=/v2", snaps[1])
	}

	// Dispatch request to /static/index.html -> 200 OK
	reqStatic, _ := httpparser.NewRequest("GET", "/static/index.html", "HTTP/1.1")
	resStatic := httpparser.NewResponse()
	r.ServeHTTP(reqStatic, resStatic)
	if resStatic.StatusCode != http.StatusOK || !strings.Contains(resStatic.Body.String(), "static page content") {
		t.Errorf("static route unexpected response: %d %q", resStatic.StatusCode, resStatic.Body.String())
	}

	// Dispatch request to evicted /v1/test -> 404 Not Found
	reqV1, _ := httpparser.NewRequest("GET", "/v1/test", "HTTP/1.1")
	resV1 := httpparser.NewResponse()
	r.ServeHTTP(reqV1, resV1)
	if resV1.StatusCode != http.StatusNotFound {
		t.Errorf("evicted /v1 route: want 404, got %d", resV1.StatusCode)
	}

	// Dispatch request to new /v2/test -> 200 OK (backend2)
	reqV2, _ := httpparser.NewRequest("GET", "/v2/test", "HTTP/1.1")
	resV2 := httpparser.NewResponse()
	r.ServeHTTP(reqV2, resV2)
	bodyV2 := resV2.BodyString()
	if resV2.StatusCode != http.StatusOK || !strings.Contains(bodyV2, "backend-2 payload") {
		t.Errorf("new /v2 route: want 200 backend-2 payload, got %d %q", resV2.StatusCode, bodyV2)
	}

	// Scenario 1.2: Empty Pruning
	// Currently routes are: Route 1 (config, /static), Route 2 (k8s-ingress, /v2)
	// Add another k8s-ingress route
	_ = r.RoutePrefixWithSource("k8s-ingress", router.RouteTypeUpstream, "", "/v3", nil, "", proxy.ProxyOptions{
		Targets: []string{backend1.URL},
	})
	if len(r.GetPrefixRoutes()) != 3 {
		t.Fatalf("expected 3 routes before empty pruning, got %d", len(r.GetPrefixRoutes()))
	}

	err = r.ReplacePrefixRoutesBySource("k8s-ingress", []router.PrefixRouteSpec{})
	if err != nil {
		t.Fatalf("ReplacePrefixRoutesBySource with empty slice failed: %v", err)
	}
	snapsAfterPrune := r.GetPrefixRoutes()
	if len(snapsAfterPrune) != 1 {
		t.Fatalf("expected exactly 1 route after empty pruning, got %d", len(snapsAfterPrune))
	}
	if snapsAfterPrune[0].Source != "config" || snapsAfterPrune[0].Prefix != "/static" {
		t.Errorf("remaining route = %+v, want source=config prefix=/static", snapsAfterPrune[0])
	}

	// Scenario 1.3: All-or-Nothing Rollback on Invalid Route Type
	// Re-add a valid k8s-ingress route
	_ = r.RoutePrefixWithSource("k8s-ingress", router.RouteTypeUpstream, "", "/api", nil, "", proxy.ProxyOptions{
		Targets: []string{backend1.URL},
	})
	snapsBeforeFail := r.GetPrefixRoutes()

	invalidTypeSpecs := []router.PrefixRouteSpec{
		{
			TargetType: router.RouteTypeUpstream,
			Prefix:     "/valid-v1",
			Opts:       proxy.ProxyOptions{Targets: []string{backend1.URL}},
		},
		{
			TargetType: router.RouteType("bogus"),
			Prefix:     "/invalid-v2",
		},
	}
	err = r.ReplacePrefixRoutesBySource("k8s-ingress", invalidTypeSpecs)
	if err == nil {
		t.Fatal("expected error on bogus route type, got nil")
	}
	snapsAfterFail := r.GetPrefixRoutes()
	if len(snapsAfterFail) != len(snapsBeforeFail) {
		t.Fatalf("table size changed on error: before=%d, after=%d", len(snapsBeforeFail), len(snapsAfterFail))
	}
	for i := range snapsBeforeFail {
		if snapsBeforeFail[i].Source != snapsAfterFail[i].Source ||
			snapsBeforeFail[i].Type != snapsAfterFail[i].Type ||
			snapsBeforeFail[i].Host != snapsAfterFail[i].Host ||
			snapsBeforeFail[i].Prefix != snapsAfterFail[i].Prefix {
			t.Errorf("route %d mismatch after rollback: %+v vs %+v", i, snapsBeforeFail[i], snapsAfterFail[i])
		}
	}

	// Scenario 1.4: Invalid RateLimit Rollback
	invalidRateSpecs := []router.PrefixRouteSpec{
		{
			TargetType: router.RouteTypeUpstream,
			Prefix:     "/rate-v1",
			Opts: proxy.ProxyOptions{
				Targets:   []string{backend1.URL},
				RateLimit: "invalid-rate-format",
			},
		},
	}
	err = r.ReplacePrefixRoutesBySource("k8s-ingress", invalidRateSpecs)
	if err == nil {
		t.Fatal("expected error on invalid rate limit, got nil")
	}
	if len(r.GetPrefixRoutes()) != len(snapsBeforeFail) {
		t.Fatalf("table modified after invalid rate limit: got %d routes", len(r.GetPrefixRoutes()))
	}

	// Scenario 1.5: Invalid Upstream URL Rollback
	invalidURLSpecs := []router.PrefixRouteSpec{
		{
			TargetType: router.RouteTypeUpstream,
			Prefix:     "/url-v1",
			Opts: proxy.ProxyOptions{
				Targets: []string{"http://bad host:9999"},
			},
		},
	}
	err = r.ReplacePrefixRoutesBySource("k8s-ingress", invalidURLSpecs)
	if err == nil {
		t.Fatal("expected error on bad host URL, got nil")
	}
	if len(r.GetPrefixRoutes()) != len(snapsBeforeFail) {
		t.Fatalf("table modified after invalid URL: got %d routes", len(r.GetPrefixRoutes()))
	}
}

func TestRouter_RoutePrefixWithSource_BackwardCompatibility(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("legacy backend payload"))
	}))
	defer backend.Close()

	r := router.New()

	// Call legacy RoutePrefix without specifying source
	err := r.RoutePrefix(router.RouteTypeUpstream, "api.example.com", "/legacy", nil, "", proxy.ProxyOptions{
		Targets: []string{backend.URL},
	})
	if err != nil {
		t.Fatalf("RoutePrefix failed: %v", err)
	}

	// Also call RoutePrefixWithSource with custom source
	err = r.RoutePrefixWithSource("custom-src", router.RouteTypeUpstream, "custom.example.com", "/custom", nil, "", proxy.ProxyOptions{
		Targets: []string{backend.URL},
	})
	if err != nil {
		t.Fatalf("RoutePrefixWithSource failed: %v", err)
	}

	// Inspect snapshots
	snaps := r.GetPrefixRoutes()
	if len(snaps) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(snaps))
	}

	// Verify legacy route defaults to source "config"
	if snaps[0].Source != "config" {
		t.Errorf("snaps[0].Source = %q, want %q", snaps[0].Source, "config")
	}
	if snaps[0].Host != "api.example.com" {
		t.Errorf("snaps[0].Host = %q, want api.example.com", snaps[0].Host)
	}
	if snaps[0].Prefix != "/legacy" {
		t.Errorf("snaps[0].Prefix = %q, want /legacy", snaps[0].Prefix)
	}
	if snaps[0].Type != "upstream" {
		t.Errorf("snaps[0].Type = %q, want upstream", snaps[0].Type)
	}

	// Verify custom-src route
	if snaps[1].Source != "custom-src" {
		t.Errorf("snaps[1].Source = %q, want custom-src", snaps[1].Source)
	}

	// Verify RoutesSnapshot parity
	routesSnaps := r.RoutesSnapshot()
	if len(routesSnaps) != len(snaps) {
		t.Fatalf("RoutesSnapshot length mismatch: %d vs %d", len(routesSnaps), len(snaps))
	}
	for i := range snaps {
		if snaps[i].Source != routesSnaps[i].Source ||
			snaps[i].Type != routesSnaps[i].Type ||
			snaps[i].Host != routesSnaps[i].Host ||
			snaps[i].Prefix != routesSnaps[i].Prefix {
			t.Errorf("RoutesSnapshot[%d] mismatch: %+v vs %+v", i, routesSnaps[i], snaps[i])
		}
	}

	// Verify request dispatching to legacy route works
	req, _ := httpparser.NewRequest("GET", "/legacy/test", "HTTP/1.1")
	req.Header.Set("Host", "api.example.com")
	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)
	body := res.BodyString()
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "legacy backend payload") {
		t.Errorf("legacy route response mismatch: %d %q", res.StatusCode, body)
	}
}

func TestRouter_HealthCheckCleanupOnRouteReplace(t *testing.T) {
	var probeCount int64
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			atomic.AddInt64(&probeCount, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	r := router.New()
	opts := proxy.ProxyOptions{
		Targets:             []string{mockUpstream.URL},
		HealthCheckPath:     "/healthz",
		HealthCheckInterval: 20 * time.Millisecond,
	}
	err := r.RoutePrefixWithSource("k8s-ingress", router.RouteTypeUpstream, "health.test", "/app", nil, "", opts)
	if err != nil {
		t.Fatalf("RoutePrefixWithSource failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	probesBefore := atomic.LoadInt64(&probeCount)
	if probesBefore == 0 {
		t.Fatalf("Expected health check probes before replacement, got 0")
	}

	// Evict route via ReplacePrefixRoutesBySource
	err = r.ReplacePrefixRoutesBySource("k8s-ingress", []router.PrefixRouteSpec{})
	if err != nil {
		t.Fatalf("ReplacePrefixRoutesBySource failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	probesAfter := atomic.LoadInt64(&probeCount)
	if probesAfter > probesBefore+1 {
		t.Errorf("Expected health checks to cease after eviction: before=%d, after=%d", probesBefore, probesAfter)
	}

	// Verify teardown on RemovePrefixRoute
	var probeCount2 int64
	mockUpstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			atomic.AddInt64(&probeCount2, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream2.Close()

	opts2 := proxy.ProxyOptions{
		Targets:             []string{mockUpstream2.URL},
		HealthCheckPath:     "/healthz",
		HealthCheckInterval: 20 * time.Millisecond,
	}
	err = r.RoutePrefixWithSource("k8s-ingress", router.RouteTypeUpstream, "health2.test", "/app2", nil, "", opts2)
	if err != nil {
		t.Fatalf("RoutePrefixWithSource 2 failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	p2Before := atomic.LoadInt64(&probeCount2)
	if p2Before == 0 {
		t.Fatalf("Expected health check probes on route 2, got 0")
	}

	r.RemovePrefixRoute("health2.test", "/app2")
	time.Sleep(100 * time.Millisecond)
	p2After := atomic.LoadInt64(&probeCount2)
	if p2After > p2Before+1 {
		t.Errorf("Expected health checks to cease after RemovePrefixRoute: before=%d, after=%d", p2Before, p2After)
	}

	// Verify teardown on Reset
	var probeCount3 int64
	mockUpstream3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			atomic.AddInt64(&probeCount3, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream3.Close()

	opts3 := proxy.ProxyOptions{
		Targets:             []string{mockUpstream3.URL},
		HealthCheckPath:     "/healthz",
		HealthCheckInterval: 20 * time.Millisecond,
	}
	err = r.RoutePrefixWithSource("k8s-ingress", router.RouteTypeUpstream, "health3.test", "/app3", nil, "", opts3)
	if err != nil {
		t.Fatalf("RoutePrefixWithSource 3 failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	p3Before := atomic.LoadInt64(&probeCount3)
	if p3Before == 0 {
		t.Fatalf("Expected health check probes on route 3, got 0")
	}

	r.Reset()
	time.Sleep(100 * time.Millisecond)
	p3After := atomic.LoadInt64(&probeCount3)
	if p3After > p3Before+1 {
		t.Errorf("Expected health checks to cease after Reset: before=%d, after=%d", p3Before, p3After)
	}
}

func TestRouter_SpecificityOrdering_NoCanaryShadowing(t *testing.T) {
	genericServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("generic"))
	}))
	defer genericServer.Close()

	canaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("canary"))
	}))
	defer canaryServer.Close()

	r := router.New()

	// Register generic fallback FIRST, and canary SECOND (adverse order)
	genericSpec := router.PrefixRouteSpec{
		TargetType: router.RouteTypeUpstream,
		Prefix:     "/api",
		Opts:       proxy.ProxyOptions{Targets: []string{genericServer.URL}},
	}
	canarySpec := router.PrefixRouteSpec{
		TargetType: router.RouteTypeUpstream,
		Prefix:     "/api",
		Headers:    map[string]string{"X-Version": "canary"},
		Opts:       proxy.ProxyOptions{Targets: []string{canaryServer.URL}},
	}

	err := r.ReplacePrefixRoutesBySource("oci-discovery", []router.PrefixRouteSpec{genericSpec, canarySpec})
	if err != nil {
		t.Fatalf("ReplacePrefixRoutesBySource failed: %v", err)
	}

	// Verify internal router ordering: Canary must precede Generic
	routes := r.GetPrefixRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if len(routes[0].Headers) != 1 || routes[0].Headers["X-Version"] != "canary" {
		t.Errorf("expected routes[0] to be canary route, got %+v", routes[0])
	}
	if len(routes[1].Headers) != 0 {
		t.Errorf("expected routes[1] to be generic route, got %+v", routes[1])
	}

	// Request with Canary header -> routes to canary
	reqCanary, _ := httpparser.NewRequest("GET", "/api/users", "HTTP/1.1")
	reqCanary.Header.Set("X-Version", "canary")
	resCanary := httpparser.NewResponse()
	r.ServeHTTP(reqCanary, resCanary)
	canaryBody := resCanary.BodyString()
	if resCanary.StatusCode != http.StatusOK || canaryBody != "canary" {
		t.Errorf("canary request failed: status %d, body %q", resCanary.StatusCode, canaryBody)
	}

	// Request without header -> falls through to generic
	reqGeneric, _ := httpparser.NewRequest("GET", "/api/users", "HTTP/1.1")
	resGeneric := httpparser.NewResponse()
	r.ServeHTTP(reqGeneric, resGeneric)
	genericBody := resGeneric.BodyString()
	if resGeneric.StatusCode != http.StatusOK || genericBody != "generic" {
		t.Errorf("generic request failed: status %d, body %q", resGeneric.StatusCode, genericBody)
	}

	// Multi-Tier Specificity Hierarchy Verification (TC-095-06)
	r2 := router.New()
	specs := []router.PrefixRouteSpec{
		{TargetType: router.RouteTypeUpstream, Prefix: "/api", Host: "", Opts: proxy.ProxyOptions{Targets: []string{genericServer.URL}}},                                                                              // Route A
		{TargetType: router.RouteTypeUpstream, Prefix: "/api/v1/auth", Host: "", Opts: proxy.ProxyOptions{Targets: []string{genericServer.URL}}},                                                                      // Route B
		{TargetType: router.RouteTypeUpstream, Prefix: "/api/v1", Host: "", Opts: proxy.ProxyOptions{Targets: []string{genericServer.URL}}},                                                                           // Route C
		{TargetType: router.RouteTypeUpstream, Prefix: "/api", Host: "api.example.com", Opts: proxy.ProxyOptions{Targets: []string{genericServer.URL}}},                                                               // Route D
		{TargetType: router.RouteTypeUpstream, Prefix: "/api", Host: "api.example.com", Headers: map[string]string{"X-Tier": "gold"}, Opts: proxy.ProxyOptions{Targets: []string{genericServer.URL}}},                 // Route E
		{TargetType: router.RouteTypeUpstream, Prefix: "/api", Host: "api.example.com", Headers: map[string]string{"X-Tier": "gold"}, Method: "POST", Opts: proxy.ProxyOptions{Targets: []string{genericServer.URL}}}, // Route F
	}

	err = r2.ReplacePrefixRoutesBySource("oci-discovery", specs)
	if err != nil {
		t.Fatalf("ReplacePrefixRoutesBySource failed: %v", err)
	}

	r2Routes := r2.GetPrefixRoutes()
	if len(r2Routes) != 6 {
		t.Fatalf("expected 6 routes, got %d", len(r2Routes))
	}

	// Expected order: B (/api/v1/auth), C (/api/v1), F (/api, api.example.com, gold, POST), E (/api, api.example.com, gold, any), D (/api, api.example.com), A (/api, "")
	if r2Routes[0].Prefix != "/api/v1/auth" {
		t.Errorf("rank 0: expected /api/v1/auth, got %+v", r2Routes[0])
	}
	if r2Routes[1].Prefix != "/api/v1" {
		t.Errorf("rank 1: expected /api/v1, got %+v", r2Routes[1])
	}
	if r2Routes[2].Prefix != "/api" || r2Routes[2].Host != "api.example.com" || r2Routes[2].Method != "POST" || len(r2Routes[2].Headers) != 1 {
		t.Errorf("rank 2: expected Route F, got %+v", r2Routes[2])
	}
	if r2Routes[3].Prefix != "/api" || r2Routes[3].Host != "api.example.com" || r2Routes[3].Method != "" || len(r2Routes[3].Headers) != 1 {
		t.Errorf("rank 3: expected Route E, got %+v", r2Routes[3])
	}
	if r2Routes[4].Prefix != "/api" || r2Routes[4].Host != "api.example.com" || len(r2Routes[4].Headers) != 0 {
		t.Errorf("rank 4: expected Route D, got %+v", r2Routes[4])
	}
	if r2Routes[5].Prefix != "/api" || r2Routes[5].Host != "" {
		t.Errorf("rank 5: expected Route A, got %+v", r2Routes[5])
	}
}

func TestRouter_PrefixRouteSpec_MethodMatching(t *testing.T) {
	postServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("post-handler"))
	}))
	defer postServer.Close()

	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fallback-handler"))
	}))
	defer fallbackServer.Close()

	r := router.New()

	// Single method route registration
	spec := router.PrefixRouteSpec{
		TargetType: router.RouteTypeUpstream,
		Method:     "post",
		Prefix:     "/submit",
		Opts:       proxy.ProxyOptions{Targets: []string{postServer.URL}},
	}
	err := r.ReplacePrefixRoutesBySource("oci-discovery", []router.PrefixRouteSpec{spec})
	if err != nil {
		t.Fatalf("ReplacePrefixRoutesBySource failed: %v", err)
	}

	snaps := r.GetPrefixRoutes()
	if len(snaps) != 1 || snaps[0].Method != "POST" {
		t.Fatalf("expected 1 route with Method=POST, got %+v", snaps)
	}

	// POST /submit -> 200 OK
	reqPost, _ := httpparser.NewRequest("POST", "/submit", "HTTP/1.1")
	resPost := httpparser.NewResponse()
	r.ServeHTTP(reqPost, resPost)
	if bodyPost := resPost.BodyString(); resPost.StatusCode != http.StatusOK || bodyPost != "post-handler" {
		t.Errorf("POST /submit failed: status %d, body %q", resPost.StatusCode, bodyPost)
	}

	// GET /submit -> 405 Method Not Allowed
	reqGet, _ := httpparser.NewRequest("GET", "/submit", "HTTP/1.1")
	resGet := httpparser.NewResponse()
	r.ServeHTTP(reqGet, resGet)
	if resGet.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /submit: expected 405, got %d", resGet.StatusCode)
	}

	// PUT /submit -> 405 Method Not Allowed
	reqPut, _ := httpparser.NewRequest("PUT", "/submit", "HTTP/1.1")
	resPut := httpparser.NewResponse()
	r.ServeHTTP(reqPut, resPut)
	if resPut.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("PUT /submit: expected 405, got %d", resPut.StatusCode)
	}

	// Method route with generic verb fallback
	rFallback := router.New()
	postRoute := router.PrefixRouteSpec{
		TargetType: router.RouteTypeUpstream,
		Method:     "POST",
		Prefix:     "/items",
		Opts:       proxy.ProxyOptions{Targets: []string{postServer.URL}},
	}
	anyRoute := router.PrefixRouteSpec{
		TargetType: router.RouteTypeUpstream,
		Method:     "",
		Prefix:     "/items",
		Opts:       proxy.ProxyOptions{Targets: []string{fallbackServer.URL}},
	}
	err = rFallback.ReplacePrefixRoutesBySource("oci-discovery", []router.PrefixRouteSpec{postRoute, anyRoute})
	if err != nil {
		t.Fatalf("ReplacePrefixRoutesBySource failed: %v", err)
	}

	// POST /items -> post-handler
	reqItemsPost, _ := httpparser.NewRequest("POST", "/items", "HTTP/1.1")
	resItemsPost := httpparser.NewResponse()
	rFallback.ServeHTTP(reqItemsPost, resItemsPost)
	if bodyItemsPost := resItemsPost.BodyString(); resItemsPost.StatusCode != http.StatusOK || bodyItemsPost != "post-handler" {
		t.Errorf("POST /items: want 200 post-handler, got %d %q", resItemsPost.StatusCode, bodyItemsPost)
	}

	// GET /items -> fallback-handler
	reqItemsGet, _ := httpparser.NewRequest("GET", "/items", "HTTP/1.1")
	resItemsGet := httpparser.NewResponse()
	rFallback.ServeHTTP(reqItemsGet, resItemsGet)
	if bodyItemsGet := resItemsGet.BodyString(); resItemsGet.StatusCode != http.StatusOK || bodyItemsGet != "fallback-handler" {
		t.Errorf("GET /items: want 200 fallback-handler, got %d %q", resItemsGet.StatusCode, bodyItemsGet)
	}

	// DELETE /items -> fallback-handler
	reqItemsDel, _ := httpparser.NewRequest("DELETE", "/items", "HTTP/1.1")
	resItemsDel := httpparser.NewResponse()
	rFallback.ServeHTTP(reqItemsDel, resItemsDel)
	if bodyItemsDel := resItemsDel.BodyString(); resItemsDel.StatusCode != http.StatusOK || bodyItemsDel != "fallback-handler" {
		t.Errorf("DELETE /items: want 200 fallback-handler, got %d %q", resItemsDel.StatusCode, bodyItemsDel)
	}
}

// TC-116-03, TC-116-04, TC-116-05, TC-116-06, TC-116-07:
// Route-Aware Static Prefix Escape Guard and ADR-062 Canonicalization Preservations (REQ-116 / ADR-116 / TASK-139.2 / TASK-139.5)
func TestRouter_RouteAwareStaticPrefixEscapeGuard(t *testing.T) {
	tempBase := t.TempDir()
	staticDir := filepath.Join(tempBase, "static_dashboard")
	if err := os.MkdirAll(filepath.Join(staticDir, "assets"), 0755); err != nil {
		t.Fatalf("failed to create static assets dir: %v", err)
	}

	indexContent := "<html><body>Static Index</body></html>"
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	jsContent := `console.log("app");`
	if err := os.WriteFile(filepath.Join(staticDir, "assets", "app.js"), []byte(jsContent), 0644); err != nil {
		t.Fatalf("failed to write app.js: %v", err)
	}

	// Deploy canary traversal secret file outside staticDir
	canaryPath := filepath.Join(tempBase, "canary_traversal.txt")
	canaryContent := "TORON_TRAVERSAL_CANARY_SECRET_DATA_DO_NOT_LEAK"
	if err := os.WriteFile(canaryPath, []byte(canaryContent), 0644); err != nil {
		t.Fatalf("failed to write canary file: %v", err)
	}

	// Construct router WITHOUT WAF middleware attached (testing standalone router-level defense)
	r := router.New()
	r.Static("/internal/dashboard", staticDir)

	// Register exact API routes to verify ADR-062 non-static canonicalization
	r.GET("/public", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("public")
	})
	r.GET("/internal", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("internal")
	})
	r.GET("/api/v1/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("status")
	})

	t.Run("TC-116-03: Raw Dot-Dot Traversal Sequence", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/dashboard/../../canary_traversal.txt", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden, got %d", res.StatusCode)
		}
		if res.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection: close, got %q", res.Header.Get("Connection"))
		}
		if res.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %q", res.Header.Get("Content-Type"))
		}
		body := res.Body.String()
		if !strings.Contains(body, `{"error":"403 Forbidden: Path Traversal Disallowed"}`) {
			t.Errorf("unexpected body payload: %s", body)
		}
		if strings.Contains(body, canaryContent) {
			t.Errorf("CRITICAL SECURITY VIOLATION: canary content was leaked in response body!")
		}
	})

	t.Run("TC-116-04: Uppercase Percent-Encoded Traversal (%2E%2E)", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden, got %d", res.StatusCode)
		}
		if res.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection: close, got %q", res.Header.Get("Connection"))
		}
		if res.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %q", res.Header.Get("Content-Type"))
		}
		body := res.Body.String()
		if !strings.Contains(body, `{"error":"403 Forbidden: Path Traversal Disallowed"}`) {
			t.Errorf("unexpected body payload: %s", body)
		}
		if strings.Contains(body, canaryContent) {
			t.Errorf("CRITICAL SECURITY VIOLATION: canary content was leaked in response body!")
		}
	})

	t.Run("TC-116-05: Double Percent-Encoded Traversal (%252e%252e)", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden, got %d", res.StatusCode)
		}
		if res.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection: close, got %q", res.Header.Get("Connection"))
		}
		if res.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %q", res.Header.Get("Content-Type"))
		}
		body := res.Body.String()
		if !strings.Contains(body, `{"error":"403 Forbidden: Path Traversal Disallowed"}`) {
			t.Errorf("unexpected body payload: %s", body)
		}
		if strings.Contains(body, canaryContent) {
			t.Errorf("CRITICAL SECURITY VIOLATION: canary content was leaked in response body!")
		}
	})

	t.Run("TC-116-06: Static Prefix Route Legitimate Subpath Access Preserved", func(t *testing.T) {
		// Valid file request
		reqFile, _ := httpparser.NewRequest("GET", "/internal/dashboard/index.html", "HTTP/1.1")
		resFile := httpparser.NewResponse()
		r.ServeHTTP(reqFile, resFile)

		if resFile.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 OK for index.html, got %d", resFile.StatusCode)
		}
		if resFile.Body.String() != indexContent {
			t.Errorf("unexpected body for index.html: got %q, want %q", resFile.Body.String(), indexContent)
		}
		if resFile.Header.Get("Connection") == "close" {
			t.Errorf("expected Connection header NOT to be close for legitimate static file")
		}

		// Valid assets subpath request
		reqAsset, _ := httpparser.NewRequest("GET", "/internal/dashboard/assets/app.js", "HTTP/1.1")
		resAsset := httpparser.NewResponse()
		r.ServeHTTP(reqAsset, resAsset)

		if resAsset.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 OK for app.js, got %d", resAsset.StatusCode)
		}
		if resAsset.Body.String() != jsContent {
			t.Errorf("unexpected body for app.js: got %q, want %q", resAsset.Body.String(), jsContent)
		}

		// Valid relative path resolving inside static directory
		reqSub, _ := httpparser.NewRequest("GET", "/internal/dashboard/assets/../index.html", "HTTP/1.1")
		resSub := httpparser.NewResponse()
		r.ServeHTTP(reqSub, resSub)

		if resSub.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 OK for internal relative path, got %d", resSub.StatusCode)
		}
		if resSub.Body.String() != indexContent {
			t.Errorf("unexpected body for internal relative path: %s", resSub.Body.String())
		}
	})

	t.Run("TC-116-07: Non-Static Route ADR-062 Canonicalization Invariant Preservation", func(t *testing.T) {
		// /public/../internal -> resolves to /internal
		req1, _ := httpparser.NewRequest("GET", "/public/../internal", "HTTP/1.1")
		res1 := httpparser.NewResponse()
		r.ServeHTTP(req1, res1)

		if res1.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 OK for /public/../internal, got %d", res1.StatusCode)
		}
		if res1.Body.String() != "internal" {
			t.Errorf("expected body %q, got %q", "internal", res1.Body.String())
		}
		if res1.Header.Get("Connection") == "close" {
			t.Errorf("expected Connection NOT to be close for valid API canonicalization")
		}

		// //internal -> resolves to /internal
		req2, _ := httpparser.NewRequest("GET", "//internal", "HTTP/1.1")
		res2 := httpparser.NewResponse()
		r.ServeHTTP(req2, res2)

		if res2.StatusCode != http.StatusOK || res2.Body.String() != "internal" {
			t.Errorf("expected 200 OK /internal for //internal, got %d %q", res2.StatusCode, res2.Body.String())
		}

		// /api/v1/../v1/status -> resolves to /api/v1/status
		req3, _ := httpparser.NewRequest("GET", "/api/v1/../v1/status", "HTTP/1.1")
		res3 := httpparser.NewResponse()
		r.ServeHTTP(req3, res3)

		if res3.StatusCode != http.StatusOK || res3.Body.String() != "status" {
			t.Errorf("expected 200 OK status for /api/v1/../v1/status, got %d %q", res3.StatusCode, res3.Body.String())
		}
	})
}

// TestRouter_ADR062_Canonicalization verifies ADR-062 canonicalization invariants for non-static API routes (REQ-116 / FR-3 / TC-116-07).
func TestRouter_ADR062_Canonicalization(t *testing.T) {
	r := router.New()

	r.GET("/public", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("public")
	})
	r.GET("/internal", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("internal")
	})
	r.GET("/api/v1/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("status")
	})

	// Register a static prefix route to ensure non-interference
	tempDir := t.TempDir()
	r.Static("/internal/dashboard", tempDir)

	// 1. /public/../internal -> resolves cleanly to /internal
	req1, _ := httpparser.NewRequest("GET", "/public/../internal", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != http.StatusOK || res1.Body.String() != "internal" {
		t.Errorf("expected 200 OK 'internal' for /public/../internal, got %d %q", res1.StatusCode, res1.Body.String())
	}

	// 2. //internal -> resolves cleanly to /internal
	req2, _ := httpparser.NewRequest("GET", "//internal", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != http.StatusOK || res2.Body.String() != "internal" {
		t.Errorf("expected 200 OK 'internal' for //internal, got %d %q", res2.StatusCode, res2.Body.String())
	}

	// 3. /api/v1/../v1/status -> resolves cleanly to /api/v1/status
	req3, _ := httpparser.NewRequest("GET", "/api/v1/../v1/status", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.StatusCode != http.StatusOK || res3.Body.String() != "status" {
		t.Errorf("expected 200 OK 'status' for /api/v1/../v1/status, got %d %q", res3.StatusCode, res3.Body.String())
	}
}

// TC-134.8: Router Host:Port Disambiguation (domain-only wildcard vs explicit port matching)
func TestRouter_HostPortDisambiguation(t *testing.T) {
	r := router.New()

	// Route A (Domain-only wildcard)
	r.GETHost("api.toron.local", "/data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("domain-wildcard-metrics")
	})

	// Route B (Port-specific route)
	r.GETHost("api.toron.local:9000", "/data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("port-9000-admin-metrics")
	})

	// 1. Request with Host: api.toron.local:9000 -> routes to handlerPort9000
	req1, _ := httpparser.NewRequest("GET", "/data", "HTTP/1.1")
	req1.Header.Set("Host", "api.toron.local:9000")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != http.StatusOK || res1.Body.String() != "port-9000-admin-metrics" {
		t.Fatalf("req1: expected 200 OK 'port-9000-admin-metrics', got %d %q", res1.StatusCode, res1.Body.String())
	}

	// 2. Request with Host: api.toron.local:8080 -> routes to handlerDomain (wildcard port)
	req2, _ := httpparser.NewRequest("GET", "/data", "HTTP/1.1")
	req2.Header.Set("Host", "api.toron.local:8080")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != http.StatusOK || res2.Body.String() != "domain-wildcard-metrics" {
		t.Fatalf("req2: expected 200 OK 'domain-wildcard-metrics', got %d %q", res2.StatusCode, res2.Body.String())
	}

	// 3. Request with Host: api.toron.local -> routes to handlerDomain (no port)
	req3, _ := httpparser.NewRequest("GET", "/data", "HTTP/1.1")
	req3.Header.Set("Host", "api.toron.local")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.StatusCode != http.StatusOK || res3.Body.String() != "domain-wildcard-metrics" {
		t.Fatalf("req3: expected 200 OK 'domain-wildcard-metrics', got %d %q", res3.StatusCode, res3.Body.String())
	}

	// 4. Request with Host: api.toron.local:80 -> routes to handlerDomain
	req4, _ := httpparser.NewRequest("GET", "/data", "HTTP/1.1")
	req4.Header.Set("Host", "api.toron.local:80")
	res4 := httpparser.NewResponse()
	r.ServeHTTP(req4, res4)
	if res4.StatusCode != http.StatusOK || res4.Body.String() != "domain-wildcard-metrics" {
		t.Fatalf("req4: expected 200 OK 'domain-wildcard-metrics', got %d %q", res4.StatusCode, res4.Body.String())
	}

	// 5. Request with Host: other.toron.local:9000 -> returns 404 (host mismatch)
	req5, _ := httpparser.NewRequest("GET", "/data", "HTTP/1.1")
	req5.Header.Set("Host", "other.toron.local:9000")
	res5 := httpparser.NewResponse()
	r.ServeHTTP(req5, res5)
	if res5.StatusCode != http.StatusNotFound {
		t.Fatalf("req5: expected 404 Not Found for host mismatch, got %d %q", res5.StatusCode, res5.Body.String())
	}

	// 6. Request with Host: other.toron.local -> returns 404 (host mismatch)
	req6, _ := httpparser.NewRequest("GET", "/data", "HTTP/1.1")
	req6.Header.Set("Host", "other.toron.local")
	res6 := httpparser.NewResponse()
	r.ServeHTTP(req6, res6)
	if res6.StatusCode != http.StatusNotFound {
		t.Fatalf("req6: expected 404 Not Found for host mismatch, got %d %q", res6.StatusCode, res6.Body.String())
	}
}

// TC-134.9: Route Precedence (explicit host:port priority over domain-only fallback)
func TestRouter_RoutePrecedence_HostPortOverDomain(t *testing.T) {
	r := router.New()

	// Exact Routes on path /config
	r.GETHost("example.com", "/config", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("domain-fallback")
	})
	r.GETHost("example.com:8443", "/config", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("secure-port-8443")
	})
	r.GETHost("example.com:8080", "/config", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("admin-port-8080")
	})

	// Prefix Routes on prefix /v2
	r.HandlePrefixWithMatcher("GET", "svc.corp", "/v2", nil, nil, func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("domain-prefix-fallback")
	})
	r.HandlePrefixWithMatcher("GET", "svc.corp:9090", "/v2", nil, nil, func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("port-prefix-9090")
	})

	// 1. Exact Route Precedence Checks
	req1, _ := httpparser.NewRequest("GET", "/config", "HTTP/1.1")
	req1.Header.Set("Host", "example.com:8443")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.Body.String() != "secure-port-8443" {
		t.Fatalf("expected 'secure-port-8443', got %q", res1.Body.String())
	}

	req2, _ := httpparser.NewRequest("GET", "/config", "HTTP/1.1")
	req2.Header.Set("Host", "example.com:8080")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.Body.String() != "admin-port-8080" {
		t.Fatalf("expected 'admin-port-8080', got %q", res2.Body.String())
	}

	req3, _ := httpparser.NewRequest("GET", "/config", "HTTP/1.1")
	req3.Header.Set("Host", "example.com:9999")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.Body.String() != "domain-fallback" {
		t.Fatalf("expected 'domain-fallback', got %q", res3.Body.String())
	}

	req4, _ := httpparser.NewRequest("GET", "/config", "HTTP/1.1")
	req4.Header.Set("Host", "example.com")
	res4 := httpparser.NewResponse()
	r.ServeHTTP(req4, res4)
	if res4.Body.String() != "domain-fallback" {
		t.Fatalf("expected 'domain-fallback', got %q", res4.Body.String())
	}

	// 2. Prefix Route Specificity Sorting Checks
	reqP1, _ := httpparser.NewRequest("GET", "/v2/users", "HTTP/1.1")
	reqP1.Header.Set("Host", "svc.corp:9090")
	resP1 := httpparser.NewResponse()
	r.ServeHTTP(reqP1, resP1)
	if resP1.Body.String() != "port-prefix-9090" {
		t.Fatalf("expected 'port-prefix-9090', got %q", resP1.Body.String())
	}

	reqP2, _ := httpparser.NewRequest("GET", "/v2/users", "HTTP/1.1")
	reqP2.Header.Set("Host", "svc.corp:7070")
	resP2 := httpparser.NewResponse()
	r.ServeHTTP(reqP2, resP2)
	if resP2.Body.String() != "domain-prefix-fallback" {
		t.Fatalf("expected 'domain-prefix-fallback', got %q", resP2.Body.String())
	}

	reqP3, _ := httpparser.NewRequest("GET", "/v2/users", "HTTP/1.1")
	reqP3.Header.Set("Host", "svc.corp")
	resP3 := httpparser.NewResponse()
	r.ServeHTTP(reqP3, resP3)
	if resP3.Body.String() != "domain-prefix-fallback" {
		t.Fatalf("expected 'domain-prefix-fallback', got %q", resP3.Body.String())
	}
}

func TestRouter_HostPortPrecedence(t *testing.T) {
	TestRouter_RoutePrecedence_HostPortOverDomain(t)
}

// TC-134.10: IPv6 Router Matching and Host Port Stripping
func TestRouter_IPv6HostMatchingAndPortStripping(t *testing.T) {
	// 1. Unit checks on extractHost
	vectors := []struct {
		input    string
		expected string
	}{
		{"[::1]:8080", "[::1]"},
		{"[::1]", "[::1]"},
		{"[2001:0db8::1]:8443", "[2001:0db8::1]"},
		{"[2001:0db8::1]", "[2001:0db8::1]"},
		{"localhost:8080", "localhost"},
	}

	for _, v := range vectors {
		req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
		req.Header.Set("Host", v.input)
		actual := router.ExtractHost(req)
		if actual != v.expected {
			t.Fatalf("extractHost(%q): expected %q, got %q", v.input, v.expected, actual)
		}
	}

	// Unit checks on hasExplicitPort
	portVectors := []struct {
		input   string
		hasPort bool
	}{
		{"[::1]:8080", true},
		{"[::1]", false},
		{"example.com:8080", true},
		{"example.com", false},
	}
	for _, pv := range portVectors {
		if actual := router.HasExplicitPort(pv.input); actual != pv.hasPort {
			t.Fatalf("hasExplicitPort(%q): expected %v, got %v", pv.input, pv.hasPort, actual)
		}
	}

	// 2. Router Integration checks
	r := router.New()

	r.GETHost("[::1]:8080", "/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("ipv6-port-8080")
	})

	r.GETHost("[::1]", "/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("ipv6-domain-fallback")
	})

	// Request 1: [::1]:8080 -> port 8080 handler
	req1, _ := httpparser.NewRequest("GET", "/status", "HTTP/1.1")
	req1.Header.Set("Host", "[::1]:8080")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.StatusCode != http.StatusOK || res1.Body.String() != "ipv6-port-8080" {
		t.Fatalf("req1: expected 200 OK 'ipv6-port-8080', got %d %q", res1.StatusCode, res1.Body.String())
	}

	// Request 2: [::1]:8443 -> domain fallback
	req2, _ := httpparser.NewRequest("GET", "/status", "HTTP/1.1")
	req2.Header.Set("Host", "[::1]:8443")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.StatusCode != http.StatusOK || res2.Body.String() != "ipv6-domain-fallback" {
		t.Fatalf("req2: expected 200 OK 'ipv6-domain-fallback', got %d %q", res2.StatusCode, res2.Body.String())
	}

	// Request 3: [::1] -> domain fallback
	req3, _ := httpparser.NewRequest("GET", "/status", "HTTP/1.1")
	req3.Header.Set("Host", "[::1]")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.StatusCode != http.StatusOK || res3.Body.String() != "ipv6-domain-fallback" {
		t.Fatalf("req3: expected 200 OK 'ipv6-domain-fallback', got %d %q", res3.StatusCode, res3.Body.String())
	}

	// Request 4: [2001:db8::2]:8080 -> 404 Not Found
	req4, _ := httpparser.NewRequest("GET", "/status", "HTTP/1.1")
	req4.Header.Set("Host", "[2001:db8::2]:8080")
	res4 := httpparser.NewResponse()
	r.ServeHTTP(req4, res4)
	if res4.StatusCode != http.StatusNotFound {
		t.Fatalf("req4: expected 404 Not Found, got %d", res4.StatusCode)
	}
}

func TestRouter_IPv6HostMatching(t *testing.T) {
	TestRouter_IPv6HostMatchingAndPortStripping(t)
}

// TC-134.11: Verification across all 9 routing methods
func TestRouter_AllNineRoutingMethods_HostPortInvariants(t *testing.T) {
	r := router.New()

	// 1. Exact Path Routing (r.Handle, r.GET)
	r.GETHost("exact.local:8080", "/exact", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("exact-port-8080")
	})
	r.GETHost("exact.local", "/exact", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("exact-domain-fallback")
	})

	// 2. Prefix Path Routing (r.prefixRoutes)
	r.HandlePrefixWithMatcher("GET", "prefix.local:8080", "/v1", nil, nil, func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("prefix-port-8080")
	})
	r.HandlePrefixWithMatcher("GET", "prefix.local", "/v1", nil, nil, func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("prefix-domain-fallback")
	})

	// 3. Domain / Virtual Host Routing
	r.GETHost("tenant-a.local:8080", "/profile", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("tenant-a")
	})
	r.GETHost("tenant-b.local:8080", "/profile", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("tenant-b")
	})

	// 4. Header-Based Routing
	r.HandleHostHeader("GET", "canary.local:8080", "/canary", map[string]string{"X-Canary": "true"}, func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("canary-matched")
	})

	// 5. Method-Based Routing
	r.GETHost("methods.local:8080", "/resource", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("get-matched")
	})
	r.POSTHost("methods.local:8080", "/resource", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("post-matched")
	})

	// 6. Reverse Proxy Upstream Routing
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream-response"))
	}))
	defer upstreamSrv.Close()

	optsProxy := proxy.ProxyOptions{
		Targets: []string{upstreamSrv.URL},
	}
	if err := r.RoutePrefix(router.RouteTypeUpstream, "proxy.local:8080", "/upstream", nil, "", optsProxy); err != nil {
		t.Fatalf("failed to register upstream route: %v", err)
	}

	// 7. Static File Serving Routing
	tmpDir := t.TempDir()
	testFilePath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(testFilePath, []byte("static-content-8080"), 0644); err != nil {
		t.Fatalf("failed to write static file: %v", err)
	}
	if err := r.RoutePrefix(router.RouteTypeStatic, "static.local:8080", "/static", nil, tmpDir, proxy.ProxyOptions{}); err != nil {
		t.Fatalf("failed to register static route: %v", err)
	}

	// 8. K8s Ingress & Container Discovery Routing
	ingressSpecs := []router.PrefixRouteSpec{
		{
			TargetType: router.RouteTypeUpstream,
			Host:       "ingress.local:8080",
			Prefix:     "/ingress",
			Opts:       proxy.ProxyOptions{Targets: []string{upstreamSrv.URL}},
		},
	}
	if err := r.ReplacePrefixRoutesBySource("k8s-ingress", ingressSpecs); err != nil {
		t.Fatalf("failed to replace prefix routes: %v", err)
	}

	// Verification of Method 1: Exact Path Routing
	reqM1a, _ := httpparser.NewRequest("GET", "/exact", "HTTP/1.1")
	reqM1a.Header.Set("Host", "exact.local:8080")
	resM1a := httpparser.NewResponse()
	r.ServeHTTP(reqM1a, resM1a)
	if resM1a.Body.String() != "exact-port-8080" {
		t.Errorf("Method 1: expected exact-port-8080, got %q", resM1a.Body.String())
	}

	reqM1b, _ := httpparser.NewRequest("GET", "/exact", "HTTP/1.1")
	reqM1b.Header.Set("Host", "exact.local:9090")
	resM1b := httpparser.NewResponse()
	r.ServeHTTP(reqM1b, resM1b)
	if resM1b.Body.String() != "exact-domain-fallback" {
		t.Errorf("Method 1: expected exact-domain-fallback, got %q", resM1b.Body.String())
	}

	// Verification of Method 2: Prefix Path Routing
	reqM2a, _ := httpparser.NewRequest("GET", "/v1/test", "HTTP/1.1")
	reqM2a.Header.Set("Host", "prefix.local:8080")
	resM2a := httpparser.NewResponse()
	r.ServeHTTP(reqM2a, resM2a)
	if resM2a.Body.String() != "prefix-port-8080" {
		t.Errorf("Method 2: expected prefix-port-8080, got %q", resM2a.Body.String())
	}

	reqM2b, _ := httpparser.NewRequest("GET", "/v1/test", "HTTP/1.1")
	reqM2b.Header.Set("Host", "prefix.local:9090")
	resM2b := httpparser.NewResponse()
	r.ServeHTTP(reqM2b, resM2b)
	if resM2b.Body.String() != "prefix-domain-fallback" {
		t.Errorf("Method 2: expected prefix-domain-fallback, got %q", resM2b.Body.String())
	}

	// Verification of Method 3: Domain / Virtual Host Routing
	reqM3a, _ := httpparser.NewRequest("GET", "/profile", "HTTP/1.1")
	reqM3a.Header.Set("Host", "tenant-a.local:8080")
	resM3a := httpparser.NewResponse()
	r.ServeHTTP(reqM3a, resM3a)
	if resM3a.Body.String() != "tenant-a" {
		t.Errorf("Method 3: expected tenant-a, got %q", resM3a.Body.String())
	}

	reqM3b, _ := httpparser.NewRequest("GET", "/profile", "HTTP/1.1")
	reqM3b.Header.Set("Host", "tenant-b.local:8080")
	resM3b := httpparser.NewResponse()
	r.ServeHTTP(reqM3b, resM3b)
	if resM3b.Body.String() != "tenant-b" {
		t.Errorf("Method 3: expected tenant-b, got %q", resM3b.Body.String())
	}

	// Verification of Method 4: Header-Based Routing
	reqM4a, _ := httpparser.NewRequest("GET", "/canary", "HTTP/1.1")
	reqM4a.Header.Set("Host", "canary.local:8080")
	reqM4a.Header.Set("X-Canary", "true")
	resM4a := httpparser.NewResponse()
	r.ServeHTTP(reqM4a, resM4a)
	if resM4a.Body.String() != "canary-matched" {
		t.Errorf("Method 4: expected canary-matched, got %q", resM4a.Body.String())
	}

	reqM4b, _ := httpparser.NewRequest("GET", "/canary", "HTTP/1.1")
	reqM4b.Header.Set("Host", "canary.local:8080")
	resM4b := httpparser.NewResponse()
	r.ServeHTTP(reqM4b, resM4b)
	if resM4b.StatusCode != http.StatusNotFound {
		t.Errorf("Method 4: expected 404 when canary header missing, got %d", resM4b.StatusCode)
	}

	// Verification of Method 5: Method-Based Routing
	reqM5a, _ := httpparser.NewRequest("GET", "/resource", "HTTP/1.1")
	reqM5a.Header.Set("Host", "methods.local:8080")
	resM5a := httpparser.NewResponse()
	r.ServeHTTP(reqM5a, resM5a)
	if resM5a.Body.String() != "get-matched" {
		t.Errorf("Method 5: expected get-matched, got %q", resM5a.Body.String())
	}

	reqM5b, _ := httpparser.NewRequest("POST", "/resource", "HTTP/1.1")
	reqM5b.Header.Set("Host", "methods.local:8080")
	resM5b := httpparser.NewResponse()
	r.ServeHTTP(reqM5b, resM5b)
	if resM5b.Body.String() != "post-matched" {
		t.Errorf("Method 5: expected post-matched, got %q", resM5b.Body.String())
	}

	reqM5c, _ := httpparser.NewRequest("DELETE", "/resource", "HTTP/1.1")
	reqM5c.Header.Set("Host", "methods.local:8080")
	resM5c := httpparser.NewResponse()
	r.ServeHTTP(reqM5c, resM5c)
	if resM5c.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Method 5: expected 405 Method Not Allowed, got %d", resM5c.StatusCode)
	}

	// Verification of Method 6: Reverse Proxy Upstream Routing
	reqM6, _ := httpparser.NewRequest("GET", "/upstream/data", "HTTP/1.1")
	reqM6.Header.Set("Host", "proxy.local:8080")
	resM6 := httpparser.NewResponse()
	r.ServeHTTP(reqM6, resM6)
	bodyM6 := resM6.Body.String()
	if resM6.StreamBody != nil {
		defer resM6.StreamBody.Close()
		b, _ := io.ReadAll(resM6.StreamBody)
		bodyM6 = string(b)
	}
	if resM6.StatusCode != http.StatusOK || bodyM6 != "upstream-response" {
		t.Errorf("Method 6: expected 200 OK 'upstream-response', got %d %q", resM6.StatusCode, bodyM6)
	}
	// Verify cache key authority reflects downstream client Host:Port, not upstream IP
	clientAuthority := router.ExtractCacheHostPort(reqM6)
	if clientAuthority != "proxy.local:8080" {
		t.Errorf("Method 6: expected cache authority 'proxy.local:8080', got %q", clientAuthority)
	}

	reqM6b, _ := httpparser.NewRequest("GET", "/upstream/data", "HTTP/1.1")
	reqM6b.Header.Set("Host", "proxy.local:9090")
	resM6b := httpparser.NewResponse()
	r.ServeHTTP(reqM6b, resM6b)
	if resM6b.StatusCode != http.StatusNotFound {
		t.Errorf("Method 6: expected 404 for wrong port on upstream route, got %d", resM6b.StatusCode)
	}

	// Verification of Method 7: Static File Serving Routing
	reqM7a, _ := httpparser.NewRequest("GET", "/static/index.html", "HTTP/1.1")
	reqM7a.Header.Set("Host", "static.local:8080")
	resM7a := httpparser.NewResponse()
	r.ServeHTTP(reqM7a, resM7a)
	if resM7a.StatusCode != http.StatusOK || resM7a.Body.String() != "static-content-8080" {
		t.Errorf("Method 7: expected 200 OK 'static-content-8080', got %d %q", resM7a.StatusCode, resM7a.Body.String())
	}

	reqM7b, _ := httpparser.NewRequest("GET", "/static/index.html", "HTTP/1.1")
	reqM7b.Header.Set("Host", "static.local:9090")
	resM7b := httpparser.NewResponse()
	r.ServeHTTP(reqM7b, resM7b)
	if resM7b.StatusCode != http.StatusNotFound {
		t.Errorf("Method 7: expected 404 for wrong port on port-isolated static route, got %d", resM7b.StatusCode)
	}

	// Verification of Method 8: K8s Ingress & Container Discovery Routing
	reqM8, _ := httpparser.NewRequest("GET", "/ingress/resource", "HTTP/1.1")
	reqM8.Header.Set("Host", "ingress.local:8080")
	resM8 := httpparser.NewResponse()
	r.ServeHTTP(reqM8, resM8)
	bodyM8 := resM8.Body.String()
	if resM8.StreamBody != nil {
		defer resM8.StreamBody.Close()
		b, _ := io.ReadAll(resM8.StreamBody)
		bodyM8 = string(b)
	}
	if resM8.StatusCode != http.StatusOK || bodyM8 != "upstream-response" {
		t.Errorf("Method 8: expected 200 OK 'upstream-response', got %d %q", resM8.StatusCode, bodyM8)
	}

	reqM8b, _ := httpparser.NewRequest("GET", "/ingress/resource", "HTTP/1.1")
	reqM8b.Header.Set("Host", "ingress.local:9090")
	resM8b := httpparser.NewResponse()
	r.ServeHTTP(reqM8b, resM8b)
	if resM8b.StatusCode != http.StatusNotFound {
		t.Errorf("Method 8: expected 404 for wrong port on ingress route, got %d", resM8b.StatusCode)
	}

	// Verification of Method 9: Multi-Port Gateway Listeners
	ports := []string{"80", "8080", "8443"}
	for _, port := range ports {
		reqM9, _ := httpparser.NewRequest("GET", "/gateway-probe", "HTTP/1.1")
		reqM9.Header.Set("Host", "gateway.local:"+port)
		auth := router.ExtractCacheHostPort(reqM9)
		if auth != "gateway.local:"+port {
			t.Errorf("Method 9: expected cache authority 'gateway.local:%s', got %q", port, auth)
		}
	}
}

func TestRouter_NineRoutingMethodsInvariants(t *testing.T) {
	TestRouter_AllNineRoutingMethods_HostPortInvariants(t)
}

func TestRouter_ContextRouteTagging(t *testing.T) {
	r := router.New()

	// 1. Register an upstream prefix route
	_ = r.RoutePrefix(router.RouteTypeUpstream, "", "/kite/api", nil, "", proxy.ProxyOptions{
		Targets: []string{"http://127.0.0.1:8080"},
	})

	// 2. Register a static prefix route
	tmpDir := t.TempDir()
	_ = r.RoutePrefix(router.RouteTypeStatic, "", "/assets", nil, tmpDir, proxy.ProxyOptions{})

	// 3. Register an exact route
	r.GET("/exact-endpoint", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("exact")
	})

	t.Run("Subpath matches parent prefix route context", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/kite/api/v1/trades/order-99", "HTTP/1.1")
		res := httpparser.NewResponse()

		r.ServeHTTP(req, res)

		matched := router.GetMatchedRoute(req.Context())
		if matched != "/kite/api" {
			t.Errorf("expected matched route '/kite/api', got %q", matched)
		}
		routeType := router.GetRouteType(req.Context())
		if routeType != "upstream" {
			t.Errorf("expected route type 'upstream', got %q", routeType)
		}
	})

	t.Run("Static route tags destination", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/assets/app.js", "HTTP/1.1")
		res := httpparser.NewResponse()

		r.ServeHTTP(req, res)

		matched := router.GetMatchedRoute(req.Context())
		if matched != "/assets" {
			t.Errorf("expected matched route '/assets', got %q", matched)
		}
		dest := router.GetDestination(req.Context())
		if dest != "static" {
			t.Errorf("expected destination 'static', got %q", dest)
		}
	})

	t.Run("Exact route tags in-process destination", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/exact-endpoint", "HTTP/1.1")
		res := httpparser.NewResponse()

		r.ServeHTTP(req, res)

		matched := router.GetMatchedRoute(req.Context())
		if matched != "/exact-endpoint" {
			t.Errorf("expected matched route '/exact-endpoint', got %q", matched)
		}
		dest := router.GetDestination(req.Context())
		if dest != "in-process" {
			t.Errorf("expected destination 'in-process', got %q", dest)
		}
	})
}

// TASK-173 / AC-173-1: Pre-Body Route Lookup
func TestRouter_LookupPrefixRoute(t *testing.T) {
	r := router.New()
	tVal := true
	_ = r.RoutePrefixWithSource("test", router.RouteTypeUpstream, "", "/api/v1", nil, "", proxy.ProxyOptions{
		Targets:               []string{"http://127.0.0.1:8080"},
		MaxConcurrency:        16,
		MaxBodyBytes:          100 * 1024 * 1024,
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		StreamRequestBody:     &tVal,
	})

	req, _ := httpparser.NewRequest("POST", "/api/v1/upload", "HTTP/1.1")
	matchInfo, found := r.LookupPrefixRoute(req)
	if !found || matchInfo == nil {
		t.Fatalf("expected route match, got found=%v", found)
	}

	if matchInfo.Prefix != "/api/v1" {
		t.Errorf("expected prefix '/api/v1', got %q", matchInfo.Prefix)
	}
	if matchInfo.MaxConcurrency != 16 {
		t.Errorf("expected MaxConcurrency 16, got %d", matchInfo.MaxConcurrency)
	}
	if matchInfo.MaxBodyBytes != 100*1024*1024 {
		t.Errorf("expected MaxBodyBytes 100MB, got %d", matchInfo.MaxBodyBytes)
	}
	if matchInfo.ReadTimeout != 10*time.Second {
		t.Errorf("expected ReadTimeout 10s, got %v", matchInfo.ReadTimeout)
	}
	if matchInfo.WriteTimeout != 15*time.Second {
		t.Errorf("expected WriteTimeout 15s, got %v", matchInfo.WriteTimeout)
	}
	if matchInfo.ResponseHeaderTimeout != 20*time.Second {
		t.Errorf("expected ResponseHeaderTimeout 20s, got %v", matchInfo.ResponseHeaderTimeout)
	}
	if matchInfo.StreamRequestBody == nil || !*matchInfo.StreamRequestBody {
		t.Errorf("expected StreamRequestBody true, got %v", matchInfo.StreamRequestBody)
	}

	// Non-matching request
	reqNonMatch, _ := httpparser.NewRequest("GET", "/other/path", "HTTP/1.1")
	_, found2 := r.LookupPrefixRoute(reqNonMatch)
	if found2 {
		t.Errorf("expected no match for /other/path, got true")
	}
}

// TASK-173 / AC-173-2, AC-173-3 / TC-145-06: Bulkhead Concurrency Gate & Fast-Fail 503 Rejection
func TestRouter_BulkheadConcurrency_FastFail503(t *testing.T) {
	r := router.New()

	holdGate := make(chan struct{})
	enteredCount := int64(0)
	enteredWg := sync.WaitGroup{}
	enteredWg.Add(4)

	err := r.AddRoute(router.PrefixRouteSpec{
		Prefix:         "/bulkhead/test",
		Method:         "GET",
		MaxConcurrency: 4,
		Handler: func(req *httpparser.Request, res *httpparser.Response) {
			atomic.AddInt64(&enteredCount, 1)
			enteredWg.Done()
			<-holdGate
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString(`{"status":"done"}`)
		},
	})
	if err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Ensure maxConcurrency is 4
	matchInfo, found := r.LookupPrefixRoute(&httpparser.Request{Path: "/bulkhead/test", Method: "GET"})
	if !found || matchInfo == nil {
		t.Fatal("route not found")
	}
	if matchInfo.MaxConcurrency != 4 {
		t.Fatalf("expected MaxConcurrency 4, got %d", matchInfo.MaxConcurrency)
	}

	// Dispatch 4 concurrent requests holding all slots
	for i := 0; i < 4; i++ {
		go func() {
			req, _ := httpparser.NewRequest("GET", "/bulkhead/test", "HTTP/1.1")
			res := httpparser.NewResponse()
			r.ServeHTTP(req, res)
		}()
	}

	enteredWg.Wait() // Ensure all 4 requests are actively holding slots

	// 5th request should be immediately rejected with 503
	req5, _ := httpparser.NewRequest("GET", "/bulkhead/test", "HTTP/1.1")
	res5 := httpparser.NewResponse()
	start := time.Now()
	r.ServeHTTP(req5, res5)
	elapsed := time.Since(start)

	if res5.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", res5.StatusCode)
	}
	if res5.Header.Get("Retry-After") != "5" {
		t.Errorf("expected Retry-After: 5, got %q", res5.Header.Get("Retry-After"))
	}
	if !strings.Contains(res5.Body.String(), "503 Service Unavailable: Route concurrency limit reached") {
		t.Errorf("expected rejection JSON body, got %q", res5.Body.String())
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected fast-fail in < 100ms, took %v", elapsed)
	}

	// Release the 4 holding requests
	close(holdGate)
}

// TASK-173 / AC-173-4: Deterministic Release Verification & Panic Recovery
func TestRouter_BulkheadConcurrency_DeterministicReleaseAndPanic(t *testing.T) {
	r := router.New()
	r.Use(router.RecoveryMiddleware())

	_ = r.RoutePrefixWithSource("test", router.RouteTypeUpstream, "", "/panic/test", nil, "", proxy.ProxyOptions{
		MaxConcurrency: 1,
	})
	r.HandlePrefix("GET", "/panic/test", func(req *httpparser.Request, res *httpparser.Response) {
		panic("simulated route handler panic")
	})

	req, _ := httpparser.NewRequest("GET", "/panic/test", "HTTP/1.1")
	res := httpparser.NewResponse()

	// Should recover gracefully with 500
	r.ServeHTTP(req, res)
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 on panic recovery, got %d", res.StatusCode)
	}

	// Concurrency slot must be decremented back to 0 so next request can acquire it!
	info, found := r.LookupPrefixRoute(req)
	if !found || info == nil {
		t.Fatal("route lookup failed")
	}

	release, acquired := r.TryAcquireRouteSlot(info)
	if !acquired {
		t.Fatal("expected slot to be acquired after panic recovery, but route was still locked")
	}
	release()
}

// TASK-173 / AC-173-5: Automatic Bulkhead Safety Guardrail Derivation in Router
func TestRouter_DefaultSafetyBulkheadDerivation(t *testing.T) {
	r := router.New()
	r.SetWorkerPoolSize(128)

	// Elevated payload ceiling (256MB) without max_concurrency
	_ = r.RoutePrefixWithSource("test", router.RouteTypeUpstream, "", "/elevated/body", nil, "", proxy.ProxyOptions{
		Targets:      []string{"http://127.0.0.1:8080"},
		MaxBodyBytes: 256 * 1024 * 1024,
	})

	req, _ := httpparser.NewRequest("POST", "/elevated/body", "HTTP/1.1")
	info, found := r.LookupPrefixRoute(req)
	if !found || info == nil {
		t.Fatal("route not found")
	}
	if info.MaxConcurrency != 32 {
		t.Errorf("expected auto guardrail 32 for elevated body, got %d", info.MaxConcurrency)
	}

	// Elevated response timeout (30s) without max_concurrency
	_ = r.RoutePrefixWithSource("test", router.RouteTypeUpstream, "", "/elevated/timeout", nil, "", proxy.ProxyOptions{
		Targets:               []string{"http://127.0.0.1:8080"},
		ResponseHeaderTimeout: 30 * time.Second,
	})
	req2, _ := httpparser.NewRequest("GET", "/elevated/timeout", "HTTP/1.1")
	info2, found2 := r.LookupPrefixRoute(req2)
	if !found2 || info2 == nil {
		t.Fatal("route 2 not found")
	}
	if info2.MaxConcurrency != 32 {
		t.Errorf("expected auto guardrail 32 for elevated timeout, got %d", info2.MaxConcurrency)
	}

	// Standard route without elevated parameters
	_ = r.RoutePrefixWithSource("test", router.RouteTypeUpstream, "", "/standard", nil, "", proxy.ProxyOptions{
		Targets:               []string{"http://127.0.0.1:8080"},
		MaxBodyBytes:          2 * 1024 * 1024,
		ResponseHeaderTimeout: 5 * time.Second,
	})
	req3, _ := httpparser.NewRequest("GET", "/standard", "HTTP/1.1")
	info3, found3 := r.LookupPrefixRoute(req3)
	if !found3 || info3 == nil {
		t.Fatal("route 3 not found")
	}
	if info3.MaxConcurrency != 0 {
		t.Errorf("expected unconstrained (0) for standard route, got %d", info3.MaxConcurrency)
	}
}

