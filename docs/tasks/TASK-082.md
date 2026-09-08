---
id: TASK-082
type: task
title: Implement Mandatory exp Claim Enforcement in JWT Verification
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-082
implements:
  - REQ-082
verified_by:
  - TC-082
decided_by:
  - ADR-077
related_to: []
---

# TASK-082 - Implement Mandatory exp Claim Enforcement in JWT Verification

## Description

Enhance `VerifyJWT` in `pkg/router/auth.go` and `JWTConfig` to require the presence of a valid `exp` claim, preventing non-expiring tokens from posing permanent replay risks.

## Scope & Implementation Breakdown

1. **Config & Verification Update (`pkg/router/auth.go`)**:
   - Add `RequireExpiration bool` to `JWTConfig` (default true).
   - In `VerifyJWT`, if `requireExp` is true, check `expVal, ok := claims["exp"]`. If `!ok`, return `fmt.Errorf("token missing required exp claim")`.
2. **Unit Testing (`pkg/router/auth_test.go`)**:
   - Add test verifying that a token without `exp` claim is rejected when expiration is required.
   - Add test verifying that tokens with valid future `exp` are accepted.

## Acceptance Criteria

- Tokens without `exp` claims are rejected when expiration requirement is active.
- Non-expiring token replay vulnerabilities are eliminated.
- Tests pass with `go test ./pkg/router/...`.

## Rationale

Limits window of exposure for leaked credentials.

## Constraints

- Pure Go standard library.
