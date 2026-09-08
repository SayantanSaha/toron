---
id: TASK-029
type: task
title: Implement Token Bucket Rate Limiting Middleware
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-13
updated: 2026-09-08

depends_on:
  - REQ-029

implements:
  - REQ-029

verified_by:
  - TC-029

decided_by:
  - ADR-024

related_to:
  - TASK-003
  - TASK-011
  - TASK-019
---

# TASK-029 - Implement Token Bucket Rate Limiting Middleware

## Goal

Add `rate_limit` parameter parsing to `pkg/config/config.go`, implement Token Bucket rate limiter in `pkg/router/rate_limiter.go`, integrate rate limiting middleware into route matching, and verify HTTP status 429 response handling.

## Sub-tasks

1. Extend `ProxyRouteConfig` in `pkg/config/config.go` with `RateLimit` string field and parser helper (`ParseRateLimit()`).
2. Create `TokenBucket` struct in `pkg/router/rate_limiter.go` supporting token replenishment and capacity tracking per key.
3. Implement `RateLimiterMiddleware` in `pkg/router/rate_limiter.go` to extract client IP or `X-API-Key` header and enforce route limits.
4. Support HTTP `429 Too Many Requests` responses with `Retry-After` header.
5. Write unit tests in `pkg/router/rate_limiter_test.go` and `pkg/config/config_test.go`.
6. Run full test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- `routes.yaml` supports `rate_limit: "10/sec"` and `rate_limit: "100/min"`.
- Requests within rate limit proceed to handler/upstream.
- Excess requests return `429 Too Many Requests` with `Retry-After` header.
- All Go unit tests pass cleanly.
