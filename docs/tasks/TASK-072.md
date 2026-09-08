---
id: TASK-072
type: task
title: Implement HTTP Parameter Pollution (HPP) Mitigation in Target Query Merging
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-072
implements:
  - REQ-072
verified_by:
  - TC-072
decided_by:
  - ADR-067
related_to:
  - TASK-060
---

# TASK-072 - Implement HTTP Parameter Pollution (HPP) Mitigation in Target Query Merging

## Description

Implement query parameter deduplication and collision guards in `pkg/proxy/proxy.go` when merging client-supplied query strings with pre-configured target query parameters, preventing HTTP Parameter Pollution (HPP) and parameter overriding on backend microservices.

## Scope & Implementation Breakdown

1. **Target Query Precedence & Filtering (`pkg/proxy/proxy.go`)**:
   - In `ServeHTTP` where `targetURL.RawQuery` and `clientQuery` are merged:
     - Parse the query keys defined in `targetURL.RawQuery` into a lookup set.
     - Parse the client query parameters.
     - Filter out any client query parameter whose key collides with a key defined in `targetURL.RawQuery`.
     - Reassemble the client query with non-colliding parameters and append to `targetURL.RawQuery`.
   - Ensure that gateway-configured target parameters are strictly preserved and cannot be overridden or duplicated by client parameters.

2. **Parameter Encoding & Ordering Preservation (`pkg/proxy/proxy.go`)**:
   - Maintain the percent-encoding format and ordering of non-conflicting client parameters per REQ-060.
   - Avoid creating multiple duplicate keys (e.g. `role=guest&role=admin`).

3. **Testing Verification (`pkg/proxy/proxy_test.go`)**:
   - Add unit test verifying that when `targetURL` has `role=guest` and client sends `?role=admin&user=alice`, the upstream request query is `role=guest&user=alice`.
   - Add unit test verifying that query parameters without collisions are passed cleanly.

## Acceptance Criteria

- Pre-configured target URL query parameters take precedence over conflicting client parameters.
- Duplicate query parameter injection via client requests is eliminated on target-defined keys.
- Non-colliding client parameters remain intact with encoding preserved.
- All proxy tests pass with `go test ./pkg/proxy/...`.

## Rationale

Different backend frameworks evaluate duplicated parameters in different orders (e.g. first value vs last value). If client query strings are blindly concatenated after target parameters, clients can override target security constraints.

## Constraints

- Backward-compatible with REQ-060 / TASK-060 query forwarding.
- Zero unnecessary allocations when target URL has no query parameters.

## Open Questions

- None.
