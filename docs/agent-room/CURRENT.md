# Current product decision

Status: active living ADR. Updated: 2026-09-29.

[CONSTITUTION](../CONSTITUTION.md) takes precedence. This page records the
current decision and acceptance state; [AGENTS](../../AGENTS.md) routes work to
one authoritative contract per area. [CHANGELOG](CHANGELOG.md) records concise
implementation/check evidence. The former long ADR and agent instructions are
preserved in the [non-normative archive](../archive/2026-09-10/README.md), not
required reading or another source of current rules.

## Reader outcomes before optimization

A newcomer should quickly see what a component does, which requests and
commands enter it, which responsibilities run in the background, and which
other systems it contacts. They can then follow a useful scenario with
checkable source links and explicit gaps. Count source-backed observations;
several call sites do not necessarily mean several external systems.

| Outcome | Required evidence |
| --- | --- |
| Understand the repository and its parts | Complete selected component inventory, responsibility and ownership; unavailable analysis stays visible. |
| Run or use it | Native launch/registration identities, operations and attributed prerequisites/instructions. A documented command is not a verified run. |
| Follow a scenario | Trigger, participating parts, observed connections, result and unresolved steps. Reading order is not call order. |
| Understand data and concepts | Original declarations, owned fields/interfaces, storage/query observations and supported lifecycle claims. |
| Understand external communication | Participants/roles, direction, purpose, interface and original call/address evidence; unresolved configuration stays unresolved. |
| Configure behavior | Source-linked settings and their observed reads/defaults/conditions, distinguished from installed dependency documentation. |
| Understand failure and recovery | Supported error, cancellation, retry, cleanup and observable-result evidence; inferred behavior remains qualified. |
| Change and check it | Relevant implementation, tests/examples, checking instructions and missing evidence. Test discovery is not coverage. |

The source inventory and the initial explanation have separate completeness
requirements. A declaration can remain available without its own caption;
that alone does not establish a useful map or useful Learn questions. Evaluate
optimizations against preserved responsibilities, scenarios, operations,
integrations, concepts, anchors and gaps, then elapsed time, requests/tokens,
local work and disk. No percentage of closed code is an acceptance rule.

## Current decision

One ordinary pipeline consumes the shared corpus. The canonical target plan
assigns `t1..tN` once; that same identity passes through selected outcomes,
ProgramIndex, facts, GroupsIndex and report joins, including failed targets.
Native adapters contribute to the same compact fact identity scheme. Adapter
`SourceRef` values are construction keys only and never persist. Within a target,
objects use `n*`, relations use `e*`, and nested facts are scoped below those IDs.
Facts own `a*`, human claims own `h*`, operations own `o*`, and the group overlay
owns `g*`, `k*` and `x*`. Provider requests reuse these IDs directly under a
closed allowlist; cross-target graph refs are merely qualified (`t1.g1`,
`t1.n22`), not renumbered. Only choices that do not exist in an artifact use
temporary `c*` refs. Natural ordinal order is mandatory, so `t10` follows `t9`.
The repository index SHA binds every derived decision; failed target outcomes
and model-row refusals remain explicit.

Atlas tables obey the same rule: provider row keys are their existing compact
place/target/joint/question IDs, never a second `r*` numbering. Question IDs are
assigned once from canonical question order and survive presentation reordering.
Symbol selection and operation review use the symbol's `s*` ID instead of
`selection:s*` / `operation:s*` pseudo-places. Atlas arrows receive persisted
compact `x*` IDs before model reading instead of joined box-pair keys.
Exact-input memo fingerprints omit owner identity entirely rather
than replacing it, so identical evidence can still share one accepted response
without exposing another name to the model.

The active simplification removes duplicated fact graphs from later artifacts.
ProgramIndex owns deterministic facts; the reading result owns model annotations
that reference those facts. Places, atlas and GroupsIndex are migration names,
not permission to persist partially self-contained copies of objects, relations,
paths or structural edges. The final repository artifact contains the fact graph
once and annotations once. `report.json` names the run's own ProgramIndex files (its
directory's and its targets' sibling directories', each with its SHA-256),
not a presentation-specific graph, and stores GroupsIndex as the same thin
semantic overlays. Native group subjects and structural edges exist only as an
in-memory join while the HTML is built.

GroupsIndex is that thin overlay: its `g*` groups, `k*` containers, `o*`
operations and `x*` connections are deterministic target-local ordinals over
canonical content, and its subject rows hold only `n*`/`e*p*` refs,
categories and interpretations. Reading it requires the exact bound
ProgramIndex. No semantic ID is a content hash. An object has one compact
`n*` identity inside its target, `t*.n*` in the shared scope; cross-target
meaning exists only in explicit `x*` connections backed by existing facts,
never in a hidden join key ([ProgramIndex](../contracts/PROGRAM_INDEX.md)).

- **Shared fact vocabulary (ProgramIndex 17):** every fact is written once
  (no `contains` relations, no derivable counts or empty fields). All adapters use
  one closed `invocation`/`dispatch` vocabulary and one resolution rule: one known
  target is `exact`, several `alternatives`, none `unresolved`. Go signatures use
  short package names, types show only their form, and struct tags become object
  `aliases`. Go resolves interface values it observed, including external types
  such as `*sql.DB`.
- **Registrations and entries (2026-09-17; entries 2026-09-27/28):** the
  fact layer names no framework. A `registration` is the shape of a non-owned
  call handing over a callable, address or named value, or a C command-table
  row (D1), with the words its call wrote. The reading stage classifies it
  (`request` for any protocol, `command`, `scheduled`, `continuous`, …) and
  names an entry by choosing among its written words, which code restores
  verbatim; there is no HTTP branch (owner, 2026-09-27). `sql_query` is a
  fixed `db` boundary only for a literal with SQL statement structure.
  Without a model there are candidates, not routes; the Echo preset test is
  the offline acceptance. Each call outside tests that gives an outside
  symbol a word is asked `enters` on its own, after its symbol's `talks`; a
  call answered an entry kind is an entry whose handler is not established
  (`handler_unknown`). `talks` offers `runs_program`: one outgoing boundary
  per launching call, the program named by the word Jev chooses
  (`atlas_program`). Each language contract lists what it lacks
  ([ProgramIndex](../contracts/PROGRAM_INDEX.md),
  [Reading](../contracts/READING.md) "Operation ownership" and "External
  symbols", [EXTRACTORS](../EXTRACTORS.md)).
  The `dynamic_execution` fact no longer names process launches
  (`internal/facts/risk.go`); the constitution's example list still does
  (the owner's to change).
- **Map of parts (2026-09-25, owner's proxy spec; the owner's open questions
  1–7 of that spec still stand):** every map request sees code structure
  only, never README, AGENTS, docstrings or package documentation. The role
  split runs first: a Jev helper question per unit, a Jev gate per file, a
  DeepSeek naming of a split file's boxes and a Jev assignment of its
  declarations. Code places what they leave open by a declaration's users
  (never a hand-over); a helper is never named or assigned but goes with its
  users, and a shared or unused helper is asked once more in a second pass.
  Then one DeepSeek parts request per target groups the units (whole files
  `f*`, boxes `c*`, and each unit no box took as a row of its own, fix B
  2026-09-29) into named parts; a unit left out or listed twice gets one
  closed-choice placement follow-up, and what that cannot place stays off the
  map with its reason. A declaration takes its unit's part; a file's part is
  the one part holding all its placed units. Descriptions and areas are
  separate requests; a refused description is an explicit no-description
  state. A parts or areas answer refused whole fails that target's map for
  the run (`map_failure`); there is no second draw. A test-only part leaves
  the canvas by the `TestSources` fact. Only the part holding a target seed,
  and its area, is the entry; a seed no part holds is named with its off-map
  reason ([Reading](../contracts/READING.md) "Architectural
  responsibilities").
- **One reach (map model step 4, 2026-09-28):** GroupsIndex derives, with one
  function on projection and hydrate alike, each input's reach, the dispatch
  sites and the inputs reaching them, hand-overs from running code, the phases
  and which arrows are quiet; the report walks no code. Arrows into helpers
  are quiet like initialization. An entry keeps a line only when the model
  wrote one. Chains and operation types are deleted
  ([Reading](../contracts/READING.md), [Report](../contracts/REPORT.md)).
- **Owner decisions of 2026-09-27/28 (map model step 3):** ask once whether a
  declaration is a helper; shared helpers go to a second pass; arrows into
  helpers are quiet like initialization; an area is purple when any part
  inside it is the domain; types stand right after keys among a part's tiles;
  the amber DNS group stays; files are split into role boxes before the units
  are grouped into parts; no utils part. Still open for the owner: a
  library's public API as entries (an `export` seed kind), which would make an
  entry never a helper by code.
- **Lead decisions awaiting the owner's review (2026-09-28/29)**, taken by
  default and shipped, each his to keep or undo:
  - rule B read literally: a whole file joins its users' box only when every
    unit it declares, its types included, is a decided helper;
  - (2026-09-29, delegated) a file that joined a box gives that box its
    imports; no single-user join rule and no per-row `called_from` field;
  - the all-quiet exception: when every arrow of a program would be quiet, its
    calls into helpers are drawn;
  - depth layering: an input's path draws every call into a part from a part
    reached earlier and counts the others;
  - the "init" label covers the launch and the main loop, everything only the
    seeds reach;
  - a seed no part holds is named in the component's reading and "Not on the
    map" with its off-map reason, not on the canvas;
  - S4-6 deleted GroupsIndex's chains and operation types;
  - C4 (the helper question) shipped despite its one known miss, pykrx's
    `get_market_ohlcv` answered helper;
  - the no-users rule applies only to a kind whose uses its adapter records
    (`recordedUses`); other kinds are asked like types;
  - u5's depth-1 fold: an input's path opens the parts its handler calls
    directly and folds deeper ones under "Reaches {n} more parts deeper".
- **Map reading on the canvas (2026-09-25):** a part's description stands on
  its box under its name, a closed area's line on the area's box; looking at
  an area or a component darkens only the arrows that cross its border
  ([Report](../contracts/REPORT.md)). An outside call in the component card is
  named by its destination (`DeepSeek · Client.Do`). Left for the owner after
  an owner-proxy review:
  - the number chips on parts and frame borders (explain or remove);
  - areas in the model's pipeline order;
  - a part's inside showing a dozen of hundreds of declarations.
- **The page needs JavaScript (2026-09-29, owner decision "b"):** the page is
  one self-contained HTML file whose data holds every answer, each fact once,
  written compactly and read back exactly; the script reads it, nothing the
  reading column renders from the data is printed again, and a `<noscript>`
  line says so. A function reads as its flow; an input opens at how a request
  reaches it ([Report](../contracts/REPORT.md)).
- **Decoders (owner, 2026-09-26):** a decoder refuses only what is wrong, at
  the smallest scope; a form difference is not a refusal, and there is no
  identical-bytes resample. Refusals that keep the report true stay: unknown
  refs, a missing or out-of-choice required decision, two different answers
  for one row, unsourced substantive answers. The cache holds only accepted
  answers; a refused answer keeps its bodies in its run's `payloads/`. Owner
  decisions of the same day: a Jev choice needs a lead of 0.10 over its
  runner-up; key declarations, part roles and keys are decided only through
  `llm.Categorizer` (Jev), with `JEV_KEY` required and no DeepSeek fallback;
  an empty HTTP-200 answer gets one transport retry; an unknown default
  target leaves the default unresolved and the report is published; glossary
  terms match in any case and with a plural -s/-es; README-backed recipe
  steps stay refused, with no labels; areas keep the model's order; a table
  asks no column nothing reads; the English alias is asked only of a name
  with a letter outside the Latin script ([Execution](../contracts/EXECUTION.md),
  [Reading](../contracts/READING.md), [Discovery](../contracts/DISCOVERY.md),
  [Terminology](../contracts/TERMINOLOGY.md)).
- **Data ownership:** an extraction belongs to the programs holding one of its
  files. One no program holds in a code file (tests, fixtures, scripts) or
  under a tooling directory (`testdata`, `.github`, `.claude`, `.vscode`) is
  no program's data; a schema or migration no adapter reads keeps the root
  rule ([EXTRACTORS](../EXTRACTORS.md)).
- **Known remaining violation:** key selection still sends a declaration's
  full docstring as `author_documentation`, and symbol selection sends
  docstrings too. The trusted-inputs rule covers both; the next change strips
  them with its own before/after on keys. Docstrings stay for captions,
  orientation, glossary and claims until the owner extends the rule.
- **Reading:** role/activation/outgoing selection is independent of captions and
  directory closure. Native boundaries and accepted operations survive missing
  prose. Questions retain original evidence, explicit coverage and independent
  results. Learn proposes from eight intents and uses the same answer path.
- **Operations:** there is no operations table (2026-09-17): an operation is
  an entry (Registrations and entries above). Process entry or asynchronous launch
  alone does not establish a responsibility; a task may delegate work to helpers.
  Caller/setup context is evidence, never a handoff choice. Persistent consumers,
  cron and supported one-shot scheduled work remain distinguishable from
  listeners, lifecycle hooks and middleware within the same request. Original
  own-call receiver/arguments/API distinguish effects on the request and response.
- **Boundaries:** v8 asks fixed native facts only for explanation and applicable
  closed address/destination fields. A destination is chosen among the names
  the row's targets' outside packages were given (`atlas_systems`, one
  question per package), or `other:`; no list of systems lives in code. Candidate communication needs an accepted
  runtime interpretation; a remote client instance is distinct from an option
  passed to its later constructor. Source chains preserve correlated uses and explicit
  unknowns; imports, timers and internal delegation do not become integrations.
  Native listener addresses stay separate from HTTP routes. Shared source
  boundaries retain each target's original fact/declaration identity through
  atlas and GroupsIndex, including distinct same-line methods, paths and
  callees. Each call has one source position in every language adapter.
- **Questions and orientation:** question-batch v4 assigns relevance per
  original anchor; question, boundary and orientation evidence preserve safe
  native receiver/arguments/API and exact call sites
  ([Reading](../contracts/READING.md) "Question evidence and per-anchor
  relevance"). Owner decision 2026-09-25: a declaration's own body may be
  sent to a provider for its caption. A decision model ranks a part's
  declarations and the top ranked (about ten) go with their bodies in one
  request; the rest stay in the analysis without a caption, visibly. Whole
  source files still never enter provider bodies, and the repository
  remains trusted input, not a security boundary.
- **Execution:** shared reasoning/output allowance is 128,000 tokens, subject to
  the provider ceiling. Output refusals partition independent questions first;
  input/context refusals preserve and repartition complete evidence. A new
  limit/packing policy is tested on one saved complete window before a full run.
  An exact request already in the air is asked once and its twins read that
  answer, so targets sharing a part get one description and a warm rerun
  makes no live call.
- **Orientation stage 2 (2026-09-29):** an overview (facts, claims,
  connections, groups without members, seed rows) answers summary, roles,
  recipe and `main_flow_target`; a second request asks that target's main
  flow over Launch ∪ Reach ∪ hand-overs, every member complete as a lossless
  tuple row in reading order. The member ladder is gone. A flow request the
  provider cannot hold is journaled under `flow_request` while its overview
  stands ([Reading](../contracts/READING.md) "Orientation").
- **Glossary:** only accepted prose enters a separate p-ref
  generation/reduction pass. Generation is three steps, each one decision
  (owner, 2026-09-28): DeepSeek lists names, Jev decides per name domain
  concept / general vocabulary / code element (a near-tie stays undecided),
  and DeepSeek explains only the decided domain concepts; code attaches every
  row that writes a name. Each text step has its own 32,768-token output
  allowance. Source/prose/fill context stays local; no deferred g-ref appendix
  is sent. Reduction keeps every original definition and scope
  ([Terminology](../contracts/TERMINOLOGY.md) "Reducer completeness").
- **Report:** compact named inventories precede maps, with five initial rows and
  All N retaining the remainder. Calls, workers and source-linked data must be
  easy to find. Code names/paths/addresses remain original; English aliases and
  operation labels coexist with localized descriptions. Saved rendering uses
  ordinary templates and no provider or cache. Detailed display/data changes
  retain their consuming contract and require fresh ordinary acceptance. Data
  links use existing exact model/callable ownership; an unresolved endpoint or
  unowned SQL literal cannot acquire a database flow in the report.
- **Where an input takes effect (consilium 2026-09-28, owner answers Q1 and
  Q2 yes):** a seed of a split file is its own grouping row, so an
  executable's `main` is on the map, and a unit only the seed uses joins that
  row. Inputs without a handed function form catalogues keyed by the object
  they are declared on (the call that made it, followed through outside calls
  that name nothing), else by the declaring function (L3); the reading says
  where they are declared and called from, never that they take effect
  there, and once "Where these take effect is not established." A wordless
  hand-over on a word entry's own result of the same kind is one input with
  it (J1). Such an input draws the ordinary Inputs arrow into the part where
  its code takes it in. GroupsIndex derives the launch walk from the seeds
  and load-time roots (Go init and package variables, module bodies outside
  C), saying per function found / unsure / could not look inside / nothing,
  shown only in the Inputs reading's fold; words only an input's handler
  checks are its sub-arguments, never tiles. C tables and callables the
  program's own functions keep are asked once each; a table row names a peer
  program's input only through the peers question (`Operation.Sends`, never
  an arrow). A launched program whose word equals a name the repository's
  build gives one of its own programs (ProgramIndex `target.executables`) is
  that program, by equal names only (2026-09-29)
  ([Reading](../contracts/READING.md), [Report](../contracts/REPORT.md),
  [C](../contracts/C.md) and the other language contracts).
- **Field readers and writers (2026-09-29):** a record type's reading gives
  each field "Written by" and "Read by", the functions by part; a function's
  says "Writes: …" once and "Reads: …" once, in place of "Uses variables"; no
  column list prints a line number. The facts come from C field paths, Python
  typed receivers and JS/TS declared properties
  ([Report](../contracts/REPORT.md)).
- **Benchmark v3 and v4 fixes (2026-09-29):** a flow's helper calls are named
  on one muted line under their step, and a call into the caller's own part
  is its work; a macro's call reads as the macro, a compiler builtin as no
  call; a Main flow step citing a registration reads as the callable it hands
  over, with where it is registered and what runs it; every reading of a
  component has one "Main flow" link, followed by "Also runs on its own:"
  (scheduled and continuous inputs). A setting reads by its key with the
  fields its branch writes; a TODO is a comment's marker in a code file. The
  column has no scroller of its own, a name is its link, and a part lists the
  declarations reached from outside it first. A canvas click never moves the
  camera; "Back to map" restores the page as it stood
  ([Report](../contracts/REPORT.md)).

## Contracts and formats

Commands/orchestration: [Surface](../contracts/SURFACE.md). Corpus, author scope
and targets: [Discovery](../contracts/DISCOVERY.md). Native materialization and
source expressions: [ProgramIndex](../contracts/PROGRAM_INDEX.md), with native
[Go](../contracts/GO.md), [Python](../contracts/PYTHON.md) and
[JSTS](../contracts/JSTS.md) contracts. Semantic stages:
[Reading](../contracts/READING.md). Provider/cache:
[Execution](../contracts/EXECUTION.md). Definitions:
[Terminology](../contracts/TERMINOLOGY.md). Display:
[Report](../contracts/REPORT.md). Facts/SQL: [EXTRACTORS](../EXTRACTORS.md).
Required checks: [Development](../contracts/DEVELOPMENT.md).

### Formats

Current shapes are defined by their owning code, not historical run headers:
[ProgramIndex](../../internal/programindex/index.go),
[graph/atlas](../../internal/atlas/atlas.go),
[reading input](../../internal/atlas/reading/input.go),
[GroupsIndex](../../internal/groupindex/index.go),
[report](../../internal/report/report.go),
[manifest](../../internal/report/manifest.go), and
[accepted cache](../../internal/llm/cache.go).
This wave uses ProgramIndex 21, places graph 21, reading input 20, atlas 19,
GroupsIndex 25, dependency catalog 2, extraction artifact 2, facts 4, claims 2
and target outcomes 3 with compact artifact-local IDs and no saved adapter
SourceRefs. Dependency catalogs assign canonical `i*` importers and `d*`
dependencies. Accepted extractor nodes use `u*`; arbitrary public-protocol IDs
remain only in the exact recorded stdout.
The current reading input and atlas carry the current semantic result with
target-qualified native object refs and short graph/part/area/joint IDs, in
addition to heading context and original dispatch observations,
boundaries v8, question-batch v4 and local cache record v3.
Question routes are v10 and knowledge is v3: both retain the same compact
question/place IDs used by provider rows.
Native adapter constants remain in their owning language packages; genuine
fixtures are regenerated through those adapters when their format changes.
There are no old-format readers or manually rewritten seals.

## Acceptance and open work

Receipts for every row, including the run log moved out of this table on
2026-09-29, are in the [CHANGELOG](CHANGELOG.md).

| Scope | Current status |
| --- | --- |
| Local checks | At 260acd7f (2026-09-29, the report tests reduced to what they protect): `make test`, `make vet` (package parallelism 2), `make ui-test` (133) and `make ui-visual-test` (64 passed, the 6 `REPOMAP_REAL_RUN` journeys skipped) pass; a second render of a saved run is byte-identical (6cf4388c). The real-run journeys last passed 6/6 on 2026-09-28 (Redis). |
| Redis 1.3.6 (C) | Ordinary runs exit 0 on 2026-09-29 (the latest 9 s with 0 live calls; report.json names every ProgramIndex file); orientation stage 2 accepted (recipe `./redis-server [/path/to/redis.conf]`, flow main → aeMain → processCommand → call); redis-server has 19 parts and an entry part holding `main`. Open: the runs use the owner's default system cache, so `cache clear` was last checked on the C fixture's scratch cache (2026-09-28). |
| litestream v24 (Go) | Ordinary runs exit 0 on 2026-09-29 with 6 of 8 targets (the two C targets fail on missing headers); orientation accepted (`LITESTREAM_CONFIG` and flags in the recipe); cmd/litestream-test's `litestream` launches join cmd/litestream. Open: the misses listed for the owner on 2026-09-28 (flag-set names answered `command`, `setuptools.Extension`, MCP tool names duplicating their AddTool inputs) and answers that drifted on re-asks (`strings.HasPrefix`). |
| freqtrade (Python) | Ordinary runs exit 0 on 2026-09-29 (the latest 256 s with 0 live calls; report.json 14.7 MB); orientation stage 2 accepted, its flow running through the trade registration into Worker.run and Worker._worker. Open: FreqtradeBot.process stays outside every input's reach, behind `_throttle(func=…)`; no online count of the 34 subcommand inputs and no question run of this wave is recorded. |
| repomap self-run | self-snap exit 0 on 2026-09-29 (116 s at e04743b1; 70 s at 66902610 with every atlas request cached); its orientation overview is accepted and its flow request exceeds the provider window (journaled under `flow_request`). The last cold self-run with a warm rerun and `cache clear` is 2026-09-26 (108.0 s, warm 25.1 s). |
| pykrx library target | 2026-09-28 (map model step 3): exit 0, 12 parts. Open: `get_market_ohlcv`, used only by its file's `__main__` demo, is answered helper, tied to the owner's open question on a library's public API as entries. |
| Syn, issue-bot, Watchtower | The latest ordinary reports are the third series of 2026-09-11 (native routes, operation activation, remote client/option distinctions and glossary provenance checked), before the map of parts and the current formats; the saved-window replays of that time (Watchtower glossary and reducer, boundary v5, issue-bot retrieval) check only packing, decoding and source distinctions. Open: Watchtower's worker answer and launch argument, issue-bot's per-call argument exception, a mixed route counter. |
| Airflow | Full current ordinary acceptance remains pending. Do not restart before prerequisite fixes, saved-window checks and Freqtrade acceptance. Old elapsed time is not a measurement of the new builder. |

Artifact consistency, a green fixture, a saved reading and a successful single
window each establish their own limited evidence. None alone establishes model
answer quality, full repository completeness or end-to-end performance. Update
this table after inspecting the corresponding ordinary result; preserve receipts
and limitations in the journal/archive rather than accumulating them here.
