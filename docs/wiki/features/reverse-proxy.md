---
title: Reverse Proxy and Gateway Routing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-10

depends_on:
  - REQ-009
  - REQ-030
  - REQ-092
  - REQ-122
  - REQ-123
  - TASK-009
  - TASK-111
  - TASK-112
  - TASK-113
  - TASK-145
  - TASK-146
  - ADR-004
  - ADR-087
  - ADR-122
  - ADR-123
  - TC-123
  - CR-119
  - SR-123

derived_from:
  - REQ-009
  - REQ-092
  - REQ-123
  - ADR-004
  - ADR-087
  - ADR-123
  - SEC-31

documents:
  - REVERSE-PROXY-FEATURE

related_to:
  - index.md
  - configuration.md
---

# Reverse Proxy and Gateway Routing

## Overview

Toron includes a native **Reverse Proxy** engine (`pkg/proxy`), allowing it to route incoming client traffic to upstream backend HTTP microservices with load balancing, header sanitization, and path rewriting.

## Features

- **Upstream Forwarding**: Forwards request methods, query parameters, HTTP headers, and streaming request bodies.
- **Forwarded Header Sanitization & Trusted Proxy Chaining**: Injects and sanitizes standard origin headers (`X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto`, `X-Forwarded-Prefix`, `X-Real-IP`, and W3C `traceparent`) while preventing client IP spoofing ([`SEC-31`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L428-L436), [`REQ-092`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-092.md), [`ADR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-087.md)).
- **Automatic 3xx Redirect Rewriting**: Intercepts upstream `Location` redirect headers (`301`, `302`, `303`, `307`, `308`) and automatically prepends the route prefix (`/login` $\rightarrow$ `/api/login`), preventing 404s on prefix-routed legacy applications.
- **Set-Cookie Path Rewriting**: Automatically rewrites upstream `Set-Cookie: Path=/` attributes to `Path=<prefix>` to keep cookies properly scoped to the gateway route.
- **Configurable Strip Prefix**: Supports stripping the route prefix before dispatching upstream, or preserving the full path for native prefix-aware backends.
- **Resilient Fallback**: Returns `502 Bad Gateway` if the upstream server is offline or times out.

## Forwarded Header Sanitization & Anti-Spoofing (`trusted_proxies`)

Toron prevents client-side IP spoofing by deriving network identity from the physical transport connection (`RemoteAddr`) across HTTP/1.1, HTTP/2, and HTTP/3 QUIC:

1. **Untrusted Connections (Default)**:
   - Client-supplied `X-Forwarded-For` and `X-Real-IP` headers are **stripped and discarded**.
   - Upstream headers are overwritten strictly with the verified physical client address (`peerIP`):
     ```http
     X-Forwarded-For: <peerIP>
     X-Real-IP: <peerIP>
     X-Forwarded-Proto: https
     ```
2. **Verified Trusted Proxies**:
   - If the client's physical socket IP matches a configured `trusted_proxies` CIDR block:
     - Existing `X-Forwarded-For` is preserved, and `peerIP` is appended (`<existingXFF>, <peerIP>`).
     - Existing `X-Real-IP` is preserved.
3. **Preserved Mobile Roaming Affinity (REQ-030)**:
   - Sticky session load balancing (`pkg/proxy/sticky.go`) operates independently from access control. For `ip_hash` balancing, client-level headers continue to be inspected so mobile devices maintain affinity across carrier IP handovers and CGNAT reassignments without regression.

## Configuration in `routes.yaml`

```yaml
routes:
  # 1. Header routing + Load balancing across ports 9001-9003
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"
      - "http://localhost:9003"
    strip_prefix: true            # Strips /api before forwarding (default: true)
    rewrite_redirects: true       # Rewrites Location: /login -> /api/login (default: true)
    rewrite_cookie_path: true     # Rewrites Set-Cookie: Path=/ -> Path=/api (default: true)

  # 2. Secure upstream with trusted proxy chaining
  - type: "upstream"
    prefix: "/services/partner"
    target: "http://localhost:9005"
    trusted_proxies:
      - "198.51.100.0/24"         # Appends peerIP for requests from this CIDR; strips headers from all others

  # 3. Legacy backend with automatic redirect and cookie path rewriting
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
    rewrite_redirects: true
    rewrite_cookie_path: true

  # 5. Tuned upstream with custom connection pooling and egress routing (REQ-123)
  - type: "upstream"
    prefix: "/services/microservice"
    target: "https://api.internal.corp:8443"
    transport:
      max_idle_conns_per_host: 500       # Keepalive socket pool sizing
      max_conns_per_host: 50             # Upstream backpressure limit
      idle_conn_timeout: 45s             # Socket reclamation timeout
      disable_compression: true          # Raw zero-allocation pass-through
      force_attempt_http2: true          # Enable upstream HTTP/2 ALPN multiplexing
      use_env_proxy: false               # Direct dialing (or true for HTTP_PROXY)
      propagate_upstream_close: false    # Isolate downstream client keep-alives
      tracing: false                     # false = raw speed; true = W3C traceparent context generation (REQ-124)

## Upstream Transport Configuration (`ProxyTransportConfig`)

Beginning with [`REQ-123`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-123.md) and [`REQ-124`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-124.md), Toron allows granular configuration of reverse proxy transport settings.

### Global Defaults (`config.yaml`)

```yaml
proxy:
  enabled: true
  transport:
    profile: "raw_speed"            # Presets: "raw_speed" (default) or "balanced"
    max_idle_conns: 10000           # Global max idle connections
    max_idle_conns_per_host: 1000   # Max idle keepalive connections per host
    max_conns_per_host: 0           # Concurrency limit (0 = unconstrained; >0 throttles & queues)
    idle_conn_timeout: 90s          # Keepalive socket retention
    disable_compression: true       # true = raw byte pass-through; false = auto-decompress gzip
    use_env_proxy: false            # true = honors HTTP_PROXY/NO_PROXY; false = direct socket dial
    proxy_url: ""                   # Explicit forward proxy URL (e.g. http://squid.corp:3128)
    propagate_upstream_close: false # false = isolates client keepalives; true = clean client teardown
    force_attempt_http2: false      # true = ALPN h2 stream multiplexing to TLS origins
    tracing: false                  # false = suppresses crypto/rand trace ID generation (raw speed); true = generates W3C traceparent
```

### Route-Level Overrides (`routes.yaml`)

Each route can override any transport knob under `transport`:
- **Delicate microservices**: Set `max_conns_per_host: 25` to protect origin from connection flooding.
- **External Partner APIs**: Set `use_env_proxy: true` or `proxy_url: "http://squid.corp:3128"`.
- **Payload Inspection**: Set `disable_compression: false` to allow downstream middleware to inspect plaintext.
- **Upstream Session Teardown**: Set `propagate_upstream_close: true` to let origin `Connection: close` tear down the client socket cleanly while still stripping hop-by-hop headers per RFC 7230.
- **Distributed Tracing**: Set `tracing: true` on observability-critical routes to generate W3C `traceparent` headers with cryptographic random IDs, or leave `tracing: false` for raw performance.

## Programmatic Route Registration

```go
r := router.New()

// Proxy all requests matching /api/v2/* to http://localhost:9090
if err := r.Proxy("/api/v2", "http://localhost:9090"); err != nil {
    log.Fatalf("Proxy error: %v", err)
}
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [Configuration Options](../reference/config-options.md)
- [Multi-Proxy Docker Benchmark](./docker-compare-benchmark.md)
- [Web Application Firewall](../features/waf.md)
- [Troubleshooting](../troubleshooting.md)
