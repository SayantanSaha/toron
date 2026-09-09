package proxy

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestUDPProxy_Forwarding(t *testing.T) {
	// Start a mock UDP backend server
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve backend addr: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("failed to start backend UDP listener: %v", err)
	}
	defer backendConn.Close()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, clientAddr, err := backendConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			resp := []byte(fmt.Sprintf("udp-echo: %s", string(buf[:n])))
			_, _ = backendConn.WriteToUDP(resp, clientAddr)
		}
	}()

	backendTargetStr := backendConn.LocalAddr().String()

	// Initialize UDPProxy
	udpProxy, err := NewUDPProxy([]string{backendTargetStr}, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create UDPProxy: %v", err)
	}
	defer udpProxy.Close()

	proxyUDPAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve proxy addr: %v", err)
	}
	proxyConn, err := net.ListenUDP("udp", proxyUDPAddr)
	if err != nil {
		t.Fatalf("failed to start proxy UDP listener: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyStr := proxyConn.LocalAddr().String()

	// Client sends UDP packet to proxy
	clientUDPAddr, err := net.ResolveUDPAddr("udp", proxyStr)
	if err != nil {
		t.Fatalf("failed to resolve proxy client addr: %v", err)
	}
	clientConn, err := net.DialUDP("udp", nil, clientUDPAddr)
	if err != nil {
		t.Fatalf("failed to dial UDP proxy: %v", err)
	}
	defer clientConn.Close()

	testPayload := "hello udp proxy"
	_, err = clientConn.Write([]byte(testPayload))
	if err != nil {
		t.Fatalf("failed to send UDP payload: %v", err)
	}

	replyBuf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(replyBuf)
	if err != nil {
		t.Fatalf("failed to receive UDP response: %v", err)
	}

	expected := "udp-echo: hello udp proxy"
	if string(replyBuf[:n]) != expected {
		t.Errorf("got %q, expected %q", string(replyBuf[:n]), expected)
	}
}

// TC-087-03: UDP Bounded Worker Pool & Saturation Backpressure
func TestUDPProxy_WorkerPoolSaturation(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve backend addr: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendConn.Close()

	// Mock UDP backend that artificially pauses for 50ms before replying
	go func() {
		buf := make([]byte, 1024)
		for {
			n, clientAddr, err := backendConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			packet := make([]byte, n)
			copy(packet, buf[:n])
			go func(c *net.UDPAddr, p []byte) {
				time.Sleep(50 * time.Millisecond)
				_, _ = backendConn.WriteToUDP([]byte("delayed-echo: "+string(p)), c)
			}(clientAddr, packet)
		}
	}()

	udpProxy, err := NewUDPProxy([]string{backendConn.LocalAddr().String()}, 2*time.Second,
		WithUDPMaxWorkers(2),
	)
	if err != nil {
		t.Fatalf("failed to create UDPProxy: %v", err)
	}
	defer udpProxy.Close()

	proxyConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to listen UDP proxy: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyAddr := proxyConn.LocalAddr().(*net.UDPAddr)

	baseline := runtime.NumGoroutine()

	// Concurrently blast 40 datagrams from distinct UDP client sockets within 5ms window
	var blastWg sync.WaitGroup
	for i := 0; i < 40; i++ {
		blastWg.Add(1)
		go func(id int) {
			defer blastWg.Done()
			c, err := net.DialUDP("udp", nil, proxyAddr)
			if err != nil {
				return
			}
			defer c.Close()
			_, _ = c.Write([]byte(fmt.Sprintf("blast-%d", id)))
		}(i)
	}

	blastWg.Wait()

	// Sample goroutines during burst processing in proxy (workers delayed by 50ms)
	sampled := runtime.NumGoroutine()

	// Goroutine count must remain strictly bounded: increase by at most O(MaxWorkers) (delta <= 8)
	delta := sampled - baseline
	if delta > 8 {
		t.Errorf("goroutine burst exploded: delta=%d (baseline=%d, sampled=%d)", delta, baseline, sampled)
	}

	// Wait 300ms for active workers to complete in-flight tasks and drain queue
	time.Sleep(300 * time.Millisecond)

	// Send single subsequent datagram within capacity from a new client and verify echo reply
	clientConn, err := net.DialUDP("udp", nil, proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial client after drain: %v", err)
	}
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("after-drain"))
	if err != nil {
		t.Fatalf("failed to write payload: %v", err)
	}

	replyBuf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(replyBuf)
	if err != nil {
		t.Fatalf("failed to receive response after drain: %v", err)
	}
	expected := "delayed-echo: after-drain"
	if string(replyBuf[:n]) != expected {
		t.Errorf("got %q, expected %q", string(replyBuf[:n]), expected)
	}
}

// TC-087-04: UDP Upstream Socket Reuse Across Datagrams
func TestUDPProxy_SocketReuse(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve backend addr: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendConn.Close()

	var portsMu sync.Mutex
	var receivedSourcePorts []int

	go func() {
		buf := make([]byte, 1024)
		for {
			n, clientAddr, err := backendConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			portsMu.Lock()
			receivedSourcePorts = append(receivedSourcePorts, clientAddr.Port)
			portsMu.Unlock()

			resp := []byte("echo: " + string(buf[:n]))
			_, _ = backendConn.WriteToUDP(resp, clientAddr)
		}
	}()

	udpProxy, err := NewUDPProxy([]string{backendConn.LocalAddr().String()}, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer udpProxy.Close()

	proxyConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to start proxy UDP: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyAddr := proxyConn.LocalAddr().(*net.UDPAddr)

	clientConn, err := net.DialUDP("udp", nil, proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer clientConn.Close()

	// Transmit 10 sequential datagrams spaced 10ms apart
	for i := 0; i < 10; i++ {
		payload := fmt.Sprintf("packet-%d", i)
		_, err := clientConn.Write([]byte(payload))
		if err != nil {
			t.Fatalf("packet %d write failed: %v", i, err)
		}
		reply := make([]byte, 1024)
		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := clientConn.Read(reply)
		if err != nil || string(reply[:n]) != "echo: "+payload {
			t.Fatalf("packet %d echo failed: got %q, err %v", i, string(reply[:n]), err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	portsMu.Lock()
	defer portsMu.Unlock()

	if len(receivedSourcePorts) != 10 {
		t.Fatalf("expected 10 recorded packets, got %d", len(receivedSourcePorts))
	}

	// Verify exact same upstream source port across all 10 packets
	firstPort := receivedSourcePorts[0]
	for idx, port := range receivedSourcePorts {
		if port != firstPort {
			t.Fatalf("port mismatch at packet %d: expected %d, got %d", idx, firstPort, port)
		}
	}
}

// TC-087-05: UDP Session Idle Timeout & Eviction
func TestUDPProxy_SessionIdleTimeout(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve backend addr: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendConn.Close()

	var portsMu sync.Mutex
	var receivedPorts []int

	go func() {
		buf := make([]byte, 1024)
		for {
			n, clientAddr, err := backendConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			portsMu.Lock()
			receivedPorts = append(receivedPorts, clientAddr.Port)
			portsMu.Unlock()

			resp := []byte("echo: " + string(buf[:n]))
			_, _ = backendConn.WriteToUDP(resp, clientAddr)
		}
	}()

	udpProxy, err := NewUDPProxy([]string{backendConn.LocalAddr().String()}, 2*time.Second,
		WithUDPIdleTimeout(200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer udpProxy.Close()

	proxyConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to start proxy UDP: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyAddr := proxyConn.LocalAddr().(*net.UDPAddr)

	clientConn, err := net.DialUDP("udp", nil, proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer clientConn.Close()

	// 1. Send datagram 1
	_, err = clientConn.Write([]byte("session-packet-1"))
	if err != nil {
		t.Fatalf("packet 1 write failed: %v", err)
	}
	reply := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(reply)
	if err != nil || string(reply[:n]) != "echo: session-packet-1" {
		t.Fatalf("packet 1 echo failed: got %q, err %v", string(reply[:n]), err)
	}

	// 2. Pause transmission for 350ms (> IdleTimeout 200ms + sweeper interval 100ms)
	time.Sleep(350 * time.Millisecond)

	// Verify session was evicted
	for i := 0; i < 10 && udpProxy.ActiveSessions() > 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if sessions := udpProxy.ActiveSessions(); sessions != 0 {
		t.Fatalf("expected active sessions to be 0 after idle eviction, got %d", sessions)
	}

	// 3. Send datagram 2 from the same client socket
	_, err = clientConn.Write([]byte("session-packet-2"))
	if err != nil {
		t.Fatalf("packet 2 write failed: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = clientConn.Read(reply)
	if err != nil || string(reply[:n]) != "echo: session-packet-2" {
		t.Fatalf("packet 2 echo failed: got %q, err %v", string(reply[:n]), err)
	}

	portsMu.Lock()
	defer portsMu.Unlock()

	if len(receivedPorts) < 2 {
		t.Fatalf("expected at least 2 received packets, got %d", len(receivedPorts))
	}
	p1 := receivedPorts[0]
	p2 := receivedPorts[len(receivedPorts)-1]

	// Backend must observe a new upstream port (P2 != P1)
	if p1 == p2 {
		t.Fatalf("expected different upstream ports after idle eviction, got p1=%d, p2=%d", p1, p2)
	}
}

// TC-087-06: UDP Buffer Pooling (Zero Heap Churn)
func TestUDPProxy_BufferPooling(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve backend addr: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendConn.Close()

	go func() {
		buf := make([]byte, 2048)
		for {
			n, clientAddrPort, err := backendConn.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			_, _ = backendConn.WriteToUDPAddrPort(buf[:n], clientAddrPort)
		}
	}()

	udpProxy, err := NewUDPProxy([]string{backendConn.LocalAddr().String()}, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}
	defer udpProxy.Close()

	proxyConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to start proxy UDP: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyAddr := proxyConn.LocalAddr().(*net.UDPAddr)

	clientConn, err := net.DialUDP("udp", nil, proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer clientConn.Close()

	// Warmup 10 datagrams to prime buffer pool and establish session
	warmupBuf := make([]byte, 1024)
	for i := 0; i < 10; i++ {
		_, _ = clientConn.Write([]byte("warmup"))
		_ = clientConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, _ = clientConn.Read(warmupBuf)
	}

	payload := []byte("pool-test-payload")
	replyBuf := make([]byte, 1024)
	_ = clientConn.SetDeadline(time.Now().Add(10 * time.Second))

	allocs := testing.AllocsPerRun(100, func() {
		_, err := clientConn.Write(payload)
		if err != nil {
			return
		}
		_, _ = clientConn.Read(replyBuf)
	})

	// Steady-state heap allocations per datagram do not allocate new 65KB slices (allocs/op <= 2)
	if allocs > 2 {
		t.Errorf("expected allocs/op <= 2, got %v", allocs)
	}
}

// TC-087-07: Graceful Shutdown (UDP Proxy)
func TestUDPProxy_GracefulShutdown(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve backend: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendConn.Close()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, clientAddr, err := backendConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = backendConn.WriteToUDP(buf[:n], clientAddr)
		}
	}()

	udpProxy, err := NewUDPProxy([]string{backendConn.LocalAddr().String()}, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	proxyConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to start proxy UDP: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyAddr := proxyConn.LocalAddr().(*net.UDPAddr)

	// Stream continuous datagrams across 5 clients
	var wg sync.WaitGroup
	var clientConns []*net.UDPConn
	for i := 0; i < 5; i++ {
		c, err := net.DialUDP("udp", nil, proxyAddr)
		if err != nil {
			t.Fatalf("dial client %d failed: %v", i, err)
		}
		clientConns = append(clientConns, c)
		wg.Add(1)
		go func(conn *net.UDPConn) {
			defer wg.Done()
			buf := make([]byte, 256)
			for {
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				_, err := conn.Write([]byte("udp-stream-data"))
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

	// Close UDP proxy and measure duration
	start := time.Now()
	err = udpProxy.Close()
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected Close to complete within 500ms, took %v", elapsed)
	}

	// Wait for clients to finish and close them
	for _, c := range clientConns {
		_ = c.Close()
	}
	wg.Wait()

	// Verify all sessions closed
	if sessions := udpProxy.ActiveSessions(); sessions != 0 {
		t.Errorf("expected 0 active sessions after Close, got %d", sessions)
	}
}

// TC-087-08: Concurrency Race Safety (UDP Proxy)
func TestUDPProxy_ConcurrencyRaceSafety(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve backend: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("failed to start backend: %v", err)
	}
	defer backendConn.Close()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, clientAddr, err := backendConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = backendConn.WriteToUDP(buf[:n], clientAddr)
		}
	}()

	udpProxy, err := NewUDPProxy([]string{backendConn.LocalAddr().String()}, 2*time.Second,
		WithUDPMaxWorkers(16),
		WithUDPIdleTimeout(200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	proxyConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to start proxy: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyAddr := proxyConn.LocalAddr().(*net.UDPAddr)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.DialUDP("udp", nil, proxyAddr)
			if err != nil {
				return
			}
			defer c.Close()
			buf := make([]byte, 64)
			for {
				select {
				case <-stop:
					return
				default:
					_ = c.SetDeadline(time.Now().Add(200 * time.Millisecond))
					_, _ = c.Write([]byte("race-udp-packet"))
					_, _ = c.Read(buf)
					time.Sleep(5 * time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(1 * time.Second)
	close(stop)
	_ = udpProxy.Close()
	wg.Wait()
}

// TC-087-09: Performance Benchmark (UDP Proxy Forwarding)
func BenchmarkUDPProxy_Forwarding(b *testing.B) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to resolve backend: %v", err)
	}
	backendConn, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		b.Fatalf("failed to start backend: %v", err)
	}
	defer backendConn.Close()

	go func() {
		buf := make([]byte, 2048)
		for {
			n, clientAddr, err := backendConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = backendConn.WriteToUDP(buf[:n], clientAddr)
		}
	}()

	udpProxy, err := NewUDPProxy([]string{backendConn.LocalAddr().String()}, 5*time.Second)
	if err != nil {
		b.Fatalf("failed to create proxy: %v", err)
	}
	defer udpProxy.Close()

	proxyConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		b.Fatalf("failed to start proxy: %v", err)
	}

	go func() {
		_ = udpProxy.Serve(proxyConn)
	}()

	proxyAddr := proxyConn.LocalAddr().(*net.UDPAddr)

	clientConn, err := net.DialUDP("udp", nil, proxyAddr)
	if err != nil {
		b.Fatalf("failed to dial proxy: %v", err)
	}
	defer clientConn.Close()

	payload := make([]byte, 512)
	for i := range payload {
		payload[i] = byte('U')
	}
	replyBuf := make([]byte, 2048)

	// Warmup 5 packets
	for i := 0; i < 5; i++ {
		_, _ = clientConn.Write(payload)
		_ = clientConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, _ = clientConn.Read(replyBuf)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := clientConn.Write(payload)
		if err != nil {
			b.Fatalf("write failed: %v", err)
		}
		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err = clientConn.Read(replyBuf)
		if err != nil {
			b.Fatalf("read failed: %v", err)
		}
	}
}
