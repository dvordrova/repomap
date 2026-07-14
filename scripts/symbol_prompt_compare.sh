#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

LEFT="${1:-}"
RIGHT="${2:-}"
if [ -z "$LEFT" ] || [ -z "$RIGHT" ]; then
    echo "Usage: $0 LEFT_EXPERIMENT_DIR RIGHT_EXPERIMENT_DIR" >&2
    exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
    echo "jq is required to compare prompt experiments" >&2
    exit 2
fi

for directory in "$LEFT" "$RIGHT"; do
    if [ ! -f "$directory/symbol_evaluation.json" ] || [ ! -f "$directory/prompt_experiment.json" ]; then
        echo "Missing experiment artifacts in $directory" >&2
        exit 2
    fi
done

jq -n \
    --slurpfile left_meta "$LEFT/prompt_experiment.json" \
    --slurpfile left_eval "$LEFT/symbol_evaluation.json" \
    --slurpfile right_meta "$RIGHT/prompt_experiment.json" \
    --slurpfile right_eval "$RIGHT/symbol_evaluation.json" \
    '{
      left: ($left_meta[0] + {
        score: $left_eval[0].score,
        warning_codes: $left_eval[0].warning_codes,
        failed_checks: [$left_eval[0].checks[] | select(.passed == false) | .name]
      }),
      right: ($right_meta[0] + {
        score: $right_eval[0].score,
        warning_codes: $right_eval[0].warning_codes,
        failed_checks: [$right_eval[0].checks[] | select(.passed == false) | .name]
      }),
      score_delta_right_minus_left: ($right_eval[0].score - $left_eval[0].score)
    }'
