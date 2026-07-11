#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ETCD_REPO="${1:-../etcd}"

if [ ! -d "$ETCD_REPO" ] || [ ! -d "$ETCD_REPO/.git" ]; then
    echo "Skipping etcd_check: $ETCD_REPO is not a git repo"
    echo "  Usage: $0 [path-to-etcd]"
    exit 0
fi

mkdir -p tmp

echo "=== snapshot-only ($ETCD_REPO) ==="
go run ./cmd/repomap orient --repo "$ETCD_REPO" --snapshot-only > tmp/etcd-snapshot.json
echo "  wrote tmp/etcd-snapshot.json"

echo ""
echo "=== llm-bundle-only ($ETCD_REPO) ==="
go run ./cmd/repomap orient --repo "$ETCD_REPO" --llm-bundle-only > tmp/etcd-llm-bundle.json
echo "  wrote tmp/etcd-llm-bundle.json"

echo ""
if command -v jq &>/dev/null; then
    echo "--- Validating with jq ---"
    FAIL=0

    if jq -e '.go_facts.packages_count > 0' tmp/etcd-snapshot.json > /dev/null; then
        echo "OK: snapshot packages_count > 0"
    else
        echo "FAIL: snapshot packages_count <= 0"
        FAIL=1
    fi

    if jq -e '.go_facts.orientation_candidates | length > 0' tmp/etcd-snapshot.json > /dev/null; then
        echo "OK: snapshot has orientation_candidates"
    else
        echo "FAIL: snapshot has no orientation_candidates"
        FAIL=1
    fi

    if jq -e '.go.entrypoints | length > 0' tmp/etcd-llm-bundle.json > /dev/null; then
        echo "OK: bundle has entrypoints"
    else
        echo "FAIL: bundle has no entrypoints"
        FAIL=1
    fi

    if jq -re '.go.orientation_candidates[]?.open_files[]?' tmp/etcd-snapshot.json 2>/dev/null | grep -q 'server/main.go'; then
        echo "OK: snapshot contains server/main.go"
    fi
    if jq -re '.go.entrypoints[]?.open_files[]?' tmp/etcd-llm-bundle.json 2>/dev/null | grep -q 'main.go'; then
        echo "OK: bundle open_files present"
    fi

    if [ $FAIL -ne 0 ]; then
        echo "FAIL: validation errors above"
        exit 1
    fi
else
    echo "(jq not found; skipping JSON validation)"
fi

echo ""
echo "OK"
