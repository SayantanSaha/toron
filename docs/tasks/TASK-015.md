---
id: TASK-015
type: task
title: Create test_endpoint.http REST Client Test File
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-09-08

depends_on:
  - REQ-015

implements:
  - REQ-015

verified_by:
  - TC-015

decided_by:
  - ADR-010

related_to:
  - TASK-013
---

# TASK-015 - Create test_endpoint.http REST Client Test File

## Goal

Create `test_endpoint.http` in root directory with HTTP request definitions for all Toron server routes.

## Sub-tasks

1. Define base variables (`@host = http://localhost:8080`).
2. Write native route tests (`/health`, `/api/status`).
3. Write static file route tests (`/`, `/style.css`, `/app.js`).
4. Write header-based proxy route tests (`X-Version: v2`, `X-Version: v1`).
5. Write path-based proxy route tests (`/services/cluster`, `/services/auth`, `/services/analytics`).

## Acceptance Criteria

- `test_endpoint.http` created and valid.
