package proxy_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if res.Body.String() != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
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
		if res.Body.String() != expected {
			t.Errorf("request %d: expected body %q, got %q", i, expected, res.Body.String())
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
		if res.Body.String() != "healthy" {
			t.Errorf("request %d: expected body 'healthy', got %q", i, res.Body.String())
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

	// Repeated requests from same IP should route to same target
	for i := 0; i < 3; i++ {
		req, _ := httpparser.NewRequest("GET", "/app", "HTTP/1.1")
		req.Header.Set("X-Forwarded-For", "192.168.1.50")
		res := httpparser.NewResponse()
		px.ServeHTTP(req, res)

		if res.Body.String() != initialBody {
			t.Errorf("expected ip_hash to pin to %q, got %q", initialBody, res.Body.String())
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
