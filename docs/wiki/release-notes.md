# Release Notes

## 2026-08-12 - Prototype 7 Release (Load Balancing)

### Added

- **Upstream Reverse Proxy Load Balancing**: Multi-target load balancing with pluggable `LoadBalancer` interface, thread-safe Round-Robin selection algorithm (`round_robin`), YAML `targets` & `algorithm` configuration, and `r.ProxyBalancer` router helpers (`TASK-011`, `REQ-011`).
- **Dummy Web Services Test Suite**: 10 dummy HTTP web services running on ports 9001–9010 (`dummy-services/`) for testing reverse proxy routing and load balancer upstream targets (`TASK-012`, `REQ-012`).
- **Upstream Health Check & Circuit Breaker**: Active HTTP health check probing (`health_check_path`), 3-state Circuit Breaker (`Closed`, `Open`, `HalfOpen`), automatic offline target filtering, and recovery cooldown management (`TASK-014`, `REQ-014`).
- **Full Proxy Route Integration Configuration**: Updated `config.yaml` with active proxy routing (`proxy.enabled: true`) mapping all path, header, single-target, and load-balanced routes to dummy services 9001–9010 (`TASK-013`, `REQ-013`).
- **Standardized REST Client Test File**: Created `test_endpoint.http` in root directory for one-click HTTP request execution across all native, static, header, path, and proxy endpoints (`TASK-015`, `REQ-015`).

### Related Tasks

- `TASK-011`: Reverse Proxy Load Balancer Implementation
- `TASK-012`: Add 10 Dummy Web Services for Upstream Testing
- `TASK-013`: Update config.yaml to Map Proxy Routes to Dummy Web Services
- `TASK-014`: Implement Upstream Health Check and Circuit Breaker
- `TASK-015`: Create test_endpoint.http REST Client Test File

## 2026-08-11 - Prototype 1, 2, 3, 4, 5 & 6 Release

### Added

- **Event Reactor Core Engine**: High-performance non-blocking TCP socket listener and connection event loop with worker pool dispatching (`TASK-001`, `REQ-001`).
- **HTTP/1.1 Protocol Parser**: Zero-copy streaming request parsing and formatted response serialization (`TASK-002`, `REQ-002`).
- **HTTP Router & Middleware**: URL routing, HTTP method dispatching, and middleware chain support (`TASK-003`, `REQ-003`).
- **Security Guards**: Request header size limits (8KB), body size limits (4MB), socket read/write timeouts, and panic recovery middleware (`TASK-004`, `REQ-005`).
- **Server Orchestration**: Command line entry point `cmd/toron/main.go` with `/health` and `/` endpoints and graceful shutdown handling (`TASK-005`, `REQ-001`).
- **Static File Serving**: Built-in static website hosting with MIME type detection, `index.html` resolution, and path traversal security guards (`TASK-006`, `REQ-006`).
- **Extensible Configuration System**: External configuration file support (`config.yaml`), CLI argument `-config` flag, and extensible `pkg/config` loader architecture (`TASK-007`, `REQ-007`).
- **Native Go Benchmarking Suite**: Standard Go `testing.B` benchmarks across `pkg/reactor`, `pkg/httpparser`, `pkg/router`, and `pkg/server` with memory allocation tracking (`TASK-008`, `REQ-008`).
- **Reverse Proxy & Upstream Gateway**: Built-in HTTP reverse proxy engine (`pkg/proxy`, `router.Proxy(prefix, target)`), forwarding requests, injecting `X-Forwarded-*` headers, and supporting YAML proxy routes (`TASK-009`, `REQ-009`).
- **Header-Based HTTP Routing**: Conditional route matching and proxy forwarding based on HTTP header key/value conditions (`r.GETHeader`, `r.ProxyHeader`, `proxy.routes[].headers`), supporting API versioning (`X-Version: v2`) and canary routing (`TASK-010`, `REQ-010`).

### Related Tasks

- `TASK-001`: Core Event Reactor Engine Implementation
- `TASK-002`: HTTP/1.1 Streaming Request Parser & Response Builder
- `TASK-003`: HTTP Router & Middleware Pipeline Implementation
- `TASK-004`: Security Guards, Request Limits & Connection Timeouts
- `TASK-005`: Toron Server Orchestration & Main Application Entry
- `TASK-006`: Static File Handler & Path Traversal Guard Implementation
- `TASK-007`: Extensible Config Loader Package & CLI Flag Integration
- `TASK-008`: Benchmark Suite Implementation for Core Server Packages
- `TASK-009`: Reverse Proxy Handler & Upstream Transport Implementation
- `TASK-010`: Header-Based Route Matching & Config Integration
