---
id: TASK-016
type: task
title: Implement Mobile-First Control Center and Proxy Dashboard UI
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-016

implements:
  - REQ-016

verified_by:
  - TC-016

decided_by:
  - ADR-011

related_to:
  - TASK-002
---

# TASK-016 - Implement Mobile-First Control Center and Proxy Dashboard UI

## Goal

Build and integrate the responsive Web Control Center and Proxy Dashboard in `/public` using HTML5, Vanilla JS, and Tailwind CSS.

## Sub-tasks

1. Configure Tailwind CSS CLI build script in `package.json` (`build:css`).
2. Create `public/input.css` with `@import "tailwindcss";` and custom utility styles.
3. Build `public/index.html` with responsive mobile bottom navbar and desktop header tabs.
4. Implement `public/app.js` with real-time HTTP health probing, live API request tester, and tab navigation.
5. Add CORS headers and 500 error simulation handling to `dummy-services/services.go`.

## Acceptance Criteria

- Responsive UI works across mobile and desktop viewports.
- Real HTTP status probes correctly flag healthy (200) vs unhealthy (500) upstreams.
- All Go package tests pass cleanly (`go test ./...`).
