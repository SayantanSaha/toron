package server_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/server"
)

func TestServer_HTTP2PriorKnowledge(t *testing.T) {
	r := router.New()
	r.GET("/http2-test", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString(`{"protocol":"HTTP/2.0","status":"success"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.HTTP2Enabled = true

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()

	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
		Timeout: 5 * time.Second,
	}

	urlStr := fmt.Sprintf("http://%s/http2-test", ln.Addr().String())
	resp, err := client.Get(urlStr)
	if err != nil {
		t.Fatalf("HTTP/2 client request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	expectedBody := `{"protocol":"HTTP/2.0","status":"success"}`
	if string(body) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestServer_HTTP2ExtendedConnect(t *testing.T) {
	// Start mock raw upstream server
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen upstream: %v", err)
	}
	defer upstreamLn.Close()

	go func() {
		conn, err := upstreamLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err == nil && n > 0 {
			upgradeResp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
			_, _ = conn.Write([]byte(upgradeResp))
		}
	}()

	r := router.New()
	if err := r.Proxy("/ws-h2", "http://"+upstreamLn.Addr().String()); err != nil {
		t.Fatalf("failed to setup proxy route: %v", err)
	}

	req := &httpparser.Request{
		Method: "CONNECT",
		Path:   "/ws-h2",
		Header: make(httpparser.Header),
	}
	req.Header.Set(":protocol", "websocket")

	if !req.IsWebSocketUpgrade() {
		t.Fatalf("expected IsWebSocketUpgrade() to return true for RFC 8441 CONNECT request")
	}

	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected proxy to return 101 Switching Protocols to router, got %d", res.StatusCode)
	}
	if res.UpgradedConn == nil {
		t.Fatalf("expected non-nil UpgradedConn from upstream")
	}
	_ = res.UpgradedConn.Close()
}
