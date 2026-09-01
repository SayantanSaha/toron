# TASK-052: Implement Reverse Proxy Prefix Management & Redirect/Cookie Rewriting

## Task Details
- **Requirement**: REQ-052
- **Status**: COMPLETED

## Tasks
1. Extend `ProxyRouteConfig` in `pkg/config/config.go` with `StripPrefix`, `RewriteRedirects`, and `RewriteCookiePath` fields and helper getter methods.
2. Extend `ProxyOptions` and `ReverseProxy` in `pkg/proxy/proxy.go` with `StripPrefix`, `RewriteRedirects`, and `RewriteCookiePath`.
3. Update `cmd/toron/main.go` to pass route options into `proxy.ProxyOptions`.
4. Implement `X-Forwarded-Prefix` injection in `pkg/proxy/proxy.go` for HTTP and WebSocket upgrade proxy requests.
5. Implement `RewriteRedirectLocation` for relative, internal backend absolute, and external third-party URL handling.
6. Implement `RewriteCookiePath` for `Set-Cookie: Path=` rewriting.
7. Add unit and integration test suite in `pkg/proxy/proxy_test.go`.
8. Implement `toron.strip_prefix`, `toron.rewrite_redirects`, and `toron.rewrite_cookie_path` label parsing in `pkg/discovery/parser.go`, `provider.go`, and `manager.go` with tests in `discovery_test.go`.
9. Update wiki reference documentation in `docs/wiki/features/reverse-proxy.md`, `docs/wiki/features/oci-container-auto-discovery.md`, and `docs/wiki/reference/config-options.md`.
