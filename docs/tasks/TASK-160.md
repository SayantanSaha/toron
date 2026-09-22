---
title: TASK-160 Implement Dashboard Datetime Formatting, Alert/Ban Sorting, and Source IP Column
type: task
project: PROJECT-001
owner: development-lead
created: 2026-09-22
status: in-progress

references:
  - REQ-137
  - ADR-137
  - TC-137
---

# TASK-160: Implement Dashboard Datetime Formatting, Alert/Ban Sorting, and Source IP Column

## Objective

Enhance `public/app.js` and `public/style.css` to implement exact `YYYY-MM-DD HH:MM:SS` timestamps, latest-first chronological sorting for alerts and bans, and a visible Source IP column in live requests.

## Implementation Steps

1. Add `dtFmt(d)` datetime formatter utility in `public/app.js`.
2. Compute explicit `timestamp` for all alert objects in `buildDataModel()` and sort `alerts` latest-first.
3. Update Active Alerts UI (`alUpdate` & `ovUpdate`) to display full date and timestamp.
4. Update Banned IP Table (`alShell` & `alUpdate`) to include a "Created At" column with full date and timestamp, sorted latest-first.
5. Update Requests Table (`lgShell` & `lgUpdate`) to include "Source IP" column and display full date and time in the Time column.
6. Verify locally and deploy to live server `/etc/toron/public/`.
