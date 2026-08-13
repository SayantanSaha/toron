package router_test

import (
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
