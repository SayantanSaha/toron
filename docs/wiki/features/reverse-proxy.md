---
title: Reverse Proxy and Gateway Routing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-009
  - TASK-009

derived_from:
  - REQ-009
  - ADR-004

documents:
  - REVERSE-PROXY-FEATURE

related_to:
  - index.md
  - configuration.md
---

# Reverse Proxy and Gateway Routing

## Overview

Toron includes a native **Reverse Proxy** engine (`pkg/proxy`), allowing it to route incoming client traffic to upstream backend HTTP microservices.

## Features

- **Upstream Forwarding**: Forwards request methods, query parameters, HTTP headers, and streaming request bodies.
- **Proxy Header Injection**: Injects standard origin headers (`X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto`, `X-Forwarded-Prefix`, `X-Real-IP`, and W3C `traceparent`).
- **Automatic 3xx Redirect Rewriting**: Intercepts upstream `Location` redirect headers (`301`, `302`, `303`, `307`, `308`) and automatically prepends the route prefix (`/login` $\rightarrow$ `/api/login`), preventing 404s on prefix-routed legacy applications.
- **Set-Cookie Path Rewriting**: Automatically rewrites upstream `Set-Cookie: Path=/` attributes to `Path=<prefix>` to keep cookies properly scoped to the gateway route.
- **Configurable Strip Prefix**: Supports stripping the route prefix before dispatching upstream, or preserving the full path for native prefix-aware backends.
- **Resilient Fallback**: Returns `502 Bad Gateway` if the upstream server is offline or times out.

## Configuration in `routes.yaml`

```yaml
routes:
  # 1. Header routing + Load balancing across ports 9001-9003
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"
      - "http://localhost:9003"
    strip_prefix: true            # Strips /api before forwarding (default: true)
    rewrite_redirects: true       # Rewrites Location: /login -> /api/login (default: true)
    rewrite_cookie_path: true     # Rewrites Set-Cookie: Path=/ -> Path=/api (default: true)

  # 2. Legacy backend with automatic redirect and cookie path rewriting
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
    rewrite_redirects: true
    rewrite_cookie_path: true

  # 3. Native prefix-aware backend (preserving full path, disabling redirect rewrites)
  - type: "upstream"
    prefix: "/services/v2"
    target: "http://localhost:9009"
    strip_prefix: false
    rewrite_redirects: false
```

## Programmatic Route Registration

```go
r := router.New()

// Proxy all requests matching /api/v2/* to http://localhost:9090
if err := r.Proxy("/api/v2", "http://localhost:9090"); err != nil {
    log.Fatalf("Proxy error: %v", err)
}
```

## Related Pages

- [Configuration Options](../reference/config-options.md)
- [Troubleshooting](../troubleshooting.md)
