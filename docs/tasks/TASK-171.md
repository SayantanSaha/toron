---
id: TASK-171
type: task
title: Activity-Refreshed Read Deadline Tracker and Downstream Timeout Decoupling in pkg/server
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
  - ./TASK-173.md

derived_from:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md

implements:
  - ../requirements/REQ-145.md

related_to:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md
  - ../requirements/REQ-005.md
  - ../requirements/REQ-126.md
  - ../architecture/ADR-126.md
  - ./TASK-169.md
  - ./TASK-170.md
  - ./TASK-172.md
  - ./TASK-173.md
---

# TASK-171 - Activity-Refreshed Read Deadline Tracker and Downstream Timeout Decoupling in pkg/server

## 1. Overview & Objective

Decompose the activity deadline tracking and downstream timeout decoupling requirements of [`REQ-145`](../requirements/REQ-145.md) (FR-145-1, FR-145-3, FR-145-4) and [`AN-006`](../analysis/AN-006.md) (Section 3.2) into concrete deliverables within [`pkg/server/server.go`](../../pkg/server/server.go).

Under Toron's current server architecture:
1. Sockets enforce an absolute, static read deadline (`SetAmortizedReadDeadline(5s)`). While effective against Slowloris attacks, this aborts legitimate large uploads (e.g. 200MB over a 10MB/s link requiring 20 seconds) with an unrecoverable `i/o timeout` at second 5.
2. The downstream socket write deadline is statically locked at 5s (`WriteTimeout`). When Toron proxies to a legacy backend that takes 15s–60s to produce response headers, the downstream client connection write deadline expires, dropping the client connection mid-flight.

This task introduces:
1. An `activityReader` that refreshes the socket read deadline forward on byte progress, enabling sustained high-volume uploads while preserving strict Slowloris stall termination.
2. Two-phase connection handling in `s.handleConn`: parsing headers, evaluating route limits, fast-failing pre-read oversized payloads (`HTTP 413`), and checking route bulkheads (`HTTP 503`).
3. Decoupling of the downstream write deadline during upstream backend processing to span the route's configured `ResponseHeaderTimeout`.

---

## 2. Traceability

- **Requirement**: [`REQ-145`](../requirements/REQ-145.md) (FR-145-1, FR-145-3, FR-145-4, AC-145-02, AC-145-03, AC-145-04, AC-145-05)
- **Analysis**: [`AN-006`](../analysis/AN-006.md) (Section 3.2, Section 8.2 Item 2)
- **Dependencies**:
  - [`TASK-169.md`](./TASK-169.md): Configuration Schema Extensions
  - [`TASK-170.md`](./TASK-170.md): Two-Phase HTTP Request Parsing in `pkg/httpparser`
  - [`TASK-173.md`](./TASK-173.md): Route Bulkhead Concurrency Limiter in `pkg/router`
- **Downstream Tasks**:
  - [`TASK-172.md`](./TASK-172.md): Zero-Copy Request Body Proxy Streaming in `pkg/proxy`

---

## 3. Work Package Breakdown

### WP-1: `activityReader` Progress Tracker Implementation ([`pkg/server/server.go`](../../pkg/server/server.go))

Implement `activityReader` wrapping the underlying network connection and buffered reader:
```go
type activityReader struct {
    r             *bufio.Reader
    conn          net.Conn
    tracker       *connDeadlineTracker
    timeout       time.Duration
    remaining     int64
    totalRead     int64
    minRateBytes  int64
    windowStart   time.Time
    windowBytes   int64
    closed        bool
    mu            sync.Mutex
}

func newActivityReader(r *bufio.Reader, conn net.Conn, tracker *connDeadlineTracker, timeout time.Duration, limit int64) *activityReader
```
Mechanics & Defensive Guarantees:
1. **Activity-Based Renewal**:
   - Before executing a blocking read, or immediately after a successful read where $n > 0$, refresh the socket read deadline:
     ```go
     if a.timeout > 0 {
         _ = a.tracker.SetAmortizedReadDeadline(a.timeout)
     }
     ```
   - This ensures continuous data transfers (e.g. 200MB taking 60 seconds) can proceed uninterrupted without triggering premature socket timeouts.
2. **Stall & Slowloris Invariant (REQ-005 / CWE-400)**:
   - If the client halts transmission or pauses mid-transfer longer than `a.timeout`, no renewal occurs. The operating system socket timer fires, unblocking the worker with `i/o timeout` and severing the connection.
3. **Cumulative Byte Clamping**:
   - Track `a.remaining -= int64(n)`. If client delivers more bytes than `limit`, immediately abort and return `httpparser.ErrBodyTooLarge`.
4. **Progress Rate Clamping (Anti-Drip Defense)**:
   - Enforce a minimum throughput rate threshold across each deadline window (e.g., minimum 1 KB per deadline window) to prevent low-rate drip-feeding Slowloris attacks (e.g., 1 byte every 4.9 seconds). If progress falls below threshold across the window, terminate connection.

### WP-2: Two-Phase Pipeline Refactoring in `s.handleConn` ([`pkg/server/server.go`](../../pkg/server/server.go))

Refactor `s.handleConn(ctx, conn)` request dispatch:
1. **Phase 1 (Header Ingestion)**:
   - Set socket read deadline to `s.config.ReadTimeout` (or `s.config.IdleTimeout` on keep-alive).
   - Ingest request line and headers via `httpparser.ParseHeader(br, opts)`.
   - On header parse errors (`ErrHeaderTooLarge`, malformed URI, smuggling), serialize error response and close connection.
2. **Interim Route Resolution**:
   - Query router using `s.router.LookupPrefixRoute(req)` to resolve the matched route's configuration:
     - `effectiveMaxBody = route.GetMaxBodyBytes(s.config.MaxBodyBytes)`
     - `effectiveReadTimeout = route.GetReadTimeout(s.config.ReadTimeout)`
     - `effectiveRespTimeout = route.GetResponseHeaderTimeout(10 * time.Second)`
3. **Fast-Fail Content-Length Validation (FR-145-2 / AC-145-02)**:
   - If `req.ContentLength > effectiveMaxBody`:
     - Reject immediately with `HTTP 413 Payload Too Large`.
     - Set `Connection: close` and serialize response.
     - Close connection immediately without reading any payload body bytes from the socket.
4. **Bulkhead Concurrency Gate Evaluation (FR-145-5 / AC-145-06)**:
   - Attempt to acquire route concurrency slot via `s.router.TryAcquireRouteSlot(req)`.
   - If capacity is exhausted:
     - Immediately serialize `HTTP 503 Service Unavailable` with `Retry-After: 5` and `{"error":"503 Service Unavailable: Route concurrency limit reached"}`.
     - Close connection and yield worker goroutine without reading body or dialing upstream.
5. **Phase 2 (Governed Body Ingestion)**:
   - If `req.ContentLength > 0` or chunked:
     - Wrap socket stream in `newActivityReader(br, conn, tracker, effectiveReadTimeout, effectiveMaxBody)`.
     - Assign to `req.Body`.
     - Tag request context with cleanup release callbacks.

### WP-3: Client Downstream Write Deadline Decoupling ([`pkg/server/server.go`](../../pkg/server/server.go))

1. **Downstream Timeout Extension during Upstream Wait**:
   - When routing to an upstream proxy route, decouple the downstream client write deadline from static `s.config.WriteTimeout` (5s).
   - Extend the downstream write deadline forward to:
     $$\text{downstreamWriteDeadline} = \text{effectiveRespTimeout} + \text{s.config.WriteTimeout}$$
   - This ensures Toron does not sever the client TCP connection while a legacy upstream backend takes 15s to 60s to generate response headers.
2. **Streaming Response Relay Synchronization**:
   - Once upstream response headers arrive and `s.relayStreamBody` begins transmitting chunks to the downstream client, revert to activity-refreshed write deadlines on each transmitted chunk, ensuring sustained downstream downloads complete smoothly.

### WP-4: Automated Verification Test Suite ([`pkg/server/server_test.go`](../../pkg/server/server_test.go))

1. **Sustained Slow Upload Test**:
   - Configure route with `read_timeout: 2s`, `max_body_bytes: 10MB`.
   - Client transmits 5MB at 1MB/s over 5 seconds (interval between reads = 1s < 2s).
   - Verify upload completes successfully with `HTTP 200 OK` via activity refresh (no `i/o timeout`).
2. **Slowloris Stalled Upload Test**:
   - Client transmits 100 bytes and halts transmission for 3 seconds (exceeding `read_timeout: 2s`).
   - Verify server triggers socket deadline, terminates transfer within 2 seconds of stall, closes connection, and releases worker.
3. **Pre-Read 413 Fast-Fail Test**:
   - Route configured with `max_body_bytes: 1MB`.
   - Client transmits headers with `Content-Length: 10485760` (10MB).
   - Verify server immediately responds with `HTTP 413 Payload Too Large` and zero payload bytes are read from socket.
4. **Decoupled Backend Latency Test**:
   - Route configured with `response_header_timeout: 10s`, server `write_timeout: 2s`.
   - Upstream backend takes 5 seconds to emit response headers.
   - Verify client connection remains open and receives `HTTP 200 OK` without premature write timeout failure.

---

## 4. Acceptance Criteria

- [ ] **AC-171-1 (Activity-Refreshed Streaming Ingress)**: Sustained data transfers where read intervals remain below `ReadTimeout` complete without `i/o timeout`, regardless of total duration (e.g. 30s–60s transfers).
- [ ] **AC-171-2 (Strict Slowloris Termination)**: Any client connection pausing transmission longer than `ReadTimeout` is terminated within that timeout window, freeing worker goroutines and socket descriptors.
- [ ] **AC-171-3 (Zero-Body Pre-Read 413 Rejection)**: Requests with `Content-Length` exceeding the resolved route limit receive `HTTP 413 Payload Too Large` during Phase 1 routing evaluation before reading body bytes.
- [ ] **AC-171-4 (Bulkhead Fast-Fail Gating)**: Requests arriving when a route's bulkhead is saturated receive `HTTP 503 Service Unavailable` with `Retry-After: 5` before body ingestion.
- [ ] **AC-171-5 (Decoupled Downstream Write Timeout)**: Upstream backend processing spanning up to `ResponseHeaderTimeout` does not trigger downstream client socket write deadline termination.
- [ ] **AC-171-6 (Regression Immunity)**: Microservices and static asset routes retain baseline static timeout and ceiling enforcement without behavioral regressions.

---

## 5. Constraints & Standards

- **Strictly Relative Links**: All internal document links must use relative syntax (`../requirements/REQ-145.md`, `../../pkg/server/server.go`).
- **Deadlock & Leak Safety**: Connection trackers, activity readers, and deferred releases must execute deterministically across all client abort, error, and timeout code paths.
- **Zero External Dependencies**: Pure Go standard library networking and synchronization primitives.
