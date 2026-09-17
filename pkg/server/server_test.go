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
	cfg.InboundChunkedMode = "reject"
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
	cfg.WorkerPoolSize = 8
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

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	baselineGoroutines := runtime.NumGoroutine()

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
	if elapsed < 90*time.Millisecond || elapsed > 400*time.Millisecond {
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

// TC-108-01 through TC-108-04: HTTP/1.1 Pipelining and Persistent Connection Tests
func TestServer_HTTP1Pipelining(t *testing.T) {
	r := router.New()

	r.GET("/pipe1", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("response-1")
	})

	r.GET("/pipe2", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("response-2")
	})

	r.GET("/pipe3", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("response-3")
	})

	r.POST("/upload", func(req *httpparser.Request, res *httpparser.Response) {
		body, _ := io.ReadAll(req.Body)
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("received:" + string(body))
	})

	r.GET("/status", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("status-ok")
	})

	var upstreamPipeEnd net.Conn
	var pipeMu sync.Mutex
	r.GET("/upgrade", func(req *httpparser.Request, res *httpparser.Response) {
		c1, c2 := net.Pipe()
		pipeMu.Lock()
		upstreamPipeEnd = c2
		pipeMu.Unlock()
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
		res.Header.Set("Connection", "Upgrade")
		res.UpgradedConn = c1
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.ReadTimeout = 2 * time.Second
	cfg.IdleTimeout = 2 * time.Second

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

	addr := ln.Addr().String()

	readResponse := func(reader *bufio.Reader) (string, int, string, error) {
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			return "", 0, "", err
		}
		var statusCode int
		if strings.Contains(statusLine, "200") {
			statusCode = 200
		} else if strings.Contains(statusLine, "101") {
			statusCode = 101
		} else {
			statusCode = 400
		}

		headers := make(http.Header)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return statusLine, statusCode, "", err
			}
			lineTrimmed := strings.TrimRight(line, "\r\n")
			if lineTrimmed == "" {
				break
			}
			parts := strings.SplitN(lineTrimmed, ":", 2)
			if len(parts) == 2 {
				headers.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}

		clStr := headers.Get("Content-Length")
		var body string
		if clStr != "" {
			var cl int
			_, _ = fmt.Sscanf(clStr, "%d", &cl)
			if cl > 0 {
				bodyBytes := make([]byte, cl)
				_, err = io.ReadFull(reader, bodyBytes)
				if err != nil {
					return statusLine, statusCode, "", err
				}
				body = string(bodyBytes)
			}
		}
		return headers.Get("Connection"), statusCode, body, nil
	}

	// TC-108-01: Dual and Multi-Request GET Pipelining in single write
	t.Run("TC-108-01: Multi-Request GET Pipelining", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		pipelinedPayload := "GET /pipe1 HTTP/1.1\r\nHost: localhost\r\n\r\n" +
			"GET /pipe2 HTTP/1.1\r\nHost: localhost\r\n\r\n" +
			"GET /pipe3 HTTP/1.1\r\nHost: localhost\r\n\r\n"

		if _, err := conn.Write([]byte(pipelinedPayload)); err != nil {
			t.Fatalf("failed to write pipelined requests: %v", err)
		}

		reader := bufio.NewReader(conn)

		// Response 1
		_, code1, body1, err := readResponse(reader)
		if err != nil {
			t.Fatalf("failed to read response 1: %v", err)
		}
		if code1 != 200 || body1 != "response-1" {
			t.Fatalf("response 1 mismatch: got code %d, body %q", code1, body1)
		}

		// Response 2
		_, code2, body2, err := readResponse(reader)
		if err != nil {
			t.Fatalf("failed to read response 2: %v", err)
		}
		if code2 != 200 || body2 != "response-2" {
			t.Fatalf("response 2 mismatch: got code %d, body %q", code2, body2)
		}

		// Response 3
		_, code3, body3, err := readResponse(reader)
		if err != nil {
			t.Fatalf("failed to read response 3: %v", err)
		}
		if code3 != 200 || body3 != "response-3" {
			t.Fatalf("response 3 mismatch: got code %d, body %q", code3, body3)
		}
	})

	// TC-108-02: Pipelined POST with Body Followed by GET
	t.Run("TC-108-02: Pipelined POST with Body Followed by GET", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		postAndGet := "POST /upload HTTP/1.1\r\nHost: localhost\r\nContent-Length: 11\r\n\r\nhello-world" +
			"GET /status HTTP/1.1\r\nHost: localhost\r\n\r\n"

		if _, err := conn.Write([]byte(postAndGet)); err != nil {
			t.Fatalf("failed to write POST + GET: %v", err)
		}

		reader := bufio.NewReader(conn)

		// Response 1 (POST)
		_, code1, body1, err := readResponse(reader)
		if err != nil {
			t.Fatalf("failed to read POST response: %v", err)
		}
		if code1 != 200 || body1 != "received:hello-world" {
			t.Fatalf("POST response mismatch: got code %d, body %q", code1, body1)
		}

		// Response 2 (GET)
		_, code2, body2, err := readResponse(reader)
		if err != nil {
			t.Fatalf("failed to read GET response: %v", err)
		}
		if code2 != 200 || body2 != "status-ok" {
			t.Fatalf("GET response mismatch: got code %d, body %q", code2, body2)
		}
	})

	// TC-108-03: Pipelined Request with Connection Close
	t.Run("TC-108-03: Pipelined Request with Connection Close", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		reqWithClose := "GET /pipe1 HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n" +
			"GET /pipe2 HTTP/1.1\r\nHost: localhost\r\n\r\n"

		if _, err := conn.Write([]byte(reqWithClose)); err != nil {
			t.Fatalf("failed to write: %v", err)
		}

		reader := bufio.NewReader(conn)

		connHdr, code1, body1, err := readResponse(reader)
		if err != nil {
			t.Fatalf("failed to read response 1: %v", err)
		}
		if code1 != 200 || body1 != "response-1" {
			t.Fatalf("response 1 mismatch: got code %d, body %q", code1, body1)
		}
		if !strings.EqualFold(connHdr, "close") {
			t.Errorf("expected Connection: close header, got %q", connHdr)
		}

		// Socket must now be closed by the server; subsequent read should yield EOF
		_, err = reader.ReadByte()
		if err == nil {
			t.Fatalf("expected EOF after Connection: close, but socket is still readable")
		}
		if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "closed") {
			t.Logf("connection closed as expected with: %v", err)
		}
	})

	// TC-108-04: Protocol Upgrade Handover with Residual Buffered Frames
	t.Run("TC-108-04: Protocol Upgrade Handover with Residual Frames", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		upgradeWithFrame := "GET /upgrade HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n" +
			"residual-websocket-data"

		if _, err := conn.Write([]byte(upgradeWithFrame)); err != nil {
			t.Fatalf("failed to write upgrade: %v", err)
		}

		reader := bufio.NewReader(conn)

		// Read 101 Switching Protocols response
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read status: %v", err)
		}
		if !strings.Contains(statusLine, "101") {
			t.Fatalf("expected 101, got: %s", statusLine)
		}
		for {
			line, err := reader.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
		}

		// Upstream pipe end should receive "residual-websocket-data"
		pipeMu.Lock()
		upstream := upstreamPipeEnd
		pipeMu.Unlock()

		if upstream == nil {
			t.Fatalf("upstream pipe was not initialized")
		}
		defer upstream.Close()

		received := make([]byte, len("residual-websocket-data"))
		_, err = io.ReadFull(upstream, received)
		if err != nil {
			t.Fatalf("failed to read residual data on upstream: %v", err)
		}
		if string(received) != "residual-websocket-data" {
			t.Fatalf("expected 'residual-websocket-data', got %q", string(received))
		}
	})
}

// TC-112-01: H2.TE Rejection (Transfer-Encoding in HTTP/2)
func TestServer_HTTP2_TransferEncodingRejected(t *testing.T) {
	r := router.New()
	r.POST("/test", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	httpReq := httptest.NewRequest("POST", "/test", strings.NewReader("body"))
	httpReq.Header.Set("Transfer-Encoding", "chunked")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for HTTP/2 with Transfer-Encoding, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "400 Bad Request") {
		t.Errorf("expected 400 Bad Request error body, got %q", rec.Body.String())
	}
}

// TC-112-02: H2.CL Multiple / Conflicting Content-Length Rejection
func TestServer_HTTP2_MultipleContentLengthRejected(t *testing.T) {
	r := router.New()
	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	t.Run("duplicate_headers", func(t *testing.T) {
		httpReq := httptest.NewRequest("POST", "/test", strings.NewReader("hello"))
		httpReq.Header["Content-Length"] = []string{"5", "10"}
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for duplicate Content-Length, got %d", rec.Code)
		}
	})

	t.Run("comma_separated_values", func(t *testing.T) {
		httpReq := httptest.NewRequest("POST", "/test", strings.NewReader("hello"))
		httpReq.Header.Set("Content-Length", "5, 10")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for comma-separated Content-Length, got %d", rec.Code)
		}
	})

	t.Run("non_numeric_value", func(t *testing.T) {
		httpReq := httptest.NewRequest("POST", "/test", strings.NewReader("hello"))
		httpReq.Header.Set("Content-Length", "five")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for non-numeric Content-Length, got %d", rec.Code)
		}
	})
}

// TC-112-03: H2.CL Payload Length Discrepancy Rejection
func TestServer_HTTP2_ContentLengthMismatchRejected(t *testing.T) {
	r := router.New()
	r.POST("/test", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	// Declared length 20, but body only provides 5 bytes
	httpReq := httptest.NewRequest("POST", "/test", strings.NewReader("hello"))
	httpReq.Header.Set("Content-Length", "20")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for Content-Length mismatch, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Content-Length") {
		t.Errorf("expected Content-Length mismatch error, got %q", rec.Body.String())
	}
}

// TC-112-04: CRLF and NUL Binary Injection Rejection
func TestServer_HTTP2_CRLFInjectionRejected(t *testing.T) {
	r := router.New()
	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	testCases := []struct {
		name  string
		setup func(req *http.Request)
	}{
		{
			name: "crlf_in_header_value",
			setup: func(req *http.Request) {
				req.Header.Set("X-Custom", "value\r\nInjected: evil")
			},
		},
		{
			name: "crlf_in_path",
			setup: func(req *http.Request) {
				req.URL.Path = "/api\r\nGET /smuggled"
			},
		},
		{
			name: "nul_in_header_value",
			setup: func(req *http.Request) {
				req.Header.Set("X-Custom", "evil\x00data")
			},
		},
		{
			name: "crlf_in_query",
			setup: func(req *http.Request) {
				req.URL.RawQuery = "param=safe\r\nHost: evil.com"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpReq := httptest.NewRequest("GET", "/test", nil)
			tc.setup(httpReq)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, httpReq)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("[%s] expected 400 Bad Request, got %d", tc.name, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "400 Bad Request") {
				t.Errorf("[%s] expected 400 Bad Request error body, got %q", tc.name, rec.Body.String())
			}
		})
	}
}

// TC-112-05: Forbidden Connection Headers Rejection
func TestServer_HTTP2_ForbiddenConnectionHeadersRejected(t *testing.T) {
	r := router.New()
	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	forbiddenHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Connection",
		"Upgrade",
	}

	for _, hdr := range forbiddenHeaders {
		t.Run(hdr, func(t *testing.T) {
			httpReq := httptest.NewRequest("GET", "/test", nil)
			httpReq.Header.Set(hdr, "close")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, httpReq)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request for forbidden header %s, got %d", hdr, rec.Code)
			}
		})
	}
}

// TC-112-06: RFC 8441 Extended CONNECT Compatibility
func TestServer_HTTP2_ExtendedConnectAllowed(t *testing.T) {
	r := router.New()
	r.Handle("CONNECT", "/chat", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusSwitchingProtocols)
	})

	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	httpReq := httptest.NewRequest("CONNECT", "/chat", nil)
	httpReq.Header.Set(":protocol", "websocket")
	httpReq.Header.Set("Upgrade", "websocket")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	// In RFC 8441, switching protocols becomes 200 OK in http2AdapterHandler
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid extended CONNECT, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// TC-125.6: Server Socket Streaming Relay & Activity-Refreshed Deadlines
func TestServer_StreamingRelay_ActivityRefreshedDeadlines(t *testing.T) {
	r := router.New()

	closedCh := make(chan struct{})
	r.GET("/sse-feed", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")

		pr, pw := io.Pipe()
		res.StreamBody = struct {
			io.Reader
			io.Closer
		}{
			Reader: pr,
			Closer: io.Closer(closerFunc(func() error {
				close(closedCh)
				return pr.Close()
			})),
		}

		go func() {
			defer pw.Close()
			for i := 1; i <= 3; i++ {
				_, _ = fmt.Fprintf(pw, "event: msg%d\ndata: tick\n\n", i)
				time.Sleep(100 * time.Millisecond)
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.WriteTimeout = 150 * time.Millisecond
	cfg.UpgradeIdleTimeout = 250 * time.Millisecond

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	start := time.Now()
	_, err = fmt.Fprintf(conn, "GET /sse-feed HTTP/1.1\r\nHost: localhost\r\n\r\n")
	if err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	reader := bufio.NewReader(conn)
	// Read status line
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}
	ttfb := time.Since(start)
	if ttfb > 100*time.Millisecond {
		t.Errorf("expected fast TTFB, took %v", ttfb)
	}
	if !strings.Contains(statusLine, "200 OK") {
		t.Fatalf("expected 200 OK, got: %s", statusLine)
	}

	// Read headers
	headers := make(http.Header)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading header line: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	if headers.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", headers.Get("Content-Type"))
	}
	if headers.Get("Content-Length") != "" {
		t.Errorf("expected Content-Length to be absent, got %q", headers.Get("Content-Length"))
	}

	// Read all chunks
	var body bytes.Buffer
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			body.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	bodyStr := body.String()
	for i := 1; i <= 3; i++ {
		expected := fmt.Sprintf("event: msg%d\ndata: tick\n\n", i)
		if !strings.Contains(bodyStr, expected) {
			t.Errorf("expected body to contain %q, got: %s", expected, bodyStr)
		}
	}

	select {
	case <-closedCh:
		// Upstream stream body closed cleanly
	case <-time.After(500 * time.Millisecond):
		t.Errorf("expected StreamBody to be closed upon completion")
	}
}

// TC-125.7: Slow-Read DoS Defense (Slow Client Disconnect)
func TestServer_StreamingRelay_SlowReadClientDisconnect(t *testing.T) {
	r := router.New()

	closedCh := make(chan struct{})
	r.GET("/infinite-stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")

		pr, pw := io.Pipe()
		res.StreamBody = struct {
			io.Reader
			io.Closer
		}{
			Reader: pr,
			Closer: io.Closer(closerFunc(func() error {
				select {
				case <-closedCh:
				default:
					close(closedCh)
				}
				return pr.Close()
			})),
		}

		go func() {
			defer pw.Close()
			chunk := bytes.Repeat([]byte("A"), 16*1024)
			for {
				if _, err := pw.Write(chunk); err != nil {
					return
				}
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.WriteTimeout = 150 * time.Millisecond
	cfg.UpgradeIdleTimeout = 150 * time.Millisecond

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetReadBuffer(1024)
	}

	_, err = fmt.Fprintf(conn, "GET /infinite-stream HTTP/1.1\r\nHost: localhost\r\n\r\n")
	if err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Read small initial slice
	initBuf := make([]byte, 256)
	_, _ = conn.Read(initBuf)

	// Now client halts reading!
	// Toron server will fill socket buffer and timeout after ~150ms write deadline
	select {
	case <-closedCh:
		// StreamBody was closed because server aborted on write deadline!
	case <-time.After(2 * time.Second):
		t.Errorf("expected StreamBody.Close() within deadline after client halted read")
	}
}

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}

// --- TC-126: Socket Deadline Amortization & Streaming Timeout Suites ---

type countingListener struct {
	net.Listener
	mu          sync.Mutex
	activeConns []*countingConn
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	cc := &countingConn{Conn: c}
	l.mu.Lock()
	l.activeConns = append(l.activeConns, cc)
	l.mu.Unlock()
	return cc, nil
}

type countingConn struct {
	net.Conn
	readDeadlines  atomic.Uint64
	writeDeadlines atomic.Uint64
}

func (c *countingConn) SetReadDeadline(t time.Time) error {
	c.readDeadlines.Add(1)
	return c.Conn.SetReadDeadline(t)
}

func (c *countingConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadlines.Add(1)
	return c.Conn.SetWriteDeadline(t)
}

func (c *countingConn) SetDeadline(t time.Time) error {
	c.readDeadlines.Add(1)
	c.writeDeadlines.Add(1)
	return c.Conn.SetDeadline(t)
}

// TC-126.1: Deadline Amortization & Syscall Reduction Verification (>80% required, verify >99% on bursts)
func TestServer_DeadlineAmortization_SyscallReduction(t *testing.T) {
	r := router.New()
	r.GET("/burst", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("OK")
	})

	cfg := server.DefaultConfig()
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	cfg.IdleTimeout = 30 * time.Second
	cfg.HTTP2Enabled = false

	srv := server.New(cfg, r)
	rawLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer rawLn.Close()

	countingLn := &countingListener{Listener: rawLn}
	go func() {
		_ = srv.Serve(countingLn)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	clientConn, err := net.Dial("tcp", rawLn.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	reqBytes := []byte("GET /burst HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n")
	reader := bufio.NewReader(clientConn)

	start := time.Now()
	numRequests := 1000
	for i := 0; i < numRequests; i++ {
		if _, err := clientConn.Write(reqBytes); err != nil {
			t.Fatalf("request %d write failed: %v", i, err)
		}
		resp, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatalf("request %d read failed: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK on request %d, got %d", i, resp.StatusCode)
		}
	}
	duration := time.Since(start)

	countingLn.mu.Lock()
	var totalRead, totalWrite uint64
	for _, cc := range countingLn.activeConns {
		totalRead += cc.readDeadlines.Load()
		totalWrite += cc.writeDeadlines.Load()
	}
	countingLn.mu.Unlock()

	if duration > 2500*time.Millisecond {
		t.Logf("warning: 1000 requests took %v (> 2.5s)", duration)
	}

	if totalRead > 5 {
		t.Errorf("expected total SetReadDeadline <= 5, got %d", totalRead)
	}
	if totalWrite > 5 {
		t.Errorf("expected total SetWriteDeadline <= 5, got %d", totalWrite)
	}

	reductionRead := float64(numRequests-int(totalRead)) / float64(numRequests) * 100.0
	reductionWrite := float64(numRequests-int(totalWrite)) / float64(numRequests) * 100.0

	if reductionRead < 80.0 {
		t.Errorf("expected read syscall reduction > 80%%, got %.2f%%", reductionRead)
	}
	if reductionWrite < 80.0 {
		t.Errorf("expected write syscall reduction > 80%%, got %.2f%%", reductionWrite)
	}
}

// TC-126.3: Strict Idle Timeout State Machine Alignment (br.Buffered() == 0 Reset)
func TestServer_IdleTimeout_StrictEnforcement(t *testing.T) {
	r := router.New()
	r.GET("/ping", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("pong")
	})

	cfg := server.DefaultConfig()
	cfg.ReadTimeout = 10 * time.Second
	cfg.IdleTimeout = 200 * time.Millisecond
	cfg.HTTP2Enabled = false

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	reqStr := "GET /ping HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	_ = resp.Body.Close()

	// Connection is now idle waiting for next request. Client halts sending.
	start := time.Now()
	buf := make([]byte, 128)
	_, readErr := conn.Read(buf)
	duration := time.Since(start)

	if readErr == nil {
		t.Fatalf("expected EOF or error on idle timeout, got data")
	}

	// Must disconnect around 200ms (+ CI tolerance), proving 10s ReadTimeout didn't bleed into idle
	if duration < 150*time.Millisecond || duration > 600*time.Millisecond {
		t.Errorf("expected idle disconnect in ~200ms, took %v", duration)
	}
}

// TC-126.4: Long-Lived Streaming Survival (> 10s or past WriteTimeout)
func TestServer_Streaming_SurvivesPastWriteTimeout(t *testing.T) {
	r := router.New()

	closedCh := make(chan struct{})
	r.GET("/sse-long", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")

		pr, pw := io.Pipe()
		res.StreamBody = struct {
			io.Reader
			io.Closer
		}{
			Reader: pr,
			Closer: io.Closer(closerFunc(func() error {
				select {
				case <-closedCh:
				default:
					close(closedCh)
				}
				return pr.Close()
			})),
		}

		go func() {
			defer pw.Close()
			// Emit 6 chunks spaced 400ms apart (total 2.4s > 800ms WriteTimeout)
			for i := 1; i <= 6; i++ {
				chunk := fmt.Sprintf("event: msg%d\ndata: tick\n\n", i)
				if _, err := pw.Write([]byte(chunk)); err != nil {
					return
				}
				time.Sleep(400 * time.Millisecond)
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.WriteTimeout = 800 * time.Millisecond
	cfg.IdleTimeout = 5 * time.Second
	cfg.HTTP2Enabled = false

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	if _, err := fmt.Fprintf(conn, "GET /sse-long HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatalf("failed to send request: %v", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("failed to read response headers: %v", err)
	}

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	var chunksReceived []string
	chunkBuf := make([]byte, 256)
	for {
		n, err := resp.Body.Read(chunkBuf)
		if n > 0 {
			chunksReceived = append(chunksReceived, string(chunkBuf[:n]))
		}
		if err != nil {
			break
		}
	}
	_ = resp.Body.Close()

	fullBody := strings.Join(chunksReceived, "")
	for i := 1; i <= 6; i++ {
		expected := fmt.Sprintf("event: msg%d\ndata: tick\n\n", i)
		if !strings.Contains(fullBody, expected) {
			t.Errorf("expected chunk %d in body, got: %s", i, fullBody)
		}
	}

	select {
	case <-closedCh:
	case <-time.After(1 * time.Second):
		t.Fatalf("expected StreamBody.Close() to be called")
	}
}

// TC-126.5: Slow-Read Client Teardown / CWE-400 Mitigation
func TestServer_Streaming_SlowReadClientTerminated(t *testing.T) {
	r := router.New()

	closedCh := make(chan struct{})
	r.GET("/infinite-stream-cwe400", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")

		pr, pw := io.Pipe()
		res.StreamBody = struct {
			io.Reader
			io.Closer
		}{
			Reader: pr,
			Closer: io.Closer(closerFunc(func() error {
				select {
				case <-closedCh:
				default:
					close(closedCh)
				}
				return pr.Close()
			})),
		}

		go func() {
			defer pw.Close()
			chunk := bytes.Repeat([]byte("X"), 32*1024)
			for {
				if _, err := pw.Write(chunk); err != nil {
					return
				}
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.WriteTimeout = 200 * time.Millisecond
	cfg.HTTP2Enabled = false

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetReadBuffer(1024)
	}

	if _, err := fmt.Fprintf(conn, "GET /infinite-stream-cwe400 HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatalf("failed to send request: %v", err)
	}

	initBuf := make([]byte, 256)
	_, _ = conn.Read(initBuf)

	// Halt reading: socket send buffer fills, write deadline expires
	select {
	case <-closedCh:
		// Succeeded: Server aborted on write timeout and closed stream
	case <-time.After(2 * time.Second):
		t.Fatalf("expected server to terminate stalled client within write timeout")
	}
}

// Helper for TCP pipe pair
func tcpPipe() (net.Conn, net.Conn, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer ln.Close()

	ch := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		ch <- c
	}()

	c1, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	select {
	case c2 := <-ch:
		return c1, c2, nil
	case err := <-errCh:
		_ = c1.Close()
		return nil, nil, err
	case <-time.After(2 * time.Second):
		_ = c1.Close()
		return nil, nil, fmt.Errorf("tcpPipe timeout")
	}
}

// TC-126.6: Bidirectional Stream Relaying Amortization (relayStreams)
func TestServer_RelayStreams_AmortizationAndIdle(t *testing.T) {
	t.Run("Subtest 6A (Syscall Amortization under High-Rate Bidirectional Data)", func(t *testing.T) {
		connA1, connA2, err := tcpPipe()
		if err != nil {
			t.Fatalf("failed tcpPipe A: %v", err)
		}
		defer connA1.Close()
		defer connA2.Close()

		connB1, connB2, err := tcpPipe()
		if err != nil {
			t.Fatalf("failed tcpPipe B: %v", err)
		}
		defer connB1.Close()
		defer connB2.Close()

		trackerA := server.NewConnDeadlineTracker(connA1)
		trackerB := server.NewConnDeadlineTracker(connB1)

		go server.RelayStreams(trackerA, trackerB, 200*time.Millisecond)

		var wgRelay sync.WaitGroup
		wgRelay.Add(2)

		// Reader on B2
		go func() {
			defer wgRelay.Done()
			buf := make([]byte, 64)
			for i := 0; i < 500; i++ {
				_, err := io.ReadFull(connB2, buf)
				if err != nil {
					t.Errorf("read B2 error at %d: %v", i, err)
					return
				}
			}
		}()

		// Reader on A2
		go func() {
			defer wgRelay.Done()
			buf := make([]byte, 64)
			for i := 0; i < 500; i++ {
				_, err := io.ReadFull(connA2, buf)
				if err != nil {
					t.Errorf("read A2 error at %d: %v", i, err)
					return
				}
			}
		}()

		// Writer on A2
		msgA := bytes.Repeat([]byte("A"), 64)
		for i := 0; i < 500; i++ {
			if _, err := connA2.Write(msgA); err != nil {
				t.Fatalf("write A2 error at %d: %v", i, err)
			}
		}

		// Writer on B2
		msgB := bytes.Repeat([]byte("B"), 64)
		for i := 0; i < 500; i++ {
			if _, err := connB2.Write(msgB); err != nil {
				t.Fatalf("write B2 error at %d: %v", i, err)
			}
		}

		wgRelay.Wait()

		rA, wA := trackerA.SyscallCounts()
		rB, wB := trackerB.SyscallCounts()

		if rA > 2 {
			t.Errorf("expected trackerA read syscalls <= 2, got %d", rA)
		}
		if wA > 2 {
			t.Errorf("expected trackerA write syscalls <= 2, got %d", wA)
		}
		if rB > 2 {
			t.Errorf("expected trackerB read syscalls <= 2, got %d", rB)
		}
		if wB > 2 {
			t.Errorf("expected trackerB write syscalls <= 2, got %d", wB)
		}
	})

	t.Run("Subtest 6B (Inactivity Idle Disconnect)", func(t *testing.T) {
		connA1, connA2, err := tcpPipe()
		if err != nil {
			t.Fatalf("failed tcpPipe A: %v", err)
		}
		defer connA2.Close()
		connB1, connB2, err := tcpPipe()
		if err != nil {
			t.Fatalf("failed tcpPipe B: %v", err)
		}
		defer connB2.Close()

		go server.RelayStreams(connA1, connB1, 200*time.Millisecond)

		// Initial handshake
		_, _ = connA2.Write([]byte("ping"))
		b := make([]byte, 4)
		_, _ = io.ReadFull(connB2, b)

		// Now halt both sides
		start := time.Now()
		buf := make([]byte, 64)
		_, errA := connA2.Read(buf)
		elapsed := time.Since(start)

		if errA == nil {
			t.Fatalf("expected EOF on connA2 after idle timeout")
		}
		if elapsed < 150*time.Millisecond || elapsed > 600*time.Millisecond {
			t.Errorf("expected idle disconnect in ~200ms, took %v", elapsed)
		}
	})

	t.Run("Subtest 6C (Heartbeat Stream Survival)", func(t *testing.T) {
		connA1, connA2, err := tcpPipe()
		if err != nil {
			t.Fatalf("failed tcpPipe A: %v", err)
		}
		defer connA2.Close()
		connB1, connB2, err := tcpPipe()
		if err != nil {
			t.Fatalf("failed tcpPipe B: %v", err)
		}
		defer connB2.Close()

		go server.RelayStreams(connA1, connB1, 200*time.Millisecond)

		stopPing := make(chan struct{})
		go func() {
			ticker := time.NewTicker(70 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if _, err := connA2.Write([]byte("P")); err != nil {
						return
					}
				case <-stopPing:
					return
				}
			}
		}()

		// Read pings for 500ms (> 2 full idle timeouts of 200ms)
		readBuf := make([]byte, 1)
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			_ = connB2.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			_, err := connB2.Read(readBuf)
			if err != nil {
				t.Fatalf("unexpected read error during heartbeat window: %v", err)
			}
		}

		// Stop heartbeats and verify disconnection within ~200ms
		close(stopPing)
		start := time.Now()
		_ = connB2.SetReadDeadline(time.Now().Add(1 * time.Second))
		buf := make([]byte, 64)
		for {
			_, err := connB2.Read(buf)
			if err != nil {
				break
			}
		}
		elapsed := time.Since(start)
		if elapsed < 150*time.Millisecond || elapsed > 600*time.Millisecond {
			t.Errorf("expected disconnect in ~200ms after pings ceased, took %v", elapsed)
		}
	})
}

// TC-126.7: Zero-Timeout Configuration Verification (read_timeout: 0, write_timeout: 0)
func TestServer_ZeroTimeout_NoSyscalls(t *testing.T) {
	r := router.New()
	r.GET("/bench", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("OK")
	})

	cfg := server.DefaultConfig()
	cfg.ReadTimeout = 0
	cfg.WriteTimeout = 0
	cfg.IdleTimeout = 0
	cfg.HTTP2Enabled = false

	srv := server.New(cfg, r)
	rawLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer rawLn.Close()

	countingLn := &countingListener{Listener: rawLn}
	go func() {
		_ = srv.Serve(countingLn)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	clientConn, err := net.Dial("tcp", rawLn.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	reqBytes := []byte("GET /bench HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n")
	reader := bufio.NewReader(clientConn)

	for i := 0; i < 50; i++ {
		if _, err := clientConn.Write(reqBytes); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		resp, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
	}

	countingLn.mu.Lock()
	var totalRead, totalWrite uint64
	for _, cc := range countingLn.activeConns {
		totalRead += cc.readDeadlines.Load()
		totalWrite += cc.writeDeadlines.Load()
	}
	countingLn.mu.Unlock()

	if totalRead != 0 {
		t.Errorf("expected 0 read deadline syscalls with zero-timeout, got %d", totalRead)
	}
	if totalWrite != 0 {
		t.Errorf("expected 0 write deadline syscalls with zero-timeout, got %d", totalWrite)
	}
}

// TC-126.10: Concurrency & Data Race Cleanliness
func TestServer_DeadlineAmortization_ConcurrencyRaceSafety(t *testing.T) {
	r := router.New()
	r.GET("/fast", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("fast")
	})
	r.GET("/stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")
		pr, pw := io.Pipe()
		res.StreamBody = struct {
			io.Reader
			io.Closer
		}{Reader: pr, Closer: pr}
		go func() {
			defer pw.Close()
			for i := 0; i < 5; i++ {
				_, _ = pw.Write([]byte("data: event\n\n"))
				time.Sleep(20 * time.Millisecond)
			}
		}()
	})
	r.GET("/slow-read", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")
		pr, pw := io.Pipe()
		res.StreamBody = struct {
			io.Reader
			io.Closer
		}{Reader: pr, Closer: pr}
		go func() {
			defer pw.Close()
			for i := 0; i < 20; i++ {
				if _, err := pw.Write(bytes.Repeat([]byte("Z"), 8192)); err != nil {
					return
				}
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.ReadTimeout = 1 * time.Second
	cfg.WriteTimeout = 300 * time.Millisecond
	cfg.IdleTimeout = 300 * time.Millisecond

	srv := server.New(cfg, r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	var wg sync.WaitGroup

	// Group 1: 20 rapid keep-alive clients (15 requests each)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()
			reader := bufio.NewReader(conn)
			for j := 0; j < 15; j++ {
				_, _ = conn.Write([]byte("GET /fast HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n"))
				resp, err := http.ReadResponse(reader, nil)
				if err != nil {
					return
				}
				_ = resp.Body.Close()
			}
		}()
	}

	// Group 2: 10 streaming clients
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = conn.Write([]byte("GET /stream HTTP/1.1\r\nHost: localhost\r\n\r\n"))
			buf := make([]byte, 512)
			for {
				_, err := conn.Read(buf)
				if err != nil {
					return
				}
			}
		}()
	}

	// Group 3: 10 slow-read clients
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()
			if tc, ok := conn.(*net.TCPConn); ok {
				_ = tc.SetReadBuffer(512)
			}
			_, _ = conn.Write([]byte("GET /slow-read HTTP/1.1\r\nHost: localhost\r\n\r\n"))
			buf := make([]byte, 128)
			_, _ = conn.Read(buf)
			// halt reading; wait for server timeout
			time.Sleep(400 * time.Millisecond)
		}()
	}

	// Group 4: 10 idle clients
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()
			reader := bufio.NewReader(conn)
			_, _ = conn.Write([]byte("GET /fast HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n"))
			resp, err := http.ReadResponse(reader, nil)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
			// Stalled waiting for idle timeout
			time.Sleep(400 * time.Millisecond)
		}()
	}

	wg.Wait()
}

// TC-129.8: Inbound ADR-056 Smuggling Guard Preservation (Server level)
func TestServer_InboundChunkedRequest_Rejected501(t *testing.T) {
	r := router.New()
	r.POST("/submit", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("ok")
	})

	cfg := server.DefaultConfig()
	cfg.InboundChunkedMode = "reject"
	cfg.Addr = "127.0.0.1:0"
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

	t.Run("Standalone Transfer-Encoding: chunked rejected with 501", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		rawReq := "POST /submit HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"
		if _, err := conn.Write([]byte(rawReq)); err != nil {
			t.Fatalf("failed to write request: %v", err)
		}

		resp, err := io.ReadAll(conn)
		if err != nil && err != io.EOF {
			t.Fatalf("failed to read response: %v", err)
		}
		respStr := string(resp)
		if !strings.Contains(respStr, "501 Not Implemented") {
			t.Fatalf("expected 501 Not Implemented, got:\n%s", respStr)
		}
	})

	t.Run("Conflicting Content-Length and Transfer-Encoding rejected with 400 or 501", func(t *testing.T) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		rawReq := "POST /submit HTTP/1.1\r\nHost: localhost\r\nContent-Length: 10\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n"
		if _, err := conn.Write([]byte(rawReq)); err != nil {
			t.Fatalf("failed to write request: %v", err)
		}

		resp, err := io.ReadAll(conn)
		if err != nil && err != io.EOF {
			t.Fatalf("failed to read response: %v", err)
		}
		respStr := string(resp)
		if !strings.Contains(respStr, "400 Bad Request") && !strings.Contains(respStr, "501 Not Implemented") {
			t.Fatalf("expected 400 or 501 rejection, got:\n%s", respStr)
		}
	})
}

// TC-129.9: Outbound HTTP/1.1 Chunked Framing & Terminal 0\r\n\r\n
func TestServer_HTTP11_OutboundChunkedFraming(t *testing.T) {
	r := router.New()
	r.GET("/stream/chunked", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")

		pr, pw := io.Pipe()
		res.StreamBody = pr

		go func() {
			defer pw.Close()
			_, _ = pw.Write([]byte("hello"))
			time.Sleep(10 * time.Millisecond)
			_, _ = pw.Write([]byte(" world"))
		}()
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
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

	reqStr := "GET /stream/chunked HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	rawResp, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read response: %v", err)
	}

	respStr := string(rawResp)
	if !strings.Contains(strings.ToLower(respStr), "transfer-encoding: chunked") {
		t.Fatalf("expected Transfer-Encoding: chunked in response, got:\n%s", respStr)
	}
	if strings.Contains(strings.ToLower(respStr), "content-length") {
		t.Fatalf("expected Content-Length to be omitted for chunked response, got:\n%s", respStr)
	}
	if !strings.Contains(respStr, "5\r\nhello\r\n") {
		t.Fatalf("expected chunk '5\\r\\nhello\\r\\n', got:\n%s", respStr)
	}
	if !strings.Contains(respStr, "6\r\n world\r\n") {
		t.Fatalf("expected chunk '6\\r\\n world\\r\\n', got:\n%s", respStr)
	}
	if !strings.HasSuffix(respStr, "0\r\n\r\n") {
		t.Fatalf("expected terminal chunk '0\\r\\n\\r\\n' at end, got:\n%s", respStr)
	}
}

// TC-129.10: HTTP/1.1 Persistent Keep-Alive Socket Reuse
func TestServer_HTTP11_ChunkedStream_KeepAliveSocketReuse(t *testing.T) {
	r := router.New()
	r.GET("/stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		pr, pw := io.Pipe()
		res.StreamBody = pr
		go func() {
			defer pw.Close()
			_, _ = pw.Write([]byte("streaming-data"))
		}()
	})
	r.GET("/api/ping", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("pong")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.IdleTimeout = 2 * time.Second
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
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

	br := bufio.NewReader(conn)

	// 1. First Request: Streaming endpoint with keep-alive
	req1 := "GET /stream HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n"
	if _, err := conn.Write([]byte(req1)); err != nil {
		t.Fatalf("failed to write req1: %v", err)
	}

	resp1, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("failed to read resp1: %v", err)
	}
	body1, err := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	if err != nil {
		t.Fatalf("failed to read body1: %v", err)
	}
	if string(body1) != "streaming-data" {
		t.Fatalf("expected body1 'streaming-data', got %q", string(body1))
	}

	// 2. Second Request on the SAME connection
	req2 := "GET /api/ping HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n"
	if _, err := conn.Write([]byte(req2)); err != nil {
		t.Fatalf("failed to write req2 on persistent connection: %v", err)
	}

	resp2, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("failed to read resp2 on persistent connection: %v", err)
	}
	body2, err := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if err != nil {
		t.Fatalf("failed to read body2: %v", err)
	}
	if string(body2) != "pong" {
		t.Fatalf("expected body2 'pong', got %q", string(body2))
	}

	// 3. Third Request with Connection: close
	req3 := "GET /api/ping HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req3)); err != nil {
		t.Fatalf("failed to write req3: %v", err)
	}

	resp3, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("failed to read resp3: %v", err)
	}
	body3, err := io.ReadAll(resp3.Body)
	_ = resp3.Body.Close()
	if err != nil {
		t.Fatalf("failed to read body3: %v", err)
	}
	if string(body3) != "pong" {
		t.Fatalf("expected body3 'pong', got %q", string(body3))
	}

	// Connection should now be closed by server
	oneByte := make([]byte, 1)
	n, err := conn.Read(oneByte)
	if n != 0 || (err != io.EOF && !errors.Is(err, net.ErrClosed)) {
		t.Fatalf("expected connection to be closed with EOF, got n=%d err=%v", n, err)
	}
}

// TC-129.11: HTTP/1.0 Raw Stream Passthrough
func TestServer_HTTP10_RawStreaming_ConnectionClose(t *testing.T) {
	r := router.New()
	r.GET("/stream-http10", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")

		pr, pw := io.Pipe()
		res.StreamBody = pr

		go func() {
			defer pw.Close()
			_, _ = pw.Write([]byte("raw-stream-chunk-1"))
			time.Sleep(10 * time.Millisecond)
			_, _ = pw.Write([]byte("raw-stream-chunk-2"))
		}()
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
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

	reqStr := "GET /stream-http10 HTTP/1.0\r\nHost: localhost\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	rawResp, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read response: %v", err)
	}

	respStr := string(rawResp)
	if strings.Contains(strings.ToLower(respStr), "transfer-encoding") {
		t.Fatalf("expected Transfer-Encoding to be omitted for HTTP/1.0, got:\n%s", respStr)
	}
	if strings.Contains(respStr, "\r\n12\r\n") || strings.Contains(respStr, "\r\n0\r\n\r\n") {
		t.Fatalf("expected no chunk framing in HTTP/1.0 response, got:\n%s", respStr)
	}
	if !strings.Contains(respStr, "raw-stream-chunk-1raw-stream-chunk-2") {
		t.Fatalf("expected payload to contain raw stream chunks, got:\n%s", respStr)
	}
}

// TC-129.12: Upstream Abort Fail-Closed Invariant
func TestServer_ChunkedStream_UpstreamAbort_FailClosed(t *testing.T) {
	r := router.New()
	r.GET("/abort-stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")

		pr, pw := io.Pipe()
		res.StreamBody = pr

		go func() {
			_, _ = pw.Write([]byte("initial-chunk-data"))
			time.Sleep(20 * time.Millisecond)
			_ = pw.CloseWithError(errors.New("simulated backend crash mid-stream"))
		}()
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
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

	reqStr := "GET /abort-stream HTTP/1.1\r\nHost: localhost\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	rawResp, _ := io.ReadAll(conn)
	respStr := string(rawResp)

	headerEnd := strings.Index(respStr, "\r\n\r\n")
	if headerEnd != -1 {
		bodyStr := respStr[headerEnd+4:]
		if strings.Contains(bodyStr, "0\r\n\r\n") {
			t.Fatalf("FAIL-CLOSED VIOLATION: server emitted terminal chunk '0\\r\\n\\r\\n' on aborted stream:\n%s", respStr)
		}
	}
	if !strings.Contains(respStr, "initial-chunk-data") {
		t.Fatalf("expected initial chunk before abort, got:\n%s", respStr)
	}
}

// TC-129.13: Client Disconnect Triggers Upstream Context Cancellation & Zero FD Leakage
func TestServer_StreamContextCancellation_ZeroFDLeakage(t *testing.T) {
	r := router.New()
	upstreamCtxCancelled := make(chan struct{})
	upstreamBodyClosed := make(chan struct{})

	r.GET("/stream/infinite-test", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		pr, pw := io.Pipe()

		res.StreamBody = &streamCloseTracker{
			ReadCloser: pr,
			onClose: func() {
				select {
				case <-upstreamBodyClosed:
				default:
					close(upstreamBodyClosed)
				}
			},
		}

		go func() {
			select {
			case <-req.Context().Done():
				select {
				case <-upstreamCtxCancelled:
				default:
					close(upstreamCtxCancelled)
				}
				_ = pw.CloseWithError(req.Context().Err())
			}
		}()

		go func() {
			defer pw.Close()
			for {
				_, err := pw.Write([]byte("infinite chunk payload\n"))
				if err != nil {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	reqStr := "GET /stream/infinite-test HTTP/1.1\r\nHost: localhost\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Read first chunk from socket
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("failed to read initial bytes: %v", err)
	}

	// Client abruptly severs the TCP connection
	_ = conn.Close()

	// Wait for upstream context cancellation
	select {
	case <-upstreamCtxCancelled:
		// Upstream context cancellation triggered!
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected upstream request context to be cancelled upon client disconnect within 500ms")
	}

	// Wait for upstream body closure
	select {
	case <-upstreamBodyClosed:
		// res.StreamBody.Close() was executed!
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected res.StreamBody to be closed upon client disconnect")
	}
}

type streamCloseTracker struct {
	io.ReadCloser
	onClose func()
}

func (s *streamCloseTracker) Close() error {
	if s.onClose != nil {
		s.onClose()
	}
	return s.ReadCloser.Close()
}

// TC-129.14: Slow-Client Write Timeout Disconnect
func TestServer_ChunkedStream_SlowClient_WriteDeadlineTimeout(t *testing.T) {
	r := router.New()
	closedCh := make(chan struct{})
	r.GET("/slow-stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/octet-stream")

		pr, pw := io.Pipe()
		res.StreamBody = struct {
			io.Reader
			io.Closer
		}{
			Reader: pr,
			Closer: io.Closer(closerFunc(func() error {
				select {
				case <-closedCh:
				default:
					close(closedCh)
				}
				return pr.Close()
			})),
		}

		go func() {
			defer pw.Close()
			bigChunk := bytes.Repeat([]byte("B"), 65536)
			for {
				_, err := pw.Write(bigChunk)
				if err != nil {
					return
				}
			}
		}()
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.WriteTimeout = 100 * time.Millisecond
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
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

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetReadBuffer(1024)
	}

	reqStr := "GET /slow-stream HTTP/1.1\r\nHost: localhost\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Read initial small block
	smallBuf := make([]byte, 128)
	_, _ = conn.Read(smallBuf)

	// Now stop reading completely (simulating stalled/slow client)
	select {
	case <-closedCh:
		// StreamBody was closed because server aborted on write deadline!
	case <-time.After(3 * time.Second):
		t.Fatal("server failed to disconnect stalled slow client within write timeout")
	}
}

// TC-129.17: Concurrent Streaming Stress Test under go test -race
func TestStreaming_ConcurrentStress_RaceSafety(t *testing.T) {
	r := router.New()
	r.GET("/stream-chunked", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		pr, pw := io.Pipe()
		res.StreamBody = pr
		go func() {
			defer pw.Close()
			for i := 0; i < 5; i++ {
				_, _ = fmt.Fprintf(pw, "chunk-%d\n", i)
				time.Sleep(2 * time.Millisecond)
			}
		}()
	})
	r.GET("/static-ping", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("pong")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.IdleTimeout = 2 * time.Second
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	addr := ln.Addr().String()
	var wg sync.WaitGroup

	// 1. 40 HTTP/1.1 keep-alive clients sending sequential requests
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer conn.Close()
			br := bufio.NewReader(conn)

			for j := 0; j < 3; j++ {
				path := "/stream-chunked"
				if j%2 == 1 {
					path = "/static-ping"
				}
				req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n", path)
				if _, err := conn.Write([]byte(req)); err != nil {
					return
				}
				resp, err := http.ReadResponse(br, nil)
				if err != nil {
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
	}

	// 2. 30 HTTP/2 streaming requests via HTTP2AdapterHandler
	h2Handler := srv.HTTP2AdapterHandler()
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			req := httptest.NewRequest("GET", "/stream-chunked", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			h2Handler.ServeHTTP(rec, req)
		}(i)
	}

	// 3. 30 concurrent REST clients
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer conn.Close()
			req := "GET /static-ping HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
			_, _ = conn.Write([]byte(req))
			_, _ = io.ReadAll(conn)
		}()
	}

	wg.Wait()
}

// TC-133-16: Default Normalization Mode Live TCP End-to-End Execution
func TestServer_E2E_InboundChunked_Normalize(t *testing.T) {
	r := router.New()
	r.POST("/echo", func(req *httpparser.Request, res *httpparser.Response) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			res.SetStatus(http.StatusBadRequest)
			return
		}
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString(string(body))
	})
	r.GET("/ping", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("pong")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.InboundChunkedMode = "normalize"
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	addr := ln.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	br := bufio.NewReader(conn)

	// 1. Send Request 1 (chunked POST)
	req1 := "POST /echo HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"d\r\n" +
		"first request\r\n" +
		"0\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req1)); err != nil {
		t.Fatalf("failed to write req1: %v", err)
	}

	resp1, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("failed to read response 1: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for req1, got %d", resp1.StatusCode)
	}
	if string(body1) != "first request" {
		t.Fatalf("expected 'first request', got %q", string(body1))
	}

	// 2. Send Request 2 on same keep-alive connection
	req2 := "GET /ping HTTP/1.1\r\nHost: localhost\r\n\r\n"
	if _, err := conn.Write([]byte(req2)); err != nil {
		t.Fatalf("failed to write req2: %v", err)
	}

	resp2, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("failed to read response 2: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for req2, got %d", resp2.StatusCode)
	}
	if string(body2) != "pong" {
		t.Fatalf("expected 'pong', got %q", string(body2))
	}
}

// TC-133-17: Legacy Rejection Mode ("reject") HTTP 501 E2E Socket Teardown
func TestServer_E2E_InboundChunked_Reject(t *testing.T) {
	r := router.New()
	r.POST("/upload", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("should-not-reach")
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.InboundChunkedMode = "reject"
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	addr := ln.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	req := "POST /upload HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"5\r\n" +
		"hello\r\n" +
		"0\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	respBytes, err := io.ReadAll(conn)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("failed to read response: %v", err)
	}
	respStr := string(respBytes)

	if !strings.Contains(respStr, "501 Not Implemented") {
		t.Fatalf("expected 501 Not Implemented, got:\n%s", respStr)
	}
	if !strings.Contains(strings.ToLower(respStr), "connection: close") {
		t.Fatalf("expected Connection: close, got:\n%s", respStr)
	}
	if !strings.Contains(respStr, "Inbound chunked transfer encoding is disabled") {
		t.Fatalf("expected body to indicate chunked disabled, got:\n%s", respStr)
	}
}

// TC-133-18: Route-Level Granular Profile Overrides & Priority Resolution Hierarchy
func TestServer_E2E_RouteLevelOverrides(t *testing.T) {
	var (
		mu       sync.Mutex
		lastTE   string
		lastCL   string
		lastBody string
	)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		lastTE = r.Header.Get("Transfer-Encoding")
		if lastTE == "" && len(r.TransferEncoding) > 0 {
			lastTE = r.TransferEncoding[0]
		}
		lastCL = r.Header.Get("Content-Length")
		lastBody = string(body)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-ok"))
	}))
	defer backend.Close()

	r := router.New()

	// Route 1: /api/upload -> passthrough
	uploadStrip := false
	uploadOpts := proxy.ProxyOptions{
		Targets:            []string{backend.URL},
		StripPrefix:        &uploadStrip,
		InboundChunkedMode: "passthrough",
	}
	if err := r.RoutePrefix(router.RouteTypeUpstream, "", "/api/upload", nil, "", uploadOpts); err != nil {
		t.Fatalf("failed to route /api/upload: %v", err)
	}

	// Route 2: /api/auth -> reject
	authStrip := false
	authOpts := proxy.ProxyOptions{
		Targets:            []string{backend.URL},
		StripPrefix:        &authStrip,
		InboundChunkedMode: "reject",
	}
	if err := r.RoutePrefix(router.RouteTypeUpstream, "", "/api/auth", nil, "", authOpts); err != nil {
		t.Fatalf("failed to route /api/auth: %v", err)
	}

	// Route 3: /web/index -> normalize (fallback or explicit)
	webStrip := false
	webOpts := proxy.ProxyOptions{
		Targets:            []string{backend.URL},
		StripPrefix:        &webStrip,
		InboundChunkedMode: "normalize",
	}
	if err := r.RoutePrefix(router.RouteTypeUpstream, "", "/web/index", nil, "", webOpts); err != nil {
		t.Fatalf("failed to route /web/index: %v", err)
	}

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.InboundChunkedMode = "normalize" // Global server default
	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	addr := ln.Addr().String()

	// 1. Request A: Chunked POST to /api/upload (expect passthrough)
	{
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		rawReq := "POST /api/upload HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"
		_, _ = conn.Write([]byte(rawReq))
		respBytes, _ := io.ReadAll(conn)
		_ = conn.Close()

		respStr := string(respBytes)
		if !strings.Contains(respStr, "200 OK") {
			t.Fatalf("expected 200 OK on /api/upload, got:\n%s", respStr)
		}

		mu.Lock()
		if lastTE != "chunked" {
			t.Errorf("expected passthrough Transfer-Encoding: chunked, got %q", lastTE)
		}
		if lastBody != "hello" {
			t.Errorf("expected body 'hello', got %q", lastBody)
		}
		mu.Unlock()
	}

	// 2. Request B: Chunked POST to /api/auth (expect reject 501)
	{
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		rawReq := "POST /api/auth HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"
		_, _ = conn.Write([]byte(rawReq))
		respBytes, _ := io.ReadAll(conn)
		_ = conn.Close()

		respStr := string(respBytes)
		if !strings.Contains(respStr, "501 Not Implemented") {
			t.Fatalf("expected 501 Not Implemented on /api/auth, got:\n%s", respStr)
		}
	}

	// 3. Request C: Chunked POST to /web/index (expect normalize 200 with Content-Length)
	{
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		rawReq := "POST /web/index HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"
		_, _ = conn.Write([]byte(rawReq))
		respBytes, _ := io.ReadAll(conn)
		_ = conn.Close()

		respStr := string(respBytes)
		if !strings.Contains(respStr, "200 OK") {
			t.Fatalf("expected 200 OK on /web/index, got:\n%s", respStr)
		}

		mu.Lock()
		if lastTE != "" {
			t.Errorf("expected Transfer-Encoding stripped in normalize mode, got %q", lastTE)
		}
		if lastCL != "5" {
			t.Errorf("expected Content-Length 5 in normalize mode, got %q", lastCL)
		}
		if lastBody != "hello" {
			t.Errorf("expected body 'hello', got %q", lastBody)
		}
		mu.Unlock()
	}
}
