# Release Notes

## 2026-08-15 - Prototype 31 Release (Core Web Application Firewall Engine & OWASP Injection Protection)

### Added
- **Core WAF Engine (`pkg/waf`)**: Implemented high-performance, modular Web Application Firewall engine (`WAFEngine`) in `pkg/waf/waf.go` with `enforce` (HTTP `403 Forbidden` blocking) and `detection` (log-only threat anomaly score) modes (`TASK-041`, `REQ-041`).
- **OWASP Top 10 Injection Protection**: Pre-compiled regex rule set in `pkg/waf/rules.go` targeting SQL Injection (`SQLI-001`, `SQLI-002`, `SQLI-003`), Cross-Site Scripting (`XSS-001`, `XSS-002`, `XSS-003`), Path Traversal / LFI (`TRAVERSAL-001`, `TRAVERSAL-002`), and Command Injection / RCE (`RCE-001`, `RCE-002`).
- **Multi-Location Request Inspection**: Inspects URL paths, raw query parameters, HTTP request headers, and payload bodies (bounded by `max_inspect_body_size`, restoring `req.Body` for downstream handlers).
- **Router Middleware Adapter**: Created `NewWAFMiddleware` in `pkg/waf/middleware.go` and integrated into `cmd/toron/main.go`, `pkg/config/config.go`, `config.yaml`, and `routes.yaml`.

### Related Tasks
- `TASK-041`: Implement Core Web Application Firewall Engine and OWASP Injection Protection Middleware

## 2026-08-14 - Prototype 30 Release (Configurable CORS Policies & Enterprise Security Headers)

### Added
- **CORS Middleware**: Implemented `CORSMiddleware` in `pkg/router/cors.go` with fast-path `OPTIONS` preflight `204 No Content` handling, origin wildcard/subdomain matching, credential policies, and exposed headers (`TASK-040`, `REQ-040`).
- **Enterprise Security Headers Middleware**: Added `SecurityHeadersMiddleware` in `pkg/router/security_headers.go` injecting `Strict-Transport-Security` (HSTS), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `Content-Security-Policy`, and `Permissions-Policy`.
- **Global & Route Configuration**: Supported configuring CORS and Security Headers in `config.yaml` (`server.cors`, `server.security_headers`) and per-route in `routes.yaml`.

### Related Tasks
- `TASK-040`: Implement Configurable CORS Policies and Enterprise Security Headers Middleware

## 2026-08-14 - Prototype 29 Release (Per-Host Dynamic SNI Certificate Mapping & mTLS Client Auth)

### Added
- **Dynamic SNI Multi-Certificate Registry**: Added `SNIRegistry` in `pkg/server/sni.go` with `tls.Config.GetConfigForClient` hook, dynamically mapping hostnames to dedicated X.509 certificate pairs (`TASK-039`, `REQ-039`).
- **Mutual TLS (mTLS) Client Verification**: Added per-host client certificate verification with configurable policies (`require_and_verify`, `verify_client_cert_if_given`, `request_client_cert`) and custom client CA pools (`ca_file`).
- **Per-Host Minimum TLS Version**: Supported enforcing `tls.min_version` (`tls1.2`, `tls1.3`) per route in `routes.yaml`.

### Related Tasks
- `TASK-039`: Implement Per-Host SNI Dynamic Certificate Dispatching and mTLS Client Auth

## 2026-08-14 - Prototype 28 Release (Native gRPC Health Checking Protocol & HTTP/2 Trailers Gateway)

### Added
- **Native `grpc.health.v1.Health` Prober**: Added binary Protobuf frame encoder/decoder in `pkg/proxy/grpc_health.go` supporting active `grpc.health.v1.Health/Check` background health probing over HTTP/2 (`TASK-038`, `REQ-038`).
- **HTTP/2 Trailers Gateway Preservation**: Forwarded upstream trailing headers (`grpc-status`, `grpc-message`, `grpc-status-details-bin`) through the reverse proxy to downstream clients.
- **gRPC Route Configuration**: Added `health_check_type: "grpc"` and `health_check_service: "<name>"` options to `routes.yaml` and `ProxyRouteConfig`.

### Related Tasks
- `TASK-038`: Implement Native gRPC Health Checking Prober and HTTP/2 Trailers Preservation

## 2026-08-14 - Prototype 27 Release (Next-Gen Response Compression: Brotli & Zstandard)

### Added
- **Brotli & Zstandard Compression**: Extended `CompressionMiddleware` in `pkg/router/compression.go` to support Brotli (`br`, RFC 7932) and Zstandard (`zstd`, RFC 8878) (`TASK-037`, `REQ-037`).
- **Encoder Object Pooling**: Implemented `sync.Pool` allocation recycling for `brotli.Writer` and `zstd.Encoder`.
- **Quality Factor Content Negotiation**: Added RFC 7231 quality factor weighting (`q=`) parsing and modern server ranking (`zstd` > `br` > `gzip` > `deflate`).

### Related Tasks
- `TASK-037`: Implement Brotli (`br`) and Zstandard (`zstd`) Response Compression with Encoders Pooling

## 2026-08-14 - Prototype 26 Release (Multi-Scheme Authentication: JWT, API Key, and Basic Auth)

### Added
- **Multi-Scheme Auth Middleware**: Added `AuthMiddleware` in `pkg/router/auth.go` supporting RFC 7519 JWT verification (HS256/HS384/HS512), API key authentication, and RFC 7617 HTTP Basic authentication (`TASK-036`, `REQ-036`).
- **Timing Attack Resistance**: Used `crypto/subtle.ConstantTimeCompare` across all signature and credential comparisons.
- **Route & Global Integration**: Supported configuring authentication per route rule in `routes.yaml` or globally in `config.yaml`, injecting `X-Authenticated-User` headers into upstream requests.

### Related Tasks
- `TASK-036`: Implement Multi-Scheme Authentication Middleware (JWT HS256, API Key, and HTTP Basic Auth)

## 2026-08-14 - Prototype 25 Release (In-Memory HTTP Response Caching & Cache-Control)

### Added
- **In-Memory Response Caching**: Added `ResponseCache` and `NewCacheMiddleware` in `pkg/router/cache.go` providing thread-safe in-memory caching for idempotent GET and HEAD requests (`TASK-035`, `REQ-035`).
- **RFC 7234 Cache-Control Engine**: Parsed `max-age`, `no-store`, `no-cache`, `private`, and `public` directives; supported client refresh bypasses (`Cache-Control: no-cache`).
- **Diagnostics & Age Headers**: Injected `X-Cache: HIT` / `X-Cache: MISS` telemetry indicators and calculated `Age: <seconds>` headers.
- **Memory Bounding & Eviction**: Added memory bounds via `max_entries` and `max_payload_size` in `config.yaml` (`server.cache`).

### Related Tasks
- `TASK-035`: Implement In-Memory Response Caching Engine, Cache-Control Parser, and Diagnostics Headers

## 2026-08-14 - Prototype 24 Release (Streaming Response Compression: Gzip & Deflate)

### Added
- **Transparent Response Compression**: Added `CompressionMiddleware` in `pkg/router/compression.go` using Go standard library `compress/gzip` and `compress/flate` (`TASK-034`, `REQ-034`).
- **sync.Pool Writer Allocation Reuse**: Implemented object pooling for gzip and flate writers to achieve zero-allocation buffer reuse during high-concurrency requests.
- **Config & Protocol Safety**: Added `server.compression` settings to `config.yaml` (`enabled`, `min_length`, `level`, `encodings`, `types`) and safeguarded WebSocket 101 upgrades and binary media types against compression.

### Related Tasks
- `TASK-034`: Implement Transparent HTTP Response Compression Middleware (Gzip & Deflate)

## 2026-08-14 - Prototype 23 Release (ACME Zero-Touch Production SSL & TLS-ALPN-01)

### Added
- **ACME Engine**: Added `ACMEManager` in `pkg/acme/acme.go` for zero-touch SSL certificate issuance and background renewal (`TASK-033`, `REQ-033`).
- **HTTP-01 & TLS-ALPN-01 Responders**: Implemented HTTP-01 token authorization responder (`/.well-known/acme-challenge/*`) and TLS-ALPN-01 responder (`acme-tls/1` ALPN negotiation with OID `1.3.6.1.5.5.7.1.31`).
- **Disk Caching & Key Security**: Implemented secure disk certificate and key caching in `cache_dir` with `0600`/`0700` POSIX permissions.

### Related Tasks
- `TASK-033`: Implement ACME Engine, HTTP-01/TLS-ALPN-01 Responders, and Certificate Caching

## 2026-08-13 - Prototype 22 Release (Telemetry Metrics in Control Plane JSON Endpoints)

### Added
- **JSON Telemetry Integration**: Added `GetSummaryJSON()` in `pkg/metrics/metrics.go` exporting total requests, active QUIC streams, active TCP connections, circuit breaker trips, and status/method breakdowns (`TASK-032`, `REQ-032`).
- **Control Plane API**: Updated `GET /internal/api/status` and registered `GET /internal/api/metrics` returning structured JSON metrics.

### Related Tasks
- `TASK-032`: Expose Comprehensive Metrics in /internal/api/ Control Plane Endpoints

## 2026-08-13 - Prototype 21 Release (Prometheus Metrics & W3C Traceparent Propagation)

### Added
- **Prometheus Metrics Exporter**: Implemented `/metrics` endpoint returning Prometheus exposition format (`text/plain; version=0.0.4`) with request counters, latency histograms, and QUIC stream gauges (`TASK-031`, `REQ-031`).
- **W3C Distributed Tracing**: Added W3C `traceparent` context header extraction, generation, and upstream propagation in `pkg/metrics/tracing.go` and `pkg/proxy/proxy.go`.

### Related Tasks
- `TASK-031`: Implement Prometheus Metrics Registry and W3C Traceparent Header Propagation

## 2026-08-13 - Prototype 20 Release (Sticky Session Load Balancing: sticky_cookie & ip_hash)

### Added
- **Session Affinity Balancers**: Added `StickyCookieBalancer` (cookie-based session affinity with `Set-Cookie` injection) and `IPHashBalancer` (client IP hash affinity) in `pkg/proxy/sticky.go` (`TASK-030`, `REQ-030`).
- **Config & Router Integration**: Supported `algorithm: "sticky_cookie"` and `algorithm: "ip_hash"` in `routes.yaml` and `pkg/proxy/proxy.go`.

### Related Tasks
- `TASK-030`: Implement Sticky Session Load Balancing (sticky_cookie and ip_hash)

## 2026-08-13 - Prototype 19 Release (Token Bucket Rate Limiting Middleware)

### Added
- **Token Bucket Rate Limiter**: Added `TokenBucket`, `RateLimiter`, and `NewRateLimitMiddleware` in `pkg/router/rate_limiter.go` for route-level DDoS protection (`TASK-029`, `REQ-029`).
- **Client Key Extraction**: Extracted client identity via `X-API-Key`, `Authorization`, `X-Forwarded-For`, or remote IP with `429 Too Many Requests` and `Retry-After` header returns.

### Related Tasks
- `TASK-029`: Implement Token Bucket Rate Limiting Middleware per Client IP and API Key

## 2026-08-13 - Prototype 18 Release (Dynamic Route Hot Reloading via fsnotify)

### Added
- **Route Hot Reloading**: Integrated `github.com/fsnotify/fsnotify` in `pkg/config/watcher.go` (`RouteWatcher`) to automatically watch `routes.yaml` edits and update routing tables dynamically without dropping socket connections (`TASK-028`, `REQ-028`).
- **Atomic Router Reset**: Added `Router.Reset()` in `pkg/router/router.go` for zero-downtime routing table reloading under write lock.

### Related Tasks
- `TASK-028`: Implement Hot Reloading of Routes via File-Watch Worker (fsnotify)

## 2026-08-13 - Prototype 17 Release (HTTP/3 Protocol Engine & QUIC Transport)

### Added
- **HTTP/3 QUIC Transport**: Integrated `github.com/quic-go/quic-go/http3` engine into `pkg/server/server.go` (`TASK-027`, `REQ-027`).
- **Alt-Svc Protocol Advertising**: Injected `Alt-Svc: h3=":8443"` headers on HTTP/1.1 and HTTP/2 response headers for automatic browser HTTP/3 upgrades.

### Related Tasks
- `TASK-027`: Implement HTTP/3 Protocol Engine and QUIC Transport Handler

## 2026-08-13 - Prototype 16 Release (Layer 4 TCP & UDP Transport Proxying)

### Added
- **L4 TCP Socket Proxy**: Added `TCPProxy` in `pkg/proxy/tcp.go` for raw socket stream forwarding and round-robin load balancing (`TASK-026`, `REQ-026`).
- **L4 UDP Datagram Proxy**: Added `UDPProxy` in `pkg/proxy/udp.go` for connectionless datagram packet proxying.

### Related Tasks
- `TASK-026`: Implement Layer 4 TCP and UDP Transport Proxying

## 2026-08-12 - Prototype 15 Release (HTTPS TLS Encryption & Dev Certificate Generator)

### Added

- **HTTPS TLS Listener**: Added `ListenAndServeTLS` and `CreateTLSConfig` in `pkg/server/tls.go` and `pkg/server/server.go` (`TASK-023`, `REQ-023`).
- **ALPN HTTP/2 Negotiation**: Integrated TLS ALPN protocol negotiation (`h2`, `http/1.1`) mapping encrypted HTTP/2 streams to the server core.
- **Auto Self-Signed Dev Certs**: Implemented `GenerateDevCert()` providing zero-configuration ECDSA P-256 self-signed certificates for `localhost` development.
- **HTTPS Unit Test Suite**: Added `TestServer_HTTPSSelfSigned` and `TestServer_HTTPSWithALPNHTTP2` in `pkg/server/tls_test.go`.

### Related Tasks

- `TASK-023`: Implement HTTPS TLS Encryption, ALPN Negotiation, and Dev Cert Generator

## 2026-08-12 - Prototype 14 Release (HTTP/2 Protocol Support & Stream Multiplexing)

### Added

- **HTTP/2 Engine Integration**: Integrated `golang.org/x/net/http2` server engine into `pkg/server/server.go` (`TASK-022`, `REQ-022`).
- **Connection Preface Auto-Detection**: Added connection preface detection (`PRI * HTTP/2.0...`) in `handleConn`, enabling zero-downtime hybrid HTTP/1.1 and HTTP/2 cleartext (`h2c`) stream handling.
- **HTTP/2 Configuration**: Added `http2` settings (`enabled`, `max_concurrent_streams`, `max_frame_size`, `allow_h2c`) to `config.yaml` and `pkg/config`.
- **HTTP/2 Integration Tests**: Added `TestServer_HTTP2PriorKnowledge` unit test in `pkg/server/http2_test.go`.

### Related Tasks

- `TASK-022`: Implement HTTP/2 Server Connection Handler and Configuration Options

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
