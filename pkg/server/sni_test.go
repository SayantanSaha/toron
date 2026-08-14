package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// generateTestCert creates an in-memory certificate for testing.
func generateTestCert(commonName string, dnsNames []string, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (tls.Certificate, *x509.Certificate, *ecdsa.PrivateKey, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Toron SNI Test"},
			CommonName:   commonName,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		DNSNames:              dnsNames,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	signTemplate := &template
	signKey := privKey
	if parent != nil && parentKey != nil {
		signTemplate = parent
		signKey = parentKey
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, signTemplate, &privKey.PublicKey, signKey)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}

	parsedCert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privKey,
	}

	return tlsCert, parsedCert, privKey, nil
}

func TestSNI_MultiCertificateDispatching(t *testing.T) {
	// 1. Generate default dev cert
	defaultCert, _, _, err := generateTestCert("default.local", []string{"default.local"}, false, nil, nil)
	if err != nil {
		t.Fatalf("failed to generate default cert: %v", err)
	}

	// 2. Generate Host A cert
	certA, parsedA, _, err := generateTestCert("site-a.example.com", []string{"site-a.example.com"}, false, nil, nil)
	if err != nil {
		t.Fatalf("failed to generate cert A: %v", err)
	}

	// 3. Generate Host B cert
	certB, parsedB, _, err := generateTestCert("site-b.example.com", []string{"site-b.example.com"}, false, nil, nil)
	if err != nil {
		t.Fatalf("failed to generate cert B: %v", err)
	}

	sniReg := NewSNIRegistry(&tls.Config{
		Certificates: []tls.Certificate{defaultCert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	})

	sniReg.RegisterProfile("site-a.example.com", certA, nil, tls.NoClientCert, tls.VersionTLS12)
	sniReg.RegisterProfile("site-b.example.com", certB, nil, tls.NoClientCert, tls.VersionTLS12)

	serverTLS := &tls.Config{
		GetConfigForClient: sniReg.GetConfigForClient,
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	ts.TLS = serverTLS
	ts.StartTLS()
	defer ts.Close()

	// Helper client test
	testClient := func(serverName string) *x509.Certificate {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         serverName,
				InsecureSkipVerify: true,
			},
		}
		client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
		resp, err := client.Get(ts.URL)
		if err != nil {
			t.Fatalf("failed GET for %s: %v", serverName, err)
		}
		defer resp.Body.Close()
		if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
			t.Fatalf("no peer certificates returned for %s", serverName)
		}
		return resp.TLS.PeerCertificates[0]
	}

	// Verify Site A gets Cert A
	peerA := testClient("site-a.example.com")
	if peerA.Subject.CommonName != parsedA.Subject.CommonName {
		t.Errorf("expected common name %s, got %s", parsedA.Subject.CommonName, peerA.Subject.CommonName)
	}

	// Verify Site B gets Cert B
	peerB := testClient("site-b.example.com")
	if peerB.Subject.CommonName != parsedB.Subject.CommonName {
		t.Errorf("expected common name %s, got %s", parsedB.Subject.CommonName, peerB.Subject.CommonName)
	}

	// Verify Unknown host gets Default Cert
	peerDefault := testClient("unknown.example.com")
	if peerDefault.Subject.CommonName != "default.local" {
		t.Errorf("expected common name default.local, got %s", peerDefault.Subject.CommonName)
	}
}

func TestSNI_mTLS_ValidAndInvalidClientCert(t *testing.T) {
	// 1. Root CA
	_, caCert, caKey, err := generateTestCert("Test Root CA", nil, true, nil, nil)
	if err != nil {
		t.Fatalf("failed to generate root CA: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// 2. Server Cert for secure.internal
	serverCert, _, _, err := generateTestCert("secure.internal", []string{"secure.internal"}, false, nil, nil)
	if err != nil {
		t.Fatalf("failed to generate server cert: %v", err)
	}

	// 3. Valid Client Cert signed by Root CA
	validClientCert, _, _, err := generateTestCert("client-alice", nil, false, caCert, caKey)
	if err != nil {
		t.Fatalf("failed to generate valid client cert: %v", err)
	}

	// 4. Untrusted Client Cert signed by unrelated key
	untrustedClientCert, _, _, err := generateTestCert("client-mallory", nil, false, nil, nil)
	if err != nil {
		t.Fatalf("failed to generate untrusted client cert: %v", err)
	}

	sniReg := NewSNIRegistry(nil)
	sniReg.RegisterProfile("secure.internal", serverCert, caPool, tls.RequireAndVerifyClientCert, tls.VersionTLS12)

	serverTLS := &tls.Config{
		GetConfigForClient: sniReg.GetConfigForClient,
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mtls success"))
	}))
	ts.TLS = serverTLS
	ts.StartTLS()
	defer ts.Close()

	// Scenario A: Client with valid CA-signed cert -> Success
	trValid := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         "secure.internal",
			Certificates:       []tls.Certificate{validClientCert},
			InsecureSkipVerify: true,
		},
	}
	clientValid := &http.Client{Transport: trValid, Timeout: 3 * time.Second}
	resp, err := clientValid.Get(ts.URL)
	if err != nil {
		t.Fatalf("expected mTLS handshake to succeed with valid cert, got error: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Scenario B: Client without client cert -> Handshake Rejected
	trNoCert := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         "secure.internal",
			InsecureSkipVerify: true,
		},
	}
	clientNoCert := &http.Client{Transport: trNoCert, Timeout: 2 * time.Second}
	_, err = clientNoCert.Get(ts.URL)
	if err == nil {
		t.Fatalf("expected mTLS handshake to fail when no client cert is provided")
	}

	// Scenario C: Client with untrusted client cert -> Handshake Rejected
	trUntrusted := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         "secure.internal",
			Certificates:       []tls.Certificate{untrustedClientCert},
			InsecureSkipVerify: true,
		},
	}
	clientUntrusted := &http.Client{Transport: trUntrusted, Timeout: 2 * time.Second}
	_, err = clientUntrusted.Get(ts.URL)
	if err == nil {
		t.Fatalf("expected mTLS handshake to fail with untrusted client cert")
	}
}

func TestSNI_MinTLSVersionEnforcement(t *testing.T) {
	serverCert, _, _, err := generateTestCert("tls13.internal", []string{"tls13.internal"}, false, nil, nil)
	if err != nil {
		t.Fatalf("failed to generate server cert: %v", err)
	}

	sniReg := NewSNIRegistry(nil)
	sniReg.RegisterProfile("tls13.internal", serverCert, nil, tls.NoClientCert, tls.VersionTLS13)

	serverTLS := &tls.Config{
		GetConfigForClient: sniReg.GetConfigForClient,
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("tls1.3 ok"))
	}))
	ts.TLS = serverTLS
	ts.StartTLS()
	defer ts.Close()

	// Client forcing max version TLS 1.2 -> Should fail handshake
	trTLS12 := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         "tls13.internal",
			MaxVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		},
	}
	clientTLS12 := &http.Client{Transport: trTLS12, Timeout: 2 * time.Second}
	_, err = clientTLS12.Get(ts.URL)
	if err == nil {
		t.Fatalf("expected handshake failure when client uses TLS 1.2 against TLS 1.3 enforced host")
	}
}
