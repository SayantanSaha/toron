---
id: TASK-142
type: task
title: Implement Historical Benchmark Result Retention and Manifest Architecture (REQ-119)
status: ready
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-12
updated: 2026-09-12
depends_on:
  - TASK-141
derived_from:
  - REQ-119
implements:
  - REQ-119
decided_by:
  - ADR-119
verified_by:
  - TC-119
related_to:
  - REQ-119
  - ADR-119
  - TC-119
  - CR-115
  - SR-119
---

# TASK-142 - Implement Historical Benchmark Result Retention and Manifest Architecture (REQ-119)

## 1. Description & Context

Decompose and coordinate the implementation of the Dual-Path Benchmark Result Retention and Historical Run Manifest Architecture across all Toron benchmark harnesses and orchestrators, as specified in [`REQ-119`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-119.md) and [`ADR-119`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-119.md).

### 1.1 Executive Summary
Prior to this task, benchmark executions destructively overwrote files in `benchmarks/results/`, destroying prior empirical data and precluding longitudinal regression analysis. 
This task implements automated retention into immutable timestamped directories under `benchmarks/results/history/<timestamp>/` (`YYYY-MM-DD_HH-MM-SS`), indexes all executions in a central `benchmarks/results/history/manifest.json` using atomic file operations, and maintains canonical files in `benchmarks/results/` for backward compatibility.

---

## 2. Subtask Breakdown

### TASK-142.1: Retention and Manifest Archival Engine
- **Target Component**: `benchmarks/retention_helper.sh` (or `benchmarks/archive_run.sh`)
- **Scope**:
  - Implement reusable POSIX Bash/Go routines to:
    1. Generate ISO/POSIX formatted timestamp directories `benchmarks/results/history/YYYY-MM-DD_HH-MM-SS` with conflict resolution (`_<N>`).
    2. Atomically record or update execution records in `benchmarks/results/history/manifest.json` using temporary file write and atomic rename.
    3. Generate per-run `session_meta.json` within the historical timestamp directory.
    4. Synchronize/copy generated artifacts (`.json`, `.md`, `.csv`, `.raw.txt`, `.log`) to both historical snapshot and canonical latest destination.
  - Support environment variable `TORON_BENCHMARK_SESSION_DIR` for child process inheritance during master suite runs.

### TASK-142.2: Master Orchestrator Integration
- **Target File**: `benchmarks/run_all.sh`
- **Scope**:
  - Initialize a single master session directory `benchmarks/results/history/${SESSION_TS}` at run start.
  - Export session context to all child stages (microbenchmarks, wrk2, saturation stress, diff fuzzer, multihop, ablation).
  - Record the overall suite execution in `manifest.json` with stage details, status, and duration.
  - Ensure canonical latest files in `benchmarks/results/` are refreshed.

### TASK-142.3: WRK2 & Saturation Stress Integration
- **Target Files**:
  - `benchmarks/wrk2/run_wrk2.sh`
  - `benchmarks/wrk2/run_saturation_stress.sh`
  - `benchmarks/wrk2/loadgen.go`
- **Scope**:
  - Integrate standalone and session-inherited retention in `run_wrk2.sh` and `run_saturation_stress.sh`.
  - Preserve `benchmark_c100_r5000.{json,csv,raw.txt}` and `saturation_stress_report.{json,md}` in both historical and canonical paths.
  - Update `manifest.json` when run standalone.

### TASK-142.4: Differential Security Fuzzer Integration
- **Target Files**:
  - `benchmarks/fuzzer/run_fuzzer.sh`
  - `benchmarks/fuzzer/diff_fuzzer.go`
- **Scope**:
  - Ensure `differential_fuzz_report.json` and `differential_fuzz_report.md` are archived to history and mirrored to canonical root.
  - Support session directory inheritance and standalone manifest registration.

### TASK-142.5: Heterogeneous Multi-Hop Testbed Integration
- **Target Files**:
  - `benchmarks/multihop/run_multihop.sh`
  - `benchmarks/multihop/runner.go`
- **Scope**:
  - Preserve `multihop_report.json` and `multihop_report.md` in both historical and canonical locations.
  - Register standalone runs in `manifest.json`.

### TASK-142.6: Controlled Ablation Experiment Integration
- **Target File**: `benchmarks/ablation/run_ablation.sh`
- **Scope**:
  - Preserve `ablation_study_report.json` and `ablation_study_report.md` in history and canonical results root.
  - Update `manifest.json` with task parameters and completion status.

### TASK-142.7: Verification Test Suite
- **Target Files**: `benchmarks/retention_test.go` and verification scripts
- **Scope**:
  - Implement comprehensive tests validating timestamp generation, manifest schema and atomicity, artifact duplication, standalone execution, and backwards compatibility.

---

## 3. Acceptance Criteria Mapping
- **AC-1 to AC-4**: Master suite archiving, artifact presence, and manifest schema verification.
- **AC-5 to AC-7**: Standalone script archiving, multi-run preservation, and atomic update safety.
- **AC-8 to AC-9**: Automated test verification and Equation 1 DAG validation.
