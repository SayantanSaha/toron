---
id: TASK-132
type: task
title: Implementation of RFC 7230 §3.2 Header Field-Name Token Grammar Hardening
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-109

derived_from:
  - REQ-109
  - SRC-04

implements:
  - REQ-109

verified_by:
  - TC-109

decided_by:
  - ADR-109

related_to:
  - REQ-109
  - ADR-109
  - TC-109
  - CR-105
  - SR-109
---

# TASK-132 - Implementation of RFC 7230 §3.2 Header Field-Name Token Grammar Hardening

## 1. Description

Decompose and implement the technical changes necessary to validate HTTP/1.1 header field-names against RFC 7230 §3.2 and §3.2.6 token grammar (`SRC-04`, `REQ-109`), eliminating parser differential and header injection vectors.

## 2. Subtask Breakdown

### TASK-132.1: Precomputed Token Lookup Table
- **Component**: `pkg/httpparser/parser.go`
- **Scope**:
  - Define a package-level static `var validHeaderTokenTable [256]bool` array initialized during package load.
  - Set `true` for all RFC 7230 §3.2.6 `tchar` bytes:
    - Digits: `'0'`–`'9'`
    - Lowercase Alpha: `'a'`–`'z'`
    - Uppercase Alpha: `'A'`–`'Z'`
    - Allowed Punctuation: `!`, `#`, `$`, `%`, `&`, `'`, `*`, `+`, `-`, `.`, `^`, `_`, `` ` ``, `|`, `~`
  - Leave all delimiters (`"()", "/", ":", ";", "<", "=", ">", "?", "@", "[", "\", "]", "{", "}"`), whitespace (`SP`, `HTAB`, `CR`, `LF`), control characters (`0x00`–`0x1F`, `0x7F`), and high bytes (`0x80`–`0xFF`) as `false`.

### TASK-132.2: Zero-Allocation Field-Name Token Scanner
- **Component**: `pkg/httpparser/parser.go`
- **Scope**:
  - In `ParseRequest`, replace the basic `strings.ContainsAny(k, " \t\r\n")` check with a dedicated token validator.
  - Reject empty field names (`k == ""`).
  - Scan each byte of `k` against `validHeaderTokenTable`.
  - If a non-token character is detected:
    - If the byte is whitespace (`' '`, `'\t'`, `'\r'`, `'\n'`), return `fmt.Errorf("%w: whitespace in header field-name", ErrBadRequest)`.
    - Otherwise, return `fmt.Errorf("%w: invalid header field-name token grammar", ErrBadRequest)`.

### TASK-132.3: Unit Tests for Delimiter and Non-Token Rejections
- **Component**: `pkg/httpparser/parser_test.go`
- **Scope**:
  - Add comprehensive test table covering:
    - Delimiter characters: `@`, `(`, `)`, `,`, `/`, `;`, `<`, `=`, `>`, `?`, `[`, `\`, `]`, `{`, `}`, `"`
    - Control characters: NUL (`0x00`), ESC (`0x1B`), BEL (`0x07`), DEL (`0x7F`)
    - High-bit / Non-ASCII bytes: `0x80`, `0xC0`, `0xFF`
    - Valid complex RFC 7230 headers (e.g., `X-Custom-Header_1.0!#$`)

### TASK-132.4: Nanosecond Rejection Microbenchmark
- **Component**: `pkg/httpparser/parser_test.go`
- **Scope**:
  - Implement `BenchmarkParseRequest_InvalidTokenRejection(b *testing.B)` to measure nanosecond-level rejection latency and assert 0 B/op allocations.

## 3. Acceptance Criteria

- All unit and benchmark tests pass under `go test -v -race -count=1 ./...`.
- 100% compliance with RFC 7230 §3.2 and §3.2.6 token grammar.
- Zero external third-party dependencies.
