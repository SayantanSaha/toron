# Release Notes

## 2026-09-09 - Toron v1.5.8 Security Release (SEC-27: Maximum Idle Deadline Enforcement on Upgraded Protocol and WebSocket Connections)

### Milestone Summary
- **Remediation of Security Vulnerability SEC-27 (`pkg/server`, `pkg/config`)**: Resolved critical Slowloris resource exhaustion vulnerability [`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392) ([CWE-400: Uncontrolled Resource Consumption](https://cwe.mitre.org/data/definitions/400.html), [`SR-081 Finding 5`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L142-L150), [`SR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-087.md)) in core HTTP/1.1 protocol upgrade handling and HTTP/2 Extended CONNECT streams, eliminating permanent socket and goroutine leaks.
- **Configurable Upgraded Inactivity Deadline (`UpgradeIdleTimeout`)**: Added `UpgradeIdleTimeout time.Duration` to [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go) and [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) (`yaml:"upgrade_idle_timeout,omitempty" json:"upgrade_idle_timeout,omitempty"`), defaulting to `60s` with a 3-tier fallback hierarchy ([`TASK-097`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-097.md), [`REQ-088`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-088.md), [`ADR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-083.md)).
- **Bidirectional Activity-Refreshed Deadline Relay (`pkg/server/server.go`)**: Replaced permanent deadline stripping (`SetDeadline(time.Time{})`) and unbounded `io.Copy` with an active deadline relay (`relayUpgradedStreams`). Every transferred chunk refreshes read and write deadlines; inactivity exceeding `UpgradeIdleTimeout` terminates both sockets deterministically via `sync.Once` and unblocks relay goroutines ([`TASK-098`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-098.md)).
- **HTTP/2 Extended CONNECT Protection (`pkg/server/server.go`)**: Enforced `UpgradeIdleTimeout` on RFC 8441 extended CONNECT streams, with immediate socket teardown upon client request context cancellation (`r.Context().Done()` / `RST_STREAM`) ([`TASK-099`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-099.md)).
- **Transport-Layer Heartbeat Transparency**: RFC 6455 WebSocket Ping/Pong control frames and application keep-alive messages continuously refresh the deadline, preserving legitimate persistent sessions indefinitely.
- **TCP Half-Close Propagation**: Supported `CloseWrite()` upon reading `io.EOF`, allowing reverse responses to drain while maintaining deadline enforcement.
- **Zero External Dependencies**: Implemented strictly with Go standard library packages (`net`, `sync`, `sync/atomic`, `time`, `io`, `log`, `errors`).
- **Automated Verification Suite (`pkg/server/server_test.go`)**: Tested idle timeout socket teardown, heartbeat keep-alive survival, peer disconnect cleanup, HTTP/2 extended CONNECT, and high concurrency under `go test -race` ([`TASK-100`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-100.md), [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md)).

### Added
- **Configuration Fields**: `UpgradeIdleTimeout time.Duration` in [`server.Config`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/config.go) and [`ServerConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go).
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
- [`TASK-097`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-097.md): Configurable Upgraded Inactivity Deadline Schema
- [`TASK-098`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-098.md): Bidirectional Activity-Refreshed Deadline Relay for HTTP/1.1 Upgrades
- [`TASK-099`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-099.md): Idle Deadline Enforcement for HTTP/2 Extended CONNECT Upgraded Streams
- [`TASK-100`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-100.md): Automated Verification Test Suite for Upgraded Connection Idle Deadlines
- [`REQ-088`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-088.md): Maximum Idle Deadline Enforcement on Upgraded Protocol and WebSocket Connections
- [`ADR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-083.md): Bidirectional Activity-Refreshed Deadlines for Upgraded Protocol Sockets
- [`TC-088`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-088.md): Upgraded Connection Idle Deadline & Resource Reclamation Test Suite
- [`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392): Missing Maximum Idle Deadlines on Upgraded Protocol Connections
- [`SR-087`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-087.md): Security Review of SEC-27 Remediation
- [`CR-084`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-084.md): Code Review of Upgraded Protocol Connection Idle Deadline Enforcement

## 2026-09-09 - Toron v1.5.7 Security Release (SEC-26: Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies)

### Milestone Summary
- **Remediation of Critical Vulnerability SEC-26 (`pkg/proxy`, `pkg/config`, `cmd/toron`)**: Successfully resolved critical vulnerability [`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383) ([CWE-400: Uncontrolled Resource Consumption](https://cwe.mitre.org/data/definitions/400.html), [`SR-081 Finding 4`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-081.md#L130-L140), [`SR-086`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-086.md)) in Layer 4 transport proxies ([`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L64) and [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L92)), eliminating memory exhaustion (OOM), host ephemeral port starvation (`bind: address already in use`), socket file descriptor leaks (`EMFILE`), and Slowloris denial of service.
- **Declarative Configuration Schema (`pkg/config`)**: Extended [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) with `MaxConnections` (`max_connections`, default `10000`), `IdleTimeout` (`idle_timeout`, default `60s`), and `MaxWorkers` (`max_workers`, default `1024`), backed by safe fallback getter methods ([`GetMaxConnections`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L461), [`GetIdleTimeout`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L469), [`GetMaxWorkers`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L477)) ([`TASK-093`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-093.md), [`REQ-087`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-087.md), [`ADR-082`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-082.md)).
- **TCP Concurrency Limits & Fast-Fail Rejection (`pkg/proxy/tcp.go`)**: Enforced atomic active connection tracking. Incoming connections exceeding `MaxConnections` are closed immediately upon `Accept()` without dialing upstream backends, allocating heap memory, or spawning relay goroutines ([`TASK-094`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-094.md)).
- **Bidirectional TCP Idle Deadlines & Slowloris Protection (`pkg/proxy/tcp.go`)**: Replaced unbounded blocking `io.Copy` stream relays with deadline-aware transfer loops. Read and write deadlines are updated on active data transfer; connections with zero throughput for `IdleTimeout` are terminated immediately, freeing socket file descriptors and unblocking worker goroutines. Cleanly supports TCP half-close (`CloseWrite`).
- **UDP Bounded Worker Pool & Saturated Queue Dropping (`pkg/proxy/udp.go`)**: Eliminated unconstrained per-packet goroutine spawning (`go p.handleDatagram(...)`) by implementing a fixed-capacity task channel (`packetQueue`) of size `MaxWorkers` serviced by long-lived worker goroutines. Saturated queues drop excess datagrams fail-safe without memory growth or panics ([`TASK-095`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-095.md)).
- **UDP Client Session Registry & Upstream Socket Reuse (`pkg/proxy/udp.go`)**: Implemented a thread-safe session registry (`sessions map[netip.AddrPort]*udpSession`). Subsequent datagrams from the same client reuse the open outbound `*net.UDPConn`, completely eliminating per-packet socket dials and ephemeral port exhaustion. A background sweeper routine evicts idle sessions after `IdleTimeout` of inactivity.
- **Zero-Allocation Buffer Recycling with `sync.Pool` (`pkg/proxy/udp.go`)**: Datagram buffers (64 KB / 65,535 bytes) are recycled across inbound reads and upstream responses, eliminating per-packet heap allocations and garbage collection pauses.
- **Deterministic Graceful Teardown (`pkg/proxy`)**: Calling `Close()` on either proxy immediately closes listeners, active client sockets, upstream connections, and worker pools within $\le 500\text{ms}$.
- **Zero External Dependencies**: Pure Go standard library implementation (`net`, `net/netip`, `sync`, `sync/atomic`, `time`, `io`).
- **Microbenchmark Verification (`pkg/proxy/tcp_test.go`, `pkg/proxy/udp_test.go`)**: Confirmed ~27,000 ops/sec (0 allocs/op steady-state) for TCP streaming and ~21,700 pkts/sec (0 buffer allocs) for UDP datagram forwarding ([`TASK-096`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-096.md), [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md)).

### Added
- **Configuration Fields**: Added `MaxConnections int`, `IdleTimeout time.Duration`, and `MaxWorkers int` to [`ProxyRouteConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L253) in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go).
- **Helper Getters**: [`GetMaxConnections()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L461) (defaults to 10,000), [`GetIdleTimeout()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L469) (defaults to 60s), and [`GetMaxWorkers()`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L477) (defaults to 1,024).
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
- [`TASK-093`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-093.md): Layer 4 Proxy Configuration Schema & Wiring
- [`TASK-094`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-094.md): Bounded Concurrency, Bidirectional Idle Deadlines & Connection Tracking in TCPProxy
- [`TASK-095`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-095.md): Bounded Worker Pool, sync.Pool Buffer Recycling & Session Socket Reuse in UDPProxy
- [`TASK-096`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-096.md): Automated Verification Suite for Layer 4 TCP & UDP Proxy Hardening
- [`REQ-087`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-087.md): Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies
- [`ADR-082`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-082.md): Bounded Concurrency, Socket Reuse, and Idle Deadline Enforcement in Layer 4 TCP and UDP Proxies
- [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md): Layer 4 Concurrency, Socket Reuse, and Idle Deadline Test Suite
- [`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383): Uncontrolled Resource Consumption in Layer 4 TCP and UDP Proxies
- [`SR-086`](file:///Users/sneha/Developer/toron-research/toron/docs/securityReview/SR-086.md): Security Review of SEC-26 Remediation
- [`CR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/codeReview/CR-083.md): Code Review of Layer 4 TCP and UDP Proxy Hardening

## 2026-09-08 - Toron v1.5.6 Security Release (SEC-25: Configurable Request Body Limits & HTTP 413 Rejection in Service Mesh Sidecar)

### Milestone Summary
- **Configurable Request Body Limit in Sidecar Proxy (`pkg/config`, `pkg/sidecar`)**: Introduced configurable maximum request body size parameter [`SidecarConfig.MaxBodyBytes`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L109) (and `max_body_bytes` in YAML/JSON) into [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103), eliminating hardcoded buffer limits and providing a reliable default fallback of 10 MB (`10,485,760` bytes) when unset or non-positive ([`TASK-090`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-090.md), [`REQ-086`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-086.md), [`ADR-081`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-081.md), [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374)).
- **Elimination of Silent Body Truncation & Upstream Data Corruption (`pkg/sidecar`)**: Fixed critical vulnerability [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374) (CWE-436, CWE-400) in [`ProxyEngine.proxyToURL`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L192) where payloads exceeding the limit were silently truncated and forwarded upstream as corrupt data. Oversized requests are now immediately rejected with HTTP `413 Request Entity Too Large` (`http.StatusRequestEntityTooLarge`), request bodies are closed, and proxying is terminated with zero bytes transmitted upstream ([`TASK-091`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-091.md)).
- **Dual-Stage Overflow Protection**:
  - **Fast-Path Declared `Content-Length` Guard**: If an incoming request declares a `Content-Length` greater than `MaxBodyBytes`, the sidecar closes the body and immediately returns HTTP 413 without reading the payload, avoiding memory allocation overhead.
  - **Streaming Bounded Over-Read Guard**: For chunked transfer encodings or streams with undeclared lengths, ingestion is bounded using `io.LimitReader(r.Body, MaxBodyBytes+1)`. If the payload exceeds the limit, the stream is aborted, read bytes discarded, and HTTP 413 returned.
- **Direct Bypass for Non-Mutating Requests**: Safe requests (`GET`, `HEAD`) or empty requests (`ContentLength == 0` or `r.Body == nil`) bypass body reading and are dispatched directly to upstream handlers without buffer allocation.
- **Byte-Fidelity Payload Forwarding**: In-bounds requests within `MaxBodyBytes` are forwarded to the target application with exact byte fidelity and accurate content lengths.
- **Automated Verification Suite (`pkg/sidecar/sidecar_test.go`)**: Implemented unit, integration, and high-concurrency race test cases ([`TASK-092`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-092.md), [`TC-086`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-086.md)) validating default fallback, fast-fail 413 responses, chunked stream rejection, in-bounds byte preservation, and concurrent race safety under `go test -race ./pkg/sidecar/...`.

### Added
- **`MaxBodyBytes int64` Field**: Added to [`SidecarConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L103) in [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) with tags `yaml:"max_body_bytes" json:"max_body_bytes"`.
- **Default Fallback Normalization**: Added 10 MB fallback in [`DefaultAppConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go#L466), [`validateConfigDefaults`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L153), and [`NewProxyEngine`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L37).
- **Automated Test Cases (`pkg/sidecar/sidecar_test.go`)**:
  - `TestTC086_01_DefaultLimitAndFallback`: Validates 10 MB fallback for zero/negative values.
  - `TestTC086_02_DeclaredContentLength413Rejection`: Validates fast-path 413 rejection on declared `Content-Length` overflow.
  - `TestTC086_03_ChunkedStreamOverRead413Rejection`: Validates bounded `io.LimitReader` 413 rejection on chunked/streamed overflow.
  - `TestTC086_04_ByteFidelityInBoundsForwarding`: Validates exact byte preservation for valid requests under the limit.
  - `TestTC086_05_NonMutatingAndEmptyPayloadBypass`: Validates direct bypass for GET/HEAD and empty requests.
  - `TestTC086_06_HighConcurrencyRaceSafety`: Validates concurrent execution across parallel goroutines with zero data races.

### Changed
- **Proxy Body Handler (`pkg/sidecar/proxy.go`)**: Replaced silent truncation logic in [`ProxyEngine.proxyToURL`](file:///Users/sneha/Developer/toron-research/toron/pkg/sidecar/proxy.go#L192) with dual-stage fail-fast HTTP 413 rejection and diagnostic logging.

### Related Tasks & Requirements
- [`TASK-090`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-090.md): Configurable Sidecar Body Size Limit in SidecarConfig
- [`TASK-091`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-091.md): Immediate HTTP 413 Payload Too Large Rejection in Sidecar ProxyEngine
- [`TASK-092`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-092.md): Automated Verification Suite for Sidecar Body Limiting & 413 Rejection
- [`REQ-086`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-086.md): Configurable Request Body Limit and Truncation Rejection in Service Mesh Sidecar Proxy
- [`ADR-081`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-081.md): Configurable Request Body Limiting and Truncation Rejection in Service Mesh Sidecar Proxy
- [`TC-086`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-086.md): Sidecar Request Body Size Limit and HTTP 413 Rejection Verification
- [`SEC-25`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L366-L374): Silent Request Body Truncation in Service Mesh Sidecar Proxy

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
