---
id: TASK-097
type: task
title: Configurable Upgraded Inactivity Deadline Schema
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-088

owns:
  - pkg/server/config.go
  - pkg/config/config.go
  - pkg/config/loader.go

references:
  - REQ-088
  - SEC-27
  - SR-081
  - ADR-083
  - TC-088

derived_from:
  - REQ-088

implements:
  - REQ-088

verified_by:
  - TC-088
---

# TASK-097 - Configurable Upgraded Inactivity Deadline Schema

## Overview

Define and wire a dedicated configuration schema for upgraded connection inactivity deadlines ([`UpgradeIdleTimeout`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L6)) across [`pkg/server/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go), [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go), and [`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go). This configuration parameter controls the maximum permissible duration of bidirectional inactivity on upgraded protocol connections (such as RFC 6455 WebSockets and RFC 8441 HTTP/2 Extended CONNECT streams) before the server terminates sockets and unblocks goroutines, mitigating [`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392) (CWE-400 Slowloris resource exhaustion).

## Scope & Implementation Breakdown

1. **Extend [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L6) Struct & Production Defaults ([`pkg/server/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go))**:
   - Add `UpgradeIdleTimeout time.Duration` to [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L6):
     - Maximum allowable period of zero byte transfer in both directions across an established upgraded connection before closing both ends.
   - Update [`DefaultConfig()`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L33):
     - Set `UpgradeIdleTimeout: 60 * time.Second`.

2. **Extend [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L214) Struct & Application Defaults ([`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go))**:
   - Add field to [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L214):
     - `UpgradeIdleTimeout time.Duration` (`` `yaml:"upgrade_idle_timeout,omitempty" json:"upgrade_idle_timeout,omitempty"` ``).
   - Update [`DefaultAppConfig()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L494):
     - Set `Server.UpgradeIdleTimeout: 60 * time.Second`.

3. **Implement Fallback Logic in [`ToServerConfig()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L580) ([`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go))**:
   - Map `UpgradeIdleTimeout` from [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L214) into [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L6).
   - Implement hierarchical fallback:
     - If `c.Server.UpgradeIdleTimeout > 0`, use `c.Server.UpgradeIdleTimeout`.
     - Else if `c.Server.IdleTimeout > 0`, fall back to `c.Server.IdleTimeout`.
     - Else, fall back to `60 * time.Second`.

4. **Add Schema Validation in [`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go)**:
   - In [`ValidateConfig(cfg *AppConfig)`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L172):
     - Validate that `cfg.Server.UpgradeIdleTimeout >= 0`. If negative, return an error:
       `fmt.Errorf("server.upgrade_idle_timeout must be non-negative, got %v", cfg.Server.UpgradeIdleTimeout)`
   - In [`validateConfigDefaults(cfg *AppConfig)`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L153):
     - If `cfg.Server.UpgradeIdleTimeout <= 0`: populate with `60 * time.Second` or propagate fallback.

## Acceptance Criteria

- [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L6) contains `UpgradeIdleTimeout time.Duration`, and [`DefaultConfig()`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L33) initializes it to `60 * time.Second`.
- [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L214) contains `UpgradeIdleTimeout time.Duration` with `upgrade_idle_timeout` YAML and JSON struct tags.
- [`DefaultAppConfig()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L494) initializes `Server.UpgradeIdleTimeout` to `60 * time.Second`.
- [`ToServerConfig()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L580) correctly propagates `UpgradeIdleTimeout`, falling back to `ServerConfig.IdleTimeout` if `<= 0`, and ultimately `60 * time.Second`.
- [`ValidateConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L172) rejects negative `upgrade_idle_timeout` durations with descriptive error messages.
- Standard Go library only (zero external dependencies). Clean compilation and test pass in `pkg/config/...` and `pkg/server/...`.

## Blockers & Risks

- None identified; schema extension is strictly additive and preserves backward compatibility for existing configurations where `upgrade_idle_timeout` is omitted.
