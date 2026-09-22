---
title: Service Mesh Sidecar Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-09-08
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
  - SERVICE-MESH-GUIDE

related_to:
  - features/service-mesh-sidecar.md
  - configuration.md
  - reference/config-options.md
---

# 🕸️ Service Mesh Sidecar Guide

> [!NOTE]
> For the comprehensive feature documentation and architecture guide, see [Service Mesh Sidecar Feature Guide](./features/service-mesh-sidecar.md).

Toron Edge Gateway provides a lightweight, zero-dependency **Service Mesh Sidecar Mode** (`pkg/sidecar`) designed to run as a local container within Kubernetes pods (`127.0.0.1`).

## Core Capabilities

- **Zero-Trust Mutual TLS (mTLS)**: Enforces strict pod-to-pod encrypted communication (`strict_mtls: true`).
- **Weighted Traffic Splitting**: Dynamic canary and A/B release traffic distribution.
- **Dual-Mode Operation**: Concurrent ingress (port 15006) and egress (port 15001) proxying.
- **Configurable Request Body Limits & HTTP 413 Protection (`SEC-25`)**: Bounded request body ingestion configured via `SidecarConfig.MaxBodyBytes` (`max_body_bytes`), defaulting to a 10 MB fallback (`10 * 1024 * 1024` bytes). Requests exceeding the limit are immediately rejected with HTTP `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`) without silent truncation or upstream data corruption.

## Quick Configuration (`toron.yaml`)

```yaml
sidecar:
  enabled: true
  mode: "dual"              # "ingress", "egress", or "dual"
  ingress_port: 15006       # Inbound mTLS port
  egress_port: 15001        # Outbound proxy port
  app_port: 8080            # Target application container (127.0.0.1:8080)
  max_body_bytes: 10485760  # Maximum request body size (default: 10MB)
  strict_mtls: true
  cert_file: "./certs/sidecar.crt"
  key_file: "./certs/sidecar.key"
  ca_file: "./certs/ca.crt"
  traffic_splits:
    - prefix: "/api"
      backends:
        - target: "http://service-v1:8080"
          weight: 80
        - target: "http://service-v2:8080"
          weight: 20
```

## Related References

- [Full Service Mesh Sidecar Documentation](./features/service-mesh-sidecar.md)
- [Configuration Guide](./configuration.md)
- [Configuration Options Reference](./reference/config-options.md)
- `SidecarConfig`
- `ProxyEngine`
