---
id: TASK-018
type: task
title: Implement Configurable Static Prefix Trailing Slash Redirect and Relative Asset Loading
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-09-08

depends_on:
  - REQ-018

implements:
  - REQ-018

verified_by:
  - TC-018

decided_by:
  - ADR-013

related_to:
  - TASK-006
  - TASK-016
---

# TASK-018 - Implement Configurable Static Prefix Trailing Slash Redirect and Relative Asset Loading

## Goal

Update static asset links in `public/index.html` to relative paths and add automatic trailing slash redirection in `pkg/router/router.go`.

## Sub-tasks

1. Update `public/index.html` asset tags from `/style.css` and `/app.js` to `./style.css` and `./app.js`.
2. Update `pkg/router/router.go` static file handler to emit `302 Found` redirect to `cleanPrefix + "/"` when `req.Path == cleanPrefix` and `cleanPrefix != ""`.
3. Add unit test in `pkg/router/router_test.go` verifying trailing slash redirect and static file serving under subpath prefix `/internal/dashboard/`.
4. Rebuild frontend CSS (`npm run build:css`).

## Acceptance Criteria

- Request to `/internal/dashboard` redirects to `/internal/dashboard/`.
- Relative assets `./style.css` and `./app.js` load cleanly under `/internal/dashboard/`.
- All Go unit tests pass cleanly.
