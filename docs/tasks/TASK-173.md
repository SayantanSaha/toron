---
id: TASK-173
type: task
title: Route Bulkhead Concurrency Limiter and Fast-Fail Gating in pkg/router
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-25
updated: 2026-09-25

depends_on:
  - ../requirements/REQ-145.md
  - ./TASK-169.md

derived_from:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md

implements:
  - ../requirements/REQ-145.md

related_to:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md
  - ../requirements/REQ-087.md
  - ../architecture/ADR-082.md
  - ./TASK-169.md
  - ./TASK-170.md
  - ./TASK-171.md
  - ./TASK-172.md
---

# TASK-173 - Route Bulkhead Concurrency Limiter and Fast-Fail Gating in pkg/router

## 1. Overview & Objective

Decompose the route bulkhead concurrency limiting and fast-fail gating requirements of [`REQ-145`](../requirements/REQ-145.md) (FR-145-1, FR-145-5, NFR-145-2, Section 6.1, Section 6.3) and [`AN-006`](../analysis/AN-006.md) (Section 3.3) into concrete engineering deliverables within [`pkg/router/router.go`](../../pkg/router/router.go).

Toron's reactor architecture maintains a fixed worker pool (`worker_pool_size: 128`). When multiple slow clients initiate sustained transfers (e.g. 200MB uploads) or wait on slow legacy backends (e.g. 45s database queries), worker goroutines remain bound to those connections. If unconstrained, 128 slow requests consume all reactor workers, filling the task queue and blocking listener `Accept()`. This creates a critical Denial-of-Service condition ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)) where health checks (`/health`), metrics (`/metrics`), and microservice APIs are completely starved.

This task implements Layer 7 route-level bulkhead concurrency gates in [`pkg/router/router.go`](../../pkg/router/router.go):
1. Atomic in-flight request tracking per route bounded by `max_concurrency`.
2. Instant `HTTP 503 Service Unavailable` with `Retry-After: 5` fast-fail rejection when a route's bulkhead is full.
3. Pre-body route lookup enabling `pkg/server` to inspect limits and enforce bulkheads before reading request bodies.
4. Permanent reservation of reactor capacity for operational probes and unconstrained routes.

---

## 2. Traceability

- **Requirement**: [`REQ-145`](../requirements/REQ-145.md) (FR-145-1, FR-145-5, NFR-145-2, AC-145-06, Section 6.1, Section 6.3)
- **Analysis**: [`AN-006`](../analysis/AN-006.md) (Section 3.3, Section 8.2 Item 5)
- **Dependencies**:
  - [`TASK-169.md`](./TASK-169.md): Configuration Schema Extensions
- **Downstream Tasks**:
  - [`TASK-171.md`](./TASK-171.md): Activity-Refreshed Read Deadline Tracker in `pkg/server`

---

## 3. Work Package Breakdown

### WP-1: Route State & Configuration Extensions ([`pkg/router/router.go`](../../pkg/router/router.go))

1. Extend `prefixRoute` struct:
   ```go
   type prefixRoute struct {
       // ... existing fields ...
       maxConcurrency         int
       activeRequests         int64
       maxBodyBytes           int64
       readTimeout            time.Duration
       writeTimeout           time.Duration
       responseHeaderTimeout  time.Duration
       streamRequestBody      *bool
   }
   ```
2. Extend `PrefixRouteSpec` and `compilePrefixRoute`:
   - Transfer `MaxConcurrency`, `MaxBodyBytes`, `ReadTimeout`, `WriteTimeout`, `ResponseHeaderTimeout`, and `StreamRequestBody` from `ProxyOptions` / `ProxyRouteConfig`.
   - **Automatic Bulkhead Safety Guardrail (REQ-145 Section 6.1)**:
     - If `maxConcurrency == 0` (unspecified) but the route specifies elevated `max_body_bytes > 4MB` or elevated `response_header_timeout > 10s`:
       $$\text{maxConcurrency} = \min(32, \max(1, \text{workerPoolSize} / 4))$$
       (e.g., 32 on a 128-worker gateway).
     - If the route does not specify elevated parameters and `maxConcurrency == 0`, concurrency remains unconstrained ($0$).

### WP-2: Pre-Body Route Lookup & Limit Resolution ([`pkg/router/router.go`](../../pkg/router/router.go))

Implement pre-body route lookup to allow `pkg/server` to query route limits during Phase 1:
```go
// RouteMatchInfo holds matched route metadata and resolved limits.
type RouteMatchInfo struct {
    Prefix                string
    RouteType             string
    MaxBodyBytes          int64
    MaxConcurrency        int
    ReadTimeout           time.Duration
    WriteTimeout          time.Duration
    ResponseHeaderTimeout time.Duration
    StreamRequestBody     *bool
    routeRef              *prefixRoute
}

// LookupPrefixRoute evaluates route matching (path, host, method, headers) without consuming request body.
func (r *Router) LookupPrefixRoute(req *httpparser.Request) (*RouteMatchInfo, bool)
```

### WP-3: Bulkhead Concurrency Gate & Fast-Fail Rejection ([`pkg/router/router.go`](../../pkg/router/router.go))

1. **Atomic Acquisition & Release**:
   ```go
   // TryAcquireRouteSlot attempts to reserve a concurrency slot on the matched route.
   // Returns release callback and true if slot was acquired, or false if route is at capacity.
   func (r *Router) TryAcquireRouteSlot(info *RouteMatchInfo) (release func(), acquired bool) {
       if info == nil || info.routeRef == nil || info.MaxConcurrency <= 0 {
           return func() {}, true // Unconstrained route
       }
       pr := info.routeRef
       current := atomic.LoadInt64(&pr.activeRequests)
       if current >= int64(info.MaxConcurrency) {
           return nil, false
       }
       newVal := atomic.AddInt64(&pr.activeRequests, 1)
       if newVal > int64(info.MaxConcurrency) {
           atomic.AddInt64(&pr.activeRequests, -1) // Rollback
           return nil, false
       }
       var once sync.Once
       return func() {
           once.Do(func() {
               atomic.AddInt64(&pr.activeRequests, -1)
           })
       }, true
   }
   ```
2. **Fast-Fail Rejection Response (REQ-145 Section 6.3)**:
   - When `acquired == false`:
     - Set status `HTTP 503 Service Unavailable`.
     - Set header `Retry-After: 5`.
     - Set header `Content-Type: application/json`.
     - Set header `Connection: close`.
     - Response body: `{"error":"503 Service Unavailable: Route concurrency limit reached"}`.
     - Serialize directly to socket, bypass body reading, and immediately close connection.
3. **Deterministic Cleanup Guarantee**:
   - Ensure `release()` is registered with `defer` at the outermost boundary in `s.handleConn` / `router.ServeHTTP`.
   - Ensure release executes cleanly under handler panic recovery, context cancellation, or upstream transport errors.

### WP-4: Automated Verification Test Suite ([`pkg/router/router_test.go`](../../pkg/router/router_test.go))

1. **Bulkhead Capacity Limit Test**:
   - Register route with `max_concurrency: 4`.
   - Dispatch 4 long-running requests in background goroutines.
   - Dispatch a 5th request; verify instant response `HTTP 503 Service Unavailable` with `Retry-After: 5`.
2. **Deterministic Release Verification**:
   - Complete 1 of the 4 active requests.
   - Dispatch a 6th request; verify it now acquires a slot and returns `HTTP 200 OK`.
3. **Panic Recovery Release**:
   - Dispatch request to handler that panics; verify active concurrency counter returns to 0 after panic recovery.
4. **Reactor Isolation & Zero Starvation Test (NFR-145-2 / AC-145-06)**:
   - Saturated route with `max_concurrency: 16` receiving flood of 50 requests (all excess rejected with 503).
   - Concurrently send requests to `/health` and `/metrics`.
   - Verify all health checks respond with `HTTP 200 OK` in $< 5\,\text{ms}$ with zero starvation.

---

## 4. Acceptance Criteria

- [ ] **AC-173-1 (Pre-Body Route Lookup)**: `LookupPrefixRoute` returns correct route metadata, limits, and timeouts before any payload body bytes are read from the network socket.
- [ ] **AC-173-2 (Bulkhead Concurrency Gate)**: Concurrent in-flight requests on a route configured with `max_concurrency: N` are capped at $N$.
- [ ] **AC-173-3 (Instant Fast-Fail Rejection)**: When active concurrency reaches $N$, request $N+1$ immediately receives `HTTP 503 Service Unavailable` with `Retry-After: 5` header and JSON error payload.
- [ ] **AC-173-4 (Deterministic Semaphore Release)**: Active concurrency counter is decremented upon normal completion, upstream error, client abort, and handler panic.
- [ ] **AC-173-5 (Automatic Safety Guardrail)**: Routes declaring elevated payload ceilings ($> 4\,\text{MB}$) or elevated backend timeouts ($> 10\,\text{s}$) with omitted `max_concurrency` automatically enforce $\min(32, \text{WorkerPoolSize} / 4)$.
- [ ] **AC-173-6 (Zero Worker Starvation)**: Operational endpoints (`/health`, `/metrics`) maintain baseline response latencies ($< 5\,\text{ms}$) even when legacy route bulkheads are completely saturated.

---

## 5. Constraints & Standards

- **Strictly Relative Links**: All internal document links must use relative syntax (`../requirements/REQ-145.md`, `../../pkg/router/router.go`).
- **Thread Safety & Race Freedom**: All concurrency checks must use atomic operations or synchronization primitives tested with `go test -race`.
- **Zero External Dependencies**: Pure Go standard library packages (`sync`, `sync/atomic`, `time`, `net/http`).
