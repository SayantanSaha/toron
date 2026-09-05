package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
