---
id: TASK-146
type: task
title: Implementation of Configurable Upstream Reverse Proxy Transport Architecture
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-14
updated: 2026-09-14

depends_on:
  - TASK-145

derived_from:
  - REQ-123

implements:
  - REQ-123

decided_by:
  - ADR-123

verified_by:
  - TC-123

related_to:
  - REQ-123
  - ADR-123
  - TC-123
  - CR-119
  - SR-123
---

# TASK-146 - Implementation of Configurable Upstream Reverse Proxy Transport Architecture

## 1. Description & Context
Implement engineering changes across `pkg/config/`, `pkg/proxy/`, and `cmd/toron/` to allow reverse proxy transport characteristics (connection pooling, egress routing, decompression, keepalive isolation, and upstream HTTP/2) to be configured globally in `config.yaml` and overridden per-route in `routes.yaml`, while retaining default "raw speed" performance when configurations are omitted, as specified in [`REQ-123`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-123.md).

---

## 2. Work Packages & Subtasks

### TASK-146.1 (WP-1): Declarative Schema in `pkg/config/config.go`
- Define `ProxyTransportConfig` struct:
  - `Profile` (`profile`, string)
  - `MaxIdleConns` (`max_idle_conns`, int)
  - `MaxIdleConnsPerHost` (`max_idle_conns_per_host`, int)
  - `MaxConnsPerHost` (`max_conns_per_host`, int)
  - `IdleConnTimeout` (`idle_conn_timeout`, time.Duration)
  - `DisableCompression` (`disable_compression`, *bool)
  - `UseEnvProxy` (`use_env_proxy`, *bool)
  - `ProxyURL` (`proxy_url`, string)
  - `PropagateUpstreamClose` (`propagate_upstream_close`, *bool)
  - `ForceAttemptHTTP2` (`force_attempt_http2`, *bool)
- Add `Transport ProxyTransportConfig` to `ProxyConfig`.
- Add `Transport *ProxyTransportConfig` to `ProxyRouteConfig`.
- Implement `DefaultProxyTransportConfig(profile string) ProxyTransportConfig` and merge resolver `(p *ProxyRouteConfig) ResolveTransport(global ProxyTransportConfig) ProxyTransportConfig`.

### TASK-146.2 (WP-2): Proxy Transport Factory in `pkg/proxy/proxy.go`
- Extend `ProxyOptions` with `Transport ProxyTransportConfig`.
- In `NewProxyWithOptions`, initialize `http.Transport` dynamically:
  - **Proxy Dialing**: Resolve `ProxyURL` $\rightarrow$ `UseEnvProxy` $\rightarrow$ `nil`.
  - **Connection Pooling**: Apply `MaxIdleConns`, `MaxIdleConnsPerHost`, `MaxConnsPerHost`, and `IdleConnTimeout`.
  - **Compression**: Apply `DisableCompression`.
  - **HTTP/2**: Apply `ForceAttemptHTTP2`.
- In `ServeHTTPWithPrefix`: Strip RFC 7230 hop-by-hop headers unconditionally per REQ-071, and honor `PropagateUpstreamClose` when origin signals close.

### TASK-146.3 (WP-3): Route Registration Plumbing in `cmd/toron/main.go`
- In `cmd/toron/main.go`, resolve route transport config by merging `appCfg.Proxy.Transport` with `pr.Transport` before calling `r.RoutePrefix`.

### TASK-146.4 (WP-4): Verification Test Suite in `pkg/proxy/` and `pkg/config/`
- Develop comprehensive table-driven tests verifying:
  - Default raw speed configuration preservation.
  - Route override taking precedence over global settings.
  - Concurrency limiting (`MaxConnsPerHost > 0`) backpressure.
  - Transparent gzip decompression when `DisableCompression: false`.
  - Upstream close propagation when `PropagateUpstreamClose: true`.
  - Upstream HTTP/2 negotiation when `ForceAttemptHTTP2: true`.
