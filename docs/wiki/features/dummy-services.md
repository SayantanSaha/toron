---
title: Dummy Web Services Test Suite
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-12
updated: 2026-08-12

depends_on:
  - REQ-012
  - TASK-012

derived_from:
  - REQ-012
  - ADR-007

documents:
  - DUMMY-SERVICES-FEATURE

related_to:
  - index.md
  - load-balancing.md
  - reverse-proxy.md
---

# Dummy Web Services Test Suite

## Overview

Toron includes a **Dummy Web Services Test Suite** inside `dummy-services/`. It allows launching 10 lightweight dummy web servers on ports `9001` through `9010` to test reverse proxy routing and load balancing algorithms.

## Running the Dummy Services

From the root directory, run:

```bash
go run ./dummy-services -port 9001 -count 10
```

This launches 10 HTTP servers listening on `http://localhost:9001` through `http://localhost:9010`.

## Response Format

Each service returns JSON metadata:

```json
{
  "service": "dummy-service-1",
  "port": 9001,
  "path": "/api/users",
  "method": "GET",
  "headers": {
    "User-Agent": ["ToronProxy/1.0"],
    "X-Forwarded-Host": ["localhost:8080"]
  }
}
```
