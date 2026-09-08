---
id: TASK-090
type: task
title: Configurable Sidecar Body Size Limit in SidecarConfig
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-086

owns:
  - pkg/config/config.go
  - pkg/config/loader.go
  - pkg/sidecar/proxy.go

references:
  - REQ-086
  - SEC-25

derived_from:
  - REQ-086

implements:
  - REQ-086

verified_by:
  - TC-086
---

# TASK-090 - Configurable Sidecar Body Size Limit in SidecarConfig

## Overview

Introduce a configurable maximum request body size parameter (`MaxBodyBytes`) to [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103) in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go), enforce a 10 MB (`10 * 1024 * 1024` bytes) default fallback when unset or non-positive in [`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go), and initialize the fallback in [`NewProxyEngine`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L37) in [`pkg/sidecar/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go) to remediate vulnerability [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374).

## Scope & Implementation Breakdown

1. **Extend [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103) Struct ([`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go))**:
   - Add field `MaxBodyBytes int64` with struct tags `` `yaml:"max_body_bytes" json:"max_body_bytes"` ``.
   - Update [`DefaultAppConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L466) to initialize `Sidecar.MaxBodyBytes = 10 * 1024 * 1024` (10 MB).

2. **Default Normalization in Configuration Loader ([`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go))**:
   - In [`validateConfigDefaults`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L153), verify `cfg.Sidecar.MaxBodyBytes`:
     - If `cfg.Sidecar.MaxBodyBytes <= 0`, default to `10 * 1024 * 1024` (10 MB).

3. **Proxy Engine Initialization Fallback ([`pkg/sidecar/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go))**:
   - In [`NewProxyEngine`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L37), ensure standalone engine instantiation guarantees the default limit:
     - If `cfg.MaxBodyBytes <= 0`, assign `cfg.MaxBodyBytes = 10 * 1024 * 1024`.

## Acceptance Criteria

- [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103) exports `MaxBodyBytes int64` with proper YAML and JSON deserialization tags.
- Default configuration loaders and standalone proxy constructors default to 10 MB (`10,485,760` bytes) when `MaxBodyBytes` is 0 or negative.
- Explicit positive values configured via YAML, JSON, or programmatic structs are preserved intact.
- Zero external/third-party dependencies.

## Blockers & Risks

- None. Additive configuration change with backward-compatible defaults.
