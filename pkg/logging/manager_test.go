package logging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"toron/pkg/httpparser"
)

func TestLogSink_FileCreationAndDirectory(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "sub1", "sub2", "test.log")

	sink, err := NewLogSink(logPath)
	if err != nil {
		t.Fatalf("failed to create LogSink: %v", err)
	}
	defer sink.Close()

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("expected log file %s to be created", logPath)
	}

	testData := []byte("hello world\n")
	n, err := sink.Write(testData)
	if err != nil || n != len(testData) {
		t.Fatalf("failed to write to sink: n=%d, err=%v", n, err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Fatalf("unexpected content: %q", string(content))
	}
}

func TestLogSink_ReopenLogrotate(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "access.log")
	rotatedPath := filepath.Join(tempDir, "access.log.1")

	sink, err := NewLogSink(logPath)
	if err != nil {
		t.Fatalf("failed to create LogSink: %v", err)
	}
	defer sink.Close()

	_, _ = sink.Write([]byte("line 1\n"))

	// Simulate system logrotate: move access.log -> access.log.1
	if err := os.Rename(logPath, rotatedPath); err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}

	// Trigger Reopen (as if SIGHUP was received)
	if err := sink.Reopen(); err != nil {
		t.Fatalf("failed to reopen sink: %v", err)
	}

	// Write line 2 after rotation
	_, _ = sink.Write([]byte("line 2\n"))

	// Verify rotated file has line 1
	rotContent, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("failed to read rotated file: %v", err)
	}
	if strings.TrimSpace(string(rotContent)) != "line 1" {
		t.Fatalf("expected 'line 1' in rotated file, got %q", string(rotContent))
	}

	// Verify new file has line 2
	newContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read new log file: %v", err)
	}
	if strings.TrimSpace(string(newContent)) != "line 2" {
		t.Fatalf("expected 'line 2' in new file, got %q", string(newContent))
	}
}

func TestLogManager_MultiStreamAndRouteOverrides(t *testing.T) {
	tempDir := t.TempDir()
	serverLog := filepath.Join(tempDir, "server.log")
	accessLog := filepath.Join(tempDir, "access.log")
	securityLog := filepath.Join(tempDir, "security.log")
	routeAccessLog := filepath.Join(tempDir, "api_access.log")
	routeSecLog := filepath.Join(tempDir, "api_security.log")

	mgr, err := NewLogManager(Config{
		Format:      "text",
		ServerLog:   serverLog,
		AccessLog:   accessLog,
		SecurityLog: securityLog,
	})
	if err != nil {
		t.Fatalf("failed to create LogManager: %v", err)
	}
	defer mgr.Close()

	// 1. Server Writer test
	sw := mgr.ServerWriter()
	_, _ = sw.Write([]byte("[TORON] Server started on :8080\n"))

	// 2. Default Access Log
	now := time.Now()
	mgr.LogAccess(AccessLogEntry{
		Timestamp:   now,
		ClientIP:    "192.168.1.10",
		Method:      "GET",
		Path:        "/index.html",
		Protocol:    "HTTP/1.1",
		StatusCode:  200,
		Duration:    5 * time.Millisecond,
		BytesSent:   1234,
		UserAgent:   "Go-Test/1.0",
		RoutePrefix: "",
	}, "")

	// 3. Route Override Access Log
	mgr.LogAccess(AccessLogEntry{
		Timestamp:   now,
		ClientIP:    "10.0.0.5",
		Method:      "POST",
		Path:        "/api/users",
		Protocol:    "HTTP/1.1",
		StatusCode:  201,
		Duration:    12 * time.Millisecond,
		BytesSent:   567,
		UserAgent:   "Go-Test/1.0",
		RoutePrefix: "/api",
	}, routeAccessLog)

	// 4. Route Access Log Silenced ("off")
	mgr.LogAccess(AccessLogEntry{
		Timestamp:  now,
		ClientIP:   "10.0.0.5",
		Method:     "GET",
		Path:       "/silent",
		StatusCode: 200,
	}, "off")

	// 5. Default Security Log
	mgr.LogSecurity(SecurityLogEntry{
		Timestamp:    now,
		Event:        "waf_block",
		ClientIP:     "203.0.113.1",
		Method:       "GET",
		Path:         "/etc/passwd",
		Category:     "path_traversal",
		AnomalyScore: 10,
		Action:       "blocked",
	}, "")

	// 6. Route Override Security Log
	mgr.LogSecurity(SecurityLogEntry{
		Timestamp:    now,
		Event:        "api_key_invalid",
		ClientIP:     "198.51.100.2",
		Method:       "POST",
		Path:         "/api/admin",
		Category:     "auth",
		AnomalyScore: 5,
		Action:       "blocked",
	}, routeSecLog)

	// Assertions
	srvBytes, _ := os.ReadFile(serverLog)
	if !strings.Contains(string(srvBytes), "Server started on :8080") {
		t.Errorf("server log missing expected string: %s", string(srvBytes))
	}

	accBytes, _ := os.ReadFile(accessLog)
	if !strings.Contains(string(accBytes), "/index.html") {
		t.Errorf("default access log missing /index.html: %s", string(accBytes))
	}
	if strings.Contains(string(accBytes), "/api/users") {
		t.Errorf("default access log should NOT contain overridden /api/users")
	}

	routeAccBytes, _ := os.ReadFile(routeAccessLog)
	if !strings.Contains(string(routeAccBytes), "/api/users") {
		t.Errorf("route access log missing /api/users: %s", string(routeAccBytes))
	}

	secBytes, _ := os.ReadFile(securityLog)
	if !strings.Contains(string(secBytes), "path_traversal") {
		t.Errorf("default security log missing path_traversal: %s", string(secBytes))
	}

	routeSecBytes, _ := os.ReadFile(routeSecLog)
	if !strings.Contains(string(routeSecBytes), "api_key_invalid") {
		t.Errorf("route security log missing api_key_invalid: %s", string(routeSecBytes))
	}
}

func TestLogManager_JSONFormat(t *testing.T) {
	tempDir := t.TempDir()
	accessLog := filepath.Join(tempDir, "access.json")

	mgr, err := NewLogManager(Config{
		Format:    "json",
		AccessLog: accessLog,
	})
	if err != nil {
		t.Fatalf("failed to create LogManager: %v", err)
	}
	defer mgr.Close()

	mgr.LogAccess(AccessLogEntry{
		Timestamp:   time.Now(),
		ClientIP:    "1.2.3.4",
		Method:      "GET",
		Path:        "/metrics",
		Protocol:    "HTTP/2.0",
		StatusCode:  200,
		Duration:    2 * time.Millisecond,
		BytesSent:   500,
		RoutePrefix: "/metrics",
	}, "")

	content, err := os.ReadFile(accessLog)
	if err != nil {
		t.Fatalf("failed to read json access log: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(content), &parsed); err != nil {
		t.Fatalf("failed to unmarshal json log line: %v, content: %s", err, string(content))
	}

	if parsed["client_ip"] != "1.2.3.4" || parsed["path"] != "/metrics" || parsed["status"] != float64(200) {
		t.Errorf("unexpected parsed JSON: %+v", parsed)
	}
}

func TestLogManager_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	accessLog := filepath.Join(tempDir, "concurrent_access.log")

	mgr, err := NewLogManager(Config{
		Format:    "text",
		AccessLog: accessLog,
	})
	if err != nil {
		t.Fatalf("failed to create LogManager: %v", err)
	}
	defer mgr.Close()

	var wg sync.WaitGroup
	workers := 50
	entriesPerWorker := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < entriesPerWorker; j++ {
				mgr.LogAccess(AccessLogEntry{
					Timestamp:  time.Now(),
					ClientIP:   "127.0.0.1",
					Method:     "GET",
					Path:       "/ping",
					StatusCode: 200,
				}, "")
			}
		}(i)
	}
	wg.Wait()

	f, err := os.Open(accessLog)
	if err != nil {
		t.Fatalf("failed to open access log: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := 0
	for scanner.Scan() {
		lines++
	}

	expected := workers * entriesPerWorker
	if lines != expected {
		t.Fatalf("expected %d log lines, got %d", expected, lines)
	}
}

type mockConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (m *mockConn) RemoteAddr() net.Addr {
	return m.remoteAddr
}

type mockAddr struct {
	addr string
}

func (a *mockAddr) Network() string { return "tcp" }
func (a *mockAddr) String() string  { return a.addr }

func TestExtractClientIP(t *testing.T) {
	req, _ := httpparser.NewRequest("GET", "/", "HTTP/1.1")
	req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
	if ip := ExtractClientIP(req); ip != "203.0.113.195" {
		t.Errorf("expected 203.0.113.195, got %q", ip)
	}

	req2, _ := httpparser.NewRequest("GET", "/", "HTTP/1.1")
	req2.Header.Set("X-Real-IP", "198.51.100.4")
	if ip := ExtractClientIP(req2); ip != "198.51.100.4" {
		t.Errorf("expected 198.51.100.4, got %q", ip)
	}

	req3, _ := httpparser.NewRequest("GET", "/", "HTTP/1.1")
	req3.RawConn = &mockConn{remoteAddr: &mockAddr{addr: "192.168.1.50:54321"}}
	if ip := ExtractClientIP(req3); ip != "192.168.1.50" {
		t.Errorf("expected 192.168.1.50, got %q", ip)
	}
}

func TestSanitizeLogField(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
			expected: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
		},
		{
			name:     "crlf injection in user agent",
			input:    "Mozilla/5.0\r\n127.0.0.1 - - [01/Jan/2026] \"GET /admin HTTP/1.1\" 200 0",
			expected: "Mozilla/5.0  127.0.0.1 - - [01/Jan/2026] \"GET /admin HTTP/1.1\" 200 0",
		},
		{
			name:     "tabs, bells, and null bytes",
			input:    "evil\x00path\x07with\ttab\x7fdel",
			expected: "evil path with tab del",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeLogField(tc.input)
			if got != tc.expected {
				t.Errorf("SanitizeLogField(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestLogAccess_CRLFInjectionMitigation(t *testing.T) {
	tempDir := t.TempDir()
	accessLog := filepath.Join(tempDir, "access.log")

	mgr, err := NewLogManager(Config{
		Level:     "INFO",
		Format:    "text",
		AccessLog: accessLog,
	})
	if err != nil {
		t.Fatalf("failed to create LogManager: %v", err)
	}
	defer mgr.Close()

	// Malicious access log entry attempting CRLF log forging
	entry := AccessLogEntry{
		Timestamp:  time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
		ClientIP:   "192.0.2.1",
		Method:     "GET\r\nINJECTED",
		Path:       "/api/v1/resource\r\n10.0.0.1 - - [05/Sep/2026:12:00:00 +0000] \"GET /admin HTTP/1.1\" 200 9999",
		Protocol:   "HTTP/1.1",
		StatusCode: 404,
		BytesSent:  128,
		Duration:   15 * time.Millisecond,
		UserAgent:  "EvilAgent/1.0\r\nForged-Header: attack",
		Referer:    "http://example.com/exploit\r\n",
	}

	mgr.LogAccess(entry, "")

	data, err := os.ReadFile(accessLog)
	if err != nil {
		t.Fatalf("failed to read access log: %v", err)
	}

	raw := string(data)
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")

	// Strictly exactly one physical line must be written
	if len(lines) != 1 {
		t.Fatalf("expected strictly 1 log line, got %d lines: %q", len(lines), raw)
	}

	// Line must not contain unescaped CR or LF
	if strings.Contains(lines[0], "\r") || strings.Contains(lines[0], "\n") {
		t.Fatalf("log line contains raw control characters: %q", lines[0])
	}
}

func BenchmarkSanitizeLogField_Clean(b *testing.B) {
	cleanUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SanitizeLogField(cleanUA)
	}
}
