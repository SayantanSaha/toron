---
id: TASK-014
type: task
title: Implement Upstream Health Check and Circuit Breaker
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-09-08

depends_on:
  - REQ-014

implements:
  - REQ-014

verified_by:
  - TC-014

decided_by:
  - ADR-009

related_to:
  - TASK-009
  - TASK-011
  - TASK-013
---

# TASK-014 - Implement Upstream Health Check and Circuit Breaker

## Goal

Add active health check probing and 3-state circuit breaker (`Closed`, `Open`, `HalfOpen`) to `pkg/proxy` and `pkg/config`.

## Sub-tasks

1. Implement `CircuitBreaker` and `UpstreamTarget` state machine in `pkg/proxy/circuit_breaker.go`.
2. Integrate health filtering into `RoundRobinBalancer.Next()` in `pkg/proxy/proxy.go`.
3. Add health check and circuit breaker config fields to `ProxyRouteConfig` in `pkg/config/config.go`.
4. Update `cmd/toron/main.go` and `pkg/router/router.go` to support health check options.
5. Write unit tests for circuit breaker state transitions, active probing, and target filtering in `pkg/proxy/proxy_test.go`.

## Acceptance Criteria

- All circuit breaker state transitions (`Closed` -> `Open` -> `HalfOpen` -> `Closed`) verified in tests.
- `go test ./...` passes 100%.
