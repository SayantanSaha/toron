package config

import (
	"strings"
	"time"

	"toron/pkg/server"
)

// AppConfig is the root configuration structure for Toron.
type AppConfig struct {
	Server  ServerConfig  `yaml:"server" json:"server"`
	Static  StaticConfig  `yaml:"static" json:"static"`
	Proxy   ProxyConfig   `yaml:"proxy" json:"proxy"`
	Logging LoggingConfig `yaml:"logging" json:"logging"`
}

// ServerConfig captures network and security settings.
type ServerConfig struct {
	Host           string        `yaml:"host" json:"host"`
	Port           int           `yaml:"port" json:"port"`
	WorkerPoolSize int           `yaml:"worker_pool_size" json:"worker_pool_size"`
	ReadTimeout    time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	MaxBodyBytes   int64         `yaml:"max_body_bytes" json:"max_body_bytes"`
}

// StaticConfig captures static asset directory settings.
type StaticConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Prefix  string `yaml:"prefix" json:"prefix"`
	Dir     string `yaml:"dir" json:"dir"`
}

// ProxyConfig captures reverse proxy routes settings.
type ProxyConfig struct {
	Enabled bool               `yaml:"enabled" json:"enabled"`
	Routes  []ProxyRouteConfig `yaml:"routes" json:"routes"`
}

// ProxyRouteConfig describes an individual prefix and optional header condition to upstream target URL mapping with optional load balancing.
type ProxyRouteConfig struct {
	Prefix    string            `yaml:"prefix" json:"prefix"`
	Headers   map[string]string `yaml:"headers" json:"headers"`
	Target    string            `yaml:"target" json:"target"`
	Targets   []string          `yaml:"targets" json:"targets"`
	Algorithm string            `yaml:"algorithm" json:"algorithm"`
}

// GetTargets returns all configured upstream target URLs for the route.
// Combines Target (single string) and Targets ([]string), eliminating duplicates while preserving order.
func (p *ProxyRouteConfig) GetTargets() []string {
	var list []string
	seen := make(map[string]bool)

	if p.Target != "" {
		trimmed := strings.TrimSpace(p.Target)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			list = append(list, trimmed)
		}
	}

	for _, t := range p.Targets {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			list = append(list, trimmed)
		}
	}

	return list
}

// GetAlgorithm returns the configured load balancing algorithm or "round_robin" by default.
func (p *ProxyRouteConfig) GetAlgorithm() string {
	if strings.TrimSpace(p.Algorithm) == "" {
		return "round_robin"
	}
	return strings.TrimSpace(p.Algorithm)
}

// LoggingConfig captures logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// DefaultAppConfig returns sensible default server configuration settings.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		Server: ServerConfig{
			Host:           "0.0.0.0",
			Port:           8080,
			WorkerPoolSize: 128,
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			IdleTimeout:    30 * time.Second,
			MaxHeaderBytes: 8 * 1024,        // 8 KB
			MaxBodyBytes:   4 * 1024 * 1024, // 4 MB
		},
		Static: StaticConfig{
			Enabled: true,
			Prefix:  "/",
			Dir:     "./public",
		},
		Proxy: ProxyConfig{
			Enabled: false,
			Routes:  []ProxyRouteConfig{},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// ToServerConfig converts AppConfig into server.Config expected by pkg/server.
func (c *AppConfig) ToServerConfig() server.Config {
	addr := c.Server.Host + ":" + string(itoa(c.Server.Port))
	if c.Server.Host == "" || c.Server.Host == "0.0.0.0" {
		addr = ":" + string(itoa(c.Server.Port))
	}

	return server.Config{
		Addr:           addr,
		WorkerPoolSize: c.Server.WorkerPoolSize,
		ReadTimeout:    c.Server.ReadTimeout,
		WriteTimeout:   c.Server.WriteTimeout,
		IdleTimeout:    c.Server.IdleTimeout,
		MaxHeaderBytes: c.Server.MaxHeaderBytes,
		MaxBodyBytes:   c.Server.MaxBodyBytes,
	}
}

func itoa(i int) []byte {
	if i == 0 {
		return []byte("0")
	}
	var b [20]byte
	bp := len(b) - 1
	n := i
	for n > 0 {
		b[bp] = byte('0' + n%10)
		bp--
		n /= 10
	}
	return b[bp+1:]
}
