---
id: TASK-043
type: task
title: Implement Route-Level WAF Overrides and CIDR IP Access Lists
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-15
updated: 2026-08-15

depends_on:
  - REQ-043

implements:
  - REQ-043

verified_by:
  - TC-043

decided_by:
  - ADR-038

related_to:
  - TASK-040
  - TASK-041
  - TASK-042
---

# TASK-043 - Implement Route-Level WAF Overrides and CIDR IP Access Lists

## Goal

Extend `pkg/waf`, `pkg/config`, `pkg/proxy`, `pkg/router`, and `cmd/toron` to support fast-path CIDR-based IP allow/deny access control lists and granular route-level WAF overrides in `routes.yaml`.

## Sub-tasks

1. Create `pkg/waf/ip_acl.go` implementing:
   - `IPAccessList` struct holding parsed `[]*net.IPNet` (subnets) and `[]net.IP` (exact addresses) for allowed and denied lists.
   - `NewIPAccessList(allowedCIDRs []string, deniedCIDRs []string) (*IPAccessList, error)`.
   - `CheckIP(ip net.IP) (allowed bool, reason string)` with fast evaluation ($O(1)$ loop over compiled subnets).
   - `ExtractClientIP(req *httpparser.Request) net.IP` helper (parsing `req.RemoteAddr` and optional `X-Forwarded-For`).
2. Update `pkg/waf/waf.go`:
   - Extend `WAFConfig` with `AllowedIPs []string` and `DeniedIPs []string`.
   - Embed `*IPAccessList` into `WAFEngine`.
3. Update `pkg/waf/middleware.go`:
   - Check `IPAccessList` first before executing protocol integrity checks or layer 7 regex scanning.
   - Return `403 Forbidden` with descriptive JSON body if client IP is blocked.
4. Update `pkg/config/config.go`:
   - Add `AllowedIPs` and `DeniedIPs` to `WAFConfig` in `ServerConfig` and `ProxyRouteConfig`.
5. Update `pkg/proxy/proxy.go` and `pkg/router/router.go`:
   - Add `WAF any` or `WAF waf.WAFConfig` to `ProxyOptions`.
   - In `router.RoutePrefix`, if route-specific WAF configuration is provided, construct a route-scoped WAF middleware and chain it to the route handler.
6. Update `cmd/toron/main.go`:
   - Pass route-level `pr.WAF` into `proxy.ProxyOptions` when registering static and upstream routes.
   - Update `config.yaml` and `routes.yaml` with documentation comments and example CIDR blocklists.
7. Write unit tests:
   - `pkg/waf/ip_acl_test.go` for IPv4/IPv6 CIDR evaluation, invalid CIDR handling, and client IP extraction.
   - `pkg/waf/waf_test.go` and `pkg/waf/middleware_test.go` for route-level override verification.
   - `pkg/router/router_test.go` / `pkg/router/router_waf_test.go` for end-to-end route WAF dispatching.
8. Execute `go test ./...` and dry-run syntax check (`.\toron -t`).

## Acceptance Criteria

- Client IPs matching `denied_ips` receive `403 Forbidden`.
- When `allowed_ips` is populated, client IPs not matching receive `403 Forbidden`.
- Per-route disabled rules allow matching payloads on configured routes while remaining blocked globally.
- Per-route `mode: "detection"` allows threat requests through with `X-Toron-WAF-Anomaly-Score`.
- All Go unit tests pass cleanly across all packages (`go test ./...`).
