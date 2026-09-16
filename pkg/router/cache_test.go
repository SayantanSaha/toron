package router

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/httpparser"
)

func TestCache_HitAndMiss(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 10 * time.Second
	r.Use(NewCacheMiddleware(cfg))

	var handlerCalls int64

	r.GET("/api/cached-endpoint", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(fmt.Sprintf(`{"count":%d,"data":"sample payload"}`, atomic.LoadInt64(&handlerCalls)))
	})

	// Request 1: Cache MISS
	req1, _ := httpparser.NewRequest("GET", "/api/cached-endpoint", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res1.StatusCode)
	}
	if cacheHeader := res1.Header.Get("X-Cache"); cacheHeader != "MISS" {
		t.Fatalf("expected X-Cache 'MISS', got %q", cacheHeader)
	}
	if calls := atomic.LoadInt64(&handlerCalls); calls != 1 {
		t.Fatalf("expected 1 handler call, got %d", calls)
	}
	body1 := res1.Body.String()

	// Request 2: Cache HIT
	req2, _ := httpparser.NewRequest("GET", "/api/cached-endpoint", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res2.StatusCode)
	}
	if cacheHeader := res2.Header.Get("X-Cache"); cacheHeader != "HIT" {
		t.Fatalf("expected X-Cache 'HIT', got %q", cacheHeader)
	}
	if age := res2.Header.Get("Age"); age == "" {
		t.Fatalf("expected Age header on cache HIT")
	}
	if calls := atomic.LoadInt64(&handlerCalls); calls != 1 {
		t.Fatalf("expected handler calls to remain 1 on cache HIT, got %d", calls)
	}
	if body2 := res2.Body.String(); body2 != body1 {
		t.Fatalf("expected cached body %q, got %q", body1, body2)
	}
}

func TestCache_MaxAgeExpiration(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	r.Use(NewCacheMiddleware(cfg))

	var handlerCalls int64

	r.GET("/api/short-ttl", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		res.Header.Set("Cache-Control", "max-age=1")
		_, _ = res.WriteString(fmt.Sprintf("response #%d", atomic.LoadInt64(&handlerCalls)))
	})

	// Request 1: MISS
	req1, _ := httpparser.NewRequest("GET", "/api/short-ttl", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS")
	}

	// Request 2: HIT
	req2, _ := httpparser.NewRequest("GET", "/api/short-ttl", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("expected HIT")
	}

	// Wait for TTL to expire (>1s)
	time.Sleep(1100 * time.Millisecond)

	// Request 3: Expired -> MISS
	req3, _ := httpparser.NewRequest("GET", "/api/short-ttl", "HTTP/1.1")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)
	if res3.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS after expiration")
	}
	if calls := atomic.LoadInt64(&handlerCalls); calls != 2 {
		t.Fatalf("expected 2 handler calls after expiration, got %d", calls)
	}
}

func TestCache_NoStoreBypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	r.Use(NewCacheMiddleware(cfg))

	var handlerCalls int64

	r.GET("/api/private-data", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Cache-Control", "no-store, private")
		_, _ = res.WriteString("secret data")
	})

	req1, _ := httpparser.NewRequest("GET", "/api/private-data", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS")
	}

	req2, _ := httpparser.NewRequest("GET", "/api/private-data", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS for no-store response")
	}
	if calls := atomic.LoadInt64(&handlerCalls); calls != 2 {
		t.Fatalf("expected handler to run twice for no-store, got %d", calls)
	}
}

func TestCache_ClientNoCacheBypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	r.Use(NewCacheMiddleware(cfg))

	var handlerCalls int64

	r.GET("/api/refreshable", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerCalls, 1)
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString(fmt.Sprintf("version %d", atomic.LoadInt64(&handlerCalls)))
	})

	// Initial request populates cache
	req1, _ := httpparser.NewRequest("GET", "/api/refreshable", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	// Client sends Cache-Control: no-cache
	req2, _ := httpparser.NewRequest("GET", "/api/refreshable", "HTTP/1.1")
	req2.Header.Set("Cache-Control", "no-cache")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS when client specifies no-cache")
	}
	if calls := atomic.LoadInt64(&handlerCalls); calls != 2 {
		t.Fatalf("expected handler to re-run on client no-cache, got %d", calls)
	}
}

func TestCache_NonGetMethodsBypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	r.Use(NewCacheMiddleware(cfg))

	var handlerCalls int64

	r.Handle("POST", "/api/mutate", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerCalls, 1)
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("created")
	})

	req1, _ := httpparser.NewRequest("POST", "/api/mutate", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	req2, _ := httpparser.NewRequest("POST", "/api/mutate", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if calls := atomic.LoadInt64(&handlerCalls); calls != 2 {
		t.Fatalf("expected 2 handler calls for POST, got %d", calls)
	}
}

func TestCache_MaxPayloadSize(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.MaxPayloadSize = 50
	r.Use(NewCacheMiddleware(cfg))

	var handlerCalls int64
	largePayload := strings.Repeat("Larger than 50 bytes payload data. ", 10)

	r.GET("/api/large", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerCalls, 1)
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString(largePayload)
	})

	req1, _ := httpparser.NewRequest("GET", "/api/large", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	req2, _ := httpparser.NewRequest("GET", "/api/large", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if calls := atomic.LoadInt64(&handlerCalls); calls != 2 {
		t.Fatalf("expected oversized payload not to be cached, got %d calls", calls)
	}
}

func TestCache_WebSocketBypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	r.Use(NewCacheMiddleware(cfg))

	r.GET("/ws", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
	})

	req1, _ := httpparser.NewRequest("GET", "/ws", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected status 101, got %d", res1.StatusCode)
	}
}

func TestCache_DistinctStaticPaths(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	r.Use(NewCacheMiddleware(cfg))

	r.GET("/internal/dashboard/style.css", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/css")
		_, _ = res.WriteString("body { color: red; }")
	})
	r.GET("/internal/dashboard/app.js", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/javascript")
		_, _ = res.WriteString("console.log('hello');")
	})

	// Request CSS
	reqCSS := &httpparser.Request{
		Method:     "GET",
		Path:       "/internal/dashboard/style.css",
		RequestURI: "/internal/dashboard/style.css",
		Proto:      "HTTP/2.0",
		Header:     make(httpparser.Header),
	}
	resCSS := httpparser.NewResponse()
	r.ServeHTTP(reqCSS, resCSS)

	if resCSS.Header.Get("Content-Type") != "text/css" {
		t.Fatalf("expected text/css, got %s", resCSS.Header.Get("Content-Type"))
	}
	if resCSS.Body.String() != "body { color: red; }" {
		t.Fatalf("unexpected CSS body: %s", resCSS.Body.String())
	}

	// Request JS (must NOT return cached CSS)
	reqJS := &httpparser.Request{
		Method:     "GET",
		Path:       "/internal/dashboard/app.js",
		RequestURI: "/internal/dashboard/app.js",
		Proto:      "HTTP/2.0",
		Header:     make(httpparser.Header),
	}
	resJS := httpparser.NewResponse()
	r.ServeHTTP(reqJS, resJS)

	if resJS.Header.Get("Content-Type") != "application/javascript" {
		t.Fatalf("expected application/javascript, got %s", resJS.Header.Get("Content-Type"))
	}
	if resJS.Body.String() != "console.log('hello');" {
		t.Fatalf("unexpected JS body: %s", resJS.Body.String())
	}
}

func TestCache_SetCookieStripped(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 10 * time.Second
	r.Use(NewCacheMiddleware(cfg))

	r.GET("/api/session", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Set-Cookie", "session=secret_token_12345; Path=/; HttpOnly")
		res.Header.Set("Set-Cookie2", "session2=secret_token_67890; Path=/")
		_, _ = res.WriteString(`{"user":"authenticated"}`)
	})

	// Request 1: Initial response emits Set-Cookie from upstream
	req1, _ := httpparser.NewRequest("GET", "/api/session", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS on first request")
	}
	if cookie := res1.Header.Get("Set-Cookie"); cookie != "session=secret_token_12345; Path=/; HttpOnly" {
		t.Fatalf("expected Set-Cookie to be present on upstream response, got %q", cookie)
	}

	// Request 2: Cached HIT must NEVER emit Set-Cookie
	req2, _ := httpparser.NewRequest("GET", "/api/session", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("expected HIT on second request")
	}
	if cookie := res2.Header.Get("Set-Cookie"); cookie != "" {
		t.Fatalf("SECURITY VIOLATION: Set-Cookie leaked in cached response: %q", cookie)
	}
	if cookie2 := res2.Header.Get("Set-Cookie2"); cookie2 != "" {
		t.Fatalf("SECURITY VIOLATION: Set-Cookie2 leaked in cached response: %q", cookie2)
	}
}

func TestCache_AuthorizationBoundary(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 10 * time.Second
	r.Use(NewCacheMiddleware(cfg))

	var privateCalls int64
	r.GET("/api/user/profile", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&privateCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(fmt.Sprintf(`{"profile_id":%d}`, atomic.LoadInt64(&privateCalls)))
	})

	var publicCalls int64
	r.GET("/api/public/data", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&publicCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Cache-Control", "public, max-age=60")
		_, _ = res.WriteString(fmt.Sprintf(`{"public_id":%d}`, atomic.LoadInt64(&publicCalls)))
	})

	// 1. Request with Authorization on private endpoint must not be cached (RFC 7234 §3.2)
	reqAuth1, _ := httpparser.NewRequest("GET", "/api/user/profile", "HTTP/1.1")
	reqAuth1.Header.Set("Authorization", "Bearer user1-token")
	resAuth1 := httpparser.NewResponse()
	r.ServeHTTP(reqAuth1, resAuth1)
	if resAuth1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS")
	}

	// Subsequent request without auth must NOT receive cached private data
	reqAnon, _ := httpparser.NewRequest("GET", "/api/user/profile", "HTTP/1.1")
	resAnon := httpparser.NewResponse()
	r.ServeHTTP(reqAnon, resAnon)
	if resAnon.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("SECURITY VIOLATION: Anonymous request received cached authenticated response")
	}
	if calls := atomic.LoadInt64(&privateCalls); calls != 2 {
		t.Fatalf("expected handler to run twice for authenticated requests without public directive, got %d", calls)
	}

	// 2. Request with Authorization on public endpoint CAN be cached
	reqPub1, _ := httpparser.NewRequest("GET", "/api/public/data", "HTTP/1.1")
	reqPub1.Header.Set("Authorization", "Bearer user1-token")
	resPub1 := httpparser.NewResponse()
	r.ServeHTTP(reqPub1, resPub1)
	if resPub1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS on first public request")
	}

	reqPub2, _ := httpparser.NewRequest("GET", "/api/public/data", "HTTP/1.1")
	reqPub2.Header.Set("Authorization", "Bearer user1-token")
	resPub2 := httpparser.NewResponse()
	r.ServeHTTP(reqPub2, resPub2)
	if resPub2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("expected HIT on second public request")
	}
	if calls := atomic.LoadInt64(&publicCalls); calls != 1 {
		t.Fatalf("expected public endpoint to be cached once, got %d calls", calls)
	}
}

func TestCache_VaryHeader(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 10 * time.Second
	r.Use(NewCacheMiddleware(cfg))

	var customVaryCalls int64
	r.GET("/api/vary-custom", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&customVaryCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Vary", "User-Agent, Cookie")
		_, _ = res.WriteString("custom vary content")
	})

	// Request with unhandled Vary dimensions must NOT be cached
	req1, _ := httpparser.NewRequest("GET", "/api/vary-custom", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	req2, _ := httpparser.NewRequest("GET", "/api/vary-custom", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if calls := atomic.LoadInt64(&customVaryCalls); calls != 2 {
		t.Fatalf("expected responses with unsupported Vary dimensions not to be cached, got %d calls", calls)
	}
}

func TestCacheMiddleware_StreamingBypass(t *testing.T) {
	t.Run("Subtest 5B: Streaming response bypasses cache, Subtest 5C: standard response cached", func(t *testing.T) {
		r := New()
		cfg := DefaultCacheConfig()
		cfg.DefaultTTL = 60 * time.Second
		r.Use(NewCacheMiddleware(cfg))

		var streamCalls int64
		r.GET("/events", func(req *httpparser.Request, res *httpparser.Response) {
			atomic.AddInt64(&streamCalls, 1)
			res.SetStatus(http.StatusOK)
			res.Header.Set("Content-Type", "text/event-stream")
			res.StreamBody = io.NopCloser(strings.NewReader("event: live\n\n"))
		})

		var jsonCalls int64
		r.GET("/api/data", func(req *httpparser.Request, res *httpparser.Response) {
			atomic.AddInt64(&jsonCalls, 1)
			res.SetStatus(http.StatusOK)
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(strings.Repeat(`{"data":123},`, 50))
		})

		// 1. Streaming request
		req1, _ := httpparser.NewRequest("GET", "/events", "HTTP/1.1")
		res1 := httpparser.NewResponse()
		r.ServeHTTP(req1, res1)

		if res1.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("expected X-Cache MISS on streaming response, got %q", res1.Header.Get("X-Cache"))
		}
		if res1.Header.Get("Age") != "" {
			t.Fatalf("expected Age header to be empty for streaming response, got %q", res1.Header.Get("Age"))
		}

		// 2. Second streaming request must also be MISS
		req2, _ := httpparser.NewRequest("GET", "/events", "HTTP/1.1")
		res2 := httpparser.NewResponse()
		r.ServeHTTP(req2, res2)

		if res2.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("expected X-Cache MISS on second streaming request, got %q", res2.Header.Get("X-Cache"))
		}
		if calls := atomic.LoadInt64(&streamCalls); calls != 2 {
			t.Fatalf("expected streaming endpoint called twice (not cached), got %d", calls)
		}

		// 3. Standard JSON request (regression check)
		reqJSON1, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
		resJSON1 := httpparser.NewResponse()
		r.ServeHTTP(reqJSON1, resJSON1)
		if resJSON1.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("expected first JSON request to be MISS, got %q", resJSON1.Header.Get("X-Cache"))
		}

		// 4. Second standard JSON request must be HIT
		reqJSON2, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
		resJSON2 := httpparser.NewResponse()
		r.ServeHTTP(reqJSON2, resJSON2)
		if resJSON2.Header.Get("X-Cache") != "HIT" {
			t.Fatalf("expected second JSON request to be HIT, got %q", resJSON2.Header.Get("X-Cache"))
		}
		if calls := atomic.LoadInt64(&jsonCalls); calls != 1 {
			t.Fatalf("expected JSON endpoint called once due to cache hit, got %d", calls)
		}
	})

	t.Run("X-Accel-Buffering: no bypasses cache", func(t *testing.T) {
		r := New()
		cfg := DefaultCacheConfig()
		cfg.DefaultTTL = 60 * time.Second
		r.Use(NewCacheMiddleware(cfg))

		var unbufCalls int64
		r.GET("/unbuffered", func(req *httpparser.Request, res *httpparser.Response) {
			atomic.AddInt64(&unbufCalls, 1)
			res.SetStatus(http.StatusOK)
			res.Header.Set("Content-Type", "text/plain")
			res.Header.Set("X-Accel-Buffering", "no")
			_, _ = res.WriteString("realtime content")
		})

		req1, _ := httpparser.NewRequest("GET", "/unbuffered", "HTTP/1.1")
		res1 := httpparser.NewResponse()
		r.ServeHTTP(req1, res1)
		if res1.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("expected MISS, got %s", res1.Header.Get("X-Cache"))
		}

		req2, _ := httpparser.NewRequest("GET", "/unbuffered", "HTTP/1.1")
		res2 := httpparser.NewResponse()
		r.ServeHTTP(req2, res2)
		if res2.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("expected MISS on second request for unbuffered, got %s", res2.Header.Get("X-Cache"))
		}
		if calls := atomic.LoadInt64(&unbufCalls); calls != 2 {
			t.Fatalf("expected unbuffered endpoint called twice, got %d", calls)
		}
	})
}

// TC-128.1: RFC 7234 §5.2.2.2 Origin Cache-Control: no-cache Bypass
func TestCache_RFC7234_OriginNoCache_Bypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var handlerHits int64
	r.GET("/api/dynamic", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Cache-Control", "no-cache")
		_, _ = res.WriteString(`{"timestamp": 123456789}`)
	})

	// Request 1: cache miss
	req1, _ := httpparser.NewRequest("GET", "/api/dynamic", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res1.StatusCode)
	}
	if xCache := res1.Header.Get("X-Cache"); xCache != "MISS" {
		t.Fatalf("expected X-Cache 'MISS', got %q", xCache)
	}
	if age := res1.Header.Get("Age"); age != "" {
		t.Fatalf("expected empty Age header, got %q", age)
	}
	if hits := atomic.LoadInt64(&handlerHits); hits != 1 {
		t.Fatalf("expected 1 handler hit, got %d", hits)
	}

	// Request 2: identical request must also be MISS and invoke handler
	req2, _ := httpparser.NewRequest("GET", "/api/dynamic", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res2.StatusCode)
	}
	if xCache := res2.Header.Get("X-Cache"); xCache != "MISS" {
		t.Fatalf("expected X-Cache 'MISS' on second request, got %q", xCache)
	}
	if age := res2.Header.Get("Age"); age != "" {
		t.Fatalf("expected empty Age header on second request, got %q", age)
	}
	if hits := atomic.LoadInt64(&handlerHits); hits != 2 {
		t.Fatalf("expected 2 handler hits, got %d", hits)
	}

	if store.Len() != 0 {
		t.Fatalf("expected 0 cached entries, got %d", store.Len())
	}
}

// TC-128.2: Origin Cache-Control: no-cache with max-age Precedence
func TestCache_OriginNoCache_WithMaxAge_Bypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var handlerHits int64
	r.GET("/api/no-cache-with-maxage", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&handlerHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Cache-Control", "no-cache, max-age=3600")
		_, _ = res.WriteString(`{"status":"fresh"}`)
	})

	req1, _ := httpparser.NewRequest("GET", "/api/no-cache-with-maxage", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if xCache := res1.Header.Get("X-Cache"); xCache != "MISS" {
		t.Fatalf("expected X-Cache 'MISS', got %q", xCache)
	}
	if age := res1.Header.Get("Age"); age != "" {
		t.Fatalf("expected empty Age header, got %q", age)
	}

	req2, _ := httpparser.NewRequest("GET", "/api/no-cache-with-maxage", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if xCache := res2.Header.Get("X-Cache"); xCache != "MISS" {
		t.Fatalf("expected X-Cache 'MISS' on second request, got %q", xCache)
	}
	if age := res2.Header.Get("Age"); age != "" {
		t.Fatalf("expected empty Age header on second request, got %q", age)
	}
	if hits := atomic.LoadInt64(&handlerHits); hits != 2 {
		t.Fatalf("expected 2 handler hits, got %d", hits)
	}

	if store.Len() != 0 {
		t.Fatalf("expected 0 cached entries, got %d", store.Len())
	}
}

// TC-128.3: Content-Type: text/event-stream Cache Exemption
func TestCache_TextEventStream_Bypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var standardHits, paramsHits, upperHits int64

	r.GET("/events/standard", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&standardHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")
		_, _ = res.WriteString("data: event\n\n")
	})

	r.GET("/events/params", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&paramsHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = res.WriteString("data: event\n\n")
	})

	r.GET("/events/uppercase", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&upperHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "TEXT/EVENT-STREAM")
		_, _ = res.WriteString("data: event\n\n")
	})

	// Subtest 3A (Standard text/event-stream)
	for i := 0; i < 2; i++ {
		req, _ := httpparser.NewRequest("GET", "/events/standard", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("iteration %d: expected X-Cache MISS, got %q", i, res.Header.Get("X-Cache"))
		}
		if res.Header.Get("Age") != "" {
			t.Fatalf("iteration %d: expected empty Age header, got %q", i, res.Header.Get("Age"))
		}
	}
	if hits := atomic.LoadInt64(&standardHits); hits != 2 {
		t.Fatalf("expected 2 hits on /events/standard, got %d", hits)
	}

	// Subtest 3B (MIME Parameters - charset=utf-8)
	for i := 0; i < 2; i++ {
		req, _ := httpparser.NewRequest("GET", "/events/params", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("iteration %d: expected X-Cache MISS, got %q", i, res.Header.Get("X-Cache"))
		}
		if res.Header.Get("Age") != "" {
			t.Fatalf("iteration %d: expected empty Age header, got %q", i, res.Header.Get("Age"))
		}
	}
	if hits := atomic.LoadInt64(&paramsHits); hits != 2 {
		t.Fatalf("expected 2 hits on /events/params, got %d", hits)
	}

	// Subtest 3C (Case-Insensitive TEXT/EVENT-STREAM)
	for i := 0; i < 2; i++ {
		req, _ := httpparser.NewRequest("GET", "/events/uppercase", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("iteration %d: expected X-Cache MISS, got %q", i, res.Header.Get("X-Cache"))
		}
		if res.Header.Get("Age") != "" {
			t.Fatalf("iteration %d: expected empty Age header, got %q", i, res.Header.Get("Age"))
		}
	}
	if hits := atomic.LoadInt64(&upperHits); hits != 2 {
		t.Fatalf("expected 2 hits on /events/uppercase, got %d", hits)
	}

	// Subtest 3D (Cache Memory Inspection)
	if store.Len() != 0 {
		t.Fatalf("expected 0 cached entries across all SSE endpoints, got %d", store.Len())
	}
}

// TC-128.4: X-Accel-Buffering: no Cache Exemption
func TestCache_XAccelBufferingNo_Bypass(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var unbufHits int64
	r.GET("/unbuffered-feed", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&unbufHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		res.Header.Set("X-Accel-Buffering", "no")
		_, _ = res.WriteString("live feed")
	})

	req1, _ := httpparser.NewRequest("GET", "/unbuffered-feed", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected X-Cache MISS, got %q", res1.Header.Get("X-Cache"))
	}

	req2, _ := httpparser.NewRequest("GET", "/unbuffered-feed", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected X-Cache MISS on second request, got %q", res2.Header.Get("X-Cache"))
	}
	if res2.Header.Get("Age") != "" {
		t.Fatalf("expected empty Age header, got %q", res2.Header.Get("Age"))
	}
	if hits := atomic.LoadInt64(&unbufHits); hits != 2 {
		t.Fatalf("expected 2 hits on unbuffered feed, got %d", hits)
	}

	// Test case variations: "No" and "NO"
	var caseHits int64
	r.GET("/unbuffered-mixed-case", func(req *httpparser.Request, res *httpparser.Response) {
		h := atomic.AddInt64(&caseHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		if h%2 == 1 {
			res.Header.Set("X-Accel-Buffering", "No")
		} else {
			res.Header.Set("X-Accel-Buffering", "NO")
		}
		_, _ = res.WriteString("live feed variation")
	})

	for i := 0; i < 2; i++ {
		req, _ := httpparser.NewRequest("GET", "/unbuffered-mixed-case", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("case variation %d: expected X-Cache MISS, got %q", i, res.Header.Get("X-Cache"))
		}
		if res.Header.Get("Age") != "" {
			t.Fatalf("case variation %d: expected empty Age header, got %q", i, res.Header.Get("Age"))
		}
	}
	if hits := atomic.LoadInt64(&caseHits); hits != 2 {
		t.Fatalf("expected 2 hits on mixed case unbuffered feed, got %d", hits)
	}

	// Test whitespace variation: "  no  "
	var wsHits int64
	r.GET("/unbuffered-whitespace", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&wsHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		res.Header.Set("X-Accel-Buffering", "  no  ")
		_, _ = res.WriteString("live feed whitespace")
	})

	for i := 0; i < 2; i++ {
		req, _ := httpparser.NewRequest("GET", "/unbuffered-whitespace", "HTTP/1.1")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("whitespace variation %d: expected X-Cache MISS, got %q", i, res.Header.Get("X-Cache"))
		}
		if res.Header.Get("Age") != "" {
			t.Fatalf("whitespace variation %d: expected empty Age header, got %q", i, res.Header.Get("Age"))
		}
	}
	if hits := atomic.LoadInt64(&wsHits); hits != 2 {
		t.Fatalf("expected 2 hits on whitespace unbuffered feed, got %d", hits)
	}

	if store.Len() != 0 {
		t.Fatalf("expected 0 cached entries for unbuffered responses, got %d", store.Len())
	}
}

// TC-128.5: Standard Cacheable Response Regression Protection
func TestCache_StandardResponses_StillCached(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var jsonHits int64
	r.GET("/api/standard-cached", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&jsonHits, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Cache-Control", "public, max-age=60")
		_, _ = res.WriteString(`{"cached": true}`)
	})

	req1, _ := httpparser.NewRequest("GET", "/api/standard-cached", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)
	if res1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("expected first request X-Cache MISS, got %q", res1.Header.Get("X-Cache"))
	}
	if hits := atomic.LoadInt64(&jsonHits); hits != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}

	req2, _ := httpparser.NewRequest("GET", "/api/standard-cached", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)
	if res2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("expected second request X-Cache HIT, got %q", res2.Header.Get("X-Cache"))
	}
	if res2.Header.Get("Age") == "" {
		t.Fatalf("expected Age header on second request (cache HIT)")
	}
	if hits := atomic.LoadInt64(&jsonHits); hits != 1 {
		t.Fatalf("expected handler not called on second request, got %d hits", hits)
	}

	if store.Len() != 1 {
		t.Fatalf("expected exactly 1 cached entry, got %d", store.Len())
	}
}

// TC-128.13: Zero Dynamic Heap Allocation for Header Inspection Guards
func TestCache_ZeroAllocations_HeaderGuards(t *testing.T) {
	res := httpparser.NewResponse()
	res.Header.Set("Content-Type", "text/event-stream; charset=utf-8")
	res.Header.Set("X-Accel-Buffering", "no")

	allocs := testing.AllocsPerRun(1000, func() {
		ct := strings.ToLower(res.Header.Get("Content-Type"))
		_ = strings.HasPrefix(ct, "text/event-stream")
		_ = strings.EqualFold(strings.TrimSpace(res.Header.Get("X-Accel-Buffering")), "no")
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocs for header inspection guards, got %f", allocs)
	}
}

// TestRouter_HeaderInspectionGuards_ZeroAllocation is an alias verifying TC-128.13 per spec.
func TestRouter_HeaderInspectionGuards_ZeroAllocation(t *testing.T) {
	TestCache_ZeroAllocations_HeaderGuards(t)
}
