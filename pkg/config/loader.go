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

// LoadFromFile loads, decodes, and merges configuration from a file path.
func (m *Manager) LoadFromFile(filePath string) (*AppConfig, error) {
	cfg := DefaultAppConfig()

	if filePath == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("config: failed to read file %s: %w", filePath, err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	loader, exists := m.loaders[ext]
	if !exists {
		return nil, fmt.Errorf("config: unsupported config file extension %q (supported: .yaml, .yml)", ext)
	}

	if err := loader.Load(data, cfg); err != nil {
		return nil, fmt.Errorf("config: failed to parse file %s: %w", filePath, err)
	}

	validateConfig(cfg)
	return cfg, nil
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
