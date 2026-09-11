# Release Notes

## 2026-09-11 - Toron v1.5.19 Security Release (SEC-38: RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-38 (`pkg/acme/acme.go`)**: Successfully resolved Low-severity input validation, lock contention, and RFC non-compliance vulnerabilities [`SEC-38`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L539-L561) ([CWE-20](https://cwe.mitre.org/data/definitions/20.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-703](https://cwe.mitre.org/data/definitions/703.html), [`SR-091 Finding 8`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L235-L251), [`SR-100`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-100.md), [`CR-096`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-096.md)) in Toron's ACME HTTP-01 challenge responder.
- **Zero-Allocation RFC 8555 §8.3 Base64URL Validator (`IsValidACMEToken`)**: Implemented high-performance zero-allocation byte scanner enforcing $1 \le \text{len} \le 128$ and strictly the unpadded base64url character set (`[a-zA-Z0-9_-]`). Disallows padding (`=`), path separators (`/`, `\`), path traversal dots (`..`), whitespace, control characters, and non-ASCII bytes.
- **Fail-Fast Lock Isolation & DoS Elimination (CWE-20, CWE-400)**: Syntax and length validations execute prior to challenge registry lookup, preventing malformed, oversized, or adversarial request paths from acquiring `m.mu.RLock()` or triggering map hashing.
- **Elimination of Silent Whitespace Trimming**: Removed `strings.TrimSpace(token)`, ensuring tokens containing leading, trailing, or embedded whitespace strictly trigger `HTTP 400 Bad Request`.
- **HTTP Method Hardening (RFC 7231 §6.5.5 Compliance)**: Restricted `ServeHTTP01Handler` strictly to `GET` and `HEAD` methods. Disallowed methods (`POST`, `PUT`, `DELETE`, `PATCH`, `OPTIONS`, `CONNECT`, `TRACE`) immediately yield `405 Method Not Allowed` with mandatory headers `Allow: GET, HEAD` and `Content-Type: text/plain`.
- **RFC 7231 §4.3.2 Compliant HEAD Semantics**: Automated CA validation probes issuing `HEAD` requests receive `200 OK`, `Content-Type: text/plain`, and exact `Content-Length: len(keyAuth)` while strictly omitting the response body (`res.Body.Len() == 0`).
- **Clean 404 Response on Unregistered Valid Tokens**: Valid tokens not registered in the challenge table return `404 Not Found` with an explanatory error body for `GET` and an empty body for `HEAD`.
- **Zero External Dependencies**: Implemented strictly using the Go standard library (`crypto/*`, `net/http`, `strconv`, `strings`, `sync`, `time`) and internal `httpparser`.
- **Comprehensive Automated Verification Suite (`TC-100`)**: Verified across all 9 automated test scenarios in `pkg/acme/acme_test.go` (`TC-100-01` through `TC-100-09`) including high-concurrency race freedom under `go test -race` ([`TASK-123`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-123.md), [`TC-100`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-100.md)).

### Fixed
- **Unvalidated Token Map Queries & Core Lock Contention (`SEC-38`, CWE-20, CWE-400)**: Fixed vulnerability where external callers could query arbitrary paths against internal token map under read locks without validation.
- **Permissive HTTP Method Acceptance (`SEC-38`, RFC 8555)**: Fixed endpoint accepting non-idempotent or arbitrary HTTP verbs, now rejecting them with `405 Method Not Allowed`.
- **Missing RFC 7231 HEAD Method Support (`SEC-38`)**: Fixed handler writing response bodies on `HEAD` requests, ensuring body is omitted while `Content-Length` is preserved.
- **Silent Whitespace Forgiveness (`SEC-38`, CWE-20)**: Fixed handler masking invalid whitespace characters via `strings.TrimSpace`.

### Changed
- **ACME Core (`pkg/acme/acme.go`)**:
  - Added public `IsValidACMEToken(token string) bool`.
  - Updated `ServeHTTP01Handler` to validate method (`GET` and `HEAD` only), remove whitespace trimming, validate token syntax and length, guard response body for `HEAD`, and return appropriate status codes (`400`, `404`, `405`).

### Added
- **Automated Verification Suite (`pkg/acme/acme_test.go`)**:
  - `TestACME_HTTP01_ValidToken_GET_Success` (TC-100-01)
  - `TestACME_HTTP01_EmptyToken_Rejection` (TC-100-02)
  - `TestACME_HTTP01_TokenLengthBoundary` (TC-100-03)
  - `TestACME_HTTP01_InvalidCharacters_Rejection` (TC-100-04)
  - `TestACME_HTTP01_PathTraversal_Rejection` (TC-100-05)
  - `TestACME_HTTP01_MethodValidation_405MethodNotAllowed` (TC-100-06)
  - `TestACME_HTTP01_HEAD_Semantics` (TC-100-07)
  - `TestACME_HTTP01_NonExistentToken_404NotFound` (TC-100-08)
  - `TestACME_HTTP01_HighConcurrency_RaceSafety` (TC-100-09)
  - `TestACME_IsValidACMEToken` (Unit test)

### Related Tasks & Requirements
- [`TASK-123`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-123.md): RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler
- [`REQ-100`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-100.md): RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler
- [`ADR-100`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-100.md): RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler
- [`TC-100`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-100.md): Automated Verification Suite for RFC 8555 Token Syntax Validation and Method Hardening in ACME Handler
- [`CR-096`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-096.md): Code Review of RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler (SEC-38)
- [`SR-100`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-100.md): Security Review and Vulnerability Assessment of SEC-38 Remediation
- [`SEC-38`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L539-L561): Missing Token Syntax and Length Validation in ACME HTTP-01 Challenge Handler

## 2026-09-11 - Toron v1.5.18 Security Release (SEC-37: Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-37 (`pkg/sidecar`, `pkg/proxy`)**: Successfully resolved Medium-severity unbounded transport allocation, memory exhaustion, and socket descriptor leak vulnerabilities [`SEC-37`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L510-L518) ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-772](https://cwe.mitre.org/data/definitions/772.html), [`SR-091 Finding 7`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md), [`SR-099`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-099.md), [`CR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-095.md)) in the Service Mesh Sidecar Proxy engine.
- **Elimination of Per-Request ReverseProxy and Transport Allocation**: Replaced historical per-request instantiation of `*proxy.ReverseProxy`, `*http.Client`, and `*http.Transport` in `proxyToURL` with thread-safe origin-keyed caching in `ProxyEngine.proxies` (`map[string]*proxy.ReverseProxy`) using double-checked locking protected by `sync.RWMutex`. In-flight requests targeting cached origins proceed with lock-free read performance.
- **Canonical Target Origin Normalization ($O(U)$ Bounded Memory)**: Implemented `normalizeTargetOrigin(rawURL)` to extract canonical `scheme://host[:port]` keys, stripping variable dynamic paths, query parameters, and fragments. Memory utilization scales strictly with the number of unique upstream microservices ($O(U)$), completely preventing cache key explosion.
- **HTTP Keep-Alive Connection Pooling & TCP Socket Reuse**: Outbound egress traffic now reuses persistent keep-alive TCP connections across successive and concurrent requests directed at the same backend microservice ($\le 2$ active TCP sockets per backend under steady traffic), slashing connection latency to near zero and eliminating GC thrashing.
- **Idle Transport Connection Teardown (`ReverseProxy.Close()` & `ProxyEngine.Stop()`)**: Extended [`ReverseProxy.Close()`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L601) to invoke `tr.CloseIdleConnections()` on `p.Client.Transport.(*http.Transport)`. When [`ProxyEngine.Stop()`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L346) executes during pod shutdown, it cleanly iterates through all cached proxies and releases all idle TCP sockets, permanently eliminating file descriptor leaks (`EMFILE`).
- **Egress Client mTLS Propagation via `ProxyOptions.TLSClientConfig`**: Added `TLSClientConfig *tls.Config` to [`ProxyOptions`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L466), attaching pre-compiled client TLS configurations directly to cached reverse proxies. Egress HTTPS/mTLS connections now seamlessly present client certificates and validate custom CA roots in compliance with [`REQ-097`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-097.md) / `SEC-35`.
- **Zero External Dependencies**: Implemented strictly using the Go standard library (`sync`, `net/http`, `crypto/tls`, `net/url`, `io`, `fmt`, `time`, `strings`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-099`)**: Fully validated via unit and integration tests TC-099-01 through TC-099-07, confirming singleton proxy reuse across 50 sequential requests, multi-target backend isolation, 50-worker high-concurrency race freedom under `go test -race`, stop lifecycle teardown, egress client mTLS propagation, canonical origin normalization across 9 patterns, and `ReverseProxy.Close()` idle connection cleanup ([`TASK-122`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-122.md), [`TC-099`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-099.md)).

### Fixed
- **Unbounded Transport & Socket Descriptor Allocation (`SEC-37`, CWE-400, CWE-772)**: Fixed vulnerability where every outbound egress request created a new `*http.Transport` connection pool that was never reused or closed, leading to rapid socket descriptor exhaustion (`EMFILE`) and process crashes under load.
- **Missing Idle Connection Teardown on Proxy Teardown (`SEC-37`, CWE-772)**: Fixed `ReverseProxy.Close()` omission of `tr.CloseIdleConnections()`, which previously orphaned idle TCP sockets upon route removal or proxy closure.
- **Cache Key Proliferation Risk**: Preempted cache map bloating by normalizing raw URLs to canonical `scheme://host[:port]` origins before caching.
- **Egress Client mTLS Disconnect**: Fixed omission of client TLS configurations in `proxyToURL`, ensuring outbound egress proxying adheres to sidecar mTLS settings.
- **TLSClientConfig NextProtos Concurrency Data Race**: Resolved Go standard library data race during concurrent TLS handshakes by implementing two-tier defensive cloning (`p.clientTLS.Clone()` in `getOrCreateProxy` and `opts.TLSClientConfig.Clone()` in `NewProxyWithOptions`), ensuring each `http.Transport` operates on an independent `*tls.Config` instance.

### Changed
- **Reverse Proxy Core (`pkg/proxy/proxy.go`)**:
  - Extended `ProxyOptions` with `TLSClientConfig *tls.Config`.
  - Added defensive cloning in `NewProxyWithOptions` (`tr.TLSClientConfig = opts.TLSClientConfig.Clone()`) to prevent shared-pointer `NextProtos` data races across concurrent transports.
  - Added `tr.CloseIdleConnections()` invocation inside `ReverseProxy.Close()`.
- **Sidecar Proxy Engine (`pkg/sidecar/proxy.go`)**:
  - Extended `ProxyEngine` with `proxies map[string]*proxy.ReverseProxy`.
  - Added `getOrCreateProxy` with thread-safe double-checked locking using `sync.RWMutex` and defensive client TLS cloning (`p.clientTLS.Clone()`).
  - Pre-compiled client TLS configuration in `NewProxyEngine` and stored in `p.clientTLS`.
  - Updated `proxyToURL` to route requests through cached origin proxies.
  - Updated `ProxyEngine.Stop()` to iterate through and close all cached proxies.

### Added
- **Canonical Origin Normalization (`normalizeTargetOrigin`)**: Added URL parser utility in `pkg/sidecar/proxy.go` ensuring canonical `scheme://host[:port]` origin caching.
- **Automated Verification Suite (`pkg/sidecar/sidecar_test.go`, `pkg/proxy/proxy_test.go`)**:
  - `TestProxyEngine_ProxyCaching_SingletonPerOrigin` (TC-099-01): Verifies single reverse proxy and $\le 2$ TCP sockets across 50 sequential requests.
  - `TestProxyEngine_MultiTarget_Isolation` (TC-099-02): Verifies distinct backends maintain isolated `ReverseProxy` instances.
  - `TestProxyEngine_HighConcurrency_RaceClean` (TC-099-03): High-concurrency test running 50 workers issuing 1,000 requests clean under `go test -race`.
  - `TestProxyEngine_Stop_CleansUpAllProxies` (TC-099-04): Verifies clean proxy closure and map teardown on `ProxyEngine.Stop()`.
  - `TestProxyEngine_ClientTLS_Propagation` (TC-099-05): Verifies client certificate and CA validation propagation through cached proxy.
  - `TestProxyEngine_OriginNormalization` (TC-099-06): Validates 9 target URL patterns for origin normalization.
  - `TestReverseProxy_Close_ClosesIdleConnections` (TC-099-07): Verifies `ReverseProxy.Close()` closes idle transport connections.

### Related Tasks & Requirements
- [`TASK-122`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-122.md): Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy
- [`REQ-099`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-099.md): Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy
- [`ADR-099`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-099.md): ReverseProxy Caching, Transport Teardown, and Client mTLS Plumbing in Sidecar Proxy
- [`TC-099`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-099.md): Verification of Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy
- [`CR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-095.md): Code Review of Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy (SEC-37)
- [`SR-099`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-099.md): Security Review and Vulnerability Assessment of SEC-37 Remediation
- [`SEC-37`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L510-L518): Unbounded HTTP Client & Transport Allocation per Request in Service Mesh Sidecar Proxy

## 2026-09-11 - Toron v1.5.17 Security Release (SEC-36: Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-36 (`pkg/server/internal_api.go`)**: Successfully resolved Medium-severity memory exhaustion and socket descriptor leak vulnerabilities [`SEC-36`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L501-L509) ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), [`SR-091 Finding 6`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L197-L212), [`SR-098`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-098.md), [`CR-094`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-094.md)) in the control plane `POST /internal/api/proxy-test` endpoint.
- **Bounded Inbound Request Body Ingestion (64 KB Cap)**: Replaced unbounded `io.ReadAll(req.Body)` with `io.LimitReader(req.Body, maxRequestBodyBytes+1)`. Payloads exceeding 64 KB (`65,536` bytes) are immediately rejected with `HTTP 400 Bad Request` (`{"error":"400 Bad Request","message":"Request body exceeds maximum allowed size of 64KB"}`), permanently preventing heap exhaustion via oversized client requests.
- **Bounded Upstream Response Buffering & Deterministic Clamping**: Eliminated unbounded `io.ReadAll(httpResp.Body)`. Upstream response bodies are ingested via `io.LimitReader(httpResp.Body, maxResponseBytes+1)`. If the upstream body exceeds `maxResponseBytes`, it is deterministically clamped to the exact limit, preserving HTTP status codes and headers while signaling `"truncated": true` in the output JSON.
- **Configurable Response Ceiling (`MaxProxyTestResponseBytes`) with Safe 1 MB Default**: Added `MaxProxyTestResponseBytes int64` to `InternalAPIConfig`. If omitted, set to `0`, or configured with a negative value, the engine safely falls back to a 1 MB (`1,048,576` bytes) default ceiling.
- **Keep-Alive Connection Pool Reuse & Stream Draining (CWE-775 Remediation)**: Remediated file descriptor leaks (`EMFILE`) by safely draining up to 64 KB of residual upstream body data into `io.Discard` (`io.Copy(io.Discard, io.LimitReader(httpResp.Body, 64*1024))`) before deferred socket closure (`httpResp.Body.Close()`). This allows the underlying HTTP/1.1 TCP connection to be returned to Go's transport keep-alive pool for reuse rather than hanging or leaking.
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`io`, `net/http`, `encoding/json`, `fmt`, `time`, `strings`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-098`)**: Validated via unit and integration test cases TC-098-01 through TC-098-08 in `pkg/server/internal_api_test.go`, covering sub-limit responses, oversized response truncation, infinite chunked stream termination within memory bounds, custom response limits, zero/negative fallback to 1 MB, inbound request body cap enforcement (boundary, overflow, empty, invalid JSON), keep-alive socket reuse, and concurrent race-free execution under `go test -race` ([`TASK-121`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-121.md), [`TC-098`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-098.md)).

### Fixed
- **Unbounded Inbound Request Ingestion (`SEC-36`, CWE-400)**: Fixed vulnerability where incoming JSON request bodies on `POST /internal/api/proxy-test` were read into memory without length limits, allowing clients to trigger excessive heap allocations.
- **Memory Exhaustion via Unbounded Upstream Responses (`SEC-36`, CWE-400, CWE-770)**: Fixed vulnerability where probing an upstream target returning a multi-gigabyte or infinite stream (e.g. `/dev/urandom`, endless SSE) caused unbounded memory expansion until the OS Out-Of-Memory (OOM) killer aborted the Toron gateway.
- **Socket Descriptor Leaks & Connection Hangs (`SEC-36`, CWE-775)**: Fixed failure to drain residual bytes on truncated upstream responses, which previously prevented HTTP/1.1 transport connection recycling and caused `EMFILE` socket exhaustion.

### Changed
- **Internal API Proxy Test Probe (`pkg/server/internal_api.go`)**:
  - Implemented 64 KB limit reader on `req.Body` with `400 Bad Request` early rejection.
  - Implemented bounded reader on `httpResp.Body` constrained by `maxResponseBytes`.
  - Added deterministic clamping and residual stream discard draining.
- **Internal API Configuration (`pkg/server/internal_api.go`)**:
  - Extended `InternalAPIConfig` with `MaxProxyTestResponseBytes int64 `yaml:"max_proxy_test_response_bytes" json:"max_proxy_test_response_bytes"``.
- **Proxy Test Response Schema (`pkg/server/internal_api.go`)**:
  - Extended `ProxyTestResponse` with `Truncated bool `json:"truncated,omitempty"``.

### Added
- **Configuration Option (`max_proxy_test_response_bytes`)**: Added configurable byte ceiling for proxy test probe upstream response buffering.
- **Automated Verification Suite (`pkg/server/internal_api_test.go`)**:
  - `TestProxyTest_NormalResponse_UnderLimit` (TC-098-01): Verifies normal response under limit returns full body with `truncated: false`.
  - `TestProxyTest_OversizedResponse_Truncated` (TC-098-02): Verifies 5 MB response clamped to 1 MB default with `truncated: true`.
  - `TestProxyTest_InfiniteStream_BoundedTermination` (TC-098-03): Verifies infinite chunked stream is terminated cleanly within memory ceiling.
  - `TestProxyTest_CustomConfiguredLimit` (TC-098-04): Verifies custom limit (e.g. 2 KB) clamping.
  - `TestProxyTest_DefaultFallback_ZeroOrNegativeLimit` (TC-098-05): Verifies `<= 0` config falls back safely to 1 MB default.
  - `TestProxyTest_OversizedRequestBody_Rejection` (TC-098-06): Verifies 64 KB inbound cap enforcement, boundary handling, and rejection.
  - `TestProxyTest_SocketDrainAndConnectionReuse` (TC-098-07): Verifies TCP socket reuse after truncated response via stream draining.
  - `TestProxyTest_ConcurrentProbes_RaceClean` (TC-098-08): High-concurrency test running 20 parallel workers clean under `go test -race`.

### Related Tasks & Requirements
- [`TASK-121`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-121.md): Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe
- [`REQ-098`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-098.md): Bounded Inbound and Upstream Body Ingestion in Internal API Proxy Test Probe
- [`ADR-098`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-098.md): Bounded Request and Response Ingestion in Internal API Proxy Test Probe
- [`TC-098`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-098.md): Verification of Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe
- [`CR-094`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-094.md): Code Review of Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe (SEC-36)
- [`SR-098`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-098.md): Security Review and Vulnerability Assessment of SEC-36 Remediation
- [`SEC-36`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L501-L509): Memory Exhaustion via Unbounded Upstream Response Buffering in Internal API Proxy Test Probe

## 2026-09-11 - Toron v1.5.16 Security Release (SEC-35: Strict Sidecar Client TLS Certificate Validation and Explicit InsecureSkipVerify Opt-In)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-35 (`pkg/sidecar`, `pkg/config`)**: Successfully resolved High-severity improper certificate validation and silent Man-in-the-Middle (MitM) eavesdropping vulnerability [`SEC-35`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L481-L489) ([CWE-295](https://cwe.mitre.org/data/definitions/295.html), [`SR-091 Finding 5`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L173-L195), [`SR-097`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-097.md), [`CR-093`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-093.md)) in the Service Mesh Sidecar Proxy client TLS engine.
- **Elimination of Insecure Hardcoded Defaults (`pkg/sidecar/mtls.go`)**: Completely eliminated the historical architectural flaw in `BuildClientTLSConfig` where omitting an explicit internal CA certificate bundle (`ca_file: ""`) automatically set `tlsConfig.InsecureSkipVerify = true`. All outbound pod-to-pod egress connections now enforce strict peer certificate validation by default.
- **Seamless System Trust Root Fallback (`x509.SystemCertPool`)**: When `ca_file` is omitted or empty (`""`), `BuildClientTLSConfig` leaves `tlsConfig.RootCAs = nil`. The Go standard library `crypto/tls` runtime automatically falls back to validating peer certificates against the host operating system's system root certificate pool (`x509.SystemCertPool()`), enabling zero-configuration validation for public PKI, cloud certificates (AWS ACM, Cloudflare), and Let's Encrypt endpoints.
- **Custom Internal CA Bundle Support**: Retained full enterprise PKI support via `ca_file`. When specified, root certificates are loaded into a dedicated `x509.CertPool` assigned to `tlsConfig.RootCAs`, guaranteeing end-to-end zero-trust validation for internal service mesh certificate authorities.
- **Explicit `insecure_skip_verify` Opt-In & Mandatory Warning Log (`pkg/config/config.go`, `pkg/sidecar/mtls.go`)**: Added explicit opt-in boolean field `InsecureSkipVerify` to [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103) (`insecure_skip_verify`), defaulting to `false`. When explicitly set to `true` for non-production environments, a high-visibility audit warning is emitted to the server log: `[SIDECAR] WARNING: InsecureSkipVerify is enabled for sidecar egress TLS. Certificate verification is disabled.`
- **Cryptographic Protocol Floor (`tls.VersionTLS12`)**: Strictly enforces `MinVersion: tls.VersionTLS12` across all client TLS configurations, completely preventing protocol downgrade attacks to SSLv3, TLS 1.0, or TLS 1.1.
- **Mutual TLS (mTLS) Client Identity Preservation**: Supports client certificate keypairs (`cert_file`, `key_file`) loaded via `tls.LoadX509KeyPair`, enabling client identity authentication during outbound egress handshakes.
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`crypto/tls`, `crypto/x509`, `fmt`, `log`, `os`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-097`)**: Fully validated via unit and end-to-end test cases TC-097-01 through TC-097-07, verifying secure default rejection of self-signed/expired/invalid certificates, successful system trust root and custom CA validation, explicit opt-in behavior with warning logs, mTLS client keypair presentation, TLS 1.2 minimum version enforcement, and high-concurrency race cleanliness under `go test -race` ([`TASK-120`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-120.md), [`TC-097`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-097.md)).

### Fixed
- **Insecure Default `InsecureSkipVerify = true` in Sidecar Client TLS (`SEC-35`, CWE-295)**: Fixed severe flaw where omitting `ca_file` defaulted `InsecureSkipVerify = true`, completely disabling certificate verification and exposing outbound inter-service traffic to silent Man-in-the-Middle (MitM) eavesdropping, tampering, and credential theft on shared container networks.
- **Failure to Fall Back to Host System Trust Roots**: Fixed issue where omitting `ca_file` prevented the sidecar from utilizing host OS trust roots for public certificate validation without disabling security checks.

### Changed
- **Sidecar Client TLS Constructor (`pkg/sidecar/mtls.go`)**:
  - Eliminated the `else { tlsConfig.InsecureSkipVerify = true }` branch in `BuildClientTLSConfig`.
  - Assigned `tlsConfig.InsecureSkipVerify = cfg.InsecureSkipVerify`.
  - Configured `tlsConfig.RootCAs = nil` when `cfg.CAFile == ""` to enable automatic host OS trust pool fallback.
  - Added high-visibility warning logging when `cfg.InsecureSkipVerify == true`.
- **Sidecar Configuration Schema (`pkg/config/config.go`)**:
  - Extended `SidecarConfig` with `InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"``.
  - Initialized `InsecureSkipVerify: false` in `defaultConfig()` and `DefaultAppConfig()`.

### Added
- **Configuration Option (`insecure_skip_verify`)**: Added explicit opt-in boolean in `toron.yaml` (`sidecar.insecure_skip_verify`) for non-production debugging environments.
- **Automated Verification Suite (`pkg/sidecar/sidecar_test.go`)**:
  - `TestBuildClientTLSConfig_DefaultSecure` (TC-097-01): Verifies `InsecureSkipVerify == false` and `RootCAs == nil` by default.
  - `TestSidecar_EgressTLS_HandshakeRejection` (TC-097-02): Verifies immediate handshake failure and HTTP 502 rejection when encountering untrusted/self-signed upstream certificates under default settings.
  - `TestBuildClientTLSConfig_CustomCA` & `TestSidecar_EgressTLS_HandshakeSuccess_WithCustomCA` (TC-097-03): Verifies successful handshake and proxying when upstream certificate is signed by configured `ca_file`.
  - `TestBuildClientTLSConfig_ExplicitInsecureOptIn` & `TestSidecar_EgressTLS_HandshakeSuccess_WithExplicitOptIn` (TC-097-04): Verifies successful bypass and mandatory audit warning when `insecure_skip_verify: true`.
  - `TestBuildClientTLSConfig_ClientCertKeypair` (TC-097-05): Verifies loading of client mTLS certificate and keypair into `tlsConfig.Certificates`.
  - `TestBuildClientTLSConfig_MinVersionTLS12` (TC-097-06): Verifies strict `tls.VersionTLS12` minimum protocol enforcement.
  - `TestSidecar_EgressTLS_ConcurrentRouting_RaceClean` (TC-097-07): High-concurrency egress proxy routing test clean under `go test -race`.

### Related Tasks & Requirements
- [`TASK-120`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-120.md): Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS
- [`REQ-097`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-097.md): Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS Configuration
- [`ADR-097`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-097.md): Secure Default Client TLS Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Proxy
- [`TC-097`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-097.md): Verification of Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS Configuration
- [`CR-093`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-093.md): Code Review of Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS Configuration
- [`SR-097`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-097.md): Security Review and Vulnerability Assessment of SEC-35 Remediation
- [`SEC-35`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L481-L489): Insecure Default InsecureSkipVerify in Sidecar Client TLS Configuration

## 2026-09-11 - Toron v1.5.16 Release (REQ-096 / TASK-119: Composite Route Key Grouping, Multi-Pod Target Aggregation, Canary Variant Isolation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller)

### Milestone Summary
- **Parity with OCI Discovery Engine (`pkg/ingress`, `pkg/router`)**: Established full architectural parity between Toron's Kubernetes Ingress Controller and the OCI Container Discovery engine ([`REQ-095`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-095.md), [`ADR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-095.md), [`CR-091`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-091.md)), eliminating Canary target pool contamination, route shadowing, and single-pod starvation in Kubernetes environments ([`TASK-119`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-119.md), [`REQ-096`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-096.md), [`ADR-096`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-096.md), [`CR-092`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-092.md), [`SR-096`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-096.md)).
- **4-Dimensional Route Partitioning via `CompositeRouteKey` (`pkg/ingress/controller.go`)**: Partitioned Ingress routes using a 4-dimensional tuple `CompositeRouteKey = (Host, CleanPrefix, Method, CanonicalHeaders)`. Multiple Ingress definitions sharing the same domain and path prefix but differing in header criteria (e.g. Canary vs Baseline) or HTTP methods produce distinct composite keys, preventing target pool pollution and cross-variant traffic leakage.
- **Deterministic Alphabetical Header Canonicalization (`canonicalizeHeaders`)**: Implemented deterministic header sorting and lowercase normalization across all header keys (`a-env=staging&z-version=v2`), eliminating Go's non-deterministic `map[string]string` traversal and preventing false route churn across repeated 30-second reconciliation passes.
- **Strict Canary vs Baseline Target Segregation (Zero Traffic Bleed)**: Fully resolved the critical issue where Canary and Baseline pod endpoints were merged into a single load balancer pool. Canary Ingresses (`nginx.ingress.kubernetes.io/canary: "true"` or `toron.io/headers`) and Baseline Ingresses maintain separate reverse proxy pools. Traffic bearing canary headers routes exclusively to Canary pods, while standard traffic routes exclusively to Baseline pods.
- **Dual-Ecosystem Annotation Support (`pkg/ingress/translator.go`)**:
  - **Native Toron Annotations**: Supports `toron.io/method` (normalized uppercase HTTP verbs), `toron.io/header.<Name>` (individual header matches), and `toron.io/headers` (supporting both JSON object and CSV key=value formats) with additive merging and override precedence.
  - **Industry-Standard NGINX Canary Annotations**: Full compatibility with `nginx.ingress.kubernetes.io/canary: "true"`, `nginx.ingress.kubernetes.io/canary-by-header`, and `nginx.ingress.kubernetes.io/canary-by-header-value` (defaulting to `"always"` if omitted), enabling seamless zero-code migrations of existing Kubernetes Canary manifests.
- **ADR-005 Specificity-Based Route Ordering & Anti-Shadowing (`pkg/ingress/controller.go`)**: Enforced a strict 5-tier specificity hierarchy (Longest prefix $\to$ Specific host $\to$ Header constraint count $\to$ Method constraint $\to$ Deterministic tie-break) on all Ingress route specifications (`router.SortPrefixRouteSpecs(specs)`). Guaranteed that header-constrained Canary routes evaluate ahead of generic fallback Baseline routes, permanently eliminating route shadowing regardless of the discovery order returned by the Kubernetes API server.
- **Multi-Pod Target Aggregation & Fair Round-Robin Load Balancing**: Aggregated all pod IP endpoints for each composite key into a unified multi-target `PrefixRouteSpec` using [`RoundRobinBalancer`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L30), eliminating single-pod overload and distributing load evenly ($\approx 1/M$ per replica) across all active pod replicas.
- **Non-Destructive Scale-Down & Clean Eviction Teardown**: Pod scaling down updates the reverse proxy target pool in place with zero HTTP 404 errors or connection drops. Deleting an Ingress purges its routes and cleanly invokes `proxy.Close()`, stopping active health check tickers (`StopActiveHealthCheck`) and closing idle TCP connection pools, preventing socket descriptor leaks (`EMFILE`).
- **Declarative HTTP Method Filtering Support**: Populated `PrefixRouteSpec.Method` from `toron.io/method`, enforcing HTTP verb constraints at runtime in `router.ServeHTTP` and returning HTTP `405 Method Not Allowed` on method mismatches or falling through to method-compatible fallback routes.
- **Zero External Dependencies**: Pure Go standard library implementation (`sync`, `net/http`, `net/url`, `sort`, `strings`, `encoding/json`, `time`, `context`, `path`, `fmt`, `strconv`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-096`)**: Validated Ingress annotation parsing, deterministic composite key generation, multi-pod target aggregation, strict Canary/Baseline segregation, ADR-005 specificity sorting without Canary shadowing, declarative method filtering, non-destructive scale-down with zero downtime, and concurrent race-clean execution under `go test -race` ([`TASK-119`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-119.md), [`TC-096`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-096.md)).

### Fixed
- **Canary Target Pool Pollution & Traffic Bleed (`REQ-096`, CWE-284)**: Fixed bug where Canary and Baseline pod endpoints were grouped using a naive 2-tuple `(Host, Prefix)`, causing Canary and Baseline pods to be lumped into a single reverse proxy target pool and sending unvalidated canary code to normal users ($\approx 33\text{--}50\%$).
- **Canary Route Shadowing via Non-Deterministic Arrival Order (`REQ-096`, CWE-284)**: Fixed issue where Ingress routes were evaluated in arbitrary Kubernetes API arrival order, allowing generic Baseline routes to intercept requests and shadow header-constrained Canary routes.
- **Missing Ingress Routing Dimension Support**: Fixed inability to specify HTTP methods and headers on Kubernetes Ingresses, bringing complete parity with OCI container discovery.
- **Single-Pod Starvation & Horizontal Scaling Collapse (`CWE-400`)**: Fixed single-pod monopolization by aggregating pod targets into round-robin load balancers.
- **Socket Descriptor & Health Checker Leaks (`CWE-775`)**: Fixed resource leaks where deleted Ingresses abandoned active health check tickers in heap memory.

### Changed
- **Ingress Route Translation (`pkg/ingress/translator.go`)**:
  - Implemented annotation extraction for `toron.io/method`, `toron.io/header.<Name>`, `toron.io/headers` (CSV and JSON), and `nginx.ingress.kubernetes.io/canary*`.
  - Populated `DiscoveredRoute.Method` and `DiscoveredRoute.Headers`.
- **Ingress Controller Dynamic Reconciliation (`pkg/ingress/controller.go`)**:
  - Introduced 4-dimensional `CompositeRouteKey` and deterministic `canonicalizeHeaders`.
  - Aggregated and deduplicated pod endpoints into multi-target `PrefixRouteSpec`s.
  - Added ADR-005 specificity sorting on compiled specs (`router.SortPrefixRouteSpecs(specs)`) prior to atomic replacement.

### Added
- **Supported Ingress Annotations**:
  - `toron.io/method`
  - `toron.io/header.<Name>`
  - `toron.io/headers`
  - `nginx.ingress.kubernetes.io/canary`
  - `nginx.ingress.kubernetes.io/canary-by-header`
  - `nginx.ingress.kubernetes.io/canary-by-header-value`
- **Automated Verification Suites (`scratch/tc_096_full_test_suite.go`, `pkg/ingress/ingress_test.go`)**:
  - `TestTranslateIngress_MethodAndHeaderAnnotations` (TC-096-01): 10 table-driven scenarios validating method, individual/grouped headers, NGINX canary defaults/custom values, override precedence, malformed JSON fallback, and backward compatibility.
  - `TestIngressController_CompositeRouteKeyAggregation` (TC-096-02): Deterministic canonicalization and key differentiation across 100 iterations.
  - `TestIngressController_CompositeRouteKeyAggregation` (TC-096-03): Multi-pod target aggregation, deduplication, and 33.3% round-robin distribution.
  - `TestIngressController_CanaryBaselineStrictSegregation` (TC-096-04): 100% Canary/Baseline isolation with zero traffic bleed.
  - `TestIngressController_SpecificityOrdering_NoCanaryShadowing` (TC-096-05): Adverse discovery order test confirming Canary route evaluation ahead of Baseline.
  - `TestIngressController_PrefixRouteSpec_MethodFiltering` (TC-096-06): Declarative method filtering and HTTP 405 Method Not Allowed handling.
  - `TestIngressController_PodScaleDown_ZeroDowntime` (TC-096-07): Zero-downtime scale-down (zero 404s) and clean Ingress deletion proxy teardown.
  - `TestIngressController_ConcurrentEventsAndRouting_RaceClean` (TC-096-08): High-concurrency test running 5 churn workers + 20 traffic workers clean under `go test -race`.

### Related Tasks & Requirements
- [`TASK-119`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-119.md): Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- [`REQ-096`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-096.md): Composite Route Key Grouping, Multi-Pod Target Aggregation, Canary Variant Isolation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- [`ADR-096`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-096.md): Composite Route Key Grouping, Multi-Pod Target Aggregation, Canary Variant Isolation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- [`ADR-005`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-005.md): Route Specificity and Path Matching Precedence
- [`ADR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-089.md): Source-Tagged Atomic Prefix Routing & Multi-Pod Ingress Endpoint Aggregation
- [`TC-096`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-096.md): Verification of Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- [`CR-092`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-092.md): Code Review of Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- [`SR-096`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-096.md): Security Review of Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller

## 2026-09-11 - Toron v1.5.15 Security Release (SEC-34: Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-34 (`pkg/discovery`, `pkg/router`)**: Successfully resolved Medium-severity premature route deletion, denial-of-service outage on replica scale-down, single-replica traffic starvation, route shadowing, and routing dimension collision vulnerability [`SEC-34`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L465-L473) ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-284](https://cwe.mitre.org/data/definitions/284.html), [CWE-662](https://cwe.mitre.org/data/definitions/662.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), [`SR-091 Finding 4`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L154-L171), [`SR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-095.md), [`CR-091`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-091.md)) in the OCI Container Auto-Discovery Engine and Core Router.
- **4-Dimensional Route Partitioning via `CompositeRouteKey` (`pkg/discovery/manager.go`)**: Established multi-dimensional route partitioning based on a 4-tuple `CompositeRouteKey = (Host, CleanPrefix, Method, CanonicalHeaders)`. Containers sharing identical host and prefix but possessing different HTTP methods or header rules (such as Canary deployments with `X-Version: canary` vs baseline deployments, or `POST` vs `GET`) produce distinct composite keys, preventing variant collisions and cross-tenant traffic leakage ([`TASK-118`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-118.md), [`REQ-095`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-095.md), [`ADR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-095.md)).
- **Deterministic Alphabetical Header Canonicalization**: Solved Go's pseudo-random `map[string]string` iteration non-determinism by sorting header keys alphabetically (`sort.Strings(keys)`) and serializing them into a canonical query-string representation (`key1=val1&key2=val2`). Guarantees invariant composite key strings across reconciliation passes and eliminates false route churn.
- **Multi-Replica Target Aggregation & Fair Round-Robin Load Balancing (`pkg/discovery/manager.go`, `pkg/proxy/proxy.go`)**: Aggregated container replicas sharing an identical `CompositeRouteKey` into a unified multi-target `PrefixRouteSpec` using [`RoundRobinBalancer`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L30). Eliminated the prior single-pod overload defect (where linear first-match prefix search sent 100% load to replica #1) and distributes traffic evenly ($\approx 1/M$ per replica) across all active instances.
- **Non-Destructive Partial Scale-Down (Zero-Downtime Guarantee)**: Eliminated the critical bug where stopping 1 container replica triggered `RemovePrefixRoute` and purged all prefix routes for that service. The discovery engine now recalculates `desiredSpecs` across surviving replicas and performs atomic source-scoped replacement via `router.ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)`. Stopping 1 replica out of $M$ updates the target list to $M-1$ in place with **zero HTTP 404 errors**, zero connection drops, and zero transient downtime.
- **ADR-005 Specificity-Based Route Ordering & Anti-Shadowing (`pkg/router/router.go`)**: Enforced a strict 5-tier specificity hierarchy (Longest prefix $\to$ Specific host $\to$ Header constraint count $\to$ Method constraint $\to$ Deterministic tie-break). Unconstrained fallback routes can never shadow more specific canary or method-gated routes, regardless of container discovery arrival or registration order.
- **Declarative Method Matching Support in Prefix Routing**: Added first-class `Method string` support to [`PrefixRouteSpec`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L172-L179), normalizing uppercase verbs and rejecting mismatched verbs with HTTP `405 Method Not Allowed` or falling through to method-compatible routes.
- **Clean Reverse Proxy & Health Check Teardown**: Evicted routes automatically invoke `pr.proxy.Close()` out of write lock, terminating active background health check ticker goroutines (`StopActiveHealthCheck`) and closing idle TCP connection sockets, eliminating socket descriptor (`EMFILE`) and goroutine leaks.
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`sync`, `net/http`, `net/url`, `sort`, `strings`, `encoding/json`, `time`, `context`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-095`)**: Validated model and label parsing, deterministic composite key generation, multi-replica target aggregation, non-destructive partial scale-down, distinct canary variant separation, ADR-005 specificity ordering without canary shadowing, declarative method matching, atomic eviction teardown, and high-concurrency race cleanliness under `go test -race` ([`TASK-118`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-118.md), [`TC-095`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-095.md)).

### Fixed
- **Premature Route Deletion & Total Outage on Replica Stop (`SEC-34`, CWE-662, CWE-284)**: Fixed bug where stopping or restarting a single container replica called `RemovePrefixRoute`, which deleted ALL routes sharing `(host, prefix)`, inducing an immediate total outage (HTTP 404) for all surviving healthy replicas.
- **Horizontal Scaling Defeat & Single-Pod Overload (`SEC-34`, CWE-400)**: Fixed issue where containers were registered as independent single-target routes, causing router first-match prefix search to route 100% of requests to replica #1 and starve replicas #2..$M$.
- **Routing Dimension Blindness & Canary Collisions**: Fixed inability to route based on HTTP headers and methods in container discovery, preventing canary containers from being merged with baseline containers or overwriting baseline routes.
- **Route Shadowing via Arbitrary Insertion Order**: Fixed router evaluation order to enforce ADR-005 specificity ranking, preventing broad generic routes from intercepting requests destined for specific canary routes.
- **Reverse Proxy and Health Check Resource Leaks (`SEC-34`, CWE-775)**: Fixed resource leaks where evicted container routes left reverse proxy health checkers running in background goroutines.

### Changed
- **Discovered Route Model (`pkg/discovery/provider.go`)**: Extended `DiscoveredRoute` struct with `Method string` and `Headers map[string]string`. Updated `ActiveRoutes()` snapshot method to expose these fields for telemetry.
- **Label Parser (`pkg/discovery/parser.go`)**: Enhanced `ParseContainerLabels` to extract `toron.method`, individual `toron.header.<Name>`, and grouped `toron.headers` (CSV and JSON formats) with additive merging, override precedence, and graceful panic-free fallback.
- **Discovery Manager Reconciliation (`pkg/discovery/manager.go`)**:
  - Replaced incremental `RoutePrefix` / `RemovePrefixRoute` calls with atomic declarative reconciliation using `m.router.ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)`.
  - Partitioned active containers using `CompositeRouteKey` with deterministic header canonicalization.
  - Aggregated multi-replica target URLs into unified round-robin reverse proxy configurations.
  - Implemented lock-inversion-free synchronization (`m.mu` released prior to router invocation).
- **Router Prefix Routing Engine (`pkg/router/router.go`)**:
  - Extended `PrefixRouteSpec` with `Method string`.
  - Implemented 5-tier specificity sorting (`comparePrefixRoutes`, `comparePrefixRouteSpecs`, `SortPrefixRouteSpecs`).
  - Added out-of-lock reverse proxy teardown on evicted routes (`pr.proxy.Close()`).
  - Exposed `Method` in `RouteSnapshot` for administrative and telemetry parity.

### Added
- **Container Discovery Labels (`pkg/discovery/parser.go`)**:
  - Added `toron.method` label for HTTP method constraint routing.
  - Added `toron.header.<Name>` and `toron.headers` labels for HTTP header constraint routing.
  - Added `toron.health_check_interval` label for configurable health check probing frequency.
- **Router Sorting & Declarative API (`pkg/router/router.go`)**:
  - Added `SortPrefixRouteSpecs` public helper function.
  - Added `Method` field to `PrefixRouteSpec` and `RouteSnapshot`.
- **Automated Verification Suites (`pkg/discovery/discovery_test.go`, `pkg/router/router_test.go`)**:
  - `TestParseContainerLabels_MethodAndHeaders` (TC-095-01): Verifies parsing of method, header labels, CSV and JSON formats, overrides, and fallbacks.
  - `TestDiscoveryManager_CompositeRouteKeyCanonicalization` (TC-095-02): Verifies deterministic 4D composite route keys and map iteration independence.
  - `TestDiscoveryManager_CompositeRouteKeyAggregation` (TC-095-03): Verifies multi-replica target aggregation, deduplication, and round-robin balancing across replicas.
  - `TestDiscoveryManager_NonDestructivePartialScaleDown` (TC-095-04): Verifies zero-downtime partial scale-down with continuous traffic forwarding on remaining replicas.
  - `TestDiscoveryManager_DistinctVariantCanarySeparation` (TC-095-05): Verifies strict target isolation between baseline and canary deployments.
  - `TestRouter_SpecificityOrdering_NoCanaryShadowing` (TC-095-06): Verifies ADR-005 5-tier specificity hierarchy and anti-shadowing proof.
  - `TestRouter_PrefixRouteSpec_MethodMatching` (TC-095-07): Verifies declarative method matching and 405 / fallback routing.
  - `TestDiscoveryManager_AtomicReplacementLifecycle` (TC-095-08): Verifies atomic route eviction and clean reverse proxy teardown.
  - `TestDiscoveryManager_ConcurrentLifecycleAndRouting_RaceClean` (TC-095-09): High-concurrency test running background container churn against parallel client traffic, clean under `go test -race`.

### Related Tasks & Requirements
- [`TASK-118`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-118.md): Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle
- [`REQ-095`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-095.md): Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- [`ADR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-095.md): Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- [`ADR-005`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-005.md): Route Specificity and Path Matching Precedence
- [`TC-095`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-095.md): Verification of Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- [`CR-091`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-091.md): Code Review of Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- [`SR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-095.md): Security Review and Vulnerability Assessment of SEC-34 Remediation
- [`SEC-34`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L465-L473): Premature Route Deletion & Load-Balancing Failure Across Multi-Replica Containers in OCI Discovery Engine

## 2026-09-10 - Toron v1.5.14 Security Release (SEC-33: Bounded Route Table Lifecycle, Atomic Route Replacement, and Multi-Target Pod Aggregation in Kubernetes Ingress Controller)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-33 (`pkg/ingress`, `pkg/router`)**: Successfully resolved High-severity unbounded route table growth, zombie route persistence, stale endpoint shadowing, and pod replica starvation vulnerability [`SEC-33`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L449-L457) ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-670](https://cwe.mitre.org/data/definitions/670.html), [CWE-1059](https://cwe.mitre.org/data/definitions/1059.html), [`SR-091 Finding 3`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L130-L152), [`SR-094`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-094.md), [`CR-090`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-090.md)) in the Kubernetes Ingress Controller and Core Router engine.
- **Source-Tagged Prefix Routing & Atomic Route Table Replacement (`pkg/router/router.go`)**: Extended prefix routing to support subsystem origin tagging (e.g. `"k8s-ingress"`, `"config"`, `"static"`). Introduced declarative route specifications ([`PrefixRouteSpec`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)) and an atomic route replacement API ([`ReplacePrefixRoutesBySource`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)). Routes are pre-compiled and validated out of lock with fail-fast rollbacks, and atomically swapped under write lock, guaranteeing all-or-nothing cutovers without route churn or request disruption ([`TASK-116`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md), [`REQ-094`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md), [`ADR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-089.md)).
- **Bounded Memory Invariant ($O(K)$ Memory Scaling)**: Eliminated monotonic append-only route table growth across periodic resync cycles (every 30s) and watch event notifications. For $K$ active Ingress rules, the prefix route count for source `"k8s-ingress"` strictly equals $K$ across arbitrary $N$ synchronization iterations ($O(1)$ memory scaling with resync count), permanently preventing memory exhaustion, GC stalls, and Out-of-Memory (OOM) gateway terminations ([`TASK-117`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-117.md), [`REQ-094`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md)).
- **Automatic Zombie Route Elimination & Immediate Deletion Pruning**: Replaced manual route tracking with dynamic reconciliation. When an Ingress or path is deleted in Kubernetes, subsequent reconciliation cycles omit the deleted route from the desired batch, causing `ReplacePrefixRoutesBySource` to immediately evict the route from the routing table. Subsequent HTTP requests return HTTP `404 Not Found`, eliminating traffic leakage to decommissioned backends.
- **Multi-Pod Endpoint Aggregation & Fair Round-Robin Load Balancing (`pkg/ingress/translator.go`, `pkg/ingress/controller.go`)**: Aggregated multiple pod endpoint IPs sharing `(Host, Prefix)` into a unified multi-target reverse proxy route using [`RoundRobinBalancer`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L30). Eliminated single-pod replica starvation (where replica #1 previously received 100% load) and distributed traffic evenly ($\approx 1/M$ per replica) across all active pods.
- **Immediate Endpoint Cutover Without Stale Route Shadowing**: Rolling updates, pod restarts, and scale events immediately swap endpoint target lists in place. Stale routes are evicted rather than appended to the end of the routing table, guaranteeing immediate cutover with zero traffic sent to terminated pod IPs.
- **Clean Background Resource Teardown**: Replaced and evicted prefix routes cleanly invoke `.Close()` on associated `ReverseProxy` instances, terminating active background health check ticker goroutines (`StopActiveHealthCheck`) and releasing idle connection pools, preventing socket descriptor exhaustion (EMFILE).
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`sync`, `net/http`, `net/url`, `time`, `context`, `strings`, `path`, `path/filepath`, `fmt`, `log`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-094`)**: Validated unit source isolation, atomic empty-slice pruning, rollback on invalid specs, controller sync bounds across 50 cycles, zombie 404 pruning, 3-pod round-robin balancing, endpoint cutover without shadowing, health check teardown, and high-concurrency race cleanliness under `go test -race` ([`TASK-116`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md), [`TASK-117`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-117.md), [`TC-094`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-094.md)).

### Fixed
- **Unbounded Route Table Memory Leak (`SEC-33`, CWE-400)**: Fixed append-only route registration where every 30-second periodic resync and watch event appended duplicate prefix routes to `r.prefixRoutes`, causing monotonic memory growth of ~144,000 duplicate entries/day per 50 routes and eventual OOM crash.
- **Zombie Route Persistence (`SEC-33`, CWE-670)**: Fixed issue where deleted Ingress resources remained in Toron's routing table indefinitely, proxying client traffic to decommissioned or reassigned backend pods.
- **Stale Endpoint Shadowing**: Fixed routing anomalies during pod restarts and rollouts where updated endpoints were appended to the end of `r.prefixRoutes` and permanently shadowed by obsolete routes at the head of the linear matching table.
- **Horizontal Pod Replica Starvation**: Fixed single-target route generation in Ingress translation where multiple pod endpoints created separate routes, causing Toron's prefix matcher to route 100% of traffic to the first pod and starve all other replicas.
- **Background Goroutine and Socket Descriptor Leaks**: Fixed resource leakage where discarded reverse proxy instances left background health check tickers and connection pools running.

### Changed
- **Router Subsystem (`pkg/router/router.go`)**:
  - Extended internal `prefixRoute` struct with `source string` and `proxy *proxy.ReverseProxy`.
  - Added public declarative specification struct `PrefixRouteSpec`.
  - Implemented `RoutePrefixWithSource(source, ...)` and delegated legacy `RoutePrefix(...)` to `RoutePrefixWithSource("config", ...)` for 100% backward compatibility.
  - Implemented atomic batch replacement `ReplacePrefixRoutesBySource(source string, specs []PrefixRouteSpec) error` with pre-compilation out of lock, atomic pointer swap under write lock, and automatic resource teardown on evicted routes.
  - Added lifecycle teardown calling `.Close()` on evicted routes in `ReplacePrefixRoutesBySource`, `RemovePrefixRoute`, and `Reset`.
  - Exposed `Source` field in `RouteSnapshot` and `GetPrefixRoutes()` for telemetry and auditing.
- **Ingress Controller Reconciliation (`pkg/ingress/controller.go`)**:
  - Refactored `syncIngresses` to aggregate endpoints by unique routing tuple `(Host, CleanPrefix)` into `router.PrefixRouteSpec` with round-robin load balancing across all healthy pod targets.
  - Synchronized routes dynamically via `c.router.ReplacePrefixRoutesBySource("k8s-ingress", specs)` on every resync and watch event.
  - Serialized reconciliation cycles using `c.syncMu` to eliminate interleaving races between resync tickers and watch event streams.
- **Ingress Route Translation (`pkg/ingress/translator.go`)**:
  - Updated endpoint resolution to aggregate all pod IP addresses per subset into unified multi-target configurations, falling back to cluster Service DNS when endpoints are unavailable.

### Added
- **Router Declarative API & Telemetry (`pkg/router/router.go`)**:
  - Added `PrefixRouteSpec` struct for declarative prefix route provisioning.
  - Added `ReplacePrefixRoutesBySource` and `RoutePrefixWithSource` methods to `Router`.
  - Added `Source` field to `RouteSnapshot` for telemetry, auditing, and observability.
- **Automated Verification Suites (`pkg/router/router_test.go`, `pkg/ingress/ingress_test.go`)**:
  - `TestRouter_ReplacePrefixRoutesBySource_SourceIsolation` (TC-094-01): Verifies atomic replacement alters only matching source, leaves other sources intact, and prunes on empty slice.
  - `TestRouter_ReplacePrefixRoutesBySource_PreCompilationValidation` (TC-094-02): Verifies all-or-nothing rollback on invalid route specs without modifying active route state.
  - `TestRouter_RoutePrefix_BackwardCompatibility` (TC-094-03): Verifies legacy callers are tagged as `"config"` and retain 100% functional equivalence.
  - `TestRouter_ReplacePrefixRoutes_StopsActiveHealthCheck` (TC-094-04): Verifies active background health checks on evicted routes are terminated cleanly.
  - `TestRouter_RemovePrefixRoute_StopsActiveHealthCheck` & `TestRouter_Reset_StopsActiveHealthCheck`: Verifies teardown on individual route deletion and router reset.
  - `TestIngressController_BoundedRouteTable` (TC-094-05): Verifies route count remains strictly equal to $K$ across 50 consecutive sync cycles ($O(1)$ memory scaling with sync count).
  - `TestIngressController_ZombieRoutePruning` (TC-094-06): Verifies deleted Ingresses are immediately pruned and return HTTP 404 Not Found.
  - `TestIngressController_MultiPodLoadBalancing` (TC-094-07): Verifies requests to 3 pod replicas distribute evenly across all replicas ($\approx 33.3\%$ per pod).
  - `TestIngressController_EndpointUpdateNoShadowing` (TC-094-08): Verifies immediate cutover to new pod IPs without stale route shadowing.
  - `TestIngressController_ConcurrentSyncAndRouting_RaceClean` (TC-094-09): High-concurrency test running background churn against parallel client traffic, clean under `go test -race`.

### Related Tasks & Requirements
- [`TASK-116`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md): Router Source-Tagged Prefix Routing and Atomic Route Replacement API
- [`TASK-117`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-117.md): Kubernetes Ingress Controller Route Table Dynamic Reconciliation and Multi-Target Pod Aggregation
- [`REQ-094`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md): Bounded Route Table Lifecycle, Atomic Source Replacement, and Multi-Target Pod Aggregation in Kubernetes Ingress Controller
- [`ADR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-089.md): Source-Tagged Atomic Prefix Routing & Multi-Pod Ingress Endpoint Aggregation
- [`TC-094`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-094.md): Verification of Kubernetes Ingress Route Table Lifecycle, Atomic Source Replacement, and Multi-Pod Balancing
- [`CR-090`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-090.md): Code Review of Bounded Route Table Lifecycle, Atomic Source Replacement, and Multi-Target Pod Aggregation in Kubernetes Ingress Controller
- [`SR-094`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-094.md): Security Review and Vulnerability Assessment of SEC-33 Remediation
- [`SEC-33`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L449-L457): Unbounded Routing Table Memory Leak & Zombie Route Persistence in Kubernetes Ingress Controller

## 2026-09-10 - Toron v1.5.13 Security Release (SEC-32: Fail-Closed WAF IP Access Control on Unidentifiable Client IP)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-32 (`pkg/waf/middleware.go`, `pkg/waf/ip_acl.go`)**: Successfully resolved fail-open WAF IP access control bypass vulnerability [`SEC-32`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L439-L447) ([CWE-284](https://cwe.mitre.org/data/definitions/284.html), [CWE-1188](https://cwe.mitre.org/data/definitions/1188.html), [CWE-693](https://cwe.mitre.org/data/definitions/693.html), [`SR-091 Finding 2`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L102-L127), [`SR-093`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-093.md), [`CR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-089.md)) in the WAF middleware and IP ACL evaluation engine.
- **Fail-Closed Allowlist Perimeter Enforcement (`pkg/waf/middleware.go`, `pkg/waf/ip_acl.go`)**: Enforced strict fail-closed access control when incoming client IP cannot be determined under an active IP allowlist (`allowed_ips` or `allowedSubnets`). Requests with missing, stripped, untrusted, or malformed IP headers are immediately rejected with HTTP `403 Forbidden` and exact JSON payload `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}` ([`TASK-114`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-114.md), [`REQ-093`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md), [`ADR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-088.md)).
- **Fail-Open Pass-Through for Denylist-Only Mode (`pkg/waf/ip_acl.go`)**: Preserved negative security model semantics when only `denied_ips` is configured without an active allowlist. Requests with unidentifiable client IP (`nil`) evaluate to `allowed: true, reason: ""` in [`CheckIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L76-L87) and pass through the IP ACL stage to subsequent WAF inspection layers, eliminating false-positive outages across internal service meshes, synthetic health probes, or intermediate proxies.
- **Single-Pass Hot-Path IP Extraction Optimization (`pkg/waf/middleware.go`)**: Eliminated redundant sequential invocations of [`ExtractClientIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L181-L207) on the request hot path. Middleware extracts client IP once at entry, caching `clientNetIP net.IP` on the stack for unconditional `acl.CheckIP(clientNetIP)` evaluation and string `clientIP` across all telemetry and audit events ([`TASK-114`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-114.md), [`ADR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-088.md)).
- **Unconditional IP ACL Delegation**: Removed the vulnerable `if ip != nil` guard in WAF middleware, delegating policy authority unconditionally to [`IPAccessList.CheckIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L76-L87) whenever `acl != nil && acl.HasRules()` evaluates to `true`.
- **Safe Telemetry & Structured Audit Logging**: Guaranteed zero-panic emission of `ip_acl_block` [`SecurityEvent`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/audit.go#L28-L43) records with `ClientIP: ""` (empty string) and metric increment `toron_waf_blocked_requests_total{category="ip_acl"}` on fail-closed rejections.
- **Zero External Dependencies**: Implemented entirely with Go standard library packages (`net`, `net/http`, `strings`, `bytes`, `sync`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-093`)**: Validated unit ACL checks, end-to-end middleware fail-closed rejection, denylist pass-through, malformed header matrix rejection, panic-free audit logging, telemetry parity, and high-concurrency race cleanliness under `go test -race` ([`TASK-115`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-115.md), [`TC-093`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-093.md)).

### Fixed
- **Fail-Open Allowlist Bypass on Missing Client IP (`SEC-32`, `REQ-093`)**: Fixed critical vulnerability where requests lacking resolvable client IP (`ExtractClientIP` returned `nil`) bypassed allowlist checks due to `if ip != nil` guard in `middleware.go` and `CheckIP(nil)` returning `true, ""` (fail-open) in `ip_acl.go`.
- **Hot-Path Redundant IP Extraction**: Fixed duplicate invocations of `ExtractClientIP(req, tp)` in WAF middleware closure, eliminating duplicate socket splitting and CIDR matching overhead on high-throughput routes.
- **Malformed Header Exploitation**: Fixed handling of corrupt, non-IP, or unparseable headers (`RemoteAddr`, `X-Forwarded-For`, `X-Real-IP`), ensuring they evaluate safely to `nil` and trigger fail-closed 403 Forbidden responses under active allowlists.

### Changed
- **WAF IP ACL Engine (`pkg/waf/ip_acl.go`)**: Refactored [`CheckIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L76-L87) to inspect active rules when `ip == nil`. If `len(acl.allowedSubnets) > 0 || len(acl.allowedIPs) > 0`, it returns `allowed: false, reason: "client IP could not be determined and allowed IP list is enforced"`; if only denylists or no rules are configured, it returns `allowed: true, reason: ""` (fail-open).
- **WAF Middleware Pipeline (`pkg/waf/middleware.go`)**:
  - Replaced duplicate `ExtractClientIP` calls with single-pass extraction at closure entry (`clientNetIP := ExtractClientIP(req, tp)`).
  - Removed `if ip != nil` evaluation guard; `acl.CheckIP(clientNetIP)` is invoked unconditionally whenever `acl != nil && acl.HasRules()`.
  - On denial (`!allowed`), immediately halts processing without invoking `next(req, res)`, sets status to `403 Forbidden`, sets `Content-Type: application/json`, writes exact JSON error payload `{"error":"Forbidden","message":"..."}`, increments Prometheus metric `WAFBlocked("ip_acl", req.Path)`, and emits structured `SecurityEvent`.

### Added
- **Automated Verification Suites (`pkg/waf/ip_acl_test.go`, `pkg/waf/middleware_test.go`)**:
  - `TestIPAccessList_CheckIP_NilIP_WithAllowlist` (TC-093-01): Unit test verifying `CheckIP(nil)` returns `allowed == false` and exact reason across IPv4 CIDRs, exact IPv4, IPv6 CIDRs, exact IPv6, and combined allow/deny sets.
  - `TestIPAccessList_CheckIP_NilIP_DenylistOnly` & `TestIPAccessList_CheckIP_NilIP_NoRules` (TC-093-02): Unit tests verifying fail-open pass-through for `CheckIP(nil)` when only denylists are present, empty ACLs, or nil receiver pointer.
  - `TestWAFMiddleware_NilClientIP_AllowlistEnforced` (TC-093-03): End-to-end middleware test verifying HTTP 403 Forbidden, exact JSON response body `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}`, immediate execution halt, and Prometheus metric increment on missing client IP.
  - `TestWAFMiddleware_NilClientIP_DenylistOnly` (TC-093-04): End-to-end middleware test verifying pass-through to downstream handler (HTTP 200) for unidentifiable client IP under denylist-only mode.
  - `TestWAFMiddleware_MalformedClientIP_AllowlistEnforced` (TC-093-05): Table-driven test evaluating 9 malformed address variants (`"not-an-ip:9999"`, `":::invalid"`, `"hostname-without-ip"`, `"unknown"`, `"localhost"`, `"999.999.999.999"`, `"garbage-header"`, `"invalid-real-ip"`, `"[bad-ipv6"`), verifying all evaluate safely to `nil` and fail closed with HTTP 403.
  - `TestWAFMiddleware_AuditLogging_NilIP_Blocked` (TC-093-06): Verification of structured audit event emission (`ip_acl_block`, `ClientIP: ""`) on nil IP block with zero panics or nil-pointer dereferences.
  - `TestWAFMiddleware_SinglePassExtraction_TelemetryParity` (TC-093-07): Verifies single-pass extraction parity across allowed IP (`200 OK`), denied IP (`403 Forbidden`), and nil IP (`403 Forbidden`).
  - `TestWAFMiddleware_NilClientIP_Concurrency` & `TestWAFMiddleware_ConcurrentRaceClean` (TC-093-08): 100 concurrent workers dispatching 5,000 requests across mixed IP scenarios under `-race`, verifying zero data races and zero deadlocks.

### Related Tasks & Requirements
- [`TASK-114`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-114.md): WAF Middleware Single-Pass IP Extraction and Fail-Closed Enforcement
- [`TASK-115`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-115.md): End-to-End WAF Middleware IP ACL Automated Verification Suite (TC-093)
- [`REQ-093`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md): Fail-Closed WAF IP Access Control Enforcement on Unidentifiable Client IP
- [`ADR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-088.md): Fail-Closed WAF IP Access Control & Single-Pass Extraction
- [`TC-093`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-093.md): Test Suite for Fail-Closed WAF IP Access Control on Unidentifiable Client IP
- [`CR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-089.md): Code Review of Fail-Closed WAF IP Access Control Enforcement on Unidentifiable Client IP
- [`SR-093`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-093.md): Security Review and Vulnerability Assessment of SEC-32 Remediation
- [`SEC-32`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L439-L447): Fail-Open WAF IP Access Control Bypass on Unidentifiable Client IP

## 2026-09-10 - Toron v1.5.12 Security Release (SEC-31: Physical RemoteAddr Binding & Ingress Anti-Spoofing across HTTP/1.1, HTTP/2, and HTTP/3)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-31 (`pkg/httpparser`, `pkg/server`, `pkg/waf`, `pkg/router`, `pkg/proxy`, `pkg/logging`)**: Successfully resolved unauthenticated client IP spoofing and perimeter security bypass vulnerability [`SEC-31`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L428-L436) ([CWE-290](https://cwe.mitre.org/data/definitions/290.html), [CWE-345](https://cwe.mitre.org/data/definitions/345.html), [CWE-693](https://cwe.mitre.org/data/definitions/693.html), [`SR-091 Finding 1`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L78-L99), [`SR-092`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-092.md)).
- **Physical `RemoteAddr` Binding in HTTP/2 and HTTP/3 Protocol Adapters**: Extended [`httpparser.Request`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/request.go) with an immutable `RemoteAddr string` field and panic-safe extraction helpers `RemoteHost()` and `RemoteIP()`. HTTP/2 streams (`http2AdapterHandler`) and HTTP/3 QUIC datagrams (`ListenAndServeH3`) bind `r.RemoteAddr` at ingress, and native HTTP/1.1 connections bind `conn.RemoteAddr().String()` in `handleConn`, establishing full protocol parity across all transports ([`TASK-111`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-111.md), [`REQ-092`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-092.md), [`ADR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-087.md)).
- **Perimeter Hardening & Trusted Proxy Gating (`pkg/waf`, `pkg/server`, `pkg/router`, `pkg/proxy`, `pkg/logging`)**: Client-supplied `X-Forwarded-For` and `X-Real-IP` headers are rejected and stripped unless the physical peer IP is verified against configured `trusted_proxies` CIDR blocks:
  - **WAF IP ACL**: Evaluates physical peer IP first; untrusted headers cannot bypass blacklists or allowlists; fails secure on unresolvable IPs.
  - **Internal Management API**: Rejects untrusted HTTP/2 and HTTP/3 subnet spoofing with `403 Forbidden` (`{"error":"403 Forbidden","message":"Access denied by administrative subnet policy"}`).
  - **Token Bucket Rate Limiting**: Neutralized header rotation DoS attacks by removing insecure `req.RawConn == nil` fallbacks; untrusted connections are contained within a single `ip:<remoteHost>` bucket.
  - **Reverse Proxy**: Strips spoofed `X-Forwarded-For` and `X-Real-IP` headers from untrusted connections, replacing them with verified physical `peerIP`; safely appends `peerIP` for verified `trusted_proxies`.
  - **Structured Access Logging**: Records true physical client IP, preventing audit trail falsification.
- **Preserved Mobile Roaming Session Affinity (`pkg/proxy/sticky.go`)**: Maintained zero changes (0 diffs) in `sticky.go`, preserving application-level session persistence across cellular IP handovers and carrier CGNAT transitions pursuant to [`REQ-030`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-030.md).
- **Zero External Dependencies**: Implemented strictly using standard library packages (`net`, `net/http`, `strings`, `sync`).
- **Comprehensive Automated Verification Suite (`TC-092`)**: Validated physical address binding, multi-protocol parity, WAF anti-spoofing, internal API subnet gates, rate limiter single-bucket containment, reverse proxy sanitization, HTTP/3 QUIC datagram parity, mobile roaming affinity, and concurrency under `go test -race` ([`TASK-113`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-113.md), [`TC-092`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-092.md)).

### Added
- **Request Model Fields & Helpers (`pkg/httpparser/request.go`)**:
  - Added `RemoteAddr string` to [`httpparser.Request`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/request.go).
  - Added `RemoteHost() string` with safe port splitting and `RawConn` fallback.
  - Added `RemoteIP() net.IP` supporting IPv4, bracketed/unbracketed IPv6, and nil safety.
- **Automated Verification Suite (`pkg/server/server_anti_spoofing_test.go`, `pkg/httpparser/request_test.go`, `pkg/waf/ip_acl_test.go`, `pkg/router/rate_limiter_test.go`, `pkg/proxy/proxy_test.go`, `pkg/proxy/sticky_test.go`, `pkg/logging/manager_test.go`)**:
  - `TestRequest_RemoteHostAndIP_Parsing`: Table-driven tests for IPv4, IPv6, port splitting, bracket stripping, whitespace, and fallbacks.
  - `TestRequest_NewRequestFromStd_RemoteAddr`: Standard request conversion remote address preservation.
  - `TestServer_Ingress_RemoteAddrPopulation_H1` & `TestServer_Ingress_RemoteAddrPopulation_H2`: Transport ingress remote address binding.
  - `TestServer_H2_WAF_SpoofingRejected` & `TestWAF_IPACL_AntiSpoofing`: WAF blacklist/allowlist anti-spoofing rejection and fail-secure verification.
  - `TestServer_H2_InternalAPI_SubnetEnforcement`: Internal API administrative subnet spoofing rejection and fail-closed default.
  - `TestServer_H2_RateLimiter_AntiSpoofing` & `TestServer_H2_RateLimiter_Integration`: Rate limiter header rotation containment.
  - `TestServer_H2_ReverseProxy_HeaderSanitization` & `TestServer_H2_ReverseProxy_TrustedProxyAppended`: Reverse proxy header stripping and trusted proxy appending.
  - `TestServer_H3_RemoteAddrBinding`: HTTP/3 QUIC datagram peer address binding.
  - `TestLogging_ExtractClientIP_PhysicalBinding`: Structured logging client IP physical binding.
  - `TestSticky_PreserveMobileRoamingAffinity`: Sticky session mobile roaming regression test (REQ-030).
  - `TestServer_AntiSpoofing_ConcurrentRaceClean`: 100 concurrent workers dispatching 5,000 requests under `-race`.

### Changed
- **Ingress Protocol Adapters (`pkg/server/server.go`, `pkg/httpparser/request.go`)**: Bound physical socket addresses in `handleConn`, `http2AdapterHandler`, and `ListenAndServeH3`.
- **Perimeter Security Modules (`pkg/waf`, `pkg/server`, `pkg/router`, `pkg/proxy`, `pkg/logging`)**: Prioritized physical connection metadata and enforced `trusted_proxies` verification before accepting client-supplied IP headers.

### Related Tasks & Requirements
- [`TASK-111`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-111.md): RemoteAddr Binding & Extraction Helpers in httpparser.Request and Ingress Protocol Adapters
- [`TASK-112`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-112.md): Security Perimeter Hardening & Trusted Proxy Gating across WAF, Internal API, Rate Limiter, Reverse Proxy, and Logging
- [`TASK-113`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-113.md): Comprehensive Automated Verification Suite for Anti-Spoofing & Protocol Parity (TC-092)
- [`REQ-092`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-092.md): Unauthenticated Client IP Spoofing and Security Bypass via Missing Physical RemoteAddr Binding in HTTP/2 and HTTP/3 Adapters
- [`ADR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-087.md): Physical RemoteAddr Binding and Ingress Anti-Spoofing across HTTP/1.1, HTTP/2, and HTTP/3
- [`TC-092`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-092.md): Test Suite for Physical RemoteAddr Binding and Ingress Anti-Spoofing
- [`SEC-31`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L428-L436): Unauthenticated Client IP Spoofing & Security Bypass via Missing Physical RemoteAddr Binding in HTTP/2 and HTTP/3 Adapters
- [`SR-092`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-092.md): Security Review and Vulnerability Assessment of SEC-31 Remediation
- [`CR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-088.md): Code Review of Physical RemoteAddr Binding & Ingress Anti-Spoofing across HTTP/1.1, HTTP/2, and HTTP/3

## 2026-09-09 - Toron v1.5.11 Security Release (SEC-30: Direct Parameterized Subpath Routing and Empty Prefix Proxy Elimination in REST-to-gRPC Transcoder)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-30 (`pkg/transcoder`, `pkg/router`)**: Resolved Subpath Routing Interception and Denial of Service vulnerability [`SEC-30`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L411-L419) ([CWE-284](https://cwe.mitre.org/data/definitions/284.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [`SR-081 Finding 8`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L185-L202), [`SR-090`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-090.md)) in the REST-to-gRPC Transcoding Engine and Edge Router.
- **Router Method-Aware Prefix Routing & Path Matcher API (`pkg/router/router.go`)**: Extended [`Router`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go) with [`HandlePrefix`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go) and [`HandlePrefixWithMatcher`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go), allowing in-process Go handlers to be registered directly on path prefixes with HTTP method gating and custom path matchers ([`TASK-108`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-108.md)).
- **Elimination of Empty Upstream Reverse Proxies (`pkg/transcoder/transcoder.go`)**: Completely removed dummy upstream reverse proxy registration with 0 targets (`RoutePrefix("upstream", ...)`) that previously caused all parameterized REST requests to abort with `502 Bad Gateway: No upstream target available` ([`TASK-109`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-109.md)).
- **Direct Parameterized Dispatch & Pattern Matching (`pkg/transcoder/transcoder.go`)**: Implemented `MatchPathPattern` to validate literal segments and wildcard parameter tokens. Parameterized routes (`/v1/users/:id`, `/v1/users/:id/orders/:orderId`) dispatch directly through [`Router.ServeHTTP`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go) to `HandleTranscode` with `200 OK` responses ([`TASK-109`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-109.md)).
- **Multi-Level Route Segregation & Method Gating**: Multiple routes sharing common path prefixes are cleanly segregated without route shadowing; invalid methods return `405 Method Not Allowed`, and segment count mismatches return `404 Not Found` without upstream leakage ([`TASK-108`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-108.md), [`TASK-109`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-109.md)).
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`strings`, `net/http`, `sync`, `net/url`).
- **Automated Verification Suite (`pkg/transcoder/transcoder_test.go`, `pkg/router/router_test.go`)**: Validated direct parameterized subpath dispatch through `router.ServeHTTP` (fulfilling Missing Security Test 5 in `SR-081`), multi-level route segregation, 405 method mismatch, 404 segment mismatch, and concurrency under `go test -race` ([`TASK-110`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-110.md), [`TC-091`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-091.md)).

### Added
- **Router APIs (`pkg/router/router.go`)**: `HandlePrefix(method, prefix string, handler HandlerFunc)` and `HandlePrefixWithMatcher(method, host, prefix string, headers map[string]string, matcher func(path string) bool, handler HandlerFunc)`.
- **Transcoder Helper (`pkg/transcoder/transcoder.go`)**: `MatchPathPattern(pattern, path string) bool`.
- **Automated Tests (`pkg/router/router_test.go`, `pkg/transcoder/transcoder_test.go`)**:
  - `TestRouter_HandlePrefix_MethodAndMatcher`: Router prefix handler and matcher dispatching.
  - `TestTranscoder_ParameterizedSubpathDispatch` (TC-091-01): Verifies parameterized `GET /v1/users/usr-777` dispatches through `router.ServeHTTP` returning 200 OK.
  - `TestTranscoder_MultiLevelRouteSegregation` (TC-091-02): Verifies shared prefix route segregation.
  - `TestTranscoder_WrongMethodOnParameterizedRoute` (TC-091-03): Verifies 405 Method Not Allowed.
  - `TestTranscoder_SegmentCountMismatch_NotFound` (TC-091-04): Verifies 404 Not Found.
  - `TestTranscoder_MatchPathPattern` (TC-091-05): Unit test coverage for pattern matcher.
  - `TestTranscoder_ParameterizedSubpath_ConcurrencyRaceSafety` (TC-091-06): 50 concurrent requests under `-race`.

### Changed
- **Transcoder Route Registration (`pkg/transcoder/transcoder.go`)**: Replaced dummy upstream proxy with direct `HandlePrefixWithMatcher` binding for parameterized routes and `Handle` for exact routes.

### Related Tasks & Requirements
- [`TASK-108`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-108.md): Router Method-Aware Prefix Routing & Path Matcher Support
- [`TASK-109`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-109.md): Direct Parameterized Subpath Routing & Empty Upstream Proxy Elimination in Transcoder
- [`TASK-110`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-110.md): Automated Verification Suite for Parameterized Transcoder Subpath Dispatch
- [`REQ-091`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-091.md): Direct Parameterized Subpath Routing and Elimination of Empty Prefix Proxy in REST-to-gRPC Transcoder
- [`ADR-086`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-086.md): Direct Parameterized Subpath Routing and Elimination of Empty Prefix Proxy in REST-to-gRPC Transcoder
- [`TC-091`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-091.md): Test Suite for Parameterized Subpath Routing and Empty Prefix Proxy Elimination
- [`SEC-30`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L411-L419): Subpath Routing Interception & 502 Denial in REST-to-gRPC Transcoder
- [`SR-090`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-090.md): Security Review of SEC-30 Remediation
- [`CR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-087.md): Code Review of Direct Parameterized Subpath Routing and Empty Prefix Proxy Elimination

## 2026-09-09 - Toron v1.5.10 Security Release (SEC-29: Hop-by-Hop Header Sanitization and Strict RFC 7540/9113 Protocol Compliance in REST-to-gRPC Transcoder)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-29 (`pkg/transcoder`)**: Resolved protocol error Denial-of-Service and HTTP request smuggling vulnerability [`SEC-29`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L403-L411) ([CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-436](https://cwe.mitre.org/data/definitions/436.html), [`SR-081 Finding 7`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L162-L171), [`SR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-089.md)) in the REST-to-gRPC Transcoding Engine, preventing hop-by-hop header leakage to upstream gRPC backends.
- **Static Hop-by-Hop Header Sanitization (`pkg/transcoder/transcoder.go`)**: Strips all standard RFC 7230 §6.1 / RFC 7540 §8.1.2.2 / RFC 9113 §8.2.2 connection-specific headers (`Connection`, `Keep-Alive`, `Upgrade`, `Proxy-Connection`, `Transfer-Encoding`, `Proxy-Authenticate`, `Proxy-Authorization`, `Trailer`, `Trailers`, `Host`) before dispatching HTTP/2 gRPC requests ([`TASK-105`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-105.md)).
- **Dynamic Connection Token Parsing (`pkg/transcoder/transcoder.go`)**: Dynamically parses comma-delimited tokens in client `Connection` headers and strips matching headers per RFC 7230 §6.1 / RFC 9110 §7.6.1 ([`TASK-105`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-105.md)).
- **Strict `TE: trailers` Invariant Enforcement (`pkg/transcoder/transcoder.go`)**: Discards client `TE` values (e.g., `gzip`, `deflate`) and strictly enforces single-valued `TE: trailers` (RFC 7540 §8.1.2.2), preventing upstream gRPC backends from terminating streams with `RST_STREAM (PROTOCOL_ERROR 0x1)` ([`TASK-106`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-106.md)).
- **Host Header Sanitization & Metadata Preservation (`pkg/transcoder/transcoder.go`)**: Strips client `Host` header to avoid host spoofing and upstream authority mismatch, while preserving legitimate application authentication and tracing metadata (`Authorization`, `X-Request-Id`, `Traceparent`, `User-Agent`) intact ([`TASK-106`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-106.md)).
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`strings`, `net/http`, `sync`).
- **Automated Verification Suite (`pkg/transcoder/transcoder_test.go`)**: Validated standard hop-by-hop stripping, dynamic token stripping, TE trailers invariant, Host header stripping, metadata preservation, and concurrency under `go test -race` ([`TASK-107`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-107.md), [`TC-090`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-090.md)).

### Added
- **Automated Tests (`pkg/transcoder/transcoder_test.go`)**:
  - `TestHandleTranscode_StandardHopByHop_Stripped`: Verifies stripping of standard hop-by-hop headers.
  - `TestHandleTranscode_DynamicConnectionTokens_Stripped`: Verifies dynamic token extraction from Connection header and stripping.
  - `TestHandleTranscode_TE_StrictTrailersInvariant`: Verifies single-valued `TE: trailers` enforcement.
  - `TestHandleTranscode_HostHeader_Stripped`: Verifies client Host header removal.
  - `TestHandleTranscode_ApplicationMetadata_Preserved`: Verifies preservation of authentication and tracing headers.
  - `TestHandleTranscode_HeaderSanitization_ConcurrencyRaceSafety`: 50 concurrent requests under `-race`.

### Changed
- **REST-to-gRPC Header Forwarding (`pkg/transcoder/transcoder.go`)**: Filtered out all standard and dynamic hop-by-hop headers, enforced canonical `Content-Type: application/grpc` and `TE: trailers`.

### Related Tasks & Requirements
- [`TASK-105`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-105.md): Static & Dynamic Hop-by-Hop Header Sanitization in Transcoder
- [`TASK-106`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-106.md): Strict TE: trailers Invariant & Metadata Preservation in Transcoder
- [`TASK-107`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-107.md): Automated Verification Suite for Transcoder Protocol Header Compliance
- [`REQ-090`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-090.md): Hop-by-Hop Header Sanitization and Strict Protocol Invariant Enforcement in REST-to-gRPC Transcoder
- [`ADR-085`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-085.md): Hop-by-Hop Header Sanitization and Canonical gRPC Wire Compliance in Transcoder
- [`TC-090`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-090.md): Test Suite for Transcoder Hop-by-Hop Header Sanitization and Protocol Compliance
- [`SEC-29`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L403-L411): Hop-by-Hop Header Leakage to Upstream in REST-to-gRPC Transcoder
- [`SR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-089.md): Security Review of SEC-29 Remediation
- [`CR-086`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-086.md): Code Review of REST-to-gRPC Transcoder Hop-by-Hop Header Sanitization and Protocol Compliance

## 2026-09-09 - Toron v1.5.9 Security Release (SEC-28: Bounded Request Body Ingestion and 413 Payload Too Large Rejection in REST-to-gRPC Transcoder)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-28 (`pkg/transcoder`, `pkg/config`)**: Resolved Out-Of-Memory (OOM) Denial of Service vulnerability [`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401) ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), [`SR-081 Finding 6`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L151-L160), [`SR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-088.md)) in the REST-to-gRPC Transcoding Engine, eliminating unbounded heap allocation from oversized request bodies.
- **Configurable Transcoder Request Body Limit (`MaxBodyBytes`)**: Added `MaxBodyBytes int64` to [`TranscoderConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L130) (`yaml:"max_body_bytes,omitempty" json:"max_body_bytes,omitempty"`), defaulting to `4MB` (`4194304` bytes) via [`GetMaxBodyBytes()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) to match [`DefaultMaxGRPCFrameSize`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/framer.go#L11) and [`ServerConfig.MaxBodyBytes`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) ([`TASK-101`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-101.md), [`REQ-089`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-089.md), [`ADR-084`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-084.md)).
- **Declared `Content-Length` Fast-Fail Rejection (`pkg/transcoder/transcoder.go`)**: Implemented pre-read check on `req.ContentLength`. Incoming requests declaring payload size $> \text{maxBodyBytes}$ are rejected immediately with `HTTP 413 Payload Too Large` without buffer allocation or socket reading ([`TASK-102`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-102.md)).
- **Bounded Stream Over-Read Protection (`pkg/transcoder/transcoder.go`)**: Replaced unbounded `io.ReadAll(req.Body)` with `io.LimitReader(req.Body, maxBody+1)`. Chunked or undeclared streams exceeding the limit are rejected with `HTTP 413` and skip `json.Unmarshal`, preventing heap memory explosion ([`TASK-103`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-103.md)).
- **Upstream Backend Isolation**: Verified that under all 413 rejection pathways, zero calls are forwarded to the upstream gRPC backend.
- **Full-Fidelity In-Limit Forwarding & Non-Mutating Bypass**: Legitimate payloads $\le \text{maxBodyBytes}$ and non-mutating `nil` body requests (`GET`, `DELETE`) pass through transparently.
- **Zero External Dependencies**: Pure Go standard library implementation (`io`, `net/http`, `encoding/json`, `fmt`, `strconv`, `sync`).
- **Automated Verification Suite (`pkg/transcoder/transcoder_test.go`, `pkg/config/config_test.go`)**: Validated declared Content-Length fast-fail, stream over-read rejection, default fallback, in-limit forwarding, and concurrency under `go test -race` ([`TASK-104`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-104.md), [`TC-089`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-089.md)).

### Added
- **Configuration Fields**: `MaxBodyBytes int64` in [`TranscoderConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L130) with `GetMaxBodyBytes()` helper method.
- **Automated Tests (`pkg/transcoder/transcoder_test.go`, `pkg/config/config_test.go`)**:
  - `TestTranscoderConfig_MaxBodyBytesSchemaAndValidation`: Schema defaults, fallback, YAML/JSON unmarshaling, negative value rejection.
  - `TestHandleTranscode_DeclaredContentLength_FastFail`: Fast-fail 413 on declared oversized `Content-Length`.
  - `TestHandleTranscode_StreamOverRead_Rejection`: Bounded stream over-read rejection with 413.
  - `TestHandleTranscode_InLimit_Success`: In-limit payload parsed and forwarded with full fidelity.
  - `TestHandleTranscode_NonMutatingNilBody_Bypass`: Safe handling of `req.Body == nil`.
  - `TestHandleTranscode_ConcurrencyRaceSafety`: 50 concurrent requests under `-race`.

### Changed
- **REST Request Ingestion (`pkg/transcoder/transcoder.go`)**: Replaced raw `io.ReadAll` with two-tier fast-fail and `io.LimitReader` bounded reading with HTTP 413 rejection.

### Related Tasks & Requirements
- [`TASK-101`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-101.md): Configurable Transcoder Request Body Limit Schema
- [`TASK-102`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-102.md): Fast-Fail Rejection on Declared Content-Length in Transcoder
- [`TASK-103`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-103.md): Bounded Stream Over-Read Protection & 413 Rejection in Transcoder
- [`TASK-104`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-104.md): Automated Verification Suite for REST-to-gRPC Transcoder Request Body Limits
- [`REQ-089`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-089.md): Bounded Request Body Ingestion and 413 Payload Too Large Rejection in REST-to-gRPC Transcoder
- [`ADR-084`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-084.md): Bounded Request Body Ingestion and 413 Rejection Architecture in REST-to-gRPC Transcoder
- [`TC-089`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-089.md): Verification Suite for REST-to-gRPC Transcoder Request Body Limits
- [`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401): Unbounded Request Body Ingestion in REST-to-gRPC Transcoder
- [`SR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-088.md): Security Review of SEC-28 Remediation
- [`CR-085`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-085.md): Code Review of REST-to-gRPC Transcoder Request Body Limiting and 413 Rejection

## 2026-09-09 - Toron v1.5.8 Security Release (SEC-27: Maximum Idle Deadline Enforcement on Upgraded Protocol and WebSocket Connections)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-27 (`pkg/server`, `pkg/config`)**: Resolved critical Slowloris resource exhaustion vulnerability [`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392) ([CWE-400: Uncontrolled Resource Consumption](https://cwe.mitre.org/data/definitions/400.html), [`SR-081 Finding 5`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L142-L150), [`SR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-087.md)) in core HTTP/1.1 protocol upgrade handling and HTTP/2 Extended CONNECT streams, eliminating permanent socket and goroutine leaks.
- **Configurable Upgraded Inactivity Deadline (`UpgradeIdleTimeout`)**: Added `UpgradeIdleTimeout time.Duration` to [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go) and [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) (`yaml:"upgrade_idle_timeout,omitempty" json:"upgrade_idle_timeout,omitempty"`), defaulting to `60s` with a 3-tier fallback hierarchy ([`TASK-097`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-097.md), [`REQ-088`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-088.md), [`ADR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-083.md)).
- **Bidirectional Activity-Refreshed Deadline Relay (`pkg/server/server.go`)**: Replaced permanent deadline stripping (`SetDeadline(time.Time{})`) and unbounded `io.Copy` with an active deadline relay (`relayUpgradedStreams`). Every transferred chunk refreshes read and write deadlines; inactivity exceeding `UpgradeIdleTimeout` terminates both sockets deterministically via `sync.Once` and unblocks relay goroutines ([`TASK-098`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-098.md)).
- **HTTP/2 Extended CONNECT Protection (`pkg/server/server.go`)**: Enforced `UpgradeIdleTimeout` on RFC 8441 extended CONNECT streams, with immediate socket teardown upon client request context cancellation (`r.Context().Done()` / `RST_STREAM`) ([`TASK-099`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-099.md)).
- **Transport-Layer Heartbeat Transparency**: RFC 6455 WebSocket Ping/Pong control frames and application keep-alive messages continuously refresh the deadline, preserving legitimate persistent sessions indefinitely.
- **TCP Half-Close Propagation**: Supported `CloseWrite()` upon reading `io.EOF`, allowing reverse responses to drain while maintaining deadline enforcement.
- **Zero External Dependencies**: Implemented strictly with Go standard library packages (`net`, `sync`, `sync/atomic`, `time`, `io`, `log`, `errors`).
- **Automated Verification Suite (`pkg/server/server_test.go`)**: Tested idle timeout socket teardown, heartbeat keep-alive survival, peer disconnect cleanup, HTTP/2 extended CONNECT, and high concurrency under `go test -race` ([`TASK-100`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-100.md), [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md)).

### Added
- **Configuration Fields**: `UpgradeIdleTimeout time.Duration` in [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go) and [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go).
- **Automated Tests (`pkg/server/server_test.go`, `pkg/config/config_test.go`)**:
  - `TestConfig_UpgradeIdleTimeoutSchemaAndFallback`: Schema defaults, fallback, and validation.
  - `TestServer_UpgradedConn_IdleTimeout`: Automatic socket teardown upon silence.
  - `TestServer_UpgradedConn_HeartbeatKeepsAlive`: Periodic heartbeats/pings sustain connection.
  - `TestServer_UpgradedConn_PeerDisconnect`: Immediate clean teardown on peer close.
  - `TestServer_UpgradedConn_HalfClose`: Client `CloseWrite()` propagation with reverse stream draining.
  - `TestServer_HTTP2_ExtendedCONNECT_IdleAndCancel`: HTTP/2 extended CONNECT idle and cancel teardown.
  - `TestServer_UpgradedConn_ConcurrencyRaceSafety`: 40 concurrent streams tested under `-race`.

### Changed
- **HTTP/1.1 Protocol Switching (`pkg/server/server.go`)**: Replaced unbounded `io.Copy` with `relayUpgradedStreams`.
- **HTTP/2 Extended CONNECT (`pkg/server/server.go`)**: Replaced unbounded `io.Copy` with `relayHTTP2UpgradedStream`.

### Related Tasks & Requirements
- [`TASK-097`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-097.md): Configurable Upgraded Inactivity Deadline Schema
- [`TASK-098`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-098.md): Bidirectional Activity-Refreshed Deadline Relay for HTTP/1.1 Upgrades
- [`TASK-099`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-099.md): Idle Deadline Enforcement for HTTP/2 Extended CONNECT Upgraded Streams
- [`TASK-100`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-100.md): Automated Verification Test Suite for Upgraded Connection Idle Deadlines
- [`REQ-088`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-088.md): Maximum Idle Deadline Enforcement on Upgraded Protocol and WebSocket Connections
- [`ADR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-083.md): Bidirectional Activity-Refreshed Deadlines for Upgraded Protocol Sockets
- [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md): Upgraded Connection Idle Deadline & Resource Reclamation Test Suite
- [`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392): Missing Maximum Idle Deadlines on Upgraded Protocol Connections
- [`SR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-087.md): Security Review of SEC-27 Remediation
- [`CR-084`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-084.md): Code Review of Upgraded Protocol Connection Idle Deadline Enforcement

## 2026-09-09 - Toron v1.5.7 Security Release (SEC-26: Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies)

### Milestone Summary
- **Remediation of Critical Vulnerability SEC-26 (`pkg/proxy`, `pkg/config`, `cmd/toron`)**: Successfully resolved critical vulnerability [`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383) ([CWE-400: Uncontrolled Resource Consumption](https://cwe.mitre.org/data/definitions/400.html), [`SR-081 Finding 4`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L130-L140), [`SR-086`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-086.md)) in Layer 4 transport proxies ([`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L64) and [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L92)), eliminating memory exhaustion (OOM), host ephemeral port starvation (`bind: address already in use`), socket file descriptor leaks (`EMFILE`), and Slowloris denial of service.
- **Declarative Configuration Schema (`pkg/config`)**: Extended [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) with `MaxConnections` (`max_connections`, default `10000`), `IdleTimeout` (`idle_timeout`, default `60s`), and `MaxWorkers` (`max_workers`, default `1024`), backed by safe fallback getter methods ([`GetMaxConnections`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L461), [`GetIdleTimeout`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L469), [`GetMaxWorkers`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L477)) ([`TASK-093`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-093.md), [`REQ-087`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-087.md), [`ADR-082`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-082.md)).
- **TCP Concurrency Limits & Fast-Fail Rejection (`pkg/proxy/tcp.go`)**: Enforced atomic active connection tracking. Incoming connections exceeding `MaxConnections` are closed immediately upon `Accept()` without dialing upstream backends, allocating heap memory, or spawning relay goroutines ([`TASK-094`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-094.md)).
- **Bidirectional TCP Idle Deadlines & Slowloris Protection (`pkg/proxy/tcp.go`)**: Replaced unbounded blocking `io.Copy` stream relays with deadline-aware transfer loops. Read and write deadlines are updated on active data transfer; connections with zero throughput for `IdleTimeout` are terminated immediately, freeing socket file descriptors and unblocking worker goroutines. Cleanly supports TCP half-close (`CloseWrite`).
- **UDP Bounded Worker Pool & Saturated Queue Dropping (`pkg/proxy/udp.go`)**: Eliminated unconstrained per-packet goroutine spawning (`go p.handleDatagram(...)`) by implementing a fixed-capacity task channel (`packetQueue`) of size `MaxWorkers` serviced by long-lived worker goroutines. Saturated queues drop excess datagrams fail-safe without memory growth or panics ([`TASK-095`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-095.md)).
- **UDP Client Session Registry & Upstream Socket Reuse (`pkg/proxy/udp.go`)**: Implemented a thread-safe session registry (`sessions map[netip.AddrPort]*udpSession`). Subsequent datagrams from the same client reuse the open outbound `*net.UDPConn`, completely eliminating per-packet socket dials and ephemeral port exhaustion. A background sweeper routine evicts idle sessions after `IdleTimeout` of inactivity.
- **Zero-Allocation Buffer Recycling with `sync.Pool` (`pkg/proxy/udp.go`)**: Datagram buffers (64 KB / 65,535 bytes) are recycled across inbound reads and upstream responses, eliminating per-packet heap allocations and garbage collection pauses.
- **Deterministic Graceful Teardown (`pkg/proxy`)**: Calling `Close()` on either proxy immediately closes listeners, active client sockets, upstream connections, and worker pools within $\le 500\text{ms}$.
- **Zero External Dependencies**: Pure Go standard library implementation (`net`, `net/netip`, `sync`, `sync/atomic`, `time`, `io`).
- **Microbenchmark Verification (`pkg/proxy/tcp_test.go`, `pkg/proxy/udp_test.go`)**: Confirmed ~27,000 ops/sec (0 allocs/op steady-state) for TCP streaming and ~21,700 pkts/sec (0 buffer allocs) for UDP datagram forwarding ([`TASK-096`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-096.md), [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md)).

### Added
- **Configuration Fields**: Added `MaxConnections int`, `IdleTimeout time.Duration`, and `MaxWorkers int` to [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go).
- **Helper Getters**: [`GetMaxConnections()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L461) (defaults to 10,000), [`GetIdleTimeout()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L469) (defaults to 60s), and [`GetMaxWorkers()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L477) (defaults to 1,024).
- **Functional Options**: Introduced `WithTCPMaxConnections`, `WithTCPIdleTimeout`, `WithUDPMaxWorkers`, and `WithUDPIdleTimeout` for flexible proxy configuration.
- **Automated Verification Test Suite (`pkg/proxy/tcp_test.go`, `pkg/proxy/udp_test.go`)**:
  - `TestTCPProxy_MaxConnections`: Connection capacity gating and fast rejection.
  - `TestTCPProxy_IdleTimeout`: Stagnant stream teardown and deadline refreshing.
  - `TestUDPProxy_WorkerPoolSaturation`: Saturated queue fail-safe drops and bounded goroutines.
  - `TestUDPProxy_SocketReuse`: Outbound socket reuse across multiple client datagrams.
  - `TestUDPProxy_SessionIdleTimeout`: Inactive session sweeper eviction and socket release.
  - `TestUDPProxy_BufferPooling`: `sync.Pool` recycling validation ($\le 2$ allocs/op).
  - `TestTCPProxy_GracefulShutdown` & `TestUDPProxy_GracefulShutdown`: Graceful teardown within $\le 500\text{ms}$.
  - `TestTCPProxy_ConcurrencyRaceSafety` & `TestUDPProxy_ConcurrencyRaceSafety`: High-concurrency race safety under `go test -race`.
  - `BenchmarkTCPProxy_Forwarding` & `BenchmarkUDPProxy_Forwarding`: Microbenchmarks.

### Changed
- **Main Gateway Startup (`cmd/toron/main.go`)**: Layer 4 proxy initialization propagates route parameters into `NewTCPProxy` and `NewUDPProxy` with structured diagnostic logging.
- **TCP Stream Forwarding (`pkg/proxy/tcp.go`)**: Replaced unbounded `io.Copy` with bidirectional deadline loops enforcing `idle_timeout`.
- **UDP Datagram Forwarding (`pkg/proxy/udp.go`)**: Replaced per-packet goroutines with bounded worker pools, session caching, and buffer recycling.

### Related Tasks & Requirements
- [`TASK-093`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-093.md): Layer 4 Proxy Configuration Schema & Wiring
- [`TASK-094`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-094.md): Bounded Concurrency, Bidirectional Idle Deadlines & Connection Tracking in TCPProxy
- [`TASK-095`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-095.md): Bounded Worker Pool, sync.Pool Buffer Recycling & Session Socket Reuse in UDPProxy
- [`TASK-096`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-096.md): Automated Verification Suite for Layer 4 TCP & UDP Proxy Hardening
- [`REQ-087`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-087.md): Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies
- [`ADR-082`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-082.md): Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies
- [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md): Layer 4 Concurrency, Socket Reuse, and Idle Deadline Test Suite
- [`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383): Uncontrolled Resource Consumption in Layer 4 TCP and UDP Proxies
- [`SR-086`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-086.md): Security Review of SEC-26 Remediation
- [`CR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-083.md): Code Review of Layer 4 TCP and UDP Proxy Hardening

## 2026-09-08 - Toron v1.5.6 Security Release (SEC-25: Configurable Request Body Limits & HTTP 413 Rejection in Service Mesh Sidecar)

### Milestone Summary
- **Configurable Request Body Limit in Sidecar Proxy (`pkg/config`, `pkg/sidecar`)**: Introduced configurable maximum request body size parameter [`SidecarConfig.MaxBodyBytes`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L109) (and `max_body_bytes` in YAML/JSON) into [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103), eliminating hardcoded buffer limits and providing a reliable default fallback of 10 MB (`10,485,760` bytes) when unset or non-positive ([`TASK-090`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-090.md), [`REQ-086`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-086.md), [`ADR-081`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-081.md), [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374)).
- **Elimination of Silent Body Truncation & Upstream Data Corruption (`pkg/sidecar`)**: Fixed critical vulnerability [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374) (CWE-436, CWE-400) in [`ProxyEngine.proxyToURL`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L192) where payloads exceeding the limit were silently truncated and forwarded upstream as corrupt data. Oversized requests are now immediately rejected with HTTP `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`), request bodies are closed, and proxying is terminated with zero bytes transmitted upstream ([`TASK-091`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-091.md)).
- **Dual-Stage Overflow Protection**:
  - **Fast-Path Declared `Content-Length` Guard**: If an incoming request declares a `Content-Length` greater than `MaxBodyBytes`, the sidecar closes the body and immediately returns HTTP 413 without reading the payload, avoiding memory allocation overhead.
  - **Streaming Bounded Over-Read Guard**: For chunked transfer encodings or streams with undeclared lengths, ingestion is bounded using `io.LimitReader(r.Body, MaxBodyBytes+1)`. If the payload exceeds the limit, the stream is aborted, read bytes discarded, and HTTP 413 returned.
- **Direct Bypass for Non-Mutating Requests**: Safe requests (`GET`, `HEAD`) or empty requests (`ContentLength == 0` or `r.Body == nil`) bypass body reading and are dispatched directly to upstream handlers without buffer allocation.
- **Byte-Fidelity Payload Forwarding**: In-bounds requests within `MaxBodyBytes` are forwarded to the target application with exact byte fidelity and accurate content lengths.
- **Automated Verification Suite (`pkg/sidecar/sidecar_test.go`)**: Implemented unit, integration, and high-concurrency race test cases ([`TASK-092`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-092.md), [`TC-086`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-086.md)) validating default fallback, fast-fail 413 responses, chunked stream rejection, in-bounds byte preservation, and concurrent race safety under `go test -race ./pkg/sidecar/...`.

### Added
- **`MaxBodyBytes int64` Field**: Added to [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103) in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) with tags `yaml:"max_body_bytes" json:"max_body_bytes"`.
- **Default Fallback Normalization**: Added 10 MB fallback in [`DefaultAppConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L466), [`validateConfigDefaults`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L153), and [`NewProxyEngine`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L37).
- **Automated Test Cases (`pkg/sidecar/sidecar_test.go`)**:
  - `TestTC086_01_DefaultLimitAndFallback`: Validates 10 MB fallback for zero/negative values.
  - `TestTC086_02_DeclaredContentLength413Rejection`: Validates fast-path 413 rejection on declared `Content-Length` overflow.
  - `TestTC086_03_ChunkedStreamOverRead413Rejection`: Validates bounded `io.LimitReader` 413 rejection on chunked/streamed overflow.
  - `TestTC086_04_ByteFidelityInBoundsForwarding`: Validates exact byte preservation for valid requests under the limit.
  - `TestTC086_05_NonMutatingAndEmptyPayloadBypass`: Validates direct bypass for GET/HEAD and empty requests.
  - `TestTC086_06_HighConcurrencyRaceSafety`: Validates concurrent execution across parallel goroutines with zero data races.

### Changed
- **Proxy Body Handler (`pkg/sidecar/proxy.go`)**: Replaced silent truncation logic in [`ProxyEngine.proxyToURL`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L192) with dual-stage fail-fast HTTP 413 rejection and diagnostic logging.

### Related Tasks & Requirements
- [`TASK-090`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-090.md): Configurable Sidecar Body Size Limit in SidecarConfig
- [`TASK-091`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-091.md): Immediate HTTP 413 Payload Too Large Rejection in Sidecar ProxyEngine
- [`TASK-092`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-092.md): Automated Verification Suite for Sidecar Body Limiting & 413 Rejection
- [`REQ-086`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-086.md): Configurable Request Body Limit and Truncation Rejection in Service Mesh Sidecar Proxy
- [`ADR-081`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-081.md): Configurable Request Body Limiting and Truncation Rejection in Service Mesh Sidecar Proxy
- [`TC-086`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-086.md): Sidecar Request Body Size Limit and HTTP 413 Rejection Verification
- [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374): Silent Request Body Truncation in Service Mesh Sidecar Proxy

## 2026-09-05 - Toron v1.5.5 Feature Release (Universal Query Parameter Forwarding & Preservation)

### Milestone Summary
- **Universal Query Parameter Forwarding (`pkg/proxy`, `pkg/server`, `pkg/httpparser`)**: Resolved an issue where query parameters were dropped when proxying HTTP/2 and HTTP/3 requests due to unpopulated `QueryParams` and `URL.RawQuery` in the ingress adapter handler (`TASK-060`, `REQ-060`, `ADR-055`).
- **Verbatim Client Query Preservation**: Reverse proxy now forwards raw query parameters (`req.URL.RawQuery`) verbatim, preserving exact parameter order, percent-encoding, and non-value flags without sorting mutations.
- **Target Query Merging**: Pre-configured query parameters in upstream `target` definitions are cleanly merged with incoming client query parameters.
- **Standardized Ingress Request Factory**: Added `httpparser.NewRequestFromStd` and lazy query accessor `req.Query()` to ensure uniform request representations across all protocols.
- **Access Log Visibility**: `AccessLoggerMiddleware` now records full `RequestURI` in `/var/log/toron/access.log`, providing complete visibility of request query strings.

### Added
- **`httpparser.NewRequestFromStd` & `req.Query()`**: Centralized adapter converting standard library `*http.Request` to `*httpparser.Request`.
- **Unit & Integration Tests (`pkg/proxy/proxy_test.go`, `pkg/httpparser/parser_test.go`)**: Tests covering HTTP/1.1 and HTTP/2 query forwarding, verbatim query string preservation, and target query merging.

### Related Tasks
- `TASK-060`: Implement Query Parameter Forwarding and Preservation Across HTTP/1.1, HTTP/2, and HTTP/3

## 2026-09-05 - Toron v1.5.4 Feature Release (Upstream Reverse Proxy Path Rewriting and Target Subpath Preservation)

### Milestone Summary
- **Deterministic Proxy Path Rewriting (`pkg/proxy`)**: Enhanced reverse proxy path joining logic (`JoinProxyPath`) to accurately preserve upstream target URLs containing subpaths (e.g. `http://127.0.0.1:8082/postback`), eliminating unwanted trailing slash injection when `strip_prefix: true` is enabled (`TASK-059`, `REQ-059`, `ADR-054`).
- **Nginx Parity for Reverse Proxying**: Brings Toron into full parity with standard reverse proxies (Nginx, Envoy, Caddy), allowing upstream microservices (such as Go standard library `http.ServeMux` or strict REST routers) to receive the exact requested path without trailing slash discrepancies.
- **Explicit Trailing Slash & Nested Path Preservation**: Client-provided trailing slashes (`/kite/postback/`) and nested subpaths (`/kite/postback/status`) continue to be accurately preserved and joined.

### Added
- **Exported `JoinProxyPath` Function (`pkg/proxy/proxy.go`)**: Dedicated function for resolving upstream request paths based on target path, request path, prefix, and strip prefix configuration.
- **Unit & Integration Test Suite (`pkg/proxy/proxy_test.go`)**: Table-driven tests (`TestJoinProxyPath_TableDriven`) and full HTTP reverse proxy integration tests (`TestReverseProxy_SubpathTarget_Integration`).

### Related Tasks
- `TASK-059`: Implement Upstream Reverse Proxy Path Rewriting and Subpath Target Preservation

## 2026-09-04 - Toron v1.5.3 Feature Release (Config-Based Multi-Stream Logging & Daily System Logrotate Support)

### Milestone Summary
- **Config-Based Multi-Stream Logging Engine (`pkg/logging`)**: Implemented a thread-safe, decoupled logging engine with dedicated streams for internal server logs (`server_log`), HTTP request/response transactional access logs (`access_log`), and security audit telemetry (`security_log`), with configurable output formats (`text` Combined format and structured `json`) (`TASK-058`, `REQ-058`, `ADR-053`).
- **Declarative Global Defaults & Per-Route Overrides (`pkg/config`, `pkg/router`)**: Default file paths are configured globally in `config.yaml`, with support for overriding `access_log` and `security_log` on a per-route basis in `routes.yaml` (including silencing via `"off"` or `"none"`).
- **Daily System Logrotate Integration (`etc/logrotate.d/toron`)**: All log files are opened in append mode (`O_APPEND`), guaranteeing zero corruption under `copytruncate`. In addition, Toron establishes an OS signal handler listening for `SIGHUP` to atomically close and reopen all active file descriptors for standard rename/rotate workflows without dropping connections or restarting the daemon.
- **Enterprise WAF & Security Audit Synchronization**: WAF audit logging, authentication failures, and rate limit blocks seamlessly route into the configured security log stream or route-specific security log files.

### Added
- **Multi-Stream Log Manager (`pkg/logging/manager.go`)**: Dedicated package managing thread-safe `LogSink` handles, automatic directory creation (`os.MkdirAll`), text/JSON formatting, and atomic `Reopen()` upon rotation signals.
- **Configuration Schema Extensions (`pkg/config/config.go`)**: Added `ServerLog`, `AccessLog`, and `SecurityLog` to `LoggingConfig`, and added `AccessLog` and `SecurityLog` to `ProxyRouteConfig` with helper query methods.
- **Access Logging Middleware (`pkg/router/router.go`)**: Added `AccessLoggerMiddleware` and `MatchPrefixRoute` to automatically direct access logs according to matched route configuration.
- **System Logrotate Template (`etc/logrotate.d/toron`)**: Standard UNIX logrotate configuration rotating daily, preserving 7 generations, compressing rotated logs, and executing `pkill -HUP -f "toron"`.
- **Unit & Race Test Suite (`pkg/logging/manager_test.go`, `pkg/config/config_test.go`, `pkg/router/router_test.go`)**:
  - `TestLogSink_FileCreationAndDirectory`: Verifies sink opening, nested folder auto-creation, and append mode.
  - `TestLogSink_ReopenLogrotate`: Verifies log rotation simulation (rename -> reopen -> verify lines in old vs new files).
  - `TestLogManager_MultiStreamAndRouteOverrides`: Verifies server log, access log, security log, route overrides, and silencing.
  - `TestLogManager_JSONFormat`: Verifies structured JSON telemetry serialization.
  - `TestLogManager_ConcurrentWrites`: Verifies 50 concurrent worker routines writing 5,000 log records under the race detector.
  - `TestExtractClientIP`: Verifies socket `RawConn.RemoteAddr` extraction, `X-Forwarded-For`, and `X-Real-IP`.
  - `TestRouter_AccessLoggerMiddleware_RouteOverrides`: Verifies HTTP access logging middleware with default, custom route, and silenced routes.
  - `TestConfig_LoggingConfigAndRouteOverrides`: Verifies configuration parsing for global logging and route overrides.

### Changed
- **Server Logging (`cmd/toron/main.go`)**: Standard Go logger (`log.SetOutput`) redirected to `server_log` sink when configured.
- **Signal Handling (`cmd/toron/main.go`)**: Added `syscall.SIGHUP` listener for live log reopening without socket drops.

### Related Tasks
- `TASK-058`: Implement Config-Based Multi-Stream Logging, Route Overrides, and Logrotate Support

## 2026-09-04 - Toron v1.5.2 Feature Release (Single Page Application HTML5 History Fallback Support)

### Milestone Summary
- **Single Page Application (SPA) HTML5 History Fallback (`pkg/router`, `pkg/config`)**: Added native client-side routing fallback support for static routes (`spa: true`, `fallback: "<file>"`), achieving parity with Nginx's `try_files $uri $uri/ /index.html;` directive without requiring external reverse proxies or application runtimes (`TASK-056`, `REQ-056`, `ADR-051`).
- **Asset Masking Protection**: Implemented strict extension-based non-masking logic (`filepath.Ext(relPath) != ""`). Missing physical static assets (e.g., `.js`, `.css`, `.png`, `.svg`, `.json`) return HTTP 404 Not Found rather than the HTML fallback document, preventing cryptic browser runtime syntax errors (such as `Uncaught SyntaxError: Unexpected token '<'`) and CSS MIME-type mismatch rejections.
- **Path Traversal & Boundary Containment Defense**: Hardened fallback path resolution with directory boundary verification (`filepath.Rel` and `filepath.EvalSymlinks`), guaranteeing that fallback documents remain strictly contained within the configured static root directory and rejecting traversal/escape attempts with HTTP 403 Forbidden or HTTP 404 Not Found.
- **Declarative YAML & JSON Configuration**: Extended `ProxyRouteConfig` and `ProxyOptions` with `spa` (boolean) and `fallback` (string) options, supporting default (`index.html`) or custom fallback documents (e.g., `app.html`, `200.html`) and implicit SPA activation whenever `fallback` is configured.

### Added
- **Declarative Route Configuration (`pkg/config`)**: Added `SPA bool` and `Fallback string` attributes to `ProxyRouteConfig` with YAML and JSON unmarshaling support (`pkg/config/config.go`).
- **Proxy Options Propagation (`pkg/proxy`)**: Extended `ProxyOptions` in `pkg/proxy/proxy.go` with `SPA` and `Fallback` fields, forwarded during static route registration in `cmd/toron/main.go`.
- **Static Route SPA Fallback Handler (`pkg/router`)**: Updated `createStaticHandler` in `pkg/router/router.go` to evaluate client navigation paths on cache/filesystem misses (`os.IsNotExist`), transparently serving the configured fallback document with HTTP 200 OK and `Content-Type: text/html; charset=utf-8` while omitting the response body for HTTP `HEAD` requests.
- **Comprehensive Unit & Race Test Suite (`pkg/config/config_test.go`, `pkg/router/router_test.go`)**:
  - `TestConfig_SPARouteConfig`: Validates parsing of `spa` and `fallback` across YAML route configurations.
  - `TestRouter_SPA_PhysicalFileServing`: Verifies direct serving of existing static files and directory indices.
  - `TestRouter_SPA_NavigationFallback`: Verifies 200 OK HTML fallback for client-side navigation paths without extensions (including deep nested paths).
  - `TestRouter_SPA_AssetProtection404`: Verifies strict 404 Not Found returns for missing assets with extensions.
  - `TestRouter_SPA_CustomFallback`: Verifies custom fallback file resolution and implicit SPA activation.
  - `TestRouter_Static_NonSPABackwardCompatibility`: Confirms static routes without SPA settings maintain standard 404 behavior.
  - `TestRouter_SPA_SecurityPathTraversal`: Validates rejection of directory traversal and boundary escape attempts.

### Changed
- **Static Route Handler (`pkg/router/router.go`)**: Modified missing file resolution pipeline to distinguish between client-side virtual navigation routes and physical asset requests when SPA mode is active.

### Related Tasks
- `TASK-056`: Implement SPA HTML5 History Fallback Support for Static Routes

## 2026-09-04 - Toron v1.5.1 Maintenance & Feature Release (HTTP/3 QUIC Listener Engine Integration)

### Milestone Summary
- **HTTP/3 QUIC Listener Engine Integration (`pkg/server`)**: Integrated `github.com/quic-go/quic-go/http3` server engine into Toron's core HTTP server (`ListenAndServeH3`, `H3Server`, `SetH3Server`), allowing native HTTP/3 transport over UDP with full router middleware reuse (`TASK-027`, `REQ-027`, `ADR-022`).
- **Concurrent UDP Startup in `cmd/toron` (`cmd/toron/main.go`)**: Main application entrypoint now spawns a concurrent background goroutine running `srv.ListenAndServeH3()` bound to the configured UDP socket when TLS and HTTP/3 are enabled, running simultaneously alongside the primary HTTPS/TLS TCP listener without blocking server orchestration.
- **Alt-Svc Protocol Upgrade Advertisement on HTTP/2 (`pkg/server`)**: Extended `http2AdapterHandler` to automatically inject `Alt-Svc: h3=":port"; ma=2592000` response headers across HTTP/2 (and HTTP/1.1) connections, notifying compliant clients and browsers to upgrade subsequent requests to HTTP/3 QUIC while preserving pre-existing `Alt-Svc` headers.
- **Graceful QUIC Socket Shutdown (`pkg/server`)**: Enhanced `Server.Shutdown(ctx)` with `sync.RWMutex`-guarded `s.h3Server.Close()` invocation to ensure all UDP QUIC listeners and active streams are cleanly drained and closed, mapping `http.ErrServerClosed` to `ErrServerClosed` and suppressing false-positive shutdown errors in `cmd/toron/main.go`.

### Added
- **Thread-Safe HTTP/3 Server Pointer Management**: Added `H3Server()` and `SetH3Server(h *http3.Server)` to `pkg/server/server.go` protected by `sync.RWMutex`.
- **HTTP/2 Adapter Alt-Svc Advertising**: Added automatic `Alt-Svc` header injection in `http2AdapterHandler()` in `pkg/server/server.go`.
- **UDP QUIC Listener Orchestration**: Concurrent background listener in `cmd/toron/main.go` handling HTTP/3 over UDP port (default: 8443) when TLS is active.
- **Unit & Regression Test Coverage (`pkg/server/server_test.go`)**:
  - `TestServer_HTTP2Adapter_AltSvcHeader`: Tests `Alt-Svc` header injection with default port 8443, custom port 9443, disabled advertising, and existing header preservation.
  - `TestServer_HTTP3_Shutdown`: Tests active HTTP/3 server shutdown (`Close()`) and nil-safety during graceful teardown.

### Changed
- **Graceful Shutdown**: `Server.Shutdown(ctx)` coordinates both TCP reactor shutdown and UDP QUIC socket termination (`s.h3Server.Close()`).

### Related Tasks
- `TASK-027`: Implement HTTP/3 Protocol Engine & QUIC Listener Support

## 2026-08-18 - Toron v1.5.0 Feature Release (Prototypes 40 & 41: Advanced Load Balancing & Universal Auto-Installer)

### Milestone Summary
- **Advanced Load Balancing Engine (`pkg/proxy`)**: Implemented high-throughput pluggable load balancing suite supporting 8 strategies: `round_robin`, `weighted_round_robin`, `random`, `weighted_random`, `least_conn`, `weighted_least_conn`, `least_latency` (EMA response-time tracking), `sticky_cookie`, and `ip_hash` (`TASK-050`, `REQ-050`, `ADR-045`).
- **Universal Auto-Installer & Service Manager (`install.sh` / `install.bat`)**: Automated zero-dependency deployment script supporting Linux (`systemd`), macOS (`launchd`), and Windows (`sc.exe`) on both ARM64 and AMD64 architectures with automatic daemon loading, daily log rotation, and clean uninstallation (`TASK-051`, `REQ-051`, `ADR-046`).
- **Prefix Management & 3xx Redirect/Cookie Rewriting (`pkg/proxy`)**: Automated inbound `X-Forwarded-Prefix` injection, 3xx `Location` redirect rewriting, and `Set-Cookie: Path=` scoping for prefix-routed legacy and modern microservices (`TASK-052`, `REQ-052`, `ADR-047`).
- **Dynamic Upstream Health Matrix & Telemetry Dashboard (`pkg/server`, `public/`)**: Removed all hardcoded mock ports and dummy services from the dashboard, replacing them with dynamic runtime route aggregation, concurrent target health probing, and dynamic API tester endpoints (`TASK-053`, `REQ-053`, `ADR-048`).
- **Dashboard v2.0 Redesign (`public/`)**: Redesigned modern minimalist, mobile-first control center with 3-way light/dark/system theme synchronization, instant route search, and upgraded interactive API console (`TASK-054`, `REQ-054`, `ADR-049`).
- **Centralized Version Management (`pkg/version`, `VERSION`)**: Unified single-source-of-truth semantic versioning across Go runtime, linker metadata injection (`-ldflags`), Makefile, and OS auto-installers with `-v`/`-version` CLI support (`TASK-055`, `REQ-055`, `ADR-050`).
- **Standardized Configuration Standard**: Enforced `/etc/toron/` (`config.yaml`, `routes.yaml`, `public/`) system configuration and `/var/log/toron/` centralized logging paths across system daemons.

### Related Tasks
- `TASK-050`: Implementation Breakdown for Advanced Load Balancing Algorithms
- `TASK-051`: Implement Universal Auto-Installer & Service Manager (`install.sh`)
- `TASK-052`: Implement Reverse Proxy Prefix Management & Redirect/Cookie Rewriting
- `TASK-053`: Implement Dynamic Upstream Discovery & Data-Driven Dashboard
- `TASK-054`: Implement Dashboard v2.0 UI Redesign & Multi-Theme Engine
- `TASK-055`: Implement Centralized Version Tracking & Build Metadata Injection

## 2026-08-16 - Toron v1.4.0 Feature Release (Prototype 39: REST-to-gRPC Transcoding Engine)

### Milestone Summary
- **REST-to-gRPC Transcoding Engine (`pkg/transcoder`)**: Implemented direct JSON HTTP REST (`GET /v1/users/:id`) to binary Protobuf HTTP/2 gRPC (`POST /user.UserService/GetUser`) request and response translation (`TASK-049`, `REQ-049`).
- **Zero External Dependencies**: Pure Go stdlib implementation using `encoding/binary`, `encoding/json`, `net/http`, and `toron/pkg/httpparser` without protobuf compiler tools.
- **gRPC 5-Byte Wire Framing**: Automated encoding and decoding of gRPC wire frames (`[0x00][4-byte length] + payload`).
- **gRPC Status Mapping**: Automatic conversion of gRPC trailer status codes (`grpc-status: 0` -> 200 OK, `grpc-status: 5` -> 404 Not Found, `grpc-status: 16` -> 401 Unauthorized) to standard HTTP status codes.

### Related Tasks
- `TASK-049`: Implement REST-to-gRPC Transcoding Engine

## 2026-08-16 - Toron v1.3.0 Feature Release (Prototype 38: Service Mesh Sidecar Mode)

### Milestone Summary
- **Service Mesh Sidecar Mode (`pkg/sidecar`)**: Implemented lightweight pod-level proxy mode enforcing pod-to-pod Mutual TLS (mTLS) encryption and dynamic weighted traffic splitting (`TASK-048`, `REQ-048`).
- **Zero External Dependencies**: Pure Go stdlib HTTP & TLS client/server listeners operating on dedicated local ports (`15006` ingress, `15001` egress).
- **Strict Mutual TLS**: Support for `RequireAndVerifyClientCert` with Root CA validation pools (`ca_file`).
- **Weighted Traffic Splitting**: Thread-safe atomic weighted round-robin selector (`WeightedSplitter`) for canary traffic distribution (e.g. 80/20 ratio).

### Related Tasks
- `TASK-048`: Implement Service Mesh Sidecar Mode Engine

## 2026-08-16 - Toron v1.2.0 Feature Release (Prototype 37: Native Kubernetes Ingress Controller)

### Milestone Summary
- **Native Zero-Dependency Kubernetes Ingress Controller (`pkg/ingress`)**: Implemented Kubernetes `networking.k8s.io/v1` Ingress Controller translating `Ingress`, `Service`, `Endpoints`, and TLS `Secret` resources into Toron's core routing matrix (`TASK-047`, `REQ-047`).
- **Zero External Dependencies**: Pure Go stdlib HTTP & TLS client communicating with Kubernetes API server without importing `k8s.io/client-go`.
- **In-Cluster Auto-Authentication**: Automated ServiceAccount bearer token and Root CA certificate loading from `/var/run/secrets/kubernetes.io/serviceaccount/`.
- **Real-Time Endpoint Watching**: Streaming watch worker (`watch=true`) dynamically updates load balancing targets as pod IP endpoints scale or shift.

### Related Tasks
- `TASK-047`: Implement Native Kubernetes Ingress Controller Engine

## 2026-08-16 - Toron v1.1.0 Feature Release (Prototype 36: Vendor-Agnostic OCI Container Auto-Discovery Engine)

### Milestone Summary
- **Vendor-Agnostic OCI Container Auto-Discovery Engine (`pkg/discovery`)**: Implemented dynamic container discovery engine monitoring Unix domain sockets across Docker Engine, Podman, Finch, and Nerdctl (`TASK-046`, `REQ-046`).
- **Zero External Dependencies**: Pure Go stdlib HTTP transport (`net.DialContext("unix", ...)`) over Unix domain sockets without 3rd-party Docker or Containerd SDKs.
- **Unified `toron.*` Metadata Label Taxonomy**: Automatic extraction of container routing metadata (`toron.enable`, `toron.host`, `toron.prefix`, `toron.port`, `toron.weight`, `toron.health_check`).
- **Real-Time Lifecycle Event Streaming**: Background worker streams container `start`, `die`, and `stop` events and dynamically inserts/removes upstreams from `router.Router` with zero downtime.

### Related Tasks
- `TASK-046`: Implement Vendor-Agnostic OCI Container Auto-Discovery Engine

## 2026-08-16 - Toron v1.0.0 Official Release (Feature Freeze Milestone)

### Milestone Summary
- **Official Version 1.0.0 Freeze**: All feature sets spanning Prototypes 1 through 35 are officially frozen for the stable **v1.0.0** release.
- **Production Scope**: Core event reactor engine, HTTP/1.1, HTTP/2 (`h2c` / TLS), HTTP/3 QUIC, gRPC gateway & trailers, ACME zero-touch SSL (HTTP-01 & ALPN-01), per-host SNI & mTLS, Zstd/Brotli compression, RFC 7234 response caching, multi-scheme authentication, Web Application Firewall (WAF) with OWASP & custom regex rules and CIDR IP ACLs, zero-downtime hot reloading, Prometheus metrics, and read-only Security Control Center Dashboard are finalized.

## 2026-08-15 - Prototype 35 Release (Custom WAF Regex Rules & Zero-Downtime Hot Reloading)

- **Web Control Center Security & WAF Dashboard (`public/`, `pkg/server`)**: Added dedicated Security & WAF Dashboard tab to the Web Control Center (`/internal/dashboard/`) featuring hero threat indicators, visual Security & Compliance Policy Matrix, real-time Security Audit Incidents log feed table, and WAF attack presets in the Live API Tester.
- **Security Incidents Feed API (`GET /internal/api/security/incidents`)**: Registered management endpoint serving recorded security events from an in-memory thread-safe ring buffer in `AuditLogger`.
- **User-Defined Custom WAF Regex Rules (`pkg/waf`)**: Supported defining custom security regex rules in `config.yaml` (`server.waf.custom_rules`) and `routes.yaml` (`pr.waf.custom_rules`) with customizable `id`, `category`, `description`, `pattern`, `score`, and granular inspection locations (`url`, `path`, `query`, `headers`, `body`) (`TASK-045`, `REQ-045`).
- **Zero-Downtime Hot Reloading via `ConfigWatcher` (`pkg/config/watcher.go`)**: Background `fsnotify` file worker monitors `config.yaml` with debouncing, automatically re-validates configuration syntax, and atomically reloads active WAF rule sets in memory via `WAFEngine.Reload()` without dropping active TCP/TLS/HTTP connections.
- **Fail-Safe Hot Reload Protection**: Invalid configuration edits or unparseable regular expressions are safely rejected with descriptive warning logs while retaining the active in-memory rule set intact.

### Related Tasks
- `TASK-045`: Implement Custom WAF Regex Rules and Zero-Downtime Hot Reloading

## 2026-08-15 - Prototype 34 Release (WAF Telemetry, Prometheus Metrics & Structured Security Audit Logging)

### Added
- **Prometheus WAF Metrics (`pkg/metrics`)**: Added specialized WAF telemetry counters and latency histograms exposed via `/metrics` in standard Prometheus text format (`TASK-044`, `REQ-044`):
  - `toron_waf_blocked_requests_total{category="...",route="..."}`: Total blocked attacks categorized by threat vector and route.
  - `toron_waf_anomalies_detected_total{category="...",mode="detection"}`: Total anomalies observed in detection mode.
  - `toron_waf_inspection_duration_seconds`: High-resolution latency histogram tracking WAF inspection processing times.
  - Telemetry integrated into JSON metrics summary API (`/internal/api/metrics`).
- **Structured JSON Security Audit Logger (`pkg/waf/audit.go`)**: Implemented thread-safe `AuditLogger` writing SIEM-ready JSON log lines (client IP, rule ID, threat score, method, path, location, and bounded payload snippet) to `stdout`, `stderr`, or dedicated log files.
- **WAF Middleware Telemetry Hooks (`pkg/waf/middleware.go`)**: Automatically records inspection duration, increments Prometheus counters, and emits structured security audit logs for IP ACL blocks, protocol violations, OWASP threat blocks, and detection anomalies.

### Related Tasks
- `TASK-044`: Implement WAF Telemetry, Prometheus Metrics, and Structured Security Audit Logging

## 2026-08-15 - Prototype 33 Release (Route-Level WAF Overrides & CIDR IP Access Control Lists)

### Added
- **CIDR-Based IP Access Control Lists (ACLs)**: Implemented fast-path $O(1)$ IP allow/deny list filtering in `pkg/waf/ip_acl.go` supporting IPv4/IPv6 CIDR subnets (`allowed_ips`, `denied_ips`) and single IP addresses with automatic port stripping and proxy header (`X-Forwarded-For`, `X-Real-IP`) parsing (`TASK-043`, `REQ-043`).
- **Per-Route WAF Configuration & Overrides**: Extended `routes.yaml` and `pkg/router/router.go` (`RoutePrefix`) to allow routes to independently configure dedicated `WAFEngine` instances with custom `mode`, `anomaly_threshold`, `disabled_rules`, and IP access lists.
- **Selective OWASP Rule Tuning**: Enabled disabling individual threat rules (e.g. `SQLI-001`) on legacy backends without weakening gateway-wide security.

### Related Tasks
- `TASK-043`: Implement Route-Level WAF Overrides and CIDR IP Access Lists

## 2026-08-15 - Prototype 32 Release (Protocol Integrity & HTTP Request Smuggling Guard)

### Added
- **HTTP Request Smuggling Prevention (CL.TE / TE.CL)**: Implemented dual-layer detection in `pkg/httpparser/parser.go` and `pkg/waf/protocol.go` rejecting requests with conflicting `Content-Length` and `Transfer-Encoding` headers or mismatched multiple `Content-Length` header values (`TASK-042`, `REQ-042`).
- **Malformed Request Control Character Guard**: Added non-printable control character filtering (`0x00–0x1F` except `\t`, `0x7F DEL`) in URL paths, query strings, and header keys/values returning `400 Bad Request`.
- **Payload Size Bounding**: Added strict boundary limits on single header values (`max_header_value_bytes`, 4 KB), query strings (`max_query_size`, 4 KB), and parameters (`max_param_size`, 2 KB) returning `413 Payload Too Large`.

### Related Tasks
- `TASK-042`: Implement Protocol Integrity and Request Smuggling Guard

## 2026-08-15 - Prototype 31 Release (Core Web Application Firewall Engine & OWASP Injection Protection)

### Added
- **Core WAF Engine (`pkg/waf`)**: Implemented high-performance, modular Web Application Firewall engine (`WAFEngine`) in `pkg/waf/waf.go` with `enforce` (HTTP `403 Forbidden` blocking) and `detection` (log-only threat anomaly score) modes (`TASK-041`, `REQ-041`).
- **OWASP Top 10 Injection Protection**: Pre-compiled regex rule set in `pkg/waf/rules.go` targeting SQL Injection (`SQLI-001`, `SQLI-002`, `SQLI-003`), Cross-Site Scripting (`XSS-001`, `XSS-002`, `XSS-003`), Path Traversal / LFI (`TRAVERSAL-001`, `TRAVERSAL-002`), and Command Injection / RCE (`RCE-001`, `RCE-002`).
- **Multi-Location Request Inspection**: Inspects URL paths, raw query parameters, HTTP request headers, and payload bodies (bounded by `max_inspect_body_size`, restoring `req.Body` for downstream handlers).
- **Router Middleware Adapter**: Created `NewWAFMiddleware` in `pkg/waf/middleware.go` and integrated into `cmd/toron/main.go`, `pkg/config/config.go`, `config.yaml`, and `routes.yaml`.

### Related Tasks
- `TASK-041`: Implement Core Web Application Firewall Engine and OWASP Injection Protection Middleware

## 2026-08-14 - Prototype 30 Release (Configurable CORS Policies & Enterprise Security Headers)

### Added
- **CORS Middleware**: Implemented `CORSMiddleware` in `pkg/router/cors.go` with fast-path `OPTIONS` preflight `204 No Content` handling, origin wildcard/subdomain matching, credential policies, and exposed headers (`TASK-040`, `REQ-040`).
- **Enterprise Security Headers Middleware**: Added `SecurityHeadersMiddleware` in `pkg/router/security_headers.go` injecting `Strict-Transport-Security` (HSTS), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `Content-Security-Policy`, and `Permissions-Policy`.
- **Global & Route Configuration**: Supported configuring CORS and Security Headers in `config.yaml` (`server.cors`, `server.security_headers`) and per-route in `routes.yaml`.

### Related Tasks
- `TASK-040`: Implement Configurable CORS Policies and Enterprise Security Headers Middleware

## 2026-08-14 - Prototype 29 Release (Per-Host Dynamic SNI Certificate Mapping & mTLS Client Auth)

### Added
- **Dynamic SNI Multi-Certificate Registry**: Added `SNIRegistry` in `pkg/server/sni.go` with `tls.Config.GetConfigForClient` hook, dynamically mapping hostnames to dedicated X.509 certificate pairs (`TASK-039`, `REQ-039`).
- **Mutual TLS (mTLS) Client Verification**: Added per-host client certificate verification with configurable policies (`require_and_verify`, `verify_client_cert_if_given`, `request_client_cert`) and custom client CA pools (`ca_file`).
- **Per-Host Minimum TLS Version**: Supported enforcing `tls.min_version` (`tls1.2`, `tls1.3`) per route in `routes.yaml`.

### Related Tasks
- `TASK-039`: Implement Per-Host SNI Dynamic Certificate Dispatching and mTLS Client Auth

## 2026-08-14 - Prototype 28 Release (Native gRPC Health Checking Protocol & HTTP/2 Trailers Gateway)

### Added
- **Native `grpc.health.v1.Health` Prober**: Added binary Protobuf frame encoder/decoder in `pkg/proxy/grpc_health.go` supporting active `grpc.health.v1.Health/Check` background health probing over HTTP/2 (`TASK-038`, `REQ-038`).
- **HTTP/2 Trailers Gateway Preservation**: Forwarded upstream trailing headers (`grpc-status`, `grpc-message`, `grpc-status-details-bin`) through the reverse proxy to downstream clients.
- **gRPC Route Configuration**: Added `health_check_type: "grpc"` and `health_check_service: "<name>"` options to `routes.yaml` and `ProxyRouteConfig`.

### Related Tasks
- `TASK-038`: Implement Native gRPC Health Checking Prober and HTTP/2 Trailers Preservation

## 2026-08-14 - Prototype 27 Release (Next-Gen Response Compression: Brotli & Zstandard)

### Added
- **Brotli & Zstandard Compression**: Extended `CompressionMiddleware` in `pkg/router/compression.go` to support Brotli (`br`, RFC 7932) and Zstandard (`zstd`, RFC 8878) (`TASK-037`, `REQ-037`).
- **Encoder Object Pooling**: Implemented `sync.Pool` allocation recycling for `brotli.Writer` and `zstd.Encoder`.
- **Quality Factor Content Negotiation**: Added RFC 7231 quality factor weighting (`q=`) parsing and modern server ranking (`zstd` > `br` > `gzip` > `deflate`).

### Related Tasks
- `TASK-037`: Implement Brotli (`br`) and Zstandard (`zstd`) Response Compression with Encoders Pooling

## 2026-08-14 - Prototype 26 Release (Multi-Scheme Authentication: JWT, API Key, and Basic Auth)

### Added
- **Multi-Scheme Auth Middleware**: Added `AuthMiddleware` in `pkg/router/auth.go` supporting RFC 7519 JWT verification (HS256/HS384/HS512), API key authentication, and RFC 7617 HTTP Basic authentication (`TASK-036`, `REQ-036`).
- **Timing Attack Resistance**: Used `crypto/subtle.ConstantTimeCompare` across all signature and credential comparisons.
- **Route & Global Integration**: Supported configuring authentication per route rule in `routes.yaml` or globally in `config.yaml`, injecting `X-Authenticated-User` headers into upstream requests.

### Related Tasks
- `TASK-036`: Implement Multi-Scheme Authentication Middleware (JWT HS256, API Key, and HTTP Basic Auth)

## 2026-08-14 - Prototype 25 Release (In-Memory HTTP Response Caching & Cache-Control)

### Added
- **In-Memory Response Caching**: Added `ResponseCache` and `NewCacheMiddleware` in `pkg/router/cache.go` providing thread-safe in-memory caching for idempotent GET and HEAD requests (`TASK-035`, `REQ-035`).
- **RFC 7234 Cache-Control Engine**: Parsed `max-age`, `no-store`, `no-cache`, `private`, and `public` directives; supported client refresh bypasses (`Cache-Control: no-cache`).
- **Diagnostics & Age Headers**: Injected `X-Cache: HIT` / `X-Cache: MISS` telemetry indicators and calculated `Age: <seconds>` headers.
- **Memory Bounding & Eviction**: Added memory bounds via `max_entries` and `max_payload_size` in `config.yaml` (`server.cache`).

### Related Tasks
- `TASK-035`: Implement In-Memory Response Caching Engine, Cache-Control Parser, and Diagnostics Headers

## 2026-08-14 - Prototype 24 Release (Streaming Response Compression: Gzip & Deflate)

### Added
- **Transparent Response Compression**: Added `CompressionMiddleware` in `pkg/router/compression.go` using Go standard library `compress/gzip` and `compress/flate` (`TASK-034`, `REQ-034`).
- **sync.Pool Writer Allocation Reuse**: Implemented object pooling for gzip and flate writers to achieve zero-allocation buffer reuse during high-concurrency requests.
- **Config & Protocol Safety**: Added `server.compression` settings to `config.yaml` (`enabled`, `min_length`, `level`, `encodings`, `types`) and safeguarded WebSocket 101 upgrades and binary media types against compression.

### Related Tasks
- `TASK-034`: Implement Transparent HTTP Response Compression Middleware (Gzip & Deflate)

## 2026-08-14 - Prototype 23 Release (ACME Zero-Touch Production SSL & TLS-ALPN-01)

### Added
- **ACME Engine**: Added `ACMEManager` in `pkg/acme/acme.go` for zero-touch SSL certificate issuance and background renewal (`TASK-033`, `REQ-033`).
- **HTTP-01 & TLS-ALPN-01 Responders**: Implemented HTTP-01 token authorization responder (`/.well-known/acme-challenge/*`) and TLS-ALPN-01 responder (`acme-tls/1` ALPN negotiation with OID `1.3.6.1.5.5.7.1.31`).
- **Disk Caching & Key Security**: Implemented secure disk certificate and key caching in `cache_dir` with `0600`/`0700` POSIX permissions.

### Related Tasks
- `TASK-033`: Implement ACME Engine, HTTP-01/TLS-ALPN-01 Responders, and Certificate Caching

## 2026-08-13 - Prototype 22 Release (Telemetry Metrics in Control Plane JSON Endpoints)

### Added
- **JSON Telemetry Integration**: Added `GetSummaryJSON()` in `pkg/metrics/metrics.go` exporting total requests, active QUIC streams, active TCP connections, circuit breaker trips, and status/method breakdowns (`TASK-032`, `REQ-032`).
- **Control Plane API**: Updated `GET /internal/api/status` and registered `GET /internal/api/metrics` returning structured JSON metrics.

### Related Tasks
- `TASK-032`: Expose Comprehensive Metrics in /internal/api/ Control Plane Endpoints

## 2026-08-13 - Prototype 21 Release (Prometheus Metrics & W3C Traceparent Propagation)

### Added
- **Prometheus Metrics Exporter**: Implemented `/metrics` endpoint returning Prometheus exposition format (`text/plain; version=0.0.4`) with request counters, latency histograms, and QUIC stream gauges (`TASK-031`, `REQ-031`).
- **W3C Distributed Tracing**: Added W3C `traceparent` context header extraction, generation, and upstream propagation in `pkg/metrics/tracing.go` and `pkg/proxy/proxy.go`.

### Related Tasks
- `TASK-031`: Implement Prometheus Metrics Registry and W3C Traceparent Header Propagation

## 2026-08-13 - Prototype 20 Release (Sticky Session Load Balancing: sticky_cookie & ip_hash)

### Added
- **Session Affinity Balancers**: Added `StickyCookieBalancer` (cookie-based session affinity with `Set-Cookie` injection) and `IPHashBalancer` (client IP hash affinity) in `pkg/proxy/sticky.go` (`TASK-030`, `REQ-030`).
- **Config & Router Integration**: Supported `algorithm: "sticky_cookie"` and `algorithm: "ip_hash"` in `routes.yaml` and `pkg/proxy/proxy.go`.

### Related Tasks
- `TASK-030`: Implement Sticky Session Load Balancing (sticky_cookie and ip_hash)

## 2026-08-13 - Prototype 19 Release (Token Bucket Rate Limiting Middleware)

### Added
- **Token Bucket Rate Limiter**: Added `TokenBucket`, `RateLimiter`, and `NewRateLimitMiddleware` in `pkg/router/rate_limiter.go` for route-level DDoS protection (`TASK-029`, `REQ-029`).
- **Client Key Extraction**: Extracted client identity via `X-API-Key`, `Authorization`, `X-Forwarded-For`, or remote IP with `429 Too Many Requests` and `Retry-After` header returns.

### Related Tasks
- `TASK-029`: Implement Token Bucket Rate Limiting Middleware per Client IP and API Key

## 2026-08-13 - Prototype 18 Release (Dynamic Route Hot Reloading via fsnotify)

### Added
- **Route Hot Reloading**: Integrated `github.com/fsnotify/fsnotify` in `pkg/config/watcher.go` (`RouteWatcher`) to automatically watch `routes.yaml` edits and update routing tables dynamically without dropping socket connections (`TASK-028`, `REQ-028`).
- **Atomic Router Reset**: Added `Router.Reset()` in `pkg/router/router.go` for zero-downtime routing table reloading under write lock.

### Related Tasks
- `TASK-028`: Implement Hot Reloading of Routes via File-Watch Worker (fsnotify)

## 2026-08-13 - Prototype 17 Release (HTTP/3 Protocol Engine & QUIC Transport)

### Added
- **HTTP/3 QUIC Transport**: Integrated `github.com/quic-go/quic-go/http3` engine into `pkg/server/server.go` (`TASK-027`, `REQ-027`).
- **Alt-Svc Protocol Advertising**: Injected `Alt-Svc: h3=":8443"` headers on HTTP/1.1 and HTTP/2 response headers for automatic browser HTTP/3 upgrades.

### Related Tasks
- `TASK-027`: Implement HTTP/3 Protocol Engine and QUIC Transport Handler

## 2026-08-13 - Prototype 16 Release (Layer 4 TCP & UDP Transport Proxying)

### Added
- **L4 TCP Socket Proxy**: Added `TCPProxy` in `pkg/proxy/tcp.go` for raw socket stream forwarding and round-robin load balancing (`TASK-026`, `REQ-026`).
- **L4 UDP Datagram Proxy**: Added `UDPProxy` in `pkg/proxy/udp.go` for connectionless datagram packet proxying.

### Related Tasks
- `TASK-026`: Implement Layer 4 TCP and UDP Transport Proxying

## 2026-08-12 - Prototype 15 Release (HTTPS TLS Encryption & Dev Certificate Generator)

### Added

- **HTTPS TLS Listener**: Added `ListenAndServeTLS` and `CreateTLSConfig` in `pkg/server/tls.go` and `pkg/server/server.go` (`TASK-023`, `REQ-023`).
- **ALPN HTTP/2 Negotiation**: Integrated TLS ALPN protocol negotiation (`h2`, `http/1.1`) mapping encrypted HTTP/2 streams to the server core.
- **Auto Self-Signed Dev Certs**: Implemented `GenerateDevCert()` providing zero-configuration ECDSA P-256 self-signed certificates for `localhost` development.
- **HTTPS Unit Test Suite**: Added `TestServer_HTTPSSelfSigned` and `TestServer_HTTPSWithALPNHTTP2` in `pkg/server/tls_test.go`.

### Related Tasks

- `TASK-023`: Implement HTTPS TLS Encryption, ALPN Negotiation, and Dev Cert Generator

## 2026-08-12 - Prototype 14 Release (HTTP/2 Protocol Support & Stream Multiplexing)

### Added

- **HTTP/2 Engine Integration**: Integrated `golang.org/x/net/http2` server engine into `pkg/server/server.go` (`TASK-022`, `REQ-022`).
- **Connection Preface Auto-Detection**: Added connection preface detection (`PRI * HTTP/2.0...`) in `handleConn`, enabling zero-downtime hybrid HTTP/1.1 and HTTP/2 cleartext (`h2c`) stream handling.
- **HTTP/2 Configuration**: Added `http2` settings (`enabled`, `max_concurrent_streams`, `max_frame_size`, `allow_h2c`) to `config.yaml` and `pkg/config`.
- **HTTP/2 Integration Tests**: Added `TestServer_HTTP2PriorKnowledge` unit test in `pkg/server/http2_test.go`.

### Related Tasks

- `TASK-022`: Implement HTTP/2 Server Connection Handler and Configuration Options

## 2026-08-12 - Prototype 13 Release (Configuration Syntax Testing & Dry-Run CLI Flag)

### Added

- **Configuration Dry-Run Flag**: Added `-test-config` (`-t`) CLI option in `cmd/toron/main.go` to test YAML syntax and configuration parameters before starting the server (`TASK-021`, `REQ-021`).
- **Strict Validator**: Implemented `ValidateConfig(cfg *AppConfig) error` in `pkg/config/loader.go` checking port ranges, static file directory accessibility, load balancing algorithms, and target URL schemes (`http://` / `https://`).
- **CLI Reference**: Updated CLI reference guide (`docs/wiki/reference/cli.md`).

### Related Tasks

- `TASK-021`: Implement Configuration Test Option (-test-config / -t) and Strict Validator

## 2026-08-12 - Prototype 12 Release (Domain-Based Virtual Host Routing)

### Added

- **Domain-Based HTTP Routing**: Added Virtual Host matching support in `pkg/router/router.go` (`r.GETHost`, `r.HandleHostHeader`, `r.ProxyWithOptions` with host matching) (`TASK-020`, `REQ-020`).
- **YAML Schema Extension**: Supported `host` and `domain` parameters under proxy route configurations in `routes.yaml`.
- **Host Header Extraction & Sanitization**: Implemented `extractHost(req)` stripping optional port numbers and matching exact subdomains.
- **HTTP REST Test Suite**: Added domain-based Host header request examples in `test_endpoint.http`.

### Related Tasks

- `TASK-020`: Implement Domain-Based HTTP Routing and Proxy Forwarding

## 2026-08-12 - Prototype 11 Release (Decoupled Dual-File YAML Configuration)

### Added

- **Dual-File Configuration Architecture**: Split application configuration into `config.yaml` (server infrastructure, static assets, logging) and `routes.yaml` (reverse proxy routing rules, load balancers, health checks) (`TASK-019`, `REQ-019`).
- **Loader Enhancement**: Added `LoadFromFiles(configPath, routesPath)` in `pkg/config` with auto-discovery of `routes.yaml`.
- **CLI Flag Integration**: Added `-routes` (`-r`) CLI flag alongside `-config` (`-c`) in `cmd/toron/main.go`.

### Related Tasks

- `TASK-019`: Implement Dual-File YAML Config Loader (config.yaml & routes.yaml)

## 2026-08-12 - Prototype 10 Release (Configurable Static Prefix & Relative Asset Resolution)

### Added

- **Relative Asset Resolution**: Updated `public/index.html` to reference `./style.css` and `./app.js` using relative URL paths (`TASK-018`, `REQ-018`).
- **Subpath Prefix Trailing Slash Redirect**: Updated `pkg/router/router.go` static file handler to automatically emit a `302 Found` redirect when a request matches a configured static prefix (e.g. `/internal/dashboard`) without trailing slash, establishing correct browser base URL resolution.
- **Config Integration**: Tested and verified static prefix configuration (`prefix: "/internal/dashboard"`) in `config.yaml`.

### Related Tasks

- `TASK-018`: Implement Configurable Static Prefix Trailing Slash Redirect and Relative Asset Loading

## 2026-08-12 - Prototype 9 Release (Internal Management API Endpoints)

### Added

- **Internal Management API Namespace**: Added `/internal/api/` control plane endpoints (`status`, `routes`, `upstreams/health`, `proxy-test`) in `pkg/server/internal_api.go` (`TASK-017`, `REQ-017`).
- **Server-Side Upstream Probing**: Backend routines perform concurrent HTTP health checks against upstreams, returning node health states to the dashboard without requiring browser-to-upstream target calls.
- **Internal Proxy Test Dispatcher**: `POST /internal/api/proxy-test` accepts path/method/header payloads, executes proxy routes server-side, and returns status codes, latency, headers, and body payloads.
- **Frontend Integration**: Updated `public/app.js` to query `/internal/api/` endpoints exclusively.

### Related Tasks

- `TASK-017`: Implement /internal/api/ Management Endpoints and Update Frontend Client

## 2026-08-12 - Prototype 8 Release (Mobile-First Control Center & Dashboard)

### Added

- **Mobile-First Control Center & Proxy Dashboard**: Transformed `/public` frontend into a responsive Web Control Center and Proxy Dashboard built with HTML5, Vanilla JavaScript, and Tailwind CSS (`TASK-016`, `REQ-016`).
- **Real-Time Upstream Health Probing**: Dynamic client-side health check execution querying upstream service ports 9001-9010 and accurately flagging status 200 (Healthy `CLOSED`), status 500 (`OPEN (500 ERR)`), or connection failure (`UNREACHABLE`).
- **Interactive Live API & Proxy Route Tester**: Interactive request composer with header presets, custom header inputs, execution time calculation (in ms), status badges, and formatted JSON response preview.
- **CORS & Error Simulation in Dummy Services**: Added CORS headers (`Access-Control-Allow-Origin: *`) and 500 error route handling (`/500`, `/error`, `?fail=true`) in `dummy-services/services.go`.

### Related Tasks

- `TASK-016`: Implement Mobile-First Control Center and Proxy Dashboard UI

## 2026-08-12 - Prototype 7 Release (Load Balancing)

### Added

- **Upstream Reverse Proxy Load Balancing**: Multi-target load balancing with pluggable `LoadBalancer` interface, thread-safe Round-Robin selection algorithm (`round_robin`), YAML `targets` & `algorithm` configuration, and `r.ProxyBalancer` router helpers (`TASK-011`, `REQ-011`).
- **Dummy Web Services Test Suite**: 10 dummy HTTP web services running on ports 9001–9010 (`dummy-services/`) for testing reverse proxy routing and load balancer upstream targets (`TASK-012`, `REQ-012`).
- **Upstream Health Check & Circuit Breaker**: Active HTTP health check probing (`health_check_path`), 3-state Circuit Breaker (`Closed`, `Open`, `HalfOpen`), automatic offline target filtering, and recovery cooldown management (`TASK-014`, `REQ-014`).
- **Full Proxy Route Integration Configuration**: Updated `config.yaml` with active proxy routing (`proxy.enabled: true`) mapping all path, header, single-target, and load-balanced routes to dummy services 9001–9010 (`TASK-013`, `REQ-013`).
- **Standardized REST Client Test File**: Created `test_endpoint.http` in root directory for one-click HTTP request execution across all native, static, header, path, and proxy endpoints (`TASK-015`, `REQ-015`).

### Related Tasks

- `TASK-011`: Reverse Proxy Load Balancer Implementation
- `TASK-012`: Add 10 Dummy Web Services for Upstream Testing
- `TASK-013`: Update config.yaml to Map Proxy Routes to Dummy Web Services
- `TASK-014`: Implement Upstream Health Check and Circuit Breaker
- `TASK-015`: Create test_endpoint.http REST Client Test File

## 2026-08-11 - Prototype 1, 2, 3, 4, 5 & 6 Release

### Added

- **Event Reactor Core Engine**: High-performance non-blocking TCP socket listener and connection event loop with worker pool dispatching (`TASK-001`, `REQ-001`).
- **HTTP/1.1 Protocol Parser**: Zero-copy streaming request parsing and formatted response serialization (`TASK-002`, `REQ-002`).
- **HTTP Router & Middleware**: URL routing, HTTP method dispatching, and middleware chain support (`TASK-003`, `REQ-003`).
- **Security Guards**: Request header size limits (8KB), body size limits (4MB), socket read/write timeouts, and panic recovery middleware (`TASK-004`, `REQ-005`).
- **Server Orchestration**: Command line entry point `cmd/toron/main.go` with `/health` and `/` endpoints and graceful shutdown handling (`TASK-005`, `REQ-001`).
- **Static File Serving**: Built-in static website hosting with MIME type detection, `index.html` resolution, and path traversal security guards (`TASK-006`, `REQ-006`).
- **Extensible Configuration System**: External configuration file support (`config.yaml`), CLI argument `-config` flag, and extensible `pkg/config` loader architecture (`TASK-007`, `REQ-007`).
- **Native Go Benchmarking Suite**: Standard Go `testing.B` benchmarks across `pkg/reactor`, `pkg/httpparser`, `pkg/router`, and `pkg/server` with memory allocation tracking (`TASK-008`, `REQ-008`).
- **Reverse Proxy & Upstream Gateway**: Built-in HTTP reverse proxy engine (`pkg/proxy`, `router.Proxy(prefix, target)`), forwarding requests, injecting `X-Forwarded-*` headers, and supporting YAML proxy routes (`TASK-009`, `REQ-009`).
- **Header-Based HTTP Routing**: Conditional route matching and proxy forwarding based on HTTP header key/value conditions (`r.GETHeader`, `r.ProxyHeader`, `proxy.routes[].headers`), supporting API versioning (`X-Version: v2`) and canary routing (`TASK-010`, `REQ-010`).

### Related Tasks

- `TASK-001`: Core Event Reactor Engine Implementation
- `TASK-002`: HTTP/1.1 Streaming Request Parser & Response Builder
- `TASK-003`: HTTP Router & Middleware Pipeline Implementation
- `TASK-004`: Security Guards, Request Limits & Connection Timeouts
- `TASK-005`: Toron Server Orchestration & Main Application Entry
- `TASK-006`: Static File Handler & Path Traversal Guard Implementation
- `TASK-007`: Extensible Config Loader Package & CLI Flag Integration
- `TASK-008`: Benchmark Suite Implementation for Core Server Packages
- `TASK-009`: Reverse Proxy Handler & Upstream Transport Implementation
- `TASK-010`: Header-Based Route Matching & Config Integration
