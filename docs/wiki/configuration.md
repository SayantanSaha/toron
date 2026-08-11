---
title: Toron Configuration Guide
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
  - ADR-002

documents:
  - CONFIGURATION-GUIDE

related_to:
  - index.md
  - reference/config-options.md
  - reference/cli.md
---

# Toron Configuration Guide

## Overview

Toron features a flexible configuration system that reads parameters from YAML configuration files or falls back to built-in default settings.

## Specifying a Configuration File

Pass the path to your configuration file using the `-config` (or `-c`) command-line flag:

```bash
go run ./cmd/toron -config config.yaml
```

If no command-line flag is passed, Toron automatically checks for `config.yaml` in the current working directory.

## Sample `config.yaml`

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  worker_pool_size: 128
  read_timeout: 5s
  write_timeout: 5s
  idle_timeout: 30s
  max_header_bytes: 8192
  max_body_bytes: 4194304

static:
  enabled: true
  prefix: "/"
  dir: "./public"

logging:
  level: "info"
  format: "text"
```

## Extensible Multi-Format Support

Toron's configuration core (`pkg/config`) uses a decoupled `Loader` interface. While YAML (`.yaml` / `.yml`) is natively supported out of the box, registering decoders for JSON (`.json`) or TOML (`.toml`) can be easily added without modifying server initialization.

## Related Pages

- [CLI Reference](./reference/cli.md)
- [Configuration Options Reference](./reference/config-options.md)
