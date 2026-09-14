# 🛡️ Differential Protocol Security & Invariant Verification Report

**Target Under Test**: `127.0.0.1:8080`  
**Execution Timestamp**: `2026-09-12T07:44:24Z`  
**Evaluation Mode**: Repeated Statistical Trials ($K=1000$, $W=50$ warm-up discarded)  
**Overall Invariant Pass Rate**: **100.00%** (19/19 tests)  

### Distributional Latency Breakdown (Fail-Fast Defense vs. Comprehensive)

- **Fail-Fast Defense Latency (N=18 Adversarial Rejection Vectors)**:  
  - **Mean**: `152.95 µs`  
  - **Median (p50)**: `68.50 µs`  
  - **Tail Latency (p90)**: `607.75 µs`  
  - **Max Rejection**: `655.00 µs`  

- **Cross-Vector Comprehensive Latency (N=19 Vectors, incl. Baseline 200 OK)**:  
  - **Mean**: `179.06 µs`  
  - **Median (p50)**: `68.50 µs`  
  - **90th Percentile (p90)**: `649.04 µs`  
  - **Tail Latency (p99)**: `655.00 µs`  
  - **Max Latency**: `655.00 µs`  

## 1. Category Summary Matrix

| Security Category | Total Tests | Passed | Failed | Pass Rate |
| :--- | :---: | :---: | :---: | :---: |
| Path Canonicalization | 3 | 3 | 0 | 100.0% |
| RFC Conformance Baseline | 1 | 1 | 0 | 100.0% |
| Request Smuggling (CL.TE) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (CL.CL) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Obfuscation) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Chunked) | 1 | 1 | 0 | 100.0% |
| Header Syntax Invariants | 3 | 3 | 0 | 100.0% |
| Control Character Guards | 3 | 3 | 0 | 100.0% |
| Resource Bounding | 2 | 2 | 0 | 100.0% |
| Cache Session Boundary | 3 | 3 | 0 | 100.0% |

## 2. Detailed Invariant Test Results

| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Mean ± StdDev | Median (p50) | p90 | p99 | 95% CI | Conn Closed | Result |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `SMUGGLE-001` | Conflicting Content-Length and Transfer-Encoding | CWE-444 | `400/501` | `400` | `50.81 ± 19.66 µs` | `46.52 µs` | `78.50 µs` | `106.75 µs` | `[49.60, 52.03]` | `Closed` | ✅ PASS |
| `SMUGGLE-002` | Multiple Divergent Content-Length Headers | CWE-444 | `400/501` | `400` | `50.34 ± 95.10 µs` | `45.94 µs` | `64.17 µs` | `124.75 µs` | `[44.45, 56.24]` | `Closed` | ✅ PASS |
| `SMUGGLE-003` | Transfer-Encoding with Obfuscated Tab Character | CWE-444 | `400/501` | `400` | `52.54 ± 97.77 µs` | `45.92 µs` | `71.04 µs` | `144.63 µs` | `[46.48, 58.60]` | `Closed` | ✅ PASS |
| `SMUGGLE-004` | Invalid Chunk Hex Size Extension | CWE-444 | `400/501` | `501` | `51.55 ± 25.44 µs` | `48.75 µs` | `68.00 µs` | `126.63 µs` | `[49.97, 53.13]` | `Closed` | ✅ PASS |
| `WHITESPACE-001` | Space Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `45.41 ± 16.03 µs` | `43.08 µs` | `59.25 µs` | `91.79 µs` | `[44.41, 46.40]` | `Closed` | ✅ PASS |
| `WHITESPACE-002` | Tab Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `46.35 ± 16.22 µs` | `44.06 µs` | `61.21 µs` | `119.75 µs` | `[45.34, 47.35]` | `Closed` | ✅ PASS |
| `WHITESPACE-003` | Obsolete Line Folding (obs-fold) | CWE-436 | `200/400` | `400` | `51.83 ± 75.65 µs` | `46.44 µs` | `60.88 µs` | `131.79 µs` | `[47.14, 56.52]` | `Closed` | ✅ PASS |
| `CONTROL-001` | Null Byte (0x00) in Request URI | CWE-117 | `400 Bad Request` | `400` | `47.10 ± 21.36 µs` | `44.94 µs` | `61.83 µs` | `127.08 µs` | `[45.78, 48.43]` | `Closed` | ✅ PASS |
| `CONTROL-002` | Bell Character (0x07) in Header Value | CWE-117 | `400 Bad Request` | `400` | `73.88 ± 34.97 µs` | `68.50 µs` | `88.50 µs` | `184.08 µs` | `[71.71, 76.05]` | `Closed` | ✅ PASS |
| `CONTROL-003` | Escape Sequence (0x1B) in Query String | CWE-117 | `400 Bad Request` | `400` | `56.71 ± 344.97 µs` | `43.71 µs` | `59.79 µs` | `125.92 µs` | `[35.33, 78.09]` | `Closed` | ✅ PASS |
| `TRAVERSAL-001` | Raw Dot-Dot Path Traversal Sequence | CWE-22 | `400/403` | `403` | `99.88 ± 136.60 µs` | `88.31 µs` | `112.21 µs` | `224.50 µs` | `[91.42, 108.35]` | `Closed` | ✅ PASS |
| `TRAVERSAL-002` | Uppercase Percent-Encoded Traversal (%2E%2E) | CWE-22 | `400/403` | `403` | `96.43 ± 30.07 µs` | `90.85 µs` | `112.71 µs` | `215.63 µs` | `[94.57, 98.30]` | `Closed` | ✅ PASS |
| `TRAVERSAL-003` | Double Percent-Encoded Traversal (%252e%252e) | CWE-22 | `400/403` | `403` | `102.98 ± 27.34 µs` | `96.94 µs` | `120.29 µs` | `212.21 µs` | `[101.29, 104.68]` | `Closed` | ✅ PASS |
| `RESOURCE-001` | Oversized Request Header (> 8KB) | CWE-400 | `400/431` | `431` | `83.40 ± 39.56 µs` | `79.63 µs` | `94.38 µs` | `170.54 µs` | `[80.95, 85.85]` | `Closed` | ✅ PASS |
| `RESOURCE-002` | Oversized Single Query Parameter (> 2KB) | CWE-400 | `400/413/414` | `413` | `93.93 ± 78.92 µs` | `83.33 µs` | `109.92 µs` | `270.46 µs` | `[89.03, 98.82]` | `Closed` | ✅ PASS |
| `BASELINE-001` | Standard Valid HTTP/1.1 GET Request | N/A | `200 OK` | `200` | `925.79 ± 1340.73 µs` | `649.04 µs` | `1416.54 µs` | `6122.50 µs` | `[842.69, 1008.89]` | `Open` | ✅ PASS |
| `CACHE-001` | Web Cache Deception (Private Cache-Control) | CWE-524 | `200/401` | `401` | `681.49 ± 663.18 µs` | `573.42 µs` | `1045.54 µs` | `2698.42 µs` | `[640.38, 722.59]` | `Open` | ✅ PASS |
| `CACHE-002` | Shared Cache Set-Cookie Header Stripping | CWE-384 | `200 OK` | `200` | `799.76 ± 845.63 µs` | `655.00 µs` | `1142.96 µs` | `4818.58 µs` | `[747.35, 852.17]` | `Open` | ✅ PASS |
| `CACHE-003` | Authorization Refusal Invariant in Shared Cache | CWE-524 | `200/401` | `401` | `758.09 ± 880.14 µs` | `607.75 µs` | `1118.17 µs` | `4176.29 µs` | `[703.54, 812.65]` | `Open` | ✅ PASS |
