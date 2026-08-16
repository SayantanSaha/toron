---
title: REST-to-gRPC Transcoding Engine
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-16
updated: 2026-08-16

depends_on:
  - REQ-049

derived_from:
  - ADR-044

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
* **URL Path & Query Parameter Extraction**: Automatically maps path parameters (`/v1/users/:id`) and query parameters into gRPC request fields.
* **gRPC Status Mapping**: Translates gRPC trailer statuses (`grpc-status: 0` -> `200 OK`, `grpc-status: 5` -> `404 Not Found`, `grpc-status: 16` -> `401 Unauthorized`).

---

## ⚙️ Configuration Reference (`toron.yaml`)

```yaml
transcoder:
  enabled: true
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
