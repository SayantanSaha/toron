# 📊 Multi-Proxy Differential Docker Benchmark Report

**Date**: `2026-09-14T05:33:47Z` | **OS**: `darwin/arm64` | **Host CPUs**: `8`

**Workload Profile**: Concurrency: `50` | Duration per Target: `3.0s` | Target Rate: `0 RPS`

## 1. Comparative Performance Matrix (Head-to-Head)

| Proxy Engine | Upstream Backend | Throughput (RPS) | P50 (ms) | P90 (ms) | P99 (ms) | P99.9 (ms) | Max (ms) | Memory RSS | CPU % | Errors |
|:---|:---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| **Toron (v1.0.0)** | `fast` | **24204.5** | 1.16 | 2.43 | 6.73 | 202.03 | 602.32 | 30.1 MB | 0.0% | 0 |
| **Toron (v1.0.0)** | `go` | **24504.1** | 1.05 | 2.12 | 6.27 | 201.93 | 208.46 | 32.0 MB | 0.0% | 0 |
| **Toron (v1.0.0)** | `node` | **20424.4** | 1.55 | 3.15 | 6.54 | 202.62 | 601.81 | 46.2 MB | 0.0% | 0 |
| **Toron (v1.0.0)** | `python` | **1102.6** | 43.44 | 46.77 | 74.15 | 282.24 | 285.92 | 56.8 MB | 0.1% | 0 |
| **NGINX (Alpine)** | `fast` | **34153.6** | 0.85 | 1.72 | 4.76 | 201.20 | 213.44 | 17.8 MB | 0.1% | 0 |
| **NGINX (Alpine)** | `go` | **29416.4** | 0.88 | 1.92 | 5.96 | 202.55 | 217.00 | 17.7 MB | 0.2% | 0 |
| **NGINX (Alpine)** | `node` | **25106.1** | 1.70 | 3.00 | 5.08 | 11.11 | 220.61 | 17.4 MB | 0.0% | 0 |
| **NGINX (Alpine)** | `python` | **904.4** | 43.94 | 48.88 | 232.34 | 1470.77 | 1472.70 | 18.1 MB | 0.0% | 0 |
| **Traefik (v3.1)** | `fast` | **23958.7** | 0.97 | 1.80 | 5.51 | 203.00 | 600.63 | 121.9 MB | 0.1% | 0 |
| **Traefik (v3.1)** | `go` | **24365.5** | 1.04 | 1.93 | 5.47 | 202.14 | 602.79 | 93.4 MB | 0.2% | 0 |
| **Traefik (v3.1)** | `node` | **16327.7** | 1.18 | 2.42 | 7.09 | 207.69 | 222.21 | 115.1 MB | 0.1% | 0 |
| **Traefik (v3.1)** | `python` | **1117.6** | 43.72 | 46.76 | 69.94 | 256.65 | 257.05 | 115.4 MB | 0.0% | 0 |
| **Caddy (Alpine)** | `fast` | **18270.2** | 1.17 | 3.58 | 9.71 | 208.06 | 214.20 | 60.1 MB | 0.4% | 0 |
| **Caddy (Alpine)** | `go` | **19715.2** | 1.20 | 3.66 | 8.70 | 203.66 | 601.76 | 59.6 MB | 0.4% | 0 |
| **Caddy (Alpine)** | `node` | **14752.1** | 1.85 | 4.62 | 10.05 | 203.64 | 216.19 | 63.2 MB | 0.0% | 0 |
| **Caddy (Alpine)** | `python` | **1113.6** | 43.61 | 46.99 | 96.63 | 270.17 | 270.95 | 66.3 MB | 0.0% | 0 |
| **HAProxy (Alpine)** | `fast` | **32349.5** | 0.81 | 1.77 | 4.61 | 201.13 | 204.28 | 23.6 MB | 0.0% | 0 |
| **HAProxy (Alpine)** | `go` | **29337.2** | 0.92 | 2.05 | 6.02 | 201.41 | 222.74 | 23.4 MB | 0.0% | 0 |
| **HAProxy (Alpine)** | `node` | **23888.9** | 1.71 | 3.35 | 5.87 | 11.53 | 205.76 | 23.6 MB | 0.0% | 0 |
| **HAProxy (Alpine)** | `python` | **681.9** | 43.71 | 48.15 | 1229.59 | 2460.89 | 2463.03 | 23.8 MB | 0.0% | 0 |

## 2. Upstream Backend Runtime Breakdown

### Backend: `fast` (Go Fast Echo)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **NGINX (Alpine)** | **34153.6** | 0.85 | 4.76 | 17.8 MB | 0.1% |
| 🥈 #2 | **HAProxy (Alpine)** | **32349.5** | 0.81 | 4.61 | 23.6 MB | 0.0% |
| 🥉 #3 | **Toron (v1.0.0)** | **24204.5** | 1.16 | 6.73 | 30.1 MB | 0.0% |
| #4 | **Traefik (v3.1)** | **23958.7** | 0.97 | 5.51 | 121.9 MB | 0.1% |
| #5 | **Caddy (Alpine)** | **18270.2** | 1.17 | 9.71 | 60.1 MB | 0.4% |

### Backend: `go` (Go Standard net/http)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **NGINX (Alpine)** | **29416.4** | 0.88 | 5.96 | 17.7 MB | 0.2% |
| 🥈 #2 | **HAProxy (Alpine)** | **29337.2** | 0.92 | 6.02 | 23.4 MB | 0.0% |
| 🥉 #3 | **Toron (v1.0.0)** | **24504.1** | 1.05 | 6.27 | 32.0 MB | 0.0% |
| #4 | **Traefik (v3.1)** | **24365.5** | 1.04 | 5.47 | 93.4 MB | 0.2% |
| #5 | **Caddy (Alpine)** | **19715.2** | 1.20 | 8.70 | 59.6 MB | 0.4% |

### Backend: `node` (Node.js 20 llhttp)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **NGINX (Alpine)** | **25106.1** | 1.70 | 5.08 | 17.4 MB | 0.0% |
| 🥈 #2 | **HAProxy (Alpine)** | **23888.9** | 1.71 | 5.87 | 23.6 MB | 0.0% |
| 🥉 #3 | **Toron (v1.0.0)** | **20424.4** | 1.55 | 6.54 | 46.2 MB | 0.0% |
| #4 | **Traefik (v3.1)** | **16327.7** | 1.18 | 7.09 | 115.1 MB | 0.1% |
| #5 | **Caddy (Alpine)** | **14752.1** | 1.85 | 10.05 | 63.2 MB | 0.0% |

### Backend: `python` (Python 3.11 uvicorn/h11)

| Rank | Proxy | RPS | P50 (ms) | P99 (ms) | Mem RSS | CPU % |
|:---|:---|---:|---:|---:|---:|---:|
| 🥇 #1 | **Traefik (v3.1)** | **1117.6** | 43.72 | 69.94 | 115.4 MB | 0.0% |
| 🥈 #2 | **Caddy (Alpine)** | **1113.6** | 43.61 | 96.63 | 66.3 MB | 0.0% |
| 🥉 #3 | **Toron (v1.0.0)** | **1102.6** | 43.44 | 74.15 | 56.8 MB | 0.1% |
| #4 | **NGINX (Alpine)** | **904.4** | 43.94 | 232.34 | 18.1 MB | 0.0% |
| #5 | **HAProxy (Alpine)** | **681.9** | 43.71 | 1229.59 | 23.8 MB | 0.0% |

## 3. Pre-Flight Functional Parity Verification

| Proxy | Upstream Origin | Health URL | Status Code | Latency | Result |
|:---|:---|:---|---:|---:|:---|
| Toron (v1.0.0) | `fast` | `http://127.0.0.1:8881/fast/health` | 200 | 2.74ms | ✅ PASS |
| Toron (v1.0.0) | `go` | `http://127.0.0.1:8881/go/health` | 200 | 0.55ms | ✅ PASS |
| Toron (v1.0.0) | `node` | `http://127.0.0.1:8881/node/health` | 200 | 1.02ms | ✅ PASS |
| Toron (v1.0.0) | `python` | `http://127.0.0.1:8881/python/health` | 200 | 1.26ms | ✅ PASS |
| NGINX (Alpine) | `fast` | `http://127.0.0.1:8882/fast/health` | 200 | 2.02ms | ✅ PASS |
| NGINX (Alpine) | `go` | `http://127.0.0.1:8882/go/health` | 200 | 0.57ms | ✅ PASS |
| NGINX (Alpine) | `node` | `http://127.0.0.1:8882/node/health` | 200 | 0.79ms | ✅ PASS |
| NGINX (Alpine) | `python` | `http://127.0.0.1:8882/python/health` | 200 | 0.62ms | ✅ PASS |
| Traefik (v3.1) | `fast` | `http://127.0.0.1:8883/fast/health` | 200 | 2.36ms | ✅ PASS |
| Traefik (v3.1) | `go` | `http://127.0.0.1:8883/go/health` | 200 | 1.75ms | ✅ PASS |
| Traefik (v3.1) | `node` | `http://127.0.0.1:8883/node/health` | 200 | 0.88ms | ✅ PASS |
| Traefik (v3.1) | `python` | `http://127.0.0.1:8883/python/health` | 200 | 0.57ms | ✅ PASS |
| Caddy (Alpine) | `fast` | `http://127.0.0.1:8884/fast/health` | 200 | 3.48ms | ✅ PASS |
| Caddy (Alpine) | `go` | `http://127.0.0.1:8884/go/health` | 200 | 0.44ms | ✅ PASS |
| Caddy (Alpine) | `node` | `http://127.0.0.1:8884/node/health` | 200 | 0.77ms | ✅ PASS |
| Caddy (Alpine) | `python` | `http://127.0.0.1:8884/python/health` | 200 | 0.48ms | ✅ PASS |
| HAProxy (Alpine) | `fast` | `http://127.0.0.1:8885/fast/health` | 200 | 1.98ms | ✅ PASS |
| HAProxy (Alpine) | `go` | `http://127.0.0.1:8885/go/health` | 200 | 0.31ms | ✅ PASS |
| HAProxy (Alpine) | `node` | `http://127.0.0.1:8885/node/health` | 200 | 3.42ms | ✅ PASS |
| HAProxy (Alpine) | `python` | `http://127.0.0.1:8885/python/health` | 200 | 0.48ms | ✅ PASS |

---
*Report automatically generated by Toron Differential Docker Benchmark Suite*
