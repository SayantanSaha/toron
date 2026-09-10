---
id: TASK-117
type: task
title: Kubernetes Ingress Controller Route Table Dynamic Reconciliation and Multi-Target Pod Aggregation
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-10
updated: 2026-09-10

depends_on:
  - REQ-094
  - TASK-116

owns:
  - pkg/ingress/controller.go
  - pkg/ingress/translator.go

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
  - TASK-116

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
  - TASK-116
---

# TASK-117 - Kubernetes Ingress Controller Route Table Dynamic Reconciliation and Multi-Target Pod Aggregation

## Description

Refactor the Kubernetes Ingress Controller reconciliation loop ([`syncIngresses`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go#L111-L161)) in [`pkg/ingress/controller.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go) and ingress route translation in [`pkg/ingress/translator.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/translator.go). Aggregate multiple pod endpoint IPs sharing `(Host, Prefix)` into a unified multi-target reverse proxy with round-robin load balancing, and dynamically synchronize the core routing table via atomic route replacement ([`router.ReplacePrefixRoutesBySource("k8s-ingress", desiredRoutes)`](file:///Users/sneha/Developer/toron-research/toron/pkg/router/router.go)).

This remediates vulnerability [`SEC-33`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L449-L457) ([`SR-091 Finding 3`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L130-L152), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-670](https://cwe.mitre.org/data/definitions/670.html), [CWE-1059](https://cwe.mitre.org/data/definitions/1059.html)) and satisfies acceptance criteria [`REQ-094-AC-03`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L139-L144), [`REQ-094-AC-04`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L145-L150), [`REQ-094-AC-05`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L151-L157), [`REQ-094-AC-06`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L158-L163), [`REQ-094-AC-08`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L170-L173), and [`REQ-094-AC-09`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L174-L177).

## Scope & Implementation Breakdown

1. **Multi-Target Pod Endpoint Aggregation**:
   - In [`pkg/ingress/translator.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/translator.go), update route translation:
     - For each Ingress rule and path backed by a Kubernetes Service with multiple pod endpoints (e.g., $M$ pod IPs in `Endpoints.Subsets[].Addresses`), aggregate all pod target URLs (`http://<ip>:<port>`) for that `(Host, Prefix)` into a unified multi-target configuration.
     - When no endpoint pod IPs exist in `Endpoints`, fallback to the cluster service DNS target `http://<svc>.<namespace>.svc.cluster.local:<port>`.
     - Eliminate the prior pattern of emitting separate 1-target routes per pod IP (which caused pod replica #1 to receive 100% of traffic and starved replicas #2..$M$).
     - Ensure exactly **one** route definition is created per unique `(Host, Prefix)`.
     - Configure load balancing with [`proxy.AlgorithmRoundRobin`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go#L30) across all aggregated targets.
     - Retain `discovery.DiscoveredRoute` structure or an aggregated route representation for telemetry and backward compatibility of [`c.ActiveRoutes()`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go#L81-L85).

2. **Ingress Controller Dynamic Route Reconciliation (`syncIngresses`)**:
   - Refactor [`syncIngresses`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go#L111-L161) in [`pkg/ingress/controller.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/ingress/controller.go):
     - Fetch current Ingresses and Endpoints from the Kubernetes API server via `c.client`.
     - Construct the complete desired slice of routes:
       ```go
       desiredRoutes := make([]router.PrefixRouteSpec, 0)
       ```
     - For each valid Ingress matching `c.cfg.IngressClass`, generate the aggregated `router.PrefixRouteSpec`:
       - `TargetType: router.RouteTypeUpstream`
       - `Host: r.Host`
       - `Prefix: r.Prefix`
       - `Opts: proxy.ProxyOptions{ Targets: aggregatedTargets, Algorithm: proxy.AlgorithmRoundRobin }`
     - If `c.router != nil`:
       - Call [`c.router.ReplacePrefixRoutesBySource("k8s-ingress", desiredRoutes)`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md) (implemented in [`TASK-116`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-116.md)).
     - Under `c.mu.Lock()`, update `c.activeRoutes` to store the active route snapshot.

3. **Automatic Pruning of Deleted Ingresses (Zombie Route Elimination)**:
   - When an Ingress resource or path is deleted in Kubernetes:
     - The deleted route is omitted from `desiredRoutes` constructed during the subsequent `syncIngresses` cycle.
     - Calling `ReplacePrefixRoutesBySource("k8s-ingress", desiredRoutes)` automatically removes the deleted route from `r.prefixRoutes` in `router.Router`.
     - When all Ingress resources are removed from the cluster, passing an empty slice prunes all `"k8s-ingress"` routes from the router.
     - HTTP requests sent to deleted paths immediately return HTTP `404 Not Found` rather than routing to obsolete backends.

4. **Immediate Endpoint Cutover Without Shadowing**:
   - When pod endpoint addresses change (pod restarts, rollouts, scale up/down):
     - `syncIngresses` constructs the new multi-target proxy with updated pod IPs and replaces the `"k8s-ingress"` route batch atomically.
     - Because obsolete route entries are evicted from `r.prefixRoutes` rather than shadowed, `router.ServeHTTP` immediately hits the new route.
     - Zero traffic is forwarded to dead or decommissioned pod IPs.

5. **Bounded Route Table Invariant Across Sync Cycles**:
   - Guarantee that executing $N$ consecutive synchronization cycles (via periodic 30s resync or watch events) for $K$ active Ingress rules maintains:
     $$\text{Count}(r.prefixRoutes, \text{source} = \text{"k8s-ingress"}) = K \quad \forall N \ge 1$$
   - Monotonic route table growth is eliminated; router memory footprint is $O(K)$ with respect to cluster rules and $O(1)$ with respect to synchronization cycle iterations.

6. **Thread Safety & Zero-Dependency Invariant**:
   - Ensure `syncIngresses` executes safely concurrently with background workers (`watchWorker`, `periodicResyncWorker`, `consumeEventsWorker`) and runtime HTTP dispatching (`router.ServeHTTP`).
   - Pure Go standard library packages only (`sync`, `context`, `net/http`, `net/url`, `time`, `strings`, `path`, `fmt`, `log`).
   - Zero data races under `go test -race ./pkg/ingress/...`.

## Acceptance Criteria

- `TranslateIngress` (or controller aggregation) consolidates all pod endpoint IPs sharing `(Host, Prefix)` into a single multi-target route using `RoundRobinBalancer`.
- Traffic to a multi-replica pod service is distributed evenly across all healthy pod targets ($\approx 1/M$ traffic per replica), eliminating replica starvation ([`REQ-094-AC-05`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L151-L157)).
- `syncIngresses` invokes `c.router.ReplacePrefixRoutesBySource("k8s-ingress", desiredRoutes)` on every sync cycle.
- The router table size for `"k8s-ingress"` remains strictly bounded at $K$ entries across arbitrary $N$ sync cycles ([`REQ-094-AC-03`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L139-L144)).
- Deleting an Ingress from the Kubernetes API server results in immediate route eviction from the router on the next sync cycle, returning HTTP `404 Not Found` ([`REQ-094-AC-04`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L145-L150)).
- Updating pod endpoints immediately cut over traffic to new IPs without stale route shadowing ([`REQ-094-AC-06`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md#L158-L163)).
- Evicted route reverse proxies have background health checkers cleanly stopped via `ReplacePrefixRoutesBySource`.
- Zero third-party dependencies; pure Go standard library packages.
- Zero data races under `go test -race ./pkg/ingress/...`.

## Rationale

Previously, `syncIngresses` called `c.router.RoutePrefix` in an append-only loop on every 30-second periodic resync, leaking memory monotonically and retaining deleted routes indefinitely. Furthermore, `TranslateIngress` produced individual single-target routes per pod IP, causing Toron's first-match prefix routing to route 100% of traffic to the first pod and starve all horizontal pod replicas. Aggregating pod endpoints into a unified multi-target route and dynamically reconciling the routing table via atomic replacement restores horizontal scaling, bounds memory consumption, and guarantees zero zombie routes.

## Constraints

- Pure Go standard library packages (`context`, `sync`, `net/http`, `net/url`, `time`, `strings`, `path`, `fmt`, `log`, `toron/pkg/router`, `toron/pkg/proxy`, `toron/pkg/discovery`, `toron/pkg/config`).
- Must not introduce third-party Kubernetes client dependencies (`k8s.io/client-go`).
- Must maintain backward compatibility for existing controller methods including `ActiveRoutes()`.

## Open Questions

- None. Reconciliation mechanics and multi-target aggregation are fully defined by [`REQ-094`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-094.md) and [`ADR-089`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-089.md).
