#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

LABEL="${1:-}"
TARGET_REPO="${2:-../etcd}"
TARGET_SYMBOL="${3:-kvServer.Put}"

if [ -z "$LABEL" ]; then
    echo "Usage: $0 LABEL [repo] [exact-symbol]" >&2
    exit 2
fi
if [ ! -d "$TARGET_REPO/.git" ]; then
    echo "$TARGET_REPO is not a git repository" >&2
    exit 2
fi

SAFE_LABEL="${LABEL//[^a-zA-Z0-9._-]/-}"
OUTPUT_DIR="tmp/source-prompt-experiments/${SAFE_LABEL}"

go run ./cmd/symbol-playground \
    --repo "$TARGET_REPO" \
    --symbol "$TARGET_SYMBOL" \
    --format json \
    --deepseek-source \
    --out-dir "$OUTPUT_DIR"

echo "Experiment: $OUTPUT_DIR"
if command -v jq >/dev/null 2>&1; then
    jq '{
        score,
        max_score,
        warning_codes,
        failed_checks: [.checks[] | select(.passed == false) | .name]
    }' "$OUTPUT_DIR/source_evaluation.json"
    jq '{
        claims: [.claims[] | {predicate, source_evidence_ids}],
        unknowns,
        next_action
    }' "$OUTPUT_DIR/source_report.json"
    if [ -f "$OUTPUT_DIR/test_evidence.json" ]; then
        jq '{
            test_references: [.references[] | {
                predicate,
                path,
                line,
                kind,
                certainty,
                provenance,
                scenarios
            }],
            warnings
        }' "$OUTPUT_DIR/test_evidence.json"
    fi
fi
