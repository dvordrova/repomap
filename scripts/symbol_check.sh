#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TARGET_REPO="${1:-../etcd}"
TARGET_SYMBOL="${2:-kvServer.Put}"
OUTPUT_DIR="${3:-tmp/symbol-examples/etcd-put}"

if [ ! -d "$TARGET_REPO/.git" ]; then
    echo "Skipping symbol_check: $TARGET_REPO is not a git repo"
    echo "  Usage: $0 [path-to-repo] [exact-symbol] [output-dir]"
    exit 0
fi

go run ./cmd/symbol-playground \
    --repo "$TARGET_REPO" \
    --symbol "$TARGET_SYMBOL" \
    --format json \
    --out-dir "$OUTPUT_DIR"

if command -v jq >/dev/null 2>&1; then
    jq --arg symbol "$TARGET_SYMBOL" -e '.target.entity.name == $symbol' "$OUTPUT_DIR/symbol_bundle.json" >/dev/null
    jq -e '.messages | length == 2' "$OUTPUT_DIR/deepseek_request.redacted.json" >/dev/null
    jq -e '.response_format.type == "json_object"' "$OUTPUT_DIR/deepseek_request.redacted.json" >/dev/null
    jq -e '[.incoming_calls[], .outgoing_calls[]] | all(.certainty == "static")' "$OUTPUT_DIR/symbol_bundle.json" >/dev/null
fi

if grep -Eiq 'authorization|bearer[[:space:]]+[a-z0-9]' "$OUTPUT_DIR/deepseek_request.redacted.json"; then
    echo "FAIL: prompt artifact contains authorization material" >&2
    exit 1
fi

echo "OK: inspect $OUTPUT_DIR/deepseek_request.redacted.json"
