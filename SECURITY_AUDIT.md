# 🛡️ Toron Web Server & Edge Gateway: Security Audit Report (`toron_v3`)

**Role**: Security Analyst / Security Researcher (AGENT-007)  
**Date**: September 5, 2026 (Post-Remediation Full Audit)  
**Audited Commit**: `a4bada8` (`master` post-`TASK-061` through `TASK-072`)  
**Reference Document**: [`docs/securityReview/SR-070.md`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-070.md) (Historical: [`docs/securityReview/SR-054.md`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-054.md))

---

## 1. Executive Summary

This security audit report has been updated to reflect the current codebase following recent commits (`TASK-058` multi-stream logging, `TASK-059` target subpath preservation via `JoinProxyPath`, and `TASK-060` query parameter forwarding).

Each identified security vulnerability has been broken down into an atomic functional/non-functional requirement document ([`REQ-061`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-061.md) through [`REQ-072`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-072.md)) owned by the Requirement Engineer (`AGENT-002`), and mapped directly to bounded engineering task specifications ([`TASK-061`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-061.md) through [`TASK-072`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-072.md)) owned by the Development Lead (`AGENT-003`) to ensure verifiable, test-driven remediation.

---

## 2. Comprehensive Vulnerability Priority & Requirement Mapping Matrix

| Priority | ID | Title | Severity | CWE | Status | Mapped Task | Verification | Commit |
| :---: | :--- | :--- | :---: | :--- | :---: | :--- | :--- | :---: |
| **P0** | **SEC-01** | HTTP Request Smuggling & Connection Desync via Chunked TE | **Critical** | CWE-444 | **Resolved** | [`TASK-061`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-061.md) | `TC-061`, `SR-058`, `CR-057` | `b148b7d` |
| **P1** | **SEC-02** | Missing Authentication & Internal SSRF on Management APIs | **High** | CWE-306, CWE-918 | **Resolved** | [`TASK-062`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-062.md) | `TC-062`, `SR-059`, `CR-058` | `5e2ca8e` |
| **P1** | **SEC-03** | Shared Cache Session Leakage (`Set-Cookie` & Auth Caching) | **High** | CWE-524, CWE-539 | **Resolved** | [`TASK-063`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-063.md) | `TC-063`, `SR-060`, `CR-059` | `45ade09` |
| **P1** | **SEC-04** | Denial of Service (OOM) via Unbounded Rate-Limiter Map | **High** | CWE-400, CWE-770 | **Resolved** | [`TASK-064`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-064.md) | `TC-064`, `SR-061`, `CR-060` | `69a0ed3` |
| **P1** | **SEC-05** | Upstream TLS Verification Disabled in WebSocket Proxy | **High** | CWE-295 | **Resolved** | [`TASK-065`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-065.md) | `TC-065`, `SR-062`, `CR-061` | `37c8878` |
| **P2** | **SEC-06** | CRLF Log Injection & Log Forgery in Access Logger | **Medium** | CWE-117 | **Resolved** | [`TASK-066`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-066.md) | `TC-066`, `SR-063`, `CR-062` | `20aa0bd` |
| **P2** | **SEC-07** | Upstream Path Traversal via Uncleaned Route Path | **Medium** | CWE-22 | **Resolved** | [`TASK-067`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-067.md) | `TC-067`, `SR-064`, `CR-063` | `f4d6652` |
| **P2** | **SEC-08** | Unbounded Allocation Panic in gRPC Frame Decoder | **Medium** | CWE-789 | **Resolved** | [`TASK-068`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-068.md) | `TC-068`, `SR-065`, `CR-064` | `ac5baf5` |
| **P2** | **SEC-09** | Permissive CORS Wildcard Reflection with Credentials | **Medium** | CWE-942 | **Resolved** | [`TASK-069`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-069.md) | `TC-069`, `SR-066`, `CR-065` | `b3bb02d` |
| **P2** | **SEC-10** | Open Redirect via Unvalidated Host Header in HTTPS Upgrade | **Medium** | CWE-601 | **Resolved** | [`TASK-070`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-070.md) | `TC-070`, `SR-067`, `CR-066` | `f7eafe2` |
| **P2** | **SEC-11** | Client IP & Protocol Header Spoofing in Reverse Proxy | **Medium** | CWE-345 | **Resolved** | [`TASK-071`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-071.md) | `TC-071`, `SR-068`, `CR-067` | `ff95b29` |
| **P3** | **SEC-12** | HTTP Parameter Pollution (HPP) in Query Forwarding | **Low** | CWE-235 | **Resolved** | [`TASK-072`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-072.md) | `TC-072`, `SR-069`, `CR-068` | `ee4d29d` |
| **P0** | **SEC-13** | Stored DOM-based XSS via Security Audit Log View | **Critical** | CWE-79 | **Open** | Pending Task | — | — |
| **P0** | **SEC-14** | HTTP Request Smuggling via Header Field-Name Trailing Whitespace | **Critical** | CWE-444 | **Open** | Pending Task | — | — |
| **P0** | **SEC-15** | HTTP Request Smuggling (CL.CL Desync) via Multiple Content-Length | **Critical** | CWE-444 | **Open** | Pending Task | — | — |
| **P1** | **SEC-16** | Denial of Service (OOM) via Unbounded Request Body in HTTP/2 Adapter | **High** | CWE-400, CWE-770 | **Open** | Pending Task | — | — |
| **P1** | **SEC-17** | Connection Starvation & DoS via Reactor Worker Pool Monopolization | **High** | CWE-400 | **Open** | Pending Task | — | — |
| **P1** | **SEC-18** | Global Authentication & WAF Bypass via Root / Empty Path Exclusions | **High** | CWE-287, CWE-693 | **Open** | Pending Task | — | — |
| **P2** | **SEC-19** | Ingress Route Hijacking via Unrestricted Container Discovery Labels | **Medium** | CWE-284 | **Open** | Pending Task | — | — |
| **P2** | **SEC-20** | WAF Path Traversal Bypass via Uppercase Percent-Encoding (`TRAVERSAL-001`) | **Medium** | CWE-693, CWE-22 | **Open** | Pending Task | — | — |
| **P2** | **SEC-21** | Request Body Dropping in Sidecar Proxy Engine | **Medium** | CWE-436 | **Open** | Pending Task | — | — |
| **P3** | **SEC-22** | Missing Expiration (`exp`) Claim Enforcement in JWT Verification | **Low** | CWE-613 | **Open** | Pending Task | — | — |

---

## 3. Detailed Vulnerability Findings & Requirement Mapping

### Priority 1: Critical (P0)

#### SEC-01: HTTP Request Smuggling & Connection Desync via Unsupported Chunked Transfer-Encoding
- **Severity**: **Critical** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N - Score 9.1)
- **Location**: [`pkg/httpparser/parser.go:L97-L120`](file:///Users/sneha/Developer/toron/pkg/httpparser/parser.go#L97-L120)
- **Mapped Requirement**: [`REQ-061: HTTP Request Smuggling Prevention and Transfer-Encoding Protocol Enforcement`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-061.md)
- **Mapped Task**: [`TASK-061: Implement HTTP Request Smuggling Prevention and Transfer-Encoding Protocol Guard`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-061.md)
- **Root Cause**:  
  `httpparser.ParseRequest` verifies that a request does not declare both `Content-Length` and `Transfer-Encoding`. However, when a client sends **only** `Transfer-Encoding: chunked`:
  - `ContentLength` defaults to `0` and `req.Body` remains `nil`.
  - The chunked payload body is **not consumed from the socket**.
  - In [`pkg/server/server.go:L182-L298`](file:///Users/sneha/Developer/toron/pkg/server/server.go#L182-L298), persistent keep-alive connections are enabled by default.
  - On the subsequent loop iteration, `ParseRequest` parses the unconsumed chunked payload from the stream as the request line of the *next* HTTP request.
- **Attack Vector**:  
  An attacker transmits an HTTP/1.1 request with `Transfer-Encoding: chunked` followed by an embedded secondary HTTP request inside the chunk payload. The parser processes the outer request, leaves the payload on the socket, and treats the embedded payload as an authentic subsequent request on the pooled connection.
- **Impact**:  
  Request smuggling (CL.TE/TE desync), cross-tenant session hijacking, cache poisoning, and unauthorized endpoint invocation.
- **Remediation**:  
  Implement [`REQ-061`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-061.md) via [`TASK-061`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-061.md): If Toron does not implement a full chunked transfer decoder in `httpparser`, it MUST reject any request containing `Transfer-Encoding` with `HTTP 501 Not Implemented` or terminate the connection immediately.

---

### Priority 2: High Severity (P1)

#### SEC-02: Missing Authentication & Internal SSRF on Management APIs
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:L/A:N - Score 8.2)
- **Location**: [`pkg/server/internal_api.go:L87-L448`](file:///Users/sneha/Developer/toron/pkg/server/internal_api.go#L87-L448)
- **Mapped Requirement**: [`REQ-062: Access Control, Authentication, and SSRF Hardening for Internal Management APIs`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-062.md)
- **Mapped Task**: [`TASK-062: Implement Authentication, Authorization, and SSRF Hardening for Internal APIs`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-062.md)
- **Root Cause**:  
  The internal management endpoints (`/internal/api/status`, `/internal/api/routes`, `/internal/api/upstreams/health`, `/internal/api/security/incidents`, `/internal/api/proxy-test`) are registered directly on the primary router without authentication or source IP filtering.
  Furthermore, `POST /internal/api/proxy-test` accepts user-supplied JSON parameters (`path`, `method`, `headers`) and issues an HTTP request to `http://127.0.0.1:<port>` with arbitrary attacker-supplied headers.
- **Attack Vector**:  
  External actors can query `/internal/api/*` to obtain internal backend IPs, hostnames, container mappings, and security incident histories. Additionally, an attacker can use `POST /internal/api/proxy-test` to issue requests originating from localhost (`127.0.0.1`), injecting custom authentication headers (`X-Authenticated-User: admin`) to access restricted administrative routes.
- **Impact**:  
  Reconnaissance of internal infrastructure, bypass of perimeter access controls, and internal Server-Side Request Forgery.
- **Remediation**:  
  Implement [`REQ-062`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-062.md) via [`TASK-062`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-062.md): Enforce authentication middleware on all `/internal/api/*` endpoints and restrict access to administrative subnets.

#### SEC-03: Shared Cache Session Leakage (`Set-Cookie` & Auth Caching)
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:L/A:N - Score 8.2)
- **Location**: [`pkg/router/cache.go:L185`](file:///Users/sneha/Developer/toron/pkg/router/cache.go#L185), [`pkg/router/cache.go:L247-L266`](file:///Users/sneha/Developer/toron/pkg/router/cache.go#L247-L266)
- **Mapped Requirement**: [`REQ-063: Shared HTTP Response Cache Security, Session Cookie Isolation, and Authorization Boundaries`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-063.md)
- **Mapped Task**: [`TASK-063: Implement Shared Cache Set-Cookie Stripping and Authorization Isolation`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-063.md)
- **Root Cause**:  
  1. In `pkg/router/cache.go`, the caching middleware stores all upstream response headers, **including `Set-Cookie`**, in the `clonedHeader` map. When subsequent users request the cached URL, the saved `Set-Cookie` header is served to them (violating RFC 7234 §8).
  2. The cache key is constructed purely from method, host, and URI (`req.Method + ":" + extractHost(req) + ":" + uri`). It does not incorporate the `Authorization` header, causing authenticated responses to be cached and served to unauthenticated clients.
- **Attack Vector**:  
  User A accesses a cached endpoint and receives a session cookie. User B accesses the same endpoint shortly after and is issued User A's session cookie.
- **Impact**:  
  Account takeover, session fixation, and cross-user data leakage.
- **Remediation**:  
  Implement [`REQ-063`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-063.md) via [`TASK-063`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-063.md): Explicitly strip `Set-Cookie` headers from responses before storing them in cache, and enforce authorization boundaries per RFC 7234 §3.2.

#### SEC-04: Denial of Service (OOM) via Unbounded Rate-Limiter Bucket Map
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H - Score 7.5)
- **Location**: [`pkg/router/rate_limiter.go:L96`](file:///Users/sneha/Developer/toron/pkg/router/rate_limiter.go#L96), [`pkg/router/rate_limiter.go:L131-L152`](file:///Users/sneha/Developer/toron/pkg/router/rate_limiter.go#L131-L152)
- **Mapped Requirement**: [`REQ-064: Rate Limiter Memory Bounding, State Eviction, and Client Identity Verification`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-064.md)
- **Mapped Task**: [`TASK-064: Implement Rate Limiter Memory Bounding, State Eviction, and Client IP Verification`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-064.md)
- **Root Cause**:  
  `RateLimiter` stores client buckets in `buckets map[string]*TokenBucket` without any size limitation, entry eviction, or TTL expiration.
  Additionally, `ExtractClientKey` prioritizes unverified headers (`X-API-Key`, `X-Forwarded-For`).
- **Attack Vector**:  
  An attacker sends high volumes of requests with randomized `X-Forwarded-For` or `X-API-Key` headers. Each unique header bypasses rate limits (receiving an unexhausted bucket) and permanently consumes memory in `rl.buckets`.
- **Impact**:  
  Complete rate limit evasion and continuous heap allocation culminating in an Out-Of-Memory (OOM) kernel termination.
- **Remediation**:  
  Implement [`REQ-064`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-064.md) via [`TASK-064`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-064.md): Introduce an LRU cache or periodic cleanup worker to purge expired buckets, and derive client IP strictly from socket `RemoteAddr` unless coming from a verified trusted proxy.

#### SEC-05: Upstream TLS Certificate Verification Disabled in WebSocket Proxy
- **Severity**: **High** (CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:N - Score 7.4)
- **Location**: [`pkg/proxy/proxy.go:L778`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go#L778)
- **Mapped Requirement**: [`REQ-065: Upstream TLS Certificate Verification and Trust Management in WebSocket Proxy`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-065.md)
- **Mapped Task**: [`TASK-065: Enforce Upstream TLS Certificate Verification in WebSocket Reverse Proxy`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-065.md)
- **Root Cause**:  
  When establishing upstream WebSocket connections over TLS (`wss://` or `https://`), Toron hardcodes `InsecureSkipVerify: true`:
  ```go
  upstreamConn, dialErr = tls.Dial("tcp", host, &tls.Config{InsecureSkipVerify: true})
  ```
- **Attack Vector**:  
  An adversary capable of intercepting traffic between Toron and the upstream backend service can present an arbitrary self-signed or forged TLS certificate. Toron accepts the certificate unconditionally.
- **Impact**:  
  Man-in-the-Middle (MitM) eavesdropping and frame injection on encrypted WebSocket communication.
- **Remediation**:  
  Implement [`REQ-065`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-065.md) via [`TASK-065`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-065.md): Remove hardcoded `InsecureSkipVerify: true` and validate against root CAs or a configured custom CA bundle in `ProxyOptions.TLS`.

---

### Priority 3: Medium Severity (P2)

#### SEC-06: NEW: CRLF Log Injection & Log Forgery in Multi-Stream Access Logger
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N - Score 5.3)
- **Location**: [`pkg/logging/manager.go:L388-L392`](file:///Users/sneha/Developer/toron/pkg/logging/manager.go#L388-L392)
- **Mapped Requirement**: [`REQ-066: Access Log CRLF Sanitization and Log Forgery Prevention`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-066.md)
- **Mapped Task**: [`TASK-066: Implement Access Log CRLF Sanitization and Control Character Filtering`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-066.md)
- **Root Cause**:  
  In text-format logging (the default format), client-controlled values (`entry.UserAgent`, `entry.Referer`, `entry.Method`, and `entry.Path`) are formatted directly into the log line using `fmt.Sprintf` without sanitizing carriage return (`\r`) or line feed (`\n`) characters.
- **Attack Vector**:  
  An attacker transmits an HTTP request with an embedded newline sequence in `User-Agent` or `Referer`, injecting falsified records into the access log file.
- **Impact**:  
  Log poisoning and audit trail tampering. Attackers can forge access logs to hide malicious activity or inject misleading entries into SIEM/log parsers.
- **Remediation**:  
  Implement [`REQ-066`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-066.md) via [`TASK-066`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-066.md): Sanitize control characters (`\r`, `\n`) from all fields before writing to text log sinks.

#### SEC-07: NEW: Upstream Path Traversal via Uncleaned Route Path in `JoinProxyPath`
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N - Score 6.5)
- **Location**: [`pkg/proxy/proxy.go:L718-L763`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go#L718-L763)
- **Mapped Requirement**: [`REQ-067: Upstream Path Canonicalization and Directory Traversal Prevention in Proxy Routing`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-067.md)
- **Mapped Task**: [`TASK-067: Implement Upstream Path Canonicalization and Route Traversal Guards`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-067.md)
- **Root Cause**:  
  `httpparser.NewRequest` sets `req.Path = parsedURL.Path` using `url.ParseRequestURI`, which preserves uncleaned dot segments (`..`).  
  When proxying, `JoinProxyPath` performs `strings.TrimPrefix(reqPath, prefix)` without normalizing path segments.
- **Attack Vector**:  
  If a route is configured with `prefix: "/api"` and `target: "http://upstream/v1"`, an incoming request with `Path = "/api/../admin"` matches the prefix route. `JoinProxyPath` produces `/v1/../admin`, escaping the `/v1` namespace on the backend.
- **Impact**:  
  Path traversal and prefix route boundary confusion against upstream microservices.
- **Remediation**:  
  Implement [`REQ-067`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-067.md) via [`TASK-067`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-067.md): Clean and normalize `req.Path` using `path.Clean` prior to prefix matching and upstream path joining.

#### SEC-08: Unbounded Memory Allocation Panic in gRPC Transcoder Frame Decoder
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H - Score 6.5)
- **Location**: [`pkg/transcoder/framer.go:L27-L35`](file:///Users/sneha/Developer/toron/pkg/transcoder/framer.go#L27-L35)
- **Mapped Requirement**: [`REQ-068: Wire Frame Size Bounding and Memory Allocation Protection in gRPC Transcoder`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-068.md)
- **Mapped Task**: [`TASK-068: Implement Wire Frame Size Bounding and Buffer Allocation Protection in gRPC Transcoder`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-068.md)
- **Root Cause**:  
  `DecodeGRPCFrame` reads a 4-byte `uint32` length directly from the wire frame header and immediately calls `make([]byte, length)`.
- **Attack Vector**:  
  A malfunctioning or hostile upstream gRPC endpoint returns a frame header declaring an oversized length (e.g. 2 GB - 4 GB). The runtime panics with an out-of-memory or allocation size error.
- **Impact**:  
  Panic and crash of worker goroutines or the entire server process.
- **Remediation**:  
  Implement [`REQ-068`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-068.md) via [`TASK-068`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-068.md): Impose a strict upper bound (e.g. 4 MB or `MaxBodyBytes`) on the declared payload length before allocating memory.

#### SEC-09: Permissive CORS Wildcard Reflection with Credentials Enabled
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:H/I:N/A:N - Score 6.5)
- **Location**: [`pkg/router/cors.go:L47-L51`](file:///Users/sneha/Developer/toron/pkg/router/cors.go#L47-L51), [`pkg/router/cors.go:L122-L129`](file:///Users/sneha/Developer/toron/pkg/router/cors.go#L122-L129)
- **Mapped Requirement**: [`REQ-069: Strict CORS Origin Validation and Credentialed Wildcard Mitigation`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-069.md)
- **Mapped Task**: [`TASK-069: Implement Strict CORS Origin Validation and Prohibit Credentialed Wildcards`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-069.md)
- **Root Cause**:  
  When `allow_origins` includes `*` and `allow_credentials` is `true`, `isOriginAllowed` matches all incoming origins. Instead of emitting `*` (which browsers disallow when credentials are true), the middleware reflects the request's `Origin` header dynamically while setting `Access-Control-Allow-Credentials: true`.
- **Impact**:  
  Any arbitrary origin is permitted to make authenticated credentialed cross-origin requests and read sensitive responses.
- **Remediation**:  
  Implement [`REQ-069`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-069.md) via [`TASK-069`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-069.md): Disallow wildcards (`*`) when `allow_credentials` is `true` during configuration validation.

#### SEC-10: Open Redirect via Unvalidated Host Header in HTTP-to-HTTPS Redirection
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N - Score 6.1)
- **Location**: [`pkg/server/server.go:L439-L485`](file:///Users/sneha/Developer/toron/pkg/server/server.go#L439-L485)
- **Mapped Requirement**: [`REQ-070: Host Header Validation and Open Redirect Mitigation in HTTP-to-HTTPS Redirection`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-070.md)
- **Mapped Task**: [`TASK-070: Implement Host Header Validation in Cleartext HTTP-to-HTTPS Redirection`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-070.md)
- **Root Cause**:  
  In `serveHTTPRedirect`, Toron sanitizes the incoming `Host` header against whitespace, backslashes, and control characters (`\r\n\t /\\`). However, it does not validate whether the host matches configured domains or SNI routes. It reflects the client-supplied `Host` directly into the `301 Moved Permanently` `Location` header.
- **Attack Vector**:  
  An attacker distributes links to `http://gateway-ip/login` with `Host: evil.com`. Victims visiting the cleartext HTTP endpoint are redirected to `https://evil.com/login`.
- **Impact**:  
  Facilitates phishing campaigns and credential harvesting using the gateway as an open redirector.
- **Remediation**:  
  Implement [`REQ-070`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-070.md) via [`TASK-070`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-070.md): Validate incoming `Host` against the server's registered SNI host registry or configured domain whitelist before issuing redirects.

#### SEC-11: Client IP & Forwarded Protocol Header Spoofing in Reverse Proxy
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N - Score 5.3)
- **Location**: [`pkg/proxy/proxy.go:L610-L625`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go#L610-L625)
- **Mapped Requirement**: [`REQ-071: Client Connection State Integrity for Forwarded Ingress Headers`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-071.md)
- **Mapped Task**: [`TASK-071: Implement Connection State Integrity and Hop-by-Hop Stripping for Forwarded Ingress Headers`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-071.md)
- **Root Cause**:  
  Toron trusts client-supplied `X-Forwarded-Proto` and `X-Real-IP` headers when generating upstream proxy headers.
- **Impact**:  
  Untrusted clients can spoof `X-Forwarded-Proto: https` over cleartext HTTP to bypass upstream HTTPS requirements, or spoof `X-Real-IP` to forge audit trails on downstream microservices.
- **Remediation**:  
  Implement [`REQ-071`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-071.md) via [`TASK-071`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-071.md): Set `X-Forwarded-Proto` strictly based on whether TLS was negotiated on the client socket, and derive `X-Forwarded-For` from `req.RawConn.RemoteAddr()`.

---

### Priority 4: Low Severity & Hardening (P3)

#### SEC-12: NEW: HTTP Parameter Pollution (HPP) Overwriting Target Parameters
- **Severity**: **Low** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N - Score 4.3)
- **Location**: [`pkg/proxy/proxy.go:L573-L581`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go#L573-L581)
- **Mapped Requirement**: [`REQ-072: HTTP Parameter Pollution (HPP) Mitigation in Target Query Merging`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-072.md)
- **Mapped Task**: [`TASK-072: Implement HTTP Parameter Pollution (HPP) Mitigation in Target Query Merging`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-072.md)
- **Root Cause**:  
  When forwarding queries, Toron appends client query strings after the target URL's existing query string (`outURL.RawQuery = targetURL.RawQuery + "&" + clientQuery`).
- **Impact**:  
  If a target URL defines constraints (e.g. `http://service?role=guest`), a client supplying `?role=admin` produces `?role=guest&role=admin`. For backend runtimes that parse query parameters by selecting the last occurrence, the client parameter overrides the target constraint.
- **Remediation**:  
  Implement [`REQ-072`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-072.md) via [`TASK-072`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-072.md): Strip or deduplicate client query keys that conflict with keys already defined in `targetURL.Query()`.

#### Defensive Hardening Observations
1. **WAF Regular Expression Case Sensitivity (`pkg/waf/rules.go:64`)**:  
   `TRAVERSAL-001` lacks the `(?i)` flag for percent-encoding, allowing uppercase percent-encoded sequences (`%2E%2E/`) to bypass detection.
2. **Container Route Namespace Scoping (`pkg/discovery/manager.go:231`)**:  
   Enforce an authorized namespace or label requirement to prevent rogue containers from registering the root path (`/`).
3. **Default Content Security Policy (`config.yaml:93`)**:  
   Define a restrictive default `csp` header in `security_headers`.

---

### Priority 5: Post-Remediation Newly Identified Findings (SR-070)

#### SEC-14: HTTP Request Smuggling via Header Field-Name Trailing Whitespace (RFC 7230 §3.2.4)
- **Severity**: **Critical** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N - Score 9.1)
- **Location**: [`pkg/httpparser/parser.go:L88-L104`](file:///Users/sneha/Developer/toron/pkg/httpparser/parser.go#L88-L104)
- **CWE**: CWE-444
- **Root Cause**:  
  `httpparser.ParseRequest` splits header lines on the first colon without rejecting trailing whitespace preceding the colon. If a client transmits `Transfer-Encoding : chunked` or `Transfer-Encoding\t: chunked`, the parsed header key contains trailing whitespace (`"Transfer-Encoding "`). Because `http.Header` lookups do not strip whitespace, `req.Header.Get("Transfer-Encoding")` returns empty, bypassing the Transfer-Encoding protocol check. Downstream services or proxies that strip whitespace will interpret the chunked payload, resulting in HTTP request smuggling.
- **Impact**: Request smuggling, cache poisoning, and security filter bypass.
- **Remediation**: Reject any header with `HTTP 400 Bad Request` if whitespace precedes the colon per RFC 7230 §3.2.4 (`strings.ContainsAny(k, " \t\r\n")`).

#### SEC-15: HTTP Request Smuggling (CL.CL Desync) via Multiple / Duplicate Content-Length Headers
- **Severity**: **Critical** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N - Score 9.1)
- **Location**: [`pkg/httpparser/parser.go:L107-L123`](file:///Users/sneha/Developer/toron/pkg/httpparser/parser.go#L107-L123)
- **CWE**: CWE-444
- **Root Cause**:  
  When multiple `Content-Length` headers are supplied (e.g. `Content-Length: 0\r\nContent-Length: 45\r\n`), `req.Header.Get("Content-Length")` returns only the first value. Toron reads 0 bytes and leaves the unconsumed 45 bytes on the persistent keep-alive connection (`pkg/server/server.go:L182-L250`), where it is parsed as the next incoming request line.
- **Impact**: Request desync, cross-tenant request hijacking, credential theft.
- **Remediation**: Enforce RFC 7230 §3.3.2 by verifying that multiple `Content-Length` headers are rejected with `HTTP 400 Bad Request` unless all values are identical.

#### SEC-16: Denial of Service (OOM) via Unbounded Request Body Ingestion in `http2AdapterHandler`
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H - Score 7.5)
- **Location**: [`pkg/server/server.go:L308-L313`](file:///Users/sneha/Developer/toron/pkg/server/server.go#L308-L313)
- **CWE**: CWE-400, CWE-770
- **Root Cause**:  
  In `http2AdapterHandler()`, the server reads incoming request bodies via `io.ReadAll(r.Body)` without enforcing `s.config.MaxBodyBytes`. An attacker transmitting an oversized payload over HTTP/2 can force unbounded memory allocation, triggering kernel Out-Of-Memory (OOM) termination.
- **Impact**: Server crash / Denial of Service.
- **Remediation**: Wrap `r.Body` with `io.LimitReader` or `http.MaxBytesReader` bounded by `MaxBodyBytes`, rejecting oversized payloads with `HTTP 413 Payload Too Large`.

#### SEC-17: Connection Starvation & Denial of Service via Reactor Worker Pool Monopolization
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H - Score 7.5)
- **Location**: [`pkg/reactor/reactor.go:L128-L176`](file:///Users/sneha/Developer/toron/pkg/reactor/reactor.go#L128-L176), [`pkg/server/server.go:L182-L301`](file:///Users/sneha/Developer/toron/pkg/server/server.go#L182-L301)
- **CWE**: CWE-400
- **Root Cause**:  
  The reactor allocates a fixed worker pool (`WorkerPoolSize`, default 128). Each worker synchronously runs `srv.handleConn`, looping over keep-alive requests for up to `IdleTimeout` (default 30s). When 128 clients idle concurrently, all workers block. Once the `tasks` channel (capacity 512) fills, `ln.Accept()` freezes, refusing all new TCP connections.
- **Impact**: Complete gateway denial of service under minimal connection load.
- **Remediation**: Process idle keep-alive states asynchronously or allocate goroutines per connection.

#### SEC-18: Global Authentication & WAF Bypass via Root / Empty Path Exclusions
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N - Score 8.6)
- **Location**: [`pkg/router/auth.go:L314-L320`](file:///Users/sneha/Developer/toron/pkg/router/auth.go#L314-L320), [`pkg/waf/middleware.go:L29-L35`](file:///Users/sneha/Developer/toron/pkg/waf/middleware.go#L29-L35)
- **CWE**: CWE-287, CWE-693
- **Root Cause**:  
  Path exclusion logic evaluates `strings.HasPrefix(req.Path, strings.TrimSuffix(p, "/")+"/")`. If `cfg.Excluded` contains `""` or `"/"`, the expression evaluates to `"/"`, matching 100% of valid paths and completely disabling authentication and WAF protection globally.
- **Impact**: Total authentication and threat protection bypass.
- **Remediation**: Discard empty exclusions and require exact matching for root path (`p == "/"`).

#### SEC-19: Ingress Route Hijacking via Unrestricted Container Discovery Labels
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:C/C:L/I:L/A:L - Score 6.0)
- **Location**: [`pkg/discovery/parser.go:L33-L40`](file:///Users/sneha/Developer/toron/pkg/discovery/parser.go#L33-L40), [`pkg/discovery/manager.go:L206-L233`](file:///Users/sneha/Developer/toron/pkg/discovery/manager.go#L206-L233)
- **CWE**: CWE-284
- **Root Cause**:  
  Any container with `toron.enable=true` on the local container engine can specify `toron.prefix: "/"` or shadow sensitive routes (`/api`, `/admin`). `m.router.RoutePrefix` registers the route and overrides existing router policies.
- **Impact**: Unauthorized traffic interception and endpoint shadowing.
- **Remediation**: Enforce container namespace restrictions and disallow registration of root (`/`) or administrative paths.

#### SEC-20: WAF Path Traversal Bypass via Uppercase Percent-Encoding in `TRAVERSAL-001`
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N - Score 5.3)
- **Location**: [`pkg/waf/rules.go:L61-L67`](file:///Users/sneha/Developer/toron/pkg/waf/rules.go#L61-L67)
- **CWE**: CWE-693, CWE-22
- **Root Cause**:  
  Rule `TRAVERSAL-001` lacks the case-insensitivity flag `(?i)`. Payloads containing uppercase percent-encoded sequences (`%2E%2E/`, `%2E%2e/`, `%2e%2E%2F`) in request headers and JSON bodies evade regex matching.
- **Impact**: WAF detection bypass for directory traversal attacks.
- **Remediation**: Add `(?i)` flag and expand pattern to cover uppercase and backslash hex encodings.

#### SEC-21: Request Body Dropping in Sidecar Proxy Engine
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:L - Score 5.3)
- **Location**: [`pkg/sidecar/proxy.go:L199-L202`](file:///Users/sneha/Developer/toron/pkg/sidecar/proxy.go#L199-L202), [`pkg/httpparser/request.go:L141`](file:///Users/sneha/Developer/toron/pkg/httpparser/request.go#L141)
- **CWE**: CWE-436
- **Root Cause**:  
  `httpparser.NewRequestFromStd(r)` instantiates `Body: bytes.NewReader(nil)`. In `pkg/sidecar/proxy.go`, incoming requests are adapted without preserving `r.Body`. Consequently, all POST, PUT, and PATCH bodies passing through the sidecar proxy are discarded.
- **Impact**: Data loss and API failure on mutating requests.
- **Remediation**: Populate `toronReq.Body` from `r.Body` prior to forwarding.

#### SEC-22: Missing Expiration (`exp`) Claim Enforcement in JWT Verification
- **Severity**: **Low** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N - Score 4.3)
- **Location**: [`pkg/router/auth.go:L171-L185`](file:///Users/sneha/Developer/toron/pkg/router/auth.go#L171-L185)
- **CWE**: CWE-613
- **Root Cause**:  
  `VerifyJWT` only validates the `exp` claim if present. Tokens without an `exp` claim remain valid indefinitely, exposing the system to permanent session replay risks.
- **Impact**: Permanent credential validity for leaked tokens.
- **Remediation**: Require `exp` claim by default or provide a configuration flag `require_exp: true`.

#### SEC-13: Stored DOM-based Cross-Site Scripting (DOM XSS) via Security Incident Audit Log
- **Severity**: **Critical** (CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:H/I:H/A:N - Score 9.3)
- **Location**: [`public/app.js:L421-L431`](file:///Users/sneha/Developer/toron/public/app.js#L421-L431)
- **CWE**: CWE-79
- **Root Cause**:  
  `fetchIncidentLogs()` renders unescaped incident fields (`inc.path`, `inc.client_ip`) directly into `tbody.innerHTML`. A malicious URI triggering a WAF rule is logged and executed as JavaScript in the administrator's browser upon viewing the dashboard.
- **Impact**: Administrative account takeover, session hijacking.
- **Remediation**: Replace `innerHTML` string interpolation with `textContent` or strict HTML escaping.

---

## 4. Remediation Sequence

- **Phase 1 (Immediate Protocol & Auth Hotfixes - P0 & P1)**:
  1. Reject unhandled `Transfer-Encoding: chunked` in [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron/pkg/httpparser/parser.go) ([`REQ-061`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-061.md), [`TASK-061`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-061.md)).
  2. Enforce administrative authentication and subnet controls on `/internal/api/*` in [`pkg/server/internal_api.go`](file:///Users/sneha/Developer/toron/pkg/server/internal_api.go) ([`REQ-062`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-062.md), [`TASK-062`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-062.md)).
  3. Strip `Set-Cookie` and check `Authorization` in [`pkg/router/cache.go`](file:///Users/sneha/Developer/toron/pkg/router/cache.go) ([`REQ-063`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-063.md), [`TASK-063`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-063.md)).
  4. Enforce TLS certificate validation in WebSocket proxy in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`REQ-065`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-065.md), [`TASK-065`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-065.md)).
  5. Add TTL expiration and capacity bounds to `RateLimiter` in [`pkg/router/rate_limiter.go`](file:///Users/sneha/Developer/toron/pkg/router/rate_limiter.go) ([`REQ-064`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-064.md), [`TASK-064`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-064.md)).
- **Phase 2 (Logging, Routing & Path Sanitization - P2)**:
  1. Sanitize control characters (`\r`, `\n`) in access logging in [`pkg/logging/manager.go`](file:///Users/sneha/Developer/toron/pkg/logging/manager.go) ([`REQ-066`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-066.md), [`TASK-066`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-066.md)).
  2. Clean `req.Path` with `path.Clean` prior to calling [`JoinProxyPath`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go#L718) in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`REQ-067`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-067.md), [`TASK-067`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-067.md)).
  3. Bound payload length in [`pkg/transcoder/framer.go`](file:///Users/sneha/Developer/toron/pkg/transcoder/framer.go) ([`REQ-068`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-068.md), [`TASK-068`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-068.md)).
  4. Disallow `*` origin reflection when credentials are enabled in [`pkg/router/cors.go`](file:///Users/sneha/Developer/toron/pkg/router/cors.go) ([`REQ-069`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-069.md), [`TASK-069`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-069.md)).
  5. Validate `Host` headers in HTTPS redirects against registered routes in [`pkg/server/server.go`](file:///Users/sneha/Developer/toron/pkg/server/server.go) ([`REQ-070`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-070.md), [`TASK-070`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-070.md)).
  6. Derive `X-Forwarded-*` headers strictly from socket connection state in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`REQ-071`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-071.md), [`TASK-071`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-071.md)).
- **Phase 3 (Query Hardening & Parameter Isolation - P3)**:
  1. Prevent query parameter pollution in target query merging in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`REQ-072`](file:///Users/sneha/Developer/toron/docs/requirements/REQ-072.md), [`TASK-072`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-072.md)).

---

## 5. Remediation Status & Verification Summary

All 12 backend security vulnerabilities (`SEC-01` through `SEC-12`) have been fully remediated, independently tested, security reviewed, code reviewed, and merged into `master`:

1. **`SEC-01` (`TASK-061`, Commit `b148b7d`)**: Enforced HTTP request smuggling mitigation via strict `Transfer-Encoding: chunked` rejection (`501 Not Implemented`) and header desync guards ([`TC-061`](file:///Users/sneha/Developer/toron/docs/testCases/TC-061.md), [`SR-058`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-058.md), [`CR-057`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-057.md)).
2. **`SEC-02` (`TASK-062`, Commit `5e2ca8e`)**: Hardened `/internal/api/*` endpoints with mandatory Bearer token authentication, subnet CIDR allowlisting, and loopback/link-local SSRF defenses ([`TC-062`](file:///Users/sneha/Developer/toron/docs/testCases/TC-062.md), [`SR-059`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-059.md), [`CR-058`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-058.md)).
3. **`SEC-03` (`TASK-063`, Commit `45ade09`)**: Enforced RFC 7234 compliance in shared caching: prohibited caching responses with `Set-Cookie` or requests with `Authorization` unless `s-maxage`, `public`, or `must-revalidate` are explicitly declared ([`TC-063`](file:///Users/sneha/Developer/toron/docs/testCases/TC-063.md), [`SR-060`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-060.md), [`CR-059`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-059.md)).
4. **`SEC-04` (`TASK-064`, Commit `69a0ed3`)**: Replaced unbounded rate-limiter sync map with bounded LRU eviction, automatic TTL cleanup, and spoof-resistant physical socket IP resolution ([`TC-064`](file:///Users/sneha/Developer/toron/docs/testCases/TC-064.md), [`SR-061`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-061.md), [`CR-060`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-060.md)).
5. **`SEC-05` (`TASK-065`, Commit `37c8878`)**: Secured upstream WebSocket TLS connections with strict certificate verification (`ServerName`, CA pools, `InsecureSkipVerify: false` by default) ([`TC-065`](file:///Users/sneha/Developer/toron/docs/testCases/TC-065.md), [`SR-062`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-062.md), [`CR-061`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-061.md)).
6. **`SEC-06` (`TASK-066`, Commit `20aa0bd`)**: Sanitized all access log fields (`Path`, `Method`, `User-Agent`, `Referer`, `RemoteAddr`) against CRLF (`\r`, `\n`) and ASCII control characters ([`TC-066`](file:///Users/sneha/Developer/toron/docs/testCases/TC-066.md), [`SR-063`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-063.md), [`CR-062`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-062.md)).
7. **`SEC-07` (`TASK-067`, Commit `f4d6652`)**: Secured `JoinProxyPath` against directory traversal (`/../`, encoded `%2e%2e`, backslashes) by canonicalizing request paths and bounding upstream root boundaries ([`TC-067`](file:///Users/sneha/Developer/toron/docs/testCases/TC-067.md), [`SR-064`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-064.md), [`CR-063`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-063.md)).
8. **`SEC-08` (`TASK-068`, Commit `ac5baf5`)**: Bounded wire frame allocation in gRPC transcoder with a strict 16MB maximum payload ceiling and safe buffer chunking ([`TC-068`](file:///Users/sneha/Developer/toron/docs/testCases/TC-068.md), [`SR-065`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-065.md), [`CR-064`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-064.md)).
9. **`SEC-09` (`TASK-069`, Commit `b3bb02d`)**: Prohibited reflective CORS `Access-Control-Allow-Origin: *` whenever `AllowCredentials: true` is configured, strictly requiring explicit origin matching ([`TC-069`](file:///Users/sneha/Developer/toron/docs/testCases/TC-069.md), [`SR-066`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-066.md), [`CR-065`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-065.md)).
10. **`SEC-10` (`TASK-070`, Commit `f7eafe2`)**: Eliminated Open Redirect in cleartext HTTP-to-HTTPS upgrade by validating `Host` against recognized registries (`AllowedHosts`, `DefaultHost`, `SNIRegistry`, route tables) ([`TC-070`](file:///Users/sneha/Developer/toron/docs/testCases/TC-070.md), [`SR-067`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-067.md), [`CR-066`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-066.md)).
11. **`SEC-11` (`TASK-071`, Commit `ff95b29`)**: Enforced connection state integrity for forwarded ingress headers (`X-Forwarded-Proto`, `X-Forwarded-For`), added `TrustedProxies` CIDR validation, and stripped RFC 7230 §6.1 hop-by-hop headers ([`TC-071`](file:///Users/sneha/Developer/toron/docs/testCases/TC-071.md), [`SR-068`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-068.md), [`CR-067`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-067.md)).
12. **`SEC-12` (`TASK-072`, Commit `ee4d29d`)**: Prevented HTTP Parameter Pollution (HPP) by enforcing immutable gateway target query parameter precedence and filtering colliding client query keys and semicolon delimiters ([`TC-072`](file:///Users/sneha/Developer/toron/docs/testCases/TC-072.md), [`SR-069`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-069.md), [`CR-068`](file:///Users/sneha/Developer/toron/docs/codeReview/CR-068.md)).

