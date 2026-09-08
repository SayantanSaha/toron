---
id: TASK-073
type: task
title: Implement HTTP Header Field-Name Whitespace Rejection (RFC 7230 §3.2.4)
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-073
implements:
  - REQ-073
verified_by:
  - TC-073
decided_by:
  - ADR-068
related_to:
  - TASK-061
---

# TASK-073 - Implement HTTP Header Field-Name Whitespace Rejection (RFC 7230 §3.2.4)

## Description

Implement strict header token validation in `pkg/httpparser/parser.go` to reject any incoming request line containing whitespace between the header field-name and the colon delimiter with `HTTP 400 Bad Request`.

## Scope & Implementation Breakdown

1. **Header Name Scanning (`pkg/httpparser/parser.go`)**:
   - In `httpparser.ParseRequest`, inspect `k := lineTrimmed[:colonIdx]`.
   - If `strings.ContainsAny(k, " \t\r\n")` or `k == ""` or starts with invalid token characters, reject immediately with `fmt.Errorf("%w: whitespace in header field-name", ErrBadRequest)`.
2. **Prevent Connection Reuse**:
   - Set connection close so socket desynchronization cannot persist.
3. **Unit Testing (`pkg/httpparser/parser_test.go`)**:
   - Add tests verifying rejection of `Transfer-Encoding : chunked`, `Host\t: example.com`, and trailing whitespace before colons.

## Acceptance Criteria

- Any header with whitespace preceding the colon is rejected with `HTTP 400 Bad Request`.
- Transfer-Encoding protocol bypass vectors via whitespace smuggling are eliminated.
- All tests pass with `go test ./pkg/httpparser/...`.

## Rationale

Prevents header whitespace smuggling against downstream proxies per RFC 7230 §3.2.4.

## Constraints

- Standard library Go only; zero performance regression.
