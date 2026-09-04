---
id: TASK-057
type: task
title: Implement HTTP-to-HTTPS Redirection with Route-Level Exemption Override
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-04
updated: 2026-09-04
depends_on: []
derived_from:
  - REQ-057
implements:
  - REQ-057
verified_by:
  - TC-057
decided_by:
  - ADR-052
related_to:
  - TASK-002
  - TASK-003
---

# TASK-057 - Implement HTTP-to-HTTPS Redirection with Route-Level Exemption Override

## Description

Implement cleartext HTTP-to-HTTPS 301 redirection on an auxiliary HTTP port (typically port 80) when TLS is active on the primary server port (typically port 443), while supporting a declarative route-level configuration override (`redirect_http: false`) in `routes.yaml` and `config.yaml` to exempt specific routes from redirection.

This ensures edge deployments can enforce end-to-end TLS encryption by default while selectively allowing unencrypted access for workflows such as automated ACME HTTP-01 challenge validations, internal health checks, or legacy webhooks without hardcoding paths into the core server binary.

## Scope & Implementation Breakdown

1. **Configuration Schema Extension (`pkg/config/config.go`)**:
   - Define `HTTPRedirectConfig` struct under `ServerConfig`:
     ```go
     type HTTPRedirectConfig struct {
         Enabled bool `yaml:"enabled" json:"enabled"`
         Port    int  `yaml:"port" json:"port"`
     }
     ```
   - Extend `ProxyRouteConfig` with:
     ```go
     RedirectHTTP *bool `yaml:"redirect_http,omitempty" json:"redirect_http,omitempty"`
     ```
   - Update YAML / JSON validation and defaults (`port: 80` when enabled and `<= 0`).

2. **Proxy Options Extension (`pkg/proxy/proxy.go`)**:
   - Extend `ProxyOptions` with:
     ```go
     RedirectHTTP *bool
     ```
   - Propagate `RedirectHTTP` when building route options.

3. **Routing Table & Override Evaluation (`pkg/router/router.go`)**:
   - Extend internal prefix route representation with `redirectHTTP *bool`.
   - Implement `ShouldRedirectHTTP(req *httpparser.Request) bool`:
     - Evaluates incoming request path and host against configured routes.
     - If matched route specifies `redirectHTTP != nil && !*redirectHTTP`, return `false` (do not redirect).
     - Otherwise, return `true` (redirect to HTTPS).

4. **Auxiliary HTTP Redirect Listener (`pkg/server/server.go` & `cmd/toron/main.go`)**:
   - Add helper in `pkg/server/server.go` or `cmd/toron/main.go` to construct the HTTP listener.
   - When a request is received on the HTTP listener:
     - Check `router.ShouldRedirectHTTP(req)`.
     - If `false`, dispatch directly to `router.ServeHTTP(req, res)` over HTTP.
     - If `true`, respond with `HTTP/1.1 301 Moved Permanently`:
       - Validate and sanitize `Host` header (prevent CRLF and open redirects).
       - Set `Location: https://<host><request_uri>`.
       - Set `Connection: keep-alive` (or `close`), `Content-Length: 0`.
   - Bind HTTP listener to the application's graceful shutdown context.

5. **Test Suite Verification**:
   - Unit tests for configuration parsing in `pkg/config/config_test.go`.
   - Unit tests for routing override and redirect helper in `pkg/router/router_test.go`.
   - Integration tests for auxiliary HTTP listener in `pkg/server/server_test.go`.
