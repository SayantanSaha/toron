# 🛡️ Differential Protocol Security & Invariant Verification Report

**Target Under Test**: `127.0.0.1:8080`  
**Execution Timestamp**: `2026-09-12T06:50:58Z`  
**Evaluation Mode**: Single-Shot Cross-Vector Verification ($K=1$, $W=0$)  
**Overall Invariant Pass Rate**: **100.00%** (19/19 tests)  

### Distributional Latency Breakdown (Fail-Fast Defense vs. Comprehensive)

- **Fail-Fast Defense Latency (N=18 Adversarial Rejection Vectors)**:  
  - **Mean**: `279.60 µs`  
  - **Median (p50)**: `230.75 µs`  
  - **Tail Latency (p90)**: `574.67 µs`  
  - **Max Rejection**: `672.83 µs`  

- **Cross-Vector Comprehensive Latency (N=19 Vectors, incl. Baseline 200 OK)**:  
  - **Mean**: `324.35 µs`  
  - **Median (p50)**: `230.75 µs`  
  - **90th Percentile (p90)**: `672.83 µs`  
  - **Tail Latency (p99)**: `1129.75 µs`  
  - **Max Latency**: `1129.75 µs`  

## 1. Category Summary Matrix

| Security Category | Total Tests | Passed | Failed | Pass Rate |
| :--- | :---: | :---: | :---: | :---: |
| Request Smuggling (CL.TE) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Obfuscation) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Chunked) | 1 | 1 | 0 | 100.0% |
| Cache Session Boundary | 3 | 3 | 0 | 100.0% |
| Request Smuggling (CL.CL) | 1 | 1 | 0 | 100.0% |
| Header Syntax Invariants | 3 | 3 | 0 | 100.0% |
| Control Character Guards | 3 | 3 | 0 | 100.0% |
| Path Canonicalization | 3 | 3 | 0 | 100.0% |
| Resource Bounding | 2 | 2 | 0 | 100.0% |
| RFC Conformance Baseline | 1 | 1 | 0 | 100.0% |

## 2. Detailed Invariant Test Results

| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Conn Closed | Latency (µs) | Result |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| `SMUGGLE-001` | Conflicting Content-Length and Transfer-Encoding | CWE-444 | `400/501` | `400` | `Closed` | `388.54 µs` | ✅ PASS |
| `SMUGGLE-002` | Multiple Divergent Content-Length Headers | CWE-444 | `400/501` | `400` | `Closed` | `120.67 µs` | ✅ PASS |
| `SMUGGLE-003` | Transfer-Encoding with Obfuscated Tab Character | CWE-444 | `400/501` | `400` | `Closed` | `95.46 µs` | ✅ PASS |
| `SMUGGLE-004` | Invalid Chunk Hex Size Extension | CWE-444 | `400/501` | `501` | `Closed` | `100.58 µs` | ✅ PASS |
| `WHITESPACE-001` | Space Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `Closed` | `73.54 µs` | ✅ PASS |
| `WHITESPACE-002` | Tab Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `Closed` | `88.17 µs` | ✅ PASS |
| `WHITESPACE-003` | Obsolete Line Folding (obs-fold) | CWE-436 | `200/400` | `400` | `Closed` | `137.79 µs` | ✅ PASS |
| `CONTROL-001` | Null Byte (0x00) in Request URI | CWE-117 | `400 Bad Request` | `400` | `Closed` | `173.17 µs` | ✅ PASS |
| `CONTROL-002` | Bell Character (0x07) in Header Value | CWE-117 | `400 Bad Request` | `400` | `Closed` | `574.67 µs` | ✅ PASS |
| `CONTROL-003` | Escape Sequence (0x1B) in Query String | CWE-117 | `400 Bad Request` | `400` | `Closed` | `200.04 µs` | ✅ PASS |
| `TRAVERSAL-001` | Raw Dot-Dot Path Traversal Sequence | CWE-22 | `400/403` | `403` | `Closed` | `333.63 µs` | ✅ PASS |
| `TRAVERSAL-002` | Uppercase Percent-Encoded Traversal (%2E%2E) | CWE-22 | `400/403` | `403` | `Closed` | `181.42 µs` | ✅ PASS |
| `TRAVERSAL-003` | Double Percent-Encoded Traversal (%252e%252e) | CWE-22 | `400/403` | `403` | `Closed` | `390.50 µs` | ✅ PASS |
| `RESOURCE-001` | Oversized Request Header (> 8KB) | CWE-400 | `400/431` | `431` | `Closed` | `238.54 µs` | ✅ PASS |
| `RESOURCE-002` | Oversized Single Query Parameter (> 2KB) | CWE-400 | `400/413/414` | `413` | `Closed` | `230.75 µs` | ✅ PASS |
| `BASELINE-001` | Standard Valid HTTP/1.1 GET Request | N/A | `200 OK` | `200` | `Open` | `1129.75 µs` | ✅ PASS |
| `CACHE-001` | Web Cache Deception (Private Cache-Control) | CWE-524 | `200/401` | `401` | `Open` | `672.83 µs` | ✅ PASS |
| `CACHE-002` | Shared Cache Set-Cookie Header Stripping | CWE-384 | `200 OK` | `200` | `Open` | `546.50 µs` | ✅ PASS |
| `CACHE-003` | Authorization Refusal Invariant in Shared Cache | CWE-524 | `200/401` | `401` | `Open` | `486.08 µs` | ✅ PASS |
