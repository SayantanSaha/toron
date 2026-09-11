#!/usr/bin/env bash
# ==============================================================================
# Toron Differential Security Fuzzer Execution Script
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
mkdir -p "${RESULTS_DIR}"

TARGET="127.0.0.1:8080"
BASELINE=""
TRIALS=1
WARMUP=0
JSON_OUT="${RESULTS_DIR}/differential_fuzz_report.json"
MD_OUT="${RESULTS_DIR}/differential_fuzz_report.md"

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -t <host:port>   Target server under test (default: 127.0.0.1:8080)"
    echo "  -b <host:port>   Optional baseline server for differential comparison (e.g., 127.0.0.1:8081)"
    echo "  -k <trials>      Number of repeated trials per test case (default: 1, e.g. 1000 for empirical stats)"
    echo "  -w <warmup>      Number of preliminary discarded warm-up runs (default: 0, e.g. 50)"
    echo "  -j <file>        Output JSON path (default: benchmarks/results/differential_fuzz_report.json)"
    echo "  -m <file>        Output Markdown path (default: benchmarks/results/differential_fuzz_report.md)"
    echo "  -h, --help       Display this help message"
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
        -h|--help) print_usage; exit 0 ;;
        *) echo "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

echo "[*] Launching Differential Protocol Security Fuzzer against ${TARGET} (trials=${TRIALS}, warmup=${WARMUP})..."
cmd=("go" "run" "${SCRIPT_DIR}/diff_fuzzer.go" "-target" "${TARGET}" "-trials" "${TRIALS}" "-warmup" "${WARMUP}" "-json" "${JSON_OUT}" "-md" "${MD_OUT}")

if [ -n "${BASELINE}" ]; then
    cmd+=("-baseline" "${BASELINE}")
fi

"${cmd[@]}"
