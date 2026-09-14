---
id: TASK-144
type: task
title: Multi-Proxy Differential Docker Benchmark Harness Implementation (Toron vs NGINX, Traefik, Caddy, HAProxy)
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-14
updated: 2026-09-14

depends_on:
  - TASK-143

derived_from:
  - REQ-121

implements:
  - REQ-121

decided_by:
  - ADR-121

verified_by:
  - TC-121

related_to:
  - REQ-121
  - ADR-121
  - TC-121
  - CR-117
  - SR-121
---

# TASK-144 - Multi-Proxy Differential Docker Benchmark Harness Implementation (Toron vs NGINX, Traefik, Caddy, HAProxy)

## 1. Description & Context

Decompose and coordinate the engineering implementation required to deliver a reproducible local Docker benchmarking suite comparing **Toron** against **NGINX**, **Traefik**, **Caddy**, and **HAProxy** with identical heterogeneous upstreams, as formally specified in [`REQ-121`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-121.md).

---

## 2. Work Packages & Subtask Breakdown

### TASK-144.1 (WP-1): Shared Heterogeneous Upstream Services Setup
- **Target Directory**: `benchmarks/docker-compare/backends/`
- **Scope**:
  - Re-use or adapt Node.js (`node-origin:9101`), Python (`python-origin:9102`), and Go (`go-origin:9103`) backends.
  - Implement an ultra-lightweight high-throughput `fast-origin:9104` service in Go to benchmark pure reverse-proxy transit overhead without upstream language/runtime bottlenecks.

### TASK-144.2 (WP-2): Standardized Proxy Configurations
- **Target Directory**: `benchmarks/docker-compare/configs/`
- **Scope**:
  - `toron_config.yaml` & `toron_routes.yaml`: Toron edge gateway configuration.
  - `nginx.conf`: NGINX Alpine configuration with upstream keepalives and matching worker processes.
  - `traefik.yml` & `traefik_dynamic.yml`: Traefik v3 configuration with file provider and HTTP services.
  - `Caddyfile`: Caddy Alpine configuration with reverse_proxy and upstream keepalives.
  - `haproxy.cfg`: HAProxy Alpine configuration with roundrobin and persistent HTTP connection pooling.
  - Verify route equivalence: `/node/` -> `node-origin:9101/`, `/python/` -> `python-origin:9102/`, `/go/` -> `go-origin:9103/`, `/fast/` -> `fast-origin:9104/`.

### TASK-144.3 (WP-3): Docker Compose Environment Definition
- **Target File**: `benchmarks/docker-compare/docker-compose.compare.yml`
- **Scope**:
  - Declare all 4 upstreams and all 5 proxies in isolated bridge network `compare-net`.
  - Assign non-conflicting host ports:
    - Toron: `8881`
    - NGINX: `8882`
    - Traefik: `8883` (web), `8880` (dashboard)
    - Caddy: `8884`
    - HAProxy: `8885`
  - Ensure uniform CPU and memory limits/reservations for fair comparison.

### TASK-144.4 (WP-4): Automated Benchmark Runner & Resource Monitor
- **Target Files**:
  - `benchmarks/docker-compare/runner.go`
  - `benchmarks/docker-compare/run_compare.sh`
- **Scope**:
  - Pre-flight validation: verifies 200 OK and route correctness across all 5 proxies and all 4 upstream backends before benchmarking.
  - Execution engine: supports `wrk2`, `wrk`, and zero-dependency `go-loadgen` fallback.
  - Telemetry capture: records container CPU % and Memory RSS (MB) via `docker stats --no-stream`.
  - Generates `benchmarks/results/docker_compare_report.json` and formatted Markdown `benchmarks/results/docker_compare_report.md`.
  - Integrates with `benchmarks/archive_run.sh` for retention manifest recording.

### TASK-144.5 (WP-5): Makefile and Developer Ergonomics Integration
- **Target File**: `Makefile`
- **Scope**:
  - Add `make benchmark-compare` and `make benchmark-compare-clean` targets.

---

## 3. Acceptance Criteria

- [ ] All Docker Compose services start and pass health checks.
- [ ] Pre-flight checks succeed for all 5 proxies across all 4 routes.
- [ ] Benchmarks run without socket leaks or unhandled errors.
- [ ] Reports generated with RPS, P50, P90, P99, P99.9, Max latency, CPU %, and RSS MB.
- [ ] Teardown removes containers and networks cleanly.
