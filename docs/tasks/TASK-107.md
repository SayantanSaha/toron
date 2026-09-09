---
id: TASK-107
type: task
title: Automated Verification Suite for Transcoder Protocol Header Compliance
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-090
  - TASK-105
  - TASK-106

owns:
  - pkg/transcoder/transcoder_test.go

references:
  - REQ-090
  - SEC-29
  - SR-081
  - ADR-085
  - TC-090

derived_from:
  - REQ-090

implements:
  - REQ-090

verified_by:
  - TC-090
---

# TASK-107 - Automated Verification Suite for Transcoder Protocol Header Compliance

## Overview

Design and implement unit and integration tests in [`pkg/transcoder/transcoder_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder_test.go) asserting that [`Engine.HandleTranscode`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L89) strips all RFC 7230 / RFC 7540 hop-by-hop headers, eliminates dynamic `Connection` header tokens, ensures clean single-valued `TE: trailers`, and preserves end-to-end metadata under `go test -race`.

## Scope & Implementation Breakdown

1. **Standard Hop-by-Hop Stripping Tests**:
   - Send requests with `Connection: keep-alive`, `Keep-Alive: timeout=10`, `Upgrade: websocket`, `Proxy-Connection: keep-alive`, `Transfer-Encoding: chunked`.
   - Verify the mock upstream gRPC server receives NONE of these headers.

2. **Dynamic Connection Token Stripping Tests**:
   - Send requests with `Connection: X-Custom-Hop, X-Another-Hop, close` and custom headers `X-Custom-Hop: secret`, `X-Another-Hop: 123`.
   - Verify the mock upstream gRPC server receives neither `X-Custom-Hop` nor `X-Another-Hop`.

3. **Strict `TE: trailers` Verification**:
   - Send request with `TE: gzip, deflate`.
   - Verify the mock upstream gRPC server receives `TE: trailers` only, without `gzip` or `deflate`.

4. **Host Header Stripping Tests**:
   - Send request with `Host: client.example.com`.
   - Verify upstream gRPC request receives authority derived from the upstream URL, not the client host header.

5. **Application Metadata Preservation Tests**:
   - Send request with `Authorization: Bearer token-xyz`, `X-Request-Id: req-12345`, `Traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01`.
   - Verify the mock upstream gRPC server receives all these headers intact.

6. **Race Safety Verification**:
   - Run tests concurrently under `go test -race ./pkg/transcoder/...`.

## Acceptance Criteria

- All test cases execute and pass cleanly.
- Zero data races detected under the Go race detector.
