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
4. **CIDR-Based IP Access Control Lists (ACLs)**:
   - **Allowed IPs (`allowed_ips`)**: Configurable list of IPv4 and IPv6 CIDR blocks (e.g. `10.0.0.0/8`, `192.168.1.0/24`) or single IP addresses. When configured, requests originating from client IPs outside these ranges shall be immediately rejected with HTTP `403 Forbidden`.
   - **Denied IPs (`denied_ips`)**: Configurable list of IPv4 and IPv6 CIDR blocks (e.g. `198.51.100.0/24`) or single IP addresses returning `403 Forbidden`.
   - **Fast-Path $O(1)$ Pre-Inspection**: IP access checking executes before deep regex scanning or body buffering.
5. **Per-Route WAF Customization & Overrides (`routes.yaml`)**:
   - Any individual route in `routes.yaml` can define its own `waf:` block to selectively override global settings (e.g. tuning anomaly thresholds, disabling specific rules like `SQLI-001` for legacy backends, or setting dedicated CIDR allowlists).
6. **Multi-Location & Bounded Inspection**:
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
    allowed_ips:              # Optional global CIDR IP allowlist
      - "10.0.0.0/8"
      - "127.0.0.1"
    denied_ips:               # Optional global CIDR IP denylist
      - "198.51.100.0/24"
```

## Route-Level WAF Overrides in `routes.yaml`

```yaml
routes:
  # 1. Admin route with strict CIDR IP access control
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

  # 2. Legacy API route with SQLI-001 rule disabled
  - type: "upstream"
    prefix: "/services/legacy-api"
    target: "http://localhost:9002"
    waf:
      enabled: true
      mode: "enforce"
      disabled_rules:
        - "SQLI-001"
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

## Prometheus Observability & Metrics

Toron exports real-time WAF telemetry at the standard `/metrics` endpoint:

- **`toron_waf_blocked_requests_total{category="sqli|xss|traversal|rce|protocol|ip_acl", route="/..."}`**: Total number of blocked requests categorized by threat vector and route.
- **`toron_waf_anomalies_detected_total{category="...", mode="detection"}`**: Total threat anomalies detected in detection mode.
- **`toron_waf_inspection_duration_seconds`**: Histogram tracking WAF inspection execution latency.

```promql
# Example Prometheus Alert Query for SQLi spikes:
sum(rate(toron_waf_blocked_requests_total{category="sqli"}[5m])) > 10
```

## Structured Security Audit Logging

Toron emits structured, SIEM-ready JSON log lines for all security violations to `stdout`, `stderr`, or a dedicated log file:

```json
{
  "timestamp": "2026-08-15T14:30:00Z",
  "event": "waf_block",
  "client_ip": "198.51.100.45",
  "method": "POST",
  "path": "/api/users",
  "category": "sqli",
  "rule_id": "SQLI-001",
  "anomaly_score": 5,
  "action": "blocked",
  "location": "query",
  "payload_snippet": "1 UNION SELECT username, password FROM users"
}
```


