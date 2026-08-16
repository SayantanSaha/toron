package sidecar

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
