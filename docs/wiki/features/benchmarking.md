---
title: Native Performance Benchmarking & Historical Result Retention
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-16

depends_on:
  - REQ-008
  - TASK-008
  - REQ-114
  - REQ-117
  - REQ-119
  - REQ-121
  - REQ-122
  - REQ-129
  - REQ-130
  - TASK-142
  - TASK-144
  - TASK-152
  - TASK-153
  - ADR-003
  - ADR-114
  - ADR-117
  - ADR-119
  - ADR-121
  - ADR-129
  - ADR-130
  - TC-114
  - TC-117
  - TC-119
  - TC-121
  - TC-129
  - TC-130
  - CR-126
  - SR-130

derived_from:
  - REQ-008
  - REQ-119
  - REQ-130
  - ADR-003
  - ADR-119
  - ADR-130

documents:
  - BENCHMARKING-FEATURE
  - MULTI-TIER-BENCHMARK-ORCHESTRATION

related_to:
  - index.md
  - features/event-reactor.md
  - features/saturation-stress-benchmark.md
  - features/docker-compare-benchmark.md
  - features/differential-fuzzer-metrics.md
  - ../release-notes.md
---

# Native Performance Benchmarking & Historical Result Retention

## Overview

Toron provides a comprehensive, research-grade evaluation and benchmarking suite covering in-process microbenchmarks, high-throughput load generation (`wrk2`), saturation stress testing with adversarial injection, differential protocol security fuzzing, containerized differential reverse proxy comparison against **NGINX**, **Traefik**, **Caddy**, and **HAProxy**, multi-hop backend origin routing, and controlled multi-agent ablation studies.

Under **REQ-119** and **ADR-119**, the benchmarking suite employs a **Dual-Path Result Retention and Atomic Manifest Architecture** that guarantees immutable persistence of every execution session while preserving 100% backward compatibility for existing canonical paths.

Under **REQ-130** and **ADR-130**, the suite integrates a **Multi-Tier Duration Benchmark Taxonomy** (`quick` 5s, `medium`/`steady` 60s, `soak` 300s, `all`) alongside automated Go runtime garbage collection telemetry capture (`GODEBUG=gctrace=1`), zero-dependency GC trace parsing, and Ordinary Least Squares (OLS) linear regression heap growth analysis.

---

## Benchmark Suite Architecture

```
benchmarks/
├── run_all.sh                                  <-- Master orchestrator (--tier, -d, auto-start, retention)
├── archive_run.sh                              <-- Archival and manifest synchronization utility
│
├── telemetry/                                  <-- Zero-dependency runtime telemetry engines
│   └── gcparser/                               <-- Pure Go runtime GC trace parser (REQ-130)
│       ├── model.go                            <-- GCTelemetry, GCPauseStatistics, GCHeapStatistics
│       ├── parser.go                           <-- Fast tokenizer (fastParseLine) & regex fallback
│       ├── stats.go                            <-- STW percentiles, cycle rates, OLS slope (MB/min)
│       └── parser_test.go                      <-- Parser unit tests, fixtures, and concurrency tests
│
├── retention/                                  <-- Go retention and manifest engine (REQ-119)
│   ├── retention.go                            <-- Replication, collision-safe timestamps, atomic manifest
│   ├── retention_test.go                       <-- Unit tests (collision, replication, concurrent updates)
│   ├── integration_test.go                     <-- End-to-end CLI integration test
│   └── cmd/main.go                             <-- CLI entrypoint for session init, archive, and record
│
├── wrk2/                                       <-- High-concurrency saturation testing (BMK-04, REQ-114)
│   ├── run_wrk2.sh                             <-- Baseline throughput and tail-latency harness
│   ├── run_saturation_stress.sh                <-- Multi-tier saturation stress runner (-d, --tier)
│   └── loadgen.go                              <-- Dual-stream load generator with GC telemetry embedding
│
├── docker-compare/                             <-- Multi-proxy differential benchmark (REQ-121, REQ-130)
│   ├── docker-compose.compare.yml              <-- 5 proxies + 4 backends with GODEBUG=gctrace=1 injection
│   ├── run_compare.sh                          <-- Multi-tier differential runner (-d, --tier)
│   ├── runner.go                               <-- Warm-up, Docker stats poller, container GC extraction
│   └── compare_test.go                         <-- Unit tests for stats, latency, and report generation
│
├── fuzzer/                                     <-- Differential protocol security fuzzer (BMK-01, BMK-02)
│   ├── run_fuzzer.sh                           <-- Fuzzer runner
│   └── diff_fuzzer.go                          <-- 19-vector protocol invariant oracle
│
├── multihop/                                   <-- Heterogeneous backend origin testbed (BMK-03, REQ-120)
│   ├── run_multihop.sh                         <-- Testbed runner (standalone in-process or Docker cluster)
│   └── runner.go                               <-- Multi-hop origin desync testbed engine
│
├── ablation/                                   <-- Controlled ablation experiment (BMK-05)
│   ├── run_ablation.sh                         <-- 10-task controlled ablation experiment runner
│   └── cmd/main.go                             <-- Ablation comparison engine
│
└── results/                                    <-- Canonical latest results (backward compatibility)
    ├── history/                                <-- Immutable historical telemetry archive
    │   ├── manifest.json                       <-- Structured central index of all historical runs
    │   └── YYYY-MM-DD_HH-MM-SS/                <-- Isolated timestamped run directory
    │       ├── session_meta.json               <-- Self-contained run metadata
    │       ├── server_gc_trace.log             <-- Raw Go runtime GC traces (REQ-130)
    │       ├── saturation_stress_*.json/.md    <-- Per-duration saturation stress artifacts
    │       └── [report files...]               <-- Immutable copies of all generated artifacts
    │
    ├── server_gc_trace.log                     <-- Latest raw Go GC traces from saturation stress
    ├── server_stress.log                       <-- Latest Toron gateway stdout logs
    ├── microbenchmarks.raw.txt                 <-- Latest microbenchmark results
    ├── benchmark_c100_r5000.json               <-- Latest wrk2 throughput / latency JSON
    ├── saturation_stress_report.json / .md     <-- Consolidated saturation stress reports with GC dynamics
    ├── docker_compare_report.json / .md        <-- Multi-proxy differential reports with GC analysis
    ├── differential_fuzz_report.json / .md     <-- Latest differential fuzzing reports
    ├── multihop_report.json / .md              <-- Latest multi-hop origin reports
    └── ablation_study_report.json / .md        <-- Latest ablation experiment reports
```

---

## Multi-Tier Duration Taxonomy (`REQ-130`)

To eliminate the 5-second evaluation blindspot (transient socket creation bias, GC pause masking, and invisible memory leaks), the benchmark suite defines four standardized execution tiers:

| Tier Name | CLI Identifier | Duration | Primary Empirical Purpose | Master Suite CI Impact |
| :--- | :---: | :---: | :--- | :--- |
| **Quick Smoke** | `quick` | 5 seconds | Fast pre-merge CI regression testing and sanity checks. | **Default: $< 60\text{s}$ suite runtime** |
| **Steady-State / GC** | `medium` (or `steady`) | 60 seconds (1 min) | Deep observation of Go GC cycles, STW pause distributions, and HDR tail latencies. | Explicit CLI invocation |
| **Long-Term Soak** | `soak` | 300 seconds (5 min) | Empirical proof of constant $O(1) \le 32\text{KB}$ memory bound (slope $\le 1.0\text{ MB/min}$). | Dedicated performance evaluation |
| **All Tiers Sweep** | `all` | 5s + 60s + 300s | Sequential sweep generating duration-keyed and consolidated comparative reports. | Research publication profiling |

---

## Execution Modes & CLI Options

### 1. Running the Master Orchestrator (`run_all.sh`)

`benchmarks/run_all.sh` coordinates all evaluation stages in sequence and consolidates all artifacts under a single unified master session directory:

```bash
# Standard Quick CI Execution (default: 5s quick tier, completes in < 60s)
bash benchmarks/run_all.sh --auto-start

# Steady-State 60-Second Evaluation across stages
bash benchmarks/run_all.sh --auto-start --tier medium

# Extended 300-Second Soak Evaluation
bash benchmarks/run_all.sh --auto-start --tier soak

# Custom Duration Specification
bash benchmarks/run_all.sh --auto-start -d 30s

# Named Session Directory in Historical Archive
bash benchmarks/run_all.sh --auto-start --session-name pre_release_eval

# Ephemeral execution without historical archiving
bash benchmarks/run_all.sh --auto-start --no-history
```

#### Duration Flag Propagation in `run_all.sh`
When `--tier` or `-d` is specified, `run_all.sh` normalizes the duration and automatically propagates the parameters downstream:
- **Stage 2 (WRK2 Baseline)**: Receives `-d <duration>`.
- **Stage 3 (Saturation Stress BMK-04)**: Receives `--tier <tier>` and `-d <duration>` while launching the Toron background gateway with `GODEBUG=gctrace=1` and redirecting GC telemetry to `server_gc_trace.log`.
- **Master Manifest Recording**: Records `duration_tier`, duration in seconds, stage parameters, and GC trace artifacts into `manifest.json`.

---

### 2. Running Standalone Subsystem Benchmarks

Each subsystem harness can be executed independently:

- **Saturation Stress Testing (BMK-04, REQ-114, REQ-130)**:
  ```bash
  # Quick smoke run (5s)
  bash benchmarks/wrk2/run_saturation_stress.sh --auto-start

  # Steady-state 60s run with GC telemetry capture
  bash benchmarks/wrk2/run_saturation_stress.sh --auto-start --tier medium -r 5000 -c 50

  # Sequential multi-tier sweep across 5s, 60s, 300s
  bash benchmarks/wrk2/run_saturation_stress.sh --auto-start --tier all -r 5000 -c 50
  ```
  See dedicated guide in [High-Concurrency Saturation Stress Benchmark & Status Classification](./saturation-stress-benchmark.md).

- **Multi-Proxy Differential Docker Benchmark (REQ-121, REQ-130)**:
  ```bash
  # Standard quick comparison across all 5 proxies and 4 backends
  ./benchmarks/docker-compare/run_compare.sh

  # Steady-state 60s evaluation with continuous Docker polling and GC analysis
  ./benchmarks/docker-compare/run_compare.sh --tier medium -c 50

  # Long-term 300s soak test
  ./benchmarks/docker-compare/run_compare.sh --tier soak -c 50
  ```
  See dedicated guide in [Multi-Proxy Differential Docker Benchmark Suite](./docker-compare-benchmark.md).

- **WRK2 Latency Harness**:
  ```bash
  bash benchmarks/wrk2/run_wrk2.sh -u "http://127.0.0.1:8080/health" -c 100 -d 10s -r 10000
  ```

- **Differential Protocol Security Fuzzer (BMK-01, BMK-02)**:
  ```bash
  bash benchmarks/fuzzer/run_fuzzer.sh -t "127.0.0.1:8080" -k 1000 -w 50
  ```

- **Multi-Hop Origin Testbed (BMK-03, REQ-120)**:
  ```bash
  # In-process mode (zero Docker dependency, pure Go)
  bash benchmarks/multihop/run_multihop.sh --standalone

  # Live container cluster mode (Docker Compose)
  bash benchmarks/multihop/run_multihop.sh --docker
  ```
  See dedicated architecture in [Heterogeneous Multi-Hop Backend Origin Testbed Architecture](./multihop-testbed.md).

- **Controlled Ablation Experiment (BMK-05)**:
  ```bash
  bash benchmarks/ablation/run_ablation.sh
  ```

---

### 3. Universal Retention CLI Flags

All benchmarking harnesses accept standardized retention flags:
- `--no-history`: Skips writing to `history/` and skips updating `manifest.json`. Only updates canonical files in `benchmarks/results/`.
- `--session-name <name>`: Appends `<name>` to the generated timestamp directory (e.g. `2026-09-16_12-30-00_opt_pass`).
- `--session-dir <dir>`: Explicitly directs outputs into `<dir>`.

---

## Dual-Path Retention Model & Manifest Schema (`manifest.json`)

1. **Immutable Historical Telemetry (`benchmarks/results/history/<timestamp>/`)**:
   - Automatically creates an isolated timestamped directory (`YYYY-MM-DD_HH-MM-SS`) containing immutable copies of all generated artifacts (`.json`, `.md`, `.csv`, `.raw.txt`, logs) and `session_meta.json`.
   - Replicates runtime GC traces (`server_gc_trace.log`) and duration-keyed reports (`saturation_stress_*.json`, `docker_compare_*.json`).
2. **Central Structured Index (`benchmarks/results/history/manifest.json`)**:
   - Chronologically catalogs every benchmark execution session.
   - Captures run ID, timestamp, suite, parameters (including `duration_tier`), duration, git commit, branch, Go version, exit status, and generated artifact file list.
   - Updates are executed atomically via temporary file writes and atomic renames, preventing JSON corruption during concurrent or aborted runs.
3. **Canonical Latest Synchronization (`benchmarks/results/`)**:
   - All canonical report files in `benchmarks/results/` are simultaneously updated with the latest run, guaranteeing 100% backward compatibility for automated CI pipelines and LaTeX citations.

### Manifest Schema Specification

```json
{
  "schema_version": "1.0",
  "updated_at": "2026-09-16T12:45:00+05:30",
  "total_runs": 3,
  "runs": [
    {
      "run_id": "run_2026-09-16_12-40-00",
      "timestamp": "2026-09-16T12:40:02+05:30",
      "directory": "benchmarks/results/history/2026-09-16_12-40-00",
      "suite": "master",
      "command": "benchmarks/run_all.sh --auto-start --tier medium",
      "git_commit": "7b8f9e0123456789abcdef0123456789abcdef01",
      "git_branch": "master",
      "go_version": "go1.24.0",
      "status": "success",
      "duration_seconds": 78,
      "parameters": {
        "duration_tier": "medium",
        "duration": "60s",
        "gc_trace_captured": true
      },
      "artifacts": [
        "benchmark_c100_r5000.json",
        "saturation_stress_60s.json",
        "saturation_stress_60s.md",
        "saturation_stress_report.json",
        "saturation_stress_report.md",
        "server_gc_trace.log",
        "server_stress.log",
        "differential_fuzz_report.json",
        "differential_fuzz_report.md"
      ]
    }
  ]
}
```

---

## In-Process Parser Microbenchmarks

Run microbenchmarks across individual packages:
```bash
go test -bench=BenchmarkParseRequest -benchmem ./pkg/httpparser/...
go test -bench=BenchmarkParseReader -benchmem ./benchmarks/telemetry/gcparser/...
```

### Baseline Microbenchmark Results

| Benchmark Target | Speed (`ns/op`) | Memory (`B/op`) | Allocations (`allocs/op`) |
| :--- | :---: | :---: | :---: |
| `BufferPool` (Reactor) | **8.29 ns** | **0 B** | **0 allocs** |
| `MatchExact` (Router) | **16.30 ns** | **0 B** | **0 allocs** |
| `MiddlewareChain` (Router) | **61.99 ns** | **48 B** | **3 allocs** |
| `Serialize` (Response) | **638.5 ns** | **480 B** | **11 allocs** |
| `ParseRequest_GET` (Parser) | **2.05 µs** | **5.8 KB** | **33 allocs** |
| `ParseRequest_POST` (Parser) | **1.86 µs** | **5.4 KB** | **32 allocs** |
| `GCParser_Scan10kLines` (Telemetry) | **28.4 ms** | **0 B** (hot-path) | **0 allocs/line** |

---

## Related Pages

- [High-Concurrency Saturation Stress Benchmark & Status Classification](./saturation-stress-benchmark.md)
- [Multi-Proxy Differential Docker Benchmark Suite](./docker-compare-benchmark.md)
- [Differential Fuzzer Metric Architecture](./differential-fuzzer-metrics.md)
- [Heterogeneous Multi-Hop Backend Origin Testbed Architecture](./multihop-testbed.md)
- [Event Reactor Core](./event-reactor.md)
- [Release Notes](../release-notes.md)
