---
id: TASK-089
type: task
title: Implement Security Audit Logging and Warning Telemetry for Rejected Ingress Rules
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-085
  - TASK-087
  - TASK-088

owns:
  - pkg/ingress/translator.go
  - pkg/ingress/ingress_test.go

references:
  - REQ-085
  - ADR-080
---

# TASK-089 - Implement Security Audit Logging and Warning Telemetry for Rejected Ingress Rules

## Overview

Provide visibility into rejected Ingress rules via structured security warning logs and verify complete coverage with unit tests.

## Scope & Implementation Breakdown

1. **Security Audit Warning Logs**:
   - Log actionable warning messages when an Ingress attempts to register unhosted root or shadow protected paths.
   - Include namespace, Ingress name, attempted path, and host in the log entry.
2. **Comprehensive Unit Tests (`pkg/ingress/ingress_test.go`)**:
   - Create tests covering unhosted root rejection, hosted root allowance, `/internal` shadowing rejection, `/api/status` rejection, and path traversal normalization.
