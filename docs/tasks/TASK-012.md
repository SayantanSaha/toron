---
id: TASK-012
type: task
title: Add 10 Dummy Web Services for Upstream Testing
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-09-08

depends_on:
  - REQ-012

implements:
  - REQ-012

verified_by:
  - TC-012

decided_by:
  - ADR-007

related_to:
  - TASK-009
  - TASK-011
---

# TASK-012 - Add 10 Dummy Web Services for Upstream Testing

## Goal

Create 10 dummy HTTP web services running on ports 9001–9010 under `dummy-services/` with a multi-service launcher and unit tests.

## Sub-tasks

1. Implement `dummy-services/services.go` supporting spawning individual or all 10 services (ports 9001–9010).
2. Implement `dummy-services/main.go` CLI entry point to launch all 10 services concurrently.
3. Write `dummy-services/services_test.go` verifying service creation and HTTP responses across all 10 ports.

## Acceptance Criteria

- All 10 services start on ports 9001 through 9010.
- `go test ./...` passes cleanly.
