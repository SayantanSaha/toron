---
id: TASK-023
type: task
title: Implement HTTPS TLS Encryption, ALPN Negotiation, and Dev Cert Generator
status: active
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-023

implements:
  - REQ-023

verified_by:
  - TC-023

decided_by:
  - ADR-018

related_to:
  - TASK-001
  - TASK-022
---

# TASK-023 - Implement HTTPS TLS Encryption, ALPN Negotiation, and Dev Cert Generator

## Goal

Add TLS configuration options, X.509 cert loader, self-signed dev certificate generator, HTTPS server listener, ALPN ALPN (`h2` / `http/1.1`) handling, and unit test suite.

## Sub-tasks

1. Add `TLSConfig` struct to `pkg/config/config.go`.
2. Update `config.yaml` with `server.tls` options.
3. Implement `pkg/server/tls.go` providing `GenerateDevCert` and `CreateTLSConfig`.
4. Add `ListenAndServeTLS` and ALPN `h2` routing in `pkg/server/server.go`.
5. Update `cmd/toron/main.go` to invoke `ListenAndServeTLS` when TLS is enabled.
6. Add HTTPS TLS unit tests in `pkg/server/tls_test.go` and run `go test ./...`.

## Acceptance Criteria

- Server successfully accepts HTTPS connections over TLS 1.2/1.3.
- Auto-generates self-signed dev certs if configured.
- Negotiates HTTP/2 (`h2`) ALPN cleanly.
- All Go unit tests pass cleanly.
