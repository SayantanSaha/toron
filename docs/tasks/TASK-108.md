---
id: TASK-108
type: task
title: Router Method-Aware Prefix Routing & Path Matcher Support
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-091

owns:
  - pkg/router/router.go

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

# TASK-108 - Router Method-Aware Prefix Routing & Path Matcher Support

## Overview

Extend the core router in [`pkg/router/router.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go) to support in-process method-aware prefix handler registration and custom path matchers. This enables subsystems like the REST-to-gRPC Transcoder to register route handlers on path prefixes directly without relying on dummy reverse proxies, resolving [`SEC-30`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L411-L419).

## Scope & Implementation Breakdown

1. **`prefixRoute` Struct Extension**:
   - Extend `prefixRoute` with:
     - `routeType string` (e.g. `"handler"`, `"static"`, `"upstream"`).
     - `method string` (HTTP method restriction, e.g. `"GET"`, `"POST"` or `""` for any).
     - `matcher func(path string) bool` (optional path pattern validator).

2. **New Router Registration Methods**:
   - Implement `HandlePrefix(method, prefix string, handler HandlerFunc)`:
     Registers an in-process handler for an HTTP method and path prefix.
   - Implement `HandlePrefixWithMatcher(method, host, prefix string, headers map[string]string, matcher func(path string) bool, handler HandlerFunc)`:
     Registers a prefix handler with method, host, header matching, and an optional path validator.

3. **`Router.ServeHTTP` & `MatchPrefixRoute` Integration**:
   - In [`Router.ServeHTTP`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L453):
     - When evaluating `prefixRoutes`:
       - If `pr.method != ""` and `!strings.EqualFold(pr.method, req.Method)`, track `methodMismatch = true` and skip.
       - If `pr.matcher != nil` and `!pr.matcher(req.Path)`, skip.
       - If prefix matches but no route accepts the HTTP method, return `405 Method Not Allowed`.
   - In [`MatchPrefixRoute`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L579), filter by `pr.method` if set.

## Acceptance Criteria

- `HandlePrefix` and `HandlePrefixWithMatcher` register prefix handlers thread-safely.
- Requests matching a prefix route are dispatched to `handler` with matching HTTP method.
- Custom `matcher` functions filter out non-matching paths before invoking `handler`.
- Zero regressions in existing static file serving or upstream reverse proxying.
- Pure Go standard library (zero external dependencies).
