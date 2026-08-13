package server_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/server"
)

func TestServer_IntegrationAndHealth(t *testing.T) {
	r := router.New()
	r.GET("/health", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"ok"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"

	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	addr := ln.Addr().String()

	// Make HTTP GET /health request
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /health HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	respBuf, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read response: %v", err)
	}

	output := string(respBuf)
	if !bytes.Contains(respBuf, []byte("HTTP/1.1 200 OK")) {
		t.Errorf("expected HTTP 200 OK, got:\n%s", output)
	}
	if !bytes.Contains(respBuf, []byte(`{"status":"ok"}`)) {
		t.Errorf("expected health body, got:\n%s", output)
	}

	// Test graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
}

func TestServer_SecurityLimitsHeaderExceeded(t *testing.T) {
	r := router.New()
	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.MaxHeaderBytes = 512 // Small limit for testing

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Send oversized header
	largeHeader := "GET / HTTP/1.1\r\nHost: localhost\r\nX-Large: " + string(make([]byte, 1024)) + "\r\n\r\n"
	_, _ = conn.Write([]byte(largeHeader))

	respBuf, _ := io.ReadAll(conn)

	if !bytes.Contains(respBuf, []byte("431 Request Header Fields Too Large")) {
		t.Errorf("expected status 431 header too large, got:\n%s", string(respBuf))
	}
}

func TestServer_AltSvcHeader(t *testing.T) {
	r := router.New()
	r.GET("/test", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("ok")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.HTTP3Enabled = true
	cfg.HTTP3Port = 8443
	cfg.HTTP3AltSvcHeader = true

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /test HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	respBuf, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read response: %v", err)
	}

	if !bytes.Contains(bytes.ToLower(respBuf), []byte(`alt-svc: h3=":8443"`)) {
		t.Errorf("expected Alt-Svc header advertising h3=:8443, got:\n%s", string(respBuf))
	}
}

func TestInternalAPI_StatusAndMetrics(t *testing.T) {
	r := router.New()
	server.RegisterInternalAPIRoutes(r, server.InternalAPIConfig{
		Port:           8080,
		WorkerPoolSize: 128,
		ProxyEnabled:   true,
	})

	// Test GET /internal/api/status
	reqStatus, _ := httpparser.NewRequest("GET", "/internal/api/status", "HTTP/1.1")
	resStatus := httpparser.NewResponse()
	r.ServeHTTP(reqStatus, resStatus)

	if resStatus.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /internal/api/status, got %d", resStatus.StatusCode)
	}
	if !bytes.Contains(resStatus.Body.Bytes(), []byte(`"metrics":`)) {
		t.Errorf("expected metrics object in /internal/api/status JSON, got:\n%s", resStatus.Body.String())
	}

	// Test GET /internal/api/metrics
	reqMetrics, _ := httpparser.NewRequest("GET", "/internal/api/metrics", "HTTP/1.1")
	resMetrics := httpparser.NewResponse()
	r.ServeHTTP(reqMetrics, resMetrics)

	if resMetrics.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /internal/api/metrics, got %d", resMetrics.StatusCode)
	}
	if !bytes.Contains(resMetrics.Body.Bytes(), []byte(`"total_requests":`)) {
		t.Errorf("expected total_requests key in /internal/api/metrics JSON, got:\n%s", resMetrics.Body.String())
	}
}
