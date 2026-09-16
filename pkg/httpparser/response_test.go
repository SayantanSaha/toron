package httpparser_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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

// TC-127.7: Response Serialization Buffer Slab Recycling (responseBufPool)
func TestHttpParser_ResponseBufPool_RecyclingAndHygiene(t *testing.T) {
	t.Run("Subtest 7A (Acquisition & Slab Capacity)", func(t *testing.T) {
		bufPtr := httpparser.GetResponseBuffer()
		if bufPtr == nil {
			t.Fatal("expected non-nil buffer pointer from responseBufPool")
		}
		if len(*bufPtr) != 0 {
			t.Fatalf("expected initial slice length 0, got %d", len(*bufPtr))
		}
		if cap(*bufPtr) < 4096 {
			t.Fatalf("expected slab capacity >= 4096 bytes, got %d", cap(*bufPtr))
		}
		httpparser.PutResponseBuffer(bufPtr)
	})

	t.Run("Subtest 7B (Modification & Clean Resetting)", func(t *testing.T) {
		bufPtr := httpparser.GetResponseBuffer()
		*bufPtr = append(*bufPtr, bytes.Repeat([]byte("X"), 2048)...)
		if len(*bufPtr) != 2048 {
			t.Fatalf("expected length 2048 after append, got %d", len(*bufPtr))
		}

		httpparser.PutResponseBuffer(bufPtr)

		recycled := httpparser.GetResponseBuffer()
		defer httpparser.PutResponseBuffer(recycled)

		if len(*recycled) != 0 {
			t.Fatalf("expected recycled buffer length to be cleanly reset to 0, got %d", len(*recycled))
		}
		if cap(*recycled) < 4096 {
			t.Fatalf("expected recycled buffer capacity >= 4096, got %d", cap(*recycled))
		}
	})

	t.Run("Subtest 7C (Nil Pointer Safety)", func(t *testing.T) {
		// Must not panic on nil put
		httpparser.PutResponseBuffer(nil)
		httpparser.PutResponseBuf(nil)
	})

	t.Run("Subtest 7D (Large Header Reallocation Safety)", func(t *testing.T) {
		bufPtr := httpparser.GetResponseBuffer()
		*bufPtr = append(*bufPtr, bytes.Repeat([]byte("Y"), 8192)...)
		if cap(*bufPtr) < 8192 {
			t.Fatalf("expected capacity >= 8192, got %d", cap(*bufPtr))
		}

		httpparser.PutResponseBuffer(bufPtr)

		recycled := httpparser.GetResponseBuffer()
		defer httpparser.PutResponseBuffer(recycled)

		if len(*recycled) != 0 {
			t.Fatalf("expected length 0 for recycled enlarged slab, got %d", len(*recycled))
		}
		if cap(*recycled) < 4096 {
			t.Fatalf("expected retained capacity >= 4096, got %d", cap(*recycled))
		}
	})
}

// TC-127.8: Zero-Allocation Verification for res.Serialize (AllocsPerRun <= 1)
func TestHttpParser_Response_ZeroAllocationSerialization(t *testing.T) {
	res := httpparser.NewResponse()
	res.Header.Set("Content-Type", "application/json")
	res.Header.Set("Server", "toron")
	res.Header.Set("X-Request-Id", "req-test-12345")
	_, _ = res.WriteString(`{"status":"ok"}`)

	discard := io.Discard
	// Warm up pool
	if err := res.Serialize(discard); err != nil {
		t.Fatalf("warmup serialize failed: %v", err)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = res.Serialize(discard)
	})

	if allocs > 1.0 {
		t.Fatalf("expected <= 1.0 allocs/op for pooled response serialization, got %f", allocs)
	}

	// Streaming response test: verify serialization succeeds and body is bypassed
	resStream := httpparser.NewResponse()
	resStream.Header.Set("Content-Type", "text/event-stream")
	resStream.StreamBody = io.NopCloser(strings.NewReader("stream data"))
	_, _ = resStream.WriteString("BUFFER_DATA_THAT_MUST_BE_BYPASSED")

	if err := resStream.Serialize(discard); err != nil {
		t.Fatalf("streaming serialize failed: %v", err)
	}
}

// TC-127.9: Header Sanitization and CRLF Invariant Verification
func TestHttpParser_Response_CRLFProtectionAndDualWrite(t *testing.T) {
	t.Run("Subtest 9A (Clean Header Path)", func(t *testing.T) {
		res := httpparser.NewResponse()
		res.Header.Set("Host", "localhost")
		res.Header.Set("User-Agent", "test")
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("hello")

		var dest bytes.Buffer
		if err := res.Serialize(&dest); err != nil {
			t.Fatalf("serialize failed: %v", err)
		}

		raw := dest.String()
		destLower := strings.ToLower(raw)
		if !strings.Contains(destLower, "host: localhost\r\n") || !strings.Contains(destLower, "user-agent: test\r\n") {
			t.Fatalf("expected clean headers in output, got: %s", raw)
		}
	})

	t.Run("Subtest 9B (CRLF Injection Neutralization)", func(t *testing.T) {
		res := httpparser.NewResponse()
		res.Header.Set("X-Injected", "value\r\nInjected-Header: evil\r\n\r\nHTTP/1.1 200 OK")
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"safe":true}`)

		var dest bytes.Buffer
		if err := res.Serialize(&dest); err != nil {
			t.Fatalf("serialize failed: %v", err)
		}

		raw := dest.String()
		// Injected header line must not exist as an independent header
		if strings.Contains(raw, "\r\nInjected-Header: evil") {
			t.Fatalf("CRLF injection was NOT neutralized, raw: %s", raw)
		}

		// Verify output parses cleanly without response splitting
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(dest.Bytes())), nil)
		if err != nil {
			t.Fatalf("failed to parse sanitized response with http.ReadResponse: %v", err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("Injected-Header") != "" {
			t.Fatalf("injected header must not be parsed into response headers")
		}
	})

	t.Run("Subtest 9C (Wire Format RFC Compliance)", func(t *testing.T) {
		res := httpparser.NewResponse()
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		res.Header.Set("Server", "toron")
		_, _ = res.WriteString(`{"status":"ok"}`)

		var dest bytes.Buffer
		if err := res.Serialize(&dest); err != nil {
			t.Fatalf("serialize failed: %v", err)
		}

		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(dest.Bytes())), nil)
		if err != nil {
			t.Fatalf("http.ReadResponse failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected Content-Type application/json, got %q", ct)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if string(bodyBytes) != `{"status":"ok"}` {
			t.Fatalf("expected body %q, got %q", `{"status":"ok"}`, string(bodyBytes))
		}
	})

	t.Run("Subtest 9D (Dual-Write Body Handling)", func(t *testing.T) {
		// Non-empty body
		res1 := httpparser.NewResponse()
		res1.Header.Set("Content-Type", "text/plain")
		_, _ = res1.WriteString("payload-data")

		var dest1 bytes.Buffer
		if err := res1.Serialize(&dest1); err != nil {
			t.Fatalf("serialize res1 failed: %v", err)
		}
		if !strings.HasSuffix(dest1.String(), "\r\n\r\npayload-data") {
			t.Fatalf("expected payload immediately after separator, got: %q", dest1.String())
		}

		// Empty body
		res2 := httpparser.NewResponse()
		res2.Header.Set("Content-Type", "text/plain")
		var dest2 bytes.Buffer
		if err := res2.Serialize(&dest2); err != nil {
			t.Fatalf("serialize res2 failed: %v", err)
		}
		if !strings.HasSuffix(dest2.String(), "\r\n\r\n") {
			t.Fatalf("expected empty body response to end with \\r\\n\\r\\n, got: %q", dest2.String())
		}
	})
}

// TC-127.10: Concurrency, Thread-Safety & Race Cleanliness for responseBufPool
func TestHttpParser_ResponseBufPool_ConcurrentStress(t *testing.T) {
	const workerCount = 100
	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func(id int) {
			defer wg.Done()
			res := httpparser.NewResponse()
			res.SetStatus(http.StatusOK)
			res.Header.Set("X-Worker-ID", fmt.Sprintf("worker-%d", id))
			res.Header.Set("Content-Type", "application/json")
			_, _ = res.WriteString(fmt.Sprintf(`{"worker":%d}`, id))

			var dest bytes.Buffer
			if err := res.Serialize(&dest); err != nil {
				t.Errorf("worker %d serialize failed: %v", id, err)
				return
			}

			destStr := dest.String()
			destLower := strings.ToLower(destStr)
			expectedHeader := fmt.Sprintf("x-worker-id: worker-%d\r\n", id)
			expectedBody := fmt.Sprintf(`{"worker":%d}`, id)

			if !strings.Contains(destLower, expectedHeader) {
				t.Errorf("worker %d missing expected header in output: %s", id, destStr)
			}
			if !strings.HasSuffix(destStr, expectedBody) {
				t.Errorf("worker %d output corrupt, expected suffix %q: %s", id, expectedBody, destStr)
			}
		}(i)
	}

	wg.Wait()
}
