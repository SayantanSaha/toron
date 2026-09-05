package sidecar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

