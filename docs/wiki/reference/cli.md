---
title: CLI Command Line Reference
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
  - CLI-REFERENCE

related_to:
  - configuration.md
  - reference/config-options.md
---

# CLI Command Line Reference

## Command Usage

```bash
toron [flags]
```

## Available Flags

| Flag | Short | Default | Description |
| ---- | ----- | ------- | ----------- |
| `-config <path>` | `-c <path>` | `""` (checks `config.yaml`) | Path to YAML server configuration file |
| `-routes <path>` | `-r <path>` | `""` (checks `routes.yaml`) | Path to YAML proxy routing configuration file |
| `-test-config` | `-t` | `false` | Dry-run test configuration file syntax and exit |
| `-help` | `-h` | N/A | Prints CLI flag usage summary |

## Examples

### Test configuration file syntax before starting server
```bash
go run ./cmd/toron -t
```

### Specify custom server and routing config files
```bash
go run ./cmd/toron -config ./configs/server.yaml -routes ./configs/routes.yaml
```

## Related Pages

- [Configuration Options](./config-options.md)
