---
title: Frequently Asked Questions (FAQ)
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-001
  - REQ-007

derived_from:
  - PRD.md

documents:
  - FAQ

related_to:
  - index.md
  - troubleshooting.md
---

# Frequently Asked Questions (FAQ)

## 1. What makes Toron different from Go's stdlib `net/http`?
Toron is built explicitly around an event reactor architecture with configurable worker pools, `sync.Pool` memory recycling, and granular connection timeout protection out of the box.

## 2. Can I use JSON for configuration files instead of YAML?
YAML is supported natively out of the box. The configuration architecture (`pkg/config`) is extensible—support for JSON or TOML can be registered via `config.Manager`.

## 3. Does Toron support static site hosting?
Yes! Static site hosting is enabled by default from `./public` and includes MIME type detection, `index.html` fallback, and path traversal security guards.

## 4. How does Toron handle panics in HTTP handlers?
Toron includes a `RecoveryMiddleware` that catches panics, logs the stack trace internally, and returns a safe `500 Internal Server Error` response without exposing stack traces to clients.

## Related Pages

- [Troubleshooting](./troubleshooting.md)
- [Getting Started](./getting-started.md)
