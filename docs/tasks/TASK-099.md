---
id: TASK-099
type: task
title: Idle Deadline Enforcement for HTTP/2 Extended CONNECT Upgraded Streams
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

# TASK-099 - Idle Deadline Enforcement for HTTP/2 Extended CONNECT Upgraded Streams

## Overview

Harden HTTP/2 Extended CONNECT protocol upgrades (RFC 8441 WebSockets over HTTP/2) in [`http2AdapterHandler`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L310-L382) within [`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go). Replace unbounded stream copying with an activity-monitored forwarding pipeline enforcing [`UpgradeIdleTimeout`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go#L6) and responsive request context cancellation (`r.Context().Done()`). Ensure upstream backend sockets ([`res.UpgradedConn`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L365)) and forwarding worker goroutines are deterministically torn down when idle timeout expires or when the client resets the stream, remediating [`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392) (CWE-400).

## Scope & Implementation Breakdown

1. **Target Area in [`http2AdapterHandler`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L365-L376)**:
   - In [`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go), replace lines 365-376:
     ```go
     if res.UpgradedConn != nil {
         if flusher, ok := w.(http.Flusher); ok {
             flusher.Flush()
         }
         go func() {
             _, _ = io.Copy(res.UpgradedConn, r.Body)
             _ = res.UpgradedConn.Close()
         }()
         _, _ = io.Copy(w, res.UpgradedConn)
         _ = res.UpgradedConn.Close()
         return
     }
     ```
   - Resolve effective idle timeout:
     - `idleTimeout := s.config.UpgradeIdleTimeout`
     - Fall back to `s.config.IdleTimeout` or `60 * time.Second` if `<= 0`.

2. **Bidirectional Activity-Monitored Forwarding**:
   - Implement activity tracking on the HTTP/2 stream relay between `r.Body` / `w` and `res.UpgradedConn`:
     - Maintain atomic timestamp of last transferred byte (e.g. `atomic.Int64` unix nano).
     - Refresh read and write deadlines on `res.UpgradedConn`:
       `res.UpgradedConn.SetDeadline(time.Now().Add(idleTimeout))`
     - On each chunk forwarded from `r.Body` to `res.UpgradedConn`, update activity timestamp and refresh `res.UpgradedConn` write/read deadlines.
     - On each chunk forwarded from `res.UpgradedConn` to `w`, flush `w` (via `http.Flusher`), update activity timestamp, and refresh deadlines.
   - Run a dedicated inactivity monitor or timer:
     - If zero data is transferred across both directions for longer than `idleTimeout`:
       - Forcibly close `res.UpgradedConn`.
       - Unblock both forwarders and terminate the stream handler.
       - Emit diagnostic log:
         `log.Printf("[Server] HTTP/2 extended CONNECT upgraded stream idle timeout reached (%s), terminating stream for %s", idleTimeout, r.RemoteAddr)`

3. **Immediate Teardown on Request Context Cancellation**:
   - Monitor `r.Context().Done()` concurrently.
   - If the client aborts, disconnects, or sends an HTTP/2 `RST_STREAM` frame:
     - Immediately invoke `res.UpgradedConn.Close()`.
     - Promptly abort active copying operations without waiting for `UpgradeIdleTimeout`.

4. **Synchronized Teardown & Goroutine Lifetime Guarantee**:
   - Guard `res.UpgradedConn.Close()` invocation with `sync.Once` to prevent double-close races across upstream copier, downstream copier, context monitor, and idle timer.
   - Ensure all launched goroutines (upstream copier, downstream copier, watchdog) terminate and are joined (e.g. via `sync.WaitGroup` or completion channels) before `http2AdapterHandler` returns, eliminating goroutine leaks.

## Acceptance Criteria

- Idle HTTP/2 Extended CONNECT streams with zero activity are terminated after `UpgradeIdleTimeout`.
- Client stream context cancellation (`r.Context().Done()`) immediately closes `res.UpgradedConn` and terminates stream forwarding goroutines.
- Data transferred in either direction refreshes the activity deadline, allowing active streams (including heartbeat frames) to remain open indefinitely.
- Upstream socket closure is protected by `sync.Once`, eliminating data races and panic conditions.
- Diagnostic log is emitted upon idle timeout expiration, identifying duration and client address.
- Zero external dependencies (strictly Go standard library packages `net`, `net/http`, `sync`, `sync/atomic`, `time`, `io`, `log`).
- Clean execution under `go test -race ./pkg/server/...`.

## Blockers & Risks

- Unlike raw TCP sockets, `http.ResponseWriter` and `r.Body` over HTTP/2 do not expose direct `SetDeadline` methods. The idle watchdog and deadline enforcement on `res.UpgradedConn` must ensure reading from `r.Body` and writing to `w` are cleanly interrupted when the upstream socket is closed.
- Ensure `http.Flusher.Flush()` is called after writing chunks to `w` so HTTP/2 data frames are transmitted immediately to the client without buffering latency.
