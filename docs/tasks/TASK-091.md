---
id: TASK-091
type: task
title: Immediate HTTP 413 Payload Too Large Rejection in Sidecar ProxyEngine
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-086
  - TASK-090

owns:
  - pkg/sidecar/proxy.go

references:
  - REQ-086
  - SEC-25

derived_from:
  - REQ-086

implements:
  - REQ-086

verified_by:
  - TC-086
---

# TASK-091 - Immediate HTTP 413 Payload Too Large Rejection in Sidecar ProxyEngine

## Overview

Eliminate silent body truncation and upstream data corruption in [`ProxyEngine.proxyToURL`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L189) within [`pkg/sidecar/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go). Enforce fast-path `Content-Length` validation and bounded stream over-read via `io.LimitReader(r.Body, maxBody+1)`. If an incoming payload exceeds `MaxBodyBytes`, immediately reject the request with HTTP `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`), close the body, emit diagnostic logs, and terminate proxying without forwarding corrupted data upstream, fixing [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374).

## Scope & Implementation Breakdown

1. **Fast-Path `Content-Length` Header Guard ([`pkg/sidecar/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go))**:
   - In [`ProxyEngine.proxyToURL`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L189), for mutating requests (`r.Body != nil` and `r.Method != "GET"` and `r.Method != "HEAD"`):
     - If `r.ContentLength > p.cfg.MaxBodyBytes`:
       - Immediately close `r.Body` (`_ = r.Body.Close()`).
       - Emit an informative diagnostic log entry detailing the rejected request path and configured limit.
       - Return HTTP 413 via `http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)`.
       - Abort proxy dispatch immediately.

2. **Stream Over-Read Guard for Chunked / Unspecified Payloads ([`pkg/sidecar/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go))**:
   - Bounded ingestion using `io.LimitReader(r.Body, p.cfg.MaxBodyBytes+1)`.
   - After reading `bodyBytes, err := io.ReadAll(...)`:
     - If `int64(len(bodyBytes)) > p.cfg.MaxBodyBytes`:
       - Close `r.Body`.
       - Emit diagnostic log entry.
       - Return HTTP 413 via `http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)`.
       - Terminate processing and return without calling upstream `px.ServeHTTP`.
     - If reading encounters an unexpected I/O error (`err != nil`), return HTTP 400 Bad Request or HTTP 500 without dispatching corrupted payloads.

3. **Byte-Fidelity Forwarding for In-Bounds Payloads**:
   - If payload size is `<= p.cfg.MaxBodyBytes`, populate `toronReq.Body = bytes.NewReader(bodyBytes)` and set `toronReq.ContentLength = int64(len(bodyBytes))`.

4. **Bypass for Non-Mutating and Empty Requests**:
   - `GET` and `HEAD` requests, or requests where `r.Body == nil` or body length is 0, must bypass body buffering and forward directly.

## Acceptance Criteria

- Requests with declared `Content-Length` exceeding `MaxBodyBytes` receive immediate HTTP 413 before reading the full body.
- Chunked or undeclared streams exceeding `MaxBodyBytes` are truncated from reading further, closed, and rejected with HTTP 413.
- No partial or corrupted request data is dispatched upstream to the target service.
- Valid requests under the limit are forwarded with exact byte fidelity.
- Zero third-party dependencies.

## Blockers & Risks

- Ensure incoming request body is closed cleanly to prevent socket descriptor or memory leaks.
- Ensure response headers are not written prior to error response writing.
