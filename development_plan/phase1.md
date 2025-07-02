# Toron Phase 1 Development Plan (Test-Driven Development)

## 🚀 Phase Goal:

Build the foundational HTTP/HTTPS reverse proxy server with basic routing, configuration loading, and complete unit test coverage.

---

## 🔁 TDD Methodology

Each component should follow this cycle:

1. Write unit tests first (`*_test.go`)
2. Implement minimal code to pass tests
3. Refactor with green tests
4. Maintain >85% test coverage

---

## 📦 Phase 1 Modules & Tasks

### 1. Configuration Loader (`internal/config`)

**Goal:** Load and validate YAML configuration from file/env

- [ ] Define `Config` structs for Server, Routes, Backends
- [ ] Write tests for invalid/missing config
- [ ] Implement file-based loader with env substitution
- [ ] Schema validation with test coverage

**Files:** `loader.go`, `schema.go`, `loader_test.go`

---

### 2. HTTP Listener (`internal/listener`)

**Goal:** Start HTTP/HTTPS server, TLS support, route delegation

- [ ] Test listener boot/shutdown behavior
- [ ] Support HTTP and TLS (HTTPS)
- [ ] Integrate with Go's `http.Server`
- [ ] Forward to router handler

**Files:** `http_listener.go`, `http_listener_test.go`

---

### 3. Router (`internal/router`)

**Goal:** Path/host-based L7 routing

- [ ] Define route config (method, host, path)
- [ ] Tests: wildcard, method match, fallback
- [ ] Use `chi` or custom trie-based router
- [ ] Dynamic config-based routing

**Files:** `router.go`, `matcher.go`, `router_test.go`

---

### 4. Reverse Proxy Core (`internal/core`)

**Goal:** Proxy HTTP requests to backend servers

- [ ] Use `httputil.ReverseProxy`
- [ ] Test header rewriting and timeout behavior
- [ ] Implement simple retry logic
- [ ] Handle edge cases (502, connection refused)

**Files:** `proxy.go`, `proxy_test.go`

---

### 5. Middleware Stack (`internal/middleware`)

**Goal:** Logging, request ID, GZIP compression

- [ ] Logger middleware using `zap`
- [ ] Request ID injection
- [ ] GZIP compression
- [ ] Unit tests for each middleware + chaining

**Files:** `logger.go`, `gzip.go`, `requestid.go`, `middleware_test.go`

---

### 6. Load Balancer (`internal/loadbalancer`)

**Goal:** Round-robin backend selection

- [ ] Define `Balancer` interface
- [ ] Implement round-robin strategy
- [ ] Unit tests for fairness and fallback
- [ ] Integrate with router backend dispatch

**Files:** `roundrobin.go`, `balancer.go`, `balancer_test.go`

---

### 7. Unit Testing Strategy

**Goal:** Full unit test coverage for Phase 1

- [ ] Write table-driven tests
- [ ] Focus on:
  - Invalid config
  - Route match matrix
  - Header transformations
  - Middleware order

**Location:** Respective module test files and `test/integration/`

---

### 8. CI/CD Setup (`.github/workflows`)

**Goal:** Automate tests and lint checks

- [ ] Create `ci.yml`
- [ ] Steps:
  - Set up Go
  - Run `golangci-lint`
  - Run `go test ./... -cover`
- [ ] Add build and test status badge

**Files:** `.github/workflows/ci.yml`

---

## 📅 Suggested Timeline (2–3 weeks)

| Week | Tasks                            |
| ---- | -------------------------------- |
| 1    | Config, Listener, Router         |
| 2    | Proxy, Middleware, Load Balancer |
| 3    | Tests, CI, Manual Validation     |

---

## ✅ Phase 1 Deliverables

- ✅ Configurable HTTP/HTTPS reverse proxy
- ✅ Config-driven route management
- ✅ Round-robin backend load balancing
- ✅ Basic middleware (logging, gzip, request ID)
- ✅ 85%+ test coverage with CI integration
- ✅ Clean modular structure aligned with TDD
