#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

restic_repo="${1:-../restic}"
caddy_repo="${2:-../caddy}"
for repo in "$restic_repo" "$caddy_repo"; do
    if [[ ! -d "$repo/.git" ]]; then
        printf 'SKIP: repository fixture not found: %s\n' "$repo"
        continue
    fi
done

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/repomap-research-budget.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT

go build -o "$tmp_dir/repomap" ./cmd/repomap

measure() {
    local label="$1"
    local repo="$2"
    local bundle="$tmp_dir/$label-bundle.json"
    local request="$tmp_dir/$label-request.json"

    "$tmp_dir/repomap" orient --repo "$repo" --llm-bundle-only --max-llm-files 250 >"$bundle"
    "$tmp_dir/repomap" orient --repo "$repo" --llm-request-only --max-llm-files 250 >"$request"

    printf '%s local_authorized_files=%s initial_model_summaries=%s bundle_bytes=%s request_bytes=%s\n' \
        "$label" \
        "$(jq -r '.local_authorized_file_count' "$bundle")" \
        "$(jq -r '.candidate_file_index | length' "$bundle")" \
        "$(wc -c <"$bundle" | tr -d ' ')" \
        "$(wc -c <"$request" | tr -d ' ')"
}

if [[ -d "$restic_repo/.git" ]]; then
    measure restic "$restic_repo"
fi
if [[ -d "$caddy_repo/.git" ]]; then
    measure caddy "$caddy_repo"
fi
