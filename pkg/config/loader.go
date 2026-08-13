package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader defines an extensible interface for loading configuration from files.
type Loader interface {
	Load(data []byte, cfg *AppConfig) error
}

// YAMLLoader parses YAML configuration data.
type YAMLLoader struct{}

func (l *YAMLLoader) Load(data []byte, cfg *AppConfig) error {
	return yaml.Unmarshal(data, cfg)
}

// Manager handles format detection and configuration file loading.
type Manager struct {
	loaders map[string]Loader
}

// NewManager creates a Manager with registered file loaders.
func NewManager() *Manager {
	m := &Manager{
		loaders: make(map[string]Loader),
	}

	yamlLoader := &YAMLLoader{}
	m.Register(".yaml", yamlLoader)
	m.Register(".yml", yamlLoader)

	return m
}

// Register registers a new Loader for a file extension (e.g. ".json", ".toml").
func (m *Manager) Register(ext string, loader Loader) {
	m.loaders[strings.ToLower(ext)] = loader
}

// LoadFromFiles loads server infrastructure config from configPath and routing config from routesPath.
func (m *Manager) LoadFromFiles(configPath, routesPath string) (*AppConfig, error) {
	cfg := DefaultAppConfig()

	// 1. Load server configuration file
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("config: failed to read config file %s: %w", configPath, err)
		}

		ext := strings.ToLower(filepath.Ext(configPath))
		loader, exists := m.loaders[ext]
		if !exists {
			return nil, fmt.Errorf("config: unsupported config file extension %q (supported: .yaml, .yml)", ext)
		}

		if err := loader.Load(data, cfg); err != nil {
			return nil, fmt.Errorf("config: failed to parse config file %s: %w", configPath, err)
		}
	}

	// 2. Determine routesPath if not explicitly specified
	if routesPath == "" {
		if configPath != "" {
			dir := filepath.Dir(configPath)
			candidate := filepath.Join(dir, "routes.yaml")
			if _, err := os.Stat(candidate); err == nil {
				routesPath = candidate
			}
		}
		if routesPath == "" {
			if _, err := os.Stat("routes.yaml"); err == nil {
				routesPath = "routes.yaml"
			}
		}
	}

	// 3. Load routes configuration file if present
	if routesPath != "" {
		routesData, err := os.ReadFile(routesPath)
		if err != nil {
			return nil, fmt.Errorf("config: failed to read routes file %s: %w", routesPath, err)
		}

		var routesWrapper struct {
			Enabled *bool              `yaml:"enabled" json:"enabled"`
			Routes  []ProxyRouteConfig `yaml:"routes" json:"routes"`
			Proxy   ProxyConfig        `yaml:"proxy" json:"proxy"`
		}
		if err := yaml.Unmarshal(routesData, &routesWrapper); err != nil {
			return nil, fmt.Errorf("config: failed to parse routes file %s: %w", routesPath, err)
		}

		if len(routesWrapper.Routes) > 0 {
			cfg.Proxy.Routes = routesWrapper.Routes
			cfg.Proxy.Enabled = true
			if routesWrapper.Enabled != nil {
				cfg.Proxy.Enabled = *routesWrapper.Enabled
			}
		} else if len(routesWrapper.Proxy.Routes) > 0 || routesWrapper.Proxy.Enabled {
			cfg.Proxy = routesWrapper.Proxy
		}
	}

	validateConfigDefaults(cfg)
	return cfg, nil
}

// LoadRoutesFromFile reads and parses a standalone routes.yaml file.
func LoadRoutesFromFile(routesPath string) ([]ProxyRouteConfig, error) {
	routesData, err := os.ReadFile(routesPath)
	if err != nil {
		return nil, fmt.Errorf("config: failed to read routes file %s: %w", routesPath, err)
	}

	var routesWrapper struct {
		Enabled *bool              `yaml:"enabled" json:"enabled"`
		Routes  []ProxyRouteConfig `yaml:"routes" json:"routes"`
		Proxy   ProxyConfig        `yaml:"proxy" json:"proxy"`
	}
	if err := yaml.Unmarshal(routesData, &routesWrapper); err != nil {
		return nil, fmt.Errorf("config: failed to parse routes file %s: %w", routesPath, err)
	}

	routes := routesWrapper.Routes
	if len(routes) == 0 && len(routesWrapper.Proxy.Routes) > 0 {
		routes = routesWrapper.Proxy.Routes
	}

	for i, r := range routes {
		if strings.TrimSpace(r.Prefix) == "" {
			return nil, fmt.Errorf("config: route #%d missing required prefix parameter", i+1)
		}
	}

	return routes, nil
}

// LoadFromFile loads, decodes, and merges configuration from a single file path or auto-discovered routes file.
func (m *Manager) LoadFromFile(filePath string) (*AppConfig, error) {
	return m.LoadFromFiles(filePath, "")
}

func validateConfigDefaults(cfg *AppConfig) {
	if cfg.Server.Port <= 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.WorkerPoolSize <= 0 {
		cfg.Server.WorkerPoolSize = 128
	}
	if cfg.Server.MaxHeaderBytes <= 0 {
		cfg.Server.MaxHeaderBytes = 8 * 1024
	}
	if cfg.Server.MaxBodyBytes <= 0 {
		cfg.Server.MaxBodyBytes = 4 * 1024 * 1024
	}
}

// ValidateConfig performs strict validation of the configuration structure for dry-run CLI test checks.
func ValidateConfig(cfg *AppConfig) error {
	if cfg == nil {
		return fmt.Errorf("configuration object is nil")
	}

	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", cfg.Server.Port)
	}

	if cfg.Server.WorkerPoolSize <= 0 {
		return fmt.Errorf("server.worker_pool_size must be greater than 0, got %d", cfg.Server.WorkerPoolSize)
	}

	if cfg.Static.Enabled {
		if cfg.Static.Dir == "" {
			return fmt.Errorf("static.dir cannot be empty when static file serving is enabled")
		}
		if _, err := os.Stat(cfg.Static.Dir); err != nil {
			return fmt.Errorf("static.dir %q does not exist or is not accessible: %w", cfg.Static.Dir, err)
		}
	}

	if cfg.Proxy.Enabled {
		for i, route := range cfg.Proxy.Routes {
			if route.IsTCP() || route.IsUDP() {
				if route.GetListenPort() <= 0 || route.GetListenPort() > 65535 {
					return fmt.Errorf("Layer 4 %s route #%d missing or invalid listen_port (must be 1-65535)", route.GetType(), i+1)
				}
				if len(route.GetTargets()) == 0 {
					return fmt.Errorf("Layer 4 %s route #%d has no target address configured", route.GetType(), i+1)
				}
				continue
			}

			prefix := strings.TrimSpace(route.Prefix)
			if prefix == "" {
				return fmt.Errorf("route #%d missing required prefix parameter", i+1)
			}

			if route.IsStatic() {
				dir := route.GetDir()
				if dir == "" {
					return fmt.Errorf("static route %q missing required dir parameter", prefix)
				}
				if _, err := os.Stat(dir); err != nil {
					return fmt.Errorf("static route %q dir %q does not exist or is not accessible: %w", prefix, dir, err)
				}
			} else if route.IsUpstream() {
				algo := strings.ToLower(route.GetAlgorithm())
				if algo != "round_robin" && algo != "random" && algo != "sticky_cookie" && algo != "ip_hash" {
					return fmt.Errorf("proxy route %q specifies unsupported load balancing algorithm %q (supported: round_robin, random, sticky_cookie, ip_hash)", prefix, algo)
				}

				targets := route.GetTargets()
				if len(targets) == 0 {
					return fmt.Errorf("proxy route %q has no valid target URLs configured", prefix)
				}

				for _, targetStr := range targets {
					parsedURL, err := url.Parse(targetStr)
					if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
						return fmt.Errorf("proxy route %q target %q is invalid (must be a valid absolute HTTP or HTTPS URL, e.g. http://localhost:9001)", prefix, targetStr)
					}
				}
			}
		}
	}

	return nil
}

// LoadFromFile helper shortcut using default Manager.
func LoadFromFile(filePath string) (*AppConfig, error) {
	return NewManager().LoadFromFile(filePath)
}

// LoadFromFiles helper shortcut using default Manager.
func LoadFromFiles(configPath, routesPath string) (*AppConfig, error) {
	return NewManager().LoadFromFiles(configPath, routesPath)
}
