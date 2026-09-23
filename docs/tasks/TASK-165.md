---
id: TASK-165
type: task
title: Implement Defense-in-Depth Authentication and Access Control for Control Center Dashboard and Management APIs
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-23
updated: 2026-09-23

depends_on:
  - REQ-142

derived_from:
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
  - AN-001
---

# TASK-165 - Implement Defense-in-Depth Authentication and Access Control for Control Center Dashboard and Management APIs

## 1. Overview & Objective

Decompose [`REQ-142`](../requirements/REQ-142.md) into concrete, testable work packages to implement defense-in-depth authentication, credential management, and access controls for the Toron Edge Gateway Control Center dashboard ([`public/index.html`](../../public/index.html), [`public/js/`](../../public/js/)) and backing management APIs ([`pkg/server/internal_api.go`](../../pkg/server/internal_api.go)).

The deliverables:
1. Multi-scheme authentication enforcement across `/internal/api/*` supporting Admin Token (`X-Toron-Admin-Key`), Bearer Token (`Authorization: Bearer`), and HTTP Basic Auth.
2. Interactive in-page Admin Authentication Modal in the dashboard SPA triggered by HTTP 401 responses.
3. Automatic credential injection across all telemetry polling (`public/js/api.js`) and mutations (`alerts.js`, `console.js`).
4. Tab-scoped session credential storage (`sessionStorage`) with explicit navigation lock/logout controls.
5. 100% test pass on Go backend tests and Node.js telemetry verification test suite.

---

## 2. Traceability

- **Requirement**: [`REQ-142`](../requirements/REQ-142.md) (Defense-in-Depth Authentication and Access Control for Control Center Dashboard and Management APIs)
- **Architecture Decision**: [`ADR-142`](../architecture/ADR-142.md) (Unified Management API Security & Client Authentication Lifecycle)
- **Verification Plan**: [`TC-142`](../testCases/TC-142.md) (Verification of Dashboard Security & Access Controls)

---

## 3. Work Package Breakdown

### 3.1 WP-1: Backend Management API Security & Challenge Standardization
- **Target**: [`pkg/server/internal_api.go`](../../pkg/server/internal_api.go), [`cmd/toron/main.go`](../../cmd/toron/main.go)
- Ensure all endpoints under `/internal/api/*` validate credentials via `validateAdminAuth`.
- Standardize HTTP 401 responses to return `WWW-Authenticate: Bearer realm="Toron Management", Basic realm="Toron Management"`.
- Support subnet CIDR restrictions returning HTTP 403 Forbidden with structured JSON.
- Verify environment variable `TORON_ADMIN_KEY` automatically activates administrative authentication.

### 3.2 WP-2: Frontend Authentication State & Credential Store
- **Target**: [`public/js/state.js`](../../public/js/state.js)
- Add reactive auth store: `auth: { token: null, authenticated: false, mode: 'live' }`.
- Implement `initAuth()` to hydrate token from `sessionStorage.getItem('toron_admin_key')`.
- Implement `setAuthToken(token)` to save in `sessionStorage` and mark authenticated.
- Implement `clearAuth()` to remove token and lock the session.
- Export `getAuthHeaders()` to construct `X-Toron-Admin-Key` and `Authorization: Bearer` headers.

### 3.3 WP-3: HTTP Interceptor & Automated Credential Injection
- **Target**: [`public/js/api.js`](../../public/js/api.js), [`public/js/views/alerts.js`](../../public/js/views/alerts.js), [`public/js/views/console.js`](../../public/js/views/console.js)
- Create `authenticatedFetch(url, options)` helper wrapping native `fetch()`:
  - Appends headers from `getAuthHeaders()`.
  - Sets `credentials: 'same-origin'` to propagate browser HTTP Basic Auth credentials automatically.
  - Intercepts HTTP 401 responses, pauses interval polling, and invokes the authentication challenge handler.
- Update `fetchBackendData()` in `api.js` to use `authenticatedFetch()`.
- Update ban/unban mutations in `alerts.js` to use `authenticatedFetch()`.
- Update endpoint probe executions in `console.js` to use `authenticatedFetch()`.

### 3.4 WP-4: Interactive Authentication Modal Component
- **Target**: [`public/js/components/authModal.js`](../../public/js/components/authModal.js), [`public/index.html`](../../public/index.html)
- Implement `showAuthModal(onSuccess)` and `hideAuthModal()`:
  - Accessible modal dialog displaying key input field with password masking toggle.
  - Direct probe verification against `/internal/api/status` before dismissing modal.
  - Inline error display on rejection (e.g. `Invalid Admin Key`).
  - "Demo Mode" fallback button for unauthenticated read-only inspection.
- Add modal container markup to [`public/index.html`](../../public/index.html).

### 3.5 WP-5: Navigation Chrome & Session Lock Controls
- **Target**: [`public/js/app.js`](../../public/js/app.js), [`public/index.html`](../../public/index.html)
- Add a session security button to the header toolbar (e.g. lock icon showing "Locked" or "Admin").
- Clicking lock button clears session credentials via `clearAuth()` and presents the auth modal.
- On successful authentication, automatically resume polling and trigger immediate UI refresh.

### 3.6 WP-6: Test Suite & Verification
- **Target**: [`tests/dashboard_telemetry_test.js`](../../tests/dashboard_telemetry_test.js), Go test suites
- Implement TC-142 test suite in `tests/dashboard_telemetry_test.js`:
  - TC-142-01: Rejection of unauthenticated API requests when auth is enabled.
  - TC-142-02: Acceptance of valid Admin Token via `X-Toron-Admin-Key`.
  - TC-142-03: Acceptance of valid Bearer token via `Authorization: Bearer`.
  - TC-142-04: Acceptance of valid Basic Auth credentials.
  - TC-142-05: Subnet CIDR rejection (403 Forbidden).
  - TC-142-06: Frontend 401 interception and auth modal triggering.
  - TC-142-07: Automated credential injection in `authenticatedFetch`.
  - TC-142-08: Session storage scoping and logout clearing.
  - TC-142-09: Strictly relative links invariant across REQ-142, TASK-165, ADR-142, TC-142.
- Verify `go test -race -count=1 ./pkg/server ./pkg/router ./pkg/config`.

---

## 4. Constraints

- Strictly NO external npm runtime dependencies or bundlers.
- Volatile session storage only (`sessionStorage`); NO persistent `localStorage` for tokens.
- Maintain full backward compatibility for development environments when auth is disabled.
- Strictly relative links in all code and documentation.
