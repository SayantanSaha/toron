---
title: Reverse Proxy and Gateway Routing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-16

depends_on:
  - REQ-009
  - REQ-030
  - REQ-092
  - REQ-122
  - REQ-123
  - REQ-125
  - REQ-126
  - TASK-009
  - TASK-111
  - TASK-112
  - TASK-113
  - TASK-145
  - TASK-146
  - TASK-148
  - TASK-149
  - ADR-004
  - ADR-087
  - ADR-122
  - ADR-123
  - ADR-125
  - ADR-126
  - TC-123
  - TC-126
  - CR-119
  - SR-123

derived_from:
  - REQ-009
  - REQ-092
  - REQ-123
  - REQ-125
  - REQ-126
  - ADR-004
  - ADR-087
  - ADR-123
  - ADR-125
  - ADR-126
  - SEC-31

documents:
  - REVERSE-PROXY-FEATURE

related_to:
  - index.md
  - configuration.md
  - features/event-reactor.md
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
    stream_response: true           # true = zero-copy socket streaming fast-path (raw speed); false = buffers in memory (balanced)
    response_header_timeout: 10s    # Bounded timeout for initial response headers; body streaming is decoupled
```

### Route-Level Overrides (`routes.yaml`)

Each route can override any transport knob under `transport`:
- **Delicate microservices**: Set `max_conns_per_host: 25` to protect origin from connection flooding.
- **External Partner APIs**: Set `use_env_proxy: true` or `proxy_url: "http://squid.corp:3128"`.
- **Payload Inspection**: Set `disable_compression: false` to allow downstream middleware to inspect plaintext.
- **Upstream Session Teardown**: Set `propagate_upstream_close: true` to let origin `Connection: close` tear down the client socket cleanly while still stripping hop-by-hop headers per RFC 7230.
- **Distributed Tracing**: Set `tracing: true` on observability-critical routes to generate W3C `traceparent` headers with cryptographic random IDs, or leave `tracing: false` for raw performance.
- **Long-Lived Live Streams (SSE)**: Toron handles Server-Sent Events (`text/event-stream`) and unbuffered feeds (`X-Accel-Buffering: no`) via a WebSocket-aligned direct socket relay with activity-refreshed write deadlines, automatically bypassing caching and compression ([`REQ-125`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md), [`REQ-126`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-126.md)).
- **Buffered Fallback**: Set `stream_response: false` on routes where downstream inspection requires complete in-memory body capture.

## Persistent Streaming & Slow-Read Protection ([REQ-125](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md), [REQ-126](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-126.md))

When proxying real-time upstream endpoints—such as Server-Sent Events (SSE `text/event-stream`), live telemetry feeds, or unbuffered data pipelines (`stream_response: true`)—Toron uses an active streaming response handle (`res.StreamBody`) paired with **Activity-Refreshed Write Deadlines** implemented by [`connDeadlineTracker`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/deadline.go#L14-L23).

### How It Works

1. **Header Phase**: Toron writes the HTTP status line and upstream response headers under an amortized `write_timeout` window.
2. **Chunk Relay Loop**: In [`pkg/server/server.go:347-363`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L347-L363), Toron allocates a pooled 32 KB copy buffer and transfers chunks from `res.StreamBody` to the downstream client socket.
3. **Per-Chunk Write Deadline Refresh**: Prior to transmitting each chunk (`n > 0`), Toron forces a write deadline update to `time.Now().Add(write_timeout)`:
   ```go
   if s.config.WriteTimeout > 0 {
       _ = tracker.ForceSetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
   }
   if _, writeErr := conn.Write(buf[:n]); writeErr != nil {
       _ = req.CloseBody()
       return nil
   }
   ```
4. **Infinite Stream Longevity**: As long as the downstream client consumes chunks in a timely manner, each emitted chunk extends the socket deadline. Healthy SSE streams or live telemetry feeds can remain open indefinitely (hours, days, or weeks) without being killed by the static 5-second `write_timeout`.

### Slow-Read Denial of Service ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)) Defense

Without write deadlines, malicious or stalled clients could advertise a zero TCP receive window (`win 0`) or consume bytes at 1 byte/minute, hanging proxy worker goroutines and pinning origin handles indefinitely until thread pool exhaustion occurs.

Toron prevents this exploit:
- If a client stops reading, the kernel socket send buffer saturates.
- `conn.Write(buf[:n])` blocks waiting for window space.
- After `write_timeout` (e.g. 5s) of write starvation, the operating system kernel times out the socket write.
- `conn.Write` unblocks with `os.ErrDeadlineExceeded`.
- Toron immediately exits the streaming loop, executes `defer res.StreamBody.Close()` to sever the upstream backend connection, and closes the client socket.
- The worker goroutine terminates immediately and frees all resources ([`TC-126.5`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-126.md#L295-L324)).

For detailed low-level deadline amortization algorithms and idle timeout mechanics, see [Event Reactor Core Architecture](./event-reactor.md).

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
