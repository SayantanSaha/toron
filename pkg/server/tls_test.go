package server_test

import (
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

func TestServer_HTTPSSelfSigned(t *testing.T) {
	r := router.New()
	r.GET("/tls-test", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString(`{"tls":"enabled","status":"success"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.TLSEnabled = true
	cfg.TLSAutoDevCert = true

	srv := server.New(cfg, r)

	tlsConfig, err := server.CreateTLSConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create tls config: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	tlsListener := tls.NewListener(ln, tlsConfig)

	go func() {
		_ = srv.Serve(tlsListener)
	}()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
	}

	urlStr := fmt.Sprintf("https://%s/tls-test", ln.Addr().String())
	resp, err := client.Get(urlStr)
	if err != nil {
		t.Fatalf("HTTPS client request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	expectedBody := `{"tls":"enabled","status":"success"}`
	if string(body) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestServer_HTTPSWithALPNHTTP2(t *testing.T) {
	r := router.New()
	r.GET("/alpn-test", func(req *httpparser.Request, res *httpparser.Response) {
		res.Header.Set("Content-Type", "application/json")
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString(`{"alpn":"h2","status":"success"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.TLSEnabled = true
	cfg.TLSAutoDevCert = true
	cfg.HTTP2Enabled = true

	srv := server.New(cfg, r)

	tlsConfig, err := server.CreateTLSConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create tls config: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	tlsListener := tls.NewListener(ln, tlsConfig)

	go func() {
		_ = srv.Serve(tlsListener)
	}()

	client := &http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{"h2"},
			},
		},
		Timeout: 5 * time.Second,
	}

	urlStr := fmt.Sprintf("https://%s/alpn-test", ln.Addr().String())
	resp, err := client.Get(urlStr)
	if err != nil {
		t.Fatalf("HTTPS ALPN h2 request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Proto != "HTTP/2.0" {
		t.Errorf("expected HTTP/2.0 protocol over ALPN, got %q", resp.Proto)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	expectedBody := `{"alpn":"h2","status":"success"}`
	if string(body) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(body))
	}
}
