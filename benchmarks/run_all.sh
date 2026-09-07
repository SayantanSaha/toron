#!/usr/bin/env bash
# ==============================================================================
# Toron Master Benchmark & Differential Security Evaluation Orchestrator
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
mkdir -p "${RESULTS_DIR}"

AUTO_START=false
TARGET_HOST="127.0.0.1:8080"
SERVER_PID=""

cleanup() {
    if [ -n "${SERVER_PID}" ]; then
        echo ""
        echo "[*] Gracefully stopping background Toron server (PID: ${SERVER_PID})..."
        kill -TERM "${SERVER_PID}" 2>/dev/null || true
        wait "${SERVER_PID}" 2>/dev/null || true
        echo "[✓] Toron server stopped."
    fi
}
trap cleanup EXIT INT TERM

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --auto-start    Automatically compile and launch local Toron server in background"
    echo "  -t <host:port>  Target host:port (default: 127.0.0.1:8080)"
    echo "  -h, --help      Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --auto-start) AUTO_START=true; shift ;;
        -t) TARGET_HOST="$2"; shift 2 ;;
        -h|--help) print_usage; exit 0 ;;
        *) echo "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

echo "================================================================================"
echo "          TORON AUTOMATED RESEARCH EVALUATION & BENCHMARK SUITE                 "
echo "================================================================================"
echo " Target Endpoint:    http://${TARGET_HOST}"
echo " Auto-Start Mode:    ${AUTO_START}"
echo " Artifact Directory: ${RESULTS_DIR}"
echo "================================================================================"

if [ "$AUTO_START" = "true" ]; then
    echo ""
    echo "[1/4] Building Toron executable..."
    (cd "${ROOT_DIR}" && go build -o "${RESULTS_DIR}/toron_eval" ./cmd/toron)

    echo "[2/4] Launching Toron server in background..."
    "${RESULTS_DIR}/toron_eval" -config "${ROOT_DIR}/config.yaml" -routes "${ROOT_DIR}/routes.yaml" > "${RESULTS_DIR}/server.log" 2>&1 &
    SERVER_PID=$!
    echo "      Server process launched (PID: ${SERVER_PID}). Waiting for socket readiness..."

    MAX_RETRIES=20
    RETRY_COUNT=0
    READY=false
    while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
        if curl -s "http://${TARGET_HOST}/health" > /dev/null 2>&1; then
            READY=true
            break
        fi
        sleep 0.2
        RETRY_COUNT=$((RETRY_COUNT + 1))
    done

    if [ "$READY" = "false" ]; then
        echo "[-] Error: Toron server failed to respond on http://${TARGET_HOST}/health within timeout."
        cat "${RESULTS_DIR}/server.log"
        exit 1
    fi
    echo "[✓] Toron server is online and responding."
fi

echo ""
echo "[3/4] Executing High-Throughput & Tail Latency Benchmark (wrk2 harness)..."
bash "${SCRIPT_DIR}/wrk2/run_wrk2.sh" -u "http://${TARGET_HOST}/health" -c 100 -d 5s -r 5000

echo ""
echo "[4/4] Executing Differential Protocol Security Fuzzer..."
bash "${SCRIPT_DIR}/fuzzer/run_fuzzer.sh" -t "${TARGET_HOST}"

echo ""
echo "================================================================================"
echo " [✓] ALL EVALUATION ARTIFACTS SUCCESSFULLY GENERATED                            "
echo "================================================================================"
echo " Generated Artifacts:"
ls -lh "${RESULTS_DIR}"
echo "================================================================================"
