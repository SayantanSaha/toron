---
id: TASK-076
type: task
title: Implement Bounded Body Reader in HTTP/2 Ingress Adapter
status: draft
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-076
implements:
  - REQ-076
verified_by: []
decided_by: []
related_to:
  - TASK-060
  - TASK-068
---

# TASK-076 - Implement Bounded Body Reader in HTTP/2 Ingress Adapter

## Description

Enforce payload size bounds in `http2AdapterHandler` in `pkg/server/server.go`, replacing unbounded `io.ReadAll(r.Body)` with a bounded reader capped at `s.config.MaxBodyBytes` and returning `HTTP 413 Payload Too Large` if exceeded.

## Scope & Implementation Breakdown

1. **Bounded Stream Reading (`pkg/server/server.go:http2AdapterHandler`)**:
   - Determine `maxBytes := s.config.MaxBodyBytes` (default 10MB if unset).
   - Read up to `maxBytes + 1` using `io.LimitReader(r.Body, maxBytes+1)`.
   - If bytes read exceed `maxBytes`, return `http.StatusRequestEntityTooLarge` (`HTTP 413`) immediately.
2. **Payload Reader Replacement**:
   - Populate `req.Body = bytes.NewReader(bodyBytes)` only for compliant payloads.
3. **Unit Testing (`pkg/server/server_test.go`)**:
   - Add test verifying that an HTTP/2 request exceeding `MaxBodyBytes` receives `413 Payload Too Large`.

## Acceptance Criteria

- Oversized HTTP/2 payloads return `413 Payload Too Large`.
- Uncontrolled memory allocation on HTTP/2 ingress is eliminated.
- Normal HTTP/2 requests continue to function seamlessly.

## Rationale

Prevents Denial of Service / Out-Of-Memory crashes from untrusted HTTP/2 clients.

## Constraints

- Pure Go standard library.
