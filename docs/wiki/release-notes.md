# Release Notes

## 2026-09-23 - Toron v1.5.35 Milestone (Disaggregated Upstream Route Pool Rendering, Hierarchical Subpath Rollup & Live Telemetry Metric Derivations in Control Center Dashboard)

### Milestone Summary
- **Distinct Upstream Route Pool Disaggregation**:
  - Eliminated the upstream route collapse anti-pattern in the Control Center dashboard (`public/app.js`) where multiple proxy routes targeting identical backend socket endpoints (e.g. `127.0.0.1:8080`) were absorbed into the first declared route's pool card.
  - Implemented route-first priority matching in `buildDataModel()`: health check probe entries (`rawApiUpstreams`) from `/internal/api/upstreams/health` are matched to configured routes prioritizing explicit route labels (`u.route`) matching route path, prefix, or host+path before falling back to target socket addresses.
  - Established deterministic route-scoped pool keys (`poolKey = primaryRoute.id`), ensuring that every configured upstream route generates an independent pool card with its own metrics, load-balancing algorithm, and instance strips.
  - Guaranteed 100% upstream route coverage in `#/upstreams` via an exhaustive secondary fallback loop registering all remaining configured proxy routes with target endpoints.
  - Enforced full visual topology consistency across the Overview SVG Sankey traffic flow diagram (`flow()`) and the Routes table (`rtUpdate()`), ensuring discrete route ribbons connect to distinct upstream pool nodes.
- **Hierarchical Subpath Rollup in `requests_by_route`**:
  - Implemented `getRouteTotalReqs(pfx)` aggregating cumulative request counters across child subpaths (e.g. `/kite/api/v1/business` under `/kite/api`), ensuring granular endpoint traffic accurately rolls up to parent route prefixes.
- **Internal Telemetry Polling Isolation**:
  - Computed `totalExternalRequests` by explicitly filtering out `/internal/*` routes from the traffic denominator, preventing 1-second background dashboard health and metrics polling from diluting external user route throughput shares.
- **Moving Average Temporal Smoothing for Instantaneous RPS**:
  - Applied a trailing 5-point moving average over `timeseries.A.rps` (`effectiveTotalRps`) to smooth 1-second sampling discretisation jitter and transient spikes across route throughput distributions.
- **Dual Rate and Cumulative Volume Display in Upstream Pool Cards**:
  - Enhanced `poolCard()` to render both real-time throughput rate and cumulative request volume in the Requests KPI tile (e.g. `0.0/s (1.7k total)`), providing operators with immediate dual-dimension operational context distinguishing dormant routes from unexercised ones.
- **Live Telemetry Schema Alignment & Proportional RPS Derivation**:
  - Replaced obsolete and non-existent property lookups in `public/app.js` with active backend fields exported by `pkg/metrics/metrics.go` and `pkg/server/internal_api.go` (`requests_by_route`, `total_requests`, `timeseries.A.rps`, `timeseries.A.p95`, `timeseries.A.err`, and `/internal/api/upstreams/health` `latency_ms`, `http_code`, `status`, `history`).
  - Derived instantaneous route-level throughput (`curRPS`) proportionally from the smoothed rolling ring buffer rate (`effectiveTotalRps`) scaled by route cumulative request share ($\text{routeCumulative} / \text{totalExternalRequests}$), displaying live requests/s rather than static `0.0/s`.
  - Aggregated pool-level Requests metric as the sum of instantaneous RPS across all routes assigned to the pool.
- **Dynamic Probe Latency & Percentile Derivation**:
  - Bound instance card latency directly to real probe round-trip measurements (`latency_ms`) returned by `/internal/api/upstreams/health`.
  - Dynamically derived pool summary p95 latency from active instance probe latencies (`Math.max(...activeProbeLats)`), route percentiles, or gateway time-series (`tsA.p95[last]`), eliminating hardcoded `2.5 ms` fallbacks.
- **Multidimensional Health Classification & Real-Time Error Rates**:
  - Classified instance health states into `down` (unreachable, down, or HTTP >= 500), `warn` (degraded or 4xx responses), and `up` (healthy, HTTP < 400).
  - Derived pool 5xx error rate dynamically from the maximum of route error averages and 48-tick probe failure rates ($\sum\text{failed\_ticks} / \sum\text{total\_ticks}$), eliminating static `0.0%` defaults.
  - Automated visual tone escalation (`err`, `warn`, `ok`) to immediately signal failing nodes and circuit breaker activations.

### Changed
- **`public/app.js`**: Refactored `buildDataModel()`, `poolCard()`, `flow()`, and `rtUpdate()` to implement route-first probe matching, distinct pool key generation, exhaustive proxy route instantiation, hierarchical subpath rollup, internal telemetry polling isolation, trailing 5-point moving average RPS smoothing, dual rate and cumulative volume display, live probe latency extraction, dynamic p95 derivation, and multi-tier health tone classification.
- **`docs/wiki/features/observability-dashboard.md`**: Updated user documentation with detailed architectural sections on disaggregated upstream route pool rendering, hierarchical subpath rollup, telemetry polling isolation, moving average rate smoothing, dual rate/volume metrics, and probe health evaluation formulas.

---

## 2026-09-21 - Toron v1.5.34 Milestone (Native Dynamic 2-Stage WAF Auto-Ban Engine, Atomic File Persistence & Multi-Tier OS Firewall Defense - REQ-136 / TASK-159)

### Milestone Summary
- **Native Dynamic 2-Stage Auto-Ban Engine (REQ-136, TASK-159, ADR-136, TC-136, CR-136, SR-136)**:
  - Implemented progressive 2-stage banning: Stage 1 applies a temporary ban (`ban_duration`, e.g. 1h) upon reaching `max_violations` within sliding `window`; Stage 2 escalates repeat offenders to a **Permanent Ban** after $X$ temporary bans (`max_temporary_bans`).
  - Fast-path $\mathcal{O}(1)$ rejection ($< 1\mu\text{s}$) drops banned traffic immediately before regex evaluation or upstream proxying.
  - CIDR allowlist (`whitelist`) with guaranteed immunity for loopback (`127.0.0.1`, `::1`) and private management subnets.
- **Atomic State File Persistence**:
  - Persistent disk serialization to JSON (`banned_ips.json`) using atomic temporary file write + sync + rename pattern (`.tmp` $\to$ target).
  - Survives server reboots and gateway restarts without losing permanent bans or active temporary bans.
- **Observability Dashboard & REST Management**:
  - Added "Dynamic 2-Stage Auto-Ban & Blocked IPs" control section to dashboard with real-time TTL countdowns and 1-click Unban action.
  - Added `/internal/api/security/banned-ips`, `/internal/api/security/unban`, and `/internal/api/security/ban`.
- **Multi-Platform OS-Level IP Blocking Guide**:
  - Created comprehensive recipes for kernel packet drops: Linux (Fail2ban + iptables/nftables), macOS (`pfctl` packet filter), and Windows (PowerShell + Windows Defender Firewall).

### Added
- **`pkg/waf/auto_ban.go`**: Progressive 2-stage auto-ban engine, atomic state persistence, and background expiration sweeper.
- **`pkg/waf/auto_ban_test.go`**: Full unit test suite verifying strike tracking, stage 1/2 escalation, allowlists, persistence reload, and concurrent stress.
- **`docs/wiki/features/os-level-ip-blocking.md`**: Guide for Linux (Fail2ban), macOS (pfctl), and Windows (Defender Firewall).

### Changed
- **`pkg/waf/waf.go` & `pkg/waf/middleware.go`**: Integrated auto-ban engine into WAF lifecycle with fast-path checking and violation recording.
- **`pkg/server/internal_api.go`**: Added endpoints for `/internal/api/security/banned-ips`, `/internal/api/security/unban`, and `/internal/api/security/ban`.
- **`public/app.js`**: Added Banned IPs live table and unban/ban management modal in the Alerts view.
- **`docs/wiki/reference/api.md` & `docs/wiki/index.md`**: Updated API reference and navigation index.

---

## 2026-09-21 - Toron v1.5.33 Milestone (Next-Generation High-Density Observability Dashboard and Zero-Allocation Telemetry Time-Series Engine - REQ-135 / TASK-158)

### Milestone Summary
- **Next-Generation Observability Control Center (REQ-135, TASK-158, ADR-135, TC-135, CR-135, SR-135)**: Delivered a high-density, professional observability dashboard at `/internal/dashboard/` featuring pure vector SVG graphics, an interactive Sankey traffic flow diagram, real-time request waterfalls, and slide-over inspector drawers with zero external JavaScript dependencies.
- **Zero-Allocation 60-Bucket Time-Series Ring Buffer**: Implemented `TimeSeriesCollector` in `pkg/metrics/timeseries.go` tracking rolling metrics for RPS, latency percentiles (p50/p95/p99), 4xx/5xx error volumes, and Go runtime stats (Goroutines, Heap MB, GC pause p99, CPU%, Open FDs).
- **Circular Request Trace Stream (300 Entries)**: Added `GlobalTraceBuffer` and `GET /internal/api/logs` exposing live request traces with detailed span breakdowns.
- **48-Tick Upstream Health Histories**: Added continuous probe history tracking to populate visual health status bars in the Upstreams view.
- **Strict DOM XSS Prevention (CWE-79)**: Enforced contextual HTML escaping (`escapeHTML`) on all dynamic UI template bindings.

### Added
- **`pkg/metrics/timeseries.go`**: 60-bucket rolling time series collector and snapshotting engine.
- **`pkg/metrics/timeseries_test.go`**: Unit tests verifying ring buffer capacity wrapping and periodic sampling.
- **`public/index.html`**: Modern high-density layout with navigation rail, drawer, and IBM Plex fonts.
- **`public/style.css`**: CSS variables, dark/light themes, and pure-SVG chart styling.
- **`public/app.js`**: Vector chart engine, 7 dashboard views, API test debugger, and data adapter.
- **`docs/wiki/features/observability-dashboard.md`**: User documentation for the gateway dashboard.

### Changed
- **`pkg/server/internal_api.go`**: Enriched `/internal/api/status` and `/internal/api/metrics` with time-series and certificate metadata, added 48-tick upstream health histories, and added `GET /internal/api/logs`.
- **`docs/wiki/reference/api.md`**: Documented new API response structures and logs endpoint.

---

## 2026-09-17 - Toron v1.5.32 Milestone (RFC 9111 Cache Session Boundary Isolation, Application Path Confusion Scope, and Comprehensive Host Port Routing Invariants - REQ-134 / TASK-157)

### Milestone Summary
- **Disambiguation of Route Table Matching & Origin Authority Derivation (REQ-134, TASK-157, ADR-134, TC-134, CR-134, SR-134)**: Resolved architectural conflation between route dispatching flexibility and origin cache identity. Toron adopts a **Decoupled Dual-Track Processing Architecture**, preserving wildcard-port domain routing for general dispatch while strictly enforcing explicit host-and-port authority in cache keys.
- **Cross-Port Cache Key Poisoning Elimination ([CWE-524](https://cwe.mitre.org/data/definitions/524.html))**: Implemented dedicated authority derivation function `extractCacheHostPort` in `pkg/router/cache.go`, constructing canonical primary cache keys:
  $$\text{CacheKey} = \text{req.Method} + \texttt{":"} + \text{extractCacheHostPort}(req) + \texttt{":"} + \text{uri} \, [ + \texttt{":ae="} + \text{AcceptEncoding} ]$$
  Preserves explicit network ports (`service.internal:8080` vs `service.internal:80`), mathematically guaranteeing that confidential payloads served on private administrative ports can never be leaked to unauthenticated clients querying public ports.
- **Disambiguated Route Table Matching & Precedence**:
  - Enhanced `headersAndHostMatch` in `pkg/router/router.go`: domain-only routes (`example.com`) act as port-wildcard matches, while port-qualified routes (`example.com:8080`) enforce strict port equality.
  - Resolved route shadowing by formalizing a **4-tier exact route precedence hierarchy** (Tier 1: Explicit port routes; Tier 2: Domain-only routes; Tier 3: Header-constrained wildcards; Tier 4: Hostless fallback routes) and refining prefix route specificity sorting.
- **Hardened IPv6 Bracket Literal Parsing**: Hardened both `extractCacheHostPort` and `extractHost` to safely parse bracketed IPv6 addresses (`[::1]:8080`, `[2001:0db8::1]:8443`), eliminating colon truncation bugs where IPv6 colons were incorrectly split as port delimiters.
- **RFC 9111 Shared Cache Session Boundary Isolation**:
  - Refuses unshared authenticated requests bearing `Authorization` headers (RFC 9111 §3.5).
  - Enforces dual-stage `Set-Cookie` and `Set-Cookie2` header stripping before storage and upon cache delivery ([CWE-384](https://cwe.mitre.org/data/definitions/384.html)).
  - Strictly enforces `Cache-Control: private`, `no-store`, and `no-cache` directives (RFC 9111 §5.2.2).
  - Exempts real-time streaming connections (`text/event-stream`, `X-Accel-Buffering: no`, upgraded sockets) (REQ-128).
- **Web Cache Deception & The Shared Responsibility Model**: Formally codified the physical boundary between edge transparent proxy caching and upstream application framework routing hygiene in `docs/wiki/features/response-caching.md`, detailing developer best practices for application frameworks.
- **Exhaustive 9 Routing Methods Verification**: Audited and confirmed host-port invariants across all 9 routing methods supported by Toron (Exact, Prefix, Domain/VHost, Header, Method, Reverse Proxy, Static File, K8s Ingress / Container Discovery, Multi-Port Gateway Listeners).
- **The 4 Non-Negotiable Invariants Preserved**:
  1. *Zero External Dependencies*: Pure Go standard library (`strings`, `net/http`, `sync`, `time`); `go.mod` untouched.
  2. *Core Reactor Modularity Preserved (ADR-001)*: Caching middleware and router dispatch logic operate exclusively on abstract `*httpparser.Request` and `*httpparser.Response` within `HandlerFunc`; physical sockets (`net.Conn`) are never accessed or wrapped.
  3. *Memory Boundedness & Minimal Footprint (ADR-030, ADR-129)*: Zero heap allocations on standard hostnames without ports; bounded storage clamped by `max_entries` and `max_payload_size`.
  4. *Zero Data Races under `go test -race ./...`*: 100% thread safety verified under Go's race detector across concurrent route dispatching, cache lookups, admissions, and evictions.
- **100% Verification across TC-134.1 to TC-134.12**: Validated all 12 formal test specifications in `TC-134` with 0 failures and 0 race warnings.

### Added
- **`pkg/router/cache.go`**:
  - `extractCacheHostPort(req *httpparser.Request) string`: Dedicated authority derivation preserving explicit ports, trimming whitespace, converting host to lowercase, and parsing bracketed IPv6 literals.
- **`pkg/router/router.go`**:
  - `hasExplicitPort(host string) bool`: Detects explicit port specifications outside bracketed IPv6 addresses.
  - `extractFullHostPort(req *httpparser.Request) string`: Extracts canonical full host-and-port authority from request headers.
- **`pkg/router/export_test.go`**:
  - Export shims `HasExplicitPort`, `ExtractFullHostPort`, `ExtractHost`, and `ExtractCacheHostPort` for unit test verification.
- **`pkg/router/cache_test.go`**:
  - `TestCache_HostPortIsolation` (`TC-134.1`): Cross-port cache isolation integration test (`:8080` vs `:9090`).
  - `TestCache_HostPortKeyDerivation` (`TC-134.2`): 18 table vectors evaluating `extractCacheHostPort`.
  - `TestCache_IPv6HostPortIsolation` (`TC-134.3`): IPv6 cross-port isolation (`[::1]:8080` vs `[::1]:8443` vs `[::1]`).
  - `TestCache_DualStageSetCookieStripped` (`TC-134.4`): Dual-stage cookie stripping at storage and delivery.
  - `TestCache_RFC9111_AuthorizationBoundary` (`TC-134.5`): Authorization refusal without `public` directive.
  - `TestCache_RFC9111_OriginDirectivesEnforcement` (`TC-134.6`): Enforcement of `private`, `no-store`, and `no-cache`.
  - `TestCache_StreamingCacheExemption` (`TC-134.7`): Real-time streaming cache bypass.
  - `TestCache_ConcurrentHostPortAccess_RaceClean` (`TC-134.12`): High-concurrency stress test (20 workers, 1,000 requests) under `-race`.
- **`pkg/router/router_test.go`**:
  - `TestRouter_HostPortDisambiguation` (`TC-134.8`): Port-specific vs domain-only route dispatching.
  - `TestRouter_RoutePrecedence_HostPortOverDomain` (`TC-134.9`): Explicit host:port precedence over domain fallbacks.
  - `TestRouter_IPv6HostMatchingAndPortStripping` (`TC-134.10`): IPv6 matching and port stripping without colon mangling.
  - `TestRouter_AllNineRoutingMethods_HostPortInvariants` (`TC-134.11`): Exhaustive audit across all 9 routing methods.

### Changed
- **`pkg/router/cache.go`**:
  - Replaced legacy `extractHost(req)` with dedicated `extractCacheHostPort(req)` at line 199.
  - Eliminated all usage of `extractHost` inside `cache.go`.
- **`pkg/router/router.go`**:
  - Hardened `extractHost` to handle bracketed IPv6 literals safely without string corruption.
  - Enhanced `headersAndHostMatch` to support dual-mode matching (port-qualified vs domain-only).
  - Implemented 4-tier exact route matching precedence hierarchy in `ServeHTTP`, resolving wildcard route shadowing.
  - Enhanced prefix route specificity sorting (Tier 2 host specificity prioritizes routes with explicit ports).
- **`docs/wiki/features/response-caching.md`**:
  - Updated title and header to reference RFC 9111 Shared Cache Session Boundary Isolation and Host:Port Cache Key Authority Derivation.
  - Documented primary cache key derivation formula and explicit port preservation rationale.
  - Added dedicated section "Web Cache Deception & The Shared Responsibility Model" and "Developer Best Practices for Application Frameworks".
  - Updated Troubleshooting & FAQ sections for cross-port isolation and path confusion defenses.
- **`docs/wiki/configuration.md`**:
  - Updated caching configuration section with Host:Port authority derivation, cross-port isolation, and virtual host routing interactions.
- **`docs/wiki/index.md`**:
  - Updated response caching feature entry and documentation milestone version to v1.5.32.

### Security Hardening (CWE-524, CWE-384, CWE-20)
- **Cross-Port Cache Key Poisoning & Information Exposure ([CWE-524](https://cwe.mitre.org/data/definitions/524.html))**: Preserving network ports in primary cache keys eliminates cross-port cache collisions across multi-tenant microservices, container sidecars, and multi-port listeners sharing a hostname.
- **Shared Cache Session Fixation & Credential Bleed ([CWE-384](https://cwe.mitre.org/data/definitions/384.html))**: Dual-stage `Set-Cookie` and `Set-Cookie2` header stripping purges cookie headers before storing responses in RAM and again before transmitting cache hits to downstream clients.
- **IPv6 Colon Mangling & Malformed Host Parsing ([CWE-20](https://cwe.mitre.org/data/definitions/20.html))**: Safely parses bracketed IPv6 literals (`[::1]:8080`), isolating IPv6 addresses from port delimiters and eliminating string truncation bugs.
- **Route Shadowing & Precedence Inversion ([CWE-20](https://cwe.mitre.org/data/definitions/20.html))**: Strict 4-tier exact route matching and Tier 2 prefix specificity sorting ensure explicit port routes and domain-specific routes are never shadowed by hostless wildcard fallbacks.
- **Denial-of-Service & Memory Exhaustion ([CWE-400](https://cwe.mitre.org/data/definitions/400.html))**: Cache storage is strictly bounded by `max_entries` and `max_payload_size` configurations with thread-safe capacity and TTL evictions under `sync.RWMutex`.
- **Web Cache Deception Defense (Shared Responsibility Model)**: Formally delineated edge transparent proxy guarantees from upstream application routing hygiene, providing clear developer best practices.

### Related Tasks & Documents
- `TASK-157`: Host Port Disambiguation in Route Matching and Cache Key Authority Derivation (RFC 9111 Session Boundary Isolation)
- `REQ-134`: RFC 9111 Cache Session Boundary Isolation, Application Path Confusion Scope, and Comprehensive Host Port Routing Invariants
- `ADR-134`: Host Port Disambiguation in Route Matching and Cache Key Authority Derivation Architecture
- `TC-134`: Test Case Specification for Host Port Disambiguation and RFC 9111 Session Boundaries
- `CR-134`: Code Review of Host Port Disambiguation in Route Matching and Cache Key Authority Derivation
- `SR-134`: Security Review of RFC 9111 Cache Session Boundary Isolation, Cross-Port Collision Prevention, and Host Port Routing Invariants
- Relevant Standards & CWEs: [CWE-524](https://cwe.mitre.org/data/definitions/524.html), [CWE-384](https://cwe.mitre.org/data/definitions/384.html), [CWE-20](https://cwe.mitre.org/data/definitions/20.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), RFC 9110 §4.2, RFC 9111 §2, RFC 9111 §3.5, RFC 9111 §5.2, RFC 9111 §8

## 2026-09-17 - Toron v1.5.31 Milestone (Inbound Chunked Transfer-Encoding Ingestion, Zero-Tolerance Wire Decoding, and Upstream Re-Framing Normalization - REQ-133 / TASK-156)

### Milestone Summary
- **Transition to Active Ingress Smuggling Firewall (REQ-133, TASK-156, ADR-133, TC-133, CR-129 / CR-133, SR-133)**: Evolved Toron from static HTTP 501 rejection (legacy `ADR-056` / `REQ-061`) into an Active Ingress Smuggling Firewall. Unlocks native support for streaming client uploads, third-party enterprise SaaS webhooks (GitHub, Stripe, Datadog), and drop-in reverse proxy replacement while fortifying backend microservices against HTTP Request Smuggling ([CWE-444](https://cwe.mitre.org/data/definitions/444.html)).
- **Zero-Tolerance Ingress Wire Decoding (RFC 9112 §7.1)**: Engineered a streaming finite-state machine (`ChunkedBodyReader` in `pkg/httpparser/chunked.go`) enforcing zero leniency at the edge boundary:
  - **Strict `1*HEXDIG` Grammar**: Validates that chunk size tokens contain exclusively ASCII hex characters (`0`–`9`, `a`–`f`, `A`–`F`). Rejects leading signs (`+`, `-`), leading/embedded whitespace, tabs, and `0x` prefixes with `HTTP 400 Bad Request`.
  - **Bounded Chunk Extension Clamping**: Limits chunk extensions to $\le 256\,\text{B}$ (`MaxChunkExtensionBytes`), rejecting overlong extensions and control characters (`0x00`–`0x1F`, `0x7F`) with `HTTP 400 Bad Request` to neutralize Denial-of-Service attacks ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)).
  - **Exact CRLF Boundaries**: Strictly enforces `\r\n` sequence delimiters via `io.ReadFull`. Bare linefeeds or corrupted delimiters immediately fail-close with `HTTP 400 Bad Request` and sever the TCP connection.
  - **Cumulative Body Bounding**: Monotonically tracks decoded bytes against `MaxBodyBytes` (default: 4 MB, or route-level override), immediately returning `HTTP/1.1 413 Payload Too Large` and terminating the TCP connection upon violation.
  - **RFC 9112 §7.1.2 Trailer Validation**: Clamps trailing headers to $\le 4\,\text{KB}$ (`MaxTrailerBytes`, returning `HTTP 431`) and enforces a strict blacklist of prohibited framing and routing headers (`Transfer-Encoding`, `Content-Length`, `Connection`, `Host`, `Keep-Alive`, `TE`, `Trailer`/`Trailers`, `Upgrade`, and `:` pseudo-headers) with `HTTP 400 Bad Request`.
- **Upstream Canonical Re-Framing Normalization (`"normalize"`, Default Profile)**: In `pkg/proxy/proxy.go`, Toron de-chunks incoming client streams at the edge into pooled memory, verifies exact body length $L$, strips the hop-by-hop `Transfer-Encoding` header, sets an authoritative `Content-Length: L` header, and forwards a clean, standard HTTP request upstream. Downstream microservices (Node.js `llhttp`, Python `uvicorn`/`h11`, Ruby `puma`, Go `net/http`) are **100% shielded** from chunked parsing bugs, delimiter desynchronizations, and request smuggling ([CWE-444](https://cwe.mitre.org/data/definitions/444.html)).
- **Canonical Passthrough Streaming Mode (`"passthrough"`)**: For high-volume streaming uploads where edge memory buffering is undesirable, Toron streams validated canonical chunks upstream with constant $O(1) \le 32\,\text{KB}$ memory. Deploys an `earlyCancelingReader` that cancels the upstream request context immediately upon client framing fault or disconnection.
- **Legacy 501 Rejection Preservation (`"reject"`)**: Preserves legacy `ADR-056` perimeter behavior (`HTTP/1.1 501 Not Implemented: Inbound chunked transfer encoding is disabled` and immediate socket severance) for ultra-hardened zero-trust deployments.
- **Fail-Closed Preflight Smuggling Guards**:
  - **Dual CL+TE Smuggling Rejection (RFC 9112 §6.3)**: Inspects incoming headers in `pkg/httpparser/parser.go`; if both `Content-Length` and `Transfer-Encoding` are present, Toron immediately rejects the request with `HTTP 400 Bad Request` and severs the TCP socket.
  - **Obfuscation Detection**: Rejects tabs following colons (`Transfer-Encoding:\tchunked`), null bytes, and non-chunked terminal codings with `HTTP 400 Bad Request`.
- **Bounded Socket Drainage & Keep-Alive Reuse on `Close()`**: Drains up to $64\,\text{KB}$ (`MaxDrainBytes`) within $100\,\text{ms}$ to preserve persistent TCP connections; if unconsumed bytes exceed limit or a syntax error occurred, the TCP socket is forcefully terminated (`conn.Close()`), mathematically preventing pipelined byte leakage into subsequent requests ([CWE-444](https://cwe.mitre.org/data/definitions/444.html)).
- **Hierarchical Priority Resolution**: Resolves operational profiles with route-level granularity: `Route-level override > Transport-level setting > Server default ("normalize")`, accompanied by route-specific `max_body_bytes` overrides.
- **The 4 Non-Negotiable Invariants Preserved**:
  1. *Zero External Dependencies*: Pure Go standard library implementation (`io`, `bufio`, `bytes`, `strconv`, `sync`); `go.mod` untouched.
  2. *Core Reactor Modularity Preserved (ADR-001)*: Reader wraps stream interfaces; event loop, epoll/kqueue workers, and socket lifecycle remain decoupled and untouched.
  3. *Memory Boundedness & $O(1)$ Footprint (ADR-129)*: Constant $O(1) \le 32\,\text{KB}$ streaming reader footprint; `sync.Pool` recycling for payloads $\le 64\,\text{KB}$.
  4. *Zero Data Races*: 100% race-free verified under `go test -race ./...`.
- **100% Verification across TC-133.01 to TC-133.19**: Verified all 19 test cases in `TC-133` and confirmed 961,853 fuzz mutations under `FuzzChunkFraming` with zero panics, crashes, or desynchronizations.

### Added
- **`pkg/httpparser/chunked.go`**:
  - `ChunkedBodyReader` struct and streaming finite-state machine (`stateChunkSize`, `stateChunkData`, `stateChunkCRLF`, `stateTrailerSection`, `stateDone`).
  - Strict `parseChunkSizeLine` enforcing RFC 9112 §7.1 `1*HEXDIG` grammar.
  - Chunk extension clamping (`MaxChunkExtensionBytes = 256`) and control character validation.
  - Strict CRLF delimiter enforcement via `io.ReadFull`.
  - Cumulative payload body bounding (`MaxBodyBytes` tracking with `ErrBodyTooLarge` -> HTTP 413).
  - RFC 9112 §7.1.2 trailer parsing, size bounding (`MaxTrailerBytes = 4096`), and forbidden header blacklist validation.
  - `Close()` method with bounded socket drainage ($\le 64\,\text{KB}$) and fail-fast TCP teardown.
- **`pkg/httpparser/chunked_test.go`**:
  - 8 comprehensive unit test suites covering single/multi-chunk streams, empty payloads, hex syntax errors, signed hex rejection, extension bounding, CRLF enforcement, body clamping, trailer validation, and socket drainage (`TC-133-01` through `TC-133-08`).
- **`docs/wiki/features/inbound-chunked-ingestion.md`**:
  - User-facing feature documentation detailing the Active Ingress Smuggling Firewall, operational profiles, wire validation rules, state machine diagrams, YAML configuration, and troubleshooting guide.

### Changed
- **`pkg/httpparser/parser.go`**:
  - Integrated fail-closed CL.TE smuggling guard rejecting dual `Content-Length` and `Transfer-Encoding` with `ErrBadRequest` (HTTP 400).
  - Added `validateTransferEncodingHeader` detecting tab-after-colon obfuscation, null bytes, and non-chunked terminal codings.
  - Integrated `ChunkedBodyReader` into `ParseRequest`, initializing streaming body reader when chunked encoding is detected.
  - Exported `GetBodyBuffer` and `PutBodyBuffer` for shared buffer pool recycling across packages.
- **`pkg/httpparser/parser_test.go`**:
  - Added unit test suites verifying CL.TE smuggling rejection (`TC-133-09`), obfuscation detection (`TC-133-10`), and transfer-encoding grammar validation (`TC-133-11`).
- **`pkg/httpparser/fuzz_test.go`**:
  - Aligned `FuzzChunkFraming` with `ChunkedBodyReader` streaming decoding and differential evaluation against Go standard library `http.ReadRequest` (`TC-133-19`).
- **`pkg/proxy/proxy.go`**:
  - Implemented upstream normalization pipeline (`"normalize"`) de-chunking client streams, calculating exact `Content-Length`, and stripping `Transfer-Encoding`.
  - Implemented canonical passthrough pipeline (`"passthrough"`) with `earlyCancelingReader` context abort on client disconnect or framing fault.
  - Implemented hop-by-hop header stripping and safe trailer header forwarding.
  - Added thread-safe `inboundChunkedMode` resolution and `modeMu sync.RWMutex`.
- **`pkg/proxy/proxy_test.go`**:
  - Added reverse proxy integration tests covering normalization (`TC-133-12`), passthrough streaming (`TC-133-13`), trailer forwarding (`TC-133-14`), and buffer pool concurrency under 50 parallel workers (`TC-133-15`).
- **`pkg/server/server.go` & `pkg/server/config.go`**:
  - Integrated `cr.SetCloser(conn)` linking physical TCP connection to reader for fail-fast teardown.
  - Added support for `InboundChunkedMode` (`"normalize"`, `"reject"`, `"passthrough"`) and preserved legacy HTTP 501 rejection in `"reject"` mode.
- **`pkg/server/server_test.go`**:
  - Added end-to-end TCP tests covering normalization keep-alive reuse (`TC-133-16`), legacy reject 501 socket teardown (`TC-133-17`), and route-level override hierarchy (`TC-133-18`).
- **`pkg/config/config.go` & `pkg/config/loader.go`**:
  - Extended `ServerConfig`, `ProxyTransportConfig`, and `ProxyRouteConfig` schemas with `InboundChunkedMode`.
  - Added `MaxBodyBytes` override to `ProxyRouteConfig`.
  - Added validation for `inbound_chunked_mode` values (`"normalize"`, `"reject"`, `"passthrough"`).
- **`cmd/toron/main.go`**:
  - Wired hierarchical priority resolution (`Route override > Transport > Server default`) into reverse proxy initialization.
- **`docs/wiki/configuration.md`**:
  - Documented `inbound_chunked_mode` in server, transport, and routes sections; added precedence hierarchy and YAML examples.
- **`docs/wiki/index.md`**:
  - Updated wiki index to v1.5.31 milestone and linked the new inbound chunked ingestion feature guide.

### Fixed
- **Inability to Ingest Streaming Uploads & Third-Party Webhooks**: Eliminated unconditional HTTP 501 rejection of chunked requests, allowing Toron to serve as a drop-in ingress gateway for streaming uploads and SaaS webhook providers.
- **HTTP Request Smuggling via Conflicting CL+TE ([CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Enforced fail-closed RFC 9112 §6.3 rejection with HTTP 400 and immediate socket teardown when both `Content-Length` and `Transfer-Encoding` are present.
- **Chunk Extension Denial-of-Service / Memory Bomb ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html))**: Bounded chunk extensions to $\le 256\,\text{B}$, neutralizing heap exhaustion attacks.
- **Unbounded Inbound Chunk Streams ([CWE-400](https://cwe.mitre.org/data/definitions/400.html))**: Enforced cumulative body byte bounding against `MaxBodyBytes`, immediately aborting oversized streams with HTTP 413.
- **Trailer Header Smuggling & Routing Hijack ([CWE-113](https://cwe.mitre.org/data/definitions/113.html), [CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Prohibited message framing and routing headers in trailers per RFC 9112 §7.1.2 and clamped trailer sections to $\le 4\,\text{KB}$.
- **Pipelined Byte Leakage on Keep-Alive Connections ([CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html))**: Implemented bounded socket drainage ($\le 64\,\text{KB}$) on `Close()` and forceful TCP teardown on overflow or error, preventing leftover bytes from poisoning subsequent keep-alive requests.
- **Heterogeneous Origin Chunk Parser Vulnerabilities**: In default `"normalize"` mode, converts external chunked streams to verified `Content-Length` requests, completely insulating backend microservices (Node.js, Python, Ruby, Go) from chunk deserialization bugs.

### Related Tasks & Requirements
- `REQ-133`: Inbound Chunked Transfer-Encoding Ingestion, Zero-Tolerance Wire Decoding, and Upstream Re-Framing Normalization
- `TASK-156`: Inbound Chunked Transfer-Encoding Ingestion, Zero-Tolerance Wire Decoding, and Upstream Re-Framing Normalization (RFC 9112 §7.1)
- `ADR-133`: Inbound Chunked Transfer-Encoding Ingestion, Zero-Tolerance Wire Decoding, and Upstream Re-Framing Normalization Architecture
- `TC-133`: Test Case Specification for Inbound Chunked Transfer-Encoding Ingestion and Wire Decoding
- `CR-129` / `CR-133`: Code Review of Inbound Chunked Transfer-Encoding Ingestion
- `SR-133`: Security Review of Inbound Chunked Transfer-Encoding Ingestion and Upstream Re-Framing Normalization
- Relevant Standards & CWEs: [CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-113](https://cwe.mitre.org/data/definitions/113.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), [CWE-20](https://cwe.mitre.org/data/definitions/20.html), [CWE-362](https://cwe.mitre.org/data/definitions/362.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), RFC 9112 §7.1, RFC 9112 §6.3, RFC 7230 §4.1

## 2026-09-17 - Toron v1.5.30 Milestone (Coverage-Guided Generative Fuzzing Engine, Native Go testing.F Differential Oracles, Protocol Regression Disambiguation, and Parser Hardening - REQ-132 / TASK-155)

### Milestone Summary
- **Resolution of the Circular Oracle Dilemma (REQ-132, TASK-155, ADR-132, TC-132, CR-128, SR-132)**: Eliminated the circular, self-referential evaluation oracle of static invariant testing (where each test case asserted its own hardcoded HTTP status codes) by deploying a non-circular differential evaluation oracle against Go's canonical standard library reference parser (`net/http.ReadRequest`).
- **Dual-Verification Taxonomy & Methodological Disambiguation**: Formalized the clear separation between two complementary testing regimes across the codebase, benchmark harnesses, and documentation:
  - **Paradigm A: Deterministic Protocol Invariant Regression Suite & Latency Profiler (`benchmarks/fuzzer/diff_fuzzer.go`)**: Retitled and dedicated to evaluating live TCP socket fail-fast rejection latencies ($T_{\text{reject}}$, Equation 7) across 19 curated historical CVE/RFC invariant attack vectors under repeated statistical trials ($K=1,000$ trials, $W=50$ warm-up runs).
  - **Paradigm B: Coverage-Guided Generative & Differential Fuzzing Engine (`pkg/httpparser/fuzz_test.go`)**: Engineered using Go 1.18+ native `testing.F` compiler basic-block edge-instrumentation to autonomously mutate unbounded HTTP byte streams, discover edge-case parsing ambiguities, and verify non-circular differential oracles against the Go standard library.
- **Four Native Go `testing.F` Fuzz Targets (`pkg/httpparser/fuzz_test.go`)**:
  - `FuzzParseRequest`: Raw byte stream mutation exploring request-line grammar, header extraction, and body size limits while certifying crash immunity and memory boundedness.
  - `FuzzDifferentialWithStdLib`: Dual-path differential comparison feeding identical byte streams to Toron's `httpparser.ParseRequest` and standard library `net/http.ReadRequest` to detect semantic desynchronizations and dangerous leniencies.
  - `FuzzHeaderGrammar`: RFC 7230 §3.2 header token grammar mutations, whitespace before colon (`Host : example.com`), obs-fold continuation lines, and control character injection.
  - `FuzzChunkFraming`: RFC 7230 §4.1 chunk framing, non-hex chunk lengths, oversized chunk extensions, and inbound chunked transfer-encoding rejection under `ADR-056`.
- **Four Non-Circular Differential & Invariant Semantic Oracles**:
  - **Oracle 1 (Crash & Panic Immunity)**: Enforces deferred panic recovery across all targets, mathematically asserting zero unhandled panics, slice bounds out of range, or nil pointer dereferences across arbitrary byte streams.
  - **Oracle 2 (Differential Desynchronization Guard & Dangerous Leniency Rule)**: If Go standard library rejects a request due to ambiguous or RFC-violating framing (`conflicting`, `multiple content-length`, `transfer-encoding`, `bad content-length`), Toron **MUST NEVER ACCEPT** the request. Permissible defensive divergences where Toron enforces stricter security bounds (e.g. 8KB header caps, 2KB query caps, control character filtering) are explicitly allowed.
  - **Oracle 3 (Framing Boundary Agreement)**: When both parsers accept valid HTTP/1.1 requests (`toronErr == nil && stdErr == nil`), asserts strict equality on HTTP `Method`, canonical `URL.Path`, and `ContentLength`.
  - **Oracle 4 (Execution Boundedness & Resource Clamp)**: Physically clamps input payload streams to $64\,\text{KB}$ ($65,536$ bytes), limits execution time to $\le 50\,\text{ms}$ per iteration to prevent ReDoS, and recycles pooled line and body buffers (`lineBufferPool`, `bodyBufferPool`).
- **Curated Seed Corpus Registration**: Pre-populated the generative fuzzer with 7 nominal RFC 7230 HTTP/1.1 requests and all 19 structural CVE invariant attack vectors from `diff_fuzzer.go` (`SMUGGLE-001..004`, `WHITESPACE-001..003`, `CONTROL-001..003`, `TRAVERSAL-001..003`, `RESOURCE-001..002`, `BASELINE-001`, `CACHE-001..003`).
- **Five Zero-Day Parser Hardenings Neutralized in `pkg/httpparser/parser.go`**:
  1. **Strict Protocol Version Validation (RFC 7230 §2.6, [CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Replaced loose prefix match `strings.HasPrefix(proto, "HTTP/1.")` with strict whitelist `proto != "HTTP/1.1" && proto != "HTTP/1.0"`, rejecting spoofed tokens like `HTTP/1.Chunk` or `HTTP/1.2`.
  2. **Empty `Transfer-Encoding:` Header Handling (ADR-056 / RFC 7230 §3.3.3, [CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Replaced string check `req.Header.Get("Transfer-Encoding") != ""` with presence check `len(req.Header.Values("Transfer-Encoding")) > 0`, catching empty `Transfer-Encoding:` headers and rejecting conflicting headers with HTTP 400 or inbound chunking with HTTP 501.
  3. **Bare CR / Bare LF In-Line Rejection (RFC 7230 §3.2, [CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Added `strings.ContainsAny(..., "\r\n")` in request line and header lines to unconditionally reject embedded bare CR or LF characters with HTTP 400.
  4. **RFC 7230 §3.2 Header Value Control Character Validation ([CWE-113](https://cwe.mitre.org/data/definitions/113.html), [CWE-117](https://cwe.mitre.org/data/definitions/117.html))**: Added byte-level validation loop in header values rejecting any character `(b < 0x20 && b != '\t') || b == 0x7F` with HTTP 400, preventing header injection and terminal log forging.
  5. **Exact Line Ending Stripping (`trimLineEnding`, [CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-436](https://cwe.mitre.org/data/definitions/436.html))**: Replaced greedy `strings.TrimRight(line, "\r\n")` with `trimLineEnding`, stripping exactly one `\r\n` or `\n` to prevent concealment of rogue carriage returns (such as `\r\r\n`).
- **Automated Generative Runner & Crash Isolator (`benchmarks/fuzzer/run_generative_fuzz.sh`)**: Built a dedicated bash CLI orchestrator with support for `-target`, `-fuzztime`, `-j`, `-m`, `--clean`, `--no-history`, crash artifact isolation into `benchmarks/results/fuzz/`, exact replay command generation (`go test -run=^Target$/CrashFile ./pkg/httpparser`), and dual-output reporting dynamically bound to `pkg/version`.
- **Sub-Second Routine CI Verification & Zero-Dependency Invariant**: Standard `go test -v ./pkg/httpparser/...` executes all seed corpus tests as rapid unit tests in $< 1.0\,\text{second}$ ($< 1.4\,\text{s}$ with race detector), preserving fast developer feedback while enabling deep fuzzing campaigns. Achieved $90,000\text{--}136,000\text{ mutations/sec/core}$ with zero third-party dependencies (`go.mod` untouched) and zero data races under `go test -race ./...`.
- **100% Verification across TC-132.1 to TC-132.12**: Audited and confirmed all 12 test cases in `TC-132` with 100% pass rate.

### Added
- **`pkg/httpparser/fuzz_test.go`**:
  - `FuzzParseRequest(f *testing.F)`: Raw byte stream fuzz target evaluating crash immunity and memory boundedness.
  - `FuzzDifferentialWithStdLib(f *testing.F)`: Differential fuzz target evaluating Toron vs Go standard library `net/http.ReadRequest`.
  - `FuzzHeaderGrammar(f *testing.F)`: Header token and value grammar fuzz target validating RFC 7230 §3.2.
  - `FuzzChunkFraming(f *testing.F)`: Chunk framing mutation target evaluating hex sizing, extension bounds, and inbound chunked rejection.
  - `seedCorpus`: Curated seed registry containing 7 nominal HTTP/1.1 requests and 19 structural CVE attack vectors from `diff_fuzzer.go`.
  - Differential semantic oracles: Oracle 1 (Crash Immunity), Oracle 2 (Desynchronization Guard), Oracle 3 (Boundary Agreement), and Oracle 4 (Resource Boundedness).
- **`benchmarks/fuzzer/run_generative_fuzz.sh`**:
  - Fully automated CLI execution orchestrator supporting `-target <name|all>`, `-fuzztime <duration>`, `-j <json_path>`, `-m <md_path>`, `--clean`, `--no-history`.
  - Crash artifact capture, minimization isolation into `benchmarks/results/fuzz/`, and reproducible standalone replay command generation.
  - Publication-grade dual-output report generation (`generative_fuzz_report.json` and `generative_fuzz_report.md`).
- **`benchmarks/results/generative_fuzz_report.json` & `generative_fuzz_report.md`**:
  - Structured and publication-grade evaluation artifacts reporting mutations evaluated, execution throughput, status, and oracle compliance matrix.

### Changed
- **`pkg/httpparser/parser.go`**:
  - Hardened protocol version parsing at line 151 enforcing strict whitelist `proto != "HTTP/1.1" && proto != "HTTP/1.0"`.
  - Hardened empty `Transfer-Encoding:` detection at lines 211–217 via `len(req.Header.Values("Transfer-Encoding")) > 0`.
  - Added bare CR and LF rejection at lines 141 and 172 using `strings.ContainsAny(..., "\r\n")`.
  - Added header value control character validation loop at lines 201–206 rejecting characters `(b < 0x20 && b != '\t') || b == 0x7F`.
  - Implemented `trimLineEnding` at lines 289–298 stripping exactly one `\r\n` or `\n` to prevent rogue CR concealment.
- **`pkg/httpparser/parser_test.go`**:
  - Added `TestParseRequest_ProtocolVersionValidation` testing exact version matching against invalid tokens.
  - Added `TestParser_InboundSmugglingGuard_Preserved` validating empty `Transfer-Encoding:` handling.
  - Added `TestParseRequest_BareCRLFRejection` validating rejection of embedded bare CR and LF characters across 6 fixtures.
  - Added `TestParseRequest_HeaderControlCharacters` validating rejection of non-printable control characters in header values.
- **`benchmarks/fuzzer/diff_fuzzer.go`**:
  - Retitled banner and header comments to "Toron Deterministic Protocol Invariant Regression Suite & Latency Profiler".
  - Disambiguated scope as wire-level Equation 7 rejection latency profiling ($K=1,000$ trials) over live TCP sockets.
- **`benchmarks/README.md`**:
  - Added Section 4.5 ("Methodological Disambiguation: Invariant Regression Suite vs. Generative Differential Fuzzing") contrasting Paradigm A and Paradigm B.
  - Documented CLI usage for `run_generative_fuzz.sh` alongside `run_fuzzer.sh`.
- **`docs/wiki/features/differential-fuzzer-metrics.md`**:
  - Updated title and overview to reflect the dual-paradigm verification architecture.
  - Added dedicated documentation for the Coverage-Guided Generative Fuzzing Engine, differential oracles, parser hardenings, and runner options.
- **`docs/wiki/index.md`**:
  - Updated wiki index metadata, dependencies, and navigation descriptions for protocol regression and generative fuzzing.

### Fixed
- **Circular Oracle Dilemma in Automated Fuzz Testing (REQ-132)**: Neutralized self-referential status assertions by evaluating mutated inputs against Go standard library `net/http.ReadRequest` as an independent ground truth.
- **HTTP/1.x Protocol Version Spoofing & Version Confusion ([CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Enforced strict RFC 7230 §2.6 version matching, rejecting malformed tokens like `HTTP/1.Chunk` or `HTTP/1.2` with `ErrUnsupportedProtocol`.
- **Empty `Transfer-Encoding:` Header Smuggling Bypass ([CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Ensured presence of `Transfer-Encoding` header field name is detected even when empty, preventing bypass of the conflicting `Content-Length` check and enforcing ADR-056 inbound chunked rejection.
- **Rogue Carriage Return Concealment via Greedy Trimming ([CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-436](https://cwe.mitre.org/data/definitions/436.html))**: Replaced greedy `strings.TrimRight(line, "\r\n")` with `trimLineEnding`, ensuring rogue embedded CRs (such as `\r\r\n`) are exposed and rejected with HTTP 400.
- **Header Value CRLF Injection and Terminal Log Forging ([CWE-113](https://cwe.mitre.org/data/definitions/113.html), [CWE-117](https://cwe.mitre.org/data/definitions/117.html))**: Validated header values byte-by-byte, rejecting non-printable control characters (`0x00..0x1F`, `0x7F`) with HTTP 400.
- **Bare CR / Bare LF Delimiter Desynchronization ([CWE-444](https://cwe.mitre.org/data/definitions/444.html))**: Enforced strict RFC 7230 §3.2 line delimiter validation, rejecting unescaped CR or LF inside lines.

### Related Tasks & Requirements
- `REQ-132`: Coverage-Guided Generative Fuzzing Engine, Native Go testing.F Differential Oracles, and Protocol Regression Suite Disambiguation
- `TASK-155`: Coverage-Guided Generative Fuzzing Engine, Native Go testing.F Differential Oracles, and Protocol Regression Suite Disambiguation
- `ADR-132`: Coverage-Guided Generative Fuzzing Engine, Native Go testing.F Differential Oracles, and Protocol Regression Suite Disambiguation Architecture
- `TC-132`: Test Case Specification for Coverage-Guided Generative Fuzzing Engine and Differential Oracles
- `CR-128`: Code Review of Coverage-Guided Generative Fuzzing Engine and Differential Oracles
- `SR-132`: Security Review of Coverage-Guided Generative Fuzzing Engine and Differential Oracles
- Relevant Standards & CWEs: [CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-113](https://cwe.mitre.org/data/definitions/113.html), [CWE-117](https://cwe.mitre.org/data/definitions/117.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), [CWE-78](https://cwe.mitre.org/data/definitions/78.html), [CWE-88](https://cwe.mitre.org/data/definitions/88.html), [CWE-362](https://cwe.mitre.org/data/definitions/362.html), [CWE-436](https://cwe.mitre.org/data/definitions/436.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), RFC 7230 §2.6, RFC 7230 §3.2, RFC 7230 §3.3.3, RFC 7230 §4.1

## 2026-09-16 - Toron v1.5.29 Release (Multi-Tier Saturation Stress Benchmarking, Go Runtime GC Telemetry Capture, and Differential Reverse Proxy Comparison - REQ-130 / TASK-153)

### Milestone Summary
- **Multi-Tier Duration Stress Testing Architecture (REQ-130, TASK-153, ADR-130, TC-130, CR-126, SR-130)**: Solved the empirical and scientific "5-Second Evaluation Blindspot" by implementing a standardized Multi-Tier Duration Taxonomy across all benchmarking harnesses (`benchmarks/wrk2/run_saturation_stress.sh`, `benchmarks/run_all.sh`, and `benchmarks/docker-compare/run_compare.sh`):
  - **Tier 1: Quick Smoke (`quick`, 5s)**: Rapid pre-merge regression verification and CI sanity checks completing in $< 60$ seconds total suite time.
  - **Tier 2: Steady-State / GC Observation (`medium` / `steady`, 60s)**: High-resolution tail latency ($p95, p99, p99.9$) and Go runtime GC cycle convergence observation under stabilized socket connection pools.
  - **Tier 3: Long-Term Soak & Memory Stability (`soak`, 300s / 5 min)**: Sustained soak testing evaluating connection pool longevity, socket descriptor retention ([CWE-775](https://cwe.mitre.org/data/definitions/775.html)), and empirical proof of constant $O(1)$ memory boundedness (`REQ-129` / `ADR-129`) under 1,500,000+ continuous requests.
  - **Tier 4: Comprehensive All-Tiers Sweep (`all`, 5s + 60s + 300s)**: Sequential evaluation matrix generating duration-keyed and consolidated comparative reports for publication-grade systems research.
- **Go Runtime GC Telemetry Capture (`GODEBUG=gctrace=1`) & Clean Stderr Segregation**: Activated non-invasive Go runtime GC tracing via process environment injection without altering production reverse proxy application code. Segregated standard output (HTTP server and routing logs) to `server_stress.log` while directing raw GC traces to dedicated artifact `benchmarks/results/server_gc_trace.log`.
- **High-Performance Zero-Dependency GC Parser Engine (`benchmarks/telemetry/gcparser`)**: Built a modular, zero-dependency GC trace parser in pure Go standard library:
  - **Fast Sub-Microsecond Tokenizer (`fastParseLine`)**: Uses byte-index searching and string slicing to parse standard Go runtime traces at $> 200,000\text{ lines/sec}$ ($< 50\text{ ms}$ for 10,000 lines) with zero heap allocations in the scanning loop.
  - **Multi-Version Compatibility**: Automatically accommodates format variations across Go 1.20, Go 1.22, and Go 1.24+ (e.g. optional `stacks`/`globals` tokens, processor count `P`, fractional milliseconds) with linear-time RE2 regex fallback (`gcRegex`).
  - **Instant Noise Filtering**: Silently discards non-GC log lines (application logs, warnings, stack traces) with zero heap allocation.
- **Stop-The-World (STW) Pause Distribution Metrics**: Computes comprehensive GC timing statistics including `MinSTWMs`, `MeanSTWMs`, `P50STWMs`, `P95STWMs`, `P99STWMs`, `MaxSTWMs`, `TotalSTWMs`, `MeanMarkMs`, and `MaxMarkMs` using sorted-index rank percentiles ($\text{Rank}(P) = \lceil P \times N \rceil - 1$).
- **Ordinary Least Squares (OLS) Linear Regression Heap Growth Engine**: Formulated an OLS linear regression model over post-GC live heap data points $(t_i, H_{\text{live}, i})$:
  $$\text{Slope} = \frac{N \sum(t_i y_i) - \sum t_i \sum y_i}{N \sum(t_i^2) - (\sum t_i)^2} \times 60.0 \quad \left(\frac{\text{MB}}{\text{min}}\right)$$
  Filters out transient GC sawtooth fluctuations to calculate true steady-state heap drift, mathematically proving that Toron's heap growth slope satisfies $\text{Slope} \le 1.0\text{ MB/min}$ under continuous saturation, verifying $O(1) \le 32\text{KB}$ memory boundedness.
- **Zero-Cycle Resilience & Division-by-Zero Elimination ([CWE-369](https://cwe.mitre.org/data/definitions/369.html))**: Implemented defensive guards for $N=0$ (zero GC cycles during short runs or zero-allocation paths) and $N=1$, ensuring `TotalCycles: 0` returns cleanly with zeroed sub-structures, preventing runtime panics, `NaN`, or `+Inf` floats in JSON serialization.
- **Multi-Proxy Differential Docker Benchmarking with Continuous Polling**: Extended `benchmarks/docker-compare` to benchmark **Toron**, **Traefik**, **Caddy**, **NGINX**, and **HAProxy** across 4 heterogeneous backends:
  - **`GODEBUG=gctrace=1` Container Injection**: Enabled on Go proxy containers (`toron-proxy`, `traefik-proxy`, `caddy-proxy`) in `docker-compose.compare.yml`; C proxies (`nginx-proxy`, `haproxy-proxy`) serve as clean manual memory baselines (`N/A (C Runtime)`).
  - **Container GC Trace Extraction**: Automatically queries `docker logs --since <cell_start>` post-cell to extract and parse Go proxy GC traces.
  - **Mandatory 5s Warm-up & 10s Cooldown**: Executes a 5-second pre-warm phase for runs $\ge 30\text{s}$ to establish upstream connection pool sockets, followed by a 10-second inter-proxy cooldown to prevent CPU thermal throttling skew.
  - **Continuous Background Docker Stats Poller**: Spawns a background goroutine sampling `docker stats --no-stream` every 10s (or 5s for 60s runs), tracking CPU % and RSS memory trajectory over time.
- **Dual-Output Reporting & Retention Manifest Synchronization (REQ-119)**:
  - Embedded structured `gc_telemetry` in JSON reports (`saturation_stress_report.json`, `docker_compare_report.json`).
  - Appended Section 5 ("Runtime Garbage Collection & Memory Dynamics") to `saturation_stress_report.md` and Section 4 ("Go Runtime GC Differential Analysis") to `docker_compare_report.md`.
  - Preserved `server_gc_trace.log` and duration tier metadata into historical session archives and indexed in `manifest.json` via `archive_run.sh`.
- **100% Verification Across TC-130.1 to TC-130.20**: Verified all 20 test cases in `TC-130` with 100% pass rate under `go test -race ./benchmarks/...` with zero data races, zero third-party dependencies, and full CI backward compatibility (< 60s quick tier default).

### Added
- **`benchmarks/telemetry/gcparser/model.go`**:
  - `GCTelemetry`: Top-level telemetry struct containing `Enabled`, `TotalCycles`, `GCCPUPercent`, `CyclesPerSecond`, `TotalReclaimedMB`, `PauseTimesMs`, and `HeapMetricsMB`.
  - `GCPauseStatistics`: Capture of `MinSTWMs`, `MeanSTWMs`, `P50STWMs`, `P95STWMs`, `P99STWMs`, `MaxSTWMs`, `TotalSTWMs`, `MeanMarkMs`, and `MaxMarkMs`.
  - `GCHeapStatistics`: Capture of `InitialLiveHeapMB`, `FinalLiveHeapMB`, `PeakLiveHeapMB`, `MeanLiveHeapMB`, `PeakTriggerHeapMB`, and `HeapGrowthSlopeMBm`.
  - `GCEvent` (and type alias `GCCycleRecord`): Parsed representation of single GC cycle line.
- **`benchmarks/telemetry/gcparser/parser.go`**:
  - `ParseLine(line string) (*GCEvent, bool)`: Dual-engine parser combining zero-allocation `fastParseLine` with regex fallback `gcRegex`.
  - `fastParseLine(line string) (*GCEvent, bool)`: High-speed tokenizer using byte slicing and direct index parsing.
  - `ParseReader(r io.Reader, totalDuration time.Duration) (*GCTelemetry, error)`: High-throughput stream scanner using 64KB recycled line buffer.
  - `ParseReaderSeconds`, `ParseFile`, and `ParseFileSeconds` convenience wrappers.
- **`benchmarks/telemetry/gcparser/stats.go`**:
  - `ComputeStatistics(records []GCEvent, totalDuration time.Duration) *GCTelemetry`: Aggregates cycle records, computes STW percentiles via `percentile()`, total reclaimed MB, cycle frequencies, and OLS linear regression slope.
- **`benchmarks/telemetry/gcparser/parser_test.go`**:
  - `TestGCParser_GoVersionFormats`: Validates syntax scanning across Go 1.20, Go 1.22, and Go 1.24+ formats (TC-130.5).
  - `TestGCParser_PauseStatistics`: Validates STW pause percentiles and mark timings against mathematical definitions (TC-130.6).
  - `TestGCParser_HeapGrowthSlope`: Validates OLS linear regression slope across flat, linear growth, cyclic, and $N=1$ inputs (TC-130.7).
  - `TestGCParser_ZeroCycles`: Validates graceful fallback on empty logs and zero-cycle inputs without division-by-zero panics or NaN (TC-130.8).
  - `TestGCParser_NoisyLogIgnored`: Validates skipping application logs and non-GC lines (TC-130.9).
  - `TestGCParser_ConcurrentSafety`: Validates thread-safe reentrancy across 50 concurrent goroutines (TC-130.20).
  - `BenchmarkParseReader`: Asserts parser processes 10,000 trace lines in $< 50\text{ ms}$ ($> 200,000\text{ lines/sec}$).
- **`benchmarks/docker-compare/compare_test.go`**:
  - Extended unit tests covering multi-tier duration parsing (`parseDurationTiers`), time series OLS slope calculation, and Section 4 differential GC table Markdown rendering.

### Changed
- **`benchmarks/wrk2/run_saturation_stress.sh`**:
  - Added multi-tier CLI flag parsing supporting `-d <duration>` (single or comma-separated list) and `--tier <quick|medium|steady|soak|all>`, defaulting to `5s` (`quick`).
  - Injected `GODEBUG=gctrace=1` into background Toron gateway execution, redirecting stderr to `server_gc_trace.log` and stdout to `server_stress.log`.
  - Implemented sequential multi-tier execution loop generating duration-keyed artifacts (`saturation_stress_5s.json/.md`, `saturation_stress_60s.json/.md`, `saturation_stress_300s.json/.md`) and consolidated master report.
  - Preserved `server_gc_trace.log` during historical archiving.
- **`benchmarks/wrk2/loadgen.go`**:
  - Embedded `GCTelemetry *gcparser.GCTelemetry` and `DurationTier string` into `SaturationStressReport`.
  - Added `-gc-trace` CLI flag to ingest and parse Go GC logs.
  - Added `ConsolidateReports` to aggregate sequential multi-tier runs into consolidated JSON and Markdown summaries.
  - Appended Section 5 ("Runtime Garbage Collection & Memory Dynamics") to Markdown report rendering GC cycles, CPU %, STW pause distribution, live heap baseline, and heap growth slope with $O(1)$ boundedness validation indicator.
- **`benchmarks/run_all.sh`**:
  - Added `--tier <quick|medium|soak|all>` and `-d <duration>` flags, defaulting to `quick` (5s) for $< 60$s CI execution.
  - Forwarded duration flags downstream to Stage 2 (`wrk2`) and Stage 3 (`saturation_stress`).
  - Injected `GODEBUG=gctrace=1` and segregated stderr to `server_gc_trace.log` during Stage 3 auto-start.
  - Updated master manifest recorder to index `duration_tier`, execution duration, and GC trace artifacts in `manifest.json`.
- **`benchmarks/docker-compare/docker-compose.compare.yml`**:
  - Injected `GODEBUG=gctrace=1` into `toron-proxy`, `traefik-proxy`, and `caddy-proxy` service environments while preserving clean C baselines for `nginx-proxy` and `haproxy-proxy`.
- **`benchmarks/docker-compare/run_compare.sh`**:
  - Added `-d` and `--tier` argument parsing and forwarded to `runner.go`.
- **`benchmarks/docker-compare/runner.go`**:
  - Implemented `parseDurationTiers` supporting single durations, comma-separated lists, and tier aliases (`quick`, `medium`, `steady`, `soak`, `all`).
  - Added mandatory 5-second pre-warm phase for runs $\ge 30\text{s}$ to prime keep-alive connection pools, discarding warm-up metrics before recording.
  - Added 10-second cooldown pause between proxy targets for runs $\ge 30\text{s}$ to mitigate testbed CPU thermal throttling.
  - Implemented continuous background Docker stats poller (`startContainerStatsPoller`) sampling CPU % and Memory RSS every 10s (or 5s for 60s runs) and computing linear regression slope.
  - Implemented container GC trace extraction (`extractContainerGCTrace`) via `docker logs --since <cell_start>`, parsing GC traces for Go proxies into `cellResult.GCTelemetry`.
  - Added "GC Cycles" and "P99 GC Pause" columns to Section 1 Comparative Table.
  - Appended Section 4 ("Go Runtime GC Differential Analysis") to Markdown report contrasting Toron, Traefik, and Caddy against C baselines NGINX and HAProxy.
- **`benchmarks/archive_run.sh`**:
  - Updated archival replication list to systematically preserve `server_gc_trace.log` and duration-keyed artifacts into historical session directories.

### Fixed
- **Empirical 5-Second Evaluation Blindspot (REQ-130)**: Neutralized transient connection setup bias, GC masking, and inability to detect memory leaks in short-duration runs by introducing 60s steady-state and 300s soak tiers.
- **Invisible Go Runtime Garbage Collection Overhead**: Surfaced previously obscured GC mark CPU overhead, STW pause distributions, and heap reclamation metrics across Toron, Traefik, and Caddy.
- **Single-Snapshot Docker Resource Skew**: Replaced post-run single snapshots with continuous 10s periodic polling, preventing post-sweep idle memory metrics from obscuring in-flight working set peaks.
- **Division-by-Zero and NaN JSON Formatting on Zero-Cycle Runs ([CWE-369](https://cwe.mitre.org/data/definitions/369.html))**: Prevented runtime panics and invalid JSON serialization when evaluating non-allocating or ultra-short tests where zero GC cycles occur.

### Related Tasks & Requirements
- `REQ-130`: Multi-Tier Duration Stress Testing (5s, 60s, 300s), Runtime GC Telemetry Capture, and Differential Reverse Proxy Benchmarking
- `TASK-153`: Implement Multi-Tier Duration Stress Testing (5s, 60s, 300s), Go Runtime GC Telemetry Capture, and Differential Reverse Proxy Benchmarking
- `ADR-130`: Multi-Tier Duration Stress Testing (5s, 60s, 300s), Runtime GC Telemetry Capture, and Differential Reverse Proxy Benchmarking Architecture
- `TC-130`: Test Specification for Multi-Tier Duration Stress Testing (5s, 60s, 300s), Go Runtime GC Telemetry Capture, and Differential Reverse Proxy Benchmarking
- `CR-126`: Code Review of Multi-Tier Duration Stress Testing (5s, 60s, 300s), Go Runtime GC Telemetry Capture, and Differential Reverse Proxy Benchmarking Architecture
- `SR-130`: Security Review of Multi-Tier Duration Stress Testing (5s, 60s, 300s), Runtime GC Telemetry Capture, and Differential Reverse Proxy Benchmarking
- Relevant Standards & CWEs: [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), [CWE-88](https://cwe.mitre.org/data/definitions/88.html), [CWE-369](https://cwe.mitre.org/data/definitions/369.html), [CWE-362](https://cwe.mitre.org/data/definitions/362.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), [CWE-1333](https://cwe.mitre.org/data/definitions/1333.html)

---

## 2026-09-16 - Toron v1.5.28 Release (Streaming by Default Reverse Proxy Architecture, RFC 7230 Outbound Chunked Framing, and Memory Boundedness Invariants - REQ-129 / TASK-152)

### Milestone Summary
- **Streaming by Default Reverse Proxy Architecture (REQ-129, TASK-152, ADR-129, TC-129, CR-125, SR-129)**: Re-architected Toron's core reverse proxy engine to operate in **streaming-by-default** mode across all configuration profiles (`stream_response: true` across `balanced`, `raw_speed`, and unconfigured fallbacks), guaranteeing constant $O(1) \le 32\text{KB}$ memory boundedness per active connection from recycled copy buffer slabs (`copyBufferPool`).
- **Neutralization of Upstream Infinite Stream OOM Bomb (SEC-36, CWE-400, CWE-770)**: Completely eliminated the critical vulnerability where reverse proxy routes configuring transparent compression or response caching evaluated `canStream = false` for generic responses, falling back to unbounded body ingestion via `io.CopyBuffer(res.Body, outResp.Body)`. When upstream endpoints emitted multi-gigabyte files, continuous telemetry feeds, or infinite streams (`/dev/urandom`), dynamic heap expansion triggered operating system Out-Of-Memory (OOM) `SIGKILL` termination, crashing the gateway. Implemented dynamic bounded clamping (`canStream`): responses exceeding `MaxPayloadSize` (default 1 MB / 1,048,576 bytes), chunked transfers (`Transfer-Encoding: chunked`), or unknown lengths (`ContentLength < 0`) dynamically activate direct socket streaming (`res.StreamBody = outResp.Body`), bypassing caching and compression memory accumulators.
- **LimitReader Fallback Safety Clamp**: For bounded payloads ($0 \le \text{Content-Length} \le \text{MaxPayloadSize}$) permitted to buffer in memory for downstream middleware transformation, implemented an `io.LimitReader(outResp.Body, int64(maxPayloadSize)+1)` clamp. If a deceptive or malicious upstream origin writes more bytes than declared or exceeds the maximum buffer limit, Toron immediately halts ingestion, marks target node failure, resets `res.Body`, and returns HTTP `502 Bad Gateway` with `"Upstream payload exceeded maximum allowed buffer limit"`.
- **Core Reactor Modularity Preservation (ADR-001)**: Verified and preserved strict reactor modularity: the reverse proxy and router layers express streaming intent purely by assigning `res.StreamBody = outResp.Body` and never reference, cast, or manipulate the client physical socket (`net.Conn`). Upstream requests are bound directly to downstream client contexts (`http.NewRequestWithContext(req.Context(), ...)`), ensuring that client drops or TCP RST packets immediately halt in-flight upstream reads and close upstream handles.
- **Outbound RFC 7230 Chunked Response Framing Engine**: In `pkg/server/server.go`, implemented an outbound chunk framing engine for HTTP/1.1 streaming responses (`res.StreamBody != nil`). Sets `Transfer-Encoding: chunked`, strips conflicting `Content-Length`, and serializes chunk frames `<hex-len>\r\n<data>\r\n` using stack-allocated hex buffers (`strconv.AppendInt` into `[32]byte`) and single-syscall scatter-gather `net.Buffers{h, buf[:n], crlfBytes}.WriteTo(conn)` (`writev`), eliminating TCP frame fragmentation and heap allocations.
- **Clean EOF Terminal Chunk (0\r\n\r\n) & HTTP/1.1 Persistent Keep-Alive Socket Reuse**: Upon clean stream termination (`io.EOF`), the server emits the RFC 7230 terminal chunk (`0\r\n\r\n`) and preserves the client TCP socket for subsequent transactions (`Connection: keep-alive`), eliminating connection churn and `TIME_WAIT` socket descriptor exhaustion.
- **Fail-Closed Anti-Desynchronization Guard (CWE-444)**: On any upstream read error, abort, client timeout, or context cancellation prior to clean `io.EOF`, the server strictly suppresses `0\r\n\r\n` and abruptly severs the client TCP connection (`conn.Close()`), preventing downstream clients and shared proxy caches from capturing truncated payloads.
- **HTTP/1.0 Raw Stream Passthrough (RFC 7230 §3.3.1)**: Complies strictly with RFC 7230 §3.3.1 for HTTP/1.0 clients by omitting `Transfer-Encoding: chunked`, streaming raw chunks, and closing the socket with `Connection: close`.
- **Multi-Protocol Streaming Parity (HTTP/2 & HTTP/3 Flusher)**: In `pkg/server/server.go` `http2AdapterHandler`, relays chunks directly to `http.ResponseWriter` via `http.Flusher.Flush()` with pooled 32KB copy buffers (`httpparser.GetCopyBuffer()`), immediately dispatching HTTP/2 multiplexed `DATA` frames and HTTP/3 QUIC frames with instant termination upon client stream reset (`RST_STREAM`, `r.Context().Done()`).
- **Ergonomic Response Body Abstraction (`Response.BodyString()`, `Response.BodyBytes()`)**: Introduced uniform body extraction methods on `httpparser.Response` in `pkg/httpparser/response.go` that handle both streaming (`res.StreamBody != nil`) and buffered (`res.Body != nil`) responses transparently with guaranteed deferred closure, unifying test and caller assertions across `router`, `discovery`, and `ingress`.
- **Inbound Request Smuggling Defense Decoupling (ADR-056 / REQ-061)**: Retained strict inbound request smuggling defenses: incoming client requests declaring `Transfer-Encoding: chunked` continue to be rejected with HTTP `501 Not Implemented`, ensuring outbound response framing is decoupled from inbound perimeter hardening.
- **100% Verification Across TC-129.1 to TC-129.17**: Achieved 100% test pass rate across all 17 test cases in `TC-129` and the full workspace test suite under `go test -race ./...` with zero data races, zero third-party dependencies, and zero regression across the entire Toron codebase.

### Added
- **`pkg/httpparser/response.go`**:
  - `BodyString() string`: Ergonomically drains and returns response payload as string across both streaming (`res.StreamBody`) and buffered (`res.Body`) responses with guaranteed deferred closure.
  - `BodyBytes() []byte`: Drains and returns response payload as byte slice across streaming and buffered responses.
- **`pkg/httpparser/request.go`**:
  - Added `ctx context.Context` field to `Request`.
  - Added `Context() context.Context`, `WithContext(ctx context.Context) *Request`, and `SetContext(ctx context.Context)` methods.
  - Bound incoming context in `NewRequest` and `NewRequestFromStd`.
- **`pkg/server/server.go`**:
  - Added package-level immutable delimiter `var crlfBytes = []byte("\r\n")` for zero-allocation chunk framing.
  - Added `relayStreamBody(conn net.Conn, req *httpparser.Request, res *httpparser.Response, tracker *connDeadlineTracker, connHeader string) (keepAlive bool, err error)` helper encapsulating stream lifecycle, RFC 7230 chunk formatting via `net.Buffers`, fail-closed socket termination, activity-refreshed write deadlines, and keep-alive socket reuse.
- **`pkg/proxy/proxy_test.go`**:
  - `TestProxy_StreamResponse_DefaultTrue`: Verifies default `streamResponse: true` in `NewReverseProxy` (TC-129.1).
  - `TestProxy_StreamResponse_RouteOverrideFalse`: Verifies explicit route override `stream_response: false` is respected (TC-129.2).
  - `TestProxy_DynamicClamp_BoundedPayload_BuffersForMiddleware`: Verifies payloads $\le \text{MaxPayloadSize}$ buffer for compression/caching (TC-129.3).
  - `TestProxy_DynamicClamp_OversizedPayload_StreamsDirectly`: Verifies payloads $> \text{MaxPayloadSize}$ dynamically stream directly (TC-129.4).
  - `TestProxy_DynamicClamp_ChunkedUnknownLength_StreamsDirectly`: Verifies chunked upstream responses dynamically stream directly (TC-129.5).
  - `TestProxy_DynamicClamp_InfiniteStream_OOMImmunity`: Verifies 50 MB infinite stream relays with constant $O(1) \le 32\text{KB}$ memory and process heap delta $< 64\text{KB}$, neutralizing SEC-36 / CWE-400 (TC-129.6).
  - `TestProxy_DynamicClamp_LimitReaderSafetyClamp_DeceptiveUpstream`: Verifies deceptive upstream exceeding limit is caught by `LimitReader`, emitting 502 Bad Gateway (TC-129.7).
- **`pkg/server/server_test.go`**:
  - `TestParser_InboundSmugglingGuard_Preserved`: Verifies inbound `Transfer-Encoding: chunked` requests continue to be rejected with HTTP 501 (TC-129.8).
  - `TestServer_HTTP11_OutboundChunkedFraming`: Verifies HTTP/1.1 streaming responses emit `Transfer-Encoding: chunked`, omit `Content-Length`, format `<hex-len>\r\n<data>\r\n`, and terminate with `0\r\n\r\n` (TC-129.9).
  - `TestServer_HTTP11_ChunkedStream_KeepAliveSocketReuse`: Verifies persistent TCP socket reuse for subsequent HTTP/1.1 requests after streaming (TC-129.10).
  - `TestServer_HTTP10_RawStreaming_ConnectionClose`: Verifies HTTP/1.0 clients receive raw streams without chunked framing and close with `Connection: close` (TC-129.11).
  - `TestServer_ChunkedStream_UpstreamAbort_FailClosed`: Verifies aborted upstream streams strictly suppress `0\r\n\r\n` and immediately close the client socket (TC-129.12).
  - `TestServer_StreamContextCancellation_ZeroFDLeakage`: Verifies downstream disconnect cancels upstream context and closes `res.StreamBody` with zero FD leaks (TC-129.13).
  - `TestServer_ChunkedStream_ConcurrentRaceSafety`: Verifies 50 concurrent streaming workers execute with zero data races (TC-129.14).
  - `TestServer_Stream_AllocsPerRun_ZeroMemoryGrowth`: Verifies steady-state streaming allocates $\le 1$ alloc/run (TC-129.17).
- **`pkg/server/http2_test.go`**:
  - `TestServer_HTTP2_StreamBody_FlusherParity`: Verifies HTTP/2 streaming emits chunks in real-time via `http.Flusher` (TC-129.15).
  - `TestServer_HTTP2_ClientReset_AbortsStream`: Verifies HTTP/2 client stream reset (`RST_STREAM`) immediately halts relay loop and closes upstream body (TC-129.16).
- **`pkg/config/config_test.go`**:
  - `TestConfig_ProxyTransport_StreamResponseDefault`: Verifies `DefaultProxyTransportConfig` sets `StreamResponse: true` for both `"raw_speed"` and `"balanced"` profiles (TC-129.1).
- **`pkg/httpparser/parser_test.go`**:
  - `TestParser_InboundSmugglingGuard_Preserved`: Verifies inbound smuggling rejection.
  - `TestRequest_ContextMethods`: Verifies `Context()`, `WithContext()`, and `SetContext()` semantics.

### Changed
- **`pkg/config/config.go`**:
  - Updated `DefaultProxyTransportConfig` to set `StreamResponse: &t` (`true`) for the `"balanced"` and `"standard"` transport profiles, making streaming by default universal across all profiles.
- **`pkg/proxy/proxy.go`**:
  - In `NewReverseProxy`, updated default fallback: `streamResponse := true`.
  - Added `MaxPayloadSize int` field to `ProxyOptions` and stored in `ReverseProxy.maxPayloadSize` (defaulting to 1 MB / `1048576` bytes).
  - In `ServeHTTPWithPrefix`, implemented dynamic bounded clamping rule: `canStream := p.streamResponse && ((!p.routeHasCompression && !p.routeHasCache) || isStreamingMIME || isUnbuffered || isChunkedOrUnknown || isOversized)`.
  - Implemented `io.LimitReader(outResp.Body, int64(maxPayloadSize)+1)` in buffered fallback path with verbatim ADR error message: `"Upstream payload exceeded maximum allowed buffer limit"`.
  - Updated upstream request dispatch to bind downstream client context: `outReq, err := http.NewRequestWithContext(req.Context(), ...)`.
- **`pkg/server/server.go`**:
  - Refactored streaming response handling in `handleConn` to invoke `s.relayStreamBody(...)`, ensuring deferred stream body closure (`res.StreamBody.Close()`) and copy buffer recycling under all execution paths.
  - In `http2AdapterHandler`, integrated zero-buffering streaming parity using pooled 32KB copy buffers and `http.Flusher.Flush()` with `r.Context().Done()` cancellation detection.
  - Bound incoming standard library request context into Toron request: `toronReq = toronReq.WithContext(r.Context())`.
- **`pkg/sidecar/proxy.go`**:
  - Updated `proxyToURL` to stream `toronRes.StreamBody` directly via `io.Copy(w, ...)` when present.
- **`pkg/httpparser/response.go`**:
  - In `Response.Serialize`, strictly omitted `Content-Length` header when `res.StreamBody != nil`.
- **Cross-Package Test Harnesses**:
  - Migrated body assertions across `pkg/router/router_test.go`, `pkg/router/router_waf_test.go`, `pkg/ingress/ingress_test.go`, and `pkg/discovery/discovery_test.go` to use `res.BodyString()`.
- **`docs/wiki/features/reverse-proxy.md`**:
  - Documented streaming-by-default architecture, dynamic bounded clamping decision matrix, `max_payload_size`, outbound chunked framing, keep-alive reuse, and fail-closed anti-desynchronization.
- **`docs/wiki/configuration.md` & `docs/wiki/reference/config-options.md`**:
  - Updated `ProxyTransportConfig` and routes configuration with `stream_response` (default: `true`) and `max_payload_size` (default: `1048576`).

### Fixed
- **Upstream Infinite Stream OOM Bomb (SEC-36, CWE-400, CWE-770)**: Eliminated uncontrolled heap expansion and gateway crashes caused by buffering infinite or oversized upstream responses into `res.Body` on routes with compression or caching enabled.
- **HTTP/1.1 Persistent Keep-Alive Connection Degradation on Streaming**: Fixed connection churn and premature socket closure by implementing outbound RFC 7230 chunked framing with clean terminal chunk (`0\r\n\r\n`), allowing persistent keep-alive connection reuse across streaming requests.
- **HTTP Stream Boundary Desynchronization & Truncation Injection (CWE-444)**: Prevented downstream caches and clients from storing truncated responses by enforcing fail-closed socket termination (never emitting `0\r\n\r\n` on stream errors or aborts).
- **Socket Descriptor and Goroutine Leaks on Stream Disconnects (CWE-775)**: Ensured upstream contexts cancel immediately upon client disconnect and upstream response bodies close under all exit paths.

### Related Tasks & Requirements
- `REQ-129`: Streaming by Default Reverse Proxy Architecture, RFC 7230 Outbound Chunked Framing, and Memory Boundedness Invariants
- `TASK-152`: Implement Streaming by Default Reverse Proxy Architecture, RFC 7230 Outbound Chunked Framing, and Memory Boundedness Invariants
- `ADR-129`: Streaming by Default Reverse Proxy Architecture, RFC 7230 Outbound Chunked Framing, and Memory Boundedness Invariants Architecture
- `TC-129`: Test Specification for Streaming by Default Reverse Proxy Architecture, RFC 7230 Outbound Chunked Framing, and Memory Boundedness Invariants
- `CR-125`: Code Review of Streaming by Default Reverse Proxy Architecture, RFC 7230 Outbound Chunked Framing, and Memory Boundedness Invariants
- `SR-129`: Security Review of Streaming by Default Reverse Proxy Architecture, RFC 7230 Outbound Chunked Framing, and Memory Boundedness Invariants
- Relevant Standards & CWEs: RFC 7230 §3.3.1, RFC 7230 §3.3.3, RFC 7230 §4.1, RFC 7230 §6.1, SEC-36, [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), [CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), [CWE-362](https://cwe.mitre.org/data/definitions/362.html)

---

## 2026-09-16 - Toron v1.5.27 Release (Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement - REQ-128 / TASK-151)

### Milestone Summary
- **Streaming Response Middleware Exemptions & RFC 7234 §5.2.2.2 Enforcement (REQ-128, TASK-151, ADR-128, TC-128, CR-124, SR-128)**: Delivered a comprehensive architectural and code-level resolution eliminating real-time streaming corruption, stream freezing, and compression event starvation across Server-Sent Events (SSE `text/event-stream`) and dynamic HTTP responses.
- **RFC 7234 §5.2.2.2 Origin Cache-Control Enforcement (`resCC.NoCache`)**: Strictly excluded origin responses specifying `Cache-Control: no-cache` from admission to `ResponseCache` in `pkg/router/cache.go`. In a shared proxy cache without conditional origin revalidation (such as `If-None-Match` or `If-Modified-Since`), `no-cache` strictly overrides `max-age` directives (e.g. `Cache-Control: no-cache, max-age=3600`). This eliminates Web Cache Deception and Shared Cache Information Exposure ([CWE-524](https://cwe.mitre.org/data/definitions/524.html)), preventing private dynamic responses or telemetry feeds from being captured in shared memory and served to other tenants.
- **Server-Sent Events (SSE) & Unbuffered Stream Cache Exemption**: Established an unconditional cache exclusion guard in `pkg/router/cache.go` for responses bearing `Content-Type: text/event-stream` (case-insensitive, with or without parameters) and reverse proxy unbuffered hints (`X-Accel-Buffering: no`). Completely eliminated the root-cause defect where SSE live feeds fell back to `DefaultTTL = 60s`, cloning an initial chunk snapshot into cache and serving static dead streams with `X-Cache: HIT` to subsequent clients.
- **Deterministic Cache Bypass Response Semantics**: Guaranteed that all bypassed responses emit `X-Cache: MISS` in response headers, strictly delete and omit the `Age` header, and bypass `cache.Set`, resulting in zero byte cloning and zero entries stored in `ResponseCache`. Client requests specifying `Cache-Control: no-cache` or `Pragma: no-cache` continue to bypass cache lookups unconditionally.
- **Streaming Response Compression Exemption**: Resolved live stream event starvation in `pkg/router/compression.go`. Previously, wildcard `"text/"` in `DefaultCompressionConfig.Types` matched `text/event-stream`, causing the compression middleware to trap streaming chunks in RAM compressor accumulators until stream termination. Implemented an early pre-compression inspection guard bypassing compression for `text/event-stream` and `X-Accel-Buffering: no`, ensuring raw uncompressed chunks pass through immediately with unmodified (empty) `Content-Encoding` and zero compressor pool writer checkouts (`zstdPool`, `brotliPool`, `gzipPool`, `deflatePool`).
- **Content-Aware Reverse Proxy Fast-Path Activation (`canStream`)**: Refined reverse proxy streaming eligibility in `pkg/proxy/proxy.go` (`canStream := p.streamResponse && ((!p.routeHasCompression && !p.routeHasCache) || isStreamingMIME || isUnbuffered)`). Previously, routes with caching or compression enabled globally forced `canStream = false`, buffering infinite SSE streams into `res.Body` and risking Denial of Service via memory exhaustion ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)). Now, streaming MIME and unbuffered responses activate direct socket streaming (`res.StreamBody = outResp.Body`) unconditionally even on routes where compression or caching are enabled.
- **RFC 7230 §6.1 Hop-by-Hop Header Stripping & Stream Hand-Off**: Upstream hop-by-hop headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailers`, `Transfer-Encoding`, `Upgrade`) are filtered prior to socket hand-off. The server streaming engine guarantees deferred socket closure (`defer res.StreamBody.Close()`) on stream completion, client disconnect, or I/O error, eliminating resource leaks.
- **Zero-Allocation Exemption Guards**: Header inspection guards utilize zero-alloc string comparisons (`strings.ToLower`, `strings.HasPrefix`, `strings.EqualFold`) and fast-path canonical header key lookup for 23 common headers in `pkg/httpparser/request.go`, executing with zero dynamic heap allocations.
- **Microbenchmark & Concurrency Verification**: Verified across all 14 test cases in `TC-128`, achieving 100% pass rate under `go test -race ./pkg/router/... ./pkg/proxy/... ./pkg/server/...` with zero data races, zero heap allocation regressions, and zero third-party dependencies.

### Added
- **`pkg/router/cache_test.go`**:
  - `TestCache_RFC7234_OriginNoCache_Bypass`: Verifies responses with `Cache-Control: no-cache` are never stored in cache, emit `X-Cache: MISS`, omit `Age`, and execute handler on every request (TC-128.1).
  - `TestCache_OriginNoCache_WithMaxAge_Bypass`: Verifies `no-cache` strictly overrides `max-age` directives (TC-128.2).
  - `TestCache_TextEventStream_Bypass`: Verifies `text/event-stream`, parameter variations (`charset=utf-8`), and uppercase MIME variants bypass cache admission (TC-128.3).
  - `TestCache_XAccelBufferingNo_Bypass`: Verifies `X-Accel-Buffering: no` responses bypass cache admission (TC-128.4).
  - `TestCache_StandardResponses_StillCached`: Regression test verifying cacheable responses (`public, max-age=60`) continue to be cached and served with `X-Cache: HIT` and `Age` header (TC-128.5).
  - `TestCache_ZeroAllocationGuards`: Validates that header inspection checks execute with $\le 1.0$ allocs/op using `testing.AllocsPerRun` (TC-128.13).
- **`pkg/router/compression_test.go`**:
  - `TestCompression_TextEventStream_Bypass`: Verifies `text/event-stream` bypasses compression, emits uncompressed payload immediately, and preserves empty `Content-Encoding` (TC-128.6).
  - `TestCompression_TextEventStream_WithParams_Bypass`: Verifies MIME parameter variants bypass compression (TC-128.7).
  - `TestCompression_XAccelBufferingNo_Bypass`: Verifies `X-Accel-Buffering: no` unbuffered responses bypass compression (TC-128.8).
  - `TestCompression_StandardText_Compressed`: Regression test verifying standard text payloads (`text/plain`, `text/html`, `application/json`) continue to be compressed when `Accept-Encoding` matches (TC-128.9).
- **`pkg/proxy/proxy_test.go`**:
  - `TestProxy_SSE_WithCompressionAndCacheEnabled`: End-to-end integration test confirming SSE responses activate `canStream = true` and stream chunks in real-time on routes with compression and caching enabled (TC-128.10).
  - `TestProxy_XAccelBufferingNo_DirectSocketFastPath`: Confirms `X-Accel-Buffering: no` activates direct socket streaming fast-path (TC-128.11).
  - `TestProxy_StandardJSON_RouteBuffering`: Verifies standard JSON responses on compression routes buffer appropriately for transformation (TC-128.12).
  - `TestProxy_Streaming_ConcurrentRaceSafety`: Validates concurrent streaming and cached/compressed requests under 50 parallel workers with zero data races (TC-128.14).
- **`pkg/router/cache.go`**:
  - Exposed `ResponseCache.Len() int` for atomic inspection of active in-memory cache entries.
  - Added `NewCacheMiddlewareWithStore(cfg CacheConfig, cache *ResponseCache) MiddlewareFunc` to enable test harness injection.

### Changed
- **`pkg/router/cache.go`**:
  - In `CacheMiddleware`, added inspection of `resCC.NoCache` alongside `resCC.NoStore` and `resCC.Private`, strictly enforcing RFC 7234 §5.2.2.2.
  - Added pre-status guard checking `strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/event-stream")` and `strings.EqualFold(strings.TrimSpace(res.Header.Get("X-Accel-Buffering")), "no")`.
  - Explicitly deleted `Age` header (`res.Header.Del("Age")`) on cache bypass and ensured `X-Cache: MISS` is set.
- **`pkg/router/compression.go`**:
  - Positioned streaming response inspection guard immediately after connection upgrade and `res.StreamBody` checks, strictly prior to status code and payload length evaluation.
  - Exempted `text/event-stream` and `X-Accel-Buffering: no` from compressor pool checkout, compression transformation, and `Content-Encoding` mutation.
- **`pkg/proxy/proxy.go`**:
  - Refined `canStream` calculation in `ServeHTTPWithPrefix` to activate direct socket streaming (`res.StreamBody = outResp.Body`) when `isStreamingMIME` or `isUnbuffered` evaluate to `true`, even if `routeHasCompression` or `routeHasCache` are active.
  - Added explicit grouping parentheses to prevent boolean operator ambiguity.
- **`pkg/httpparser/request.go`**:
  - Optimized `canonicalKey` with a rodata switch covering 23 common HTTP headers, eliminating dynamic heap allocations during header parsing and lookups.
- **`docs/wiki/features/response-caching.md`**:
  - Documented RFC 7234 §5.2.2.2 origin `no-cache` enforcement, streaming MIME cache exemptions, and cache bypass header semantics.
- **`docs/wiki/features/compression.md`**:
  - Documented streaming compression exemptions for `text/event-stream` and `X-Accel-Buffering: no`, zero-buffering passthrough, and compressor pool bypass.
- **`docs/wiki/features/reverse-proxy.md`**:
  - Documented content-aware streaming fast-path activation (`canStream`), WebSocket-aligned stream hand-off, and memory exhaustion (CWE-400) prevention.

### Fixed
- **SSE Stream Freezing Defect & Web Cache Deception (CWE-524)**: Prevented shared in-memory response cache from capturing dynamic `no-cache` or `text/event-stream` responses and serving frozen static snapshots to subsequent clients.
- **SSE Event Starvation via Wildcard Compression**: Eliminated the defect where wildcard `"text/"` trapped Server-Sent Events in compressor memory accumulators until connection termination, restoring immediate $< 1\text{ms}$ event delivery.
- **Denial of Service via Unbounded Memory Buffering (CWE-400)**: Prevented infinite upstream SSE streams from being forced into `res.Body` buffer accumulation on routes with caching or compression enabled.
- **Improper Cache Directive Evaluation (RFC 7234 §5.2.2.2)**: Fixed precedence defect where `max-age` previously caused `no-cache` responses to be admitted to shared storage without origin revalidation.

### Related Tasks & Requirements
- `REQ-128`: Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement
- `TASK-151`: Implement Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement
- `ADR-128`: Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement Architecture
- `TC-128`: Test Specification for Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement
- `CR-124`: Code Review of Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement
- `SR-128`: Security Review of Streaming Response Middleware Exemptions and RFC 7234 Origin Cache-Control Enforcement
- Relevant Standards & CWEs: RFC 7234 §5.2.2.2, RFC 7230 §6.1, [CWE-524](https://cwe.mitre.org/data/definitions/524.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html)

---

## 2026-09-16 - Toron v1.5.26 Performance Release (Explicit Client Socket TCP_NODELAY Configuration, 60s Keep-Alive Probing, and Zero-Allocation Response Serialization - REQ-127 / TASK-150)

### Milestone Summary
- **Explicit Client Socket Transport Tuning (REQ-127, TASK-150, ADR-127, TC-127, CR-123, SR-127)**: Implemented explicit transport-layer socket tuning on all accepted client TCP connections (`SetNoDelay(true)`, `SetKeepAlive(true)`, `SetKeepAlivePeriod(60s)`), resolving latency and socket management bottlenecks identified during multi-proxy differential benchmarking (`REQ-121`).
- **Elimination of Nagle's Delayed-ACK Latency Freeze (RFC 896 & RFC 1122 §4.2.3.2)**: Configured `SetNoDelay(true)` immediately upon connection acceptance in `Reactor.Serve` and reinforced in `Server.handleConn`. Disabling Nagle packet buffering eliminates the catastrophic 40ms–200ms delayed-ACK latency stalls on small JSON microservice responses and Server-Sent Events (SSE `text/event-stream`), enabling immediate sub-millisecond wire delivery ($< 2.4\text{ms}$ inter-frame delta, total 5-frame burst delivery in $9.30\text{ms}$).
- **Half-Open Connection Detection via 60s Keep-Alive Probes (CWE-400)**: Configured kernel TCP keep-alive probes (`SetKeepAlive(true)` and `SetKeepAlivePeriod(60 * time.Second)`). During quiet intervals in persistent streaming feeds or long-polling sessions, silent client disconnects (WiFi drops, mobile handoffs, NAT timeouts) are actively probed and reaped by the kernel within the probe interval, cleanly tearing down worker goroutines and upstream proxy handles and preventing file descriptor exhaustion ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)).
- **Recursive Socket Unwrapping Architecture (`ExtractTCPConn`)**: Developed an iterative, depth-bounded socket unwrapper (`ExtractTCPConn`) capable of penetrating plain TCP, standard TLS (`*crypto/tls.Conn` / `NetConn()`), deadline amortization trackers (`connDeadlineTracker` / `Unwrap()`), HTTP/2 preface sniffing wrappers (`prefixConn` / `Unwrap()`), and 4-tier nested wrapper chains. Incorporates a strict iteration bound (`maxDepth = 10`) providing guaranteed immunity against circular wrapper graphs and stack exhaustion (CWE-674).
- **Zero-Allocation Response Serialization (`responseBufPool`)**: Implemented a dedicated 4KB buffer slab pool in `pkg/httpparser` (`responseBufPool`) via `sync.Pool`, eliminating ~24,500 dynamic heap allocations per second at 24.5k RPS. Enforces double-reset length hygiene (`*b = (*b)[:0]` on both get and put) to eliminate cross-request data leaks ([CWE-200](https://cwe.mitre.org/data/definitions/200.html) / [CWE-226](https://cwe.mitre.org/data/definitions/226.html)).
- **Fast Static Status Lines & Zero-Allocation CRLF Header Sanitization**: Pre-computed static byte slices for common HTTP status codes (200, 204, 301, 302, 304, 400, 401, 403, 404, 500, 502, 503) and direct `strconv.AppendInt` for custom codes. Implemented SIMD-accelerated `appendSanitizedHeader` with in-place byte filtering, neutralizing CRLF injection and HTTP response splitting (CWE-113) with zero heap allocations on clean headers.
- **Zero-Copy Dual-Write Wire Emission**: Decoupled HTTP header formatting from payload transmission. Serialized header blocks are written directly to the wire, followed by direct payload emission (`conn.Write(r.Body.Bytes())`), eliminating monolithic combined buffer allocations. For streaming responses (`r.StreamBody != nil`), `Content-Length` is strictly omitted and body emission is bypassed for direct delegation to the server socket streaming loop.
- **Microbenchmark & Zero-Allocation Verification**: Benchmark `BenchmarkResponse_Serialize_Pooled` verified **0 B/op and 0 allocs/op** ($137.0\text{ ns/op}$); `testing.AllocsPerRun(1000)` confirmed $\le 1.0$ allocs/op.
- **Zero Third-Party Dependencies & Concurrency Safety**: Implemented strictly using standard library packages (`net`, `crypto/tls`, `sync`, `strconv`, `strings`, `bytes`, `time`). Verified 100% race-free under `go test -race ./pkg/reactor/... ./pkg/httpparser/... ./pkg/server/...`.

### Added
- **`pkg/reactor/socket.go`**: Implemented `ExtractTCPConn` with bounded iteration (`maxDepth = 10`) and `ConfigureTCPSocket` enforcing `SetNoDelay(true)`, `SetKeepAlive(true)`, and `SetKeepAlivePeriod(60s)`.
- **`pkg/reactor/socket_test.go`**: Dedicated test suite verifying `ExtractTCPConn` across 6 wrapping permutations (direct TCP, TLS, tracker, prefix, 4-tier nesting, circular wrapper termination, in-memory pipe) and `ConfigureTCPSocket` enforcement on live and mock sockets.
- **`pkg/server/socket_options_test.go`**: Server-level test suite validating `ExtractTCPConn` wrapped socket unwrapping, `Server.handleConn` safeguard idempotence, SSE immediate frame wire delivery eliminating Nagle delay, keep-alive verification, and 50-client concurrent stress testing under `-race`.
- **`pkg/httpparser/response_bench_test.go`**: Benchmark `BenchmarkResponse_Serialize_Pooled` validating throughput ($137\text{ ns/op}$) and zero-allocation memory performance.
- **Automated Verification Suites in `pkg/httpparser/response_test.go`**:
  - `TestHttpParser_ResponseBufPool_RecyclingAndHygiene` (TC-127.7: slab capacity $\ge 4096$, clean resetting, nil-safety).
  - `TestHttpParser_Response_ZeroAllocationSerialization` (TC-127.8: $\le 1.0$ alloc/op under `testing.AllocsPerRun`).
  - `TestHttpParser_Response_CRLFProtectionAndDualWrite` (TC-127.9: response splitting neutralization and RFC compliance via `http.ReadResponse`).
  - `TestHttpParser_ResponseBufPool_ConcurrentStress` (TC-127.10: 100 concurrent workers serializing unique responses without cross-worker data leaks).

### Changed
- **`pkg/reactor/reactor.go`**:
  - In `Reactor.Serve`, invoked `ConfigureTCPSocket(conn)` immediately upon return from `ln.Accept()`, strictly prior to connection tracking and worker queue submission.
- **`pkg/server/server.go`**:
  - Added package-level delegates `ExtractTCPConn` and `ConfigureTCPSocket`.
  - Added idempotent `ConfigureTCPSocket(conn)` entry safeguard at the start of `handleConn`.
- **`pkg/httpparser/response.go`**:
  - Added 4KB slab pool `responseBufPool`, `getResponseBuf()`, and `putResponseBuf()`.
  - Added public aliases `GetResponseBuffer`, `PutResponseBuffer`, `GetResponseBuf`, `PutResponseBuf`.
  - Added pre-computed static status lines (`statusLine200`, `statusLine404`, etc.) and `appendStatusLine`.
  - Added `appendSanitizedHeader` with SIMD scan and in-place CRLF stripping.
  - Refactored `Response.Serialize` to use pooled slabs, fast status lines, zero-allocation header sanitization, and dual-write wire emission.
- **`pkg/server/export_test.go`**:
  - Exported `NewPrefixConn` for whitebox unwrapping tests in `server_test`.
- **`docs/wiki/features/event-reactor.md`**:
  - Documented explicit socket option tuning (`TCP_NODELAY`, 60s keep-alive probes), Nagle vs delayed ACK physics, quiet stream half-open socket defense, recursive unwrapper (`ExtractTCPConn`), and TC-127 verification results.
- **`docs/wiki/features/reverse-proxy.md`**:
  - Documented zero-allocation response serialization architecture, 4KB slab recycling, fast status lookup, CRLF sanitization, dual-write wire emission, and streaming compatibility.

### Fixed
- **Nagle Delayed-ACK Latency Freezes on Small Frames & SSE**: Eliminated the 40ms–200ms inter-frame packet stalls caused by OS Nagle buffering interacting with client delayed ACKs on Server-Sent Events and small JSON responses.
- **Half-Open Socket Leaks During Quiet Streaming Feeds (CWE-400)**: Prevented orphaned sockets from remaining pinned indefinitely when clients disconnect silently during quiet streaming feeds without sending FIN/RST packets.
- **Response Serialization Heap Churn**: Eliminated ~24,500 dynamic heap allocations per second during high-concurrency request serialization, stabilizing garbage collection pause times and CPU cache efficiency.

### Related Tasks & Requirements
- `REQ-127`: Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling
- `TASK-150`: Implement Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling
- `ADR-127`: Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling Architecture
- `TC-127`: Verification of Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling
- `CR-123`: Code Review of Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling
- `SR-127`: Security Review of Explicit Client Socket TCP_NODELAY Configuration and Serialization Buffer Recycling

---

## 2026-09-16 - Toron v1.5.25 Performance & Security Release (Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts - REQ-126 / TASK-149)

### Milestone Summary
- **Adaptive Socket Deadline Amortization (REQ-126, TASK-149, ADR-126, TC-126, CR-122, SR-126)**: Implemented an adaptive connection deadline amortization engine (`connDeadlineTracker`) that resolves kernel socket system call saturation (~49,000 syscalls/sec at 24.5k RPS) during high-concurrency keep-alive HTTP request bursts.
- **Syscall Reduction (>99%)**: Bypasses redundant operating system `SetReadDeadline` and `SetWriteDeadline` system calls when more than half of the configured timeout window remains active ($R > \tau/2$). Benchmarking under `TC-126.1` confirmed $>99.8\%$ syscall reduction (only $\le 2$ read and $\le 2$ write syscalls per 1,000 burst requests), substantially reducing ring-3 to ring-0 context switches and CPU instruction cache thrashing.
- **Strict Idle Timeout State Machine Alignment**: Resolved the boundary condition between active request processing and keep-alive idle states. When an HTTP transaction completes and the connection buffer is empty (`br.Buffered() == 0`), Toron immediately resets the read amortization cache (`ResetReadAmortization()`) and forces an explicit `idle_timeout` deadline (`ForceSetReadDeadline(now + idle_timeout)`). This prevents long active request read deadlines from bleeding into idle periods, preserving 100% compliance with Slowloris defense mandates (`REQ-005` §2, `TASK-004` §3).
- **Activity-Refreshed Streaming Write Deadlines (`res.StreamBody`)**: Resolved the conflict between static write deadlines (premature termination after 5s) and persistent streaming responses (Server-Sent Events `text/event-stream`, live feeds, and unbuffered reverse proxy streams). By refreshing the socket write deadline on every transmitted chunk (`ForceSetWriteDeadline(now + write_timeout)`), healthy streams persist indefinitely across minutes, hours, or days.
- **Slow-Read Denial of Service (CWE-400) Defense**: Maintained strict protection against slow-reading or stalled clients. If a client stalls or advertises a zero TCP window, the socket send buffer saturates, `conn.Write` blocks, and the kernel write deadline expires within `write_timeout`. Toron cleanly terminates `res.StreamBody` (tearing down upstream origin handles), closes the client socket, and terminates the worker goroutine without resource leaks (`TC-126.5`).
- **Bidirectional Upgraded Relay Amortization (`relayStreams`)**: Integrated `connDeadlineTracker` into upgraded WebSocket and L4 transparent tunnels, amortizing read/write deadlines during bidirectional message bursts while enforcing inactivity timeouts and gracefully forwarding TCP half-close (`CloseWrite()`).
- **Zero-Timeout Benchmark Mode (`read_timeout: 0`, `write_timeout: 0`)**: Added native support for disabling connection deadlines completely in trusted benchmark environments, executing exactly zero deadline system calls without default overwrites.
- **Strict Non-Negative Configuration Validation**: Enforced startup validation across all server timeout options (`read_timeout`, `write_timeout`, `idle_timeout`, `upgrade_idle_timeout`), immediately rejecting negative durations with actionable error messages.
- **Zero Third-Party Dependencies & Concurrency Safety**: Implemented entirely with Go standard library primitives (`sync`, `sync/atomic`, `net`, `time`, `syscall`). 100% race-clean across server and proxy suites under `go test -race`.

### Added
- **`pkg/server/deadline.go`**: Implemented `connDeadlineTracker` wrapping `net.Conn` with `SetAmortizedReadDeadline`, `SetAmortizedWriteDeadline`, `ForceSetReadDeadline`, `ForceSetWriteDeadline`, `ResetReadAmortization`, `ResetWriteAmortization`, atomic telemetry counters, and transparent interface delegation (`Unwrap`, `CloseWrite`, `SyscallConn`).
- **`pkg/server/export_test.go`**: Whitebox test exports for `connDeadlineTracker` constructor and `relayStreams` helper in package `server_test`.
- **`pkg/server/deadline_test.go`**: Dedicated unit tests verifying deadline renewal on half-window decay ($R \le \tau/2$), timeout mutation, zero clearing, interface forwarding, and concurrent access safety.
- **Automated Verification Suites in `pkg/server/server_test.go` & `pkg/config/config_test.go`**:
  - `TestServer_DeadlineAmortization_SyscallReduction` (TC-126.1: $>99.8\%$ syscall reduction).
  - `TestServer_IdleTimeout_StrictEnforcement` (TC-126.3: immediate idle disconnect without bleed).
  - `TestServer_Streaming_SurvivesPastWriteTimeout` (TC-126.4: persistent streaming survival).
  - `TestServer_Streaming_SlowReadClientTerminated` (TC-126.5: CWE-400 slow-read client teardown).
  - `TestServer_RelayStreams_AmortizationAndIdle` (TC-126.6: tunnel amortization and idle teardown).
  - `TestServer_ZeroTimeout_NoSyscalls` (TC-126.7: zero syscalls in zero-timeout mode).
  - `TestConfig_ServerTimeouts_ValidationAndZeroSupport` (TC-126.8: non-negative duration validation).
  - `TestServer_DeadlineAmortization_ConcurrencyRaceSafety` (TC-126.10: 100-client concurrent race verification).

### Changed
- **`pkg/server/server.go`**:
  - Wrapped client connections in `connDeadlineTracker` in `handleConn`.
  - Integrated strict idle transition state machine checking `br.Buffered() == 0`.
  - Replaced static write deadline in `res.StreamBody` with per-chunk activity-refreshed write deadlines.
  - Updated `relayStreams` to wrap connections in deadline trackers with amortized I/O and half-close propagation.
  - Implemented `Unwrap()`, `CloseWrite()`, and `SyscallConn()` on `prefixConn`.
- **`pkg/config/loader.go`**:
  - Added non-negative validation for `read_timeout`, `write_timeout`, `idle_timeout`, and `upgrade_idle_timeout` in `ValidateConfig`.
  - Preserved explicit `0` values in `validateConfigDefaults`.
- **`docs/wiki/reference/config-options.md`**: Updated server timeout definitions, non-negative validation rules, and zero-timeout mode documentation.
- **`docs/wiki/features/event-reactor.md`**: Expanded core event reactor documentation with deadline amortization algorithms, idle transition state machines, and streaming activity refreshes.
- **`docs/wiki/features/reverse-proxy.md`**: Documented persistent streaming response fast-paths and Slow-Read DoS (CWE-400) protection.

### Fixed
- **Kernel Deadline Syscall Contention at Scale**: Eliminated ~49,000 redundant socket deadline system calls per second under keep-alive saturation workloads.
- **Premature Teardown of Long-Lived Streams**: Fixed premature disconnect of Server-Sent Events (SSE) and live telemetry streams caused by static write deadlines exceeding 5s.
- **Vulnerability to Slow Read DoS (CWE-400)**: Prevented malicious zero-window or stalled clients from holding worker goroutines and origin connections open indefinitely.

### Related Tasks & Requirements
- `REQ-126`: Adaptive Socket Deadline Amortization and Slowloris Protection Optimization
- `TASK-149`: Implement Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts
- `ADR-126`: Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Architecture
- `TC-126`: Verification of Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts
- `CR-122`: Code Review of Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts
- `SR-126`: Security Review of Adaptive Socket Deadline Amortization and Activity-Refreshed Streaming Timeouts

---

## 2026-09-12 - Toron v1.5.24 Benchmark Release (Heterogeneous Multi-Hop Live Docker Harness Network Execution Architecture - REQ-120 / TASK-143)

### Milestone Summary
- **Heterogeneous Multi-Hop Live Docker Harness Network Execution Architecture (REQ-120, TASK-143, ADR-120, TC-120, CR-116, SR-120)**: Implemented a unified dual-mode network execution architecture for the multi-hop testbed (`benchmarks/multihop/runner.go`), enabling seamless operation in both `--standalone` (pure in-process CI mode) and `--docker` (live multi-container cluster mode via Docker Compose).
- **Nil Pointer Dereference Elimination (`SetupLiveTestbed`)**: Resolved the runtime panic at `runner.go:687` during live Docker execution by explicitly constructing non-nil backend descriptors (`SimulatedBackend`) for Node.js 20 LTS (`llhttp`), Python 3.11 (`uvicorn / h11`), and Go 1.24 (`net/http`) with safe teardown guards (`sb.Server != nil`).
- **Wire-Level Protocol Execution Adapters**: Eliminated in-memory handler dispatch (`HTTP2AdapterHandler().ServeHTTP`) in favor of true physical network socket operations over TCP/h2c:
  - **Cleartext HTTP/2 (`h2c`) Prior Knowledge Client**: `executeH2CRequest` via `golang.org/x/net/http2.Transport` with custom cleartext TCP dialer and bounded 3-second deadlines.
  - **Raw HTTP/2 Wire Framing Adapter**: `executeH2WireProbe` serializing HTTP/2 connection preface (`PRI * HTTP/2.0...`), SETTINGS, and HPACK-encoded HEADERS/DATA frames over raw TCP to test RFC compliance for attack vectors (`VECTOR-01` [H2.TE], `VECTOR-02` [H2.CL-Duplicate], `VECTOR-03` [H2.CL-Mismatch], `VECTOR-07` [CRLF-Header-Injection]) that standard client libraries sanitize client-side.
  - **Raw TCP Socket Stream Probing**: `executeRawSocketProbe` validating fail-fast socket teardown (`FIN`/`RST`) for HTTP/1.1 smuggling vectors (`VECTOR-04`, `VECTOR-05`, `VECTOR-06`).
- **Wire-Level Upstream Header Isolation Verification (`VECTOR-08`)**: Replaced in-memory struct reflection with response body JSON payload inspection via `parseAndValidateEchoHeaders`, verifying that zero colon-prefixed (`:`) pseudo-headers leak into upstream origins during HTTP/2-to-HTTP/1.1 translation (RFC 7540 §8.1.2.1).
- **Two-Stage Canary Protocol ($r_{\text{poison}} \,\|\, r_{\text{benign}}$)**: Confirmed 100% pass rate (30/30 scenarios) with strictly 0 desynchronization events and 0 pool poisoning events across all three production runtimes.
- **Result Retention Compliance (`REQ-119`)**: Preserved dual-path retention and atomic `manifest.json` tracking across both `--standalone` and `--docker` executions, enabling seamless execution within the master benchmark suite orchestrator (`./benchmarks/run_all.sh --auto-start --docker`).
- **Zero Third-Party Dependencies & Concurrency Safety**: Maintained zero third-party dependencies using exclusively the Go standard library and vendored `golang.org/x/net/http2`; 100% race-free under `go test -race -count=1 ./benchmarks/multihop/...`.

### Added
- **`SetupLiveTestbed`**: Live environment constructor in `benchmarks/multihop/runner.go` initializing non-nil backend target descriptors.
- **`executeH2WireProbe`**: Raw TCP HTTP/2 wire framing adapter delivering connection preface, SETTINGS, and raw HPACK frames directly over TCP.
- **`executeH2CRequest`**: Cleartext prior-knowledge HTTP/2 client transport utilizing `golang.org/x/net/http2.Transport`.
- **`parseAndValidateEchoHeaders`**: Wire-level response payload parser asserting zero colon-prefixed pseudo-headers leaked in origin `/echo` responses.
- **`docs/wiki/features/multihop-testbed.md`**: Dedicated documentation wiki for the Heterogeneous Multi-Hop Backend Origin Testbed Architecture.
- **Unit & Integration Tests**: Added `TestMultiHop_LiveModeInitialization`, `TestMultiHop_PseudoHeaderEchoVerification`, and `TestMultiHop_LiveModeSimulation` in `benchmarks/multihop/multihop_test.go`.

### Changed
- **`benchmarks/multihop/runner.go`**: Refactored `ExecuteScenario`, `Teardown`, and `main()` to support wire-level network execution and eliminate nil dereferences.
- **`benchmarks/multihop/run_multihop.sh`**: Enhanced container startup readiness checks and trap cleanup.
- **`benchmarks/README.md`**: Expanded Section 5 to document dual execution modes, wire-level protocol adapters, SetupLiveTestbed architecture, and echo header isolation.
- **`docs/wiki/features/benchmarking.md`**: Updated multi-hop execution commands and linked dedicated architecture documentation.
- **`docs/wiki/index.md`**: Added navigation link to `multihop-testbed.md`.

### Related Tasks & Requirements
- `REQ-120`: Heterogeneous Multi-Hop Live Docker Harness Network Execution Architecture
- `TASK-143`: Heterogeneous Multi-Hop Live Docker Harness Network Execution Implementation
- `ADR-120`: Heterogeneous Multi-Hop Live Docker Harness Network Execution Architecture
- `TC-120`: Test Specification for Heterogeneous Multi-Hop Live Docker Harness Network Execution
- `CR-116`: Code Review for Heterogeneous Multi-Hop Live Docker Harness Network Execution Architecture
- `SR-120`: Security Review for Heterogeneous Multi-Hop Live Docker Harness Network Execution Architecture

---

## 2026-09-12 - Toron v1.5.23 Benchmark Release (Historical Benchmark Result Retention and Manifest Architecture - REQ-119 / TASK-142)

### Milestone Summary
- **Historical Benchmark Result Retention & Structured Manifest Architecture (REQ-119, TASK-142, ADR-119, TC-119, CR-115, SR-119)**: Implemented an automated dual-path retention and manifest cataloging system across all six benchmark subsystems in Toron.
- **Dual-Path Retention Model**: Every benchmark execution session creates an immutable historical directory under `benchmarks/results/history/<timestamp>/` (`YYYY-MM-DD_HH-MM-SS`) containing all generated artifacts and `session_meta.json`, while simultaneously synchronizing canonical latest files in `benchmarks/results/` for 100% backward compatibility with academic paper citations, Markdown links, and automated CI pipelines.
- **Central Structured Index (`benchmarks/results/history/manifest.json`)**: Machine-readable JSON catalog recording every benchmark execution session, including run ID, timestamp, suite, command, git commit, git branch, Go runtime version, duration, parameter dictionary, exit status, and generated artifact file list.
- **Atomic Concurrency Protection (CWE-362 / CWE-377)**: Implemented atomic manifest and metadata updates using temporary file writes and atomic POSIX renames (`os.Rename`), completely preventing partial file writes, data loss, or JSON corruption during concurrent runs or process termination.
- **Master Orchestrator Consolidation (`benchmarks/run_all.sh`)**: Integrated session inheritance across all six evaluation stages (microbenchmarks, wrk2, saturation stress, differential fuzzer, multi-hop testbed, and controlled ablation), bundling all generated artifacts into a unified historical session archive and registering a consolidated suite entry in `manifest.json`.
- **Standalone Runner Independence**: Extended individual benchmark scripts (`run_wrk2.sh`, `run_saturation_stress.sh`, `run_fuzzer.sh`, `run_multihop.sh`, `run_ablation.sh`) to support independent timestamp directory creation and manifest registration when executed outside `run_all.sh`.
- **CLI Configurability**: Added universal support for `--no-history` (bypassing historical writes for ephemeral passes), `--session-name <name>` (custom naming suffixes), and `--session-dir <dir>` (explicit destination directories).
- **Zero Third-Party Dependencies & Race-Free Concurrency**: Preserved zero third-party dependencies using exclusively the Go standard library and POSIX shell utilities. Passed full race detection verification under `go test -race -count=1 ./benchmarks/...`.

### Added
- **`benchmarks/retention/retention.go`**: Core retention engine providing timestamp formatting, collision handling, dual-path artifact replication, atomic manifest updates, and git metadata extraction.
- **`benchmarks/retention/cmd/main.go`**: Command-line interface supporting `init`, `archive`, and `record` actions.
- **`benchmarks/retention/retention_test.go`**: Unit tests verifying timestamp formatting, directory creation, collision avoidance, artifact replication, and concurrent manifest update safety.
- **`benchmarks/retention/integration_test.go`**: End-to-end integration test validating full CLI workflow, artifact parity, and multi-run append behavior.
- **`benchmarks/archive_run.sh`**: POSIX shell helper library for session initialization, artifact replication, and manifest registration.
- **`benchmarks/results/history/manifest.json`**: Central historical manifest index.

### Changed
- **`benchmarks/run_all.sh`**: Integrated master session provisioning, child stage export, stage artifact replication, and master manifest registration.
- **`benchmarks/wrk2/run_wrk2.sh`**: Added dual-path retention and manifest registration.
- **`benchmarks/wrk2/run_saturation_stress.sh`**: Added dual-path retention and manifest registration.
- **`benchmarks/fuzzer/run_fuzzer.sh`**: Added dual-path retention and manifest registration.
- **`benchmarks/multihop/run_multihop.sh`**: Added dual-path retention and manifest registration.
- **`benchmarks/ablation/run_ablation.sh`**: Added dual-path retention and manifest registration.
- **`docs/wiki/features/benchmarking.md`**: Updated documentation detailing the dual-path retention architecture, directory structure, manifest schema, and CLI options.

### Related Tasks & Requirements
- `REQ-119`: Benchmark Result Retention and Historical Run Manifest Architecture
- `TASK-142`: Implement Historical Benchmark Result Retention and Manifest Architecture (REQ-119)
- `ADR-119`: Dual-Path Benchmark Result Retention and Atomic Manifest Architecture (REQ-119)
- `TC-119`: Verification of Benchmark Result Retention, Directory Isolation, and Manifest Integrity (REQ-119)
- `CR-115`: Code Review for Benchmark Result Retention and Historical Run Manifest Architecture
- `SR-119`: Security & Operational Resilience Review for Benchmark Retention Architecture

---

## 2026-09-12 - Toron v1.5.22 Benchmark Release (Latency Distributional Metric Calibration in Differential Fuzzer - HARN-02 / TASK-141 / REQ-118)

### Milestone Summary
- **Latency Distributional Metric Calibration (HARN-02, TASK-141, REQ-118, BMK-02)**: Completely resolved distributional reporting ambiguities and percentile conflation in Toron's Differential Protocol Security Fuzzer (`benchmarks/fuzzer/diff_fuzzer.go`, `ADR-118`, `CR-114`, `SR-118`, `TC-118`).
- **Disaggregation of Fail-Fast Rejections from Baseline Traffic**: Partitioned the 19 invariant vectors into two distinct analytical cohorts:
  1. **Fail-Fast Defense Latency ($N=18$ Adversarial Vectors)**: Mean, Median ($p50$), 90th Percentile ($p90$), and Max Rejection Latency, isolating active security defense from benign traffic.
  2. **Cross-Vector Comprehensive Latency ($N=19$ Vectors, incl. `BASELINE-001` 200 OK)**: Mean, Median ($p50$), 90th Percentile ($p90$), 99th Percentile ($p99$), and Max Latency across the entire test suite.
- **Unconditional Percentile Reporting Across Execution Modes**: Eliminated conditional suppression of percentiles, ensuring $p50$, $p90$, and $p99$ are output distinctly in terminal stdout summaries, JSON reports, and Markdown reports across both single-shot ($K=1$) and repeated statistical trial ($K > 1$) modes.
- **Elimination of Ambiguity Between p90 and p99**: Explicitly decoupled $p90$ ($712.08\ \mu\text{s}$) from $p99$ ($2,378.00\ \mu\text{s}$), resolving reviewer critiques in `AER-001.md`, `MSR-001.md`, and `AR-001.md` and providing ground truth alignment for Paper 1 Section 5.2 (REV-02).
- **Backwards-Compatible JSON Telemetry Serialization**: Augmented `DifferentialReport` with nested `fail_fast_defense` and `comprehensive_suite` of type `CohortMetrics` while preserving top-level `average_latency_us`, `median_latency_us`, `p90_latency_us`, and `p99_latency_us`.
- **Zero Race Verification & Zero External Dependencies**: Preserved zero third-party dependencies and verified race-clean execution under `go test -race -count=1 ./benchmarks/fuzzer/...`.

### Fixed
- **Single-Shot Percentile Suppression**: Resolved logic in `diff_fuzzer.go` that previously withheld $p50$, $p90$, and $p99$ when executing in single-shot mode ($K=1$).
- **Conflation of Rejection and Baseline Traffic**: Resolved aggregated reporting that previously mixed benign $200\text{ OK}$ payload transfer with fail-fast socket teardown.
- **Ambiguous Combined Report Header**: Replaced combined `Tail Latency (p90 / p99)` line with distinct, structured metric rows.

### Changed
- **`benchmarks/fuzzer/diff_fuzzer.go`**: Added `CohortMetrics`, `calculateCohortMetrics`, disaggregated cohort calculations, updated terminal summary, and updated Markdown report generator.
- **`benchmarks/fuzzer/diff_fuzzer_test.go`**: Added `TestCalculateCohortMetrics` and `TestDifferentialReport_JSONSerialization`.
- **Artifacts**: Regenerated `benchmarks/results/differential_fuzz_report.json` and `benchmarks/results/differential_fuzz_report.md`.

### Added
- **`docs/wiki/features/differential-fuzzer-metrics.md`**: New user documentation covering metric cohort architecture, CLI options, and artifact schemas.

---

## 2026-09-12 - Toron v1.5.21 Benchmark Release (Disaggregated Adversarial Status Classification and Route-Miss Separation in Load Generator - HARN-01 / TASK-140 / REQ-117)

### Milestone Summary
- **Remediation of Circular Defense Scoring Anomaly (HARN-01, TASK-140, REQ-117, BMK-04)**: Completely resolved the circular defense scoring defect and route-miss conflation anomaly (`ADR-117`, `CR-113`, `SR-117`, `TC-117`) in Toron's high-concurrency saturation benchmark load generator harness (`benchmarks/wrk2/loadgen.go`).
- **Elimination of Circular Catch-All Else Block (`benchmarks/wrk2/loadgen.go`)**: Permanently removed the circular `else { attackRejected.Add(1); stats.rejected.Add(1) }` fallback (previously lines 382–386). This catch-all had indiscriminately scored any non-200 HTTP response—including passive `404 Not Found` responses (such as 349 path traversal directory escape probes under `ADV-06`) and server errors (`500 Internal Server Error`)—as active security defense blocks.
- **Strict Four-Tier Mutually Exclusive Status Classification Taxonomy**: Replaced ambiguous branch logic with an exhaustive `switch code` construct establishing four architectural tiers for adversarial probe evaluation:
  1. **Tier 1 (Active Defense - `attackRejected`)**: Explicit defensive rejections (`400 Bad Request`, `403 Forbidden`, `413 Payload Too Large`, `431 Request Header Fields Too Large`, `501 Not Implemented`) and transport-level socket resets.
  2. **Tier 2 (Route Miss - `attackRouteMiss`)**: Unmatched URI paths reaching passive 404 handlers (`404 Not Found`). Explicitly isolated from defense metrics.
  3. **Tier 3 (Attack Bypass - `attackBypassed`)**: Invariant violations accepted and processed successfully (`200 OK` or unexpected 2xx/3xx).
  4. **Tier 4 (Unhandled / Protocol Anomaly - `attackUnhandled`)**: Unexpected server crashes, panics, or transport desynchronizations (`500 Internal Server Error`, 5xx, or non-standard codes).
- **Explicit RFC 6585 Status 431 Active Defense Inclusion**: Formally incorporated HTTP `431 Request Header Fields Too Large` into Tier 1 active defense, correctly reflecting Toron's bounded header parser protections against oversized header blocks (`ADV-07`) without relying on fallback logic.
- **Route-Miss Isolation & Strict Overall Verdict Logic**: Enforced that `404 Not Found` responses are captured in dedicated 64-bit atomic counters (`attackRouteMiss`, `stats.routeMiss`). Any non-zero route miss, attack bypass, or unhandled anomaly prevents achieving 100.0% active defense, reduces `ActiveDefenseRatePct`, and immediately marks `OverallVerdict = "FAIL"`.
- **Publication-Grade Table 6 Alignment & Telemetry Parity**: Updated Markdown report generation (`GenerateMarkdownReport`), JSON serialization, and CSV exports to output full 9-column disaggregated breakdowns (`Vector ID`, `Attack Name`, `Category`, `Probes Sent`, `Active Defense (4xx/501)`, `Route Miss (404)`, `Bypass Count (200)`, `Unhandled`, `Active Defense Rate`), aligning directly with Paper 1 and Paper 2 manuscript Table 6 requirements (`REV-03`).
- **Sub-5ns Hot-Path Overhead & Zero Heap Allocations (NFR-3)**: Implemented the four-tier classification using direct integer switch evaluation with zero heap allocations on the critical benchmarking path, maintaining load generator throughput fidelity beyond 10,000 RPS.
- **Zero Race Concurrency Verification (`TC-117`)**: Comprehensive automated verification suite verifying all four tiers, AST absence of catch-all else, markdown table formatting, JSON/CSV parity, and race freedom under `go test -race -count=1 ./benchmarks/wrk2/...`.

### Fixed
- **Circular Catch-All Else Scoring Anomaly (`BMK-04`, `HARN-01`)**: Fixed circular fallback in `loadgen.go` where any non-200 status code was scored as `attackRejected`, falsely crediting passive 404 route misses as active security defense.
- **Masking of Routing Invariant Deficiencies**: Eliminated false-positive defense classification of 349 path traversal probes (`ADV-06`) that had previously returned `404 Not Found` due to premature path canonicalization prior to the fix in `REQ-116`/`TASK-139`.
- **Missing RFC 6585 Status 431 Active Defense Code**: Fixed omission of HTTP 431 in explicit active defense checks, which previously relied on accidental fallback.
- **Anomaly and Server Crash Masking**: Prevented 5xx internal server errors, unhandled panics, or transport anomalies from inflating active defense counts.

### Changed
- **Benchmark Load Generator Core (`benchmarks/wrk2/loadgen.go`)**:
  - Replaced conditional branch logic in worker goroutine with exhaustive `switch code` dispatch.
  - Added atomic counters `attackRouteMiss`, `attackUnhandled`, and expanded `vecStats` with `routeMiss` and `unhandled`.
  - Augmented `StreamMetrics` struct with `ActiveDefenseRequests`, `RouteMissRequests`, `BypassedRequests`, `UnhandledRequests`.
  - Augmented `AttackVectorSummary` struct with `RouteMiss`, `Unhandled`, `ActiveDefenseRatePct`, `RouteMissRatePct`.
  - Augmented `SaturationStressReport` struct with `ActiveDefenseRatePct`, `RouteMissRatePct`, and strict overall verdict logic.
  - Updated `GenerateMarkdownReport` Section 1, Section 2, and Section 4 to format Table 6 with disaggregated columns.
  - Updated CLI summary output and CSV exporter to output disaggregated four-tier telemetry.

### Added
- **User-Facing Documentation (`docs/wiki/features/saturation-stress-benchmark.md`)**: Comprehensive documentation detailing high-concurrency saturation stress benchmarking, the four-tier status classification taxonomy, Table 6 metrics alignment, and CLI flags.
- **Automated Verification Suite (`benchmarks/wrk2/loadgen_test.go`)**:
  - `TestLoadGen_ClassificationLogic`: Validates classification across 400, 403, 413, 431, 501, socket reset, 404, 200, 500, 502, and 999.
  - `TestLoadGen_StatusClassification_FourTiers`: End-to-end multi-status mock server verification of atomic counters and vector telemetry.
  - `TestLoadGen_RouteMissFailsVerdict`: Asserts 404 increments route miss counter and fails overall verdict.
  - `TestLoadGen_BypassFailsVerdict` & `TestLoadGen_AttackBypassFailsVerdict`: Asserts 200 increments bypass counter and fails overall verdict.
  - `TestLoadGen_UnhandledAnomalyFailsVerdict`: Asserts 500 increments unhandled counter and fails overall verdict.
  - `TestLoadGen_AST_NoCatchAllElse`: Static AST analysis verifying complete elimination of circular `else` blocks in `loadgen.go`.
  - `TestLoadGen_MarkdownTable6Format` & `TestLoadGen_ReportGeneration_Table6`: Validates 9-column publication-grade Table 6 Markdown formatting.
  - `TestLoadGen_TelemetryParity_JSON_CSV`: Asserts 100% telemetry parity between memory, JSON, and CSV.
  - `TestLoadGen_RaceFreeConcurrency`: Asserts partition invariants and race freedom under concurrent load.

### Related Tasks & Requirements
- `TASK-140`: Implementation of Disaggregated Status Classification and Route-Miss Separation in Benchmark Load Generator (HARN-01)
- `REQ-117`: Disaggregated Adversarial Status Classification and Route-Miss Separation in Benchmark Load Generator (HARN-01)
- `ADR-117`: Disaggregated Adversarial Status Classification and Route-Miss Separation in Benchmark Load Generator (HARN-01)
- `TC-117`: Automated Verification Suite for Benchmark Load Generator Status Classification
- `CR-113`: Code Review for Disaggregated Adversarial Status Classification and Route-Miss Separation
- `SR-117`: Security & Empirical Review for Table 6 Benchmark Alignment & Invariant Scoring Audit
- `REQ-114` / `TASK-137`: High-Concurrency Saturation Stress Testing with Background Traffic (BMK-04)
- `REQ-116` / `TASK-139`: Layered Route-Aware Path Traversal Defense Architecture (CWE-22)

## 2026-09-12 - Toron v1.5.20 Security Release (Layered Route-Aware Path Traversal Defense Architecture - CWE-22 / TASK-139 / REQ-116)

### Milestone Summary
- **Remediation of Path Traversal Security Vulnerability (CWE-22, REQ-116, TASK-139)**: Successfully resolved directory traversal, path canonicalization bypass, routing desynchronization, and defensive masking vulnerabilities ([CWE-22](https://cwe.mitre.org/data/definitions/22.html), [CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), `ADR-116`, `CR-112`, `SR-116`) in Toron's core router and Web Application Firewall (WAF) engines.
- **Elimination of Router Pre-Routing Canonicalization Dead-Code Contradiction (`pkg/router/router.go`)**: Permanently eliminated the structural defect where `cleanRequestPath(req.Path)` in `Router.ServeHTTP` prematurely collapsed dot-dot sequences before route matching. For requests targeting static mounts (e.g., `GET /internal/dashboard/../../canary_traversal.txt HTTP/1.1`), the sanitized path `/canary_traversal.txt` failed to match the prefix route, silently falling through to `r.NotFound` (`404 Not Found`) with open keep-alive connections. This rendered the explicit path traversal check in `Router.createStaticHandler` (`filepath.Rel`) unreachable dead code.
- **Layer 1: Layer 7 WAF Raw Wire URI Inspection (`pkg/waf/waf.go`)**: Extended `WAFEngine.InspectToron` and `WAFEngine.Inspect` to extract the uncleaned raw wire path directly from `req.RequestURI`, stripping query strings (`?`) and URL fragments (`#`) via zero-allocation byte scanning. Implemented dual-path pattern evaluation across both the raw wire path (`urlPath`, preserving `%2e%2e`, `/../`, `%2E%2E`) and unescaped path (`normPath = url.PathUnescape(urlPath)`). Actively blocks traversal attempts matching rule `TRAVERSAL-001` with `403 Forbidden`, `Connection: close`, structured JSON payloads, and SIEM audit logging.
- **Layer 2: Route-Aware Static Prefix Escape Guard (`pkg/router/router.go`)**: Added standalone static prefix boundary validation in `Router.ServeHTTP` before fallback routing. Detects when an ingress raw or unescaped request path targets a registered static prefix route (`pr.routeType == string(RouteTypeStatic)`), but canonicalizes to a path escaping that prefix boundary. Actively rejects with `403 Forbidden: Path Traversal Disallowed` and `Connection: close`, ensuring fail-fast containment even if the WAF engine is disabled, bypassed, or in detection-only mode.
- **Fail-Fast Transport Socket Teardown Enforcement (`REQ-107`, `ADR-107`)**: Strictly enforced transport socket closure on all security rejections at both Layer 1 and Layer 2 via `res.Header.Set("Connection", "close")`. The reactor event loop terminates the persistent connection immediately upon response delivery, closing the TCP socket and eliminating HTTP pipelined request smuggling ([CWE-444](https://cwe.mitre.org/data/definitions/444.html)) and persistent connection descriptor exhaustion ([CWE-400](https://cwe.mitre.org/data/definitions/400.html)).
- **Preservation of ADR-062 Canonicalization on Non-Static Routes**: Confirmed that exact API routes (`r.GET`, `r.POST`) and upstream reverse proxy routes (`RouteTypeUpstream`) retain clean two-stage RFC 3986 / ADR-062 canonicalization, allowing legitimate relative subpath routing (e.g. `GET /public/../internal` resolving to `/internal`) without false-positive escape blocks.
- **100.0% Differential Protocol Security Fuzzer Pass Rate (19/19 Tests)**: Elevated Toron's differential protocol security invariant pass rate from 89.47% (17/19) to **100.0% (19/19)** across all 10 security categories in `benchmarks/fuzzer/`, with `TRAVERSAL-001` (Raw dot-dot), `TRAVERSAL-002` (Uppercase `%2E%2E`), and `TRAVERSAL-003` (Double percent-encoding) achieving active defense compliance (`403 Forbidden` with physical socket teardown).
- **Sub-Microsecond Fail-Fast Rejection Latency (NFR-2)**: Maintained high-performance rejection latency budgets (Mean $< 300\ \mu\text{s}$, Median $< 70\ \mu\text{s}$) with zero heap allocations on common benign request paths.
- **Zero External Dependencies**: Implemented strictly using pure Go standard library packages (`bytes`, `fmt`, `net/http`, `net/url`, `path`, `path/filepath`, `strings`, `sync`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-116`)**: Fully validated via 10 automated test scenarios across `pkg/waf/waf_test.go`, `pkg/router/router_test.go`, and `benchmarks/fuzzer/diff_fuzzer_test.go`, confirming raw, uppercase, double-encoded, query-stripped, fallback, legitimate static, and non-static routing behaviors with complete data race cleanliness under `go test -race -count=1 ./...` (`TASK-139`, `TC-116`).

### Fixed
- **Premature Router Path Canonicalization & Defensive Masking (`CWE-22`, `REQ-116`)**: Fixed vulnerability where `cleanRequestPath(req.Path)` in `Router.ServeHTTP` collapsed directory traversal dot-dot segments before route matching, causing static prefix escapes to bypass route handlers and return `404 Not Found` rather than active security rejections.
- **Static Handler Directory Containment Dead-Code Contradiction**: Resolved architectural defect where the explicit path traversal and directory boundary containment check in `Router.createStaticHandler` (`filepath.Rel`) was rendered unreachable dead code because escaped paths never matched static prefix routes.
- **WAF Layer 7 Raw Wire URI Blind Spot (`CWE-22`, `REQ-116`)**: Fixed vulnerability where `WAFEngine.InspectToron` inspected pre-sanitized `req.Path`, allowing raw and percent-encoded traversal sequences to bypass rule `TRAVERSAL-001`.
- **Persistent Keep-Alive Socket Vulnerability on Security Rejections (`CWE-444`, `CWE-400`, `REQ-107`)**: Fixed missing transport teardown on traversal rejections, ensuring every 403 response injects `Connection: close` and triggers immediate TCP socket closure.

### Changed
- **Router Core (`pkg/router/router.go`)**:
  - Captured `rawPath` from `req.RequestURI` (stripping query strings and fragments) and computed iteratively unescaped candidate (`url.PathUnescape`) prior to path canonicalization.
  - Implemented Route-Aware Static Prefix Escape Guard in `Router.ServeHTTP` before fallback routing: evaluates targeting and escaping predicates against all registered `RouteTypeStatic` prefix routes.
  - Injected `res.SetStatus(http.StatusForbidden)`, `Connection: close`, `Content-Type: application/json`, and body `{"error":"403 Forbidden: Path Traversal Disallowed"}` upon prefix escape detection.
- **WAF Engine (`pkg/waf/waf.go`)**:
  - Updated `WAFEngine.Inspect` and `WAFEngine.InspectToron` to extract the raw wire path from `req.RequestURI` (stripping `?` query and `#` fragment) and fall back to `req.Path` only when `RequestURI` is empty.
  - Updated `WAFEngine.inspectInternal` to evaluate all rules targeting `InspectURL` against both the raw wire URL path (`urlPath`) and the unescaped path (`normPath = url.PathUnescape(urlPath)`).
- **Differential Security Fuzzer (`benchmarks/fuzzer/diff_fuzzer.go`, `benchmarks/fuzzer/diff_fuzzer_test.go`)**:
  - Realigned test oracle for `TRAVERSAL-001` and `TRAVERSAL-002` to require `ExpectClose: true` in accordance with `REQ-107` and `REQ-116`.
  - Updated mock test server fixtures to inject `Connection: close` and close underlying sockets.

### Added
- **User-Facing Documentation (`docs/wiki/features/path-traversal-defense.md`)**: Comprehensive documentation detailing the Layered Route-Aware Path Traversal Defense Architecture, WAF raw wire URI inspection, router prefix escape guards, sequence diagrams, decision flowcharts, configuration, and troubleshooting.
- **Automated Verification Suite (`pkg/waf/waf_test.go`, `pkg/router/router_test.go`)**:
  - `TestWAF_InspectToron_RawWireURI_Traversal` (`TC-116-01`): Validates WAF detection of raw dot-dot, uppercase `%2E%2E`, lowercase with query/fragment, encoded slashes, double percent-encoding, and legitimate requests.
  - `TestWAF_InspectToron_Fallback_EmptyRequestURI` (`TC-116-02`): Validates graceful fallback to `req.Path` on empty `RequestURI` without panics.
  - `TestRouter_RouteAwareStaticPrefixEscapeGuard` (`TC-116-03` through `TC-116-06`): Standalone router test validating raw dot-dot, uppercase encoded, double encoded, legitimate subpath access, and standalone protection with WAF disabled. Explicitly asserts zero canary secret data leakage (`TORON_TRAVERSAL_CANARY_SECRET_DATA_DO_NOT_LEAK`).
  - `TestRouter_ADR062_Canonicalization` (`TC-116-07`): Validates preservation of ADR-062 two-stage canonicalization on non-static exact API and upstream routes without false-positive blocks.

### Related Tasks & Requirements
- `TASK-139`: Implementation of Layered Route-Aware Path Traversal Defense Architecture (CWE-22)
- `REQ-116`: Layered Route-Aware Path Traversal Defense Architecture (CWE-22)
- `ADR-116`: Layered Route-Aware Path Traversal Defense Architecture (CWE-22)
- `TC-116`: Test Specification for Layered Route-Aware Path Traversal Defense Architecture (CWE-22)
- `CR-112`: Code Review for Layered Route-Aware Path Traversal Defense Architecture (CWE-22)
- `SR-116`: Security Review & CWE-22 Boundary Analysis for Layered Route-Aware Path Traversal Defense
- `REQ-107` / `ADR-107`: Fail-Fast Transport Socket Teardown Enforcement on Security Rejections (`Connection: close`)
- `REQ-067` / `ADR-062`: Upstream Path Canonicalization & Route Traversal Guards
- `REQ-102`: Canary Traversal Target Deployment & Differential Fuzzer Active Defense Oracle

## 2026-09-11 - Toron v1.5.19 Security Release (SEC-38: RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-38 (`pkg/acme/acme.go`)**: Successfully resolved Low-severity input validation, lock contention, and RFC non-compliance vulnerabilities `SEC-38` ([CWE-20](https://cwe.mitre.org/data/definitions/20.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-703](https://cwe.mitre.org/data/definitions/703.html), `SR-091 Finding 8`, `SR-100`, `CR-096`) in Toron's ACME HTTP-01 challenge responder.
- **Zero-Allocation RFC 8555 §8.3 Base64URL Validator (`IsValidACMEToken`)**: Implemented high-performance zero-allocation byte scanner enforcing $1 \le \text{len} \le 128$ and strictly the unpadded base64url character set (`[a-zA-Z0-9_-]`). Disallows padding (`=`), path separators (`/`, `\`), path traversal dots (`..`), whitespace, control characters, and non-ASCII bytes.
- **Fail-Fast Lock Isolation & DoS Elimination (CWE-20, CWE-400)**: Syntax and length validations execute prior to challenge registry lookup, preventing malformed, oversized, or adversarial request paths from acquiring `m.mu.RLock()` or triggering map hashing.
- **Elimination of Silent Whitespace Trimming**: Removed `strings.TrimSpace(token)`, ensuring tokens containing leading, trailing, or embedded whitespace strictly trigger `HTTP 400 Bad Request`.
- **HTTP Method Hardening (RFC 7231 §6.5.5 Compliance)**: Restricted `ServeHTTP01Handler` strictly to `GET` and `HEAD` methods. Disallowed methods (`POST`, `PUT`, `DELETE`, `PATCH`, `OPTIONS`, `CONNECT`, `TRACE`) immediately yield `405 Method Not Allowed` with mandatory headers `Allow: GET, HEAD` and `Content-Type: text/plain`.
- **RFC 7231 §4.3.2 Compliant HEAD Semantics**: Automated CA validation probes issuing `HEAD` requests receive `200 OK`, `Content-Type: text/plain`, and exact `Content-Length: len(keyAuth)` while strictly omitting the response body (`res.Body.Len() == 0`).
- **Clean 404 Response on Unregistered Valid Tokens**: Valid tokens not registered in the challenge table return `404 Not Found` with an explanatory error body for `GET` and an empty body for `HEAD`.
- **Zero External Dependencies**: Implemented strictly using the Go standard library (`crypto/*`, `net/http`, `strconv`, `strings`, `sync`, `time`) and internal `httpparser`.
- **Comprehensive Automated Verification Suite (`TC-100`)**: Verified across all 9 automated test scenarios in `pkg/acme/acme_test.go` (`TC-100-01` through `TC-100-09`) including high-concurrency race freedom under `go test -race` (`TASK-123`, `TC-100`).

### Fixed
- **Unvalidated Token Map Queries & Core Lock Contention (`SEC-38`, CWE-20, CWE-400)**: Fixed vulnerability where external callers could query arbitrary paths against internal token map under read locks without validation.
- **Permissive HTTP Method Acceptance (`SEC-38`, RFC 8555)**: Fixed endpoint accepting non-idempotent or arbitrary HTTP verbs, now rejecting them with `405 Method Not Allowed`.
- **Missing RFC 7231 HEAD Method Support (`SEC-38`)**: Fixed handler writing response bodies on `HEAD` requests, ensuring body is omitted while `Content-Length` is preserved.
- **Silent Whitespace Forgiveness (`SEC-38`, CWE-20)**: Fixed handler masking invalid whitespace characters via `strings.TrimSpace`.

### Changed
- **ACME Core (`pkg/acme/acme.go`)**:
  - Added public `IsValidACMEToken(token string) bool`.
  - Updated `ServeHTTP01Handler` to validate method (`GET` and `HEAD` only), remove whitespace trimming, validate token syntax and length, guard response body for `HEAD`, and return appropriate status codes (`400`, `404`, `405`).

### Added
- **Automated Verification Suite (`pkg/acme/acme_test.go`)**:
  - `TestACME_HTTP01_ValidToken_GET_Success` (TC-100-01)
  - `TestACME_HTTP01_EmptyToken_Rejection` (TC-100-02)
  - `TestACME_HTTP01_TokenLengthBoundary` (TC-100-03)
  - `TestACME_HTTP01_InvalidCharacters_Rejection` (TC-100-04)
  - `TestACME_HTTP01_PathTraversal_Rejection` (TC-100-05)
  - `TestACME_HTTP01_MethodValidation_405MethodNotAllowed` (TC-100-06)
  - `TestACME_HTTP01_HEAD_Semantics` (TC-100-07)
  - `TestACME_HTTP01_NonExistentToken_404NotFound` (TC-100-08)
  - `TestACME_HTTP01_HighConcurrency_RaceSafety` (TC-100-09)
  - `TestACME_IsValidACMEToken` (Unit test)

### Related Tasks & Requirements
- `TASK-123`: RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler
- `REQ-100`: RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler
- `ADR-100`: RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler
- `TC-100`: Automated Verification Suite for RFC 8555 Token Syntax Validation and Method Hardening in ACME Handler
- `CR-096`: Code Review of RFC 8555 Token Syntax & Length Validation and HTTP Method Hardening in ACME HTTP-01 Challenge Handler (SEC-38)
- `SR-100`: Security Review and Vulnerability Assessment of SEC-38 Remediation
- `SEC-38`: Missing Token Syntax and Length Validation in ACME HTTP-01 Challenge Handler

## 2026-09-11 - Toron v1.5.18 Security Release (SEC-37: Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-37 (`pkg/sidecar`, `pkg/proxy`)**: Successfully resolved Medium-severity unbounded transport allocation, memory exhaustion, and socket descriptor leak vulnerabilities `SEC-37` ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-772](https://cwe.mitre.org/data/definitions/772.html), `SR-091 Finding 7`, `SR-099`, `CR-095`) in the Service Mesh Sidecar Proxy engine.
- **Elimination of Per-Request ReverseProxy and Transport Allocation**: Replaced historical per-request instantiation of `*proxy.ReverseProxy`, `*http.Client`, and `*http.Transport` in `proxyToURL` with thread-safe origin-keyed caching in `ProxyEngine.proxies` (`map[string]*proxy.ReverseProxy`) using double-checked locking protected by `sync.RWMutex`. In-flight requests targeting cached origins proceed with lock-free read performance.
- **Canonical Target Origin Normalization ($O(U)$ Bounded Memory)**: Implemented `normalizeTargetOrigin(rawURL)` to extract canonical `scheme://host[:port]` keys, stripping variable dynamic paths, query parameters, and fragments. Memory utilization scales strictly with the number of unique upstream microservices ($O(U)$), completely preventing cache key explosion.
- **HTTP Keep-Alive Connection Pooling & TCP Socket Reuse**: Outbound egress traffic now reuses persistent keep-alive TCP connections across successive and concurrent requests directed at the same backend microservice ($\le 2$ active TCP sockets per backend under steady traffic), slashing connection latency to near zero and eliminating GC thrashing.
- **Idle Transport Connection Teardown (`ReverseProxy.Close()` & `ProxyEngine.Stop()`)**: Extended `ReverseProxy.Close()` to invoke `tr.CloseIdleConnections()` on `p.Client.Transport.(*http.Transport)`. When `ProxyEngine.Stop()` executes during pod shutdown, it cleanly iterates through all cached proxies and releases all idle TCP sockets, permanently eliminating file descriptor leaks (`EMFILE`).
- **Egress Client mTLS Propagation via `ProxyOptions.TLSClientConfig`**: Added `TLSClientConfig *tls.Config` to `ProxyOptions`, attaching pre-compiled client TLS configurations directly to cached reverse proxies. Egress HTTPS/mTLS connections now seamlessly present client certificates and validate custom CA roots in compliance with `REQ-097` / `SEC-35`.
- **Zero External Dependencies**: Implemented strictly using the Go standard library (`sync`, `net/http`, `crypto/tls`, `net/url`, `io`, `fmt`, `time`, `strings`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-099`)**: Fully validated via unit and integration tests TC-099-01 through TC-099-07, confirming singleton proxy reuse across 50 sequential requests, multi-target backend isolation, 50-worker high-concurrency race freedom under `go test -race`, stop lifecycle teardown, egress client mTLS propagation, canonical origin normalization across 9 patterns, and `ReverseProxy.Close()` idle connection cleanup (`TASK-122`, `TC-099`).

### Fixed
- **Unbounded Transport & Socket Descriptor Allocation (`SEC-37`, CWE-400, CWE-772)**: Fixed vulnerability where every outbound egress request created a new `*http.Transport` connection pool that was never reused or closed, leading to rapid socket descriptor exhaustion (`EMFILE`) and process crashes under load.
- **Missing Idle Connection Teardown on Proxy Teardown (`SEC-37`, CWE-772)**: Fixed `ReverseProxy.Close()` omission of `tr.CloseIdleConnections()`, which previously orphaned idle TCP sockets upon route removal or proxy closure.
- **Cache Key Proliferation Risk**: Preempted cache map bloating by normalizing raw URLs to canonical `scheme://host[:port]` origins before caching.
- **Egress Client mTLS Disconnect**: Fixed omission of client TLS configurations in `proxyToURL`, ensuring outbound egress proxying adheres to sidecar mTLS settings.
- **TLSClientConfig NextProtos Concurrency Data Race**: Resolved Go standard library data race during concurrent TLS handshakes by implementing two-tier defensive cloning (`p.clientTLS.Clone()` in `getOrCreateProxy` and `opts.TLSClientConfig.Clone()` in `NewProxyWithOptions`), ensuring each `http.Transport` operates on an independent `*tls.Config` instance.

### Changed
- **Reverse Proxy Core (`pkg/proxy/proxy.go`)**:
  - Extended `ProxyOptions` with `TLSClientConfig *tls.Config`.
  - Added defensive cloning in `NewProxyWithOptions` (`tr.TLSClientConfig = opts.TLSClientConfig.Clone()`) to prevent shared-pointer `NextProtos` data races across concurrent transports.
  - Added `tr.CloseIdleConnections()` invocation inside `ReverseProxy.Close()`.
- **Sidecar Proxy Engine (`pkg/sidecar/proxy.go`)**:
  - Extended `ProxyEngine` with `proxies map[string]*proxy.ReverseProxy`.
  - Added `getOrCreateProxy` with thread-safe double-checked locking using `sync.RWMutex` and defensive client TLS cloning (`p.clientTLS.Clone()`).
  - Pre-compiled client TLS configuration in `NewProxyEngine` and stored in `p.clientTLS`.
  - Updated `proxyToURL` to route requests through cached origin proxies.
  - Updated `ProxyEngine.Stop()` to iterate through and close all cached proxies.

### Added
- **Canonical Origin Normalization (`normalizeTargetOrigin`)**: Added URL parser utility in `pkg/sidecar/proxy.go` ensuring canonical `scheme://host[:port]` origin caching.
- **Automated Verification Suite (`pkg/sidecar/sidecar_test.go`, `pkg/proxy/proxy_test.go`)**:
  - `TestProxyEngine_ProxyCaching_SingletonPerOrigin` (TC-099-01): Verifies single reverse proxy and $\le 2$ TCP sockets across 50 sequential requests.
  - `TestProxyEngine_MultiTarget_Isolation` (TC-099-02): Verifies distinct backends maintain isolated `ReverseProxy` instances.
  - `TestProxyEngine_HighConcurrency_RaceClean` (TC-099-03): High-concurrency test running 50 workers issuing 1,000 requests clean under `go test -race`.
  - `TestProxyEngine_Stop_CleansUpAllProxies` (TC-099-04): Verifies clean proxy closure and map teardown on `ProxyEngine.Stop()`.
  - `TestProxyEngine_ClientTLS_Propagation` (TC-099-05): Verifies client certificate and CA validation propagation through cached proxy.
  - `TestProxyEngine_OriginNormalization` (TC-099-06): Validates 9 target URL patterns for origin normalization.
  - `TestReverseProxy_Close_ClosesIdleConnections` (TC-099-07): Verifies `ReverseProxy.Close()` closes idle transport connections.

### Related Tasks & Requirements
- `TASK-122`: Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy
- `REQ-099`: Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy
- `ADR-099`: ReverseProxy Caching, Transport Teardown, and Client mTLS Plumbing in Sidecar Proxy
- `TC-099`: Verification of Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy
- `CR-095`: Code Review of Thread-Safe ReverseProxy Caching, Transport Teardown, and Client mTLS Lifecycle Management in Service Mesh Sidecar Proxy (SEC-37)
- `SR-099`: Security Review and Vulnerability Assessment of SEC-37 Remediation
- `SEC-37`: Unbounded HTTP Client & Transport Allocation per Request in Service Mesh Sidecar Proxy

## 2026-09-11 - Toron v1.5.17 Security Release (SEC-36: Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-36 (`pkg/server/internal_api.go`)**: Successfully resolved Medium-severity memory exhaustion and socket descriptor leak vulnerabilities `SEC-36` ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), `SR-091 Finding 6`, `SR-098`, `CR-094`) in the control plane `POST /internal/api/proxy-test` endpoint.
- **Bounded Inbound Request Body Ingestion (64 KB Cap)**: Replaced unbounded `io.ReadAll(req.Body)` with `io.LimitReader(req.Body, maxRequestBodyBytes+1)`. Payloads exceeding 64 KB (`65,536` bytes) are immediately rejected with `HTTP 400 Bad Request` (`{"error":"400 Bad Request","message":"Request body exceeds maximum allowed size of 64KB"}`), permanently preventing heap exhaustion via oversized client requests.
- **Bounded Upstream Response Buffering & Deterministic Clamping**: Eliminated unbounded `io.ReadAll(httpResp.Body)`. Upstream response bodies are ingested via `io.LimitReader(httpResp.Body, maxResponseBytes+1)`. If the upstream body exceeds `maxResponseBytes`, it is deterministically clamped to the exact limit, preserving HTTP status codes and headers while signaling `"truncated": true` in the output JSON.
- **Configurable Response Ceiling (`MaxProxyTestResponseBytes`) with Safe 1 MB Default**: Added `MaxProxyTestResponseBytes int64` to `InternalAPIConfig`. If omitted, set to `0`, or configured with a negative value, the engine safely falls back to a 1 MB (`1,048,576` bytes) default ceiling.
- **Keep-Alive Connection Pool Reuse & Stream Draining (CWE-775 Remediation)**: Remediated file descriptor leaks (`EMFILE`) by safely draining up to 64 KB of residual upstream body data into `io.Discard` (`io.Copy(io.Discard, io.LimitReader(httpResp.Body, 64*1024))`) before deferred socket closure (`httpResp.Body.Close()`). This allows the underlying HTTP/1.1 TCP connection to be returned to Go's transport keep-alive pool for reuse rather than hanging or leaking.
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`io`, `net/http`, `encoding/json`, `fmt`, `time`, `strings`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-098`)**: Validated via unit and integration test cases TC-098-01 through TC-098-08 in `pkg/server/internal_api_test.go`, covering sub-limit responses, oversized response truncation, infinite chunked stream termination within memory bounds, custom response limits, zero/negative fallback to 1 MB, inbound request body cap enforcement (boundary, overflow, empty, invalid JSON), keep-alive socket reuse, and concurrent race-free execution under `go test -race` (`TASK-121`, `TC-098`).

### Fixed
- **Unbounded Inbound Request Ingestion (`SEC-36`, CWE-400)**: Fixed vulnerability where incoming JSON request bodies on `POST /internal/api/proxy-test` were read into memory without length limits, allowing clients to trigger excessive heap allocations.
- **Memory Exhaustion via Unbounded Upstream Responses (`SEC-36`, CWE-400, CWE-770)**: Fixed vulnerability where probing an upstream target returning a multi-gigabyte or infinite stream (e.g. `/dev/urandom`, endless SSE) caused unbounded memory expansion until the OS Out-Of-Memory (OOM) killer aborted the Toron gateway.
- **Socket Descriptor Leaks & Connection Hangs (`SEC-36`, CWE-775)**: Fixed failure to drain residual bytes on truncated upstream responses, which previously prevented HTTP/1.1 transport connection recycling and caused `EMFILE` socket exhaustion.

### Changed
- **Internal API Proxy Test Probe (`pkg/server/internal_api.go`)**:
  - Implemented 64 KB limit reader on `req.Body` with `400 Bad Request` early rejection.
  - Implemented bounded reader on `httpResp.Body` constrained by `maxResponseBytes`.
  - Added deterministic clamping and residual stream discard draining.
- **Internal API Configuration (`pkg/server/internal_api.go`)**:
  - Extended `InternalAPIConfig` with `MaxProxyTestResponseBytes int64 `yaml:"max_proxy_test_response_bytes" json:"max_proxy_test_response_bytes"``.
- **Proxy Test Response Schema (`pkg/server/internal_api.go`)**:
  - Extended `ProxyTestResponse` with `Truncated bool `json:"truncated,omitempty"``.

### Added
- **Configuration Option (`max_proxy_test_response_bytes`)**: Added configurable byte ceiling for proxy test probe upstream response buffering.
- **Automated Verification Suite (`pkg/server/internal_api_test.go`)**:
  - `TestProxyTest_NormalResponse_UnderLimit` (TC-098-01): Verifies normal response under limit returns full body with `truncated: false`.
  - `TestProxyTest_OversizedResponse_Truncated` (TC-098-02): Verifies 5 MB response clamped to 1 MB default with `truncated: true`.
  - `TestProxyTest_InfiniteStream_BoundedTermination` (TC-098-03): Verifies infinite chunked stream is terminated cleanly within memory ceiling.
  - `TestProxyTest_CustomConfiguredLimit` (TC-098-04): Verifies custom limit (e.g. 2 KB) clamping.
  - `TestProxyTest_DefaultFallback_ZeroOrNegativeLimit` (TC-098-05): Verifies `<= 0` config falls back safely to 1 MB default.
  - `TestProxyTest_OversizedRequestBody_Rejection` (TC-098-06): Verifies 64 KB inbound cap enforcement, boundary handling, and rejection.
  - `TestProxyTest_SocketDrainAndConnectionReuse` (TC-098-07): Verifies TCP socket reuse after truncated response via stream draining.
  - `TestProxyTest_ConcurrentProbes_RaceClean` (TC-098-08): High-concurrency test running 20 parallel workers clean under `go test -race`.

### Related Tasks & Requirements
- `TASK-121`: Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe
- `REQ-098`: Bounded Inbound and Upstream Body Ingestion in Internal API Proxy Test Probe
- `ADR-098`: Bounded Request and Response Ingestion in Internal API Proxy Test Probe
- `TC-098`: Verification of Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe
- `CR-094`: Code Review of Bounded Request Ingestion and Upstream Response Buffering in Internal API Proxy Test Probe (SEC-36)
- `SR-098`: Security Review and Vulnerability Assessment of SEC-36 Remediation
- `SEC-36`: Memory Exhaustion via Unbounded Upstream Response Buffering in Internal API Proxy Test Probe

## 2026-09-11 - Toron v1.5.16 Security Release (SEC-35: Strict Sidecar Client TLS Certificate Validation and Explicit InsecureSkipVerify Opt-In)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-35 (`pkg/sidecar`, `pkg/config`)**: Successfully resolved High-severity improper certificate validation and silent Man-in-the-Middle (MitM) eavesdropping vulnerability `SEC-35` ([CWE-295](https://cwe.mitre.org/data/definitions/295.html), `SR-091 Finding 5`, `SR-097`, `CR-093`) in the Service Mesh Sidecar Proxy client TLS engine.
- **Elimination of Insecure Hardcoded Defaults (`pkg/sidecar/mtls.go`)**: Completely eliminated the historical architectural flaw in `BuildClientTLSConfig` where omitting an explicit internal CA certificate bundle (`ca_file: ""`) automatically set `tlsConfig.InsecureSkipVerify = true`. All outbound pod-to-pod egress connections now enforce strict peer certificate validation by default.
- **Seamless System Trust Root Fallback (`x509.SystemCertPool`)**: When `ca_file` is omitted or empty (`""`), `BuildClientTLSConfig` leaves `tlsConfig.RootCAs = nil`. The Go standard library `crypto/tls` runtime automatically falls back to validating peer certificates against the host operating system's system root certificate pool (`x509.SystemCertPool()`), enabling zero-configuration validation for public PKI, cloud certificates (AWS ACM, Cloudflare), and Let's Encrypt endpoints.
- **Custom Internal CA Bundle Support**: Retained full enterprise PKI support via `ca_file`. When specified, root certificates are loaded into a dedicated `x509.CertPool` assigned to `tlsConfig.RootCAs`, guaranteeing end-to-end zero-trust validation for internal service mesh certificate authorities.
- **Explicit `insecure_skip_verify` Opt-In & Mandatory Warning Log (`pkg/config/config.go`, `pkg/sidecar/mtls.go`)**: Added explicit opt-in boolean field `InsecureSkipVerify` to `SidecarConfig` (`insecure_skip_verify`), defaulting to `false`. When explicitly set to `true` for non-production environments, a high-visibility audit warning is emitted to the server log: `[SIDECAR] WARNING: InsecureSkipVerify is enabled for sidecar egress TLS. Certificate verification is disabled.`
- **Cryptographic Protocol Floor (`tls.VersionTLS12`)**: Strictly enforces `MinVersion: tls.VersionTLS12` across all client TLS configurations, completely preventing protocol downgrade attacks to SSLv3, TLS 1.0, or TLS 1.1.
- **Mutual TLS (mTLS) Client Identity Preservation**: Supports client certificate keypairs (`cert_file`, `key_file`) loaded via `tls.LoadX509KeyPair`, enabling client identity authentication during outbound egress handshakes.
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`crypto/tls`, `crypto/x509`, `fmt`, `log`, `os`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-097`)**: Fully validated via unit and end-to-end test cases TC-097-01 through TC-097-07, verifying secure default rejection of self-signed/expired/invalid certificates, successful system trust root and custom CA validation, explicit opt-in behavior with warning logs, mTLS client keypair presentation, TLS 1.2 minimum version enforcement, and high-concurrency race cleanliness under `go test -race` (`TASK-120`, `TC-097`).

### Fixed
- **Insecure Default `InsecureSkipVerify = true` in Sidecar Client TLS (`SEC-35`, CWE-295)**: Fixed severe flaw where omitting `ca_file` defaulted `InsecureSkipVerify = true`, completely disabling certificate verification and exposing outbound inter-service traffic to silent Man-in-the-Middle (MitM) eavesdropping, tampering, and credential theft on shared container networks.
- **Failure to Fall Back to Host System Trust Roots**: Fixed issue where omitting `ca_file` prevented the sidecar from utilizing host OS trust roots for public certificate validation without disabling security checks.

### Changed
- **Sidecar Client TLS Constructor (`pkg/sidecar/mtls.go`)**:
  - Eliminated the `else { tlsConfig.InsecureSkipVerify = true }` branch in `BuildClientTLSConfig`.
  - Assigned `tlsConfig.InsecureSkipVerify = cfg.InsecureSkipVerify`.
  - Configured `tlsConfig.RootCAs = nil` when `cfg.CAFile == ""` to enable automatic host OS trust pool fallback.
  - Added high-visibility warning logging when `cfg.InsecureSkipVerify == true`.
- **Sidecar Configuration Schema (`pkg/config/config.go`)**:
  - Extended `SidecarConfig` with `InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"``.
  - Initialized `InsecureSkipVerify: false` in `defaultConfig()` and `DefaultAppConfig()`.

### Added
- **Configuration Option (`insecure_skip_verify`)**: Added explicit opt-in boolean in `toron.yaml` (`sidecar.insecure_skip_verify`) for non-production debugging environments.
- **Automated Verification Suite (`pkg/sidecar/sidecar_test.go`)**:
  - `TestBuildClientTLSConfig_DefaultSecure` (TC-097-01): Verifies `InsecureSkipVerify == false` and `RootCAs == nil` by default.
  - `TestSidecar_EgressTLS_HandshakeRejection` (TC-097-02): Verifies immediate handshake failure and HTTP 502 rejection when encountering untrusted/self-signed upstream certificates under default settings.
  - `TestBuildClientTLSConfig_CustomCA` & `TestSidecar_EgressTLS_HandshakeSuccess_WithCustomCA` (TC-097-03): Verifies successful handshake and proxying when upstream certificate is signed by configured `ca_file`.
  - `TestBuildClientTLSConfig_ExplicitInsecureOptIn` & `TestSidecar_EgressTLS_HandshakeSuccess_WithExplicitOptIn` (TC-097-04): Verifies successful bypass and mandatory audit warning when `insecure_skip_verify: true`.
  - `TestBuildClientTLSConfig_ClientCertKeypair` (TC-097-05): Verifies loading of client mTLS certificate and keypair into `tlsConfig.Certificates`.
  - `TestBuildClientTLSConfig_MinVersionTLS12` (TC-097-06): Verifies strict `tls.VersionTLS12` minimum protocol enforcement.
  - `TestSidecar_EgressTLS_ConcurrentRouting_RaceClean` (TC-097-07): High-concurrency egress proxy routing test clean under `go test -race`.

### Related Tasks & Requirements
- `TASK-120`: Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS
- `REQ-097`: Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS Configuration
- `ADR-097`: Secure Default Client TLS Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Proxy
- `TC-097`: Verification of Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS Configuration
- `CR-093`: Code Review of Secure Default Certificate Validation and Explicit InsecureSkipVerify Opt-In in Sidecar Client TLS Configuration
- `SR-097`: Security Review and Vulnerability Assessment of SEC-35 Remediation
- `SEC-35`: Insecure Default InsecureSkipVerify in Sidecar Client TLS Configuration

## 2026-09-11 - Toron v1.5.16 Release (REQ-096 / TASK-119: Composite Route Key Grouping, Multi-Pod Target Aggregation, Canary Variant Isolation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller)

### Milestone Summary
- **Parity with OCI Discovery Engine (`pkg/ingress`, `pkg/router`)**: Established full architectural parity between Toron's Kubernetes Ingress Controller and the OCI Container Discovery engine (`REQ-095`, `ADR-095`, `CR-091`), eliminating Canary target pool contamination, route shadowing, and single-pod starvation in Kubernetes environments (`TASK-119`, `REQ-096`, `ADR-096`, `CR-092`, `SR-096`).
- **4-Dimensional Route Partitioning via `CompositeRouteKey` (`pkg/ingress/controller.go`)**: Partitioned Ingress routes using a 4-dimensional tuple `CompositeRouteKey = (Host, CleanPrefix, Method, CanonicalHeaders)`. Multiple Ingress definitions sharing the same domain and path prefix but differing in header criteria (e.g. Canary vs Baseline) or HTTP methods produce distinct composite keys, preventing target pool pollution and cross-variant traffic leakage.
- **Deterministic Alphabetical Header Canonicalization (`canonicalizeHeaders`)**: Implemented deterministic header sorting and lowercase normalization across all header keys (`a-env=staging&z-version=v2`), eliminating Go's non-deterministic `map[string]string` traversal and preventing false route churn across repeated 30-second reconciliation passes.
- **Strict Canary vs Baseline Target Segregation (Zero Traffic Bleed)**: Fully resolved the critical issue where Canary and Baseline pod endpoints were merged into a single load balancer pool. Canary Ingresses (`nginx.ingress.kubernetes.io/canary: "true"` or `toron.io/headers`) and Baseline Ingresses maintain separate reverse proxy pools. Traffic bearing canary headers routes exclusively to Canary pods, while standard traffic routes exclusively to Baseline pods.
- **Dual-Ecosystem Annotation Support (`pkg/ingress/translator.go`)**:
  - **Native Toron Annotations**: Supports `toron.io/method` (normalized uppercase HTTP verbs), `toron.io/header.<Name>` (individual header matches), and `toron.io/headers` (supporting both JSON object and CSV key=value formats) with additive merging and override precedence.
  - **Industry-Standard NGINX Canary Annotations**: Full compatibility with `nginx.ingress.kubernetes.io/canary: "true"`, `nginx.ingress.kubernetes.io/canary-by-header`, and `nginx.ingress.kubernetes.io/canary-by-header-value` (defaulting to `"always"` if omitted), enabling seamless zero-code migrations of existing Kubernetes Canary manifests.
- **ADR-005 Specificity-Based Route Ordering & Anti-Shadowing (`pkg/ingress/controller.go`)**: Enforced a strict 5-tier specificity hierarchy (Longest prefix $\to$ Specific host $\to$ Header constraint count $\to$ Method constraint $\to$ Deterministic tie-break) on all Ingress route specifications (`router.SortPrefixRouteSpecs(specs)`). Guaranteed that header-constrained Canary routes evaluate ahead of generic fallback Baseline routes, permanently eliminating route shadowing regardless of the discovery order returned by the Kubernetes API server.
- **Multi-Pod Target Aggregation & Fair Round-Robin Load Balancing**: Aggregated all pod IP endpoints for each composite key into a unified multi-target `PrefixRouteSpec` using `RoundRobinBalancer`, eliminating single-pod overload and distributing load evenly ($\approx 1/M$ per replica) across all active pod replicas.
- **Non-Destructive Scale-Down & Clean Eviction Teardown**: Pod scaling down updates the reverse proxy target pool in place with zero HTTP 404 errors or connection drops. Deleting an Ingress purges its routes and cleanly invokes `proxy.Close()`, stopping active health check tickers (`StopActiveHealthCheck`) and closing idle TCP connection pools, preventing socket descriptor leaks (`EMFILE`).
- **Declarative HTTP Method Filtering Support**: Populated `PrefixRouteSpec.Method` from `toron.io/method`, enforcing HTTP verb constraints at runtime in `router.ServeHTTP` and returning HTTP `405 Method Not Allowed` on method mismatches or falling through to method-compatible fallback routes.
- **Zero External Dependencies**: Pure Go standard library implementation (`sync`, `net/http`, `net/url`, `sort`, `strings`, `encoding/json`, `time`, `context`, `path`, `fmt`, `strconv`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-096`)**: Validated Ingress annotation parsing, deterministic composite key generation, multi-pod target aggregation, strict Canary/Baseline segregation, ADR-005 specificity sorting without Canary shadowing, declarative method filtering, non-destructive scale-down with zero downtime, and concurrent race-clean execution under `go test -race` (`TASK-119`, `TC-096`).

### Fixed
- **Canary Target Pool Pollution & Traffic Bleed (`REQ-096`, CWE-284)**: Fixed bug where Canary and Baseline pod endpoints were grouped using a naive 2-tuple `(Host, Prefix)`, causing Canary and Baseline pods to be lumped into a single reverse proxy target pool and sending unvalidated canary code to normal users ($\approx 33\text{--}50\%$).
- **Canary Route Shadowing via Non-Deterministic Arrival Order (`REQ-096`, CWE-284)**: Fixed issue where Ingress routes were evaluated in arbitrary Kubernetes API arrival order, allowing generic Baseline routes to intercept requests and shadow header-constrained Canary routes.
- **Missing Ingress Routing Dimension Support**: Fixed inability to specify HTTP methods and headers on Kubernetes Ingresses, bringing complete parity with OCI container discovery.
- **Single-Pod Starvation & Horizontal Scaling Collapse (`CWE-400`)**: Fixed single-pod monopolization by aggregating pod targets into round-robin load balancers.
- **Socket Descriptor & Health Checker Leaks (`CWE-775`)**: Fixed resource leaks where deleted Ingresses abandoned active health check tickers in heap memory.

### Changed
- **Ingress Route Translation (`pkg/ingress/translator.go`)**:
  - Implemented annotation extraction for `toron.io/method`, `toron.io/header.<Name>`, `toron.io/headers` (CSV and JSON), and `nginx.ingress.kubernetes.io/canary*`.
  - Populated `DiscoveredRoute.Method` and `DiscoveredRoute.Headers`.
- **Ingress Controller Dynamic Reconciliation (`pkg/ingress/controller.go`)**:
  - Introduced 4-dimensional `CompositeRouteKey` and deterministic `canonicalizeHeaders`.
  - Aggregated and deduplicated pod endpoints into multi-target `PrefixRouteSpec`s.
  - Added ADR-005 specificity sorting on compiled specs (`router.SortPrefixRouteSpecs(specs)`) prior to atomic replacement.

### Added
- **Supported Ingress Annotations**:
  - `toron.io/method`
  - `toron.io/header.<Name>`
  - `toron.io/headers`
  - `nginx.ingress.kubernetes.io/canary`
  - `nginx.ingress.kubernetes.io/canary-by-header`
  - `nginx.ingress.kubernetes.io/canary-by-header-value`
- **Automated Verification Suites (`scratch/tc_096_full_test_suite.go`, `pkg/ingress/ingress_test.go`)**:
  - `TestTranslateIngress_MethodAndHeaderAnnotations` (TC-096-01): 10 table-driven scenarios validating method, individual/grouped headers, NGINX canary defaults/custom values, override precedence, malformed JSON fallback, and backward compatibility.
  - `TestIngressController_CompositeRouteKeyAggregation` (TC-096-02): Deterministic canonicalization and key differentiation across 100 iterations.
  - `TestIngressController_CompositeRouteKeyAggregation` (TC-096-03): Multi-pod target aggregation, deduplication, and 33.3% round-robin distribution.
  - `TestIngressController_CanaryBaselineStrictSegregation` (TC-096-04): 100% Canary/Baseline isolation with zero traffic bleed.
  - `TestIngressController_SpecificityOrdering_NoCanaryShadowing` (TC-096-05): Adverse discovery order test confirming Canary route evaluation ahead of Baseline.
  - `TestIngressController_PrefixRouteSpec_MethodFiltering` (TC-096-06): Declarative method filtering and HTTP 405 Method Not Allowed handling.
  - `TestIngressController_PodScaleDown_ZeroDowntime` (TC-096-07): Zero-downtime scale-down (zero 404s) and clean Ingress deletion proxy teardown.
  - `TestIngressController_ConcurrentEventsAndRouting_RaceClean` (TC-096-08): High-concurrency test running 5 churn workers + 20 traffic workers clean under `go test -race`.

### Related Tasks & Requirements
- `TASK-119`: Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- `REQ-096`: Composite Route Key Grouping, Multi-Pod Target Aggregation, Canary Variant Isolation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- `ADR-096`: Composite Route Key Grouping, Multi-Pod Target Aggregation, Canary Variant Isolation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- `ADR-005`: Route Specificity and Path Matching Precedence
- `ADR-089`: Source-Tagged Atomic Prefix Routing & Multi-Pod Ingress Endpoint Aggregation
- `TC-096`: Verification of Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- `CR-092`: Code Review of Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller
- `SR-096`: Security Review of Composite Route Key Grouping, Multi-Pod Target Aggregation, and Specificity-Based Route Lifecycle in Kubernetes Ingress Controller

## 2026-09-11 - Toron v1.5.15 Security Release (SEC-34: Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-34 (`pkg/discovery`, `pkg/router`)**: Successfully resolved Medium-severity premature route deletion, denial-of-service outage on replica scale-down, single-replica traffic starvation, route shadowing, and routing dimension collision vulnerability `SEC-34` ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-284](https://cwe.mitre.org/data/definitions/284.html), [CWE-662](https://cwe.mitre.org/data/definitions/662.html), [CWE-775](https://cwe.mitre.org/data/definitions/775.html), `SR-091 Finding 4`, `SR-095`, `CR-091`) in the OCI Container Auto-Discovery Engine and Core Router.
- **4-Dimensional Route Partitioning via `CompositeRouteKey` (`pkg/discovery/manager.go`)**: Established multi-dimensional route partitioning based on a 4-tuple `CompositeRouteKey = (Host, CleanPrefix, Method, CanonicalHeaders)`. Containers sharing identical host and prefix but possessing different HTTP methods or header rules (such as Canary deployments with `X-Version: canary` vs baseline deployments, or `POST` vs `GET`) produce distinct composite keys, preventing variant collisions and cross-tenant traffic leakage (`TASK-118`, `REQ-095`, `ADR-095`).
- **Deterministic Alphabetical Header Canonicalization**: Solved Go's pseudo-random `map[string]string` iteration non-determinism by sorting header keys alphabetically (`sort.Strings(keys)`) and serializing them into a canonical query-string representation (`key1=val1&key2=val2`). Guarantees invariant composite key strings across reconciliation passes and eliminates false route churn.
- **Multi-Replica Target Aggregation & Fair Round-Robin Load Balancing (`pkg/discovery/manager.go`, `pkg/proxy/proxy.go`)**: Aggregated container replicas sharing an identical `CompositeRouteKey` into a unified multi-target `PrefixRouteSpec` using `RoundRobinBalancer`. Eliminated the prior single-pod overload defect (where linear first-match prefix search sent 100% load to replica #1) and distributes traffic evenly ($\approx 1/M$ per replica) across all active instances.
- **Non-Destructive Partial Scale-Down (Zero-Downtime Guarantee)**: Eliminated the critical bug where stopping 1 container replica triggered `RemovePrefixRoute` and purged all prefix routes for that service. The discovery engine now recalculates `desiredSpecs` across surviving replicas and performs atomic source-scoped replacement via `router.ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)`. Stopping 1 replica out of $M$ updates the target list to $M-1$ in place with **zero HTTP 404 errors**, zero connection drops, and zero transient downtime.
- **ADR-005 Specificity-Based Route Ordering & Anti-Shadowing (`pkg/router/router.go`)**: Enforced a strict 5-tier specificity hierarchy (Longest prefix $\to$ Specific host $\to$ Header constraint count $\to$ Method constraint $\to$ Deterministic tie-break). Unconstrained fallback routes can never shadow more specific canary or method-gated routes, regardless of container discovery arrival or registration order.
- **Declarative Method Matching Support in Prefix Routing**: Added first-class `Method string` support to `PrefixRouteSpec`, normalizing uppercase verbs and rejecting mismatched verbs with HTTP `405 Method Not Allowed` or falling through to method-compatible routes.
- **Clean Reverse Proxy & Health Check Teardown**: Evicted routes automatically invoke `pr.proxy.Close()` out of write lock, terminating active background health check ticker goroutines (`StopActiveHealthCheck`) and closing idle TCP connection sockets, eliminating socket descriptor (`EMFILE`) and goroutine leaks.
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`sync`, `net/http`, `net/url`, `sort`, `strings`, `encoding/json`, `time`, `context`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-095`)**: Validated model and label parsing, deterministic composite key generation, multi-replica target aggregation, non-destructive partial scale-down, distinct canary variant separation, ADR-005 specificity ordering without canary shadowing, declarative method matching, atomic eviction teardown, and high-concurrency race cleanliness under `go test -race` (`TASK-118`, `TC-095`).

### Fixed
- **Premature Route Deletion & Total Outage on Replica Stop (`SEC-34`, CWE-662, CWE-284)**: Fixed bug where stopping or restarting a single container replica called `RemovePrefixRoute`, which deleted ALL routes sharing `(host, prefix)`, inducing an immediate total outage (HTTP 404) for all surviving healthy replicas.
- **Horizontal Scaling Defeat & Single-Pod Overload (`SEC-34`, CWE-400)**: Fixed issue where containers were registered as independent single-target routes, causing router first-match prefix search to route 100% of requests to replica #1 and starve replicas #2..$M$.
- **Routing Dimension Blindness & Canary Collisions**: Fixed inability to route based on HTTP headers and methods in container discovery, preventing canary containers from being merged with baseline containers or overwriting baseline routes.
- **Route Shadowing via Arbitrary Insertion Order**: Fixed router evaluation order to enforce ADR-005 specificity ranking, preventing broad generic routes from intercepting requests destined for specific canary routes.
- **Reverse Proxy and Health Check Resource Leaks (`SEC-34`, CWE-775)**: Fixed resource leaks where evicted container routes left reverse proxy health checkers running in background goroutines.

### Changed
- **Discovered Route Model (`pkg/discovery/provider.go`)**: Extended `DiscoveredRoute` struct with `Method string` and `Headers map[string]string`. Updated `ActiveRoutes()` snapshot method to expose these fields for telemetry.
- **Label Parser (`pkg/discovery/parser.go`)**: Enhanced `ParseContainerLabels` to extract `toron.method`, individual `toron.header.<Name>`, and grouped `toron.headers` (CSV and JSON formats) with additive merging, override precedence, and graceful panic-free fallback.
- **Discovery Manager Reconciliation (`pkg/discovery/manager.go`)**:
  - Replaced incremental `RoutePrefix` / `RemovePrefixRoute` calls with atomic declarative reconciliation using `m.router.ReplacePrefixRoutesBySource("oci-discovery", desiredSpecs)`.
  - Partitioned active containers using `CompositeRouteKey` with deterministic header canonicalization.
  - Aggregated multi-replica target URLs into unified round-robin reverse proxy configurations.
  - Implemented lock-inversion-free synchronization (`m.mu` released prior to router invocation).
- **Router Prefix Routing Engine (`pkg/router/router.go`)**:
  - Extended `PrefixRouteSpec` with `Method string`.
  - Implemented 5-tier specificity sorting (`comparePrefixRoutes`, `comparePrefixRouteSpecs`, `SortPrefixRouteSpecs`).
  - Added out-of-lock reverse proxy teardown on evicted routes (`pr.proxy.Close()`).
  - Exposed `Method` in `RouteSnapshot` for administrative and telemetry parity.

### Added
- **Container Discovery Labels (`pkg/discovery/parser.go`)**:
  - Added `toron.method` label for HTTP method constraint routing.
  - Added `toron.header.<Name>` and `toron.headers` labels for HTTP header constraint routing.
  - Added `toron.health_check_interval` label for configurable health check probing frequency.
- **Router Sorting & Declarative API (`pkg/router/router.go`)**:
  - Added `SortPrefixRouteSpecs` public helper function.
  - Added `Method` field to `PrefixRouteSpec` and `RouteSnapshot`.
- **Automated Verification Suites (`pkg/discovery/discovery_test.go`, `pkg/router/router_test.go`)**:
  - `TestParseContainerLabels_MethodAndHeaders` (TC-095-01): Verifies parsing of method, header labels, CSV and JSON formats, overrides, and fallbacks.
  - `TestDiscoveryManager_CompositeRouteKeyCanonicalization` (TC-095-02): Verifies deterministic 4D composite route keys and map iteration independence.
  - `TestDiscoveryManager_CompositeRouteKeyAggregation` (TC-095-03): Verifies multi-replica target aggregation, deduplication, and round-robin balancing across replicas.
  - `TestDiscoveryManager_NonDestructivePartialScaleDown` (TC-095-04): Verifies zero-downtime partial scale-down with continuous traffic forwarding on remaining replicas.
  - `TestDiscoveryManager_DistinctVariantCanarySeparation` (TC-095-05): Verifies strict target isolation between baseline and canary deployments.
  - `TestRouter_SpecificityOrdering_NoCanaryShadowing` (TC-095-06): Verifies ADR-005 5-tier specificity hierarchy and anti-shadowing proof.
  - `TestRouter_PrefixRouteSpec_MethodMatching` (TC-095-07): Verifies declarative method matching and 405 / fallback routing.
  - `TestDiscoveryManager_AtomicReplacementLifecycle` (TC-095-08): Verifies atomic route eviction and clean reverse proxy teardown.
  - `TestDiscoveryManager_ConcurrentLifecycleAndRouting_RaceClean` (TC-095-09): High-concurrency test running background container churn against parallel client traffic, clean under `go test -race`.

### Related Tasks & Requirements
- `TASK-118`: Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle
- `REQ-095`: Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- `ADR-095`: Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- `ADR-005`: Route Specificity and Path Matching Precedence
- `TC-095`: Verification of Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- `CR-091`: Code Review of Composite Route Key Grouping, Multi-Replica Target Aggregation, and Specificity-Based Route Lifecycle in OCI Discovery Engine
- `SR-095`: Security Review and Vulnerability Assessment of SEC-34 Remediation
- `SEC-34`: Premature Route Deletion & Load-Balancing Failure Across Multi-Replica Containers in OCI Discovery Engine

## 2026-09-10 - Toron v1.5.14 Security Release (SEC-33: Bounded Route Table Lifecycle, Atomic Route Replacement, and Multi-Target Pod Aggregation in Kubernetes Ingress Controller)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-33 (`pkg/ingress`, `pkg/router`)**: Successfully resolved High-severity unbounded route table growth, zombie route persistence, stale endpoint shadowing, and pod replica starvation vulnerability `SEC-33` ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-670](https://cwe.mitre.org/data/definitions/670.html), [CWE-1059](https://cwe.mitre.org/data/definitions/1059.html), `SR-091 Finding 3`, `SR-094`, `CR-090`) in the Kubernetes Ingress Controller and Core Router engine.
- **Source-Tagged Prefix Routing & Atomic Route Table Replacement (`pkg/router/router.go`)**: Extended prefix routing to support subsystem origin tagging (e.g. `"k8s-ingress"`, `"config"`, `"static"`). Introduced declarative route specifications (`PrefixRouteSpec`) and an atomic route replacement API (`ReplacePrefixRoutesBySource`). Routes are pre-compiled and validated out of lock with fail-fast rollbacks, and atomically swapped under write lock, guaranteeing all-or-nothing cutovers without route churn or request disruption (`TASK-116`, `REQ-094`, `ADR-089`).
- **Bounded Memory Invariant ($O(K)$ Memory Scaling)**: Eliminated monotonic append-only route table growth across periodic resync cycles (every 30s) and watch event notifications. For $K$ active Ingress rules, the prefix route count for source `"k8s-ingress"` strictly equals $K$ across arbitrary $N$ synchronization iterations ($O(1)$ memory scaling with resync count), permanently preventing memory exhaustion, GC stalls, and Out-of-Memory (OOM) gateway terminations (`TASK-117`, `REQ-094`).
- **Automatic Zombie Route Elimination & Immediate Deletion Pruning**: Replaced manual route tracking with dynamic reconciliation. When an Ingress or path is deleted in Kubernetes, subsequent reconciliation cycles omit the deleted route from the desired batch, causing `ReplacePrefixRoutesBySource` to immediately evict the route from the routing table. Subsequent HTTP requests return HTTP `404 Not Found`, eliminating traffic leakage to decommissioned backends.
- **Multi-Pod Endpoint Aggregation & Fair Round-Robin Load Balancing (`pkg/ingress/translator.go`, `pkg/ingress/controller.go`)**: Aggregated multiple pod endpoint IPs sharing `(Host, Prefix)` into a unified multi-target reverse proxy route using `RoundRobinBalancer`. Eliminated single-pod replica starvation (where replica #1 previously received 100% load) and distributed traffic evenly ($\approx 1/M$ per replica) across all active pods.
- **Immediate Endpoint Cutover Without Stale Route Shadowing**: Rolling updates, pod restarts, and scale events immediately swap endpoint target lists in place. Stale routes are evicted rather than appended to the end of the routing table, guaranteeing immediate cutover with zero traffic sent to terminated pod IPs.
- **Clean Background Resource Teardown**: Replaced and evicted prefix routes cleanly invoke `.Close()` on associated `ReverseProxy` instances, terminating active background health check ticker goroutines (`StopActiveHealthCheck`) and releasing idle connection pools, preventing socket descriptor exhaustion (EMFILE).
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`sync`, `net/http`, `net/url`, `time`, `context`, `strings`, `path`, `path/filepath`, `fmt`, `log`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-094`)**: Validated unit source isolation, atomic empty-slice pruning, rollback on invalid specs, controller sync bounds across 50 cycles, zombie 404 pruning, 3-pod round-robin balancing, endpoint cutover without shadowing, health check teardown, and high-concurrency race cleanliness under `go test -race` (`TASK-116`, `TASK-117`, `TC-094`).

### Fixed
- **Unbounded Route Table Memory Leak (`SEC-33`, CWE-400)**: Fixed append-only route registration where every 30-second periodic resync and watch event appended duplicate prefix routes to `r.prefixRoutes`, causing monotonic memory growth of ~144,000 duplicate entries/day per 50 routes and eventual OOM crash.
- **Zombie Route Persistence (`SEC-33`, CWE-670)**: Fixed issue where deleted Ingress resources remained in Toron's routing table indefinitely, proxying client traffic to decommissioned or reassigned backend pods.
- **Stale Endpoint Shadowing**: Fixed routing anomalies during pod restarts and rollouts where updated endpoints were appended to the end of `r.prefixRoutes` and permanently shadowed by obsolete routes at the head of the linear matching table.
- **Horizontal Pod Replica Starvation**: Fixed single-target route generation in Ingress translation where multiple pod endpoints created separate routes, causing Toron's prefix matcher to route 100% of traffic to the first pod and starve all other replicas.
- **Background Goroutine and Socket Descriptor Leaks**: Fixed resource leakage where discarded reverse proxy instances left background health check tickers and connection pools running.

### Changed
- **Router Subsystem (`pkg/router/router.go`)**:
  - Extended internal `prefixRoute` struct with `source string` and `proxy *proxy.ReverseProxy`.
  - Added public declarative specification struct `PrefixRouteSpec`.
  - Implemented `RoutePrefixWithSource(source, ...)` and delegated legacy `RoutePrefix(...)` to `RoutePrefixWithSource("config", ...)` for 100% backward compatibility.
  - Implemented atomic batch replacement `ReplacePrefixRoutesBySource(source string, specs []PrefixRouteSpec) error` with pre-compilation out of lock, atomic pointer swap under write lock, and automatic resource teardown on evicted routes.
  - Added lifecycle teardown calling `.Close()` on evicted routes in `ReplacePrefixRoutesBySource`, `RemovePrefixRoute`, and `Reset`.
  - Exposed `Source` field in `RouteSnapshot` and `GetPrefixRoutes()` for telemetry and auditing.
- **Ingress Controller Reconciliation (`pkg/ingress/controller.go`)**:
  - Refactored `syncIngresses` to aggregate endpoints by unique routing tuple `(Host, CleanPrefix)` into `router.PrefixRouteSpec` with round-robin load balancing across all healthy pod targets.
  - Synchronized routes dynamically via `c.router.ReplacePrefixRoutesBySource("k8s-ingress", specs)` on every resync and watch event.
  - Serialized reconciliation cycles using `c.syncMu` to eliminate interleaving races between resync tickers and watch event streams.
- **Ingress Route Translation (`pkg/ingress/translator.go`)**:
  - Updated endpoint resolution to aggregate all pod IP addresses per subset into unified multi-target configurations, falling back to cluster Service DNS when endpoints are unavailable.

### Added
- **Router Declarative API & Telemetry (`pkg/router/router.go`)**:
  - Added `PrefixRouteSpec` struct for declarative prefix route provisioning.
  - Added `ReplacePrefixRoutesBySource` and `RoutePrefixWithSource` methods to `Router`.
  - Added `Source` field to `RouteSnapshot` for telemetry, auditing, and observability.
- **Automated Verification Suites (`pkg/router/router_test.go`, `pkg/ingress/ingress_test.go`)**:
  - `TestRouter_ReplacePrefixRoutesBySource_SourceIsolation` (TC-094-01): Verifies atomic replacement alters only matching source, leaves other sources intact, and prunes on empty slice.
  - `TestRouter_ReplacePrefixRoutesBySource_PreCompilationValidation` (TC-094-02): Verifies all-or-nothing rollback on invalid route specs without modifying active route state.
  - `TestRouter_RoutePrefix_BackwardCompatibility` (TC-094-03): Verifies legacy callers are tagged as `"config"` and retain 100% functional equivalence.
  - `TestRouter_ReplacePrefixRoutes_StopsActiveHealthCheck` (TC-094-04): Verifies active background health checks on evicted routes are terminated cleanly.
  - `TestRouter_RemovePrefixRoute_StopsActiveHealthCheck` & `TestRouter_Reset_StopsActiveHealthCheck`: Verifies teardown on individual route deletion and router reset.
  - `TestIngressController_BoundedRouteTable` (TC-094-05): Verifies route count remains strictly equal to $K$ across 50 consecutive sync cycles ($O(1)$ memory scaling with sync count).
  - `TestIngressController_ZombieRoutePruning` (TC-094-06): Verifies deleted Ingresses are immediately pruned and return HTTP 404 Not Found.
  - `TestIngressController_MultiPodLoadBalancing` (TC-094-07): Verifies requests to 3 pod replicas distribute evenly across all replicas ($\approx 33.3\%$ per pod).
  - `TestIngressController_EndpointUpdateNoShadowing` (TC-094-08): Verifies immediate cutover to new pod IPs without stale route shadowing.
  - `TestIngressController_ConcurrentSyncAndRouting_RaceClean` (TC-094-09): High-concurrency test running background churn against parallel client traffic, clean under `go test -race`.

### Related Tasks & Requirements
- `TASK-116`: Router Source-Tagged Prefix Routing and Atomic Route Replacement API
- `TASK-117`: Kubernetes Ingress Controller Route Table Dynamic Reconciliation and Multi-Target Pod Aggregation
- `REQ-094`: Bounded Route Table Lifecycle, Atomic Source Replacement, and Multi-Target Pod Aggregation in Kubernetes Ingress Controller
- `ADR-089`: Source-Tagged Atomic Prefix Routing & Multi-Pod Ingress Endpoint Aggregation
- `TC-094`: Verification of Kubernetes Ingress Route Table Lifecycle, Atomic Source Replacement, and Multi-Pod Balancing
- `CR-090`: Code Review of Bounded Route Table Lifecycle, Atomic Source Replacement, and Multi-Target Pod Aggregation in Kubernetes Ingress Controller
- `SR-094`: Security Review and Vulnerability Assessment of SEC-33 Remediation
- `SEC-33`: Unbounded Routing Table Memory Leak & Zombie Route Persistence in Kubernetes Ingress Controller

## 2026-09-10 - Toron v1.5.13 Security Release (SEC-32: Fail-Closed WAF IP Access Control on Unidentifiable Client IP)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-32 (`pkg/waf/middleware.go`, `pkg/waf/ip_acl.go`)**: Successfully resolved fail-open WAF IP access control bypass vulnerability `SEC-32` ([CWE-284](https://cwe.mitre.org/data/definitions/284.html), [CWE-1188](https://cwe.mitre.org/data/definitions/1188.html), [CWE-693](https://cwe.mitre.org/data/definitions/693.html), `SR-091 Finding 2`, `SR-093`, `CR-089`) in the WAF middleware and IP ACL evaluation engine.
- **Fail-Closed Allowlist Perimeter Enforcement (`pkg/waf/middleware.go`, `pkg/waf/ip_acl.go`)**: Enforced strict fail-closed access control when incoming client IP cannot be determined under an active IP allowlist (`allowed_ips` or `allowedSubnets`). Requests with missing, stripped, untrusted, or malformed IP headers are immediately rejected with HTTP `403 Forbidden` and exact JSON payload `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}` (`TASK-114`, `REQ-093`, `ADR-088`).
- **Fail-Open Pass-Through for Denylist-Only Mode (`pkg/waf/ip_acl.go`)**: Preserved negative security model semantics when only `denied_ips` is configured without an active allowlist. Requests with unidentifiable client IP (`nil`) evaluate to `allowed: true, reason: ""` in `CheckIP` and pass through the IP ACL stage to subsequent WAF inspection layers, eliminating false-positive outages across internal service meshes, synthetic health probes, or intermediate proxies.
- **Single-Pass Hot-Path IP Extraction Optimization (`pkg/waf/middleware.go`)**: Eliminated redundant sequential invocations of `ExtractClientIP` on the request hot path. Middleware extracts client IP once at entry, caching `clientNetIP net.IP` on the stack for unconditional `acl.CheckIP(clientNetIP)` evaluation and string `clientIP` across all telemetry and audit events (`TASK-114`, `ADR-088`).
- **Unconditional IP ACL Delegation**: Removed the vulnerable `if ip != nil` guard in WAF middleware, delegating policy authority unconditionally to `IPAccessList.CheckIP` whenever `acl != nil && acl.HasRules()` evaluates to `true`.
- **Safe Telemetry & Structured Audit Logging**: Guaranteed zero-panic emission of `ip_acl_block` `SecurityEvent` records with `ClientIP: ""` (empty string) and metric increment `toron_waf_blocked_requests_total{category="ip_acl"}` on fail-closed rejections.
- **Zero External Dependencies**: Implemented entirely with Go standard library packages (`net`, `net/http`, `strings`, `bytes`, `sync`), keeping `go.mod` and `go.sum` with 0 diffs.
- **Comprehensive Automated Verification Suite (`TC-093`)**: Validated unit ACL checks, end-to-end middleware fail-closed rejection, denylist pass-through, malformed header matrix rejection, panic-free audit logging, telemetry parity, and high-concurrency race cleanliness under `go test -race` (`TASK-115`, `TC-093`).

### Fixed
- **Fail-Open Allowlist Bypass on Missing Client IP (`SEC-32`, `REQ-093`)**: Fixed critical vulnerability where requests lacking resolvable client IP (`ExtractClientIP` returned `nil`) bypassed allowlist checks due to `if ip != nil` guard in `middleware.go` and `CheckIP(nil)` returning `true, ""` (fail-open) in `ip_acl.go`.
- **Hot-Path Redundant IP Extraction**: Fixed duplicate invocations of `ExtractClientIP(req, tp)` in WAF middleware closure, eliminating duplicate socket splitting and CIDR matching overhead on high-throughput routes.
- **Malformed Header Exploitation**: Fixed handling of corrupt, non-IP, or unparseable headers (`RemoteAddr`, `X-Forwarded-For`, `X-Real-IP`), ensuring they evaluate safely to `nil` and trigger fail-closed 403 Forbidden responses under active allowlists.

### Changed
- **WAF IP ACL Engine (`pkg/waf/ip_acl.go`)**: Refactored `CheckIP` to inspect active rules when `ip == nil`. If `len(acl.allowedSubnets) > 0 || len(acl.allowedIPs) > 0`, it returns `allowed: false, reason: "client IP could not be determined and allowed IP list is enforced"`; if only denylists or no rules are configured, it returns `allowed: true, reason: ""` (fail-open).
- **WAF Middleware Pipeline (`pkg/waf/middleware.go`)**:
  - Replaced duplicate `ExtractClientIP` calls with single-pass extraction at closure entry (`clientNetIP := ExtractClientIP(req, tp)`).
  - Removed `if ip != nil` evaluation guard; `acl.CheckIP(clientNetIP)` is invoked unconditionally whenever `acl != nil && acl.HasRules()`.
  - On denial (`!allowed`), immediately halts processing without invoking `next(req, res)`, sets status to `403 Forbidden`, sets `Content-Type: application/json`, writes exact JSON error payload `{"error":"Forbidden","message":"..."}`, increments Prometheus metric `WAFBlocked("ip_acl", req.Path)`, and emits structured `SecurityEvent`.

### Added
- **Automated Verification Suites (`pkg/waf/ip_acl_test.go`, `pkg/waf/middleware_test.go`)**:
  - `TestIPAccessList_CheckIP_NilIP_WithAllowlist` (TC-093-01): Unit test verifying `CheckIP(nil)` returns `allowed == false` and exact reason across IPv4 CIDRs, exact IPv4, IPv6 CIDRs, exact IPv6, and combined allow/deny sets.
  - `TestIPAccessList_CheckIP_NilIP_DenylistOnly` & `TestIPAccessList_CheckIP_NilIP_NoRules` (TC-093-02): Unit tests verifying fail-open pass-through for `CheckIP(nil)` when only denylists are present, empty ACLs, or nil receiver pointer.
  - `TestWAFMiddleware_NilClientIP_AllowlistEnforced` (TC-093-03): End-to-end middleware test verifying HTTP 403 Forbidden, exact JSON response body `{"error":"Forbidden","message":"client IP could not be determined and allowed IP list is enforced"}`, immediate execution halt, and Prometheus metric increment on missing client IP.
  - `TestWAFMiddleware_NilClientIP_DenylistOnly` (TC-093-04): End-to-end middleware test verifying pass-through to downstream handler (HTTP 200) for unidentifiable client IP under denylist-only mode.
  - `TestWAFMiddleware_MalformedClientIP_AllowlistEnforced` (TC-093-05): Table-driven test evaluating 9 malformed address variants (`"not-an-ip:9999"`, `":::invalid"`, `"hostname-without-ip"`, `"unknown"`, `"localhost"`, `"999.999.999.999"`, `"garbage-header"`, `"invalid-real-ip"`, `"bad-ipv6"`), verifying all evaluate safely to `nil` and fail closed with HTTP 403.
  - `TestWAFMiddleware_AuditLogging_NilIP_Blocked` (TC-093-06): Verification of structured audit event emission (`ip_acl_block`, `ClientIP: ""`) on nil IP block with zero panics or nil-pointer dereferences.
  - `TestWAFMiddleware_SinglePassExtraction_TelemetryParity` (TC-093-07): Verifies single-pass extraction parity across allowed IP (`200 OK`), denied IP (`403 Forbidden`), and nil IP (`403 Forbidden`).
  - `TestWAFMiddleware_NilClientIP_Concurrency` & `TestWAFMiddleware_ConcurrentRaceClean` (TC-093-08): 100 concurrent workers dispatching 5,000 requests across mixed IP scenarios under `-race`, verifying zero data races and zero deadlocks.

### Related Tasks & Requirements
- [`TASK-114`: WAF Middleware Single-Pass IP Extraction and Fail-Closed Enforcement
- `TASK-115`: End-to-End WAF Middleware IP ACL Automated Verification Suite (TC-093)
- `REQ-093`: Fail-Closed WAF IP Access Control Enforcement on Unidentifiable Client IP
- `ADR-088`: Fail-Closed WAF IP Access Control & Single-Pass Extraction
- `TC-093`: Test Suite for Fail-Closed WAF IP Access Control on Unidentifiable Client IP
- `CR-089`: Code Review of Fail-Closed WAF IP Access Control Enforcement on Unidentifiable Client IP
- `SR-093`: Security Review and Vulnerability Assessment of SEC-32 Remediation
- `SEC-32`: Fail-Open WAF IP Access Control Bypass on Unidentifiable Client IP

## 2026-09-10 - Toron v1.5.12 Security Release (SEC-31: Physical RemoteAddr Binding & Ingress Anti-Spoofing across HTTP/1.1, HTTP/2, and HTTP/3)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-31 (`pkg/httpparser`, `pkg/server`, `pkg/waf`, `pkg/router`, `pkg/proxy`, `pkg/logging`)**: Successfully resolved unauthenticated client IP spoofing and perimeter security bypass vulnerability `SEC-31` ([CWE-290](https://cwe.mitre.org/data/definitions/290.html), [CWE-345](https://cwe.mitre.org/data/definitions/345.html), [CWE-693](https://cwe.mitre.org/data/definitions/693.html), `SR-091 Finding 1`, `SR-092`).
- **Physical `RemoteAddr` Binding in HTTP/2 and HTTP/3 Protocol Adapters**: Extended `httpparser.Request` with an immutable `RemoteAddr string` field and panic-safe extraction helpers `RemoteHost()` and `RemoteIP()`. HTTP/2 streams (`http2AdapterHandler`) and HTTP/3 QUIC datagrams (`ListenAndServeH3`) bind `r.RemoteAddr` at ingress, and native HTTP/1.1 connections bind `conn.RemoteAddr().String()` in `handleConn`, establishing full protocol parity across all transports (`TASK-111`, `REQ-092`, `ADR-087`).
- **Perimeter Hardening & Trusted Proxy Gating (`pkg/waf`, `pkg/server`, `pkg/router`, `pkg/proxy`, `pkg/logging`)**: Client-supplied `X-Forwarded-For` and `X-Real-IP` headers are rejected and stripped unless the physical peer IP is verified against configured `trusted_proxies` CIDR blocks:
  - **WAF IP ACL**: Evaluates physical peer IP first; untrusted headers cannot bypass blacklists or allowlists; fails secure on unresolvable IPs.
  - **Internal Management API**: Rejects untrusted HTTP/2 and HTTP/3 subnet spoofing with `403 Forbidden` (`{"error":"403 Forbidden","message":"Access denied by administrative subnet policy"}`).
  - **Token Bucket Rate Limiting**: Neutralized header rotation DoS attacks by removing insecure `req.RawConn == nil` fallbacks; untrusted connections are contained within a single `ip:<remoteHost>` bucket.
  - **Reverse Proxy**: Strips spoofed `X-Forwarded-For` and `X-Real-IP` headers from untrusted connections, replacing them with verified physical `peerIP`; safely appends `peerIP` for verified `trusted_proxies`.
  - **Structured Access Logging**: Records true physical client IP, preventing audit trail falsification.
- **Preserved Mobile Roaming Session Affinity (`pkg/proxy/sticky.go`)**: Maintained zero changes (0 diffs) in `sticky.go`, preserving application-level session persistence across cellular IP handovers and carrier CGNAT transitions pursuant to `REQ-030`.
- **Zero External Dependencies**: Implemented strictly using standard library packages (`net`, `net/http`, `strings`, `sync`).
- **Comprehensive Automated Verification Suite (`TC-092`)**: Validated physical address binding, multi-protocol parity, WAF anti-spoofing, internal API subnet gates, rate limiter single-bucket containment, reverse proxy sanitization, HTTP/3 QUIC datagram parity, mobile roaming affinity, and concurrency under `go test -race` (`TASK-113`, `TC-092`).

### Added
- **Request Model Fields & Helpers (`pkg/httpparser/request.go`)**:
  - Added `RemoteAddr string` to `httpparser.Request`.
  - Added `RemoteHost() string` with safe port splitting and `RawConn` fallback.
  - Added `RemoteIP() net.IP` supporting IPv4, bracketed/unbracketed IPv6, and nil safety.
- **Automated Verification Suite (`pkg/server/server_anti_spoofing_test.go`, `pkg/httpparser/request_test.go`, `pkg/waf/ip_acl_test.go`, `pkg/router/rate_limiter_test.go`, `pkg/proxy/proxy_test.go`, `pkg/proxy/sticky_test.go`, `pkg/logging/manager_test.go`)**:
  - `TestRequest_RemoteHostAndIP_Parsing`: Table-driven tests for IPv4, IPv6, port splitting, bracket stripping, whitespace, and fallbacks.
  - `TestRequest_NewRequestFromStd_RemoteAddr`: Standard request conversion remote address preservation.
  - `TestServer_Ingress_RemoteAddrPopulation_H1` & `TestServer_Ingress_RemoteAddrPopulation_H2`: Transport ingress remote address binding.
  - `TestServer_H2_WAF_SpoofingRejected` & `TestWAF_IPACL_AntiSpoofing`: WAF blacklist/allowlist anti-spoofing rejection and fail-secure verification.
  - `TestServer_H2_InternalAPI_SubnetEnforcement`: Internal API administrative subnet spoofing rejection and fail-closed default.
  - `TestServer_H2_RateLimiter_AntiSpoofing` & `TestServer_H2_RateLimiter_Integration`: Rate limiter header rotation containment.
  - `TestServer_H2_ReverseProxy_HeaderSanitization` & `TestServer_H2_ReverseProxy_TrustedProxyAppended`: Reverse proxy header stripping and trusted proxy appending.
  - `TestServer_H3_RemoteAddrBinding`: HTTP/3 QUIC datagram peer address binding.
  - `TestLogging_ExtractClientIP_PhysicalBinding`: Structured logging client IP physical binding.
  - `TestSticky_PreserveMobileRoamingAffinity`: Sticky session mobile roaming regression test (REQ-030).
  - `TestServer_AntiSpoofing_ConcurrentRaceClean`: 100 concurrent workers dispatching 5,000 requests under `-race`.

### Changed
- **Ingress Protocol Adapters (`pkg/server/server.go`, `pkg/httpparser/request.go`)**: Bound physical socket addresses in `handleConn`, `http2AdapterHandler`, and `ListenAndServeH3`.
- **Perimeter Security Modules (`pkg/waf`, `pkg/server`, `pkg/router`, `pkg/proxy`, `pkg/logging`)**: Prioritized physical connection metadata and enforced `trusted_proxies` verification before accepting client-supplied IP headers.

### Related Tasks & Requirements
- `TASK-111`: RemoteAddr Binding & Extraction Helpers in httpparser.Request and Ingress Protocol Adapters
- `TASK-112`: Security Perimeter Hardening & Trusted Proxy Gating across WAF, Internal API, Rate Limiter, Reverse Proxy, and Logging
- `TASK-113`: Comprehensive Automated Verification Suite for Anti-Spoofing & Protocol Parity (TC-092)
- `REQ-092`: Unauthenticated Client IP Spoofing and Security Bypass via Missing Physical RemoteAddr Binding in HTTP/2 and HTTP/3 Adapters
- `ADR-087`: Physical RemoteAddr Binding and Ingress Anti-Spoofing across HTTP/1.1, HTTP/2, and HTTP/3
- `TC-092`: Test Suite for Physical RemoteAddr Binding and Ingress Anti-Spoofing
- `SEC-31`: Unauthenticated Client IP Spoofing & Security Bypass via Missing Physical RemoteAddr Binding in HTTP/2 and HTTP/3 Adapters
- `SR-092`: Security Review and Vulnerability Assessment of SEC-31 Remediation
- `CR-088`: Code Review of Physical RemoteAddr Binding & Ingress Anti-Spoofing across HTTP/1.1, HTTP/2, and HTTP/3

## 2026-09-09 - Toron v1.5.11 Security Release (SEC-30: Direct Parameterized Subpath Routing and Empty Prefix Proxy Elimination in REST-to-gRPC Transcoder)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-30 (`pkg/transcoder`, `pkg/router`)**: Resolved Subpath Routing Interception and Denial of Service vulnerability `SEC-30` ([CWE-284](https://cwe.mitre.org/data/definitions/284.html), [CWE-400](https://cwe.mitre.org/data/definitions/400.html), `SR-081 Finding 8`, `SR-090`) in the REST-to-gRPC Transcoding Engine and Edge Router.
- **Router Method-Aware Prefix Routing & Path Matcher API (`pkg/router/router.go`)**: Extended `Router` with `HandlePrefix` and `HandlePrefixWithMatcher`, allowing in-process Go handlers to be registered directly on path prefixes with HTTP method gating and custom path matchers (`TASK-108`).
- **Elimination of Empty Upstream Reverse Proxies (`pkg/transcoder/transcoder.go`)**: Completely removed dummy upstream reverse proxy registration with 0 targets (`RoutePrefix("upstream", ...)`) that previously caused all parameterized REST requests to abort with `502 Bad Gateway: No upstream target available` (`TASK-109`).
- **Direct Parameterized Dispatch & Pattern Matching (`pkg/transcoder/transcoder.go`)**: Implemented `MatchPathPattern` to validate literal segments and wildcard parameter tokens. Parameterized routes (`/v1/users/:id`, `/v1/users/:id/orders/:orderId`) dispatch directly through `Router.ServeHTTP` to `HandleTranscode` with `200 OK` responses (`TASK-109`).
- **Multi-Level Route Segregation & Method Gating**: Multiple routes sharing common path prefixes are cleanly segregated without route shadowing; invalid methods return `405 Method Not Allowed`, and segment count mismatches return `404 Not Found` without upstream leakage (`TASK-108`, `TASK-109`).
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`strings`, `net/http`, `sync`, `net/url`).
- **Automated Verification Suite (`pkg/transcoder/transcoder_test.go`, `pkg/router/router_test.go`)**: Validated direct parameterized subpath dispatch through `router.ServeHTTP` (fulfilling Missing Security Test 5 in `SR-081`), multi-level route segregation, 405 method mismatch, 404 segment mismatch, and concurrency under `go test -race` (`TASK-110`, `TC-091`).

### Added
- **Router APIs (`pkg/router/router.go`)**: `HandlePrefix(method, prefix string, handler HandlerFunc)` and `HandlePrefixWithMatcher(method, host, prefix string, headers map[string]string, matcher func(path string) bool, handler HandlerFunc)`.
- **Transcoder Helper (`pkg/transcoder/transcoder.go`)**: `MatchPathPattern(pattern, path string) bool`.
- **Automated Tests (`pkg/router/router_test.go`, `pkg/transcoder/transcoder_test.go`)**:
  - `TestRouter_HandlePrefix_MethodAndMatcher`: Router prefix handler and matcher dispatching.
  - `TestTranscoder_ParameterizedSubpathDispatch` (TC-091-01): Verifies parameterized `GET /v1/users/usr-777` dispatches through `router.ServeHTTP` returning 200 OK.
  - `TestTranscoder_MultiLevelRouteSegregation` (TC-091-02): Verifies shared prefix route segregation.
  - `TestTranscoder_WrongMethodOnParameterizedRoute` (TC-091-03): Verifies 405 Method Not Allowed.
  - `TestTranscoder_SegmentCountMismatch_NotFound` (TC-091-04): Verifies 404 Not Found.
  - `TestTranscoder_MatchPathPattern` (TC-091-05): Unit test coverage for pattern matcher.
  - `TestTranscoder_ParameterizedSubpath_ConcurrencyRaceSafety` (TC-091-06): 50 concurrent requests under `-race`.

### Changed
- **Transcoder Route Registration (`pkg/transcoder/transcoder.go`)**: Replaced dummy upstream proxy with direct `HandlePrefixWithMatcher` binding for parameterized routes and `Handle` for exact routes.

### Related Tasks & Requirements
- `TASK-108`: Router Method-Aware Prefix Routing & Path Matcher Support
- `TASK-109`: Direct Parameterized Subpath Routing & Empty Upstream Proxy Elimination in Transcoder
- `TASK-110`: Automated Verification Suite for Parameterized Transcoder Subpath Dispatch
- `REQ-091`: Direct Parameterized Subpath Routing and Elimination of Empty Prefix Proxy in REST-to-gRPC Transcoder
- `ADR-086`: Direct Parameterized Subpath Routing and Elimination of Empty Prefix Proxy in REST-to-gRPC Transcoder
- `TC-091`: Test Suite for Parameterized Subpath Routing and Empty Prefix Proxy Elimination
- `SEC-30`: Subpath Routing Interception & 502 Denial in REST-to-gRPC Transcoder
- `SR-090`: Security Review of SEC-30 Remediation
- `CR-087`: Code Review of Direct Parameterized Subpath Routing and Empty Prefix Proxy Elimination

## 2026-09-09 - Toron v1.5.10 Security Release (SEC-29: Hop-by-Hop Header Sanitization and Strict RFC 7540/9113 Protocol Compliance in REST-to-gRPC Transcoder)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-29 (`pkg/transcoder`)**: Resolved protocol error Denial-of-Service and HTTP request smuggling vulnerability `SEC-29` ([CWE-444](https://cwe.mitre.org/data/definitions/444.html), [CWE-436](https://cwe.mitre.org/data/definitions/436.html), `SR-081 Finding 7`, `SR-089`) in the REST-to-gRPC Transcoding Engine, preventing hop-by-hop header leakage to upstream gRPC backends.
- **Static Hop-by-Hop Header Sanitization (`pkg/transcoder/transcoder.go`)**: Strips all standard RFC 7230 §6.1 / RFC 7540 §8.1.2.2 / RFC 9113 §8.2.2 connection-specific headers (`Connection`, `Keep-Alive`, `Upgrade`, `Proxy-Connection`, `Transfer-Encoding`, `Proxy-Authenticate`, `Proxy-Authorization`, `Trailer`, `Trailers`, `Host`) before dispatching HTTP/2 gRPC requests (`TASK-105`).
- **Dynamic Connection Token Parsing (`pkg/transcoder/transcoder.go`)**: Dynamically parses comma-delimited tokens in client `Connection` headers and strips matching headers per RFC 7230 §6.1 / RFC 9110 §7.6.1 (`TASK-105`).
- **Strict `TE: trailers` Invariant Enforcement (`pkg/transcoder/transcoder.go`)**: Discards client `TE` values (e.g., `gzip`, `deflate`) and strictly enforces single-valued `TE: trailers` (RFC 7540 §8.1.2.2), preventing upstream gRPC backends from terminating streams with `RST_STREAM (PROTOCOL_ERROR 0x1)` (`TASK-106`).
- **Host Header Sanitization & Metadata Preservation (`pkg/transcoder/transcoder.go`)**: Strips client `Host` header to avoid host spoofing and upstream authority mismatch, while preserving legitimate application authentication and tracing metadata (`Authorization`, `X-Request-Id`, `Traceparent`, `User-Agent`) intact (`TASK-106`).
- **Zero External Dependencies**: Implemented strictly with the Go standard library (`strings`, `net/http`, `sync`).
- **Automated Verification Suite (`pkg/transcoder/transcoder_test.go`)**: Validated standard hop-by-hop stripping, dynamic token stripping, TE trailers invariant, Host header stripping, metadata preservation, and concurrency under `go test -race` (`TASK-107`, `TC-090`).

### Added
- **Automated Tests (`pkg/transcoder/transcoder_test.go`)**:
  - `TestHandleTranscode_StandardHopByHop_Stripped`: Verifies stripping of standard hop-by-hop headers.
  - `TestHandleTranscode_DynamicConnectionTokens_Stripped`: Verifies dynamic token extraction from Connection header and stripping.
  - `TestHandleTranscode_TE_StrictTrailersInvariant`: Verifies single-valued `TE: trailers` enforcement.
  - `TestHandleTranscode_HostHeader_Stripped`: Verifies client Host header removal.
  - `TestHandleTranscode_ApplicationMetadata_Preserved`: Verifies preservation of authentication and tracing headers.
  - `TestHandleTranscode_HeaderSanitization_ConcurrencyRaceSafety`: 50 concurrent requests under `-race`.

### Changed
- **REST-to-gRPC Header Forwarding (`pkg/transcoder/transcoder.go`)**: Filtered out all standard and dynamic hop-by-hop headers, enforced canonical `Content-Type: application/grpc` and `TE: trailers`.

### Related Tasks & Requirements
- `TASK-105`: Static & Dynamic Hop-by-Hop Header Sanitization in Transcoder
- `TASK-106`: Strict TE: trailers Invariant & Metadata Preservation in Transcoder
- `TASK-107`: Automated Verification Suite for Transcoder Protocol Header Compliance
- `REQ-090`: Hop-by-Hop Header Sanitization and Strict Protocol Invariant Enforcement in REST-to-gRPC Transcoder
- `ADR-085`: Hop-by-Hop Header Sanitization and Canonical gRPC Wire Compliance in Transcoder
- `TC-090`: Test Suite for Transcoder Hop-by-Hop Header Sanitization and Protocol Compliance
- `SEC-29`: Hop-by-Hop Header Leakage to Upstream in REST-to-gRPC Transcoder
- `SR-089`: Security Review of SEC-29 Remediation
- `CR-086`: Code Review of REST-to-gRPC Transcoder Hop-by-Hop Header Sanitization and Protocol Compliance

## 2026-09-09 - Toron v1.5.9 Security Release (SEC-28: Bounded Request Body Ingestion and 413 Payload Too Large Rejection in REST-to-gRPC Transcoder)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-28 (`pkg/transcoder`, `pkg/config`)**: Resolved Out-Of-Memory (OOM) Denial of Service vulnerability `SEC-28` ([CWE-400](https://cwe.mitre.org/data/definitions/400.html), [CWE-770](https://cwe.mitre.org/data/definitions/770.html), `SR-081 Finding 6`, `SR-088`) in the REST-to-gRPC Transcoding Engine, eliminating unbounded heap allocation from oversized request bodies.
- **Configurable Transcoder Request Body Limit (`MaxBodyBytes`)**: Added `MaxBodyBytes int64` to `TranscoderConfig` (`yaml:"max_body_bytes,omitempty" json:"max_body_bytes,omitempty"`), defaulting to `4MB` (`4194304` bytes) via `GetMaxBodyBytes()` to match `DefaultMaxGRPCFrameSize` and `ServerConfig.MaxBodyBytes` (`TASK-101`, `REQ-089`, `ADR-084`).
- **Declared `Content-Length` Fast-Fail Rejection (`pkg/transcoder/transcoder.go`)**: Implemented pre-read check on `req.ContentLength`. Incoming requests declaring payload size $> \text{maxBodyBytes}$ are rejected immediately with `HTTP 413 Payload Too Large` without buffer allocation or socket reading (`TASK-102`).
- **Bounded Stream Over-Read Protection (`pkg/transcoder/transcoder.go`)**: Replaced unbounded `io.ReadAll(req.Body)` with `io.LimitReader(req.Body, maxBody+1)`. Chunked or undeclared streams exceeding the limit are rejected with `HTTP 413` and skip `json.Unmarshal`, preventing heap memory explosion (`TASK-103`).
- **Upstream Backend Isolation**: Verified that under all 413 rejection pathways, zero calls are forwarded to the upstream gRPC backend.
- **Full-Fidelity In-Limit Forwarding & Non-Mutating Bypass**: Legitimate payloads $\le \text{maxBodyBytes}$ and non-mutating `nil` body requests (`GET`, `DELETE`) pass through transparently.
- **Zero External Dependencies**: Pure Go standard library implementation (`io`, `net/http`, `encoding/json`, `fmt`, `strconv`, `sync`).
- **Automated Verification Suite (`pkg/transcoder/transcoder_test.go`, `pkg/config/config_test.go`)**: Validated declared Content-Length fast-fail, stream over-read rejection, default fallback, in-limit forwarding, and concurrency under `go test -race` (`TASK-104`, `TC-089`).

### Added
- **Configuration Fields**: `MaxBodyBytes int64` in `TranscoderConfig` with `GetMaxBodyBytes()` helper method.
- **Automated Tests (`pkg/transcoder/transcoder_test.go`, `pkg/config/config_test.go`)**:
  - `TestTranscoderConfig_MaxBodyBytesSchemaAndValidation`: Schema defaults, fallback, YAML/JSON unmarshaling, negative value rejection.
  - `TestHandleTranscode_DeclaredContentLength_FastFail`: Fast-fail 413 on declared oversized `Content-Length`.
  - `TestHandleTranscode_StreamOverRead_Rejection`: Bounded stream over-read rejection with 413.
  - `TestHandleTranscode_InLimit_Success`: In-limit payload parsed and forwarded with full fidelity.
  - `TestHandleTranscode_NonMutatingNilBody_Bypass`: Safe handling of `req.Body == nil`.
  - `TestHandleTranscode_ConcurrencyRaceSafety`: 50 concurrent requests under `-race`.

### Changed
- **REST Request Ingestion (`pkg/transcoder/transcoder.go`)**: Replaced raw `io.ReadAll` with two-tier fast-fail and `io.LimitReader` bounded reading with HTTP 413 rejection.

### Related Tasks & Requirements
- `TASK-101`: Configurable Transcoder Request Body Limit Schema
- `TASK-102`: Fast-Fail Rejection on Declared Content-Length in Transcoder
- `TASK-103`: Bounded Stream Over-Read Protection & 413 Rejection in Transcoder
- `TASK-104`: Automated Verification Suite for REST-to-gRPC Transcoder Request Body Limits
- `REQ-089`: Bounded Request Body Ingestion and 413 Payload Too Large Rejection in REST-to-gRPC Transcoder
- `ADR-084`: Bounded Request Body Ingestion and 413 Rejection Architecture in REST-to-gRPC Transcoder
- `TC-089`: Verification Suite for REST-to-gRPC Transcoder Request Body Limits
- `SEC-28`: Unbounded Request Body Ingestion in REST-to-gRPC Transcoder
- `SR-088`: Security Review of SEC-28 Remediation
- `CR-085`: Code Review of REST-to-gRPC Transcoder Request Body Limiting and 413 Rejection

## 2026-09-09 - Toron v1.5.8 Security Release (SEC-27: Maximum Idle Deadline Enforcement on Upgraded Protocol and WebSocket Connections)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-27 (`pkg/server`, `pkg/config`)**: Resolved critical Slowloris resource exhaustion vulnerability `SEC-27` ([CWE-400: Uncontrolled Resource Consumption](https://cwe.mitre.org/data/definitions/400.html), `SR-081 Finding 5`, `SR-087`) in core HTTP/1.1 protocol upgrade handling and HTTP/2 Extended CONNECT streams, eliminating permanent socket and goroutine leaks.
- **Configurable Upgraded Inactivity Deadline (`UpgradeIdleTimeout`)**: Added `UpgradeIdleTimeout time.Duration` to `server.Config` and `ServerConfig` (`yaml:"upgrade_idle_timeout,omitempty" json:"upgrade_idle_timeout,omitempty"`), defaulting to `60s` with a 3-tier fallback hierarchy (`TASK-097`, `REQ-088`, `ADR-083`).
- **Bidirectional Activity-Refreshed Deadline Relay (`pkg/server/server.go`)**: Replaced permanent deadline stripping (`SetDeadline(time.Time{})`) and unbounded `io.Copy` with an active deadline relay (`relayUpgradedStreams`). Every transferred chunk refreshes read and write deadlines; inactivity exceeding `UpgradeIdleTimeout` terminates both sockets deterministically via `sync.Once` and unblocks relay goroutines (`TASK-098`).
- **HTTP/2 Extended CONNECT Protection (`pkg/server/server.go`)**: Enforced `UpgradeIdleTimeout` on RFC 8441 extended CONNECT streams, with immediate socket teardown upon client request context cancellation (`r.Context().Done()` / `RST_STREAM`) (`TASK-099`).
- **Transport-Layer Heartbeat Transparency**: RFC 6455 WebSocket Ping/Pong control frames and application keep-alive messages continuously refresh the deadline, preserving legitimate persistent sessions indefinitely.
- **TCP Half-Close Propagation**: Supported `CloseWrite()` upon reading `io.EOF`, allowing reverse responses to drain while maintaining deadline enforcement.
- **Zero External Dependencies**: Implemented strictly with Go standard library packages (`net`, `sync`, `sync/atomic`, `time`, `io`, `log`, `errors`).
- **Automated Verification Suite (`pkg/server/server_test.go`)**: Tested idle timeout socket teardown, heartbeat keep-alive survival, peer disconnect cleanup, HTTP/2 extended CONNECT, and high concurrency under `go test -race` (`TASK-100`, `TC-088`).

### Added
- **Configuration Fields**: `UpgradeIdleTimeout time.Duration` in `server.Config` and `ServerConfig`.
- **Automated Tests (`pkg/server/server_test.go`, `pkg/config/config_test.go`)**:
  - `TestConfig_UpgradeIdleTimeoutSchemaAndFallback`: Schema defaults, fallback, and validation.
  - `TestServer_UpgradedConn_IdleTimeout`: Automatic socket teardown upon silence.
  - `TestServer_UpgradedConn_HeartbeatKeepsAlive`: Periodic heartbeats/pings sustain connection.
  - `TestServer_UpgradedConn_PeerDisconnect`: Immediate clean teardown on peer close.
  - `TestServer_UpgradedConn_HalfClose`: Client `CloseWrite()` propagation with reverse stream draining.
  - `TestServer_HTTP2_ExtendedCONNECT_IdleAndCancel`: HTTP/2 extended CONNECT idle and cancel teardown.
  - `TestServer_UpgradedConn_ConcurrencyRaceSafety`: 40 concurrent streams tested under `-race`.

### Changed
- **HTTP/1.1 Protocol Switching (`pkg/server/server.go`)**: Replaced unbounded `io.Copy` with `relayUpgradedStreams`.
- **HTTP/2 Extended CONNECT (`pkg/server/server.go`)**: Replaced unbounded `io.Copy` with `relayHTTP2UpgradedStream`.

### Related Tasks & Requirements
- `TASK-097`: Configurable Upgraded Inactivity Deadline Schema
- `TASK-098`: Bidirectional Activity-Refreshed Deadline Relay for HTTP/1.1 Upgrades
- `TASK-099`: Idle Deadline Enforcement for HTTP/2 Extended CONNECT Upgraded Streams
- `TASK-100`: Automated Verification Test Suite for Upgraded Connection Idle Deadlines
- `REQ-088`: Maximum Idle Deadline Enforcement on Upgraded Protocol and WebSocket Connections
- `ADR-083`: Bidirectional Activity-Refreshed Deadlines for Upgraded Protocol Sockets
- `TC-088`: Upgraded Connection Idle Deadline & Resource Reclamation Test Suite
- `SEC-27`: Missing Maximum Idle Deadlines on Upgraded Protocol Connections
- `SR-087`: Security Review of SEC-27 Remediation
- `CR-084`: Code Review of Upgraded Protocol Connection Idle Deadline Enforcement

## 2026-09-09 - Toron v1.5.7 Security Release (SEC-26: Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies)

### Milestone Summary
- **Remediation of Critical Vulnerability SEC-26 (`pkg/proxy`, `pkg/config`, `cmd/toron`)**: Successfully resolved critical vulnerability `SEC-26` ([CWE-400: Uncontrolled Resource Consumption](https://cwe.mitre.org/data/definitions/400.html), `SR-081 Finding 4`, `SR-086`) in Layer 4 transport proxies (`TCPProxy` and `UDPProxy`), eliminating memory exhaustion (OOM), host ephemeral port starvation (`bind: address already in use`), socket file descriptor leaks (`EMFILE`), and Slowloris denial of service.
- **Declarative Configuration Schema (`pkg/config`)**: Extended `ProxyRouteConfig` with `MaxConnections` (`max_connections`, default `10000`), `IdleTimeout` (`idle_timeout`, default `60s`), and `MaxWorkers` (`max_workers`, default `1024`), backed by safe fallback getter methods (`GetMaxConnections`, `GetIdleTimeout`, `GetMaxWorkers`) (`TASK-093`, `REQ-087`, `ADR-082`).
- **TCP Concurrency Limits & Fast-Fail Rejection (`pkg/proxy/tcp.go`)**: Enforced atomic active connection tracking. Incoming connections exceeding `MaxConnections` are closed immediately upon `Accept()` without dialing upstream backends, allocating heap memory, or spawning relay goroutines (`TASK-094`).
- **Bidirectional TCP Idle Deadlines & Slowloris Protection (`pkg/proxy/tcp.go`)**: Replaced unbounded blocking `io.Copy` stream relays with deadline-aware transfer loops. Read and write deadlines are updated on active data transfer; connections with zero throughput for `IdleTimeout` are terminated immediately, freeing socket file descriptors and unblocking worker goroutines. Cleanly supports TCP half-close (`CloseWrite`).
- **UDP Bounded Worker Pool & Saturated Queue Dropping (`pkg/proxy/udp.go`)**: Eliminated unconstrained per-packet goroutine spawning (`go p.handleDatagram(...)`) by implementing a fixed-capacity task channel (`packetQueue`) of size `MaxWorkers` serviced by long-lived worker goroutines. Saturated queues drop excess datagrams fail-safe without memory growth or panics (`TASK-095`).
- **UDP Client Session Registry & Upstream Socket Reuse (`pkg/proxy/udp.go`)**: Implemented a thread-safe session registry (`sessions map[netip.AddrPort]*udpSession`). Subsequent datagrams from the same client reuse the open outbound `*net.UDPConn`, completely eliminating per-packet socket dials and ephemeral port exhaustion. A background sweeper routine evicts idle sessions after `IdleTimeout` of inactivity.
- **Zero-Allocation Buffer Recycling with `sync.Pool` (`pkg/proxy/udp.go`)**: Datagram buffers (64 KB / 65,535 bytes) are recycled across inbound reads and upstream responses, eliminating per-packet heap allocations and garbage collection pauses.
- **Deterministic Graceful Teardown (`pkg/proxy`)**: Calling `Close()` on either proxy immediately closes listeners, active client sockets, upstream connections, and worker pools within $\le 500\text{ms}$.
- **Zero External Dependencies**: Pure Go standard library implementation (`net`, `net/netip`, `sync`, `sync/atomic`, `time`, `io`).
- **Microbenchmark Verification (`pkg/proxy/tcp_test.go`, `pkg/proxy/udp_test.go`)**: Confirmed ~27,000 ops/sec (0 allocs/op steady-state) for TCP streaming and ~21,700 pkts/sec (0 buffer allocs) for UDP datagram forwarding (`TASK-096`, `TC-087`).

### Added
- **Configuration Fields**: Added `MaxConnections int`, `IdleTimeout time.Duration`, and `MaxWorkers int` to `ProxyRouteConfig` in `pkg/config/config.go`.
- **Helper Getters**: `GetMaxConnections()` (defaults to 10,000), `GetIdleTimeout()` (defaults to 60s), and `GetMaxWorkers()` (defaults to 1,024).
- **Functional Options**: Introduced `WithTCPMaxConnections`, `WithTCPIdleTimeout`, `WithUDPMaxWorkers`, and `WithUDPIdleTimeout` for flexible proxy configuration.
- **Automated Verification Test Suite (`pkg/proxy/tcp_test.go`, `pkg/proxy/udp_test.go`)**:
  - `TestTCPProxy_MaxConnections`: Connection capacity gating and fast rejection.
  - `TestTCPProxy_IdleTimeout`: Stagnant stream teardown and deadline refreshing.
  - `TestUDPProxy_WorkerPoolSaturation`: Saturated queue fail-safe drops and bounded goroutines.
  - `TestUDPProxy_SocketReuse`: Outbound socket reuse across multiple client datagrams.
  - `TestUDPProxy_SessionIdleTimeout`: Inactive session sweeper eviction and socket release.
  - `TestUDPProxy_BufferPooling`: `sync.Pool` recycling validation ($\le 2$ allocs/op).
  - `TestTCPProxy_GracefulShutdown` & `TestUDPProxy_GracefulShutdown`: Graceful teardown within $\le 500\text{ms}$.
  - `TestTCPProxy_ConcurrencyRaceSafety` & `TestUDPProxy_ConcurrencyRaceSafety`: High-concurrency race safety under `go test -race`.
  - `BenchmarkTCPProxy_Forwarding` & `BenchmarkUDPProxy_Forwarding`: Microbenchmarks.

### Changed
- **Main Gateway Startup (`cmd/toron/main.go`)**: Layer 4 proxy initialization propagates route parameters into `NewTCPProxy` and `NewUDPProxy` with structured diagnostic logging.
- **TCP Stream Forwarding (`pkg/proxy/tcp.go`)**: Replaced unbounded `io.Copy` with bidirectional deadline loops enforcing `idle_timeout`.
- **UDP Datagram Forwarding (`pkg/proxy/udp.go`)**: Replaced per-packet goroutines with bounded worker pools, session caching, and buffer recycling.

### Related Tasks & Requirements
- `TASK-093`: Layer 4 Proxy Configuration Schema & Wiring
- `TASK-094`: Bounded Concurrency, Bidirectional Idle Deadlines & Connection Tracking in TCPProxy
- `TASK-095`: Bounded Worker Pool, sync.Pool Buffer Recycling & Session Socket Reuse in UDPProxy
- `TASK-096`: Automated Verification Suite for Layer 4 TCP & UDP Proxy Hardening
- `REQ-087`: Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies
- `ADR-082`: Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies
- `TC-087`: Layer 4 Concurrency, Socket Reuse, and Idle Deadline Test Suite
- `SEC-26`: Uncontrolled Resource Consumption in Layer 4 TCP and UDP Proxies
- `SR-086`: Security Review of SEC-26 Remediation
- `CR-083`: Code Review of Layer 4 TCP and UDP Proxy Hardening

## 2026-09-08 - Toron v1.5.6 Security Release (SEC-25: Configurable Request Body Limits & HTTP 413 Rejection in Service Mesh Sidecar)

### Milestone Summary
- **Configurable Request Body Limit in Sidecar Proxy (`pkg/config`, `pkg/sidecar`)**: Introduced configurable maximum request body size parameter `SidecarConfig.MaxBodyBytes` (and `max_body_bytes` in YAML/JSON) into `SidecarConfig`, eliminating hardcoded buffer limits and providing a reliable default fallback of 10 MB (`10,485,760` bytes) when unset or non-positive (`TASK-090`, `REQ-086`, `ADR-081`, `SEC-25`).
- **Elimination of Silent Body Truncation & Upstream Data Corruption (`pkg/sidecar`)**: Fixed critical vulnerability `SEC-25` (CWE-436, CWE-400) in `ProxyEngine.proxyToURL` where payloads exceeding the limit were silently truncated and forwarded upstream as corrupt data. Oversized requests are now immediately rejected with HTTP `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`), request bodies are closed, and proxying is terminated with zero bytes transmitted upstream (`TASK-091`).
- **Dual-Stage Overflow Protection**:
  - **Fast-Path Declared `Content-Length` Guard**: If an incoming request declares a `Content-Length` greater than `MaxBodyBytes`, the sidecar closes the body and immediately returns HTTP 413 without reading the payload, avoiding memory allocation overhead.
  - **Streaming Bounded Over-Read Guard**: For chunked transfer encodings or streams with undeclared lengths, ingestion is bounded using `io.LimitReader(r.Body, MaxBodyBytes+1)`. If the payload exceeds the limit, the stream is aborted, read bytes discarded, and HTTP 413 returned.
- **Direct Bypass for Non-Mutating Requests**: Safe requests (`GET`, `HEAD`) or empty requests (`ContentLength == 0` or `r.Body == nil`) bypass body reading and are dispatched directly to upstream handlers without buffer allocation.
- **Byte-Fidelity Payload Forwarding**: In-bounds requests within `MaxBodyBytes` are forwarded to the target application with exact byte fidelity and accurate content lengths.
- **Automated Verification Suite (`pkg/sidecar/sidecar_test.go`)**: Implemented unit, integration, and high-concurrency race test cases (`TASK-092`, `TC-086`) validating default fallback, fast-fail 413 responses, chunked stream rejection, in-bounds byte preservation, and concurrent race safety under `go test -race ./pkg/sidecar/...`.

### Added
- **`MaxBodyBytes int64` Field**: Added to `SidecarConfig` in `pkg/config/config.go` with tags `yaml:"max_body_bytes" json:"max_body_bytes"`.
- **Default Fallback Normalization**: Added 10 MB fallback in `DefaultAppConfig`, `validateConfigDefaults`, and `NewProxyEngine`.
- **Automated Test Cases (`pkg/sidecar/sidecar_test.go`)**:
  - `TestTC086_01_DefaultLimitAndFallback`: Validates 10 MB fallback for zero/negative values.
  - `TestTC086_02_DeclaredContentLength413Rejection`: Validates fast-path 413 rejection on declared `Content-Length` overflow.
  - `TestTC086_03_ChunkedStreamOverRead413Rejection`: Validates bounded `io.LimitReader` 413 rejection on chunked/streamed overflow.
  - `TestTC086_04_ByteFidelityInBoundsForwarding`: Validates exact byte preservation for valid requests under the limit.
  - `TestTC086_05_NonMutatingAndEmptyPayloadBypass`: Validates direct bypass for GET/HEAD and empty requests.
  - `TestTC086_06_HighConcurrencyRaceSafety`: Validates concurrent execution across parallel goroutines with zero data races.

### Changed
- **Proxy Body Handler (`pkg/sidecar/proxy.go`)**: Replaced silent truncation logic in `ProxyEngine.proxyToURL` with dual-stage fail-fast HTTP 413 rejection and diagnostic logging.

### Related Tasks & Requirements
- `TASK-090`: Configurable Sidecar Body Size Limit in SidecarConfig
- `TASK-091`: Immediate HTTP 413 Payload Too Large Rejection in Sidecar ProxyEngine
- `TASK-092`: Automated Verification Suite for Sidecar Body Limiting & 413 Rejection
- `REQ-086`: Configurable Request Body Limit and Truncation Rejection in Service Mesh Sidecar Proxy
- `ADR-081`: Configurable Request Body Limiting and Truncation Rejection in Service Mesh Sidecar Proxy
- `TC-086`: Sidecar Request Body Size Limit and HTTP 413 Rejection Verification
- `SEC-25`: Silent Request Body Truncation in Service Mesh Sidecar Proxy

## 2026-09-05 - Toron v1.5.5 Feature Release (Universal Query Parameter Forwarding & Preservation)

### Milestone Summary
- **Universal Query Parameter Forwarding (`pkg/proxy`, `pkg/server`, `pkg/httpparser`)**: Resolved an issue where query parameters were dropped when proxying HTTP/2 and HTTP/3 requests due to unpopulated `QueryParams` and `URL.RawQuery` in the ingress adapter handler (`TASK-060`, `REQ-060`, `ADR-055`).
- **Verbatim Client Query Preservation**: Reverse proxy now forwards raw query parameters (`req.URL.RawQuery`) verbatim, preserving exact parameter order, percent-encoding, and non-value flags without sorting mutations.
- **Target Query Merging**: Pre-configured query parameters in upstream `target` definitions are cleanly merged with incoming client query parameters.
- **Standardized Ingress Request Factory**: Added `httpparser.NewRequestFromStd` and lazy query accessor `req.Query()` to ensure uniform request representations across all protocols.
- **Access Log Visibility**: `AccessLoggerMiddleware` now records full `RequestURI` in `/var/log/toron/access.log`, providing complete visibility of request query strings.

### Added
- **`httpparser.NewRequestFromStd` & `req.Query()`**: Centralized adapter converting standard library `*http.Request` to `*httpparser.Request`.
- **Unit & Integration Tests (`pkg/proxy/proxy_test.go`, `pkg/httpparser/parser_test.go`)**: Tests covering HTTP/1.1 and HTTP/2 query forwarding, verbatim query string preservation, and target query merging.

### Related Tasks
- `TASK-060`: Implement Query Parameter Forwarding and Preservation Across HTTP/1.1, HTTP/2, and HTTP/3

## 2026-09-05 - Toron v1.5.4 Feature Release (Upstream Reverse Proxy Path Rewriting and Target Subpath Preservation)

### Milestone Summary
- **Deterministic Proxy Path Rewriting (`pkg/proxy`)**: Enhanced reverse proxy path joining logic (`JoinProxyPath`) to accurately preserve upstream target URLs containing subpaths (e.g. `http://127.0.0.1:8082/postback`), eliminating unwanted trailing slash injection when `strip_prefix: true` is enabled (`TASK-059`, `REQ-059`, `ADR-054`).
- **Nginx Parity for Reverse Proxying**: Brings Toron into full parity with standard reverse proxies (Nginx, Envoy, Caddy), allowing upstream microservices (such as Go standard library `http.ServeMux` or strict REST routers) to receive the exact requested path without trailing slash discrepancies.
- **Explicit Trailing Slash & Nested Path Preservation**: Client-provided trailing slashes (`/kite/postback/`) and nested subpaths (`/kite/postback/status`) continue to be accurately preserved and joined.

### Added
- **Exported `JoinProxyPath` Function (`pkg/proxy/proxy.go`)**: Dedicated function for resolving upstream request paths based on target path, request path, prefix, and strip prefix configuration.
- **Unit & Integration Test Suite (`pkg/proxy/proxy_test.go`)**: Table-driven tests (`TestJoinProxyPath_TableDriven`) and full HTTP reverse proxy integration tests (`TestReverseProxy_SubpathTarget_Integration`).

### Related Tasks
- `TASK-059`: Implement Upstream Reverse Proxy Path Rewriting and Subpath Target Preservation

## 2026-09-04 - Toron v1.5.3 Feature Release (Config-Based Multi-Stream Logging & Daily System Logrotate Support)

### Milestone Summary
- **Config-Based Multi-Stream Logging Engine (`pkg/logging`)**: Implemented a thread-safe, decoupled logging engine with dedicated streams for internal server logs (`server_log`), HTTP request/response transactional access logs (`access_log`), and security audit telemetry (`security_log`), with configurable output formats (`text` Combined format and structured `json`) (`TASK-058`, `REQ-058`, `ADR-053`).
- **Declarative Global Defaults & Per-Route Overrides (`pkg/config`, `pkg/router`)**: Default file paths are configured globally in `config.yaml`, with support for overriding `access_log` and `security_log` on a per-route basis in `routes.yaml` (including silencing via `"off"` or `"none"`).
- **Daily System Logrotate Integration (`etc/logrotate.d/toron`)**: All log files are opened in append mode (`O_APPEND`), guaranteeing zero corruption under `copytruncate`. In addition, Toron establishes an OS signal handler listening for `SIGHUP` to atomically close and reopen all active file descriptors for standard rename/rotate workflows without dropping connections or restarting the daemon.
- **Enterprise WAF & Security Audit Synchronization**: WAF audit logging, authentication failures, and rate limit blocks seamlessly route into the configured security log stream or route-specific security log files.

### Added
- **Multi-Stream Log Manager (`pkg/logging/manager.go`)**: Dedicated package managing thread-safe `LogSink` handles, automatic directory creation (`os.MkdirAll`), text/JSON formatting, and atomic `Reopen()` upon rotation signals.
- **Configuration Schema Extensions (`pkg/config/config.go`)**: Added `ServerLog`, `AccessLog`, and `SecurityLog` to `LoggingConfig`, and added `AccessLog` and `SecurityLog` to `ProxyRouteConfig` with helper query methods.
- **Access Logging Middleware (`pkg/router/router.go`)**: Added `AccessLoggerMiddleware` and `MatchPrefixRoute` to automatically direct access logs according to matched route configuration.
- **System Logrotate Template (`etc/logrotate.d/toron`)**: Standard UNIX logrotate configuration rotating daily, preserving 7 generations, compressing rotated logs, and executing `pkill -HUP -f "toron"`.
- **Unit & Race Test Suite (`pkg/logging/manager_test.go`, `pkg/config/config_test.go`, `pkg/router/router_test.go`)**:
  - `TestLogSink_FileCreationAndDirectory`: Verifies sink opening, nested folder auto-creation, and append mode.
  - `TestLogSink_ReopenLogrotate`: Verifies log rotation simulation (rename -> reopen -> verify lines in old vs new files).
  - `TestLogManager_MultiStreamAndRouteOverrides`: Verifies server log, access log, security log, route overrides, and silencing.
  - `TestLogManager_JSONFormat`: Verifies structured JSON telemetry serialization.
  - `TestLogManager_ConcurrentWrites`: Verifies 50 concurrent worker routines writing 5,000 log records under the race detector.
  - `TestExtractClientIP`: Verifies socket `RawConn.RemoteAddr` extraction, `X-Forwarded-For`, and `X-Real-IP`.
  - `TestRouter_AccessLoggerMiddleware_RouteOverrides`: Verifies HTTP access logging middleware with default, custom route, and silenced routes.
  - `TestConfig_LoggingConfigAndRouteOverrides`: Verifies configuration parsing for global logging and route overrides.

### Changed
- **Server Logging (`cmd/toron/main.go`)**: Standard Go logger (`log.SetOutput`) redirected to `server_log` sink when configured.
- **Signal Handling (`cmd/toron/main.go`)**: Added `syscall.SIGHUP` listener for live log reopening without socket drops.

### Related Tasks
- `TASK-058`: Implement Config-Based Multi-Stream Logging, Route Overrides, and Logrotate Support

## 2026-09-04 - Toron v1.5.2 Feature Release (Single Page Application HTML5 History Fallback Support)

### Milestone Summary
- **Single Page Application (SPA) HTML5 History Fallback (`pkg/router`, `pkg/config`)**: Added native client-side routing fallback support for static routes (`spa: true`, `fallback: "<file>"`), achieving parity with Nginx's `try_files $uri $uri/ /index.html;` directive without requiring external reverse proxies or application runtimes (`TASK-056`, `REQ-056`, `ADR-051`).
- **Asset Masking Protection**: Implemented strict extension-based non-masking logic (`filepath.Ext(relPath) != ""`). Missing physical static assets (e.g., `.js`, `.css`, `.png`, `.svg`, `.json`) return HTTP 404 Not Found rather than the HTML fallback document, preventing cryptic browser runtime syntax errors (such as `Uncaught SyntaxError: Unexpected token '<'`) and CSS MIME-type mismatch rejections.
- **Path Traversal & Boundary Containment Defense**: Hardened fallback path resolution with directory boundary verification (`filepath.Rel` and `filepath.EvalSymlinks`), guaranteeing that fallback documents remain strictly contained within the configured static root directory and rejecting traversal/escape attempts with HTTP 403 Forbidden or HTTP 404 Not Found.
- **Declarative YAML & JSON Configuration**: Extended `ProxyRouteConfig` and `ProxyOptions` with `spa` (boolean) and `fallback` (string) options, supporting default (`index.html`) or custom fallback documents (e.g., `app.html`, `200.html`) and implicit SPA activation whenever `fallback` is configured.

### Added
- **Declarative Route Configuration (`pkg/config`)**: Added `SPA bool` and `Fallback string` attributes to `ProxyRouteConfig` with YAML and JSON unmarshaling support (`pkg/config/config.go`).
- **Proxy Options Propagation (`pkg/proxy`)**: Extended `ProxyOptions` in `pkg/proxy/proxy.go` with `SPA` and `Fallback` fields, forwarded during static route registration in `cmd/toron/main.go`.
- **Static Route SPA Fallback Handler (`pkg/router`)**: Updated `createStaticHandler` in `pkg/router/router.go` to evaluate client navigation paths on cache/filesystem misses (`os.IsNotExist`), transparently serving the configured fallback document with HTTP 200 OK and `Content-Type: text/html; charset=utf-8` while omitting the response body for HTTP `HEAD` requests.
- **Comprehensive Unit & Race Test Suite (`pkg/config/config_test.go`, `pkg/router/router_test.go`)**:
  - `TestConfig_SPARouteConfig`: Validates parsing of `spa` and `fallback` across YAML route configurations.
  - `TestRouter_SPA_PhysicalFileServing`: Verifies direct serving of existing static files and directory indices.
  - `TestRouter_SPA_NavigationFallback`: Verifies 200 OK HTML fallback for client-side navigation paths without extensions (including deep nested paths).
  - `TestRouter_SPA_AssetProtection404`: Verifies strict 404 Not Found returns for missing assets with extensions.
  - `TestRouter_SPA_CustomFallback`: Verifies custom fallback file resolution and implicit SPA activation.
  - `TestRouter_Static_NonSPABackwardCompatibility`: Confirms static routes without SPA settings maintain standard 404 behavior.
  - `TestRouter_SPA_SecurityPathTraversal`: Validates rejection of directory traversal and boundary escape attempts.

### Changed
- **Static Route Handler (`pkg/router/router.go`)**: Modified missing file resolution pipeline to distinguish between client-side virtual navigation routes and physical asset requests when SPA mode is active.

### Related Tasks
- `TASK-056`: Implement SPA HTML5 History Fallback Support for Static Routes

## 2026-09-04 - Toron v1.5.1 Maintenance & Feature Release (HTTP/3 QUIC Listener Engine Integration)

### Milestone Summary
- **HTTP/3 QUIC Listener Engine Integration (`pkg/server`)**: Integrated `github.com/quic-go/quic-go/http3` server engine into Toron's core HTTP server (`ListenAndServeH3`, `H3Server`, `SetH3Server`), allowing native HTTP/3 transport over UDP with full router middleware reuse (`TASK-027`, `REQ-027`, `ADR-022`).
- **Concurrent UDP Startup in `cmd/toron` (`cmd/toron/main.go`)**: Main application entrypoint now spawns a concurrent background goroutine running `srv.ListenAndServeH3()` bound to the configured UDP socket when TLS and HTTP/3 are enabled, running simultaneously alongside the primary HTTPS/TLS TCP listener without blocking server orchestration.
- **Alt-Svc Protocol Upgrade Advertisement on HTTP/2 (`pkg/server`)**: Extended `http2AdapterHandler` to automatically inject `Alt-Svc: h3=":port"; ma=2592000` response headers across HTTP/2 (and HTTP/1.1) connections, notifying compliant clients and browsers to upgrade subsequent requests to HTTP/3 QUIC while preserving pre-existing `Alt-Svc` headers.
- **Graceful QUIC Socket Shutdown (`pkg/server`)**: Enhanced `Server.Shutdown(ctx)` with `sync.RWMutex`-guarded `s.h3Server.Close()` invocation to ensure all UDP QUIC listeners and active streams are cleanly drained and closed, mapping `http.ErrServerClosed` to `ErrServerClosed` and suppressing false-positive shutdown errors in `cmd/toron/main.go`.

### Added
- **Thread-Safe HTTP/3 Server Pointer Management**: Added `H3Server()` and `SetH3Server(h *http3.Server)` to `pkg/server/server.go` protected by `sync.RWMutex`.
- **HTTP/2 Adapter Alt-Svc Advertising**: Added automatic `Alt-Svc` header injection in `http2AdapterHandler()` in `pkg/server/server.go`.
- **UDP QUIC Listener Orchestration**: Concurrent background listener in `cmd/toron/main.go` handling HTTP/3 over UDP port (default: 8443) when TLS is active.
- **Unit & Regression Test Coverage (`pkg/server/server_test.go`)**:
  - `TestServer_HTTP2Adapter_AltSvcHeader`: Tests `Alt-Svc` header injection with default port 8443, custom port 9443, disabled advertising, and existing header preservation.
  - `TestServer_HTTP3_Shutdown`: Tests active HTTP/3 server shutdown (`Close()`) and nil-safety during graceful teardown.

### Changed
- **Graceful Shutdown**: `Server.Shutdown(ctx)` coordinates both TCP reactor shutdown and UDP QUIC socket termination (`s.h3Server.Close()`).

### Related Tasks
- `TASK-027`: Implement HTTP/3 Protocol Engine & QUIC Listener Support

## 2026-08-18 - Toron v1.5.0 Feature Release (Prototypes 40 & 41: Advanced Load Balancing & Universal Auto-Installer)

### Milestone Summary
- **Advanced Load Balancing Engine (`pkg/proxy`)**: Implemented high-throughput pluggable load balancing suite supporting 8 strategies: `round_robin`, `weighted_round_robin`, `random`, `weighted_random`, `least_conn`, `weighted_least_conn`, `least_latency` (EMA response-time tracking), `sticky_cookie`, and `ip_hash` (`TASK-050`, `REQ-050`, `ADR-045`).
- **Universal Auto-Installer & Service Manager (`install.sh` / `install.bat`)**: Automated zero-dependency deployment script supporting Linux (`systemd`), macOS (`launchd`), and Windows (`sc.exe`) on both ARM64 and AMD64 architectures with automatic daemon loading, daily log rotation, and clean uninstallation (`TASK-051`, `REQ-051`, `ADR-046`).
- **Prefix Management & 3xx Redirect/Cookie Rewriting (`pkg/proxy`)**: Automated inbound `X-Forwarded-Prefix` injection, 3xx `Location` redirect rewriting, and `Set-Cookie: Path=` scoping for prefix-routed legacy and modern microservices (`TASK-052`, `REQ-052`, `ADR-047`).
- **Dynamic Upstream Health Matrix & Telemetry Dashboard (`pkg/server`, `public/`)**: Removed all hardcoded mock ports and dummy services from the dashboard, replacing them with dynamic runtime route aggregation, concurrent target health probing, and dynamic API tester endpoints (`TASK-053`, `REQ-053`, `ADR-048`).
- **Dashboard v2.0 Redesign (`public/`)**: Redesigned modern minimalist, mobile-first control center with 3-way light/dark/system theme synchronization, instant route search, and upgraded interactive API console (`TASK-054`, `REQ-054`, `ADR-049`).
- **Centralized Version Management (`pkg/version`, `VERSION`)**: Unified single-source-of-truth semantic versioning across Go runtime, linker metadata injection (`-ldflags`), Makefile, and OS auto-installers with `-v`/`-version` CLI support (`TASK-055`, `REQ-055`, `ADR-050`).
- **Standardized Configuration Standard**: Enforced `/etc/toron/` (`config.yaml`, `routes.yaml`, `public/`) system configuration and `/var/log/toron/` centralized logging paths across system daemons.

### Related Tasks
- `TASK-050`: Implementation Breakdown for Advanced Load Balancing Algorithms
- `TASK-051`: Implement Universal Auto-Installer & Service Manager (`install.sh`)
- `TASK-052`: Implement Reverse Proxy Prefix Management & Redirect/Cookie Rewriting
- `TASK-053`: Implement Dynamic Upstream Discovery & Data-Driven Dashboard
- `TASK-054`: Implement Dashboard v2.0 UI Redesign & Multi-Theme Engine
- `TASK-055`: Implement Centralized Version Tracking & Build Metadata Injection

## 2026-08-16 - Toron v1.4.0 Feature Release (Prototype 39: REST-to-gRPC Transcoding Engine)

### Milestone Summary
- **REST-to-gRPC Transcoding Engine (`pkg/transcoder`)**: Implemented direct JSON HTTP REST (`GET /v1/users/:id`) to binary Protobuf HTTP/2 gRPC (`POST /user.UserService/GetUser`) request and response translation (`TASK-049`, `REQ-049`).
- **Zero External Dependencies**: Pure Go stdlib implementation using `encoding/binary`, `encoding/json`, `net/http`, and `toron/pkg/httpparser` without protobuf compiler tools.
- **gRPC 5-Byte Wire Framing**: Automated encoding and decoding of gRPC wire frames (`[0x00][4-byte length] + payload`).
- **gRPC Status Mapping**: Automatic conversion of gRPC trailer status codes (`grpc-status: 0` -> 200 OK, `grpc-status: 5` -> 404 Not Found, `grpc-status: 16` -> 401 Unauthorized) to standard HTTP status codes.

### Related Tasks
- `TASK-049`: Implement REST-to-gRPC Transcoding Engine

## 2026-08-16 - Toron v1.3.0 Feature Release (Prototype 38: Service Mesh Sidecar Mode)

### Milestone Summary
- **Service Mesh Sidecar Mode (`pkg/sidecar`)**: Implemented lightweight pod-level proxy mode enforcing pod-to-pod Mutual TLS (mTLS) encryption and dynamic weighted traffic splitting (`TASK-048`, `REQ-048`).
- **Zero External Dependencies**: Pure Go stdlib HTTP & TLS client/server listeners operating on dedicated local ports (`15006` ingress, `15001` egress).
- **Strict Mutual TLS**: Support for `RequireAndVerifyClientCert` with Root CA validation pools (`ca_file`).
- **Weighted Traffic Splitting**: Thread-safe atomic weighted round-robin selector (`WeightedSplitter`) for canary traffic distribution (e.g. 80/20 ratio).

### Related Tasks
- `TASK-048`: Implement Service Mesh Sidecar Mode Engine

## 2026-08-16 - Toron v1.2.0 Feature Release (Prototype 37: Native Kubernetes Ingress Controller)

### Milestone Summary
- **Native Zero-Dependency Kubernetes Ingress Controller (`pkg/ingress`)**: Implemented Kubernetes `networking.k8s.io/v1` Ingress Controller translating `Ingress`, `Service`, `Endpoints`, and TLS `Secret` resources into Toron's core routing matrix (`TASK-047`, `REQ-047`).
- **Zero External Dependencies**: Pure Go stdlib HTTP & TLS client communicating with Kubernetes API server without importing `k8s.io/client-go`.
- **In-Cluster Auto-Authentication**: Automated ServiceAccount bearer token and Root CA certificate loading from `/var/run/secrets/kubernetes.io/serviceaccount/`.
- **Real-Time Endpoint Watching**: Streaming watch worker (`watch=true`) dynamically updates load balancing targets as pod IP endpoints scale or shift.

### Related Tasks
- `TASK-047`: Implement Native Kubernetes Ingress Controller Engine

## 2026-08-16 - Toron v1.1.0 Feature Release (Prototype 36: Vendor-Agnostic OCI Container Auto-Discovery Engine)

### Milestone Summary
- **Vendor-Agnostic OCI Container Auto-Discovery Engine (`pkg/discovery`)**: Implemented dynamic container discovery engine monitoring Unix domain sockets across Docker Engine, Podman, Finch, and Nerdctl (`TASK-046`, `REQ-046`).
- **Zero External Dependencies**: Pure Go stdlib HTTP transport (`net.DialContext("unix", ...)`) over Unix domain sockets without 3rd-party Docker or Containerd SDKs.
- **Unified `toron.*` Metadata Label Taxonomy**: Automatic extraction of container routing metadata (`toron.enable`, `toron.host`, `toron.prefix`, `toron.port`, `toron.weight`, `toron.health_check`).
- **Real-Time Lifecycle Event Streaming**: Background worker streams container `start`, `die`, and `stop` events and dynamically inserts/removes upstreams from `router.Router` with zero downtime.

### Related Tasks
- `TASK-046`: Implement Vendor-Agnostic OCI Container Auto-Discovery Engine

## 2026-08-16 - Toron v1.0.0 Official Release (Feature Freeze Milestone)

### Milestone Summary
- **Official Version 1.0.0 Freeze**: All feature sets spanning Prototypes 1 through 35 are officially frozen for the stable **v1.0.0** release.
- **Production Scope**: Core event reactor engine, HTTP/1.1, HTTP/2 (`h2c` / TLS), HTTP/3 QUIC, gRPC gateway & trailers, ACME zero-touch SSL (HTTP-01 & ALPN-01), per-host SNI & mTLS, Zstd/Brotli compression, RFC 7234 response caching, multi-scheme authentication, Web Application Firewall (WAF) with OWASP & custom regex rules and CIDR IP ACLs, zero-downtime hot reloading, Prometheus metrics, and read-only Security Control Center Dashboard are finalized.

## 2026-08-15 - Prototype 35 Release (Custom WAF Regex Rules & Zero-Downtime Hot Reloading)

- **Web Control Center Security & WAF Dashboard (`public/`, `pkg/server`)**: Added dedicated Security & WAF Dashboard tab to the Web Control Center (`/internal/dashboard/`) featuring hero threat indicators, visual Security & Compliance Policy Matrix, real-time Security Audit Incidents log feed table, and WAF attack presets in the Live API Tester.
- **Security Incidents Feed API (`GET /internal/api/security/incidents`)**: Registered management endpoint serving recorded security events from an in-memory thread-safe ring buffer in `AuditLogger`.
- **User-Defined Custom WAF Regex Rules (`pkg/waf`)**: Supported defining custom security regex rules in `config.yaml` (`server.waf.custom_rules`) and `routes.yaml` (`pr.waf.custom_rules`) with customizable `id`, `category`, `description`, `pattern`, `score`, and granular inspection locations (`url`, `path`, `query`, `headers`, `body`) (`TASK-045`, `REQ-045`).
- **Zero-Downtime Hot Reloading via `ConfigWatcher` (`pkg/config/watcher.go`)**: Background `fsnotify` file worker monitors `config.yaml` with debouncing, automatically re-validates configuration syntax, and atomically reloads active WAF rule sets in memory via `WAFEngine.Reload()` without dropping active TCP/TLS/HTTP connections.
- **Fail-Safe Hot Reload Protection**: Invalid configuration edits or unparseable regular expressions are safely rejected with descriptive warning logs while retaining the active in-memory rule set intact.

### Related Tasks
- `TASK-045`: Implement Custom WAF Regex Rules and Zero-Downtime Hot Reloading

## 2026-08-15 - Prototype 34 Release (WAF Telemetry, Prometheus Metrics & Structured Security Audit Logging)

### Added
- **Prometheus WAF Metrics (`pkg/metrics`)**: Added specialized WAF telemetry counters and latency histograms exposed via `/metrics` in standard Prometheus text format (`TASK-044`, `REQ-044`):
  - `toron_waf_blocked_requests_total{category="...",route="..."}`: Total blocked attacks categorized by threat vector and route.
  - `toron_waf_anomalies_detected_total{category="...",mode="detection"}`: Total anomalies observed in detection mode.
  - `toron_waf_inspection_duration_seconds`: High-resolution latency histogram tracking WAF inspection processing times.
  - Telemetry integrated into JSON metrics summary API (`/internal/api/metrics`).
- **Structured JSON Security Audit Logger (`pkg/waf/audit.go`)**: Implemented thread-safe `AuditLogger` writing SIEM-ready JSON log lines (client IP, rule ID, threat score, method, path, location, and bounded payload snippet) to `stdout`, `stderr`, or dedicated log files.
- **WAF Middleware Telemetry Hooks (`pkg/waf/middleware.go`)**: Automatically records inspection duration, increments Prometheus counters, and emits structured security audit logs for IP ACL blocks, protocol violations, OWASP threat blocks, and detection anomalies.

### Related Tasks
- `TASK-044`: Implement WAF Telemetry, Prometheus Metrics, and Structured Security Audit Logging

## 2026-08-15 - Prototype 33 Release (Route-Level WAF Overrides & CIDR IP Access Control Lists)

### Added
- **CIDR-Based IP Access Control Lists (ACLs)**: Implemented fast-path $O(1)$ IP allow/deny list filtering in `pkg/waf/ip_acl.go` supporting IPv4/IPv6 CIDR subnets (`allowed_ips`, `denied_ips`) and single IP addresses with automatic port stripping and proxy header (`X-Forwarded-For`, `X-Real-IP`) parsing (`TASK-043`, `REQ-043`).
- **Per-Route WAF Configuration & Overrides**: Extended `routes.yaml` and `pkg/router/router.go` (`RoutePrefix`) to allow routes to independently configure dedicated `WAFEngine` instances with custom `mode`, `anomaly_threshold`, `disabled_rules`, and IP access lists.
- **Selective OWASP Rule Tuning**: Enabled disabling individual threat rules (e.g. `SQLI-001`) on legacy backends without weakening gateway-wide security.

### Related Tasks
- `TASK-043`: Implement Route-Level WAF Overrides and CIDR IP Access Lists

## 2026-08-15 - Prototype 32 Release (Protocol Integrity & HTTP Request Smuggling Guard)

### Added
- **HTTP Request Smuggling Prevention (CL.TE / TE.CL)**: Implemented dual-layer detection in `pkg/httpparser/parser.go` and `pkg/waf/protocol.go` rejecting requests with conflicting `Content-Length` and `Transfer-Encoding` headers or mismatched multiple `Content-Length` header values (`TASK-042`, `REQ-042`).
- **Malformed Request Control Character Guard**: Added non-printable control character filtering (`0x00–0x1F` except `\t`, `0x7F DEL`) in URL paths, query strings, and header keys/values returning `400 Bad Request`.
- **Payload Size Bounding**: Added strict boundary limits on single header values (`max_header_value_bytes`, 4 KB), query strings (`max_query_size`, 4 KB), and parameters (`max_param_size`, 2 KB) returning `413 Payload Too Large`.

### Related Tasks
- `TASK-042`: Implement Protocol Integrity and Request Smuggling Guard

## 2026-08-15 - Prototype 31 Release (Core Web Application Firewall Engine & OWASP Injection Protection)

### Added
- **Core WAF Engine (`pkg/waf`)**: Implemented high-performance, modular Web Application Firewall engine (`WAFEngine`) in `pkg/waf/waf.go` with `enforce` (HTTP `403 Forbidden` blocking) and `detection` (log-only threat anomaly score) modes (`TASK-041`, `REQ-041`).
- **OWASP Top 10 Injection Protection**: Pre-compiled regex rule set in `pkg/waf/rules.go` targeting SQL Injection (`SQLI-001`, `SQLI-002`, `SQLI-003`), Cross-Site Scripting (`XSS-001`, `XSS-002`, `XSS-003`), Path Traversal / LFI (`TRAVERSAL-001`, `TRAVERSAL-002`), and Command Injection / RCE (`RCE-001`, `RCE-002`).
- **Multi-Location Request Inspection**: Inspects URL paths, raw query parameters, HTTP request headers, and payload bodies (bounded by `max_inspect_body_size`, restoring `req.Body` for downstream handlers).
- **Router Middleware Adapter**: Created `NewWAFMiddleware` in `pkg/waf/middleware.go` and integrated into `cmd/toron/main.go`, `pkg/config/config.go`, `config.yaml`, and `routes.yaml`.

### Related Tasks
- `TASK-041`: Implement Core Web Application Firewall Engine and OWASP Injection Protection Middleware

## 2026-08-14 - Prototype 30 Release (Configurable CORS Policies & Enterprise Security Headers)

### Added
- **CORS Middleware**: Implemented `CORSMiddleware` in `pkg/router/cors.go` with fast-path `OPTIONS` preflight `204 No Content` handling, origin wildcard/subdomain matching, credential policies, and exposed headers (`TASK-040`, `REQ-040`).
- **Enterprise Security Headers Middleware**: Added `SecurityHeadersMiddleware` in `pkg/router/security_headers.go` injecting `Strict-Transport-Security` (HSTS), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `Content-Security-Policy`, and `Permissions-Policy`.
- **Global & Route Configuration**: Supported configuring CORS and Security Headers in `config.yaml` (`server.cors`, `server.security_headers`) and per-route in `routes.yaml`.

### Related Tasks
- `TASK-040`: Implement Configurable CORS Policies and Enterprise Security Headers Middleware

## 2026-08-14 - Prototype 29 Release (Per-Host Dynamic SNI Certificate Mapping & mTLS Client Auth)

### Added
- **Dynamic SNI Multi-Certificate Registry**: Added `SNIRegistry` in `pkg/server/sni.go` with `tls.Config.GetConfigForClient` hook, dynamically mapping hostnames to dedicated X.509 certificate pairs (`TASK-039`, `REQ-039`).
- **Mutual TLS (mTLS) Client Verification**: Added per-host client certificate verification with configurable policies (`require_and_verify`, `verify_client_cert_if_given`, `request_client_cert`) and custom client CA pools (`ca_file`).
- **Per-Host Minimum TLS Version**: Supported enforcing `tls.min_version` (`tls1.2`, `tls1.3`) per route in `routes.yaml`.

### Related Tasks
- `TASK-039`: Implement Per-Host SNI Dynamic Certificate Dispatching and mTLS Client Auth

## 2026-08-14 - Prototype 28 Release (Native gRPC Health Checking Protocol & HTTP/2 Trailers Gateway)

### Added
- **Native `grpc.health.v1.Health` Prober**: Added binary Protobuf frame encoder/decoder in `pkg/proxy/grpc_health.go` supporting active `grpc.health.v1.Health/Check` background health probing over HTTP/2 (`TASK-038`, `REQ-038`).
- **HTTP/2 Trailers Gateway Preservation**: Forwarded upstream trailing headers (`grpc-status`, `grpc-message`, `grpc-status-details-bin`) through the reverse proxy to downstream clients.
- **gRPC Route Configuration**: Added `health_check_type: "grpc"` and `health_check_service: "<name>"` options to `routes.yaml` and `ProxyRouteConfig`.

### Related Tasks
- `TASK-038`: Implement Native gRPC Health Checking Prober and HTTP/2 Trailers Preservation

## 2026-08-14 - Prototype 27 Release (Next-Gen Response Compression: Brotli & Zstandard)

### Added
- **Brotli & Zstandard Compression**: Extended `CompressionMiddleware` in `pkg/router/compression.go` to support Brotli (`br`, RFC 7932) and Zstandard (`zstd`, RFC 8878) (`TASK-037`, `REQ-037`).
- **Encoder Object Pooling**: Implemented `sync.Pool` allocation recycling for `brotli.Writer` and `zstd.Encoder`.
- **Quality Factor Content Negotiation**: Added RFC 7231 quality factor weighting (`q=`) parsing and modern server ranking (`zstd` > `br` > `gzip` > `deflate`).

### Related Tasks
- `TASK-037`: Implement Brotli (`br`) and Zstandard (`zstd`) Response Compression with Encoders Pooling

## 2026-08-14 - Prototype 26 Release (Multi-Scheme Authentication: JWT, API Key, and Basic Auth)

### Added
- **Multi-Scheme Auth Middleware**: Added `AuthMiddleware` in `pkg/router/auth.go` supporting RFC 7519 JWT verification (HS256/HS384/HS512), API key authentication, and RFC 7617 HTTP Basic authentication (`TASK-036`, `REQ-036`).
- **Timing Attack Resistance**: Used `crypto/subtle.ConstantTimeCompare` across all signature and credential comparisons.
- **Route & Global Integration**: Supported configuring authentication per route rule in `routes.yaml` or globally in `config.yaml`, injecting `X-Authenticated-User` headers into upstream requests.

### Related Tasks
- `TASK-036`: Implement Multi-Scheme Authentication Middleware (JWT HS256, API Key, and HTTP Basic Auth)

## 2026-08-14 - Prototype 25 Release (In-Memory HTTP Response Caching & Cache-Control)

### Added
- **In-Memory Response Caching**: Added `ResponseCache` and `NewCacheMiddleware` in `pkg/router/cache.go` providing thread-safe in-memory caching for idempotent GET and HEAD requests (`TASK-035`, `REQ-035`).
- **RFC 7234 Cache-Control Engine**: Parsed `max-age`, `no-store`, `no-cache`, `private`, and `public` directives; supported client refresh bypasses (`Cache-Control: no-cache`).
- **Diagnostics & Age Headers**: Injected `X-Cache: HIT` / `X-Cache: MISS` telemetry indicators and calculated `Age: <seconds>` headers.
- **Memory Bounding & Eviction**: Added memory bounds via `max_entries` and `max_payload_size` in `config.yaml` (`server.cache`).

### Related Tasks
- `TASK-035`: Implement In-Memory Response Caching Engine, Cache-Control Parser, and Diagnostics Headers

## 2026-08-14 - Prototype 24 Release (Streaming Response Compression: Gzip & Deflate)

### Added
- **Transparent Response Compression**: Added `CompressionMiddleware` in `pkg/router/compression.go` using Go standard library `compress/gzip` and `compress/flate` (`TASK-034`, `REQ-034`).
- **sync.Pool Writer Allocation Reuse**: Implemented object pooling for gzip and flate writers to achieve zero-allocation buffer reuse during high-concurrency requests.
- **Config & Protocol Safety**: Added `server.compression` settings to `config.yaml` (`enabled`, `min_length`, `level`, `encodings`, `types`) and safeguarded WebSocket 101 upgrades and binary media types against compression.

### Related Tasks
- `TASK-034`: Implement Transparent HTTP Response Compression Middleware (Gzip & Deflate)

## 2026-08-14 - Prototype 23 Release (ACME Zero-Touch Production SSL & TLS-ALPN-01)

### Added
- **ACME Engine**: Added `ACMEManager` in `pkg/acme/acme.go` for zero-touch SSL certificate issuance and background renewal (`TASK-033`, `REQ-033`).
- **HTTP-01 & TLS-ALPN-01 Responders**: Implemented HTTP-01 token authorization responder (`/.well-known/acme-challenge/*`) and TLS-ALPN-01 responder (`acme-tls/1` ALPN negotiation with OID `1.3.6.1.5.5.7.1.31`).
- **Disk Caching & Key Security**: Implemented secure disk certificate and key caching in `cache_dir` with `0600`/`0700` POSIX permissions.

### Related Tasks
- `TASK-033`: Implement ACME Engine, HTTP-01/TLS-ALPN-01 Responders, and Certificate Caching

## 2026-08-13 - Prototype 22 Release (Telemetry Metrics in Control Plane JSON Endpoints)

### Added
- **JSON Telemetry Integration**: Added `GetSummaryJSON()` in `pkg/metrics/metrics.go` exporting total requests, active QUIC streams, active TCP connections, circuit breaker trips, and status/method breakdowns (`TASK-032`, `REQ-032`).
- **Control Plane API**: Updated `GET /internal/api/status` and registered `GET /internal/api/metrics` returning structured JSON metrics.

### Related Tasks
- `TASK-032`: Expose Comprehensive Metrics in /internal/api/ Control Plane Endpoints

## 2026-08-13 - Prototype 21 Release (Prometheus Metrics & W3C Traceparent Propagation)

### Added
- **Prometheus Metrics Exporter**: Implemented `/metrics` endpoint returning Prometheus exposition format (`text/plain; version=0.0.4`) with request counters, latency histograms, and QUIC stream gauges (`TASK-031`, `REQ-031`).
- **W3C Distributed Tracing**: Added W3C `traceparent` context header extraction, generation, and upstream propagation in `pkg/metrics/tracing.go` and `pkg/proxy/proxy.go`.

### Related Tasks
- `TASK-031`: Implement Prometheus Metrics Registry and W3C Traceparent Header Propagation

## 2026-08-13 - Prototype 20 Release (Sticky Session Load Balancing: sticky_cookie & ip_hash)

### Added
- **Session Affinity Balancers**: Added `StickyCookieBalancer` (cookie-based session affinity with `Set-Cookie` injection) and `IPHashBalancer` (client IP hash affinity) in `pkg/proxy/sticky.go` (`TASK-030`, `REQ-030`).
- **Config & Router Integration**: Supported `algorithm: "sticky_cookie"` and `algorithm: "ip_hash"` in `routes.yaml` and `pkg/proxy/proxy.go`.

### Related Tasks
- `TASK-030`: Implement Sticky Session Load Balancing (sticky_cookie and ip_hash)

## 2026-08-13 - Prototype 19 Release (Token Bucket Rate Limiting Middleware)

### Added
- **Token Bucket Rate Limiter**: Added `TokenBucket`, `RateLimiter`, and `NewRateLimitMiddleware` in `pkg/router/rate_limiter.go` for route-level DDoS protection (`TASK-029`, `REQ-029`).
- **Client Key Extraction**: Extracted client identity via `X-API-Key`, `Authorization`, `X-Forwarded-For`, or remote IP with `429 Too Many Requests` and `Retry-After` header returns.

### Related Tasks
- `TASK-029`: Implement Token Bucket Rate Limiting Middleware per Client IP and API Key

## 2026-08-13 - Prototype 18 Release (Dynamic Route Hot Reloading via fsnotify)

### Added
- **Route Hot Reloading**: Integrated `github.com/fsnotify/fsnotify` in `pkg/config/watcher.go` (`RouteWatcher`) to automatically watch `routes.yaml` edits and update routing tables dynamically without dropping socket connections (`TASK-028`, `REQ-028`).
- **Atomic Router Reset**: Added `Router.Reset()` in `pkg/router/router.go` for zero-downtime routing table reloading under write lock.

### Related Tasks
- `TASK-028`: Implement Hot Reloading of Routes via File-Watch Worker (fsnotify)

## 2026-08-13 - Prototype 17 Release (HTTP/3 Protocol Engine & QUIC Transport)

### Added
- **HTTP/3 QUIC Transport**: Integrated `github.com/quic-go/quic-go/http3` engine into `pkg/server/server.go` (`TASK-027`, `REQ-027`).
- **Alt-Svc Protocol Advertising**: Injected `Alt-Svc: h3=":8443"` headers on HTTP/1.1 and HTTP/2 response headers for automatic browser HTTP/3 upgrades.

### Related Tasks
- `TASK-027`: Implement HTTP/3 Protocol Engine and QUIC Transport Handler

## 2026-08-13 - Prototype 16 Release (Layer 4 TCP & UDP Transport Proxying)

### Added
- **L4 TCP Socket Proxy**: Added `TCPProxy` in `pkg/proxy/tcp.go` for raw socket stream forwarding and round-robin load balancing (`TASK-026`, `REQ-026`).
- **L4 UDP Datagram Proxy**: Added `UDPProxy` in `pkg/proxy/udp.go` for connectionless datagram packet proxying.

### Related Tasks
- `TASK-026`: Implement Layer 4 TCP and UDP Transport Proxying

## 2026-08-12 - Prototype 15 Release (HTTPS TLS Encryption & Dev Certificate Generator)

### Added

- **HTTPS TLS Listener**: Added `ListenAndServeTLS` and `CreateTLSConfig` in `pkg/server/tls.go` and `pkg/server/server.go` (`TASK-023`, `REQ-023`).
- **ALPN HTTP/2 Negotiation**: Integrated TLS ALPN protocol negotiation (`h2`, `http/1.1`) mapping encrypted HTTP/2 streams to the server core.
- **Auto Self-Signed Dev Certs**: Implemented `GenerateDevCert()` providing zero-configuration ECDSA P-256 self-signed certificates for `localhost` development.
- **HTTPS Unit Test Suite**: Added `TestServer_HTTPSSelfSigned` and `TestServer_HTTPSWithALPNHTTP2` in `pkg/server/tls_test.go`.

### Related Tasks

- `TASK-023`: Implement HTTPS TLS Encryption, ALPN Negotiation, and Dev Cert Generator

## 2026-08-12 - Prototype 14 Release (HTTP/2 Protocol Support & Stream Multiplexing)

### Added

- **HTTP/2 Engine Integration**: Integrated `golang.org/x/net/http2` server engine into `pkg/server/server.go` (`TASK-022`, `REQ-022`).
- **Connection Preface Auto-Detection**: Added connection preface detection (`PRI * HTTP/2.0...`) in `handleConn`, enabling zero-downtime hybrid HTTP/1.1 and HTTP/2 cleartext (`h2c`) stream handling.
- **HTTP/2 Configuration**: Added `http2` settings (`enabled`, `max_concurrent_streams`, `max_frame_size`, `allow_h2c`) to `config.yaml` and `pkg/config`.
- **HTTP/2 Integration Tests**: Added `TestServer_HTTP2PriorKnowledge` unit test in `pkg/server/http2_test.go`.

### Related Tasks

- `TASK-022`: Implement HTTP/2 Server Connection Handler and Configuration Options

## 2026-08-12 - Prototype 13 Release (Configuration Syntax Testing & Dry-Run CLI Flag)

### Added

- **Configuration Dry-Run Flag**: Added `-test-config` (`-t`) CLI option in `cmd/toron/main.go` to test YAML syntax and configuration parameters before starting the server (`TASK-021`, `REQ-021`).
- **Strict Validator**: Implemented `ValidateConfig(cfg *AppConfig) error` in `pkg/config/loader.go` checking port ranges, static file directory accessibility, load balancing algorithms, and target URL schemes (`http://` / `https://`).
- **CLI Reference**: Updated CLI reference guide (`docs/wiki/reference/cli.md`).

### Related Tasks

- `TASK-021`: Implement Configuration Test Option (-test-config / -t) and Strict Validator

## 2026-08-12 - Prototype 12 Release (Domain-Based Virtual Host Routing)

### Added

- **Domain-Based HTTP Routing**: Added Virtual Host matching support in `pkg/router/router.go` (`r.GETHost`, `r.HandleHostHeader`, `r.ProxyWithOptions` with host matching) (`TASK-020`, `REQ-020`).
- **YAML Schema Extension**: Supported `host` and `domain` parameters under proxy route configurations in `routes.yaml`.
- **Host Header Extraction & Sanitization**: Implemented `extractHost(req)` stripping optional port numbers and matching exact subdomains.
- **HTTP REST Test Suite**: Added domain-based Host header request examples in `test_endpoint.http`.

### Related Tasks

- `TASK-020`: Implement Domain-Based HTTP Routing and Proxy Forwarding

## 2026-08-12 - Prototype 11 Release (Decoupled Dual-File YAML Configuration)

### Added

- **Dual-File Configuration Architecture**: Split application configuration into `config.yaml` (server infrastructure, static assets, logging) and `routes.yaml` (reverse proxy routing rules, load balancers, health checks) (`TASK-019`, `REQ-019`).
- **Loader Enhancement**: Added `LoadFromFiles(configPath, routesPath)` in `pkg/config` with auto-discovery of `routes.yaml`.
- **CLI Flag Integration**: Added `-routes` (`-r`) CLI flag alongside `-config` (`-c`) in `cmd/toron/main.go`.

### Related Tasks

- `TASK-019`: Implement Dual-File YAML Config Loader (config.yaml & routes.yaml)

## 2026-08-12 - Prototype 10 Release (Configurable Static Prefix & Relative Asset Resolution)

### Added

- **Relative Asset Resolution**: Updated `public/index.html` to reference `./style.css` and `./app.js` using relative URL paths (`TASK-018`, `REQ-018`).
- **Subpath Prefix Trailing Slash Redirect**: Updated `pkg/router/router.go` static file handler to automatically emit a `302 Found` redirect when a request matches a configured static prefix (e.g. `/internal/dashboard`) without trailing slash, establishing correct browser base URL resolution.
- **Config Integration**: Tested and verified static prefix configuration (`prefix: "/internal/dashboard"`) in `config.yaml`.

### Related Tasks

- `TASK-018`: Implement Configurable Static Prefix Trailing Slash Redirect and Relative Asset Loading

## 2026-08-12 - Prototype 9 Release (Internal Management API Endpoints)

### Added

- **Internal Management API Namespace**: Added `/internal/api/` control plane endpoints (`status`, `routes`, `upstreams/health`, `proxy-test`) in `pkg/server/internal_api.go` (`TASK-017`, `REQ-017`).
- **Server-Side Upstream Probing**: Backend routines perform concurrent HTTP health checks against upstreams, returning node health states to the dashboard without requiring browser-to-upstream target calls.
- **Internal Proxy Test Dispatcher**: `POST /internal/api/proxy-test` accepts path/method/header payloads, executes proxy routes server-side, and returns status codes, latency, headers, and body payloads.
- **Frontend Integration**: Updated `public/app.js` to query `/internal/api/` endpoints exclusively.

### Related Tasks

- `TASK-017`: Implement /internal/api/ Management Endpoints and Update Frontend Client

## 2026-08-12 - Prototype 8 Release (Mobile-First Control Center & Dashboard)

### Added

- **Mobile-First Control Center & Proxy Dashboard**: Transformed `/public` frontend into a responsive Web Control Center and Proxy Dashboard built with HTML5, Vanilla JavaScript, and Tailwind CSS (`TASK-016`, `REQ-016`).
- **Real-Time Upstream Health Probing**: Dynamic client-side health check execution querying upstream service ports 9001-9010 and accurately flagging status 200 (Healthy `CLOSED`), status 500 (`OPEN (500 ERR)`), or connection failure (`UNREACHABLE`).
- **Interactive Live API & Proxy Route Tester**: Interactive request composer with header presets, custom header inputs, execution time calculation (in ms), status badges, and formatted JSON response preview.
- **CORS & Error Simulation in Dummy Services**: Added CORS headers (`Access-Control-Allow-Origin: *`) and 500 error route handling (`/500`, `/error`, `?fail=true`) in `dummy-services/services.go`.

### Related Tasks

- `TASK-016`: Implement Mobile-First Control Center and Proxy Dashboard UI

## 2026-08-12 - Prototype 7 Release (Load Balancing)

### Added

- **Upstream Reverse Proxy Load Balancing**: Multi-target load balancing with pluggable `LoadBalancer` interface, thread-safe Round-Robin selection algorithm (`round_robin`), YAML `targets` & `algorithm` configuration, and `r.ProxyBalancer` router helpers (`TASK-011`, `REQ-011`).
- **Dummy Web Services Test Suite**: 10 dummy HTTP web services running on ports 9001–9010 (`dummy-services/`) for testing reverse proxy routing and load balancer upstream targets (`TASK-012`, `REQ-012`).
- **Upstream Health Check & Circuit Breaker**: Active HTTP health check probing (`health_check_path`), 3-state Circuit Breaker (`Closed`, `Open`, `HalfOpen`), automatic offline target filtering, and recovery cooldown management (`TASK-014`, `REQ-014`).
- **Full Proxy Route Integration Configuration**: Updated `config.yaml` with active proxy routing (`proxy.enabled: true`) mapping all path, header, single-target, and load-balanced routes to dummy services 9001–9010 (`TASK-013`, `REQ-013`).
- **Standardized REST Client Test File**: Created `test_endpoint.http` in root directory for one-click HTTP request execution across all native, static, header, path, and proxy endpoints (`TASK-015`, `REQ-015`).

### Related Tasks

- `TASK-011`: Reverse Proxy Load Balancer Implementation
- `TASK-012`: Add 10 Dummy Web Services for Upstream Testing
- `TASK-013`: Update config.yaml to Map Proxy Routes to Dummy Web Services
- `TASK-014`: Implement Upstream Health Check and Circuit Breaker
- `TASK-015`: Create test_endpoint.http REST Client Test File

## 2026-08-11 - Prototype 1, 2, 3, 4, 5 & 6 Release

### Added

- **Event Reactor Core Engine**: High-performance non-blocking TCP socket listener and connection event loop with worker pool dispatching (`TASK-001`, `REQ-001`).
- **HTTP/1.1 Protocol Parser**: Zero-copy streaming request parsing and formatted response serialization (`TASK-002`, `REQ-002`).
- **HTTP Router & Middleware**: URL routing, HTTP method dispatching, and middleware chain support (`TASK-003`, `REQ-003`).
- **Security Guards**: Request header size limits (8KB), body size limits (4MB), socket read/write timeouts, and panic recovery middleware (`TASK-004`, `REQ-005`).
- **Server Orchestration**: Command line entry point `cmd/toron/main.go` with `/health` and `/` endpoints and graceful shutdown handling (`TASK-005`, `REQ-001`).
- **Static File Serving**: Built-in static website hosting with MIME type detection, `index.html` resolution, and path traversal security guards (`TASK-006`, `REQ-006`).
- **Extensible Configuration System**: External configuration file support (`config.yaml`), CLI argument `-config` flag, and extensible `pkg/config` loader architecture (`TASK-007`, `REQ-007`).
- **Native Go Benchmarking Suite**: Standard Go `testing.B` benchmarks across `pkg/reactor`, `pkg/httpparser`, `pkg/router`, and `pkg/server` with memory allocation tracking (`TASK-008`, `REQ-008`).
- **Reverse Proxy & Upstream Gateway**: Built-in HTTP reverse proxy engine (`pkg/proxy`, `router.Proxy(prefix, target)`), forwarding requests, injecting `X-Forwarded-*` headers, and supporting YAML proxy routes (`TASK-009`, `REQ-009`).
- **Header-Based HTTP Routing**: Conditional route matching and proxy forwarding based on HTTP header key/value conditions (`r.GETHeader`, `r.ProxyHeader`, `proxy.routes[].headers`), supporting API versioning (`X-Version: v2`) and canary routing (`TASK-010`, `REQ-010`).

### Related Tasks

- `TASK-001`: Core Event Reactor Engine Implementation
- `TASK-002`: HTTP/1.1 Streaming Request Parser & Response Builder
- `TASK-003`: HTTP Router & Middleware Pipeline Implementation
- `TASK-004`: Security Guards, Request Limits & Connection Timeouts
- `TASK-005`: Toron Server Orchestration & Main Application Entry
- `TASK-006`: Static File Handler & Path Traversal Guard Implementation
- `TASK-007`: Extensible Config Loader Package & CLI Flag Integration
- `TASK-008`: Benchmark Suite Implementation for Core Server Packages
- `TASK-009`: Reverse Proxy Handler & Upstream Transport Implementation
- `TASK-010`: Header-Based Route Matching & Config Integration
