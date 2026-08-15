package waf

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditLogger_LogEventJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLoggerWithWriter(&buf)

	event := SecurityEvent{
		Event:          "waf_block",
		ClientIP:       "198.51.100.25",
		Method:         "GET",
		Path:           "/api/users",
		Category:       "sqli",
		RuleID:         "SQLI-001",
		AnomalyScore:   5,
		Action:         "blocked",
		Location:       "query",
		PayloadSnippet: "1 UNION SELECT username, password FROM users",
	}

	logger.LogEvent(event)

	line := buf.String()
	if !strings.HasSuffix(line, "\n") {
		t.Error("expected log line to end with newline")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &parsed); err != nil {
		t.Fatalf("failed to parse emitted JSON log: %v", err)
	}

	if parsed["event"] != "waf_block" {
		t.Errorf("expected event waf_block, got %v", parsed["event"])
	}
	if parsed["client_ip"] != "198.51.100.25" {
		t.Errorf("expected client_ip 198.51.100.25, got %v", parsed["client_ip"])
	}
	if parsed["rule_id"] != "SQLI-001" {
		t.Errorf("expected rule_id SQLI-001, got %v", parsed["rule_id"])
	}
	if parsed["timestamp"] == nil || parsed["timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestAuditLogger_PayloadTruncation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLoggerWithWriter(&buf)

	hugePayload := strings.Repeat("A", 300)
	logger.LogEvent(SecurityEvent{
		Event:          "waf_block",
		ClientIP:       "10.0.0.1",
		PayloadSnippet: hugePayload,
	})

	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &parsed)

	snippet, ok := parsed["payload_snippet"].(string)
	if !ok {
		t.Fatal("payload_snippet is missing or not a string")
	}
	if len(snippet) > 140 { // 128 + "..."
		t.Errorf("expected payload snippet to be truncated, got length %d", len(snippet))
	}
	if !strings.HasSuffix(snippet, "...") {
		t.Errorf("expected truncated snippet to end with '...', got: %s", snippet)
	}
}

func TestAuditLogger_FileSink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "toron-audit-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFilePath := filepath.Join(tmpDir, "security.log")

	logger, err := NewAuditLogger(AuditLogConfig{
		Enabled: true,
		Output:  logFilePath,
		Format:  "json",
	})
	if err != nil {
		t.Fatalf("failed to init file audit logger: %v", err)
	}

	logger.LogEvent(SecurityEvent{
		Event:        "ip_acl_block",
		ClientIP:     "10.99.1.50",
		Category:     "ip_acl",
		Action:       "blocked",
		AnomalyScore: 0,
	})
	_ = logger.Close()

	content, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("failed to read written log file: %v", err)
	}

	if !strings.Contains(string(content), "ip_acl_block") {
		t.Errorf("expected log file to contain ip_acl_block, got: %s", string(content))
	}
}
