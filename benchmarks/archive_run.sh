#!/usr/bin/env bash
# ==============================================================================
# Toron Benchmark Result Retention & Historical Archival Helper (REQ-119)
# ==============================================================================
set -euo pipefail

RETENTION_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RETENTION_ROOT_DIR="$(cd "${RETENTION_SCRIPT_DIR}/.." && pwd)"
RETENTION_RESULTS_DIR="${RETENTION_SCRIPT_DIR}/results"
RETENTION_HISTORY_BASE="${RETENTION_RESULTS_DIR}/history"
RETENTION_CANONICAL_DIR="${RETENTION_RESULTS_DIR}"

mkdir -p "${RETENTION_HISTORY_BASE}"

# Helper to execute the Go retention utility
exec_retention() {
    go run "${RETENTION_SCRIPT_DIR}/retention/cmd/main.go" \
        -history-base "${RETENTION_HISTORY_BASE}" \
        -canonical-dir "${RETENTION_CANONICAL_DIR}" \
        "$@"
}

# ------------------------------------------------------------------------------
# Functions for sourcing into benchmark scripts
# ------------------------------------------------------------------------------

init_benchmark_session() {
    local suite_name="${1:-standalone}"
    local session_suffix="${2:-}"

    # If an outer master session is already active, inherit it
    if [ -n "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
        echo "[Retention] Inheriting master session: ${TORON_BENCHMARK_SESSION_DIR}"
        IS_STANDALONE_SESSION=false
        return 0
    fi

    # Check if history archiving is globally disabled
    if [ "${TORON_NO_HISTORY:-false}" = "true" ]; then
        echo "[Retention] History retention is disabled (--no-history)."
        IS_STANDALONE_SESSION=false
        return 0
    fi

    local init_output
    init_output=$(exec_retention -action init -suite "${suite_name}" -session-name "${session_suffix}")
    eval "${init_output}" # sets SESSION_DIR, SESSION_NAME, SESSION_TS

    export TORON_BENCHMARK_SESSION_DIR="${SESSION_DIR}"
    export TORON_BENCHMARK_SESSION_TS="${SESSION_TS}"
    export IS_STANDALONE_SESSION=true

    echo "[Retention] Initialized historical session: ${TORON_BENCHMARK_SESSION_DIR}"
}

archive_benchmark_artifacts() {
    local suite_name="$1"
    local status="$2"
    local duration="$3"
    local artifacts="$4"
    local params_json="${5:-{}}"
    local command_str="${6:-}"

    if [ "${TORON_NO_HISTORY:-false}" = "true" ]; then
        return 0
    fi

    if [ -z "${TORON_BENCHMARK_SESSION_DIR:-}" ]; then
        return 0
    fi

    # If this is a standalone execution, run full archive and update manifest.json
    if [ "${IS_STANDALONE_SESSION:-false}" = "true" ]; then
        exec_retention -action archive \
            -session-dir "${TORON_BENCHMARK_SESSION_DIR}" \
            -suite "${suite_name}" \
            -status "${status}" \
            -duration "${duration}" \
            -artifacts "${artifacts}" \
            -params-json "${params_json}" \
            -command "${command_str}"
        echo "[Retention] Standalone run archived and registered in ${RETENTION_HISTORY_BASE}/manifest.json"
    else
        # In master session: copy artifacts to session dir and ensure canonical dir is updated
        IFS=',' read -ra ARTIFACT_ARRAY <<< "${artifacts}"
        for f in "${ARTIFACT_ARRAY[@]}"; do
            f_clean="$(echo "${f}" | xargs)"
            if [ -f "${f_clean}" ]; then
                base_f="$(basename "${f_clean}")"
                cp -f "${f_clean}" "${TORON_BENCHMARK_SESSION_DIR}/${base_f}"
                if [ "${f_clean}" != "${RETENTION_CANONICAL_DIR}/${base_f}" ]; then
                    cp -f "${f_clean}" "${RETENTION_CANONICAL_DIR}/${base_f}"
                fi
            fi
        done
        echo "[Retention] Replicated stage artifacts (${suite_name}) to ${TORON_BENCHMARK_SESSION_DIR}"
    fi
}

# ------------------------------------------------------------------------------
# Direct CLI execution dispatcher
# ------------------------------------------------------------------------------
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    if [ $# -eq 0 ]; then
        echo "Usage: $0 <init|archive|record> [options]"
        exit 1
    fi

    action="$1"
    shift
    exec_retention -action "${action}" "$@"
fi
