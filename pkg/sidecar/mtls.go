package sidecar

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"toron/pkg/config"
)

// BuildServerTLSConfig constructs a server-side TLS configuration enforcing strict pod-to-pod mTLS.
func BuildServerTLSConfig(cfg config.SidecarConfig) (*tls.Config, error) {
	if !cfg.StrictmTLS && cfg.CertFile == "" {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load sidecar tls cert/key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if cfg.StrictmTLS {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if cfg.CAFile != "" {
			caBytes, err := os.ReadFile(cfg.CAFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read sidecar root ca cert: %w", err)
			}
			caPool := x509.NewCertPool()
			caPool.AppendCertsFromPEM(caBytes)
			tlsConfig.ClientCAs = caPool
		}
	}

	return tlsConfig, nil
}

// BuildClientTLSConfig constructs a client-side TLS configuration for egress pod-to-pod mTLS.
func BuildClientTLSConfig(cfg config.SidecarConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load sidecar client tls cert/key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if cfg.CAFile != "" {
		caBytes, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read sidecar ca cert: %w", err)
		}
		caPool := x509.NewCertPool()
		caPool.AppendCertsFromPEM(caBytes)
		tlsConfig.RootCAs = caPool
	} else {
		// Default to insecure verify if custom CA is omitted in test environments
		tlsConfig.InsecureSkipVerify = true
	}

	return tlsConfig, nil
}
