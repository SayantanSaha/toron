---
id: TASK-075
type: task
title: Remediate Stored DOM-based XSS in Security Control Center
status: draft
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-075
implements:
  - REQ-075
verified_by: []
decided_by: []
related_to: []
---

# TASK-075 - Remediate Stored DOM-based XSS in Security Control Center

## Description

Refactor dynamic DOM rendering in `public/app.js` to eliminate unsafe `innerHTML` interpolation of user-controlled audit log entries and route names, preventing Stored DOM XSS (CWE-79).

## Scope & Implementation Breakdown

1. **HTML Entity Escaping Helper (`public/app.js`)**:
   - Implement an efficient `escapeHTML(str)` utility in `public/app.js` escaping `&`, `<`, `>`, `"`, `'`, and `/`.
2. **Sanitize Incident Log Rows (`public/app.js:fetchIncidentLogs`)**:
   - Apply `escapeHTML` to `inc.path`, `inc.client_ip`, `inc.rule_id`, and `inc.timestamp` before table row creation, or construct rows using safe `document.createElement` and `.textContent`.
3. **Sanitize Route Lists (`public/app.js:updateRoutesTable`)**:
   - Apply `escapeHTML` to `r.container_name`, `r.prefix`, `r.host`, and headers.
4. **Manual & Automated Verification**:
   - Inject simulated XSS payload `/<script>alert(1)</script>` into incident log and verify safe literal text rendering.

## Acceptance Criteria

- All dynamic data rendered in `public/app.js` is contextually escaped or inserted via `textContent`.
- XSS payloads in incident logs or container names do not execute JavaScript.

## Rationale

Protects administrators from cross-site scripting when viewing real-world attack logs in the control center.

## Constraints

- Zero external JS frameworks; vanilla JavaScript only.
