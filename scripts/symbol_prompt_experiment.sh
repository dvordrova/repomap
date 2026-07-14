#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

LABEL="${1:-}"
TARGET_REPO="${2:-../etcd}"
TARGET_SYMBOL="${3:-kvServer.Put}"
FORMAT="${4:-tagged}"

if [ -z "$LABEL" ]; then
    echo "Usage: $0 LABEL [repo] [exact-symbol] [json|tagged]" >&2
    exit 2
fi
if [ "$FORMAT" != "json" ] && [ "$FORMAT" != "tagged" ]; then
    echo "Format must be json or tagged" >&2
    exit 2
fi
if [ ! -d "$TARGET_REPO/.git" ]; then
    echo "$TARGET_REPO is not a git repository" >&2
    exit 2
fi

SAFE_LABEL="${LABEL//[^a-zA-Z0-9._-]/-}"
OUTPUT_DIR="tmp/prompt-experiments/${SAFE_LABEL}-${FORMAT}"

go run ./cmd/symbol-playground \
    --repo "$TARGET_REPO" \
    --symbol "$TARGET_SYMBOL" \
    --format "$FORMAT" \
    --deepseek \
    --out-dir "$OUTPUT_DIR"

echo "Experiment: $OUTPUT_DIR"
if command -v jq >/dev/null 2>&1; then
    jq '{score, max_score, warning_codes, failed_checks: [.checks[] | select(.passed == false) | .name]}' \
        "$OUTPUT_DIR/symbol_evaluation.json"
fi
