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
| `-config <path>` | `-c <path>` | `""` (checks `config.yaml`) | Path to YAML configuration file |
| `-help` | `-h` | N/A | Prints CLI flag usage summary |

## Examples

### Specify custom config file
```bash
go run ./cmd/toron -config ./configs/production.yaml
```

### Use default config resolution
```bash
go run ./cmd/toron
```

## Related Pages

- [Configuration Options](./config-options.md)
