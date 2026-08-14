---
title: gRPC Edge Gateway & Health Probing
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-038
  - TASK-038
  - ADR-033

derived_from:
  - REQ-038

documents:
  - FEATURE-GRPC-GATEWAY

related_to:
  - ../configuration.md
  - ./circuit-breaker.md
  - ../index.md
---

# gRPC Edge Gateway & Native Health Probing

## Overview

Toron functions as a high-performance **gRPC Edge Gateway**, routing, load balancing, authenticating, and monitoring backend gRPC microservices over HTTP/2 (`h2` and `h2c`). It features native **gRPC Health Checking Protocol (`grpc.health.v1.Health`)** support and strict **HTTP/2 Trailers preservation** (`grpc-status`, `grpc-message`, `grpc-status-details-bin`).

## Key Features

1. **Native `grpc.health.v1.Health/Check` Probing**:
   - Active background health worker sends binary Protobuf framed health check RPCs to upstream gRPC servers over HTTP/2.
   - Evaluates `ServingStatus == SERVING (1)` and `grpc-status == 0` to maintain healthy node pools and trigger circuit breakers.
2. **HTTP/2 Trailers Preservation**:
   - Forwarding pipeline guarantees that upstream trailing headers (`grpc-status`, `grpc-message`, `grpc-status-details-bin`) are delivered to gRPC client applications.
3. **Service & Method Prefix Routing**:
   - Route traffic by gRPC package, service name, or individual RPC method (e.g. `prefix: "/order.OrderService/"`).
4. **Traffic Management**:
   - Apply Round-Robin, Random, or IP Hash load balancing, token bucket rate limiting, and JWT/API key authentication to gRPC endpoints.

## Configuration in `routes.yaml`

```yaml
routes:
  # Upstream gRPC Microservice Route with Native gRPC Health Probing
  - type: "upstream"
    prefix: "/order.OrderService"
    algorithm: "round_robin"
    targets:
      - "http://127.0.0.1:50051"
      - "http://127.0.0.1:50052"
    health_check_type: "grpc"            # Enable native gRPC health check
    health_check_service: "OrderService" # Target service name (or empty for server check)
    health_check_interval: 5s
    consecutive_failures: 3
    cooldown_period: 15s
    rate_limit: "1000/sec"
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [Circuit Breaker & Health Checks](./circuit-breaker.md)
- [Load Balancing & Session Affinity](./load-balancing.md)
