package config

import (
	"fmt"
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

// HTTP2Config captures HTTP/2 protocol settings.
type HTTP2Config struct {
	Enabled              bool   `yaml:"enabled" json:"enabled"`
	MaxConcurrentStreams uint32 `yaml:"max_concurrent_streams" json:"max_concurrent_streams"`
	MaxFrameSize         uint32 `yaml:"max_frame_size" json:"max_frame_size"`
	AllowH2C             bool   `yaml:"allow_h2c" json:"allow_h2c"`
}

// TLSConfig captures HTTPS TLS settings and certificate locations.
type TLSConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	CertFile    string `yaml:"cert_file" json:"cert_file"`
	KeyFile     string `yaml:"key_file" json:"key_file"`
	AutoDevCert bool   `yaml:"auto_dev_cert" json:"auto_dev_cert"`
}

// HTTP3Config captures HTTP/3 protocol settings over QUIC.
type HTTP3Config struct {
	Enabled      bool `yaml:"enabled" json:"enabled"`
	Port         int  `yaml:"port" json:"port"`
	AltSvcHeader bool `yaml:"alt_svc_header" json:"alt_svc_header"`
}

// ACMEConfig captures zero-touch production SSL certificate settings.
type ACMEConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	DirectoryURL  string   `yaml:"directory_url" json:"directory_url"`
	Email         string   `yaml:"email" json:"email"`
	Domains       []string `yaml:"domains" json:"domains"`
	CacheDir      string   `yaml:"cache_dir" json:"cache_dir"`
	ChallengeType string   `yaml:"challenge_type" json:"challenge_type"`
}

// CompressionConfig captures transparent response compression settings.
type CompressionConfig struct {
	Enabled   bool     `yaml:"enabled" json:"enabled"`
	MinLength int      `yaml:"min_length" json:"min_length"`
	Level     int      `yaml:"level" json:"level"`
	Encodings []string `yaml:"encodings" json:"encodings"`
	Types     []string `yaml:"types" json:"types"`
}

// ServerConfig captures network and security settings.
type ServerConfig struct {
	Host           string            `yaml:"host" json:"host"`
	Port           int               `yaml:"port" json:"port"`
	WorkerPoolSize int               `yaml:"worker_pool_size" json:"worker_pool_size"`
	ReadTimeout    time.Duration     `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout   time.Duration     `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout    time.Duration     `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes int               `yaml:"max_header_bytes" json:"max_header_bytes"`
	MaxBodyBytes   int64             `yaml:"max_body_bytes" json:"max_body_bytes"`
	HTTP2          HTTP2Config       `yaml:"http2" json:"http2"`
	HTTP3          HTTP3Config       `yaml:"http3" json:"http3"`
	TLS            TLSConfig         `yaml:"tls" json:"tls"`
	ACME           ACMEConfig        `yaml:"acme" json:"acme"`
	Compression    CompressionConfig `yaml:"compression" json:"compression"`
}

// StaticConfig captures legacy static asset directory settings.
type StaticConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Prefix  string `yaml:"prefix" json:"prefix"`
	Dir     string `yaml:"dir" json:"dir"`
}

// ProxyConfig captures routing rules settings (static sites & upstream reverse proxies).
type ProxyConfig struct {
	Enabled bool               `yaml:"enabled" json:"enabled"`
	Routes  []ProxyRouteConfig `yaml:"routes" json:"routes"`
}

// ProxyRouteConfig describes a route rule that can serve either a static site or act as an upstream reverse proxy.
type ProxyRouteConfig struct {
	Type                string            `yaml:"type" json:"type"` // "static" or "upstream" / "proxy"
	Host                string            `yaml:"host" json:"host"`
	Domain              string            `yaml:"domain" json:"domain"`
	Prefix              string            `yaml:"prefix" json:"prefix"`
	Headers             map[string]string `yaml:"headers" json:"headers"`
	Dir                 string            `yaml:"dir" json:"dir"`
	StaticDir           string            `yaml:"static_dir" json:"static_dir"`
	ListenPort          int               `yaml:"listen_port" json:"listen_port"`
	Port                int               `yaml:"port" json:"port"`
	Target              string            `yaml:"target" json:"target"`
	Targets             []string          `yaml:"targets" json:"targets"`
	Algorithm           string            `yaml:"algorithm" json:"algorithm"`
	HealthCheckPath     string            `yaml:"health_check_path" json:"health_check_path"`
	HealthCheckInterval time.Duration     `yaml:"health_check_interval" json:"health_check_interval"`
	ConsecutiveFailures int               `yaml:"consecutive_failures" json:"consecutive_failures"`
	CooldownPeriod      time.Duration     `yaml:"cooldown_period" json:"cooldown_period"`
	RateLimit           string            `yaml:"rate_limit" json:"rate_limit"`
	StickyCookieName    string            `yaml:"sticky_cookie_name" json:"sticky_cookie_name"`
}

// GetType returns the normalized route target type ("static", "upstream", "tcp", or "udp").
func (p *ProxyRouteConfig) GetType() string {
	t := strings.ToLower(strings.TrimSpace(p.Type))
	if t == "tcp" {
		return "tcp"
	}
	if t == "udp" {
		return "udp"
	}
	if t == "static" {
		return "static"
	}
	if t == "upstream" || t == "proxy" {
		return "upstream"
	}
	if p.GetDir() != "" {
		return "static"
	}
	return "upstream"
}

// IsStatic returns true if the route serves static site assets.
func (p *ProxyRouteConfig) IsStatic() bool {
	return p.GetType() == "static"
}

// IsUpstream returns true if the route acts as an upstream reverse proxy.
func (p *ProxyRouteConfig) IsUpstream() bool {
	return p.GetType() == "upstream"
}

// IsTCP returns true if the route is a Layer 4 TCP proxy.
func (p *ProxyRouteConfig) IsTCP() bool {
	return p.GetType() == "tcp"
}

// IsUDP returns true if the route is a Layer 4 UDP proxy.
func (p *ProxyRouteConfig) IsUDP() bool {
	return p.GetType() == "udp"
}

// GetListenPort returns the configured listener port for Layer 4 TCP/UDP routes.
func (p *ProxyRouteConfig) GetListenPort() int {
	if p.ListenPort > 0 {
		return p.ListenPort
	}
	if p.Port > 0 {
		return p.Port
	}
	return 0
}

// GetDir returns the configured local directory path for static routes.
func (p *ProxyRouteConfig) GetDir() string {
	if strings.TrimSpace(p.Dir) != "" {
		return strings.TrimSpace(p.Dir)
	}
	if strings.TrimSpace(p.StaticDir) != "" {
		return strings.TrimSpace(p.StaticDir)
	}
	return ""
}

// GetHost returns configured domain host matching string.
func (p *ProxyRouteConfig) GetHost() string {
	if strings.TrimSpace(p.Host) != "" {
		return strings.ToLower(strings.TrimSpace(p.Host))
	}
	if strings.TrimSpace(p.Domain) != "" {
		return strings.ToLower(strings.TrimSpace(p.Domain))
	}
	return ""
}

// GetTargets returns all configured upstream target URLs for the route.
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
			HTTP2: HTTP2Config{
				Enabled:              true,
				MaxConcurrentStreams: 250,
				MaxFrameSize:         16384,
				AllowH2C:             true,
			},
			HTTP3: HTTP3Config{
				Enabled:      true,
				Port:         8443,
				AltSvcHeader: true,
			},
			Compression: CompressionConfig{
				Enabled:   true,
				MinLength: 512,
				Level:     -1,
				Encodings: []string{"gzip", "deflate"},
				Types: []string{
					"text/",
					"application/json",
					"application/javascript",
					"application/xml",
					"application/xhtml+xml",
					"image/svg+xml",
				},
			},
		},
		Static: StaticConfig{
			Enabled: false,
			Prefix:  "/internal/dashboard/",
			Dir:     "./public",
		},
		Proxy: ProxyConfig{
			Enabled: true,
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
		Addr:                      addr,
		WorkerPoolSize:            c.Server.WorkerPoolSize,
		ReadTimeout:               c.Server.ReadTimeout,
		WriteTimeout:              c.Server.WriteTimeout,
		IdleTimeout:               c.Server.IdleTimeout,
		MaxHeaderBytes:            c.Server.MaxHeaderBytes,
		MaxBodyBytes:              c.Server.MaxBodyBytes,
		HTTP2Enabled:              c.Server.HTTP2.Enabled,
		HTTP2MaxConcurrentStreams: c.Server.HTTP2.MaxConcurrentStreams,
		HTTP2MaxFrameSize:         c.Server.HTTP2.MaxFrameSize,
		TLSEnabled:                c.Server.TLS.Enabled,
		TLSCertFile:               c.Server.TLS.CertFile,
		TLSKeyFile:                c.Server.TLS.KeyFile,
		TLSAutoDevCert:            c.Server.TLS.AutoDevCert,
		HTTP3Enabled:              c.Server.HTTP3.Enabled,
		HTTP3Port:                 c.Server.HTTP3.Port,
		HTTP3AltSvcHeader:         c.Server.HTTP3.AltSvcHeader,
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

// ParseRateLimit parses a rate limit string into refill rate per second and max burst capacity.
func ParseRateLimit(s string) (ratePerSec float64, burst int, err error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, 0, nil
	}

	parts := strings.Split(s, "/")
	countStr := strings.TrimSpace(parts[0])
	var count int
	if _, parseErr := fmt.Sscanf(countStr, "%d", &count); parseErr != nil || count <= 0 {
		return 0, 0, fmt.Errorf("config: invalid rate limit count %q in %q", countStr, s)
	}

	unit := "sec"
	if len(parts) > 1 {
		unit = strings.TrimSpace(parts[1])
	}

	switch unit {
	case "s", "sec", "second", "seconds":
		return float64(count), count, nil
	case "m", "min", "minute", "minutes":
		return float64(count) / 60.0, count, nil
	case "h", "hr", "hour", "hours":
		return float64(count) / 3600.0, count, nil
	default:
		return 0, 0, fmt.Errorf("config: unknown rate limit unit %q in %q (expected sec, min, or hour)", unit, s)
	}
}
