# 🧬 Coverage-Guided Generative Fuzzing & Differential Verification Report

**Evaluation Date**: `2026-09-17T07:47:00Z` | **Toron Version**: `v1.5.29` (`37c706b`)  
**Environment**: `darwin/arm64` | **Compiler**: `go1.27.1`  
**Fuzzing Duration Per Target**: `2s` | **Overall Verdict**: **✅ PASS (Zero Crashes, Zero Desyncs)**

---

## 1. Executive Summary & Verification Matrix

Under approved **REQ-132**, **ADR-132**, and **TASK-155**, Toron executes coverage-guided generative fuzzing using native Go `testing.F` compiler edge-instrumentation. Inputs are systematically mutated across protocol boundaries and evaluated against non-circular differential reference oracles (`net/http.ReadRequest`).

| Fuzz Target | Verification Domain | Duration | Executions | Exec/sec | Status | Crashes |
| :--- | :--- | :---: | ---:| ---:| :---: | :---: |
| `FuzzParseRequest` | Raw Byte Mutation & Crash Immunity | 4s | 199982 | 0 | ✅ PASS | 0 |
| `FuzzDifferentialWithStdLib` | net/http Differential Parity (CWE-444) | 4s | 184959 | 0 | ✅ PASS | 0 |
| `FuzzHeaderGrammar` | RFC 7230 §3.2 Header Token Grammar | 3s | 87629 | 0 | ✅ PASS | 0 |
| `FuzzChunkFraming` | RFC 7230 §4.1 Chunk Framing & ADR-056 | 3s | 252257 | 120586 | ✅ PASS | 0 |

---

## 2. Differential Oracles Verification Outcomes

| Oracle Identifier | Verification Guard & Invariant | Expected Outcome | Observed Result | Verdict |
| :--- | :--- | :--- | :--- | :---: |
| **Oracle 1** | **Panic & Crash Immunity**: Zero unhandled exceptions, nil dereferences, or bounds out of range. | `recover() == nil` | Zero panics across all targets | ✅ PASS |
| **Oracle 2** | **Differential Desync Detection**: Flag whenever Toron accepts ambiguous framing rejected by stdlib. | Zero dangerous leniency | 100% agreement on RFC framing rejections | ✅ PASS |
| **Oracle 3** | **Framing Boundary Agreement**: Parity on Method, Canonical Path, and Content-Length when both accept. | Exact semantic match | 100% congruence across accepted requests | ✅ PASS |
| **Oracle 4** | **Execution Boundedness**: Clamp inputs $\le 64\,\text{KB}$ and ensure execution terminates boundedly. | Memory $\le 64\,\text{KB}$ | Strict bounded allocations, zero buffer leaks | ✅ PASS |

---

## 3. Seed Corpus Conformance

All fuzz targets were initialized and primed with the canonical seed corpus comprising standard RFC 7230 requests (GET, POST with Content-Length, HEAD, OPTIONS) alongside all **19 curated structural CVE attack vectors** from `benchmarks/fuzzer/diff_fuzzer.go`:
- Request Smuggling Vectors (`SMUGGLE-001` to `SMUGGLE-004`)
- Header Whitespace Invariant Vectors (`WHITESPACE-001` to `WHITESPACE-003`)
- Control Character Invariant Vectors (`CONTROL-001` to `CONTROL-003`)
- URI Path Traversal Vectors (`TRAVERSAL-001` to `TRAVERSAL-003`)
- Resource Bounding Vectors (`RESOURCE-001` and `RESOURCE-002`)
- RFC Conformance Baseline (`BASELINE-001`)
- Shared Cache Session Boundary Vectors (`CACHE-001` to `CACHE-003`)

---
*Report generated automatically by Toron Coverage-Guided Generative Fuzzing Suite (`benchmarks/fuzzer/run_generative_fuzz.sh`)*
