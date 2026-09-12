#!/usr/bin/env bash
# ==============================================================================
# Toron 10-Task Controlled Ablation Experiment Suite (BMK-05)
# Paper 1 Comparative Evaluation: Multi-Agent with ADRs vs Direct Prompting
# Retains results in benchmarks/results/history/<timestamp>/ with manifest tracking.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Source retention helper
source "${ROOT_DIR}/benchmarks/archive_run.sh"

DATA_DIR="${SCRIPT_DIR}/data"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
JSON_OUT="${RESULTS_DIR}/ablation_study_report.json"
MD_OUT="${RESULTS_DIR}/ablation_study_report.md"
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""

mkdir -p "${RESULTS_DIR}"

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -j <file>             Output JSON path (default: benchmarks/results/ablation_study_report.json)"
    echo "  -m <file>             Output Markdown path (default: benchmarks/results/ablation_study_report.md)"
    echo "  --no-history          Disable historical retention"
    echo "  --session-name <name> Custom suffix for historical run directory"
    echo "  --session-dir <dir>   Explicit destination session directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
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
    init_benchmark_session "ablation" "${SESSION_NAME}"
fi

echo "=================================================================="
echo " Toron 10-Task Controlled Ablation Experiment Suite (BMK-05)"
echo " Condition A: Artifact-Anchored Multi-Agent Pipeline with ADRs"
echo " Condition B: Direct Single-Agent Baseline Prompting"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo " History Session: ${TORON_BENCHMARK_SESSION_DIR}"
fi
echo "=================================================================="

# 1. Execute Unit & Race Tests
echo "==> Running ablation harness unit and race tests..."
go test -v -race -count=1 "${ROOT_DIR}/benchmarks/ablation/..."

# 2. Run the Ablation Evaluation Engine
echo "==> Executing ablation study evaluation runner..."
go run "${ROOT_DIR}/benchmarks/ablation/cmd/main.go" \
  -data-dir "${DATA_DIR}" \
  -json "${JSON_OUT}" \
  -md "${MD_OUT}"

# 3. Assert Report Generation
if [[ ! -f "${JSON_OUT}" ]] || [[ ! -f "${MD_OUT}" ]]; then
  echo "[-] Error: Expected ablation reports were not generated!" >&2
  exit 1
fi

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

PARAMS_JSON='{"tasks": 10, "condition_a": "multi-agent-with-adrs", "condition_b": "direct-prompting"}'
archive_benchmark_artifacts "ablation" "success" "${ELAPSED}" "${JSON_OUT},${MD_OUT}" "${PARAMS_JSON}" "$0 $*"

echo "==> Successfully generated ablation reports:"
echo "    JSON:     ${JSON_OUT}"
echo "    Markdown: ${MD_OUT}"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo "    History:  ${TORON_BENCHMARK_SESSION_DIR}"
fi
echo "=================================================================="
echo " Ablation Experiment Suite Verified & Completed Successfully"
echo "=================================================================="
