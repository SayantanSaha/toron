#!/usr/bin/env bash
# ==============================================================================
# 🛡️ Toron Heterogeneous Multi-Hop Testbed Execution Script (BMK-03)
# Retains results in benchmarks/results/history/<timestamp>/ with manifest tracking.
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
mkdir -p "${RESULTS_DIR}"

# Source retention helper
source "${ROOT_DIR}/benchmarks/archive_run.sh"

MODE="standalone"
EDGE_ADDR="127.0.0.1:8080"
JSON_OUT="${RESULTS_DIR}/multihop_report.json"
MD_OUT="${RESULTS_DIR}/multihop_report.md"
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --standalone          Run in-process zero-dependency standalone testbed (default)"
    echo "  --docker              Launch multi-container Docker Compose cluster and evaluate"
    echo "  -e <host:port>        Target live Edge Gateway address (default: 127.0.0.1:8080)"
    echo "  -j <file>             Output JSON path (default: benchmarks/results/multihop_report.json)"
    echo "  -m <file>             Output Markdown path (default: benchmarks/results/multihop_report.md)"
    echo "  --no-history          Disable historical retention"
    echo "  --session-name <name> Custom suffix for historical run directory"
    echo "  --session-dir <dir>   Explicit destination session directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --standalone) MODE="standalone"; shift ;;
        --docker) MODE="docker"; shift ;;
        -e) EDGE_ADDR="$2"; MODE="live"; shift 2 ;;
        -j) JSON_OUT="$2"; shift 2 ;;
        -m) MD_OUT="$2"; shift 2 ;;
        --no-history) NO_HISTORY=true; export TORON_NO_HISTORY=true; shift ;;
        --session-name) SESSION_NAME="$2"; shift 2 ;;
        --session-dir) CUSTOM_SESSION_DIR="$2"; shift 2 ;;
        -h|--help) print_usage; exit 0 ;;
        *) echo "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

START_TIME=$(date +%s)

if [ "$NO_HISTORY" = "true" ]; then
    export TORON_NO_HISTORY=true
elif [ -n "${CUSTOM_SESSION_DIR}" ]; then
    export TORON_BENCHMARK_SESSION_DIR="${CUSTOM_SESSION_DIR}"
    mkdir -p "${TORON_BENCHMARK_SESSION_DIR}"
    export IS_STANDALONE_SESSION=false
else
    init_benchmark_session "multihop" "${SESSION_NAME}"
fi

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

if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo "[*] Historical session snapshot: ${TORON_BENCHMARK_SESSION_DIR}"
fi

if [ "${MODE}" = "standalone" ]; then
    echo "[*] Executing standalone in-process multi-hop evaluation..."
    go run "${SCRIPT_DIR}/runner.go" -standalone -json "${JSON_OUT}" -md "${MD_OUT}"
else
    echo "[*] Executing multi-hop evaluation against Edge Gateway at ${EDGE_ADDR}..."
    go run "${SCRIPT_DIR}/runner.go" -standalone=false -edge-addr "${EDGE_ADDR}" -json "${JSON_OUT}" -md "${MD_OUT}"
fi

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

PARAMS_JSON=$(printf '{"mode": "%s", "edge_addr": "%s"}' "${MODE}" "${EDGE_ADDR}")
archive_benchmark_artifacts "multihop" "success" "${ELAPSED}" "${JSON_OUT},${MD_OUT}" "${PARAMS_JSON}" "$0 $*"

echo "[*] Multi-hop evaluation complete. Telemetry saved to ${RESULTS_DIR}"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo "[*] Historical session snapshot preserved in ${TORON_BENCHMARK_SESSION_DIR}"
fi
