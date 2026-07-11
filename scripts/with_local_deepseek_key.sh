#!/usr/bin/env bash
set -euo pipefail
set +x

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ "$#" -eq 0 ]; then
    echo "Usage: $0 COMMAND [ARG ...]" >&2
    exit 2
fi

# Generic provider configuration is atomic and must never inherit a DeepSeek
# key from the convenience .env file.
if [ -n "${REPOMAP_LLM_ENDPOINT+x}${REPOMAP_LLM_MODEL+x}${REPOMAP_LLM_API_KEY+x}${REPOMAP_LLM_AUTH+x}${REPOMAP_LLM_MAX_TOKENS+x}${REPOMAP_LLM_TIMEOUT+x}" ]; then
    exec "$@"
fi
if [ "${DEEPSEEK_AUTH:-bearer}" = "none" ] || [ -n "${DEEPSEEK_API_KEY:-}" ]; then
    exec "$@"
fi

ENV_FILE="$REPO_ROOT/.env"
if [ ! -f "$ENV_FILE" ]; then
    echo "DEEPSEEK_API_KEY is unset and $ENV_FILE does not exist" >&2
    exit 2
fi

KEY_COUNT="$(awk 'index($0, "DEEPSEEK_API_KEY=") == 1 { count++ } END { print count + 0 }' "$ENV_FILE")"
if [ "$KEY_COUNT" -ne 1 ]; then
    echo "$ENV_FILE must contain exactly one DEEPSEEK_API_KEY entry" >&2
    exit 2
fi

unset REPOMAP_LOCAL_DEEPSEEK_KEY
REPOMAP_LOCAL_DEEPSEEK_KEY="$(awk 'index($0, "DEEPSEEK_API_KEY=") == 1 { sub(/^DEEPSEEK_API_KEY=/, ""); print; exit }' "$ENV_FILE")"
REPOMAP_LOCAL_DEEPSEEK_KEY="${REPOMAP_LOCAL_DEEPSEEK_KEY%$'\r'}"
case "$REPOMAP_LOCAL_DEEPSEEK_KEY" in
    \"*\")
        REPOMAP_LOCAL_DEEPSEEK_KEY="${REPOMAP_LOCAL_DEEPSEEK_KEY#\"}"
        REPOMAP_LOCAL_DEEPSEEK_KEY="${REPOMAP_LOCAL_DEEPSEEK_KEY%\"}"
        ;;
    \'*\')
        REPOMAP_LOCAL_DEEPSEEK_KEY="${REPOMAP_LOCAL_DEEPSEEK_KEY#\'}"
        REPOMAP_LOCAL_DEEPSEEK_KEY="${REPOMAP_LOCAL_DEEPSEEK_KEY%\'}"
        ;;
esac
if [ -z "$REPOMAP_LOCAL_DEEPSEEK_KEY" ]; then
    echo "$ENV_FILE contains an empty DEEPSEEK_API_KEY" >&2
    exit 2
fi

export DEEPSEEK_API_KEY="$REPOMAP_LOCAL_DEEPSEEK_KEY"
unset REPOMAP_LOCAL_DEEPSEEK_KEY
exec "$@"
