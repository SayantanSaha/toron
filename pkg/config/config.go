package config

import (
	"fmt"
	"strings"
	"time"

	"toron/pkg/server"
	"toron/pkg/waf"
)

// AppConfig is the root configuration structure for Toron.
type AppConfig struct {
	Server     ServerConfig     `yaml:"server" json:"server"`
	Static     StaticConfig     `yaml:"static" json:"static"`
	Proxy      ProxyConfig      `yaml:"proxy" json:"proxy"`
	Logging    LoggingConfig    `yaml:"logging" json:"logging"`
	Discovery  DiscoveryConfig  `yaml:"discovery" json:"discovery"`
	Ingress    IngressConfig    `yaml:"ingress" json:"ingress"`
	Sidecar    SidecarConfig    `yaml:"sidecar" json:"sidecar"`
	Transcoder TranscoderConfig `yaml:"transcoder" json:"transcoder"`
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

// RouteTLSConfig captures per-host SSL/TLS and mutual TLS (mTLS) configuration.
type RouteTLSConfig struct {
	CertFile   string `yaml:"cert_file" json:"cert_file"`
	KeyFile    string `yaml:"key_file" json:"key_file"`
	CAFile     string `yaml:"ca_file" json:"ca_file"`
	ClientAuth string `yaml:"client_auth" json:"client_auth"`
	MinVersion string `yaml:"min_version" json:"min_version"`
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

// CacheConfig captures in-memory HTTP response caching settings.
type CacheConfig struct {
	Enabled        bool          `yaml:"enabled" json:"enabled"`
	DefaultTTL     time.Duration `yaml:"default_ttl" json:"default_ttl"`
	MaxEntries     int           `yaml:"max_entries" json:"max_entries"`
	MaxPayloadSize int           `yaml:"max_payload_size" json:"max_payload_size"`
}

// DiscoveryConfig captures OCI container auto-discovery settings.
type DiscoveryConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	Engine        string        `yaml:"engine" json:"engine"`
	SocketPath    string        `yaml:"socket_path" json:"socket_path"`
	PollInterval  time.Duration `yaml:"poll_interval" json:"poll_interval"`
	DefaultWeight int           `yaml:"default_weight" json:"default_weight"`
}

// IngressConfig captures native Kubernetes Ingress Controller settings.
type IngressConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	IngressClass      string        `yaml:"ingress_class" json:"ingress_class"`
	KubeAPIServer     string        `yaml:"kube_apiserver" json:"kube_apiserver"`
	KubeConfigPath    string        `yaml:"kubeconfig_path" json:"kubeconfig_path"`
	ServiceAccountDir string        `yaml:"service_account_dir" json:"service_account_dir"`
	ResyncPeriod      time.Duration `yaml:"resync_period" json:"resync_period"`
}

// SidecarConfig captures Service Mesh Sidecar proxy mode settings.
type SidecarConfig struct {
	Enabled            bool                `yaml:"enabled" json:"enabled"`
	Mode               string              `yaml:"mode" json:"mode"` // "ingress", "egress", "dual"
	IngressPort        int                 `yaml:"ingress_port" json:"ingress_port"`
	EgressPort         int                 `yaml:"egress_port" json:"egress_port"`
	AppPort            int                 `yaml:"app_port" json:"app_port"`
	MaxBodyBytes       int64               `yaml:"max_body_bytes" json:"max_body_bytes"`
	StrictmTLS         bool                `yaml:"strict_mtls" json:"strict_mtls"`
	CertFile           string              `yaml:"cert_file" json:"cert_file"`
	KeyFile            string              `yaml:"key_file" json:"key_file"`
	CAFile             string              `yaml:"ca_file" json:"ca_file"`
	InsecureSkipVerify bool                `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	TrafficSplits      []TrafficSplitRoute `yaml:"traffic_splits" json:"traffic_splits"`
}

// TrafficSplitRoute captures subpath weighted traffic splitting (canary releases).
type TrafficSplitRoute struct {
	Prefix   string         `yaml:"prefix" json:"prefix"`
	Backends []SplitBackend `yaml:"backends" json:"backends"`
}

// SplitBackend defines a target URL and integer weight.
type SplitBackend struct {
	Target string `yaml:"target" json:"target"`
	Weight int    `yaml:"weight" json:"weight"`
}

// TranscoderConfig captures REST JSON to gRPC Protobuf transcoding rules.
type TranscoderConfig struct {
	Enabled      bool                  `yaml:"enabled" json:"enabled"`
	MaxBodyBytes int64                 `yaml:"max_body_bytes,omitempty" json:"max_body_bytes,omitempty"`
	Routes       []TranscoderRouteRule `yaml:"routes" json:"routes"`
}

// GetMaxBodyBytes returns the configured maximum request body bytes, or defaults to 4MB (4194304 bytes) if <= 0.
func (c TranscoderConfig) GetMaxBodyBytes() int64 {
	if c.MaxBodyBytes > 0 {
		return c.MaxBodyBytes
	}
	return 4 * 1024 * 1024 // 4 MB default matching DefaultMaxGRPCFrameSize
}

// TranscoderRouteRule captures a REST HTTP route to binary gRPC RPC mapping.
type TranscoderRouteRule struct {
	HTTPMethod    string            `yaml:"http_method" json:"http_method"`
	HTTPPath      string            `yaml:"http_path" json:"http_path"`
	GRPCMethod    string            `yaml:"grpc_method" json:"grpc_method"`
	UpstreamURL   string            `yaml:"upstream_url" json:"upstream_url"`
	FieldMappings map[string]string `yaml:"field_mappings" json:"field_mappings"`
}

// JWTConfig captures JWT authentication settings.
type JWTConfig struct {
	Secret   string `yaml:"secret" json:"secret"`
	Issuer   string `yaml:"issuer" json:"issuer"`
	Audience string `yaml:"audience" json:"audience"`
}

// APIKeyConfig captures API key authentication settings.
type APIKeyConfig struct {
	Keys   []string `yaml:"keys" json:"keys"`
	Header string   `yaml:"header" json:"header"`
	Query  string   `yaml:"query" json:"query"`
}

// BasicAuthConfig captures HTTP Basic authentication settings.
type BasicAuthConfig struct {
	Users map[string]string `yaml:"users" json:"users"`
	Realm string            `yaml:"realm" json:"realm"`
}

// AdminAuthConfig captures administrative API authentication settings.
type AdminAuthConfig struct {
	Enabled  bool              `yaml:"enabled" json:"enabled"`
	Token    string            `yaml:"token" json:"token"`
	APIKeys  []string          `yaml:"api_keys" json:"api_keys"`
	Username string            `yaml:"username" json:"username"`
	Password string            `yaml:"password" json:"password"`
	Users    map[string]string `yaml:"users" json:"users"`
}

// AuthConfig captures multi-scheme authentication settings.
type AuthConfig struct {
	Type     string          `yaml:"type" json:"type"`
	JWT      JWTConfig       `yaml:"jwt" json:"jwt"`
	APIKey   APIKeyConfig    `yaml:"api_key" json:"api_key"`
	Basic    BasicAuthConfig `yaml:"basic" json:"basic"`
	Excluded []string        `yaml:"excluded" json:"excluded"`
}

// CORSConfig captures Cross-Origin Resource Sharing settings.
type CORSConfig struct {
	Enabled          bool     `yaml:"enabled" json:"enabled"`
	AllowOrigins     []string `yaml:"allow_origins" json:"allow_origins"`
	AllowMethods     []string `yaml:"allow_methods" json:"allow_methods"`
	AllowHeaders     []string `yaml:"allow_headers" json:"allow_headers"`
	ExposeHeaders    []string `yaml:"expose_headers" json:"expose_headers"`
	AllowCredentials bool     `yaml:"allow_credentials" json:"allow_credentials"`
	MaxAge           int      `yaml:"max_age" json:"max_age"`
}

// SecurityHeadersConfig captures HTTP browser security headers settings.
type SecurityHeadersConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	HSTS               string `yaml:"hsts" json:"hsts"`
	ContentTypeOptions string `yaml:"content_type_options" json:"content_type_options"`
	FrameOptions       string `yaml:"frame_options" json:"frame_options"`
	ReferrerPolicy     string `yaml:"referrer_policy" json:"referrer_policy"`
	CSP                string `yaml:"csp" json:"csp"`
	PermissionsPolicy  string `yaml:"permissions_policy" json:"permissions_policy"`
}

// HTTPRedirectConfig captures cleartext HTTP to HTTPS redirection settings.
type HTTPRedirectConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	Port         int      `yaml:"port" json:"port"`
	AllowedHosts []string `yaml:"allowed_hosts" json:"allowed_hosts"`
	DefaultHost  string   `yaml:"default_host" json:"default_host"`
}

// ServerConfig captures network and security settings.
type ServerConfig struct {
	Host               string                `yaml:"host" json:"host"`
	Port               int                   `yaml:"port" json:"port"`
	WorkerPoolSize     int                   `yaml:"worker_pool_size" json:"worker_pool_size"`
	ReadTimeout        time.Duration         `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout       time.Duration         `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout        time.Duration         `yaml:"idle_timeout" json:"idle_timeout"`
	UpgradeIdleTimeout time.Duration         `yaml:"upgrade_idle_timeout,omitempty" json:"upgrade_idle_timeout,omitempty"`
	MaxHeaderBytes     int                   `yaml:"max_header_bytes" json:"max_header_bytes"`
	MaxBodyBytes       int64                 `yaml:"max_body_bytes" json:"max_body_bytes"`
	HTTP2              HTTP2Config           `yaml:"http2" json:"http2"`
	HTTP3              HTTP3Config           `yaml:"http3" json:"http3"`
	HTTPRedirect       HTTPRedirectConfig    `yaml:"http_redirect" json:"http_redirect"`
	TLS                TLSConfig             `yaml:"tls" json:"tls"`
	ACME               ACMEConfig            `yaml:"acme" json:"acme"`
	Compression        CompressionConfig     `yaml:"compression" json:"compression"`
	Cache              CacheConfig           `yaml:"cache" json:"cache"`
	Auth               AuthConfig            `yaml:"auth" json:"auth"`
	CORS               CORSConfig            `yaml:"cors" json:"cors"`
	SecurityHeaders    SecurityHeadersConfig `yaml:"security_headers" json:"security_headers"`
	WAF                waf.WAFConfig         `yaml:"waf" json:"waf"`
	AdminAuth          AdminAuthConfig       `yaml:"admin_auth" json:"admin_auth"`
	AdminSubnets       []string              `yaml:"admin_subnets" json:"admin_subnets"`
	TrustedProxies     []string              `yaml:"trusted_proxies" json:"trusted_proxies"`
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
	Type                string                `yaml:"type" json:"type"` // "static" or "upstream" / "proxy"
	Host                string                `yaml:"host" json:"host"`
	Domain              string                `yaml:"domain" json:"domain"`
	Prefix              string                `yaml:"prefix" json:"prefix"`
	Headers             map[string]string     `yaml:"headers" json:"headers"`
	Dir                 string                `yaml:"dir" json:"dir"`
	StaticDir           string                `yaml:"static_dir" json:"static_dir"`
	SPA                 bool                  `yaml:"spa" json:"spa"`
	Fallback            string                `yaml:"fallback" json:"fallback"`
	RedirectHTTP        *bool                 `yaml:"redirect_http,omitempty" json:"redirect_http,omitempty"`
	HTTPSRedirect       *bool                 `yaml:"https_redirect,omitempty" json:"https_redirect,omitempty"`
	AccessLog           string                `yaml:"access_log,omitempty" json:"access_log,omitempty"`
	SecurityLog         string                `yaml:"security_log,omitempty" json:"security_log,omitempty"`
	ListenPort          int                   `yaml:"listen_port" json:"listen_port"`
	Port                int                   `yaml:"port" json:"port"`
	Target              string                `yaml:"target" json:"target"`
	Targets             []string              `yaml:"targets" json:"targets"`
	Algorithm           string                `yaml:"algorithm" json:"algorithm"`
	HealthCheckType     string                `yaml:"health_check_type" json:"health_check_type"`
	HealthCheckPath     string                `yaml:"health_check_path" json:"health_check_path"`
	HealthCheckService  string                `yaml:"health_check_service" json:"health_check_service"`
	HealthCheckInterval time.Duration         `yaml:"health_check_interval" json:"health_check_interval"`
	ConsecutiveFailures int                   `yaml:"consecutive_failures" json:"consecutive_failures"`
	CooldownPeriod      time.Duration         `yaml:"cooldown_period" json:"cooldown_period"`
	RateLimit           string                `yaml:"rate_limit" json:"rate_limit"`
	StickyCookieName    string                `yaml:"sticky_cookie_name" json:"sticky_cookie_name"`
	StripPrefix         *bool                 `yaml:"strip_prefix,omitempty" json:"strip_prefix,omitempty"`
	RewriteRedirects    *bool                 `yaml:"rewrite_redirects,omitempty" json:"rewrite_redirects,omitempty"`
	RewriteCookiePath   *bool                 `yaml:"rewrite_cookie_path,omitempty" json:"rewrite_cookie_path,omitempty"`
	Auth                AuthConfig            `yaml:"auth" json:"auth"`
	TLS                 RouteTLSConfig        `yaml:"tls" json:"tls"`
	InsecureSkipVerify  bool                  `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	CORS                CORSConfig            `yaml:"cors" json:"cors"`
	SecurityHeaders     SecurityHeadersConfig `yaml:"security_headers" json:"security_headers"`
	WAF                 waf.WAFConfig         `yaml:"waf" json:"waf"`
	TrustedProxies      []string              `yaml:"trusted_proxies" json:"trusted_proxies"`
	MaxConnections      int                   `yaml:"max_connections,omitempty" json:"max_connections,omitempty"`
	IdleTimeout         time.Duration         `yaml:"idle_timeout,omitempty" json:"idle_timeout,omitempty"`
	MaxWorkers          int                   `yaml:"max_workers,omitempty" json:"max_workers,omitempty"`
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

// ShouldStripPrefix returns true if the route prefix should be stripped before forwarding (default: true).
func (p *ProxyRouteConfig) ShouldStripPrefix() bool {
	if p.StripPrefix != nil {
		return *p.StripPrefix
	}
	return true
}

// ShouldRewriteRedirects returns true if 3xx redirect Location headers should be rewritten to include prefix (default: true).
func (p *ProxyRouteConfig) ShouldRewriteRedirects() bool {
	if p.RewriteRedirects != nil {
		return *p.RewriteRedirects
	}
	return true
}

// ShouldRewriteCookiePath returns true if Set-Cookie Path attributes should be rewritten to include prefix (default: true).
func (p *ProxyRouteConfig) ShouldRewriteCookiePath() bool {
	if p.RewriteCookiePath != nil {
		return *p.RewriteCookiePath
	}
	return true
}

// GetRedirectHTTP returns the explicit redirect_http or https_redirect pointer if configured.
func (p *ProxyRouteConfig) GetRedirectHTTP() *bool {
	if p.RedirectHTTP != nil {
		return p.RedirectHTTP
	}
	if p.HTTPSRedirect != nil {
		return p.HTTPSRedirect
	}
	return nil
}

// ShouldRedirectHTTP returns true if the route should be upgraded from HTTP to HTTPS (default: true).
func (p *ProxyRouteConfig) ShouldRedirectHTTP() bool {
	if redir := p.GetRedirectHTTP(); redir != nil {
		return *redir
	}
	return true
}

// GetAccessLog returns the route-specific access log destination or default if not configured.
func (p *ProxyRouteConfig) GetAccessLog(defaultPath string) string {
	if strings.TrimSpace(p.AccessLog) != "" {
		return strings.TrimSpace(p.AccessLog)
	}
	return defaultPath
}

// GetSecurityLog returns the route-specific security log destination or default if not configured.
func (p *ProxyRouteConfig) GetSecurityLog(defaultPath string) string {
	if strings.TrimSpace(p.SecurityLog) != "" {
		return strings.TrimSpace(p.SecurityLog)
	}
	return defaultPath
}

// GetMaxConnections returns the configured maximum concurrent TCP connections or default 10,000 if not set.
func (p *ProxyRouteConfig) GetMaxConnections() int {
	if p.MaxConnections > 0 {
		return p.MaxConnections
	}
	return 10000
}

// GetIdleTimeout returns the configured idle timeout duration or default 60s if not set.
func (p *ProxyRouteConfig) GetIdleTimeout() time.Duration {
	if p.IdleTimeout > 0 {
		return p.IdleTimeout
	}
	return 60 * time.Second
}

// GetMaxWorkers returns the configured maximum UDP workers or default 1,024 if not set.
func (p *ProxyRouteConfig) GetMaxWorkers() int {
	if p.MaxWorkers > 0 {
		return p.MaxWorkers
	}
	return 1024
}

// LoggingConfig captures logging settings.
type LoggingConfig struct {
	Level       string `yaml:"level" json:"level"`
	Format      string `yaml:"format" json:"format"`
	ServerLog   string `yaml:"server_log" json:"server_log"`
	AccessLog   string `yaml:"access_log" json:"access_log"`
	SecurityLog string `yaml:"security_log" json:"security_log"`
}

// DefaultAppConfig returns sensible default server configuration settings.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		Server: ServerConfig{
			Host:               "0.0.0.0",
			Port:               8080,
			WorkerPoolSize:     128,
			ReadTimeout:        5 * time.Second,
			WriteTimeout:       5 * time.Second,
			IdleTimeout:        30 * time.Second,
			UpgradeIdleTimeout: 60 * time.Second,
			MaxHeaderBytes:     8 * 1024,        // 8 KB
			MaxBodyBytes:       4 * 1024 * 1024, // 4 MB
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
				Encodings: []string{"zstd", "br", "gzip", "deflate"},
				Types: []string{
					"text/",
					"application/json",
					"application/javascript",
					"application/xml",
					"application/xhtml+xml",
					"image/svg+xml",
				},
			},
			Cache: CacheConfig{
				Enabled:        true,
				DefaultTTL:     60 * time.Second,
				MaxEntries:     1000,
				MaxPayloadSize: 1024 * 1024, // 1 MB
			},
			CORS: CORSConfig{
				Enabled:          false,
				AllowOrigins:     []string{"*"},
				AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"},
				AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
				AllowCredentials: false,
				MaxAge:           86400,
			},
			SecurityHeaders: SecurityHeadersConfig{
				Enabled:            true,
				HSTS:               "max-age=31536000; includeSubDomains",
				ContentTypeOptions: "nosniff",
				FrameOptions:       "DENY",
				ReferrerPolicy:     "strict-origin-when-cross-origin",
			},
			WAF: waf.DefaultConfig(),
			AdminAuth: AdminAuthConfig{
				Enabled: false,
			},
			AdminSubnets: []string{"127.0.0.1/32", "::1/128"},
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
		Sidecar: SidecarConfig{
			MaxBodyBytes:       10 * 1024 * 1024,
			InsecureSkipVerify: false,
		},
		Transcoder: TranscoderConfig{
			MaxBodyBytes: 4 * 1024 * 1024,
		},
		Logging: LoggingConfig{
			Level:       "info",
			Format:      "text",
			ServerLog:   "logs/server.log",
			AccessLog:   "logs/access.log",
			SecurityLog: "logs/security.log",
		},
	}
}

// ToServerConfig converts AppConfig into server.Config expected by pkg/server.
func (c *AppConfig) ToServerConfig() server.Config {
	addr := c.Server.Host + ":" + string(itoa(c.Server.Port))
	if c.Server.Host == "" || c.Server.Host == "0.0.0.0" {
		addr = ":" + string(itoa(c.Server.Port))
	}

	upgradeIdleTimeout := c.Server.UpgradeIdleTimeout
	if upgradeIdleTimeout <= 0 {
		upgradeIdleTimeout = c.Server.IdleTimeout
	}
	if upgradeIdleTimeout <= 0 {
		upgradeIdleTimeout = 60 * time.Second
	}

	return server.Config{
		Addr:                      addr,
		WorkerPoolSize:            c.Server.WorkerPoolSize,
		ReadTimeout:               c.Server.ReadTimeout,
		WriteTimeout:              c.Server.WriteTimeout,
		IdleTimeout:               c.Server.IdleTimeout,
		UpgradeIdleTimeout:        upgradeIdleTimeout,
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
		HTTPRedirectEnabled:       c.Server.HTTPRedirect.Enabled,
		HTTPRedirectPort:          c.Server.HTTPRedirect.Port,
		HTTPRedirectAllowedHosts:  c.Server.HTTPRedirect.AllowedHosts,
		HTTPRedirectDefaultHost:   c.Server.HTTPRedirect.DefaultHost,
		HTTPSPort:                 c.Server.Port,
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
