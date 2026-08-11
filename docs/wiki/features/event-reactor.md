---
title: Event Reactor Core Architecture
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-001
  - REQ-004
  - TASK-001

derived_from:
  - REQ-001
  - ADR-001

documents:
  - EVENT-REACTOR-FEATURE

related_to:
  - index.md
  - features/static-file-serving.md
---

# Event Reactor Core Architecture

## Overview

At the heart of Toron is the **Event Reactor** (`pkg/reactor`), a high-performance network engine designed to handle concurrent TCP connections efficiently with low latency and minimal memory allocations.

## Key Design Principles

1. **Non-Blocking Socket Acceptance**: Socket connections are accepted on an asynchronous event loop and dispatched to worker queues.
2. **Bounded Worker Pool**: Configurable worker pool (default `128` workers) limits goroutine spawn overhead under heavy traffic spikes.
3. **Buffer Pooling (`sync.Pool`)**: Reuses byte buffers across connection reads/writes to reduce GC pressure and heap allocations.
4. **Graceful Shutdown**: Listens for OS termination signals (`SIGINT`, `SIGTERM`) and drains active connections before shutting down.

## Connection Lifecycle Flow

```text
TCP Client -> Reactor Listener -> Task Channel -> Worker Pool -> HTTP Parser -> Router -> Client Response
```

## Related Pages

- [Static File Serving](./static-file-serving.md)
- [Configuration Options](../reference/config-options.md)
