---
id: TASK-027
type: task
title: Implement HTTP/3 Protocol Engine & QUIC Listener Support
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-13
updated: 2026-09-04

depends_on:
  - REQ-027

implements:
  - REQ-027

verified_by:
  - TC-027

decided_by:
  - ADR-022

related_to:
  - TASK-022
  - TASK-023
---

# TASK-027 - Implement HTTP/3 Protocol Engine & QUIC Listener Support

## Goal

Add HTTP/3 configuration fields to `config.yaml`, integrate `quic-go/http3` server engine in `pkg/server`, append `Alt-Svc` headers for protocol upgrade advertising, and verify HTTP/3 request routing and unit tests.

## Sub-tasks

1. Extend `HTTP3Config` and `ServerConfig` in `pkg/config/config.go` to capture `server.http3` settings (`enabled`, `port`).
2. Add `HTTP3` configuration fields in `pkg/server/server.go` `Config` struct.
3. Add `ListenAndServeQUIC()` and `ListenAndServeH3()` methods in `pkg/server/server.go` using `http3.Server`.
4. Inject `Alt-Svc: h3=":port"` response headers into HTTP/HTTPS response middleware to advertise HTTP/3 support to clients.
5. Add unit and integration tests for HTTP/3 configuration, `Alt-Svc` headers, and server lifecycle in `pkg/config/config_test.go` and `pkg/server/server_test.go`.
6. Run full test suite `go test ./...` to confirm all tests pass cleanly.

## Acceptance Criteria

- `config.yaml` parses `server.http3.enabled` and `server.http3.port`.
- `Server` initializes `http3.Server` with TLS certificate configuration and serves HTTP/3 over QUIC UDP sockets.
- HTTPS responses automatically append `Alt-Svc: h3=":port"` headers.
- All Go unit tests pass cleanly.
