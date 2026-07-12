#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

RUN_DIR=${1:-}
if [[ -z "$RUN_DIR" ]]; then
	echo "Usage: $0 RUN_DIR" >&2
	exit 2
fi

for required in metadata.json orientation_report.json report.json report.html run_manifest.json onboarding-feedback.md; do
	if [[ ! -f "$RUN_DIR/$required" ]]; then
		echo "missing friend artifact: $RUN_DIR/$required" >&2
		exit 1
	fi
done

jq -e '
	.version == 2
	and .repository_state.version == 1
	and (.repository_state.identity | type == "string" and length > 0)
	and (.analysis_root | type == "string" and startswith("/"))
	and (.repository_state_sha256 | test("^[0-9a-f]{64}$"))
	and (.report_sha256 | test("^[0-9a-f]{64}$"))
	and .report_format_version >= 8
	and (.openable_paths | length) > 0
	and (.components | length) > 0
	and ([.components[].anchors[]? | select(.can_list_symbols == true)] | length) > 0
' "$RUN_DIR/run_manifest.json" >/dev/null

jq -e '
  .format_version >= 7
	and (.candidate_directions | length) > 0
	and (.flows | length) == (.candidate_directions | length)
	and .run.compact_context_bytes > 0
	and .run.external_request_bytes > 0
	and .run.provider_request_count >= 1
	and ([.flows[] | select(.flow_status != "local_only" or .evidence_only != true)] | length) == 0
	and ([.flows[] | (.bundle_summary.selected_files_count + .bundle_summary.selected_tests_count + .bundle_summary.selected_docs_count) > 20] | any | not)
' "$RUN_DIR/report.json" >/dev/null

for bundle in "$RUN_DIR"/flows/*/flow_bundle.json; do
	jq -e '
		.flow_seed.valid_seed_files as $seeds
		| ([.selected_files[]?.path, .selected_tests[]?.path, .selected_docs[]?.path]) as $selected
		| ($seeds | length) > 0
		and (($selected | length) <= 20)
		and (all($seeds[]; $selected | index(.) != null))
	' "$bundle" >/dev/null
done

for status_file in "$RUN_DIR"/flows/*/flow_status.json; do
	jq -e '.version == 1 and .mode == "local_only"' "$status_file" >/dev/null
done

for heading in "## Correct" "## Missing" "## Misleading"; do
	rg -F --quiet "$heading" "$RUN_DIR/onboarding-feedback.md"
done

if rg -i -l 'authorization:[[:space:]]*bearer|bearer[[:space:]]+[a-z0-9_-]{12,}|sk-[a-z0-9]{12,}' "$RUN_DIR" >/dev/null 2>&1; then
	echo "friend artifacts contain possible authorization material" >&2
	exit 1
fi

echo "OK: friend onboarding artifacts at $RUN_DIR"
