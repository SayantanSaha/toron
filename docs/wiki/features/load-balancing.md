---
title: Reverse Proxy Upstream Load Balancing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-011
  - TASK-011

derived_from:
  - REQ-011
  - ADR-006

documents:
  - LOAD-BALANCING-FEATURE

related_to:
  - index.md
  - reverse-proxy.md
  - configuration.md
---

# Upstream Load Balancing

## Overview

Toron includes native **Upstream Load Balancing** in its reverse proxy engine (`pkg/proxy`). It distributes incoming client HTTP requests across multiple upstream backend servers using configurable balancing algorithms.

## Supported Algorithms

- **`round_robin` (Default)**: Sequentially distributes incoming requests across configured target URLs.
- *Extensible design*: Pluggable `LoadBalancer` interface for adding custom algorithms (e.g. `random`, `least_connections`).

## Configuration in `config.yaml`

```yaml
proxy:
  enabled: true
  routes:
    - prefix: "/api"
      headers:
        X-Version: "v2"
      algorithm: "round_robin"
      targets:
        - "http://backend1:9092"
        - "http://backend2:9093"
```

## Programmatic Route Registration

```go
r := router.New()

// Round-Robin load balancing across multiple targets
targets := []string{"http://backend1:8080", "http://backend2:8080"}
if err := r.ProxyBalancer("/api", targets, proxy.AlgorithmRoundRobin); err != nil {
    log.Fatalf("Proxy balancer error: %v", err)
}
```
