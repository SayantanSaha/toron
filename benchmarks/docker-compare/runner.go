package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"toron/benchmarks/telemetry/gcparser"
	"toron/pkg/version"
)

// ProxyDescriptor defines metadata and address for a reverse proxy target.
type ProxyDescriptor struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContainerName string `json:"container_name"`
	Port          int    `json:"port"`
	HealthPath    string `json:"health_path"`
}

// BackendDescriptor defines metadata and route prefix for an upstream origin.
type BackendDescriptor struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PathPrefix string `json:"path_prefix"`
	ProbePath  string `json:"probe_path"`
}

// PreflightResult records pre-flight connectivity check outcome.
type PreflightResult struct {
	ProxyID    string  `json:"proxy_id"`
	ProxyName  string  `json:"proxy_name"`
	BackendID  string  `json:"backend_id"`
	URL        string  `json:"url"`
	StatusCode int     `json:"status_code"`
	Passed     bool    `json:"passed"`
	LatencyMs  float64 `json:"latency_ms"`
	Details    string  `json:"details,omitempty"`
}

// LatencyStats contains calculated latency percentiles in milliseconds.
type LatencyStats struct {
	Min   float64 `json:"min_ms"`
	Mean  float64 `json:"mean_ms"`
	P50   float64 `json:"p50_ms"`
	P75   float64 `json:"p75_ms"`
	P90   float64 `json:"p90_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
	P999  float64 `json:"p999_ms"`
	Max   float64 `json:"max_ms"`
}

// ResourceTelemetry records container CPU and memory consumption.
type ResourceTelemetry struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	RawMem     string  `json:"raw_mem"`
	RawCPU     string  `json:"raw_cpu"`
}

// TimeSeriesSample records a periodic container stats sample.
type TimeSeriesSample struct {
	ElapsedSec float64 `json:"elapsed_sec"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
}

// ResourceTimeSeries records dynamic container resource utilization over time.
type ResourceTimeSeries struct {
	Samples      []TimeSeriesSample `json:"samples"`
	PeakMemoryMB float64            `json:"peak_memory_mb"`
	MeanMemoryMB float64            `json:"mean_memory_mb"`
	PeakCPU      float64            `json:"peak_cpu"`
	MeanCPU      float64            `json:"mean_cpu"`
	GrowthSlope  float64            `json:"growth_slope_mb_per_min"`
}

// BenchmarkCellResult records results for one Proxy on one Backend origin.
type BenchmarkCellResult struct {
	ProxyID       string                `json:"proxy_id"`
	ProxyName     string                `json:"proxy_name"`
	BackendID     string                `json:"backend_id"`
	BackendName   string                `json:"backend_name"`
	TargetURL     string                `json:"target_url"`
	Concurrency   int                   `json:"concurrency"`
	DurationSec   float64               `json:"duration_seconds"`
	DurationTier  string                `json:"duration_tier,omitempty"`
	TotalRequests int64                 `json:"total_requests"`
	SuccessCount  int64                 `json:"success_count"`
	ErrorCount    int64                 `json:"error_count"`
	ActualRPS     float64               `json:"actual_rps"`
	ThroughputMBs float64               `json:"throughput_mb_s"`
	BytesRead     int64                 `json:"bytes_read"`
	Latencies     LatencyStats          `json:"latencies"`
	Telemetry     ResourceTelemetry     `json:"telemetry"`
	GCTelemetry   *gcparser.GCTelemetry `json:"gc_telemetry,omitempty"`
	TimeSeries    *ResourceTimeSeries   `json:"time_series,omitempty"`
}

// DockerCompareReport aggregates all empirical benchmark results.
type DockerCompareReport struct {
	Timestamp       string                `json:"timestamp"`
	HostOS          string                `json:"host_os"`
	HostArch        string                `json:"host_arch"`
	NumCPU          int                   `json:"num_cpu"`
	Concurrency     int                   `json:"concurrency"`
	DurationSeconds float64               `json:"duration_seconds"`
	DurationTier    string                `json:"duration_tier,omitempty"`
	TargetRateRPS   int                   `json:"target_rate_rps"`
	PreflightChecks []PreflightResult     `json:"preflight_checks"`
	Results         []BenchmarkCellResult `json:"results"`
}

var defaultProxies = []ProxyDescriptor{
	{ID: "toron", Name: fmt.Sprintf("Toron (%s)", version.ShortString()), ContainerName: "toron-cmp-toron", Port: 8881, HealthPath: "/health"},
	{ID: "nginx", Name: "NGINX (Alpine)", ContainerName: "toron-cmp-nginx", Port: 8882, HealthPath: "/health"},
	{ID: "traefik", Name: "Traefik (v3.1)", ContainerName: "toron-cmp-traefik", Port: 8883, HealthPath: "/ping"},
	{ID: "caddy", Name: "Caddy (Alpine)", ContainerName: "toron-cmp-caddy", Port: 8884, HealthPath: "/health"},
	{ID: "haproxy", Name: "HAProxy (Alpine)", ContainerName: "toron-cmp-haproxy", Port: 8885, HealthPath: "/health"},
}

var defaultBackends = []BackendDescriptor{
	{ID: "fast", Name: "Go Fast Echo", PathPrefix: "/fast", ProbePath: "/fast/health"},
	{ID: "go", Name: "Go Standard net/http", PathPrefix: "/go", ProbePath: "/go/health"},
	{ID: "node", Name: "Node.js 20 llhttp", PathPrefix: "/node", ProbePath: "/node/health"},
	{ID: "python", Name: "Python 3.11 uvicorn/h11", PathPrefix: "/python", ProbePath: "/python/health"},
}

func parseDurationTiers(durationStr, tierStr string) ([]time.Duration, []string, error) {
	if tierStr != "" {
		switch strings.ToLower(strings.TrimSpace(tierStr)) {
		case "quick":
			return []time.Duration{5 * time.Second}, []string{"quick"}, nil
		case "medium", "steady":
			return []time.Duration{60 * time.Second}, []string{"medium"}, nil
		case "soak":
			return []time.Duration{300 * time.Second}, []string{"soak"}, nil
		case "all":
			return []time.Duration{5 * time.Second, 60 * time.Second, 300 * time.Second}, []string{"quick", "medium", "soak"}, nil
		default:
			return nil, nil, fmt.Errorf("unknown tier '%s'. Valid: quick, medium, soak, all", tierStr)
		}
	}

	parts := strings.Split(durationStr, ",")
	durations := make([]time.Duration, 0, len(parts))
	tiers := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		d, err := time.ParseDuration(p)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid duration '%s': %w", p, err)
		}
		durations = append(durations, d)
		tier := "quick"
		if d >= 120*time.Second {
			tier = "soak"
		} else if d >= 30*time.Second {
			tier = "medium"
		}
		tiers = append(tiers, tier)
	}

	if len(durations) == 0 {
		return []time.Duration{5 * time.Second}, []string{"quick"}, nil
	}
	return durations, tiers, nil
}

func main() {
	var (
		preflightOnly bool
		proxyFilter   string
		backendFilter string
		concurrency   int
		durationStr   string
		tierStr       string
		targetRate    int
		jsonPath      string
		mdPath        string
	)

	flag.BoolVar(&preflightOnly, "preflight-only", false, "Run pre-flight functional routing checks only and exit")
	flag.StringVar(&proxyFilter, "proxies", "toron,nginx,traefik,caddy,haproxy", "Comma-separated list of proxy IDs to evaluate")
	flag.StringVar(&backendFilter, "backends", "fast,go,node,python", "Comma-separated list of backend IDs to evaluate")
	flag.IntVar(&concurrency, "c", 50, "Number of concurrent worker connections")
	flag.StringVar(&durationStr, "d", "5s", "Test duration per target (comma-separated, e.g. 5s,60s,300s)")
	flag.StringVar(&tierStr, "tier", "", "Duration tier preset (quick, medium, soak, all)")
	flag.IntVar(&targetRate, "r", 0, "Target rate in RPS per proxy (0 = maximum throughput saturation)")
	flag.StringVar(&jsonPath, "json", "benchmarks/results/docker_compare_report.json", "Destination path for JSON report")
	flag.StringVar(&mdPath, "md", "benchmarks/results/docker_compare_report.md", "Destination path for Markdown report")
	flag.Parse()

	durations, tiers, err := parseDurationTiers(durationStr, tierStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid duration/tier configuration: %v\n", err)
		os.Exit(1)
	}

	activeProxies := filterProxies(proxyFilter)
	activeBackends := filterBackends(backendFilter)

	fmt.Println("================================================================================")
	fmt.Println("       TORON DIFFERENTIAL DOCKER BENCHMARK HARNESS (MULTI-PROXY)                ")
	fmt.Println("================================================================================")
	fmt.Printf(" Active Proxies:  %s\n", strings.Join(getProxyNames(activeProxies), ", "))
	fmt.Printf(" Active Backends: %s\n", strings.Join(getBackendNames(activeBackends), ", "))
	fmt.Printf(" Concurrency:     %d workers\n", concurrency)
	fmt.Printf(" Durations:       %v (Tiers: %v)\n", durations, tiers)
	if targetRate > 0 {
		fmt.Printf(" Target Rate:     %d RPS (Rate-limited)\n", targetRate)
	} else {
		fmt.Printf(" Target Rate:     Unthrottled Saturation\n")
	}
	fmt.Println("================================================================================")

	// Step 1: Pre-Flight Routing & Health Checks
	fmt.Println("\n[*] Executing Pre-Flight Health & Route Verification...")
	preflightResults, preflightPassed := runPreflightChecks(activeProxies, activeBackends)

	printPreflightSummary(preflightResults)

	if !preflightPassed {
		fmt.Fprintf(os.Stderr, "\n[-] FATAL: Pre-flight validation failed for one or more endpoints. Aborting benchmark.\n")
		os.Exit(1)
	}
	fmt.Println("[✓] All proxies and upstream backends verified operational.")

	if preflightOnly {
		fmt.Println("[*] Pre-flight only flag specified. Exiting cleanly.")
		return
	}

	// Step 2: Comparative Load Testing Matrix
	report := DockerCompareReport{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		HostOS:          runtime.GOOS,
		HostArch:        runtime.GOARCH,
		NumCPU:          runtime.NumCPU(),
		Concurrency:     concurrency,
		DurationSeconds: durations[0].Seconds(),
		DurationTier:    tiers[0],
		TargetRateRPS:   targetRate,
		PreflightChecks: preflightResults,
		Results:         make([]BenchmarkCellResult, 0, len(durations)*len(activeProxies)*len(activeBackends)),
	}
	if len(durations) > 1 {
		report.DurationTier = "all"
	}

	fmt.Println("\n[*] Executing Benchmark Matrix across Proxies and Heterogeneous Upstreams...")

	for dIdx, duration := range durations {
		tier := tiers[dIdx]
		fmt.Printf("\n================================================================================")
		fmt.Printf("\n>>> EXECUTION TIER: %s (Duration: %s) <<<\n", strings.ToUpper(tier), duration)
		fmt.Printf("================================================================================\n")

		for _, proxy := range activeProxies {
			fmt.Printf("\n>>> Benchmarking Proxy: %s (Port %d, Container %s) [%s] <<<\n",
				proxy.Name, proxy.Port, proxy.ContainerName, tier)

			for _, backend := range activeBackends {
				targetURL := fmt.Sprintf("http://127.0.0.1:%d%s/health", proxy.Port, backend.PathPrefix)
				fmt.Printf("  -> Target: %s [%s] @ %s ... ", proxy.Name, backend.Name, targetURL)

				// Mandatory 5s warm-up phase for runs >= 30s
				if duration >= 30*time.Second {
					fmt.Print("[Warm-up 5s] ")
					prewarmTargetDuration(targetURL, 5*time.Second, concurrency)
				} else {
					prewarmTarget(targetURL, 100)
				}

				cellStartTime := time.Now()

				// Background continuous Docker stats polling for duration >= 30s
				var stopPoller func() *ResourceTimeSeries
				if duration >= 30*time.Second {
					pollInterval := 10 * time.Second
					if duration < 120*time.Second {
						pollInterval = 5 * time.Second
					}
					cellCtx, cellCancel := context.WithTimeout(context.Background(), duration+2*time.Second)
					defer cellCancel()
					stopPoller = startContainerStatsPoller(cellCtx, proxy.ContainerName, pollInterval)
				}

				// Execute benchmark load cell
				cellResult := runBenchmarkCell(proxy, backend, targetURL, concurrency, duration, targetRate)
				cellResult.DurationTier = tier

				if stopPoller != nil {
					ts := stopPoller()
					cellResult.TimeSeries = ts
					cellResult.Telemetry = ResourceTelemetry{
						CPUPercent: ts.MeanCPU,
						MemoryMB:   ts.PeakMemoryMB,
						RawCPU:     fmt.Sprintf("%.1f%%", ts.MeanCPU),
						RawMem:     fmt.Sprintf("%.1fMiB", ts.PeakMemoryMB),
					}
				} else {
					telemetry := sampleContainerTelemetry(proxy.ContainerName)
					cellResult.Telemetry = telemetry
				}

				// Extract container GC trace for Go-based proxies
				gcTele, err := extractContainerGCTrace(proxy.ID, proxy.ContainerName, cellStartTime, cellResult.DurationSec)
				if err == nil && gcTele != nil {
					cellResult.GCTelemetry = gcTele
				}

				report.Results = append(report.Results, cellResult)

				gcInfo := "GC: N/A"
				if cellResult.GCTelemetry != nil && cellResult.GCTelemetry.Enabled {
					gcInfo = fmt.Sprintf("GC: %d cycles, P99: %.2fms", cellResult.GCTelemetry.TotalCycles, cellResult.GCTelemetry.PauseTimesMs.P99STWMs)
				}

				fmt.Printf("Done: %.1f RPS | P50: %.2fms | P99: %.2fms | Errors: %d | Mem: %.1f MB | CPU: %.1f%% | %s\n",
					cellResult.ActualRPS,
					cellResult.Latencies.P50,
					cellResult.Latencies.P99,
					cellResult.ErrorCount,
					cellResult.Telemetry.MemoryMB,
					cellResult.Telemetry.CPUPercent,
					gcInfo,
				)

				time.Sleep(500 * time.Millisecond) // Inter-test cooldown
			}

			// Inter-proxy cooldown to prevent thermal throttling on long soak runs
			if duration >= 30*time.Second {
				fmt.Println("  [*] Cooldown 10s between proxy targets...")
				time.Sleep(10 * time.Second)
			}
		}
	}

	// Step 3: Generate Reports
	fmt.Println("\n[*] Writing benchmark evaluation reports...")
	_ = os.MkdirAll(filepath.Dir(jsonPath), 0o755)
	_ = os.MkdirAll(filepath.Dir(mdPath), 0o755)

	if err := writeJSONReport(jsonPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "[-] Failed to write JSON report: %v\n", err)
	} else {
		fmt.Printf("[✓] JSON Report written to: %s\n", jsonPath)
	}

	if err := writeMarkdownReport(mdPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "[-] Failed to write Markdown report: %v\n", err)
	} else {
		fmt.Printf("[✓] Markdown Report written to: %s\n", mdPath)
	}

	// Output console comparative table
	printConsoleTable(report)
}

func filterProxies(filter string) []ProxyDescriptor {
	if filter == "" || filter == "all" {
		return defaultProxies
	}
	parts := strings.Split(filter, ",")
	lookup := make(map[string]bool)
	for _, p := range parts {
		lookup[strings.ToLower(strings.TrimSpace(p))] = true
	}
	var filtered []ProxyDescriptor
	for _, p := range defaultProxies {
		if lookup[strings.ToLower(p.ID)] {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return defaultProxies
	}
	return filtered
}

func filterBackends(filter string) []BackendDescriptor {
	if filter == "" || filter == "all" {
		return defaultBackends
	}
	parts := strings.Split(filter, ",")
	lookup := make(map[string]bool)
	for _, b := range parts {
		lookup[strings.ToLower(strings.TrimSpace(b))] = true
	}
	var filtered []BackendDescriptor
	for _, b := range defaultBackends {
		if lookup[strings.ToLower(b.ID)] {
			filtered = append(filtered, b)
		}
	}
	if len(filtered) == 0 {
		return defaultBackends
	}
	return filtered
}

func getProxyNames(proxies []ProxyDescriptor) []string {
	names := make([]string, len(proxies))
	for i, p := range proxies {
		names[i] = p.Name
	}
	return names
}

func getBackendNames(backends []BackendDescriptor) []string {
	names := make([]string, len(backends))
	for i, b := range backends {
		names[i] = b.Name
	}
	return names
}

func runPreflightChecks(proxies []ProxyDescriptor, backends []BackendDescriptor) ([]PreflightResult, bool) {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	var results []PreflightResult
	allPassed := true

	for _, proxy := range proxies {
		for _, backend := range backends {
			targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", proxy.Port, backend.ProbePath)
			start := time.Now()
			resp, err := client.Get(targetURL)
			latency := float64(time.Since(start).Microseconds()) / 1000.0

			res := PreflightResult{
				ProxyID:   proxy.ID,
				ProxyName: proxy.Name,
				BackendID: backend.ID,
				URL:       targetURL,
				LatencyMs: latency,
			}

			if err != nil {
				res.Passed = false
				res.StatusCode = 0
				res.Details = err.Error()
				allPassed = false
			} else {
				res.StatusCode = resp.StatusCode
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					res.Passed = true
					res.Details = fmt.Sprintf("OK (body len: %d)", len(body))
				} else {
					res.Passed = false
					res.Details = fmt.Sprintf("Unexpected status: %s", resp.Status)
					allPassed = false
				}
			}
			results = append(results, res)
		}
	}

	return results, allPassed
}

func printPreflightSummary(results []PreflightResult) {
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("%-20s | %-12s | %-6s | %-9s | %s\n", "Proxy", "Backend", "Status", "Latency", "Details")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, r := range results {
		statusStr := fmt.Sprintf("%d", r.StatusCode)
		passTag := "[PASS]"
		if !r.Passed {
			passTag = "[FAIL]"
			statusStr = "ERR"
		}
		fmt.Printf("%-20s | %-12s | %-6s | %7.2fms | %s %s\n",
			r.ProxyName, r.BackendID, statusStr, r.LatencyMs, passTag, r.Details)
	}
	fmt.Println("--------------------------------------------------------------------------------")
}

func prewarmTarget(url string, count int) {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	for i := 0; i < count; i++ {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func prewarmTargetDuration(targetURL string, duration time.Duration, concurrency int) {
	if concurrency <= 0 {
		concurrency = 10
	}
	transport := &http.Transport{
		MaxIdleConns:        concurrency * 2,
		MaxIdleConnsPerHost: concurrency * 2,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   2 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
					if err == nil {
						resp, err := client.Do(req)
						if err == nil {
							_, _ = io.Copy(io.Discard, resp.Body)
							_ = resp.Body.Close()
						}
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()
}

func startContainerStatsPoller(ctx context.Context, containerName string, interval time.Duration) func() *ResourceTimeSeries {
	var (
		mu      sync.Mutex
		samples []TimeSeriesSample
		start   = time.Now()
	)

	stopCh := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial sample
		s0 := sampleContainerTelemetry(containerName)
		mu.Lock()
		samples = append(samples, TimeSeriesSample{
			ElapsedSec: 0.0,
			CPUPercent: s0.CPUPercent,
			MemoryMB:   s0.MemoryMB,
		})
		mu.Unlock()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				elapsed := time.Since(start).Seconds()
				tel := sampleContainerTelemetry(containerName)
				mu.Lock()
				samples = append(samples, TimeSeriesSample{
					ElapsedSec: elapsed,
					CPUPercent: tel.CPUPercent,
					MemoryMB:   tel.MemoryMB,
				})
				mu.Unlock()
			}
		}
	}()

	return func() *ResourceTimeSeries {
		close(stopCh)
		mu.Lock()
		defer mu.Unlock()
		return computeTimeSeriesStats(samples)
	}
}

func computeTimeSeriesStats(samples []TimeSeriesSample) *ResourceTimeSeries {
	if len(samples) == 0 {
		return &ResourceTimeSeries{}
	}

	var peakMem, sumMem, peakCPU, sumCPU float64
	var sumT, sumY, sumTY, sumT2 float64
	n := float64(len(samples))

	for _, s := range samples {
		if s.MemoryMB > peakMem {
			peakMem = s.MemoryMB
		}
		sumMem += s.MemoryMB

		if s.CPUPercent > peakCPU {
			peakCPU = s.CPUPercent
		}
		sumCPU += s.CPUPercent

		t := s.ElapsedSec
		y := s.MemoryMB
		sumT += t
		sumY += y
		sumTY += t * y
		sumT2 += t * t
	}

	var slope float64
	denom := n*sumT2 - sumT*sumT
	if len(samples) > 1 && math.Abs(denom) > 1e-9 {
		slope = ((n*sumTY - sumT*sumY) / denom) * 60.0
	}

	return &ResourceTimeSeries{
		Samples:      samples,
		PeakMemoryMB: peakMem,
		MeanMemoryMB: sumMem / n,
		PeakCPU:      peakCPU,
		MeanCPU:      sumCPU / n,
		GrowthSlope:  slope,
	}
}

func extractContainerGCTrace(proxyID, containerName string, startTime time.Time, durationSec float64) (*gcparser.GCTelemetry, error) {
	if proxyID != "toron" && proxyID != "traefik" && proxyID != "caddy" {
		return &gcparser.GCTelemetry{Enabled: false}, nil
	}

	sinceStr := startTime.UTC().Format(time.RFC3339Nano)
	cmd := exec.Command("docker", "logs", "--since", sinceStr, containerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to read docker logs: %w", err)
	}

	var gcLines strings.Builder
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "gc ") {
			gcLines.WriteString(trimmed)
			gcLines.WriteByte('\n')
		}
	}

	return gcparser.ParseReader(strings.NewReader(gcLines.String()), time.Duration(durationSec*float64(time.Second)))
}

func runBenchmarkCell(
	proxy ProxyDescriptor,
	backend BackendDescriptor,
	targetURL string,
	concurrency int,
	duration time.Duration,
	targetRate int,
) BenchmarkCellResult {
	transport := &http.Transport{
		MaxIdleConns:        concurrency * 4,
		MaxIdleConnsPerHost: concurrency * 4,
		IdleConnTimeout:     60 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var (
		totalReqs  int64
		successReq int64
		errorReq   int64
		totalBytes int64
		mu         sync.Mutex
		latencies  = make([]float64, 0, 50000)
	)

	var wg sync.WaitGroup
	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			localLatencies := make([]float64, 0, 1000)

			for {
				select {
				case <-ctx.Done():
					mu.Lock()
					latencies = append(latencies, localLatencies...)
					mu.Unlock()
					return
				default:
					t0 := time.Now()
					req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
					if err != nil {
						atomic.AddInt64(&errorReq, 1)
						atomic.AddInt64(&totalReqs, 1)
						continue
					}

					resp, err := client.Do(req)
					latMs := float64(time.Since(t0).Microseconds()) / 1000.0

					if err != nil {
						if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
							atomic.AddInt64(&errorReq, 1)
						}
					} else {
						n, _ := io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()

						atomic.AddInt64(&totalBytes, n)
						if resp.StatusCode >= 200 && resp.StatusCode < 400 {
							atomic.AddInt64(&successReq, 1)
						} else {
							atomic.AddInt64(&errorReq, 1)
						}
						localLatencies = append(localLatencies, latMs)
					}
					atomic.AddInt64(&totalReqs, 1)

					// If rate limiting specified, sleep proportional interval
					if targetRate > 0 {
						interval := time.Duration(int64(time.Second) * int64(concurrency) / int64(targetRate))
						time.Sleep(interval)
					}
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(startTime).Seconds()
	if elapsed <= 0 {
		elapsed = duration.Seconds()
	}

	actualRPS := float64(successReq) / elapsed
	throughputMBs := (float64(totalBytes) / (1024 * 1024)) / elapsed

	stats := computeLatencyStats(latencies)

	return BenchmarkCellResult{
		ProxyID:       proxy.ID,
		ProxyName:     proxy.Name,
		BackendID:     backend.ID,
		BackendName:   backend.Name,
		TargetURL:     targetURL,
		Concurrency:   concurrency,
		DurationSec:   elapsed,
		TotalRequests: totalReqs,
		SuccessCount:  successReq,
		ErrorCount:    errorReq,
		ActualRPS:     actualRPS,
		ThroughputMBs: throughputMBs,
		BytesRead:     totalBytes,
		Latencies:     stats,
	}
}

func computeLatencyStats(latencies []float64) LatencyStats {
	if len(latencies) == 0 {
		return LatencyStats{}
	}

	sort.Float64s(latencies)
	n := len(latencies)

	var sum float64
	for _, v := range latencies {
		sum += v
	}
	mean := sum / float64(n)

	percentile := func(p float64) float64 {
		idx := int(float64(n) * p)
		if idx >= n {
			idx = n - 1
		}
		return latencies[idx]
	}

	return LatencyStats{
		Min:   latencies[0],
		Mean:  mean,
		P50:   percentile(0.50),
		P75:   percentile(0.75),
		P90:   percentile(0.90),
		P95:   percentile(0.95),
		P99:   percentile(0.99),
		P999:  percentile(0.999),
		Max:   latencies[n-1],
	}
}

func sampleContainerTelemetry(containerName string) ResourceTelemetry {
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}", containerName)
	out, err := cmd.Output()
	if err != nil {
		return ResourceTelemetry{RawCPU: "N/A", RawMem: "N/A"}
	}

	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, "\t")
	if len(parts) < 3 {
		return ResourceTelemetry{RawCPU: "N/A", RawMem: "N/A"}
	}

	rawCPU := strings.TrimSpace(parts[1])
	rawMem := strings.TrimSpace(parts[2])

	var cpuPct float64
	cpuClean := strings.TrimSuffix(rawCPU, "%")
	if val, err := strconv.ParseFloat(cpuClean, 64); err == nil {
		cpuPct = val
	}

	var memMB float64
	memParts := strings.Split(rawMem, "/")
	if len(memParts) > 0 {
		used := strings.TrimSpace(memParts[0])
		if strings.HasSuffix(used, "MiB") {
			v := strings.TrimSuffix(used, "MiB")
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				memMB = f
			}
		} else if strings.HasSuffix(used, "GiB") {
			v := strings.TrimSuffix(used, "GiB")
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				memMB = f * 1024
			}
		} else if strings.HasSuffix(used, "kB") || strings.HasSuffix(used, "KiB") {
			v := strings.TrimSuffix(strings.TrimSuffix(used, "kB"), "KiB")
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				memMB = f / 1024
			}
		}
	}

	return ResourceTelemetry{
		CPUPercent: cpuPct,
		MemoryMB:   memMB,
		RawMem:     rawMem,
		RawCPU:     rawCPU,
	}
}

func writeJSONReport(destPath string, report DockerCompareReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0o644)
}

func writeMarkdownReport(destPath string, report DockerCompareReport) error {
	var sb strings.Builder

	sb.WriteString("# 📊 Multi-Proxy Differential Docker Benchmark Report\n\n")
	sb.WriteString(fmt.Sprintf("**Date**: `%s` | **OS**: `%s/%s` | **Host CPUs**: `%d`\n\n",
		report.Timestamp, report.HostOS, report.HostArch, report.NumCPU))
	sb.WriteString(fmt.Sprintf("**Workload Profile**: Concurrency: `%d` | Duration per Target: `%.1fs` | Tier: `%s` | Target Rate: `%d RPS`\n\n",
		report.Concurrency, report.DurationSeconds, report.DurationTier, report.TargetRateRPS))

	sb.WriteString("## 1. Comparative Performance Matrix (Head-to-Head)\n\n")
	sb.WriteString("| Proxy Engine | Upstream Backend | Tier | Duration | Throughput (RPS) | P50 (ms) | P90 (ms) | P99 (ms) | P99.9 (ms) | Max (ms) | Memory RSS | GC Cycles | P99 GC Pause | CPU % | Errors |\n")
	sb.WriteString("|:---|:---|:---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")

	for _, r := range report.Results {
		gcCyclesStr := "-"
		gcP99Str := "-"
		if r.GCTelemetry != nil && r.GCTelemetry.Enabled {
			gcCyclesStr = fmt.Sprintf("%d", r.GCTelemetry.TotalCycles)
			gcP99Str = fmt.Sprintf("%.3f ms", r.GCTelemetry.PauseTimesMs.P99STWMs)
		} else if r.ProxyID == "nginx" || r.ProxyID == "haproxy" {
			gcCyclesStr = "N/A (C)"
			gcP99Str = "N/A (C)"
		}

		tierStr := r.DurationTier
		if tierStr == "" {
			tierStr = "-"
		}

		sb.WriteString(fmt.Sprintf("| **%s** | `%s` | `%s` | `%.1fs` | **%.1f** | %.2f | %.2f | %.2f | %.2f | %.2f | %.1f MB | %s | %s | %.1f%% | %d |\n",
			r.ProxyName,
			r.BackendID,
			tierStr,
			r.DurationSec,
			r.ActualRPS,
			r.Latencies.P50,
			r.Latencies.P90,
			r.Latencies.P99,
			r.Latencies.P999,
			r.Latencies.Max,
			r.Telemetry.MemoryMB,
			gcCyclesStr,
			gcP99Str,
			r.Telemetry.CPUPercent,
			r.ErrorCount,
		))
	}

	sb.WriteString("\n## 2. Upstream Backend Runtime Breakdown\n\n")
	backends := make(map[string][]BenchmarkCellResult)
	for _, r := range report.Results {
		backends[r.BackendID] = append(backends[r.BackendID], r)
	}

	backendOrder := []string{"fast", "go", "node", "python"}
	for _, bID := range backendOrder {
		items, ok := backends[bID]
		if !ok || len(items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### Backend: `%s` (%s)\n\n", bID, items[0].BackendName))
		sb.WriteString("| Rank | Proxy | Tier | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |\n")
		sb.WriteString("|:---|:---|:---:|---:|---:|---:|---:|---:|\n")

		sortedItems := make([]BenchmarkCellResult, len(items))
		copy(sortedItems, items)
		sort.Slice(sortedItems, func(i, j int) bool {
			return sortedItems[i].ActualRPS > sortedItems[j].ActualRPS
		})

		for rank, item := range sortedItems {
			badge := fmt.Sprintf("#%d", rank+1)
			if rank == 0 {
				badge = "🥇 #1"
			} else if rank == 1 {
				badge = "🥈 #2"
			} else if rank == 2 {
				badge = "🥉 #3"
			}
			tierStr := item.DurationTier
			if tierStr == "" {
				tierStr = "-"
			}
			sb.WriteString(fmt.Sprintf("| %s | **%s** | `%s` | **%.1f** | %.2f | %.2f | %.1f MB | %.1f%% |\n",
				badge, item.ProxyName, tierStr, item.ActualRPS, item.Latencies.P50, item.Latencies.P99, item.Telemetry.MemoryMB, item.Telemetry.CPUPercent))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 3. Pre-Flight Functional Parity Verification\n\n")
	sb.WriteString("| Proxy | Upstream Origin | Health URL | Status Code | Latency | Result |\n")
	sb.WriteString("|:---|:---|:---|---:|---:|:---|\n")
	for _, pf := range report.PreflightChecks {
		passIcon := "✅ PASS"
		if !pf.Passed {
			passIcon = "❌ FAIL"
		}
		sb.WriteString(fmt.Sprintf("| %s | `%s` | `%s` | %d | %.2fms | %s |\n",
			pf.ProxyName, pf.BackendID, pf.URL, pf.StatusCode, pf.LatencyMs, passIcon))
	}

	// Section 4: Go Runtime GC Differential Analysis
	hasGoGC := false
	for _, r := range report.Results {
		if r.GCTelemetry != nil && r.GCTelemetry.Enabled {
			hasGoGC = true
			break
		}
	}
	if hasGoGC {
		sb.WriteString("\n## 4. Go Runtime GC Differential Analysis (Toron vs Traefik vs Caddy)\n\n")
		sb.WriteString("| Proxy Engine | Backend | Tier | Total Cycles | Cycles/sec | GC CPU % | Reclaimed | P50 STW | P99 STW | Max STW | Live Heap Floor | Heap Slope |\n")
		sb.WriteString("|:---|:---|:---:|---:|---:|---:|---:|---:|---:|---:|:---:|---:|\n")
		for _, r := range report.Results {
			if r.GCTelemetry != nil && r.GCTelemetry.Enabled {
				g := r.GCTelemetry
				heapFloor := fmt.Sprintf("%.1f -> %.1f MB", g.HeapMetricsMB.InitialLiveHeapMB, g.HeapMetricsMB.FinalLiveHeapMB)
				sb.WriteString(fmt.Sprintf("| **%s** | `%s` | `%s` | %d | %.2f | %.1f%% | %.1f MB | %.3f ms | %.3f ms | %.3f ms | %s | %.2f MB/min |\n",
					r.ProxyName, r.BackendID, r.DurationTier, g.TotalCycles, g.CyclesPerSecond, g.GCCPUPercent,
					g.TotalReclaimedMB, g.PauseTimesMs.P50STWMs, g.PauseTimesMs.P99STWMs, g.PauseTimesMs.MaxSTWMs,
					heapFloor, g.HeapMetricsMB.HeapGrowthSlopeMBm))
			}
		}
	}

	sb.WriteString("\n---\n*Report automatically generated by Toron Differential Docker Benchmark Suite*\n")

	return os.WriteFile(destPath, []byte(sb.String()), 0o644)
}

func printConsoleTable(report DockerCompareReport) {
	fmt.Println("\n=========================================================================================================")
	fmt.Println("                                FINAL COMPARATIVE BENCHMARK RESULTS                                      ")
	fmt.Println("=========================================================================================================")
	fmt.Printf("%-18s | %-8s | %-8s | %-9s | %-8s | %-8s | %-10s | %-10s | %s\n",
		"Proxy", "Backend", "Tier", "RPS", "P50(ms)", "P99(ms)", "Mem RSS", "GC Cycles", "Errors")
	fmt.Println("---------------------------------------------------------------------------------------------------------")
	for _, r := range report.Results {
		gcStr := "-"
		if r.GCTelemetry != nil && r.GCTelemetry.Enabled {
			gcStr = fmt.Sprintf("%d", r.GCTelemetry.TotalCycles)
		} else if r.ProxyID == "nginx" || r.ProxyID == "haproxy" {
			gcStr = "N/A(C)"
		}
		tierStr := r.DurationTier
		if tierStr == "" {
			tierStr = "-"
		}
		fmt.Printf("%-18s | %-8s | %-8s | %9.1f | %8.2f | %8.2f | %8.1fMB | %10s | %d\n",
			r.ProxyName, r.BackendID, tierStr, r.ActualRPS, r.Latencies.P50, r.Latencies.P99, r.Telemetry.MemoryMB, gcStr, r.ErrorCount)
	}
	fmt.Println("=========================================================================================================")
}
