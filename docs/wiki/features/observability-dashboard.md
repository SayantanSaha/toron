---
title: High-Density Gateway Observability Dashboard
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-09-21
updated: 2026-09-21

depends_on:
  - REQ-135
  - TASK-158

derived_from:
  - REQ-135
  - TASK-158
  - ADR-135

documents:
  - OBSERVABILITY-DASHBOARD

related_to:
  - ../reference/api.md
  - ../configuration.md
---

# High-Density Gateway Observability Dashboard

## Overview

Toron provides a high-density, real-time observability control plane served directly from `/internal/dashboard/`. Designed with pure vector SVG graphics and zero external JavaScript dependencies, it delivers instant visibility into traffic topologies, routing distributions, latency percentiles, upstream node health histories, and live request trace waterfalls.

---

## Key Dashboard Views

### 1. Overview
- **Live Status Banner**: Operational health status (`All systems operational`, `Degraded`, `Needs attention`) and active incident alerts.
- **Sankey Traffic Flow**: Vector ribbons visualizing throughput (RPS) flowing from listeners (`:443`, `:80`) $\to$ route rules $\to$ backend upstream pools.
- **Key Signals Matrix**: 5 core KPI tiles (Requests/s, p95 Latency, 5xx Rate, Active Connections, Egress Mb/s) with trend sparklines.
- **Dual Time-Series Curves**: Latency percentiles (p50, p95, p99) and stacked 4xx/5xx error volumes over 15m, 1h, 6h, and 24h ranges.

### 2. Routes & Route Detail Drawer
- Sortable and filterable route table showing RPS sparklines, p95 latency, and error rates.
- Clicking any route opens the **Route Detail Drawer** displaying latency vs. SLO target reference curves, error breakdowns, latency distribution histograms, and configured middlewares.

### 3. Upstreams & Node Health Histories
- Grouped by pool with load balancing algorithm and active instance counts.
- Per-instance cards featuring **48-tick visual health check history strips** (green = passing, red = failed).

### 4. Live Requests & Trace Waterfall Drawer
- Real-time tailing request stream with status code chips (`2xx`, `3xx`, `4xx`, `5xx`), "Over SLO" toggle, and route filter.
- Clicking a request opens the **Trace Waterfall Drawer** decomposing execution latency across listener parsing, TLS handshake, routing, token verification, rate limiting, upstream connect, and response streaming.

### 5. Certificates & Modules
- **Certificates**: Let's Encrypt challenge types (HTTP-01, TLS-ALPN-01, DNS-01), days to expiration with progress bars, and renewal failure alerts.
- **Modules & Runtime**: Compiled engine reactors, event bus throughput, and Go runtime stats (Goroutines, Heap MB, GC pause p99, Open FDs).

### 6. API Console
- Interactive diagnostic probe tool calling `POST /internal/api/proxy-test` to test endpoints and inspect response headers and bodies.

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
