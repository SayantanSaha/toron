---
title: Transparent Response Compression
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-034
  - REQ-037
  - TASK-034
  - TASK-037
  - ADR-029
  - ADR-032

derived_from:
  - REQ-034
  - REQ-037

documents:
  - FEATURE-RESPONSE-COMPRESSION

related_to:
  - ../configuration.md
  - ../index.md
---

# Transparent Response Compression (Zstd, Brotli, Gzip & Deflate)

## Overview

Toron includes a transparent streaming HTTP response compression engine supporting **Zstandard (`zstd`)**, **Brotli (`br`)**, **Gzip (`gzip`)**, and **Deflate (`deflate`)**. Responses exceeding a configurable byte threshold are automatically compressed based on client `Accept-Encoding` negotiation and quality factor weights (`q=`).

## Features

- **Modern Encodings**: Transparently compresses responses using Zstandard (RFC 8878), Brotli (RFC 7932), Gzip (RFC 1952), or Deflate (RFC 1951).
- **Algorithm Negotiation & Quality Factors**: Respects client `q=` weighting and applies high-efficiency server ranking (`zstd` > `br` > `gzip` > `deflate`).
- **Zero-Allocation Pooling**: Utilizes `sync.Pool` for Zstandard, Brotli, Gzip, and Deflate writer/encoder instances, eliminating heap allocation on hot request paths.
- **Selective MIME Type Filtering**: Compresses text and structured data (`text/*`, `application/json`, `application/javascript`, `application/xml`, `image/svg+xml`) while bypassing incompressible binary media (JPEG, PNG, WebP, MP4, zip).
- **Header Synchronization**: Automatically injects matching `Content-Encoding`, appends `Vary: Accept-Encoding`, and adjusts `Content-Length`.
- **WebSocket & Stream Safe**: Automatically bypasses WebSocket handshakes (`101 Switching Protocols`) and hijacked raw socket streams.

## Configuration

In `config.yaml`:

```yaml
server:
  compression:
    enabled: true             # Enable automatic response compression
    min_length: 512           # Minimum response byte threshold
    level: -1                 # -1 = default compression, 1 = best speed, 9 = best compression
    encodings:
      - "zstd"
      - "br"
      - "gzip"
      - "deflate"
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [In-Memory Response Caching](./response-caching.md)
