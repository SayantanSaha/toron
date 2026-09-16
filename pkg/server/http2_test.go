package server_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

type mockFlusherRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (m *mockFlusherRecorder) Flush() {
	m.flushed++
}

// TC-125.9: HTTP/2 Streaming Adapter Flush Verification
func TestServer_HTTP2Adapter_StreamingImmediateFlush(t *testing.T) {
	r := router.New()
	r.GET("/h2-stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")

		pr, pw := io.Pipe()
		res.StreamBody = pr

		go func() {
			defer pw.Close()
			for i := 1; i <= 3; i++ {
				_, _ = fmt.Fprintf(pw, "data: event-%d\n\n", i)
			}
		}()
	})

	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	rec := &mockFlusherRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
	req := httptest.NewRequest("GET", "/h2-stream", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.flushed < 1 {
		t.Fatalf("expected flusher.Flush() called at least once, got %d", rec.flushed)
	}
	bodyStr := rec.Body.String()
	for i := 1; i <= 3; i++ {
		expected := fmt.Sprintf("data: event-%d\n\n", i)
		if !strings.Contains(bodyStr, expected) {
			t.Errorf("expected body to contain %q, got: %s", expected, bodyStr)
		}
	}
}

type countingFlusherRecorder struct {
	*httptest.ResponseRecorder
	mu         sync.Mutex
	writeCalls int
	flushCalls int
}

func (c *countingFlusherRecorder) Write(b []byte) (int, error) {
	c.mu.Lock()
	c.writeCalls++
	c.mu.Unlock()
	return c.ResponseRecorder.Write(b)
}

func (c *countingFlusherRecorder) Flush() {
	c.mu.Lock()
	c.flushCalls++
	c.mu.Unlock()
}

// TC-129.15: HTTP/2 http.Flusher Zero-Buffering Streaming Parity
func TestServer_HTTP2_StreamBody_FlusherParity(t *testing.T) {
	r := router.New()
	r.GET("/h2-parity-stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/event-stream")

		pr, pw := io.Pipe()
		res.StreamBody = pr

		go func() {
			defer pw.Close()
			for i := 1; i <= 3; i++ {
				_, _ = fmt.Fprintf(pw, "chunk-%d\n", i)
				time.Sleep(15 * time.Millisecond)
			}
		}()
	})

	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	rec := &countingFlusherRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
	req := httptest.NewRequest("GET", "/h2-parity-stream", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.writeCalls < 3 {
		t.Errorf("expected at least 3 distinct Write calls, got %d", rec.writeCalls)
	}
	if rec.flushCalls < 3 {
		t.Errorf("expected at least 3 distinct Flush calls, got %d", rec.flushCalls)
	}
	expected := "chunk-1\nchunk-2\nchunk-3\n"
	if rec.Body.String() != expected {
		t.Errorf("expected body %q, got %q", expected, rec.Body.String())
	}
}

// TC-129.16: HTTP/2 RST_STREAM Client Abort Handling
func TestServer_HTTP2_ClientReset_AbortsStream(t *testing.T) {
	r := router.New()
	streamClosed := make(chan struct{})

	r.GET("/h2-abort-stream", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		pr, pw := io.Pipe()
		res.StreamBody = &watchCloseReader{
			ReadCloser: pr,
			onClose: func() {
				close(streamClosed)
			},
		}

		go func() {
			defer pw.Close()
			for i := 0; i < 100; i++ {
				_, err := fmt.Fprintf(pw, "data chunk %d\n", i)
				if err != nil {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
	})

	srv := server.New(server.DefaultConfig(), r)
	handler := srv.HTTP2AdapterHandler()

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/h2-abort-stream", nil).WithContext(ctx)
	rec := &mockFlusherRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// Wait briefly for first chunk
	time.Sleep(30 * time.Millisecond)
	// Client resets stream / cancels context
	cancel()

	select {
	case <-done:
		// Succeeded in terminating ServeHTTP
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ServeHTTP did not terminate promptly upon context cancellation")
	}

	select {
	case <-streamClosed:
		// Verified upstream stream handle closed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("upstream res.StreamBody was not closed upon stream reset")
	}
}

type watchCloseReader struct {
	io.ReadCloser
	onClose func()
}

func (w *watchCloseReader) Close() error {
	if w.onClose != nil {
		w.onClose()
	}
	return w.ReadCloser.Close()
}

