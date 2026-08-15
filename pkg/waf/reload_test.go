package waf

import (
	"fmt"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWAF_AtomicReload(t *testing.T) {
	// 1. Initial engine with no custom rules
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = "enforce"
	cfg.AnomalyThreshold = 5

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to initialize WAFEngine: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("User-Agent", "sqlmap/1.5")
	blocked, score, _, _ := engine.Inspect(req)
	if blocked || score != 0 {
		t.Fatalf("expected initial clean pass before custom rule reload")
	}

	// 2. Reload with custom bot rule
	newCfg := cfg
	newCfg.CustomRules = []CustomRuleConfig{
		{
			ID:        "CUSTOM-001",
			Category:  "bot",
			Pattern:   `(?i)sqlmap`,
			Score:     10,
			Locations: []string{"headers"},
		},
	}

	if err := engine.Reload(newCfg); err != nil {
		t.Fatalf("failed to reload WAFEngine: %v", err)
	}

	blocked, score, matched, _ := engine.Inspect(req)
	if !blocked || score < 10 || len(matched) == 0 || matched[0].ID != "CUSTOM-001" {
		t.Fatalf("expected request blocked after reloading custom rules, blocked=%v score=%d", blocked, score)
	}

	// 3. Reload with invalid regex returns error and preserves state
	invalidCfg := newCfg
	invalidCfg.CustomRules = []CustomRuleConfig{
		{
			ID:      "INVALID-001",
			Pattern: "(?i)[broken_regex",
		},
	}
	if err := engine.Reload(invalidCfg); err == nil {
		t.Fatalf("expected error reloading invalid regex")
	}

	// Verify engine is still working and retains previous rule
	blocked, score, matched, _ = engine.Inspect(req)
	if !blocked || score < 10 || len(matched) == 0 {
		t.Fatalf("expected engine to retain previous valid state after failed reload")
	}
}

func TestWAF_AtomicReload_Concurrent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = "enforce"
	cfg.AnomalyThreshold = 5

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	var stopFlag int32
	var wg sync.WaitGroup

	// Spin up 20 reader goroutines evaluating requests concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for atomic.LoadInt32(&stopFlag) == 0 {
				req := httptest.NewRequest("GET", fmt.Sprintf("/api/item/%d", workerID), nil)
				req.Header.Set("User-Agent", "bot-scanner")
				_, _, _, _ = engine.Inspect(req)
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Simultaneously perform 10 reloads
	for r := 0; r < 10; r++ {
		reloadCfg := cfg
		reloadCfg.CustomRules = []CustomRuleConfig{
			{
				ID:        fmt.Sprintf("CUSTOM-RELOAD-%d", r),
				Category:  "bot",
				Pattern:   `(?i)bot-scanner`,
				Score:     10,
				Locations: []string{"headers"},
			},
		}
		if err := engine.Reload(reloadCfg); err != nil {
			t.Errorf("reload #%d failed: %v", r, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	atomic.StoreInt32(&stopFlag, 1)
	wg.Wait()
}
