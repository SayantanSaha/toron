# 🛡️ Differential Protocol Security & Invariant Verification Report

**Target Under Test**: `127.0.0.1:8080`  
**Execution Timestamp**: `2026-09-12T03:50:14Z`  
**Overall Invariant Pass Rate**: **100.00%** (19/19 tests)  
**Average Fail-Fast Rejection Latency**: `398.71 µs`  

## 1. Category Summary Matrix

| Security Category | Total Tests | Passed | Failed | Pass Rate |
| :--- | :---: | :---: | :---: | :---: |
| Request Smuggling (CL.TE) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (CL.CL) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Obfuscation) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Chunked) | 1 | 1 | 0 | 100.0% |
| Header Syntax Invariants | 3 | 3 | 0 | 100.0% |
| Resource Bounding | 2 | 2 | 0 | 100.0% |
| Control Character Guards | 3 | 3 | 0 | 100.0% |
| Path Canonicalization | 3 | 3 | 0 | 100.0% |
| RFC Conformance Baseline | 1 | 1 | 0 | 100.0% |
| Cache Session Boundary | 3 | 3 | 0 | 100.0% |

## 2. Detailed Invariant Test Results

| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Conn Closed | Fail-Fast Latency | Result |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| `SMUGGLE-001` | Conflicting Content-Length and Transfer-Encoding | CWE-444 | `400/501` | `400` | `Closed` | `362.13 µs` | ✅ PASS |
| `SMUGGLE-002` | Multiple Divergent Content-Length Headers | CWE-444 | `400/501` | `400` | `Closed` | `238.79 µs` | ✅ PASS |
| `SMUGGLE-003` | Transfer-Encoding with Obfuscated Tab Character | CWE-444 | `400/501` | `400` | `Closed` | `208.75 µs` | ✅ PASS |
| `SMUGGLE-004` | Invalid Chunk Hex Size Extension | CWE-444 | `400/501` | `501` | `Closed` | `250.08 µs` | ✅ PASS |
| `WHITESPACE-001` | Space Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `Closed` | `169.58 µs` | ✅ PASS |
| `WHITESPACE-002` | Tab Before Colon in Field Name | CWE-444 | `400 Bad Request` | `400` | `Closed` | `177.04 µs` | ✅ PASS |
| `WHITESPACE-003` | Obsolete Line Folding (obs-fold) | CWE-436 | `200/400` | `400` | `Closed` | `100.29 µs` | ✅ PASS |
| `CONTROL-001` | Null Byte (0x00) in Request URI | CWE-117 | `400 Bad Request` | `400` | `Closed` | `179.04 µs` | ✅ PASS |
| `CONTROL-002` | Bell Character (0x07) in Header Value | CWE-117 | `400 Bad Request` | `400` | `Closed` | `712.08 µs` | ✅ PASS |
| `CONTROL-003` | Escape Sequence (0x1B) in Query String | CWE-117 | `400 Bad Request` | `400` | `Closed` | `87.29 µs` | ✅ PASS |
| `TRAVERSAL-001` | Raw Dot-Dot Path Traversal Sequence | CWE-22 | `400/403` | `403` | `Closed` | `210.92 µs` | ✅ PASS |
| `TRAVERSAL-002` | Uppercase Percent-Encoded Traversal (%2E%2E) | CWE-22 | `400/403` | `403` | `Closed` | `165.58 µs` | ✅ PASS |
| `TRAVERSAL-003` | Double Percent-Encoded Traversal (%252e%252e) | CWE-22 | `400/403` | `403` | `Closed` | `187.96 µs` | ✅ PASS |
| `RESOURCE-001` | Oversized Request Header (> 8KB) | CWE-400 | `400/431` | `431` | `Closed` | `148.46 µs` | ✅ PASS |
| `RESOURCE-002` | Oversized Single Query Parameter (> 2KB) | CWE-400 | `400/413/414` | `413` | `Closed` | `131.25 µs` | ✅ PASS |
| `BASELINE-001` | Standard Valid HTTP/1.1 GET Request | N/A | `200 OK` | `200` | `Open` | `2378.00 µs` | ✅ PASS |
| `CACHE-001` | Web Cache Deception (Private Cache-Control) | CWE-524 | `200/401` | `401` | `Open` | `577.00 µs` | ✅ PASS |
| `CACHE-002` | Shared Cache Set-Cookie Header Stripping | CWE-384 | `200 OK` | `200` | `Open` | `708.58 µs` | ✅ PASS |
| `CACHE-003` | Authorization Refusal Invariant in Shared Cache | CWE-524 | `200/401` | `401` | `Open` | `582.58 µs` | ✅ PASS |
