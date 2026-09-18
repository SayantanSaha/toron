# Toron [তোরণ · pronounced toh-ron · Bengali for gateway] - A next generation all in one gateway

**Toron** is an event-driven, high-performance, zero-dependency web server, reverse proxy API gateway, and edge security engine written in pure Go. Engineered with performance, enterprise security, and developer ergonomics as primary design goals, Toron features a non-blocking TCP reactor event loop, zero-copy HTTP/1.1 parsing, HTTP/2 stream multiplexing (`h2c`), HTTP/3 QUIC (UDP), zero-touch ACME production SSL issuance (Let's Encrypt / ZeroSSL), per-host mTLS, dynamic SNI, a Layer 7 Web Application Firewall (WAF), a 3-state Circuit Breaker, and background zero-downtime hot reloading via `fsnotify`.

---

## 📋 Executive Summary

Toron decouples infrastructure settings ([`config.yaml`](./config.yaml)) from routing rules ([`routes.yaml`](./routes.yaml)). It functions simultaneously as a static site host, reverse proxy gateway, gRPC router, Layer 4 TCP/UDP load balancer, and edge WAF security gateway.

* **Core Architecture**: Event-driven TCP reactor engine driving a thread-safe worker pool for concurrent request dispatching.
* **Protocols & Discovery**: HTTP/1.1, HTTP/2 Cleartext (`h2c`), HTTP/3 QUIC (UDP), WebSocket (RFC 6455 & RFC 8441), Layer 4 TCP/UDP, gRPC, and vendor-agnostic OCI container auto-discovery (Docker, Podman, Finch, Nerdctl).
* **Edge Security & Compliance**: OWASP Top 10 WAF inspection (SQLi, XSS, Path Traversal, RCE), custom regex rules, fast-path CIDR IP ACLs, constant-time authentication, CRLF header sanitization, OWASP security headers, and SSRF guards.
* **Resilience & Traffic Control**: Multi-algorithm load balancing (`round_robin`, `random`, `sticky_cookie`, `ip_hash`), active HTTP/gRPC health probing, 3-state Circuit Breaker, RFC 7234 response caching, and Token Bucket rate limiting.
* **Observability & Management**: Built-in Web Control Center & Security Audit Dashboard UI (`/internal/dashboard/`), Prometheus `/metrics` endpoint, SIEM-ready JSON audit logger, and W3C `traceparent` OpenTelemetry header propagation.

---

## 🌟 Feature Breakdown & Sample Configurations

Below is the complete catalog of Toron features with dedicated configuration snippets for each capability.

---

### 1. Event Reactor Engine & Concurrency Worker Pool

Toron handles concurrent connections using an event-driven non-blocking socket loop paired with a configurable worker pool.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  host: "0.0.0.0"
  port: 8080
  worker_pool_size: 128       # Number of concurrent worker threads
  read_timeout: 5s            # Max time to read request headers and body
  write_timeout: 5s           # Max time to write response
  idle_timeout: 30s           # Keep-alive socket idle timeout
  upgrade_idle_timeout: 60s   # Upgraded WebSocket/tunnel idle teardown deadline (SEC-27)
  max_header_bytes: 8192      # 8 KB header limit
  max_body_bytes: 4194304     # 4 MB payload body limit
```

---

### 2. Multi-Protocol Engine (HTTP/1.1, HTTP/2 `h2c` & HTTP/3 QUIC)

Toron supports non-blocking HTTP/1.1, HTTP/2 cleartext (`h2c`) prior-knowledge upgrade connections, and HTTP/3 QUIC over UDP with automatic `Alt-Svc` header injection.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  http2:
    enabled: true
    max_concurrent_streams: 250
    max_frame_size: 16384
    allow_h2c: true           # Allow HTTP/2 Cleartext prior-knowledge connections
  http3:
    enabled: true             # Enable HTTP/3 QUIC protocol listener
    port: 8443                # QUIC UDP listener port
    alt_svc_header: true      # Automatically add Alt-Svc: h3=":8443" headers
```

---

### 3. HTTPS TLS Encryption & Auto Dev Certificate Generator

Supports TLS 1.2/1.3 with ALPN negotiation (`h2`, `http/1.1`). If certificate files are omitted in development mode, Toron automatically generates an in-memory self-signed ECDSA certificate.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  tls:
    enabled: true
    cert_file: ""             # Path to custom X.509 cert (optional)
    key_file: ""              # Path to custom private key (optional)
    auto_dev_cert: true       # Auto-generate self-signed ECDSA certificate for dev
```

---

### 4. ACME Zero-Touch SSL Management (Let's Encrypt / ZeroSSL)

Automates production SSL/TLS certificate issuance and background renewal using ACME HTTP-01 or TLS-ALPN-01 (`acme-tls/1`) challenge strategies with on-disk caching.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  acme:
    enabled: true
    directory_url: "https://acme-v02.api.letsencrypt.org/directory"
    email: "admin@company.com"
    domains:
      - "api.company.com"
      - "company.com"
    cache_dir: "./certs"
    challenge_type: "http-01" # "http-01" or "tls-alpn-01"
```

---

### 5. WebSocket Protocol Upgrade & Bi-Directional Tunneling

Supports WebSocket upgrades over HTTP/1.1 (RFC 6455 `101 Switching Protocols`) and HTTP/2 Extended CONNECT protocol (RFC 8441 `:protocol = websocket`) with full bi-directional stream proxying, hardened with strict inactivity deadlines against denial-of-service ([`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392), CWE-400):

* **Bidirectional Activity-Refreshed Deadlines**: Eliminates indefinite blocking relays by applying `upgrade_idle_timeout` (default `60s`, configurable via `server.upgrade_idle_timeout`). Every chunk/frame transferred in either direction continually refreshes socket read and write deadlines.
* **Slowloris & Silent Drop Immunity**: Abandoned or silent connections are terminated cleanly via `sync.Once` double-socket shutdown upon timeout expiration, releasing file descriptors and unblocking both relay goroutines.
* **Transparent Heartbeat Support**: Legitimate long-lived sessions running RFC 6455 Ping/Pong frames or application-layer heartbeats automatically reset the inactivity deadline without extra configuration.
* **HTTP/2 Extended CONNECT Safety**: Streams monitor `upgrade_idle_timeout` and stream context cancellation (`r.Context().Done()`), promptly tearing down upstream sockets if a client aborts or resets the stream.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    prefix: "/ws"
    target: "http://localhost:9001"
```

---

### 6. Layer 4 TCP & UDP Transport Proxying

Provides raw socket stream forwarding (`type: "tcp"`) and connectionless datagram proxying (`type: "udp"`) with dedicated listener port binding, engineered with strict resource ceilings, zero external dependencies, and defense against denial-of-service ([`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383), CWE-400):

* **TCP Concurrency Limits & Fast-Fail Rejection**: Atomically tracks active client connections against `max_connections` (default `10,000`). Saturated connections are closed immediately upon `Accept()` with zero buffer allocation and zero upstream dial overhead.
* **Slowloris Immunity & Bidirectional Idle Deadlines**: Replaces unbounded blocking `io.Copy` stream relays with deadline-enforcing bidirectional transfer loops (`idle_timeout`, default `60s`). Active data transfer continuously refreshes socket deadlines; stagnant streams are terminated immediately, freeing socket file descriptors and unblocking relay routines. Cleanly handles TCP half-close (`CloseWrite`).
* **UDP Bounded Worker Pool & Saturated Queue Dropping**: Replaces unthrottled per-packet goroutines with a fixed-capacity task queue (`max_workers`, default `1,024`) serviced by persistent workers. Under flood saturation, excess datagrams are dropped fail-safe without memory growth or runtime scheduler panics.
* **Upstream UDP Socket Reuse & Client Session Cache**: Maintains a thread-safe registry (`sessions map[netip.AddrPort]*udpSession`) mapping client endpoints to active upstream sockets (`*net.UDPConn`). Subsequent datagrams from the same client reuse the open outbound socket, completely eliminating per-packet socket dials, ephemeral port exhaustion (`bind: address already in use`), and file descriptor starvation (`EMFILE`). A background sweeper terminates idle sessions after `idle_timeout`.
* **Zero-Allocation Buffer Recycling (`sync.Pool`)**: Datagram buffers (64 KB / 65,535 bytes) are recycled across inbound reads and upstream responses, eliminating per-packet heap allocations and GC latency spikes.
* **Deterministic Graceful Teardown**: Calling `Close()` immediately interrupts active streams, closes client and backend sockets, terminates worker pools, and completes within $\le 500\text{ms}$.
* **High-Throughput Benchmark Performance**: Steady-state forwarding delivers **~27,000 ops/sec at 0 allocs/op** for TCP streaming and **~21,700 pkts/sec at 0 buffer allocs** for UDP datagrams.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  # L4 TCP Socket Stream Proxy (Database / Redis / Binary Protocols)
  - type: "tcp"
    listen_port: 8090
    target: "127.0.0.1:9090"
    max_connections: 5000       # Max concurrent active TCP connections (default: 10000)
    idle_timeout: "60s"          # Stream inactivity teardown deadline (default: 60s)

  # L4 UDP Datagram Proxy (DNS / Syslog / Telemetry)
  - type: "udp"
    listen_port: 8091
    target: "127.0.0.1:9091"
    max_workers: 1024           # Max worker goroutines / queue capacity (default: 1024)
    idle_timeout: "60s"          # Client session idle eviction timeout (default: 60s)
```

---

### 7. Static File Serving with Symlink & Path Traversal Guards

Serves static web applications and assets with automatic MIME resolution, index page handling, and physical symlink target verification (`filepath.EvalSymlinks`) preventing directory escape attacks.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"
```

---

### 8. Reverse Proxy Gateway & Subpath Prefix Routing

Proxies incoming requests to backend microservice targets with automatic subpath prefix stripping and forwarding headers (`X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto`).

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
```

---

### 9. Domain-Based Virtual Host Matching

Dispatches incoming HTTP requests based on the `Host` header, allowing multi-tenant domain hosting on a single port listener.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    host: "api.toron.local"
    prefix: "/"
    target: "http://localhost:9001"
```

---

### 10. Conditional Header-Based HTTP Routing & API Versioning

Routes requests conditionally based on specific incoming HTTP request header key/value pairs (e.g., API versioning headers).

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  # API v2 Route
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"

  # API v1 Route
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v1"
    target: "http://localhost:9004"
```

---

### 11. Upstream Load Balancing & Session Affinity

Distributes request traffic across multiple backend targets using 4 load balancing algorithms: `round_robin`, `random`, `sticky_cookie`, and `ip_hash`.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  # Cookie-Based Sticky Session Affinity
  - type: "upstream"
    prefix: "/services/analytics"
    algorithm: "sticky_cookie"
    sticky_cookie_name: "TORON_STICKY"
    targets:
      - "http://localhost:9009"
      - "http://localhost:9010"

  # Client IP Hash Affinity
  - type: "upstream"
    prefix: "/services/sockets"
    algorithm: "ip_hash"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"
```

---

### 12. Active Upstream Health Probing & 3-State Circuit Breaker

Probes upstream target health periodically (`health_check_path`). If a target fails consecutive checks, the 3-state Circuit Breaker (`Closed` -> `Open` -> `HalfOpen`) trips and short-circuits traffic for a cooldown duration.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    prefix: "/services/cluster"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9005"
      - "http://localhost:9006"
      - "http://localhost:9007"
    health_check_path: "/health"
    health_check_interval: 5s
    consecutive_failures: 3
    cooldown_period: 15s
```

---

### 13. Token Bucket Rate Limiting Middleware

Defines per-route Token Bucket rate limits per client IP or API key, returning `429 Too Many Requests` with standard `Retry-After` headers upon threshold exceedance.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    prefix: "/api"
    rate_limit: "100/min"      # e.g., "10/sec", "100/min", "1000/hour"
    target: "http://localhost:9001"
```

---

### 14. gRPC Edge Gateway & Active Binary Health Probing

Routes gRPC microservice traffic over HTTP/2 while preserving binary trailers (`grpc-status`, `grpc-message`). Supports active binary health checks via `grpc.health.v1.Health/Check`.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    prefix: "/order.OrderService"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9005"
      - "http://localhost:9006"
    health_check_type: "grpc"
    health_check_service: "OrderService"
    health_check_interval: 5s
    consecutive_failures: 3
    cooldown_period: 15s
```

---

### 15. Per-Host Dynamic SNI & Mutual TLS (mTLS) Client Verification

Registers dedicated X.509 certificate pairs and client CA pools per virtual host domain, enforcing client certificate verification (`client_auth`) and minimum TLS versions.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    host: "secure.internal.local"
    prefix: "/"
    target: "http://localhost:9001"
    tls:
      cert_file: "./certs/server.crt"
      key_file: "./certs/server.key"
      ca_file: "./certs/ca.crt"
      client_auth: "require_and_verify" # "no_client_cert", "request_client_cert", "require_any_client_cert", "verify_client_cert_if_given", "require_and_verify"
      min_version: "tls1.3"
```

---

### 16. Streaming Response Compression (Zstd, Brotli, Gzip & Deflate)

Compresses outgoing response payloads automatically based on `Accept-Encoding` quality weighting (`q=`), using `sync.Pool` allocation reuse.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  compression:
    enabled: true             # Enable automatic response compression
    min_length: 512           # Minimum byte size threshold for compression
    level: -1                 # Compression level (-1 = default)
    encodings:
      - "zstd"
      - "br"
      - "gzip"
      - "deflate"
```

---

### 17. In-Memory Response Caching (RFC 7234)

Thread-safe in-memory cache for GET and HEAD requests with TTL expiration, `Cache-Control` (`no-store`/`no-cache`) validation, `Age` calculation, and `X-Cache: HIT/MISS` headers.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  cache:
    enabled: true             # Enable RFC 7234 response caching
    default_ttl: 60s          # Default cache TTL if max-age is omitted
    max_entries: 1000         # Max cached items in RAM
    max_payload_size: 1048576 # 1 MB maximum payload size per entry
```

---

### 18. Multi-Scheme Authentication Middleware (JWT, API Keys, Basic Auth)

Provides edge authentication supporting RFC 7519 JWT Bearer tokens (HS256/384/512), API keys, and Basic Auth with constant-time SHA-256 comparisons (`crypto/subtle`) and `X-Authenticated-User` header propagation.

**Sample Configuration (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
    auth:
      type: "api_key"
      api_key:
        keys:
          - "demo-api-key-12345"
```

**Global Auth Configuration (`config.yaml`)**:
```yaml
server:
  auth:
    type: "jwt"
    jwt:
      secret: "super-secret-jwt-signing-key"
      issuer: "toron-auth-service"
      audience: "toron-api-clients"
    excluded:
      - "/health"
      - "/public"
```

---

### 19. CORS Policies & Enterprise Security Headers

Short-circuits `OPTIONS` preflights (204 No Content), matches origin domain boundaries safely, and injects baseline OWASP security headers into all outgoing HTTP responses.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  cors:
    enabled: true
    allow_origins:
      - "https://app.company.com"
      - "https://*.company.com"
    allow_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "DELETE"
      - "OPTIONS"
    allow_headers:
      - "Origin"
      - "Content-Type"
      - "Authorization"
    expose_headers:
      - "X-Cache"
    allow_credentials: true
    max_age: 86400

  security_headers:
    enabled: true
    hsts: "max-age=31536000; includeSubDomains"
    content_type_options: "nosniff"
    frame_options: "DENY"
    referrer_policy: "strict-origin-when-cross-origin"
    csp: "default-src 'self'"
```

---

### 20. Web Application Firewall (WAF) & Custom Regex Rules + CIDR IP ACLs

Layer 7 WAF engine scanning URLs, query parameters, headers, and request bodies for OWASP Top 10 injection vectors (SQLi, XSS, Path Traversal, RCE). Supports custom user regex rules, `enforce` (403 block) vs `detection` (log-only) modes, and fast-path CIDR IP allow/deny lists.

**Sample Configuration (`config.yaml`)**:
```yaml
server:
  waf:
    enabled: true
    mode: "enforce"           # "enforce" (403 block) or "detection" (log-only)
    anomaly_threshold: 5      # Threat score limit above which request is blocked
    max_inspect_body_size: 65536 # 64 KB max body scanned
    allowed_ips:
      - "10.0.0.0/8"
      - "127.0.0.1"
    denied_ips:
      - "198.51.100.0/24"
    custom_rules:
      - id: "CUSTOM-001"
        category: "bot"
        description: "Block malicious scanners"
        pattern: "(?i)(sqlmap|nikto|nmap|acunetix)"
        score: 10
        locations: ["headers"]
    audit_log:
      enabled: true
      output: "stdout"        # "stdout", "stderr", or file path
      format: "json"
```

**Route-Level WAF Overrides (`routes.yaml`)**:
```yaml
routes:
  - type: "upstream"
    prefix: "/services/legacy-api"
    target: "http://localhost:9002"
    waf:
      enabled: true
      mode: "enforce"
      disabled_rules:
        - "SQLI-001"          # Disable specific rule for legacy endpoint
```

---

### 21. Prometheus Metrics & SIEM JSON Security Audit Logging

Exports standardized Prometheus metrics on `/metrics` (`toron_http_requests_total`, `toron_waf_blocked_requests_total`, latency histograms, QUIC gauges) and emits structured JSON audit logs for security incidents while propagating W3C `traceparent` OpenTelemetry headers.

**Sample Configuration (`config.yaml`)**:
```yaml
logging:
  level: "info"               # "debug", "info", "warn", "error"
  format: "text"              # "text" or "json"
```

---

### 22. Web Control Center & Security Audit Dashboard UI

Serves an interactive Web Control Center served at `/internal/dashboard/`.

Features:
* Dedicated Security & WAF Dashboard with threat counters and active compliance matrix.
* Real-time Security Incident Stream with XSS-sanitized payload details.
* Upstream Service Node Health Matrix (Ports 9001–9010).
* REST API Request Composer with live threat test presets.

**Sample Route Setup (`routes.yaml`)**:
```yaml
routes:
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"
```

---

### 23. Configuration Dry-Run Syntax Validator (`-t` / `-test-config`)

CLI flag validating YAML syntax, regex compilations, and file paths without starting server network listeners.

**Command Usage**:
```powershell
go run ./cmd/toron -t
```
**Output**:
```text
2026/08/16 09:12:00 [TORON] Configuration syntax OK: config.yaml and routes.yaml are valid.
```

---

### 24. Zero-Downtime Hot Reloading via `fsnotify`

Background file workers monitor `config.yaml` and `routes.yaml` file modifications, validating syntax and atomically swapping active routing tables and WAF rules in memory without dropping active TCP/UDP/QUIC/TLS sockets.

---

### 25. Testing & Dummy Microservices Cluster

Includes a standalone test cluster of 10 dummy upstream microservices running on ports 9001–9010 to verify load balancing, sticky sessions, rate limits, and circuit breakers.

**Command Usage**:
```powershell
# Start 10 dummy microservices in background terminal
go run ./dummy-services
```

---

### 26. Vendor-Agnostic OCI Container Auto-Discovery Engine (`pkg/discovery`)

Toron features a zero-dependency, vendor-agnostic OCI container auto-discovery engine that monitors Docker Engine (`/var/run/docker.sock`), Podman (`/run/podman/podman.sock`), Finch, and Nerdctl via Unix domain sockets in real time. It parses container metadata labels and dynamically registers/deregisters upstream backend targets in Toron's routing matrix with zero downtime.

**Sample Configuration (`config.yaml`)**:
```yaml
discovery:
  enabled: true
  engine: "auto"              # Options: "auto", "docker", "podman"
  socket_path: "auto"          # Auto-probes standard socket locations if "auto"
  poll_interval: 10s           # Fallback periodic scan interval
  default_weight: 1            # Default round-robin balancing weight
```

**Supported Container Label Taxonomy**:
- `toron.enable: "true"` (Required opt-in flag)
- `toron.host: "api.example.com"` (Host / domain routing rule)
- `toron.prefix: "/v1"` (Subpath prefix routing rule)
- `toron.port: "8080"` (Target container port)
- `toron.weight: "5"` (Load balancing weight)
- `toron.health_check: "/healthz"` (HTTP health probe path)

**CLI Container Launch Examples**:
```bash
# Launch a Docker container with Toron routing labels
docker run -d \
  --name user-service-1 \
  --label "toron.enable=true" \
  --label "toron.host=api.example.com" \
  --label "toron.prefix=/v1/users" \
  --label "toron.port=8080" \
  my-user-api:latest

# Launch a Podman container with Toron routing labels
podman run -d \
  --name order-service-1 \
  --label "toron.enable=true" \
  --label "toron.host=shop.example.com" \
  --label "toron.port=9090" \
  my-order-api:latest
```

---

### 27. Native Zero-Dependency Kubernetes Ingress Controller (`pkg/ingress`)

Toron operates natively as a Kubernetes Ingress Controller (`networking.k8s.io/v1`). Using pure Go stdlib HTTP & TLS, Toron connects to the Kubernetes API server via in-cluster ServiceAccount authentication, watches `Ingress`, `Service`, `Endpoints`, and `Secret` resources in real time, and dynamically synchronizes cluster routing rules and pod IP endpoints into Toron's core routing engine.

**Sample Configuration (`config.yaml`)**:
```yaml
ingress:
  enabled: true
  ingress_class: "toron"                            # Target ingress class name
  kube_apiserver: "https://kubernetes.default.svc"   # K8s API server endpoint
  service_account_dir: "/var/run/secrets/kubernetes.io/serviceaccount"
  resync_period: 30s                                # Fallback periodic resync interval
```

**Sample Ingress Resource Manifest**:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-ingress
  namespace: default
spec:
  ingressClassName: toron
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /v1
            pathType: Prefix
            backend:
              service:
                name: user-service
                port:
                  number: 8080
```

---

### 28. Service Mesh Sidecar Mode (`pkg/sidecar`)

Toron operates as a lightweight Service Mesh Sidecar proxy (`pkg/sidecar`) running alongside pod application containers (`127.0.0.1`). It transparently enforces pod-to-pod Mutual TLS (mTLS) encryption (`client_auth: "require_and_verify"`), weighted traffic splitting (e.g. 80/20 canary releases), and configurable request body limits (`max_body_bytes`, defaulting to 10MB) with immediate HTTP 413 Payload Too Large rejection on overflow, all with minimal memory overhead (<10MB RAM per pod).

**Sample Configuration (`config.yaml`)**:
```yaml
sidecar:
  enabled: true
  mode: "dual"              # Operational mode: "ingress", "egress", or "dual"
  ingress_port: 15006       # Inbound pod mTLS listener port
  egress_port: 15001        # Outbound pod proxy listener port
  app_port: 8080            # Target local app container port (127.0.0.1:8080)
  max_body_bytes: 10485760  # Maximum request body size in bytes (default: 10MB; HTTP 413 if exceeded)
  strict_mtls: true         # RequireAndVerifyClientCert mTLS verification
  traffic_splits:           # Weighted canary traffic distribution
    - prefix: "/api"
      backends:
        - target: "http://service-v1:8080"
          weight: 80
        - target: "http://service-v2:8080"
          weight: 20
```

---

### 29. REST-to-gRPC Transcoding Engine (`pkg/transcoder`)

Toron features a zero-dependency REST-to-gRPC Transcoding Engine ([`pkg/transcoder`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder)). It translates incoming RESTful JSON HTTP calls (e.g. `GET /v1/users/123`) into binary Protobuf-encoded HTTP/2 gRPC requests (e.g. `POST /user.UserService/GetUser`), extracts path/query params into JSON payload maps, and converts returning binary gRPC frames and `grpc-status` headers into REST JSON HTTP responses, hardened against Denial-of-Service, memory exhaustion, and routing interception ([`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401), [`SEC-29`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L403-L411), [`SEC-30`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L411-L419)):

* **Direct Parameterized Subpath Routing ([`SEC-30`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L411-L419), CWE-284 / CWE-400)**: Eliminates empty upstream reverse proxy registration; binds parameterized paths directly to `Router.HandlePrefixWithMatcher` with path pattern validation (`MatchPathPattern`), ensuring requests (e.g. `/v1/users/123`) route directly through `router.ServeHTTP` to gRPC backends without 502 Bad Gateway errors.
* **Hop-by-Hop Header Sanitization ([`SEC-29`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L403-L411), CWE-444 / CWE-436)**: Strips standard connection-specific headers (`Connection`, `Keep-Alive`, `Upgrade`, `Proxy-Connection`, `Transfer-Encoding`, `Proxy-Authenticate`, `Proxy-Authorization`, `Trailer`, `Trailers`, `Host`) and dynamic tokens from the `Connection` header before issuing gRPC calls.
* **Strict RFC 7540 `TE: trailers` Enforcement**: Discards client `TE` values (`gzip`, `deflate`) and strictly enforces single-valued `TE: trailers` (RFC 7540 §8.1.2.2 / RFC 9113 §8.2.2), preventing upstream gRPC backends from terminating streams with `RST_STREAM (PROTOCOL_ERROR 0x1)`.
* **Configurable Request Body Limit ([`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401), CWE-400 / CWE-770)**: Enforces a strict upper byte ceiling (`max_body_bytes`, default `4MB` / `4,194,304` bytes) via [`TranscoderConfig.MaxBodyBytes`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L130).
* **Declared `Content-Length` Fast-Fail**: Requests declaring `Content-Length` exceeding `max_body_bytes` are rejected immediately with `HTTP 413 Payload Too Large` without reading the stream or allocating heap memory.
* **Bounded Stream Over-Read**: Chunked or undeclared streams are bounded using `io.LimitReader(req.Body, maxBody+1)`. If the payload exceeds the limit, the server responds with `HTTP 413` and skips `json.Unmarshal`, eliminating heap explosion.
* **Upstream Backend Isolation**: The upstream gRPC server receives 0 requests on oversized rejections.

**Sample Configuration (`config.yaml`)**:
```yaml
transcoder:
  enabled: true
  max_body_bytes: 4194304     # 4 MB payload body ceiling (SEC-28)
  routes:
    - http_method: "GET"
      http_path: "/v1/users/:id"
      grpc_method: "/user.UserService/GetUser"
      upstream_url: "http://localhost:9005"
      field_mappings:
        id: "userId"
```

---

## 🛠️ Building & Running Toron

### Prerequisites
* **Go**: Version 1.20 or later.

### Building Binary
```powershell
# Build executable binary
go build -o toron ./cmd/toron
```

### Running Server
```powershell
# Run using default config.yaml & routes.yaml in working directory
./toron

# Or specify custom config file paths
./toron -config ./my-config.yaml -routes ./my-routes.yaml
```

### Running Test Suite
```powershell
# Run full unit test suite
go test -v ./...

# Run benchmarks
go test -bench=. -benchmem ./pkg/reactor ./pkg/httpparser ./pkg/router ./pkg/server
```

---

## 📄 Complete Infrastructure Configuration (`config.yaml`)

```yaml
# Toron Web Server Infrastructure Configuration File

server:
  host: "0.0.0.0"             # Listener IP address (0.0.0.0 for all interfaces)
  port: 8080                  # Server HTTP/HTTPS port
  worker_pool_size: 128       # Concurrent worker pool threads
  read_timeout: 5s            # Maximum time to read request headers/body
  write_timeout: 5s           # Maximum time to write response
  idle_timeout: 30s           # Keep-alive socket idle duration
  upgrade_idle_timeout: 60s   # Inactivity deadline for upgraded WebSocket/tunnel connections
  max_header_bytes: 8192      # 8 KB maximum header size limit
  max_body_bytes: 4194304     # 4 MB maximum request body size limit

  # HTTP/2 Stream Multiplexing & Cleartext h2c Engine Settings
  http2:
    enabled: true
    max_concurrent_streams: 250
    max_frame_size: 16384
    allow_h2c: true           # Allow HTTP/2 Cleartext (h2c) upgrade connections

  # HTTP/3 QUIC (UDP) Protocol Engine Settings
  http3:
    enabled: true             # Enable HTTP/3 QUIC protocol engine over UDP
    port: 8443                # HTTP/3 QUIC UDP listener port
    alt_svc_header: true      # Automatically advertise Alt-Svc: h3=":8443" headers

  # HTTPS TLS Encryption & ALPN Protocol Negotiation Settings
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    auto_dev_cert: true       # Auto-generate self-signed ECDSA certificate for development if TLS enabled

  # ACME Zero-Touch Production SSL Certificate Management (Let's Encrypt / ZeroSSL)
  acme:
    enabled: false            # Enable ACME automated SSL certificate issuance and background renewal
    directory_url: "https://acme-v02.api.letsencrypt.org/directory"
    email: "admin@toron.local"
    domains:
      - "api.toron.local"
      - "toron.local"
    cache_dir: "./certs"
    challenge_type: "http-01" # Challenge validation strategy: "http-01" or "tls-alpn-01"

  # Transparent Response Compression Settings (Zstd, Brotli, Gzip & Deflate)
  compression:
    enabled: true             # Enable automatic gzip/deflate response compression
    min_length: 512           # Minimum response byte threshold for compression
    level: -1                 # Compression level (-1 = default, 1 = best speed, 9 = best compression)
    encodings:
      - "zstd"
      - "br"
      - "gzip"
      - "deflate"

  # In-Memory HTTP Response Caching Settings
  cache:
    enabled: true             # Enable in-memory RFC 7234 response caching for GET/HEAD
    default_ttl: 60s          # Default cache expiration time if Cache-Control max-age is omitted
    max_entries: 1000         # Maximum number of response entries stored in memory
    max_payload_size: 1048576 # 1 MB maximum response body size per cached entry

  # Cross-Origin Resource Sharing (CORS) Settings
  cors:
    enabled: true             # Enable CORS preflight handling and header injection
    allow_origins:
      - "*"
    allow_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "DELETE"
      - "OPTIONS"
      - "PATCH"
    allow_headers:
      - "Origin"
      - "Content-Type"
      - "Accept"
      - "Authorization"
      - "X-Requested-With"
    expose_headers:
      - "X-Cache"
      - "Content-Length"
    allow_credentials: false
    max_age: 86400

  # Enterprise Browser Security Headers
  security_headers:
    enabled: true
    hsts: "max-age=31536000; includeSubDomains"
    content_type_options: "nosniff"
    frame_options: "DENY"
    referrer_policy: "strict-origin-when-cross-origin"
    csp: ""

  # Web Application Firewall (WAF) & Layer 7 OWASP Injection Protection Engine
  waf:
    enabled: true             # Enable WAF Layer 7 threat inspection middleware
    mode: "enforce"           # Evaluation mode: "enforce" (403 block) or "detection" (log-only score)
    anomaly_threshold: 5      # Threat score limit above which request is blocked
    max_inspect_body_size: 65536 # Maximum payload body bytes scanned (64 KB)
    # allowed_ips:            # Global CIDR IP allowlist (when set, non-matching IPs get 403)
    #   - "10.0.0.0/8"
    # denied_ips:             # Global CIDR IP denylist (matching IPs get 403)
    #   - "198.51.100.0/24"
    custom_rules:             # User-defined regex rules (hot reloaded dynamically via fsnotify)
      - id: "CUSTOM-001"
        category: "bot"
        description: "Block malicious scrapers and security scanners"
        pattern: "(?i)(sqlmap|nikto|nmap|acunetix)"
        score: 10
        locations: ["headers"]
    audit_log:
      enabled: true           # Enable structured JSON security audit logging
      output: "stdout"        # Destination: "stdout", "stderr", or file path (e.g. "./logs/security.log")
      format: "json"          # Output format (json)

  # REST-to-gRPC Transcoding Engine Settings
  transcoder:
    enabled: true             # Enable REST-to-gRPC transcoding
    max_body_bytes: 4194304   # 4 MB maximum request body payload limit (SEC-28)
    routes:
      - http_method: "GET"
        http_path: "/v1/users/:id"
        grpc_method: "/user.UserService/GetUser"
        upstream_url: "http://localhost:9005"
        field_mappings:
          id: "userId"

# Logging and Telemetry Output Settings
logging:
  level: "info"
  format: "text"
```

---

## 🛣️ Complete Routing Configuration (`routes.yaml`)

```yaml
# Toron Web Server Routing Configuration File (Static Sites, Upstream Reverse Proxies, Layer 4 TCP/UDP)

routes:
  # 1. Static Site Route - Control Center & Dashboard UI
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"

  # 2. Domain-Based Upstream Routing (Host: api.toron.local -> Dummy Services 1, 4, 6)
  - type: "upstream"
    host: "api.toron.local"
    prefix: "/"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9004"
      - "http://localhost:9006"

  # 3. Header-Based Upstream Routing + Token Bucket Rate Limiting (X-Version: v2, 100 req/min)
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    algorithm: "round_robin"
    rate_limit: "100/min"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"
      - "http://localhost:9003"

  # 4. Header-Based Upstream Routing + Single Target (X-Version: v1)
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v1"
    target: "http://localhost:9004"

  # 5. Load-Balanced Upstream Cluster + Active Health Checks & Circuit Breaker
  - type: "upstream"
    prefix: "/services/cluster"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9005"
      - "http://localhost:9006"
      - "http://localhost:9007"
    health_check_path: "/health"
    health_check_interval: 5s
    consecutive_failures: 3
    cooldown_period: 15s

  # 6. Upstream Routing + Cookie-Based Sticky Session Affinity
  - type: "upstream"
    prefix: "/services/analytics"
    algorithm: "sticky_cookie"
    sticky_cookie_name: "TORON_STICKY"
    targets:
      - "http://localhost:9009"
      - "http://localhost:9010"

  # 7. Upstream Routing + Client IP Hash Affinity
  - type: "upstream"
    prefix: "/services/sockets"
    algorithm: "ip_hash"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"

  # 8. Single Target Upstream Route + API Key Authentication
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
    auth:
      type: "api_key"
      api_key:
        keys:
          - "demo-api-key-12345"

  # 9. gRPC Service Route + Native grpc.health.v1 Health Prober
  - type: "upstream"
    prefix: "/order.OrderService"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9005"
      - "http://localhost:9006"
    health_check_type: "grpc"
    health_check_service: "OrderService"
    health_check_interval: 5s
    consecutive_failures: 3
    cooldown_period: 15s

  # 10. Per-Host Dedicated SSL/TLS & Mutual TLS (mTLS) Virtual Host Route
  - type: "upstream"
    host: "secure.internal.local"
    prefix: "/"
    target: "http://localhost:9001"
    tls:
      cert_file: "./certs/server.crt"
      key_file: "./certs/server.key"
      ca_file: "./certs/ca.crt"
      client_auth: "require_and_verify" # "no_client_cert", "request_client_cert", "require_any_client_cert", "verify_client_cert_if_given", "require_and_verify"
      min_version: "tls1.3"

  # 11. Layer 4 TCP Socket Stream Proxy Route (Listen Port 8090 -> TCP Backend 9090)
  - type: "tcp"
    listen_port: 8090
    target: "127.0.0.1:9090"
    max_connections: 5000       # Max active concurrent TCP connections (default: 10000)
    idle_timeout: "60s"          # Stream inactivity teardown deadline (default: 60s)

  # 12. Layer 4 UDP Datagram Proxy Route (Listen Port 8091 -> UDP Backend 9091)
  - type: "udp"
    listen_port: 8091
    target: "127.0.0.1:9091"
    max_workers: 1024           # Max worker goroutines / queue capacity (default: 1024)
    idle_timeout: "60s"          # Client session idle eviction timeout (default: 60s)

  # 13. Route-Level WAF Override & CIDR IP Access Control List (Allowlist & Denylist)
  - type: "upstream"
    prefix: "/services/secure-admin"
    target: "http://localhost:9001"
    waf:
      enabled: true
      mode: "enforce"
      allowed_ips:
        - "10.0.0.0/8"
        - "127.0.0.1"
      denied_ips:
        - "10.99.0.0/16"

  # 14. Route-Level WAF Rule Tuning (Legacy API with SQLI-001 disabled)
  - type: "upstream"
    prefix: "/services/legacy-api"
    target: "http://localhost:9002"
    waf:
      enabled: true
      mode: "enforce"
      disabled_rules:
        - "SQLI-001"
```

---

## 📜 License & Dual-Licensing Terms

Toron Web Server is dual-licensed under **GNU Affero General Public License v3.0 (AGPL-3.0)** and a **Commercial License**:

* **Open Source & Free Use (GNU AGPLv3)**: Free of charge for **Personal**, **Educational**, **Academic Research**, and **Open-Source** projects under the terms of the GNU Affero General Public License v3.0.
* **Commercial Use (Paid)**: Commercial entities, for-profit production deployments, or SaaS integration without AGPL-3.0 copyleft obligations require a separate paid **Commercial License Agreement**.

See the full [`LICENSE`](./LICENSE) file for legal details or contact `sayantan.somu@gmail.com` for commercial licensing terms.


