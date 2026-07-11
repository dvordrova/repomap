#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TASK="${1:-internal/quality/testdata/etcd-put-v1/task.json}"
OUTPUT="${2:-tmp/quality/etcd-put-v1-result.json}"

go run ./cmd/quality-evaluate --task "$TASK" --out "$OUTPUT"

if command -v jq >/dev/null 2>&1; then
    jq -e '
        .version == 1 and
        .passed == true and
        .direction_coverage.complete == true and
        (.direction_coverage.checks | length) == 5 and
        ([.direction_coverage.checks[] | select((.candidate_names | length) == 1)] | length) == 5 and
        .grounding.valid == true and
        .grounding.referenced_path_count == 21 and
        .grounding.unscored_prose_evidence_count == 17 and
        .important_evidence.complete == true and
        .semantic_drilldown.complete == true and
        ([.semantic_drilldown.tests[] | select(.found and .context_compatible)] | length) == 2 and
        .forbidden_phrase_tripwires.clear == true and
        .contract_adherence.orientation_response.valid == true and
        .contract_adherence.orientation_response.measured == false and
        .contract_adherence.orientation_response.clean == false and
        .contract_adherence.orientation_response.warning_codes == ["orientation.contract_unmeasured"] and
        .contract_adherence.source_response.clean == true and
        .contract_adherence.source_response.evaluation.version == 2 and
        .contract_adherence.source_response.evaluation.score == 100 and
        .contract_adherence.source_response.evaluation.max_score == 100 and
        .bytes_and_latency.orientation.replay_input_bytes == 6715 and
        .bytes_and_latency.orientation.model_context_bytes == 108668 and
        .bytes_and_latency.orientation.provider_request_bytes == null and
        .bytes_and_latency.orientation.latency_ms == null and
        .bytes_and_latency.source.replay_input_bytes == 3536 and
        .bytes_and_latency.source.model_context_bytes == 3001 and
        .bytes_and_latency.source.provider_request_bytes == 6601 and
        .bytes_and_latency.source.latency_ms == null
    ' "$OUTPUT" >/dev/null
fi

echo "OK: offline quality replay is in $OUTPUT"
