---
title: Toron Documentation Index
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-23

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
  - REQ-087
  - REQ-088
  - REQ-089
  - REQ-090
  - REQ-091
  - REQ-123
  - REQ-124
  - REQ-126
  - REQ-127
  - REQ-128
  - REQ-130
  - REQ-132
  - REQ-133
  - REQ-134
  - TASK-156
  - TASK-157

derived_from:
  - PRD.md
  - ADR-001
  - ADR-022
  - ADR-082
  - ADR-083
  - ADR-084
  - ADR-085
  - ADR-086
  - ADR-123
  - ADR-124
  - ADR-126
  - ADR-127
  - ADR-128
  - ADR-130
  - ADR-132
  - ADR-133
  - ADR-134
  - SEC-26
  - SEC-27
  - SEC-28
  - SEC-29
  - SEC-30

documents:
  - TORON-DOCUMENTATION-INDEX

related_to:
  - getting-started.md
  - configuration.md
  - release-notes.md
---

# Toron Documentation Wiki (v1.5.35 Milestone)

Welcome to the **Toron Web Server** documentation wiki. Toron is an event-driven, high-performance, zero-dependency web server, reverse proxy gateway, and edge security engine written in pure Go.

## Toron at a Glance

**তোরণ** (*to-ron*) means **gateway** in Bengali.

Toron is an event-driven, high-performance web server, reverse proxy, API gateway, and edge security engine written in Go.

- **Project:** [GitHub repository](https://github.com/SayantanSaha/toron_v3)
- **Benchmarks:** [Comparative benchmark suite](https://github.com/SayantanSaha/toron_v3/tree/master/benchmarks)
- **Comparisons:** [Toron vs NGINX, HAProxy, Traefik and Caddy](./features/docker-compare-benchmark.md)
- **Contact:** [sayantan.somu@gmail.com](mailto:sayantan.somu@gmail.com)

## Wiki Navigation

### 🚀 Getting Started & Operations
- [Getting Started](./getting-started.md) – Quickstart guide for building and running Toron.
- [High-Density Gateway Observability Dashboard](./features/observability-dashboard.md) – Pure vector SVG real-time control plane, disaggregated upstream route pool rendering, and live telemetry metric derivations.
- [Universal Installer & Service Manager](./features/installation-guide.md) – Auto-installer script (`install.sh`), systemd (Linux) & launchd (macOS) service setup.
- [Docker Containerization Guide](./features/docker-container.md) – Multi-stage Dockerfile packaging, image creation, and Docker Compose configuration.
- [Configuration Guide](./configuration.md) – Dual-file YAML configuration guide (`config.yaml` & `routes.yaml`).

### ⚙️ Core Architecture & Protocols
- [Inbound Chunked Ingestion & Edge Normalization](./features/inbound-chunked-ingestion.md) – Native HTTP/1.1 chunked request ingestion, zero-tolerance wire decoding (RFC 9112 §7.1), Active Ingress Smuggling Firewall, and upstream Content-Length re-framing.
- [Event Reactor Core](./features/event-reactor.md) – Event-driven concurrency, explicit TCP_NODELAY tuning, adaptive deadline amortization, and worker pool.
- [HTTP/2 Engine](./features/http2.md) – Cleartext `h2c` prior-knowledge and stream multiplexing.
- [HTTP/3 QUIC Protocol Engine](./features/http3.md) – HTTP/3 over QUIC (UDP), concurrent listener, and Alt-Svc upgrade advertising.
- [HTTPS TLS & Auto Dev Certs](./features/tls-https.md) – TLS 1.2/1.3 encryption and ECDSA dev certificate generation.
- [ACME Zero-Touch Production SSL & Protocol Hardening](./features/acme.md) – Automated Let's Encrypt SSL issuance, HTTP-01 & TLS-ALPN-01 challenge responders, and RFC 8555 token validation.
- [WebSocket Tunneling](./features/websocket.md) – RFC 6455 and RFC 8441 Extended CONNECT bi-directional stream tunneling.
- [Static File Serving](./features/static-file-serving.md) – Hosting web apps, MIME resolution, and directory index handling.

### 🌐 Routing, Proxying & Resilience
- [Layer 4 TCP & UDP Transport Proxies](./features/layer4-proxy.md) – Raw stream and datagram proxying with bounded concurrency, buffer recycling, and Slowloris idle deadline protection.
- [REST-to-gRPC Transcoding Engine](./features/grpc-transcoding.md) – Direct JSON REST to binary Protobuf gRPC RPC transcoding, bounded request limits (HTTP 413), hop-by-hop header sanitization, and direct parameterized subpath dispatch.
- [Service Mesh Sidecar Mode](./features/service-mesh-sidecar.md) – Lightweight pod-to-pod mTLS, bounded request body limits (HTTP 413), and weighted traffic splitting.
- [Native Kubernetes Ingress Controller](./features/kubernetes-ingress.md) – Zero-dependency Kubernetes `networking.k8s.io/v1` Ingress Controller.
- [OCI Container Auto-Discovery](./features/oci-container-auto-discovery.md) – Vendor-agnostic Docker & Podman Unix socket container auto-discovery.
- [Reverse Proxy & Gateway Routing](./features/reverse-proxy.md) – Upstream request forwarding, zero-allocation response serialization, proxy headers, configurable transport, and distributed tracing (REQ-123, REQ-127).
- [gRPC Edge Gateway & Health Probing](./features/grpc-gateway.md) – Native `grpc.health.v1` probing and HTTP/2 trailers preservation.
- [Advanced Load Balancing Engine](./features/advanced-load-balancing.md) – 8 strategies: round-robin, weighted, least conn, least latency, sticky cookie, and IP hash.
- [Circuit Breaker & Health Checks](./features/circuit-breaker.md) – 3-state circuit breaker and active upstream health probing.
- [Header-Based HTTP Routing](./features/header-routing.md) – API versioning and conditional header routing.
- [Domain-Based Virtual Host Routing](./features/domain-routing.md) – Multi-tenant host header dispatching.

### 🛡️ Traffic Control, Performance & Security
- [Layered Path Traversal Defense](./features/path-traversal-defense.md) – Route-aware prefix boundary protection, WAF raw wire URI inspection, and fail-fast transport socket teardown (CWE-22).
- [Web Application Firewall (WAF) & Injection Protection](./features/waf.md) – OWASP Top 10 SQLi, XSS, Path Traversal, and RCE threat mitigation.
- [Multi-Tier Threat Defense & OS-Level IP Blocking Guide](./features/os-level-ip-blocking.md) – 2-stage dynamic WAF auto-ban, state persistence, Fail2ban (Linux), pfctl (macOS), and PowerShell Defender Firewall (Windows).
- [CORS Policies & Enterprise Security Headers](./features/cors-security-headers.md) – Preflight OPTIONS handling, origin matching, and OWASP security headers.
- [Transparent Response Compression](./features/compression.md) – Streaming Zstd, Brotli, Gzip & Deflate response compression.
- [In-Memory Response Caching](./features/response-caching.md) – RFC 9111 HTTP response caching, Shared Cache Session Boundary Isolation, Host:Port authority derivation, cross-port cache isolation (CWE-524), dual-stage Set-Cookie purging (CWE-384), and Web Cache Deception Shared Responsibility Model (REQ-134 / TASK-157).
- [Multi-Scheme Authentication](./features/authentication.md) – JWT (HS256), API Key, and HTTP Basic authentication.
- [Native Go Benchmarking](./features/benchmarking.md) – Performance benchmarks and allocation metrics.
- [High-Concurrency Saturation Stress Benchmark & Status Classification](./features/saturation-stress-benchmark.md) – Dual-stream saturation testing, four-tier status classification taxonomy, and Table 6 metrics (BMK-04, HARN-01).
- [Heterogeneous Multi-Hop Origin Testbed](./features/multihop-testbed.md) – Dual-mode (standalone & Docker Compose) evaluation across Node.js (llhttp), Python (uvicorn/h11), and Go (net/http) origins, wire-level protocol adapters, and zero-desync canary validation (BMK-03, REQ-120).
- [Multi-Proxy Differential Docker Benchmark](./features/docker-compare-benchmark.md) – Automated comparative benchmarking comparing Toron against NGINX, Traefik, Caddy, and HAProxy fronting identical heterogeneous upstreams (REQ-121, REQ-122).
- [Protocol Invariant Regression Suite & Generative Fuzzing Engine](./features/differential-fuzzer-metrics.md) – Wire-level rejection latency profiling (Equation 7, K=1,000) and coverage-guided generative differential fuzzing against Go standard library (REQ-118, REQ-132).
- [Dummy Microservices Suite](./features/dummy-services.md) – Cluster of 10 test microservices.

### 📖 References
- [CLI Reference](./reference/cli.md) – Command-line interface options and usage flags.
- [Configuration Options Reference](./reference/config-options.md) – Complete reference for YAML configuration settings.
- [HTTP API Reference](./reference/api.md) – Built-in health, metrics, and internal management endpoints.

### 💡 Help & Support
- [Troubleshooting Guide](./troubleshooting.md) – Common runtime issues and solutions.
- [Frequently Asked Questions (FAQ)](./faq.md) – Common questions about Toron.
- [Release Notes](./release-notes.md) – Changelog and release milestones (v1.0.0–v1.5.35).

