---
id: TASK-163
type: task
title: Implement Dynamic Upstream Target Context Propagation, Route Tagging, and Telemetry Trace Fidelity
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-23
updated: 2026-09-23

depends_on:
  - REQ-140

derived_from:
  - REQ-140

implements:
  - REQ-140

verified_by:
  - TC-140

decided_by:
  - ADR-140

related_to:
  - REQ-140
  - ADR-140
  - TC-140
  - REQ-135
  - REQ-139
  - TASK-162
---

# TASK-163 - Implement Dynamic Upstream Target Context Propagation, Route Tagging, and Telemetry Trace Fidelity

## 1. Overview & Objective

Decompose [`REQ-140`](../requirements/REQ-140.md) into concrete, testable engineering deliverables across [`pkg/proxy/proxy.go`](../../pkg/proxy/proxy.go), [`pkg/router/router.go`](../../pkg/router/router.go), [`cmd/toron/main.go`](../../cmd/toron/main.go), and [`public/app.js`](../../public/app.js).

The primary defect is that in the Toron Edge Gateway Control Center dashboard at `#/logs`, the **Upstream** column unconditionally renders `"gateway"` for all requests because the global telemetry tracing middleware in [`cmd/toron/main.go`](../../cmd/toron/main.go) hardcodes `Up: "gateway"`, and the reverse proxy does not propagate the load-balancer-selected upstream target address back to the request context. Furthermore, `Route` is recorded as the raw `req.Path` rather than the canonical matched route prefix/ID, breaking route-based log filtering.

---

## 2. Traceability

- **Requirement**: [`REQ-140`](../requirements/REQ-140.md) (Dynamic Upstream Target Resolution, Route Identification, and Live Request Log Telemetry Fidelity)
- **Architecture Decision**: [`ADR-140`](../architecture/ADR-140.md) (Context-Based Telemetry Ingestion and Dynamic Upstream Resolution Architecture)
- **Verification Suite**: [`TC-140`](../testCases/TC-140.md) (Verification Test Cases for Dynamic Upstream Target Logging and Trace Fidelity)

---

## 3. Engineering Breakdown

### 3.1 Upstream Target Context Key & Propagation ([`pkg/proxy/proxy.go`](../../pkg/proxy/proxy.go))
- Define a package-level context key type and exported getter/setter or context value accessor for the resolved upstream target socket:
  ```go
  type contextKey string
  const UpstreamTargetContextKey contextKey = "toron.upstream_target"
  ```
- In `ReverseProxy.ServeHTTPWithPrefix(req, res, prefix)`:
  - When `targetNode` is successfully selected by `p.Balancer.Next(req)` (or from `p.TargetURL`):
    - Extract the socket address (`targetNode.URL.Host`).
    - Attach it to the request context:
      ```go
      req.SetContext(context.WithValue(req.Context(), UpstreamTargetContextKey, targetNode.URL.Host))
      ```
  - If load balancing fails with 503 or 502, record `"unavailable"` or the attempted address.
  - Also provide an exported helper `GetUpstreamTarget(ctx context.Context) string` so consuming packages can retrieve the target safely without hardcoded strings.

### 3.2 Route Identification & Route Type Context Injection ([`pkg/router/router.go`](../../pkg/router/router.go))
- Define context keys in `pkg/router`:
  ```go
  type contextKey string
  const (
      MatchedRouteContextKey contextKey = "toron.matched_route"
      RouteTypeContextKey    contextKey = "toron.route_type"
      DestinationContextKey  contextKey = "toron.destination"
  )
  ```
- In `Router.ServeHTTP(req, res)`:
  - When matching a prefix route (`pr := &r.prefixRoutes[i]`):
    - Tag `req.SetContext` with:
      - Matched route prefix (e.g., `/kite/api`).
      - Route type (`upstream`, `static`, etc.).
      - Destination: For static routes, tag as `fmt.Sprintf("static (%s)", pr.prefix)` or `static`.
  - For unproxied built-in routes (like `/health`, `/metrics`, HTTP redirects, or 404/405):
    - Tag destination as `in-process` or `gateway`.
- Provide helper functions `GetMatchedRoute(ctx context.Context) string` and `GetDestination(ctx context.Context) string`.

### 3.3 Telemetry Tracing Middleware Ingestion ([`cmd/toron/main.go`](../../cmd/toron/main.go))
- Update the global telemetry middleware in `cmd/toron/main.go`:
  - After `next(req, res)` executes:
    - Check `proxy.GetUpstreamTarget(req.Context())`.
    - If non-empty, set `Up` to that socket address (e.g., `127.0.0.1:8080`).
    - If empty, check `router.GetDestination(req.Context())` (e.g., `static (/var/www)` or `in-process`).
    - Default to `"in-process"` or `"gateway"` only for internal unrouted handlers.
    - Set `Route`: Retrieve `router.GetMatchedRoute(req.Context())`. If present, use that canonical prefix (e.g., `/kite/api`), otherwise fall back to `req.Path`.
    - Set `Short`: If `reqHost != ""`, format as `reqHost + " " + routeStr`.

### 3.4 Dashboard Log Tailing & Hierarchical Filter Updates ([`public/app.js`](../../public/app.js))
- In `lgUpdate(D, force)`:
  - Ensure `e.up` renders the concrete upstream socket (e.g. `127.0.0.1:8080`), static destination, or `in-process`.
  - Enhance the route filtering check:
    ```javascript
    (state.lroute === 'all' || e.route === state.lroute || (e.short && e.short.includes(state.lroute)) || e.path.startsWith(state.lroute))
    ```
    allowing operators to select `/kite/api` and seamlessly see all subpath requests (`/kite/api/v1/trades`, `/kite/api/v1/business`).
- In `openDrawer('log', id)`:
  - Confirm the drawer shows the concrete `e.up` under `<dt>Upstream Node</dt><dd><code>${esc(e.up)}</code></dd>`.

### 3.5 Automated Verification Suite
- Unit tests in `pkg/proxy/proxy_test.go`: Verify `GetUpstreamTarget` extracts the target host on proxied requests.
- Unit tests in `pkg/router/router_test.go`: Verify `GetMatchedRoute` and route type context tagging.
- Integration tests in `pkg/server/internal_api_test.go`: Verify `GlobalTraceBuffer` records real upstream socket addresses rather than `"gateway"`.
- Frontend tests in `tests/dashboard_telemetry_test.js`: Verify upstream socket rendering and route subpath filtering.

---

## 4. Acceptance Criteria

- [ ] Proxied requests record the concrete backend host:port (e.g., `127.0.0.1:8080`) in `TraceLogEntry.Up`.
- [ ] Static routes record `static` or `static (<dir>)` in `TraceLogEntry.Up`.
- [ ] In-process routes record `in-process` in `TraceLogEntry.Up`.
- [ ] `TraceLogEntry.Route` reflects the matched route prefix, enabling dashboard route filtering for all subpath traffic.
- [ ] Upstream column in the Control Center dashboard renders actual upstream values instead of `"gateway"`.
- [ ] All Go tests and race conditions pass (`go test -race -count=1 ./...`).
- [ ] Zero third-party dependencies introduced.
- [ ] All document links strictly use relative links.
