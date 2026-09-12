---
id: TASK-139
type: task
title: Implementation of Layered Route-Aware Path Traversal Defense Architecture (CWE-22)
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - TASK-067
  - TASK-125
  - TASK-130

derived_from:
  - REQ-116

implements:
  - REQ-116

verified_by:
  - TC-116

decided_by:
  - ADR-116

related_to:
  - CR-112
  - SR-116
---

# TASK-139 - Implementation of Layered Route-Aware Path Traversal Defense Architecture (CWE-22)

## 1. Description & Context

Decompose and coordinate the engineering implementation of the Layered Route-Aware Path Traversal Defense Architecture specified in [`REQ-116`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-116.md) and [`ADR-116`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-116.md).

### 1.1 Problem Background
In automated differential protocol security benchmarking (`benchmarks/results/differential_fuzz_report.md`), Toron achieved an 89.47% pass rate (17/19 invariant tests), but failed two critical path traversal security test vectors:
- `TRAVERSAL-001`: Raw Dot-Dot Path Traversal Sequence (`GET /internal/dashboard/../../canary_traversal.txt HTTP/1.1`)
- `TRAVERSAL-002`: Uppercase Percent-Encoded Traversal (`GET /internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt HTTP/1.1`)

Both test cases resulted in `404 Not Found` with an open keep-alive connection (`Connection: Open`) instead of the required active defensive rejection (`403 Forbidden` with `Connection: close`).

### 1.2 Root Cause Analysis
1. **Router Premature Path Canonicalization & Dead-Code Contradiction (`pkg/router/router.go:789`)**:
   - `Router.ServeHTTP` unconditionally executes `req.Path = cleanRequestPath(req.Path)` before route matching or middleware invocation.
   - For an ingress path like `/internal/dashboard/../../canary_traversal.txt`, `cleanRequestPath` collapses the dot-dot segments into `/canary_traversal.txt`.
   - The prefix route registered at `/internal/dashboard` fails to match `/canary_traversal.txt`, causing the router to dispatch to `r.NotFound` (`404 Not Found`).
   - Consequently, the explicit traversal check inside `createStaticHandler` (`pkg/router/router.go:643-650`) is rendered dead code because escaped paths never reach the static handler.
2. **WAF Layer 7 Raw Wire URI Blind Spot (`pkg/waf/waf.go:313`)**:
   - `WAFEngine.InspectToron` assigns `urlPath := req.Path`. Because `req.Path` was sanitized by `cleanRequestPath`, the WAF inspects `/canary_traversal.txt`.
   - The WAF rule `TRAVERSAL-001` (`(?i)(\.\./|\.\.\\|%2e%2e/|%2e%2e%2f|%2e%2e\\|%2e%2e%5c)`) does not match `/canary_traversal.txt`.
   - `InspectToron` only consults `req.RequestURI` if `urlPath == ""`, creating an inspection bypass.

### 1.3 Target Architecture: Layered Defense-in-Depth
1. **Layer 7 WAF Raw Wire URI Inspection (`pkg/waf/waf.go`)**: Extract the raw wire path directly from `req.RequestURI` (stripping query string and fragment), and evaluate URL threat rules against both the raw wire path and the unescaped path (`url.PathUnescape`). Actively block with `403 Forbidden` and `Connection: close`.
2. **Route-Aware Static Prefix Escape Guard (`pkg/router/router.go`)**: During route evaluation in `Router.ServeHTTP`, detect when an ingress raw or unescaped path targets a static route prefix but canonicalizes to a path escaping that prefix boundary. Actively reject with `403 Forbidden: Path Traversal Disallowed` and `Connection: close`.

---

## 2. Subtask Breakdown

### TASK-139.1: Layer 7 WAF Raw Wire URI Extraction and Dual-Path Pattern Evaluation
- **Component**: `pkg/waf/waf.go`
- **Scope**:
  - In `WAFEngine.InspectToron(req *httpparser.Request)`:
    - Extract the raw wire path directly from `req.RequestURI` if present:
      - Strip any query string (delimited by `?`) and URL fragment (delimited by `#`) from `req.RequestURI` to isolate the raw wire path component.
      - Fallback to `req.Path` only if `req.RequestURI` is empty.
    - Preserve query string extraction from `req.URL.RawQuery` or `req.RequestURI`.
  - In `WAFEngine.inspectInternal(method, urlPath, rawQuery, headersStr, bodyReader, restoreBody)`:
    - For all rules configured with `Locations & InspectURL != 0`:
      - Evaluate `rule.Pattern.MatchString` against:
        1. The raw wire URL path (`urlPath`, preserving `%2e%2e`, `/../`, `%2E%2E`, etc.).
        2. The unescaped/normalized URL path (`normPath = url.PathUnescape(urlPath)`).
      - If either matches, register the rule match and accumulate the threat score.
  - In `WAFEngine.InspectToron` / `WAFMiddleware`:
    - On anomaly score meeting or exceeding `AnomalyThreshold` in `enforce` mode:
      - Set HTTP status code `403 Forbidden`.
      - Set header `Connection: close` (enforcing transport socket teardown per `REQ-107`).
      - Set header `Content-Type: application/json`.
      - Write payload using `FormatBlockedResponse(score, matched)`.
      - Record structured audit log event `waf_block`.

### TASK-139.2: Route-Aware Static Prefix Escape Guard in Router Dispatch Layer
- **Component**: `pkg/router/router.go`
- **Scope**:
  - In `Router.ServeHTTP`:
    - Before delegating to `r.NotFound` when exact route lookup and prefix matching fail:
      - Inspect all registered prefix routes where `routeType == string(RouteTypeStatic)`.
      - Extract the raw incoming request path from `req.RequestURI` (stripping query string and fragment). If empty, use pre-canonicalized `req.Path`.
      - Compute the unescaped path candidate using `url.PathUnescape(rawPath)`.
      - For each static prefix route with prefix $P$ (e.g. `/internal/dashboard`):
        - Normalize prefix $P$ (trimming trailing `/` for uniform matching, e.g. `/internal/dashboard`).
        - Check if the incoming path (either `rawPath` or unescaped candidate) targeted prefix $P$:
          - Specifically: `strings.HasPrefix(rawPath, P+"/") || rawPath == P` OR `strings.HasPrefix(unescaped, P+"/") || unescaped == P`.
        - Check if the canonicalized path `cleanRequestPath(rawPath)` escapes prefix $P$:
          - Specifically: `!strings.HasPrefix(canonicalPath, P+"/") && canonicalPath != P`.
        - If both conditions hold, classify the request as a **Static Route Prefix Boundary Escape Attempt**.
    - On detecting a prefix boundary escape:
      - Immediately terminate request dispatching without invoking `r.NotFound`.
      - Set HTTP status code `403 Forbidden` (`http.StatusForbidden`).
      - Set response header `Connection: close`.
      - Set response header `Content-Type: application/json`.
      - Write response body: `{"error":"403 Forbidden: Path Traversal Disallowed"}`.
      - Record metric telemetry in `metrics.DefaultRegistry`.
      - Return immediately to invoke socket teardown.
  - This eliminates the dead-code contradiction in `createStaticHandler` and guarantees active defensive rejection even if the WAF engine is disabled or omitted.

### TASK-139.3: Non-Static Route Canonicalization & ADR-062 Invariant Preservation
- **Component**: `pkg/router/router.go`
- **Scope**:
  - Guarantee that reverse proxy routes (`RouteTypeUpstream`) and standard exact API routes (`r.GET`, `r.POST`, etc.) continue to adhere to ADR-062 two-stage canonicalization.
  - Verify that legitimate requests with benign dot-dot segments that resolve within valid routes (e.g., `/api/v1/../v1/status` resolving to `/api/v1/status`) are routed correctly without false-positive escape rejections.
  - Ensure static route prefix matching only flags requests that deliberately target and escape registered static prefixes.

### TASK-139.4: Differential Security Fuzzer Oracle Realignment
- **Component**: `benchmarks/fuzzer/diff_fuzzer.go`, `benchmarks/fuzzer/diff_fuzzer_test.go`
- **Scope**:
  - In `benchmarks/fuzzer/diff_fuzzer.go` (`getTestCases`):
    - Update `TRAVERSAL-001` (`Raw Dot-Dot Path Traversal Sequence`):
      - Update `ExpectClose: true` (aligning with `REQ-107` and `REQ-116` fail-fast socket teardown specification).
    - Update `TRAVERSAL-002` (`Uppercase Percent-Encoded Traversal`):
      - Update `ExpectClose: true`.
    - Keep `TRAVERSAL-003` with `ExpectClose: true`.
  - In `benchmarks/fuzzer/diff_fuzzer_test.go`:
    - Ensure all mock test servers in unit tests (`TestExecuteRawTest_Traversal_Accepts403And400`, etc.) send `Connection: close` and close TCP sockets cleanly to reflect the updated `ExpectClose: true` oracle.

### TASK-139.5: WAF and Router Path Traversal Unit and Integration Test Suite
- **Component**: `pkg/waf/waf_test.go`, `pkg/router/router_test.go`
- **Scope**:
  - In `pkg/waf/waf_test.go`:
    - Add test cases verifying `WAFEngine.InspectToron`:
      - Ingress `req.RequestURI = "/internal/dashboard/../../canary_traversal.txt"` triggers rule `TRAVERSAL-001`, `score >= 5`, `blocked = true`.
      - Ingress `req.RequestURI = "/internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt"` triggers rule `TRAVERSAL-001`, `score >= 5`, `blocked = true`.
      - Ingress `req.RequestURI = "/internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt"` triggers rule `TRAVERSAL-001`, `score >= 5`, `blocked = true`.
      - Verify query string and fragment stripping: `"/internal/dashboard/../../canary_traversal.txt?foo=bar#section"` correctly triggers traversal detection.
      - Fallback to `req.Path` when `req.RequestURI` is empty.
  - In `pkg/router/router_test.go`:
    - Add `TestRouter_RouteAwareStaticPrefixEscapeGuard`:
      - Setup router with static route at `/internal/dashboard` mounting a temporary directory.
      - Test 1: `GET /internal/dashboard/../../canary_traversal.txt HTTP/1.1` -> assert status 403, `Connection: close`, JSON body.
      - Test 2: `GET /internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt HTTP/1.1` -> assert status 403, `Connection: close`, JSON body.
      - Test 3: `GET /internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt HTTP/1.1` -> assert status 403, `Connection: close`, JSON body.
      - Test 4: Valid subpath `GET /internal/dashboard/index.html HTTP/1.1` -> assert status 200, index content.
      - Test 5: Standalone router without WAF middleware attached -> assert all traversal attempts still return status 403 and `Connection: close`.
    - Add `TestRouter_ADR062_Canonicalization_NonStatic`:
      - Verify API and Upstream routes maintain clean canonicalization and do not trigger false-positive prefix escape blocks.
    - Run existing router test suite (`TestRouter_PathCanonicalizationAndTraversalGuards`, `TestRouter_SPA_SecurityPathTraversal`) to verify zero regressions.

### TASK-139.6: End-to-End Differential Fuzzer Execution & Benchmark Report Regeneration
- **Component**: `benchmarks/fuzzer/diff_fuzzer.go`, `benchmarks/results/`
- **Scope**:
  - Run the differential fuzzer against the live Toron edge gateway instance with static routes enabled:
    - Execute `bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -k 1000 -w 50`.
  - Verify that:
    - `TRAVERSAL-001` passes with status 403 and `Connection: Closed`.
    - `TRAVERSAL-002` passes with status 403 and `Connection: Closed`.
    - `TRAVERSAL-003` passes with status 403 and `Connection: Closed`.
    - Overall protocol security pass rate reaches **100.0% (19/19 tests)**.
    - Mean rejection latency remains $< 300\ \mu\text{s}$ and median latency remains $< 70\ \mu\text{s}$.
  - Regenerate `benchmarks/results/differential_fuzz_report.json` and `benchmarks/results/differential_fuzz_report.md`.
  - Execute `go test -race -count=1 ./...` across all packages to ensure zero race conditions.

---

## 3. Acceptance Criteria

- **AC-1 (Fuzzer Pass Rate)**: Differential security fuzzer reports **19/19 passed (100.0% pass rate)**. `TRAVERSAL-001` and `TRAVERSAL-002` pass with status 403 and physical socket closure (`Connection: Closed`).
- **AC-2 (WAF Wire Inspection)**: `WAFEngine.InspectToron` detects raw and percent-encoded traversal sequences directly from `req.RequestURI` and enforces `403 Forbidden` with `Connection: close` and audit logging.
- **AC-3 (Standalone Router Defense)**: Router prefix matching detects static prefix boundary escapes independently of WAF middleware and immediately emits `403 Forbidden: Path Traversal Disallowed` with `Connection: close`.
- **AC-4 (No Regressions)**: Legitimate static file requests, upstream proxy routes, and standard API endpoints function with zero false positives or routing regressions.
- **AC-5 (Zero External Dependencies)**: Implementation utilizes strictly pure Go standard library packages (`net/url`, `path`, `path/filepath`, `strings`, `bytes`).
- **AC-6 (Concurrency & Latency Budgets)**: All test suites pass cleanly under `go test -race -count=1 ./...` with zero race conditions. Mean fail-fast rejection latency remains under $300\ \mu\text{s}$.

---

## 4. Technical Constraints & Architecture Invariants

1. **Pure Go Standard Library**:
   - Zero external third-party dependencies. Only standard library modules (`net/url`, `path`, `path/filepath`, `strings`, `bytes`, `sync`) are permitted.
2. **Defense-in-Depth Layering**:
   - Both Layer 7 (WAF) and the routing layer (Router) must independently identify and reject static prefix escape attempts. The system must remain secure even if WAF middleware is omitted from the pipeline.
3. **Fail-Fast Transport Socket Teardown**:
   - Every path traversal rejection at both WAF and Router layers must inject `Connection: close` header to trigger immediate socket closure upon serialization per `REQ-107` and `TASK-130`.
4. **Dead-Code Elimination**:
   - The architectural dead-code contradiction where `cleanRequestPath` diverted traversal probes to `NotFound` before static boundary guards could execute is permanently resolved.
5. **Reader-Lock Thread Safety**:
   - All routing prefix evaluations and WAF rule inspections must execute under `sync.RWMutex` reader locks (`mu.RLock()`), ensuring concurrent request safety.

---

## 5. Security & Threat Modeling (CWE-22)

### CWE-22: Improper Limitation of a Pathname to a Restricted Directory ('Path Traversal')

| Threat Vector | Ingress Wire URI Pattern | Layer 1: WAF (Raw Wire URI) | Layer 2: Router (Prefix Escape Guard) | Layer 3: Static Handler (`filepath.Rel`) |
| :--- | :--- | :--- | :--- | :--- |
| **Raw Dot-Dot Climbing** (`TRAVERSAL-001`) | `GET /internal/dashboard/../../canary_traversal.txt` | **Blocked**: 403 Forbidden, `Connection: close` | **Blocked**: 403 Forbidden, `Connection: close` | Blocked: 403 Forbidden, `Connection: close` |
| **Uppercase Encoded Dot-Dot** (`TRAVERSAL-002`) | `GET /internal/dashboard/%2E%2E/%2E%2E/canary_traversal.txt` | **Blocked**: 403 Forbidden, `Connection: close` | **Blocked**: 403 Forbidden, `Connection: close` | Blocked: 403 Forbidden, `Connection: close` |
| **Double Percent Encoded** (`TRAVERSAL-003`) | `GET /internal/dashboard/%252e%252e/%252e%252e/canary_traversal.txt` | **Blocked**: 403 Forbidden, `Connection: close` | **Blocked**: 403 Forbidden, `Connection: close` | Blocked: 403 Forbidden, `Connection: close` |
| **Mixed / Nested Obfuscation** | `GET /internal/dashboard/..%2f%2e%2e/canary_traversal.txt` | **Blocked**: 403 Forbidden, `Connection: close` | **Blocked**: 403 Forbidden, `Connection: close` | Blocked: 403 Forbidden, `Connection: close` |
| **WAF Disabled / Bypassed** | `GET /internal/dashboard/../../canary_traversal.txt` | *Bypassed* | **Blocked**: 403 Forbidden, `Connection: close` | Blocked: 403 Forbidden, `Connection: close` |

---

## 6. Verification Plan & Test Strategy

### 6.1 Unit & Subsystem Tests
- `go test -v -race -run TestWAF_RawWireURI pkg/waf/...`
- `go test -v -race -run TestRouter_RouteAwareStaticPrefixEscapeGuard pkg/router/...`
- `go test -v -race -run TestRouter_ADR062_Canonicalization pkg/router/...`
- `go test -v -race ./pkg/router/... ./pkg/waf/...`

### 6.2 Differential Protocol Security Fuzzer
- `go test -v -race ./benchmarks/fuzzer/...`
- `bash benchmarks/fuzzer/run_fuzzer.sh -t 127.0.0.1:8080 -k 1000 -w 50`
- Assert that `benchmarks/results/differential_fuzz_report.json` records:
  - `"total_tests": 19`
  - `"passed_tests": 19`
  - `"failed_tests": 0`
  - `"security_pass_rate": 1.0`

### 6.3 Full Repository Race-Clean Validation
- `go test -race -count=1 ./...`

---

## 7. Traceability Matrix

| Relationship | Identifier | Description |
| :--- | :--- | :--- |
| **Derived From** | [`REQ-116`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-116.md) | Requirement specification for Layered Route-Aware Path Traversal Defense Architecture |
| **Derived From** | `differential_fuzz_report.md` | Protocol security benchmark identifying TRAVERSAL-001 & TRAVERSAL-002 404 failures |
| **Derived From** | `CWE-22` | MITRE CWE-22: Improper Limitation of a Pathname to a Restricted Directory ('Path Traversal') |
| **Depends On** | [`TASK-067`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-067.md) | Upstream Path Canonicalization & Route Traversal Guards (ADR-062) |
| **Depends On** | [`TASK-125`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-125.md) | Path Traversal Test Oracle Hardening and Deterministic Canary Enforcement |
| **Depends On** | [`TASK-130`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-130.md) | Connection Close on WAF and Router Security Rejections (`REQ-107`) |
| **Decided By** | [`ADR-116`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-116.md) | Architecture Decision Record: Layered / Route-Aware Path Traversal Defense Architecture |
| **Verified By** | [`TC-116`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-116.md) | Test Specification: Layer 7 WAF Wire URI Inspection & Static Prefix Escape Guards |
| **Implements** | [`REQ-116`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-116.md) | Implementation of Layered Route-Aware Path Traversal Defense Architecture |
| **Reviewed By** | `CR-112` | Code Review for WAF and Router Path Traversal Defenses |
| **Reviewed By** | `SR-116` | Security Review & CWE-22 Boundary Analysis |
