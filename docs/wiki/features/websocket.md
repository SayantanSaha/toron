---
title: WebSocket Upgrade and Bi-directional Proxy Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-024
  - TASK-024

derived_from:
  - REQ-024
  - ADR-019

documents:
  - WEBSOCKET-GUIDE

related_to:
  - index.md
  - reverse-proxy.md
  - load-balancing.md
---

# WebSocket Upgrade and Bi-directional Proxy Guide

## Overview

Toron supports full-duplex WebSocket protocol connections (RFC 6455) for real-time web applications, chat services, and live data streaming. The gateway detects HTTP/1.1 `Upgrade: websocket` headers, verifies 101 Switching Protocols handshakes, and provides transparent bi-directional TCP stream tunneling between client sockets and backend microservice targets.

## Configuration in `routes.yaml`

WebSocket proxying works transparently over any standard `upstream` route in `routes.yaml`:

```yaml
routes:
  # WebSocket Live Stream / Chat Upstream Target
  - type: "upstream"
    prefix: "/ws"
    target: "http://localhost:9009"
```

## How It Works

1. **Handshake Verification**: When an incoming request contains `Connection: Upgrade` and `Upgrade: websocket`, Toron identifies the connection upgrade request.
2. **Upstream Connection**: Toron connects to the configured upstream target and forwards the original WebSocket handshake headers.
3. **101 Handshake Response**: Once the upstream target accepts the upgrade with a `101 Switching Protocols` status, Toron relays the response to the client socket.
4. **Bi-directional Stream Tunneling**: Toron suspends request timeout deadlines and launches concurrent full-duplex socket copying goroutines (`io.Copy`), maintaining raw binary/text frame pass-through until either side closes the session.
