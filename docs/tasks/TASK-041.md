---
id: TASK-041
type: task
title: Implement Core Web Application Firewall Engine and OWASP Injection Protection Middleware
status: in_progress
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-15
updated: 2026-08-15

depends_on:
  - REQ-041

implements:
  - REQ-041

verified_by:
  - TC-041

decided_by:
  - ADR-036

related_to:
  - TASK-003
  - TASK-004
  - TASK-040
---

# TASK-041 - Implement Core Web Application Firewall Engine and OWASP Injection Protection Middleware

## Goal

Design and implement `pkg/waf` providing a high-performance WAF middleware engine with OWASP injection protection (SQLi, XSS, Path Traversal, RCE), and integrate into `pkg/config/config.go`, `pkg/router/router.go`, `cmd/toron/main.go`, `config.yaml`, and `routes.yaml`.

## Sub-tasks

1. Create `pkg/waf/waf.go` implementing:
   - `RuleCategory` (SQLi, XSS, Traversal, RCE).
   - `WAFRule` struct (ID, Category, Pattern, Score, Description, TargetLocations).
   - `WAFConfig` struct (`Enabled`, `Mode`, `AnomalyThreshold`, `MaxInspectBodySize`, `CustomRules`).
   - `WAFEngine` struct with pre-compiled regex matchers and zero-alloc inspection functions.
   - `NewEngine(cfg WAFConfig) (*WAFEngine, error)`.
   - `Inspect(req *http.Request) (blocked bool, score int, matchedRules []WAFRule)`.
2. Create `pkg/waf/rules.go` with default OWASP Top 10 rule sets for SQLi, XSS, Path Traversal, and Command Injection.
3. Create `pkg/waf/middleware.go` providing `NewWAFMiddleware(engine *WAFEngine) router.MiddlewareFunc`.
4. Update `pkg/config/config.go` with `WAFConfig` struct embedded in `ServerConfig` and `ProxyRouteConfig`.
5. Update `cmd/toron/main.go`, `config.yaml`, and `routes.yaml` to initialize and bind WAF middleware.
6. Write comprehensive unit tests in `pkg/waf/waf_test.go` and `pkg/waf/middleware_test.go`.
7. Execute full test suite `go test ./...` and configuration dry-run syntax check (`.\toron -t`).

## Acceptance Criteria

- WAF middleware blocks malicious requests with `403 Forbidden` in `enforce` mode and attaches anomaly headers in `detection` mode.
- Unit tests verify SQLi, XSS, Path Traversal, and RCE vector detection.
- All Go unit tests pass cleanly across all packages (`go test ./...`).
