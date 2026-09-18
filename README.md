# Toron [তোরণ · pronounced toh-ron · Bengali for gateway]

> A security-first, high-performance, all-in-one gateway.

[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](./LICENSE)
[![Zero Dependencies](https://img.shields.io/badge/Dependencies-Zero%20(Pure%20Go%20Stdlib)-success)](./go.mod)
[![Documentation](https://img.shields.io/badge/Docs-sayantansaha.github.io%2Ftoron-brightgreen)](https://sayantansaha.github.io/toron/)

**Toron** is an event-driven, zero-dependency, ultra-high-performance web server, reverse proxy API gateway, and edge security engine written in pure Go. Built around a non-blocking TCP socket reactor event loop, Toron consolidates the entire cloud-native ingress edge into a single, memory-bounded, race-free binary. It delivers zero-copy HTTP/1.1 wire parsing, HTTP/2 stream multiplexing (`h2c` / TLS), HTTP/3 QUIC over UDP, WebSocket streaming (RFC 6455 & RFC 8441), Layer 4 TCP/UDP streaming proxying, and gRPC routing with trailer preservation. Enterprise-grade security is baked into its foundation: an Active Ingress Smuggling Firewall (RFC 9112 §7.1), an OWASP Top 10 Web Application Firewall (WAF) with custom regex rules and CIDR IP ACLs, automated zero-touch ACME SSL (Let's Encrypt / ZeroSSL), per-host mTLS, and dynamic SNI. With multi-algorithm load balancing, 3-state circuit breaking, RFC 9111 shared response caching with strict session boundary isolation, token-bucket rate limiting, Kubernetes Ingress translation, and OCI container auto-discovery, Toron provides an uncompromising, all-in-one edge data plane for modern distributed architectures.

---

## 📖 Documentation

Full documentation, architectural decision records (ADRs), configuration manuals, security threat models, and API references are published at:

👉 **[https://sayantansaha.github.io/toron/](https://sayantansaha.github.io/toron/)**

Explore the documentation portal for in-depth topic guides:
* [Getting Started Guide](https://sayantansaha.github.io/toron/)
* [Configuration Specification & Reference](https://sayantansaha.github.io/toron/)
* [Active Ingress Smuggling Firewall & Chunked Streaming](https://sayantansaha.github.io/toron/)
* [RFC 9111 Response Caching & Session Boundaries](https://sayantansaha.github.io/toron/)
* [Web Application Firewall (WAF) Engine](https://sayantansaha.github.io/toron/)
* [Architectural Decision Records (ADRs)](https://sayantansaha.github.io/toron/)

---

## 🚀 Getting Started

### 1. Installation

Toron is compiled as a self-contained, statically linked binary with **zero third-party runtime dependencies**.

#### Option A: Universal Auto-Installer (Recommended)

On Linux / macOS:
```bash
# Automated install script (downloads/builds binary, sets up /usr/local/bin/toron and systemd service)
sudo ./install.sh

# Or install remotely via curl:
curl -fsSL https://raw.githubusercontent.com/SayantanSaha/toron/master/install.sh | sudo bash
```

On Windows (PowerShell / Command Prompt as Administrator):
```bat
install.bat
```

#### Option B: Build from Source

Requires **Go 1.22+** installed on your system.

```bash
# Clone repository
git clone https://github.com/SayantanSaha/toron.git
cd toron

# Compile native binary into bin/toron
make build

# Or compile directly with standard Go toolchain:
CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/toron ./cmd/toron
```

To cross-compile for other architectures:
```bash
make build-linux-amd64    # Linux x86_64
make build-linux-arm64    # Linux aarch64 (AWS Graviton, Raspberry Pi)
make build-darwin-arm64   # macOS Apple Silicon
make build-windows-amd64  # Windows x86_64
```

#### Option C: Docker & Containerization

```bash
# Build lightweight scratch-based container image
docker build -t toron:latest .

# Run Toron container publishing HTTP (8080) and HTTP/3 QUIC (8443/udp)
docker run -d \
  --name toron-gateway \
  -p 8080:8080 \
  -p 8443:8443/udp \
  -v $(pwd)/config.yaml:/etc/toron/config.yaml:ro \
  -v $(pwd)/routes.yaml:/etc/toron/routes.yaml:ro \
  toron:latest

# Or launch with docker-compose:
docker-compose up -d
```

---

### 2. Configuration

Toron cleanly decouples **infrastructure & listener parameters** ([`config.yaml`](./config.yaml)) from **routing rules, traffic control & security policies** ([`routes.yaml`](./routes.yaml)).

#### Server Infrastructure Configuration (`config.yaml`)

Defines network listener bindings, worker thread concurrency, protocol parameters, ACME SSL certificates, logging, and metrics:

```yaml
server:
  host: "0.0.0.0"
  port: 8080                  # Cleartext HTTP listener port
  worker_pool_size: 128       # Bounded worker threads handling connection pool
  read_timeout: 5s            # Request header and body read deadline
  write_timeout: 5s           # Response transmission deadline
  idle_timeout: 30s           # HTTP keep-alive socket idle timeout
  max_header_bytes: 8192      # 8 KB header clamp
  max_body_bytes: 4194304     # 4 MB payload clamp

  # Multi-Protocol Listeners
  http2:
    enabled: true
    max_concurrent_streams: 250
    allow_h2c: true           # Support HTTP/2 cleartext prior-knowledge upgrade

  http3:
    enabled: true             # HTTP/3 QUIC listener
    port: 8443                # UDP port for HTTP/3
    alt_svc_header: true      # Automatically advertise Alt-Svc: h3=":8443"

  tls:
    enabled: true
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    auto_dev_cert: true       # Auto-generate self-signed ECDSA certificate in dev

  # RFC 9111 Shared In-Memory Response Caching
  cache:
    enabled: true
    default_ttl: 60s
    max_entries: 10000
    max_payload_size: 1048576 # 1 MB maximum per cached item

  # Observability & Metrics
  metrics:
    enabled: true
    path: "/metrics"          # Prometheus metrics endpoint
```

#### Declarative Routing Rules (`routes.yaml`)

Defines Layer 4 and Layer 7 route tables, reverse proxies, static sites, virtual hosts, and rate limits:

```yaml
routes:
  # 1. Reverse Proxy API Gateway with Round-Robin Load Balancing & Active Health Checks
  - type: "upstream"
    prefix: "/api/v1"
    algorithm: "round_robin"
    targets:
      - "http://127.0.0.1:9001"
      - "http://127.0.0.1:9002"
    health_check_path: "/health"
    health_check_interval: 5s
    rate_limit: "500/min"
    waf:
      enabled: true
      mode: "enforce"

  # 2. Domain-Specific Virtual Host (Port-Preserving & Disambiguated)
  - type: "upstream"
    host: "api.toron.local"
    prefix: "/"
    target: "http://127.0.0.1:9003"

  # 3. Static Web Application Root with Symlink Boundary Verification
  - type: "static"
    prefix: "/dashboard"
    dir: "./public"

  # 4. Layer 4 TCP Raw Socket Proxy (Database / Redis / Streaming)
  - type: "tcp"
    listen_port: 8090
    target: "127.0.0.1:9090"
    max_connections: 5000
    idle_timeout: "60s"

  # 5. Layer 4 UDP Datagram Proxy (DNS / Syslog / Telemetry)
  - type: "udp"
    listen_port: 8091
    target: "127.0.0.1:9091"
    max_workers: 1024
    idle_timeout: "60s"
```

---

### 3. Running Toron

```bash
# Verify and validate configuration syntax without starting listeners
./bin/toron -config config.yaml -routes routes.yaml -test-config

# Launch Toron server
./bin/toron -config config.yaml -routes routes.yaml

# Check running version and build commit
./bin/toron -version
```

#### Command-Line Flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `-config <path>` | `-c` | `config.yaml` | Path to server infrastructure YAML configuration file. |
| `-routes <path>` | `-r` | `routes.yaml` | Path to proxy routing YAML configuration file. |
| `-test-config` | `-t` | `false` | Validates configuration syntax and exits without launching sockets. |
| `-version` | `-v` | `false` | Displays version, Git commit hash, and build timestamp. |

---

## 🧩 Adding New Functionalities in a Modular Fashion

Toron was engineered from the ground up to be **strictly modular, extensible, and composable**. Components interact across well-defined interfaces without touching the physical network transport.

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│                               TORON MODULAR PIPELINE ARCHITECTURE                        │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  [Network Wire] ──► [pkg/reactor (TCP Loop)] ──► [pkg/httpparser (Zero-Copy Wire)]       │
│                                                            │                             │
│                                                            ▼                             │
│                           ┌──────────────────────────────────────────────────────────┐   │
│                           │        pkg/router Middleware Pipeline Chain              │   │
│                           ├──────────────────────────────────────────────────────────┤   │
│                           │  ► RecoveryMiddleware                                    │   │
│                           │  ► AccessLoggerMiddleware                                │   │
│                           │  ► RateLimiterMiddleware (Token Bucket)                  │   │
│                           │  ► WAFMiddleware (OWASP & Regex Engine)                  │   │
│                           │  ► CacheMiddleware (RFC 9111 Shared Memory Store)        │   │
│                           │  ► [YOUR CUSTOM MIDDLEWARE HERE]                         │   │
│                           └────────────────────────────┬─────────────────────────────┘   │
│                                                        │                                 │
│                                                        ▼                                 │
│                                           [Route Dispatcher: Exact / Prefix]             │
│                                                        │                                 │
│             ┌─────────────────────────┬────────────────┼────────────────────────┐        │
│             ▼                         ▼                ▼                        ▼        │
│    [pkg/proxy (L7 Upstream)]   [pkg/proxy (L4)]  [Static Files]         [Custom Handler] │
│    • Load Balancers            • TCP Proxy       • File Server          • Internal API   │
│    • Circuit Breaker           • UDP Proxy       • Symlink Guards       • Metrics / Ping │
│    • Chunked Ingress Firewall                                                            │
│                                                                                          │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

### 1. Writing Custom HTTP Middlewares (`pkg/router`)

Toron uses idiomatic function signatures for request handling:
```go
// HandlerFunc defines the core HTTP processing function
type HandlerFunc func(req *httpparser.Request, res *httpparser.Response)

// MiddlewareFunc wraps a downstream HandlerFunc to inspect or transform traffic
type MiddlewareFunc func(next HandlerFunc) HandlerFunc
```

To create and register a new middleware:
```go
package mymiddleware

import (
    "strings"
    "toron/pkg/httpparser"
    "toron/pkg/router"
)

// CustomAuthMiddleware verifies custom security tokens or tenant IDs
func CustomAuthMiddleware(secretKey string) router.MiddlewareFunc {
    return func(next router.HandlerFunc) router.HandlerFunc {
        return func(req *httpparser.Request, res *httpparser.Response) {
            token := req.Header.Get("X-Custom-Auth")
            if token != secretKey {
                res.SetStatus(401)
                res.Header.Set("Content-Type", "application/json")
                res.Body.WriteString(`{"error":"unauthorized"}`)
                return // Short-circuit pipeline
            }

            // Add contextual header and call downstream handler
            req.Header.Set("X-Tenant-Validated", "true")
            next(req, res)
        }
    }
}
```

Attach your middleware globally or to specific subrouters:
```go
r := router.New()
r.Use(mymiddleware.CustomAuthMiddleware("my-secret-key"))
```

---

### 2. Adding Custom Route Types & Protocol Handlers

Toron's router dispatches traffic based on `RouteType`. You can register custom path handlers directly:

```go
// Register a custom in-memory health endpoint
r.GET("/healthz", func(req *httpparser.Request, res *httpparser.Response) {
    res.SetStatus(200)
    res.Header.Set("Content-Type", "text/plain")
    res.Body.WriteString("OK")
})

// Register an exact match or prefix route with host disambiguation
r.Handle("POST", "/api/v2/compute", myComputeHandler)
```

---

### 3. Adding Custom Load Balancing Algorithms (`pkg/proxy`)

The reverse proxy subsystem abstracts load balancing strategies. To add a new algorithm (e.g. `weighted_round_robin` or `consistent_hash`):

1. Define the algorithm constant in [`pkg/proxy/loadbalancer.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/loadbalancer.go):
   ```go
   const AlgorithmWeightedRoundRobin LoadBalanceAlgorithm = "weighted_round_robin"
   ```
2. Implement the selection logic inside the load balancer factory:
   ```go
   func (lb *WeightedRoundRobinLB) Next(req *httpparser.Request) (*UpstreamTarget, error) {
       // Your deterministic, thread-safe selection logic here
   }
   ```
3. Expose the new algorithm name in [`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go) validation.

---

### 4. Adding Custom WAF Rules & Security Filters (`pkg/waf`)

Toron's Web Application Firewall evaluates requests across request headers, URI parameters, and body payloads:

1. Open [`pkg/waf/rules.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/rules.go).
2. Append your inspection rule to `DefaultRules`:
   ```go
   {
       ID:          "CUSTOM-001",
       Category:    "BusinessLogic",
       Description: "Block suspicious header patterns",
       Severity:    SeverityHigh,
       MatchFunc: func(req *httpparser.Request) bool {
           return strings.Contains(req.Header.Get("X-Debug-Probe"), "exploit")
       },
   }
   ```
3. Rules are automatically compiled and evaluated with zero allocation overhead during proxy request evaluation.

---

### 5. Extending Configuration Schemas (`pkg/config`)

To expose new configuration parameters in `config.yaml` or `routes.yaml`:
1. Add the fields to `Config` in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go).
2. Set secure, defensive default values in `DefaultConfig()`.
3. Add syntax and boundary checks in `Validate()` in [`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go).

---

### 6. The 4 Non-Negotiable Invariants to Preserve

Any new module, handler, or middleware contributed to Toron **MUST** uphold the 4 core architectural invariants:

1. **Zero External Dependencies**: Use exclusively the Go standard library (`net`, `io`, `sync`, `time`, `crypto`, etc.). No external 3rd-party dependencies are permitted in production paths.
2. **Core Reactor Modularity ([ADR-001](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-001.md))**: Middleware and handlers operate strictly on abstract `*httpparser.Request` and `*httpparser.Response` objects. Never access, cast, or manipulate the underlying `net.Conn` physical socket inside HTTP layers.
3. **Memory Boundedness & Zero-Alloc Fast Paths ([ADR-030](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-030.md), [ADR-129](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-129.md))**:
   - Reuse byte slices via `sync.Pool`.
   - Never spawn unthrottled goroutines per connection/request.
   - Enforce bounded task queues and payload size limits to protect against memory exhaustion ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)).
4. **Race-Free Concurrency**:
   - Protect shared mutable state with `sync.RWMutex` or atomic operations.
   - All code must pass `go test -race ./...` with zero data races.

---

## 🏛️ Codebase Structure & Architecture

```
toron/
├── cmd/
│   └── toron/                 # Main CLI application entry point (flags, lifecycle, signals)
│
├── pkg/                       # Modular Go Subsystems (Zero 3rd-Party Dependencies)
│   ├── acme/                  # Automatic SSL/TLS certificate client (HTTP-01 & TLS-ALPN-01)
│   ├── config/                # YAML configuration loader, validator, and live hot-reloader
│   ├── discovery/             # Dynamic OCI container auto-discovery (Docker/Podman sockets)
│   ├── httpparser/            # Zero-copy HTTP/1.1 wire parser & RFC 9112 chunked transfer engine
│   ├── ingress/               # Kubernetes Ingress controller (networking.k8s.io/v1 translator)
│   ├── logging/               # High-throughput JSON access & SIEM security audit logger
│   ├── metrics/               # Prometheus metrics collector & /metrics HTTP exposition
│   ├── proxy/                 # Reverse proxy, load balancers, circuit breaker & L4 stream proxies
│   ├── reactor/               # Non-blocking TCP socket event loop & bounded worker pool
│   ├── router/                # Route matching, Host:Port disambiguation & RFC 9111 response cache
│   ├── server/                # Multi-protocol server orchestrator (HTTP, HTTPS, H2C, H3 QUIC)
│   ├── sidecar/               # Service mesh sidecar proxy (transparent ingress/egress proxying)
│   ├── transcoder/            # gRPC-JSON protocol transcoder & trailer preservation engine
│   ├── version/               # Single-source-of-truth build and version metadata
│   └── waf/                   # Web Application Firewall (OWASP Top 10, CIDR ACLs, custom rules)
│
├── docs/                      # Comprehensive Architecture & Engineering Documentation
│   ├── architecture/          # Architecture Decision Records (ADR-001 through ADR-134)
│   ├── requirements/          # Formal Requirement Specifications (REQ-001 through REQ-134)
│   ├── tasks/                 # Actionable Work Breakdown Tasks (TASK-001 through TASK-157)
│   ├── testCases/             # Test Case Specifications (TC-001 through TC-134)
│   ├── codeReview/            # Formal Code Reviews (CR-001 through CR-134)
│   ├── securityReview/        # Threat Models & Vulnerability Audits (SR-001 through SR-134)
│   └── wiki/                  # Navigable User & Administrator Documentation Wiki
│
├── benchmarks/                # Performance, Stress & Concurrency Evaluation Suite
│   ├── wrk2/                  # High-load HTTP benchmark generator & latency histograms
│   ├── multihop/              # Multi-tier proxy latency & desynchronization testing
│   ├── docker-compare/        # Comparative benchmarks against NGINX, Envoy, Traefik, Caddy
│   ├── fuzzer/                # Generative coverage-guided differential wire fuzzing
│   ├── ablation/              # Component ablation & GC pause telemetry analyzers
│   └── results/               # Authoritative benchmark reports, logs, and GC traces
│
├── dummy-services/            # Mock microservices cluster for integration testing
├── public/                    # Static assets & Web Control Center / Dashboard UI
├── k8s/                       # Kubernetes manifests & Helm charts
├── config.yaml                # Default server infrastructure configuration
├── routes.yaml                # Default L4/L7 routing and proxy configuration
├── Makefile                   # Multi-platform build, test, benchmark, and run automation
├── install.sh                 # Linux / macOS universal auto-installation script
├── install.bat                # Windows PowerShell / CMD installation script
├── Dockerfile                 # Minimal, security-hardened scratch container definition
└── LICENSE                    # GNU AGPLv3 License specification
```

---

## ⚡ Feature Matrix

| Category | Capability | Standards & Protocols | Details |
| :--- | :--- | :--- | :--- |
| **Core Transport** | Event Reactor Loop | Pure Go non-blocking I/O | Fixed-capacity task queue with bounded worker threads. |
| **Protocols** | HTTP/1.1 | RFC 9112 / RFC 7230 | Zero-copy header/body parser with streaming chunk ingestion. |
| | HTTP/2 Cleartext (`h2c`) | RFC 7540 / RFC 9113 | Prior-knowledge connection upgrade & stream multiplexing. |
| | HTTP/3 QUIC | RFC 9000 / RFC 9114 | Low-latency UDP transport with automatic `Alt-Svc` discovery. |
| | WebSockets | RFC 6455 / RFC 8441 | Bi-directional streaming with activity-refreshed idle timeouts. |
| | gRPC & Trailers | gRPC over HTTP/2 | Transparent trailer preservation and gRPC health checks. |
| | Layer 4 TCP Proxy | TCP Stream Relay | Fast-fail connection clamping and deadline-enforcing transfers. |
| | Layer 4 UDP Proxy | UDP Datagram Relay | Session connection registry and buffer pool recycling (`sync.Pool`). |
| **Security & WAF** | Active Smuggling Firewall | RFC 9112 §7.1 | Re-framing normalization shielding backends from request smuggling. |
| | OWASP WAF Engine | OWASP Top 10 | SQLi, XSS, Path Traversal, RCE inspection with CIDR IP ACLs. |
| | Automated SSL (ACME) | RFC 8555 | Let's Encrypt / ZeroSSL HTTP-01 and TLS-ALPN-01 challenge automation. |
| | Dedicated & Dynamic TLS | TLS 1.2 / TLS 1.3 | Per-host certificates, dynamic SNI routing, and client mTLS verification. |
| | Authentication | Multi-Scheme | Constant-time validation for JWT, API Key, and HTTP Basic Auth. |
| **Traffic Control** | Routing Methods (9 Total)| Exact, Prefix, VHost, etc.| Domain-only wildcard matching and port-qualified exact matching. |
| | Load Balancing | 5 Algorithms | Round Robin, Least Connections, Latency, Sticky Cookie, IP Hash. |
| | Circuit Breaker | 3-State FSM | Closed, Open, Half-Open state machine with auto-cooldown. |
| | Response Caching | RFC 9111 | Host:Port authority isolation, dual-stage `Set-Cookie` stripping. |
| | Rate Limiting | Token Bucket | Configurable burst and sustained rate limiting per route. |
| **Cloud Native** | Kubernetes Ingress | `networking.k8s.io/v1` | Dynamic Ingress resource translation and canary annotations. |
| | OCI Auto-Discovery | Docker / Podman | Live container socket polling and automatic route generation. |
| | Service Mesh Sidecar | Ingress / Egress | Transparent microservice proxying with local loopback forwarding. |
| **Observability** | Prometheus Metrics | OpenMetrics | Native `/metrics` endpoint exporting latency, bytes, and status. |
| | Security Dashboard | Web UI | Live Web Control Center at `/internal/dashboard/`. |
| | Distributed Tracing | W3C `traceparent` | Automatic OpenTelemetry context propagation across hops. |

---

## 🧪 Verification, Testing & Benchmarking

Toron maintains an exhaustive verification suite containing unit tests, concurrency race tests, differential fuzzer oracles, and high-throughput soak benchmarks.

```bash
# Run the complete test suite across all 25 packages
make test

# Run all unit tests with Go race detector enabled (zero data races guaranteed)
go test -race -count=1 ./...

# Run performance benchmarks
make bench

# Execute coverage-guided differential fuzzer
go test -fuzz=FuzzChunkFraming ./pkg/httpparser
```

---

## 🤝 Contributing & Multi-Agent Governance

Toron follows a disciplined **multi-agent specification-driven engineering methodology**:

1. **Requirements First**: Every architectural change originates from an approved specification in `docs/requirements/REQ-XXX.md`.
2. **Actionable Tasks**: Decomposed into work packages in `docs/tasks/TASK-XXX.md`.
3. **Architecture Decisions**: Codified with trade-offs and Mermaid diagrams in `docs/architecture/ADR-XXX.md`.
4. **Test Specifications**: Formal test vectors designed in `docs/testCases/TC-XXX.md` prior to code completion.
5. **Rigorous Dual Reviews**: Every implementation requires independent, approved code reviews (`docs/codeReview/CR-XXX.md`) and threat-modeled security reviews (`docs/securityReview/SR-XXX.md`).
6. **Synchronized Documentation**: Updated user guides in `docs/wiki/` and release notes.

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for details on development setup, style guides, and PR requirements.

---

## 📜 License & Dual-Licensing Terms

Toron is dual-licensed under **GNU Affero General Public License v3.0 (AGPL-3.0)** and a **Commercial License**:

* **Open Source & Free Use (GNU AGPLv3)**: Free of charge for **Personal**, **Educational**, **Academic Research**, and **Open-Source** projects under the terms of the GNU Affero General Public License v3.0.
* **Commercial Use (Paid)**: Commercial entities, for-profit production deployments, or SaaS integrations without AGPL-3.0 copyleft obligations require a separate paid **Commercial License Agreement**.

See the full [`LICENSE`](./LICENSE) file for legal details or contact `sayantan.somu@gmail.com` for commercial licensing inquiries.
