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

// WAFConfig holds configuration for the WAF engine.
type WAFConfig struct {
	Enabled            bool     `json:"enabled" yaml:"enabled"`
	Mode               string   `json:"mode" yaml:"mode"` // "enforce" or "detection"
	AnomalyThreshold   int      `json:"anomaly_threshold" yaml:"anomaly_threshold"`
	MaxInspectBodySize int64    `json:"max_inspect_body_size" yaml:"max_inspect_body_size"`
	DisabledRules      []string `json:"disabled_rules" yaml:"disabled_rules"`
}

// DefaultConfig returns safe default WAF settings.
func DefaultConfig() WAFConfig {
	return WAFConfig{
		Enabled:            true,
		Mode:               "enforce",
		AnomalyThreshold:   5,
		MaxInspectBodySize: 64 * 1024, // 64 KB
		DisabledRules:      nil,
	}
}

// WAFEngine executes inspection rules against incoming HTTP requests.
type WAFEngine struct {
	config WAFConfig
	rules  []WAFRule
	mu     sync.RWMutex
}

// NewEngine initializes a WAFEngine with default OWASP rules and configuration.
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

	disabledMap := make(map[string]bool)
	for _, id := range cfg.DisabledRules {
		disabledMap[id] = true
	}

	allRules := defaultRules()
	activeRules := make([]WAFRule, 0, len(allRules))
	for _, r := range allRules {
		if !disabledMap[r.ID] {
			activeRules = append(activeRules, r)
		}
	}

	return &WAFEngine{
		config: cfg,
		rules:  activeRules,
	}, nil
}

// Inspect evaluates a stdlib *http.Request against active WAF rules.
func (e *WAFEngine) Inspect(req *http.Request) (blocked bool, score int, matched []WAFRule, err error) {
	if !e.config.Enabled || req == nil {
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
	if !e.config.Enabled || req == nil {
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
	e.mu.RLock()
	defer e.mu.RUnlock()

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
