package proxy_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/proxy"
)

func TestReverseProxy_ForwardRequest(t *testing.T) {
	// Start mock upstream HTTP server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Errorf("expected upstream path /api/users, got %q", r.URL.Path)
		}
		if r.Header.Get("X-Forwarded-Host") != "toron.local" {
			t.Errorf("expected X-Forwarded-Host toron.local, got %q", r.Header.Get("X-Forwarded-Host"))
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"echo":%q}`, string(bodyBytes))
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewReverseProxy(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create reverse proxy: %v", err)
	}

	req, err := httpparser.NewRequest("POST", "/api/users", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	req.Header.Set("Host", "toron.local")
	req.Body = strings.NewReader("hello upstream")

	res := httpparser.NewResponse()

	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json Content-Type, got %q", res.Header.Get("Content-Type"))
	}
	expectedBody := `{"echo":"hello upstream"}`
	bodyStr := res.Body.String()
	if res.StreamBody != nil {
		defer res.StreamBody.Close()
		b, _ := io.ReadAll(res.StreamBody)
		bodyStr = string(b)
	}
	if bodyStr != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, bodyStr)
	}
}

func TestReverseProxy_BadGateway(t *testing.T) {
	// Target an offline port
	px, err := proxy.NewReverseProxy("http://127.0.0.1:59999", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	res := httpparser.NewResponse()

	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway, got %d", res.StatusCode)
	}
}

func TestReverseProxy_RoundRobinLoadBalancing(t *testing.T) {
	var count1, count2 int

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-1"))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-2"))
	}))
	defer server2.Close()

	targets := []string{server1.URL, server2.URL}
	px, err := proxy.NewLoadBalancerProxy(targets, proxy.AlgorithmRoundRobin, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create load balancer proxy: %v", err)
	}

	if px.Balancer.Algorithm() != proxy.AlgorithmRoundRobin {
		t.Errorf("expected round_robin algorithm, got %s", px.Balancer.Algorithm())
	}

	// Make 4 requests, expecting round-robin distribution: s1, s2, s1, s2
	expectedResponses := []string{"server-1", "server-2", "server-1", "server-2"}
	for i, expected := range expectedResponses {
		req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200 OK, got %d", i, res.StatusCode)
		}
		bodyStr := res.Body.String()
		if res.StreamBody != nil {
			b, _ := io.ReadAll(res.StreamBody)
			_ = res.StreamBody.Close()
			bodyStr = string(b)
		}
		if bodyStr != expected {
			t.Errorf("request %d: expected body %q, got %q", i, expected, bodyStr)
		}
	}

	if count1 != 2 || count2 != 2 {
		t.Errorf("expected 2 requests each, got server1=%d, server2=%d", count1, count2)
	}
}

func TestLoadBalancer_Validation(t *testing.T) {
	// Empty targets
	_, err := proxy.NewLoadBalancerProxy(nil, proxy.AlgorithmRoundRobin, time.Second)
	if err == nil {
		t.Error("expected error for empty target URLs")
	}

	// Unsupported algorithm
	_, err = proxy.NewLoadBalancerProxy([]string{"http://localhost:8080"}, proxy.Algorithm("unknown_algo"), time.Second)
	if err == nil {
		t.Error("expected error for unsupported algorithm")
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy"))
	}))
	defer healthyServer.Close()

	offlinePort := "http://127.0.0.1:59998"

	opts := proxy.ProxyOptions{
		Targets:             []string{healthyServer.URL, offlinePort},
		Algorithm:           proxy.AlgorithmRoundRobin,
		Timeout:             500 * time.Millisecond,
		MaxFailures:         2,
		CooldownPeriod:      200 * time.Millisecond,
		HealthCheckPath:     "/health",
		HealthCheckInterval: 50 * time.Millisecond,
	}

	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy with options: %v", err)
	}
	defer px.Close()

	// Wait for background active health checker to trip offline server to Open
	time.Sleep(150 * time.Millisecond)

	// Make 4 requests - all should automatically route to healthyServer because offline target is Open/tripped
	for i := 0; i < 4; i++ {
		req, _ := httpparser.NewRequest("GET", "/", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200 OK from healthy node, got %d", i, res.StatusCode)
		}
		bodyStr := res.Body.String()
		if res.StreamBody != nil {
			b, _ := io.ReadAll(res.StreamBody)
			_ = res.StreamBody.Close()
			bodyStr = string(b)
		}
		if bodyStr != "healthy" {
			t.Errorf("request %d: expected body 'healthy', got %q", i, bodyStr)
		}
	}
}

func TestWebSocketProxyTunnel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil || !strings.Contains(strings.ToLower(string(buf[:n])), "upgrade: websocket") {
			return
		}

		upgradeResp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
		_, _ = conn.Write([]byte(upgradeResp))

		n, err = conn.Read(buf)
		if err == nil && n > 0 {
			_, _ = conn.Write(buf[:n])
		}
	}()

	px, err := proxy.NewReverseProxy("http://"+ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/ws", "HTTP/1.1")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "testkey")

	res := httpparser.NewResponse()
	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", res.StatusCode)
	}
	if res.UpgradedConn == nil {
		t.Fatalf("expected non-nil UpgradedConn")
	}

	_, _ = res.UpgradedConn.Write([]byte("ping"))
	buf := make([]byte, 10)
	n, err := res.UpgradedConn.Read(buf)
	if err != nil || string(buf[:n]) != "ping" {
		t.Errorf("expected echo 'ping', got %q (err %v)", string(buf[:n]), err)
	}
	res.UpgradedConn.Close()
}

func TestLoadBalancer_StickyCookie(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-1"))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-2"))
	}))
	defer server2.Close()

	opts := proxy.ProxyOptions{
		Targets:          []string{server1.URL, server2.URL},
		Algorithm:        proxy.AlgorithmStickyCookie,
		StickyCookieName: "TORON_STICKY",
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Request 1: Initial request (no cookie) -> receives Set-Cookie header
	req1, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	px.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res1.StatusCode)
	}

	setCookieHeader := res1.Header.Get("Set-Cookie")
	if !strings.Contains(setCookieHeader, "TORON_STICKY=") {
		t.Fatalf("expected Set-Cookie header containing TORON_STICKY=, got %q", setCookieHeader)
	}

	// Extract cookie value
	kv := strings.SplitN(setCookieHeader, ";", 2)[0]

	// Request 2 & 3: Send sticky cookie -> expect routing to identical server instance
	firstBody := res1.Body.String()
	for i := 0; i < 3; i++ {
		reqSticky, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
		reqSticky.Header.Set("Cookie", kv)
		resSticky := httpparser.NewResponse()
		px.ServeHTTP(reqSticky, resSticky)

		if resSticky.Body.String() != firstBody {
			t.Errorf("request %d: expected sticky response %q, got %q", i+1, firstBody, resSticky.Body.String())
		}
	}
}

func TestLoadBalancer_IPHash(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-1"))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server-2"))
	}))
	defer server2.Close()

	opts := proxy.ProxyOptions{
		Targets:   []string{server1.URL, server2.URL},
		Algorithm: proxy.AlgorithmIPHash,
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create ip_hash proxy: %v", err)
	}

	req1, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
	req1.Header.Set("X-Forwarded-For", "192.168.1.50")
	res1 := httpparser.NewResponse()
	px.ServeHTTP(req1, res1)

	initialBody := res1.Body.String()
	if res1.StreamBody != nil {
		b, _ := io.ReadAll(res1.StreamBody)
		_ = res1.StreamBody.Close()
		initialBody = string(b)
	}

	// Repeated requests from same IP should route to same target
	for i := 0; i < 3; i++ {
		req, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
		req.Header.Set("X-Forwarded-For", "192.168.1.50")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		bodyStr := res.Body.String()
		if res.StreamBody != nil {
			b, _ := io.ReadAll(res.StreamBody)
			_ = res.StreamBody.Close()
			bodyStr = string(b)
		}

		if bodyStr != initialBody {
			t.Errorf("expected ip_hash to pin to %q, got %q", initialBody, bodyStr)
		}
	}
}

func TestProxy_TraceparentPropagation(t *testing.T) {
	var capturedTraceparent string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewReverseProxy(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	res := httpparser.NewResponse()

	px.ServeHTTP(req, res)

	if !strings.HasPrefix(capturedTraceparent, "00-4bf92f3577b34da6a3ce929d0e0e4736-") {
		t.Errorf("expected traceparent preserving trace ID 4bf92f3577b34da6a3ce929d0e0e4736, got %q", capturedTraceparent)
	}
}

func TestReverseProxy_XForwardedPrefixAndRedirectRewrite(t *testing.T) {
	var capturedPrefix string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPrefix = r.Header.Get("X-Forwarded-Prefix")
		// Simulate backend issuing a relative 302 redirect
		w.Header().Set("Location", "/login?ref=dashboard")
		w.Header().Set("Set-Cookie", "session=abc1234; Path=/; HttpOnly")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewReverseProxy(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/api/dashboard", "HTTP/1.1")
	req.Header.Set("Host", "example.com")
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/api")

	if capturedPrefix != "/api" {
		t.Errorf("expected X-Forwarded-Prefix /api, got %q", capturedPrefix)
	}
	if res.StatusCode != http.StatusFound {
		t.Errorf("expected status 302, got %d", res.StatusCode)
	}

	// Verify Location was rewritten to include /api
	expectedLoc := "/api/login?ref=dashboard"
	if loc := res.Header.Get("Location"); loc != expectedLoc {
		t.Errorf("expected rewritten Location %q, got %q", expectedLoc, loc)
	}

	// Verify Set-Cookie Path was rewritten to /api
	expectedCookie := "session=abc1234; Path=/api; HttpOnly"
	if cookie := res.Header.Get("Set-Cookie"); cookie != expectedCookie {
		t.Errorf("expected rewritten Set-Cookie %q, got %q", expectedCookie, cookie)
	}
}

func TestReverseProxy_AbsoluteRedirectRewrite(t *testing.T) {
	var upstreamURL string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate backend issuing an absolute internal redirect matching its own URL
		w.Header().Set("Location", upstreamURL+"/auth/callback")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer upstreamServer.Close()
	upstreamURL = upstreamServer.URL

	px, err := proxy.NewReverseProxy(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/services/legacy/start", "HTTP/1.1")
	req.Header.Set("Host", "gateway.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/services/legacy")

	expectedLoc := "https://gateway.example.com/services/legacy/auth/callback"
	if loc := res.Header.Get("Location"); loc != expectedLoc {
		t.Errorf("expected rewritten absolute Location %q, got %q", expectedLoc, loc)
	}
}

func TestReverseProxy_ExternalRedirectPreserved(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate OAuth external redirect
		w.Header().Set("Location", "https://accounts.google.com/o/oauth2/v2/auth?client_id=123")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewReverseProxy(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/api/oauth", "HTTP/1.1")
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/api")

	expectedLoc := "https://accounts.google.com/o/oauth2/v2/auth?client_id=123"
	if loc := res.Header.Get("Location"); loc != expectedLoc {
		t.Errorf("expected untouched external Location %q, got %q", expectedLoc, loc)
	}
}

func TestReverseProxy_StripPrefixDisabled(t *testing.T) {
	var capturedPath string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	stripPrefixFalse := false
	opts := proxy.ProxyOptions{
		Targets:     []string{upstreamServer.URL},
		StripPrefix: &stripPrefixFalse,
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/api/v1/users", "HTTP/1.1")
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/api")

	// Since StripPrefix is false, upstream must receive the full path /api/v1/users
	if capturedPath != "/api/v1/users" {
		t.Errorf("expected upstream path /api/v1/users with strip_prefix=false, got %q", capturedPath)
	}
}

func TestReverseProxy_RewriteRedirectsDisabled(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/raw-backend-path")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstreamServer.Close()

	rewriteRedirectsFalse := false
	opts := proxy.ProxyOptions{
		Targets:          []string{upstreamServer.URL},
		RewriteRedirects: &rewriteRedirectsFalse,
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/api/test", "HTTP/1.1")
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/api")

	// Since RewriteRedirects is false, Location header must remain verbatim "/raw-backend-path"
	if loc := res.Header.Get("Location"); loc != "/raw-backend-path" {
		t.Errorf("expected raw Location /raw-backend-path with rewrite_redirects=false, got %q", loc)
	}
}

func TestJoinProxyPath_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		targetPath  string
		reqPath     string
		prefix      string
		stripPrefix bool
		expected    string
	}{
		// TC-059-01: Exact prefix match to subpath target without trailing slash
		{
			name:        "exact prefix match to subpath target",
			targetPath:  "/postback",
			reqPath:     "/kite/postback",
			prefix:      "/kite/postback",
			stripPrefix: true,
			expected:    "/postback",
		},
		// TC-059-02: Exact prefix match with client trailing slash
		{
			name:        "prefix with client trailing slash",
			targetPath:  "/postback",
			reqPath:     "/kite/postback/",
			prefix:      "/kite/postback",
			stripPrefix: true,
			expected:    "/postback/",
		},
		// TC-059-03: Nested subpath under subpath target
		{
			name:        "nested subpath under subpath target",
			targetPath:  "/postback",
			reqPath:     "/kite/postback/status",
			prefix:      "/kite/postback",
			stripPrefix: true,
			expected:    "/postback/status",
		},
		// TC-059-04: Non-subpath target regression
		{
			name:        "non-subpath target exact prefix",
			targetPath:  "",
			reqPath:     "/kite/api",
			prefix:      "/kite/api",
			stripPrefix: true,
			expected:    "/",
		},
		{
			name:        "non-subpath target prefix with trailing slash",
			targetPath:  "",
			reqPath:     "/kite/api/",
			prefix:      "/kite/api",
			stripPrefix: true,
			expected:    "/",
		},
		{
			name:        "non-subpath target nested subpath",
			targetPath:  "",
			reqPath:     "/kite/api/v1/trades",
			prefix:      "/kite/api",
			stripPrefix: true,
			expected:    "/v1/trades",
		},
		// TC-059-05: Target explicit trailing slash preservation
		{
			name:        "target explicit trailing slash preserved",
			targetPath:  "/postback/",
			reqPath:     "/kite/postback",
			prefix:      "/kite/postback",
			stripPrefix: true,
			expected:    "/postback/",
		},
		// TC-059-06: StripPrefix false
		{
			name:        "strip prefix false keeps full path",
			targetPath:  "",
			reqPath:     "/kite/postback",
			prefix:      "/kite/postback",
			stripPrefix: false,
			expected:    "/kite/postback",
		},
		{
			name:        "strip prefix false with subpath target joins cleanly",
			targetPath:  "/api",
			reqPath:     "/v1/users",
			prefix:      "/v1",
			stripPrefix: false,
			expected:    "/api/v1/users",
		},
		// Traversal containment tests (ADR-062 / TASK-067)
		{
			name:        "traversal guard prevents escaping subpath target with stripPrefix",
			targetPath:  "/subpath",
			reqPath:     "/api/../admin",
			prefix:      "/api",
			stripPrefix: true,
			expected:    "/subpath/admin",
		},
		{
			name:        "traversal guard prevents escaping subpath target without stripPrefix",
			targetPath:  "/v1",
			reqPath:     "/v1/../../etc/passwd",
			prefix:      "/v1",
			stripPrefix: false,
			expected:    "/v1/etc/passwd",
		},
		{
			name:        "traversal above root on non-subpath target normalizes safely",
			targetPath:  "",
			reqPath:     "/api/../../../root",
			prefix:      "/api",
			stripPrefix: true,
			expected:    "/root",
		},
		{
			name:        "traversal with trailing slash preserved",
			targetPath:  "/api",
			reqPath:     "/v1/../users/",
			prefix:      "/v1",
			stripPrefix: true,
			expected:    "/api/users/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := proxy.JoinProxyPath(tc.targetPath, tc.reqPath, tc.prefix, tc.stripPrefix)
			if got != tc.expected {
				t.Errorf("JoinProxyPath(%q, %q, %q, %v) = %q; want %q",
					tc.targetPath, tc.reqPath, tc.prefix, tc.stripPrefix, got, tc.expected)
			}
		})
	}
}

func TestReverseProxy_SubpathTarget_Integration(t *testing.T) {
	var capturedPath string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	// Upstream target with subpath /postback
	targetURL := upstreamServer.URL + "/postback"
	stripPrefixTrue := true
	opts := proxy.ProxyOptions{
		Targets:     []string{targetURL},
		StripPrefix: &stripPrefixTrue,
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// 1. Exact match /kite/postback -> upstream must receive /postback (NO trailing slash)
	req1, _ := httpparser.NewRequest("POST", "/kite/postback", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	px.ServeHTTPWithPrefix(req1, res1, "/kite/postback")
	if capturedPath != "/postback" {
		t.Errorf("expected upstream path /postback, got %q", capturedPath)
	}

	// 2. Trailing slash /kite/postback/ -> upstream must receive /postback/
	req2, _ := httpparser.NewRequest("POST", "/kite/postback/", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	px.ServeHTTPWithPrefix(req2, res2, "/kite/postback")
	if capturedPath != "/postback/" {
		t.Errorf("expected upstream path /postback/, got %q", capturedPath)
	}

	// 3. Nested path /kite/postback/webhook -> upstream must receive /postback/webhook
	req3, _ := httpparser.NewRequest("POST", "/kite/postback/webhook", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	px.ServeHTTPWithPrefix(req3, res3, "/kite/postback")
	if capturedPath != "/postback/webhook" {
		t.Errorf("expected upstream path /postback/webhook, got %q", capturedPath)
	}
}

func TestReverseProxy_QueryParametersForwarding(t *testing.T) {
	var capturedQuery string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	opts := proxy.ProxyOptions{
		Targets: []string{upstreamServer.URL},
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// 1. Standard HTTP/1.1 request parsed via httpparser
	req1, _ := httpparser.NewRequest("GET", "/api/v1/trades?symbol=INFY&status=COMPLETE", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	px.ServeHTTPWithPrefix(req1, res1, "/api")
	if capturedQuery != "symbol=INFY&status=COMPLETE" {
		t.Errorf("expected upstream query 'symbol=INFY&status=COMPLETE', got %q", capturedQuery)
	}

	// 2. HTTP/2 converted request via NewRequestFromStd
	httpReq, _ := http.NewRequest("GET", "https://example.com/api/v1/trades?page=2&limit=50&sort=desc", nil)
	req2 := httpparser.NewRequestFromStd(httpReq)
	res2 := httpparser.NewResponse()
	px.ServeHTTPWithPrefix(req2, res2, "/api")
	if capturedQuery != "page=2&limit=50&sort=desc" {
		t.Errorf("expected upstream query 'page=2&limit=50&sort=desc', got %q", capturedQuery)
	}

	// 3. Flags and unencoded characters preserved verbatim
	httpReq3, _ := http.NewRequest("GET", "https://example.com/api/search?q=hello+world&verbose", nil)
	req3 := httpparser.NewRequestFromStd(httpReq3)
	res3 := httpparser.NewResponse()
	px.ServeHTTPWithPrefix(req3, res3, "/api")
	if capturedQuery != "q=hello+world&verbose" {
		t.Errorf("expected verbatim query 'q=hello+world&verbose', got %q", capturedQuery)
	}
}

func TestReverseProxy_QueryParametersMergeWithTarget(t *testing.T) {
	var capturedQuery string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	// Upstream target with configured query parameters
	targetWithQuery := upstreamServer.URL + "/data?apiKey=secret123"
	opts := proxy.ProxyOptions{
		Targets: []string{targetWithQuery},
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Client sends additional query parameters
	httpReq, _ := http.NewRequest("GET", "https://example.com/data?filter=active&limit=10", nil)
	req := httpparser.NewRequestFromStd(httpReq)
	res := httpparser.NewResponse()
	px.ServeHTTPWithPrefix(req, res, "/data")

	expectedQuery := "apiKey=secret123&filter=active&limit=10"
	if capturedQuery != expectedQuery {
		t.Errorf("expected merged query %q, got %q", expectedQuery, capturedQuery)
	}
}

func TestWebSocketProxy_TLSVerification(t *testing.T) {
	// Setup TLS server that upgrades to WebSocket
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijack unsupported", 500)
				return
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			defer conn.Close()

			upgradeResp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
			_, _ = bufrw.WriteString(upgradeResp)
			_ = bufrw.Flush()

			buf := make([]byte, 1024)
			n, _ := bufrw.Read(buf)
			if n > 0 {
				_, _ = bufrw.Write(buf[:n])
				_ = bufrw.Flush()
			}
		}
	}))
	defer tlsServer.Close()

	t.Run("Self-signed certificate is rejected with 502 Bad Gateway by default", func(t *testing.T) {
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{tlsServer.URL},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		req, _ := httpparser.NewRequest("GET", "/ws", "HTTP/1.1")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Key", "testkey")

		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusBadGateway {
			t.Fatalf("SECURITY VIOLATION: Expected 502 Bad Gateway due to untrusted self-signed cert, got %d", res.StatusCode)
		}
		if res.UpgradedConn != nil {
			t.Fatalf("expected UpgradedConn to be nil on TLS verification failure")
		}
	})

	t.Run("Custom CA certificate pool succeeds handshake", func(t *testing.T) {
		caPool := x509.NewCertPool()
		caPool.AddCert(tlsServer.Certificate())

		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets:       []string{tlsServer.URL},
			TLSCACertPool: caPool,
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		req, _ := httpparser.NewRequest("GET", "/ws", "HTTP/1.1")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Key", "testkey")

		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("expected 101 Switching Protocols with trusted CA, got %d", res.StatusCode)
		}
		if res.UpgradedConn == nil {
			t.Fatalf("expected non-nil UpgradedConn")
		}
		res.UpgradedConn.Close()
	})

	t.Run("InsecureSkipVerify true succeeds handshake", func(t *testing.T) {
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets:            []string{tlsServer.URL},
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		req, _ := httpparser.NewRequest("GET", "/ws", "HTTP/1.1")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Key", "testkey")

		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("expected 101 Switching Protocols with InsecureSkipVerify: true, got %d", res.StatusCode)
		}
		if res.UpgradedConn == nil {
			t.Fatalf("expected non-nil UpgradedConn")
		}
		res.UpgradedConn.Close()
	})
}

type mockProxyAddr struct {
	addr string
}

func (a *mockProxyAddr) Network() string { return "tcp" }
func (a *mockProxyAddr) String() string  { return a.addr }

type mockProxyConn struct {
	net.Conn
	remoteAddr string
}

func (m *mockProxyConn) RemoteAddr() net.Addr {
	return &mockProxyAddr{addr: m.remoteAddr}
}

func TestReverseProxy_HopByHopStripping(t *testing.T) {
	var capturedHeaders http.Header
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{upstreamServer.URL},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/api/test", "HTTP/1.1")
	req.Header.Set("Connection", "close, X-Custom-Hop1, X-Custom-Hop2")
	req.Header.Set("Keep-Alive", "timeout=10, max=100")
	req.Header.Set("Upgrade", "rogue-protocol")
	req.Header.Set("TE", "trailers")
	req.Header.Set("Proxy-Authenticate", "Basic")
	req.Header.Set("Proxy-Authorization", "Basic 12345")
	req.Header.Set("X-Custom-Hop1", "secret-hop-value")
	req.Header.Set("X-Custom-Hop2", "secret-hop-value-2")
	req.Header.Set("X-Legitimate-Header", "allowed-value")

	res := httpparser.NewResponse()
	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	// Standard hop-by-hop headers must be removed
	for _, hopHeader := range []string{"Connection", "Keep-Alive", "Upgrade", "TE", "Proxy-Authenticate", "Proxy-Authorization"} {
		if val := capturedHeaders.Get(hopHeader); val != "" {
			t.Errorf("expected hop-by-hop header %q to be stripped, got %q", hopHeader, val)
		}
	}

	// Custom headers declared in Connection token list must be removed
	for _, customHop := range []string{"X-Custom-Hop1", "X-Custom-Hop2"} {
		if val := capturedHeaders.Get(customHop); val != "" {
			t.Errorf("expected custom hop-by-hop header %q to be stripped, got %q", customHop, val)
		}
	}

	// Legitimate headers must be preserved
	if val := capturedHeaders.Get("X-Legitimate-Header"); val != "allowed-value" {
		t.Errorf("expected X-Legitimate-Header 'allowed-value', got %q", val)
	}
}

func TestReverseProxy_ConnectionIntegrityAndTrustedProxies(t *testing.T) {
	var capturedHeaders http.Header
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:        []string{upstreamServer.URL},
		TrustedProxies: []string{"10.0.0.1/32"},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Subtest 1: Untrusted physical client trying to spoof X-Forwarded-Proto and X-Forwarded-For
	t.Run("Untrusted Socket IP Anti-Spoofing", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/api/secure", "HTTP/1.1")
		req.RawConn = &mockProxyConn{remoteAddr: "198.51.100.50:49152"}
		req.Header.Set("X-Forwarded-Proto", "https") // Spoofed!
		req.Header.Set("X-Forwarded-For", "1.1.1.1") // Spoofed!

		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}

		// Proto must be derived from physical socket (cleartext TCP -> http)
		if proto := capturedHeaders.Get("X-Forwarded-Proto"); proto != "http" {
			t.Errorf("expected X-Forwarded-Proto 'http' for cleartext socket, got %q", proto)
		}

		// Client IP must be overwritten by physical socket IP (198.51.100.50)
		if xff := capturedHeaders.Get("X-Forwarded-For"); xff != "198.51.100.50" {
			t.Errorf("expected X-Forwarded-For '198.51.100.50', got %q", xff)
		}
	})

	// Subtest 2: Authorized trusted proxy chaining
	t.Run("Trusted Proxy Forwarding", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/api/secure", "HTTP/1.1")
		req.RawConn = &mockProxyConn{remoteAddr: "10.0.0.1:49152"} // In TrustedProxies!
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-For", "203.0.113.195")

		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}

		// Proto should be honored from trusted proxy
		if proto := capturedHeaders.Get("X-Forwarded-Proto"); proto != "https" {
			t.Errorf("expected X-Forwarded-Proto 'https' from trusted proxy, got %q", proto)
		}

		// Client IP should append proxy IP to existing chain
		expectedXFF := "203.0.113.195, 10.0.0.1"
		if xff := capturedHeaders.Get("X-Forwarded-For"); xff != expectedXFF {
			t.Errorf("expected X-Forwarded-For %q, got %q", expectedXFF, xff)
		}
	})
}

func TestReverseProxy_HTTPParameterPollutionMitigation(t *testing.T) {
	var capturedQuery string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	// Upstream target configured with gateway policy constraints: role=guest&env=production
	targetWithQuery := upstreamServer.URL + "/api?role=guest&env=production"
	opts := proxy.ProxyOptions{
		Targets: []string{targetWithQuery},
	}
	px, err := proxy.NewProxyWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	t.Run("Colliding parameter stripped and non-colliding preserved", func(t *testing.T) {
		// Client attempts to escalate privileges by overriding role=admin
		httpReq, _ := http.NewRequest("GET", "https://example.com/api?role=admin&user=alice&action=view", nil)
		req := httpparser.NewRequestFromStd(httpReq)
		res := httpparser.NewResponse()
		px.ServeHTTPWithPrefix(req, res, "/api")

		// Expect role=guest (from target) & env=production (from target) & user=alice & action=view (from client)
		// role=admin MUST NOT appear in upstream query!
		expected := "role=guest&env=production&user=alice&action=view"
		if capturedQuery != expected {
			t.Errorf("expected query %q, got %q", expected, capturedQuery)
		}
	})

	t.Run("Duplicate colliding parameter injection stripped", func(t *testing.T) {
		// Client attempts multiple parameter pollution
		httpReq, _ := http.NewRequest("GET", "https://example.com/api?role=admin&role=root&env=dev", nil)
		req := httpparser.NewRequestFromStd(httpReq)
		res := httpparser.NewResponse()
		px.ServeHTTPWithPrefix(req, res, "/api")

		// All colliding keys stripped; only target query remains
		expected := "role=guest&env=production"
		if capturedQuery != expected {
			t.Errorf("expected query %q, got %q", expected, capturedQuery)
		}
	})

	t.Run("Semicolon delimited parameter pollution stripped", func(t *testing.T) {
		// Client attempts delimiter injection via semicolon
		httpReq, _ := http.NewRequest("GET", "https://example.com/api?user=alice&other=1;role=admin", nil)
		req := httpparser.NewRequestFromStd(httpReq)
		res := httpparser.NewResponse()
		px.ServeHTTPWithPrefix(req, res, "/api")

		expected := "role=guest&env=production&user=alice"
		if capturedQuery != expected {
			t.Errorf("expected query %q, got %q", expected, capturedQuery)
		}
	})

	t.Run("Empty target parameters preserves client query verbatim", func(t *testing.T) {
		optsNoQuery := proxy.ProxyOptions{
			Targets: []string{upstreamServer.URL + "/plain"},
		}
		pxPlain, err := proxy.NewProxyWithOptions(optsNoQuery)
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		httpReq, _ := http.NewRequest("GET", "https://example.com/plain?q=hello+world&debug", nil)
		req := httpparser.NewRequestFromStd(httpReq)
		res := httpparser.NewResponse()
		pxPlain.ServeHTTPWithPrefix(req, res, "/plain")

		expected := "q=hello+world&debug"
		if capturedQuery != expected {
			t.Errorf("expected query %q, got %q", expected, capturedQuery)
		}
	})
}

func TestServer_H2_ReverseProxy_HeaderSanitization(t *testing.T) {
	var capturedHeaders http.Header
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{upstreamServer.URL},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Subtest 6A: Untrusted HTTP/2 Client Header Sanitization
	t.Run("Untrusted HTTP/2 client forged headers are stripped", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/2.0")
		req.RemoteAddr = "198.51.100.70:52000"
		req.Header.Set("X-Forwarded-For", "1.1.1.1, 8.8.8.8")
		req.Header.Set("X-Real-IP", "1.1.1.1")
		req.Header.Set("X-Forwarded-Proto", "http")

		res := httpparser.NewResponse()
		px.ServeHTTPWithPrefix(req, res, "/api")

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}

		if xff := capturedHeaders.Get("X-Forwarded-For"); xff != "198.51.100.70" {
			t.Errorf("SECURITY VIOLATION: upstream received unsanitized XFF %q, want '198.51.100.70'", xff)
		}
		if xri := capturedHeaders.Get("X-Real-IP"); xri != "198.51.100.70" {
			t.Errorf("SECURITY VIOLATION: upstream received unsanitized X-Real-IP %q, want '198.51.100.70'", xri)
		}
		if proto := capturedHeaders.Get("X-Forwarded-Proto"); proto != "https" {
			t.Errorf("expected X-Forwarded-Proto 'https' for HTTP/2, got %q", proto)
		}
	})

	// Subtest 6C: Untrusted IPv6 Client Sanitization
	t.Run("Untrusted IPv6 client brackets stripped and forged headers discarded", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/2.0")
		req.RemoteAddr = "[2001:db8::beef]:60000"
		req.Header.Set("X-Forwarded-For", "10.0.0.1")

		res := httpparser.NewResponse()
		px.ServeHTTPWithPrefix(req, res, "/api")

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}

		if xff := capturedHeaders.Get("X-Forwarded-For"); xff != "2001:db8::beef" {
			t.Errorf("expected X-Forwarded-For '2001:db8::beef', got %q", xff)
		}
		if xri := capturedHeaders.Get("X-Real-IP"); xri != "2001:db8::beef" {
			t.Errorf("expected X-Real-IP '2001:db8::beef', got %q", xri)
		}
	})
}

func TestServer_H2_ReverseProxy_TrustedProxyAppended(t *testing.T) {
	var capturedHeaders http.Header
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:        []string{upstreamServer.URL},
		TrustedProxies: []string{"10.0.0.0/8"},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Subtest 6B: Trusted Proxy Ingress Header Appending
	t.Run("Trusted proxy appends physical IP and preserves client X-Real-IP", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/2.0")
		req.RemoteAddr = "10.0.1.5:43000"
		req.Header.Set("X-Forwarded-For", "203.0.113.99")
		req.Header.Set("X-Real-IP", "203.0.113.99")

		res := httpparser.NewResponse()
		px.ServeHTTPWithPrefix(req, res, "/api")

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}

		expectedXFF := "203.0.113.99, 10.0.1.5"
		if xff := capturedHeaders.Get("X-Forwarded-For"); xff != expectedXFF {
			t.Errorf("expected X-Forwarded-For %q, got %q", expectedXFF, xff)
		}
		if xri := capturedHeaders.Get("X-Real-IP"); xri != "203.0.113.99" {
			t.Errorf("expected X-Real-IP '203.0.113.99', got %q", xri)
		}
	})
}

func TestReverseProxy_Close_ClosesIdleConnections(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{upstreamServer.URL},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/", "HTTP/1.1")
	res := httpparser.NewResponse()
	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}

	// Verify px.Close executes cleanly and closes idle transport connections
	px.Close()

	// Calling px.Close() again should be idempotent and not panic
	px.Close()

	// Verify nil-safety
	emptyPx := &proxy.ReverseProxy{}
	emptyPx.Close()
}

// TC-112-07: Upstream HTTP/1.1 Pseudo-Header Stripping Verification
func TestProxy_HTTP2ToHTTP1_PseudoHeaderStripping(t *testing.T) {
	var receivedHeaders http.Header
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstreamServer.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{upstreamServer.URL},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	// Simulate translated HTTP/2 request with pseudo-headers in req.Header
	req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/2.0")
	req.Header.Set(":protocol", "websocket")
	req.Header.Set(":path", "/api/data")
	req.Header.Set(":authority", "example.com")
	req.Header.Set(":method", "GET")
	req.Header.Set(":scheme", "https")
	req.Header.Set("X-Normal-Header", "present")

	res := httpparser.NewResponse()
	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}

	// Verify that NO headers received by upstreamServer start with ":"
	for k := range receivedHeaders {
		if strings.HasPrefix(k, ":") {
			t.Errorf("pseudo-header leaked to upstream: %s", k)
		}
	}
	if receivedHeaders.Get("X-Normal-Header") != "present" {
		t.Errorf("expected X-Normal-Header to be forwarded, got %q", receivedHeaders.Get("X-Normal-Header"))
	}
}

// TC-123: Verification of Configurable Upstream Reverse Proxy Transport Architecture
func TestProxy_TransportConfig_DefaultsAndCustom(t *testing.T) {
	t.Run("TC-123.1: Raw Speed Defaults Preservation", func(t *testing.T) {
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{"http://127.0.0.1:9099"},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		tr := px.GetTransport()
		if tr == nil {
			t.Fatal("expected non-nil http.Transport")
		}
		if tr.MaxIdleConns != 10000 {
			t.Errorf("expected MaxIdleConns 10000, got %d", tr.MaxIdleConns)
		}
		if tr.MaxIdleConnsPerHost != 1000 {
			t.Errorf("expected MaxIdleConnsPerHost 1000, got %d", tr.MaxIdleConnsPerHost)
		}
		if tr.MaxConnsPerHost != 0 {
			t.Errorf("expected MaxConnsPerHost 0, got %d", tr.MaxConnsPerHost)
		}
		if tr.IdleConnTimeout != 90*time.Second {
			t.Errorf("expected IdleConnTimeout 90s, got %v", tr.IdleConnTimeout)
		}
		if !tr.DisableCompression {
			t.Errorf("expected DisableCompression true, got false")
		}
		if tr.ForceAttemptHTTP2 {
			t.Errorf("expected ForceAttemptHTTP2 false, got true")
		}
		if tr.Proxy != nil {
			t.Errorf("expected Proxy nil for raw speed, got non-nil")
		}
	})

	t.Run("TC-123.2/7: Custom Transport Tuning", func(t *testing.T) {
		f := false
		trTrue := true
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{"http://127.0.0.1:9099"},
			Transport: proxy.ProxyTransportConfig{
				MaxIdleConns:        500,
				MaxIdleConnsPerHost: 50,
				MaxConnsPerHost:     25,
				IdleConnTimeout:     15 * time.Second,
				DisableCompression:  &f,
				ForceAttemptHTTP2:   &trTrue,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		tr := px.GetTransport()
		if tr == nil {
			t.Fatal("expected non-nil http.Transport")
		}
		if tr.MaxIdleConns != 500 {
			t.Errorf("expected MaxIdleConns 500, got %d", tr.MaxIdleConns)
		}
		if tr.MaxIdleConnsPerHost != 50 {
			t.Errorf("expected MaxIdleConnsPerHost 50, got %d", tr.MaxIdleConnsPerHost)
		}
		if tr.MaxConnsPerHost != 25 {
			t.Errorf("expected MaxConnsPerHost 25, got %d", tr.MaxConnsPerHost)
		}
		if tr.IdleConnTimeout != 15*time.Second {
			t.Errorf("expected IdleConnTimeout 15s, got %v", tr.IdleConnTimeout)
		}
		if tr.DisableCompression {
			t.Errorf("expected DisableCompression false, got true")
		}
		if !tr.ForceAttemptHTTP2 {
			t.Errorf("expected ForceAttemptHTTP2 true, got false")
		}
	})

	t.Run("TC-123.3: Explicit Egress Proxy URL", func(t *testing.T) {
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{"http://127.0.0.1:9099"},
			Transport: proxy.ProxyTransportConfig{
				ProxyURL: "http://squid.corp:3128",
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		tr := px.GetTransport()
		if tr == nil || tr.Proxy == nil {
			t.Fatal("expected non-nil Proxy resolver on transport")
		}
		testReq, _ := http.NewRequest("GET", "http://example.com/test", nil)
		proxyURL, err := tr.Proxy(testReq)
		if err != nil {
			t.Fatalf("tr.Proxy failed: %v", err)
		}
		if proxyURL == nil || proxyURL.String() != "http://squid.corp:3128" {
			t.Errorf("expected proxy URL 'http://squid.corp:3128', got %v", proxyURL)
		}
	})
}

func TestProxy_TransportConfig_PropagateUpstreamClose(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("origin-terminating"))
	}))
	defer upstreamServer.Close()

	t.Run("TC-123.6: Default Raw Speed Isolates KeepAlive", func(t *testing.T) {
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstreamServer.URL},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
		// In raw speed mode, upstream Connection: close is stripped, so downstream res does not contain close
		if res.Header.Get("Connection") == "close" {
			t.Errorf("expected Connection header NOT to be 'close' in raw speed mode, got %q", res.Header.Get("Connection"))
		}
	})

	t.Run("TC-123.6: PropagateUpstreamClose Enables Socket Teardown", func(t *testing.T) {
		tClose := true
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstreamServer.URL},
			Transport: proxy.ProxyTransportConfig{
				PropagateUpstreamClose: &tClose,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
		if res.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection header to be 'close' when PropagateUpstreamClose is true, got %q", res.Header.Get("Connection"))
		}
	})
}

func TestProxy_TransportConfig_DisableCompressionToggle(t *testing.T) {
	testPayload := "hello-world-decompressed-stream-data"
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If client asked for gzip, return gzipped body
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusOK)
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, _ = gz.Write([]byte(testPayload))
			_ = gz.Close()
			_, _ = w.Write(buf.Bytes())
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(testPayload))
	}))
	defer upstreamServer.Close()

	t.Run("TC-123.5: DisableCompression false transparently decompresses", func(t *testing.T) {
		f := false
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstreamServer.URL},
			Transport: proxy.ProxyTransportConfig{
				DisableCompression: &f,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		// Request without client Accept-Encoding
		req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
		bodyStr := res.Body.String()
		if res.StreamBody != nil {
			b, _ := io.ReadAll(res.StreamBody)
			_ = res.StreamBody.Close()
			bodyStr = string(b)
		}
		if bodyStr != testPayload {
			t.Errorf("expected decompressed body %q, got %q", testPayload, bodyStr)
		}
	})
}

func TestProxy_TransportConfig_ConcurrencyBackpressure(t *testing.T) {
	var currentActive int64
	var maxObserved int64
	var mu sync.Mutex

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&currentActive, 1)
		mu.Lock()
		if cur > maxObserved {
			maxObserved = cur
		}
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt64(&currentActive, -1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstreamServer.Close()

	t.Run("TC-123.4: MaxConnsPerHost throttles active connections", func(t *testing.T) {
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstreamServer.URL},
			Transport: proxy.ProxyTransportConfig{
				MaxConnsPerHost: 2,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		var wg sync.WaitGroup
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
				res := httpparser.NewResponse()
				px.ServeHTTP(req, res)
				if res.StreamBody != nil {
					_ = res.StreamBody.Close()
				}
			}()
		}
		wg.Wait()

		mu.Lock()
		observed := maxObserved
		mu.Unlock()

		if observed > 2 {
			t.Errorf("expected at most 2 concurrent connections to upstream, got %d", observed)
		}
	})
}

func TestProxy_TransportConfig_Tracing(t *testing.T) {
	var (
		mu              sync.Mutex
		lastTraceparent string
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastTraceparent = r.Header.Get("traceparent")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	t.Run("TC-124.3: Tracing false with missing incoming traceparent omits header", func(t *testing.T) {
		trFalse := false
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstream.URL},
			Transport: proxy.ProxyTransportConfig{
				Tracing: &trFalse,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		mu.Lock()
		observed := lastTraceparent
		mu.Unlock()

		if observed != "" {
			t.Errorf("expected no traceparent header to upstream when tracing=false and incoming is empty, got %q", observed)
		}
	})

	t.Run("TC-124.4: Tracing false with incoming traceparent propagates verbatim", func(t *testing.T) {
		trFalse := false
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstream.URL},
			Transport: proxy.ProxyTransportConfig{
				Tracing: &trFalse,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		incoming := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
		req.Header.Set("traceparent", incoming)
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		mu.Lock()
		observed := lastTraceparent
		mu.Unlock()

		if observed != incoming {
			t.Errorf("expected verbatim propagation %q, got %q", incoming, observed)
		}
	})

	t.Run("TC-124.5: Tracing true with missing incoming traceparent generates W3C header", func(t *testing.T) {
		trTrue := true
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstream.URL},
			Transport: proxy.ProxyTransportConfig{
				Tracing: &trTrue,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		mu.Lock()
		observed := lastTraceparent
		mu.Unlock()

		if observed == "" {
			t.Fatal("expected generated traceparent header to upstream when tracing=true, got empty")
		}
		parts := strings.Split(observed, "-")
		if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
			t.Errorf("malformed generated traceparent header: %q", observed)
		}
	})

	t.Run("TC-124.6: Tracing true with incoming traceparent preserves traceID and updates spanID", func(t *testing.T) {
		trTrue := true
		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{upstream.URL},
			Transport: proxy.ProxyTransportConfig{
				Tracing: &trTrue,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		incoming := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
		req.Header.Set("traceparent", incoming)
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		mu.Lock()
		observed := lastTraceparent
		mu.Unlock()

		parts := strings.Split(observed, "-")
		if len(parts) != 4 || parts[0] != "00" || parts[1] != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("expected preserved trace ID 4bf92f3577b34da6a3ce929d0e0e4736, got %q", observed)
		}
		if parts[2] == "00f067aa0ba902b7" {
			t.Errorf("expected updated child span ID, got unchanged span ID %q", parts[2])
		}
	})
}

func TestProxy_Transport_ResponseHeaderTimeoutEnforcement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("delayed headers"))
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{upstream.URL},
		Transport: proxy.ProxyTransportConfig{
			ResponseHeaderTimeout: 50 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, _ := httpparser.NewRequest("GET", "/slow-headers", "HTTP/1.1")
	res := httpparser.NewResponse()

	start := time.Now()
	px.ServeHTTP(req, res)
	elapsed := time.Since(start)

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway on ResponseHeaderTimeout, got %d", res.StatusCode)
	}
	if elapsed > 140*time.Millisecond {
		t.Fatalf("expected failure within ~50ms ResponseHeaderTimeout, took %v", elapsed)
	}
	bodyStr := res.Body.String()
	if !strings.Contains(bodyStr, "timeout awaiting response headers") && !strings.Contains(bodyStr, "header timeout") && !strings.Contains(bodyStr, "Client.Timeout") && !strings.Contains(bodyStr, "Upstream unreachable") {
		t.Fatalf("expected bad gateway message mentioning timeout, got: %s", bodyStr)
	}
}

func TestProxy_Transport_BodyStreamingDecoupled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			_, _ = fmt.Fprintf(w, "data: chunk-%d\n\n", i)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer upstream.Close()

	trTrue := true
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{upstream.URL},
		Transport: proxy.ProxyTransportConfig{
			StreamResponse:        &trTrue,
			ResponseHeaderTimeout: 2 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, _ := httpparser.NewRequest("GET", "/stream", "HTTP/1.1")
	res := httpparser.NewResponse()
	px.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody == nil {
		t.Fatal("expected res.StreamBody != nil")
	}
	defer res.StreamBody.Close()

	bodyBytes, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("failed to read stream body: %v", err)
	}
	bodyStr := string(bodyBytes)
	for i := 0; i < 3; i++ {
		expectedChunk := fmt.Sprintf("data: chunk-%d\n\n", i)
		if !strings.Contains(bodyStr, expectedChunk) {
			t.Fatalf("expected stream body to contain %q, got: %s", expectedChunk, bodyStr)
		}
	}
}

func TestProxy_StreamingEligibilityMatrix(t *testing.T) {
	cases := []struct {
		name                string
		streamResponse      bool
		routeHasCompression bool
		routeHasCache       bool
		contentType         string
		accelBuffering      string
		expectedCanStream   bool
	}{
		{
			name:                "Case 1: streamResponse false (balanced)",
			streamResponse:      false,
			routeHasCompression: false,
			routeHasCache:       false,
			contentType:         "text/event-stream",
			accelBuffering:      "no",
			expectedCanStream:   false,
		},
		{
			name:                "Case 2: streamResponse true, no compression, no cache, json",
			streamResponse:      true,
			routeHasCompression: false,
			routeHasCache:       false,
			contentType:         "application/json",
			expectedCanStream:   true,
		},
		{
			name:                "Case 3: streamResponse true, compression true, json",
			streamResponse:      true,
			routeHasCompression: true,
			routeHasCache:       false,
			contentType:         "application/json",
			expectedCanStream:   false,
		},
		{
			name:                "Case 4: streamResponse true, cache true, json",
			streamResponse:      true,
			routeHasCompression: false,
			routeHasCache:       true,
			contentType:         "application/json",
			expectedCanStream:   false,
		},
		{
			name:                "Case 5: streamResponse true, compression & cache, text/event-stream",
			streamResponse:      true,
			routeHasCompression: true,
			routeHasCache:       true,
			contentType:         "text/event-stream",
			expectedCanStream:   true,
		},
		{
			name:                "Case 6: streamResponse true, uppercase TEXT/EVENT-STREAM; charset=utf-8",
			streamResponse:      true,
			routeHasCompression: true,
			routeHasCache:       true,
			contentType:         "TEXT/EVENT-STREAM; charset=utf-8",
			expectedCanStream:   true,
		},
		{
			name:                "Case 7: streamResponse true, X-Accel-Buffering no",
			streamResponse:      true,
			routeHasCompression: true,
			routeHasCache:       true,
			contentType:         "application/octet-stream",
			accelBuffering:      "no",
			expectedCanStream:   true,
		},
		{
			name:                "Case 7b: streamResponse true, X-Accel-Buffering whitespace no",
			streamResponse:      true,
			routeHasCompression: true,
			routeHasCache:       true,
			contentType:         "application/octet-stream",
			accelBuffering:      "  no  ",
			expectedCanStream:   true,
		},
		{
			name:                "Case 8: streamResponse true, compression & cache, X-Accel-Buffering yes",
			streamResponse:      true,
			routeHasCompression: true,
			routeHasCache:       true,
			contentType:         "application/json",
			accelBuffering:      "yes",
			expectedCanStream:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				if tc.accelBuffering != "" {
					w.Header().Set("X-Accel-Buffering", tc.accelBuffering)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"result":"payload"}`))
			}))
			defer upstream.Close()

			px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
				Targets:             []string{upstream.URL},
				RouteHasCompression: tc.routeHasCompression,
				RouteHasCache:       tc.routeHasCache,
				Transport: proxy.ProxyTransportConfig{
					StreamResponse: &tc.streamResponse,
				},
			})
			if err != nil {
				t.Fatalf("failed to create proxy: %v", err)
			}
			defer px.Close()

			req, _ := httpparser.NewRequest("GET", "/matrix", "HTTP/1.1")
			res := httpparser.NewResponse()
			px.ServeHTTP(req, res)

			if res.StatusCode != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d", res.StatusCode)
			}

			if tc.expectedCanStream {
				if res.StreamBody == nil {
					t.Fatalf("expected res.StreamBody != nil for canStream=true")
				}
				if res.Body.Len() != 0 {
					t.Fatalf("expected res.Body.Len() == 0 for canStream=true, got %d", res.Body.Len())
				}
				_ = res.StreamBody.Close()
			} else {
				if res.StreamBody != nil {
					t.Fatalf("expected res.StreamBody == nil for canStream=false")
				}
				if res.Body.Len() == 0 {
					t.Fatalf("expected res.Body.Len() > 0 for canStream=false")
				}
			}
		})
	}
}

func TestProxy_WebSocketAlignedStreamHandOff(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "keep-alive, Upgrade")
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Set("Proxy-Authenticate", "Basic")
		w.Header().Set("Proxy-Authorization", "Secret")
		w.Header().Set("TE", "trailers")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("X-Custom-Data", "valid-app-header")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Location", "/v1/stream")
		w.Header().Set("Set-Cookie", "session=xyz; Path=/v1/stream")
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("initial stream bytes from upstream"))
	}))
	defer upstream.Close()

	trTrue := true
	rewrTrue := true
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:           []string{upstream.URL},
		RewriteRedirects:  &rewrTrue,
		RewriteCookiePath: &rewrTrue,
		Transport: proxy.ProxyTransportConfig{
			StreamResponse: &trTrue,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, _ := httpparser.NewRequest("GET", "/public/stream", "HTTP/1.1")
	res := httpparser.NewResponse()

	start := time.Now()
	px.ServeHTTPWithPrefix(req, res, "/public/stream")
	duration := time.Since(start)

	// Subtest 4A: Zero-Copy Non-Blocking Return
	if duration > 100*time.Millisecond {
		t.Fatalf("expected immediate non-blocking return, took %v", duration)
	}
	if res.StreamBody == nil {
		t.Fatal("expected res.StreamBody != nil")
	}
	if res.Body.Len() != 0 {
		t.Fatalf("expected res.Body.Len() == 0, got %d", res.Body.Len())
	}

	// Subtest 4B: RFC 7230 §6.1 Hop-by-Hop Header Stripping
	if res.Header.Get("X-Custom-Data") != "valid-app-header" {
		t.Errorf("expected X-Custom-Data preserved, got %q", res.Header.Get("X-Custom-Data"))
	}
	for _, h := range []string{"Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Transfer-Encoding", "Upgrade"} {
		if res.Header.Get(h) != "" {
			t.Errorf("expected hop-by-hop header %q to be stripped, got %q", h, res.Header.Get(h))
		}
	}

	// Subtest 4D: Cookie & Redirect Rewriting
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "/public/stream") {
		t.Errorf("expected rewritten Location containing /public/stream, got %q", loc)
	}
	cookie := res.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, "Path=/public/stream") {
		t.Errorf("expected rewritten cookie path /public/stream, got %q", cookie)
	}

	// Subtest 4E: Body Stream Ownership Transfer
	buf := make([]byte, 14)
	n, err := io.ReadFull(res.StreamBody, buf)
	if err != nil {
		t.Fatalf("failed reading from res.StreamBody: %v", err)
	}
	if string(buf[:n]) != "initial stream" {
		t.Fatalf("expected 'initial stream', got %q", string(buf[:n]))
	}
	_ = res.StreamBody.Close()

	// Subtest 4C: Upstream Close Propagation
	t.Run("Subtest 4C: Upstream Close Propagation", func(t *testing.T) {
		closeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close")
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		}))
		defer closeUpstream.Close()

		propClose := true
		pxClose, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets: []string{closeUpstream.URL},
			Transport: proxy.ProxyTransportConfig{
				StreamResponse:         &trTrue,
				PropagateUpstreamClose: &propClose,
			},
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer pxClose.Close()

		reqClose, _ := httpparser.NewRequest("GET", "/stream-close", "HTTP/1.1")
		resClose := httpparser.NewResponse()
		pxClose.ServeHTTP(reqClose, resClose)
		if resClose.Header.Get("Connection") != "close" {
			t.Errorf("expected Connection: close propagated, got %q", resClose.Header.Get("Connection"))
		}
		if resClose.StreamBody != nil {
			_ = resClose.StreamBody.Close()
		}
	})
}

// TC-128.10: SSE Direct Socket Fast-Path Activation on Route with Compression & Cache Enabled
func TestProxy_SSE_WithCompressionAndCacheEnabled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		chunks := []string{
			"data: {\"seq\":1}\n\n",
			"data: {\"seq\":2}\n\n",
			"data: {\"seq\":3}\n\n",
		}

		for _, chunk := range chunks {
			_, _ = w.Write([]byte(chunk))
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	trTrue := true
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		RouteHasCache:       true,
		Transport: proxy.ProxyTransportConfig{
			StreamResponse: &trTrue,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	// Client 1 sends request with compression headers
	req1, _ := httpparser.NewRequest("GET", "/events", "HTTP/1.1")
	req1.Header.Set("Accept", "text/event-stream")
	req1.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	res1 := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req1, res1, "/events")

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res1.StatusCode)
	}
	if res1.StreamBody == nil {
		t.Fatalf("expected res1.StreamBody != nil (socket fast-path activated)")
	}
	if res1.Body.Len() != 0 {
		t.Fatalf("expected res1.Body.Len() == 0, got %d", res1.Body.Len())
	}

	body1, err := io.ReadAll(res1.StreamBody)
	if err != nil {
		t.Fatalf("failed to read from StreamBody: %v", err)
	}
	_ = res1.StreamBody.Close()

	expectedAllChunks := "data: {\"seq\":1}\n\ndata: {\"seq\":2}\n\ndata: {\"seq\":3}\n\n"
	if string(body1) != expectedAllChunks {
		t.Fatalf("expected %q, got %q", expectedAllChunks, string(body1))
	}

	// Client 2 sends identical request - must also receive live stream, not cached body
	req2, _ := httpparser.NewRequest("GET", "/events", "HTTP/1.1")
	req2.Header.Set("Accept", "text/event-stream")
	req2.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	res2 := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req2, res2, "/events")

	if res2.StreamBody == nil {
		t.Fatalf("expected res2.StreamBody != nil on second connection")
	}
	if res2.Body.Len() != 0 {
		t.Fatalf("expected res2.Body.Len() == 0 on second connection")
	}

	body2, err := io.ReadAll(res2.StreamBody)
	if err != nil {
		t.Fatalf("failed to read from StreamBody 2: %v", err)
	}
	_ = res2.StreamBody.Close()

	if string(body2) != expectedAllChunks {
		t.Fatalf("expected Client 2 to receive live stream %q, got %q", expectedAllChunks, string(body2))
	}
}

// TC-128.11: X-Accel-Buffering: no Direct Socket Fast-Path Activation
func TestProxy_XAccelBufferingNo_DirectSocketFastPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("streaming unbuffered binary payload"))
	}))
	defer upstream.Close()

	trTrue := true
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		RouteHasCache:       true,
		Transport: proxy.ProxyTransportConfig{
			StreamResponse: &trTrue,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, _ := httpparser.NewRequest("GET", "/stream-unbuf", "HTTP/1.1")
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/stream-unbuf")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody == nil {
		t.Fatalf("expected res.StreamBody != nil due to X-Accel-Buffering: no")
	}
	if res.Body.Len() != 0 {
		t.Fatalf("expected res.Body.Len() == 0, got %d", res.Body.Len())
	}

	data, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("failed to read from stream body: %v", err)
	}
	_ = res.StreamBody.Close()

	if string(data) != "streaming unbuffered binary payload" {
		t.Fatalf("payload mismatch: %q", string(data))
	}
}

// TC-128.12: Standard Response Route Buffering
func TestProxy_StandardJSON_RouteBuffering(t *testing.T) {
	jsonPayload := `{"status":"ok","count":42}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(jsonPayload))
	}))
	defer upstream.Close()

	trTrue := true
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		Transport: proxy.ProxyTransportConfig{
			StreamResponse: &trTrue,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, _ := httpparser.NewRequest("GET", "/api/status", "HTTP/1.1")
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/api/status")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody != nil {
		t.Fatalf("expected res.StreamBody == nil for standard json on compression route")
	}
	if res.Body.Len() == 0 {
		t.Fatalf("expected res.Body.Len() > 0 (buffered payload)")
	}
	if res.Body.String() != jsonPayload {
		t.Fatalf("expected body %q, got %q", jsonPayload, res.Body.String())
	}
}

// TC-128.14: Concurrency, Thread-Safety & Race Cleanliness
func TestProxy_Concurrency_RaceSafety(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: ping\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"sample"}`))
	}))
	defer upstream.Close()

	trTrue := true
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		RouteHasCache:       true,
		Transport: proxy.ProxyTransportConfig{
			StreamResponse: &trTrue,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	var wg sync.WaitGroup
	const concurrency = 50

	// 50 concurrent SSE streaming requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := httpparser.NewRequest("GET", "/events", "HTTP/1.1")
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Accept-Encoding", "gzip, br")
			res := httpparser.NewResponse()

			px.ServeHTTPWithPrefix(req, res, "/events")

			if res.StreamBody != nil {
				_, _ = io.ReadAll(res.StreamBody)
				_ = res.StreamBody.Close()
			}
		}()
	}

	// 50 concurrent standard requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
			req.Header.Set("Accept-Encoding", "gzip")
			res := httpparser.NewResponse()

			px.ServeHTTPWithPrefix(req, res, "/api/data")

			if res.StreamBody != nil {
				_ = res.StreamBody.Close()
			}
		}()
	}

	wg.Wait()
}

// TestProxy_Router_StreamingConcurrencyRaceSafety is an alias verifying TC-128.14 per spec.
func TestProxy_Router_StreamingConcurrencyRaceSafety(t *testing.T) {
	TestProxy_Concurrency_RaceSafety(t)
}

// TC-129.1: Default Configuration Verification (proxy package)
func TestProxy_StreamResponse_DefaultTrue(t *testing.T) {
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{Targets: []string{"http://127.0.0.1:9999"}})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	if !px.IsStreamResponseEnabled() {
		t.Fatalf("expected px.IsStreamResponseEnabled() == true by default")
	}

	f := false
	pxOverride, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{"http://127.0.0.1:9999"},
		Transport: proxy.ProxyTransportConfig{
			StreamResponse: &f,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy with override: %v", err)
	}
	defer pxOverride.Close()

	if pxOverride.IsStreamResponseEnabled() {
		t.Fatalf("expected pxOverride.IsStreamResponseEnabled() == false with explicit override")
	}

	pxFallback, err := proxy.NewReverseProxy("http://127.0.0.1:9999", time.Second)
	if err != nil {
		t.Fatalf("failed to create fallback proxy: %v", err)
	}
	defer pxFallback.Close()

	if !pxFallback.IsStreamResponseEnabled() {
		t.Fatalf("expected pxFallback.IsStreamResponseEnabled() == true")
	}
}

// TC-129.2: Pure Proxy Route Streaming Fast-Path
func TestProxy_PureProxyRoute_StreamingFastPath(t *testing.T) {
	expectedPayload := `{"items":[1,2,3]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedPayload))
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: false,
		RouteHasCache:       false,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, err := httpparser.NewRequest("GET", "/pure-proxy", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	res := httpparser.NewResponse()

	start := time.Now()
	px.ServeHTTPWithPrefix(req, res, "/pure-proxy")
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("expected fast-path handoff < 100ms, took %v", elapsed)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody == nil {
		t.Fatalf("expected non-nil res.StreamBody for pure proxy fast-path")
	}
	if res.Body.Len() != 0 {
		t.Fatalf("expected zero bytes buffered in res.Body, got %d", res.Body.Len())
	}

	bodyBytes, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("failed to read from StreamBody: %v", err)
	}
	_ = res.StreamBody.Close()

	if string(bodyBytes) != expectedPayload {
		t.Fatalf("expected payload %q, got %q", expectedPayload, string(bodyBytes))
	}
}

// TC-129.3: Middleware Route Bounded Ingestion
func TestProxy_DynamicClamp_BoundedPayload_BuffersForMiddleware(t *testing.T) {
	payload50KB := bytes.Repeat([]byte("A"), 51200)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "51200")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload50KB)
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		RouteHasCache:       true,
		MaxPayloadSize:      1048576, // 1 MB
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, err := httpparser.NewRequest("GET", "/api/bounded", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/api/bounded")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody != nil {
		t.Fatalf("expected res.StreamBody == nil for bounded payload on middleware route")
	}
	if res.Body.Len() != 51200 {
		t.Fatalf("expected 51200 bytes buffered in res.Body, got %d", res.Body.Len())
	}
	if !bytes.Equal(res.Body.Bytes(), payload50KB) {
		t.Fatalf("buffered body does not match expected payload")
	}
}

// TC-129.4: Middleware Route Oversized Dynamic Bypass
func TestProxy_DynamicClamp_OversizedPayload_StreamsDirectly(t *testing.T) {
	payloadSize := 5 * 1024 * 1024 // 5 MB
	chunk := bytes.Repeat([]byte("X"), 32768)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", payloadSize))
		w.WriteHeader(http.StatusOK)
		written := 0
		for written < payloadSize {
			toWrite := len(chunk)
			if payloadSize-written < toWrite {
				toWrite = payloadSize - written
			}
			n, _ := w.Write(chunk[:toWrite])
			written += n
		}
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		RouteHasCache:       true,
		MaxPayloadSize:      1048576, // 1 MB
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, err := httpparser.NewRequest("GET", "/large-file", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/large-file")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody == nil {
		t.Fatalf("expected dynamic bypass activated: res.StreamBody != nil")
	}
	if res.Body.Len() != 0 {
		t.Fatalf("expected zero bytes buffered in res.Body, got %d", res.Body.Len())
	}

	copied, err := io.Copy(io.Discard, res.StreamBody)
	if err != nil {
		t.Fatalf("failed to read from StreamBody: %v", err)
	}
	_ = res.StreamBody.Close()

	if copied != int64(payloadSize) {
		t.Fatalf("expected %d bytes copied, got %d", payloadSize, copied)
	}
}

// TC-129.5: Middleware Route Chunked/Unknown Dynamic Bypass
func TestProxy_DynamicClamp_ChunkedUnknownLength_StreamsDirectly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", "application/json")
		flusher, ok := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		for i := 1; i <= 3; i++ {
			_, _ = fmt.Fprintf(w, `{"chunk":%d}`, i)
			if ok {
				flusher.Flush()
			}
		}
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		RouteHasCache:       true,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, err := httpparser.NewRequest("GET", "/dynamic-feed", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/dynamic-feed")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody == nil {
		t.Fatalf("expected chunked response to stream directly: res.StreamBody != nil")
	}
	if res.Body.Len() != 0 {
		t.Fatalf("expected zero bytes buffered in res.Body, got %d", res.Body.Len())
	}

	bodyBytes, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("failed to read from StreamBody: %v", err)
	}
	_ = res.StreamBody.Close()

	expectedContent := `{"chunk":1}{"chunk":2}{"chunk":3}`
	if string(bodyBytes) != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, string(bodyBytes))
	}
}

// TC-129.6: Memory Boundedness & Infinite Stream OOM Bomb Immunity
func TestProxy_DynamicClamp_InfiniteStream_OOMImmunity(t *testing.T) {
	streamBytesTotal := 50 * 1024 * 1024 // 50 MB
	slab := bytes.Repeat([]byte("U"), 32768)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		flusher, ok := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		written := 0
		for written < streamBytesTotal {
			n, err := w.Write(slab)
			if err != nil {
				return
			}
			written += n
			if ok {
				flusher.Flush()
			}
		}
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		RouteHasCache:       true,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, err := httpparser.NewRequest("GET", "/stream/infinite", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	res := httpparser.NewResponse()

	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	px.ServeHTTPWithPrefix(req, res, "/stream/infinite")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StreamBody == nil {
		t.Fatalf("expected res.StreamBody != nil")
	}

	copied, err := io.Copy(io.Discard, res.StreamBody)
	if err != nil {
		t.Fatalf("failed to copy from StreamBody: %v", err)
	}
	_ = res.StreamBody.Close()

	if copied != int64(streamBytesTotal) {
		t.Fatalf("expected %d bytes copied, got %d", streamBytesTotal, copied)
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)
	if res.Body.Len() > 0 {
		t.Fatalf("expected res.Body to remain empty, got %d bytes", res.Body.Len())
	}
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	if heapGrowth > 5*1024*1024 {
		t.Fatalf("heap growth unexpectedly high: %d bytes", heapGrowth)
	}
}

// TC-129.7: LimitReader Safety Clamp on Deceptive Upstreams
func TestProxy_DynamicClamp_LimitReaderSafetyClamp_DeceptiveUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		bigData := bytes.Repeat([]byte("D"), 2*1024*1024)
		_, _ = w.Write(bigData)
	}))
	defer upstream.Close()

	f := false
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:             []string{upstream.URL},
		RouteHasCompression: true,
		MaxPayloadSize:      1048576, // 1 MB
		Transport: proxy.ProxyTransportConfig{
			StreamResponse: &f,
		},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	req, err := httpparser.NewRequest("GET", "/deceptive-endpoint", "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "/deceptive-endpoint")

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway, got %d", res.StatusCode)
	}
	bodyStr := res.Body.String()
	if !strings.Contains(bodyStr, "Upstream payload exceeded maximum allowed buffer limit") {
		t.Fatalf("expected 502 message to indicate buffer limit exceeded, got: %s", bodyStr)
	}
}

// TC-133-12: Canonical Normalization Mode ("normalize") De-chunking & Exact Content-Length Re-framing
func TestReverseProxy_InboundChunked_Normalize(t *testing.T) {
	var (
		mu           sync.Mutex
		receivedTE   string
		receivedCL   string
		receivedBody string
		upstreamHit  bool
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		upstreamHit = true
		receivedTE = r.Header.Get("Transfer-Encoding")
		receivedCL = r.Header.Get("Content-Length")
		receivedBody = string(body)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:            []string{upstream.URL},
		InboundChunkedMode: "normalize",
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	raw := "POST /normalize-target HTTP/1.1\r\n" +
		"Host: proxy.example.com\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"7\r\n" +
		"Hello, \r\n" +
		"6\r\n" +
		"world!\r\n" +
		"0\r\n\r\n"

	req, err := httpparser.ParseRequest(strings.NewReader(raw), httpparser.DefaultParserOptions())
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if !upstreamHit {
		t.Fatal("expected upstream to be called")
	}
	if receivedTE != "" {
		t.Errorf("expected Transfer-Encoding to be stripped, got %q", receivedTE)
	}
	if receivedCL != "13" {
		t.Errorf("expected Content-Length 13, got %q", receivedCL)
	}
	if receivedBody != "Hello, world!" {
		t.Errorf("expected body 'Hello, world!', got %q", receivedBody)
	}
}

// TC-133-13: Canonical Passthrough Streaming Mode ("passthrough") with Validated Chunk Framing
func TestReverseProxy_InboundChunked_Passthrough(t *testing.T) {
	t.Run("FullStream1MB", func(t *testing.T) {
		var (
			mu           sync.Mutex
			receivedTE   string
			totalRead    int
			upstreamDone = make(chan struct{})
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedTE = r.Header.Get("Transfer-Encoding")
			if receivedTE == "" && len(r.TransferEncoding) > 0 {
				receivedTE = r.TransferEncoding[0]
			}
			mu.Unlock()

			buf := make([]byte, 32*1024)
			nRead := 0
			for {
				n, err := r.Body.Read(buf)
				nRead += n
				if err != nil {
					break
				}
			}
			mu.Lock()
			totalRead = nRead
			mu.Unlock()
			close(upstreamDone)

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("streamed-ok"))
		}))
		defer upstream.Close()

		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets:            []string{upstream.URL},
			InboundChunkedMode: "passthrough",
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		// Generate 1 MB in 16 chunks of 64 KB
		var buf bytes.Buffer
		buf.WriteString("POST /stream/upload HTTP/1.1\r\nHost: proxy.example.com\r\nTransfer-Encoding: chunked\r\n\r\n")
		chunk64K := bytes.Repeat([]byte("A"), 64*1024)
		for i := 0; i < 16; i++ {
			buf.WriteString(fmt.Sprintf("%x\r\n", len(chunk64K)))
			buf.Write(chunk64K)
			buf.WriteString("\r\n")
		}
		buf.WriteString("0\r\n\r\n")

		req, err := httpparser.ParseRequest(bytes.NewReader(buf.Bytes()), httpparser.ParserOptions{
			MaxHeaderBytes: 8192,
			MaxBodyBytes:   2 * 1024 * 1024,
		})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}
		res := httpparser.NewResponse()

		px.ServeHTTPWithPrefix(req, res, "")

		select {
		case <-upstreamDone:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for upstream read completion")
		}

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
		}

		mu.Lock()
		defer mu.Unlock()
		if receivedTE != "chunked" {
			t.Errorf("expected upstream Transfer-Encoding: chunked, got %q", receivedTE)
		}
		if totalRead != 16*64*1024 {
			t.Errorf("expected 1MB (1048576) bytes, got %d", totalRead)
		}
	})

	t.Run("PrematureDisconnectCancelsUpstream", func(t *testing.T) {
		upstreamCtxDone := make(chan struct{})

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			go func() {
				<-r.Context().Done()
				select {
				case <-upstreamCtxDone:
				default:
					close(upstreamCtxDone)
				}
			}()

			// Try reading from body until client aborts
			buf := make([]byte, 1024)
			for {
				_, err := r.Body.Read(buf)
				if err != nil {
					return
				}
			}
		}))
		defer upstream.Close()

		px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
			Targets:            []string{upstream.URL},
			InboundChunkedMode: "passthrough",
		})
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}
		defer px.Close()

		// Stream that ends prematurely without terminal 0\r\n\r\n
		raw := "POST /stream/upload HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n5\r\nworld\r\n"
		req, err := httpparser.ParseRequest(strings.NewReader(raw), httpparser.DefaultParserOptions())
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}
		res := httpparser.NewResponse()

		px.ServeHTTPWithPrefix(req, res, "")

		select {
		case <-upstreamCtxDone:
			// Success: upstream request context was cancelled!
		case <-time.After(2 * time.Second):
			t.Fatal("expected upstream context cancellation upon premature disconnect within 2s")
		}
	})
}

// TC-133-14: Upstream Hop-by-Hop Header Sanitization & Trailer Forwarding
func TestReverseProxy_InboundChunked_HopByHopAndTrailers(t *testing.T) {
	var (
		mu              sync.Mutex
		receivedHeaders http.Header
		receivedBody    string
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedHeaders = r.Header.Clone()
		receivedBody = string(body)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:            []string{upstream.URL},
		InboundChunkedMode: "normalize",
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	raw := "POST /hopbyhop-test HTTP/1.1\r\n" +
		"Host: proxy.local\r\n" +
		"Connection: close, X-Custom-Hop\r\n" +
		"X-Custom-Hop: sensitive-gateway-token\r\n" +
		"Keep-Alive: timeout=5\r\n" +
		"Upgrade: websocket\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"Trailer: X-Checksum\r\n" +
		"\r\n" +
		"5\r\n" +
		"hello\r\n" +
		"0\r\n" +
		"X-Checksum: sha256-abcdef123456\r\n" +
		"\r\n"

	req, err := httpparser.ParseRequest(strings.NewReader(raw), httpparser.DefaultParserOptions())
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}
	res := httpparser.NewResponse()

	px.ServeHTTPWithPrefix(req, res, "")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	// Assert hop-by-hop headers are strictly absent
	if receivedHeaders.Get("Keep-Alive") != "" {
		t.Errorf("Keep-Alive must be stripped, got %q", receivedHeaders.Get("Keep-Alive"))
	}
	if receivedHeaders.Get("Upgrade") != "" {
		t.Errorf("Upgrade must be stripped, got %q", receivedHeaders.Get("Upgrade"))
	}
	if receivedHeaders.Get("X-Custom-Hop") != "" {
		t.Errorf("X-Custom-Hop must be stripped, got %q", receivedHeaders.Get("X-Custom-Hop"))
	}
	if receivedHeaders.Get("Transfer-Encoding") != "" {
		t.Errorf("Transfer-Encoding must be stripped, got %q", receivedHeaders.Get("Transfer-Encoding"))
	}
	if receivedHeaders.Get("Trailer") != "" {
		t.Errorf("Trailer must be stripped, got %q", receivedHeaders.Get("Trailer"))
	}

	// Assert valid trailer is forwarded
	if receivedHeaders.Get("X-Checksum") != "sha256-abcdef123456" {
		t.Errorf("expected X-Checksum sha256-abcdef123456, got %q", receivedHeaders.Get("X-Checksum"))
	}

	if receivedBody != "hello" {
		t.Errorf("expected body 'hello', got %q", receivedBody)
	}
}

// TC-133-15: Zero-Allocation Buffer Pool Management & Recycling under High Concurrency
func TestReverseProxy_BufferPoolRecyclingConcurrency(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		h := sha256.Sum256(body)
		w.Header().Set("X-Echo-Hash", hex.EncodeToString(h[:]))
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets:            []string{upstream.URL},
		InboundChunkedMode: "normalize",
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer px.Close()

	const workers = 50
	const iterations = 5

	for it := 0; it < iterations; it++ {
		var wg sync.WaitGroup
		errCh := make(chan error, workers)

		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				// Build unique 32KB payload
				pattern := fmt.Sprintf("Worker-%03d-Iter-%02d-", workerID, it)
				var payloadBuf bytes.Buffer
				for payloadBuf.Len() < 32*1024 {
					payloadBuf.WriteString(pattern)
				}
				payload := payloadBuf.Bytes()[:32*1024]
				expectedHash := sha256.Sum256(payload)
				expectedHashHex := hex.EncodeToString(expectedHash[:])

				// Format as chunked HTTP request
				var chunkedReq bytes.Buffer
				chunkedReq.WriteString("POST /concurrency HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n")
				// Send in 4 chunks of 8KB
				chunkSize := 8 * 1024
				for offset := 0; offset < len(payload); offset += chunkSize {
					chunkedReq.WriteString(fmt.Sprintf("%x\r\n", chunkSize))
					chunkedReq.Write(payload[offset : offset+chunkSize])
					chunkedReq.WriteString("\r\n")
				}
				chunkedReq.WriteString("0\r\n\r\n")

				req, err := httpparser.ParseRequest(bytes.NewReader(chunkedReq.Bytes()), httpparser.DefaultParserOptions())
				if err != nil {
					errCh <- fmt.Errorf("worker %d: ParseRequest failed: %w", workerID, err)
					return
				}
				res := httpparser.NewResponse()

				px.ServeHTTPWithPrefix(req, res, "")

				if res.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("worker %d: expected 200, got %d", workerID, res.StatusCode)
					return
				}

				gotHash := res.Header.Get("X-Echo-Hash")
				if gotHash != expectedHashHex {
					errCh <- fmt.Errorf("worker %d: hash mismatch: expected %s, got %s", workerID, expectedHashHex, gotHash)
					return
				}
			}(w)
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				t.Fatalf("iteration %d failed: %v", it, err)
			}
		}
	}
}
