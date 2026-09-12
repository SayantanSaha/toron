---
title: Native Performance Benchmarking & Historical Result Retention
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-12

depends_on:
  - REQ-008
  - TASK-008
  - REQ-119
  - TASK-142

derived_from:
  - REQ-008
  - REQ-119
  - ADR-003
  - ADR-119

documents:
  - BENCHMARKING-FEATURE

related_to:
  - index.md
  - features/event-reactor.md
  - features/saturation-stress-benchmark.md
  - features/differential-fuzzer-metrics.md
---

# Native Performance Benchmarking & Historical Result Retention

## Overview

Toron provides a comprehensive, research-grade evaluation and benchmarking suite covering in-process microbenchmarks, high-throughput load generation (`wrk2`), saturation stress testing with adversarial injection, differential protocol security fuzzing, multi-hop backend origin routing, and controlled multi-agent ablation studies.

Under **REQ-119** and **ADR-119**, the benchmarking suite employs a **Dual-Path Result Retention and Atomic Manifest Architecture** that guarantees immutable persistence of every execution session while preserving 100% backward compatibility for existing canonical paths.

---

## Benchmark Suite Architecture

```
benchmarks/
├── run_all.sh                                  <-- Master orchestrator executing all 6 evaluation stages
├── archive_run.sh                              <-- Archival and manifest helper utility
├── retention/                                  <-- Go retention and manifest engine
│   ├── retention.go                            <-- Core replication, timestamping, and atomic manifest logic
│   ├── retention_test.go                       <-- Unit tests (collision, replication, concurrent manifest updates)
│   ├── integration_test.go                     <-- End-to-end CLI integration test
│   └── cmd/main.go                             <-- CLI entrypoint for session init, archive, and record
│
├── wrk2/
│   ├── run_wrk2.sh                             <-- Baseline throughput and tail-latency harness
│   ├── run_saturation_stress.sh                <-- Saturation stress testing with adversarial injection
│   └── loadgen.go                              <-- High-performance zero-dependency load generator
│
├── fuzzer/
│   ├── run_fuzzer.sh                           <-- Differential protocol security fuzzer runner
│   └── diff_fuzzer.go                          <-- 19-vector invariant oracle
│
├── multihop/
│   ├── run_multihop.sh                         <-- Heterogeneous backend origin testbed runner
│   └── runner.go                               <-- In-process/live origin desync testbed
│
├── ablation/
│   ├── run_ablation.sh                         <-- 10-task controlled ablation experiment runner
│   └── cmd/main.go                             <-- Ablation comparison engine
│
└── results/                                    <-- Canonical latest results (backward compatibility)
    ├── history/                                <-- Immutable historical telemetry archive
    │   ├── manifest.json                       <-- Structured central index of all historical runs
    │   └── YYYY-MM-DD_HH-MM-SS/                <-- Isolated timestamped run directory
    │       ├── session_meta.json               <-- Self-contained run metadata
    │       └── [report files...]               <-- Immutable copies of all generated artifacts
    │
    ├── microbenchmarks.raw.txt                 <-- Latest microbenchmark results
    ├── benchmark_c100_r5000.json               <-- Latest wrk2 throughput / latency JSON
    ├── saturation_stress_report.json / .md     <-- Latest saturation stress reports
    ├── differential_fuzz_report.json / .md     <-- Latest differential fuzzing reports
    ├── multihop_report.json / .md              <-- Latest multi-hop origin reports
    └── ablation_study_report.json / .md        <-- Latest ablation experiment reports
```

---

## Dual-Path Retention Model

1. **Immutable Historical Telemetry (`benchmarks/results/history/<timestamp>/`)**:
   - Every benchmark execution automatically creates an isolated directory named with format `YYYY-MM-DD_HH-MM-SS` (e.g. `2026-09-12_12-30-00`).
   - If sub-second repeated executions occur, an incremental suffix `_<N>` is appended to prevent collisions.
   - Contains immutable copies of all generated artifacts (`.json`, `.md`, `.csv`, `.raw.txt`, logs) and a self-contained `session_meta.json`.

2. **Central Structured Index (`benchmarks/results/history/manifest.json`)**:
   - Chronologically catalogs every benchmark execution session.
   - Captures run ID, timestamp, suite, parameters, duration, git commit, git branch, Go version, exit status, and generated artifact file list.
   - Updates are executed atomically via temporary file writes and atomic renames, preventing JSON corruption during concurrent or aborted runs.

3. **Canonical Latest Synchronization (`benchmarks/results/`)**:
   - All canonical report files in `benchmarks/results/` are simultaneously updated with the latest run.
   - Guarantees 100% backward compatibility for LaTeX manuscript citations, Wiki documentation links, and automated CI/CD pipelines.

---

## Execution Modes & CLI Options

### 1. Running the Master Orchestrator (`run_all.sh`)
Executes all 6 evaluation stages in sequence and consolidates all artifacts under a single unified master session directory:
```bash
# Standard execution (automatically creates historical session and updates canonical latest)
bash benchmarks/run_all.sh --auto-start

# Custom session suffix tag
bash benchmarks/run_all.sh --auto-start --session-name pre_release_eval

# Ephemeral execution without historical archiving
bash benchmarks/run_all.sh --auto-start --no-history
```

### 2. Running Standalone Subsystem Benchmarks
Each individual benchmark script operates standalone or as part of the master suite:

- **WRK2 Latency Harness**:
  ```bash
  bash benchmarks/wrk2/run_wrk2.sh -u "http://127.0.0.1:8080/health" -c 100 -d 10s -r 10000
  ```
- **Saturation Stress Testing (BMK-04)**:
  ```bash
  bash benchmarks/wrk2/run_saturation_stress.sh -u "http://127.0.0.1:8080/health" -c 50 -d 10s -r 5000 -a 0.10
  ```
- **Differential Protocol Security Fuzzer (BMK-01, BMK-02)**:
  ```bash
  bash benchmarks/fuzzer/run_fuzzer.sh -t "127.0.0.1:8080" -k 1000 -w 50
  ```
- **Multi-Hop Origin Testbed (BMK-03)**:
  ```bash
  bash benchmarks/multihop/run_multihop.sh --standalone
  ```
- **Controlled Ablation Experiment (BMK-05)**:
  ```bash
  bash benchmarks/ablation/run_ablation.sh
  ```

### 3. Universal Retention CLI Flags
All scripts support the following retention flags:
- `--no-history`: Skips writing to `history/` and skips updating `manifest.json`. Only updates canonical files in `benchmarks/results/`.
- `--session-name <name>`: Appends `<name>` to the generated timestamp directory (e.g. `2026-09-12_12-30-00_opt_pass`).
- `--session-dir <dir>`: Explicitly directs outputs into `<dir>`.

---

## Manifest Schema (`manifest.json`)

The central manifest at `benchmarks/results/history/manifest.json` conforms to the following schema:
```json
{
  "schema_version": "1.0",
  "updated_at": "2026-09-12T12:41:43+05:30",
  "total_runs": 2,
  "runs": [
    {
      "run_id": "run_2026-09-12_12-41-19",
      "timestamp": "2026-09-12T12:41:21+05:30",
      "directory": "benchmarks/results/history/2026-09-12_12-41-19",
      "suite": "ablation",
      "command": "benchmarks/ablation/run_ablation.sh",
      "git_commit": "1637ed28b5eac04591644a152dbce784ef07d78a",
      "git_branch": "master",
      "go_version": "go1.26.6",
      "status": "success",
      "duration_seconds": 2,
      "artifacts": [
        "ablation_study_report.json",
        "ablation_study_report.md"
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
```

### Baseline Performance Results

| Benchmark Target | Speed (`ns/op`) | Memory (`B/op`) | Allocations (`allocs/op`) |
| :--- | :---: | :---: | :---: |
| `BufferPool` (Reactor) | **8.29 ns** | **0 B** | **0 allocs** |
| `MatchExact` (Router) | **16.30 ns** | **0 B** | **0 allocs** |
| `MiddlewareChain` (Router) | **61.99 ns** | **48 B** | **3 allocs** |
| `Serialize` (Response) | **638.5 ns** | **480 B** | **11 allocs** |
| `ParseRequest_GET` (Parser) | **2.05 µs** | **5.8 KB** | **33 allocs** |
| `ParseRequest_POST` (Parser) | **1.86 µs** | **5.4 KB** | **32 allocs** |

---

## Related Pages

- [High-Concurrency Saturation Stress Benchmark & Status Classification](./saturation-stress-benchmark.md)
- [Differential Fuzzer Metric Architecture](./differential-fuzzer-metrics.md)
- [Event Reactor Core](./event-reactor.md)
- [Release Notes](../release-notes.md)
