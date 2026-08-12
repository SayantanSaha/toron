---
id: TASK-011
type: task
title: Implement Reverse Proxy Upstream Load Balancing
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-011
  - ADR-006

implements:
  - REQ-011

verified_by:
  - TC-011

decided_by:
  - ADR-006

related_to:
  - TASK-007
  - TASK-009
  - TASK-010
---

# TASK-011 - Implement Reverse Proxy Upstream Load Balancing

## Goal

Add multi-target load balancing support to `pkg/proxy`, `pkg/config`, `pkg/router`, and `cmd/toron` using Round-Robin as the default selection algorithm.

## Sub-tasks

1. Define `LoadBalancer` interface and `RoundRobinBalancer` in `pkg/proxy/proxy.go`.
2. Add `NewLoadBalancerProxy` and update `ReverseProxy.ServeHTTPWithPrefix` to dispatch via `Balancer`.
3. Add `Targets []string` and `Algorithm string` to `ProxyRouteConfig` in `pkg/config/config.go`.
4. Add `ProxyBalancer` and `ProxyBalancerHeaders` methods to `pkg/router/router.go`.
5. Wire multi-target load proxying in `cmd/toron/main.go`.
6. Write unit tests for load balancing in `pkg/proxy/proxy_test.go`, `pkg/config/config_test.go`, and `pkg/router/router_test.go`.

## Acceptance Criteria

- All unit tests pass cleanly (`go test ./...`).
- Round-Robin selects targets sequentially.
- Backward compatibility maintained for single-target configurations.
