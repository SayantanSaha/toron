package acme_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

// TC-100-01: Valid Token GET Returns HTTP 200 OK with Key Authorization Body
func TestACME_HTTP01_ValidToken_GET_Success(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_valid_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	token := "LoqXcYV8q5ONbJQxWtGtTK-GwfeqST3gahAuhKhBpDU"
	expectedKeyAuth := token + ".account_thumbprint_abc123"
	mgr.SetHTTP01Challenge(token, expectedKeyAuth)

	req, err := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+token, "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	res := httpparser.NewResponse()

	mgr.ServeHTTP01Handler(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
	expectedCL := strconv.Itoa(len(expectedKeyAuth))
	if cl := res.Header.Get("Content-Length"); cl != expectedCL {
		t.Errorf("expected Content-Length %s, got %q", expectedCL, cl)
	}
	if res.Body.String() != expectedKeyAuth {
		t.Errorf("expected body %q, got %q", expectedKeyAuth, res.Body.String())
	}
}

// TC-100-02: Empty Token Returns HTTP 400 Bad Request
func TestACME_HTTP01_EmptyToken_Rejection(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_empty_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{
			name: "Scenario 2.1: trailing slash with empty token",
			path: "/.well-known/acme-challenge/",
		},
		{
			name: "Scenario 2.2: path without trailing slash",
			path: "/.well-known/acme-challenge",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := httpparser.NewRequest("GET", tc.path, "HTTP/1.1")
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			res := httpparser.NewResponse()

			mgr.ServeHTTP01Handler(req, res)

			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400 Bad Request, got %d", res.StatusCode)
			}
			if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
				t.Errorf("expected Content-Type text/plain, got %q", ct)
			}
			expectedBody := "400 Bad Request: Invalid ACME Challenge Token"
			if res.Body.String() != expectedBody {
				t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
			}
		})
	}
}

// TC-100-03: Token Length Boundary Enforcement (1 <= len <= 128)
func TestACME_HTTP01_TokenLengthBoundary(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_len_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	tests := []struct {
		name           string
		token          string
		expectedValid  bool
		expectedStatus int
	}{
		{
			name:           "Scenario 3.1: single character token",
			token:          "a",
			expectedValid:  true,
			expectedStatus: http.StatusNotFound, // Valid syntax, not registered
		},
		{
			name:           "Scenario 3.2: standard 43-character base64url token",
			token:          "LoqXcYV8q5ONbJQxWtGtTK-GwfeqST3gahAuhKhBpDU",
			expectedValid:  true,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Scenario 3.3: maximum boundary token (128 chars)",
			token:          strings.Repeat("a", 128),
			expectedValid:  true,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Scenario 3.4: oversized boundary token (129 chars)",
			token:          strings.Repeat("a", 129),
			expectedValid:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Scenario 3.5: double maximum token (256 chars)",
			token:          strings.Repeat("b", 256),
			expectedValid:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Scenario 3.6: large payload flood token (64 KB)",
			token:          strings.Repeat("c", 65536),
			expectedValid:  false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if valid := acme.IsValidACMEToken(tc.token); valid != tc.expectedValid {
				t.Errorf("IsValidACMEToken(%q) = %v, expected %v", tc.token, valid, tc.expectedValid)
			}

			req, err := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+tc.token, "HTTP/1.1")
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			res := httpparser.NewResponse()

			mgr.ServeHTTP01Handler(req, res)

			if res.StatusCode != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, res.StatusCode)
			}
			if tc.expectedStatus == http.StatusBadRequest {
				expectedBody := "400 Bad Request: Invalid ACME Challenge Token"
				if res.Body.String() != expectedBody {
					t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
				}
			}
		})
	}

	// Sub-test: Registered 128-byte token returns 200 OK
	t.Run("Registered 128-byte token returns 200 OK", func(t *testing.T) {
		token128 := strings.Repeat("x", 128)
		keyAuth128 := token128 + ".thumbprint"
		mgr.SetHTTP01Challenge(token128, keyAuth128)

		req, err := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+token128, "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		res := httpparser.NewResponse()

		mgr.ServeHTTP01Handler(req, res)

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for registered 128-byte token, got %d", res.StatusCode)
		}
		if res.Body.String() != keyAuth128 {
			t.Errorf("expected body %q, got %q", keyAuth128, res.Body.String())
		}
	})
}

// TC-100-04: Invalid Character Set Rejection
func TestACME_HTTP01_InvalidCharacters_Rejection(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_chars_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "Scenario 4.1: base64 padding ==", token: "token_with_padding=="},
		{name: "Scenario 4.2: standard base64 plus +", token: "token+with+plus"},
		{name: "Scenario 4.3: at-sign symbol @", token: "token@domain"},
		{name: "Scenario 4.4: asterisk symbol *", token: "token*star"},
		{name: "Scenario 4.5: ascii space 0x20", token: "token with spaces"},
		{name: "Scenario 4.6: leading whitespace", token: "  leading_space"},
		{name: "Scenario 4.7: trailing whitespace", token: "trailing_space  "},
		{name: "Scenario 4.8: tab character \\t", token: "token\twith\ttab"},
		{name: "Scenario 4.9: newline character \\n", token: "token\nwith\nnewline"},
		{name: "Scenario 4.10: null byte \\x00", token: "token\x00null"},
		{name: "Scenario 4.11: control character 0x1F", token: "token\x1fctrl"},
		{name: "Scenario 4.12: multibyte UTF-8 non-ASCII", token: "token_üñîçødé"},
		{name: "Scenario 4.13: 4-byte UTF-8 emoji", token: "token_🚀_emoji"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if valid := acme.IsValidACMEToken(tc.token); valid {
				t.Errorf("IsValidACMEToken(%q) should be false", tc.token)
			}

			req, err := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+tc.token, "HTTP/1.1")
			if err != nil {
				// If net/url rejects raw control characters in URI, construct Request directly with raw Path
				req = &httpparser.Request{
					Method: "GET",
					Path:   "/.well-known/acme-challenge/" + tc.token,
					Proto:  "HTTP/1.1",
					Header: make(httpparser.Header),
				}
			}
			res := httpparser.NewResponse()

			mgr.ServeHTTP01Handler(req, res)

			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400 Bad Request, got %d", res.StatusCode)
			}
			if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
				t.Errorf("expected Content-Type text/plain, got %q", ct)
			}
			expectedBody := "400 Bad Request: Invalid ACME Challenge Token"
			if res.Body.String() != expectedBody {
				t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
			}
		})
	}
}

// TC-100-05: Path Traversal & Separator Rejection
func TestACME_HTTP01_PathTraversal_Rejection(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_traversal_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "Scenario 5.1: parent directory reference ..", token: ".."},
		{name: "Scenario 5.2: directory traversal escape ../etc/passwd", token: "../etc/passwd"},
		{name: "Scenario 5.3: subpath separator /", token: "foo/bar"},
		{name: "Scenario 5.4: windows path separator \\", token: `foo\bar`},
		{name: "Scenario 5.5: current directory reference .", token: "."},
		{name: "Scenario 5.6: embedded dot token.with.dot", token: "token.with.dot"},
		{name: "Scenario 5.7: multiple dots ...", token: "..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if valid := acme.IsValidACMEToken(tc.token); valid {
				t.Errorf("IsValidACMEToken(%q) should be false", tc.token)
			}

			req, err := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+tc.token, "HTTP/1.1")
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			res := httpparser.NewResponse()

			mgr.ServeHTTP01Handler(req, res)

			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400 Bad Request, got %d", res.StatusCode)
			}
			expectedBody := "400 Bad Request: Invalid ACME Challenge Token"
			if res.Body.String() != expectedBody {
				t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
			}
		})
	}
}

// TC-100-06: HTTP Method Hardening & 405 Rejection
func TestACME_HTTP01_MethodValidation_405MethodNotAllowed(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_methods_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	token := "valid_token"
	keyAuth := "key_auth_payload"
	mgr.SetHTTP01Challenge(token, keyAuth)

	disallowedMethods := []string{
		"POST",
		"PUT",
		"DELETE",
		"PATCH",
		"OPTIONS",
		"CONNECT",
		"TRACE",
	}

	for _, method := range disallowedMethods {
		t.Run("Method: "+method, func(t *testing.T) {
			req, err := httpparser.NewRequest(method, "/.well-known/acme-challenge/"+token, "HTTP/1.1")
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			res := httpparser.NewResponse()

			mgr.ServeHTTP01Handler(req, res)

			if res.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("expected 405 Method Not Allowed, got %d", res.StatusCode)
			}
			if allow := res.Header.Get("Allow"); allow != "GET, HEAD" {
				t.Errorf("expected Allow header 'GET, HEAD', got %q", allow)
			}
			if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
				t.Errorf("expected Content-Type 'text/plain', got %q", ct)
			}
			expectedBody := "405 Method Not Allowed: Only GET and HEAD methods are permitted"
			if res.Body.String() != expectedBody {
				t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
			}
		})
	}
}

// TC-100-07: RFC 7231 HEAD Request Returns 200 OK with Headers and Empty Body
func TestACME_HTTP01_HEAD_Semantics(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_head_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	token := "head_test_token_123"
	expectedKeyAuth := "head_test_token_123.thumbprint_xyz"
	mgr.SetHTTP01Challenge(token, expectedKeyAuth)

	req, err := httpparser.NewRequest("HEAD", "/.well-known/acme-challenge/"+token, "HTTP/1.1")
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	res := httpparser.NewResponse()

	mgr.ServeHTTP01Handler(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got %q", ct)
	}
	expectedCL := strconv.Itoa(len(expectedKeyAuth))
	if cl := res.Header.Get("Content-Length"); cl != expectedCL {
		t.Errorf("expected Content-Length %s, got %q", expectedCL, cl)
	}
	if res.Body.Len() != 0 {
		t.Errorf("expected empty body for HEAD request, got %d bytes: %q", res.Body.Len(), res.Body.String())
	}
	if res.Body.String() != "" {
		t.Errorf("expected empty body string, got %q", res.Body.String())
	}
}

// TC-100-08: Non-Existent Valid Token Returns HTTP 404 Not Found
func TestACME_HTTP01_NonExistentToken_404NotFound(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_404_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	token := "valid_unregistered_token_abc"

	// Scenario 1: GET on missing token -> 404 with error body
	t.Run("GET on non-existent token", func(t *testing.T) {
		req, err := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+token, "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		res := httpparser.NewResponse()

		mgr.ServeHTTP01Handler(req, res)

		if res.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found, got %d", res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("expected Content-Type 'text/plain', got %q", ct)
		}
		expectedBody := "404 ACME Challenge Token Not Found"
		if res.Body.String() != expectedBody {
			t.Errorf("expected body %q, got %q", expectedBody, res.Body.String())
		}
	})

	// Scenario 2: HEAD on missing token -> 404 with empty body
	t.Run("HEAD on non-existent token", func(t *testing.T) {
		req, err := httpparser.NewRequest("HEAD", "/.well-known/acme-challenge/"+token, "HTTP/1.1")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		res := httpparser.NewResponse()

		mgr.ServeHTTP01Handler(req, res)

		if res.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found, got %d", res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("expected Content-Type 'text/plain', got %q", ct)
		}
		if res.Body.Len() != 0 {
			t.Errorf("expected empty body for HEAD on missing token, got %d bytes: %q", res.Body.Len(), res.Body.String())
		}
	})
}

// TC-100-09: High-Concurrency Race Safety under go test -race
func TestACME_HTTP01_HighConcurrency_RaceSafety(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "acme_http01_concurrency_*")
	defer os.RemoveAll(tmpDir)

	mgr, err := acme.NewACMEManager(acme.ACMEConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create ACMEManager: %v", err)
	}

	const baseToken = "concurrent_base_token"
	mgr.SetHTTP01Challenge(baseToken, "base_key_auth_payload")

	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	// 1. Concurrent reader workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch workerID % 5 {
				case 0:
					// GET on active base token (200 OK)
					req, _ := httpparser.NewRequest("GET", "/.well-known/acme-challenge/"+baseToken, "HTTP/1.1")
					res := httpparser.NewResponse()
					mgr.ServeHTTP01Handler(req, res)
					if res.StatusCode != http.StatusOK {
						t.Errorf("worker %d: expected 200, got %d", workerID, res.StatusCode)
					}
				case 1:
					// HEAD on active base token (200 OK, empty body)
					req, _ := httpparser.NewRequest("HEAD", "/.well-known/acme-challenge/"+baseToken, "HTTP/1.1")
					res := httpparser.NewResponse()
					mgr.ServeHTTP01Handler(req, res)
					if res.StatusCode != http.StatusOK {
						t.Errorf("worker %d: expected 200, got %d", workerID, res.StatusCode)
					}
					if res.Body.Len() != 0 {
						t.Errorf("worker %d: expected empty body on HEAD, got %d", workerID, res.Body.Len())
					}
				case 2:
					// Disallowed method (405)
					req, _ := httpparser.NewRequest("POST", "/.well-known/acme-challenge/"+baseToken, "HTTP/1.1")
					res := httpparser.NewResponse()
					mgr.ServeHTTP01Handler(req, res)
					if res.StatusCode != http.StatusMethodNotAllowed {
						t.Errorf("worker %d: expected 405, got %d", workerID, res.StatusCode)
					}
				case 3:
					// Invalid token (400)
					req, _ := httpparser.NewRequest("GET", "/.well-known/acme-challenge/invalid..token", "HTTP/1.1")
					res := httpparser.NewResponse()
					mgr.ServeHTTP01Handler(req, res)
					if res.StatusCode != http.StatusBadRequest {
						t.Errorf("worker %d: expected 400, got %d", workerID, res.StatusCode)
					}
				case 4:
					// Non-existent token (404)
					req, _ := httpparser.NewRequest("GET", fmt.Sprintf("/.well-known/acme-challenge/missing_token_%d_%d", workerID, i), "HTTP/1.1")
					res := httpparser.NewResponse()
					mgr.ServeHTTP01Handler(req, res)
					if res.StatusCode != http.StatusNotFound {
						t.Errorf("worker %d: expected 404, got %d", workerID, res.StatusCode)
					}
				}
			}
		}(w)
	}

	// 2. Concurrent writer workers modifying challenge map
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				dynamicToken := fmt.Sprintf("dyn_tok_%d_%d", writerID, i)
				mgr.SetHTTP01Challenge(dynamicToken, "dyn_auth")
				time.Sleep(100 * time.Microsecond)
				mgr.RemoveHTTP01Challenge(dynamicToken)
			}
		}(w)
	}

	wg.Wait()
}

// Unit test for IsValidACMEToken
func TestACME_IsValidACMEToken(t *testing.T) {
	tests := []struct {
		token string
		valid bool
	}{
		{"", false},
		{"a", true},
		{"Z", true},
		{"0", true},
		{"9", true},
		{"-", true},
		{"_", true},
		{"a-b_c-1_2-3", true},
		{strings.Repeat("x", 128), true},
		{strings.Repeat("x", 129), false},
		{"token=", false},
		{"token==", false},
		{"token+", false},
		{"token/", false},
		{"token\\", false},
		{"token.ext", false},
		{"token with space", false},
		{" token", false},
		{"token ", false},
		{"token\t", false},
		{"token\n", false},
		{"token\x00", false},
		{"token\x7f", false},
		{"töken", false},
	}

	for _, tc := range tests {
		if got := acme.IsValidACMEToken(tc.token); got != tc.valid {
			t.Errorf("IsValidACMEToken(%q) = %v, expected %v", tc.token, got, tc.valid)
		}
	}
}
