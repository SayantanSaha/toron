---
id: TASK-039
type: task
title: Implement Per-Host SNI Dynamic Certificate Dispatching and mTLS Client Auth
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-039

implements:
  - REQ-039

verified_by:
  - TC-039

decided_by:
  - ADR-034

related_to:
  - TASK-004
  - TASK-007
  - TASK-018
---

# TASK-039 - Implement Per-Host SNI Dynamic Certificate Dispatching and mTLS Client Auth

## Goal

Create `pkg/server/sni.go` implementing `SNIRegistry`, per-host TLS profile lookup, mTLS CA verification, and integrate with `pkg/config/config.go`, `pkg/server/server.go`, `cmd/toron/main.go`, and `routes.yaml`.

## Sub-tasks

1. Create `pkg/server/sni.go` implementing:
   - `RouteTLSConfig`, `HostTLSProfile`, and `SNIRegistry`.
   - `ParseClientAuthType(str string) tls.ClientAuthType`.
   - `ParseTLSVersion(str string) uint16`.
   - `RegisterHostProfile(host string, cfg RouteTLSConfig) error`.
   - `GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error)`.
2. Update `pkg/config/config.go`:
   - Add `RouteTLSConfig` struct and embed `TLS RouteTLSConfig` inside `ProxyRouteConfig`.
3. Update `pkg/server/tls.go` & `pkg/server/server.go`:
   - Attach `SNIRegistry` to `Server` and set `tlsConfig.GetConfigForClient = sniRegistry.GetConfigForClient`.
4. Update `cmd/toron/main.go` and `routes.yaml` with per-host TLS routing configurations.
5. Create comprehensive unit tests in `pkg/server/sni_test.go`.
6. Run full test suite `go test ./...` and configuration dry-run `.\toron.exe -t`.

## Acceptance Criteria

- Multi-domain SNI requests receive the exact certificate matching `hello.ServerName`.
- Routes configured with `client_auth: "require_and_verify"` strictly validate client certs against the configured CA file.
- Fallback domains without custom TLS settings continue serving the default or auto dev cert.
- Full Go test suite passes cleanly with 0 failures.
