---
id: TASK-110
type: task
title: Automated Verification Suite for Parameterized Transcoder Subpath Dispatch
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-091
  - TASK-108
  - TASK-109

owns:
  - pkg/transcoder/transcoder_test.go
  - pkg/router/router_test.go

references:
  - REQ-091
  - SEC-30
  - SR-081
  - ADR-086
  - TC-091

derived_from:
  - REQ-091

implements:
  - REQ-091

verified_by:
  - TC-091
---

# TASK-110 - Automated Verification Suite for Parameterized Transcoder Subpath Dispatch

## Overview

Implement a comprehensive automated test suite verifying that parameterized REST-to-gRPC transcoding routes dispatch through [`Router.ServeHTTP`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L453) directly, resolving [`SEC-30`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L411-L419) and validating Missing Security Test 5 in [`SR-081`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L221).

## Scope & Implementation Breakdown

1. **Router Prefix Matching Tests (`pkg/router/router_test.go`)**:
   - `TestRouter_HandlePrefix_MethodAndMatcher`: Verify `HandlePrefix` and `HandlePrefixWithMatcher` dispatches matching paths and methods, returns 405 on method mismatch, and falls through on matcher failure.

2. **Transcoder Subpath Dispatch Tests (`pkg/transcoder/transcoder_test.go`)**:
   - `TestTranscoder_ParameterizedSubpathDispatch` (TC-091-01): Verify `r.ServeHTTP(req, res)` with `GET /v1/users/usr-777` directly routes to gRPC upstream and returns `200 OK` with JSON response (eliminating 502 Bad Gateway).
   - `TestTranscoder_MultiLevelRouteSegregation` (TC-091-02): Verify multiple routes sharing the same prefix (e.g. `GET /v1/users/:id` and `GET /v1/users/:id/orders/:orderId`) dispatch to their respective gRPC methods without collision.
   - `TestTranscoder_WrongMethodOnParameterizedRoute` (TC-091-03): Verify sending `POST /v1/users/usr-777` to a `GET`-only route returns `405 Method Not Allowed`.
   - `TestTranscoder_SegmentCountMismatch_NotFound` (TC-091-04): Verify sending `/v1/users/usr-777/extra` or `/v1/users` returns `404 Not Found`.
   - `TestTranscoder_MatchPathPattern` (TC-091-05): Unit test coverage for `MatchPathPattern` across various pattern shapes, single/multi parameters, and edge cases.
   - `TestTranscoder_ParameterizedSubpath_ConcurrencyRaceSafety` (TC-091-06): Verify 50 concurrent parameterized requests through `router.ServeHTTP` execute race-clean under `go test -race`.

## Acceptance Criteria

- All test cases in `TC-091` implemented and passing cleanly.
- 100% race-clean under `go test -race ./...`.
- Zero external test dependencies (Go stdlib only).
