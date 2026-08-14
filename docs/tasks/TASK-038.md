---
id: TASK-038
type: task
title: Implement Native gRPC Health Checking Prober and HTTP/2 Trailers Preservation
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-038

implements:
  - REQ-038

verified_by:
  - TC-038

decided_by:
  - ADR-033

related_to:
  - TASK-004
  - TASK-009
  - TASK-018
---

# TASK-038 - Implement Native gRPC Health Checking Prober and HTTP/2 Trailers Preservation

## Goal

Create `pkg/proxy/grpc_health.go` implementing the standard `grpc.health.v1.Health/Check` prober and integrate it into `pkg/proxy/proxy.go`, `pkg/config/config.go`, and `routes.yaml`.

## Sub-tasks

1. Create `pkg/proxy/grpc_health.go` implementing:
   - `EncodeGRPCHealthCheckRequest(service string) []byte` (5-byte gRPC prefix + Protobuf payload).
   - `DecodeGRPCHealthCheckResponse(body []byte) (ServingStatus, error)`.
   - `ProbeGRPCHealth(ctx context.Context, targetURLStr, service string, timeout time.Duration) (bool, error)`.
2. Update `pkg/proxy/proxy.go`:
   - Extend `ProxyOptions` with `HealthCheckType string` and `HealthCheckService string`.
   - In `probeTarget(target)`, branch to `ProbeGRPCHealth` when `HealthCheckType == "grpc"`.
   - Ensure trailing headers (`grpc-status`, `grpc-message`, `Trailer`) are cloned into the client response.
3. Update `pkg/config/config.go` with `HealthCheckType` and `HealthCheckService` fields.
4. Update `cmd/toron/main.go` and `routes.yaml` with gRPC health check examples.
5. Create comprehensive unit and mock tests in `pkg/proxy/grpc_health_test.go`.
6. Run full test suite `go test ./...` and configuration dry-run `.\toron.exe -t`.

## Acceptance Criteria

- `ProbeGRPCHealth` accurately formats gRPC frames and validates `SERVING (1)` responses.
- `grpc-status` and `grpc-message` trailers are accurately forwarded to reverse proxy clients.
- Full Go test suite passes cleanly with 0 failures.
