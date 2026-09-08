---
id: TASK-022
type: task
title: Implement HTTP/2 Server Connection Handler and Configuration Options
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-09-08

depends_on:
  - REQ-022

implements:
  - REQ-022

verified_by:
  - TC-022

decided_by:
  - ADR-017

related_to:
  - TASK-001
  - TASK-005
---

# TASK-022 - Implement HTTP/2 Server Connection Handler and Configuration Options

## Goal

Add HTTP/2 configuration options to `pkg/config`, HTTP/2 connection detection and handler dispatch to `pkg/server`, and unit tests.

## Sub-tasks

1. Add `HTTP2Config` struct (`Enabled`, `MaxConcurrentStreams`, `MaxFrameSize`, `AllowH2C`) to `pkg/config`.
2. Integrate `golang.org/x/net/http2` server handler in `pkg/server/server.go`.
3. Add connection preface detection for HTTP/2 (`PRI * HTTP/2.0...`) in `handleConn`.
4. Bridge `net/http` / HTTP/2 request context to `httpparser.Request` & `router.Router`.
5. Add HTTP/2 unit tests in `pkg/server/http2_test.go` and run `go test ./...`.

## Acceptance Criteria

- HTTP/2 client connections (over h2c / TLS) correctly route through `router.Router`.
- All Go unit tests pass cleanly.
