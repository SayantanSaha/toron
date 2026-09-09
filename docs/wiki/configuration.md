---
title: Toron Configuration Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-09

depends_on:
  - REQ-007
  - REQ-019
  - REQ-027
  - REQ-034
  - REQ-035
  - REQ-036
  - REQ-056
  - REQ-086
  - REQ-087
  - TASK-007
  - TASK-019
  - TASK-027
  - TASK-034
  - TASK-035
  - TASK-036
  - TASK-056
  - TASK-090
  - TASK-093

derived_from:
  - REQ-007
  - REQ-027
  - REQ-056
  - ADR-002
  - ADR-022
  - ADR-051
  - ADR-082
  - SEC-26

documents:
  - CONFIGURATION-GUIDE

related_to:
  - index.md
  - features/http3.md
  - reference/config-options.md
  - reference/cli.md
---

# Toron Configuration Guide

## Overview

Toron utilizes a decoupled dual-file YAML configuration architecture separating infrastructure parameters ([`config.yaml`](../../config.yaml)) from application routing rules ([`routes.yaml`](../../routes.yaml)).

## Specifying Configuration Files

Pass paths to your server and routing configuration files using the `-config` (`-c`) and `-routes` (`-r`) command-line flags:

```bash
go run ./cmd/toron -config config.yaml -routes routes.yaml
```

If no flags are passed, Toron automatically checks for `config.yaml` and `routes.yaml` in the current working directory.

### Configuration Dry-Run Validation (`-t`)

Validate YAML configuration syntax before starting listeners:

```bash
toron -t
```

---

## 1. Infrastructure Configuration (`config.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  worker_pool_size: 128
  read_timeout: 5s
  write_timeout: 5s
  idle_timeout: 30s
  max_header_bytes: 8192
  max_body_bytes: 4194304

  # HTTP/2 Cleartext (h2c) and Stream Multiplexing
  http2:
    enabled: true
    max_concurrent_streams: 250
    allow_h2c: true

  # HTTP/3 QUIC (UDP) Protocol Engine
  http3:
    enabled: true
    port: 8443
    alt_svc_header: true

  # HTTPS TLS 1.2/1.3 Encryption
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    auto_dev_cert: true

  # Cleartext HTTP-to-HTTPS 301 Redirection
  http_redirect:
    enabled: true             # Starts auxiliary HTTP cleartext redirect listener
    port: 80                  # Cleartext listener port (default: 80)

  # ACME Zero-Touch Production SSL Certificate Management
  acme:
    enabled: false
    directory_url: "https://acme-v02.api.letsencrypt.org/directory"
    email: "admin@toron.local"
    domains:
      - "api.toron.local"
    cache_dir: "./certs"
    challenge_type: "http-01" # "http-01" or "tls-alpn-01"

  # Transparent Response Compression (Zstd, Brotli, Gzip & Deflate)
  compression:
    enabled: true
    min_length: 512
    level: -1
    encodings:
      - "zstd"
      - "br"
      - "gzip"
      - "deflate"

  # In-Memory HTTP Response Caching (RFC 7234)
  cache:
    enabled: true
    default_ttl: 60s
  # Enterprise Browser Security Headers
  security_headers:
    enabled: true
    hsts: "max-age=31536000; includeSubDomains"
    content_type_options: "nosniff"
    frame_options: "DENY"
    referrer_policy: "strict-origin-when-cross-origin"
    csp: ""

  # Cross-Origin Resource Sharing (CORS) Policy
  cors:
    enabled: true
    allow_origins:
      - "*"
    allow_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "DELETE"
      - "OPTIONS"
    allow_headers:
      - "Content-Type"
      - "Authorization"
      - "X-Version"
    expose_headers:
      - "X-Cache"
      - "X-Toron-WAF-Anomaly-Score"
    allow_credentials: false
    max_age: 86400

  # Web Application Firewall (WAF) & Layer 7 Threat Inspection
  waf:
    enabled: true
    mode: "enforce"           # "enforce" (403 block) or "detection" (log-only anomaly score)
    anomaly_threshold: 5
    max_inspect_body_size: 65536
    allowed_ips: []           # Optional global CIDR IP allowlist (e.g. ["10.0.0.0/8"])
    denied_ips: []            # Optional global CIDR IP denylist (e.g. ["198.51.100.0/24"])
    disabled_rules: []        # Optional list of rule IDs to bypass globally
    custom_rules:             # User-defined regex rules (hot reloaded dynamically via fsnotify)
      - id: "CUSTOM-001"
        category: "bot"
        description: "Block malicious scrapers and security scanners"
        pattern: "(?i)(sqlmap|nikto|nmap|acunetix)"
        score: 10
        locations: ["headers"]
    audit_log:
      enabled: true
      output: "stdout"        # Destination: "stdout", "stderr", or file path (e.g. "./logs/security.log")
      format: "json"

# Vendor-Agnostic OCI Container Auto-Discovery Engine
discovery:
  enabled: true
  engine: "auto"              # Options: "auto", "docker", "podman"
  socket_path: "auto"          # Auto-probes standard socket locations if "auto"
  poll_interval: 10s           # Fallback periodic scan interval
  default_weight: 1            # Default round-robin balancing weight

# Native Kubernetes Ingress Controller Engine
ingress:
  enabled: false             # Enable native Kubernetes Ingress Controller
  ingress_class: "toron"                            # Target ingress class name
  kube_apiserver: "https://kubernetes.default.svc"   # K8s API server endpoint
  service_account_dir: "/var/run/secrets/kubernetes.io/serviceaccount"
  resync_period: 30s                                # Fallback periodic resync interval

# Service Mesh Sidecar Mode Engine
sidecar:
  enabled: false             # Enable Service Mesh Sidecar mode
  mode: "dual"              # Operational mode: "ingress", "egress", or "dual"
  ingress_port: 15006       # Pod inbound mTLS listener port
  egress_port: 15001        # Pod outbound proxy listener port
  app_port: 8080            # Local app container target port (127.0.0.1:8080)
  max_body_bytes: 10485760  # Max request body size in bytes (default: 10MB; 413 rejection if exceeded)
  strict_mtls: false        # Enforce RequireAndVerifyClientCert mTLS
  traffic_splits:           # Weighted canary traffic splitting
    - prefix: "/api"
      backends:
        - target: "http://service-v1:8080"
          weight: 80
        - target: "http://service-v2:8080"
          weight: 20

# REST-to-gRPC Transcoding Engine
transcoder:
  enabled: true
  routes:
    - http_method: "GET"
      http_path: "/v1/users/:id"
      grpc_method: "/user.UserService/GetUser"
      upstream_url: "http://localhost:9005"
      field_mappings:
        id: "userId"

logging:
  level: "info"
  format: "text"
```

### HTTP/3 QUIC (UDP) Configuration (`server.http3`)

Toron provides native HTTP/3 (RFC 9114) protocol support over QUIC (RFC 9000 UDP transport). When TLS and HTTP/3 are enabled, Toron automatically initiates a concurrent UDP listener running `quic-go/http3` alongside the primary TCP listener.

#### Configuration Options

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `server.http3.enabled` | `boolean` | `true` | Enables or disables the HTTP/3 protocol engine and QUIC UDP socket listener. |
| `server.http3.port` | `integer` | `8443` | UDP port on which Toron listens for incoming HTTP/3 QUIC datagrams. If `<= 0`, defaults to `8443`. |
| `server.http3.alt_svc_header` | `boolean` | `true` | Automatically advertises HTTP/3 availability to HTTP/1.1 and HTTP/2 clients via `Alt-Svc: h3=":<port>"; ma=2592000` response headers. |

> [!NOTE]
> **Prerequisites for HTTP/3 QUIC**:
> - HTTP/3 strictly mandates TLS 1.3 encryption. Toron will start the HTTP/3 UDP listener only when both `server.tls.enabled: true` and `server.http3.enabled: true`.
> - The HTTP/3 QUIC listener uses the TLS certificate and private key configured under `server.tls` (`cert_file` and `key_file`, or automatically generated via `server.tls.auto_dev_cert: true`).
> - During server shutdown, Toron performs graceful socket drainage via `s.h3Server.Close()`, cleanly terminating QUIC streams without socket leaks.

#### Configuration Examples

##### 1. Production Dual HTTPS & HTTP/3 Setup (Port 443)
Accept incoming TLS TCP connections on standard port 443 (HTTP/1.1 and HTTP/2) and QUIC UDP datagrams on port 443 (HTTP/3), advertising automatic protocol upgrade to browsers:

```yaml
server:
  host: "0.0.0.0"
  port: 443

  tls:
    enabled: true
    cert_file: "/etc/toron/certs/fullchain.pem"
    key_file: "/etc/toron/certs/privkey.pem"
    auto_dev_cert: false

  http3:
    enabled: true
    port: 443
    alt_svc_header: true
```

##### 2. Local Development with Auto Dev Certificates
Run HTTPS and HTTP/3 on port 8443 using Toron's built-in zero-config ECDSA P-256 self-signed development certificate:

```yaml
server:
  host: "0.0.0.0"
  port: 8443

  tls:
    enabled: true
    auto_dev_cert: true       # Generates ECDSA localhost certificate

  http3:
    enabled: true
    port: 8443                # Listens on UDP :8443
    alt_svc_header: true      # Injects Alt-Svc: h3=":8443"; ma=2592000
```

##### 3. Custom UDP Port Mapping
Configure Toron with TCP HTTPS on port 443 while routing HTTP/3 QUIC traffic over a custom UDP port (e.g. 8443):

```yaml
server:
  host: "0.0.0.0"
  port: 443

  tls:
    enabled: true
    cert_file: "./cert.pem"
    key_file: "./key.pem"

  http3:
    enabled: true
    port: 8443                # Listens on UDP :8443
    alt_svc_header: true      # Injects Alt-Svc: h3=":8443"; ma=2592000
```

##### 4. Disabling HTTP/3 Engine
To disable HTTP/3 QUIC listener entirely and omit `Alt-Svc` discovery headers:

```yaml
server:
  host: "0.0.0.0"
  port: 443

  tls:
    enabled: true
    cert_file: "./cert.pem"
    key_file: "./key.pem"

  http3:
    enabled: false            # Disables UDP listener and Alt-Svc headers
```

---

## 2. Routing Configuration (`routes.yaml`)

```yaml
routes:
  # Static Site Route with Relative Asset Resolution
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"

  # Single Page Application (SPA) with HTML5 History Fallback (React / Vue)
  - type: "static"
    prefix: "/app"
    dir: "/var/www/react-app/dist"
    spa: true
    fallback: "index.html"

  # Route-Level HTTP Redirection Exemption (e.g. Automated ACME HTTP-01 Challenges or Public Webhooks)
  - type: "static"
    prefix: "/.well-known/acme-challenge"
    dir: "/var/www/challenges"
    redirect_http: false      # Direct cleartext HTTP serving without 301 redirect

  # Reverse Proxy with Load Balancing & Token Bucket Rate Limiting
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    algorithm: "round_robin"
    rate_limit: "100/min"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"

  # Reverse Proxy with Sticky Session Load Balancing
  - type: "upstream"
    prefix: "/services/analytics"
    algorithm: "sticky_cookie"
    sticky_cookie_name: "TORON_STICKY"
    targets:
      - "http://localhost:9009"
      - "http://localhost:9010"

  # Protected Upstream Route with API Key Authentication
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
    auth:
      type: "api_key"
      api_key:
        keys:
          - "secret-api-key-12345"

  # Protected Upstream Route with JWT Bearer Authentication
  - type: "upstream"
    prefix: "/services/admin"
    target: "http://localhost:9007"
    auth:
      type: "jwt"
      jwt:
        secret: "my-jwt-secret-key"
        issuer: "toron-auth"
        audience: "api.toron.local"

  # Layer 4 TCP Stream Proxy (Bounded Concurrency & Idle Deadlines)
  - type: "tcp"
    listen_port: 8090
    target: "127.0.0.1:9090"
    max_connections: 5000       # Max active concurrent TCP connections (default: 10000)
    idle_timeout: "60s"          # Stream inactivity teardown deadline (default: 60s)

  # Layer 4 UDP Datagram Proxy (Worker Pool, sync.Pool Buffers & Session Socket Reuse)
  - type: "udp"
    listen_port: 8091
    target: "127.0.0.1:9091"
    max_workers: 1024           # Max worker goroutines / queue capacity (default: 1024)
    idle_timeout: "60s"          # Client session idle eviction timeout (default: 60s)

  # Route-Level WAF Override & CIDR IP Access List
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

  # Route-Level WAF Rule Tuning (Legacy API with SQLI-001 disabled)
  - type: "upstream"
    prefix: "/services/legacy-api"
    target: "http://localhost:9002"
    waf:
      enabled: true
      mode: "enforce"
      disabled_rules:
        - "SQLI-001"
```

### Static Routes & Single Page Application (SPA) Fallback

Toron provides native static website and application hosting directly within the edge router. For modern web applications built using frameworks such as **React**, **Vue**, **Angular**, or **Svelte**, client-side routing uses the HTML5 History API (`pushState`, `replaceState`). When a browser user refreshes a deep virtual link (e.g. `/app/dashboard` or `/portal/settings`), the requested path does not exist as a physical file on the server's disk.

By configuring `spa: true` and optional `fallback: "<filename>"`, Toron provides native fallback routing without requiring external web servers or secondary proxy containers.

#### Configuration Options

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `type` | `string` | *(Required)* | Set to `"static"` to serve static files from disk. |
| `prefix` | `string` | `"/"` | URL route prefix matching incoming client requests (e.g. `"/app"`, `"/portal"`). |
| `dir` | `string` | *(Required)* | Local filesystem root directory containing static assets (e.g. `"./dist"`, `"/var/www/app"`). |
| `spa` | `boolean` | `false` | Enables Single Page Application (SPA) HTML5 History fallback for virtual navigation paths. |
| `fallback` | `string` | `"index.html"` | Name of the fallback HTML document inside `dir`. Setting a custom filename automatically enables SPA mode. |

#### Key Capabilities & Protections

1. **Deterministic SPA Fallback**:
   - For incoming `GET` and `HEAD` requests matching an SPA route, if the target path does not physically exist on disk and has **no file extension** (e.g., `/app/dashboard`, `/app/users/42`), Toron transparently serves the configured fallback document with HTTP `200 OK` and `Content-Type: text/html; charset=utf-8`.
   - Supports deep nested virtual paths (e.g., `/app/team/engineering/settings`).
   - For HTTP `HEAD` requests, Toron returns `200 OK` with correct content headers and omits the response payload.

2. **Asset Masking Protection**:
   - A critical challenge in traditional SPA rewrites is *asset masking*, where missing JavaScript or CSS files inadvertently return HTML, causing client-side syntax errors (`Uncaught SyntaxError: Unexpected token '<'`) and CSS MIME-type rejections.
   - Toron inspects the requested relative path: any request containing a file extension (`filepath.Ext(relPath) != ""`, such as `.js`, `.css`, `.png`, `.svg`, `.json`, `.woff2`) that does not exist on disk **strictly returns HTTP 404 Not Found** and is never served the fallback document.

3. **Path Traversal & Boundary Containment Defense**:
   - Fallback file resolution is verified against path traversal (`../`) and symlink directory escapes using `filepath.Rel` and `filepath.EvalSymlinks`.
   - Fallback paths attempting to escape the configured static root directory `dir` are strictly rejected with HTTP `403 Forbidden` or `404 Not Found`.

4. **Zero Performance Overhead on Physical Hits**:
   - Existing physical assets (e.g. `bundle.js`, `style.css`, images) and direct directory indices (`dashboard/index.html`) continue to serve directly on the first filesystem lookup with optimal performance and automatic MIME detection. Fallback logic runs only on filesystem misses (`os.IsNotExist`).

---

#### Configuration Examples

##### 1. React / Vite SPA Setup (Default Fallback)

Host a React or Vite single-page application under the `/app` URL prefix. All virtual routes resolve to `index.html`:

```yaml
routes:
  - type: "static"
    prefix: "/app"
    dir: "./frontend/dist"
    spa: true
    # fallback defaults to "index.html"
```

##### 2. Vue / Nuxt SPA Setup with Custom Fallback

Host a Vue application requiring a custom fallback document (such as `200.html` or `app.html` produced by static site generators):

```yaml
routes:
  - type: "static"
    prefix: "/portal"
    dir: "/var/www/portal/dist"
    spa: true
    fallback: "200.html"
```

> [!TIP]
> Setting `fallback: "200.html"` automatically activates SPA mode even if `spa: true` is not explicitly declared.

##### 3. Multi-SPA Architecture under Different Route Prefixes

Toron can host multiple independent SPAs concurrently alongside API microservices:

```yaml
routes:
  # Customer-facing React Application
  - type: "static"
    prefix: "/customer"
    dir: "/var/www/customer-portal/dist"
    spa: true
    fallback: "index.html"

  # Internal Admin Vue Application
  - type: "static"
    prefix: "/admin"
    dir: "/var/www/admin-portal/dist"
    spa: true
    fallback: "admin.html"

  # Backend REST API
  - type: "upstream"
    prefix: "/api"
    target: "http://localhost:9001"
```

##### 4. Standard Non-SPA Static Directory (Documentation / Downloads)

For static assets or documentation where non-existent files must return HTTP `404 Not Found` without fallback:

```yaml
routes:
  - type: "static"
    prefix: "/docs"
    dir: "./public/docs"
    spa: false                # Preserves standard 404 behavior for all missing paths
```

### Cleartext HTTP-to-HTTPS Redirection & Route-Level Overrides

When operating a secure edge gateway with TLS enabled (typically on port 443), production environments require unencrypted HTTP requests (typically on port 80) to be upgraded to HTTPS using `HTTP/1.1 301 Moved Permanently`. At the same time, specific automated workflows—such as Let's Encrypt automated HTTP-01 challenge validations (`/.well-known/acme-challenge/`), internal health probes, or unencrypted webhooks—require direct cleartext HTTP access without redirection.

Toron handles this cleanly by decoupling protocol redirect policies from routing endpoints.

#### 1. Enabling Auxiliary HTTP Listener (`config.yaml`)

In `config.yaml`, configure `server.http_redirect`:

```yaml
server:
  port: 443
  tls:
    enabled: true
    cert_file: "/etc/toron/certs/cert.pem"
    key_file: "/etc/toron/certs/key.pem"

  # Auxiliary cleartext HTTP listener
  http_redirect:
    enabled: true             # Activates HTTP listener alongside primary TLS server
    port: 80                  # Listener port (default: 80)
```

#### 2. Declaring Route-Level Exemption Overrides (`routes.yaml`)

By default, any route served by Toron is upgraded to HTTPS when accessed via the HTTP listener. To exempt a specific route from redirection and serve it directly over plain HTTP, add `redirect_http: false` to that route in `routes.yaml`:

```yaml
routes:
  # ACME Challenge Validation (Served directly over HTTP without 301 redirection)
  - type: "static"
    prefix: "/.well-known/acme-challenge"
    dir: "/var/www/certbot/.well-known/acme-challenge"
    redirect_http: false

  # Application UI (Redirects to https://example.com/app)
  - type: "static"
    prefix: "/app"
    dir: "/var/www/app/dist"
    spa: true

  # Backend API (Redirects to https://example.com/api/...)
  - type: "upstream"
    prefix: "/api"
    target: "http://127.0.0.1:8080"
```

#### Request Handling Rules on the HTTP Port:
1. If the request matches a route with `redirect_http: false`, Toron executes the route handler directly (serving the static file or proxying to the upstream backend) and returns `HTTP 200 OK`.
2. For all other requests (routes without `redirect_http: false` or unmatched paths), Toron immediately responds with `HTTP/1.1 301 Moved Permanently` pointing to `https://<host><request_uri>`, strictly preserving query parameters and path elements.
3. Open redirect protection: The incoming `Host` header is sanitized, port numbers are stripped, and malformed characters (CRLF, backslashes, spaces) are rejected with `HTTP 400 Bad Request`.

---

### Multi-Stream Logging & Daily System Logrotate (`logging`)

Toron provides enterprise-grade, configuration-driven multi-stream logging segregating internal server diagnostics, transactional HTTP access logs, and security audit telemetry.

#### 1. Global Logging Configuration (`config.yaml`)

```yaml
logging:
  level: "info"               # Severity threshold: "debug", "info", "warn", "error"
  format: "text"              # Output format: "text" (Combined format with latency) or "json"
  server_log: "logs/server.log"     # Internal server diagnostics, lifecycle events, and panics
  access_log: "logs/access.log"     # HTTP request/response transactional logs
  security_log: "logs/security.log" # WAF audit events, auth failures, and ACL blocks
```

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `logging.level` | `string` | `"info"` | Minimum log level threshold (`"debug"`, `"info"`, `"warn"`, `"error"`). |
| `logging.format` | `string` | `"text"` | Log formatting scheme (`"text"` or `"json"`). |
| `logging.server_log` | `string` | `"logs/server.log"` | Destination for server events, startup messages, and error traces (`"stdout"`, `"stderr"`, or file path). |
| `logging.access_log` | `string` | `"logs/access.log"` | Default destination for HTTP transactional access records (`"stdout"`, `"off"`, or file path). |
| `logging.security_log` | `string` | `"logs/security.log"` | Default destination for security audit logs and WAF blocks (`"stdout"`, `"off"`, or file path). |

#### 2. Declaring Per-Route Overrides (`routes.yaml`)

Individual routes can direct access and security logs to dedicated log files for compliance (e.g. PCI-DSS audit isolation), or silence access logs for high-frequency internal routes:

```yaml
routes:
  # Isolated logging for high-security API routes
  - type: "upstream"
    prefix: "/api/v2"
    target: "http://localhost:9001"
    access_log: "logs/api_v2_access.log"
    security_log: "logs/api_v2_security.log"

  # Silenced access logging for internal health probes
  - type: "static"
    prefix: "/health"
    dir: "/var/www/health"
    access_log: "off"
```

#### 3. Daily Log Rotation with System Logrotate

All log files are opened in append mode (`O_APPEND`), making them immediately safe for rotation tools using `copytruncate`. In addition, Toron handles the `SIGHUP` operating system signal to atomically close and reopen all active file handles after a rename/rotate:

1. Install the logrotate configuration:
   ```bash
   sudo cp etc/logrotate.d/toron /etc/logrotate.d/toron
   sudo chmod 0644 /etc/logrotate.d/toron
   ```
2. On rotation, logrotate automatically sends `SIGHUP` to Toron:
   ```bash
   pkill -HUP -f "toron"
   ```
3. Toron flushes buffers, closes existing descriptors, and opens new log files with zero dropped requests or socket restarts.

---

### Layer 4 TCP & UDP Transport Proxy Configuration (`routes.yaml`)

Toron provides raw Layer 4 socket forwarding (`type: "tcp"`) and datagram forwarding (`type: "udp"`). Both proxies feature resource bounds and idle deadline enforcement ([`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383)):

#### Configuration Parameters

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `type` | `string` | *(Required)* | Set to `"tcp"` for stream proxying or `"udp"` for datagram proxying. |
| `listen_port` | `integer` | *(Required)* | Local port on which Toron listens for incoming connections or datagrams. |
| `target` | `string` | `""` | Single upstream backend address (e.g. `"127.0.0.1:9090"`). |
| `targets` | `list[string]` | `[]` | Upstream backend cluster addresses for round-robin dispatch. |
| `max_connections` | `integer` | `10000` | Maximum concurrent active TCP connections. Saturated connections are rejected immediately upon `Accept()` without dialing upstream. |
| `idle_timeout` | `duration` | `"60s"` | Inactivity duration before closing idle TCP streams (Slowloris protection) or expiring inactive UDP client sessions. |
| `max_workers` | `integer` | `1024` | Maximum worker goroutines and task queue capacity for UDP datagram processing. Saturated packets drop fail-safe. |

For deep architectural details, buffer recycling (`sync.Pool`), session socket reuse, and microbenchmark performance data, consult the [Layer 4 TCP & UDP Transport Proxies Feature Guide](./features/layer4-proxy.md).

---

## Related Pages

- [Documentation Index](./index.md)
- [Layer 4 TCP & UDP Transport Proxies Feature Guide](./features/layer4-proxy.md)
- [Static File Serving Feature Guide](./features/static-file-serving.md)
- [Configuration Options Reference](./reference/config-options.md)
- [CLI Reference](./reference/cli.md)
