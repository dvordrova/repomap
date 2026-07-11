#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

EXPERIMENT_DIR="${1:-}"
if [ -z "$EXPERIMENT_DIR" ]; then
    echo "Usage: $0 EXPERIMENT_DIR" >&2
    exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
    echo "jq is required" >&2
    exit 2
fi

required=(
    manifest.json
    facts.json
    actions.json
    final_result.json
    final_response.raw.json
    metrics.json
    protocol_evaluation.json
    symbol_report.json
    symbol_evaluation.json
    stage-01-triage/schema.json
    stage-01-triage/normalized.json
    stage-02-hypothesis/schema.json
    stage-02-hypothesis/normalized.json
    stage-03-action/schema.json
    stage-03-action/normalized.json
)
for relative_path in "${required[@]}"; do
    path="$EXPERIMENT_DIR/$relative_path"
    if [ ! -f "$path" ]; then
        echo "Missing staged artifact: $path" >&2
        exit 1
    fi
    jq -e . "$path" >/dev/null
done

jq -e '.protocol_version == "local-symbol-v2" and .schema_version == 2 and .reducer_version == 2' \
    "$EXPERIMENT_DIR/manifest.json" >/dev/null
jq -e '.model_free_text_fields == 0 and .next_action.operation == "read_target"' \
    "$EXPERIMENT_DIR/final_result.json" >/dev/null
jq -e '.passed == .total and .total > 0' \
    "$EXPERIMENT_DIR/protocol_evaluation.json" >/dev/null
jq -e '.score == .max_score and .max_score == 100' \
    "$EXPERIMENT_DIR/symbol_evaluation.json" >/dev/null
jq -e '.files_to_read_in_order[0].structural_role == "target"' \
    "$EXPERIMENT_DIR/symbol_report.json" >/dev/null
jq -e '.totals.model_calls >= 2 and .totals.prompt_tokens > 0 and .totals.output_tokens > 0' \
    "$EXPERIMENT_DIR/metrics.json" >/dev/null

if rg -i -l 'authorization|bearer[[:space:]]+[a-z0-9]|api[_-]?key|password|secret' \
    "$EXPERIMENT_DIR"/*/request.json >/dev/null 2>&1; then
    echo "Staged request artifact contains possible authorization material" >&2
    exit 1
fi

echo "OK: $EXPERIMENT_DIR"
jq -s '{
  target: .[0].target.name,
  role: .[0].hypothesis.role,
  evidence_ids: .[0].hypothesis.evidence_ids,
  next_action: .[0].next_action.operation,
  protocol_checks: ((.[1].passed | tostring) + "/" + (.[1].total | tostring)),
  total_seconds: .[2].totals.total_seconds,
  model_calls: .[2].totals.model_calls,
  prompt_tokens: .[2].totals.prompt_tokens,
  output_tokens: .[2].totals.output_tokens
}' \
    "$EXPERIMENT_DIR/final_result.json" \
    "$EXPERIMENT_DIR/protocol_evaluation.json" \
    "$EXPERIMENT_DIR/metrics.json"
