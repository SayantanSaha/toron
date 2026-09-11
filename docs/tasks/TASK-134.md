---
id: TASK-134
type: task
title: Implementation of Bounded Worker Pool Connection Dispatching in Reactor
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-111

derived_from:
  - REQ-111
  - SRC-03

implements:
  - REQ-111

verified_by:
  - TC-111

decided_by:
  - ADR-111

related_to:
  - REQ-111
  - ADR-111
  - TC-111
  - CR-107
  - SR-111
---

# TASK-134 - Implementation of Bounded Worker Pool Connection Dispatching in Reactor

## 1. Description

Decompose and implement bounded worker pool connection dispatching and queue management in `pkg/reactor/reactor.go` (`SRC-03`, `REQ-111`), eliminating unbounded per-connection goroutine spawning, strictly enforcing `WorkerPoolSize`, and guaranteeing deterministic resource consumption under connection floods.

## 2. Subtask Breakdown

### TASK-134.1: Reactor Config and Bounded Task Queue Initialization
- **Component**: `pkg/reactor/reactor.go`
- **Scope**:
  - Add `MaxQueueSize int` to `reactor.Config` (defaulting to `WorkerPoolSize * 4` in `New()` if $\le 0$).
  - Add task channel `tasks chan net.Conn` and active worker counter `activeWorkers atomic.Int64` to `Reactor` struct.
  - In `New(cfg Config, handler Handler)`, validate `WorkerPoolSize` (defaulting to 64 if $\le 0$) and initialize `tasks` channel with capacity `cfg.MaxQueueSize`.

### TASK-134.2: Worker Pool Loop & Concurrency Bounding
- **Component**: `pkg/reactor/reactor.go`
- **Scope**:
  - Implement `workerLoop()` method in `Reactor`:
    - Continuously receive connections from `r.tasks`.
    - Atomically increment `activeWorkers` on job acquisition and decrement upon connection completion.
    - Execute `r.processConn(conn)` and untrack connection via `r.trackConn(conn, false)`.
    - Terminate when `r.tasks` channel is closed.
  - In `Serve(ln net.Listener)`, start exactly `r.config.WorkerPoolSize` worker goroutines tracked by `r.workerWg sync.WaitGroup`.

### TASK-134.3: Deterministic Connection Ingress Dispatch & Backpressure
- **Component**: `pkg/reactor/reactor.go`
- **Scope**:
  - In `Serve(ln net.Listener)`, for each connection accepted from `ln.Accept()`:
    - Check shutdown status (`r.isShutdown.Load()`), closing socket if shut down.
    - Register connection in `r.conns` via `r.trackConn(conn, true)`.
    - Dispatch to `r.tasks` via `select`:
      ```go
      select {
      case r.tasks <- conn:
      case <-r.ctx.Done():
          r.trackConn(conn, false)
          _ = conn.Close()
          return ErrServerClosed
      }
      ```
    - Apply backpressure to `ln.Accept()` when the queue fills, naturally utilizing TCP backlog flow control.

### TASK-134.4: Safe Teardown & Graceful Shutdown Synchronization
- **Component**: `pkg/reactor/reactor.go`
- **Scope**:
  - In `Serve(ln net.Listener)`, run `defer func() { close(r.tasks); r.workerWg.Wait() }()`.
  - In `Shutdown(ctx context.Context)`:
    - Set shutdown atomic flag and cancel `r.ctx`.
    - Close `r.listener` to abort blocking `Accept()`.
    - Close all active sockets in `r.conns` under `connsMu` to immediately unblock workers blocked on network I/O.
    - Wait for workers to finish draining closed tasks or abort if timeout context expires.

### TASK-134.5: Observability APIs
- **Component**: `pkg/reactor/reactor.go`
- **Scope**:
  - Implement `func (r *Reactor) ActiveWorkers() int` returning current count of workers executing `processConn`.
  - Implement `func (r *Reactor) QueueLen() int` returning pending task channel length `len(r.tasks)`.

### TASK-134.6: Concurrency and Regression Unit Testing
- **Component**: `pkg/reactor/reactor_test.go`, `pkg/reactor/reactor_bench_test.go`
- **Scope**:
  - Implement `TestReactor_BoundedWorkerPoolEnforcement` asserting `ActiveWorkers() <= WorkerPoolSize`.
  - Implement `TestReactor_WorkerPoolQueueBackpressure` testing sequential queue consumption.
  - Implement `TestReactor_GracefulShutdownDrainsQueue` testing zero socket leaks during shutdown.
  - Update `TestReactor_KeepAliveConcurrencyAndShutdown` configuring worker pool appropriate for 20+ persistent connections.
  - Update `BenchmarkReactor_ConnectionDispatch` measuring bounded channel dispatch throughput.

## 3. Acceptance Criteria

- Exactly `WorkerPoolSize` worker goroutines handle incoming connections.
- Zero unbounded goroutine spawning in `Serve()`.
- Queue backpressure throttles connections when worker capacity is saturated.
- `ActiveWorkers()` never exceeds `WorkerPoolSize` under any load.
- All tests pass with `go test -race -count=1 ./...`.
