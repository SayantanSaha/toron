# Toron Automated Empirical Evaluation & Differential Security Testbed

This directory contains the automated performance benchmarking harnesses and differential protocol security fuzzer suites designed to generate empirical figures, tables, and artifact evaluation data for academic peer review (USENIX Security, ACM CCS, EuroSys, SoCC, ICSE).

---

## 📁 Directory Structure

```text
benchmarks/
├── README.md                     # Comprehensive artifact evaluation documentation
├── run_all.sh                    # Master automated runner (end-to-end evaluation)
├── results/                      # Auto-generated JSON, CSV, and Markdown artifacts
├── wrk2/                         # Throughput & tail latency evaluation
│   ├── run_wrk2.sh               # Shell harness with wrk2 / wrk / Go fallback
│   ├── loadgen.go                # High-precision Go constant-rate load generator
│   └── scripts/
│       ├── pipeline.lua          # Pipelined HTTP request script
│       └── post_payload.lua      # JSON POST payload ingestion script
└── fuzzer/                       # Differential protocol security evaluation
    ├── diff_fuzzer.go            # Protocol invariant test engine & raw socket fuzzer
    ├── diff_fuzzer_test.go       # Unit tests for socket termination verification & latency isolation
    └── run_fuzzer.sh             # Differential fuzzer execution script
```

---

## 🚀 Quick Start (Artifact Evaluation)

### 1. One-Click End-to-End Evaluation
To compile Toron, launch it in background, execute throughput latency benchmarks, run the differential security fuzzer, and shut down cleanly:

```bash
bash benchmarks/run_all.sh --auto-start
```

All evaluation artifacts will be written to `benchmarks/results/`.

---

## 📊 1. Performance & Tail Latency Benchmarking (`benchmarks/wrk2/`)

### Overview
Measures requests/second, data throughput (MB/s), and fine-grained latency distribution (p50, p75, p90, p95, p99, p99.9) under constant-rate load.

### Execution Options
```bash
# Standard benchmark (default: 100 conns, 10,000 req/sec, 10s duration)
bash benchmarks/wrk2/run_wrk2.sh -u http://127.0.0.1:8080/health -c 100 -d 10s -r 10000

# Automated Concurrency Scaling Sweep (50, 100, 250, 500, 1000 connections)
bash benchmarks/wrk2/run_wrk2.sh -u http://127.0.0.1:8080/health --sweep

# POST ingestion benchmark with custom Lua script
bash benchmarks/wrk2/run_wrk2.sh -u http://127.0.0.1:8080/health -s benchmarks/wrk2/scripts/post_payload.lua
```

### Zero-Dependency Fallback
If `wrk2` or `wrk` is not installed on the evaluator's system, the harness seamlessly runs `loadgen.go` (pure Go standard library). It implements token-bucket rate pacing and microsecond monotonic latency tracking to reproduce coordinated-omission-free tail latency metrics without requiring C compiler toolchains.

---

## 🛡️ 2. Differential Security Protocol Fuzzer (`benchmarks/fuzzer/`)

### Overview
Transmits raw TCP byte streams containing deliberate RFC protocol violations, desynchronization sequences, control character injections, and parameter collisions. Verifies that Toron enforces strict fail-fast rejection (`400 Bad Request` or `501 Not Implemented`) and immediate TCP teardown where required.

### Covered Threat Invariants:
1. **HTTP Request Smuggling (CWE-444 / RFC 7230 §3.3.3)**:
   - Conflicting `Content-Length` and `Transfer-Encoding: chunked`.
   - Multiple divergent `Content-Length` headers.
   - Obfuscated `Transfer-Encoding` with tab characters (`\t`).
   - Invalid hex chunk extensions.
2. **Header Syntax Invariants (RFC 7230 §3.2.4)**:
   - Whitespace (space or tab) immediately preceding colon in field name.
   - Obsolete line folding (`obs-fold`).
3. **Control Characters & Log Forgery (CWE-117)**:
   - Null bytes (`0x00`) in URI paths.
   - Terminal control sequences (`0x07` Bell, `0x1B` ANSI escape) in headers and query parameters.
4. **Path Traversal & Normalization (CWE-22 / RFC 3986)**:
   - Strict active defense oracle: asserts `400 Bad Request` or `403 Forbidden` (`404 Not Found` rejected as false-positive).
   - Raw dot-dot (`..`) segments escaping root boundaries targeting canary file (`canary_traversal.txt`).
   - Uppercase percent-encoded (`%2E%2E`) sequences.
   - Double percent-encoded (`%252e%252e`) sequences.
5. **Heap Allocation Bounding (CWE-400)**:
   - Oversized request headers (>8KB).
   - Oversized single query parameters (>2KB).

### Running the Fuzzer
```bash
# Run against Toron
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080

# Run in Differential Mode comparing Toron against a baseline (e.g. NGINX on :8081)
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -b 127.0.0.1:8081
```

---

## 📈 3. Generated Evaluation Artifacts

Results are automatically saved in `benchmarks/results/`:
- `benchmark_c*.json` & `benchmark_c*.csv`: Latency percentiles and throughput numbers suitable for Gnuplot / Python Matplotlib.
- `concurrency_sweep_summary.csv`: Aggregated latency curve data across concurrency points.
- `differential_fuzz_report.json`: Machine-readable fuzzer pass/fail data with microsecond rejection latencies.
- `differential_fuzz_report.md`: Formatted Markdown table comparing invariant conformance.
