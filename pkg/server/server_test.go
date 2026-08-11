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
