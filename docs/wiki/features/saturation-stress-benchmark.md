---
title: High-Concurrency Saturation Stress Benchmark & Status Classification (BMK-04, HARN-01, REQ-130)
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-09-12
updated: 2026-09-16

depends_on:
  - REQ-114
  - REQ-117
  - REQ-129
  - REQ-130
  - TASK-137
  - TASK-140
  - TASK-152
  - TASK-153
  - ADR-114
  - ADR-117
  - ADR-129
  - ADR-130
  - TC-114
  - TC-117
  - TC-129
  - TC-130
  - CR-113
  - CR-126
  - SR-117
  - SR-130

derived_from:
  - REQ-114
  - REQ-117
  - REQ-129
  - REQ-130
  - TASK-140
  - TASK-153
  - ADR-117
  - ADR-130

documents:
  - SATURATION-STRESS-BENCHMARK-HARNESS
  - MULTI-TIER-GC-TELEMETRY

related_to:
  - ../configuration.md
  - ./benchmarking.md
  - ./docker-compare-benchmark.md
  - ./waf.md
  - ./path-traversal-defense.md
  - ../index.md
  - ../release-notes.md
---

# High-Concurrency Saturation Stress Benchmark & Status Classification (BMK-04, HARN-01)

## 1. Overview & Problem Context

Toron includes a native high-concurrency load generation and saturation stress testing harness implemented in pure Go (`benchmarks/wrk2/loadgen.go`). Inspired by the constant-throughput architecture of `wrk2`, the harness evaluates gateway behavior under heavy saturation ($5{,}000+\text{ RPS}$) while interleaving malicious protocol vectors with standard benign traffic.

```
                  ┌────────────────────────────────────────────────────────┐
                  │       Native Benchmark Load Generator (loadgen.go)     │
                  └───────────────────────────┬────────────────────────────┘
                                              │
                    ┌─────────────────────────┴─────────────────────────┐
                    ▼                                                   ▼
       ┌────────────────────────┐                          ┌────────────────────────┐
       │     Benign Stream      │                          │   Adversarial Stream   │
       │ (90% Background Load)  │                          │  (10% Injection Ratio) │
       │  e.g. GET /health      │                          │  Vectors ADV-01..08    │
       └────────────┬───────────┘                          └────────────┬───────────┘
                    │                                                   │
                    └─────────────────────────┬─────────────────────────┘
                                              │ Interleaved TCP Traffic
                                              ▼
                        ┌───────────────────────────────────────────┐
                        │        Toron High-Performance Gateway     │
                        │    (Reactor, Parser, Router, WAF)         │
                        └─────────────────────┬─────────────────────┘
                                              │
                                              ▼
       ┌────────────────────────────────────────────────────────────────────────┐
       │              Disaggregated Four-Tier Classification Oracle             │
       ├──────────────────┬──────────────────┬──────────────────┬───────────────┤
       │ Tier 1: Defense  │ Tier 2: RouteMiss│ Tier 3: Bypass   │Tier 4: Anomaly│
       │ 400/403/413/431  │  404 Not Found   │  200 OK (Bypass) │  500/502/5xx  │
       └──────────────────┴──────────────────┴──────────────────┴───────────────┘
```

### 1.1 Remediation of Circular Defense Scoring (`HARN-01` / `REQ-117`)

In benchmark and security audits (formalized in `REQ-117` and `ADR-117`), engineers identified a circular scoring anomaly in earlier versions of `loadgen.go`:
- **The Circular Catch-All Else**: The worker loop contained an `else { attackRejected.Add(1) }` branch that indiscriminately scored any non-`200 OK` response as active security defense.
- **Defensive Masking of Route Misses**: During high-concurrency saturation, **349 probes** targeting `ADV-06` (Path Traversal, `GET /../../canary_traversal.txt`) returned `404 Not Found` because the router stripped dot-dot sequences before route matching. Due to the catch-all `else`, these 349 route misses were scored as active defense (`attackRejected`), inflating reported defense scores to 100.0% and obscuring the underlying routing defect.
- **Omission of Valid Defense Codes**: RFC 6585 status `431 Request Header Fields Too Large` was omitted from explicit checks and only captured through accidental fallback.
- **Masking Server Crashes**: Unhandled server errors (`500 Internal Server Error`) or transport faults were similarly routed into active defense.

Under `HARN-01`, the circular catch-all was permanently removed and replaced with a strict, mutually exclusive **Four-Tier Status Classification Taxonomy**.

### 1.2 Remediation of the 5-Second Evaluation Blindspot & Multi-Tier Duration Taxonomy (`REQ-130` / `TASK-153` / `ADR-130`)

In subsequent empirical audits formalized in `REQ-130` and `ADR-130`, performance engineers identified three critical scientific and empirical limitations in default 5-to-10-second benchmark runs:

1. **Transient Startup Bias**: During the initial 1 to 3 seconds of execution, TCP socket connection pooling (`net.Conn` pools, epoll reactor event loops), CPU frequency scaling (DVFS governor priming), and Go runtime netpoller priming dominate the measurement window, skewing tail latency measurements ($p99, p99.9$).
2. **Garbage Collector Masking**: Go's concurrent mark-and-sweep garbage collector (GC) triggers only when heap allocations reach $2 \times \text{GOGC}$. In a short 5-second burst with zero-allocation routing, total allocations frequently remain beneath the initial trigger threshold ($0$ to $2$ cycles), masking Stop-The-World (STW) pause times, mark-assist CPU overhead, and heap expansion dynamics.
3. **Invisible Memory Leaks & Heap Drift**: Memory bloat, buffer pool degradation (`sync.Pool` retaining oversized slabs), goroutine leaks, and socket descriptor leaks (`EMFILE`, [CWE-775](https://cwe.mitre.org/data/definitions/775.html)) cannot be detected during a transient 5-second window, preventing empirical validation of constant $O(1) \le 32\text{KB}$ memory boundedness (`REQ-129` / `ADR-129`).

To resolve these empirical blindspots, `REQ-130` and `TASK-153` establish a standardized **Multi-Tier Duration Taxonomy**:

| Tier Name | CLI Identifier | Duration | Primary Empirical Objective |
| :--- | :---: | :---: | :--- |
| **Quick Smoke** | `quick` | 5 seconds | Rapid pre-merge continuous integration regression testing and smoke verification ($< 60\text{s}$ suite runtime). |
| **Steady-State / GC Observation** | `medium` (or `steady`) | 60 seconds (1 min) | Deep observation of Go runtime GC cycles, mark/pause clock times, and steady-state tail latency HDR histograms ($p95, p99, p99.9$). |
| **Long-Term Soak & Memory Stability** | `soak` | 300 seconds (5 min) | Extended soak testing validating $O(1)$ memory boundedness, zero heap drift, connection pool longevity, and proxy resilience under 1,500,000+ requests. |
| **All Tiers Sweep** | `all` | 5s + 60s + 300s | Comprehensive sequential evaluation sweep across all three duration tiers for publication-grade systems research. |

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│                            EMPIRICAL SYSTEM BEHAVIOR ACROSS DURATIONS                            │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│  [5s: Quick Smoke]       Transient startup phase; connection pool priming; GC cycles: 0 to 2     │
│  [60s: Steady-State]     Connection pools stabilized; 50-150 GC cycles; HDR tail latencies       │
│  [300s: Long-Term Soak]  1.5M+ requests; proves O(1) bound (slope <= 1.0 MB/min); leak immunity  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Decoupled Dual-Stream Load Generation Architecture

The load generator coordinates concurrent worker goroutines paced by Poisson-distributed inter-arrival intervals to eliminate coordinated omission. Traffic is split into two concurrent streams:

1. **Benign Stream ($1.0 - \text{attackRatio}$)**: Legitimate HTTP requests (typically `GET /health` or API endpoints) continuously measure baseline throughput, error rates, and latency percentiles ($p50, p90, p99$) under stress.
2. **Adversarial Stream ($\text{attackRatio}$)**: Malformed protocol probes randomly selected from the 8-vector attack catalog test parser and WAF invariant enforcement in real time.

```
Worker Goroutine (Paced Interval)
       │
       ├─ Rand < AttackRatio ──► Select Vector (ADV-01..08) ──► executeAttackProbe() ──► Classify Status Code
       │
       └─ Otherwise ───────────► Generate Benign Request ──────► executeBenignProbe() ──► Record Latency / Status
```

---

## 3. Four-Tier Status Classification Taxonomy

To ensure scientific reproducibility and conformance with RFC specifications and MITRE CWE taxonomies, `benchmarks/wrk2/loadgen.go` enforces a strict, four-tier classification model:

| Architectural Tier | Response Status Codes | Counter Variable | Invariant Semantics | Overall Verdict Impact |
| :--- | :--- | :--- | :--- | :---: |
| **Tier 1: Active Defense** | `400 Bad Request`<br>`403 Forbidden`<br>`413 Payload Too Large`<br>`431 Request Header Fields Too Large`<br>`501 Not Implemented`<br>Transport Socket Reset | `attackRejected`<br>`stats.rejected` | Protocol parser, router, or WAF actively rejected violation and tore down socket. | **Required for PASS** |
| **Tier 2: Route Miss** | `404 Not Found` | `attackRouteMiss`<br>`stats.routeMiss` | Request did not match any registered route; reached passive fallback. Excluded from defense score. | **Forces FAIL** |
| **Tier 3: Attack Bypass** | `200 OK`<br>(Unexpected 2xx/3xx) | `attackBypassed`<br>`stats.bypassed` | Malicious probe bypassed security filters and was accepted by server. | **Forces FAIL** |
| **Tier 4: Unhandled Anomaly** | `500 Internal Server Error`<br>`502 Bad Gateway`<br>`503 Service Unavailable`<br>Any other unexpected code | `attackUnhandled`<br>`stats.unhandled` | Server panic, crash, transport error, or unhandled internal fault. | **Forces FAIL** |

### 3.1 Hot-Path Classification Logic

The classification executes in $< 5\text{ ns}$ with zero heap allocations on the worker hot path:

```go
// Direct integer evaluation in benchmarks/wrk2/loadgen.go
switch code {
case http.StatusBadRequest,
     http.StatusForbidden,
     http.StatusRequestEntityTooLarge,
     http.StatusRequestHeaderFieldsTooLarge,
     http.StatusNotImplemented:
    attackRejected.Add(1)
    stats.rejected.Add(1)

case http.StatusNotFound:
    attackRouteMiss.Add(1)
    stats.routeMiss.Add(1)

case http.StatusOK:
    attackBypassed.Add(1)
    stats.bypassed.Add(1)

default:
    attackUnhandled.Add(1)
    stats.unhandled.Add(1)
}
```

> [!IMPORTANT]
> If a connection is terminated by the server (physical socket teardown / RST) during probe transmission, `executeAttackProbe` returns `400 Bad Request`, correctly attributing transport-level active defense to Tier 1.

---

## 4. Adversarial Attack Catalog (Vectors `ADV-01` to `ADV-08`)

The load generator exercises eight representative protocol-level attack vectors covering transport, parsing, routing, and header invariants:

| Vector ID | Attack Name | Target CWE / RFC | Wire Payload Characteristics | Expected Defense Status |
| :--- | :--- | :--- | :--- | :---: |
| **`ADV-01`** | HTTP Request Smuggling (CL-TE) | CWE-444 / RFC 7230 §3.3.3 | Conflicting `Content-Length: 5` and `Transfer-Encoding: chunked` | `400 Bad Request` |
| **`ADV-02`** | HTTP Request Smuggling (TE-CL) | CWE-444 / RFC 7230 §3.3.3 | Conflicting `Transfer-Encoding: chunked` and `Content-Length: 6` | `400 Bad Request` |
| **`ADV-03`** | Null Byte Injection in URI | CWE-20 / RFC 7230 §3.1.1 | Non-printable null byte: `GET /health\x00evil HTTP/1.1` | `400 Bad Request` |
| **`ADV-04`** | Line Folding / Obsolete Header | RFC 7230 §3.2.4 | Disallowed header line folding: `X-Fold: hello\r\n world` | `400 Bad Request` |
| **`ADV-05`** | Space Before Colon in Header | RFC 7230 §3.2 | Illegal whitespace preceding colon: `X-Bad-Header : evil` | `400 Bad Request` |
| **`ADV-06`** | Path Traversal Directory Escape | CWE-22 / RFC 3986 §3.3 | Directory traversal: `GET /../../canary_traversal.txt HTTP/1.1` | `403 Forbidden` (`Connection: close`) |
| **`ADV-07`** | Oversized Header Block | CWE-400 / RFC 6585 §5 | Single header line exceeding 8 KB buffer limit | `431 Request Header Fields Too Large` |
| **`ADV-08`** | Oversized Request Entity | CWE-400 / RFC 7231 §6.5.11 | Content length declaring 100 MB body exceeding gateway limit | `413 Payload Too Large` |

---

## 5. Table 6 Metric Definitions & Publication Formatting

To provide transparent, publication-grade reporting for Paper 1 and Paper 2, `GenerateMarkdownReport` outputs Section 4 with full 9-column disaggregated telemetry aligning with manuscript Table 6:

```markdown
## 4. Concurrent Adversarial Invariant Breakdown (Table 6 Alignment)

| Vector ID | Attack Name | Category | Probes Sent | Active Defense (4xx/501) | Route Miss (404) | Bypass Count (200) | Unhandled | Active Defense Rate |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| `ADV-01` | HTTP Request Smuggling (CL-TE) | Protocol Smuggling (CWE-444) | 305 | 305 | 0 | 0 | 0 | **100.0%** |
| `ADV-02` | HTTP Request Smuggling (TE-CL) | Protocol Smuggling (CWE-444) | 312 | 312 | 0 | 0 | 0 | **100.0%** |
| `ADV-03` | Null Byte Injection in URI | Ingress Sanitization (CWE-20) | 298 | 298 | 0 | 0 | 0 | **100.0%** |
| `ADV-04` | Line Folding / Obsolete Line (RFC 7230) | Protocol Smuggling (CWE-444) | 301 | 301 | 0 | 0 | 0 | **100.0%** |
| `ADV-05` | Space Before Colon in Header (RFC 7230) | Header Grammar (RFC 7230) | 308 | 308 | 0 | 0 | 0 | **100.0%** |
| `ADV-06` | Path Traversal Directory Escape | Path Traversal Defense (CWE-22) | 349 | 349 | 0 | 0 | 0 | **100.0%** |
| `ADV-07` | Oversized Header Block (>8KB) | Resource Exhaustion (CWE-400) | 263 | 263 | 0 | 0 | 0 | **100.0%** |
| `ADV-08` | Oversized Request Entity (>Limit) | Resource Exhaustion (CWE-400) | 295 | 295 | 0 | 0 | 0 | **100.0%** |
```

### 5.1 Telemetry Formulas & Partition Invariant

The load generator maintains a strict partition invariant across all executed attack probes:

$$\text{Total Attack Probes} = \text{Active Defense} + \text{Route Miss} + \text{Attack Bypass} + \text{Unhandled Anomaly}$$

Key evaluation formulas:

$$\text{Active Defense Rate (\%)} = \frac{\text{Active Defense Requests}}{\text{Total Adversarial Requests}} \times 100$$

$$\text{Route Miss Rate (\%)} = \frac{\text{Route Miss Requests}}{\text{Total Adversarial Requests}} \times 100$$

### 5.2 Strict Overall Evaluation Verdict

The overall benchmark verdict strictly evaluates all five operational invariants:

```go
overallVerdict := "PASS"
if !zeroStarvation ||
   activeDefenseRate < 100.0 ||
   totBypassed > 0 ||
   totRouteMiss > 0 ||
   totUnhandled > 0 {
    overallVerdict = "FAIL"
}
```

- **Zero Starvation Verified**: Legitimate benign background traffic achieves $\ge 99.9\%$ success with $p99 < 10\text{ ms}$ under peak saturation.
- **Active Defense Rate**: Exactly $100.0\%$ of malformed probes are intercepted.
- **Zero Route Misses**: Unintercepted probes falling through to $404$ strictly fail the benchmark.
- **Zero Bypasses**: Malformed probes returning $200\text{ OK}$ strictly fail the benchmark.
- **Zero Unhandled Anomalies**: Server $5\text{xx}$ errors or crashes strictly fail the benchmark.

---

## 6. Runtime Go Garbage Collection Telemetry Capture & Analysis (`GODEBUG=gctrace=1`)

To address the garbage collection masking blindspot formalized in `REQ-130` and `ADR-130`, the saturation stress harness captures and analyzes Go runtime memory management dynamics during load generation.

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│                         RUNTIME GC TELEMETRY CAPTURE & PARSING PIPELINE                          │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│  Toron Gateway (PID)  ──► stderr ──► benchmarks/results/server_gc_trace.log                      │
│                                           │                                                      │
│                                           ▼                                                      │
│                           gcparser.ParseReader (Zero-Allocation Scanner)                         │
│                                           │                                                      │
│                     ┌─────────────────────┴─────────────────────┐                                │
│                     ▼                                           ▼                                │
│        GCPauseStatistics (STW Pauses)              GCHeapStatistics (OLS Slope)                  │
│        • Min, Mean, P50, P95, P99, Max             • Initial, Final, Peak Live Heap              │
│        • Total STW Pause, Mark Durations           • OLS Growth Slope (<= 1.0 MB/min Bounded)    │
│                     │                                           │                                │
│                     └─────────────────────┬─────────────────────┘                                │
│                                           ▼                                                      │
│                 saturation_stress_report.json  &  saturation_stress_report.md                    │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 6.1 `GODEBUG=gctrace=1` Process Environment Injection & Log Segregation

When the background Toron gateway is launched via `--auto-start` in `benchmarks/wrk2/run_saturation_stress.sh`, it is launched with `GODEBUG=gctrace=1`:

```bash
GODEBUG=gctrace=1 "${RESULTS_DIR}/toron_stress" \
    -config "${ROOT_DIR}/config.yaml" \
    -routes "${ROOT_DIR}/routes.yaml" \
    > "${SERVER_LOG}" 2> "${GC_LOG}" &
```

- **Clean Stderr Segregation**: Standard output (HTTP server lifecycle and routing logs) is directed to `server_stress.log`, while standard error (Go runtime GC traces) is cleanly segregated into `benchmarks/results/server_gc_trace.log`.
- **Zero Production Modification**: Application code in `pkg/server` and `pkg/proxy` remains 100% untouched.
- **Session Manifest Archiving (REQ-119)**: When historical retention is enabled, `server_gc_trace.log` is preserved in `benchmarks/results/history/<timestamp>/` and indexed in `manifest.json`.

### 6.2 Zero-Dependency Fast GC Parser Engine (`benchmarks/telemetry/gcparser`)

The parser engine implemented in `benchmarks/telemetry/gcparser` extracts structured metrics from raw Go runtime `gctrace` output without third-party dependencies:

1. **Canonical Go GC Trace Grammar**:
   ```text
   gc 42 @12.345s 2%: 0.045+1.23+0.015 ms clock, 0.36+0.45/1.10/2.30+0.12 ms cpu, 14->16->8 MB, 18 MB goal, 8 MB stacks, 0 MB globals, 8 P
   ```
2. **Sub-Microsecond Fast Tokenizer (`fastParseLine`)**:
   - Decomposes lines using byte-level index searching (`strings.IndexByte`, `strings.Index`) and string slicing, completely bypassing regular expressions for canonical lines.
   - Operates at $> 200,000\text{ lines/sec}$ ($< 50\text{ ms}$ for 10,000 lines) with zero memory allocations in the hot path.
3. **Multi-Version Grammar Fallback (`gcRegex`)**:
   - Employs a linear-time RE2 regular expression fallback accommodating formatting variations across Go 1.20, Go 1.22, and Go 1.24+ (e.g. optional `stacks`/`globals` tokens, processor count `P`, fractional milliseconds).
4. **Instant Noise Filtering**:
   - Any line not beginning with `"gc "` is discarded immediately before scanning, preventing application log spam or panic traces from corrupting GC metrics.

### 6.3 Stop-The-World (STW) Pause Distribution Metrics

Go's concurrent garbage collector executes two brief Stop-The-World (STW) pause phases per cycle:
- $t_{\text{stw1}}$: Sweep termination pause.
- $t_{\text{stw2}}$: Mark termination pause.
- $\text{Total STW Pause per Cycle}: T_{\text{pause}} = t_{\text{stw1}} + t_{\text{stw2}}$.

In `benchmarks/telemetry/gcparser/stats.go`, `ComputeStatistics` aggregates these pauses across all cycles using the sorted-index rank formula $\text{Rank}(P) = \lceil P \times N \rceil - 1$:
- **Min / Max STW Pause**: Boundary pause durations across the entire test run.
- **Mean STW Pause**: $\frac{1}{N} \sum_{i=1}^N T_{\text{pause}, i}$.
- **P50 / P95 / P99 STW Pauses**: Median and high-percentile tail pause distributions.
- **Total STW Pause**: Cumulative time spent in Stop-The-World pauses.
- **Mark Phase Durations**: Mean and Max concurrent mark clock times ($t_{\text{mark}}$).
- **GC Cycle Frequency**: Cycles per second $= N / \text{durationSec}$.
- **Total Heap Reclaimed**: $\sum_{i=1}^N (H_{\text{sweep}, i} - H_{\text{live}, i})$.

### 6.4 Ordinary Least Squares (OLS) Heap Growth Slope & Constant $O(1)$ Boundedness Verification

Evaluating heap growth by subtracting final heap from initial heap ($H_{\text{final}} - H_{\text{initial}}$) introduces severe distortion due to GC sawtooth behavior. The parser applies Ordinary Least Squares (OLS) linear regression across all post-GC live heap points $(t_i, H_{\text{live}, i})$:

$$\text{Slope} = \frac{N \sum_{i=1}^N (t_i \cdot H_{\text{live}, i}) - \left(\sum_{i=1}^N t_i\right) \left(\sum_{i=1}^N H_{\text{live}, i}\right)}{N \sum_{i=1}^N (t_i^2) - \left(\sum_{i=1}^N t_i\right)^2} \times 60.0 \quad \left(\frac{\text{MB}}{\text{min}}\right)$$

- **Invariant 2 Verification (REQ-130 §3.1, ADR-129)**:
  Under sustained 300-second soak saturation, Toron's live heap growth slope must satisfy:
  $$\text{Slope} \le 1.0\text{ MB/min}$$
  This empirically proves that streaming by default maintains strict constant $O(1) \le 32\text{KB}$ memory boundedness per stream and that no heap memory leaks exist.

### 6.5 Zero-Cycle Resilience & Division-by-Zero Elimination ([CWE-369](https://cwe.mitre.org/data/definitions/369.html))

In short 5-second smoke runs or workloads with zero heap allocations, zero GC cycles occur ($N = 0$). In `benchmarks/telemetry/gcparser/stats.go`:
- If $N = 0$, `ComputeStatistics` immediately returns `&GCTelemetry{Enabled: true, TotalCycles: 0}` with zeroed sub-structures.
- If $N = 1$ or if timestamps are collinear ($\text{denom} \le 10^{-9}$), the linear regression denominator check sets `HeapGrowthSlopeMBm = 0.0`.
- All rate calculations guard against `effectiveDuration <= 0`.
- This ensures zero division-by-zero panics, zero `NaN`, and zero `+Inf` values in serialized JSON.

---

## 7. CLI Usage & Multi-Tier Flags Reference

The benchmark load generator supports direct Go execution, single-tier runs, and sequential multi-tier sweep orchestration.

### 7.1 CLI Flags Reference

| Flag (Go / Shell) | Type | Default Value | Description |
| :--- | :---: | :---: | :--- |
| `-url` | `string` | `http://127.0.0.1:8080/health` | Target HTTP URL endpoint for benign stream traffic |
| `-c` | `int` | `20` | Concurrency: number of parallel worker goroutines |
| `-d` | `string` / `duration` | `5s` | Duration per tier (`5s`, `60s`, `300s`) or comma-separated list (`5s,60s,300s`) |
| `--tier` (shell) / `-tier` (Go) | `string` | `quick` | Preset tier: `quick` (5s), `medium`/`steady` (60s), `soak` (300s), `all` (5s,60s,300s) |
| `-rate` | `int` | `2000` | Aggregate target request rate across all workers in RPS |
| `-attack-ratio` / `-a` | `float` | `0.10` | Fraction of traffic allocated to adversarial probes ($0.0 \le r \le 1.0$) |
| `-gc-trace` | `string` | `""` | Optional path to `server_gc_trace.log` to parse and embed GC telemetry |
| `-m` | `string` | `GET` | HTTP method for benign background requests (`GET`, `POST`, etc.) |
| `-body` | `string` | `""` | Optional request body string for benign requests |
| `-json` | `string` | `""` | Filesystem path to write structured JSON report |
| `-csv` | `string` | `""` | Filesystem path to write aggregated CSV telemetry |
| `-md` | `string` | `""` | Filesystem path to write publication Markdown report |
| `--auto-start` (shell) | `bool` | `false` | Automatically build and launch Toron gateway with `GODEBUG=gctrace=1` |
| `--dry-run` (shell) | `bool` | `false` | Validate arguments, display normalized duration list, and exit |

### 7.2 Running via Go Toolchain

```bash
# Run steady-state 60-second test with GC telemetry parsing
go run ./benchmarks/wrk2/loadgen.go \
    -url http://127.0.0.1:8080/health \
    -c 50 \
    -d 60s \
    -rate 5000 \
    -attack-ratio 0.10 \
    -tier medium \
    -gc-trace benchmarks/results/server_gc_trace.log \
    -json benchmarks/results/saturation_stress_60s.json \
    -md benchmarks/results/saturation_stress_60s.md
```

### 7.3 Running via Automated Shell Harness (`run_saturation_stress.sh`)

`benchmarks/wrk2/run_saturation_stress.sh` provides full multi-tier execution management:

```bash
# 1. Quick CI Smoke Test (default: 5s, < 60s runtime)
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start

# 2. Steady-State GC Observation Tier (60s)
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start --tier medium -r 5000 -c 50

# 3. Long-Term Soak Tier (300s / 5 minutes)
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start --tier soak -r 5000 -c 50

# 4. Comprehensive All-Tiers Sweep (5s + 60s + 300s)
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start --tier all -r 5000 -c 50
```

#### Sequential Multi-Tier Execution & Artifact Generation
When multiple durations are specified (e.g. `--tier all` or `-d 5s,60s,300s`):
1. The script loops sequentially through each duration tier.
2. For each tier, the background Toron gateway is maintained or cleanly recycled, and isolated duration-keyed reports are generated:
   - `benchmarks/results/saturation_stress_5s.json` and `.md`
   - `benchmarks/results/saturation_stress_60s.json` and `.md`
   - `benchmarks/results/saturation_stress_300s.json` and `.md`
3. After all tiers complete, `loadgen.go` executes `ConsolidateReports`, synthesizing a master consolidated report:
   - `benchmarks/results/saturation_stress_report.json`
   - `benchmarks/results/saturation_stress_report.md`
   Featuring cross-tier comparative tables contrasting throughput, $p99$ tail latency, and GC overhead across 5s, 60s, and 300s.

---

## 8. Telemetry Export Formats

### 8.1 JSON Report Structure (with `gc_telemetry`)

The generated JSON artifact (`SaturationStressReport`) captures complete disaggregated telemetry and Go runtime GC dynamics:

```json
{
  "timestamp": "2026-09-16T12:00:00Z",
  "target_url": "http://127.0.0.1:8080/health",
  "concurrency": 50,
  "duration_seconds": 60.0,
  "duration_tier": "medium",
  "target_rate_rps": 5000,
  "attack_ratio": 0.10,
  "total_requests_executed": 298450,
  "total_actual_rps": 4974.1,
  "zero_starvation_verified": true,
  "invariant_enforcement_rate_pct": 100.0,
  "active_defense_rate_pct": 100.0,
  "route_miss_rate_pct": 0.0,
  "overall_verdict": "PASS",
  "benign_stream": {
    "stream_type": "benign",
    "total_requests": 268605,
    "success_requests": 268605,
    "failed_requests": 0,
    "actual_rps": 4476.7,
    "latencies_ms": { "p50": 1.15, "p90": 2.48, "p99": 4.92 }
  },
  "adversarial_stream": {
    "stream_type": "adversarial",
    "total_requests": 29845,
    "success_requests": 29845,
    "failed_requests": 0,
    "active_defense_requests": 29845,
    "route_miss_requests": 0,
    "bypassed_requests": 0,
    "unhandled_requests": 0,
    "actual_rps": 497.4,
    "latencies_ms": { "p50": 0.38, "p90": 0.68, "p99": 1.25 },
    "status_codes": { "400": 22682, "403": 2089, "413": 1790, "431": 1584, "501": 1700 }
  },
  "gc_telemetry": {
    "enabled": true,
    "total_cycles": 112,
    "gc_cpu_percent": 1.4,
    "cycles_per_second": 1.87,
    "total_reclaimed_mb": 1845.2,
    "pause_times_ms": {
      "min_stw_ms": 0.021,
      "mean_stw_ms": 0.048,
      "p50_stw_ms": 0.045,
      "p95_stw_ms": 0.078,
      "p99_stw_ms": 0.112,
      "max_stw_ms": 0.185,
      "total_stw_ms": 5.38,
      "mean_mark_ms": 0.95,
      "max_mark_ms": 2.10
    },
    "heap_metrics_mb": {
      "initial_live_heap_mb": 5.4,
      "final_live_heap_mb": 6.1,
      "peak_live_heap_mb": 7.8,
      "mean_live_heap_mb": 6.0,
      "peak_trigger_heap_mb": 14.2,
      "heap_growth_slope_mb_per_min": 0.08
    }
  }
}
```

### 8.2 Markdown Section 5 Table: Runtime Garbage Collection & Memory Dynamics

In `saturation_stress_report.md`, Section 5 renders a dedicated table detailing GC dynamics:

```markdown
## 5. Runtime Garbage Collection & Memory Dynamics (`GODEBUG=gctrace=1`)

| Metric | Measured Value | Analysis / Compliance Invariant |
| :--- | :---: | :--- |
| **Total GC Cycles** | 112 | 1.87 cycles/sec across 60.0s window |
| **GC CPU Overhead** | 1.4% | Background mark & assist CPU overhead |
| **Total Heap Reclaimed** | 1,845.2 MB | Cumulative memory reclaimed by sweeper |
| **STW Pause (Min / Mean / Max)** | 0.021 ms / 0.048 ms / 0.185 ms | Stop-The-World clock duration range |
| **STW Pause (P50 / P95 / P99)** | 0.045 ms / 0.078 ms / 0.112 ms | High-percentile Stop-The-World latency impact |
| **Concurrent Mark (Mean / Max)**| 0.950 ms / 2.100 ms | Non-blocking background mark clock duration |
| **Live Heap Floor (Init / Final)**| 5.4 MB / 6.1 MB | Baseline live objects retained across sweep |
| **Live Heap Peak** | 7.8 MB | Maximum live heap size during test window |
| **Heap Growth Slope** | **0.08 MB/min** | **PASS: Strictly O(1) <= 1.0 MB/min Bounded** |
```

### 8.3 CSV Telemetry Summary

```csv
Concurrency,TargetRPS,TotalActualRPS,BenignRPS,BenignP50_ms,BenignP99_ms,AttackRPS,AttackRejected,AttackRouteMiss,AttackBypassed,AttackUnhandled,ActiveDefenseRatePct,ZeroStarvation,GCCycles,GCMeanSTW_ms,GCP99STW_ms,GCSlope_MBm
50,5000,4974.10,4476.70,1.15,4.92,497.40,29845,0,0,0,100.0,true,112,0.048,0.112,0.08
```

---

## 9. Verification & Automated Test Coverage (`TC-117`, `TC-130`)

The saturation stress benchmark harness and telemetry parser are verified by dedicated test suites across unit, integration, and benchmark layers:

```bash
# Execute telemetry parser unit tests
go test -v -race -count=1 ./benchmarks/telemetry/gcparser/...

# Execute wrk2 loadgen test suite
go test -v -race -count=1 ./benchmarks/wrk2/...
```

### Verification Oracles & Traceability

| Test Identifier | Test Function / File | Verification Target |
| :--- | :--- | :--- |
| **`TC-117.1-9`** | `loadgen_test.go` | Four-tier status classification logic, route miss rejection, 0% credit for 404, Table 6 formatting, and AST check for no catch-all else. |
| **`TC-130.1`** | `run_saturation_stress.sh` | CLI flag parsing and validation (`-d`, `--tier quick\|medium\|soak\|all`, invalid rejection, default 5s). |
| **`TC-130.2`** | `run_saturation_stress.sh`, `loadgen.go` | Sequential multi-tier execution loop, PID management, duration-keyed artifacts, and report consolidation. |
| **`TC-130.4`** | `run_saturation_stress.sh` | `GODEBUG=gctrace=1` process environment injection and clean stderr segregation to `server_gc_trace.log`. |
| **`TC-130.5`** | `parser_test.go:14-74` | Syntax scanning across Go 1.20, Go 1.22, and Go 1.24+ `gctrace` formats. |
| **`TC-130.6`** | `parser_test.go:77-140` | STW pause percentiles (Min, Mean, P50, P95, P99, Max, Total) and mark duration computation. |
| **`TC-130.7`** | `parser_test.go:143-197` | OLS linear regression heap growth slope ($MB/\text{min}$) across flat, linear, cyclic, and $N=1$ inputs. |
| **`TC-130.8`** | `parser_test.go:200-234` | Zero-cycle GC trace handling ($N=0$ graceful fallback without panics, `NaN`, or `+Inf`). |
| **`TC-130.9`** | `parser_test.go:237-281` | Skipping non-GC lines (application logs, stack traces) with zero heap allocations ($> 200\text{k}$ lines/sec). |
| **`TC-130.15`** | `loadgen.go` | JSON report schema extension embedding `gc_telemetry` and `duration_tier`. |
| **`TC-130.16`** | `loadgen.go` | Markdown report generation with Section 5 GC dynamics comparative table. |
| **`TC-130.18`** | `loadgen.go` | Invariant 1: Uncompromised dual-stream telemetry and 4-tier status classification across all tiers. |
| **`TC-130.19`** | `loadgen.go`, `stats.go` | Invariant 2: Constant $O(1)$ memory boundedness soak verification (heap growth slope $\le 1.0\text{ MB/min}$). |
| **`TC-130.20`** | `parser_test.go:284-306` | Concurrency and thread safety validation under `go test -race ./benchmarks/...`. |

---

## 10. Related Specifications & Documentation

- `REQ-130` – Multi-Tier Duration Stress Testing (5s, 60s, 300s), Runtime GC Telemetry Capture, and Differential Reverse Proxy Benchmarking
- `TASK-153` – Engineering Task for Multi-Tier Duration Stress Testing and Go Runtime GC Telemetry Capture
- `ADR-130` – Architectural Decision Record for Multi-Tier Stress Testing and GC Telemetry Engine
- `TC-130` – Test Specification for Multi-Tier Duration Testing and GC Trace Extraction
- `CR-126` – Code Review of Multi-Tier Duration Stress Testing and GC Telemetry
- `SR-130` – Security Review of Multi-Tier Stress Testing and Process Boundary Isolation
- `REQ-129` / `ADR-129` – Streaming by Default and Memory Boundedness Invariants ($O(1) \le 32\text{KB}$)
- `REQ-114` / `TASK-137` – High-Concurrency Saturation Stress Testing with Background Traffic (BMK-04)
- `REQ-117` / `TASK-140` – Disaggregated Adversarial Status Classification and Route-Miss Separation (HARN-01)
- `REQ-119` / `TASK-142` – Historical Result Retention and Manifest Indexing
- [Multi-Proxy Differential Docker Benchmark Suite](./docker-compare-benchmark.md) – Containerized comparison against NGINX, Traefik, Caddy, HAProxy
- [Master Benchmark Suite Guide](./benchmarking.md) – Microbenchmarks, loadgen, retention model, and CI execution
- [Release Notes](../release-notes.md) – Toron v1.5.29 Release Notes

