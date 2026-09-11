---
id: TASK-129
type: task
title: Implement Latency Timing Alignment (Eq. 7) and Repeated Statistical Distribution Trials in Differential Fuzzer
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-106

owns:
  - benchmarks/fuzzer/diff_fuzzer.go
  - benchmarks/fuzzer/diff_fuzzer_test.go
  - benchmarks/fuzzer/run_fuzzer.sh

references:
  - REQ-106
  - BMK-01
  - BMK-02
  - AER-002
  - MSR-002
  - ADR-106
  - TC-106

derived_from:
  - REQ-106
  - BMK-01
  - BMK-02

implements:
  - REQ-106

verified_by:
  - TC-106

decided_by:
  - ADR-106

related_to:
  - REQ-106
  - ADR-106
  - TC-106
---

# TASK-129 - Implement Latency Timing Alignment (Eq. 7) and Repeated Statistical Distribution Trials in Differential Fuzzer

## 1. Description

Refactor `benchmarks/fuzzer/diff_fuzzer.go` to strictly align latency measurement with Equation 7 ($T_{\text{rejection}} = t_{\text{status\_line\_read}} - t_{\text{socket\_write\_start}}$) by capturing `time.Now()` immediately before socket write, isolating TCP connection setup. Implement repeated benchmark trial execution ($K \ge 1,000$) with a preliminary discarded warm-up phase ($W=50$), and calculate full statistical distribution metrics (Mean, Sample StdDev, Median, p90, p99, p99.9, and 95% Confidence Interval) in accordance with [`REQ-106`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-106.md), [`ADR-106`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-106.md), and [`TC-106`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-106.md).

## 2. Work Breakdown

### Task 129.1: Equation 7 Timing Alignment (BMK-01)
- In `executeRawTestSingle`, shift `start := time.Now()` to immediately before `conn.Write([]byte(finalPayload))`.
- Execute `net.DialTimeout`, socket deadline configurations, and multi-stage preparatory requests (`sendAndDrain`) strictly outside the timed block.
- Record latency with nanosecond precision converted to float64 microseconds (`float64(time.Since(start).Nanoseconds()) / 1000.0`).

### Task 129.2: Statistical Distribution Engine (BMK-02)
- Implement `computeDistribution(samples []float64) DistributionMetrics` using pure Go standard library (`math`, `sort`):
  - In-place ascending sort using `sort.Float64s`.
  - Sample Mean ($\bar{x}$).
  - Sample Standard Deviation ($s$, using Bessel's correction with $K-1$).
  - Median ($p50$) and tail percentiles ($p90, p99, p99.9$) via nearest-rank indexing.
  - 95% Confidence Interval ($\bar{x} \pm 1.96 \cdot \frac{s}{\sqrt{K}}$).

### Task 129.3: Multi-Trial Execution & Warm-up Phase
- Add CLI flags:
  - `-trials <int>`: Number of measured trials $K$ (default: 1).
  - `-warmup <int>`: Number of discarded warm-up trials $W$ (default: 0 for $K=1$, 50 for $K > 1$).
- Implement multi-trial execution loop running $W$ warm-up cycles (discarded) followed by $K$ timed evaluation cycles.
- For multi-stage vectors (`CACHE-001..003`), re-execute preparatory requests before each probe iteration.

### Task 129.4: Data Model & Report Struct Expansion
- Extend `TestResult` and `DifferentialReport` schemas with statistical distribution fields (`mean_latency_us`, `std_dev_latency_us`, `median_latency_us`, `p90_latency_us`, `p99_latency_us`, `ci95_margin_us`, `ci95_lower_us`, `ci95_upper_us`).
- Update Markdown report generation to render statistical dispersion columns.

### Task 129.5: Shell Harness & Unit Test Suite
- Update `benchmarks/fuzzer/run_fuzzer.sh` to forward `-k` (trials) and `-w` (warm-up) flags.
- Add unit tests in `diff_fuzzer_test.go` verifying timing isolation, distribution mathematical precision, and multi-trial repeatability.
