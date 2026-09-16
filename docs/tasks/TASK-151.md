---
id: TASK-151
type: task
title: Implement Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-16
updated: 2026-09-16

depends_on:
  - REQ-128

derived_from:
  - REQ-128

implements:
  - REQ-128

verified_by:
  - TC-128

decided_by:
  - ADR-128

related_to:
  - REQ-017
  - REQ-019
  - REQ-034
  - REQ-035
  - ADR-029
  - ADR-030
  - TASK-148
  - REQ-125
  - REQ-126
  - REQ-127
---

# TASK-151 - Implement Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement

## 1. Overview & Objective

During end-to-end architectural audits and streaming performance benchmarks of Toron's reverse proxy and middleware pipeline, two severe root causes were identified that corrupt real-time streaming connections (such as Server-Sent Events / SSE) and dynamic HTTP endpoints:

1. **Root Cause 2: In-Memory Cache Stores `no-cache` and SSE Responses (RFC 7234 §5.2.2.2 Violation)**:
   - In [`pkg/router/cache.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/cache.go#L250-L255), `CacheMiddleware` inspects response directives:
     ```go
     resCC := ParseCacheControl(res.Header.Get("Cache-Control"))
     if resCC.NoStore || resCC.Private {
         return
     }
     ```
   - While `no-store` and `private` were inspected, **`resCC.NoCache` was completely ignored**.
   - Under RFC 7234 §5.2.2.2, the `Cache-Control: no-cache` response directive explicitly mandates that a cache must not use the response to satisfy subsequent requests without successful revalidation with the origin server (using conditional headers such as `If-None-Match` or `If-Modified-Since`). Because Toron's in-memory cache does not implement conditional origin revalidation, storing responses marked `no-cache` directly violates RFC 7234 §5.2.2.2.
   - When an upstream origin responds with `Cache-Control: no-cache` (the standard specification for SSE feeds and dynamic APIs) without specifying a `max-age`, Toron's caching engine falls back to `ttl := cfg.DefaultTTL` (default: 60 seconds). Toron clones the body snapshot into `ResponseCache`, effectively freezing the live stream.
   - Any subsequent client connecting to the streaming endpoint receives a frozen, dead snapshot with `X-Cache: HIT` and `Age: N`, breaking real-time updates and causing severe cache deception and stale state.
   - Furthermore, responses bearing `Content-Type: text/event-stream` or the reverse proxy directive `X-Accel-Buffering: no` were not systematically guarded prior to cache admission, allowing real-time feeds to be captured as static cache entries.

2. **Root Cause 3: Transparent Compression Traps `text/event-stream` via Wildcard `text/*`**:
   - In [`pkg/router/compression.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/compression.go#L42), `DefaultCompressionConfig.Types` includes `"text/"` to compress HTML, plain text, and CSS assets.
   - In `isMIMECompressible(contentType, allowedTypes)`, `strings.HasPrefix(ct, "text/")` matches `text/event-stream`.
   - When a client sends standard browser headers (`Accept-Encoding: gzip, deflate, br, zstd`), `CompressionMiddleware` intercepts the live stream, buffers the streaming payload in RAM until stream completion or disconnection, compresses it into a single zstd/brotli/gzip block, and updates `Content-Length` and `Content-Encoding`.
   - This completely destroys real-time streaming delivery: clients experience event starvation and receive zero events until the stream terminates or disconnects.
   - Additionally, upstream responses signaling `X-Accel-Buffering: no` (the standard reverse proxy directive requesting unbuffered streaming) were ignored and subjected to buffering and compression.

The objective of this task is to implement the engineering solutions approved in [`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md):
1. **RFC 7234 §5.2.2.2 Cache Enforcement (`pkg/router/cache.go`)**: Enforce unconditional cache bypass when origin responses include `resCC.NoCache`, `Content-Type: text/event-stream`, or `X-Accel-Buffering: no`. Ensure bypassed responses emit `X-Cache: MISS`, omit the `Age` header, and are never cloned into `ResponseCache`.
2. **Streaming Compression Exemption (`pkg/router/compression.go`)**: Enforce unconditional compression bypass for responses with `Content-Type: text/event-stream` or `X-Accel-Buffering: no`, ensuring raw uncompressed chunks pass through immediately with zero intermediate buffering and unmodified `Content-Encoding`.
3. **Reverse Proxy Streaming Fast-Path Refinement (`pkg/proxy/proxy.go`)**: Update `canStream` logic so that `text/event-stream` and `X-Accel-Buffering: no` responses take the direct socket streaming fast-path even if compression or caching are enabled on the route.
4. **Automated Verification & Test Suite**: Deliver comprehensive unit tests, integration tests, and data race validation ensuring flawless real-time streaming delivery and strict RFC compliance.

### Conflict Audit & Architecture Compliance

A thorough cross-audit against existing Toron specifications and architectural decisions confirms complete harmony:

- **[`REQ-034`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-034.md) / [`ADR-029`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-029.md) (Transparent Response Compression)**:
  - ADR-029 established transparent compression for static assets and REST API bodies while explicitly excluding streaming tunnels (`101 Switching Protocols` / `UpgradedConn != nil`).
  - Exempting `text/event-stream` and `X-Accel-Buffering: no` directly upholds ADR-029's core architectural principle of preserving live streaming semantics.
- **[`REQ-035`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-035.md) / [`ADR-030`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-030.md) (In-Memory Response Caching)**:
  - ADR-030 mandates *"strict adherence to RFC 7234 HTTP caching specifications."*
  - Omitting `no-cache` on response evaluation was an implementation defect that violated ADR-030's stated goal. Enforcing cache bypass on `resCC.NoCache` and streaming MIME types directly fulfills ADR-030 and prevents cache deception.
- **[`REQ-125`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md) / [`TASK-148`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-148.md) (Conditional Direct Socket Streaming Fast-Path)**:
  - Under REQ-125, direct socket streaming requires knowing whether downstream compression or caching will intercept the response.
  - By establishing that `text/event-stream` and `X-Accel-Buffering: no` unconditionally bypass caching and compression, `ReverseProxy` can immediately activate the direct socket streaming fast-path (`canStream = true`) for streaming responses even on routes where caching and compression are globally enabled.
- **Verdict**: **ZERO CONFLICTS.**

---

## 2. Work Breakdown Structure (WBS)

```
TASK-151: Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement
├── WP-1: Cache Middleware Exemption & RFC 7234 §5.2.2.2 Enforcement (pkg/router/cache.go)
│   ├── Subtask 1.1: Response Cache-Control Directive Inspection (resCC.NoCache Enforcement)
│   ├── Subtask 1.2: Streaming MIME & Reverse Proxy Directive Exemption (text/event-stream, X-Accel-Buffering: no)
│   ├── Subtask 1.3: Cache Bypass Response Semantics (X-Cache: MISS, Omission of Age, Zero Cloning)
│   └── Subtask 1.4: Client Request Cache Bypass Preservation (Cache-Control: no-cache, Pragma: no-cache)
├── WP-2: Compression Middleware Exemption for Streaming Payloads (pkg/router/compression.go)
│   ├── Subtask 2.1: Upstream Response Header Inspection Guard
│   ├── Subtask 2.2: Compressor Writer Acquisition & Execution Bypass
│   ├── Subtask 2.3: Zero-Buffering Streaming Passthrough & Payload Integrity
│   └── Subtask 2.4: Standard MIME Type Compression Preservation (Regression Protection)
├── WP-3: Reverse Proxy Streaming Eligibility Guard Refinement (pkg/proxy/proxy.go)
│   ├── Subtask 3.1: Content-Aware Streaming Eligibility (canStream) Logic Refinement
│   ├── Subtask 3.2: Direct Socket Streaming Fast-Path Activation on Cache/Compression Routes
│   └── Subtask 3.3: Hop-by-Hop Filter, Trailer Handling & Streaming Hand-Off Integrity
└── WP-4: Automated Verification & Test Suite
    ├── Subtask 4.1: Cache Middleware Unit Tests (pkg/router/cache_test.go)
    ├── Subtask 4.2: Compression Middleware Unit Tests (pkg/router/compression_test.go)
    ├── Subtask 4.3: Reverse Proxy End-to-End Streaming Integration Tests (pkg/proxy/proxy_test.go)
    └── Subtask 4.4: Concurrency, Thread-Safety & Race Cleanliness Validation (-race)
```

---

### Work Package 1 (WP-1): Cache Middleware Exemption & RFC 7234 §5.2.2.2 Enforcement (`pkg/router/cache.go`)

#### Subtask 1.1: Response Cache-Control Directive Inspection (`resCC.NoCache` Enforcement)
- In [`pkg/router/cache.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/cache.go), inspect response directives during cacheability evaluation following downstream handler execution:
  ```go
  resCC := ParseCacheControl(res.Header.Get("Cache-Control"))
  
  // RFC 7234 §5.2.2.2: responses marked no-cache must not be stored or used
  // to satisfy subsequent requests without origin revalidation.
  if resCC.NoStore || resCC.NoCache || resCC.Private {
      return
  }
  ```
- **Rationale**: Toron does not maintain origin validator caches (ETag / Last-Modified) for background revalidation. Under RFC 7234 §5.2.2.2, a cache that cannot conditionally revalidate must treat `no-cache` as an absolute prohibition against cache admission.

#### Subtask 1.2: Streaming MIME & Reverse Proxy Directive Exemption
- Ensure robust, zero-heap-allocation inspection of streaming headers prior to evaluating status code or payload size:
  ```go
  contentType := strings.ToLower(res.Header.Get("Content-Type"))
  isEventStream := strings.HasPrefix(contentType, "text/event-stream")
  isUnbuffered := strings.EqualFold(res.Header.Get("X-Accel-Buffering"), "no")

  if isEventStream || isUnbuffered {
      return
  }
  ```
- Guarantee support for:
  - Standard Server-Sent Events: `Content-Type: text/event-stream`.
  - SSE with parameters: `Content-Type: text/event-stream; charset=utf-8`.
  - Uppercase MIME variants: `Content-Type: TEXT/EVENT-STREAM`.
  - NGINX/Reverse Proxy unbuffered streaming flag: `X-Accel-Buffering: no` (case-insensitive).

#### Subtask 1.3: Cache Bypass Response Semantics
- When any bypass condition is met (`resCC.NoStore`, `resCC.NoCache`, `resCC.Private`, `isEventStream`, `isUnbuffered`, `res.StreamBody != nil`, or `res.UpgradedConn != nil`):
  - Retain `X-Cache: MISS` header already injected at handler exit (`res.Header.Set("X-Cache", "MISS")`).
  - Guarantee that the `Age` header is **never** added to bypassed responses.
  - Never execute `cache.Set(cacheKey, entry)` or clone `res.Body` into `ResponseCache`.
  - Guarantee zero memory retention in `ResponseCache.entries` for streaming endpoints.

#### Subtask 1.4: Client Request Cache Bypass Preservation
- Maintain existing client-side bypass evaluation in `pkg/router/cache.go`:
  ```go
  clientCC := ParseCacheControl(req.Header.Get("Cache-Control"))
  clientPragma := strings.ToLower(req.Header.Get("Pragma"))
  clientBypass := clientCC.NoCache || clientCC.NoStore || (clientCC.MaxAge != nil && *clientCC.MaxAge == 0) || strings.Contains(clientPragma, "no-cache")
  ```
- Ensure client requests containing `Cache-Control: no-cache` or `Pragma: no-cache` bypass cache lookup unconditionally.

---

### Work Package 2 (WP-2): Compression Middleware Exemption for Streaming Payloads (`pkg/router/compression.go`)

#### Subtask 2.1: Upstream Response Header Inspection Guard
- In [`pkg/router/compression.go:NewCompressionMiddleware`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/compression.go#L122-L140), position the streaming exemption guard immediately after connection upgrade / `StreamBody` checks and before status code or payload length evaluation:
  ```go
  // WebSocket upgrades, raw hijacked connections, or live streaming responses must not be compressed
  if res.StatusCode == http.StatusSwitchingProtocols || res.UpgradedConn != nil || res.StreamBody != nil {
      return
  }

  // Streaming MIME or unbuffered responses must not be compressed (REQ-128)
  contentType := strings.ToLower(res.Header.Get("Content-Type"))
  isEventStream := strings.HasPrefix(contentType, "text/event-stream")
  isUnbuffered := strings.EqualFold(res.Header.Get("X-Accel-Buffering"), "no")

  if isEventStream || isUnbuffered {
      return
  }
  ```

#### Subtask 2.2: Compressor Writer Acquisition & Execution Bypass
- When `isEventStream == true` or `isUnbuffered == true`:
  - Bypass `isMIMECompressible` evaluation completely.
  - Bypass client `Accept-Encoding` header negotiation.
  - Do not acquire any compressor writer from `cm.zstdPool`, `cm.gzipPool`, `cm.deflatePool`, or `cm.brotliPool`.
  - Avoid any allocation or recycling of compressor instances.

#### Subtask 2.3: Zero-Buffering Streaming Passthrough & Payload Integrity
- Ensure that:
  - `res.Body` (or `res.StreamBody`) passes downstream in its raw, uncompressed state.
  - No intermediate in-memory buffering, concatenation, or windowing occurs.
  - `Content-Encoding` remains completely unmodified (empty if uncompressed by origin).
  - Streaming event chunks reach the client socket immediately upon emission.

#### Subtask 2.4: Standard MIME Type Compression Preservation (Regression Protection)
- Ensure that standard text MIME types defined in `DefaultCompressionConfig.Types` (`"text/"`, `"text/html"`, `"text/plain"`, `"text/css"`, `"application/json"`, etc.) continue to be compressed normally when not matching `text/event-stream`.
- Ensure wildcard `"text/"` continues to match and compress standard static text assets.

---

### Work Package 3 (WP-3): Reverse Proxy Streaming Eligibility Guard Refinement (`pkg/proxy/proxy.go`)

#### Subtask 3.1: Content-Aware Streaming Eligibility (`canStream`) Logic Refinement
- In [`pkg/proxy/proxy.go:ServeHTTPWithPrefix`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L1102-L1106):
  - Review and refine the streaming decision logic:
    ```go
    contentType := strings.ToLower(outResp.Header.Get("Content-Type"))
    isStreamingMIME := strings.HasPrefix(contentType, "text/event-stream")
    isUnbuffered := strings.EqualFold(outResp.Header.Get("X-Accel-Buffering"), "no")
    canStream := p.streamResponse && (!p.routeHasCompression && !p.routeHasCache || isStreamingMIME || isUnbuffered)
    ```
  - **Eligibility Invariant**: When `streamResponse` is enabled (`p.streamResponse == true`):
    - If route has neither compression nor cache: any response can stream (`canStream = true`).
    - If route has compression or cache enabled: standard responses must buffer (`canStream = false`) to allow middleware transformation, **EXCEPT** when `isStreamingMIME` or `isUnbuffered` is true. Because WP-1 and WP-2 guarantee that `text/event-stream` and `X-Accel-Buffering: no` bypass compression and cache middlewares, these responses are unconditionally eligible for the direct socket streaming fast-path (`canStream = true`).

#### Subtask 3.2: Direct Socket Streaming Fast-Path Activation on Cache/Compression Routes
- When `canStream == true`:
  - Assign live upstream reader: `res.StreamBody = outResp.Body`.
  - Do NOT close `outResp.Body` in `ServeHTTPWithPrefix`.
  - Do NOT read body into `res.Body` via `io.Copy` or `io.CopyBuffer`.
  - Return immediately from `ServeHTTPWithPrefix`, delegating chunked socket relaying to `pkg/server`.

#### Subtask 3.3: Hop-by-Hop Filter, Trailer Handling & Streaming Hand-Off Integrity
- Ensure RFC 7230 hop-by-hop headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailers`, `Transfer-Encoding`, `Upgrade`) are stripped before hand-off.
- Ensure 3xx redirect rewriting (`RewriteRedirectLocation`) and cookie path rewriting (`RewriteCookiePath`) remain functional for streaming responses.
- Propagate upstream trailers cleanly when streams terminate.

---

### Work Package 4 (WP-4): Automated Verification & Test Suite

#### Subtask 4.1: Cache Middleware Unit Tests (`pkg/router/cache_test.go`)
- In [`pkg/router/cache_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/cache_test.go), implement dedicated test cases:
  1. `TestCache_RFC7234_OriginNoCache_Bypass`:
     - Handler sets `res.Header.Set("Cache-Control", "no-cache")` with JSON body.
     - First request: assert `X-Cache: MISS`, `Age` header empty.
     - Second identical request: assert `X-Cache: MISS`, `Age` header empty, handler hit count == 2.
     - Assert response is not stored in `ResponseCache`.
  2. `TestCache_OriginNoCache_WithMaxAge_Bypass`:
     - Handler sets `res.Header.Set("Cache-Control", "no-cache, max-age=3600")`.
     - Assert that `no-cache` directive takes precedence over `max-age` in the absence of conditional revalidation, bypassing cache entirely.
  3. `TestCache_TextEventStream_Bypass`:
     - Handler sets `res.Header.Set("Content-Type", "text/event-stream")`.
     - Repeated requests return `X-Cache: MISS` with handler executed on every request.
  4. `TestCache_XAccelBufferingNo_Bypass`:
     - Handler sets `res.Header.Set("X-Accel-Buffering", "no")`.
     - Repeated requests return `X-Cache: MISS` with handler executed on every request.
  5. `TestCache_StandardResponses_StillCached`:
     - Handler returns standard cacheable response (`Cache-Control: public, max-age=60`).
     - Request 1: `X-Cache: MISS`.
     - Request 2: `X-Cache: HIT`, `Age` header present.

#### Subtask 4.2: Compression Middleware Unit Tests (`pkg/router/compression_test.go`)
- In [`pkg/router/compression_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/compression_test.go), implement dedicated test cases:
  1. `TestCompression_TextEventStream_Bypass`:
     - Enable compression (`gzip`, `zstd`, `br`, `deflate`).
     - Client sends `Accept-Encoding: gzip, deflate, br, zstd`.
     - Handler returns `Content-Type: text/event-stream` with multi-line SSE payload.
     - Assert `res.Header.Get("Content-Encoding") == ""`.
     - Assert `res.Body` contains original uncompressed text verbatim.
  2. `TestCompression_TextEventStream_WithParameters_Bypass`:
     - Handler returns `Content-Type: text/event-stream; charset=utf-8`.
     - Client sends `Accept-Encoding: gzip`.
     - Assert `Content-Encoding` is empty.
  3. `TestCompression_XAccelBufferingNo_Bypass`:
     - Handler returns `Content-Type: text/plain` and `X-Accel-Buffering: no`.
     - Client sends `Accept-Encoding: gzip`.
     - Assert `Content-Encoding` is empty and body is uncompressed.
  4. `TestCompression_StandardText_Compressed`:
     - Handler returns `Content-Type: text/plain` without `X-Accel-Buffering: no`.
     - Client sends `Accept-Encoding: gzip`.
     - Assert `Content-Encoding: gzip` and payload is compressed.

#### Subtask 4.3: Reverse Proxy End-to-End Streaming Integration Tests (`pkg/proxy/proxy_test.go`)
- In [`pkg/proxy/proxy_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy_test.go), implement integration test cases:
  1. `TestProxy_SSE_WithCompressionAndCacheEnabled`:
     - Spin up Toron reverse proxy with route configured with `compression.enabled: true` and `cache.enabled: true`.
     - Mock upstream emits Server-Sent Events (`Content-Type: text/event-stream`).
     - Client connects sending `Accept-Encoding: gzip, deflate, br, zstd`.
     - Assert `canStream == true` and `res.StreamBody != nil`.
     - Assert downstream client receives individual events in real-time without buffering delay.
     - Assert subsequent client connection also connects to live upstream stream (not a cached snapshot).
  2. `TestProxy_XAccelBufferingNo_DirectSocketFastPath`:
     - Route with compression and cache enabled.
     - Upstream sends `X-Accel-Buffering: no`.
     - Assert `canStream == true` and socket streaming fast-path is selected.
  3. `TestProxy_StandardJSON_RouteBuffering`:
     - Route with compression enabled. Upstream sends `application/json`.
     - Assert `canStream == false`, payload is buffered and compressed downstream.

#### Subtask 4.4: Concurrency, Thread-Safety & Race Cleanliness Validation
- Execute the full test suite under the Go race detector:
  ```bash
  go test -race -v ./pkg/router/...
  go test -race -v ./pkg/proxy/...
  go test -race -v ./pkg/server/...
  ```
- Verify zero data races across concurrent streaming sessions and standard cached/compressed requests.

---

## 3. Acceptance Criteria & Verification

### 3.1 Functional Acceptance Criteria
- [ ] **RFC 7234 `no-cache` Enforcement**: Responses containing origin `Cache-Control: no-cache` are strictly excluded from storage in `ResponseCache` in `pkg/router/cache.go`.
- [ ] **Streaming MIME Cache Exemption**: Responses containing `Content-Type: text/event-stream` (case-insensitive, with or without parameters) are never stored in `ResponseCache`.
- [ ] **Unbuffered Directive Cache Exemption**: Responses containing `X-Accel-Buffering: no` (case-insensitive) are never stored in `ResponseCache`.
- [ ] **Cache Bypass Header Semantics**: All bypassed responses emit `X-Cache: MISS` and never emit an `Age` header.
- [ ] **Client Request Cache Bypass**: Requests with `Cache-Control: no-cache` or `Pragma: no-cache` continue to bypass cache lookups.
- [ ] **Streaming Compression Exemption**: Responses with `Content-Type: text/event-stream` or `X-Accel-Buffering: no` pass through `CompressionMiddleware` uncompressed without modifying `res.Body` or acquiring compressor writers.
- [ ] **Header Preservation**: `Content-Encoding` is left untouched for exempted streaming responses.
- [ ] **Standard Compression Regression Protection**: Standard compressible types (`text/html`, `text/plain`, `application/json`, etc.) continue to be compressed when `Accept-Encoding` allows.
- [ ] **Reverse Proxy Fast-Path Activation**: `pkg/proxy/proxy.go` activates `canStream = true` for `text/event-stream` and `X-Accel-Buffering: no` responses even when `routeHasCompression` or `routeHasCache` are true.

### 3.2 Non-Functional, Performance & Standards Compliance Criteria
- [ ] **Zero Dynamic Allocations**: Header inspection guards for cache and compression exemptions execute with **zero dynamic heap allocations**, using standard string prefix and equality checks.
- [ ] **Zero Intermediate Buffering**: Streaming payloads are relayed directly without in-memory buffering or concatenation in compression middleware or cache middleware.
- [ ] **Real-Time Delivery Latency**: Streaming event delivery latency is $< 1\text{ms}$ upon origin emission.
- [ ] **Standards Compliance**: Strict compliance with RFC 7234 §5.2.2.2 and prevention of cache deception / session leakage across streaming clients.
- [ ] **Zero Third-Party Dependencies**: Implementation uses exclusively Go standard library packages (`strings`, `net/http`).
- [ ] **Race Cleanliness**: All tests pass cleanly with zero race warnings under `go test -race ./pkg/router/... ./pkg/proxy/... ./pkg/server/...`.
