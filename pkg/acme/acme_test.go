package acme_test

import (
	"crypto/tls"
	"net/http"
	"os"
	"strings"
	"testing"

	"toron/pkg/acme"
	"toron/pkg/httpparser"
)

func TestACME_KeyAuthorization(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "acme_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := acme.ACMEConfig{
		Enabled:      true,
		CacheDir:     tmpDir,
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
	}

	mgr, err := acme.NewACMEManager(cfg)
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	token := "sample_token_123"
	keyAuth := mgr.ComputeKeyAuthorization(token)

	if !strings.HasPrefix(keyAuth, "sample_token_123.") {
		t.Errorf("expected key authorization to start with token prefix, got %q", keyAuth)
	}
}

func TestACME_HTTP01ChallengeResponder(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	token := "token_abc_xyz"
	expectedKeyAuth := mgr.ComputeKeyAuthorization(token)
	mgr.SetHTTP01Challenge(token, expectedKeyAuth)

	req, _ := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+token, "HTTP/1.1")
	res := httpparser.NewResponse()

	mgr.ServeHTTP01Handler(req, res)

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.Body.String() != expectedKeyAuth {
		t.Errorf("expected key auth %q, got %q", expectedKeyAuth, res.Body.String())
	}
}

func TestACME_TLSALPN01CertificateGeneration(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_alpn_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	domain := "api.toron.local"
	keyAuth := mgr.ComputeKeyAuthorization("token_alpn")

	cert, err := mgr.GenerateTLSALPN01Certificate(domain, keyAuth)
	if err != nil {
		t.Fatalf("failed to generate ALPN cert: %v", err)
	}

	mgr.SetTLSALPN01Challenge(domain, cert)

	clientHello := &tls.ClientHelloInfo{
		ServerName:      domain,
		SupportedProtos: []string{"acme-tls/1"},
	}

	resCert, err := mgr.GetCertificate(clientHello)
	if err != nil || resCert == nil {
		t.Fatalf("expected ALPN challenge cert for domain %q, got error %v", domain, err)
	}
}
