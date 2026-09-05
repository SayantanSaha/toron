package router_test

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
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
