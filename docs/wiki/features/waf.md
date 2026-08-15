---
title: Web Application Firewall (WAF) & Injection Protection
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-15
updated: 2026-08-15

depends_on:
  - REQ-041
  - TASK-041
  - ADR-036

derived_from:
  - REQ-041

documents:
  - WAF-GUIDE

related_to:
  - ../configuration.md
  - ./cors-security-headers.md
  - ./authentication.md
  - ../index.md
---

# Web Application Firewall (WAF) & OWASP Injection Protection

## Overview

Toron includes a native, high-throughput **Web Application Firewall (WAF)** middleware engine (`pkg/waf`) designed to intercept and mitigate Layer 7 security threats (OWASP Top 10) before they reach downstream application handlers or backend microservices.

## Key Capabilities

1. **OWASP Top 10 Injection Mitigation**:
   - **SQL Injection (SQLi)**: Detects `UNION SELECT`, `' OR 1=1`, and destructive DDL statements (`DROP TABLE`, `INSERT INTO`).
   - **Cross-Site Scripting (XSS)**: Intercepts script tags (`<script>`), inline event handlers (`onerror=`, `onload=`), and `javascript:` URIs.
   - **Path Traversal / LFI**: Detects directory escape sequences (`../`, `%2e%2e/`) and sensitive system files (`/etc/passwd`).
   - **Command Injection / RCE**: Intercepts shell command chaining (`; /bin/sh`, `| bash`) and PHP/system execution primitives (`eval()`, `system()`).
2. **Flexible Evaluation Modes**:
   - `enforce`: Immediately blocks malicious requests with HTTP `403 Forbidden` and a structured JSON error response.
   - `detection`: Passes requests through to application handlers while logging anomaly threat scores and attaching `X-Toron-WAF-Anomaly-Score` headers.
3. **Protocol Integrity & HTTP Request Smuggling Guard**:
   - **Request Smuggling Prevention**: Rejects requests containing both `Content-Length` and `Transfer-Encoding` headers or mismatched duplicate `Content-Length` header values with `400 Bad Request`.
   - **Control Character Filtering**: Rejects non-printable ASCII control characters (`0x00–0x1F`, `0x7F`) in paths, query parameters, or header fields.
   - **Payload Size Limits**: Enforces strict byte limits on single header values (4 KB), query strings (4 KB), and parameters (2 KB) returning `413 Payload Too Large`.
4. **Multi-Location & Bounded Inspection**:
   - Scans URL paths, raw query strings, HTTP headers, and request body payloads up to a configurable maximum size (`max_inspect_body_size`), preserving payload streams for downstream handlers.

## Configuration in `config.yaml`

```yaml
server:
  # Web Application Firewall (WAF) & Layer 7 OWASP Injection Protection Engine
  waf:
    enabled: true             # Enable WAF Layer 7 threat inspection middleware
    mode: "enforce"           # Evaluation mode: "enforce" (403 block) or "detection" (log-only score)
    anomaly_threshold: 5      # Threat score limit above which request is blocked
    max_inspect_body_size: 65536 # Maximum payload body bytes scanned (64 KB)
    disabled_rules: []        # Optional list of rule IDs to bypass (e.g. ["SQLI-002"])
```

## Triggered Block Response Format

When a request is blocked in `enforce` mode, Toron returns `403 Forbidden` with application/json body:

```json
{
  "error": "Forbidden",
  "message": "WAF security violation detected",
  "threat_score": 5,
  "triggered_rules": ["SQLI-001"]
}
```
