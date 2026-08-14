package router

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"

	"toron/pkg/httpparser"
)

func TestCompression_Gzip(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 100
	r.Use(NewCompressionMiddleware(cfg))

	largeBody := strings.Repeat("Toron high-performance event-driven web server in Go. ", 20)

	r.GET("/compressed-data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json; charset=utf-8")
		_, _ = res.WriteString(largeBody)
	})

	req, _ := httpparser.NewRequest("GET", "/compressed-data", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	if enc := res.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("expected Content-Encoding 'gzip', got %q", enc)
	}

	if vary := res.Header.Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("expected Vary header to contain 'Accept-Encoding', got %q", vary)
	}

	if res.Body.Len() >= len(largeBody) {
		t.Fatalf("expected compressed body size (%d) to be smaller than original (%d)", res.Body.Len(), len(largeBody))
	}

	// Decompress and verify content
	gr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read decompressed gzip data: %v", err)
	}

	if string(decompressed) != largeBody {
		t.Fatalf("decompressed content mismatch: expected %q, got %q", largeBody, string(decompressed))
	}
}

func TestCompression_Deflate(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 100
	r.Use(NewCompressionMiddleware(cfg))

	largeBody := strings.Repeat("Deflate compression test payload for Toron HTTP router. ", 25)

	r.GET("/deflate-data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString(largeBody)
	})

	req, _ := httpparser.NewRequest("GET", "/deflate-data", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "deflate")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if enc := res.Header.Get("Content-Encoding"); enc != "deflate" {
		t.Fatalf("expected Content-Encoding 'deflate', got %q", enc)
	}

	// Decompress deflate
	fr := flate.NewReader(res.Body)
	defer fr.Close()

	decompressed, err := io.ReadAll(fr)
	if err != nil {
		t.Fatalf("failed to read decompressed deflate data: %v", err)
	}

	if string(decompressed) != largeBody {
		t.Fatalf("decompressed deflate content mismatch")
	}
}

func TestCompression_MinLengthBypass(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 512
	r.Use(NewCompressionMiddleware(cfg))

	shortBody := "Short text payload"

	r.GET("/short", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString(shortBody)
	})

	req, _ := httpparser.NewRequest("GET", "/short", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "gzip")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if enc := res.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("expected empty Content-Encoding for short payload, got %q", enc)
	}

	if res.Body.String() != shortBody {
		t.Fatalf("expected unchanged body %q, got %q", shortBody, res.Body.String())
	}
}

func TestCompression_NoAcceptEncoding(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 50
	r.Use(NewCompressionMiddleware(cfg))

	body := strings.Repeat("No compression requested. ", 20)

	r.GET("/no-compress", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString(body)
	})

	req, _ := httpparser.NewRequest("GET", "/no-compress", "HTTP/1.1")
	// No Accept-Encoding header
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if enc := res.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("expected empty Content-Encoding, got %q", enc)
	}
	if res.Body.String() != body {
		t.Fatalf("expected raw body matching original")
	}
}

func TestCompression_BinaryMimeTypeBypass(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 50
	r.Use(NewCompressionMiddleware(cfg))

	binaryPayload := bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, 20)

	r.GET("/image.png", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "image/png")
		_, _ = res.Write(binaryPayload)
	})

	req, _ := httpparser.NewRequest("GET", "/image.png", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "gzip")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if enc := res.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("expected no compression for image/png, got %q", enc)
	}
	if !bytes.Equal(res.Body.Bytes(), binaryPayload) {
		t.Fatalf("expected unchanged binary payload")
	}
}

func TestCompression_VaryHeaderAppend(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 50
	r.Use(NewCompressionMiddleware(cfg))

	body := strings.Repeat("Testing Vary Header Append functionality. ", 10)

	r.GET("/custom-vary", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Vary", "Origin")
		_, _ = res.WriteString(body)
	})

	req, _ := httpparser.NewRequest("GET", "/custom-vary", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "gzip")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	vary := res.Header.Get("Vary")
	if !strings.Contains(vary, "Origin") || !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("expected Vary to contain both Origin and Accept-Encoding, got %q", vary)
	}
}

func TestCompression_WebSocketBypass(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 10
	r.Use(NewCompressionMiddleware(cfg))

	r.GET("/ws", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusSwitchingProtocols)
		res.Header.Set("Upgrade", "websocket")
		res.Header.Set("Connection", "Upgrade")
	})

	req, _ := httpparser.NewRequest("GET", "/ws", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "gzip")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if enc := res.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("expected no compression for 101 Switching Protocols, got %q", enc)
	}
}
