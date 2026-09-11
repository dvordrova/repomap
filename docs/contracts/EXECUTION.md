# Provider execution, response validation and cache

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Stage ownership and limits

- Model-assisted stages own `State`, complete input authority and its provider-sized
  request preparation, and semantic
  validation. The shared LLM layer owns exact provider requests, transport,
  retries, provider-envelope/JSON decoding, batch execution, cache, accounting,
  and semantic-journal events.
- Repository scale is not a correctness boundary and not a warning either:
  there are no size thresholds, no scale warnings and no page-size limits.
  Complete reservoirs are processed through as many deterministic disjoint
  provider batches and convergent closed-ref reduction rounds as necessary.
  Every stage uses the shared actual 32 MiB request envelope and 16 MiB
  decoded-response ceiling and requests up to 128,000 output tokens; a lower
  configured provider token ceiling remains authoritative. Composite input is
  repartitioned when its prepared request does not fit. A real provider
  envelope failure is terminal unless the owning stage defines a lossless
  repartition; it never authorizes truncation or partial publication. Only
  such a provider envelope, a representation overflow, canonical
  identity/path/format validation, or an explicit user narrowing option may
  remain terminal. Every printed console event starts with
  `[elapsed-seconds +seconds-since-previous-event]`; multiline details align
  underneath. Target pages, model-wait messages and terminal errors share the
  run clock. Suppressed events never advance the visible delta. Exchanges keep
  `latency_ms`, and a closing `Time` stage separates wall time from provider work.

## Parallelism and provider failures

- Execute independent stage-planned batch items through the shared bounded LLM
  worker pool, with the ordinary product limit set to four. Preserve the
  caller's item index as the only in-memory result slot and replay observer
  events in that order; do not add a random batch identity to semantic or cache
  state. Every provider transport attempt acquires the run-shared adaptive
  gate. An HTTP 429 collapses that gate to one and pauses new attempts for at
  least one minute, honoring a longer Retry-After seconds/date value or a
  relative `retry after`/`reset after` duration in its error message. Later
  429s can extend but never shorten that shared cooldown. Already-started
  attempts finish while retries and new calls wait and then become serial.
  After cooldown, four successes in the current gate epoch raise concurrency
  from one to two; four more restore four. A new 429 resets recovery, and older
  in-flight successes cannot shorten that new cooldown or restore concurrency.
  Cancellation interrupts the wait; other retryable failures retain their
  short backoff. `ExecuteJSONBatch` fails closed: a terminal item
  error cancels the batch child context, prevents queued items from starting,
  and the owning stage rejects the complete batch. `ExecuteJSONEach` is the
  table form: the same pool, gate and observer, but one window's failure
  leaves its neighbours untouched, its rows take their fallback line, and a
  wholly refused answer is written to `rejected.jsonl` and never cached.
  Partly accepted responses retain their original bytes; every reuse revalidates
  individual results and optional terminology follows only accepted output. Validation
  annotates and does not abort a run ([Constitution](../CONSTITUTION.md)); accepted sibling
  calls keep their identity-bound cache entries in both forms.
  Shared adaptive batches also memoize an actual context/output/response
  resource refusal when the owning stage can split the complete item. The memo
  binds exact prepared bytes, provider state and all request limits; it stores
  no child boundaries or answers. A later run rebuilds children through the
  current owner and validates their ordinary cached or live responses. A cached
  whole-parent answer/replay takes precedence. Semantic refusals never create
  this memo, NoCache bypasses it and cache clear removes it. Existing run journals
  are not migrated into split memos.
  Failed model exchanges show their committed request/response/journal paths
  beside the error, with an explicit unavailable-body marker when necessary.
  The last attempt's HTTP status and diagnostic response IDs/retry/rate-limit
  headers accompany those diagnostics. Request authorization and cookies never
  enter that metadata, which has no role in semantic or cache identity.

## Results and prompt ownership

- A model-assisted stage returns a fully validated result, a contractually
  legitimate empty result, or an error. Backend orchestration, report
  projection, and browser code must never supply semantic fallback, repair, promotion, or partial
  success for failed or incomplete stage output.
- Keep static prompt prose in readable Markdown beside its owning stage and
  compile it with `go:embed`. Go owns complete dynamic reservoirs and their
  provider-sized request partitions, not long prompt string literals; the
  provider layer does not own domain prompts or schemas.
  Analytical owners supply their computed result shape through `ResponseExample`;
  stage prose describes fields and decisions without a second output example or
  competing root-format instruction. Analytical responses use that direct owner
  shape; glossary work has a separate request and cannot add a `result/terms`
  wrapper. Table examples derive from the current `fill` columns and mode.
- Models select only closed request-local short refs. Catalog rows may show
  exact repository-relative paths, file names, symbol names/signatures, and
  dependency names because that context has semantic value. The model is
  never required to copy those values: UUIDs, canonical identities, ref
  resolution, and graph restoration remain local. Unknown refs have no
  authority and are discarded rather than guessed, repaired, or clarified;
  absolute host paths are never sent.

- Provider request bodies must never contain full repository source contents,
  raw internal edges, canonical internal IDs, the LLM client's authentication
  credentials, or unadvertised paths.
  A complete names-only tracked-file dictionary is explicitly allowed for the
  README file-role classifier.

## Independent validation

- Set-valued closed-ref responses are filtered to advertised refs and
  deduplicated locally; never require a model to echo each selected ref exactly
  once. Assignment rows keyed by an unknown ref are likewise discarded. A
  known prose ref paired with a valid non-documentation class is an unsupported
  set member and is discarded before its hypotheses or per-file bounds gain
  authority; it is never repaired or promoted into `documentation`. Go
  owns exhaustive batching, completeness, and identity. If filtering leaves a
  mandatory scalar choice or complete assignment unresolved, reject that
  incomplete result without inventing a replacement. An atlas table row asks
  one short line or one closed choice. Independent atlas rows validate separately:
  an invalid, missing or duplicate known row loses only its own model answer;
  accepted neighbours keep their exact-response cache. Description and operation
  rows additionally retain their entity memos. Independent arrows, target lines and zone choices/lines use the ordinary window cache. Joint/peer decisions also retain their existing exact complete-row memos. Unknown
  keys are recorded and ignored; unused extra fields do not invalidate answers.
  Unparseable envelopes and incomplete coupled assignments still refuse their
  window.

Only complete coupled assignments can establish their shared result. Unknown set members are removed; an unresolved mandatory scalar or conflicting known assignment is refused, without first-wins repair or a manufactured semantic complement.

## JSON syntax and reasoning controls

The owner's 2026-09-09 syntax rule permits balancing JSON brackets before the
ordinary decoder: outside quoted strings, a closing bracket with no matching
open bracket anywhere in the active stack becomes whitespace; missing closing
brackets are appended at EOF. Whitespace prevents separate tokens from merging
(`1}2` must not become `12`). An unfinished quoted string may receive its
closing quote at EOF before those brackets, preserving its original contents,
including trailing spaces and literal bracket characters. An unfinished escape,
invalid string character, missing value, crossed nesting, multiple roots or
trailing prose is still refused; interior quotes and commas are never inserted.
The same rule applies
inside the existing JSON fence and after the existing complete thinking block.
The entire resulting object or array must pass JSON decoding and the owning
stage's unchanged completeness, schema and closed-ref validation. This does not
override provider-reported truncation or resource limits. Raw responses in
cache and journals remain original; normalization needs no model call and does
not change request identity.

The shared JSON normalizer separates one complete leading
`<think>...</think>` block before inspecting the final answer. Code fences or
draft JSON inside that block cannot become the answer. An incomplete block,
nested/repeated leading blocks, multiple final values or malformed final JSON
remain rejected. The live outcome, observer and reused response retain all
original bytes; normalization changes only the input to semantic validation.
This compatibility handling does not itself turn thinking off at the provider.
The owner clarified on 2026-09-08 that the earlier default-off preference must
still honor a stage's explicit reasoning opt-in. Compatible endpoints now send
`chat_template_kwargs: {enable_thinking: true}` for shared question retrieval
and final answers, and `false` for ordinary fast stages and display translation.
`REPOMAP_LLM_CHAT_TEMPLATE_KWARGS` or its legacy
`DEEPSEEK_CHAT_TEMPLATE_KWARGS` alias explicitly replaces that object in full;
a forced `false` therefore overrides stage reasoning, and an empty object omits
the extension. No silent retry removes the control. The actual endpoint host
selects native DeepSeek handling: `api.deepseek.com` encodes the same stage
preference with its existing `thinking` control and no default kwargs.
The configured variable prefix does not classify the endpoint. `DEEPSEEK_*`
and `REPOMAP_LLM_*` configure the same client, with the generic namespace
authoritative whenever any of its settings is set. These request options take
part in exact cache identity, and replay does not reapply environment options.
The provider regressions cover both environment families, endpoint-host routing,
explicit on/off/omission overrides and unchanged client configuration. An HTTP
executor regression alternates fast/reasoning requests, parses a Qwen-style
leading think block containing a non-JSON code fence, retains the original
response bytes and reuses each mode only from its own exact-response cache.
Parser rules remain unchanged: a complete `</think>` terminator is required;
a literal `<think/>`, truncation or ambiguous final JSON is refused.

## Exact cache and replay

The shared executor stores entity-to-response-row indexes in its existing
.llm-cache directory. Each memo contains only the request key and original row
key; the answer comes from the current shared response and is revalidated by
the owning table. Response tables are loaded once per request per reading. Answer basis identity includes provider configuration, prompt/table
contract and the exact single-row model request. Repository labels, native
subject IDs, ownership and parent knowledge IDs do not affect answer reuse.
They instead contribute to the current knowledge ID together with the answer
basis and accepted cells. Every reuse rebuilds that binding: dependencies point
to this run's parent knowledge, not obsolete records. A parent's changed source
evidence can therefore produce a new provenance chain without model calls for
children receiving identical text. If its supplied model line changes, the
child's request and answer basis change too. Missing or changed inputs are
batched together. Missing rows with the same exact answer basis now share one
provider row within the reading. Every original subject still receives its own
current knowledge/owner/location binding and the same validated response-row
reference. This removes cold-run disagreements between identical inputs that
could otherwise change a warm run's recalled hint and invalidate question
retrieval. The regression covers distinct parent interpretations, rejected
aliases, changed batching, and replay updating all matching subjects. Exact
accepted request caching still applies to those batches.
No-cache bypasses reusable answer reads and pointer/index writes. Diagnostic
payloads still go to the shared cache directory. Cache clear removes payloads,
answer pointers and memo indexes; run snapshots remain, but raw-payload links
then stop resolving. Recalled rows are revalidated and are not rewritten.
An optional entity-memo write failure reports its stage and exact path without
discarding accepted cells or their current provenance. Each identical basis
gets one write attempt per reading; mandatory knowledge and window artifacts
still fail their owning persistence operation.

`repomap replay --file REQUEST.json [--debug-dir DIR]` sends the exact prepared
provider payload through the existing configured client, always live. It keeps
all saved request options, including unknown provider fields; the configured
client supplies endpoint, authentication, timeout and retry policy. Only a
successful provider response containing valid JSON replaces the answer pointer.
The owning stage checks its domain schema on later reuse. Replaying a batch
updates the rows recalled by entity indexes even if a later reading uses a
different batch size. Old run results and HTML remain snapshots.

The cache has one current accepted-record format, defined in [cache.go](../../internal/llm/cache.go). Records reference request and
response payloads stored by content hash. Semantic journals v3 retain per-run
accounting and relative links into this same store. Atlas tables likewise write
prompt/request/response ref JSON, plus run-local normalized results. Replay
prints the shared request and response paths, duration, attempts and usage.
Cache reads distinguish proven corruption from operational failures. Invalid
JSON, identity/accounting, unsafe entries and missing referenced payloads may
evict an accepted pointer; an I/O failure, an observed inode replacement during
opening, or a stricter current response-byte limit is a diagnosed miss and
does not remove it. Domain validation still rejects and evicts incompatible
answers. A subsequent provider failure therefore leaves a previously usable
answer available for another run. This classification does not make pathname
eviction atomic against a later concurrent writer.

- Persistent caches remain part of the ordinary path. Cache hits must be
  identity-bound and fully validated before use; `--no-cache` is the explicit
  live-provider bypass.

## Local response context

`ResponseContextProvider` derives compact original source/result-row scope, explicit prose paths and necessary text/prose fill descriptors from the owning Prompt. `Prepared` retains an immutable local copy. It is never appended to provider input, changes neither provider state nor exact request identity, and is not a process-global registry. The accepted cache record stores that context once beside its exact request reference; it stores no second full Prompt or evidence catalogue. Ordinary live/warm execution rebuilds current context. Entity memos read the original complete-window context. Question memos reconstruct and byte-check the full original call before adaptation. Exact replay replaces the response while preserving that request’s local context. Collection follows only accepted owning rows; current corpus authority is rechecked. Old formats have no migration reader.

## Transport diagnostics

Transport retry progress is independent of wait-heartbeat throttling. Each
retryable failure immediately prints its request digest, attempt number, closed
reason or HTTP status and planned minimum delay. A second event marks the actual
retry start after the shared gate permits it. Both use the ordinary run clock;
request, response and credential contents never enter these messages.

The owner subsequently supplied a compatible server's actual 429 message:
`rate limit exceeded: retry after 9.636307001s, reset after 45.636307001s`.
The adapter now reads both labelled relative durations from a plain error body
or its JSON `message`, string `error`, or `error.message`. The longest valid
body hint and Retry-After header feed that same minute-minimum shared cooldown.
The supplied example still waits one minute; a longer reset can extend it.
Negative, unitless or invalid values are ignored, the original error bytes
remain available unchanged, and non-429 responses keep their existing backoff.
The owner's reported 3,000 requests/minute, 300,000 tokens/minute and million-token
context describe their custom endpoint, not global product defaults. The current
gate controls concurrent attempts and server-requested cooldowns; it does not
preallocate a rolling token quota or equate context capacity with output length.

## Budget-change acceptance

Before a full repository run, any request-packing or output-budget change is checked on one complete saved window through the existing provider path. Record original and changed prepared bytes, scope, output/reasoning use, refusals and accepted rows. A successful probe is not ordinary full-report acceptance. [DEEPSEEK_API_NOTES](../DEEPSEEK_API_NOTES.md) owns endpoint transport details only.
