package proxy

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTCPProxy_Forwarding(t *testing.T) {
	// Start a mock TCP backend server
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start backend listener: %v", err)
	}
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, err := c.Read(buf)
				if err != nil && err != io.EOF {
					return
				}
				_, _ = c.Write([]byte(fmt.Sprintf("echo: %s", string(buf[:n]))))
			}(conn)
		}
	}()

	backendAddr := backendListener.Addr().String()

	// Initialize TCPProxy
	tcpProxy, err := NewTCPProxy([]string{backendAddr}, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create TCPProxy: %v", err)
	}
	defer tcpProxy.Close()

	// Start proxy listener
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		_ = tcpProxy.Serve(proxyListener)
	}()

	proxyAddr := proxyListener.Addr().String()

	// Dial proxy and send test data
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer conn.Close()

	testPayload := "hello tcp proxy"
	_, err = conn.Write([]byte(testPayload))
	if err != nil {
		t.Fatalf("failed to write to proxy: %v", err)
	}

	replyBuf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(replyBuf)
	if err != nil {
		t.Fatalf("failed to read from proxy: %v", err)
	}

	expected := "echo: hello tcp proxy"
	if string(replyBuf[:n]) != expected {
		t.Errorf("got %q, expected %q", string(replyBuf[:n]), expected)
	}
}

// TC-087-01: TCP Concurrency Gating & Over-Capacity Fast Rejection
func TestTCPProxy_MaxConnections(t *testing.T) {
	var backendConnCount int64

	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start backend listener: %v", err)
	}
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(&backendConnCount, 1)
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					_, _ = c.Write([]byte(fmt.Sprintf("echo: %s", string(buf[:n]))))
				}
			}(conn)
		}
	}()

	backendAddr := backendListener.Addr().String()

	tcpProxy, err := NewTCPProxy([]string{backendAddr}, 5*time.Second,
		WithTCPMaxConnections(2),
		WithTCPIdleTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to create TCPProxy: %v", err)
	}
	defer tcpProxy.Close()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		_ = tcpProxy.Serve(proxyListener)
	}()

	proxyAddr := proxyListener.Addr().String()

	// 1. Client dials connection 1
	conn1, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial conn1: %v", err)
	}
	defer conn1.Close()

	_, err = conn1.Write([]byte("client1-hello"))
	if err != nil {
		t.Fatalf("conn1 write failed: %v", err)
	}
	buf1 := make([]byte, 1024)
	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	n1, err := conn1.Read(buf1)
	if err != nil || string(buf1[:n1]) != "echo: client1-hello" {
		t.Fatalf("conn1 echo failed: got %q, err %v", string(buf1[:n1]), err)
	}

	// 2. Client dials connection 2
	conn2, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial conn2: %v", err)
	}
	defer conn2.Close()

	_, err = conn2.Write([]byte("client2-hello"))
	if err != nil {
		t.Fatalf("conn2 write failed: %v", err)
	}
	buf2 := make([]byte, 1024)
	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	n2, err := conn2.Read(buf2)
	if err != nil || string(buf2[:n2]) != "echo: client2-hello" {
		t.Fatalf("conn2 echo failed: got %q, err %v", string(buf2[:n2]), err)
	}

	// 3. Verify backendConnCount equals 2
	if count := atomic.LoadInt64(&backendConnCount); count != 2 {
		t.Fatalf("expected backendConnCount 2, got %d", count)
	}

	// 4. Attempt connection 3 (must be rejected immediately)
	conn3, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial conn3: %v", err)
	}
	defer conn3.Close()

	_ = conn3.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf3 := make([]byte, 1024)
	_, readErr := conn3.Read(buf3)
	if readErr == nil {
		t.Fatalf("expected conn3 read to return EOF or closed error, got nil")
	}

	// Backend count must remain at 2
	if count := atomic.LoadInt64(&backendConnCount); count != 2 {
		t.Fatalf("expected backendConnCount to remain 2, got %d", count)
	}

	// 5. Close conn1 and wait briefly for counter to decrement
	_ = conn1.Close()
	time.Sleep(50 * time.Millisecond)

	// 6. Attempt connection 4 (should succeed)
	conn4, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial conn4: %v", err)
	}
	defer conn4.Close()

	_, err = conn4.Write([]byte("client4-hello"))
	if err != nil {
		t.Fatalf("conn4 write failed: %v", err)
	}
	buf4 := make([]byte, 1024)
	_ = conn4.SetReadDeadline(time.Now().Add(2 * time.Second))
	n4, err := conn4.Read(buf4)
	if err != nil || string(buf4[:n4]) != "echo: client4-hello" {
		t.Fatalf("conn4 echo failed: got %q, err %v", string(buf4[:n4]), err)
	}

	// Backend count should now be 3
	if count := atomic.LoadInt64(&backendConnCount); count != 3 {
		t.Fatalf("expected backendConnCount 3, got %d", count)
	}
}

// TC-087-02: TCP Bidirectional Idle Deadline Enforcement (Slowloris Mitigation)
func TestTCPProxy_IdleTimeout(t *testing.T) {
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start backend listener: %v", err)
	}
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					_, _ = c.Write([]byte(fmt.Sprintf("echo: %s", string(buf[:n]))))
				}
			}(conn)
		}
	}()

	backendAddr := backendListener.Addr().String()

	tcpProxy, err := NewTCPProxy([]string{backendAddr}, 5*time.Second,
		WithTCPIdleTimeout(150*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("failed to create TCPProxy: %v", err)
	}
	defer tcpProxy.Close()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		_ = tcpProxy.Serve(proxyListener)
	}()

	proxyAddr := proxyListener.Addr().String()

	// Subtest 2A: Idle Timeout Expiration
	t.Run("IdleTimeoutExpiration", func(t *testing.T) {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			t.Fatalf("failed to dial proxy: %v", err)
		}
		defer conn.Close()

		_, err = conn.Write([]byte("ping"))
		if err != nil {
			t.Fatalf("write failed: %v", err)
		}
		buf := make([]byte, 1024)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil || string(buf[:n]) != "echo: ping" {
			t.Fatalf("echo failed: got %q, err %v", string(buf[:n]), err)
		}

		// Cease transmission and sleep for 250ms (> 150ms IdleTimeout)
		time.Sleep(250 * time.Millisecond)

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, err = conn.Read(buf)
		if err == nil {
			t.Fatal("expected connection to be closed after idle timeout, but read succeeded")
		}
	})

	// Subtest 2B: Active Stream Deadline Refreshing
	t.Run("ActiveStreamDeadlineRefreshing", func(t *testing.T) {
		connActive, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			t.Fatalf("failed to dial proxy: %v", err)
		}
		defer connActive.Close()

		// Transmit every 50ms for 450ms (3x IdleTimeout)
		startTime := time.Now()
		for time.Since(startTime) < 450*time.Millisecond {
			_, err := connActive.Write([]byte("ping"))
			if err != nil {
				t.Fatalf("active write failed: %v", err)
			}
			buf := make([]byte, 1024)
			_ = connActive.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := connActive.Read(buf)
			if err != nil || string(buf[:n]) != "echo: ping" {
				t.Fatalf("active read failed: got %q, err %v", string(buf[:n]), err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
}

// TC-087-07: Graceful Shutdown (TCP Proxy)
func TestTCPProxy_GracefulShutdown(t *testing.T) {
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					_, _ = c.Write(buf[:n])
				}
			}(conn)
		}
	}()

	tcpProxy, err := NewTCPProxy([]string{backendListener.Addr().String()}, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen proxy: %v", err)
	}

	go func() {
		_ = tcpProxy.Serve(proxyListener)
	}()

	proxyAddr := proxyListener.Addr().String()

	// Establish 5 concurrent connections streaming data
	var wg sync.WaitGroup
	var clientConns []net.Conn
	for i := 0; i < 5; i++ {
		c, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			t.Fatalf("dial client %d failed: %v", i, err)
		}
		clientConns = append(clientConns, c)
		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			buf := make([]byte, 256)
			for {
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				_, err := conn.Write([]byte("stream-data"))
				if err != nil {
					return
				}
				_, err = conn.Read(buf)
				if err != nil {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(c)
	}

	time.Sleep(50 * time.Millisecond)

	// Close proxy and measure time
	start := time.Now()
	err = tcpProxy.Close()
	if err != nil {
		t.Fatalf("close returned error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected Close to complete within 500ms, took %v", elapsed)
	}

	// Verify all client streams receive errors
	wg.Wait()
	for _, c := range clientConns {
		_ = c.Close()
	}

	// Verify new connection is refused
	connAfter, err := net.DialTimeout("tcp", proxyAddr, 100*time.Millisecond)
	if err == nil {
		connAfter.Close()
		t.Fatal("expected connection to be refused after shutdown, but dial succeeded")
	}
}

// TC-087-08: Concurrency Race Safety (TCP Proxy)
func TestTCPProxy_ConcurrencyRaceSafety(t *testing.T) {
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					_, _ = c.Write(buf[:n])
				}
			}(conn)
		}
	}()

	tcpProxy, err := NewTCPProxy([]string{backendListener.Addr().String()}, 2*time.Second,
		WithTCPMaxConnections(20),
		WithTCPIdleTimeout(200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen proxy: %v", err)
	}

	go func() {
		_ = tcpProxy.Serve(proxyListener)
	}()

	proxyAddr := proxyListener.Addr().String()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					c, err := net.DialTimeout("tcp", proxyAddr, 100*time.Millisecond)
					if err != nil {
						time.Sleep(5 * time.Millisecond)
						continue
					}
					if tc, ok := c.(*net.TCPConn); ok {
						_ = tc.SetLinger(0)
					}
					_ = c.SetDeadline(time.Now().Add(200 * time.Millisecond))
					buf := make([]byte, 64)
					for j := 0; j < 3; j++ {
						_, _ = c.Write([]byte("race-safety-data"))
						_, _ = c.Read(buf)
					}
					_ = c.Close()
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(1 * time.Second)
	close(stop)
	_ = tcpProxy.Close()
	wg.Wait()
}

// TC-087-09: Performance Benchmark (TCP Proxy Forwarding)
func BenchmarkTCPProxy_Forwarding(b *testing.B) {
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to start backend: %v", err)
	}
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 2048)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					_, _ = c.Write(buf[:n])
				}
			}(conn)
		}
	}()

	tcpProxy, err := NewTCPProxy([]string{backendListener.Addr().String()}, 5*time.Second)
	if err != nil {
		b.Fatalf("failed to create proxy: %v", err)
	}
	defer tcpProxy.Close()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to listen proxy: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		_ = tcpProxy.Serve(proxyListener)
	}()

	conn, err := net.Dial("tcp", proxyListener.Addr().String())
	if err != nil {
		b.Fatalf("failed to dial proxy: %v", err)
	}
	defer conn.Close()

	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte('A' + (i % 26))
	}
	replyBuf := make([]byte, 2048)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := conn.Write(payload)
		if err != nil {
			b.Fatalf("write failed: %v", err)
		}
		_, err = io.ReadFull(conn, replyBuf[:len(payload)])
		if err != nil {
			b.Fatalf("read failed: %v", err)
		}
	}
}
