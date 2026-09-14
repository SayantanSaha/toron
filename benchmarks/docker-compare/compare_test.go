package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilterProxies(t *testing.T) {
	t.Run("all proxies default", func(t *testing.T) {
		res := filterProxies("all")
		if len(res) != len(defaultProxies) {
			t.Fatalf("expected %d proxies, got %d", len(defaultProxies), len(res))
		}
	})

	t.Run("filtered subset", func(t *testing.T) {
		res := filterProxies("toron,nginx")
		if len(res) != 2 {
			t.Fatalf("expected 2 proxies, got %d", len(res))
		}
		if res[0].ID != "toron" || res[1].ID != "nginx" {
			t.Fatalf("unexpected proxies: %+v", res)
		}
	})

	t.Run("empty string returns all", func(t *testing.T) {
		res := filterProxies("")
		if len(res) != len(defaultProxies) {
			t.Fatalf("expected all proxies, got %d", len(res))
		}
	})
}

func TestFilterBackends(t *testing.T) {
	t.Run("all backends default", func(t *testing.T) {
		res := filterBackends("all")
		if len(res) != len(defaultBackends) {
			t.Fatalf("expected %d backends, got %d", len(defaultBackends), len(res))
		}
	})

	t.Run("filtered subset", func(t *testing.T) {
		res := filterBackends("fast,go")
		if len(res) != 2 {
			t.Fatalf("expected 2 backends, got %d", len(res))
		}
		if res[0].ID != "fast" || res[1].ID != "go" {
			t.Fatalf("unexpected backends: %+v", res)
		}
	})
}

func TestComputeLatencyStats(t *testing.T) {
	t.Run("empty latencies", func(t *testing.T) {
		stats := computeLatencyStats(nil)
		if stats.Mean != 0 || stats.P50 != 0 {
			t.Fatalf("expected 0 for empty, got %+v", stats)
		}
	})

	t.Run("sorted percentiles", func(t *testing.T) {
		samples := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}
		stats := computeLatencyStats(samples)

		if stats.Min != 1.0 {
			t.Errorf("expected min 1.0, got %f", stats.Min)
		}
		if stats.Max != 10.0 {
			t.Errorf("expected max 10.0, got %f", stats.Max)
		}
		if stats.Mean != 5.5 {
			t.Errorf("expected mean 5.5, got %f", stats.Mean)
		}
		if stats.P50 < 5.0 || stats.P50 > 6.0 {
			t.Errorf("expected P50 ~ 5.0-6.0, got %f", stats.P50)
		}
	})
}

func TestReportGeneration(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "report.json")
	mdPath := filepath.Join(tempDir, "report.md")

	report := DockerCompareReport{
		Timestamp:       "2026-09-14T10:00:00Z",
		HostOS:          "darwin",
		HostArch:        "arm64",
		NumCPU:          8,
		Concurrency:     50,
		DurationSeconds: 5.0,
		TargetRateRPS:   0,
		PreflightChecks: []PreflightResult{
			{
				ProxyID:    "toron",
				ProxyName:  "Toron (v1.0.0)",
				BackendID:  "fast",
				URL:        "http://127.0.0.1:8881/fast/health",
				StatusCode: 200,
				Passed:     true,
				LatencyMs:  1.2,
				Details:    "OK",
			},
		},
		Results: []BenchmarkCellResult{
			{
				ProxyID:       "toron",
				ProxyName:     "Toron (v1.0.0)",
				BackendID:     "fast",
				BackendName:   "Go Fast Echo",
				TargetURL:     "http://127.0.0.1:8881/fast/health",
				Concurrency:   50,
				DurationSec:   5.0,
				TotalRequests: 25000,
				SuccessCount:  25000,
				ErrorCount:    0,
				ActualRPS:     5000.0,
				ThroughputMBs: 4.5,
				BytesRead:     22500000,
				Latencies: LatencyStats{
					Min:  0.2,
					Mean: 0.8,
					P50:  0.7,
					P90:  1.2,
					P95:  1.5,
					P99:  2.1,
					P999: 3.5,
					Max:  5.0,
				},
				Telemetry: ResourceTelemetry{
					CPUPercent: 45.2,
					MemoryMB:   28.4,
					RawMem:     "28.4MiB / 8GiB",
					RawCPU:     "45.2%",
				},
			},
		},
	}

	if err := writeJSONReport(jsonPath, report); err != nil {
		t.Fatalf("writeJSONReport failed: %v", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("JSON report was not written or empty: %v", err)
	}

	if err := writeMarkdownReport(mdPath, report); err != nil {
		t.Fatalf("writeMarkdownReport failed: %v", err)
	}

	mdData, err := os.ReadFile(mdPath)
	if err != nil || len(mdData) == 0 {
		t.Fatalf("Markdown report was not written or empty: %v", err)
	}
}
