---
id: TASK-017
type: task
title: Implement /internal/api/ Management Endpoints and Update Frontend Client
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-017

implements:
  - REQ-017

verified_by:
  - TC-017

decided_by:
  - ADR-012

related_to:
  - TASK-016
---

# TASK-017 - Implement /internal/api/ Management Endpoints and Update Frontend Client

## Goal

Create Go handlers for `/internal/api/` endpoints and update `public/app.js` to rely solely on internal API responses.

## Sub-tasks

1. Create `pkg/server/internal_api.go` providing handlers for:
   - `GET /internal/api/status`
   - `GET /internal/api/routes`
   - `GET /internal/api/upstreams/health`
   - `POST /internal/api/proxy-test`
2. Register `/internal/api/` routes in `cmd/toron/main.go` and `pkg/server`.
3. Add unit test suite `pkg/server/internal_api_test.go`.
4. Update `public/app.js` to call `/internal/api/upstreams/health` and `/internal/api/proxy-test`.
5. Run full test suite (`go test ./...`) and rebuild frontend styles if needed.

## Acceptance Criteria

- All `/internal/api/` endpoints function correctly.
- Frontend fetches metrics and health states strictly via `/internal/api/`.
- All Go tests pass cleanly.
