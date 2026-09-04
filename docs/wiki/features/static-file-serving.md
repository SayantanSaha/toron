---
title: Static File Serving
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-04

depends_on:
  - REQ-006
  - REQ-056
  - TASK-006
  - TASK-056

derived_from:
  - REQ-006
  - REQ-056
  - TASK-006
  - TASK-056
  - ADR-051

documents:
  - STATIC-FILE-SERVING-FEATURE

related_to:
  - index.md
  - configuration.md
---

# Static File Serving

## Overview

Toron provides native static file serving capabilities, allowing developers to host single-page or multi-page web applications (HTML, CSS, JS, images, fonts) directly alongside REST API routes without external reverse proxies.

## Features

- **Automatic MIME Detection**: Injects accurate `Content-Type` response headers (`text/html`, `text/css`, `application/javascript`, `image/png`, etc.).
- **Directory Index Fallback**: Automatically serves `index.html` when a directory URL is requested (e.g. `http://localhost:8080/`).
- **Single Page Application (SPA) HTML5 History Fallback**: Transparently resolves client-side virtual routes lacking file extensions to `index.html` (or a custom fallback document) with HTTP 200 OK, delivering full Nginx `try_files` parity (`TASK-056`).
- **Asset Masking Protection**: Missing static asset requests containing file extensions (`.js`, `.css`, `.png`, `.json`) return HTTP 404 Not Found rather than HTML, preventing runtime browser syntax errors.
- **Path Traversal Security**: Automatically blocks directory traversal attacks (`../`) and symlink directory escapes, returning `403 Forbidden`.

## Configuration Example

In `routes.yaml`:

```yaml
routes:
  # Standard Static Route
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"

  # Single Page Application (SPA) Route
  - type: "static"
    prefix: "/app"
    dir: "./frontend/dist"
    spa: true
    fallback: "index.html"
```

*(Alternatively, legacy static file hosting can also be configured under `static:` in `config.yaml`)*

## Programmatic Route Registration

```go
r := router.New()
// Standard static handler
r.Static("/", "./public")

// SPA-enabled static handler with ProxyOptions
r.RoutePrefix(router.RouteTypeStatic, "/app", "./frontend/dist", proxy.ProxyOptions{
    SPA:      true,
    Fallback: "index.html",
})
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [Configuration Options Reference](../reference/config-options.md)
- [Troubleshooting](../troubleshooting.md)
