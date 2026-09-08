---
id: TASK-002
type: task
title: HTTP/1.1 Streaming Request Parser & Response Builder
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-09-08

depends_on:
  - TASK-001

derived_from:
  - REQ-002
  - REQ-005

implements:
  - REQ-002
  - REQ-005

verified_by: []

decided_by: []

related_to: []
---

# TASK-002 - HTTP/1.1 Streaming Request Parser & Response Builder

## Description

Implement `pkg/httpparser` (or `pkg/http`) to parse raw byte streams into structured `Request` objects and construct formatted `Response` objects.

## Acceptance Criteria

- `Request` object captures Method, URI, Path, QueryParams, Proto, Headers (`Header`), and Body (`io.Reader`).
- Zero-copy or low-allocation header parsing.
- `Response` struct provides `SetStatus(code)`, `Header().Set(key, val)`, and `Write(b []byte)` methods.
- Correct formatting of HTTP/1.1 status lines, header lines, and payload body.

## Rationale

Decoupled HTTP parsing allows protocol-level handling independent of the underlying network transport.

## Constraints

- Handle standard HTTP syntax per RFC 7230 / RFC 9112.

## Open Questions

- None.
