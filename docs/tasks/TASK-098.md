---
id: TASK-098
type: task
title: Bidirectional Activity-Refreshed Deadline Relay for HTTP/1.1 Upgrades
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-088
  - TASK-097

owns:
  - pkg/server/server.go

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

# TASK-098 - Bidirectional Activity-Refreshed Deadline Relay for HTTP/1.1 Upgrades

## Overview

Replace the permanent socket deadline clearance and unbounded stream copying in [`pkg/server/server.go:L272-L294`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L272-L294) with an active, bidirectional activity-refreshed deadline relay. Enforce [`UpgradeIdleTimeout`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L6) across upgraded HTTP/1.1 connections (such as RFC 6455 WebSockets and raw bidirectional TCP tunnels), ensuring both client socket and upstream backend socket are deterministically terminated upon prolonged inactivity. This eliminates Slowloris socket exhaustion and permanent goroutine leaks ([`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392), CWE-400).

## Scope & Implementation Breakdown

1. **Eliminate Unbounded Deadlines & Raw Copying ([`pkg/server/server.go:L272-L294`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L272-L294))**:
   - Remove `conn.SetDeadline(time.Time{})` and `res.UpgradedConn.SetDeadline(time.Time{})`.
   - Remove unbounded `io.Copy(res.UpgradedConn, conn)` and `io.Copy(conn, res.UpgradedConn)`.
   - Resolve effective idle timeout from server configuration:
     - `idleTimeout := s.config.UpgradeIdleTimeout`
     - If `idleTimeout <= 0`, fall back to `s.config.IdleTimeout`.
     - If still `<= 0`, fall back to `60 * time.Second`.

2. **Implement Bidirectional Activity-Refreshed Relay Helper ([`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go))**:
   - Implement `relayUpgradedStreams(conn1, conn2 net.Conn, idleTimeout time.Duration)` (or equivalent stream forwarding function):
     - Initialize initial read/write deadlines on both `conn1` and `conn2` using `time.Now().Add(idleTimeout)`.
     - Forward data concurrently between `conn1` -> `conn2` and `conn2` -> `conn1` using fixed chunk buffers (e.g. 32 KB).
     - On every chunk transferred, refresh read and write deadlines on both sockets (`SetReadDeadline` / `SetWriteDeadline` or `SetDeadline`).
     - Support WebSocket ping/pong frames, application keep-alives, and regular data transparently, refreshing deadlines on byte transmission.

3. **Deterministic Teardown & Safe Socket Synchronization ([`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go))**:
   - Use `sync.Once` to coordinate teardown across forwarding directions so closing both sockets occurs safely without double-close panics or race conditions.
   - When idle inactivity exceeds `idleTimeout`, I/O operations unblock with a deadline exceeded error (`net.Error.Timeout()` or `os.ErrDeadlineExceeded`).
   - Emit diagnostic log when idle timeout expires:
     `log.Printf("[Server] Upgraded connection idle timeout reached (%s), terminating stream between %s and %s", idleTimeout, conn1.RemoteAddr(), conn2.RemoteAddr())`
   - Differentiate normal peer termination (`io.EOF` or clean peer closure) from timeout expiration; do not log normal disconnects as timeout errors.
   - Ensure both sockets are closed promptly, unblocking both forwarding goroutines and releasing operating system file descriptors (`FD`s).

4. **Half-Closed State Handling ([`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go))**:
   - If one direction receives `io.EOF` (e.g., client closed write half of TCP connection), attempt half-close propagation via `CloseWrite()` (e.g. `tcpConn.CloseWrite()`) if supported by the socket type.
   - Continue forwarding the reverse stream direction until it reaches `io.EOF` or exceeds `idleTimeout`.

5. **Wait for Teardown Completion**:
   - Wait for both relay goroutines to exit (via `sync.WaitGroup`) before returning from `handleConn`, ensuring no orphaned goroutines remain.

## Acceptance Criteria

- Unconditional deadline clearance (`SetDeadline(time.Time{})`) is eliminated for upgraded connections in [`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go).
- Bidirectional stream relay continuously refreshes deadlines on client and upstream sockets on every byte transfer.
- Idle upgraded connections with zero data in both directions are forcibly terminated after `UpgradeIdleTimeout`.
- Both client and upstream sockets are closed via `sync.Once`, unblocking all relay goroutines and releasing socket file descriptors.
- Diagnostic log is emitted upon idle timeout, detailing timeout duration and peer addresses.
- Normal disconnections and standard WebSocket close handshakes complete cleanly without timeout log noise.
- Half-closed sockets are handled gracefully without early truncation of remaining buffered responses.
- Implementation uses standard library only (`net`, `sync`, `time`, `io`, `errors`, `log`). Free of data races under `go test -race`.

## Blockers & Risks

- Avoid excessive syscall overhead by refreshing deadlines appropriately per chunk transfer rather than per byte.
- Ensure TLS connections (wrapping `crypto/tls.Conn`) or virtual buffers handle deadline setting without runtime errors.
