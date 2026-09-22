---
title: WebSocket Upgrade and Bi-directional Proxy Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-09-09

depends_on:
  - REQ-024
  - REQ-025
  - REQ-088
  - TASK-097
  - TASK-098
  - TASK-099
  - TASK-100

derived_from:
  - REQ-024
  - REQ-025
  - REQ-088
  - ADR-019
  - ADR-083
  - SEC-27
  - SR-087

documents:
  - WEBSOCKET-GUIDE

related_to:
  - index.md
  - reverse-proxy.md
  - load-balancing.md
---

# WebSocket Upgrade and Bi-directional Proxy Guide

## Overview

Toron supports full-duplex WebSocket protocol connections (RFC 6455) and HTTP/2 Extended CONNECT WebSockets (RFC 8441) for real-time web applications, chat services, and live telemetry streaming. The gateway detects HTTP/1.1 `Upgrade: websocket` headers, verifies 101 Switching Protocols handshakes, and provides transparent bi-directional stream tunneling between client sockets and backend microservice targets with built-in Slowloris defense and activity-refreshed idle deadline enforcement (`SEC-27`, CWE-400).

## Configuration

WebSocket proxying works transparently over any standard `upstream` route in `routes.yaml`, governed by server-level idle timeout settings in `config.yaml`:

```yaml
# config.yaml
server:
  port: 8080
  idle_timeout: "30s"
  upgrade_idle_timeout: "60s"    # Maximum inactivity deadline on WebSocket/upgraded streams (default: 60s)
```

```yaml
# routes.yaml
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
4. **Activity-Refreshed Bidirectional Stream Relay (`SEC-27`)**:
   - Rather than permanently stripping socket deadlines, Toron arms both client and upstream sockets with `upgrade_idle_timeout` (default `60s`).
   - Every chunk of data transferred in either direction refreshes read and write deadlines.
   - If bidirectional silence exceeds `upgrade_idle_timeout`, both sockets are deterministically severed via `sync.Once`, unblocking relay routines and reclaiming file descriptors.
   - Standard WebSocket Ping/Pong control frames (RFC 6455) and application keep-alives continuously refresh the deadline, keeping legitimate long-lived sessions active indefinitely.
   - For HTTP/2 Extended CONNECT, a background watchdog monitors stream activity, tearing down the backend socket if inactivity occurs or if the client request context is cancelled (`r.Context().Done()` / `RST_STREAM`).
