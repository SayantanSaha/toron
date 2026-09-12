---
id: TASK-137
type: task
title: High-Concurrency Saturation Stress Testing with Background Traffic (BMK-04)
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-114

derived_from:
  - REQ-114
  - BMK-04
  - AER-002
  - MSR-002

implements:
  - REQ-114

verified_by:
  - TC-114

decided_by:
  - ADR-114

related_to:
  - REQ-114
  - ADR-114
  - TC-114
  - CR-110
  - SR-114
---

# TASK-137 - High-Concurrency Saturation Stress Testing with Background Traffic (BMK-04)

## 1. Description

Decompose and implement the High-Concurrency Saturation Stress Testing Harness (`BMK-04`, `REQ-114`) in `benchmarks/wrk2/loadgen.go`. This harness addresses peer review critiques (`AER-002` lines 288–291, `MSR-002` lines 239–245 & 298–301) by evaluating Toron under sustained saturation (5,000+ RPS) while concurrently interleaving protocol attack vectors, measuring wire-rate throughput, high-order tail latency percentiles ($p95, p99, p99.9$), and proving zero-starvation for legitimate traffic.

---

## 2. Subtask Breakdown

### TASK-137.1: Load Generator CLI & Rate Pacing Extensions
- **Component**: `benchmarks/wrk2/loadgen.go`
- **Scope**:
  - Add CLI flags:
    - `-attack-ratio <float>`: Fractional ratio of adversarial traffic (default: `0.10` = 10%).
    - `-md <string>`: Path to output Markdown report (`benchmarks/results/saturation_stress_report.md`).
    - `-json <string>`: Path to output JSON report (`benchmarks/results/saturation_stress_report.json`).
  - Maintain high-precision rate limiter channel for coordinated-omission-free pacing at 5,000+ RPS.

### TASK-137.2: Adversarial Attack Catalog Implementation
- **Component**: `benchmarks/wrk2/loadgen.go`
- **Scope**:
  - Implement a thread-safe catalog of 8 adversarial request vectors:
    - `ADV-01`: Conflicting `Content-Length` and `Transfer-Encoding: chunked` (CWE-444 / CL.TE).
    - `ADV-02`: Obfuscated whitespace preceding colon in `Transfer-Encoding : chunked` (RFC 7230 §3.2.4).
    - `ADV-03`: Non-token character in header name (`X-Header@Bad: test`, RFC 7230 §3.2).
    - `ADV-04`: CRLF injection in header value (`X-Custom: val\r\nInjected: evil`).
    - `ADV-05`: Multiple conflicting `Content-Length` headers (`5` and `10`).
    - `ADV-06`: Path traversal probe attempting canary escape (`GET /../../canary_traversal.txt`).
    - `ADV-07`: Oversized request header block exceeding 8 KB limit.
    - `ADV-08`: Null byte injection in URI path (`GET /health%00evil`).
  - Support both standard `http.Request` constructions and raw socket dispatch where raw HTTP/1.1 syntax is required.

### TASK-137.3: Decoupled Dual-Stream Metrics Collection
- **Component**: `benchmarks/wrk2/loadgen.go`
- **Scope**:
  - Implement independent metrics collection for:
    1. **Benign Stream**: Total requests, 200 OK responses, failed requests, bytes read, actual RPS, throughput (MB/s), and latency percentiles ($p50, p75, p90, p95, p99, p99.9, \text{Max}$).
    2. **Adversarial Stream**: Total attack probes, 400/403 rejections, bypasses/leakages (must be 0), and fast-fail rejection latency percentiles ($p50, p90, p99, \text{Max}$).
  - Implement Zero-Starvation validation: Verify that benign $p99 \le 50.0$ ms under concurrent saturation.

### TASK-137.4: Automated Execution Scripting
- **Component**: `benchmarks/wrk2/run_saturation_stress.sh`
- **Scope**:
  - Author shell script supporting flags `-u`, `-c`, `-d`, `-r`, `-a` (attack ratio), and `--auto-start`.
  - Provide automated compilation, background Toron startup, load generation execution, artifact writing, and clean shutdown.

### TASK-137.5: Unit Testing & Publication Telemetry Artifacts
- **Component**: `benchmarks/wrk2/loadgen_test.go`, `benchmarks/results/`
- **Scope**:
  - Implement unit test `TestSaturationStress_ConcurrentExecution` verifying:
    - Dual-stream metrics separation.
    - Zero data races under `-race`.
    - Zero-starvation assertion.
    - 100% invariant rejection on attack stream.
  - Generate publication artifacts `saturation_stress_report.json` and `saturation_stress_report.md`.

---

## 3. Acceptance Criteria

- Sustains 5,000+ RPS background traffic with 10% concurrent attack injection.
- Zero data races under `go test -race -count=1 ./benchmarks/wrk2/...`.
- Benign $p99$ tail latency $\le 50.0$ ms (zero-starvation verified).
- Invariant rejection rate strictly 100.0% on attack stream ($400/403$, 0 bypasses).
- Machine-readable JSON and publication Markdown reports generated in `benchmarks/results/`.
- Pure Go standard library (zero external dependencies).
