---
id: TASK-162
type: task
title: Implement Distinct Upstream Route Pool Rendering, Hierarchical Telemetry Aggregation, and Live Metric Derivation in Dashboard
status: approved
version: 1.1

project: PROJECT-001
owner: development-lead

created: 2026-09-22
updated: 2026-09-23

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

# TASK-162 - Implement Distinct Upstream Route Pool Rendering, Hierarchical Telemetry Aggregation, and Live Metric Derivation in Dashboard

## 1. Overview & Objective

Decompose [`REQ-139`](../requirements/REQ-139.md) (v1.1) into concrete, testable frontend engineering deliverables in [`public/app.js`](../../public/app.js).

During live operational audits of the Toron Edge Gateway Control Center dashboard at `https://sayantansaha.in/internal/dashboard/#/upstreams`, several critical operational observability defects were identified:
1. **Upstream Route Collapse Anti-Pattern**: Multiple upstream proxy routes targeting the same backend socket (e.g., `/kite/callback`, `/kite/api`, `/kite/broker`, `/kite/auth` all proxying to `http://127.0.0.1:8080`) are collapsed into the first declared route (`/kite/callback`). Subsequent routes are omitted from the Upstreams view (`#/upstreams`) because pool grouping and instance registration logic conflates target host:port with distinct route pools.
2. **Telemetry Field Mismatch & Static Metric Fallbacks**: Metric calculation functions in `buildDataModel()` query non-existent property names (`metrics.by_route`, `metrics.rate_per_sec`, `metrics.latency_p95_ms`, `metrics.err_rate`) instead of the schema exposed by [`pkg/metrics/metrics.go`](../../pkg/metrics/metrics.go) and [`pkg/server/internal_api.go`](../../pkg/server/internal_api.go). This causes route and pool metrics to silently fall back to static defaults (`0.0/s`, `2.5 ms`, `0.0%`), blinding operators to live traffic distribution and upstream latency.
3. **Subpath Fragmentation in Telemetry**: In [`pkg/router/router.go`](../../pkg/router/router.go), requests are recorded in `requests_by_route` keyed by the full incoming request path (e.g., `/kite/api/v1/business`, `/kite/api/v1/trades`, `/kite/api/v1/auth`). Exact key lookups (`requests_by_route[r.prefix]`) return `0` or `1`, omitting subpath requests. Telemetry requires **hierarchical prefix aggregation**: summing all keys `k` where `k === prefix || k.startsWith(prefix + '/')`.
4. **Internal Polling Dilution of External Throughput**: Over 91% (48,000+ of 53,000+) of recorded gateway requests are internal dashboard polling requests (`/internal/api/*`). Dividing route requests by total requests dilutes external route traffic ratios by an order of magnitude, falsely rounding external RPS down to `0.0/s`. Internal requests must be excluded from the denominator.
5. **Instantaneous RPS Oscillation**: Dashboard polling executes every 2 seconds, causing instantaneous `tsA.rps` points to alternate between `0` and `6` req/s due to bursty polling intervals. A smoothed moving average (`avg(tsA.rps.slice(-5))`) is required to present a stable real-time rate.
6. **Cumulative Volume Visibility**: When an upstream is legitimately idle (`0.0/s`), operators cannot distinguish between a healthy idle backend and an unrouted service. Displaying cumulative request volume alongside live RPS in upstream cards (e.g., `0.0/s (1.7k total)`) provides essential operational context.

This task resolves these defects across `buildDataModel()`, `poolCard(p)`, `rtUpdate(D)`, and `flow(host, D)` in [`public/app.js`](../../public/app.js).

---

## 2. Traceability

- **Requirement**: [`REQ-139`](../requirements/REQ-139.md) (Distinct Upstream Route Pool Rendering, Hierarchical Telemetry Aggregation, and Live Metric Derivation in Control Center Dashboard)
- **Architecture Decision**: [`ADR-139`](../architecture/ADR-139.md) (Dashboard Route Pool Disaggregation and Proportional Telemetry Derivation Architecture)
- **Verification Suite**: [`TC-139`](../testCases/TC-139.md) (Verification Test Cases for Distinct Route Pool Rendering and Live Telemetry Derivation)

---

## 3. Engineering Breakdown

All engineering modifications will be applied to [`public/app.js`](../../public/app.js).

### 3.1 Distinct Pool Matching & Unique `poolKey` Generation (`buildDataModel`)
- **Target Function**: `buildDataModel()` (Step 3: Upstream Pools construction).
- **Current Defect**:
  - `matchingRoutes` filters routes matching `targetAddr` or `r.targets.some(...)` with an OR condition, matching all routes sharing the same backend socket.
  - `primaryRoute` is assigned `matchingRoutes[0]` (the first route), and `poolKey` is set to `primaryRoute.id`.
  - Subsequent routes targeting that address are absorbed into the first route's pool and skipped in subsequent registration loops.
- **Implementation Work**:
  1. **Strict Route Priority Matching**:
     - Match health probe entries `u` from `rawApiUpstreams` by route label first:
       ```javascript
       const routeMatches = routes.filter(r => {
         if (!u.route) return false;
         return r.short === u.route || r.path === u.route || (r.host + r.path) === u.route ||
                r.short.includes(u.route) || (r.host + r.path).includes(u.route);
       });
       ```
     - Fall back to target address matching (`targetAddr` / `u.name` against `r.pool` or `r.targets`) only if `routeMatches` is empty.
  2. **Unique Route-Scoped `poolKey`**:
     - Key pools by the specific route ID when a route match is established:
       ```javascript
       const primaryRoute = routeMatches.length > 0 ? routeMatches[0] : matchingRoutes[0];
       const poolKey = primaryRoute ? primaryRoute.id : (u.route ? u.route.replace(/[^a-zA-Z0-9]/g, '_') : targetAddr);
       ```
     - For each distinct route, associate only that route in `pool.routes: [primaryRoute]`.
  3. **Distinct Instance Registration**:
     - Key instances in `pool.insts` within the pool scope so each route pool maintains its dedicated instance record (`targetAddr`, `latency`, `state`, `history`).
  4. **Exhaustive Fallback Route Loop**:
     - In the secondary routes loop (`routes.forEach(r => ...)`), check `if (!poolMap.has(r.id))`.
     - Instantiate an independent pool card for any proxy route (`r.type !== 'static'` and has targets) not yet registered in `poolMap`, ensuring routes such as `/kite/api`, `/kite/broker`, and `/kite/auth` are guaranteed independent pool representation.

### 3.2 Hierarchical Prefix Aggregation & Internal Polling Exclusion (`buildDataModel`)
- **Target Function**: `buildDataModel()` (Step 1: Routes list initialization).
- **Current Defect**:
  - Exact prefix lookup `m.by_route[r.prefix]` returns 0 because [`pkg/router/router.go`](../../pkg/router/router.go) records requests by full subpath.
  - Internal polling requests (`/internal/api/*`) inflate `total_requests`, diluting user route ratios.
- **Implementation Work**:
  1. **Subpath Prefix Aggregation**:
     - Define helper to aggregate cumulative requests for a route prefix across all recorded subpaths:
       ```javascript
       const reqByRoute = (rawApiStatus && rawApiStatus.metrics && rawApiStatus.metrics.requests_by_route) || {};
       function getRouteTotalReqs(prefix) {
         if (!prefix) return 0;
         return Object.entries(reqByRoute).reduce((acc, [k, count]) => {
           if (k === prefix || k.startsWith(prefix + '/')) {
             return acc + (Number(count) || 0);
           }
           return acc;
         }, 0);
       }
       ```
     - Store aggregated cumulative requests on the route:
       ```javascript
       const totalReqs = getRouteTotalReqs(r.prefix);
       ```
  2. **Internal Polling Isolation & Denominator Normalization**:
     - Calculate external requests denominator by excluding all `/internal/` keys:
       ```javascript
       const totalExternalRequests = Object.entries(reqByRoute).reduce((acc, [k, count]) => {
         if (!k.startsWith('/internal/')) {
           return acc + (Number(count) || 0);
         }
         return acc;
       }, 0);
       ```
     - Calculate external route ratio:
       ```javascript
       const routeRatio = totalExternalRequests > 0 ? (totalReqs / totalExternalRequests) : 0;
       ```

### 3.3 Smoothed Moving Average RPS Derivation (`buildDataModel`)
- **Target Function**: `buildDataModel()` (Step 1: Routes list initialization & Step 3: Upstream pools aggregation).
- **Current Defect**:
  - Instantaneous `tsA.rps[tsA.rps.length - 1]` oscillates between 0 and 6 due to 2-second dashboard polling bursts.
- **Implementation Work**:
  1. **Moving Average Gateway RPS**:
     - Calculate smoothed throughput base using the last 5 time-series points:
       ```javascript
       const tsA = rawApiStatus && rawApiStatus.timeseries && rawApiStatus.timeseries.A;
       let smoothedRPS = 0;
       if (tsA && tsA.rps && tsA.rps.length > 0) {
         const recentPoints = tsA.rps.slice(-5);
         smoothedRPS = avg(recentPoints);
       }
       ```
  2. **Proportional Route RPS**:
     - Compute route instantaneous throughput:
       ```javascript
       let curRPS = 0;
       if (totalExternalRequests > 0 && smoothedRPS > 0) {
         curRPS = smoothedRPS * routeRatio;
       } else if (smoothedRPS > 0 && rawApiRoutes.length > 0) {
         curRPS = smoothedRPS / rawApiRoutes.length;
       }
       ```
     - Attach `totalReqs` and `curRPS` to route data model:
       ```javascript
       return {
         ...
         totalReqs,
         base: curRPS,
         cur: { rps: curRPS, totalReqs, p50: lat * 0.5, p95: lat, p99: lat * 1.5, e4: 0, e5: 0, err5: err, err4: 0 }
       };
       ```
  3. **Pool RPS & Total Requests Aggregation**:
     - In `pools = Array.from(poolMap.values()).map(p => ...)`:
       ```javascript
       const rps = sum(p.routes.map(r => r.cur.rps));
       const totalReqs = sum(p.routes.map(r => r.totalReqs || 0));
       return { ...p, insts, rps, totalReqs, p95, err5, up, down, tone };
       ```

### 3.4 Upstream Pool Card Header Volume Rendering (`poolCard`)
- **Target Function**: `poolCard(p)` in [`public/app.js`](../../public/app.js).
- **Current Defect**:
  - Requests header renders only `${fmt.n(p.rps)}/s`. When traffic is idle, operators see `0.0/s` without knowing if the backend has ever handled traffic.
- **Implementation Work**:
  - Update `<dl class="pool-sum">` in `poolCard(p)` to render both live rate and cumulative volume:
    ```javascript
    <div>
      <dt>Requests</dt>
      <dd>${fmt.n(p.rps)}/s ${p.totalReqs > 0 ? `<span class="mut" style="font-size:11px;font-weight:400">(${fmt.n(p.totalReqs)} total)</span>` : ''}</dd>
    </div>
    ```

### 3.5 Live Health Probe Latency & Percentile Derivation (`buildDataModel` & `poolCard`)
- **Target Functions**: `buildDataModel()` and `poolCard(p)`.
- **Implementation Work**:
  1. **Route Latency**: Extract latest p95 from `tsA.p95` (`tsA.p95[tsA.p95.length - 1]`) or average of probe latencies for matching upstreams.
  2. **Instance Row Latency Binding**: Ensure `i.latency` (derived from `/internal/api/upstreams/health` `u.latency_ms`) is formatted directly:
     ```javascript
     const latStr = (i.latency && i.latency > 0) ? `${i.latency.toFixed(1)} ms` : fmt.ms(p.p95);
     ```
  3. **Pool Summary p95**: Derive from active instance probe latencies:
     ```javascript
     const activeProbeLats = insts.map(i => i.latency).filter(l => l > 0);
     const p95 = activeProbeLats.length > 0 ? Math.max(...activeProbeLats) : avg(rs.map(r => r.cur.p95));
     ```

### 3.6 Error Rate & Health State Derivation (`buildDataModel` & `poolCard`)
- **Target Functions**: `buildDataModel()` and `poolCard(p)`.
- **Implementation Work**:
  1. **Instance Health State**:
     ```javascript
     const isDown = u.status === 'UNREACHABLE' || u.status === 'DOWN' || (u.http_code && u.http_code >= 500);
     const isUp = (u.status === 'HEALTHY' || u.status === 'UP') && (!u.http_code || u.http_code < 400);
     const state = isDown ? 'down' : (isUp ? 'up' : 'warn');
     ```
  2. **Error Rate Derivation**:
     ```javascript
     const failedTicks = insts.reduce((acc, i) => acc + (i.history ? i.history.filter(h => !h).length : 0), 0);
     const totalTicks = insts.reduce((acc, i) => acc + (i.history ? i.history.length : 0), 0) || 1;
     const probeFailureRate = failedTicks / totalTicks;
     const err5 = Math.max(avg(rs.map(r => r.cur.err5)), probeFailureRate);
     const tone = (down > 0 || err5 >= 0.02) ? 'err' : (warn > 0 || err5 > 0 ? 'warn' : 'ok');
     ```

### 3.7 Traffic Flow Diagram (Sankey) & Routes View Parity (`flow` & `rtUpdate`)
- **Target Functions**: `flow()` and `rtUpdate()`.
- **Implementation Work**:
  1. **Overview Sankey Disaggregation (`flow`)**:
     - Map `poolNodes` to disaggregated pools (`r.id === p.id || (r.pool && (r.pool === p.id || r.pool.includes(p.id)))`).
     - Route ribbons connect to their specific pool node without conflation.
  2. **Routes View Parity (`rtUpdate`)**:
     - Render derived `cur.rps`, `cur.p95`, `cur.err5`, and sparklines for every route row.

---

## 4. Acceptance Criteria

- [ ] **Discrete Pool Card Rendering**: Navigating to `#/upstreams` renders discrete pool cards for `/kite/callback`, `/kite/api`, `/kite/broker`, and `/kite/auth`. No routes targeting `http://127.0.0.1:8080` are collapsed or omitted.
- [ ] **Accurate Route Header Information**: Each pool card header correctly displays its specific route prefix, match host, middleware chips, and target address.
- [ ] **Hierarchical Subpath Prefix Aggregation**: All subpath requests (e.g., `/kite/api/v1/business`, `/kite/api/v1/trades`, `/kite/api/v1/auth`) are rolled up into the parent route prefix (`/kite/api`), populating `totalReqs` accurately.
- [ ] **Internal Polling Isolation**: Internal dashboard polling requests (`/internal/api/*`) are excluded from external throughput denominator calculations, preventing artificial traffic dilution.
- [ ] **Smoothed Moving Average RPS**: Live RPS values in route rows and pool cards utilize a moving average of recent time-series points (`avg(tsA.rps.slice(-5))`), eliminating 0 to 6 RPS oscillations between 2-second polls.
- [ ] **Cumulative Volume Visibility in Pool Cards**: Upstream pool cards render Requests as `${fmt.n(p.rps)}/s ${p.totalReqs > 0 ? \`<span class="mut" style="font-size:11px;font-weight:400">(\${fmt.n(p.totalReqs)} total)</span>\` : ''}`. Idle upstreams display their historical volume (e.g. `0.0/s (1.7k total)`).
- [ ] **Live Health Probe Latency**: Each upstream instance row reflects real round-trip probe latency (`latency_ms`) returned by `/internal/api/upstreams/health`. Static `2.5 ms` defaults are eliminated when live data is present.
- [ ] **Health Status & Tone Escalation**: Failing probes or 5xx responses trigger `down` instance state, non-zero error rate, and visual `warn` or `err` tone escalation.
- [ ] **Sankey Traffic Flow Ribbon Parity**: The Overview Sankey diagram renders distinct links and pool blocks for each proxy route without ribbon collision or route conflation.
- [ ] **Zero Console Exceptions**: Clean browser console with zero runtime `TypeError`, `NaN`, or unhandled promise rejections.
- [ ] **Backend Test Parity**: All backend Go test suites (`go test ./...`) pass without regressions.

---

## 5. Constraints & Project Standards

- **Strictly Relative Links**: All document links must be strictly relative (`../...` or `../../...`). No absolute filesystem paths (`file://` or leading slashes).
- **Zero External Dependencies**: Pure vanilla JavaScript (ES6+) and native SVG in [`public/app.js`](../../public/app.js). No bundlers, npm packages, or external frameworks.
- **Performance Budget**: `buildDataModel()` must execute in under 2 milliseconds ($\mathcal{O}(R + U)$) during 1-second dashboard polling cycles.
- **Defensive Fallbacks**: Handle empty arrays and missing telemetry fields gracefully without raising runtime errors.
