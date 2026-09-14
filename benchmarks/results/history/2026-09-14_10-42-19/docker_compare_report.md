# 📊 Multi-Proxy Differential Docker Benchmark Report

**Date**: `2026-09-14T05:12:19Z` | **OS**: `darwin/arm64` | **Host CPUs**: `8`

**Workload Profile**: Concurrency: `50` | Duration per Target: `3.0s` | Target Rate: `0 RPS`

## 1. Comparative Performance Matrix (Head-to-Head)

| Proxy Engine | Upstream Backend | Throughput (RPS) | P50 (ms) | P90 (ms) | P99 (ms) | P99.9 (ms) | Max (ms) | Memory RSS | CPU % | Errors |
|:---|:---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| **Toron (v1.0.0)** | `fast` | **8295.8** | 3.33 | 9.80 | 45.09 | 211.39 | 243.87 | 24.6 MB | 3.0% | 50 |
| **Toron (v1.0.0)** | `go` | **8125.0** | 2.84 | 8.54 | 74.41 | 213.15 | 267.67 | 27.5 MB | 0.0% | 50 |
| **Toron (v1.0.0)** | `node` | **11434.6** | 2.87 | 7.92 | 13.83 | 208.98 | 229.88 | 26.1 MB | 0.0% | 50 |
| **Toron (v1.0.0)** | `python` | **1568.0** | 41.62 | 44.68 | 63.37 | 265.43 | 419.03 | 29.7 MB | 7.3% | 48 |
| **NGINX (Alpine)** | `fast` | **32505.3** | 0.81 | 1.62 | 4.81 | 201.49 | 604.11 | 18.1 MB | 0.1% | 50 |
| **NGINX (Alpine)** | `go` | **33033.2** | 0.83 | 1.69 | 5.02 | 201.30 | 212.32 | 19.1 MB | 0.1% | 48 |
| **NGINX (Alpine)** | `node` | **25599.9** | 1.64 | 2.92 | 4.67 | 200.84 | 207.26 | 18.2 MB | 0.0% | 49 |
| **NGINX (Alpine)** | `python` | **828.8** | 43.99 | 49.14 | 260.26 | 2295.01 | 2302.36 | 17.9 MB | 0.0% | 48 |
| **Traefik (v3.1)** | `fast` | **26216.0** | 0.97 | 1.76 | 4.93 | 202.06 | 600.83 | 110.8 MB | 0.6% | 47 |
| **Traefik (v3.1)** | `go` | **24313.0** | 1.00 | 1.80 | 4.91 | 202.23 | 211.03 | 120.3 MB | 0.3% | 50 |
| **Traefik (v3.1)** | `node` | **15156.3** | 1.08 | 2.47 | 8.32 | 206.68 | 209.76 | 133.2 MB | 0.3% | 50 |
| **Traefik (v3.1)** | `python` | **1091.8** | 43.67 | 46.91 | 83.96 | 270.88 | 277.41 | 132.9 MB | 0.0% | 49 |
| **Caddy (Alpine)** | `fast` | **19688.0** | 1.13 | 3.24 | 8.54 | 203.60 | 603.82 | 33.8 MB | 0.5% | 50 |
| **Caddy (Alpine)** | `go` | **19868.4** | 1.15 | 3.37 | 8.51 | 202.90 | 601.95 | 25.7 MB | 0.2% | 50 |
| **Caddy (Alpine)** | `node` | **13660.8** | 1.50 | 4.00 | 10.76 | 205.66 | 215.87 | 48.4 MB | 0.3% | 48 |
| **Caddy (Alpine)** | `python` | **1129.7** | 43.96 | 47.32 | 64.17 | 93.61 | 246.94 | 54.3 MB | 0.1% | 48 |
| **HAProxy (Alpine)** | `fast` | **30181.7** | 0.80 | 1.61 | 5.21 | 201.51 | 213.23 | 24.6 MB | 0.2% | 50 |
| **HAProxy (Alpine)** | `go` | **33704.5** | 0.80 | 1.56 | 4.41 | 201.04 | 204.62 | 25.4 MB | 0.2% | 45 |
| **HAProxy (Alpine)** | `node` | **26527.2** | 1.36 | 2.92 | 5.10 | 201.00 | 205.06 | 25.2 MB | 0.0% | 47 |
| **HAProxy (Alpine)** | `python` | **693.8** | 44.17 | 50.04 | 1269.79 | 2283.32 | 2285.24 | 25.6 MB | 0.0% | 50 |

## 2. Upstream Backend Runtime Breakdown

### Backend: `fast` (Go Fast Echo)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **NGINX (Alpine)** | **32505.3** | 0.81 | 4.81 | 18.1 MB | 0.1% |
| 🥈 #2 | **HAProxy (Alpine)** | **30181.7** | 0.80 | 5.21 | 24.6 MB | 0.2% |
| 🥉 #3 | **Traefik (v3.1)** | **26216.0** | 0.97 | 4.93 | 110.8 MB | 0.6% |
| #4 | **Caddy (Alpine)** | **19688.0** | 1.13 | 8.54 | 33.8 MB | 0.5% |
| #5 | **Toron (v1.0.0)** | **8295.8** | 3.33 | 45.09 | 24.6 MB | 3.0% |

### Backend: `go` (Go Standard net/http)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **HAProxy (Alpine)** | **33704.5** | 0.80 | 4.41 | 25.4 MB | 0.2% |
| 🥈 #2 | **NGINX (Alpine)** | **33033.2** | 0.83 | 5.02 | 19.1 MB | 0.1% |
| 🥉 #3 | **Traefik (v3.1)** | **24313.0** | 1.00 | 4.91 | 120.3 MB | 0.3% |
| #4 | **Caddy (Alpine)** | **19868.4** | 1.15 | 8.51 | 25.7 MB | 0.2% |
| #5 | **Toron (v1.0.0)** | **8125.0** | 2.84 | 74.41 | 27.5 MB | 0.0% |

### Backend: `node` (Node.js 20 llhttp)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **HAProxy (Alpine)** | **26527.2** | 1.36 | 5.10 | 25.2 MB | 0.0% |
| 🥈 #2 | **NGINX (Alpine)** | **25599.9** | 1.64 | 4.67 | 18.2 MB | 0.0% |
| 🥉 #3 | **Traefik (v3.1)** | **15156.3** | 1.08 | 8.32 | 133.2 MB | 0.3% |
| #4 | **Caddy (Alpine)** | **13660.8** | 1.50 | 10.76 | 48.4 MB | 0.3% |
| #5 | **Toron (v1.0.0)** | **11434.6** | 2.87 | 13.83 | 26.1 MB | 0.0% |

### Backend: `python` (Python 3.11 uvicorn/h11)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **Toron (v1.0.0)** | **1568.0** | 41.62 | 63.37 | 29.7 MB | 7.3% |
| 🥈 #2 | **Caddy (Alpine)** | **1129.7** | 43.96 | 64.17 | 54.3 MB | 0.1% |
| 🥉 #3 | **Traefik (v3.1)** | **1091.8** | 43.67 | 83.96 | 132.9 MB | 0.0% |
| #4 | **NGINX (Alpine)** | **828.8** | 43.99 | 260.26 | 17.9 MB | 0.0% |
| #5 | **HAProxy (Alpine)** | **693.8** | 44.17 | 1269.79 | 25.6 MB | 0.0% |

## 3. Pre-Flight Functional Parity Verification

| Proxy | Upstream Origin | Health URL | Status Code | Latency | Result |
|:---|:---|:---|---:|---:|:---|
| Toron (v1.0.0) | `fast` | `http://127.0.0.1:8881/fast/health` | 200 | 2.89ms | ✅ PASS |
| Toron (v1.0.0) | `go` | `http://127.0.0.1:8881/go/health` | 200 | 0.40ms | ✅ PASS |
| Toron (v1.0.0) | `node` | `http://127.0.0.1:8881/node/health` | 200 | 0.83ms | ✅ PASS |
| Toron (v1.0.0) | `python` | `http://127.0.0.1:8881/python/health` | 200 | 3.14ms | ✅ PASS |
| NGINX (Alpine) | `fast` | `http://127.0.0.1:8882/fast/health` | 200 | 5.20ms | ✅ PASS |
| NGINX (Alpine) | `go` | `http://127.0.0.1:8882/go/health` | 200 | 0.59ms | ✅ PASS |
| NGINX (Alpine) | `node` | `http://127.0.0.1:8882/node/health` | 200 | 0.72ms | ✅ PASS |
| NGINX (Alpine) | `python` | `http://127.0.0.1:8882/python/health` | 200 | 0.49ms | ✅ PASS |
| Traefik (v3.1) | `fast` | `http://127.0.0.1:8883/fast/health` | 200 | 2.92ms | ✅ PASS |
| Traefik (v3.1) | `go` | `http://127.0.0.1:8883/go/health` | 200 | 0.55ms | ✅ PASS |
| Traefik (v3.1) | `node` | `http://127.0.0.1:8883/node/health` | 200 | 0.77ms | ✅ PASS |
| Traefik (v3.1) | `python` | `http://127.0.0.1:8883/python/health` | 200 | 0.49ms | ✅ PASS |
| Caddy (Alpine) | `fast` | `http://127.0.0.1:8884/fast/health` | 200 | 1.64ms | ✅ PASS |
| Caddy (Alpine) | `go` | `http://127.0.0.1:8884/go/health` | 200 | 0.38ms | ✅ PASS |
| Caddy (Alpine) | `node` | `http://127.0.0.1:8884/node/health` | 200 | 0.67ms | ✅ PASS |
| Caddy (Alpine) | `python` | `http://127.0.0.1:8884/python/health` | 200 | 0.45ms | ✅ PASS |
| HAProxy (Alpine) | `fast` | `http://127.0.0.1:8885/fast/health` | 200 | 1.60ms | ✅ PASS |
| HAProxy (Alpine) | `go` | `http://127.0.0.1:8885/go/health` | 200 | 0.40ms | ✅ PASS |
| HAProxy (Alpine) | `node` | `http://127.0.0.1:8885/node/health` | 200 | 2.70ms | ✅ PASS |
| HAProxy (Alpine) | `python` | `http://127.0.0.1:8885/python/health` | 200 | 1.31ms | ✅ PASS |

---
*Report automatically generated by Toron Differential Docker Benchmark Suite*
