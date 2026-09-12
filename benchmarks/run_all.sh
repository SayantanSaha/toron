#!/usr/bin/env bash
# ==============================================================================
# Toron Master Benchmark & Differential Security Evaluation Orchestrator
# Executes full benchmark suite across microbenchmarks, loadgen, stress testing,
# differential fuzzing (K=1,000), multi-hop testbed, and controlled ablation.
# Retains all empirical outputs in benchmarks/results/history/<timestamp>/
# with structured manifest indexing and canonical latest synchronization.
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
HISTORY_BASE="${RESULTS_DIR}/history"
mkdir -p "${RESULTS_DIR}" "${HISTORY_BASE}"

# Source retention helper
source "${SCRIPT_DIR}/archive_run.sh"

AUTO_START=false
TARGET_HOST="127.0.0.1:8080"
SERVER_PID=""
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""
MULTIHOP_MODE="standalone"

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
    echo "  --auto-start          Automatically compile and launch local Toron server in background"
    echo "  -t <host:port>        Target host:port (default: 127.0.0.1:8080)"
    echo "  --docker              Run multi-hop stage with multi-container Docker Compose cluster"
    echo "  --no-history          Disable historical retention (only update canonical benchmarks/results)"
    echo "  --session-name <name> Optional custom suffix for the historical run directory"
    echo "  --session-dir <dir>   Explicit destination session directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --auto-start) AUTO_START=true; shift ;;
        -t) TARGET_HOST="$2"; shift 2 ;;
        --docker) MULTIHOP_MODE="docker"; shift ;;
        --no-history) NO_HISTORY=true; export TORON_NO_HISTORY=true; shift ;;
        --session-name) SESSION_NAME="$2"; shift 2 ;;
        --session-dir) CUSTOM_SESSION_DIR="$2"; shift 2 ;;
        -h|--help) print_usage; exit 0 ;;
        *) echo "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

SUITE_START_TIME=$(date +%s)

# Initialize retention session
if [ "$NO_HISTORY" = "true" ]; then
    echo "[*] Historical result retention is DISABLED."
    ACTIVE_SESSION_DIR=""
elif [ -n "${CUSTOM_SESSION_DIR}" ]; then
    export TORON_BENCHMARK_SESSION_DIR="${CUSTOM_SESSION_DIR}"
    mkdir -p "${TORON_BENCHMARK_SESSION_DIR}"
    ACTIVE_SESSION_DIR="${TORON_BENCHMARK_SESSION_DIR}"
    export IS_STANDALONE_SESSION=false
else
    init_benchmark_session "all" "${SESSION_NAME}"
    ACTIVE_SESSION_DIR="${TORON_BENCHMARK_SESSION_DIR}"
fi

echo "================================================================================"
echo "          TORON AUTOMATED RESEARCH EVALUATION & BENCHMARK SUITE                 "
echo "================================================================================"
echo " Target Endpoint:    http://${TARGET_HOST}"
echo " Auto-Start Mode:    ${AUTO_START}"
echo " Canonical Results:  ${RESULTS_DIR}"
if [ -n "${ACTIVE_SESSION_DIR}" ]; then
    echo " History Directory:  ${ACTIVE_SESSION_DIR}"
fi
echo "================================================================================"

echo ""
echo "[1/6] Running In-Process Parser Microbenchmarks (testing.B, TST-05)..."
go test -bench=BenchmarkParseRequest -benchmem -run=^$ "${ROOT_DIR}/pkg/httpparser/..." | tee "${RESULTS_DIR}/microbenchmarks.raw.txt"
if [ -n "${ACTIVE_SESSION_DIR}" ]; then
    cp -f "${RESULTS_DIR}/microbenchmarks.raw.txt" "${ACTIVE_SESSION_DIR}/microbenchmarks.raw.txt"
fi

if [ "$AUTO_START" = "true" ]; then
    echo ""
    echo "[*] Building Toron executable..."
    (cd "${ROOT_DIR}" && go build -o "${RESULTS_DIR}/toron_eval" ./cmd/toron)

    echo "[*] Launching Toron server in background..."
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
    if [ -n "${ACTIVE_SESSION_DIR}" ]; then
        cp -f "${RESULTS_DIR}/server.log" "${ACTIVE_SESSION_DIR}/server.log"
    fi
fi

echo ""
echo "[2/6] Executing Baseline High-Throughput & Tail Latency Benchmark (wrk2 harness)..."
bash "${SCRIPT_DIR}/wrk2/run_wrk2.sh" -u "http://${TARGET_HOST}/health" -c 100 -d 5s -r 5000

echo ""
echo "[3/6] Executing High-Concurrency Saturation Stress Testing with 10% Adversarial Injection (BMK-04)..."
bash "${SCRIPT_DIR}/wrk2/run_saturation_stress.sh" -u "http://${TARGET_HOST}/health" -c 50 -d 5s -r 5000 -a 0.10

echo ""
echo "[4/6] Executing Differential Protocol Security Fuzzer (Equation 7 & K=1,000 Trials, BMK-01, BMK-02)..."
bash "${SCRIPT_DIR}/fuzzer/run_fuzzer.sh" -t "${TARGET_HOST}" -k 1000 -w 50

if [ -n "${SERVER_PID}" ]; then
    echo ""
    echo "[*] Stopping background Toron server before isolated testbeds..."
    kill -TERM "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    SERVER_PID=""
fi

echo ""
echo "[5/6] Executing Heterogeneous Multi-Hop Backend Origin Testbed (Node.js, Python, Go, BMK-03)..."
bash "${SCRIPT_DIR}/multihop/run_multihop.sh" --${MULTIHOP_MODE}

echo ""
echo "[6/6] Executing 10-Task Controlled Ablation Experiment Suite (TASK-061 to TASK-070, BMK-05)..."
bash "${SCRIPT_DIR}/ablation/run_ablation.sh"

SUITE_END_TIME=$(date +%s)
SUITE_DURATION=$((SUITE_END_TIME - SUITE_START_TIME))

# Finalize master session recording in manifest.json
if [ -n "${ACTIVE_SESSION_DIR}" ]; then
    echo ""
    echo "[*] Finalizing historical session archive in ${ACTIVE_SESSION_DIR}..."

    # Replicate all canonical report files to ensure 100% completion in history directory
    ALL_FILES=(
        "microbenchmarks.raw.txt"
        "benchmark_c100_r5000.json"
        "benchmark_c100_r5000.csv"
        "benchmark_c100_r5000.raw.txt"
        "saturation_stress_report.json"
        "saturation_stress_report.md"
        "differential_fuzz_report.json"
        "differential_fuzz_report.md"
        "multihop_report.json"
        "multihop_report.md"
        "ablation_study_report.json"
        "ablation_study_report.md"
    )
    for f in "${ALL_FILES[@]}"; do
        if [ -f "${RESULTS_DIR}/${f}" ]; then
            cp -f "${RESULTS_DIR}/${f}" "${ACTIVE_SESSION_DIR}/${f}"
        fi
    done

    STAGES_JSON='[
      {"name": "microbenchmarks", "status": "success", "artifacts": ["microbenchmarks.raw.txt"]},
      {"name": "wrk2", "status": "success", "parameters": {"concurrency": 100, "rate": 5000, "duration": "5s"}, "artifacts": ["benchmark_c100_r5000.json", "benchmark_c100_r5000.csv", "benchmark_c100_r5000.raw.txt"]},
      {"name": "saturation_stress", "status": "success", "parameters": {"concurrency": 50, "rate": 5000, "duration": "5s", "attack_ratio": 0.10}, "artifacts": ["saturation_stress_report.json", "saturation_stress_report.md"]},
      {"name": "differential_fuzzer", "status": "success", "parameters": {"trials": 1000, "warmup": 50}, "artifacts": ["differential_fuzz_report.json", "differential_fuzz_report.md"]},
      {"name": "multihop", "status": "success", "parameters": {"mode": "'"${MULTIHOP_MODE}"'"}, "artifacts": ["multihop_report.json", "multihop_report.md"]},
      {"name": "ablation", "status": "success", "parameters": {"tasks": 10}, "artifacts": ["ablation_study_report.json", "ablation_study_report.md"]}
    ]'

    ARTIFACTS_LIST=$(IFS=,; echo "${ALL_FILES[*]}")

    bash "${SCRIPT_DIR}/archive_run.sh" record \
        -session-dir "${ACTIVE_SESSION_DIR}" \
        -suite "all" \
        -status "success" \
        -duration "${SUITE_DURATION}" \
        -command "$0 $*" \
        -artifacts "${ARTIFACTS_LIST}" \
        -stages-json "${STAGES_JSON}"

    echo "[✓] Manifest successfully updated at: ${HISTORY_BASE}/manifest.json"
fi

echo ""
echo "================================================================================"
echo " [✓] ALL EVALUATION ARTIFACTS SUCCESSFULLY GENERATED                            "
echo "================================================================================"
echo " Latest Canonical Artifacts: ${RESULTS_DIR}"
if [ -n "${ACTIVE_SESSION_DIR}" ]; then
    echo " Historical Session Archive: ${ACTIVE_SESSION_DIR}"
    echo " Master Manifest Index:      ${HISTORY_BASE}/manifest.json"
fi
echo "================================================================================"
ls -lh "${RESULTS_DIR}"
echo "================================================================================"
