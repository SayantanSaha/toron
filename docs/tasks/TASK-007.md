---
id: TASK-007
type: task
title: Extensible Config Loader Package & CLI Flag Integration
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-08-11

depends_on:
  - TASK-005
  - TASK-006

derived_from:
  - REQ-007

implements:
  - REQ-007

verified_by: []

decided_by: []

related_to: []
---

# TASK-007 - Extensible Config Loader Package & CLI Flag Integration

## Description

Create `pkg/config` with `Loader` interface, `YAMLLoader` implementation, schema validation, default fallbacks, and integrate `-config` CLI flag parsing into `cmd/toron/main.go`. Create `config.yaml` as reference config file.

## Acceptance Criteria

- `pkg/config` package defines `Config` struct and `Decoder` / `Loader` interface.
- `YAMLLoader` decodes YAML files using `gopkg.in/yaml.v3`.
- `cmd/toron/main.go` parses `-config` flag, loads config file if provided, and initializes `server.Config` and `router.Static`.
- Sample `config.yaml` provided in repository root.

## Rationale

Provides extensible, file-driven server configuration.

## Constraints

- Backward-compatible default fallbacks.

## Open Questions

- None.
