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
6. **RFC 7234 Shared Cache Session Boundary Isolation (CWE-524, CWE-384)**:
   - Multi-stage sequential request execution evaluating cross-session cache isolation.
   - `CACHE-001`: Web Cache Deception prevention (unauthenticated probe cannot retrieve private cached content; asserts `X-Cache: MISS`).
   - `CACHE-002`: `Set-Cookie` and `Set-Cookie2` header stripping prior to shared cache storage and emission (`X-Cache: HIT` without cookie leakage).
   - `CACHE-003`: `Authorization` refusal (requests with `Authorization` are refused storage unless explicitly marked `Cache-Control: public`).
7. **RFC Conformance Baseline (`BASELINE-001`)**:
   - Standard valid HTTP/1.1 request formatted canonically as `200 OK` in generated Markdown and JSON reports via dynamic RFC status resolution.

### Running the Fuzzer
```bash
# Run against Toron
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080

# Run in Differential Mode comparing Toron against a baseline (e.g. NGINX on :8081)
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -b 127.0.0.1:8081
```

## 📈 3. In-Process HTTP Parser Microbenchmarks (`pkg/httpparser/`)

### Overview
To isolate pure state machine execution latency from operating system TCP network stack overhead (~298 $\mu$s over loopback sockets), the test suite includes in-process Go parser microbenchmarks (`testing.B`) in `pkg/httpparser/parser_test.go`. The harness eliminates allocation noise by using zero-allocation reader resetting (`r.Reset(payload)` inside `for i := 0; i < b.N; i++`) and tracks exact memory overhead via `b.ReportAllocs()`.

### Benchmark Suite
1. **`BenchmarkParseRequest_WhitespaceRejection`**:
   - Evaluates nanosecond fail-fast rejection of RFC 7230 §3.2.4 whitespace violations (space/tab preceding header field colon).
   - *Result*: ~1.13 $\mu$s/op, 15 allocs/op (50% allocation reduction vs baseline).
2. **`BenchmarkParseRequest_MultipleCL`**:
   - Evaluates nanosecond fail-fast rejection of RFC 7230 §3.3.2 / CWE-444 conflicting multiple `Content-Length` headers before payload ingestion.
   - *Result*: ~2.01 $\mu$s/op, 34 allocs/op.
3. **`BenchmarkParseRequest_ValidBaseline`**:
   - Evaluates baseline parsing latency and memory allocation profile of a valid RFC 7230 HTTP/1.1 request for comparative ablation.
   - *Result*: ~1.91 $\mu$s/op, 30 allocs/op.

### Running Parser Microbenchmarks
```bash
go test -bench=BenchmarkParseRequest -benchmem ./pkg/httpparser/...
```

---

## 📈 4. Generated Evaluation Artifacts

Results are automatically saved in `benchmarks/results/`:
- `benchmark_c*.json` & `benchmark_c*.csv`: Latency percentiles and throughput numbers suitable for Gnuplot / Python Matplotlib.
- `concurrency_sweep_summary.csv`: Aggregated latency curve data across concurrency points.
- `differential_fuzz_report.json`: Machine-readable fuzzer pass/fail data with microsecond rejection latencies.
- `differential_fuzz_report.md`: Formatted Markdown table comparing invariant conformance.

