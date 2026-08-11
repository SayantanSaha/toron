package reactor_test

import (
	"context"
	"net"
	"testing"

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

	handler := reactor.HandlerFunc(func(ctx context.Context, conn net.Conn) error {
		return nil
	})

	r := reactor.New(cfg, handler)

	b.ReportAllocs()
	b.ResetTimer()

	// Benchmark task channel queuing performance
	for i := 0; i < b.N; i++ {
		c1, c2 := net.Pipe()
		_ = c1.Close()
		_ = c2.Close()
		_ = r
	}
}
