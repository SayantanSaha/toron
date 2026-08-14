---
id: TASK-040
type: task
title: Implement Configurable CORS Policies and Enterprise Security Headers Middleware
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-040

implements:
  - REQ-040

verified_by:
  - TC-040

decided_by:
  - ADR-035

related_to:
  - TASK-003
  - TASK-007
  - TASK-018
---

# TASK-040 - Implement Configurable CORS Policies and Enterprise Security Headers Middleware

## Goal

Create `pkg/router/cors.go` and `pkg/router/security_headers.go` implementing CORS preflight/request handling and browser security header injection, and integrate into `pkg/config/config.go`, `cmd/toron/main.go`, `config.yaml`, and `routes.yaml`.

## Sub-tasks

1. Create `pkg/router/cors.go` implementing:
   - `CORSConfig` struct.
   - `NewCORSMiddleware(cfg CORSConfig) MiddlewareFunc`.
   - Origin pattern matching (exact, wildcard `*`, and subdomain `https://*.example.com`).
   - Preflight `OPTIONS` short-circuiting with `204 No Content`.
2. Create `pkg/router/security_headers.go` implementing:
   - `SecurityHeadersConfig` struct.
   - `NewSecurityHeadersMiddleware(cfg SecurityHeadersConfig) MiddlewareFunc`.
   - Injection of `Strict-Transport-Security`, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Content-Security-Policy`, `Permissions-Policy`.
3. Update `pkg/config/config.go` with `CORSConfig` and `SecurityHeadersConfig` structures, embedding in `ServerConfig` and `ProxyRouteConfig`.
4. Update `cmd/toron/main.go`, `config.yaml`, and `routes.yaml`.
5. Create comprehensive unit tests in `pkg/router/cors_test.go` and `pkg/router/security_headers_test.go`.
6. Run full test suite `go test ./...` and configuration dry-run `.\toron.exe -t`.

## Acceptance Criteria

- CORS middleware accurately responds to preflight `OPTIONS` and cross-origin requests.
- Security headers middleware attaches all configured security headers to outgoing responses.
- Full Go test suite passes cleanly with 0 failures.
