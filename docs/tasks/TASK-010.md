---
id: TASK-010
type: task
title: Header-Based Route Matching & Config Integration
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-08-11

depends_on:
  - TASK-003
  - TASK-007
  - TASK-009

derived_from:
  - REQ-010

implements:
  - REQ-010

verified_by: []

decided_by: []

related_to: []
---

# TASK-010 - Header-Based Route Matching & Config Integration

## Description

Enhance `pkg/router` to evaluate HTTP headers during route matching, add header routing helpers (`r.GETHeader`, `r.ProxyHeader`), update `pkg/config` with `Headers map[string]string` in YAML proxy routes, and add unit tests.

## Acceptance Criteria

- `HeaderRule` struct created in `pkg/router` representing header key/value match conditions.
- `ServeHTTP` checks header rules prior to falling back to path-only routes.
- `r.ProxyHeader(prefix, headerKey, headerVal, targetURL)` registers a header-conditional proxy route.
- `AppConfig` updated to parse `headers` map in `config.yaml`.
- Table-driven unit tests pass for header routing.

## Rationale

Provides fine-grained traffic control and API versioning.

## Constraints

- Case-insensitive header name matching.

## Open Questions

- None.
