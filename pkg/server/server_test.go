package server_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"

	"toron/pkg/httpparser"
	"toron/pkg/proxy"
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

func TestServer_HTTP2Adapter_AltSvcHeader(t *testing.T) {
	// Subtest 1: Configuration A (Default Port 8443)
	t.Run("Default Port 8443", func(t *testing.T) {
		r := router.New()
		r.GET("/test", func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString("ok")
		})

		cfg := server.DefaultConfig()
		cfg.HTTP3Enabled = true
		cfg.HTTP3Port = 8443
		cfg.HTTP3AltSvcHeader = true

		srv := server.New(cfg, r)
		handler := srv.HTTP2AdapterHandler()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Proto = "HTTP/2.0"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		expected := `h3=":8443"; ma=2592000`
		if actual := rec.Header().Get("Alt-Svc"); actual != expected {
			t.Errorf("expected Alt-Svc header %q, got %q", expected, actual)
		}
	})

	// Subtest 2: Configuration B (Custom Port 9443)
	t.Run("Custom Port 9443", func(t *testing.T) {
		r := router.New()
		r.GET("/test", func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString("ok")
		})

		cfg := server.DefaultConfig()
		cfg.HTTP3Enabled = true
		cfg.HTTP3Port = 9443
		cfg.HTTP3AltSvcHeader = true

		srv := server.New(cfg, r)
		handler := srv.HTTP2AdapterHandler()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Proto = "HTTP/2.0"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		expected := `h3=":9443"; ma=2592000`
		if actual := rec.Header().Get("Alt-Svc"); actual != expected {
			t.Errorf("expected Alt-Svc header %q, got %q", expected, actual)
		}
	})

	// Subtest 3: Configuration C (Disabled Advertising)
	t.Run("Disabled Advertising", func(t *testing.T) {
		r := router.New()
		r.GET("/test", func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString("ok")
		})

		cfg := server.DefaultConfig()
		cfg.HTTP3Enabled = false
		cfg.HTTP3AltSvcHeader = false

		srv := server.New(cfg, r)
		handler := srv.HTTP2AdapterHandler()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Proto = "HTTP/2.0"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if actual := rec.Header().Get("Alt-Svc"); actual != "" {
			t.Errorf("expected Alt-Svc header to be absent, got %q", actual)
		}
	})

	// Subtest 4: Preserve Pre-existing Alt-Svc Header
	t.Run("Preserve Pre-existing Alt-Svc Header", func(t *testing.T) {
		r := router.New()
		r.GET("/test", func(req *httpparser.Request, res *httpparser.Response) {
			res.Header.Set("Alt-Svc", `h3=":443"; ma=3600`)
			res.SetStatus(http.StatusOK)
			_, _ = res.WriteString("ok")
		})

		cfg := server.DefaultConfig()
		cfg.HTTP3Enabled = true
		cfg.HTTP3Port = 8443
		cfg.HTTP3AltSvcHeader = true

		srv := server.New(cfg, r)
		handler := srv.HTTP2AdapterHandler()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Proto = "HTTP/2.0"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		expected := `h3=":443"; ma=3600`
		if actual := rec.Header().Get("Alt-Svc"); actual != expected {
			t.Errorf("expected pre-existing Alt-Svc header %q, got %q", expected, actual)
		}
	})
}

func TestServer_HTTP3_Shutdown(t *testing.T) {
	// Subtest 1: Active HTTP/3 server shutdown invokes Close()
	t.Run("Active H3 Server Close", func(t *testing.T) {
		cfg := server.DefaultConfig()
		cfg.HTTP3Enabled = true
		cfg.HTTP3Port = 8443
		srv := server.New(cfg, nil)

		h3Srv := &http3.Server{}
		srv.SetH3Server(h3Srv)

		if srv.H3Server() != h3Srv {
			t.Fatalf("expected H3Server to return attached instance")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			t.Fatalf("expected clean shutdown, got: %v", err)
		}

		// Verify h3Srv was closed
		if err := h3Srv.ServeQUICConn(nil); !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("expected http.ErrServerClosed after Close(), got: %v", err)
		}
	})

	// Subtest 2: Nil safety check
	t.Run("Nil Safety", func(t *testing.T) {
		cfg := server.DefaultConfig()
		cfg.HTTP3Enabled = false
		srv := server.New(cfg, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			t.Fatalf("expected clean shutdown with nil h3Server, got: %v", err)
		}
	})
}

func TestServer_HTTPRedirect_Integration(t *testing.T) {
	r := router.New()
	falseVal := false

	// Register exempt route: /challenge/
	r.GET("/challenge/token.txt", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("challenge-token-data")
	})
	// Prefix route with redirect_http = false
	_ = r.RoutePrefix(router.RouteTypeStatic, "", "/challenge", nil, t.TempDir(), proxy.ProxyOptions{
		RedirectHTTP: &falseVal,
	})

	// Register normal route: /dashboard
	r.GET("/dashboard", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("dashboard-ok")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.HTTPRedirectEnabled = true
	cfg.HTTPRedirectPort = 8080
	cfg.HTTPSPort = 443
	cfg.HTTPRedirectAllowedHosts = []string{"example.com"}

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()

	addr := ln.Addr().String()

	// Subtest 1: Normal route returns 301 redirect to HTTPS
	t.Run("Standard 301 Redirect to HTTPS", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer conn.Close()

		req := "GET /dashboard?tab=analytics HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		resp, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
		respStr := string(resp)

		if !strings.Contains(respStr, "301 Moved Permanently") {
			t.Errorf("expected 301 Moved Permanently, got:\n%s", respStr)
		}
		if !strings.Contains(strings.ToLower(respStr), "location: https://example.com/dashboard?tab=analytics") {
			t.Errorf("expected Location header with query preserved, got:\n%s", respStr)
		}
	})

	// Subtest 2: Host header with port strips port in redirect
	t.Run("Host Header Port Stripping", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer conn.Close()

		req := "GET /dashboard HTTP/1.1\r\nHost: example.com:8080\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		resp, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
		respStr := string(resp)

		if !strings.Contains(strings.ToLower(respStr), "location: https://example.com/dashboard\r\n") {
			t.Errorf("expected port stripped in Location, got:\n%s", respStr)
		}
	})

	// Subtest 3: Non-standard HTTPS port includes port in Location
	t.Run("Non-standard HTTPS Port", func(t *testing.T) {
		cfgCustom := cfg
		cfgCustom.HTTPSPort = 8443
		srvCustom := server.New(cfgCustom, r)
		lnCustom, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen failed: %v", err)
		}
		defer lnCustom.Close()
		go func() { _ = srvCustom.Serve(lnCustom) }()

		conn, err := net.Dial("tcp", lnCustom.Addr().String())
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer conn.Close()

		req := "GET /login HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		resp, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
		respStr := string(resp)

		if !strings.Contains(strings.ToLower(respStr), "location: https://example.com:8443/login") {
			t.Errorf("expected Location with custom port :8443, got:\n%s", respStr)
		}
	})

	// Subtest 4: Exempt route (redirect_http = false) served directly over HTTP (200 OK)
	t.Run("Exempt Route Direct HTTP Serving", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer conn.Close()

		req := "GET /challenge/token.txt HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		resp, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
		respStr := string(resp)

		if !strings.Contains(respStr, "200 OK") {
			t.Errorf("expected 200 OK for exempt route, got:\n%s", respStr)
		}
		if !strings.Contains(respStr, "challenge-token-data") {
			t.Errorf("expected token body for exempt route, got:\n%s", respStr)
		}
		if strings.Contains(strings.ToLower(respStr), "location:") {
			t.Errorf("did not expect Location header on exempt route, got:\n%s", respStr)
		}
	})

	// Subtest 5: Security - Malformed Host with illegal characters rejected with 400 Bad Request
	t.Run("Security Malformed Host Rejection", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer conn.Close()

		req := "GET /dashboard HTTP/1.1\r\nHost: example.com/evil\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		resp, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
		respStr := string(resp)

		if !strings.Contains(respStr, "400 Bad Request") {
			t.Errorf("expected 400 Bad Request for malicious host, got:\n%s", respStr)
		}
	})

	// Subtest 6: Unrecognized host header returns 400 Bad Request (Open Redirect Prevention - ADR-065)
	t.Run("Security Unrecognized Host Rejection", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer conn.Close()

		req := "GET /dashboard HTTP/1.1\r\nHost: attacker.com\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		resp, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
		respStr := string(resp)

		if !strings.Contains(respStr, "400 Bad Request") {
			t.Errorf("expected 400 Bad Request for unrecognized host, got:\n%s", respStr)
		}
		if !strings.Contains(respStr, "Unrecognized Host Header") {
			t.Errorf("expected 'Unrecognized Host Header' error message, got:\n%s", respStr)
		}
		if strings.Contains(strings.ToLower(respStr), "location:") {
			t.Errorf("Location header should NOT be present for unrecognized host, got:\n%s", respStr)
		}
	})

	// Subtest 7: Unrecognized host with DefaultHost configured redirects to DefaultHost
	t.Run("DefaultHost Fallback For Unrecognized Host", func(t *testing.T) {
		cfgFallback := cfg
		cfgFallback.HTTPRedirectDefaultHost = "canonical.example.com"
		srvFallback := server.New(cfgFallback, r)
		lnFallback, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen failed: %v", err)
		}
		defer lnFallback.Close()
		go func() { _ = srvFallback.Serve(lnFallback) }()

		conn, err := net.Dial("tcp", lnFallback.Addr().String())
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer conn.Close()

		req := "GET /dashboard HTTP/1.1\r\nHost: attacker.com\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		resp, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
		respStr := string(resp)

		if !strings.Contains(respStr, "301 Moved Permanently") {
			t.Errorf("expected 301 Moved Permanently, got:\n%s", respStr)
		}
		if !strings.Contains(strings.ToLower(respStr), "location: https://canonical.example.com/dashboard") {
			t.Errorf("expected Location redirect to canonical.example.com, got:\n%s", respStr)
		}
	})

	// Subtest 8: Clean graceful shutdown
	t.Run("Graceful Shutdown", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Fatalf("shutdown error: %v", err)
		}
	})
}

func TestServer_UnsupportedTransferEncodingRejection(t *testing.T) {
	r := router.New()
	r.POST("/upload", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("uploaded")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	addr := ln.Addr().String()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Send chunked request followed by pipelined smuggled request
	req := "POST /upload HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\nGET /smuggled HTTP/1.1\r\nHost: localhost\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	resp, err := io.ReadAll(conn)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("read failed: %v", err)
	}

	respStr := string(resp)
	if !strings.Contains(respStr, "501 Not Implemented") {
		t.Errorf("expected 501 Not Implemented, got:\n%s", respStr)
	}
	if !strings.Contains(strings.ToLower(respStr), "connection: close") {
		t.Errorf("expected Connection: close, got:\n%s", respStr)
	}
}

func TestHTTP2AdapterHandler_MaxBodyBytes(t *testing.T) {
	r := router.New()
	r.POST("/upload", func(req *httpparser.Request, res *httpparser.Response) {
		body, _ := io.ReadAll(req.Body)
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("received:" + string(body))
	})

	cfg := server.DefaultConfig()
	cfg.MaxBodyBytes = 1024 // 1 KB limit
	srv := server.New(cfg, r)
	handler := srv.HTTP2AdapterHandler()

	t.Run("TC-076-01: Compliant Request Under MaxBodyBytes", func(t *testing.T) {
		payload := bytes.Repeat([]byte("a"), 512)
		httpReq := httptest.NewRequest("POST", "/upload", bytes.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "received:") {
			t.Errorf("expected router to receive payload, got %q", rec.Body.String())
		}
	})

	t.Run("TC-076-02: Oversized Request Exceeding MaxBodyBytes", func(t *testing.T) {
		payload := bytes.Repeat([]byte("a"), 2048) // Exceeds 1024 limit
		httpReq := httptest.NewRequest("POST", "/upload", bytes.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413 Payload Too Large, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "413 Payload Too Large") {
			t.Errorf("expected 413 error body, got %q", rec.Body.String())
		}
	})
}

// TC-088-02: HTTP/1.1 Upgraded Connection Idle Timeout Teardown
func TestServer_UpgradedConn_IdleTimeout(t *testing.T) {
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen upstream: %v", err)
	}
	defer upstreamLn.Close()

	var backendClosed atomic.Bool
	go func() {
		backendConn, err := upstreamLn.Accept()
		if err != nil {
			return
		}
		defer backendConn.Close()
		defer backendClosed.Store(true)

		buf := make([]byte, 1024)
		for {
			n, err := backendConn.Read(buf)
			if err != nil {
				break
			}
			resp := append([]byte("echo: "), buf[:n]...)
			if _, err := backendConn.Write(resp); err != nil {
				break
			}
		}
	}()

	r := router.New()
	r.GET("/ws-idle", func(req *httpparser.Request, res *httpparser.Response) {
		upstreamConn, err := net.Dial("tcp", upstreamLn.Addr().String())
		if err != nil {
			res.SetStatus(http.StatusBadGateway)
			return
		}
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
		res.Header.Set("Connection", "Upgrade")
		res.UpgradedConn = upstreamConn
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.UpgradeIdleTimeout = 150 * time.Millisecond
	cfg.ReadTimeout = 2 * time.Second

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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	baselineGoroutines := runtime.NumGoroutine()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /ws-idle HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write upgrade request: %v", err)
	}

	respBuf := make([]byte, 512)
	n, err := conn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read upgrade response: %v", err)
	}
	if !bytes.Contains(respBuf[:n], []byte("101 Switching Protocols")) {
		t.Fatalf("expected 101 Switching Protocols, got:\n%s", string(respBuf[:n]))
	}

	if _, err := conn.Write([]byte("hello-handshake")); err != nil {
		t.Fatalf("failed to write initial payload: %v", err)
	}
	echoBuf := make([]byte, 512)
	n, err = conn.Read(echoBuf)
	if err != nil {
		t.Fatalf("failed to read echo: %v", err)
	}
	if !strings.Contains(string(echoBuf[:n]), "echo: hello-handshake") {
		t.Fatalf("expected 'echo: hello-handshake', got %q", string(echoBuf[:n]))
	}

	idleStart := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	readBuf := make([]byte, 512)
	_, readErr := conn.Read(readBuf)
	deltaT := time.Since(idleStart)

	if readErr == nil {
		t.Fatal("expected connection read to fail with EOF or timeout after idle period, got nil")
	}

	if deltaT < 130*time.Millisecond || deltaT > 600*time.Millisecond {
		t.Errorf("expected teardown duration ~150ms, got %v", deltaT)
	}

	time.Sleep(50 * time.Millisecond)
	if !backendClosed.Load() {
		t.Errorf("expected upstream backend socket to be closed")
	}

	currentGoroutines := runtime.NumGoroutine()
	if currentGoroutines > baselineGoroutines+5 {
		t.Errorf("potential goroutine leak: baseline %d, current %d", baselineGoroutines, currentGoroutines)
	}
}

// TC-088-03: HTTP/1.1 Heartbeat / Active Stream Preservation
func TestServer_UpgradedConn_HeartbeatKeepsAlive(t *testing.T) {
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen upstream: %v", err)
	}
	defer upstreamLn.Close()

	go func() {
		for {
			conn, err := upstreamLn.Accept()
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
					resp := append([]byte("echo: "), buf[:n]...)
					if _, err := c.Write(resp); err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	r := router.New()
	r.GET("/ws-heartbeat", func(req *httpparser.Request, res *httpparser.Response) {
		upstreamConn, err := net.Dial("tcp", upstreamLn.Addr().String())
		if err != nil {
			res.SetStatus(http.StatusBadGateway)
			return
		}
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
		res.Header.Set("Connection", "Upgrade")
		res.UpgradedConn = upstreamConn
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.UpgradeIdleTimeout = 150 * time.Millisecond

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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /ws-heartbeat HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write upgrade: %v", err)
	}

	respBuf := make([]byte, 512)
	n, err := conn.Read(respBuf)
	if err != nil || !bytes.Contains(respBuf[:n], []byte("101 Switching Protocols")) {
		t.Fatalf("failed to establish upgrade: %v, resp: %s", err, string(respBuf[:n]))
	}

	// Send heartbeats every 50ms for 400ms (> 2.6x UpgradeIdleTimeout)
	for i := 1; i <= 8; i++ {
		pingMsg := fmt.Sprintf("ping-%d", i)
		if _, err := conn.Write([]byte(pingMsg)); err != nil {
			t.Fatalf("failed to send ping %d: %v", i, err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		echoBuf := make([]byte, 256)
		n, err := conn.Read(echoBuf)
		if err != nil {
			t.Fatalf("failed to read echo for ping %d: %v", i, err)
		}
		expected := fmt.Sprintf("echo: ping-%d", i)
		if !strings.Contains(string(echoBuf[:n]), expected) {
			t.Fatalf("expected %q, got %q", expected, string(echoBuf[:n]))
		}

		if i < 8 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Cease heartbeats; connection must terminate after idle timeout (~150ms)
	ceaseTime := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(800 * time.Millisecond))
	buf := make([]byte, 256)
	_, readErr := conn.Read(buf)
	elapsed := time.Since(ceaseTime)

	if readErr == nil {
		t.Fatal("expected connection to close after heartbeats ceased, but read succeeded")
	}
	if elapsed < 100*time.Millisecond || elapsed > 400*time.Millisecond {
		t.Errorf("expected idle timeout ~150ms after heartbeats ceased, got %v", elapsed)
	}
}

// TC-088-04: HTTP/1.1 Peer Disconnect Teardown
func TestServer_UpgradedConn_PeerDisconnect(t *testing.T) {
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen upstream: %v", err)
	}
	defer upstreamLn.Close()

	upstreamAcceptedCh := make(chan net.Conn, 5)

	go func() {
		for {
			c, err := upstreamLn.Accept()
			if err != nil {
				return
			}
			upstreamAcceptedCh <- c
		}
	}()

	r := router.New()
	r.GET("/ws-disconnect", func(req *httpparser.Request, res *httpparser.Response) {
		upstreamConn, err := net.Dial("tcp", upstreamLn.Addr().String())
		if err != nil {
			res.SetStatus(http.StatusBadGateway)
			return
		}
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
		res.Header.Set("Connection", "Upgrade")
		res.UpgradedConn = upstreamConn
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.UpgradeIdleTimeout = 5 * time.Second // Generous timeout to verify peer close triggers teardown

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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	t.Run("Subtest 4A: Client Disconnection Triggers Immediate Upstream Closure", func(t *testing.T) {
		clientConn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}

		reqStr := "GET /ws-disconnect HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
		_, _ = clientConn.Write([]byte(reqStr))

		respBuf := make([]byte, 512)
		_, _ = clientConn.Read(respBuf)

		backendConn := <-upstreamAcceptedCh
		defer backendConn.Close()

		closeStart := time.Now()
		_ = clientConn.Close()

		_ = backendConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 256)
		_, readErr := backendConn.Read(buf)
		elapsed := time.Since(closeStart)

		if readErr == nil {
			t.Fatal("expected upstream to observe socket closure, got nil error")
		}
		if elapsed > 300*time.Millisecond {
			t.Errorf("expected closure within 100-300ms, took %v", elapsed)
		}
	})

	t.Run("Subtest 4B: Upstream Disconnection Triggers Immediate Client Closure", func(t *testing.T) {
		clientConn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer clientConn.Close()

		reqStr := "GET /ws-disconnect HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
		_, _ = clientConn.Write([]byte(reqStr))

		respBuf := make([]byte, 512)
		_, _ = clientConn.Read(respBuf)

		backendConn := <-upstreamAcceptedCh

		closeStart := time.Now()
		_ = backendConn.Close()

		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 256)
		_, readErr := clientConn.Read(buf)
		elapsed := time.Since(closeStart)

		if readErr == nil {
			t.Fatal("expected client to observe socket closure, got nil error")
		}
		if elapsed > 300*time.Millisecond {
			t.Errorf("expected closure within 100-300ms, took %v", elapsed)
		}
	})
}

// TC-088-05: HTTP/1.1 Half-Close Propagation
func TestServer_UpgradedConn_HalfClose(t *testing.T) {
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
		if err != nil {
			return
		}
		if string(buf[:n]) != "query-payload" {
			return
		}

		// Subsequent read should return EOF due to half-close
		_, err = conn.Read(buf)
		if !errors.Is(err, io.EOF) {
			return
		}

		// Pause 60ms to simulate response processing
		time.Sleep(60 * time.Millisecond)

		// Transmit final response data
		_, _ = conn.Write([]byte("final-response-data"))
	}()

	r := router.New()
	r.GET("/ws-halfclose", func(req *httpparser.Request, res *httpparser.Response) {
		upstreamConn, err := net.Dial("tcp", upstreamLn.Addr().String())
		if err != nil {
			res.SetStatus(http.StatusBadGateway)
			return
		}
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
		res.Header.Set("Connection", "Upgrade")
		res.UpgradedConn = upstreamConn
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.UpgradeIdleTimeout = 300 * time.Millisecond

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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /ws-halfclose HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write upgrade: %v", err)
	}

	respBuf := make([]byte, 512)
	n, err := conn.Read(respBuf)
	if err != nil || !bytes.Contains(respBuf[:n], []byte("101 Switching Protocols")) {
		t.Fatalf("upgrade failed: %v", err)
	}

	// Client writes query-payload
	if _, err := conn.Write([]byte("query-payload")); err != nil {
		t.Fatalf("failed to write query payload: %v", err)
	}

	// Client invokes CloseWrite()
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Fatalf("connection is not *net.TCPConn")
	}
	if err := tcpConn.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite failed: %v", err)
	}

	// Client continues reading with 1-second deadline
	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	dataBuf := make([]byte, 512)
	n, err = conn.Read(dataBuf)
	if err != nil {
		t.Fatalf("client failed to read final response data: %v", err)
	}
	if string(dataBuf[:n]) != "final-response-data" {
		t.Fatalf("expected 'final-response-data', got %q", string(dataBuf[:n]))
	}

	// Subsequent read should yield EOF
	_, err = conn.Read(dataBuf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF after final response data, got: %v", err)
	}
}

// TC-088-06: HTTP/2 Extended CONNECT Idle Teardown & Context Cancellation
func TestServer_HTTP2_ExtendedCONNECT_IdleAndCancel(t *testing.T) {
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen upstream: %v", err)
	}
	defer upstreamLn.Close()

	go func() {
		for {
			c, err := upstreamLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 1024)
				for {
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					_, _ = conn.Write(buf[:n])
				}
			}(c)
		}
	}()

	r := router.New()
	r.Handle("CONNECT", "/ws-h2", func(req *httpparser.Request, res *httpparser.Response) {
		upstreamConn, err := net.Dial("tcp", upstreamLn.Addr().String())
		if err != nil {
			res.SetStatus(http.StatusBadGateway)
			return
		}
		res.SetStatus(http.StatusOK)
		res.UpgradedConn = upstreamConn
	})

	cfg := server.DefaultConfig()
	cfg.UpgradeIdleTimeout = 150 * time.Millisecond
	cfg.HTTP2Enabled = true
	srv := server.New(cfg, r)

	t.Run("Subtest 6A: HTTP/2 Inactivity Timeout", func(t *testing.T) {
		idlePipeReader, idlePipeWriter := io.Pipe()
		defer idlePipeWriter.Close()

		req := httptest.NewRequest("CONNECT", "/ws-h2", idlePipeReader)
		req.Header.Set(":protocol", "websocket")
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		start := time.Now()
		go func() {
			srv.HTTP2AdapterHandler().ServeHTTP(rec, req)
			close(done)
		}()

		select {
		case <-done:
			duration := time.Since(start)
			if duration < 120*time.Millisecond || duration > 400*time.Millisecond {
				t.Errorf("expected stream termination in 150ms ± 50ms, took %v", duration)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("HTTP/2 stream did not terminate within timeout window")
		}
	})

	t.Run("Subtest 6B: HTTP/2 Context Cancellation Immediate Teardown", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		pipeReader, pipeWriter := io.Pipe()
		defer pipeWriter.Close()

		req := httptest.NewRequest("CONNECT", "/ws-h2", pipeReader).WithContext(ctx)
		req.Header.Set(":protocol", "websocket")
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		start := time.Now()
		go func() {
			srv.HTTP2AdapterHandler().ServeHTTP(rec, req)
			close(done)
		}()

		// Immediately cancel context
		cancel()

		select {
		case <-done:
			duration := time.Since(start)
			if duration > 100*time.Millisecond {
				t.Errorf("expected teardown <= 100ms, took %v", duration)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("HTTP/2 stream did not terminate upon context cancellation")
		}
	})
}

// TC-088-07: Concurrency & Race Safety
func TestServer_UpgradedConn_ConcurrencyRaceSafety(t *testing.T) {
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen upstream: %v", err)
	}
	defer upstreamLn.Close()

	go func() {
		for {
			c, err := upstreamLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 2048)
				for {
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					_, _ = conn.Write(buf[:n])
				}
			}(c)
		}
	}()

	r := router.New()
	r.GET("/ws-race", func(req *httpparser.Request, res *httpparser.Response) {
		upstreamConn, err := net.Dial("tcp", upstreamLn.Addr().String())
		if err != nil {
			res.SetStatus(http.StatusBadGateway)
			return
		}
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
		res.Header.Set("Connection", "Upgrade")
		res.UpgradedConn = upstreamConn
	})
	r.Handle("CONNECT", "/ws-race-h2", func(req *httpparser.Request, res *httpparser.Response) {
		upstreamConn, err := net.Dial("tcp", upstreamLn.Addr().String())
		if err != nil {
			res.SetStatus(http.StatusBadGateway)
			return
		}
		res.SetStatus(http.StatusOK)
		res.UpgradedConn = upstreamConn
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.UpgradeIdleTimeout = 100 * time.Millisecond
	cfg.WorkerPoolSize = 64
	cfg.HTTP2Enabled = true

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

	var wg sync.WaitGroup

	// Group 1: 10 goroutines with active heartbeats
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()

			reqStr := "GET /ws-race HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
			_, _ = conn.Write([]byte(reqStr))
			resp := make([]byte, 256)
			_, _ = conn.Read(resp)

			for p := 0; p < 8; p++ {
				_, _ = conn.Write([]byte(fmt.Sprintf("race-ping-%d-%d", id, p)))
				buf := make([]byte, 64)
				_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				_, _ = conn.Read(buf)
				time.Sleep(30 * time.Millisecond)
			}
		}(i)
	}

	// Group 2: 10 goroutines completely silent (expect idle timeout)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()

			reqStr := "GET /ws-race HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
			_, _ = conn.Write([]byte(reqStr))
			resp := make([]byte, 256)
			_, _ = conn.Read(resp)

			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			buf := make([]byte, 64)
			_, _ = conn.Read(buf)
		}()
	}

	// Group 3: 10 goroutines abruptly closing or half-closing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()

			reqStr := "GET /ws-race HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
			_, _ = conn.Write([]byte(reqStr))
			resp := make([]byte, 256)
			_, _ = conn.Read(resp)

			_, _ = conn.Write([]byte("quick-data"))
			if id%2 == 0 {
				if tcp, ok := conn.(*net.TCPConn); ok {
					_ = tcp.CloseWrite()
				}
			} else {
				_ = conn.Close()
			}
		}(i)
	}

	// 10 HTTP/2 extended CONNECT streams with random context cancellations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			pr, pw := io.Pipe()
			defer pw.Close()

			req := httptest.NewRequest("CONNECT", "/ws-race-h2", pr).WithContext(ctx)
			req.Header.Set(":protocol", "websocket")
			rec := httptest.NewRecorder()

			go func() {
				time.Sleep(time.Duration(10+id*5) * time.Millisecond)
				cancel()
			}()

			srv.HTTP2AdapterHandler().ServeHTTP(rec, req)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines completed cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent test timed out after 5s")
	}
}

// TC-107-02 & TC-107-03: Response-Driven Connection Teardown and Benign Keep-Alive Preservation
func TestServer_ResponseConnectionClose_SocketTeardown(t *testing.T) {
	r := router.New()

	// Route that explicitly terminates connection via response header
	r.GET("/close-me", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusForbidden)
		res.Header.Set("Connection", "close")
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"error":"403 Forbidden"}`)
	})

	// Benign route that keeps connection open
	r.GET("/keep-alive", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("hello keepalive")
	})

	cfg := server.DefaultConfig()
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
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

	// TC-107-02: Server-initiated teardown when client sent keep-alive
	t.Run("TC-107-02: Response Connection Close Teardown", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		req := "GET /close-me HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n"
		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatalf("failed to write: %v", err)
		}

		respBytes, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}

		respStr := string(respBytes)
		if !strings.Contains(respStr, "403 Forbidden") {
			t.Errorf("expected 403 Forbidden, got:\n%s", respStr)
		}
		if !strings.Contains(strings.ToLower(respStr), "connection: close") {
			t.Errorf("expected Connection: close, got:\n%s", respStr)
		}
		// io.ReadAll terminated with EOF, proving the physical TCP socket was closed by the server
	})

	// TC-107-03: Benign requests preserve keep-alive connection across multiple requests
	t.Run("TC-107-03: Benign Keep-Alive Preserved", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)

		// Request 1
		req1 := "GET /keep-alive HTTP/1.1\r\nHost: localhost\r\n\r\n"
		if _, err := conn.Write([]byte(req1)); err != nil {
			t.Fatalf("write 1 failed: %v", err)
		}

		status1, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read status 1 failed: %v", err)
		}
		if !strings.Contains(status1, "200") {
			t.Fatalf("expected 200, got: %s", status1)
		}

		// Drain headers and body for request 1
		for {
			line, err := reader.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
		}
		bodyBuf1 := make([]byte, len("hello keepalive"))
		_, _ = io.ReadFull(reader, bodyBuf1)

		// Request 2 on the EXACT SAME TCP socket
		req2 := "GET /keep-alive HTTP/1.1\r\nHost: localhost\r\n\r\n"
		if _, err := conn.Write([]byte(req2)); err != nil {
			t.Fatalf("write 2 failed: %v", err)
		}

		status2, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read status 2 failed (socket closed prematurely): %v", err)
		}
		if !strings.Contains(status2, "200") {
			t.Fatalf("expected 200 on request 2, got: %s", status2)
		}
	})
}
