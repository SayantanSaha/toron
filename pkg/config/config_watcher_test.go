package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConfigWatcher_HotReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "toron-config-watcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `server:
  port: 8080
  worker_pool_size: 64
  waf:
    enabled: true
    mode: "enforce"
    anomaly_threshold: 5
    custom_rules: []
`
	if err := os.WriteFile(configPath, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	var mu sync.Mutex
	var reloadedCfg *AppConfig
	reloadCount := 0

	watcher, err := NewConfigWatcher(configPath, "", func(cfg *AppConfig) {
		mu.Lock()
		reloadedCfg = cfg
		reloadCount++
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("failed to create ConfigWatcher: %v", err)
	}
	defer watcher.Stop()

	// Wait briefly for watcher setup
	time.Sleep(100 * time.Millisecond)

	// Update config.yaml with custom rule
	updatedYAML := `server:
  port: 8080
  worker_pool_size: 64
  waf:
    enabled: true
    mode: "enforce"
    anomaly_threshold: 5
    custom_rules:
      - id: "CUSTOM-001"
        category: "bot"
        description: "Block malicious scrapers"
        pattern: "(?i)(sqlmap|nikto|nmap)"
        score: 10
        locations: ["headers"]
`
	if err := os.WriteFile(configPath, []byte(updatedYAML), 0644); err != nil {
		t.Fatalf("failed to update config file: %v", err)
	}

	// Wait for debounced reload (100ms debouncer)
	time.Sleep(350 * time.Millisecond)

	mu.Lock()
	count := reloadCount
	cfg := reloadedCfg
	mu.Unlock()

	if count == 0 || cfg == nil {
		t.Fatalf("expected ConfigWatcher to trigger reload, count=%d cfg=%+v", count, cfg)
	}

	if len(cfg.Server.WAF.CustomRules) != 1 || cfg.Server.WAF.CustomRules[0].ID != "CUSTOM-001" {
		t.Fatalf("expected reloaded config to have CUSTOM-001, got %+v", cfg.Server.WAF.CustomRules)
	}
}

func TestConfigWatcher_InvalidConfigSafePreserve(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "toron-config-watcher-invalid-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `server:
  port: 8080
  worker_pool_size: 64
`
	if err := os.WriteFile(configPath, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	reloadCount := 0
	var mu sync.Mutex

	watcher, err := NewConfigWatcher(configPath, "", func(cfg *AppConfig) {
		mu.Lock()
		reloadCount++
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("failed to create ConfigWatcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(100 * time.Millisecond)

	// Write invalid YAML with bad regex
	invalidYAML := `server:
  port: 8080
  worker_pool_size: 64
  waf:
    enabled: true
    custom_rules:
      - id: "INVALID"
        pattern: "(?i)[broken_regex"
`
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	time.Sleep(350 * time.Millisecond)

	mu.Lock()
	count := reloadCount
	mu.Unlock()

	if count != 0 {
		t.Fatalf("expected invalid config NOT to trigger onReload, got count=%d", count)
	}
}
