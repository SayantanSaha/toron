package router

import (
	"fmt"
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
