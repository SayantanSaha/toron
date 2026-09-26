---
id: TASK-169
type: task
title: Configuration Schema & Loader Extensions for Route-Scoped Ingress, Deadlines, and Bulkhead Limits
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-25
updated: 2026-09-25

depends_on:
  - ../requirements/REQ-145.md

derived_from:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md

implements:
  - ../requirements/REQ-145.md

related_to:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md
  - ../requirements/REQ-005.md
  - ../requirements/REQ-087.md
  - ../architecture/ADR-082.md
  - ../architecture/ADR-084.md
  - ./TASK-170.md
  - ./TASK-171.md
  - ./TASK-172.md
  - ./TASK-173.md
---

# TASK-169 - Configuration Schema & Loader Extensions for Route-Scoped Ingress, Deadlines, and Bulkhead Limits

## 1. Overview & Objective

Decompose the configuration requirements of [`REQ-145`](../requirements/REQ-145.md) and [`AN-006`](../analysis/AN-006.md) into concrete, testable extensions to Toron's configuration schema and loader.

Currently, Toron enforces static, global server ceilings (`max_body_bytes: 4MB`, `read_timeout: 5s`, `write_timeout: 5s`, `ResponseHeaderTimeout: 10s`). To support legacy enterprise workloads (such as 200MB+ file uploads and long-running database queries) without weakening global DoS protections, route definitions in `ProxyRouteConfig` ([`pkg/config/config.go`](../../pkg/config/config.go)) must be extended with route-scoped limits, safe defaults, accessor helpers, and validation in [`pkg/config/loader.go`](../../pkg/config/loader.go).

---

## 2. Traceability

- **Requirement**: [`REQ-145`](../requirements/REQ-145.md) (FR-145-2, FR-145-3, FR-145-4, FR-145-5, Section 6.1)
- **Analysis**: [`AN-006`](../analysis/AN-006.md) (Section 8.2, Item 3)
- **Downstream Tasks**:
  - [`TASK-170.md`](./TASK-170.md): Two-Phase HTTP Request Parsing in `pkg/httpparser`
  - [`TASK-171.md`](./TASK-171.md): Activity-Refreshed Read Deadline Tracker in `pkg/server`
  - [`TASK-172.md`](./TASK-172.md): Zero-Copy Request Body Proxy Streaming in `pkg/proxy`
  - [`TASK-173.md`](./TASK-173.md): Route Bulkhead Concurrency Limiter in `pkg/router`

---

## 3. Work Package Breakdown

### WP-1: Schema Extensions in `ProxyRouteConfig` ([`pkg/config/config.go`](../../pkg/config/config.go))

Extend `ProxyRouteConfig` with the following route-scoped fields:
```go
type ProxyRouteConfig struct {
    // ... existing fields ...

    // MaxBodyBytes overrides the global server.max_body_bytes for this route.
    // A value of 0 indicates fallback to the global server ceiling (default: 4 MB).
    MaxBodyBytes int64 `yaml:"max_body_bytes,omitempty" json:"max_body_bytes,omitempty"`

    // MaxConcurrency bounds the maximum concurrent in-flight requests on this route.
    // 0 indicates that default safety guardrails or unconstrained handling apply.
    MaxConcurrency int `yaml:"max_concurrency,omitempty" json:"max_concurrency,omitempty"`

    // ReadTimeout overrides the global server.read_timeout for request body reading on this route.
    // 0 indicates fallback to the global server read timeout (default: 5s).
    ReadTimeout time.Duration `yaml:"read_timeout,omitempty" json:"read_timeout,omitempty"`

    // WriteTimeout overrides the global server.write_timeout for response writing on this route.
    // 0 indicates fallback to the global server write timeout (default: 5s).
    WriteTimeout time.Duration `yaml:"write_timeout,omitempty" json:"write_timeout,omitempty"`

    // StreamRequestBody explicitly controls whether incoming request bodies stream directly upstream
    // without in-memory buffering. nil defaults to automatic streaming for payloads > 64 KB.
    StreamRequestBody *bool `yaml:"stream_request_body,omitempty" json:"stream_request_body,omitempty"`

    // ResponseHeaderTimeout overrides the proxy transport response header timeout (default: 10s).
    // Governs maximum time to wait for upstream backend to produce HTTP response headers.
    ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout,omitempty" json:"response_header_timeout,omitempty"`
}
```

### WP-2: Safe Default Resolution & Accessor Helpers ([`pkg/config/config.go`](../../pkg/config/config.go))

Implement defensive accessor methods on `ProxyRouteConfig`:
1. `GetMaxBodyBytes(defaultVal int64) int64`:
   - Returns `p.MaxBodyBytes` if `p.MaxBodyBytes > 0`.
   - Otherwise, returns `defaultVal` (global `server.max_body_bytes`, default: `4,194,304` bytes).
2. `GetMaxConcurrency(workerPoolSize int) int`:
   - If `p.MaxConcurrency > 0`, return `p.MaxConcurrency`.
   - If `p.MaxConcurrency == 0` and route has elevated payload ceiling (`p.MaxBodyBytes > 4*1024*1024`) or elevated backend timeout (`p.ResponseHeaderTimeout > 10*time.Second`), apply the approved safety guardrail from [`REQ-145`](../requirements/REQ-145.md) Section 6.1:
     $$\text{safeGuardrail} = \min(32, \max(1, \text{workerPoolSize} / 4))$$
     (e.g., 32 on a 128-worker gateway).
   - Otherwise, return `0` (unconstrained route).
3. `GetReadTimeout(defaultVal time.Duration) time.Duration`:
   - Returns `p.ReadTimeout` if `p.ReadTimeout > 0`.
   - Otherwise, returns `defaultVal`.
4. `GetWriteTimeout(defaultVal time.Duration) time.Duration`:
   - Returns `p.WriteTimeout` if `p.WriteTimeout > 0`.
   - Otherwise, returns `defaultVal`.
5. `GetResponseHeaderTimeout(defaultVal time.Duration) time.Duration`:
   - Returns `p.ResponseHeaderTimeout` if `p.ResponseHeaderTimeout > 0`.
   - Otherwise, returns `defaultVal` (proxy default: `10 * time.Second`).
6. `ShouldStreamRequestBody(contentLength int64) bool`:
   - If `p.StreamRequestBody != nil`, return `*p.StreamRequestBody`.
   - If `p.StreamRequestBody == nil` (auto), return `true` if `contentLength > 65536` (64 KB) or `contentLength == -1` (chunked streaming); return `false` if `contentLength >= 0 && contentLength <= 65536`.

### WP-3: Validation and Normalization ([`pkg/config/loader.go`](../../pkg/config/loader.go))

In `ValidateConfig`:
1. Iterate over all configured proxy routes (`cfg.Proxy.Routes`).
2. Validate that:
   - `route.MaxBodyBytes >= 0` (negative values return a descriptive configuration error).
   - `route.MaxConcurrency >= 0` (negative values return a configuration error).
   - `route.ReadTimeout >= 0` (negative duration returns a configuration error).
   - `route.WriteTimeout >= 0` (negative duration returns a configuration error).
   - `route.ResponseHeaderTimeout >= 0` (negative duration returns a configuration error).
3. Ensure YAML and JSON serialization/deserialization preserves these fields without truncation or parsing errors.

### WP-4: Unit Tests & Verification ([`pkg/config/config_test.go`](../../pkg/config/config_test.go), [`pkg/config/loader_test.go`](../../pkg/config/loader_test.go))

1. Test YAML unmarshaling of route configuration blocks with all new fields.
2. Test `GetMaxBodyBytes` fallback to global ceiling when unspecified.
3. Test `GetMaxConcurrency` automatic safety guardrail derivation (`min(32, WorkerPoolSize/4)`) when route specifies elevated `max_body_bytes: 256MB` without `max_concurrency`.
4. Test `GetMaxConcurrency` returning explicit value when configured.
5. Test `ShouldStreamRequestBody` auto-activation for payloads $> 64\,\text{KB}$ and chunked transfers, and explicit override honoring.
6. Test rejection of negative limits in `ValidateConfig`.

---

## 4. Acceptance Criteria

- [ ] **AC-169-1 (Field Deserialization)**: YAML route blocks containing `max_body_bytes`, `max_concurrency`, `read_timeout`, `write_timeout`, `stream_request_body`, and `response_header_timeout` parse correctly into `ProxyRouteConfig`.
- [ ] **AC-169-2 (Hierarchical Ceiling Fallback)**: When `max_body_bytes` is omitted on a route, `GetMaxBodyBytes(4194304)` returns `4194304`. When specified (e.g. `268435456`), `GetMaxBodyBytes` returns `268435456`.
- [ ] **AC-169-3 (Automatic Bulkhead Guardrail)**: When a route defines `max_body_bytes: 268435456` or `response_header_timeout: 30s` with omitted `max_concurrency`, `GetMaxConcurrency(128)` automatically defaults to `32`.
- [ ] **AC-169-4 (Explicit Bulkhead Override)**: When a route explicitly defines `max_concurrency: 8`, `GetMaxConcurrency(128)` returns `8`.
- [ ] **AC-169-5 (Automatic Streaming Ingress Heuristic)**: `ShouldStreamRequestBody` returns `true` for `contentLength = 70000` and `contentLength = -1` (chunked) when `StreamRequestBody` is `nil`, and respects explicit `false` overrides.
- [ ] **AC-169-6 (Negative Value Validation)**: `ValidateConfig` returns explicit errors if any timeout, body limit, or concurrency ceiling is negative.
- [ ] **AC-169-7 (Backward Compatibility)**: Existing route configuration files without these new fields load and validate without modification or regression.

---

## 5. Constraints & Standards

- **Strictly Relative Links**: All internal document links must use relative syntax (e.g. `../requirements/REQ-145.md`, `../../pkg/config/config.go`).
- **Zero External Dependencies**: Use standard Go library packages (`time`, `strings`, `fmt`) and existing `gopkg.in/yaml.v3`.
