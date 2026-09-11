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
- **`/internal/api/proxy-test`** (`POST`): Accepts a diagnostic JSON request body (`path`, `method`, `headers`), dispatches an internal HTTP test probe against configured local reverse proxy routes, and returns structured execution metrics (`status_code`, `status_text`, `latency_ms`, `headers`, `body`, `truncated`).
  - **Inbound Request Ingestion Limit (64 KB)**: Inbound JSON payloads are strictly bounded to a 64 KB (`65,536` bytes) ceiling via `io.LimitReader`. Requests exceeding 64 KB are rejected immediately with `HTTP 400 Bad Request` (`{"error":"400 Bad Request","message":"Request body exceeds maximum allowed size of 64KB"}`), preventing memory exhaustion attacks (CWE-400).
  - **Configurable Response Buffering Limit (`max_proxy_test_response_bytes`)**: Upstream response body buffering is constrained by `InternalAPIConfig.MaxProxyTestResponseBytes` (defaulting to 1 MB / `1,048,576` bytes if omitted, zero, or negative). Responses exceeding this limit are deterministically clamped to `maxResponseBytes` with `"truncated": true` signaled in the JSON response payload.
  - **Socket Reuse & Stream Draining (CWE-775 Remediation)**: When upstream payloads exceed the buffer limit, up to 64 KB of residual stream bytes are drained into `io.Discard` before deferred socket closure (`httpResp.Body.Close()`). This guarantees that underlying HTTP/1.1 TCP connections are returned to the Go transport keep-alive pool for reuse, eliminating file descriptor leaks (`EMFILE`).
  - **Method & SSRF Security Guards**:
    - Permitted diagnostic methods are constrained strictly to `GET`, `HEAD`, and `POST` (other methods return `400 Bad Request`).
    - Target paths must be relative or root-relative without authority (`http://`, hostnames, userinfo).
    - Access to internal management endpoints (`/internal/*`) is strictly forbidden.
    - Path must match an active route prefix or allowed diagnostic path (`/health`, `/api/status`).
    - Sensitive identity headers (`x-authenticated-user`, `x-admin`, `x-user`, `x-remote-user`) are rejected, and hop-by-hop headers (`x-forwarded-*`, `authorization`, `cookie`) are stripped before dispatching.
  
  **Example Request (`POST /internal/api/proxy-test`)**:
  ```json
  {
    "path": "/health",
    "method": "GET",
    "headers": {
      "Accept": "application/json"
    }
  }
  ```

  **Example Response (200 OK)**:
  ```json
  {
    "status_code": 200,
    "status_text": "OK",
    "latency_ms": 1.42,
    "headers": {
      "Content-Type": "application/json"
    },
    "body": "{\"status\":\"ok\"}",
    "truncated": false
  }
  ```

  **Example Response on Oversized Upstream Payload (Clamped)**:
  ```json
  {
    "status_code": 200,
    "status_text": "OK",
    "latency_ms": 12.85,
    "headers": {
      "Content-Type": "text/plain"
    },
    "body": "<clamped payload to MaxProxyTestResponseBytes>",
    "truncated": true
  }
  ```

## Related Pages

- [Getting Started](../getting-started.md)
