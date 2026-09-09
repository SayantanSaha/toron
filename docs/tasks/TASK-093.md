---
id: TASK-093
type: task
title: Layer 4 Proxy Configuration Schema & Wiring
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-087

owns:
  - pkg/config/config.go
  - cmd/toron/main.go

references:
  - REQ-087
  - SEC-26
  - SR-081
  - ADR-082
  - TC-087

derived_from:
  - REQ-087

implements:
  - REQ-087

verified_by:
  - TC-087
---

# TASK-093 - Layer 4 Proxy Configuration Schema & Wiring

## Overview

Extend [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) to support Layer 4 transport proxy tuning parameters: `MaxConnections` (maximum concurrent TCP connections), `IdleTimeout` (stagnant connection / session teardown deadline), and `MaxWorkers` (maximum concurrent UDP packet processing workers). Implement helper getters with safe fallback defaults, and update the Layer 4 proxy instantiation logic in [`cmd/toron/main.go`](file:///Users/sneha/Developer/toron-research/toron/cmd/toron/main.go#L390-L429) to wire these parameters into [`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L15) and [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L14), remediating [`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383).

## Scope & Implementation Breakdown

1. **Extend [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) Struct (`pkg/config/config.go`)**:
   - Add fields with YAML and JSON struct tags:
     - `MaxConnections int` (`` `yaml:"max_connections,omitempty" json:"max_connections,omitempty"` ``): Maximum concurrent TCP client connections.
     - `IdleTimeout time.Duration` (`` `yaml:"idle_timeout,omitempty" json:"idle_timeout,omitempty"` ``): Inactivity timeout before closing idle TCP streams or expiring UDP sessions.
     - `MaxWorkers int` (`` `yaml:"max_workers,omitempty" json:"max_workers,omitempty"` ``): Maximum concurrent UDP worker goroutines / processing capacity.

2. **Implement Helper Getters on [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) (`pkg/config/config.go`)**:
   - `func (p *ProxyRouteConfig) GetMaxConnections() int`: Returns `p.MaxConnections` if `> 0`; otherwise defaults to `10000`.
   - `func (p *ProxyRouteConfig) GetIdleTimeout() time.Duration`: Returns `p.IdleTimeout` if `> 0`; otherwise defaults to `60 * time.Second`.
   - `func (p *ProxyRouteConfig) GetMaxWorkers() int`: Returns `p.MaxWorkers` if `> 0`; otherwise defaults to `1024`.

3. **Wire Configuration into Main Gateway Service ([`cmd/toron/main.go`](file:///Users/sneha/Developer/toron-research/toron/cmd/toron/main.go))**:
   - In the `pr.IsTCP()` route initialization branch:
     - Extract `maxConns := pr.GetMaxConnections()` and `idleTimeout := pr.GetIdleTimeout()`.
     - Pass these settings when constructing [`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L15) (e.g., via options or constructor parameters).
     - Update diagnostic log output:
       `log.Printf("[TORON] Configuring Layer 4 TCP Stream Proxy: listen_port %d -> targets %v [max_connections: %d, idle_timeout: %s]", port, targets, maxConns, idleTimeout)`
   - In the `pr.IsUDP()` route initialization branch:
     - Extract `maxWorkers := pr.GetMaxWorkers()` and `idleTimeout := pr.GetIdleTimeout()`.
     - Pass these settings when constructing [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L14) (e.g., via options or constructor parameters).
     - Update diagnostic log output:
       `log.Printf("[TORON] Configuring Layer 4 UDP Datagram Proxy: listen_port %d -> targets %v [max_workers: %d, idle_timeout: %s]", port, targets, maxWorkers, idleTimeout)`

## Acceptance Criteria

- [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) exports `MaxConnections`, `IdleTimeout`, and `MaxWorkers` with appropriate serialization tags.
- Helper methods provide guaranteed fallbacks: `GetMaxConnections() == 10000`, `GetIdleTimeout() == 60s`, and `GetMaxWorkers() == 1024` when unset or non-positive.
- [`cmd/toron/main.go`](file:///Users/sneha/Developer/toron-research/toron/cmd/toron/main.go#L390-L429) propagates route configuration into both TCP and UDP proxy constructors.
- Diagnostic startup logs report configured limits and timeouts for Layer 4 routes.
- Zero external/third-party dependencies (strictly Go standard library).

## Blockers & Risks

- Ensure backward compatibility of [`NewTCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L26) and [`NewUDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L25) constructors or utilize functional options to prevent breaking existing unit tests.
