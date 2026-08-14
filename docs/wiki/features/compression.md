---
title: Transparent Response Compression
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-034
  - TASK-034
  - ADR-029

derived_from:
  - REQ-034

documents:
  - FEATURE-RESPONSE-COMPRESSION

related_to:
  - ../configuration.md
  - ../index.md
---

# Transparent Response Compression (Gzip & Deflate)

## Overview

Toron includes transparent streaming HTTP response compression middleware utilizing the Go standard library packages `compress/gzip` and `compress/flate`. Responses exceeding a configurable byte threshold are automatically compressed when the client advertises `Accept-Encoding: gzip` or `Accept-Encoding: deflate`.

## Features

- **Standard Encodings**: Transparently compresses responses using Gzip or Deflate.
- **Zero-Allocation Pooling**: Utilizes `sync.Pool` for gzip and flate writer instances, recycling memory under heavy concurrency.
- **Selective MIME Type Filtering**: Compresses text and data types (`text/*`, `application/json`, `application/javascript`, `application/xml`, `image/svg+xml`) while bypassing incompressible binary payloads (JPEG, PNG, audio, video, zip).
- **Header Synchronization**: Automatically injects `Content-Encoding`, appends `Vary: Accept-Encoding`, and adjusts `Content-Length`.
- **WebSocket & Stream Safe**: Automatically bypasses WebSocket handshakes (`101 Switching Protocols`) and streaming connections.

## Configuration

In `config.yaml`:

```yaml
server:
  compression:
    enabled: true
    min_length: 512           # Minimum response byte threshold
    level: -1                 # -1 = default compression, 1 = best speed, 9 = best compression
    encodings:
      - "gzip"
      - "deflate"
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [In-Memory Response Caching](./response-caching.md)
