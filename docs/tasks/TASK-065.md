---
id: TASK-065
type: task
title: Enforce Upstream TLS Certificate Verification in WebSocket Reverse Proxy
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-065
implements:
  - REQ-065
verified_by:
  - TC-065
decided_by:
  - ADR-060
related_to:
  - TASK-026
---

# TASK-065 - Enforce Upstream TLS Certificate Verification in WebSocket Reverse Proxy

## Description

Eliminate hardcoded `InsecureSkipVerify: true` in `pkg/proxy/proxy.go` when proxying WebSocket connections over TLS, enforcing standard CA certificate verification and preventing Man-in-the-Middle (MitM) attacks.

## Scope & Implementation Breakdown

1. **Remove Hardcoded InsecureSkipVerify (`pkg/proxy/proxy.go`)**:
   - In `serveWebSocketProxy`, replace `&tls.Config{InsecureSkipVerify: true}` with a properly initialized `*tls.Config`.
   - Propagate the target hostname to `tlsConfig.ServerName` for correct SNI validation.

2. **Custom CA Support (`pkg/proxy/proxy.go`)**:
   - Check if `p.TLSCACertPool` or `ProxyOptions.TLS.CAFile` is specified; if so, populate `tlsConfig.RootCAs`.
   - Allow `InsecureSkipVerify` to be true ONLY if explicitly configured in route options for development testing.

3. **Testing Verification (`pkg/proxy/proxy_test.go`)**:
   - Add test verifying that WebSocket proxying fails when connecting to an upstream TLS service presenting a self-signed cert (without trusted CA configured).
   - Add test verifying successful handshake when the upstream certificate is signed by a configured trusted root CA.

## Acceptance Criteria

- Outbound WebSocket TLS connections validate certificates by default.
- Untrusted or invalid certificates are rejected with `502 Bad Gateway`.
- Route options allow specifying a custom CA bundle.
- Tests pass cleanly with `go test ./pkg/proxy/...`.

## Rationale

Bypassing TLS verification allows any network observer on the path between Toron and the upstream backend to intercept or modify WebSocket traffic.

## Constraints

- Seamless integration with Go `crypto/tls` standard library.

## Open Questions

- Should client certificates (mTLS) be supported on outbound WebSocket connections?
