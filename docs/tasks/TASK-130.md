---
id: TASK-130
type: task
title: Implementation of Connection Close on WAF and Router Security Rejections
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-107

derived_from:
  - REQ-107
  - SRC-01

implements:
  - REQ-107

verified_by:
  - TC-107

decided_by:
  - ADR-107

related_to:
  - REQ-107
  - ADR-107
  - TC-107
  - CR-103
  - SR-107
---

# TASK-130 - Implementation of Connection Close on WAF and Router Security Rejections

## 1. Description

Decompose and implement the technical changes necessary to enforce fail-fast transport socket teardown whenever a request is blocked by WAF security rules, RFC protocol integrity checks, or filesystem path traversal guards (`SRC-01`, `REQ-107`).

## 2. Subtask Breakdown

### TASK-130.1: WAF Middleware Rejection Header Enforcement
- **Component**: `pkg/waf/middleware.go`
- **Scope**:
  - In fast-path IP ACL rejection (`acl.CheckIP`), add `res.Header.Set("Connection", "close")`.
  - In protocol integrity violation rejection (`ValidateProtocolIntegrity`), add `res.Header.Set("Connection", "close")`.
  - In Layer 7 OWASP threat inspection block (`engine.InspectToron`), add `res.Header.Set("Connection", "close")`.

### TASK-130.2: Router Path Traversal Rejection Header Enforcement
- **Component**: `pkg/router/router.go`
- **Scope**:
  - In `createStaticHandler`, on relative path traversal rejection (`403 Forbidden: Path Traversal Disallowed`), add `res.Header.Set("Connection", "close")`.
  - On symlink escape path traversal rejection (`403 Forbidden: Symlink Path Traversal Disallowed`), add `res.Header.Set("Connection", "close")`.

### TASK-130.3: Server Connection Loop Egress Header Inspection & Teardown
- **Component**: `pkg/server/server.go`
- **Scope**:
  - In `handleConn`, inspect `strings.ToLower(res.Header.Get("Connection")) == "close"` in addition to client `connHeader == "close"` after routing and `res.Serialize(conn)`.
  - If either is `"close"`, exit `handleConn` (`return nil`), triggering the deferred `conn.Close()` on the reactor socket.

### TASK-130.4: Differential Security Fuzzer ExpectClose Oracle Alignment
- **Component**: `benchmarks/fuzzer/diff_fuzzer.go`
- **Scope**:
  - Update `ExpectClose: true` for `CONTROL-001`, `CONTROL-002`, `CONTROL-003`, `RESOURCE-002`, `TRAVERSAL-001`, `TRAVERSAL-002`, `TRAVERSAL-003`.

### TASK-130.5: Automated Test Verification
- **Components**: `pkg/waf/middleware_test.go`, `pkg/server/server_test.go`, `benchmarks/fuzzer/diff_fuzzer_test.go`
- **Scope**:
  - Assert that responses to blocked requests include `Connection: close`.
  - Assert that client sockets are physically closed by the server following WAF and router rejections.
  - Verify that benign requests retain keep-alive functionality.

## 3. Acceptance Criteria

- All unit and benchmark tests execute cleanly under `go test -v -race -count=1 ./...`.
- Differential fuzzer passes all security vectors with physical socket termination confirmed.
