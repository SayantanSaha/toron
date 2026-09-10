---
id: TASK-114
type: task
title: WAF Middleware Single-Pass IP Extraction and Fail-Closed Enforcement
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-10
updated: 2026-09-10

depends_on:
  - REQ-093
  - TASK-112

owns:
  - pkg/waf/middleware.go

references:
  - REQ-093
  - SEC-32
  - SR-091
  - ADR-088
  - TC-093
  - REQ-001
  - REQ-019
  - REQ-041
  - REQ-092
  - TASK-112

derived_from:
  - REQ-093
  - SEC-32
  - SR-091

implements:
  - REQ-093

verified_by:
  - TC-093

decided_by:
  - ADR-088

related_to:
  - SEC-32
  - SR-091
  - REQ-092
  - TASK-115
---

# TASK-114 - WAF Middleware Single-Pass IP Extraction and Fail-Closed Enforcement

## Description

Refactor the WAF middleware in [`pkg/waf/middleware.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware.go) to perform single-pass client IP extraction and enforce fail-closed access control when incoming client IP cannot be determined under an active IP allowlist. This resolves vulnerability [`SEC-32`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L439-L447) ([CWE-284](https://cwe.mitre.org/data/definitions/284.html), [CWE-1188](https://cwe.mitre.org/data/definitions/1188.html), [CWE-693](https://cwe.mitre.org/data/definitions/693.html)) and audit finding [`SR-091 Finding 2`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-091.md#L102-L127), and eliminates the redundant double-invocation of [`ExtractClientIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L181-L207) on the request hot path.

## Scope & Implementation Breakdown

1. **Eliminate Redundant Double-Invocation of `ExtractClientIP`**:
   - In [`NewWAFMiddleware`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware.go#L19-L91), replace duplicate sequential calls to `ExtractClientIP(req, tp)` with single-pass extraction:
     ```go
     clientNetIP := ExtractClientIP(req, tp)
     clientIP := ""
     if clientNetIP != nil {
         clientIP = clientNetIP.String()
     }
     ```
   - Store extracted `clientNetIP` in a local variable and reuse it for:
     - Populating string telemetry `clientIP` for structured security audit logging.
     - Fast-path CIDR IP Access Control check via `acl.CheckIP(clientNetIP)`.
     - Subsequent inspection stages (Protocol Integrity, Custom Rules, OWASP Injection Rules) for telemetry and audit events.

2. **Unconditional IP ACL Evaluation**:
   - Ensure `acl.CheckIP(clientNetIP)` is invoked unconditionally whenever `acl := engine.IPAccessList(); acl != nil && acl.HasRules()` evaluates to `true`.
   - Remove any guard condition that skips IP access evaluation when `clientNetIP == nil`.
   - When incoming client IP cannot be determined (`clientNetIP == nil`), `acl.CheckIP(nil)` must evaluate whether an allowlist is enforced:
     - If an allowlist (`allowed_ips` or `allowedSubnets`) is configured, `acl.CheckIP(nil)` returns `allowed: false, reason: "client IP could not be determined and allowed IP list is enforced"`.
     - If only a denylist (`denied_ips` or `deniedSubnets`) is configured or no allowlist is active, `acl.CheckIP(nil)` returns `allowed: true, reason: ""` (preserving fail-open behavior for unknown IPs under denylist-only configurations as specified in [`REQ-093-AC-02`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md#L96-L101)).

3. **Enforce Fail-Closed 403 Forbidden Response**:
   - When `acl.CheckIP(clientNetIP)` returns `allowed == false`:
     - Halt downstream middleware execution immediately without invoking `next(req, res)`.
     - Reset response body buffer and set HTTP status code to `403`.
     - Set response header `Content-Type: application/json`.
     - Write structured JSON error payload matching existing WAF error conventions:
       ```json
       {"error":"Forbidden","message":"<reason>"}
       ```
     - For unidentifiable client IP under an active allowlist, the resulting response body must be:
       ```json
       {"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}
       ```

4. **Security Telemetry and Audit Event Emission**:
   - On denial (`!allowed`), increment Prometheus blocked counter:
     ```go
     metrics.DefaultRegistry.RecordWAFBlocked("ip_acl", req.Path)
     ```
   - If an audit logger is registered (`logger := engine.AuditLogger(); logger != nil`), emit a structured [`SecurityEvent`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/engine.go#L37-L49):
     - `Event: "ip_acl_block"`
     - `ClientIP: clientIP` (empty string `""` when `clientNetIP == nil`)
     - `Method: req.Method`
     - `Path: req.Path`
     - `Category: "ip_acl"`
     - `AnomalyScore: 0`
     - `Action: "blocked"`
     - `Location: "remote_addr"`

5. **Concurrency & Thread Safety Invariant**:
   - Guarantee that single-pass extraction and ACL checks are completely thread-safe across concurrent goroutines without shared mutable state (`go test -race ./...`).

## Acceptance Criteria

- [`ExtractClientIP`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl.go#L181-L207) is executed at most once per request in [`pkg/waf/middleware.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware.go).
- `acl.CheckIP(clientNetIP)` is evaluated unconditionally whenever `acl != nil && acl.HasRules()`.
- Requests with unidentifiable client IP (`clientNetIP == nil`) are denied with HTTP `403 Forbidden` and JSON error body `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}` when an allowlist is active ([`REQ-093-AC-01`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md#L83-L95)).
- Requests with unidentifiable client IP pass through IP ACL evaluation when only denylists are active ([`REQ-093-AC-02`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md#L96-L101)).
- `metrics.DefaultRegistry.RecordWAFBlocked("ip_acl", req.Path)` is incremented on every denial.
- Structured `SecurityEvent` with `Event: "ip_acl_block"` and empty `ClientIP` is emitted to the audit logger on denial when client IP is missing.
- Zero third-party dependencies; pure Go standard library packages.
- Zero data races under `go test -race ./pkg/waf/...`.

## Rationale

Previously, the WAF middleware checked `if ip := ExtractClientIP(req); ip != nil` before calling `acl.CheckIP(ip)`. When an attacker routed requests through intermediate nodes that stripped IP headers or submitted synthetic payloads without connection IP, `ip` was `nil`, bypassing the allowlist check entirely (fail-open). Moreover, `ExtractClientIP` was called twice in sequence on the hot path. Caching the extracted `net.IP` and passing it unconditionally into `acl.CheckIP` ensures that allowlist policies are strictly fail-closed while optimizing CPU cache and latency overhead.

## Constraints

- Pure Go standard library packages (`bytes`, `strings`, `time`, `net`, `toron/pkg/httpparser`, `toron/pkg/metrics`).
- Retain exact error payload format `{"error":"Forbidden","message":"..."}`.
- Do not introduce heap allocations on benign request hot paths beyond existing IP parsing.

## Open Questions

- None. Implementation behavior and error payload formatting are fully defined by [`REQ-093`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md) and [`ADR-088`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-088.md).
