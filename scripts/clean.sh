#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "Cleaning..."
rm -rf tmp/ .repomap-runs/ .bin/
echo "OK"
