---
id: TASK-059
type: task
title: Implement Upstream Reverse Proxy Path Rewriting and Subpath Target Preservation
status: completed
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-059
implements:
  - REQ-059
verified_by:
  - TC-059
decided_by:
  - ADR-054
related_to:
  - TASK-003
  - TASK-019
  - TASK-058
---

# TASK-059 - Implement Upstream Reverse Proxy Path Rewriting and Subpath Target Preservation

## Description

Refactor and enhance Toron's reverse proxy path resolution and joining logic in `pkg/proxy/proxy.go` so that upstream targets containing subpaths (such as `http://127.0.0.1:8082/postback`) preserve their target path without unsolicited trailing slash insertion when `strip_prefix: true` is enabled, while strictly respecting client-provided trailing slashes and nested subpath joins.

## Scope & Implementation Breakdown

1. **Proxy Path Resolution Helper (`pkg/proxy/proxy.go`)**:
   - Implement `joinProxyPath(targetPath, reqPath, prefix string, stripPrefix bool) string` helper function.
   - Accurately determine whether `trimmed` relative path is empty or a root slash.
   - For targets with subpaths:
     - Exact prefix without trailing slash (`trimmed == ""`): return `targetPath`.
     - Request with explicit trailing slash (`trimmed == "/"`): preserve trailing slash (`targetPath + "/"` or `targetPath`).
     - Request with nested subpath (`/sub`): join cleanly using `singleJoiningSlash(targetPath, trimmed)`.
   - For root/empty target paths: maintain existing standard behavior returning `/` or trimmed relative path.

2. **ReverseProxy Integration (`pkg/proxy/proxy.go`)**:
   - Replace manual `relPath` manipulation in `ServeHTTPWithPrefix` with `joinProxyPath`.
   - Ensure WebSocket proxying inherits the correct resolved path.

3. **Comprehensive Unit Tests (`pkg/proxy/proxy_test.go`)**:
   - Test subpath upstream target without trailing slash (e.g. `/postback`).
   - Test subpath upstream target with trailing slash (e.g. `/postback/`).
   - Test nested subpath joins under subpath target (e.g. `/postback/status`).
   - Test non-subpath target regression compatibility.
   - Test `strip_prefix: false` with subpath targets.

4. **Production Deployment**:
   - Compile Linux AMD64 binary (`bin/toron-linux-amd64`).
   - Update remote `/etc/toron/routes.yaml` for route 4 (`target: "http://127.0.0.1:8082/postback"`, `strip_prefix: true`).
   - Transfer binary, reload Toron, and verify postback endpoint responds with HTTP 200.
