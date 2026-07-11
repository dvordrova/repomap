#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "=== go test ==="
go test ./...

echo ""
echo "=== go vet ==="
go vet ./...

echo ""
echo "OK"
