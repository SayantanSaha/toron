package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

// Config defines configuration options for server, access, and security logs.
type Config struct {
	Level       string `json:"level" yaml:"level"`
	Format      string `json:"format" yaml:"format"` // "text" or "json"
	ServerLog   string `json:"server_log" yaml:"server_log"`
	AccessLog   string `json:"access_log" yaml:"access_log"`
	SecurityLog string `json:"security_log" yaml:"security_log"`
}

// AccessLogEntry captures transactional HTTP request and response execution telemetry.
type AccessLogEntry struct {
	Timestamp   time.Time     `json:"timestamp"`
	ClientIP    string        `json:"client_ip"`
	Method      string        `json:"method"`
	Path        string        `json:"path"`
	Protocol    string        `json:"protocol"`
	StatusCode  int           `json:"status"`
	Duration    time.Duration `json:"duration_ms"`
	BytesSent   int64         `json:"bytes_sent"`
	UserAgent   string        `json:"user_agent,omitempty"`
	Referer     string        `json:"referer,omitempty"`
	Host        string        `json:"host,omitempty"`
	RoutePrefix string        `json:"route,omitempty"`
}

// SecurityLogEntry captures security-related events for security logging.
type SecurityLogEntry struct {
	Timestamp      time.Time `json:"timestamp"`
	Event          string    `json:"event"`
	ClientIP       string    `json:"client_ip"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	Category       string    `json:"category"`
	RuleID         string    `json:"rule_id,omitempty"`
	AnomalyScore   int       `json:"anomaly_score"`
	Action         string    `json:"action"` // "blocked" or "logged"
	Location       string    `json:"location,omitempty"`
	PayloadSnippet string    `json:"payload_snippet,omitempty"`
	Message        string    `json:"message,omitempty"`
}

// LogSink represents a thread-safe, reopenable destination for log output.
type LogSink struct {
	path   string
	mu     sync.Mutex
	file   *os.File
	writer io.Writer
	isStd  bool
}

// NewLogSink creates or opens a LogSink for the specified path or standard stream.
func NewLogSink(path string) (*LogSink, error) {
	norm := strings.TrimSpace(path)
	if norm == "" || norm == "stdout" {
		return &LogSink{path: "stdout", writer: os.Stdout, isStd: true}, nil
	}
	if norm == "stderr" {
		return &LogSink{path: "stderr", writer: os.Stderr, isStd: true}, nil
	}

	dir := filepath.Dir(norm)
	if dir != "." && dir != "/" && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory for %q: %w", norm, err)
		}
	}

	f, err := os.OpenFile(norm, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %q: %w", norm, err)
	}

	return &LogSink{
		path:   norm,
		file:   f,
		writer: f,
		isStd:  false,
	}, nil
}

// Write writes data thread-safely to the sink.
func (s *LogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return 0, nil
	}
	return s.writer.Write(p)
}

// Reopen closes the current file handle and reopens it at the configured path in append mode.
// Enables seamless integration with system logrotators without dropping connections.
func (s *LogSink) Reopen() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isStd || s.path == "" {
		return nil
	}

	if s.file != nil {
		_ = s.file.Sync()
		_ = s.file.Close()
	}

	dir := filepath.Dir(s.path)
	if dir != "." && dir != "/" && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory on reopen for %q: %w", s.path, err)
		}
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to reopen log file %q: %w", s.path, err)
	}

	s.file = f
	s.writer = f
	return nil
}

// Close flushes and closes the sink file handle.
func (s *LogSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isStd || s.file == nil {
		return nil
	}

	err := s.file.Close()
	s.file = nil
	s.writer = nil
	return err
}

// Path returns the configured sink path.
func (s *LogSink) Path() string {
	return s.path
}

// LogManager manages multi-stream log sinks, per-route routing, and logrotate reopening.
type LogManager struct {
	cfg        Config
	mu         sync.RWMutex
	sinks      map[string]*LogSink
	serverSink *LogSink
	accessSink *LogSink
	secSink    *LogSink
}

// NewLogManager initializes a LogManager with the specified configuration.
func NewLogManager(cfg Config) (*LogManager, error) {
	if cfg.Level == "" {
		cfg.Level = "info"
	}
	if cfg.Format == "" {
		cfg.Format = "text"
	}

	m := &LogManager{
		cfg:   cfg,
		sinks: make(map[string]*LogSink),
	}

	// 1. Initialize Server Log Sink
	serverPath := strings.TrimSpace(cfg.ServerLog)
	if serverPath != "" && serverPath != "off" && serverPath != "none" {
		sink, err := m.getOrCreateSinkLocked(serverPath)
		if err != nil {
			return nil, fmt.Errorf("server_log error: %w", err)
		}
		m.serverSink = sink
	}

	// 2. Initialize Default Access Log Sink
	accessPath := strings.TrimSpace(cfg.AccessLog)
	if accessPath != "" && accessPath != "off" && accessPath != "none" {
		sink, err := m.getOrCreateSinkLocked(accessPath)
		if err != nil {
			return nil, fmt.Errorf("access_log error: %w", err)
		}
		m.accessSink = sink
	}

	// 3. Initialize Default Security Log Sink
	secPath := strings.TrimSpace(cfg.SecurityLog)
	if secPath != "" && secPath != "off" && secPath != "none" {
		sink, err := m.getOrCreateSinkLocked(secPath)
		if err != nil {
			return nil, fmt.Errorf("security_log error: %w", err)
		}
		m.secSink = sink
	}

	return m, nil
}

func (m *LogManager) getOrCreateSinkLocked(path string) (*LogSink, error) {
	norm := strings.TrimSpace(path)
	if norm == "" {
		return nil, nil
	}
	if sink, exists := m.sinks[norm]; exists {
		return sink, nil
	}
	sink, err := NewLogSink(norm)
	if err != nil {
		return nil, err
	}
	m.sinks[norm] = sink
	return sink, nil
}

// GetOrCreateSink retrieves an existing sink or opens a new one for the given path.
func (m *LogManager) GetOrCreateSink(path string) (*LogSink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getOrCreateSinkLocked(path)
}

// ServerWriter returns an io.Writer for server logging (suitable for log.SetOutput).
// Returns os.Stdout if no server log sink is configured.
func (m *LogManager) ServerWriter() io.Writer {
	if m == nil {
		return os.Stdout
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.serverSink != nil {
		return m.serverSink
	}
	return os.Stdout
}

// ServerSink returns the underlying LogSink for server logs.
func (m *LogManager) ServerSink() *LogSink {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serverSink
}

// AccessSink returns the default access LogSink.
func (m *LogManager) AccessSink() *LogSink {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accessSink
}

// SecuritySink returns the default security LogSink.
func (m *LogManager) SecuritySink() *LogSink {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secSink
}

// SecurityWriter returns an io.Writer for security audit logs for a given route override (or default).
func (m *LogManager) SecurityWriter(routeOverride string) io.Writer {
	if m == nil {
		return os.Stdout
	}
	override := strings.ToLower(strings.TrimSpace(routeOverride))
	if override == "off" || override == "none" {
		return io.Discard
	}
	if override != "" {
		if sink, err := m.GetOrCreateSink(routeOverride); err == nil {
			return sink
		}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.secSink != nil {
		return m.secSink
	}
	return os.Stdout
}

// LogAccess formats and writes an access log entry to the designated sink (route override or default).
func (m *LogManager) LogAccess(entry AccessLogEntry, routeOverride string) {
	if m == nil {
		return
	}

	override := strings.ToLower(strings.TrimSpace(routeOverride))
	if override == "off" || override == "none" {
		return
	}

	var targetSink *LogSink
	if override != "" {
		sink, err := m.GetOrCreateSink(routeOverride)
		if err != nil {
			return
		}
		targetSink = sink
	} else {
		m.mu.RLock()
		targetSink = m.accessSink
		m.mu.RUnlock()
	}

	if targetSink == nil {
		return
	}

	var line string
	if strings.ToLower(m.cfg.Format) == "json" {
		type jsonAccessEntry struct {
			Timestamp   string  `json:"timestamp"`
			ClientIP    string  `json:"client_ip"`
			Method      string  `json:"method"`
			Path        string  `json:"path"`
			Protocol    string  `json:"protocol"`
			StatusCode  int     `json:"status"`
			DurationMs  float64 `json:"duration_ms"`
			BytesSent   int64   `json:"bytes_sent"`
			UserAgent   string  `json:"user_agent,omitempty"`
			Referer     string  `json:"referer,omitempty"`
			Host        string  `json:"host,omitempty"`
			RoutePrefix string  `json:"route,omitempty"`
		}
		durationMs := float64(entry.Duration.Nanoseconds()) / 1e6
		jEntry := jsonAccessEntry{
			Timestamp:   entry.Timestamp.UTC().Format(time.RFC3339),
			ClientIP:    entry.ClientIP,
			Method:      entry.Method,
			Path:        entry.Path,
			Protocol:    entry.Protocol,
			StatusCode:  entry.StatusCode,
			DurationMs:  durationMs,
			BytesSent:   entry.BytesSent,
			UserAgent:   entry.UserAgent,
			Referer:     entry.Referer,
			Host:        entry.Host,
			RoutePrefix: entry.RoutePrefix,
		}
		data, err := json.Marshal(jEntry)
		if err != nil {
			return
		}
		line = string(data)
	} else {
		// Combined Log Format with latency appended
		proto := entry.Protocol
		if proto == "" {
			proto = "HTTP/1.1"
		}
		clientIP := entry.ClientIP
		if clientIP == "" {
			clientIP = "-"
		}
		referer := entry.Referer
		if referer == "" {
			referer = "-"
		}
		ua := entry.UserAgent
		if ua == "" {
			ua = "-"
		}
		ts := entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700")
		line = fmt.Sprintf(`%s - - [%s] "%s %s %s" %d %d "%s" "%s" %v`,
			clientIP, ts, entry.Method, entry.Path, proto, entry.StatusCode, entry.BytesSent, referer, ua, entry.Duration)
	}

	_, _ = targetSink.Write([]byte(line + "\n"))
}

// LogSecurity formats and writes a security log entry to the designated sink (route override or default).
func (m *LogManager) LogSecurity(entry SecurityLogEntry, routeOverride string) {
	if m == nil {
		return
	}

	override := strings.ToLower(strings.TrimSpace(routeOverride))
	if override == "off" || override == "none" {
		return
	}

	var targetSink *LogSink
	if override != "" {
		sink, err := m.GetOrCreateSink(routeOverride)
		if err != nil {
			return
		}
		targetSink = sink
	} else {
		m.mu.RLock()
		targetSink = m.secSink
		m.mu.RUnlock()
	}

	if targetSink == nil {
		return
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	type jsonSecEntry struct {
		Timestamp      string `json:"timestamp"`
		Event          string `json:"event"`
		ClientIP       string `json:"client_ip"`
		Method         string `json:"method"`
		Path           string `json:"path"`
		Category       string `json:"category"`
		RuleID         string `json:"rule_id,omitempty"`
		AnomalyScore   int    `json:"anomaly_score"`
		Action         string `json:"action"`
		Location       string `json:"location,omitempty"`
		PayloadSnippet string `json:"payload_snippet,omitempty"`
		Message        string `json:"message,omitempty"`
	}

	data, err := json.Marshal(jsonSecEntry{
		Timestamp:      entry.Timestamp.UTC().Format(time.RFC3339),
		Event:          entry.Event,
		ClientIP:       entry.ClientIP,
		Method:         entry.Method,
		Path:           entry.Path,
		Category:       entry.Category,
		RuleID:         entry.RuleID,
		AnomalyScore:   entry.AnomalyScore,
		Action:         entry.Action,
		Location:       entry.Location,
		PayloadSnippet: entry.PayloadSnippet,
		Message:        entry.Message,
	})
	if err != nil {
		return
	}

	_, _ = targetSink.Write(append(data, '\n'))
}

// Reopen closes and reopens all registered sinks. Thread-safe for SIGHUP log rotation.
func (m *LogManager) Reopen() error {
	m.mu.RLock()
	sinks := make([]*LogSink, 0, len(m.sinks))
	for _, s := range m.sinks {
		sinks = append(sinks, s)
	}
	m.mu.RUnlock()

	var firstErr error
	for _, s := range sinks {
		if err := s.Reopen(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Close flushes and closes all managed log sinks.
func (m *LogManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for _, s := range m.sinks {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ExtractClientIP extracts the client remote IP from an incoming request, inspecting socket and proxy headers.
func ExtractClientIP(req *httpparser.Request) string {
	if req == nil {
		return ""
	}
	if req.RawConn != nil {
		if remoteAddr := req.RawConn.RemoteAddr(); remoteAddr != nil {
			raw := remoteAddr.String()
			if host, _, err := net.SplitHostPort(raw); err == nil {
				return host
			}
			return raw
		}
	}
	if req.Header != nil {
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
				raw := strings.TrimSpace(parts[0])
				if host, _, err := net.SplitHostPort(raw); err == nil {
					return host
				}
				return raw
			}
		}
		if xri := req.Header.Get("X-Real-IP"); xri != "" {
			raw := strings.TrimSpace(xri)
			if host, _, err := net.SplitHostPort(raw); err == nil {
				return host
			}
			return raw
		}
	}
	return "127.0.0.1"
}
