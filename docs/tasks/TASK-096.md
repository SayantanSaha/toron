---
id: TASK-096
type: task
title: Automated Verification Suite for Layer 4 TCP & UDP Proxy Hardening
status: approved
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-09
updated: 2026-09-09

depends_on:
  - REQ-087
  - TASK-093
  - TASK-094
  - TASK-095

owns:
  - pkg/proxy/tcp_test.go
  - pkg/proxy/udp_test.go

references:
  - REQ-087
  - SEC-26
  - SR-081
  - ADR-082
  - TC-087

derived_from:
  - REQ-087

implements:
  - REQ-087

verified_by:
  - TC-087
---

# TASK-096 - Automated Verification Suite for Layer 4 TCP & UDP Proxy Hardening

## Overview

Implement a comprehensive automated verification test suite in [`pkg/proxy/tcp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp_test.go) and [`pkg/proxy/udp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp_test.go) verifying bounded connection concurrency, bidirectional idle timeout enforcement, UDP worker pool bounding, upstream UDP socket reuse across datagrams, UDP buffer pooling, session expiration, and race-free deterministic shutdown, validating test specification [`TC-087`](file:///Users/sneha/Developer/toron-research/toron/docs/testCases/TC-087.md) and resolving [`SEC-26`](file:///Users/sneha/Developer/toron-research/toron/SECURITY_AUDIT.md#L375-L383).

## Scope & Implementation Breakdown

1. **TCP Connection Limit & Over-Capacity Rejection ([`pkg/proxy/tcp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp_test.go))**:
   - `TestTCPProxy_MaxConnections`:
     - Configure [`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L15) with `MaxConnections: 2`.
     - Open 2 simultaneous TCP connections and keep them active.
     - Attempt to open a 3rd TCP connection: assert that the connection is immediately closed/rejected by the proxy without forward relaying.
     - Close one of the active connections; verify that a subsequent 3rd connection is now accepted successfully.

2. **TCP Bidirectional Idle Timeout Teardown ([`pkg/proxy/tcp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp_test.go))**:
   - `TestTCPProxy_IdleTimeout`:
     - Configure [`TCPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp.go#L15) with a short `IdleTimeout` (e.g. `150ms`).
     - Establish a client connection, send a test payload, and verify echo response.
     - Keep the connection open with zero data transmission exceeding `IdleTimeout`.
     - Assert that the connection is terminated by the proxy (subsequent client read or write yields EOF or closed connection error).

3. **UDP Worker Pool Bounding & Saturation Drop ([`pkg/proxy/udp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp_test.go))**:
   - `TestUDPProxy_WorkerPoolSaturation`:
     - Configure [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L14) with a constrained worker count (`MaxWorkers: 2`).
     - Transmit a burst of datagrams faster than worker processing throughput.
     - Verify that the proxy does not spawn unconstrained goroutines and drops saturated packets fail-safe without panicking or leaking resources.

4. **UDP Upstream Socket Reuse Across Datagrams ([`pkg/proxy/udp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp_test.go))**:
   - `TestUDPProxy_SocketReuse`:
     - Set up an echo UDP backend that records the client remote port for each received packet.
     - Client sends multiple datagrams sequentially from the same local address.
     - Verify that the backend records the **exact same upstream source port** across all incoming packets for that client session, confirming that `net.DialUDP` was called only once and reused rather than dialed per packet.

5. **UDP Session Idle Timeout & Eviction ([`pkg/proxy/udp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp_test.go))**:
   - `TestUDPProxy_SessionIdleTimeout`:
     - Configure [`UDPProxy`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp.go#L14) with short `IdleTimeout` (e.g. `200ms`).
     - Send a packet to establish an upstream session.
     - Wait for `IdleTimeout + 100ms`.
     - Confirm that the idle session is evicted and its upstream socket closed.

6. **UDP Buffer Pooling Validation ([`pkg/proxy/udp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp_test.go))**:
   - `TestUDPProxy_BufferPooling`:
     - Process datagrams and assert buffer pool reuse to ensure minimal heap allocation.

8. **Performance Monitoring & Benchmarking ([`pkg/proxy/tcp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/tcp_test.go), [`pkg/proxy/udp_test.go`](file:///Users/sneha/Developer/toron-research/toron/pkg/proxy/udp_test.go))**:
   - `BenchmarkTCPProxy_Forwarding`:
     - Benchmark bidirectional TCP streaming throughput and latency with `b.ReportAllocs()`.
     - Monitor B/op and allocs/op during sustained forwarding.
   - `BenchmarkUDPProxy_Forwarding`:
     - Benchmark datagram packet relay rate and memory allocations with `b.ReportAllocs()`.
     - Confirm that `sync.Pool` buffer recycling achieves near-zero heap allocations per datagram.
   - If performance profiling shows unexpected throughput degradation, explore low-overhead implementation alternatives (e.g. atomic fast-paths, fine-grained sharding).

## Acceptance Criteria

- All unit and integration tests pass via `go test -v -race ./pkg/proxy/...`.
- Performance benchmarks execute cleanly via `go test -bench=. -benchmem ./pkg/proxy/...` verifying low memory allocations and high throughput.
- Test suite verifies TCP connection rejection at capacity, TCP idle timeout teardown, UDP worker pool bounding, UDP upstream socket reuse, UDP session eviction, and buffer pooling.
- Shutdown tests verify deterministic cleanup without hanging goroutines or socket descriptor leaks.
- Zero external/third-party dependencies (strictly standard Go library: `testing`, `net`, `sync`, `sync/atomic`, `time`).

## Blockers & Risks

- In test environments, ensure short idle timeouts (100-300ms) to keep test execution fast while avoiding timing flakes under slower CI test runners.
- Use dynamic loopback port allocation (`:0`) for all mock backend servers and test listeners to prevent port conflict collisions.
