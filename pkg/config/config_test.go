package config_test

import (
	"encoding/json"
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
	if len(cfg.Proxy.Routes) < 10 {
		t.Errorf("expected at least 10 routes in routes.yaml, got %d", len(cfg.Proxy.Routes))
	}
	if !cfg.Proxy.Routes[0].IsStatic() {
		t.Errorf("expected first route to be a static site route")
	}
}

func TestConfig_ValidateConfig(t *testing.T) {
	// 1. Valid config
	cfg := config.DefaultAppConfig()
	cfg.Static.Enabled = false
	if err := config.ValidateConfig(cfg); err != nil {
		t.Errorf("expected valid default config, got error: %v", err)
	}

	// 2. Invalid port
	cfgInvalidPort := config.DefaultAppConfig()
	cfgInvalidPort.Server.Port = 70000
	if err := config.ValidateConfig(cfgInvalidPort); err == nil {
		t.Error("expected error for port 70000, got nil")
	}

	// 3. Invalid target URL
	cfgInvalidTarget := config.DefaultAppConfig()
	cfgInvalidTarget.Proxy.Enabled = true
	cfgInvalidTarget.Proxy.Routes = []config.ProxyRouteConfig{
		{
			Prefix: "/api",
			Target: "not-a-valid-url",
		},
	}
	if err := config.ValidateConfig(cfgInvalidTarget); err == nil {
		t.Error("expected error for invalid target URL, got nil")
	}
}

func TestConfig_ParseRateLimit(t *testing.T) {
	rateSec, burstSec, err := config.ParseRateLimit("10/sec")
	if err != nil || rateSec != 10.0 || burstSec != 10 {
		t.Errorf("expected 10/sec -> (10.0, 10), got (%v, %d, %v)", rateSec, burstSec, err)
	}

	rateMin, burstMin, err := config.ParseRateLimit("120/min")
	if err != nil || rateMin != 2.0 || burstMin != 120 {
		t.Errorf("expected 120/min -> (2.0, 120), got (%v, %d, %v)", rateMin, burstMin, err)
	}

	_, _, errInvalid := config.ParseRateLimit("abc/invalid")
	if errInvalid == nil {
		t.Errorf("expected error for invalid rate limit format, got nil")
	}
}

func TestConfig_SPARouteConfig(t *testing.T) {
	tmpDir := t.TempDir()
	routesPath := filepath.Join(tmpDir, "routes.yaml")

	yamlData := `
proxy:
  enabled: true
  routes:
    - prefix: "/kite"
      type: static
      dir: "/var/www/kite"
      spa: true

    - prefix: "/custom"
      type: static
      dir: "/var/www/custom"
      spa: true
      fallback: "app.html"

    - prefix: "/portal"
      type: static
      dir: "/var/www/portal"
      fallback: "portal.html"

    - prefix: "/legacy"
      type: static
      dir: "/var/www/legacy"
      spa: false
`
	if err := os.WriteFile(routesPath, []byte(yamlData), 0644); err != nil {
		t.Fatalf("failed to write routes yaml: %v", err)
	}

	cfg, err := config.LoadFromFiles("", routesPath)
	if err != nil {
		t.Fatalf("failed to load routes file: %v", err)
	}

	if len(cfg.Proxy.Routes) != 4 {
		t.Fatalf("expected 4 proxy routes, got %d", len(cfg.Proxy.Routes))
	}

	r0 := cfg.Proxy.Routes[0]
	if !r0.SPA {
		t.Errorf("route 0: expected SPA true, got false")
	}
	if r0.Fallback != "" {
		t.Errorf("route 0: expected fallback empty, got %q", r0.Fallback)
	}

	r1 := cfg.Proxy.Routes[1]
	if !r1.SPA {
		t.Errorf("route 1: expected SPA true, got false")
	}
	if r1.Fallback != "app.html" {
		t.Errorf("route 1: expected fallback 'app.html', got %q", r1.Fallback)
	}

	r2 := cfg.Proxy.Routes[2]
	if r2.Fallback != "portal.html" {
		t.Errorf("route 2: expected fallback 'portal.html', got %q", r2.Fallback)
	}

	r3 := cfg.Proxy.Routes[3]
	if r3.SPA {
		t.Errorf("route 3: expected SPA false, got true")
	}
	if r3.Fallback != "" {
		t.Errorf("route 3: expected fallback empty, got %q", r3.Fallback)
	}

	// Also test LoadRoutesFromFile directly
	routes, err := config.LoadRoutesFromFile(routesPath)
	if err != nil {
		t.Fatalf("failed to load routes from file: %v", err)
	}
	if len(routes) != 4 {
		t.Fatalf("expected 4 routes, got %d", len(routes))
	}
	if !routes[0].SPA || routes[1].Fallback != "app.html" || routes[2].Fallback != "portal.html" || routes[3].SPA {
		t.Errorf("unexpected route unmarshaling from LoadRoutesFromFile")
	}

	// Test JSON unmarshaling
	jsonData := `[
		{"prefix": "/json-spa", "type": "static", "dir": "/var/www/json", "spa": true, "fallback": "index.html"}
	]`
	var jsonRoutes []config.ProxyRouteConfig
	if err := json.Unmarshal([]byte(jsonData), &jsonRoutes); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if len(jsonRoutes) != 1 || !jsonRoutes[0].SPA || jsonRoutes[0].Fallback != "index.html" {
		t.Errorf("unexpected JSON routes unmarshaling: %+v", jsonRoutes)
	}
}

func TestConfig_HTTPRedirectAndRouteOverride(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	routesPath := filepath.Join(tmpDir, "routes.yaml")

	cfgData := `
server:
  port: 8443
  tls:
    enabled: true
  http_redirect:
    enabled: true
    port: 8080
`
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	challengeDir := filepath.Join(tmpDir, "challenges")
	appDir := filepath.Join(tmpDir, "app")
	_ = os.MkdirAll(challengeDir, 0755)
	_ = os.MkdirAll(appDir, 0755)

	routesData := `
routes:
  - type: "static"
    prefix: "/.well-known/acme-challenge"
    dir: "` + challengeDir + `"
    redirect_http: false
  - type: "static"
    prefix: "/app"
    dir: "` + appDir + `"
`
	if err := os.WriteFile(routesPath, []byte(routesData), 0644); err != nil {
		t.Fatalf("failed to write routes.yaml: %v", err)
	}

	cfg, err := config.LoadFromFiles(cfgPath, routesPath)
	if err != nil {
		t.Fatalf("failed to load config files: %v", err)
	}

	if !cfg.Server.HTTPRedirect.Enabled {
		t.Errorf("expected Server.HTTPRedirect.Enabled to be true")
	}
	if cfg.Server.HTTPRedirect.Port != 8080 {
		t.Errorf("expected Server.HTTPRedirect.Port to be 8080, got %d", cfg.Server.HTTPRedirect.Port)
	}

	srvCfg := cfg.ToServerConfig()
	if !srvCfg.HTTPRedirectEnabled {
		t.Errorf("expected srvCfg.HTTPRedirectEnabled to be true")
	}
	if srvCfg.HTTPRedirectPort != 8080 {
		t.Errorf("expected srvCfg.HTTPRedirectPort to be 8080, got %d", srvCfg.HTTPRedirectPort)
	}
	if srvCfg.HTTPSPort != 8443 {
		t.Errorf("expected srvCfg.HTTPSPort to be 8443, got %d", srvCfg.HTTPSPort)
	}

	if len(cfg.Proxy.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(cfg.Proxy.Routes))
	}

	r0 := cfg.Proxy.Routes[0]
	if r0.RedirectHTTP == nil || *r0.RedirectHTTP != false {
		t.Errorf("route 0: expected RedirectHTTP false, got %v", r0.RedirectHTTP)
	}
	if r0.ShouldRedirectHTTP() {
		t.Errorf("route 0: expected ShouldRedirectHTTP false, got true")
	}

	r1 := cfg.Proxy.Routes[1]
	if r1.RedirectHTTP != nil {
		t.Errorf("route 1: expected RedirectHTTP nil, got %v", r1.RedirectHTTP)
	}
	if !r1.ShouldRedirectHTTP() {
		t.Errorf("route 1: expected ShouldRedirectHTTP true, got false")
	}
}
