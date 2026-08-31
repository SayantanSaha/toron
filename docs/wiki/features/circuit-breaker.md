---
title: Upstream Health Check and Circuit Breaker
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-014
  - TASK-014

derived_from:
  - REQ-014
  - ADR-009

documents:
  - CIRCUIT-BREAKER-FEATURE

related_to:
  - index.md
  - load-balancing.md
  - reverse-proxy.md
  - configuration.md
---

# Upstream Health Check and Circuit Breaker

## Overview

Toron includes an **Active Health Checker** and **3-State Circuit Breaker** (`pkg/proxy`) to protect gateway proxy routes from dispatching traffic to offline or degraded upstream microservices.

## Circuit Breaker States

- **`Closed` (Healthy)**: Target is operating normally. Requests and health probes succeed.
- **`Open` (Tripped)**: Target has reached $N$ consecutive failures. Requests immediately skip this target.
- **`HalfOpen` (Probing Recovery)**: After `cooldown_period` elapses, trial probes test target recovery. If successful, state resets to `Closed`.

## Configuration in `routes.yaml`

```yaml
routes:
  - type: "upstream"
    prefix: "/api"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"
    health_check_path: "/health"
    health_check_interval: 5s
    consecutive_failures: 3
    cooldown_period: 10s
```
