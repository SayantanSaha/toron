# 🛡️ Differential Protocol Security & Invariant Verification Report

**Target Under Test**: `127.0.0.1:8080`  
**Execution Timestamp**: `2026-09-12T09:29:17Z`  
**Evaluation Mode**: Repeated Statistical Trials ($K=1000$, $W=50$ warm-up discarded)  
**Overall Invariant Pass Rate**: **100.00%** (19/19 tests)  

### Distributional Latency Breakdown (Fail-Fast Defense vs. Comprehensive)

- **Fail-Fast Defense Latency (N=18 Adversarial Rejection Vectors)**:  
  - **Mean**: `162.65 µs`  
  - **Median (p50)**: `64.40 µs`  
  - **Tail Latency (p90)**: `679.42 µs`  
  - **Max Rejection**: `681.13 µs`  

- **Cross-Vector Comprehensive Latency (N=19 Vectors, incl. Baseline 200 OK)**:  
  - **Mean**: `196.45 µs`  
  - **Median (p50)**: `64.40 µs`  
  - **90th Percentile (p90)**: `681.13 µs`  
  - **Tail Latency (p99)**: `804.92 µs`  
  - **Max Latency**: `804.92 µs`  

## 1. Category Summary Matrix

| Security Category | Total Tests | Passed | Failed | Pass Rate |
| :--- | :---: | :---: | :---: | :---: |
| Request Smuggling (CL.CL) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Obfuscation) | 1 | 1 | 0 | 100.0% |
| Header Syntax Invariants | 3 | 3 | 0 | 100.0% |
| Control Character Guards | 3 | 3 | 0 | 100.0% |
| Path Canonicalization | 3 | 3 | 0 | 100.0% |
| Resource Bounding | 2 | 2 | 0 | 100.0% |
| Request Smuggling (CL.TE) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Chunked) | 1 | 1 | 0 | 100.0% |
| RFC Conformance Baseline | 1 | 1 | 0 | 100.0% |
| Cache Session Boundary | 3 | 3 | 0 | 100.0% |

## 2. Detailed Invariant Test Results

| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Mean ± StdDev | Median (p50) | p90 | p99 | 95% CI | Conn Closed | Result |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `SMUGGLE-001` | Conflicting Content-Length and Transfer-Encoding | CWE-444 | `400/501` | `400` | `45.14 ± 22.56 µs` | `42.29 µs` | `60.08 µs` | `102.92 µs` | `[43.74, 46.53]` | `Closed` | ✅ PASS |
| `SMUGGLE-002` | Multiple Divergent Content-Length Headers | CWE-444 | `400/501` | `400` | `49.11 ± 15.38 µs` | `47.42 µs` | `62.13 µs` | `112.13 µs` | `[48.16, 50.07]` | `Closed` | ✅ PASS |
| `SMUGGLE-003` | Transfer-Encoding with Obfuscated Tab Character | CWE-444 | `400/501` | `400` | `46.83 ± 23.98 µs` | `43.92 µs` | `59.29 µs` | `113.79 µs` | `[45.34, 48.32]` | `Closed` | ✅ PASS |
| `SMUGGLE-004` | Invalid Chunk Hex Size Extension | CWE-444 | `400/501` | `501` | `46.21 ± 16.05 µs` | `43.83 µs` | `59.63 µs` | `96.58 µs` | `[45.21, 47.20]` | `Closed` | ✅ PASS |
| `WHITESPACE-001` | Space Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `44.58 ± 14.97 µs` | `43.02 µs` | `57.54 µs` | `94.21 µs` | `[43.65, 45.51]` | `Closed` | ✅ PASS |
| `WHITESPACE-002` | Tab Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `47.83 ± 20.45 µs` | `43.75 µs` | `64.63 µs` | `137.00 µs` | `[46.57, 49.10]` | `Closed` | ✅ PASS |
| `WHITESPACE-003` | Obsolete Line Folding (obs-fold) | CWE-436 | `200/400` | `400` | `49.35 ± 14.23 µs` | `47.75 µs` | `61.63 µs` | `96.17 µs` | `[48.47, 50.23]` | `Closed` | ✅ PASS |
| `CONTROL-001` | Null Byte (0x00) in Request URI | CWE-117 | `400 Bad Request` | `400` | `43.90 ± 25.52 µs` | `42.75 µs` | `57.67 µs` | `126.96 µs` | `[42.32, 45.48]` | `Closed` | ✅ PASS |
| `CONTROL-002` | Bell Character (0x07) in Header Value | CWE-117 | `400 Bad Request` | `400` | `75.73 ± 204.71 µs` | `64.40 µs` | `86.79 µs` | `175.71 µs` | `[63.04, 88.42]` | `Closed` | ✅ PASS |
| `CONTROL-003` | Escape Sequence (0x1B) in Query String | CWE-117 | `400 Bad Request` | `400` | `46.84 ± 17.49 µs` | `45.44 µs` | `59.13 µs` | `92.08 µs` | `[45.76, 47.93]` | `Closed` | ✅ PASS |
| `TRAVERSAL-001` | Raw Dot-Dot Path Traversal Sequence | CWE-22 | `400/403` | `403` | `99.99 ± 27.42 µs` | `94.71 µs` | `116.96 µs` | `197.00 µs` | `[98.29, 101.69]` | `Closed` | ✅ PASS |
| `TRAVERSAL-002` | Uppercase Percent-Encoded Traversal (%2E%2E) | CWE-22 | `400/403` | `403` | `98.81 ± 26.76 µs` | `94.85 µs` | `113.17 µs` | `205.04 µs` | `[97.15, 100.47]` | `Closed` | ✅ PASS |
| `TRAVERSAL-003` | Double Percent-Encoded Traversal (%252e%252e) | CWE-22 | `400/403` | `403` | `108.43 ± 29.42 µs` | `103.94 µs` | `123.17 µs` | `223.08 µs` | `[106.61, 110.26]` | `Closed` | ✅ PASS |
| `RESOURCE-001` | Oversized Request Header (> 8KB) | CWE-400 | `400/431` | `431` | `82.41 ± 21.33 µs` | `80.46 µs` | `94.71 µs` | `137.00 µs` | `[81.09, 83.73]` | `Closed` | ✅ PASS |
| `RESOURCE-002` | Oversized Single Query Parameter (> 2KB) | CWE-400 | `400/413/414` | `413` | `98.63 ± 87.35 µs` | `89.08 µs` | `111.54 µs` | `274.33 µs` | `[93.22, 104.05]` | `Closed` | ✅ PASS |
| `BASELINE-001` | Standard Valid HTTP/1.1 GET Request | N/A | `200 OK` | `200` | `844.90 ± 501.45 µs` | `804.92 µs` | `1197.58 µs` | `3261.83 µs` | `[813.82, 875.98]` | `Open` | ✅ PASS |
| `CACHE-001` | Web Cache Deception (Private Cache-Control) | CWE-524 | `200/401` | `401` | `767.48 ± 570.87 µs` | `679.42 µs` | `1139.38 µs` | `3071.38 µs` | `[732.09, 802.86]` | `Open` | ✅ PASS |
| `CACHE-002` | Shared Cache Set-Cookie Header Stripping | CWE-384 | `200 OK` | `200` | `765.39 ± 650.37 µs` | `639.52 µs` | `1227.88 µs` | `3910.46 µs` | `[725.08, 805.70]` | `Open` | ✅ PASS |
| `CACHE-003` | Authorization Refusal Invariant in Shared Cache | CWE-524 | `200/401` | `401` | `795.12 ± 647.68 µs` | `681.13 µs` | `1240.67 µs` | `3494.38 µs` | `[754.97, 835.26]` | `Open` | ✅ PASS |
