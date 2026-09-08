# TASK-053: Implement Dynamic Upstream Discovery & Data-Driven Dashboard

## Task Details
- **Requirement**: REQ-053
- **Status**: COMPLETED

## Tasks
1. Refactor `/internal/api/upstreams/health` in `pkg/server/internal_api.go` to dynamically extract all upstream targets from `router.Router` and `cfg.Routes`.
2. Concurrently probe discovered upstreams with bounded timeouts and return live health status in JSON.
3. Remove `const UPSTREAM_SERVICES` from `public/app.js` and render cards dynamically from `/internal/api/upstreams/health`.
4. Refactor `#tester-url` in `public/index.html` and `public/app.js` to dynamically populate all active route prefixes.
5. Bind header version badge, WAF rules count, and anomaly threshold dynamically from `/internal/api/status`.
6. Add unit and regression tests in `pkg/server/server_test.go` or `internal_api_test.go`.
7. Update release notes and documentation.
