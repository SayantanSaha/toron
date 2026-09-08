---
id: TASK-092
type: task
title: Automated Verification Suite for Sidecar Body Limiting & 413 Rejection
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-086
  - TASK-090
  - TASK-091

owns:
  - pkg/sidecar/sidecar_test.go

references:
  - REQ-086
  - SEC-25
  - TC-086

derived_from:
  - REQ-086

implements:
  - REQ-086

verified_by:
  - TC-086
---

# TASK-092 - Automated Verification Suite for Sidecar Body Limiting & 413 Rejection

## Overview

Implement a comprehensive automated verification test suite in [`pkg/sidecar/sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go) validating configurable body limits, immediate HTTP 413 Payload Too Large rejections, byte-fidelity payload forwarding, non-mutating request bypass, and concurrent race safety, satisfying test specification [`TC-086`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-086.md) and resolving [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374).

## Scope & Implementation Breakdown

1. **Test Default Limit & Fallback Behavior ([`pkg/sidecar/sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go))**:
   - Construct [`ProxyEngine`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L23) instances with `MaxBodyBytes: 0` and `MaxBodyBytes: -1`.
   - Verify that engine configuration defaults to `10 * 1024 * 1024` bytes (10 MB).

2. **Test Fast-Path Declared `Content-Length` 413 Rejection ([`pkg/sidecar/sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go))**:
   - Configure a sidecar proxy with a custom limit (e.g. `MaxBodyBytes: 1024` bytes).
   - Dispatch an HTTP POST request with `Content-Length: 2048` containing an oversized payload.
   - Assert the client receives HTTP `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`).
   - Assert the mock upstream application server receives 0 requests.

3. **Test Chunked / Streamed Over-Read 413 Rejection ([`pkg/sidecar/sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go))**:
   - Send a chunked transfer request or stream without explicit oversized `Content-Length` where transmitted bytes exceed `MaxBodyBytes`.
   - Assert the sidecar halts streaming ingestion via `io.LimitReader`, responds with HTTP 413, and aborts proxying.
   - Assert the upstream server does NOT receive partial or truncated data.

4. **Test Byte-Fidelity In-Bounds Forwarding ([`pkg/sidecar/sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go))**:
   - Dispatch valid HTTP POST / PUT requests within the configured limit (e.g. 512 bytes with limit 1024 bytes).
   - Assert the client receives HTTP 200 OK.
   - Assert the upstream mock server receives the identical payload byte-for-byte.

5. **Test Non-Mutating and Empty Payload Bypass ([`pkg/sidecar/sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go))**:
   - Dispatch `GET` and `HEAD` requests and verify direct forwarding without body allocation or false-positive 413 errors.

6. **Test Concurrency and Race Safety ([`pkg/sidecar/sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go))**:
   - Concurrently send parallel requests (valid, oversized, chunked, GET) across multiple goroutines.
   - Verify zero race conditions or memory corruption under `go test -race ./pkg/sidecar/...`.

## Acceptance Criteria

- All unit and integration tests pass with `go test -v -race ./pkg/sidecar/...`.
- Test suite covers default limit fallback, custom configured limits, declared Content-Length overflow, stream over-read overflow, in-bounds byte preservation, and non-mutating requests.
- No upstream mock receives partial or truncated payloads upon oversized requests.
- Zero third-party dependencies (uses only standard library `testing`, `net/http/httptest`, `io`, `sync`).

## Blockers & Risks

- Port collisions during test suite execution: ensure dynamic port binding (`:0` or unique port assignments) is used for test listeners.
