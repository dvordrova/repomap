#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

REPO="${1:-../etcd}"

if [ ! -d "$REPO" ] || [ ! -d "$REPO/.git" ]; then
    echo "Skipping: $REPO is not a git repo"
    echo "  Usage: $0 [path-to-repo]"
    exit 0
fi

echo "=== Capturing etcd LLM bundle ($REPO) ==="
go run ./cmd/repomap orient --repo "$REPO" --llm-bundle-only --debug-dir .repomap-runs > /dev/null

echo ""
LAST_RUN=$(find .repomap-runs -maxdepth 1 -type d -not -name .repomap-runs | sort -r | head -1)
if [ -n "$LAST_RUN" ]; then
    echo "Debug dir: $LAST_RUN"
    echo "Files:"
    find "$LAST_RUN" -type f | sed "s|$LAST_RUN/|  |" | sort
else
    echo "WARNING: no debug directory created"
    exit 1
fi

echo ""
echo "OK"
