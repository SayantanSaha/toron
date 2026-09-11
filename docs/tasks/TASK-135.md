---
id: TASK-135
type: task
title: Implementation of HTTP/2 Translation Invariant Guards (H2.CL / H2.TE)
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-12
updated: 2026-09-12

depends_on:
  - REQ-112

derived_from:
  - REQ-112
  - SRC-06

implements:
  - REQ-112

verified_by:
  - TC-112

decided_by:
  - ADR-112

related_to:
  - REQ-112
  - ADR-112
  - TC-112
  - CR-108
  - SR-112
---

# TASK-135 - Implementation of HTTP/2 Translation Invariant Guards (H2.CL / H2.TE)

## 1. Description

Decompose and implement HTTP/2 semantic translation invariant validation in `pkg/server/server.go` / `pkg/httpparser` and upstream pseudo-header isolation in `pkg/proxy/proxy.go` (`SRC-06`, `REQ-112`). This resolves H2.CL / H2.TE cross-protocol request smuggling vulnerabilities, enforces RFC 7540 §8.1.2 connection header restrictions, and blocks CRLF/NUL injection attacks.

## 2. Subtask Breakdown

### TASK-135.1: Translation Validation Engine in `pkg/httpparser` / `pkg/server/server.go`
- **Component**: `pkg/httpparser/request.go` or `pkg/httpparser/parser.go`, `pkg/server/server.go`
- **Scope**:
  - Define `ValidateHTTP2Translation(r *http.Request) error` (or on `*httpparser.Request`):
    1. **Forbidden Connection Headers (RFC 7540 §8.1.2.2)**:
       - Check for presence of `Transfer-Encoding`, `Connection`, `Keep-Alive`, `Proxy-Connection`, or `Upgrade`.
       - If `Upgrade` is present, only permit it if `:protocol` pseudo-header is present (RFC 8441 extended CONNECT).
       - Return explicit bad request error on violation.
    2. **Content-Length Invariant (H2.CL)**:
       - Validate that `Content-Length` contains no multiple headers, comma-separated values, or invalid non-digit characters.
    3. **CRLF & NUL Binary Injection Scanner**:
       - Scan request method, URL path, raw query, host/authority, and all header keys and values for `\r`, `\n`, or `0x00`.
       - Return bad request error if any forbidden octet is found.

### TASK-135.2: Server Integration & Payload Length Enforcement
- **Component**: `pkg/server/server.go:http2AdapterHandler`
- **Scope**:
  - In `http2AdapterHandler`, invoke translation validation prior to router dispatch.
  - If validation fails, respond with `400 Bad Request` and descriptive JSON error payload without router forwarding.
  - In non-CONNECT requests where `Content-Length` is declared:
    - After ingesting body bytes, assert that the read payload byte length exactly matches declared `Content-Length`.
    - If there is a discrepancy between received body length and declared `Content-Length`, reject with `400 Bad Request`.

### TASK-135.3: Upstream Pseudo-Header Stripping in `pkg/proxy/proxy.go`
- **Component**: `pkg/proxy/proxy.go`
- **Scope**:
  - In `ServeHTTPWithPrefix`:
    - When copying headers from `req.Header` to `outReq.Header`, filter out any key starting with `:` (e.g. `:protocol`, `:path`, `:authority`, `:method`, `:scheme`).
  - In `proxyRawStream`:
    - When serializing HTTP/1.1 wire lines for WebSocket/raw TCP upstream connections, filter out any key starting with `:`.
  - Prevent internal or downstream pseudo-headers from ever leaking into upstream HTTP/1.1 wire requests.

### TASK-135.4: Automated Test Suite & Microbenchmarks
- **Component**: `pkg/server/server_test.go`, `pkg/proxy/proxy_test.go`
- **Scope**:
  - Implement `TestServer_HTTP2_TransferEncodingRejected` (asserting 400 on H2.TE).
  - Implement `TestServer_HTTP2_MultipleContentLengthRejected` (asserting 400 on duplicate CL).
  - Implement `TestServer_HTTP2_ContentLengthMismatchRejected` (asserting 400 on body mismatch).
  - Implement `TestServer_HTTP2_CRLFInjectionRejected` (asserting 400 on CRLF in headers or path).
  - Implement `TestServer_HTTP2_ForbiddenConnectionHeadersRejected` (asserting 400 on Connection/Keep-Alive/Upgrade).
  - Implement `TestServer_HTTP2_ExtendedConnectAllowed` (verifying RFC 8441 WebSocket).
  - Implement `TestProxy_HTTP2ToHTTP1_PseudoHeaderStripping` (verifying `:` headers are stripped on upstream HTTP/1.1 socket).

## 3. Acceptance Criteria

- All HTTP/2 requests with forbidden connection headers are rejected with `400 Bad Request`.
- All conflicting or mismatched `Content-Length` headers are rejected with `400 Bad Request`.
- All CRLF or NUL injections in headers/pseudo-headers are rejected with `400 Bad Request`.
- Upstream HTTP/1.1 requests never contain colon-prefixed pseudo-headers.
- Zero third-party dependencies; 100% race-clean test suite (`go test -race -count=1 ./...`).
