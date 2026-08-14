# Toron Web Server

**Toron** is an event-driven, high-performance, modular web server and API gateway written in Go. Designed with performance, security, and developer ergonomics as primary priorities, Toron features non-blocking TCP socket event handling, zero-copy HTTP/1.1 parsing, HTTP/2 stream multiplexing, HTTPS TLS 1.2/1.3 with auto-dev certificate generation, dual-file YAML configuration, and unified static site & reverse proxy gateway routing.

---

## 🌟 Key Features

* **Event Reactor Engine**: High-performance, non-blocking TCP event loop with a configurable worker pool for concurrent request dispatching.
* **HTTP/1.1, HTTP/2 & HTTP/3 Support**: Non-blocking HTTP/1.1, HTTP/2 cleartext `h2c` / stream multiplexing, and HTTP/3 QUIC (UDP) engine with automatic `Alt-Svc` protocol advertising headers.
* **HTTPS TLS Encryption & ACME Zero-Touch SSL**: TLS 1.2/1.3 support, ALPN negotiation (`h2`, `http/1.1`), ACME (Let's Encrypt / ZeroSSL) HTTP-01 and TLS-ALPN-01 (`acme-tls/1`) zero-touch production SSL certificate issuance and renewal, disk certificate caching, and zero-config self-signed ECDSA dev certificate generator (`auto_dev_cert`).
* **WebSocket Protocol Upgrade & Tunneling**: Full support for HTTP/1.1 (RFC 6455 101 Switching Protocols) and HTTP/2 Extended CONNECT protocol (RFC 8441 `:protocol = websocket`) with bi-directional stream tunneling for real-time web services.
* **Layer 4 TCP & UDP Transport Proxying**: Raw socket stream forwarding (`type: "tcp"`) and connectionless datagram proxying (`type: "udp"`) with port listener binding and load balancing.
* **Dual-File YAML Configuration**: Decoupled infrastructure settings ([`config.yaml`](./config.yaml)) and routing rules ([`routes.yaml`](./routes.yaml)).
* **Unified Routing Architecture**: Routing rules accept `type: "static"` or `type: "upstream"`, sharing identical domain host matching, header-conditional dispatching, and subpath prefix routing capabilities.
* **Upstream Load Balancing & Sticky Sessions**: Multi-target load balancing (`round_robin`, `random`, `sticky_cookie`, `ip_hash`), cookie-based session affinity, active HTTP health check probing, and a 3-state Circuit Breaker (`Closed`, `Open`, `HalfOpen`).
* **Token Bucket Rate Limiting Middleware**: Route-level DDoS and abuse protection (`rate_limit: "100/min"`, `"10/sec"`) per client IP or `X-API-Key` header returning `429 Too Many Requests` and `Retry-After` headers.
* **Streaming Response Compression (Gzip & Deflate)**: Transparent HTTP response compression middleware utilizing standard library `compress/gzip` and `compress/flate` with `sync.Pool` allocation reuse for high throughput and reduced bandwidth.
* **Prometheus Metrics & W3C Tracing**: Standardized `/metrics` endpoint exporting Prometheus counters (`toron_http_requests_total`), latency histograms (`toron_http_request_duration_seconds`), QUIC stream gauges, and circuit breaker trip counters; automatic OpenTelemetry W3C `traceparent` context header propagation across upstream target microservices.
* **Web Control Center & JSON Metrics API**: Mobile-first Web Dashboard UI served on `/internal/dashboard/` powered by internal management JSON API endpoints (`/internal/api/status`, `/internal/api/metrics`, `/internal/api/routes`, `/internal/api/upstreams/health`, `/internal/api/proxy-test`).
* **Security & Path Traversal Guards**: Strict header (8 KB) and body (4 MB) size limits, socket read/write timeouts, path traversal sanitization, and panic recovery middleware.
* **Zero-Downtime Route Hot Reloading**: Background `fsnotify` file worker (`RouteWatcher`) monitors `routes.yaml` file edits, automatically validating and atomically reloading routing tables without dropping active TCP/UDP/QUIC sockets.
* **Configuration Dry-Run Validator**: Native CLI flag (`-t` / `-test-config`) to validate YAML syntax without starting the server listener.
* **Testing & Microservices Suite**: Includes 10 dummy upstream microservices ([`dummy-services/`](./dummy-services/)) and a standardized REST client file ([`test_endpoint.http`](./test_endpoint.http)).

---

## 🛠️ Installation & Building

### Prerequisites
* **Go**: Version 1.20 or later.

### Building Toron
Clone the repository and build the binary:

```powershell
# Clone repository
cd server

# Build executable binary
go build -o toron.exe ./cmd/toron
```

---

## ⚙️ Configuration Guide

Toron splits configuration into two files:
1. [`config.yaml`](./config.yaml): Infrastructure, network listener, worker pool, HTTP/2, TLS, and logging settings.
2. [`routes.yaml`](./routes.yaml): Static site hosting and upstream reverse proxy routing rules.

---

### 1. Infrastructure Configuration (`config.yaml`)

```yaml
# Toron Web Server Infrastructure Configuration File

server:
  host: "0.0.0.0"             # Listener IP address (0.0.0.0 for all interfaces)
  port: 8080                  # Server HTTP/HTTPS port
  worker_pool_size: 128       # Concurrent worker pool threads
  read_timeout: 5s            # Maximum time to read request headers/body
  write_timeout: 5s           # Maximum time to write response
  idle_timeout: 30s           # Keep-alive socket idle duration
  max_header_bytes: 8192      # 8 KB maximum header size limit
  max_body_bytes: 4194304     # 4 MB maximum request body size limit
  http2:
    enabled: true             # Enable HTTP/2 protocol engine
    max_concurrent_streams: 250
    max_frame_size: 16384
    allow_h2c: true           # Allow HTTP/2 Cleartext (h2c) prior-knowledge connections
  http3:
    enabled: true             # Enable HTTP/3 QUIC (UDP) protocol engine
    port: 8443                # HTTP/3 QUIC UDP port
    alt_svc_header: true      # Automatically advertise Alt-Svc: h3=":8443" response headers
  tls:
    enabled: false            # Set true to enable HTTPS TLS listener
    cert_file: ""             # Path to TLS X.509 certificate file
    key_file: ""              # Path to TLS private key file
  acme:
    enabled: false            # Enable ACME zero-touch production SSL issuance (Let's Encrypt / ZeroSSL)
    directory_url: "https://acme-v02.api.letsencrypt.org/directory"
    email: "admin@toron.local"
    domains:
      - "api.toron.local"
    cache_dir: "./certs"
    challenge_type: "http-01" # Challenge validation strategy: "http-01" or "tls-alpn-01"
  compression:
    enabled: true             # Enable automatic gzip/deflate response compression
    min_length: 512           # Minimum response byte threshold
    level: -1                 # Compression level (-1 = default)
    encodings:
      - "gzip"
      - "deflate"

logging:
  level: "info"               # Logging level: debug, info, warn, error
  format: "text"              # Logging format: text or json
```

#### TLS & ACME Certificate Configuration Patterns

Toron resolves TLS certificates using a strict priority fallback chain:
1. **ACME Managed Certificate (`acme.enabled: true`)**: Automated Let's Encrypt / ZeroSSL production certificate.
2. **Static Certificate Files (`tls.cert_file` & `tls.key_file`)**: Custom corporate certificate files.
3. **Auto Dev Certificate (`tls.auto_dev_cert: true`)**: Local self-signed ECDSA certificate fallback for local development.

* **Pattern A: Local Development (Self-Signed HTTPS)**
  ```yaml
  server:
    tls:
      enabled: true
      auto_dev_cert: true
    acme:
      enabled: false
  ```

* **Pattern B: Production Zero-Touch Automated SSL (Let's Encrypt)**
  ```yaml
  server:
    tls:
      enabled: true
    acme:
      enabled: true
      email: "admin@mycompany.com"
      domains:
        - "api.mycompany.com"
      cache_dir: "./certs"
      challenge_type: "http-01"
  ```

* **Pattern C: Custom Corporate SSL Certificates**
  ```yaml
  server:
    tls:
      enabled: true
      cert_file: "/etc/ssl/certs/custom.crt"
      key_file: "/etc/ssl/private/custom.key"
      auto_dev_cert: false
    acme:
      enabled: false
  ```

---

### 2. Routing Configuration (`routes.yaml`)

Routes support both **Static Sites** (`type: "static"`) and **Upstream Reverse Proxies** (`type: "upstream"`). Both route types share host matching, header conditions, and path prefixes.

```yaml
# Toron Web Server Routing Configuration File

routes:
  # 1. Static Site Route - Control Center UI
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"

  # 2. Domain-Based Upstream Route (api.toron.local -> Dummy Services 1, 4, 6)
  - type: "upstream"
    host: "api.toron.local"
    prefix: "/"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9004"
      - "http://localhost:9006"

  # 3. Header-Based Upstream Route (API Versioning v2)
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    algorithm: "round_robin"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"
      - "http://localhost:9003"

  # 4. Header-Based Upstream Route (API Versioning v1)
  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v1"
    target: "http://localhost:9004"

  # 5. Path Prefix Upstream Route (Load Balanced Cluster)
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

  # 6. Single Target Upstream Route
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
```

#### Route Options Reference
* `type`: Route handler mode (`"static"`, `"upstream"`, `"tcp"`, or `"udp"`).
* `listen_port` / `port`: Dedicated port for Layer 4 `"tcp"` and `"udp"` proxy socket listeners.
* `host` / `domain`: Optional domain matching (e.g., `api.toron.local` or `docs.toron.local`).
* `prefix`: Path prefix matcher (e.g., `/api`, `/internal/dashboard`).
* `headers`: Key/value map of expected request HTTP headers (e.g., `X-Version: "v2"`).
* `dir`: Local filesystem path for `static` routes (e.g., `./public`).
* `targets` / `target`: Target URL string or list of URLs for `upstream` reverse proxy routes.
* `algorithm`: Load balancing algorithm (`"round_robin"`, `"random"`, `"sticky_cookie"`, or `"ip_hash"`).
* `sticky_cookie_name`: Optional custom cookie name for `"sticky_cookie"` session affinity (default: `"TORON_STICKY"`).
* `rate_limit`: Route rate limit threshold string (e.g. `"100/min"`, `"10/sec"`, `"1000/hour"`) enforcing Token Bucket client rate limits.
* `health_check_path`: Path for upstream active health probing (e.g., `/health`).

---

## 🚀 Running the Server

### 1. Test Configuration Syntax (Dry-Run Mode)
Before starting the server, test YAML syntax and file paths using `-t` or `-test-config`:

```powershell
go run ./cmd/toron -t
```
*Output:*
```text
2026/08/12 15:50:16 [TORON] Configuration syntax OK: config.yaml and routes.yaml are valid.
```

---

### 2. Start the Toron Server
Start Toron using default configuration files (`config.yaml` & `routes.yaml` in current working directory):

```powershell
go run ./cmd/toron
```

Or specify custom configuration file paths using `-c` / `-config` and `-r` / `-routes`:

```powershell
go run ./cmd/toron -c ./config.yaml -r ./routes.yaml
```

---

### 3. Start Upstream Dummy Web Services (For Testing Proxy & Load Balancer)
In a separate terminal, launch the 10 dummy upstream web microservices running on ports 9001–9010:

```powershell
go run ./dummy-services
```
*Output:*
```text
2026/08/12 15:30:00 [DUMMY-SERVICES] Cluster started 10 HTTP services on ports 9001 - 9010.
```

---

### 4. Access Web Control Center & Proxy Dashboard
Open your browser and navigate to:
```text
http://localhost:8080/internal/dashboard/
```
The Control Center provides:
* Real-time active health probing of upstream microservices (Ports 9001–9010).
* Interactive REST API request composer to test headers (`X-Version: v1` / `v2`), subpaths, and domain routing.
* Formatted JSON response preview and latency benchmarking (ms).

---

### 5. Test Server Endpoints via `curl`

```powershell
# Built-in Health Endpoint
curl http://localhost:8080/health

# Built-in Server Status Endpoint
curl http://localhost:8080/api/status

# Header Routing (API v2 -> Dummy Services 1, 2, 3)
curl -H "X-Version: v2" http://localhost:8080/api/users

# Header Routing (API v1 -> Dummy Service 4)
curl -H "X-Version: v1" http://localhost:8080/api/users

# Cluster Load Balancing (Dummy Services 5, 6, 7)
curl http://localhost:8080/services/cluster

# Domain Host Matching
curl -H "Host: api.toron.local" http://localhost:8080/
```

---

### 6. Enable and Test HTTPS TLS

To test HTTPS mode:
1. Update `config.yaml`:
   ```yaml
   server:
     port: 8443
     tls:
       enabled: true
       auto_dev_cert: true
   ```
2. Start the server:
   ```powershell
   go run ./cmd/toron
   ```
3. Execute encrypted HTTPS requests:
   ```powershell
   # Test HTTPS connection
   curl -k https://localhost:8443/health

   # Test HTTP/2 over TLS via ALPN negotiation
   curl -k --http2 https://localhost:8443/api/status -i
   ```

---

## 🧪 Testing & Benchmarking

### Run Unit Test Suite
Run unit tests across all packages:

```powershell
# Run unit tests across all packages
go test -v ./...

# Run test coverage report
go test -cover ./pkg/... ./dummy-services/...
```

### Run Performance Benchmarks
Run statement execution benchmarks and memory allocation tracking:

```powershell
go test -bench=. -benchmem ./pkg/reactor ./pkg/httpparser ./pkg/router ./pkg/server
```

---

## 📂 Project Architecture

```text
server/
├── cmd/
│   └── toron/              # Application entry point & CLI orchestration
├── config.yaml             # Infrastructure & server config file
├── routes.yaml             # Routing rules (static sites & reverse proxies)
├── dummy-services/         # Test suite of 10 dummy upstream microservices
├── pkg/
│   ├── config/             # YAML config loader & syntax validator
│   ├── httpparser/         # HTTP/1.1 request parser & response builder
│   ├── proxy/              # Reverse proxy, load balancer & circuit breaker
│   ├── reactor/            # Non-blocking TCP socket event engine
│   ├── router/             # URL/Domain/Header router & static file handler
│   └── server/             # Server lifecycle, HTTP/2 & HTTPS TLS engine
├── public/                 # Static Web Control Center & Proxy Dashboard UI
├── test_endpoint.http      # Standardized REST client HTTP request runner
└── PRD.md                  # Project Requirements Document
```

---

## 📜 License

This project is open-source software built under the prototype development model.
