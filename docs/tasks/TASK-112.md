---
id: TASK-112
type: task
title: Security Perimeter Hardening & Trusted Proxy Gating across WAF, Internal API, Rate Limiter, Reverse Proxy, and Logging
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-10
updated: 2026-09-10

depends_on:
  - REQ-092
  - TASK-111

owns:
  - pkg/waf/ip_acl.go
  - pkg/server/internal_api.go
  - pkg/router/rate_limiter.go
  - pkg/proxy/proxy.go
  - pkg/logging/manager.go

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

# TASK-112 - Security Perimeter Hardening & Trusted Proxy Gating across WAF, Internal API, Rate Limiter, Reverse Proxy, and Logging

## Description

Harden all security-critical modules across Toron's perimeter against IP spoofing attacks by anchoring client network identity to the physical remote address established in [`TASK-111`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-111.md). Eliminate insecure header fallbacks across WAF IP ACL, Internal Management API subnet validation, Token Bucket rate limiting, Reverse Proxy header forwarding, and Structured Access Logging. Ensure forwarded headers (`X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Proto`) are strictly gated behind verified `trusted_proxies` CIDR configurations, resolving [`SEC-31`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L428-L436) and [`SR-091`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L78-L99).

> [!CAUTION]
> **Explicit Non-Goal & Scope Exclusion**: Under NO circumstances should [`pkg/proxy/sticky.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/sticky.go) be modified. Application-level sticky session load balancing relies on header inspection to preserve affinity across mobile carrier roaming pursuant to [`REQ-030`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-030.md) and is explicitly excluded from [`REQ-092`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-092.md).

## Scope & Implementation Breakdown

1. **WAF IP Access Control Hardening (`pkg/waf/ip_acl.go`)**:
   - In [`pkg/waf/ip_acl.go:ExtractClientIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L112-L164):
     - Extract client IP from physical network connection metadata first (`req.RemoteIP()`, `req.RemoteAddr`, `req.RawConn`).
     - Client-supplied `X-Forwarded-For` and `X-Real-IP` headers MUST NOT be trusted or evaluated when a physical remote address is present, unless the peer IP is explicitly verified against configured `trusted_proxies`.
     - In synthetic unit tests where physical address metadata is absent (`req.RemoteAddr == ""` and `req.RawConn == nil`), safely fall back to forwarded headers for testing harnesses only.
     - Fail-secure: If the client IP cannot be determined and an IP allowlist is enforced, deny access by default.

2. **Internal Management API Subnet Gate Hardening (`pkg/server/internal_api.go`)**:
   - In [`pkg/server/internal_api.go:wrapHandler`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/internal_api.go#L194-L224):
     - Eliminate unverified fallback to `X-Forwarded-For` when `req.RawConn == nil`.
     - Inspect `req.RemoteIP()` / `req.RemoteAddr` as primary client IP.
     - Only inspect `X-Forwarded-For` if the physical peer IP matches a configured trusted proxy subnet.
     - Fail-closed: If client IP cannot be extracted or does not fall within `parsedSubnets`, reject immediately with `403 Forbidden` (`{"error":"403 Forbidden","message":"Access denied by administrative subnet policy"}`).

3. **Token Bucket Rate Limiting Anti-Spoofing (`pkg/router/rate_limiter.go`)**:
   - In [`pkg/router/rate_limiter.go:extractClientKeyInternal`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/rate_limiter.go#L286-L325):
     - Remove the insecure condition `peerIsTrusted || req.RawConn == nil` (line 314).
     - Resolve `socketIP` and `socketHost` by inspecting `req.RemoteIP()` / `req.RemoteAddr` alongside `req.RawConn`.
     - Check `socketIP` against `trustedProxies`.
     - If `peerIsTrusted == false`:
       - If physical remote address is present (`socketHost != ""`), anchor key strictly to `"ip:" + socketHost`.
       - Client-supplied `X-Forwarded-For`, `X-API-Key`, and `Authorization` headers MUST be ignored for rate limit bucket keying.
     - If `peerIsTrusted == true`:
       - Utilize `X-Forwarded-For`, `X-API-Key`, or `Authorization` headers as requested.
     - Synthetic test requests without physical connection info must fall back safely without enabling evasion.

4. **Reverse Proxy Header Forwarding & Sanitization (`pkg/proxy/proxy.go`)**:
   - In [`pkg/proxy/proxy.go:ServeHTTPWithPrefix`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L816-L857):
     - Resolve `peerIP` and `isTrusted` by inspecting `req.RemoteAddr` as well as `req.RawConn.RemoteAddr()`.
     - Support address strings in `isPeerTrusted` or parse host from `req.RemoteAddr`.
     - When `isTrusted == false` (or `peerIP` is untrusted):
       - Outgoing upstream `X-Forwarded-For` MUST be set strictly to `peerIP`.
       - Outgoing upstream `X-Real-IP` MUST be set strictly to `peerIP`.
       - Client-supplied `X-Forwarded-For` and `X-Real-IP` headers MUST be stripped and discarded.
     - When `isTrusted == true`:
       - If incoming `X-Forwarded-For` exists, append `peerIP` (`existingXFF + ", " + peerIP`).
       - If incoming `X-Real-IP` exists, preserve it.
     - Ensure protocol derivation for HTTP/2 and HTTP/3 sets `X-Forwarded-Proto: https`.

5. **Structured Access Logging Hardening (`pkg/logging/manager.go`)**:
   - In [`pkg/logging/manager.go:ExtractClientIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/logging/manager.go#L498-L531):
     - Update extraction logic to check `req.RemoteHost()` / `req.RemoteAddr` and `req.RawConn` first.
     - Ensure physical client IP is returned whenever present, preventing spoofed `X-Forwarded-For` or `X-Real-IP` headers from polluting audit logs for untrusted requests.

6. **Preservation of Non-Goal Boundaries (`pkg/proxy/sticky.go`)**:
   - Maintain [`pkg/proxy/sticky.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/sticky.go) untouched to preserve mobile roaming session affinity under [`REQ-030`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-030.md).

## Acceptance Criteria

- Untrusted HTTP/2 and HTTP/3 requests cannot bypass WAF IP blacklist or circumvent IP allowlist by injecting `X-Forwarded-For` or `X-Real-IP`.
- Internal Management API rejects untrusted HTTP/2 and HTTP/3 requests attempting subnet bypass with `403 Forbidden`.
- Token bucket rate limiter groups untrusted HTTP/2 and HTTP/3 requests into a single physical bucket (`ip:<remoteHost>`), ignoring header rotation.
- Reverse proxy strips untrusted forwarded headers from HTTP/2 and HTTP/3 requests, setting upstream `X-Forwarded-For` and `X-Real-IP` strictly to verified physical `peerIP`.
- Reverse proxy properly appends `peerIP` to `X-Forwarded-For` when arriving from verified `trusted_proxies`.
- Structured access logs reflect true physical client IP for untrusted connections.
- [`pkg/proxy/sticky.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/sticky.go) remains unmodified with zero diffs.
- 100% pure Go standard library (zero third-party dependencies).
- Concurrency-safe and clean under `go test -race ./...`.

## Rationale

Allowing unauthenticated clients on multiplexed protocols to inject arbitrary IP headers allows attackers to spoof trusted networks, bypass WAF blocks, defeat rate limiting, and pollute audit trails. Grounding security enforcement in the physical remote connection restores the integrity of Toron's security perimeter.

## Constraints

- Pure Go standard library packages (`net`, `net/http`, `strings`, `sync`). No external dependencies.
- Zero modifications to [`pkg/proxy/sticky.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/sticky.go).
- Fail-closed security default: unresolvable addresses deny administrative access and fall back safely in WAF.

## Open Questions

- None. Configuration schemas for `trusted_proxies` are already established in [`pkg/config`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L246).
