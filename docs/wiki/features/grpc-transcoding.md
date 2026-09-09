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

derived_from:
  - ADR-044
  - ADR-084
  - SEC-28

documents:
  - REST-TO-GRPC-TRANSCODING-GUIDE

related_to:
  - grpc-gateway.md
  - reverse-proxy.md
  - configuration.md
---

# 🔀 REST-to-gRPC Transcoding Engine (`pkg/transcoder`)

Toron Edge Gateway features a native, zero-dependency **REST-to-gRPC Transcoding Engine** ([`pkg/transcoder`](file:///Users/sneha/Developer/toron-research/toron/pkg/transcoder)). It translates incoming RESTful JSON HTTP requests (e.g. `GET /v1/users/123`) into binary Protobuf-encoded HTTP/2 gRPC requests (e.g. `POST /user.UserService/GetUser`) and converts returning binary gRPC payloads and `grpc-status` headers back into REST JSON responses.

---

## 🌟 Key Features

* **Zero External Dependencies**: Implements JSON payload parsing, path parameter extraction, gRPC 5-byte wire framing, and status code mapping using Go stdlib without protobuf compiler dependencies.
* **Bounded Ingestion & 413 Rejection ([`SEC-28`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L393-L401), CWE-400 / CWE-770)**: Protects against memory exhaustion and OOM kills via configurable `max_body_bytes` (default: 4 MB / `4194304` bytes):
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
