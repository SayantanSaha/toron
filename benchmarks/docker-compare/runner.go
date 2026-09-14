package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	ProxyID    string `json:"proxy_id"`
	ProxyName  string `json:"proxy_name"`
	BackendID  string `json:"backend_id"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Passed     bool   `json:"passed"`
	LatencyMs  float64 `json:"latency_ms"`
	Details    string `json:"details,omitempty"`
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

// BenchmarkCellResult records results for one Proxy on one Backend origin.
type BenchmarkCellResult struct {
	ProxyID       string            `json:"proxy_id"`
	ProxyName     string            `json:"proxy_name"`
	BackendID     string            `json:"backend_id"`
	BackendName   string            `json:"backend_name"`
	TargetURL     string            `json:"target_url"`
	Concurrency   int               `json:"concurrency"`
	DurationSec   float64           `json:"duration_seconds"`
	TotalRequests int64             `json:"total_requests"`
	SuccessCount  int64             `json:"success_count"`
	ErrorCount    int64             `json:"error_count"`
	ActualRPS     float64           `json:"actual_rps"`
	ThroughputMBs float64           `json:"throughput_mb_s"`
	BytesRead     int64             `json:"bytes_read"`
	Latencies     LatencyStats      `json:"latencies"`
	Telemetry     ResourceTelemetry `json:"telemetry"`
}

// DockerCompareReport aggregates all empirical benchmark results.
type DockerCompareReport struct {
	Timestamp       string                `json:"timestamp"`
	HostOS          string                `json:"host_os"`
	HostArch        string                `json:"host_arch"`
	NumCPU          int                   `json:"num_cpu"`
	Concurrency     int                   `json:"concurrency"`
	DurationSeconds float64               `json:"duration_seconds"`
	TargetRateRPS   int                   `json:"target_rate_rps"`
	PreflightChecks []PreflightResult     `json:"preflight_checks"`
	Results         []BenchmarkCellResult `json:"results"`
}

var defaultProxies = []ProxyDescriptor{
	{ID: "toron", Name: "Toron (v1.0.0)", ContainerName: "toron-cmp-toron", Port: 8881, HealthPath: "/health"},
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

func main() {
	var (
		preflightOnly bool
		proxyFilter   string
		backendFilter string
		concurrency   int
		durationStr   string
		targetRate    int
		jsonPath      string
		mdPath        string
	)

	flag.BoolVar(&preflightOnly, "preflight-only", false, "Run pre-flight functional routing checks only and exit")
	flag.StringVar(&proxyFilter, "proxies", "toron,nginx,traefik,caddy,haproxy", "Comma-separated list of proxy IDs to evaluate")
	flag.StringVar(&backendFilter, "backends", "fast,go,node,python", "Comma-separated list of backend IDs to evaluate")
	flag.IntVar(&concurrency, "c", 50, "Number of concurrent worker connections")
	flag.StringVar(&durationStr, "d", "5s", "Test duration per target (e.g. 5s, 10s)")
	flag.IntVar(&targetRate, "r", 0, "Target rate in RPS per proxy (0 = maximum throughput saturation)")
	flag.StringVar(&jsonPath, "json", "benchmarks/results/docker_compare_report.json", "Destination path for JSON report")
	flag.StringVar(&mdPath, "md", "benchmarks/results/docker_compare_report.md", "Destination path for Markdown report")
	flag.Parse()

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid duration %q: %v\n", durationStr, err)
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
	fmt.Printf(" Duration:        %s per target\n", duration)
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
		DurationSeconds: duration.Seconds(),
		TargetRateRPS:   targetRate,
		PreflightChecks: preflightResults,
		Results:         make([]BenchmarkCellResult, 0, len(activeProxies)*len(activeBackends)),
	}

	fmt.Println("\n[*] Executing Benchmark Matrix across Proxies and Heterogeneous Upstreams...")

	for _, proxy := range activeProxies {
		fmt.Printf("\n>>> Benchmarking Proxy: %s (Port %d, Container %s) <<<\n", proxy.Name, proxy.Port, proxy.ContainerName)

		for _, backend := range activeBackends {
			targetURL := fmt.Sprintf("http://127.0.0.1:%d%s/health", proxy.Port, backend.PathPrefix)
			fmt.Printf("  -> Target: %s [%s] @ %s ... ", proxy.Name, backend.Name, targetURL)

			// Pre-warm proxy and upstream connection pool
			prewarmTarget(targetURL, 100)

			// Execute load test
			cellResult := runBenchmarkCell(proxy, backend, targetURL, concurrency, duration, targetRate)

			// Sample Docker container stats
			telemetry := sampleContainerTelemetry(proxy.ContainerName)
			cellResult.Telemetry = telemetry

			report.Results = append(report.Results, cellResult)

			fmt.Printf("Done: %.1f RPS | P50: %.2fms | P99: %.2fms | Errors: %d | Mem: %.1f MB | CPU: %.1f%%\n",
				cellResult.ActualRPS,
				cellResult.Latencies.P50,
				cellResult.Latencies.P99,
				cellResult.ErrorCount,
				cellResult.Telemetry.MemoryMB,
				cellResult.Telemetry.CPUPercent,
			)

			time.Sleep(500 * time.Millisecond) // Inter-test cooldown
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

	// MemUsage is typically "14.5MiB / 7.669GiB"
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
	sb.WriteString(fmt.Sprintf("**Workload Profile**: Concurrency: `%d` | Duration per Target: `%.1fs` | Target Rate: `%d RPS`\n\n",
		report.Concurrency, report.DurationSeconds, report.TargetRateRPS))

	sb.WriteString("## 1. Comparative Performance Matrix (Head-to-Head)\n\n")
	sb.WriteString("| Proxy Engine | Upstream Backend | Throughput (RPS) | P50 (ms) | P90 (ms) | P99 (ms) | P99.9 (ms) | Max (ms) | Memory RSS | CPU % | Errors |\n")
	sb.WriteString("|:---|:---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")

	for _, r := range report.Results {
		sb.WriteString(fmt.Sprintf("| **%s** | `%s` | **%.1f** | %.2f | %.2f | %.2f | %.2f | %.2f | %.1f MB | %.1f%% | %d |\n",
			r.ProxyName,
			r.BackendID,
			r.ActualRPS,
			r.Latencies.P50,
			r.Latencies.P90,
			r.Latencies.P99,
			r.Latencies.P999,
			r.Latencies.Max,
			r.Telemetry.MemoryMB,
			r.Telemetry.CPUPercent,
			r.ErrorCount,
		))
	}

	sb.WriteString("\n## 2. Upstream Backend Runtime Breakdown\n\n")
	// Group by backend
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
		sb.WriteString("| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |\n")
		sb.WriteString("|:---|:---|---:|---:|---:|---:|---:|\n")

		// Sort by RPS descending
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
			sb.WriteString(fmt.Sprintf("| %s | **%s** | **%.1f** | %.2f | %.2f | %.1f MB | %.1f%% |\n",
				badge, item.ProxyName, item.ActualRPS, item.Latencies.P50, item.Latencies.P99, item.Telemetry.MemoryMB, item.Telemetry.CPUPercent))
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

	sb.WriteString("\n---\n*Report automatically generated by Toron Differential Docker Benchmark Suite*\n")

	return os.WriteFile(destPath, []byte(sb.String()), 0o644)
}

func printConsoleTable(report DockerCompareReport) {
	fmt.Println("\n================================================================================")
	fmt.Println("                       FINAL COMPARATIVE BENCHMARK RESULTS                      ")
	fmt.Println("================================================================================")
	fmt.Printf("%-18s | %-8s | %-9s | %-8s | %-8s | %-10s | %s\n",
		"Proxy", "Backend", "RPS", "P50(ms)", "P99(ms)", "Mem RSS", "Errors")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, r := range report.Results {
		fmt.Printf("%-18s | %-8s | %9.1f | %8.2f | %8.2f | %8.1fMB | %d\n",
			r.ProxyName, r.BackendID, r.ActualRPS, r.Latencies.P50, r.Latencies.P99, r.Telemetry.MemoryMB, r.ErrorCount)
	}
	fmt.Println("================================================================================")
}
