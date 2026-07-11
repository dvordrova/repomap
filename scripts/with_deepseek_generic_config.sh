#!/usr/bin/env bash
set -euo pipefail
set +x

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ "$#" -eq 0 ]; then
    echo "Usage: $0 COMMAND [ARG ...]" >&2
    exit 2
fi

for name in \
    REPOMAP_LLM_ENDPOINT \
    REPOMAP_LLM_MODEL \
    REPOMAP_LLM_API_KEY \
    REPOMAP_LLM_AUTH \
    REPOMAP_LLM_MAX_TOKENS \
    REPOMAP_LLM_TIMEOUT; do
    if [ -n "${!name+x}" ]; then
        echo "$name is already set; refusing to overwrite generic provider configuration" >&2
        exit 2
    fi
done

if [ -z "${DEEPSEEK_API_KEY:-}" ]; then
    exec "$REPO_ROOT/scripts/with_local_deepseek_key.sh" "$0" "$@"
fi

export REPOMAP_LLM_ENDPOINT="https://api.deepseek.com/chat/completions"
export REPOMAP_LLM_MODEL="deepseek-v4-flash"
export REPOMAP_LLM_API_KEY="$DEEPSEEK_API_KEY"
export REPOMAP_LLM_AUTH="bearer"

unset DEEPSEEK_ENDPOINT
unset DEEPSEEK_MODEL
unset DEEPSEEK_API_KEY
unset DEEPSEEK_MAX_TOKENS
unset DEEPSEEK_TIMEOUT
unset DEEPSEEK_AUTH

exec "$@"
