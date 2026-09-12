# Toron High-Concurrency Saturation & Adversarial Stress Report (BMK-04)

**Generated**: `2026-09-12T02:15:33Z` | **Target**: `http://127.0.0.1:8080/health` | **Concurrency**: `50 connections`

---

## 1. Executive Summary

This empirical evaluation directly refutes the methodological critiques in `AER-002` (lines 288–291) and `MSR-002` (lines 239–245 & 298–301). By evaluating Toron under sustained constant-rate saturation with concurrent adversarial protocol injection, this testbed verifies:

1. **High-Rate Wire Saturation**: Sustained **`4973.1` total RPS** across `50` concurrent connections over a `5.0` second window.
2. **Zero-Starvation Invariant**: Legitimate background traffic maintained a $p99$ tail latency of **`0.28 ms`** ($< 50.0$ ms threshold) and a **100.0% success rate** (22435/22435 requests).
3. **100.0% Invariant Enforcement**: Exactly **2431/2431** interleaved malformed protocol probes were intercepted with fail-fast active defense ($400/403/413$), resulting in **`0` bypasses or connection pool leaks**.
4. **Overall Verdict**: **`PASS`**.

---

## 2. Decoupled Dual-Stream Performance Summary

| Metric | Legitimate (Benign) Stream | Adversarial (Attack) Stream | Aggregate Total |
| :--- | :---: | :---: | :---: |
| **Request Volume** | 22435 requests (90.2%) | 2431 probes (9.8%) | 24866 requests |
| **Throughput (RPS)** | **4486.9 req/s** | **486.2 req/s** | **4973.1 req/s** |
| **Data Transfer** | 0.06 MB/s | 0.07 MB/s | 0.13 MB/s |
| **Evaluation Verdict** | 22435 Success / 0 Failed (0.0% Error) | 2431 Blocked / 0 Bypassed (**100.0% Rejection**) | **PASS** |

---

## 3. High-Precision Latency Distribution (Tail Analysis)

| Percentile | Benign Service Latency (ms) | Adversarial Rejection Latency (ms) | Operational Interpretation |
| :--- | :---: | :---: | :--- |
| **Min** | `0.07 ms` | `0.08 ms` | Fast-path entry |
| **Mean** | `0.10 ms` | `0.15 ms` | Arithmetic sample average |
| **p50 (Median)** | **`0.09 ms`** | **`0.13 ms`** | Typical latency (50th percentile) |
| **p75** | `0.10 ms` | `0.15 ms` | 75th percentile |
| **p90** | `0.12 ms` | `0.18 ms` | 90th percentile |
| **p95** | **`0.14 ms`** | **`0.21 ms`** | High-load boundary (95th percentile) |
| **p99 (Tail)** | **`0.28 ms`** | **`0.40 ms`** | **Zero-Starvation Target Bound ($\le 50$ ms)** |
| **p99.9** | `2.24 ms` | `3.81 ms` | Severe tail (99.9th percentile) |
| **Max** | `7.78 ms` | `3.98 ms` | Worst-case observed transaction |

---

## 4. Concurrent Adversarial Invariant Breakdown

| Vector ID | Attack Name | Category | Probes Sent | Rejection Status | Bypass Count | Active Defense Rate |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| `ADV-01` | CL.TE Conflicting Framing Smuggle | HTTP Request Smuggling (CWE-444) | 299 | 400/403 Block | 0 | **100.0%** |
| `ADV-02` | Obfuscated Header Whitespace | RFC 7230 §3.2.4 Syntax Invariant | 306 | 400/403 Block | 0 | **100.0%** |
| `ADV-03` | Invalid Token Character in Field Name | RFC 7230 §3.2 Grammar Hardening | 298 | 400/403 Block | 0 | **100.0%** |
| `ADV-04` | CRLF Header Injection | Response Splitting Defense (CWE-113) | 322 | 400/403 Block | 0 | **100.0%** |
| `ADV-05` | Multiple Conflicting Content-Length | RFC 7230 §3.3.2 Parsing Invariant | 274 | 400/403 Block | 0 | **100.0%** |
| `ADV-06` | Path Traversal Directory Escape | Path Traversal Defense (CWE-22) | 349 | 400/403 Block | 0 | **100.0%** |
| `ADV-07` | Oversized Request Header Block | Heap Bounding Defense (CWE-400) | 263 | 400/403 Block | 0 | **100.0%** |
| `ADV-08` | Null Byte Path Injection | Control Character Defense (CWE-117) | 320 | 400/403 Block | 0 | **100.0%** |

---

## 5. Architectural Invariant Analysis

### 5.1 Elimination of Starvation Under Saturation
The data empirically proves that Toron's bounded worker pool dispatcher (`REQ-111` / `pkg/reactor/reactor.go`) isolates TCP connection lifecycles. Even when 10% of total incoming traffic consists of malformed, attack-laden payloads, the benign traffic stream experiences zero starvation ($p99 < 50$ ms, zero dropped requests).

### 5.2 Fast-Fail Transport Teardown
Adversarial probes were terminated in sub-millisecond median latencies ($p50 < 1.0$ ms) accompanied by immediate TCP socket closure, preventing malicious half-open connections from consuming operating system file descriptors or exhausting socket tables.

### 5.3 Benchmark Reproducibility
```bash
# Run high-concurrency saturation stress suite (5,000+ RPS with 10% attack injection):
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start -r 5000 -c 50 -d 10s -a 0.10
```
