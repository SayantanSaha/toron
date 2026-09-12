---
id: TASK-140
type: task
title: Implementation of Disaggregated Status Classification and Route-Miss Separation in Benchmark Load Generator (HARN-01)
status: ready
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-12
updated: 2026-09-12
depends_on:
  - TASK-134
  - TASK-139
derived_from:
  - REQ-117
implements:
  - REQ-117
verified_by:
  - TC-117
decided_by:
  - ADR-117
related_to:
  - CR-113
  - SR-117
  - REQ-114
  - REQ-116
---

# TASK-140 - Implementation of Disaggregated Status Classification and Route-Miss Separation in Benchmark Load Generator (HARN-01)

## 1. Description & Context

Decompose and coordinate the engineering implementation of Disaggregated Adversarial Status Classification and Route-Miss Separation in Toron's benchmark load generator harness ([`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go)) as specified in [`REQ-117`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-117.md) and [`ADR-117`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-117.md).

### 1.1 Executive Summary & Review Critique
In academic peer reviews of Paper 1 and Paper 2 (documented in `AER-001.md`, `AER-002.md`, and formalized under Directive `REV-03` / Task `HARN-01` of `ReviewTaskSummary.md`), reviewers raised critical methodological challenges against the adversarial stress evaluation reported in Table 6. 

Specifically, Table 6 reported a **100.0% active defense enforcement rate** across all eight adversarial protocol vectors under high-concurrency saturation (5,000+ RPS). However, deep telemetry extraction and status code auditing revealed a severe circular scoring anomaly in [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go):
- In BMK-04 saturation evaluations, **349 probes** targeting vector `ADV-06` (Path Traversal Directory Escape, `GET /../../canary_traversal.txt`) returned HTTP **`404 Not Found`**, not an active security rejection (**`400 Bad Request`**, **`403 Forbidden`**, or **`413 Payload Too Large`**).
- Due to a circular catch-all `else` clause in [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L382-L386), these 349 route misses were indiscriminately incremented into `attackRejected` and recorded as successful active defense.
- This conflated passive router non-matching with active security defense, artificially inflating the reported security score to 100.0% and obscuring the underlying path traversal routing bug addressed in [`REQ-116`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-116.md) and [`TASK-139`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-139.md).

Reviewers correctly argued that classifying a `404 Not Found` response as active defense is scientifically invalid:
> *"A 404 response simply denotes that the request did not match a registered URI handler. If an adversarial probe penetrates routing filters and reaches a passive 404 handler, scoring this as an active security defense masks routing vulnerabilities and inflates defense metrics. The benchmark harness must explicitly decouple active defensive rejections from passive route misses."* (`AER-002`, lines 312–318)

### 1.2 Root Cause Analysis of Circular Scoring Anomaly
Inspection of [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L375-L387) revealed the following flawed scoring logic:

```go
// Line 375-386 in benchmarks/wrk2/loadgen.go
// Assert active defense enforcement: 400 Bad Request or 403 Forbidden
if code == http.StatusBadRequest || code == http.StatusForbidden || code == http.StatusRequestEntityTooLarge || code == http.StatusNotImplemented {
    attackRejected.Add(1)
    stats.rejected.Add(1)
} else if code == http.StatusOK {
    attackBypassed.Add(1)
    stats.bypassed.Add(1)
} else {
    // Other rejections (e.g. 500, socket drop) still constitute defense block
    attackRejected.Add(1)
    stats.rejected.Add(1)
}
```

The fundamental flaws in this architectural logic are:
1. **The Circular Catch-All**: Any HTTP response code other than `200 OK` (such as `404 Not Found`, `301 Moved Permanently`, `500 Internal Server Error`, or `502 Bad Gateway`) was routed into the `else` branch, incrementing `attackRejected` and `stats.rejected`.
2. **Omission of Valid Active Defense Codes**: Standard HTTP defense codes such as `431 Request Header Fields Too Large` (returned by Toron's parser on oversized headers, `ADV-07`) were omitted from the explicit `if` condition and only captured via the accidental `else` fallback.
3. **Route Miss Conflation**: An attack probe targeting an unmapped endpoint or evading router prefix boundaries to produce a `404 Not Found` was erroneously credited as active defense rather than flagged as an unintercepted route miss.
4. **Lack of Anomaly Isolation**: Internal server panics or unhandled errors producing `500 Internal Server Error` were scored as active defense rather than server crashes or implementation failures.

### 1.3 Target Architectural Model: Four-Tier Status Classification Taxonomy
To ensure scientific integrity and alignment with RFC standards and MITRE CWE taxonomies, [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go) shall establish a strict, mutually exclusive four-tier classification system for all adversarial probe responses:
1. **Tier 1: Active Defense (`attackRejected`)**: Explicit defensive rejections (`400 Bad Request`, `403 Forbidden`, `413 Payload Too Large`, `431 Request Header Fields Too Large`, `501 Not Implemented`).
2. **Tier 2: Route Miss (`attackRouteMiss`)**: Unmatched URI paths reaching passive 404 handlers (`404 Not Found`). Excluded from defense scores.
3. **Tier 3: Attack Bypass (`attackBypassed`)**: Invariant violations accepted and processed successfully (`200 OK` or unexpected 2xx/3xx).
4. **Tier 4: Unhandled / Protocol Anomaly (`attackUnhandled`)**: Unexpected server errors, crashes, or protocol desynchronizations (`500 Internal Server Error`, 5xx, or non-standard codes).

---

## 2. Subtask Breakdown

### TASK-140.1: Refactor Response Code Handling and Eliminate Circular Catch-All Else (FR-1)
- **Target File**: [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L375-L387)
- **Scope**:
  - Locate the worker goroutine loop in [`RunLoadGen`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L237).
  - Completely remove the existing conditional block:
    ```go
    if code == http.StatusBadRequest || code == http.StatusForbidden || ... {
        ...
    } else if code == http.StatusOK {
        ...
    } else {
        attackRejected.Add(1)
        stats.rejected.Add(1)
    }
    ```
  - Implement the strict, exhaustive `switch code` construct:
    ```go
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
  - Preserve the fail-fast socket reset behavior in [`executeAttackProbe`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L203-L234): when a connection is actively reset or terminated by the server during header transmission, it continues to return status code `400`, ensuring valid transport-level defensive blocks are classified under Tier 1 (`http.StatusBadRequest`).
- **Invariants**:
  - No response code shall ever fall through to an ambiguous or circular counter.
  - Zero heap allocations within the `switch` block on the critical path ($< 5\text{ ns}$ execution budget).

### TASK-140.2: Augment Core Memory Data Structures & Atomic Telemetry (FR-2)
- **Target File**: [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go)
- **Scope**:
  1. **Worker Goroutine Local & Global Atomic Counters** in [`RunLoadGen`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L288-L293):
     - Replace the current three attack counters (`attackTotal`, `attackRejected`, `attackBypassed`) with five distinct 64-bit atomic counters:
       ```go
       var attackTotal atomic.Int64
       var attackRejected atomic.Int64   // Active defense: 400, 403, 413, 431, 501
       var attackRouteMiss atomic.Int64  // Route miss: 404
       var attackBypassed atomic.Int64   // Bypass: 200 (or unexpected 2xx)
       var attackUnhandled atomic.Int64  // Unhandled / server error: 5xx, etc.
       var attackBytes atomic.Int64
       ```
  2. **Per-Vector Atomic Telemetry (`vecStats`)** in [`RunLoadGen`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L296-L300):
     - Augment the internal [`vecStats`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L296-L300) struct:
       ```go
       type vecStats struct {
           sent      atomic.Int64
           rejected  atomic.Int64
           routeMiss atomic.Int64
           bypassed  atomic.Int64
           unhandled atomic.Int64
       }
       ```
  3. **Stream Metrics Struct ([`StreamMetrics`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L40-L50))**:
     - Expand [`StreamMetrics`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L40-L50) to include disaggregated probe classification fields:
       ```go
       type StreamMetrics struct {
           StreamType            string             `json:"stream_type"` // "benign" or "adversarial"
           TotalRequests         int64              `json:"total_requests"`
           SuccessRequests       int64              `json:"success_requests"`        // benign: 200 OK; adversarial: active rejections
           FailedRequests        int64              `json:"failed_requests"`         // benign: non-200; adversarial: non-rejections
           ActiveDefenseRequests int64              `json:"active_defense_requests"` // explicit 400/403/413/431/501
           RouteMissRequests     int64              `json:"route_miss_requests"`     // explicit 404
           BypassedRequests      int64              `json:"bypassed_requests"`       // explicit 200
           UnhandledRequests     int64              `json:"unhandled_requests"`      // explicit 5xx/other
           ActualRPS             float64            `json:"actual_rps"`
           BytesRead             int64              `json:"bytes_read"`
           ThroughputMBs         float64            `json:"throughput_mb_s"`
           LatenciesMs           LatencyPercentiles `json:"latencies_ms"`
           StatusCodes           map[int]int64      `json:"status_codes"`
       }
       ```
  4. **Attack Vector Summary Struct ([`AttackVectorSummary`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L53-L61))**:
     - Augment [`AttackVectorSummary`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L53-L61) with disaggregated telemetry fields:
       ```go
       type AttackVectorSummary struct {
           ID                   string  `json:"id"`
           Name                 string  `json:"name"`
           Category             string  `json:"category"`
           ProbesSent           int64   `json:"probes_sent"`
           Rejected             int64   `json:"rejected"`                 // Active defense (400, 403, 413, 431, 501)
           RouteMiss            int64   `json:"route_miss"`               // Route miss (404)
           Bypassed             int64   `json:"bypassed"`                 // Bypass (200)
           Unhandled            int64   `json:"unhandled"`                // Anomaly (5xx, etc.)
           ActiveDefenseRatePct float64 `json:"active_defense_rate_pct"` // Rejected / ProbesSent * 100
           RouteMissRatePct     float64 `json:"route_miss_rate_pct"`      // RouteMiss / ProbesSent * 100
           PassRatePct          float64 `json:"pass_rate_pct"`            // Alias for ActiveDefenseRatePct
       }
       ```
  5. **Saturation Stress Report Struct ([`SaturationStressReport`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L64-L80))**:
     - Augment top-level report telemetry:
       ```go
       type SaturationStressReport struct {
           Timestamp                string                `json:"timestamp"`
           TargetURL                string                `json:"target_url"`
           TargetHostPort           string                `json:"target_host_port"`
           Concurrency              int                   `json:"concurrency"`
           DurationSeconds          float64               `json:"duration_seconds"`
           TargetRateRPS            int                   `json:"target_rate_rps"`
           AttackRatio              float64               `json:"attack_ratio"`
           TotalRequestsExecuted    int64                 `json:"total_requests_executed"`
           TotalActualRPS           float64               `json:"total_actual_rps"`
           BenignStream             StreamMetrics         `json:"benign_stream"`
           AdversarialStream        StreamMetrics         `json:"adversarial_stream"`
           AttackVectors            []AttackVectorSummary `json:"attack_vectors"`
           ZeroStarvationVerified   bool                  `json:"zero_starvation_verified"`
           InvariantEnforcementRate float64               `json:"invariant_enforcement_rate_pct"` // Strictly Active Defense Rate
           ActiveDefenseRatePct     float64               `json:"active_defense_rate_pct"`
           RouteMissRatePct         float64               `json:"route_miss_rate_pct"`
           OverallVerdict           string                `json:"overall_verdict"`
       }
       ```

### TASK-140.3: Realign Invariant Rate Computations and Enforce Strict Verdict Logic (FR-3)
- **Target File**: [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L476-L547)
- **Scope**:
  - Realign mathematical rate computations in [`RunLoadGen`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L476-L508):
    ```go
    totAttack := attackTotal.Load()
    totRej := attackRejected.Load()
    totRouteMiss := attackRouteMiss.Load()
    totBypassed := attackBypassed.Load()
    totUnhandled := attackUnhandled.Load()

    activeDefenseRate := 100.0
    routeMissRate := 0.0
    if totAttack > 0 {
        activeDefenseRate = float64(totRej) / float64(totAttack) * 100.0
        routeMissRate = float64(totRouteMiss) / float64(totAttack) * 100.0
    }
    ```
  - Ensure `InvariantEnforcementRate` strictly equals `activeDefenseRate` ($\frac{\text{attackRejected}}{\text{attackTotal}} \times 100.0$).
  - In per-vector loop over `catalog`:
    ```go
    vecSummaries := make([]AttackVectorSummary, 0, len(catalog))
    for _, v := range catalog {
        st := vStats[v.ID]
        sent := st.sent.Load()
        rej := st.rejected.Load()
        rm := st.routeMiss.Load()
        byp := st.bypassed.Load()
        unh := st.unhandled.Load()

        rate := 100.0
        rmRate := 0.0
        if sent > 0 {
            rate = float64(rej) / float64(sent) * 100.0
            rmRate = float64(rm) / float64(sent) * 100.0
        }
        vecSummaries = append(vecSummaries, AttackVectorSummary{
            ID:                   v.ID,
            Name:                 v.Name,
            Category:             v.Category,
            ProbesSent:           sent,
            Rejected:             rej,
            RouteMiss:            rm,
            Bypassed:             byp,
            Unhandled:            unh,
            ActiveDefenseRatePct: rate,
            RouteMissRatePct:     rmRate,
            PassRatePct:          rate,
        })
    }
    ```
  - Realign strict evaluation criteria for `OverallVerdict`:
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
  - In `report.AdversarialStream`, populate all new fields:
    ```go
    AdversarialStream: StreamMetrics{
        StreamType:            "adversarial",
        TotalRequests:         totAttack,
        SuccessRequests:       totRej,
        FailedRequests:        totAttack - totRej, // Any non-active defense is considered failed defense
        ActiveDefenseRequests: totRej,
        RouteMissRequests:     totRouteMiss,
        BypassedRequests:      totBypassed,
        UnhandledRequests:     totUnhandled,
        ActualRPS:             attackRPS,
        BytesRead:             attackBytes.Load(),
        ThroughputMBs:         attackThroughput,
        LatenciesMs:           attackPercentiles,
        StatusCodes:           attackStatusCodes,
    }
    ```
  - Populate `ActiveDefenseRatePct: activeDefenseRate` and `RouteMissRatePct: routeMissRate` on [`SaturationStressReport`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L509-L547).

### TASK-140.4: Update Publication-Grade Report Generators, Serializers, and CLI Output (FR-4, FR-5)
- **Target File**: [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L550-L736)
- **Scope**:
  1. **Publication-Grade Markdown Report (`GenerateMarkdownReport`)**:
     - In **Section 1 (Executive Summary)**:
       ```go
       md.WriteString(fmt.Sprintf("3. **100.0%% Active Defense Enforcement**: Exactly **%d/%d** interleaved malformed protocol probes were intercepted with verified active defense ($400/403/413/431/501$), with **0 route misses ($404$)**, **0 unhandled anomalies**, and **`0` security bypasses**.\n",
           rep.AdversarialStream.ActiveDefenseRequests, rep.AdversarialStream.TotalRequests))
       ```
     - In **Section 2 (Decoupled Dual-Stream Performance Summary)**:
       ```go
       md.WriteString(fmt.Sprintf("| **Evaluation Verdict** | %d Success / %d Failed (0.0%% Error) | %d Active Defense / %d Route Miss / %d Bypass (**%.1f%% Active Defense**) | **%s** |\n",
           rep.BenignStream.SuccessRequests, rep.BenignStream.FailedRequests,
           rep.AdversarialStream.ActiveDefenseRequests, rep.AdversarialStream.RouteMissRequests, rep.AdversarialStream.BypassedRequests,
           rep.ActiveDefenseRatePct, rep.OverallVerdict))
       ```
     - In **Section 4 (Concurrent Adversarial Invariant Breakdown - Table 6 Alignment)**:
       Update table headers and row formatting to match Paper 1 and Paper 2 Table 6 with disaggregated columns:
       ```go
       md.WriteString("## 4. Concurrent Adversarial Invariant Breakdown (Table 6 Alignment)\n\n")
       md.WriteString("| Vector ID | Attack Name | Category | Probes Sent | Active Defense (4xx/501) | Route Miss (404) | Bypass Count (200) | Unhandled | Active Defense Rate |\n")
       md.WriteString("| :--- | :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |\n")
       for _, v := range rep.AttackVectors {
           md.WriteString(fmt.Sprintf("| `%s` | %s | %s | %d | %d | %d | %d | %d | **%.1f%%** |\n",
               v.ID, v.Name, v.Category, v.ProbesSent, v.Rejected, v.RouteMiss, v.Bypassed, v.Unhandled, v.ActiveDefenseRatePct))
       }
       ```
  2. **JSON Serializer**:
     - Ensure all newly added fields on [`StreamMetrics`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L40-L50), [`AttackVectorSummary`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L53-L61), and [`SaturationStressReport`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L64-L80) serialize cleanly to standard JSON (`indent: "  "`).
  3. **CSV Telemetry Export**:
     - In [`main()`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L625-L736), expand the CSV column headers and records:
       ```go
       writer.Write([]string{
           "Concurrency", "TargetRPS", "TotalActualRPS", "BenignRPS", "BenignP50_ms", "BenignP99_ms",
           "AttackRPS", "AttackRejected", "AttackRouteMiss", "AttackBypassed", "AttackUnhandled",
           "ActiveDefenseRatePct", "ZeroStarvation",
       })
       writer.Write([]string{
           strconv.Itoa(report.Concurrency),
           strconv.Itoa(report.TargetRateRPS),
           fmt.Sprintf("%.2f", report.TotalActualRPS),
           fmt.Sprintf("%.2f", report.BenignStream.ActualRPS),
           fmt.Sprintf("%.2f", report.BenignStream.LatenciesMs.P50),
           fmt.Sprintf("%.2f", report.BenignStream.LatenciesMs.P99),
           fmt.Sprintf("%.2f", report.AdversarialStream.ActualRPS),
           strconv.FormatInt(report.AdversarialStream.ActiveDefenseRequests, 10),
           strconv.FormatInt(report.AdversarialStream.RouteMissRequests, 10),
           strconv.FormatInt(report.AdversarialStream.BypassedRequests, 10),
           strconv.FormatInt(report.AdversarialStream.UnhandledRequests, 10),
           fmt.Sprintf("%.1f", report.ActiveDefenseRatePct),
           strconv.FormatBool(report.ZeroStarvationVerified),
       })
       ```
  4. **CLI Console Output**:
     - In [`main()`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L625-L736), format stdout terminal block with disaggregated rows:
       ```text
       --------------------------------------------------------------------------------
        Adversarial Stream (%d attack probes):
          Active Defense (4xx/501): %d (%.1f%%)
          Route Misses (404):       %d (%.1f%%)
          Attack Bypasses (200):    %d (%.1f%%)
          Unhandled Anomalies:      %d (%.1f%%)
          Active Defense Rate:       %.1f%%
          Fast-Fail p50:            %8.2f ms
          Fast-Fail p90:            %8.2f ms
          Fast-Fail p99:            %8.2f ms
       --------------------------------------------------------------------------------
       ```

### TASK-140.5: Implement Comprehensive Unit, Integration, and Race-Free Test Coverage (NFR-1, NFR-4, AC-1..AC-8)
- **Target File**: [`benchmarks/wrk2/loadgen_test.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen_test.go)
- **Scope**:
  1. **Status Classification Four-Tier Unit Test (`TestLoadGen_StatusClassification_FourTiers`)**:
     - Construct a mock HTTP/TCP server that systematically returns representative status codes across all four tiers:
       - Active Defense: `400 Bad Request`, `403 Forbidden`, `413 Payload Too Large`, `431 Request Header Fields Too Large`, `501 Not Implemented`.
       - Route Miss: `404 Not Found`.
       - Attack Bypass: `200 OK`.
       - Unhandled Anomaly: `500 Internal Server Error`, `502 Bad Gateway`.
     - Execute simulated probes and assert with 100% precision that each status code maps exclusively to its intended atomic counter and vector statistic.
  2. **Route Miss Verdict Failure Test (`TestLoadGen_RouteMissFailsVerdict`)**:
     - Configure a test server where adversarial probes receive `404 Not Found`.
     - Run `RunLoadGen` with `AttackRatio > 0`.
     - Assert `report.AdversarialStream.RouteMissRequests > 0`.
     - Assert `report.AdversarialStream.ActiveDefenseRequests == 0`.
     - Assert `report.ActiveDefenseRatePct < 100.0`.
     - Assert `report.OverallVerdict == "FAIL"`.
  3. **Attack Bypass Verdict Failure Test (`TestLoadGen_AttackBypassFailsVerdict`)**:
     - Configure a test server returning `200 OK` on adversarial probes.
     - Assert `report.AdversarialStream.BypassedRequests > 0` and `report.OverallVerdict == "FAIL"`.
  4. **Unhandled Anomaly Verdict Failure Test (`TestLoadGen_UnhandledAnomalyFailsVerdict`)**:
     - Configure a test server returning `500 Internal Server Error` on adversarial probes.
     - Assert `report.AdversarialStream.UnhandledRequests > 0` and `report.OverallVerdict == "FAIL"`.
  5. **Table 6 Markdown & JSON Verification Test (`TestLoadGen_ReportGeneration_Table6`)**:
     - Execute `RunLoadGen` saving Markdown, JSON, and CSV artifacts.
     - Inspect generated Markdown file content:
       - Verify presence of header: `## 4. Concurrent Adversarial Invariant Breakdown (Table 6 Alignment)`.
       - Verify presence of column headers: `Active Defense (4xx/501)`, `Route Miss (404)`, `Bypass Count (200)`, `Unhandled`, `Active Defense Rate`.
     - Inspect generated JSON file:
       - Unmarshal into [`SaturationStressReport`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go#L64-L80) and assert equality of all atomic counters with serialized fields.
     - Inspect generated CSV file:
       - Assert presence of `AttackRouteMiss`, `AttackBypassed`, `AttackUnhandled`, and `ActiveDefenseRatePct` headers.
  6. **Race-Condition & High Concurrency Verification (`TestLoadGen_RaceFreeConcurrency`)**:
     - Execute high-concurrency (concurrency $= 20$, duration $= 1\text{s}$, rate $= 5000$) run against mock multi-status server.
     - Execute with `go test -race -count=1 ./benchmarks/wrk2/...` ensuring zero race detections.

---

## 3. Acceptance Criteria

- **AC-1 (Catch-All Elimination)**: The circular `else` block (`attackRejected.Add(1); stats.rejected.Add(1)`) is completely absent in [`benchmarks/wrk2/loadgen.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen.go). An explicit `switch code` construct dispatches status codes.
- **AC-2 (Strict Active Defense Classification)**: Ingress status codes `400`, `403`, `413`, `431`, and `501` increment `attackRejected` and `stats.rejected`, leaving `attackRouteMiss`, `attackBypassed`, and `attackUnhandled` untouched.
- **AC-3 (Route-Miss Isolation)**: Ingress status code `404 Not Found` increments `attackRouteMiss` and `stats.routeMiss`. It is strictly excluded from `attackRejected`, causes `ActiveDefenseRatePct` to drop below 100.0%, and forces `OverallVerdict = "FAIL"`.
- **AC-4 (Attack Bypass Detection)**: Ingress status code `200 OK` increments `attackBypassed` and `stats.bypassed`, does not increment `attackRejected`, and forces `OverallVerdict = "FAIL"`.
- **AC-5 (Unhandled Anomaly Classification)**: Ingress status codes $\ge 500$ (except 501) and all other unexpected codes increment `attackUnhandled` and `stats.unhandled`, do not increment `attackRejected`, and force `OverallVerdict = "FAIL"`.
- **AC-6 (Table 6 Markdown Alignment)**: Generated Markdown reports format Section 4 with disaggregated columns (`Active Defense (4xx/501)`, `Route Miss (404)`, `Bypass Count (200)`, `Unhandled`, `Active Defense Rate`), matching Table 6 in Paper 1 and Paper 2.
- **AC-7 (JSON & CSV Telemetry Consistency)**: Serialized JSON fields (`active_defense_requests`, `route_miss_requests`, `bypassed_requests`, `unhandled_requests`) and CSV columns match internal atomic counters with zero loss or distortion.
- **AC-8 (Zero-Race Concurrency Verification)**: Full test execution passes cleanly under `go test -race -count=1 ./benchmarks/wrk2/...` with zero race warnings.

---

## 4. Technical Constraints & Architecture Invariants

1. **Race-Free Concurrency & Atomic Memory Model (NFR-1)**:
   - All counter mutations across worker goroutines must utilize `sync/atomic.Int64`.
   - All aggregations and reads of atomic counters occur strictly after worker goroutine termination via `wg.Wait()`.
   - The test suite must pass under `go test -race -count=1 ./benchmarks/wrk2/...`.
2. **Zero External Dependencies (NFR-2)**:
   - The implementation shall strictly utilize Go standard library packages:
     - `sync/atomic`, `sync`
     - `net/http`, `net`, `net/url`
     - `encoding/json`, `encoding/csv`
     - `fmt`, `os`, `strings`, `time`, `bufio`, `bytes`, `errors`, `flag`, `math/rand`, `sort`, `strconv`
   - No third-party metrics or benchmarking libraries are permitted.
3. **Sub-Nanosecond Hot-Path Overhead Bounds (NFR-3)**:
   - The `switch code` construct evaluates integer status codes directly with zero memory allocations on the heap.
   - Classification overhead must remain $< 5\text{ ns}$ per probe, preventing measurement distortion at 10,000+ RPS.
4. **Harness Backward Compatibility (NFR-4)**:
   - All CLI flags (`-url`, `-c`, `-d`, `-rate`, `-attack-ratio`, `-m`, `-body`, `-json`, `-csv`, `-md`) must maintain full compatibility with [`benchmarks/wrk2/run_saturation_stress.sh`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/run_saturation_stress.sh).

---

## 5. Security & Status Code Taxonomy Matrix

| Status Code | HTTP Status Constant | Architectural Tier | RFC / CWE Specification | Loadgen Metric Counter | Security Evaluation Semantics |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`400`** | `http.StatusBadRequest` | Active Defense | RFC 7230 §3 / CWE-444 | `attackRejected` | Gateway parser rejected malformed syntax or protocol framing |
| **`403`** | `http.StatusForbidden` | Active Defense | RFC 7231 §6.5.3 / CWE-22 | `attackRejected` | Gateway WAF / router actively blocked unauthorized access |
| **`413`** | `http.StatusRequestEntityTooLarge` | Active Defense | RFC 7231 §6.5.11 / CWE-400 | `attackRejected` | Bounded memory limit enforced on request body payload |
| **`431`** | `http.StatusRequestHeaderFieldsTooLarge` | Active Defense | RFC 6585 §5 / CWE-400 | `attackRejected` | Bounded header limit enforced on oversized header block (`ADV-07`) |
| **`501`** | `http.StatusNotImplemented` | Active Defense | RFC 7231 §6.6.2 / RFC 7230 | `attackRejected` | Unsupported encoding / method explicitly rejected by gateway parser |
| **`404`** | `http.StatusNotFound` | Route Miss | RFC 7231 §6.5.4 | `attackRouteMiss` | Unmatched path reaching default 404 handler; NOT active defense |
| **`200`** | `http.StatusOK` | Attack Bypass | RFC 7231 §6.3.1 | `attackBypassed` | Malicious probe bypassed security filters and executed successfully |
| **`500`** | `http.StatusInternalServerError` | Unhandled Anomaly | RFC 7231 §6.6.1 | `attackUnhandled` | Server crashed, panicked, or failed internally under stress |
| **`Other`** | Any other status code | Unhandled Anomaly | N/A | `attackUnhandled` | Unexpected status code or transport protocol desynchronization |

---

## 6. Verification Plan & Test Strategy

### 6.1 Unit Test Specifications
Execute targeted unit tests in [`benchmarks/wrk2/loadgen_test.go`](file:///Users/sneha/Developer/toron-research/toron/benchmarks/wrk2/loadgen_test.go):
```bash
go test -v -race -run TestLoadGen_StatusClassification_FourTiers ./benchmarks/wrk2/...
go test -v -race -run TestLoadGen_RouteMissFailsVerdict ./benchmarks/wrk2/...
go test -v -race -run TestLoadGen_AttackBypassFailsVerdict ./benchmarks/wrk2/...
go test -v -race -run TestLoadGen_UnhandledAnomalyFailsVerdict ./benchmarks/wrk2/...
go test -v -race -run TestLoadGen_ReportGeneration_Table6 ./benchmarks/wrk2/...
go test -v -race -run TestLoadGen_RaceFreeConcurrency ./benchmarks/wrk2/...
```

### 6.2 Integration Benchmark Execution
Execute the full saturation benchmark harness with attack injection:
```bash
go test -v -race -run TestLoadGen_AdversarialInjection ./benchmarks/wrk2/...
go test -v -race -run TestLoadGen_ReportGeneration ./benchmarks/wrk2/...
```

### 6.3 End-to-End Shell Harness Verification
Verify script execution and artifact generation:
```bash
bash benchmarks/wrk2/run_saturation_stress.sh --auto-start -r 2000 -c 20 -d 3s -a 0.10
```

Assert that:
1. `saturation_stress_report.json` contains `active_defense_requests`, `route_miss_requests`, `bypassed_requests`, `unhandled_requests`.
2. `saturation_stress_report.md` formats Section 4 with Table 6 columns.
3. Exit status is 0 when all probes are actively rejected (once [`TASK-139`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-139.md) is applied).

---

## 7. Traceability Matrix

| Relationship | Identifier | Description |
| :--- | :--- | :--- |
| **Derived From** | [`REQ-117`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-117.md) | Requirement: Disaggregated Adversarial Status Classification and Route-Miss Separation (HARN-01) |
| **Derived From** | `ReviewTaskSummary.md` | Directive `REV-03` / Task `HARN-01`: Benchmark circular scoring anomaly remediation |
| **Derived From** | `AER-001.md` | Associate Editor Report (Paper 1): Peer review critique on benchmark validation rigour |
| **Derived From** | `AER-002.md` | Associate Editor Report (Paper 2): Lines 312–318 (Route Miss vs Active Defense Conflation) |
| **Depends On** | [`TASK-134`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-134.md) | Bounded Worker Pool Connection Dispatching in Reactor (`REQ-111`) |
| **Depends On** | [`TASK-139`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-139.md) | Layered Route-Aware Path Traversal Defense Architecture (`REQ-116`) |
| **Implements** | [`REQ-117`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-117.md) | Disaggregated Status Classification in Benchmark Load Generator |
| **Decided By** | `ADR-117` | Architectural Decision Record: Status Classification Disaggregation and Route-Miss Separation |
| **Verified By** | `TC-117` | Test Specification: Unit and Integration Oracles for Load Generator Status Disaggregation |
| **Related To** | `CR-113` | Code Review: Benchmark Load Generator Status Classification |
| **Related To** | `SR-117` | Security & Empirical Review: Table 6 Benchmark Alignment & Invariant Scoring Audit |
| **Related To** | [`REQ-114`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-114.md) | High-Concurrency Saturation Stress Testing with Background Traffic (BMK-04) |
| **Related To** | [`REQ-116`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-116.md) | Layered Route-Aware Path Traversal Defense Architecture |
