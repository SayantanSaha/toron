---
title: Toron Documentation Index
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-08

depends_on:
  - REQ-001
  - REQ-007
  - REQ-019
  - REQ-027
  - REQ-033
  - REQ-034
  - REQ-035
  - REQ-036
  - REQ-086

derived_from:
  - PRD.md
  - ADR-001
  - ADR-022

documents:
  - TORON-DOCUMENTATION-INDEX

related_to:
  - getting-started.md
  - configuration.md
  - release-notes.md
---

# Toron Documentation Wiki (v1.5.6 Release)

Welcome to the **Toron Web Server** documentation wiki. Toron is an event-driven, high-performance, zero-dependency web server, reverse proxy gateway, and edge security engine written in pure Go.

## Wiki Navigation

### 🚀 Getting Started & Operations
- [Getting Started](./getting-started.md) – Quickstart guide for building and running Toron.
- [Universal Installer & Service Manager](./features/installation-guide.md) – Auto-installer script (`install.sh`), systemd (Linux) & launchd (macOS) service setup.
- [Docker Containerization Guide](./features/docker-container.md) – Multi-stage Dockerfile packaging, image creation, and Docker Compose configuration.
- [Configuration Guide](./configuration.md) – Dual-file YAML configuration guide (`config.yaml` & `routes.yaml`).

### ⚙️ Core Architecture & Protocols
- [Event Reactor Core](./features/event-reactor.md) – Event-driven concurrency, non-blocking I/O, and worker pool.
- [HTTP/2 Engine](./features/http2.md) – Cleartext `h2c` prior-knowledge and stream multiplexing.
- [HTTP/3 QUIC Protocol Engine](./features/http3.md) – HTTP/3 over QUIC (UDP), concurrent listener, and Alt-Svc upgrade advertising.
- [HTTPS TLS & Auto Dev Certs](./features/tls-https.md) – TLS 1.2/1.3 encryption and ECDSA dev certificate generation.
- [WebSocket Tunneling](./features/websocket.md) – RFC 6455 and RFC 8441 Extended CONNECT bi-directional stream tunneling.
- [Static File Serving](./features/static-file-serving.md) – Hosting web apps, MIME resolution, and directory index handling.

### 🌐 Routing, Proxying & Resilience
- [REST-to-gRPC Transcoding Engine](./features/grpc-transcoding.md) – Direct JSON REST to binary Protobuf gRPC RPC transcoding.
- [Service Mesh Sidecar Mode](./features/service-mesh-sidecar.md) – Lightweight pod-to-pod mTLS, bounded request body limits (HTTP 413), and weighted traffic splitting.
- [Native Kubernetes Ingress Controller](./features/kubernetes-ingress.md) – Zero-dependency Kubernetes `networking.k8s.io/v1` Ingress Controller.
- [OCI Container Auto-Discovery](./features/oci-container-auto-discovery.md) – Vendor-agnostic Docker & Podman Unix socket container auto-discovery.
- [Reverse Proxy & Gateway Routing](./features/reverse-proxy.md) – Upstream request forwarding and proxy headers.
- [gRPC Edge Gateway & Health Probing](./features/grpc-gateway.md) – Native `grpc.health.v1` probing and HTTP/2 trailers preservation.
- [Advanced Load Balancing Engine](./features/advanced-load-balancing.md) – 8 strategies: round-robin, weighted, least conn, least latency, sticky cookie, and IP hash.
- [Circuit Breaker & Health Checks](./features/circuit-breaker.md) – 3-state circuit breaker and active upstream health probing.
- [Header-Based HTTP Routing](./features/header-routing.md) – API versioning and conditional header routing.
- [Domain-Based Virtual Host Routing](./features/domain-routing.md) – Multi-tenant host header dispatching.

### 🛡️ Traffic Control, Performance & Security
- [Web Application Firewall (WAF) & Injection Protection](./features/waf.md) – OWASP Top 10 SQLi, XSS, Path Traversal, and RCE threat mitigation.
- [CORS Policies & Enterprise Security Headers](./features/cors-security-headers.md) – Preflight OPTIONS handling, origin matching, and OWASP security headers.
- [Transparent Response Compression](./features/compression.md) – Streaming Zstd, Brotli, Gzip & Deflate response compression.
- [In-Memory Response Caching](./features/response-caching.md) – RFC 7234 HTTP response caching and telemetry headers.
- [Multi-Scheme Authentication](./features/authentication.md) – JWT (HS256), API Key, and HTTP Basic authentication.
- [Native Go Benchmarking](./features/benchmarking.md) – Performance benchmarks and allocation metrics.
- [Dummy Microservices Suite](./features/dummy-services.md) – Cluster of 10 test microservices.

### 📖 References
- [CLI Reference](./reference/cli.md) – Command-line interface options and usage flags.
- [Configuration Options Reference](./reference/config-options.md) – Complete reference for YAML configuration settings.
- [HTTP API Reference](./reference/api.md) – Built-in health, metrics, and internal management endpoints.

### 💡 Help & Support
- [Troubleshooting Guide](./troubleshooting.md) – Common runtime issues and solutions.
- [Frequently Asked Questions (FAQ)](./faq.md) – Common questions about Toron.
- [Release Notes](./release-notes.md) – Changelog and release milestones (v1.0.0–v1.5.6).
