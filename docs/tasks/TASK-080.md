---
id: TASK-080
type: task
title: Harden WAF TRAVERSAL-001 Rule with Case-Insensitive Matching
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-080
implements:
  - REQ-080
verified_by:
  - TC-080
decided_by:
  - ADR-075
related_to:
  - TASK-067
---

# TASK-080 - Harden WAF TRAVERSAL-001 Rule with Case-Insensitive Matching

## Description

Update the regex pattern for `TRAVERSAL-001` in `pkg/waf/rules.go` to enforce case-insensitive `(?i)` evaluation and match uppercase hex percent-encoded path traversal sequences.

## Scope & Implementation Breakdown

1. **Regex Pattern Update (`pkg/waf/rules.go`)**:
   - Update `TRAVERSAL-001` pattern to `regexp.MustCompile(`(?i)(\.\./|\.\.\\|%2e%2e/|%2e%2e%2f|%2e%2e\\|%2e%2e%5c)`)`.
2. **Unit Testing (`pkg/waf/waf_test.go`)**:
   - Add test verifying detection of `%2E%2E/`, `%2E%2e/`, `%2e%2E%2F`, and `%2E%2E%5C` in headers, query strings, and body.

## Acceptance Criteria

- Uppercase percent-encoded directory traversal sequences trigger `TRAVERSAL-001`.
- Traversal evasion vectors against downstream backends are blocked.
- Tests pass with `go test ./pkg/waf/...`.

## Rationale

Fixes case-sensitivity evasion in default WAF rules.

## Constraints

- Standard library `regexp`.
