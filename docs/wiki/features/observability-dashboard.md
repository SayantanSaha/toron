---
title: High-Density Gateway Observability Dashboard
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-09-21
updated: 2026-09-22

documents:
  - OBSERVABILITY-DASHBOARD

related_to:
  - ../reference/api.md
  - ../configuration.md
  - ../release-notes.md
---

# High-Density Gateway Observability Dashboard

## Overview

Toron provides a high-density, real-time observability control plane served directly from `/internal/dashboard/`. Designed with pure vector SVG graphics and zero external JavaScript dependencies, it delivers instant visibility into traffic topologies, routing distributions, latency percentiles, upstream node health histories, and live request trace waterfalls.

The dashboard client engine (`public/app.js`) polls Toron's internal management APIs (`/internal/api/status`, `/internal/api/routes`, and `/internal/api/upstreams/health`) on a 1-second interval, constructing an in-memory client data model with sub-millisecond execution overhead.

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
- Grouped into distinct upstream pool cards by route identity, showing load-balancing algorithms, summary RPS, p95 latency, and error rates.
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
- `rawApiStatus.metrics.requests_by_route`: Cumulative request counter map keyed by route prefix.
- `rawApiStatus.metrics.total_requests`: Cumulative total request counter across all routes.
- `rawApiStatus.timeseries.A.rps`: Rolling requests-per-second array from the 60-bucket ring buffer.
- `rawApiStatus.timeseries.A.p95`: Rolling p95 latency percentiles from the 60-bucket ring buffer.
- `rawApiStatus.timeseries.A.err`: Rolling 5xx error rate from the 60-bucket ring buffer.
- `/internal/api/upstreams/health`: Active probe measurements including `latency_ms`, `http_code`, `status`, and `history`.

### 2. Proportional Requests/s (RPS) Derivation
Instantaneous throughput for route $r$ is derived by scaling the latest gateway total RPS by the route's cumulative request share:

$$\text{curRPS}(r) = \begin{cases} \text{total\_rps} \times \left(\dfrac{\text{requests\_by\_route}[r.\text{prefix}]}{\max(1, \text{total\_requests})}\right) & \text{if } \text{total\_requests} > 0 \\ \dfrac{\text{total\_rps}}{|R|} & \text{if } \text{total\_requests} = 0 \land \text{total\_rps} > 0 \\ 0.0 & \text{otherwise} \end{cases}$$

where $\text{total\_rps} = \text{tsA.rps}[\text{len} - 1]$. If a route has handled zero requests, `curRPS` evaluates cleanly to `0.0/s`.

The pool summary Requests metric is computed as the sum across all routes associated with that pool:

$$\text{pool.rps} = \sum_{r \in \text{pool.routes}} \text{curRPS}(r)$$

and formatted in the pool card header (e.g. `12.4/s`).

### 3. Dynamic Health Probe Latency & p95 Percentile
Instance latency in `poolCard(p)` is bound directly to `i.latency` (from `/internal/api/upstreams/health` `latency_ms`).

The pool card header p95 latency (`p.p95`) is derived dynamically:

$$\text{pool.p95} = \begin{cases} \max_{i \in \text{insts}, i.\text{lat} > 0}(i.\text{latency}) & \text{if active probe latencies exist} \\ \text{avg}_{r \in \text{routes}}(r.\text{cur.p95}) & \text{fallback to route percentiles} \\ \text{tsA.p95}[\text{len} - 1] & \text{fallback to gateway time-series} \end{cases}$$

This ensures pool cards reflect real probe round-trip measurements rather than static initial fallbacks.

### 4. Multidimensional Health Classification & Error Rate
Each upstream instance is classified into a tri-state health status:

$$\text{state}(i) = \begin{cases} \text{"down"} & \text{if } u.\text{status} \in \{\text{"UNREACHABLE"}, \text{"DOWN"}\} \lor u.\text{http\_code} \ge 500 \\ \text{"up"} & \text{if } u.\text{status} \in \{\text{"HEALTHY"}, \text{"UP"}\} \land (u.\text{http\_code} < 400 \lor u.\text{http\_code} = \text{null}) \\ \text{"warn"} & \text{otherwise (e.g. 4xx responses or degraded state)} \end{cases}$$

The pool card summary 5xx error rate (`p.err5`) combines route-level error averages and probe failure history:

$$\text{pool.err5} = \max\left( \text{avg}_{r \in \text{routes}}(r.\text{cur.err5}), \frac{\sum \text{failed\_ticks}}{\sum \text{total\_ticks}} \right)$$

where $\text{failed\_ticks}$ is the count of failing checks in the 48-tick history strips.

### 5. Deterministic Visual Tone Escalation
Pool card tone is automatically escalated to alert operators to degradation:

$$\text{tone} = \begin{cases} \text{"err"} & \text{if } \text{down\_count} > 0 \lor \text{pool.err5} \ge 0.02 \\ \text{"warn"} & \text{if } \text{warn\_count} > 0 \lor \text{pool.err5} > 0 \\ \text{"ok"} & \text{otherwise} \end{cases}$$

When an upstream returns errors or fails health probes, the pool card border, status pill, and metric badges update dynamically to reflect the incident state.

---

## Administrative Access & Security

Access to `/internal/dashboard/` and all backing APIs (`/internal/api/*`) can be protected using token, API key, Basic auth, and CIDR subnet restrictions configured in `config.yaml`:

```yaml
admin_auth_enabled: true
admin_token: "secret-admin-token"
admin_subnets:
  - "127.0.0.1/32"
  - "10.0.0.0/8"
```

For endpoint schemas and REST response structures, see the [HTTP API Reference](../reference/api.md). For gateway configuration directives, see the [Configuration Guide](../configuration.md).
