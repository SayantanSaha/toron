---
id: TASK-158
type: task
title: Implementation of Next-Generation Observability Dashboard and Zero-Allocation Telemetry Time-Series Engine
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-21
updated: 2026-09-21

depends_on:
  - REQ-135

derived_from:
  - REQ-135

implements:
  - REQ-135

verified_by:
  - TC-135

decided_by:
  - ADR-135

related_to:
  - REQ-135
  - ADR-135
  - TC-135
  - TASK-075
  - TASK-121
---

# TASK-158 - Implementation of Next-Generation Observability Dashboard and Zero-Allocation Telemetry Time-Series Engine

## 1. Overview & Objective

Decompose [`REQ-135`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-135.md) into concrete engineering deliverables:
1. **Frontend Asset Overhaul (`public/index.html`, `public/style.css`, `public/app.js`)**:
   - Replace Traefik tabbed UI with the high-density layout featuring a sticky left rail, topbar breadcrumbs, main view container, and slide-over inspector drawer.
   - Implement pure-SVG chart rendering engine (Sankey traffic flow ribbons, latency percentiles, 4xx/5xx error bars, sparklines, histograms, and 48-tick health strips).
   - Implement all 7 primary dashboard views: Overview, Routes, Upstreams, Requests (Live Logs), Certificates, Modules & Runtime, and Alerts.
   - Preserve and integrate the interactive API debugger (`/internal/api/proxy-test`).
   - Support Light, Dark, and System theme switching.
   - Ensure contextual HTML entity escaping (`escapeHTML`) on all dynamic DOM injections to prevent XSS (CWE-79).
2. **Backend Engine & API Telemetry (`pkg/metrics`, `pkg/server`)**:
   - Create preallocated, zero-allocation rolling 60-bucket time-series metric collector in `pkg/metrics/metrics.go` for `rps`, `p50`, `p95`, `p99`, `4xx`, `5xx`, and Go runtime gauges (`cpu`, `heap`, `goroutines`, `gc_p99`, `open_fds`).
   - Implement upstream 48-tick health history bitmask/buffer.
   - Implement in-memory circular request log and trace span buffer (300 items) in `pkg/server/internal_api.go`.
   - Expose certificate lifecycle states and Go runtime stats in `GET /internal/api/status`.
   - Maintain authentication and CIDR subnet restrictions on `/internal/api/*`.

---

## 2. Acceptance Criteria

- [ ] All 7 dashboard views render properly and update via periodic live polling.
- [ ] SVG Sankey diagram correctly maps Listeners $\to$ Routes $\to$ Upstreams.
- [ ] Time-series charts display p50/p95/p99 latency curves and stacked error volumes.
- [ ] Route drawer displays p95 against SLO and latency distribution histogram.
- [ ] Requests view tails live logs and displays waterfall trace spans upon selection.
- [ ] Upstreams view displays 48-tick health history strips per instance.
- [ ] API console successfully dispatches probes and renders responses.
- [ ] All dynamic values in the UI are sanitized via `escapeHTML()`.
- [ ] Backend metrics updates introduce zero memory allocations on the hot proxy path.
- [ ] Comprehensive unit tests verify serialization and telemetry aggregation (`go test -race -count=1 ./...`).

---

## 3. Constraints & Dependencies

- Zero external JavaScript/CSS libraries (pure vanilla ES6+ and native SVG).
- Complete isolation between telemetry collection and data-plane forwarding.
