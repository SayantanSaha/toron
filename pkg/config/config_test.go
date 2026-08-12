package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"toron/pkg/config"
)

func TestConfig_DefaultValues(t *testing.T) {
	cfg, err := config.LoadFromFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Static.Dir != "./public" {
		t.Errorf("expected default static dir './public', got %q", cfg.Static.Dir)
	}
}

func TestConfig_YAMLFileLoading(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "config.yaml")

	yamlData := `
server:
  host: "127.0.0.1"
  port: 9090
  worker_pool_size: 256
  read_timeout: 10s
  write_timeout: 10s

static:
  enabled: true
  prefix: "/assets"
  dir: "./custom-assets"

logging:
  level: "debug"
`
	if err := os.WriteFile(yamlPath, []byte(yamlData), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	cfg, err := config.LoadFromFile(yamlPath)
	if err != nil {
		t.Fatalf("failed to load yaml config: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.WorkerPoolSize != 256 {
		t.Errorf("expected worker pool 256, got %d", cfg.Server.WorkerPoolSize)
	}
	if cfg.Server.ReadTimeout != 10*time.Second {
		t.Errorf("expected read timeout 10s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Static.Prefix != "/assets" {
		t.Errorf("expected static prefix '/assets', got %q", cfg.Static.Prefix)
	}
	if cfg.Static.Dir != "./custom-assets" {
		t.Errorf("expected static dir './custom-assets', got %q", cfg.Static.Dir)
	}
}

func TestConfig_UnsupportedExtension(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "config.unknown")
	_ = os.WriteFile(invalidPath, []byte("data"), 0644)

	_, err := config.LoadFromFile(invalidPath)
	if err == nil {
		t.Error("expected error for unsupported extension, got nil")
	}
}

func TestConfig_ProxyLoadBalancer(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "routes.yaml")

	yamlData := `
proxy:
  enabled: true
  routes:
    - prefix: "/api"
      target: "http://backend1:8080"
      targets:
        - "http://backend2:8080"
        - "http://backend3:8080"
      algorithm: "round_robin"
`
	_ = os.WriteFile(yamlPath, []byte(yamlData), 0644)

	cfg, err := config.LoadFromFiles("", yamlPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Proxy.Routes) != 1 {
		t.Fatalf("expected 1 proxy route, got %d", len(cfg.Proxy.Routes))
	}

	route := cfg.Proxy.Routes[0]
	targets := route.GetTargets()
	if len(targets) != 3 {
		t.Errorf("expected 3 targets, got %d (%v)", len(targets), targets)
	}
	if targets[0] != "http://backend1:8080" || targets[1] != "http://backend2:8080" || targets[2] != "http://backend3:8080" {
		t.Errorf("unexpected target ordering: %v", targets)
	}

	if route.GetAlgorithm() != "round_robin" {
		t.Errorf("expected round_robin algorithm, got %q", route.GetAlgorithm())
	}
}

func TestConfig_DualFileLoading(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	routesPath := filepath.Join(tmpDir, "routes.yaml")

	configYaml := `
server:
  port: 8080
`
	routesYaml := `
proxy:
  enabled: true
  routes:
    - prefix: "/api"
      target: "http://localhost:9001"
`
	_ = os.WriteFile(configPath, []byte(configYaml), 0644)
	_ = os.WriteFile(routesPath, []byte(routesYaml), 0644)

	cfg, err := config.LoadFromFiles(configPath, routesPath)
	if err != nil {
		t.Fatalf("failed to load dual configs: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if !cfg.Proxy.Enabled {
		t.Error("expected proxy enabled true")
	}
	if len(cfg.Proxy.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Proxy.Routes))
	}
}

func TestConfig_DefaultConfigYamlLoading(t *testing.T) {
	cfg, err := config.LoadFromFiles("../../config.yaml", "../../routes.yaml")
	if err != nil {
		t.Fatalf("failed to load root config.yaml & routes.yaml: %v", err)
	}

	if !cfg.Proxy.Enabled {
		t.Error("expected proxy.enabled to be true in routes.yaml")
	}
	if len(cfg.Proxy.Routes) != 5 {
		t.Errorf("expected 5 proxy routes in routes.yaml, got %d", len(cfg.Proxy.Routes))
	}
}
