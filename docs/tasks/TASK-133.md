---
id: TASK-133
type: task
title: Implementation of Bounded Memory Buffer Optimization via sync.Pool
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-110

derived_from:
  - REQ-110
  - SRC-05

implements:
  - REQ-110

verified_by:
  - TC-110

decided_by:
  - ADR-110

related_to:
  - REQ-110
  - ADR-110
  - TC-110
  - CR-106
  - SR-110
---

# TASK-133 - Implementation of Bounded Memory Buffer Optimization via sync.Pool

## 1. Description

Decompose and implement memory buffer recycling using `sync.Pool` across header line reading and request body ingestion (`SRC-05`, `REQ-110`), eliminating multi-megabyte heap churn and reducing Garbage Collection pause latencies.

## 2. Subtask Breakdown

### TASK-133.1: Header Line Buffer Pooling
- **Component**: `pkg/httpparser/parser.go`
- **Scope**:
  - Define `lineBufferPool sync.Pool` returning `*bytes.Buffer` with initial 1 KB capacity.
  - In `readLineBounded`, acquire buffer from `lineBufferPool`, reset before use, and return via `defer lineBufferPool.Put(buf)`.
  - Replace `strings.Builder` with pooled buffer to eliminate per-header line slice allocations.

### TASK-133.2: Tiered Request Body Buffer Pooling
- **Component**: `pkg/httpparser/parser.go`
- **Scope**:
  - Define `maxPooledBodySize = 64 * 1024` (64 KB).
  - Define `bodyBufferPool sync.Pool` allocating 64 KB slices (`*[]byte`).
  - In `ParseRequest`, for payloads $0 < \text{clInt} \le 64\text{ KB}$, acquire from `bodyBufferPool` and read body payload via `io.ReadFull`.
  - For oversized payloads ($64\text{ KB} < \text{clInt} \le \text{opts.MaxBodyBytes}$), allocate a one-off slice `make([]byte, clInt)` without polluting `bodyBufferPool`.

### TASK-133.3: Idempotent `pooledBodyReader`
- **Component**: `pkg/httpparser/parser.go`
- **Scope**:
  - Implement `pooledBodyReader` satisfying `io.ReadCloser`.
  - Delegate `Read` to internal `*bytes.Reader`.
  - Implement idempotent `Close()` guarded by `sync.Once` to return `bufPtr` to `bodyBufferPool` exactly once and prevent use-after-free.

### TASK-133.4: Request Body Lifecycle & Server Integration
- **Components**: `pkg/httpparser/request.go`, `pkg/server/server.go`
- **Scope**:
  - Add `func (r *Request) CloseBody() error` to `*httpparser.Request`.
  - In `pkg/server/server.go:handleConn`, call `req.CloseBody()` at the conclusion of each persistent request cycle (post router dispatch and response serialization).

### TASK-133.5: Unit, Concurrency, and Microbenchmark Verification
- **Components**: `pkg/httpparser/parser_test.go`, `pkg/server/server_test.go`
- **Scope**:
  - Implement tests covering:
    - Pooled body reading and buffer reuse (`TC-110-01`).
    - Large body fallback exceeding 64 KB (`TC-110-02`).
    - Idempotent `Close()` double-free safety (`TC-110-03`).
    - Multi-goroutine concurrent pool race safety (`TC-110-04`).
    - Allocation reduction microbenchmarks `BenchmarkParseRequest_PooledBody` and `BenchmarkReadLineBounded_Pooled` (`TC-110-05`).

## 3. Acceptance Criteria

- Full test suite passes under `go test -v -race -count=1 ./...`.
- Measurable reduction in memory allocations for header line reading and request body ingestion.
- Zero external third-party dependencies.
