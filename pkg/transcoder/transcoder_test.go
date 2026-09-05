package transcoder

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
