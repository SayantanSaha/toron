package router_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"toron/pkg/httpparser"
	"toron/pkg/router"
)

func BenchmarkRouter_MatchExact(b *testing.B) {
	r := router.New()
	r.GET("/api/v1/resource", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	req, _ := httpparser.NewRequest("GET", "/api/v1/resource", "HTTP/1.1")
	res := httpparser.NewResponse()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.ServeHTTP(req, res)
	}
}

func BenchmarkRouter_MiddlewareChain(b *testing.B) {
	r := router.New()

	mw1 := func(next router.HandlerFunc) router.HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			next(req, res)
		}
	}
	mw2 := func(next router.HandlerFunc) router.HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			next(req, res)
		}
	}

	r.Use(mw1, mw2)
	r.GET("/api/v1/chain", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	req, _ := httpparser.NewRequest("GET", "/api/v1/chain", "HTTP/1.1")
	res := httpparser.NewResponse()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.ServeHTTP(req, res)
	}
}

func BenchmarkRouter_StaticFileServing(b *testing.B) {
	tmpDir := b.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html><body>Bench</body></html>"), 0644)

	r := router.New()
	r.Static("/", tmpDir)

	req, _ := httpparser.NewRequest("GET", "/", "HTTP/1.1")
	res := httpparser.NewResponse()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res.Body.Reset()
		r.ServeHTTP(req, res)
	}
}
