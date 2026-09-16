package httpparser_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"toron/pkg/httpparser"
)

func TestParseRequest_ValidGET(t *testing.T) {
	rawReq := "GET /api/v1/health?param=value HTTP/1.1\r\nHost: localhost\r\nUser-Agent: test-client\r\n\r\n"
	opts := httpparser.DefaultParserOptions()

	req, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
	if err != nil {
		t.Fatalf("unexpected error parsing GET request: %v", err)
	}

	if req.Method != "GET" {
		t.Errorf("expected method GET, got %q", req.Method)
	}
	if req.Path != "/api/v1/health" {
		t.Errorf("expected path /api/v1/health, got %q", req.Path)
	}
	if req.QueryParams.Get("param") != "value" {
		t.Errorf("expected query param 'param=value', got %q", req.QueryParams.Get("param"))
	}
	if req.Header.Get("Host") != "localhost" {
		t.Errorf("expected Header Host=localhost, got %q", req.Header.Get("Host"))
	}
}

func TestParseRequest_ValidPOSTWithBody(t *testing.T) {
	bodyStr := "hello toron payload"
	rawReq := "POST /submit HTTP/1.1\r\nHost: localhost\r\nContent-Type: text/plain\r\nContent-Length: 19\r\n\r\n" + bodyStr
	opts := httpparser.DefaultParserOptions()

	req, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
	if err != nil {
		t.Fatalf("unexpected error parsing POST request: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("expected method POST, got %q", req.Method)
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if string(bodyBytes) != bodyStr {
		t.Errorf("expected body %q, got %q", bodyStr, string(bodyBytes))
	}
}

func TestParseRequest_SmugglingRejection(t *testing.T) {
	rawReq := "POST /submit HTTP/1.1\r\nHost: localhost\r\nContent-Length: 10\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n"
	opts := httpparser.DefaultParserOptions()

	_, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
	if err == nil {
		t.Fatal("expected error parsing request with conflicting Content-Length and Transfer-Encoding headers")
	}
}

func TestParseRequest_StandaloneTransferEncodingRejection(t *testing.T) {
	rawReq := "POST /submit HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"
	opts := httpparser.DefaultParserOptions()

	_, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
	if err == nil {
		t.Fatal("expected error parsing request with standalone Transfer-Encoding: chunked")
	}
	if !errors.Is(err, httpparser.ErrUnsupportedTransferEncoding) {
		t.Fatalf("expected ErrUnsupportedTransferEncoding, got %v", err)
	}
}

func TestParseRequest_HeaderLimits(t *testing.T) {
	hugeHeader := "GET / HTTP/1.1\r\nHost: localhost\r\nX-Large-Header: " + string(make([]byte, 10000)) + "\r\n\r\n"
	opts := httpparser.ParserOptions{MaxHeaderBytes: 1024, MaxBodyBytes: 1024}

	_, err := httpparser.ParseRequest(bytes.NewBufferString(hugeHeader), opts)
	if err != httpparser.ErrHeaderTooLarge {
		t.Errorf("expected ErrHeaderTooLarge, got %v", err)
	}
}

func TestResponse_Serialize(t *testing.T) {
	res := httpparser.NewResponse()
	res.SetStatus(200)
	res.Header.Set("X-Custom", "test")
	_, _ = res.WriteString("hello response")

	var buf bytes.Buffer
	if err := res.Serialize(&buf); err != nil {
		t.Fatalf("failed to serialize response: %v", err)
	}

	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("HTTP/1.1 200 OK")) {
		t.Errorf("expected 200 OK status line in response, got %q", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("hello response")) {
		t.Errorf("expected body in response, got %q", out)
	}
}

func TestResponse_HeaderCRLFInjection(t *testing.T) {
	res := httpparser.NewResponse()
	res.SetStatus(200)
	res.Header.Set("X-Injected\r\nHeader", "value\r\nSet-Cookie: session=stolen")
	_, _ = res.WriteString("ok")

	var buf bytes.Buffer
	if err := res.Serialize(&buf); err != nil {
		t.Fatalf("failed to serialize response: %v", err)
	}

	out := buf.String()
	if bytes.Contains(buf.Bytes(), []byte("\r\nSet-Cookie:")) || bytes.Contains(buf.Bytes(), []byte("\nSet-Cookie:")) {
		t.Errorf("header CRLF injection produced secondary header line! output: %q", out)
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	wsReq := "GET /ws HTTP/1.1\r\nHost: localhost\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
	opts := httpparser.DefaultParserOptions()

	req, err := httpparser.ParseRequest(bytes.NewBufferString(wsReq), opts)
	if err != nil {
		t.Fatalf("failed to parse websocket request: %v", err)
	}

	if !req.IsWebSocketUpgrade() {
		t.Errorf("expected IsWebSocketUpgrade() to return true")
	}

	normalReq := "GET /api HTTP/1.1\r\nHost: localhost\r\n\r\n"
	nReq, err := httpparser.ParseRequest(bytes.NewBufferString(normalReq), opts)
	if err != nil {
		t.Fatalf("failed to parse normal request: %v", err)
	}

	if nReq.IsWebSocketUpgrade() {
		t.Errorf("expected IsWebSocketUpgrade() to return false for normal request")
	}

	h2WSReq, err := httpparser.NewRequest("CONNECT", "/ws", "HTTP/2.0")
	if err != nil {
		t.Fatalf("failed to create h2 connect req: %v", err)
	}
	h2WSReq.Header.Set(":protocol", "websocket")
	if !h2WSReq.IsWebSocketUpgrade() {
		t.Errorf("expected IsWebSocketUpgrade() to return true for RFC 8441 Extended CONNECT")
	}
}

func TestNewRequestFromStd(t *testing.T) {
	stdReq, err := http.NewRequest("GET", "https://example.com/api/v1/trades?symbol=TCS&limit=25", nil)
	if err != nil {
		t.Fatalf("failed to create std request: %v", err)
	}
	stdReq.Header.Set("User-Agent", "Go-Test")
	stdReq.Header.Set(":protocol", "websocket")

	toronReq := httpparser.NewRequestFromStd(stdReq)
	if toronReq == nil {
		t.Fatal("expected non-nil Request")
	}

	if toronReq.Method != "GET" {
		t.Errorf("expected method GET, got %q", toronReq.Method)
	}
	if toronReq.Path != "/api/v1/trades" {
		t.Errorf("expected path /api/v1/trades, got %q", toronReq.Path)
	}
	if toronReq.QueryParams.Get("symbol") != "TCS" {
		t.Errorf("expected query param symbol=TCS, got %q", toronReq.QueryParams.Get("symbol"))
	}
	if toronReq.QueryParams.Get("limit") != "25" {
		t.Errorf("expected query param limit=25, got %q", toronReq.QueryParams.Get("limit"))
	}
	if toronReq.URL.RawQuery != "symbol=TCS&limit=25" {
		t.Errorf("expected RawQuery 'symbol=TCS&limit=25', got %q", toronReq.URL.RawQuery)
	}
	if toronReq.Header.Get("User-Agent") != "Go-Test" {
		t.Errorf("expected Header User-Agent=Go-Test, got %q", toronReq.Header.Get("User-Agent"))
	}
	toronReq.Method = "CONNECT"
	if !toronReq.IsWebSocketUpgrade() {
		t.Errorf("expected IsWebSocketUpgrade to be true from :protocol header")
	}
}

func TestRequest_QueryLazy(t *testing.T) {
	stdReq, _ := http.NewRequest("GET", "/test?flag=1", nil)
	req := &httpparser.Request{
		URL: stdReq.URL,
	}

	q := req.Query()
	if q == nil || q.Get("flag") != "1" {
		t.Errorf("expected Query() to lazily parse URL query, got %v", q)
	}
}

func TestParseRequest_HeaderFieldNameWhitespaceRejection(t *testing.T) {
	opts := httpparser.DefaultParserOptions()

	tests := []struct {
		name        string
		rawReq      string
		expectError bool
	}{
		{
			name:        "TC-073-01: Transfer-Encoding Space Before Colon",
			rawReq:      "GET / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding : chunked\r\n\r\n",
			expectError: true,
		},
		{
			name:        "TC-073-02: Header Tab Before Colon",
			rawReq:      "GET / HTTP/1.1\r\nHost\t: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Leading Space Before Header Name",
			rawReq:      "GET / HTTP/1.1\r\n Host: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Empty Header Name",
			rawReq:      "GET / HTTP/1.1\r\n: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "TC-073-03: Valid Header Whitespace Handling",
			rawReq:      "GET / HTTP/1.1\r\nHost: example.com\r\nX-Custom:  value with spaces  \r\n\r\n",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := httpparser.ParseRequest(bytes.NewBufferString(tt.rawReq), opts)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for %s, but got none", tt.name)
				}
				if !errors.Is(err, httpparser.ErrBadRequest) {
					t.Fatalf("expected error wrapping ErrBadRequest, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %s: %v", err, tt.name)
				}
				if req.Header.Get("X-Custom") != "value with spaces" {
					t.Errorf("expected trimmed value 'value with spaces', got %q", req.Header.Get("X-Custom"))
				}
			}
		})
	}
}

func TestParseRequest_ContentLengthMultiplicityAndConflict(t *testing.T) {
	opts := httpparser.DefaultParserOptions()

	tests := []struct {
		name         string
		rawReq       string
		expectError  bool
		expectedBody string
	}{
		{
			name:        "TC-074-01: Conflicting Multiple Content-Length Headers",
			rawReq:      "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\nContent-Length: 45\r\n\r\n",
			expectError: true,
		},
		{
			name:        "TC-074-02: Comma-Separated Distinct Content-Length",
			rawReq:      "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Length: 10, 20\r\n\r\n0123456789",
			expectError: true,
		},
		{
			name:         "TC-074-03: Duplicate Identical Content-Length Headers",
			rawReq:       "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Length: 10\r\nContent-Length: 10\r\n\r\n0123456789",
			expectError:  false,
			expectedBody: "0123456789",
		},
		{
			name:         "Duplicate Identical Comma-Separated Content-Length",
			rawReq:       "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Length: 10, 10\r\n\r\n0123456789",
			expectError:  false,
			expectedBody: "0123456789",
		},
		{
			name:        "Invalid Non-Numeric Content-Length",
			rawReq:      "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Length: abc\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Negative Content-Length",
			rawReq:      "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Length: -5\r\n\r\n",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := httpparser.ParseRequest(bytes.NewBufferString(tt.rawReq), opts)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for %s, but got none", tt.name)
				}
				if !errors.Is(err, httpparser.ErrBadRequest) {
					t.Fatalf("expected ErrBadRequest, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %s: %v", err, tt.name)
				}
				if req.Body == nil {
					t.Fatal("expected non-nil Body")
				}
				body, _ := io.ReadAll(req.Body)
				if string(body) != tt.expectedBody {
					t.Errorf("expected body %q, got %q", tt.expectedBody, string(body))
				}
				if req.Header.Get("Content-Length") != "10" {
					t.Errorf("expected normalized Content-Length '10', got %q", req.Header.Get("Content-Length"))
				}
			}
		})
	}
}

// --- In-Process Microbenchmarks (TST-05 / REQ-105) ---

// BenchmarkParseRequest_WhitespaceRejection measures nanosecond-level fail-fast rejection
// of RFC 7230 §3.2.4 whitespace violations preceding the field colon.
func BenchmarkParseRequest_WhitespaceRejection(b *testing.B) {
	payload := []byte("GET / HTTP/1.1\r\nHost : example.com\r\nUser-Agent: toron-bench\r\n\r\n")
	opts := httpparser.DefaultParserOptions()
	r := bytes.NewReader(payload)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(payload)
		_, err := httpparser.ParseRequest(r, opts)
		if err == nil {
			b.Fatal("expected whitespace rejection error, got nil")
		}
	}
}

// BenchmarkParseRequest_MultipleCL measures nanosecond-level fail-fast rejection
// of RFC 7230 §3.3.2 conflicting multiple Content-Length headers.
func BenchmarkParseRequest_MultipleCL(b *testing.B) {
	payload := []byte("POST /api/v1/submit HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nContent-Length: 10\r\n\r\nhello")
	opts := httpparser.DefaultParserOptions()
	r := bytes.NewReader(payload)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(payload)
		_, err := httpparser.ParseRequest(r, opts)
		if err == nil {
			b.Fatal("expected multiple Content-Length rejection error, got nil")
		}
	}
}

// BenchmarkParseRequest_ValidBaseline measures baseline parsing latency and memory allocation profile
// of a valid RFC 7230 HTTP/1.1 request for comparative ablation.
func BenchmarkParseRequest_ValidBaseline(b *testing.B) {
	payload := []byte("GET /api/v1/resource HTTP/1.1\r\nHost: example.com\r\nUser-Agent: toron-bench\r\nAccept: application/json\r\n\r\n")
	opts := httpparser.DefaultParserOptions()
	r := bytes.NewReader(payload)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(payload)
		_, err := httpparser.ParseRequest(r, opts)
		if err != nil {
			b.Fatalf("unexpected parse error on valid baseline request: %v", err)
		}
	}
}

// TC-108-05: Verify *bufio.Reader preservation and reuse across consecutive ParseRequest calls
func TestParseRequest_BufioReaderReuse(t *testing.T) {
	pipelinedData := "GET /first HTTP/1.1\r\nHost: localhost\r\n\r\nGET /second HTTP/1.1\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(bytes.NewBufferString(pipelinedData))
	opts := httpparser.DefaultParserOptions()

	// Parse first request
	req1, err := httpparser.ParseRequest(br, opts)
	if err != nil {
		t.Fatalf("unexpected error parsing request 1: %v", err)
	}
	if req1.Path != "/first" {
		t.Errorf("expected path /first, got %s", req1.Path)
	}

	// Unconsumed bytes of request 2 must remain in br
	if br.Buffered() == 0 {
		t.Fatalf("expected buffered bytes remaining in br, got 0")
	}

	// Parse second request using the EXACT same *bufio.Reader
	req2, err := httpparser.ParseRequest(br, opts)
	if err != nil {
		t.Fatalf("unexpected error parsing request 2 from preserved bufio.Reader: %v", err)
	}
	if req2.Path != "/second" {
		t.Errorf("expected path /second, got %s", req2.Path)
	}
}

// TC-109-01 through TC-109-04: Header field-name token grammar validation (RFC 7230 §3.2)
func TestParseRequest_HeaderFieldNameTokenGrammar(t *testing.T) {
	opts := httpparser.DefaultParserOptions()

	tests := []struct {
		name        string
		rawReq      string
		expectError bool
	}{
		// TC-109-01: RFC 7230 Delimiters
		{
			name:        "Delimiter @",
			rawReq:      "GET / HTTP/1.1\r\nHost@Domain: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Parentheses ( )",
			rawReq:      "GET / HTTP/1.1\r\nHeader(Name): example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Comma ,",
			rawReq:      "GET / HTTP/1.1\r\nHeader,Name: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Slash /",
			rawReq:      "GET / HTTP/1.1\r\nHeader/Name: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Semicolon ;",
			rawReq:      "GET / HTTP/1.1\r\nHeader;Name: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Angle Brackets < >",
			rawReq:      "GET / HTTP/1.1\r\nHeader<Name>: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Equal Sign =",
			rawReq:      "GET / HTTP/1.1\r\nHeader=Name: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Question Mark ?",
			rawReq:      "GET / HTTP/1.1\r\nHeader?Name: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Square Brackets [ ]",
			rawReq:      "GET / HTTP/1.1\r\nHeader[Name]: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Backslash \\",
			rawReq:      "GET / HTTP/1.1\r\nHeader\\Name: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Curly Braces { }",
			rawReq:      "GET / HTTP/1.1\r\nHeader{Name}: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Delimiter Double Quote \"",
			rawReq:      "GET / HTTP/1.1\r\nHeader\"Name: example.com\r\n\r\n",
			expectError: true,
		},
		// TC-109-02: ASCII Control Characters
		{
			name:        "Control Byte NUL (0x00)",
			rawReq:      "GET / HTTP/1.1\r\nHead\x00er: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Control Byte BEL (0x07)",
			rawReq:      "GET / HTTP/1.1\r\nHead\x07er: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Control Byte ESC (0x1B)",
			rawReq:      "GET / HTTP/1.1\r\nHead\x1Ber: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "Control Byte DEL (0x7F)",
			rawReq:      "GET / HTTP/1.1\r\nHead\x7Fer: example.com\r\n\r\n",
			expectError: true,
		},
		// TC-109-03: High-Bit Non-ASCII Bytes
		{
			name:        "High-Bit Byte 0x80",
			rawReq:      "GET / HTTP/1.1\r\nHead\x80er: example.com\r\n\r\n",
			expectError: true,
		},
		{
			name:        "High-Bit Byte 0xFF",
			rawReq:      "GET / HTTP/1.1\r\nHead\xFFer: example.com\r\n\r\n",
			expectError: true,
		},
		// TC-109-04: Valid Complex RFC 7230 Tokens
		{
			name:        "Valid RFC 7230 Special Characters",
			rawReq:      "GET / HTTP/1.1\r\nHost: example.com\r\nX-Custom_Header.1!#$%&'*+-.^_`|~: special-token-value\r\n\r\n",
			expectError: false,
		},
		{
			name:        "Valid Standard Headers",
			rawReq:      "GET / HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\nAccept-Encoding: gzip, deflate\r\n\r\n",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := httpparser.ParseRequest(bytes.NewBufferString(tt.rawReq), opts)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tt.name)
				}
				if !errors.Is(err, httpparser.ErrBadRequest) {
					t.Fatalf("expected error wrapping ErrBadRequest for %s, got %v", tt.name, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %s: %v", tt.name, err)
				}
				if tt.name == "Valid RFC 7230 Special Characters" {
					expectedVal := "special-token-value"
					gotVal := req.Header.Get("X-Custom_Header.1!#$%&'*+-.^_`|~")
					if gotVal != expectedVal {
						t.Errorf("expected header value %q, got %q", expectedVal, gotVal)
					}
				}
			}
		})
	}
}

// TC-109-05: Nanosecond In-Process Microbenchmark for Invalid Header Token Rejection
func BenchmarkParseRequest_InvalidTokenRejection(b *testing.B) {
	payload := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nX-Invalid@Header: test-value\r\n\r\n")
	opts := httpparser.DefaultParserOptions()
	r := bytes.NewReader(payload)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(payload)
		_, err := httpparser.ParseRequest(r, opts)
		if err == nil {
			b.Fatal("expected invalid token rejection error, got nil")
		}
	}
}

// TC-110-01: Standard Payload Body Buffer Pooling (<= 64 KB)
func TestParseRequest_PooledBodyLifecycle(t *testing.T) {
	opts := httpparser.DefaultParserOptions()
	bodyData := bytes.Repeat([]byte("test-payload-data-"), 100) // ~1.9 KB
	rawReq := fmt.Sprintf("POST /api/upload HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\n\r\n%s", len(bodyData), string(bodyData))

	req, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	readBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if !bytes.Equal(readBody, bodyData) {
		t.Fatalf("body data mismatch: expected %d bytes, got %d", len(bodyData), len(readBody))
	}

	if err := req.CloseBody(); err != nil {
		t.Fatalf("unexpected CloseBody error: %v", err)
	}

	// Immediate reuse from pool should yield clean buffer without corruption
	bodyData2 := []byte("second-request-body-payload")
	rawReq2 := fmt.Sprintf("POST /api/upload2 HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\n\r\n%s", len(bodyData2), string(bodyData2))

	req2, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq2), opts)
	if err != nil {
		t.Fatalf("unexpected second parse error: %v", err)
	}

	readBody2, err := io.ReadAll(req2.Body)
	if err != nil {
		t.Fatalf("failed to read body 2: %v", err)
	}
	if !bytes.Equal(readBody2, bodyData2) {
		t.Fatalf("second body mismatch: expected %q, got %q", string(bodyData2), string(readBody2))
	}
	_ = req2.CloseBody()
}

// TC-110-02: Large Payload Heap Allocation Fallback (> 64 KB)
func TestParseRequest_LargeBodyFallback(t *testing.T) {
	opts := httpparser.DefaultParserOptions()
	largeBody := bytes.Repeat([]byte("large-payload-chunk-"), 5000) // ~100 KB > 64 KB
	rawReq := fmt.Sprintf("POST /api/large HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\n\r\n%s", len(largeBody), string(largeBody))

	req, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
	if err != nil {
		t.Fatalf("unexpected parse error on large payload: %v", err)
	}

	readBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read large body: %v", err)
	}
	if !bytes.Equal(readBody, largeBody) {
		t.Fatalf("large body mismatch: expected %d bytes, got %d", len(largeBody), len(readBody))
	}

	if err := req.CloseBody(); err != nil {
		t.Fatalf("CloseBody failed on large payload: %v", err)
	}
}

// TC-110-03: Idempotent Close and Double-Free Prevention
func TestParseRequest_IdempotentClose(t *testing.T) {
	opts := httpparser.DefaultParserOptions()
	rawReq := "POST /api/idempotent HTTP/1.1\r\nHost: localhost\r\nContent-Length: 5\r\n\r\nhello"

	req, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close multiple times in succession
	for i := 0; i < 5; i++ {
		if err := req.CloseBody(); err != nil {
			t.Fatalf("iteration %d CloseBody returned error: %v", i, err)
		}
	}
}

// TC-110-04: Concurrent Pool Race Safety
func TestParseRequest_ConcurrentPoolRace(t *testing.T) {
	opts := httpparser.DefaultParserOptions()
	var wg sync.WaitGroup
	numWorkers := 30
	iterations := 20

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				content := fmt.Sprintf("worker-%d-iter-%d-payload", workerID, j)
				rawReq := fmt.Sprintf("POST /api/race HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\n\r\n%s", len(content), content)
				req, err := httpparser.ParseRequest(bytes.NewBufferString(rawReq), opts)
				if err != nil {
					t.Errorf("worker %d parse error: %v", workerID, err)
					return
				}
				readBytes, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("worker %d read error: %v", workerID, err)
					return
				}
				if string(readBytes) != content {
					t.Errorf("worker %d data corruption: expected %q, got %q", workerID, content, string(readBytes))
					return
				}
				_ = req.CloseBody()
			}
		}(i)
	}

	wg.Wait()
}

// TC-110-05: Allocation Reduction Microbenchmark for Pooled Body Reading
func BenchmarkParseRequest_PooledBody(b *testing.B) {
	body := bytes.Repeat([]byte("a"), 1024)
	raw := fmt.Sprintf("POST /submit HTTP/1.1\r\nHost: example.com\r\nContent-Length: %d\r\n\r\n%s", len(body), string(body))
	payload := []byte(raw)
	opts := httpparser.DefaultParserOptions()
	r := bytes.NewReader(payload)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Reset(payload)
		req, err := httpparser.ParseRequest(r, opts)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_ = req.CloseBody()
	}
}
