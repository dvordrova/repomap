#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 ORIENTATION_REQUEST_JSON SOURCE_REQUEST_JSON" >&2
    exit 2
fi

for command in awk dd jq mktemp od rm tail tr wc; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "missing required command: $command" >&2
        exit 2
    fi
done
if command -v shasum >/dev/null 2>&1; then
    SHA256_COMMAND=shasum
elif command -v sha256sum >/dev/null 2>&1; then
    SHA256_COMMAND=sha256sum
else
    echo "missing required command: shasum or sha256sum" >&2
    exit 2
fi

ORIENTATION_REQUEST="$1"
SOURCE_REQUEST="$2"
for path in "$ORIENTATION_REQUEST" "$SOURCE_REQUEST"; do
    if [ ! -f "$path" ]; then
        echo "request artifact does not exist: $path" >&2
        exit 2
    fi
done

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/repomap-quality-capture.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

strip_one_terminal_newline() {
    local source="$1"
    local destination="$2"
    local size last_byte

    size="$(wc -c < "$source" | tr -d ' ')"
    if [ "$size" -eq 0 ]; then
        echo "request artifact is empty: $source" >&2
        exit 2
    fi
    last_byte="$(tail -c 1 "$source" | od -An -tuC | tr -d '[:space:]')"
    if [ "$last_byte" = "10" ]; then
        dd if="$source" of="$destination" bs=1 count="$((size - 1))" 2>/dev/null
    else
        dd if="$source" of="$destination" bs=1 count="$size" 2>/dev/null
    fi
}

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

ORIENTATION_EXACT="$TMP_DIR/orientation-request.json"
SOURCE_EXACT="$TMP_DIR/source-request.json"
ORIENTATION_CONTEXT="$TMP_DIR/orientation-context.json"
SOURCE_CONTEXT="$TMP_DIR/source-context.json"

strip_one_terminal_newline "$ORIENTATION_REQUEST" "$ORIENTATION_EXACT"
strip_one_terminal_newline "$SOURCE_REQUEST" "$SOURCE_EXACT"
extract_context "$ORIENTATION_EXACT" $'Facts bundle JSON:\n' "$ORIENTATION_CONTEXT"
extract_context "$SOURCE_EXACT" $'SOURCE ASSESSMENT BUNDLE:\n' "$SOURCE_CONTEXT"

file_bytes() {
    wc -c < "$1" | tr -d ' '
}

file_sha256() {
    if [ "$SHA256_COMMAND" = shasum ]; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        sha256sum "$1" | awk '{print $1}'
    fi
}

jq -n \
    --arg orientation_model "$(jq -er '.model' "$ORIENTATION_EXACT")" \
    --arg orientation_context_sha256 "$(file_sha256 "$ORIENTATION_CONTEXT")" \
    --argjson orientation_context_bytes "$(file_bytes "$ORIENTATION_CONTEXT")" \
    --arg orientation_request_sha256 "$(file_sha256 "$ORIENTATION_EXACT")" \
    --argjson orientation_request_bytes "$(file_bytes "$ORIENTATION_EXACT")" \
    --arg source_model "$(jq -er '.model' "$SOURCE_EXACT")" \
    --arg source_context_sha256 "$(file_sha256 "$SOURCE_CONTEXT")" \
    --argjson source_context_bytes "$(file_bytes "$SOURCE_CONTEXT")" \
    --arg source_request_sha256 "$(file_sha256 "$SOURCE_EXACT")" \
    --argjson source_request_bytes "$(file_bytes "$SOURCE_EXACT")" \
    '{
        orientation: {
            model: $orientation_model,
            model_context_sha256: $orientation_context_sha256,
            model_context_bytes: $orientation_context_bytes,
            provider_request_sha256: $orientation_request_sha256,
            provider_request_bytes: $orientation_request_bytes
        },
        source: {
            model: $source_model,
            model_context_sha256: $source_context_sha256,
            model_context_bytes: $source_context_bytes,
            provider_request_sha256: $source_request_sha256,
            provider_request_bytes: $source_request_bytes
        }
    }'
