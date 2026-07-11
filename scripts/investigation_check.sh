#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TARGET_REPO="${1:-../etcd}"
TARGET_SYMBOL="${2:-kvServer.Put}"
OUTPUT_DIR="${3:-tmp/investigation-check}"
MODE="${4:-local}"

if [ ! -d "$TARGET_REPO/.git" ]; then
    echo "Skipping investigation_check: $TARGET_REPO is not a git repo"
    echo "  Usage: $0 [repo] [exact-symbol] [output-dir] [local|deepseek]"
    exit 0
fi
if [ "$MODE" != "local" ] && [ "$MODE" != "deepseek" ]; then
    echo "Mode must be local or deepseek" >&2
    exit 2
fi

ARGS=(
    --repo "$TARGET_REPO"
    --symbol "$TARGET_SYMBOL"
    --out-dir "$OUTPUT_DIR"
)
EXPECTED_STATE="assessing_source"
EXPECTED_ACTION="assess_source"
if [ "$MODE" = "deepseek" ]; then
    ARGS+=(--deepseek)
    EXPECTED_STATE="waiting_user"
    EXPECTED_ACTION="await_user"
fi

go run ./cmd/investigation-playground "${ARGS[@]}"

if command -v jq >/dev/null 2>&1; then
    jq --arg state "$EXPECTED_STATE" --arg action "$EXPECTED_ACTION" -e '
        .version == 1 and
        .state == $state and
        (.next | length == 1) and
        .next[0].kind == $action and
        .symbol.target.entity.name != "" and
        .source.window.included_bytes > 0 and
        .assessment.source.complete == false
    ' "$OUTPUT_DIR/investigation_session.json" >/dev/null

    if [ "$MODE" = "deepseek" ]; then
        jq -e '.score == 100 and .max_score == 100 and (.warning_codes | length == 0)' \
            "$OUTPUT_DIR/source_evaluation.json" >/dev/null
        jq -e '
            (.references | length > 0) and
            all(.references[];
                .kind == "test_reference" and
                .certainty == "static" and
                (.provenance | length > 0) and
                (.scenarios | length > 0)
            )
        ' "$OUTPUT_DIR/test_evidence.json" >/dev/null
    fi
fi

if grep -ERiq 'authorization|bearer[[:space:]]+[a-z0-9]|sk-[a-z0-9_-]{16,}' "$OUTPUT_DIR"; then
    echo "FAIL: investigation artifacts contain authorization material" >&2
    exit 1
fi

echo "OK: inspect $OUTPUT_DIR/investigation_session.json"
