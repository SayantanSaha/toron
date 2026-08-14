---
title: HTTPS TLS Encryption and Certificate Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-023
  - TASK-023

derived_from:
  - REQ-023
  - ADR-018

documents:
  - TLS-HTTPS-GUIDE

related_to:
  - index.md
  - configuration.md
  - http2.md
---

# HTTPS TLS Encryption and Certificate Guide

## Overview

Toron features built-in HTTPS TLS transport layer security with automatic development certificate generation, ALPN HTTP/2 negotiation (`h2`, `http/1.1`), and TLS 1.2+ protocol enforcement.

## Configuration in `config.yaml`

Enable HTTPS and configure certificate paths under the `server.tls` section of `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8443
  tls:
    enabled: true
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    auto_dev_cert: false
```

### Auto Development Mode (Self-Signed Certificates)

For local development without existing certificate files, enable `auto_dev_cert`:

```yaml
server:
  tls:
    enabled: true
    auto_dev_cert: true
```

Toron automatically generates an in-memory ECDSA P-256 certificate for `localhost` and `127.0.0.1`.

## Per-Host Dynamic SNI & Mutual TLS (mTLS) in `routes.yaml`

Toron supports configuring dedicated X.509 certificates, Mutual TLS (mTLS) client certificate verification, and minimum TLS versions per virtual host domain directly within `routes.yaml`:

```yaml
routes:
  # 1. Standard Multi-Tenant Virtual Host with Dedicated Certificate
  - type: "upstream"
    host: "app.example.com"
    prefix: "/"
    target: "http://localhost:9001"
    tls:
      cert_file: "./certs/app.crt"
      key_file: "./certs/app.key"

  # 2. High-Security Internal Host with Strict Mutual TLS (mTLS) & TLS 1.3
  - type: "upstream"
    host: "secure.internal.local"
    prefix: "/"
    target: "http://localhost:9002"
    tls:
      cert_file: "./certs/internal.crt"
      key_file: "./certs/internal.key"
      ca_file: "./certs/internal_ca.crt"
      client_auth: "require_and_verify" # "no_client_cert", "request_client_cert", "require_any_client_cert", "verify_client_cert_if_given", "require_and_verify"
      min_version: "tls1.3"             # "tls1.2", "tls1.3"
```

### Supported Client Authentication Policies (`client_auth`)
* `no_client_cert`: Standard TLS without requesting client certificates (default).
* `request_client_cert`: Requests a client certificate during handshake but allows unauthenticated clients.
* `require_any_client_cert`: Requires the client to present a certificate without verifying against a CA.
* `verify_client_cert_if_given`: Verifies client certificate against `ca_file` only if provided.
* `require_and_verify`: Strictly mandates and validates client certificates signed by the configured `ca_file` (mTLS).
