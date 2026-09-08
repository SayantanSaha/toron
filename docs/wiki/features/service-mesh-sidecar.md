---
title: Service Mesh Sidecar Mode
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-16
updated: 2026-09-08

depends_on:
  - REQ-048
  - REQ-086
  - TASK-048
  - TASK-090
  - TASK-091
  - TASK-092

derived_from:
  - ADR-043
  - ADR-081
  - SEC-25

documents:
  - SERVICE-MESH-SIDECAR-GUIDE

related_to:
  - kubernetes-ingress.md
  - tls-https.md
  - configuration.md
  - reference/config-options.md
---

# 🕸️ Service Mesh Sidecar Mode (`pkg/sidecar`)

Toron Edge Gateway features a lightweight, zero-dependency **Service Mesh Sidecar Mode** ([`pkg/sidecar`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go)). In Sidecar Mode, Toron runs alongside pod application containers (`127.0.0.1`), transparently enforcing pod-to-pod Mutual TLS (mTLS) zero-trust encryption, dynamic weighted traffic splitting (e.g. 80/20 canary releases), and configurable request body bounding with fail-fast HTTP 413 rejection with minimal memory overhead (<10MB RAM).

---

## 🌟 Key Features

* **Zero External Dependencies**: Implements mTLS certificate verification, weighted traffic splitting, and bounded body ingestion using Go standard library `crypto/tls`, `net/http`, and `io`.
* **Pod-to-Pod Strict mTLS**: Enforces `client_auth: "require_and_verify"` with Root CA certificate pools (`ca_file`).
* **Weighted Traffic Splitting**: Distributes outbound egress traffic across multiple canary backends according to assigned integer weights (e.g. 80% to v1, 20% to v2).
* **Ingress / Egress / Dual Modes**: Flexible operational modes running on dedicated local ports (Ingress: 15006, Egress: 15001, Application: 8080).
* **Configurable Request Body Limit & HTTP 413 Protection ([`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374))**: Configurable request payload threshold via [`SidecarConfig.MaxBodyBytes`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L109) (`max_body_bytes`), defaulting to a 10 MB fallback. Prevents silent truncation and upstream data corruption by rejecting oversized payloads immediately with `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`).

---

## ⚙️ Configuration Reference (`toron.yaml` / `config.yaml`)

```yaml
sidecar:
  enabled: true
  mode: "dual"              # Operational mode: "ingress", "egress", or "dual"
  ingress_port: 15006       # Pod inbound mTLS listener port
  egress_port: 15001        # Pod outbound proxy listener port
  app_port: 8080            # Local app container target port (127.0.0.1:8080)
  max_body_bytes: 10485760  # Maximum request body size in bytes (default: 10MB)
  strict_mtls: true         # Enforce RequireAndVerifyClientCert mTLS
  cert_file: "./certs/sidecar.crt"
  key_file: "./certs/sidecar.key"
  ca_file: "./certs/ca.crt"
  traffic_splits:           # Weighted canary traffic splitting
    - prefix: "/api"
      backends:
        - target: "http://service-v1:8080"
          weight: 80
        - target: "http://service-v2:8080"
          weight: 20
```

### Parameter Reference

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enabled` | `boolean` | `false` | Enables or disables Service Mesh Sidecar mode. |
| `mode` | `string` | `"dual"` | Operational proxy mode: `"ingress"` (inbound mTLS), `"egress"` (outbound traffic routing), or `"dual"` (both). |
| `ingress_port` | `integer` | `15006` | Inbound mTLS listener port for incoming pod-to-pod traffic. |
| `egress_port` | `integer` | `15001` | Outbound proxy listener port for outgoing application traffic. |
| `app_port` | `integer` | `8080` | Local target application port (`127.0.0.1:<app_port>`). |
| `max_body_bytes` | `integer` (`int64`) | `10485760` (10 MB) | Maximum permitted request body size in bytes for mutating requests. If omitted, `0`, or negative (`<= 0`), automatically defaults to 10 MB (`10 * 1024 * 1024` bytes). |
| `strict_mtls` | `boolean` | `false` | Enforce strict client certificate authentication (`RequireAndVerifyClientCert`). |
| `cert_file` | `string` | `""` | Path to sidecar X.509 public certificate file. |
| `key_file` | `string` | `""` | Path to sidecar private key file. |
| `ca_file` | `string` | `""` | Path to trusted Root CA certificate bundle for mTLS validation. |
| `traffic_splits` | `list` | `[]` | List of weighted routing rules for canary releases (`prefix`, `backends: [{target, weight}]`). |

---

## 🛡️ Request Body Size Limits & HTTP 413 Rejection ([`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374))

### Background & Security Remediation

Prior to the remediation of [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374) (addressed in [`REQ-086`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-086.md), [`ADR-081`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-081.md), and [`TASK-090`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-090.md)–[`TASK-092`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-092.md)), the sidecar proxy engine used a hardcoded 10 MB limit and an unbounded `io.LimitReader` when proxying mutating request bodies. When a client transmitted a payload exceeding 10 MB, the proxy silently truncated the incoming data, updated the request `Content-Length` to the truncated length, and forwarded the partial payload upstream.

This resulted in silent data corruption, truncated JSON payloads, and desynchronization in backend microservices without error visibility.

The enhanced sidecar architecture resolves this vulnerability through:
1. **Configurable Limits**: Exposed via [`SidecarConfig.MaxBodyBytes`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L109) (`max_body_bytes`).
2. **Safe Fallback**: Any configuration with `max_body_bytes <= 0` or missing defaults to 10 MB (`10,485,760` bytes) in [`DefaultAppConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L466), [`validateConfigDefaults`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L153), and [`NewProxyEngine`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L37).
3. **Dual-Stage Overflow Protection**: Oversized requests are caught either before body reading (via `Content-Length`) or during bounded stream ingestion (via `io.LimitReader(r.Body, maxBody+1)`), returning HTTP `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`).
4. **Zero Partial Forwarding**: When rejected, `r.Body` is cleanly closed and zero bytes are dispatched to upstream application backends.
5. **Byte-Fidelity Forwarding**: Valid requests below the limit are forwarded intact with exact byte fidelity.
6. **Non-Mutating Bypass**: `GET` and `HEAD` requests, or requests with `r.Body == nil` or `ContentLength == 0`, bypass body buffering entirely.

### Overflow Detection Pipeline

```mermaid
flowchart TD
    Req(["Incoming HTTP Request"]) --> BypassCheck{"Method in [GET, HEAD]<br/>or Body == nil<br/>or ContentLength == 0?"}
    
    BypassCheck -- Yes --> Bypass["Direct Bypass<br/>(Zero Buffer Allocation)"] --> Forward["Forward to Upstream Target"]
    
    BypassCheck -- No --> FastFail{"Content-Length > 0 &<br/>Content-Length > max_body_bytes?<br/>(Stage 1: Fast-Fail)"}
    
    FastFail -- Yes --> Reject413Fast["Fast-Path Reject:<br/>1. Close r.Body<br/>2. Log diagnostic warning<br/>3. HTTP 413 Request Entity Too Large<br/>4. Terminate Proxying"]
    Reject413Fast --> Client413(["Client Receives HTTP 413<br/>(Upstream receives 0 bytes)"])
    
    FastFail -- No --> StreamRead["Read Stream via<br/>io.LimitReader(r.Body, maxBody + 1)"]
    
    StreamRead --> ReadErr{"io.ReadAll Error?"}
    ReadErr -- Yes --> BadReq["HTTP 400 Bad Request<br/>Close r.Body & Abort"]
    BadReq --> Client400(["Client Receives HTTP 400"])
    
    ReadErr -- No --> OverflowCheck{"len(bodyBytes) > max_body_bytes?<br/>(Stage 2: Stream Overflow)"}
    
    OverflowCheck -- Yes --> Reject413Stream["Stream Overflow Reject:<br/>1. Close r.Body<br/>2. Discard read bytes<br/>3. Log diagnostic warning<br/>4. HTTP 413 Request Entity Too Large"]
    Reject413Stream --> Client413
    
    OverflowCheck -- No --> ValidBody["In-Bounds Body:<br/>Populate toronReq.Body<br/>ContentLength = len(bodyBytes)"]
    ValidBody --> Forward
```

### Diagnostic Logging on 413 Rejection

When an incoming payload exceeds `max_body_bytes`, the sidecar proxy engine logs a structured diagnostic warning:

```text
[SIDECAR] 413 Payload Too Large: POST /api/upload declared Content-Length 20971520 exceeds limit 10485760
[SIDECAR] 413 Payload Too Large: POST /api/stream request body exceeds limit 10485760
```

---

## 🚀 Running Sidecar Mode Example

### Dual Ingress/Egress Command

```bash
# Start Toron in Sidecar Mode alongside local app container
./toron -config ./sidecar-config.yaml
```

### Verification with cURL

```bash
# 1. Valid request within limit (200 OK)
curl -v -X POST http://127.0.0.1:15006/api/data \
  -H "Content-Type: application/json" \
  -d '{"status":"active"}'

# 2. Oversized declared Content-Length (Immediate 413 Payload Too Large)
curl -v -X POST http://127.0.0.1:15006/api/data \
  -H "Content-Length: 20000000" \
  --data-binary @huge-file.bin

# 3. Oversized chunked stream (Stream bounded 413 Payload Too Large)
curl -v -X POST http://127.0.0.1:15006/api/upload \
  -H "Transfer-Encoding: chunked" \
  --data-binary @huge-stream.bin
```

---

## 🔗 Related Documentation & Code References

* [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103) – Sidecar configuration struct in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go).
* [`SidecarConfig.MaxBodyBytes`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L109) – Maximum request body size field definition.
* [`validateConfigDefaults`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L153) – 10 MB normalization fallback in [`pkg/config/loader.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go).
* [`NewProxyEngine`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L37) – Sidecar proxy constructor and fallback in [`pkg/sidecar/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go).
* [`ProxyEngine.proxyToURL`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L192) – HTTP 413 rejection and payload forwarding implementation.
* [`sidecar_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/sidecar_test.go) – Automated test suite verifying SEC-25 fixes.
* [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374) – Security audit finding record.
* [`REQ-086`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-086.md) – Requirement specification for configurable sidecar body limits.
* [`ADR-081`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-081.md) – Architecture decision record for sidecar body limiting and rejection.
* [`TC-086`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-086.md) – Test case specification for sidecar body bounding.
* [Configuration Options Reference](../reference/config-options.md) – Global configuration options.
* [Configuration Guide](../configuration.md) – Dual-file YAML configuration guide.
