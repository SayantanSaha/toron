---
id: TASK-064
type: task
title: Implement Memory Capacity Limits and TTL Eviction in Token Bucket Rate Limiter
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-064
implements:
  - REQ-064
verified_by:
  - TC-064
decided_by:
  - ADR-059
related_to:
  - TASK-022
---

# TASK-064 - Implement Memory Capacity Limits and TTL Eviction in Token Bucket Rate Limiter

## Description

Harden `pkg/router/rate_limiter.go` against memory exhaustion (DoS) and spoofing attacks by capping bucket map capacity, introducing TTL-based state eviction, and deriving client identity strictly from trusted connection properties.

## Scope & Implementation Breakdown

1. **Bounded Storage & LRU / Sweeper (`pkg/router/rate_limiter.go`)**:
   - Add `maxBuckets` limit (default: 10,000) to `RateLimiter`.
   - Implement an eviction mechanism (or periodic background sweeper) to purge buckets that have remained full and idle for longer than `idleTTL` (default: 10 minutes).
   - If capacity is exceeded, evict the oldest idle bucket rather than growing unbounded.

2. **Trusted Client Key Derivation (`pkg/router/rate_limiter.go`)**:
   - Modify `ExtractClientKey` to use physical socket `RemoteAddr` as the default client key.
   - Do not trust client-supplied `X-Forwarded-For` or unvalidated `X-API-Key` headers unless the remote connection IP is in an explicitly configured `TrustedProxies` list.

3. **Lifecycle & Clean Shutdown (`pkg/router/rate_limiter.go`)**:
   - Ensure any background sweeper goroutine is safely stopped when the router or server is shut down.

4. **Testing Verification (`pkg/router/rate_limiter_test.go`)**:
   - Add test validating that inserting 20,000 distinct client keys does not exceed the maximum capacity limit.
   - Add test confirming that spoofed `X-Forwarded-For` headers from untrusted connections share the same physical socket bucket.

## Acceptance Criteria

- `RateLimiter` enforces an upper bound on bucket map entries.
- Stale buckets are evicted automatically.
- Untrusted clients cannot bypass limits by rotating headers.
- Tests pass cleanly with `go test ./pkg/router/...`.

## Rationale

Currently, `buckets map[string]*TokenBucket` grows indefinitely. Attackers rotating randomized headers can crash the server via Out-Of-Memory (OOM) while completely evading rate limits.

## Constraints

- Pure Go stdlib implementation without external caching dependencies.
- High-concurrency read lock performance.

## Open Questions

- What should the default maximum bucket capacity be for low-memory embedded environments?
