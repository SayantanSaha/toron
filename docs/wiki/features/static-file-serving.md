---
title: Static File Serving
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-006
  - TASK-006

derived_from:
  - REQ-006
  - TASK-006

documents:
  - STATIC-FILE-SERVING-FEATURE

related_to:
  - index.md
  - configuration.md
---

# Static File Serving

## Overview

Toron provides native static file serving capabilities, allowing developers to host single-page or multi-page web applications (HTML, CSS, JS, images, fonts) directly alongside REST API routes.

## Features

- **Automatic MIME Detection**: Injects accurate `Content-Type` response headers (`text/html`, `text/css`, `application/javascript`, `image/png`, etc.).
- **Directory Index Fallback**: Automatically serves `index.html` when a directory URL is requested (e.g. `http://localhost:8080/`).
- **Path Traversal Security**: Automatically blocks directory traversal attacks (`../`) and returns `403 Forbidden`.

## Configuration Example

In `routes.yaml`:

```yaml
routes:
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"
```

*(Alternatively, legacy static file hosting can also be configured under `static:` in `config.yaml`)*

## Programmatic Route Registration

```go
r := router.New()
r.Static("/", "./public")
```

## Related Pages

- [Configuration Options](../reference/config-options.md)
- [Troubleshooting](../troubleshooting.md)
