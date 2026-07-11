#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TARGET_REPO="${1:-../etcd}"
TARGET_SYMBOL="${2:-kvServer.Put}"
OUTPUT_ROOT="${3:-tmp/investigation-handoff-check}"

if [ ! -d "$TARGET_REPO/.git" ]; then
    echo "Skipping investigation_handoff_check: $TARGET_REPO is not a git repo"
    echo "  Usage: $0 [repo] [exact-symbol] [output-root]"
    exit 0
fi
if ! command -v jq >/dev/null 2>&1; then
    echo "Skipping investigation_handoff_check: jq is required to build and verify the fixture"
    exit 0
fi

REPO_NAME="$(basename "$(cd "$TARGET_REPO" && pwd)")"
EXPECTED_REVISION="$(git -C "$TARGET_REPO" rev-parse HEAD)"
if [ -n "$(git -C "$TARGET_REPO" status --porcelain=v1 --untracked-files=all)" ]; then
    EXPECTED_REVISION="${EXPECTED_REVISION}-dirty"
fi
REPORT_PATH="$OUTPUT_ROOT/orientation_fixture.json"
HANDOFF_DIR="$OUTPUT_ROOT/handoff"
RESUME_DIR="$OUTPUT_ROOT/resumed"
SESSION_PATH="$OUTPUT_ROOT/handoff_session.json"
mkdir -p "$OUTPUT_ROOT"

jq -n --arg repo "$REPO_NAME" '{
    repo_name: $repo,
    explained_flows: [{
        flow_seed: {
            id: "investigation-handoff",
            name: "Investigation handoff",
            likely_entrypoint: "must.not.become.Symbol"
        }
    }]
}' >"$REPORT_PATH"

go run ./cmd/investigation-playground \
    --repo "$TARGET_REPO" \
    --orientation-json "$REPORT_PATH" \
    --flow-id investigation-handoff \
    --symbol "$TARGET_SYMBOL" \
    --out-dir "$HANDOFF_DIR"

jq --arg revision "$EXPECTED_REVISION" -e '
    .state == "assessing_source" and
    .origin.kind == "orientation_flow" and
    .origin.status == "candidate" and
    .origin.flow_id == "investigation-handoff" and
    .origin.flow_name == "Investigation handoff" and
    .origin.accepted_revision == $revision and
    (.origin.report_sha256 | test("^[0-9a-f]{64}$")) and
    (.origin | tostring | contains("must.not.become.Symbol") | not)
' "$HANDOFF_DIR/investigation_session.json" >/dev/null

cp "$HANDOFF_DIR/investigation_session.json" "$SESSION_PATH"
go run ./cmd/investigation-playground \
    --resume "$SESSION_PATH" \
    --out-dir "$RESUME_DIR"

jq -e '
    .state == "assessing_source" and
    .next[0].kind == "assess_source" and
    .origin.flow_id == "investigation-handoff"
' "$RESUME_DIR/investigation_session.json" >/dev/null

if grep -ERiq 'authorization|bearer[[:space:]]+[a-z0-9]|sk-[a-z0-9_-]{16,}' "$OUTPUT_ROOT"; then
    echo "FAIL: handoff artifacts contain authorization material" >&2
    exit 1
fi

echo "OK: orientation handoff and local resume are replayable under $OUTPUT_ROOT"
