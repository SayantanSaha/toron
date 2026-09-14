---
id: TASK-145
type: task
title: Reverse Proxy Upstream Connection Pooling, Transport Optimization, and Serialization Pipeline Implementation
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-14
updated: 2026-09-14

depends_on:
  - TASK-144

derived_from:
  - REQ-122

implements:
  - REQ-122

decided_by:
  - ADR-122

verified_by:
  - TC-122

related_to:
  - REQ-122
  - ADR-122
  - TC-122
  - CR-118
  - SR-122
---

# TASK-145 - Reverse Proxy Upstream Connection Pooling, Transport Optimization, and Serialization Pipeline Implementation

## 1. Description & Context
Implement engineering changes across `pkg/proxy/proxy.go` and `pkg/httpparser/response.go` to eliminate upstream connection churn, environment lookup locks, unnecessary gzip decompression, and serialization heap allocations, as formally specified in [`REQ-122`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-122.md).

---

## 2. Work Packages & Subtasks

### TASK-145.1 (WP-1): Upstream Connection Pooling & Transport Tuning in `pkg/proxy/proxy.go`
- Configure `http.Transport` in `NewProxyWithOptions`:
  - `MaxIdleConns: 10000`
  - `MaxIdleConnsPerHost: 1000`
  - `MaxConnsPerHost: 0`
  - `DisableCompression: true`
  - `WriteBufferSize: 64 * 1024`
  - `ReadBufferSize: 64 * 1024`
  - Direct proxy dialing (`Proxy: nil` unless explicitly specified).

### TASK-145.2 (WP-2): Hop-by-Hop Upstream Header Filtering
- In `ServeHTTPWithPrefix`, filter `hopByHopHeaders` when copying upstream headers into `res.Header` so upstream `Connection: close` does not terminate downstream client keepalives.

### TASK-145.3 (WP-3): Zero-Allocation Response Serialization Fast-Path
- In `pkg/httpparser/response.go`, optimize `Serialize`:
  - Replace `fmt.Fprintf` with `buf.WriteString`.
  - Add fast-path index check in `sanitizeHeader` to avoid string allocations for clean headers.

### TASK-145.4 (WP-4): Rebuild, Benchmark Verification & Retesting
- Rebuild Toron Docker image.
- Re-run benchmark suite and verify improved throughput and reduced latency.

---

## 3. Acceptance Criteria
- [ ] Unit tests pass with `-race`.
- [ ] P50 latency on `fast` and `go` backends drops to $\le 1\,\text{ms}$.
- [ ] Throughput increases significantly.
