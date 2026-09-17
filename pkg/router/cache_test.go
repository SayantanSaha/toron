package router

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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

// TC-134.1: Cross-Port Cache Isolation (service.local:8080 vs service.local:9090)
func TestCache_HostPortIsolation(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var hitCount int64

	r.GET("/api/isolated", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&hitCount, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		res.Header.Set("Cache-Control", "max-age=60")
		_, _ = res.WriteString("response-from-" + req.Header.Get("Host"))
	})

	// 1. Dispatch Request 1 (Host: service.local:8080) -> Cache MISS
	req1, _ := httpparser.NewRequest("GET", "/api/isolated", "HTTP/1.1")
	req1.Header.Set("Host", "service.local:8080")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("req1: expected status 200, got %d", res1.StatusCode)
	}
	if xCache := res1.Header.Get("X-Cache"); xCache != "MISS" {
		t.Fatalf("req1: expected X-Cache MISS, got %q", xCache)
	}
	if age := res1.Header.Get("Age"); age != "" {
		t.Fatalf("req1: expected empty Age header, got %q", age)
	}
	if calls := atomic.LoadInt64(&hitCount); calls != 1 {
		t.Fatalf("req1: expected 1 handler call, got %d", calls)
	}
	if body := res1.Body.String(); body != "response-from-service.local:8080" {
		t.Fatalf("req1: expected body %q, got %q", "response-from-service.local:8080", body)
	}

	// 2. Dispatch Request 2 (Host: service.local:9090) -> CRITICAL: Cache MISS (must NOT hit port 8080 cache!)
	req2, _ := httpparser.NewRequest("GET", "/api/isolated", "HTTP/1.1")
	req2.Header.Set("Host", "service.local:9090")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusOK {
		t.Fatalf("req2: expected status 200, got %d", res2.StatusCode)
	}
	if xCache := res2.Header.Get("X-Cache"); xCache != "MISS" {
		t.Fatalf("req2: expected X-Cache MISS for distinct port, got %q", xCache)
	}
	if age := res2.Header.Get("Age"); age != "" {
		t.Fatalf("req2: expected empty Age header, got %q", age)
	}
	if calls := atomic.LoadInt64(&hitCount); calls != 2 {
		t.Fatalf("req2: expected 2 handler calls, got %d", calls)
	}
	if body := res2.Body.String(); body != "response-from-service.local:9090" {
		t.Fatalf("req2: expected body %q, got %q", "response-from-service.local:9090", body)
	}

	// 3. Dispatch Request 3 (Host: service.local:8080) -> Cache HIT
	req3, _ := httpparser.NewRequest("GET", "/api/isolated", "HTTP/1.1")
	req3.Header.Set("Host", "service.local:8080")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)

	if res3.StatusCode != http.StatusOK {
		t.Fatalf("req3: expected status 200, got %d", res3.StatusCode)
	}
	if xCache := res3.Header.Get("X-Cache"); xCache != "HIT" {
		t.Fatalf("req3: expected X-Cache HIT, got %q", xCache)
	}
	if age := res3.Header.Get("Age"); age == "" {
		t.Fatalf("req3: expected Age header on cache HIT")
	}
	if calls := atomic.LoadInt64(&hitCount); calls != 2 {
		t.Fatalf("req3: handler should not be called on cache HIT, got %d", calls)
	}
	if body := res3.Body.String(); body != "response-from-service.local:8080" {
		t.Fatalf("req3: expected body %q, got %q", "response-from-service.local:8080", body)
	}

	// 4. Dispatch Request 4 (Host: service.local:9090) -> Cache HIT
	req4, _ := httpparser.NewRequest("GET", "/api/isolated", "HTTP/1.1")
	req4.Header.Set("Host", "service.local:9090")
	res4 := httpparser.NewResponse()
	r.ServeHTTP(req4, res4)

	if res4.StatusCode != http.StatusOK {
		t.Fatalf("req4: expected status 200, got %d", res4.StatusCode)
	}
	if xCache := res4.Header.Get("X-Cache"); xCache != "HIT" {
		t.Fatalf("req4: expected X-Cache HIT, got %q", xCache)
	}
	if age := res4.Header.Get("Age"); age == "" {
		t.Fatalf("req4: expected Age header on cache HIT")
	}
	if calls := atomic.LoadInt64(&hitCount); calls != 2 {
		t.Fatalf("req4: handler should not be called on cache HIT, got %d", calls)
	}
	if body := res4.Body.String(); body != "response-from-service.local:9090" {
		t.Fatalf("req4: expected body %q, got %q", "response-from-service.local:9090", body)
	}

	// 5. Assert cache store contains exactly 2 isolated entries
	if count := store.Len(); count != 2 {
		t.Fatalf("expected 2 distinct entries in cache store, got %d", count)
	}
}

// TC-134.2: Host Authority Derivation Unit Vectors for extractCacheHostPort
func TestCache_HostPortKeyDerivation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		isNilReq bool
		expected string
	}{
		{"V1 standard host without port", "example.com", false, "example.com"},
		{"V2 uppercase host without port", "EXAMPLE.COM", false, "example.com"},
		{"V3 standard host with port", "example.com:8080", false, "example.com:8080"},
		{"V4 uppercase host with port", "EXAMPLE.COM:8080", false, "example.com:8080"},
		{"V5 whitespace around host and port", "   api.toron.local:9090   ", false, "api.toron.local:9090"},
		{"V6 whitespace between hostname and colon/port", "  api.toron.local : 9090  ", false, "api.toron.local:9090"},
		{"V7 IPv6 localhost with port", "[::1]:8080", false, "[::1]:8080"},
		{"V8 IPv6 localhost without port", "[::1]", false, "[::1]"},
		{"V9 full uppercase IPv6 address with port", "[2001:0DB8::1]:8443", false, "[2001:0db8::1]:8443"},
		{"V10 full uppercase IPv6 address without port", "[2001:0DB8::1]", false, "[2001:0db8::1]"},
		{"V11 IPv4 address with port", "127.0.0.1:9000", false, "127.0.0.1:9000"},
		{"V12 IPv4 address without port", "127.0.0.1", false, "127.0.0.1"},
		{"V13 empty host header", "", false, ""},
		{"V14 missing host header (nil request)", "", true, ""},
		{"V15 whitespace-only host header", "    ", false, ""},
		{"V16 trailing colon with empty port", "example.com:", false, "example.com"},
		{"V17 malformed IPv6 (unclosed bracket)", "[::1:8080", false, "[::1:8080"},
		{"V18 custom sidecar high port", "mesh.internal:65535", false, "mesh.internal:65535"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *httpparser.Request
			if !tc.isNilReq {
				var err error
				req, err = httpparser.NewRequest("GET", "/test", "HTTP/1.1")
				if err != nil {
					t.Fatalf("failed to create request: %v", err)
				}
				req.Header.Set("Host", tc.input)
			}
			actual := extractCacheHostPort(req)
			if actual != tc.expected {
				t.Fatalf("for input %q: expected %q, got %q", tc.input, tc.expected, actual)
			}
		})
	}
}

// TC-134.3: IPv6 Cross-Port Isolation ([::1]:8080 vs [::1]:8443 vs [::1])
func TestCache_IPv6HostPortIsolation(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	r.GET("/ipv6/endpoint", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		res.Header.Set("Cache-Control", "max-age=60")
		_, _ = res.WriteString("ipv6-response-" + req.Header.Get("Host"))
	})

	// Req A: Host: [::1]:8080
	reqA, _ := httpparser.NewRequest("GET", "/ipv6/endpoint", "HTTP/1.1")
	reqA.Header.Set("Host", "[::1]:8080")
	resA := httpparser.NewResponse()
	r.ServeHTTP(reqA, resA)
	if resA.Header.Get("X-Cache") != "MISS" || resA.Body.String() != "ipv6-response-[::1]:8080" {
		t.Fatalf("reqA MISS expected, got X-Cache=%q, body=%q", resA.Header.Get("X-Cache"), resA.Body.String())
	}

	// Req B: Host: [::1]:8443
	reqB, _ := httpparser.NewRequest("GET", "/ipv6/endpoint", "HTTP/1.1")
	reqB.Header.Set("Host", "[::1]:8443")
	resB := httpparser.NewResponse()
	r.ServeHTTP(reqB, resB)
	if resB.Header.Get("X-Cache") != "MISS" || resB.Body.String() != "ipv6-response-[::1]:8443" {
		t.Fatalf("reqB MISS expected, got X-Cache=%q, body=%q", resB.Header.Get("X-Cache"), resB.Body.String())
	}

	// Req C: Host: [::1] (no port)
	reqC, _ := httpparser.NewRequest("GET", "/ipv6/endpoint", "HTTP/1.1")
	reqC.Header.Set("Host", "[::1]")
	resC := httpparser.NewResponse()
	r.ServeHTTP(reqC, resC)
	if resC.Header.Get("X-Cache") != "MISS" || resC.Body.String() != "ipv6-response-[::1]" {
		t.Fatalf("reqC MISS expected, got X-Cache=%q, body=%q", resC.Header.Get("X-Cache"), resC.Body.String())
	}

	// Re-executions must all HIT with their respective payloads
	resA2 := httpparser.NewResponse()
	r.ServeHTTP(reqA, resA2)
	if resA2.Header.Get("X-Cache") != "HIT" || resA2.Body.String() != "ipv6-response-[::1]:8080" {
		t.Fatalf("reqA HIT expected, got X-Cache=%q, body=%q", resA2.Header.Get("X-Cache"), resA2.Body.String())
	}

	resB2 := httpparser.NewResponse()
	r.ServeHTTP(reqB, resB2)
	if resB2.Header.Get("X-Cache") != "HIT" || resB2.Body.String() != "ipv6-response-[::1]:8443" {
		t.Fatalf("reqB HIT expected, got X-Cache=%q, body=%q", resB2.Header.Get("X-Cache"), resB2.Body.String())
	}

	resC2 := httpparser.NewResponse()
	r.ServeHTTP(reqC, resC2)
	if resC2.Header.Get("X-Cache") != "HIT" || resC2.Body.String() != "ipv6-response-[::1]" {
		t.Fatalf("reqC HIT expected, got X-Cache=%q, body=%q", resC2.Header.Get("X-Cache"), resC2.Body.String())
	}

	if count := store.Len(); count != 3 {
		t.Fatalf("expected 3 distinct entries in cache store, got %d", count)
	}
}

// TC-134.4: Dual-Stage Set-Cookie / Set-Cookie2 Stripping Verification
func TestCache_DualStageSetCookieStripped(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	r.GET("/auth/session", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Cache-Control", "max-age=60")
		res.Header.Set("Set-Cookie", "session_id=SECRET987; Secure; HttpOnly")
		res.Header.Set("Set-Cookie2", "session_id2=SECRET456")
		_, _ = res.WriteString(`{"authenticated": true}`)
	})

	// 1. Client 1 makes first request (Cache MISS)
	req1, _ := httpparser.NewRequest("GET", "/auth/session", "HTTP/1.1")
	req1.Header.Set("Host", "example.com")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("req1: expected status 200, got %d", res1.StatusCode)
	}
	if res1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("req1: expected X-Cache MISS, got %q", res1.Header.Get("X-Cache"))
	}
	if res1.Header.Get("Set-Cookie") == "" {
		t.Fatalf("req1: expected initial response to have Set-Cookie header")
	}

	// 2. Stage 1 Purge: Inspect the cached snapshot in store
	cached, found := store.Get("GET:example.com:/auth/session", time.Now())
	if !found {
		t.Fatalf("expected cached entry in store")
	}
	if cookie := cached.Header.Get("Set-Cookie"); cookie != "" {
		t.Fatalf("CRITICAL Stage 1 Purge Failure: Set-Cookie leaked into cached snapshot: %q", cookie)
	}
	if cookie2 := cached.Header.Get("Set-Cookie2"); cookie2 != "" {
		t.Fatalf("CRITICAL Stage 1 Purge Failure: Set-Cookie2 leaked into cached snapshot: %q", cookie2)
	}

	// 3. Stage 2 Purge: Client 2 makes subsequent request (Cache HIT)
	req2, _ := httpparser.NewRequest("GET", "/auth/session", "HTTP/1.1")
	req2.Header.Set("Host", "example.com")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusOK {
		t.Fatalf("req2: expected status 200, got %d", res2.StatusCode)
	}
	if res2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("req2: expected X-Cache HIT, got %q", res2.Header.Get("X-Cache"))
	}
	if cookie := res2.Header.Get("Set-Cookie"); cookie != "" {
		t.Fatalf("CRITICAL Stage 2 Purge Failure: Set-Cookie leaked on cache HIT: %q", cookie)
	}
	if cookie2 := res2.Header.Get("Set-Cookie2"); cookie2 != "" {
		t.Fatalf("CRITICAL Stage 2 Purge Failure: Set-Cookie2 leaked on cache HIT: %q", cookie2)
	}

	// 4. Direct Poisoning Injection Test: Manually inject a poisoned CachedResponse containing Set-Cookie
	poisonedHeader := make(httpparser.Header)
	poisonedHeader["Set-Cookie"] = []string{"rogue=leak"}
	poisonedHeader["Set-Cookie2"] = []string{"rogue2=leak"}
	poisonedHeader["Content-Type"] = []string{"application/json"}
	now := time.Now()
	store.Set("GET:example.com:/auth/poisoned", &CachedResponse{
		StatusCode: http.StatusOK,
		Header:     poisonedHeader,
		Body:       []byte(`{"poisoned": true}`),
		CachedAt:   now,
		ExpiresAt:  now.Add(60 * time.Second),
		Public:     true,
	})

	reqPoison, _ := httpparser.NewRequest("GET", "/auth/poisoned", "HTTP/1.1")
	reqPoison.Header.Set("Host", "example.com")
	resPoison := httpparser.NewResponse()
	r.ServeHTTP(reqPoison, resPoison)

	if resPoison.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("poisoned req: expected HIT, got %q", resPoison.Header.Get("X-Cache"))
	}
	if leak := resPoison.Header.Get("Set-Cookie"); leak != "" {
		t.Fatalf("CRITICAL: Delivery-stage purge failed to strip poisoned Set-Cookie: %q", leak)
	}
	if leak2 := resPoison.Header.Get("Set-Cookie2"); leak2 != "" {
		t.Fatalf("CRITICAL: Delivery-stage purge failed to strip poisoned Set-Cookie2: %q", leak2)
	}
}

// TC-134.5: RFC 9111 Authorization Refusal & Shared Cache Public Exception
func TestCache_RFC9111_AuthorizationBoundary(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var privateCalls int64
	r.GET("/api/private-data", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&privateCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Cache-Control", "max-age=60")
		_, _ = res.WriteString(`{"data": "secret"}`)
	})

	var publicCalls int64
	r.GET("/api/public-catalog", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&publicCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Cache-Control", "public, max-age=60")
		_, _ = res.WriteString(`{"catalog": "items"}`)
	})

	var unauthCalls int64
	r.GET("/api/unauth-resource", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&unauthCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Cache-Control", "max-age=60")
		_, _ = res.WriteString(`{"unauth": "data"}`)
	})

	// Case A: Authenticated request without public directive
	reqA1, _ := httpparser.NewRequest("GET", "/api/private-data", "HTTP/1.1")
	reqA1.Header.Set("Host", "example.com")
	reqA1.Header.Set("Authorization", "Bearer token-alice")
	resA1 := httpparser.NewResponse()
	r.ServeHTTP(reqA1, resA1)
	if resA1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("case A1: expected X-Cache MISS, got %q", resA1.Header.Get("X-Cache"))
	}
	if store.Len() != 0 {
		t.Fatalf("case A1: response without public must NOT be cached, store.Len() = %d", store.Len())
	}

	reqA2, _ := httpparser.NewRequest("GET", "/api/private-data", "HTTP/1.1")
	reqA2.Header.Set("Host", "example.com")
	reqA2.Header.Set("Authorization", "Bearer token-alice")
	resA2 := httpparser.NewResponse()
	r.ServeHTTP(reqA2, resA2)
	if resA2.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("case A2: expected X-Cache MISS, got %q", resA2.Header.Get("X-Cache"))
	}
	if calls := atomic.LoadInt64(&privateCalls); calls != 2 {
		t.Fatalf("case A2: expected handler called twice, got %d", calls)
	}

	// Case B: Authenticated request with explicit public directive (RFC 9111 §3.5 exception)
	reqB1, _ := httpparser.NewRequest("GET", "/api/public-catalog", "HTTP/1.1")
	reqB1.Header.Set("Host", "example.com")
	reqB1.Header.Set("Authorization", "Bearer token-bob")
	resB1 := httpparser.NewResponse()
	r.ServeHTTP(reqB1, resB1)
	if resB1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("case B1: expected X-Cache MISS, got %q", resB1.Header.Get("X-Cache"))
	}
	if store.Len() != 1 {
		t.Fatalf("case B1: public response must be stored in cache, store.Len() = %d", store.Len())
	}

	reqB2, _ := httpparser.NewRequest("GET", "/api/public-catalog", "HTTP/1.1")
	reqB2.Header.Set("Host", "example.com")
	reqB2.Header.Set("Authorization", "Bearer token-bob")
	resB2 := httpparser.NewResponse()
	r.ServeHTTP(reqB2, resB2)
	if resB2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("case B2: expected X-Cache HIT for public cached resource, got %q", resB2.Header.Get("X-Cache"))
	}
	if calls := atomic.LoadInt64(&publicCalls); calls != 1 {
		t.Fatalf("case B2: expected handler called only once on cache HIT, got %d", calls)
	}

	// Case C: Authenticated request querying pre-cached non-public resource
	reqC1, _ := httpparser.NewRequest("GET", "/api/unauth-resource", "HTTP/1.1")
	reqC1.Header.Set("Host", "example.com")
	resC1 := httpparser.NewResponse()
	r.ServeHTTP(reqC1, resC1)
	if resC1.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("case C1: expected MISS, got %q", resC1.Header.Get("X-Cache"))
	}
	if calls := atomic.LoadInt64(&unauthCalls); calls != 1 {
		t.Fatalf("case C1: expected 1 unauth call, got %d", calls)
	}

	// Client with Authorization arrives for that unauth-cached resource
	reqC2, _ := httpparser.NewRequest("GET", "/api/unauth-resource", "HTTP/1.1")
	reqC2.Header.Set("Host", "example.com")
	reqC2.Header.Set("Authorization", "Bearer token-charlie")
	resC2 := httpparser.NewResponse()
	r.ServeHTTP(reqC2, resC2)
	if resC2.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("case C2: request with Authorization must refuse non-public cached entry, got %q", resC2.Header.Get("X-Cache"))
	}
	if calls := atomic.LoadInt64(&unauthCalls); calls != 2 {
		t.Fatalf("case C2: handler must be invoked when cached entry refused, got %d calls", calls)
	}
}

// TC-134.6: RFC 9111 Origin Directives Enforcement (private, no-store, no-cache)
func TestCache_RFC9111_OriginDirectivesEnforcement(t *testing.T) {
	directives := []string{
		"private",
		"no-store",
		"no-cache",
		"no-cache, max-age=3600",
		"private, no-store, max-age=86400",
	}

	for _, ccVal := range directives {
		t.Run(ccVal, func(t *testing.T) {
			r := New()
			cfg := DefaultCacheConfig()
			cfg.DefaultTTL = 60 * time.Second
			store := NewResponseCache(cfg)
			r.Use(NewCacheMiddlewareWithStore(cfg, store))

			var calls int64
			r.GET("/directive-test", func(req *httpparser.Request, res *httpparser.Response) {
				atomic.AddInt64(&calls, 1)
				res.SetStatus(http.StatusOK)
				res.Header.Set("Cache-Control", ccVal)
				_, _ = res.WriteString("directive-payload")
			})

			req1, _ := httpparser.NewRequest("GET", "/directive-test", "HTTP/1.1")
			req1.Header.Set("Host", "example.com")
			res1 := httpparser.NewResponse()
			r.ServeHTTP(req1, res1)
			if res1.Header.Get("X-Cache") != "MISS" {
				t.Fatalf("req1: expected MISS, got %q", res1.Header.Get("X-Cache"))
			}
			if res1.Header.Get("Age") != "" {
				t.Fatalf("req1: expected empty Age header")
			}
			if store.Len() != 0 {
				t.Fatalf("response with Cache-Control: %q must NOT be cached, store.Len() = %d", ccVal, store.Len())
			}

			req2, _ := httpparser.NewRequest("GET", "/directive-test", "HTTP/1.1")
			req2.Header.Set("Host", "example.com")
			res2 := httpparser.NewResponse()
			r.ServeHTTP(req2, res2)
			if res2.Header.Get("X-Cache") != "MISS" {
				t.Fatalf("req2: expected MISS, got %q", res2.Header.Get("X-Cache"))
			}
			if calls := atomic.LoadInt64(&calls); calls != 2 {
				t.Fatalf("expected handler invoked on second request for non-cacheable directive %q, got %d calls", ccVal, calls)
			}
		})
	}
}

// TC-134.7: Streaming Cache Exemption (text/event-stream, X-Accel-Buffering: no)
func TestCache_StreamingCacheExemption(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.DefaultTTL = 60 * time.Second
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	var sseCalls int64
	r.GET("/events", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&sseCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = res.WriteString("data: live-event\n\n")
	})

	var unbufferedCalls int64
	r.GET("/unbuffered", func(req *httpparser.Request, res *httpparser.Response) {
		atomic.AddInt64(&unbufferedCalls, 1)
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		res.Header.Set("X-Accel-Buffering", "no")
		_, _ = res.WriteString("unbuffered data")
	})

	// 1. SSE Stream
	for i := 0; i < 2; i++ {
		req, _ := httpparser.NewRequest("GET", "/events", "HTTP/1.1")
		req.Header.Set("Host", "stream.local")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("SSE iter %d: expected MISS, got %q", i, res.Header.Get("X-Cache"))
		}
	}
	if calls := atomic.LoadInt64(&sseCalls); calls != 2 {
		t.Fatalf("expected 2 SSE handler calls, got %d", calls)
	}

	// 2. Unbuffered feed
	for i := 0; i < 2; i++ {
		req, _ := httpparser.NewRequest("GET", "/unbuffered", "HTTP/1.1")
		req.Header.Set("Host", "stream.local")
		res := httpparser.NewResponse()
		r.ServeHTTP(req, res)
		if res.Header.Get("X-Cache") != "MISS" {
			t.Fatalf("unbuffered iter %d: expected MISS, got %q", i, res.Header.Get("X-Cache"))
		}
	}
	if calls := atomic.LoadInt64(&unbufferedCalls); calls != 2 {
		t.Fatalf("expected 2 unbuffered handler calls, got %d", calls)
	}

	if store.Len() != 0 {
		t.Fatalf("streaming responses must never be stored in cache, store.Len() = %d", store.Len())
	}
}

// TestCache_SessionBoundaryDirectives verifies RFC 9111 session boundaries: Set-Cookie stripping, Authorization refusal without public, and private/no-store/no-cache directives.
func TestCache_SessionBoundaryDirectives(t *testing.T) {
	t.Run("SetCookieStripping", func(t *testing.T) {
		TestCache_DualStageSetCookieStripped(t)
	})
	t.Run("AuthorizationBoundary", func(t *testing.T) {
		TestCache_RFC9111_AuthorizationBoundary(t)
	})
	t.Run("OriginDirectives", func(t *testing.T) {
		TestCache_RFC9111_OriginDirectivesEnforcement(t)
	})
	t.Run("StreamingExemption", func(t *testing.T) {
		TestCache_StreamingCacheExemption(t)
	})
}

// TC-134.12: High-Concurrency Thread Safety & Race-Free Verification under go test -race
func TestCache_ConcurrentHostPortAccess_RaceClean(t *testing.T) {
	r := New()
	cfg := DefaultCacheConfig()
	cfg.MaxEntries = 500
	store := NewResponseCache(cfg)
	r.Use(NewCacheMiddlewareWithStore(cfg, store))

	targets := []string{
		"service.local:8080",
		"service.local:9090",
		"[::1]:8080",
		"[::1]:8443",
	}

	for _, host := range targets {
		h := host
		r.GETHost(h, "/data", func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			res.Header.Set("Cache-Control", "public, max-age=60")
			_, _ = res.WriteString("payload-for-" + h)
		})
	}

	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				targetHost := targets[(workerID+i)%len(targets)]
				req, _ := httpparser.NewRequest("GET", "/data", "HTTP/1.1")
				req.Header.Set("Host", targetHost)

				// Alternate with Authorization header
				if (workerID+i)%3 == 0 {
					req.Header.Set("Authorization", "Bearer concurrent-token")
				}

				res := httpparser.NewResponse()
				r.ServeHTTP(req, res)

				if res.StatusCode != http.StatusOK {
					t.Errorf("worker %d iter %d: expected status 200, got %d", workerID, i, res.StatusCode)
				}
				expectedBody := "payload-for-" + targetHost
				if body := res.Body.String(); body != expectedBody {
					t.Errorf("worker %d iter %d: expected body %q, got %q", workerID, i, expectedBody, body)
				}
			}
		}(w)
	}

	wg.Wait()
}

