package sidecar

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"toron/pkg/config"
	"toron/pkg/router"
)

func TestWeightedTrafficSplitter(t *testing.T) {
	targets := []SplitTarget{
		{URL: "http://svc-v1:8080", Weight: 80},
		{URL: "http://svc-v2:8080", Weight: 20},
	}

	sp, err := NewWeightedSplitter(targets)
	if err != nil {
		t.Fatalf("NewWeightedSplitter failed: %v", err)
	}

	v1Count := 0
	v2Count := 0
	totalRequests := 1000

	for i := 0; i < totalRequests; i++ {
		selected := sp.Select()
		if selected == "http://svc-v1:8080" {
			v1Count++
		} else if selected == "http://svc-v2:8080" {
			v2Count++
		}
	}

	// Verify exact ~800 / ~200 distribution
	if v1Count != 800 {
		t.Errorf("v1Count = %d, want 800", v1Count)
	}
	if v2Count != 200 {
		t.Errorf("v2Count = %d, want 200", v2Count)
	}
}

func TestBuildTLSConfig(t *testing.T) {
	cfg := config.SidecarConfig{
		Enabled:    true,
		StrictmTLS: true,
	}

	serverTLS, err := BuildServerTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig error: %v", err)
	}

	if serverTLS.ClientAuth != 4 { // RequireAndVerifyClientCert = 4
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", serverTLS.ClientAuth)
	}

	clientTLS, err := BuildClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig error: %v", err)
	}

	if clientTLS == nil {
		t.Fatalf("clientTLS should not be nil")
	}
	if clientTLS.InsecureSkipVerify {
		t.Errorf("expected InsecureSkipVerify to be false by default, got true")
	}
}

func TestSidecarProxyEngine(t *testing.T) {
	// Mock local app container
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello from App Container"))
	}))
	defer appServer.Close()

	// Extract port from appServer URL
	var appPort int
	_, err := fmt.Sscanf(appServer.URL, "http://127.0.0.1:%d", &appPort)
	if err != nil {
		// Fallback parse port
		appPort = 8080
	}

	cfg := config.SidecarConfig{
		Enabled:     true,
		Mode:        "ingress",
		IngressPort: 15999,
		AppPort:     appPort,
	}

	r := router.New()
	engine, err := NewProxyEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start() failed: %v", err)
	}
	defer engine.Stop()

	// Send HTTP request to sidecar ingress listener (:15999)
	resp, err := http.Get("http://127.0.0.1:15999/test")
	if err != nil {
		t.Fatalf("Failed to send request to sidecar ingress: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestSidecarProxyEngine_BodyForwarding(t *testing.T) {
	var receivedBody string
	var receivedMethod string

	// Mock upstream server capturing received body
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer appServer.Close()

	var appPort int
	_, _ = fmt.Sscanf(appServer.URL, "http://127.0.0.1:%d", &appPort)
	if appPort == 0 {
		appPort = 8080
	}

	cfg := config.SidecarConfig{
		Enabled:     true,
		Mode:        "ingress",
		IngressPort: 15998,
		AppPort:     appPort,
	}

	r := router.New()
	engine, err := NewProxyEngine(cfg, r)
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start() failed: %v", err)
	}
	defer engine.Stop()

	// TC-081-01: JSON POST Body Forwarding
	postPayload := `{"account":"123","amount":500}`
	resp, err := http.Post("http://127.0.0.1:15998/api/data", "application/json", strings.NewReader(postPayload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected method POST, got %q", receivedMethod)
	}
	if receivedBody != postPayload {
		t.Errorf("TC-081-01: expected body %q, got %q", postPayload, receivedBody)
	}

	// TC-081-02: Empty GET Request Body Handling
	respGet, err := http.Get("http://127.0.0.1:15998/api/data")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	_ = respGet.Body.Close()

	if respGet.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", respGet.StatusCode)
	}
	if receivedMethod != "GET" {
		t.Errorf("expected method GET, got %q", receivedMethod)
	}
	if receivedBody != "" {
		t.Errorf("TC-081-02: expected empty body on GET, got %q", receivedBody)
	}
}

func parseTestServerPort(serverURL string) int {
	u, err := url.Parse(serverURL)
	if err != nil {
		return 8080
	}
	_, p, err := net.SplitHostPort(u.Host)
	if err != nil {
		return 8080
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return 8080
	}
	return port
}

// TC-086-01: Declared Content-Length Fast-Fail Rejection
func TestTC086_01_DeclaredContentLengthFastFailRejection(t *testing.T) {
	var upstreamRequests atomic.Int64
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer appServer.Close()

	appPort := parseTestServerPort(appServer.URL)
	cfg := config.SidecarConfig{
		Enabled:      true,
		Mode:         "ingress",
		IngressPort:  16001,
		AppPort:      appPort,
		MaxBodyBytes: 1024, // 1 KB
	}

	engine, err := NewProxyEngine(cfg, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start failed: %v", err)
	}
	defer engine.Stop()

	// Client issues POST with declared Content-Length: 2048
	payload := bytes.Repeat([]byte("A"), 2048)
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16001/api/data", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body failed: %v", err)
	}
	bodyStr := string(bodyBytes)

	// Assertion 1: Status code MUST equal 413
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 StatusRequestEntityTooLarge, got %d", resp.StatusCode)
	}

	// Assertion 2: Response body contains 413 error message
	if !strings.Contains(bodyStr, "Payload Too Large") && !strings.Contains(bodyStr, "Request Entity Too Large") {
		t.Errorf("expected 413 rejection message, got %q", bodyStr)
	}

	// Assertion 3: Upstream request counter MUST equal 0
	if reqCount := upstreamRequests.Load(); reqCount != 0 {
		t.Errorf("expected upstream to receive 0 requests, got %d", reqCount)
	}
}

// TC-086-02: Bounded Stream Over-Read Rejection
func TestTC086_02_BoundedStreamOverReadRejection(t *testing.T) {
	var upstreamRequests atomic.Int64
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer appServer.Close()

	appPort := parseTestServerPort(appServer.URL)
	cfg := config.SidecarConfig{
		Enabled:      true,
		Mode:         "ingress",
		IngressPort:  16002,
		AppPort:      appPort,
		MaxBodyBytes: 512,
	}

	engine, err := NewProxyEngine(cfg, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start failed: %v", err)
	}
	defer engine.Stop()

	// Chunked transfer POST with 1024 bytes (ContentLength = -1)
	streamPayload := bytes.Repeat([]byte("B"), 1024)
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16002/api/stream", bytes.NewReader(streamPayload))
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	req.ContentLength = -1 // force chunked transfer encoding

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("chunked POST request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body failed: %v", err)
	}
	bodyStr := string(bodyBytes)

	// Assertion 1: Status code MUST equal 413
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 StatusRequestEntityTooLarge, got %d", resp.StatusCode)
	}

	// Assertion 2: Response body contains 413 error message
	if !strings.Contains(bodyStr, "Payload Too Large") && !strings.Contains(bodyStr, "Request Entity Too Large") {
		t.Errorf("expected 413 rejection message, got %q", bodyStr)
	}

	// Assertion 3: Upstream mock server receives 0 requests
	if reqCount := upstreamRequests.Load(); reqCount != 0 {
		t.Errorf("expected upstream to receive 0 requests, got %d", reqCount)
	}
}

// TC-086-03: Default 10MB Fallback Enforcement
func TestTC086_03_Default10MBFallbackEnforcement(t *testing.T) {
	const tenMB = int64(10 * 1024 * 1024) // 10,485,760 bytes

	// Subtest 3A: Config Struct Defaults & Fallback Verification
	engineZero, err := NewProxyEngine(config.SidecarConfig{MaxBodyBytes: 0}, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}
	if engineZero.cfg.MaxBodyBytes != tenMB {
		t.Errorf("expected default 10MB (%d), got %d", tenMB, engineZero.cfg.MaxBodyBytes)
	}

	engineNeg, err := NewProxyEngine(config.SidecarConfig{MaxBodyBytes: -100}, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}
	if engineNeg.cfg.MaxBodyBytes != tenMB {
		t.Errorf("expected default 10MB (%d), got %d", tenMB, engineNeg.cfg.MaxBodyBytes)
	}

	defaultApp := config.DefaultAppConfig()
	if defaultApp.Sidecar.MaxBodyBytes != tenMB {
		t.Errorf("expected DefaultAppConfig sidecar MaxBodyBytes %d, got %d", tenMB, defaultApp.Sidecar.MaxBodyBytes)
	}

	// Setup upstream server
	var upstreamBytesReceived atomic.Int64
	var upstreamRequests atomic.Int64
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests.Add(1)
		b, _ := io.ReadAll(r.Body)
		upstreamBytesReceived.Store(int64(len(b)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer appServer.Close()

	appPort := parseTestServerPort(appServer.URL)
	cfg := config.SidecarConfig{
		Enabled:      true,
		Mode:         "ingress",
		IngressPort:  16003,
		AppPort:      appPort,
		MaxBodyBytes: 0, // Unset, must default to 10MB
	}

	engine, err := NewProxyEngine(cfg, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start failed: %v", err)
	}
	defer engine.Stop()

	client := &http.Client{Timeout: 30 * time.Second}

	// Subtest 3B: Exact 10MB Boundary Accepted
	payload10MB := make([]byte, tenMB)
	req3B, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16003/api/upload", bytes.NewReader(payload10MB))
	if err != nil {
		t.Fatalf("req3B create failed: %v", err)
	}
	req3B.Header.Set("Content-Type", "application/octet-stream")

	resp3B, err := client.Do(req3B)
	if err != nil {
		t.Fatalf("req3B failed: %v", err)
	}
	defer resp3B.Body.Close()

	if resp3B.StatusCode != http.StatusOK {
		t.Errorf("Subtest 3B: expected 200 OK for exact 10MB, got %d", resp3B.StatusCode)
	}
	if received := upstreamBytesReceived.Load(); received != tenMB {
		t.Errorf("Subtest 3B: expected %d bytes upstream, got %d", tenMB, received)
	}

	// Subtest 3C: 10MB + 1 Byte Boundary Rejected
	reqsBefore := upstreamRequests.Load()
	payload10MBPlus1 := make([]byte, tenMB+1)
	req3C, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16003/api/upload", bytes.NewReader(payload10MBPlus1))
	if err != nil {
		t.Fatalf("req3C create failed: %v", err)
	}
	req3C.Header.Set("Content-Type", "application/octet-stream")

	resp3C, err := client.Do(req3C)
	if err != nil {
		t.Fatalf("req3C failed: %v", err)
	}
	defer resp3C.Body.Close()

	if resp3C.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("Subtest 3C: expected 413 for 10MB+1, got %d", resp3C.StatusCode)
	}
	if reqsAfter := upstreamRequests.Load(); reqsAfter != reqsBefore {
		t.Errorf("Subtest 3C: expected 0 upstream requests forwarded, got %d", reqsAfter-reqsBefore)
	}
}

// TC-086-04: Full-Fidelity In-Limit Body Forwarding
func TestTC086_04_FullFidelityInLimitBodyForwarding(t *testing.T) {
	var mu sync.Mutex
	var lastMethod string
	var lastBody []byte
	var lastContentLength int64

	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastMethod = r.Method
		lastContentLength = r.ContentLength
		b, _ := io.ReadAll(r.Body)
		lastBody = b
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("saved"))
	}))
	defer appServer.Close()

	appPort := parseTestServerPort(appServer.URL)
	cfg := config.SidecarConfig{
		Enabled:      true,
		Mode:         "ingress",
		IngressPort:  16004,
		AppPort:      appPort,
		MaxBodyBytes: 8192, // 8 KB
	}

	engine, err := NewProxyEngine(cfg, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start failed: %v", err)
	}
	defer engine.Stop()

	client := &http.Client{Timeout: 5 * time.Second}

	// Subtest 4A: POST with 1024-byte structured JSON
	jsonPayload := bytes.Repeat([]byte(`{"k":"v"}`), 128) // 1024 bytes
	reqA, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:16004/api/records", bytes.NewReader(jsonPayload))
	reqA.Header.Set("Content-Type", "application/json")
	respA, err := client.Do(reqA)
	if err != nil {
		t.Fatalf("POST 4A failed: %v", err)
	}
	_ = respA.Body.Close()

	if respA.StatusCode != http.StatusOK {
		t.Errorf("4A expected 200 OK, got %d", respA.StatusCode)
	}
	mu.Lock()
	if lastMethod != "POST" {
		t.Errorf("4A expected method POST, got %s", lastMethod)
	}
	if !bytes.Equal(lastBody, jsonPayload) {
		t.Errorf("4A payload corrupted or truncated")
	}
	if lastContentLength != int64(len(jsonPayload)) {
		t.Errorf("4A expected Content-Length %d, got %d", len(jsonPayload), lastContentLength)
	}
	mu.Unlock()

	// Subtest 4B: PUT with 2048-byte binary pattern
	binPayload := make([]byte, 2048)
	for i := range binPayload {
		binPayload[i] = byte(i % 256)
	}
	reqB, _ := http.NewRequest(http.MethodPut, "http://127.0.0.1:16004/api/records/101", bytes.NewReader(binPayload))
	reqB.Header.Set("Content-Type", "application/octet-stream")
	respB, err := client.Do(reqB)
	if err != nil {
		t.Fatalf("PUT 4B failed: %v", err)
	}
	_ = respB.Body.Close()

	if respB.StatusCode != http.StatusOK {
		t.Errorf("4B expected 200 OK, got %d", respB.StatusCode)
	}
	mu.Lock()
	if lastMethod != "PUT" {
		t.Errorf("4B expected method PUT, got %s", lastMethod)
	}
	if !bytes.Equal(lastBody, binPayload) {
		t.Errorf("4B binary payload corrupted or truncated")
	}
	if lastContentLength != int64(len(binPayload)) {
		t.Errorf("4B expected Content-Length %d, got %d", len(binPayload), lastContentLength)
	}
	mu.Unlock()

	// Subtest 4C: PATCH with 512-byte text payload
	textPayload := bytes.Repeat([]byte("X"), 512)
	reqC, _ := http.NewRequest(http.MethodPatch, "http://127.0.0.1:16004/api/records/101", bytes.NewReader(textPayload))
	reqC.Header.Set("Content-Type", "text/plain")
	respC, err := client.Do(reqC)
	if err != nil {
		t.Fatalf("PATCH 4C failed: %v", err)
	}
	_ = respC.Body.Close()

	if respC.StatusCode != http.StatusOK {
		t.Errorf("4C expected 200 OK, got %d", respC.StatusCode)
	}
	mu.Lock()
	if lastMethod != "PATCH" {
		t.Errorf("4C expected method PATCH, got %s", lastMethod)
	}
	if !bytes.Equal(lastBody, textPayload) {
		t.Errorf("4C text payload corrupted or truncated")
	}
	if lastContentLength != int64(len(textPayload)) {
		t.Errorf("4C expected Content-Length %d, got %d", len(textPayload), lastContentLength)
	}
	mu.Unlock()
}

// TC-086-05: Non-Mutating and Empty Request Bypass
func TestTC086_05_NonMutatingAndEmptyRequestBypass(t *testing.T) {
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case http.MethodHead:
			w.Header().Set("X-Custom-Header", "toron-head-check")
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			if len(b) == 0 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"empty_ok"}`))
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer appServer.Close()

	appPort := parseTestServerPort(appServer.URL)
	// Very small limit: 32 bytes
	cfg := config.SidecarConfig{
		Enabled:      true,
		Mode:         "ingress",
		IngressPort:  16005,
		AppPort:      appPort,
		MaxBodyBytes: 32,
	}

	engine, err := NewProxyEngine(cfg, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start failed: %v", err)
	}
	defer engine.Stop()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET with nil body
	respGet, err := client.Get("http://127.0.0.1:16005/api/status")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer respGet.Body.Close()
	bodyGet, _ := io.ReadAll(respGet.Body)
	if respGet.StatusCode != http.StatusOK {
		t.Errorf("GET: expected 200 OK, got %d", respGet.StatusCode)
	}
	if string(bodyGet) != `{"status":"ok"}` {
		t.Errorf("GET: expected body %q, got %q", `{"status":"ok"}`, string(bodyGet))
	}

	// 2. HEAD with nil body
	respHead, err := client.Head("http://127.0.0.1:16005/api/status")
	if err != nil {
		t.Fatalf("HEAD request failed: %v", err)
	}
	defer respHead.Body.Close()
	bodyHead, _ := io.ReadAll(respHead.Body)
	if respHead.StatusCode != http.StatusOK {
		t.Errorf("HEAD: expected 200 OK, got %d", respHead.StatusCode)
	}
	if len(bodyHead) != 0 {
		t.Errorf("HEAD: expected empty body, got %q", string(bodyHead))
	}
	if respHead.Header.Get("X-Custom-Header") != "toron-head-check" {
		t.Errorf("HEAD: expected header X-Custom-Header to be preserved, got %q", respHead.Header.Get("X-Custom-Header"))
	}

	// 3. POST with Content-Length: 0 and http.NoBody
	reqPost, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16005/api/empty", http.NoBody)
	if err != nil {
		t.Fatalf("POST empty request failed: %v", err)
	}
	reqPost.ContentLength = 0
	respPost, err := client.Do(reqPost)
	if err != nil {
		t.Fatalf("POST empty request failed: %v", err)
	}
	defer respPost.Body.Close()
	if respPost.StatusCode != http.StatusOK {
		t.Errorf("POST with Content-Length 0: expected 200 OK, got %d", respPost.StatusCode)
	}
}

// TC-086-06: High-Concurrency & Race-Condition Safety
func TestTC086_06_HighConcurrencyRaceSafety(t *testing.T) {
	var validPostCount atomic.Int64
	var getCount atomic.Int64
	var oversizedLeakCount atomic.Int64

	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			if len(b) > 1024 {
				oversizedLeakCount.Add(1)
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			} else {
				validPostCount.Add(1)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}
		case http.MethodGet:
			getCount.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("get-ok"))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer appServer.Close()

	appPort := parseTestServerPort(appServer.URL)
	cfg := config.SidecarConfig{
		Enabled:      true,
		Mode:         "ingress",
		IngressPort:  16006,
		AppPort:      appPort,
		MaxBodyBytes: 1024,
	}

	engine, err := NewProxyEngine(cfg, router.New())
	if err != nil {
		t.Fatalf("NewProxyEngine failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine.Start failed: %v", err)
	}
	defer engine.Stop()

	const numPerCategory = 25
	var wg sync.WaitGroup
	var validPostSuccess atomic.Int64
	var declaredOversized413 atomic.Int64
	var chunkedOversized413 atomic.Int64
	var getSuccess atomic.Int64

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        150,
			MaxIdleConnsPerHost: 150,
		},
	}

	// Category 1: Valid in-bounds POST (256 bytes)
	for i := 0; i < numPerCategory; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := bytes.Repeat([]byte("V"), 256)
			req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16006/api/valid", bytes.NewReader(payload))
			if err != nil {
				t.Errorf("NewRequest Category 1 failed: %v", err)
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Client.Do Category 1 failed: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				validPostSuccess.Add(1)
			}
		}()
	}

	// Category 2: Declared oversized POST (2048 bytes with Content-Length: 2048)
	for i := 0; i < numPerCategory; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := bytes.Repeat([]byte("O"), 2048)
			req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16006/api/oversized", bytes.NewReader(payload))
			if err != nil {
				t.Errorf("NewRequest Category 2 failed: %v", err)
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Client.Do Category 2 failed: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusRequestEntityTooLarge {
				declaredOversized413.Add(1)
			}
		}()
	}

	// Category 3: Chunked oversized POST (1500 bytes with ContentLength = -1)
	for i := 0; i < numPerCategory; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := bytes.Repeat([]byte("C"), 1500)
			req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:16006/api/chunked", bytes.NewReader(payload))
			if err != nil {
				t.Errorf("NewRequest Category 3 failed: %v", err)
				return
			}
			req.ContentLength = -1
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Client.Do Category 3 failed: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusRequestEntityTooLarge {
				chunkedOversized413.Add(1)
			}
		}()
	}

	// Category 4: Non-mutating GET requests
	for i := 0; i < numPerCategory; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get("http://127.0.0.1:16006/api/status")
			if err != nil {
				t.Errorf("Client.Get Category 4 failed: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				getSuccess.Add(1)
			}
		}()
	}

	wg.Wait()

	// Assertions
	if validPostSuccess.Load() != numPerCategory {
		t.Errorf("expected %d valid POST successes, got %d", numPerCategory, validPostSuccess.Load())
	}
	if declaredOversized413.Load() != numPerCategory {
		t.Errorf("expected %d declared oversized 413s, got %d", numPerCategory, declaredOversized413.Load())
	}
	if chunkedOversized413.Load() != numPerCategory {
		t.Errorf("expected %d chunked oversized 413s, got %d", numPerCategory, chunkedOversized413.Load())
	}
	if getSuccess.Load() != numPerCategory {
		t.Errorf("expected %d GET successes, got %d", numPerCategory, getSuccess.Load())
	}
	if leaks := oversizedLeakCount.Load(); leaks != 0 {
		t.Errorf("expected 0 oversized leaks to upstream backend, got %d", leaks)
	}
	if upstreamValid := validPostCount.Load(); upstreamValid != numPerCategory {
		t.Errorf("expected %d valid POSTs at upstream, got %d", numPerCategory, upstreamValid)
	}
	if upstreamGet := getCount.Load(); upstreamGet != numPerCategory {
		t.Errorf("expected %d GETs at upstream, got %d", numPerCategory, upstreamGet)
	}
}

// --- TC-097: Verification of Strict Sidecar Client TLS Validation ---

func generateTestCA(t *testing.T) ([]byte, *x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		t.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Toron Test CA"},
			CommonName:   "Toron Root CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}

	caCert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return caPEM, caCert, priv
}

func generateSignedServerCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, hosts ...string) ([]byte, []byte, tls.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate server private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		t.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Toron Test Server"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create server certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes := x509.MarshalPKCS1PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to create tls.Certificate: %v", err)
	}

	return certPEM, keyPEM, tlsCert
}

func generateClientCert(t *testing.T) ([]byte, []byte) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate client private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		t.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Toron Test Client"},
			CommonName:   "client",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create client certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes := x509.MarshalPKCS1PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})

	return certPEM, keyPEM
}

// TC-097-01: Default Configuration Enforces Strict Validation (InsecureSkipVerify == false, RootCAs == nil)
func TestBuildClientTLSConfig_DefaultSecure(t *testing.T) {
	testCases := []struct {
		name string
		cfg  config.SidecarConfig
	}{
		{
			name: "1.1 Zero-Value Struct",
			cfg:  config.SidecarConfig{},
		},
		{
			name: "1.2 Default Config",
			cfg:  config.DefaultAppConfig().Sidecar,
		},
		{
			name: "1.3 Egress Mode Omitted CA",
			cfg: config.SidecarConfig{
				Enabled: true,
				Mode:    "egress",
				CAFile:  "",
			},
		},
		{
			name: "1.4 Explicit False",
			cfg: config.SidecarConfig{
				InsecureSkipVerify: false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tlsConfig, err := BuildClientTLSConfig(tc.cfg)
			if err != nil {
				t.Fatalf("unexpected error from BuildClientTLSConfig: %v", err)
			}
			if tlsConfig == nil {
				t.Fatalf("expected non-nil *tls.Config")
			}
			if tlsConfig.InsecureSkipVerify != false {
				t.Errorf("expected InsecureSkipVerify == false, got %v", tlsConfig.InsecureSkipVerify)
			}
			if tlsConfig.RootCAs != nil {
				t.Errorf("expected RootCAs == nil for system trust root fallback, got %v", tlsConfig.RootCAs)
			}
			if tlsConfig.MinVersion != tls.VersionTLS12 {
				t.Errorf("expected MinVersion == tls.VersionTLS12 (0x%04x), got 0x%04x", tls.VersionTLS12, tlsConfig.MinVersion)
			}
		})
	}
}

// TC-097-02: System Trust Store Fallback Handshake Rejection on Untrusted Server
func TestSidecar_EgressTLS_HandshakeRejection(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("should-not-reach"))
	}))
	defer upstream.Close()

	tlsConfig, err := BuildClientTLSConfig(config.SidecarConfig{})
	if err != nil {
		t.Fatalf("BuildClientTLSConfig failed: %v", err)
	}

	if tlsConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify == false")
	}
	if tlsConfig.RootCAs != nil {
		t.Fatalf("expected RootCAs == nil")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(upstream.URL)
	if err == nil {
		if resp != nil {
			defer resp.Body.Close()
		}
		t.Fatalf("expected TLS handshake failure for untrusted certificate, but request succeeded with status %d", resp.StatusCode)
	}

	if resp != nil {
		t.Errorf("expected resp == nil on handshake failure, got %v", resp)
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "unknown authority") &&
		!strings.Contains(errStr, "certificate signed by unknown authority") &&
		!strings.Contains(errStr, "tls: failed to verify certificate") &&
		!strings.Contains(errStr, "x509: certificate") {
		t.Errorf("expected unknown authority or certificate verification error, got: %v", err)
	}
}

// TC-097-03: Custom CA Certificate Pool Validation and Validated Handshake Success
func TestBuildClientTLSConfig_CustomCA(t *testing.T) {
	tmpDir := t.TempDir()
	caPEM, _, _ := generateTestCA(t)
	caPath := filepath.Join(tmpDir, "ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0644); err != nil {
		t.Fatalf("failed to write CA PEM: %v", err)
	}

	cfg := config.SidecarConfig{
		CAFile:             caPath,
		InsecureSkipVerify: false,
	}

	tlsConfig, err := BuildClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig failed: %v", err)
	}

	if tlsConfig.RootCAs == nil {
		t.Fatalf("expected RootCAs != nil when CAFile is provided")
	}
	if tlsConfig.InsecureSkipVerify {
		t.Errorf("expected InsecureSkipVerify == false with custom CA, got true")
	}

	// Invalid CA File Path
	cfg.CAFile = filepath.Join(tmpDir, "nonexistent-ca.pem")
	_, err = BuildClientTLSConfig(cfg)
	if err == nil {
		t.Fatalf("expected error for nonexistent CAFile, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read sidecar ca cert") {
		t.Errorf("expected 'failed to read sidecar ca cert' in error, got: %v", err)
	}
}

func TestSidecar_EgressTLS_HandshakeSuccess_WithCustomCA(t *testing.T) {
	tmpDir := t.TempDir()
	caPEM, caCert, caKey := generateTestCA(t)
	caPath := filepath.Join(tmpDir, "ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0644); err != nil {
		t.Fatalf("failed to write CA PEM: %v", err)
	}

	_, _, serverTLSCert := generateSignedServerCert(t, caCert, caKey, "127.0.0.1", "localhost")

	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("verified-via-custom-ca"))
	}))
	upstream.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverTLSCert},
	}
	upstream.StartTLS()
	defer upstream.Close()

	cfg := config.SidecarConfig{
		CAFile:             caPath,
		InsecureSkipVerify: false,
	}

	tlsConfig, err := BuildClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig failed: %v", err)
	}

	if tlsConfig.RootCAs == nil {
		t.Fatalf("expected RootCAs != nil")
	}
	if tlsConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify == false")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("expected successful TLS handshake with custom CA, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected HTTP 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != "verified-via-custom-ca" {
		t.Errorf("unexpected response body %q", string(body))
	}
}

// TC-097-04: Explicit Opt-In InsecureSkipVerify: true and Mandatory Security Warning
func TestBuildClientTLSConfig_ExplicitInsecureOptIn(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	cfg := config.SidecarConfig{
		InsecureSkipVerify: true,
	}

	tlsConfig, err := BuildClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig failed: %v", err)
	}

	if !tlsConfig.InsecureSkipVerify {
		t.Errorf("expected InsecureSkipVerify == true, got false")
	}

	expectedWarning := "[SIDECAR] WARNING: InsecureSkipVerify is enabled for sidecar egress TLS. Certificate verification is disabled."
	if !strings.Contains(logBuf.String(), expectedWarning) {
		t.Errorf("expected log warning %q, got %q", expectedWarning, logBuf.String())
	}
}

func TestSidecar_EgressTLS_HandshakeSuccess_WithExplicitOptIn(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("insecure-opt-in-success"))
	}))
	defer upstream.Close()

	cfg := config.SidecarConfig{
		InsecureSkipVerify: true,
	}

	tlsConfig, err := BuildClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig failed: %v", err)
	}

	if !tlsConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify == true")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("expected successful TLS connection with explicit InsecureSkipVerify: true, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected HTTP 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != "insecure-opt-in-success" {
		t.Errorf("unexpected response body %q", string(body))
	}
}

// TC-097-05: Client Identity Certificate Keypair Loading for Egress mTLS
func TestBuildClientTLSConfig_ClientCertKeypair(t *testing.T) {
	tmpDir := t.TempDir()
	certPEM, keyPEM := generateClientCert(t)
	certPath := filepath.Join(tmpDir, "client.crt")
	keyPath := filepath.Join(tmpDir, "client.key")

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("failed to write client cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write client key: %v", err)
	}

	cfg := config.SidecarConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
	}

	tlsConfig, err := BuildClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig failed: %v", err)
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Fatalf("expected 1 client certificate loaded, got %d", len(tlsConfig.Certificates))
	}

	// Invalid KeyFile Path
	cfg.KeyFile = filepath.Join(tmpDir, "nonexistent.key")
	_, err = BuildClientTLSConfig(cfg)
	if err == nil {
		t.Fatalf("expected error for nonexistent KeyFile, got nil")
	}
	if !strings.Contains(err.Error(), "failed to load sidecar client tls cert/key pair") {
		t.Errorf("expected 'failed to load sidecar client tls cert/key pair' in error, got: %v", err)
	}
}

// TC-097-06: Minimum TLS Protocol Version Enforcement (tls.VersionTLS12)
func TestBuildClientTLSConfig_MinVersionTLS12(t *testing.T) {
	tmpDir := t.TempDir()
	caPEM, _, _ := generateTestCA(t)
	caPath := filepath.Join(tmpDir, "ca.pem")
	_ = os.WriteFile(caPath, caPEM, 0644)

	certPEM, keyPEM := generateClientCert(t)
	certPath := filepath.Join(tmpDir, "client.crt")
	keyPath := filepath.Join(tmpDir, "client.key")
	_ = os.WriteFile(certPath, certPEM, 0644)
	_ = os.WriteFile(keyPath, keyPEM, 0600)

	testConfigs := []struct {
		name string
		cfg  config.SidecarConfig
	}{
		{
			name: "Default Config",
			cfg:  config.SidecarConfig{},
		},
		{
			name: "Custom CA Config",
			cfg:  config.SidecarConfig{CAFile: caPath},
		},
		{
			name: "Insecure Opt-In Config",
			cfg:  config.SidecarConfig{InsecureSkipVerify: true},
		},
		{
			name: "Client mTLS Config",
			cfg:  config.SidecarConfig{CertFile: certPath, KeyFile: keyPath},
		},
	}

	for _, tc := range testConfigs {
		t.Run(tc.name, func(t *testing.T) {
			tlsConfig, err := BuildClientTLSConfig(tc.cfg)
			if err != nil {
				t.Fatalf("BuildClientTLSConfig failed: %v", err)
			}
			if tlsConfig.MinVersion != tls.VersionTLS12 {
				t.Errorf("expected MinVersion == tls.VersionTLS12 (0x0303), got 0x%04x", tlsConfig.MinVersion)
			}
			if tlsConfig.MinVersion == tls.VersionTLS10 {
				t.Errorf("MinVersion must not be TLS 1.0")
			}
			if tlsConfig.MinVersion == tls.VersionTLS11 {
				t.Errorf("MinVersion must not be TLS 1.1")
			}
		})
	}
}

// TC-097-07: High-Concurrency Thread Safety and Race Detection
func TestSidecar_EgressTLS_ConcurrentRouting_RaceClean(t *testing.T) {
	tmpDir := t.TempDir()
	caPEM, caCert, caKey := generateTestCA(t)
	caPath := filepath.Join(tmpDir, "ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0644); err != nil {
		t.Fatalf("failed to write CA PEM: %v", err)
	}

	_, _, serverTLSCert := generateSignedServerCert(t, caCert, caKey, "127.0.0.1", "localhost")

	var requestCount atomic.Int64
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("concurrent-ok"))
	}))
	upstream.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverTLSCert},
	}
	upstream.StartTLS()
	defer upstream.Close()

	cfg := config.SidecarConfig{
		CAFile:             caPath,
		InsecureSkipVerify: false,
	}

	tlsConfig, err := BuildClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 10 * time.Second,
	}

	const numGoroutines = 20
	const requestsPerGoroutine = 50
	var wg sync.WaitGroup

	// Phase 1: Parallel HTTPS requests through shared TLSClientConfig
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				resp, err := client.Get(upstream.URL)
				if err != nil {
					t.Errorf("concurrent request failed: %v", err)
					return
				}
				if resp.StatusCode != http.StatusOK {
					t.Errorf("unexpected status code: %d", resp.StatusCode)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}()
	}

	// Phase 2: Simultaneous concurrent invocation of BuildClientTLSConfig
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				var testCfg config.SidecarConfig
				switch (idx + j) % 3 {
				case 0:
					testCfg = config.SidecarConfig{}
				case 1:
					testCfg = config.SidecarConfig{CAFile: caPath}
				case 2:
					testCfg = config.SidecarConfig{InsecureSkipVerify: true}
				}
				c, cErr := BuildClientTLSConfig(testCfg)
				if cErr != nil {
					t.Errorf("concurrent BuildClientTLSConfig failed: %v", cErr)
				}
				if c == nil {
					t.Errorf("concurrent BuildClientTLSConfig returned nil")
				}
			}
		}(i)
	}

	wg.Wait()

	expectedTotal := int64(numGoroutines * requestsPerGoroutine)
	if requestCount.Load() != expectedTotal {
		t.Errorf("expected %d total upstream requests, got %d", expectedTotal, requestCount.Load())
	}
}

