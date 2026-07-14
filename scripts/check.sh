#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "=== shell syntax ==="
for script in scripts/*.sh; do
    bash -n "$script"
done

echo ""
echo "=== go test ==="
go test ./...

echo ""
echo "=== go vet ==="
go vet ./...

echo ""
echo "=== offline quality replay ==="
./scripts/quality_check.sh

echo ""
echo "OK"
