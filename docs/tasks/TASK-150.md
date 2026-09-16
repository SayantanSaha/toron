---
id: TASK-150
type: task
title: Implement Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-16
updated: 2026-09-16

depends_on:
  - REQ-127

derived_from:
  - REQ-127

implements:
  - REQ-127

verified_by:
  - TC-127

decided_by:
  - ADR-127

related_to:
  - TASK-004
  - ADR-001
  - ADR-021
  - TASK-148
  - TASK-149
  - REQ-125
  - REQ-126
---

# TASK-150 - Implement Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling

## 1. Overview & Objective

During high-concurrency multi-proxy differential benchmarking ([`REQ-121`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-121.md)), NGINX demonstrated ~34.1k RPS and HAProxy demonstrated ~32.3k RPS, compared to Toron's ~24.5k RPS. Deep socket inspection and profiling under [`REQ-127`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-127.md) uncovered two fundamental architectural gaps in Toron's connection acceptance and response formatting pipelines:

1. **Implicit TCP Socket Options & Nagle Buffering Latency**:
   - In [`pkg/reactor/reactor.go:178`](file:///Users/sneha/Developer/toron-research/toron/pkg/reactor/reactor.go#L178), Toron accepts incoming client sockets via `ln.Accept()` without explicitly configuring transport layer socket flags (`TCP_NODELAY`, TCP keep-alive probes, or probe timing).
   - By default, TCP sockets operate with **Nagle's algorithm** enabled. Nagle buffers small outgoing TCP segments until an acknowledgment (ACK) is received for previously transmitted segments or until a full Maximum Segment Size (MSS, ~1460 bytes) has accumulated in the kernel send queue.
   - When paired with modern operating systems' TCP Delayed ACK implementation (RFC 1122 §4.2.3.2, which delays sending ACKs by 40ms to 200ms when waiting for piggybacked return traffic), Nagle's algorithm introduces catastrophic 40ms–200ms latency spikes for small responses and streaming frames.
   - For **real-time streaming architectures** (such as Server-Sent Events / SSE `text/event-stream` enabled in [`REQ-125`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md) and [`TASK-148`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-148.md)), event updates are emitted as small text frames (e.g. `data: {"id":123,"status":"running"}\n\n`), typically 20 to 200 bytes in size. Without `TCP_NODELAY`, each individual SSE event frame stalls in the kernel buffer waiting for the client's delayed ACK, severely degrading real-time stream fidelity.
   - For standard microservices, small JSON payloads and health-check responses suffer identical latency penalties.
2. **Half-Open Connection Leaks Across Quiet Streaming Intervals**:
   - Persistent streaming connections (SSE, telemetry feeds, long-polling HTTP/1.1 keep-alive sessions) frequently experience quiet intervals between event emissions.
   - If a client abruptly disappears without an orderly TCP FIN/RST handshake (e.g., laptop lid close, mobile cellular handoff, silent NAT state timeout, or device sleep), the socket enters a half-open state.
   - Without explicit TCP keep-alive configuration (`SetKeepAlive(true)` and `SetKeepAlivePeriod(60s)`), the server reactor cannot detect dropped sockets during quiet streaming intervals. Worker goroutines, upstream proxy connections, and socket file descriptors remain pinned indefinitely, inducing resource exhaustion.
3. **Response Serialization Heap Allocations & GC Churn**:
   - In [`pkg/httpparser/response.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response.go), formatting HTTP/1.1 status lines and headers in `res.Serialize(conn)` dynamically allocates temporary buffers or string conversions. At 24.5k RPS, this allocates ~24,500 heap objects per second, driving up garbage collection pause times and CPU instruction cache pressure.

The objective of this task is to implement the engineering solutions approved in [`REQ-127`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-127.md):
1. **Explicit TCP Socket Tuning (`pkg/reactor`, `pkg/server`)**: Immediately upon accepting a client socket, extract the underlying `*net.TCPConn` (handling unwrapping across `tls.Conn`, `connDeadlineTracker`, and `prefixConn`), enforce `SetNoDelay(true)` to eliminate Nagle packet buffering delays, and enforce `SetKeepAlive(true)` with `SetKeepAlivePeriod(60s)` for robust half-open socket detection.
2. **Response Serialization Buffer Memory Recycling (`pkg/httpparser`)**: Implement and refine a zero-allocation `sync.Pool` slab in `pkg/httpparser/response.go` for status line and header formatting, ensuring clean buffer resetting before return to pool and zero heap allocations for responses with small payloads ($< 4\text{ KB}$).
3. **Automated Verification & Regression Protection**: Deliver comprehensive unit tests, allocation benchmarks (`testing.AllocsPerRun`), immediate wire delivery tests for SSE frames, keep-alive verification, and thread-safety tests under `go test -race`.

### Conflict Audit & Architecture Compliance

A conflict audit confirms complete harmony across existing Toron subsystems:
- **[`REQ-001`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-001.md) / [`ADR-001`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-001.md) (Core Event Reactor)**: Reactor already utilizes a `sync.Pool` for connection read buffers. Extending pooling to response serialization reinforces this core pattern.
- **[`REQ-021`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-021.md) / [`ADR-021`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-021.md) (Layer 4 Transport Proxies)**: Layer 4 TCP proxying manages raw socket options and connection tracking without interference.
- **[`REQ-125`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md) / [`TASK-148`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-148.md) (Direct Streaming Fast-Path)**: Disabling Nagle delays directly empowers SSE streaming by ensuring small text frames reach the wire immediately without buffering.
- **[`REQ-126`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-126.md) / [`TASK-149`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-149.md) (Adaptive Socket Deadline Amortization)**: Unwrapping helpers ensure `connDeadlineTracker` interoperates cleanly with underlying `*net.TCPConn` socket options.
- **Verdict**: **ZERO CONFLICTS.**

---

## 2. Work Breakdown Structure (WBS)

```
TASK-150: Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling
├── WP-1: Explicit TCP Socket Tuning (pkg/reactor, pkg/server)
│   ├── Subtask 1.1: Underlying *net.TCPConn Extraction & Recursive Unwrapping Helper
│   ├── Subtask 1.2: Socket Configuration Routine & TCP Option Enforcement (SetNoDelay, SetKeepAlive)
│   ├── Subtask 1.3: Reactor Accept Loop Integration (pkg/reactor/reactor.go)
│   └── Subtask 1.4: Server Connection Handler Safeguard & Wrapped Socket Support (pkg/server/server.go)
├── WP-2: Response Serialization Buffer Memory Recycling (pkg/httpparser)
│   ├── Subtask 2.1: sync.Pool Buffer Slab Implementation & Clean Resetting (pkg/httpparser/response.go)
│   ├── Subtask 2.2: Zero-Heap-Allocation Status Line & Header Formatting
│   └── Subtask 2.3: Zero-Copy Dual-Write Serialization Fast-Path & Data Sanitization
└── WP-3: Automated Verification & Test Suite
    ├── Subtask 3.1: Socket Option Verification Unit Tests (pkg/reactor, pkg/server)
    ├── Subtask 3.2: Immediate Small Frame Delivery Test (SSE Nagle Elimination)
    ├── Subtask 3.3: TCP Keep-Alive Configuration & Half-Open Socket Detection Test
    ├── Subtask 3.4: Serialization Memory Allocation Benchmarks & Zero-Allocation Tests
    └── Subtask 3.5: Concurrency, Thread-Safety & Race Cleanliness Validation (-race)
```

---

### Work Package 1 (WP-1): Explicit TCP Socket Tuning (`pkg/reactor`, `pkg/server`)

#### Subtask 1.1: Underlying `*net.TCPConn` Extraction & Recursive Unwrapping Helper
- In [`pkg/reactor`](file:///Users/sneha/Developer/toron-research/toron/pkg/reactor) or [`pkg/server`](file:///Users/sneha/Developer/toron-research/toron/pkg/server), implement a robust, recursive/iterative socket unwrapping helper `ExtractTCPConn(conn net.Conn) *net.TCPConn`:
  ```go
  // ExtractTCPConn attempts to unwrap and return the underlying *net.TCPConn from conn.
  // It transparently handles *tls.Conn, connDeadlineTracker, prefixConn, and custom wrappers.
  func ExtractTCPConn(conn net.Conn) *net.TCPConn {
      current := conn
      for current != nil {
          if tcpConn, ok := current.(*net.TCPConn); ok {
              return tcpConn
          }
          // Check standard tls.Conn via NetConn()
          if tc, ok := current.(interface{ NetConn() net.Conn }); ok {
              current = tc.NetConn()
              continue
          }
          // Check custom wrapper via Unwrap() net.Conn
          if uw, ok := current.(interface{ Unwrap() net.Conn }); ok {
              current = uw.Unwrap()
              continue
          }
          break
      }
      return nil
  }
  ```
- Verify support for all wrapped connection types across the codebase:
  1. Plain `*net.TCPConn` accepted from standard TCP listeners.
  2. `*tls.Conn` returned by `tls.NewListener` or TLS handshakes (unwrapped via `NetConn() net.Conn`).
  3. `*connDeadlineTracker` introduced in [`pkg/server/deadline.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/deadline.go) (unwrapped via `Unwrap() net.Conn`).
  4. `*prefixConn` in [`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go) used for HTTP/2 client preface buffering (unwrapped via `Unwrap() net.Conn`).
  5. Multi-level nested wrappers (e.g., `*connDeadlineTracker` wrapping `*prefixConn` wrapping `*tls.Conn` wrapping `*net.TCPConn`).

#### Subtask 1.2: Socket Configuration Routine & TCP Option Enforcement
- Implement a reusable socket configuration function `ConfigureTCPSocket(conn net.Conn) error`:
  ```go
  // ConfigureTCPSocket inspects conn and applies mission-critical TCP socket options.
  func ConfigureTCPSocket(conn net.Conn) error {
      tcpConn := ExtractTCPConn(conn)
      if tcpConn == nil {
          // Non-TCP connection (e.g. in-memory pipe or Unix domain socket) - no-op
          return nil
      }
      
      // 1. Disable Nagle's algorithm: emit small frames immediately without delayed ACK stalls
      if err := tcpConn.SetNoDelay(true); err != nil {
          return fmt.Errorf("failed to set TCP_NODELAY: %w", err)
      }
      
      // 2. Enable TCP keep-alive probes for detecting half-open sockets during quiet intervals
      if err := tcpConn.SetKeepAlive(true); err != nil {
          return fmt.Errorf("failed to enable TCP keepalive: %w", err)
      }
      
      // 3. Set keep-alive probe period to 60 seconds
      if err := tcpConn.SetKeepAlivePeriod(60 * time.Second); err != nil {
          return fmt.Errorf("failed to set TCP keepalive period: %w", err)
      }
      
      return nil
  }
  ```
- **Rationale for TCP_NODELAY**:
  - Eliminates the 40ms–200ms delayed-ACK stall on every small outgoing packet.
  - Essential for Server-Sent Events (`text/event-stream`), where individual events are sent in real-time.
  - Essential for small JSON HTTP API responses ($< 1.4\text{ KB}$), preventing latency outliers at P90/P99.
- **Rationale for TCP Keep-Alive (60s)**:
  - Detects dead clients that disconnected silently without sending FIN/RST packets.
  - Releases idle server goroutines, buffers, and upstream handles that would otherwise remain pinned indefinitely.

#### Subtask 1.3: Reactor Accept Loop Integration (`pkg/reactor/reactor.go`)
- In [`pkg/reactor/reactor.go:Serve`](file:///Users/sneha/Developer/toron-research/toron/pkg/reactor/reactor.go#L177-L206):
  - Immediately following `conn, err := ln.Accept()`:
    ```go
    conn, err := ln.Accept()
    if err != nil {
        ...
    }
    
    // Explicitly tune TCP socket options immediately upon accept
    _ = ConfigureTCPSocket(conn)
    ```
  - Ensure tuning is performed **before** `conn` is tracked in `r.trackConn(conn, true)` and enqueued to `r.tasks <- conn`.
  - Errors during socket option tuning on accepted sockets should not terminate the accept loop; log a debug message or proceed gracefully.

#### Subtask 1.4: Server Connection Handler Safeguard & Wrapped Socket Support (`pkg/server/server.go`)
- In [`pkg/server/server.go:handleConn`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L162-L206):
  - Add an idempotent safeguard call `_ = ConfigureTCPSocket(conn)` at the beginning of `handleConn`.
  - When TLS is negotiated (`conn.(*tls.Conn)`), `ConfigureTCPSocket(conn)` extracts the underlying `*net.TCPConn` via `tlsConn.NetConn()` and reinforces `TCP_NODELAY` on the raw socket.
  - Ensure that when HTTP/2 creates `prefixConn` or deadline tracking wraps the socket with `newConnDeadlineTracker(conn)`, the underlying TCP options remain active.

---

### Work Package 2 (WP-2): Response Serialization Buffer Memory Recycling (`pkg/httpparser`)

#### Subtask 2.1: `sync.Pool` Buffer Slab Implementation & Clean Resetting (`pkg/httpparser/response.go`)
- In [`pkg/httpparser/response.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response.go):
  - Refine the package-level `responseBufPool` to supply pre-allocated byte slices or `bytes.Buffer` slabs sized to $4096$ bytes ($4\text{ KB}$), accommodating status lines and typical HTTP headers with zero reallocations:
    ```go
    // responseBufPool manages recycled 4KB byte buffer slabs for HTTP response serialization.
    var responseBufPool = sync.Pool{
        New: func() any {
            b := make([]byte, 0, 4096)
            return &b
        },
    }

    func getResponseBuf() *[]byte {
        return responseBufPool.Get().(*[]byte)
    }

    func putResponseBuf(b *[]byte) {
        if b == nil {
            return
        }
        // Strict buffer hygiene: reset slice length to zero while retaining capacity
        *b = (*b)[:0]
        responseBufPool.Put(b)
    }
    ```
  - Guarantee strict thread safety and clean resetting to prevent any cross-request data leaks.

#### Subtask 2.2: Zero-Heap-Allocation Status Line & Header Formatting
- In [`pkg/httpparser/response.go:Serialize`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response.go#L100-L163):
  - Optimize status line formatting:
    - Pre-build status line byte constants for common status codes (e.g. `200 OK`, `204 No Content`, `301 Moved Permanently`, `302 Found`, `304 Not Modified`, `400 Bad Request`, `401 Unauthorized`, `403 Forbidden`, `404 Not Found`, `500 Internal Server Error`, `502 Bad Gateway`, `503 Service Unavailable`):
      ```go
      var statusLine200 = []byte("HTTP/1.1 200 OK\r\n")
      var statusLine404 = []byte("HTTP/1.1 404 Not Found\r\n")
      ...
      ```
    - For non-standard status codes, format using `strconv.AppendInt` directly into the byte slice, avoiding `strconv.Itoa` heap string escapes.
  - Optimize header formatting:
    - Append header names, separator `": "`, sanitized header values, and `"\r\n"` directly into the pooled byte buffer slice.
    - Sanitize CRLF injection without allocating new strings when no `\r` or `\n` characters exist.
  - Append final header-terminating `"\r\n"` to complete the HTTP header block.

#### Subtask 2.3: Zero-Copy Dual-Write Serialization Fast-Path & Data Sanitization
- Write header block to `w`:
  ```go
  if _, err := w.Write(*bufPtr); err != nil {
      return err
  }
  ```
- Payload Body Handling:
  - If `r.StreamBody != nil`:
    - Headers are flushed immediately; body relaying is delegated to the server streaming loop.
    - Return `nil` immediately.
  - If `r.StreamBody == nil && r.Body != nil && r.Body.Len() > 0`:
    - Write body payload directly to `w` via `_, err := w.Write(r.Body.Bytes())` or `r.Body.WriteTo(w)`.
    - Eliminates monolithic concatenated buffer allocations combining headers and body.
- Return the buffer to `responseBufPool` via `defer putResponseBuf(bufPtr)`.
- Achieve **zero heap allocations** in `res.Serialize` for responses with headers and payloads under $4\text{ KB}$.

---

### Work Package 3 (WP-3): Automated Verification & Test Suite

#### Subtask 3.1: Socket Option Verification Unit Tests (`pkg/reactor`, `pkg/server`)
- In [`pkg/reactor/reactor_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/reactor/reactor_test.go) and [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Implement `TestReactor_SocketOptions_TCPNoDelayAndKeepAlive`:
    1. Bind a real TCP listener (`net.Listen("tcp", "127.0.0.1:0")`).
    2. Connect a test client (`net.Dial("tcp", ln.Addr().String())`).
    3. Accept the socket in the reactor.
    4. Assert that `ExtractTCPConn(acceptedConn)` returns a valid non-nil `*net.TCPConn`.
    5. Verify socket options using platform socket inspection or verifying that `SetNoDelay(true)` and `SetKeepAlive(true)` were invoked without errors.
  - Implement `TestServer_ExtractTCPConn_WrappedSockets`:
    1. Test plain `*net.TCPConn`.
    2. Test `*tls.Conn` wrapping `*net.TCPConn`.
    3. Test `*connDeadlineTracker` wrapping `*net.TCPConn`.
    4. Test `*prefixConn` wrapping `*net.TCPConn`.
    5. Test multi-nested wrappers (`*connDeadlineTracker` wrapping `*prefixConn` wrapping `*tls.Conn` wrapping `*net.TCPConn`).
    6. Assert that `ExtractTCPConn` resolves to the base `*net.TCPConn` across all permutations.

#### Subtask 3.2: Immediate Small Frame Delivery Test (SSE Nagle Elimination)
- In [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Implement `TestServer_SSE_ImmediateWireDelivery_NoNagleDelay`:
    1. Start a Toron server with an SSE streaming handler pushing 5 small event frames (30 bytes each: `data: {"seq":%d}\n\n`).
    2. Frame 1 emitted immediately, frame 2 emitted 5ms later, etc.
    3. Client connects over TCP and records timestamp of receipt for each frame.
    4. Assert that inter-frame arrival intervals are consistently $\le 10\text{ms}$.
    5. Verify that no frame encounters the 40ms–200ms delayed-ACK latency stall, proving `TCP_NODELAY` is active and functioning.

#### Subtask 3.3: TCP Keep-Alive Configuration & Half-Open Socket Detection Test
- In [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Implement `TestServer_TCPKeepAlive_Configured`:
    1. Start server, establish client connection.
    2. In `handleConn`, verify that the underlying connection has keep-alive enabled.
    3. Verify that keep-alive period configuration (60 seconds) is accepted without error on supported platforms.

#### Subtask 3.4: Serialization Memory Allocation Benchmarks & Zero-Allocation Tests
- In [`pkg/httpparser/response_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response_test.go):
  - Implement `TestHttpParser_Response_ZeroAllocationSerialization`:
    ```go
    func TestHttpParser_Response_ZeroAllocationSerialization(t *testing.T) {
        res := httpparser.NewResponse()
        res.Header.Set("Content-Type", "application/json")
        res.Header.Set("Server", "toron")
        res.Header.Set("X-Request-Id", "req-test-12345")
        _, _ = res.WriteString(`{"status":"ok"}`)

        discard := io.Discard
        // Warm up pool
        _ = res.Serialize(discard)

        allocs := testing.AllocsPerRun(1000, func() {
            _ = res.Serialize(discard)
        })

        // Assert 0 allocations (or <= 1 for interface boxing if applicable)
        if allocs > 1 {
            t.Fatalf("expected <= 1 allocs/op for pooled response serialization, got %f", allocs)
        }
    }
    ```
  - Implement benchmark `BenchmarkResponse_Serialize_Pooled` in `pkg/httpparser/response_bench_test.go`.

#### Subtask 3.5: Concurrency, Thread-Safety & Race Cleanliness Validation
- Execute comprehensive race detection across all modified packages:
  ```bash
  go test -race -v ./pkg/reactor/...
  go test -race -v ./pkg/httpparser/...
  go test -race -v ./pkg/server/...
  ```
- Run concurrent stress test with 100 goroutines concurrently calling `res.Serialize` from pooled slabs to confirm complete absence of buffer cross-talk or race conditions.

---

## 3. Acceptance Criteria & Verification

### 3.1 Functional Acceptance Criteria
- [ ] **Socket Option Configuration**: `ConfigureTCPSocket` is executed on all accepted client connections in `pkg/reactor` and `pkg/server`.
- [ ] **Nagle Algorithm Disabled**: `SetNoDelay(true)` is explicitly applied to accepted TCP sockets immediately upon accept, guaranteeing zero Nagle buffering delay.
- [ ] **Keep-Alive Enabled**: `SetKeepAlive(true)` and `SetKeepAlivePeriod(60s)` are explicitly configured on accepted TCP sockets.
- [ ] **Wrapper Unwrapping**: `ExtractTCPConn` successfully unwraps `*tls.Conn`, `*connDeadlineTracker`, `*prefixConn`, and arbitrary combinations to access the underlying `*net.TCPConn`.
- [ ] **Buffer Slab Pooling**: `responseBufPool` in `pkg/httpparser/response.go` manages recycled buffer slabs, ensuring slices are reset to length 0 before returning to the pool.
- [ ] **Dual-Write Fast-Path**: `res.Serialize` writes formatted headers and payload directly to `w` without allocating monolithic concatenated buffers.
- [ ] **Streaming Compatibility**: For `res.StreamBody != nil`, headers are serialized immediately without `Content-Length`, and body writing is cleanly bypassed.

### 3.2 Non-Functional, Latency & Quality Criteria
- [ ] **Elimination of Nagle Delays**: Wire arrival latency for small SSE frames and JSON responses is $\le 2\text{ms}$, completely eliminating the 40ms–200ms delayed-ACK latency stall.
- [ ] **Zero-Allocation Serialization**: Header serialization in `res.Serialize` achieves $\le 1$ heap allocation per run (0 heap allocations for status line and standard headers).
- [ ] **Half-Open Socket Resilience**: Silent client drops during quiet streaming intervals are detected by kernel keep-alive probes, releasing server handles.
- [ ] **Zero Third-Party Dependencies**: Implementation uses exclusively standard library packages (`net`, `sync`, `time`, `bytes`, `strconv`).
- [ ] **Race Cleanliness**: All tests pass cleanly with zero warnings or errors under `go test -race ./pkg/reactor/... ./pkg/httpparser/... ./pkg/server/...`.
