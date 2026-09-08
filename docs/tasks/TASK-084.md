---
id: TASK-084
type: task
title: Implement Non-Blocking, Context-Aware Watch Stream Sender in Ingress Client
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-084

owns:
  - pkg/ingress/client.go

references:
  - REQ-084
  - ADR-079
---

# TASK-084 - Implement Non-Blocking, Context-Aware Watch Stream Sender in Ingress Client

## Overview

Eliminate the un-cancellable blocking send vulnerability in `WatchIngresses` in `pkg/ingress/client.go` by wrapping channel transmission in a context-aware `select` block.

## Scope & Implementation Breakdown

1. **Context-Aware Event Dispatch**:
   - In `Client.WatchIngresses()`, locate the JSON decoding block (`var evt K8sWatchEvent`).
   - Replace the naked channel send `events <- evt` with:
     ```go
     select {
     case events <- evt:
     case <-ctx.Done():
         return ctx.Err()
     }
     ```
2. **Immediate Connection Termination on Cancellation**:
   - Verify that cancelling `ctx` immediately closes the HTTP response body reader and aborts the streaming HTTP connection without leaking the underlying socket.
3. **Unit Test Coverage**:
   - Ensure `WatchIngresses` returns `context.Canceled` cleanly when the channel receiver stops reading or the context expires.
