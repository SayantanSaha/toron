---
id: TASK-005
type: task
title: Toron Server Orchestration & Main Application Entry
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-09-08

depends_on:
  - TASK-001
  - TASK-002
  - TASK-003
  - TASK-004

derived_from:
  - REQ-001
  - REQ-003
  - REQ-004

implements:
  - REQ-001
  - REQ-003

verified_by: []

decided_by: []

related_to: []
---

# TASK-005 - Toron Server Orchestration & Main Application Entry

## Description

Assemble the top-level `Server` interface in `pkg/server` and create example executable entry point `cmd/toron/main.go` that initializes configuration, binds reactor listener, registers routes, handles SIGINT/SIGTERM OS signals, and manages graceful server shutdown.

## Acceptance Criteria

- `Server` struct cleanly orchestrates `Reactor`, `Router`, and `SecurityConfig`.
- `cmd/toron/main.go` compiles into a standalone binary.
- Clean shutdown on `SIGINT` / `SIGTERM` with pending connection drain.
- Includes `/health` endpoint returning `200 OK`.

## Rationale

Provides the runnable server binary and top-level public API for consumers.

## Constraints

- Standard Go binary layout (`cmd/toron/main.go`).

## Open Questions

- None.
