---
title: Toron Documentation Index
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-001
  - REQ-002
  - REQ-003
  - REQ-004
  - REQ-005
  - REQ-006
  - REQ-007
  - REQ-008

derived_from:
  - PRD.md
  - ADR-001
  - ADR-002
  - ADR-003

documents:
  - TORON-DOCUMENTATION-INDEX

related_to:
  - getting-started.md
  - configuration.md
  - release-notes.md
---

# Toron Documentation Wiki

Welcome to the **Toron Web Server** documentation wiki. Toron is an event-driven, high-performance, modular web server written in Go.

## Wiki Navigation

### 🚀 Getting Started & Installation
- [Getting Started](./getting-started.md) – Quickstart guide for running Toron.
- [Configuration Guide](./configuration.md) – Overview of configuring Toron via YAML or CLI flags.

### ⚙️ Features & Architecture
- [Event Reactor Core](./features/event-reactor.md) – Event-driven concurrency, non-blocking I/O, and worker pool.
- [Static File Serving](./features/static-file-serving.md) – Hosting web applications, MIME type resolution, and security.
- [Native Go Benchmarking](./features/benchmarking.md) – Performance benchmarks and allocation metrics.

### 📖 References
- [CLI Reference](./reference/cli.md) – Command-line interface options and usage flags.
- [Configuration Options Reference](./reference/config-options.md) – Complete reference for `config.yaml` parameters.
- [HTTP API Reference](./reference/api.md) – Built-in health and status HTTP endpoints.

### 💡 Help & Support
- [Troubleshooting Guide](./troubleshooting.md) – Common runtime issues, 404 errors, and solutions.
- [Frequently Asked Questions (FAQ)](./faq.md) – Common questions about Toron.
- [Release Notes](./release-notes.md) – Changelog generated from completed task documents.
