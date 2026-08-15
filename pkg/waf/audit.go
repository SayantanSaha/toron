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

// AuditLogger writes structured security audit events to a configured sink and maintains a recent events buffer.
type AuditLogger struct {
	cfg          AuditLogConfig
	writer       io.Writer
	closer       io.Closer
	recentEvents []SecurityEvent
	maxEvents    int
	mu           sync.Mutex
}

// NewAuditLogger initializes an AuditLogger from configuration.
func NewAuditLogger(cfg AuditLogConfig) (*AuditLogger, error) {
	if !cfg.Enabled {
		return &AuditLogger{cfg: cfg, maxEvents: 50}, nil
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
		cfg:          cfg,
		writer:       w,
		closer:       c,
		recentEvents: make([]SecurityEvent, 0, 50),
		maxEvents:    50,
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
		writer:       w,
		recentEvents: make([]SecurityEvent, 0, 50),
		maxEvents:    50,
	}
}

// LogEvent writes a SecurityEvent as a single JSON line and stores it in the recent events buffer.
func (l *AuditLogger) LogEvent(event SecurityEvent) {
	if l == nil || !l.cfg.Enabled {
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

	l.recentEvents = append(l.recentEvents, event)
	if len(l.recentEvents) > l.maxEvents {
		l.recentEvents = l.recentEvents[len(l.recentEvents)-l.maxEvents:]
	}

	if l.writer != nil {
		_, _ = l.writer.Write(append(data, '\n'))
	}
}

// GetRecentEvents returns up to 'limit' most recent security events, sorted newest first.
func (l *AuditLogger) GetRecentEvents(limit int) []SecurityEvent {
	if l == nil {
		return []SecurityEvent{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	n := len(l.recentEvents)
	if n == 0 {
		return []SecurityEvent{}
	}

	if limit <= 0 || limit > n {
		limit = n
	}

	out := make([]SecurityEvent, limit)
	copy(out, l.recentEvents[n-limit:])

	// Reverse so newest event is first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
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
