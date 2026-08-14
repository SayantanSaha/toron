---
title: In-Memory HTTP Response Caching
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-035
  - TASK-035
  - ADR-030

derived_from:
  - REQ-035

documents:
  - FEATURE-RESPONSE-CACHING

related_to:
  - ../configuration.md
  - ./compression.md
  - ../index.md
---

# In-Memory HTTP Response Caching (RFC 7234)

## Overview

Toron features a thread-safe in-memory HTTP response caching engine. Repeated idempotent `GET` and `HEAD` requests are served directly from RAM with sub-millisecond response latency, reducing compute and network load on upstream microservices.

## Features

- **RFC 7234 Directive Compliance**: Parses `Cache-Control` directives (`max-age=N`, `no-store`, `no-cache`, `private`, `public`).
- **Telemetry & Diagnostic Headers**: Injects `X-Cache: HIT` / `X-Cache: MISS` and calculated `Age: <seconds>` headers on cache hits.
- **Client Cache Refresh**: Honors client request headers `Cache-Control: no-cache` and `Pragma: no-cache` to bypass cached copies and revalidate upstream.
- **Memory Safety & Bounding**: Enforces `max_entries` and `max_payload_size` limits, automatically evicting expired entries.
- **Bypass Protections**: Non-idempotent methods (POST, PUT, DELETE), error codes, and WebSocket handshakes (`101 Switching Protocols`) are never cached.

## Configuration

In `config.yaml`:

```yaml
server:
  cache:
    enabled: true             # Enable in-memory response caching
    default_ttl: 60s          # Expiration duration if Cache-Control max-age is omitted
    max_entries: 1000         # Maximum number of response entries in memory
    max_payload_size: 1048576 # 1 MB max body size per cached entry
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [Response Compression](./compression.md)
