---
id: TASK-019
type: task
title: Implement Dual-File YAML Config Loader (config.yaml & routes.yaml)
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-09-08

depends_on:
  - REQ-019

implements:
  - REQ-019

verified_by:
  - TC-019

decided_by:
  - ADR-014

related_to:
  - TASK-007
  - TASK-013
---

# TASK-019 - Implement Dual-File YAML Config Loader (config.yaml & routes.yaml)

## Goal

Split application configuration into `config.yaml` (server infrastructure) and `routes.yaml` (proxy routing rules), updating `pkg/config` and `cmd/toron/main.go`.

## Sub-tasks

1. Extract proxy routing configuration from `config.yaml` into repository root `routes.yaml`.
2. Update `config.yaml` to retain `server`, `static`, and `logging` parameters.
3. Enhance `pkg/config/loader.go` with `LoadFromFiles(configPath, routesPath string) (*AppConfig, error)` and auto-discovery of `routes.yaml`.
4. Update `cmd/toron/main.go` CLI flags (`-config`/`-c`, `-routes`/`-r`).
5. Update `pkg/config/config_test.go` and run `go test ./...`.

## Acceptance Criteria

- `config.yaml` and `routes.yaml` are clean and valid.
- Toron server successfully loads both files on startup.
- All Go unit tests pass cleanly.
