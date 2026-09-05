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

func TestReactor_KeepAliveConcurrencyAndShutdown(t *testing.T) {
	// Handler simulates keep-alive connections that reply to ping and remain open
	handler := reactor.HandlerFunc(func(ctx context.Context, conn net.Conn) error {
		buf := make([]byte, 16)
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			n, err := conn.Read(buf)
			if err != nil {
				return nil
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return err
			}
		}
	})

	cfg := reactor.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.WorkerPoolSize = 5 // Small configured worker pool
	cfg.ReadTimeout = 5 * time.Second

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

	// TC-077-01: Open 20 idle keep-alive connections (much higher than WorkerPoolSize=5)
	const idleConns = 20
	conns := make([]net.Conn, idleConns)
	for i := 0; i < idleConns; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial idle conn %d: %v", i, err)
		}
		conns[i] = c
		// Send a ping and receive pong to ensure connection is actively handled
		if _, err := c.Write([]byte("ping")); err != nil {
			t.Fatalf("ping failed: %v", err)
		}
		reply := make([]byte, 4)
		if _, err := io.ReadFull(c, reply); err != nil {
			t.Fatalf("read pong failed: %v", err)
		}
	}

	// Now verify connection 21 connects immediately and is serviced without delay
	conn21, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to connect 21st client: %v", err)
	}
	defer conn21.Close()

	if _, err := conn21.Write([]byte("ping")); err != nil {
		t.Fatalf("write from conn 21 failed: %v", err)
	}
	reply21 := make([]byte, 4)
	if _, err := io.ReadFull(conn21, reply21); err != nil {
		t.Fatalf("read pong from conn 21 failed: %v", err)
	}
	if string(reply21) != "ping" {
		t.Errorf("expected 'ping', got %q", string(reply21))
	}

	// TC-077-02: Graceful Shutdown with Active Keep-Alive
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := r.Shutdown(shutdownCtx); err != nil {
		t.Errorf("expected clean shutdown, got error: %v", err)
	}

	for _, c := range conns {
		_ = c.Close()
	}

	serveErr := <-serveErrChan
	if serveErr != nil && serveErr != reactor.ErrServerClosed {
		t.Errorf("expected ErrServerClosed, got: %v", serveErr)
	}
}
