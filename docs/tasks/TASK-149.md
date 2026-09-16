---
id: TASK-149
type: task
title: Implement Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-15
updated: 2026-09-15

depends_on:
  - REQ-126

derived_from:
  - REQ-126

implements:
  - REQ-126

verified_by:
  - TC-126

decided_by:
  - ADR-126

related_to:
  - TASK-004
  - ADR-083
  - TC-087
  - TC-088
  - TASK-148
  - REQ-125
---

# TASK-149 - Implement Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts

## 1. Overview & Objective

During high-concurrency benchmarks under [`REQ-121`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-121.md), Toron processes ~24.5k requests per second per container. In the server connection loop ([`pkg/server/server.go:199`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L199) and [line 352](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L352)), Toron executes two operating system socket system calls on every HTTP transaction within persistent keep-alive connections:
```go
conn.SetReadDeadline(time.Now().Add(timeout))
...
conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
```
At 24.5k RPS, the engine executes **~49,000 socket deadline system calls per second**, causing repeated kernel context switches, ring-3 to ring-0 transitions, and CPU instruction and data cache evictions.

Furthermore, architectural analysis of persistent streaming connections (such as Server-Sent Events / SSE, telemetry feeds, chunked transfers, or direct streaming fast-paths per [`REQ-125`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md) and [`TASK-148`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-148.md)) revealed a critical tension:
1. Setting a static write deadline (e.g. 5 seconds into the future) prior to emitting response headers terminates healthy, long-lived streams prematurely when their lifetime exceeds `WriteTimeout`.
2. Conversely, completely eliminating `SetWriteDeadline` opens Toron to **Slow Read Denial of Service ([CWE-400](https://cwe.mitre.org/data/definitions/400.html))**, directly violating [`REQ-005`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-005.md) §2, [`TASK-004`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-004.md) §3, [`ADR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-083.md), and security verification suites [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md) / [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md). A malicious or stalled client advertising a zero TCP window or reading at 1 byte/minute would pin worker goroutines and socket file descriptors indefinitely, inducing thread pool exhaustion.
3. Completely eliminating `SetReadDeadline` exposes persistent keep-alive sockets to classic Slowloris socket starvation.

The objective of this task is to implement the safe, non-conflicting path approved in [`REQ-126`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-126.md):
1. **Adaptive Deadline Amortization (`pkg/server`)**: Wrap persistent client connections in a lightweight tracker (`connDeadlineTracker`) that caches the last applied read and write deadlines. During rapid request bursts, if an active deadline exists with more than half the configured timeout remaining ($> \text{timeout}/2$), redundant `SetReadDeadline` and `SetWriteDeadline` system calls are bypassed safely.
2. **Idle-State Transition Alignment (`pkg/server`)**: When an HTTP transaction finishes and the connection becomes idle waiting for the next request header (`br.Buffered() == 0`), Toron immediately applies an explicit `IdleTimeout` deadline and resets the amortization state, preserving 100% of Slowloris security defenses.
3. **Activity-Refreshed Streaming Write Deadlines (`pkg/server`)**: In the persistent streaming loop (`res.StreamBody`) and bidirectional relay routines (`relayStreams`), refresh socket write deadlines upon each successfully transferred and flushed chunk. This allows healthy streams to run indefinitely (minutes or hours) while terminating slow-reading or stalled clients within `WriteTimeout`.
4. **Zero-Timeout Configuration Support (`pkg/config`, `pkg/server`)**: Ensure operators in isolated or benchmark environments can configure `read_timeout: 0` and `write_timeout: 0` for zero deadline syscalls without validation errors or unintended defaults.
5. **Automated Verification & Regression Protection**: Prove $>80\%$ syscall reduction under rapid request sequences, verify that streaming responses survive beyond 10 seconds under a 5-second `WriteTimeout`, confirm that slow-read clients are terminated within `WriteTimeout`, and verify zero regression across TC-087 and TC-088.

---

## 2. Work Breakdown Structure (WBS)

```
TASK-149: Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts
├── WP-1: Amortized Connection Deadline Tracker (pkg/server)
│   ├── Subtask 1.1: Tracker Data Structure & Lifetime Wrapper (connDeadlineTracker)
│   ├── Subtask 1.2: Read & Write Deadline Amortization Heuristic
│   └── Subtask 1.3: Strict Idle Transition & Loop Integration in handleConnection
├── WP-2: Activity-Refreshed Streaming Write Deadlines (pkg/server)
│   ├── Subtask 2.1: Streaming Response Chunk Write Deadline Refresh (res.StreamBody)
│   ├── Subtask 2.2: Slow-Read DoS Defense (CWE-400) & Stalled Connection Teardown
│   └── Subtask 2.3: Activity-Refreshed Relaying in Bidirectional Streams (relayStreams)
├── WP-3: Zero-Timeout Configuration & Environment Support (pkg/config, pkg/server)
│   ├── Subtask 3.1: Zero-Timeout Engine Handling (pkg/server)
│   └── Subtask 3.2: Configuration Schema Validation & Invariants (pkg/config)
└── WP-4: Automated Verification & Regression Test Suite
    ├── Subtask 4.1: Deadline Amortization & Syscall Reduction Verification
    ├── Subtask 4.2: Long-Lived Streaming Survival Test (> 10s under 5s timeout)
    ├── Subtask 4.3: Slow-Read Client Termination Security Test (CWE-400)
    ├── Subtask 4.4: Idle Timeout State Machine Alignment Test
    └── Subtask 4.5: Zero-Regression Verification of TC-087 and TC-088
```

---

### Work Package 1 (WP-1): Amortized Connection Deadline Tracker (`pkg/server`)

#### Subtask 1.1: Tracker Data Structure & Lifetime Wrapper (`connDeadlineTracker`)
- In [`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go) (or a new dedicated file [`pkg/server/deadline.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/deadline.go)), implement a connection wrapper struct embedding `net.Conn`:
  ```go
  type connDeadlineTracker struct {
      net.Conn
      lastReadDeadline   time.Time
      lastWriteDeadline  time.Time
      lastReadTimeout    time.Duration
      lastWriteTimeout   time.Duration
      mu                 sync.Mutex // guards concurrent deadline access
      syscallCountRead   atomic.Uint64 // telemetry / test verification hook
      syscallCountWrite  atomic.Uint64 // telemetry / test verification hook
  }
  ```
- Implement factory constructor:
  ```go
  func newConnDeadlineTracker(conn net.Conn) *connDeadlineTracker {
      return &connDeadlineTracker{Conn: conn}
  }
  ```
- Implement unwrapping helper `Unwrap() net.Conn` and ensure standard `net.Conn` interfaces (including `CloseWrite()` or `syscall.Conn` if supported by underlying socket) are cleanly accessible or forwarded.

#### Subtask 1.2: Read & Write Deadline Amortization Heuristic
- In `connDeadlineTracker`, implement `SetAmortizedReadDeadline(timeout time.Duration) error`:
  1. If `timeout <= 0`:
     - If `!t.lastReadDeadline.IsZero()`, clear deadline on underlying socket: `err := t.Conn.SetReadDeadline(time.Time{})`.
     - Reset `t.lastReadDeadline = time.Time{}` and `t.lastReadTimeout = 0`.
     - Return `err`.
  2. Evaluate amortization condition:
     ```go
     now := time.Now()
     if !t.lastReadDeadline.IsZero() && timeout == t.lastReadTimeout {
         remaining := t.lastReadDeadline.Sub(now)
         if remaining > timeout/2 {
             // Redundant syscall bypassed: current socket deadline remains valid
             return nil
         }
     }
     ```
  3. If amortization condition is not met (first call, timeout changed, or remaining deadline $\le \text{timeout}/2$):
     - Compute new absolute deadline: `deadline := now.Add(timeout)`.
     - Invoke underlying socket syscall: `err := t.Conn.SetReadDeadline(deadline)`.
     - If `err == nil`:
       - Record `t.lastReadDeadline = deadline`.
       - Record `t.lastReadTimeout = timeout`.
       - Increment `t.syscallCountRead`.
     - Return `err`.
- Implement identical amortization logic in `SetAmortizedWriteDeadline(timeout time.Duration) error`:
  1. If `timeout <= 0`: clear write deadline if set, reset `t.lastWriteDeadline = time.Time{}` and `t.lastWriteTimeout = 0`, return nil.
  2. If `!t.lastWriteDeadline.IsZero() && timeout == t.lastWriteTimeout && t.lastWriteDeadline.Sub(now) > timeout/2`:
     - Return `nil` (syscall bypassed).
  3. Otherwise:
     - `deadline := now.Add(timeout)`.
     - `err := t.Conn.SetWriteDeadline(deadline)`.
     - If `err == nil`:
       - Record `t.lastWriteDeadline = deadline`.
       - Record `t.lastWriteTimeout = timeout`.
       - Increment `t.syscallCountWrite`.
     - Return `err`.

#### Subtask 1.3: Strict Idle Transition & Loop Integration in `handleConnection`
- Implement explicit override methods to force immediate deadline application without amortization:
  ```go
  func (t *connDeadlineTracker) ForceSetReadDeadline(deadline time.Time) error
  func (t *connDeadlineTracker) ResetReadAmortization()
  ```
- In [`pkg/server/server.go:handleConnection`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L185-L367):
  1. Wrap accepted client connection:
     ```go
     tracker := newConnDeadlineTracker(conn)
     conn = tracker
     br := bufio.NewReader(conn)
     ```
  2. In the request processing loop:
     - For persistent connections waiting for the next request:
       ```go
       if !firstRequest && br.Buffered() == 0 && s.config.IdleTimeout > 0 {
           // Transition to idle state: strictly enforce IdleTimeout immediately
           tracker.ResetReadAmortization()
           _ = tracker.ForceSetReadDeadline(time.Now().Add(s.config.IdleTimeout))
       } else if s.config.ReadTimeout > 0 {
           // Active parsing or pipelined requests: apply amortized ReadTimeout
           _ = tracker.SetAmortizedReadDeadline(s.config.ReadTimeout)
       }
       ```
     - For discrete static responses (non-streaming), replace static `conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))` with:
       ```go
       if s.config.WriteTimeout > 0 {
           _ = tracker.SetAmortizedWriteDeadline(s.config.WriteTimeout)
       }
       ```
  3. When an idle timeout triggers (`errors.As(err, &netErr) && netErr.Timeout() && !firstRequest`), exit loop silently as previously established, closing the idle socket cleanly.

---

### Work Package 2 (WP-2): Activity-Refreshed Streaming Write Deadlines (`pkg/server`)

#### Subtask 2.1: Streaming Response Chunk Write Deadline Refresh (`res.StreamBody`)
- In [`pkg/server/server.go:handleConnection`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L311-L349):
  1. Ensure initial stream headers are guarded:
     ```go
     if s.config.WriteTimeout > 0 {
         _ = tracker.SetAmortizedWriteDeadline(s.config.WriteTimeout)
     }
     if err := res.Serialize(conn); err != nil {
         _ = req.CloseBody()
         return fmt.Errorf("server: failed to write stream headers: %w", err)
     }
     ```
  2. In the streaming chunk relay loop:
     - On each chunk read from `res.StreamBody`:
       ```go
       for {
           n, readErr := res.StreamBody.Read(buf)
           if n > 0 {
               if s.config.WriteTimeout > 0 {
                   // Refresh write deadline for this specific chunk transmission
                   _ = tracker.ForceSetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
               }
               if _, writeErr := conn.Write(buf[:n]); writeErr != nil {
                   _ = req.CloseBody()
                   return nil
               }
           }
           if readErr != nil {
               _ = req.CloseBody()
               return nil
           }
       }
       ```
  3. This ensures that persistent streams (such as SSE feeds or live audio/data relays) transmitting chunks every 1–2 seconds can remain active for days without being killed by a static 5-second `WriteTimeout`.

#### Subtask 2.2: Slow-Read DoS Defense ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)) & Stalled Connection Teardown
- Verify that if a client stops reading, advertises a zero TCP receive window, or throttles reading below what is required to consume chunk `buf[:n]` within `s.config.WriteTimeout`:
  1. The socket write buffer fills and `conn.Write(buf[:n])` blocks.
  2. The refreshed write deadline `time.Now().Add(s.config.WriteTimeout)` expires in the kernel TCP stack.
  3. `conn.Write` returns an `os.ErrDeadlineExceeded` / `net.Error` with `Timeout() == true`.
  4. Toron captures this write error, terminates the relay loop, closes `res.StreamBody`, closes client connection, and exits `handleConnection`.
  5. The worker goroutine terminates immediately, freeing file descriptors and worker pool capacity.

#### Subtask 2.3: Activity-Refreshed Relaying in Bidirectional Streams (`relayStreams`)
- In [`pkg/server/server.go:relayStreams`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L683-L757):
  - Ensure connections passed to `relayStreams` utilize amortized deadline tracking:
    - If `conn1` or `conn2` is not already a `*connDeadlineTracker`, wrap in `newConnDeadlineTracker`.
    - In `copyDirection`:
      ```go
      for {
          _ = srcTracker.SetAmortizedReadDeadline(idleTimeout)
          n, readErr := src.Read(buf)
          if n > 0 {
              _ = dstTracker.SetAmortizedWriteDeadline(idleTimeout)
              if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
                  ...
              }
          }
      ```
  - Eliminates thousands of redundant `SetReadDeadline` and `SetWriteDeadline` syscalls per second on high-throughput WebSocket / TCP tunnel connections while preserving idle termination if both directions fall silent for longer than `idleTimeout`.

---

### Work Package 3 (WP-3): Zero-Timeout Configuration & Environment Support (`pkg/config`, `pkg/server`)

#### Subtask 3.1: Zero-Timeout Engine Handling (`pkg/server`)
- Ensure that if `s.config.ReadTimeout == 0`, `s.config.WriteTimeout == 0`, or `s.config.IdleTimeout == 0`:
  1. `connDeadlineTracker` treats `0` as an explicit directive to disable socket deadlines (infinite / unbounded).
  2. If a deadline was previously applied and a 0-duration timeout is specified, `tracker` clears the deadline via `conn.SetDeadline(time.Time{})`.
  3. Under steady-state request processing with `read_timeout: 0` and `write_timeout: 0`, exactly **zero socket deadline syscalls** are executed.

#### Subtask 3.2: Configuration Schema Validation & Invariants (`pkg/config`)
- In [`pkg/config/loader.go:ValidateConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L175-L225):
  - Ensure explicit non-negative checks for all timeout fields:
    ```go
    if cfg.Server.ReadTimeout < 0 {
        return fmt.Errorf("server.read_timeout must be non-negative, got %v", cfg.Server.ReadTimeout)
    }
    if cfg.Server.WriteTimeout < 0 {
        return fmt.Errorf("server.write_timeout must be non-negative, got %v", cfg.Server.WriteTimeout)
    }
    if cfg.Server.IdleTimeout < 0 {
        return fmt.Errorf("server.idle_timeout must be non-negative, got %v", cfg.Server.IdleTimeout)
    }
    ```
  - Verify that `validateConfigDefaults` does NOT overwrite user-specified `read_timeout: 0` or `write_timeout: 0` with default values (only negative values are rejected, `0` is preserved).

---

### Work Package 4 (WP-4): Automated Verification & Regression Test Suite

#### Subtask 4.1: Deadline Amortization & Syscall Reduction Verification
- In [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Implement `TestServer_DeadlineAmortization_SyscallReduction(t *testing.T)`:
    1. Create mock connection wrapper tracking exact underlying invocations of `SetReadDeadline` and `SetWriteDeadline`.
    2. Execute 1,000 rapid HTTP/1.1 requests over a single persistent keep-alive connection with `ReadTimeout = 5s` and `WriteTimeout = 5s` (all requests completing within 500ms).
    3. Verify that without amortization, 1,000 requests would trigger 2,000 deadline syscalls.
    4. Assert that with amortization active:
       - Number of `SetReadDeadline` syscalls is $\le 2$.
       - Number of `SetWriteDeadline` syscalls is $\le 2$.
       - Syscall reduction exceeds **$99\%$** under burst keep-alive traffic, easily beating the $> 80\%$ requirement.

#### Subtask 4.2: Long-Lived Streaming Survival Test ($> 10\text{s}$ under $5\text{s}$ timeout)
- In [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Implement `TestServer_Streaming_SurvivesPastWriteTimeout(t *testing.T)`:
    1. Configure Toron server with `WriteTimeout = 3 * time.Second` (or `5s`).
    2. Register a streaming handler (`res.StreamBody`) that produces 8 chunks, each emitted after a 1.5-second sleep (total duration: $8 \times 1.5\text{s} = 12\text{s}$).
    3. Client connects and consumes chunks as they arrive.
    4. Assert that all 8 chunks are received successfully across the 12-second stream.
    5. Verify that the stream was NOT terminated at $T = 3\text{s}$ or $T = 5\text{s}$, proving activity-refreshed write deadlines allow indefinite continuous streaming.

#### Subtask 4.3: Slow-Read Client Termination Security Test ([CWE-400](https://cwe.mitre.org/data/definitions/400.html))
- In [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Implement `TestServer_Streaming_SlowReadClientTerminated(t *testing.T)`:
    1. Configure Toron server with `WriteTimeout = 300 * time.Millisecond`.
    2. Register a streaming handler generating infinite or large payloads (e.g. 10 MB in 64 KB chunks).
    3. Client connects, reads the initial response headers and first 64 KB chunk, and then completely stops reading from the TCP socket.
    4. Server attempts to write subsequent chunks until the socket send buffer saturates.
    5. Measure time until server terminates the connection: assert that termination occurs within $300\text{ms} \pm 100\text{ms}$ after the socket buffer fills.
    6. Verify that the server goroutine exits cleanly and does not leak.

#### Subtask 4.4: Idle Timeout State Machine Alignment Test
- In [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Implement `TestServer_IdleTimeout_StrictEnforcement(t *testing.T)`:
    1. Configure Toron server with `ReadTimeout = 10 * time.Second` and `IdleTimeout = 200 * time.Millisecond`.
    2. Client sends a fast HTTP request and reads the response.
    3. Client ceases transmission, leaving connection idle.
    4. Assert that the server closes the connection after $200\text{ms} \pm 50\text{ms}$.
    5. Proves that the earlier 10-second `ReadTimeout` was NOT amortized into the idle state; `IdleTimeout` was strictly and immediately enforced.

#### Subtask 4.5: Zero-Regression Verification of TC-087 and TC-088
- Execute full test suite across proxy and server packages:
  ```bash
  go test -v ./pkg/proxy/...
  go test -v ./pkg/server/...
  go test -v -race ./pkg/server/... ./pkg/proxy/... ./pkg/config/...
  ```
- Confirm that [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md) (Layer 4 TCP/UDP idle timeouts, concurrency limits) and [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md) (upgraded WebSocket idle timeouts, peer disconnects, half-close propagation) pass with 100% success.

---

## 3. Acceptance Criteria & Verification

### 3.1 Functional Acceptance Criteria
- [ ] **Connection Deadline Tracker**: `pkg/server` incorporates `connDeadlineTracker` wrapping persistent client connections and maintaining `lastReadDeadline` and `lastWriteDeadline`.
- [ ] **Syscall Amortization**: When an active deadline has $> \text{timeout}/2$ remaining, redundant calls to `SetReadDeadline` and `SetWriteDeadline` are bypassed without operating system calls.
- [ ] **Strict Idle Transition**: When an HTTP transaction finishes and `br.Buffered() == 0`, Toron immediately applies an explicit `IdleTimeout` deadline and resets the amortization state.
- [ ] **Activity-Refreshed Streaming**: In `res.StreamBody` chunk transfers, write deadlines are refreshed per chunk (`now.Add(WriteTimeout)`), allowing healthy streams to persist indefinitely.
- [ ] **Slow-Read DoS Defense**: A client stalling or refusing to read chunked data triggers write timeout expiration within `WriteTimeout`, terminating the socket and unblocking server workers.
- [ ] **Zero-Timeout Support**: Setting `read_timeout: 0` and `write_timeout: 0` in configuration disables deadline system calls entirely with zero overhead.

### 3.2 Security, Performance & Non-Functional Criteria
- [ ] **Syscall Reduction**: Reduces socket deadline syscalls by $> 80\%$ under keep-alive saturation workloads ($> 20\text{k RPS}$).
- [ ] **Slowloris Immunity**: Full compliance with [`REQ-005`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-005.md) §2 and [`TASK-004`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-004.md) §3 is preserved; slow headers and idle sockets are terminated deterministically.
- [ ] **Zero Regressions**: Existing test suites [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md) and [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md) pass without failure.
- [ ] **Race & Thread Safety**: All tests pass cleanly under `go test -race ./pkg/server/... ./pkg/proxy/... ./pkg/config/...`.
