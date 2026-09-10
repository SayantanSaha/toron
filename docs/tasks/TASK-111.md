---
id: TASK-111
type: task
title: RemoteAddr Binding & Extraction Helpers in httpparser.Request and Ingress Protocol Adapters
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-10
updated: 2026-09-10

depends_on:
  - REQ-092

owns:
  - pkg/httpparser/request.go
  - pkg/server/server.go

references:
  - REQ-092
  - SEC-31
  - SR-091
  - ADR-087
  - TC-092

derived_from:
  - REQ-092
  - SEC-31
  - SR-091

implements:
  - REQ-092

verified_by:
  - TC-092

decided_by:
  - ADR-087

related_to:
  - SEC-31
  - SR-091
  - REQ-001
  - REQ-071
  - REQ-030
---

# TASK-111 - RemoteAddr Binding & Extraction Helpers in httpparser.Request and Ingress Protocol Adapters

## Description

Extend the core request model [`httpparser.Request`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/request.go#L58-L70) and protocol ingress adapters across HTTP/1.1, HTTP/2, and HTTP/3 to establish an immutable binding to the client's physical remote network address. This addresses the root architectural defect documented in [`SEC-31`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L428-L436) and [`SR-091 Finding 1`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L78-L99), where `httpparser.NewRequestFromStd` discarded `r.RemoteAddr` and left `RawConn` as `nil` for multiplexed protocols.

## Scope & Implementation Breakdown

1. **Request Struct Extension (`pkg/httpparser/request.go`)**:
   - Add exported field `RemoteAddr string` to `httpparser.Request` ([`pkg/httpparser/request.go:Request`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/request.go#L58-L70)).
   - Document `RemoteAddr` as the physical peer network address (`"IP:port"`) assigned by the transport listener.

2. **Physical Address Extraction Helpers (`pkg/httpparser/request.go`)**:
   - Implement `func (r *Request) RemoteHost() string`:
     - Safely separates the host/IP from the port using standard library `net.SplitHostPort`.
     - If `net.SplitHostPort` fails (e.g. bare IP address without port or invalid port formatting), falls back to `strings.TrimSpace(r.RemoteAddr)`.
     - If `r.RemoteAddr` is empty but `r.RawConn != nil && r.RawConn.RemoteAddr() != nil`, falls back to extracting host from `r.RawConn.RemoteAddr().String()`.
     - Returns an empty string `""` if neither `RemoteAddr` nor `RawConn` yields an address.
   - Implement `func (r *Request) RemoteIP() net.IP`:
     - Calls `r.RemoteHost()`.
     - Strips any surrounding IPv6 brackets (`"["` and `"]"`) if present.
     - Parses and returns `net.IP` via `net.ParseIP`. Supports both IPv4 and IPv6 representations.
     - Returns `nil` if the host string is empty or fails IP parsing.

3. **HTTP/2 & HTTP/3 Protocol Adapter Ingress Binding (`pkg/httpparser/request.go`, `pkg/server/server.go`)**:
   - In [`httpparser.NewRequestFromStd(r *http.Request)`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/request.go#L124):
     - Bind `req.RemoteAddr = r.RemoteAddr`.
   - In [`pkg/server/server.go:http2AdapterHandler`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L313-L353):
     - Ensure all incoming HTTP/2 requests converted via `NewRequestFromStd(r)` preserve `req.RemoteAddr`.
   - In [`pkg/server/server.go:ListenAndServeH3`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L406-L437):
     - Verify incoming HTTP/3 QUIC datagram requests routed through `http2AdapterHandler` preserve the UDP peer remote address in `req.RemoteAddr`.

4. **HTTP/1.1 Core Reactor Connection Binding (`pkg/server/server.go:handleConn`)**:
   - In [`pkg/server/server.go:handleConn`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L240):
     - When populating connection metadata on parsed requests (`req.RawConn = conn`), also populate:
       `if conn != nil && conn.RemoteAddr() != nil { req.RemoteAddr = conn.RemoteAddr().String() }`.
     - Ensures protocol parity across HTTP/1.1, HTTP/2, and HTTP/3.

## Acceptance Criteria

- `httpparser.Request` contains an exported `RemoteAddr string` field.
- `req.RemoteHost()` returns the host portion for IPv4 (`"192.0.2.1:8080"` -> `"192.0.2.1"`), IPv6 (`"[2001:db8::1]:8443"` -> `"2001:db8::1"`), bare IPs (`"192.0.2.1"` -> `"192.0.2.1"`), and falls back to `RawConn.RemoteAddr()` if `RemoteAddr` is unpopulated.
- `req.RemoteIP()` returns a valid parsed `net.IP` for valid IPv4/IPv6 addresses, and `nil` for empty/malformed inputs.
- `NewRequestFromStd(r)` populates `req.RemoteAddr = r.RemoteAddr`.
- `handleConn` populates `req.RemoteAddr = conn.RemoteAddr().String()`.
- Thread-safe and race-clean under `go test -race ./...`.
- Strictly standard library packages (`net`, `strings`). Zero external third-party dependencies.

## Rationale

Without an explicit `RemoteAddr` string in `httpparser.Request`, multiplexed protocols (HTTP/2 and HTTP/3) cannot bind client identity to a physical network connection because `RawConn` cannot represent multiplexed streams. Populating `RemoteAddr` at adapter ingress creates a reliable, immutable source of truth for all downstream security perimeters.

## Constraints

- Pure Go standard library packages (`net`, `strings`). No external dependencies.
- `req.RemoteAddr` must be immutable once populated to ensure race safety during concurrent reads by middlewares and handlers.
- Backward compatibility: Existing code inspecting `req.RawConn` must not break; `RemoteHost()` and `RemoteIP()` must transparently fall back to `req.RawConn` when `req.RemoteAddr` is not set.

## Open Questions

- None identified. The standard library `http.Request.RemoteAddr` convention provides full fidelity across `net/http` and `quic-go`.
