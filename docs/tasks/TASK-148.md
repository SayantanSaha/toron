---
id: TASK-148
type: task
title: Implement Conditional Direct Socket Streaming Fast-Path, Transport Timeout Decoupling, and Response Buffer Pooling
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-15
updated: 2026-09-15

depends_on:
  - REQ-125

derived_from:
  - REQ-125

implements:
  - REQ-125

verified_by:
  - TC-125

decided_by:
  - ADR-125

related_to:
  - REQ-125
  - ADR-125
  - TC-125
  - REQ-017
  - ADR-017
  - REQ-019
  - ADR-019
  - REQ-123
  - ADR-123
  - REQ-124
  - ADR-124
  - REQ-128
  - TASK-146
  - TASK-147
---

# TASK-148 - Implement Conditional Direct Socket Streaming Fast-Path, Transport Timeout Decoupling, and Response Buffer Pooling

## 1. Overview & Objective

Reverse proxy performance profiling under [`REQ-121`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-121.md) and [`REQ-124`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-124.md) demonstrated that Toron achieves competitive throughput (~24.5k RPS), but experiences a ~0.19ms median latency penalty relative to Traefik (P50 1.16ms vs 0.97ms). This discrepancy originates from Toron's proxy response handling:
1. In [`pkg/proxy/proxy.go:1037`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L1037), `io.Copy(res.Body, outResp.Body)` reads the entire upstream HTTP response payload into an in-memory buffer (`*bytes.Buffer`).
2. In [`pkg/httpparser/response.go:97`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response.go#L97), `res.Serialize(conn)` allocates an additional `bytes.Buffer`, serializes status line and headers, copies `res.Body.Bytes()`, and executes `conn.Write(buf.Bytes())`.

This pipeline incurs **two full in-memory body copies** and dynamic heap allocations per request. In addition, `p.Client.Timeout = 10s` configures an aggregate deadline over the entire request lifecycle including reading `Response.Body`, which abruptly terminates long-lived streaming connections (such as Server-Sent Events / SSE `text/event-stream`, telemetry feeds, or chunked streams exceeding 10 seconds).

The objective of this task is to implement the architectural blueprint specified in [`REQ-125`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-125.md), incorporating a **WebSocket-aligned stream hand-off model**:
1. **Configuration Model Extension (`pkg/config`)**: Introduce `StreamResponse` and `ResponseHeaderTimeout` in `ProxyTransportConfig` with raw-speed/balanced profile defaults and route-level overrides.
2. **Upstream Transport Timeout Decoupling (`pkg/proxy`)**: Configure `tr.ResponseHeaderTimeout` to bound time-to-first-byte while setting `p.Client.Timeout = 0` to decouple body streaming from monolithic client deadlines.
3. **Response Streaming Representation & Hand-Off (`pkg/httpparser`, `pkg/proxy`)**: Mirror the `res.UpgradedConn` design pattern by adding `res.StreamBody io.ReadCloser` to `httpparser.Response`. When streaming eligibility (`canStream`) is satisfied, `ReverseProxy` sets status/headers on `res`, assigns `res.StreamBody = outResp.Body`, and returns immediately without blocking on body reads.
4. **Middleware Streaming Passthrough (`pkg/router`)**: Allow streaming responses (`res.StreamBody != nil`) to bypass `CompressionMiddleware` and `CacheMiddleware` without in-memory buffering, satisfying [`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md).
5. **Server Streaming Relay (`pkg/server`)**: In `Server.handleConnection` and `http2AdapterHandler`, serialize status/headers to the client socket and relay stream chunks using activity-refreshed write deadlines (modeled on `s.relayUpgradedStreams`).
6. **Response Buffer Pooling (`pkg/httpparser`, `pkg/proxy`)**: Provide thread-safe `sync.Pool` slabs for `*bytes.Buffer` and copy slices for buffered fallback routes and header serialization.
7. **Automated Verification Suite**: Validate streaming fast-path, long-lived streams ($> 10\text{s}$), buffered fallback, and race safety.

---

## 2. Work Breakdown Structure (WBS)

```
TASK-148: Conditional Streaming Fast-Path, Transport Timeout Decoupling & Buffer Pooling
├── WP-1: Configuration Model Extension (pkg/config, cmd/toron)
│   ├── Subtask 1.1: Schema Extension in ProxyTransportConfig
│   ├── Subtask 1.2: Profile Defaults & Merge Logic
│   └── Subtask 1.3: Route-Level Cascading & CLI Wiring
├── WP-2: Upstream Transport Timeout Decoupling (pkg/proxy)
│   ├── Subtask 2.1: Transport ResponseHeaderTimeout Configuration
│   └── Subtask 2.2: Body Streaming Client Timeout Decoupling
├── WP-3: Streaming Representation, Eligibility Guard & WebSocket-Aligned Hand-Off
│   ├── Subtask 3.1: Response Streaming Handle in pkg/httpparser
│   ├── Subtask 3.2: Content-Aware Eligibility Guard & Proxy Hand-Off
│   ├── Subtask 3.3: Middleware Streaming Passthrough (pkg/router)
│   └── Subtask 3.4: Server Streaming Socket Relay (pkg/server)
├── WP-4: Response Buffer Pooling (pkg/httpparser, pkg/proxy)
│   ├── Subtask 4.1: sync.Pool Buffer Slab Implementation
│   ├── Subtask 4.2: Zero-Copy Dual-Write Serialization Fast-Path
│   └── Subtask 4.3: Buffered Fallback Memory Slab Recycling
└── WP-5: Automated Unit & Integration Testing
    ├── Subtask 5.1: Config & Timeout Decoupling Unit Tests
    ├── Subtask 5.2: Reverse Proxy Hand-Off & Hop-by-Hop Filter Tests
    ├── Subtask 5.3: Long-Lived Stream (> 10s) Integration Tests
    ├── Subtask 5.4: Middleware Compression & Cache Exemption Tests
    └── Subtask 5.5: HTTP/2 & Concurrency Race Safety Validation
```

### Work Package 1 (WP-1): Configuration Model Extension (`pkg/config`, `cmd/toron`)

#### Subtask 1.1: Schema Extension in `ProxyTransportConfig`
- In [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go), extend [`ProxyTransportConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L258) with the following fields:
  ```go
  StreamResponse        *bool         `yaml:"stream_response,omitempty" json:"stream_response,omitempty"`
  ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout,omitempty" json:"response_header_timeout,omitempty"`
  ```

#### Subtask 1.2: Profile Defaults & Merge Logic
- Update [`DefaultProxyTransportConfig(profile string)`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L274):
  - **`profile: "raw_speed"`**:
    - `StreamResponse = &true`
    - `ResponseHeaderTimeout = 10 * time.Second`
  - **`profile: "balanced"`**:
    - `StreamResponse = &false`
    - `ResponseHeaderTimeout = 10 * time.Second`
- Update [`MergeProxyTransportConfig(base, override)`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L309):
  - If `override.StreamResponse != nil`, set `res.StreamResponse = override.StreamResponse`.
  - If `override.ResponseHeaderTimeout > 0`, set `res.ResponseHeaderTimeout = override.ResponseHeaderTimeout`.

#### Subtask 1.3: Route-Level Cascading & CLI Wiring
- Verify that `(p *ProxyRouteConfig) ResolveTransport(global ProxyTransportConfig)` correctly cascades route-level `transport.stream_response` and `transport.response_header_timeout` overrides over global settings.
- In [`cmd/toron/main.go`](file:///Users/sneha/Developer/toron-research/toron/cmd/toron/main.go#L530-L543), map the new fields into `proxy.ProxyOptions.Transport`:
  ```go
  StreamResponse:        tc.StreamResponse,
  ResponseHeaderTimeout: tc.ResponseHeaderTimeout,
  ```
- In [`cmd/toron/main.go`](file:///Users/sneha/Developer/toron-research/toron/cmd/toron/main.go#L497-L543), populate route middleware indicators on `proxy.ProxyOptions`:
  ```go
  RouteHasCompression: appCfg.Server.Compression.Enabled,
  RouteHasCache:       appCfg.Server.Cache.Enabled,
  ```

---

### Work Package 2 (WP-2): Upstream Transport Timeout Decoupling (`pkg/proxy`)

#### Subtask 2.1: Transport `ResponseHeaderTimeout` Configuration
- Mirror `StreamResponse *bool` and `ResponseHeaderTimeout time.Duration` in `pkg/proxy/proxy.go:ProxyTransportConfig`.
- In `pkg/proxy/proxy.go:DefaultProxyTransportConfig`:
  - `raw_speed`: `StreamResponse: &true`, `ResponseHeaderTimeout: 10 * time.Second`.
  - `balanced`: `StreamResponse: &false`, `ResponseHeaderTimeout: 10 * time.Second`.
- In `pkg/proxy/proxy.go:NewProxyWithOptions`:
  - Resolve `tc.ResponseHeaderTimeout`. If `<= 0`, default to `10 * time.Second`.
  - Assign `tr.ResponseHeaderTimeout = tc.ResponseHeaderTimeout` on `http.Transport`.

#### Subtask 2.2: Body Streaming Client Timeout Decoupling
- In [`pkg/proxy/proxy.go:653`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L653):
  - Replace `p.Client.Timeout = opts.Timeout` with `client.Timeout = 0` (unbounded response body read).
  - Guarantee that connection establishment remains strictly guarded by `tr.DialContext` using `opts.Timeout` (default `10s`), and initial response headers remain guarded by `tr.ResponseHeaderTimeout` (`10s`).
  - Add fields to `ReverseProxy`:
    ```go
    streamResponse        bool
    routeHasCompression  bool
    routeHasCache        bool
    ```
  - Populate them in `NewProxyWithOptions` based on `tc.StreamResponse`, `opts.RouteHasCompression`, and `opts.RouteHasCache`.

---

### Work Package 3 (WP-3): Streaming Representation, Eligibility Guard & WebSocket-Aligned Upstream Hand-Off

#### Subtask 3.1: Response Streaming Handle in `pkg/httpparser`
- Mirroring the proven `res.UpgradedConn` design pattern for WebSockets, extend [`httpparser.Response`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response.go#L22):
  ```go
  type Response struct {
      StatusCode   int
      Header       Header
      Body         *bytes.Buffer
      UpgradedConn net.Conn
      StreamBody   io.ReadCloser
  }
  ```
- In [`httpparser.Response.Serialize(w io.Writer)`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response.go#L54):
  - If `r.StreamBody != nil`:
    - Do NOT inject or calculate a static `Content-Length` header (streaming responses have dynamic or chunked lengths).
    - If `Content-Type` is unset, default to `text/plain; charset=utf-8`.
    - Serialize the HTTP/1.1 status line and all headers followed by the terminating `\r\n`.
    - Do NOT write `r.Body` to `w` (stream body relaying is delegated to the socket streaming loop).

#### Subtask 3.2: Content-Aware Eligibility Guard & Proxy Hand-Off in `pkg/proxy`
- In [`pkg/proxy/proxy.go:ServeHTTPWithPrefix`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L1040):
  - Execute `outResp, err := p.Client.Do(outReq)`.
  - Handle errors and circuit breaker telemetry ($\ge 500$ registers failure, $< 500$ registers success).
  - Evaluate streaming eligibility per `REQ-125 §2.2` and `REQ-128`:
    ```go
    contentType := strings.ToLower(outResp.Header.Get("Content-Type"))
    isStreamingMIME := strings.HasPrefix(contentType, "text/event-stream")
    isUnbuffered := strings.EqualFold(outResp.Header.Get("X-Accel-Buffering"), "no")
    canStream := p.streamResponse && (!p.routeHasCompression && !p.routeHasCache || isStreamingMIME || isUnbuffered)
    ```
  - **Streaming Hand-Off Fast-Path (`canStream == true`)**:
    - Do NOT call `defer outResp.Body.Close()` inside `ServeHTTPWithPrefix`.
    - Set status: `res.SetStatus(outResp.StatusCode)`.
    - Filter RFC 7230 hop-by-hop headers and copy valid headers into `res.Header`.
    - Propagate upstream close if `p.propagateUpstreamClose && upstreamClosed`.
    - Execute 3xx redirect rewriting (`RewriteRedirectLocation`) and cookie path rewriting (`RewriteCookiePath`).
    - Inject sticky session cookie if sticky balancer is active.
    - Attach the live upstream stream: `res.StreamBody = outResp.Body`.
    - Return immediately from `ServeHTTPWithPrefix`, avoiding any blocking `io.Copy(res.Body, outResp.Body)`.
  - **Buffered Fallback Path (`canStream == false`)**:
    - Call `defer outResp.Body.Close()`.
    - Copy upstream headers, status, redirects, and cookies into `res`.
    - Read `outResp.Body` into `res.Body` using pooled buffer copy slab.
    - Copy trailers into `res.Header`.

#### Subtask 3.3: Middleware Streaming Passthrough (`pkg/router`)
- In [`pkg/router/compression.go:131`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/compression.go#L131):
  - Extend bypass condition:
    ```go
    if res.StatusCode == http.StatusSwitchingProtocols || res.UpgradedConn != nil || res.StreamBody != nil {
        return
    }
    ```
  - Live streams immediately exit compression middleware without payload buffering or header modification.
- In [`pkg/router/cache.go:230`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/cache.go#L230):
  - Extend bypass condition:
    ```go
    if res.StatusCode == http.StatusSwitchingProtocols || res.UpgradedConn != nil || res.StreamBody != nil {
        return
    }
    ```
  - Live streams immediately exit cache middleware, preventing payload cloning or cache registry insertion.

#### Subtask 3.4: Server Streaming Socket Relay (`pkg/server`)
- In [`pkg/server/server.go:handleConnection`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L280-L325):
  - Inspect `res.StreamBody != nil`:
    - Ensure cleanup: `defer res.StreamBody.Close()`.
    - Set write deadline for initial headers: `_ = conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))`.
    - Serialize initial response (status line & headers) to client `conn` via `res.Serialize(conn)`.
    - Pipe stream chunks from `res.StreamBody` to `conn` using a pooled copy buffer (e.g. 32KB/64KB).
    - After each chunk written, refresh socket write deadline to `time.Now().Add(idleTimeout)` (matching the activity-refreshed pattern in `relayUpgradedStreams`).
    - Terminate stream cleanly when `res.StreamBody` returns `io.EOF` or client disconnects.
    - Return `nil` from `handleConnection` (closing client connection if requested or upon completion).
- In [`pkg/server/server.go:http2AdapterHandler`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L407-L422):
  - Inspect `res.StreamBody != nil`:
    - Ensure cleanup: `defer res.StreamBody.Close()`.
    - Acquire `flusher, ok := w.(http.Flusher)`.
    - Read chunks from `res.StreamBody` into a pooled buffer, write to `w`, and call `flusher.Flush()` on every chunk.
    - Return immediately on EOF or client context cancellation (`r.Context().Done()`).

---

### Work Package 4 (WP-4): Response Buffer Pooling (`pkg/httpparser`, `pkg/proxy`)

#### Subtask 4.1: `sync.Pool` Buffer Slab Implementation
- Create thread-safe buffer pools in [`pkg/httpparser`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser):
  - `var bufferPool = sync.Pool{ New: func() any { return bytes.NewBuffer(make([]byte, 0, 8*1024)) } }`
  - `GetBuffer() *bytes.Buffer`
  - `PutBuffer(buf *bytes.Buffer)`: resets buffer via `buf.Reset()` prior to returning to pool.
- Create thread-safe copy slice pool:
  - `var copyBufferPool = sync.Pool{ New: func() any { b := make([]byte, 32*1024); return &b } }`
  - `GetCopyBuffer() *[]byte`
  - `PutCopyBuffer(b *[]byte)`

#### Subtask 4.2: Zero-Copy Dual-Write Serialization Fast-Path
- In [`httpparser.Response.Serialize(w io.Writer)`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/response.go#L54):
  - Acquire a header buffer from `bufferPool`.
  - Format status line and sanitized headers into the header buffer.
  - Write header buffer bytes to `w`.
  - Return header buffer to `bufferPool`.
  - If `r.StreamBody == nil && r.Body.Len() > 0`:
    - Write payload directly to `w` via `_, err = r.Body.WriteTo(w)` or `_, err = w.Write(r.Body.Bytes())`, eliminating the monolithic memory allocation of status, headers, and body combined.

#### Subtask 4.3: Buffered Fallback Memory Slab Recycling
- In `ReverseProxy.ServeHTTPWithPrefix`, when falling back to buffering (`canStream == false`):
  - Use `copyBufferPool` with `io.CopyBuffer(res.Body, outResp.Body, *copyBuf)` to eliminate allocation churn during upstream body reads.

---

### Work Package 5 (WP-5): Automated Unit & Integration Testing

#### Subtask 5.1: Config & Timeout Decoupling Unit Tests
- In [`pkg/config/config_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config_test.go):
  - Verify default `StreamResponse == true` and `ResponseHeaderTimeout == 10s` under `profile: "raw_speed"`.
  - Verify default `StreamResponse == false` and `ResponseHeaderTimeout == 10s` under `profile: "balanced"`.
  - Verify route-level `stream_response: false` overrides global `raw_speed` default.
  - Verify route-level `response_header_timeout: 5s` cascades correctly.

#### Subtask 5.2: Reverse Proxy Hand-Off & Hop-by-Hop Filter Tests
- In [`pkg/proxy/proxy_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy_test.go):
  - Test `canStream` decision matrix (streaming MIME, `X-Accel-Buffering: no`, route compression/cache flags).
  - Verify that when `canStream == true`, `res.StreamBody != nil`, `res.Body.Len() == 0`, status code is copied, and hop-by-hop headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`) are strictly stripped.
  - Verify that cookie rewriting and redirect location rewriting function accurately in streaming hand-off mode.
  - Verify that when `canStream == false`, `res.StreamBody == nil` and `res.Body` contains full payload.

#### Subtask 5.3: Long-Lived Stream ($> 10\text{s}$) Integration Tests
- In [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go):
  - Spin up mock upstream sending Server-Sent Events over 12 seconds (e.g. 6 chunks spaced 2 seconds apart).
  - Send client GET request through Toron reverse proxy.
  - Assert that all 6 SSE chunks are received downstream without connection abort by `http.Client.Timeout`.
  - Assert that time-to-first-event is $\le 10\text{ms}$ (zero payload buffering delay).

#### Subtask 5.4: Middleware Compression & Cache Exemption Tests
- In `pkg/server/server_test.go` / `pkg/router/compression_test.go` / `pkg/router/cache_test.go`:
  - Configure a route with both `compression.enabled: true` and `cache.enabled: true`.
  - Send request resulting in `Content-Type: text/event-stream`:
    - Assert `res.Header.Get("Content-Encoding") == ""` (not compressed).
    - Assert `res.Header.Get("X-Cache") == "MISS"` and subsequent request is also forwarded upstream (not cached).
  - Send standard request resulting in `Content-Type: application/json`:
    - Assert response is compressed with gzip/zstd (`Content-Encoding` set).
    - Assert response is cached on subsequent request (`X-Cache: HIT`).

#### Subtask 5.5: HTTP/2 & Concurrency Race Safety Validation
- In [`pkg/server/http2_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/http2_test.go):
  - Verify SSE streaming through `http2AdapterHandler` with immediate flushing.
- Execute race detection across all modified packages:
  `go test -race -v ./pkg/config/... ./pkg/proxy/... ./pkg/router/... ./pkg/httpparser/... ./pkg/server/...`

---

## 3. Acceptance Criteria & Verification

### 3.1 Functional Acceptance Criteria
- [x] **Config Extension**: `ProxyTransportConfig` supports `StreamResponse *bool` and `ResponseHeaderTimeout time.Duration`. `raw_speed` defaults to `StreamResponse: true`, `balanced` to `false`. Route-level overrides cascade cleanly.
- [x] **Transport Decoupling**: Upstream dial is bounded by `opts.Timeout` and header receipt is bounded by `ResponseHeaderTimeout`, while `p.Client.Timeout = 0` allows response body streams to remain active indefinitely.
- [x] **WebSocket-Aligned Hand-Off**: `httpparser.Response` holds `StreamBody io.ReadCloser`. `ReverseProxy` hands off live streams to `res.StreamBody` when `canStream == true` without blocking on `io.Copy(res.Body, outResp.Body)`.
- [x] **Middleware Passthrough**: Streaming responses (`res.StreamBody != nil`) automatically bypass `CompressionMiddleware` and `CacheMiddleware` per [`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md).
- [x] **Server Socket Relay**: `Server.handleConnection` serializes headers to `conn` without static `Content-Length` and relays stream chunks using activity-refreshed write deadlines (matching `relayUpgradedStreams`). HTTP/2 adapter flushes chunks immediately.
- [x] **Buffer Recycling**: `sync.Pool` manages recycled `bytes.Buffer` and copy slices, eliminating dynamic heap allocations on streaming and buffered paths.

### 3.2 Non-Functional & Quality Criteria
- [x] **RFC Compliance**: RFC 7230 §6.1 hop-by-hop headers are stripped before the first streaming byte is transmitted. RFC 7234 cache rules and REQ-128 exemptions are strictly preserved.
- [x] **Long-Lived Resilience**: SSE streams lasting $> 10\text{s}$ stream continuously without premature termination.
- [x] **Zero Regressions**: Routes with active compression or caching continue to compress and cache standard responses without data corruption.
- [x] **Race & Thread Safety**: All unit and integration tests pass cleanly under `go test -race`.
