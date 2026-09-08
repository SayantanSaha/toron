---
id: TASK-078
type: task
title: Harden Path Exclusion Matching in Authentication and WAF Middlewares
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-078
implements:
  - REQ-078
verified_by:
  - TC-078
decided_by:
  - ADR-073
related_to: []
---

# TASK-078 - Harden Path Exclusion Matching in Authentication and WAF Middlewares

## Description

Harden path exclusion evaluation in `pkg/router/auth.go` and `pkg/waf/middleware.go` to ignore empty strings and require exact matching for root (`"/"`) exclusions, preventing unintentional global bypasses.

## Scope & Implementation Breakdown

1. **Exclusion Validation (`pkg/router/auth.go`)**:
   - In `NewAuthMiddleware`, filter out empty strings (`p == ""`).
   - For `p == "/"`, check only `if req.Path == "/"`. Do not evaluate `strings.HasPrefix(req.Path, "/")`.
   - For subpaths `p != "/"`, check `req.Path == p || strings.HasPrefix(req.Path, strings.TrimSuffix(p, "/")+"/")`.
2. **WAF Exclusion Validation (`pkg/waf/middleware.go`)**:
   - Apply identical hardened matching logic in `NewWAFMiddleware`.
3. **Unit Testing (`pkg/router/auth_test.go`, `pkg/waf/waf_test.go`)**:
   - Add test with `excluded: ["/"]` confirming `/` is excluded while `/api/secure` and `/admin` still require authentication.
   - Add test with `excluded: [""]` confirming no path is bypassed.

## Acceptance Criteria

- `excluded: ["/"]` only excludes exact root `/`.
- `excluded: [""]` is ignored.
- All auth and WAF tests pass with `go test ./pkg/router/... ./pkg/waf/...`.

## Rationale

Prevents accidental total security deactivation due to common configuration mistakes.

## Constraints

- Pure Go standard library.
