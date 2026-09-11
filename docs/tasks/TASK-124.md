---
id: TASK-124
type: task
title: Physical TCP Socket Termination Verification and Invariant Enforcement in Differential Fuzzer
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-101

owns:
  - benchmarks/fuzzer/diff_fuzzer.go
  - benchmarks/fuzzer/diff_fuzzer_test.go

references:
  - REQ-101
  - TST-01
  - AER-001
  - ADR-101
  - TC-101

derived_from:
  - REQ-101
  - TST-01

implements:
  - REQ-101

verified_by:
  - TC-101

decided_by:
  - ADR-101

related_to:
  - REQ-101
  - ADR-101
  - TC-101
  - SRC-01
---

# TASK-124 - Physical TCP Socket Termination Verification and Invariant Enforcement in Differential Fuzzer

## 1. Description

Implement active physical TCP socket termination verification and invariant assertion in `benchmarks/fuzzer/diff_fuzzer.go` in accordance with [`REQ-101`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-101.md), [`ADR-101`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-101.md), and [`TC-101`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-101.md).

## 2. Work Breakdown

### Task 124.1: Socket Verification Helper (`verifySocketClosed`)
- Implement `verifySocketClosed(conn net.Conn, reader *bufio.Reader) bool` in `benchmarks/fuzzer/diff_fuzzer.go`.
- Drain remaining response bytes with a bounded read deadline (250ms) to detect EOF from server teardown (`FIN`).
- Check for `io.EOF`, `net.ErrClosed`, `syscall.ECONNRESET`, `syscall.EPIPE`, and textual error signatures ("connection reset", "broken pipe").
- If reading times out without EOF, attempt a subsequent probe write and read to verify whether the socket is active on keep-alive or closed.

### Task 124.2: Invariant Assertion & Test Result Evaluation
- In `executeRawTest`, evaluate `result.ConnectionClose = verifySocketClosed(conn, reader)`.
- If `tc.ExpectClose == true` and `!result.ConnectionClose`:
  - Fail the test (`result.Passed = false`).
  - Set `result.FailureReason` detailing that physical connection termination was expected but connection remained open.

### Task 124.3: Latency Measurement Isolation
- Ensure `result.LatencyUs` is recorded strictly before calling `verifySocketClosed`, preventing probe timeouts from distorting fail-fast rejection latency figures.

### Task 124.4: Console and Report Upgrades
- Update CLI execution summary logging to include `[conn: closed]` or `[conn: open]`.
- Update Markdown report generation (`benchmarks/results/differential_fuzz_report.md`) to include a `Conn Closed` column.
- Verify `benchmarks/results/differential_fuzz_report.json` outputs accurate `"connection_closed"` booleans.

### Task 124.5: Unit Test Suite (`diff_fuzzer_test.go`)
- Create `benchmarks/fuzzer/diff_fuzzer_test.go` with automated unit tests for:
  - Detection of closed connections when server writes response and calls `Close()`.
  - Detection of keep-alive connections when server writes response and remains idle.
  - Verification of `ExpectClose` pass/fail evaluation in `executeRawTest`.
