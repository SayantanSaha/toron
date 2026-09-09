---
id: TASK-095
type: task
title: Bounded Worker Pool, sync.Pool Buffer Recycling & Session Socket Reuse in UDPProxy
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
  - pkg/proxy/udp.go

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

# TASK-095 - Bounded Worker Pool, sync.Pool Buffer Recycling & Session Socket Reuse in UDPProxy

## Overview

Harden [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L14) in [`pkg/proxy/udp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go) against denial of service, ephemeral port starvation, memory exhaustion (OOM), and socket file descriptor exhaustion (`EMFILE`) under high-frequency datagram packet floods ([`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383), CWE-400). Replace unthrottled per-packet goroutines with a bounded worker pool limited to `MaxWorkers` (default 1024), recycle 64 KB datagram buffers via `sync.Pool`, establish an upstream UDP session registry caching outbound sockets per client address with `IdleTimeout` expiration, and ensure complete teardown of session sockets and cleaner routines on [`UDPProxy.Close`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L111).

## Scope & Implementation Breakdown

1. **Proxy Configuration & Options Pattern ([`pkg/proxy/udp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go))**:
   - Extend [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L14) struct with:
     - `maxWorkers int` (default `1024`)
     - `idleTimeout time.Duration` (default `60 * time.Second`)
     - `packetQueue chan udpPacketTask` (bounded task channel)
     - `bufPool sync.Pool` (allocating `make([]byte, 65535)`)
     - `sessionMu sync.RWMutex` and `sessions map[string]*udpSession`
     - `stopOnce sync.Once` and `stopCleaner chan struct{}`
   - Support functional options or configuration arguments in [`NewUDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L25) (e.g., `WithUDPMaxWorkers(n int)` and `WithUDPIdleTimeout(d time.Duration)`), ensuring existing call sites remain backward compatible.

2. **Bounded Worker Pool & Saturated Queue Dropping ([`pkg/proxy/udp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go))**:
   - Replace unconstrained `go p.handleDatagram(...)` in [`UDPProxy.Serve`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L57-L78) with a worker pool consuming from a bounded channel of capacity `MaxWorkers`.
   - Initialize `MaxWorkers` background worker goroutines during proxy startup.
   - When a packet arrives in `Serve`:
     - If the channel is full, drop the datagram fail-safe to protect proxy stability.
     - Emit a diagnostic log:
       `log.Printf("[UDPProxy] Dropping datagram from %s: worker queue saturated (%d workers)", clientAddr, p.maxWorkers)`
     - Avoid unbounded heap allocations or goroutine explosion under flood traffic.

3. **Buffer Recycling with `sync.Pool` ([`pkg/proxy/udp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go))**:
   - Initialize `p.bufPool = sync.Pool{New: func() any { return make([]byte, 65535) }}`.
   - Acquire 64 KB buffers from `bufPool` for reading from incoming UDP sockets and receiving responses from upstream UDP backends.
   - Return buffers to `bufPool` once datagram forwarding completes, eliminating per-packet garbage collection spikes.

4. **Upstream UDP Session Registry & Socket Reuse ([`pkg/proxy/udp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go))**:
   - Define `udpSession` struct containing:
     - `clientAddr *net.UDPAddr`
     - `upstreamConn *net.UDPConn`
     - `targetAddr string`
     - `lastActivity atomic.Int64` (unix nano timestamp)
     - `closed chan struct{}`
   - For incoming packets from `clientAddr`:
     - Look up session by `clientAddr.String()` in `p.sessions`.
     - If existing session found:
       - Update `lastActivity` timestamp.
       - Send packet using existing cached `upstreamConn`.
     - If not found:
       - Resolve target via `SelectTarget()`.
       - Dial `net.DialUDP("udp", nil, targetUDPAddr)` once for that client session.
       - Create new `udpSession`, store in `p.sessions`.
       - Launch a response listener goroutine per session that reads upstream replies from `upstreamConn` (using pooled buffers) and writes them back to `clientAddr` via the inbound proxy socket.
   - Periodic idle session cleanup routine:
     - Run a background ticker (e.g. every `p.idleTimeout / 2` or at least every 10s).
     - Evict sessions whose `lastActivity` exceeds `p.idleTimeout`.
     - Close `session.upstreamConn` and log:
       `log.Printf("[UDPProxy] Idle session expired for client %s, closed upstream socket", clientKey)`

5. **Graceful Teardown on `Close()` ([`pkg/proxy/udp.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go))**:
   - In [`UDPProxy.Close`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L111-L126):
     - Close inbound UDP listener socket `p.conn`.
     - Signal `stopCleaner` channel to terminate idle session sweeper.
     - Close task queue channel and drain worker pool goroutines.
     - Lock `sessionMu`, close all open `upstreamConn` sockets across all cached sessions, and clear `p.sessions`.

## Acceptance Criteria

- Per-packet goroutine allocation is eliminated; concurrency is strictly bounded by `MaxWorkers`.
- Excess datagrams during extreme saturation are dropped fail-safe without memory growth or panics.
- Datagram read and response buffers use `sync.Pool`, eliminating per-packet heap allocations.
- Multiple datagrams from the same client session reuse the established upstream socket, eliminating port exhaustion (`bind: address already in use`).
- Idle sessions are closed and removed from registry after `IdleTimeout` of inactivity.
- [`UDPProxy.Close`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L111) terminates all session sockets, listener socket, and worker goroutines cleanly without hangs or race conditions.
- Zero external dependencies. Race-clean under `go test -race`.

## Blockers & Risks

- When closing an idle or terminated session, ensure the session's upstream read goroutine exits promptly when `upstreamConn` is closed.
- Protect shared session map with appropriate read/write locking to prevent race conditions during concurrent session creation and eviction sweeps.
