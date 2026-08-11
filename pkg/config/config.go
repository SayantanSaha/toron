package config

import (
	"time"

	"toron/pkg/server"
)

// AppConfig is the root configuration structure for Toron.
type AppConfig struct {
	Server  ServerConfig  `yaml:"server" json:"server"`
	Static  StaticConfig  `yaml:"static" json:"static"`
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
