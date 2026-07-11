#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

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
cd "$REPO_ROOT"

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
echo "=== offline report generation ==="
SMOKE_RUNS="tmp/.repomap-runs-smoke"
rm -rf "$SMOKE_RUNS"
go run ./cmd/repomap "$SMOKE_DIR" --offline --debug-dir "$SMOKE_RUNS" > /dev/null 2>/dev/stderr
REPORT_HTML=$(ls -1 "$SMOKE_RUNS"/*/report.html 2>/dev/null | head -1)
if [ -n "$REPORT_HTML" ] && [ -s "$REPORT_HTML" ]; then
    echo "OK: report.html generated"
else
    echo "FAIL: report.html not found or empty"
    exit 1
fi
REPORT_JSON=$(echo "$REPORT_HTML" | sed 's/\.html$/.json/')
if [ -n "$REPORT_JSON" ] && [ -s "$REPORT_JSON" ]; then
    echo "OK: report.json generated"
else
    echo "FAIL: report.json not found or empty"
    exit 1
fi

echo ""
echo "OK"
