---
id: TASK-037
type: task
title: Implement Brotli (`br`) and Zstandard (`zstd`) Response Compression with Encoders Pooling
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-14
updated: 2026-08-14

depends_on:
  - REQ-037

implements:
  - REQ-037

verified_by:
  - TC-037

decided_by:
  - ADR-032

related_to:
  - TASK-034
  - TASK-035
---

# TASK-037 - Implement Brotli (`br`) and Zstandard (`zstd`) Response Compression with Encoders Pooling

## Goal

Enhance [`pkg/router/compression.go`](file:///D:/Work/server/pkg/router/compression.go) to support Brotli (`br`) and Zstandard (`zstd`) compression algorithms, integrating encoder pools and quality value negotiation.

## Sub-tasks

1. Extend `CompressionMiddleware` in `pkg/router/compression.go`:
   - Add Brotli (`brotli.NewWriterLevel`) and Zstandard (`zstd.NewWriter`) streaming encoders.
   - Implement `brotliPool` and `zstdPool` via `sync.Pool`.
   - Update `selectEncoding(acceptHeader string, supported []string)` to support `zstd`, `br`, `gzip`, `deflate`, and quality weighting.
2. Update default encodings in `pkg/config/config.go` and `config.yaml` to include `zstd` and `br`.
3. Create unit tests in `pkg/router/compression_test.go` verifying:
   - Brotli roundtrip decompression and header verification.
   - Zstandard roundtrip decompression and header verification.
   - Quality weighting resolution.
   - Pool reuse and zero corruption under concurrency.
4. Run full test suite `go test ./...` and configuration dry-run `.\toron.exe -t`.

## Acceptance Criteria

- Brotli and Zstandard compression achieve high compression ratios on text/JSON payloads.
- Responses contain accurate `Content-Encoding` and `Vary: Accept-Encoding` headers.
- All Go unit tests pass cleanly.
