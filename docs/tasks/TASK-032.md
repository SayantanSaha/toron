---
id: TASK-032
type: task
title: Expose Comprehensive Metrics in /internal/api/ Control Plane Endpoints
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-13
updated: 2026-08-13

depends_on:
  - REQ-032

implements:
  - REQ-032

verified_by:
  - TC-032

decided_by:
  - ADR-027

related_to:
  - TASK-017
  - TASK-031
---

# TASK-032 - Expose Comprehensive Metrics in /internal/api/ Control Plane Endpoints

## Goal

Add `GetSummaryJSON()` to `MetricsRegistry` in `pkg/metrics/metrics.go`, update `GET /internal/api/status`, and register `GET /internal/api/metrics` in `pkg/server/internal_api.go`.

## Sub-tasks

1. Implement `GetSummaryJSON()` in `pkg/metrics/metrics.go` to extract request breakdowns by status, method, route, active QUIC streams, active TCP conns, and circuit breaker trips.
2. Update `GET /internal/api/status` in `pkg/server/internal_api.go` to include `metrics` key in JSON response.
3. Register `GET /internal/api/metrics` in `pkg/server/internal_api.go` returning JSON metrics.
4. Write unit tests for `/internal/api/status` and `/internal/api/metrics` in `pkg/server/server_test.go`.
5. Run full test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- `GET /internal/api/status` includes `metrics` JSON object with request breakdowns and active connections.
- `GET /internal/api/metrics` returns HTTP status 200 with `Content-Type: application/json`.
- All Go unit tests pass cleanly.
