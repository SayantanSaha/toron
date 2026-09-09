---
id: TASK-101
type: task
title: Configurable Transcoder Request Body Limit Schema
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-089

owns:
  - pkg/config/config.go
  - pkg/config/loader.go
  - pkg/transcoder/transcoder.go

references:
  - REQ-089
  - SEC-28
  - SR-081
  - ADR-084
  - TC-089

derived_from:
  - REQ-089

implements:
  - REQ-089

verified_by:
  - TC-089
---

# TASK-101 - Configurable Transcoder Request Body Limit Schema

## Overview

Define and wire a dedicated configuration schema for REST-to-gRPC transcoding request body limits across [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go), [`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go), and [`pkg/transcoder/transcoder.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go). This parameter establishes the maximum allowable incoming HTTP body payload size for REST endpoints before rejection, eliminating unbounded memory allocations under [`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401) (CWE-400/CWE-770).

## Scope & Implementation Breakdown

1. **Extend [`TranscoderConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L130) Struct & Defaults ([`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go))**:
   - Add field `MaxBodyBytes int64` (`` `yaml:"max_body_bytes,omitempty" json:"max_body_bytes,omitempty"` ``).
   - Implement helper method `(c TranscoderConfig) GetMaxBodyBytes() int64`:
     - If `c.MaxBodyBytes > 0`, return `c.MaxBodyBytes`.
     - Otherwise, return default `4 * 1024 * 1024` (4 MB / 4,194,304 bytes), aligning with [`DefaultMaxGRPCFrameSize`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/framer.go#L11).
   - In `DefaultAppConfig()`:
     - Set `Transcoder.MaxBodyBytes: 4 * 1024 * 1024` (4 MB).

2. **Configuration Validation ([`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go))**:
   - In `ValidateConfig()` or loader routines:
     - Validate that `cfg.Transcoder.MaxBodyBytes >= 0`.
     - If negative, return descriptive error: `fmt.Errorf("transcoder.max_body_bytes cannot be negative: %d", cfg.Transcoder.MaxBodyBytes)`.

3. **Store and Enforce Limit in [`Engine`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L24) ([`pkg/transcoder/transcoder.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go))**:
   - Add `maxBodyBytes int64` field to [`Engine`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L24).
   - In [`NewEngine(cfg config.TranscoderConfig, r *router.Router)`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L33):
     - Initialize `maxBodyBytes := cfg.GetMaxBodyBytes()`.
     - Ensure if `maxBodyBytes <= 0`, default to `4 * 1024 * 1024`.

## Acceptance Criteria

- `TranscoderConfig` serializes and deserializes `max_body_bytes` in YAML and JSON.
- `GetMaxBodyBytes()` returns 4 MB when unset or $\le 0$.
- Negative `max_body_bytes` values are rejected during configuration loading.
- `Engine` instance correctly retains the configured byte limit.
