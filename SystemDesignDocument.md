# Toron System Design Document

## 1. Overview

**Toron** is a high-performance, Go-based, extensible web server, reverse proxy, and API gateway. Its design goals include simplicity, modularity, performance, and support for modern web protocols and deployment environments. Toron handles both Layer 4 (TCP/UDP) and Layer 7 (HTTP/HTTPS/gRPC/WebSocket) traffic and includes a dynamic configuration system, plugin architecture, and security mechanisms like WAF and rate limiting.

---

## 2. Architecture Diagram

```
Client ─▶ Listener ─▶ Protocol Handler ─▶ Router ─▶ Middleware Chain ─▶ Load Balancer ─▶ Backend
                      │                      │                        │
                      └──── Admin API ◀──── Config Loader ◀───── File / Dynamic Configs
```

---

## 3. Component Breakdown

### 3.1 Listeners

- Accept traffic over supported protocols: HTTP/1.1, HTTP/2, HTTP/3 (QUIC), gRPC, WebSocket, TCP/UDP
- Protocol negotiation and TLS/mTLS handling
- Multiplexed and concurrent connection management

### 3.2 Protocol Handlers

- HTTP handler (net/http, http2)
- QUIC (quic-go)
- gRPC support with custom metadata forwarding
- TCP/UDP raw socket forwarding
- WebSocket bridging with control frame handling

### 3.3 Router

- Layer 7 routing (path, host, method, headers)
- Layer 4 routing (IP, port, SNI)
- Hierarchical routing tree for performance
- Runtime route reload on config change

### 3.4 Middleware Chain

- JWT/OAuth2/API key authentication
- Rate limiting (in-memory/Redis distributed)
- Request validation, header manipulation, IP filtering
- Caching (Redis/in-memory), compression
- OpenTelemetry tracing injection
- WAF (regex and OWASP ruleset evaluation)

### 3.5 Load Balancer

- Strategies: Round-robin, least-connections, IP-hash
- Custom plugins for intelligent load balancing
- Backend health checks (active/passive)

### 3.6 Service Discovery

- Static configuration
- File watcher
- DNS polling
- Consul/Etcd client
- (Optional) Kubernetes CRD watcher

### 3.7 Plugin System

- Build-time: Go interfaces for middleware, routing, load balancing, etc.
- Runtime: WASM sandboxed plugin loader
- Safe dynamic module loading (e.g., auth, rate-limit, observability plugins)

### 3.8 Admin Interface

- REST API for config, health, metrics, plugin mgmt
- Planned: Web UI dashboard
- RBAC for operations and plugin control

---

## 4. Configuration System

- Supported formats: YAML / JSON / TOML
- Hot-reload with validation
- Env substitution and secrets via external vaults
- Modular: listeners, routes, middleware, backends

---

## 5. Observability

- Structured logs (zap)
- Prometheus metrics endpoint
- OpenTelemetry-compatible tracing export
- Configurable event logging (request/response headers, status codes)

---

## 6. Security

- TLS/mTLS with SNI support
- Pluggable auth: JWT, OAuth2, LDAP
- Web Application Firewall (WAF) engine:
  - OWASP top 10 defense
  - Regex rules
- DDoS Mitigation:
  - IP bans and blacklists
  - SYN flood detection
  - Rate limiting per IP/user

---

## 7. Deployment Modes

- Single static binary (Golang build)
- Dockerized deployment (multi-arch)
- Optional Helm chart (for Kubernetes, not mandatory)

---

## 8. Future Enhancements

- Plugin Marketplace with signature scanning
- AI-assisted config validator and recommender
- Visual routing and config editor
- Traffic replay and simulation mode
- Autoscaler plugin for backend targets

---

## 9. Tech Stack

- **Language:** Go
- **Libs:** chi, zap, promhttp, quic-go, otel-go, wasm-go
- **Data Stores:** Redis (optional), YAML/JSON config files
- **Auth:** jose, ldap-go, oauth2

---

## 10. Design Principles

- **Extensibility:** via plugins and WASM
- **Performance:** through native concurrency, zero-copy routing
- **Security-first:** default-deny config, safe plugin sandboxing
- **Single Responsibility:** modular component architecture
- **Observability-by-default:** logs, metrics, and tracing enabled

---

## 11. Known Limitations (WIP)

- No UI dashboard yet
- Dynamic cluster-wide config replication (planned)
- AI/ML analytics not yet available

---

## 12. Contact & Contribution

- GitHub Issues and PRs welcome
- Slack/Discord (TBD)
- Contributors: [Sayantan Saha](https://github.com/SayantanSaha)

---

**Toron** — Bridging modern protocols with elegant engineering.
