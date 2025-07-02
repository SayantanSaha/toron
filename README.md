# Toron: A Modern Go-Based Web Server, Reverse Proxy, and API Gateway

**Project Status:** In Development
**License:** MIT or Apache 2.0 *(TBD)*

---

## 🌉 What is Toron?

**Toron** (Bengali: তোরণ — meaning *gateway* or *portal*) is a fast, extensible, single-binary reverse proxy and API gateway written in Go. Designed for modern workloads, it seamlessly supports HTTP/1.1, HTTP/2, HTTP/3, gRPC, WebSocket, TCP, and UDP traffic. Built with simplicity, performance, and security at its core, Toron bridges services across diverse protocols, dynamically discovering, routing, securing, and transforming traffic.

---

## 🚀 Key Features

### ⚙️ Protocol Support

- HTTP/1.1, HTTP/2, HTTP/3 (QUIC)
- gRPC (with metadata passthrough)
- WebSocket full-duplex streaming
- Raw TCP/UDP proxying with L4 routing

### 🔁 Routing & Load Balancing

- Layer 7 routing (path, host, method, header)
- Layer 4 routing (IP, port, SNI)
- Load balancing: Round-robin, Least Connections, IP-hash
- Sticky sessions and health checks

### 🔍 Service Discovery

- File-based, DNS, Consul, Etcd
- Kubernetes via CRDs (optional)
- Auto-reload routes on config change

### 🔐 Security & Access Control

- JWT, API Key, OAuth2, LDAP
- Role- and attribute-based access control (RBAC/ABAC)
- TLS termination & mTLS
- Built-in WAF (OWASP rules)
- DDoS protection: IP banning, SYN flood defense

### 🧩 Extensibility

- WASM plugin engine
- Custom middleware, auth, metrics, and protocol handlers
- Lua/JS scripting (optional)

### 📈 Observability

- Structured logging (zap)
- Prometheus metrics
- OpenTelemetry tracing
- Admin API and UI dashboard (coming soon)

### ⚡ Deployment

- Single static binary (no dependencies)
- Docker image (multi-arch)
- Optional Helm charts (Kubernetes)

---

## 🧱 Architecture Overview

```
Client ─▶ Listener ─▶ Protocol Handler ─▶ Router ─▶ Middleware Chain ─▶ Load Balancer ─▶ Backend
                      │                      │                        │
                      └──── Admin API ◀──── Config Loader ◀───── File / Dynamic Configs
```

For detailed architecture, see the [System Design Document](#).

---

## 🛠️ Getting Started

### 📦 Prerequisites

- Go 1.22+
- (Optional) Redis (for rate limiting or caching)

### 🧪 Run from source

```bash
git clone https://github.com/SayantanSaha/toron.git
cd toron
go run main.go --config=config.yaml
```

### 🐳 Docker (coming soon)

```bash
docker run -p 8080:8080 toron/toron:latest
```

---

## 🧾 Configuration

Toron supports:

- YAML / JSON / TOML configuration files
- Live reload on file changes
- Secrets via env or external vault

Example:

```yaml
listeners:
  - protocol: http
    port: 8080
routes:
  - match:
      path: /api/v1
    backend:
      url: http://localhost:9000
    middlewares:
      - jwt
      - rate-limit
```

---

## 🧩 Plugin System

- Build-time: Go interfaces
- Runtime: WASM plugin engine
- Plugin types:
  - Middleware
  - Protocol Handlers
  - Load Balancer
  - Auth Provider

Example middleware interface:

```go
type Middleware interface {
  Name() string
  Process(*Context, NextFunc) error
}
```

---

## 🎯 Roadmap

- ✅ Phase 1: HTTP core, routing, admin API
- ⏳ Phase 2: gRPC, WebSocket, TCP/UDP, QUIC
- 🔐 Phase 3: WAF, DDoS, caching, developer portal
- 🔮 Phase 4: AI-powered config tuning, plugin marketplace

See [Trello-style Roadmap](#) and [GitHub Issues](../../issues)

---

## 🤝 Contributing

We welcome contributions! Please:

- Review our [CONTRIBUTING.md](CONTRIBUTING.md)
- Follow the [code of conduct](CODE_OF_CONDUCT.md)
- Open issues or PRs with clear description

---

## 📄 License

MIT OR Apache 2.0 (choose one and update)

---

## 📬 Contact / Community

- Twitter: [@sayantansaha\_](https://twitter.com/sayantansaha_)
- Discussions: [GitHub Discussions](../../discussions)
- Email: [[you@example.com](mailto\:you@example.com)]

---

**Toron** — A blazing gateway, built for the next generation of infrastructure.

---
