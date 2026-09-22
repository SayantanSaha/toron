---
title: REST-to-gRPC Transcoding Engine
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-16
updated: 2026-09-09

depends_on:
  - REQ-049
  - REQ-089
  - REQ-090
  - REQ-091

derived_from:
  - ADR-044
  - ADR-084
  - ADR-085
  - ADR-086
  - SEC-28
  - SEC-29
  - SEC-30

documents:
  - REST-TO-GRPC-TRANSCODING-GUIDE

related_to:
  - grpc-gateway.md
  - reverse-proxy.md
  - configuration.md
---

# 🔀 REST-to-gRPC Transcoding Engine (`pkg/transcoder`)

Toron Edge Gateway features a native, zero-dependency **REST-to-gRPC Transcoding Engine** (`pkg/transcoder`). It translates incoming RESTful JSON HTTP requests (e.g. `GET /v1/users/123`) into binary Protobuf-encoded HTTP/2 gRPC requests (e.g. `POST /user.UserService/GetUser`) and converts returning binary gRPC payloads and `grpc-status` headers back into REST JSON responses.

---

## 🌟 Key Features

* **Zero External Dependencies**: Implements JSON payload parsing, path parameter extraction, gRPC 5-byte wire framing, and status code mapping using Go stdlib without protobuf compiler dependencies.
* **Direct Parameterized Subpath Routing (`SEC-30`, CWE-284 / CWE-400)**:
  * **Elimination of Empty Upstream Proxies**: Completely removed dummy upstream reverse proxy registration that previously caused parameterized REST requests to abort with `502 Bad Gateway: No upstream target available`.
  * **Native In-Process Prefix Binding**: Parameterized endpoints (e.g. `GET /v1/users/:id`, `GET /v1/users/:id/orders/:orderId`) bind directly to `Router.HandlePrefixWithMatcher` with path pattern validation (`MatchPathPattern`), executing cleanly through `router.ServeHTTP`.
  * **Multi-Level Route Segregation & Method Gating**: Multiple routes sharing common path prefixes are cleanly segregated without route shadowing; invalid methods return `405 Method Not Allowed`, and segment count mismatches return `404 Not Found`.
* **Hop-by-Hop Header Sanitization & Strict RFC 7540 Compliance (`SEC-29`, CWE-444 / CWE-436)**:
  * **Static Hop-by-Hop Header Stripping**: Strips standard RFC 7230 / RFC 7540 connection-specific headers (`Connection`, `Keep-Alive`, `Upgrade`, `Proxy-Connection`, `Transfer-Encoding`, `Proxy-Authenticate`, `Proxy-Authorization`, `Trailer`, `Trailers`, `Host`) before dispatching HTTP/2 gRPC requests.
  * **Dynamic Connection Token Parsing**: Dynamically parses comma-delimited tokens from the client `Connection` header and strips matching nominated headers per RFC 7230 §6.1 / RFC 9110 §7.6.1.
  * **Strict `TE: trailers` Invariant**: Discards client `TE` values (e.g. `gzip`, `deflate`) and strictly enforces single-valued `TE: trailers` per RFC 7540 §8.1.2.2 / RFC 9113 §8.2.2, preventing upstream gRPC backends from terminating streams with `RST_STREAM (PROTOCOL_ERROR 0x1)`.
  * **Metadata Preservation**: Preserves application authentication, tracing, and custom metadata headers (`Authorization`, `X-Request-Id`, `Traceparent`, `User-Agent`) intact with full byte fidelity.
* **Bounded Ingestion & 413 Rejection (`SEC-28`, CWE-400 / CWE-770)**: Protects against memory exhaustion and OOM kills via configurable `max_body_bytes` (default: 4 MB / `4194304` bytes):
  * **Declared `Content-Length` Fast-Fail**: Requests declaring payload size $> \text{max\_body\_bytes}$ are rejected immediately with `HTTP 413 Payload Too Large` without socket reading or memory allocation.
  * **Bounded Stream Over-Read**: Chunked or undeclared streams are capped via `io.LimitReader` and rejected with `HTTP 413` if bytes exceed the ceiling, preventing `json.Unmarshal` heap explosion.
  * **Upstream Isolation**: Upstream gRPC backends receive 0 requests on rejected payloads.
* **URL Path & Query Parameter Extraction**: Automatically maps path parameters (`/v1/users/:id`) and query parameters into gRPC request fields.
* **gRPC Status Mapping**: Translates gRPC trailer statuses (`grpc-status: 0` -> `200 OK`, `grpc-status: 5` -> `404 Not Found`, `grpc-status: 16` -> `401 Unauthorized`).

---

## ⚙️ Configuration Reference (`config.yaml`)

```yaml
transcoder:
  enabled: true
  max_body_bytes: 4194304   # Maximum incoming request body limit in bytes (default: 4MB; SEC-28)
  routes:
    - http_method: "GET"
      http_path: "/v1/users/:id"
      grpc_method: "/user.UserService/GetUser"
      upstream_url: "http://localhost:9005"
      field_mappings:
        id: "userId"
```

---

## 🚀 Usage Example

### Curl Command
```bash
curl -i http://localhost:8080/v1/users/123
```

### JSON Response
```json
{
  "userId": "123",
  "name": "John Doe",
  "status": "active"
}
```
