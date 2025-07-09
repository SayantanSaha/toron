package config

import (
	"errors"
	"net"
	"os"
	"strings"
	"gopkg.in/yaml.v3"
)

type MTLSConfig struct {
	CACertFile     string `yaml:"ca_cert_file"`
	ClientAuthType string `yaml:"client_auth_type"` // "require", "requireandverify", "optional"
}
type ServerConfig struct {
	Address          string     `yaml:"address"`
	UseTLS           bool       `yaml:"use_tls"`
	TLSMode          string     `yaml:"tls_mode"`         // "autocert", "manual", "mtls"
	AutocertDomains  []string   `yaml:"autocert_domains"` // for Let's Encrypt
	CertFile         string     `yaml:"cert_file"`
	KeyFile          string     `yaml:"key_file"`
	RedirectHTTP     bool       `yaml:"redirect_http"`
	HTTPRedirectPort string     `yaml:"http_redirect_port"`
	MTLS             MTLSConfig `yaml:"mtls"`
}

type Route struct {
	Host        string `yaml:"host"`
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
		if (r.Host == "" && r.Path == "") || r.Backend == "" {
			return errors.New("each route must have a backend and either a host or a path defined")
		}
		if r.Host != "" {
			if _, _, err := net.SplitHostPort(r.Host); err != nil {
				if _, err := net.LookupHost(r.Host); err != nil {
					// a host without a port is also valid
					if !strings.Contains(err.Error(), "missing port in address") {
						return errors.New("invalid host specified: " + r.Host)
					}
				}
			}
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
