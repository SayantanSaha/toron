---
id: TASK-031
type: task
title: Implement Prometheus Metrics Registry and W3C Traceparent Header Propagation
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-13
updated: 2026-08-13

depends_on:
  - REQ-031

implements:
  - REQ-031

verified_by:
  - TC-031

decided_by:
  - ADR-026

related_to:
  - TASK-017
  - TASK-027
---

# TASK-031 - Implement Prometheus Metrics Registry and W3C Traceparent Header Propagation

## Goal

Create `pkg/metrics/metrics.go` for recording Prometheus counters/histograms, register `/metrics` endpoint on the router, and inject/propagate W3C `traceparent` headers in `pkg/proxy/proxy.go`.

## Sub-tasks

1. Create `pkg/metrics/metrics.go` with thread-safe counters, gauges, histograms, and Prometheus text exporter (`ExportPrometheus()`).
2. Add W3C `traceparent` extraction and generation helper functions in `pkg/metrics/tracing.go`.
3. Register GET `/metrics` endpoint in `pkg/router/router.go` returning Prometheus format.
4. Update `pkg/proxy/proxy.go` to extract or generate W3C `traceparent` headers and forward them to upstream targets.
5. Record metric events (request durations, circuit breaker trips, QUIC streams) in router, proxy, and server packages.
6. Write unit tests in `pkg/metrics/metrics_test.go` and `pkg/proxy/proxy_test.go`.
7. Run full test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- GET `/metrics` returns HTTP status 200 with `Content-Type: text/plain; version=0.0.4`.
- Prometheus metrics `toron_http_requests_total`, `toron_http_request_duration_seconds`, `toron_active_quic_streams`, and `toron_circuit_breaker_trips_total` are exported correctly.
- Reverse proxy forwards incoming W3C `traceparent` or injects valid `00-<trace_id>-<span_id>-01` headers.
- All Go unit tests pass cleanly.
