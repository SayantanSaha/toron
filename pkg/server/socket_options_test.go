package server_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/server"
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

// TC-127.1: Underlying Socket Extraction & Unwrapping (Server Level)
func TestServer_ExtractTCPConn_UnwrappingPermutations(t *testing.T) {
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
		tcpConn := server.ExtractTCPConn(rawConn)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn for raw accepted TCP connection")
		}
		if expected, ok := rawConn.(*net.TCPConn); !ok || tcpConn != expected {
			t.Fatalf("extracted tcpConn pointer mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1B (TLS NetConn Unwrapping)", func(t *testing.T) {
		tlsWrapped := &mockTLSConn{Conn: rawConn}
		tcpConn := server.ExtractTCPConn(tlsWrapped)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn through mockTLSConn")
		}
		if expected := rawConn.(*net.TCPConn); tcpConn != expected {
			t.Fatalf("extracted tcpConn mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1C (Deadline Tracker Wrapper)", func(t *testing.T) {
		tracker := server.NewConnDeadlineTracker(rawConn)
		tcpConn := server.ExtractTCPConn(tracker)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn through ConnDeadlineTracker")
		}
		if expected := rawConn.(*net.TCPConn); tcpConn != expected {
			t.Fatalf("extracted tcpConn mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1D (HTTP/2 Prefix Connection Wrapper)", func(t *testing.T) {
		pConn := server.NewPrefixConn(rawConn, []byte("PRI"))
		tcpConn := server.ExtractTCPConn(pConn)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn through prefixConn")
		}
		if expected := rawConn.(*net.TCPConn); tcpConn != expected {
			t.Fatalf("extracted tcpConn mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1E (Multi-Level Nested Wrappers: 4 Tiers)", func(t *testing.T) {
		// tracker -> prefixConn -> tlsConn -> rawConn
		tlsConn := &mockTLSConn{Conn: rawConn}
		pConn := server.NewPrefixConn(tlsConn, []byte("PRI"))
		tracker := server.NewConnDeadlineTracker(pConn)

		tcpConn := server.ExtractTCPConn(tracker)
		if tcpConn == nil {
			t.Fatal("expected non-nil *net.TCPConn through 4-tier wrapper")
		}
		if expected := rawConn.(*net.TCPConn); tcpConn != expected {
			t.Fatalf("extracted tcpConn mismatch: got %p, want %p", tcpConn, expected)
		}
	})

	t.Run("Subtest 1F (Non-TCP and Edge Cases)", func(t *testing.T) {
		p1, p2 := net.Pipe()
		defer p1.Close()
		defer p2.Close()

		if tcpConn := server.ExtractTCPConn(p1); tcpConn != nil {
			t.Fatalf("expected nil for net.Pipe, got %v", tcpConn)
		}

		if tcpConn := server.ExtractTCPConn(nil); tcpConn != nil {
			t.Fatalf("expected nil for nil conn, got %v", tcpConn)
		}

		op := &opaqueConn{Conn: p1}
		if tcpConn := server.ExtractTCPConn(op); tcpConn != nil {
			t.Fatalf("expected nil for opaque non-TCP conn, got %v", tcpConn)
		}

		circA := &circularConnA{Conn: p1}
		circB := &circularConnB{Conn: p2, a: circA}
		circA.b = circB

		if tcpConn := server.ExtractTCPConn(circA); tcpConn != nil {
			t.Fatalf("expected nil for circular wrapper, got %v", tcpConn)
		}
	})
}

// TC-127.4: Server Connection Handler Safeguard
func TestServer_HandleConn_SafeguardIdempotence(t *testing.T) {
	r := router.New()
	var handlerCalled atomic.Bool
	var tcpExtracted atomic.Bool

	r.GET("/safeguard", func(req *httpparser.Request, res *httpparser.Response) {
		handlerCalled.Store(true)
		tcpConn := server.ExtractTCPConn(req.RawConn)
		if tcpConn != nil {
			tcpExtracted.Store(true)
		}
		// Idempotently configure socket again
		_ = server.ConfigureTCPSocket(req.RawConn)

		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"safeguard":"ok"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	reqStr := "GET /safeguard HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := client.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	reader := bufio.NewReader(client)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if !handlerCalled.Load() {
		t.Fatal("handler was not invoked")
	}
	if !tcpExtracted.Load() {
		t.Fatal("underlying TCPConn was not extracted in handler")
	}
}

// TC-127.5: Immediate Small Frame Wire Delivery (SSE Nagle Elimination)
func TestServer_SSE_ImmediateWireDelivery_NoNagleDelay(t *testing.T) {
	r := router.New()
	r.GET("/sse-fast", func(req *httpparser.Request, res *httpparser.Response) {
		pr, pw := io.Pipe()
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")
		res.Header.Set("Cache-Control", "no-cache")
		res.Header.Set("Connection", "keep-alive")
		res.StreamBody = pr

		go func() {
			defer pw.Close()
			for seq := 1; seq <= 5; seq++ {
				frame := fmt.Sprintf("data: {\"seq\":%d}\n\n", seq)
				if _, err := pw.Write([]byte(frame)); err != nil {
					return
				}
				if seq < 5 {
					time.Sleep(2 * time.Millisecond)
				}
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	reqStr := "GET /sse-fast HTTP/1.1\r\nHost: localhost\r\n\r\n"
	if _, err := client.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	reader := bufio.NewReader(client)

	// Read response headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read header: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	timestamps := make([]time.Time, 5)

	// Read 5 discrete SSE frames
	for seq := 1; seq <= 5; seq++ {
		dataLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("frame %d: read data line failed: %v", seq, err)
		}
		emptyLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("frame %d: read empty line failed: %v", seq, err)
		}

		now := time.Now()
		timestamps[seq-1] = now

		expectedData := fmt.Sprintf("data: {\"seq\":%d}\n", seq)
		if dataLine != expectedData {
			t.Fatalf("frame %d: unexpected data line: got %q, want %q", seq, dataLine, expectedData)
		}
		if emptyLine != "\n" {
			t.Fatalf("frame %d: expected empty line separator, got %q", seq, emptyLine)
		}
	}

	// Verify inter-frame arrival deltas
	for i := 1; i < 5; i++ {
		delta := timestamps[i].Sub(timestamps[i-1])
		t.Logf("Inter-frame arrival delta %d -> %d: %v", i, i+1, delta)
		// With TCP_NODELAY, inter-frame delivery is immediate (< 15ms).
		// Under Nagle's algorithm, this would freeze for 40ms-200ms per frame.
		if delta > 15*time.Millisecond {
			t.Fatalf("delta %d -> %d (%v) exceeded 15ms threshold; Nagle buffering detected", i, i+1, delta)
		}
	}

	totalDuration := timestamps[4].Sub(timestamps[0])
	t.Logf("Total SSE 5-frame transmission duration: %v", totalDuration)
	if totalDuration > 35*time.Millisecond {
		t.Fatalf("total duration (%v) exceeded 35ms threshold", totalDuration)
	}
}

// TC-127.6: Half-Open Socket Detection via Keep-Alive Probes
func TestServer_TCPKeepAlive_Verification(t *testing.T) {
	r := router.New()
	var keepAliveOk atomic.Bool

	r.GET("/keepalive-check", func(req *httpparser.Request, res *httpparser.Response) {
		tcpConn := server.ExtractTCPConn(req.RawConn)
		if tcpConn != nil {
			// Verify setting keep alive succeeds without error
			err1 := tcpConn.SetKeepAlive(true)
			err2 := tcpConn.SetKeepAlivePeriod(60 * time.Second)
			if err1 == nil && err2 == nil {
				keepAliveOk.Store(true)
			}
		}
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"keepalive":"ok"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	reqStr := "GET /keepalive-check HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := client.Write([]byte(reqStr)); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	reader := bufio.NewReader(client)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if !keepAliveOk.Load() {
		t.Fatal("TCP Keep-Alive was not successfully configured on accepted connection")
	}
}

// TC-127.10: Concurrency, Thread-Safety & Race Cleanliness
func TestServer_SocketOptions_ConcurrentStress(t *testing.T) {
	r := router.New()
	var successCount atomic.Int64

	r.GET("/stress", func(req *httpparser.Request, res *httpparser.Response) {
		tcpConn := server.ExtractTCPConn(req.RawConn)
		if tcpConn != nil {
			_ = server.ConfigureTCPSocket(req.RawConn)
			successCount.Add(1)
		}
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"ok"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.WorkerPoolSize = 32

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	addr := ln.Addr().String()
	const clients = 50
	var wg sync.WaitGroup
	wg.Add(clients)

	for i := 0; i < clients; i++ {
		go func(id int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				t.Errorf("client %d dial error: %v", id, err)
				return
			}
			defer conn.Close()

			reqStr := "GET /stress HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
			if _, err := conn.Write([]byte(reqStr)); err != nil {
				t.Errorf("client %d write error: %v", id, err)
				return
			}

			reader := bufio.NewReader(conn)
			resp, err := http.ReadResponse(reader, nil)
			if err != nil {
				t.Errorf("client %d read error: %v", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("client %d expected status 200, got %d", id, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()

	if successCount.Load() != clients {
		t.Fatalf("expected %d successfully configured connections, got %d", clients, successCount.Load())
	}
}
