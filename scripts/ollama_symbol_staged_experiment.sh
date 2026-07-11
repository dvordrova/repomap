#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

MODEL="${1:-}"
BUNDLE="${2:-}"
OUTPUT_DIR="${3:-}"
NUM_CTX="${OLLAMA_NUM_CTX:-2048}"
NUM_THREAD="${OLLAMA_NUM_THREAD:-6}"
OLLAMA_ENDPOINT="${OLLAMA_ENDPOINT:-http://127.0.0.1:11434}"

if [ -z "$MODEL" ] || [ -z "$BUNDLE" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "Usage: $0 MODEL SYMBOL_BUNDLE_JSON OUTPUT_DIR" >&2
    exit 2
fi
for command in curl jq go ollama shasum; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "$command is required" >&2
        exit 2
    fi
done
if [ ! -f "$BUNDLE" ]; then
    echo "Bundle does not exist: $BUNDLE" >&2
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
BUNDLE_SHA256="$(shasum -a 256 "$BUNDLE" | awk '{print $1}')"
SYSTEM_PROMPT="You are a conservative classifier for a local Go code index. Output only schema values. Static symbol names and call edges are not source semantics. Never invent request data, literals, runtime outcomes, paths, tests, or identifiers."

jq '{
  target: {
    alias: "T",
    evidence_id: .target.evidence_id,
    name: .target.entity.name,
    kind: .target.entity.kind,
    path: .target.entity.location.path,
    line: .target.entity.location.line
  },
  outgoing: [
    .outgoing_calls[:8] | to_entries[] | {
      alias: ("O" + (.key + 1 | tostring)),
      evidence_id: .value.evidence_id,
      name: .value.callee.name,
      kind: .value.callee.kind,
      path: .value.callee.location.path,
      line: .value.callee.location.line
    }
  ],
  incoming_count: (.incoming_calls | length),
  outgoing_count: (.outgoing_calls | length),
  truncated: .truncated
}' "$BUNDLE" > "$OUTPUT_DIR/facts.json.tmp"
mv "$OUTPUT_DIR/facts.json.tmp" "$OUTPUT_DIR/facts.json"

if [ "$(jq '.outgoing | length' "$OUTPUT_DIR/facts.json")" -eq 0 ]; then
    echo "Staged experiment currently requires at least one outgoing static call" >&2
    exit 2
fi

# Rank obvious high-signal names before the model sees them. Aliases remain
# stable and still resolve to the original evidence IDs.
jq '
  (.target.name | split(".") | last | ascii_downcase) as $target_operation |
  def expected_hint($name):
    ($name | ascii_downcase) as $n |
    if ($n | test("check|validat")) then "validation_name"
    elif $n == $target_operation then "delegation_name"
    elif ($n | test("error")) then "error_translation_name"
    elif ($n | test("fill|header|enrich")) then "response_enrichment_name"
    elif ($n | test("^save|store|persist|commit")) then "persistence_name"
    elif ($n | test("sync|flush|fsync|read|write")) then "io_name"
    else null end;
  def name_priority($name):
    expected_hint($name) as $hint |
    if $hint == null then 9
    else {validation_name: 0, delegation_name: 1, error_translation_name: 2,
          response_enrichment_name: 3, persistence_name: 4, io_name: 5}[$hint]
    end;
  .outgoing |= sort_by(name_priority(.name)) |
  .preclassified = [
    .outgoing[] |
    expected_hint(.name) as $hint |
    select($hint != null) |
    {id: .alias, hint: $hint, source: "deterministic_name_rule"}
  ] |
  .ambiguous = [
    .outgoing[] |
    select(expected_hint(.name) == null)
  ]
' "$OUTPUT_DIR/facts.json" > "$OUTPUT_DIR/facts.json.tmp"
mv "$OUTPUT_DIR/facts.json.tmp" "$OUTPUT_DIR/facts.json"

run_stage() {
    stage="$1"
    max_predict="$2"
    stage_dir="$OUTPUT_DIR/$stage"
    mkdir -p "$stage_dir"

    jq -n \
        --arg model "$MODEL" \
        --arg system "$SYSTEM_PROMPT" \
        --rawfile prompt "$stage_dir/prompt.txt" \
        --slurpfile schema "$stage_dir/schema.json" \
        --argjson num_ctx "$NUM_CTX" \
        --argjson num_predict "$max_predict" \
        --argjson num_thread "$NUM_THREAD" \
        '{
          model: $model,
          stream: false,
          keep_alive: "10m",
          messages: [
            {role: "system", content: $system},
            {role: "user", content: $prompt}
          ],
          format: $schema[0],
          options: {
            num_ctx: $num_ctx,
            num_predict: $num_predict,
            num_thread: $num_thread,
            temperature: 0,
            top_p: 0.9
          }
        }' > "$stage_dir/request.json.tmp"
    mv "$stage_dir/request.json.tmp" "$stage_dir/request.json"

    curl --silent --show-error --fail --max-time 300 \
        --header 'Content-Type: application/json' \
        --data-binary "@$stage_dir/request.json" \
        --output "$stage_dir/response.envelope.json.tmp" \
        "$OLLAMA_ENDPOINT/api/chat"
    mv "$stage_dir/response.envelope.json.tmp" "$stage_dir/response.envelope.json"

    jq -er '.message.content' "$stage_dir/response.envelope.json" > "$stage_dir/response.raw.json.tmp"
    jq -e . "$stage_dir/response.raw.json.tmp" >/dev/null
    mv "$stage_dir/response.raw.json.tmp" "$stage_dir/response.raw.json"

    jq '{
      done_reason,
      total_seconds: (.total_duration / 1000000000),
      load_seconds: (.load_duration / 1000000000),
      prompt_tokens: .prompt_eval_count,
      prompt_seconds: (.prompt_eval_duration / 1000000000),
      output_tokens: .eval_count,
      output_seconds: (.eval_duration / 1000000000),
      output_tokens_per_second: (if .eval_duration > 0 then (.eval_count / (.eval_duration / 1000000000)) else 0 end)
    }' "$stage_dir/response.envelope.json" > "$stage_dir/metrics.json.tmp"
    mv "$stage_dir/metrics.json.tmp" "$stage_dir/metrics.json"
}

# Stage 1: classify name-level signals. There is deliberately no prose field.
jq -r '
  [
    "Classify the remaining ambiguous outgoing static-call names. Do not explain target behavior.",
    "A hint describes only what a callee name suggests, never what executes at runtime.",
    "Return each listed alias once when four or fewer are listed.",
    "Hint guide: check/validate -> validation; same operation name -> delegation; error conversion -> error translation; fill/header/enrich -> response enrichment; save/store/persist -> persistence; sync/flush/read/write -> I/O.",
    "Hard rule: when a callee name exactly equals TARGET_OPERATION_NAME, use delegation_name.",
    "",
    ("TARGET: T | " + .target.name + " [" + .target.kind + "]"),
    ("TARGET_OPERATION_NAME: " + (.target.name | split(".") | last)),
    "OUTGOING STATIC CALL NAMES:",
    (.ambiguous[] | (.alias + " | TARGET -> " + .name + " [" + .kind + "]"))
  ] | join("\n")
' "$OUTPUT_DIR/facts.json" > "$OUTPUT_DIR/stage-01-triage.prompt.tmp"
mkdir -p "$OUTPUT_DIR/stage-01-triage"
mv "$OUTPUT_DIR/stage-01-triage.prompt.tmp" "$OUTPUT_DIR/stage-01-triage/prompt.txt"

jq '
  (.ambiguous | map(.alias)) as $ids |
  ($ids | length | if . > 4 then 4 else . end) as $wanted |
  {
    type: "object",
    additionalProperties: false,
    required: ["signals"],
    properties: {
      signals: {
        type: "array",
        minItems: $wanted,
        maxItems: $wanted,
        items: {
          type: "object",
          additionalProperties: false,
          required: ["id", "hint"],
          properties: {
            id: {type: "string", enum: $ids},
            hint: {
              type: "string",
              enum: [
                "validation_name",
                "delegation_name",
                "error_translation_name",
                "response_enrichment_name",
                "persistence_name",
                "io_name",
                "coordination_name",
                "other"
              ]
            }
          }
        }
      }
    }
  }
' "$OUTPUT_DIR/facts.json" > "$OUTPUT_DIR/stage-01-triage/schema.json.tmp"
mv "$OUTPUT_DIR/stage-01-triage/schema.json.tmp" "$OUTPUT_DIR/stage-01-triage/schema.json"
if [ "$(jq '.ambiguous | length' "$OUTPUT_DIR/facts.json")" -eq 0 ]; then
    jq -n --arg model "$MODEL" '{skipped: true, model: $model, reason: "all names preclassified locally"}' \
        > "$OUTPUT_DIR/stage-01-triage/request.json"
    jq -n '{signals: []}' > "$OUTPUT_DIR/stage-01-triage/response.raw.json"
    jq -n '{skipped: true, reason: "all names preclassified locally"}' \
        > "$OUTPUT_DIR/stage-01-triage/response.envelope.json"
    jq -n '{
      skipped: true,
      total_seconds: 0,
      load_seconds: 0,
      prompt_tokens: 0,
      prompt_seconds: 0,
      output_tokens: 0,
      output_seconds: 0,
      output_tokens_per_second: 0
    }' > "$OUTPUT_DIR/stage-01-triage/metrics.json"
else
    run_stage "stage-01-triage" 128
fi

jq -n \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile response "$OUTPUT_DIR/stage-01-triage/response.raw.json" '
  ($facts[0].ambiguous | map(.alias)) as $allowed |
  [
    $response[0].signals[]? |
    select(.id as $id | $allowed | index($id)) |
    select(.hint | IN(
      "validation_name", "delegation_name", "error_translation_name",
      "response_enrichment_name", "persistence_name", "io_name",
      "coordination_name", "other"
    )) |
    . + {source: "model"}
  ] |
  unique_by(.id) |
  .[:4] |
  {signals: .}
' > "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp"
mv "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp" "$OUTPUT_DIR/stage-01-triage/normalized.json"

if [ "$(jq '.ambiguous | length' "$OUTPUT_DIR/facts.json")" -gt 0 ] && \
   [ "$(jq '.signals | length' "$OUTPUT_DIR/stage-01-triage/normalized.json")" -eq 0 ]; then
    jq '{signals: [(.ambiguous[0] | {id: .alias, hint: "other", source: "fallback"})], fallback: "first_ambiguous"}' \
        "$OUTPUT_DIR/facts.json" > "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp"
    mv "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp" "$OUTPUT_DIR/stage-01-triage/normalized.json"
fi

mv "$OUTPUT_DIR/stage-01-triage/normalized.json" "$OUTPUT_DIR/stage-01-triage/model_normalized.json"
jq -n \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile model "$OUTPUT_DIR/stage-01-triage/model_normalized.json" '{
      signals: ($facts[0].preclassified + $model[0].signals | unique_by(.id)),
      fallback: ($model[0].fallback // null)
    }' > "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp"
mv "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp" "$OUTPUT_DIR/stage-01-triage/normalized.json"

jq -n \
    --slurpfile raw "$OUTPUT_DIR/stage-01-triage/response.raw.json" \
    --slurpfile normalized "$OUTPUT_DIR/stage-01-triage/normalized.json" '{
      raw_items: ($raw[0].signals | length),
      accepted_unique_items: ($normalized[0].signals | length),
      deterministic_items: ($normalized[0].signals | map(select(.source == "deterministic_name_rule")) | length),
      model_items: ($normalized[0].signals | map(select(.source == "model")) | length),
      fallback: ($normalized[0].fallback // null)
    }' > "$OUTPUT_DIR/stage-01-triage/validation.json"

# Put the highest-value navigation signals first. Small local models exhibit a
# strong first-items bias, so ordering is an explicit part of context assembly.
jq '
  def priority($hint): {
    validation_name: 0,
    delegation_name: 1,
    error_translation_name: 2,
    response_enrichment_name: 3,
    persistence_name: 4,
    io_name: 5,
    coordination_name: 6,
    other: 7
  }[$hint];
  def source_priority($source):
    if $source == "deterministic_name_rule" then 0
    elif $source == "model" then 1
    else 2 end;
  .signals |= sort_by([source_priority(.source), priority(.hint)])
' "$OUTPUT_DIR/stage-01-triage/normalized.json" > "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp"
mv "$OUTPUT_DIR/stage-01-triage/normalized.json.tmp" "$OUTPUT_DIR/stage-01-triage/normalized.json"

# Stage 2: choose one role hypothesis from the validated name signals.
jq -n -r \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile triage "$OUTPUT_DIR/stage-01-triage/normalized.json" '
  ($facts[0]) as $f |
  ($triage[0]) as $t |
  [
    "Choose one cautious structural-role hypothesis from the validated name signals.",
    "This is a navigation hypothesis only. The target body and tests are absent.",
    "Use exactly three distinct signal aliases when three are available. Prefer validation, delegation, and error translation over response enrichment.",
    "Role guide: persistence/I-O collaborators suggest orchestrator or delegator; validation/delegation/error collaborators suggest handler or orchestrator. A method is not automatically a handler.",
    "Confidence can only be low or medium.",
    "",
    ("TARGET: T | " + $f.target.name),
    "VALIDATED NAME SIGNALS:",
    ($t.signals[] as $s |
      ($f.outgoing[] | select(.alias == $s.id)) as $fact |
      ($s.id + " | " + $fact.name + " | " + $s.hint))
  ] | join("\n")
' > "$OUTPUT_DIR/stage-02-hypothesis.prompt.tmp"
mkdir -p "$OUTPUT_DIR/stage-02-hypothesis"
mv "$OUTPUT_DIR/stage-02-hypothesis.prompt.tmp" "$OUTPUT_DIR/stage-02-hypothesis/prompt.txt"

jq '
  (.signals | map(.id)) as $ids |
  (.signals | map(.hint)) as $hints |
  (if ($hints | any(. == "persistence_name" or . == "io_name")) then
     ["orchestrator_candidate", "delegator_candidate", "wrapper_candidate", "unknown"]
   elif ($hints | any(. == "validation_name" or . == "delegation_name" or . == "error_translation_name")) then
     ["handler_candidate", "orchestrator_candidate", "adapter_candidate", "wrapper_candidate", "delegator_candidate", "unknown"]
   else
     ["orchestrator_candidate", "adapter_candidate", "wrapper_candidate", "delegator_candidate", "mapper_candidate", "unknown"]
   end) as $roles |
  {
    type: "object",
    additionalProperties: false,
    required: ["role", "use", "confidence"],
    properties: {
      role: {
        type: "string",
        enum: $roles
      },
      use: {
        type: "array",
        minItems: (if ($ids | length) < 3 then ($ids | length) else 3 end),
        maxItems: (if ($ids | length) < 3 then ($ids | length) else 3 end),
        uniqueItems: true,
        items: {type: "string", enum: $ids}
      },
      confidence: {type: "string", enum: ["low", "medium"]}
    }
  }
' "$OUTPUT_DIR/stage-01-triage/normalized.json" > "$OUTPUT_DIR/stage-02-hypothesis/schema.json.tmp"
mv "$OUTPUT_DIR/stage-02-hypothesis/schema.json.tmp" "$OUTPUT_DIR/stage-02-hypothesis/schema.json"
run_stage "stage-02-hypothesis" 64

jq -n \
    --slurpfile triage "$OUTPUT_DIR/stage-01-triage/normalized.json" \
    --slurpfile response "$OUTPUT_DIR/stage-02-hypothesis/response.raw.json" '
  ($triage[0].signals | map(.id)) as $allowed |
  ($response[0].use | map(select(. as $id | $allowed | index($id))) | unique | .[:3]) as $use |
  {
    role: ($response[0].role // "unknown"),
    use: (if ($use | length) == 0 then [$allowed[0]] else $use end),
    confidence: (if $response[0].confidence == "medium" then "medium" else "low" end)
  }
' > "$OUTPUT_DIR/stage-02-hypothesis/normalized.json.tmp"
mv "$OUTPUT_DIR/stage-02-hypothesis/normalized.json.tmp" "$OUTPUT_DIR/stage-02-hypothesis/normalized.json"

jq -n \
    --slurpfile raw "$OUTPUT_DIR/stage-02-hypothesis/response.raw.json" \
    --slurpfile normalized "$OUTPUT_DIR/stage-02-hypothesis/normalized.json" '{
      raw_use_count: ($raw[0].use | length),
      accepted_unique_use_count: ($normalized[0].use | length),
      confidence: $normalized[0].confidence
    }' > "$OUTPUT_DIR/stage-02-hypothesis/validation.json"

# Stage 3: choose one executable evidence action, not a prose query.
jq -n \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile hypothesis "$OUTPUT_DIR/stage-02-hypothesis/normalized.json" '
  ($facts[0]) as $f |
  ($hypothesis[0]) as $h |
  ([{
    operation: "read_target",
    anchor_alias: "T",
    anchor_evidence_id: $f.target.evidence_id,
    label: ("read target " + $f.target.name)
  }] + [
    $h.use[] as $id |
    $f.outgoing[] |
    select(.alias == $id) |
    {
      operation: "read_callee",
      anchor_alias: .alias,
      anchor_evidence_id: .evidence_id,
      label: ("read static callee " + .name)
    }
  ] + [{
    operation: "find_tests",
    anchor_alias: "T",
    anchor_evidence_id: $f.target.evidence_id,
    label: ("find tests for " + $f.target.name)
  }, {
    operation: "expand_callers",
    anchor_alias: "T",
    anchor_evidence_id: $f.target.evidence_id,
    label: ("expand callers of " + $f.target.name)
  }]) |
  to_entries |
  map(.value + {id: ("A" + (.key + 1 | tostring))})
' > "$OUTPUT_DIR/actions.json.tmp"
mv "$OUTPUT_DIR/actions.json.tmp" "$OUTPUT_DIR/actions.json"

jq -n -r \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile hypothesis "$OUTPUT_DIR/stage-02-hypothesis/normalized.json" \
    --slurpfile actions "$OUTPUT_DIR/actions.json" '
  [
    "Choose the single local evidence action that best reduces the current unknown.",
    "The target body is absent. Prefer reading it before explaining behavior.",
    "Return only an unknown code and an action ID.",
    "",
    ("TARGET: T | " + $facts[0].target.name),
    ("ROLE HYPOTHESIS: " + $hypothesis[0].role + " | confidence=" + $hypothesis[0].confidence),
    "AVAILABLE ACTIONS:",
    ($actions[0][] | (.id + " | " + .label))
  ] | join("\n")
' > "$OUTPUT_DIR/stage-03-action.prompt.tmp"
mkdir -p "$OUTPUT_DIR/stage-03-action"
mv "$OUTPUT_DIR/stage-03-action.prompt.tmp" "$OUTPUT_DIR/stage-03-action/prompt.txt"

jq '{
  type: "object",
  additionalProperties: false,
  required: ["unknown", "choice"],
  properties: {
    unknown: {
      type: "string",
      enum: [
        "target_body",
        "branch_conditions",
        "call_order",
        "error_conditions",
        "side_effects",
        "runtime_reachability",
        "tests",
        "dynamic_calls",
        "build_variants"
      ]
    },
    choice: {type: "string", enum: map(.id)}
  }
}' "$OUTPUT_DIR/actions.json" > "$OUTPUT_DIR/stage-03-action/schema.json.tmp"
mv "$OUTPUT_DIR/stage-03-action/schema.json.tmp" "$OUTPUT_DIR/stage-03-action/schema.json"
run_stage "stage-03-action" 48

jq -n \
    --slurpfile actions "$OUTPUT_DIR/actions.json" \
    --slurpfile response "$OUTPUT_DIR/stage-03-action/response.raw.json" '
  ($actions[0] | map(.id)) as $allowed |
  ($response[0].choice as $choice |
    if ($allowed | index($choice)) then $choice else "A1" end) as $choice |
  {
    unknown: ($response[0].unknown // "target_body"),
    choice: $choice,
    fallback: (if $choice == $response[0].choice then null else "read_target" end)
  }
' > "$OUTPUT_DIR/stage-03-action/normalized.json.tmp"
mv "$OUTPUT_DIR/stage-03-action/normalized.json.tmp" "$OUTPUT_DIR/stage-03-action/normalized.json"

jq -n \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile triage "$OUTPUT_DIR/stage-01-triage/normalized.json" \
    --slurpfile hypothesis "$OUTPUT_DIR/stage-02-hypothesis/normalized.json" \
    --slurpfile action_result "$OUTPUT_DIR/stage-03-action/normalized.json" \
    --slurpfile actions "$OUTPUT_DIR/actions.json" \
    -f scripts/ollama_symbol_staged_result.jq > "$OUTPUT_DIR/final_result.json.tmp"
mv "$OUTPUT_DIR/final_result.json.tmp" "$OUTPUT_DIR/final_result.json"

# Render a legacy-compatible report locally so the existing contract evaluator
# remains useful. This score measures the reducer output, not model intelligence.
jq -n \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile result "$OUTPUT_DIR/final_result.json" \
    -f scripts/ollama_symbol_staged_report.jq > "$OUTPUT_DIR/final_response.raw.json.tmp"
mv "$OUTPUT_DIR/final_response.raw.json.tmp" "$OUTPUT_DIR/final_response.raw.json"

go run ./cmd/symbol-evaluate \
    --bundle "$BUNDLE" \
    --response "$OUTPUT_DIR/final_response.raw.json" \
    --out-dir "$OUTPUT_DIR" >/dev/null

metric_paths=("$OUTPUT_DIR/stage-01-triage/metrics.json")
metric_paths+=("$OUTPUT_DIR/stage-02-hypothesis/metrics.json" "$OUTPUT_DIR/stage-03-action/metrics.json")

jq -s '{
  stages: .,
  totals: {
    total_seconds: (map(.total_seconds) | add),
    load_seconds: (map(.load_seconds) | add),
    prompt_tokens: (map(.prompt_tokens) | add),
    output_tokens: (map(.output_tokens) | add),
    model_calls: (map(select(.skipped != true)) | length)
  }
}' "${metric_paths[@]}" > "$OUTPUT_DIR/metrics.json.tmp"
mv "$OUTPUT_DIR/metrics.json.tmp" "$OUTPUT_DIR/metrics.json"

# This evaluates only the constrained decisions for which a deterministic
# name-level expectation is high precision. It is not a source-truth score.
jq -n \
    --slurpfile facts "$OUTPUT_DIR/facts.json" \
    --slurpfile result "$OUTPUT_DIR/final_result.json" \
    --slurpfile hypothesis_schema "$OUTPUT_DIR/stage-02-hypothesis/schema.json" \
    -f scripts/ollama_symbol_staged_evaluation.jq > "$OUTPUT_DIR/protocol_evaluation.json.tmp"
mv "$OUTPUT_DIR/protocol_evaluation.json.tmp" "$OUTPUT_DIR/protocol_evaluation.json"

OLLAMA_VERSION="$(curl --silent --fail "$OLLAMA_ENDPOINT/api/version" | jq -r '.version')"
jq -n \
    --arg model "$MODEL" \
    --arg ollama_version "$OLLAMA_VERSION" \
    --arg bundle_sha256 "$BUNDLE_SHA256" \
    --argjson num_ctx "$NUM_CTX" \
    --argjson num_thread "$NUM_THREAD" '{
      protocol_version: "local-symbol-v2",
      prompt_version: "staged-enum-v2",
      schema_version: 2,
      reducer_version: 2,
      evaluator_version: 1,
      model: $model,
      ollama_version: $ollama_version,
      bundle_sha256: $bundle_sha256,
      options: {num_ctx: $num_ctx, num_thread: $num_thread, temperature: 0},
      stages: ["local name preclassification", "stage-01-triage", "stage-02-hypothesis", "stage-03-action"]
    }' > "$OUTPUT_DIR/manifest.json.tmp"
mv "$OUTPUT_DIR/manifest.json.tmp" "$OUTPUT_DIR/manifest.json"

echo "Experiment: $OUTPUT_DIR"
jq -s '{
  protocol: .[0].protocol_version,
  model: .[0].model,
  totals: .[1].totals,
  hypothesis: .[2].hypothesis,
  next_action: .[2].next_action,
  reducer_contract_score: {score: .[3].score, max_score: .[3].max_score},
  protocol_evaluation: {passed: .[4].passed, total: .[4].total}
}' \
    "$OUTPUT_DIR/manifest.json" \
    "$OUTPUT_DIR/metrics.json" \
    "$OUTPUT_DIR/final_result.json" \
    "$OUTPUT_DIR/symbol_evaluation.json" \
    "$OUTPUT_DIR/protocol_evaluation.json"
