---
id: TASK-070
type: task
title: Implement Host Header Validation in Cleartext HTTP-to-HTTPS Redirection
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-070
implements:
  - REQ-070
verified_by:
  - TC-070
decided_by:
  - ADR-065
related_to:
  - TASK-057
---

# TASK-070 - Implement Host Header Validation in Cleartext HTTP-to-HTTPS Redirection

## Description

Implement domain validation for incoming `Host` headers in `pkg/server/server.go` (`serveHTTPRedirect`), ensuring HTTP-to-HTTPS redirection only reflects recognized hostnames registered with SNI or route configurations, preventing Open Redirect vulnerabilities (CWE-601).

## Scope & Implementation Breakdown

1. **Host Recognition Registry (`pkg/server/server.go`)**:
   - In `Server`, provide a method or lookup function (e.g. `isRecognizedHost(host string) bool`) that checks:
     a. Configured server primary hostname / domain,
     b. Registered domains in `s.sniManager`,
     c. Explicitly configured allowed redirect hosts in `HTTPRedirectConfig`.
   - If no explicit whitelist is configured, check against known SNI certificate hostnames or configured route hosts.

2. **Redirect Validation Guard (`pkg/server/server.go`)**:
   - In `serveHTTPRedirect`:
     - After parsing and sanitizing `host` (existing character and port stripping checks), verify `isRecognizedHost(host)`.
     - If the host is not recognized:
       - If a `DefaultHost` is configured in `HTTPRedirectConfig`, redirect to `DefaultHost`.
       - Otherwise, return `http.StatusBadRequest` (`400 Bad Request: Unrecognized Host`).
     - Never emit a `Location: https://<unrecognized-host>` header.

3. **Testing Verification (`pkg/server/server_test.go`)**:
   - Add test verifying that requesting an unrecognized domain (e.g. `Host: attacker.com`) on the redirect port returns `400 Bad Request` (or redirects to canonical host) and does NOT redirect to `attacker.com`.
   - Add test verifying that recognized domains redirect normally with `301 Moved Permanently`.

## Acceptance Criteria

- Cleartext HTTP redirects reject or sanitize unconfigured client `Host` headers.
- Unrecognized hostnames are never reflected in redirect `Location` headers.
- Recognized SNI domains and route hosts continue to redirect seamlessly.
- Server redirect tests pass with `go test ./pkg/server/...`.

## Rationale

Edge gateways that blindly reflect incoming `Host` headers into redirect locations allow attackers to craft phishing links on the gateway's IP address that redirect victims to external malicious sites.

## Constraints

- Retain existing CRLF and response splitting sanitization.
- Support standard port stripping via `net.SplitHostPort`.

## Open Questions

- None.
