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
REPOMAP_LLM_MAX_TOKENS    positive integer (default 64000; sole output-ceiling override)
REPOMAP_LLM_TIMEOUT       Go duration (default 10m)
```

Once any `REPOMAP_LLM_*` variable is present, the endpoint is required and no
`DEEPSEEK_*` value is inherited. This prevents a company key from falling back
to the public DeepSeek endpoint and prevents a stale DeepSeek key from reaching
an internal endpoint.

`DEEPSEEK_ENDPOINT`, `DEEPSEEK_MODEL`, `DEEPSEEK_API_KEY`,
`DEEPSEEK_TIMEOUT`, and `DEEPSEEK_AUTH` remain a legacy-only compatibility
mode. With no explicit legacy endpoint/model, the DeepSeek defaults below are
used. `DEEPSEEK_MAX_TOKENS` is intentionally ignored: the only output-ceiling
override in either configuration mode is `REPOMAP_LLM_MAX_TOKENS`.

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

Ordinary debug runs write every covered semantic request and its validated
response, or a truthful closed unavailable marker, under `semantic_exchanges/`.
The bounded payloads are redacted and secret-scanned, and `exchange.v1.json` is
published last as the commit marker. Use request preview when the exact request
must be inspected without making a provider call. Request-attempt metadata is
still written before network access, so a failed or canceled request does not
look as though no request was prepared.

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
  "max_tokens": 64000,
  "response_format": {"type": "json_object"}
}
```

## JSON mode requirements

- Request must include `"response_format": {"type": "json_object"}`.
- Prompt must contain the word **json** (case-insensitive match is fine).
- Prompt must include an example JSON shape so the model knows the expected schema.
- Every semantic request serializes the exact configured global output ceiling.
  The default is 64,000 and `REPOMAP_LLM_MAX_TOKENS` is its only override.
  Request builders do not raise, lower, or double it for a stage or endpoint.
  Historical Pebble calibration returned empty content at 1,600 tokens and
  succeeded at the then-configured 6,000-token ceiling; that evidence motivated
  removing smaller stage-specific caps, not a current per-stage exception.
- DeepSeek V4 enables thinking by default. The architecture grouping request is
  a bounded classification task, so the official DeepSeek endpoint receives
  `"thinking": {"type":"disabled"}` for that request only. Generic compatible
  endpoints do not receive this DeepSeek-specific extension.
- Orientation is likewise a bounded classification over an already compact
  local facts bundle. The official endpoint receives explicit disabled
  thinking; compatible endpoints do not. This prevents hidden reasoning from
  consuming the whole JSON envelope before any report content is returned.
- The guided-onboarding comparison uses the opposite, purpose-specific policy
  on the official endpoint: independently verifiable semantic leaves receive
  `"thinking":{"type":"enabled"}` with `"reasoning_effort":"high"`; the
  monolithic editor and final fan-in planner receive `reasoning_effort:"max"`.
  Thinking mode ignores temperature, so determinism comes from strict JSON
  contracts, opaque-ID validation, canonical inputs, and replayable caches.
- Guided editor, fan-in, and leaf requests all use the exact global ceiling.
  Historical Chatto and self-experiment runs showed `thinking/max` consuming
  6,000 and 12,000-token envelopes before completing the strict JSON wrapper;
  the current 64,000 default replaces those former stage floors. An explicit
  `finish_reason=length`, malformed or incomplete semantic content, or local
  Guided Tour rejection is terminal for that semantic request. It is never
  resent for more output or replaced by a fresh proposal. Only a locally
  accepted proposal can be cached or published.
- Semantic discovery always uses JSON mode. On the official DeepSeek endpoint,
  the opportunity scan, fan-in synthesis, and monolithic comparison use
  `"thinking":{"type":"enabled"}` with `reasoning_effort:"max"`; bounded
  evidence leaves use `reasoning_effort:"high"`. Temperature is omitted for
  all of these thinking-mode requests. Global tasks and bounded leaves use the
  same exact configured ceiling. Generic compatible endpoints receive the
  ordinary OpenAI-compatible JSON request without DeepSeek-specific thinking
  fields. Historical scans exhausted 6,000 and 12,000 output-token envelopes
  and ended in locally rejected truncated JSON; no invalid response was
  accepted or written to the semantic replay record. Those measurements
  informed the current global 64,000 default rather than creating per-stage
  minimums.
- Study reading-pack review is a bounded classification over three to five
  exact anchors. The official endpoint receives explicit disabled thinking for
  that review only; compatible endpoints receive no DeepSeek-specific field.
  A Casdoor control run showed four of eight otherwise independent reviews
  returning reasoning-only completions, including one `finish_reason=stop`, so
  increasing the output cap would not recover the missing JSON verdicts.
- Complete Russian presentation projection is assembled from deterministic
  bounded requests rather than one monolithic completion. Fields are sorted by
  their stable presentation ID and partitioned by a predicted output budget;
  every batch records an exact manifest and content hash. The provider wire is
  the compact ordered `[index, text]` tuple form, while the saved sidecar and
  internal projection remain keyed by the original stable IDs. Each batch has
  an exact request/cache identity, passes the same strict completeness,
  placeholder, language, secret, and tuple-order validation on a live response
  or cache hit, and uses the exact global output ceiling. A batch contains
  at most 64 fields. The current saved Casdoor replay contains 508 fields and
  deterministically produces eight batches: seven batches of 64 fields and one
  final batch of 60. This is a replay measurement, not a claim of complete live
  RU success. A live batch that fails quality validation is rejected without a
  localization-specific repair request; a rejected result is neither cached
  nor applied. The final RU sidecar is published only after every batch validates
  and the merged projection validates against the full canonical English
  inventory. A missing, corrupt, truncated, or rejected batch therefore
  degrades the whole presentation atomically to canonical English; partial RU
  is never published or labelled successful. Opaque paths, IDs, symbols, URLs,
  packages, protocol
  identities, and exact technical spans are supplied through typed ownership
  and reversible object-local placeholders. Localization does not maintain a
  lexical allow/deny dictionary for words in human prose; unprotected prose is
  translated by the model as prose. It does not apply a blanket policy to
  translate or transliterate every Latin span.
- Cache contract changes use clean invalidation rather than replay adapters.
  `repomap cache clear [--debug-dir DIR]` removes only the known persistent
  model-research, component-synthesis, and localization cache directories;
  saved run artifacts remain untouched.
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
	  "evidence_refs": ["e0001"],
      "why_it_matters": "..."
    }
  ],
  "first_files_to_open": [
	{"file_ref": "f0001", "reason": "..."}
  ],
  "candidate_flows": [
    {
      "name": "runtime or event flow name",
      "flow_type": "request | operational",
      "trigger": "what starts this flow",
	  "likely_entrypoint_ref": "f0001",
	  "likely_file_refs": ["f0001"],
      "why_interesting": "...",
	  "evidence_refs": ["e0001"],
      "confidence": 0.0
    }
  ],
  "important_domain_words": [
    {
      "word": "term",
      "guess": "what it probably means",
	  "evidence_refs": ["e0001"]
    }
  ],
  "questions_for_human": [
    "question that helps guide next analysis step"
  ],
	"research_questions": [
	  {
		"id": "short id",
		"purpose": "why this matters",
		"question": "one bounded question",
		"candidate_file_refs": ["f0001"],
		"evidence_categories": ["declaration", "callsite"]
	  }
  ],
  "warnings": [
    "uncertainty or missing context"
  ]
}
```

The request uses Orientation prompt `orientation-json-v13`. Its compact wire
projection has one request-local `file_index`; each concrete model-visible path
appears there once. Candidate-file rows replace long canonical IDs and paths
with a `file_ref`, raw `allowed_paths` is not repeated, and signals, entrypoint
anchors/open files, command traces, orientation candidates, and import edges
carry inline file/evidence refs without restating their facts. This projection
does not reselect, shrink, or reorder the already bounded bundle.

Operational flows must cite bounded `source_signals` evidence through its exact
inline evidence ref. When the static
evidence remains weak and no local proof establishes execution, confidence is
capped at `0.3`. Request and operational flows remain one naturally ranked
candidate list.

Orientation response decoding is strict and typed. The provider returns only
the decision AST above; private catalog digests and backend contract versions
are not response fields. Unknown fields, unknown refs, wrong namespaces,
duplicate refs, prefixes, shortening, substitutions, and raw paths in ref
fields reject the response as a whole. There is no unique-prefix, regex/path
grammar, fuzzy/semantic repair, or entrypoint-from-first-file fallback on this
path. Provider prose is
non-authoritative and never parsed into evidence or navigation. Canonical paths,
evidence statements/locations, and research candidate IDs are restored locally
from exact refs before shared structural validation and downstream consumers;
the legacy evidence-path grammar is not run over those locally owned values.
A fact file outside `candidate_file_index` remains valid bounded navigation,
but has no candidate mapping and therefore cannot be selected for targeted
research. Canonical
`unverified_paths` remains empty because the provider contract has no such
field.

The cache fingerprint binds the exact provider request and a backend-owned
private catalog digest that includes the response contract and canonical
candidate-ID mapping. Equal provider-visible wire bytes with a different
private mapping therefore miss rather than replaying a stale response. The
provider never copies that digest or those versions.

The component `role` is a bounded orientation hypothesis used only to arrange
the browser landscape. Unsupported provider role literals reject the strict
typed response. Static package imports remain separate evidence and never
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
it deliberately does not pretend to score semantic truth. Historical prompt
experiments are retained in Git history rather than as production shell tools.

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
`reasoning_effort: "high"` and omits temperature. It uses the same exact global
output ceiling as every other semantic request; the historical development run
that truncated at 6,000 no longer creates a purpose-specific minimum.
Compatible endpoints receive the ordinary configured JSON-mode request. The
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
- Semantic responses still require their consumer-owned JSON contract. Focused symbol responses are parsed
  tolerantly and fail only when neither a JSON object nor tagged report can be recovered.
- No key is required for an ordinary `--offline` run. Live bearer calls require
  the key from the active namespace.

## Retry behavior

- The shared transport may replay the exact same request after retryable network
  errors, HTTP 429, or HTTP 5xx, using bounded exponential backoff with jitter
  and at most three retries.
- Context cancellation, non-429 HTTP 4xx, response-body overflow,
  `finish_reason=length`, malformed semantic content, and local semantic
  rejection are not retried.
- Transport replay never changes `max_tokens`, prompt content, request identity,
  or any other request byte. There is no semantic completion/proposal retry.

## Debugging

```bash
repomap ../etcd --debug-dir .repomap-runs --no-open --no-serve
```

The CLI prints the exact run directory near the start of the run. Inspect that
directory directly; there is no second debug wrapper.

Artifacts produced:
- `metadata.json` — run metadata (model, endpoint, command)
- `snapshot.json` — full local deterministic snapshot
- `repository_atlas.v1.json` — complete canonical local Repository Atlas
- `navigator_request.v1.json` — bounded request-local Navigator projection and backend catalog
- `semantic_exchanges/<id>/request.{json,txt}` — bounded redacted semantic request
- `semantic_exchanges/<id>/response.{json,txt}` — bounded redacted response, when available and safe
- `semantic_exchanges/<id>/response.marker.json` — closed unavailable or unsafe marker
- `semantic_exchanges/<id>/exchange.v1.json` — closed committed outcome metadata
- `navigator_status.v1.json` — closed Navigator state
- `navigator_result.v1.json` — accepted or empty canonical recommendation
- `report.json` — authoritative machine report
- `error.txt` — error message if any step failed

Never commit `.repomap-runs/`.
