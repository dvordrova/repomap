#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TARGET_REPO="${1:-../etcd}"
TARGET_SYMBOL="${2:-kvServer.Put}"
OUTPUT_DIR="${3:-tmp/source-examples/etcd-put}"

if [ ! -d "$TARGET_REPO/.git" ]; then
    echo "Skipping source_artifacts_check: $TARGET_REPO is not a git repo"
    echo "  Usage: $0 [path-to-repo] [exact-symbol] [output-dir]"
    exit 0
fi

go run ./cmd/symbol-playground \
    --repo "$TARGET_REPO" \
    --symbol "$TARGET_SYMBOL" \
    --format json \
    --source \
    --out-dir "$OUTPUT_DIR"

if command -v jq >/dev/null 2>&1; then
    jq -e --slurpfile bundle "$OUTPUT_DIR/symbol_bundle.json" '
        .version == 1 and
        .language == "go" and
        .target.name == $bundle[0].target.entity.name and
        .target.path == $bundle[0].target.entity.location.path and
        .target.line == $bundle[0].target.entity.location.line and
        (.target.path | startswith("/") | not) and
        .window.start_line == .target.line and
        (.lines | length > 0) and
        ((.lines | map(.evidence_id) | unique | length) == (.lines | length))
    ' "$OUTPUT_DIR/source_card.json" >/dev/null
    jq -e --slurpfile card "$OUTPUT_DIR/source_card.json" '
        .version == 1 and
        .target == $card[0].target and
        (.questions | length > 0) and
        (.questions | all(. as $question | ($question.candidate_source_evidence_ids | index($question.anchor_source_evidence_id)) != null)) and
        (([.questions[].candidate_source_evidence_ids[]] - [$card[0].lines[].evidence_id]) | length == 0) and
        .allowed_actions[0].operation == "find_tests"
    ' "$OUTPUT_DIR/source_assessment_bundle.json" >/dev/null
    jq -e '(.messages | length == 2) and (.response_format.type == "json_object")' \
        "$OUTPUT_DIR/deepseek_source_request.redacted.json" >/dev/null

    if [ "$TARGET_SYMBOL" = "kvServer.Put" ]; then
        jq -e '
            [.questions[] | {predicate, anchor_source_evidence_id}] == [
                {"predicate":"validates_input","anchor_source_evidence_id":"source-91"},
                {"predicate":"delegates_operation","anchor_source_evidence_id":"source-95"},
                {"predicate":"maps_error","anchor_source_evidence_id":"source-97"},
                {"predicate":"fills_response","anchor_source_evidence_id":"source-100"}
            ]
        ' "$OUTPUT_DIR/source_assessment_bundle.json" >/dev/null
    fi
fi

if grep -Eiq 'authorization|bearer[[:space:]]+[a-z0-9]' \
    "$OUTPUT_DIR/deepseek_source_request.redacted.json" \
    "$OUTPUT_DIR/source_card.json"; then
    echo "FAIL: source artifact contains authorization material" >&2
    exit 1
fi

echo "OK: inspect $OUTPUT_DIR/source_assessment_bundle.json"
