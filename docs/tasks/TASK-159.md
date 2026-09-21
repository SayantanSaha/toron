---
id: TASK-159
type: task
title: Implementation of Native Dynamic 2-Stage WAF Auto-Ban Engine with Persistent Storage & OS-Level IP Banning
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-21
updated: 2026-09-21

depends_on:
  - REQ-136

derived_from:
  - REQ-136

implements:
  - REQ-136

verified_by:
  - TC-136

decided_by:
  - ADR-136

related_to:
  - REQ-136
  - ADR-136
  - TC-136
  - REQ-059
  - TASK-075
---

# TASK-159 - Implementation of Native Dynamic 2-Stage WAF Auto-Ban Engine with Persistent Storage & OS-Level IP Banning

## 1. Overview & Objective

Decompose [`REQ-136`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-136.md) into concrete engineering deliverables:
1. **Config Schema Extension (`pkg/config/config.go`)**:
   - Define `AutoBanConfig` struct under `WAFConfig`: `Enabled`, `MaxViolations`, `Window` (duration), `BanDuration` (duration), `MaxTemporaryBans`, `PersistenceFile`, `Whitelist` ([]string).
   - Set sensible defaults (`Enabled: false`, `MaxViolations: 5`, `Window: 1m`, `BanDuration: 1h`, `MaxTemporaryBans: 3`, `PersistenceFile: "/etc/toron/banned_ips.json"`).
2. **Two-Stage Auto-Ban Engine (`pkg/waf/auto_ban.go`)**:
   - Implement `AutoBanManager` with concurrency-safe strike tracking, sliding time window violation counting, Stage 1 temporary ban expiration, Stage 2 permanent ban escalation, CIDR whitelist verification, and periodic expired-ban cleanup.
   - Implement atomic JSON file persistence (`.tmp` write followed by `os.Rename`) for both temporary and permanent bans so ban states survive process restarts.
   - Implement fast-path $\mathcal{O}(1)$ `IsBanned(ip string) (bool, *BanEntry)` check.
3. **WAF Engine Integration (`pkg/waf/waf.go`)**:
   - Embed `AutoBanManager` inside `Engine`.
   - Fast-path check at start of `Inspect` / `InspectToron` returning immediate 403 when client IP is banned.
   - On WAF anomaly violation (`score >= AnomalyThreshold` or blocked action), record violation in `AutoBanManager`.
4. **Management APIs & Observability Dashboard (`pkg/server/internal_api.go`, `public/app.js`, `public/index.html`)**:
   - Expose `GET /internal/api/security/banned-ips`, `POST /internal/api/security/unban`, and `POST /internal/api/security/ban`.
   - Integrate "Banned IPs & Threat Defense" table in dashboard Alerts/Security view with 1-click Unban and Manual Ban modals.
5. **OS-Level IP Blocking Documentation & Server Configuration**:
   - Create `docs/wiki/security/os-level-ip-blocking.md` covering Fail2ban on Linux, `pfctl` on macOS, and PowerShell on Windows.
   - Deploy Fail2ban jail on `sayantansaha.in` reading `/var/log/toron/security.log`.

---

## 2. Acceptance Criteria

- [ ] `AutoBanManager` correctly tracks strikes within configured sliding window.
- [ ] Accumulating `MaxViolations` within `Window` triggers a Stage 1 temporary ban for `BanDuration`.
- [ ] Accumulating `MaxTemporaryBans` temporary bans escalates the IP to a Stage 2 permanent ban.
- [ ] Banned IPs are rejected in $\mathcal{O}(1)$ time before request body or regex evaluation.
- [ ] Active and permanent bans are atomically saved to `persistence_file` and restored on startup.
- [ ] Whitelisted CIDRs (`127.0.0.1/32`, `::1/128`, etc.) are never banned.
- [ ] `GET /internal/api/security/banned-ips` lists all active bans with TTL and ban type.
- [ ] `POST /internal/api/security/unban` removes ban from memory and disk persistence.
- [ ] Dashboard displays active bans with 1-click unban button.
- [ ] Complete OS-level blocking documentation created and Fail2ban jail deployed on remote server.
- [ ] All tests pass with race detector enabled (`go test -race ./pkg/waf/... ./pkg/server/...`).

---

## 3. Constraints & Dependencies

- Zero external CGO dependencies; pure standard library Go.
- Atomic state file persistence to avoid state corruption during power failure or SIGKILL.
- Sub-microsecond fast-path IP check without heap allocations.
