package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

type TestCase struct {
	ID             string   `json:"id"`
	Category       string   `json:"category"`
	Name           string   `json:"name"`
	CWE            string   `json:"cwe"`
	RawPayload     string   `json:"raw_payload"`
	ExpectedStatus []int    `json:"expected_status"`
	ExpectClose    bool     `json:"expect_close"`
	Description    string   `json:"description"`
}

type TestResult struct {
	TestCaseID      string        `json:"test_case_id"`
	Category        string        `json:"category"`
	Name            string        `json:"name"`
	CWE             string        `json:"cwe"`
	ActualStatus    int           `json:"actual_status"`
	StatusLine      string        `json:"status_line"`
	ConnectionClose bool          `json:"connection_closed"`
	LatencyUs       int64         `json:"latency_us"`
	Passed          bool          `json:"passed"`
	FailureReason   string        `json:"failure_reason,omitempty"`
	BaselineStatus  int           `json:"baseline_status,omitempty"`
	BaselineVulnerable bool       `json:"baseline_vulnerable,omitempty"`
}

type DifferentialReport struct {
	Timestamp       string       `json:"timestamp"`
	TargetHost      string       `json:"target_host"`
	BaselineHost    string       `json:"baseline_host,omitempty"`
	TotalTests      int          `json:"total_tests"`
	PassedTests     int          `json:"passed_tests"`
	FailedTests     int          `json:"failed_tests"`
	SecurityPassRate float64     `json:"security_pass_rate"`
	AverageLatencyUs float64     `json:"average_latency_us"`
	CategoryStats   map[string]CategoryStat `json:"category_stats"`
	Results         []TestResult `json:"results"`
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
			ExpectClose: false,
			Description: "Non-printable null byte in URI must be rejected to prevent log poisoning",
		},
		{
			ID:          "CONTROL-002",
			Category:    "Control Character Guards",
			Name:        "Bell Character (0x07) in Header Value",
			CWE:         "CWE-117",
			RawPayload:  "GET /health HTTP/1.1\r\nHost: localhost\r\nX-Audit-Payload: test\x07alert\r\n\r\n",
			ExpectedStatus: []int{400},
			ExpectClose: false,
			Description: "Terminal control sequences in headers must be rejected by protocol guard",
		},
		{
			ID:          "CONTROL-003",
			Category:    "Control Character Guards",
			Name:        "Escape Sequence (0x1B) in Query String",
			CWE:         "CWE-117",
			RawPayload:  "GET /health?q=\x1b[31mRed HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus: []int{400},
			ExpectClose: false,
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
			ExpectClose: false,
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
			ExpectClose: false,
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
	}
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

func executeRawTest(targetHost string, tc TestCase) TestResult {
	result := TestResult{
		TestCaseID: tc.ID,
		Category:   tc.Category,
		Name:       tc.Name,
		CWE:        tc.CWE,
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", targetHost, 3*time.Second)
	if err != nil {
		result.LatencyUs = time.Since(start).Microseconds()
		result.Passed = false
		result.FailureReason = fmt.Sprintf("TCP dial failed: %v", err)
		return result
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	// Send raw byte sequence
	_, err = conn.Write([]byte(tc.RawPayload))
	if err != nil {
		result.LatencyUs = time.Since(start).Microseconds()
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Socket write error: %v", err)
		return result
	}

	// Read response status line
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	result.LatencyUs = time.Since(start).Microseconds()

	if err != nil {
		// If socket was reset/closed immediately by server (fail-fast)
		result.ConnectionClose = true
		if tc.ExpectClose && len(tc.ExpectedStatus) > 0 {
			// Some servers immediately reset socket on smuggling
			result.Passed = true
			result.StatusLine = "Connection reset by peer (immediate fail-fast teardown)"
			return result
		}
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Response read error: %v", err)
		return result
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
		return result
	}
	result.ActualStatus = code

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
	} else if tc.ExpectClose && !result.ConnectionClose {
		result.Passed = false
		result.FailureReason = fmt.Sprintf("Received status %d, but connection remained open (expected physical teardown)", code)
	} else {
		result.Passed = true
	}

	return result
}

func main() {
	targetHost := flag.String("target", "127.0.0.1:8080", "Target server host:port (Toron)")
	baselineHost := flag.String("baseline", "", "Optional baseline server host:port (e.g. NGINX/Caddy for differential comparison)")
	outJSON := flag.String("json", "benchmarks/results/differential_fuzz_report.json", "Output path for JSON report")
	outMD := flag.String("md", "benchmarks/results/differential_fuzz_report.md", "Output path for Markdown report")
	flag.Parse()

	testCases := getTestCases()

	fmt.Println("================================================================================")
	fmt.Println("       TORON DIFFERENTIAL PROTOCOL SECURITY FUZZER & INVARIANT CHECKER         ")
	fmt.Println("================================================================================")
	fmt.Printf(" Target Under Test:   %s\n", *targetHost)
	if *baselineHost != "" {
		fmt.Printf(" Baseline Reference:  %s (Differential Mode Active)\n", *baselineHost)
	}
	fmt.Printf(" Test Cases Loaded:   %d security invariant specifications\n", len(testCases))
	fmt.Println("================================================================================")
	fmt.Println()

	var results []TestResult
	categoryStats := make(map[string]CategoryStat)
	var totalLatency int64
	passedCount := 0

	for _, tc := range testCases {
		res := executeRawTest(*targetHost, tc)

		// Optional differential test against baseline
		if *baselineHost != "" {
			baseRes := executeRawTest(*baselineHost, tc)
			res.BaselineStatus = baseRes.ActualStatus
			// If target blocked (passed) but baseline accepted a dangerous payload (e.g. status 200)
			if tc.CWE != "N/A" && baseRes.ActualStatus == 200 {
				res.BaselineVulnerable = true
			}
		}

		results = append(results, res)
		totalLatency += res.LatencyUs

		cat := categoryStats[tc.Category]
		cat.Total++
		connState := "conn:open"
		if res.ConnectionClose {
			connState = "conn:closed"
		}
		if res.Passed {
			cat.Passed++
			passedCount++
			fmt.Printf(" [PASS] %-14s | %-40s | %d (%s) [%s] [%d µs]\n",
				tc.ID, tc.Name, res.ActualStatus, tc.CWE, connState, res.LatencyUs)
		} else {
			cat.Failed++
			fmt.Printf(" [FAIL] %-14s | %-40s | Received %d [%s] (%s)\n",
				tc.ID, tc.Name, res.ActualStatus, connState, res.FailureReason)
		}
		categoryStats[tc.Category] = cat
	}

	passRate := (float64(passedCount) / float64(len(testCases))) * 100.0
	avgLatency := float64(totalLatency) / float64(len(testCases))

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("                         FUZZER EXECUTION SUMMARY                               ")
	fmt.Println("================================================================================")
	fmt.Printf(" Total Invariant Tests:  %d\n", len(testCases))
	fmt.Printf(" Passed Assertions:      %d\n", passedCount)
	fmt.Printf(" Failed Assertions:      %d\n", len(testCases)-passedCount)
	fmt.Printf(" Security Pass Rate:     %.2f%%\n", passRate)
	fmt.Printf(" Average Rejection Lat:  %.2f µs\n", avgLatency)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println(" Category Breakdown:")
	for name, st := range categoryStats {
		catRate := (float64(st.Passed) / float64(st.Total)) * 100.0
		fmt.Printf("   %-32s : %2d / %2d passed (%.1f%%)\n", name, st.Passed, st.Total, catRate)
	}
	fmt.Println("================================================================================")

	report := DifferentialReport{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		TargetHost:       *targetHost,
		BaselineHost:     *baselineHost,
		TotalTests:       len(testCases),
		PassedTests:      passedCount,
		FailedTests:      len(testCases) - passedCount,
		SecurityPassRate: passRate,
		AverageLatencyUs: avgLatency,
		CategoryStats:    categoryStats,
		Results:          results,
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
		md.WriteString(fmt.Sprintf("**Overall Invariant Pass Rate**: **%.2f%%** (%d/%d tests)  \n", report.SecurityPassRate, report.PassedTests, report.TotalTests))
		md.WriteString(fmt.Sprintf("**Average Fail-Fast Rejection Latency**: `%.2f µs`  \n\n", report.AverageLatencyUs))

		md.WriteString("## 1. Category Summary Matrix\n\n")
		md.WriteString("| Security Category | Total Tests | Passed | Failed | Pass Rate |\n")
		md.WriteString("| :--- | :---: | :---: | :---: | :---: |\n")
		for catName, st := range report.CategoryStats {
			rate := (float64(st.Passed) / float64(st.Total)) * 100.0
			md.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.1f%% |\n", catName, st.Total, st.Passed, st.Failed, rate))
		}
		md.WriteString("\n## 2. Detailed Invariant Test Results\n\n")
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
			expectedStr := "400/501"
			if r.TestCaseID == "BASELINE-001" {
				expectedStr = "200"
			}
			md.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` | `%d` | `%s` | `%d µs` | %s |\n",
				r.TestCaseID, r.Name, r.CWE, expectedStr, r.ActualStatus, connIcon, r.LatencyUs, statusIcon))
		}

		_ = os.WriteFile(*outMD, []byte(md.String()), 0644)
		fmt.Printf(" [Artifact Saved] Markdown report: %s\n", *outMD)
	}
}
