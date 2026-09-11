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
