---
id: TASK-103
type: task
title: Bounded Stream Over-Read Protection & 413 Rejection in Transcoder
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

# TASK-103 - Bounded Stream Over-Read Protection & 413 Rejection in Transcoder

## Overview

Replace unbounded `io.ReadAll(req.Body)` in [`Engine.HandleTranscode`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L89) with bounded stream reading via `io.LimitReader(req.Body, maxBodyBytes+1)`. When incoming payload streams exceed `maxBodyBytes` (e.g., chunked transfer encoding, omitted `Content-Length`, or spoofed headers), the transcoder MUST reject the request with `HTTP 413 Payload Too Large` and abort processing without allocating unbounded memory or invoking `json.Unmarshal`, mitigating [`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401) (CWE-400/CWE-770).

## Scope & Implementation Breakdown

1. **Bounded Stream Reading via `io.LimitReader` ([`pkg/transcoder/transcoder.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go))**:
   - Replace unbounded `bodyBytes, err := io.ReadAll(req.Body)` with:
     ```go
     bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, maxBody+1))
     ```
   - Handle read errors cleanly (return `HTTP 400 Bad Request` if reading fails).

2. **Stream Over-Read Detection & HTTP 413 Rejection**:
   - Check if `int64(len(bodyBytes)) > maxBody`:
     - Set status code `http.StatusRequestEntityTooLarge` (413).
     - Set header `Content-Type: application/json`.
     - Write structured JSON payload:
       `{"error": fmt.Sprintf("Payload Too Large: request body exceeds limit of %d bytes", maxBody)}`
     - If `req.Body` implements `io.Closer`, close it.
     - Abort execution immediately.
     - Do NOT execute `json.Unmarshal`.
     - Do NOT dispatch to the upstream gRPC backend (upstream receives 0 requests).

3. **Full-Fidelity In-Limit Body Processing**:
   - If `len(bodyBytes) <= maxBody`:
     - Parse JSON body into `bodyMap` as normal.
     - Handle empty bodies (`len(bodyBytes) == 0`) safely without error.

## Acceptance Criteria

- Chunked or streaming requests exceeding `maxBodyBytes` are capped at `maxBodyBytes + 1` bytes and rejected with `HTTP 413`.
- Response format is valid JSON with `Content-Type: application/json`.
- `json.Unmarshal` is never called on oversized payloads.
- Valid payloads $\le \text{maxBodyBytes}$ are processed and forwarded with 100% byte fidelity.
- Upstream gRPC backend receives 0 requests on oversized stream rejections.
