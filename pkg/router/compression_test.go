package router

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"toron/pkg/httpparser"
)

func TestCompression_Brotli(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 100
	r.Use(NewCompressionMiddleware(cfg))

	largeBody := strings.Repeat("Brotli compression algorithm verification for Toron HTTP router. ", 20)

	r.GET("/brotli-data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(largeBody)
	})

	req, _ := httpparser.NewRequest("GET", "/brotli-data", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "br")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	if enc := res.Header.Get("Content-Encoding"); enc != "br" {
		t.Fatalf("expected Content-Encoding 'br', got %q", enc)
	}

	if res.Body.Len() >= len(largeBody) {
		t.Fatalf("expected compressed body (%d) to be smaller than original (%d)", res.Body.Len(), len(largeBody))
	}

	// Decompress Brotli
	br := brotli.NewReader(res.Body)
	decompressed, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("failed to read decompressed brotli data: %v", err)
	}

	if string(decompressed) != largeBody {
		t.Fatalf("decompressed brotli content mismatch")
	}
}

func TestCompression_Zstandard(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 100
	r.Use(NewCompressionMiddleware(cfg))

	largeBody := strings.Repeat("Zstandard ultra-fast real-time compression engine in Toron. ", 20)

	r.GET("/zstd-data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString(largeBody)
	})

	req, _ := httpparser.NewRequest("GET", "/zstd-data", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "zstd")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	if enc := res.Header.Get("Content-Encoding"); enc != "zstd" {
		t.Fatalf("expected Content-Encoding 'zstd', got %q", enc)
	}

	// Decompress Zstandard
	zr, err := zstd.NewReader(res.Body)
	if err != nil {
		t.Fatalf("failed to create zstd reader: %v", err)
	}
	defer zr.Close()

	decompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("failed to read decompressed zstd data: %v", err)
	}

	if string(decompressed) != largeBody {
		t.Fatalf("decompressed zstd content mismatch")
	}
}

func TestCompression_QualityWeighting(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 50
	r.Use(NewCompressionMiddleware(cfg))

	body := strings.Repeat("Testing Quality Factor Weighting in Accept-Encoding. ", 10)

	r.GET("/quality-test", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString(body)
	})

	// Client explicitly prefers gzip (q=1.0) over br (q=0.5)
	req, _ := httpparser.NewRequest("GET", "/quality-test", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "gzip;q=1.0, br;q=0.5")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if enc := res.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("expected 'gzip' due to higher q value (1.0 vs 0.5), got %q", enc)
	}
}

func TestCompression_PrecedenceOrder(t *testing.T) {
	r := New()
	cfg := DefaultCompressionConfig()
	cfg.MinLength = 50
	r.Use(NewCompressionMiddleware(cfg))

	body := strings.Repeat("Testing Server Precedence Order for Compression. ", 10)

	r.GET("/precedence-test", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(body)
	})

	// When all have equal q, zstd takes precedence over br, gzip, deflate
	req, _ := httpparser.NewRequest("GET", "/precedence-test", "HTTP/1.1")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if enc := res.Header.Get("Content-Encoding"); enc != "zstd" {
		t.Fatalf("expected 'zstd' precedence when all q values are equal, got %q", enc)
	}

	// When zstd not requested, br takes precedence over gzip
	req2, _ := httpparser.NewRequest("GET", "/precedence-test", "HTTP/1.1")
	req2.Header.Set("Accept-Encoding", "gzip, deflate, br")
	res2 := httpparser.NewResponse()

	r.ServeHTTP(req2, res2)

	if enc2 := res2.Header.Get("Content-Encoding"); enc2 != "br" {
		t.Fatalf("expected 'br' precedence over gzip, got %q", enc2)
	}
}

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

func TestCompressionMiddleware_StreamingBypass(t *testing.T) {
	t.Run("Subtest 5A: res.StreamBody != nil bypasses compression", func(t *testing.T) {
		r := New()
		cfg := DefaultCompressionConfig()
		cfg.MinLength = 10
		r.Use(NewCompressionMiddleware(cfg))

		r.GET("/stream", func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			res.Header.Set("Content-Type", "text/event-stream")
			res.StreamBody = io.NopCloser(strings.NewReader("event: data\ndata: hello\n\n"))
		})

		req, _ := httpparser.NewRequest("GET", "/stream", "HTTP/1.1")
		req.Header.Set("Accept-Encoding", "gzip, zstd")
		res := httpparser.NewResponse()

		r.ServeHTTP(req, res)

		if enc := res.Header.Get("Content-Encoding"); enc != "" {
			t.Fatalf("expected no Content-Encoding for streaming response, got %q", enc)
		}
		if cl := res.Header.Get("Content-Length"); cl != "" {
			t.Fatalf("expected no Content-Length for streaming response, got %q", cl)
		}
		if res.Body.Len() != 0 {
			t.Fatalf("expected res.Body.Len() == 0, got %d", res.Body.Len())
		}
	})

	t.Run("Subtest 5A2: text/event-stream MIME bypasses compression even without StreamBody", func(t *testing.T) {
		r := New()
		cfg := DefaultCompressionConfig()
		cfg.MinLength = 10
		r.Use(NewCompressionMiddleware(cfg))

		r.GET("/sse", func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			res.Header.Set("Content-Type", "text/event-stream")
			_, _ = res.WriteString(strings.Repeat("data: message\n\n", 20))
		})

		req, _ := httpparser.NewRequest("GET", "/sse", "HTTP/1.1")
		req.Header.Set("Accept-Encoding", "gzip")
		res := httpparser.NewResponse()

		r.ServeHTTP(req, res)

		if enc := res.Header.Get("Content-Encoding"); enc != "" {
			t.Fatalf("expected no Content-Encoding for text/event-stream, got %q", enc)
		}
	})

	t.Run("Subtest 5A3: X-Accel-Buffering: no bypasses compression", func(t *testing.T) {
		r := New()
		cfg := DefaultCompressionConfig()
		cfg.MinLength = 10
		r.Use(NewCompressionMiddleware(cfg))

		r.GET("/unbuffered", func(req *httpparser.Request, res *httpparser.Response) {
			res.SetStatus(http.StatusOK)
			res.Header.Set("Content-Type", "text/plain")
			res.Header.Set("X-Accel-Buffering", "no")
			_, _ = res.WriteString(strings.Repeat("unbuffered message ", 20))
		})

		req, _ := httpparser.NewRequest("GET", "/unbuffered", "HTTP/1.1")
		req.Header.Set("Accept-Encoding", "gzip")
		res := httpparser.NewResponse()

		r.ServeHTTP(req, res)

		if enc := res.Header.Get("Content-Encoding"); enc != "" {
			t.Fatalf("expected no Content-Encoding for X-Accel-Buffering: no, got %q", enc)
		}
	})
}
