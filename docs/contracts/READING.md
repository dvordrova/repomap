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
  their docstrings, every eligible callable, type, module body and module-level
  value without a per-file rank cutoff, the boundaries
  (registrations with their holder, SQL statements and configuration reads),
  the file-to-file edges and the seeds. A registration's holder is the value
  the call acts on, as `path:line:column` of the call that produced it,
  followed back through the calls outside the repository (a route put into a
  group made from a router is held by the router). Native boundary places share only exact
  source observations: path, line, column, kind, method, literal values,
  compiler-located subject, call word and external symbol. Every language
  adapter gives each call its own position, so one target has at most one
  fact in a place; two would be refused, never paired with another target's
  facts by order. Each place retains every original target's FactID
  and target-qualified `t*.n*` ObjectID behind local `origins`, sorted and
  deduplicated when sealed. The sealed graph assigns compact `d*`, `f*`, `s*`,
  `b*`, `y*`, `m*` and `a*` place IDs once and rewrites every graph reference
  in the same pass; source-shaped construction keys never survive in JSON.
  A native place keeps only the observing targets that also hold its file, with
  their origins; an observation no holding target made is dropped before
  reading, and a target's projection names only boxes that target has.
  Target coverage must be complete and known target origins cannot conflict.
  Target atlas projection restores those original identities; it never borrows
  a sibling target's fact. Shared source context uses the original SubjectID
  before a target-scoped ObjectID. `reading` walks them in rounds and asks
  one keyed table per round: directories by depth, independent files with direct caller facts,
  symbols, boundaries, the parts of each target's files with their placement
  follow-up and descriptions, the drawn arrows, the core, the areas over the
  described parts, the keys, the portfolio, the joints. Stages with no data
  dependency run concurrently: after the files, the symbols; the outside
  symbols with the boundaries their roles make and the layers between them;
  and the parts read nothing of one another and join before the arrows, which
  read them all. After the core, the areas (`atlas_areas`), the keys of each
  part and the targets with their joints likewise run at once. Each
  prints, counts and rejects into its own record, added at the join in step
  order, so `tables.md` keeps step order and the requests, compact IDs,
  knowledge, the reading's rejected rows and atlas are those of the serial
  walk; only the exchange journal, with the response rejections it appends to
  `rejected.jsonl` as responses arrive, interleaves. The first failure stops the stages beside it
  and is the error reported; `--through` stops inside them at its own stage.
  A target's parts request, placement follow-up and part descriptions use its
  position p+1 as their round, as the core table does; its areas request uses
  it too. Targets read their parts side by side, each on its own record joined
  in target order; parts take their compact IDs in target order, the one point
  where a target waits for the ones before it, and areas take theirs in target
  order once every target's areas answer is in. A row carries the place's own facts and its directory's line, one step up.
  File callers contribute deterministic facts, never another file's model
  line. Description candidates include every eligible declaration in authored
  code; visibility, documentation and callers order them without removing
  lower-ranked evidence. Generated declarations retain their existing source
  evidence without becoming description candidates. An
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
  Every table row uses its existing artifact ID as the provider-visible `key`:
  `d*`, `f*`, `s*`, `b*`, `t*`, `j*` or `q*` as owned by that artifact. The
  table layer never renumbers rows to `r1..rN`; symbol selection and operation
  review use their source symbol's `s*` rather than `selection:<id>` or
  `operation:<id>` pseudo-places. Drawn arrows receive persisted compact `x*`
  IDs before their sentence request instead of using a joined box-pair key.
  Request-local `c*` values remain
  only for choices which have no artifact identity, such as the answer-source
  catalogue. A symbol row's calls are context, not choices, and carry none.
  The model writes one line or one closed choice per
  cell; the architecture stage selects membership, while code validates exact
  references and owns native arrows and their direction, joints by
  matched values, counts and identities. Rejected independent rows fall back on
  their own deterministic lines and are written to `rejected.jsonl`; valid
  neighbours survive in the original exact-response cache. An entirely refused
  window is never cached, and each of its rows' reasons is journaled. The
  key declarations, part roles and keys are closed tables answered only by
  the categorizer (Jev, `JEV_KEY` required; EXECUTION), never by the text
  model; a live reading without it is refused. A
  decision model's (Jev's) choice is taken when the chosen option leads
  every other listed option, `none of these` included, by at least 0.10;
  a closer answer, or a choice that is not the top option, leaves the row
  explicitly uncertain, journaled with its runner-up, and nothing picks
  another option for it. So `support` at 0.49 against 0.32 is a
  role and 0.51 against 0.49 is not (EXECUTION gives the evidence for
  0.10). A yes/no asked as a noul keeps its own band (yes from 0.6, no to
  0.4), a yes/no with a cutoff (`YesAt`, symbol selection at 0.8) decides
  every row, and ranked keys keep their probability. A decision-model
  window whose every row was answered, even uncertainly, is an explicit
  answer that decides none of them and is cached, and one malformed
  decision-model answer leaves only its question unanswered. A response may carry its rows as `{"rows": [...]}`, as a bare array, or in one
  wrapping object; rows match by key alone, trimmed of surrounding whitespace.
  A cell is a JSON string, a list of refs for a sequence, or `true`/`false` on
  a yes/no choice (`false` on an optional choice is no value). A choice may
  carry surrounding quotes or backticks and one final `.`, `,`, `;` or `!`;
  `p3: Storage` is still not `p3`. A prose or text cell with an empty value
  such as `none` reads empty, null, missing, `None.` or `NONE` as that value;
  without one, empty text is refused. A symbol row whose `activation` is missing or null settles as `unassessed`,
  the choice that already means no decision; a written choice is validated
  as before. A missing outbound `address` is the declared `unknown`, and an
  empty or missing joint or peer label is `-`. A sequence cell citing only refs outside its row's options, or nothing at
  all (a provider may send null), is an empty selection and keeps the row's
  other cells; commas separate refs like spaces. A selection past its
  `limit_from` keeps every advertised ref the model chose, in its order: the
  limit is guidance, the owner journals the excess (`menu_over_limit` for
  Learn), and a row without a positive integer limit is a preparation error.
  Every response row must copy an asked artifact ID. Missing
  and unknown keys are refused; a key answered twice the same way is one
  answer, and twice differently is refused, except that copies differing only
  in Alone cells lose those cells; response order never substitutes
  for identity. Directory and file captions, `open`, a target's line and
  role, an outbound boundary's line, destination and address, and an alias are
  Alone: a refused one loses only itself, is journaled as `cell_rejected`,
  and takes the fallback its whole row takes (the given title or line, no
  destination or address, the native role). A type's prose line is shown as
  one line in the atlas. Every row and answer is printed to `tables.md`, with prompts, requests, raw
  responses and normalized source-bound results under `tables/`. The ordinary
  path saves `reading-input.json` before its first atlas call, from the same
  sealed graph bytes as `places.json`; `read` consumes
  exactly that format and runs the same reader. Above two thousand files the directory and
  file rows carry an `open` cell and what the model closes keeps its
  fallback line.
  The directories/files owner prompts follow the complete request `fill`
  catalogue and demonstrate both modes. If requested, `open` is a mandatory
  `yes`/`no` string; prompts must not forbid it by prescribing only the base
  two cells. A missing or unlisted `open` is refused alone and closes
  nothing; its row keeps its captions.

## Architectural responsibilities

`reading/design.go` owns the map of parts; `reading/areas.go` owns its areas.
File captions do not assign files to parts. Owner, 2026-09-25: "я доверяю
только имени пакета, имени символа, и сигнатуре" — every request of the map
sees code structure only: paths, names, signatures, kinds and counts. No
README or AGENTS text, docstring, package documentation or `author_context`
reaches it.

**The grouping unit is the source file.** One `atlas_zones` request per target
lists one row per target file holding at least one unit: `ref` (the sealed
graph's `f*`), `path`, `units`, `types`, `functions` and `variables`. A unit
is a function, a variable, a type together with its methods, or a
source-located module body. An exported name (the adapter's visibility fact:
Go capitalization, JS/TS `export`, Python `__all__` when the module declares
it and otherwise no leading underscore, Clojure not `defn-` nor `^:private`)
is followed by its signature, a type's by its form. Method names are not
sent. A declaration takes its file's part. A method goes with its type
through the native owner the type's members name, even from another file; a
lexical child (a declaration inside the source range of a function or method
of its file, by the adapter's own positions and end lines, such as Go `f$1`
closures and nested JS or Python functions) takes its parent's part and is
neither a row, a name nor a unit. A declaration that repeats the name of an
earlier unit of its file follows that unit the same way: a second Go
`init`, Python `@overload` stubs and their implementation, TypeScript overload
signatures, a Clojure `declare` and its `defn` are one unit and one name, shown
with the first declaration's signature (C has no such repeat). `calls` counts exact call sites between
listed files once per distinct pair (`"f3 -> f7 (12)"`); calls resolved only
to alternatives are left out. `imports` lists imports the adapter resolves to
one listed file (`"f3 -> f7"`); Go package imports resolve to a directory and
add nothing. Files without a unit are not listed. A file that declares only
what follows units of other files, such as a Go method declared outside its
type's file, is therefore no row, yet it is on the map through those
declarations. The prompt sets no count of parts.

**Small targets.** A target without a unit-bearing file sends no request and
has a legitimate empty map. A target of one such file sends no parts request:
its one part takes the target's name, unless the role split below splits
the file. Without a model a target of several
files has no map: an explicit map failure, never an invented grouping.

**Too large.** A parts request is split into windows only when its prepared
request does not fit the provider or the provider refuses its input or
context size, through the shared adaptive split memo. A window is a whole
directory subtree, halved by file count until it fits; a single flat
directory halves into contiguous runs in path order. Parts never cross
windows; nothing is sampled or truncated.

**Validation, placement and refusal.** Owner, on a decoder that refused a
whole good answer: "кто ему дал такое право?" A parts answer
(`{"groups":[{"name","files"}]}`) is validated as independent file → part
rows: an unknown ref is discarded and recorded, a file named twice in one
part is kept once, a file listed in two parts loses both memberships (no
first-wins) and a file left out stays unplaced; a group without a name or
without a listed file is not drawn and its files are left out; two parts
sharing a name over different files are both kept, and a group repeated
with the same name and files is drawn once. One part holding everything, or one part per
file, is accepted as returned and recorded. Every annotation is recorded in
`rejected.jsonl` without refusing the answer. When at least one part was
drawn, one closed-choice `atlas_placement` table places the unplaced files:
a left-out file chooses among every drawn part, a conflicting one only
between the parts that listed it, with its own calls and imports to and
from placed files as part refs. An unknown, missing or refused choice leaves
the file off the map with its reason. A group given twice with the same name,
ignoring case, and the same set of listed files is one answer: it is drawn
once and the repeat is recorded as `part_repeated_group`. The same name over
other files keeps both parts, and a file two different groups list is a
conflict. A `files` string of refs separated by spaces or commas is read as
that list, and each ref is still checked. Only an answer that draws no part
is refused whole: it is not JSON, holds no groups, has no group holding a
listed file of its own (every ref unknown, such as paths instead of refs,
every group without a name, or every file in two different groups), or ends
at the output allowance. Such an answer is not asked again, split or accepted in
part. A refused window is recorded as `window_rejected`; its files are left
out for the follow-up when another window of the target drew parts. A target all of whose windows
are refused gets an explicit `map_failure` with every file off the map, and
the atlas target's `map_failure` word `refused` (`no_model` when no model was
asked); the refusal texts stay in `rejected.jsonl`. Its other analysis
survives.

**A file in several boxes (the role split).** Owner, choosing option "в"
(2026-09-26): "what's in one file can have different roles, and one role can
span different files. We build our own map, we group and abstract." A file
the parts answer placed whole can hold the code of several boxes of our map.
Three requests decide it, each file on its own, beside the parts request of
its target (a one-file target included; never without a model):

- *Candidates* are the unit-bearing files of the target that are neither
  test nor generated code and hold at least two units (one unit cannot go in
  two boxes). No size, count or threshold decides it.
- *The gate* (`atlas_role_gate`, Jev, `lines.RoleGate`) asks, in one request
  per file, "Does the code of `file` go in one box of our map, or in several
  boxes?". `state.task` is what we want (`lines/prompts/role_map.md`: our own
  map for a newcomer, a box is a responsibility a newcomer names, a helper
  goes in the box it serves, a file is the author's unit, not ours) plus
  `role_gate.md`; the item `file` is the path and every unit's name, kind,
  signature, methods and same-file calls; each option carries its criteria
  from `role_gate_options.md` (what, includes, not for, examples): a box is
  a responsibility *of the program*; the steps, stages, decoding, options,
  output and helpers of one responsibility are one box even when each has
  names of its own, and a helper or two is never a group. Only "several
  boxes" leading by `ClassifierMargin` goes on; a file nearer the cut stays
  whole. Measured over every candidate of redis-1.3.6, pykrx, litestream
  and repomap (377 files, 2 draws, then 5 draws of the 13 nearest the cut):
  redis.c 0.97–0.98, pykrx's stock_api.py 0.74–0.79, litestream's main.go
  0.69–0.76; redis-cli.c 0.15–0.21, redis-check-dump.c 0.22–0.30,
  redis-benchmark.c 0.38–0.48. 13 files split in every draw; two pykrx
  query files (`etx/wrap.py`, `bond/core.py`, 0.49–0.60) and repomap's
  `llm/api.go` (0.46–0.55) still land either side of the cut. The first criteria, which counted groups "with their own vocabulary
  of names" as several boxes, had shown no flip on the 11 files first
  measured, but on the whole set split redis-cli.c, redis-benchmark.c and
  repomap's render.go, table.go and api.go in every draw (redis-cli.c's
  `main` then became a box of its own) and flipped redis-check-dump.c.
  Known limit: a Go file's methods on a type declared in another file
  follow that type and are not in the item, so repomap's design.go shows
  only its types and helpers and still goes in several boxes (0.86–0.90).
- *The naming* (`atlas_role_boxes`, DeepSeek, `prompts/design_boxes.md`
  after `role_map.md`) names the boxes the file's code goes in:
  `{"boxes":[{"name","holds"}]}` over every unit, whole, with its name, kind,
  signature, `lines` (its code lines with those of followers outside its
  range; a module body counts its file's code lines less its other top-level
  declarations; zero is unknown and not sent), methods, same-file `calls` and
  `called_by` (decorations included) and `callers_elsewhere` (the distinct
  units of the target's other non-test files that call it). No count is set.
  A wrapper object, keys in another case, whitespace and an identical repeat
  are forms; `null` is no boxes. Two boxes sharing a name are both kept
  (`role_repeated_name`), and the assignment offers each by its ref with its
  own `holds`. A box without a name or `holds` keeps the file whole
  (`role_boxes_incomplete`): the list is one partition decision, and the
  smallest scope that keeps the closed choice complete is the file. Fewer
  than two boxes (`role_one_box`), an answer that is not JSON or has no list,
  or one the provider refuses keep the file whole.
- *The assignment* (`atlas_role_assign`, Jev, `lines.RoleAssign`) asks for
  each unit, the module body included, "Which box of our map does
  `declaration` go in?": `state.task` is `role_map.md` plus
  `role_assign.md`, `state.context` holds only the file's path, the item
  `declaration` is its name, kind, signature, methods, same-file calls and
  callers and `calls_elsewhere` ("path:name"), and each box is an option
  whose criteria are its `holds`. A unit whose choice does not lead by the
  margin, is not answered or is in a refused window is undecided.

A file is split only when at least two boxes hold a unit; otherwise it stays
whole (`role_not_split`). Each box that holds a unit becomes a part, titled
with the box's name, holding those units with their followers (methods,
lexical children, repeated names); a box holding none is not drawn
(`role_box_empty`). An undecided unit, with its followers, goes to the
off-map record under the closed reason `undecided` (`role_undecided`): the
file-level decision was superseded by the gate, so reusing it would promote
a failed result. The part the answer gave the file loses it, and is drawn
only while it still holds a whole file, described by those files' units
alone. The split applies only to a file the parts answer itself placed in a
drawn part: a file it left out or listed twice is placed whole and never
split (`role_not_applied`), and nothing is split under a map failure. IDs:
the answer's parts in answer order, skipping one left without a file, then
each split file's boxes in f* order and naming order. **A role part's
membership is final when it gets its ID; placement may only add whole
left-out or conflicting files to it.** A file shared by two targets is
split per target, since `callers_elsewhere` is per target; it may split
differently in each and costs a naming in each. Every outcome is recorded in
`rejected.jsonl` and `tables.md`, with no label on the page, and a split
failure never fails the target.

The three requests carry request-local refs (the gate's row `f1`, the
assignment's `d1…dn` in the file's unit order, boxes `b1…bn`), so a warm
cache survives a file added or edited earlier in path order: only an edited
file's own requests change, and a call into a file from elsewhere changes
only that file's naming (and so, when the boxes change, its assignment).
Measured on the saved V0WFR boxes (3 identical draws, 118 Jev calls in all
with the gate, $0.067): redis.c 8–11 of 342 units undecided, 1 decided
choice flipped; pykrx 2–4 of 87, litestream 1–2 of 46, none flipped. The
map's rule that a helper goes in the box it serves most is inert in the
assignment: a helper-only box's `holds` names its helper and Jev puts it
there (redis "Logging" = redisLog, litestream's value-parsing and flag
boxes); no code rule empties such a box.

**A split file has no endpoint.** Every lookup that takes a file's part
follows one written rule:

- A split file is in no part's endpoint rows; its role parts name it only
  as a *source* (their directory, test fact, description and area dirs).
- Arrows: a file edge (an import or an aggregated call) into or out of a
  split file draws nothing; the declarations' own calls draw the arrows, so
  no part gets an arrow its declarations do not make.
- Placement evidence counts a left-out file's calls into and out of a split
  file per unit, in the part that holds the unit (an undecided unit in
  none); a split file's imports are no evidence.
- The entry: a seed file's parts are its endpoint, else the parts holding
  its seed declarations (places `seed_decls`), so the trace, the "in" column
  and "starts the program" (core) survive a split seed file.
- A boundary with no subject declaration in a split file takes the part of
  the declaration whose source range holds its line, else of the module
  body.
- Learn's evidence of a split file's own chunk names no area; its
  declarations name their parts. A cross-target joint of a file edge into or
  out of a split file names no part and is not drawn; its declarations'
  calls still join their parts inside the target.

**Membership and the off-map record.** Atlas v12 saves explicit `member_ids`
per part, and an explicit per-target `off_map` record: every file, or stray
declaration, no drawn part holds, with its reason (`left_out`, `conflict`,
`no_units`, `map_failure`, `undecided`), its file line, captions and keys. `no_units` is a
file that declares nothing. A type's methods declared elsewhere follow it off
the map; a method whose file is off the map stays with its placed type. A
file endpoint is its partitioned part directly; a method that follows its
type into another part changes only its own membership. A file that is no
row takes as endpoint the one part its drawn declarations share (none when
they sit in two parts) and has no entry of its own. Declarations off the map
in a file a part holds, such as a method whose type is off the map, are
listed under their type's reason with that part as `box_id`: the file itself
stays on the map. A boundary takes its declaration's part, else its file's
part; one in a file off the map names no box and is still read. A part whose every
file is test code (the adapter's `TestSources` fact) keeps its membership,
file lines, captions and keys in the atlas, is not described, not grouped
into areas and not asked for a core role, and leaves the canvas. The model
has no `tests` role. Accepted parts and areas are born as short `p*` and `z*`
IDs.

**Descriptions.** Each drawn part that is not test code gets one
`atlas_describe` request (`prompts/design_describe.md`): the part's name and
every unit it holds grouped dir → file with its name and signature, never
documentation: all units of a file it holds whole, only its own of a split
file, so an unsplit part's request keeps its bytes (a golden per language
fixture checks it against the requests before the role split). A part whose
only units are module bodies has no member to describe it by and sends no
request; its name alone would invite an invented description. The answer is
`{"description":"…"}`. A long description is
kept; an empty or undecodable one leaves the explicit no-description state,
recorded, and nothing fills it in. The requests of a target run at once
after its parts and placement. Core, keys, arrows and orientation read the
part lines. Descriptions, like the parts names, are asked in every model
run, with or without `--captions`. Each `atlas_core` row and each
`atlas_keys` context carries `declarations`: the names of every declaration
the part holds, in ID order, never cut to a count (the first twelve were
sent until 2026-09-26, while the keys prompt called the list everything the
part holds).

**Areas.** When a target has at least three drawn parts that are not test
code, one `atlas_areas` request (`prompts/design_areas.md`) lists them with
`ref`, `name`, `description`, `dirs` (of its source files, a split file's
included) and `units` (the units it holds), and the exact call sites
between them (`"p3 -> p7 (12)"`), and answers a closed split
`{"areas":[{"name","parts"}]}`. It sets no count; a part may stay outside
every area. A part in two areas or in none stands alone, an area of fewer
than two parts is not drawn. An area given twice with the same name,
ignoring case, and the same set of listed parts is one answer, drawn once
and noted; two different areas that list one part leave that part alone. A
`parts` string of refs separated by spaces or commas is read as that list.
An empty list leaves every part alone; an answer
that is not JSON, has no list, or lists areas none of which holds a listed
part (parts named instead of referenced, every area without a name) is
refused whole, draws no areas and is recorded. Each area's line comes from the same
description prompt with its parts' names and lines as members. Jev assigns nothing in this stage.
Accepted areas keep the order the answer lists them, which usually follows
the pipeline, and take their `z*` IDs in it. The request asks for no order and
nothing validates or repairs one: code only carries it. GroupsIndex numbers
its containers `k*` in that order, and the page lists areas and hands them to
the canvas layout in it, with the parts in no area after them.

The browser does not choose, validate or repair architectural membership.

## Operation ownership

An entry is named the same way whatever its protocol (owner, 2026-09-27:
gRPC, UDP, TCP and WebSocket are assembled at the surface from one thing).
Its registration's `words` are what the code wrote there, as written: the call
word, every literal in order and the address its mount prefixes compose
(`GET` and `/users/:id` of `e.GET("/users/:id", h)`; `redisCommand` and `get`
of a command table's `{"get", getCommand, ...}` row). The incoming boundaries
table asks each entry `name`, a sequence of closed `w*` refs over those words;
code restores the chosen words verbatim, joined by one space in the order the
model wrote them, as the atlas boundary's `name`. A word that cannot stand in
a one-line name as written (a control character, space around it) is not
offered and never trimmed into one. GroupsIndex names the operation by that
name, or, with no accepted choice, by its handler's native name; it never
composes a name from a fact's method and values, and no code tells a verb, a
path, a command or a topic apart. Nothing is asked when the registration wrote
no word. A declaration whose native route already is its operation is not an
operation of its own: the entry carries it. An arrow without witnesses (an
import-only edge) takes its fallback sentence "A uses B." without a model
row.

A part's arrows are its declarations' own relations (GroupsIndex
`native_*` connections). One target is exact; several alternatives are
possible, drawn dashed. A call left unresolved because its field or name was
stored under a branch draws the same possible arrow to each declaration its
store witnesses name by identity (C `fe->rfileProc`, Go `readyLoop.read`,
Python `handler`); the relation stays unresolved and its witnesses stay
witnesses. A call whose stores name nothing draws nothing.

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
  remain eligible. Every original call site and native boundary fact survives.
  Accepted communication retains its exact call site and reaches GroupsIndex
  independently of the containing group's lane or key descriptions. The entrance
  shows those observations and source links, not a count of unique remote systems.
  Native HTTP addresses survive missing or refused prose. Standard-library
  transports may establish communication; this does not promote their package
  objects into remote participants. No package blacklist or API handbook is added.

## External symbols: the `atlas_api` table

The model reads the symbols the code calls or hands something to, one row per
symbol, once per repository: `symbol`, `declared` (its type as its package
declares it), `usage` (the first line that calls it), its `literals` and
`hands_callable` when a repository callable is passed to it. A symbol is a
symbol outside the repository, or, for the rows of a repository table (owner
decision D1), the field those rows store a callable in, named by the file
declaring the row's record type, the type and the field
(`redis.c.redisCommand.proc`); its usage is its first row. `hands_callable`
holds only when a registration handed a callable over, or a value the
repository built (`Register("k6/x/dns", new(DNS))`, whose extension entry
exists only through `binds`): a registration that hands nothing names the
declaration making the call, and `fopen("/dev/null")` was once asked what a
callable becomes and answered a request handler. The
symbols handed a callable and the others are two tables asked at once. Every
cell is optional and absent when the symbol does not do that. A handed symbol
is asked `binds`, what the callable becomes, in the activation vocabulary:
`request` (what a client sends over a connection, whatever the protocol: a
route, an RPC method, a command a client sends), `command` (a person runs it
from a command line or task runner), `interaction` (a person's action in a
user interface), `scheduled` (a timer runs it), `continuous` (it runs for as
long as the program does, on a thread, task or loop of its own),
`queue_consumer` and `extension`; a symbol that runs the callable in place, or
only when the process is signalled or fails, binds nothing. It is also asked
`middleware` (`yes` when the callable runs around or before the handlers; such
a symbol binds and publishes nothing) and `publishes`. Every other symbol is
asked `publishes` (`yes` when the call makes what its holder holds reachable:
a server started, an app run, a consumer connected) and `talks` (the kind of
other running system it sends to, reads from or opens a connection to:
`client_request`, `db`, `queue_producer`, `queue_consumer`, `sdk`).
`client_request` is the outgoing side of `request` and, like it, names no
protocol (owner, 2026-09-27): an HTTP request, an RPC and a raw socket
`connect` are one kind, and a report never calls a TCP connection HTTP. A
symbol may hold several cells. The roles are recorded on the atlas as `api`.
A request, like every entry, is named from its registration's words
(Operation ownership above).

The table asks only the decisions the boundaries read (`repomap.atlas.api.v5`,
owner decision 2026-09-26). `reads_input`, `writes_output`, `auth`, `config`
and `validates` were asked of every symbol, stored in `atlas.json` and read by
nothing; a window of them flipped between runs. A decision without a reader
is not asked and keeps no dormant field. When a reader for one arrives, it
returns as its own table with an explicit `none` option rather than an
optional cell.

The roles make the boundaries; no call site is asked whether it is one. A
registration handing a callable to a `binds` symbol is that entry, with the
role's kind; one to a symbol without `binds` is nothing. A registration on a
`publishes` symbol is the listener (`listen_address`, direction in) and gives
its address to every entry on the same holder; when the code could not follow
the value to a holder, the `atlas_publish` table shows the holders that hold
entries and asks which one the call serves. Every call site of a `talks`
symbol is an outgoing boundary of that kind, whether or not it carries a
literal. Bound entries are operations through their boundary and are not
reviewed as operation candidates; the boundaries table explains a boundary and
chooses the destination and address of an outgoing one, never its existence
or kind. The `listen_address` facts and the per-site `decision`, `kind` and
`basis` cells, and the symbols' `outbound` selection, are gone.

GroupsIndex derives `chains` from the program index, its operations and its
outbound calls: every path of exact or alternative calls from an operation's
subject to a subject that makes an outbound call, at most eight subjects
long, in reading order. An outbound call names the extracted tables among
its values as `data_ids`. A chain carries the repository types its
signatures name; an operation records the repository types its subject
takes and the ones it hands to calls outside the repository (the value
written into a response, found through the call that produced it). None of
this is persisted; all of it is recomputed when an overlay is hydrated.

## Layers: the table that reads code

`atlas_layers` walks, from every bound entry, the graph's calls to every
declaration that makes an outgoing call, and asks about each declaration on
the way. Its row is the declaration's source as written — numbered lines
from its first line to its `end_line`, the middle elided beyond sixty — with
the declarations `before` and `after` it on those ways. One decision:
`access` (makes the outgoing call itself), `adapter` (changes the shape of
what passes through), `logic` (decides), `passthrough` (only forwards). The
answer is the symbol's `role` and the subject's interpretation role. Source
is read through `Options.ReadSource`; without it the rows carry no source.

Every subject and connection of the overlay carries a derived phase: `init`
for what the seeds reach by ordinary calls before anything serves, `runtime`
for an operation's subject and its chains, `both` when a declaration serves
in each. It is recomputed on hydrate, never persisted.

Without `--captions` the model is asked for decisions alone: every prose cell
(titles, lines, sentences, operation descriptions) keeps its fallback, and a
table of prose alone is not sent. The alias is not a caption: it is asked by
name, with or without `--captions` (below). A prompt whose table keeps some of
its cells in a request describes every cell and leaves which ones to write to
`fill`: the symbols, types, targets and joints prompts ask for "the cells
advertised by fill", so a caption-less types request of an English name asks
`line` alone and the model writes no `alias` (it wrote one on 82 of 82 rows,
all discarded, while the prompt demanded two cells).

A row whose only address option is `unknown` accepts any address answer as `unknown`: nothing else can be chosen there, and the model tends to copy the observed path into that cell (27 Freqtrade rows were refused for it). Fixed native boundaries request explanation rather than pointless existence/kind choices. A fixed incoming entry additionally requests its `name` among its `words` (Operation ownership), with or without `--captions`, and shows no `method` field; its rows share windows without an owner, since its handler's calls near a registration elsewhere say nothing about its name (one window per handler cost Redis 98 requests for 97 names). Without captions a fixed row with no decision to make is not sent and keeps its given line. Each fixed cell fails alone: a refused line keeps the given line and a refused name leaves the handler's name. Fixed outgoing facts additionally request a destination and closed original address ref; their native kind and dispatch basis cannot be changed by model cells. Refused prose preserves the native fact. Candidate runtime communication still requires the existing accepted semantic decision. A selected observation must establish the external mechanism or explicit remote configuration; internal delegation is evidence of delegation. Boundary source context carries each original call site to its owning declaration, including safe receiver/source arguments, native API and same-line columns. Calls remain individually anchored; grouping by a shared name or counting Do sites cannot establish the number of systems. The address catalogue lists only literals that can be addresses (no format templates, nothing from formatting, logging, time or string packages) and is sent only for outgoing rows whose address the code does not know; `destination` is a closed choice from the shared known-systems list (`internal/atlas/destinations`) annotated with the target's dependencies, with `other: ` for a system outside it; an owner's calls near the line and its source context are sent once per window and rows reference them. A native outbound fact without a column claims every selected call on its path and line, so the same call is not reviewed a second time as a candidate; a fact with a known column claims only that call.

Boundary v5 names the candidate basis `dispatch` or `remote_client_instance`.
The latter requires this call itself to create or configure the actual remote
client/exporter instance; returning an option for a later constructor is not a
separate relationship. The stored atlas basis remains `configuration` through
an explicit owning projection. Negative rows acquire no purpose, destination or
address from their unused response cells. No API-name allowlist or later
semantic repair enforces this distinction.

The candidate prompt places the three `decision` choices beside their separate
`basis` choices in one table. `external`, `direction: out` and
`invokes_external` describe source indexing or call direction, not a proven
exchange with another process. Selected rows precede the complete shared owner
context and destination catalogue. The catalogue offers names only; no entry
establishes a call's runtime role. A service precedes the API it is compatible
with, so DeepSeek's OpenAI-compatible endpoint is offered and folded as
DeepSeek, not OpenAI. The response shape, closed choices, evidence
and stage reasoning setting are unchanged. A tagged free value keeps its written
name when whitespace around the colon varies (`other:Name`, `other : Name`);
the tag and a nonempty name remain required. This formatting normalization does
not choose a destination or infer a positive decision.

## Type and concept descriptions

- Type descriptions use the existing symbol stage and symbol knowledge, with
  the active `lines/prompts/types.md` prompt embedded by `lines/tables.go`.
  The atlas graph and saved reading input retain a type's declarations
  through exact native owner IDs, including cross-file methods and explicit Go
  interface method declarations. Interface declarations have no invented
  direct-call node. Type context retains the existing bounded author quotes,
  including later sentences; ordinary file/callable rows still use first
  sentences. The type table asks for a short explanation,
  an English alias only for a name outside the Latin script, and key flag; it cannot classify activations. Its explanation is prose, so
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

  This currently covers nominal native type declarations, not every domain
  value. Functions returning maps/vectors and constants keep their callable or
  variable identity; their existing descriptions are available within the
  owning part. An architectural core responsibility does not imply a declared
  data type. In functional code, missing concept cards therefore do not prove
  missing native extraction or an absence of domain entities. Recognizing such
  values as independent concepts would require an explicit source-grounded
  interpretation contract, not a renderer name/path heuristic.

Symbols and Types may also return one short English `alias` beside their
explanation, through the same existing tables. It is a reader-facing label for
a non-English identifier; `none` records no alias. Owner decision 2026-09-26:
the alias is asked only for a name that is not English, with or without
`--captions`, and code decides which names those are: `lines.NeedsAlias` holds
for a name with a letter outside the Latin script (`parse한국` does,
`snake_case_ascii` does not; digits, underscores and punctuation are not
letters). The model does not choose which names are asked: the symbols and
types prompts say the alias is asked only for a name not written in Latin
letters and never ask the model to judge whether a name is English. Columns
belong to a request, so the overview rows are split by name: a name that
needs an alias is asked the complete table (rounds 5 and 6), the others the
table without the alias (rounds 3 and 4) in requests of their own. No
description request reads another, so the functions' and the types' requests
of both shapes are asked side by side and join in step order. An English type
asks its `line` alone, in one request shape and memo shared by both modes;
without captions an English function is asked nothing, and a function that
needs an alias is asked `alias` alone and has no line. A transliterated name
in Latin letters (`juga_jeongbo`) gets no alias: a known limitation. Accepted
aliases flow through symbol knowledge and the atlas into GroupsIndex
interpretations. Native names, IDs and source anchors remain unchanged. The
report keeps aliases English in every language, displays the original code
name beside them, and localizes the description. Both literal spellings
address the same glossary definition. Apart from that one request-side
decision, no script detector, transliteration, per-name request or
browser-generated label is added. Current format constants are linked from
[CURRENT](../agent-room/CURRENT.md#formats).

Names, signatures and argument names are useful clues for model hypotheses; absence of comments must not prevent orientation. Keep those hypotheses distinct from observed actions and effects established by native caller evidence. Earlier source-body/context experiments are development measurements, not authorization for implementation-body retrieval in ordinary requests.

## Questions and Learn

This optional cascade — `atlas_learn`, `atlas_question` and `atlas_answer` —
runs only with `--learn`, or when a `--question` is given. Without it the
ordinary atlas completes through joints and publishes its full map,
orientation and source reading without question results.

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
row returns its state, an answer, its basis, sources in useful reading order,
and the remaining gap. The supporting guide projects those ordered sources; it makes no
separate selection and requests no extra open question.
Questions and source records are canonically ordered for exact-request reuse;
results retain the user's question order. Adding a question changes its answer
window and regenerates that window. There is no per-question answer memo.
The ordinary attempt has no question quota or 64 KiB planning cap. Actual
prepared-input, context, output or response-envelope refusals split questions
first and rebuild each child's complete evidence union. Only a singleton question
whose complete evidence does not fit partitions its original sources into
separate answer parts. Each question validates independently: a malformed,
missing or differently repeated answer leaves that question unavailable while
accepted neighbours survive, and an identical repeat is one answer. The state
is the model's closed decision and is never promoted: an `unanswered` row
keeps its state and gap, and the answer, basis and sources it carries anyway
are discarded and journaled as `cell_rejected`. An `unanswered` or `partial`
answer may leave its gap unnamed (`none`), and a sourced answer its basis. A
substantive answer without text or original sources, a settled answer with a
gap, and inapplicability on incomplete evidence are refused. An unparseable response, a failed provider call or a
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
a refused intent remains unavailable without deleting its neighbours. The
reviews may arrive as a bare array. An intent matches after trimming and
case-folding, a state after trimming, case-folding and reading spaces or
hyphens as underscores. A review is read field by field: a string of refs is
a list and a reason that is not text is none. Two identical reviews of one
intent are one review; two different ones are refused, never joined. Within a
`questions` review each proposed question validates alone: one that is
malformed or fails a rule (blank wording, no or only unadvertised sources) is
dropped with a `question_rejected` journal row and the review keeps the rest;
a question without a why keeps its wording and sources. A review is refused
only when none survive. A `questions` review that arrives without a reason takes
the first sentence of its first accepted question's why and records
`reason_from: why`. A `not_applicable` or `unknown` review keeps its state,
reason and sources as written, even an empty reason; questions it carries
anyway are dropped as `question_rejected` rows and never promote its state.
`not_applicable` still needs complete context and positive sources, and a
review without a readable state, or in a state outside the set, is refused.
An intent with no entry at all in an accepted response is re-asked over the
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
intent, reason and sources. Each intent's menu is asked for at most five of its
candidates; a menu past that limit keeps every advertised question it chose,
journaled as `menu_over_limit`, and a menu without its rationale keeps its
choices. The merge step groups the selected questions by information need in
one call per pool (`groups` of members with a representative; members may be
one string of refs), an unplaced question staying its own group, an empty
`groups` list meaning no repeats, and a refused window keeping every question.
A merge answer whose groups name no advertised question is refused.
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
mandatory in a response and retains per-chunk inspection coverage. Decisions are
read per (question,row) cell. An unknown entry, row or anchor is discarded; a
selection that names no known row is journaled as discarded. A malformed
selection, including one that names a known row only in another shape (the
bare ref, a list or another field), a positive selection with no advertised
anchor, a relevance other than direct/context, or one anchor given both
relevances refuses only that cell. An entry whose selections are not a list
has no cell to compare and refuses its question in that window, even beside a
readable entry. A refused cell stays unavailable rather than becoming a
negative finding and blocks optional glossary metadata of its row; the
question's other rows and accepted neighbouring questions survive, including
cache and memo reuse. A later run asks a refused cell again in its own smaller
window. Harmless forms are the same answer: relevance in any letter case or
padding, one anchor written as a string, a blank reason (a selection without a
hint), a bare array of entries, or one wrapper object around `questions`.
Repeated entries for one question are compared cell by cell: cells they answer
alike are one answer with merged hints, and a cell they answer differently,
including a selection against an explicit empty list, is refused. Only explicit
provider context/output/response resource refusals authorize lossless partition
of complete evidence rows; there is no ordinary row-count or 64KiB planning cap
for this cube. A window asks at most eight questions over its complete rows:
the saved Freqtrade window that asked 64 questions over 460 code rows (4,661
anchors) got four back, the same rows with eight questions got eight, and the
same 64 questions over document rows got 64 — density of decisions per response,
not key names, loses answers. Windows over the same rows share a request prefix;
the first of them runs before its siblings so the provider's prefix cache serves
the rest. A question the model left out, or named only with missing or null
selections, is asked once more over the same rows with the other omitted
questions (`question_omitted` journal rows, marked recovered when the second
round answers). That also follows a response that decided no question, unless
asking every one of its questions again would repeat its request; a question
omitted twice stays unavailable. Output refusals split independent questions
first while retaining their complete evidence; input/context refusals split by
actual encoded input weight. Explicit development budgets remain available.
Per-question memos store references to the original shared response, with the
input metadata needed to reconstruct and compare its exact prepared request.
Replay is revalidated against that complete original window before reuse.
Adding or reordering questions reuses existing decisions; canonical ownership
is restored from the current graph. This implementation is under ordinary
quality acceptance; its measured drafts and limitations are in CURRENT.md.
Symbol selection asks `key_symbol` (yes/no). No column selects a call, so a
symbol row's calls carry no `c*` refs and the row no `call_options`. Rendered
calls leave out the default `invocation`/`resolution`, the column and the
callee IDs, and collapse exact local callees into `local_calls`. Calls whose
rendered evidence is identical apart from their line are one entry whose
`lines` list every site in call order, a line twice when it holds two such
calls; expanding each entry over its lines gives back every call with its own
evidence. The `canvas.spec.mjs` selection row, refused for its size on every
run, falls from 550 entries and 70,906 B of calls to 245 entries and 31,046 B.
Boundary rows render origin trees two levels deep without anchors. Every
rendered evidence value is defined in the evidence vocabulary attached to the
symbol prompts; a contract test fails on a value the prompt does not name.
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

Question-batch v3 validates relevance for each original `(row, anchor)`. Distinct anchors in one chunk may be direct/context independently; different rationales are retained in stable order. Conflicting relevance for the same anchor refuses only that (question,row) cell; the question's other rows and accepted neighbours stay intact. The prompt still says such a conflict leaves the question unavailable, so that the request bytes stay unchanged. Each selected declaration retains its native call/API facts, safe source arguments, receiver/result expressions and source-qualified ownership. `BoundarySourceContext` and question call catalogues use the same safe source projection; canonical IDs stay local. A main-flow or business-effect claim must follow actual observations or remain an explicitly qualified interpretation. A source path is not proof of a call, and its reading order is not execution order.

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
Rows are prepared and their memos recalled on all processors; counts,
`tables.md` lines, rejections and acceptance still follow row order.
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

The common system map composes these saved per-input paths across exact matched
input endpoints, with cycle protection. Reaching a peer part alone never starts
all its inputs. Each continued path retains a source witness from both sides;
the integration step remains distinct from a native call. External communication
belongs to an input only when its original caller subject is reached by that
input's execution relations, not merely when its owning part is reached.

Independent joint protocol decisions additionally use exact row memos keyed by the complete boundary, eligible peer catalogue and target context. The memo restores and revalidates its original response row; equal labels alone do not establish equivalent inputs or a match.

## Stage concurrency

Stages with no data dependency run concurrently; a stage starts once every
value it reads exists. Scheduling never reaches request bytes, cache and memo
keys, compact IDs or artifacts: each concurrent stage keeps its own inputs,
results are folded in step order, and `tables.md` keeps step order. A failure
is reported in step order, never as the cancellation it caused in a sibling.
Facts and claims are built side by side. The report is assembled from the
targets, groups, facts and claims while the orientation is asked; only the
glossary, the display translation and publication wait for the orientation,
whose failure is still the reported cause and publishes nothing of the report.

## Orientation

`orientation` is one model-assisted stage over facts, claims, and the complete
matched GroupsIndex set. It returns one repository summary, one role per
target, a run recipe, and one main flow. The model selects request-local refs
from the exact advertised artifact identities: `t*` targets, `a*` facts, `h*`
claims and target-qualified graph subjects such as `t1.n22`. Groups likewise use
qualified existing IDs such as `t1.g3`. Go does not allocate a second numbering
scheme before the call. Validation is a pure
function over the response and the advertised catalog. Set refs filter unknown
or incompatible members and deduplicate repeats, recording ignored refs; one ref
written as a bare string is the same one-element list. A cited member keeps its
target-qualified ref (`t1.n22`) in the summary refs and a role's subject ids,
because bare member ids repeat across targets; the report resolves that
qualified ref to the member's own source. Flow steps keep their target beside
the bare member id. A row
with no required evidence, a recipe step with no manifest or entrypoint evidence,
a row naming an unknown target, a role without a label, a multi-line `cwd`,
or a flow step naming another target's member is rejected with its raw JSON and
a reason into `rejected.jsonl`. Target refs and `cwd` are trimmed before they
are checked. A role keeps an empty purpose, never filled in: the label is the
decision. An invalid optional recipe note is dropped and recorded while its
step stays. Summary, roles, recipe and flow validate separately,
so a wrong field type does not discard accepted sections. Equivalent target
roles, including purposes that differ only in whitespace, combine their
evidence; conflicting roles leave only that target's role
unavailable. Complete prose and qualifications survive; only short labels collapse
whitespace. Unparseable responses, and responses whose every row was refused,
yield the legitimate empty orientation and are not cached; the latter journal
each row's own reason. Optional terms follow accepted sections and rows on live and cached
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
