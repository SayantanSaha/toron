---
id: TASK-081
type: task
title: Implement Request Body Preservation in Sidecar Proxy Engine
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-081
implements:
  - REQ-081
verified_by:
  - TC-081
decided_by:
  - ADR-076
related_to:
  - TASK-060
---

# TASK-081 - Implement Request Body Preservation in Sidecar Proxy Engine

## Description

Fix payload dropping in `pkg/sidecar/proxy.go` by properly reading and populating `toronReq.Body` from standard library `http.Request.Body` before dispatching requests through `px.ServeHTTP`.

## Scope & Implementation Breakdown

1. **Payload Extraction (`pkg/sidecar/proxy.go`)**:
   - In `ServeHTTP`, if `r.Body != nil` and `r.Method != "GET" && r.Method != "HEAD"`, read `r.Body` with `io.LimitReader` (bounded by max payload size).
   - Assign `toronReq.Body = bytes.NewReader(bodyBytes)`.
2. **Unit Testing (`pkg/sidecar/sidecar_test.go`)**:
   - Add integration test sending a POST request with JSON payload through `ProxyEngine` and verify the target server receives the complete body.

## Acceptance Criteria

- POST/PUT/PATCH bodies are preserved and forwarded intact through the sidecar proxy.
- Tests pass with `go test ./pkg/sidecar/...`.

## Rationale

Restores payload integrity for microservices intercepted by the sidecar proxy.

## Constraints

- Pure Go standard library.
