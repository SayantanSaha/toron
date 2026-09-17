package transcoder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"toron/pkg/config"
	"toron/pkg/httpparser"
	"toron/pkg/router"
)

func TestEncodeDecodeGRPCFrame(t *testing.T) {
	rawPayload := []byte(`{"id":"user-123","name":"John Doe"}`)
	framed := EncodeGRPCFrame(rawPayload)

	if len(framed) != 5+len(rawPayload) {
		t.Fatalf("Framed length = %d, want %d", len(framed), 5+len(rawPayload))
	}

	if framed[0] != 0x00 {
		t.Errorf("Compressed byte = %x, want 0x00", framed[0])
	}

	decoded, err := DecodeGRPCFrame(bytes.NewReader(framed))
	if err != nil {
		t.Fatalf("DecodeGRPCFrame failed: %v", err)
	}

	if string(decoded) != string(rawPayload) {
		t.Errorf("Decoded payload = %q, want %q", string(decoded), string(rawPayload))
	}
}

func TestMapGRPCStatusToHTTP(t *testing.T) {
	tests := []struct {
		grpcStatus string
		wantHTTP   int
	}{
		{"0", http.StatusOK},
		{"3", http.StatusBadRequest},
		{"5", http.StatusNotFound},
		{"7", http.StatusForbidden},
		{"14", http.StatusServiceUnavailable},
		{"16", http.StatusUnauthorized},
		{"99", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run("status-"+tt.grpcStatus, func(t *testing.T) {
			got := MapGRPCStatusToHTTP(tt.grpcStatus)
			if got != tt.wantHTTP {
				t.Errorf("MapGRPCStatusToHTTP(%q) = %d, want %d", tt.grpcStatus, got, tt.wantHTTP)
			}
		})
	}
}

func TestExtractPathParams(t *testing.T) {
	pattern := "/v1/users/:id/details"
	path := "/v1/users/usr-999/details"

	params := extractPathParams(pattern, path)
	if params["id"] != "usr-999" {
		t.Errorf("params[\"id\"] = %q, want usr-999", params["id"])
	}
}

func TestTranscoderEngineMockGRPC(t *testing.T) {
	// Mock gRPC upstream server
	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user.UserService/GetUser" {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		// Read request wire frame
		frameBytes, err := DecodeGRPCFrame(r.Body)
		if err != nil {
			http.Error(w, "Bad gRPC Frame", http.StatusBadRequest)
			return
		}

		var reqData map[string]interface{}
		_ = json.Unmarshal(frameBytes, &reqData)

		userID, _ := reqData["id"].(string)

		resData := map[string]interface{}{
			"id":     userID,
			"name":   "Alice Bob",
			"status": "active",
		}
		resJSON, _ := json.Marshal(resData)

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write(EncodeGRPCFrame(resJSON))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id",
				GRPCMethod:  "/user.UserService/GetUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	req := &httpparser.Request{
		Method: "GET",
		Path:   "/v1/users/usr-777",
		Header: make(httpparser.Header),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200", res.StatusCode)
	}

	var jsonOut map[string]interface{}
	bodyBytes, _ := io.ReadAll(res.Body)
	if err := json.Unmarshal(bodyBytes, &jsonOut); err != nil {
		t.Fatalf("Failed to decode response JSON: %v (body: %s)", err, string(bodyBytes))
	}

	if jsonOut["id"] != "usr-777" {
		t.Errorf("JSON id = %q, want usr-777", jsonOut["id"])
	}
	if jsonOut["name"] != "Alice Bob" {
		t.Errorf("JSON name = %q, want Alice Bob", jsonOut["name"])
	}
}

func TestDecodeGRPCFrame_OversizedFrameRejected(t *testing.T) {
	// Craft a malicious wire frame declaring 4 GB (0xFFFFFFFF) length
	buf := make([]byte, 5)
	buf[0] = 0x00 // uncompressed
	buf[1] = 0xFF
	buf[2] = 0xFF
	buf[3] = 0xFF
	buf[4] = 0xFF

	reader := bytes.NewReader(buf)
	_, err := DecodeGRPCFrame(reader)
	if err == nil {
		t.Fatal("expected error for 4GB frame length, got nil")
	}

	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got: %v", err)
	}
}

func TestDecodeGRPCFrameWithLimit(t *testing.T) {
	payload := []byte("hello world")
	framed := EncodeGRPCFrame(payload)

	// Test 1: Limit lower than payload size (11 bytes payload vs 5 byte limit)
	_, err := DecodeGRPCFrameWithLimit(bytes.NewReader(framed), 5)
	if err == nil {
		t.Fatal("expected ErrFrameTooLarge when payload exceeds limit")
	}
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}

	// Test 2: Limit equal or higher than payload size
	decoded, err := DecodeGRPCFrameWithLimit(bytes.NewReader(framed), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(decoded) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(decoded))
	}
}

func TestHandleTranscode_OversizedUpstreamFrame(t *testing.T) {
	// Upstream gRPC server returning oversized frame header
	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)

		// Header declaring 10 MB payload (> 4 MB DefaultMaxGRPCFrameSize)
		oversizedHeader := make([]byte, 5)
		oversizedHeader[0] = 0x00
		oversizedHeader[1] = 0x00
		oversizedHeader[2] = 0xA0
		oversizedHeader[3] = 0x00
		oversizedHeader[4] = 0x00 // 10,485,760 bytes
		_, _ = w.Write(oversizedHeader)
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/large",
				GRPCMethod:  "/large.Service/GetLarge",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	req := &httpparser.Request{
		Method: "GET",
		Path:   "/v1/large",
		Header: make(httpparser.Header),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("StatusCode = %d, want 502 Bad Gateway", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "502 Bad Gateway") {
		t.Errorf("expected 502 Bad Gateway error message, got: %s", string(body))
	}
}

// TC-089-01: Declared Content-Length Fast-Fail Rejection (SEC-28)
func TestHandleTranscode_DeclaredContentLength_FastFail(t *testing.T) {
	var upstreamCalls int32

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled:      true,
		MaxBodyBytes: 1024, // 1 KB limit
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/users",
				GRPCMethod:  "/user.UserService/CreateUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Payload declaring 2048 bytes (> 1024 maxBodyBytes)
	req := &httpparser.Request{
		Method:        "POST",
		Path:          "/v1/users",
		ContentLength: 2048,
		Header:        httpparser.Header{"content-length": []string{"2048"}},
		Body:          bytes.NewReader(make([]byte, 2048)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("StatusCode = %d, want 413 (StatusRequestEntityTooLarge)", res.StatusCode)
	}

	if res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", res.Header.Get("Content-Type"))
	}

	bodyBytes, _ := io.ReadAll(res.Body)
	expectedMsg := "Payload Too Large: request Content-Length 2048 exceeds limit of 1024 bytes"
	if !strings.Contains(string(bodyBytes), expectedMsg) {
		t.Errorf("expected body to contain %q, got %q", expectedMsg, string(bodyBytes))
	}

	// Assert upstream gRPC received 0 calls
	if calls := atomic.LoadInt32(&upstreamCalls); calls != 0 {
		t.Errorf("expected upstream calls to be 0, got %d", calls)
	}
}

// TC-089-02: Bounded Stream Over-Read Rejection (SEC-28)
func TestHandleTranscode_StreamOverRead_Rejection(t *testing.T) {
	var upstreamCalls int32

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled:      true,
		MaxBodyBytes: 1024, // 1 KB limit
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/users",
				GRPCMethod:  "/user.UserService/CreateUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Streaming request with undeclared ContentLength (0), but body delivers 1500 bytes
	req := &httpparser.Request{
		Method:        "POST",
		Path:          "/v1/users",
		ContentLength: 0,
		Header:        make(httpparser.Header),
		Body:          bytes.NewReader(make([]byte, 1500)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("StatusCode = %d, want 413 (StatusRequestEntityTooLarge)", res.StatusCode)
	}

	if res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", res.Header.Get("Content-Type"))
	}

	bodyBytes, _ := io.ReadAll(res.Body)
	expectedMsg := "Payload Too Large: request body exceeds limit of 1024 bytes"
	if !strings.Contains(string(bodyBytes), expectedMsg) {
		t.Errorf("expected body to contain %q, got %q", expectedMsg, string(bodyBytes))
	}

	// Assert upstream gRPC received 0 calls
	if calls := atomic.LoadInt32(&upstreamCalls); calls != 0 {
		t.Errorf("expected upstream calls to be 0, got %d", calls)
	}
}

// TC-089-04: Full-Fidelity In-Limit Payload Forwarding (SEC-28)
func TestHandleTranscode_InLimit_Success(t *testing.T) {
	var upstreamCalls int32
	var receivedPayload map[string]interface{}

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)

		frameBytes, err := DecodeGRPCFrame(r.Body)
		if err != nil {
			http.Error(w, "Bad gRPC Frame", http.StatusBadRequest)
			return
		}

		_ = json.Unmarshal(frameBytes, &receivedPayload)

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)

		resJSON, _ := json.Marshal(map[string]interface{}{
			"id":     receivedPayload["id"],
			"status": "created",
		})
		_, _ = w.Write(EncodeGRPCFrame(resJSON))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled:      true,
		MaxBodyBytes: 4096, // 4 KB limit
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/users",
				GRPCMethod:  "/user.UserService/CreateUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	validJSON := `{"id":"usr-42","name":"Valid User","role":"admin"}`
	req := &httpparser.Request{
		Method:        "POST",
		Path:          "/v1/users",
		ContentLength: int64(len(validJSON)),
		Header:        httpparser.Header{"content-length": []string{fmt.Sprintf("%d", len(validJSON))}},
		Body:          bytes.NewReader([]byte(validJSON)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200 OK", res.StatusCode)
	}

	if calls := atomic.LoadInt32(&upstreamCalls); calls != 1 {
		t.Errorf("expected upstream calls to be 1, got %d", calls)
	}

	if receivedPayload["id"] != "usr-42" || receivedPayload["name"] != "Valid User" {
		t.Errorf("upstream payload mismatch: got %v", receivedPayload)
	}

	resBytes, _ := io.ReadAll(res.Body)
	var resMap map[string]interface{}
	if err := json.Unmarshal(resBytes, &resMap); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	if resMap["status"] != "created" || resMap["id"] != "usr-42" {
		t.Errorf("unexpected response map: %v", resMap)
	}
}

// TC-089-05: Non-Mutating & Nil Body Request Bypass (SEC-28)
func TestHandleTranscode_NonMutatingNilBody_Bypass(t *testing.T) {
	var upstreamCalls int32

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)

		resJSON, _ := json.Marshal(map[string]interface{}{
			"id":     "usr-100",
			"status": "found",
		})
		_, _ = w.Write(EncodeGRPCFrame(resJSON))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled:      true,
		MaxBodyBytes: 1024,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id",
				GRPCMethod:  "/user.UserService/GetUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	req := &httpparser.Request{
		Method: "GET",
		Path:   "/v1/users/usr-100",
		Header: make(httpparser.Header),
		Body:   nil, // explicitly nil body
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200 OK", res.StatusCode)
	}

	if calls := atomic.LoadInt32(&upstreamCalls); calls != 1 {
		t.Errorf("expected upstream calls to be 1, got %d", calls)
	}
}

// TC-089-06: Concurrency & Race Safety Verification (SEC-28)
func TestHandleTranscode_ConcurrencyRaceSafety(t *testing.T) {
	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled:      true,
		MaxBodyBytes: 1024,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/items",
				GRPCMethod:  "/item.ItemService/ProcessItem",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	const totalGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

	for i := 0; i < totalGoroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()

			res := httpparser.NewResponse()
			if idx%2 == 0 {
				// Valid in-limit request
				validJSON := fmt.Sprintf(`{"item_id":"item-%d","count":%d}`, idx, idx)
				req := &httpparser.Request{
					Method:        "POST",
					Path:          "/v1/items",
					ContentLength: int64(len(validJSON)),
					Header:        httpparser.Header{"content-length": []string{fmt.Sprintf("%d", len(validJSON))}},
					Body:          bytes.NewReader([]byte(validJSON)),
				}
				engine.HandleTranscode(req, res, engine.rules[0])
				if res.StatusCode != http.StatusOK {
					t.Errorf("goroutine %d: want 200, got %d", idx, res.StatusCode)
				}
			} else {
				// Oversized request (2048 bytes > 1024)
				req := &httpparser.Request{
					Method:        "POST",
					Path:          "/v1/items",
					ContentLength: 2048,
					Header:        httpparser.Header{"content-length": []string{"2048"}},
					Body:          bytes.NewReader(make([]byte, 2048)),
				}
				engine.HandleTranscode(req, res, engine.rules[0])
				if res.StatusCode != http.StatusRequestEntityTooLarge {
					t.Errorf("goroutine %d: want 413, got %d", idx, res.StatusCode)
				}
			}
		}()
	}

	wg.Wait()
}

// TC-090-01: Standard Hop-by-Hop Header Stripping (SEC-29)
func TestHandleTranscode_StandardHopByHop_Stripped(t *testing.T) {
	var receivedHeaders http.Header
	var mu sync.Mutex

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/test",
				GRPCMethod:  "/test.TestService/TestMethod",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	hdr := make(httpparser.Header)
	hdr.Set("Connection", "keep-alive")
	hdr.Set("Keep-Alive", "timeout=10")
	hdr.Set("Upgrade", "websocket")
	hdr.Set("Proxy-Connection", "keep-alive")
	hdr.Set("Transfer-Encoding", "chunked")
	hdr.Set("Proxy-Authenticate", "Basic")
	hdr.Set("Proxy-Authorization", "Basic dXNlcjpwYXNz")
	hdr.Set("Trailer", "X-Custom-Trailer")

	req := &httpparser.Request{
		Method: "POST",
		Path:   "/v1/test",
		Header: hdr,
		Body:   bytes.NewReader([]byte(`{}`)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want status 200, got %d: %s", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	prohibited := []string{
		"Connection",
		"Keep-Alive",
		"Upgrade",
		"Proxy-Connection",
		"Transfer-Encoding",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Trailer",
		"Trailers",
	}

	for _, h := range prohibited {
		if val := receivedHeaders.Get(h); val != "" {
			t.Errorf("expected hop-by-hop header %q to be stripped, got %q", h, val)
		}
	}
}

// TC-090-02: Dynamic Connection Token Stripping (SEC-29)
func TestHandleTranscode_DynamicConnectionTokens_Stripped(t *testing.T) {
	var receivedHeaders http.Header
	var mu sync.Mutex

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/test",
				GRPCMethod:  "/test.TestService/TestMethod",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	hdr := make(httpparser.Header)
	hdr.Set("Connection", "X-Custom-Hop, X-Another-Hop, close")
	hdr.Set("X-Custom-Hop", "secret-token")
	hdr.Set("X-Another-Hop", "temporary-value")
	hdr.Set("X-Valid-Header", "keep-me")

	req := &httpparser.Request{
		Method: "POST",
		Path:   "/v1/test",
		Header: hdr,
		Body:   bytes.NewReader([]byte(`{}`)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want status 200, got %d: %s", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if val := receivedHeaders.Get("X-Custom-Hop"); val != "" {
		t.Errorf("expected dynamic hop-by-hop header X-Custom-Hop to be stripped, got %q", val)
	}
	if val := receivedHeaders.Get("X-Another-Hop"); val != "" {
		t.Errorf("expected dynamic hop-by-hop header X-Another-Hop to be stripped, got %q", val)
	}
	if val := receivedHeaders.Get("Connection"); val != "" {
		t.Errorf("expected Connection header to be stripped, got %q", val)
	}
	if val := receivedHeaders.Get("X-Valid-Header"); val != "keep-me" {
		t.Errorf("expected X-Valid-Header to be preserved as 'keep-me', got %q", val)
	}
}

// TC-090-03: Strict TE: trailers Invariant (SEC-29)
func TestHandleTranscode_TE_StrictTrailersInvariant(t *testing.T) {
	var receivedTE []string
	var mu sync.Mutex

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedTE = r.Header["Te"]
		if len(receivedTE) == 0 {
			receivedTE = r.Header["TE"]
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/test",
				GRPCMethod:  "/test.TestService/TestMethod",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	hdr := make(httpparser.Header)
	hdr.Set("TE", "gzip, deflate")

	req := &httpparser.Request{
		Method: "POST",
		Path:   "/v1/test",
		Header: hdr,
		Body:   bytes.NewReader([]byte(`{}`)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want status 200, got %d: %s", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedTE) != 1 || receivedTE[0] != "trailers" {
		t.Errorf("expected TE header to be exactly [trailers], got %v", receivedTE)
	}
}

// TC-090-04: Host Header Sanitization (SEC-29)
func TestHandleTranscode_HostHeader_Stripped(t *testing.T) {
	var receivedHostHeader string
	var mu sync.Mutex

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHostHeader = r.Header.Get("Host")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/test",
				GRPCMethod:  "/test.TestService/TestMethod",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	hdr := make(httpparser.Header)
	hdr.Set("Host", "malicious.client.com")

	req := &httpparser.Request{
		Method: "POST",
		Path:   "/v1/test",
		Header: hdr,
		Body:   bytes.NewReader([]byte(`{}`)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want status 200, got %d: %s", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedHostHeader == "malicious.client.com" {
		t.Errorf("expected client Host header to be stripped from Header, got %q", receivedHostHeader)
	}
}

// TC-090-05: Application Metadata Preservation (SEC-29)
func TestHandleTranscode_ApplicationMetadata_Preserved(t *testing.T) {
	var receivedHeaders http.Header
	var mu sync.Mutex

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/test",
				GRPCMethod:  "/test.TestService/TestMethod",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	hdr := make(httpparser.Header)
	hdr.Set("Authorization", "Bearer my-secret-jwt")
	hdr.Set("X-Request-Id", "req-abcdef-12345")
	hdr.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	hdr.Set("User-Agent", "MyCustomClient/1.0")

	req := &httpparser.Request{
		Method: "POST",
		Path:   "/v1/test",
		Header: hdr,
		Body:   bytes.NewReader([]byte(`{}`)),
	}
	res := httpparser.NewResponse()

	engine.HandleTranscode(req, res, engine.rules[0])

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want status 200, got %d: %s", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	expected := map[string]string{
		"Authorization": "Bearer my-secret-jwt",
		"X-Request-Id":  "req-abcdef-12345",
		"Traceparent":   "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"User-Agent":    "MyCustomClient/1.0",
	}

	for k, expectedVal := range expected {
		if got := receivedHeaders.Get(k); got != expectedVal {
			t.Errorf("header %q = %q, want %q", k, got, expectedVal)
		}
	}
}

// TC-090-06: Concurrency & Race Safety Verification (SEC-29)
func TestHandleTranscode_HeaderSanitization_ConcurrencyRaceSafety(t *testing.T) {
	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify hop-by-hop headers are never received upstream
		if r.Header.Get("Connection") != "" || r.Header.Get("Upgrade") != "" || r.Header.Get("X-Hop-Token") != "" {
			http.Error(w, "Hop-by-hop header leaked", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/v1/concurrent",
				GRPCMethod:  "/test.TestService/Concurrent",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	engine, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	const totalGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

	for i := 0; i < totalGoroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()

			hdr := make(httpparser.Header)
			if idx%2 == 0 {
				hdr.Set("Connection", "X-Hop-Token, keep-alive")
				hdr.Set("X-Hop-Token", fmt.Sprintf("val-%d", idx))
				hdr.Set("Upgrade", "websocket")
				hdr.Set("TE", "gzip")
			}
			hdr.Set("Authorization", fmt.Sprintf("Bearer token-%d", idx))
			hdr.Set("X-Request-Id", fmt.Sprintf("req-%d", idx))

			req := &httpparser.Request{
				Method: "POST",
				Path:   "/v1/concurrent",
				Header: hdr,
				Body:   bytes.NewReader([]byte(`{}`)),
			}
			res := httpparser.NewResponse()

			engine.HandleTranscode(req, res, engine.rules[0])

			if res.StatusCode != http.StatusOK {
				t.Errorf("goroutine %d: want status 200, got %d", idx, res.StatusCode)
			}
		}()
	}

	wg.Wait()
}

// TC-091-01: Direct Parameterized Subpath Dispatch via router.ServeHTTP (SEC-30)
func TestTranscoder_ParameterizedSubpathDispatch(t *testing.T) {
	var grpcCalls int64
	var requestedPath string
	var requestedID string
	var mu sync.Mutex

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&grpcCalls, 1)

		mu.Lock()
		requestedPath = r.URL.Path
		mu.Unlock()

		frameBytes, err := DecodeGRPCFrame(r.Body)
		if err != nil {
			http.Error(w, "Bad gRPC Frame", http.StatusBadRequest)
			return
		}

		var reqData map[string]interface{}
		_ = json.Unmarshal(frameBytes, &reqData)

		mu.Lock()
		if idVal, ok := reqData["id"].(string); ok {
			requestedID = idVal
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)

		resJSON, _ := json.Marshal(map[string]interface{}{
			"id":     reqData["id"],
			"status": "active",
		})
		_, _ = w.Write(EncodeGRPCFrame(resJSON))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id",
				GRPCMethod:  "/user.UserService/GetUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	_, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Dispatch request directly through router.ServeHTTP (verifying SEC-30 remediation)
	req, _ := httpparser.NewRequest("GET", "/v1/users/usr-777", "HTTP/1.1")
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want status 200 OK (not 502 Bad Gateway), got %d: %s", res.StatusCode, res.Body.String())
	}

	mu.Lock()
	path := requestedPath
	id := requestedID
	mu.Unlock()

	if path != "/user.UserService/GetUser" {
		t.Errorf("upstream path = %q, want /user.UserService/GetUser", path)
	}
	if id != "usr-777" {
		t.Errorf("upstream payload id = %q, want usr-777", id)
	}
	if calls := atomic.LoadInt64(&grpcCalls); calls != 1 {
		t.Errorf("grpcCalls = %d, want 1", calls)
	}

	var resData map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &resData); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if resData["id"] != "usr-777" {
		t.Errorf("response id = %v, want usr-777", resData["id"])
	}
}

// TC-091-02: Multi-Level Route Segregation on Shared Prefix (SEC-30)
func TestTranscoder_MultiLevelRouteSegregation(t *testing.T) {
	var userCalls, orderCalls int64

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frameBytes, _ := DecodeGRPCFrame(r.Body)
		var reqData map[string]interface{}
		_ = json.Unmarshal(frameBytes, &reqData)

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)

		if r.URL.Path == "/user.UserService/GetUser" {
			atomic.AddInt64(&userCalls, 1)
			resJSON, _ := json.Marshal(map[string]interface{}{"user_id": reqData["id"], "type": "user"})
			_, _ = w.Write(EncodeGRPCFrame(resJSON))
			return
		}

		if r.URL.Path == "/order.OrderService/GetOrder" {
			atomic.AddInt64(&orderCalls, 1)
			resJSON, _ := json.Marshal(map[string]interface{}{"user_id": reqData["id"], "order_id": reqData["orderId"], "type": "order"})
			_, _ = w.Write(EncodeGRPCFrame(resJSON))
			return
		}

		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id",
				GRPCMethod:  "/user.UserService/GetUser",
				UpstreamURL: grpcServer.URL,
			},
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id/orders/:orderId",
				GRPCMethod:  "/order.OrderService/GetOrder",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	_, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// 1. Dispatch Request A (single parameter)
	reqA, _ := httpparser.NewRequest("GET", "/v1/users/usr-100", "HTTP/1.1")
	resA := httpparser.NewResponse()
	r.ServeHTTP(reqA, resA)

	if resA.StatusCode != http.StatusOK {
		t.Fatalf("resA: want 200, got %d: %s", resA.StatusCode, resA.Body.String())
	}

	// 2. Dispatch Request B (multi-level parameter)
	reqB, _ := httpparser.NewRequest("GET", "/v1/users/usr-100/orders/ord-500", "HTTP/1.1")
	resB := httpparser.NewResponse()
	r.ServeHTTP(reqB, resB)

	if resB.StatusCode != http.StatusOK {
		t.Fatalf("resB: want 200, got %d: %s", resB.StatusCode, resB.Body.String())
	}

	if atomic.LoadInt64(&userCalls) != 1 {
		t.Errorf("userCalls = %d, want 1", userCalls)
	}
	if atomic.LoadInt64(&orderCalls) != 1 {
		t.Errorf("orderCalls = %d, want 1", orderCalls)
	}
}

// TC-091-03: Method Gating on Parameterized Subpath (SEC-30)
func TestTranscoder_WrongMethodOnParameterizedRoute(t *testing.T) {
	var grpcCalls int64

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&grpcCalls, 1)
		w.Header().Set("Content-Type", "application/grpc")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id",
				GRPCMethod:  "/user.UserService/GetUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	_, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Issue POST to GET-only parameterized route
	req, _ := httpparser.NewRequest("POST", "/v1/users/usr-777", "HTTP/1.1")
	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("want 405 Method Not Allowed, got %d: %s", res.StatusCode, res.Body.String())
	}

	if calls := atomic.LoadInt64(&grpcCalls); calls != 0 {
		t.Errorf("expected 0 upstream calls on 405, got %d", calls)
	}
}

// TC-091-04: Segment Count and Pattern Mismatch Fall-Through (SEC-30)
func TestTranscoder_SegmentCountMismatch_NotFound(t *testing.T) {
	var grpcCalls int64

	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&grpcCalls, 1)
		w.Header().Set("Content-Type", "application/grpc")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id",
				GRPCMethod:  "/user.UserService/GetUser",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	_, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Request 1: Missing parameter segment (/v1/users)
	req1, _ := httpparser.NewRequest("GET", "/v1/users", "HTTP/1.1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusNotFound {
		t.Errorf("req1: want 404, got %d", res1.StatusCode)
	}

	// Request 2: Excess segments (/v1/users/usr-777/extra)
	req2, _ := httpparser.NewRequest("GET", "/v1/users/usr-777/extra", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusNotFound {
		t.Errorf("req2: want 404, got %d", res2.StatusCode)
	}

	if calls := atomic.LoadInt64(&grpcCalls); calls != 0 {
		t.Errorf("expected 0 upstream calls on 404, got %d", calls)
	}
}

// TC-091-05: Unit Test Coverage for MatchPathPattern (SEC-30)
func TestTranscoder_MatchPathPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/v1/users/:id", "/v1/users/usr-123", true},
		{"/v1/users/:id", "/v1/users/usr-123/", true},
		{"/v1/users/:id", "/v1/users", false},
		{"/v1/users/:id", "/v1/users/usr-123/extra", false},
		{"/v1/users/:id", "/v1/items/usr-123", false},
		{"/v1/orgs/:org/users/:user", "/v1/orgs/acme/users/alice", true},
		{"/v1/orgs/:org/users/:user", "/v1/orgs/acme/users", false},
		{"/v1/orgs/:org/users/:user", "/v1/orgs/acme/projects/alice", false},
		{"/v1/items/:id/details", "/v1/items/itm-999/details", true},
		{"/v1/items/:id/details", "/v1/items/itm-999/settings", false},
		{"/v1/users/:id", "/v1/users//", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.pattern, tt.path), func(t *testing.T) {
			got := MatchPathPattern(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("MatchPathPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// TC-091-06: Concurrency & Race Safety for Parameterized Subpaths (SEC-30)
func TestTranscoder_ParameterizedSubpath_ConcurrencyRaceSafety(t *testing.T) {
	grpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(EncodeGRPCFrame([]byte(`{"status":"ok"}`)))
	}))
	defer grpcServer.Close()

	cfg := config.TranscoderConfig{
		Enabled: true,
		Routes: []config.TranscoderRouteRule{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id",
				GRPCMethod:  "/user.UserService/GetUser",
				UpstreamURL: grpcServer.URL,
			},
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/v1/users/:id/orders/:orderId",
				GRPCMethod:  "/order.OrderService/GetOrder",
				UpstreamURL: grpcServer.URL,
			},
		},
	}

	r := router.New()
	_, err := NewEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	const totalGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

	for i := 0; i < totalGoroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()

			res := httpparser.NewResponse()
			switch idx % 3 {
			case 0:
				// Valid single-parameter request
				req, _ := httpparser.NewRequest("GET", fmt.Sprintf("/v1/users/user-%d", idx), "HTTP/1.1")
				r.ServeHTTP(req, res)
				if res.StatusCode != http.StatusOK {
					t.Errorf("goroutine %d: want 200, got %d", idx, res.StatusCode)
				}
			case 1:
				// Valid multi-level parameter request
				req, _ := httpparser.NewRequest("GET", fmt.Sprintf("/v1/users/user-%d/orders/order-%d", idx, idx), "HTTP/1.1")
				r.ServeHTTP(req, res)
				if res.StatusCode != http.StatusOK {
					t.Errorf("goroutine %d: want 200, got %d", idx, res.StatusCode)
				}
			case 2:
				// Wrong method request -> 405
				req, _ := httpparser.NewRequest("POST", fmt.Sprintf("/v1/users/user-%d", idx), "HTTP/1.1")
				r.ServeHTTP(req, res)
				if res.StatusCode != http.StatusMethodNotAllowed {
					t.Errorf("goroutine %d: want 405, got %d", idx, res.StatusCode)
				}
			}
		}()
	}

	wg.Wait()
}
