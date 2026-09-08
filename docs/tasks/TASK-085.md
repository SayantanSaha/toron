---
id: TASK-085
type: task
title: Implement Dedicated Ingress Event Consumer Loop & Debounced Route Sync
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-084
  - TASK-084

owns:
  - pkg/ingress/controller.go

references:
  - REQ-084
  - ADR-079
---

# TASK-085 - Implement Dedicated Ingress Event Consumer Loop & Debounced Route Sync

## Overview

Implement a continuous event consumer worker in `Controller` (`pkg/ingress/controller.go`) to drain `c.events` and trigger route synchronization upon Ingress lifecycle changes (`ADDED`, `MODIFIED`, `DELETED`), preventing channel buffer saturation.

## Scope & Implementation Breakdown

1. **Consumer Goroutine (`consumeEventsWorker`)**:
   - Add `consumeEventsWorker(ctx context.Context)` to `pkg/ingress/controller.go`.
   - Track the worker in `c.wg` (`c.wg.Add(1)` in `Controller.Start()`).
   - Listen on both `<-ctx.Done()` and `evt, ok := <-c.events`.
2. **Debounce & Coalescing Engine**:
   - Introduce a lightweight timer (25ms) to coalesce rapid bursts of incoming watch events into single invocations of `c.syncIngresses(ctx)`, protecting against downstream route calculation thrashing.
3. **Reactive Route Updates**:
   - Ensure `c.syncIngresses(ctx)` executes when watch events are received, making route additions, modifications, and deletions live in real time.
