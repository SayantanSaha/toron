---
id: TASK-156
type: task
title: Inbound Chunked Transfer-Encoding Ingestion, Zero-Tolerance Wire Decoding, and Upstream Re-Framing Normalization (RFC 9112 §7.1)
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-17
updated: 2026-09-17

depends_on:
  - REQ-133
  - TASK-155

derived_from:
  - REQ-133

implements:
  - REQ-133

verified_by:
  - TC-133

decided_by:
  - ADR-133

related_to:
  - ADR-001
  - ADR-056
  - ADR-128
  - ADR-129
  - ADR-131
  - ADR-132
  - ADR-133
  - REQ-001
  - REQ-004
  - REQ-056
  - REQ-061
  - REQ-098
  - REQ-128
  - REQ-129
  - REQ-131
  - REQ-132
  - REQ-133
  - TASK-155
  - TC-133
---

# TASK-156 - Inbound Chunked Transfer-Encoding Ingestion, Zero-Tolerance Wire Decoding, and Upstream Re-Framing Normalization (RFC 9112 §7.1)

## 1. Overview & Objective

### 1.1 Problem Statement & Background: Moving Beyond the Legacy REST-Only Perimeter

Toron was originally architected as an ultra-high-performance Layer 4 and Layer 7 reverse proxy, web server, and API gateway. Under [`ADR-056`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-056.md) (*Inbound Transfer-Encoding Rejection & Request Smuggling Defense*) and [`REQ-061`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-061.md), Toron established an uncompromising inbound perimeter policy: all incoming HTTP requests carrying a `Transfer-Encoding: chunked` header (or any transfer-coding) were immediately rejected with `HTTP/1.1 501 Not Implemented`, and the underlying TCP socket was closed without reading the stream body.

While this defensive posture immunized Toron against HTTP Request Smuggling ([CWE-444](https://cwe.mitre.org/data/definitions/444.html)) during its initial development phases, it severely limits Toron's operational utility in production cloud-native environments:
1. **Inability to Support Streaming Uploads**: Clients streaming large files, audio/video data, or real-time event feeds cannot ingress through Toron because chunked uploads are rejected out-of-hand.
2. **Third-Party Webhook Incompatibility**: Automated SaaS webhooks (e.g. GitHub, Stripe, Datadog) transmit JSON or multipart payloads chunked by default. Toron's unconditional 501 rejection prevents ingestion of standard webhooks.
3. **Microservice Ingress Inflexibility**: Heterogeneous backend architectures rely on reverse proxies (e.g., NGINX, Envoy, Traefik) to accept chunked client streams. Toron's inability to ingest chunked streams prevents drop-in ingress replacement.
4. **Architectural Asymmetry**: Toron already features streaming outbound chunked responses under [`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md) and [`REQ-129`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-129.md). Rejecting inbound chunking creates an architectural asymmetry that blocks full-duplex HTTP/1.1 capabilities.

The objective of **TASK-156** is to break approved requirement [`REQ-133`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-133.md) into concrete, implementation-ready engineering work packages to enable native, zero-tolerance inbound chunked transfer-encoding ingestion and upstream canonical re-framing.

---

### 1.2 Evolution from ADR-056 / REQ-061: Controlled Supersedence

TASK-156 does not discard the security motivations of [`ADR-056`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-056.md); rather, it **evolves** ADR-056 from a static rejection rule into an **Active Ingress Smuggling Firewall**:
- **Strict RFC 9112 Wire Validation**: Replaces naive chunk reading with a zero-tolerance state machine that rejects malformed hex, sign markers, whitespace, or oversized extensions with `HTTP 400 Bad Request` and forceful socket teardown.
- **Configurable Backward Compatibility**: Preserves the legacy 501 rejection posture as a configurable operational profile (`inbound_chunked_mode: "reject"`) for ultra-hardened, zero-trust deployments.
- **Zero Dilution of Invariants**: Implements chunk decoding in pure Go standard library without external dependencies, preserves reactor transport modularity, maintains $O(1)$ memory streaming bounds ($\le 32\,\text{KB}$ per active reader), and guarantees zero data races under `go test -race ./...`.

---

### 1.3 Active Ingress Smuggling Firewall & Canonical Normalization Concept

Rather than blindly forwarding raw, potentially ambiguous chunked payloads to downstream origin backends (which may run fragile or non-conformant parsers such as Node.js `llhttp`, Python `h11`, or Ruby Puma), Toron acts as an active firewall:
1. **Zero-Tolerance Ingress Decoding**: Consumes chunks incrementally via `chunkedBodyReader`, rigorously checking chunk hex length, chunk extensions ($\le 256\,\text{B}$), CRLF boundaries, cumulative payload length ($\le \text{MaxBodyBytes}$), and trailing headers ($\le 4\,\text{KB}$ with forbidden header filtering).
2. **Canonical Edge Normalization (`"normalize"`)**: Absorbs decoded chunks into pooled buffers (`sync.Pool`), computes exact body length $L$, strips `Transfer-Encoding`, injects `Content-Length: L`, and forwards a clean, standard HTTP request upstream. Downstream backends are $100\%$ shielded from chunk-level parsing vulnerabilities and desynchronizations.
3. **Canonical Passthrough Mode (`"passthrough"`)**: Re-frames validated chunks with strict canonical formatting for high-throughput, unbounded streaming scenarios where edge buffering is undesirable.

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│                         ACTIVE INGRESS SMUGGLING FIREWALL & RE-FRAMING                           │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│   UNTRUSTED CLIENT                               TORON INGRESS EDGE GATEWAY       UPSTREAM ORIGIN│
│   (Potential Attacker)                          (Zero-Tolerance Invariant Guard)  (Backend Nodes)│
│                                                                                                  │
│   POST /upload HTTP/1.1 ────────► [ Preflight Smuggling Guards ]                                 │
│   Transfer-Encoding: chunked      • Reject Conflicting CL+TE (400)                               │
│                                   • Reject Obfuscated TE (400)                                   │
│                                   • Check Inbound Profile                                        │
│                                              │                                                   │
│                                              ▼                                                   │
│   1a;ext=valid\r\n ─────────────► [ Zero-Tolerance Chunk Decoder ]                               │
│   <26 bytes of data>\r\n          • Strict 1*HEXDIG (No +, -, SP)                                │
│   0\r\n                           • Clamp Chunk Ext <= 256B                                      │
│   X-Checksum: abc\r\n\r\n         • Clamp Cumulative Body <= MaxBodyBytes                        │
│                                   • Validate & Filter Trailers <= 4KB                            │
│                                              │                                                   │
│                                              ▼                                                   │
│                                   [ Canonical Edge Normalization ]                               │
│                                   • Strip Transfer-Encoding                                      │
│                                   • Compute Exact Content-Length (26)                            │
│                                   • Re-frame Clean Request                                       │
│                                              │                                                   │
│                                              └────────────────────────► POST /upload HTTP/1.1    │
│                                                                         Content-Length: 26       │
│                                                                         X-Checksum: abc          │
│                                                                         [Clean 26 Bytes Body]    │
│                                                                                                  │
│   [RESULT: Downstream microservice is 100% shielded from chunked parsing bugs and desyncs!]      │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### 1.4 Prior Requirements & Standards Cross-Audit (Conflict Analysis)

A comprehensive cross-audit against existing Toron specifications and architectural decisions confirms complete harmonization:

| Prior Requirement / ADR | Core Architectural Invariant | Potential Conflict & Cross-Audit Resolution | Compliance Verdict |
| :--- | :--- | :--- | :--- |
| **[`ADR-056`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-056.md) / [`REQ-061`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-061.md)** (Inbound Smuggling Guard) | Rejected inbound chunked requests with HTTP 501. | **Evolution & Supersedence**: REQ-133 supersedes the unconditional 501 rejection of ADR-056 by introducing a zero-tolerance decoder and canonical edge normalization. Legacy 501 rejection is preserved as a configurable operational profile (`inbound_chunked_mode: "reject"`). | **Controlled Evolution** |
| **[`REQ-001`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-001.md) / [`REQ-004`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-004.md)** (Parser Contract) | Core HTTP/1.1 zero-allocation parsing and connection lifecycle. | **Harmonized**: `httpparser.ParseRequest` delegates chunk decoding to a streaming `chunkedBodyReader` implementing standard `io.ReadCloser`. Connection lifecycle and keep-alive rules remain intact. | **100% Compliant** |
| **[`ADR-001`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-001.md)** (Reactor Modularity) | Reactor event loops decoupled from socket ownership; parser never wraps raw TCP socket directly. | **Preserved**: `chunkedBodyReader` wraps the existing `bufio.Reader` interface. Physical socket draining or closure on teardown is mediated via cleanly passed transport hooks. | **100% Compliant** |
| **[`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md) / [`REQ-129`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-129.md)** (Outbound Streaming & Memory Boundedness) | Outbound chunked responses; strict memory boundedness ($O(1) \le 32\,\text{KB}$). | **Harmonized & Symmetric**: Inbound chunked reading operates in constant memory ($O(1) \le 32\,\text{KB}$ streaming buffers). In `"normalize"` mode, body pooling is clamped to `MaxBodyBytes`. | **100% Compliant** |
| **[`REQ-131`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-131.md)** (Version SSOT) | Universal SSOT alignment (`v1.5.29`). | **Preserved**: Configuration bindings and emitted headers dynamically bind to `pkg/version`. | **100% Compliant** |
| **[`REQ-132`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-132.md) / [`TASK-155`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-155.md)** (Coverage-Guided Fuzzing) | Fuzz targets validate parser crash immunity, desync guards, and chunk framing (`FuzzChunkFraming`). | **Synergistic**: Fuzz targets in `pkg/httpparser/fuzz_test.go` directly exercise `chunkedBodyReader` and differential oracles against Go's `net/http` chunked decoder. | **100% Synergistic** |

---

### 1.5 Safe Non-Conflicting Ingress Ingestion Pipeline

```mermaid
flowchart TD
    CLIENT["Client Connection<br/>(Raw TCP Ingress)"] --> PARSE["httpparser.ParseRequest()"]
    
    PARSE --> HAS_TE{"Has Transfer-Encoding?"}
    HAS_TE -->|No| STD_REQ["Standard Content-Length / Empty Body"]
    
    HAS_TE -->|Yes| CHECK_CL{"Also has Content-Length?<br/>(RFC 9112 §6.3)"}
    CHECK_CL -->|Yes| SMUGGLE_400["400 Bad Request<br/>Immediate Socket Teardown<br/>(CL.TE Smuggling Blocked)"]
    
    CHECK_CL -->|No| MODE{"Check InboundChunkedMode"}
    MODE -->|"reject"| REJECT_501["501 Not Implemented<br/>(Legacy ADR-056 Mode)"]
    
    MODE -->|"normalize" / "passthrough"| DECODE["Init chunkedBodyReader<br/>Strict RFC 9112 §7.1 Decoder"]
    
    DECODE --> STREAM_LOOP["Read Chunk Size (1*HEXDIG)<br/>Clamp Ext <= 256B<br/>Read Chunk Data<br/>Assert CRLF"]
    
    STREAM_LOOP -->|Syntax Error / Overflow| ERR_400["400 Bad Request<br/>Forceful conn.Close()"]
    STREAM_LOOP -->|Size > MaxBodyBytes| ERR_413["413 Payload Too Large<br/>Forceful conn.Close()"]
    
    STREAM_LOOP -->|Terminal 0\\r\\n| TRAILER["Read Trailers <= 4KB<br/>Filter Forbidden Headers"]
    
    TRAILER --> ROUTE_FWD{"Forwarding Strategy"}
    
    ROUTE_FWD -->|"normalize"| NORM["Buffer Body in Pool<br/>Compute exact Content-Length<br/>Strip Transfer-Encoding<br/>Forward Non-Chunked Req"]
    
    ROUTE_FWD -->|"passthrough"| PASS["Re-frame Canonical Chunks<br/>Forward Stream Upstream"]
    
    NORM --> UPSTREAM["Upstream Microservice<br/>(Node.js / Python / Go)"]
    PASS --> UPSTREAM
```

---

## 2. Work Breakdown Structure (WBS)

```
TASK-156: Inbound Chunked Transfer-Encoding Ingestion, Zero-Tolerance Wire Decoding, and Upstream Re-Framing Normalization
├── WP-1: Streaming Zero-Tolerance Chunked Decoder (pkg/httpparser/chunked.go)
│   ├── Subtask 1.1: chunkedBodyReader Struct & State Machine Implementation
│   ├── Subtask 1.2: Strict RFC 9112 §7.1 Hex Size Parsing (1*HEXDIG)
│   ├── Subtask 1.3: Bounded Chunk Extension Clamping (MaxChunkExtensionBytes <= 256B)
│   ├── Subtask 1.4: Strict Chunk Delimiter & CRLF Boundary Enforcement
│   ├── Subtask 1.5: Cumulative Payload Body Bounding (MaxBodyBytes & HTTP 413)
│   ├── Subtask 1.6: Trailer Header Parsing & RFC 9112 §7.1.2 Prohibitions (<= 4KB)
│   └── Subtask 1.7: Unconsumed Body Cleanup & Socket Drainage on Close()
├── WP-2: Preflight Smuggling Guards & Parser Integration (pkg/httpparser/parser.go)
│   ├── Subtask 2.1: Dual CL+TE Smuggling Guard (Fail-Closed RFC 9112 §6.3)
│   ├── Subtask 2.2: Transfer-Encoding Field Grammar & Obfuscation Validation
│   ├── Subtask 2.3: Parser Pipeline Integration in ParseRequest
│   └── Subtask 2.4: Connection Lifecycle & Teardown Coordination
├── WP-3: Canonical Edge Normalization & Upstream Forwarding (pkg/proxy/proxy.go)
│   ├── Subtask 3.1: Canonical Normalization Pipeline ("normalize" Mode)
│   ├── Subtask 3.2: Canonical Passthrough Streaming Pipeline ("passthrough" Mode)
│   ├── Subtask 3.3: Upstream Hop-by-Hop & Trailer Header Sanitization
│   └── Subtask 3.4: Buffer Pool Management & Zero-Allocation Recycling
├── WP-4: Configurable Ingress Operational Profiles (pkg/config/config.go, pkg/server/server.go)
│   ├── Subtask 4.1: Configuration Schema Extensions (InboundChunkedMode)
│   ├── Subtask 4.2: Route-Level Overrides & Priority Resolution (ProxyRouteConfig)
│   ├── Subtask 4.3: Legacy Reject Mode Preservation (HTTP 501 Not Implemented)
│   └── Subtask 4.4: Dynamic Configuration Validation & Defaults
└── WP-5: Comprehensive Verification & Test Suites (TC-133)
    ├── Subtask 5.1: Unit Test Suite for Chunked Decoder (pkg/httpparser/chunked_test.go)
    ├── Subtask 5.2: Parser Preflight Smuggling Tests (pkg/httpparser/parser_test.go)
    ├── Subtask 5.3: Reverse Proxy Integration Tests (pkg/proxy/proxy_test.go)
    ├── Subtask 5.4: Server End-to-End Operational Profile Tests (pkg/server/server_test.go)
    ├── Subtask 5.5: Generative Fuzzing Engine Alignment (pkg/httpparser/fuzz_test.go)
    └── Subtask 5.6: Concurrency & Race Detector Sweep (go test -race ./...)
```

---

### Work Package 1 (WP-1): Streaming Zero-Tolerance Chunked Decoder (`pkg/httpparser/chunked.go`)

#### Subtask 1.1: `chunkedBodyReader` Struct & State Machine Implementation
- **Target File**: [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - Define `chunkedReaderState` enum representing internal state transitions:
    ```go
    type chunkedReaderState int

    const (
        stateChunkSize chunkedReaderState = iota
        stateChunkData
        stateChunkCRLF
        stateTrailerSection
        stateDone
    )
    ```
  - Implement struct `chunkedBodyReader` satisfying `io.ReadCloser`:
    ```go
    type chunkedBodyReader struct {
        r              *bufio.Reader
        rawConn        net.Conn
        state          chunkedReaderState
        chunkRemaining int64
        totalBodyRead  int64
        maxBodyBytes   int64
        reqHeader      Header
        err            error
        closed         bool
        drainRemaining int64
    }
    ```
  - Implement `Read(p []byte) (n int, err error)` maintaining state transitions:
    - In `stateChunkSize`: Parse chunk hex length and extensions. Transition to `stateChunkData` if size $> 0$, or `stateTrailerSection` if size $== 0$.
    - In `stateChunkData`: Read up to `min(len(p), chunkRemaining)` bytes from `bufio.Reader`. Decrement `chunkRemaining`. If $0$, transition to `stateChunkCRLF`.
    - In `stateChunkCRLF`: Read exactly 2 bytes (`\r\n`). If valid, transition back to `stateChunkSize`.
    - In `stateTrailerSection`: Read trailing headers until blank line (`\r\n`). Transition to `stateDone`.
    - In `stateDone`: Return `0, io.EOF`.
- **Deliverables**:
  - Complete state machine skeleton in [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go).
- **Acceptance Criteria**:
  - Implements standard `io.ReadCloser` without leaks or invalid state transitions.

---

#### Subtask 1.2: Strict RFC 9112 §7.1 Hex Size Parsing (`1*HEXDIG`)
- **Target File**: [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - Enforce RFC 9112 §7.1: `chunk-size = 1*HEXDIG`.
  - Zero-Tolerance parsing algorithm:
    1. Read line using bounded helper until `\n` or `maxBytes` ($256 + 64$).
    2. Strip trailing `\r\n` (reject if bare `\n`).
    3. Split into hex token and optional chunk extension at semicolon `;` or space.
    4. Validate hex token:
       - Length MUST be $\ge 1$. Empty hex token triggers `ErrBadRequest`.
       - Must contain ONLY ASCII hex characters: `0`–`9`, `a`–`f`, `A`–`F`.
       - Leading signs (`+` or `-`) SHALL be rejected with `ErrBadRequest`.
       - Leading or embedded whitespace/tabs SHALL be rejected with `ErrBadRequest`.
    5. Parse integer value using `strconv.ParseUint(hexStr, 16, 64)`. Overflow triggers `ErrBadRequest`.
- **Deliverables**:
  - Strict hex size parsing routine `parseChunkSizeLine` in `chunked.go`.
- **Acceptance Criteria**:
  - Rejects `1G\r\n`, `+10\r\n`, `-5\r\n`, ` 10\r\n`, `0x10\r\n`, `\t10\r\n` with `ErrBadRequest`.

---

#### Subtask 1.3: Bounded Chunk Extension Clamping (`MaxChunkExtensionBytes <= 256B`)
- **Target File**: [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - Constant definition: `const MaxChunkExtensionBytes = 256`.
  - Extension handling:
    - If semicolon `;` follows hex token, measure the remaining bytes on the chunk size line.
    - If cumulative extension bytes exceed $256$ bytes, abort immediately with `ErrBadRequest` (HTTP 400).
    - Inspect extension characters: any control characters (`0x00`–`0x1F` except HTAB `0x09`, and `0x7F`) trigger `ErrBadRequest`.
    - Discard unrecognized extension tokens without heap allocation.
- **Deliverables**:
  - Clamped extension validation logic in `chunked.go`.
- **Acceptance Criteria**:
  - Chunk extensions $> 256$ bytes trigger `ErrBadRequest`; control characters trigger `ErrBadRequest`.

---

#### Subtask 1.4: Strict Chunk Delimiter & CRLF Boundary Enforcement
- **Target File**: [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - In `stateChunkCRLF`:
    - Read exactly 2 bytes via `io.ReadFull(r, crlfBuf[:2])`.
    - Assert `crlfBuf[0] == '\r' && crlfBuf[1] == '\n'`.
    - If delimiter is bare `\n` or contains any other character, set `cr.err = ErrBadRequest` and return error.
  - Reader must never read beyond the declared chunk length before validating the CRLF boundary.
- **Deliverables**:
  - Delimiter enforcement logic in `chunked.go`.
- **Acceptance Criteria**:
  - Bare `\n` or garbage delimiters trigger immediate `ErrBadRequest` and prevent further reading.

---

#### Subtask 1.5: Cumulative Payload Body Bounding (`MaxBodyBytes` & HTTP 413)
- **Target File**: [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - Track `totalBodyRead += int64(n)` across chunk data reads.
  - Check invariant:
    ```go
    if cr.maxBodyBytes > 0 && cr.totalBodyRead > cr.maxBodyBytes {
        cr.err = ErrBodyTooLarge
        if cr.rawConn != nil {
            _ = cr.rawConn.Close() // fail-fast socket teardown
        }
        return n, ErrBodyTooLarge
    }
    ```
  - Ensure `ErrBodyTooLarge` propagates immediately to the caller and server loop, mapping to `HTTP/1.1 413 Payload Too Large`.
- **Deliverables**:
  - Cumulative byte bounding check in `Read()` method.
- **Acceptance Criteria**:
  - Streaming payloads exceeding `opts.MaxBodyBytes` terminate immediately with `ErrBodyTooLarge` and socket closure.

---

#### Subtask 1.6: Trailer Header Parsing & RFC 9112 §7.1.2 Prohibitions (`<= 4KB`)
- **Target File**: [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - Constant definitions:
    ```go
    const MaxTrailerBytes = 4096 // 4 KB
    ```
  - Define set of forbidden trailer headers per RFC 9112 §7.1.2:
    - `Transfer-Encoding`
    - `Content-Length`
    - `Connection`
    - `Host`
    - `Keep-Alive`
    - `TE`
    - `Trailer` / `Trailers`
    - `Upgrade`
    - Pseudo-headers starting with `:` (e.g. `:method`, `:path`)
  - In `stateTrailerSection`:
    - Read trailer lines bounded by cumulative limit ($4\,\text{KB}$). Exceeding $4\,\text{KB}$ returns `ErrHeaderTooLarge` (HTTP 431).
    - Parse `Key: Value` format.
    - If canonical key matches any forbidden header, return `ErrBadRequest` (HTTP 400).
    - Append valid trailers to `cr.reqHeader`.
- **Deliverables**:
  - Trailer parsing and forbidden header validation in `chunked.go`.
- **Acceptance Criteria**:
  - Forbidden headers in trailers trigger `ErrBadRequest`; oversized trailers trigger `ErrHeaderTooLarge`.

---

#### Subtask 1.7: Unconsumed Body Cleanup & Socket Drainage on `Close()`
- **Target File**: [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - Constant definition: `const MaxDrainBytes = 65536` ($64\,\text{KB}$).
  - Implement `Close() error`:
    ```go
    func (cr *chunkedBodyReader) Close() error {
        if cr.closed {
            return nil
        }
        cr.closed = true
        if cr.state == stateDone {
            return nil
        }
        if cr.err != nil {
            if cr.rawConn != nil {
                _ = cr.rawConn.Close()
            }
            return cr.err
        }
        // Attempt bounded socket drainage
        drainBuf := make([]byte, 4096)
        var drained int64
        for cr.state != stateDone && drained < MaxDrainBytes {
            n, err := cr.Read(drainBuf)
            drained += int64(n)
            if errors.Is(err, io.EOF) {
                return nil
            }
            if err != nil {
                break
            }
        }
        if cr.state != stateDone && cr.rawConn != nil {
            _ = cr.rawConn.Close() // Socket could not be drained within 64KB; teardown connection
        }
        return nil
    }
    ```
- **Deliverables**:
  - Robust `Close()` method ensuring no byte leakage into subsequent keep-alive requests.
- **Acceptance Criteria**:
  - Drains up to $64\,\text{KB}$ to preserve keep-alive; forcefully closes TCP socket if unread payload exceeds limit or on parsing error.

---

### Work Package 2 (WP-2): Preflight Smuggling Guards & Parser Integration (`pkg/httpparser/parser.go`)

#### Subtask 2.1: Dual CL+TE Smuggling Guard (Fail-Closed RFC 9112 §6.3)
- **Target File**: [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/parser.go)
- **Scope & Implementation**:
  - In `ParseRequest`, inspect presence of both `Content-Length` and `Transfer-Encoding`:
    ```go
    clValues := req.Header.Values("Content-Length")
    teValues := req.Header.Values("Transfer-Encoding")
    if len(teValues) > 0 && len(clValues) > 0 {
        return nil, fmt.Errorf("%w: conflicting Content-Length and Transfer-Encoding headers", ErrBadRequest)
    }
    ```
  - Ensure failure returns `ErrBadRequest` (HTTP 400), not HTTP 501, and signals connection teardown.
- **Deliverables**:
  - Fail-closed CL+TE rejection in `parser.go`.
- **Acceptance Criteria**:
  - Any request containing both `Content-Length` and `Transfer-Encoding` is immediately rejected with HTTP 400.

---

#### Subtask 2.2: Transfer-Encoding Field Grammar & Obfuscation Validation
- **Target File**: [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/parser.go)
- **Scope & Implementation**:
  - Validate `teValues`:
    1. If empty or whitespace-only (e.g. `Transfer-Encoding:\r\n` or `Transfer-Encoding:   \r\n`), return `ErrBadRequest` (HTTP 400).
    2. Collect all comma-separated codings across all `Transfer-Encoding` header lines:
       - Strip whitespace per RFC 9112 §6.1.
       - Final encoding MUST be `chunked` (case-insensitive).
       - If final encoding is not `chunked`, return `ErrUnsupportedTransferEncoding` (HTTP 501).
       - If multiple codings exist and any non-chunked coding is present (e.g. `gzip, chunked`), verify or reject unsupported codings with `ErrUnsupportedTransferEncoding`.
    3. Detect obfuscation:
       - Reject horizontal tab immediately following colon (`Transfer-Encoding:\tchunked`) with `ErrBadRequest`.
       - Reject null bytes or illegal control characters in header value with `ErrBadRequest`.
- **Deliverables**:
  - Robust `validateTransferEncodingHeader` helper in `parser.go`.
- **Acceptance Criteria**:
  - Passes all RFC 7230 / RFC 9112 Transfer-Encoding edge-case tests; rejects obfuscations with HTTP 400.

---

#### Subtask 2.3: Parser Pipeline Integration in `ParseRequest`
- **Target File**: [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/parser.go)
- **Scope & Implementation**:
  - When valid `Transfer-Encoding: chunked` is confirmed:
    - Set `req.ContentLength = -1`.
    - Instantiate `chunkedBodyReader`:
      ```go
      req.Body = newChunkedBodyReader(bufr, req.RawConn, opts.MaxBodyBytes, req.Header)
      ```
    - Bypass standard `Content-Length` body reading block.
    - Return initialized `*Request` with streaming `req.Body`.
- **Deliverables**:
  - Streaming chunked reader integration in `ParseRequest`.
- **Acceptance Criteria**:
  - `ParseRequest` successfully parses request headers and provides an `io.Reader` body streaming chunked data.

---

#### Subtask 2.4: Connection Lifecycle & Teardown Coordination
- **Target File**: [`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go), [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/parser.go)
- **Scope & Implementation**:
  - Ensure any chunked framing error (`ErrBadRequest`, `ErrBodyTooLarge`, `ErrHeaderTooLarge`) encountered during request parsing or body streaming triggers immediate connection closure:
    - In `server.go`, assert `res.Header.Set("Connection", "close")`.
    - If body reader encounters error mid-stream, invoke `rawConn.Close()`.
- **Deliverables**:
  - Connection teardown hooks in `pkg/server/server.go`.
- **Acceptance Criteria**:
  - Broken chunk streams cause physical TCP socket teardown, preventing request pipelining contamination.

---

### Work Package 3 (WP-3): Canonical Edge Normalization & Upstream Forwarding (`pkg/proxy/proxy.go`)

#### Subtask 3.1: Canonical Normalization Pipeline (`"normalize"` Mode)
- **Target File**: [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go)
- **Scope & Implementation**:
  - In `ReverseProxy.ServeHTTPWithPrefix`:
    - Determine active `inbound_chunked_mode` for the route (default: `"normalize"`).
    - If `req.ContentLength == -1 && req.Body != nil`:
      - Mode `"normalize"`:
        1. Acquire byte buffer from `bodyBufferPool` (or allocate bounded slice up to `maxBodyBytes`).
        2. Read entire chunked body: `decodedBytes, err := io.ReadAll(req.Body)`.
        3. If error occurs, write 400 Bad Request or 413 Payload Too Large and return.
        4. Calculate exact length: `exactCL := int64(len(decodedBytes))`.
        5. Remove `Transfer-Encoding` header from `outReq.Header`.
        6. Set `outReq.Header.Set("Content-Length", strconv.FormatInt(exactCL, 10))`.
        7. Set `outReq.ContentLength = exactCL`.
        8. Provide `bytes.NewReader(decodedBytes)` as body to `outReq`.
- **Deliverables**:
  - Edge normalization routine in `proxy.go`.
- **Acceptance Criteria**:
  - Upstream requests in `"normalize"` mode carry exact `Content-Length` and NO `Transfer-Encoding` header.

---

#### Subtask 3.2: Canonical Passthrough Streaming Pipeline (`"passthrough"` Mode)
- **Target File**: [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go)
- **Scope & Implementation**:
  - If mode is `"passthrough"`:
    - Pass streaming `req.Body` directly to upstream `outReq`.
    - Ensure `outReq.ContentLength = -1`.
    - Upstream transport (`http.Transport`) transmits standard canonical chunks.
    - If client stream errors or disconnects during transfer, abort upstream request context immediately (`req.Context().Cancel()`).
- **Deliverables**:
  - Passthrough streaming pipeline in `proxy.go`.
- **Acceptance Criteria**:
  - Upstream request forwards streaming chunks without full edge buffering.

---

#### Subtask 3.3: Upstream Hop-by-Hop & Trailer Header Sanitization
- **Target File**: [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go)
- **Scope & Implementation**:
  - Ensure RFC 7230 §6.1 hop-by-hop headers (`Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailers`, `Upgrade`) are stripped before forwarding.
  - In `"normalize"` mode, unconditionally strip `Transfer-Encoding`.
  - In `"normalize"` mode, merge decoded trailers from `req.Header` into upstream request headers (or trailing headers).
- **Deliverables**:
  - Header stripping and trailer forwarding logic in `proxy.go`.
- **Acceptance Criteria**:
  - Hop-by-hop headers cleanly sanitized; trailers forwarded without corruption.

---

#### Subtask 3.4: Buffer Pool Management & Zero-Allocation Recycling
- **Target File**: [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go), [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go)
- **Scope & Implementation**:
  - In `chunked.go`: Hex string parsing uses stack buffers or small pooled buffers to eliminate per-chunk allocations.
  - In `proxy.go`: Payload normalization uses `bodyBufferPool` for payloads $\le 64\,\text{KB}$. Return buffer to pool after upstream request finishes.
- **Deliverables**:
  - Zero-allocation chunk state tracking and buffer pool recycling.
- **Acceptance Criteria**:
  - Zero heap allocations during chunk boundary parsing; buffer pool recycling verified under high concurrency.

---

### Work Package 4 (WP-4): Configurable Ingress Operational Profiles (`pkg/config/config.go`, `pkg/server/server.go`)

#### Subtask 4.1: Configuration Schema Extensions (`InboundChunkedMode`)
- **Target File**: [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go)
- **Scope & Implementation**:
  - Add `InboundChunkedMode` to `ServerConfig`:
    ```go
    // InboundChunkedMode defines the policy for handling incoming client Transfer-Encoding: chunked requests.
    // Options: "normalize" (default, de-chunk to Content-Length), "reject" (legacy ADR-056 501), "passthrough".
    InboundChunkedMode string `yaml:"inbound_chunked_mode" json:"inbound_chunked_mode"`
    ```
  - Add `InboundChunkedMode` to `ProxyTransportConfig` and `ProxyRouteConfig`.
- **Deliverables**:
  - Configuration schema fields in `config.go`.
- **Acceptance Criteria**:
  - Schema serializes and deserializes YAML/JSON configuration files with `inbound_chunked_mode`.

---

#### Subtask 4.2: Route-Level Overrides & Priority Resolution (`ProxyRouteConfig`)
- **Target File**: [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go), [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go)
- **Scope & Implementation**:
  - Route priority rule:
    1. If `ProxyRouteConfig.InboundChunkedMode` is non-empty, use route-level mode.
    2. Else if `ProxyTransportConfig.InboundChunkedMode` is non-empty, use transport mode.
    3. Else fallback to `ServerConfig.InboundChunkedMode`.
    4. Default is `"normalize"`.
- **Deliverables**:
  - Mode resolution helper in `proxy.go`.
- **Acceptance Criteria**:
  - Route-level settings cleanly override global server settings.

---

#### Subtask 4.3: Legacy Reject Mode Preservation (HTTP 501 Not Implemented)
- **Target File**: [`pkg/server/server.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go), [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/parser.go)
- **Scope & Implementation**:
  - When `inbound_chunked_mode == "reject"`:
    - Any request with `Transfer-Encoding` is rejected with `ErrUnsupportedTransferEncoding`.
    - Server responds with `HTTP/1.1 501 Not Implemented: Inbound chunked transfer encoding is disabled` and closes the TCP connection.
    - Preserves exact legacy [`ADR-056`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-056.md) behavior.
- **Deliverables**:
  - Legacy rejection path in `server.go`.
- **Acceptance Criteria**:
  - In `"reject"` mode, chunked requests return HTTP 501 and close socket.

---

#### Subtask 4.4: Dynamic Configuration Validation & Defaults
- **Target File**: [`pkg/config/config.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go)
- **Scope & Implementation**:
  - In `Validate()`:
    - Validate `InboundChunkedMode` matches one of `"normalize"`, `"reject"`, `"passthrough"`, or empty (defaults to `"normalize"`).
    - Invalid values trigger configuration load error.
- **Deliverables**:
  - Validation logic in `config.go`.
- **Acceptance Criteria**:
  - Invalid modes (e.g. `"allow"`, `"stream"`) are rejected during config parsing with clear error messages.

---

### Work Package 5 (WP-5): Comprehensive Verification & Test Suites (`TC-133`)

#### Subtask 5.1: Unit Test Suite for Chunked Decoder (`pkg/httpparser/chunked_test.go`)
- **Target File**: [`pkg/httpparser/chunked_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked_test.go)
- **Scope & Implementation**:
  - Implement unit test table covering:
    - Single chunk: `4\r\nWiki\r\n0\r\n\r\n` -> `"Wiki"`.
    - Multiple chunks: `4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n` -> `"Wikipedia"`.
    - Empty payload: `0\r\n\r\n` -> `""`.
    - Non-hex characters: `1G\r\n`, `ZZ\r\n` -> `ErrBadRequest`.
    - Signed hex: `+10\r\n`, `-5\r\n` -> `ErrBadRequest`.
    - Whitespace before hex: ` 10\r\n`, `\t10\r\n` -> `ErrBadRequest`.
    - Chunk extension $\le 256\,\text{B}$ accepted; $> 256\,\text{B}$ rejected.
    - Control characters in extension rejected.
    - Bare `\n` delimiter rejected.
    - Cumulative payload $> \text{MaxBodyBytes}$ returns `ErrBodyTooLarge`.
    - Trailers $\le 4\,\text{KB}$ parsed; $> 4\,\text{KB}$ returns `ErrHeaderTooLarge`.
    - Forbidden headers in trailers (`Content-Length`, `Transfer-Encoding`, `Host`, etc.) trigger `ErrBadRequest`.
    - Socket drainage up to $64\,\text{KB}$ on `Close()`.
- **Deliverables**:
  - Comprehensive unit test file `chunked_test.go` with $> 95\%$ code coverage.
- **Acceptance Criteria**:
  - All unit test cases pass.

---

#### Subtask 5.2: Parser Preflight Smuggling Tests (`pkg/httpparser/parser_test.go`)
- **Target File**: [`pkg/httpparser/parser_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/parser_test.go)
- **Scope & Implementation**:
  - Add tests for:
    - Conflicting `Content-Length` and `Transfer-Encoding` -> HTTP 400.
    - Obfuscated `Transfer-Encoding:\tchunked` -> HTTP 400.
    - Empty `Transfer-Encoding:` -> HTTP 400.
    - Unsupported encoding (e.g. `Transfer-Encoding: gzip`) -> HTTP 501.
- **Deliverables**:
  - Updated test cases in `parser_test.go`.
- **Acceptance Criteria**:
  - All smuggling preflight tests pass.

---

#### Subtask 5.3: Reverse Proxy Integration Tests (`pkg/proxy/proxy_test.go`)
- **Target File**: [`pkg/proxy/proxy_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy_test.go)
- **Scope & Implementation**:
  - Setup upstream mock test server.
  - Test `"normalize"` mode: Client sends chunked request; mock upstream verifies receiving clean `Content-Length` request without `Transfer-Encoding`.
  - Test `"passthrough"` mode: Mock upstream receives canonical chunked request.
  - Test trailer header forwarding upstream.
  - Test hop-by-hop header removal.
- **Deliverables**:
  - Integration test cases in `proxy_test.go`.
- **Acceptance Criteria**:
  - Upstream request inspection confirms canonical re-framing.

---

#### Subtask 5.4: Server End-to-End Operational Profile Tests (`pkg/server/server_test.go`)
- **Target File**: [`pkg/server/server_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server_test.go)
- **Scope & Implementation**:
  - Test live TCP server with all three profiles:
    - `"normalize"` profile: Full POST request with chunked body returns 200 OK.
    - `"reject"` profile: Chunked POST returns 501 Not Implemented and closes connection.
    - `"passthrough"` profile: Chunked POST streams to backend handler.
- **Deliverables**:
  - E2E tests in `server_test.go`.
- **Acceptance Criteria**:
  - All three profiles function as specified over live TCP connections.

---

#### Subtask 5.5: Generative Fuzzing Engine Alignment (`pkg/httpparser/fuzz_test.go`)
- **Target File**: [`pkg/httpparser/fuzz_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/fuzz_test.go)
- **Scope & Implementation**:
  - Align `FuzzChunkFraming` with `chunkedBodyReader`:
    - Ensure mutated chunk headers, extensions, delimiters, and trailers are exercised.
    - Verify Differential Oracle against Go standard library `net/http.ReadRequest`.
    - Zero crashes, zero panics, zero unhandled desynchronizations.
- **Deliverables**:
  - Fuzzing integration in `fuzz_test.go`.
- **Acceptance Criteria**:
  - Passes 100,000+ fuzzing mutations without panics or memory leaks.

---

#### Subtask 5.6: Concurrency & Race Detector Sweep (`go test -race ./...`)
- **Target Scope**: Entire repository
- **Scope & Implementation**:
  - Execute full test suite under Go race detector:
    ```bash
    go test -race ./pkg/httpparser/...
    go test -race ./pkg/proxy/...
    go test -race ./pkg/server/...
    go test -race ./...
    ```
- **Deliverables**:
  - Clean race detector execution report.
- **Acceptance Criteria**:
  - Zero race warnings or deadlocks under `go test -race`.

---

## 3. The 4 Non-Negotiable Invariants

```
+--------------------------------------------------------------------------------------------------+
│                                  THE 4 NON-NEGOTIABLE INVARIANTS                                 │
│                                                                                                  │
│  1. ZERO EXTERNAL THIRD-PARTY DEPENDENCIES:                                                      │
│     All chunked decoding, trailer parsing, and re-framing logic MUST use pure Go standard        │
│     library packages (io, bufio, bytes, strconv, sync). No third-party parsers allowed.          │
│                                                                                                  │
│  2. CORE REACTOR MODULARITY PRESERVED (ADR-001):                                                 │
│     The chunked reader operates on io.Reader byte streams. The reactor event loop, epoll/kqueue  │
│     worker routines, and socket lifecycle remain decoupled and 100% untouched.                   │
│                                                                                                  │
│  3. MEMORY BOUNDEDNESS & O(1) STREAMING FOOTPRINT (ADR-129):                                     │
│     Chunk decoding MUST execute in constant O(1) <= 32KB streaming memory. In "normalize" mode, │
│     buffered payloads MUST be strictly clamped to MaxBodyBytes and use sync.Pool recycling.     │
│                                                                                                  │
│  4. ZERO DATA RACES UNDER `go test -race ./...`:                                                 │
│     All reader operations, buffer pool accesses, and configuration state reads must be 100%      │
│     race-free under high-concurrency evaluation.                                                 │
+--------------------------------------------------------------------------------------------------+
```

---

## 4. Acceptance Criteria & Verification

### 4.1 Functional Acceptance Criteria
- [ ] **`chunkedBodyReader` Implementation**: Dedicated `io.ReadCloser` in [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go).
- [ ] **Strict Hex Parsing**: Non-hex characters, leading signs (`+`, `-`), or leading whitespace trigger immediate `ErrBadRequest` (HTTP 400).
- [ ] **Chunk Extensions Clamped**: Extensions $> 256\,\text{B}$ or containing control characters trigger `ErrBadRequest` (HTTP 400).
- [ ] **CRLF Boundary Enforcement**: Missing or malformed CRLF after chunk data triggers `ErrBadRequest` (HTTP 400).
- [ ] **Cumulative Body Bounding**: Decoded payload exceeding `MaxBodyBytes` triggers `ErrBodyTooLarge` (HTTP 413) and closes connection.
- [ ] **Trailer Limits**: Trailers $> 4\,\text{KB}$ trigger `ErrHeaderTooLarge` (HTTP 431).
- [ ] **Forbidden Trailer Blocking**: Prohibited headers (`Transfer-Encoding`, `Content-Length`, `Connection`, `Host`, etc.) trigger `ErrBadRequest` (HTTP 400).
- [ ] **Socket Drainage on Close**: Unread payload drains up to $64\,\text{KB}$ within 100ms or physically terminates TCP socket.
- [ ] **CL.TE Smuggling Blocked**: Requests with both `Content-Length` and `Transfer-Encoding` are rejected with HTTP 400 and socket teardown.
- [ ] **Obfuscated TE Blocked**: Empty TE or obfuscated TE (`Transfer-Encoding:\tchunked`) rejected with HTTP 400.
- [ ] **Canonical Normalization**: `"normalize"` mode de-chunks client stream, computes exact `Content-Length`, strips `Transfer-Encoding`, and forwards standard request upstream.
- [ ] **Canonical Passthrough**: `"passthrough"` mode re-frames canonical chunks upstream without buffering.
- [ ] **Configurable Profiles**: `inbound_chunked_mode` (`"normalize"`, `"reject"`, `"passthrough"`) supported in `ServerConfig` and `ProxyRouteConfig`.
- [ ] **Legacy 501 Preservation**: `"reject"` mode returns `501 Not Implemented` and closes connection.

### 4.2 Non-Functional, Performance & Robustness Criteria
- [ ] **Zero External Dependencies**: Standard library only; no new packages added to `go.mod`.
- [ ] **Memory Boundedness**: Constant $O(1) \le 32\,\text{KB}$ streaming reader footprint; `sync.Pool` buffer recycling.
- [ ] **Decoding Overhead**: $< 5\%$ CPU overhead compared to raw `Content-Length` streaming.
- [ ] **Normalization Latency**: $< 50\,\mu\text{s}$ edge re-framing for payloads $\le 64\,\text{KB}$.
- [ ] **Race Detector Cleanliness**: 100% test pass rate under `go test -race ./...`.
- [ ] **Fuzzing Robustness**: `FuzzChunkFraming` passes with zero panics, zero crashes, zero desyncs.

---

## 5. Threat Modeling & Security Risk Mitigation Matrix

| Threat Scenario | Failure Mode / Attack Vector | Impact | Mitigation in TASK-156 |
| :--- | :--- | :--- | :--- |
| **CL.TE Smuggling ([CWE-444](https://cwe.mitre.org/data/definitions/444.html))** | Client provides both `Content-Length` and `Transfer-Encoding: chunked`. | Front-end and back-end interpret different body boundaries. | Preflight guard strictly rejects dual headers with `HTTP 400` and closes TCP socket. |
| **Non-Hex Size Obfuscation** | Client sends signed hex (`+10\r\n`), non-hex (`0x10\r\n`, `ZZ\r\n`), or leading whitespace. | Downstream parsers may deserialize chunks differently. | Strict `1*HEXDIG` enforcement. Any non-hex character triggers immediate `HTTP 400` + socket reset. |
| **Chunk Extension DoS ([CWE-400](https://cwe.mitre.org/data/definitions/400.html))** | Client sends megabytes of chunk extensions (`10;ext=AAAA...`) to exhaust heap memory. | Heap exhaustion / memory spike on gateway. | Extensions strictly bounded to $\le 256$ bytes (`MaxChunkExtensionBytes`); excess triggers `HTTP 400`. |
| **Unbounded Body / Zip Bomb** | Client streams infinite chunks without `Content-Length`. | Gateway or backend disk/memory exhaustion. | Cumulative payload body strictly clamped to `MaxBodyBytes` (default 4MB); excess triggers `HTTP 413`. |
| **Trailer Header Smuggling** | Attacker inserts `Transfer-Encoding`, `Content-Length`, or `Host` into chunk trailers. | Secondary request smuggling or routing hijack in downstream proxy. | Prohibited header list strictly enforces RFC 9112 §7.1.2; presence triggers immediate `HTTP 400`. |
| **Downstream Parsing Bugs** | Fragile downstream backend (e.g. Node.js `llhttp` or Python `h11`) has zero-day chunk parser flaw. | Vulnerability exploited on origin service behind Toron. | In `"normalize"` mode, Toron converts chunked stream to standard `Content-Length` request, completely shielding backends. |
| **Pipelined Byte Leakage** | Client leaves unconsumed chunk bytes on socket after request completes. | Leftover bytes read as next request on keep-alive connection. | `Close()` drains up to 64KB or forcefully terminates the TCP connection (`conn.Close()`). |

---

## 6. Open Questions & Architectural Resolutions

- **Open Question 1: Should `"normalize"` or `"passthrough"` be the default mode for inbound chunked requests?**
  - *Resolution*: `"normalize"` SHALL be the default mode. Normalizing chunked requests into clean `Content-Length` requests at the edge transforms Toron into an active smuggling firewall that eliminates downstream desynchronization vulnerabilities across heterogeneous microservices. `"passthrough"` is available for routes requiring unbounded streaming.
- **Open Question 2: Does supporting inbound chunked requests weaken Toron's security compared to the legacy ADR-056 501 rejection?**
  - *Resolution*: No. By combining zero-tolerance RFC 9112 validation at the edge with upstream canonical normalization (converting chunks to verified `Content-Length`), Toron provides stronger security for backend systems than merely passing through or requiring backends to parse chunked streams themselves. Operators requiring the legacy perimeter posture can retain `"reject"` mode.
- **Open Question 3: How does the decoder handle chunk extensions with quotes or parameters?**
  - *Resolution*: RFC 9112 §7.1.1 specifies that recipients must ignore unrecognized chunk extensions. Toron parses the extension up to the CRLF, enforces the $256$-byte limit, validates that no non-printable control characters exist, and discards the extensions without allocating strings.
- **Open Question 4: Under what conditions should the socket be closed on `Close()`?**
  - *Resolution*: If the request handler finishes without reading the entire chunked body, Toron attempts to drain up to $64\,\text{KB}$ within a short deadline ($100\,\text{ms}$) to preserve keep-alive reuse. If more than $64\,\text{KB}$ remains or a syntax error was encountered, the physical TCP connection is immediately closed.

---

## 7. Traceability Matrix

| Requirement / Artifact | Relationship | Description / Verification Target |
| :--- | :--- | :--- |
| **[`REQ-133 §2.1`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-133.md#L149-L243)** | Implements | WP-1: Streaming zero-tolerance chunked decoder in [`pkg/httpparser/chunked.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/chunked.go). |
| **[`REQ-133 §2.2`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-133.md#L245-L260)** | Implements | WP-2: Preflight smuggling guards (CL.TE rejection and TE validation) in [`pkg/httpparser/parser.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/httpparser/parser.go). |
| **[`REQ-133 §2.3`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-133.md#L262-L321)** | Implements | WP-3: Canonical edge normalization and upstream re-framing in [`pkg/proxy/proxy.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/proxy.go). |
| **[`REQ-133 §2.4`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-133.md#L322-L355)** | Implements | WP-4: Configurable ingress operational profiles (`"normalize"`, `"reject"`, `"passthrough"`) in [`pkg/config`](file:///Users/sneha/Developer/toron-research/toron/pkg/config/config.go) and [`pkg/server`](file:///Users/sneha/Developer/toron-research/toron/pkg/server/server.go). |
| **[`ADR-056`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-056.md) / [`REQ-061`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-061.md)** | Evolves / Supersedes | Evolves static 501 rejection into zero-tolerance ingestion; preserves 501 as configurable `"reject"` mode. |
| **[`REQ-001`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-001.md) / [`REQ-004`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-004.md)** | Conforms To | Preserves zero-allocation parser contract and connection lifecycle semantics. |
| **[`ADR-001`](file:///Users/sneha/Developer/toron-research/toron/docs/architecture/ADR-001.md)** | Preserves | Core reactor modularity and transport isolation remain untouched. |
| **[`REQ-128`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-128.md) / [`REQ-129`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-129.md)** | Harmonizes With | Complements outbound streaming chunking with inbound streaming ingestion while preserving memory boundedness. |
| **[`REQ-131`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-131.md)** | Preserves | Universal SSOT alignment (`v1.5.29`). |
| **[`REQ-132`](file:///Users/sneha/Developer/toron-research/toron/docs/requirements/REQ-132.md) / [`TASK-155`](file:///Users/sneha/Developer/toron-research/toron/docs/tasks/TASK-155.md)** | Synergizes With | Generative fuzzing engine directly evaluates `FuzzChunkFraming` against `chunkedBodyReader`. |
| **`TC-133`** | Verified By | Test Case Specification verifying zero-tolerance decoding, smuggling guards, normalization, and profiles. |
| **`ADR-133`** | Decided By | Architectural Decision Record governing inbound chunked decoding and canonical edge normalization. |
| **`TASK-156`** | Self-Reference | Engineering task specification decomposing REQ-133 into implementation work packages. |
