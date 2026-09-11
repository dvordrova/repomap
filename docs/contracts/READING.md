# Atlas reading, operations and questions

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Selection and captions

Symbol selection reviews key roles, activation and outgoing calls independently
of directory/file closure. It keeps insufficient activation evidence as
`unassessed`. Symbols and Types then describe only the selected keys displayed
by the existing per-file/per-box overview rules. Selection and caption retain
separate exact-input memos and knowledge records; refused prose cannot erase
accepted roles or Learn evidence. GroupsIndex projects those independent fields
even without a caption; operations never require a symbol description or key
selection to reach the report. Full original declarations remain available
to question retrieval, and question-only readings recall both records without
new description or selection requests.

- The atlas (`internal/atlas`) is the model path. `places` builds
  `places.json` from the program indexes, claims, facts and corpus: every
  directory and file, the declarations of a file with the first sentence of
  their docstrings, every callable and type symbol plus other declarations
  within the first ten ranked candidates per file, the boundaries
  (native routes, client calls, listeners, configuration reads and supported SDK source candidates), the
  file-to-file edges and the seeds. Native listener addresses keep the fixed
  `listen_address` kind; they remain available to reading and matching but do
  not become HTTP request operations. Native boundary places share only exact
  source observations: path, line, column, kind, method, literal values and
  compiler-located subject. Each place retains every original target's FactID
  and ObjectID behind local `origins`, sorted and deduplicated when sealed.
  A native place keeps only the observing targets that also hold its file, with
  their origins; an observation no holding target made is dropped before
  reading, and a target's projection names only boxes that target has.
  Target coverage must be complete and known target origins cannot conflict.
  Target atlas projection restores those original identities; it never borrows
  a sibling target's fact. Shared source context uses the original SubjectID
  before a target-scoped ObjectID. `reading` walks them in rounds and asks
  one keyed table per round: directories by depth, independent files with direct caller facts,
  symbols, boundaries, the parts of a target (one row asking for
  exactly `want` names, then every top box choosing from that closed list),
  the drawn arrows, the portfolio, the joints. A row carries the place's own facts and its directory's line, one step up.
  File callers contribute deterministic facts, never another file's model
  line. Description candidates include every callable and the first ten ranked
  declarations; additional types remain available to question reading. An
  accepted directory `open=no` can leave descendant directory/file rows unasked
  in exploration mode. Symbol selection still reviews its complete candidate
  set; caption requests follow the existing displayed-key selection, not an
  independent directory-closure veto. Missing or refused decisions do not close
  descendants. The complete graph and question evidence remain, and accepted
  activation evidence enters its independent operation review.
  Independent directory, file, callable, type, boundary and operation
  tables pack consecutive complete rows toward a 64 KiB default input size,
  without artificial 8/40-row caps. A larger complete row, including its shared
  context, runs alone; the default packing size never rejects its evidence.
  Explicit read-stage input and row budgets
  remain available. Other tables retain their owning context and round bounds.
  The actual prepared provider request still obeys the shared transport envelope.
  The model writes one line or one closed choice per
  cell; the code owns membership, arrows and their direction, joints by
  matched values, counts and identities. Rejected independent rows fall back on
  their own deterministic lines and are written to `rejected.jsonl`; valid
  neighbours survive in the original exact-response cache. An entirely refused
  window is never cached. A symbol row whose `activation` is missing or null settles as `unassessed`,
  the choice that already means no decision; a written choice is validated
  as before. A sequence cell citing only refs outside its row's options, or nothing at
  all (a provider may send null), is an empty selection and keeps the row's
  other cells; commas separate refs like spaces; exceeding the limit still
  refuses the cell. A response that answers exactly one row per asked row with no key on any
  of them is read in asked order; a partial or partly keyed response keeps the
  strict rule, every row names its key or is refused. Every row and answer is printed to `tables.md`, with prompts, requests, raw
  responses and normalized source-bound results under `tables/`. The ordinary
  path saves `reading-input.json` before its first atlas call; `read` consumes
  exactly that format and runs the same reader. Above two thousand files the directory and
  file rows carry an `open` cell and what the model closes keeps its
  fallback line.
  The directories/files owner prompts follow the complete request `fill`
  catalogue and demonstrate both modes. If requested, `open` is a mandatory
  `yes`/`no` string; prompts must not forbid it by prescribing only the base
  two cells. A missing choice still refuses only its own row.

## Operation ownership

The operation review sends immediate caller declarations once with
their distinct call sites; it does not join names or recursively expand callers.
It also retains literal arguments of the exact registration that receives each
callback, through the native argument identity. The operation table uses a
closed registration ref plus a method choice for an HTTP name; Go restores the
original path verbatim. Other work uses a descriptive label. Generated HTTP
path text is not a name cell in the HTTP branch. A declaration whose native
route already is its operation is not sent to the operation table: the route
fact carries the method and path, and the group index dropped the model's
duplicate anyway. An arrow without witnesses (an import-only edge) takes its
fallback sentence "A uses B." without a model row.

The current operation table asks only `self` or `none` for this declaration. Immediate caller declarations and distinct sites are evidence, never an assignment destination. `none` transfers nothing. Only a complete `self` decision publishes the declaration’s activation, name and description. A real independently launched notification/metrics consumer may be `self` while its AddLogHook/PreRun/constructor/lifespan launcher is `none`. Synchronous helpers within the same responsibility are not separate work. Listener blocking alone is not a worker. Cron, persistent consumers and source-supported one-shot delayed work remain legitimate. Registration, callback and control evidence are interpreted by the model; projection never invents a semantic promotion. Native HTTP registration refs restore their original path and method verbatim; free text does not replace a known route. A `label` row whose `name` comes back empty, null or missing keeps the model's own `description` as its label (first sentence, at most 60 runes) and records `name_from: description`; an empty description still refuses the row.

Operation v19 requires evidence of both the task and its independent activation
in the existing `entry` decision. Process entry, asynchronous launch or staying
alive alone does not establish a task; starting or dispatching the host runtime
is `none`. Delegating a supported task to helpers remains legitimate.
It preserves each declaration's own native calls with receiver,
arguments, result origins, API, exact sites and dispatch uncertainty through the
existing source catalogue. It does not recursively copy callees. Middleware
continuing the same request is a step within its incoming operation, including
authentication, header mutation and error handling; an independently activated
endpoint or consumer can still own its work while calling helpers. Source
receiver context distinguishes request-header mutation from response-header
mutation; a shared API name alone does not establish the affected object. These
remain model decisions with `self`/`none`, not local middleware classification.

## Boundaries

- Outgoing call selections are candidates for the existing boundaries table,
  not accepted integrations. That table records the other runtime participant,
  purpose, dispatch or client-configuration basis, and a closed original address
  ref or unknown. Internal delegation, local mechanisms and package membership
  do not themselves establish external communication. A call's observed indexed
  callee candidate alone is neutral resolution evidence. A `calls` observation
  with complete exact resolution to one repository callee and no external API
  is internal delegation at that site: it stays in source context, but is not
  advertised as an outgoing choice. Possible/unresolved and external API calls
  remain eligible. Original c-ref positions and native boundary facts survive.
  Accepted communication retains its exact call site and reaches GroupsIndex
  independently of the containing group's lane or key descriptions. The entrance
  shows those observations and source links, not a count of unique remote systems.
  Native HTTP addresses survive missing or refused prose. Standard-library
  transports may establish communication; this does not promote their package
  objects into remote participants. No package blacklist or API handbook is added.

A row whose only address option is `unknown` accepts any address answer as `unknown`: nothing else can be chosen there, and the model tends to copy the observed path into that cell (27 Freqtrade rows were refused for it). Fixed native boundaries request explanation rather than pointless existence/kind choices. Fixed outgoing facts additionally request a destination and closed original address ref; their native kind and dispatch basis cannot be changed by model cells. Refused prose preserves the native fact. Candidate runtime communication still requires the existing accepted semantic decision. A selected observation must establish the external mechanism or explicit remote configuration; internal delegation is evidence of delegation. Boundary source context carries each original call site to its owning declaration, including safe receiver/source arguments, native API and same-line columns. Calls remain individually anchored; grouping by a shared name or counting Do sites cannot establish the number of systems. The address catalogue lists only literals that can be addresses (no format templates, nothing from formatting, logging, time or string packages) and is sent only for outgoing rows whose address the code does not know; `destination` is a closed choice from the shared known-systems list (`internal/atlas/destinations`) annotated with the target's dependencies, with `other: ` for a system outside it; an owner's calls near the line and its source context are sent once per window and rows reference them. A native outbound fact without a column claims every selected call on its path and line, so the same call is not reviewed a second time as a candidate; a fact with a known column claims only that call.

Boundary v5 names the candidate basis `dispatch` or `remote_client_instance`.
The latter requires this call itself to create or configure the actual remote
client/exporter instance; returning an option for a later constructor is not a
separate relationship. The stored atlas basis remains `configuration` through
an explicit owning projection. Negative rows acquire no purpose, destination or
address from their unused response cells. No API-name allowlist or later
semantic repair enforces this distinction.

## Type and concept descriptions

- Type descriptions use the existing symbol stage and symbol knowledge, with
  the active `lines/prompts/types.md` prompt embedded by `lines/tables.go`.
  The atlas graph and saved reading input retain a type's declarations
  through exact native owner IDs, including cross-file methods and explicit Go
  interface method declarations. Interface declarations have no invented
  direct-call node. Type context retains the existing bounded author quotes,
  including later sentences; ordinary file/callable rows still use first
  sentences. The type table asks for a short explanation,
  optional English alias and key flag; it cannot classify activations. Its explanation is prose, so
  normalization preserves complete sentences and qualifications rather than
  cutting them at 240 characters. Undocumented lifecycle topics are omitted
  instead of appending irrelevant absence claims. Bare names without owned
  declarations or author documentation stay in the source index without an
  invented definition. A file's model hypothesis is not type evidence. Ordinary
  callable rows keep their own contract and memo identity. Concept explanations
  on maps are a projection of existing interpreted type subjects and also seed
  the shared glossary without new definition calls or a second semantic graph.
  Effects implemented elsewhere still require their
  actual source contract; method ownership alone does not establish them.

Symbols and Types may also return one short English `alias` beside their
explanation, through the same existing table request. It is an optional
reader-facing label for a name that needs explaining, especially a non-English
identifier; `none` records no alias. Accepted aliases flow through symbol
knowledge and the atlas into GroupsIndex interpretations. Native names, IDs and
source anchors remain unchanged. The report keeps aliases English in every
language, displays the original code name beside them, and localizes the
description. Both literal spellings address the same glossary definition;
no script detector, transliteration, per-name request or browser-generated label
is added. Current format constants are linked from [CURRENT](../agent-room/CURRENT.md#formats).

Names, signatures and argument names are useful clues for model hypotheses; absence of comments must not prevent orientation. Keep those hypotheses distinct from observed actions and effects established by native caller evidence. Earlier source-body/context experiments are development measurements, not authorization for implementation-body retrieval in ordinary requests.

## Questions and Learn

`--question TEXT` is repeatable and supplements local `.repomap.conf` questions.
Settings are loaded once and passed as a typed value through target work and
serving. Questions share the graph, recalled descriptions and provider input;
each keeps independent source selections and question-keyed request references. Learn links questions
to short source-anchored answers; supporting reading routes remain collapsed.
The flag adds shared retrieval and a shared final-answer batch after the ordinary
atlas; on `read` it runs those stages without the ordinary atlas.
`--through question` stops after retrieval. `--through answer` changes the final
answer contract while reusing unchanged retrieval. The separate `route` stage
has been removed; requesting it returns migration guidance. `question-routes.json`
stores the current reading records, retaining every candidate and its original
evidence. The answer batch reads the complete union of those original sources
once, with each question's own allowed refs and retrieval coverage. Each accepted
row returns an answer, its basis, sources in useful reading order, and a specific
remaining gap. The supporting guide projects those ordered sources; it makes no
separate selection and requests no extra open question.
Questions and source records are canonically ordered for exact-request reuse;
results retain the user's question order. Adding a question changes its answer
window and regenerates that window. There is no per-question answer memo.
The ordinary attempt has no question quota or 64 KiB planning cap. Actual
prepared-input, context, output or response-envelope refusals split questions
first and rebuild each child's complete evidence union. Only a singleton question
whose complete evidence does not fit partitions its original sources into
separate answer parts. Each question validates independently: a malformed,
missing or duplicate answer leaves that question unavailable while accepted
neighbours survive. An unparseable response, a failed provider call or a
response with no accepted row on a window of several questions divides the
questions the same way a resource refusal does, rebuilding each child's complete
evidence, until a request holds one question; that question's refusal is its
unavailable answer. The refused attempt stays in the journal as superseded;
nothing is repaired or retried with the same bytes. Explicit read-stage
development budgets still apply.
Final answer prose preserves paragraphs and complete qualifications; only
short label cells are whitespace-collapsed or length-trimmed. Source checks
keep owned declarations together, each with its original code link.
The shared question retrieval and final answer opt into provider-supported reasoning.
Learn proposals, question retrieval and final answers use the shared
128,000-token output allowance, also used by separate glossary generation and reduction. A lower
configured provider ceiling still applies; reasoning and visible output share
it. Complete input is prepared first, with lossless
partitioning on actual provider preparation/resource refusals. The DeepSeek adapter encodes that preference on its
official endpoint. Other compatible endpoints encode the same stage preference
in `chat_template_kwargs.enable_thinking`: shared question
retrieval and final answers enable it; other stages, including translation,
keep it disabled. The owner clarified this stage-specific behavior on 2026-09-08.
`REPOMAP_LLM_CHAT_TEMPLATE_KWARGS` (or the legacy
`DEEPSEEK_CHAT_TEMPLATE_KWARGS`) replaces that object; `{}` omits the extension.
The actual endpoint host selects native DeepSeek controls, never the variable
prefix. Exact request
and memo identities distinguish the preference. Other atlas tables keep their
existing fast mode. The final answer reads the original question and evidence. Required unfamiliar names are briefly
explained at first use. Completeness compares the original question with the
answer at its requested level: silent omission of a central part is partial;
an overview does not require unasked implementation details. Behavior supported
only by author documentation is attributed in the answer itself; a separate
basis sentence does not turn an unconditional implementation claim into a
documented one. A documented responsibility alone does not prove its implementation.
Retrieval and answers use the same worker distinction as operation review:
hosting or dispatching an HTTP runtime alone is not a task with its own
persistent or scheduled responsibility. Answers preserve each supplied call's
receiver and argument associations, including exceptions between similar calls;
a shared client or setting does not establish uniform behavior.
The answer's existing basis is visible directly below its prose, with the same
display binding and shared source control; original source checks remain collapsed.
Useful deductions are welcome, including from names and signatures; they stay
recognizable as interpretations and can be checked beside the original excerpts.
Answered, partial, unanswered, not-applicable and
unavailable remain distinct; incomplete evidence cannot establish inapplicability.
If selected evidence needs multiple windows, their original anchors survive in
separate partial answer parts. No body retrieval or new semantic graph is added.
The ordinary reading adapts eight base learning intents after the atlas using
the configured client, then answers its proposals through the same question and answer stages. Proposal preparation starts with the complete original
learning evidence and all eight intents, using the actual provider request
envelope rather than an ordinary 64KiB fragment budget. Each proposal request
encodes repeated component/area context once behind local refs while retaining
every evidence item's complete original observations. Partitions rebuild their
own context catalogue without relying on a parent or sibling window. Explicit development
budgets remain available. Actual context/output/response resource refusals
partition complete original evidence by encoded byte weight; accepted sibling
reviews survive, children retain their partial-context scope, and failed parents
supply no semantic review. Proposal and menu decisions validate per intent;
a refused intent remains unavailable without deleting its neighbours. Within a
`questions` review each proposed question validates alone: one that fails a rule
(blank wording or why, no or only unadvertised sources) is dropped with a
`question_rejected` journal row and the review keeps the rest; a review is refused
only when none survive. A `questions` review that arrives without a reason takes
the first sentence of its first accepted question's why and records
`reason_from: why`; `not_applicable` and `unknown` reviews still need their own
reason. An intent with no entry at all in an accepted response is re-asked over the
same evidence — first together with the other omitted intents, then alone —
before it is unavailable (`intent_omitted` journal rows, `recovered` when a
later window reviewed it); the intents are listed in the request after the
evidence so every re-ask shares its parent's request prefix. After all
windows, an intent reviewed by any window is not marked unavailable by the
windows that skipped it. Failed
consolidation preserves original accepted questions and any accepted comparisons,
with the plan marked partial. The owner explicitly approved this for user-selected
repositories on 2026-09-06. `read --through learn` stops after the plan.
`learning-plan.json` retains each context review and every proposal's original
intent, reason and sources. Each intent's menu selects at most five of its candidates; the merge step
groups the selected questions by information need in one call per pool
(`groups` of members with a representative), an unplaced question staying its
own group and a refused window keeping every question.
Overlapping automatic questions share one answer;
explicit questions remain visible. Exact duplicate explicit/generated wording
shares a result carrying both origins. Questions expose their selection reasons
and original excerpts. Answers offer existing term explanations when they
selected that exact named declaration, with links to the term's map memberships.
Generated declarations participate.
Automatic proposal selection composes the base-intent menus together through
the same executor. Each intent has one mandatory set-valued decision over its
closed candidate refs; all rows share the original candidate catalogue and read
their full curated learning goals. Selected and unselected questions retain
their original sources and the menu rationale; an unselected question is not
declared inapplicable or individually judged unsuitable. Explicit questions
bypass this selection. Context partitions reduce without a question quota.
Decisions made before
the candidates fit together retain their partial-comparison scope, including a
nonshrinking fixed point; the report does not imply a whole-menu comparison.
Stage prompt overrides
do not replace the separate selection and consolidation contracts. Large-repository
menu quality remains under acceptance review; a technically valid plan is not
by itself evidence of a useful introduction. [CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work) owns ordinary report and answer acceptance; earlier experiments remain in the archive.
Go test declarations also enter the same index and question rows. Existing
build-selected go-list test inventories supply parsed declarations with exact
locations, including external tests, private packages and test-only directories.
They belong to their module component (or their own executable package), never
become production API or launch seeds, and currently carry no test call graph,
execution result or assertion semantics.
The shared question cube selects closed source anchors for every question
from one evidence catalogue. Complete evidence precedes the changing questions;
unselected rows need no separate negative explanation. Every question is
mandatory in a response and retains per-chunk inspection coverage. A refused
question stays unavailable for that chunk rather than becoming a negative finding;
accepted neighbouring questions survive, including cache and memo reuse. Only explicit
provider context/output/response resource refusals authorize lossless partition
of complete evidence rows; there is no ordinary row-count or 64KiB planning cap
for this cube. A window asks at most eight questions over its complete rows:
the saved Freqtrade window that asked 64 questions over 460 code rows (4,661
anchors) got four back, the same rows with eight questions got eight, and the
same 64 questions over document rows got 64 — density of decisions per response,
not key names, loses answers. Windows over the same rows share a request prefix;
the first of them runs before its siblings so the provider's prefix cache serves
the rest. A question the model left out of an accepted response is asked once
more over the same rows with the other omitted questions (`question_omitted`
journal rows, marked recovered when the second round answers); a question
omitted twice stays unavailable. Output refusals split independent questions
first while retaining their complete evidence; input/context refusals split by
actual encoded input weight. Explicit development budgets remain available.
Per-question memos store references to the original shared response, with the
input metadata needed to reconstruct and compare its exact prepared request.
Replay is revalidated against that complete original window before reuse.
Adding or reordering questions reuses existing decisions; canonical ownership
is restored from the current graph. This implementation is under ordinary
quality acceptance; its measured drafts and limitations are in CURRENT.md.
Symbol selection asks `key_symbol` and `operation_candidate` (yes/no; the
operation table decides the kind) plus `outbound` call refs; rendered calls
leave out the default `invocation`/`resolution`, collapse exact local callees
into `local_calls`, and render origin trees two levels deep without anchors.
Every rendered evidence value is defined in the evidence vocabulary attached
to the symbols, operations and boundaries prompts; a contract test fails on a
value the prompt does not name.
A retrieval row states each fact once: a unit's `heading_path` lists only its
parent section titles, `anchor_path` appears only when it differs from the
row's path, and a type member that is itself a unit of the same chunk is
referenced by its `a*` ref instead of copied; the stored evidence restores the
complete record.
Selected type anchors keep their native owned declarations and exact member
locations, also shown under the answer's source checks. Types beyond the
description-candidate budget remain available for question reading.
Every selected anchor retains its own subject and original evidence. Its row's
reason is a shared relevance hint, not a separate proof about each declaration.
The reader uses the same sealed in-memory graph that SaveInput persists, so
ordinary and saved readings bind their question results to the same graph hash.
Connections retain their source kind; the route is not an execution trace.

## Question evidence and per-anchor relevance

Question-batch v3 validates relevance for each original `(row, anchor)`. Distinct anchors in one chunk may be direct/context independently; different rationales are retained in stable order. Conflicting relevance for the same anchor still refuses that question, with accepted neighbours intact. Each selected declaration retains its native call/API facts, safe source arguments, receiver/result expressions and source-qualified ownership. `BoundarySourceContext` and question call catalogues use the same safe source projection; canonical IDs stay local. A main-flow or business-effect claim must follow actual observations or remain an explicitly qualified interpretation. A source path is not proof of a call, and its reading order is not execution order.

## Entity knowledge

The owner wants descriptions and discoveries to accumulate on internal
entities and serve both later model cubes and people. A function's general
behavior can be reused across callers; the purpose and arguments of one call
belong to that call's context. Do not re-read a shared lower-level subtree just
because another higher-level caller reaches it. Keep source facts and model
interpretations distinct and remember which evidence and earlier interpretations
each conclusion depends on, so the affected knowledge can be invalidated when
its basis changes. This should remain a small record boundary, not a large
taxonomy or another plugin system.

SubjectID on a question stop is distinct from its context PlaceID, with original
selected evidence retained. Native declarations use their ProgramIndex object
IDs; boundaries and observed entities use existing graph IDs. Context fields
retain their actual context path, so generated-file attributes cannot silently
describe a configuration anchor. Internal IDs are restored locally, never sent
to the model.

Independent directory, file, symbol, operation and boundary interpretations persist
as knowledge.json v2. Each record binds the internal subject, current owners, context, exact
single-row evidence, cells and dependencies on earlier model interpretations.
The model still receives batches; single-row preparation only computes identity.
Only these independent tables opt into reuse and their prompts explicitly require
row independence. Comparative stages keep complete request identity. Existing provider exchanges retain real transport accounting;
reused entities are counted as reused_rows, not fictional provider cache hits.

Question-only readings use the same independent row builders in recall-only mode:
they restore current descriptions but make no description requests. Source rows
receive explicitly labelled prior model hypotheses; stops retain used knowledge
IDs. This is reuse of extracted-evidence descriptions, not implementation
analysis. Functions still cover selected declarations using names, signatures
and docs. An uninspected body change does not invalidate those descriptions.
Changed target inventories may reuse byte-identical row inputs while producing
new entity bindings. This is exact input reuse, not fuzzy identity matching;
there are no old-format readers or cache migration paths.

## Matching and operation paths

Matching retains native imports, calls and callback transfers separately from
model-confirmed integration hypotheses. Every peer window is considered rather
than silently selecting a first candidate subset. Rows share a peer dictionary
only when they have identical eligible counterparts: the same source object,
same-component-only counterparts and incompatible fixtures never appear as
choices in a request that forbids them. Shared dictionaries remain exhaustive over admissible peers. Inferred connections retain
their original joint identity, source/destination subjects and exact anchors,
including distinct calls at one source line. The ordinary report projection
keeps this information in GroupsIndex, with no legacy reader. Blind matching now compares each peer window's winners again against
the original evidence until one counterpart or none remains per outgoing
boundary. It no longer publishes one independent winner from every window.
Peer eligibility is partitioned inside each bounded window to keep unrelated
outgoing rows batched. Candidate declarations retain exact signatures and
comments, including streaming result types; protocol similarity alone is not
the same operation. No Cobra-specific detector or parallel graph has been introduced.

The operation map follows native calls and executions from the selected
subject, with cycle protection, and projects the visited subjects to groups.
Imports and the act of supplying a callback or service object do not become
execution paths. This currently also leaves an open path where an external
framework invokes a supplied callback without a separately observed call;
the binding stays in the underlying graph. A model-matched endpoint is included in
an operation's path only if its caller is reached; otherwise it remains visible
on the caller's group. Cross-component endpoints link to the matching operation
where its anchor is known. These are possible static paths, not a runtime trace;
unresolved dispatch and model integrations use dashed lines.

Independent joint protocol decisions additionally use exact row memos keyed by the complete boundary, eligible peer catalogue and target context. The memo restores and revalidates its original response row; equal labels alone do not establish equivalent inputs or a match.

## Orientation

`orientation` is one model-assisted stage over facts, claims, and the complete
matched GroupsIndex set. It returns one repository summary, one role per
target, a run recipe, and one main flow. The model selects request-local refs
that Go restores to exact fact, claim, and subject ids. Validation is a pure
function over the response and the advertised catalog. Set refs filter unknown
or incompatible members and deduplicate repeats, recording ignored refs; a row
with no required evidence, a recipe step with no manifest or entrypoint evidence,
or a flow step naming another target's member is rejected with its raw JSON and
a reason into `rejected.jsonl`. Summary, roles, recipe and flow validate separately,
so a wrong field type does not discard accepted sections. Equivalent target
roles combine their evidence; conflicting roles leave only that target's role
unavailable. Complete prose and qualifications survive; only short labels collapse
whitespace. Unparseable responses yield the legitimate empty orientation and are
not cached. Optional terms follow accepted sections and rows on live and cached
responses. Rejection never aborts the run and no replacement is invented.
Preparation keeps complete author claims and all validated group members with their native evidence against the actual provider envelope; no first-twelve-members or short-claim slice substitutes for the complete input.
the former 2 MiB limit and its size-triggered claim/member/fact removal are gone.
The stage caches on its stage identity, prompt version, and input digests through
the shared executor.

The orientation request includes source-backed member evidence, not only accepted captions. Original declaration kinds, calls, source arguments and owned fields can qualify member behavior. A launcher/router, implementation mechanism, callback and remote destination remain distinct roles. Parent heading context labels the scope of author claims. No missing source relation is supplied by a prose explanation.
A launch fact supports an entry point, not a complete invocation. Run recipes
check supplied member observations and author instructions for required
arguments and prerequisites; known required values may use explicit placeholders.
Unsupported invocations remain absent rather than losing those requirements.

## Optimization acceptance

Follow [Development: evidence before optimization](DEVELOPMENT.md#evidence-before-optimization). Missing a caption, fewer selected symbols or a smaller request count alone does not prove that reader outcomes survive.
