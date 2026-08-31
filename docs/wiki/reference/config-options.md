---
title: Configuration Options Reference
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-14

depends_on:
  - REQ-007
  - REQ-034
  - REQ-035
  - REQ-036
  - TASK-007
  - TASK-034
  - TASK-035
  - TASK-036

derived_from:
  - REQ-007

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
| `port` | `integer` | `8080` | TCP port to listen on |
| `worker_pool_size` | `integer` | `128` | Concurrent worker pool count |
| `read_timeout` | `duration` | `"5s"` | Socket read deadline timeout |
| `write_timeout` | `duration` | `"5s"` | Socket write deadline timeout |
| `idle_timeout` | `duration` | `"30s"` | Socket idle keep-alive timeout |
| `max_header_bytes` | `integer` | `8192` (8 KB) | Maximum HTTP header size |
| `max_body_bytes` | `integer` | `4194304` (4 MB) | Maximum HTTP body payload size |

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
| `strict_mtls` | `boolean` | `false` | Enforce RequireAndVerifyClientCert mTLS |
| `traffic_splits` | `list` | `[]` | Canary weighted traffic splits (`prefix`, `backends: [{target, weight}]`) |

## Section: `transcoder` (REST-to-gRPC Transcoding)

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable REST JSON to Protobuf gRPC transcoding |
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
| `listen_port` | `integer` | Inbound listening port for Layer 4 `"tcp"` or `"udp"` proxies |
| `tls` | `object` | Per-host TLS/mTLS settings (`cert_file`, `key_file`, `ca_file`, `client_auth`, `min_version`) |
| `waf` | `object` | Route-level WAF overrides (`enabled`, `mode`, `allowed_ips`, `denied_ips`, `disabled_rules`) |
| `auth` | `object` | Route-level authentication overrides (`type`, `jwt`, `api_key`, `basic`) |

## Section: `logging`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `level` | `string` | `"info"` | Log output verbosity level (`debug`, `info`, `warn`, `error`) |
| `format` | `string` | `"text"` | Log formatting style (`text`, `json`) |

## Related Pages

- [Configuration Guide](../configuration.md)
- [CLI Reference](./cli.md)
