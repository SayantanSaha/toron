#!/usr/bin/env bash
# ==============================================================================
# Toron Differential Multi-Proxy Docker Benchmark Runner
# Compares Toron against NGINX, Traefik, Caddy, and HAProxy with identical
# heterogeneous upstream origins (Node.js, Python, Go, and Fast-Echo).
# Integrates with Toron historical retention tier (manifest.json, REQ-119).
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RESULTS_DIR="${ROOT_DIR}/benchmarks/results"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.compare.yml"
mkdir -p "${RESULTS_DIR}"

# Source historical retention helper
source "${ROOT_DIR}/benchmarks/archive_run.sh"

CONCURRENCY=50
DURATION="5s"
TIER=""
RATE=0
PROXIES="toron,nginx,traefik,caddy,haproxy"
BACKENDS="fast,go,node,python"
PREFLIGHT_ONLY=false
FORCE_BUILD=false
AUTO_TEARDOWN=false
DOWN_ONLY=false
NO_HISTORY=false
SESSION_NAME=""
CUSTOM_SESSION_DIR=""

print_usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -c <conns>            Worker concurrency (default: 50)"
    echo "  -d <duration>         Test duration per target (default: 5s, e.g. 5s,60s,300s)"
    echo "  --tier <tier>         Duration tier preset (quick=5s, medium/steady=60s, soak=300s, all=5s,60s,300s)"
    echo "  -r <rate>             Rate limit RPS (default: 0 = unthrottled saturation)"
    echo "  --proxies <list>      Comma-separated proxies: toron,nginx,traefik,caddy,haproxy"
    echo "  --backends <list>     Comma-separated origins: fast,go,node,python"
    echo "  --preflight-only      Run pre-flight functional checks only and exit"
    echo "  --build               Force rebuild Docker images before running"
    echo "  --teardown            Automatically stop containers after benchmark run"
    echo "  --down, --clean       Tear down all benchmark containers and networks"
    echo "  --no-history          Disable historical result retention in manifest.json"
    echo "  --session-name <name> Custom suffix for historical run directory"
    echo "  -h, --help            Display this help message"
    echo ""
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -c) CONCURRENCY="$2"; shift 2 ;;
        -d) DURATION="$2"; shift 2 ;;
        --tier)
            TIER="$2"
            case "$TIER" in
                quick) DURATION="5s" ;;
                medium|steady) DURATION="60s" ;;
                soak) DURATION="300s" ;;
                all) DURATION="5s,60s,300s" ;;
                *) echo "[-] Error: Unknown tier '$TIER'. Valid tiers: quick, medium, soak, all"; exit 1 ;;
            esac
            shift 2
            ;;
        -r) RATE="$2"; shift 2 ;;
        --proxies) PROXIES="$2"; shift 2 ;;
        --backends) BACKENDS="$2"; shift 2 ;;
        --preflight-only) PREFLIGHT_ONLY=true; shift ;;
        --build) FORCE_BUILD=true; shift ;;
        --teardown) AUTO_TEARDOWN=true; shift ;;
        --down|--clean) DOWN_ONLY=true; shift ;;
        --no-history) NO_HISTORY=true; export TORON_NO_HISTORY=true; shift ;;
        --session-name) SESSION_NAME="$2"; shift 2 ;;
        --session-dir) CUSTOM_SESSION_DIR="$2"; shift 2 ;;
        -h|--help) print_usage; exit 0 ;;
        *) echo "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

teardown_cluster() {
    echo ""
    echo "[*] Tearing down benchmark Docker Compose cluster..."
    docker compose -f "${COMPOSE_FILE}" down --remove-orphans || true
    echo "[✓] Benchmark cluster stopped and ports released."
}

if [ "$DOWN_ONLY" = "true" ]; then
    teardown_cluster
    exit 0
fi

if [ "$AUTO_TEARDOWN" = "true" ]; then
    trap teardown_cluster EXIT INT TERM
fi

START_TIME=$(date +%s)

# Initialize retention session
if [ "$NO_HISTORY" = "true" ]; then
    export TORON_NO_HISTORY=true
elif [ -n "${CUSTOM_SESSION_DIR}" ]; then
    export TORON_BENCHMARK_SESSION_DIR="${CUSTOM_SESSION_DIR}"
    mkdir -p "${TORON_BENCHMARK_SESSION_DIR}"
    export IS_STANDALONE_SESSION=false
else
    init_benchmark_session "docker-compare" "${SESSION_NAME}"
fi

JSON_OUT="${RESULTS_DIR}/docker_compare_report.json"
MD_OUT="${RESULTS_DIR}/docker_compare_report.md"

echo "================================================================================"
echo "          TORON MULTI-PROXY DIFFERENTIAL DOCKER BENCHMARK ORCHESTRATOR           "
echo "================================================================================"
echo " Proxies:        ${PROXIES}"
echo " Backends:       ${BACKENDS}"
echo " Concurrency:    ${CONCURRENCY}"
echo " Duration:       ${DURATION}"
echo " Rate Limit:     ${RATE} RPS (0 = unthrottled)"
echo " Pre-flight:     ${PREFLIGHT_ONLY}"
echo " Canonical JSON: ${JSON_OUT}"
echo " Canonical MD:   ${MD_OUT}"
if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
    echo " History Archive:${TORON_BENCHMARK_SESSION_DIR}"
fi
echo "================================================================================"

# Check if containers are already running or need to be launched
RUNNING_CONTAINERS=$(docker compose -f "${COMPOSE_FILE}" ps -q 2>/dev/null || true)

if [ -z "${RUNNING_CONTAINERS}" ] || [ "${FORCE_BUILD}" = "true" ]; then
    echo ""
    echo "[*] Starting/Rebuilding benchmark Docker Compose cluster..."
    if [ "${FORCE_BUILD}" = "true" ]; then
        docker compose -f "${COMPOSE_FILE}" up -d --build
    else
        docker compose -f "${COMPOSE_FILE}" up -d
    fi

    echo "[*] Waiting for proxy gateways and origin backends to be ready..."
    sleep 4
fi

# Run the Go orchestrator
RUNNER_ARGS=(
    "-c" "${CONCURRENCY}"
    "-d" "${DURATION}"
    "-r" "${RATE}"
    "-proxies" "${PROXIES}"
    "-backends" "${BACKENDS}"
    "-json" "${JSON_OUT}"
    "-md" "${MD_OUT}"
)

if [ -n "${TIER}" ]; then
    RUNNER_ARGS+=("-tier" "${TIER}")
fi

if [ "${PREFLIGHT_ONLY}" = "true" ]; then
    RUNNER_ARGS+=("-preflight-only")
fi

echo ""
echo "[*] Executing Go Benchmark Orchestrator..."
go run "${SCRIPT_DIR}/runner.go" "${RUNNER_ARGS[@]}"

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

# Archive artifacts to historical retention manifest if retention is enabled
if [ "${PREFLIGHT_ONLY}" = "false" ] && [ -f "${JSON_OUT}" ] && [ -f "${MD_OUT}" ]; then
    PARAMS_JSON=$(printf '{"concurrency": %d, "duration": "%s", "tier": "%s", "rate": %d, "proxies": "%s", "backends": "%s"}' \
        "${CONCURRENCY}" "${DURATION}" "${TIER:-custom}" "${RATE}" "${PROXIES}" "${BACKENDS}")

    archive_benchmark_artifacts "docker-compare" "success" "${ELAPSED}" "${JSON_OUT},${MD_OUT}" "${PARAMS_JSON}" "$0 $*"
    echo ""
    echo "[✓] Master retention index updated in benchmarks/results/history/manifest.json"
fi

echo ""
echo "================================================================================"
echo " [✓] DIFFERENTIAL BENCHMARK EXECUTION COMPLETE                                  "
echo "================================================================================"
echo " Markdown Report: ${MD_OUT}"
echo " JSON Report:     ${JSON_OUT}"
echo "================================================================================"
