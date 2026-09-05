---
id: TASK-063
type: task
title: Enforce Shared Cache Isolation by Stripping Set-Cookie and Enforcing Authorization Boundaries
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-063
implements:
  - REQ-063
verified_by:
  - TC-063
decided_by:
  - ADR-058
related_to:
  - TASK-021
---

# TASK-063 - Enforce Shared Cache Isolation by Stripping Set-Cookie and Enforcing Authorization Boundaries

## Description

Refactor `pkg/router/cache.go` to comply with RFC 7234 shared caching standards, ensuring `Set-Cookie` headers are stripped before storing response snapshots and requests bearing `Authorization` headers are excluded from shared caching unless explicitly marked public.

## Scope & Implementation Breakdown

1. **Set-Cookie Stripping (`pkg/router/cache.go`)**:
   - In `NewCacheMiddleware`, filter out `Set-Cookie` response headers when cloning headers into `clonedHeader` before calling `cache.Set`.
   - Ensure cached responses served from memory never include a `Set-Cookie` header.

2. **Authorization Boundary Checks (`pkg/router/cache.go`)**:
   - In `NewCacheMiddleware`, check if `req.Header.Get("Authorization") != ""`.
   - If present, do not serve from cache and do not store response in cache unless `resCC.Public` is explicitly true (RFC 7234 §3.2).

3. **Vary Header Evaluation (`pkg/router/cache.go`)**:
   - If upstream response declares a `Vary` header other than `Accept-Encoding`, bypass caching or partition the cache key appropriately.

4. **Testing Verification (`pkg/router/cache_test.go`)**:
   - Add test verifying that an upstream response containing `Set-Cookie: session=xyz` does not store the cookie in cache.
   - Add test verifying that requests with `Authorization: Bearer ...` bypass shared caching unless `Cache-Control: public` is returned.

## Acceptance Criteria

- `Set-Cookie` is never saved in or emitted from the shared response cache.
- Authenticated requests do not leak private data into the shared cache.
- Test suite passes with `go test ./pkg/router/...`.

## Rationale

Storing `Set-Cookie` in a shared cache causes session fixation and credential leakage, where subsequent users receive another user's session identifier.

## Constraints

- Compliance with RFC 7234 §3.2 and §8.
- Zero-allocation header filtering where feasible.

## Open Questions

- Should a route-level override allow caching private responses per-user in local session storage?
