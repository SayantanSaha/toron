package reactor_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"toron/pkg/reactor"
)

func BenchmarkReactor_BufferPool(b *testing.B) {
	cfg := reactor.DefaultConfig()
	r := reactor.New(cfg, reactor.HandlerFunc(nil))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf := r.GetBuffer()
		r.PutBuffer(buf)
	}
}

func BenchmarkReactor_ConnectionDispatch(b *testing.B) {
	cfg := reactor.DefaultConfig()
	cfg.WorkerPoolSize = 64
	cfg.MaxQueueSize = 1024

	handler := reactor.HandlerFunc(func(ctx context.Context, conn net.Conn) error {
		buf := make([]byte, 8)
		n, _ := conn.Read(buf)
		_, _ = conn.Write(buf[:n])
		return nil
	})

	r := reactor.New(cfg, handler)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen failed: %v", err)
	}

	go func() {
		_ = r.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	addr := ln.Addr().String()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			b.Fatalf("dial failed: %v", err)
		}
		_, _ = conn.Write([]byte("ping"))
		buf := make([]byte, 4)
		_, _ = io.ReadFull(conn, buf)
		_ = conn.Close()
	}
}
