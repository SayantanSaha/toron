---
id: TASK-105
type: task
title: Static & Dynamic Hop-by-Hop Header Sanitization in Transcoder
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-090

owns:
  - pkg/transcoder/transcoder.go

references:
  - REQ-090
  - SEC-29
  - SR-081
  - ADR-085
  - TC-090

derived_from:
  - REQ-090

implements:
  - REQ-090

verified_by:
  - TC-090
---

# TASK-105 - Static & Dynamic Hop-by-Hop Header Sanitization in Transcoder

## Overview

Implement static and dynamic hop-by-hop header sanitization in [`Engine.HandleTranscode`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L89) ([`pkg/transcoder/transcoder.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go)) to ensure outgoing HTTP/2 gRPC requests never transmit connection-specific headers prohibited by [RFC 7540 §8.1.2.2](https://datatracker.ietf.org/doc/html/rfc7540#section-8.1.2.2) and [RFC 7230 §6.1](https://datatracker.ietf.org/doc/html/rfc7230#section-6.1), mitigating [`SEC-29`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L402-L410).

## Scope & Implementation Breakdown

1. **Static Hop-by-Hop Discard Map**:
   - Define a static package-level lookup map of prohibited connection-specific and transport headers:
     `connection`, `keep-alive`, `proxy-authenticate`, `proxy-authorization`, `te`, `trailer`, `trailers`, `transfer-encoding`, `upgrade`, `proxy-connection`.
   - Also discard the `host` header to prevent conflicting with the gRPC client's `:authority`.

2. **Dynamic `Connection` Token Parsing**:
   - In [`HandleTranscode`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L89), inspect the incoming `Connection` header.
   - Parse all comma-separated tokens (trimming whitespace and lower-casing) into a dynamic discard set (`customHopByHop`).

3. **Sanitized Header Copy Loop**:
   - Iterate over `req.Header`.
   - Discard any header whose lower-cased name matches the static hop-by-hop map, the dynamic `Connection` tokens, or starts with `"content-"`.
   - Forward remaining valid headers to `grpcReq.Header`.

## Acceptance Criteria

- All standard RFC 7230 / RFC 7540 hop-by-hop headers are excluded from outgoing gRPC requests.
- Headers listed in `Connection` header values are dynamically excluded.
- The `Host` header is excluded.
- Existing `content-*` filtering is preserved.
