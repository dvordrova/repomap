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
once and annotations once. `report.json` embeds the same ProgramIndexes directly,
not a presentation-specific graph, and stores GroupsIndex as the same thin
semantic overlays. Native group subjects and structural edges exist only as an
in-memory join while the HTML is built.

GroupsIndex v13 is now that thin overlay: `g*` groups, `k*` containers, `o*`
operations and `x*`
connections are deterministic target-local ordinals over canonical content;
subject rows contain only `n*`/`e*p*` refs, categories and interpretations.
Reading it requires the exact bound ProgramIndex and derives native subject facts
and structural edges in memory. No semantic ID is a content hash.

ProgramIndex v16 removes the unused symbol-link hash layer. An object has its
one compact `n*` identity inside a target; the shared scope refers to it as
`t*.n*`. Cross-target meaning exists only in explicit `x*` connections backed
by those existing facts. There is no adapter-specific hidden join key to mint,
persist, copy into GroupsIndex or expose in `report.json`.
Report hydration uses that same target-qualified identity; local `n*` values are
never entered into or read from a repository-wide unqualified map.

- **Shared fact vocabulary (ProgramIndex 17):** every fact is written once
  (no `contains` relations, no derivable counts or empty fields). All adapters use
  one closed `invocation`/`dispatch` vocabulary and one resolution rule: one known
  target is `exact`, several `alternatives`, none `unresolved`. Go signatures use
  short package names, types show only their form, and struct tags become object
  `aliases`. Go resolves interface values it observed, including external types
  such as `*sql.DB`.
- **Registrations (2026-09-17; entries 2026-09-27):** the fact layer names no
  framework. A `registration` is the shape of a non-owned call handing over a
  callable, address or named value, or a C command-table row (D1), with the
  words its call wrote. The reading stage classifies it (`request` for any
  protocol, `command`, `interaction`, `scheduled`, `continuous`,
  `queue_consumer`, `extension`, `http_client`, `db`…) and names an entry by
  choosing among its written words, which code restores verbatim. There is no
  HTTP branch (owner: "там есть GRPC, UDP, TCP, WEBSOCKET … из одного на
  поверхности должно лепиться при помощи моделей"). Operations, routes,
  client calls and portals follow from accepted decisions. `sql_query` is a fixed `db` boundary, recorded only for a literal
  with SQL statement structure (the shared `internal/sqltext` admission, not a
  leading verb; a table filled in by `%s` or a template hole still counts). Without a model there are
  candidates, not routes; the Echo preset test is the offline acceptance.
  Since 2026-09-28 (inputs pass 1, every language; per call since the
  per-call change) each call outside tests that gives an outside symbol a
  word is asked `enters` on its own, what the words become
  (`repomap.atlas.enters.v1`: the symbol, the call as written, its
  declaration, every literal, where each argument comes from, the
  symbol's talks answer), from the one entry criteria file every entry
  question reads, after its symbol's `talks`; a call whose symbol talks to
  other programs or serves, a call of a symbol handed a callable and a call
  another fact names are not asked. A call answered an entry kind is that
  entry. Such an entry, like a value handed over, has its handler not
  established (`handler_unknown`): declared in its caller's part, with no
  subject, reach or phase, no arrow into a part, named by its words. The
  outside symbols' `usage` is the call as written; each symbol's answer and
  each call's are remembered on their own by what they show (never a
  line), an uncertain one as undecided. Every language has this (S2a);
  what each language does not have yet is listed in its contract. The entry
  criteria, the api prompts and the asks are frozen at their digests
  (CHANGELOG; entry criteria re-frozen when `per_call` went).
  Since 2026-09-28 (runs another program) `talks` offers `runs_program`: a
  call that starts another program is one outgoing boundary per launching
  call (a call on its result folds into it), Jev chooses per call which of
  its words names the program (`atlas_program`), and no call of a symbol
  that talks to other programs is asked what its words become. dynamic_execution no longer names
  process launches (the constitution's example list still does: owner).
- **Map of parts (2026-09-25, owner's proxy spec; the owner's open questions
  1–7 of that spec still stand):** one DeepSeek request per target groups the
  target's units into named parts from code structure only (paths, names,
  exported signatures, exact call sites and file imports between rows, a box
  carrying those of the whole files that joined it, 2026-09-29); no README,
  AGENTS, docstring or package documentation reaches it. Since 2026-09-28 the
  role split runs first and a unit is a whole file (`f*`) or one box of a
  split file (`c*`, parts request v2). A declaration takes its unit's part, a
  method its type's, a lexical child its parent's; a place takes its
  declaration's part, and a file's own part is the one part holding all its
  placed units (one rule for every file, no split-file branch). The answer is
  validated unit by unit; a unit left out or listed in two parts gets one
  closed-choice placement follow-up, and only what that cannot place stays
  off the map with its reason. Each drawn part is described
  from its members' names and signatures in a separate request; a refused
  description is an explicit no-description state. Areas are a closed split of
  the described parts asked beside the keys, with no count. The Jev zone
  tables, the 0.50 floor, the proposal catalogue, the file inventory boxes and
  the model's `tests` role are removed; a part made only of test code leaves
  the canvas by the `TestSources` fact. The atlas and GroupsIndex carry an
  explicit off-map record and `map_failure`; the component card lists Tests
  and Not on the map. A parts or areas answer refused whole, including one
  that decodes but draws nothing or is cut at the output cap, fails that
  target's map for the run; there is no second draw.
  Since 2026-09-27 a file can go in several boxes: a Jev gate, a DeepSeek
  naming of its boxes and a Jev assignment of its declarations (READING, "A
  file in several boxes"); a file nearer the cut stays whole and undecided
  declarations stayed off the map as `undecided` (each is a row of its own
  in the parts request since fix B, 2026-09-29). Since 2026-09-28 a
  declaration the assignment leaves open goes, by code, to the box where
  every declaration of its file that calls it, is decorated by it or reads
  it went (never a hand-over), else where everything it uses went; the
  neighbours' second question is deleted. Since 2026-09-28 (map model
  step 3) Jev also asks once per unit whether it is a helper, the code of a
  responsibility or none of these, with no `exported` field: a function,
  method, lambda or variable nothing in the program uses is none by code
  and not asked, while types and module bodies always are; only a decided
  helper is one. A helper is never named or assigned: code places it with
  its users (callers, decorated units, readers; never a hand-over), a
  whole file every declaration of which, its types included, is a helper
  joins its users' box, and only a helper its users share between rows, or
  one nothing uses, is asked the assignment once more in a second pass of
  its own (a near-tie leaves it undecided). Since 2026-09-29 (fix B) a
  unit no box took is a row of its own in the parts request, named by its
  declaration as a seed's row is, which the parts step places with its
  calls or leaves `left_out`; its box question stays undecided in the
  record and its helper mark is the helper question's (Redis's
  lookupKeyRead is in Server core state). A callable taking a repository
  type as a parameter is that type's user (places `takes`), so rule C
  places a type by its takers (iojob goes with freeIOJob and queueIOJob;
  Clojure has no parameter types). The atlas symbol and GroupsIndex
  carry the helper mark; an arrow into helpers is quiet like
  initialization, with its exception (a program all of whose arrows would
  be quiet draws them); an area is purple when any part in it is the
  domain, the entry area included; a part's tiles stand keys, then types,
  then the rest. Only the part holding the program's launch point (a
  target seed), and its area, is the entry (GroupsIndex 19): a part that
  takes requests or listens is not, and a launch point no part holds makes
  no entry part and is named, with its off-map reason, in the component's
  reading and its "Not on the map" list.
- **Map model step 4 (2026-09-28): one reach.** GroupsIndex derives, with
  one function on projection and hydrate alike, what each input's handler
  reaches (calls and terminal reads, never an alternatives relation into
  another input's handler), the dispatch sites and the inputs reaching
  them, hand-overs from running code, the phases (runtime is any input's
  reach, init what only the seeds reach) and which arrows are quiet; the
  report walks no code. A pinned input draws every call into a part from a
  part reached earlier and counts the rest (depth layering, a lead decision
  awaiting the owner, below); its
  reading says where it is dispatched from and that how a request for it
  gets to the site is not established, and apart what its handler itself
  calls there; the site's own reading lists the inputs reaching it. An
  entry keeps a line only when the model wrote one. Chains and operation
  types are deleted.
- **Follow-ups of sanity check 3 (2026-09-28).** The no-users rule applies
  only to a kind whose uses the adapter records (`recordedUses`, per
  language and kind): Go variables and Clojure macros (ProgramIndex
  `macro`) are asked like types. A Go package-level variable is the caller
  of its initializer's calls (direct-call index 15). A call through a stored
  function value is no user for placement, the other half of a hand-over.
  The second pass repeats rounds until no waiting helper qualifies; a
  helper no round could ask is off the map as `blocked`, apart from
  `undecided`.
- **Owner decisions of 2026-09-27/28 (map model step 3)**, what the owner
  said: ask once whether a declaration is a helper; shared helpers go to a
  second pass (his "a"); arrows into helpers are quiet like
  initialization; an area is purple when any part inside it is the domain;
  types stand right after keys among a part's tiles; the amber DNS group
  stays (one DNS resolver box with three arrows, the planned deletion is
  void); files are split into role boxes before the units are grouped into
  parts; no utils part. Still open for the owner: a library's public API
  as entries (an `export` seed kind), which would make an entry never a
  helper by code.
- **Lead decisions awaiting the owner's review (2026-09-28)**, taken by
  default and shipped, each his to keep or undo:
  - rule B read literally as "a file of helpers": a whole file joins its
    users' box only when every unit it declares, its types included, is a
    decided helper (Redis: staticsymbols.h, lzf_c.c and lzf_d.c join;
    pqsort.c stays whole);
  - (2026-09-29, delegated, skeptic and probe) a file that joined a box
    gives that box its imports ("c4 -> f13", lzf_c.c's include of lzfP.h);
    no single-user join rule and no per-row `called_from` field;
  - the all-quiet exception: when every arrow of a program would be quiet,
    its calls into helpers are drawn, so quieting never empties a map;
  - depth layering in an input's path: every call into a part from a part
    reached earlier is drawn and listed, the others counted;
  - the "init" label covers the launch and the main loop (aeMain,
    serverCron), everything only the seeds reach, not startup alone;
  - a seed no part holds is named in the component's reading column (and
    its "Not on the map" list) with its off-map reason, not on the canvas;
  - S4-6 deleted GroupsIndex's chains and operation types, which nothing
    read after step 4;
  - C4 (the helper question) shipped despite its one known miss, pykrx's
    `get_market_ohlcv` (its only user its file's `__main__` demo) answered
    helper;
  - the no-users rule with `recordedUses`: a declaration nothing uses is
    none by code and not asked only for a kind whose uses its adapter
    records; other kinds are asked like types;
  - u5's depth-1 fold: an input's path opens the parts its handler calls
    directly and folds every deeper part under "Reaches {n} more parts
    deeper" (for GET the open part holds getGenericCommand alone).
- **Map reading on the canvas (2026-09-25):** a part's description stands
  on its box under its name, a closed area's line on the area's box in the
  whole lines it leaves; a loose part beside areas is drawn at a peer's size.
  Looking at an area or a component darkens only the arrows that cross its
  border. An outside call in the component card is named by its destination
  (`DeepSeek · Client.Do`). Left for the owner after an owner-proxy review:
  the number chips on parts and frame borders (explain or remove), areas in
  the model's pipeline order (GroupsIndex containers carry no position; they
  sort by value), and a part's inside showing a dozen of hundreds of
  declarations.
- **The page needs JavaScript (2026-09-29, owner decision "b"):** the page
  is one self-contained HTML file whose data holds every answer, each fact
  once; the script reads it. Nothing the reading column renders from the
  data is printed again (no part cards, connection rows, source index,
  input catalogue rows, state changes or static drawing; glossary files
  written when opened); a `<noscript>` line says so. The page data is
  written compactly and read back exactly (REPORT). A function reads as its
  flow, its calls in written order grouped by part, helper calls named on
  one muted line under their step (a call into its own part is its work);
  an input opens at how a request reaches it (REPORT).
- **Decoders (2026-09-26, owner: "валидаторы и строгие декодеры нам уже 30
  дней палки в колеса вставляют"):** the one-time identical-bytes resample is
  deleted; a decoder refuses only what is wrong, at the smallest scope, and an
  identical repeat, an extra field, a wrapper, case or whitespace, null for
  empty or a string for a list is not a refusal. A refused table cell loses
  only itself; one bad (question, row) loses only that cell; one bad
  translation entry, glossary group, Learn review, concept or portfolio batch
  loses only itself; an unambiguous doubled closer, a prose tail and an
  identical second root are syntax, not refusals. A cached answer the current
  decoder refuses stays on disk for a later decoder. A refused answer, or an
  accepted one with a part refused, keeps its bodies in its run's
  `payloads/` for debugging; the cache holds only accepted answers, and
  `cache clear` leaves runs readable. Refusals that keep the
  report true stay: unknown refs, a missing or out-of-choice required
  decision, two different answers for one row, unsourced substantive answers.
  Owner decisions of 2026-09-26: a Jev choice is taken by a lead of 0.10
  over its runner-up (near-ties stay uncertain); the key declarations, part
  roles and keys are decided only through one categorizer interface
  (`llm.Categorizer`, Jev its only implementation), `JEV_KEY` is required
  for a model run and `read`, and there is no DeepSeek fallback; an HTTP-200 answer without
  content or with insufficient_system_resource gets one transport retry; an
  unknown default-target answer leaves the default unresolved and the report
  is published; glossary terms match in any case and with a plural -s/-es
  (a code declaration's name only as written, an acronym's plural in lower
  case); README-backed recipe steps stay refused, with no labels ("пометки
  меня бесят"); the map legend explains the number chips; areas keep the
  order the model gave, carried by code with no rule for the model. A table
  consilium (66-call DeepSeek probe) found the row format sound: 0 invented
  keys in 79,363 rows. It found empty-means-no optional cells unreliable. So
  the api table no longer asks its five unread columns; prompts demand only
  the cells fill advertises; keys and core get a part's whole declaration
  list; a symbol row writes identical calls once with their lines. The
  English alias is asked, by code, only of a name with a letter outside the
  Latin script, with or without captions.
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
- **Operations:** v19 asks whether evidence supports this declaration's task and
  its independent activation (`self`/`none`). Process entry or asynchronous launch
  alone does not establish a responsibility; a task may delegate work to helpers.
  Caller/setup context is evidence, never a handoff choice. Persistent consumers,
  cron and supported one-shot scheduled work remain distinguishable from
  listeners, lifecycle hooks and middleware within the same request. Original
  own-call receiver/arguments/API distinguish effects on the request and response.
- **Boundaries:** v5 asks fixed native facts only for explanation and applicable
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
- **Questions and orientation:** question-batch v3 assigns relevance per original
  anchor, allowing different roles inside one evidence chunk. Question and
  boundary evidence preserve safe native receiver/arguments/API and exact call
  sites; declared-interface identity retains unresolved dispatch observations.
  Orientation receives original member evidence too. Document heading
  ancestry preserves author scope without treating a model file-role hint as a
  native exclusion rule. Retrieval/answers distinguish task responsibility from
  HTTP hosting and preserve differing call arguments; launch recipes retain
  supported required arguments. Owner decision 2026-09-25: a declaration's
  own body may be sent to a provider for its caption. A decision model ranks
  a part's declarations and the top ranked (about ten) go with their bodies
  in one request; the rest stay in the analysis without a caption, visibly.
  Whole source files still never enter provider bodies, and the repository
  remains trusted input, not a security boundary.
- **Execution:** shared reasoning/output allowance is 128,000 tokens, subject to
  the provider ceiling. Output refusals partition independent questions first;
  input/context refusals preserve and repartition complete evidence. A new
  limit/packing policy is tested on one saved complete window before a full run.
  An exact request already in the air is asked once and its twins read that
  answer, so targets sharing a part get one description and a warm rerun
  makes no live call.
- **Glossary:** only accepted prose enters a separate p-ref generation/reduction
  pass. Generation is three steps, each one decision (owner, 2026-09-28):
  DeepSeek lists names, Jev decides per name domain concept / general
  vocabulary / code element (a near-tie stays undecided), and DeepSeek
  explains only the decided domain concepts; code attaches every row that
  writes a name. Each text step has its own 32,768-token output allowance: legitimate windows need 1–16 thousand tokens, and two Watchtower windows looped to 128,000. Source/prose/fill context
  stays local in Prepared and the accepted cache record; no deferred g-ref
  appendix is sent. Original scope survives current warm execution, entity and
  question memos, and exact replay. Reduction v5 factors repeated source anchors
  and complete source sets into request-local catalogues. Every original
  definition and scope survives, including each independently prepared child.
  Both owners use the shared exact-request split memo. Exhausted provider-local
  timeouts remain optional refusals while the run itself has not been cancelled.
- **Report:** compact named inventories precede maps, with five initial rows and
  All N retaining the remainder. Calls, workers and source-linked data must be
  easy to find. Code names/paths/addresses remain original; English aliases and
  operation labels coexist with localized descriptions. Saved rendering uses
  ordinary templates and no provider or cache. Detailed display/data changes
  retain their consuming contract and require fresh ordinary acceptance. Data
  links use existing exact model/callable ownership; an unresolved endpoint or
  unowned SQL literal cannot acquire a database flow in the report.
- **Where an input takes effect (consilium 2026-09-28, owner answers Q1 and
  Q2 yes; built in speed mode, tested in the repair pass):** a seed of a
  split file is its own grouping row, so an executable's `main` is on the
  map (e1), and a unit only the seed uses joins that row. Inputs without a handed function form catalogues keyed by the
  object they are declared on (the call that made it, followed through
  outside calls that name nothing), else by the declaring function (L3); the
  reading says "Declared on/in", "called from", "F also uses V" (never
  "takes effect") and once "Where these take effect is not established." A
  wordless hand-over on a word entry's own result of the same kind is one
  input with it (J1). Such an input draws the ordinary Inputs arrow into the
  part where its code takes it in (declared in, looked up in). GroupsIndex
  derives the launch walk from the seeds and load-time roots (Go init and
  package variables, module bodies outside C) with found / unsure / could not
  look inside (the function's unresolved calls) / nothing per function,
  shown only in the Inputs reading's fold; words only an input's
  handler checks are its sub-arguments, never tiles. `enters` is asked
  of each word call on its own (no symbol-level answer, no `per_call`),
  and every literal is sent. C: a table's
  rows that store no function and a callable the program's own function
  keeps are asked once each (atlas_inputs); an accepted table is one
  catalogue "looked up in" its readers; a table row names the peer program's
  input only through the joints peers question (Operation.Sends, never an
  arrow); a dispatched input's reading names every outer input reaching its
  dispatch site, the registration hop included. A launched program whose
  word equals the name the repository's build gives one of its own
  programs (ProgramIndex 21 `target.executables`: C's link output, a Go
  main package's `go build` name, a Python console script, a package.json
  bin command; Clojure none) is that program (2026-09-29): its record
  reads "Runs this repository's program", and on the system map the call's
  arrow goes into that component with no outside tile (cmd/litestream-test
  → cmd/litestream; a program starting itself keeps its tile). Equal names
  only; the report joins them (`programsNamed`).
- **Field readers and writers (2026-09-29):** a record type's reading
  gives each field "Written by" and "Read by", the functions by part; a
  global variable's lists the fields reached through it
  (`server.masterhost`); a function's says "Writes: …" once and
  "Reads: …" once, the fields by their paths and the global variables it
  reads, in place of "Uses variables". No column list prints a line number:
  a caller, callee or variable is its name, once (REPORT). From C field
  paths, Python typed receivers and JS/TS declared properties.
- **Benchmark v3 fixes (2026-09-29, REPORT):** a flow's helper calls are
  named on one muted line under their step ("+ helpers: …", each a link),
  and a call into the caller's own part is its work, never folded; a call a
  macro's expansion makes reads as the macro as written, a compiler builtin
  as no call. A Main flow step citing a registration, or naming a callable
  a registration hands over, reads as that callable with where on the
  flow's path it is registered and what runs it (the event loop's chain
  from main), never the registrar or an arbitrary site; the component
  reading holds the one Main flow and every reading of the component has
  one "Main flow" link; a program no model flow passes opens its entry's
  calls. A setting reads by its key with the fields its branch writes (C
  pattern `branch`, PROGRAM_INDEX). A TODO is a comment's marker in a code
  file (facts). A C field records its repository type (`types`), which its
  row links. A column click shows a tile with its whole part; a canvas
  click never moves the camera; "Back to map" restores the page as it
  stood; a reading reached anew opens in its default state.
- **Benchmark v4 fixes (2026-09-29, REPORT):** the column has no scroller
  of its own inside it: a dispatcher's "Its handlers by input" folds under
  its count, each handler a link like every name. A kind chosen in an
  Inputs frame (Settings) opens the collection at that kind's section.
  "Also runs on its own:" follows the Main flow: the program's scheduled,
  then continuous inputs, each its registered callable with what registers
  and what runs it, from saved registrations and kinds only (Redis
  serverCron and IOThreadEntryPoint, freqtrade's four threads, litestream
  none); a call through a function value resolved to one callable runs
  it. A Main flow step's name links all of its function's code and
  "registers it" the registration line; its twist is always shown. A
  column name chosen in a part too dense to read at a fitting zoom shows
  the part's closed card with the chosen tile in it. A part lists the
  declarations reached from outside it first, counted. A caller, callee or
  flow call carries a "</>" mark per call site linking that line, its
  place on hover, no number. The canvas location row's frames go up a
  level; every reading ends with the home's text-page links.

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
This wave uses ProgramIndex 21, places graph 21, reading input 19, atlas 19,
GroupsIndex 25, dependency catalog 2, extraction artifact 2, facts 4, claims 2
and target outcomes 3 with compact artifact-local IDs and no saved adapter
SourceRefs. Dependency catalogs assign canonical `i*` importers and `d*`
dependencies. Accepted extractor nodes use `u*`; arbitrary public-protocol IDs
remain only in the exact recorded stdout.
The current reading input and atlas carry the current semantic result with
target-qualified native object refs and short graph/part/area/joint IDs, in
addition to heading context and original dispatch observations,
operations v19, boundaries v5, question-batch v3 and local cache record v3.
Question routes are v10 and knowledge is v3: both retain the same compact
question/place IDs used by provider rows.
Native adapter constants remain in their owning language packages; genuine
fixtures are regenerated through those adapters when their format changes.
There are no old-format readers or manually rewritten seals.

## Acceptance and open work

| Scope | Current status |
| --- | --- |
| Local corrected contracts | Full `go test ./...`, `go vet ./...` and `make build` pass. The standalone Echo/sqlc/Cobra module passes its own test/vet. A fresh ordinary no-model API run completed with browser-checked handler → converter → service → repository → sqlc navigation; its saved dependency/extraction refs and report DOM IDs are compact and unique. |
| Saved Watchtower glossary window | One controlled output-budget probe completed: same 433 rows/727 texts, 21,040 input, 8,903 output tokens, 28.803 s, finish=stop. 216 term rows, 211 exact occurrences; five unsupported optional names discarded. Only structure/refs/text checked; original-source provenance still needs ordinary acceptance. |
| Saved Watchtower reducer window | The complete 458-variant input shrank from 4,733,303 to 502,076 provider bytes by factoring source scopes. One replay completed in 15.503 s (155,669 input / 5,502 output tokens); current validation accepted 412 groups retaining all 458 original variants and their exact sources/origins. This checks packing and restoration, not every semantic merge or a new ordinary report. |
| Saved boundary contract windows | One issue-bot replay retained its client constructor and five GitLab dispatches while rejecting the option factory; one complete service window retained OTLP construction and HTTP dispatch while rejecting local delegation. Current v5 decoder accepted all rows. This checks those source distinctions only; destinations, answer quality and full ordinary reports remain separate acceptance. |
| Saved issue-bot retrieval response | Current per-anchor validation accepts 21/21 original rows (previously 18/21); offline response validation only. |
| Syn / issue-bot / Watchtower ordinary reports | Third series completed in 225 / 575 / 307 seconds, with 30 / 20 / 27 questions and no unavailable answers. Native routes, operation activation, remote client/option distinctions and glossary provenance were checked. Watchtower's worker answer and required launch argument, issue-bot's per-call argument exception, and a mixed route counter still needed correction; current follow-up checks remain pending. The issue-bot glossary exhausted 128k output before lossless splitting. |
| repomap self-run, no `--target` (2026-09-25) | `.bin/repomap .` cold, report server ready: 58.2 / 57.2 / 66.2 s over three consecutive runs (two targets), each reaching the glossary at 46–47 s; `make test`, `make vet` pass; one browser walkthrough (map, glossary page, part card connections). The remaining spread is model output: glossary draws of 37–90 terms took 7–17 s, enumerating draws (165–618 terms) 23–60 s plus a reduce pass; a design proposal that loops to its 8,192-token allowance is refused and leaves that target's map as file inventory. |
| Decoders relaxed, ordinary self-run (2026-09-26) | `.bin/repomap .` at f819f4be on a checkout named `repomap`, own cold cache: exit 0 in 108.0 s (an enumerating glossary draw took 48 s; the previous run's took 15 s). CLI 53 parts in 8 areas, UI 5 drawn parts and 1 test-only part, no map failure, 0 data rows; the refusals left are rules that keep the report true (invented guidance refs, unsourced glossary terms, Jev core roles under the floor, the oversized symbols window). Warm rerun exit 0 in 25.1 s; `cache clear` removed 47 MB; `make test`, `make vet`, `make build` pass; Playwright walk without page errors. The 2026-09-25 map-of-parts run (36 and 61 parts in two draws) is in the journal. |
| Redis 1.3.6, C (2026-09-27) | `c/integrate` binary: four programs from `make -n`; cold ordinary run exit 0 in 27–38 s, warm rerun 3 s with 0 live calls and a report.json identical but for `timing`; `cache clear` exit 0; `make test`, `make vet` pass. redis-server lists 97 command requests and one continuous thread. Open: the accept handler and serverCron (handed to Redis's own event loop), redis.c drawn as one "Core server" part until the role split lands, outside boxes repeated per component on the system map. |
| Redis 1.3.6, map model step 3 (2026-09-28) | Helper question, code placement and second pass, quiet helper arrows, area marks, tile order (C4–C6 and the rule B fix), on the default system response cache: ordinary run exit 0 (the C4 cold run 33 s; the final run 25 s with one live parts request), warm rerun 6 s with 0 live calls; `make test`, `make vet` pass. The helper question asks 654 declarations of the four programs in 46 per-file Jev requests and takes 420 for helpers; redis-server: 479 asked, 284 helpers, 21 near-ties, 20 taken out as used by nothing; processCommand, serverCron, rdbSave, syncWithMaster and call are responsibility, lookupCommand a helper. redis.c splits into 18 boxes (none empty, lone or only helpers); code places 82 helpers with their users, joins staticsymbols.h, lzf_c.c and lzf_d.c to their users' boxes (pqsort.c stays whole: `_pqsort` is a near-tie) and places 1 open unit; the second pass asks 62 shared helpers and places 58; 19 units stay undecided, 13 of them helpers blocked by an open user and `main` among the rest (0.54 against 0.46), so redis-server has no entry part yet. 21 parts, 0 lone. At rest the redis-server map draws 19 arrows (6 two-headed) and its runtime area 10 (7 two-headed); headless walk without page errors. `cache clear` was not run on the owner's system cache. |
| pykrx library target, map model step 3 (2026-09-28) | `--target python:.:library:library`, exit 0 (cold 31 s, rerun 3 s with 0 live calls): 188 declarations asked in 24 Jev requests, 50 helpers, 107 taken out as used by nothing; the other three dispatchers (`get_market_cap`, `get_index_ohlcv`, `get_etf_isin`) are out by the no-users rule. 6 of 24 files split into 43 boxes, and the grouping put each split file's boxes back into one part; 12 parts, 0 lone, 4 units (8 declarations) undecided. Known miss: `get_market_ohlcv`, used only by its file's `__main__` demo, is a helper (lead 0.23), tied to the owner's open question on a library's public API as entries. |
| litestream v24 `cmd/litestream`, Go, map model step 3 (2026-09-28) | Ordinary run exit 0 (cold 2m6s with 58 helper and 52 gate Jev requests over six targets; after the rule B fix 65 s), warm rerun 7 s with 0 live calls. main.go splits into 3 boxes; the six storage backends stay out of its box (each `ReplicaClient` type is responsibility) and stand with file and s3 in one "Replica clients" part; 12 parts (11 drawn), 0 lone, 2 units (4 declarations) undecided; `replica_url.go` is a part made only of helpers. The map draws 16 arrows at rest, 18 in all. cmd/litestream-test keeps 2 one-unit parts (a box of its split main.go and shrink.go). |
| Redis 1.3.6, map model step 4 (2026-09-28) | One reach in GroupsIndex (S4-1–S4-6) at 08f6a3ce on the default system response cache: ordinary run exit 0 in 6 s with 0 live calls (the lanes' one orientation call and one glossary call were made by the S4-4 run), warm rerun 5 s with 0 live calls and a report.json identical but for `timing`; `repomap render` byte-identical to report.html for both; `make test`, `make vet`, `make ui-test`, `make ui-visual-test` pass. GET is dispatched from call and from loadAppendOnlyFile ("How get reaches call is not established."), its reading lists no route; call's own reading lists exec, lpush, rpoplpush, rpush and slaveof with their calls, loadAppendOnlyFile debug. GET draws 15 part-pair arrows (12 before), exec 11 (17). IOThreadEntryPoint reads "Registered by" 11 inputs. redis-server has no entry part (main is undecided off the map) and its reading says "The program's entry is not on the map: main:9124 · In no part of its file"; 10 command and networking parts lost the entry mark. 96 of 96 entry summaries that restated the registration are empty. Arrows at rest unchanged: redis-server 19, redis-cli 4, redis-benchmark 5, redis-check-dump 1. report.html 12,555,599 → 12,268,850 bytes. Headless walk (Find → get, name reads, modifier-click opens, why it appears, outside this path, reload with GET pinned) without page errors. |
| litestream v24, Go, map model step 4 (2026-09-28) | Ordinary run exit 0 in 24 s (orientation and glossary asked once live: the lanes changed), warm rerun 7 s with 0 live calls, report.json identical but for `timing`, render byte-identical. cmd/litestream keeps one entry part (Command entry point, 5 before) and its area; cmd/litestream-test one (6). 33 entry and 16 outgoing summaries that restated their facts are empty. Arrows at rest: cmd/litestream 16 = 16, cmd/litestream-test 2 → 3. report.html +680,407 bytes, almost all a longer glossary from the live glossary answer (47 terms, 42 before); `data-call-paths` −160,964, `data-input-path` +37,649. |
| Redis 1.3.6, follow-ups f1–f3 (2026-09-28) | At 030dbc9e on the default system response cache: ordinary run exit 0 in 27 s (23 live calls: 3 assignment windows re-asked, then the parts' descriptions, areas, keys, core, zones, glossary and orientation that followed; 138 cached), warm rerun exit 0 in 4 s with 0 live. redis-server off the map 19 → 6: 13 helpers left waiting by order → 12 placed (7 asked in round 2, 5 by rule A after it) and 1 `blocked` (resetServerSaveParams, whose user main is undecided); near-ties 4 → 3 helpers (re-asked window: checkType now a near-tie, createZsetObject and oom placed), main and selectCommand unchanged. dupClientReplyValue no longer follows listDup into adlist.c's "Core data structures": asked in round 1, it is in redis.c's "Server core state". Second pass: round 1 asked 63, placed 60; round 2 asked 7, placed 7. Headless walk: exec reads "How a request for exec gets to call is not established." and "exec's handler itself calls call, where 95 inputs are dispatched:"; "Used by code in no part" is listed; no page errors. |
| litestream v24, follow-ups f1–f3 (2026-09-28) | Ordinary run exit 0 in 7 s, 0 live calls (195 cached). No Go package-level variable of its targets calls repository code, so nothing moved: cmd/litestream keeps its two undecided helpers (IsSQLiteDatabase, txidVar), second pass one round (14 asked, 12 placed), nothing blocked. etcd's server library (no-model probe): addNewField ← `calls exact` from the variable schemaChanges; GetCluster, filterNoPut and filterNoDelete are function values and still have no relation (GO's recorded gap). |
| Redis 1.3.6, inputs pass 1 (2026-09-28) | Default system cache: the first run (c9ca55bd, A3 before its registration-words fix) exit 0 in 23 s with 11 atlas_api Jev windows and 3 joint windows live, once; at 28a57619 two runs of 18 s and 11 s with 0 live, report.json identical but for `timing`, `repomap render` byte-identical. `strcmp` answered `command` (lead 0.64), every one of its 18 word-given calls compares `argv[i]` in redis-cli's and redis-benchmark's `parseOptions`: redis-cli 0 → 6 options, redis-benchmark 0 → 11 (handler not established; the two `-h` sites of one caller are one input); `strcasecmp`, `printf`, `fprintf` none; redis-server 96 = 96, GET's reading unchanged. In-view arrows at rest: redis-server 20 → 15 from the neighbours' new layout (its own frame and saved connections unchanged), redis-cli 4, redis-benchmark 5, redis-check-dump 1. |
| litestream v24, inputs pass 1 (2026-09-28) | First run (c9ca55bd) exit 0 in 130 s (36 atlas_api Jev windows; boundaries, core, joints, publish, glossary and orientation re-asked once); at 28a57619 11 s with one boundaries window live (the re-worded registrations' names), warm 9 s with 0 live, report.json identical but for `timing`, render byte-identical. cmd/litestream 33 → 93 inputs (the 14 `X.Usage` commands and 3 errgroup goroutines leave by the merged criteria; its FlagSet options, flag-set names and 7 MCP tool names arrive with their handler not established, and `svc.Run`, re-asked with its call as usage, no longer serves, so the Windows service registration is an extension entry instead of a listener), cmd/litestream-test 6 → 34. Misses for the owner: `exec.CommandContext` answered `command` (12 calls launching `litestream` subprocesses, 10 inputs), `flag.NewFlagSet` `command` (20 flag-set names), `setuptools.Extension` `extension` (1); MCP tool names duplicate their AddTool inputs. Arrows at rest 16 = 16, 3 → 2. |
| Runs another program (2026-09-28) | `runs_program` talks outcome, per-call `atlas_program` Jev question, entry refused beside any talks but none. Redis 17.7 s / warm 15.3 s 0 live, inputs unchanged; litestream 41.7 s / warm 8.3 s 0 live, launch inputs 11 → 0, `litestream` launched ×9 (cmd/litestream MCP) and ×3 (litestream-test), 4 launches not established; repomap self (`cmd/repomap`, cold) git ×4, go ×2, clj-kondo, python3, 2 named at run time, 8 not established. Own-executable links not done (joints pair two boundaries). Full make test/ui tests not run. |
| Inputs: catalogues, launch, C tables, peers, outer inputs (2026-09-28, speed mode) | Default system cache, no `cache clear`, tests owed (scratchpad `impl/cleanup-todo.md`). Redis exit 0 in 20.8 s / warm 19.6 s, 0 live: redis-server's entry part holds main; redis-cli 6 options in parseOptions's catalogue and 94 cmdTable requests "looked up in lookupCommand, called from cliSendCommand :311, main :522", 91 of them naming redis-server's input (model match, no arrow); acceptHandler request and serverCron scheduled; GET names both as outer inputs with their registration hops. litestream exit 0 in 29.7 s (22 Jev windows live) / warm 8.8 s 0 live: one catalogue per FlagSet, registerConfigFlag called from 6 Runs, 75 inputs (115 before). repomap self 104.9 s, 36 inputs in 5 catalogues. freqtrade cold 430 s: its subcommands are unresolved calls (Python adapter gap), 0 commands. Headless look without page errors. |
| `enters` per call (2026-09-28, speed mode) | Default system cache. Redis exit 0 in 7 s (314 per-call questions, 301 decided; 29 atlas_api Jev requests), warm 6 s 0 live: inputs server 98 → 104 (+6 settings-file keys of loadServerConfig answered command: C records `argv[0]` as not followed), cli 100, benchmark 11 unchanged. litestream exit 0 in 14 s (1,440 per-call questions, 1,425 decided; 135 requests), warm 9 s 0 live: cmd/litestream 106 → 77 (31 `strings.HasPrefix` words gone; `-` twice stays; MCP argument names ±2), cmd/litestream-test 25 unchanged, every flag kept. Jev ≈ $0.12 runs + $0.08 measurement. |
| Launch on either branch (2026-09-28, speed mode) | Go φ → `alternatives` in edge order (a join expanded once per value); the launch fold takes a receiver all of whose alternatives are launches. litestream exit 0 in 1m48s on the default cache: cmd/litestream-test's CombinedOutput folds into both `litestream` launches (not established 1 → 0), cmd/litestream 2 and s3_mock 1 unchanged (no word). Re-asks drifted: `strings.HasPrefix` answered `command` (31 false inputs in cmd/litestream), azblob named "S3 storage", glossary 263 terms. Python and JSTS give `unknown` (missing equivalents). |
| Glossary in three steps (2026-09-28, speed mode) | At 84f54d94 on the default cache: litestream 149 names → 55 entries (263 before), report.html 13.88 → 11.12 MB; Redis 64 names → 24 entries (20 before), its concepts section −168 KB. Warm reruns and the bc41d667 runs 0 live, glossary.json byte-identical. On the saved windows, 5 draws: 54–61 accepted per draw against 45–270 terms of the one open request. Jev misjudged vacuum and lease (code element), Google Cloud Storage and SFTP (undecided). |
| Settings from struct tags (2026-09-28, speed mode) | At 2e1ac061 on the default cache: each Go field whose tag names a key is asked `setting`/`none` (Jev) with its structure's use from go/types type declarations. litestream 200 fields: 96/96 YAML setting, 102/104 JSON none; Config's 17 declared on `yaml.Unmarshal(buf, &config)`. Redis: redis-server 37 settings, redis-cli 6 and redis-benchmark 11 options stay commands. No DeepSeek call; Jev < $0.01. |
| Repair pass after the speed-mode sequence (2026-09-28) | At 4fb25a6a: `make test`, `make vet`, `make ui-test` (131) and `make ui-visual-test` (63 passed, the 5 real-run specs skipped) pass; the Go runs used a `-overlay` restoring the Go SDK's `slices.go`, into which text had been typed (CHANGELOG). Fixture tests for every step of the inputs sequence, each rule's test failing when the rule is reverted (47 rules). Redis on the default cache: `make build` binary, exit 0 in 14.5 s and warm 6.7 s, 0 live calls both, report.json identical but for `timing` (and to the 12:57 run), one common manifest/report JSON and report.html for four programs; "could not look inside" now lists aeProcessEvents's calls through rfileProc/wfileProc. The C fixture on a scratch cache: cold 91 live calls in 24 s, warm 0 live in 3 s, `cache clear` exit 0, then 79 live again; the real models answer --symbols command, port and dbfilename settings, acceptHandler request, and misjudge staticsyms.h's symsTable rows as requests. The `REPOMAP_REAL_RUN` specs on that Redis run fail 2 of 3 (breadcrumb after a part click; two connection cards open at 1280×800), UI. |
| C field reads and writes (2026-09-29) | ProgramIndex 20: each C read or write of a repository record's field is a `reads`/`writes` relation to the field with its `field_path` (C), carried as GroupsIndex edges and the places graph's `fields`; no model request reads them. No-model redis-server: 3,088 field accesses of 7,249 relations; `server.masterhost` written by initServerConfig, loadServerConfig and slaveofCommand ×2, read by genRedisInfoString ×3, slaveofCommand ×4 and syncWithMaster; `server.replstate` written by initServerConfig, loadServerConfig, freeClient, syncWithMaster and slaveofCommand ×2, read by serverCron and genRedisInfoString; `redisDb.expires` written only by initServer (as `server.db.expires`), read by 12 functions (deleteKey, expireIfNeeded, serverCron, …). Go, JS/TS and Clojure record no field writes (GO, JSTS, CLOJURE). Saved runs from ProgramIndex 19 no longer render. |
| freqtrade subcommands, no model (2026-09-29) | A Python field stored once from a call carries it (PYTHON): `self.parser.add_subparsers(...)` is argparse's, all 34 `add_parser` subcommands are exact word-given calls, each joined by its result to the `set_defaults(func=…)` handing its handler over; an online run has not confirmed the inputs yet. |
| Report batch, "b", compaction and flow (2026-09-29) | Runs at 68d6d4c5 on the default cache, rendered at 656e8a16: Redis exit 0 in 11 s (0 live), 28.96 → 4.27 MB; litestream v24 exit 0 in 74 s, 9.23 → 3.92 MB; freqtrade exit 0 in 319 s (orientation refused by context size), 46.54 → 9.30 MB; self-snap exit 0 in 124 s, 37.34 → 9.72 MB. `make test`, `make vet`, `make ui-test`, `make ui-visual-test` pass; headless walks without page errors (CHANGELOG). |
| Fix B, type takers, field readers/writers, own-executable join (2026-09-29) | Runs with the e04743b1 binary, rendered at 61c9dd00, on the default cache, no `cache clear`: Redis exit 0 in 35 s, litestream v24 exit 0 in 63 s, freqtrade exit 0 in 260 s (orientation refused by context size, as before), self-snap (e04743b1) exit 0 in 116 s. Redis's 4 undecided units (dupClientReplyValue, dupStringObject, lookupKeyRead, convertToRealHash) became rows the parts answer put in Server core state, no follow-up, no lone part; iojob placed by rule C with freeIOJob/queueIOJob in Virtual memory; litestream's 2 and self's 2 joined existing parts. litestream-test's 3 `litestream` launches draw an arrow into cmd/litestream; cmd/litestream's 9 self-launches keep their tile and name it. Pages: Redis 4.31 MB, litestream 3.92 MB, freqtrade 9.41 MB, repomap 9.97 MB. `make test`, `make vet` (package parallelism 2), `make ui-test` (134), `make ui-visual-test` (64 passed, 6 skipped) pass; headless walks without page errors. |
| Joined files' imports, Connections headings (2026-09-29) | At 66902610 on the default cache, rendered at the same commit: Redis exit 0 in 34 s, one live parts request (redis-server, `"c4 -> f13"` its only change): pqsort.c now in Sort command, lzfP.h in Persistence (RDB and AOF), 19 parts, no other part changed. litestream v24 (6/8, the two C targets fail on missing headers as before), freqtrade (10/10) and self-snap (2/2) ran every atlas request from the cache; only orientation went live where its first packing is refused by context size, as before. litestream's Connections: no repeated heading or row naming its heading (13 and 30 before). `make test`, `make vet` (package parallelism 2), `make ui-test` (135), `make ui-visual-test` (64 passed, 6 skipped) pass; headless walks of the four renders without page errors. |
| Freqtrade | Latest larger run stopped after repeated 16k retrieval refusals; no accepted final report for this wave. A corrected ordinary run with worker, destination, question and data checks is required. The 2026-09-28 cold run completed (430 s) with the orientation refused by context size. |
| Airflow | Full current ordinary acceptance remains pending. Do not restart before prerequisite fixes, saved-window checks and Freqtrade acceptance. Old elapsed time is not a measurement of the new builder. |

Artifact consistency, a green fixture, a saved reading and a successful single
window each establish their own limited evidence. None alone establishes model
answer quality, full repository completeness or end-to-end performance. Update
this table after inspecting the corresponding ordinary result; preserve receipts
and limitations in the journal/archive rather than accumulating them here.
