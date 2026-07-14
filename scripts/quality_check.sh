#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [ "$#" -gt 2 ]; then
    echo "Usage: $0 [task.json [result.json]]" >&2
    exit 2
fi

QUALITY_DIR="tmp/quality"
EVALUATOR="$QUALITY_DIR/.bin/quality-evaluate"
mkdir -p "$(dirname "$EVALUATOR")"
go build -o "$EVALUATOR" ./cmd/quality-evaluate

run_task() {
    local task="$1"
    local output="$2"

    "$EVALUATOR" --task "$task" --out "$output"
}

if [ "$#" -gt 0 ]; then
    TASK="$1"
    TASK_DIR="$(basename "$(dirname "$TASK")")"
    OUTPUT="${2:-$QUALITY_DIR/$TASK_DIR-result.json}"
    run_task "$TASK" "$OUTPUT"
    echo "OK: offline quality replay is in $OUTPUT"
    exit 0
fi

TASKS=(internal/quality/testdata/*/task.json)
if [ "${#TASKS[@]}" -eq 1 ] && [ ! -f "${TASKS[0]}" ]; then
    echo "FAIL: no committed quality tasks found" >&2
    exit 1
fi

for TASK in "${TASKS[@]}"; do
    TASK_DIR="$(basename "$(dirname "$TASK")")"
    run_task "$TASK" "$QUALITY_DIR/$TASK_DIR-result.json"
done

echo "OK: replayed ${#TASKS[@]} committed quality task(s) in $QUALITY_DIR"
