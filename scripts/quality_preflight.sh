#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [ "$#" -ne 3 ]; then
    echo "Usage: $0 LABEL REPOSITORY EXACT_SYMBOL" >&2
    exit 2
fi

LABEL="$1"
TARGET_REPO="$2"
TARGET_SYMBOL="$3"

if [[ ! "$LABEL" =~ ^[a-zA-Z0-9._-]+$ ]]; then
    echo "LABEL may contain only letters, digits, dots, underscores, and dashes" >&2
    exit 2
fi
if ! git -C "$TARGET_REPO" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "$TARGET_REPO is not a git repository" >&2
    exit 2
fi
if [ -n "$(git -C "$TARGET_REPO" status --porcelain --untracked-files=normal)" ]; then
    echo "$TARGET_REPO must be clean before a quality preflight" >&2
    exit 2
fi

for command in awk git go gopls jq wc; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "missing required command: $command" >&2
        exit 2
    fi
done

OUTPUT_BASE="${QUALITY_PREFLIGHT_DIR:-tmp/quality-preflight}"
OUTPUT_DIR="$OUTPUT_BASE/$LABEL"
if [ -e "$OUTPUT_DIR" ]; then
    echo "preflight output already exists: $OUTPUT_DIR" >&2
    exit 2
fi
mkdir -p "$OUTPUT_DIR/.bin" "$OUTPUT_DIR/source"

TARGET_REPO="$(cd "$TARGET_REPO" && pwd -P)"
REVISION="$(git -C "$TARGET_REPO" rev-parse HEAD)"
GOOS_VALUE="$(go env GOOS)"
GOARCH_VALUE="$(go env GOARCH)"
GO_VERSION="$(go version | awk '{print $3}')"
GOPLS_VERSION="$(gopls version | awk 'NR == 1 {print $2}')"

REPOMAP_BIN="$OUTPUT_DIR/.bin/repomap"
SYMBOL_BIN="$OUTPUT_DIR/.bin/symbol-playground"
go build -o "$REPOMAP_BIN" ./cmd/repomap
go build -o "$SYMBOL_BIN" ./cmd/symbol-playground

ORIENTATION_REQUEST="$OUTPUT_DIR/orientation_request.json"
SOURCE_REQUEST="$OUTPUT_DIR/source/deepseek_source_request.redacted.json"

"$REPOMAP_BIN" "$TARGET_REPO" \
    --preview-request \
    --out "$ORIENTATION_REQUEST"

"$SYMBOL_BIN" \
    --repo "$TARGET_REPO" \
    --symbol "$TARGET_SYMBOL" \
    --format json \
    --source \
    --out-dir "$OUTPUT_DIR/source"

ORIENTATION_PROMPT_VERSION="$(
    "$REPOMAP_BIN" dev prompt-versions | jq -er '.orientation_json'
)"

extract_context() {
    local request="$1"
    local marker="$2"
    local destination="$3"

    jq -erj --arg marker "$marker" '
        ([.messages[] | select(.role == "user") | .content]
            | if length == 1 then .[0] else error("expected exactly one user message") end) as $content
        | ($content | index($marker)) as $offset
        | if $offset == null
          then error("model context marker is missing")
          else $content[($offset + ($marker | length)):]
          end
    ' "$request" > "$destination"
    jq -e . "$destination" >/dev/null
}

ORIENTATION_MODEL_CONTEXT="$OUTPUT_DIR/orientation_model_context.json"
SOURCE_MODEL_CONTEXT="$OUTPUT_DIR/source_model_context.json"
extract_context "$ORIENTATION_REQUEST" $'Facts bundle JSON:\n' "$ORIENTATION_MODEL_CONTEXT"
extract_context "$SOURCE_REQUEST" $'SOURCE ASSESSMENT BUNDLE:\n' "$SOURCE_MODEL_CONTEXT"

if ! jq -e '
    .allowed_paths as $allowed
    | [
        .known_docs[]?,
        .source_signals[]?.path,
        .go.entrypoints[]?.open_files[]?,
        .go.orientation_candidates[]?.open_files[]?
    ] as $visible
    | [$visible[] as $path | select(($allowed | index($path)) == null) | $path]
    | length == 0
' "$ORIENTATION_MODEL_CONTEXT" >/dev/null; then
    echo "quality preflight rejected an incoherent orientation bundle" >&2
    echo "a model-visible file path is absent from allowed_paths" >&2
    exit 1
fi

jq -S '{
    version: 1,
    repo_name: .repo_name,
    allowed_paths: .allowed_paths
}' "$ORIENTATION_MODEL_CONTEXT" > "$OUTPUT_DIR/orientation_context.json"

TARGET_PATH="$(jq -er '.target.path' "$OUTPUT_DIR/source/source_assessment_bundle.json")"
if ! jq -e --arg path "$TARGET_PATH" '.allowed_paths | index($path) != null' \
    "$ORIENTATION_MODEL_CONTEXT" >/dev/null; then
    echo "quality preflight rejected an unlinked drill-down" >&2
    echo "symbol path $TARGET_PATH is absent from the bounded orientation context" >&2
    exit 1
fi

./scripts/quality_capture_meta.sh \
    "$ORIENTATION_REQUEST" \
    "$SOURCE_REQUEST" > "$OUTPUT_DIR/request_metadata.json"

ORIENTATION_MODEL="$(jq -er '.model' "$ORIENTATION_REQUEST")"
SOURCE_MODEL="$(jq -er '.model' "$SOURCE_REQUEST")"
if [ "$ORIENTATION_MODEL" != "deepseek-v4-flash" ] || [ "$SOURCE_MODEL" != "deepseek-v4-flash" ]; then
    echo "M3 reference captures require deepseek-v4-flash" >&2
    echo "configured models: orientation=$ORIENTATION_MODEL source=$SOURCE_MODEL" >&2
    exit 1
fi

jq -n \
    --arg label "$LABEL" \
    --arg repo_path "$TARGET_REPO" \
    --arg revision "$REVISION" \
    --arg goos "$GOOS_VALUE" \
    --arg goarch "$GOARCH_VALUE" \
    --arg go_version "$GO_VERSION" \
    --arg gopls_version "$GOPLS_VERSION" \
    --arg symbol "$TARGET_SYMBOL" \
    --arg target_path "$TARGET_PATH" \
    --arg orientation_prompt_version "$ORIENTATION_PROMPT_VERSION" \
    --slurpfile requests "$OUTPUT_DIR/request_metadata.json" \
    --slurpfile source_experiment "$OUTPUT_DIR/source/prompt_experiment.json" \
    '{
        version: 1,
        label: $label,
        repository: {
            path: $repo_path,
            revision: $revision,
            clean: true,
            goos: $goos,
            goarch: $goarch,
            go_version: $go_version,
            gopls_version: $gopls_version,
            build_tags: []
        },
        drilldown: {
            symbol: $symbol,
            path: $target_path,
            present_in_orientation_context: true
        },
        prompt_versions: {
            orientation: $orientation_prompt_version,
            source: $source_experiment[0].source_prompt_version
        },
        requests: $requests[0]
    }' > "$OUTPUT_DIR/preflight.json"

if [ "$(git -C "$TARGET_REPO" rev-parse HEAD)" != "$REVISION" ] || \
    [ -n "$(git -C "$TARGET_REPO" status --porcelain --untracked-files=normal)" ]; then
    echo "$TARGET_REPO changed during quality preflight" >&2
    exit 1
fi

echo "Quality preflight: $OUTPUT_DIR"
jq '{repository, drilldown, prompt_versions, requests}' "$OUTPUT_DIR/preflight.json"
