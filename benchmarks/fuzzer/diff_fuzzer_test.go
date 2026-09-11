package main

import (
	"bufio"
	"net"
	"strings"
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
