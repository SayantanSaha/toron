---
title: High-Density Gateway Observability Dashboard
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-09-21
updated: 2026-09-28

documents:
  - OBSERVABILITY-DASHBOARD

related_to:
  - ../reference/api.md
  - ../configuration.md
  - ../release-notes.md
  - ./waf.md
  - ./os-level-ip-blocking.md
---

# High-Density Gateway Observability Dashboard

## Overview

Toron provides a high-density, real-time observability control plane served directly from `/internal/dashboard/`. Designed with pure vector SVG graphics, native ECMAScript Modules (`ESM`), and zero external JavaScript dependencies, it delivers instant visibility into traffic topologies, routing distributions, latency percentiles, upstream node health histories, and live request trace waterfalls.

The dashboard client engine (`public/js/app.js`) polls Toron's internal management APIs (`/internal/api/status`, `/internal/api/routes`, and `/internal/api/upstreams/health`) on a 2-second interval, constructing an in-memory client data model with sub-millisecond execution overhead.

---

## Key Dashboard Views

### 1. Overview
- **Live Status Banner**: Operational health status (`All systems operational`, `Degraded`, `Needs attention`) and active incident alerts.
- **Sankey Traffic Flow**: Vector ribbons visualizing throughput (RPS) flowing from listeners (`:443`, `:80`) $\to$ route rules $\to$ backend upstream pools.
- **Key Signals Matrix**: 5 core KPI tiles (Requests/s, p95 Latency, 5xx Rate, Active Connections, Egress Mb/s) with trend sparklines.
- **Dual Time-Series Curves**: Latency percentiles (p50, p95, p99) and stacked 4xx/5xx error volumes over 15m, 1h, 6h, and 24h ranges.

### 2. Routes & Route Detail Drawer
- Sortable and filterable route table showing live RPS sparklines, p95 latency, and error rates.
- Displays match hosts, path prefixes, proxy target destinations, and middleware chips.
- Clicking any route opens the **Route Detail Drawer** displaying latency vs. SLO target reference curves, error breakdowns, latency distribution histograms, and configured middlewares.

### 3. Upstreams & Node Health Histories
- Grouped into distinct upstream pool cards by route identity, showing load-balancing algorithms, summary RPS alongside cumulative traffic volume, p95 latency, and error rates.
- Requests tile renders dual metrics: real-time rate alongside cumulative total requests (e.g. `0.0/s (1.7k total)`).
- Per-instance cards featuring **48-tick visual health check history strips** (green = passing, red = failed).
- Instance rows display real-time probe round-trip latency (`latency_ms`), HTTP response code status, and circuit-breaker warnings.

### 4. Live Requests & Trace Waterfall Drawer
- Real-time tailing request stream with status code chips (`2xx`, `3xx`, `4xx`, `5xx`), "Over SLO" toggle, and route filter.
- Clicking a request opens the **Trace Waterfall Drawer** decomposing execution latency across listener parsing, TLS handshake, routing, token verification, rate limiting, upstream connect, and response streaming.

### 5. Certificates & Modules
- **Certificates**: Let's Encrypt challenge types (HTTP-01, TLS-ALPN-01, DNS-01), days to expiration with progress bars, and renewal failure alerts.
- **Modules & Runtime**: Compiled engine reactors, event bus throughput, and Go runtime stats (Goroutines, Heap MB, GC pause p99, Open FDs).

### 6. API Console
- Interactive diagnostic probe tool calling `POST /internal/api/proxy-test` to test endpoints and inspect response headers and bodies.

### 7. Alerts & Threat Defense Control Center
- **4-Card Security KPI Strip**: Top-level executive metric cards displaying active alerts and unmitigated incidents, recent WAF-blocked attacks, Stage 1 temporary bans, and Stage 2 permanent firewall bans.
- **Decoupled Threat Actor Quarantine**: Dedicated manual IP blocking toolbar with client-side IPv4/IPv6 syntax validation, duration presets (`15m`, `1h`, `6h`, `24h`, `7d`, `Permanent`), custom interval inputs, and incident context reasons.
- **Active Incidents & Forensic Investigation**: Real-time searchable and faceted security incident feed with slide-out forensic drawer detailing OWASP rule IDs, anomaly scores, parameter locations, raw attack payloads, and instant remediation actions.
- **Dynamic 2-Stage Auto-Ban Table**: Sortable, paginated IP firewall table featuring deterministic IP tie-breaking, non-blocking in-app confirmation modal, and floating toast feedback for unban operations.

---

## Disaggregated Upstream Route Pool Rendering

In microservice gateways and reverse proxy configurations, multiple distinct ingress routes frequently proxy to the same physical socket address or loopback container port (for example, `/kite/callback`, `/kite/api`, `/kite/broker`, and `/kite/auth` all forwarding to `http://127.0.0.1:8080`).

To avoid route collapse where multiple business routes are merged into a single backend pool card, the client data engine (`public/app.js`) implements a disaggregated route pooling architecture:

```
[ Ingress Routes ]                     [ Route-First Matching ]             [ Upstream Pool Cards ]
  /kite/callback  --> 127.0.0.1:8080   ==> Match by u.route === prefix  --> Pool Card: _kite_callback
  /kite/api       --> 127.0.0.1:8080   ==> Match by u.route === prefix  --> Pool Card: _kite_api
  /kite/broker    --> 127.0.0.1:8080   ==> Match by u.route === prefix  --> Pool Card: _kite_broker
  /kite/auth      --> 127.0.0.1:8080   ==> Match by u.route === prefix  --> Pool Card: _kite_auth
```

### 1. Route-First Priority Matching
When processing health probe entries (`rawApiUpstreams`) from `/internal/api/upstreams/health`, `buildDataModel()` matches probe records against configured routes by prioritizing explicit route labels (`u.route`):

```javascript
const routeMatches = routes.filter(r => {
  if (!u.route) return false;
  return r.short === u.route || r.path === u.route || (r.host + r.path) === u.route ||
         r.short.includes(u.route) || (r.host + r.path).includes(u.route) ||
         u.route.includes(r.path) || u.route.includes(r.short);
});
```

Generic target address matching (comparing `targetAddr` against `r.pool` or `r.targets`) is only executed as a secondary fallback when `u.route` is empty.

### 2. Deterministic Route-Scoped Pool Keys
Each route receives a unique, route-scoped pool identifier:

```javascript
const primaryRoute = routeMatches.length > 0 ? routeMatches[0] : fallbackMatches[0];
const poolKey = primaryRoute ? primaryRoute.id : (u.route ? u.route.replace(/[^a-zA-Z0-9]/g, '_') : targetAddr.replace(/[^a-zA-Z0-9]/g, '_'));
```

Because `poolKey` is derived from the route ID rather than the backend host:port, each route generates an independent pool card in `#/upstreams`.

### 3. Instance Isolation per Route Pool
Each pool card maintains dedicated instance records (`pool.insts`) containing:
- Target address (`targetAddr`, e.g. `127.0.0.1:8080`)
- Route affiliation label
- Instance health state (`up`, `warn`, `down`)
- Real-time probe response latency (`latency_ms`)
- HTTP probe response status code (e.g. `200`, `502`)
- 48-tick visual health history array (`history`)

Instances targeting the same backend server are isolated within their respective route pool, ensuring separate health history tracking and circuit-breaker alerts.

### 4. Guaranteed Fallback Route Instantiation
An exhaustive secondary registration loop evaluates all configured proxy routes (`r.type !== 'static'` with defined targets). Any route not yet present in `poolMap` is instantiated as an independent pool card, guaranteeing 100% upstream route coverage in the dashboard even when active probe records have not yet arrived.

### 5. Topology Parity Across Views
- **Sankey Traffic Flow (`flow()`)**: Connects route ribbons to their dedicated upstream pool nodes, displaying accurate throughput distribution without collapsing shared backend targets into a single node.
- **Routes Table (`rtUpdate()`)**: Displays matching derived metrics (`curRPS`, `p95`, `err5`) consistent with the Upstreams view.

---

## Live Telemetry Metric Derivations

The dashboard client engine dynamically derives throughput, latency percentiles, error rates, and health tones from live gateway telemetry exported by `pkg/metrics/metrics.go` and `pkg/server/internal_api.go`:

### 1. Telemetry Schema Alignment
The data model directly binds to the following backend telemetry fields:
- `rawApiStatus.metrics.requests_by_route`: Cumulative request counter map keyed by individual route paths and subpaths.
- `rawApiStatus.metrics.total_requests`: Cumulative total request counter across all routes.
- `rawApiStatus.timeseries.A.rps`: Rolling requests-per-second array from the 60-bucket ring buffer.
- `rawApiStatus.timeseries.A.p95`: Rolling p95 latency percentiles from the 60-bucket ring buffer.
- `rawApiStatus.timeseries.A.err`: Rolling 5xx error rate from the 60-bucket ring buffer.
- `/internal/api/upstreams/health`: Active probe measurements including `latency_ms`, `http_code`, `status`, and `history`.

### 2. Hierarchical Subpath Rollup in `requests_by_route`
In API gateways, clients query deep hierarchical endpoints (e.g., `/kite/api/v1/business`, `/kite/api/v2/orders`, or `/kite/auth/refresh`), which are recorded as granular path keys in `requests_by_route`.

To ensure child API requests are attributed to their configured parent route prefix (`r.prefix`), `buildDataModel()` executes a hierarchical subpath aggregation:

```javascript
function getRouteTotalReqs(pfx) {
  if (!pfx) return 0;
  if (pfx === '/') {
    return Object.entries(reqByRoute).reduce((acc, [k, count]) => {
      if (!k.startsWith('/internal/')) return acc + (Number(count) || 0);
      return acc;
    }, 0);
  }
  return Object.entries(reqByRoute).reduce((acc, [k, count]) => {
    if (k === pfx || k.startsWith(pfx + '/')) {
      return acc + (Number(count) || 0);
    }
    return acc;
  }, 0);
}
const routeCumulative = getRouteTotalReqs(prefix);
```

This ensures that any subpath falling under a configured prefix is automatically rolled up into that route's cumulative volume (`routeCumulative`).

### 3. Internal Telemetry Polling Isolation
The dashboard client polls internal management endpoints (`/internal/api/status`, `/internal/api/routes`, `/internal/api/upstreams/health`, etc.) every second. Without filtering, these background management calls accumulate in `requests_by_route` under `/internal/*` and artificially inflate total gateway counts, thereby diluting the calculated traffic share of external user routes.

To eliminate this measurement distortion, the engine computes `totalExternalRequests` by explicitly excluding all `/internal/` keys from the denominator:

```javascript
const totalExternalRequests = Object.entries(reqByRoute).reduce((acc, [k, count]) => {
  if (!k.startsWith('/internal/')) {
    return acc + (Number(count) || 0);
  }
  return acc;
}, 0);
```

External traffic ratios are then evaluated against `totalExternalRequests`, isolating internal diagnostic polling from user-facing throughput statistics.

### 4. Moving Average Temporal Smoothing for Gateway RPS
Instantaneous throughput values from single-second telemetry snapshots can exhibit high variance or sampling discretisation spikes. To produce smooth and stable rate indications without sacrificing responsiveness to sustained traffic changes, `buildDataModel()` computes a trailing 5-point moving average over recent gateway RPS samples:

```javascript
const recentRpsSamples = (tsA && tsA.rps && tsA.rps.length > 0) ? tsA.rps.slice(-5) : [];
const avgRecentRps = recentRpsSamples.length > 0 ? avg(recentRpsSamples) : 0;
const effectiveTotalRps = avgRecentRps > 0 ? avgRecentRps : totalRPS;
```

The resulting `effectiveTotalRps` is used to compute instantaneous route throughput:

$$\text{curRPS}(r) = \begin{cases} \text{effectiveTotalRps} \times \left(\dfrac{\text{routeCumulative}(r)}{\max(1, \text{totalExternalRequests})}\right) & \text{if } \text{totalExternalRequests} > 0 \\ \dfrac{\text{effectiveTotalRps}}{|R|} & \text{if } \text{totalExternalRequests} = 0 \land \text{effectiveTotalRps} > 0 \\ 0.0 & \text{otherwise} \end{cases}$$

### 5. Dual Rate and Cumulative Volume Display in Upstream Pool Cards
In `#/upstreams`, operators need to immediately differentiate between routes that are temporarily idle but have handled substantial traffic versus routes that are inactive or have never received requests.

Each upstream pool card aggregates cumulative request counts across its member routes (`p.totalReqs = sum(rs.map(r => r.totalReqs))`) and displays both real-time throughput and cumulative volume in the Requests KPI tile:

```html
<dl class="pool-sum">
  <div>
    <dt>Requests</dt>
    <dd>${fmt.n(p.rps)}/s ${p.totalReqs > 0 ? `<span class="mut">(${fmt.n(p.totalReqs)} total)</span>` : ''}</dd>
  </div>
  <div><dt>p95 Latency</dt><dd>${fmt.ms(p.p95)}</dd></div>
  <div><dt>5xx Error Rate</dt><dd class="${p.err5 >= .02 ? 't-err' : ''}">${fmt.pct(p.err5, 1)}</dd></div>
</dl>
```

For example, a pool card with historical traffic that is currently quiescent displays `0.0/s (1.7k total)`, providing clear dual-dimension operational context.

### 6. Dynamic Health Probe Latency & p95 Percentile
Instance latency in `poolCard(p)` is bound directly to `i.latency` (from `/internal/api/upstreams/health` `latency_ms`).

The pool card header p95 latency (`p.p95`) is derived dynamically:

$$\text{pool.p95} = \begin{cases} \max_{i \in \text{insts}, i.\text{lat} > 0}(i.\text{latency}) & \text{if active probe latencies exist} \\ \text{avg}_{r \in \text{routes}}(r.\text{cur.p95}) & \text{fallback to route percentiles} \\ \text{tsA.p95}[\text{len} - 1] & \text{fallback to gateway time-series} \end{cases}$$

This ensures pool cards reflect real probe round-trip measurements rather than static initial fallbacks.

### 7. Multidimensional Health Classification & Error Rate
Each upstream instance is classified into a tri-state health status:

$$\text{state}(i) = \begin{cases} \text{"down"} & \text{if } u.\text{status} \in \{\text{"UNREACHABLE"}, \text{"DOWN"}\} \lor u.\text{http\_code} \ge 500 \\ \text{"up"} & \text{if } u.\text{status} \in \{\text{"HEALTHY"}, \text{"UP"}\} \land (u.\text{http\_code} < 400 \lor u.\text{http\_code} = \text{null}) \\ \text{"warn"} & \text{otherwise (e.g. 4xx responses or degraded state)} \end{cases}$$

The pool card summary 5xx error rate (`p.err5`) combines route-level error averages and probe failure history:

$$\text{pool.err5} = \max\left( \text{avg}_{r \in \text{routes}}(r.\text{cur.err5}), \frac{\sum \text{failed\_ticks}}{\sum \text{total\_ticks}} \right)$$

where $\text{failed\_ticks}$ is the count of failing checks in the 48-tick history strips.

### 8. Deterministic Visual Tone Escalation
Pool card tone is automatically escalated to alert operators to degradation:

$$\text{tone} = \begin{cases} \text{"err"} & \text{if } \text{down\_count} > 0 \lor \text{pool.err5} \ge 0.02 \\ \text{"warn"} & \text{if } \text{warn\_count} > 0 \lor \text{pool.err5} > 0 \\ \text{"ok"} & \text{otherwise} \end{cases}$$

When an upstream returns errors or fails health probes, the pool card border, status pill, and metric badges update dynamically to reflect the incident state.

---

## Live Request Tracing & Dynamic Upstream Target Fidelity

The **Live Requests & Logs** section (`#/logs`) and Trace Waterfall Drawer (`openDrawer('log', id)`) provide end-to-end telemetry on active gateway transactions:

### 1. Dynamic Upstream Socket Resolution
When a request is handled by a reverse proxy route, the active backend socket selected by the load balancer (e.g., `127.0.0.1:8080`, `10.0.1.11:8080`) is propagated from `pkg/proxy/proxy.go` via request context into `server.GlobalTraceBuffer`.
- **Proxy Routes**: Render the concrete upstream socket (`127.0.0.1:8080`), eliminating ambiguous generic `"gateway"` strings.
- **Static Assets Routes**: Tagged with destination `static`.
- **In-Process Routes**: Built-in engine endpoints (such as `/health`, `/metrics`, and HTTPS redirects) are tagged as `in-process`.

### 2. Hierarchical Route Subpath Filtering
In `public/app.js`, the route dropdown filter (`#lgRoute`) matches both the canonical route prefix and all child subpaths. For example, filtering by `/kite/api` seamlessly displays requests to `/kite/api/v1/trades`, `/kite/api/v1/business`, and `/kite/api/v1/auth`.

### 3. Trace Waterfall & Security Isolation
Opening a request row renders execution spans across listener, router, WAF, and proxy phases. The resolved upstream node is displayed in the inspector drawer. In accordance with CWE-200 security requirements, internal upstream network sockets are recorded strictly within in-memory telemetry buffers and are never leaked to external client response headers.

---

## Alerts & Threat Defense Control Center

The **Alerts & Threat Defense Control Center** (`#/alerts`) is Toron's operational cockpit for edge security monitoring, threat analysis, and firewall policy management. Serving as the primary interface for the native [Web Application Firewall (WAF)](./waf.md) and dynamic 2-stage auto-ban engine (see [OS-Level IP Blocking](./os-level-ip-blocking.md)), this view combines high-density forensic analysis with zero-dependency native browser ECMAScript Modules (`public/js/views/alerts.js`).

The interface provides an executive security KPI strip, an ergonomically decoupled threat quarantine panel, multi-attribute real-time searching and faceted filtering, client-side pagination, interactive table sorting with deterministic tie-breaking, non-blocking modal workflows, and a slide-out forensic incident investigation drawer.

```text
+---------------------------------------------------------------------------------------------------------+
|                                  SECURITY KPI METRICS STRIP                                             |
|  [ Active Incidents: 2 ]   [ Recent WAF Blocks: 18 ]   [ Stage 1 Temp Bans: 7 ]   [ Stage 2 Perm: 3 ]   |
+---------------------------------------------------------------------------------------------------------+
|                                MANUAL THREAT ACTOR QUARANTINE                                           |
|  [ IP Address ]  [ Tier: 1h Temp v ]  [ Context / Reason ]  [ Ban Threat Actor ]                        |
+---------------------------------------------------------------------------------------------------------+
|  ACTIVE ALERTS & WAF INCIDENTS                                                                          |
|  [ Search incidents... ] [ All | Critical | Warning ]                                                    |
|  - SQL Injection [Blocked] POST /v1/auth/login · Rule 942100 · IP 198.51.100.99 · Score: 15              |
|  - Cross-Site Scripting [Blocked] GET /search · Rule 941100 · IP 203.0.113.42 · Score: 12              |
|  Showing 1–10 of 18 incidents  |  << < Page 1 / 2 > >>  |  [ 10 / page v ]                              |
+---------------------------------------------------------------------------------------------------------+
|  DYNAMIC 2-STAGE AUTO-BAN & BLOCKED IPS                                                                 |
|  [ Search bans... ] [ All | Stage 1 (1h Temp) | Stage 2 (Permanent) ]                                   |
|  Table: Client IP ▲ | Ban Tier | Created At | Temp Bans | Reason / Category | TTL / Expiry | Action      |
|  Showing 1–10 of 10 banned IPs  |  << < Page 1 / 1 > >>  |  [ 25 / page v ]                             |
+---------------------------------------------------------------------------------------------------------+
```

### 1. Executive 4-Card Security KPI Metrics Strip

The top of the view renders a responsive 4-column metric grid (`.al-kpi-grid`) presenting immediate high-level situational awareness across edge threat activity:

| KPI Metric Card | Target Element | Derived Data Source | Tone Escalation | Description |
| :--- | :--- | :--- | :--- | :--- |
| **Active Incidents / Alerts** | `#alKpiActive` | `D.alerts.length` + unmitigated `D.incidents.length` | `var(--err)` if $> 0$, `var(--ok)` if 0 | Total unresolved operational degradations and active Layer 7 security alerts requiring operator attention. |
| **Recent WAF Blocks** | `#alKpiBlocks` | Count of `D.incidents` where `action === 'blocked'` | `var(--c5)` (Indigo) | Total malicious request payloads intercepted and rejected by OWASP inspection rules within the telemetry buffer. |
| **Stage 1 Temp Bans** | `#alKpiTemp` | Count of `D.bannedIps` where `type === 'temporary'` | `var(--warn)` (Amber) | Malicious client IP addresses currently undergoing automated or manual temporary quarantine (default 1 hour). |
| **Stage 2 Permanent Bans** | `#alKpiPerm` | Count of `D.bannedIps` where `type === 'permanent'` | `var(--err)` (Red) | Repeat offending IP addresses permanently dropped at the socket layer by the firewall. |

The KPI cards adapt responsively (`repeat(auto-fit, minmax(200px, 1fr))`) to varying screen widths, ensuring readability across tablet and multi-monitor operations screens.

---

### 2. Decoupled Manual Threat Actor Quarantine Toolbar

Rather than nesting input forms inside table headers, manual quarantine is decoupled into an independent action panel (`.al-ban-box` within `.al-ban-card`). Operators can manually isolate suspicious actors, automated vulnerability scanners, or abusive bots:

- **Client-Side IP Syntax Validation (`isValidIP`)**:
  - The client engine validates both IPv4 (`255.255.255.255` dotted decimal format) and IPv6 (hexadecimal colon notation) before network transmission.
  - If an invalid address format is entered, an inline feedback warning (`#alBanError`) immediately renders without dispatching invalid API traffic over the network.
- **Duration Presets & Custom Interval Selection**:
  - **Presets**: `15m` (15 Minutes), `1h` (Stage 1: 1h Temp - default), `6h` (6 Hours), `24h` (24 Hours), `7d` (7 Days), and `Permanent` (Stage 2: Permanent).
  - **Custom Duration**: Selecting `custom` dynamically displays a duration text input (`#alBanCustomDuration`) accepting standard duration strings (e.g. `30m`, `48h`, `72h`).
- **Context & Reason Attribution**:
  - An optional context input (`#alBanReason`) captures audit justification (e.g. `Credential stuffing against /v1/auth`). If omitted, defaults to `Manually banned via dashboard`.
- **API Dispatch & State Sync**:
  - Clicking **Ban Threat Actor** submits `POST /internal/api/security/ban` with JSON payload `{ ip, type, duration, reason }`.
  - On HTTP `200 OK`, input fields clear, a floating toast confirms success, and `fetchBackendData()` immediately refreshes the dashboard data model.

---

### 3. Real-Time Multi-Attribute Search & Faceted Filtering

Both the Active Incidents feed and the Banned IPs table feature real-time search inputs and categorical faceted filter chips:

#### A. Active Incidents Filtering
- **Multi-Attribute Search (`#alIncSearch`)**:
  - Executes instant case-insensitive substring matching against `client_ip`, `rule_id`, `path`, `category`, and `payload_snippet`.
  - Includes heuristic aliases: typing `sqli` matches SQL injection (rule 942100), `xss` matches cross-site scripting (rule 941100), and `lfi` or `traversal` matches directory traversal (rule 930100).
- **Severity Faceted Chips (`#alSevChips`)**:
  - **All**: All detected anomalies and alerts.
  - **Critical**: Intercepted attacks where `action === 'blocked'`, anomaly score $\ge 10$, or severity is marked critical.
  - **Warning**: Monitored anomalies where `action === 'logged'`, anomaly score $< 10$, or severity is warning.

#### B. Banned IPs Filtering
- **Multi-Attribute Search (`#alBanSearch`)**:
  - Matches across client IP address (`ip`), justification reason (`reason`), and last violation category (`last_category`).
- **Ban Tier Faceted Chips (`#alTierChips`)**:
  - **All**: All banned actors.
  - **Stage 1 (1h Temp)**: Temporary bans (`type === 'temporary'`).
  - **Stage 2 (Permanent)**: Permanent firewall bans (`type === 'permanent'`).

Typing into either search bar or toggling any filter chip automatically resets the respective table's active page index to 1 (`state.alIncPage = 1` or `state.alBanPage = 1`), preventing empty slice views.

---

### 4. Client-Side Non-Blocking Pagination

To prevent unbounded DOM growth, memory bloat, and excessive vertical scrolling when inspecting hundreds of security incidents or thousands of banned IP records, the view implements client-side pagination (`.al-foot-bar`):

- **Configurable Page Sizes**: Dropdown controls support `10`, `25`, and `50` rows per page (defaulting to 10 for incidents and 25 for banned IPs).
- **Navigation Controls**: First (`«`), Previous (`‹`), Next (`›`), and Last (`»`) buttons with boundary disabling (First and Previous are disabled on Page 1; Next and Last are disabled on the final page).
- **Status Indicators**:
  - Summary display: `Showing start–end of total` (e.g. `Showing 1–10 of 45`).
  - Page indicator: `Page X / Y` (e.g. `Page 1 / 5`).
- **Zero Server Overhead**: Slicing occurs entirely in browser memory (`Array.prototype.slice`) in $< 1\text{ ms}$, preserving high interactivity without server queries.

---

### 5. Deterministic Multi-Level Interactive Column Sorting

The Banned IPs table provides interactive sorting across 6 telemetry columns:

| Column Header | Field Key | Default Sorting Rule | Secondary Tie-Breaker |
| :--- | :--- | :--- | :--- |
| **Client IP** | `ip` | Natural IP address collation | Collation direction |
| **Ban Tier** | `type` | String comparison (`permanent` vs `temporary`) | Deterministic IP ascending |
| **Created At** | `created_at` | Epoch millisecond timestamp comparison | Deterministic IP ascending |
| **Temp Bans** | `temp_ban_count` | Numeric strike count comparison | Deterministic IP ascending |
| **Reason / Category** | `reason` | Lexicographical string comparison | Deterministic IP ascending |
| **TTL / Expiry** | `remaining_seconds` | Numeric countdown seconds comparison | Deterministic IP ascending |

#### Deterministic Secondary IP Tie-Breaker
Clicking any column header toggles sort direction between ascending (`▲`) and descending (`▼`). When two records have identical values in the primary sort column (e.g. multiple IPs banned at the same second or sharing identical strike counts), the table breaks ties deterministically on client IP using numeric natural collation:

```javascript
diff !== 0 ? diff * mult : (a.ip || '').localeCompare(b.ip || '', undefined, { numeric: true });
```

This mathematical tie-breaker ensures stable ordering across 2-second background telemetry polling refreshes, eliminating visual row jitter and preventing items from shifting between pagination pages.

---

### 6. Non-Blocking In-App Confirmation Modal & Toast Feedback

Earlier synchronous browser dialogs (`window.confirm()`, `window.alert()`) caused browser thread blocking, halting background telemetry polling, freezing animations, and breaking headless testing suites.

The redesigned control center replaces all native dialogs with an in-app confirmation workflow:

1. **In-App Modal Trigger (`#alConfirmModal`)**:
   - Clicking the **Unban** button on any table row opens an in-app confirmation card overlay (`.modal-card` over `.modal-scrim`).
   - The modal explicitly displays the target client IP (`#alConfirmIp`) and informs the operator that traffic filtering will be revoked.
   - The modal can be dismissed safely via the Cancel button, the top-right close icon (`#alConfirmClose`), or by pressing Escape.
2. **Asynchronous Execution & Background Continuity**:
   - Confirming the unban initiates an asynchronous call (`POST /internal/api/security/unban`) via `authenticatedFetch`.
   - The main browser event loop and 2-second telemetry polling cycles continue without interruption.
3. **Floating Toast Notifications (`#alToast`)**:
   - Operation outcomes are surfaced via a non-blocking floating toast card positioned at `bottom: 20px; right: 20px` (`z-index: 100`).
   - Success operations display green tone badges (`var(--ok)`); server errors or network rejections display red tone badges (`var(--err)`).
   - Toasts automatically dismiss after 4,000 milliseconds without requiring manual user interaction.

---

### 7. Slide-Out Forensic Incident Investigation Drawer

Clicking any incident row in the **Active Alerts & WAF Incidents** feed activates Toron's slide-out inspector drawer (`openDrawer('incident', id)`), rendering in-depth Layer 7 forensic audit telemetry:

#### Forensic Display Elements
- **Header**: Incident category, action tone pill (`Blocked` vs `Logged`), and full calendar timestamp (`YYYY-MM-DD HH:MM:SS`) with relative elapsed time (e.g. `2m ago`).
- **Forensic Attributes Table**:
  - **OWASP Rule ID**: The matched Core Rule Set or custom security rule identifier (e.g. `942100` for SQL Injection, `941100` for XSS, `930100` for Path Traversal).
  - **Anomaly Score**: Numerical severity score computed during rule evaluation.
  - **Action Taken**: Visual status pill indicating whether the transaction was dropped (`Blocked`) or recorded (`Logged`).
  - **Parameter Location**: Request segment where the malicious payload was identified (`query`, `header`, `body`, or `cookie`).
  - **HTTP Method & Path**: Incoming verb and full URI endpoint (e.g. `POST /v1/auth/login?redirect=true`).
  - **Client IP**: Offending client network address, paired with a 1-click **Copy** button (`#alDrawerCopyIp`) for rapid external firewall or threat intelligence lookup.
  - **Timestamp**: High-precision calendar timestamp.
- **Attack Payload Snippet**:
  - Rendered inside a horizontally scrollable code block (`<pre class="cs-pre">`).
  - The raw attack vector (e.g. `' OR '1'='1' --`, `<script>alert(1)</script>`, `../../../../etc/passwd`) is safely sanitized via `esc()` to prevent DOM XSS execution while preserving exact byte sequences for forensic review.

#### Integrated 1-Click Remediation Actions
The drawer footer provides two immediate operational shortcuts:
1. **"Ban Client IP" (`#alDrawerBanBtn`)**:
   - Closes the drawer and automatically pre-populates the **Manual Threat Actor Quarantine** toolbar with the offending client IP and context-derived reason (e.g. `SQL Injection (Rule 942100)`).
   - Places browser focus directly into the quarantine input form for single-keystroke ban enforcement.
2. **"Filter Logs for IP" (`#alDrawerLogBtn`)**:
   - Closes the drawer, sets the log query filter state (`state.lq = client_ip`), and navigates browser routing to the Live Requests view (`#/logs`).
   - Immediately displays all historical and streaming request traces associated with the malicious IP address across the gateway.

---

### 8. Telemetry Data Model Deduplication & Badge Accuracy

In high-throughput environments, data model integrity is critical to avoid operator alarm fatigue. In `public/js/model.js`, telemetry collections are cleanly separated:

- `D.alerts`: Strictly contains unresolved operational system issues, including upstream health check failures, route 5xx error spikes exceeding 2%, and failing ACME SSL certificate renewals.
- `D.incidents`: Dedicated collection of Layer 7 WAF security events and injection attempts.
- `D.bannedIps`: Dedicated collection of active Stage 1 temporary bans and Stage 2 permanent firewall bans.

By isolating `bannedIps` from `alerts`, quarantined threat actors no longer inflate the navigation alert badge (`#nav button[data-nav="alerts"] .badge`) or trigger misleading degraded health banners on the Overview screen. Mitigated threats remain visible in their designated threat table without obscuring actionable operational gateway incidents.

---

## Native ECMAScript Modules Architecture

The dashboard is structured into a clean hierarchy of native browser ECMAScript Modules (`ESM`) under `public/js/`, delivering zero-build modularity, instantaneous development updates, and zero supply-chain vulnerabilities:

```text
public/js/
├── app.js                    # Router, navigation, event dispatch & bootstrap entrypoint
├── state.js                  # Central reactive state store, time ranges & theme engine
├── utils.js                  # Core DOM helpers, HTML sanitizers & math/time formatters
├── api.js                    # Telemetry background polling adapter & authenticatedFetch interceptor
├── model.js                  # Telemetry data model builder, rollups & rate smoothing
├── components/
│   ├── icons.js              # Vector SVG icon renderers & status tone badges
│   ├── charts.js             # SVG sparklines, timeseries graphs & latency histograms
│   ├── sankey.js             # Vector SVG Sankey traffic flow diagram generator
│   ├── drawer.js             # Slide-over inspector drawer controller
│   └── authModal.js          # Interactive in-page admin authentication modal & probe validator
└── views/
    ├── overview.js           # View 1: Gateway health, Sankey flow & signal cards
    ├── routes.js             # View 2: Route table, filtering, sorting & RPS sparklines
    ├── upstreams.js          # View 3: Upstream pool cards & 48-tick probe strips
    ├── logs.js               # View 4: Live request stream & tail pause controls
    ├── certs.js              # View 5: ACME zero-touch TLS certificates & expiration
    ├── modules.js            # View 6: Compiled engine reactors & Go runtime internals
    ├── alerts.js             # View 7: Alerts & Threat Defense control center, ban management & search
    └── console.js            # View 8: Interactive API endpoint probe debugger
```

### Key Architectural Invariants
1. **Zero External Dependencies**: The client requires zero npm runtime libraries and zero build tools (no Webpack, Vite, or Rollup). The total uncompressed script footprint is under 100 KB.
2. **Strictly Relative Specifiers**: All internal imports utilize relative paths (`./utils.js`, `../components/icons.js`) with explicit `.js` extensions, ensuring native compatibility across all modern browser module loaders.
3. **Unidirectional Data Flow**: The background poller in `public/js/api.js` updates shared raw state and invalidates `public/js/model.js`. The active view controller's `update()` method re-renders the DOM using pure string templates and SVG elements.

---

## Administrative Access & Defense-in-Depth Security

Control Center access and backing management endpoints (`/internal/api/*`) are secured via a hybrid defense-in-depth architecture:

### 1. Multi-Scheme Backend Access Controls
When administrative authentication is enabled via `config.yaml` or the `TORON_ADMIN_KEY` environment variable:
- **Admin Token**: Evaluated against `X-Toron-Admin-Key: <token>` and `Authorization: Bearer <token>` using constant-time comparison (`crypto/subtle.ConstantTimeCompare`).
- **Basic Authentication**: Evaluated against standard `Authorization: Basic <base64(user:pass)>` headers for configured administrative operators.
- **CIDR Subnet Filtering**: Requests originating from client IPs outside configured `admin_subnets` receive HTTP `403 Forbidden`.
- **Standardized 401 Challenges**: Missing or rejected credentials return HTTP `401 Unauthorized` with `WWW-Authenticate: Bearer realm="Toron Management", Basic realm="Toron Management"`.

```yaml
admin_auth_enabled: true
admin_token: "secret-admin-token"
admin_subnets:
  - "127.0.0.1/32"
  - "10.0.0.0/8"
```

### 2. Client Authentication Lifecycle & Interactive Modal
When an operator accesses `/internal/dashboard/` against a secured gateway:
1. **401 Response Interception**: The HTTP client interceptor (`authenticatedFetch` in `public/js/api.js`) pauses background polling and presents the in-page **Admin Authentication Modal** (`public/js/components/authModal.js`).
2. **Direct Verification Probe**: Submitting credentials triggers an immediate probe to `GET /internal/api/status`. If valid, the modal dismisses and polling resumes. Invalid credentials render inline error feedback.
3. **Session Scoping**: Active credentials reside exclusively in volatile browser `sessionStorage` (scoped to the active tab) and are never stored in persistent `localStorage`.
4. **Session Lock Action**: The top navigation toolbar contains a lock action button (`#authLockBtn`), enabling instant session revocation and manual re-authentication.

For endpoint schemas and REST response structures, see the [HTTP API Reference](../reference/api.md). For gateway configuration directives, see the [Configuration Guide](../configuration.md).
