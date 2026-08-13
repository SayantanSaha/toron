package proxy

import (
	"fmt"
	"net"
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
