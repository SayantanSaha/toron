# 🛡️ Toron Web Server & Edge Gateway: Security Audit Report (`toron_v3`)

**Role**: Security Analyst / Security Researcher (AGENT-007)  
**Date**: September 5, 2026 (Post-Remediation Comprehensive Audit)  
**Audited Commit**: `fd10764` (`master` post-`TASK-073` through `TASK-082`)  
**Reference Document**: [`docs/securityReview/SR-081.md`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-081.md) (Historical: [`docs/securityReview/SR-070.md`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-070.md), [`docs/securityReview/SR-054.md`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-054.md))

---

## 1. Executive Summary

This security audit report reflects the current state of the Toron Web Server and Edge Gateway codebase following the complete remediation, testing, code review, and merging of all 22 prior security tasks (`TASK-061` through `TASK-082`).

A comprehensive post-remediation security audit ([`SR-081`](file:///Users/sneha/Developer/toron/docs/securityReview/SR-081.md)) was conducted across all subsystems—including Kubernetes Ingress Controllers, Layer 4 Proxies, REST-to-gRPC Transcoding, Service Mesh Sidecars, WebSocket Upgrades, and Logging. The 22 previously identified vulnerabilities remain 100% verified and resolved. **8 new findings** (`SEC-23` through `SEC-30`) have been identified in the extended subsystem surfaces and are tracked below for remediation.

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
| **P0** | **SEC-13** | Stored DOM-based XSS via Security Audit Log View | **Critical** | CWE-79 | **Resolved** | [`TASK-075`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-075.md) | `TC-075`, `SR-073`, `CR-071` | `4196b31` |
| **P0** | **SEC-14** | HTTP Request Smuggling via Header Field-Name Trailing Whitespace | **Critical** | CWE-444 | **Resolved** | [`TASK-073`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-073.md) | `TC-073`, `SR-071`, `CR-069` | `06301d6` |
| **P0** | **SEC-15** | HTTP Request Smuggling (CL.CL Desync) via Multiple Content-Length | **Critical** | CWE-444 | **Resolved** | [`TASK-074`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-074.md) | `TC-074`, `SR-072`, `CR-070` | `15ddbca` |
| **P1** | **SEC-16** | Denial of Service (OOM) via Unbounded Request Body in HTTP/2 Adapter | **High** | CWE-400, CWE-770 | **Resolved** | [`TASK-076`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-076.md) | `TC-076`, `SR-074`, `CR-072` | `f3d4843` |
| **P1** | **SEC-17** | Connection Starvation & DoS via Reactor Worker Pool Monopolization | **High** | CWE-400 | **Resolved** | [`TASK-077`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-077.md) | `TC-077`, `SR-075`, `CR-073` | `f7727de` |
| **P1** | **SEC-18** | Global Authentication & WAF Bypass via Root / Empty Path Exclusions | **High** | CWE-287, CWE-693 | **Resolved** | [`TASK-078`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-078.md) | `TC-078`, `SR-076`, `CR-074` | `ff2557f` |
| **P2** | **SEC-19** | Ingress Route Hijacking via Unrestricted Container Discovery Labels | **Medium** | CWE-284 | **Resolved** | [`TASK-079`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-079.md) | `TC-079`, `SR-077`, `CR-075` | `4bb3e98` |
| **P2** | **SEC-20** | WAF Path Traversal Bypass via Uppercase Percent-Encoding (`TRAVERSAL-001`) | **Medium** | CWE-693, CWE-22 | **Resolved** | [`TASK-080`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-080.md) | `TC-080`, `SR-078`, `CR-076` | `4203aba` |
| **P2** | **SEC-21** | Request Body Dropping in Sidecar Proxy Engine | **Medium** | CWE-436 | **Resolved** | [`TASK-081`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-081.md) | `TC-081`, `SR-079`, `CR-077` | `0bd0cc8` |
| **P3** | **SEC-22** | Missing Expiration (`exp`) Claim Enforcement in JWT Verification | **Low** | CWE-613 | **Resolved** | [`TASK-082`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-082.md) | `TC-082`, `SR-080`, `CR-078` | `b48848e` |
| **P1** | **SEC-23** | K8s Ingress Controller Watch Stream Unconsumed Channel Deadlock | **High** | CWE-400, CWE-833 | **Resolved** | [`TASK-084`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-084.md)..[`086`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-086.md) | `TC-084`, `SR-083`, `CR-080` | `defc678` |
| **P2** | **SEC-24** | Ingress Route Hijacking & Route Shadowing in K8s Ingress Controller | **Medium** | CWE-284, CWE-285 | **Resolved** | [`TASK-087`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-087.md)..[`089`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-089.md) | `TC-085`, `SR-084`, `CR-081` | `ec758e5` |
| **P2** | **SEC-25** | Silent Request Body Truncation in Service Mesh Sidecar Proxy | **Medium** | CWE-436, CWE-400 | **Resolved** | [`TASK-090`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-090.md)..[`092`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-092.md) | `TC-086`, `SR-085`, `CR-082` | `aceddda` |
| **P1** | **SEC-26** | Unbounded Goroutine & Socket Allocation in Layer 4 UDP/TCP Proxies | **High** | CWE-400 | **Resolved** | [`TASK-093`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-093.md)..[`096`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-096.md) | `TC-087`, `SR-086`, `CR-083` | `c53d655` |
| **P2** | **SEC-27** | Missing Maximum Idle Deadlines on Upgraded Protocol Connections | **Medium** | CWE-400 | **Resolved** | [`TASK-097`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-097.md)..[`100`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-100.md) | `TC-088`, `SR-087`, `CR-084` | `16a4df4` |
| **P2** | **SEC-28** | Unbounded Request Body Ingestion in REST-to-gRPC Transcoder | **Medium** | CWE-400, CWE-770 | **Resolved** | [`TASK-101`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-101.md)..[`104`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-104.md) | `TC-089`, `SR-088`, `CR-085` | `b789f93` |
| **P3** | **SEC-29** | Hop-by-Hop Header Leakage to Upstream in REST-to-gRPC Transcoder | **Low** | CWE-444, CWE-436 | **Resolved** | [`TASK-105`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-105.md)..[`107`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-107.md) | `TC-090`, `SR-089`, `CR-086` | `e1d86bc` |
| **P2** | **SEC-30** | Subpath Routing Interception & 502 Denial in REST-to-gRPC Transcoder | **Medium** | CWE-284, CWE-400 | **Resolved** | [`TASK-108`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-108.md)..[`110`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-110.md) | `TC-091`, `SR-090`, `CR-087` | `c5ede1b` |

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

### Priority 6: Newly Identified Findings Post-Remediation (SR-081)

#### SEC-23: K8s Ingress Controller Watch Stream Unconsumed Channel Deadlock & Goroutine Leak
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:H - Score 7.5)
- **Location**: [`pkg/ingress/controller.go:L44, L159-L164`](file:///Users/sneha/Developer/toron/pkg/ingress/controller.go#L44), [`pkg/ingress/client.go:L209`](file:///Users/sneha/Developer/toron/pkg/ingress/client.go#L209)
- **CWE**: CWE-400, CWE-833
- **Root Cause**:  
  `Controller` creates `c.events = make(chan K8sWatchEvent, 100)` and invokes `WatchIngresses`. However, no worker consumes `c.events`. When 100 events are sent, `events <- evt` blocks indefinitely. During server shutdown, `c.wg.Wait()` hangs forever waiting for `watchWorker` to exit.
- **Impact**: Server shutdown deadlock, goroutine leaks, and dropped Ingress lifecycle update events.
- **Remediation**: Implement an event consumer loop in `Controller` that drains `c.events` and dispatches route updates, and select on `ctx.Done()` when writing to `events`.

#### SEC-24: Ingress Route Hijacking & Sensitive Route Shadowing in Kubernetes Ingress Controller
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:L/A:L - Score 6.3)
- **Location**: [`pkg/ingress/translator.go:L40-L48`](file:///Users/sneha/Developer/toron/pkg/ingress/translator.go#L40-L48), [`pkg/ingress/controller.go:L146-L154`](file:///Users/sneha/Developer/toron/pkg/ingress/controller.go#L146-L154)
- **CWE**: CWE-284, CWE-285
- **Root Cause**:  
  `TranslateIngress` does not enforce prefix scoping or root shadowing restrictions. An Ingress with unhosted root (`host: ""` and `path: "/"`) or sensitive management routes (`/internal`, `/api/status`) is accepted and registered on the gateway router.
- **Impact**: Gateway root traffic hijacking and administrative endpoint shadowing.
- **Remediation**: Reject unhosted root paths and `/internal`, `/api/status` prefixes in `TranslateIngress`.

#### SEC-25: Silent Request Body Truncation in Service Mesh Sidecar Proxy
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N - Score 5.3)
- **Location**: [`pkg/sidecar/proxy.go:L201-L210`](file:///Users/sneha/Developer/toron/pkg/sidecar/proxy.go#L201-L210)
- **CWE**: CWE-436, CWE-400
- **Root Cause**:  
  `proxyToURL` reads request bodies using `io.LimitReader(r.Body, maxSidecarBody+1)`. When the body exceeds `maxSidecarBody` (10 MB), the excess bytes are truncated without returning `HTTP 413 Payload Too Large`, silently forwarding truncated data upstream.
- **Impact**: Upstream data corruption and protocol desynchronization.
- **Remediation**: Return `HTTP 413 Payload Too Large` immediately when `len(bodyBytes) > maxSidecarBody`.

#### SEC-26: Unbounded Goroutine & Socket Allocation in Layer 4 UDP/TCP Proxies
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H - Score 7.5)
- **Location**: [`pkg/proxy/udp.go:L76-L108`](file:///Users/sneha/Developer/toron/pkg/proxy/udp.go#L76-L108), [`pkg/proxy/tcp.go:L74-L109`](file:///Users/sneha/Developer/toron/pkg/proxy/tcp.go#L74-L109)
- **CWE**: CWE-400
- **Root Cause**:  
  In `UDPProxy.Serve`, every datagram spawns a new goroutine and dials a new outbound UDP socket without limits or socket reuse. In `TCPProxy.Serve`, accepted connections lack idle timeouts and connection tracking.
- **Impact**: File descriptor and memory exhaustion (OOM) via UDP floods or idle TCP Slowloris connections.
- **Remediation**: Enforce connection concurrency limits, socket reuse, and idle deadlines.

#### SEC-27: Missing Maximum Idle Deadlines on Upgraded Protocol Connections (WebSockets)
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:M - Score 5.3)
- **Location**: [`pkg/server/server.go:L273-L294`](file:///Users/sneha/Developer/toron/pkg/server/server.go#L273-L294)
- **CWE**: CWE-400
- **Root Cause**:  
  Upon protocol switching (`101 Switching Protocols`), all deadlines on client and upstream sockets are permanently cleared (`conn.SetDeadline(time.Time{})`), allowing silent/abandoned connections to leak goroutines indefinitely.
- **Impact**: Slowloris resource exhaustion on WebSocket/tunnel endpoints.
- **Remediation**: Implement a configurable tunnel idle deadline or enable TCP keep-alive probes.

#### SEC-28: Unbounded Request Body Ingestion in REST-to-gRPC Transcoder
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:L - Score 5.3)
- **Location**: [`pkg/transcoder/transcoder.go:L93-L103`](file:///Users/sneha/Developer/toron/pkg/transcoder/transcoder.go#L93-L103)
- **CWE**: CWE-400, CWE-770
- **Root Cause**:  
  `HandleTranscode` performs `io.ReadAll(req.Body)` without wrapping in an `io.LimitReader`, enabling memory exhaustion when handling oversized request payloads.
- **Impact**: Out-of-memory denial of service via large streaming JSON payloads.
- **Remediation**: Wrap `req.Body` with `io.LimitReader` bounded to a configured maximum payload ceiling (e.g. 4 MB).

#### SEC-29: Hop-by-Hop Header Leakage to Upstream in REST-to-gRPC Transcoder
- **Severity**: **Low** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N - Score 3.7)
- **Location**: [`pkg/transcoder/transcoder.go:L153-L161`](file:///Users/sneha/Developer/toron/pkg/transcoder/transcoder.go#L153-L161)
- **CWE**: CWE-444, CWE-436
- **Root Cause**:  
  `HandleTranscode` copies client request headers to the outgoing gRPC HTTP/2 request without stripping standard hop-by-hop headers (`Connection`, `Keep-Alive`, `Upgrade`, `TE`).
- **Impact**: Upstream gRPC server rejection or HTTP/2 protocol violation.
- **Remediation**: Strip hop-by-hop headers before forwarding headers to the gRPC client request.

#### SEC-30: Subpath Routing Interception & 502 Denial in REST-to-gRPC Transcoder
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:M - Score 5.3)
- **Location**: [`pkg/transcoder/transcoder.go:L83-L85`](file:///Users/sneha/Developer/toron/pkg/transcoder/transcoder.go#L83-L85)
- **CWE**: CWE-284, CWE-400
- **Root Cause**:  
  `registerRoutes` registers a dummy upstream prefix route with empty options (0 targets) alongside exact path routes. Parameterized requests (e.g. `/v1/users/123`) fall through to the empty prefix proxy and return `502 Bad Gateway`.
- **Impact**: Parameterized REST-to-gRPC endpoints are unreachable through the gateway.
- **Remediation**: Register prefix routes directly binding to `HandleTranscode` instead of dummy upstream proxies.

---

## 4. Remediation Sequence

- **Phase 1 (Immediate Protocol & Auth Hotfixes - P0 & P1 - COMPLETED)**:
  1. Reject unhandled `Transfer-Encoding: chunked` in [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron/pkg/httpparser/parser.go) ([`TASK-061`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-061.md), `b148b7d`).
  2. Enforce administrative authentication and subnet controls on `/internal/api/*` in [`pkg/server/internal_api.go`](file:///Users/sneha/Developer/toron/pkg/server/internal_api.go) ([`TASK-062`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-062.md), `5e2ca8e`).
  3. Strip `Set-Cookie` and check `Authorization` in [`pkg/router/cache.go`](file:///Users/sneha/Developer/toron/pkg/router/cache.go) ([`TASK-063`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-063.md), `45ade09`).
  4. Enforce TLS certificate validation in WebSocket proxy in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`TASK-065`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-065.md), `37c8878`).
  5. Add TTL expiration and capacity bounds to `RateLimiter` in [`pkg/router/rate_limiter.go`](file:///Users/sneha/Developer/toron/pkg/router/rate_limiter.go) ([`TASK-064`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-064.md), `69a0ed3`).
- **Phase 2 (Logging, Routing & Path Sanitization - P2 - COMPLETED)**:
  1. Sanitize control characters (`\r`, `\n`) in access logging in [`pkg/logging/manager.go`](file:///Users/sneha/Developer/toron/pkg/logging/manager.go) ([`TASK-066`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-066.md), `20aa0bd`).
  2. Clean `req.Path` with `path.Clean` prior to calling [`JoinProxyPath`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go#L718) in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`TASK-067`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-067.md), `f4d6652`).
  3. Bounded payload length in [`pkg/transcoder/framer.go`](file:///Users/sneha/Developer/toron/pkg/transcoder/framer.go) ([`TASK-068`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-068.md), `ac5baf5`).
  4. Disallow `*` origin reflection when credentials are enabled in [`pkg/router/cors.go`](file:///Users/sneha/Developer/toron/pkg/router/cors.go) ([`TASK-069`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-069.md), `b3bb02d`).
  5. Validate `Host` headers in HTTPS redirects against registered routes in [`pkg/server/server.go`](file:///Users/sneha/Developer/toron/pkg/server/server.go) ([`TASK-070`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-070.md), `f7eafe2`).
  6. Derive `X-Forwarded-*` headers strictly from socket connection state in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`TASK-071`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-071.md), `ff95b29`).
- **Phase 3 (Query Hardening & Parameter Isolation - P3 - COMPLETED)**:
  1. Prevent query parameter pollution in target query merging in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron/pkg/proxy/proxy.go) ([`TASK-072`](file:///Users/sneha/Developer/toron/docs/tasks/TASK-072.md), `ee4d29d`).
- **Phase 4 (Post-Remediation Hardening Tasks - SEC-13..22 - COMPLETED)**:
  1. `TASK-073` (SEC-14): Whitespace rejection before header colon delimiter (`06301d6`).
  2. `TASK-074` (SEC-15): Multiple and conflicting `Content-Length` rejection (`15ddbca`).
  3. `TASK-075` (SEC-13): Contextual HTML escaping in dashboard to eliminate DOM XSS (`4196b31`).
  4. `TASK-076` (SEC-16): Bounded request body reader with HTTP 413 in HTTP/2 adapter (`f3d4843`).
  5. `TASK-077` (SEC-17): Concurrent connection dispatch in TCP reactor (`f7727de`).
  6. `TASK-078` (SEC-18): Exact-match hardening for path exclusions in auth & WAF (`ff2557f`).
  7. `TASK-079` (SEC-19): Container route prefix scoping and shadowing guards (`4bb3e98`).
  8. `TASK-080` (SEC-20): Case-insensitive `(?i)` WAF `TRAVERSAL-001` pattern hardening (`4203aba`).
  9. `TASK-081` (SEC-21): Request body preservation in sidecar proxy engine (`0bd0cc8`).
  10. `TASK-082` (SEC-22): Mandatory `exp` claim enforcement in JWT verification (`b48848e`).
- **Phase 5 (Extended Subsystem Hardening - SEC-23..30 - COMPLETED)**:
  1. Resolve K8s Ingress Controller watch stream deadlock (`SEC-23`) - **COMPLETED** (`TASK-084`..`086`, `TC-084`, `SR-083`, `CR-080`).
  2. Enforce prefix scoping and route shadowing guards in K8s Ingress Controller (`SEC-24`) - **COMPLETED** (`TASK-087`..`089`, `TC-085`, `SR-084`, `CR-081`).
  3. Enforce 413 Payload Too Large on sidecar body overflow (`SEC-25`) - **COMPLETED** (`TASK-090`..`092`, `TC-086`, `SR-085`, `CR-082`).
  4. Implement worker pools and connection limits in Layer 4 proxies (`SEC-26`) - **COMPLETED** (`TASK-093`..`096`, `TC-087`, `SR-086`, `CR-083`).
  5. Support idle deadlines on upgraded WebSocket connections (`SEC-27`) - **COMPLETED** (`TASK-097`..`100`, `TC-088`, `SR-087`, `CR-084`).
  6. Bound request bodies in gRPC transcoder (`SEC-28`) - **COMPLETED** (`TASK-101`..`104`, `TC-089`, `SR-088`, `CR-085`).
  7. Filter hop-by-hop headers in gRPC transcoder (`SEC-29`) - **COMPLETED** (`TASK-105`..`107`, `TC-090`, `SR-089`, `CR-086`).
  8. Fix subpath dispatch in gRPC transcoder routing (`SEC-30`) - **COMPLETED** (`TASK-108`..`110`, `TC-091`, `SR-090`, `CR-087`).

---

## 5. Remediation Status & Verification Summary

30 security vulnerabilities (`SEC-01` through `SEC-30`) have been fully remediated, verified under `go test -count=1 -race ./...`, security reviewed, and code reviewed:
- **`SEC-01`..`SEC-12`**: Merged in commits `b148b7d` through `ee4d29d`.
- **`SEC-13`..`SEC-22`**: Merged in commits `06301d6` through `b48848e`.
- **`SEC-23`**: Verified in `TC-084` (`TASK-084`..`086`, `defc678`).
- **`SEC-24`**: Verified in `TC-085` (`TASK-087`..`089`).
- **`SEC-25`**: Verified in `TC-086` (`TASK-090`..`092`).
- **`SEC-26`**: Verified in `TC-087` (`TASK-093`..`096`).
- **`SEC-27`**: Verified in `TC-088` (`TASK-097`..`100`).
- **`SEC-28`**: Verified in `TC-089` (`TASK-101`..`104`).
- **`SEC-29`**: Verified in `TC-090` (`TASK-105`..`107`).
- **`SEC-30`**: Verified in `TC-091` (`TASK-108`..`110`, `c5ede1b`).

All 30 security vulnerabilities across core and extended subsystems are 100% verified and resolved.


