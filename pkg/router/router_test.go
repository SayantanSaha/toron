package router_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"toron/pkg/httpparser"
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
		t.Errorf("request 2: expected backend-2, got %q", res2.Body.String())
	}
}
