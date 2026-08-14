package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"toron/pkg/httpparser"
)

// ACMEExtensionOID is the RFC 8737 X.509 extension OID for TLS-ALPN-01 (1.3.6.1.5.5.7.1.31).
var ACMEExtensionOID = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 31}

// ACMEConfig defines settings for zero-touch production SSL certificate issuance and renewal.
type ACMEConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	DirectoryURL  string   `yaml:"directory_url" json:"directory_url"`
	Email         string   `yaml:"email" json:"email"`
	Domains       []string `yaml:"domains" json:"domains"`
	CacheDir      string   `yaml:"cache_dir" json:"cache_dir"`
	ChallengeType string   `yaml:"challenge_type" json:"challenge_type"` // "http-01" or "tls-alpn-01"
}

// ACMEManager manages ACME account keys, HTTP-01/TLS-ALPN-01 challenge responders, and certificate caching.
type ACMEManager struct {
	cfg              ACMEConfig
	accountKey       *ecdsa.PrivateKey
	mu               sync.RWMutex
	http01Tokens     map[string]string         // token -> keyAuth
	tlsALPN01Certs   map[string]*tls.Certificate // domain -> challenge cert
	certCache        map[string]*tls.Certificate // domain -> cached cert
}

// NewACMEManager initializes the ACME manager, loading or generating account keys and disk caches.
func NewACMEManager(cfg ACMEConfig) (*ACMEManager, error) {
	if cfg.CacheDir == "" {
		cfg.CacheDir = "./certs"
	}
	if cfg.DirectoryURL == "" {
		cfg.DirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	if cfg.ChallengeType == "" {
		cfg.ChallengeType = "http-01"
	}

	if err := os.MkdirAll(cfg.CacheDir, 0700); err != nil {
		return nil, fmt.Errorf("acme: failed to create cache dir %q: %w", cfg.CacheDir, err)
	}

	accountKey, err := loadOrGenerateAccountKey(filepath.Join(cfg.CacheDir, "acme_account_key.pem"))
	if err != nil {
		return nil, fmt.Errorf("acme: failed to initialize account key: %w", err)
	}

	mgr := &ACMEManager{
		cfg:            cfg,
		accountKey:     accountKey,
		http01Tokens:   make(map[string]string),
		tlsALPN01Certs: make(map[string]*tls.Certificate),
		certCache:      make(map[string]*tls.Certificate),
	}

	_ = mgr.loadCachedCertificates()
	return mgr, nil
}

// AccountKeyThumbprint calculates base64url-encoded SHA-256 digest of account public key (JWK thumbprint).
func (m *ACMEManager) AccountKeyThumbprint() string {
	pub := &m.accountKey.PublicKey
	// JWK format for ECDSA P-256
	xBytes := pub.X.Bytes()
	yBytes := pub.Y.Bytes()
	jwkJSON := fmt.Sprintf(`{"crv":"P-256","kty":"EC","x":%q,"y":%q}`,
		base64.RawURLEncoding.EncodeToString(xBytes),
		base64.RawURLEncoding.EncodeToString(yBytes))

	hash := sha256.Sum256([]byte(jwkJSON))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// ComputeKeyAuthorization returns token + "." + thumbprint according to RFC 8555.
func (m *ACMEManager) ComputeKeyAuthorization(token string) string {
	return token + "." + m.AccountKeyThumbprint()
}

// SetHTTP01Challenge registers an HTTP-01 challenge token and key authorization.
func (m *ACMEManager) SetHTTP01Challenge(token, keyAuth string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.http01Tokens[token] = keyAuth
}

// GetHTTP01Challenge retrieves key authorization for an HTTP-01 challenge token.
func (m *ACMEManager) GetHTTP01Challenge(token string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keyAuth, exists := m.http01Tokens[token]
	return keyAuth, exists
}

// SetTLSALPN01Challenge registers a TLS-ALPN-01 challenge certificate for a domain.
func (m *ACMEManager) SetTLSALPN01Challenge(domain string, cert *tls.Certificate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tlsALPN01Certs[strings.ToLower(domain)] = cert
}

// GenerateTLSALPN01Certificate constructs a transient X.509 certificate containing RFC 8737 acmeIdentifier extension.
func (m *ACMEManager) GenerateTLSALPN01Certificate(domain, keyAuth string) (*tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("acme: failed to generate temp key: %w", err)
	}

	keyAuthDigest := sha256.Sum256([]byte(keyAuth))
	extValue, err := asn1.Marshal(keyAuthDigest[:])
	if err != nil {
		return nil, fmt.Errorf("acme: failed to marshal acmeIdentifier extension: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(24 * time.Hour),
		DNSNames:  []string{domain},
		ExtraExtensions: []pkix.Extension{
			{
				Id:       ACMEExtensionOID,
				Critical: true,
				Value:    extValue,
			},
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("acme: failed to create ALPN cert: %w", err)
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
	}
	return cert, nil
}

// GetCertificate implements tls.Config.GetCertificate for SNI and TLS-ALPN-01 ALPN negotiation.
func (m *ACMEManager) GetCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := strings.ToLower(strings.TrimSpace(clientHello.ServerName))

	// 1. Check if client negotiated ALPN "acme-tls/1" for TLS-ALPN-01 challenge
	for _, proto := range clientHello.SupportedProtos {
		if proto == "acme-tls/1" {
			m.mu.RLock()
			challengeCert, exists := m.tlsALPN01Certs[domain]
			m.mu.RUnlock()
			if exists {
				return challengeCert, nil
			}
		}
	}

	// 2. Check cached valid TLS certificates
	m.mu.RLock()
	cert, exists := m.certCache[domain]
	m.mu.RUnlock()
	if exists && cert != nil {
		return cert, nil
	}

	return nil, fmt.Errorf("acme: no valid certificate available for domain %q", domain)
}

// ServeHTTP01Handler handles incoming GET /.well-known/acme-challenge/{token} requests.
func (m *ACMEManager) ServeHTTP01Handler(req *httpparser.Request, res *httpparser.Response) {
	token := strings.TrimPrefix(req.Path, "/.well-known/acme-challenge/")
	token = strings.TrimSpace(token)

	keyAuth, exists := m.GetHTTP01Challenge(token)
	if !exists {
		res.SetStatus(http.StatusNotFound)
		res.Header.Set("Content-Type", "text/plain")
		_, _ = res.WriteString("404 ACME Challenge Token Not Found")
		return
	}

	res.SetStatus(http.StatusOK)
	res.Header.Set("Content-Type", "text/plain")
	_, _ = res.WriteString(keyAuth)
}

func loadOrGenerateAccountKey(keyPath string) (*ecdsa.PrivateKey, error) {
	if data, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err == nil {
				return key, nil
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	der, err := x509.MarshalECPrivateKey(key)
	if err == nil {
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
		_ = os.WriteFile(keyPath, pemBytes, 0600)
	}

	return key, nil
}

func (m *ACMEManager) loadCachedCertificates() error {
	entries, err := os.ReadDir(m.cfg.CacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".crt") {
			domain := strings.TrimSuffix(entry.Name(), ".crt")
			certFile := filepath.Join(m.cfg.CacheDir, entry.Name())
			keyFile := filepath.Join(m.cfg.CacheDir, domain+".key")

			if _, err := os.Stat(keyFile); err == nil {
				cert, err := tls.LoadX509KeyPair(certFile, keyFile)
				if err == nil {
					m.mu.Lock()
					m.certCache[strings.ToLower(domain)] = &cert
					m.mu.Unlock()
				}
			}
		}
	}
	return nil
}
