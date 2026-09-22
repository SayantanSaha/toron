---
id: TASK-161
type: task
title: Implement Multi-Level Sorting on Alerts and Requests in Dashboard
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-22
updated: 2026-09-22

depends_on:
  - REQ-138

implements:
  - REQ-138

verified_by:
  - TC-138

decided_by:
  - ADR-138
---

# TASK-161 - Implement Multi-Level Sorting on Alerts and Requests in Dashboard

## Context

Following REQ-138, dashboard data lists must sort consistently and deterministically when timestamps match. This task details the specific frontend implementation work required.

## Task Breakdown

1. **Alert Model Enrichment & Sorting (`buildDataModel`)**:
   - Ensure every alert item generated in `buildDataModel()` has an explicit `ip` string property.
   - Update the sorting function on `alerts` to compare `(b.timestamp || 0) - (a.timestamp || 0)` first, and if 0, compare `(a.ip || '').localeCompare(b.ip || '')`.
2. **Banned IP Table Multi-Level Sorting (`alUpdate`)**:
   - Update sorting in `alUpdate()` for `bannedIps` to compare `new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime()` first, and if 0, compare `(a.ip || '').localeCompare(b.ip || '')`.
3. **Live Requests Multi-Level Sorting (`lgUpdate`)**:
   - Update sorting in `lgUpdate()` for filtered `logs` to apply 3-tier sorting:
     - Tier 1: `(b.ts || 0) - (a.ts || 0)` (latest first).
     - Tier 2: `(a.ip || a.client_ip || '').localeCompare(b.ip || b.client_ip || '')` (ascending).
     - Tier 3: `(a.path || '').localeCompare(b.path || '')` (ascending).

## Acceptance Criteria

- All acceptance criteria in REQ-138 are satisfied.
- Verified by TC-138 and reviewed by Code Reviewer and Security Analyst.
