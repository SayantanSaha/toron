---
id: TASK-128
type: task
title: Implement In-Process HTTP Parser Microbenchmarks for Fail-Fast Rejection Latency
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-105

owns:
  - pkg/httpparser/parser_test.go

references:
  - REQ-105
  - TST-05
  - AER-002
  - PDR-002
  - ADR-105
  - TC-105

derived_from:
  - REQ-105
  - TST-05

implements:
  - REQ-105

verified_by:
  - TC-105

decided_by:
  - ADR-105

related_to:
  - REQ-105
  - ADR-105
  - TC-105
---

# TASK-128 - Implement In-Process HTTP Parser Microbenchmarks for Fail-Fast Rejection Latency

## 1. Description

Implement in-process Go parser microbenchmarks (`testing.B`) in [`pkg/httpparser/parser_test.go`](pkg/httpparser/parser_test.go) to isolate state machine execution latency and memory allocations from transport-layer TCP network overhead, in compliance with [`REQ-105`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-105.md), [`ADR-105`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-105.md), and [`TC-105`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-105.md).

## 2. Work Breakdown

### Task 128.1: Benchmark Suite Payload & Error Oracle Definition
- Prepare pre-allocated byte slices for:
  - **RFC 7230 §3.2.4 Whitespace Rejection**: `GET / HTTP/1.1\r\nHost : example.com\r\nUser-Agent: toron-bench\r\n\r\n` (asserts `ErrBadRequest`).
  - **RFC 7230 §3.3.2 Conflicting Multiple Content-Length**: `POST /api/v1/submit HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nContent-Length: 10\r\n\r\nhello` (asserts `ErrBadRequest`).
  - **Valid RFC 7230 HTTP/1.1 Baseline**: `GET /api/v1/resource HTTP/1.1\r\nHost: example.com\r\nUser-Agent: toron-bench\r\nAccept: application/json\r\n\r\n` (asserts successful parse).

### Task 128.2: Zero-Allocation Reader Resetting Engine
- Pre-allocate `bytes.Reader` outside `b.ResetTimer()`.
- Invoke `r.Reset(payload)` on each iteration within `for i := 0; i < b.N; i++`.
- Enable memory allocation tracking via `b.ReportAllocs()`.

### Task 128.3: Verification & Profiling
- Execute `go test -v -race -count=1 ./pkg/httpparser/...` to verify race safety.
- Execute `go test -bench=BenchmarkParseRequest -benchmem ./pkg/httpparser/...` to profile nanoseconds per operation and bytes allocated per operation.
