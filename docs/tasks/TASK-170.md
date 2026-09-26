---
id: TASK-170
type: task
title: Two-Phase HTTP Request Parsing and Bounded Streaming Body Reader in pkg/httpparser
status: ready
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-25
updated: 2026-09-25

depends_on:
  - ../requirements/REQ-145.md
  - ./TASK-169.md

derived_from:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md

implements:
  - ../requirements/REQ-145.md

related_to:
  - ../requirements/REQ-145.md
  - ../analysis/AN-006.md
  - ../architecture/ADR-084.md
  - ../requirements/REQ-133.md
  - ./TASK-169.md
  - ./TASK-171.md
  - ./TASK-172.md
---

# TASK-170 - Two-Phase HTTP Request Parsing and Bounded Streaming Body Reader in pkg/httpparser

## 1. Overview & Objective

Decompose the two-phase HTTP request parsing requirements specified in [`REQ-145`](../requirements/REQ-145.md) (FR-145-1, FR-145-2) and analyzed in [`AN-006`](../analysis/AN-006.md) (Section 3.1) into concrete engineering deliverables in [`pkg/httpparser/parser.go`](../../pkg/httpparser/parser.go).

Currently, `httpparser.ParseRequest(br, opts)` eagerly consumes and buffers incoming request bodies into heap memory (`make([]byte, clInt)`) or pooled 64KB buffers before route matching or authorization takes place. If an incoming payload exceeds the global default `opts.MaxBodyBytes` (4MB), the request is rejected with `ErrBodyTooLarge` before Toron can inspect route-level configurations. Furthermore, globally raising `MaxBodyBytes` creates an instantaneous Out-Of-Memory (OOM) vulnerability ([`SEC-28`](../architecture/ADR-084.md) / [CWE-770](https://cwe.mitre.org/data/definitions/770.html)) when attackers declare large `Content-Length` headers.

This task decouples request line and header parsing from payload body ingestion, producing:
1. `ParseHeader`: Ingests request line and headers with zero heap allocation for the request body.
2. `NewStreamingBodyReader`: Creates an on-demand bounded streaming reader enforcing byte limits without eager buffering.
3. Preservation of full backward compatibility for callers of `ParseRequest`.

---

## 2. Traceability

- **Requirement**: [`REQ-145`](../requirements/REQ-145.md) (FR-145-1, FR-145-2, NFR-145-1, AC-145-01, AC-145-02)
- **Analysis**: [`AN-006`](../analysis/AN-006.md) (Section 3.1, Section 8.2 Item 1)
- **Dependencies**: [`TASK-169.md`](./TASK-169.md) (Configuration Schema Extensions)
- **Downstream Tasks**:
  - [`TASK-171.md`](./TASK-171.md): Activity-Refreshed Read Deadline Tracker in `pkg/server`
  - [`TASK-172.md`](./TASK-172.md): Zero-Copy Request Body Proxy Streaming in `pkg/proxy`

---

## 3. Work Package Breakdown

### WP-1: Phase 1 Header Parsing Decoupling (`ParseHeader`) ([`pkg/httpparser/parser.go`](../../pkg/httpparser/parser.go))

Extract the header ingestion pipeline from `ParseRequest` into a dedicated export:
```go
// ParseHeader parses the HTTP request line and headers from r into a Request metadata structure.
// It performs all RFC 7230 / RFC 9112 protocol validation, smuggling checks, and Content-Length
// parsing without reading any request body bytes into memory. req.Body is left uninitialized.
func ParseHeader(r io.Reader, opts ParserOptions) (*Request, *bufio.Reader, error)
```
Implementation Requirements:
1. **Pooled Buffer Ingestion**: Use `lineBufferPool` to parse the request line and header fields bounded by `opts.MaxHeaderBytes` (default: 8KB). Return `ErrHeaderTooLarge` if the header section exceeds this ceiling.
2. **Grammar & Smuggling Guards**:
   - Enforce RFC 7230 §3.2.6 token character grammar on header field names.
   - Prohibit bare CR / LF characters.
   - Enforce HTTP request smuggling guards: reject requests containing both `Transfer-Encoding` and `Content-Length`.
   - Validate and normalize multiple `Content-Length` headers; return `ErrBadRequest` on conflicting or non-numeric values.
3. **Content-Length Resolution**:
   - For requests with valid `Content-Length`, assign `req.ContentLength = clInt` and normalize the header.
   - For chunked requests (`Transfer-Encoding: chunked`), assign `req.ContentLength = -1`.
   - For requests without body indicators (e.g. standard GET/HEAD/OPTIONS), assign `req.ContentLength = 0`.
4. **Zero Body Allocation**: Ensure `ParseHeader` reads strictly up to the empty line separating headers from body (`\r\n\r\n` or `\n\n`), returning the active `*bufio.Reader` so subsequent body reads retain buffered bytes without loss.

### WP-2: Bounded Streaming Body Reader (`NewStreamingBodyReader`) ([`pkg/httpparser/parser.go`](../../pkg/httpparser/parser.go))

Implement a streaming body reader that limits consumed bytes without eager heap allocation:
```go
// StreamingBodyReader wraps an underlying reader with an uncompressed byte ceiling limit.
type StreamingBodyReader struct {
    r         io.Reader
    remaining int64
    totalRead int64
    limit     int64
    closer    io.Closer
}

// NewStreamingBodyReader returns an io.ReadCloser that permits reading up to limit bytes.
// If the stream exceeds limit bytes, Read returns ErrBodyTooLarge.
func NewStreamingBodyReader(r io.Reader, limit int64, closer io.Closer) *StreamingBodyReader
```
Implementation Details:
- Enforce constant $O(1)$ memory consumption ($< 64\,\text{KB}$).
- On each `Read(p []byte)`, read from `r` up to `remaining`.
- If client stream attempts to deliver more than `limit` bytes, return `ErrBodyTooLarge`.
- On `Close()`, invoke underlying `closer` if non-nil.

### WP-3: Streaming Chunked Reader Route-Ceiling Integration ([`pkg/httpparser/chunked.go`](../../pkg/httpparser/chunked.go))

1. Ensure `newChunkedBodyReader` can accept a dynamically resolved route ceiling (`maxBodyBytes int64`) rather than relying on parser-time global options.
2. Expose an initialization helper:
   ```go
   func AttachChunkedBodyReader(req *Request, r *bufio.Reader, closer io.Closer, maxBodyBytes int64)
   ```
3. Maintain RFC 9112 chunk boundary validation, chunk extension parsing, and chunked trailer capture during stream ingestion.

### WP-4: Backward Compatibility Layer for Existing Callers ([`pkg/httpparser/parser.go`](../../pkg/httpparser/parser.go))

Refactor `ParseRequest(r io.Reader, opts ParserOptions) (*Request, error)`:
```go
func ParseRequest(r io.Reader, opts ParserOptions) (*Request, error) {
    req, bufr, err := ParseHeader(r, opts)
    if err != nil {
        return nil, err
    }
    // Fast-fail check against global opts.MaxBodyBytes
    if req.ContentLength > opts.MaxBodyBytes {
        return nil, ErrBodyTooLarge
    }
    // Existing pooling / buffering behavior for backward compatibility
    // ...
    return req, nil
}
```
All existing unit tests and callers expecting eager buffering in `ParseRequest` must continue to pass without code changes.

### WP-5: Automated Unit Tests ([`pkg/httpparser/parser_test.go`](../../pkg/httpparser/parser_test.go))

1. **Header Parsing Isolation**: Submit a POST request with headers and a 10MB body; verify `ParseHeader` returns immediately with `req.ContentLength = 10485760` and `req.Body == nil` without reading body bytes from the input reader.
2. **Header Too Large Fast-Fail**: Submit request with 10KB headers when `MaxHeaderBytes` is 8KB; verify `ErrHeaderTooLarge`.
3. **Conflicting Content-Length & Smuggling**: Verify `ParseHeader` detects multiple conflicting Content-Length values and conflicting Transfer-Encoding headers.
4. **StreamingBodyReader Enforcing Limits**:
   - Stream 1MB through `NewStreamingBodyReader(r, 2*1024*1024, nil)`; verify successful EOF and correct byte count.
   - Stream 3MB through `NewStreamingBodyReader(r, 2*1024*1024, nil)`; verify `ErrBodyTooLarge` is returned when byte 2,097,153 is reached.
5. **Memory Allocation Verification**: Benchmark `NewStreamingBodyReader` against a 100MB stream; verify heap allocations are $0$ additional megabytes ($O(1)$ constant).
6. **Backward Compatibility Parity**: Run entire existing `parser_test.go` suite to verify `ParseRequest` behavior remains identical.

---

## 4. Acceptance Criteria

- [ ] **AC-170-1 (Two-Phase Separation)**: `ParseHeader` parses method, URI, protocol, headers, and `Content-Length` without reading any request body bytes from the network socket or allocating body slices.
- [ ] **AC-170-2 (Header Ceiling Enforcement)**: Headers exceeding `MaxHeaderBytes` trigger `ErrHeaderTooLarge` (mapped to `HTTP 431 Request Header Fields Too Large`).
- [ ] **AC-170-3 (Smuggling Rejection)**: Conflicting `Content-Length` and `Transfer-Encoding` headers are rejected with `ErrBadRequest`.
- [ ] **AC-170-4 (Bounded Streaming)**: `NewStreamingBodyReader` streams request bodies in constant $O(1)$ memory buffer chunks and returns `ErrBodyTooLarge` immediately if cumulative bytes exceed the configured limit.
- [ ] **AC-170-5 (Chunked Route-Limit Attachment)**: Chunked body reader correctly attaches to `req.Body` using a route-specified body ceiling.
- [ ] **AC-170-6 (Backward Compatibility)**: Existing `httpparser.ParseRequest` callers continue to function identically with existing behavior and pass all tests.

---

## 5. Constraints & Standards

- **Strictly Relative Links**: All document links must be strictly relative (`../requirements/REQ-145.md`, `../../pkg/httpparser/parser.go`).
- **Zero Monolithic Heap Allocation**: Prohibit `make([]byte, clInt)` for body streams in `ParseHeader` and `NewStreamingBodyReader`.
- **Zero External Dependencies**: Pure Go standard library (`bufio`, `bytes`, `errors`, `fmt`, `io`, `net`, `strconv`, `strings`, `sync`).
