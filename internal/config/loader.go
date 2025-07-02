package config

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Address  string `yaml:"address"`
	UseTLS   bool   `yaml:"use_tls"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type Route struct {
	Path        string `yaml:"path"`
	Backend     string `yaml:"backend"`
	StripPrefix bool   `yaml:"strip_prefix"`
	MatchType   string `yaml:"match_type"` // "exact_match" or "prefix_match"
}

type Config struct {
	Server ServerConfig `yaml:"server"`
	Routes []Route      `yaml:"routes"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	applyEnvOverrides(&cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Optional validation logic for early detection of misconfigurations
func validateConfig(cfg Config) error {
	if cfg.Server.Address == "" {
		return errors.New("server.address is required")
	}
	for _, r := range cfg.Routes {
		if r.Path == "" || r.Backend == "" {
			return errors.New("each route must have both path and backend defined")
		}
		if r.MatchType != "" && r.MatchType != "exact_match" && r.MatchType != "prefix_match" {
			return errors.New("route.match_type must be either 'exact_match' or 'prefix_match'")
		}
	}
	return nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("TORON_ADDRESS"); v != "" {
		cfg.Server.Address = v
	}
	if v := os.Getenv("TORON_USE_TLS"); strings.ToLower(v) == "true" {
		cfg.Server.UseTLS = true
	}
	if v := os.Getenv("TORON_CERT_FILE"); v != "" {
		cfg.Server.CertFile = v
	}
	if v := os.Getenv("TORON_KEY_FILE"); v != "" {
		cfg.Server.KeyFile = v
	}
}
