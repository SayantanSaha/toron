#!/usr/bin/env bash
# ==============================================================================
# Toron Kubernetes Ingress & Service Mesh Sidecar Comparative Benchmark Runner
# Compares Toron against NGINX, Traefik, and Envoy inside a local Kubernetes cluster.
# Integrates with Toron historical retention tier (manifest.json, REQ-119).
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
CONFIGS_DIR="${SCRIPT_DIR}/configs"
mkdir -p "${RESULTS_DIR}"

# Source historical retention helper if present
if [ -f "${ROOT_DIR}/benchmarks/archive_run.sh" ]; then
    source "${ROOT_DIR}/benchmarks/archive_run.sh"
fi

REQUESTS=1500
CONCURRENCY=25
DEPLOY=false
TEARDOWN=false
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -n <requests>         Total requests per test (default: 1500)"
    echo "  -c <conns>            Worker concurrency (default: 25)"
    echo "  --deploy              Apply all 4 testbed manifests before running"
    echo "  --teardown            Tear down comparison namespaces after benchmark"
    echo "  --no-history          Disable historical result retention in manifest.json"
    echo "  --session-name <name> Custom suffix for historical run directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -n) REQUESTS="$2"; shift 2 ;;
        -c) CONCURRENCY="$2"; shift 2 ;;
        --deploy) DEPLOY=true; shift ;;
        --teardown) TEARDOWN=true; shift ;;
        --no-history) NO_HISTORY=true; export TORON_NO_HISTORY=true; shift ;;
        --session-name) SESSION_NAME="$2"; shift 2 ;;
        --session-dir) CUSTOM_SESSION_DIR="$2"; shift 2 ;;
        -h|--help) print_usage; exit 0 ;;
        *) echo "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

if [ "$DEPLOY" = "true" ]; then
    echo "[*] Deploying testbeds from ${CONFIGS_DIR}..."
    kubectl apply -f "${CONFIGS_DIR}/toron/manifest.yaml"
    kubectl apply -f "${CONFIGS_DIR}/nginx/manifest.yaml"
    kubectl apply -f "${CONFIGS_DIR}/traefik/manifest.yaml"
    kubectl apply -f "${CONFIGS_DIR}/envoy/manifest.yaml"
    echo "[*] Waiting for testbed deployments to become ready..."
    kubectl wait --for=condition=ready pod -l app=toron-ingress-controller -n toron-test --timeout=60s
    kubectl wait --for=condition=ready pod -l app=nginx-ingress -n nginx-test --timeout=60s
    kubectl wait --for=condition=ready pod -l app=traefik-ingress -n traefik-test --timeout=60s
    kubectl wait --for=condition=ready pod -l app=envoy-ingress -n envoy-test --timeout=60s
fi

SUITE_START=$(date +%s)

if [ "$NO_HISTORY" = "false" ] && type init_benchmark_session >/dev/null 2>&1; then
    init_benchmark_session "k8s_compare" "${SESSION_NAME}"
fi

JSON_REPORT="${RESULTS_DIR}/k8s_compare_report.json"
MD_REPORT="${RESULTS_DIR}/k8s_compare_report.md"

python3 "${SCRIPT_DIR}/runner.py" \
    --requests "${REQUESTS}" \
    --concurrency "${CONCURRENCY}" \
    --json-out "${JSON_REPORT}" \
    --md-out "${MD_REPORT}"

SUITE_END=$(date +%s)
SUITE_DURATION=$((SUITE_END - SUITE_START))

if [ "$NO_HISTORY" = "false" ] && type archive_benchmark_artifacts >/dev/null 2>&1; then
    archive_benchmark_artifacts "k8s_compare" "success" "${SUITE_DURATION}" "${JSON_REPORT},${MD_REPORT}"
fi

if [ "$TEARDOWN" = "true" ]; then
    echo "[*] Tearing down comparison testbeds..."
    kubectl delete ns nginx-test traefik-test envoy-test || true
fi

echo "[*] Kubernetes comparative benchmark completed in ${SUITE_DURATION}s."
