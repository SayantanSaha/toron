package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"sync"
)

// RouteTLSConfig captures per-host SSL/TLS and mutual TLS (mTLS) configuration.
type RouteTLSConfig struct {
	CertFile   string `yaml:"cert_file" json:"cert_file"`
	KeyFile    string `yaml:"key_file" json:"key_file"`
	CAFile     string `yaml:"ca_file" json:"ca_file"`
	ClientAuth string `yaml:"client_auth" json:"client_auth"`
	MinVersion string `yaml:"min_version" json:"min_version"`
}

// ParseClientAuthType converts string config to tls.ClientAuthType.
func ParseClientAuthType(str string) tls.ClientAuthType {
	switch strings.ToLower(strings.TrimSpace(str)) {
	case "request_client_cert", "request":
		return tls.RequestClientCert
	case "require_any_client_cert", "require_any":
		return tls.RequireAnyClientCert
	case "verify_client_cert_if_given", "verify_if_given":
		return tls.VerifyClientCertIfGiven
	case "require_and_verify", "require_and_verify_client_cert", "mtls":
		return tls.RequireAndVerifyClientCert
	default:
		return tls.NoClientCert
	}
}

// ParseTLSVersion converts string config to uint16 TLS version.
func ParseTLSVersion(str string) uint16 {
	switch strings.ToLower(strings.TrimSpace(str)) {
	case "tls1.3", "1.3", "tls13":
		return tls.VersionTLS13
	case "tls1.2", "1.2", "tls12":
		return tls.VersionTLS12
	default:
		return tls.VersionTLS12
	}
}

// HostTLSProfile encapsulates a dedicated per-host TLS certificate, client CA pool, and policies.
type HostTLSProfile struct {
	Host       string
	Cert       tls.Certificate
	ClientCAs  *x509.CertPool
	ClientAuth tls.ClientAuthType
	MinVersion uint16
}

// SNIRegistry maintains thread-safe host-to-TLS profile mappings for dynamic SNI negotiation.
type SNIRegistry struct {
	mu             sync.RWMutex
	profiles       map[string]*HostTLSProfile
	fallbackConfig *tls.Config
}

// NewSNIRegistry initializes a new SNIRegistry with a default fallback config.
func NewSNIRegistry(fallback *tls.Config) *SNIRegistry {
	return &SNIRegistry{
		profiles:       make(map[string]*HostTLSProfile),
		fallbackConfig: fallback,
	}
}

// SetFallbackConfig updates the fallback TLS configuration.
func (r *SNIRegistry) SetFallbackConfig(cfg *tls.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbackConfig = cfg
}

// RegisterProfile registers an in-memory TLS certificate and policies for a specific host.
func (r *SNIRegistry) RegisterProfile(host string, cert tls.Certificate, caPool *x509.CertPool, clientAuth tls.ClientAuthType, minVer uint16) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if minVer == 0 {
		minVer = tls.VersionTLS12
	}

	h := strings.ToLower(strings.TrimSpace(host))
	r.profiles[h] = &HostTLSProfile{
		Host:       h,
		Cert:       cert,
		ClientCAs:  caPool,
		ClientAuth: clientAuth,
		MinVersion: minVer,
	}
}

// RegisterRouteTLS loads certificate and CA files from disk and registers the host profile.
func (r *SNIRegistry) RegisterRouteTLS(host string, cfg RouteTLSConfig) error {
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil
	}

	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return fmt.Errorf("sni: failed to load keypair for host %q: %w", host, err)
	}

	var caPool *x509.CertPool
	if cfg.CAFile != "" {
		caData, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return fmt.Errorf("sni: failed to read CA file %q for host %q: %w", cfg.CAFile, host, err)
		}
		caPool = x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caData) {
			return fmt.Errorf("sni: no valid CA certificates found in %q for host %q", cfg.CAFile, host)
		}
	}

	clientAuth := ParseClientAuthType(cfg.ClientAuth)
	minVer := ParseTLSVersion(cfg.MinVersion)

	r.RegisterProfile(host, cert, caPool, clientAuth, minVer)
	return nil
}

// GetConfigForClient evaluates incoming TLS ClientHello and returns host-specific or fallback *tls.Config.
func (r *SNIRegistry) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	serverName := strings.ToLower(strings.TrimSpace(hello.ServerName))
	if serverName != "" {
		// Exact match
		if profile, exists := r.profiles[serverName]; exists {
			return &tls.Config{
				Certificates: []tls.Certificate{profile.Cert},
				ClientAuth:   profile.ClientAuth,
				ClientCAs:    profile.ClientCAs,
				MinVersion:   profile.MinVersion,
				NextProtos:   []string{"h2", "http/1.1"},
			}, nil
		}

		// Wildcard match (*.domain.com)
		for pattern, profile := range r.profiles {
			if strings.HasPrefix(pattern, "*.") {
				suffix := pattern[1:] // .domain.com
				if strings.HasSuffix(serverName, suffix) {
					return &tls.Config{
						Certificates: []tls.Certificate{profile.Cert},
						ClientAuth:   profile.ClientAuth,
						ClientCAs:    profile.ClientCAs,
						MinVersion:   profile.MinVersion,
						NextProtos:   []string{"h2", "http/1.1"},
					}, nil
				}
			}
		}
	}

	return r.fallbackConfig, nil
}

// HasHost checks whether a host matches an exact or wildcard registered SNI profile.
func (r *SNIRegistry) HasHost(host string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	h := strings.ToLower(strings.TrimSpace(host))
	if _, exists := r.profiles[h]; exists {
		return true
	}
	for pattern := range r.profiles {
		if strings.HasPrefix(pattern, "*.") {
			suffix := pattern[1:] // .domain.com
			if strings.HasSuffix(h, suffix) || h == pattern[2:] {
				return true
			}
		}
	}
	return false
}
