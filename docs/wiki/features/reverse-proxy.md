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
- **Proxy Header Injection**: Injects standard origin headers (`X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto`, `X-Real-IP`).
- **Resilient Fallback**: Returns `502 Bad Gateway` if the upstream server is offline or times out.

## Configuration in `routes.yaml`

```yaml
routes:
  # Header routing + Round-Robin load balancing across ports 9001-9003
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"
      - "http://localhost:9003"

  # Path prefix routing + Single target on port 9008
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
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
