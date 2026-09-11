---
id: TASK-126
type: task
title: Implement RFC 7234 Shared Cache Session Boundary Isolation Test Vectors in Differential Fuzzer
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-103

owns:
  - benchmarks/fuzzer/diff_fuzzer.go
  - benchmarks/fuzzer/diff_fuzzer_test.go

references:
  - REQ-103
  - TST-03
  - AER-001
  - ADR-103
  - TC-103

derived_from:
  - REQ-103
  - TST-03

implements:
  - REQ-103

verified_by:
  - TC-103

decided_by:
  - ADR-103

related_to:
  - REQ-103
  - ADR-103
  - TC-103
---

# TASK-126 - Implement RFC 7234 Shared Cache Session Boundary Isolation Test Vectors in Differential Fuzzer

## 1. Description

Implement multi-stage sequential request execution and RFC 7234 shared cache session boundary isolation test vectors (`CACHE-001..003`) in `benchmarks/fuzzer/diff_fuzzer.go` in accordance with [`REQ-103`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-103.md), [`ADR-103`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-103.md), and [`TC-103`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-103.md).

## 2. Work Breakdown

### Task 126.1: Multi-Stage Execution Engine & Header Parser
- Extend `TestCase` struct in `diff_fuzzer.go`:
  - `SequentialPayloads []string`: List of raw HTTP request payloads to execute sequentially over TCP.
  - `ForbiddenResponseHeaders []string`: Header names that must not appear in the final probe response.
  - `RequiredResponseHeaders map[string]string`: Headers that must match in the final probe response.
- Enhance `executeRawTest` to parse response headers into an `http.Header` map and evaluate forbidden/required header assertions.

### Task 126.2: Define Cache Test Cases (`CACHE-001..003`)
- `CACHE-001` (Web Cache Deception): Sequential requests asserting unauthenticated client receives `X-Cache: MISS` for private cached resources.
- `CACHE-002` (`Set-Cookie` Stripping): Sequential requests asserting cached responses (`X-Cache: HIT`) have `Set-Cookie` and `Set-Cookie2` stripped.
- `CACHE-003` (`Authorization` Refusal): Sequential requests asserting authenticated private responses are refused by shared cache.

### Task 126.3: Automated Unit Test Suite
- Add unit tests in `diff_fuzzer_test.go` mocking a cache server and asserting `CACHE-001`, `CACHE-002`, and `CACHE-003` pass/fail conditions.
