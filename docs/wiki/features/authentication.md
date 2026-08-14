---
title: Multi-Scheme Authentication Middleware
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-036
  - TASK-036
  - ADR-031

derived_from:
  - REQ-036

documents:
  - FEATURE-AUTHENTICATION

related_to:
  - ../configuration.md
  - ../index.md
---

# Multi-Scheme Authentication Middleware (JWT, API Key, Basic Auth)

## Overview

Toron includes a multi-scheme authentication middleware for securing static assets and upstream reverse proxy endpoints. The engine supports JWT Bearer verification, API key validation, and RFC 7617 HTTP Basic authentication.

## Supported Authentication Strategies

### 1. JSON Web Token (JWT) Bearer
- **Header**: `Authorization: Bearer <token>`
- **Cryptographic Algorithms**: Standard HMAC-SHA256 (`HS256`, `HS384`, `HS512`).
- **Claim Enforcement**: Enforces `exp` (expiration), `nbf` (not-before), `iss` (issuer), and `aud` (audience).
- **Identity Propagation**: Injects `X-Authenticated-User: <sub/user>` into downstream requests forwarded to upstream microservices.

### 2. API Key Authentication
- **Sources**: `X-API-Key` header, `Authorization: ApiKey <key>`, or `?api_key=` query parameter.
- **Security**: Utilizes constant-time string comparisons (`crypto/subtle`) to prevent timing side-channel attacks.

### 3. HTTP Basic Authentication (RFC 7617)
- **Header**: `Authorization: Basic <base64(user:password)>`
- **Rejection**: Emits `401 Unauthorized` with `WWW-Authenticate: Basic realm="..."` on failure.

## Route-Level Configuration (`routes.yaml`)

```yaml
routes:
  # Route protected with API Key
  - type: "upstream"
    prefix: "/services/auth"
    target: "http://localhost:9008"
    auth:
      type: "api_key"
      api_key:
        keys:
          - "secret-api-key-12345"

  # Route protected with JWT Bearer Token
  - type: "upstream"
    prefix: "/services/admin"
    target: "http://localhost:9007"
    auth:
      type: "jwt"
      jwt:
        secret: "my-super-secret-key"
        issuer: "toron-auth"
        audience: "api.toron.local"
```

## Related Pages

- [Configuration Guide](../configuration.md)
- [Reverse Proxy & Gateway Routing](./reverse-proxy.md)
