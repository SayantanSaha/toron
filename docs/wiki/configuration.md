---
title: Toron Configuration Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-04

depends_on:
  - REQ-007
  - REQ-019
  - REQ-027
  - REQ-034
  - REQ-035
  - REQ-036
  - TASK-007
  - TASK-019
  - TASK-027
  - TASK-034
  - TASK-035
  - TASK-036

derived_from:
  - REQ-007
  - REQ-027
  - ADR-002
  - ADR-022

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

  # Layer 4 TCP Stream Proxy
  - type: "tcp"
    listen_port: 8090
    target: "127.0.0.1:9090"

  # Layer 4 UDP Datagram Proxy
  - type: "udp"
    listen_port: 8091
    target: "127.0.0.1:9091"

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

## Related Pages

- [Documentation Index](./index.md)
- [Configuration Options Reference](./reference/config-options.md)
- [CLI Reference](./reference/cli.md)
