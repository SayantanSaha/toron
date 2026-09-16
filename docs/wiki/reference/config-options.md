---
title: Configuration Options Reference
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-09-16

depends_on:
  - REQ-007
  - REQ-027
  - REQ-034
  - REQ-035
  - REQ-036
  - REQ-086
  - REQ-087
  - REQ-092
  - REQ-126
  - TASK-007
  - TASK-019
  - TASK-027
  - TASK-034
  - TASK-035
  - TASK-036
  - TASK-090
  - TASK-093
  - TASK-094
  - TASK-095
  - TASK-111
  - TASK-112
  - TASK-113
  - TASK-149
  - ADR-126
  - TC-126

derived_from:
  - REQ-007
  - REQ-092
  - SEC-26
  - SEC-31

documents:
  - CONFIG-OPTIONS-REFERENCE

related_to:
  - configuration.md
  - reference/cli.md
---

# Configuration Options Reference

Complete parameter reference for `config.yaml` and `routes.yaml`.

## Section: `server`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `host` | `string` | `"0.0.0.0"` | Network interface IP binding |
| `port` | `integer` | `8080` | TCP port to listen on (1–65535) |
| `worker_pool_size` | `integer` | `128` | Concurrent worker pool count (> 0) |
| `read_timeout` | `duration` | `"5s"` | Maximum duration allowed for reading client request headers and payload. Subject to adaptive socket deadline amortization during rapid keep-alive bursts (bypasses kernel syscalls when >50% of the window remains). Set to `0` or `0s` to completely disable read deadlines (zero syscalls). Negative values strictly rejected |
| `write_timeout` | `duration` | `"5s"` | Maximum duration allowed for writing responses. Amortized for discrete responses; automatically refreshed per chunk for persistent streaming (`res.StreamBody`) to support indefinite healthy streams while mitigating Slow-Read DoS (CWE-400). Set to `0` or `0s` to completely disable write deadlines (zero syscalls). Negative values strictly rejected |
| `idle_timeout` | `duration` | `"30s"` | Inactivity deadline between transactions on persistent keep-alive connections. Strictly enforced immediately when a transaction completes and reader buffer is empty (`br.Buffered() == 0`), resetting amortization cache to prevent Slowloris starvation. Negative values strictly rejected |
| `upgrade_idle_timeout` | `duration` | `"60s"` | Inactivity deadline on upgraded protocol/WebSocket/tunnel streams (`relayStreams`) before termination. Defaults to `idle_timeout` or `60s` if omitted or <= 0 as a fail-safe against unbounded tunnels (SEC-27, ADR-083). Negative values strictly rejected |
| `max_header_bytes` | `integer` | `8192` (8 KB) | Maximum HTTP header size |
| `max_body_bytes` | `integer` | `4194304` (4 MB) | Maximum HTTP body payload size |
| `trusted_proxies` | `list` | `[]` | List of trusted proxy CIDR subnets gating `X-Forwarded-For` and `X-Real-IP` evaluation |
| `admin_subnets` | `list` | `[]` | Allowed CIDR subnets permitted to access `/internal/api/*` administrative endpoints |

### Server Connection Timeouts & Adaptive Amortization ([REQ-126](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-126.md))

Toron enforces strict socket connection timeouts to guarantee immunity against Slowloris socket exhaustion and Slow-Read Denial of Service attacks ([`REQ-005`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-005.md) §2, [`TASK-004`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-004.md) §3, [`ADR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-083.md), [`ADR-126`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-126.md)).

#### Parameter Specifications

- **`read_timeout`** (`time.Duration`):
  Defines the maximum time Toron will wait to read the entire HTTP request headers and body. Under steady-state keep-alive traffic bursts, client connections are wrapped in an adaptive tracker ([`connDeadlineTracker`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/deadline.go#L14-L23)). If an active kernel read deadline was already established and more than half the timeout window remains ($R > \tau/2$), redundant operating system calls to `SetReadDeadline` are safely bypassed, achieving $>99\%$ syscall reduction at high request rates.
- **`write_timeout`** (`time.Duration`):
  Defines the maximum duration allowed to serialize response headers and write body bytes to the client socket.
  - **Discrete Responses**: Amortized identically to read deadlines during rapid keep-alive transactions.
  - **Streaming Responses (`res.StreamBody`)**: For long-lived streaming connections (such as Server-Sent Events `text/event-stream`, live feeds, or unbuffered proxy streams), Toron refreshes the socket write deadline on **every transmitted chunk** (`ForceSetWriteDeadline(now + write_timeout)`). This allows active, healthy streams to remain connected indefinitely (hours or days) while ensuring that stalled or slow-reading clients (advertising a zero TCP window or reading below rate) are terminated within `write_timeout` after socket buffers saturate ([CWE-400](https://cwe.mitre.org/data/definitions/400.html) Slow-Read defense).
- **`idle_timeout`** (`time.Duration`):
  Defines the maximum duration an idle keep-alive connection can wait for the arrival of the next request. The moment an HTTP transaction completes and the socket reader buffer is empty (`br.Buffered() == 0`), Toron immediately invalidates the read amortization cache (`ResetReadAmortization()`) and forces an explicit kernel deadline (`ForceSetReadDeadline(now + idle_timeout)`). This prevents long active request read deadlines (e.g. 5s or 10s) from lingering into idle periods, guaranteeing that idle connections disconnect promptly after `idle_timeout`.
- **`upgrade_idle_timeout`** (`time.Duration`):
  Defines the maximum inactivity timeout for upgraded full-duplex protocols (WebSockets, RFC 8441 HTTP/2 CONNECT tunnels, L4 transparent TCP relays). In bidirectional relay loops ([`relayStreams`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go#L696-L770)), deadlines are amortized during active frame transfers, while silence across both directions terminates the connection after `upgrade_idle_timeout`. If set to `0` or omitted, Toron defaults to `idle_timeout` (or `60s` if `idle_timeout` is also 0) as a critical security fail-safe ([`SEC-27`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L384-L392), [`ADR-083`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-083.md)).

#### Non-Negative Validation Rules
All server timeout parameters are strictly validated by [`ValidateConfig`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/loader.go#L175-L203) during startup and configuration dry-run (`toron -t`):
- Any negative duration (e.g. `read_timeout: -5s`, `write_timeout: -1s`, `idle_timeout: -200ms`, `upgrade_idle_timeout: -10s`) is strictly rejected with an explicit error:
  ```text
  server.read_timeout must be non-negative, got -5s
  ```
- Startup halts immediately, preventing misconfigured services from deploying with invalid or negative timeout calculations.

#### Zero-Timeout Mode (Benchmark & Isolated Environments)
For performance engineers conducting raw benchmark evaluations or deploying in isolated, trusted private enclaves where maximum throughput is paramount:
- Setting `read_timeout: 0` (or `0s`) or `write_timeout: 0` (or `0s`) explicitly disables socket deadline enforcement.
- When configured to `0`, [`connDeadlineTracker`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/deadline.go#L57-L64) clears any existing deadline via `conn.SetDeadline(time.Time{})` and executes **exactly zero socket deadline system calls** during steady-state request processing.
- The configuration loader preserves explicit `0` values and does **not** overwrite them with default values (`validateConfigDefaults`).

> [!WARNING]
> Disabling connection deadlines (`read_timeout: 0`, `write_timeout: 0`) removes Slowloris and Slow-Read DoS protections. Only utilize zero-timeout mode in trusted networks, closed benchmark clusters, or behind upstream edge load balancers with their own deadline enforcement.


## Section: `server.http2`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable/disable HTTP/2 protocol engine |
| `max_concurrent_streams` | `integer` | `250` | Maximum HTTP/2 streams per connection |
| `allow_h2c` | `boolean` | `true` | Allow HTTP/2 Cleartext (h2c) prior-knowledge upgrades |

## Section: `server.http3`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable/disable HTTP/3 QUIC protocol engine over UDP |
| `port` | `integer` | `8443` | UDP listener port for HTTP/3 QUIC |
| `alt_svc_header` | `boolean` | `true` | Automatically inject `Alt-Svc: h3=":8443"` response headers |

## Section: `server.tls`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `false` | Enable/disable HTTPS TLS listener |
| `cert_file` | `string` | `""` | Path to X.509 certificate file |
| `key_file` | `string` | `""` | Path to private key file |
| `auto_dev_cert` | `boolean` | `true` | Auto-generate self-signed ECDSA dev cert if files empty |

## Section: `server.acme`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `false` | Enable ACME zero-touch production SSL issuance |
| `directory_url` | `string` | `"https://acme-v02.api.letsencrypt.org/directory"` | ACME directory endpoint URL |
| `email` | `string` | `"admin@toron.local"` | ACME registration contact email |
| `domains` | `list` | `[]` | List of target domains for ACME SSL issuance |
| `cache_dir` | `string` | `"./certs"` | Local directory path for caching keys and certs |
| `challenge_type` | `string` | `"http-01"` | Challenge validation strategy (`"http-01"` or `"tls-alpn-01"`) |

## Section: `server.compression`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable transparent response compression (Zstd, Brotli, Gzip, Deflate) |
| `min_length` | `integer` | `512` | Minimum response byte threshold for compression |
| `level` | `integer` | `-1` | Compression level (-1 = default, 1 = best speed, 9 = best compression) |
| `encodings` | `list` | `["zstd", "br", "gzip", "deflate"]` | Supported compression encoding algorithms |

## Section: `server.cache`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable in-memory RFC 7234 HTTP response caching |
| `default_ttl` | `duration` | `"60s"` | Default TTL if `Cache-Control: max-age` is omitted |
| `max_entries` | `integer` | `1000` | Maximum number of cached responses in memory |
| `max_payload_size` | `integer` | `1048576` (1 MB) | Maximum body size eligible for caching |

## Section: `server.cors` / `routes[].cors`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable CORS preflight handling and header injection |
| `allow_origins` | `list` | `["*"]` | Allowed CORS origins |
| `allow_methods` | `list` | `["GET", "POST", ...]` | Allowed HTTP request methods |
| `allow_headers` | `list` | `["Origin", ...]` | Allowed HTTP request headers |
| `expose_headers` | `list` | `["X-Cache", ...]` | Headers exposed to client scripts |
| `allow_credentials` | `boolean` | `false` | Enable Access-Control-Allow-Credentials |
| `max_age` | `integer` | `86400` | Preflight cache max age in seconds |

## Section: `server.security_headers` / `routes[].security_headers`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable enterprise browser security headers |
| `hsts` | `string` | `"max-age=31536000; ..."` | Strict-Transport-Security header value |
| `content_type_options` | `string` | `"nosniff"` | X-Content-Type-Options header value |
| `frame_options` | `string` | `"DENY"` | X-Frame-Options header value |
| `referrer_policy` | `string` | `"strict-origin-..."` | Referrer-Policy header value |
| `csp` | `string` | `""` | Content-Security-Policy header value |

## Section: `server.waf` / `routes[].waf`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable Layer 7 WAF inspection middleware |
| `mode` | `string` | `"enforce"` | Operational mode: `"enforce"` (403 block) or `"detection"` (log-only) |
| `anomaly_threshold` | `integer` | `5` | Threat anomaly score threshold for blocking |
| `max_inspect_body_size` | `integer` | `65536` (64 KB) | Maximum payload body bytes inspected |
| `allowed_ips` | `list` | `[]` | Fast-path CIDR IP allowlist |
| `denied_ips` | `list` | `[]` | Fast-path CIDR IP denylist |
| `disabled_rules` | `list` | `[]` | List of rule IDs to bypass (e.g. `["SQLI-001"]`) |
| `custom_rules` | `list` | `[]` | User-defined regex threat rules (`id`, `category`, `pattern`, `score`, `locations`) |
| `trusted_proxies` | `list` | `[]` | Trusted proxy CIDR subnets allowed to pass forwarded client IPs for WAF evaluation |
| `audit_log.enabled` | `boolean` | `true` | Enable structured JSON security audit logger |
| `audit_log.output` | `string` | `"stdout"` | Audit destination (`"stdout"`, `"stderr"`, or file path) |
| `audit_log.format` | `string` | `"json"` | Audit output format |

## Section: `discovery` (OCI Container Auto-Discovery)

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable OCI container auto-discovery over Unix socket |
| `engine` | `string` | `"auto"` | Runtime engine detection (`"auto"`, `"docker"`, `"podman"`) |
| `socket_path` | `string` | `"auto"` | Unix domain socket path (`"auto"` probes standard paths) |
| `poll_interval` | `duration` | `"10s"` | Background periodic polling interval |
| `default_weight` | `integer` | `1` | Default load balancing weight for discovered nodes |

## Section: `ingress` (Kubernetes Ingress Controller)

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `false` | Enable native Kubernetes Ingress Controller |
| `ingress_class` | `string` | `"toron"` | Target IngressClass resource filter |
| `kube_apiserver` | `string` | `"https://kubernetes.default.svc"` | Kubernetes API server URL |
| `service_account_dir` | `string` | `"/var/run/secrets/..."` | Path to in-cluster ServiceAccount token directory |
| `resync_period` | `duration` | `"30s"` | Periodic resource reconciliation interval |

## Section: `sidecar` (Service Mesh Sidecar Mode)

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `false` | Enable Service Mesh Sidecar mode |
| `mode` | `string` | `"dual"` | Mode: `"ingress"`, `"egress"`, or `"dual"` |
| `ingress_port` | `integer` | `15006` | Pod inbound mTLS listener port |
| `egress_port` | `integer` | `15001` | Pod outbound proxy listener port |
| `app_port` | `integer` | `8080` | Local application container port (127.0.0.1) |
| `max_body_bytes` | `integer` | `10485760` (10 MB) | Maximum request body size in bytes before returning HTTP 413 (defaults to 10MB if omitted or <= 0) |
| `strict_mtls` | `boolean` | `false` | Enforce RequireAndVerifyClientCert mTLS |
| `cert_file` | `string` | `""` | Path to sidecar X.509 certificate file |
| `key_file` | `string` | `""` | Path to sidecar private key file |
| `ca_file` | `string` | `""` | Path to trusted CA bundle file for client verification |
| `traffic_splits` | `list` | `[]` | Canary weighted traffic splits (`prefix`, `backends: [{target, weight}]`) |

## Section: `transcoder` (REST-to-gRPC Transcoding)

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable REST JSON to Protobuf gRPC transcoding |
| `max_body_bytes` | `integer` | `4194304` (4MB) | Maximum permissible request body size in bytes before HTTP 413 rejection (SEC-28) |
| `routes` | `list` | `[]` | Transcoding rules (`http_method`, `http_path`, `grpc_method`, `upstream_url`, `field_mappings`) |

## Section: `server.auth` / `routes[].auth`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `type` | `string` | `""` | Authentication scheme (`"jwt"`, `"api_key"`, `"basic"`, `""`) |
| `jwt.secret` | `string` | `""` | HMAC secret for JWT verification |
| `jwt.issuer` | `string` | `""` | Expected JWT `iss` claim |
| `jwt.audience` | `string` | `""` | Expected JWT `aud` claim |
| `api_key.keys` | `list` | `[]` | Allowlist of valid API keys |
| `basic.users` | `map` | `{}` | Map of valid username to password pairs |
| `excluded` | `list` | `[]` | List of public paths exempt from authentication |

## Section: `routes[]` (`routes.yaml` Schema)

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| `type` | `string` | Route type: `"static"`, `"upstream"` (or `"proxy"`), `"tcp"`, `"udp"` |
| `prefix` | `string` | URL path prefix to match (e.g. `"/api"`, `"/internal/dashboard"`) |
| `host` | `string` | Virtual host domain name to match (e.g. `"api.toron.local"`) |
| `headers` | `map` | Conditional request headers to match (e.g. `X-Version: "v2"`) |
| `target` | `string` | Single upstream backend target URL (e.g. `"http://localhost:9001"`) |
| `targets` | `list` | List of upstream target URLs for load balancing clusters |
| `algorithm` | `string` | Balancing algorithm: `round_robin`, `weighted_round_robin`, `least_conn`, `weighted_least_conn`, `least_latency`, `random`, `sticky_cookie`, `ip_hash` |
| `sticky_cookie_name` | `string` | Cookie name used for session affinity (default: `"TORON_STICKY"`) |
| `strip_prefix` | `boolean` | Strip route prefix from URL path before forwarding upstream (default: `true`) |
| `rewrite_redirects` | `boolean` | Intercept and rewrite 3xx `Location` redirect URLs to include route prefix (default: `true`) |
| `rewrite_cookie_path` | `boolean` | Rewrite `Set-Cookie: Path=/` attributes to `Path=<prefix>` (default: `true`) |
| `rate_limit` | `string` | Token bucket rate limit spec (e.g. `"100/min"`, `"10/s"`) |
| `health_check_type` | `string` | Health probe protocol: `"http"` (default) or `"grpc"` |
| `health_check_path` | `string` | HTTP endpoint path for active health probes (e.g. `"/health"`) |
| `health_check_service`| `string` | Target gRPC service name for `grpc.health.v1` probes |
| `health_check_interval`| `duration` | Frequency of background health probes (e.g. `"5s"`) |
| `consecutive_failures`| `integer` | Failure count required to trip circuit breaker to `Open` |
| `cooldown_period` | `duration` | Recovery wait duration before trial probe in `HalfOpen` |
| `dir` | `string` | Local filesystem directory for `"static"` routes (e.g. `"./public"`) |
| `spa` | `boolean` | Enable Single Page Application (SPA) HTML5 History fallback (default: `false`) |
| `fallback` | `string` | Fallback document filename in `dir` (default: `"index.html"`, auto-enables SPA) |
| `listen_port` | `integer` | Inbound listening port for Layer 4 `"tcp"` or `"udp"` proxies |
| `max_connections` | `integer` | Maximum concurrent active TCP connections for Layer 4 `"tcp"` proxies (default: `10000`). Saturated connections are fast-rejected |
| `idle_timeout` | `duration` | Inactivity timeout before closing idle Layer 4 TCP connections (Slowloris protection) or expiring idle UDP client sessions (default: `"60s"`) |
| `max_workers` | `integer` | Maximum worker goroutines / queue capacity for Layer 4 `"udp"` datagram processing (default: `1024`). Excess datagrams during saturation are dropped fail-safe |
| `tls` | `object` | Per-host TLS/mTLS settings (`cert_file`, `key_file`, `ca_file`, `client_auth`, `min_version`) |
| `waf` | `object` | Route-level WAF overrides (`enabled`, `mode`, `allowed_ips`, `denied_ips`, `disabled_rules`) |
| `auth` | `object` | Route-level authentication overrides (`type`, `jwt`, `api_key`, `basic`) |
| `trusted_proxies` | `list` | Route-level trusted proxy CIDR subnets gating forwarded headers for this route |
| `transport` | `object` | Route-level upstream transport override (`max_conns_per_host`, `disable_compression`, etc.) |

## Section: `proxy.transport` / `routes[].transport` (REQ-123, REQ-124)

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `profile` | `string` | `"raw_speed"` | Transport preset profile: `"raw_speed"` (default) or `"balanced"` / `"standard"` |
| `max_idle_conns` | `integer` | `10000` | Global maximum idle connections across all upstream target hosts |
| `max_idle_conns_per_host` | `integer` | `1000` | Maximum idle keep-alive connections retained per upstream origin |
| `max_conns_per_host` | `integer` | `0` | Total concurrent active connections per host (`0` = unconstrained; `>0` throttles & queues) |
| `idle_conn_timeout` | `duration` | `"90s"` | Inactivity duration before closing idle persistent keep-alive sockets |
| `disable_compression` | `boolean` | `true` | `true` = zero-copy raw byte passthrough; `false` = transparent gzip decompression |
| `use_env_proxy` | `boolean` | `false` | `false` = direct socket dialing; `true` = honors `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` |
| `proxy_url` | `string` | `""` | Explicit forward proxy URL (e.g. `"http://squid.corp:3128"`) |
| `propagate_upstream_close` | `boolean` | `false` | `false` = isolate client keep-alives; `true` = clean client teardown on origin close |
| `force_attempt_http2` | `boolean` | `false` | `false` = HTTP/1.1 wire transport; `true` = ALPN `h2` stream multiplexing to TLS origins |
| `tracing` | `boolean` | `false` | `false` = suppress CSPRNG trace ID generation for raw speed (REQ-124); `true` = generate W3C `traceparent` |
| `stream_response` | `boolean` | `true` (`raw_speed`) / `false` (`balanced`) | `true` = zero-copy socket streaming fast-path for unbuffered/SSE and pure routes; `false` = buffer in memory (REQ-125) |
| `response_header_timeout` | `duration` | `"10s"` | Bounded timeout for upstream response header arrival (dial-to-first-byte), decoupling body streaming (REQ-125) |

## Section: `logging`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `level` | `string` | `"info"` | Log output verbosity level (`debug`, `info`, `warn`, `error`) |
| `format` | `string` | `"text"` | Log formatting style (`text`, `json`) |

## Related Pages

- [Configuration Guide](../configuration.md)
- [Layer 4 TCP & UDP Transport Proxies](../features/layer4-proxy.md)
- [CLI Reference](./cli.md)
