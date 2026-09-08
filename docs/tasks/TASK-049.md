---
id: TASK-049
type: task
title: Implement REST-to-gRPC Transcoding Engine
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-16
updated: 2026-09-08

depends_on:
  - REQ-049

owns:
  - pkg/transcoder
  - pkg/config

references:
  - REQ-049
  - ADR-044
---

# TASK-049 - Implement REST-to-gRPC Transcoding Engine

## Overview

Implement the `pkg/transcoder` package supporting REST JSON to gRPC Protobuf wire framing, URL path parameter extraction, and HTTP/2 gRPC response translation.

## Task Breakdown

1. [x] **Requirement Spec**: Define `REQ-049.md`.
2. [ ] **Config Extension**: Add `TranscoderConfig` struct to `pkg/config/config.go` and `config.yaml`.
3. [ ] **Core Models**: Create `pkg/transcoder/types.go` for transcoding rules and gRPC status mappings.
4. [ ] **Wire Framer**: Create `pkg/transcoder/framer.go` implementing 5-byte gRPC prefix framing.
5. [ ] **Transcoder Engine**: Create `pkg/transcoder/transcoder.go` mapping REST HTTP to gRPC HTTP/2 calls.
6. [ ] **Unit Tests & Race Verification**: Create `pkg/transcoder/transcoder_test.go`.
7. [ ] **Architecture, Quality & Security Reports**: Create `ADR-044`, `CR-044`, `SR-044`, `TC-049`.
8. [ ] **Documentation**: Update Wiki, `README.md`, and `PRODUCT_REVIEW.md`.
