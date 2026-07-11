#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

go test ./cmd/repomap -run '^TestRunDefaultCompletesOneRequestOrientationJourney$' -count=1
go test ./internal/orient -run '^(TestRunWritesLocalEvidenceForEveryDirectionWithoutExtraModelCalls|TestRunCountsExpandedFlowRequestBodyAndPersistsStatus)$' -count=1

OUT_DIR=${1:-tmp/friend-handoff}
mkdir -p "$OUT_DIR"
go build -o "$OUT_DIR/repomap" ./cmd/repomap

version=$("$OUT_DIR/repomap" --version)
case "$version" in
	repomap*) ;;
	*)
		echo "unexpected version output: $version" >&2
		exit 1
		;;
esac

"$OUT_DIR/repomap" --help >/dev/null 2>&1

echo "friend handoff check passed"
echo "binary: $OUT_DIR/repomap"
