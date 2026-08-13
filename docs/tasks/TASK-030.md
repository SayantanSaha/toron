---
id: TASK-030
type: task
title: Implement Sticky Session Load Balancing (sticky_cookie and ip_hash)
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-13
updated: 2026-08-13

depends_on:
  - REQ-030

implements:
  - REQ-030

verified_by:
  - TC-030

decided_by:
  - ADR-025

related_to:
  - TASK-011
  - TASK-014
  - TASK-019
---

# TASK-030 - Implement Sticky Session Load Balancing (sticky_cookie and ip_hash)

## Goal

Extend `ProxyRouteConfig` with `sticky_cookie_name`, implement `StickyCookieBalancer` and `IPHashBalancer` in `pkg/proxy/proxy.go`, handle cookie injection/decoding and unhealthy target fallback, and verify unit tests.

## Sub-tasks

1. Add `StickyCookieName` field to `ProxyRouteConfig` in `pkg/config/config.go` and `ProxyOptions` in `pkg/proxy/proxy.go`.
2. Implement `IPHashBalancer` in `pkg/proxy/proxy.go` using FNV-1a IP hashing over healthy targets.
3. Implement `StickyCookieBalancer` in `pkg/proxy/proxy.go` for cookie lookup, target mapping, and `Set-Cookie` header injection.
4. Integrate `sticky_cookie` and `ip_hash` in `NewLoadBalancer()` factory method.
5. Write unit tests for `StickyCookieBalancer` and `IPHashBalancer` in `pkg/proxy/proxy_test.go`.
6. Run full test suite `go test ./...` to verify zero-regression execution.

## Acceptance Criteria

- `sticky_cookie` load balancer routes requests with matching cookie to pinned backend and injects `Set-Cookie` on new connections.
- `ip_hash` load balancer maps requests from same client IP to identical target.
- Circuit breaker / health check fallback works if target is unhealthy.
- All Go unit tests pass cleanly.
