---
title: CORS Policies & Enterprise Security Headers
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-040
  - TASK-040
  - ADR-035

derived_from:
  - REQ-040

documents:
  - CORS-SECURITY-HEADERS-GUIDE

related_to:
  - ../configuration.md
  - ./authentication.md
  - ../index.md
---

# CORS Policies & Enterprise Security Headers

## Overview

Toron provides built-in edge protection and browser security through **Cross-Origin Resource Sharing (CORS)** and **Enterprise Security Headers** middleware.

## Key Capabilities

1. **Automatic CORS Preflight Interception**:
   - Responds to `OPTIONS` preflight requests from authorized origins with `204 No Content` and appropriate `Access-Control-Allow-*` headers without hitting backend upstreams.
   - Origin pattern matching supports exact hosts (`https://app.example.com`), wildcards (`*`), and wildcard subdomains (`https://*.example.com`).
   - Supports credentials (`Access-Control-Allow-Credentials: true`), exposed headers, and preflight max-age caching.
2. **OWASP Baseline Security Headers**:
   - `Strict-Transport-Security` (HSTS): Enforces HTTPS connections.
   - `X-Content-Type-Options`: Prevents MIME confusion and sniffing (`nosniff`).
   - `X-Frame-Options`: Protects against clickjacking (`DENY` / `SAMEORIGIN`).
   - `Referrer-Policy`: Controls cross-origin referrer leakage (`strict-origin-when-cross-origin`).
   - `Content-Security-Policy` (CSP) & `Permissions-Policy`: Restricts resource execution and device hardware access.

## Configuration in `config.yaml`

```yaml
server:
  # Cross-Origin Resource Sharing (CORS) Settings
  cors:
    enabled: true
    allow_origins:
      - "https://app.example.com"
      - "https://*.internal.net"
    allow_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "DELETE"
      - "OPTIONS"
    allow_headers:
      - "Origin"
      - "Content-Type"
      - "Accept"
      - "Authorization"
      - "X-Requested-With"
    expose_headers:
      - "X-Cache"
      - "Content-Length"
    allow_credentials: true
    max_age: 86400

  # Enterprise Browser Security Headers
  security_headers:
    enabled: true
    hsts: "max-age=31536000; includeSubDomains; preload"
    content_type_options: "nosniff"
    frame_options: "DENY"
    referrer_policy: "strict-origin-when-cross-origin"
    csp: "default-src 'self'"
    permissions_policy: "camera=(), microphone=(), geolocation=()"
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [Multi-Scheme Authentication](./authentication.md)
- [HTTPS TLS & Auto Dev Certs](./tls-https.md)
