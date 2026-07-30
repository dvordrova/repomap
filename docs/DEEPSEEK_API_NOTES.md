# DeepSeek API notes

Implementation reference for the OpenAI-compatible transport and DeepSeek
reference prompts in `internal/deepseek/`.

## Configuration modes

New integrations use one atomic namespace:

```text
REPOMAP_LLM_ENDPOINT      required full chat/completions URL
REPOMAP_LLM_MODEL         model name
REPOMAP_LLM_API_KEY       required for bearer auth
REPOMAP_LLM_AUTH          bearer (default) or none
REPOMAP_LLM_MAX_TOKENS    positive integer (default 6000)
REPOMAP_LLM_TIMEOUT       Go duration (default 10m)
```

Once any `REPOMAP_LLM_*` variable is present, the endpoint is required and no
`DEEPSEEK_*` value is inherited. This prevents a company key from falling back
to the public DeepSeek endpoint and prevents a stale DeepSeek key from reaching
an internal endpoint.

`DEEPSEEK_ENDPOINT`, `DEEPSEEK_MODEL`, `DEEPSEEK_API_KEY`,
`DEEPSEEK_MAX_TOKENS`, `DEEPSEEK_TIMEOUT`, and `DEEPSEEK_AUTH` remain a
legacy-only compatibility mode. With no explicit legacy endpoint/model, the
DeepSeek defaults below are used.

The application does not source `.env` files. The current repository may be
untrusted input and its environment file may contain unrelated credentials or
shell syntax. The repomap development `Makefile` imports only repomap's own
ignored `.env`; this does not read an analyzed repository's environment file.

For repository development commands, use Make targets. The ignored `.env` may
contain `DEEPSEEK_API_KEY`:

```bash
make source-prompt-experiment LABEL=trial ETCD_REPO=../etcd SYMBOL=kvServer.Put
```

Direct script execution uses only the caller's exported environment and never
loads `.env` implicitly.

To calibrate the generic OpenAI-compatible namespace against the same DeepSeek
reference endpoint/model without exposing the ignored local key, use:

```bash
make generic-deepseek-doctor
```

The target maps the local DeepSeek key into the atomic `REPOMAP_LLM_*`
namespace for this command. The application ignores legacy values whenever the
generic namespace is active.

## Endpoint

```
https://api.deepseek.com/chat/completions
```

This is the default only in legacy/DeepSeek mode.

## Authentication

Bearer sends `Authorization: Bearer <key>`. Explicit `AUTH=none` sends no
Authorization header and does not retain an otherwise configured key. No-auth
requires an explicit endpoint in both generic and legacy modes.

Must NEVER be written to disk or debug artifacts.

## Failure diagnostics

Normal artifact-backed runs persist safe effective invocation data in
`metadata.json` before the orientation request is sent. This includes the
parsed CLI options, endpoint, model, auth mode (`bearer` or `none`), timeout,
token budget, request stage, serialized request size, state, and latency. It
never includes the API key, raw environment, request headers, or
`Authorization` value. On a failed default run the CLI prints the exact
`metadata.json` path so another developer can verify which non-secret settings
were actually applied.

Long orientation, targeted-research, and architecture requests emit a
content-free progress heartbeat every ten seconds. The message contains only
the stage, model where applicable, elapsed time, and Ctrl-C guidance; it never
contains prompt, response, source, credential, or header material. Ctrl-C
cancels the shared request context even though the default transport timeout is
ten minutes.

With `--dump-llm`, `llm_request.redacted.json` remains the inspectable request
body written before network access. Without that flag, a provider failure now
writes the same redacted request body automatically, while `error.txt` retains
the bounded safe provider error. Request-attempt metadata is written before
network access, so a failed or canceled request does not look as though no
request was prepared.

## Model

DeepSeek-mode default: `deepseek-v4-flash`.

## Request shape

```json
{
  "model": "deepseek-v4-flash",
  "messages": [
    {
      "role": "system",
      "content": "You are a senior software engineer helping orient inside a large unfamiliar repository. Infer the language from language_hints and use only the provided facts. Do not pretend to have read files that were not provided. Return valid json only."
    },
    {
      "role": "user",
      "content": "Do not explain the whole repo. Help the developer choose what runtime/event flow to inspect next.\n\nProduce a json orientation report with this exact shape:\n{...}\n\nImportant rules:\n- Candidate flows must be runtime/event-oriented ...\n\nFacts bundle JSON:\n{...}"
    }
  ],
  "temperature": 0.1,
  "max_tokens": 6000,
  "response_format": {"type": "json_object"}
}
```

## JSON mode requirements

- Request must include `"response_format": {"type": "json_object"}`.
- Prompt must contain the word **json** (case-insensitive match is fine).
- Prompt must include an example JSON shape so the model knows the expected schema.
- `max_tokens` must be high enough to avoid truncation (default 6000).
- Do not replace the configured/default budget with a small stage-specific cap.
  The Pebble component planner returned empty content at 1600 tokens and
  succeeded at 6000.
- DeepSeek V4 enables thinking by default. The architecture grouping request is
  a bounded classification task, so the official DeepSeek endpoint receives
  `"thinking": {"type":"disabled"}` for that request only. Generic compatible
  endpoints do not receive this DeepSeek-specific extension.
- The guided-onboarding comparison uses the opposite, purpose-specific policy
  on the official endpoint: independently verifiable semantic leaves receive
  `"thinking":{"type":"enabled"}` with `"reasoning_effort":"high"`; the
  monolithic editor and final fan-in planner receive `reasoning_effort:"max"`.
  Thinking mode ignores temperature, so determinism comes from strict JSON
  contracts, opaque-ID validation, canonical inputs, and replayable caches.
- The monolithic guided editor and final guided fan-in use a minimum
  12,000-token response envelope on the official endpoint. Bounded leaves keep
  the configured default, and larger explicit user configuration is preserved.
  Real Chatto and self-experiment runs showed `thinking/max` consuming the
  ordinary 6,000-token envelope before completing the strict JSON wrapper. If
  the provider explicitly ends a partial JSON response with
  `finish_reason=length`, the guided editor retries exactly once with twice the
  first attempt's output headroom. Retryable network failures, malformed
  response envelopes, and incomplete JSON receive one bounded replay of the
  same request. A syntactically valid proposal rejected by local Guided Tour
  validation also receives one fresh bounded proposal attempt. Only a locally
  accepted proposal can be cached or published.
- Semantic discovery always uses JSON mode. On the official DeepSeek endpoint,
  the opportunity scan, fan-in synthesis, and monolithic comparison use
  `"thinking":{"type":"enabled"}` with `reasoning_effort:"max"`; bounded
  evidence leaves use `reasoning_effort:"high"`. Temperature is omitted for
  all of these thinking-mode requests. These three global semantic tasks use
  a minimum 20,000-token response envelope, while
  bounded leaves retain the configured default and larger explicit user
  configurations are preserved. Generic compatible endpoints receive the
  ordinary OpenAI-compatible JSON request without DeepSeek-specific thinking
  fields. The first self scan exhausted exactly 6,000 output tokens and ended
  in a locally rejected, truncated JSON object; no invalid response was
  accepted or written to the semantic replay record.
  A later five-artifact fan-in also exhausted exactly 12,000 output tokens and
  ended mid-JSON, so 12,000 is not sufficient for DeepSeek V4 Flash at
  `reasoning_effort:"max"` even when the final JSON itself is small. The
  20,000-token minimum is limited to global semantic tasks; bounded leaves and
  unrelated stages keep their existing limits.
- DeepSeek usage may report `prompt_cache_hit_tokens` and
  `prompt_cache_miss_tokens`. Guided experiment records and semantic-discovery
  measured results preserve both fields independently from total prompt tokens
  so a stable common leaf prefix can be evaluated rather than assumed to hit
  cache.
- Empty content errors include safe `finish_reason` and token-count diagnostics
  when the provider supplies them. Reasoning content itself is never echoed or
  retained.

## Expected orientation report shape

```json
{
  "project_guess": "short guess",
  "confidence": 0.0,
  "high_level_map": [
    {
      "name": "component or subsystem name",
      "role": "entry | boundary | coordination | domain | state | support | unknown",
      "evidence": ["facts or paths from bundle"],
      "why_it_matters": "..."
    }
  ],
  "first_files_to_open": [
    {"path": "repo-relative path", "reason": "..."}
  ],
  "candidate_flows": [
    {
      "name": "runtime or event flow name",
      "flow_type": "request | operational",
      "trigger": "what starts this flow",
      "likely_entrypoint": "exact full path from allowed_paths",
      "likely_files": ["repo-relative paths"],
      "why_interesting": "...",
      "evidence": ["facts from bundle supporting this flow"],
      "confidence": 0.0
    }
  ],
  "important_domain_words": [
    {
      "word": "term",
      "guess": "what it probably means",
      "evidence": ["paths or readme excerpts"]
    }
  ],
  "questions_for_human": [
    "question that helps guide next analysis step"
  ],
  "unverified_paths": [
    {"path": "suspected/repo-relative/path", "reason": "not in allowed_paths"}
  ],
  "warnings": [
    "uncertainty or missing context"
  ]
}
```

Operational flows must cite bounded `source_signals` evidence. When the static
evidence remains weak and no local proof establishes execution, confidence is
capped at `0.3`. Request and operational flows remain one naturally ranked
candidate list.

Orientation parsing separates recoverable prose drift from structured
navigation. A free-form evidence item that contains an invalid or unprovided
path-like mention is dropped with a warning. If `likely_entrypoint` is neither
an allowed file nor a provided entrypoint package, it may be replaced with that
flow's first already-allowed `likely_file`, again with a warning.
`first_files_to_open` and `candidate_flows[].likely_files` are never repaired to
invented values. Invalid or unallowed items are removed with an explicit parser
warning; the remaining structured paths still validate fail-closed. A response
with no grounded candidate flow remains fatal.
Orientation prompt v6 retains the atomic-evidence and closed-`allowed_paths`
rules introduced in v3, explicitly rejects directory, package, import, and
trailing-slash values in structured file fields, and adds the bounded component
role used by the landscape layout.
`main.go` is not accepted as an abbreviation for `cmd/prometheus/main.go`, and
an import such as `pkg/goanalysis` is not accepted as a file. Prompt v2 captures
remain replayable historical artifacts. The Prometheus capture still contains
mixed prose items, which quality replay leaves explicitly unscored. Its clean
raw-contract flag means the JSON wire shape was clean, not that every semantic
prompt instruction was obeyed.

The component `role` is a bounded orientation hypothesis used only to arrange
the browser landscape. It is normalized to `unknown` when a provider returns an
unsupported value. Static package imports remain separate evidence and never
upgrade a semantic role into a verified fact.

## Component planning and probe handoff

Component planning is a bounded question-and-selection call, not a repository
explanation. Prompt `component-plan-json-v3` returns Plan v2. It must choose one
`primary_question_id`, two to four questions, at most two file IDs, and at most
three symbol IDs from the supplied bundle. IDs are opaque: repository paths,
symbol names, certainty, and provenance are reconstructed locally and are never
accepted from model prose.

The parser is deliberately tolerant at the wire boundary. It accepts exact,
fenced, or embedded JSON, common scalar-list drift, and ID-only objects. Unknown
IDs and malformed optional entries are dropped with diagnostics while surviving
selections remain usable. Old captures without `primary_question_id` replay as
Plan v2 by selecting the first surviving question and recording the repair.
Replay stores the current request prompt version and the source response prompt
version separately and makes no provider call.

Two empirical cases define the next boundary:

- Soft Serve selected `serve.go`, `server.go`, and `NewServer` / `Start` /
  `Shutdown`, giving a useful lifecycle frontier.
- Pebble selected `Batch.Commit`, `commitPipeline.Commit`, and `directWrite`, but
  exact-symbol inspection showed the direct edge is `Batch.Commit -> DB.Apply`.
  The attractive planner story was therefore not yet connected evidence.

For that reason a focused teacher must not consume a planner result directly.
The local handoff is:

```text
Plan -> bounded exact-symbol Probe -> connected | frontier | blocked -> Teacher
```

The probe makes no model call. It collects direct static relations, bounded
source and call-site windows, and test references for the primary question. A
`frontier` may offer opaque IDs, but the research budget permits only one
accepted frontier symbol and two probe rounds total. Once that budget is spent,
Teacher receives a partial dossier and must state the unresolved link; it does
not trigger an unbounded search. This contract is still an experiment and does
not change the default orientation request.

Focused teaching currently uses `component-teach-json-v2`. The provider sees a
validated path-free bundle of evidence IDs, source text, named static
relations, support bases, and opaque unresolved frontier hints. The local
`file:line` index is a separate artifact and is never placed in the request.
The request uses JSON mode, temperature zero, and the configured/default token
budget without a stage-specific cap.

The response parser accepts plain, fenced, or embedded JSON, singleton section
objects, and scalar ID drift. It assigns item IDs locally and drops unknown
evidence/frontier IDs without discarding valid siblings. It also rejects an
unqualified closed-world claim such as "only used" or "does not call" when its
anchors are only bounded static/navigation evidence. This produces a warning
instead of silently presenting absence as proof. A source-supported local
negative branch is not rejected by that guard.

## Focused symbol investigation

Symbol investigation is a separate bounded request. It does not send the raw
evidence graph, file tree, package edge list, repository contents, environment,
or local working directory.

The local pipeline is:

1. gopls fuzzy candidates (`possible`)
2. one unique case-sensitive exact symbol resolution (`static`)
3. direct incoming/outgoing call hierarchy edges (`static`)
4. bounded `symbol_bundle.json` with evidence IDs and `allowed_paths`
5. DeepSeek request with a compact JSON or `KEY: VALUE` interpretation contract
6. tolerant local parsing with explicit repair warnings
7. deterministic reconstruction and strict validation of the normalized report

Generate and inspect the exact request without an API key:

```bash
go run ./cmd/symbol-playground \
  --repo ../etcd \
  --symbol kvServer.Put \
  --out-dir tmp/symbol-examples/etcd-put
```

This writes:

- `evidence_graph.json` — full local analyzer output; never sent
- `symbol_bundle.json` — bounded facts sent to the model
- `deepseek_request.redacted.json` — exact request body; no Authorization header

Add `--deepseek` to make the API call. Use `--format tagged` (the default) or
`--format json` as a control format. Raw output is always saved before parsing. The parser accepts
common weak-model drift such as fenced JSON, trailing commas, scalar list items,
and the tagged line format. Invalid evidence and paths are dropped with warnings;
only an unreadable response is fatal.

Both symbol prompts ask the model only for:

- a summary and likely responsibility, both explicitly treated as inference
- evidence IDs for those interpretations and for the reading order
- unknowns, concrete next queries, and warnings

The model does not copy target, caller, callee, path, or structural-role fields.
Those are reconstructed from the local bundle. The normalized report still
enforces:

- every substantive statement cites an existing `evidence_id`
- static-only summary/responsibility inference confidence to be capped at 0.75
- static call edges to never be presented as observed runtime execution
- caller/callee identity to match the cited call fact
- every referenced file to appear in `allowed_paths` and in cited evidence
- file recommendations to use validated structural roles instead of semantic prose
- `tests_to_read` to contain only evidence-backed `*_test.go` files
- runtime paths/hypotheses to be omitted at depth one; missing runtime evidence belongs in unknowns and next queries

The JSON wire contract is intentionally smaller than `internal/symbol.Report`:

```json
{
  "summary": {"statement":"...","evidence_ids":["resolution-001"],"confidence":0.7},
  "responsibility": {"statement":"...","evidence_ids":["call-out-001"],"confidence":0.6},
  "read_evidence_ids": ["resolution-001", "call-out-001"],
  "test_evidence_ids": [],
  "unknowns": [],
  "next_queries": [],
  "warnings": []
}
```

The tagged contract expresses the same information as repeated `KEY: VALUE`
lines. `symbol_evaluation.json` scores observable contract adherence out of 100;
it deliberately does not pretend to score semantic truth. Run and compare real
prompt versions with:

```bash
./scripts/symbol_prompt_experiment.sh baseline ../etcd kvServer.Put json
./scripts/symbol_prompt_experiment.sh tagged ../etcd kvServer.Put tagged
./scripts/symbol_prompt_compare.sh \
  tmp/prompt-experiments/baseline-json \
  tmp/prompt-experiments/tagged-tagged
```

Stable bundle/response fixtures and an in-memory explainer live in
`internal/deepseektest`; they let higher layers test without calling DeepSeek.
Capture tooling reads current contract IDs from `repomap dev prompt-versions`
instead of duplicating prompt-version literals in shell.

## Task investigation synthesis

Task Lens uses one compact JSON-mode editing request after deterministic local
retrieval. The provider receives only the bounded task bundle projection:
opaque IDs, selected source/document excerpts, exact local relations, and the
closed `allowed_paths` set. It never receives the checkout display name, raw
file tree, global edges, or generic onboarding artifacts.

On the official DeepSeek endpoint, this request enables thinking with
`reasoning_effort: "high"`, omits temperature, and uses a purpose-specific
10,000-token minimum because the development run demonstrated truncation at
the ordinary 6,000-token envelope. Compatible endpoints receive the ordinary
configured JSON-mode request. The
response is only a proposal: local reduction rejects unknown IDs, fabricated
local relations, unsupported causal claims, ungrounded commands, and paths
outside the bundle. A substantive rejection is not retried; Task Lens may
publish a clearly labeled deterministic local partial instead.

## Source assessment

Source prompt v5 receives one bounded lexical source bundle. For a question
whose call result is used on a separate line, `shown` must cite both the exact
call anchor and the candidate result-use line. The response stays compact:
question ID, verdict, source evidence IDs, explicit unknowns, and one allowed
next action.

Three syntax-only predicates avoid inventing semantics when a callee name is
uninformative: `checks_call_result`, `returns_call_result`, and
`calls_from_branch`. The first requires the immediate guard/comparison proof,
the second requires the call itself to be the direct return expression, and the
third requires both a locally visible branch line and a complete standalone call
statement. These observations do not establish callee behavior, runtime
reachability, or which branch executes.

Local parsing remains tolerant without trusting the model to manufacture proof.
If the model cites the anchor but omits an immediate guard or returned nil
comparison, the assessment becomes `ambiguous` with
`assessment.shown_without_predicate_support`, and the evaluator deducts the
evidence-contract points. Unsupported, reassigned, transformed, non-immediate,
split-line, or truncated shapes are not promoted.

## Error handling

- Non-2xx HTTP response: return status plus a bounded response body. Obvious
  credential-like content is replaced with a redaction marker rather than echoed
  into stderr or debug artifacts.
- Orientation responses still require JSON. Focused symbol responses are parsed
  tolerantly and fail only when neither a JSON object nor tagged report can be recovered.
- No key is required for `--snapshot-only`, `--llm-bundle-only`, or request
  preview. Live bearer calls require the key from the active namespace.

## Retry behavior

- Retry on network errors (unless context canceled/exceeded).
- Retry on HTTP 429 and 5xx.
- No retry on HTTP 4xx (except 429).
- Small exponential backoff with jitter, max 3 retries.

## Debugging

```bash
repomap orient --repo ../etcd --debug-dir .repomap-runs --dump-llm
./scripts/debug_last_run.sh .repomap-runs
```

Artifacts produced:
- `metadata.json` — run metadata (model, endpoint, command)
- `snapshot.json` — full local deterministic snapshot
- `llm_bundle.json` — compact bounded bundle sent to DeepSeek
- `llm_request.redacted.json` — full request body (no Authorization header)
- `llm_response.raw.json` — raw DeepSeek HTTP response body (redacted)
- `orientation_report.json` — parsed/pretty orientation report
- `error.txt` — error message if any step failed

Never commit `.repomap-runs/`.
