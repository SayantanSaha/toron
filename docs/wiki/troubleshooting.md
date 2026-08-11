---
title: Troubleshooting Guide
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-005
  - REQ-006

derived_from:
  - REQ-005
  - REQ-006

documents:
  - TROUBLESHOOTING-GUIDE

related_to:
  - faq.md
  - getting-started.md
---

# Troubleshooting Guide

Common issues encountered when running or deploying Toron and their resolutions.

## Problem Resolution Matrix

| Problem | Cause | Fix |
| ------- | ----- | --- |
| `bind: address already in use` | Another process (or previous Toron instance) is using port 8080. | Stop the conflicting process (`Ctrl+C` or kill PID), or change `port` in `config.yaml`. |
| `404 Not Found` for static files | Target file does not exist in `./public` or directory path is relative to wrong CWD. | Verify terminal prompt working directory matches project root (`D:/Work/server`) and file exists in `./public`. |
| `431 Request Header Fields Too Large` | HTTP client headers exceed `max_header_bytes` limit (8KB). | Reduce request header size or increase `max_header_bytes` in `config.yaml`. |
| `413 Payload Too Large` | Request body payload exceeds `max_body_bytes` limit (4MB). | Increase `max_body_bytes` in `config.yaml`. |
| `403 Forbidden` on static asset | Path traversal attempt (`../`) detected in URL. | Ensure requested asset paths do not escape static directory root. |

## Related Pages

- [FAQ](./faq.md)
- [Getting Started](./getting-started.md)
