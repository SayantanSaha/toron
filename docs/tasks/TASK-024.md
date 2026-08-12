---
id: TASK-024
type: task
title: Implement WebSocket Protocol Upgrade and Reverse Proxy Tunneling
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-024

implements:
  - REQ-024

verified_by:
  - TC-024

decided_by:
  - ADR-019

related_to:
  - TASK-001
  - TASK-014
---

# TASK-024 - Implement WebSocket Protocol Upgrade and Reverse Proxy Tunneling

## Goal

Add support for detecting WebSocket upgrade requests, executing 101 Switching Protocols handshakes, socket hijacking in the server engine, and bi-directional TCP stream tunneling in the reverse proxy gateway.

## Sub-tasks

1. Add `IsWebSocketUpgrade()` helper method to `pkg/httpparser/request.go`.
2. Add connection hijacking/upgraded connection handling support to `pkg/httpparser/response.go` and `pkg/server/server.go`.
3. Implement `ServeWebSocketProxy` / upgraded stream tunneling in `pkg/proxy/proxy.go`.
4. Update `pkg/server/server.go` connection loop to disable deadlines during WebSocket raw socket streaming.
5. Write unit tests for WebSocket upgrade and proxy tunneling in `pkg/proxy/proxy_test.go` and `pkg/server/server_test.go`.
6. Run full test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- HTTP/1.1 `Upgrade: websocket` requests are correctly detected.
- Reverse proxy establishes raw TCP connection to upstream target, forwards handshake, and tunnels frames bi-directionally.
- Socket deadlines do not terminate active WebSocket streams.
- All Go unit tests pass cleanly.
