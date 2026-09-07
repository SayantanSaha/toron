---
id: TASK-083
type: task
title: Implement Automated wrk2 Benchmark Suite and Differential Security Fuzzer Harness
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-07
updated: 2026-09-07
depends_on: []
derived_from:
  - REQ-083
implements:
  - REQ-083
verified_by:
  - TC-083
decided_by:
  - ADR-078
related_to: []
---

# TASK-083 - Implement Automated wrk2 Benchmark Suite and Differential Security Fuzzer Harness

## Description

Create an automated, repeatable benchmarking framework and differential security fuzzer suite in `benchmarks/` to support academic evaluation, research publication figures, and artifact verification.

## Scope & Implementation Breakdown

1. **Throughput & Tail Latency Benchmarking (`benchmarks/wrk2/`)**:
   - Create `benchmarks/wrk2/loadgen.go`: High-performance Go load generator executing constant-rate throughput tests (`-rate`, `-c`, `-d`), computing p50, p90, p99, p99.9 latency percentiles, and exporting results in JSON/CSV.
   - Create `benchmarks/wrk2/run_wrk2.sh`: Shell script to probe for `wrk2` / `wrk` and fallback cleanly to `loadgen.go`. Supports multi-run averaging, concurrency sweeps (50, 100, 500, 1000), and rate curves (5k to 50k RPS).
   - Create Lua scripts: `benchmarks/wrk2/scripts/pipeline.lua` and `benchmarks/wrk2/scripts/post_payload.lua`.

2. **Differential Security Fuzzer (`benchmarks/fuzzer/`)**:
   - Create `benchmarks/fuzzer/diff_fuzzer.go`: Differential security test engine sending raw HTTP socket payloads covering CL.TE desync, multiple `Content-Length`, RFC 7230 §3.2.4 whitespace violations, control characters (`\x00-\x1F`), path traversal (`%2e%2e`), and HTTP Parameter Pollution.
   - Evaluates target responses against expected security invariants (fail-fast 400/501 vs. permissive bypass), measuring microsecond rejection latency.
   - Emits structured JSON (`differential_fuzz_report.json`) and formatted Markdown (`differential_fuzz_report.md`).
   - Create `benchmarks/fuzzer/run_fuzzer.sh`.

3. **Orchestration & Documentation (`benchmarks/`)**:
   - Create `benchmarks/run_all.sh`: Top-level orchestration script capable of starting a standalone test Toron server, running all suites, saving artifacts, and shutting down cleanly.
   - Create `benchmarks/README.md`: Reproduction instructions, artifact evaluation steps, and experimental setup details.

## Acceptance Criteria

- All scripts and Go tools compile and run cleanly without third-party dependencies.
- `diff_fuzzer.go` validates all core vulnerability mitigations (SEC-01 through SEC-22).
- `run_all.sh` executes end-to-end and outputs structured files into `benchmarks/results/`.
- Verified by `TC-083`.
