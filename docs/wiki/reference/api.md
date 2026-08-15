---
title: HTTP API Reference
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-002
  - TASK-005

derived_from:
  - REQ-002

documents:
  - API-REFERENCE

related_to:
  - getting-started.md
---

# HTTP API Reference

Built-in endpoint reference for Toron.

## 1. Health Check Endpoint

- **Path**: `/health`
- **Method**: `GET`
- **Response**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "status": "ok"
}
```

## 2. Server Status Endpoint

- **Path**: `/api/status`
- **Method**: `GET`
- **Response**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "server": "Toron",
  "version": "1.0.0",
  "uptime": "healthy",
  "engine": "event-driven"
}
```

## 3. Prometheus Metrics Endpoint

- **Path**: `/metrics`
- **Method**: `GET`
- **Response**: `200 OK`
- **Content-Type**: `text/plain; version=0.0.4`
- **Description**: Returns Prometheus exposition text format including request counters, latency histograms, QUIC stream gauges, and circuit breaker trips.

## 4. Internal Control Plane API Endpoints

- **`/internal/api/status`** (`GET`): Returns server runtime stats, port, worker pool size, embedded JSON telemetry metrics, and security metadata (`waf_enabled`, `waf_mode`, `waf_rules_count`, `waf_allowed_ips`, `cors_enabled`, `security_headers`, `mtls_enabled`).
- **`/internal/api/security/incidents`** (`GET`): Returns JSON payload containing total incident count and recent security audit events recorded by the WAF audit logger (`timestamp`, `client_ip`, `method`, `path`, `category`, `rule_id`, `anomaly_score`, `action`, `payload_snippet`).
- **`/internal/api/metrics`** (`GET`): Returns structured JSON metrics summary (`total_requests`, `active_quic_streams`, `active_tcp_connections`, `circuit_breaker_trips`, `waf` stats, status/method breakdowns).
- **`/internal/api/routes`** (`GET`): Returns active proxy route configurations, headers, and load balancing target nodes.
- **`/internal/api/upstreams/health`** (`GET`): Executes backend HTTP health probes against all upstream targets (`9001-9010`) and returns node statuses (`CLOSED`, `OPEN`, `UNREACHABLE`).
- **`/internal/api/proxy-test`** (`POST`): Accepts JSON request body (`path`, `method`, `headers`), dispatches internal test request, and returns execution metrics (`status_code`, `latency_ms`, `headers`, `body`).

## Related Pages

- [Getting Started](../getting-started.md)
