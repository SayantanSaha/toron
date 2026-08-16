---
title: Service Mesh Sidecar Mode
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-16
updated: 2026-08-16

depends_on:
  - REQ-048

derived_from:
  - ADR-043

documents:
  - SERVICE-MESH-SIDECAR-GUIDE

related_to:
  - kubernetes-ingress.md
  - tls-https.md
  - configuration.md
---

# 🕸️ Service Mesh Sidecar Mode (`pkg/sidecar`)

Toron Edge Gateway features a lightweight, zero-dependency **Service Mesh Sidecar Mode** (`pkg/sidecar`). In Sidecar Mode, Toron runs alongside pod application containers (`127.0.0.1`), transparently enforcing pod-to-pod Mutual TLS (mTLS) zero-trust encryption and dynamic weighted traffic splitting (e.g. 80/20 canary releases) with minimal memory overhead (<10MB RAM).

---

## 🌟 Key Features

* **Zero External Dependencies**: Implements mTLS certificate verification and weighted traffic splitting using Go stdlib `crypto/tls` and `net/http`.
* **Pod-to-Pod Strict mTLS**: Enforces `client_auth: "require_and_verify"` with Root CA certificate pools.
* **Weighted Traffic Splitting**: Distributes outbound egress traffic across multiple canary backends according to assigned integer weights (e.g. 80% to v1, 20% to v2).
* **Ingress / Egress / Dual Modes**: Flexible operational modes running on dedicated local ports (Ingress: 15006, Egress: 15001).

---

## ⚙️ Configuration Reference (`toron.yaml`)

```yaml
sidecar:
  enabled: true
  mode: "dual"              # Operational mode: "ingress", "egress", or "dual"
  ingress_port: 15006       # Pod inbound mTLS listener port
  egress_port: 15001        # Pod outbound proxy listener port
  app_port: 8080            # Local app container target port (127.0.0.1:8080)
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

---

## 🚀 Running Sidecar Mode Example

### Dual Ingress/Egress Command

```bash
# Start Toron in Sidecar Mode alongside local app container
./toron -config ./sidecar-config.yaml
```
