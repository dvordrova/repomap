#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

LAST_RUN=$(find .repomap-runs -maxdepth 1 -type d -not -name .repomap-runs 2>/dev/null | sort -r | head -1)

if [ -z "$LAST_RUN" ]; then
    echo "No .repomap-runs/ directories found"
    exit 0
fi

echo "=== Last run: $LAST_RUN ==="
echo ""

if [ -f "$LAST_RUN/metadata.json" ]; then
    echo "--- metadata.json ---"
    if command -v jq &>/dev/null; then
        jq '.' "$LAST_RUN/metadata.json"
    else
        cat "$LAST_RUN/metadata.json"
    fi
    echo ""
fi

echo "--- Files ---"
find "$LAST_RUN" -type f | sed "s|$LAST_RUN/|  |" | sort
echo ""

if [ -f "$LAST_RUN/error.txt" ]; then
    echo "--- error.txt ---"
    cat "$LAST_RUN/error.txt"
    echo ""
fi

if [ -f "$LAST_RUN/llm_bundle.json" ] && command -v jq &>/dev/null; then
    echo "--- llm_bundle.json summary ---"
    jq '{repo_name, go: {modules_count: .go.modules_count, packages_count: .go.packages_count, entrypoints_count: (.go.entrypoints | length), candidates_count: (.go.orientation_candidates | length)}}' "$LAST_RUN/llm_bundle.json"
    echo ""
fi

if [ -f "$LAST_RUN/orientation_report.json" ] && command -v jq &>/dev/null; then
    echo "--- orientation_report.json summary ---"
    jq '{project_guess, confidence, candidate_flows_count: (.candidate_flows | length), first_files: (.first_files_to_open | length)}' "$LAST_RUN/orientation_report.json"
    echo ""
fi

echo "OK"
