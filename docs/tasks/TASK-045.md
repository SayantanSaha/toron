---
id: TASK-045
type: task
title: Implement Custom WAF Regex Rules and Zero-Downtime Hot Reloading
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-15
updated: 2026-08-15

depends_on:
  - REQ-045

implements:
  - REQ-045

verified_by:
  - TC-045

decided_by:
  - ADR-040

related_to:
  - TASK-028
  - TASK-041
  - TASK-043
  - TASK-044
---

# TASK-045 - Implement Custom WAF Regex Rules and Zero-Downtime Hot Reloading

## Goal

Extend the Toron WAF subsystem (`pkg/waf`) and configuration engine (`pkg/config`) to support user-defined regex rules in YAML and zero-downtime hot reloading of server configuration and WAF rule sets via background `fsnotify` file watching.

## Sub-tasks

1. **Extend WAF Rule Models (`pkg/waf/waf.go`)**:
   - Define `CustomRuleConfig` struct with YAML/JSON tags (`id`, `category`, `description`, `pattern`, `score`, `locations`).
   - Add `CategoryBot` and `CategoryCustom` constants to `WAFCategory`.
   - Add `CustomRules []CustomRuleConfig` to `WAFConfig`.
   - Implement `compileCustomRule(c CustomRuleConfig) (WAFRule, error)` to parse location bitmasks (`url`, `path`, `query`, `headers`, `body`) and compile regular expressions.
   - Update `NewEngine(cfg WAFConfig)` to compile and include custom rules alongside default OWASP rules (filtering any disabled rules).

2. **Atomic Rule Swapping & Engine Reloading (`pkg/waf/waf.go`)**:
   - Implement `(e *WAFEngine) Reload(cfg WAFConfig) error` on `WAFEngine`.
   - Ensure `Reload` validates and compiles new custom rules, IP ACLs, and disabled rules maps before acquiring `e.mu.Lock()`.
   - Atomically replace `e.config`, `e.rules`, `e.ipACL`, and audit logger settings under write lock.

3. **Extend Server Configuration Watcher (`pkg/config/watcher.go`)**:
   - Implement `ConfigWatcher` using `fsnotify` with 100ms debouncing, monitoring `config.yaml`.
   - On file write/create/rename events, parse updated server configuration, validate syntax via `ValidateConfig`, and invoke the `onReload` callback.
   - Retain active server state and log clear warnings if an invalid configuration is saved.

4. **Update Configuration Validation & Defaults (`pkg/config/config.go`)**:
   - Add validation for `custom_rules` in `ValidateConfig` (ensuring non-empty rule ID, valid location names, non-negative scores, and valid regex patterns).
   - Ensure both global `server.waf.custom_rules` and route-level `proxy.routes[i].waf.custom_rules` are supported.

5. **Integrate Hot Reloading in Entrypoint (`cmd/toron/main.go`)**:
   - Initialize `ConfigWatcher` when `configPath` is provided or `config.yaml` exists.
   - Connect the watcher callback to atomically reload the active `WAFEngine` instance(s).

6. **Configuration Examples (`config.yaml` & `routes.yaml`)**:
   - Add sample user-defined custom WAF regex rule (`CUSTOM-001` for malicious scanners/bots like sqlmap, nikto, acunetix) to `config.yaml` and `routes.yaml`.

7. **Unit & Integration Tests**:
   - `pkg/waf/custom_rules_test.go`: Test custom regex matching across headers, path, query, and body; location filtering; anomaly score accumulation.
   - `pkg/waf/reload_test.go`: Test thread-safe `Reload()` under concurrent `Inspect()` / `InspectToron()` traffic without data races.
   - `pkg/config/config_watcher_test.go`: Test `ConfigWatcher` file modification triggers and dynamic hot reloading of custom rules.
   - Run full regression test suite `go test -race ./...`.

## Acceptance Criteria

- Custom regex rules defined in YAML successfully compile and evaluate during WAF inspection.
- Malicious requests matching custom rules trigger the appropriate threat score and action (`403 Forbidden` in enforce mode).
- `WAFEngine.Reload()` atomically swaps active rules with zero data races under `-race`.
- `ConfigWatcher` detects file modifications to `config.yaml` and executes atomic reload without connection drops.
- 100% Go unit test pass rate across all packages.
