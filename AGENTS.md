# AGENTS.md

## Product

`repomap` is an online, model-assisted repository orientation tool. Its
supported user-facing surface is deliberately small:

- `repomap [repository] [flags]` runs the ordinary analysis and publishes the
  report artifacts.
- `repomap conf [repository]` creates the local `.repomap.conf` if absent and
  opens it using its editor setting. This command performs no analysis.
- `repomap replay --file REQUEST.json [--debug-dir DIR]` resends exact saved
  provider bytes through the configured client and refreshes its cached answer.
- `repomap cache clear [--debug-dir DIR]` clears persistent model-response
  caches.

`repomap read READING_INPUT.json [--through STAGE] [flags]` runs the same
atlas reading stages from the current saved input format for development.
It can override one stage's prompt and context budgets, saves inputs and
normalized results, and never renders HTML. No old-format adapters or
parallel analysis implementations are supported. There is no separate
`serve` subcommand. Report serving remains part of the ordinary run and is
controlled by `--no-serve` and `--port`. `--no-model` walks the atlas without
a provider, every cell on its fallback line and no orientation, and needs
`--target` because no target is selected without the model. Do not add
script entrypoints or sidecar tools.

`--question TEXT` is repeatable and supplements local `.repomap.conf` questions.
Settings are loaded once and passed as a typed value through target work and
serving. Questions share the graph, recalled descriptions and provider input;
each keeps independent source selections and question-keyed request references. Learn links questions
to short source-anchored answers; supporting reading routes remain collapsed.
The flag adds retrieval, route selection and an answer after the ordinary atlas;
on `read` it runs those stages without the ordinary atlas. `--through question`
stops before route selection; `--through route` edits the selector alone
while unchanged retrieval is cached; `--through answer` changes only the final
answer with unchanged retrieval and route inputs reused. `question-routes.json`
v1 stores a list of v7 routes. Each retains every
candidate and its original evidence, plus a short ordered list of source locations
and an open question. Route table v7 does not request a redundant summary;
selected locations retain their original reasons. Answer table v6 reads the
selected original evidence with labelled prior model interpretations and returns
a brief explanation, its basis, source refs and a specific remaining gap.
Final answer prose preserves paragraphs and complete qualifications; only
short label cells are whitespace-collapsed or length-trimmed. Source checks
keep owned declarations together, each with its original code link.
The shared question retrieval and final answer opt into provider-supported reasoning. Tables
have no individual output-token ceilings: they use the shared 128,000-token
request envelope, and the configured provider ceiling still applies. Reasoning
and visible output share that allowance; concise prose is a prompt requirement,
not a smaller generation cutoff. The DeepSeek adapter encodes that preference on its
official endpoint. Other compatible endpoints encode the same stage preference
in `chat_template_kwargs.enable_thinking`: shared question
retrieval and final answers enable it; other stages, including translation,
keep it disabled. The owner clarified this stage-specific behavior on 2026-09-08.
`REPOMAP_LLM_CHAT_TEMPLATE_KWARGS` (or the legacy
`DEEPSEEK_CHAT_TEMPLATE_KWARGS`) replaces that object; `{}` omits the extension.
The actual endpoint host selects native DeepSeek controls, never the variable
prefix. Exact request
and memo identities distinguish the preference. Other atlas tables keep their
existing fast mode. The final answer reads the original question and evidence;
the route's open question remains in its supporting reading and does not become
an extra requirement for the answer. Required unfamiliar names are briefly
explained at first use. Completeness compares the original question with the
answer at its requested level: silent omission of a central part is partial;
an overview does not require unasked implementation details.
Useful deductions are welcome, including from names and signatures; they stay
recognizable as interpretations and can be checked beside the original excerpts.
Answered, partial, unanswered, not-applicable and
unavailable remain distinct; incomplete evidence cannot establish inapplicability.
If selected evidence needs multiple windows, their original anchors survive in
separate partial answer parts. No body retrieval or new semantic graph is added.
The ordinary reading adapts eight base learning intents after the atlas using
the configured client, then answers its proposals through the same question,
route and answer stages. Proposal preparation starts with the complete original
learning evidence and all eight intents, using the actual provider request
envelope rather than an ordinary 64KiB fragment budget. Explicit development
budgets remain available. Actual context/output/response resource refusals
partition complete original evidence by encoded byte weight; accepted sibling
reviews survive, children retain their partial-context scope, and failed parents
supply no semantic review. The owner explicitly approved this for user-selected
repositories on 2026-09-06. `read --through learn` stops after the plan.
`learning-plan.json` retains each context review and every proposal's original
intent, reason and sources. Overlapping automatic questions share one answer;
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
by itself evidence of a useful introduction. The full saved etcd check now
selects 22 questions from 630 original candidates, covering all eight goals.
Ordinary report and answer quality still need acceptance. Prompt-only merge
changes, an extra explanation cell and a reasoning-mode replay did not solve
the earlier oversized menu; those experiments are not ordinary-path changes.
Go test declarations also enter the same index and question rows. Existing
build-selected go-list test inventories supply parsed declarations with exact
locations, including external tests, private packages and test-only directories.
They belong to their module component (or their own executable package), never
become production API or launch seeds, and currently carry no test call graph,
execution result or assertion semantics.
The shared question cube v1 selects closed source anchors for every question
from one evidence catalogue. Complete evidence precedes the changing questions;
unselected rows need no separate negative explanation. Every question is
mandatory in a response and retains per-chunk inspection coverage. Refused
windows stay unavailable rather than becoming negative findings. Only explicit
provider context/output/response resource refusals authorize lossless partition
of complete evidence rows or questions; there is no ordinary row-count or 64KiB
planning cap for this cube. Output refusals split independent questions first
while retaining their complete evidence; input/context refusals split by actual
encoded input weight. Explicit development budgets remain available.
Per-question memos store references to the original shared response, with the
input metadata needed to reconstruct and compare its exact prepared request.
Replay is revalidated against that complete original window before reuse.
Adding or reordering questions reuses existing decisions; canonical ownership
is restored from the current graph. This implementation is under ordinary
quality acceptance; its measured drafts and limitations are in CURRENT.md.
Selected type anchors keep their native owned declarations and exact member
locations, also shown under the answer's source checks. Types beyond the
description-candidate budget remain available for question reading.
Every selected anchor retains its own subject and original evidence. Its row's
reason is a shared relevance hint, not a separate proof about each declaration.
The reader uses the same sealed in-memory graph that SaveInput persists, so
ordinary and saved readings bind their question results to the same graph hash.
Connections retain their source kind; the route is not an execution trace.
The selector aims for six stops; extra valid anchors remain valid in every
round. Pools partition complete original evidence by the configured input-byte
budget, with no candidate-count ceiling. A round either reduces the source
set or finishes with independently ordered reading parts; a fixed point never
retries unchanged evidence, invents a global ranking, or discards sources to
force convergence. Their exact union feeds the answer stage. The facts stage runs built-in sqlc and configured external commands through
the same nodes/links contract in docs/EXTRACTORS.md (facts format v2).
Extensions supply source observations, not architecture role assignments.
These rows enter the same places graph and question table, with producer
declarations and corpus membership distinguished from compiler call edges.
Graph and saved reading input are v9; there are no old-format readers.
Observed entrypoint seeds and manifest values from the existing facts result
also enter this graph, with their exact source and component context. They are
available to both Learn proposals and question retrieval without requiring a
key-symbol interpretation. Native launch identities stay local; no framework
command, call edge or new architecture group is inferred. Corpus-excluded
configuration-file references remain excluded. These observations are appended
after the existing question reservoir so unchanged earlier requests can reuse
their cache entries. Supporting answer excerpts retain the original source.
Markdown documents enter that same graph as source sections, independently
of code-file groups. Question rows include bounded verbatim excerpts with
commands, links and later paragraphs, labelled as author instructions rather
than runtime evidence. Oversized sections are partitioned without omissions;
exact source lines/columns and section identities remain local. This extends
documentation evidence, not matching's pending caller/callee evidence change.
Callable observations from different target indexes meet at their existing
compiler-located symbol place. Incoming calls retain that place identity as
well as the native object ID; outgoing calls retain callee place IDs for local
retrieval. Outgoing calls also retain their source column locally. An empty
unresolved target view is subsumed only by possible receiver observations at
the same exact call site with otherwise identical call facts; distinct sites,
dispatch details and independent evidence remain. Possible dispatch never
becomes exact. Generated callables retain facts without becoming description or
operation-review candidates. These local keys and call columns never enter provider rows.
The operation review sends immediate caller declarations once with
their distinct call sites; it does not join names or recursively expand callers.
Go callable bindings retain anchored literal assignments to other fields of
the same SSA receiver, independent of framework names. These observations enter
the callable's own review; neighbouring caller registrations omit their field
metadata. No field observation asserts a final runtime value or callback call.

JSTS result v13 / helper v16 also preserves every compiler-observed callable
JSX attribute as an anchored callback binding, including render props. The
element and attribute are source facts; neither their names nor a framework
allowlist classify an operation. Wrapped function-valued declarations retain
compiler callable identity; inline anonymous callbacks and unindexed callable
factory results remain unresolved. A factory result never becomes a binding or
method call on the factory itself. Calls in ordinary local value initializers
belong to their enclosing callable. Symbol table v5 and operation table v12
can interpret these observations as user `interaction`, alongside commands,
requests, scheduled and continuous work, using the same graph and review.

Direct TypeScript interface property declarations retain their written type,
optional/readonly modifiers, exact source location and native owner. Go core
objects v3 retain explicitly declared struct fields, including tags and embedded
field declarations, under their native type. Both project into the existing
type-owned variable objects used by Python class fields. The same atlas members
and question evidence carry them onward; no field creates a runtime call or an
inherited declaration at a new owner. Comparable count-field examples live in
the cumulative TypeScript, Python and Go testdata repositories.

Knowledge is attached to internal entities, separately from provider batches.
Independent directory, file, symbol and boundary rows persist their accepted
cells, exact evidence and prior interpretation dependencies in knowledge.json.
The shared executor stores request/row references beside its exact-response cache;
reuse is keyed by the table contract, provider configuration and exact model
input. `knowledge.json` v2 separately binds reused answers to the current
subject, owners and parent knowledge IDs. Ownership/provenance-only changes do
not trigger model calls; a changed parent line in the input does. Only missing
or changed inputs enter new batches. Question-only readings recall
current descriptions without description calls, then use labelled model hints
alongside source evidence. Question stops retain SubjectID separately from
their context PlaceID and reference used knowledge records. See CURRENT.md for
the distinction between entity behavior and a purpose at one call site. These
descriptions still inspect extracted declarations, not function bodies.
Request/response bytes live once in .llm-cache/payloads; run journals and table
artifacts link to them. Entity memos contain no copied cells: reuse resolves the
current cached response and validates its row, so replay affects the next read.
Old runs remain snapshots. Clearing the cache also removes raw journal payloads.

## Authority

- Code on the ordinary `repomap` main path is the source of truth. Verify what
  the built binary actually does before documenting or extending it.
- [docs/CONSTITUTION.md](docs/CONSTITUTION.md) is the product constitution: what
  the product is, who it is for, and the invariants every change preserves.
  Where it conflicts with this file or the ADR, the constitution wins and the
  other file is corrected.
- [docs/agent-room/CURRENT.md](docs/agent-room/CURRENT.md) is the single living
  ADR. Change the relevant section in place when the product changes.
- Historical decisions and deleted planning documents remain available in Git
  at the pre-cleanup commit `4e54ab3`; they are not current requirements.
- [docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md) owns only the live
  online transport contract. Prompt and response schemas live with their code.
- The owner has approved the single ProgramIndex-to-atlas pipeline recorded
  in CURRENT.md and a shared LLM-provider executor: the atlas tables read
  every target and their boxes are projected into the GroupsIndex the page
  reads. Do not add another analysis graph, semantic authority, or
  presentation layer outside that pipeline without fresh approval.

## Product contract

- Extract deterministic repository facts locally, then send bounded,
  request-local evidence to the configured model provider. The initial
  repository-guidance file-role classifier may send the complete names-only
  safe-corpus dictionary as a lossless prefix-compressed path tree with
  compact `f*` leaves, the complete closed set of prose-file refs derived from
  that same dictionary, plus complete textual README and AGENTS.md documents;
  it sends no other source-file contents. Before any `f*` identity exists, exclude
  `.npmrc`, every `.env*`, dependency/generated subtrees `node_modules`,
  `dist`, `build`, and `coverage`, and every `*.tsbuildinfo` file from the
  shared corpus, freshness state, model input, debug output, and publication.
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
  remain terminal. Every run prints where its time went: a `(t+…)` on each
  stage line, `latency_ms` on each exchange, and a closing `Time` stage.
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
  Cancellation interrupts the wait; other retryable failures retain their
  short backoff. `ExecuteJSONBatch` fails closed: a terminal item
  error cancels the batch child context, prevents queued items from starting,
  and the owning stage rejects the complete batch. `ExecuteJSONEach` is the
  table form: the same pool, gate and observer, but one window's failure
  leaves its neighbours untouched, its rows take their fallback line, and the
  refused answer is written to `rejected.jsonl` and never cached. Validation
  annotates and does not abort a run (docs/CONSTITUTION.md); accepted sibling
  calls keep their identity-bound cache entries in both forms.
- A model-assisted stage returns a fully validated result, a contractually
  legitimate empty result, or an error. Backend orchestration, report
  projection, and browser code must never supply semantic fallback, repair, promotion, or partial
  success for failed or incomplete stage output.
- Keep static prompt prose in readable Markdown beside its owning stage and
  compile it with `go:embed`. Go owns complete dynamic reservoirs and their
  provider-sized request partitions, not long prompt string literals; the
  provider layer does not own domain prompts or schemas.
- Models select only closed request-local short refs. Catalog rows may show
  exact repository-relative paths, file names, symbol names/signatures, and
  dependency names because that context has semantic value. The model is
  never required to copy those values: UUIDs, canonical identities, ref
  resolution, and graph restoration remain local. Unknown refs have no
  authority and are discarded rather than guessed, repaired, or clarified;
  absolute host paths are never sent.
- Set-valued closed-ref responses are filtered to advertised refs and
  deduplicated locally; never require a model to echo each selected ref exactly
  once. Assignment rows keyed by an unknown ref are likewise discarded. A
  known prose ref paired with a valid non-documentation class is an unsupported
  set member and is discarded before its hypotheses or per-file bounds gain
  authority; it is never repaired or promoted into `documentation`. Go
  owns exhaustive batching, completeness, and identity. If filtering leaves a
  mandatory scalar choice or complete assignment unresolved, reject that
  incomplete result without inventing a replacement. An atlas table row asks
  one short line or one closed choice; every key the code sent comes back
  exactly once, and a window that does not is refused whole. Every later model call is an
  LLM cube — a simple prepared format in, one decision, a simple validated
  format out: consolidation returns `{ref, cluster}` labels and Go unions the
  members; parts are named first and then chosen from that closed list;
  titles, lanes and connections are decided in Go. Groups that carry one
  title in one lane are joined by Go before and between model passes. No
  local `support`, `unassigned`, or other semantic complement is manufactured.
  Every accepted group member must itself carry a category compatible with
  that lane: `inbound` or `background_activity` for `triggers`, `core` for
  `core`, and `dependency` for `dependencies`. One compatible member never
  promotes an incompatible peer into its lane. Incompatible member refs are
  discarded as unsupported set members while valid siblings survive. Merge
  preserves every already validated same-lane membership and may duplicate a
  member into another group only when that member's own categories support the
  other lane.
  Missing candidate membership rejects the complete merge; Go never inserts or
  promotes the omitted member. Group proposals left with no advertised known
  member are discarded. Only genuinely incompatible assignments for one known
  ref remain an explicit ambiguity; do not apply first-wins repair.
- Provider request bodies must never contain full repository source contents,
  raw internal edges, canonical internal IDs, the LLM client's authentication
  credentials, or unadvertised paths.
  A complete names-only tracked-file dictionary is explicitly allowed for the
  README file-role classifier.
- The selected repository is trusted and the tool is not a security boundary.
  Nothing is scanned or redacted, the run directory is as sensitive as the
  repository, the run manifest is a record and is never verified against the
  artifacts, and the report server opens the files a page names under the
  analysed root. The provider key is read from the environment and never
  enters a request body or a cache record.
- Analyze every eligible target by default. `--target` selects an explicit
  target; `--force-platform GOOS/GOARCH` overrides the normal Go platform
  selection.
- Run every active Go and Python target scout plus the JavaScript/TypeScript
  package-target catalog scout over the same repository corpus. Merge their
  exact file candidates and resolvable repository-guidance candidates into one
  repository-wide `TargetPortfolio` request; the presence of one supported
  language must never suppress another. Bind one canonical required file
  representative for every exact native target, deduplicating a shared
  representative and never requiring every alternative file for the same
  target. The portfolio must retain every
  required representative, chooses one retained file as the repository default,
  and may additionally retain positively supported guidance candidates. Restore
  every positive file ref through exactly one language adapter into one typed
  target plan. An exact `--target` bypasses the model portfolio but must still
  resolve unambiguously through that same typed adapter boundary. Target scouts
  may not execute an adapter's page-local ProgramIndex, dependency, or semantic
  path. A compiler projection used to build that page, including the JSTS
  TypeScript Compiler API projection, likewise belongs only to selected typed
  targets, so an unselected language's page prerequisite cannot block an exact
  target owned by another adapter.
- The Go fact and target inventory excludes non-`DepOnly` `go list` root rows
  that have no build-selected `GoFiles` or `CgoFiles`. In particular, a
  directory containing only external `*_test.go` files is not an ordinary
  package when the product loads with `Tests=false`; its raw row may inform
  dependency metadata but must not enter package counts, target identity, or
  the typed ProgramIndex scope. A source-bearing package that fails type
  checking is not filtered and still fails its owning target closed.
- Execute every selected typed target through its own complete page-local path:
  sealed base `ProgramIndex`, target-scoped dependency authority, exact reduced
  documentation, the atlas tables over that ProgramIndex, one target-local
  `GroupsIndex` projected from the atlas, and a validated report page. A selected
  non-default target is not a structural substitute for that path. Multi-target
  publication seals a language-neutral `ProgramPagePortfolio` keyed by exact
  `ProgramTarget` IDs and child run IDs plus one exhaustive
  `TargetOutcomePortfolio`; joints between targets come from matched boundary
  values confirmed by the model. Retain target-local ProgramIndex, dependency
  catalogue and GroupsIndex artifacts. Publish repository-wide facts, claims,
  orientation, atlas, portfolios, manifest, `report.json` and `report.html` once
  in the successful owner run. Stages pass typed values in memory; persistence
  is separate from computation. Load a saved artifact only when its value is
  absent from memory. The report server consumes the generated result directly,
  or restores one common report and manifest in another process. Every target
  is a section of that common page.
  Single-target publication uses the same one-page `ProgramPagePortfolio` and
  one-row exhaustive `TargetOutcomePortfolio`; it has no direct page,
  manifest, report, or browser fallback.
- A multi-target run contains a target-local preparation, analysis, semantic,
  or page-validation failure instead of discarding completed sibling pages.
  Persist one exhaustive, adapter-neutral `TargetOutcomePortfolio` for every
  selected target: a row is either bound to one complete ProgramTarget/page or
  carries only a closed public failure stage and reason. Never persist raw
  errors or adapter-native refs in that authority and never turn a failed
  target into a partial page. The picker keeps failed rows visible,
  red, disabled, and linkless; the repository overview reports analyzed versus
  selected coverage. Materialize each selected JS/TS compiler project at its
  own target boundary so a missing compiler does not preflight-fail unrelated
  languages or packages. A shared selected-target Go workspace is only an
  optimization: if its union cannot be prepared, retry the current exact target
  locally and keep every later Go target isolated; never reuse a target-local
  fallback workspace as sibling authority. Context cancellation and shared
  portfolio, persistence, manifest, repository-overview, or bundle failures
  remain publication-terminal. When at least one target succeeds, the analyzed-page
  portfolio and its neutral bundle remain valid even if that is the only
  successful page; the first successful page owns the one physical HTML while
  the originally selected default remains the logical default in
  TargetOutcomePortfolio. If every selected target
  fails, retain diagnostics but do not invent a targetless or synthetic report.
- The current JavaScript/TypeScript slice owns every eligible `package.json`
  project. Package-source ownership is assigned to the deepest containing
  manifest; every manifest with at least one owned tracked JavaScript/TypeScript
  source contributes one required target representative. A source-less
  manifest is tooling rather than target authority and cannot suppress sibling,
  child, or ancestor package targets. TargetPortfolio retains all exact package
  targets and chooses only their repository default; an explicit
  `jsts:<manifest>` narrows the typed plan to that one owned package before
  TypeScript compiler execution. `package.json#name` is optional: an exact
  top-level npm lockfile name is the secondary package identity, otherwise the
  root uses `root-package` and a nested package uses its repository-relative
  project directory. These fallbacks never authorize an implicit string-form
  `package.json#bin` command. Its corpus-only scout owns the exact
  manifest/source-ownership catalog, target identities, and package candidates
  without invoking Node or the TypeScript compiler; it participates in the same
  portfolio and typed execution plan as Go and Python. Each retained or
  explicitly selected package target receives its own page, which uses an
  owner-prepared, repository-local TypeScript Compiler API to honor
  `tsconfig.json` or `jsconfig.json`, repository-confined solution-style
  project references, aliases, and module resolution. A project reference that
  stays inside the owning package extends that page's complete compiler graph;
  an exact repository-local reference outside the package is a cross-target
  boundary and does not pull the sibling package into the page. Missing
  references and references outside the analyzed repository still fail closed.
  Repomap never installs npm, yarn, pnpm, or other packages. Browser and Node
  server surfaces plus
  canonical safe, package-owned, tracked `package.json#bin` command/path pairs
  are product surfaces inside their owning page-local ProgramTarget. A CLI
  surface never invents a bin-wrapper-to-source relation: its entry refs stay
  empty, while an exact canonical `dev` or `start` script with one
  helper-selected source may independently seed that source only after CLI
  product authority exists.
  Compiler/type-resolved declarations and exact external imports are the only
  call-target authority. Every ProgramIndex external symbol carries its exact
  raw package origin plus an adapter-derived `package` or `platform` authority
  kind. Go derives it from the complete build-selected `go list -deps`
  package-origin universe (including `DepOnly`) and its `Standard` bit; Python
  derives it from the exact `sys.stdlib_module_names` set; JS/TS maps
  TypeScript default-library and Node standard-library origins to `platform`
  and npm origins to `package`. Missing or unknown authority fails closed;
  shared stages never infer it from a package-path prefix. A platform external
  object and an exact external invocation pattern whose complete target set is
  platform-only can never receive `dependency`, support a `dependencies`-lane
  group, or become a matching boundary. They remain valid structural evidence
  and may receive another positively supported category; for example,
  `requestAnimationFrame` may support `background_activity`. Calls and
  constructions retain their distinct exact invocation authority; an
  unresolved property name remains an unresolved frontier and is never matched
  to repository declarations by name alone.
  Compiler authority comes only from `typescript` or an exact npm alias to it
  declared by the selected manifest; a nested package may inherit candidates
  from the repository-root manifest only when it declares none itself. Each
  candidate resolves from the package.json scope that declared it. The
  installed package must identify itself as `typescript`. Distinct compatible
  candidates in the selected API tier fail closed; one stable legacy Compiler
  API candidate is preferred over a native-preview candidate when both are
  deliberately declared.
  Shared contracts are supporting code, build/migration scripts remain tools,
  and a runtime script, library, or tool-only root must never promote itself
  into an application.
- Language adapters retain method/path-shaped calls, decorators, arguments,
  reconstructed values, exact targets, alternatives, and unresolved frontiers
  only as neutral ProgramIndex evidence. Protocol meaning arises through the
  atlas tables over that evidence; deterministic stages preserve the neutral
  evidence and its exact provenance.
- The atlas (`internal/atlas`) is the model path. `places` builds
  `places.json` from the program indexes, claims, facts and corpus: every
  directory and file, the declarations of a file with the first sentence of
  their docstrings, up to ten candidate symbols per file, the boundaries
  (routes, client calls, listeners, configuration reads, dynamic execution,
  and calls into non-platform packages with a literal argument), the
  file-to-file edges and the seeds. `reading` walks them in rounds and asks
  one keyed table per round: directories by depth, independent files with direct caller facts,
  symbols, boundaries, the parts of a target (one row asking for
  exactly `want` names, then every top box choosing from that closed list),
  the drawn arrows, the portfolio, the joints. A row carries the place's own facts and its directory's line, one step up.
  File callers contribute deterministic facts, never another file's model
  line. Independent directory, file, callable, type, boundary and operation
  tables pack consecutive complete rows by their input-byte budget (64 KiB
  default), without artificial 8/40-row caps. Explicit read-stage row budgets
  remain available. Other tables retain their owning context and round bounds.
  An oversized singleton is an explicit preparation
  error requiring a different evidence representation in its owning stage. The model writes one line or one closed choice per
  cell; the code owns membership, arrows and their direction, joints by
  matched values, counts and identities. A refused window falls back on its
  rows' deterministic lines and is written to `rejected.jsonl`, never
  cached. Every row and answer is printed to `tables.md`, with prompts, requests, raw
  responses and normalized source-bound results under `tables/`. The ordinary
  path saves `reading-input.json` before its first atlas call; `read` consumes
  exactly that format and runs the same reader. Above two thousand files the directory and
  file rows carry an `open` cell and what the model closes keeps its
  fallback line.
- Type descriptions use the existing symbol stage and symbol knowledge, with
  the active `lines/prompts/types.md` prompt embedded by `lines/tables.go`.
  Atlas graph and saved reading input version 8 retain a type's declarations
  through exact native owner IDs, including cross-file methods and explicit Go
  interface method declarations. Interface declarations have no invented
  direct-call node. Type context retains the existing bounded author quotes,
  including later sentences; ordinary file/callable rows still use first
  sentences. The type table v5 asks for a short explanation
  and key flag; it cannot classify activations. Its explanation is prose, so
  normalization preserves complete sentences and qualifications rather than
  cutting them at 240 characters. Undocumented lifecycle topics are omitted
  instead of appending irrelevant absence claims. Bare names without owned
  declarations or author documentation stay in the source index without an
  invented definition. A file's model hypothesis is not type evidence. Ordinary
  callable rows keep their own contract and memo identity. Concept explanations
  on maps are a projection of existing interpreted type subjects, not a glossary
  or a second semantic graph. Effects implemented elsewhere still require their
  actual source contract; method ownership alone does not establish them.
  Names, signatures and argument names are useful clues for model hypotheses;
  absence of comments must not prevent orientation. Distinguish such hypotheses
  from observed local actions and effects established by following callers.
  The owner explicitly authorized an incremental context/cost experiment on
  the public acceptance repositories: compare names, signatures, own code
  without comments, and documented code through the configured replay client.
  This is a development measurement, not ordinary report acceptance or a
  general implementation-body expansion of the product pipeline. The separate
  matching-context approval recorded in CURRENT.md remains pending.
- `groupindex.ProjectAtlas` turns the atlas into the GroupsIndex the page,
  the orientation and the publication read: a box is a group whose members
  are the objects declared in its files, a zone is a container, an arrow is
  a connection labelled with the model's sentence, a joint is a connection
  into another target. Lanes follow a box's side: `triggers` where the
  outside calls in or execution starts, `dependencies` where it only calls
  out, `core` otherwise. No request sends repository source text: paths,
  names, signatures, first sentences of docstrings and documentation excerpts,
  literal values and the model's own earlier lines cross the wire. Question
  excerpts preserve Markdown-authored command/code examples; implementation
  source-file bodies are not sent.
- The ordinary Go direct-call traversal is complete for the selected target:
  `--depth 0` and `--edges-limit 0` are the defaults and mean retain every
  exact call and edge across loaded repository declarations, including functions
  outside the launch tree. Positive values are explicit user-requested
  narrowing controls. Graph depth above 10, more than 10,000 exact edges, or
  more than 65,536 exact nodes emits one aggregate warning but never truncates
  the ordinary graph or fails a target. Dynamic and unresolved call frontiers
  remain represented separately. Warning derivation is diagnostic-only and
  can never reject an accepted target or publication.
- ProgramIndex version 11 retains every source-distinct nested pattern without
  local sampling or truncation, including its exact location, neutral
  call-result/receiver provenance, any exact callback source-argument
  provenance, and reconstructed value candidates with their source-object and
  source-argument provenance. Duplicate compiler witnesses do not become pattern omissions.
  The former 64 MiB aggregate-semantic-text and 128 MiB canonical-JSON sizes
  are ordinary scale warnings only. Structural JSON overhead never consumes
  semantic evidence authority, and crossing either size cannot reject or
  truncate the index.
- Everything the report shows is a deterministic fact, a claim quoted from a
  human-written artifact, or a model hypothesis, and the three are always
  labeled. `facts.json` holds the anchored fact layer: entrypoints, HTTP routes
  and client calls with method and path literals, cross-target portals,
  environment keys, the places where the program runs code it was given,
  manifest rows, TODO markers, imports, dead modules, negatives, and
  dependencies. `claims.json` holds quotes with their
  source path, date and age. `orientation.json` holds the model's repository
  summary, roles, run recipe, and main flow; every row cites fact, claim, or
  subject ids, and a row whose refs do not resolve goes to `rejected.jsonl`
  with its raw output and reason instead of being repaired. Validation
  annotates and never aborts the run.
- The report is one static page rendered in Go. Its reader is a newcomer, so
  pipeline vocabulary never reaches the screen: retained, source-bound,
  authority, projection, selector, outcome, target contract, and raw selector
  strings such as `python:backend:guard:main` are banned on screen. The page
  ships only its own small stylesheet and one optional editor-link script; it
  embeds no analysis payload.
- Semantic output is canonical English. The owner approved final presentation
  localization on 2026-09-07/08: `--lang ru` translates the already-built English
  frontend structure, with an ordinary UI dictionary and a separate LLM cube
  for generated display prose, then renders `report.<repo>.ru.html`. Source
  excerpts, names, IDs and topology remain original. The default English file
  remains `report.html`. Ordinary publication passed on the small server,
  chi, Python/TypeScript fixture and Chinese Jieba repository; answer quality
  and the retained visual alternatives remain explicit owner-review items in
  CURRENT.md. `llm.Prepare` adds one shared response-language system fragment
  before provider encoding, execution, fit checks and memo identity. Empty
  ResponseLanguage means English; the final translator supplies its language.
  Exact-byte replay does not rebuild or add this instruction.
- Repository changes during a run do not fail publication. Do not reintroduce a
  freshness gate or strict-snapshot mode.
- `--no-serve` requires resolvable GitHub or GitLab source links and fails in
  preflight with corrective flag guidance otherwise. Served reports may only
  add manifest-authorized local editor opening; do not add browser APIs for
  workspace reads, investigation, symbols, source context, or run selection.
- Persistent caches remain part of the ordinary path. Cache hits must be
  identity-bound and fully validated before use; `--no-cache` is the explicit
  live-provider bypass.

## Development and acceptance

- `testdata/acceptance/python-tutorial-game` is the acceptance fixture: a tracked copy of
  that repository at revision `78714d34ee` with `expected.json` and the sealed
  artifacts of one real run. Its focused test rebuilds the fact layer and
  asserts every expected row with its anchor.
- Focused discovery and ProgramIndex regressions keep exactly one cumulative
  real repository fixture per active language under
  `testdata/repositories/<language>`. Extend that repository with new scenario
  files instead of creating per-bug repositories. Every tracked fixture file
  must have an exact file-inventory expectation; deterministic language
  adapter stages run for real, while provider-backed stages use exact
  request-bound, fail-closed local presets with no network access. Fixture
  success is focused test evidence only and never replaces ordinary online
  product acceptance.
- Every language-cube behavior change must be recorded in that language's
  cumulative `testdata/repositories` fixture and an executable expectation.
  Add or extend an understandable source example, run the actual extractor
  and adapter, and assert the changed contract at its consuming boundary.
  Existing coverage may be reused when it demonstrates the exact changed
  behavior. Ownership or resolution changes also keep a contrasting case that
  would expose an invented owner, call or relationship. A passing unrelated
  test or a prose note alone does not record the change.
  When a case found in one language has equivalents in other supported
  languages, immediately add or extend comparable examples and expectations
  in all of them, without waiting for a separate owner request. Check the
  actual language-specific semantics; a missing equivalent is recorded as
  such rather than fabricated. Code that already handles the case needs its
  regression example, not an unnecessary production change.
- Keep the cumulative language file inventories under `testdata/contracts`
  exact and update their executable expectations with each fixture change.
  The separate prompt, numeric-limit and test-file inventories were removed
  in `b9f1c182`; do not recreate them as a second bookkeeping step. Embedded
  prompts and provider contracts are checked by their owning packages, and
  `make test` covers all product packages under `cmd` and `internal`.
- Build the owner-facing binary with `make build`; it must write
  `.bin/repomap`.
- Canonical `make test` and `make vet` use the ambient system Go build and
  module caches, cap package-level parallelism at four, and give each test
  binary at most five minutes. Focused commands must keep the same or tighter
  bounds; ordinary development must not relocate `GOCACHE` or `GOMODCACHE`.
- Product acceptance means running that binary on a real repository through
  the normal online provider path. Offline runs, fixtures, replay commands, and
  helper tools are not acceptance evidence.
- Verify the process exit status and the generated manifest, exact reduced
  documentation, sealed ProgramIndex set, each target-scoped
  `dependency-catalog.json`, `places.json`, `atlas.json`, `tables.md`, every
  projected GroupsIndex, the complete graph in report JSON, and report HTML. For a multi-target run, verify
  exactly one common manifest/report JSON and one physical report HTML in the
  successful owner run. For cache changes, also verify a real second run and `repomap
  cache clear`.
- Browser QA for a generated standalone report serves the narrow run root that
  contains the owner report and any backing sibling target-run directories from
  a temporary loopback-only `python3 -m http.server`, then opens the corresponding
  `http://127.0.0.1:<port>/<run>/report.html` URL; do not rely on `file://`
  behavior.
  This is a development-only inspection step, not a supported repomap command,
  product server, or checked-in sidecar entrypoint.
- Run focused Go tests for changed contracts and `go vet` for changed
  packages. Never leave known broken tests.
- Debug artifacts must never include API keys or Authorization headers and must
  never be committed.
