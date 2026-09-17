package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"toron/benchmarks/telemetry/gcparser"
	"toron/pkg/version"
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
				ProxyName:  fmt.Sprintf("Toron (%s)", version.ShortString()),
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
				ProxyName:     fmt.Sprintf("Toron (%s)", version.ShortString()),
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

func TestParseDurationTiers(t *testing.T) {
	cases := []struct {
		name         string
		durationStr  string
		tierStr      string
		expectedDurs []time.Duration
		expectedT    []string
		expectErr    bool
	}{
		{
			name:         "tier quick",
			durationStr:  "",
			tierStr:      "quick",
			expectedDurs: []time.Duration{5 * time.Second},
			expectedT:    []string{"quick"},
		},
		{
			name:         "tier medium",
			durationStr:  "",
			tierStr:      "medium",
			expectedDurs: []time.Duration{60 * time.Second},
			expectedT:    []string{"medium"},
		},
		{
			name:         "tier steady alias",
			durationStr:  "",
			tierStr:      "steady",
			expectedDurs: []time.Duration{60 * time.Second},
			expectedT:    []string{"medium"},
		},
		{
			name:         "tier soak",
			durationStr:  "",
			tierStr:      "soak",
			expectedDurs: []time.Duration{300 * time.Second},
			expectedT:    []string{"soak"},
		},
		{
			name:         "tier all",
			durationStr:  "",
			tierStr:      "all",
			expectedDurs: []time.Duration{5 * time.Second, 60 * time.Second, 300 * time.Second},
			expectedT:    []string{"quick", "medium", "soak"},
		},
		{
			name:        "invalid tier",
			durationStr: "",
			tierStr:     "infinite",
			expectErr:   true,
		},
		{
			name:         "custom duration single quick",
			durationStr:  "10s",
			tierStr:      "",
			expectedDurs: []time.Duration{10 * time.Second},
			expectedT:    []string{"quick"},
		},
		{
			name:         "custom duration single medium",
			durationStr:  "45s",
			tierStr:      "",
			expectedDurs: []time.Duration{45 * time.Second},
			expectedT:    []string{"medium"},
		},
		{
			name:         "custom duration single soak",
			durationStr:  "150s",
			tierStr:      "",
			expectedDurs: []time.Duration{150 * time.Second},
			expectedT:    []string{"soak"},
		},
		{
			name:         "custom comma-separated multi-tier",
			durationStr:  "5s,60s,300s",
			tierStr:      "",
			expectedDurs: []time.Duration{5 * time.Second, 60 * time.Second, 300 * time.Second},
			expectedT:    []string{"quick", "medium", "soak"},
		},
		{
			name:         "empty defaults to 5s quick",
			durationStr:  "",
			tierStr:      "",
			expectedDurs: []time.Duration{5 * time.Second},
			expectedT:    []string{"quick"},
		},
		{
			name:        "invalid duration",
			durationStr: "invalid-time",
			tierStr:     "",
			expectErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			durs, tiers, err := parseDurationTiers(tc.durationStr, tc.tierStr)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(durs) != len(tc.expectedDurs) {
				t.Fatalf("expected %d durations, got %d", len(tc.expectedDurs), len(durs))
			}
			for i := range durs {
				if durs[i] != tc.expectedDurs[i] {
					t.Errorf("duration[%d] = %v; want %v", i, durs[i], tc.expectedDurs[i])
				}
				if tiers[i] != tc.expectedT[i] {
					t.Errorf("tier[%d] = %s; want %s", i, tiers[i], tc.expectedT[i])
				}
			}
		})
	}
}

func TestComputeTimeSeriesStats(t *testing.T) {
	t.Run("empty samples", func(t *testing.T) {
		res := computeTimeSeriesStats(nil)
		if res == nil {
			t.Fatal("expected non-nil struct")
		}
		if res.PeakMemoryMB != 0 || res.GrowthSlope != 0 {
			t.Fatalf("expected 0 values for empty samples, got %+v", res)
		}
	})

	t.Run("single sample", func(t *testing.T) {
		res := computeTimeSeriesStats([]TimeSeriesSample{
			{ElapsedSec: 0, CPUPercent: 50.0, MemoryMB: 100.0},
		})
		if res.PeakMemoryMB != 100.0 || res.MeanMemoryMB != 100.0 {
			t.Errorf("expected 100.0 MB memory, got peak=%f, mean=%f", res.PeakMemoryMB, res.MeanMemoryMB)
		}
		if res.PeakCPU != 50.0 || res.MeanCPU != 50.0 {
			t.Errorf("expected 50.0 CPU, got peak=%f, mean=%f", res.PeakCPU, res.MeanCPU)
		}
		if res.GrowthSlope != 0 {
			t.Errorf("expected slope 0 with 1 sample, got %f", res.GrowthSlope)
		}
	})

	t.Run("linear growth slope", func(t *testing.T) {
		// 10 MB at t=0s, 20 MB at t=60s
		// Growth rate: (20 - 10) MB / 60 s = 0.16667 MB/s = 10.0 MB/min
		samples := []TimeSeriesSample{
			{ElapsedSec: 0, CPUPercent: 40.0, MemoryMB: 10.0},
			{ElapsedSec: 60, CPUPercent: 60.0, MemoryMB: 20.0},
		}
		res := computeTimeSeriesStats(samples)
		if math.Abs(res.GrowthSlope-10.0) > 1e-4 {
			t.Errorf("expected GrowthSlope ~ 10.0 MB/min, got %f", res.GrowthSlope)
		}
		if res.PeakMemoryMB != 20.0 {
			t.Errorf("expected peak memory 20.0, got %f", res.PeakMemoryMB)
		}
		if res.MeanMemoryMB != 15.0 {
			t.Errorf("expected mean memory 15.0, got %f", res.MeanMemoryMB)
		}
		if res.PeakCPU != 60.0 || res.MeanCPU != 50.0 {
			t.Errorf("expected peak CPU 60.0, mean CPU 50.0; got %f, %f", res.PeakCPU, res.MeanCPU)
		}
	})
}

func TestReportGenerationWithGCTelemetry(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "report_gc.json")
	mdPath := filepath.Join(tempDir, "report_gc.md")

	report := DockerCompareReport{
		Timestamp:       "2026-09-14T10:00:00Z",
		HostOS:          "darwin",
		HostArch:        "arm64",
		NumCPU:          8,
		Concurrency:     50,
		DurationSeconds: 60.0,
		Results: []BenchmarkCellResult{
			{
				ProxyID:       "toron",
				ProxyName:     fmt.Sprintf("Toron (%s)", version.ShortString()),
				BackendID:     "fast",
				BackendName:   "Go Fast Echo",
				TargetURL:     "http://127.0.0.1:8881/fast/health",
				Concurrency:   50,
				DurationSec:   60.0,
				DurationTier:  "medium",
				TotalRequests: 300000,
				SuccessCount:  300000,
				ActualRPS:     5000.0,
				Latencies: LatencyStats{
					P50: 0.7,
					P99: 2.1,
				},
				Telemetry: ResourceTelemetry{
					CPUPercent: 45.2,
					MemoryMB:   28.4,
				},
				GCTelemetry: &gcparser.GCTelemetry{
					Enabled:          true,
					TotalCycles:      12,
					CyclesPerSecond:  0.2,
					GCCPUPercent:     0.8,
					TotalReclaimedMB: 48.0,
					PauseTimesMs: gcparser.GCPauseStatistics{
						P50STWMs: 0.120,
						P99STWMs: 0.250,
						MaxSTWMs: 0.310,
					},
					HeapMetricsMB: gcparser.GCHeapStatistics{
						InitialLiveHeapMB:  10.0,
						FinalLiveHeapMB:    12.0,
						HeapGrowthSlopeMBm: 0.05,
					},
				},
				TimeSeries: &ResourceTimeSeries{
					PeakMemoryMB: 30.0,
					MeanMemoryMB: 28.0,
					GrowthSlope:  0.02,
				},
			},
			{
				ProxyID:       "nginx",
				ProxyName:     "NGINX (1.25)",
				BackendID:     "fast",
				DurationSec:   60.0,
				DurationTier:  "medium",
				TotalRequests: 280000,
				SuccessCount:  280000,
				ActualRPS:     4666.0,
				Latencies: LatencyStats{
					P50: 0.8,
					P99: 2.4,
				},
				Telemetry: ResourceTelemetry{
					CPUPercent: 55.0,
					MemoryMB:   15.0,
				},
				GCTelemetry: &gcparser.GCTelemetry{
					Enabled: false,
				},
			},
		},
	}

	if err := writeJSONReport(jsonPath, report); err != nil {
		t.Fatalf("writeJSONReport failed: %v", err)
	}

	if err := writeMarkdownReport(mdPath, report); err != nil {
		t.Fatalf("writeMarkdownReport failed: %v", err)
	}

	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("failed reading markdown report: %v", err)
	}
	mdContent := string(mdBytes)

	// Invariant checks on Section 4
	if !strings.Contains(mdContent, "## 4. Go Runtime GC Differential Analysis (Toron vs Traefik vs Caddy)") {
		t.Errorf("missing Section 4 Go Runtime GC Differential Analysis header in markdown")
	}
	expectedToronName := fmt.Sprintf("Toron (%s)", version.ShortString())
	if !strings.Contains(mdContent, expectedToronName) {
		t.Errorf("missing %s row in markdown GC analysis", expectedToronName)
	}
	// Check Section 1 Comparative Table columns
	if !strings.Contains(mdContent, "GC Cycles") || !strings.Contains(mdContent, "P99 GC Pause") {
		t.Errorf("missing GC columns in Section 1 comparative table")
	}
}
