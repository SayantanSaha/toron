#!/usr/bin/env bash
# ==============================================================================
# Toron Automated Performance & Latency Benchmark Harness (wrk2 / Go fallback)
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
CONCURRENCY=100
THREADS=4
TARGET_RATE=10000
LUA_SCRIPT=""
SWEEP_MODE="false"
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -u <url>              Target URL (default: http://127.0.0.1:8080/health)"
    echo "  -d <duration>         Test duration (default: 10s)"
    echo "  -c <conns>            Concurrent connections (default: 100)"
    echo "  -t <threads>          Worker threads (for wrk/wrk2, default: 4)"
    echo "  -r <rate>             Target RPS rate limit (for wrk2, default: 10000)"
    echo "  -s <lua_script>       Optional wrk Lua script (e.g. scripts/pipeline.lua)"
    echo "  --sweep               Run automated multi-concurrency sweep (50, 100, 250, 500, 1000)"
    echo "  --no-history          Disable historical retention"
    echo "  --session-name <name> Custom suffix for historical run directory"
    echo "  --session-dir <dir>   Explicit destination session directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -u) TARGET_URL="$2"; shift 2 ;;
        -d) DURATION="$2"; shift 2 ;;
        -c) CONCURRENCY="$2"; shift 2 ;;
        -t) THREADS="$2"; shift 2 ;;
        -r) TARGET_RATE="$2"; shift 2 ;;
        -s) LUA_SCRIPT="$2"; shift 2 ;;
        --sweep) SWEEP_MODE="true"; shift ;;
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
    init_benchmark_session "wrk2" "${SESSION_NAME}"
fi

echo "================================================================================"
echo "          TORON AUTOMATED WRK2 / REPEATABLE BENCHMARK HARNESS                   "
echo "================================================================================"
echo " Target URL:       ${TARGET_URL}"
echo " Default Duration: ${DURATION}"
echo " Default Conns:    ${CONCURRENCY}"
echo " Output Directory: ${RESULTS_DIR}"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo " History Session:  ${TORON_BENCHMARK_SESSION_DIR}"
fi
echo "================================================================================"

HAS_WRK2=false
HAS_WRK=false
if command -v wrk2 &>/dev/null; then
    HAS_WRK2=true
    BENCHMARK_BIN="wrk2"
elif command -v wrk &>/dev/null; then
    HAS_WRK=true
    BENCHMARK_BIN="wrk"
else
    BENCHMARK_BIN="go-loadgen"
fi

GENERATED_ARTIFACTS=()

run_single_benchmark() {
    local conn="$1"
    local rate="$2"
    local dur="$3"
    local tag="c${conn}_r${rate}"
    local json_out="${RESULTS_DIR}/benchmark_${tag}.json"
    local csv_out="${RESULTS_DIR}/benchmark_${tag}.csv"
    local raw_out="${RESULTS_DIR}/benchmark_${tag}.raw.txt"

    GENERATED_ARTIFACTS+=("${json_out}" "${csv_out}" "${raw_out}")

    echo ""
    echo "[+] Running benchmark: Concurrency=${conn}, TargetRate=${rate} req/s, Duration=${dur}"

    if [ "$BENCHMARK_BIN" = "wrk2" ]; then
        echo "[Engine: wrk2 (C-based with coordinated omission correction)]"
        local cmd=("wrk2" "-t${THREADS}" "-c${conn}" "-d${dur}" "-R${rate}" "--latency" "${TARGET_URL}")
        if [ -n "${LUA_SCRIPT}" ]; then
            cmd+=("-s" "${LUA_SCRIPT}")
        fi
        "${cmd[@]}" | tee "${raw_out}"
    elif [ "$BENCHMARK_BIN" = "wrk" ]; then
        echo "[Engine: wrk (Standard C-based)]"
        local cmd=("wrk" "-t${THREADS}" "-c${conn}" "-d${dur}" "--latency" "${TARGET_URL}")
        if [ -n "${LUA_SCRIPT}" ]; then
            cmd+=("-s" "${LUA_SCRIPT}")
        fi
        "${cmd[@]}" | tee "${raw_out}"
    else
        echo "[Engine: go-loadgen (Zero-dependency fallback)]"
        go run "${SCRIPT_DIR}/loadgen.go" \
            -url "${TARGET_URL}" \
            -c "${conn}" \
            -d "${dur}" \
            -rate "${rate}" \
            -json "${json_out}" \
            -csv "${csv_out}" | tee "${raw_out}"
    fi
}

if [ "$SWEEP_MODE" = "true" ]; then
    echo "[*] Initiating Automated Concurrency Scaling Sweep for Research Graphs..."
    SWEEP_CSV="${RESULTS_DIR}/concurrency_sweep_summary.csv"
    GENERATED_ARTIFACTS+=("${SWEEP_CSV}")
    echo "Concurrency,TargetRate,ResultJSON" > "${SWEEP_CSV}"

    CONCURRENCIES=(50 100 250 500 1000)
    for c in "${CONCURRENCIES[@]}"; do
        run_single_benchmark "${c}" "${TARGET_RATE}" "${DURATION}"
        echo "${c},${TARGET_RATE},benchmark_c${c}_r${TARGET_RATE}.json" >> "${SWEEP_CSV}"
        sleep 1
    done
    echo ""
    echo "[✓] Concurrency sweep completed. Master summary written to: ${SWEEP_CSV}"
else
    run_single_benchmark "${CONCURRENCY}" "${TARGET_RATE}" "${DURATION}"
fi

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

ARTIFACTS_JOINED=$(IFS=,; echo "${GENERATED_ARTIFACTS[*]}")
PARAMS_JSON=$(printf '{"concurrency": %d, "rate": %d, "duration": "%s", "engine": "%s", "sweep": %s}' "${CONCURRENCY}" "${TARGET_RATE}" "${DURATION}" "${BENCHMARK_BIN}" "${SWEEP_MODE}")

archive_benchmark_artifacts "wrk2" "success" "${ELAPSED}" "${ARTIFACTS_JOINED}" "${PARAMS_JSON}" "$0 $*"

echo ""
echo "================================================================================"
echo " [✓] Performance benchmarking complete. Artifacts ready in:"
echo "     ${RESULTS_DIR}"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo "     History: ${TORON_BENCHMARK_SESSION_DIR}"
fi
echo "================================================================================"
