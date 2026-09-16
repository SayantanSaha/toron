---
title: Transparent Response Compression
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-09-16

depends_on:
  - REQ-034
  - REQ-037
  - TASK-034
  - TASK-037
  - ADR-029
  - ADR-032
  - REQ-128
  - TASK-151
  - ADR-128

derived_from:
  - REQ-034
  - REQ-037
  - REQ-128
  - ADR-128

documents:
  - FEATURE-RESPONSE-COMPRESSION

related_to:
  - ../configuration.md
  - ./response-caching.md
  - ./reverse-proxy.md
  - ../index.md
---

# Transparent Response Compression (Zstd, Brotli, Gzip & Deflate)

## Overview

Toron includes a transparent, high-throughput HTTP response compression engine ([`pkg/router/compression.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/compression.go)) supporting **Zstandard (`zstd`)**, **Brotli (`br`)**, **Gzip (`gzip`)**, and **Deflate (`deflate`)**. 

When downstream clients advertise supported algorithms via the `Accept-Encoding` header, Toron automatically negotiates the optimal algorithm, compresses payloads exceeding a configurable byte threshold, adjusts headers, and delivers compressed responses with minimal CPU overhead.

Under [`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md) and [`ADR-128`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-128.md), Toron enforces **content-aware streaming exemptions**, ensuring that Server-Sent Events (SSE `text/event-stream`) and unbuffered proxy feeds (`X-Accel-Buffering: no`) bypass compression entirely, delivering raw event chunks to clients with sub-millisecond latency and zero buffering delays.

---

## Key Features

- **Modern Standards-Compliant Encodings**: Supports modern Zstandard (RFC 8878), Brotli (RFC 7932), Gzip (RFC 1952), and Deflate (RFC 1951) compression.
- **Client Preference & Algorithm Negotiation**: Parses client `Accept-Encoding` quality weights (`q=`) and selects the highest-efficiency supported algorithm (`zstd` > `br` > `gzip` > `deflate`).
- **Zero-Allocation Buffer Pooling**: Recycles encoder and writer instances for all algorithms using `sync.Pool` (`zstdPool`, `brotliPool`, `gzipPool`, `deflatePool`), eliminating heap allocations on hot request paths.
- **Streaming Response Exemption (REQ-128)**: Live Server-Sent Events (`text/event-stream`) and unbuffered proxy feeds (`X-Accel-Buffering: no`) bypass compression unconditionally, avoiding event starvation.
- **Selective MIME Type Filtering**: Compresses text and structured data formats (`text/*`, `application/json`, `application/javascript`, `application/xml`, `image/svg+xml`) while automatically bypassing pre-compressed binary media (JPEG, PNG, WebP, AVIF, MP4, zip).
- **Automated Header Management**: Automatically sets `Content-Encoding`, appends `Vary: Accept-Encoding`, updates `Content-Length`, and removes stale length headers.
- **Tunnel & Upgrade Safety**: Transparently bypasses WebSocket handshakes (`101 Switching Protocols`), raw hijacked sockets (`res.UpgradedConn != nil`), and direct socket stream relays (`res.StreamBody != nil`).

---

## Streaming Response Compression Exemption

### The Wildcard `"text/"` Event Starvation Defect

In standard gateway configurations, compression is enabled for text assets via wildcard prefixes such as `"text/"` in `DefaultCompressionConfig.Types`:

```go
Types: []string{
    "text/",
    "application/json",
    "application/javascript",
    ...
}
```

Because `strings.HasPrefix("text/event-stream", "text/")` evaluates to `true`, generic compression middlewares treat Server-Sent Events (SSE) as compressible text.

When a client connects with standard browser headers (`Accept-Encoding: gzip, deflate, br, zstd`), an unexempted compression middleware:
1. Intercepts the live stream from the upstream server.
2. Checks out a compressor writer from the pool.
3. Accumulates streaming chunks in an in-memory buffer, waiting for stream termination to compute compression frames, write headers, and calculate checksums.
4. **Result**: The client suffers **event starvation**, receiving **zero events** while the connection remains open. Only upon server disconnect or timeout are buffered events flushed as a single compressed block, defeating the purpose of real-time streaming.

### The Content-Aware Pre-Compression Guard

In [`pkg/router/compression.go:130-138`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/compression.go#L130-L138), Toron evaluates streaming exemption guards **prior** to MIME type matching, payload size checks, or compressor writer acquisition:

```go
// 1. WebSocket upgrades, raw hijacked connections, or live streaming responses must not be compressed
if res.StatusCode == http.StatusSwitchingProtocols || res.UpgradedConn != nil || res.StreamBody != nil {
    return
}

// 2. Streaming MIME or unbuffered responses must not be compressed (REQ-128)
if strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/event-stream") || 
   strings.EqualFold(strings.TrimSpace(res.Header.Get("X-Accel-Buffering")), "no") {
    return
}
```

### Zero-Buffering & Zero-Checkout Mechanics

When an exempted response is detected:
- **Zero Buffering Delay**: Streaming chunks pass directly through the middleware pipeline without in-memory windowing, concatenation, or staging buffers. Chunks are transmitted to the downstream socket immediately.
- **Empty `Content-Encoding`**: The `Content-Encoding` header is left untouched (empty if the origin emitted raw uncompressed text). Downstream HTTP clients and browser `EventSource` parsers consume raw text frames immediately without decompression overhead.
- **Zero Compressor Pool Checkout**: No encoder instances are acquired from `zstdPool`, `brotliPool`, `gzipPool`, or `deflatePool`. This eliminates mutex lock contention on compressor pools during high-concurrency streaming connections.
- **Zero Dynamic Allocations**: The guard evaluates via standard zero-alloc string comparisons (`strings.ToLower`, `strings.HasPrefix`, `strings.EqualFold`), introducing zero GC pressure.

### Exemption Evaluation Matrix

| Response Characteristic | Compression Applied? | `Content-Encoding` | Delivery Latency |
| :--- | :--- | :--- | :--- |
| **`Content-Type: text/event-stream`** | **No (Bypassed)** | *(unmodified / empty)* | Sub-millisecond ($< 1\text{ms}$) immediate wire dispatch |
| **`Content-Type: text/event-stream; charset=utf-8`** | **No (Bypassed)** | *(unmodified / empty)* | Sub-millisecond ($< 1\text{ms}$) immediate wire dispatch |
| **`X-Accel-Buffering: no`** | **No (Bypassed)** | *(unmodified / empty)* | Sub-millisecond ($< 1\text{ms}$) immediate wire dispatch |
| **`res.StreamBody != nil`** | **No (Bypassed)** | *(unmodified / empty)* | Sub-millisecond ($< 1\text{ms}$) immediate wire dispatch |
| **`Content-Type: text/plain`** ($\ge 512$ bytes) | **Yes** | `zstd`, `br`, or `gzip` | Compressed block emitted on completion |
| **`Content-Type: text/html`** ($\ge 512$ bytes) | **Yes** | `zstd`, `br`, or `gzip` | Compressed block emitted on completion |
| **`Content-Type: application/json`** ($\ge 512$ bytes)| **Yes** | `zstd`, `br`, or `gzip` | Compressed block emitted on completion |

---

## Algorithm Negotiation & Priority Ranking

When a client sends an `Accept-Encoding` header, Toron evaluates the advertised algorithms against its supported encodings:

```
Accept-Encoding: gzip, deflate, br;q=0.8, zstd;q=0.9
```

1. **Quality Parsing**: Parses `q=` factors (defaulting to `1.0` if omitted). An algorithm with `q=0` is explicitly rejected.
2. **Priority Ordering**: If multiple algorithms share the same quality weight, Toron prioritizes algorithms by compression efficiency and throughput:
   $$\text{Zstandard (zstd)} \succ \text{Brotli (br)} \succ \text{Gzip (gzip)} \succ \text{Deflate (deflate)}$$
3. **Vary Header Injection**: Injects `Vary: Accept-Encoding` into downstream response headers to ensure downstream caches and CDNs maintain correct variant representations.

---

## Configuration

Configure compression in `config.yaml`:

```yaml
server:
  compression:
    enabled: true             # Enable transparent response compression
    min_length: 512           # Minimum byte size required to trigger compression
    level: -1                 # Compression level (-1 = default, 1 = best speed, 9 = best ratio)
    encodings:                # Supported encoding algorithms in preferred order
      - "zstd"
      - "br"
      - "gzip"
      - "deflate"
    types:                    # MIME types eligible for compression
      - "text/"
      - "application/json"
      - "application/javascript"
      - "application/xml"
      - "image/svg+xml"
```

### Configuration Options

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enabled` | `bool` | `true` | Enables or disables transparent response compression. |
| `min_length` | `int` | `512` | Minimum response body size (in bytes) before compression is attempted. |
| `level` | `int` | `-1` | Compression level: `-1` (default), `1` (fastest), `9` (maximum compression). |
| `encodings` | `[]string` | `["zstd", "br", "gzip", "deflate"]` | List of enabled compression algorithms. |
| `types` | `[]string` | `["text/", "application/json", ...]` | Compressible MIME types or prefix wildcards (`"text/"`). |

---

## Troubleshooting & FAQ

### Problem: My Server-Sent Events (SSE) feed is arriving without compression. Is compression broken?
> **Answer**: No, this is intended and critical behavior. Under [`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md), `text/event-stream` responses are exempted from compression. Compressing SSE streams buffers event chunks into memory until connection termination, causing event starvation for clients. Raw chunks are transmitted immediately to preserve real-time delivery.

### Problem: Small JSON responses (< 512 bytes) are not compressed.
> **Answer**: Compressing very small payloads frequently produces compressed blocks larger than the original payload due to framing and checksum headers. Responses smaller than `min_length` (default: 512 bytes) are intentionally emitted uncompressed.

### Problem: How do I force unbuffered, uncompressed streaming from my backend application?
> **Answer**: Have your upstream backend include the header `X-Accel-Buffering: no` or set `Content-Type: text/event-stream`. Toron will automatically bypass both compression and caching middleware pipelines and activate direct socket streaming.

---

## Related Pages

- [Configuration Guide](../configuration.md)
- [In-Memory Response Caching](./response-caching.md)
- [Reverse Proxy & Gateway Routing](./reverse-proxy.md)
- [Release Notes](../release-notes.md)
