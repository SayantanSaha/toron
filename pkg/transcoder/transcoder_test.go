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

