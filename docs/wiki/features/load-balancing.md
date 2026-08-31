---
title: Reverse Proxy Upstream Load Balancing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-18

depends_on:
  - REQ-011
  - REQ-050
  - TASK-011
  - TASK-050

derived_from:
  - REQ-011
  - REQ-050
  - ADR-006
  - ADR-045

documents:
  - LOAD-BALANCING-FEATURE

related_to:
  - index.md
  - advanced-load-balancing.md
  - reverse-proxy.md
  - configuration.md
---

# Upstream Load Balancing

## Overview

Toron includes a native, high-performance **Upstream Load Balancing Engine** in `pkg/proxy`. It distributes incoming client HTTP requests across multiple upstream backend server targets using 8 built-in balancing strategies with active health checking and 3-state circuit breaking.

> [!TIP]
> For in-depth strategy guides, see the dedicated [Advanced Load Balancing Guide](./advanced-load-balancing.md).

## Supported Balancing Algorithms

| Algorithm Keyword | Strategy | Description |
| :--- | :--- | :--- |
| `round_robin` | Circular Round-Robin (Default) | Sequentially distributes requests across active targets. |
| `weighted_round_robin` | Weighted Round-Robin | Smooth weighted distribution based on target capacity weights. |
| `least_conn` | Fewest Active Connections | Routes requests to the upstream node with the fewest in-flight requests. |
| `weighted_least_conn` | Weighted Least Connections | Balances by `ActiveConns / Weight` for heterogeneous backend fleets. |
| `least_latency` | Exponential Moving Average (EMA) | Routes to the target with the lowest rolling response latency. |
| `random` / `weighted_random` | Randomized / Weighted Random | Selects target via pseudo-random probability distribution. |
| `sticky_cookie` | Cookie-Based Session Affinity | Binds client sessions to specific upstreams using sticky cookies. |
| `ip_hash` | Client IP Hash Persistence | Hashes client IP to ensure deterministic upstream routing. |

## Configuration Example (`routes.yaml`)

```yaml
routes:
  - type: "upstream"
    prefix: "/api/v1"
    algorithm: "least_latency"
    targets:
      - "http://backend1:8081"
      - "http://backend2:8082"

  - type: "upstream"
    prefix: "/app/session"
    algorithm: "sticky_cookie"
    targets:
      - "http://app1:9001"
      - "http://app2:9002"
```

## Programmatic Route Registration

```go
r := router.New()

// Load balancing across multiple targets with selected algorithm
targets := []string{"http://backend1:8080", "http://backend2:8080"}
if err := r.ProxyBalancer("/api", targets, proxy.AlgorithmLeastLatency); err != nil {
    log.Fatalf("Proxy balancer error: %v", err)
}
```
