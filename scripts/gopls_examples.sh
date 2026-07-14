#!/usr/bin/env bash
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

FETCH=0
if [ "${1:-}" = "--fetch" ]; then
    FETCH=1
elif [ "$#" -gt 0 ]; then
    echo "Usage: $0 [--fetch]" >&2
    exit 2
fi

if ! command -v gopls >/dev/null 2>&1; then
    echo "gopls is required; install it with: go install golang.org/x/tools/gopls@latest" >&2
    exit 1
fi

BIN_DIR="${BIN_DIR:-.bin}"
CHECKOUT_DIR="${GOPLS_EXAMPLE_REPOS:-tmp/example-repos}"
OUTPUT_DIR="${GOPLS_EXAMPLE_OUTPUT:-tmp/evidence-examples}"
COMMAND_TIMEOUT="${GOPLS_COMMAND_TIMEOUT:-2m}"
MAX_SYMBOLS="${GOPLS_MAX_SYMBOLS:-25}"
MAX_CALL_ROOTS="${GOPLS_MAX_CALL_ROOTS:-2}"
mkdir -p "$BIN_DIR" "$CHECKOUT_DIR" "$OUTPUT_DIR"

echo "=== Building gopls playground ==="
go build -o "$BIN_DIR/gopls-playground" ./cmd/gopls-playground

FAIL=0
RAN=0
while IFS='|' read -r name url query; do
    repo="$CHECKOUT_DIR/$name"
    if [ "$name" = "etcd" ] && [ -d "../etcd/.git" ]; then
        repo="../etcd"
    fi

    if [ ! -d "$repo/.git" ]; then
        if [ "$FETCH" -eq 0 ]; then
            echo "SKIP $name: clone not found at $repo (use --fetch)"
            continue
        fi
        echo "=== Cloning $name ==="
        if ! git clone --depth=1 "$url" "$repo"; then
            echo "FAIL $name: clone failed" >&2
            FAIL=1
            continue
        fi
    fi

    echo "=== Analyzing $name (query: $query) ==="
    if "$BIN_DIR/gopls-playground" \
        --repo "$repo" \
        --query "$query" \
        --out "$OUTPUT_DIR/$name.json" \
        --summary-out "$OUTPUT_DIR/$name.md" \
        --max-symbols "$MAX_SYMBOLS" \
        --max-call-roots "$MAX_CALL_ROOTS" \
        --command-timeout "$COMMAND_TIMEOUT"; then
        RAN=$((RAN + 1))
    else
        echo "FAIL $name: analysis failed" >&2
        FAIL=1
    fi
done <<'REPOS'
etcd|https://github.com/etcd-io/etcd.git|kvServer.Put
k6|https://github.com/grafana/k6.git|Scheduler.Run
prometheus|https://github.com/prometheus/prometheus.git|NewInstantQuery
nats-server|https://github.com/nats-io/nats-server.git|processInboundMsg
golangci-lint|https://github.com/golangci/golangci-lint.git|runAnalysis
REPOS

echo "=== Complete: $RAN repositories analyzed; artifacts in $OUTPUT_DIR ==="
exit "$FAIL"
