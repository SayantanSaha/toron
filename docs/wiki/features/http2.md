---
title: HTTP/2 Protocol Support Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-09-10

depends_on:
  - REQ-022
  - REQ-092
  - TASK-022
  - TASK-111
  - TASK-112
  - TASK-113
  - ADR-017
  - ADR-087

derived_from:
  - REQ-022
  - REQ-092
  - ADR-017
  - ADR-087
  - SEC-31

documents:
  - HTTP2-GUIDE

related_to:
  - index.md
  - configuration.md
---

# HTTP/2 Protocol Support Guide

## Overview

Toron includes native HTTP/2 (RFC 7540) protocol support with cleartext `h2c` prior knowledge preface detection, stream multiplexing, and physical socket address binding.

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
3. **Physical RemoteAddr Binding & Ingress Anti-Spoofing**: Multiplexed HTTP/2 streams bind the physical TCP client socket address (`r.RemoteAddr`) at transport ingress into `httpparser.Request.RemoteAddr`, establishing tamper-proof client identity parity with HTTP/1.1 across WAF IP ACL, Rate Limiting, Internal Management APIs, Reverse Proxying, and Structured Logging (`SEC-31`, `REQ-092`, `ADR-087`).
