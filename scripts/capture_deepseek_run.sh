#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

REPO="${1:-../etcd}"

if [ -z "${DEEPSEEK_API_KEY:-}" ]; then
    echo "Skipping: DEEPSEEK_API_KEY is not set"
    exit 0
fi

if [ ! -d "$REPO" ] || [ ! -d "$REPO/.git" ]; then
    echo "Skipping: $REPO is not a git repo"
    echo "  Usage: $0 [path-to-repo]"
    exit 0
fi

echo "=== Capturing DeepSeek run ($REPO) ==="

set +e
go run ./cmd/repomap orient --repo "$REPO" --debug-dir .repomap-runs --dump-llm > /dev/null 2>tmp/capture-deepseek-stderr.txt
EXIT_CODE=$?
set -e

LAST_RUN=$(find .repomap-runs -maxdepth 1 -type d -not -name .repomap-runs 2>/dev/null | sort -r | head -1)

if [ $EXIT_CODE -ne 0 ]; then
    echo ""
    echo "FAIL: repomap exited with code $EXIT_CODE"
    echo "stderr:"
    cat tmp/capture-deepseek-stderr.txt 2>/dev/null || true
    echo ""
    if [ -n "$LAST_RUN" ]; then
        echo "Debug dir: $LAST_RUN"
        if [ -f "$LAST_RUN/error.txt" ]; then
            echo "--- error.txt ---"
            cat "$LAST_RUN/error.txt"
        fi
        if [ -f "$LAST_RUN/llm_response.raw.json" ]; then
            echo "--- llm_response.raw.json (first 500 bytes, redacted) ---"
            head -c 500 "$LAST_RUN/llm_response.raw.json"
        fi
    fi
    exit $EXIT_CODE
fi

echo ""
if [ -n "$LAST_RUN" ]; then
    echo "Debug dir: $LAST_RUN"
    echo "Files:"
    find "$LAST_RUN" -type f | sed "s|$LAST_RUN/|  |" | sort
else
    echo "WARNING: no debug directory created"
    exit 1
fi

echo ""
echo "OK"
