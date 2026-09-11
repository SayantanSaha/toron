---
id: TASK-125
type: task
title: Path Traversal Test Oracle Hardening and Deterministic Canary Enforcement
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-102

owns:
  - benchmarks/fuzzer/diff_fuzzer.go
  - benchmarks/fuzzer/diff_fuzzer_test.go
  - canary_traversal.txt

references:
  - REQ-102
  - TST-02
  - AER-001
  - ADR-102
  - TC-102

derived_from:
  - REQ-102
  - TST-02

implements:
  - REQ-102

verified_by:
  - TC-102

decided_by:
  - ADR-102

related_to:
  - REQ-102
  - ADR-102
  - TC-102
---

# TASK-125 - Path Traversal Test Oracle Hardening and Deterministic Canary Enforcement

## 1. Description

Harden the differential fuzzer test suite (`benchmarks/fuzzer/diff_fuzzer.go`) by removing false-positive HTTP 404 pass conditions from traversal test vectors `TRAVERSAL-001..003` and deploying a canary file outside the static route root in accordance with [`REQ-102`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-102.md), [`ADR-102`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-102.md), and [`TC-102`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-102.md).

## 2. Work Breakdown

### Task 125.1: Canary File Provisioning
- Create `canary_traversal.txt` in repository root outside `./public` with static token `TORON_CANARY_TRAVERSAL_PROTECTION_VERIFIED`.

### Task 125.2: Test Oracle Hardening in `diff_fuzzer.go`
- Update `TRAVERSAL-001`, `TRAVERSAL-002`, and `TRAVERSAL-003`:
  - Change `ExpectedStatus` from `[]int{400, 403, 404}` to strictly `[]int{400, 403}`.
  - Update `RawPayload` to target `canary_traversal.txt` via relative dot-dot segments (`/internal/dashboard/../../canary_traversal.txt`, `%2E%2E`, and `%252e%252e`).

### Task 125.3: Automated Unit Test Oracle Verification
- Add unit tests in `benchmarks/fuzzer/diff_fuzzer_test.go`:
  - `TestExecuteRawTest_Traversal_Rejects404AsFailure`: Assert that a mock server returning HTTP 404 for traversal is marked as FAILED.
  - `TestExecuteRawTest_Traversal_Accepts403And400`: Assert that a mock server returning HTTP 403 or 400 is marked as PASSED.
  - `TestExecuteRawTest_Traversal_Rejects200CanaryLeak`: Assert that a mock server returning HTTP 200 (canary leak) is marked as FAILED.

### Task 125.4: Documentation and Benchmark Sync
- Update `benchmarks/README.md` and `ReviewTaskSummary.md`.
