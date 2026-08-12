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
