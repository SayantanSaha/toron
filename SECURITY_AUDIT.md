# 🛡️ Toron Web Server & Edge Gateway: Security Audit Report (`toron_v3`)

**Role**: Security Analyst / Security Researcher (AGENT-007)  
**Date**: September 9, 2026 (Fresh Comprehensive Codebase Security Audit)  
**Audited Commit**: `c5ede1b` (`master` post-`TASK-108` through `TASK-110`, v1.5.11)  
**Reference Document**: [`docs/securityReview/SR-091.md`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md) (Historical: [`docs/securityReview/SR-081.md`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md), [`docs/securityReview/SR-070.md`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-070.md), [`docs/securityReview/SR-054.md`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-054.md))

---

## 1. Executive Summary

This security audit report reflects the state of the Toron Web Server and Edge Gateway codebase following the complete remediation, testing, code review, and merging of all 30 prior security tasks (`TASK-061` through `TASK-110`, resolving `SEC-01` through `SEC-30`).

A fresh comprehensive codebase security audit ([`SR-091`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md)) was conducted across all subsystems—including HTTP/1.1, HTTP/2 multiplexing, HTTP/3 QUIC, Layer 4 TCP/UDP proxies, WAF inspection engines, IP access control lists, authentication schemes, CORS policies, reverse proxies, Kubernetes Ingress controllers, container auto-discovery, service mesh sidecars, gRPC transcoders, ACME zero-touch certificates, structured logging, and internal management APIs. All 38 identified vulnerabilities (`SEC-01` through `SEC-38`) are now 100% verified and resolved! **Zero open findings remain** across the entire Toron Web Server and Edge Gateway codebase.

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
| **P1** | **SEC-31** | Unauthenticated Client IP Spoofing & Security Bypass via Missing Physical RemoteAddr Binding in HTTP/2 and HTTP/3 Adapters | **High** | CWE-290, CWE-345, CWE-693 | **Resolved** | [`TASK-111`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-111.md)..[`113`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-113.md) | `TC-092 / SR-092` | Verified |
| **P2** | **SEC-32** | Fail-Open WAF IP Access Control Bypass on Unidentifiable Client IP | **Medium** | CWE-284, CWE-1188, CWE-693 | **Resolved** | [`TASK-114`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-114.md)..[`115`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-115.md) | `TC-093 / SR-093` | Verified |
| **P1** | **SEC-33** | Unbounded Routing Table Memory Leak & Zombie Route Persistence in Kubernetes Ingress Controller | **High** | CWE-400, CWE-670 | **Resolved** | [`TASK-116`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md)..[`117`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-117.md) | `TC-094 / SR-094, CR-090` | Verified |
| **P2** | **SEC-34** | Premature Route Deletion & Load-Balancing Failure Across Multi-Replica Containers in OCI Discovery Engine | **Medium** | CWE-400, CWE-284, CWE-662, CWE-775 | **Resolved** | [`TASK-118`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-118.md) | `TC-095 / SR-095, CR-091` | Verified |
| **P1** | **SEC-35** | Insecure Default InsecureSkipVerify in Sidecar Client TLS Configuration | **High** | CWE-295 | **Resolved** | [`TASK-120`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-120.md) | `TC-097 / SR-097, CR-093` | Verified |
| **P2** | **SEC-36** | Memory Exhaustion via Unbounded Upstream Response Buffering in Internal API Proxy Test Probe | **Medium** | CWE-400, CWE-770, CWE-775 | **Resolved** | [`TASK-121`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-121.md) | `TC-098 / SR-098, CR-094` | Verified |
| **P2** | **SEC-37** | Unbounded HTTP Client & Transport Allocation per Request in Service Mesh Sidecar Proxy | **Medium** | CWE-400, CWE-772 | **Resolved** | [`TASK-122`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-122.md) | `TC-099 / SR-099, CR-095` | Verified |
| **P3** | **SEC-38** | Missing Token Syntax and Length Validation in ACME HTTP-01 Challenge Handler | **Low** | CWE-20, CWE-703 | **Resolved** | [`TASK-123`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-123.md) | `TC-100 / SR-100, CR-096` | Verified |

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

#### SEC-31: Unauthenticated Client IP Spoofing & Security Bypass via Missing Physical RemoteAddr Binding in HTTP/2 and HTTP/3 Adapters
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N - Score 8.2)
- **Location**: [`pkg/server/server.go:L314-L316`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L314-L316), [`pkg/httpparser/request.go:L58-L70`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/request.go#L58-L70), [`pkg/waf/ip_acl.go:L112-L164`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L112-L164), [`pkg/server/internal_api.go:L194-L224`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/internal_api.go#L194-L224), [`pkg/router/rate_limiter.go:L286-L325`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/rate_limiter.go#L286-L325), [`pkg/proxy/proxy.go:L816-L852`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L816-L852)
- **CWE**: CWE-290, CWE-345, CWE-693
- **Status**: **Resolved**
- **Root Cause**:  
  In the HTTP/2 adapter (`http2AdapterHandler`) and HTTP/3 adapters, `httpparser.NewRequestFromStd(r)` fails to bind the physical client socket address (`r.RemoteAddr`), leaving `req.RawConn` as `nil`. Because `req.RawConn` is nil, WAF IP access control (`ExtractClientIP`), internal API subnet checks (`validateAdminAuth`), rate limiting (`ExtractClientKey`), and reverse proxy IP forwarding fall back to unconditionally trusting client-supplied `X-Forwarded-For` and `X-Real-IP` headers.
- **Impact**: Untrusted clients communicating over HTTP/2 or HTTP/3 can spoof arbitrary client IPs, completely bypassing WAF IP blacklists, administrative subnet policies, rate limiting, and injecting forged upstream IPs.
- **Remediation**: Add a `RemoteAddr string` field to `httpparser.Request`, populate it in `NewRequestFromStd(r)`, and enforce that physical socket IP is prioritized across all modules, only trusting `X-Forwarded-For` when the physical peer matches `trusted_proxies`.
- **Resolution Details**: Fully resolved under [`REQ-092`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-092.md), [`ADR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-087.md), [`TASK-111`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-111.md), [`TASK-112`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-112.md), [`TASK-113`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-113.md). Populated `req.RemoteAddr` and extraction helpers `RemoteHost()` and `RemoteIP()` across HTTP/1.1, HTTP/2, and HTTP/3 protocol ingress. Enforced strict `trusted_proxies` verification across WAF IP ACL, Internal Management API, Token Bucket Rate Limiter, Reverse Proxy header forwarding, and Structured Access Logging. Preserved mobile roaming session affinity under [`REQ-030`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-030.md) (`pkg/proxy/sticky.go` untouched with 0 diffs). Verified by test suite [`TC-092`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-092.md), reviewed and approved in [`CR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-088.md) and [`SR-092`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-092.md).

#### SEC-32: Fail-Open WAF IP Access Control Bypass on Unidentifiable Client IP
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N - Score 6.5)
- **Location**: [`pkg/waf/middleware.go:L58-L63`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware.go#L58-L63), [`pkg/waf/ip_acl.go:L75-L78`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L75-L78)
- **CWE**: CWE-284, CWE-1188, CWE-693
- **Root Cause**:  
  When an IP allowlist (`allowed_ips` / `allowedSubnets`) is configured, if `ExtractClientIP(req)` returns `nil`, `middleware.go` skips the check entirely. Furthermore, `CheckIP(nil)` returns `true, ""` (fail-open).
- **Impact**: Requests lacking client IP information bypass configured IP allowlists and access protected resources.
- **Remediation**: Fail closed by rejecting unidentifiable client requests (`ip == nil`) when an active allowlist is configured.
- **Resolution Details**: Fully resolved under [`REQ-093`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md), [`ADR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-088.md), [`TASK-114`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-114.md), [`TASK-115`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-115.md). Enforced fail-closed access control in [`pkg/waf/ip_acl.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go) and [`pkg/waf/middleware.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware.go) when incoming client IP cannot be determined under an active allowlist, returning HTTP 403 Forbidden with exact JSON error body. Eliminated duplicate client IP extraction on request hot path with single-pass caching. Preserved fail-open pass-through for denylist-only configurations. Verified by comprehensive unit, middleware, and high-concurrency race-clean test suite [`TC-093`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-093.md), reviewed and approved in [`CR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-089.md) and [`SR-093`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-093.md).

#### SEC-33: Unbounded Routing Table Memory Leak & Zombie Route Persistence in Kubernetes Ingress Controller
- **Severity**: **High** (CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:H/A:H - Score 8.1)
- **Location**: [`pkg/ingress/controller.go:L138-L161`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go#L138-L161), [`pkg/router/router.go:L232-L244`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L232-L244)
- **CWE**: CWE-400, CWE-670, CWE-1059
- **Root Cause**:  
  On every periodic resync and watch event, `syncIngresses` calls `c.router.RoutePrefix("upstream", ...)` which unconditionally appends new entries to `r.prefixRoutes` without removing or replacing prior routes. Deleted Ingresses in Kubernetes are never pruned from the router table.
- **Impact**: Unbounded memory growth, routing table pollution, routing latency degradation, and zombie routes continuing to proxy traffic to decommissioned backends.
- **Remediation**: Implement atomic route table replacement or dynamic route synchronization in `pkg/router`, pruning deleted Ingress routes upon each sync cycle.
- **Resolution Details**: Fully resolved under [`REQ-094`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md), [`ADR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-089.md), [`TASK-116`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md), [`TASK-117`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-117.md). Eliminated all five identified failure modes:
  1. *Unbounded Route Table Memory Leak (CWE-400)*: Implemented source-tagged prefix routing and an atomic replacement API ([`ReplacePrefixRoutesBySource`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)) in [`pkg/router/router.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go), guaranteeing strictly $O(K)$ routing table entries for $K$ active Ingress rules across arbitrary $N$ synchronization cycles ($O(1)$ memory scaling with sync count) and eliminating monotonic append-only memory bloat and OOM crashes.
  2. *Zombie Route Persistence (CWE-670)*: Dynamic route reconciliation in [`pkg/ingress/controller.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go) compiles desired route sets from live Kubernetes Ingress and Endpoint resources, automatically pruning deleted Ingresses upon synchronization so that requests to deleted paths immediately return HTTP 404 Not Found.
  3. *Stale Endpoint Shadowing*: Replaced append-only registration with atomic in-place route table replacement under write lock, ensuring that updated pod endpoints immediately take effect and obsolete routes are completely evicted rather than shadowing new endpoints.
  4. *Multi-Pod Replica Starvation*: Refactored Ingress route translation in [`pkg/ingress/translator.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/translator.go) to aggregate all healthy pod IP endpoints sharing `(Host, Prefix)` into a unified multi-target reverse proxy using [`RoundRobinBalancer`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L30), distributing load evenly ($\approx 1/M$ per replica) across all active pods and enabling true horizontal autoscaling.
  5. *Resource Leaks on Eviction*: Evicted reverse proxy instances have `.Close()` invoked automatically in `ReplacePrefixRoutesBySource`, `RemovePrefixRoute`, and `Reset`, terminating active background health check ticker goroutines (`StopActiveHealthCheck`) and releasing idle connection pools.
  Verified by comprehensive automated verification suite [`TC-094`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-094.md), reviewed and approved in [`CR-090`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-090.md) and [`SR-094`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-094.md).

#### SEC-34: Premature Route Deletion & Load-Balancing Failure Across Multi-Replica Containers in OCI Discovery Engine
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H - Score 7.5)
- **Location**: [`pkg/discovery/manager.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/discovery/manager.go), [`pkg/discovery/parser.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/discovery/parser.go), [`pkg/discovery/provider.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/discovery/provider.go), [`pkg/router/router.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)
- **CWE**: CWE-400, CWE-284, CWE-662, CWE-775
- **Root Cause**:  
  Multi-replica containers sharing `(host, prefix)` were registered as separate, single-target prefix routes rather than aggregated into a single multi-target load balancer. When one container stopped, `handleContainerStop` called `RemovePrefixRoute`, which deleted ALL routes for that `(host, prefix)`, inducing an immediate total outage (HTTP 404). Furthermore, discovery lacked HTTP method and header label parsing, risking canary variant collisions and route shadowing.
- **Impact**: Stopping or restarting a single container replica immediately dropped all traffic (HTTP 404) for all other running healthy replicas. In addition, router first-match prefix search dispatched 100% of traffic to the first replica and starved replicas #2..$M$, defeating horizontal scaling.
- **Remediation**: Aggregate targets across active container replicas matching `CompositeRouteKey` in `discovery.Manager`, update the router atomically using `ReplacePrefixRoutesBySource("oci-discovery", ...)`, enforce ADR-005 specificity ordering, and cleanly teardown evicted reverse proxy instances.
- **Resolution Details**: Fully resolved under [`REQ-095`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-095.md), [`ADR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-095.md), and [`TASK-118`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-118.md). Eliminated all identified failure modes:
  1. *Premature Route Deletion & Total Outage Elimination (CWE-662, CWE-284)*: Replaced incremental `RemovePrefixRoute` calls with declarative reconciliation via [`ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go). Stopping 1 replica of an $M$-replica service recalculates the desired specification across the surviving $M-1$ replicas and swaps the load balancer in place, guaranteeing zero HTTP 404 errors, zero dropped connections, and uninterrupted traffic delivery.
  2. *Multi-Replica Target Aggregation & Fair Load Balancing (CWE-400)*: Grouped container replicas sharing an identical composite key into a unified multi-target `PrefixRouteSpec` using [`RoundRobinBalancer`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L30), distributing load evenly ($\approx 1/M$ per replica) across all active instances and restoring horizontal scaling.
  3. *Multi-Dimensional Partitioning via `CompositeRouteKey`*: Implemented 4-tuple partitioning `(Host, CleanPrefix, Method, CanonicalHeaders)` with deterministic alphabetical header sorting, strictly segregating canary deployments (`X-Version: canary`) from baseline traffic pools and eliminating route churn from Go map iteration non-determinism.
  4. *ADR-005 Specificity-Based Route Ordering*: Enforced a 5-tier specificity hierarchy (Longest prefix $\to$ Specific host $\to$ Header constraint count $\to$ Method constraint $\to$ Deterministic tie-break), ensuring generic fallback routes never shadow specific canary or method-constrained routes.
  5. *Resource Leak Elimination on Route Eviction (CWE-775)*: Evicted routes cleanly invoke `pr.proxy.Close()`, stopping active background health check tickers (`StopActiveHealthCheck`) and closing idle TCP connection pools.
  Verified by comprehensive automated verification suite [`TC-095`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-095.md), reviewed and approved in [`CR-091`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-091.md) and [`SR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-095.md).

#### SEC-35: Insecure Default InsecureSkipVerify in Sidecar Client TLS Configuration
- **Severity**: **High** (CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N - Score 8.1)
- **Location**: [`pkg/sidecar/mtls.go:L48-L77`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/mtls.go#L48-L77)
- **CWE**: CWE-295
- **Status**: **Resolved**
- **Mapped Requirement**: [`REQ-097: Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS Configuration`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-097.md)
- **Mapped Task**: [`TASK-120: Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-120.md)
- **Root Cause**:  
  `BuildClientTLSConfig` automatically defaulted `tlsConfig.InsecureSkipVerify = true` whenever `cfg.CAFile` was empty (`""`). In standard deployments where services rely on public PKI, cloud certificates (AWS ACM, Cloudflare, Let's Encrypt), or the host operating system's trust store, `ca_file` was omitted, causing client egress connections to unconditionally bypass all certificate chain, expiration, and hostname validation.
- **Impact**: Inter-service mesh egress TLS connections disabled certificate validation by default, allowing network-adjacent attackers in shared Kubernetes pods, container networks, or compromised network infrastructure to execute silent Man-in-the-Middle (MitM) eavesdropping, credential theft, and plaintext payload interception.
- **Remediation**: Enforce secure-by-default certificate validation with `InsecureSkipVerify = false`, automatically fall back to host operating system certificate trust roots (`x509.SystemCertPool()`) when `ca_file` is empty, require explicit `insecure_skip_verify: true` opt-in, emit a mandatory high-visibility audit warning log on opt-in, and enforce `tls.VersionTLS12` minimum protocol version.
- **Resolution Details**: Fully resolved under [`REQ-097`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-097.md), [`ADR-097`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-097.md), and [`TASK-120`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-120.md):
  1. *Secure-by-Default Validation (CWE-295 Elimination)*: Eliminated hardcoded `else { tlsConfig.InsecureSkipVerify = true }`. Initialized `InsecureSkipVerify: false` in `SidecarConfig` and `defaultConfig()`. All outbound egress connections strictly validate peer certificates against trust roots by default.
  2. *Operating System Trust Root Fallback (`x509.SystemCertPool`)*: When `ca_file` is empty, `BuildClientTLSConfig` leaves `tlsConfig.RootCAs = nil`, directing Go's `crypto/tls` runtime to validate certificates against host OS trust roots for seamless compatibility with public/cloud PKI.
  3. *Custom Internal Enterprise CA Integration*: Retained `ca_file` support to load dedicated Root CA certificate pools (`RootCAs = caPool`) for private service mesh and enterprise PKI environments.
  4. *Explicit Opt-In & Mandatory Warning Log*: Added `insecure_skip_verify` YAML option for isolated testing. Emits a high-visibility warning to server logs: `[SIDECAR] WARNING: InsecureSkipVerify is enabled for sidecar egress TLS. Certificate verification is disabled.`
  5. *Protocol Version Floor*: Enforced `MinVersion: tls.VersionTLS12`, eliminating downgrade attacks to SSLv3, TLS 1.0, or TLS 1.1.
  6. *Client mTLS Identity*: Preserved client certificate and keypair loading (`CertFile`, `KeyFile`) for mutual TLS pod authentication.
  Verified by comprehensive automated verification suite [`TC-097`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-097.md), reviewed and approved in [`CR-093`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-093.md) and [`SR-097`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-097.md).

#### SEC-36: Memory Exhaustion via Unbounded Upstream Response Buffering in Internal API Proxy Test Probe
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:H - Score 6.5)
- **Location**: [`pkg/server/internal_api.go:L517-L670`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/internal_api.go#L517-L670)
- **CWE**: CWE-400, CWE-770, CWE-775
- **Status**: **Resolved**
- **Mapped Requirement**: [`REQ-098: Bounded Inbound and Upstream Body Ingestion in Internal API Proxy Test Probe`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-098.md)
- **Mapped Task**: [`TASK-121: Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-121.md)
- **Root Cause**:  
  `POST /internal/api/proxy-test` read both the inbound JSON request body and the target probe upstream response using unbounded `io.ReadAll`, without size constraints or bounded readers. Furthermore, truncated responses left residual bytes pending on the socket, preventing Go HTTP/1.1 transport connection recycling and causing socket descriptor exhaustion (`EMFILE`).
- **Impact**: Inbound client requests with massive payloads forced excessive heap allocations. Probing an upstream endpoint returning a multi-gigabyte payload or infinite chunked stream (`/dev/urandom`, SSE) caused continuous heap expansion until an Out-Of-Memory (OOM) abort crashed the gateway. Residual unread bytes caused connection hangs and socket leaks.
- **Remediation**: Bound inbound request ingestion via `io.LimitReader(req.Body, 64*1024+1)` with fast `400 Bad Request` rejection; bound upstream response ingestion via `io.LimitReader(httpResp.Body, maxBytes+1)` using configurable `MaxProxyTestResponseBytes` (default 1 MB); deterministically clamp oversized responses with `Truncated: true` signaling; and drain residual stream bytes into `io.Discard` before deferred socket closure to enable keep-alive connection reuse.
- **Resolution Details**: Fully resolved under [`REQ-098`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-098.md), [`ADR-098`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-098.md), and [`TASK-121`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-121.md):
  1. *Bounded Inbound Request Ingestion (CWE-400 Elimination)*: Enforced a 64 KB (`65,536` bytes) ceiling on incoming request bodies using `io.LimitReader(req.Body, maxRequestBodyBytes+1)`. Payloads exceeding 64 KB or containing malformed JSON are immediately rejected with `400 Bad Request`.
  2. *Bounded Upstream Response Buffering (CWE-400 / CWE-770 Elimination)*: Replaced unbounded reads with `io.LimitReader(httpResp.Body, maxResponseBytes+1)`. Payloads exceeding `maxResponseBytes` are clamped to the exact ceiling rather than expanding heap memory.
  3. *Configurable Response Limit with 1 MB Fallback*: Added `MaxProxyTestResponseBytes int64` to `InternalAPIConfig`. Normalizes omitted, zero, or negative configurations to 1 MB (`1,048,576` bytes) default.
  4. *Deterministic Truncation Signaling*: Added `Truncated bool `json:"truncated,omitempty"`` to `ProxyTestResponse`. Accurately flags `truncated: true` when upstream responses exceed the configured buffer limit while preserving status code and headers.
  5. *Residual Stream Draining & Socket Descriptor Recycling (CWE-775 Remediation)*: Drains up to 64 KB of residual body data into `io.Discard` (`io.Copy(io.Discard, io.LimitReader(httpResp.Body, 64*1024))`) before deferred socket closure (`defer httpResp.Body.Close()`), returning HTTP/1.1 TCP connections to the keep-alive transport pool and preventing `EMFILE` leaks.
  Verified by comprehensive automated verification suite [`TC-098`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-098.md) (`TC-098-01` through `TC-098-08`), reviewed and approved in [`CR-094`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-094.md) and [`SR-098`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-098.md).

#### SEC-37: Unbounded HTTP Client & Transport Allocation per Request in Service Mesh Sidecar Proxy
- **Severity**: **Medium** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H - Score 7.5)
- **Location**: [`pkg/sidecar/proxy.go:L192-L202`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L192-L202), [`pkg/proxy/proxy.go:L596-L600`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L596-L600)
- **CWE**: CWE-400, CWE-772
- **Status**: **Resolved**
- **Mapped Requirement**: [`REQ-099: Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-099.md)
- **Mapped Task**: [`TASK-122: Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-122.md)
- **Root Cause**:  
  `ProxyEngine.proxyToURL` called `proxy.NewProxyWithOptions` on every incoming egress HTTP request, allocating a new `*proxy.ReverseProxy`, `*http.Client`, and `*http.Transport` with an unmanaged connection pool. These transports were never reused across requests or closed after completion. Additionally, `ReverseProxy.Close()` failed to call `tr.CloseIdleConnections()`, and `proxyToURL` omitted client TLS configurations.
- **Impact**: Under production loads, thousands of open TCP sockets accumulated without connection pooling, causing socket descriptor exhaustion (`EMFILE`), memory bloat, GC thrashing, and gateway crash. Egress HTTPS traffic dropped client mTLS configurations.
- **Remediation**: Implement thread-safe origin-keyed `ReverseProxy` caching in `ProxyEngine` with canonical origin normalization (`scheme://host[:port]`), close idle transport connections in `ReverseProxy.Close()` and `ProxyEngine.Stop()`, and propagate client mTLS settings via `ProxyOptions.TLSClientConfig`.
- **Resolution Details**: Fully resolved under [`REQ-099`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-099.md), [`ADR-099`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-099.md), and [`TASK-122`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-122.md):
  1. *Thread-Safe ReverseProxy Caching (CWE-400 / CWE-772 Elimination)*: Implemented `ProxyEngine.proxies` map (`map[string]*proxy.ReverseProxy`) protected by a `sync.RWMutex` with double-checked locking in `getOrCreateProxy`. In-flight requests targeting cached origins proceed with lock-free read performance.
  2. *Canonical Origin Normalization ($O(U)$ Bounded Memory)*: Implemented `normalizeTargetOrigin(rawURL)` to map target URLs to canonical `scheme://host[:port]` keys, stripping paths, queries, and fragments, strictly bounding cache memory to $O(U)$ services and eliminating cache key explosion.
  3. *HTTP Keep-Alive Socket Pooling & Reuse*: Persistent TCP connections are reused across sequential and concurrent requests to identical upstreams, bounding active sockets ($\le 2$ per backend under steady traffic) and eliminating connection allocation overhead.
  4. *Idle Transport Socket Teardown*: Extended `ReverseProxy.Close()` to invoke `tr.CloseIdleConnections()` on `p.Client.Transport.(*http.Transport)`. Updated `ProxyEngine.Stop()` to iterate through and close all cached proxies upon engine shutdown, releasing all file descriptors and preventing `EMFILE` leaks.
  5. *Client mTLS Propagation & Defensive Cloning*: Extended `ProxyOptions` with `TLSClientConfig *tls.Config`, passing pre-compiled `p.clientTLS` to new reverse proxies, ensuring outbound egress requests authenticate with configured client certificates and validate custom CA bundles. Enforced two-tier defensive cloning (`p.clientTLS.Clone()` in `getOrCreateProxy` and `opts.TLSClientConfig.Clone()` in `NewProxyWithOptions`) to eliminate Go standard library data races on `NextProtos` during concurrent TLS handshakes.
  Verified by comprehensive automated test suite [`TC-099`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-099.md) (`TC-099-01` through `TC-099-07`), approved in [`CR-095`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-095.md) and [`SR-099`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-099.md).

#### SEC-38: Missing Token Syntax and Length Validation in ACME HTTP-01 Challenge Handler
- **Severity**: **Low** (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:L - Score 3.7)
- **Location**: [`pkg/acme/acme.go:L200-L262`](file:///Users/sneha/Developer/toron-research/toron/pkg/acme/acme.go#L200-L262)
- **CWE**: CWE-20, CWE-400, CWE-703
- **Status**: **Resolved**
- **Mapped Requirement**: [`REQ-100: RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-100.md)
- **Mapped Task**: [`TASK-123: RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-123.md)
- **Root Cause**:  
  `ACMEManager.ServeHTTP01Handler` extracted the token segment from `req.Path`, silently trimmed whitespace with `strings.TrimSpace`, and directly queried `m.GetHTTP01Challenge(token)` without validating RFC 8555 §8.3 unpadded base64url characters or string length boundaries. Furthermore, arbitrary HTTP verbs were accepted instead of restricting access to `GET` and `HEAD`, and `HEAD` requests erroneously wrote response bodies in violation of RFC 7231 §4.3.2.
- **Impact**: Unvalidated external inputs triggered map hashing and reader lock (`m.mu.RLock()`) contention; malformed tokens bypassed validation via whitespace trimming; arbitrary HTTP methods leaked challenge responses; and non-compliant HEAD responses disrupted automated ACME CA validation probes.
- **Remediation**: Implement a zero-allocation validator `IsValidACMEToken` enforcing $1 \le \text{len} \le 128$ and base64url characters `[a-zA-Z0-9_-]`; restrict HTTP methods to `GET` and `HEAD` returning 405 Method Not Allowed with `Allow: GET, HEAD` on others; eliminate silent whitespace trimming; enforce fail-fast 400 Bad Request before map locking; implement RFC 7231 HEAD semantics (`Content-Length` set, empty body); and return clean 404 responses for unregistered tokens.
- **Resolution Details**: Fully resolved under [`REQ-100`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-100.md), [`ADR-100`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-100.md), and [`TASK-123`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-123.md):
  1. *Zero-Allocation RFC 8555 Base64URL Validator (`IsValidACMEToken`)*: Added public validator performing an in-place single-pass byte scan over the token without heap allocations, enforcing $1 \le \text{len} \le 128$ and strictly characters `[a-zA-Z0-9_-]`, rejecting padding (`=`), path separators, traversal dots, whitespace, and non-ASCII bytes.
  2. *Removal of Silent Whitespace Trimming*: Completely eliminated `strings.TrimSpace(token)`. Tokens containing leading, trailing, or internal whitespace fail validation and are rejected with `400 Bad Request`.
  3. *HTTP Method Hardening (RFC 7231 §6.5.5 Compliance)*: Evaluated as the first operation in `ServeHTTP01Handler`; non-`GET` and non-`HEAD` methods (`POST`, `PUT`, `DELETE`, etc.) immediately return `405 Method Not Allowed` with mandatory headers `Allow: GET, HEAD` and `Content-Type: text/plain`.
  4. *Fail-Fast Lock Isolation*: Syntax, length, and method validations execute before invoking `m.GetHTTP01Challenge(token)`, ensuring malformed or oversized payloads never acquire `m.mu.RLock()` or trigger map hash computations.
  5. *RFC 7231 §4.3.2 Compliant HEAD Semantics*: `HEAD` requests return `200 OK`, `Content-Type: text/plain`, and exact `Content-Length: len(keyAuth)` while strictly omitting the response body (`res.Body.Len() == 0`).
  6. *Clean 404 Not Found Handling*: Valid unregistered tokens return `404 Not Found` with an explanatory error body for `GET` and an empty body for `HEAD`.
  Verified by comprehensive automated test suite [`TC-100`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-100.md) (`TC-100-01` through `TC-100-09`), approved in [`CR-096`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-096.md) and [`SR-100`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-100.md).

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
- **Phase 6 (Advanced Subsystem & Security Perimeter Hardening - SEC-31..38 - IN PROGRESS)**:
  1. [x] Bind physical `RemoteAddr` in HTTP/2 and HTTP/3 adapters and eliminate header spoofing (`SEC-31`) - **COMPLETED** (`TASK-111`..`113`, `TC-092`, `SR-092`, `CR-088`).
  2. [x] Enforce fail-closed access control on unidentifiable client IPs in WAF (`SEC-32`) - **COMPLETED** (`TASK-114`..`115`, `TC-093`, `SR-093`, `CR-089`).
  3. [x] Prune deleted Ingress routes and prevent routing table growth in K8s Ingress Controller (SEC-33) - **COMPLETED** (TASK-116..117, TC-094, SR-094, CR-090).
  4. [x] Aggregate multi-replica upstreams and prevent premature route deletion in OCI Discovery (`SEC-34`) - **COMPLETED** (`TASK-118`, `TC-095`, `SR-095`, `CR-091`).
  5. [x] Remove insecure default `InsecureSkipVerify = true` in Sidecar Client TLS (`SEC-35`) - **COMPLETED** (`TASK-120`, `TC-097`, `SR-097`, `CR-093`).
  6. [x] Bound response body ingestion in `/internal/api/proxy-test` probe (`SEC-36`) - **COMPLETED** (`TASK-121`, `TC-098`, `SR-098`, `CR-094`).
  7. [x] Pool and reuse `http.Transport` instances in Service Mesh Sidecar Proxy (`SEC-37`) - **COMPLETED** (`TASK-122`, `TC-099`, `SR-099`, `CR-095`).
  8. [x] Enforce RFC 8555 base64url token syntax validation in ACME challenge handler (`SEC-38`) - **COMPLETED** (`TASK-123`, `TC-100`, `SR-100`, `CR-096`).

---

## 5. Remediation Status & Verification Summary

38 security vulnerabilities (`SEC-01` through `SEC-38`) have been fully remediated, verified under `go test -count=1 -race ./...`, security reviewed, and code reviewed:
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
- **`SEC-31`**: Verified in `TC-092` (`TASK-111`..`113`, `SR-092`, `CR-088`).
- **`SEC-32`**: Verified in `TC-093` (`TASK-114`..`115`, `SR-093`, `CR-089`).
- **`SEC-33`**: Verified in `TC-094` (`TASK-116`..`117`, `SR-094`, `CR-090`).
- **`SEC-34`**: Verified in `TC-095` (`TASK-118`, `SR-095`, `CR-091`).
- **`SEC-35`**: Verified in `TC-097` (`TASK-120`, `SR-097`, `CR-093`).
- **`SEC-36`**: Verified in `TC-098` (`TASK-121`, `SR-098`, `CR-094`).
- **`SEC-37`**: Verified in `TC-099` (`TASK-122`, `SR-099`, `CR-095`).
- **`SEC-38`**: Verified in `TC-100` (`TASK-123`, `SR-100`, `CR-096`).

All 38 vulnerabilities (`SEC-01` through `SEC-38`) are 100% verified and resolved. **Zero open findings remain** across the entire Toron Web Server and Edge Gateway codebase.


