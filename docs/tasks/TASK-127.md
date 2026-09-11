---
id: TASK-127
type: task
title: Differential Fuzzer Report Formatting Bug Fix for Baseline Request
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-11
updated: 2026-09-11

depends_on:
  - REQ-104

owns:
  - benchmarks/fuzzer/diff_fuzzer.go
  - benchmarks/fuzzer/diff_fuzzer_test.go

references:
  - REQ-104
  - TST-04
  - AER-001
  - ADR-104
  - TC-104

derived_from:
  - REQ-104
  - TST-04

implements:
  - REQ-104

verified_by:
  - TC-104

decided_by:
  - ADR-104

related_to:
  - REQ-104
  - ADR-104
  - TC-104
---

# TASK-127 - Differential Fuzzer Report Formatting Bug Fix for Baseline Request

## 1. Description

Resolve the hardcoded status string defect in `benchmarks/fuzzer/diff_fuzzer.go` where `differential_fuzz_report.md` rendered expected status codes with a static fallback (`400/501`), causing benign baseline request `BASELINE-001` to be displayed incorrectly. Implement dynamic expected status code formatting using Go standard library `http.StatusText(code)` for canonical RFC status rendering (e.g. `200 OK`) across Markdown and JSON report outputs.

## 2. Work Breakdown

### Task 127.1: Dynamic Status Code Formatting Engine
- Implement `formatExpectedStatus(expected []int) string` helper function in `diff_fuzzer.go`:
  - Single status code (e.g. `[200]`): render with canonical text phrase (e.g. `200 OK`).
  - Multiple status codes (e.g. `[400, 501]`, `[400, 403]`): render as slash-delimited list (e.g. `400/501`, `400/403`).
- Extend `TestResult` struct to store `ExpectedStatus []int` and `ExpectedStatusStr string`.
- Update `executeRawTest` to populate `ExpectedStatus` and `ExpectedStatusStr` in `TestResult`.

### Task 127.2: Markdown & JSON Report Integration
- Refactor markdown table generation in `diff_fuzzer.go` to use `r.ExpectedStatusStr` directly, eliminating hardcoded string branches.
- Ensure `BASELINE-001` renders explicitly as `200 OK` in `differential_fuzz_report.md`.

### Task 127.3: Automated Unit Verification
- Add unit tests in `diff_fuzzer_test.go` asserting canonical formatting output across single-code, multi-code, and baseline test cases.
