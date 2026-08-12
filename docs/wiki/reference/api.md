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

## 3. Internal Management API Endpoints

- **`/internal/api/status`** (`GET`): Returns server runtime stats, port, and worker pool size.
- **`/internal/api/routes`** (`GET`): Returns active proxy route configurations, headers, and load balancing target nodes.
- **`/internal/api/upstreams/health`** (`GET`): Executes backend HTTP health probes against all upstream targets (`9001-9010`) and returns node statuses (`CLOSED`, `OPEN`, `UNREACHABLE`).
- **`/internal/api/proxy-test`** (`POST`): Accepts JSON request body (`path`, `method`, `headers`), dispatches internal test request, and returns execution metrics (`status_code`, `latency_ms`, `headers`, `body`).

## Related Pages

- [Getting Started](../getting-started.md)
