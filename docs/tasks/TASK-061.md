---
id: TASK-061
type: task
title: Implement HTTP Request Smuggling Prevention and Transfer-Encoding Protocol Guard
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-061
implements:
  - REQ-061
verified_by:
  - TC-061
decided_by:
  - ADR-056
related_to:
  - TASK-002
  - TASK-042
---

# TASK-061 - Implement HTTP Request Smuggling Prevention and Transfer-Encoding Protocol Guard

## Description

Implement strict protocol guards in `pkg/httpparser/parser.go` and `pkg/server/server.go` to reject unsupported `Transfer-Encoding: chunked` requests and terminate socket connections upon transfer coding violations, preventing HTTP request smuggling and connection desynchronization.

## Scope & Implementation Breakdown

1. **Parser Protocol Guard (`pkg/httpparser/parser.go`)**:
   - In `ParseRequest`, inspect the `Transfer-Encoding` header.
   - If `Transfer-Encoding` is present and request body dechunking is not supported by the parser, return an explicit error (e.g. `ErrUnsupportedTransferEncoding`).
   - Ensure conflicting `Content-Length` and `Transfer-Encoding` continues to return `ErrBadRequest`.

2. **Connection Lifecycle & Teardown (`pkg/server/server.go`)**:
   - In `handleConn`, if `ParseRequest` returns `ErrUnsupportedTransferEncoding`, emit `HTTP/1.1 501 Not Implemented` with `Connection: close`.
   - Immediately close the underlying socket connection (`conn.Close()`) to purge unconsumed chunk data from the stream.

3. **Unit & Integration Testing (`pkg/httpparser/parser_test.go`, `pkg/server/server_test.go`)**:
   - Add unit test verifying that requests containing standalone `Transfer-Encoding: chunked` fail with the expected error.
   - Add integration test verifying that persistent keep-alive connections do not parse subsequent pipelined requests from unconsumed chunk bytes.

## Acceptance Criteria

- Any request declaring `Transfer-Encoding` is rejected with `501 Not Implemented` or connection termination.
- Unread chunk bytes cannot desynchronize subsequent requests on the TCP connection.
- Unit and integration tests pass cleanly with `go test ./pkg/httpparser/... ./pkg/server/...`.

## Rationale

Without native dechunking, accepting chunked requests while treating body length as zero leaves payload bytes in the TCP socket, causing subsequent requests to be deserialized from arbitrary attacker data.

## Constraints

- Zero external dependencies.
- Constant-time header inspection without buffering overhead.

## Open Questions

- Should future iterations implement a full zero-copy chunked streaming decoder?
