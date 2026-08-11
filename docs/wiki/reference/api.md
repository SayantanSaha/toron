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

## Related Pages

- [Getting Started](../getting-started.md)
