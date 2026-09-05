package httpparser_test

import (
	"bytes"
	"io"
	"net/http"
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

