package router_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if res1.Body.String() != "backend-1" {
		t.Errorf("request 1: expected backend-1, got %q", res1.Body.String())
	}

	// Request 2 -> server2
	req2, _ := httpparser.NewRequest("GET", "/api/users", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.Body.String() != "backend-2" {
		t.Errorf("expected backend-2 response, got %q", res2.Body.String())
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
