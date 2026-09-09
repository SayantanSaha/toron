---
id: TASK-102
type: task
title: Fast-Fail Rejection on Declared Content-Length in Transcoder
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-089
  - TASK-101

owns:
  - pkg/transcoder/transcoder.go

references:
  - REQ-089
  - SEC-28
  - SR-081
  - ADR-084
  - TC-089

derived_from:
  - REQ-089

implements:
  - REQ-089

verified_by:
  - TC-089
---

# TASK-102 - Fast-Fail Rejection on Declared Content-Length in Transcoder

## Overview

Implement an instantaneous pre-read fast-fail guard in [`Engine.HandleTranscode`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L89) based on the incoming request's declared `Content-Length`. When a client sends a request whose declared payload exceeds `maxBodyBytes`, the engine MUST reject the request immediately with `HTTP 413 Payload Too Large` without performing buffer allocations or reading the body stream, mitigating [`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401) (CWE-400/CWE-770).

## Scope & Implementation Breakdown

1. **Declared Content-Length Pre-Check ([`pkg/transcoder/transcoder.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go))**:
   - In [`HandleTranscode`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L89), prior to inspecting or reading `req.Body`:
     - Evaluate `req.ContentLength` (or `Content-Length` header if `req.ContentLength == 0`).
     - Check: `if req.ContentLength > maxBody && req.ContentLength > 0`.

2. **HTTP 413 Response Formatting & Early Return**:
   - Set status code `http.StatusRequestEntityTooLarge` (413).
   - Set header `Content-Type: application/json`.
   - Write structured JSON payload:
     `{"error": fmt.Sprintf("Payload Too Large: request Content-Length %d exceeds limit of %d bytes", req.ContentLength, maxBody)}`
   - If `req.Body` implements `io.Closer`, invoke `req.Body.Close()`.
   - Return immediately without attempting `io.ReadAll`, `json.Unmarshal`, or gRPC upstream dialing.

3. **Zero Upstream Leakage Guarantee**:
   - Ensure the upstream gRPC server is never contacted on fast-fail rejections.

## Acceptance Criteria

- Requests with declared `Content-Length > maxBodyBytes` receive `HTTP 413` immediately.
- Response header contains `Content-Type: application/json` and structured JSON error message.
- Zero bytes are read into the heap beyond header inspection.
- Upstream gRPC backend receives 0 requests.
