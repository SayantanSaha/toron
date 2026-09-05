---
id: TASK-067
type: task
title: Implement Upstream Path Canonicalization and Route Traversal Guards
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-067
implements:
  - REQ-067
verified_by:
  - TC-067
decided_by:
  - ADR-062
related_to:
  - TASK-001
  - TASK-019
  - TASK-059
---

# TASK-067 - Implement Upstream Path Canonicalization and Route Traversal Guards

## Description

Implement strict path canonicalization and directory traversal guards in `pkg/router/router.go` and `pkg/proxy/proxy.go` (`JoinProxyPath`), ensuring incoming request paths are normalized before prefix route matching and upstream path synthesis, preventing route escape and unauthorized backend access.

## Scope & Implementation Breakdown

1. **Pre-Routing Canonicalization (`pkg/router/router.go`)**:
   - In `Router.ServeHTTP`, canonicalize `req.Path` using `path.Clean` prior to exact route lookup and prefix route evaluation.
   - Ensure leading slashes are preserved and redundant segments (`/api/../admin` -> `/admin`, `//admin` -> `/admin`) are resolved before matching.
   - If an incoming path attempts to traverse above root (e.g. `/..`), normalize safely to `/`.

2. **Upstream Path Synthesis Guard (`pkg/proxy/proxy.go`)**:
   - In `JoinProxyPath`, ensure that the joined destination path does not escape the configured `targetPath` root.
   - Apply `path.Clean` to relative trimmed segments before joining to avoid dot-segment leakage into upstream requests.
   - Preserve client trailing slashes when explicitly requested by the client on normalized paths.

3. **Testing Verification (`pkg/router/router_test.go`, `pkg/proxy/proxy_test.go`)**:
   - Add router tests verifying that requests with traversal segments like `/public/../internal` are evaluated against `/internal` rather than erroneously matching `/public`.
   - Add `JoinProxyPath` tests verifying traversal attempts such as `reqPath = "/api/../admin"` with prefix `/api` do not leak into parent directories on target URLs.

## Acceptance Criteria

- Incoming paths are canonicalized prior to route prefix matching.
- Dot-segment traversal sequences cannot escape route prefix boundaries.
- Trailing slashes are preserved on legitimate directory requests.
- All router and proxy unit tests pass with `go test ./pkg/router/... ./pkg/proxy/...`.

## Rationale

Matching uncanonicalized paths against prefix routes enables attackers to match authorized route prefixes (e.g. `/public`) while dispatching requests to unauthorized endpoints (e.g. `/admin`) on upstream services.

## Constraints

- RFC 3986 §5.2.4 dot segment normalization rules.
- Maintain minimal memory allocations on the hot routing path.

## Open Questions

- None.
