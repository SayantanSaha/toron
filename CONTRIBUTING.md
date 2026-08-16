# 🤝 Contributing to Toron Edge Gateway

Thank you for your interest in contributing to **Toron**! Toron is an event-driven, high-performance, zero-dependency Web Server, Reverse Proxy Gateway, and Edge Security Engine written in pure Go.

---

## 📜 Architectural Principles & Strict Guidelines

Before submitting code, please review our core technical constraints:

1. **Zero 3rd-Party Runtime Dependencies**:
   - Toron core MUST depend strictly on the **Go standard library (`net`, `http`, `crypto`, `os`, `sync`, etc.)** and OS syscalls.
   - **Do NOT add third-party modules** to `go.mod`.
2. **Non-Blocking Reactor Concurrency**:
   - Never introduce blocking thread locks or synchronous I/O operations on the main event loop thread pool (`pkg/reactor`).
3. **Memory Allocation Control**:
   - Keep per-request allocations strictly zero-to-minimal in critical path proxy forwarding (`pkg/proxy` and `pkg/httpparser`).
4. **9-Artifact Agentic Workflow**:
   - All major features must include formal documentation artifacts in `docs/`:
     - Requirements (`docs/requirements/REQ-xxx.md`)
     - Tasks (`docs/tasks/TASK-xxx.md`)
     - Architecture Decision Records (`docs/architecture/ADR-xxx.md`)
     - Test Cases (`docs/testCases/TC-xxx.md`)
     - Code Review (`docs/codeReview/CR-xxx.md`)
     - Security Review (`docs/securityReview/SR-xxx.md`)
     - Wiki Feature Document (`docs/wiki/features/xxx.md`)

---

## 🗺️ Codebase Map & Package Responsibilities

```text
toron_v3/
├── cmd/
│   └── toron/            # CLI entrypoint and main server launcher
├── dummy-services/       # Cluster of 10 test backend microservices
├── pkg/
│   ├── acme/             # ACME HTTP-01 & TLS-ALPN-01 automated SSL engine
│   ├── config/           # Hot-reloading YAML configuration loader & validator
│   ├── discovery/        # Vendor-agnostic OCI Container auto-discovery (Unix sockets)
│   ├── httpparser/       # Zero-allocation HTTP/1.1 & HTTP/2 frame parsers
│   ├── ingress/          # Native Kubernetes networking.k8s.io/v1 Ingress Controller
│   ├── metrics/          # Prometheus metrics scraper & W3C traceparent telemetry
│   ├── proxy/            # 8 Load Balancers, 3-State Circuit Breakers, gRPC Health Prober
│   ├── reactor/          # Non-blocking epoll/kqueue event reactor & worker pool
│   ├── router/           # L4 TCP/UDP & L7 Host, Prefix, and Header router
│   ├── server/           # Edge gateway server wrapper, TLS/mTLS, & internal APIs
│   ├── sidecar/          # Service Mesh pod-to-pod mTLS sidecar proxy & traffic splitter
│   ├── transcoder/       # Binary REST-to-gRPC Protobuf transcoder engine
│   └── waf/              # OWASP WAF engine, CIDR IP ACLs, and request smuggling guards
├── public/               # Web Control Center Dashboard UI (HTML, CSS, JS)
├── install.sh            # Universal auto-installer script for Linux & macOS
├── install.bat           # Windows auto-installer script
├── Makefile              # Cross-platform build automation Makefile
└── test_endpoint.http    # VS Code / HTTP client test endpoint suite
```

---

## 🚀 Local Developer Setup

### Prerequisites
- **Go 1.20+** installed on your system.
- **Docker / Podman** (optional, for OCI container auto-discovery testing).

### 1. Build and Run Toron
```bash
# Clone the repository
git clone https://github.com/SayantanSaha/toron_v3.git
cd toron_v3

# Compile Toron binary to bin/
make build

# Start Toron server with default config
make run
```

### 2. Launch Test Upstream Microservices
In a separate terminal window:
```bash
make dummy
./bin/dummy -port 9004 -count 7
```

### 3. Verify Server & Web Control Center
- **Health Check**: `curl http://localhost:8080/health`
- **Dashboard UI**: [http://localhost:8080/internal/dashboard/](http://localhost:8080/internal/dashboard/)

---

## 🧪 Testing Guidelines

Before opening a Pull Request, run the full test suite across all 13 core packages:

```bash
# Run unit tests across all packages
make test

# Run verbose unit tests
make test-v

# Run native performance benchmarks
go test -bench=. ./pkg/proxy ./pkg/reactor
```

All 13 package test suites must pass with **100% success**.

---

## 🌿 Pull Request Workflow

1. Create a descriptive feature branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```
2. Commit changes with conventional commit messages:
   ```bash
   git commit -m "feat(proxy): add custom sticky header load balancer"
   ```
3. Update the Knowledge Graph AST:
   ```bash
   make graph
   ```
4. Push your branch and open a Pull Request against `master`.
