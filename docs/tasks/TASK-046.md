---
id: TASK-046
type: task
title: Implement Vendor-Agnostic OCI Container Auto-Discovery Engine
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-08-16
updated: 2026-08-16

depends_on:
  - REQ-046

owns:
  - pkg/discovery
  - pkg/config

references:
  - REQ-046
  - ADR-041
---

# TASK-046 - Implement Vendor-Agnostic OCI Container Auto-Discovery Engine

## Overview

Implement the `pkg/discovery` package supporting OCI container discovery over Unix domain sockets, label parsing, lifecycle event streaming, and dynamic upstream route registration.

## Task Breakdown

1. [x] **Config Extension**: Add `DiscoveryConfig` struct to `pkg/config/config.go`.
2. [x] **Core Provider Abstraction**: Create `pkg/discovery/provider.go` defining `Container`, `ContainerEvent`, `DiscoveredRoute`, and `Provider` interface.
3. [x] **Label Parser**: Create `pkg/discovery/parser.go` parsing `toron.*` labels.
4. [x] **Unix REST Provider**: Create `pkg/discovery/unix_provider.go` supporting Docker/Podman Unix socket REST streaming.
5. [x] **Discovery Manager**: Create `pkg/discovery/manager.go` orchestrating providers and updating `router.Router`.
6. [x] **Unit Testing & Verification**: Create `pkg/discovery/discovery_test.go` with mock Unix domain socket server and race detector validation.
