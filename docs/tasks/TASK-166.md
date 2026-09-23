---
id: TASK-166
type: task
title: Suppress Browser HTTP Basic Auth Dialog by Scoping Management API Challenge to Bearer
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-23
updated: 2026-09-23

depends_on:
  - TASK-165
  - REQ-142

derived_from:
  - ../analysis/AN-002.md
  - REQ-142

implements:
  - REQ-142

verified_by:
  - TC-142

decided_by:
  - ADR-142

related_to:
  - REQ-142
  - ADR-142
  - TC-142
  - ../analysis/AN-002.md
---

# TASK-166 - Suppress Browser HTTP Basic Auth Dialog by Scoping Management API Challenge to Bearer

## 1. Overview & Objective

Address [`AN-002`](../analysis/AN-002.md) by adjusting the `WWW-Authenticate` challenge header emitted by the Toron Management API security middleware in [`pkg/server/internal_api.go`](../../pkg/server/internal_api.go).

By restricting the unauthenticated challenge to `Bearer realm="Toron Management"`, web browsers will not invoke their native Username/Password modal dialog when unauthenticated SPA requests to `/internal/api/*` return HTTP 401. Instead, the 401 response is passed directly to the SPA's `authenticatedFetch` handler in [`public/js/api.js`](../../public/js/api.js), activating the custom in-page modal dialog in [`public/js/components/authModal.js`](../../public/js/components/authModal.js).

---

## 2. Work Breakdown

- **WP-1**: Update `pkg/server/internal_api.go` to emit `res.Header.Set("WWW-Authenticate", `Bearer realm="Toron Management"`)` when unauthenticated.
- **WP-2**: Ensure `validateAdminAuth()` retains full backward compatibility for `X-Toron-Admin-Key`, Bearer tokens, and Basic auth credentials when provided by clients.
- **WP-3**: Update test assertions in `pkg/server/internal_api_test.go` and verify all Go tests pass with `-race`.
- **WP-4**: Rebuild Linux AMD64 binary and deploy to `sayantansaha.in`.
- **WP-5**: Verify on live server that browser loads dashboard without native browser prompt and shows in-page auth modal.
