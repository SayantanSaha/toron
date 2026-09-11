---
id: TASK-131
type: task
title: Persistent bufio.Reader Preservation for HTTP/1.1 Pipelining and Connection Keep-Alive
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-108

derived_from:
  - REQ-108
  - SRC-02

implements:
  - REQ-108

verified_by:
  - TC-108

decided_by:
  - ADR-108

related_to:
  - REQ-108
  - ADR-108
  - TC-108
  - CR-104
  - SR-108
---

# TASK-131 - Persistent bufio.Reader Preservation for HTTP/1.1 Pipelining and Connection Keep-Alive

## 1. Description

Decompose and implement the technical changes necessary to eliminate `bufio.Reader` buffer eviction on persistent HTTP/1.1 connections, preserving unconsumed pipelined bytes across consecutive request cycles (`SRC-02`, `REQ-108`).

## 2. Subtask Breakdown

### TASK-131.1: Parser-Level `*bufio.Reader` Pass-Through & Reuse
- **Component**: `pkg/httpparser/parser.go`
- **Scope**:
  - In `ParseRequest(r io.Reader, opts ParserOptions)`, check if `r` can be type-asserted to `*bufio.Reader` via `if br, ok := r.(*bufio.Reader); ok { bufr = br } else { bufr = bufio.NewReader(r) }`.
  - Ensure existing callers passing `bytes.Buffer`, `strings.Reader`, or generic `io.Reader` maintain backward compatibility without interface changes.

### TASK-131.2: Server-Level Persistent `*bufio.Reader` Lifecycle
- **Component**: `pkg/server/server.go`
- **Scope**:
  - In `handleConn(ctx context.Context, conn net.Conn)`, initialize a single `br := bufio.NewReader(conn)` once prior to entering the persistent request processing loop.
  - In every iteration of the loop, pass `br` into `httpparser.ParseRequest(br, opts)`.
  - Maintain the single `*bufio.Reader` instance across the entire lifetime of the persistent TCP connection.

### TASK-131.3: Buffered Inactivity & Idle Deadline Management
- **Component**: `pkg/server/server.go`
- **Scope**:
  - Prior to parsing on iteration $N > 1$:
    - Check if `br.Buffered() > 0`.
    - If `br.Buffered() > 0`, pipelined bytes are already present in user-space buffer; apply `ReadTimeout` (do not stall on `IdleTimeout`).
    - If `br.Buffered() == 0`, connection is idle waiting for network ingress; apply `IdleTimeout` (`s.config.IdleTimeout`).

### TASK-131.4: Protocol Upgrade Handover & Residual Buffer Preservation
- **Component**: `pkg/server/server.go`
- **Scope**:
  - When handling protocol upgrades (`res.UpgradedConn != nil` or `res.StatusCode == http.StatusSwitchingProtocols`):
    - Check if `br.Buffered() > 0`.
    - If unconsumed bytes remain in `br`, extract them via `unconsumed := make([]byte, br.Buffered()); br.Read(unconsumed)`.
    - Wrap `conn` using `prefixConn{Conn: conn, prefix: unconsumed}` before delegating to `relayUpgradedStreams`.

### TASK-131.5: Error Response Serialization & Connection Teardown
- **Component**: `pkg/server/server.go`
- **Scope**:
  - On parser error (`httpparser.ParseRequest` returning non-nil error):
    - Ensure error response serialization (`res.Serialize(conn)`) sets `Connection: close`.
    - Terminate `handleConn` immediately, executing deferred socket closure and purging reader buffer.

### TASK-131.6: Unit, Integration, and Regression Test Suite
- **Components**: `pkg/httpparser/parser_test.go`, `pkg/server/server_test.go`
- **Scope**:
  - Add tests for:
    - Multiple pipelined GET requests sent in a single `conn.Write()` call processed in strict FIFO order (`TC-108-01`).
    - Pipelined POST request with body followed immediately by GET request (`TC-108-02`).
    - Pipelined requests with `Connection: close` terminating socket after the designated request (`TC-108-03`).
    - Pipelined protocol upgrade preserving residual frames (`TC-108-04`).
    - Direct `*bufio.Reader` vs standard reader zero-allocation verification in parser (`TC-108-05`).

## 3. Acceptance Criteria

- All unit and integration tests execute with 100% pass rate under `go test -v -race -count=1 ./...`.
- Zero external third-party dependencies.
- HTTP/1.1 pipelining functions correctly across persistent keep-alive connections without byte loss or buffer eviction.
