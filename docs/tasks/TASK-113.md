---
id: TASK-113
type: task
title: Comprehensive Automated Verification Suite for Anti-Spoofing & Protocol Parity (TC-092)
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-10
updated: 2026-09-10

depends_on:
  - REQ-092
  - TASK-111
  - TASK-112

owns:
  - pkg/server/server_anti_spoofing_test.go
  - pkg/httpparser/request_test.go
  - pkg/waf/ip_acl_test.go
  - pkg/router/rate_limiter_test.go
  - pkg/proxy/proxy_test.go
  - pkg/proxy/sticky_test.go

references:
  - REQ-092
  - SEC-31
  - SR-091
  - ADR-087
  - TC-092
  - REQ-001
  - REQ-019
  - REQ-030
  - REQ-041
  - REQ-042
  - REQ-071

derived_from:
  - REQ-092
  - SEC-31
  - SR-091

implements:
  - REQ-092

verified_by:
  - TC-092

decided_by:
  - ADR-087

related_to:
  - SEC-31
  - SR-091
  - REQ-071
  - REQ-030
---

# TASK-113 - Comprehensive Automated Verification Suite for Anti-Spoofing & Protocol Parity (TC-092)

## Description

Implement a comprehensive automated verification test suite validating test specification [`TC-092`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-092.md). Ensure physical connection remote address binding, forwarded header sanitization, trusted proxy gating, and protocol parity across HTTP/1.1, HTTP/2, and HTTP/3 QUIC, verifying complete remediation of vulnerability [`SEC-31`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L428-L436) and audit finding [`SR-091 Finding 1`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L78-L99).

## Scope & Implementation Breakdown

1. **Request RemoteAddr & Resolution Helpers Test Suite (`pkg/httpparser/request_test.go`)**:
   - `TestRequest_RemoteHostAndIP_Parsing`:
     - Test IPv4 with port (`"192.0.2.1:50000"` -> host `"192.0.2.1"`, valid IP).
     - Test IPv4 without port (`"192.0.2.1"` -> host `"192.0.2.1"`, valid IP).
     - Test IPv6 with port (`"[2001:db8::1]:8443"` -> host `"2001:db8::1"`, valid IP).
     - Test IPv6 without port (`"2001:db8::1"` -> host `"2001:db8::1"`, valid IP).
     - Test malformed addresses, unparseable strings, and empty strings.
     - Test fallback to `req.RawConn.RemoteAddr()` when `RemoteAddr` is empty.
   - `TestRequest_NewRequestFromStd_RemoteAddr`:
     - Verify `NewRequestFromStd(r)` preserves `r.RemoteAddr` into `req.RemoteAddr`.

2. **WAF IP ACL Anti-Spoofing & Protocol Parity Tests (`pkg/server/server_anti_spoofing_test.go` / `pkg/waf/ip_acl_test.go`)**:
   - `TestServer_H2_WAF_SpoofingRejected`:
     - Configure WAF IP ACL blacklist blocking client physical IP (`127.0.0.1` or mock peer).
     - Send HTTP/2 requests with spoofed `X-Forwarded-For: 198.51.100.1` and `X-Real-IP: 198.51.100.1`.
     - Assert that WAF blocks the request with `403 Forbidden`, discarding spoofed headers.
     - Verify parity across native HTTP/1.1 and HTTP/2.

3. **Internal Management API Subnet Enforcement Tests (`pkg/server/server_anti_spoofing_test.go`)**:
   - `TestServer_H2_InternalAPI_SubnetEnforcement`:
     - Configure internal management API restricted to admin subnet `10.50.0.0/16`.
     - Send HTTP/2 requests from non-admin peer address (`127.0.0.1` / loopback) with `X-Forwarded-For: 10.50.0.5`.
     - Assert that access is denied with `403 Forbidden` (`{"error":"403 Forbidden","message":"Access denied by administrative subnet policy"}`).
     - Verify that when connecting from a verified `trusted_proxies` IP, legitimate forwarded headers are evaluated.

4. **Token Bucket Rate Limiter Anti-Spoofing Tests (`pkg/router/rate_limiter_test.go`)**:
   - `TestServer_H2_RateLimiter_AntiSpoofing`:
     - Configure route with token bucket limit of `5 req/sec`.
     - Issue 10 rapid HTTP/2 requests from the same client, each with a different forged `X-Forwarded-For: 10.0.0.X` or `X-API-Key: user_X`.
     - Assert that because the physical remote address is untrusted, all 10 requests map to `ip:<remoteHost>`, exhausting the bucket and triggering `429 Too Many Requests`.
     - Verify that when peer IP is in `TrustedProxies`, distinct `X-Forwarded-For` IPs receive distinct buckets.

5. **Reverse Proxy Header Forwarding & Sanitization Tests (`pkg/proxy/proxy_test.go`)**:
   - `TestServer_H2_ReverseProxy_HeaderSanitization`:
     - Send HTTP/2 request with forged `X-Forwarded-For: 203.0.113.195` and `X-Real-IP: 203.0.113.195` to reverse proxy without trusted proxies configured.
     - Verify backend receives `X-Forwarded-For` and `X-Real-IP` set strictly to client physical `RemoteAddr` IP; spoofed client headers are removed.
   - `TestServer_H2_ReverseProxy_TrustedProxyAppended`:
     - Configure reverse proxy with client IP in `TrustedProxies: ["127.0.0.1/32"]`.
     - Send HTTP/2 request with `X-Forwarded-For: 203.0.113.195`.
     - Verify backend receives `X-Forwarded-For: 203.0.113.195, 127.0.0.1`.

6. **HTTP/3 Protocol Ingress Parity Tests (`pkg/server/server_anti_spoofing_test.go`)**:
   - `TestServer_H3_RemoteAddrBinding`:
     - Verify HTTP/3 requests processed through `http2AdapterHandler` / QUIC have physical UDP `RemoteAddr` bound to `req.RemoteAddr`.
     - Assert that spoofed forwarded headers cannot bypass WAF or rate limiting over HTTP/3.

7. **Sticky Session Mobile Roaming Non-Regression Tests (`pkg/proxy/sticky_test.go`)**:
   - `TestSticky_PreserveMobileRoamingAffinity`:
     - Verify that `sticky.go` continues to evaluate `X-Forwarded-For` for IP-hash session balancing without regression, proving non-goals and [`REQ-030`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-030.md) fidelity are preserved.

8. **Race-Clean Execution & Concurrency Invariants**:
   - Execute all test suites under `go test -v -race ./pkg/httpparser/... ./pkg/server/... ./pkg/waf/... ./pkg/router/... ./pkg/proxy/...`.
   - Verify zero data races, memory leaks, or deadlocks under high concurrency.

## Acceptance Criteria

- All test cases defined in [`TC-092`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-092.md) implemented and passing cleanly.
- HTTP/2 and HTTP/3 anti-spoofing tests prove WAF, internal API, rate limiter, and reverse proxy reject spoofed headers from untrusted connections.
- Sticky session load balancer regression test proves mobile roaming session preservation under [`REQ-030`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-030.md) remains unaffected.
- 100% race-clean under `go test -race ./...`.
- Pure Go standard library (zero external dependencies).

## Rationale

Thorough multi-protocol integration testing guarantees that client IP spoofing vulnerability `SEC-31` is completely eliminated across all entry points, while preventing regression of existing routing and session affinity features.

## Constraints

- Pure Go standard library packages (`testing`, `net`, `net/http`, `time`, `sync`). No third-party test assertions or mock frameworks.
- Tests must execute quickly in CI without flaky race conditions or fixed sleep dependencies.

## Open Questions

- None. Test harnesses can use standard `httptest.Server` with HTTP/2 enabled, or direct invocation of `http2AdapterHandler` and `Router.ServeHTTP` with mock `http.Request` objects with configured `RemoteAddr`.
