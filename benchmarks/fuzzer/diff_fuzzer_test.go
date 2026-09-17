package main

import (
	"bufio"
	"encoding/json"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TC-101-01: Verification of Closed Connection Following Error Response
func TestVerifySocketClosed_ClosedByServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 15\r\nConnection: close\r\n\r\n{\"error\":\"400\"}"))
		// Connection closes immediately on defer
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("GET /malicious HTTP/1.1\r\nHost: localhost\r\n\r\n"))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	reader := bufio.NewReader(clientConn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}
	if !strings.Contains(statusLine, "400") {
		t.Fatalf("unexpected status line: %s", statusLine)
	}

	closed := verifySocketClosed(clientConn, reader)
	if !closed {
		t.Errorf("expected verifySocketClosed to return true for closed socket, got false")
	}
}

// TC-101-02: Verification of Keep-Alive Connection Detection
func TestVerifySocketClosed_KeepAlive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nOK"))

		// Wait briefly to allow client inspection before closing
		time.Sleep(500 * time.Millisecond)
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n"))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	reader := bufio.NewReader(clientConn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}
	if !strings.Contains(statusLine, "200") {
		t.Fatalf("unexpected status line: %s", statusLine)
	}

	closed := verifySocketClosed(clientConn, reader)
	if closed {
		t.Errorf("expected verifySocketClosed to return false for keep-alive connection, got true")
	}
	<-done
}

// TC-101-03: Invariant Enforcement When ExpectClose is True
func TestExecuteRawTest_ExpectClose_Assertion(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	// Handler that returns 400 but erroneously keeps connection open
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
				_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nOK"))
				time.Sleep(500 * time.Millisecond)
			}(conn)
		}
	}()

	tc := TestCase{
		ID:             "TEST-INVARIANT-001",
		Category:       "Smuggling",
		Name:           "Expect Close Test",
		CWE:            "CWE-444",
		RawPayload:     "POST / HTTP/1.1\r\nHost: localhost\r\n\r\n",
		ExpectedStatus: []int{400},
		ExpectClose:    true,
	}

	res := executeRawTest(ln.Addr().String(), tc)
	if res.Passed {
		t.Errorf("expected test to fail when ExpectClose=true but socket remained open, got passed")
	}
	if res.ConnectionClose {
		t.Errorf("expected ConnectionClose=false for keep-alive server, got true")
	}
	if !strings.Contains(res.FailureReason, "connection remained open") {
		t.Errorf("expected failure reason to mention connection remained open, got: %s", res.FailureReason)
	}
}

// TC-101-04: Isolation of Rejection Latency From Probe Deadline
func TestExecuteRawTest_LatencyIsolation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		// Write response fast (< 5ms)
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
		time.Sleep(400 * time.Millisecond)
	}()

	tc := TestCase{
		ID:             "TEST-LATENCY-001",
		Category:       "Baseline",
		Name:           "Latency Isolation Test",
		CWE:            "N/A",
		RawPayload:     "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n",
		ExpectedStatus: []int{200},
		ExpectClose:    false,
	}

	res := executeRawTest(ln.Addr().String(), tc)
	// Latency must NOT include the 250ms probe drain timeout
	if res.LatencyUs > 100000 {
		t.Errorf("expected LatencyUs < 100000 (100ms), got %d µs (probe timeout leaked into latency)", res.LatencyUs)
	}
}

// TC-102-01: Rejection of HTTP 404 False-Positive Pass for Traversal
func TestExecuteRawTest_Traversal_Rejects404AsFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nNot Found"))
	}()

	testCases := getTestCases()
	var traversalTC TestCase
	for _, tc := range testCases {
		if tc.ID == "TRAVERSAL-001" {
			traversalTC = tc
			break
		}
	}
	if traversalTC.ID == "" {
		t.Fatalf("TRAVERSAL-001 test case not found")
	}

	res := executeRawTest(ln.Addr().String(), traversalTC)
	if res.Passed {
		t.Errorf("expected TRAVERSAL-001 to fail when server returns 404, but it passed")
	}
	if !strings.Contains(res.FailureReason, "404") {
		t.Errorf("expected failure reason to mention status 404, got: %s", res.FailureReason)
	}
}

// TC-102-02 & TC-102-03: Acceptance of HTTP 403 and 400 Active Defensive Status Codes
func TestExecuteRawTest_Traversal_Accepts403And400(t *testing.T) {
	for _, expectedCode := range []int{403, 400} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}

		go func(code int) {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			buf := make([]byte, 1024)
			_, _ = conn.Read(buf)
			statusText := "Forbidden"
			if code == 400 {
				statusText = "Bad Request"
			}
			_, _ = conn.Write([]byte(strings.Join([]string{
				"HTTP/1.1 " + string(rune('0'+code/100)) + string(rune('0'+(code/10)%10)) + string(rune('0'+code%10)) + " " + statusText,
				"Content-Length: 0",
				"Connection: close",
				"\r\n",
			}, "\r\n")))
		}(expectedCode)

		testCases := getTestCases()
		var traversalTC TestCase
		for _, tc := range testCases {
			if tc.ID == "TRAVERSAL-001" {
				traversalTC = tc
				break
			}
		}

		res := executeRawTest(ln.Addr().String(), traversalTC)
		ln.Close()

		if !res.Passed {
			t.Errorf("expected status %d to pass active traversal oracle, failed with: %s", expectedCode, res.FailureReason)
		}
	}
}

// TC-102-04: Rejection of HTTP 200 Canary Leakage
func TestExecuteRawTest_Traversal_Rejects200CanaryLeak(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		// Server mistakenly leaks canary file
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 42\r\n\r\nTORON_CANARY_TRAVERSAL_PROTECTION_VERIFIED\n"))
	}()

	testCases := getTestCases()
	var traversalTC TestCase
	for _, tc := range testCases {
		if tc.ID == "TRAVERSAL-001" {
			traversalTC = tc
			break
		}
	}

	res := executeRawTest(ln.Addr().String(), traversalTC)
	if res.Passed {
		t.Errorf("expected TRAVERSAL-001 to fail when server leaks canary with 200, but it passed")
	}
	if !strings.Contains(res.FailureReason, "200") {
		t.Errorf("expected failure reason to mention status 200, got: %s", res.FailureReason)
	}
}

// TC-104-01 & TC-104-02 & TC-104-04: Dynamic Canonical RFC Status Code Formatting
func TestFormatExpectedStatus(t *testing.T) {
	tests := []struct {
		input    []int
		expected string
	}{
		{nil, "N/A"},
		{[]int{}, "N/A"},
		{[]int{200}, "200 OK"},
		{[]int{400}, "400 Bad Request"},
		{[]int{403}, "403 Forbidden"},
		{[]int{404}, "404 Not Found"},
		{[]int{414}, "414 Request URI Too Long"},
		{[]int{501}, "501 Not Implemented"},
		{[]int{400, 501}, "400/501"},
		{[]int{400, 403}, "400/403"},
		{[]int{400, 413, 414}, "400/413/414"},
		{[]int{200, 401}, "200/401"},
	}

	for _, tt := range tests {
		actual := formatExpectedStatus(tt.input)
		if actual != tt.expected {
			t.Errorf("formatExpectedStatus(%v): expected %q, got %q", tt.input, tt.expected, actual)
		}
	}
}

// TC-104-03: BASELINE-001 Canonical Status Rendering in TestResult
func TestExecuteRawTest_Baseline_ExpectedStatusStr(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
	}()

	testCases := getTestCases()
	var baselineTC TestCase
	for _, tc := range testCases {
		if tc.ID == "BASELINE-001" {
			baselineTC = tc
			break
		}
	}
	if baselineTC.ID == "" {
		t.Fatalf("BASELINE-001 test case not found")
	}

	res := executeRawTest(ln.Addr().String(), baselineTC)
	if !res.Passed {
		t.Errorf("expected BASELINE-001 to pass, failed: %s", res.FailureReason)
	}
	if res.ExpectedStatusStr != "200 OK" {
		t.Errorf("expected ExpectedStatusStr to be '200 OK', got %q", res.ExpectedStatusStr)
	}
}

// TC-103-01 & TC-103-02: CACHE-001 (Web Cache Deception Prevention)
func TestExecuteRawTest_Cache_WebCacheDeception_PassAndFail(t *testing.T) {
	testCases := getTestCases()
	var cache001 TestCase
	for _, tc := range testCases {
		if tc.ID == "CACHE-001" {
			cache001 = tc
			break
		}
	}
	if cache001.ID == "" {
		t.Fatalf("CACHE-001 test case not found")
	}

	// 1. Conformant Cache Mock: Private request is NOT served on subsequent probe
	t.Run("Conformant_MissOnProbe", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			// Request 1 (prep authenticated request)
			c1, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 1024)
			_, _ = c1.Read(buf)
			_, _ = c1.Write([]byte("HTTP/1.1 200 OK\r\nCache-Control: private\r\nX-Cache: MISS\r\nContent-Length: 7\r\n\r\nprivate"))
			_ = c1.Close()

			// Request 2 (probe unauthenticated request)
			c2, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c2.Read(buf)
			_, _ = c2.Write([]byte("HTTP/1.1 401 Unauthorized\r\nX-Cache: MISS\r\nContent-Length: 12\r\n\r\nunauthorized"))
			_ = c2.Close()
		}()

		res := executeRawTest(ln.Addr().String(), cache001)
		if !res.Passed {
			t.Errorf("expected CACHE-001 to pass on conformant cache, failed with: %s", res.FailureReason)
		}
		if res.ResponseHeaders["X-Cache"] != "MISS" {
			t.Errorf("expected X-Cache header to be 'MISS', got %q", res.ResponseHeaders["X-Cache"])
		}
	})

	// 2. Vulnerable Cache Mock: Erroneously serves cached private data to probe
	t.Run("Vulnerable_HitOnProbeLeaksToken", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			c1, _ := ln.Accept()
			buf := make([]byte, 1024)
			_, _ = c1.Read(buf)
			_, _ = c1.Write([]byte("HTTP/1.1 200 OK\r\nX-Cache: MISS\r\nContent-Length: 7\r\n\r\nprivate"))
			_ = c1.Close()

			c2, _ := ln.Accept()
			_, _ = c2.Read(buf)
			// Buggy cache emits HIT and leaks private token
			_, _ = c2.Write([]byte("HTTP/1.1 200 OK\r\nX-Cache: HIT\r\nX-Private-Token: leaked-token\r\nContent-Length: 7\r\n\r\nprivate"))
			_ = c2.Close()
		}()

		res := executeRawTest(ln.Addr().String(), cache001)
		if res.Passed {
			t.Errorf("expected CACHE-001 to fail when private cache was leaked to probe, but it passed")
		}
	})
}

// TC-103-03: CACHE-002 (Set-Cookie Header Stripping)
func TestExecuteRawTest_Cache_SetCookieStripping_PassAndFail(t *testing.T) {
	testCases := getTestCases()
	var cache002 TestCase
	for _, tc := range testCases {
		if tc.ID == "CACHE-002" {
			cache002 = tc
			break
		}
	}
	if cache002.ID == "" {
		t.Fatalf("CACHE-002 test case not found")
	}

	// 1. Conformant Cache: Strips Set-Cookie before serving cached response
	t.Run("Conformant_SetCookieStrippedOnHit", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			c1, _ := ln.Accept()
			buf := make([]byte, 1024)
			_, _ = c1.Read(buf)
			_, _ = c1.Write([]byte("HTTP/1.1 200 OK\r\nSet-Cookie: session=xyz123\r\nCache-Control: public, max-age=60\r\nX-Cache: MISS\r\nContent-Length: 2\r\n\r\nOK"))
			_ = c1.Close()

			c2, _ := ln.Accept()
			_, _ = c2.Read(buf)
			// Conformant: X-Cache: HIT and Set-Cookie stripped!
			_, _ = c2.Write([]byte("HTTP/1.1 200 OK\r\nCache-Control: public, max-age=60\r\nX-Cache: HIT\r\nContent-Length: 2\r\n\r\nOK"))
			_ = c2.Close()
		}()

		res := executeRawTest(ln.Addr().String(), cache002)
		if !res.Passed {
			t.Errorf("expected CACHE-002 to pass when Set-Cookie is stripped on hit, failed: %s", res.FailureReason)
		}
	})

	// 2. Vulnerable Cache: Leaks Set-Cookie in cached response
	t.Run("Vulnerable_SetCookieLeakedOnHit", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			c1, _ := ln.Accept()
			buf := make([]byte, 1024)
			_, _ = c1.Read(buf)
			_, _ = c1.Write([]byte("HTTP/1.1 200 OK\r\nSet-Cookie: session=victim_secret\r\nX-Cache: MISS\r\nContent-Length: 2\r\n\r\nOK"))
			_ = c1.Close()

			c2, _ := ln.Accept()
			_, _ = c2.Read(buf)
			// Vulnerable: Set-Cookie leaked to second client!
			_, _ = c2.Write([]byte("HTTP/1.1 200 OK\r\nSet-Cookie: session=victim_secret\r\nX-Cache: HIT\r\nContent-Length: 2\r\n\r\nOK"))
			_ = c2.Close()
		}()

		res := executeRawTest(ln.Addr().String(), cache002)
		if res.Passed {
			t.Errorf("expected CACHE-002 to fail when Set-Cookie was leaked in cached response, but it passed")
		}
		if !strings.Contains(res.FailureReason, "Forbidden header") {
			t.Errorf("expected failure reason to mention forbidden header, got: %s", res.FailureReason)
		}
	})
}

// TC-103-04: CACHE-003 (Authorization Refusal Invariant)
func TestExecuteRawTest_Cache_AuthorizationRefusal_PassAndFail(t *testing.T) {
	testCases := getTestCases()
	var cache003 TestCase
	for _, tc := range testCases {
		if tc.ID == "CACHE-003" {
			cache003 = tc
			break
		}
	}
	if cache003.ID == "" {
		t.Fatalf("CACHE-003 test case not found")
	}

	// 1. Conformant: Authenticated response lacking public is refused storage
	t.Run("Conformant_AuthResponseRefusedStorage", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			c1, _ := ln.Accept()
			buf := make([]byte, 1024)
			_, _ = c1.Read(buf)
			_, _ = c1.Write([]byte("HTTP/1.1 200 OK\r\nX-Cache: MISS\r\nContent-Length: 9\r\n\r\nprotected"))
			_ = c1.Close()

			c2, _ := ln.Accept()
			_, _ = c2.Read(buf)
			// Probe cannot get cached entry: returns 401 with X-Cache: MISS
			_, _ = c2.Write([]byte("HTTP/1.1 401 Unauthorized\r\nX-Cache: MISS\r\nContent-Length: 12\r\n\r\nunauthorized"))
			_ = c2.Close()
		}()

		res := executeRawTest(ln.Addr().String(), cache003)
		if !res.Passed {
			t.Errorf("expected CACHE-003 to pass when auth response refused storage, failed: %s", res.FailureReason)
		}
	})

	// 2. Vulnerable: Cache improperly serves authenticated content to unauthenticated probe
	t.Run("Vulnerable_AuthResponseServedFromCache", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			c1, _ := ln.Accept()
			buf := make([]byte, 1024)
			_, _ = c1.Read(buf)
			_, _ = c1.Write([]byte("HTTP/1.1 200 OK\r\nX-Cache: MISS\r\nContent-Length: 9\r\n\r\nprotected"))
			_ = c1.Close()

			c2, _ := ln.Accept()
			_, _ = c2.Read(buf)
			_, _ = c2.Write([]byte("HTTP/1.1 200 OK\r\nX-Cache: HIT\r\nX-User-Data: leaked-info\r\nContent-Length: 9\r\n\r\nprotected"))
			_ = c2.Close()
		}()

		res := executeRawTest(ln.Addr().String(), cache003)
		if res.Passed {
			t.Errorf("expected CACHE-003 to fail when auth response served from cache, but it passed")
		}
	})
}

// TC-103-05 & TC-103-06: Header Assertion Failure Oracles
func TestExecuteRawTest_HeaderAssertionOracles(t *testing.T) {
	// 1. Missing Required Header
	t.Run("MissingRequiredHeader", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			c, _ := ln.Accept()
			defer c.Close()
			buf := make([]byte, 1024)
			_, _ = c.Read(buf)
			_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
		}()

		tc := TestCase{
			ID:                      "HEADER-REQ-001",
			Category:                "HeaderAssertion",
			Name:                    "Missing Required Header Test",
			RawPayload:              "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus:          []int{200},
			RequiredResponseHeaders: map[string]string{"X-Expected": "Match"},
		}

		res := executeRawTest(ln.Addr().String(), tc)
		if res.Passed {
			t.Errorf("expected test to fail when required header is missing, got passed")
		}
		if !strings.Contains(res.FailureReason, "Required header") {
			t.Errorf("expected failure reason to mention required header, got: %s", res.FailureReason)
		}
	})

	// 2. Forbidden Header Present
	t.Run("ForbiddenHeaderPresent", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			c, _ := ln.Accept()
			defer c.Close()
			buf := make([]byte, 1024)
			_, _ = c.Read(buf)
			_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nX-Leaked: secret\r\nContent-Length: 2\r\n\r\nOK"))
		}()

		tc := TestCase{
			ID:                       "HEADER-FORB-001",
			Category:                 "HeaderAssertion",
			Name:                     "Forbidden Header Test",
			RawPayload:               "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n",
			ExpectedStatus:           []int{200},
			ForbiddenResponseHeaders: []string{"X-Leaked"},
		}

		res := executeRawTest(ln.Addr().String(), tc)
		if res.Passed {
			t.Errorf("expected test to fail when forbidden header is present, got passed")
		}
		if !strings.Contains(res.FailureReason, "Forbidden header") {
			t.Errorf("expected failure reason to mention forbidden header, got: %s", res.FailureReason)
		}
	})
}

// TC-106-01: Verification of Equation 7 Timing Isolation
func TestExecuteRawTest_Equation7_TimingIsolation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	// Server handles 1 prep request (with 50ms artificial delay) and 1 probe request (instant)
	go func() {
		// Prep connection
		c1, err := ln.Accept()
		if err != nil {
			return
		}
		defer c1.Close()
		buf := make([]byte, 1024)
		_, _ = c1.Read(buf)
		time.Sleep(50 * time.Millisecond) // Artificial prep delay
		_, _ = c1.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))

		// Probe connection
		c2, err := ln.Accept()
		if err != nil {
			return
		}
		defer c2.Close()
		_, _ = c2.Read(buf)
		// Instant response
		_, _ = c2.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n"))
	}()

	tc := TestCase{
		ID:       "EQ7-ISOLATION-001",
		Category: "Timing",
		Name:     "Equation 7 Timing Isolation Test",
		SequentialPayloads: []string{
			"GET /prep HTTP/1.1\r\nHost: localhost\r\n\r\n",
			"GET /probe HTTP/1.1\r\nHost: localhost\r\n\r\n",
		},
		ExpectedStatus: []int{400},
	}

	res := executeRawTest(ln.Addr().String(), tc)
	if !res.Passed {
		t.Fatalf("expected test to pass, failed: %s", res.FailureReason)
	}
	// Measured rejection latency MUST be strictly isolated from the 50ms prep stage
	if res.Distribution.MeanLatencyUs >= 30000.0 {
		t.Errorf("expected MeanLatencyUs < 30,000 µs (isolated from 50ms prep delay), got %.2f µs", res.Distribution.MeanLatencyUs)
	}
}

// TC-106-02: Verification of Statistical Distribution Precision
func TestComputeDistribution_StatisticalPrecision(t *testing.T) {
	samples := []float64{10.0, 20.0, 30.0, 40.0, 50.0, 60.0, 70.0, 80.0, 90.0, 100.0}
	dist := computeDistribution(samples, 5)

	if dist.Trials != 10 {
		t.Errorf("expected Trials=10, got %d", dist.Trials)
	}
	if dist.WarmupRuns != 5 {
		t.Errorf("expected WarmupRuns=5, got %d", dist.WarmupRuns)
	}
	if math.Abs(dist.MeanLatencyUs-55.0) > 0.01 {
		t.Errorf("expected Mean 55.0, got %.4f", dist.MeanLatencyUs)
	}
	if math.Abs(dist.MedianLatencyUs-55.0) > 0.01 {
		t.Errorf("expected Median 55.0, got %.4f", dist.MedianLatencyUs)
	}
	// Sample standard deviation (Bessel's correction N-1): sqrt(8250 / 9) ≈ 30.2765
	if math.Abs(dist.StdDevLatencyUs-30.2765) > 0.01 {
		t.Errorf("expected StdDev 30.2765, got %.4f", dist.StdDevLatencyUs)
	}
	// Nearest-rank p90: ceil(0.9 * 10) - 1 = 8 -> 90.0
	if math.Abs(dist.P90LatencyUs-90.0) > 0.01 {
		t.Errorf("expected P90 90.0, got %.4f", dist.P90LatencyUs)
	}
	// Nearest-rank p99: ceil(0.99 * 10) - 1 = 9 -> 100.0
	if math.Abs(dist.P99LatencyUs-100.0) > 0.01 {
		t.Errorf("expected P99 100.0, got %.4f", dist.P99LatencyUs)
	}
	// CI 95 margin: 1.96 * (30.2765 / sqrt(10)) ≈ 18.7656
	expectedMargin := 1.96 * (dist.StdDevLatencyUs / math.Sqrt(10))
	if math.Abs(dist.CI95MarginUs-expectedMargin) > 0.01 {
		t.Errorf("expected CI95Margin %.4f, got %.4f", expectedMargin, dist.CI95MarginUs)
	}
	if math.Abs(dist.CI95LowerUs-(55.0-expectedMargin)) > 0.01 {
		t.Errorf("expected CI95Lower %.4f, got %.4f", 55.0-expectedMargin, dist.CI95LowerUs)
	}
	if math.Abs(dist.CI95UpperUs-(55.0+expectedMargin)) > 0.01 {
		t.Errorf("expected CI95Upper %.4f, got %.4f", 55.0+expectedMargin, dist.CI95UpperUs)
	}
}

// TC-106-03: Multi-Trial Execution & Warm-up Verification
func TestExecuteRawTest_RepeatedTrials_Warmup(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	var requestCount int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				atomic.AddInt64(&requestCount, 1)
				buf := make([]byte, 1024)
				_, _ = conn.Read(buf)
				_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
			}(c)
		}
	}()

	tc := TestCase{
		ID:             "REPEAT-001",
		Category:       "Baseline",
		Name:           "Repeated Trials Verification",
		RawPayload:     "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n",
		ExpectedStatus: []int{200},
	}

	trials := 10
	warmup := 3
	res := executeRawTest(ln.Addr().String(), tc, trials, warmup)

	if !res.Passed {
		t.Fatalf("expected test to pass, failed: %s", res.FailureReason)
	}
	if res.Distribution.Trials != trials {
		t.Errorf("expected Trials=%d, got %d", trials, res.Distribution.Trials)
	}
	if res.Distribution.WarmupRuns != warmup {
		t.Errorf("expected WarmupRuns=%d, got %d", warmup, res.Distribution.WarmupRuns)
	}
	if len(res.Distribution.RawSamples) != trials {
		t.Errorf("expected len(RawSamples)=%d, got %d", trials, len(res.Distribution.RawSamples))
	}
	totalExpected := int64(trials + warmup)
	if atomic.LoadInt64(&requestCount) != totalExpected {
		t.Errorf("expected server to receive %d requests (%d warmup + %d trials), got %d",
			totalExpected, warmup, trials, atomic.LoadInt64(&requestCount))
	}
}

// TC-106-04: Multi-Stage Cache Vector Repeatability under Multiple Trials
func TestExecuteRawTest_MultiStage_RepeatedTrials(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	var totalConns int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				atomic.AddInt64(&totalConns, 1)
				buf := make([]byte, 1024)
				n, _ := conn.Read(buf)
				req := string(buf[:n])
				if strings.Contains(req, "Authorization:") {
					// Prep request
					_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nCache-Control: private\r\nX-Cache: MISS\r\nContent-Length: 7\r\n\r\nprivate"))
				} else {
					// Probe request
					_, _ = conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\nX-Cache: MISS\r\nContent-Length: 12\r\n\r\nunauthorized"))
				}
			}(c)
		}
	}()

	testCases := getTestCases()
	var cache001 TestCase
	for _, tc := range testCases {
		if tc.ID == "CACHE-001" {
			cache001 = tc
			break
		}
	}
	if cache001.ID == "" {
		t.Fatalf("CACHE-001 test case not found")
	}

	trials := 5
	warmup := 2
	res := executeRawTest(ln.Addr().String(), cache001, trials, warmup)
	if !res.Passed {
		t.Fatalf("expected multi-stage repeated trials to pass, failed: %s", res.FailureReason)
	}
	if res.Distribution.Trials != trials {
		t.Errorf("expected Trials=%d, got %d", trials, res.Distribution.Trials)
	}
	// Total connections = (warmup + trials) * 2 (1 prep + 1 probe per iteration)
	expectedConns := int64((trials + warmup) * 2)
	if atomic.LoadInt64(&totalConns) != expectedConns {
		t.Errorf("expected %d total connections, got %d", expectedConns, atomic.LoadInt64(&totalConns))
	}
}

// TC-118-01: Verification of Cohort Disaggregation and Statistical Calculation
func TestCalculateCohortMetrics(t *testing.T) {
	// Empty slice test
	emptyCohort := calculateCohortMetrics(nil, true)
	if emptyCohort.Count != 0 {
		t.Errorf("expected count 0 for empty slice, got %d", emptyCohort.Count)
	}

	// 18 Fail-Fast Rejection Vector latencies from empirical benchmark
	failFastSamples := []float64{
		87.29, 100.29, 131.25, 148.46, 165.58, 169.58,
		177.04, 179.04, 187.96, 208.75, 210.92, 238.79,
		250.08, 362.13, 577.00, 582.58, 708.58, 712.08,
	}

	cohort18 := calculateCohortMetrics(failFastSamples, false)
	if cohort18.Count != 18 {
		t.Fatalf("expected count 18, got %d", cohort18.Count)
	}
	if math.Abs(cohort18.MeanLatencyUs-288.74) > 0.1 {
		t.Errorf("expected mean ~288.74 µs, got %.2f", cohort18.MeanLatencyUs)
	}
	if cohort18.MedianLatencyUs != 208.75 {
		t.Errorf("expected median 208.75 µs, got %.2f", cohort18.MedianLatencyUs)
	}
	if cohort18.P90LatencyUs != 708.58 {
		t.Errorf("expected p90 708.58 µs, got %.2f", cohort18.P90LatencyUs)
	}
	if cohort18.MaxLatencyUs != 712.08 {
		t.Errorf("expected max 712.08 µs, got %.2f", cohort18.MaxLatencyUs)
	}
	if cohort18.P99LatencyUs != 0 {
		t.Errorf("expected p99 0 when includeP99=false, got %.2f", cohort18.P99LatencyUs)
	}

	// 19 Comprehensive Vector latencies (including BASELINE-001 at 2378.00)
	compSamples := append([]float64{}, failFastSamples...)
	compSamples = append(compSamples, 2378.00)

	cohort19 := calculateCohortMetrics(compSamples, true)
	if cohort19.Count != 19 {
		t.Fatalf("expected count 19, got %d", cohort19.Count)
	}
	if math.Abs(cohort19.MeanLatencyUs-398.71) > 0.1 {
		t.Errorf("expected mean ~398.71 µs, got %.2f", cohort19.MeanLatencyUs)
	}
	if cohort19.MedianLatencyUs != 208.75 {
		t.Errorf("expected median 208.75 µs, got %.2f", cohort19.MedianLatencyUs)
	}
	if cohort19.P90LatencyUs != 712.08 {
		t.Errorf("expected p90 712.08 µs (CONTROL-002), got %.2f", cohort19.P90LatencyUs)
	}
	if cohort19.P99LatencyUs != 2378.00 {
		t.Errorf("expected p99 2378.00 µs (BASELINE-001), got %.2f", cohort19.P99LatencyUs)
	}
	if cohort19.MaxLatencyUs != 2378.00 {
		t.Errorf("expected max 2378.00 µs, got %.2f", cohort19.MaxLatencyUs)
	}
}

// TC-118-04: Verification of JSON Report Serialization with Cohort Disaggregation
func TestDifferentialReport_JSONSerialization(t *testing.T) {
	failFast := CohortMetrics{
		Count:           18,
		MeanLatencyUs:   288.75,
		MedianLatencyUs: 208.75,
		P90LatencyUs:    708.58,
		MaxLatencyUs:    712.08,
	}
	comp := CohortMetrics{
		Count:           19,
		MeanLatencyUs:   398.71,
		MedianLatencyUs: 208.75,
		P90LatencyUs:    712.08,
		P99LatencyUs:    2378.00,
		MaxLatencyUs:    2378.00,
	}

	report := DifferentialReport{
		Timestamp:          "2026-09-12T12:00:00Z",
		TargetHost:         "127.0.0.1:8080",
		TrialsPerTest:      1,
		WarmupRunsPerTest:  0,
		TotalTests:         19,
		PassedTests:        19,
		FailedTests:        0,
		SecurityPassRate:   100.0,
		AverageLatencyUs:   comp.MeanLatencyUs,
		MedianLatencyUs:    comp.MedianLatencyUs,
		P90LatencyUs:       comp.P90LatencyUs,
		P99LatencyUs:       comp.P99LatencyUs,
		FailFastDefense:    failFast,
		ComprehensiveSuite: comp,
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	var parsed DifferentialReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if parsed.FailFastDefense.Count != 18 {
		t.Errorf("expected FailFastDefense.Count 18, got %d", parsed.FailFastDefense.Count)
	}
	if parsed.ComprehensiveSuite.Count != 19 {
		t.Errorf("expected ComprehensiveSuite.Count 19, got %d", parsed.ComprehensiveSuite.Count)
	}
	if parsed.ComprehensiveSuite.P90LatencyUs != 712.08 {
		t.Errorf("expected ComprehensiveSuite.P90LatencyUs 712.08, got %.2f", parsed.ComprehensiveSuite.P90LatencyUs)
	}
	if parsed.ComprehensiveSuite.P99LatencyUs != 2378.00 {
		t.Errorf("expected ComprehensiveSuite.P99LatencyUs 2378.00, got %.2f", parsed.ComprehensiveSuite.P99LatencyUs)
	}
}
