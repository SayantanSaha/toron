package reactor_test

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/reactor"
)

func TestReactor_LifecycleAndConcurrency(t *testing.T) {
	var handledCount atomic.Int64

	handler := reactor.HandlerFunc(func(ctx context.Context, conn net.Conn) error {
		handledCount.Add(1)
		buf := make([]byte, 128)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		_, err = conn.Write(buf[:n])
		return err
	})

	cfg := reactor.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.WorkerPoolSize = 8

	r := reactor.New(cfg, handler)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	serveErrChan := make(chan error, 1)
	go func() {
		serveErrChan <- r.Serve(ln)
	}()

	addr := ln.Addr().String()

	// Connect 5 concurrent clients
	const clientCount = 5
	for i := 0; i < clientCount; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}

		msg := []byte("hello toron")
		if _, err := conn.Write(msg); err != nil {
			t.Fatalf("write failed: %v", err)
		}

		reply := make([]byte, len(msg))
		if _, err := io.ReadFull(conn, reply); err != nil {
			t.Fatalf("read failed: %v", err)
		}

		if string(reply) != string(msg) {
			t.Errorf("expected %q, got %q", string(msg), string(reply))
		}
		_ = conn.Close()
	}

	// Verify all clients handled
	if handledCount.Load() != clientCount {
		t.Errorf("expected %d handled connections, got %d", clientCount, handledCount.Load())
	}

	// Test graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := r.Shutdown(shutdownCtx); err != nil {
		t.Errorf("shutdown returned error: %v", err)
	}

	err = <-serveErrChan
	if err != nil && err != reactor.ErrServerClosed {
		t.Errorf("serve returned unexpected error: %v", err)
	}
}

func TestReactor_BufferPool(t *testing.T) {
	cfg := reactor.DefaultConfig()
	r := reactor.New(cfg, reactor.HandlerFunc(nil))

	buf := r.GetBuffer()
	if buf == nil || len(*buf) != cfg.MaxBufferBytes {
		t.Fatalf("expected buffer length %d", cfg.MaxBufferBytes)
	}

	r.PutBuffer(buf)
}
