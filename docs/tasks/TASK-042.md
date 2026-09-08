---
id: TASK-042
type: task
title: Implement Protocol Integrity and Request Smuggling Guard
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-15
updated: 2026-09-08

depends_on:
  - REQ-042

implements:
  - REQ-042

verified_by:
  - TC-042

decided_by:
  - ADR-037

related_to:
  - TASK-002
  - TASK-004
  - TASK-041
---

# TASK-042 - Implement Protocol Integrity and Request Smuggling Guard

## Goal

Extend `pkg/waf` and `pkg/httpparser` with RFC 7230/9110 protocol integrity inspection, HTTP Request Smuggling detection (CL.TE / TE.CL / TE.TE), non-printable control character filtering, and per-route payload size bounding.

## Sub-tasks

1. Create `pkg/waf/protocol.go` implementing:
   - `ProtocolIntegrityConfig` struct (`MaxHeaderValueSize`, `MaxQuerySize`, `MaxParamSize`, `RejectControlChars`, `RejectSmugglingHeaders`).
   - `ValidateProtocolIntegrity(req *httpparser.Request, cfg ProtocolIntegrityConfig) (errCode int, errMsg string)`.
   - Inspection for conflicting `Content-Length` and `Transfer-Encoding` headers.
   - Non-printable control character check (ASCII 0x00–0x1F except `\t`, ASCII 0x7F `DEL`).
   - Size bounding check on query string, parameters, and headers.
2. Integrate `ValidateProtocolIntegrity` into `pkg/waf/waf.go` / `pkg/waf/middleware.go`.
3. Update `pkg/httpparser/parser.go` / `pkg/httpparser/request.go` to reject conflicting `Content-Length` / `Transfer-Encoding` during HTTP/1.1 stream parsing.
4. Update `pkg/config/config.go`, `config.yaml`, and `routes.yaml` to support `protocol_integrity` configuration settings and route-level `max_body_bytes`.
5. Write unit tests in `pkg/waf/protocol_test.go` and `pkg/httpparser/parser_test.go`.
6. Run full test suite `go test ./...` and configuration dry-run (`.\toron -t`).

## Acceptance Criteria

- Conflicting transfer headers return `400 Bad Request`.
- Control characters in URI or headers return `400 Bad Request`.
- Oversized headers, query parameters, or request bodies return `413 Payload Too Large`.
- 100% Go package test suite pass rate.
