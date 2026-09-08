---
id: TASK-008
type: task
title: Benchmark Suite Implementation for Core Server Packages
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-09-08

depends_on:
  - TASK-001
  - TASK-002
  - TASK-003
  - TASK-005

derived_from:
  - REQ-008

implements:
  - REQ-008

verified_by: []

decided_by: []

related_to: []
---

# TASK-008 - Benchmark Suite Implementation for Core Server Packages

## Description

Write native Go benchmark tests (`*_bench_test.go` or `*_test.go`) in `pkg/reactor`, `pkg/httpparser`, `pkg/router`, and `pkg/server` using `testing.B` and `b.ReportAllocs()`.

## Acceptance Criteria

- `BenchmarkParseRequest_GET` & `BenchmarkParseRequest_POST` in `pkg/httpparser`.
- `BenchmarkResponse_Serialize` in `pkg/httpparser`.
- `BenchmarkRouter_Match` & `BenchmarkRouter_MiddlewareChain` in `pkg/router`.
- `BenchmarkReactor_BufferPool` & `BenchmarkReactor_ConcurrentConns` in `pkg/reactor`.
- `BenchmarkServer_EndToEnd` in `pkg/server`.
- All benchmarks run cleanly via `go test -bench=. -benchmem ./...`.

## Rationale

Provides measurable performance regression metrics for developers.

## Constraints

- Idiomatic Go benchmark practices (`b.ResetTimer()`, `b.ReportAllocs()`).

## Open Questions

- None.
