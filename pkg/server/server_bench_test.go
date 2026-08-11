package server_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/server"
)

func BenchmarkServer_EndToEnd(b *testing.B) {
	r := router.New()
	r.GET("/bench", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("OK")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"

	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		b.Fatalf("failed to listen: %v", err)
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	addr := ln.Addr().String()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			b.Fatalf("dial failed: %v", err)
		}

		reqStr := "GET /bench HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(reqStr))
		_, _ = io.ReadAll(conn)
		_ = conn.Close()
	}

	b.StopTimer()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
