---
title: HTTP/3 Protocol Engine & QUIC Transport Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-09-04
updated: 2026-09-10

depends_on:
  - REQ-027
  - REQ-092
  - TASK-027
  - TASK-111
  - TASK-112
  - TASK-113
  - ADR-022
  - ADR-087

derived_from:
  - REQ-027
  - REQ-092
  - ADR-022
  - ADR-087
  - SEC-31

documents:
  - HTTP3-GUIDE

related_to:
  - ../index.md
  - ../configuration.md
  - http2.md
  - tls-https.md
---

# HTTP/3 Protocol Engine & QUIC Transport Guide

## Overview

Toron includes native **HTTP/3** (RFC 9114) protocol support running over **QUIC** (RFC 9000) UDP transport layer. HTTP/3 eliminates TCP head-of-line blocking by multiplexing independent byte streams over datagram packets, accelerates connection handshakes via zero round-trip time (0-RTT), and enhances resilience on mobile or lossy networks.

Toron integrates the `quic-go/http3` engine directly into the core server, allowing HTTP/3 connections to seamlessly execute through the same router, reverse proxy, load balancer, and security middleware (WAF, rate limiting, CORS, authentication) as HTTP/1.1 and HTTP/2.

---

## Key Features

1. **Native QUIC Transport Engine**: Built directly into `pkg/server/server.go` using `http3.Server`.
2. **Concurrent UDP Startup**: `cmd/toron/main.go` spawns a concurrent background goroutine running `srv.ListenAndServeH3()` bound to UDP, operating in parallel with the primary HTTPS TCP listener.
3. **Automatic `Alt-Svc` Advertisement**: Injects `Alt-Svc: h3=":<port>"; ma=2592000` headers on HTTP/1.1 and HTTP/2 responses so compliant browsers and HTTP clients automatically upgrade subsequent requests to HTTP/3 QUIC.
4. **Graceful UDP Socket Shutdown**: `Server.Shutdown(ctx)` coordinates socket teardown via `s.h3Server.Close()`, draining active streams cleanly without connection aborts or socket leaks.
5. **Full Middleware & Router Parity**: Requests arriving over HTTP/3 QUIC pass through the unified router and middleware pipeline with identical security policies, rate limits, and proxy rules.
6. **Physical RemoteAddr Binding & Anti-Spoofing**: Ingress HTTP/3 requests arriving over QUIC datagrams bind the physical UDP peer network address (`r.RemoteAddr`) into `httpparser.Request.RemoteAddr`, guaranteeing that unauthenticated external clients cannot spoof client IPs to bypass WAF IP ACLs, administrative subnet policies, rate limiters, or upstream proxy audit trails ([`SEC-31`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L428-L436), [`REQ-092`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-092.md), [`ADR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-087.md)).

---

## Architecture & Lifecyle

```text
 Client Browser / HTTP Client
        │
        ├──────────────────────────┐
        │ TCP: Port 443            │ UDP: Port 443
        ▼                          ▼
 ┌──────────────────────┐   ┌──────────────────────┐
 │   Toron TCP Reactor  │   │  quic-go HTTP/3 Srv  │
 │  (HTTP/1.1 & HTTP/2) │   │     (QUIC Engine)    │
 └──────────┬───────────┘   └──────────┬───────────┘
            │                          │
            │ Alt-Svc: h3=":443"       │ Native Streams
            ▼                          ▼
 ┌─────────────────────────────────────────────────┐
 │               http2AdapterHandler               │
 └────────────────────────┬────────────────────────┘
                          ▼
 ┌─────────────────────────────────────────────────┐
 │        Toron Core Router & Middleware           │
 │  (WAF, Auth, Rate Limiter, Reverse Proxy, APIs) │
 └─────────────────────────────────────────────────┘
```

### 1. Concurrent Startup in `cmd/toron`

When `server.tls.enabled: true` and `server.http3.enabled: true`, `cmd/toron/main.go` starts the HTTP/3 QUIC listener concurrently in a dedicated background goroutine:

```go
if appCfg.Server.TLS.Enabled && appCfg.Server.HTTP3.Enabled {
    h3Port := appCfg.Server.HTTP3.Port
    if h3Port <= 0 {
        h3Port = 8443
    }
    go func() {
        log.Printf("[TORON] HTTP/3 QUIC Server listening on UDP :%d...", h3Port)
        if err := srv.ListenAndServeH3(appCfg.Server.TLS.CertFile, appCfg.Server.TLS.KeyFile); err != nil && !errors.Is(err, server.ErrServerClosed) {
            log.Printf("[TORON] HTTP/3 QUIC Server error: %v", err)
        }
    }()
}
```

### 2. Protocol Upgrade via `Alt-Svc` Header

Modern browsers discover HTTP/3 endpoints by receiving an `Alt-Svc` header over an existing HTTP/1.1 or HTTP/2 TLS connection:

```http
HTTP/2 200 OK
content-type: application/json
alt-svc: h3=":8443"; ma=2592000
```

When `server.http3.alt_svc_header: true`, Toron automatically injects this header into HTTP responses unless a custom `Alt-Svc` header has already been set.

### 3. Graceful Socket Teardown

During SIGINT / SIGTERM shutdown:
- `Server.Shutdown(ctx)` safely extracts `h3 := s.h3Server` under `sync.RWMutex` lock and calls `h3.Close()`.
- Underlying QUIC UDP sockets are immediately drained and closed.
- Expected shutdown errors (`http.ErrServerClosed`) are translated to `server.ErrServerClosed` to prevent false-positive log errors.

---

## Configuration Reference

Configure HTTP/3 under `server.http3` in `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8443

  tls:
    enabled: true
    auto_dev_cert: true       # Auto-generates ECDSA localhost certificate

  http3:
    enabled: true             # Enable HTTP/3 QUIC protocol engine over UDP
    port: 8443                # UDP listener port for HTTP/3 QUIC
    alt_svc_header: true      # Automatically advertise Alt-Svc: h3=":8443"
```

### Configuration Options

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enabled` | `boolean` | `true` | Enables or disables the HTTP/3 QUIC protocol engine. |
| `port` | `integer` | `8443` | UDP socket port for HTTP/3 QUIC. Defaults to `8443` if `<= 0`. |
| `alt_svc_header` | `boolean` | `true` | Automatically injects `Alt-Svc: h3=":<port>"; ma=2592000` response headers on TLS endpoints. |

---

## Verifying HTTP/3

### 1. Verification using `curl` with HTTP/3 Support

Ensure `curl` is built with `--http3` support (e.g. using `nghttp3` / `quiche`):

```bash
# Direct HTTP/3 request over UDP
curl --http3 -k -v https://localhost:8443/health
```

Expected output confirms the HTTP/3 protocol exchange:
```text
* Connected to localhost (127.0.0.1) port 8443
* using HTTP/3
* [HTTP/3] [0] OPENED stream for /health
< HTTP/3 200
< content-type: application/json
< alt-svc: h3=":8443"; ma=2592000
{"status":"healthy"}
```

### 2. Checking `Alt-Svc` Header over HTTP/2

```bash
curl -k -I https://localhost:8443/health
```

Expected response headers include:
```http
alt-svc: h3=":8443"; ma=2592000
```

---

## Troubleshooting

| Problem | Potential Cause | Resolution |
| :--- | :--- | :--- |
| HTTP/3 listener does not start | `server.tls.enabled` is `false` | HTTP/3 QUIC requires TLS. Enable TLS in `config.yaml` (`server.tls.enabled: true`). |
| Firewall drops UDP packets | UDP port blocked | Ensure your network firewall and cloud security groups allow inbound traffic on the configured UDP port (e.g. UDP port 443 / 8443). |
| Browser does not upgrade to HTTP/3 | `alt_svc_header` disabled or invalid cert | Ensure `server.http3.alt_svc_header: true` and that the TLS certificate is trusted by the browser. |
| Alt-Svc header missing | Custom handler sets `Alt-Svc` | Toron preserves pre-existing `Alt-Svc` headers. Verify upstream routes are not overriding it. |

---

## Related Documentation

- [Documentation Index](../index.md)
- [Configuration Guide](../configuration.md)
- [HTTP/2 Protocol Support](./http2.md)
- [HTTPS TLS & Certificates](./tls-https.md)
- [Configuration Options Reference](../reference/config-options.md)
