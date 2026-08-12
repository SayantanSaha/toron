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

1. **HTTP/1.1 Handshake Verification (RFC 6455)**: When an incoming HTTP/1.1 request contains `Connection: Upgrade` and `Upgrade: websocket`, Toron identifies the connection upgrade request and verifies the 101 status line.
2. **HTTP/2 Extended CONNECT (RFC 8441)**: For HTTP/2 connections, clients send `:method = CONNECT` and `:protocol = websocket`. Toron maps the extended CONNECT request, returns HTTP `200 OK` on the HTTP/2 stream, and bridges full-duplex stream data without dropping connection multiplexing.
3. **Upstream Connection**: Toron connects to the configured upstream target and forwards the original WebSocket handshake headers.
4. **Bi-directional Stream Tunneling**: Toron suspends request timeout deadlines and launches concurrent full-duplex socket/stream copying goroutines (`io.Copy`), maintaining raw binary/text frame pass-through until either side closes the session.
