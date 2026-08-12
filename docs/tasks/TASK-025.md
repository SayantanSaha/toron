---
id: TASK-025
type: task
title: Implement HTTP/2 WebSocket Support via Extended CONNECT (RFC 8441)
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-025

implements:
  - REQ-025

verified_by:
  - TC-025

decided_by:
  - ADR-020

related_to:
  - TASK-022
  - TASK-024
---

# TASK-025 - Implement HTTP/2 WebSocket Support via Extended CONNECT (RFC 8441)

## Goal

Enable HTTP/2 extended CONNECT protocol settings (`EnableExtendedConnectProtocol`), support pseudo-header `:protocol = websocket` detection in request parser, and update HTTP/2 adapter & reverse proxy to stream data over HTTP/2 WebSocket channels.

## Sub-tasks

1. Add `IsHTTP2WebSocketUpgrade()` helper method to `pkg/httpparser/request.go`.
2. Configure `EnableExtendedConnectProtocol: true` on `http2.Server` instances in `pkg/server/server.go`.
3. Update `http2AdapterHandler` in `pkg/server/server.go` to handle streaming extended CONNECT requests and copy stream data between client HTTP/2 stream and upstream target.
4. Update `pkg/proxy/proxy.go` to handle RFC 8441 HTTP/2 WebSocket requests when proxied.
5. Write unit tests for HTTP/2 RFC 8441 WebSocket support in `pkg/server/http2_test.go` or `pkg/server/server_test.go`.
6. Run full test suite `go test -count=1 ./...` to verify clean execution.

## Acceptance Criteria

- HTTP/2 server advertises `SETTINGS_ENABLE_CONNECT_PROTOCOL` to connecting clients.
- Extended `CONNECT` requests with `:protocol: websocket` are parsed and routed correctly.
- Streams return `200 OK` and relay frame payload data bi-directionally.
- All Go unit tests pass cleanly.
