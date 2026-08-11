---
title: Configuration Options Reference
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-007
  - TASK-007

derived_from:
  - REQ-007

documents:
  - CONFIG-OPTIONS-REFERENCE

related_to:
  - configuration.md
  - reference/cli.md
---

# Configuration Options Reference

Complete parameter reference for `config.yaml`.

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

## Section: `static`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `enabled` | `boolean` | `true` | Enable/disable static file serving |
| `prefix` | `string` | `"/"` | URL prefix for static asset route |
| `dir` | `string` | `"./public"` | Filesystem directory to serve files from |

## Section: `logging`

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `level` | `string` | `"info"` | Log output verbosity level (`debug`, `info`, `warn`, `error`) |
| `format` | `string` | `"text"` | Log formatting style (`text`, `json`) |

## Related Pages

- [Configuration Guide](../configuration.md)
