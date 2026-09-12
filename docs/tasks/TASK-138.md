---
id: TASK-138
type: task
title: 10-Task Controlled Ablation Experiment Suite (BMK-05)
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-115

derived_from:
  - REQ-115
  - BMK-05
  - AER-001
  - MSR-001
  - PDR-001
  - AR-001

implements:
  - REQ-115

verified_by:
  - TC-115

decided_by:
  - ADR-115

related_to:
  - REQ-115
  - ADR-115
  - TC-115
  - CR-111
  - SR-115
---

# TASK-138 - 10-Task Controlled Ablation Experiment Suite (BMK-05)

## 1. Description

Decompose and coordinate the implementation of the 10-Task Controlled Ablation Experiment Suite (`BMK-05`, `REQ-115`) across `toron/benchmarks/ablation/`. This experiment suite addresses academic peer review critiques (`AER-001` lines 81–82, `MSR-001` lines 75–81, `PDR-001` lines 167–181) by establishing an empirical comparative evaluation on tasks `TASK-061` through `TASK-070`, contrasting the proposed artifact-anchored multi-agent pipeline with formal ADR contracts (Condition A) against direct single-agent baseline prompting (Condition B).

---

## 2. Subtask Breakdown

### TASK-138.1: Target Task Catalog & Contract Schema
- **Component**: `benchmarks/ablation/tasks.go` / `benchmarks/ablation/schema.go`
- **Scope**:
  - Define formal task catalog data structures representing the 10 evaluated systems engineering tasks (`TASK-061` to `TASK-070`):
    - Task identifier, title, subsystem component, and target vulnerabilities (e.g. CWE-444, CWE-522, CWE-525, CWE-770, CWE-295, CWE-117, CWE-22, CWE-400, CWE-942, CWE-601).
    - Condition A artifacts: requirement document, ADR contract identifier, test case specification, and review findings.
    - Acceptance criteria enumeration for strict automated drift scoring.

### TASK-138.2: Condition A Metadata & Baseline Archival
- **Component**: `benchmarks/ablation/data/condition_a/`
- **Scope**:
  - Ingest and structure historical execution records for `TASK-061` through `TASK-070` from `docs/requirements/`, `docs/architecture/`, `docs/testCases/`, `docs/codeReview/`, and `docs/securityReview/`.
  - Record the multi-agent token expenditures, reviewer-identified latent defects, defect arrest checkpoints, and test outcomes.

### TASK-138.3: Condition B Single-Agent Data Collection & Execution Harness
- **Component**: `benchmarks/ablation/data/condition_b/task_06X/`
- **Scope**:
  - Structure dedicated directories for each evaluated task (`task_061` through `task_070`) storing authentic single-agent execution artifacts:
    - `prompt.txt`: Identical requirement prompt directly submitted to the foundation LLM.
    - `response_raw.md`: Raw model generation containing single-step code solutions.
    - `patch.diff`: Unified git patch representing the single agent's code modifications.
    - `test_execution.log`: Output of `go test -v -race` executed against the target package.
    - `telemetry.json`: Input/output tokens, execution latency, defect counts, and specification compliance scores.

### TASK-138.4: Automated Evaluation Engine & Metrics Calculator
- **Component**: `benchmarks/ablation/runner.go`
- **Scope**:
  - Implement a Go evaluation harness computing the 5 core comparative metrics:
    1. **Specification Drift Rate (%)**: Fraction of mandatory acceptance criteria missed or violated.
    2. **Defect Injection Count**: Protocol oversights, security flaws (CWEs), regressions, and concurrency hazards.
    3. **Test Pass Rate (%)**: Percentage of unit and integration test assertions passed under `go test -race`.
    4. **Token Expenditure & Cost Ratio**: Input, output, total token usage and token expansion ratio ($\text{Cost}_A / \text{Cost}_B$).
    5. **Reviewer Flaw Arrest Rate (%)**: Percentage of latent defects arrested by `CR` and `SR` before merging in Condition A.
  - Calculate summary statistics: Mean ($\bar{x}$), Standard Deviation ($s$), and Relative Improvement percentages.

### TASK-138.5: Reproducibility Automation & Reporting
- **Component**: `benchmarks/ablation/run_ablation.sh`, `benchmarks/results/`
- **Scope**:
  - Script end-to-end execution of the ablation runner.
  - Generate machine-readable JSON: `benchmarks/results/ablation_study_report.json`.
  - Generate publication-grade Markdown: `benchmarks/results/ablation_study_report.md` with:
    - Academic executive summary responding directly to peer reviews.
    - Side-by-side metric tables across all 10 tasks.
    - Aggregate statistics table.
    - Qualitative failure mode analysis for Condition B.

### TASK-138.6: Unit & Race Verification
- **Component**: `benchmarks/ablation/ablation_test.go`
- **Scope**:
  - Add comprehensive unit tests validating parser, metrics calculation, and report generation.
  - Ensure zero external dependencies and 100% clean under `go test -race -count=1 ./benchmarks/ablation/...`.

---

## 3. Acceptance Criteria

- All 10 tasks (`TASK-061` through `TASK-070`) are represented with complete Condition A and Condition B evaluation data.
- Automated runner compiles and executes without third-party dependencies.
- Both JSON and Markdown reports are generated to `benchmarks/results/`.
- All unit tests pass cleanly under `go test -race -count=1 ./benchmarks/ablation/...`.
