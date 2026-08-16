---
title: Getting Started with Toron
type: user-documentation
project: PROJECT-001
owner: document-writer
created: 2026-08-11
updated: 2026-08-11

depends_on:
  - REQ-001
  - TASK-005

derived_from:
  - REQ-001
  - TASK-005

documents:
  - GETTING-STARTED

related_to:
  - index.md
  - configuration.md
  - troubleshooting.md
---

# Getting Started with Toron

## Overview

This guide walks you through building, running, and verifying your first instance of the Toron web server.

## Prerequisites

- **Go 1.20+** installed on your operating system.
- Git (optional, for version control).

## Quickstart Steps

### 1. Clone or Open Project
Ensure you are in the Toron repository directory:
```bash
git clone https://github.com/SayantanSaha/toron_v3.git
cd toron_v3
```

### 2. Run the Server
You can start Toron directly using `go run`:

```bash
go run ./cmd/toron
```

Expected log output:
```text
2026/08/11 16:36:38 [TORON] Loaded configuration from config.yaml
2026/08/11 16:36:38 [TORON] Serving static assets from ./public under prefix "/"...
2026/08/11 16:36:38 [TORON] Server listening on http://localhost:8080...
```

### 3. Verify Server Functionality
Open a browser or terminal and test the server:

- **Web Browser**: Visit `http://localhost:8080` to view the static sample web application.
- **cURL Health Check**:
  ```bash
  curl http://localhost:8080/health
  ```
  Output: `{"status":"ok"}`

### 4. Build a Executable Binary
To build a standalone binary for production or distribution:

```bash
# Windows
go build -o toron.exe ./cmd/toron
./toron.exe

# Linux / macOS
go build -o toron ./cmd/toron
./toron
```

## Related Pages

- [Configuration Guide](./configuration.md)
- [Troubleshooting](./troubleshooting.md)
