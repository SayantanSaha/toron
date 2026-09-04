---
id: TASK-058
type: task
title: Implement Config-Based Multi-Stream Logging, Route Overrides, and Logrotate Support
status: draft
version: 1.0
project: PROJECT-001
owner: development-lead
created: 2026-09-04
updated: 2026-09-04
depends_on: []
derived_from:
  - REQ-058
implements:
  - REQ-058
verified_by:
  - TC-058
decided_by:
  - ADR-053
related_to:
  - TASK-003
  - TASK-005
  - TASK-041
---

# TASK-058 - Implement Config-Based Multi-Stream Logging, Route Overrides, and Logrotate Support

## Description

Implement configuration-driven multi-stream logging in Toron, supporting dedicated server, access, and security log streams configured via `config.yaml`, per-route access and security log path overrides configured via `routes.yaml`, and daily log rotation compatibility via `os.O_APPEND` file modes, `SIGHUP` atomic file descriptor reopening, and an `etc/logrotate.d/toron` logrotate configuration.

## Scope & Implementation Breakdown

1. **Configuration Schema Extension (`pkg/config/config.go`)**:
   - Update `LoggingConfig`:
     ```go
     type LoggingConfig struct {
         Level       string `yaml:"level" json:"level"`
         Format      string `yaml:"format" json:"format"`
         ServerLog   string `yaml:"server_log" json:"server_log"`
         AccessLog   string `yaml:"access_log" json:"access_log"`
         SecurityLog string `yaml:"security_log" json:"security_log"`
     }
     ```
   - Extend `ProxyRouteConfig` with:
     ```go
     AccessLog   string `yaml:"access_log,omitempty" json:"access_log,omitempty"`
     SecurityLog string `yaml:"security_log,omitempty" json:"security_log,omitempty"`
     ```
   - Update `DefaultAppConfig()` and add validation in `ValidateConfig()`.

2. **Dedicated Logging Engine & Manager (`pkg/logging/manager.go`)**:
   - Define thread-safe `LogManager` managing sinks:
     - `ServerWriter() io.Writer`: For general server and operational logs.
     - `LogAccess(entry AccessLogEntry, routeOverride string)`: Writes access log entries to route-specific sink or default access log sink.
     - `LogSecurity(entry SecurityLogEntry, routeOverride string)`: Writes security audit entries to route-specific sink or default security log sink.
     - `Reopen() error`: Thread-safe atomic close & re-open of all active file handles for `SIGHUP` log rotation.
     - `Close() error`: Clean teardown of all opened file descriptors.
   - Support `os.O_CREATE|os.O_WRONLY|os.O_APPEND` file mode and auto-creation of parent directories (`os.MkdirAll`).
   - Formatters for Common/Combined text log format and structured JSON format based on `logging.format`.

3. **Routing & Access Logging Middleware Integration (`pkg/router/router.go`)**:
   - Enhance access logging middleware to interact with `LogManager`:
     - Inspect request route to identify applicable `access_log` path (per-route override or default).
     - Record client IP, timestamp, method, path, HTTP version, status code, latency duration, bytes written, User-Agent, and W3C traceparent.
     - Bypass logging if `access_log` is `"off"` or `"none"`.
   - Update error recovery to pipe panic traces into the ServerLog sink.

4. **Security Logging Integration (`pkg/waf`, `pkg/router`)**:
   - Connect WAF audit logging, Auth failures (JWT, APIKey, BasicAuth), and Rate Limiting rejections to write into the security log stream with route-level destination support.

5. **Server & Main Integration with SIGHUP Handling (`cmd/toron/main.go`)**:
   - Initialize `LogManager` using `appCfg.Logging`.
   - Set standard library `log.SetOutput` to the server log writer.
   - Listen for `syscall.SIGHUP` in the signal handler goroutine alongside SIGINT/SIGTERM.
   - Trigger `logManager.Reopen()` on `SIGHUP` and log the rotation event.
   - Close `LogManager` on server shutdown.

6. **System Logrotate Template (`etc/logrotate.d/toron`)**:
   - Create the system logrotate configuration file for daily rotation with `postrotate` signal or `copytruncate`.

7. **Verification & Test Coverage**:
   - Config deserialization unit tests in `pkg/config/config_test.go`.
   - LogManager unit tests (writing, multi-sink, concurrent writes, SIGHUP reopen) in `pkg/logging/manager_test.go`.
   - Router access logging with route overrides in `pkg/router/router_test.go`.
