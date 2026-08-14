---
title: Toron Configuration Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-007
  - TASK-007

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

Toron features a flexible configuration system that reads parameters from YAML configuration files or falls back to built-in default settings.

## Specifying Configuration Files

Pass paths to your server and routing configuration files using the `-config` (`-c`) and `-routes` (`-r`) command-line flags:

```bash
go run ./cmd/toron -config config.yaml -routes routes.yaml
```

If no flags are passed, Toron automatically checks for `config.yaml` and `routes.yaml` in the current working directory.

## Sample `config.yaml` (Infrastructure)

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
  http2:
    enabled: true
    max_concurrent_streams: 250
    allow_h2c: true
  http3:
    enabled: true
    port: 8443
    alt_svc_header: true
  tls:
    enabled: false
    auto_dev_cert: true
  acme:
    enabled: false
    directory_url: "https://acme-v02.api.letsencrypt.org/directory"
    email: "admin@toron.local"
    domains:
      - "api.toron.local"
    cache_dir: "./certs"
    challenge_type: "http-01"

logging:
  level: "info"
  format: "text"
```

## Sample `routes.yaml` (Proxy Routing & Layer 4 Socket Proxying)

```yaml
routes:
  - type: "static"
    prefix: "/internal/dashboard"
    dir: "./public"

  - type: "upstream"
    prefix: "/api"
    headers:
      X-Version: "v2"
    algorithm: "round_robin"
    rate_limit: "100/min"
    targets:
      - "http://localhost:9001"
      - "http://localhost:9002"

  - type: "upstream"
    prefix: "/services/analytics"
    algorithm: "sticky_cookie"
    sticky_cookie_name: "TORON_STICKY"
    targets:
      - "http://localhost:9009"
      - "http://localhost:9010"

  - type: "tcp"
    listen_port: 8090
    target: "127.0.0.1:9090"

  - type: "udp"
    listen_port: 8091
    target: "127.0.0.1:9091"
```

## Extensible Multi-Format Support

Toron's configuration core (`pkg/config`) uses a decoupled `Loader` interface. While YAML (`.yaml` / `.yml`) is natively supported out of the box, registering decoders for JSON (`.json`) or TOML (`.toml`) can be easily added without modifying server initialization.

## Related Pages

- [CLI Reference](./reference/cli.md)
- [Configuration Options Reference](./reference/config-options.md)
