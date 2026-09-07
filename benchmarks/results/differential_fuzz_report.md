# 🛡️ Differential Protocol Security & Invariant Verification Report

**Target Under Test**: `127.0.0.1:8080`  
**Execution Timestamp**: `2026-09-07T08:55:30Z`  
**Overall Invariant Pass Rate**: **100.00%** (16/16 tests)  
**Average Fail-Fast Rejection Latency**: `298.88 µs`  

## 1. Category Summary Matrix

| Security Category | Total Tests | Passed | Failed | Pass Rate |
| :--- | :---: | :---: | :---: | :---: |
| Resource Bounding | 2 | 2 | 0 | 100.0% |
| RFC Conformance Baseline | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Obfuscation) | 1 | 1 | 0 | 100.0% |
| Path Canonicalization | 3 | 3 | 0 | 100.0% |
| Request Smuggling (CL.TE) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (CL.CL) | 1 | 1 | 0 | 100.0% |
| Request Smuggling (Chunked) | 1 | 1 | 0 | 100.0% |
| Header Syntax Invariants | 3 | 3 | 0 | 100.0% |
| Control Character Guards | 3 | 3 | 0 | 100.0% |

## 2. Detailed Invariant Test Results

| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Fail-Fast Latency | Result |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: |
| `SMUGGLE-001` | Conflicting Content-Length and Transfer-Encoding | CWE-444 | `400/501` | `400` | `646 µs` | ✅ PASS |
| `SMUGGLE-002` | Multiple Divergent Content-Length Headers | CWE-444 | `400/501` | `400` | `271 µs` | ✅ PASS |
| `SMUGGLE-003` | Transfer-Encoding with Obfuscated Tab Character | CWE-444 | `400/501` | `400` | `344 µs` | ✅ PASS |
| `SMUGGLE-004` | Invalid Chunk Hex Size Extension | CWE-444 | `400/501` | `501` | `329 µs` | ✅ PASS |
| `WHITESPACE-001` | Space Before Colon in Field Name | CWE-444 | `400/501` | `400` | `229 µs` | ✅ PASS |
| `WHITESPACE-002` | Tab Before Colon in Field Name | CWE-444 | `400/501` | `400` | `263 µs` | ✅ PASS |
| `WHITESPACE-003` | Obsolete Line Folding (obs-fold) | CWE-436 | `400/501` | `400` | `236 µs` | ✅ PASS |
| `CONTROL-001` | Null Byte (0x00) in Request URI | CWE-117 | `400/501` | `400` | `254 µs` | ✅ PASS |
| `CONTROL-002` | Bell Character (0x07) in Header Value | CWE-117 | `400/501` | `400` | `679 µs` | ✅ PASS |
| `CONTROL-003` | Escape Sequence (0x1B) in Query String | CWE-117 | `400/501` | `400` | `239 µs` | ✅ PASS |
| `TRAVERSAL-001` | Raw Dot-Dot Path Traversal Sequence | CWE-22 | `400/501` | `403` | `232 µs` | ✅ PASS |
| `TRAVERSAL-002` | Uppercase Percent-Encoded Traversal (%2E%2E) | CWE-22 | `400/501` | `404` | `201 µs` | ✅ PASS |
| `TRAVERSAL-003` | Double Percent-Encoded Traversal (%252e%252e) | CWE-22 | `400/501` | `403` | `218 µs` | ✅ PASS |
| `RESOURCE-001` | Oversized Request Header (> 8KB) | CWE-400 | `400/501` | `431` | `231 µs` | ✅ PASS |
| `RESOURCE-002` | Oversized Single Query Parameter (> 2KB) | CWE-400 | `400/501` | `413` | `202 µs` | ✅ PASS |
| `BASELINE-001` | Standard Valid HTTP/1.1 GET Request | N/A | `400/501` | `200` | `208 µs` | ✅ PASS |
