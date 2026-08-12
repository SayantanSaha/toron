# Release Notes

## 2026-08-12 - Prototype 13 Release (Configuration Syntax Testing & Dry-Run CLI Flag)

### Added

- **Configuration Dry-Run Flag**: Added `-test-config` (`-t`) CLI option in `cmd/toron/main.go` to test YAML syntax and configuration parameters before starting the server (`TASK-021`, `REQ-021`).
- **Strict Validator**: Implemented `ValidateConfig(cfg *AppConfig) error` in `pkg/config/loader.go` checking port ranges, static file directory accessibility, load balancing algorithms, and target URL schemes (`http://` / `https://`).
- **CLI Reference**: Updated CLI reference guide (`docs/wiki/reference/cli.md`).

### Related Tasks

- `TASK-021`: Implement Configuration Test Option (-test-config / -t) and Strict Validator

## 2026-08-12 - Prototype 12 Release (Domain-Based Virtual Host Routing)

### Added

- **Domain-Based HTTP Routing**: Added Virtual Host matching support in `pkg/router/router.go` (`r.GETHost`, `r.HandleHostHeader`, `r.ProxyWithOptions` with host matching) (`TASK-020`, `REQ-020`).
- **YAML Schema Extension**: Supported `host` and `domain` parameters under proxy route configurations in `routes.yaml`.
- **Host Header Extraction & Sanitization**: Implemented `extractHost(req)` stripping optional port numbers and matching exact subdomains.
- **HTTP REST Test Suite**: Added domain-based Host header request examples in `test_endpoint.http`.

### Related Tasks

- `TASK-020`: Implement Domain-Based HTTP Routing and Proxy Forwarding

## 2026-08-12 - Prototype 11 Release (Decoupled Dual-File YAML Configuration)

### Added

- **Dual-File Configuration Architecture**: Split application configuration into `config.yaml` (server infrastructure, static assets, logging) and `routes.yaml` (reverse proxy routing rules, load balancers, health checks) (`TASK-019`, `REQ-019`).
- **Loader Enhancement**: Added `LoadFromFiles(configPath, routesPath)` in `pkg/config` with auto-discovery of `routes.yaml`.
- **CLI Flag Integration**: Added `-routes` (`-r`) CLI flag alongside `-config` (`-c`) in `cmd/toron/main.go`.

### Related Tasks

- `TASK-019`: Implement Dual-File YAML Config Loader (config.yaml & routes.yaml)

## 2026-08-12 - Prototype 10 Release (Configurable Static Prefix & Relative Asset Resolution)

### Added

- **Relative Asset Resolution**: Updated `public/index.html` to reference `./style.css` and `./app.js` using relative URL paths (`TASK-018`, `REQ-018`).
- **Subpath Prefix Trailing Slash Redirect**: Updated `pkg/router/router.go` static file handler to automatically emit a `302 Found` redirect when a request matches a configured static prefix (e.g. `/internal/dashboard`) without trailing slash, establishing correct browser base URL resolution.
- **Config Integration**: Tested and verified static prefix configuration (`prefix: "/internal/dashboard"`) in `config.yaml`.

### Related Tasks

- `TASK-018`: Implement Configurable Static Prefix Trailing Slash Redirect and Relative Asset Loading

## 2026-08-12 - Prototype 9 Release (Internal Management API Endpoints)

### Added

- **Internal Management API Namespace**: Added `/internal/api/` control plane endpoints (`status`, `routes`, `upstreams/health`, `proxy-test`) in `pkg/server/internal_api.go` (`TASK-017`, `REQ-017`).
- **Server-Side Upstream Probing**: Backend routines perform concurrent HTTP health checks against upstreams, returning node health states to the dashboard without requiring browser-to-upstream target calls.
- **Internal Proxy Test Dispatcher**: `POST /internal/api/proxy-test` accepts path/method/header payloads, executes proxy routes server-side, and returns status codes, latency, headers, and body payloads.
- **Frontend Integration**: Updated `public/app.js` to query `/internal/api/` endpoints exclusively.

### Related Tasks

- `TASK-017`: Implement /internal/api/ Management Endpoints and Update Frontend Client

## 2026-08-12 - Prototype 8 Release (Mobile-First Control Center & Dashboard)

### Added

- **Mobile-First Control Center & Proxy Dashboard**: Transformed `/public` frontend into a responsive Web Control Center and Proxy Dashboard built with HTML5, Vanilla JavaScript, and Tailwind CSS (`TASK-016`, `REQ-016`).
- **Real-Time Upstream Health Probing**: Dynamic client-side health check execution querying upstream service ports 9001-9010 and accurately flagging status 200 (Healthy `CLOSED`), status 500 (`OPEN (500 ERR)`), or connection failure (`UNREACHABLE`).
- **Interactive Live API & Proxy Route Tester**: Interactive request composer with header presets, custom header inputs, execution time calculation (in ms), status badges, and formatted JSON response preview.
- **CORS & Error Simulation in Dummy Services**: Added CORS headers (`Access-Control-Allow-Origin: *`) and 500 error route handling (`/500`, `/error`, `?fail=true`) in `dummy-services/services.go`.

### Related Tasks

- `TASK-016`: Implement Mobile-First Control Center and Proxy Dashboard UI

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
