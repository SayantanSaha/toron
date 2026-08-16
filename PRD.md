# Toron - An Event-Driven Web Server & Edge Gateway in Go (v1.0.0 Official Release)

## Project Brief

Toron is an event-driven, zero-dependency, ultra-lightweight Web Server, Reverse Proxy Gateway, and Edge Security Engine written in Go. Performance, security, and developer ergonomics are primary focus areas. The architecture is strictly modular for high extensibility.

## Development Method & Release Milestone

Development follows an iterative prototype model (`PROTOTYPE-01` through `PROTOTYPE-35`). As of **v1.0.0**, all core capabilities—including non-blocking TCP reactor, HTTP/1.1, HTTP/2 multiplexing (`h2c` / TLS), HTTP/3 QUIC, gRPC gateway & trailer preservation, ACME zero-touch SSL (HTTP-01 & ALPN-01), per-host SNI & mTLS, Brotli/Zstd streaming compression, RFC 7234 response caching, multi-scheme authentication (JWT, APIKey, Basic), Web Application Firewall (WAF) with OWASP & custom regex rules and CIDR IP ACLs, zero-downtime hot reloading, and an intentionally read-only Security Audit Control Center—are **feature-frozen for Version 1.0.0**.
