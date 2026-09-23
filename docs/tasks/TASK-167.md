---
id: TASK-167
type: task
title: Implement Dual-Pane Analytics & Time-Window Filtered Paginated Request Explorer
status: in-progress
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-23
updated: 2026-09-23

depends_on:
  - ../requirements/REQ-143.md

derived_from:
  - ../requirements/REQ-143.md
  - ../analysis/AN-003.md

implements:
  - ../requirements/REQ-143.md

verified_by:
  - ../testCases/TC-143.md

decided_by:
  - ../architecture/ADR-143.md

related_to:
  - ../requirements/REQ-143.md
  - ../architecture/ADR-143.md
  - ../testCases/TC-143.md
  - ../analysis/AN-003.md
---

# TASK-167 - Implement Dual-Pane Analytics & Time-Window Filtered Paginated Request Explorer

## 1. Overview & Objective

Decompose [`REQ-143`](../requirements/REQ-143.md) and [`ADR-143`](../architecture/ADR-143.md) into concrete, testable implementation packages to redesign the Requests view ([`public/js/views/logs.js`](../../public/js/views/logs.js)) into a scalable, dual-pane analytics and paginated request explorer.

---

## 2. Work Package Breakdown

### WP-1: State Management Extensions ([`public/js/state.js`](../../public/js/state.js))
- Add state properties:
  - `ltime`: `'1m' | '5m' | '1h' | 'all' | 'custom'` (default `'all'`)
  - `lfrom`: timestamp (ms) or null
  - `lto`: timestamp (ms) or null
  - `lnohealth`: boolean (default `false`)
  - `lsort`: `'ts' | 'status' | 'ms' | 'path'` (default `'ts'`)
  - `lsortDir`: `'desc' | 'asc'` (default `'desc'`)
  - `lpage`: number (default `1`)
  - `lpageSize`: number (default `25`)
  - `lhover`: boolean (default `false`)

### WP-2: Dual-Pane Top Analytics Strip ([`public/js/views/logs.js`](../../public/js/views/logs.js))
- Generate SVG status-distribution mini-histogram across time buckets.
- Render clickable facet cards:
  - Top 3 Routes with request count and click-to-filter action.
  - P95 latency and slowest request endpoint.
  - Overall error rate percentage for the filtered window.

### WP-3: Time-Window & Expressive Filter Toolbar ([`public/js/views/logs.js`](../../public/js/views/logs.js))
- Time window dropdown selector (`1m`, `5m`, `1h`, `All`, `Custom`).
- Custom datetime input fields (start/end) toggled when `custom` is selected.
- `Exclude /health` toggle chip.
- Search input, status chips (`2xx`, `3xx`, `4xx`, `5xx`), `Over SLO`, and route dropdown integration.

### WP-4: Paginated Table, Interactive Sorting & Freeze-on-Hover ([`public/js/views/logs.js`](../../public/js/views/logs.js))
- Interactive sortable column headers with sort direction indicators.
- Capped rendering at `lpageSize` rows per page.
- Pagination controls: First, Prev, Page indicator, Next, Last, Page size selector.
- `mouseenter` / `mouseleave` event listeners on `#lgBody` to set `lhover` and prevent visual layout shifts.

### WP-5: Design System Styles ([`public/style.css`](../../public/style.css))
- Add compact styles for `.lg-analytics`, `.lg-hist`, `.lg-facets`, `.facet-pill`, `.pagination`, and `.sort-btn`.
- Ensure full light and dark mode compatibility and responsive mobile stacking.

### WP-6: Automated Verification Test Suite ([`docs/testCases/TC-143.md`](../testCases/TC-143.md), [`tests/dashboard_telemetry_test.js`](../../tests/dashboard_telemetry_test.js))
- Add comprehensive unit and integration tests covering:
  - Time window filtering (`1m`, `5m`, `1h`, custom range).
  - Health exclusion.
  - Histogram SVG generation.
  - Interactive sorting by latency, status, time, and path.
  - Pagination boundary handling.
  - Total JS bundle size $\le 100\text{ KB}$.
  - Strictly relative links across all artifacts.
