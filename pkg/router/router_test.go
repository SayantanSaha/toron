package router_test

import (
	"net/http"
	"testing"

	"toron/pkg/httpparser"
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
