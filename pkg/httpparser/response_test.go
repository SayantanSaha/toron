package httpparser_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"toron/pkg/httpparser"
)

func TestHttpParser_BufferPool_RecyclingAndReset(t *testing.T) {
	t.Run("Subtest 8A (Buffer Hygiene & Clean Reset)", func(t *testing.T) {
		buf := httpparser.GetBuffer()
		buf.WriteString("SECRET_TOKEN_FROM_REQUEST_A")
		httpparser.PutBuffer(buf)

		buf2 := httpparser.GetBuffer()
		defer httpparser.PutBuffer(buf2)
		if buf2.Len() != 0 {
			t.Fatalf("expected recycled buffer length to be 0, got %d", buf2.Len())
		}
		if buf2.String() != "" {
			t.Fatalf("expected recycled buffer string to be empty, got %q", buf2.String())
		}
	})

	t.Run("Subtest 8B (Copy Buffer Slab Pool)", func(t *testing.T) {
		slicePtr := httpparser.GetCopyBuffer()
		if slicePtr == nil {
			t.Fatal("expected non-nil copy buffer pointer")
		}
		if len(*slicePtr) != 32*1024 {
			t.Fatalf("expected copy buffer slab of 32KB (32768 bytes), got %d", len(*slicePtr))
		}
		httpparser.PutCopyBuffer(slicePtr)
	})
}

func TestHttpParser_Response_ZeroCopyDualWriteSerialization(t *testing.T) {
	t.Run("Subtest 8C (Zero-Copy Dual-Write Serialization)", func(t *testing.T) {
		res := httpparser.NewResponse()
		res.Header.Set("X-Custom-Header", "value123")
		payload := strings.Repeat("X", 64*1024)
		_, _ = res.WriteString(payload)

		var dest bytes.Buffer
		if err := res.Serialize(&dest); err != nil {
			t.Fatalf("failed to serialize response: %v", err)
		}

		destStr := dest.String()
		destLower := strings.ToLower(destStr)
		if !strings.HasPrefix(destStr, "HTTP/1.1 200 OK\r\n") {
			t.Fatalf("expected status line in serialized response, got: %s", destStr[:min(len(destStr), 50)])
		}
		if !strings.Contains(destLower, "x-custom-header: value123\r\n") {
			t.Fatalf("expected custom header in serialized output")
		}
		if !strings.Contains(destLower, "content-length: 65536\r\n") {
			t.Fatalf("expected Content-Length: 65536 in serialized output")
		}
		if !strings.HasSuffix(destStr, payload) {
			t.Fatalf("expected payload to be written verbatim")
		}

		// Verify allocation profile does not allocate a monolithic 64KB+ buffer
		discard := io.Discard
		allocs := testing.AllocsPerRun(100, func() {
			_ = res.Serialize(discard)
		})
		// With buffer pooling and dual write, allocations are very small (0-2 per serialize run for io.Writer interface conversion)
		if allocs > 5 {
			t.Fatalf("expected low allocation count for pooled serialization, got %f allocs/op", allocs)
		}
	})

	t.Run("Subtest 8D (Streaming Header Serialization)", func(t *testing.T) {
		res := httpparser.NewResponse()
		res.Header.Set("Content-Type", "text/event-stream")
		res.Header.Set("X-Stream-Id", "stream-42")
		// Put something in res.Body to verify it is NOT written
		_, _ = res.WriteString("SHOULD_NOT_BE_SERIALIZED")
		res.StreamBody = io.NopCloser(strings.NewReader("live event stream"))

		var dest bytes.Buffer
		if err := res.Serialize(&dest); err != nil {
			t.Fatalf("failed to serialize streaming response: %v", err)
		}

		destStr := dest.String()
		destLower := strings.ToLower(destStr)
		if !strings.HasPrefix(destStr, "HTTP/1.1 200 OK\r\n") {
			t.Fatalf("expected 200 OK status line, got: %s", destStr)
		}
		if !strings.Contains(destLower, "content-type: text/event-stream\r\n") {
			t.Fatalf("expected Content-Type: text/event-stream, got: %s", destStr)
		}
		if !strings.Contains(destLower, "x-stream-id: stream-42\r\n") {
			t.Fatalf("expected X-Stream-Id header, got: %s", destStr)
		}
		if strings.Contains(destLower, "content-length:") {
			t.Fatalf("Content-Length must be strictly omitted for streaming response, got: %s", destStr)
		}
		if strings.Contains(destStr, "SHOULD_NOT_BE_SERIALIZED") {
			t.Fatalf("res.Body must NOT be written when res.StreamBody != nil")
		}
		if !strings.HasSuffix(destStr, "\r\n\r\n") {
			t.Fatalf("expected headers to terminate with \\r\\n\\r\\n, got: %q", destStr)
		}
	})
}
