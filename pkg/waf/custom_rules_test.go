package waf

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"toron/pkg/httpparser"
)

func TestWAF_ParseLocations(t *testing.T) {
	// Default when empty
	all := ParseLocations(nil)
	if all&(InspectURL|InspectQuery|InspectHeaders|InspectBody) == 0 {
		t.Fatalf("expected all locations mask, got %d", all)
	}

	// Specific locations
	loc1 := ParseLocations([]string{"url", "headers"})
	if loc1&InspectURL == 0 || loc1&InspectHeaders == 0 {
		t.Fatalf("expected InspectURL and InspectHeaders, got %d", loc1)
	}
	if loc1&InspectQuery != 0 || loc1&InspectBody != 0 {
		t.Fatalf("unexpected Query or Body in mask %d", loc1)
	}

	loc2 := ParseLocations([]string{"path", "query", "body"})
	if loc2&InspectURL == 0 || loc2&InspectQuery == 0 || loc2&InspectBody == 0 {
		t.Fatalf("expected URL, Query, Body in mask %d", loc2)
	}
}

func TestWAF_CompileCustomRule_Validation(t *testing.T) {
	// Empty ID
	_, err := CompileCustomRule(CustomRuleConfig{
		ID:      "",
		Pattern: "test",
	})
	if err == nil {
		t.Fatalf("expected error for empty rule ID")
	}

	// Empty Pattern
	_, err = CompileCustomRule(CustomRuleConfig{
		ID:      "CUSTOM-001",
		Pattern: "",
	})
	if err == nil {
		t.Fatalf("expected error for empty pattern")
	}

	// Invalid Regex
	_, err = CompileCustomRule(CustomRuleConfig{
		ID:      "CUSTOM-001",
		Pattern: "(?i)[unclosed_regex",
	})
	if err == nil {
		t.Fatalf("expected error for invalid regex pattern")
	}

	// Valid rule with defaults
	rule, err := CompileCustomRule(CustomRuleConfig{
		ID:          "CUSTOM-001",
		Description: "Detect bad bot",
		Pattern:     "(?i)(sqlmap|nikto)",
	})
	if err != nil {
		t.Fatalf("unexpected error compiling valid custom rule: %v", err)
	}
	if rule.ID != "CUSTOM-001" || rule.Category != CategoryCustom || rule.Score != 5 {
		t.Fatalf("expected default category custom and score 5, got %+v", rule)
	}
}

func TestWAF_CustomRegexRules_Locations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = "enforce"
	cfg.AnomalyThreshold = 5
	cfg.CustomRules = []CustomRuleConfig{
		{
			ID:          "CUSTOM-001",
			Category:    "bot",
			Description: "Block malicious scanner User-Agents",
			Pattern:     `(?i)(sqlmap|nikto|nmap|acunetix)`,
			Score:       10,
			Locations:   []string{"headers"},
		},
		{
			ID:          "CUSTOM-002",
			Category:    "data_leak",
			Description: "Inspect sensitive body keyword",
			Pattern:     `(?i)credit_card_leak`,
			Score:       10,
			Locations:   []string{"body"},
		},
		{
			ID:          "CUSTOM-003",
			Category:    "debug_leak",
			Description: "Inspect query debug parameter",
			Pattern:     `(?i)admin_debug=1`,
			Score:       10,
			Locations:   []string{"query"},
		},
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create WAFEngine with custom rules: %v", err)
	}

	// 1. Headers match for CUSTOM-001
	req1 := httptest.NewRequest("GET", "/api/v1/users", nil)
	req1.Header.Set("User-Agent", "sqlmap/1.5.2#stable")
	blocked, score, matched, err := engine.Inspect(req1)
	if err != nil || !blocked || score < 10 || len(matched) == 0 || matched[0].ID != "CUSTOM-001" {
		t.Fatalf("expected CUSTOM-001 to block request via headers, blocked=%v score=%d matched=%+v err=%v", blocked, score, matched, err)
	}

	// 2. Query containing "sqlmap" should NOT match CUSTOM-001 since locations only has "headers"
	req2 := httptest.NewRequest("GET", "/api/search?q=sqlmap", nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0")
	blocked, score, matched, _ = engine.Inspect(req2)
	if blocked || score != 0 || len(matched) != 0 {
		t.Fatalf("expected no match for sqlmap in query when rule is headers-only, blocked=%v score=%d matched=%+v", blocked, score, matched)
	}

	// 3. Body match for CUSTOM-002
	bodyPayload := []byte(`{"data":"something credit_card_leak here"}`)
	req3 := httptest.NewRequest("POST", "/api/v1/checkout", bytes.NewReader(bodyPayload))
	req3.Header.Set("Content-Type", "application/json")
	blocked, score, matched, _ = engine.Inspect(req3)
	if !blocked || score < 10 || len(matched) == 0 || matched[0].ID != "CUSTOM-002" {
		t.Fatalf("expected CUSTOM-002 to block POST body, blocked=%v score=%d matched=%+v", blocked, score, matched)
	}

	// 4. Query match for CUSTOM-003 via Toron httpparser.Request
	toronReq := &httpparser.Request{
		Method:     "GET",
		Path:       "/api/dashboard",
		RequestURI: "/api/dashboard?admin_debug=1",
		Header:     make(map[string][]string),
	}
	blocked, score, matched, _ = engine.InspectToron(toronReq)
	if !blocked || score < 10 || len(matched) == 0 || matched[0].ID != "CUSTOM-003" {
		t.Fatalf("expected CUSTOM-003 to block Toron request with query debug param, blocked=%v score=%d matched=%+v", blocked, score, matched)
	}
}

func TestWAF_CustomRule_DisabledRules(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = "enforce"
	cfg.DisabledRules = []string{"CUSTOM-001"}
	cfg.CustomRules = []CustomRuleConfig{
		{
			ID:        "CUSTOM-001",
			Pattern:   `(?i)blockme`,
			Score:     10,
			Locations: []string{"headers"},
		},
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to initialize engine: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "blockme")
	blocked, score, matched, _ := engine.Inspect(req)
	if blocked || score != 0 || len(matched) != 0 {
		t.Fatalf("expected disabled custom rule not to trigger, blocked=%v score=%d matched=%+v", blocked, score, matched)
	}
}
