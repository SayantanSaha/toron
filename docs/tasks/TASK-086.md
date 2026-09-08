---
id: TASK-086
type: task
title: Implement Zero-Deadlock Channel Lifecycle Coordination & Graceful Teardown
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-084
  - TASK-084
  - TASK-085

owns:
  - pkg/ingress/controller.go
  - pkg/ingress/ingress_test.go

references:
  - REQ-084
  - ADR-079
---

# TASK-086 - Implement Zero-Deadlock Channel Lifecycle Coordination & Graceful Teardown

## Overview

Ensure that `Controller.Stop()` can terminate cleanly and instantly under all conditions (including when channel buffer is saturated with >100 unconsumed events or when watch streaming is actively receiving data), eliminating the shutdown deadlock.

## Scope & Implementation Breakdown

1. **Graceful Teardown Coordination**:
   - In `Controller.Stop()`, verify that cancelling `c.cancel()` immediately unblocks both `watchWorker` and `consumeEventsWorker`.
   - Ensure `c.wg.Wait()` completes within milliseconds without relying on arbitrary timeouts or risk of hanging.
2. **Reconnection & Error Handling**:
   - Handle watch stream EOF / disconnects cleanly in `watchWorker` without panics or leaked goroutines.
3. **Comprehensive Automated Verification**:
   - Create tests in `pkg/ingress/ingress_test.go` that simulate streaming K8s API watch server responses with >100 events.
   - Verify zero deadlocks during concurrent startup, event burst ingestion, and shutdown.
