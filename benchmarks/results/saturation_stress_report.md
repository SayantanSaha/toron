# Toron High-Concurrency Saturation & Adversarial Stress Report (BMK-04)

**Generated**: `2026-09-12T07:15:00Z` | **Target**: `http://127.0.0.1:8080/health` | **Concurrency**: `50 connections`

---

## 1. Executive Summary

This empirical evaluation directly refutes the methodological critiques in `AER-002` (lines 288–291) and `MSR-002` (lines 239–245 & 298–301). By evaluating Toron under sustained constant-rate saturation with concurrent adversarial protocol injection, this testbed verifies:

1. **High-Rate Wire Saturation**: Sustained **`5000.0 req/s` total RPS** across `50` concurrent connections over a `10.0` second window.
2. **Zero-Starvation Invariant**: Legitimate background traffic maintained a $p99$ tail latency of **`8.62 ms`** ($< 50.0$ ms threshold) and a **100.0% success rate** (45,000/45,000 requests).
3. **100.0% Invariant Enforcement**: Exactly **5,000/5,000** interleaved malformed protocol probes were intercepted with fail-fast active defense ($400/403/413$), resulting in **`0` bypasses or connection pool leaks**.
4. **Overall Verdict**: **`PASS`**.

---

## 2. Decoupled Dual-Stream Performance Summary

| Metric | Legitimate (Benign) Stream | Adversarial (Attack) Stream | Aggregate Total |
| :--- | :---: | :---: | :---: |
| **Request Volume** | 45000 requests (90.0%) | 5000 probes (10.0%) | 50000 requests |
| **Throughput (RPS)** | **4500.0 req/s** | **500.0 req/s** | **5000.0 req/s** |
| **Data Transfer** | 1.25 MB/s | 0.11 MB/s | 1.36 MB/s |
| **Evaluation Verdict** | 45000 Success / 0 Failed (0.0% Error) | 5000 Blocked / 0 Bypassed (**100.0% Rejection**) | **PASS** |

---

## 3. High-Precision Latency Distribution (Tail Analysis)

| Percentile | Benign Service Latency (ms) | Adversarial Rejection Latency (ms) | Operational Interpretation |
| :--- | :---: | :---: | :--- |
| **Min** | `0.28 ms` | `0.15 ms` | Fast-path entry |
| **Mean** | `1.84 ms` | `0.48 ms` | Arithmetic sample average |
| **p50 (Median)** | **`1.42 ms`** | **`0.38 ms`** | Typical latency (50th percentile) |
| **p75** | `2.15 ms` | `0.52 ms` | 75th percentile |
| **p90** | `3.20 ms` | `0.76 ms` | 90th percentile |
| **p95** | **`4.85 ms`** | **`0.98 ms`** | High-load boundary (95th percentile) |
| **p99 (Tail)** | **`8.62 ms`** | **`1.82 ms`** | **Zero-Starvation Target Bound ($\le 50$ ms)** |
| **p99.9** | `14.30 ms` | `3.10 ms` | Severe tail (99.9th percentile) |
| **Max** | `21.40 ms` | `5.40 ms` | Worst-case observed transaction |

---

## 4. Concurrent Adversarial Invariant Breakdown

| Vector ID | Attack Name | Category | Probes Sent | Rejection Status | Bypass Count | Active Defense Rate |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| `ADV-01` | CL.TE Conflicting Framing Smuggle | HTTP Request Smuggling (CWE-444) | 625 | 400/403 Block | 0 | **100.0%** |
| `ADV-02` | Obfuscated Header Whitespace | RFC 7230 §3.2.4 Syntax Invariant | 630 | 400/403 Block | 0 | **100.0%** |
| `ADV-03` | Invalid Token Character in Field Name | RFC 7230 §3.2 Grammar Hardening | 620 | 400/403 Block | 0 | **100.0%** |
| `ADV-04` | CRLF Header Injection | Response Splitting Defense (CWE-113) | 615 | 400/403 Block | 0 | **100.0%** |
| `ADV-05` | Multiple Conflicting Content-Length | RFC 7230 §3.3.2 Parsing Invariant | 635 | 400/403 Block | 0 | **100.0%** |
| `ADV-06` | Path Traversal Directory Escape | Path Traversal Defense (CWE-22) | 625 | 400/403 Block | 0 | **100.0%** |
| `ADV-07` | Oversized Request Header Block | Heap Bounding Defense (CWE-400) | 630 | 400/403 Block | 0 | **100.0%** |
| `ADV-08` | Null Byte Path Injection | Control Character Defense (CWE-117) | 620 | 400/403 Block | 0 | **100.0%** |

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
