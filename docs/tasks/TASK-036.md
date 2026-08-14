---
id: TASK-036
type: task
title: Implement Multi-Scheme Authentication Middleware (JWT HS256, API Key, and HTTP Basic Auth)
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-036

implements:
  - REQ-036

verified_by:
  - TC-036

decided_by:
  - ADR-031

related_to:
  - TASK-003
  - TASK-009
  - TASK-029
---

# TASK-036 - Implement Multi-Scheme Authentication Middleware (JWT HS256, API Key, and HTTP Basic Auth)

## Goal

Create `pkg/router/auth.go` implementing `AuthMiddleware`, JWT verification (HS256/HS384/HS512), API Key verification, HTTP Basic Auth, route option extensions in `pkg/config`, and YAML configuration.

## Sub-tasks

1. Create `pkg/router/auth.go` implementing:
   - `AuthConfig`, `JWTConfig`, `BasicAuthConfig`, `APIKeyConfig`.
   - `VerifyJWT(tokenString string, secret []byte, expectedIssuer, expectedAudience string) (*JWTClaims, error)`.
   - `NewAuthMiddleware(cfg AuthConfig)` wrapping router requests.
   - `crypto/subtle.ConstantTimeCompare` for secret/password/key evaluations to prevent timing attacks.
2. Add `AuthConfig` in `pkg/config/config.go` under `ProxyRouteConfig.Auth` and `ServerConfig.Auth`.
3. Support route-level authentication in `pkg/router/router.go` and `cmd/toron/main.go`.
4. Update `routes.yaml` and `config.yaml` with commented authentication configuration patterns.
5. Create comprehensive unit tests in `pkg/router/auth_test.go`.
6. Run the complete test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- Valid JWT token passes authentication and injects `X-Authenticated-User: <sub/user>` into request headers.
- Expired or tampered JWT returns `401 Unauthorized` with descriptive JSON error.
- Valid API Key passes authentication; invalid API Key returns `401 Unauthorized`.
- Valid Basic Auth credentials pass authentication; invalid returns `401 Unauthorized` and `WWW-Authenticate: Basic realm="Toron"`.
- Full Go test suite passes cleanly.
