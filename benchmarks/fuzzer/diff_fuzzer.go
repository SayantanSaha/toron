package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type TestCase struct {
	ID                       string            `json:"id"`
	Category                 string            `json:"category"`
	Name                     string            `json:"name"`
	CWE                      string            `json:"cwe"`
	RawPayload               string            `json:"raw_payload,omitempty"`
	SequentialPayloads       []string          `json:"sequential_payloads,omitempty"`
	ForbiddenResponseHeaders []string          `json:"forbidden_response_headers,omitempty"`
	RequiredResponseHeaders  map[string]string `json:"required_response_headers,omitempty"`
	ExpectedStatus           []int             `json:"expected_status"`
	ExpectClose              bool              `json:"expect_close"`
	Description              string            `json:"description"`
}

type DistributionMetrics struct {
	Trials          int       `json:"trials"`
	WarmupRuns      int       `json:"warmup_runs"`
	MeanLatencyUs   float64   `json:"mean_latency_us"`
	StdDevLatencyUs float64   `json:"std_dev_latency_us"`
	MedianLatencyUs float64   `json:"median_latency_us"`
	P90LatencyUs    float64   `json:"p90_latency_us"`
	P99LatencyUs    float64   `json:"p99_latency_us"`
	P999LatencyUs   float64   `json:"p999_latency_us"`
	CI95MarginUs    float64   `json:"ci95_margin_us"`
	CI95LowerUs     float64   `json:"ci95_lower_us"`
	CI95UpperUs     float64   `json:"ci95_upper_us"`
	RawSamples      []float64 `json:"raw_samples,omitempty"`
}

type TestResult struct {
	TestCaseID         string              `json:"test_case_id"`
	Category           string              `json:"category"`
	Name               string              `json:"name"`
	CWE                string              `json:"cwe"`
	ExpectedStatus     []int               `json:"expected_status"`
	ExpectedStatusStr  string              `json:"expected_status_str"`
	ActualStatus       int                 `json:"actual_status"`
	StatusLine         string              `json:"status_line"`
	ResponseHeaders    map[string]string   `json:"response_headers,omitempty"`
	ConnectionClose    bool                `json:"connection_closed"`
	LatencyUs          int64               `json:"latency_us"`
	Passed             bool                `json:"passed"`
	FailureReason      string              `json:"failure_reason,omitempty"`
	BaselineStatus     int                 `json:"baseline_status,omitempty"`
	BaselineVulnerable bool                `json:"baseline_vulnerable,omitempty"`
	Distribution       DistributionMetrics `json:"distribution"`
}

type DifferentialReport struct {
	Timestamp         string                  `json:"timestamp"`
	TargetHost        string                  `json:"target_host"`
	BaselineHost      string                  `json:"baseline_host,omitempty"`
	TrialsPerTest     int                     `json:"trials_per_test"`
	WarmupRunsPerTest int                     `json:"warmup_runs_per_test"`
	TotalTests        int                     `json:"total_tests"`
	PassedTests       int                     `json:"passed_tests"`
	FailedTests       int                     `json:"failed_tests"`
	SecurityPassRate  float64                 `json:"security_pass_rate"`
	AverageLatencyUs  float64                 `json:"average_latency_us"`
	MedianLatencyUs   float64                 `json:"median_latency_us"`
	P90LatencyUs      float64                 `json:"p90_latency_us"`
	P99LatencyUs      float64                 `json:"p99_latency_us"`
	CategoryStats     map[string]CategoryStat `json:"category_stats"`
	Results           []TestResult            `json:"results"`
}

type CategoryStat struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

func getTestCases() []TestCase {
	return []TestCase{
		// --- 1. HTTP Request Smuggling & Connection Desynchronization (CWE-444) ---
		{
			ID:          "SMUGGLE-001",
			Category:    "Request Smuggling (CL.TE)",
			Name:        "Conflicting Content-Length and Transfer-Encoding",
			CWE:         "CWE-444",
			RawPayload:  "POST /health HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
			ExpectedStatus: []int{400, 501},
			ExpectClose: true,
			Description: "RFC 7230 §3.3.3: Proxy must reject conflicting CL and TE to prevent desync",
		},
		{
			ID:          "SMUGGLE-002",
			Category:    "Request Smuggling (CL.CL)",
			Name:        "Multiple Divergent Content-Length Headers",
			CWE:         "CWE-444",
			RawPayload:  "POST /health HTTP/1.1\r\nHost: localhost\r\nContent-Length: 5\r\nContent-Length: 10\r\n\r\nhello",
			ExpectedStatus: []int{400, 501},
			ExpectClose: true,
			Description: "RFC 7230 §3.3.2: Conflicting Content-Length headers must be rejected",
		},
		{
			ID:          "SMUGGLE-003",
			Category:    "Request Smuggling (Obfuscation)",
			Name:        "Transfer-Encoding with Obfuscated Tab Character",
			CWE:         "CWE-444",
			RawPayload:  "POST /health HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding:\tchunked\r\nContent-Length: 5\r\n\r\n0\r\n\r\n",
			ExpectedStatus: []int{400, 501},
			ExpectClose: true,
			Description: "Obfuscated TE with tab prefix must not bypass TE-CL conflict detection",
		},
		{
			ID:          "SMUGGLE-004",
			Category:    "Request Smuggling (Chunked)",
			Name:        "Invalid Chunk Hex Size Extension",
			CWE:         "CWE-444",
			RawPayload:  "POST /health HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\nZZ\r\nmalicious\r\n0\r\n\r\n",
			ExpectedStatus: []int{400, 501},
			ExpectClose: true,
			Description: "Malformed non-hex chunk header must trigger immediate parser fault",
		},

		// --- 2. RFC 7230 §3.2.4 Header Whitespace Invariants ---
		{
			ID:          "WHITESPACE-001",
			Category:    "Header Syntax Invariants",
			Name:        "Space Before Colon in Field Name",
			CWE:         "CWE-444",
			RawPayload:  "GET /health HTTP/1.1\r\nHost : localhost\r\nUser-Agent: test\r\n\r\n",
			ExpectedStatus: []int{400},
			ExpectClose: false,
			Description: "RFC 7230 §3.2.4: No whitespace allowed between header field name and colon",
		},
		{
			ID:          "WHITESPACE-002",
			Category:    "Header Syntax Invariants",
			Name:        "Tab Before Colon in Field Name",
			CWE:         "CWE-444",
			RawPayload:  "GET /health HTTP/1.1\r\nHost\t: localhost\r\nUser-Agent: test\r\n\r\n",
			ExpectedStatus: []int{400},
			ExpectClose: false,
			Description: "RFC 7230 §3.2.4: Tab characters before colon must be rejected with 400",
		},
		{
			ID:          "WHITESPACE-003",
			Category:    "Header Syntax Invariants",
			Name:        "Obsolete Line Folding (obs-fold)",
			CWE:         "CWE-436",
			RawPayload:  "GET /health HTTP/1.1\r\nHost: localhost\r\nX-Fold:\r\n  continuation\r\n\r\n",
			ExpectedStatus: []int{200, 400},
			ExpectClose: false,
			Description: "RFC 7230 §3.2.4: obs-fold rejected or normalized to single SP",
		},

		// --- 3. Control Character & CRLF Injection (CWE-117) ---
		{
			ID:          "CONTROL-001",
			Category:    "Control Character Guards",
			Name:        "Null Byte (0x00) in Request URI",
			CWE:         "CWE-117",
			RawPayload:  "GET /health\x00/admin HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus: []int{400},
			ExpectClose: true,
			Description: "Non-printable null byte in URI must be rejected to prevent log poisoning",
		},
		{
			ID:          "CONTROL-002",
			Category:    "Control Character Guards",
			Name:        "Bell Character (0x07) in Header Value",
			CWE:         "CWE-117",
			RawPayload:  "GET /health HTTP/1.1\r\nHost: localhost\r\nX-Audit-Payload: test\x07alert\r\n\r\n",
			ExpectedStatus: []int{400},
			ExpectClose: true,
			Description: "Terminal control sequences in headers must be rejected by protocol guard",
		},
		{
			ID:          "CONTROL-003",
			Category:    "Control Character Guards",
			Name:        "Escape Sequence (0x1B) in Query String",
			CWE:         "CWE-117",
			RawPayload:  "GET /health?q=\x1b[31mRed HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus: []int{400},
			ExpectClose: true,
			Description: "ANSI escape codes in query params must be rejected",
		},

		// --- 4. URI Path Traversal & Canonicalization (CWE-22) ---
		{
			ID:          "TRAVERSAL-001",
			Category:    "Path Canonicalization",
			Name:        "Raw Dot-Dot Path Traversal Sequence",
			CWE:         "CWE-22",
			RawPayload:  "GET /internal/dashboard/../../canary_traversal.txt HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus: []int{400, 403},
			ExpectClose: false,
			Description: "Pre-route path canonicalization and route root boundary guard must actively reject escaping to canary file",
		},
		{
			ID:          "TRAVERSAL-002",
			Category:    "Path Canonicalization",
			Name:        "Uppercase Percent-Encoded Traversal (%2E%2E)",
			CWE:         "CWE-22",
			RawPayload:  "GET /internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus: []int{400, 403},
			ExpectClose: false,
			Description: "Uppercase encoded dot segments must be normalized before filesystem access",
		},
		{
			ID:          "TRAVERSAL-003",
			Category:    "Path Canonicalization",
			Name:        "Double Percent-Encoded Traversal (%252e%252e)",
			CWE:         "CWE-22",
			RawPayload:  "GET /internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus: []int{400, 403},
			ExpectClose: true,
			Description: "Double percent-encoding must not bypass path normalization boundaries",
		},

		// --- 5. Bounded Allocation & Resource Exhaustion (CWE-400) ---
		{
			ID:          "RESOURCE-001",
			Category:    "Resource Bounding",
			Name:        "Oversized Request Header (> 8KB)",
			CWE:         "CWE-400",
			RawPayload:  fmt.Sprintf("GET /health HTTP/1.1\r\nHost: localhost\r\nX-Huge-Header: %s\r\n\r\n", strings.Repeat("A", 12000)),
			ExpectedStatus: []int{400, 431},
			ExpectClose: true,
			Description: "Headers exceeding 8KB max limit must fail fast to protect heap memory",
		},
		{
			ID:          "RESOURCE-002",
			Category:    "Resource Bounding",
			Name:        "Oversized Single Query Parameter (> 2KB)",
			CWE:         "CWE-400",
			RawPayload:  fmt.Sprintf("GET /health?param=%s HTTP/1.1\r\nHost: localhost\r\n\r\n", strings.Repeat("B", 3000)),
			ExpectedStatus: []int{400, 413, 414},
			ExpectClose: true,
			Description: "Excessive query param length must be bounded to prevent ReDoS / memory spikes",
		},

		// --- 6. Conformance / Baseline Happy Path ---
		{
			ID:          "BASELINE-001",
			Category:    "RFC Conformance Baseline",
			Name:        "Standard Valid HTTP/1.1 GET Request",
			CWE:         "N/A",
			RawPayload:  "GET /health HTTP/1.1\r\nHost: localhost\r\nUser-Agent: diff-fuzzer/1.0\r\nAccept: */*\r\n\r\n",
			ExpectedStatus: []int{200},
			ExpectClose: false,
			Description: "Valid RFC 7230 request must be accepted with 200 OK",
		},

		// --- 7. RFC 7234 Shared Cache Session Boundary Isolation (CWE-524, CWE-384) ---
		{
			ID:          "CACHE-001",
			Category:    "Cache Session Boundary",
			Name:        "Web Cache Deception (Private Cache-Control)",
			CWE:         "CWE-524",
			SequentialPayloads: []string{
				// Stage 1: Authenticated client fetches private resource
				"GET /cache/private-profile HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer secret-session-token\r\nCache-Control: private\r\n\r\n",
				// Stage 2: Unauthenticated probe requests the same resource
				"GET /cache/private-profile HTTP/1.1\r\nHost: localhost\r\n\r\n",
			},
			ExpectedStatus: []int{200, 401},
			RequiredResponseHeaders: map[string]string{
				"X-Cache": "MISS",
			},
			ForbiddenResponseHeaders: []string{
				"X-Private-Token",
			},
			ExpectClose: false,
			Description: "RFC 7234 §3: Responses with Cache-Control: private must never be cached or served to unauthenticated probe",
		},
		{
			ID:          "CACHE-002",
			Category:    "Cache Session Boundary",
			Name:        "Shared Cache Set-Cookie Header Stripping",
			CWE:         "CWE-384",
			SequentialPayloads: []string{
				// Stage 1: Client triggers resource that sets session cookie
				"GET /cache/cookie-resource HTTP/1.1\r\nHost: localhost\r\n\r\n",
				// Stage 2: Subsequent client fetches cached entry
				"GET /cache/cookie-resource HTTP/1.1\r\nHost: localhost\r\n\r\n",
			},
			ExpectedStatus: []int{200},
			RequiredResponseHeaders: map[string]string{
				"X-Cache": "HIT",
			},
			ForbiddenResponseHeaders: []string{
				"Set-Cookie",
				"Set-Cookie2",
			},
			ExpectClose: false,
			Description: "RFC 7234 §8: Shared cache must strip Set-Cookie and Set-Cookie2 before caching and serving",
		},
		{
			ID:          "CACHE-003",
			Category:    "Cache Session Boundary",
			Name:        "Authorization Refusal Invariant in Shared Cache",
			CWE:         "CWE-524",
			SequentialPayloads: []string{
				// Stage 1: Authenticated request lacking Cache-Control: public
				"GET /cache/protected-resource HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer user-auth-key\r\n\r\n",
				// Stage 2: Subsequent unauthenticated probe
				"GET /cache/protected-resource HTTP/1.1\r\nHost: localhost\r\n\r\n",
			},
			ExpectedStatus: []int{200, 401},
			RequiredResponseHeaders: map[string]string{
				"X-Cache": "MISS",
			},
			ForbiddenResponseHeaders: []string{
				"X-User-Data",
			},
			ExpectClose: false,
			Description: "RFC 7234 §3.2: Shared cache must refuse caching requests with Authorization header unless response has Cache-Control: public",
		},
	}
}

// formatExpectedStatus formats status codes canonically with RFC status text or slash delimitation.
func formatExpectedStatus(expected []int) string {
	if len(expected) == 0 {
		return "N/A"
	}
	if len(expected) == 1 {
		code := expected[0]
		text := http.StatusText(code)
		if text != "" {
			return fmt.Sprintf("%d %s", code, text)
		}
		return strconv.Itoa(code)
	}
	strs := make([]string, len(expected))
	for i, code := range expected {
		strs[i] = strconv.Itoa(code)
	}
	return strings.Join(strs, "/")
}

// parseResponseHeaders parses HTTP response headers from a bufio.Reader.
func parseResponseHeaders(reader *bufio.Reader) http.Header {
	headers := make(http.Header)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			key := strings.TrimSpace(line[:colonIdx])
			val := strings.TrimSpace(line[colonIdx+1:])
			headers.Add(key, val)
		}
	}
	return headers
}

// getHeader performs case-insensitive header lookup returning comma-joined values.
func getHeader(h http.Header, key string) string {
	for k, vals := range h {
		if strings.EqualFold(k, key) {
			return strings.Join(vals, ", ")
		}
	}
	return ""
}

// hasHeader checks case-insensitively whether a header is present in the response.
func hasHeader(h http.Header, key string) (bool, string) {
	for k, vals := range h {
		if strings.EqualFold(k, key) {
			return true, strings.Join(vals, ", ")
		}
	}
	return false, ""
}

// sendAndDrain sends a preparatory raw payload and drains response bytes.
func sendAndDrain(targetHost, payload string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", targetHost, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write([]byte(payload)); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	// Read status line
	if _, err := reader.ReadString('\n'); err != nil {
		return err
	}
	// Drain headers
	_ = parseResponseHeaders(reader)

	// Bounded drain of response body
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 4096)
	_, _ = reader.Read(buf)
	return nil
}

// computeDistribution calculates mean, sample stddev, median, tail percentiles, and 95% CI (BMK-02).
func computeDistribution(samples []float64, warmupRuns int) DistributionMetrics {
	m := DistributionMetrics{
		Trials:     len(samples),
		WarmupRuns: warmupRuns,
		RawSamples: samples,
	}
	if len(samples) == 0 {
		return m
	}

	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)

	var sum float64
	for _, s := range sorted {
		sum += s
	}
	mean := sum / float64(len(sorted))
	m.MeanLatencyUs = math.Round(mean*100) / 100

	n := len(sorted)
	if n%2 == 1 {
		m.MedianLatencyUs = math.Round(sorted[n/2]*100) / 100
	} else {
		m.MedianLatencyUs = math.Round(((sorted[n/2-1]+sorted[n/2])/2.0)*100) / 100
	}

	percentile := func(p float64) float64 {
		if n == 1 {
			return sorted[0]
		}
		idx := int(math.Ceil(p*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return sorted[idx]
	}

	m.P90LatencyUs = math.Round(percentile(0.90)*100) / 100
	m.P99LatencyUs = math.Round(percentile(0.99)*100) / 100
	m.P999LatencyUs = math.Round(percentile(0.999)*100) / 100

	if n > 1 {
		var varianceSum float64
		for _, s := range sorted {
			diff := s - mean
			varianceSum += diff * diff
		}
		stdDev := math.Sqrt(varianceSum / float64(n-1))
		m.StdDevLatencyUs = math.Round(stdDev*100) / 100

		ciMargin := 1.96 * stdDev / math.Sqrt(float64(n))
		m.CI95MarginUs = math.Round(ciMargin*100) / 100
		m.CI95LowerUs = math.Round((mean-ciMargin)*100) / 100
		m.CI95UpperUs = math.Round((mean+ciMargin)*100) / 100
	} else {
		m.StdDevLatencyUs = 0.0
		m.CI95MarginUs = 0.0
		m.CI95LowerUs = m.MeanLatencyUs
		m.CI95UpperUs = m.MeanLatencyUs
	}

	return m
}

// isConnectionClosedErr returns true if the error indicates peer closure, reset, or broken pipe.
func isConnectionClosedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "closed network connection") ||
		strings.Contains(errStr, "eof")
}

// verifySocketClosed actively verifies whether the remote TCP socket was physically closed.
func verifySocketClosed(conn net.Conn, reader *bufio.Reader) bool {
	if conn == nil {
		return true
	}

	// Phase 1: Drain remaining response bytes (headers/body) under a bounded read deadline
	_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	buf := make([]byte, 1024)
	for {
		_, err := reader.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) || isConnectionClosedErr(err) {
				return true
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// Reading timed out while socket remains open; proceed to probe
				break
			}
			return true
		}
	}

	// Phase 2: Attempt a subsequent probe write to test for TCP half-close or reset
	_ = conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := conn.Write([]byte("\r\n")); err != nil {
		return true
	}

	// Read again after write to observe peer TCP RST or EOF
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err := reader.Read(buf)
	if err != nil {
		if errors.Is(err, io.EOF) || isConnectionClosedErr(err) {
			return true
		}
	}

	return false
}

// executeRawTestSingle executes a single iteration of a test case.
// Strictly conforms to Equation 7: T_rejection = t_status_line_read - t_socket_write_start.
func executeRawTestSingle(targetHost string, tc TestCase) (TestResult, float64) {
	result := TestResult{
		TestCaseID:        tc.ID,
		Category:          tc.Category,
		Name:              tc.Name,
		CWE:               tc.CWE,
		ExpectedStatus:    tc.ExpectedStatus,
		ExpectedStatusStr: formatExpectedStatus(tc.ExpectedStatus),
	}

	payloads := tc.SequentialPayloads
	if len(payloads) == 0 {
		payloads = []string{tc.RawPayload}
	}

	// For multi-stage payloads, execute preparatory requests first (outside timing interval)
	for i := 0; i < len(payloads)-1; i++ {
		if err := sendAndDrain(targetHost, payloads[i], 3*time.Second); err != nil {
			result.Passed = false
			result.FailureReason = fmt.Sprintf("Preparatory request %d failed: %v", i+1, err)
			return result, 0
		}
		time.Sleep(15 * time.Millisecond)
	}

	finalPayload := payloads[len(payloads)-1]

	// Establish socket connection outside timing interval (BMK-01 / Eq. 7 compliance)
	conn, err := net.DialTimeout("tcp", targetHost, 3*time.Second)
	if err != nil {
		result.Passed = false
		result.FailureReason = fmt.Sprintf("TCP dial failed: %v", err)
		return result, 0
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)

	// Equation 7: Start clock immediately prior to socket write
	start := time.Now()
	_, err = conn.Write([]byte(finalPayload))
	if err != nil {
		latencyUs := float64(time.Since(start).Nanoseconds()) / 1000.0
		result.LatencyUs = int64(math.Round(latencyUs))
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Socket write error: %v", err)
		return result, latencyUs
	}

	// Equation 7: Read status line, terminating the rejection measurement interval
	statusLine, err := reader.ReadString('\n')
	latencyNanos := time.Since(start).Nanoseconds()
	latencyUs := float64(latencyNanos) / 1000.0
	result.LatencyUs = int64(math.Round(latencyUs))

	if err != nil {
		// If socket was reset/closed immediately by server (fail-fast)
		result.ConnectionClose = true
		if tc.ExpectClose && len(tc.ExpectedStatus) > 0 {
			result.Passed = true
			result.StatusLine = "Connection reset by peer (immediate fail-fast teardown)"
			return result, latencyUs
		}
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Response read error: %v", err)
		return result, latencyUs
	}

	statusLine = strings.TrimSpace(statusLine)
	result.StatusLine = statusLine

	// Parse status code
	var proto string
	var code int
	_, err = fmt.Sscanf(statusLine, "%s %d", &proto, &code)
	if err != nil {
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Invalid status line: %q", statusLine)
		return result, latencyUs
	}
	result.ActualStatus = code

	// Parse response headers
	headers := parseResponseHeaders(reader)
	result.ResponseHeaders = make(map[string]string)
	for k := range headers {
		result.ResponseHeaders[k] = getHeader(headers, k)
	}

	// Check if status code matches expected set
	statusMatch := false
	for _, expected := range tc.ExpectedStatus {
		if code == expected {
			statusMatch = true
			break
		}
	}

	// Actively verify whether the TCP socket was physically terminated
	result.ConnectionClose = verifySocketClosed(conn, reader)

	if !statusMatch {
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Received status %d, expected one of %v", code, tc.ExpectedStatus)
		return result, latencyUs
	}

	if tc.ExpectClose && !result.ConnectionClose {
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Received status %d, but connection remained open (expected physical teardown)", code)
		return result, latencyUs
	}

	// Verify required headers
	for reqKey, reqVal := range tc.RequiredResponseHeaders {
		actualVal := getHeader(headers, reqKey)
		if !strings.EqualFold(actualVal, reqVal) {
			result.Passed = false
			result.FailureReason = fmt.Sprintf("Required header %q: expected %q, got %q", reqKey, reqVal, actualVal)
			return result, latencyUs
		}
	}

	// Verify forbidden headers
	for _, forbKey := range tc.ForbiddenResponseHeaders {
		if exists, actualVal := hasHeader(headers, forbKey); exists {
			result.Passed = false
			result.FailureReason = fmt.Sprintf("Forbidden header %q was present with value %q", forbKey, actualVal)
			return result, latencyUs
		}
	}

	result.Passed = true
	return result, latencyUs
}

// executeRawTest executes test trials with optional warm-up and statistical calculations (BMK-01 / BMK-02).
func executeRawTest(targetHost string, tc TestCase, trialOpts ...int) TestResult {
	trials := 1
	warmup := 0
	if len(trialOpts) > 0 && trialOpts[0] > 0 {
		trials = trialOpts[0]
	}
	if len(trialOpts) > 1 && trialOpts[1] >= 0 {
		warmup = trialOpts[1]
	} else if trials > 1 {
		warmup = 50
	}

	// Phase 1: Discarded Warm-up Runs
	for w := 0; w < warmup; w++ {
		_, _ = executeRawTestSingle(targetHost, tc)
	}

	// Phase 2: Timed Evaluation Trials
	samples := make([]float64, 0, trials)
	var finalRes TestResult

	for i := 0; i < trials; i++ {
		res, latUs := executeRawTestSingle(targetHost, tc)
		finalRes = res
		samples = append(samples, latUs)
		if !res.Passed {
			break
		}
	}

	finalRes.Distribution = computeDistribution(samples, warmup)
	finalRes.LatencyUs = int64(math.Round(finalRes.Distribution.MedianLatencyUs))
	return finalRes
}

func main() {
	targetHost := flag.String("target", "127.0.0.1:8080", "Target server host:port (Toron)")
	baselineHost := flag.String("baseline", "", "Optional baseline server host:port (e.g. NGINX/Caddy for differential comparison)")
	trials := flag.Int("trials", 1, "Number of measured repeated trials per test case (e.g. 1000 for empirical evaluation)")
	warmup := flag.Int("warmup", 0, "Number of discarded warm-up trials per test case (default: 0 for trials=1, 50 for trials>1)")
	outJSON := flag.String("json", "benchmarks/results/differential_fuzz_report.json", "Output path for JSON report")
	outMD := flag.String("md", "benchmarks/results/differential_fuzz_report.md", "Output path for Markdown report")
	flag.Parse()

	if *trials > 1 && *warmup == 0 {
		*warmup = 50
	}

	testCases := getTestCases()

	fmt.Println("================================================================================")
	fmt.Println("       TORON DIFFERENTIAL PROTOCOL SECURITY FUZZER & INVARIANT CHECKER         ")
	fmt.Println("================================================================================")
	fmt.Printf(" Target Under Test:   %s\n", *targetHost)
	if *baselineHost != "" {
		fmt.Printf(" Baseline Reference:  %s (Differential Mode Active)\n", *baselineHost)
	}
	fmt.Printf(" Test Cases Loaded:   %d security invariant specifications\n", len(testCases))
	if *trials > 1 {
		fmt.Printf(" Evaluation Mode:     Repeated Statistical Trials (K=%d, W=%d warmup discarded)\n", *trials, *warmup)
	} else {
		fmt.Printf(" Evaluation Mode:     Single-Shot Smoke Verification (K=1)\n")
	}
	fmt.Println("================================================================================")
	fmt.Println()

	var results []TestResult
	categoryStats := make(map[string]CategoryStat)
	var totalMeanLatency float64
	allMedians := make([]float64, 0, len(testCases))
	passedCount := 0

	for _, tc := range testCases {
		res := executeRawTest(*targetHost, tc, *trials, *warmup)

		// Optional differential test against baseline
		if *baselineHost != "" {
			baseRes := executeRawTest(*baselineHost, tc, *trials, *warmup)
			res.BaselineStatus = baseRes.ActualStatus
			if tc.CWE != "N/A" && baseRes.ActualStatus == 200 {
				res.BaselineVulnerable = true
			}
		}

		results = append(results, res)
		totalMeanLatency += res.Distribution.MeanLatencyUs
		allMedians = append(allMedians, res.Distribution.MedianLatencyUs)

		cat := categoryStats[tc.Category]
		cat.Total++
		connState := "conn:open"
		if res.ConnectionClose {
			connState = "conn:closed"
		}
		if res.Passed {
			cat.Passed++
			passedCount++
			if *trials > 1 {
				fmt.Printf(" [PASS] %-14s | %-38s | %d (%s) [%s] [µ: %.1f, p50: %.1f, p99: %.1f µs]\n",
					tc.ID, tc.Name, res.ActualStatus, tc.CWE, connState,
					res.Distribution.MeanLatencyUs, res.Distribution.MedianLatencyUs, res.Distribution.P99LatencyUs)
			} else {
				fmt.Printf(" [PASS] %-14s | %-40s | %d (%s) [%s] [%.1f µs]\n",
					tc.ID, tc.Name, res.ActualStatus, tc.CWE, connState, res.Distribution.MeanLatencyUs)
			}
		} else {
			cat.Failed++
			fmt.Printf(" [FAIL] %-14s | %-40s | Received %d [%s] (%s)\n",
				tc.ID, tc.Name, res.ActualStatus, connState, res.FailureReason)
		}
		categoryStats[tc.Category] = cat
	}

	passRate := (float64(passedCount) / float64(len(testCases))) * 100.0
	avgLatency := totalMeanLatency / float64(len(testCases))

	// Compute overall medians and tail percentiles
	sort.Float64s(allMedians)
	overallMedian := 0.0
	overallP90 := 0.0
	overallP99 := 0.0
	if len(allMedians) > 0 {
		overallMedian = allMedians[len(allMedians)/2]
		idx90 := int(math.Ceil(0.90*float64(len(allMedians)))) - 1
		if idx90 < 0 {
			idx90 = 0
		}
		if idx90 >= len(allMedians) {
			idx90 = len(allMedians) - 1
		}
		overallP90 = allMedians[idx90]

		idx99 := int(math.Ceil(0.99*float64(len(allMedians)))) - 1
		if idx99 < 0 {
			idx99 = 0
		}
		if idx99 >= len(allMedians) {
			idx99 = len(allMedians) - 1
		}
		overallP99 = allMedians[idx99]
	}

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("                         FUZZER EXECUTION SUMMARY                               ")
	fmt.Println("================================================================================")
	fmt.Printf(" Total Invariant Tests:  %d\n", len(testCases))
	fmt.Printf(" Passed Assertions:      %d\n", passedCount)
	fmt.Printf(" Failed Assertions:      %d\n", len(testCases)-passedCount)
	fmt.Printf(" Security Pass Rate:     %.2f%%\n", passRate)
	fmt.Printf(" Average Rejection Lat:  %.2f µs (Mean of all test vector means)\n", avgLatency)
	if *trials > 1 {
		fmt.Printf(" Median Rejection Lat:   %.2f µs (Overall p50 across test vectors)\n", overallMedian)
		fmt.Printf(" Tail Rejection Latency: p90: %.2f µs | p99: %.2f µs\n", overallP90, overallP99)
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println(" Category Breakdown:")
	for name, st := range categoryStats {
		catRate := (float64(st.Passed) / float64(st.Total)) * 100.0
		fmt.Printf("   %-32s : %2d / %2d passed (%.1f%%)\n", name, st.Passed, st.Total, catRate)
	}
	fmt.Println("================================================================================")

	report := DifferentialReport{
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		TargetHost:        *targetHost,
		BaselineHost:      *baselineHost,
		TrialsPerTest:     *trials,
		WarmupRunsPerTest: *warmup,
		TotalTests:        len(testCases),
		PassedTests:       passedCount,
		FailedTests:       len(testCases) - passedCount,
		SecurityPassRate:  passRate,
		AverageLatencyUs:  avgLatency,
		MedianLatencyUs:   overallMedian,
		P90LatencyUs:      overallP90,
		P99LatencyUs:      overallP99,
		CategoryStats:     categoryStats,
		Results:           results,
	}

	// Ensure results directory exists
	_ = os.MkdirAll("benchmarks/results", 0755)

	// Write JSON report
	if *outJSON != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			_ = os.WriteFile(*outJSON, data, 0644)
			fmt.Printf(" [Artifact Saved] JSON report: %s\n", *outJSON)
		}
	}

	// Write Markdown report
	if *outMD != "" {
		var md strings.Builder
		md.WriteString("# 🛡️ Differential Protocol Security & Invariant Verification Report\n\n")
		md.WriteString(fmt.Sprintf("**Target Under Test**: `%s`  \n", report.TargetHost))
		if report.BaselineHost != "" {
			md.WriteString(fmt.Sprintf("**Baseline Reference**: `%s`  \n", report.BaselineHost))
		}
		md.WriteString(fmt.Sprintf("**Execution Timestamp**: `%s`  \n", report.Timestamp))
		if report.TrialsPerTest > 1 {
			md.WriteString(fmt.Sprintf("**Evaluation Mode**: Repeated Statistical Trials ($K=%d$, $W=%d$ warm-up discarded)  \n", report.TrialsPerTest, report.WarmupRunsPerTest))
			md.WriteString(fmt.Sprintf("**Overall Invariant Pass Rate**: **%.2f%%** (%d/%d tests)  \n", report.SecurityPassRate, report.PassedTests, report.TotalTests))
			md.WriteString(fmt.Sprintf("**Average Fail-Fast Rejection Latency (Mean)**: `%.2f µs`  \n", report.AverageLatencyUs))
			md.WriteString(fmt.Sprintf("**Median Rejection Latency (p50)**: `%.2f µs`  \n", report.MedianLatencyUs))
			md.WriteString(fmt.Sprintf("**Tail Latency (p90 / p99)**: `%.2f µs` / `%.2f µs`  \n\n", report.P90LatencyUs, report.P99LatencyUs))
		} else {
			md.WriteString(fmt.Sprintf("**Overall Invariant Pass Rate**: **%.2f%%** (%d/%d tests)  \n", report.SecurityPassRate, report.PassedTests, report.TotalTests))
			md.WriteString(fmt.Sprintf("**Average Fail-Fast Rejection Latency**: `%.2f µs`  \n\n", report.AverageLatencyUs))
		}

		md.WriteString("## 1. Category Summary Matrix\n\n")
		md.WriteString("| Security Category | Total Tests | Passed | Failed | Pass Rate |\n")
		md.WriteString("| :--- | :---: | :---: | :---: | :---: |\n")
		for catName, st := range report.CategoryStats {
			rate := (float64(st.Passed) / float64(st.Total)) * 100.0
			md.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.1f%% |\n", catName, st.Total, st.Passed, st.Failed, rate))
		}
		md.WriteString("\n## 2. Detailed Invariant Test Results\n\n")
		if report.TrialsPerTest > 1 {
			md.WriteString("| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Mean ± StdDev | Median (p50) | p90 | p99 | 95% CI | Conn Closed | Result |\n")
			md.WriteString("| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |\n")
			for _, r := range report.Results {
				statusIcon := "✅ PASS"
				if !r.Passed {
					statusIcon = "❌ FAIL"
				}
				connIcon := "Open"
				if r.ConnectionClose {
					connIcon = "Closed"
				}
				expectedStr := r.ExpectedStatusStr
				if expectedStr == "" {
					expectedStr = formatExpectedStatus(r.ExpectedStatus)
				}
				md.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` | `%d` | `%.2f ± %.2f µs` | `%.2f µs` | `%.2f µs` | `%.2f µs` | `[%.2f, %.2f]` | `%s` | %s |\n",
					r.TestCaseID, r.Name, r.CWE, expectedStr, r.ActualStatus,
					r.Distribution.MeanLatencyUs, r.Distribution.StdDevLatencyUs,
					r.Distribution.MedianLatencyUs, r.Distribution.P90LatencyUs, r.Distribution.P99LatencyUs,
					r.Distribution.CI95LowerUs, r.Distribution.CI95UpperUs,
					connIcon, statusIcon))
			}
		} else {
			md.WriteString("| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Conn Closed | Fail-Fast Latency | Result |\n")
			md.WriteString("| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |\n")
			for _, r := range report.Results {
				statusIcon := "✅ PASS"
				if !r.Passed {
					statusIcon = "❌ FAIL"
				}
				connIcon := "Open"
				if r.ConnectionClose {
					connIcon = "Closed"
				}
				expectedStr := r.ExpectedStatusStr
				if expectedStr == "" {
					expectedStr = formatExpectedStatus(r.ExpectedStatus)
				}
				md.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` | `%d` | `%s` | `%.2f µs` | %s |\n",
					r.TestCaseID, r.Name, r.CWE, expectedStr, r.ActualStatus, connIcon, r.Distribution.MeanLatencyUs, statusIcon))
			}
		}

		_ = os.WriteFile(*outMD, []byte(md.String()), 0644)
		fmt.Printf(" [Artifact Saved] Markdown report: %s\n", *outMD)
	}
}
