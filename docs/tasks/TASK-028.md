---
id: TASK-028
type: task
title: Implement Route Hot Reloading via fsnotify File Watcher
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-13
updated: 2026-09-08

depends_on:
  - REQ-028

implements:
  - REQ-028

verified_by:
  - TC-028

decided_by:
  - ADR-023

related_to:
  - TASK-003
  - TASK-011
  - TASK-019
---

# TASK-028 - Implement Route Hot Reloading via fsnotify File Watcher

## Goal

Implement `RouteWatcher` using `fsnotify` in `pkg/config/watcher.go`, enable atomic route swapping in `pkg/router/router.go`, and test zero-downtime hot reloading of `routes.yaml`.

## Sub-tasks

1. Add `ReloadRoutesFromFile(path string) ([]ProxyRouteConfig, error)` in `pkg/config/loader.go`.
2. Implement `RouteWatcher` struct in `pkg/config/watcher.go` using `github.com/fsnotify/fsnotify` with event debouncing (e.g. 100ms timer).
3. Add `ClearRoutes()` or `UpdateRoutes(newRoutes)` support in `pkg/router/router.go` for thread-safe atomic route swapping.
4. Integrate `RouteWatcher` lifecycle into `pkg/server/server.go` when server starts.
5. Write unit tests in `pkg/config/watcher_test.go` and `pkg/router/router_test.go` verifying dynamic route reloading.
6. Run full test suite `go test ./...` to verify zero-regression execution.

## Acceptance Criteria

- File changes to `routes.yaml` are detected and debounced automatically.
- Invalid YAML updates log a warning and retain the current active route table without crashing or clearing existing routes.
- Valid YAML updates reload static routes and upstream reverse proxy targets atomically.
- All Go unit tests pass cleanly.
