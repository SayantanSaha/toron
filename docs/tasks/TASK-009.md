---
id: TASK-009
type: task
title: Reverse Proxy Handler & Upstream Transport Implementation
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-08-11

depends_on:
  - TASK-003
  - TASK-005
  - TASK-007

derived_from:
  - REQ-009

implements:
  - REQ-009

verified_by: []

decided_by: []

related_to: []
---

# TASK-009 - Reverse Proxy Handler & Upstream Transport Implementation

## Description

Create `pkg/proxy` package implementing `ReverseProxy` and HTTP upstream forwarding, integrate `router.Proxy(prefix, targetURL)` into `pkg/router`, add YAML configuration support (`proxy.enabled`, `proxy.routes`), and add unit/integration tests against mock upstream HTTP servers.

## Acceptance Criteria

- `pkg/proxy` package created with `NewReverseProxy(targetURL)` function.
- Injects `X-Forwarded-*` headers and forwards request/response streams.
- Handles upstream connection errors returning `502 Bad Gateway`.
- `router.Proxy(prefix, targetURL)` helper method added to `Router`.
- `AppConfig` updated with `ProxyConfig` and `ProxyRouteConfig` fields.
- `cmd/toron/main.go` parses YAML proxy routes and attaches them to `Router`.

## Rationale

Provides built-in reverse proxy gateway capabilities.

## Constraints

- Robust error handling for offline upstream targets (`502 Bad Gateway`).

## Open Questions

- None.
