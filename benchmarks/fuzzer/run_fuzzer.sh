#!/usr/bin/env bash
# ==============================================================================
# Toron Differential Security Fuzzer Execution Script
# Retains results in benchmarks/results/history/<timestamp>/ with manifest tracking.
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
mkdir -p "${RESULTS_DIR}"

# Source retention helper
source "${ROOT_DIR}/benchmarks/archive_run.sh"

TARGET="127.0.0.1:8080"
BASELINE=""
TRIALS=1
WARMUP=0
JSON_OUT="${RESULTS_DIR}/differential_fuzz_report.json"
MD_OUT="${RESULTS_DIR}/differential_fuzz_report.md"
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -t <host:port>        Target server under test (default: 127.0.0.1:8080)"
    echo "  -b <host:port>        Optional baseline server for differential comparison (e.g., 127.0.0.1:8081)"
    echo "  -k <trials>           Number of repeated trials per test case (default: 1, e.g. 1000 for empirical stats)"
    echo "  -w <warmup>           Number of preliminary discarded warm-up runs (default: 0, e.g. 50)"
    echo "  -j <file>             Output JSON path (default: benchmarks/results/differential_fuzz_report.json)"
    echo "  -m <file>             Output Markdown path (default: benchmarks/results/differential_fuzz_report.md)"
    echo "  --no-history          Disable historical retention"
    echo "  --session-name <name> Custom suffix for historical run directory"
    echo "  --session-dir <dir>   Explicit destination session directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -t) TARGET="$2"; shift 2 ;;
        -b) BASELINE="$2"; shift 2 ;;
        -k) TRIALS="$2"; shift 2 ;;
        -w) WARMUP="$2"; shift 2 ;;
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
    init_benchmark_session "differential_fuzzer" "${SESSION_NAME}"
fi

echo "[*] Launching Differential Protocol Security Fuzzer against ${TARGET} (trials=${TRIALS}, warmup=${WARMUP})..."
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo "[*] Historical session snapshot: ${TORON_BENCHMARK_SESSION_DIR}"
fi

cmd=("go" "run" "${SCRIPT_DIR}/diff_fuzzer.go" "-target" "${TARGET}" "-trials" "${TRIALS}" "-warmup" "${WARMUP}" "-json" "${JSON_OUT}" "-md" "${MD_OUT}")

if [ -n "${BASELINE}" ]; then
    cmd+=("-baseline" "${BASELINE}")
fi

"${cmd[@]}"

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

PARAMS_JSON=$(printf '{"target": "%s", "baseline": "%s", "trials": %d, "warmup": %d}' "${TARGET}" "${BASELINE}" "${TRIALS}" "${WARMUP}")
archive_benchmark_artifacts "differential_fuzzer" "success" "${ELAPSED}" "${JSON_OUT},${MD_OUT}" "${PARAMS_JSON}" "$0 $*"
