package router

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"net/http"
	"strings"
	"time"

	"toron/pkg/httpparser"
)

// AuthType describes the supported authentication strategy.
type AuthType string

const (
	AuthTypeNone   AuthType = ""
	AuthTypeJWT    AuthType = "jwt"
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeBasic  AuthType = "basic"
)

// JWTConfig defines settings for JWT verification.
type JWTConfig struct {
	Secret            string `yaml:"secret" json:"secret"`
	Issuer            string `yaml:"issuer" json:"issuer"`
	Audience          string `yaml:"audience" json:"audience"`
	RequireExpiration bool   `yaml:"require_expiration" json:"require_expiration"`
	AllowNoExpiration bool   `yaml:"allow_no_expiration" json:"allow_no_expiration"`
}

// APIKeyConfig defines settings for API key validation.
type APIKeyConfig struct {
	Keys   []string `yaml:"keys" json:"keys"`
	Header string   `yaml:"header" json:"header"`
	Query  string   `yaml:"query" json:"query"`
}

// BasicAuthConfig defines settings for HTTP Basic authentication.
type BasicAuthConfig struct {
	Users map[string]string `yaml:"users" json:"users"` // username -> password
	Realm string            `yaml:"realm" json:"realm"`
}

// AuthConfig defines multi-scheme authentication middleware settings.
type AuthConfig struct {
	Type     AuthType        `yaml:"type" json:"type"`
	JWT      JWTConfig       `yaml:"jwt" json:"jwt"`
	APIKey   APIKeyConfig    `yaml:"api_key" json:"api_key"`
	Basic    BasicAuthConfig `yaml:"basic" json:"basic"`
	Excluded []string        `yaml:"excluded" json:"excluded"`
}

// JWTHeader represents standard RFC 7519 JOSE Header fields.
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

func decodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func encodeBase64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// SignJWT creates a signed JWT string using HMAC algorithms (HS256, HS384, HS512).
func SignJWT(header JWTHeader, claims map[string]any, secret []byte) (string, error) {
	if header.Alg == "" {
		header.Alg = "HS256"
	}
	if header.Typ == "" {
		header.Typ = "JWT"
	}

	hdrJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("jwt: failed to marshal header: %w", err)
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("jwt: failed to marshal claims: %w", err)
	}

	part1 := encodeBase64URL(hdrJSON)
	part2 := encodeBase64URL(claimsJSON)
	signingInput := part1 + "." + part2

	var h hash.Hash
	switch strings.ToUpper(header.Alg) {
	case "HS256":
		h = hmac.New(sha256.New, secret)
	case "HS384":
		h = hmac.New(sha512.New384, secret)
	case "HS512":
		h = hmac.New(sha512.New, secret)
	default:
		return "", fmt.Errorf("jwt: unsupported signing algorithm %q", header.Alg)
	}

	h.Write([]byte(signingInput))
	sig := encodeBase64URL(h.Sum(nil))

	return signingInput + "." + sig, nil
}

// VerifyJWT validates HMAC-signed JWT tokens and claims.
func VerifyJWT(tokenString string, secret []byte, expectedIssuer, expectedAudience string, requireExpOpt ...bool) (map[string]any, error) {
	requireExp := true
	if len(requireExpOpt) > 0 {
		requireExp = requireExpOpt[0]
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format (expected 3 parts)")
	}

	headerBytes, err := decodeBase64URL(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header encoding: %w", err)
	}

	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding: %w", err)
	}

	signatureBytes, err := decodeBase64URL(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	var hdr JWTHeader
	if err := json.Unmarshal(headerBytes, &hdr); err != nil {
		return nil, fmt.Errorf("invalid header json: %w", err)
	}

	var h hash.Hash
	switch strings.ToUpper(hdr.Alg) {
	case "HS256":
		h = hmac.New(sha256.New, secret)
	case "HS384":
		h = hmac.New(sha512.New384, secret)
	case "HS512":
		h = hmac.New(sha512.New, secret)
	default:
		return nil, fmt.Errorf("unsupported or missing algorithm %q", hdr.Alg)
	}

	if len(secret) == 0 {
		return nil, fmt.Errorf("empty secret key not permitted")
	}

	signingInput := parts[0] + "." + parts[1]
	h.Write([]byte(signingInput))
	expectedSignature := h.Sum(nil)

	if subtle.ConstantTimeCompare(signatureBytes, expectedSignature) != 1 {
		return nil, fmt.Errorf("signature verification failed")
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims json: %w", err)
	}

	now := time.Now().Unix()

	// Validate exp claim
	expVal, hasExp := claims["exp"]
	if !hasExp {
		if requireExp {
			return nil, fmt.Errorf("token missing required exp claim")
		}
	} else {
		var exp int64
		switch v := expVal.(type) {
		case float64:
			exp = int64(v)
		case int64:
			exp = v
		case json.Number:
			exp, _ = v.Int64()
		}
		if exp <= 0 {
			if requireExp {
				return nil, fmt.Errorf("token missing required exp claim")
			}
		} else if now >= exp {
			return nil, fmt.Errorf("token has expired")
		}
	}

	// Validate nbf claim
	if nbfVal, ok := claims["nbf"]; ok {
		var nbf int64
		switch v := nbfVal.(type) {
		case float64:
			nbf = int64(v)
		case int64:
			nbf = v
		case json.Number:
			nbf, _ = v.Int64()
		}
		if nbf > 0 && now < nbf {
			return nil, fmt.Errorf("token not active yet (nbf)")
		}
	}

	// Validate iss claim
	if expectedIssuer != "" {
		if iss, ok := claims["iss"].(string); !ok || iss != expectedIssuer {
			return nil, fmt.Errorf("issuer mismatch: expected %q", expectedIssuer)
		}
	}

	// Validate aud claim
	if expectedAudience != "" {
		audMatch := false
		if audStr, ok := claims["aud"].(string); ok && audStr == expectedAudience {
			audMatch = true
		} else if audArr, ok := claims["aud"].([]any); ok {
			for _, a := range audArr {
				if aStr, ok := a.(string); ok && aStr == expectedAudience {
					audMatch = true
					break
				}
			}
		}
		if !audMatch {
			return nil, fmt.Errorf("audience mismatch: expected %q", expectedAudience)
		}
	}

	return claims, nil
}

func verifyAPIKey(req *httpparser.Request, cfg APIKeyConfig) (string, bool) {
	headerName := cfg.Header
	if headerName == "" {
		headerName = "X-API-Key"
	}

	key := req.Header.Get(headerName)
	if key == "" {
		authHdr := req.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(authHdr), "apikey ") {
			key = strings.TrimSpace(authHdr[7:])
		}
	}
	if key == "" && cfg.Query != "" && req.URL != nil {
		key = req.URL.Query().Get(cfg.Query)
	}
	if key == "" && req.URL != nil {
		key = req.URL.Query().Get("api_key")
	}

	if key == "" {
		return "", false
	}

	// Hash incoming key to SHA-256 digest to ensure constant length comparison and eliminate length leaks
	keyHash := sha256.Sum256([]byte(key))
	matchedKey := ""
	matched := false

	for _, validKey := range cfg.Keys {
		validHash := sha256.Sum256([]byte(validKey))
		if subtle.ConstantTimeCompare(keyHash[:], validHash[:]) == 1 {
			matchedKey = key
			matched = true
		}
	}
	return matchedKey, matched
}

func verifyBasicAuth(req *httpparser.Request, cfg BasicAuthConfig) (string, bool) {
	authHdr := req.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(authHdr), "basic ") {
		return "", false
	}

	payload := strings.TrimSpace(authHdr[6:])
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", false
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", false
	}
	user, pass := parts[0], parts[1]

	expectedPass, exists := cfg.Users[user]
	if !exists {
		// Perform constant-time dummy comparison to prevent username enumeration via timing side-channels
		dummyHash := sha256.Sum256([]byte(pass))
		_ = subtle.ConstantTimeCompare(dummyHash[:], dummyHash[:])
		return "", false
	}

	passHash := sha256.Sum256([]byte(pass))
	expectedHash := sha256.Sum256([]byte(expectedPass))

	if subtle.ConstantTimeCompare(passHash[:], expectedHash[:]) == 1 {
		return user, true
	}
	return "", false
}

// NewAuthMiddleware returns a middleware validating JWT tokens, API keys, or Basic Auth credentials.
func NewAuthMiddleware(cfg AuthConfig) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *httpparser.Request, res *httpparser.Response) {
			if cfg.Type == "" || cfg.Type == AuthTypeNone {
				next(req, res)
				return
			}

			// Check path exclusions
			for _, p := range cfg.Excluded {
				pTrimmed := strings.TrimSpace(p)
				if pTrimmed == "" {
					continue
				}
				if pTrimmed == "/" {
					if req.Path == "/" {
						next(req, res)
						return
					}
					continue
				}
				if req.Path == pTrimmed || strings.HasPrefix(req.Path, strings.TrimSuffix(pTrimmed, "/")+"/") {
					next(req, res)
					return
				}
			}

			var authenticatedUser string

			switch cfg.Type {
			case AuthTypeJWT:
				authHdr := req.Header.Get("Authorization")
				if !strings.HasPrefix(strings.ToLower(authHdr), "bearer ") {
					res.SetStatus(http.StatusUnauthorized)
					res.Header.Set("Content-Type", "application/json")
					_, _ = res.WriteString(`{"error":"401 Unauthorized","message":"missing or malformed Authorization header (Bearer token required)"}`)
					return
				}
				tokenStr := strings.TrimSpace(authHdr[7:])
				requireExp := true
				if cfg.JWT.AllowNoExpiration {
					requireExp = false
				}
				claims, err := VerifyJWT(tokenStr, []byte(cfg.JWT.Secret), cfg.JWT.Issuer, cfg.JWT.Audience, requireExp)
				if err != nil {
					res.SetStatus(http.StatusUnauthorized)
					res.Header.Set("Content-Type", "application/json")
					_, _ = res.WriteString(fmt.Sprintf(`{"error":"401 Unauthorized","message":%q}`, err.Error()))
					return
				}
				if sub, ok := claims["sub"].(string); ok {
					authenticatedUser = sub
				} else if user, ok := claims["user"].(string); ok {
					authenticatedUser = user
				} else {
					authenticatedUser = "jwt-user"
				}

			case AuthTypeAPIKey:
				key, valid := verifyAPIKey(req, cfg.APIKey)
				if !valid {
					res.SetStatus(http.StatusUnauthorized)
					res.Header.Set("Content-Type", "application/json")
					_, _ = res.WriteString(`{"error":"401 Unauthorized","message":"invalid or missing API key"}`)
					return
				}
				authenticatedUser = key

			case AuthTypeBasic:
				user, valid := verifyBasicAuth(req, cfg.Basic)
				if !valid {
					res.SetStatus(http.StatusUnauthorized)
					realm := cfg.Basic.Realm
					if realm == "" {
						realm = "Toron"
					}
					res.Header.Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", realm))
					res.Header.Set("Content-Type", "application/json")
					_, _ = res.WriteString(`{"error":"401 Unauthorized","message":"invalid or missing Basic authentication credentials"}`)
					return
				}
				authenticatedUser = user

			default:
				next(req, res)
				return
			}

			if authenticatedUser != "" {
				req.Header.Set("X-Authenticated-User", authenticatedUser)
			}

			next(req, res)
		}
	}
}
