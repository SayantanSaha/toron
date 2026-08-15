---
id: TASK-044
type: task
title: Implement WAF Telemetry, Prometheus Metrics, and Structured Security Audit Logging
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-15
updated: 2026-08-15

depends_on:
  - REQ-044

implements:
  - REQ-044

verified_by:
  - TC-044

decided_by:
  - ADR-039

related_to:
  - TASK-019
  - TASK-041
  - TASK-042
  - TASK-043
---

# TASK-044 - Implement WAF Telemetry, Prometheus Metrics, and Structured Security Audit Logging

## Goal

Extend `pkg/metrics` with Prometheus WAF metrics counters and latency histograms, implement a structured JSON security audit logger in `pkg/waf/audit.go`, and integrate into `pkg/waf/middleware.go`, `pkg/config/config.go`, `config.yaml`, and `routes.yaml`.

## Sub-tasks

1. Update `pkg/metrics/metrics.go`:
   - Add `wafBlockedCounters map[string]uint64` (`category`, `route`).
   - Add `wafAnomalyCounters map[string]uint64` (`category`, `mode`).
   - Add `wafHistograms *Histogram` for WAF inspection duration in seconds.
   - Implement `RecordWAFBlocked(category, route string)`.
   - Implement `RecordWAFAnomaly(category, mode string)`.
   - Implement `RecordWAFInspectionDuration(durationSec float64)`.
   - Update `ExportPrometheus()` to emit `toron_waf_blocked_requests_total`, `toron_waf_anomalies_detected_total`, and `toron_waf_inspection_duration_seconds`.
   - Update `GetSummaryJSON()` with WAF telemetry counters.
2. Create `pkg/waf/audit.go`:
   - Define `SecurityEvent` struct (timestamp, event, client_ip, method, path, category, rule_id, score, action, location, payload_snippet).
   - Define `AuditLogger` interface / struct supporting output to `io.Writer` or file with thread-safe `LogEvent(event SecurityEvent)`.
   - Define `AuditLogConfig` (`Enabled bool`, `Output string`, `Format string`).
3. Update `pkg/waf/waf.go` & `pkg/waf/middleware.go`:
   - Extend `WAFConfig` with `AuditLog AuditLogConfig`.
   - Integrate metrics recording and audit logging on IP ACL blocks, protocol integrity failures, OWASP rule blocks, and detection anomalies.
4. Update `pkg/config/config.go`:
   - Add `AuditLog` configuration struct in `WAFConfig`.
5. Update `config.yaml` and `routes.yaml` with example audit logging settings.
6. Write unit tests:
   - `pkg/metrics/metrics_test.go` for WAF counters and Prometheus export.
   - `pkg/waf/audit_test.go` for JSON formatting and sink writing.
   - `pkg/waf/middleware_test.go` for metrics and logging invocation.
7. Execute full test suite `go test ./...` and configuration dry-run (`.\toron -t`).

## Acceptance Criteria

- WAF block and detection events increment Prometheus metrics counters correctly.
- Security audit events write structured, valid JSON entries containing client IP, rule ID, category, location, and payload snippet.
- Prometheus `/metrics` outputs all WAF metrics cleanly.
- 100% Go unit test pass rate across all packages.
