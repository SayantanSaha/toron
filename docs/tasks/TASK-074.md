---
id: TASK-074
type: task
title: Implement Multiple & Conflicting Content-Length Rejection (RFC 7230 §3.3.2)
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-074
implements:
  - REQ-074
verified_by:
  - TC-074
decided_by:
  - ADR-069
related_to:
  - TASK-061
  - TASK-073
---

# TASK-074 - Implement Multiple & Conflicting Content-Length Rejection (RFC 7230 §3.3.2)

## Description

Implement strict multiplicity and conflict validation for `Content-Length` headers in `pkg/httpparser/parser.go`, eliminating CL.CL HTTP request smuggling and connection desynchronization.

## Scope & Implementation Breakdown

1. **Content-Length Multiplicity Check (`pkg/httpparser/parser.go`)**:
   - Inspect `clValues := req.Header.Values("Content-Length")`.
   - If `len(clValues) > 1`:
     - Parse each value; if any values conflict or if any header contains comma-separated distinct values, reject with `HTTP 400 Bad Request`.
     - If all multiple headers contain identical numeric values, normalize to a single value.
2. **Comma-Separated Value Parsing**:
   - Split comma-separated tokens within any single `Content-Length` header; reject distinct values.
3. **Unit Testing (`pkg/httpparser/parser_test.go`)**:
   - Add test case with conflicting `Content-Length: 0` and `Content-Length: 45` returning `HTTP 400`.
   - Add test case with comma-separated `Content-Length: 10, 20` returning `HTTP 400`.

## Acceptance Criteria

- Multiple distinct `Content-Length` headers are rejected with `HTTP 400 Bad Request`.
- Residual socket bytes are not parsed as subsequent requests.
- All tests pass with `go test ./pkg/httpparser/...`.

## Rationale

Compliance with RFC 7230 §3.3.2 prevents CL.CL request desynchronization attacks.

## Constraints

- Pure Go standard library.
