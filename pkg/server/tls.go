package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"
)

// GenerateDevCert creates an in-memory self-signed X.509 certificate for development testing.
func GenerateDevCert() (tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("tls: failed to generate private key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("tls: failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Toron Web Server Development"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "toron.local", "api.toron.local"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("tls: failed to create certificate: %w", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privKey,
	}

	return cert, nil
}

// CreateTLSConfig constructs a tls.Config with ALPN next-protocols and cert loading.
func CreateTLSConfig(cfg Config) (*tls.Config, error) {
	var cert tls.Certificate
	var err error

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err = tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("tls: failed to load keypair (%s, %s): %w", cfg.TLSCertFile, cfg.TLSKeyFile, err)
		}
	} else if cfg.TLSAutoDevCert || (cfg.TLSCertFile == "" && cfg.TLSKeyFile == "") {
		cert, err = GenerateDevCert()
		if err != nil {
			return nil, fmt.Errorf("tls: failed to generate self-signed dev certificate: %w", err)
		}
	} else {
		return nil, fmt.Errorf("tls: invalid certificate parameters")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	}

	if cfg.SNIRegistry != nil {
		cfg.SNIRegistry.SetFallbackConfig(tlsConfig)
		tlsConfig.GetConfigForClient = cfg.SNIRegistry.GetConfigForClient
	}

	return tlsConfig, nil
}

// ListenTLS binds a TCP listener and wraps it in a TLS listener.
func ListenTLS(network, laddr string, cfg Config) (net.Listener, error) {
	tlsConfig, err := CreateTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen(network, laddr)
	if err != nil {
		return nil, err
	}

	return tls.NewListener(ln, tlsConfig), nil
}
