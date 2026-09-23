---
id: TASK-162
type: task
title: Implement Distinct Upstream Route Pool Rendering and Live Telemetry Metric Derivation in Dashboard
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-22
updated: 2026-09-22

depends_on:
  - REQ-139

derived_from:
  - REQ-139

implements:
  - REQ-139

verified_by:
  - TC-139

decided_by:
  - ADR-139

related_to:
  - REQ-139
  - ADR-139
  - TC-139
  - REQ-135
  - REQ-137
  - REQ-138
  - TASK-158
  - TASK-160
  - TASK-161
---

# TASK-162 - Implement Distinct Upstream Route Pool Rendering and Live Telemetry Metric Derivation in Dashboard

## 1. Overview & Objective

Decompose [`REQ-139`](../requirements/REQ-139.md) into concrete, testable frontend engineering deliverables in [`public/app.js`](../../public/app.js).

Currently, the Toron Edge Gateway Control Center dashboard exhibits two operational observability defects:
1. **Upstream Route Collapse Anti-Pattern**: Multiple upstream proxy routes targeting the same backend socket (e.g., `/kite/callback`, `/kite/api`, `/kite/broker`, `/kite/auth` all proxying to `http://127.0.0.1:8080`) are collapsed into the first declared route (`/kite/callback`). Subsequent routes are omitted from the Upstreams view (`#/upstreams`) because pool grouping and instance registration logic conflates target host:port with distinct route pools.
2. **Telemetry Field Mismatch & Static Metric Fallbacks**: Metric calculation functions in `buildDataModel()` query non-existent property names (`metrics.by_route`, `metrics.rate_per_sec`, `metrics.latency_p95_ms`, `metrics.err_rate`) instead of the schema exposed by [`pkg/metrics/metrics.go`](../../pkg/metrics/metrics.go) and [`pkg/server/internal_api.go`](../../pkg/server/internal_api.go). This causes route and pool metrics to silently fall back to static defaults (`0.0/s`, `2.5 ms`, `0.0%`), blinding operators to live traffic distribution and upstream latency.

This task resolves both issues by disaggregating route pools, prioritizing route-label matching over socket addresses, and dynamically deriving RPS, p95 latency, and 5xx error rates from live telemetry payloads.

---

## 2. Traceability

- **Requirement**: [`REQ-139`](../requirements/REQ-139.md) (Distinct Upstream Route Pool Rendering and Live Telemetry Metric Derivation in Control Center Dashboard)
- **Architecture Decision**: [`ADR-139`](../architecture/ADR-139.md) (Dashboard Route Pool Disaggregation and Proportional Telemetry Derivation Architecture)
- **Verification Suite**: [`TC-139`](../testCases/TC-139.md) (Verification Test Cases for Distinct Route Pool Rendering and Live Telemetry Derivation)

---

## 3. Engineering Breakdown

All engineering modifications will be applied to [`public/app.js`](../../public/app.js).

### 3.1 Distinct Pool Matching & Unique `poolKey` Generation (`buildDataModel`)
- **Target Function**: `buildDataModel()` (Step 3: Upstream Pools construction).
- **Current Defect**:
  - `matchingRoutes` filters routes matching `targetAddr` or `r.targets.some(...)` with an OR condition, matching all routes sharing the same backend port.
  - `primaryRoute` is assigned `matchingRoutes[0]` (the first route), and `poolKey` is set to `primaryRoute.id`.
  - All subsequent routes sharing that target are grouped into the same pool and skipped in subsequent loops.
- **Implementation Work**:
  1. **Strict Route Priority Matching**:
     - When iterating through `rawApiUpstreams`, match health probe entries `u` by route label first:
       ```javascript
       const routeMatches = routes.filter(r => {
         if (!u.route) return false;
         return r.short === u.route || r.path === u.route || (r.host + r.path) === u.route ||
                r.short.includes(u.route) || (r.host + r.path).includes(u.route);
       });
       ```
     - Only if `routeMatches` is empty should fallback matching on `targetAddr` / `u.name` be performed against `r.pool` or `r.targets`.
  2. **Unique Route-Scoped `poolKey`**:
     - Key pools by the specific route ID when a route match is established:
       ```javascript
       const primaryRoute = routeMatches.length > 0 ? routeMatches[0] : matchingRoutes[0];
       const poolKey = primaryRoute ? primaryRoute.id : (u.route ? u.route.replace(/[^a-zA-Z0-9]/g, '_') : targetAddr);
       ```
     - For each distinct route, associate only that route in `pool.routes: [primaryRoute]`.
  3. **Distinct Instance Registration**:
     - Add probe instances to `pool.insts` keyed by route/pool scope so each route pool maintains its dedicated instance record (`targetAddr`, `latency`, `state`, `history`).
  4. **Exhaustive Fallback Route Loop**:
     - In the secondary routes loop (`routes.forEach(r => ...)`), check `if (!poolMap.has(r.id))`.
     - Instantiate an independent pool for any proxy route (`r.type !== 'static'` and has targets) not yet registered in `poolMap`, ensuring routes such as `/kite/api`, `/kite/broker`, and `/kite/auth` are guaranteed independent pool representation.

### 3.2 Dynamic Requests Rate (RPS) Derivation (`buildDataModel`)
- **Target Function**: `buildDataModel()` (Step 1: Routes list initialization & Step 3: Upstream pools aggregation).
- **Current Defect**:
  - References `rawApiStatus.metrics.by_route` and `rawApiStatus.metrics.rate_per_sec`, both of which are `undefined`.
- **Implementation Work**:
  1. **Schema Alignment with `pkg/metrics/metrics.go`**:
     - Replace `m.by_route` with `rawApiStatus.metrics.requests_by_route`.
     - Replace `m.rate_per_sec` with instantaneous total RPS from `rawApiStatus.timeseries.A.rps`.
  2. **Proportional Route RPS Calculation**:
     - Extract latest gateway instantaneous RPS:
       ```javascript
       const tsA = rawApiStatus && rawApiStatus.timeseries && rawApiStatus.timeseries.A;
       const totalRPS = (tsA && tsA.rps && tsA.rps.length > 0) ? (tsA.rps[tsA.rps.length - 1] || 0) : 0;
       ```
     - Extract cumulative route requests and total requests:
       ```javascript
       const reqByRoute = (rawApiStatus && rawApiStatus.metrics && rawApiStatus.metrics.requests_by_route) || {};
       const routeCumulative = reqByRoute[r.prefix] || reqByRoute[r.path] || 0;
       const totalRequests = (rawApiStatus && rawApiStatus.metrics && rawApiStatus.metrics.total_requests) || 0;
       ```
     - Compute instantaneous route throughput:
       ```javascript
       let curRPS = 0;
       if (totalRequests > 0 && totalRPS > 0) {
         curRPS = totalRPS * (routeCumulative / totalRequests);
       } else if (totalRPS > 0 && rawApiRoutes.length > 0) {
         curRPS = totalRPS / rawApiRoutes.length;
       }
       ```
     - If `routeCumulative === 0` and gateway has recorded other traffic, assign `curRPS = 0.0`.
  3. **Pool RPS Aggregation**:
     - In `pools.map(p => ...)`:
       ```javascript
       const rps = sum(p.routes.map(r => r.cur.rps));
       ```
     - Ensure pool cards display `${fmt.n(p.rps)}/s`.

### 3.3 Live Health Probe Latency & Percentile Derivation (`buildDataModel` & `poolCard`)
- **Target Functions**: `buildDataModel()` and `poolCard(p)`.
- **Current Defect**:
  - `m.latency_p95_ms` lookup yields `undefined`, falling back to constant `lat = 2.5`.
  - `poolCard(p)` displays hardcoded fallback or uncalibrated p95 values.
- **Implementation Work**:
  1. **Route Latency Derivation**:
     - In `buildDataModel()`, read latest p95 from time-series:
       ```javascript
       let lat = (tsA && tsA.p95 && tsA.p95.length > 0) ? tsA.p95[tsA.p95.length - 1] : 2.5;
       ```
     - If health probe data exists for route instances, compute latency from probe response:
       ```javascript
       const probeLatencies = matchingUpstreams.map(u => u.latency_ms).filter(l => l > 0);
       if (probeLatencies.length > 0) {
         lat = avg(probeLatencies);
       }
       ```
  2. **Instance Row Latency Binding in `poolCard(p)`**:
     - Ensure `i.latency` (derived from `/internal/api/upstreams/health` `u.latency_ms`) is formatted directly:
       ```javascript
       const latStr = (i.latency && i.latency > 0) ? `${i.latency.toFixed(1)} ms` : fmt.ms(p.p95);
       ```
  3. **Pool Summary p95 Derivation**:
     - Pool p95 must reflect active instance probe latencies:
       ```javascript
       const activeProbeLats = insts.map(i => i.latency).filter(l => l > 0);
       const p95 = activeProbeLats.length > 0 ? Math.max(...activeProbeLats) : avg(rs.map(r => r.cur.p95));
       ```

### 3.4 Error Rate & Health State Derivation (`buildDataModel` & `poolCard`)
- **Target Functions**: `buildDataModel()` and `poolCard(p)`.
- **Current Defect**:
  - `m.err_rate` lookup yields `undefined`, falling back to `0.0`.
  - Upstream instance state only checked `u.status === 'HEALTHY' || u.status === 'UP'`, missing `UNREACHABLE` or HTTP 5xx codes.
- **Implementation Work**:
  1. **Instance Health State Classification**:
     - Evaluate both HTTP status codes and probe status strings:
       ```javascript
       const isDown = u.status === 'UNREACHABLE' || u.status === 'DOWN' || (u.http_code && u.http_code >= 500);
       const isUp = (u.status === 'HEALTHY' || u.status === 'UP') && (!u.http_code || u.http_code < 400);
       const state = isDown ? 'down' : (isUp ? 'up' : 'warn');
       ```
  2. **Route Error Rate Derivation**:
     - Extract latest gateway error rate from `tsA.err[tsA.err.length - 1]`.
     - Calculate cumulative 5xx error percentage from `rawApiStatus.metrics.requests_by_status`:
       ```javascript
       const statusCounts = (rawApiStatus && rawApiStatus.metrics && rawApiStatus.metrics.requests_by_status) || {};
       const err5xxCount = Object.entries(statusCounts).filter(([k]) => k.startsWith('5')).reduce((sum, [, c]) => sum + c, 0);
       const errRateFromStatus = totalRequests > 0 ? (err5xxCount / totalRequests) : 0;
       const err = (tsA && tsA.err && tsA.err.length > 0) ? tsA.err[tsA.err.length - 1] : errRateFromStatus;
       ```
  3. **Pool Error Rate & Visual Tone Escalation**:
     - Compute pool error rate incorporating failing health probe ticks:
       ```javascript
       const failedTicks = insts.reduce((acc, i) => acc + (i.history ? i.history.filter(h => !h).length : 0), 0);
       const totalTicks = insts.reduce((acc, i) => acc + (i.history ? i.history.length : 0), 0) || 1;
       const probeFailureRate = failedTicks / totalTicks;
       const err5 = Math.max(avg(rs.map(r => r.cur.err5)), probeFailureRate);
       const tone = (down > 0 || err5 >= 0.02) ? 'err' : (warn > 0 || err5 > 0 ? 'warn' : 'ok');
       ```

### 3.5 Traffic Flow Diagram (Sankey) & Routes View Parity
- **Target Functions**: `flow()` and `rtUpdate()`.
- **Implementation Work**:
  1. **Overview Sankey Disaggregation (`flow`)**:
     - Map `poolNodes` directly to the disaggregated pool models:
       ```javascript
       const matchingRoutes = routeNodes.filter(r => r.id === p.id || (r.pool && (r.pool === p.id || r.pool.includes(p.id))));
       ```
     - In route-to-pool ribbon connection:
       ```javascript
       let targetPool = poolMap.get(r.id) || poolMap.get(r.pool) || poolNodes.find(p => p.id === r.id);
       ```
     - Ensure each route displays an independent flowing ribbon into its respective upstream pool block.
  2. **Routes View Table Parity (`rtUpdate`)**:
     - Confirm all derived route metrics (`cur.rps`, `cur.p95`, `cur.err5`) render in the Routes table with live sparklines and status badges.

---

## 4. Acceptance Criteria

- [ ] **Distinct Pool Card Generation**: Navigating to `#/upstreams` renders discrete pool cards for `/kite/callback`, `/kite/api`, `/kite/broker`, and `/kite/auth`. No routes targeting `http://127.0.0.1:8080` are collapsed or omitted.
- [ ] **Accurate Route Header Information**: Each pool card header correctly displays its specific route prefix, match host, middleware chips, and target address.
- [ ] **Live RPS Telemetry**: Pool cards and route rows calculate live throughput from `requests_by_route` and `timeseries.A.rps`. Active routes display non-zero, proportional RPS; idle routes display `0.0/s`.
- [ ] **Live Health Probe Latency**: Each upstream instance row reflects real round-trip probe latency (`latency_ms`) returned by `/internal/api/upstreams/health`. Static `2.5 ms` defaults are eliminated when live data is present.
- [ ] **Health Status & Tone Escalation**: Failing probes or 5xx responses trigger `down` instance state, non-zero error rate, and visual `warn` or `err` tone escalation.
- [ ] **Sankey Traffic Flow Ribbon Parity**: The Overview Sankey diagram renders distinct links and pool blocks for each proxy route without ribbon collision or route conflation.
- [ ] **Zero Console Exceptions**: Clean browser console with zero runtime `TypeError`, `NaN`, or unhandled promise rejections.
- [ ] **Backend Test Parity**: All backend Go test suites (`go test ./...`) pass without regressions.

---

## 5. Constraints & Project Standards

- **Strictly Relative Links**: All document links must be strictly relative (`../...` or `../../...`). No absolute filesystem paths or leading slashes.
- **Zero External Dependencies**: Pure vanilla JavaScript (ES6+) and native SVG in [`public/app.js`](../../public/app.js). No bundlers, npm packages, or external frameworks.
- **Performance Budget**: `buildDataModel()` must execute in under 2 milliseconds ($\mathcal{O}(R + U)$) during 1-second dashboard polling cycles.
- **Defensive Fallbacks**: Handle empty arrays and missing telemetry fields gracefully without raising runtime errors.
