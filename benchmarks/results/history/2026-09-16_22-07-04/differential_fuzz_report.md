# 🛡️ Differential Protocol Security & Invariant Verification Report

**Target Under Test**: `127.0.0.1:8080`  
**Execution Timestamp**: `2026-09-16T17:02:13Z`  
**Evaluation Mode**: Repeated Statistical Trials ($K=1000$, $W=50$ warm-up discarded)  
**Overall Invariant Pass Rate**: **100.00%** (19/19 tests)  

### Distributional Latency Breakdown (Fail-Fast Defense vs. Comprehensive)

- **Fail-Fast Defense Latency (N=18 Adversarial Rejection Vectors)**:  
  - **Mean**: `138.01 µs`  
  - **Median (p50)**: `72.40 µs`  
  - **Tail Latency (p90)**: `505.04 µs`  
  - **Max Rejection**: `538.65 µs`  

- **Cross-Vector Comprehensive Latency (N=19 Vectors, incl. Baseline 200 OK)**:  
  - **Mean**: `159.61 µs`  
  - **Median (p50)**: `72.40 µs`  
  - **90th Percentile (p90)**: `538.65 µs`  
  - **Tail Latency (p99)**: `548.46 µs`  
  - **Max Latency**: `548.46 µs`  

## 1. Category Summary Matrix

| Security Category | Total Tests | Passed | Failed | Pass Rate |
| :--- | :---: | :---: | :---: | :---: |
| Request Smuggling (Obfuscation) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Chunked) | 1 | 1 | 0 | 100.0% |
| Header Syntax Invariants | 3 | 3 | 0 | 100.0% |
| Control Character Guards | 3 | 3 | 0 | 100.0% |
| Resource Bounding | 2 | 2 | 0 | 100.0% |
| RFC Conformance Baseline | 1 | 1 | 0 | 100.0% |
| Path Canonicalization | 3 | 3 | 0 | 100.0% |
| Cache Session Boundary | 3 | 3 | 0 | 100.0% |
| Request Smuggling (CL.TE) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (CL.CL) | 1 | 1 | 0 | 100.0% |

## 2. Detailed Invariant Test Results

| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Mean ± StdDev | Median (p50) | p90 | p99 | 95% CI | Conn Closed | Result |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `SMUGGLE-001` | Conflicting Content-Length and Transfer-Encoding | CWE-444 | `400/501` | `400` | `60.49 ± 199.62 µs` | `48.50 µs` | `70.75 µs` | `147.83 µs` | `[48.12, 72.87]` | `Closed` | ✅ PASS |
| `SMUGGLE-002` | Multiple Divergent Content-Length Headers | CWE-444 | `400/501` | `400` | `49.86 ± 16.83 µs` | `47.98 µs` | `64.83 µs` | `91.21 µs` | `[48.82, 50.90]` | `Closed` | ✅ PASS |
| `SMUGGLE-003` | Transfer-Encoding with Obfuscated Tab Character | CWE-444 | `400/501` | `400` | `60.31 ± 203.34 µs` | `49.31 µs` | `71.13 µs` | `150.33 µs` | `[47.71, 72.91]` | `Closed` | ✅ PASS |
| `SMUGGLE-004` | Invalid Chunk Hex Size Extension | CWE-444 | `400/501` | `501` | `49.96 ± 19.23 µs` | `48.42 µs` | `63.46 µs` | `93.63 µs` | `[48.77, 51.15]` | `Closed` | ✅ PASS |
| `WHITESPACE-001` | Space Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `49.23 ± 20.37 µs` | `47.35 µs` | `62.83 µs` | `105.71 µs` | `[47.97, 50.50]` | `Closed` | ✅ PASS |
| `WHITESPACE-002` | Tab Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `48.74 ± 20.27 µs` | `46.67 µs` | `63.83 µs` | `104.04 µs` | `[47.49, 50.00]` | `Closed` | ✅ PASS |
| `WHITESPACE-003` | Obsolete Line Folding (obs-fold) | CWE-436 | `200/400` | `400` | `54.60 ± 90.79 µs` | `49.75 µs` | `64.17 µs` | `133.96 µs` | `[48.97, 60.23]` | `Closed` | ✅ PASS |
| `CONTROL-001` | Null Byte (0x00) in Request URI | CWE-117 | `400 Bad Request` | `400` | `46.89 ± 14.97 µs` | `45.77 µs` | `59.88 µs` | `84.13 µs` | `[45.96, 47.81]` | `Closed` | ✅ PASS |
| `CONTROL-002` | Bell Character (0x07) in Header Value | CWE-117 | `400 Bad Request` | `400` | `79.29 ± 44.22 µs` | `72.40 µs` | `95.25 µs` | `256.25 µs` | `[76.55, 82.03]` | `Closed` | ✅ PASS |
| `CONTROL-003` | Escape Sequence (0x1B) in Query String | CWE-117 | `400 Bad Request` | `400` | `52.52 ± 50.52 µs` | `49.77 µs` | `64.58 µs` | `102.83 µs` | `[49.39, 55.65]` | `Closed` | ✅ PASS |
| `TRAVERSAL-001` | Raw Dot-Dot Path Traversal Sequence | CWE-22 | `400/403` | `403` | `101.49 ± 26.31 µs` | `97.27 µs` | `117.21 µs` | `218.38 µs` | `[99.86, 103.12]` | `Closed` | ✅ PASS |
| `TRAVERSAL-002` | Uppercase Percent-Encoded Traversal (%2E%2E) | CWE-22 | `400/403` | `403` | `102.62 ± 32.85 µs` | `97.21 µs` | `119.25 µs` | `241.17 µs` | `[100.59, 104.66]` | `Closed` | ✅ PASS |
| `TRAVERSAL-003` | Double Percent-Encoded Traversal (%252e%252e) | CWE-22 | `400/403` | `403` | `114.60 ± 164.35 µs` | `99.77 µs` | `125.33 µs` | `281.25 µs` | `[104.41, 124.78]` | `Closed` | ✅ PASS |
| `RESOURCE-001` | Oversized Request Header (> 8KB) | CWE-400 | `400/431` | `431` | `82.25 ± 23.89 µs` | `78.71 µs` | `97.29 µs` | `142.88 µs` | `[80.77, 83.73]` | `Closed` | ✅ PASS |
| `RESOURCE-002` | Oversized Single Query Parameter (> 2KB) | CWE-400 | `400/413/414` | `413` | `101.01 ± 89.87 µs` | `91.08 µs` | `116.46 µs` | `274.04 µs` | `[95.44, 106.58]` | `Closed` | ✅ PASS |
| `BASELINE-001` | Standard Valid HTTP/1.1 GET Request | N/A | `200 OK` | `200` | `666.79 ± 466.41 µs` | `548.46 µs` | `1038.88 µs` | `2317.38 µs` | `[637.88, 695.70]` | `Open` | ✅ PASS |
| `CACHE-001` | Web Cache Deception (Private Cache-Control) | CWE-524 | `200/401` | `401` | `696.69 ± 766.96 µs` | `538.65 µs` | `1120.63 µs` | `3610.75 µs` | `[649.15, 744.23]` | `Open` | ✅ PASS |
| `CACHE-002` | Shared Cache Set-Cookie Header Stripping | CWE-384 | `200 OK` | `200` | `582.08 ± 532.03 µs` | `505.04 µs` | `823.04 µs` | `2758.08 µs` | `[549.10, 615.06]` | `Open` | ✅ PASS |
| `CACHE-003` | Authorization Refusal Invariant in Shared Cache | CWE-524 | `200/401` | `401` | `547.70 ± 487.92 µs` | `470.50 µs` | `798.54 µs` | `2234.25 µs` | `[517.46, 577.95]` | `Open` | ✅ PASS |
