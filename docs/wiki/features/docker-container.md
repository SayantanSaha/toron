# 🐳 Toron Edge Gateway — Docker Containerization Guide

Toron Edge Gateway is packaged as a multi-stage, ultra-lightweight Docker appliance (~15 MB image size) based on Alpine Linux with zero third-party runtime dependencies.

---

## ⚡ Quickstart Container Commands

```bash
# Build the Toron Docker image using Makefile
make docker-build

# Or build directly with Docker CLI
docker build -t toron:latest .

# Run Toron container with Docker socket auto-discovery
docker run -d \
  --name toron-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /etc/toron/config.yaml:/etc/toron/config.yaml \
  toron:latest
```

---

## 🏗️ Multi-Stage Docker Architecture

1. **Stage 1 (`builder`)**: Uses `golang:alpine` to statically compile `cmd/toron/main.go` with `CGO_ENABLED=0 GOOS=linux`.
2. **Stage 2 (`runner`)**: Uses `alpine:latest` with CA certificates (`ca-certificates`), timezone data (`tzdata`), `/etc/toron` default configuration, and static Web Control Center UI files (`/etc/toron/public/`).

---

## 🔌 Exposed Container Ports

| Port | Protocol | Description |
| :--- | :--- | :--- |
| **`8080`** | `HTTP` | Primary HTTP Gateway & Web Control Center Dashboard |
| **`8443`** | `HTTPS` | TLS 1.2/1.3 Gateway & Automated ACME Endpoint |
| **`8090`** | `TCP` | Layer 4 Stream Proxy Port |
| **`8091`** | `UDP` | Layer 4 Datagram Proxy Port |

---

## 🐳 Docker Compose Deployment

Toron can be run alongside containerized upstream microservices using `docker compose`:

```yaml
version: '3.8'

services:
  toron:
    image: toron:latest
    build: .
    container_name: toron-edge-gateway
    ports:
      - "8080:8080"
      - "8443:8443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./config.yaml:/etc/toron/config.yaml
      - ./routes.yaml:/etc/toron/routes.yaml
    restart: always
```
