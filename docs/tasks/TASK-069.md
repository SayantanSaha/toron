---
id: TASK-069
type: task
title: Implement Strict CORS Origin Validation and Prohibit Credentialed Wildcards
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-069
implements:
  - REQ-069
verified_by:
  - TC-069
decided_by:
  - ADR-064
related_to:
  - TASK-023
---

# TASK-069 - Implement Strict CORS Origin Validation and Prohibit Credentialed Wildcards

## Description

Enforce strict CORS configuration validation and runtime origin matching in `pkg/router/cors.go` and `pkg/config/config.go`, disallowing configurations that combine `allow_credentials: true` with wildcard origins (`*`), and prohibiting dynamic origin reflection for credentialed endpoints.

## Scope & Implementation Breakdown

1. **Configuration Validation (`pkg/config/config.go`, `pkg/router/cors.go`)**:
   - In `ValidateCORSConfig` / `NewCORSMiddleware`, check if `AllowCredentials` is true and `AllowOrigins` contains `"*"`.
   - If detected, return an explicit configuration error (e.g. `ErrInsecureCORSCredentialsWildcard`) to prevent the gateway from starting with this dangerous configuration.

2. **Runtime Origin Matching Guard (`pkg/router/cors.go`)**:
   - In `isOriginAllowed`, if `allowCredentials` is active, skip wildcard matching rules and require exact or validated subdomain matches (`*.example.com`).
   - In `NewCORSMiddleware`, ensure that if `AllowCredentials` is true, the `Access-Control-Allow-Origin` header is only populated if the origin explicitly matched an allowed origin pattern.
   - Always append `Vary: Origin` when origin-specific CORS headers are emitted.

3. **Testing Verification (`pkg/router/cors_test.go`, `pkg/config/config_test.go`)**:
   - Add unit test verifying that combining `AllowOrigins: []string{"*"}` with `AllowCredentials: true` is rejected during configuration validation.
   - Add middleware test verifying that an unauthorized origin receives `403 Forbidden` on preflight and no `Access-Control-Allow-Origin` on simple requests.

## Acceptance Criteria

- Configuration parser rejects `allow_credentials: true` combined with `allow_origins: ["*"]`.
- Wildcard origin reflection with credentials enabled is impossible at runtime.
- Responses containing dynamic CORS headers include `Vary: Origin`.
- All CORS tests pass with `go test ./pkg/router/... ./pkg/config/...`.

## Rationale

Bypassing browser CORS protections by reflecting arbitrary origins with credentials enabled allows attacker websites to read sensitive API responses and steal user data (CWE-942).

## Constraints

- WHATWG / W3C CORS specification compliance.
- Support safe subdomain wildcard patterns (e.g. `https://*.domain.com`).

## Open Questions

- None.
