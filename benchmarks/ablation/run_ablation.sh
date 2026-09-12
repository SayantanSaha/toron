#!/usr/bin/env bash
# ==============================================================================
# Toron 10-Task Controlled Ablation Experiment Suite (BMK-05)
# Paper 1 Comparative Evaluation: Multi-Agent with ADRs vs Direct Prompting
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

DATA_DIR="${SCRIPT_DIR}/data"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
JSON_OUT="${RESULTS_DIR}/ablation_study_report.json"
MD_OUT="${RESULTS_DIR}/ablation_study_report.md"

mkdir -p "${RESULTS_DIR}"

echo "=================================================================="
echo " Toron 10-Task Controlled Ablation Experiment Suite (BMK-05)"
echo " Condition A: Artifact-Anchored Multi-Agent Pipeline with ADRs"
echo " Condition B: Direct Single-Agent Baseline Prompting"
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

echo "==> Successfully generated ablation reports:"
echo "    JSON:     ${JSON_OUT}"
echo "    Markdown: ${MD_OUT}"
echo "=================================================================="
echo " Ablation Experiment Suite Verified & Completed Successfully"
echo "=================================================================="
