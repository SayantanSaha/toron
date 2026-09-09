---
id: TASK-104
type: task
title: Automated Verification Suite for REST-to-gRPC Transcoder Request Body Limits
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-089
  - TASK-101
  - TASK-102
  - TASK-103

owns:
  - pkg/transcoder/transcoder_test.go
  - pkg/config/config_test.go

references:
  - REQ-089
  - SEC-28
  - SR-081
  - ADR-084
  - TC-089

derived_from:
  - REQ-089

implements:
  - REQ-089

verified_by:
  - TC-089
---

# TASK-104 - Automated Verification Suite for REST-to-gRPC Transcoder Request Body Limits

## Overview

Design and implement a comprehensive automated verification test suite in [`pkg/transcoder/transcoder_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder_test.go) and [`pkg/config/config_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config_test.go) verifying that the REST-to-gRPC Transcoder rigorously enforces `MaxBodyBytes` limits across declared `Content-Length` headers, streaming bodies, default fallbacks, in-limit processing, non-mutating requests, and concurrent execution under the Go race detector (`go test -race`).

## Scope & Implementation Breakdown

1. **Configuration Schema and Validation Tests ([`pkg/config/config_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config_test.go))**:
   - Verify `TranscoderConfig.GetMaxBodyBytes()` returns `4 * 1024 * 1024` when `MaxBodyBytes == 0` or negative.
   - Verify YAML/JSON unmarshaling of `max_body_bytes`.
   - Verify negative value rejection in config loader.

2. **Transcoder Body Limit Test Cases ([`pkg/transcoder/transcoder_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder_test.go))**:
   - **Declared `Content-Length` Fast-Fail**:
     Send request with declared `Content-Length > maxBodyBytes`. Assert `res.StatusCode == 413`, JSON error response, and mock gRPC backend records 0 incoming calls.
   - **Stream Over-Read Rejection**:
     Send streaming/chunked request (omitted `Content-Length` or within limit) with body larger than `maxBodyBytes`. Assert `res.StatusCode == 413`, JSON error response, and mock gRPC backend records 0 incoming calls.
   - **Configurable Limit Customization**:
     Configure custom `MaxBodyBytes` (e.g. 1 KB). Verify 1000-byte body succeeds, 1025-byte body fails with 413.
   - **Default 4 MB Fallback**:
     Configure `MaxBodyBytes: 0`. Verify default 4 MB is applied.
   - **In-Limit Byte Fidelity**:
     Send valid payload $\le \text{maxBodyBytes}$. Assert 200 OK and mock gRPC backend receives correctly framed Protobuf payload.
   - **Non-Mutating / Nil Body Bypass**:
     Send `GET` or `DELETE` with `req.Body == nil` or empty body. Assert normal handling and 200 OK.
   - **High-Concurrency & Race Safety**:
     Execute concurrent valid and oversized transcode requests against the engine under `go test -race`.

## Acceptance Criteria

- All unit and integration test cases pass cleanly.
- Upstream gRPC mock verifies 0 requests received for all 413 rejection paths.
- Zero data races reported under `go test -race ./pkg/transcoder/...` and `go test -race ./pkg/config/...`.
