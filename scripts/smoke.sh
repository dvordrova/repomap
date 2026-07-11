#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

SMOKE_DIR="tmp/smoke-repo"

echo "=== Setting up smoke repo ==="
rm -rf "$SMOKE_DIR"
mkdir -p "$SMOKE_DIR"
cd "$SMOKE_DIR"
git init
echo 'module example.com/smoke' > go.mod
echo 'go 1.22' >> go.mod
mkdir -p cmd/app
echo 'package main'    > cmd/app/main.go
echo 'import "os"'    >> cmd/app/main.go
echo 'func main() { os.Exit(0) }' >> cmd/app/main.go
git add -A
cd "$(dirname "$0")/.."

echo ""
echo "=== snapshot-only ==="
go run ./cmd/repomap orient --repo "$SMOKE_DIR" --snapshot-only

echo ""
echo "=== llm-bundle-only ==="
go run ./cmd/repomap orient --repo "$SMOKE_DIR" --llm-bundle-only

echo ""
if command -v jq &>/dev/null; then
    echo "--- Validating with jq ---"
    if go run ./cmd/repomap orient --repo "$SMOKE_DIR" --snapshot-only | jq -e '.go_facts.packages_count > 0' > /dev/null; then
        echo "OK: go_facts present"
    else
        echo "FAIL: go_facts missing or packages_count=0"
        exit 1
    fi
    if go run ./cmd/repomap orient --repo "$SMOKE_DIR" --llm-bundle-only | jq -e '.go.entrypoints[0].open_files | length > 0' > /dev/null; then
        echo "OK: llm bundle open_files present"
    else
        echo "FAIL: llm bundle missing open_files"
        exit 1
    fi
else
    echo "(jq not found; skipping JSON validation)"
fi

echo ""
echo "OK"
