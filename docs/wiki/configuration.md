---
title: Toron Configuration Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-14

depends_on:
  - REQ-007
  - REQ-019
  - REQ-034
  - REQ-035
  - REQ-036
  - TASK-007
  - TASK-019
  - TASK-034
  - TASK-035
  - TASK-036

derived_from:
  - REQ-007
  - ADR-002

documents:
  - CONFIGURATION-GUIDE

related_to:
  - index.md
  - reference/config-options.md
  - reference/cli.md
---

# Toron Configuration Guide

## Overview

Toron utilizes a decoupled dual-file YAML configuration architecture separating infrastructure parameters ([`config.yaml`](file:///D:/Work/server/config.yaml)) from application routing rules ([`routes.yaml`](file:///D:/Work/server/routes.yaml)).

## Specifying Configuration Files

Pass paths to your server and routing configuration files using the `-config` (`-c`) and `-routes` (`-r`) command-line flags:

```bash
go run ./cmd/toron -config config.yaml -routes routes.yaml
```

If no flags are passed, Toron automatically checks for `config.yaml` and `routes.yaml` in the current working directory.

### Configuration Dry-Run Validation (`-t`)

Validate YAML configuration syntax before starting listeners:

```bash
toron.exe -t
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

  # Transparent Response Compression (Gzip & Deflate)
  compression:
    enabled: true
    min_length: 512
    level: -1
    encodings:
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
    allow_credentials: true
    max_age: 86400

  # Web Application Firewall (WAF) & Layer 7 OWASP Threat Inspection
  waf:
    enabled: true
    mode: "enforce"           # "enforce" (403 block) or "detection" (log-only anomaly score)
    anomaly_threshold: 5
    max_inspect_body_size: 65536
    allowed_ips: []           # Optional global CIDR IP allowlist (e.g. ["10.0.0.0/8"])
    denied_ips: []            # Optional global CIDR IP denylist (e.g. ["198.51.100.0/24"])
    disabled_rules: []        # Optional list of rule IDs to bypass globally

logging:
  level: "info"
  format: "text"
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
