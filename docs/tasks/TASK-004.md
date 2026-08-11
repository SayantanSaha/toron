---
id: TASK-004
type: task
title: Security Guards, Request Limits & Connection Timeouts
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-08-11

depends_on:
  - TASK-001
  - TASK-002

derived_from:
  - REQ-005

implements:
  - REQ-005

verified_by: []

decided_by: []

related_to: []
---

# TASK-004 - Security Guards, Request Limits & Connection Timeouts

## Description

Implement security enforcement mechanisms including HTTP request header/body size limiters, connection read/write/idle deadline timers, and input sanitization guards.

## Acceptance Criteria

- Enforce max header size (default 8KB) returning `400 Bad Request` or `431 Request Header Fields Too Large`.
- Enforce max body size (default 4MB) returning `413 Payload Too Large`.
- Set socket read and write deadlines per connection to prevent Slowloris attacks.
- Sanitize response headers to prevent response splitting / CRLF injection.

## Rationale

Protects server stability and prevents resource exhaustion or protocol exploitation.

## Constraints

- Configurable via `ServerConfig` struct.

## Open Questions

- None.
