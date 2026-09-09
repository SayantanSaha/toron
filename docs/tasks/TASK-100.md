---
id: TASK-100
type: task
title: Automated Verification Test Suite for Upgraded Connection Idle Deadlines
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-088
  - TASK-097
  - TASK-098
  - TASK-099

owns:
  - pkg/server/server_test.go

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

# TASK-100 - Automated Verification Test Suite for Upgraded Connection Idle Deadlines

## Overview

Implement an automated test suite in [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go) (and dedicated test files in `pkg/server/`) verifying configurable inactivity deadline enforcement, activity-refreshed lifetime extensions via ping/pong heartbeats, clean peer disconnection teardown, and HTTP/2 Extended CONNECT idle stream termination. Validate test specification [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md), proving complete remediation of [`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392) (CWE-400 Slowloris socket exhaustion) without data races or resource leaks.

## Scope & Implementation Breakdown

1. **Idle HTTP/1.1 WebSocket Upgrade Termination Test (`TestServer_UpgradedConn_IdleTimeout`)**:
   - Start a Toron test server with a short `UpgradeIdleTimeout` (e.g. `150ms`).
   - Register a route that simulates a protocol upgrade (`101 Switching Protocols`) connected to a mock upstream TCP echo server.
   - Client sends HTTP/1.1 WebSocket upgrade handshake and receives `101 Switching Protocols`.
   - Perform an initial data transfer round-trip to verify active relaying.
   - Cease all data transmission from both client and upstream sides.
   - Assert that within `UpgradeIdleTimeout + 50ms`:
     - Client connection read returns `io.EOF` or closed network error.
     - Upstream mock connection is closed.
     - Operating system socket descriptors and relay goroutines are completely released.

2. **Heartbeat / Active Stream Preservation Test (`TestServer_UpgradedConn_HeartbeatKeepsAlive`)**:
   - Configure server with `UpgradeIdleTimeout: 200ms`.
   - Establish upgraded connection between client and upstream backend.
   - Client transmits periodic ping/heartbeat packets every `70ms` (well below the 200ms idle threshold) over a sustained duration of `600ms` (3x the idle timeout).
   - Assert that the upgraded connection remains continuously open and functional throughout the duration.
   - Cease heartbeat transmissions and assert that the connection subsequently terminates after 200ms of inactivity.

3. **Peer Disconnection Teardown Tests (`TestServer_UpgradedConn_PeerClose`)**:
   - **Client-Initiated Closure**:
     - Client abruptly closes connection after handshake.
     - Assert that the upstream socket is promptly closed without waiting for `UpgradeIdleTimeout`.
   - **Upstream-Initiated Closure**:
     - Upstream server terminates its socket.
     - Assert that the client socket receives `io.EOF` immediately, and both relay goroutines terminate cleanly.

4. **HTTP/2 Extended CONNECT Idle Teardown Test (`TestServer_HTTP2_ExtendedCONNECT_IdleTimeout`)**:
   - Configure server with `UpgradeIdleTimeout: 150ms`.
   - Establish an RFC 8441 HTTP/2 Extended CONNECT tunnel (`Method: "CONNECT"`, `:protocol: "websocket"`) using `http2AdapterHandler`.
   - Keep the HTTP/2 stream idle with no data chunks transferred in either direction.
   - Assert that after `UpgradeIdleTimeout`, the upstream connection `res.UpgradedConn` is closed and stream handler exits.
   - Test stream context cancellation: cancel `r.Context()` and assert that `res.UpgradedConn` is closed immediately.

5. **Configuration Fallback and Validation Tests (`pkg/config/loader_test.go`)**:
   - Verify `DefaultConfig()` initializes `UpgradeIdleTimeout` to `60s`.
   - Verify `ToServerConfig()` falls back to `IdleTimeout` when `UpgradeIdleTimeout <= 0`.
   - Verify `ValidateConfig()` rejects negative `UpgradeIdleTimeout` values.

6. **Race-Clean Execution & Concurrency Invariants**:
   - Execute all unit and integration tests under `go test -v -race ./pkg/server/... ./pkg/config/...`.
   - Verify zero data races, deadlocks, or leaked goroutines.

## Acceptance Criteria

- `TestServer_UpgradedConn_IdleTimeout` verifies automatic termination of idle HTTP/1.1 upgraded connections after `UpgradeIdleTimeout`.
- `TestServer_UpgradedConn_HeartbeatKeepsAlive` proves periodic heartbeats/pings successfully refresh deadlines and preserve long-lived connections.
- `TestServer_UpgradedConn_PeerClose` verifies immediate bidirectional teardown on peer close.
- `TestServer_HTTP2_ExtendedCONNECT_IdleTimeout` verifies timeout and context cancellation on HTTP/2 Extended CONNECT streams.
- Full test suite passes cleanly under `go test -v -race ./pkg/server/... ./pkg/config/...`.
- Zero third-party dependencies (pure standard Go library: `testing`, `net`, `net/http`, `time`, `io`, `sync`).

## Blockers & Risks

- Ensure test timeouts (150ms-200ms) provide sufficient margin to prevent flakiness in slow or heavily loaded CI environments while executing rapidly.
- Ensure mock upstream backends bind to loopback address with dynamic port assignment (`127.0.0.1:0`).
