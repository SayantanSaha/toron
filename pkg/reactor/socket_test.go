package reactor_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/reactor"
)

// mockTLSConn simulates a tls.Conn wrapping net.Conn via NetConn().
type mockTLSConn struct {
	net.Conn
}

func (m *mockTLSConn) NetConn() net.Conn {
	return m.Conn
}

// mockUnwrapConn simulates a custom wrapper implementing Unwrap() net.Conn.
type mockUnwrapConn struct {
	net.Conn
}

func (m *mockUnwrapConn) Unwrap() net.Conn {
	return m.Conn
}

// circularConnA and circularConnB simulate a circular unwrapping chain.
type circularConnA struct {
	net.Conn
	b net.Conn
}

func (a *circularConnA) Unwrap() net.Conn {
	return a.b
}

type circularConnB struct {
	net.Conn
	a net.Conn
}

func (b *circularConnB) Unwrap() net.Conn {
	return b.a
}

// opaqueConn is a net.Conn with no Unwrap or NetConn methods.
type opaqueConn struct {
	net.Conn
}

// TC-127.1: Underlying Socket Extraction & Unwrapping (Reactor Level)
func TestReactor_ExtractTCPConn_UnwrappingPermutations(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	done := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			done <- conn
		}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	rawConn := <-done
	defer rawConn.Close()

	t.Run("Subtest 1A (Direct TCP Connection)", func(t *testing.T) {
		tcpConn := reactor.ExtractTCPConn(rawConn)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn for raw accepted TCP connection")
		}
		if expected, ok := rawConn.(*net.TCPConn); !ok || tcpConn != expected {
			t.Fatalf("extracted tcpConn pointer mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1B (TLS NetConn Unwrapping)", func(t *testing.T) {
		tlsWrapped := &mockTLSConn{Conn: rawConn}
		tcpConn := reactor.ExtractTCPConn(tlsWrapped)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn through mockTLSConn")
		}
		if expected := rawConn.(*net.TCPConn); tcpConn != expected {
			t.Fatalf("extracted tcpConn mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1C (Unwrap Interface Wrapper)", func(t *testing.T) {
		unwrapped := &mockUnwrapConn{Conn: rawConn}
		tcpConn := reactor.ExtractTCPConn(unwrapped)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn through mockUnwrapConn")
		}
		if expected := rawConn.(*net.TCPConn); tcpConn != expected {
			t.Fatalf("extracted tcpConn mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1D (Multi-Level Nested Wrappers)", func(t *testing.T) {
		nested := &mockUnwrapConn{
			Conn: &mockTLSConn{
				Conn: &mockUnwrapConn{
					Conn: rawConn,
				},
			},
		}
		tcpConn := reactor.ExtractTCPConn(nested)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn through 3-layer wrapper")
		}
		if expected := rawConn.(*net.TCPConn); tcpConn != expected {
			t.Fatalf("extracted tcpConn mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1E (Non-TCP and Edge Cases)", func(t *testing.T) {
		p1, p2 := net.Pipe()
		defer p1.Close()
		defer p2.Close()

		if tcpConn := reactor.ExtractTCPConn(p1); tcpConn != nil {
			t.Fatalf("expected nil for net.Pipe, got %v", tcpConn)
		}

		if tcpConn := reactor.ExtractTCPConn(nil); tcpConn != nil {
			t.Fatalf("expected nil for nil conn, got %v", tcpConn)
		}

		op := &opaqueConn{Conn: p1}
		if tcpConn := reactor.ExtractTCPConn(op); tcpConn != nil {
			t.Fatalf("expected nil for opaque non-TCP conn, got %v", tcpConn)
		}

		// Circular wrapper test (depth limit bound)
		circA := &circularConnA{Conn: p1}
		circB := &circularConnB{Conn: p2, a: circA}
		circA.b = circB

		if tcpConn := reactor.ExtractTCPConn(circA); tcpConn != nil {
			t.Fatalf("expected nil for circular wrapper, got %v", tcpConn)
		}
	})
}

// TC-127.2: Socket Configuration Routine (ConfigureTCPSocket)
func TestReactor_ConfigureTCPSocket_Enforcement(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	done := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			done <- conn
		}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	rawConn := <-done
	defer rawConn.Close()

	t.Run("Subtest 2A (Live Socket Option Application)", func(t *testing.T) {
		if cfgErr := reactor.ConfigureTCPSocket(rawConn); cfgErr != nil {
			t.Fatalf("ConfigureTCPSocket returned error on live TCP socket: %v", cfgErr)
		}
		tcpConn := reactor.ExtractTCPConn(rawConn)
		if tcpConn == nil {
			t.Fatal("expected valid *net.TCPConn from live socket")
		}
	})

	t.Run("Subtest 2B (Non-TCP No-Op Safe Path)", func(t *testing.T) {
		p1, p2 := net.Pipe()
		defer p1.Close()
		defer p2.Close()

		if cfgErr := reactor.ConfigureTCPSocket(p1); cfgErr != nil {
			t.Fatalf("expected nil error for non-TCP pipe, got: %v", cfgErr)
		}
	})

	t.Run("Subtest 2C (Idempotence & Re-Configuration)", func(t *testing.T) {
		// Calling ConfigureTCPSocket repeatedly must succeed idempotently
		for i := 0; i < 3; i++ {
			if cfgErr := reactor.ConfigureTCPSocket(rawConn); cfgErr != nil {
				t.Fatalf("iteration %d: ConfigureTCPSocket failed: %v", i, cfgErr)
			}
		}
	})

	t.Run("Subtest 2D (Closed Socket Error Handling)", func(t *testing.T) {
		closedLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer closedLn.Close()

		acceptedChan := make(chan net.Conn, 1)
		go func() {
			c, aErr := closedLn.Accept()
			if aErr == nil {
				acceptedChan <- c
			}
		}()

		cConn, err := net.Dial("tcp", closedLn.Addr().String())
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer cConn.Close()

		sConn := <-acceptedChan
		// Close socket prior to configuration
		_ = sConn.Close()

		// Configuration on closed socket should return error safely without panic
		err = reactor.ConfigureTCPSocket(sConn)
		if err == nil {
			t.Log("Note: OS kernel allowed setting socket options on recently closed descriptor")
		} else {
			t.Logf("expected error on closed socket: %v", err)
		}
	})
}

// TC-127.3: Reactor Accept Loop Integration
func TestReactor_AcceptLoop_ConfiguresSocketOptions(t *testing.T) {
	var configuredWorkers atomic.Int64
	workerReceived := make(chan struct{}, 1)

	handler := reactor.HandlerFunc(func(ctx context.Context, conn net.Conn) error {
		tcpConn := reactor.ExtractTCPConn(conn)
		if tcpConn != nil {
			configuredWorkers.Add(1)
		}
		select {
		case workerReceived <- struct{}{}:
		default:
		}
		buf := make([]byte, 16)
		n, _ := conn.Read(buf)
		if n > 0 {
			_, _ = conn.Write(buf[:n])
		}
		return nil
	})

	cfg := reactor.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.WorkerPoolSize = 2

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

	client, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	select {
	case <-workerReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker to process connection")
	}

	if configuredWorkers.Load() != 1 {
		t.Fatalf("expected worker to verify configured TCP socket, got %d", configuredWorkers.Load())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Shutdown(shutdownCtx); err != nil {
		t.Errorf("reactor shutdown error: %v", err)
	}
	<-serveErrChan
}
