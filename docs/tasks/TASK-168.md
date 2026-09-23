---
id: TASK-168
type: task
title: Fix AutoBanManager Lifecycle Desynchronization Across WAF Hot Reloads
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-23
updated: 2026-09-23

depends_on:
  - ../requirements/REQ-144.md

derived_from:
  - ../requirements/REQ-144.md
  - ../analysis/AN-005.md

implements:
  - ../requirements/REQ-144.md

verified_by:
  - ../testCases/TC-144.md

related_to:
  - ../requirements/REQ-144.md
  - ../analysis/AN-005.md
  - ../testCases/TC-144.md
---

# TASK-168 - Fix AutoBanManager Lifecycle Desynchronization Across WAF Hot Reloads

## 1. Overview & Objective

Resolve the split-brain state identified in [`AN-005`](../analysis/AN-005.md) and specified in [`REQ-144`](../requirements/REQ-144.md) where `InternalAPIConfig.AutoBanManager` retains an orphaned pointer following `WAFEngine.Reload()`.

---

## 2. Work Package Breakdown

### WP-1: In-Place AutoBanManager Reconfiguration (`pkg/waf/auto_ban.go`)
- Implement `UpdateConfig(cfg AutoBanConfig, logger *AuditLogger)` on `AutoBanManager`:
  - Dynamically updates thresholds (`MaxViolations`, `Window`, `BanDuration`, `MaxTemporaryBans`).
  - Recompiles whitelist CIDRs / IP addresses.
  - Updates audit logger reference.
  - Retains existing in-memory `bans` and `strikes` maps.

### WP-2: WAF Engine Reload Preservation (`pkg/waf/waf.go`)
- Refactor `WAFEngine.Reload(cfg WAFConfig)`:
  - If `cfg.AutoBan.Enabled` is true and `e.autoBanMgr != nil`, invoke `e.autoBanMgr.UpdateConfig(cfg.AutoBan, logger)` in place without orphaning the instance.
  - If `cfg.AutoBan.Enabled` is true and `e.autoBanMgr == nil`, instantiate `NewAutoBanManager`.
  - If `cfg.AutoBan.Enabled` is false and `e.autoBanMgr != nil`, cleanly close the sweeper and set to nil.

### WP-3: Dynamic API Resolution Provider (`pkg/server/internal_api.go` & `cmd/toron/main.go`)
- Update `InternalAPIConfig`:
  - Add `AutoBanManagerFunc func() *waf.AutoBanManager`.
  - Add accessor `func (c *InternalAPIConfig) GetAutoBanManager() *waf.AutoBanManager`.
  - Replace direct field references with `cfg.GetAutoBanManager()`.
- Update `cmd/toron/main.go`:
  - Pass dynamic closure `AutoBanManagerFunc: func() *waf.AutoBanManager { if globalWafEngine != nil { return globalWafEngine.AutoBanManager() }; return nil }`.

### WP-4: Verification & Automated Tests (`pkg/waf/auto_ban_test.go` & `pkg/server/internal_api_test.go`)
- Add unit test for `AutoBanManager.UpdateConfig`.
- Add test verifying that `WAFEngine.Reload` preserves the `AutoBanManager` pointer and active ban entries.
- Add test verifying `POST /internal/api/security/unban` through `AutoBanManagerFunc` after reload.
