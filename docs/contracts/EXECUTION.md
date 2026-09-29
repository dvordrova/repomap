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
  decoded-response ceiling and requests up to 128,000 output tokens unless its
  owning contract states a smaller measured allowance (glossary 32,768; parts
  and areas answers min(128,000, max(8,192, 16 × listed rows)); part and area
  descriptions 200; orientation 16,384, where 117 accepted answers used at
  most 1,260 output tokens, median 848). An allowance bounds a runaway answer, never evidence; an
  answer that reaches it is the ordinary output-token refusal, never truncated
  or partly accepted. A lower configured provider token ceiling remains
  authoritative. Composite input is
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
  worker pool, with the ordinary product limit set to twelve (owner decision
  2026-09-25; it was four). Preserve the
  caller's item index as the only in-memory result slot and replay observer
  events in that order; do not add a random batch identity to semantic or cache
  state. Every provider transport attempt acquires the run-shared adaptive
  gate. An HTTP 429 collapses that gate to one and pauses new attempts for at
  least one minute, honoring a longer Retry-After seconds/date value or a
  relative `retry after`/`reset after` duration in its error message. Later
  429s can extend but never shorten that shared cooldown. Already-started
  attempts finish while retries and new calls wait and then become serial.
  After cooldown, every four successes in the current gate epoch double
  concurrency from one, up to the configured twelve. A new 429 resets recovery, and older
  in-flight successes cannot shorten that new cooldown or restore concurrency.
  Cancellation interrupts the wait; other retryable failures retain their
  short backoff. An HTTP 200 answer with empty content, or with finish reason
  `insufficient_system_resource`, is the provider's fault and gets one
  transport retry of the same bytes (owner decision 2026-09-26). It counts as
  a transport attempt and in the call's token usage, keeps the short backoff
  and the caller's deadline, and a second such answer is refused as before.
  This adds no retry to an output cut, a context refusal, content that ends
  with another finish reason, a choice count other than one, or any decoder
  or validation refusal
  ([DeepSeek notes](../DEEPSEEK_API_NOTES.md#wire-and-failure-contract)).
  `ExecuteJSONBatch` fails closed: a terminal item
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
  whole-parent answer or replay that the current decoder accepts takes
  precedence. A semantic refusal creates this
  memo only for an owner that opts into `SplitRejectedResponse` (display
  translation). NoCache bypasses it and cache clear removes it. Existing run journals
  are not migrated into split memos.
  Failed model exchanges show their committed request/response/journal paths
  beside the error, with an explicit unavailable-body marker when necessary.
  The last attempt's HTTP status and diagnostic response IDs/retry/rate-limit
  headers accompany those diagnostics. Request authorization and cookies never
  enter that metadata, which has no role in semantic or cache identity.
- One exact request in the air is asked once. With the cache enabled, a call
  whose exact cache key (provider state and prepared bytes) another call on
  the same `BatchController` is already answering waits for that answer
  instead of calling the provider (`internal/llm/flight.go`). Targets of one
  repository send byte-identical requests at once: Redis 1.3.6 builds
  zmalloc.c and ae.c into the server and each tool, so their part descriptions
  were asked twice concurrently, drew different sentences, and the cache kept
  whichever landed last. The next run then read another description than the
  first had shown, and every areas, core, keys, orientation and glossary
  request quoting it missed (10 live calls on an otherwise warm rerun); with
  one call per request the second run made none and wrote the same report
  apart from its timing. The follower reads the leader's raw answer through
  its own limits, response adapter and decoder, like a cache record: it makes
  no provider call, counts and journals as cached and, like a cache hit,
  carries no HTTP status or response headers; those stay with the one
  exchange that made the call. A provider failure is
  shared as the same error, so an adaptive owner splits every copy alike; a
  refused answer is each follower's own refusal, neither cached nor asked
  again for it. A leader stopped by its own context leaves no answer, and its
  followers ask again. A recall-only call waits for a flight and then reads
  the cache. After the flight lands an identical call reads the accepted
  record as before, and a refused one is asked again, as in the next run.
  `--no-cache` keeps every call live and unshared.

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
- Models select only closed short refs already owned by their source artifacts:
  `t*`, `n*`, `e*`, `a*`, `h*`, `g*`, `o*`, `k*`, `x*`, and the sealed graph's
  file refs `f*` and the atlas's part refs `p*`. Cross-target refs
  qualify those identities rather than renumbering them. Only genuine
  request-local alternatives use `c*` refs. Catalog rows may show
  exact repository-relative paths, file names, symbol names/signatures, and
  dependency names because that context has semantic value. The model is
  never required to copy those values: UUIDs, canonical identities, ref
  resolution, and graph restoration remain local. Unknown refs have no
  authority and are discarded rather than guessed, repaired, or clarified;
  absolute host paths are never sent.

- Provider request bodies must never contain full repository source contents,
  unselected raw internal edges, digests, the LLM client's authentication
  credentials, or unadvertised paths.
  A complete names-only tracked-file dictionary is explicitly allowed for the
  README file-role classifier. The map of parts may send aggregates over the
  refs its request advertises: `calls` as `"f3 -> c7 (12)"`, each exact call
  site counted once per distinct other listed unit row it reaches (a whole
  file `f*` or a request-local box `c*`; calls resolved only to alternatives
  are left out), `imports` as `"f3 -> f7"` for an import the adapter resolves
  to one listed whole file, and the same call counts between parts as
  `"p3 -> p7 (12)"`. These are counts over advertised refs, not raw edges. A
  parts or areas request allows min(128,000, max(8,192, 16 × listed rows))
  output tokens; a part or area description 200.

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
  one short line or one closed choice, and every atlas table validates its
  rows separately: a missing, invalid or differently repeated known row loses
  only its own model answer, and a row repeated the same way is one answer.
  A cell its owner declares Alone (a caption, `open`, a target's role, an
  outbound line, destination or address, an alias) loses only itself on a
  missing, null, mistyped, empty or unlisted value or on differing copies; the
  owner reads it with the fallback the whole row takes, and the row keeps its
  other decisions unless a cell without Alone failed or no cell the model
  wrote survived (a value filled in for an absent cell, such as a missing
  address's `unknown`, is no answer). An
  optional cell with a bad value is likewise discarded alone. Refused cells
  are journaled as `cell_rejected`, and a row that lost a cell authorizes no
  glossary prose. Accepted neighbours keep their exact-response cache.
  Description and operation rows additionally retain their entity memos,
  recalled under the same rules. A decision model's memo basis pins what its
  questions ask beyond the serialized columns: each column's `Ask`, `Item`,
  options and their criteria (only for a table whose columns carry one of
  them, so the symbol selection keeps its basis); a remembered answer to a
  question worded otherwise is not recalled. A row the decision model
  answered uncertainly is remembered too, as that explicit undecided
  answer: a warm reading recalls it undecided and never asks again for a
  clearer draw. Independent arrows and target
  lines use the ordinary window cache. Aggregate architecture decisions use the
  same exact prepared-request cache and the owning closed-ref decoder for
  parts, merge and areas; there is no directory-assignment memo or browser
  validation step. Joint/peer decisions also retain their existing exact complete-row memos. Unknown
  keys are recorded and ignored; unused extra fields do not invalidate answers.
  A response without a rows array, or with no accepted row, is refused and
  not cached, and each of its rows' reasons is journaled.
- The closed tables (key declarations, part roles, keys, the role split's
  helper question, gate and assignment, and the outside symbols' roles:
  `Definition.Classifier`) go only to the run's
  categorizer
  (`llm.Categorizer`), never to the text model; there is no fallback
  (owner decision 2026-09-26). Jev is the only categorizer, and `JEV_KEY` is
  required like the model key: a model run or `read` without it stops before
  any artifact, corpus or model call, and a live reading without a
  categorizer is refused. `--no-model` needs neither key. The categorizer is
  an `llm.Provider` with `Prompt` (the exact request for keyed questions over
  one task and shared context) and `Verdicts` (the response read by question
  key; an unreadable verdict is absent, a response without verdicts is an
  error). The executor caches, journals and gates it like any provider, so
  its exact-cache keys are its `State` and the prepared bytes; the decision
  rule below, the question texts (which name Jev's `task`, `context.<field>`
  and `row`), request packing, concurrency and the one-token output limit
  stay with the table code and are sized for Jev. Another implementation
  would take over those Jev-specific limits when it exists.
- A Jev request follows the owner's shape (2026-09-26: a flaky decision
  means it was explained poorly): `state.task` says what we want, each
  question holds its item under the name the question uses (`row` unless a
  column's `Item` names it, such as `file` or `declaration`), and each
  option carries its criteria. A column's static `Criteria` are an object
  `{examples, includes, not_for, what}` (keys in sorted order); a column's
  `CriteriaFrom` takes an option's criteria from the text of that field of
  its catalogue entry, and the catalogue then reaches Jev only as the
  options and their criteria, never again in `state.context`. Two entries
  sharing a title are both offered, each by its ref with its own criteria.
  Any other option's criteria are its meaning or null, so the tables that
  existed before keep their exact request bytes (the Jev request golden).
- A decision model (Jev) answers a closed choice with a probability per
  option. Its choice is taken when the chosen option leads every other
  listed option, `none of these` included, by at least 0.10
  (`table.ClassifierMargin`; owner decision 2026-09-26, replacing an
  absolute 0.50 floor that refused `support` at 0.49 against 0.32 yet took
  0.51 against 0.49). A closer answer, or a choice that is not the top
  option, leaves its row explicitly uncertain, journaled with the runner-up;
  no code picks the runner-up or any other option instead. Probabilities
  are hundredths carried as floats, so a lead of exactly 0.10 counts. The
  margin comes from saved answers: the 60 saved role distributions and
  that 0.49 lead their runners-up by 0.00, 0.02, then 0.17, 0.21 and up,
  and 0.10 sits in the middle of that gap; on 1,072 saved eleven-option
  choices it decides 101 the floor refused and leaves uncertain the 9 the
  floor took with leads of 0.02 to 0.09. TypeSafe's guidance
  gates on its `confidence` (floors of 0.5 and 0.6 in its examples, set by
  the stakes and tested on one's own data); in the saved answers that
  confidence equals this lead for two options but is (n·top−1)/(n−1) for
  more, blind to the runner-up, so it is not used. A yes/no asked as a noul
  is one probability and keeps its band (yes at 0.6 or above, no at 0.4 or
  below); a yes/no choice with a cutoff (`YesAt`) decides every row at it;
  ranked nouls keep their probability; a yes/no choice without a cutoff
  follows the margin. A column marked `Alone` is a decision independent of
  its row's others: when it is not decided, only that cell is left
  unanswered and journaled, and the row keeps its other decisions unless
  none was decided. A window whose every row was answered, even
  uncertainly, is an explicit answer and is cached.

Unknown set members are removed; an unresolved mandatory scalar or conflicting known assignment is refused, without first-wins repair or a manufactured semantic complement.

A parts answer is not a coupled assignment: it is validated as independent
unit → part rows, a unit being a whole file (`f*`) or one box of a split
file (`c*`). An unknown ref is discarded, a unit named twice in one part is
kept once, a unit listed in two parts loses both memberships (no first
wins) and, like a unit left out, goes to one closed-choice placement
follow-up; a group without a name or without a listed unit is not drawn and
its units are left out. A group's list is `units`, or `files` (the form the
answers to the file-only request wrote): the two given alike are one list,
given differently they answer one row twice differently and refuse that
group alone. A group given twice with the same name, ignoring case, and the
same set of listed units is one answer and is drawn once; two groups that
differ in name or in units keep the rules above, so a unit both list is a
conflict. The list may also be one string of refs separated by spaces or
commas; each ref is still checked. Only an answer that draws no part is
refused whole: not JSON, no groups, or no group holding a listed unit of its
own. An areas answer follows the same rules at part level: a part in
two areas or in none stands alone, and an area given twice with the same name
and the same parts is drawn once. A refused answer, including one cut at the
output-token cap, is the window's refusal: it is not asked again, not
accepted in part and not cached. Only an input too large for the provider,
before sending or by its refusal of the input or context size, splits a parts
window.

Validation preserves unambiguous formatting variants before checking meaning.
Table choices with an advertised free-text tag normalize whitespace around its
colon: `other:Name` and `OTHER : Name` retain the written name as `other: Name`.
The requested tag and a nonempty value remain necessary; this does not infer a
closed choice or substitute a catalogue member. The original response stays in
the exact cache, so current decoding can recover previously refused formatting
without another provider request.

## JSON syntax and reasoning controls

The owner's 2026-09-09 syntax rule permits balancing JSON brackets before the
ordinary decoder: outside quoted strings, a closing bracket with no matching
open bracket anywhere in the active stack becomes whitespace; missing closing
brackets are appended at EOF. Whitespace prevents separate tokens from merging
(`1}2` must not become `12`). An unfinished quoted string may receive its
closing quote at EOF before those brackets, preserving its original contents,
including trailing spaces and literal bracket characters. Since 2026-09-25 a
crossed closer, whose opener lies deeper in the stack, has two readings:
deleting it, or closing the inner brackets before it. It is deleted only when
that gives one valid root and the other reading is invalid or decodes to the
same value, as for the journaled doubled `}` after a table's last row. Readings
that differ, such as `[{"a":[1}, 2]`, stay refused; past 64 readings the answer
is refused rather than searched. After the root closes, a tail with no `{`,
`}`, `[`, `]` or fence that does not start with `,`, `:` or `"` is discarded as
prose, and a repeated root that decodes to the same value counts once; a tail
such as `, "b": 2` continues the answer and is refused. An unfinished escape,
invalid string character,
missing value, a different second root, a structural tail or an ambiguous
closer is still refused; interior quotes and commas are never inserted and no
value is chosen from inside a malformed answer.
The same rule applies inside a fence and after complete thinking blocks. The
fence tag (`json`, `jsonc` or another) and an inline fence layout carry no
content. Prose may precede the fence, including brackets that close before it
outside any string and form no complete JSON object or array. A complete value
before the fence competes and is refused, and so does an unfinished one, or a
string the fence may belong to. After the closing fence prose is discarded, and a second fence must
repeat the same value. A string still open at the closing fence refuses any
following text, because the fence may belong to that string.
The entire resulting object or array must pass JSON decoding and the owning
stage's unchanged completeness, schema and closed-ref validation. This does not
override provider-reported truncation or resource limits. Raw responses in
cache and journals remain original; normalization needs no model call and does
not change request identity.

The shared JSON normalizer separates complete leading `<think>...</think>`
blocks before inspecting the final answer: consecutive blocks are removed in
turn, and a nested block ends at the `</think>` that balances its openers.
Code fences or draft JSON inside a block cannot become the answer. An
incomplete or unbalanced block, different final values or malformed final JSON
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
A balancing `</think>` terminator is required; a literal `<think/>`,
truncation or ambiguous final JSON is refused.

## Exact cache and replay

Reading an accepted answer sets its record and both payloads to the time of the read, so a file's modification time says when a run last used it; answers no run reads any more are found by age (owner, 2026-09-29).

The shared executor stores entity-to-response-row indexes in its existing
.llm-cache directory. Each memo contains only the request key and original row
key; the answer comes from the current shared response and is revalidated by
the owning table. Response tables are loaded once per request per reading.
Answer basis identity includes provider configuration, prompt/table contract
and the exact single-row evidence, but deliberately omits the artifact owner
ID. It does not replace that ID with a memo-local key. Thus identical evidence
may share one interpretation while every actual provider request and accepted
response retains the representative artifact's real ID. Repository labels,
native subject IDs, ownership and parent knowledge IDs do not affect answer reuse.
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
No-cache bypasses reusable answer reads and pointer/index writes. An accepted
answer's diagnostic payloads still go to the shared payload store. Cache clear
removes payloads, answer pointers and memo indexes; run snapshots and each
run's own payloads remain, so a wholly accepted exchange's raw-payload links
stop resolving while a refused or partly refused exchange stays readable.
Recalled rows are revalidated and are not rewritten.
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
prompt/input/request/response ref JSON, plus run-local normalized results.

Refused answers are kept for the developer and never in the cache, which
serves the user's next run (owner decision 2026-09-26). The shared store
receives a model exchange's payloads only when its answer was accepted, live
or from the cache. When the answer was refused (a decoder or validator
refusal, an envelope refusal, a provider failure or a cancellation), the
journal entry's request and response and the window's prompt, input, request
and response refs link the same content-hashed copies in the run's own
`payloads/` directory, with the same relative `file` shape. Cache clear leaves
them. A refused live answer never becomes a cache record, so it is never a
hit: the next run asks again. Identical bytes that an accepted record or an
accepted window also owns stay in the shared store for it. Replay prints the
shared request and response paths of an accepted answer, with duration,
attempts and usage. A refused replay stores nothing: its answer is on stdout
and its request is the given file.

Decoders refuse at the smallest scope, so most refusals are parts of an
accepted answer: a row, cell, member or term, or an annotation the atlas
reader records after validating it (a file in no part, a one-part area).
Each such `rejected.jsonl` row points at its exchange, which therefore keeps
its payloads both ways. Its accepted cache record, store payloads, key and
hits are unchanged. The journal entry and window refs of every run that reads
it, live, as a cache hit or through a memo, link that run's own
content-hashed copy instead, as for a refused answer. Each is decided where it
is written: a journal entry from its event's rejections, which become exactly
the rows pointing at it; a window from its outcome's rejections and from the
annotations its stage records before writing the refs. A wholly accepted
answer makes no copy. After cache clear every `response_ref` of a run leads
to bytes. A window whose call failed without any response has no response
ref, so its row names none; its journal entry records the response as
unavailable. One case stays uncovered: an answer its decoder accepted whole but
the atlas reader annotated keeps its window refs in the run, while its journal
entry, written before the annotation existed and named by no row, links the
store.

Cache reads distinguish proven corruption from operational failures. Invalid
JSON, identity/accounting, unsafe entries and missing referenced payloads may
evict an accepted pointer; an I/O failure, an observed inode replacement during
opening, or a stricter current response-byte limit is a diagnosed miss and
does not remove it. So is an answer the owning decoder or validator refuses
(`cache_validate`): it is not used and not evicted, an accepted live answer
replaces it, and a later, more tolerant decoder reads it without a provider
call. Only proven corruption evicts. A split memo yields only to a
whole-request answer the current decoder accepts, so a refused record never
costs another live call on the split request. A subsequent provider failure
therefore leaves a previously usable answer available for another run. This
classification does not make pathname eviction atomic against a later
concurrent writer.

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
