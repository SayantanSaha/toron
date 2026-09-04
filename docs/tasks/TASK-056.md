---
id: TASK-056
type: task
title: Implement SPA HTML5 History Fallback Support for Static Routes
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-04
updated: 2026-09-04
depends_on: []
derived_from:
  - REQ-056
implements:
  - REQ-056
verified_by:
  - TC-056
decided_by:
  - ADR-051
related_to:
  - TASK-002
  - TASK-027
---

# TASK-056 - Implement SPA HTML5 History Fallback Support for Static Routes

## Description

Implement Single Page Application (SPA) HTML5 History fallback support for static routes in Toron. Modern web applications built using frameworks like React, Vue, Angular, and Svelte utilize client-side routing (`pushState`, `replaceState`). When a client navigates directly to a virtual route (e.g. `/kite/dashboard`, `/settings`, `/users/123`), the request reaches the server for a path that does not physically exist on disk. Without fallback support, the static file server returns HTTP 404 Not Found.

This task enables static routes configured in `routes.yaml` and `config.yaml` to specify `spa: true` and an optional custom `fallback` filename (defaulting to `index.html`). When a missing file path has no file extension (representing a client-side navigation request), the router transparently serves the fallback file with an HTTP 200 OK status and `Content-Type: text/html; charset=utf-8`. Requests for non-existent static assets with file extensions (such as `.js`, `.css`, `.png`) will continue to return HTTP 404 Not Found to prevent masking missing asset errors.

## Scope & Implementation Details

1. **Configuration Schema Extension (`pkg/config/config.go`)**:
   - Extend `ProxyRouteConfig` with:
     ```go
     SPA      bool   `yaml:"spa" json:"spa"`
     Fallback string `yaml:"fallback" json:"fallback"`
     ```
   - Ensure serialization and deserialization support both YAML and JSON definitions.

2. **Proxy Options Extension (`pkg/proxy/proxy.go`)**:
   - Extend `ProxyOptions` with:
     ```go
     SPA      bool
     Fallback string
     ```
   - Propagate these options from configuration callers through to route handlers.

3. **Static Route Handler & SPA Fallback Logic (`pkg/router/router.go`)**:
   - Update `RoutePrefix` to pass `opts.SPA` and `opts.Fallback` to `createStaticHandler`.
   - In `createStaticHandler`:
     - Determine if SPA mode is active (`opts.SPA == true || opts.Fallback != ""`).
     - Resolve the fallback filename, defaulting to `"index.html"` if `opts.Fallback` is empty.
     - When `os.Stat(targetPath)` returns a file not found error (`os.IsNotExist(err)`):
       - If SPA mode is enabled:
         - Inspect the requested relative path: check if `filepath.Ext(relPath) == ""`.
         - If no file extension is present:
           - Resolve the fallback file target path: `filepath.Join(absDir, fallbackFile)`.
           - Validate the fallback path against path traversal / symlink escapes within `absDir`.
           - If the fallback file exists on disk:
             - Read the fallback file data.
             - Respond with HTTP 200 OK and `Content-Type: text/html; charset=utf-8`.
             - If the HTTP method is `HEAD`, omit the body.
             - Return immediately.
           - If the fallback file does not exist, return HTTP 404 Not Found (or HTTP 403 if traversal fails).
         - If a file extension is present (`filepath.Ext(relPath) != ""`), return HTTP 404 Not Found.
       - If SPA mode is disabled, maintain existing behavior and call `r.NotFound(req, res)`.

4. **Command CLI Integration (`cmd/toron/main.go`)**:
   - In `cmd/toron/main.go`, when registering static routes with `r.RoutePrefix(router.RouteTypeStatic, ...)`, pass `pr.SPA` and `pr.Fallback` into `proxy.ProxyOptions`:
     ```go
     proxy.ProxyOptions{
         RateLimit: pr.RateLimit,
         Auth:      authCfg,
         WAF:       pr.WAF,
         SPA:       pr.SPA,
         Fallback:  pr.Fallback,
     }
     ```

5. **Unit & Regression Testing**:
   - Add unit tests in `pkg/config/config_test.go` verifying parsing of `spa` and `fallback` fields from YAML configuration.
   - Add unit tests in `pkg/router/router_test.go` verifying:
     - SPA fallback for virtual navigation paths without file extensions (e.g. `/app/dashboard`, `/app/users/42`) serving `index.html` with status 200 OK and `text/html; charset=utf-8`.
     - Missing static assets with extensions (e.g. `/app/bundle.js`, `/app/styles.css`, `/app/missing.png`) returning 404 Not Found.
     - Custom fallback filename (e.g. `fallback: "200.html"`).
     - Static routes with SPA disabled preserving standard 404 responses for non-existent paths without extensions.
     - Security: Path traversal attempts on fallback resolution fail safely.

## Acceptance Criteria

1. **Configuration Attributes**:
   - `ProxyRouteConfig` parses `spa` (bool) and `fallback` (string) from YAML and JSON.
   - If `fallback` is non-empty, SPA mode is automatically treated as active even if `spa: true` is not explicitly declared.
   - If `spa: true` is set without `fallback`, fallback defaults to `"index.html"`.

2. **Client-Side Virtual Route Fallback**:
   - Requests matching an SPA-enabled static route for non-existent files without a file extension serve the resolved fallback file with HTTP 200 OK and `Content-Type: text/html; charset=utf-8`.
   - Works across nested route prefixes (e.g. prefix `/dashboard` handling `/dashboard/settings/profile`).

3. **Asset Non-Masking Protection**:
   - Requests containing file extensions (`filepath.Ext(path) != ""`) for non-existent files return HTTP 404 Not Found and never return the fallback document.

4. **Security & Sandboxing**:
   - Fallback file resolution is verified to remain within the configured directory root, disallowing directory traversal.

5. **Backward Compatibility & Existing Behavior**:
   - Static routes without `spa` or `fallback` configured continue returning HTTP 404 Not Found for missing paths.
   - Existing physical files, assets, and directory index serving are unaffected.

## Verification Plan

- `go test -v ./pkg/config` to verify config parsing of `spa` and `fallback`.
- `go test -v ./pkg/router` to verify SPA fallback behavior, asset 404 protection, and custom fallback handling.
- `go test ./...` to verify zero regressions across the codebase.

## Rationale

Single Page Applications (SPAs) are standard across modern web architectures. Toron's built-in static file server can host SPAs directly without requiring an intermediate Nginx or Caddy reverse proxy. Preventing the fallback mechanism from serving HTML for missing `.js`, `.css`, and image assets prevents cryptic browser runtime syntax errors (such as `Uncaught SyntaxError: Unexpected token '<'`) when a browser tries to execute HTML as JavaScript.

## Constraints

- Only HTTP `GET` and `HEAD` methods are eligible for static file and fallback serving; other HTTP methods shall receive HTTP 405 Method Not Allowed.
- Fallback lookup must only execute when the initial target path does not exist on disk, avoiding I/O overhead on normal hits.
- Fallback file must reside within the configured static root directory.

## Open Questions

- None. REQ-056 clarifies that any request with a non-empty `filepath.Ext(relPath)` represents an asset and should return 404 on miss.
