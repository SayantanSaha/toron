---
id: TASK-033
type: task
title: Implement ACME Engine, HTTP-01/TLS-ALPN-01 Responders, and Certificate Caching
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-14
updated: 2026-09-08

depends_on:
  - REQ-033

implements:
  - REQ-033

verified_by:
  - TC-033

decided_by:
  - ADR-028

related_to:
  - TASK-010
  - TASK-018
---

# TASK-033 - Implement ACME Engine, HTTP-01/TLS-ALPN-01 Responders, and Certificate Caching

## Goal

Create `pkg/acme/acme.go` implementing `ACMEManager`, HTTP-01 challenge routing, TLS-ALPN-01 ALPN negotiation, disk key/cert storage, and integration with `pkg/server` and `pkg/config`.

## Sub-tasks

1. Create `pkg/acme/acme.go` implementing `ACMEManager` with `HTTP-01` token authorization resolver and `TLS-ALPN-01` certificate generator.
2. Add `ACMEConfig` fields in `pkg/config/config.go` (`enabled`, `directory_url`, `email`, `domains`, `cache_dir`, `challenge_type`).
3. Add `RegisterACMEHTTP01` route handler in `pkg/router/router.go` for `/.well-known/acme-challenge/*`.
4. Integrate `ACMEManager.GetCertificate()` callback into `pkg/server/server.go` TLS configuration.
5. Write unit tests in `pkg/acme/acme_test.go`.
6. Run full test suite `go test ./...` to verify clean execution.

## Acceptance Criteria

- HTTP-01 challenge handler responds to `/.well-known/acme-challenge/{token}` with correct key authorization thumbprint.
- TLS-ALPN-01 handler negotiates `acme-tls/1` ALPN and presents ACME challenge certificate.
- Certificates are cached on disk in `cache_dir`.
- All Go unit tests pass cleanly.
