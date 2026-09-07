package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type BenchmarkResult struct {
	TargetURL       string             `json:"target_url"`
	Concurrency     int                `json:"concurrency"`
	DurationSeconds float64            `json:"duration_seconds"`
	TargetRateRPS   int                `json:"target_rate_rps"`
	TotalRequests   int64              `json:"total_requests"`
	SuccessRequests int64              `json:"success_requests"`
	FailedRequests  int64              `json:"failed_requests"`
	ActualRPS       float64            `json:"actual_rps"`
	BytesRead       int64              `json:"bytes_read"`
	ThroughputMBs   float64            `json:"throughput_mb_s"`
	LatenciesMs     LatencyPercentiles `json:"latencies_ms"`
	StatusCodes     map[int]int64      `json:"status_codes"`
}

type LatencyPercentiles struct {
	Min   float64 `json:"min"`
	Mean  float64 `json:"mean"`
	P50   float64 `json:"p50"`
	P75   float64 `json:"p75"`
	P90   float64 `json:"p90"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	P999  float64 `json:"p999"`
	Max   float64 `json:"max"`
}

func main() {
	targetURL := flag.String("url", "http://127.0.0.1:8080/health", "Target HTTP URL")
	concurrency := flag.Int("c", 50, "Number of concurrent connections / workers")
	duration := flag.Duration("d", 10*time.Second, "Test duration (e.g. 10s, 30s, 1m)")
	targetRate := flag.Int("rate", 10000, "Target total requests per second (0 = unbounded max rate)")
	method := flag.String("m", "GET", "HTTP method (GET, POST, etc.)")
	bodyPayload := flag.String("body", "", "HTTP POST/PUT body payload")
	jsonPath := flag.String("json", "", "Path to write JSON results")
	csvPath := flag.String("csv", "", "Path to write CSV results")
	flag.Parse()

	if *concurrency <= 0 {
		*concurrency = 1
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        *concurrency * 2,
		MaxIdleConnsPerHost: *concurrency * 2,
		MaxConnsPerHost:     *concurrency * 4,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression: true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	fmt.Println("================================================================================")
	fmt.Println("   TORON HIGH-PRECISION LOAD GENERATOR & LATENCY PROFILER (wrk2-compatible)    ")
	fmt.Println("================================================================================")
	fmt.Printf(" Target URL:   %s\n", *targetURL)
	fmt.Printf(" Concurrency:  %d connections\n", *concurrency)
	fmt.Printf(" Duration:     %s\n", duration.String())
	if *targetRate > 0 {
		fmt.Printf(" Target Rate:  %d req/sec\n", *targetRate)
	} else {
		fmt.Printf(" Target Rate:  Unbounded (Max throughput)\n")
	}
	fmt.Println(" Running benchmark warmup and execution...")

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var totalReqs atomic.Int64
	var successReqs atomic.Int64
	var failedReqs atomic.Int64
	var totalBytes atomic.Int64

	statusLock := sync.Mutex{}
	statusCodes := make(map[int]int64)

	// Collect latency samples per worker
	workerSamples := make([][]time.Duration, *concurrency)
	var wg sync.WaitGroup

	startTime := time.Now()

	// Rate limiter channel if targetRate > 0
	var rateTicker *time.Ticker
	var rateChan <-chan time.Time
	if *targetRate > 0 {
		rateTicker = time.NewTicker(time.Second / time.Duration(*targetRate))
		defer rateTicker.Stop()
		rateChan = rateTicker.C
	}

	for i := 0; i < *concurrency; i++ {
		workerID := i
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			samples := make([]time.Duration, 0, 50000)

			for {
				select {
				case <-ctx.Done():
					workerSamples[id] = samples
					return
				default:
				}

				if rateChan != nil {
					select {
					case <-rateChan:
					case <-ctx.Done():
						workerSamples[id] = samples
						return
					}
				}

				var reqBody io.Reader
				if *bodyPayload != "" {
					reqBody = bytes.NewReader([]byte(*bodyPayload))
				}

				req, err := http.NewRequestWithContext(ctx, *method, *targetURL, reqBody)
				if err != nil {
					failedReqs.Add(1)
					continue
				}
				req.Header.Set("User-Agent", "toron-loadgen/1.0")
				req.Header.Set("Accept", "*/*")
				if *bodyPayload != "" {
					req.Header.Set("Content-Type", "application/json")
				}

				t0 := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(t0)

				totalReqs.Add(1)
				if err != nil {
					failedReqs.Add(1)
					continue
				}

				n, _ := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				successReqs.Add(1)
				totalBytes.Add(n)
				samples = append(samples, elapsed)

				statusLock.Lock()
				statusCodes[resp.StatusCode]++
				statusLock.Unlock()
			}
		}(workerID)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Aggregate and sort all latency samples
	var allSamples []time.Duration
	for _, ws := range workerSamples {
		allSamples = append(allSamples, ws...)
	}
	sort.Slice(allSamples, func(i, j int) bool {
		return allSamples[i] < allSamples[j]
	})

	var p LatencyPercentiles
	if len(allSamples) > 0 {
		var sum float64
		for _, s := range allSamples {
			sum += float64(s.Microseconds()) / 1000.0
		}
		p.Min = float64(allSamples[0].Microseconds()) / 1000.0
		p.Max = float64(allSamples[len(allSamples)-1].Microseconds()) / 1000.0
		p.Mean = sum / float64(len(allSamples))
		p.P50 = float64(allSamples[int(float64(len(allSamples))*0.50)].Microseconds()) / 1000.0
		p.P75 = float64(allSamples[int(float64(len(allSamples))*0.75)].Microseconds()) / 1000.0
		p.P90 = float64(allSamples[int(float64(len(allSamples))*0.90)].Microseconds()) / 1000.0
		p.P95 = float64(allSamples[int(float64(len(allSamples))*0.95)].Microseconds()) / 1000.0
		p.P99 = float64(allSamples[int(float64(len(allSamples))*0.99)].Microseconds()) / 1000.0
		p.P999 = float64(allSamples[int(float64(len(allSamples))*0.999)].Microseconds()) / 1000.0
	}

	actualRPS := float64(totalReqs.Load()) / totalDuration.Seconds()
	mbRead := float64(totalBytes.Load()) / (1024 * 1024)
	throughput := mbRead / totalDuration.Seconds()

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf(" Benchmark Completed in:  %.2f seconds\n", totalDuration.Seconds())
	fmt.Printf(" Total Requests Executed: %d\n", totalReqs.Load())
	fmt.Printf(" Successful Responses:    %d\n", successReqs.Load())
	fmt.Printf(" Failed / Errored Reqs:   %d\n", failedReqs.Load())
	fmt.Printf(" Requests / sec (RPS):    %.2f\n", actualRPS)
	fmt.Printf(" Transfer Throughput:     %.2f MB/s\n", throughput)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println(" Latency Distribution (ms):")
	fmt.Printf("   Min:    %8.2f ms\n", p.Min)
	fmt.Printf("   Mean:   %8.2f ms\n", p.Mean)
	fmt.Printf("   50%%:    %8.2f ms (p50 median)\n", p.P50)
	fmt.Printf("   75%%:    %8.2f ms (p75)\n", p.P75)
	fmt.Printf("   90%%:    %8.2f ms (p90)\n", p.P90)
	fmt.Printf("   95%%:    %8.2f ms (p95)\n", p.P95)
	fmt.Printf("   99%%:    %8.2f ms (p99 tail)\n", p.P99)
	fmt.Printf("   99.9%%:  %8.2f ms (p99.9 tail)\n", p.P999)
	fmt.Printf("   Max:    %8.2f ms\n", p.Max)
	fmt.Println(" HTTP Status Codes:")
	for code, count := range statusCodes {
		fmt.Printf("   [%d]: %d requests\n", code, count)
	}
	fmt.Println("================================================================================")

	result := BenchmarkResult{
		TargetURL:       *targetURL,
		Concurrency:     *concurrency,
		DurationSeconds: totalDuration.Seconds(),
		TargetRateRPS:   *targetRate,
		TotalRequests:   totalReqs.Load(),
		SuccessRequests: successReqs.Load(),
		FailedRequests:  failedReqs.Load(),
		ActualRPS:       actualRPS,
		BytesRead:       totalBytes.Load(),
		ThroughputMBs:   throughput,
		LatenciesMs:     p,
		StatusCodes:     statusCodes,
	}

	if *jsonPath != "" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err == nil {
			_ = os.WriteFile(*jsonPath, data, 0644)
			fmt.Printf(" [Artifact Saved] JSON result -> %s\n", *jsonPath)
		}
	}

	if *csvPath != "" {
		f, err := os.Create(*csvPath)
		if err == nil {
			writer := csv.NewWriter(f)
			_ = writer.Write([]string{"Concurrency", "TargetRPS", "ActualRPS", "ThroughputMBs", "p50_ms", "p90_ms", "p99_ms", "p999_ms"})
			_ = writer.Write([]string{
				fmt.Sprintf("%d", result.Concurrency),
				fmt.Sprintf("%d", result.TargetRateRPS),
				fmt.Sprintf("%.2f", result.ActualRPS),
				fmt.Sprintf("%.2f", result.ThroughputMBs),
				fmt.Sprintf("%.2f", result.LatenciesMs.P50),
				fmt.Sprintf("%.2f", result.LatenciesMs.P90),
				fmt.Sprintf("%.2f", result.LatenciesMs.P99),
				fmt.Sprintf("%.2f", result.LatenciesMs.P999),
			})
			writer.Flush()
			f.Close()
			fmt.Printf(" [Artifact Saved] CSV result  -> %s\n", *csvPath)
		}
	}
}
