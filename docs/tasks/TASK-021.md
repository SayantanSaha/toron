---
id: TASK-021
type: task
title: Implement Configuration Test Option (-test-config / -t) and Strict Validator
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-021

implements:
  - REQ-021

verified_by:
  - TC-021

decided_by:
  - ADR-016

related_to:
  - TASK-007
  - TASK-019
---

# TASK-021 - Implement Configuration Test Option (-test-config / -t) and Strict Validator

## Goal

Add configuration syntax validation logic in `pkg/config` and wire up the `-test-config` (`-t`) CLI dry-run flag in `cmd/toron/main.go`.

## Sub-tasks

1. Implement `ValidateConfig(cfg *AppConfig) error` in `pkg/config/loader.go`.
2. Add `-test-config` and `-t` flags to `cmd/toron/main.go`.
3. Implement exit code behavior (0 on valid syntax, 1 on error).
4. Add unit test suite in `pkg/config/config_test.go`.
5. Run `go test ./...`.

## Acceptance Criteria

- `go run ./cmd/toron -t` prints syntax check results and exits safely without running server socket listener.
- All Go unit tests pass.
