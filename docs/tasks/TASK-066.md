---
id: TASK-066
type: task
title: Implement Access Log CRLF Sanitization and Control Character Filtering
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-066
implements:
  - REQ-066
verified_by:
  - TC-066
decided_by:
  - ADR-061
related_to:
  - TASK-058
---

# TASK-066 - Implement Access Log CRLF Sanitization and Control Character Filtering

## Description

Implement sanitization of client-controlled string fields (`User-Agent`, `Referer`, `Host`, `Method`, `Path`) in `pkg/logging/manager.go` before formatting text access log entries (Combined Log Format), preventing CRLF log injection (CWE-117) and SIEM log forgery.

## Scope & Implementation Breakdown

1. **Sanitization Helper (`pkg/logging/manager.go`)**:
   - Implement `sanitizeLogField(val string) string` to inspect strings for carriage returns (`\r`), line feeds (`\n`), and non-printable ASCII control characters (`< 0x20` or `0x7f`).
   - Replace newline/CR characters with a single space or escape sequence (e.g. `\n` -> `\n`, or replacement `_`).
   - Ensure zero-allocation fast path when no control characters are present in the input.

2. **Access Log Text Formatting Guard (`pkg/logging/manager.go`)**:
   - In `LogAccess`, apply `sanitizeLogField` to `entry.Method`, `entry.Path`, `referer`, `ua`, and `clientIP` before `fmt.Sprintf` Combined Log Format assembly.
   - Verify that JSON access logging continues to rely on `json.Marshal` which already escapes control characters safely.

3. **Testing Verification (`pkg/logging/manager_test.go`)**:
   - Add unit tests verifying that multi-line payloads in `User-Agent` or `Path` (containing `\r\n`) produce strictly a single physical line in the text sink.
   - Verify that log forging attempts do not split lines or inject fake HTTP status codes.

## Acceptance Criteria

- Text-format access log writes never emit more than one newline per access log event.
- Client headers containing `\r` and `\n` are sanitized into safe single-line representations.
- Benchmark shows zero or negligible allocation overhead on benign headers.
- Unit tests pass cleanly with `go test ./pkg/logging/...`.

## Rationale

Without sanitization, an unauthenticated client sending crafted HTTP request headers can write arbitrary fake log lines into access logs, enabling audit evasion, log poisoning, and SIEM confusion.

## Constraints

- Backward-compatible with Combined Log Format (CLF) log parsers.
- Must execute efficiently inside the hot access logging path.

## Open Questions

- None.
