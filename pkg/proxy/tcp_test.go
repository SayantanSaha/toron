package proxy

import (
	"fmt"
	"io"
	"net"
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
