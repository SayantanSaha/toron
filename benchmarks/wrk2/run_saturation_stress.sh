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
CONCURRENCY=50
TARGET_RATE=5000
ATTACK_RATIO=0.10
AUTO_START=false
JSON_OUT="${RESULTS_DIR}/saturation_stress_report.json"
MD_OUT="${RESULTS_DIR}/saturation_stress_report.md"
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""
TIER=""
DURATIONS_LIST=()

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -u <url>              Target URL (default: http://127.0.0.1:8080/health)"
    echo "  -c <conns>            Concurrent connections (default: 50)"
    echo "  -d <duration>         Duration or comma-separated list (default: 5s, e.g. 5s,60s,300s)"
    echo "  --tier <tier>         Duration tier preset (quick=5s, medium/steady=60s, soak=300s, all=5s,60s,300s)"
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
        -d)
            IFS=',' read -ra DURATIONS_LIST <<< "$2"
            shift 2
            ;;
        --tier)
            TIER="$2"
            case "$TIER" in
                quick) DURATIONS_LIST=("5s") ;;
                medium|steady) DURATIONS_LIST=("60s") ;;
                soak) DURATIONS_LIST=("300s") ;;
                all) DURATIONS_LIST=("5s" "60s" "300s") ;;
                *) echo "[-] Error: Unknown tier '$TIER'. Valid tiers: quick, medium, soak, all"; exit 1 ;;
            esac
            shift 2
            ;;
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

if [ ${#DURATIONS_LIST[@]} -eq 0 ]; then
    DURATIONS_LIST=("5s")
    TIER="quick"
fi

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

GC_LOG="${RESULTS_DIR}/server_gc_trace.log"
SERVER_LOG="${RESULTS_DIR}/server_stress.log"

if [ "${AUTO_START}" = "true" ]; then
    echo "[*] Compiling Toron binary for evaluation..."
    go build -o "${RESULTS_DIR}/toron_stress" "${ROOT_DIR}/cmd/toron"
    echo "[*] Starting Toron background gateway on :8080 with GODEBUG=gctrace=1..."
    GODEBUG=gctrace=1 "${RESULTS_DIR}/toron_stress" -config "${ROOT_DIR}/config.yaml" -routes "${ROOT_DIR}/routes.yaml" > "${SERVER_LOG}" 2> "${GC_LOG}" &
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
echo " Durations:        ${DURATIONS_LIST[*]}"
if [ -n "${TIER}" ]; then
    echo " Tier:             ${TIER}"
fi
echo " Target Rate:      ${TARGET_RATE} RPS"
echo " Attack Ratio:     ${ATTACK_RATIO} (10% adversarial vectors)"
echo " Output Artifacts: ${JSON_OUT}"
echo "                   ${MD_OUT}"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo " History Session:  ${TORON_BENCHMARK_SESSION_DIR}"
fi
echo "================================================================================"

GENERATED_JSONS=()
GENERATED_MDS=()

for DUR in "${DURATIONS_LIST[@]}"; do
    CUR_TIER="quick"
    if [ "$DUR" = "60s" ] || [ "$DUR" = "1m" ]; then
        CUR_TIER="medium"
    elif [ "$DUR" = "300s" ] || [ "$DUR" = "5m" ]; then
        CUR_TIER="soak"
    fi
    if [ ${#DURATIONS_LIST[@]} -eq 1 ] && [ -n "$TIER" ] && [ "$TIER" != "all" ]; then
        CUR_TIER="$TIER"
    fi

    TIER_JSON="${RESULTS_DIR}/saturation_stress_${DUR}.json"
    TIER_MD="${RESULTS_DIR}/saturation_stress_${DUR}.md"

    echo ""
    echo "[*] Running Saturation Stress Tier: ${CUR_TIER} (${DUR})..."
    go run "${SCRIPT_DIR}/loadgen.go" \
        -url "${TARGET_URL}" \
        -c "${CONCURRENCY}" \
        -d "${DUR}" \
        -rate "${TARGET_RATE}" \
        -attack-ratio "${ATTACK_RATIO}" \
        -tier "${CUR_TIER}" \
        -gc-trace "${GC_LOG}" \
        -json "${TIER_JSON}" \
        -md "${TIER_MD}"

    GENERATED_JSONS+=("${TIER_JSON}")
    GENERATED_MDS+=("${TIER_MD}")
done

if [ ${#DURATIONS_LIST[@]} -eq 1 ]; then
    cp -f "${GENERATED_JSONS[0]}" "${JSON_OUT}"
    cp -f "${GENERATED_MDS[0]}" "${MD_OUT}"
else
    echo ""
    echo "[*] Consolidating multi-tier saturation reports..."
    JOINED_JSONS=$(IFS=,; echo "${GENERATED_JSONS[*]}")
    go run "${SCRIPT_DIR}/loadgen.go" \
        -consolidate-from "${JOINED_JSONS}" \
        -json "${JSON_OUT}" \
        -md "${MD_OUT}"
fi

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

DURATION_SUMMARY=$(IFS=,; echo "${DURATIONS_LIST[*]}")
PARAMS_JSON=$(printf '{"concurrency": %d, "rate": %d, "duration": "%s", "tier": "%s", "attack_ratio": %f}' "${CONCURRENCY}" "${TARGET_RATE}" "${DURATION_SUMMARY}" "${TIER:-custom}" "${ATTACK_RATIO}")

# Build artifacts list for retention
ARTIFACTS_ARRAY=("${JSON_OUT}" "${MD_OUT}")
if [ -f "${GC_LOG}" ]; then
    ARTIFACTS_ARRAY+=("${GC_LOG}")
fi
for f in "${GENERATED_JSONS[@]}" "${GENERATED_MDS[@]}"; do
    if [ -f "$f" ]; then
        ARTIFACTS_ARRAY+=("$f")
    fi
done
ALL_ARTIFACTS=$(IFS=,; echo "${ARTIFACTS_ARRAY[*]}")

archive_benchmark_artifacts "saturation_stress" "success" "${ELAPSED}" "${ALL_ARTIFACTS}" "${PARAMS_JSON}" "$0 $*"

echo "[*] Saturation evaluation complete. Reports saved to ${RESULTS_DIR}."
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo "[*] Historical session snapshot preserved in ${TORON_BENCHMARK_SESSION_DIR}."
fi
