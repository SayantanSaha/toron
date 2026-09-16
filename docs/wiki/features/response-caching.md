---
title: In-Memory HTTP Response Caching
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-09-16

depends_on:
  - REQ-035
  - TASK-035
  - ADR-030
  - REQ-128
  - TASK-151
  - ADR-128

derived_from:
  - REQ-035
  - REQ-128
  - ADR-128

documents:
  - FEATURE-RESPONSE-CACHING

related_to:
  - ../configuration.md
  - ./compression.md
  - ./reverse-proxy.md
  - ../index.md
---

# In-Memory HTTP Response Caching (RFC 7234)

## Overview

Toron features an enterprise-grade, thread-safe in-memory HTTP response caching engine ([`pkg/router/cache.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/cache.go)). Operating as a shared gateway cache, Toron caches repeated idempotent `GET` and `HEAD` responses in RAM, serving subsequent matching requests with sub-millisecond latency while reducing CPU, database, and network overhead on upstream microservices.

Toron adheres strictly to the **RFC 7234 HTTP/1.1 Caching specification**, enforcing origin cache-control directives, preventing shared cache information exposure ([CWE-524](https://cwe.mitre.org/data/definitions/524.html)), and systematically exempting real-time streaming connections (such as Server-Sent Events / SSE) from cache capture ([`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md), [`ADR-128`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-128.md)).

---

## Key Features

- **Strict RFC 7234 §5.2.2.2 Compliance**: Origin responses with `Cache-Control: no-cache` are strictly prohibited from cache admission. In the absence of origin conditional revalidation, `no-cache` strictly overrides `max-age`.
- **Streaming Response Exemption**: Responses bearing `Content-Type: text/event-stream` (Server-Sent Events) or the reverse proxy directive `X-Accel-Buffering: no` unconditionally bypass cache storage, preventing live stream freezing.
- **Deterministic Header Semantics**: Cache misses and bypassed responses consistently emit `X-Cache: MISS` while strictly omitting the `Age` header. Cache hits emit `X-Cache: HIT` and a computed `Age: <seconds>` header.
- **Zero-Allocation Exemption Guards**: Streaming MIME and directive checks execute via zero-alloc string comparisons, incurring 0 bytes of dynamic heap overhead and zero payload cloning.
- **Client Cache Refresh**: Honors client request headers `Cache-Control: no-cache` and `Pragma: no-cache`, allowing clients and operators to force fresh fetches directly from origin backends.
- **Shared Cache Privacy Controls (RFC 7234 §3.2)**: Responses marked `private` or responses to authenticated requests (`Authorization` header present) are never stored in shared cache memory unless explicitly flagged `public`.
- **Memory Bounding & Eviction**: Enforces configurable `max_entries` and `max_payload_size` thresholds, utilizing thread-safe mutex locking (`sync.RWMutex`) and TTL-based eviction to safeguard system RAM.
- **Selective Method & Status Whitelisting**: Only successful or permanent status codes (`200 OK`, `301 Moved Permanently`, `404 Not Found`) on idempotent `GET` and `HEAD` requests are eligible for caching. WebSocket handshakes (`101 Switching Protocols`) and dynamic socket streams (`res.StreamBody != nil`) are never cached.

---

## Cache Admission & Exclusion Rules

The caching middleware evaluates responses following downstream route or proxy handler execution. Admission to in-memory storage requires satisfying every validation rule:

| Condition / Header | Value / Pattern | Cache Action | Rationale & Standard |
| :--- | :--- | :--- | :--- |
| **Origin `Cache-Control`** | `no-cache` | **Bypass** | RFC 7234 §5.2.2.2: Cannot store without origin revalidation. Overrides `max-age`. |
| **Origin `Cache-Control`** | `no-store` | **Bypass** | RFC 7234 §5.2.2.3: Explicit prohibition against storage. |
| **Origin `Cache-Control`** | `private` | **Bypass** | RFC 7234 §5.2.2.6: Private user response; forbidden in shared proxy cache. |
| **Origin `Cache-Control`** | `public, max-age=N` | **Cached for $N$ seconds** | Explicit shared cache authorization with duration. |
| **Origin `Cache-Control`** | *(omitted or no max-age)* | **Cached for `default_ttl`** | Fallback to configured route TTL (default: 60s). |
| **Content-Type** | `text/event-stream` (SSE) | **Bypass** | Real-time live feed; caching freezes stream snapshots ([CWE-524](https://cwe.mitre.org/data/definitions/524.html)). |
| **X-Accel-Buffering** | `no` | **Bypass** | Reverse proxy hint requesting unbuffered streaming. |
| **Stream Body** | `res.StreamBody != nil` | **Bypass** | Active direct socket stream relay ([`REQ-125`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md)). |
| **Upgraded Socket** | `101 Switching Protocols` | **Bypass** | WebSocket or hijacked raw TCP tunnel. |
| **Request Authorization** | `Authorization: ...` | **Bypass (unless `public`)** | RFC 7234 §3.2: Shared cache must protect authenticated endpoints. |
| **Vary Header** | `Vary: *` or complex headers | **Bypass** | Only `Vary: Accept-Encoding` is supported for shared variant lookups. |
| **Payload Size** | `Body.Len() > max_payload_size` | **Bypass** | Memory protection against oversized responses. |
| **Payload Size** | `Body.Len() == 0` | **Bypass** | Empty responses bypass cache storage. |

---

## RFC 7234 §5.2.2.2 Origin Cache-Control Enforcement

### The `no-cache` Directives and Precedence Rules

Under **RFC 7234 §5.2.2.2**, the `no-cache` response directive specifies that a cache must not use the response to satisfy subsequent requests without successful validation with the origin server (e.g. using `If-None-Match` or `If-Modified-Since`).

Because Toron's high-speed in-memory cache is designed as a zero-allocation shared proxy cache without origin conditional validator tracking, Toron strictly treats `no-cache` as an **absolute prohibition against cache storage**:

```go
resCC := ParseCacheControl(res.Header.Get("Cache-Control"))
if resCC.NoStore || resCC.NoCache || resCC.Private {
    return
}
```

> [!IMPORTANT]
> **`no-cache` Overrides `max-age`**: If an upstream origin returns `Cache-Control: no-cache, max-age=3600`, Toron's caching engine gives strict precedence to `no-cache`. The response is **never stored** in `ResponseCache`, eliminating Web Cache Deception and Shared Cache Information Exposure ([CWE-524](https://cwe.mitre.org/data/definitions/524.html)).

---

## Streaming Response Cache Exemptions

### Preventing SSE Stream Snapshot Freezing

In real-time architectures, Server-Sent Events (`text/event-stream`) and telemetry streams emit continuous event chunks over persistent HTTP connections. Upstream servers commonly set `Content-Type: text/event-stream` and `Cache-Control: no-cache`.

If a caching proxy fails to inspect streaming MIME types or origin `no-cache` directives:
1. The proxy evaluates that `max-age` is absent and falls back to `default_ttl` (e.g. 60 seconds).
2. The proxy captures the initial events emitted into a static buffer snapshot.
3. Subsequent clients connecting to the stream receive the frozen snapshot with `X-Cache: HIT`, never receiving live updates.

### Content-Aware Exemption Guard

Toron neutralizes this failure mode at the entry point of cache post-processing ([`pkg/router/cache.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/cache.go#L246-L249)):

```go
// Streaming MIME or unbuffered responses must never be cached (REQ-128)
if strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/event-stream") || 
   strings.EqualFold(strings.TrimSpace(res.Header.Get("X-Accel-Buffering")), "no") {
    return
}
```

- **Server-Sent Events**: Any response starting with `text/event-stream` (case-insensitive, supporting parameters such as `text/event-stream; charset=utf-8`) is unconditionally excluded from storage.
- **Unbuffered Hints**: Any response with `X-Accel-Buffering: no` (case-insensitive) is unconditionally excluded from storage.
- **Active Socket Relays**: Any response where `res.StreamBody != nil` or `res.UpgradedConn != nil` immediately skips cache evaluation.

---

## Header Semantics on Cache Bypass

When a request bypasses caching—due to client directives, origin directives, streaming MIME types, or uncacheable status codes—Toron enforces clean header semantics:

1. **`X-Cache: MISS`**: Injected into downstream headers to inform the client that the response originated directly from the upstream backend.
2. **`Age` Header Omission**: The `Age` header is explicitly deleted (`res.Header.Del("Age")`), guaranteeing that downstream clients or intermediate CDN layers do not mistake the live response for a stale or cached asset.
3. **Zero Dynamic Allocation Overhead**: Header checks use zero-alloc string inspections. When an exemption fires, the middleware returns immediately without calling `cache.Set`, copying byte buffers, or allocating memory.

| Header | Cache Hit | Origin `no-cache` Bypass | Streaming (`text/event-stream`) Bypass | Client `no-cache` Bypass |
| :--- | :--- | :--- | :--- | :--- |
| **`X-Cache`** | `HIT` | `MISS` | `MISS` | `MISS` |
| **`Age`** | `<seconds>` (e.g. `14`) | *(omitted)* | *(omitted)* | *(omitted)* |
| **`Cache Memory`** | Read from RAM | 0 bytes allocated | 0 bytes allocated | 0 bytes allocated |
| **`Origin Contacted`**| No (served from RAM) | Yes (live upstream fetch) | Yes (direct socket stream) | Yes (live upstream fetch) |

---

## Client Cache Refresh

Toron supports forced cache revalidation initiated by HTTP clients or network operators:

- **`Cache-Control: no-cache`** or **`Cache-Control: no-store`**: Bypasses cache read lookup, fetching a fresh copy from origin.
- **`Cache-Control: max-age=0`**: Bypasses cached copies.
- **`Pragma: no-cache`**: Complies with legacy HTTP/1.0 client refresh headers.

When client bypass headers are detected during the request phase, Toron bypasses `cache.Get` and invokes downstream handlers directly.

---

## Configuration

Enable and tune response caching in `config.yaml`:

```yaml
server:
  cache:
    enabled: true             # Enable in-memory response caching
    default_ttl: 60s          # Fallback TTL if origin omits Cache-Control max-age
    max_entries: 10000        # Maximum number of responses retained in memory
    max_payload_size: 1048576 # 1 MB maximum body size per cached entry (bytes)
```

### Configuration Options

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enabled` | `bool` | `false` | Enables or disables in-memory response caching. |
| `default_ttl` | `duration` | `60s` | Expiration duration for responses without explicit `max-age`. |
| `max_entries` | `int` | `1000` | Maximum number of cache entries stored before eviction. |
| `max_payload_size` | `int` | `1048576` | Maximum response body size (in bytes) eligible for caching (1 MB). |

---

## Troubleshooting & FAQ

### Problem: Real-time SSE streams return `X-Cache: MISS` on every connection. Is this an error?
> **Answer**: No, this is intended and correct behavior. Under [`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md), Server-Sent Events (`text/event-stream`) are permanently exempted from cache storage to prevent stream freezing and Web Cache Deception ([CWE-524](https://cwe.mitre.org/data/definitions/524.html)). Every connection receives live events directly from origin.

### Problem: My origin returns `Cache-Control: no-cache, max-age=300`, but Toron never returns `X-Cache: HIT`.
> **Answer**: Under RFC 7234 §5.2.2.2, `no-cache` strictly overrides `max-age` in shared proxy caches without origin conditional validation. To make the response cacheable in Toron, configure your origin backend to emit `Cache-Control: public, max-age=300`.

### Problem: Authenticated requests (`Authorization: Bearer ...`) are not cached.
> **Answer**: Under RFC 7234 §3.2, shared caches must not store responses to authenticated requests unless the origin response explicitly specifies `Cache-Control: public`. Ensure your upstream origin emits `Cache-Control: public, max-age=...` if authenticated responses are safe for shared caching.

---

## Related Pages

- [Configuration Guide](../configuration.md)
- [Transparent Response Compression](./compression.md)
- [Reverse Proxy & Gateway Routing](./reverse-proxy.md)
- [Release Notes](../release-notes.md)
