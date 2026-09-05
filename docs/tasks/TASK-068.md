---
id: TASK-068
type: task
title: Implement Wire Frame Size Bounding and Buffer Allocation Protection in gRPC Transcoder
status: approved
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-05
updated: 2026-09-05
depends_on: []
derived_from:
  - REQ-068
implements:
  - REQ-068
verified_by:
  - TC-068
decided_by:
  - ADR-063
related_to:
  - TASK-048
---

# TASK-068 - Implement Wire Frame Size Bounding and Buffer Allocation Protection in gRPC Transcoder

## Description

Enforce maximum wire frame size limits in `pkg/transcoder/framer.go` before allocating memory buffers during gRPC frame decoding, preventing Out-Of-Memory (OOM) heap exhaustion and denial-of-service crashes from malicious or corrupted upstream frames.

## Scope & Implementation Breakdown

1. **Max Frame Size Constant & Validation (`pkg/transcoder/framer.go`)**:
   - Define `DefaultMaxGRPCFrameSize = 4 * 1024 * 1024` (4 MB) in `pkg/transcoder`.
   - In `DecodeGRPCFrame(reader io.Reader)`, after extracting the 4-byte big-endian `length`, validate `length <= DefaultMaxGRPCFrameSize`.
   - Provide `DecodeGRPCFrameWithLimit(reader io.Reader, maxFrameSize uint32) ([]byte, error)` allowing custom configured thresholds.
   - If `length > maxFrameSize`, return an explicit error `ErrFrameTooLarge` immediately without executing `make([]byte, length)`.

2. **Transcoder Error Handling (`pkg/transcoder/transcoder.go`)**:
   - In transcoding dispatch loops where `DecodeGRPCFrame` is invoked, catch `ErrFrameTooLarge` and map it to `502 Bad Gateway` (or gRPC `ResourceExhausted` status code).
   - Ensure the server logs the frame violation with upstream context.

3. **Testing Verification (`pkg/transcoder/framer_test.go`)**:
   - Add unit test verifying that a synthetic frame with length `0x7FFFFFFF` or `0xFFFFFFFF` returns `ErrFrameTooLarge` instantly without allocating memory or hanging.
   - Verify that frames up to the configured limit are decoded successfully.

## Acceptance Criteria

- `DecodeGRPCFrame` rejects any frame exceeding `DefaultMaxGRPCFrameSize` without allocating memory buffers.
- `DecodeGRPCFrameWithLimit` respects caller-specified custom limits.
- The transcoder handles oversized upstream responses gracefully without panics.
- All transcoder tests pass with `go test ./pkg/transcoder/...`.

## Rationale

Unbounded buffer allocations based directly on wire frame header integers allow rogue or compromised upstream services to crash the Toron gateway via heap exhaustion.

## Constraints

- Zero allocations before frame length validation.
- Standard gRPC wire frame compatibility ([1 byte flag] + [4 bytes big-endian length]).

## Open Questions

- None.
