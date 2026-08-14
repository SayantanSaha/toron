package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"toron/pkg/httpparser"
)

func TestGRPCHealth_Encoding(t *testing.T) {
	// 1. Empty service
	frameEmpty := EncodeGRPCHealthCheckRequest("")
	if len(frameEmpty) != 5 {
		t.Fatalf("expected 5-byte frame for empty service, got %d bytes", len(frameEmpty))
	}
	if frameEmpty[0] != 0x00 {
		t.Fatalf("expected compression flag 0x00, got 0x%x", frameEmpty[0])
	}
	if length := binary.BigEndian.Uint32(frameEmpty[1:5]); length != 0 {
		t.Fatalf("expected payload length 0, got %d", length)
	}

	// 2. Named service
	frameNamed := EncodeGRPCHealthCheckRequest("order.OrderService")
	if len(frameNamed) <= 5 {
		t.Fatalf("expected frame length > 5, got %d", len(frameNamed))
	}
	payloadLen := binary.BigEndian.Uint32(frameNamed[1:5])
	if int(payloadLen) != len(frameNamed)-5 {
		t.Fatalf("frame header length %d does not match body size %d", payloadLen, len(frameNamed)-5)
	}
	// Check tag 0x0a
	if frameNamed[5] != 0x0a {
		t.Fatalf("expected protobuf tag 0x0a, got 0x%x", frameNamed[5])
	}
}

func TestGRPCHealth_Decoding(t *testing.T) {
	// Build mock response frame for SERVING (1): tag=0x08, val=0x01
	payload := []byte{0x08, 0x01}
	frame := make([]byte, 5+len(payload))
	frame[0] = 0x00
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)

	status, err := DecodeGRPCHealthCheckResponse(frame)
	if err != nil {
		t.Fatalf("failed to decode valid response: %v", err)
	}
	if status != ServingStatusServing {
		t.Fatalf("expected ServingStatusServing (1), got %v", status)
	}

	// Build mock response frame for NOT_SERVING (2): tag=0x08, val=0x02
	payloadNot := []byte{0x08, 0x02}
	frameNot := make([]byte, 5+len(payloadNot))
	binary.BigEndian.PutUint32(frameNot[1:5], uint32(len(payloadNot)))
	copy(frameNot[5:], payloadNot)

	statusNot, err := DecodeGRPCHealthCheckResponse(frameNot)
	if err != nil {
		t.Fatalf("failed to decode valid response: %v", err)
	}
	if statusNot != ServingStatusNotServing {
		t.Fatalf("expected ServingStatusNotServing (2), got %v", statusNot)
	}
}

func TestGRPCHealth_ProbeServing(t *testing.T) {
	// Launch mock HTTP/2 gRPC health server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/grpc.health.v1.Health/Check") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "grpc-status, grpc-message")
		w.Header().Set("grpc-status", "0")

		// Response payload for SERVING (1)
		payload := []byte{0x08, 0x01}
		frame := make([]byte, 5+len(payload))
		binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
		copy(frame[5:], payload)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(frame)
	}))
	defer ts.Close()

	ctx := context.Background()
	ok, err := ProbeGRPCHealth(ctx, ts.URL, "order.OrderService", 3*time.Second)
	if err != nil {
		t.Fatalf("expected probe to succeed, got error: %v", err)
	}
	if !ok {
		t.Fatalf("expected probe to report healthy (true), got false")
	}
}

func TestGRPCHealth_ProbeNotServing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("grpc-status", "0")

		// Response payload for NOT_SERVING (2)
		payload := []byte{0x08, 0x02}
		frame := make([]byte, 5+len(payload))
		binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
		copy(frame[5:], payload)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(frame)
	}))
	defer ts.Close()

	ctx := context.Background()
	ok, err := ProbeGRPCHealth(ctx, ts.URL, "", 3*time.Second)
	if ok || err == nil {
		t.Fatalf("expected probe to fail for NOT_SERVING status")
	}
}

func TestGRPCHealth_ProbeError(t *testing.T) {
	ctx := context.Background()
	ok, err := ProbeGRPCHealth(ctx, "http://127.0.0.1:59999", "", 500*time.Millisecond)
	if ok || err == nil {
		t.Fatalf("expected probe to fail on closed socket")
	}
}

func TestProxy_GRPCTrailersPropagation(t *testing.T) {
	// Upstream returns gRPC trailer
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "grpc-status, grpc-message")
		w.Header().Set("grpc-status", "0")
		w.Header().Set("grpc-message", "OK")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x00})
	}))
	defer upstream.Close()

	proxy, err := NewLoadBalancerProxy([]string{upstream.URL}, AlgorithmRoundRobin, 3*time.Second)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req, _ := httpparser.NewRequest("POST", "/helloworld.Greeter/SayHello", "HTTP/1.1")
	req.Header.Set("Content-Type", "application/grpc")
	req.Body = bytes.NewBuffer([]byte{0x00, 0x00, 0x00, 0x00, 0x00})

	res := httpparser.NewResponse()
	proxy.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	if gs := res.Header.Get("grpc-status"); gs != "0" {
		t.Fatalf("expected grpc-status trailer '0', got %q", gs)
	}
	if gm := res.Header.Get("grpc-message"); gm != "OK" {
		t.Fatalf("expected grpc-message trailer 'OK', got %q", gm)
	}
}
