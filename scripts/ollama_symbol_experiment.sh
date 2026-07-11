#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

MODEL="${1:-}"
BUNDLE="${2:-}"
OUTPUT_DIR="${3:-}"
NUM_CTX="${OLLAMA_NUM_CTX:-2048}"
NUM_PREDICT="${OLLAMA_NUM_PREDICT:-320}"
NUM_THREAD="${OLLAMA_NUM_THREAD:-6}"
OLLAMA_ENDPOINT="${OLLAMA_ENDPOINT:-http://127.0.0.1:11434}"
OUTPUT_FORMAT="${OLLAMA_OUTPUT_FORMAT:-json}"

if [ -z "$MODEL" ] || [ -z "$BUNDLE" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "Usage: $0 MODEL SYMBOL_BUNDLE_JSON OUTPUT_DIR" >&2
    exit 2
fi
for command in curl jq go ollama; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "$command is required" >&2
        exit 2
    fi
done
if [ ! -f "$BUNDLE" ]; then
    echo "Bundle does not exist: $BUNDLE" >&2
    exit 2
fi
if [ "$OUTPUT_FORMAT" != "json" ] && [ "$OUTPUT_FORMAT" != "tagged" ]; then
    echo "OLLAMA_OUTPUT_FORMAT must be json or tagged" >&2
    exit 2
fi
if ! curl --silent --fail --max-time 2 "$OLLAMA_ENDPOINT/api/version" >/dev/null; then
    echo "Ollama is not reachable at $OLLAMA_ENDPOINT" >&2
    exit 2
fi
if ! ollama show "$MODEL" >/dev/null 2>&1; then
    echo "Ollama model is not installed: $MODEL" >&2
    exit 2
fi

mkdir -p "$OUTPUT_DIR"
PROMPT_FILTER="scripts/ollama_compact_symbol_prompt_json.jq"
if [ "$OUTPUT_FORMAT" = "tagged" ]; then
    PROMPT_FILTER="scripts/ollama_compact_symbol_prompt.jq"
fi
PROMPT="$(jq -r -f "$PROMPT_FILTER" "$BUNDLE")"
REQUEST_PATH="$OUTPUT_DIR/ollama_request.json"
ENVELOPE_PATH="$OUTPUT_DIR/ollama_response.envelope.json"
RAW_PATH="$OUTPUT_DIR/ollama_response.raw.txt"

jq -n \
    --arg model "$MODEL" \
    --arg prompt "$PROMPT" \
    --argjson num_ctx "$NUM_CTX" \
    --argjson num_predict "$NUM_PREDICT" \
    --argjson num_thread "$NUM_THREAD" \
    --arg output_format "$OUTPUT_FORMAT" \
    '({
      model: $model,
      stream: false,
      keep_alive: "10m",
      messages: [
        {role: "system", content: "Follow the requested evidence contract exactly."},
        {role: "user", content: $prompt}
      ],
      options: {
        num_ctx: $num_ctx,
        num_predict: $num_predict,
        num_thread: $num_thread,
        temperature: 0.1,
        top_p: 0.9
      }
    } + (if $output_format == "json" then {
      format: {
        type: "object",
        additionalProperties: false,
        required: [
          "summary", "responsibility", "read_evidence_ids", "test_evidence_ids",
          "unknowns", "next_queries", "warnings"
        ],
        properties: {
          summary: {
            type: "object",
            additionalProperties: false,
            required: ["statement", "evidence_ids", "confidence"],
            properties: {
              statement: {type: "string"},
              evidence_ids: {type: "array", maxItems: 4, items: {type: "string"}},
              confidence: {type: "number", minimum: 0, maximum: 0.75}
            }
          },
          responsibility: {
            type: "object",
            additionalProperties: false,
            required: ["statement", "evidence_ids", "confidence"],
            properties: {
              statement: {type: "string"},
              evidence_ids: {type: "array", maxItems: 4, items: {type: "string"}},
              confidence: {type: "number", minimum: 0, maximum: 0.75}
            }
          },
          read_evidence_ids: {type: "array", maxItems: 4, items: {type: "string"}},
          test_evidence_ids: {type: "array", items: {type: "string"}, maxItems: 0},
          unknowns: {type: "array", minItems: 1, maxItems: 1, items: {type: "string"}},
          next_queries: {
            type: "array",
            minItems: 1,
            maxItems: 1,
            items: {
              type: "object",
              additionalProperties: false,
              required: ["query", "reason"],
              properties: {query: {type: "string"}, reason: {type: "string"}}
            }
          },
          warnings: {type: "array", maxItems: 1, items: {type: "string"}}
        }
      }
    } else {} end))' > "$REQUEST_PATH.tmp"
mv "$REQUEST_PATH.tmp" "$REQUEST_PATH"

curl --silent --show-error --fail --max-time 300 \
    --header 'Content-Type: application/json' \
    --data-binary "@$REQUEST_PATH" \
    --output "$ENVELOPE_PATH.tmp" \
    "$OLLAMA_ENDPOINT/api/chat"
mv "$ENVELOPE_PATH.tmp" "$ENVELOPE_PATH"

jq -er '.message.content' "$ENVELOPE_PATH" > "$RAW_PATH.tmp"
mv "$RAW_PATH.tmp" "$RAW_PATH"

jq '{
  model,
  done_reason,
  total_seconds: (.total_duration / 1000000000),
  load_seconds: (.load_duration / 1000000000),
  prompt_tokens: .prompt_eval_count,
  prompt_seconds: (.prompt_eval_duration / 1000000000),
  output_tokens: .eval_count,
  output_seconds: (.eval_duration / 1000000000),
  output_tokens_per_second: (if .eval_duration > 0 then (.eval_count / (.eval_duration / 1000000000)) else 0 end)
}' "$ENVELOPE_PATH" > "$OUTPUT_DIR/ollama_metrics.json.tmp"
mv "$OUTPUT_DIR/ollama_metrics.json.tmp" "$OUTPUT_DIR/ollama_metrics.json"

go run ./cmd/symbol-evaluate \
    --bundle "$BUNDLE" \
    --response "$RAW_PATH" \
    --out-dir "$OUTPUT_DIR" >/dev/null

echo "Experiment: $OUTPUT_DIR"
wc -c "$REQUEST_PATH" "$RAW_PATH"
jq -s '.[0] + {evaluation: {score: .[1].score, max_score: .[1].max_score, warning_codes: .[1].warning_codes}}' \
    "$OUTPUT_DIR/ollama_metrics.json" "$OUTPUT_DIR/symbol_evaluation.json"
