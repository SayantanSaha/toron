---
id: TASK-109
type: task
title: Direct Parameterized Subpath Routing & Empty Upstream Proxy Elimination in Transcoder
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-091
  - TASK-108

owns:
  - pkg/transcoder/transcoder.go

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

# TASK-109 - Direct Parameterized Subpath Routing & Empty Upstream Proxy Elimination in Transcoder

## Overview

Eliminate the registration of empty upstream reverse proxies in [`Engine.registerRoutes`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L68) and implement direct parameterized subpath routing using the router's new `HandlePrefixWithMatcher` API, resolving [`SEC-30`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L411-L419).

## Scope & Implementation Breakdown

1. **Path Pattern Matcher Function**:
   - Implement `MatchPathPattern(pattern, path string) bool` in `pkg/transcoder/transcoder.go` (exported or package-level) that splits `pattern` and `path` into segments and verifies:
     - Segment lengths match (`len(patternParts) == len(pathParts)`).
     - Literal segments match exactly.
     - Wildcard parameter segments (prefixed with `:`) match any non-empty segment value.

2. **Elimination of Empty Upstream Proxy**:
   - In [`Engine.registerRoutes`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder/transcoder.go#L68), remove the call:
     `_ = e.router.RoutePrefix("upstream", "", cleanPrefix, nil, "", proxy.ProxyOptions{})`

3. **Direct Parameterized & Exact Route Binding**:
   - For parameterized rules containing `/:`:
     - Determine the literal path prefix before the first parameter segment.
     - Bind the prefix to `e.router.HandlePrefixWithMatcher` with a matcher invoking `MatchPathPattern(rRule.HTTPPath, p)`.
     - Direct dispatch routes incoming subpaths (e.g. `/v1/users/usr-777`) directly to `HandleTranscode`.
   - For non-parameterized rules (e.g. `POST /v1/users`):
     - Bind directly to `e.router.Handle(rRule.HTTPMethod, rRule.HTTPPath, handler)`.

## Acceptance Criteria

- Parameterized paths like `/v1/users/usr-777` route directly to `HandleTranscode` and return 200 OK.
- Zero empty upstream proxies are registered.
- Requests with mismatched segment count or literal mismatch are rejected cleanly with 404 Not Found rather than 502 Bad Gateway.
- Pure Go standard library (zero external dependencies).
