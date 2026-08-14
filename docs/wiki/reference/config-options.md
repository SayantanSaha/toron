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
| `enabled` | `boolean` | `true` | Enable transparent response compression (Gzip & Deflate) |
| `min_length` | `integer` | `512` | Minimum response byte threshold for compression |
| `level` | `integer` | `-1` | Compression level (-1 = default, 1 = best speed, 9 = best compression) |
| `encodings` | `list` | `["gzip", "deflate"]` | Supported compression encoding algorithms |

## Section: `server.cache`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable in-memory RFC 7234 HTTP response caching |
| `default_ttl` | `duration` | `"60s"` | Default TTL if `Cache-Control: max-age` is omitted |
| `max_entries` | `integer` | `1000` | Maximum number of cached responses in memory |
| `max_payload_size` | `integer` | `1048576` (1 MB) | Maximum body size eligible for caching |

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

## Section: `logging`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `level` | `string` | `"info"` | Log output verbosity level (`debug`, `info`, `warn`, `error`) |
| `format` | `string` | `"text"` | Log formatting style (`text`, `json`) |

## Related Pages

- [Configuration Guide](../configuration.md)
- [CLI Reference](./cli.md)
