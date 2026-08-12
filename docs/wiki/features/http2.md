---
title: HTTP/2 Protocol Support Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-022
  - TASK-022

derived_from:
  - REQ-022
  - ADR-017

documents:
  - HTTP2-GUIDE

related_to:
  - index.md
  - configuration.md
---

# HTTP/2 Protocol Support Guide

## Overview

Toron includes native HTTP/2 (RFC 7540) protocol support with cleartext `h2c` prior knowledge preface detection and stream multiplexing.

## Configuration in `config.yaml`

Configure HTTP/2 parameters under the `server.http2` section of `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  http2:
    enabled: true
    max_concurrent_streams: 250
    max_frame_size: 16384
    allow_h2c: true
```

## Features

1. **Automatic Protocol Detection**: Detects HTTP/2 connection preface (`PRI * HTTP/2.0...`) and routes streams to the HTTP/2 engine while preserving HTTP/1.1 fallback transparently.
2. **Multiplexing & Frame Tuning**: Stream limits (`max_concurrent_streams`) and frame sizes (`max_frame_size`) protect against resource exhaustion DoS attacks.
