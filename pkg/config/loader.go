package config

import (
	"fmt"
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

// LoadFromFiles loads server infrastructure config from configPath and proxy routing config from routesPath.
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
			Proxy ProxyConfig `yaml:"proxy" json:"proxy"`
		}
		if err := yaml.Unmarshal(routesData, &routesWrapper); err != nil {
			return nil, fmt.Errorf("config: failed to parse routes file %s: %w", routesPath, err)
		}

		if len(routesWrapper.Proxy.Routes) > 0 || routesWrapper.Proxy.Enabled {
			cfg.Proxy = routesWrapper.Proxy
		}
	}

	validateConfig(cfg)
	return cfg, nil
}

// LoadFromFile loads, decodes, and merges configuration from a single file path or auto-discovered routes file.
func (m *Manager) LoadFromFile(filePath string) (*AppConfig, error) {
	return m.LoadFromFiles(filePath, "")
}

func validateConfig(cfg *AppConfig) {
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

// LoadFromFile helper shortcut using default Manager.
func LoadFromFile(filePath string) (*AppConfig, error) {
	return NewManager().LoadFromFile(filePath)
}

// LoadFromFiles helper shortcut using default Manager.
func LoadFromFiles(configPath, routesPath string) (*AppConfig, error) {
	return NewManager().LoadFromFiles(configPath, routesPath)
}
