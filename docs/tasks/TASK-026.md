---
id: TASK-026
type: task
title: Implement TCP and UDP Layer 4 Transport Layer Proxying and Listener Support
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-13
updated: 2026-09-08

depends_on:
  - REQ-026

implements:
  - REQ-026

verified_by:
  - TC-026

decided_by:
  - ADR-021

related_to:
  - TASK-001
  - TASK-011
  - TASK-019
---

# TASK-026 - Implement TCP and UDP Layer 4 Transport Layer Proxying and Listener Support

## Goal

Add configuration, proxy forwarding logic, and listener lifecycle management for raw TCP stream and UDP datagram proxying to support non-HTTP Layer 4 transport protocol routing.

## Sub-tasks

1. Extend `pkg/config/config.go` `RouteConfig` to parse `type: "tcp"` and `type: "udp"` route types and optional listener ports (`listen_port` / `port`).
2. Add `TCPProxy` implementation in `pkg/proxy/tcp.go` for establishing upstream TCP socket connections and performing bi-directional byte copying (`io.Copy`).
3. Add `UDPProxy` implementation in `pkg/proxy/udp.go` for datagram socket forwarding (`net.UDPConn`) between client endpoints and upstream UDP targets.
4. Update `pkg/server/server.go` to initialize, manage, and gracefully shut down dedicated TCP and UDP listeners defined in `routes.yaml`.
5. Add unit and integration tests in `pkg/proxy/tcp_test.go`, `pkg/proxy/udp_test.go`, and `pkg/server/server_test.go`.
6. Run full test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- `routes.yaml` supports `type: "tcp"` and `type: "udp"` routes with designated ports and target backends.
- TCP stream connections are accepted and proxied bi-directionally to backend servers.
- UDP datagram packets are received and proxied to upstream target datagram listeners.
- Server manages TCP and UDP listener lifecycles cleanly without leaks or blocking shutdown.
- All Go unit tests pass cleanly.
