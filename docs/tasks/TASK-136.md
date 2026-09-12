---
id: TASK-136
type: task
title: Heterogeneous Multi-Hop Backend Origin Testbed Implementation (BMK-03)
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-113

derived_from:
  - REQ-113
  - BMK-03
  - AER-002
  - AR-002

implements:
  - REQ-113

verified_by:
  - TC-113

decided_by:
  - ADR-113

related_to:
  - REQ-113
  - ADR-113
  - TC-113
  - CR-109
  - SR-113
---

# TASK-136 - Heterogeneous Multi-Hop Backend Origin Testbed Implementation (BMK-03)

## 1. Description

Decompose and construct an automated, publication-grade Heterogeneous Multi-Hop Backend Origin Testbed (`BMK-03`, `REQ-113`) in `benchmarks/multihop/`. The testbed addresses academic peer reviews (`AER-002` Issue 6, `AR-002` Alternative Explanation 3) by testing Toron fronting three live, distinct backend HTTP runtime parsers:
1. **Node.js**: C-based `llhttp` parser engine (Node.js 20 LTS).
2. **Python**: ASGI/`h11`/`httptools` under Uvicorn (Python 3.11).
3. **Go**: Canonical standard library `net/http` parser (Go 1.24).

The testbed implements a formal two-stage desynchronization evaluation protocol ($r_{\text{poison}} \,\|\, r_{\text{benign}}$) across 10 attack and baseline vectors, verifying edge enforcement, transport socket teardown, zero residual backend byte leakage, and 100% upstream connection pool integrity.

---

## 2. Subtask Breakdown

### TASK-136.1: Heterogeneous Backend Origin Services
- **Component**: `benchmarks/multihop/backends/{node,python,go}/`
- **Scope**:
  - Implement Node.js backend origin (`server.js`, `package.json`, `Dockerfile`):
    - Endpoints: `GET /health`, `GET /canary`, `POST /echo`.
    - Tracks sequential request counters, echoes raw headers, records connection IDs.
  - Implement Python backend origin (`server.py`, `requirements.txt`, `Dockerfile`):
    - Endpoints: `GET /health`, `GET /canary`, `POST /echo`.
    - Implements ASGI HTTP server tracking request counters and connection state.
  - Implement Go backend origin (`main.go`, `Dockerfile`):
    - Endpoints: `GET /health`, `GET /canary`, `POST /echo`.
    - Standard library `net/http` tracking request counters and headers.
  - Provide lightweight, fast-building multi-arch Dockerfiles for all 3 backends.

### TASK-136.2: Multi-Hop Orchestration & Gateway Routing Configuration
- **Component**: `benchmarks/multihop/`
- **Scope**:
  - Author `docker-compose.multihop.yml` defining isolated network bridge (`multihop-net`) with `toron-edge`, `node-origin`, `python-origin`, and `go-origin`.
  - Author `routes.multihop.yaml` defining upstream routes:
    - Prefix `/node` -> `target: http://node-origin:9101` (`strip_prefix: true`).
    - Prefix `/python` -> `target: http://python-origin:9102` (`strip_prefix: true`).
    - Prefix `/go` -> `target: http://go-origin:9103` (`strip_prefix: true`).
  - Author `config.multihop.yaml` specifying Toron edge gateway runtime parameters (worker pool, cleartext h2c, timeouts).

### TASK-136.3: Two-Stage Desynchronization Evaluation Harness
- **Component**: `benchmarks/multihop/runner.go`
- **Scope**:
  - Implement standard-library Go runner executing the two-stage protocol:
    - Stage 1: Send $r_{\text{poison}}$ probe (H2.TE, H2.CL-Duplicate, H2.CL-Mismatch, H1-CL.TE, H1-TE.CL-Obfuscated, H1-Pipelined-Smuggle, CRLF-Header-Injection, Pseudo-Header-Isolation, Baseline-GET, Baseline-POST).
    - Measure edge status, verify socket teardown on security rejections.
    - Stage 2: Transmit benign canary $r_{\text{benign}}$ (`GET /canary`) over the connection session.
    - Validate canary response status (`200 OK`), payload integrity, and backend request sequence count consistency.
  - Assert zero desynchronization and zero backend byte leakage.

### TASK-136.4: Standalone In-Process CI Test Suite
- **Component**: `benchmarks/multihop/multihop_test.go`
- **Scope**:
  - Implement in-process listeners simulating Node.js (`llhttp`), Python (`h11`), and Go (`net/http`) parsing and sequence semantics.
  - Spin up in-process Toron edge gateway with routing table pointing to simulated backends.
  - Execute full suite of 10 test vectors across all 3 backends (30 attack scenarios).
  - Run with `go test -race -count=1 ./benchmarks/multihop/...` with zero external dependencies and 100% CI determinism.

### TASK-136.5: Publication Reporting & Documentation
- **Component**: `benchmarks/results/`, `benchmarks/README.md`, `benchmarks/multihop/run_multihop.sh`
- **Scope**:
  - Generate `benchmarks/results/multihop_report.json` containing structured execution telemetry.
  - Generate `benchmarks/results/multihop_report.md` with cross-runtime matrix and academic analysis.
  - Author `benchmarks/multihop/run_multihop.sh` with `--docker` and `--standalone` execution flags.
  - Update `benchmarks/README.md` documenting multi-hop testbed architecture, usage, and artifact reproducibility.

---

## 3. Acceptance Criteria

- All three backend origins (Node.js, Python, Go) build and run cleanly.
- Dual execution modes: Live Docker Compose and standalone in-process Go test harness.
- All 10 test vectors executed across all 3 backends ($10 \times 3 = 30$ scenarios).
- Rejections enforce socket closure with zero residual bytes reaching backends.
- Canaries succeed with $200\text{ OK}$ and correct sequence IDs.
- Reports generated in both JSON and Markdown formats in `benchmarks/results/`.
- 100% race-clean (`go test -race -count=1 ./benchmarks/multihop/...`).
- Zero third-party Go dependencies (pure standard library).
