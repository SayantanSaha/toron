---
id: TASK-094
type: task
title: Bounded Concurrency, Bidirectional Idle Deadlines & Connection Tracking in TCPProxy
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-087
  - TASK-093

owns:
  - pkg/proxy/tcp.go

references:
  - REQ-087
  - SEC-26
  - SR-081
  - ADR-082
  - TC-087

derived_from:
  - REQ-087

implements:
  - REQ-087

verified_by:
  - TC-087
---

# TASK-094 - Bounded Concurrency, Bidirectional Idle Deadlines & Connection Tracking in TCPProxy

## Overview

Harden [`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L15) in [`pkg/proxy/tcp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go) against denial of service, memory exhaustion, and Slowloris socket descriptor starvation ([`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383), CWE-400). Implement atomic active connection tracking, immediate client connection rejection upon reaching `MaxConnections` capacity, bidirectional idle deadline tracking during stream forwarding using `IdleTimeout`, and thread-safe connection registry tracking for clean, deterministic shutdown on [`TCPProxy.Close`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L112).

## Scope & Implementation Breakdown

1. **Proxy Configuration & Options Pattern ([`pkg/proxy/tcp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go))**:
   - Extend [`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L15) struct with:
     - `maxConnections int` (default `10000`)
     - `idleTimeout time.Duration` (default `60 * time.Second`)
     - `activeConns int64` (atomic counter)
     - `connsMu sync.Mutex` and `activeConnSet map[net.Conn]struct{}` (active socket registry)
   - Support functional options or configuration arguments in [`NewTCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L26) (e.g., `WithTCPMaxConnections(n int)` and `WithTCPIdleTimeout(d time.Duration)`), ensuring existing call signatures remain compatible while allowing explicit overrides.

2. **Atomic Connection Tracking & Over-Capacity Rejection ([`pkg/proxy/tcp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go))**:
   - In [`TCPProxy.Serve`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L58-L76):
     - Upon `clientConn, err := l.Accept()`:
       - Check if `atomic.LoadInt64(&p.activeConns) >= int64(p.maxConnections)`:
         - If capacity reached, log a warning:
           `log.Printf("[TCPProxy] Maximum connection limit reached (%d), rejecting connection from %s", p.maxConnections, clientConn.RemoteAddr())`
         - Immediately close `clientConn.Close()` without proxying or dialing upstream.
         - Do not track or spawn goroutines for rejected connections.
       - If within capacity, increment `atomic.AddInt64(&p.activeConns, 1)`.
       - Launch `go p.handleConn(clientConn)`.
       - Ensure `atomic.AddInt64(&p.activeConns, -1)` is decremented when connection handling completes.

3. **Bidirectional Idle Deadline Tracking (Slowloris Mitigation) ([`pkg/proxy/tcp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go))**:
   - In [`TCPProxy.handleConn`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L78-L109):
     - Replace unbounded blocking `io.Copy` transfers with a deadline-enforcing bidirectional copy routine.
     - Before each read/write operation, refresh deadlines:
       `clientConn.SetDeadline(time.Now().Add(p.idleTimeout))`
       `upstreamConn.SetDeadline(time.Now().Add(p.idleTimeout))`
     - If either side remains idle with zero data transferred for longer than `IdleTimeout`, the I/O call fails with a timeout error, prompting immediate termination of both `clientConn` and `upstreamConn`.
     - Log diagnostic message upon idle timeout:
       `log.Printf("[TCPProxy] Idle timeout (%s) reached for connection %s <-> %s, terminating stream", p.idleTimeout, clientConn.RemoteAddr(), upstreamConn.RemoteAddr())`
     - Maintain half-close propagation (`CloseWrite()` on TCP sockets) where appropriate, while keeping idle timeouts active on surviving stream directions until full closure.

4. **Connection Registry & Graceful Teardown on `Close()` ([`pkg/proxy/tcp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go))**:
   - Register established `clientConn` and `upstreamConn` into `activeConnSet` under `connsMu`.
   - Ensure connections are unregistered upon termination.
   - In [`TCPProxy.Close`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L112-L127):
     - Mark proxy closed and close `p.listener`.
     - Under `connsMu`, iterate over all open sockets in `activeConnSet` and invoke `conn.Close()` to interrupt pending I/O and unblock forwarding goroutines.
     - Clear the registry.

## Acceptance Criteria

- Incoming connections exceeding `MaxConnections` are rejected immediately with warning logs, maintaining constant active connection count.
- Active connection counter is decremented accurately on socket close, client abort, error, or timeout.
- Both client and upstream connections enforce `IdleTimeout`; stagnant connections are terminated cleanly without leaking file descriptors or goroutines.
- [`TCPProxy.Close`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L112) closes the listener and forces closure of all open client and upstream sockets.
- Zero third-party dependencies. Clean execution under `go test -race`.

## Blockers & Risks

- Ensure deadline refresh does not introduce excessive lock contention or syscall overhead on high-throughput continuous streams.
- Ensure half-closed TCP streams (`CloseWrite`) are handled gracefully without prematurely aborting active reverse data transfers.
