---
title: Header-Based HTTP Routing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-010
  - TASK-010

derived_from:
  - REQ-010
  - ADR-005

documents:
  - HEADER-ROUTING-FEATURE

related_to:
  - index.md
  - features/reverse-proxy.md
---

# Header-Based HTTP Routing

## Overview

Toron supports **Header-Based Routing**, allowing requests to be routed to specific handlers or reverse proxy upstreams based on HTTP request header names and values (e.g. `X-Version`, `X-Environment`, `X-Tenant-ID`).

## Common Use Cases

- **API Versioning**: Route `X-Version: v2` to new microservices while `X-Version: v1` routes to legacy endpoints.
- **Canary & A/B Testing**: Direct requests with `X-Canary: true` to experimental deployment targets.
- **Multi-Tenant Routing**: Direct `X-Tenant-ID: acme` requests to isolated database or server clusters.

## Configuration in `config.yaml`

```yaml
proxy:
  enabled: true
  routes:
    - prefix: "/api"
      headers:
        X-Version: "v2"
      target: "http://localhost:9092"
    - prefix: "/api"
      headers:
        X-Version: "v1"
      target: "http://localhost:9091"
```

## Programmatic Route Registration

```go
r := router.New()

// Register route conditional on X-Version: v2 header
r.GETHeader("/api/data", "X-Version", "v2", func(req *httpparser.Request, res *httpparser.Response) {
    res.Header.Set("Content-Type", "application/json")
    _, _ = res.WriteString(`{"version":"v2"}`)
})

// Register fallback route with no header condition
r.GET("/api/data", func(req *httpparser.Request, res *httpparser.Response) {
    res.Header.Set("Content-Type", "application/json")
    _, _ = res.WriteString(`{"version":"v1"}`)
})

// Register header-conditional reverse proxy route
_ = r.ProxyHeader("/api", "X-Env", "staging", "http://staging-server:9090")
```

## Related Pages

- [Reverse Proxy & Gateway Routing](./reverse-proxy.md)
- [Configuration Options](../reference/config-options.md)
