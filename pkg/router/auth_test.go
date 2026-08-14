package router

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"toron/pkg/httpparser"
)

func TestAuth_JWT_Valid(t *testing.T) {
	secret := []byte("super-secret-key-1234567890123456")
	authCfg := AuthConfig{
		Type: AuthTypeJWT,
		JWT: JWTConfig{
			Secret:   string(secret),
			Issuer:   "toron-auth",
			Audience: "api.toron.local",
		},
	}

	r := New()
	r.Use(NewAuthMiddleware(authCfg))

	var receivedUser string
	r.GET("/secure/data", func(req *httpparser.Request, res *httpparser.Response) {
		receivedUser = req.Header.Get("X-Authenticated-User")
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString(`{"status":"authorized"}`)
	})

	// Generate valid token
	now := time.Now().Unix()
	claims := map[string]any{
		"iss":  "toron-auth",
		"aud":  "api.toron.local",
		"sub":  "alice_99",
		"iat":  now,
		"exp":  now + 3600, // 1 hour valid
		"role": "admin",
	}
	token, err := SignJWT(JWTHeader{Alg: "HS256", Typ: "JWT"}, claims, secret)
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}

	req, _ := httpparser.NewRequest("GET", "/secure/data", "HTTP/1.1")
	req.Header.Set("Authorization", "Bearer "+token)
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", res.StatusCode, res.Body.String())
	}
	if receivedUser != "alice_99" {
		t.Fatalf("expected authenticated user 'alice_99', got %q", receivedUser)
	}
}

func TestAuth_JWT_Expired(t *testing.T) {
	secret := []byte("secret-key")
	authCfg := AuthConfig{
		Type: AuthTypeJWT,
		JWT: JWTConfig{
			Secret: string(secret),
		},
	}

	r := New()
	r.Use(NewAuthMiddleware(authCfg))

	r.GET("/secure", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	// Expired token (exp in past)
	claims := map[string]any{
		"sub": "bob",
		"exp": time.Now().Unix() - 100,
	}
	token, _ := SignJWT(JWTHeader{Alg: "HS256"}, claims, secret)

	req, _ := httpparser.NewRequest("GET", "/secure", "HTTP/1.1")
	req.Header.Set("Authorization", "Bearer "+token)
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for expired JWT, got %d", res.StatusCode)
	}
}

func TestAuth_JWT_TamperedSignature(t *testing.T) {
	secret := []byte("secret-key")
	otherSecret := []byte("wrong-secret-key")

	authCfg := AuthConfig{
		Type: AuthTypeJWT,
		JWT: JWTConfig{
			Secret: string(secret),
		},
	}

	r := New()
	r.Use(NewAuthMiddleware(authCfg))

	r.GET("/secure", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	// Sign with wrong secret
	claims := map[string]any{
		"sub": "charlie",
		"exp": time.Now().Unix() + 1000,
	}
	token, _ := SignJWT(JWTHeader{Alg: "HS256"}, claims, otherSecret)

	req, _ := httpparser.NewRequest("GET", "/secure", "HTTP/1.1")
	req.Header.Set("Authorization", "Bearer "+token)
	res := httpparser.NewResponse()

	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for tampered JWT, got %d", res.StatusCode)
	}
}

func TestAuth_JWT_IssuerAndAudience(t *testing.T) {
	secret := []byte("secret-key")
	authCfg := AuthConfig{
		Type: AuthTypeJWT,
		JWT: JWTConfig{
			Secret:   string(secret),
			Issuer:   "auth-service",
			Audience: "gateway-api",
		},
	}

	r := New()
	r.Use(NewAuthMiddleware(authCfg))

	r.GET("/secure", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
	})

	// Wrong issuer
	claims := map[string]any{
		"iss": "wrong-issuer",
		"aud": "gateway-api",
		"sub": "user1",
		"exp": time.Now().Unix() + 1000,
	}
	token, _ := SignJWT(JWTHeader{Alg: "HS256"}, claims, secret)

	req, _ := httpparser.NewRequest("GET", "/secure", "HTTP/1.1")
	req.Header.Set("Authorization", "Bearer "+token)
	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for wrong issuer, got %d", res.StatusCode)
	}

	// Wrong audience
	claims["iss"] = "auth-service"
	claims["aud"] = "wrong-aud"
	token2, _ := SignJWT(JWTHeader{Alg: "HS256"}, claims, secret)

	req2, _ := httpparser.NewRequest("GET", "/secure", "HTTP/1.1")
	req2.Header.Set("Authorization", "Bearer "+token2)
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for wrong audience, got %d", res2.StatusCode)
	}
}

func TestAuth_APIKey_ValidAndInvalid(t *testing.T) {
	authCfg := AuthConfig{
		Type: AuthTypeAPIKey,
		APIKey: APIKeyConfig{
			Keys:   []string{"secret-key-1", "secret-key-2"},
			Header: "X-API-Key",
			Query:  "api_key",
		},
	}

	r := New()
	r.Use(NewAuthMiddleware(authCfg))

	r.GET("/api/data", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("api data")
	})

	// Valid header key
	req1, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
	req1.Header.Set("X-API-Key", "secret-key-1")
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for valid API key header, got %d", res1.StatusCode)
	}

	// Valid query key
	req2, _ := httpparser.NewRequest("GET", "/api/data?api_key=secret-key-2", "HTTP/1.1")
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for valid API key query, got %d", res2.StatusCode)
	}

	// Invalid key
	req3, _ := httpparser.NewRequest("GET", "/api/data", "HTTP/1.1")
	req3.Header.Set("X-API-Key", "invalid-key")
	res3 := httpparser.NewResponse()
	r.ServeHTTP(req3, res3)

	if res3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for invalid API key, got %d", res3.StatusCode)
	}
}

func TestAuth_BasicAuth_ValidAndInvalid(t *testing.T) {
	authCfg := AuthConfig{
		Type: AuthTypeBasic,
		Basic: BasicAuthConfig{
			Users: map[string]string{
				"admin": "superpass123",
			},
			Realm: "Toron Gate",
		},
	}

	r := New()
	r.Use(NewAuthMiddleware(authCfg))

	var authenticatedUser string
	r.GET("/admin", func(req *httpparser.Request, res *httpparser.Response) {
		authenticatedUser = req.Header.Get("X-Authenticated-User")
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("admin portal")
	})

	// Valid Basic auth
	validCredentials := base64.StdEncoding.EncodeToString([]byte("admin:superpass123"))
	req1, _ := httpparser.NewRequest("GET", "/admin", "HTTP/1.1")
	req1.Header.Set("Authorization", "Basic "+validCredentials)
	res1 := httpparser.NewResponse()
	r.ServeHTTP(req1, res1)

	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for valid Basic auth, got %d", res1.StatusCode)
	}
	if authenticatedUser != "admin" {
		t.Fatalf("expected authenticated user 'admin', got %q", authenticatedUser)
	}

	// Invalid Basic auth
	invalidCredentials := base64.StdEncoding.EncodeToString([]byte("admin:wrongpass"))
	req2, _ := httpparser.NewRequest("GET", "/admin", "HTTP/1.1")
	req2.Header.Set("Authorization", "Basic "+invalidCredentials)
	res2 := httpparser.NewResponse()
	r.ServeHTTP(req2, res2)

	if res2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for invalid Basic auth, got %d", res2.StatusCode)
	}
	if wwwAuth := res2.Header.Get("WWW-Authenticate"); wwwAuth != `Basic realm="Toron Gate"` {
		t.Fatalf("expected WWW-Authenticate header, got %q", wwwAuth)
	}
}

func TestAuth_ExcludedPaths(t *testing.T) {
	authCfg := AuthConfig{
		Type: AuthTypeAPIKey,
		APIKey: APIKeyConfig{
			Keys: []string{"secret"},
		},
		Excluded: []string{"/public", "/health"},
	}

	r := New()
	r.Use(NewAuthMiddleware(authCfg))

	r.GET("/health", func(req *httpparser.Request, res *httpparser.Response) {
		res.SetStatus(http.StatusOK)
		_, _ = res.WriteString("healthy")
	})

	// Excluded path accessed without API key
	req, _ := httpparser.NewRequest("GET", "/health", "HTTP/1.1")
	res := httpparser.NewResponse()
	r.ServeHTTP(req, res)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 on excluded path without key, got %d", res.StatusCode)
	}
}
