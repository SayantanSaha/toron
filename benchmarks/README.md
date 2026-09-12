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
├── fuzzer/                       # Differential protocol security evaluation
│   ├── diff_fuzzer.go            # Protocol invariant test engine & raw socket fuzzer
│   ├── diff_fuzzer_test.go       # Unit tests for socket termination verification & latency isolation
│   └── run_fuzzer.sh             # Differential fuzzer execution script
└── multihop/                     # Heterogeneous multi-hop backend testbed (BMK-03)
    ├── backends/                 # Real-world backend runtimes (Node.js, Python, Go)
    ├── docker-compose.multihop.yml # Multi-container cluster orchestration
    ├── routes.multihop.yaml      # Multi-hop upstream gateway routing table
    ├── config.multihop.yaml      # Edge gateway configuration
    ├── runner.go                 # Two-stage desync evaluation engine (pure Go standard library)
    ├── multihop_test.go          # Standalone in-process test suite
    └── run_multihop.sh           # Testbed execution script (standalone & Docker)
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

# High-Concurrency Saturation & Adversarial Stress Testing (BMK-04, 5,000+ RPS, 10% attack injection)
bash benchmarks/wrk2/run_saturation_stress.sh -r 5000 -c 50 -d 10s -a 0.10
```

### Zero-Dependency Fallback
If `wrk2` or `wrk` is not installed on the evaluator's system, the harness seamlessly runs `loadgen.go` (pure Go standard library). It implements token-bucket rate pacing, decoupled dual-stream telemetry (benign service vs. fast-fail attack rejection), and microsecond monotonic latency tracking to reproduce coordinated-omission-free tail latency metrics without requiring C compiler toolchains.

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
   - Enforces fail-fast transport socket teardown (`Connection: close`) and physical TCP closure upon rejection (`REQ-107`, `ADR-116`).
   - `TRAVERSAL-001`: Raw dot-dot (`..`) segments escaping static route prefix (`GET /internal/dashboard/../../canary_traversal.txt HTTP/1.1`) -> **`403 Forbidden` / `Connection: close` (PASS)**.
   - `TRAVERSAL-002`: Uppercase percent-encoded (`%2E%2E`) sequences (`GET /internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt HTTP/1.1`) -> **`403 Forbidden` / `Connection: close` (PASS)**.
   - `TRAVERSAL-003`: Double percent-encoded (`%252e%252e`) sequences (`GET /internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt HTTP/1.1`) -> **`403 Forbidden` / `Connection: close` (PASS)**.
   - Verifies zero canary secret leakage (`canary_traversal.txt`) across all boundary escape attempts.
   - Mitigates defensive masking where premature router path canonicalization previously diverted traversal probes to `404 Not Found` with keep-alive connections.
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

### 🛡️ Empirical Invariant Conformance & Active Defense Results (100.0% Pass Rate)

Following the implementation of the **Layered Route-Aware Path Traversal Defense Architecture** ([`REQ-116`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-116.md), [`ADR-116`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-116.md), [`TASK-139`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-139.md)), Toron achieves a **100.0% invariant pass rate (19/19 tests passed, 0 failed)** across all 10 evaluated security categories:

| Security Category | Total Tests | Passed | Failed | Pass Rate | Target Invariant & Standard |
| :--- | :---: | :---: | :---: | :---: | :--- |
| **Path Canonicalization (CWE-22)** | **3** | **3** | **0** | **100.0%** | **Active 403 Forbidden & Socket Teardown on Prefix Escapes (REQ-116 / ADR-116)** |
| Request Smuggling (CL.CL) | 1 | 1 | 0 | 100.0% | RFC 7230 §3.3.3 Multiple Content-Length Rejection (CWE-444) |
| Request Smuggling (Obfuscation) | 1 | 1 | 0 | 100.0% | RFC 7230 §3.3.3 Obfuscated Transfer-Encoding Rejection (CWE-444) |
| Request Smuggling (Chunked) | 1 | 1 | 0 | 100.0% | RFC 7230 §4.1.1 Chunk Hex Size Syntax Integrity (CWE-444) |
| Request Smuggling (CL.TE) | 1 | 1 | 0 | 100.0% | RFC 7230 §3.3.3 Conflicting CL and TE Rejection (CWE-444) |
| Header Syntax Invariants | 3 | 3 | 0 | 100.0% | RFC 7230 §3.2.4 Leading Whitespace & Line Folding (CWE-436) |
| Control Character Guards | 3 | 3 | 0 | 100.0% | CWE-117 Null Byte & Terminal Control Sequence Filtering |
| Resource Bounding (CWE-400) | 2 | 2 | 0 | 100.0% | CWE-400 Header (>8KB) & Query Parameter (>2KB) Limits |
| Cache Session Boundary | 3 | 3 | 0 | 100.0% | RFC 7234 Web Cache Deception & Cookie Stripping (CWE-524) |
| RFC Conformance Baseline | 1 | 1 | 0 | 100.0% | RFC 7230 Canonical 200 OK Baseline Evaluation |
| **Total Conformance** | **19** | **19** | **0** | **100.0%** | **Full Protocol Invariant & Active Security Conformance** |

#### Path Traversal Active Defense Verification Matrix (`TRAVERSAL-001` .. `TRAVERSAL-003`)

The differential security fuzzer enforces strict active defense oracles, asserting that path traversal attempts receive active security rejections (`403 Forbidden`) with mandatory transport socket teardown (`Connection: close`), completely rejecting passive `404 Not Found` masking as a failure:

| Test ID | Attack / Invariant Vector | CWE | Expected Status | Actual Status | Transport Teardown (`conn:closed`) | Result | Defense Mechanism |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :--- |
| `TRAVERSAL-001` | Raw Dot-Dot Path Traversal Sequence (`/../../canary_traversal.txt`) | CWE-22 | `400/403` | `403` | `Closed` | ✅ PASS | Layer 1 WAF Raw Wire URI Inspection + Layer 2 Router Prefix Escape Guard |
| `TRAVERSAL-002` | Uppercase Percent-Encoded Traversal (`/%2E%2E/%2E%2E/canary_traversal.txt`) | CWE-22 | `400/403` | `403` | `Closed` | ✅ PASS | Layer 1 WAF Case-Insensitive Wire Regex + Layer 2 Router Unescape Guard |
| `TRAVERSAL-003` | Double Percent-Encoded Traversal (`/%252e%252e/%252e%252e/canary_traversal.txt`) | CWE-22 | `400/403` | `403` | `Closed` | ✅ PASS | Layer 1 Dual-Path Evaluation + Layer 2 Iterative Unescaping Fixpoint Guard |

- **Zero Canary Secret Leakage**: The canary secret file (`canary_traversal.txt`) is strictly contained; zero response bytes ever leak canary content.
- **Fail-Fast Rejection Latency**: Mean rejection latency remains $< 300\ \mu\text{s}$ (Median $< 70\ \mu\text{s}$) with zero heap allocations on common benign requests.
- **Standalone Router Protection**: When WAF middleware is disabled or omitted, Layer 2 Router Static Prefix Escape Guard independently catches all prefix escape attempts and enforces `403 Forbidden` with socket closure.

### Running the Fuzzer
```bash
# Rapid CI Invariant Verification (single probe per test vector)
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080

# Empirical Statistical Evaluation (K=1,000 repeated trials, W=50 discarded warm-up runs)
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -k 1000 -w 50

# Run in Differential Mode comparing Toron against a baseline (e.g. NGINX on :8081)
bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -b 127.0.0.1:8081 -k 1000 -w 50
```

### Statistical Evaluation Methodology & Equation 7 Alignment
To ensure peer-reviewed scientific reproducibility (`BMK-01`, `BMK-02`):
1. **Equation 7 Timing Isolation**: The fuzzer strictly adheres to Equation 7 ($T_{\text{rejection}} = t_{\text{status\_line\_read}} - t_{\text{socket\_write\_start}}$). Operating system TCP loopback connection establishment (`net.DialTimeout`) and multi-stage cache setup requests (`sendAndDrain`) execute strictly outside the timing window. The clock starts immediately before the probe bytes are written to the wire (`conn.Write`), capturing pure proxy state machine rejection latency.
2. **Warm-up Phase ($W=50$)**: Configured via `-w <runs>` / `-warmup <runs>`, the fuzzer performs preliminary discarded iterations to warm operating system socket tables and JIT/branch predictors before recording empirical measurements.
3. **Repeated Statistical Trials ($K \ge 1,000$)**: Configured via `-k <trials>` / `-trials <trials>`, each vector is probed $K$ times to derive comprehensive statistical dispersion metrics:
   - Sample Mean ($\bar{x}$)
   - Sample Standard Deviation ($s = \sqrt{\frac{1}{K-1} \sum_{i=1}^K (x_i - \bar{x})^2}$, applying Bessel's correction)
   - Median ($p50$) and Tail Percentiles ($p90, p99, p99.9$) via deterministic nearest-rank indexing ($I_p = \lceil p \cdot K \rceil - 1$)
   - 95% Confidence Interval ($\left[ \bar{x} - 1.96 \cdot \frac{s}{\sqrt{K}}, \; \bar{x} + 1.96 \cdot \frac{s}{\sqrt{K}} \right]$)

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

## 🌐 4. Heterogeneous Multi-Hop Backend Origin Testbed (`benchmarks/multihop/`)

### Overview
Directly refuting the "multi-hop blindspot" critique (`AER-002` Issue 6, `AR-002` Alternative Explanation 3, `BMK-03`), this testbed evaluates Toron fronting three live, distinct backend HTTP runtime parsing engines:
1. **Node.js 20 LTS**: C-based `llhttp` parser engine (`/node`).
2. **Python 3.11**: ASGI `uvicorn` / `h11` parser engine (`/python`).
3. **Go 1.24**: Standard library `net/http` parser engine (`/go`).

### Two-Stage Desynchronization Evaluation Protocol ($r_{\text{poison}} \,\|\, r_{\text{benign}}$)
For each vector across all three runtimes ($10 \times 3 = 30$ scenarios):
1. **Stage 1 ($r_{\text{poison}}$)**: Transmits crafted smuggling vectors (H2.TE, H2.CL duplicate/mismatch, H1-CL.TE, H1-TE.CL whitespace obfuscation, pipelining buffer eviction, CRLF header injection, pseudo-header isolation, and benign baselines).
2. **Stage 2 ($r_{\text{benign}}$)**: Immediately issues a benign canary request (`GET /canary`) over the connection session to prove that:
   - Edge security rejections physically tear down transport sockets without leaking residual bytes into backend pools.
   - Forwarded traffic maintains 100% connection pool integrity without response poisoning or desynchronization.

### Running the Multi-Hop Testbed
```bash
# Standalone in-process mode (zero-dependency, pure Go standard library for CI):
go test -v -race -count=1 ./benchmarks/multihop/...

# Shell harness execution:
bash benchmarks/multihop/run_multihop.sh --standalone

# Live multi-container Docker Compose cluster:
bash benchmarks/multihop/run_multihop.sh --docker
```

---

## 📈 5. Generated Evaluation Artifacts

Results are automatically saved in `benchmarks/results/`:
- `benchmark_c*.json` & `benchmark_c*.csv`: Latency percentiles and throughput numbers suitable for Gnuplot / Python Matplotlib.
- `concurrency_sweep_summary.csv`: Aggregated latency curve data across concurrency points.
- `differential_fuzz_report.json`: Machine-readable fuzzer pass/fail data with microsecond rejection latencies.
- `differential_fuzz_report.md`: Formatted Markdown table comparing invariant conformance.
- `multihop_report.json`: Full telemetry from the heterogeneous multi-hop testbed (30 scenarios, 3 runtimes).
- `multihop_report.md`: Publication-grade cross-runtime evaluation matrix.
- `saturation_stress_report.json`: High-concurrency saturation stress telemetry (5,000+ RPS with 10% adversarial injection).
- `saturation_stress_report.md`: Publication-grade dual-stream tail latency and active defense verification report.

