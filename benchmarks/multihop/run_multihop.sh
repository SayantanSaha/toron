#!/usr/bin/env bash
# ==============================================================================
# 🛡️ Toron Heterogeneous Multi-Hop Testbed Execution Script (BMK-03)
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
mkdir -p "${RESULTS_DIR}"

MODE="standalone"
EDGE_ADDR="127.0.0.1:8080"
JSON_OUT="${RESULTS_DIR}/multihop_report.json"
MD_OUT="${RESULTS_DIR}/multihop_report.md"

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --standalone     Run in-process zero-dependency standalone testbed (default)"
    echo "  --docker         Launch multi-container Docker Compose cluster and evaluate"
    echo "  -e <host:port>   Target live Edge Gateway address (default: 127.0.0.1:8080)"
    echo "  -j <file>        Output JSON path (default: benchmarks/results/multihop_report.json)"
    echo "  -m <file>        Output Markdown path (default: benchmarks/results/multihop_report.md)"
    echo "  -h, --help       Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --standalone) MODE="standalone"; shift ;;
        --docker) MODE="docker"; shift ;;
        -e) EDGE_ADDR="$2"; MODE="live"; shift 2 ;;
        -j) JSON_OUT="$2"; shift 2 ;;
        -m) MD_OUT="$2"; shift 2 ;;
        -h|--help) print_usage; exit 0 ;;
        *) echo "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

cleanup_docker() {
    if [ "${MODE}" = "docker" ]; then
        echo "[*] Tearing down multi-hop Docker Compose cluster..."
        docker compose -f "${SCRIPT_DIR}/docker-compose.multihop.yml" down --volumes --remove-orphans || true
    fi
}
trap cleanup_docker EXIT

if [ "${MODE}" = "docker" ]; then
    echo "[*] Building and launching multi-hop Docker Compose cluster..."
    docker compose -f "${SCRIPT_DIR}/docker-compose.multihop.yml" up -d --build
    echo "[*] Waiting for backend origin services to initialize..."
    sleep 5
    EDGE_ADDR="127.0.0.1:8080"
fi

if [ "${MODE}" = "standalone" ]; then
    echo "[*] Executing standalone in-process multi-hop evaluation..."
    go run "${SCRIPT_DIR}/runner.go" -standalone -json "${JSON_OUT}" -md "${MD_OUT}"
else
    echo "[*] Executing multi-hop evaluation against Edge Gateway at ${EDGE_ADDR}..."
    go run "${SCRIPT_DIR}/runner.go" -standalone=false -edge-addr "${EDGE_ADDR}" -json "${JSON_OUT}" -md "${MD_OUT}"
fi

echo "[*] Multi-hop evaluation complete. Telemetry saved to ${RESULTS_DIR}"
