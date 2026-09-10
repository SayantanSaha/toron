---
id: TASK-116
type: task
title: Router Source-Tagged Prefix Routing and Atomic Route Replacement API
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-10
updated: 2026-09-10

depends_on:
  - REQ-094

owns:
  - pkg/router/router.go

references:
  - REQ-094
  - SEC-33
  - SR-091
  - ADR-089
  - TC-094
  - REQ-001
  - REQ-004
  - REQ-047
  - REQ-084
  - REQ-085
  - TASK-117

derived_from:
  - REQ-094
  - SEC-33
  - SR-091

implements:
  - REQ-094

verified_by:
  - TC-094

decided_by:
  - ADR-089

related_to:
  - SEC-33
  - SR-091
  - REQ-047
  - REQ-084
  - REQ-085
  - ADR-089
  - TC-094
  - TASK-117
---

# TASK-116 - Router Source-Tagged Prefix Routing and Atomic Route Replacement API

## Description

Implement source-tagged prefix routing, active reverse proxy tracking and teardown, and an atomic route replacement API ([`ReplacePrefixRoutesBySource`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)) in [`pkg/router/router.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go). This provides the foundational routing primitives required to remediate vulnerability [`SEC-33`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L449-L457) ([`SR-091 Finding 3`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L130-L152), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-670](https://cwe.mitre.org/data/definitions/670.html), [CWE-1059](https://cwe.mitre.org/data/definitions/1059.html)) and satisfies acceptance criteria [`REQ-094-AC-01`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L120-L125), [`REQ-094-AC-02`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L126-L138), [`REQ-094-AC-07`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L164-L169), and [`REQ-094-AC-08`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L170-L173).

## Scope & Implementation Breakdown

1. **Source-Tagged Prefix Route Definition (`prefixRoute`)**:
   - Extend internal struct `prefixRoute` in [`pkg/router/router.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L42-L53) to include:
     - `source string`: Originating subsystem identifier (e.g., `"k8s-ingress"`, `"config"`, `"static"`).
     - `proxy *proxy.ReverseProxy`: Reference to the active upstream reverse proxy instance, enabling lifecycle cleanup of load balancers, health check ticker goroutines, and idle socket pools.
   - Update [`RouteSnapshot`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L728-L734) and [`GetPrefixRoutes()`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L737-L755) to expose `Source string `json:"source,omitempty"` for telemetry, observability, and audit inspection.

2. **Route Specification Structure (`PrefixRouteSpec`)**:
   - Define a public route specification struct in [`pkg/router/router.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go):
     ```go
     // PrefixRouteSpec defines a declarative configuration for registering or replacing a prefix route.
     type PrefixRouteSpec struct {
         TargetType RouteType
         Host       string
         Prefix     string
         Headers    map[string]string
         DirPath    string
         Opts       proxy.ProxyOptions
     }
     ```

3. **Source-Tagged Route Registration API (`RoutePrefixWithSource`)**:
   - Implement:
     ```go
     func (r *Router) RoutePrefixWithSource(source string, targetType RouteType, host, prefix string, headers map[string]string, dirPath string, opts proxy.ProxyOptions) error
     ```
   - Retain created `*proxy.ReverseProxy` on `prefixRoute.proxy` when `normType == RouteTypeUpstream`.
   - Maintain 100% backward compatibility for existing callers of [`RoutePrefix`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L163-L248) by delegating to `RoutePrefixWithSource("config", targetType, host, prefix, headers, dirPath, opts)`.

4. **Atomic Route Replacement by Source (`ReplacePrefixRoutesBySource`)**:
   - Implement:
     ```go
     func (r *Router) ReplacePrefixRoutesBySource(source string, specs []PrefixRouteSpec) error
     ```
   - **Validation & Fail-Fast**: Prior to acquiring the write lock, validate all `specs` (valid route type, valid path prefixes, non-empty directory paths for static routes, and compile reverse proxies via [`proxy.NewProxyWithOptions`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L441-L467)). If any spec fails, return error immediately without mutating existing routing state.
   - **Atomic Partition & Swap Under Write Lock**:
     - Acquire `r.mu.Lock()`.
     - Partition existing `r.prefixRoutes`:
       - Retain routes where `pr.source != source` in their exact original order.
       - Isolate routes where `pr.source == source` as evicted routes.
     - Append the validated new pre-built routes tagged with `source` to the retained routes slice.
     - Replace `r.prefixRoutes` with the updated slice.
     - Release `r.mu.Unlock()`.
   - **Resource Teardown**:
     - For every evicted route, if `pr.proxy != nil`, invoke `pr.proxy.Close()` to stop active health checks ([`t.StopActiveHealthCheck()`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L100-L103)) and balancer routines.
     - Ensure cleanup runs safely without panicking if `pr.proxy` is nil or already stopped.
   - **Empty Replacement Slice**: When `len(specs) == 0`, all routes matching `source` are pruned and closed, leaving routes belonging to other sources completely unaffected.

5. **Lifecycle Teardown on Route Deletion & Router Reset**:
   - Update [`RemovePrefixRoute`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L758-L776) and [`Reset`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L100-L113) to call `.Close()` on evicted `prefixRoute.proxy` instances, preventing socket and goroutine leaks upon route deletion or server shutdown.

6. **Concurrency Safety & Read-Path Non-Blocking Invariant**:
   - Ensure `ReplacePrefixRoutesBySource` maintains atomic cutover: concurrent callers of `ServeHTTP` acquiring `r.mu.RLock()` observe either the full prior route set or full new route set, with zero partial or corrupt states.
   - Ensure zero data races under `go test -race ./pkg/router/...`.

## Acceptance Criteria

- `prefixRoute` stores `source string` and `proxy *proxy.ReverseProxy`.
- `RouteSnapshot` contains `Source string `json:"source,omitempty"`.
- `RoutePrefixWithSource` accepts explicit `source` and registers the prefix route tagged with that source identifier.
- `RoutePrefix` delegates to `RoutePrefixWithSource("config", ...)` preserving 100% backward compatibility.
- `ReplacePrefixRoutesBySource(source, specs)` atomically replaces all prefix routes matching `source` under write lock.
- Existing prefix routes from other sources (e.g. `"config"`, `"static"`) remain completely intact and preserve their relative order.
- When `specs` is empty, all routes belonging to `source` are pruned cleanly.
- Evicted routes with active `*proxy.ReverseProxy` have `.Close()` invoked, stopping active health checkers and socket resources ([`REQ-094-AC-07`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L164-L169)).
- Invalid specs return error without modifying existing route state (all-or-nothing guarantee).
- `RemovePrefixRoute` and `Reset` cleanly close evicted proxy instances.
- Zero third-party dependencies; pure Go standard library packages.
- Zero data races under `go test -race ./pkg/router/...`.

## Rationale

Previously, [`RoutePrefix`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go#L163-L248) lacked route origin metadata and offered only an append-only mutation model. Dynamic subsystems (such as the Kubernetes Ingress Controller) had no way to replace or synchronize their routes without clearing manual or static routes via `Reset()`. By introducing source tagging and an atomic `ReplacePrefixRoutesBySource` API with automatic `proxy.Close()` teardown, the router enables subsystems to achieve bounded route lifecycles and leak-free dynamic routing.

## Constraints

- Pure Go standard library packages (`sync`, `net/http`, `path`, `path/filepath`, `strings`, `fmt`, `toron/pkg/proxy`, `toron/pkg/httpparser`, `toron/pkg/metrics`, `toron/pkg/waf`).
- Must not break existing callers of `RoutePrefix` or `HandlePrefix`.
- Handlers and proxies must be pre-built prior to lock acquisition to minimize write lock hold duration.

## Open Questions

- None. Struct signatures and replacement semantics are fully specified by [`REQ-094`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md) and [`ADR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-089.md).
