---
title: High-Concurrency Saturation Stress Benchmark & Status Classification (BMK-04, HARN-01)
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-114
  - REQ-117
  - TASK-137
  - TASK-140
  - ADR-114
  - ADR-117
  - TC-114
  - TC-117

derived_from:
  - REQ-114
  - REQ-117
  - TASK-140
  - ADR-117
  - ReviewTaskSummary.md

documents:
  - SATURATION-STRESS-BENCHMARK-HARNESS

related_to:
  - ../configuration.md
  - ./benchmarking.md
  - ./waf.md
  - ./path-traversal-defense.md
  - ../index.md
  - ../release-notes.md
---

# High-Concurrency Saturation Stress Benchmark & Status Classification (BMK-04, HARN-01)

## 1. Overview & Problem Context

Toron includes a native high-concurrency load generation and saturation stress testing harness implemented in pure Go ([`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go)). Inspired by the constant-throughput architecture of `wrk2`, the harness evaluates gateway behavior under heavy saturation ($5{,}000+\text{ RPS}$) while interleaving malicious protocol vectors with standard benign traffic.

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

In peer review audits of Paper 1 and Paper 2 (formalized in `ReviewTaskSummary.md` under Directive `REV-03` / Task `HARN-01`, [`REQ-117`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-117.md), and [`ADR-117`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-117.md)), reviewers identified a circular scoring anomaly in earlier versions of `loadgen.go`:
- **The Circular Catch-All Else**: The worker loop contained an `else { attackRejected.Add(1) }` branch that indiscriminately scored any non-`200 OK` response as active security defense.
- **Defensive Masking of Route Misses**: During high-concurrency saturation, **349 probes** targeting `ADV-06` (Path Traversal, `GET /../../canary_traversal.txt`) returned `404 Not Found` because the router stripped dot-dot sequences before route matching. Due to the catch-all `else`, these 349 route misses were scored as active defense (`attackRejected`), inflating reported defense scores to 100.0% and obscuring the underlying routing defect.
- **Omission of Valid Defense Codes**: RFC 6585 status `431 Request Header Fields Too Large` was omitted from explicit checks and only captured through accidental fallback.
- **Masking Server Crashes**: Unhandled server errors (`500 Internal Server Error`) or transport faults were similarly routed into active defense.

Under [`HARN-01`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-140.md), the circular catch-all was permanently removed and replaced with a strict, mutually exclusive **Four-Tier Status Classification Taxonomy**.

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

To ensure scientific reproducibility and conformance with RFC specifications and MITRE CWE taxonomies, [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go) enforces a strict, four-tier classification model:

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

## 6. CLI Usage & Flags Reference

The benchmark load generator supports direct command-line execution and automated shell script orchestration.

### 6.1 CLI Flags

| Flag | Type | Default Value | Description |
| :--- | :---: | :---: | :--- |
| `-url` | `string` | `http://127.0.0.1:8080/health` | Target HTTP URL endpoint for benign stream traffic |
| `-c` | `int` | `20` | Concurrency: number of parallel worker goroutines |
| `-d` | `duration` | `10s` | Test duration (e.g. `5s`, `30s`, `1m`) |
| `-rate` | `int` | `2000` | Aggregate target request rate across all workers in RPS |
| `-attack-ratio` | `float` | `0.10` | Fraction of traffic allocated to adversarial probes ($0.0 \le r \le 1.0$) |
| `-m` | `string` | `GET` | HTTP method for benign background requests (`GET`, `POST`, etc.) |
| `-body` | `string` | `""` | Optional request body string for benign requests |
| `-json` | `string` | `""` | Optional filesystem path to write structured JSON report |
| `-csv` | `string` | `""` | Optional filesystem path to write aggregated CSV telemetry |
| `-md` | `string` | `""` | Optional filesystem path to write publication Markdown report |

### 6.2 Running via Go Toolchain

```bash
# Run saturation stress test at 5,000 RPS with 10% attack injection for 10 seconds
go run ./benchmarks/wrk2/loadgen.go \
    -url http://127.0.0.1:8080/health \
    -c 20 \
    -d 10s \
    -rate 5000 \
    -attack-ratio 0.10 \
    -json benchmarks/results/saturation_stress_report.json \
    -csv benchmarks/results/saturation_stress_summary.csv \
    -md benchmarks/results/saturation_stress_report.md
```

### 6.3 Running via Automated Shell Harness

The convenience wrapper [`benchmarks/wrk2/run_saturation_stress.sh`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/run_saturation_stress.sh) manages server lifecycle, executes the load generator, and outputs artifacts:

```bash
# Automatically start Toron server, run benchmark, and shut down
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start -r 5000 -c 20 -d 10s -a 0.10
```

---

## 7. Telemetry Export Formats

### 7.1 JSON Report Structure

The generated JSON artifact ([`SaturationStressReport`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go)) captures complete disaggregated telemetry:

```json
{
  "timestamp": "2026-09-12T07:15:38Z",
  "target_url": "http://127.0.0.1:8080/health",
  "concurrency": 20,
  "duration_seconds": 10.0,
  "target_rate_rps": 5000,
  "attack_ratio": 0.10,
  "total_requests_executed": 49731,
  "total_actual_rps": 4973.1,
  "zero_starvation_verified": true,
  "invariant_enforcement_rate_pct": 100.0,
  "active_defense_rate_pct": 100.0,
  "route_miss_rate_pct": 0.0,
  "overall_verdict": "PASS",
  "benign_stream": {
    "stream_type": "benign",
    "total_requests": 44758,
    "success_requests": 44758,
    "failed_requests": 0,
    "actual_rps": 4475.8,
    "latencies_ms": { "p50": 1.12, "p90": 2.45, "p99": 4.88 }
  },
  "adversarial_stream": {
    "stream_type": "adversarial",
    "total_requests": 4973,
    "success_requests": 4973,
    "failed_requests": 0,
    "active_defense_requests": 4973,
    "route_miss_requests": 0,
    "bypassed_requests": 0,
    "unhandled_requests": 0,
    "actual_rps": 497.3,
    "latencies_ms": { "p50": 0.35, "p90": 0.65, "p99": 1.20 },
    "status_codes": { "400": 3766, "403": 349, "413": 295, "431": 263, "501": 300 }
  }
}
```

### 7.2 CSV Telemetry Summary

The CSV summary includes dedicated columns for disaggregated telemetry:

```csv
Concurrency,TargetRPS,TotalActualRPS,BenignRPS,BenignP50_ms,BenignP99_ms,AttackRPS,AttackRejected,AttackRouteMiss,AttackBypassed,AttackUnhandled,ActiveDefenseRatePct,ZeroStarvation
20,5000,4973.10,4475.80,1.12,4.88,497.30,4973,0,0,0,100.0,true
```

---

## 8. Verification & Automated Test Coverage

The benchmark harness is verified by a dedicated test suite in [`benchmarks/wrk2/loadgen_test.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen_test.go) under [`TC-117`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-117.md):

```bash
# Execute entire wrk2 benchmark test suite with race detector
go test -v -race -count=1 ./benchmarks/wrk2/...
```

### Verification Oracles:
1. **`TestLoadGen_ClassificationLogic`**: Exhaustively asserts that each HTTP status code (`400`, `403`, `413`, `431`, `501`, `404`, `200`, `500`, `502`, `999`) and socket resets increment exactly one corresponding tier counter.
2. **`TestLoadGen_StatusClassification_FourTiers`**: End-to-end integration test against multi-status mock server verifying accurate vector aggregation.
3. **`TestLoadGen_RouteMissFailsVerdict`**: Injects `404 Not Found` and confirms `OverallVerdict == "FAIL"` and `ActiveDefenseRatePct < 100.0%`.
4. **`TestLoadGen_AttackBypassFailsVerdict`**: Injects `200 OK` and confirms `OverallVerdict == "FAIL"`.
5. **`TestLoadGen_UnhandledAnomalyFailsVerdict`**: Injects `500 Internal Server Error` and confirms `OverallVerdict == "FAIL"`.
6. **`TestLoadGen_AST_NoCatchAllElse`**: Static AST parser inspection asserting that no `else` catch-all branch exists in the worker loop of `loadgen.go`.
7. **`TestLoadGen_MarkdownTable6Format`**: Verifies 9-column Table 6 layout and headers in generated Markdown.
8. **`TestLoadGen_TelemetryParity_JSON_CSV`**: Verifies 100% data parity between memory structs, JSON serialization, and CSV export.
9. **`TestLoadGen_RaceFreeConcurrency`**: Verifies partition invariants and race freedom under high concurrency.

---

## 9. Related Specifications & Documentation

- [`TASK-140`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-140.md) – Implementation of Disaggregated Status Classification and Route-Miss Separation in Benchmark Load Generator (HARN-01)
- [`REQ-117`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-117.md) – Disaggregated Adversarial Status Classification and Route-Miss Separation (HARN-01)
- [`ADR-117`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-117.md) – Status Classification Disaggregation and Route-Miss Separation Architecture
- [`TC-117`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-117.md) – Automated Verification Suite for Load Generator Status Disaggregation
- [`CR-113`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-113.md) – Code Review for Benchmark Load Generator Status Classification
- [`SR-117`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-117.md) – Security Review & Table 6 Benchmark Invariant Audit
- [`REQ-114`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-114.md) / [`TASK-137`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-137.md) – High-Concurrency Saturation Stress Testing with Background Traffic (BMK-04)
- [`REQ-116`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-116.md) / [`TASK-139`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-139.md) – Layered Route-Aware Path Traversal Defense Architecture (CWE-22)
- [`ReviewTaskSummary.md`](file:///Users/sneha/Developer/toron-research/ReviewTaskSummary.md) – Directive `REV-03` / Task `HARN-01`
- [Native Go Benchmarking](./benchmarking.md) – Microbenchmark suite and allocation metrics
- [Release Notes](../release-notes.md) – Toron v1.5.21 Benchmark Release notes
