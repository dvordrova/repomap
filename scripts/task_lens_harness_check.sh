#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$REPO_ROOT"

python3 -B -c 'from pathlib import Path; [compile(path.read_bytes(), str(path), "exec") for path in map(Path, ("scripts/task_lens_harness.py", "scripts/task_lens_eval.py"))]'
python3 scripts/task_lens_harness.py self-test

CHECK_TMP="$(mktemp -d)"
trap 'rm -rf "$CHECK_TMP"' EXIT
GOCACHE="$CHECK_TMP/go-cache" go build -o "$CHECK_TMP/repomap" ./cmd/repomap
python3 scripts/task_lens_harness.py real-smoke --binary "$CHECK_TMP/repomap"
