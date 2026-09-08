---
id: TASK-079
type: task
title: Implement Prefix and Protected Path Validation in Container Auto-Discovery
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-079
implements:
  - REQ-079
verified_by:
  - TC-079
decided_by:
  - ADR-074
related_to: []
---

# TASK-079 - Implement Prefix and Protected Path Validation in Container Auto-Discovery

## Description

Enforce prefix scoping and protected path restrictions in `pkg/discovery/parser.go` and `pkg/discovery/manager.go`, preventing containers from registering root (`"/"`) or administrative (`"/internal"`) prefixes.

## Scope & Implementation Breakdown

1. **Prefix Validation (`pkg/discovery/parser.go`)**:
   - In `ParseContainerLabels`, reject container registration if `prefix == ""` or `prefix == "/"` unless an explicit non-empty `host` is specified.
   - Reject any container attempting to bind to prefixes starting with `/internal` or `/api/status`.
2. **Warning Logging (`pkg/discovery/manager.go`)**:
   - Log warning when a container label is rejected for attempting to shadow protected paths.
3. **Unit Testing (`pkg/discovery/discovery_test.go`)**:
   - Add test verifying that a container with `toron.prefix: "/"` and no host is rejected.
   - Add test verifying that a container with `toron.prefix: "/internal/api"` is rejected.

## Acceptance Criteria

- Rogue containers cannot hijack root `/` ingress or shadow `/internal` routes.
- Discovery manager gracefully skips unauthorized container route labels.
- Tests pass with `go test ./pkg/discovery/...`.

## Rationale

Protects edge gateway routing from untrusted container tenants sharing the Docker engine.

## Constraints

- Pure Go standard library.
