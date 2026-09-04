package waf

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"toron/pkg/httpparser"
)

// WAFCategory represents the type of security threat.
type WAFCategory string

const (
	CategorySQLi      WAFCategory = "sqli"
	CategoryXSS       WAFCategory = "xss"
	CategoryTraversal WAFCategory = "traversal"
	CategoryRCE       WAFCategory = "rce"
	CategoryBot       WAFCategory = "bot"
	CategoryCustom    WAFCategory = "custom"
)

// InspectLocation flags where the rule should be checked.
type InspectLocation int

const (
	InspectURL InspectLocation = 1 << iota
	InspectQuery
	InspectHeaders
	InspectBody
)

// WAFRule defines a single inspection rule.
type WAFRule struct {
	ID          string
	Category    WAFCategory
	Description string
	Pattern     *regexp.Regexp
	Score       int
	Locations   InspectLocation
}

// CustomRuleConfig defines a user-configurable regex rule in YAML/JSON.
type CustomRuleConfig struct {
	ID          string   `json:"id" yaml:"id"`
	Category    string   `json:"category" yaml:"category"`
	Description string   `json:"description" yaml:"description"`
	Pattern     string   `json:"pattern" yaml:"pattern"`
	Score       int      `json:"score" yaml:"score"`
	Locations   []string `json:"locations" yaml:"locations"`
}

// WAFConfig holds configuration for the WAF engine.
type WAFConfig struct {
	Enabled            bool               `json:"enabled" yaml:"enabled"`
	Mode               string             `json:"mode" yaml:"mode"` // "enforce" or "detection"
	AnomalyThreshold   int                `json:"anomaly_threshold" yaml:"anomaly_threshold"`
	MaxInspectBodySize int64              `json:"max_inspect_body_size" yaml:"max_inspect_body_size"`
	DisabledRules      []string           `json:"disabled_rules" yaml:"disabled_rules"`
	AllowedIPs         []string           `json:"allowed_ips" yaml:"allowed_ips"`
	DeniedIPs          []string           `json:"denied_ips" yaml:"denied_ips"`
	Excluded           []string           `json:"excluded" yaml:"excluded"`
	CustomRules        []CustomRuleConfig `json:"custom_rules" yaml:"custom_rules"`
	AuditLog           AuditLogConfig     `json:"audit_log" yaml:"audit_log"`
}

// DefaultConfig returns safe default WAF settings.
func DefaultConfig() WAFConfig {
	return WAFConfig{
		Enabled:            true,
		Mode:               "enforce",
		AnomalyThreshold:   5,
		MaxInspectBodySize: 64 * 1024, // 64 KB
		DisabledRules:      nil,
		AllowedIPs:         nil,
		DeniedIPs:          nil,
		Excluded:           nil,
		CustomRules:        nil,
		AuditLog:           DefaultAuditLogConfig(),
	}
}

// ParseLocations maps string location identifiers to an InspectLocation bitmask.
func ParseLocations(locs []string) InspectLocation {
	if len(locs) == 0 {
		return InspectURL | InspectQuery | InspectHeaders | InspectBody
	}
	var mask InspectLocation
	for _, l := range locs {
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "url", "path":
			mask |= InspectURL
		case "query":
			mask |= InspectQuery
		case "headers", "header":
			mask |= InspectHeaders
		case "body":
			mask |= InspectBody
		}
	}
	if mask == 0 {
		return InspectURL | InspectQuery | InspectHeaders | InspectBody
	}
	return mask
}

// CompileCustomRule compiles a CustomRuleConfig into an active WAFRule.
func CompileCustomRule(c CustomRuleConfig) (WAFRule, error) {
	if strings.TrimSpace(c.ID) == "" {
		return WAFRule{}, fmt.Errorf("custom WAF rule id cannot be empty")
	}
	if strings.TrimSpace(c.Pattern) == "" {
		return WAFRule{}, fmt.Errorf("custom WAF rule %q pattern cannot be empty", c.ID)
	}
	re, err := regexp.Compile(c.Pattern)
	if err != nil {
		return WAFRule{}, fmt.Errorf("custom WAF rule %q invalid regex %q: %w", c.ID, c.Pattern, err)
	}

	cat := WAFCategory(strings.ToLower(strings.TrimSpace(c.Category)))
	if cat == "" {
		cat = CategoryCustom
	}

	score := c.Score
	if score <= 0 {
		score = 5
	}

	return WAFRule{
		ID:          c.ID,
		Category:    cat,
		Description: c.Description,
		Pattern:     re,
		Score:       score,
		Locations:   ParseLocations(c.Locations),
	}, nil
}

func buildActiveRules(cfg WAFConfig) ([]WAFRule, error) {
	disabledMap := make(map[string]bool)
	for _, id := range cfg.DisabledRules {
		disabledMap[id] = true
	}

	allRules := defaultRules()
	activeRules := make([]WAFRule, 0, len(allRules)+len(cfg.CustomRules))
	for _, r := range allRules {
		if !disabledMap[r.ID] {
			activeRules = append(activeRules, r)
		}
	}

	for _, cr := range cfg.CustomRules {
		if disabledMap[cr.ID] {
			continue
		}
		compiled, err := CompileCustomRule(cr)
		if err != nil {
			return nil, err
		}
		activeRules = append(activeRules, compiled)
	}

	return activeRules, nil
}

// WAFEngine executes inspection rules against incoming HTTP requests.
type WAFEngine struct {
	config      WAFConfig
	rules       []WAFRule
	ipACL       *IPAccessList
	auditLogger *AuditLogger
	mu          sync.RWMutex
}

// NewEngine initializes a WAFEngine with default OWASP rules, custom rules, IP ACLs, and configuration.
func NewEngine(cfg WAFConfig) (*WAFEngine, error) {
	if cfg.AnomalyThreshold <= 0 {
		cfg.AnomalyThreshold = 5
	}
	if cfg.MaxInspectBodySize <= 0 {
		cfg.MaxInspectBodySize = 64 * 1024
	}
	if cfg.Mode == "" {
		cfg.Mode = "enforce"
	}

	acl, err := NewIPAccessList(cfg.AllowedIPs, cfg.DeniedIPs)
	if err != nil {
		return nil, err
	}

	var logger *AuditLogger
	if cfg.AuditLog.Enabled {
		logger, err = NewAuditLogger(cfg.AuditLog)
		if err != nil {
			return nil, err
		}
	}

	activeRules, err := buildActiveRules(cfg)
	if err != nil {
		return nil, err
	}

	return &WAFEngine{
		config:      cfg,
		rules:       activeRules,
		ipACL:       acl,
		auditLogger: logger,
	}, nil
}

// Reload dynamically updates the WAF engine configuration and rule sets atomically under mutex lock.
func (e *WAFEngine) Reload(cfg WAFConfig) error {
	if cfg.AnomalyThreshold <= 0 {
		cfg.AnomalyThreshold = 5
	}
	if cfg.MaxInspectBodySize <= 0 {
		cfg.MaxInspectBodySize = 64 * 1024
	}
	if cfg.Mode == "" {
		cfg.Mode = "enforce"
	}

	acl, err := NewIPAccessList(cfg.AllowedIPs, cfg.DeniedIPs)
	if err != nil {
		return fmt.Errorf("failed to reload WAF IP ACLs: %w", err)
	}

	activeRules, err := buildActiveRules(cfg)
	if err != nil {
		return fmt.Errorf("failed to reload WAF custom rules: %w", err)
	}

	var logger *AuditLogger
	if cfg.AuditLog.Enabled {
		logger, err = NewAuditLogger(cfg.AuditLog)
		if err != nil {
			return fmt.Errorf("failed to reload WAF audit logger: %w", err)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.config = cfg
	e.rules = activeRules
	e.ipACL = acl
	if cfg.AuditLog.Enabled {
		e.auditLogger = logger
	}

	return nil
}

// Rules returns a copy of the currently active WAF rules.
func (e *WAFEngine) Rules() []WAFRule {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make([]WAFRule, len(e.rules))
	copy(res, e.rules)
	return res
}

// Inspect evaluates a stdlib *http.Request against active WAF rules.
func (e *WAFEngine) Inspect(req *http.Request) (blocked bool, score int, matched []WAFRule, err error) {
	if e == nil || req == nil {
		return false, 0, nil, nil
	}

	urlPath := ""
	rawQuery := ""
	if req.URL != nil {
		urlPath = req.URL.Path
		rawQuery = req.URL.RawQuery
	}

	var headersCombined strings.Builder
	for k, vals := range req.Header {
		for _, v := range vals {
			headersCombined.WriteString(k)
			headersCombined.WriteString(": ")
			headersCombined.WriteString(v)
			headersCombined.WriteString("\n")
		}
	}

	var bodyReader io.Reader = req.Body
	restoreBody := func(b []byte) {
		req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(b), req.Body))
	}

	return e.inspectInternal(req.Method, urlPath, rawQuery, headersCombined.String(), bodyReader, restoreBody)
}

// InspectToron evaluates a Toron *httpparser.Request against active WAF rules.
func (e *WAFEngine) InspectToron(req *httpparser.Request) (blocked bool, score int, matched []WAFRule, err error) {
	if e == nil || req == nil {
		return false, 0, nil, nil
	}

	urlPath := req.Path
	rawQuery := ""
	if req.URL != nil {
		rawQuery = req.URL.RawQuery
	} else if req.RequestURI != "" {
		if u, parseErr := url.ParseRequestURI(req.RequestURI); parseErr == nil {
			rawQuery = u.RawQuery
			if urlPath == "" {
				urlPath = u.Path
			}
		}
	}

	var headersCombined strings.Builder
	for k, vals := range req.Header {
		for _, v := range vals {
			headersCombined.WriteString(k)
			headersCombined.WriteString(": ")
			headersCombined.WriteString(v)
			headersCombined.WriteString("\n")
		}
	}

	var bodyReader io.Reader = req.Body
	restoreBody := func(b []byte) {
		req.Body = io.MultiReader(bytes.NewReader(b), req.Body)
	}

	return e.inspectInternal(req.Method, urlPath, rawQuery, headersCombined.String(), bodyReader, restoreBody)
}

func (e *WAFEngine) inspectInternal(method, urlPath, rawQuery, headersStr string, bodyReader io.Reader, restoreBody func([]byte)) (blocked bool, score int, matched []WAFRule, err error) {
	if e == nil {
		return false, 0, nil, nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.config.Enabled {
		return false, 0, nil, nil
	}

	normPath := urlPath
	if unescaped, unErr := url.PathUnescape(urlPath); unErr == nil {
		normPath = unescaped
	}

	normQuery := rawQuery
	if unescaped, unErr := url.QueryUnescape(rawQuery); unErr == nil {
		normQuery = unescaped
	} else {
		normQuery = strings.ReplaceAll(rawQuery, "+", " ")
	}

	var bodyStr string
	if bodyReader != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(bodyReader, e.config.MaxInspectBodySize))
		if readErr == nil {
			bodyStr = string(bodyBytes)
			if restoreBody != nil {
				restoreBody(bodyBytes)
			}
		}
	}

	for _, rule := range e.rules {
		matchFound := false

		if (rule.Locations&InspectURL != 0) && normPath != "" && rule.Pattern.MatchString(normPath) {
			matchFound = true
		}
		if !matchFound && (rule.Locations&InspectQuery != 0) && normQuery != "" && rule.Pattern.MatchString(normQuery) {
			matchFound = true
		}
		if !matchFound && (rule.Locations&InspectHeaders != 0) && headersStr != "" && rule.Pattern.MatchString(headersStr) {
			matchFound = true
		}
		if !matchFound && (rule.Locations&InspectBody != 0) && bodyStr != "" && rule.Pattern.MatchString(bodyStr) {
			matchFound = true
		}

		if matchFound {
			score += rule.Score
			matched = append(matched, rule)
		}
	}

	if score >= e.config.AnomalyThreshold && strings.EqualFold(e.config.Mode, "enforce") {
		blocked = true
	}

	return blocked, score, matched, nil
}

// FormatBlockedResponse generates a standard 403 Forbidden payload for blocked requests.
func FormatBlockedResponse(score int, matched []WAFRule) string {
	var ruleIDs []string
	for _, r := range matched {
		ruleIDs = append(ruleIDs, r.ID)
	}
	return fmt.Sprintf(`{"error":"Forbidden","message":"WAF security violation detected","threat_score":%d,"triggered_rules":[%s]}`,
		score, `"`+strings.Join(ruleIDs, `","`)+`"`)
}

// Config returns the current engine configuration.
func (e *WAFEngine) Config() WAFConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}

// IPAccessList returns the compiled IP access list associated with the engine.
func (e *WAFEngine) IPAccessList() *IPAccessList {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ipACL
}

// AuditLogger returns the audit logger associated with the engine.
func (e *WAFEngine) AuditLogger() *AuditLogger {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.auditLogger
}

// SetAuditLogger overrides or sets the engine's audit logger.
func (e *WAFEngine) SetAuditLogger(l *AuditLogger) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.auditLogger = l
}
