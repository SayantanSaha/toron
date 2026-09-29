package router_test

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/proxy"
	"toron/pkg/router"
	"toron/pkg/waf"
)

func TestRateLimiter_TokenBucket(t *testing.T) {
	mw, err := router.NewRateLimitMiddleware("2/sec")
	if err != nil {
		t.Fatalf("failed to create rate limit middleware: %v", err)
	}

	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("allowed")
	})

	// Request 1: Allowed (uses token 1)
	req1, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req1.Header.Set("X-API-Key", "test-client-key")
	res1 := httpparser.NewResponse()
	handler(req1, res1)
	if res1.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for req1, got %d", res1.StatusCode)
	}

	// Request 2: Allowed (uses token 2)
	req2, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req2.Header.Set("X-API-Key", "test-client-key")
	res2 := httpparser.NewResponse()
	handler(req2, res2)
	if res2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for req2, got %d", res2.StatusCode)
	}

	// Request 3: Exhausted -> 429 Too Many Requests
	req3, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req3.Header.Set("X-API-Key", "test-client-key")
	res3 := httpparser.NewResponse()
	handler(req3, res3)
	if res3.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 Too Many Requests for req3, got %d", res3.StatusCode)
	}

	if res3.Header.Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}

	// Request 4 from different client key: Allowed (has its own bucket)
	req4, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req4.Header.Set("X-API-Key", "other-client-key")
	res4 := httpparser.NewResponse()
	handler(req4, res4)
	if res4.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for req4 from distinct client, got %d", res4.StatusCode)
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	mw, err := router.NewRateLimitMiddleware("5/sec")
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	// Exhaust 5 tokens
	for i := 0; i < 5; i++ {
		req, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
		req.Header.Set("X-Forwarded-For", "192.168.1.100")
		res := httpparser.NewResponse()
		handler(req, res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("request %d failed prematurely with status %d", i+1, res.StatusCode)
		}
	}

	// 6th request fails
	reqBlocked, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	reqBlocked.Header.Set("X-Forwarded-For", "192.168.1.100")
	resBlocked := httpparser.NewResponse()
	handler(reqBlocked, resBlocked)
	if resBlocked.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", resBlocked.StatusCode)
	}

	// Wait for token refill (250ms -> 1+ tokens accumulated)
	time.Sleep(250 * time.Millisecond)

	reqRefilled, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	reqRefilled.Header.Set("X-Forwarded-For", "192.168.1.100")
	resRefilled := httpparser.NewResponse()
	handler(reqRefilled, resRefilled)
	if resRefilled.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK after refill, got %d", resRefilled.StatusCode)
	}
}

type mockAddr struct {
	addr string
}

func (m *mockAddr) Network() string { return "tcp" }
func (m *mockAddr) String() string  { return m.addr }

type mockConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (m *mockConn) RemoteAddr() net.Addr { return m.remoteAddr }

func TestRateLimiter_CapacityBoundedLRU(t *testing.T) {
	const maxCap = 50
	limiter := router.NewRateLimiter(10, 10, router.RateLimiterOptions{
		MaxBuckets: maxCap,
	})
	defer limiter.Stop()

	// Insert 200 distinct client keys
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("client-%d", i)
		bucket := limiter.GetBucket(key)
		if bucket == nil {
			t.Fatalf("failed to get bucket for %s", key)
		}
	}

	// Verify total stored buckets does not exceed max capacity
	if currentLen := limiter.Len(); currentLen > maxCap {
		t.Fatalf("capacity bound violated: expected <= %d buckets, got %d", maxCap, currentLen)
	}
}

func TestRateLimiter_TTLEviction(t *testing.T) {
	limiter := router.NewRateLimiter(10, 10, router.RateLimiterOptions{
		MaxBuckets: 100,
		IdleTTL:    50 * time.Millisecond,
	})
	defer limiter.Stop()

	limiter.GetBucket("client-stale-1")
	limiter.GetBucket("client-stale-2")

	if limiter.Len() != 2 {
		t.Fatalf("expected 2 active buckets, got %d", limiter.Len())
	}

	// Wait for TTL to pass
	time.Sleep(100 * time.Millisecond)

	// Trigger capacity/access or wait for next bucket access
	limiter.GetBucket("client-active-3")

	// Old stale buckets must have been purged
	if limiter.Len() > 2 {
		t.Fatalf("expected stale buckets to be evicted, got %d", limiter.Len())
	}
}

func TestRateLimiter_UntrustedSocketIP_IgnoresSpoofedXFF(t *testing.T) {
	// Rate limit: 2 requests per second
	mw, err := router.NewRateLimitMiddleware("2/sec")
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("allowed")
	})

	conn := &mockConn{remoteAddr: &mockAddr{addr: "198.51.100.5:54321"}}

	// Request 1 from 198.51.100.5 claiming to be 1.1.1.1 -> OK (token 1)
	req1, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req1.RawConn = conn
	req1.Header.Set("X-Forwarded-For", "1.1.1.1")
	res1 := httpparser.NewResponse()
	handler(req1, res1)
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for req1, got %d", res1.StatusCode)
	}

	// Request 2 from same physical socket claiming to be 2.2.2.2 -> OK (token 2)
	req2, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req2.RawConn = conn
	req2.Header.Set("X-Forwarded-For", "2.2.2.2")
	res2 := httpparser.NewResponse()
	handler(req2, res2)
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for req2, got %d", res2.StatusCode)
	}

	// Request 3 from same physical socket claiming to be 3.3.3.3 -> MUST FAIL with 429
	// Because socket 198.51.100.5 is untrusted, XFF is ignored and all requests share the socket bucket!
	req3, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	req3.RawConn = conn
	req3.Header.Set("X-Forwarded-For", "3.3.3.3")
	res3 := httpparser.NewResponse()
	handler(req3, res3)
	if res3.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("SECURITY VIOLATION: Untrusted peer bypassed rate limits using spoofed X-Forwarded-For! Got %d, expected 429", res3.StatusCode)
	}
}

func TestRateLimiter_TrustedProxy_HonorsXFF(t *testing.T) {
	// Rate limit: 2 requests per second with trusted proxy configured
	mw, err := router.NewRateLimitMiddleware("2/sec", router.RateLimiterOptions{
		TrustedProxies: []string{"10.0.0.1/32"},
	})
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("allowed")
	})

	proxyConn := &mockConn{remoteAddr: &mockAddr{addr: "10.0.0.1:54321"}}

	// Client A through trusted proxy
	reqA1, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	reqA1.RawConn = proxyConn
	reqA1.Header.Set("X-Forwarded-For", "203.0.113.10")
	resA1 := httpparser.NewResponse()
	handler(reqA1, resA1)
	if resA1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for client A1, got %d", resA1.StatusCode)
	}

	reqA2, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	reqA2.RawConn = proxyConn
	reqA2.Header.Set("X-Forwarded-For", "203.0.113.10")
	resA2 := httpparser.NewResponse()
	handler(reqA2, resA2)
	if resA2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for client A2, got %d", resA2.StatusCode)
	}

	// Client B through same trusted proxy gets its OWN isolated bucket
	reqB1, _ := httpparser.NewRequest("GET", "/api", "HTTP/1.1")
	reqB1.RawConn = proxyConn
	reqB1.Header.Set("X-Forwarded-For", "203.0.113.20")
	resB1 := httpparser.NewResponse()
	handler(reqB1, resB1)
	if resB1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for client B1 from trusted proxy, got %d", resB1.StatusCode)
	}
}

func TestServer_H2_RateLimiter_AntiSpoofing(t *testing.T) {
	// Subtest 5A: Header Rotation Attack via X-Forwarded-For over HTTP/2
	t.Run("XFF header rotation does not evade rate limits on untrusted HTTP/2 connection", func(t *testing.T) {
		mw, err := router.NewRateLimitMiddleware("5/sec")
		if err != nil {
			t.Fatalf("failed to create middleware: %v", err)
		}

		handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString("ok")
		})

		// Single attacker on physical connection req.RemoteAddr = "198.51.100.88:51000", RawConn == nil (HTTP/2)
		// 10 rapid requests rotating X-Forwarded-For
		statusCodes := make([]int, 10)
		for i := 0; i < 10; i++ {
			req, _ := httpparser.NewRequest("GET", "/api/v1/search", "HTTP/2.0")
			req.RemoteAddr = "198.51.100.88:51000"
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i+1))
			res := httpparser.NewResponse()
			handler(req, res)
			statusCodes[i] = res.StatusCode
		}

		// First 5 should succeed (burst = 5)
		for i := 0; i < 5; i++ {
			if statusCodes[i] != http.StatusOK {
				t.Fatalf("expected request %d to be 200 OK, got %d", i+1, statusCodes[i])
			}
		}
		// Requests 6 to 10 must be 429 Too Many Requests
		for i := 5; i < 10; i++ {
			if statusCodes[i] != http.StatusTooManyRequests {
				t.Fatalf("SECURITY VIOLATION: request %d with forged XFF bypassed rate limit! Got %d, want 429", i+1, statusCodes[i])
			}
		}
	})

	// Subtest 5B: Header Rotation Attack via X-API-Key over HTTP/2
	t.Run("X-API-Key rotation does not evade rate limits on untrusted HTTP/2 connection", func(t *testing.T) {
		mw, err := router.NewRateLimitMiddleware("5/sec")
		if err != nil {
			t.Fatalf("failed to create middleware: %v", err)
		}

		handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString("ok")
		})

		// Single attacker on 198.51.100.88:51000 rotating X-API-Key
		statusCodes := make([]int, 10)
		for i := 0; i < 10; i++ {
			req, _ := httpparser.NewRequest("GET", "/api/v1/search", "HTTP/2.0")
			req.RemoteAddr = "198.51.100.88:51000"
			req.Header.Set("X-API-Key", fmt.Sprintf("user_%d", i+1))
			res := httpparser.NewResponse()
			handler(req, res)
			statusCodes[i] = res.StatusCode
		}

		for i := 0; i < 5; i++ {
			if statusCodes[i] != http.StatusOK {
				t.Fatalf("expected request %d to be 200 OK, got %d", i+1, statusCodes[i])
			}
		}
		for i := 5; i < 10; i++ {
			if statusCodes[i] != http.StatusTooManyRequests {
				t.Fatalf("SECURITY VIOLATION: request %d with rotating API key bypassed rate limit! Got %d, want 429", i+1, statusCodes[i])
			}
		}
	})

	// Subtest 5C: Trusted Proxy Multi-Client Isolation
	t.Run("Trusted proxy client isolation", func(t *testing.T) {
		mw, err := router.NewRateLimitMiddleware("5/sec", router.RateLimiterOptions{
			TrustedProxies: []string{"172.20.0.0/16"},
		})
		if err != nil {
			t.Fatalf("failed to create middleware: %v", err)
		}

		handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString("ok")
		})

		// Client 1 sends 5 requests with X-Forwarded-For: 203.0.113.1
		for i := 0; i < 5; i++ {
			req, _ := httpparser.NewRequest("GET", "/api/v1/search", "HTTP/2.0")
			req.RemoteAddr = "172.20.0.1:40000"
			req.Header.Set("X-Forwarded-For", "203.0.113.1")
			res := httpparser.NewResponse()
			handler(req, res)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("expected client 1 req %d to succeed, got %d", i+1, res.StatusCode)
			}
		}

		// Client 2 sends 5 requests with X-Forwarded-For: 203.0.113.2 through the same trusted proxy
		for i := 0; i < 5; i++ {
			req, _ := httpparser.NewRequest("GET", "/api/v1/search", "HTTP/2.0")
			req.RemoteAddr = "172.20.0.1:40000"
			req.Header.Set("X-Forwarded-For", "203.0.113.2")
			res := httpparser.NewResponse()
			handler(req, res)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("expected client 2 req %d to succeed in isolated bucket, got %d", i+1, res.StatusCode)
			}
		}
	})
}

func TestRateLimiter_AuditLogger_TelemetryDispatch(t *testing.T) {
	auditLogger, err := waf.NewAuditLogger(waf.AuditLogConfig{
		Enabled: true,
		Output:  "stdout",
		Format:  "json",
	})
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	opts := router.RateLimiterOptions{
		AuditLogger: auditLogger,
	}
	mw, err := router.NewRateLimitMiddleware("2/sec", opts)
	if err != nil {
		t.Fatalf("failed to create rate limit middleware: %v", err)
	}

	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("ok")
	})

	// Request 1: 200 OK
	req1, _ := httpparser.NewRequest("POST", "/api/v1/checkout", "HTTP/1.1")
	req1.Header.Set("X-Forwarded-For", "198.51.100.99")
	res1 := httpparser.NewResponse()
	handler(req1, res1)
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for req1, got %d", res1.StatusCode)
	}

	// Request 2: 200 OK
	req2, _ := httpparser.NewRequest("POST", "/api/v1/checkout", "HTTP/1.1")
	req2.Header.Set("X-Forwarded-For", "198.51.100.99")
	res2 := httpparser.NewResponse()
	handler(req2, res2)
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for req2, got %d", res2.StatusCode)
	}

	// Assert no drops logged before limit exceeded
	if len(auditLogger.RecentEvents()) != 0 {
		t.Fatalf("expected 0 events logged before limit exceeded, got %d", len(auditLogger.RecentEvents()))
	}

	// Request 3: 429 Too Many Requests (burst exceeded)
	req3, _ := httpparser.NewRequest("POST", "/api/v1/checkout", "HTTP/1.1")
	req3.Header.Set("X-Forwarded-For", "198.51.100.99")
	res3 := httpparser.NewResponse()
	handler(req3, res3)

	// Assert HTTP 429 response attributes
	if res3.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests for req3, got %d", res3.StatusCode)
	}
	retryAfter := res3.Header.Get("Retry-After")
	if retryAfter == "" {
		t.Fatalf("expected Retry-After header on 429 response")
	}
	if !strings.Contains(res3.Body.String(), `"error":"429 Too Many Requests"`) {
		t.Fatalf("expected JSON 429 body, got: %s", res3.Body.String())
	}

	// Assert Telemetry Emission to AuditLogger
	recent := auditLogger.RecentEvents()
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent audit event, got %d", len(recent))
	}

	ev := recent[0]
	if ev.Event != "rate_limit_drop" {
		t.Errorf("expected Event 'rate_limit_drop', got %q", ev.Event)
	}
	if ev.Action != "throttled" {
		t.Errorf("expected Action 'throttled', got %q", ev.Action)
	}
	if ev.Category != "rate_limit" {
		t.Errorf("expected Category 'rate_limit', got %q", ev.Category)
	}
	if ev.RuleID != "rate_limit" {
		t.Errorf("expected RuleID 'rate_limit', got %q", ev.RuleID)
	}
	if ev.AnomalyScore != 0 {
		t.Errorf("expected AnomalyScore 0, got %d", ev.AnomalyScore)
	}
	if ev.ClientIP != "198.51.100.99" {
		t.Errorf("expected ClientIP '198.51.100.99', got %q", ev.ClientIP)
	}
	if ev.Method != "POST" {
		t.Errorf("expected Method 'POST', got %q", ev.Method)
	}
	if ev.Path != "/api/v1/checkout" {
		t.Errorf("expected Path '/api/v1/checkout', got %q", ev.Path)
	}
	if !strings.Contains(ev.PayloadSnippet, "Rate limit exceeded: retry after") {
		t.Errorf("expected PayloadSnippet to contain 'Rate limit exceeded: retry after', got %q", ev.PayloadSnippet)
	}
}

func TestRateLimiter_AuditLogger_NilSafety(t *testing.T) {
	mw, err := router.NewRateLimitMiddleware("1/sec", router.RateLimiterOptions{
		AuditLogger: nil,
	})
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	req1, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	req1.Header.Set("X-API-Key", "k1")
	res1 := httpparser.NewResponse()
	handler(req1, res1)
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res1.StatusCode)
	}

	req2, _ := httpparser.NewRequest("GET", "/test", "HTTP/1.1")
	req2.Header.Set("X-API-Key", "k1")
	res2 := httpparser.NewResponse()
	handler(req2, res2)
	if res2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", res2.StatusCode)
	}
}

func TestRateLimiter_AuditLogger_MemoryBounding(t *testing.T) {
	auditLogger, err := waf.NewAuditLogger(waf.AuditLogConfig{
		Enabled: true,
		Output:  "stdout",
		Format:  "json",
	})
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	mw, err := router.NewRateLimitMiddleware("1/sec", router.RateLimiterOptions{
		AuditLogger: auditLogger,
	})
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw(func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	req0, _ := httpparser.NewRequest("GET", "/drop", "HTTP/1.1")
	req0.Header.Set("X-API-Key", "same-client")
	res0 := httpparser.NewResponse()
	handler(req0, res0)

	for i := 0; i < 60; i++ {
		req, _ := httpparser.NewRequest("GET", "/drop", "HTTP/1.1")
		req.Header.Set("X-API-Key", "same-client")
		res := httpparser.NewResponse()
		handler(req, res)
		if res.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("expected 429 on drop %d, got %d", i+1, res.StatusCode)
		}
	}

	recent := auditLogger.RecentEvents()
	if len(recent) != 50 {
		t.Fatalf("expected ring buffer capped at 50 events, got %d", len(recent))
	}
}

func TestRouter_AuditLogger_RouteRateLimitWiring(t *testing.T) {
	r := router.New()
	auditLogger, err := waf.NewAuditLogger(waf.AuditLogConfig{
		Enabled: true,
		Output:  "stdout",
		Format:  "json",
	})
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	r.SetAuditLogger(auditLogger)

	if r.AuditLogger() != auditLogger {
		t.Fatalf("expected router AuditLogger to match set logger")
	}

	err = r.AddRoute(router.PrefixRouteSpec{
		TargetType: router.RouteTypeStatic,
		Prefix:     "/limited",
		DirPath:    ".",
		Opts: proxy.ProxyOptions{
			RateLimit: "1/sec",
		},
	})
	if err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	req1, _ := httpparser.NewRequest("GET", "/limited/go.mod", "HTTP/1.1")
	req1.Header.Set("X-Forwarded-For", "203.0.113.88")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	req2, _ := httpparser.NewRequest("GET", "/limited/go.mod", "HTTP/1.1")
	req2.Header.Set("X-Forwarded-For", "203.0.113.88")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", res2.StatusCode)
	}

	recent := auditLogger.RecentEvents()
	if len(recent) != 1 {
		t.Fatalf("expected 1 audit event emitted via router rate limit, got %d", len(recent))
	}
	if recent[0].Action != "throttled" || recent[0].Event != "rate_limit_drop" {
		t.Fatalf("unexpected event: %+v", recent[0])
	}
	if recent[0].ClientIP != "203.0.113.88" {
		t.Fatalf("expected client IP 203.0.113.88, got %s", recent[0].ClientIP)
	}
}
