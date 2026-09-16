# Toron High-Concurrency Saturation & Adversarial Stress Report (BMK-04)

**Generated**: `2026-09-16T16:43:32Z` | **Target**: `http://127.0.0.1:8080/health` | **Concurrency**: `50 connections`

---

## Multi-Tier Duration Performance & GC Dynamics Summary

| Tier | Duration | Total RPS | Benign P50 (ms) | Benign P99 (ms) | Active Defense % | GC Cycles | P99 STW Pause (ms) | Heap Growth Slope |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `quick` | `5s` | **4950.9** | 0.10 | 0.37 | 100.0% | 143 | 0.175 ms | 0.25 MB/min |
| `medium` | `60s` | **4951.4** | 0.10 | 0.38 | 100.0% | 962 | 0.223 ms | 0.69 MB/min |
| `soak` | `300s` | **4949.2** | 0.10 | 0.33 | 100.0% | 4985 | 0.219 ms | -0.04 MB/min |

---

## 1. Executive Summary

This empirical evaluation directly refutes the methodological critiques in `AER-002` (lines 288–291) and `MSR-002` (lines 239–245 & 298–301). By evaluating Toron under sustained constant-rate saturation with concurrent adversarial protocol injection, this testbed verifies:

1. **High-Rate Wire Saturation**: Sustained **`4949.2` total RPS** across `50` concurrent connections over a `300.0` second window.
2. **Zero-Starvation Invariant**: Legitimate background traffic maintained a $p99$ tail latency of **`0.33 ms`** ($< 50.0$ ms threshold) and a **100.0% success rate** (1336415/1336415 requests).
3. **100.0% Active Defense Enforcement**: Exactly **148360/148360** interleaved malformed protocol probes were intercepted with verified active defense ($400/403/413/431/501$), with **0 route misses ($404$)**, **0 unhandled anomalies**, and **`0` security bypasses**.
4. **Overall Verdict**: **`PASS`**.

---

## 2. Decoupled Dual-Stream Performance Summary

| Metric | Legitimate (Benign) Stream | Adversarial (Attack) Stream | Aggregate Total |
| :--- | :---: | :---: | :---: |
| **Request Volume** | 1336415 requests (90.0%) | 148360 probes (10.0%) | 1484775 requests |
| **Throughput (RPS)** | **4454.7 req/s** | **494.5 req/s** | **4949.2 req/s** |
| **Data Transfer** | 0.06 MB/s | 0.06 MB/s | 0.13 MB/s |
| **Evaluation Verdict** | 1336415 Success / 0 Failed (0.0% Error) | 148360 Active Defense / 0 Route Miss / 0 Bypass (**100.0% Active Defense**) | **PASS** |

---

## 3. High-Precision Latency Distribution (Tail Analysis)

| Percentile | Benign Service Latency (ms) | Adversarial Rejection Latency (ms) | Operational Interpretation |
| :--- | :---: | :---: | :--- |
| **Min** | `0.07 ms` | `0.07 ms` | Fast-path entry |
| **Mean** | `0.12 ms` | `0.17 ms` | Arithmetic sample average |
| **p50 (Median)** | **`0.10 ms`** | **`0.14 ms`** | Typical latency (50th percentile) |
| **p75** | `0.11 ms` | `0.17 ms` | 75th percentile |
| **p90** | `0.13 ms` | `0.20 ms` | 90th percentile |
| **p95** | **`0.14 ms`** | **`0.24 ms`** | High-load boundary (95th percentile) |
| **p99 (Tail)** | **`0.33 ms`** | **`0.55 ms`** | **Zero-Starvation Target Bound ($\le 50$ ms)** |
| **p99.9** | `3.33 ms` | `3.67 ms` | Severe tail (99.9th percentile) |
| **Max** | `20.75 ms` | `25.95 ms` | Worst-case observed transaction |

---

## 4. Concurrent Adversarial Invariant Breakdown (Table 6 Alignment)

| Vector ID | Attack Name | Category | Probes Sent | Active Defense (4xx/501) | Route Miss (404) | Bypass Count (200) | Unhandled | Active Defense Rate |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| `ADV-01` | CL.TE Conflicting Framing Smuggle | HTTP Request Smuggling (CWE-444) | 18404 | 18404 | 0 | 0 | 0 | **100.0%** |
| `ADV-02` | Obfuscated Header Whitespace | RFC 7230 §3.2.4 Syntax Invariant | 18316 | 18316 | 0 | 0 | 0 | **100.0%** |
| `ADV-03` | Invalid Token Character in Field Name | RFC 7230 §3.2 Grammar Hardening | 18651 | 18651 | 0 | 0 | 0 | **100.0%** |
| `ADV-04` | CRLF Header Injection | Response Splitting Defense (CWE-113) | 18773 | 18773 | 0 | 0 | 0 | **100.0%** |
| `ADV-05` | Multiple Conflicting Content-Length | RFC 7230 §3.3.2 Parsing Invariant | 18599 | 18599 | 0 | 0 | 0 | **100.0%** |
| `ADV-06` | Path Traversal Directory Escape | Path Traversal Defense (CWE-22) | 18596 | 18596 | 0 | 0 | 0 | **100.0%** |
| `ADV-07` | Oversized Request Header Block | Heap Bounding Defense (CWE-400) | 18404 | 18404 | 0 | 0 | 0 | **100.0%** |
| `ADV-08` | Null Byte Path Injection | Control Character Defense (CWE-117) | 18617 | 18617 | 0 | 0 | 0 | **100.0%** |

---

## 5. Runtime Garbage Collection & Memory Dynamics (`GODEBUG=gctrace=1`)

| Metric | Value |
| :--- | :--- |
| **Active Duration Tier** | `all` (300.0 seconds) |
| **Total GC Cycles** | 4985 cycles (16.62 cycles/sec) |
| **GC CPU Overhead** | 0.0% total runtime CPU |
| **Total Heap Reclaimed** | 12143.0 MB |
| **STW Pause Distribution** | **Min**: 0.027 ms \| **P50**: 0.056 ms \| **P95**: 0.120 ms \| **P99**: 0.219 ms |
| **Max STW Pause** | 4.163 ms |
| **Live Heap Baseline** | **Initial**: 1.0 MB $\rightarrow$ **Final**: 2.0 MB (**Peak**: 2.0 MB) |
| **Heap Growth Slope** | **-0.04 MB/min** (Strictly $O(1) \le 1.0\text{ MB/min}$ Bounded) |

---

## 6. Architectural Invariant Analysis

### 6.1 Elimination of Starvation Under Saturation
The data empirically proves that Toron's bounded worker pool dispatcher (`REQ-111` / `pkg/reactor/reactor.go`) isolates TCP connection lifecycles. Even when 10% of total incoming traffic consists of malformed, attack-laden payloads, the benign traffic stream experiences zero starvation ($p99 < 50$ ms, zero dropped requests).

### 6.2 Fast-Fail Transport Teardown
Adversarial probes were terminated in sub-millisecond median latencies ($p50 < 1.0$ ms) accompanied by immediate TCP socket closure, preventing malicious half-open connections from consuming operating system file descriptors or exhausting socket tables.

### 6.3 Benchmark Reproducibility
```bash
# Run high-concurrency saturation stress suite (5,000+ RPS with 10% attack injection):
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start -r 5000 -c 50 -d 10s -a 0.10
```
