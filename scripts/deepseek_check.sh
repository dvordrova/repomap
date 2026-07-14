#!/usr/bin/env bash
set -euo pipefail
set +x

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

ETCD_REPO="${1:-../etcd}"

unset REPOMAP_CHECK_AUTH REPOMAP_CHECK_HAS_KEY
if [ -n "${REPOMAP_LLM_ENDPOINT+x}${REPOMAP_LLM_MODEL+x}${REPOMAP_LLM_API_KEY+x}${REPOMAP_LLM_AUTH+x}${REPOMAP_LLM_MAX_TOKENS+x}${REPOMAP_LLM_TIMEOUT+x}" ]; then
    REPOMAP_CHECK_AUTH="${REPOMAP_LLM_AUTH:-bearer}"
    REPOMAP_CHECK_HAS_KEY=false
    if [ -n "${REPOMAP_LLM_API_KEY:-}" ]; then
        REPOMAP_CHECK_HAS_KEY=true
    fi
else
    REPOMAP_CHECK_AUTH="${DEEPSEEK_AUTH:-bearer}"
    REPOMAP_CHECK_HAS_KEY=false
    if [ -n "${DEEPSEEK_API_KEY:-}" ]; then
        REPOMAP_CHECK_HAS_KEY=true
    fi
fi
if [ "$REPOMAP_CHECK_AUTH" != "none" ] && [ "$REPOMAP_CHECK_HAS_KEY" != true ]; then
    echo "Skipping deepseek_check: bearer auth is configured but no REPOMAP_LLM_API_KEY or DEEPSEEK_API_KEY is set"
    exit 0
fi
unset REPOMAP_CHECK_AUTH REPOMAP_CHECK_HAS_KEY

if ! git -C "$ETCD_REPO" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "Skipping deepseek_check: $ETCD_REPO is not a git repo"
    echo "  Usage: $0 [path-to-etcd]"
    exit 0
fi

mkdir -p tmp .repomap-runs

echo "=== configured LLM orient ($ETCD_REPO) ==="
set +e
go run ./cmd/repomap orient --repo "$ETCD_REPO" --debug-dir .repomap-runs --dump-llm > tmp/deepseek-orientation.json 2>tmp/deepseek-stderr.txt
EXIT_CODE=$?
set -e

if [ $EXIT_CODE -ne 0 ]; then
    echo ""
    echo "FAIL: repomap exited with code $EXIT_CODE"
    echo "stderr:"
    cat tmp/deepseek-stderr.txt
    echo ""
    echo "Debug run dirs:"
    find .repomap-runs -maxdepth 1 -type d -not -name .repomap-runs | sort -r | head -5
    exit $EXIT_CODE
fi

echo "  wrote tmp/deepseek-orientation.json"

echo ""
if command -v jq &>/dev/null; then
    echo "--- Validating with jq ---"
    FAIL=0

    if jq -e '.orientation.candidate_flows | length > 0' tmp/deepseek-orientation.json > /dev/null; then
        echo "OK: candidate_flows present"
    else
        echo "FAIL: candidate_flows missing"
        FAIL=1
    fi

    if jq -e '.orientation.first_files_to_open' tmp/deepseek-orientation.json > /dev/null; then
        echo "OK: first_files_to_open present"
    else
        echo "FAIL: first_files_to_open missing"
        FAIL=1
    fi

    if [ $FAIL -ne 0 ]; then
        echo "FAIL: validation errors above"
        exit 1
    fi
else
    echo "(jq not found; skipping JSON validation)"
fi

echo ""
echo "=== Debug artifacts ==="
find .repomap-runs -maxdepth 2 -type f | sort -r | head -20
echo ""
echo "OK"
