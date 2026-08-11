---
id: TASK-006
type: task
title: Static File Handler & Path Traversal Guard Implementation
status: draft
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-11
updated: 2026-08-11

depends_on:
  - TASK-003
  - TASK-004

derived_from:
  - REQ-006

implements:
  - REQ-006

verified_by: []

decided_by: []

related_to: []
---

# TASK-006 - Static File Handler & Path Traversal Guard Implementation

## Description

Implement `StaticDir(prefix, dirPath)` in `pkg/router` to serve static assets from the filesystem, resolve `index.html` for directory requests, infer MIME content-types, and enforce path traversal guards.

## Acceptance Criteria

- `router.Static(prefix, dirPath)` registers prefix route handling.
- `mime.TypeByExtension` resolves file content types (e.g. `.html` -> `text/html`, `.css` -> `text/css`, `.js` -> `application/javascript`).
- Prevents `../` directory traversal escapes using `filepath.Clean` and `strings.HasPrefix`.
- Serves `index.html` when path targets a directory.

## Rationale

Provides built-in static site web hosting capabilities to Toron.

## Constraints

- Cross-platform file path resolution (`filepath.ToSlash`).

## Open Questions

- None.
