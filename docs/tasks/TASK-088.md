---
id: TASK-088
type: task
title: Implement Reserved Administrative & Gateway Endpoint Shadowing Guards
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-085
  - TASK-087

owns:
  - pkg/ingress/translator.go

references:
  - REQ-085
  - ADR-080
---

# TASK-088 - Implement Reserved Administrative & Gateway Endpoint Shadowing Guards

## Overview

Protect internal administrative, status, telemetry, and health probe endpoints from being shadowed or intercepted by Kubernetes Ingress rules in `pkg/ingress/translator.go`.

## Scope & Implementation Breakdown

1. **Internal & Management API Protection**:
   - Reject any rule where canonical path is `/internal` or starts with `/internal/`.
   - Reject any rule where canonical path is `/api/status` or starts with `/api/status/`.
2. **Telemetry & Health Probe Protection**:
   - Reject unhosted (`host == ""`) rules targeting `/health`, `/health/*`, `/metrics`, or `/metrics/*`.
