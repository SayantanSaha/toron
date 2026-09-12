package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"toron/pkg/httpparser"
	"toron/pkg/router"
	"toron/pkg/server"
)

// setupInProcessToronServer boots an in-process Toron edge gateway for load testing.
func setupInProcessToronServer(t *testing.T) (*server.Server, string, func()) {
	t.Helper()

	r := router.New()
	r.GET("/health", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"ok","time":"` + time.Now().Format(time.RFC3339) + `"}`)
	})
	r.POST("/echo", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		res.Header.Set("Content-Type", "application/json")
		_, _ = res.WriteString(`{"status":"echo"}`)
	})

	cfg := server.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.WorkerPoolSize = 64
	cfg.HTTP2Enabled = true

	srv := server.New(cfg, r)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("failed to listen on %s: %v", cfg.Addr, err)
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	addr := ln.Addr().String()

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		_ = ln.Close()
	}

	return srv, addr, cleanup
}

func TestLoadGen_AdversarialInjection(t *testing.T) {
	_, addr, cleanup := setupInProcessToronServer(t)
	defer cleanup()

	targetURL := "http://" + addr + "/health"

	cfg := LoadGenConfig{
		TargetURL:   targetURL,
		Concurrency: 10,
		Duration:    2 * time.Second,
		TargetRate:  2000,
		AttackRatio: 0.15, // 15% adversarial injection
		Method:      "GET",
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	if report.TotalRequestsExecuted == 0 {
		t.Fatalf("expected requests to be executed, got 0")
	}

	if report.BenignStream.TotalRequests == 0 {
		t.Errorf("expected benign requests, got 0")
	}

	if report.AdversarialStream.TotalRequests == 0 {
		t.Errorf("expected adversarial probes, got 0")
	}

	// Assert zero bypasses on attack stream
	if report.AdversarialStream.FailedRequests > 0 {
		t.Errorf("expected 0 attack bypasses, got %d", report.AdversarialStream.FailedRequests)
	}

	// Assert 100% invariant enforcement rate
	if report.InvariantEnforcementRate < 100.0 {
		t.Errorf("expected 100%% invariant enforcement, got %.2f%%", report.InvariantEnforcementRate)
	}

	// Assert zero-starvation on benign stream
	if !report.ZeroStarvationVerified {
		t.Errorf("expected zero starvation verified (p99 <= 50ms), got p99 = %.2f ms",
			report.BenignStream.LatenciesMs.P99)
	}

	if report.OverallVerdict != "PASS" {
		t.Errorf("expected overall verdict PASS, got %s", report.OverallVerdict)
	}
}

func TestLoadGen_ReportGeneration(t *testing.T) {
	_, addr, cleanup := setupInProcessToronServer(t)
	defer cleanup()

	targetURL := "http://" + addr + "/health"
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "report.json")
	mdPath := filepath.Join(tmpDir, "report.md")

	cfg := LoadGenConfig{
		TargetURL:   targetURL,
		Concurrency: 5,
		Duration:    1 * time.Second,
		TargetRate:  1000,
		AttackRatio: 0.10,
		Method:      "GET",
		JSONPath:    jsonPath,
		MDPath:      mdPath,
	}

	report, err := RunLoadGen(cfg)
	if err != nil {
		t.Fatalf("RunLoadGen failed: %v", err)
	}

	// Write JSON
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		t.Fatalf("failed to write JSON: %v", err)
	}

	// Write Markdown
	if err := GenerateMarkdownReport(report, mdPath); err != nil {
		t.Fatalf("failed to generate Markdown: %v", err)
	}

	// Verify file sizes
	jsonInfo, err := os.Stat(jsonPath)
	if err != nil || jsonInfo.Size() == 0 {
		t.Errorf("expected non-empty JSON report, got err=%v", err)
	}

	mdInfo, err := os.Stat(mdPath)
	if err != nil || mdInfo.Size() == 0 {
		t.Errorf("expected non-empty MD report, got err=%v", err)
	}
}
