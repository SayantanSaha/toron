---
id: TASK-013
type: task
title: Update config.yaml to Map Proxy Routes to Dummy Web Services
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-013

implements:
  - REQ-013

verified_by:
  - TC-013

decided_by:
  - ADR-008

related_to:
  - TASK-007
  - TASK-009
  - TASK-010
  - TASK-011
  - TASK-012
---

# TASK-013 - Update config.yaml to Map Proxy Routes to Dummy Web Services

## Goal

Update `config.yaml` to enable the reverse proxy and map all routing mechanisms to ports 9001–9010 of the dummy services cluster.

## Sub-tasks

1. Edit `config.yaml` to enable `proxy.enabled: true`.
2. Configure header-based, path-based, single-target, and multi-target proxy routes.
3. Validate config loading with `config.LoadFromFile("config.yaml")` in unit test `pkg/config/config_test.go`.

## Acceptance Criteria

- All routing methods map to ports 9001–9010.
- `go test ./...` passes cleanly.
