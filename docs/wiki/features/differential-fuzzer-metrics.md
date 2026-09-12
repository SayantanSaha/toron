# Differential Protocol Security Fuzzer & Latency Metric Distribution (BMK-02 / HARN-02)

## Overview

The **Toron Differential Protocol Security Fuzzer & Invariant Checker** (`benchmarks/fuzzer/diff_fuzzer.go`) evaluates edge gateway protocol correctness, RFC compliance, and active security defense across 19 curated invariant attack and conformance vectors.

Under **HARN-02** / **REQ-118**, the fuzzer harness was calibrated to disaggregate fail-fast adversarial rejection traffic from benign baseline traffic and provide unconditional, publication-grade distributional metric reporting ($p50$, $p90$, $p99$).

---

## 1. Distributional Metric Architecture

### 1.1 Analytical Cohort Disaggregation

The fuzzer partitions test executions into two distinct analytical cohorts:

```
+--------------------------------------------------------------------------------------------------+
|                            Differential Fuzzer Metric Reporting Pipeline                         |
+--------------------------------------------------------------------------------------------------+
                                                 |
         +---------------------------------------+---------------------------------------+
         |                                                                               |
         v                                                                               v
[ Cohort A: Fail-Fast Defense ]                                         [ Cohort B: Comprehensive Suite ]
  - N = 18 Adversarial Vectors                                            - N = 19 Total Vectors (incl. BASELINE-001)
  - Intercepted with 400/403/413/431/501                                  - Combines rejections + 200 OK baseline
  - Socket closed immediately (conn:closed)                               - Evaluates full testbed response profile
  - Metrics: Mean, p50, p90, Max                                          - Metrics: Mean, p50, p90, p99, Max
```

1. **Cohort A: Fail-Fast Defense Latency ($N=18$ Vectors)**:
   - Includes all adversarial attack vectors (`SMUGGLE-001`..`004`, `WHITESPACE-001`..`003`, `CONTROL-001`..`003`, `TRAVERSAL-001`..`003`, `RESOURCE-001`..`002`, `CACHE-001`, `CACHE-003`).
   - Asserts active rejection (`400 Bad Request`, `403 Forbidden`, `413 Payload Too Large`, `431 Request Header Fields Too Large`, `501 Not Implemented`) and physical socket closure (`Connection: close`).
   - Reports: Mean, Median ($p50$), Tail Rejection ($p90$), and Max Rejection.
2. **Cohort B: Cross-Vector Comprehensive Latency ($N=19$ Vectors)**:
   - Includes the complete suite of 19 vectors, incorporating benign baseline reference traffic (`BASELINE-001`, `200 OK`, `Connection: keep-alive`) and passive cache compliance (`CACHE-002`, `200 OK`).
   - Reports: Mean, Median ($p50$), 90th Percentile ($p90$), Tail Latency ($p99$), and Max.

### 1.2 Unconditional Distribution Output Across Modes

The fuzzer calculates and outputs $p50$, $p90$, and $p99$ distinctly in all execution modes:
- **Single-Shot Verification ($K=1$, $W=0$)**: Evaluates cross-vector distributions across the 19 heterogeneous invariant test vectors.
- **Repeated Statistical Trials ($K > 1$, $W \ge 0$)**: Evaluates per-vector repeated distributions across $K$ trials with $W$ discarded warm-up runs, computing 95% confidence intervals alongside cross-vector summary metrics.

---

## 2. CLI Usage & Flags

Execute the fuzzer harness via the runner script or directly with `go run`:

```bash
# Single-shot verification mode (K=1, W=0)
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -k 1 -w 0

# Repeated statistical trials mode (e.g. K=1,000 trials, W=50 warmup discard)
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -k 1000 -w 50
```

### Options Reference

| Flag | Script Option | Description | Default |
| :--- | :--- | :--- | :--- |
| `-target` | `-t <host:port>` | Target server host:port (Toron) | `127.0.0.1:8080` |
| `-baseline` | `-b <host:port>` | Optional baseline server for differential comparison | `""` |
| `-trials` | `-k <trials>` | Number of measured repeated trials per test case | `1` |
| `-warmup` | `-w <warmup>` | Number of preliminary discarded warm-up runs | `0` (or `50` for $K>1$) |
| `-json` | `-j <file>` | Path for JSON artifact export | `benchmarks/results/differential_fuzz_report.json` |
| `-md` | `-m <file>` | Path for Markdown artifact export | `benchmarks/results/differential_fuzz_report.md` |

---

## 3. Artifact Schemas

### 3.1 JSON Schema (`differential_fuzz_report.json`)

```json
{
  "timestamp": "2026-09-12T06:50:58Z",
  "target_host": "127.0.0.1:8080",
  "trials_per_test": 1,
  "warmup_runs_per_test": 0,
  "total_tests": 19,
  "passed_tests": 19,
  "failed_tests": 0,
  "security_pass_rate": 100,
  "average_latency_us": 324.35,
  "median_latency_us": 230.75,
  "p90_latency_us": 672.83,
  "p99_latency_us": 1129.75,
  "fail_fast_defense": {
    "count": 18,
    "mean_latency_us": 279.60,
    "median_latency_us": 230.75,
    "p90_latency_us": 574.67,
    "max_latency_us": 672.83
  },
  "comprehensive_suite": {
    "count": 19,
    "mean_latency_us": 324.35,
    "median_latency_us": 230.75,
    "p90_latency_us": 672.83,
    "p99_latency_us": 1129.75,
    "max_latency_us": 1129.75
  }
}
```

### 3.2 Markdown Report Schema (`differential_fuzz_report.md`)

The Markdown report features an executive summary with explicit disaggregation:
- **Fail-Fast Defense Latency ($N=18$ Vectors)**: Mean, Median ($p50$), Tail Latency ($p90$), Max Rejection.
- **Cross-Vector Comprehensive Latency ($N=19$ Vectors)**: Mean, Median ($p50$), 90th Percentile ($p90$), Tail Latency ($p99$), Max Latency.
- **Detailed Invariant Test Results Table**: Itemizes every test case with explicit response status, socket closure confirmation, and latency.

---

## 4. Academic Traceability & Invariant Verification

- **Manuscript Section 5.2 Alignment (REV-02)**: Eliminates ambiguity between $p90$ ($712.08\ \mu\text{s}$) and $p99$ ($2,378.00\ \mu\text{s}$). Ground truth telemetry accurately distinguishes fail-fast rejection behavior from $200\text{ OK}$ payload transfer.
- **Governing Requirement**: [`REQ-118`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-118.md)
- **Architectural Decision**: [`ADR-118`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-118.md)
- **Test Specification**: [`TC-118`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-118.md)
- **Code Review**: [`CR-114`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-114.md)
- **Security Audit**: [`SR-118`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-118.md)
