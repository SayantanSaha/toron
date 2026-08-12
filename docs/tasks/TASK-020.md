---
id: TASK-020
type: task
title: Implement Domain-Based HTTP Routing and Proxy Forwarding
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-020

implements:
  - REQ-020

verified_by:
  - TC-020

decided_by:
  - ADR-015

related_to:
  - TASK-010
  - TASK-011
---

# TASK-020 - Implement Domain-Based HTTP Routing and Proxy Forwarding

## Goal

Add Host header domain matching to `pkg/router`, `pkg/config`, `cmd/toron/main.go`, and `routes.yaml`.

## Sub-tasks

1. Add `Host string` field to `ProxyRouteConfig` in `pkg/config/config.go`.
2. Add `host` matching logic and `r.HandleHost`, `r.GETHost`, `r.ProxyHostWithOptions` to `pkg/router/router.go`.
3. Update `cmd/toron/main.go` to pass `pr.Host` into `r.ProxyWithOptions`.
4. Update `routes.yaml` with sample domain-based proxy routes.
5. Update `test_endpoint.http` with domain-based HTTP test cases.
6. Add unit tests in `pkg/router/router_test.go` and run `go test ./...`.

## Acceptance Criteria

- Host header matching works for exact domain names ignoring optional port numbers.
- Domain-based proxy routes forward requests to correct upstreams.
- All Go unit tests pass cleanly.
