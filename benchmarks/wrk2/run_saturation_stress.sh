#!/usr/bin/env bash
# ==============================================================================
# 🛡️ Toron High-Concurrency Saturation & Adversarial Stress Harness (BMK-04)
# Retains results in benchmarks/results/history/<timestamp>/ with manifest tracking.
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
mkdir -p "${RESULTS_DIR}"

# Source retention helper
source "${ROOT_DIR}/benchmarks/archive_run.sh"

TARGET_URL="http://127.0.0.1:8080/health"
DURATION="10s"
CONCURRENCY=50
TARGET_RATE=5000
ATTACK_RATIO=0.10
AUTO_START=false
JSON_OUT="${RESULTS_DIR}/saturation_stress_report.json"
MD_OUT="${RESULTS_DIR}/saturation_stress_report.md"
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -u <url>              Target URL (default: http://127.0.0.1:8080/health)"
    echo "  -c <conns>            Concurrent connections (default: 50)"
    echo "  -d <duration>         Duration (default: 10s)"
    echo "  -r <rate>             Target requests/second rate (default: 5000)"
    echo "  -a <ratio>            Adversarial attack injection ratio (default: 0.10 = 10%)"
    echo "  -j <file>             JSON report destination"
    echo "  -m <file>             Markdown report destination"
    echo "  --auto-start          Automatically build and start background Toron server"
    echo "  --no-history          Disable historical retention"
    echo "  --session-name <name> Custom suffix for historical run directory"
    echo "  --session-dir <dir>   Explicit destination session directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -u) TARGET_URL="$2"; shift 2 ;;
        -c) CONCURRENCY="$2"; shift 2 ;;
        -d) DURATION="$2"; shift 2 ;;
        -r) TARGET_RATE="$2"; shift 2 ;;
        -a) ATTACK_RATIO="$2"; shift 2 ;;
        -j) JSON_OUT="$2"; shift 2 ;;
        -m) MD_OUT="$2"; shift 2 ;;
        --auto-start) AUTO_START=true; shift ;;
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
    init_benchmark_session "saturation_stress" "${SESSION_NAME}"
fi

TORON_PID=""
cleanup() {
    if [ -n "${TORON_PID}" ]; then
        echo "[*] Stopping background Toron instance (PID: ${TORON_PID})..."
        kill -15 "${TORON_PID}" 2>/dev/null || true
        wait "${TORON_PID}" 2>/dev/null || true
    fi
}
trap cleanup EXIT

if [ "${AUTO_START}" = "true" ]; then
    echo "[*] Compiling Toron binary for evaluation..."
    go build -o "${RESULTS_DIR}/toron_stress" "${ROOT_DIR}/cmd/toron"
    echo "[*] Starting Toron background gateway on :8080..."
    "${RESULTS_DIR}/toron_stress" -config "${ROOT_DIR}/config.yaml" -routes "${ROOT_DIR}/routes.yaml" > "${RESULTS_DIR}/server_stress.log" 2>&1 &
    TORON_PID=$!
    echo "[*] Waiting for Toron to listen..."
    for i in {1..30}; do
        if curl -s -f "http://127.0.0.1:8080/health" >/dev/null 2>&1; then
            echo "[*] Toron is healthy and ready on :8080."
            break
        fi
        sleep 0.2
    done
fi

echo "================================================================================"
echo "  LAUNCHING SATURATION STRESS BENCHMARK (5,000+ RPS, 10% ATTACK INJECTION)      "
echo "================================================================================"
echo " Target URL:       ${TARGET_URL}"
echo " Concurrency:      ${CONCURRENCY}"
echo " Duration:         ${DURATION}"
echo " Target Rate:      ${TARGET_RATE} RPS"
echo " Attack Ratio:     ${ATTACK_RATIO} (10% adversarial vectors)"
echo " Output Artifacts: ${JSON_OUT}"
echo "                   ${MD_OUT}"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo " History Session:  ${TORON_BENCHMARK_SESSION_DIR}"
fi
echo "================================================================================"

go run "${SCRIPT_DIR}/loadgen.go" \
    -url "${TARGET_URL}" \
    -c "${CONCURRENCY}" \
    -d "${DURATION}" \
    -rate "${TARGET_RATE}" \
    -attack-ratio "${ATTACK_RATIO}" \
    -json "${JSON_OUT}" \
    -md "${MD_OUT}"

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

PARAMS_JSON=$(printf '{"concurrency": %d, "rate": %d, "duration": "%s", "attack_ratio": %f}' "${CONCURRENCY}" "${TARGET_RATE}" "${DURATION}" "${ATTACK_RATIO}")
archive_benchmark_artifacts "saturation_stress" "success" "${ELAPSED}" "${JSON_OUT},${MD_OUT}" "${PARAMS_JSON}" "$0 $*"

echo "[*] Saturation evaluation complete. Reports saved to ${RESULTS_DIR}."
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo "[*] Historical session snapshot preserved in ${TORON_BENCHMARK_SESSION_DIR}."
fi
