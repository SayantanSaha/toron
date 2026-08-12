---
title: Domain-Based Virtual Host Routing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-020
  - TASK-020

derived_from:
  - REQ-020
  - ADR-015

documents:
  - DOMAIN-ROUTING-GUIDE

related_to:
  - index.md
  - header-routing.md
  - reverse-proxy.md
---

# Domain-Based Virtual Host Routing

## Overview

Toron supports domain-based (Virtual Host) routing by matching incoming `Host` headers against configured domain rules. This enables serving multiple distinct subdomains or domain names (`api.example.com`, `auth.example.com`) from a single listening TCP port.

## Configuration in `routes.yaml`

Add the `host` parameter under any route definition in `routes.yaml`:

```yaml
proxy:
  enabled: true
  routes:
    # Route requests for Host: api.toron.local
    - host: "api.toron.local"
      prefix: "/"
      algorithm: "round_robin"
      targets:
        - "http://localhost:9001"
        - "http://localhost:9002"

    # Route requests for Host: auth.toron.local
    - host: "auth.toron.local"
      prefix: "/"
      target: "http://localhost:9008"
```

## Programmatic API Usage

In Go, register domain routes using convenience helpers on `router.Router`:

```go
r := router.New()

// Register GET handler for specific host
r.GETHost("api.example.com", "/users", func(req *httpparser.Request, res *httpparser.Response) {
    res.SetStatus(200)
    res.WriteString(`{"status":"api_domain"}`)
})

// Register Proxy route with host matching
r.ProxyWithOptions("api.example.com", "/v1", nil, proxy.ProxyOptions{
    Targets: []string{"http://localhost:9001"},
})
```
