package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/proxy"
	"toron/pkg/router"
	"toron/pkg/waf"
)

type antiSpoofMockAddr struct {
	addr string
}

func (m *antiSpoofMockAddr) Network() string { return "tcp" }
func (m *antiSpoofMockAddr) String() string  { return m.addr }

type antiSpoofMockConn struct {
	net.Conn
	remoteAddr string
}

func (m *antiSpoofMockConn) RemoteAddr() net.Addr {
	if m.remoteAddr == "" {
		return nil
	}
	return &antiSpoofMockAddr{addr: m.remoteAddr}
}

// TC-092-02: TestServer_Ingress_RemoteAddrPopulation_H1 verifies that handleConn
// binds req.RemoteAddr and req.RawConn for native HTTP/1.1 requests.
func TestServer_Ingress_RemoteAddrPopulation_H1(t *testing.T) {
	r := router.New()
	var capturedReq *httpparser.Request
	var mu sync.Mutex

	r.GET("/h1-test", func(req *httpparser.Request, res *httpparser.Response) {
		mu.Lock()
		capturedReq = req
		mu.Unlock()
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("ok")
	})

	srv := New(DefaultConfig(), r)

	clientPipe, serverPipe := net.Pipe()
	mockServerConn := &antiSpoofMockConn{
		Conn:       serverPipe,
		remoteAddr: "198.51.100.20:55555",
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.handleConn(context.Background(), mockServerConn)
	}()

	reqBytes := []byte("GET /h1-test HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n")
	_, err := clientPipe.Write(reqBytes)
	if err != nil {
		t.Fatalf("failed to write request bytes: %v", err)
	}

	buf := make([]byte, 1024)
	_, _ = clientPipe.Read(buf)
	_ = clientPipe.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn timed out")
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedReq == nil {
		t.Fatal("captured request was nil")
	}
	if capturedReq.RawConn == nil {
		t.Fatal("expected RawConn to be non-nil")
	}
	if capturedReq.RawConn.RemoteAddr() == nil || capturedReq.RawConn.RemoteAddr().String() != "198.51.100.20:55555" {
		t.Errorf("expected RawConn.RemoteAddr() '198.51.100.20:55555', got %v", capturedReq.RawConn.RemoteAddr())
	}
	if capturedReq.RemoteAddr != "198.51.100.20:55555" {
		t.Errorf("expected RemoteAddr '198.51.100.20:55555', got %q", capturedReq.RemoteAddr)
	}
	if capturedReq.RemoteHost() != "198.51.100.20" {
		t.Errorf("expected RemoteHost '198.51.100.20', got %q", capturedReq.RemoteHost())
	}
	if ip := capturedReq.RemoteIP(); ip == nil || ip.String() != "198.51.100.20" {
		t.Errorf("expected RemoteIP '198.51.100.20', got %v", ip)
	}
}

// TC-092-02: TestServer_Ingress_RemoteAddrPopulation_H2 verifies that http2AdapterHandler
// preserves RemoteAddr while keeping RawConn as nil for HTTP/2 multiplexed streams.
func TestServer_Ingress_RemoteAddrPopulation_H2(t *testing.T) {
	r := router.New()
	var capturedReq *httpparser.Request
	var mu sync.Mutex

	r.GET("/h2-test", func(req *httpparser.Request, res *httpparser.Response) {
		mu.Lock()
		capturedReq = req
		mu.Unlock()
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("ok")
	})

	srv := New(DefaultConfig(), r)
	adapter := srv.HTTP2AdapterHandler()

	stdReq, err := http.NewRequest("GET", "https://example.com/h2-test", nil)
	if err != nil {
		t.Fatalf("failed to create std request: %v", err)
	}
	stdReq.RemoteAddr = "198.51.100.30:60000"
	stdReq.Proto = "HTTP/2.0"

	rr := httptest.NewRecorder()
	adapter.ServeHTTP(rr, stdReq)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedReq == nil {
		t.Fatal("captured request was nil")
	}
	if capturedReq.RawConn != nil {
		t.Errorf("expected RawConn to be nil for HTTP/2 multiplexed stream, got %v", capturedReq.RawConn)
	}
	if capturedReq.RemoteAddr != "198.51.100.30:60000" {
		t.Errorf("expected RemoteAddr '198.51.100.30:60000', got %q", capturedReq.RemoteAddr)
	}
	if capturedReq.RemoteHost() != "198.51.100.30" {
		t.Errorf("expected RemoteHost '198.51.100.30', got %q", capturedReq.RemoteHost())
	}
	if ip := capturedReq.RemoteIP(); ip == nil || ip.String() != "198.51.100.30" {
		t.Errorf("expected RemoteIP '198.51.100.30', got %v", ip)
	}
}

// TC-092-03: TestServer_H2_WAF_SpoofingRejected verifies that WAF IP ACL rejects spoofed
// forwarded headers on HTTP/2 requests, gating forwarded headers strictly behind trusted proxies.
func TestServer_H2_WAF_SpoofingRejected(t *testing.T) {
	// Subtest 3A: Blacklist Evasion Attempt via Spoofed X-Forwarded-For
	t.Run("Blacklist evasion via X-Forwarded-For is rejected", func(t *testing.T) {
		r := router.New()
		wafCfg := waf.DefaultConfig()
		wafCfg.DeniedIPs = []string{"198.51.100.99/32"}
		engine, err := waf.NewEngine(wafCfg)
		if err != nil {
			t.Fatalf("failed to create WAF engine: %v", err)
		}
		wafMw := waf.NewWAFMiddleware(engine)

		r.GET("/secure/data", func(req *httpparser.Request, res *httpparser.Response) {
			wafMw(func(rq *httpparser.Request, rs *httpparser.Response) {
				rs.SetStatus(http.StatusOK)
				_, _ = rs.WriteString(`{"data":"secret"}`)
			})(req, res)
		})

		srv := New(DefaultConfig(), r)
		adapter := srv.HTTP2AdapterHandler()

		stdReq, _ := http.NewRequest("GET", "https://example.com/secure/data", nil)
		stdReq.RemoteAddr = "198.51.100.99:50000"
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("X-Forwarded-For", "203.0.113.1")

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("SECURITY VIOLATION: untrusted client bypassed WAF blacklist via XFF! Got %d, want 403", rr.Code)
		}
	})

	// Subtest 3B: Blacklist Evasion Attempt via Spoofed X-Real-IP
	t.Run("Blacklist evasion via X-Real-IP is rejected", func(t *testing.T) {
		r := router.New()
		wafCfg := waf.DefaultConfig()
		wafCfg.DeniedIPs = []string{"198.51.100.99/32"}
		engine, err := waf.NewEngine(wafCfg)
		if err != nil {
			t.Fatalf("failed to create WAF engine: %v", err)
		}
		wafMw := waf.NewWAFMiddleware(engine)

		r.GET("/secure/data", func(req *httpparser.Request, res *httpparser.Response) {
			wafMw(func(rq *httpparser.Request, rs *httpparser.Response) {
				rs.SetStatus(http.StatusOK)
				_, _ = rs.WriteString(`{"data":"secret"}`)
			})(req, res)
		})

		srv := New(DefaultConfig(), r)
		adapter := srv.HTTP2AdapterHandler()

		stdReq, _ := http.NewRequest("GET", "https://example.com/secure/data", nil)
		stdReq.RemoteAddr = "198.51.100.99:50000"
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("X-Real-IP", "203.0.113.1")

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("SECURITY VIOLATION: untrusted client bypassed WAF blacklist via X-Real-IP! Got %d, want 403", rr.Code)
		}
	})

	// Subtest 3C: Allowlist Bypass Attempt
	t.Run("Allowlist bypass attempt is rejected", func(t *testing.T) {
		r := router.New()
		wafCfg := waf.DefaultConfig()
		wafCfg.AllowedIPs = []string{"10.0.0.0/8"}
		engine, err := waf.NewEngine(wafCfg)
		if err != nil {
			t.Fatalf("failed to create WAF engine: %v", err)
		}
		wafMw := waf.NewWAFMiddleware(engine)

		r.GET("/secure/data", func(req *httpparser.Request, res *httpparser.Response) {
			wafMw(func(rq *httpparser.Request, rs *httpparser.Response) {
				rs.SetStatus(http.StatusOK)
				_, _ = rs.WriteString(`{"data":"secret"}`)
			})(req, res)
		})

		srv := New(DefaultConfig(), r)
		adapter := srv.HTTP2AdapterHandler()

		stdReq, _ := http.NewRequest("GET", "https://example.com/secure/data", nil)
		stdReq.RemoteAddr = "203.0.113.50:45000"
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("X-Forwarded-For", "10.1.2.3")

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("SECURITY VIOLATION: external client bypassed WAF allowlist via XFF! Got %d, want 403", rr.Code)
		}
	})

	// Subtest 3D: Trusted Proxy Delegation
	t.Run("Trusted proxy delegation honors forwarded header", func(t *testing.T) {
		r := router.New()
		wafCfg := waf.DefaultConfig()
		wafCfg.DeniedIPs = []string{"198.51.100.99/32"}
		wafCfg.TrustedProxies = []string{"172.16.0.0/16"}
		engine, err := waf.NewEngine(wafCfg)
		if err != nil {
			t.Fatalf("failed to create WAF engine: %v", err)
		}
		wafMw := waf.NewWAFMiddleware(engine)

		r.GET("/secure/data", func(req *httpparser.Request, res *httpparser.Response) {
			wafMw(func(rq *httpparser.Request, rs *httpparser.Response) {
				rs.SetStatus(http.StatusOK)
				_, _ = rs.WriteString(`{"data":"secret"}`)
			})(req, res)
		})

		srv := New(DefaultConfig(), r)
		adapter := srv.HTTP2AdapterHandler()

		stdReq, _ := http.NewRequest("GET", "https://example.com/secure/data", nil)
		stdReq.RemoteAddr = "172.16.1.1:40000" // Trusted proxy
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("X-Forwarded-For", "198.51.100.99") // Blacklisted end client

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected blacklisted client behind trusted proxy to be blocked, got %d", rr.Code)
		}
	})
}

// TC-092-04: TestServer_H2_InternalAPI_SubnetEnforcement verifies that internal
// management API enforces subnet policies on HTTP/2 requests.
func TestServer_H2_InternalAPI_SubnetEnforcement(t *testing.T) {
	r := router.New()
	cfg := InternalAPIConfig{
		Port:             8080,
		AdminSubnets:     []string{"10.50.0.0/16"},
		AdminAuthEnabled: true,
		AdminToken:       "admin-secret-token",
		TrustedProxies:   []string{"172.16.0.0/12"},
	}
	RegisterInternalAPIRoutes(r, cfg)

	srv := New(DefaultConfig(), r)
	adapter := srv.HTTP2AdapterHandler()

	// Subtest 4A: Untrusted external client spoofing admin subnet
	t.Run("Untrusted client spoofing admin subnet is rejected with 403", func(t *testing.T) {
		stdReq, _ := http.NewRequest("GET", "https://example.com/internal/api/status", nil)
		stdReq.RemoteAddr = "198.51.100.44:54321" // External untrusted IP
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("Authorization", "Bearer admin-secret-token")
		stdReq.Header.Set("X-Forwarded-For", "10.50.1.1")
		stdReq.Header.Set("X-Real-IP", "10.50.1.1")

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("SECURITY VIOLATION: untrusted client accessed internal API via forged XFF! Got %d, want 403", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Access denied by administrative subnet policy") {
			t.Errorf("expected subnet policy denial message, got %q", rr.Body.String())
		}
	})

	// Subtest 4B: Legitimate internal admin client
	t.Run("Legitimate internal admin client is allowed", func(t *testing.T) {
		stdReq, _ := http.NewRequest("GET", "https://example.com/internal/api/status", nil)
		stdReq.RemoteAddr = "10.50.2.15:43210" // Inside admin subnet 10.50.0.0/16
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("Authorization", "Bearer admin-secret-token")

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for legitimate admin client, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	// Subtest 4C: Trusted proxy forwarding admin traffic
	t.Run("Trusted proxy forwarding valid admin subnet is allowed", func(t *testing.T) {
		stdReq, _ := http.NewRequest("GET", "https://example.com/internal/api/status", nil)
		stdReq.RemoteAddr = "172.16.0.10:30000" // In TrustedProxies 172.16.0.0/12
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("Authorization", "Bearer admin-secret-token")
		stdReq.Header.Set("X-Forwarded-For", "10.50.3.4")

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for admin client via trusted proxy, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	// Subtest 4D: Fail-closed default on unresolvable address
	t.Run("Fail-closed denial when client address is unresolvable", func(t *testing.T) {
		req, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/2.0")
		req.RemoteAddr = ""
		req.RawConn = nil
		req.Header.Set("Authorization", "Bearer admin-secret-token")
		req.Header.Set("X-Forwarded-For", "10.50.1.1")

		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)

		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("expected fail-closed 403 Forbidden on unresolvable address, got %d", res.StatusCode)
		}
	})
}

// TC-092-05: TestServer_H2_RateLimiter_Integration verifies rate limiting on HTTP/2 requests.
func TestServer_H2_RateLimiter_Integration(t *testing.T) {
	r := router.New()
	rlMw, err := router.NewRateLimitMiddleware("5/sec")
	if err != nil {
		t.Fatalf("failed to create rate limit middleware: %v", err)
	}

	r.GET("/api/v1/search", func(req *httpparser.Request, res *httpparser.Response) {
		rlMw(func(rq *httpparser.Request, rs *httpparser.Response) {
			rs.SetStatus(http.StatusOK)
			_, _ = rs.WriteString("search results")
		})(req, res)
	})

	srv := New(DefaultConfig(), r)
	adapter := srv.HTTP2AdapterHandler()

	statusCodes := make([]int, 10)
	for i := 0; i < 10; i++ {
		stdReq, _ := http.NewRequest("GET", "https://example.com/api/v1/search", nil)
		stdReq.RemoteAddr = "198.51.100.88:51000"
		stdReq.Proto = "HTTP/2.0"
		stdReq.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i+1))

		rr := httptest.NewRecorder()
		adapter.ServeHTTP(rr, stdReq)
		statusCodes[i] = rr.Code
	}

	for i := 0; i < 5; i++ {
		if statusCodes[i] != http.StatusOK {
			t.Fatalf("expected request %d to be 200 OK, got %d", i+1, statusCodes[i])
		}
	}
	for i := 5; i < 10; i++ {
		if statusCodes[i] != http.StatusTooManyRequests {
			t.Fatalf("SECURITY VIOLATION: request %d with forged XFF evaded rate limits! Got %d, want 429", i+1, statusCodes[i])
		}
	}
}

// TC-092-07: TestServer_H3_RemoteAddrBinding verifies that HTTP/3 requests processed through
// http2AdapterHandler have physical UDP peer RemoteAddr bound, rejecting spoofing attempts.
func TestServer_H3_RemoteAddrBinding(t *testing.T) {
	r := router.New()
	wafCfg := waf.DefaultConfig()
	wafCfg.DeniedIPs = []string{"198.51.100.40/32"}
	engine, err := waf.NewEngine(wafCfg)
	if err != nil {
		t.Fatalf("failed to create WAF engine: %v", err)
	}
	wafMw := waf.NewWAFMiddleware(engine)

	var capturedReq *httpparser.Request
	var mu sync.Mutex

	r.GET("/h3-secure", func(req *httpparser.Request, res *httpparser.Response) {
		mu.Lock()
		capturedReq = req
		mu.Unlock()
		wafMw(func(rq *httpparser.Request, rs *httpparser.Response) {
			rs.SetStatus(http.StatusOK)
			_, _ = rs.WriteString("h3 ok")
		})(req, res)
	})

	srv := New(DefaultConfig(), r)
	adapter := srv.HTTP2AdapterHandler()

	// Construct simulated HTTP/3 request
	stdReq, err := http.NewRequest("GET", "https://example.com/h3-secure", nil)
	if err != nil {
		t.Fatalf("failed to create HTTP/3 request: %v", err)
	}
	stdReq.RemoteAddr = "198.51.100.40:4433" // UDP datagram peer address
	stdReq.Proto = "HTTP/3.0"
	stdReq.Header.Set("X-Forwarded-For", "10.0.0.1") // Attempted spoofing

	rr := httptest.NewRecorder()
	adapter.ServeHTTP(rr, stdReq)

	// Assertions
	if rr.Code != http.StatusForbidden {
		t.Fatalf("SECURITY VIOLATION: HTTP/3 request bypassed WAF blacklist via spoofed XFF! Got %d, want 403", rr.Code)
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedReq == nil {
		t.Fatal("captured request was nil")
	}
	if capturedReq.RemoteAddr != "198.51.100.40:4433" {
		t.Errorf("expected RemoteAddr '198.51.100.40:4433', got %q", capturedReq.RemoteAddr)
	}
	if capturedReq.RemoteHost() != "198.51.100.40" {
		t.Errorf("expected RemoteHost '198.51.100.40', got %q", capturedReq.RemoteHost())
	}
	if ip := capturedReq.RemoteIP(); ip == nil || ip.String() != "198.51.100.40" {
		t.Errorf("expected RemoteIP '198.51.100.40', got %v", ip)
	}
}

// TC-092-10: TestServer_AntiSpoofing_ConcurrentRaceClean verifies concurrency safety
// and freedom from data races across WAF, Internal API, Rate Limiter, and Reverse Proxy.
func TestServer_AntiSpoofing_ConcurrentRaceClean(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream response"))
	}))
	defer upstreamServer.Close()

	r := router.New()

	// 1. WAF endpoint
	wafCfg := waf.DefaultConfig()
	wafCfg.DeniedIPs = []string{"198.51.100.99/32"}
	wafEngine, _ := waf.NewEngine(wafCfg)
	wafMw := waf.NewWAFMiddleware(wafEngine)
	r.GET("/concurrent/waf", func(req *httpparser.Request, res *httpparser.Response) {
		wafMw(func(rq *httpparser.Request, rs *httpparser.Response) {
			rs.SetStatus(http.StatusOK)
			_, _ = rs.WriteString("waf ok")
		})(req, res)
	})

	// 2. Internal API endpoint
	apiCfg := InternalAPIConfig{
		Port:             8080,
		AdminSubnets:     []string{"10.50.0.0/16"},
		AdminAuthEnabled: true,
		AdminToken:       "race-secret-token",
	}
	RegisterInternalAPIRoutes(r, apiCfg)

	// 3. Rate-limited endpoint
	rlMw, _ := router.NewRateLimitMiddleware("5000/sec")
	r.GET("/concurrent/ratelimit", func(req *httpparser.Request, res *httpparser.Response) {
		rlMw(func(rq *httpparser.Request, rs *httpparser.Response) {
			rs.SetStatus(http.StatusOK)
			_, _ = rs.WriteString("rl ok")
		})(req, res)
	})

	// 4. Reverse Proxy endpoint
	px, err := proxy.NewProxyWithOptions(proxy.ProxyOptions{
		Targets: []string{upstreamServer.URL},
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	r.GET("/concurrent/proxy", func(req *httpparser.Request, res *httpparser.Response) {
		px.ServeHTTPWithPrefix(req, res, "/concurrent/proxy")
	})

	srv := New(DefaultConfig(), r)
	adapter := srv.HTTP2AdapterHandler()

	concurrency := 100
	iterations := 50
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		workerID := i
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				var path string
				var remoteAddr string
				var xff string

				switch workerID % 4 {
				case 0:
					path = "/concurrent/waf"
					remoteAddr = fmt.Sprintf("198.51.100.%d:%d", (workerID%20)+1, 40000+j)
					xff = fmt.Sprintf("10.0.%d.%d", workerID, j)
				case 1:
					path = "/internal/api/status"
					remoteAddr = fmt.Sprintf("10.50.%d.%d:%d", workerID%5, j%20, 50000+j)
					xff = "10.50.1.1"
				case 2:
					path = "/concurrent/ratelimit"
					remoteAddr = fmt.Sprintf("192.0.2.%d:%d", workerID, 30000+j)
					xff = fmt.Sprintf("172.16.%d.%d", workerID, j)
				case 3:
					path = "/concurrent/proxy"
					remoteAddr = fmt.Sprintf("203.0.113.%d:%d", workerID, 20000+j)
					xff = fmt.Sprintf("8.8.%d.%d", workerID, j)
				}

				stdReq, err := http.NewRequest("GET", "https://example.com"+path, bytes.NewReader(nil))
				if err != nil {
					continue
				}
				stdReq.RemoteAddr = remoteAddr
				stdReq.Proto = "HTTP/2.0"
				stdReq.Header.Set("X-Forwarded-For", xff)
				if strings.HasPrefix(path, "/internal/api") {
					stdReq.Header.Set("Authorization", "Bearer race-secret-token")
				}

				rr := httptest.NewRecorder()
				adapter.ServeHTTP(rr, stdReq)
			}
		}()
	}

	wg.Wait()
}
