---
title: Native Performance Benchmarking
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-008
  - TASK-008

derived_from:
  - REQ-008
  - ADR-003

documents:
  - BENCHMARKING-FEATURE

related_to:
  - index.md
  - features/event-reactor.md
---

# Native Performance Benchmarking

## Overview

Toron includes a native Go benchmarking suite integrated into all core packages using Go's standard `testing.B` infrastructure.

## Running Benchmarks

Run benchmarks across individual packages or the entire project using the standard Go test command:

### Run HTTP Parser Benchmarks
```bash
go test -bench . -benchmem ./pkg/httpparser
```

### Run Router Benchmarks
```bash
go test -bench . -benchmem ./pkg/router
```

### Run Reactor Benchmarks
```bash
go test -bench . -benchmem ./pkg/reactor
```

### Run Server End-to-End Benchmarks
```bash
go test -bench . -benchmem ./pkg/server
```

## Baseline Performance Results

| Benchmark Target | Speed (`ns/op`) | Memory (`B/op`) | Allocations (`allocs/op`) |
| ---------------- | --------------- | --------------- | ------------------------- |
| `BufferPool` (Reactor) | **8.29 ns** | **0 B** | **0 allocs** |
| `MatchExact` (Router) | **16.30 ns** | **0 B** | **0 allocs** |
| `MiddlewareChain` (Router) | **61.99 ns** | **48 B** | **3 allocs** |
| `Serialize` (Response) | **638.5 ns** | **480 B** | **11 allocs** |
| `ParseRequest_GET` (Parser) | **2.05 µs** | **5.8 KB** | **33 allocs** |
| `ParseRequest_POST` (Parser) | **1.86 µs** | **5.4 KB** | **32 allocs** |

## Related Pages

- [Event Reactor Core](./event-reactor.md)
- [Release Notes](../release-notes.md)
