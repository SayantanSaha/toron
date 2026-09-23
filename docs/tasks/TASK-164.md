---
id: TASK-164
type: task
title: Deconstruct Monolithic Dashboard into Native Browser ECMAScript Modules
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-23
updated: 2026-09-23

depends_on:
  - REQ-141

derived_from:
  - REQ-141

implements:
  - REQ-141

verified_by:
  - TC-141

decided_by:
  - ADR-141

related_to:
  - REQ-141
  - ADR-141
  - TC-141
  - TASK-162
  - TASK-163
---

# TASK-164 - Deconstruct Monolithic Dashboard into Native Browser ECMAScript Modules

## 1. Overview & Objective

Decompose [`REQ-141`](../requirements/REQ-141.md) into concrete, testable work packages to transition the Toron Edge Gateway Control Center dashboard from a monolithic 1,663-line script ([`public/app.js`](../../public/app.js)) into a clean, modular hierarchy of native browser ECMAScript Modules under `public/js/`.

The primary deliverables:
1. Establish a native ES module hierarchy without introducing any bundler, compiler, or npm runtime dependencies.
2. Maintain 100% behavioral, visual, and operational telemetry parity across all 8 views and 2 inspector drawers.
3. Update [`public/index.html`](../../public/index.html) to load `<script type="module" src="./js/app.js"></script>`.
4. Ensure test runner ([`tests/dashboard_telemetry_test.js`](../../tests/dashboard_telemetry_test.js)) continues to pass with 100% fidelity.

---

## 2. Traceability

- **Requirement**: [`REQ-141`](../requirements/REQ-141.md) (Modularization of Toron Gateway Observability Dashboard into Native ES Modules)
- **Architecture Record**: [`ADR-141`](../architecture/ADR-141.md) (Native ECMAScript Modules Architecture for Control Center Dashboard)
- **Verification Plan**: [`TC-141`](../testCases/TC-141.md) (Verification of Modular ES Dashboard Architecture and Feature Parity)

---

## 3. Work Package Breakdown

### 3.1 WP-1: Utilities, State Machine & Icon Renderers
- Create `public/js/utils.js`:
  - DOM selection shortcuts (`$`, `$$`).
  - HTML entity escaping (`esc`).
  - Numeric, latency, percentage, and byte formatters (`fmt`, `axis`).
  - Datetime formatters (`dtFmt`, `hms`, `hm`, `ago`).
  - Array and mathematical utilities (`clamp`, `sum`, `avg`, `H`).
- Create `public/js/state.js`:
  - Central reactive state store (`state`).
  - Time range configurations (`RANGES`, `N`).
  - Cache invalidation and force refresh flags (`invalidate`, `getCache`, `setCache`).
  - Theme detection and toggling engine (`toggleTheme`, `themeIcon`, `effTheme`).
- Create `public/js/components/icons.js`:
  - SVG icon generator (`ICON`).
  - Tone mappings (`TI`, `TONE`, `toneErr`, `pill`).

### 3.2 WP-2: Data Ingestion & Metric Modeling
- Create `public/js/api.js`:
  - Asynchronous background polling via `Promise.allSettled`.
  - State storage for raw telemetry payloads (`rawApiStatus`, `rawApiRoutes`, `rawApiUpstreams`, `rawApiIncidents`, `rawApiLogs`, `rawApiBannedIps`).
  - Error suppression and version badge updates.
- Create `public/js/model.js`:
  - In-memory data model aggregation (`buildDataModel`).
  - Hierarchical subpath rollup for route request counts (`getRouteTotalReqs`).
  - Internal polling traffic isolation from user rate calculations (`totalExternalRequests`).
  - Moving-average temporal smoothing for gateway RPS.
  - Disaggregated upstream route pool generation and instance isolation.
  - Dynamic probe latency binding, dynamic p95 calculations, and health tone escalation.

### 3.3 WP-3: Shared Visual Components & Drawers
- Create `public/js/components/charts.js`:
  - Vector SVG sparkline generator (`spark`).
  - Dual time-series canvas/SVG curve renderer (`chart`).
  - Latency distribution histogram renderer (`histo`).
- Create `public/js/components/sankey.js`:
  - Native vector SVG Sankey traffic flow diagram (`flow`).
  - Ribbon path geometry and node layout calculations.
- Create `public/js/components/drawer.js`:
  - Slide-out inspector drawer manager (`openDrawer`, `closeDrawer`, `renderDrawer`, `dw`).
  - Route detail drawer rendering (SLO target curves, error distribution, middlewares).
  - Request trace waterfall timeline rendering.

### 3.4 WP-4: Dedicated View Modules
Partition view implementations into `public/js/views/`:
- `overview.js`: `ovShell()`, `ovUpdate()`, operational health banner, signal matrix.
- `routes.js`: `rtShell()`, `rtInit()`, `rtUpdate()`, route table filtering, sorting.
- `upstreams.js`: `upShell()`, `upUpdate()`, `poolCard()` with 48-tick probe history strips.
- `logs.js`: `lgShell()`, `lgInit()`, `lgUpdate()`, `lgSync()`, request tail stream.
- `certs.js`: `ceShell()`, `ceUpdate()`, ACME TLS certificates and expiration progress bars.
- `modules.js`: `mdShell()`, `mdUpdate()`, compiled reactors and Go runtime metrics.
- `alerts.js`: `alShell()`, `alUpdate()`, WAF security incidents and auto-ban IP manager.
- `console.js`: `consoleShell()`, `consoleInit()`, interactive diagnostic endpoint runner.

### 3.5 WP-5: Router, Orchestration & Entrypoint
- Create `public/js/app.js`:
  - View registry (`VIEWS`, `META`, `NAV`).
  - Navigation rendering (`renderNav()`, `head()`, `mountView()`, `chrome()`).
  - Hash routing engine (`go()`, `activate()`).
  - Live mode toggling (`setLive()`).
  - Global event listener registration (clicks, keyboard navigation, drawer scrim).
  - Bootstrap sequence (`boot()`) and 2000 ms polling interval.
- Update `public/index.html`:
  - Update script tag to `<script type="module" src="./js/app.js"></script>`.
  - Maintain backwards-compatible fallback or bundle reference if needed.

### 3.6 WP-6: Test Suite Compatibility & Verification
- Update `tests/dashboard_telemetry_test.js`:
  - Ensure Node.js runner can load and test the modular architecture.
  - Verify all TC-139 and TC-140 test cases pass against the modular files.
  - Enforce strictly relative links across all module imports and documentation.

---

## 4. Dependencies & Constraints

- Strictly NO external build tools (no webpack, vite, or rollup).
- Strictly NO external npm runtime libraries.
- Strictly relative imports (`from './utils.js'`, `from '../components/icons.js'`).
- Preserves the ~15 MB Docker appliance image and zero-allocation Go runtime.
