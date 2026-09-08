---
id: TASK-077
type: task
title: Implement Concurrent Connection Dispatch in TCP Reactor
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-08
depends_on: []
derived_from:
  - REQ-077
implements:
  - REQ-077
verified_by:
  - TC-077
decided_by:
  - ADR-072
related_to: []
---

# TASK-077 - Implement Concurrent Connection Dispatch in TCP Reactor

## Description

Refactor connection handling in `pkg/reactor/reactor.go` and `pkg/server/server.go` so that persistent HTTP/1.1 keep-alive connections do not monopolize worker pool slots, preventing worker exhaustion and listener starvation.

## Scope & Implementation Breakdown

1. **Reactor Connection Dispatch (`pkg/reactor/reactor.go`)**:
   - Update worker loop to dispatch connection processing asynchronously or spawn connection handlers per accepted socket, ensuring `tasks` channel never deadlocks `ln.Accept()`.
   - Maintain graceful shutdown tracking with `sync.WaitGroup`.
2. **Server Keep-Alive Handling (`pkg/server/server.go`)**:
   - Ensure idle connections waiting for subsequent requests do not prevent new connections from being accepted.
3. **Concurrency Testing (`pkg/reactor/reactor_test.go`)**:
   - Add test simulating 200 concurrent idle keep-alive connections and verify that connection 201 is accepted immediately without delay.

## Acceptance Criteria

- Idle keep-alive connections cannot exhaust the server's connection capacity.
- The reactor accept loop never blocks under idle load.
- Graceful shutdown waits for all active connections.

## Rationale

Eliminates connection starvation Denial of Service attacks against the edge gateway.

## Constraints

- Pure Go standard library.
- Zero data races under `go test -race`.
