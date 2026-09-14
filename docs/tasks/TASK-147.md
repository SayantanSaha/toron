---
id: TASK-147
type: task
title: Implement Configurable Distributed Tracing in Reverse Proxy Transport Engine
status: in_progress
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-14
updated: 2026-09-14

depends_on:
  - REQ-124

derived_from:
  - REQ-124

implements:
  - REQ-124

verified_by:
  - TC-124

decided_by:
  - ADR-124

related_to:
  - TASK-146
  - ADR-124
  - TC-124
  - CR-120
  - SR-124
---

# TASK-147 - Implement Configurable Distributed Tracing in Reverse Proxy Transport Engine

## 1. Overview & Objective

Under [`REQ-124`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-124.md), Toron must eliminate the unconditional invocation of `crypto/rand.Read` in `EnsureW3CTraceparent` during reverse proxy forwarding, which incurs ~49,000 cryptographic system calls per second at 24.5k RPS. This task implements a configurable `tracing` boolean across the global and route-level transport configurations.

---

## 2. Work Breakdown Structure (WBS)

### Work Package 1 (WP-1): Configuration Model Extension (`pkg/config`)
- Add `Tracing *bool `yaml:"tracing,omitempty" json:"tracing,omitempty"`` to `ProxyTransportConfig` in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go).
- Update `DefaultProxyTransportConfig`:
  - `profile: "raw_speed"`: `Tracing = &false`.
  - `profile: "balanced"`: `Tracing = &true`.
- Update `MergeProxyTransportConfig` to override `Tracing` when `override.Tracing != nil`.
- Ensure `ResolveTransport` seamlessly cascades route-level `tracing` overrides over global transport configurations.

### Work Package 2 (WP-2): Reverse Proxy Engine Integration (`pkg/proxy`)
- Mirror `Tracing *bool` in `pkg/proxy/proxy.go:ProxyTransportConfig`.
- In `pkg/proxy/proxy.go:DefaultProxyTransportConfig`:
  - Default `Tracing: &false` for `raw_speed`.
  - Default `Tracing: &true` for `balanced`.
- Update `ReverseProxy.ServeHTTPWithPrefix`:
  - Inspect `p.TransportConfig.Tracing`.
  - If `tracing` is `true` (or unset in balanced mode), invoke `EnsureW3CTraceparent(incomingTrace)` and inject the resulting header.
  - If `tracing` is `false`, propagate any existing `incomingTrace` verbatim without re-generating IDs, and do NOT invoke `crypto/rand` if `incomingTrace` is empty.

### Work Package 3 (WP-3): Server Wiring (`cmd/toron/main.go`)
- Ensure `cmd/toron/main.go` maps `tc.Tracing` from `pr.ResolveTransport(appCfg.Proxy.Transport)` into `proxy.ProxyOptions.Transport.Tracing`.

### Work Package 4 (WP-4): Automated Unit Testing & Benchmarks
- Add unit tests in `pkg/config/config_test.go` verifying:
  - Default `tracing: false` in `raw_speed` profile.
  - Default `tracing: true` in `balanced` profile.
  - Route-level `tracing` override over global default.
- Add unit tests in `pkg/proxy/proxy_test.go` verifying:
  - When `tracing: false` and client sends no `traceparent`: upstream receives NO `traceparent` header (0 CSPRNG calls).
  - When `tracing: false` and client sends valid `traceparent`: upstream receives the client's `traceparent` unchanged.
  - When `tracing: true` and client sends no `traceparent`: upstream receives a newly generated W3C `traceparent` header.

---

## 3. Acceptance Criteria & Verification
- All tests in `pkg/config/...` and `pkg/proxy/...` pass cleanly under `go test -v` and `go test -race`.
- Binary compiles with zero errors via `go build ./cmd/toron`.
