#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

BUNDLE="${1:-internal/deepseektest/testdata/source_bundle.json}"
RESPONSE="${2:-internal/deepseektest/testdata/source_response.json}"
OUTPUT_DIR="${3:-tmp/source-replay}"

go run ./cmd/source-evaluate \
    --bundle "$BUNDLE" \
    --response "$RESPONSE" \
    --out-dir "$OUTPUT_DIR"

if command -v jq >/dev/null 2>&1; then
    jq -e '.score == 100 and .max_score == 100' "$OUTPUT_DIR/source_evaluation.json" >/dev/null
    jq -e '
        [.claims[].predicate] == [
            "validates_input",
            "delegates_operation",
            "maps_error",
            "fills_response"
        ] and
        .next_action.operation == "find_tests"
    ' "$OUTPUT_DIR/source_report.json" >/dev/null
fi

echo "OK: inspect $OUTPUT_DIR/source_report.json"
