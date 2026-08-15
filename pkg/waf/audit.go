package waf

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// AuditLogConfig configures security audit logging.
type AuditLogConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Output  string `json:"output" yaml:"output"` // "stdout", "stderr", or file path
	Format  string `json:"format" yaml:"format"` // "json"
}

// DefaultAuditLogConfig returns default security audit log configuration.
func DefaultAuditLogConfig() AuditLogConfig {
	return AuditLogConfig{
		Enabled: true,
		Output:  "stdout",
		Format:  "json",
	}
}

// SecurityEvent records details of an intercepted threat or anomaly.
type SecurityEvent struct {
	Timestamp      string `json:"timestamp"`
	Event          string `json:"event"`
	ClientIP       string `json:"client_ip"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	Category       string `json:"category"`
	RuleID         string `json:"rule_id,omitempty"`
	AnomalyScore   int    `json:"anomaly_score"`
	Action         string `json:"action"` // "blocked" or "logged"
	Location       string `json:"location,omitempty"`
	PayloadSnippet string `json:"payload_snippet,omitempty"`
}

// AuditLogger writes structured security audit events to a configured sink.
type AuditLogger struct {
	cfg    AuditLogConfig
	writer io.Writer
	closer io.Closer
	mu     sync.Mutex
}

// NewAuditLogger initializes an AuditLogger from configuration.
func NewAuditLogger(cfg AuditLogConfig) (*AuditLogger, error) {
	if !cfg.Enabled {
		return &AuditLogger{cfg: cfg}, nil
	}

	var w io.Writer
	var c io.Closer

	switch cfg.Output {
	case "", "stdout":
		w = os.Stdout
	case "stderr":
		w = os.Stderr
	default:
		// File output
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		w = f
		c = f
	}

	return &AuditLogger{
		cfg:    cfg,
		writer: w,
		closer: c,
	}, nil
}

// NewAuditLoggerWithWriter creates an AuditLogger writing directly to a custom io.Writer (useful for testing).
func NewAuditLoggerWithWriter(w io.Writer) *AuditLogger {
	return &AuditLogger{
		cfg: AuditLogConfig{
			Enabled: true,
			Output:  "custom",
			Format:  "json",
		},
		writer: w,
	}
}

// LogEvent writes a SecurityEvent as a single JSON line.
func (l *AuditLogger) LogEvent(event SecurityEvent) {
	if l == nil || !l.cfg.Enabled || l.writer == nil {
		return
	}

	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Truncate payload snippet to 128 chars to prevent unbounded memory growth
	if len(event.PayloadSnippet) > 128 {
		event.PayloadSnippet = event.PayloadSnippet[:128] + "..."
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.writer.Write(append(data, '\n'))
}

// Close closes any underlying open file handles.
func (l *AuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}
