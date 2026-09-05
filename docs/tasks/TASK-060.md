---
id: TASK-060
type: task
title: Implement Query Parameter Forwarding and Preservation Across HTTP/1.1, HTTP/2, and HTTP/3
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-060
implements:
  - REQ-060
verified_by:
  - TC-060
decided_by:
  - ADR-055
related_to:
  - TASK-019
  - TASK-058
  - TASK-059
---

# TASK-060 - Implement Query Parameter Forwarding and Preservation Across HTTP/1.1, HTTP/2, and HTTP/3

## Description

Fix query parameter omission by ensuring that incoming HTTP/2, HTTP/3, and sidecar adapter handlers accurately populate `QueryParams` and `URL.RawQuery` on `httpparser.Request`, and enhancing `pkg/proxy/proxy.go` to forward raw query parameters verbatim to upstream services.

## Scope & Implementation Breakdown

1. **Request Factory & Adapter (`pkg/httpparser/request.go`)**:
   - Implement `NewRequestFromStd(r *http.Request) *Request` to standardize adaptation from Go standard library `*http.Request` to Toron's `*httpparser.Request`, ensuring `r.URL.Query()` and `r.URL.RawQuery` are always populated.
   - Add `Query()` lazy helper method on `*Request`.

2. **Server & Sidecar Adapters (`pkg/server/server.go`, `pkg/sidecar/proxy.go`)**:
   - Update `http2AdapterHandler()` in `pkg/server/server.go` to use `NewRequestFromStd(r)` or populate `QueryParams`.
   - Update sidecar request adapter in `pkg/sidecar/proxy.go` to populate `QueryParams`.

3. **Proxy Query Parameter Resolution (`pkg/proxy/proxy.go`)**:
   - In `ServeHTTPWithPrefix`, resolve `clientQuery` from `req.URL.RawQuery` (fallback to `req.QueryParams.Encode()`).
   - Merge `targetURL.RawQuery` with `clientQuery` if target defines existing parameters.

4. **Access Logging Telemetry (`pkg/router/router.go`)**:
   - Use `req.RequestURI` in `AccessLoggerMiddleware` to ensure access logs capture full request query strings.

5. **Unit & Integration Tests (`pkg/proxy/proxy_test.go`, `pkg/server/server_test.go`)**:
   - Test query parameter forwarding for HTTP/1.1 and HTTP/2 requests.
   - Test query parameter merging with target-configured queries.
   - Test non-value flag query parameters.

6. **Production Deployment**:
   - Recompile Linux binary, deploy to remote server, reload Toron, and verify with curl requests.
