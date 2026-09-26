---
id: TASK-172
type: task
title: Zero-Copy Request Body Proxy Streaming in pkg/proxy
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-25
updated: 2026-09-25

depends_on:
  - ../requirements/REQ-145.md
  - ./TASK-169.md
  - ./TASK-170.md
  - ./TASK-171.md

derived_from:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md

implements:
  - ../requirements/REQ-145.md

related_to:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md
  - ../architecture/ADR-084.md
  - ../requirements/REQ-133.md
  - ./TASK-169.md
  - ./TASK-170.md
  - ./TASK-171.md
---

# TASK-172 - Zero-Copy Request Body Proxy Streaming in pkg/proxy

## 1. Overview & Objective

Decompose the zero-copy request body proxy streaming and decoupled response timeout requirements of [`REQ-145`](../requirements/REQ-145.md) (FR-145-4, FR-145-6, NFR-145-1) and [`AN-006`](../analysis/AN-006.md) (Section 3.4) into concrete deliverables within [`pkg/proxy/proxy.go`](../../pkg/proxy/proxy.go).

Currently, in [`pkg/proxy/proxy.go:1164`](../../pkg/proxy/proxy.go#L1164), Toron executes an eager in-memory buffering operation on incoming request bodies:
```go
bodyBytes, err := io.ReadAll(req.Body)
if err == nil && len(bodyBytes) > 0 {
    bodyReader = bytes.NewReader(bodyBytes)
}
```
This forces all request payloads into heap slices twice (once in parser, once in proxy), causing memory bloat and immediate OOM crashes on large payloads ($200\,\text{MB}+$, [`SEC-28`](../architecture/ADR-084.md)). In addition, the upstream reverse proxy transport enforces a hardcoded fallback timeout (`ResponseHeaderTimeout: 10s`) that terminates slow legacy backend transactions prematurely.

This task eliminates duplicate in-memory body buffering by piping `req.Body` directly to the upstream `http.Transport`, enforces constant $O(1)$ memory consumption, configures route-scoped `ResponseHeaderTimeout`, and ensures bidirectional cancellation propagation.

---

## 2. Traceability

- **Requirement**: [`REQ-145`](../requirements/REQ-145.md) (FR-145-4, FR-145-6, NFR-145-1, AC-145-01, AC-145-05)
- **Analysis**: [`AN-006`](../analysis/AN-006.md) (Section 3.4, Section 8.2 Item 4)
- **Dependencies**:
  - [`TASK-169.md`](./TASK-169.md): Configuration Schema Extensions
  - [`TASK-170.md`](./TASK-170.md): Two-Phase HTTP Request Parsing in `pkg/httpparser`
  - [`TASK-171.md`](./TASK-171.md): Activity-Refreshed Read Deadline Tracker in `pkg/server`

---

## 3. Work Package Breakdown

### WP-1: Direct Zero-Copy Body Streaming to Upstream Transport ([`pkg/proxy/proxy.go`](../../pkg/proxy/proxy.go))

Refactor request body preparation in `p.ServeHTTP`:
1. **Streaming Ingress Activation**:
   - Determine whether streaming ingress should activate:
     ```go
     shouldStream := p.opts.StreamRequestBody != nil && *p.opts.StreamRequestBody
     if p.opts.StreamRequestBody == nil {
         // Auto-stream for payloads > 64 KB or chunked transfers (ContentLength == -1)
         shouldStream = req.ContentLength > 65536 || req.ContentLength == -1
     }
     ```
2. **Elimination of `io.ReadAll`**:
   - When `shouldStream` is true:
     - Assign `bodyReader = req.Body` directly.
     - Bypass `io.ReadAll(req.Body)` completely.
     - For fixed-length payloads, assign `outReq.ContentLength = req.ContentLength`.
     - For chunked payloads, assign `outReq.ContentLength = -1` and set `outReq.TransferEncoding = []string{"chunked"}`.
     - Upstream `http.Transport` will read chunks directly from the socket wrapper (`activityReader` / `StreamingBodyReader`), transferring bytes with $O(1)$ memory overhead ($\le 64\,\text{KB}$).
3. **Small Payload Optimization**:
   - For small non-streaming payloads ($\le 64\,\text{KB}$), continue using pooled buffers (`httpparser.GetBodyBuffer()`) to avoid per-request allocations.

### WP-2: Route-Scoped `ResponseHeaderTimeout` Integration ([`pkg/proxy/proxy.go`](../../pkg/proxy/proxy.go))

1. **Transport Configuration & Dynamic Timeout**:
   - In `ResolveTransport` and proxy request execution:
     - Allow route-level `ResponseHeaderTimeout` to override the default 10-second transport ceiling.
     - Bind an upstream context timeout matching the effective route response header timeout:
       ```go
       effectiveRespTimeout := p.opts.ResponseHeaderTimeout
       if effectiveRespTimeout <= 0 {
           effectiveRespTimeout = 10 * time.Second
       }
       reqCtx, cancel := context.WithTimeout(req.Context(), effectiveRespTimeout)
       defer cancel()
       ```
2. **Error Translation & Fast-Fail**:
   - If upstream context deadline expires before headers are received:
     - Log upstream timeout event.
     - Return `HTTP 504 Gateway Timeout` with JSON body:
       `{"error":"504 Gateway Timeout: Upstream response header timeout expired"}`.
   - If upstream backend closes the connection, resets TCP, or returns a transport dial error:
     - Return `HTTP 502 Bad Gateway` with JSON body:
       `{"error":"502 Bad Gateway: Upstream connection failed"}`.

### WP-3: Bidirectional Cancellation & Fail-Closed Semantics ([`pkg/proxy/proxy.go`](../../pkg/proxy/proxy.go))

1. **Client Downstream Abort Propagation**:
   - When downstream client disconnects or socket terminates during upload:
     - Client context cancellation immediately triggers `cancel()` on the upstream context (`reqCtx`).
     - Upstream `http.Transport` tears down the backend socket immediately, preventing orphaned backend execution.
2. **Upstream Streaming Abort (Fail-Closed)**:
   - If upstream backend resets or errors out midway through streaming request or response data:
     - Forcefully terminate the downstream connection without sending clean EOF or trailer framing.
     - Ensure client is alerted to partial payload truncation.

### WP-4: Automated Verification Test Suite ([`pkg/proxy/proxy_test.go`](../../pkg/proxy/proxy_test.go))

1. **Large Upload Constant Memory Test**:
   - Stream a 100MB payload through the proxy to a dummy upstream server.
   - Monitor heap allocations during transfer; assert heap memory growth $\le 10\,\text{MB}$ ($O(1)$ verification, AC-145-01 / NFR-145-1).
2. **Decoupled Backend Execution Test**:
   - Configure proxy with `ResponseHeaderTimeout: 5s`.
   - Upstream server sleeps for 3 seconds before sending headers; verify successful `HTTP 200 OK`.
   - Upstream server sleeps for 7 seconds; verify proxy returns `HTTP 504 Gateway Timeout`.
3. **Downstream Client Disconnect Propagation**:
   - Client initiates large upload and abruptly closes socket after transmitting 500KB.
   - Verify upstream server receives context cancellation within 100ms.
4. **Upstream Mid-Stream Abort Test**:
   - Upstream abruptly drops TCP connection during response streaming.
   - Verify downstream connection is terminated fail-closed without corrupt completion status.

---

## 4. Acceptance Criteria

- [ ] **AC-172-1 (Zero Eager In-Memory Buffering)**: Request bodies exceeding 64KB stream directly from `req.Body` to upstream `outReq.Body` without executing `io.ReadAll` or allocating heap slices.
- [ ] **AC-172-2 (Constant $O(1)$ Memory Footprint)**: Processing 100MB+ streaming payloads results in $< 10\,\text{MB}$ total heap allocation delta during active transfer.
- [ ] **AC-172-3 (Route-Scoped ResponseHeaderTimeout)**: Upstream backends requiring extended execution time (e.g. 15s–60s) complete successfully when route configures matching `ResponseHeaderTimeout`.
- [ ] **AC-172-4 (Gateway Timeout on Slow Upstream)**: Upstream backends failing to emit headers within configured `ResponseHeaderTimeout` trigger `HTTP 504 Gateway Timeout`.
- [ ] **AC-172-5 (Client Abort Propagation)**: Premature client socket closure immediately cancels upstream request context and terminates backend connection.
- [ ] **AC-172-6 (Fail-Closed Truncation Defense)**: Mid-stream upstream network failures trigger immediate downstream connection termination.

---

## 5. Constraints & Standards

- **Strictly Relative Links**: All internal document links must use relative syntax (`../requirements/REQ-145.md`, `../../pkg/proxy/proxy.go`).
- **Memory Safety**: No monolithic slices (`make([]byte, size)`) for streaming payloads.
- **Zero External Dependencies**: Pure Go standard library packages (`net/http`, `io`, `context`, `time`).
