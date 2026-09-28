---
id: TASK-174
type: task
title: Alerts and Threat Defense Control Center Redesign (Deduplication, Search, Pagination, Incident Drawer, and Metrics)
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-28
updated: 2026-09-28

depends_on:
  - ../requirements/REQ-146.md

derived_from:
  - ../requirements/REQ-146.md
  - ../analysis/AN-007.md
  - ../../PRD.md

implements:
  - ../requirements/REQ-146.md

verified_by:
  - ../testCases/TC-146.md

decided_by:
  - ../architecture/ADR-146.md

related_to:
  - ../requirements/REQ-146.md
  - ../analysis/AN-007.md
  - ../requirements/REQ-137.md
  - ../requirements/REQ-138.md
  - ../requirements/REQ-141.md
  - ../architecture/ADR-138.md
  - ../architecture/ADR-146.md
  - ../testCases/TC-146.md
---

# TASK-174 - Alerts and Threat Defense Control Center Redesign

## 1. Overview & Objective

Decompose the approved requirement specification [`REQ-146`](../requirements/REQ-146.md) and architectural analysis [`AN-007`](../analysis/AN-007.md) into concrete, implementation-ready frontend engineering deliverables.

The Toron Control Center (`#/alerts`) is the primary operations cockpit for real-time edge security and threat defense. Under active traffic workloads, WAF anomalies and automated IP bans currently suffer from six structural defects:
1. **Data Model Duplication**: Banned IPs are injected into both `D.alerts` and `D.bannedIps` in [`public/js/model.js`](../../public/js/model.js), causing every banned IP to render twice on screen and falsely inflating the navigation alert badge and Overview health banner.
2. **Missing Pagination & Data Filtering**: Unpaginated rendering of incident feeds and ban lists causes unbounded DOM growth, excessive vertical scrolling, and lack of search or category filtering.
3. **Responsive Header Layout Collisions**: The manual ban form inside `.card-h` wraps and breaks alignment on screens narrower than 1200px.
4. **Thread-Blocking Synchronous Dialogs**: Unban operations trigger `window.confirm()` and `window.alert()`, freezing the browser execution loop, stopping background telemetry polling, and preventing headless automation.
5. **Inaccessible Forensic Telemetry**: Backend WAF audit metadata (OWASP rule ID, anomaly score, matched parameter location, and payload snippet) is dropped by the drawer controller ([`public/js/components/drawer.js`](../../public/js/components/drawer.js)), which only supports routes and HTTP request logs.
6. **Absence of High-Level Operational Metrics**: Operators lack an executive Key Performance Indicator (KPI) strip providing situational awareness of active threats, WAF block counts, and ban tier distribution.

This task redesigns the Alerts & Threat Defense view into a high-density, forensic-grade security cockpit strictly using native ES Modules and CSS with zero external dependencies.

---

## 2. Traceability

- **Requirement**: [`REQ-146`](../requirements/REQ-146.md) (FR-146-1 through FR-146-7, NFR-146-1 through NFR-146-5, AC-146-01 through AC-146-08)
- **Analysis**: [`AN-007`](../analysis/AN-007.md) (Sections 3, 5, 7, 8, 9 Option 1, 10)
- **Architecture**: [`ADR-146`](../architecture/ADR-146.md), [`ADR-138`](../architecture/ADR-138.md)
- **Core Principles**: [`PRD.md`](../../PRD.md)
- **Related Requirements**:
  - [`REQ-137`](../requirements/REQ-137.md): Full timestamp calendar formatting (`YYYY-MM-DD HH:MM:SS`) and chronological ordering
  - [`REQ-138`](../requirements/REQ-138.md): Multi-level deterministic column sorting and IP tie-breaking
  - [`REQ-141`](../requirements/REQ-141.md): Native ES module modular architecture with zero external dependencies
- **Verification Target**: [`docs/testCases/TC-146.md`](../testCases/TC-146.md), [`tests/dashboard_telemetry_test.js`](../../tests/dashboard_telemetry_test.js)

---

## 3. Work Package Breakdown

### WP-1: Data Model Normalization and Deduplication ([`public/js/model.js`](../../public/js/model.js))

Isolate operational alerts, WAF incidents, and active IP firewall bans into distinct properties on computed data model `D`:
1. **Deduplicate Banned IPs**:
   - Remove the `(rawApiBannedIps || []).forEach` block that pushes banned IPs into `alerts`.
   - Ensure banned IPs exist strictly in `D.bannedIps` (`rawApiBannedIps || []`).
2. **Dedicated Incidents Collection**:
   - Provide `D.incidents = rawApiIncidents || []`.
   - Ensure each incident record preserves raw forensic fields: `timestamp`, `event`, `client_ip`, `method`, `path`, `category`, `rule_id`, `anomaly_score`, `action`, `location`, `payload_snippet`.
3. **Operational Alerts Isolation**:
   - `D.alerts` shall strictly contain:
     - Upstream instance health check failures (`upstream_*`).
     - Route 5xx error rate spikes exceeding 2% threshold (`route_*`).
     - Failing ACME SSL certificate renewals (`cert_*`).
     - Active unmitigated operational anomalies.
   - Maintain newest-first chronological sort with secondary IP/ID tie-breaker.
4. **Navigation Badge & Overview Parity**:
   - Verify `#nav button[data-nav="alerts"] .badge` in [`public/js/app.js`](../../public/js/app.js) and the status banner in [`public/js/views/overview.js`](../../public/js/views/overview.js) reflect only true unresolved alerts (`D.alerts.length`), eliminating false high badge counts from mitigated/quarantined actors.

### WP-2: Application State Extensions ([`public/js/state.js`](../../public/js/state.js))

Extend the centralized reactive dashboard state in [`public/js/state.js`](../../public/js/state.js) with alerts and threat defense view parameters:
```javascript
export const state = {
  // ... existing properties ...
  // Alerts & Threat Defense State
  alIncQ: '',             // Incident search query string
  alBanQ: '',             // Banned IPs search query string
  alSev: 'all',           // Severity filter: 'all' | 'critical' | 'warning'
  alTier: 'all',          // Ban tier filter: 'all' | 'temporary' | 'permanent'
  alBanSort: 'created_at',// Sort column for Banned IPs table
  alBanSortDir: 'desc',   // Sort direction: 'desc' | 'asc'
  alIncPage: 1,           // Incidents active page index (1-based)
  alIncPageSize: 10,      // Incidents page size: 10 | 25 | 50 (default: 10)
  alBanPage: 1,           // Banned IPs active page index (1-based)
  alBanPageSize: 25       // Banned IPs page size: 10 | 25 | 50 (default: 25)
};
```

### WP-3: Alerts & Threat Defense UI Redesign ([`public/js/views/alerts.js`](../../public/js/views/alerts.js))

Completely rebuild [`public/js/views/alerts.js`](../../public/js/views/alerts.js) implementing:
1. **Executive 4-Card Security KPI Metrics Strip**:
   - Top-level grid displaying 4 executive signal cards:
     - **Card 1: Active Incidents / Alerts**: Total active operational alerts and unmitigated incidents (`D.alerts.length + D.incidents.length`).
     - **Card 2: Recent WAF Blocks**: Total threats intercepted and dropped (`action === 'blocked'`) from telemetry window.
     - **Card 3: Stage 1 Temp Bans**: Count of IP addresses undergoing 1-hour quarantine (`type === 'temporary'`).
     - **Card 4: Stage 2 Permanent Bans**: Count of repeat offenders permanently blocked (`type === 'permanent'`).
   - Tone badges matching Toron design system (`var(--ok)`, `var(--warn)`, `var(--c5)`).
2. **Unified Search & Facet Filter Bar**:
   - Search input (`<input type="search">`) filtering records in real time:
     - For Incidents: matches `client_ip`, `rule_id`, `path`, `category`, `payload_snippet`.
     - For Banned IPs: matches `ip`, `reason`, `last_category`.
     - Case-insensitive substring matching; typing automatically resets active page to 1.
   - Filter chips:
     - Severity: `All`, `Critical`, `Warning`.
     - Ban Tier: `All`, `Stage 1 (1h Temp)`, `Stage 2 (Permanent)`.
3. **Client-Side Non-Blocking Pagination**:
   - Implemented for both the Incidents list and the Banned IPs table:
     - Configurable page sizes: `10`, `25`, `50` records per page.
     - Controls: First (`«`), Previous (`‹`), Page indicator (`Page X / Y`), Next (`›`), Last (`»`), Page size dropdown.
     - Status summary: `Showing start–end of total items`.
     - Clean boundary enforcement: First/Prev disabled on page 1, Next/Last disabled on last page.
4. **Deterministic Multi-Level Interactive Column Sorting**:
   - Table columns: `Client IP`, `Ban Tier`, `Created At`, `Temp Bans`, `Reason / Category`, `TTL / Expiry`, `Action`.
   - Clicking column header (`<th>`) toggles `asc` / `desc` with directional arrows (`▲`/`▼`).
   - Tie-breaking on identical values uses secondary IP collation (`ip.localeCompare(..., { numeric: true })`) per [`REQ-138`](../requirements/REQ-138.md) and [`ADR-138`](../architecture/ADR-138.md).
5. **Decoupled Ergonomic Manual Ban Toolbar / Modal**:
   - Decoupled from table card header into a clean dedicated management toolbar.
   - Input fields:
     - **Client IP**: Text input with client-side syntax verification for valid IPv4 (dotted-decimal `0-255.0-255.0-255.0-255`) or IPv6 (hexadecimal colon notation). Inline validation message prevents invalid network dispatches.
     - **Ban Tier & Preset Duration**: Quick preset dropdown (`15m`, `1h`, `6h`, `24h`, `7d`, `Permanent`) plus custom duration text input.
     - **Reason**: Description context input (e.g. `Credential stuffing on /v1/auth`).
   - Submits `POST /internal/api/security/ban` with payload `{ ip, type, duration, reason }`. Form clears on success and invokes `fetchBackendData()`.
6. **Non-Blocking In-App Unban Confirmation Modal & Toast Feedback**:
   - Zero calls to `window.confirm()` or `window.alert()`.
   - Clicking `Unban` displays an in-app confirmation modal card (leveraging `#scrim` or dedicated `.modal-scrim`) detailing target IP, ban tier, and confirmation prompt.
   - Confirming unban dispatches `POST /internal/api/security/unban` with payload `{ ip }`.
   - Displays non-blocking floating toast notification for operation success or backend error with auto-dismiss after 4 seconds.

### WP-4: Incident Investigation Drawer Integration ([`public/js/components/drawer.js`](../../public/js/components/drawer.js))

Extend [`public/js/components/drawer.js`](../../public/js/components/drawer.js) to support `kind === 'incident'`:
1. **Drawer Invocation**:
   - Clicking any incident row or list item opens `#drawer` with `kind = 'incident'` and incident `id` or index.
2. **Forensic Telemetry Display**:
   - **Header**: Incident title, rule category, severity pill badge (`blocked` vs `logged`).
   - **Forensic Attributes Table / KV List**:
     - OWASP/Custom Rule ID (`rule_id`).
     - Anomaly Score (`anomaly_score`).
     - Action Taken (`blocked` vs `logged`).
     - HTTP Method and Target Path (`method`, `path`).
     - Offending Client IP (`client_ip`) with convenient copy action button.
     - Parameter Location (`location`, e.g. `query`, `header`, `body`, `cookie`).
     - Timestamp (`timestamp` in calendar format `YYYY-MM-DD HH:MM:SS` and relative elapsed time).
   - **Attack Payload Snippet**:
     - Rendered inside `<pre class="cs-pre">` with horizontal scrolling for inspectable attack signatures.
3. **Integrated Direct Quick Actions**:
   - **"Ban Client IP"**: Opens the manual ban form pre-filled with the incident's `client_ip`, recommended tier (`temporary` or `permanent`), and rule-derived `reason`.
   - **"Filter Logs for IP"**: Sets `state.lq = client_ip`, navigates to `#/logs`, and displays matching request traces.

### WP-5: Design System & Responsive Styles ([`public/style.css`](../../public/style.css))

Add clean, responsive CSS rules adhering to Toron's design system:
1. **Security KPI Metrics Grid**:
   - `.al-kpi-grid`: Responsive 4-column grid (`repeat(auto-fit, minmax(200px, 1fr))`) with gap spacing.
   - `.al-kpi-card`: Metric cards with value typography, sublabel, and tone indicators.
2. **Toolbar & Filter Chips**:
   - `.al-toolbar`: Flexbox toolbar with search input, chips, and action buttons.
   - Responsive wrap and spacing down to mobile viewports.
3. **Pagination & Footer Controls**:
   - `.al-foot-bar`: Flex container matching `.lg-foot-bar` with item summary and `.pagination` button controls.
4. **Manual Ban Form & Modal Styling**:
   - `.al-ban-box`: Dedicated action panel with clean grid inputs.
   - `.al-confirm-modal`: Modal card overlay for unban confirmation.
   - `.al-toast`: Non-blocking floating toast notification container (`position: fixed; bottom: 20px; right: 20px; z-index: 100`).
5. **Drawer Payload Viewer**:
   - Styling for syntax block `<pre class="cs-pre">` with monospace font, overflow-x scroll, and dark/light mode surface colors.

### WP-6: Automated Verification & Test Suite ([`tests/dashboard_telemetry_test.js`](../../tests/dashboard_telemetry_test.js))

Create automated verification tests in [`tests/dashboard_telemetry_test.js`](../../tests/dashboard_telemetry_test.js) (and test specification [`docs/testCases/TC-146.md`](../testCases/TC-146.md)):
1. **Data Model Deduplication Test**:
   - Verify `D.bannedIps` entries are NOT present in `D.alerts`.
   - Verify `D.incidents` contains structured WAF events.
   - Verify navigation badge count equals `D.alerts.length` and is not inflated by banned IPs.
2. **KPI Calculation Parity**:
   - Verify 4 KPI values match active alert counts, blocked WAF incidents, temporary bans, and permanent bans.
3. **Client-Side Pagination Verification**:
   - Test 45 banned IP records with page size 10; verify 5 pages, boundary button disablement, and correct row slicing.
4. **Real-Time Search & Facet Filtering**:
   - Test search query matching across IP, rule ID, path, category, and payload.
   - Test severity filter and ban tier filter chips.
5. **Deterministic Multi-Level Interactive Sorting**:
   - Test column sorting on IP, Created At, TTL, Temp Bans, and Reason.
   - Test deterministic secondary collation on IP address for ties.
6. **Decoupled Ban Form & Syntax Validation**:
   - Verify IPv4 and IPv6 format validation rejects invalid addresses.
   - Verify duration preset selection and JSON payload construction.
7. **Non-Blocking Unban Modal & Absence of Window.Confirm**:
   - Verify zero occurrences of `window.confirm` or `window.alert` in alerts view code.
   - Verify modal confirm trigger dispatches unban and triggers toast feedback.
8. **Forensic Incident Drawer Verification**:
   - Verify `openDrawer('incident', id)` renders rule ID, anomaly score, method, path, client IP, location, and payload snippet.
9. **Zero Dependencies & Footprint Budget**:
   - Verify total frontend JS footprint remains $\le 100\text{ KB}$.
   - Verify zero npm packages or build tools.
10. **Strictly Relative Links**:
    - Verify all markdown and source references strictly use relative links.

---

## 4. Acceptance Criteria

- [ ] **AC-174-1 (Data Model Normalization & Deduplication)**: Banned IPs from `/internal/api/security/banned-ips` are strictly isolated in `D.bannedIps` and never pushed into `D.alerts`. Alerts navigation badge and Overview status banner count only true unresolved operational alerts and unmitigated incidents (adhering to REQ-146 AC-146-01).
- [ ] **AC-174-2 (Executive Security KPI Metrics Strip)**: Top of `#/alerts` renders a 4-card KPI strip displaying Active Incidents/Alerts, Recent WAF Blocks, Stage 1 Temp Bans, and Stage 2 Permanent Bans with correct numeric values and tone indicators (adhering to REQ-146 AC-146-08).
- [ ] **AC-174-3 (Unified Multi-Attribute Search & Facet Filtering)**: Real-time search matches IP, rule ID, category, path, and payload for incidents, and IP, reason, and category for banned IPs. Filter chips allow filtering by Severity (`All`, `Critical`, `Warning`) and Ban Tier (`All`, `Stage 1`, `Stage 2`). Filtering resets pagination to Page 1 (adhering to REQ-146 AC-146-03).
- [ ] **AC-174-4 (Client-Side Pagination)**: Both incidents feed and banned IPs table support pagination with configurable page sizes (10, 25, 50), first/prev/next/last navigation, page indicators (`Page X / Y`), and summary text (`Showing start–end of total`) (adhering to REQ-146 AC-146-02).
- [ ] **AC-174-5 (Deterministic Multi-Level Interactive Column Sorting)**: Banned IPs table headers support interactive toggling between ascending (`▲`) and descending (`▼`) sorting. Records with identical sort keys break ties deterministically on `ip` ascending (`localeCompare` with `{ numeric: true }`) (adhering to REQ-146 AC-146-04).
- [ ] **AC-174-6 (Decoupled Ergonomic Manual Ban Form)**: Manual IP ban form is decoupled from `.card-h` into a dedicated toolbar with IPv4/IPv6 client-side syntax validation, preset durations (`15m`, `1h`, `6h`, `24h`, `7d`, `Permanent`), custom duration input, and reason description field (adhering to REQ-146 AC-146-05).
- [ ] **AC-174-7 (Non-Blocking In-App Confirmation & Toast Feedback)**: Zero calls to `window.confirm()` or `window.alert()`. Unban operations trigger a non-blocking in-app confirmation dialog/modal. Background telemetry continues without pause. Success and error states trigger non-blocking toast notifications (adhering to REQ-146 AC-146-06).
- [ ] **AC-174-8 (Forensic Incident Investigation Drawer)**: Clicking any incident opens `#drawer` with `kind === 'incident'`, displaying OWASP Rule ID, Anomaly Score, Action taken, Parameter Location, HTTP Method, Target Path, Client IP, Full Datetime (`YYYY-MM-DD HH:MM:SS`), and raw Attack Payload Snippet in `<pre class="cs-pre">`. Includes quick actions to "Ban Client IP" and "Filter Logs for IP" (adhering to REQ-146 AC-146-07).
- [ ] **AC-174-9 (Performance & Zero External Dependencies)**: Pagination, search, and sorting execute in $< 16\text{ ms}$ for 2,000 ban records and 500 incidents. All code uses native vanilla ES Modules with total JS footprint $\le 100\text{ KB}$ and zero external npm/CDN dependencies (adhering to NFR-146-1 and NFR-146-2).
- [ ] **AC-174-10 (Strictly Relative Links Invariant)**: All documentation and code links strictly use relative paths (`../requirements/REQ-146.md`, `../../public/js/model.js`). No absolute filesystem or machine URIs exist.

---

## 5. Scope

### Include
- Normalization of computed data model `D` in [`public/js/model.js`](../../public/js/model.js) to isolate `D.alerts`, `D.incidents`, and `D.bannedIps`.
- Complete UI redesign of [`public/js/views/alerts.js`](../../public/js/views/alerts.js) with 4-card KPI strip, search bar, filter chips, pagination, sortable headers, decoupled ban form, in-app unban modal, and toast alerts.
- Extension of [`public/js/components/drawer.js`](../../public/js/components/drawer.js) with `kind === 'incident'` forensic breakdown and 1-click ban/logs actions.
- Addition of alerts view state properties to [`public/js/state.js`](../../public/js/state.js).
- Styling additions in [`public/style.css`](../../public/style.css) for KPI cards, toolbars, drawer payload container, and modal dialogs.
- Creation of automated test cases in [`tests/dashboard_telemetry_test.js`](../../tests/dashboard_telemetry_test.js) and test document [`docs/testCases/TC-146.md`](../testCases/TC-146.md).

### Exclude
- Backend REST API schema changes in `pkg/server/internal_api.go` (existing endpoints `/internal/api/security/incidents`, `/internal/api/security/banned-ips`, `/internal/api/security/ban`, and `/internal/api/security/unban` remain unchanged).
- WAF engine rule modifications or core regex rule adjustments in `pkg/waf/`.
- Addition of third-party build tools, bundlers, or frontend npm packages.

---

## 6. Constraints & Standards

1. **Zero External Frontend Dependencies**: Pure vanilla JavaScript (ES2022+) and native CSS3. No frameworks, build steps, or CDNs permitted.
2. **Strictly Relative Links**: All document and code references must strictly use relative links. Absolute file paths and machine URIs are forbidden.
3. **No UI Freezing**: Synchronous `window.confirm()` and `window.alert()` are strictly prohibited.
4. **Footprint Budget**: Total uncompressed JavaScript footprint under `public/js/` must not exceed 100 KB.
5. **Deterministic Collation**: Sorting tie-breaking must strictly adhere to [`REQ-138`](../requirements/REQ-138.md) and [`ADR-138`](../architecture/ADR-138.md) using numeric IP collation.
6. **Full Datetime Formatting**: All timestamps must display full calendar format `YYYY-MM-DD HH:MM:SS` adhering to [`REQ-137`](../requirements/REQ-137.md).

---

## 7. Open Questions

1. **Incident Retention Horizon**: The in-memory `AuditLogger` ring buffer stores the 50 most recent security events. Historical investigation beyond 50 events is accessible via server disk logs and stdout telemetry sinks.
2. **Auto-Ban Duration Presets**: Presets (`15m`, `1h`, `6h`, `24h`, `7d`, `Permanent`) cover $> 99\%$ of edge operations; custom duration input handles non-standard intervals.
