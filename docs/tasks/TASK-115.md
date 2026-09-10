---
id: TASK-115
type: task
title: End-to-End WAF Middleware IP ACL Automated Verification Suite (TC-093)
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-10
updated: 2026-09-10

depends_on:
  - REQ-093
  - TASK-114

owns:
  - pkg/waf/middleware_test.go
  - pkg/waf/ip_acl_test.go

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
  - TASK-114

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
  - TASK-114
---

# TASK-115 - End-to-End WAF Middleware IP ACL Automated Verification Suite (TC-093)

## Description

Implement a comprehensive automated verification test suite in [`pkg/waf/middleware_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware_test.go) and [`pkg/waf/ip_acl_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl_test.go) validating test specification [`TC-093`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-093.md). Verify fail-closed denial with HTTP `403 Forbidden` and exact JSON error response when incoming client IP is unidentifiable under an active allowlist, verify fail-open pass-through when only denylists are configured, verify robust rejection of malformed/unparseable IP headers, and verify security metric recording and audit logging without panic.

## Scope & Implementation Breakdown

1. **Unit Verification of `IPAccessList.CheckIP` with Nil IP (`pkg/waf/ip_acl_test.go`)**:
   - `TestIPAccessList_CheckIP_NilIP_WithAllowlist`:
     - Construct `IPAccessList` with allowed CIDRs (`[]string{"10.0.0.0/8"}`) and allowed exact IPs (`[]string{"192.168.1.100"}`).
     - Invoke `allowed, reason := acl.CheckIP(nil)`.
     - Assert `allowed == false`.
     - Assert `reason == "client IP could not be determined and allowed IP list is enforced"` ([`REQ-093-AC-01`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md#L83-L95)).
   - `TestIPAccessList_CheckIP_NilIP_DenylistOnly`:
     - Construct `IPAccessList` with denied CIDRs (`[]string{"198.51.100.0/24"}`) and no allowed CIDRs/IPs (`allowedCIDRs == nil`).
     - Invoke `allowed, reason := acl.CheckIP(nil)`.
     - Assert `allowed == true`.
     - Assert `reason == ""` (fail-open pass-through for unidentifiable IP under denylist-only mode, as specified in [`REQ-093-AC-02`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md#L96-L101)).
   - `TestIPAccessList_CheckIP_NilIP_NoRules`:
     - Construct empty `IPAccessList` (`HasRules() == false`) and `nil` ACL.
     - Assert `allowed == true, reason == ""`.

2. **WAF Middleware Fail-Closed Denial with Allowlist (`pkg/waf/middleware_test.go`)**:
   - `TestWAFMiddleware_NilClientIP_AllowlistEnforced`:
     - Initialize `WAFEngine` with `allowed_ips: ["10.0.0.0/8"]` and mode `"enforce"`.
     - Instantiate middleware via [`NewWAFMiddleware`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware.go#L19-L91).
     - Construct request where client IP cannot be extracted:
       - `req.RemoteAddr = ""`
       - `req.RawConn = nil`
       - No `X-Forwarded-For` or `X-Real-IP` headers present.
     - Track handler execution with `nextCalled := false`.
     - Execute middleware handler.
     - Assert `nextCalled == false` (execution halted immediately).
     - Assert `res.StatusCode == 403`.
     - Assert header `Content-Type` equals `"application/json"`.
     - Assert response body matches exactly:
       ```json
       {"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}
       ```
     - Assert `metrics.DefaultRegistry.RecordWAFBlocked("ip_acl", req.Path)` was recorded.

3. **WAF Middleware Fail-Open Pass-Through with Denylist-Only (`pkg/waf/middleware_test.go`)**:
   - `TestWAFMiddleware_NilClientIP_DenylistOnly`:
     - Initialize `WAFEngine` with `denied_ips: ["198.51.100.0/24"]`, mode `"enforce"`, and no allowlist configured.
     - Construct request with empty client IP (`req.RemoteAddr = ""`, no forwarded headers).
     - Execute middleware handler.
     - Assert `nextCalled == true` (request allowed through IP ACL stage).
     - Assert response status is `200` (or benign handler response).
     - Assert IP ACL does not block requests when source IP is unknown in blacklist-only mode.

4. **Malformed & Unparseable IP Header Handling (`pkg/waf/middleware_test.go`)**:
   - `TestWAFMiddleware_MalformedClientIP_AllowlistEnforced`:
     - Initialize engine with active allowlist `allowed_ips: ["10.0.0.0/8"]`.
     - Test the following malformed input scenarios:
       1. Malformed `RemoteAddr`: `"not-an-ip:9999"`, `":::invalid"`, `"hostname-without-ip"`.
       2. Malformed `X-Forwarded-For`: `"unknown"`, `"localhost"`, `"999.999.999.999"`, `"garbage-header"`.
       3. Malformed `X-Real-IP`: `"invalid-real-ip"`, `"[bad-ipv6"`.
     - In each case, assert that `ExtractClientIP` returns `nil`.
     - Assert that the WAF middleware rejects the request with HTTP `403 Forbidden` and the standard unidentifiable IP error JSON.

5. **Security Audit Logging Verification on Nil IP Block (`pkg/waf/middleware_test.go`)**:
   - `TestWAFMiddleware_AuditLogging_NilIP_Blocked`:
     - Attach in-memory audit logger buffer via `engine.SetAuditLogger(NewAuditLoggerWithWriter(&buf))`.
     - Send request with missing IP under active allowlist.
     - Verify structured audit event is logged:
       - Contains `"event":"ip_acl_block"`
       - Contains `"action":"blocked"`
       - Contains `"category":"ip_acl"`
       - Contains `"location":"remote_addr"`
       - Contains `"client_ip":""` (empty string, confirming no panic or null dereference occurs when `clientIP` is empty).

6. **Single-Pass IP Extraction Invariant & Telemetry Parity (`pkg/waf/middleware_test.go`)**:
   - `TestWAFMiddleware_SinglePassExtraction_TelemetryParity`:
     - Verify that for both valid and nil client IPs, telemetry strings and access check outcomes are consistent and extracted in a single pass.
     - Verify that benign requests matching allowlist (`10.1.2.3`) pass through with HTTP `200` and correct telemetry logged.

7. **Race Detection and Concurrency Suite**:
   - Execute tests with `go test -v -race ./pkg/waf/...`.
   - Assert zero data races under parallel goroutine execution.

## Acceptance Criteria

- All unit test cases for `IPAccessList.CheckIP(nil)` implemented and passing in [`pkg/waf/ip_acl_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/ip_acl_test.go).
- All end-to-end middleware test cases implemented and passing in [`pkg/waf/middleware_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/waf/middleware_test.go).
- Strict verification of HTTP `403 Forbidden` status and exact JSON payload `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}` on unidentifiable IP under allowlist.
- Strict verification of fail-open pass-through when only denylist is active and client IP is unidentifiable.
- Strict verification of malformed header rejection under allowlist.
- Audit logging and metrics emission verified with zero panics on nil IP.
- 100% race-clean under `go test -race ./pkg/waf/...`.
- Pure Go standard library (zero external test frameworks or third-party packages).

## Rationale

Without dedicated end-to-end middleware automated tests, regressions in request handling or IP extraction ordering could re-introduce silent fail-open vulnerabilities (`SEC-32`). An explicit automated verification suite guaranteeing fail-closed behavior on missing and corrupt IP inputs provides continuous regression prevention in CI.

## Constraints

- Pure Go standard library packages (`testing`, `net`, `strings`, `bytes`, `net/url`).
- Deterministic assertions; no arbitrary sleep delays.

## Open Questions

- None. Test scenarios align directly with [`TC-093`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-093.md) and [`REQ-093`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-093.md).
