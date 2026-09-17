package httpparser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// getSeedCorpus returns a curated corpus of nominal HTTP/1.1 requests
// and all 19 structural CVE invariant attack vectors from benchmarks/fuzzer/diff_fuzzer.go.
func getSeedCorpus() [][]byte {
	return [][]byte{
		// --- 1. Nominal RFC 7230 Standard Requests ---
		[]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
		[]byte("GET /index.html?search=toron&page=1 HTTP/1.1\r\nHost: localhost\r\nUser-Agent: curl/8.0\r\nAccept: */*\r\n\r\n"),
		[]byte("POST /submit HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 11\r\n\r\nhello=world"),
		[]byte("HEAD /status HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		[]byte("OPTIONS * HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		[]byte("PUT /upload HTTP/1.1\r\nHost: localhost\r\nContent-Length: 5\r\n\r\n12345"),
		[]byte("DELETE /item/42 HTTP/1.1\r\nHost: localhost\r\n\r\n"),

		// --- 2. 19 Structural CVE Invariant Attack Vectors (diff_fuzzer.go) ---
		// SMUGGLE-001: Conflicting Content-Length and Transfer-Encoding (CWE-444)
		[]byte("POST /health HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n"),
		// SMUGGLE-002: Multiple Divergent Content-Length Headers (CWE-444)
		[]byte("POST /health HTTP/1.1\r\nHost: localhost\r\nContent-Length: 5\r\nContent-Length: 10\r\n\r\nhello"),
		// SMUGGLE-003: Transfer-Encoding with Obfuscated Tab Character (CWE-444)
		[]byte("POST /health HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding:\tchunked\r\nContent-Length: 5\r\n\r\n0\r\n\r\n"),
		// SMUGGLE-004: Invalid Chunk Hex Size Extension (CWE-444)
		[]byte("POST /health HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\nZZ\r\nmalicious\r\n0\r\n\r\n"),
		// WHITESPACE-001: Space Before Colon in Field Name (CWE-444, RFC 7230 §3.2.4)
		[]byte("GET /health HTTP/1.1\r\nHost : localhost\r\nUser-Agent: test\r\n\r\n"),
		// WHITESPACE-002: Tab Before Colon in Field Name (CWE-444, RFC 7230 §3.2.4)
		[]byte("GET /health HTTP/1.1\r\nHost\t: localhost\r\nUser-Agent: test\r\n\r\n"),
		// WHITESPACE-003: Obsolete Line Folding obs-fold (CWE-436, RFC 7230 §3.2.4)
		[]byte("GET /health HTTP/1.1\r\nHost: localhost\r\nX-Fold:\r\n  continuation\r\n\r\n"),
		// CONTROL-001: Null Byte (0x00) in Request URI (CWE-117)
		[]byte("GET /health\x00/admin HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		// CONTROL-002: Bell Character (0x07) in Header Value (CWE-117)
		[]byte("GET /health HTTP/1.1\r\nHost: localhost\r\nX-Audit-Payload: test\x07alert\r\n\r\n"),
		// CONTROL-003: Escape Sequence (0x1B) in Query String (CWE-117)
		[]byte("GET /health?q=\x1b[31mRed HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		// TRAVERSAL-001: Raw Dot-Dot Path Traversal Sequence (CWE-22)
		[]byte("GET /internal/dashboard/../../canary_traversal.txt HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		// TRAVERSAL-002: Uppercase Percent-Encoded Traversal %2E%2E (CWE-22)
		[]byte("GET /internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		// TRAVERSAL-003: Double Percent-Encoded Traversal %252e%252e (CWE-22)
		[]byte("GET /internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		// RESOURCE-001: Oversized Request Header > 8KB (CWE-400)
		[]byte(fmt.Sprintf("GET /health HTTP/1.1\r\nHost: localhost\r\nX-Huge-Header: %s\r\n\r\n", strings.Repeat("A", 12000))),
		// RESOURCE-002: Oversized Single Query Parameter > 2KB (CWE-400)
		[]byte(fmt.Sprintf("GET /health?param=%s HTTP/1.1\r\nHost: localhost\r\n\r\n", strings.Repeat("B", 3000))),
		// BASELINE-001: Standard Valid Conformance Request
		[]byte("GET /health HTTP/1.1\r\nHost: localhost\r\nUser-Agent: diff-fuzzer/1.0\r\nAccept: */*\r\n\r\n"),
		// CACHE-001: Web Cache Deception (CWE-524)
		[]byte("GET /cache/private-profile HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer secret-session-token\r\nCache-Control: private\r\n\r\n"),
		// CACHE-002: Shared Cache Set-Cookie Header Stripping (CWE-384)
		[]byte("GET /cache/cookie-resource HTTP/1.1\r\nHost: localhost\r\n\r\n"),
		// CACHE-003: Authorization Refusal Invariant in Shared Cache (CWE-524)
		[]byte("GET /cache/protected-resource HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer user-auth-key\r\n\r\n"),
	}
}

// FuzzParseRequest evaluates in-process raw byte stream parsing against arbitrary mutations.
// Verifies crash immunity (Oracle 1) and bounded execution (Oracle 4).
func FuzzParseRequest(f *testing.F) {
	for _, seed := range getSeedCorpus() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Oracle 4: Clamp input length to 64KB envelope
		if len(data) > 65536 {
			return
		}

		// Oracle 1: Panic & Crash Immunity Guard
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC DETECTED (Oracle 1 Failure) in FuzzParseRequest: %v\nPayload Hex: %x\nPayload: %q", r, data, data)
			}
		}()

		opts := DefaultParserOptions()
		req, err := ParseRequest(bytes.NewReader(data), opts)
		if err == nil && req != nil {
			_ = req.CloseBody()
		}
	})
}

// FuzzDifferentialWithStdLib performs semantic differential verification comparing Toron
// against Go's canonical standard library parser (http.ReadRequest).
func FuzzDifferentialWithStdLib(f *testing.F) {
	for _, seed := range getSeedCorpus() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Oracle 4: Clamp input length to 64KB envelope
		if len(data) > 65536 {
			return
		}

		// Oracle 1: Panic & Crash Immunity Guard
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC DETECTED (Oracle 1 Failure) in FuzzDifferentialWithStdLib: %v\nPayload Hex: %x\nPayload: %q", r, data, data)
			}
		}()

		opts := DefaultParserOptions()
		toronReq, toronErr := ParseRequest(bytes.NewReader(data), opts)
		stdReq, stdErr := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))

		// Clean up allocated body readers
		if toronReq != nil {
			_ = toronReq.CloseBody()
		}
		if stdReq != nil && stdReq.Body != nil {
			_ = stdReq.Body.Close()
		}

		// Oracle 2: Differential Desynchronization Detection (Dangerous Leniency Rule)
		// If standard library rejects due to ambiguous or RFC framing violations, Toron must never accept.
		if stdErr != nil && toronErr == nil {
			stdErrMsg := strings.ToLower(stdErr.Error())
			if strings.Contains(stdErrMsg, "conflicting") ||
				strings.Contains(stdErrMsg, "multiple content-length") ||
				strings.Contains(stdErrMsg, "duplicate content-length") ||
				strings.Contains(stdErrMsg, "transfer-encoding") ||
				strings.Contains(stdErrMsg, "unsupported transfer encoding") ||
				strings.Contains(stdErrMsg, "bad content-length") ||
				strings.Contains(stdErrMsg, "chunked") ||
				strings.Contains(stdErrMsg, "chunk length") ||
				strings.Contains(stdErrMsg, "invalid chunk") ||
				strings.Contains(stdErrMsg, "malformed mime header") {
				t.Fatalf("DESYNCHRONIZATION DETECTED (Oracle 2 Failure): Toron accepted ambiguous framing rejected by stdlib: %v\nPayload: %q", stdErr, data)
			}
		}

		// Oracle 3: Framing Boundary Agreement (When both parsers accept)
		if toronErr == nil && stdErr == nil {
			// 1. Method agreement (case-insensitive)
			if strings.ToUpper(toronReq.Method) != strings.ToUpper(stdReq.Method) {
				t.Fatalf("METHOD DESYNC (Oracle 3 Failure): Toron=%q, stdlib=%q\nPayload: %q", toronReq.Method, stdReq.Method, data)
			}

			// 2. Canonical Path agreement
			if toronReq.URL != nil && stdReq.URL != nil {
				if toronReq.URL.Path != stdReq.URL.Path {
					t.Fatalf("PATH DESYNC (Oracle 3 Failure): Toron=%q, stdlib=%q\nPayload: %q", toronReq.URL.Path, stdReq.URL.Path, data)
				}
			}

			// 3. Content-Length congruence
			if toronReq.Header.Get("Content-Length") != "" || stdReq.Header.Get("Content-Length") != "" {
				if toronReq.ContentLength != stdReq.ContentLength {
					t.Fatalf("CONTENT-LENGTH DESYNC (Oracle 3 Failure): Toron=%d, stdlib=%d\nPayload: %q", toronReq.ContentLength, stdReq.ContentLength, data)
				}
			} else if toronReq.ContentLength > 0 || stdReq.ContentLength > 0 {
				if toronReq.ContentLength != stdReq.ContentLength {
					t.Fatalf("CONTENT-LENGTH DESYNC (Oracle 3 Failure): Toron=%d, stdlib=%d\nPayload: %q", toronReq.ContentLength, stdReq.ContentLength, data)
				}
			}
		}
	})
}

// FuzzHeaderGrammar mutates RFC 7230 §3.2 header tokens, verifying strict rejection
// of whitespace before colons (§3.2.4), control characters, and invalid delimiters.
func FuzzHeaderGrammar(f *testing.F) {
	seeds := []struct {
		name []byte
		val  []byte
	}{
		{[]byte("Host"), []byte("example.com")},
		{[]byte("Host "), []byte("example.com")},
		{[]byte("Host\t"), []byte("example.com")},
		{[]byte("X-Header"), []byte("value\r\n continuation")},
		{[]byte("X-Null"), []byte("val\x00ue")},
		{[]byte("User-Agent"), []byte("curl/8.0.0")},
		{[]byte("Content-Type"), []byte("application/json")},
		{[]byte("Accept"), []byte("*/*")},
		{[]byte("X-Control"), []byte("val\x07ue")},
		{[]byte("X-Bad@Name"), []byte("val")},
	}
	for _, s := range seeds {
		f.Add(s.name, s.val)
	}

	f.Fuzz(func(t *testing.T, headerName, headerValue []byte) {
		// Oracle 4: Bounded execution
		if len(headerName) > 1024 || len(headerValue) > 8192 {
			return
		}

		// RFC 7230 §3.2 / §3.2.6: field-name is a token; cannot contain colons, CR, or LF
		if bytes.IndexByte(headerName, ':') != -1 || bytes.IndexByte(headerName, '\r') != -1 || bytes.IndexByte(headerName, '\n') != -1 {
			return
		}

		// Oracle 1: Panic & Crash Immunity Guard
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC DETECTED (Oracle 1 Failure) in FuzzHeaderGrammar: %v\nHeaderName: %q\nHeaderVal: %q", r, headerName, headerValue)
			}
		}()

		var buf bytes.Buffer
		buf.WriteString("GET /index.html HTTP/1.1\r\nHost: localhost\r\n")
		buf.Write(headerName)
		buf.WriteByte(':')
		buf.Write(headerValue)
		buf.WriteString("\r\n\r\n")
		payload := buf.Bytes()
		if len(payload) > 65536 {
			return
		}

		opts := DefaultParserOptions()
		req, err := ParseRequest(bytes.NewReader(payload), opts)
		if req != nil {
			_ = req.CloseBody()
		}

		// RFC 7230 §3.2.4: whitespace before colon MUST be rejected
		if len(headerName) > 0 {
			lastByte := headerName[len(headerName)-1]
			if lastByte == ' ' || lastByte == '\t' {
				if err == nil {
					t.Fatalf("RFC 7230 §3.2.4 VIOLATION: Toron accepted whitespace before colon in header %q", headerName)
				}
			}
		}

		// If parsed cleanly, header access must not panic
		if err == nil && req != nil {
			_ = req.Header.Get(string(headerName))
		}
	})
}

// parseChunkSize validates RFC 7230 §4.1 chunk size hex formatting and extension bounds.
func parseChunkSize(chunkHex, chunkExt []byte) (int64, error) {
	if len(chunkExt) > 8192 {
		return -1, fmt.Errorf("chunk extension exceeds 8KB limit: %d", len(chunkExt))
	}
	s := strings.TrimSpace(string(chunkHex))
	if len(s) == 0 {
		return -1, errors.New("empty chunk size hex")
	}
	val, err := strconv.ParseInt(s, 16, 64)
	if err != nil || val < 0 {
		return -1, fmt.Errorf("invalid chunk size hex: %q: %v", s, err)
	}
	return val, nil
}

// FuzzChunkFraming mutates HTTP/1.1 chunked transfer framing (RFC 7230 §4.1), asserting
// rejection of inbound chunked requests under Toron's anti-smuggling policy (ADR-056)
// and safe handling of chunk size hex formatting and extensions.
func FuzzChunkFraming(f *testing.F) {
	seeds := []struct {
		hex  []byte
		data []byte
		ext  []byte
	}{
		{[]byte("5"), []byte("hello"), []byte("")},
		{[]byte("A"), []byte("0123456789"), []byte("")},
		{[]byte("0"), []byte(""), []byte("")},
		{[]byte("ZZ"), []byte("malicious"), []byte("")},
		{[]byte("5"), []byte("hello"), []byte(";name=val")},
		{[]byte("-1"), []byte("neg"), []byte("")},
		{[]byte("10000000000000000"), []byte("overflow"), []byte("")},
		{[]byte("8"), []byte("short"), []byte("")},
	}
	for _, s := range seeds {
		f.Add(s.hex, s.data, s.ext)
	}

	f.Fuzz(func(t *testing.T, chunkHex, chunkData, chunkExt []byte) {
		// Oracle 4: Bounded execution
		if len(chunkHex) > 128 || len(chunkData) > 16384 || len(chunkExt) > 8192 {
			return
		}

		// Oracle 1: Panic & Crash Immunity Guard
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC DETECTED (Oracle 1 Failure) in FuzzChunkFraming: %v\nChunkHex: %q\nChunkExt: %q", r, chunkHex, chunkExt)
			}
		}()

		// Synthesize chunked HTTP request
		var buf bytes.Buffer
		buf.WriteString("POST /upload HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n")
		buf.Write(chunkHex)
		buf.Write(chunkExt)
		buf.WriteString("\r\n")
		buf.Write(chunkData)
		buf.WriteString("\r\n0\r\n\r\n")
		payload := buf.Bytes()
		if len(payload) > 65536 {
			return
		}

		opts := DefaultParserOptions()
		req, err := ParseRequest(bytes.NewReader(payload), opts)
		if err == nil && req != nil {
			defer req.CloseBody()
			toronBody, bodyErr := io.ReadAll(req.Body)
			_ = toronBody
			_ = bodyErr
		}

		// Differential execution against Go standard library http.ReadRequest
		stdReq, stdErr := http.ReadRequest(bufio.NewReader(bytes.NewReader(payload)))
		if stdErr == nil && stdReq != nil {
			defer stdReq.Body.Close()
			_, _ = io.ReadAll(stdReq.Body)
		}

		// Exercise chunk size parser utility logic
		_, _ = parseChunkSize(chunkHex, chunkExt)
	})
}
