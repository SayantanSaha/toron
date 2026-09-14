package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"toron/pkg/config"
	"toron/pkg/server"
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
	if cfg.Sidecar.MaxBodyBytes != 10*1024*1024 {
		t.Errorf("expected default sidecar max body bytes 10MB, got %d", cfg.Sidecar.MaxBodyBytes)
	}
	if cfg.Sidecar.InsecureSkipVerify != false {
		t.Errorf("expected default sidecar InsecureSkipVerify false, got %v", cfg.Sidecar.InsecureSkipVerify)
	}
}

func TestConfig_SidecarMaxBodyBytes(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "config.yaml")

	yamlData := `
sidecar:
  enabled: true
  max_body_bytes: 2097152
`
	if err := os.WriteFile(yamlPath, []byte(yamlData), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	cfg, err := config.LoadFromFile(yamlPath)
	if err != nil {
		t.Fatalf("failed to load yaml config: %v", err)
	}

	if cfg.Sidecar.MaxBodyBytes != 2097152 {
		t.Errorf("expected sidecar max body bytes 2097152, got %d", cfg.Sidecar.MaxBodyBytes)
	}
}

func TestConfig_SidecarInsecureSkipVerify(t *testing.T) {
	tmpDir := t.TempDir()

	// Default case (omitted in YAML)
	yamlPathDefault := filepath.Join(tmpDir, "config_default.yaml")
	yamlDataDefault := `
sidecar:
  enabled: true
`
	if err := os.WriteFile(yamlPathDefault, []byte(yamlDataDefault), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	cfgDefault, err := config.LoadFromFile(yamlPathDefault)
	if err != nil {
		t.Fatalf("failed to load yaml config: %v", err)
	}

	if cfgDefault.Sidecar.InsecureSkipVerify != false {
		t.Errorf("expected sidecar InsecureSkipVerify to default to false, got %v", cfgDefault.Sidecar.InsecureSkipVerify)
	}

	// Explicit true case
	yamlPathOptIn := filepath.Join(tmpDir, "config_optin.yaml")
	yamlDataOptIn := `
sidecar:
  enabled: true
  insecure_skip_verify: true
`
	if err := os.WriteFile(yamlPathOptIn, []byte(yamlDataOptIn), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	cfgOptIn, err := config.LoadFromFile(yamlPathOptIn)
	if err != nil {
		t.Fatalf("failed to load yaml config: %v", err)
	}

	if cfgOptIn.Sidecar.InsecureSkipVerify != true {
		t.Errorf("expected sidecar InsecureSkipVerify true when specified in YAML, got %v", cfgOptIn.Sidecar.InsecureSkipVerify)
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

func TestConfig_LoggingConfigAndRouteOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	routesPath := filepath.Join(tmpDir, "routes.yaml")

	cfgData := `
server:
  port: 8080

logging:
  level: "warn"
  format: "json"
  server_log: "logs/custom_server.log"
  access_log: "logs/custom_access.log"
  security_log: "logs/custom_security.log"
`
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	routesData := `
routes:
  - type: "upstream"
    prefix: "/api"
    target: "http://localhost:9001"
    access_log: "logs/api_access.log"
    security_log: "logs/api_security.log"

  - type: "static"
    prefix: "/silent"
    dir: "./public"
    access_log: "off"

  - type: "static"
    prefix: "/default"
    dir: "./public"
`
	if err := os.WriteFile(routesPath, []byte(routesData), 0644); err != nil {
		t.Fatalf("failed to write routes.yaml: %v", err)
	}

	appCfg, err := config.LoadFromFiles(cfgPath, routesPath)
	if err != nil {
		t.Fatalf("failed to load configs: %v", err)
	}

	// 1. Verify Global Logging Config
	if appCfg.Logging.Level != "warn" {
		t.Errorf("expected level 'warn', got %q", appCfg.Logging.Level)
	}
	if appCfg.Logging.Format != "json" {
		t.Errorf("expected format 'json', got %q", appCfg.Logging.Format)
	}
	if appCfg.Logging.ServerLog != "logs/custom_server.log" {
		t.Errorf("expected server_log 'logs/custom_server.log', got %q", appCfg.Logging.ServerLog)
	}
	if appCfg.Logging.AccessLog != "logs/custom_access.log" {
		t.Errorf("expected access_log 'logs/custom_access.log', got %q", appCfg.Logging.AccessLog)
	}
	if appCfg.Logging.SecurityLog != "logs/custom_security.log" {
		t.Errorf("expected security_log 'logs/custom_security.log', got %q", appCfg.Logging.SecurityLog)
	}

	// 2. Verify Route Overrides
	if len(appCfg.Proxy.Routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(appCfg.Proxy.Routes))
	}

	r0 := appCfg.Proxy.Routes[0]
	if r0.AccessLog != "logs/api_access.log" {
		t.Errorf("route 0 access_log: expected 'logs/api_access.log', got %q", r0.AccessLog)
	}
	if r0.SecurityLog != "logs/api_security.log" {
		t.Errorf("route 0 security_log: expected 'logs/api_security.log', got %q", r0.SecurityLog)
	}
	if r0.GetAccessLog("default.log") != "logs/api_access.log" {
		t.Errorf("route 0 GetAccessLog: expected 'logs/api_access.log', got %q", r0.GetAccessLog("default.log"))
	}
	if r0.GetSecurityLog("default.log") != "logs/api_security.log" {
		t.Errorf("route 0 GetSecurityLog: expected 'logs/api_security.log', got %q", r0.GetSecurityLog("default.log"))
	}

	r1 := appCfg.Proxy.Routes[1]
	if r1.AccessLog != "off" {
		t.Errorf("route 1 access_log: expected 'off', got %q", r1.AccessLog)
	}

	r2 := appCfg.Proxy.Routes[2]
	if r2.AccessLog != "" {
		t.Errorf("route 2 access_log: expected empty, got %q", r2.AccessLog)
	}
	if r2.GetAccessLog("default.log") != "default.log" {
		t.Errorf("route 2 GetAccessLog: expected fallback 'default.log', got %q", r2.GetAccessLog("default.log"))
	}
}

func TestConfig_ValidateCORS(t *testing.T) {
	// 1. Insecure configuration: AllowCredentials: true and AllowOrigins: ["*"]
	insecureCORS := config.CORSConfig{
		Enabled:          true,
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}
	if err := config.ValidateCORS(insecureCORS); err == nil {
		t.Fatal("expected error when AllowCredentials is true with wildcard origin, got nil")
	}

	// 2. Insecure configuration inside AppConfig ValidateConfig
	cfg := config.DefaultAppConfig()
	cfg.Static.Enabled = false
	cfg.Server.CORS = insecureCORS
	if err := config.ValidateConfig(cfg); err == nil {
		t.Fatal("expected ValidateConfig to reject server.cors with wildcard credentials")
	}

	// 3. Valid CORS configuration with explicit origins and credentials
	validCORS := config.CORSConfig{
		Enabled:          true,
		AllowOrigins:     []string{"https://app.example.com"},
		AllowCredentials: true,
	}
	if err := config.ValidateCORS(validCORS); err != nil {
		t.Fatalf("expected valid CORS config, got: %v", err)
	}
}

func TestConfig_Layer4ProxySettings(t *testing.T) {
	// Defaults when values are unset or non-positive
	routeDefault := config.ProxyRouteConfig{}
	if routeDefault.GetMaxConnections() != 10000 {
		t.Errorf("expected default MaxConnections 10000, got %d", routeDefault.GetMaxConnections())
	}
	if routeDefault.GetIdleTimeout() != 60*time.Second {
		t.Errorf("expected default IdleTimeout 60s, got %v", routeDefault.GetIdleTimeout())
	}
	if routeDefault.GetMaxWorkers() != 1024 {
		t.Errorf("expected default MaxWorkers 1024, got %d", routeDefault.GetMaxWorkers())
	}

	// Custom configured values
	routeCustom := config.ProxyRouteConfig{
		MaxConnections: 500,
		IdleTimeout:    15 * time.Second,
		MaxWorkers:     128,
	}
	if routeCustom.GetMaxConnections() != 500 {
		t.Errorf("expected custom MaxConnections 500, got %d", routeCustom.GetMaxConnections())
	}
	if routeCustom.GetIdleTimeout() != 15*time.Second {
		t.Errorf("expected custom IdleTimeout 15s, got %v", routeCustom.GetIdleTimeout())
	}
	if routeCustom.GetMaxWorkers() != 128 {
		t.Errorf("expected custom MaxWorkers 128, got %d", routeCustom.GetMaxWorkers())
	}
}

func TestConfig_UpgradeIdleTimeoutSchemaAndFallback(t *testing.T) {
	t.Run("Subtest 1A (Default Invariants)", func(t *testing.T) {
		srvDef := server.DefaultConfig()
		if srvDef.UpgradeIdleTimeout != 60*time.Second {
			t.Errorf("expected server.DefaultConfig().UpgradeIdleTimeout 60s, got %v", srvDef.UpgradeIdleTimeout)
		}

		appDef := config.DefaultAppConfig()
		if appDef.Server.UpgradeIdleTimeout != 60*time.Second {
			t.Errorf("expected DefaultAppConfig().Server.UpgradeIdleTimeout 60s, got %v", appDef.Server.UpgradeIdleTimeout)
		}
	})

	t.Run("Subtest 1B (YAML & JSON Deserialization)", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlPath := filepath.Join(tmpDir, "config.yaml")
		yamlContent := `
server:
  upgrade_idle_timeout: 45s
`
		if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("failed to write test yaml: %v", err)
		}

		loadedCfg, err := config.LoadFromFile(yamlPath)
		if err != nil {
			t.Fatalf("failed to load yaml: %v", err)
		}
		if loadedCfg.Server.UpgradeIdleTimeout != 45*time.Second {
			t.Errorf("expected YAML UpgradeIdleTimeout 45s, got %v", loadedCfg.Server.UpgradeIdleTimeout)
		}

		jsonPayload := []byte(`{"server": {"upgrade_idle_timeout": 30000000000}}`)
		var jsonCfg config.AppConfig
		if err := json.Unmarshal(jsonPayload, &jsonCfg); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		if jsonCfg.Server.UpgradeIdleTimeout != 30*time.Second {
			t.Errorf("expected JSON UpgradeIdleTimeout 30s, got %v", jsonCfg.Server.UpgradeIdleTimeout)
		}
	})

	t.Run("Subtest 1C (Hierarchical Fallback Resolution)", func(t *testing.T) {
		// Tier 1: Explicit UpgradeIdleTimeout > 0
		cfgTier1 := config.DefaultAppConfig()
		cfgTier1.Server.UpgradeIdleTimeout = 15 * time.Second
		cfgTier1.Server.IdleTimeout = 30 * time.Second
		srvCfg1 := cfgTier1.ToServerConfig()
		if srvCfg1.UpgradeIdleTimeout != 15*time.Second {
			t.Errorf("Tier 1: expected 15s, got %v", srvCfg1.UpgradeIdleTimeout)
		}

		// Tier 2: UpgradeIdleTimeout <= 0, IdleTimeout > 0
		cfgTier2 := config.DefaultAppConfig()
		cfgTier2.Server.UpgradeIdleTimeout = 0
		cfgTier2.Server.IdleTimeout = 25 * time.Second
		srvCfg2 := cfgTier2.ToServerConfig()
		if srvCfg2.UpgradeIdleTimeout != 25*time.Second {
			t.Errorf("Tier 2: expected 25s, got %v", srvCfg2.UpgradeIdleTimeout)
		}

		// Tier 3: Both <= 0 -> 60s fallback
		cfgTier3 := config.DefaultAppConfig()
		cfgTier3.Server.UpgradeIdleTimeout = 0
		cfgTier3.Server.IdleTimeout = 0
		srvCfg3 := cfgTier3.ToServerConfig()
		if srvCfg3.UpgradeIdleTimeout != 60*time.Second {
			t.Errorf("Tier 3: expected 60s, got %v", srvCfg3.UpgradeIdleTimeout)
		}
	})

	t.Run("Subtest 1D (Negative Value Rejection)", func(t *testing.T) {
		cfg := config.DefaultAppConfig()
		cfg.Static.Enabled = false
		cfg.Server.UpgradeIdleTimeout = -5 * time.Second

		err := config.ValidateConfig(cfg)
		if err == nil {
			t.Fatal("expected ValidateConfig to return error for negative upgrade_idle_timeout, got nil")
		}
		if !strings.Contains(err.Error(), "server.upgrade_idle_timeout must be non-negative") {
			t.Errorf("expected error containing 'server.upgrade_idle_timeout must be non-negative', got: %v", err)
		}
	})
}

// TC-089-03: Configuration Schema, Default Fallback & Loader Validation (SEC-28)
func TestTranscoderConfig_MaxBodyBytesSchemaAndValidation(t *testing.T) {
	t.Run("Subtest 3A (Default and Fallback)", func(t *testing.T) {
		// Zero value returns default 4MB
		cfgZero := config.TranscoderConfig{MaxBodyBytes: 0}
		if got := cfgZero.GetMaxBodyBytes(); got != 4*1024*1024 {
			t.Errorf("expected default 4MB (4194304), got %d", got)
		}

		// Negative value returns default 4MB
		cfgNeg := config.TranscoderConfig{MaxBodyBytes: -100}
		if got := cfgNeg.GetMaxBodyBytes(); got != 4*1024*1024 {
			t.Errorf("expected default 4MB (4194304) for negative, got %d", got)
		}

		// Custom positive value returns exact value
		cfgCustom := config.TranscoderConfig{MaxBodyBytes: 8 * 1024 * 1024}
		if got := cfgCustom.GetMaxBodyBytes(); got != 8*1024*1024 {
			t.Errorf("expected 8MB (8388608), got %d", got)
		}
	})

	t.Run("Subtest 3B (YAML and JSON Deserialization)", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "config.yaml")
		yamlData := `
transcoder:
  enabled: true
  max_body_bytes: 2097152
`
		if err := os.WriteFile(cfgPath, []byte(yamlData), 0644); err != nil {
			t.Fatalf("failed to write config yaml: %v", err)
		}

		appCfg, err := config.LoadFromFiles(cfgPath, "")
		if err != nil {
			t.Fatalf("failed to load YAML config: %v", err)
		}
		if appCfg.Transcoder.MaxBodyBytes != 2097152 {
			t.Errorf("expected YAML MaxBodyBytes 2097152, got %d", appCfg.Transcoder.MaxBodyBytes)
		}

		jsonData := `{"transcoder":{"enabled":true,"max_body_bytes":1048576}}`
		var jsonCfg config.AppConfig
		if err := json.Unmarshal([]byte(jsonData), &jsonCfg); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		if jsonCfg.Transcoder.MaxBodyBytes != 1048576 {
			t.Errorf("expected JSON MaxBodyBytes 1048576, got %d", jsonCfg.Transcoder.MaxBodyBytes)
		}
	})

	t.Run("Subtest 3C (Negative Value Rejection in ValidateConfig)", func(t *testing.T) {
		cfg := config.DefaultAppConfig()
		cfg.Static.Enabled = false
		cfg.Transcoder.MaxBodyBytes = -500

		err := config.ValidateConfig(cfg)
		if err == nil {
			t.Fatal("expected ValidateConfig error for negative transcoder.max_body_bytes, got nil")
		}
		if !strings.Contains(err.Error(), "transcoder.max_body_bytes must be non-negative") {
			t.Errorf("expected error containing 'transcoder.max_body_bytes must be non-negative', got: %v", err)
		}
	})
}

// TC-123: Verification of Configurable Upstream Reverse Proxy Transport Architecture
func TestConfig_ProxyTransportConfig_HierarchyAndDefaults(t *testing.T) {
	t.Run("TC-123.1/8: Default Raw Speed and Balanced Presets", func(t *testing.T) {
		raw := config.DefaultProxyTransportConfig("raw_speed")
		if raw.MaxIdleConns != 10000 || raw.MaxIdleConnsPerHost != 1000 || raw.MaxConnsPerHost != 0 {
			t.Errorf("unexpected raw_speed pool values: %+v", raw)
		}
		if raw.IdleConnTimeout != 90*time.Second {
			t.Errorf("unexpected raw_speed idle timeout: %v", raw.IdleConnTimeout)
		}
		if raw.DisableCompression == nil || !*raw.DisableCompression {
			t.Errorf("expected raw_speed DisableCompression to be true")
		}
		if raw.UseEnvProxy == nil || *raw.UseEnvProxy {
			t.Errorf("expected raw_speed UseEnvProxy to be false")
		}
		if raw.PropagateUpstreamClose == nil || *raw.PropagateUpstreamClose {
			t.Errorf("expected raw_speed PropagateUpstreamClose to be false")
		}
		if raw.ForceAttemptHTTP2 == nil || *raw.ForceAttemptHTTP2 {
			t.Errorf("expected raw_speed ForceAttemptHTTP2 to be false")
		}
		if raw.Tracing == nil || *raw.Tracing {
			t.Errorf("expected raw_speed Tracing to be false")
		}

		balanced := config.DefaultProxyTransportConfig("balanced")
		if balanced.MaxIdleConns != 1000 || balanced.MaxIdleConnsPerHost != 100 || balanced.MaxConnsPerHost != 200 {
			t.Errorf("unexpected balanced pool values: %+v", balanced)
		}
		if balanced.IdleConnTimeout != 30*time.Second {
			t.Errorf("unexpected balanced idle timeout: %v", balanced.IdleConnTimeout)
		}
		if balanced.DisableCompression == nil || *balanced.DisableCompression {
			t.Errorf("expected balanced DisableCompression to be false")
		}
		if balanced.UseEnvProxy == nil || !*balanced.UseEnvProxy {
			t.Errorf("expected balanced UseEnvProxy to be true")
		}
		if balanced.PropagateUpstreamClose == nil || !*balanced.PropagateUpstreamClose {
			t.Errorf("expected balanced PropagateUpstreamClose to be true")
		}
		if balanced.ForceAttemptHTTP2 == nil || !*balanced.ForceAttemptHTTP2 {
			t.Errorf("expected balanced ForceAttemptHTTP2 to be true")
		}
		if balanced.Tracing == nil || !*balanced.Tracing {
			t.Errorf("expected balanced Tracing to be true")
		}
	})

	t.Run("TC-123.2: Hierarchical Override Resolution", func(t *testing.T) {
		global := config.ProxyTransportConfig{
			Profile:             "raw_speed",
			MaxIdleConnsPerHost: 500,
		}

		// Route without override inherits global
		routeDefault := config.ProxyRouteConfig{Prefix: "/api"}
		resolved := routeDefault.ResolveTransport(global)
		if resolved.MaxIdleConnsPerHost != 500 {
			t.Errorf("expected inherited MaxIdleConnsPerHost 500, got %d", resolved.MaxIdleConnsPerHost)
		}
		if resolved.MaxIdleConns != 10000 {
			t.Errorf("expected default MaxIdleConns 10000, got %d", resolved.MaxIdleConns)
		}

		// Route with override takes precedence
		maxConns := 25
		routeOverridden := config.ProxyRouteConfig{
			Prefix: "/slow",
			Transport: &config.ProxyTransportConfig{
				MaxConnsPerHost: maxConns,
			},
		}
		resolvedOverride := routeOverridden.ResolveTransport(global)
		if resolvedOverride.MaxConnsPerHost != 25 {
			t.Errorf("expected overridden MaxConnsPerHost 25, got %d", resolvedOverride.MaxConnsPerHost)
		}
		if resolvedOverride.MaxIdleConnsPerHost != 500 {
			t.Errorf("expected inherited MaxIdleConnsPerHost 500, got %d", resolvedOverride.MaxIdleConnsPerHost)
		}
	})

	t.Run("TC-123.3: YAML and JSON Deserialization", func(t *testing.T) {
		yamlData := `
proxy:
  enabled: true
  transport:
    profile: "balanced"
    max_idle_conns_per_host: 250
    proxy_url: "http://proxy.corp:3128"
  routes:
    - prefix: "/custom"
      targets: ["http://backend:8080"]
      transport:
        max_conns_per_host: 42
        disable_compression: true
`
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(cfgPath, []byte(yamlData), 0644); err != nil {
			t.Fatalf("failed to write config yaml: %v", err)
		}

		appCfg, err := config.LoadFromFiles(cfgPath, "")
		if err != nil {
			t.Fatalf("failed to load YAML config: %v", err)
		}

		if appCfg.Proxy.Transport.Profile != "balanced" {
			t.Errorf("expected global profile balanced, got %q", appCfg.Proxy.Transport.Profile)
		}
		if appCfg.Proxy.Transport.MaxIdleConnsPerHost != 250 {
			t.Errorf("expected global MaxIdleConnsPerHost 250, got %d", appCfg.Proxy.Transport.MaxIdleConnsPerHost)
		}
		if appCfg.Proxy.Transport.ProxyURL != "http://proxy.corp:3128" {
			t.Errorf("expected global ProxyURL 'http://proxy.corp:3128', got %q", appCfg.Proxy.Transport.ProxyURL)
		}

		route := appCfg.Proxy.Routes[0]
		if route.Transport == nil {
			t.Fatal("expected route Transport to be populated, got nil")
		}
		if route.Transport.MaxConnsPerHost != 42 {
			t.Errorf("expected route MaxConnsPerHost 42, got %d", route.Transport.MaxConnsPerHost)
		}

		resolved := route.ResolveTransport(appCfg.Proxy.Transport)
		if resolved.MaxConnsPerHost != 42 {
			t.Errorf("expected resolved MaxConnsPerHost 42, got %d", resolved.MaxConnsPerHost)
		}
		if resolved.MaxIdleConnsPerHost != 250 {
			t.Errorf("expected resolved MaxIdleConnsPerHost 250, got %d", resolved.MaxIdleConnsPerHost)
		}
		if resolved.ProxyURL != "http://proxy.corp:3128" {
			t.Errorf("expected resolved ProxyURL 'http://proxy.corp:3128', got %q", resolved.ProxyURL)
		}
		if resolved.DisableCompression == nil || !*resolved.DisableCompression {
			t.Errorf("expected resolved DisableCompression to be true")
		}
	})

	t.Run("TC-124.1/2: Tracing Configuration Defaults and Overrides", func(t *testing.T) {
		global := config.ProxyTransportConfig{
			Profile: "raw_speed",
		}
		// In raw_speed, tracing defaults to false
		routeDef := config.ProxyRouteConfig{Prefix: "/api"}
		resDef := routeDef.ResolveTransport(global)
		if resDef.Tracing == nil || *resDef.Tracing {
			t.Errorf("expected default raw_speed tracing to be false, got: %v", resDef.Tracing)
		}

		// Route override enables tracing
		trTrue := true
		routeWithTracing := config.ProxyRouteConfig{
			Prefix: "/traced",
			Transport: &config.ProxyTransportConfig{
				Tracing: &trTrue,
			},
		}
		resTraced := routeWithTracing.ResolveTransport(global)
		if resTraced.Tracing == nil || !*resTraced.Tracing {
			t.Errorf("expected route override tracing to be true")
		}

		// Balanced profile defaults tracing to true
		globalBalanced := config.ProxyTransportConfig{
			Profile: "balanced",
		}
		resBalanced := routeDef.ResolveTransport(globalBalanced)
		if resBalanced.Tracing == nil || !*resBalanced.Tracing {
			t.Errorf("expected default balanced tracing to be true")
		}

		// Route override disables tracing under balanced profile
		trFalse := false
		routeNoTracing := config.ProxyRouteConfig{
			Prefix: "/fast",
			Transport: &config.ProxyTransportConfig{
				Tracing: &trFalse,
			},
		}
		resNoTracing := routeNoTracing.ResolveTransport(globalBalanced)
		if resNoTracing.Tracing == nil || *resNoTracing.Tracing {
			t.Errorf("expected route override tracing to be false under balanced profile")
		}
	})

	t.Run("Validation: Invalid ProxyURL Rejection", func(t *testing.T) {
		cfg := config.DefaultAppConfig()
		cfg.Static.Enabled = false
		cfg.Proxy.Enabled = true
		cfg.Proxy.Transport.ProxyURL = "invalid-url-without-scheme"

		err := config.ValidateConfig(cfg)
		if err == nil {
			t.Fatal("expected ValidateConfig error for invalid proxy.transport.proxy_url, got nil")
		}
		if !strings.Contains(err.Error(), "proxy.transport.proxy_url") {
			t.Errorf("expected error containing 'proxy.transport.proxy_url', got: %v", err)
		}
	})
}


