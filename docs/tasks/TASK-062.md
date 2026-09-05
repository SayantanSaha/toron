---
id: TASK-062
type: task
title: Implement Authentication Middleware and Subnet Restrictions on Internal Management APIs
status: draft
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-062
implements:
  - REQ-062
verified_by: []
decided_by:
  - ADR-057
related_to:
  - TASK-022
---

# TASK-062 - Implement Authentication Middleware and Subnet Restrictions on Internal Management APIs

## Description

Harden the internal management API suite (`/internal/api/*`) by enforcing authentication middleware, optional administrative CIDR subnet whitelisting, and SSRF restrictions on `/internal/api/proxy-test`.

## Scope & Implementation Breakdown

1. **Configuration Extension (`pkg/config/config.go`)**:
   - Add `AdminAuth` configuration struct under `ServerConfig` supporting API key or Basic Auth credentials.
   - Add `AdminSubnets` list of allowed CIDRs for accessing `/internal/api/*`.

2. **Route Protection & Authentication (`pkg/server/internal_api.go`)**:
   - Update `RegisterInternalAPIRoutes` to apply authentication middleware to all `/internal/api/*` routes.
   - Add client IP evaluation rejecting requests outside configured `AdminSubnets` with `403 Forbidden`.

3. **Proxy-Test SSRF Hardening (`pkg/server/internal_api.go`)**:
   - Restrict `testReq.Path` to a safe whitelist of diagnostic endpoints (e.g. `/health`, `/api/status`).
   - Strip identity headers (`X-Authenticated-User`, `X-Forwarded-For`) from `testReq.Headers` to prevent spoofing against localhost.

4. **Testing Verification (`pkg/server/internal_api_test.go`)**:
   - Verify unauthenticated requests to `/internal/api/*` return `401 Unauthorized`.
   - Verify unauthorized source IPs receive `403 Forbidden`.
   - Verify `POST /internal/api/proxy-test` rejects attempts to inject `X-Authenticated-User` or access non-whitelisted paths.

## Acceptance Criteria

- Management endpoints cannot be accessed without valid credentials.
- Local SSRF via `/internal/api/proxy-test` cannot spoof identity headers or invoke arbitrary routes.
- Unit tests pass cleanly with `go test ./pkg/server/...`.

## Rationale

Internal APIs currently expose complete topology, routing, and audit logs to unauthenticated clients, and proxy-test allows arbitrary request forging to localhost.

## Constraints

- Zero external runtime dependencies.
- Retain backward compatibility with the Control Center UI when administrative credentials are supplied.

## Open Questions

- Should proxy-test be completely disabled when Toron is launched in production mode?
