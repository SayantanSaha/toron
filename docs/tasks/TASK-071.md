---
id: TASK-071
type: task
title: Implement Connection State Integrity and Hop-by-Hop Stripping for Forwarded Ingress Headers
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-071
implements:
  - REQ-071
verified_by:
  - TC-071
decided_by:
  - ADR-066
related_to:
  - TASK-019
---

# TASK-071 - Implement Connection State Integrity and Hop-by-Hop Stripping for Forwarded Ingress Headers

## Description

Implement verified connection state inspection for `X-Forwarded-Proto` and `X-Forwarded-For` headers in `pkg/proxy/proxy.go`, introduce trusted proxy validation, and enforce RFC 7230 §6.1 hop-by-hop header stripping to prevent header spoofing and protocol confusion attacks.

## Scope & Implementation Breakdown

1. **Protocol & Remote Address Verification (`pkg/proxy/proxy.go`)**:
   - Inspect `req.RawConn` to determine whether the incoming connection is TLS-encrypted. Set `X-Forwarded-Proto` to `"https"` strictly if TLS is active; otherwise `"http"`.
   - Disregard client-supplied `X-Forwarded-Proto` unless the request originates from an IP within the configured `trusted_proxies` list.
   - Derive client IP for `X-Forwarded-For` from `req.RawConn.RemoteAddr()`. If `X-Forwarded-For` already exists:
     - If upstream peer IP is in `trusted_proxies`, append client IP.
     - If untrusted, overwrite `X-Forwarded-For` with the verified peer IP.

2. **Hop-by-Hop Header Stripping (`pkg/proxy/proxy.go`)**:
   - Strip standard hop-by-hop headers before dispatching upstream: `Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailers`, `Upgrade` (unless valid WebSocket handshake).
   - Strip any custom headers listed in the incoming `Connection` header token list per RFC 7230 §6.1.

3. **Testing Verification (`pkg/proxy/proxy_test.go`)**:
   - Add unit test verifying that a cleartext HTTP request with `X-Forwarded-Proto: https` from an untrusted client is rewritten to `X-Forwarded-Proto: http`.
   - Add test verifying that client IP is correctly populated from `RemoteAddr` and hop-by-hop headers are stripped.

## Acceptance Criteria

- `X-Forwarded-Proto` reflects physical socket TLS status for untrusted clients.
- `X-Forwarded-For` is anchored by the physical TCP socket IP address.
- Hop-by-hop headers are removed before forwarding to upstreams.
- Proxy forwarding tests pass with `go test ./pkg/proxy/...`.

## Rationale

Allowing untrusted clients to set `X-Forwarded-Proto: https` allows attackers to bypass backend HTTPS checks, while spoofed `X-Forwarded-For` headers corrupt backend rate limiting, access control, and forensic logging.

## Constraints

- Compliance with RFC 7230 §6.1.
- High-efficiency header processing without extraneous allocations.

## Open Questions

- None.
