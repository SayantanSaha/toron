---
title: OCI Container Auto-Discovery Engine
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-16
updated: 2026-08-16

depends_on:
  - REQ-046

derived_from:
  - ADR-041

documents:
  - OCI-CONTAINER-AUTO-DISCOVERY-GUIDE

related_to:
  - reverse-proxy.md
  - domain-routing.md
  - configuration.md
---

# 🐳 Vendor-Agnostic OCI Container Auto-Discovery Engine (`pkg/discovery`)

Toron Edge Gateway features a vendor-agnostic, zero-dependency **OCI Container Auto-Discovery Engine** (`pkg/discovery`). It monitors container runtime Unix domain sockets in real time and automatically registers/deregisters upstream backend targets in Toron's routing matrix with zero downtime.

---

## 🌟 Supported Container Runtimes & Engines

Toron communicates directly over Unix domain sockets (`unix://`) using pure Go stdlib HTTP transport, eliminating the need for 3rd-party Docker or Containerd SDKs:

* **Docker Engine** (`/var/run/docker.sock`)
* **Podman** (`/run/podman/podman.sock` or rootless `/run/user/$UID/podman/podman.sock`)
* **Finch** (AWS OCI toolchain)
* **Nerdctl** (containerd CLI)

---

## ⚙️ Configuration Reference (`toron.yaml`)

To enable container auto-discovery, add the `discovery:` section to `toron.yaml`:

```yaml
discovery:
  enabled: true
  engine: "auto"              # Options: "auto", "docker", "podman"
  socket_path: "auto"          # Auto-probes standard socket locations if "auto"
  poll_interval: 10s           # Fallback periodic scan interval
  default_weight: 1            # Default round-robin balancing weight
```

---

## 🏷️ Standard Container Label Taxonomy

Attach `toron.*` metadata labels when starting containers:

| Label Key | Type | Description | Example |
| :--- | :--- | :--- | :--- |
| **`toron.enable`** | `boolean` | **Required**. Opt-in flag (`"true"` / `"1"`) | `toron.enable: "true"` |
| **`toron.host`** | `string` | Domain host rule for routing | `toron.host: "api.example.com"` |
| **`toron.prefix`** | `string` | Subpath prefix rule for routing | `toron.prefix: "/v1/users"` |
| **`toron.port`** | `integer` | Target port exposed inside container | `toron.port: "8080"` |
| **`toron.weight`** | `integer` | Load balancing weight | `toron.weight: "5"` |
| **`toron.health_check`**| `string` | Optional HTTP health probe path | `toron.health_check: "/healthz"` |

---

## 🚀 Running Containers Example

### Docker CLI Example

```bash
docker run -d \
  --name user-service-1 \
  --label "toron.enable=true" \
  --label "toron.host=api.example.com" \
  --label "toron.prefix=/v1/users" \
  --label "toron.port=8080" \
  --label "toron.weight=10" \
  my-user-api:latest
```

### Podman CLI Example

```bash
podman run -d \
  --name order-service-1 \
  --label "toron.enable=true" \
  --label "toron.host=shop.example.com" \
  --label "toron.port=9090" \
  my-order-api:latest
```

---

## 🔄 Real-Time Lifecycle Behavior

1. **Container Start (`EventStart`)**: Toron intercepts container startup events via Unix socket HTTP streaming (`/events`), parses `toron.*` labels, and dynamically inserts the container's IP/port into `router.Router` upstreams.
2. **Container Stop/Die (`EventStop` / `EventDie`)**: Toron immediately detects container shutdown, removes the target from load balancing pools, and cleans up route entries without dropping active request connections.
