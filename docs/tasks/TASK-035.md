---
id: TASK-035
type: task
title: Implement In-Memory Response Caching Engine, Cache-Control Parser, and Diagnostics Headers
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-035

implements:
  - REQ-035

verified_by:
  - TC-035

decided_by:
  - ADR-030

related_to:
  - TASK-003
  - TASK-009
  - TASK-034
---

# TASK-035 - Implement In-Memory Response Caching Engine, Cache-Control Parser, and Diagnostics Headers

## Goal

Create `pkg/router/cache.go` implementing `ResponseCache`, RFC 7234 `Cache-Control` directive parsing, `X-Cache: HIT/MISS` and `Age` header injection, entry TTL expiration, memory bounding, and YAML configuration.

## Sub-tasks

1. Create `pkg/router/cache.go` implementing:
   - `CacheConfig` and `DefaultCacheConfig()`.
   - `ResponseCache` struct with thread-safe `sync.RWMutex` map storage, TTL validation, capacity capping (`max_entries`), and payload size limits (`max_payload_size`).
   - `ParseCacheControl(header string)` extracting `max-age`, `no-store`, `no-cache`, `private`, and `public`.
   - `NewCacheMiddleware(cfg CacheConfig)` wrapping router requests.
2. Add `CacheConfig` struct in `pkg/config/config.go` and include under `ServerConfig.Cache`.
3. Wire `CacheMiddleware` into default router pipeline in `cmd/toron/main.go` when `server.cache.enabled: true`.
4. Update `config.yaml` with commented `cache` section and default settings.
5. Create comprehensive unit tests in `pkg/router/cache_test.go`.
6. Run the complete test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- First request to a cacheable endpoint sets `X-Cache: MISS` and stores the response in cache.
- Subsequent request to the same endpoint returns `X-Cache: HIT`, original payload, and calculated `Age` header.
- `Cache-Control: no-store` prevents caching.
- Request with `Cache-Control: no-cache` fetches fresh response.
- Expired entries are evicted when TTL elapses.
- Full test suite passes cleanly.
