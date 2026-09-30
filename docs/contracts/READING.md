# Atlas reading, operations and questions

Current implementation contract. Read only the sections relevant to the
change. [Constitution](../CONSTITUTION.md) takes precedence;
[CURRENT](../agent-room/CURRENT.md) records the current product decisions and
acceptance status. Historical runs and experiments are in the [non-normative
archive](../archive/2026-09-10/README.md).

## Selection and captions

Symbol selection asks each candidate one `key_symbol` yes/no, independently of
directory/file closure; no column selects a call. Symbols and Types describe
only the selected keys the per-file/per-box overview displays. Selection and
caption keep separate exact-input memos and knowledge records; refused prose
cannot erase accepted roles or Learn evidence. GroupsIndex projects those
fields even without a caption; operations never need a symbol description or
key selection to reach the report.

- The atlas (`internal/atlas`) is the model path. `places` builds
  `places.json` from the program indexes, claims, facts and corpus: every
  directory and file; a file's declarations with the first sentence of their
  docstrings; every eligible callable, type, module body and module-level
  value, with no per-file rank cutoff; the boundaries (registrations with
  their holder, SQL statements, configuration reads); the file-to-file edges;
  the seeds.
- Each declaration carries `uses`, local keys that never reach a provider:
  every declaration its program index's exact or alternatives `reads`,
  `passes_callback` and `decorates` relations name (a decorated declaration
  uses its decorator), once, with kind and resolution, with or without a
  pattern; the calls lifted for context keep only such relations that carry a
  pattern. A callable also uses, exactly, each repository type one of its
  parameters carries (`takes`, from the parameter's `type_id`: C, Go, Python
  annotations, TS/JS declared types; Clojure has none). A read or write of a
  record's field (a relation with a `field_path`, C and Go) is no use but one of the
  declaration's `fields`, one per site: kind, the record type's place, the
  field name and the path as the code reaches it (`server.masterhost`); a
  field's readers and writers are the declarations whose `fields` name that
  type and field. No model request reads them.
- A registration's holder is the value the call acts on (`path:line:column` of
  the call that produced it), followed back through calls outside the
  repository (a route put into a group made from a router is held by the
  router). A receiver that is a parameter is the value its repository callers
  pass in that position, when all pass the same one. Following visits each
  parameter once: a caller handing on a parameter already reached (a function
  passing its own parameter to itself, functions passing it round, two paths
  meeting) adds no value, while a second value handed round leaves no holder;
  following never recurses without end. Clojure records no parameter values
  and C calls have no receiver, so neither has a registration to follow.
- Native boundary places share only exact source observations: path, line,
  column, kind, method, literal values, compiler-located subject, call word
  and external symbol. Every adapter gives each call its own position, so a
  target has at most one fact in a place; two are refused, never paired with
  another target's facts by order. A place keeps every original target's
  FactID and target-qualified `t*.n*` ObjectID behind local `origins`, sorted
  and deduplicated when sealed. The sealed graph assigns compact `d*`, `f*`,
  `s*`, `b*`, `y*`, `m*` and `a*` place IDs once; no source-shaped
  construction key survives in JSON. A native place keeps only the observing
  targets that also hold its file, with their origins; an observation no
  holding target made is dropped before reading, and a target's projection
  names only its own boxes. Target coverage must be complete and known target
  origins cannot conflict. Target projection restores the original identities
  and never borrows a sibling target's fact. Shared source context uses the
  original SubjectID before a target-scoped ObjectID.
- `reading` walks the places in rounds, one keyed table per round: directories
  by depth, independent files with direct caller facts, symbols, boundaries,
  each target's parts with placement and descriptions, the drawn arrows, the
  core, the areas, the keys, the portfolio, the joints (Stage concurrency). A
  target's role split, parts, placement, part description and areas requests
  use its position p+1 as their round, as the core does.
- A row carries the place's own facts and its directory's line. File callers
  contribute deterministic facts, never another file's model line. Description
  candidates are every eligible declaration in authored code; visibility,
  documentation and callers order them without removing lower-ranked evidence.
  Generated declarations keep their source evidence but are no description
  candidates. Symbol selection reviews its complete candidate set; caption
  requests follow the displayed-key selection (per file, at most three chosen
  keys, those with a line or docstring first, then by name), never a
  directory-closure veto. A part has no key list; the page reads each
  declaration's `key`. The complete graph and question evidence, with the full
  original declarations, remain.
- Above two thousand files, directory and file rows carry an `open` cell, and
  what the model closes keeps its fallback line; in exploration mode an
  accepted directory `open=no` can leave descendant rows unasked. The
  directories/files prompts follow the complete request `fill` catalogue and
  demonstrate both modes. If requested, `open` is a mandatory `yes`/`no`
  string; prompts must not forbid it by prescribing only the base two cells. A
  missing or unlisted `open` is refused alone and closes nothing; its row
  keeps its captions. Missing or refused decisions never close descendants.
- Independent directory, file, callable, type and boundary tables pack
  consecutive complete rows toward a 64 KiB default input, with no row-count
  cap; a larger complete row, with its shared context, runs alone, never
  rejected. Explicit read-stage input and row budgets remain available. Other
  tables keep their owning context and round bounds. Every prepared request
  obeys the shared transport envelope.
- A row whose subject has an artifact identity uses that ID as its
  provider-visible `key` (`d*`, `f*`, `s*`, `b*`, `t*`, `j*`, `q*`), never
  renumbered to `r1..rN` or wrapped in a `selection:<id>` pseudo-place. Drawn
  arrows get persisted compact `x*` IDs before their sentence request, not a
  joined box-pair key. Request-local refs exist only for rows and choices with
  no artifact identity, such as the answer-source catalogue's `c*`, the role
  split's rows, the parts request's boxes, and the api `sym*`, enters `call*`,
  inputs `kept*`/`table*` and publish `pub*` rows. A symbol
  row's calls are context, not choices: they carry no `c*` refs and the row
  has no `call_options`.
- The model writes one line or one closed choice per cell; the architecture
  stage selects membership, while code validates exact references and owns
  native arrows and their direction, joints by matched values, counts and
  identities. Rejected independent rows fall back on their own deterministic
  lines and go to `rejected.jsonl`; valid neighbours survive in the original
  exact-response cache. An entirely refused window is never cached, and each
  row's reason is journaled.
- The closed tables marked Jev here (key declarations, part roles, keys,
  outside symbols' roles, their calls, programs and inputs, and the role
  split's helper, gate and assignment) are answered only by the categorizer
  (Jev, `JEV_KEY` required; EXECUTION), never the text model; a live reading
  without it is refused. A choice is taken when it leads every other listed
  option, `none of these` included, by at least 0.10 (`ClassifierMargin`;
  evidence in EXECUTION). A closer answer, or a choice that is not the top
  option, leaves the row explicitly uncertain, journaled with its runner-up;
  nothing picks another option. A yes/no asked as a noul keeps its band (yes
  from 0.6, no to 0.4), a yes/no with a cutoff (`YesAt`, symbol selection at
  0.8) decides every row, and ranked keys keep their probability. A
  decision-model window whose every row was answered, even uncertainly, is an
  explicit answer that decides none of them and is cached; one malformed
  answer leaves only its question unanswered.
- Rows may come as `{"rows": [...]}`, a bare array or in one wrapping object;
  they match by key alone, trimmed. A cell is a JSON string, a list of refs
  for a sequence, or `true`/`false` on a yes/no choice (`false` on an optional
  choice is no value). A choice may carry surrounding quotes or backticks and
  one final `.`, `,`, `;` or `!`; `p3: Storage` is not `p3`. A text cell with
  an empty value such as `none` reads empty, null, missing, `None.` or `NONE`
  as it; without one, empty text is refused. A cell with a declared
  no-decision value takes it when missing or null, and a text cell also when
  empty (an outbound `address` `unknown`, a joint or peer label `-`); a
  written choice is still validated.
  A sequence citing only refs outside its row's options, or nothing (null
  included), is an empty selection and keeps the row's other cells; commas
  separate refs like spaces. A selection past its `limit_from` keeps every
  advertised ref chosen, in order: the limit is guidance, the owner journals
  the excess (`menu_over_limit` for Learn), and a row without a positive
  integer limit is a preparation error.
- Every response row must copy an asked key; missing and unknown keys are
  refused. A key answered twice alike is one answer, twice differently is
  refused, except that copies differing only in Alone cells lose those cells;
  response order never substitutes for identity. Directory and file captions,
  `open`, a target's line and role, an incoming entry's line and name, an
  outbound boundary's line, destination and address, and an alias are Alone: a refused one loses only itself, is
  journaled as `cell_rejected`, and takes its row's fallback (the given title
  or line, no destination or address, the native role). A type's prose line
  shows as one line in the atlas.
- Every row and answer is printed to `tables.md`, with prompts, requests, raw
  responses and normalized source-bound results under `tables/`. The ordinary
  path saves `reading-input.json` before its first atlas call from the same
  sealed graph bytes as `places.json`, naming that file by its SHA-256
  (`graph_file`) instead of repeating it when it lies beside it; `read`
  consumes exactly that format with the same reader.
- Rendered symbol calls omit the default `invocation`/`resolution`, the column
  and callee IDs, and collapse exact local callees into `local_calls`. Calls
  whose rendered evidence differs only by line are one entry whose `lines`
  list every site in call order (a line twice when it holds two), so expanding
  restores every call. Boundary rows render origin trees two levels deep
  without anchors. The evidence vocabulary attached to the symbol prompts
  defines every rendered value.

## Architectural responsibilities

`reading/design.go` owns the map of parts; `reading/areas.go` owns its areas.
File captions do not assign files to parts. Every map request sees code
structure only (owner, 2026-09-25): paths, names, signatures, kinds and
counts; no README or AGENTS text, docstring, package documentation or
`author_context`.

**The grouping unit is a whole file or a box.** Each target's role split
(below) runs first; then one `atlas_zones` request per target
(`repomap.atlas.parts.v2`) lists one row per unit of its code: a whole file
holding a unit (`ref`, the graph's `f*`); each box of a split file (a
request-local `c*` ref numbered across the target in `f*` and naming order,
its name in `box`; an empty box is no row, a split file never a whole row);
then that file's seeds and the units no box took, each a `c*` row named in
`box` by its declaration. Each row has `path`, `units`, `types`, `functions`
and `variables`, including units code placed there from other files (below),
so its counts and names may exceed what its `path` holds.

A unit is a function, a variable, a type with its methods, or a source-located
module body. An exported name (the adapter's visibility fact: Go
capitalization, JS/TS `export`, Python `__all__` when declared, else no
leading underscore, Clojure neither `defn-` nor `^:private`) is followed by
its signature, a type's by its form; method names are not sent. A declaration
takes its unit's part; a method goes with its type through the native owner,
even across files. A lexical child (inside the source range of a function or
method of its file, by the adapter's positions and end lines: Go `f$1`
closures, nested JS or Python functions) takes its parent's part and is no
row, name or unit. A declaration repeating the name of an earlier unit of its
file follows that unit: a second Go `init`, Python `@overload` stubs and
implementation, TypeScript overload signatures are one unit and name, with
the first signature (C has no such repeat, and a Clojure `declare` is no
declaration).

`calls` counts, per exact call site, each distinct other listed row the site
reaches (`"f3 -> c7 (12)"`), from the unit whose code holds the site (a
method's from its type's unit); calls resolved only to alternatives are left
out. `imports` lists the adapter's resolved imports between two files, each
file standing for one row: a whole file for its own (`"f3 -> f7"`), a file
that is no row for the one row holding all its units (a whole file that joined
a box). A file whose units sit in several rows or none stands for no row, and
an import within one row is none. The parts and placement prompts keep their
wording (imports of whole files): a box's lines are those of the files that
joined it. Go package imports resolve to a directory and add nothing. Files
without a unit are not listed; a file declaring only followers of other files'
units (a Go method outside its type's file) is no row yet is on the map
through them. The prompt sets no count of parts.

**Small targets.** A target without a unit sends no request and has a
legitimate empty map. A target of one unit (one unit-bearing file kept whole)
sends no parts request: its one part takes the target's name. A one-file
target whose file splits sends one, over its boxes. Without a model a target
of several files has no map: an explicit map failure, never an invented
grouping.

**Too large.** A parts request splits into windows only when its prepared
request does not fit the provider or the provider refuses its input or context
size, through the shared adaptive split memo. A window is a whole directory
subtree, halved by unit count until it fits; a flat directory halves into
contiguous runs in path order; a split file's boxes stay in one window. Parts
never cross windows; nothing is sampled or truncated.

**Validation, placement and refusal.** A parts answer
(`{"groups":[{"name","units"}]}`) is validated as independent unit → part
rows. An unknown ref is discarded and recorded; a unit named twice in one part
is kept once; a unit two different groups list is a conflict and loses both
memberships (no first-wins); a unit left out stays unplaced. A group without a
name or listed unit is not drawn, its units left out. Two parts sharing a name
over different units are both kept; a group repeated with the same name
(ignoring case) and units is drawn once, recorded as `part_repeated_group`.
`files` is the same list as `units`: alike they are one list, different they
refuse that group alone. A string of refs separated by spaces or commas is
that list, each ref still checked. One part holding everything, or one per
unit, is accepted and recorded. Every annotation goes to `rejected.jsonl`
without refusing the answer.

Only an answer that draws no part is refused whole: not JSON, no groups, no
group holding a listed unit of its own (every ref unknown, such as paths
instead of refs, every group nameless, or every unit in two groups), or ending
at the output allowance. It is not asked again, split or accepted in part. A
refused window is recorded as `window_rejected`; its units are left out for
the follow-up when another window of the target drew parts. A target whose
every window is refused gets an explicit `map_failure` with every file off the
map (atlas word `refused`, `no_model` when no model was asked), the refusal
texts in `rejected.jsonl`; its other analysis survives.

When a part was drawn, one closed-choice `atlas_placement` table
(`repomap.atlas.placement.v2`) places the unplaced units, one row per unit
keyed by its ref: a left-out unit chooses among every drawn part, a
conflicting one only among the parts that listed it. A row carries its path,
box name, counts and names, and its calls (per site) and imports to and from
the placed rows as part refs. An unknown, missing or refused choice leaves the
unit's declarations off the map with its reason; a box left out leaves its
file on the map through its other boxes.

**A file in several boxes (the role split).** One file can hold several roles
and one role can span files: we build our own map (owner, option "в",
2026-09-26). Four questions decide it before the target's parts request (a
one-file target included; never without a model), and code places what they
leave open. Each Jev question's `state.task` is `lines/prompts/role_map.md`
plus its own prompt; the helper and gate options carry their criteria from
`role_helper_options.md` and `role_gate_options.md`, and the assignment's
options are the named boxes, each with its `holds` as criteria. Requests use request-local refs (the gate's row `f1`, `dN` by
the unit's place in its file, boxes `b1…bn`), so an edit asks again only the
requests whose items it changes.

- *The helper question* (`atlas_role_helper`, Jev; owner, 2026-09-28) asks
  once per unit of every non-test, non-generated file of the target, one
  request per file (`state.context` empty, prompt `role_helper.md`, options
  `role_helper_options.md`, `none of these` among them): "What is
  `declaration` on our map: a helper, the code of a responsibility, or none of
  these?". The item is the unit's name, kind, `file`, signature, `lines`,
  methods, `calls` and `called_by` (decorations included), `read_by` and
  `handed_over_by` (exact reads and hand-overs), each "path:name" without test
  and generated code, and `registered`; no documentation or visibility. A unit
  nothing uses (no call, decoration, hand-over or read, exact or alternative,
  no registration; test and generated code aside) is no helper by code and is
  not asked, but only when its adapter records such uses of its kind: C
  functions (calls, hand-overs) and variables (reads); Go functions and
  methods (calls, hand-overs); Python, TypeScript and JavaScript functions,
  methods, lambdas (calls, decorations, hand-overs, reads) and variables
  (reads); Clojure functions (calls, hand-overs, reads) and variables (reads).
  Any other unit is asked: a type, a module body, a Go variable (GO) and a
  Clojure macro (CLOJURE). A gap in a recorded kind stays that adapter's
  recorded gap: Go function values kept in a slice, map or package variable
  (GO), Python's unresolved module-attribute calls (PYTHON). Only a decided
  `helper` is a helper; any other answer, a near-tie, an unanswered row or a
  refused window leaves the unit named and assigned like any unit. The
  question is never asked twice, and a refusal never fails the target. A
  helper carries the atlas symbol's `helper` mark. The question and the gate
  run at once.
- *Candidates* of the gate are the target's non-test, non-generated files
  holding at least two units; no size, count or threshold decides. A file the
  gate splits with fewer than two units that are no helpers stays whole
  (`role_not_split`).
- *The gate* (`atlas_role_gate`, Jev, prompt `role_gate.md`, options
  `role_gate_options.md`) asks per file "Does the code of `file` go in one box
  of our map, or in several boxes?", the item being the path and every unit's
  name, kind, signature, methods and same-file calls. Only "several boxes"
  leading by the margin goes on; a file nearer the cut stays whole. Known
  limit: a Go file's methods on a type declared elsewhere follow that type and
  are not in the item.
- *The naming* (`atlas_role_boxes`, DeepSeek, `prompts/design_boxes.md` after
  `role_map.md`) answers `{"boxes":[{"name","holds"}]}` over every unit that
  is no helper, whole (helpers are also left out of the others' `calls` and
  `called_by`), with name, kind, signature, `lines` (its code lines with its
  outside followers'; a module body counts its file's code lines less its
  other top-level declarations; zero is unknown and not sent), methods,
  same-file `calls` and `called_by` (decorations included) and
  `callers_elsewhere` (distinct units of the target's other non-test files
  calling it). No count is set. A wrapper object, keys in another case,
  whitespace and an identical repeat are forms; `null` is no boxes. Two boxes
  sharing a name are both kept (`role_repeated_name`), each offered by its ref
  with its own `holds`. A box without a name or `holds` keeps the file whole
  (`role_boxes_incomplete`: the list is one partition decision, whose smallest
  scope is the file), as do fewer than two boxes (`role_one_box`), an answer
  not JSON or without a list, and a provider refusal.
- *The assignment* (`atlas_role_assign`, Jev, prompt `role_assign.md`) asks
  each unit that is no helper, the module body included, "Which box of our map
  does `declaration` go in?". `state.context` holds only the file's path; the
  item is its name, kind, signature, methods, same-file calls and callers,
  `calls_elsewhere` ("path:name") and `registered`: once each, the words of
  every registration handing over the unit or a follower. Each box is an
  option whose criteria are its `holds`. `role_assign.md` has no wording that
  keeps a command whose work is the connection with the box that runs
  commands: that is the naming's to give. A choice not leading by the margin,
  unanswered or in a refused window leaves the unit open; a decided unit is
  not asked again. The map's rule that a helper goes in the box it serves most
  is inert here: a helper-only box's `holds` names its helper and Jev puts it
  there; no code rule empties such a box.
- *Code places what the questions leave open*: where a declaration's users are
  is a code fact, never asked again (owner, 2026-09-28). A *user* is a unit
  that calls a unit, is decorated by it, reads it when it does not run or, for
  a type, takes it as a parameter (the graph's exact `uses`), between
  non-test, non-generated files. A hand-over is no use: a command table row or
  route registrar hands its handler over without using it, and a read of a
  callable is a function value taken to be called later, so no unit follows
  its table or registrar. Nor is a call through the function value a hand-over
  stored (ProgramIndex `function_value` dispatch, exact when one store reaches
  it) a use, so a callback never follows the code that runs it; that call
  still counts in the helper question's `called_by`. A *row* is a box of a
  split file or a whole file's row. Three rules run together to a fixed point,
  each recorded in `rejected.jsonl` and `tables.md`:
  - A: a helper of a split file whose users, in any file, all stand in one row
    joins it: a box of its own or another split file, or a whole file's row
    (`role_attached`, by name).
  - B: a whole file of helpers joins its users' box (owner's map model,
    2026-09-28). A non-test, non-generated whole file every unit of which,
    types included, is a decided helper, and whose users in other files (at
    least one) all stand in one box of a split file, joins that box
    (`role_attached`, by path). Any unit that is no helper keeps the file out:
    a type answered `responsibility`, a function nothing uses, `none of
    these`, a near-tie or an unanswered row. A whole file never joins a whole
    file.
  - C: an open unit that is no helper takes the row that every unit of its
    file using it stands in (box k or a seed's own row); one no unit of its
    file uses takes the row that everything of its file it uses that is no
    helper stands in, or, once the second pass has nothing more to ask, the
    row of the helpers of its file it uses when it uses nothing else
    (`role_placed_by_users`, `role_placed_by_uses`). Anything else stays open.
- *The second pass* asks the assignment again, after code has settled, about
  split files' open helpers whose users stand in two or more rows or that
  nothing uses, with every named box of their file as options and the same
  item; a helper with a user still open waits. It decodes like the first (a
  near-tie leaves the helper undecided), and code settles again. Once a
  waiting helper's users have rows, rule A places it when they share one;
  otherwise a further round asks it. Rounds repeat until none qualifies; each
  unit is asked the assignment at most once (`role_second_pass`; `tables.md`
  counts each round's asked and placed). The k-th round is its own round of
  windows (`len(targets)·k + round`, after every target's first pass and each
  earlier round), so no window overwrites another. When a round has nothing to
  ask, a unit that can never get a row (a near-tie, an undecided helper) is no
  longer waited on as a user: the rules settle once more and the rounds go on
  asking helpers whose users with rows stand in two or more rows or none,
  until again none qualifies (owner, 2026-09-28). A helper is *blocked*
  (`role_blocked`) only when never asked, which these rounds prevent; the
  reason stays for saved runs.

A file is split only when the assignment puts units that are no helpers in at
least two boxes (code only places a unit in a box already holding one);
otherwise it stays whole (`role_not_split`), one parts row, helpers included.
Each box holding a unit is one parts row with its units, their followers
(methods, lexical children, repeated names) and what code placed there, and
takes the part the answer gives it; two boxes of a file may share a part. An
empty box is no row (`role_box_empty`); `tables.md` counts helper-only boxes.
A unit no box took stays undecided (`role_undecided`; its helper mark is
whatever the helper question decided) and is a parts row of its own with its
followers, named by its declaration like a seed's row; the placement follow-up
places it if the parts answer leaves it out, or leaves it off the map as
`left_out` (owner's fix B, 2026-09-29). A blocked helper goes to the off-map
record under `blocked`, its file staying on the map through its boxes. Parts
take their IDs in answer order; nothing is split under a map failure or
without a model. A file shared by two targets is split per target
(`callers_elsewhere` is per target), possibly differently, with a naming in
each. Every outcome is recorded in `rejected.jsonl` and `tables.md`, with no
label on the page; a split failure never fails the target. Each language's
map-of-parts fixture test builds its graph with the fact layer, as an ordinary
run does. Recorded missing in every language: no fixture has a whole file
joining a box (rule B), so none shows a box's imports (a reading test holds
it), and Clojure's fixture has no route or command registered in a split file.

**One rule for every file.** A declaration takes its unit's part, a place its
declaration's. A file's own part is the one part holding every placed unit
declared in it; a file declaring only followers of other files' units takes
the one part holding its placed declarations; a file whose units sit in two
parts has none. Every lookup of a file's part follows this rule, with no
branch for a split file:

- A part's *sources* are its units' files (its directory, test fact,
  description and area dirs).
- Arrows: a file edge (an import or aggregated call) into or out of a file
  with no part draws nothing; only declarations' own calls draw arrows, and an
  import-only arrow touches a part holding a split file's box only through a
  file all of whose placed code is there.
- The entry: the parts holding a seed file's seed declarations (places
  `seed_decls`), else the seed file's part, so the "in" column and "starts the
  program" (core) survive a split seed file. A seed of a split file is never
  asked the helper question or a box: it is a `c*` grouping row named by its
  declaration, with the helpers and open units only it uses (places
  `SymbolFacts.Seeds`, per holding target). The atlas keeps no main path;
  orientation's main flow and the report's start list read the entry forward.
  GroupsIndex marks as entry only the part holding a seed declaration, and an
  area only when one of its parts is; every other part stands in the middle,
  or with the dependencies when it only calls out. The atlas keeps the
  reading's column fact: a box that takes requests or listens stands "in"
  without being the program's entry. A seed no part holds makes no entry part;
  GroupsIndex keeps it with its off-map reason (`Entries`), and code never
  picks a part for it.
- A boundary takes its subject's part. An input handing a declaration over (a
  command table row, a route) stands only in that declaration's part (an
  undecided declaration's own row included), and names no part when the
  declaration is blocked, left out or in a file off the map, never the part
  holding the table or registering call. Without a subject, a boundary takes
  the part of the innermost declaration whose source range holds its line
  (none when that one is off the map), else of its file's module body, else
  its file's part: a read inside a method stands in its type's part.
- Learn's evidence of a file with no part names no area; its declarations name
  their parts. A cross-target joint of a file edge into or out of a file with
  no part names no part and is not drawn; its declarations' calls still join
  their parts inside the target.

**Membership and the off-map record.** The atlas saves explicit `member_ids`
per part and a per-target `off_map` record: every file, or stray declaration,
no drawn part holds, with its unit's reason (`left_out`, `conflict`,
`blocked`) or the file's (`no_units`, declaring nothing; `map_failure`), its
file line, captions and keys. A type's methods declared elsewhere follow it
off the map; a method whose file is off the map stays with its placed type. A
file that is no row has no entry while a part holds its declarations.
Declarations off the map in a file a part holds (a method whose type is off
the map, the units of a box left out) are listed under their unit's reason,
with the file's part as `box_id` when it has one; the file stays on the map,
and GroupsIndex lists them by subject. A boundary in a file off the map names
no box and is still read.

A part whose every file is test code (the adapter's `TestSources` fact) keeps
its membership, file lines, captions and keys in the atlas, is not described,
grouped into areas or asked for a core role, and leaves the canvas; the model
has no `tests` role. A part its program never runs (atlas `unreached`: the
program's adapter proves every function, method or lambda in it `unreachable`
there, PROGRAM_INDEX; its types, fields and variables follow) likewise keeps
its membership, is not described, grouped into areas or asked for a core role
or keys, and leaves that program's canvas with its arrows; GroupsIndex lists
its declarations off the map by file with its name (reason `unreachable`,
REPORT). A part holding one declaration the program may run stays; a part of
types alone proves nothing and stays. Only the C adapter proves `unreachable`
(GO, PYTHON, JSTS, CLOJURE). Accepted parts and areas are born as short `p*`
and `z*` IDs.

**Descriptions.** Each drawn non-test part gets one `atlas_describe` request
(`prompts/design_describe.md`): the part's name and every unit it holds,
grouped dir → file with name and signature, never documentation. A part whose
only units are module bodies sends none. The answer is `{"description":"…"}`.
A long description is kept; an empty or undecodable one leaves the explicit
no-description state, recorded, and nothing fills it in. A target's requests
run at once after its parts and placement. Core, keys, arrows and orientation
read the part lines. Descriptions, like part names, are asked in every model
run, with or without `--captions`. Each `atlas_core` row and `atlas_keys`
context carries `declarations`: every declaration the part holds, in ID order,
never cut to a count.

**Core.** Every drawn non-test part but the one its program starts in is one
`atlas_core` row (Jev, `prompts/core.md`): `domain`, `interface`, `wiring`,
`support` or `example`, code the program ships for its users to read, copy
or start from and does not run itself (criteria in `core_options.md`; the
task defines the other four). A `domain` part is core; an example part is
not (freqtrade's sample strategies had carried the core mark as domain).

**Areas.** A target with at least three drawn non-test parts sends one
`atlas_areas` request (`prompts/design_areas.md`) listing them with `ref`,
`name`, `description`, `dirs` (of its source files, split files included) and
`units`, and the exact call sites between them (`"p3 -> p7 (12)"`), answered
as a closed split `{"areas":[{"name","parts"}]}`. It sets no count. A part two
areas list, or none, stands alone; an area of fewer than two parts is not
drawn. An area repeated with the same name (ignoring case) and parts is drawn
once and noted. A `parts` string of refs separated by spaces or commas is that
list. An empty list leaves every part alone; an answer not JSON, without a
list, or whose areas hold no listed part (names instead of refs, every area
nameless) is refused whole, draws no areas and is recorded. Each area's line
comes from the description prompt with its parts' names and lines as members.
Jev assigns nothing here.

Accepted areas keep the answer's order and take their `z*` IDs in it; the
request asks for no order and nothing validates or repairs one. GroupsIndex
numbers its containers `k*` in that order, and the page lists areas and hands
them to the canvas layout in it, parts in no area after them. A container's
marks are data: its lane is `triggers` only when one of its groups holds a
target seed, otherwise the majority of its groups' lanes, a part that takes
requests counting as core; it is core when any of its groups is (owner,
2026-09-27).

The browser does not choose, validate or repair architectural membership.

## Operation ownership

An entry is named the same way whatever its protocol (owner, 2026-09-27). Its
registration's `words` are what the code wrote, as written: the call word,
every literal in order and the address its mount prefixes compose (`GET` and
`/users/:id` of `e.GET("/users/:id", h)`; `kvCommand` and `get` of a command
table's `{"get", getCommand, ...}` row). The incoming boundaries table asks
each entry `name`, a sequence of closed `w*` refs over those words; code
restores the chosen words verbatim, joined by one space in the model's order,
as the atlas boundary's `name` (a word written as its value is that word:
EXECUTION); free text never replaces a known route. A word
that cannot stand in a one-line name as written (a control character,
surrounding space) is not offered and never trimmed into one. A refused name
leaves the handler's native name. GroupsIndex names the operation by that name
or, with no accepted choice, by its handler's native name, never by the
function making the registration (othello's `:draw draw/draw-state`, handed
over in `start!` with its handler in another file, had read
`othello.ui.sketch/start!`, 2026-09-30); an entry whose
handler is not established (External symbols) is named by its one word without
a question, and with several and no accepted choice by the first nameable word
its code wrote (`lines.FirstEntryWord`; a handed value's first literal), never
by its declaring caller: a flag's name before its default and usage
(`fs.String("socket", "/var/run/litestream.sock", "control socket path")` is
`socket`), a row's first field, a case's first spelling. The other words are
its registration as written (`Written`), which its reading shows, never its
name (owner, 2026-09-29: litestream's tiles read "socket
/var/run/litestream.sock control socket path"). A handler written inline (Go's
numbered closure `Run$1`, a lambda) names its entry as a reader names it:
the repository function it only wraps (its one call), else the function whose
lines hold it, a method with its type, followed by " (inline)" (GroupsIndex
`ObjectFacts.Inline`, never persisted; a relation between parts GroupsIndex
stores as a connection is labelled by that holder form alone, never by
what the callable wraps, which could name both ends alike: litestream's
cards had read "FindSQLiteDatabases$1 calls IsSQLiteDatabase", 2026-09-30;
a declaration an input's reading names
reads the same). litestream's `RestoreTool$1` calls `isReplicaURL` beside its
outside calls and is "RestoreTool (inline)", never that helper. No name is composed from a fact's
method and values, and no code tells a verb, path, command or topic apart.
Nothing is asked when the registration wrote no word. A declaration whose
native route already is its operation is no operation of its own: the entry
carries it. Registration, callback and control evidence are interpreted by the
model; projection never invents a semantic promotion.

An arrow without witnesses (an import-only edge) takes the fallback "A uses
B." without a model row; one with witnesses falls back on "A calls B: x, y,
z.", the most observed callees, each once. Ties go in source order, never
alphabetical: the call written first (file, line, column); among the functions
one call reaches through a field or name, the one stored there first (a
witness naming it by identity, places `stores`); then the one declared first.
The arrow row's witnesses rank the same way. A call through a function value
writes a field or variable; its witness names each function found stored
there, never the field.

A part's arrows are its declarations' own relations (GroupsIndex `native_*`
connections), a read of a variable or table included (`native_reads`): one
target is exact, several alternatives are possible, drawn dashed. A call
unresolved because its field or name was stored under a branch draws a
possible arrow to each declaration its store witnesses name by identity; the
relation stays unresolved and its witnesses stay witnesses. A call whose
stores name nothing draws nothing. Such a connection never borrows the
sentence of the pair's exact calls: its fallback lists the names its stores
wrote, most often first, each once, ties in the same source order, and a
part's card shows it beside the pair's own.

## Boundaries

- The boundaries table explains a boundary the facts and symbol roles already
  made (External symbols), never its existence or kind. Internal delegation,
  local mechanisms and package membership do not themselves establish external
  communication. An observed indexed callee candidate alone is neutral
  resolution evidence. A `calls` observation exactly resolved to one
  repository callee with no external API is internal delegation at that site
  and stays in source context. Every original call site and native boundary
  fact survives. Accepted communication keeps its exact call site and reaches
  GroupsIndex independently of its group's lane or key descriptions. The
  entrance shows those observations and source links, not a count of unique
  remote systems. Native facts and HTTP addresses survive missing or refused
  prose. Standard-library transports may establish communication without
  promoting their package objects into remote participants. No package
  blacklist or API handbook is added.
- A boundary belongs to every program that holds its declaration's file and
  may run the declaration. A program whose index proves the declaration
  `unreachable` (PROGRAM_INDEX) does not make its calls: the declaration stays
  in that program's parts (unless its part holds nothing the program runs),
  but its outgoing boundaries, listener, registrations, configuration reads,
  the code it runs and any destination chain through it are not that program's
  (REPORT lists them under "Not reachable from the entrypoints"). Facts do the
  same per target: a registration, SQL statement, configuration read or
  code-running call in code a target never runs belongs to the targets that
  run it. Other adapters prove nothing, so in their programs a boundary stays
  wherever its file is linked.
- Test code is testing, not the program. The reading's one test rule is its
  adapter's `TestSources` fact (atlas `test` files, the rule its calls,
  inputs and settings already follow). Once every boundary is made, one
  written in a test file is dropped before any question: it is no input,
  outbound call, listener or program started, and nothing is asked about it.
  A data object the extractors find in a test file is no data of a program.
  Both stop where the reading produces a target's boundaries and data; the
  report hides nothing. An extraction in a code file belongs only to the
  programs holding that file, never to a target merely because its root holds
  the path, so a test no load selects (`//go:build integration`, litestream's
  `tests/`) is no program's data either. A file a test runner does not
  collect (a helper under `tests/` pytest's patterns do not match) is
  ordinary code until its adapter lists it.

## External symbols: the `atlas_api` table

The categorizer (Jev) reads the symbols the code calls or hands something to,
one question per symbol and decision, once per repository. A symbol is one
outside the repository or, for a repository table's rows (owner decision D1),
the field those rows store a callable in, named by the file declaring the
row's record type, the type and the field (`kvd.h.kvCommand.proc`), whose
usage is its first row. The item is `symbol`, `declared` (its type as its
package declares it), `usage` (its first call as written), its `literals`, and
`hands_callable`, which holds only when a registration handed over a
repository callable or a value the repository built (`Register("k6/x/dns",
new(DNS))`, whose extension entry exists only through `binds`); a registration
handing nothing names the declaration making the call. Three tables are
asked at once, each symbol in exactly one: a handed symbol the code calls is
asked `binds` and `talks`, a handed symbol no call names (a table row's
field) `binds` alone, and every other symbol `talks`. `usage` is the call at
the adapter's position, whole: receiver
chain, name and arguments through the closing parenthesis, comments dropped,
whitespace folded, never the numbered source line; a table row or a Clojure
form that is no call gives that row or form, an assignment its statement.
Lexers read C, Go, JavaScript/TypeScript, Python and Clojure; other languages
send no usage. Nothing shortens the call: a row too large for its request is
refused, never trimmed. Each symbol's answer is remembered on its own
(`Memoize`, subject `api:<symbol>`, naming no place), so a warm reading asks
nothing; an undecided answer is remembered as such, never drawn again.

`state.task` is `prompts/api.md` (what the map wants: a program's entries and
its outside systems). Every question is one closed choice, every option,
`none` included, carrying its criteria (what it is, includes, is not for,
examples) from embedded Markdown beside the stage. Every question asking what
something of the repository becomes on our map reads one criteria file,
`prompts/entry_options.md`, so an option means the same wherever offered; its
examples are generic and name no repository the questions were measured on.
Since 2026-09-29 it says whose input is whose: a `request` arrives over a
connection to this program and is never what this program sends; a
`command` is what a person types or passes to this program, words typed into
a client included (owner decision 2026-09-29: they are that client's
commands); an `interaction` is input from a window or a screen this program
draws, while a press or a message another program's interface delivers over
a connection is a `request` and a button this program puts into a message it
sends is `none`; a value a key may take is no `setting`.

A handed symbol is asked `binds`, what the callable becomes: an entry kind
(`request`, `command`, `interaction`, `scheduled`, `continuous`,
`queue_consumer`, `extension`, `setting`), `middleware` or `none` (criteria in
`prompts/entry_options.md`); a symbol that runs the callable in place, wraps
or stores it is `none`. A handed symbol the code calls is also asked `talks`,
the question every other symbol is asked, in the place of the retired
`publishes` (whose `serves` is talks's): litestream's `ssh.Dial`, handed a
configuration with a callback in it, was only asked whether it serves and
never what it dials (2026-09-29). The two are independent (`Alone`): a
near-tie on one leaves the other standing. A handed symbol no call names has
no call for `talks` to decide.

Every other symbol is asked one `talks` question, whatever its calls give it:
`serves` (the program's own listening side), `client_request`, `db`,
`queue_producer`, `queue_consumer`, `sdk`, `runs_program`, `file` or `none`
(criteria in `prompts/api_talks_options.md`). `file` is a call that reaches a
file or a directory of the machine by a path it is given (opening, creating,
renaming, removing, listing); a file is no outside system, so its calls make
no boundary and are not asked what their words become, and which argument
names the path is asked (What a call reaches, below). `client_request` is the outgoing side of
`request` and, like it, names no protocol (owner, 2026-09-27): a report never
calls a TCP connection HTTP. A `talks` symbol's `usage` is its first call
outside tests that gives it words (a literal), else its first call; a row may
show `result_receives`, the calls made on what the symbol returns, counted
(`add_argument ×1`): a factory's code fact, while what the symbol is stays the
model's answer.

Code only restores the closed choices: `serves` is `publishes`, `middleware`
the middleware role (the symbol binds and publishes nothing), `none` no role;
an answer under the margin leaves its decision explicitly unanswered, and a
symbol without a role makes no boundary. The roles are recorded on the atlas
as `api`. The table asks only the decisions the boundaries read
(`repomap.atlas.api.v9`, `.handed`, `.handed.uncalled`; owner decision
2026-09-26): a decision without a
reader is not asked and keeps no dormant field (no `reads_input`,
`writes_output`, `auth`, `config` or `validates`, and no role of a declaration
on an input's path such as access, adapter, logic or passthrough); a reader's
arrival brings one back as its own question with an explicit `none` option.

**Calls given words.** Each call outside tests that gives such a symbol at
least one word is asked on its own what its words become on our map
(`repomap.atlas.enters.v1`, `enters`, Jev, `prompts/api_call.md`): an entry
kind but `queue_consumer` (no outcome is offered in both this and `talks`), or
`none`, from the one criteria file. The item is the symbol and its declared
type, the call as written (comments dropped, never cut), its enclosing
declaration with signature, every literal it is given, where each argument
comes from in words without a position (a word, parameter #2 of a declaration,
an element of a value, a field of a parameter, the result of calling X, code
not followed), the key it may be a value of (`compared_after`, below), and
the symbol's decided `talks` answer. A call is asked only
beside `talks` `none` or no decided answer: words passed to a program started
or sent to are that program's, and a listener stays the listening side, so no
call is both. Nor is a call asked of a symbol handed a callable (`binds`
decides), or when another fact already names it at its line and column (a SQL
statement, a setting read, a table row, a kept callable); a registration of
the symbol itself handing nothing over is no other fact, and its call's answer
decides it. Nor is a call made on a table the program wrote: the table
read at one of its read sites, an element or a field of it, or what an
outside call naming nothing made of one (a package function's first
argument, a method's receiver: freqtrade's `options.pop("help")` on
`deepcopy(AVAILABLE_CLI_OPTIONS[val].kwargs)`); its words are keys of the
code's own data (`onOwnTable`; tables of names are Python's and C's, and a
C call has no receiver). A registration whose own call gives no word is not
asked. One
symbol's calls share windows in source order, with request-local row refs;
each call is remembered by what its item shows, never by line or program. An
oversized row goes alone, a refusal leaves it unanswered, and nothing is cut.
An undecided call makes no entry and is unsure in the launch walk, which says
of each function it reaches what it holds: the inputs it declares (found), its
unsure calls, the calls code cannot follow (its unresolved calls), or nothing.

A call written in an established entry's handler that compares its word with
part of what the handler was handed is that entry's sub-argument (owner's rule
K3), a code fact, and is not asked: an argument written before the call's
first word is a field or an element of one of the handler's own parameters
(`strcasecmp(c->argv[j]->ptr, "limit")` in `sortCommand`), or the call is made
on one and the word is its first argument (`r.Header.Get("X-Verbose")`).
Established handlers are the callables a `binds` answer, a kept-callable
answer or a `starts` answer made an entry of, one kind per handler (the kept
callables are therefore asked before the calls). A parameter used whole
beside a word (`fmt.Fprintf(w, …)`, `res.send("…")`), a word written before
the value (`log.Printf("from %s", r.RemoteAddr)`) and a method's receiver are
no comparison, and the rule applies only to a call the question would ask, so
a call to a `talks` symbol stays that outgoing boundary. The call is an entry
of the handler's kind whose handler is not established, declared by the
handler; GroupsIndex nests it under the entry (`Launch.Nested`,
`Reach.SubArguments`). Redis's SORT (7 words), DEBUG (5) and SLAVEOF (2) were
14 near-tie questions. Only the handler's own body's words are its
sub-arguments, each word once: a word a helper deeper in the reach checks
is an input of its own, and a configuration key the reach reads a setting
(critic, 2026-09-30: freqtrade's `trade` had listed every word of its 65
parts as "Words its handler checks"; the Python fixture's `bare_variant`).

A flag belongs to its subcommand (owner, 2026-09-29): GroupsIndex nests a
handler-less input under an input of the same kind, as its option
(`Reach.Options`, `Launch.Nested`; derived, never persisted), by three code
facts. It is declared on the object that input's own call made (argparse's
`init.add_argument("--force")` on `commands.add_parser("init")`: the
catalogue's `OnOperationID`), unless an input with its own handler is declared
on that object too (freqtrade's subparsers hold 34 handled subcommands); a
handler-less input whose own call made an object other inputs of its kind
are declared on, and whose words that call is given only under a
parameter's name, names where the chosen one is kept and is no input at all
(GroupsIndex projection, after the hand-over join: freqtrade's
`add_subparsers(dest="command")`; every pattern written at the call's site
counts), while a command group given its word by position keeps it
(commander's `program.command("remote")` before `add` and `rm`, argparse's
`add_parser("remote")`; a limit: Go struct fields, C designated initializers
and Clojure maps give every word by name, and the rule is safe there only
while no handler-less holder is made in those languages); or it is declared
in a case's branch, or by code only that branch runs: the calls written in
the lines a comparison case or a guarding call selects (ProgramIndex comparison case and pattern `branch`,
GroupsIndex `Branches`), followed as the launch walk follows calls, while the
walk from the launch's roots that takes no call written in a case's branch
never reaches that code. litestream's `Main.Run` switch runs
`(&DatabasesCommand{}).Run(ctx, args)` in case `databases`, whose own flag set
declares `-json`: `-json` is databases' option, as each subcommand's `-json`
is its own, and no flag word is twice among the tiles; C's kvcli runs `bench`
in its `bench` branch, whose `--requests` is bench's. A line belongs to the
branch starting last at or before it, so a nested branch is its own and
`} else if (…) {` opens the next. An input of another kind is none (a replica
URL's `endpoint` setting the subcommand's code reads), nor is a value of
another input (`ValueOf`). Third, it is handed (`groupindex.Handed`,
compiled from ProgramIndex): a declaration F declares it on its own
parameter P (a call whose receiver value is that `parameter`), or it is a
row of a table F looks up with keys it is handed while F makes calls on
one parameter P (a `keys` read of a list at a call of F; the row's key is
its first word), and a call of F hands P the object an input's own call
made (a `call_result` argument anchored at that input): it is that input's
option, a row only under the calls whose list names its key. It is no
tile of its own only when every call of F is followed: F reached by exact
calls alone, each handing P an input's object and, for a row, a list the
code wrote rows for, F alone reading the table; otherwise it is listed and
stays a tile, as the program takes it too. Freqtrade's
`_build_args(optionlist=ARGS_TRADE, parser=trade_cmd)` lists 164 rows of
`AVAILABLE_CLI_OPTIONS` under 27 subcommands (trade's `--db-url`,
`--dry-run`, …); every row stays a tile, since 7 subcommand calls hand
lists with no rows (`*ARGS_…` spreads, one-element and empty lists, a
filtered comprehension), `ARGS_MAIN` goes to the program's parser and
`ARGS_COMMON`/`ARGS_STRATEGY` to parser groups taken through `parents=`,
none of them followed yet. Go's facts carry the same joint; its
`addCommon(fs)`, handed each subcommand's own flag set in code only that
case runs, is nested by the branch rule. JS declares options on the root
program alone and the Clojure fixture uses no option library; C's helpers
run in a case's branch (branch rule). TypeScript's switch and Clojure's
case form carry the same branches; their fixtures declare no option in a
subcommand's own code yet.

A call whose answer makes its words an entry, and that no fact boundary, in or
out, names at its line and column, is that entry: a model boundary, direction
in, of the kind answered, whose `words` are the literals given (never the call
word or caller's name) and whose handler is not established
(`handler_unknown`). A call giving words to a registration the code found (a
literal on a value holding callables, an address literal) becomes the same
entry by its own answer, with its own words (the registration's values keep
only its address); a value handed over without a callable is also an entry
whose handler is not established. No code is taken to act on such an entry: it
is declared where its call is written, in its caller's part, and binds to no
part. It has no subject, reach or phase in GroupsIndex; it does not make its
caller a key's entry, its part the side work comes in at or a core part's
entry, and gives no address to a holder's entries. A call none of whose words
can name an entry (a format ending in a line break) makes none, recorded as
`entry_unnamed`, and is read as any other call; a call in a test file makes
none. The entry question's command and request options are not for a
parameter's description, help or usage text, nor for a character or prefix a
word is tested to start with (litestream's `strings.HasPrefix(u, "-")`); the
call's arguments show each literal with the parameter it is given as
(`description: "Number of months to fetch data for"`), and the name column
offers each word with it (`given`, `atlas.BoundaryFacts.WordsGiven`, from
the call's keyword arguments) and never takes a description, help or usage
text. Only Python and Clojure write keyword arguments; Go, C and JS/TS words
are positional, so their words carry none. GroupsIndex keeps one input per
kind, words as written and declaring caller: an option written twice in one
function is one input at its first site; the same word in another caller is
another. One handler registered by several calls of one kind under the same
word is one input: the registration written first stands, and each other is
among its `Aliases` with its site and call as written, so no site is lost.
Different words are different inputs even with one handler: a word a person
types is its own (redis's `smembers` beside `sinter`, freqtrade's
`list-pairs` beside `list-markets` and its `update_*` callbacks beside the
commands they repeat, two routes of one handler; how many such inputs a
reading shows is the display's matter). A name that cannot stand refuses
that entry alone.

Spellings of one value are one input, as a case listing several words is
(reviewer's item 6a, 2026-09-30). A call whose ProgramIndex pattern
reads the same value as an earlier call (`same_value_as`: litestream's
`query.Get("storage-class")` in the else-if arm after
`query.Get("storageClass")`, `query.Get("force-path-style")` joined by
`||` to `query.Get("forcePathStyle")`) and that its own answer made an
entry of the same kind as another call of that read, in the same
declaration, is that entry's other spelling (`spellings.go`,
`atlas.Boundary.AliasOf`): the call written first stands, named by its own
words as written, and GroupsIndex keeps each other spelling's name, site
and call as written among its `Operation.Aliases`, in source order, with
no input of its own; a joint naming a spelling names the input. Each call
is still asked on its own and keeps its answer: a spelling answered
another kind, or none, is not folded, and nothing new is asked. A value of
another entry (below) is none. A condition testing two different values
together with `||` (`q.Get("user") == "" || q.Get("password") == ""`)
reads as one value if both are answered the same kind: a known limit of
the code fact, which cannot tell a presence test from a required pair.

A directive's values are its sub-arguments (owner's rule K3: the words
compared inside an input's handling are its sub-arguments), a code fact. Among
the entries one function's calls make that compare literal elements of one
value, the lowest element's words are entries and each higher element's words
are values of the entry written last before them (`atlas.Boundary.ValueOf`):
Redis's `loadServerConfig` compares `argv[0]` with "appendfsync", then
`argv[1]` with "always", "everysec" and "no": `appendfsync` is a directive
with three values. A value with no entry before it stays an entry. GroupsIndex
nests a value under its entry (`Operation.ValueOf`, `Launch.Nested`,
`Reach.SubArguments`); it is no catalogue member or tile, and the entry's and
the Inputs readings list it ("appendfsync: always | everysec | no"). Which
entry a value belongs to is the code's, and so is whether it is asked: a word
call comparing a literal element of a value, after a call written before it
in the same declaration compared a lower element of that value (its key),
shows that key as written (`compared_after`) and waits for the key's answer;
when the key is an entry, the call is its value, of its kind, and is not
asked (the step before decided it); otherwise it is asked on its own
(`key_values.go`). The criteria keep a value a key may take out of the
settings, so a value asked on its own, or a value no key comes before, is
`none`; Redis's `strcasecmp(argv[1],"debug")` drew setting and none at
0.49/0.46 with its key shown, and is no longer asked. Every incoming boundary keeps its registration as written at its
site (`atlas.Boundary.Written`, folded to one line, a command table row with
its arity and flags), which the input's reading shows; no request carries it.
A table's row is its own element of the table, bounded by the neighbouring
rows' words (`lines.CallFile.RowText`): a dict's `"verbosity": Arg(...)`,
never the whole dict (freqtrade's 124 rows each carried the 19.7 KB
`AVAILABLE_CLI_OPTIONS`).

**Programs started.** A call to a `runs_program` symbol is one outgoing
boundary of that kind, unless its receiver is the result of such a call (a
`call_result` receiver anchored at the launching call), which starts, waits
for or reads the same program: the naming call is the one boundary (the
one-exchange rule, below, for every kind). A receiver
that is `alternatives` all of which are such results (a command built on
either branch of an if/else) is each launch's: the call on it is no boundary,
and each launch keeps its own boundary and program, one destination when they
name the same word. One part from elsewhere (a nil branch, a field) leaves the
call its own boundary: litestream's `c.cmd.Start()` on a field holding
`exec.CommandContext`'s command stays one, since Go records no field's stored
value. Which program it starts is asked per call
(`repomap.atlas.program.v1`, stage `atlas_program`, Jev). The item is the
symbol, the call as written (`usage`) and `words`: each word the call is given
that can stand on one line, once, in call order, as request-local refs whose
option names are the words; the options are those words and `not_named`, every
word carrying one set of criteria (`prompts/program_options.md`). One symbol's
rows share windows, each exact row is remembered on its own (`Memoize`), and
an undecided answer stays undecided. A call given no word is not asked, and
its program stays not established: a program named inside a list
(`subprocess.run(["git", …])`) gives no literal word, so no word is no
evidence that none names it. The boundary's destination is the chosen word as
written (`atlas.Boundary.Destination`); `ProgramNotNamed` marks `not_named`;
both empty is not established. Such a boundary is not sent to the boundaries
table: it has no address, catalogue destination or line. A launch no word
names is an unknown about that one call, never an outside system: the
report reads it with the function making it and under "What is missing"
(REPORT, 2026-09-30; litestream's `-exec` launch had drawn a "Program not
established" frame twice). A chosen word equal
to a name the repository's build gives one of its own programs (ProgramIndex
`target.executables`) makes the launch that program, joined by equal names
only (REPORT; 2026-09-29).

**Values compared with several words.** A value the repository's own code
compares with two or more words in two or more cases (ProgramIndex
`comparisons`: a switch on a program's first argument, an if/elif chain, a
match or case form) is no call, so no per-call question sees it; each
comparison outside tests, in a declaration a program runs, is asked once
for all its cases in the inputs step (`repomap.atlas.dispatch.v1`, column
`enters`, stage `atlas_inputs`, Jev, `Memoize`; its own state
`prompts/api_dispatch.md`, the call question's options and criteria file).
The item is `compares` (the value as written), `from` (its origin in the
words of a call's `arguments`), `in` (the declaration with its signature)
and `cases` (each case's words in source order). A comparison none of whose
words can name an entry is not asked. An entry answer makes one input per
case whose words can name one, at the case's first word, named from its
words and declared by the comparing declaration. A case whose lines (its
`branch`) hold a call into the program's own code is handled there: its
handler is the comparing declaration, its atlas boundary carries the lines
(`branch_line`, `branch_end`) and its GroupsIndex operation `Branch`, and
its reach and spine start from the handler's calls, reads and hand-overs
in those lines alone (groupindex `branch.go`): litestream's `replicate`
reaches NewReplicateCommand and ReplicateCommand's Run, never
DatabasesCommand.Run. Any other case's handler is not established (a case
returning a word, printing usage), and those cases are one catalogue; a
case none of whose words can name an entry is `entry_unnamed`. None and an undecided answer make nothing; an undecided
comparison is not yet one of the launch walk's unsure calls. A case's
`branch` is the lines a setting reads its written fields from (REPORT).
litestream's `Main.Run` switch is one question whose 17 cases hold its 14
subcommands.

**Tables of names.** A table of names (places `rows`) is asked once
(`repomap.atlas.inputs.v1.table`, stage `atlas_inputs`, Jev, `Memoize`) what
its rows become: the call question's options and criteria. The item is the
table, its declared type, `file` (the file declaring it), every row's words
and `read_by`: each declaration reading it outside tests by name and
signature, each line reading it as written (`reads`, places `read_at`) and
its callers outside tests by name and signature, each with its lines calling
the reader as written (`called_by`). How a table is read and who hands its
reader what say what its rows are: redis-cli's `cmdTable`, read by
`lookupCommand`, which `cliSendCommand` calls with `argv[0]`, was request
0.96 with its readers' names alone, request 0.81 with the reading lines and
callers, and is command 0.83–0.88 with its file; DEBUG's `strencoding`, read
into a reply, went from command 0.55 to none 0.88–0.93.

Two tables are not asked, since how they are read says their rows are no
inputs of their own (places `read_at` forms, PROGRAM_INDEX's shared
witnesses); each is a tables.md line naming its readers, and makes nothing.
A table every read of which outside tests looks another table up with each
of its rows, all one word (a list, tuple or set of strings), holds that
table's rows: freqtrade's 28 `ARGS_*` lists of option keys, which
`_build_args` looks up in `AVAILABLE_CLI_OPTIONS`, were each answered
command beside the 124 real flags. The other table must be asked or hold
in turn the rows of one that is; a cycle of such tables is asked. A table
every read of which outside tests only tests a value's membership holds
the words of one condition, as a list written in the condition is one case
and no comparison: `NO_CONF_REQURIED`, which `_parse_args` tests the
parsed subcommand against, was answered command and listed
`backtest-filter`, no subcommand. A table with any other read is asked as
before. freqtrade asks 29 of its 60 tables outside tests (28 hold keys,
`NO_CONF_REQURIED`, `NO_CONF_ALLOWED` and `SUPPORTED_EXCHANGES` are only
tested); the asked tables' items are unchanged. The acceptance run's 272
command extras held 167 rows of these tables and its one trap hit
(`backtest-filter`); every subcommand they duplicated is found at its
`add_parser` call.

A third table is not asked: one an established entry's handler looks up
with part of what it was handed (K3 for a table; `handed_tables.go`): a
call in the handler given both the table and a field or an element of one
of its own parameters, or, when the handler hands such a part on to
another declaration by an exact call, a call there given the table and
that declaration's parameter at the position the part was handed at. A
table is a call's argument or receiver where the code reads it (places
`read_at`) or whose value the adapter traced to the table's own
declaration. Each row is a value of the entry (`ValueOf`, listed under it
and no tile), of its kind, named by its first word, handler not
established, its row as written (`RowText`; a Lisp map's row is its key and
value). othello's key-pressed handler `host/on-key` hands `(:key event)` to
`events/on-key`, which calls `(get key->command key)`: n, u, h, 1 and 2 are
the keys key-pressed takes, each with the command it names. A table two
handlers look up is neither's and is asked.

**Settings in tagged fields.** A repository structure field whose tag names a
key (Go's object aliases, `yaml:"dbs"`) is asked on its own what that key is
(`repomap.atlas.inputs.v1.field`, stage `atlas_inputs`, Jev, `Memoize`):
`setting` or `none` (criteria in `prompts/entry_options.md`). The item is the
field and its type, its structure with its file, the tag as written (each key
after its format's name, which is data: code names no library or format), and
`structure_use`: each outside call given a value of the structure, with the
calling declaration and the call as written, and each field of another
structure typed with it followed by that structure's use ("the type of field
DBs (yaml:"dbs") of Config, which is given to …"), matched by where types are
declared, never by name (GO). Every tagged field outside tests is one row;
nothing is capped. A `setting` field is an entry whose handler is not
established, at the field, named by its tag's keys, declared by its structure,
so one structure's fields are one catalogue; when the facts name exactly one
call decoding the structure, the entry is also declared on it (`Declared on
yaml.Unmarshal(buf, &config) in Config`). `none` and an undecided answer make
nothing.

**Starting statements.** A registration with an `invocation` (a Go `go`
statement, a coroutine handed to `asyncio.create_task`; PROGRAM_INDEX) hands
its function to no outside symbol, so no `binds` answer decides it: one answer
would take its example from the first statement and decide every other
(litestream starts 20 functions with `go`, ticker monitors and signal
waiters alike). Each statement outside tests is asked on its own
(`repomap.atlas.starts.v1`, `starts`, Jev, `prompts/api_start.md`, after the
symbol tables): `request`, `scheduled`, `continuous`, `queue_consumer` or
`none` from the one criteria file. The item is the statement as written (a
closure's body included; for a coroutine, the call it is handed to), `in`
(the declaration it is written in, with its signature), `starts` (the started
function with its signature), `calls` (what that function calls, by name, in
source order) and the started call's `literals`; each row is remembered by
what it shows. An entry answer is that entry, handled by the started
function and named by its declaration; `none` and an undecided answer make
nothing. A function the graph holds no declaration of (a closure calling
nothing) is not asked and makes nothing.

**The roles make the boundaries**; no call site is asked whether it is one. A
registration handing a callable to a `binds` symbol is that entry, of the
role's kind; one to a symbol without `binds` is nothing. A registration on a
`publishes` symbol is the listener (`listen_address`, direction in) and gives
its address to every entry on the same holder; when the code could not follow
the value to a holder, the `atlas_publish` table shows the holders holding
entries and asks which one the call serves. Every call site of a `talks`
symbol is an outgoing boundary of that kind, with or without a literal; a
native outbound fact without a column claims every call on its path and line,
one with a column only its call, so no call is a second boundary. Bound
entries are operations through their boundary and are not reviewed as
operation candidates. There are no `listen_address` facts, per-site
`decision`, `kind` or `basis` cells, or symbol `outbound` selection.

**One exchange, one boundary** (2026-09-30). A call on what a call of a
symbol answered the same `talks` kind returned continues that call's
exchange (`cmd.Run()` on `exec.Command`'s command, `select(...).filter(...)`
on the statement `select` began), and a call whose result is handed whole
as an argument to a call of a symbol answered the same kind is part of that
call's (`func.count(...)` in `select(...)`, a `text()` statement in
`execute(...)`), with any call made on it (`func.sum(...).label(...)`): only
the call that begins the exchange, or receives the parts, is a boundary. A
call made on such a part with no `talks` answer of its own hands the part on
too (`func.count(...).label("count")` in `select(...)`, `func.count.label`
never answered, 2026-09-30); a call answered another kind ends the chain
(`requests.get(...).json()` in `select(...)` keeps the request). A
receiver of alternatives each such a result counts; a field of a result, a
value formatted from one or another kind never does. freqtrade's SQLAlchemy
rows (all answered `db`, whose criteria include building the statement a
session runs) had made `sum`, `filter` and `label` destination rows of their
own: 116 db rows, now 63 (29 `select`, 3 `update`, 2 `inspect`, 2
`order_by.limit` on a repository function's statement, 1 `read_sql`, 24
SQL facts, and 2 `func.count` whose `.label`, handed to `select`, had no
`talks` answer, so the chain stopped there until the rule above) (`reading/boxes.go`
`sameExchange`, `handedOnExchanges`).

### Outside systems

Which outside system an outgoing call reaches is asked once per outside
package, never of a list in code (`repomap.atlas.systems.v1`, stage
`atlas_systems`, the text model). Code groups the boundaries table's outgoing
rows by the outside package their own call goes through, as the facts record
it at the site (a Go import path, a Python or JavaScript module, a C header, a
Clojure namespace). The row's own call is the call its external names or, for
a fact naming none, the call at its exact site whose symbol `talks` (an
outgoing kind; the fact claimed that call's boundary, as an SQL text on
`db.Exec`), else the talking call handed what the call at its site returns
(`db.Exec(fmt.Sprintf(…))` for the text Sprintf formats); a row with none has
no package and no reaching call. One row per package for the
whole run: `package`, `dependency` (each module and version its targets'
dependency catalogues record) and `calls`, each package symbol the program
calls with one call as written (the first outside tests given a literal, else
the first outside tests). The one cell, `system`, is a short name as a
newcomer would say it, or `none` when calls through the package reach no one
outside system (criteria in `prompts/systems.md`). Each package is remembered
on its own (`Memoize`); the cell is a decision, kept without captions. An
undecided package has no name.

**One destination, one name** (F3, 2026-09-29; skeptic-reviewed). What an
outgoing call reaches is decided once per destination, never per call
(`reading/destination_groups.go`). A row whose package atlas_systems named
takes that name in code, with no question: the catalogue entry listing the
package, which the per-row prompt only looked up. A destination is one
program's (2026-09-30): a row written in code several programs share
(Redis's anet.c `connect`) is each program's own call, reached from that
program's callers and walked to that program's values, so it is named once
per program. Rows are one destination when the code knows they reach one
place: the same program, the same kind (the talks answer) and the same walk
ends there of their reaching call's decided argument (DestinationReader).
Two kinds are two exchanges even on one value: redis-server's
`gethostbyname(server.masterhost)` (sdk) is answered by the resolver, the
`connect` after it (client_request) by the primary, and the destination
criteria say a lookup of an address ends where it is answered
(`TestALookupOfAnAddressEndsAtTheResolver`). No fixture looks an address up
yet: C's `netConnect` converts one with `inet_pton`, no lookup (sdk is not
for a conversion), and the Go, Python, JS/TS and Clojure fixtures resolve
no host. An
end is an absolute URL by its scheme and host as written (cut at the first
`/`, `?` or `#`), a setting by the setting that begins it (`{--socket}`,
`{env:API}/users` as `{env:API}`), and any other address or an unresolved
expression by that value and the site where the walk stopped (two clients
both asking `"/health"` stay apart; `?.DB` at `main.go:55` is one origin). A
row with no reaching call is its own destination; the walk of another call
at its site (the fmt.Sprintf formatting a query) says nothing of it.

**The object an exchange goes through** (owner, 2026-09-30: calls reaching
the same session or engine are one destination; skeptic-reviewed). A row's
destination key and its destination's `ends` are where its exchange ends
(`boundaryState.through`, `reading/destination_objects.go`); its own walk
stays its address, chain and the published `Uses`. The exchange ends:
- for a call made on what a call of its kind returned, where the call that
  began the exchange ends (`inspector.get_columns("trades")` on
  `inspect(engine)`), whatever its own argument names;
- for a call whose symbol decides no argument names what it reaches (an
  ORM's `select`, `text`, `update`: decided none), where the object it is
  sent through ends (`DestinationReader.Exchange`). Its senders are the
  calls its statement is handed whole to (a local name bound to it, an
  alternative of it, a call answered its kind or nothing made on it, a
  call of its kind it is handed to, a repository function's parameter and
  return, the result of a call to no known function it is handed, as a
  column's `not_in(subquery)`, and a call of its kind handed a column of
  it, as a CTE's), counting a call to no known function
  (`Trade.session.scalars`) and a call of its kind made on an object; a
  call of another kind (a logger's) sends nothing. Their receivers, else the call's own receiver,
  are followed back: a parameter to each caller's argument, a field to the
  value its one store gives it, a context manager's entered value to the
  manager, a repository call to what its function returns, a call to no
  known function giving no words (`engine.begin()`) to its receiver, and an
  outside call never asked which argument names what it reaches and giving
  no words to the object it is made from (receiver, else its first argument
  another call made or a caller hands: `scoped_session(sessionmaker(bind=
  engine))`). The outside call with a decided argument that made the
  object is where it ends, through that argument (`create_engine(db_url)`);
  any other call is the object itself, keyed by its site
  (`boto3.client("s3")` and `("sqs")` stay two). An object an outside call
  answered another kind made, a field no store gives a value, an unknown
  name and `self` give nothing, and the row keeps its own walk;
- for a walk stopping at a field of an object (`Trade.session.bind`), where
  the decided call that made the object ends, only when one did: two values
  read from one configuration stay apart.
freqtrade's database rows had been 62 destinations, each asked in its
window ("Freqtrade database" 39 rows, "Database" 24); its 61 rows (two
`func.count` rows are now part of their `select`) are one destination,
"Database", asked once. A destination whose rows' packages
name exactly one system gives it to its rows without a package; one naming
none is asked once (`repomap.atlas.destinations.v2`, table
`atlas_boundaries`, round 3, the text model, `Memoize` by the item under the
subject `destination:<key>`, `lines/prompts/destinations.md`); one naming
several is shown by the facts not to be one destination, so each row keeps
its package's name and each row without one is asked alone. The item is the
destination: `ends` (the walked addresses as written, the unresolved
expressions, each with `written`, the code where the walk ended), `calls`
(each call as written with its kind, package and literals, each once),
`callers` (each declaration making them once, with its file, signature, the
calls beside the rows as written (`OwnerCalls`: within three lines, or with
address literals) and the callables it hands over or is handed by, with the
receiving call: litestream's `Run$1` is handed by `InfoCommand.Run` to
`net/http.Transport.DialContext`), and `reached_from` (below; by name, each
once). No author documentation, README claim or source context is sent. The
catalogue is built from the systems' names for the destination's program
(`destination_catalog`, `destination_options`): one `d*` entry per name
(case-insensitively), in name order, listing the `packages` reaching it; a
package answered `none` gives none. After them, marked `program`, come the
repository's other programs that serve requests or listen while they run,
each with what it `takes` (those entries by kind, method and name); never
the destination's own program (a replica's primary is another copy of it),
and a fixture only to fixtures of its root. Only a destination with a
`client_request` call (a request sent or a connection opened, the talks
answer of its symbol) is offered them: a call through a library reaches
the system the library talks to, and redis-cli's `gethostbyname`, offered
redis-server, had been drawn into it where it had been "DNS Resolver". A program chosen, by its ref
or by its name after `other: ` (case aside: freqtrade's run drew "other:
Freqtrade" once), is the boundary's `destination_target`, and the report
draws the destination into
that program: freqtrade-client's one request, whose path the code computes,
matched none of freqtrade's inputs and had stood as "Freqtrade Server"
beside it (5 of 5 draws chose freqtrade, and scripts/ws_client.py's
WebSocket 5 of 5). It offers names only; no entry establishes a call's
runtime role. One program's destinations of one
catalogue share windows, whose context names the program (`program`, its
target name); another program's never see them: beside the clients'
identical calls in one window, redis-server's connect to its master was
named "Redis server", in its own window "Replication primary" and then
"Primary". The cell is a `d*` ref or `other: ` and a name, written as a
proper name, a capital first letter and no article: a vendor's service by
its product name, any other system by its role for this program ("the
webhook endpoint" and "the program's own database" had stood beside
Telegram); a package, protocol, host, URL, file path or key is no system's
name. A refused answer names none; each call keeps its own site,
address and chains. A tagged free value keeps its name whatever the
whitespace around the colon (`other:Name`, `other : Name`); the tag and a
nonempty name remain required, and this normalization chooses no
destination. The report groups records by the stored name without its
parenthetical qualifier, case-insensitively, and folds no text onto another
name. litestream's seven subcommands each dial `{--socket}` and post to
`http://localhost/<command>`: 14 per-row questions drew 7, 7 and 9 names in
three draws, now two questions (asked in one window) draw one name per draw;
its populate.go rows, 11 of them unanswered before, take SQLite from
`database/sql` with no question. The socket and the HTTP server stay two
destinations that happen to share a name: the code does not follow the
client's `Transport.DialContext` to the socket it dials.

**What a call reaches.** Which value of a call names what it reaches is
the model's one decision per outside symbol, never a list of packages in
code (`repomap.atlas.argument.v1`, column `argument`, stage `atlas_api`,
Jev, `Memoize` under `apiargument:<symbol>`, `prompts/api_argument.md`).
It is asked of each symbol whose `talks` answer reaches something by what
its call is given: `client_request`, `db`, `queue_producer`,
`queue_consumer`, `sdk` or `file` (`serves` has its address among its
words and `runs_program` its own question). The item is the symbol, its
declared type, one call as written, its `talks` answer and `arguments`:
each argument by position (with the parameter's name when the declared type
gives it) or keyword, and the receiver, as request-local refs, each with
where the call's value comes from; the options are those refs and `none`,
every ref with one set of criteria. The walk (`DestinationReader`) follows
the chosen value at every call of the symbol, by position, keyword or the
parameter's name, never through test code (2026-09-30: the URL a test hands
the program's sender is not where the program's call goes; freqtrade's
webhook calls, once its tests were its own files, walked only into them and
left the page). Where it meets what another outside call returned: a
call whose words an answer made a command-line option gives `{--name}`, a
call the facts name as an environment read `{env:KEY}`, a symbol with a
decided argument goes on through that argument, and any other stops the
walk at its call; a symbol not yet asked is asked the same question in a
next round with `result_given_to` (the reaching symbols given its result),
until a walk meets no symbol not yet asked: `http.NewRequestWithContext`'s
request handed to `Client.Do` is asked and answers `url`. A `file`
symbol's calls are walked the same way and logged as `atlas_files` lines in
`tables.md`. A method a request builder states is no longer read from its
arguments.

**Files a program keeps** are its data (owner, 2026-09-29), gathered in code
from those answers with no question of their own and no role (skeptic,
2026-09-29: the path as written and the functions reaching it,
`reading.FileReader`). Every call outside tests of a symbol answered `file`,
in declarations the program runs, is walked along its decided argument, and
the calls are grouped by where the walk ends: a literal or a template as the
walk writes it (`dump.rdb`, `{db.path}-wal`, `{--config}`,
`{env:KVD_CONFIG}`, a stored field value the walk reads as `initializer:`)
is one file; a template writes a part whose walk established no address as
the code wrote it there when it names a value (a field, a parameter, a
receiver, an element), else where its walk ended, so litestream's
`db.path + "-wal"` is `{db.path}-wal` however deep `db.path`'s walk went (it
had read `-wal` through a failing Go return, GO § Source values,
2026-09-30); a walk ending at a value its adapter could not read (kind
`unknown`, C's `(struct sockaddr*)&sa`) is `Unread`: its expression stays
its frontier for the destinations question, and the page says the address
is not established from code (REPORT); a field the walk cannot follow further whose accesses the
program records by that path (`server.dbfilename`, a field of a file-scope
variable) is one file named `{server.dbfilename}`, whose values are the
field's writes in the program, each walked from the value it stores
(ProgramIndex `Relation.Value`: `"dump.rdb"` at `initServerConfig`, what
`zstrdup(argv[1])` returns in `loadServerConfig`, not established); any
other end, including a symbol with no decided argument, is a path not
established, one per function making the calls, never invented. Each file
is a data record `w1`, `w2`, … of kind `file` (origin `call`, scope the
program's target) in the order of its first call, listing its calls (the
outside symbol, the declaration making it, its site, each site once) and a
field's values (the path, or none, the declaration writing it, the site).
Only C records a write's value today; a Go field's writes carry none, Python
sets no path and a field stored twice is unknown to it, JS/TS and Clojure
record no field writes, so their files end at a literal, a template, a
setting or a path not established. A file reads nothing else from the
model: which of its calls read and which write it is not decided (a
per-call question would be).

An outbound call names the extracted tables among its values as `data_ids`.
It also names where its program reaches it from (`reached_from`, a code
fact computed once at projection and persisted with the index): from the
declaration making the call, exact `calls` relations are followed backwards,
per program, only through callers in that declaration's own part; a
declaration no exact call reaches is reached through the calls resolved to
alternatives among which it stands, and every caller past that step is
`possible` (2026-09-30: freqtrade's `Webhook.send_msg`, called only by
`RPCManager.send_msg`'s loop over its registered handlers, had no callers;
the reading marks such a caller "possible"). Each path
ends at the first caller in another part (or in none), kept with its call
site; a path whose callers run out inside the part is kept only when it
ends at a seed or an input's handler, with no site, and otherwise dropped,
so a helper nothing uses names nobody. Callers in the program's test
sources and callers its adapter proved it never runs are skipped; a cycle
stops where it closes, and there is no depth cap. A call no drawn part
holds has none. Redis's connect, written in anet.c's
anetTcpGenericConnect, is reached from syncWithMaster in redis-server,
cliConnect in redis-cli and createClient in redis-benchmark, not from the
wrapper one hop inside anet.c. No model is asked.
GroupsIndex derives no chains (paths from an operation to an outbound call)
and no operation types. It derives what each input's handler reaches, the
dispatch sites and the phases, after the joints are in and whenever an index
is loaded or built; none of it is persisted, and a rendering from the saved
overlay derives exactly what the ordinary run did.

- **Reach.** From an operation's handler the walk follows structural
  relation-target edges of kind `calls`, `executes` or `invokes_external`,
  resolved exactly or as alternatives, from every declaration so reached. A
  read from a function, method, lambda or module body into a variable or type
  adds the target one step deeper and walks nothing from it (a table's stored
  callbacks are not reached by reading the table). Imports, hand-overs
  (`passes_callback`), decorations, writes and unresolved calls are never
  followed; a stored callback's witnesses draw possible arrows but are not
  reach. A relation resolved as alternatives is not followed into another
  input's handler; an exact call into one is. Depth is the fewest calls from
  the handler, a read counting one; declarations are listed breadth-first over
  the structural edges.
- **Parts entered.** For each part holding reached declarations the reach
  lists every followed relation into it from a part reached at a lower depth
  (its witnesses; none is chosen) and counts the others. A caller off the map
  stands for every earlier part reaching it only through code off the map; a
  handler off the map enters its first parts itself.
- **Hand-overs.** A reached declaration that runs (not one only read) handing
  another input's handler over registers that input
  (`HandsOver`/`HandedOverBy`). A table read registers nothing.
- **Dispatch sites** are the relations resolved as alternatives with at least
  two targets, in source order. A site names the inputs whose handler is one
  of its alternatives and, only when it dispatches one, the inputs whose reach
  holds its declaration, each with every followed call on any route from the
  handler to it (none is chosen).
- **Phases.** `runtime` is what any input's reach holds; `init` what the
  target's seeds reach over the same execution edges and no input does (the
  launch and the main loop around the work); `both` is in each; a declaration
  neither reaches (a callback a loop stores until what stores it is an input)
  has no phase and is never quiet. A program with no input handled by a
  declaration has no phases. A connection has its source subject's phase. The
  helper mark is persisted as the subject's interpretation (`helper`), and
  each connection into a helper subject is marked `ToHelper`.
- **Quiet.** A connection is quiet (drawn only while one of its ends is looked
  at) when it is `init` or `ToHelper`, in a program with a handled input. When
  every connection of a program would be quiet, those quiet only as calls into
  helpers are not, so quieting never empties a map; initialization stays
  quiet. The report draws `Quiet` and nothing else.

Without `--captions` the model is asked for decisions alone: every table's
prose cell (titles, lines, sentences) keeps its fallback, and a table of prose
alone is not sent; part names and part and area descriptions are still asked
(Architectural responsibilities), and so is the alias (Type and concept
descriptions). A prompt whose table keeps some cells in a request describes
every cell and leaves which to write to `fill`: the symbols, types, targets
and joints prompts ask for "the cells advertised by fill", so a caption-less
types request of an English name asks `line` alone and the model writes no
`alias`.

**Boundary rows.** A boundary row requests explanation, never existence or
kind: a `line`; for an incoming entry its `name` (Operation ownership), with
or without `--captions`, and no `method` field; for an outgoing fact a closed
original address ref, while its native kind and dispatch basis cannot be
changed by model cells. What an outgoing call reaches is its destination's
one answer (One destination, one name), never a cell of its row. An entry's
rows share windows without an owner. Without captions a row with no decision
to make (an outgoing row whose address code knows or that has no candidate) is not sent
and has no line: `Boundary.Line` holds only a line the model wrote, the fact's
given text being only the joint request's context, so an entry's or outgoing
fact's summary is empty unless the model explained it (the reading and Find
name the handler; an outgoing row names its call or kind). A refused line
leaves no line. A row whose only address option is `unknown` reads any address
answer as `unknown`. The address catalogue lists only literals that can be
addresses (no format templates, nothing from formatting, logging, time or
string packages), sent only for outgoing rows whose address code does not
know; an outgoing row carries its call's outside `package` when code names
the call at its site. A destination's item carries `reached_from`, the
declarations its program reaches the calls from, by name, each once:
GroupsIndex's `reached_from` as the reading computes it before the parts are
drawn (a file stands for its part: exact callers are followed back through
the call's own file to the first caller in another file, or to a seed or an
input's handler where they run out, through alternatives only where no exact
caller is; test callers and callers the row's programs never run are
skipped), so a connect written in a network helper is
named by what reaches it (a replica's primary, a client's server), not "TCP
endpoint". A row's choices are the same whatever rows share its window; an
owner's calls near the line and its source context are sent once per window
and rows reference them. Boundary source context
carries each original call site to its owning declaration, with safe
receiver/source arguments, native API and same-line columns. `external`,
`direction: out` and `invokes_external` describe source indexing or call
direction, not a proven exchange with another process. Calls stay individually
anchored; a shared name or a count of Do sites cannot establish the number of
systems: only the walk ends above make calls one destination.

## Type and concept descriptions

- Type descriptions use the symbol stage and knowledge with
  `lines/prompts/types.md`. The atlas graph and saved reading input keep a
  type's declarations through exact native owner IDs, including cross-file
  methods and explicit Go interface method declarations; interface
  declarations get no invented direct-call node. Type context keeps the
  bounded author quotes, later sentences included; file/callable rows use
  first sentences. The type table asks for a short explanation, an English
  alias (below) and key flag; it cannot classify activations. Its explanation
  is prose: normalization keeps complete sentences and qualifications, never
  cutting at 240 characters; what the explanation covers is `types.md`'s. Bare
  names without owned declarations or author documentation stay in the source
  index without an invented definition. A file's model hypothesis is not type
  evidence. Callable rows keep their own contract and memo identity. Concept
  explanations on maps project interpreted type subjects and seed the shared
  glossary without new definition calls or a second semantic graph. Effects
  implemented elsewhere still need their actual source contract; method
  ownership alone does not establish them.
- This covers nominal native type declarations, not every domain value:
  functions returning maps/vectors and constants keep their callable or
  variable identity, their descriptions available within the owning part, and
  an architectural core responsibility does not imply a declared data type, so
  missing concept cards prove neither missing extraction nor absent domain
  entities. Recognizing such values as concepts needs an
  explicit source-grounded interpretation contract, never a renderer name/path
  heuristic.

Symbols and Types may return one short English `alias` beside their
explanation, a reader-facing label for a non-English identifier; `none`
records no alias. It is asked, with or without `--captions`, only for a name
code finds non-English (owner decision 2026-09-26): one with a letter outside
the Latin script (`parse한국` yes, `snake_case_ascii` no; digits, underscores
and punctuation are not letters). The symbols and types prompts say the alias
is asked only for a name not in Latin letters and never ask the model whether
a name is English. Overview rows split by name: a name needing an alias is
asked the complete table (rounds 5 and 6), the others the table without it
(rounds 3 and 4), in separate requests. Both shapes of the functions' and
types' requests run side by side and join in step order. An English type asks
`line` alone, in one request shape and memo shared by both modes; without
captions an English function is asked nothing, and a function needing an alias
is asked `alias` alone and has no line. A transliterated Latin name
(`juga_jeongbo`) gets no alias: a known limitation. Accepted aliases flow
through symbol knowledge and the atlas into GroupsIndex interpretations;
native names, IDs and source anchors are unchanged. The report keeps aliases
English in every language, shows the code name beside them, and localizes the
description; both spellings address one glossary definition. Beyond that
request-side decision, no script detector, transliteration, per-name request
or browser-generated label is added. Current format constants are linked from
[CURRENT](../agent-room/CURRENT.md#formats).

Names, signatures and argument names are useful clues for model hypotheses;
absence of comments must not prevent orientation. Keep those hypotheses
distinct from observed actions and effects established by native caller
evidence. Earlier source-body/context experiments do not authorize
implementation-body retrieval in ordinary requests.

## Questions and Learn

This optional cascade — `atlas_learn`, `atlas_question` and `atlas_answer` —
runs only with `--learn` or a `--question`. Without it the ordinary atlas
completes through joints and publishes its full map, orientation and source
reading without question results. Explicit read-stage development budgets
remain available throughout; otherwise there is no question quota, row-count
cap or 64 KiB planning cap, and actual provider preparation/resource refusals
partition complete input losslessly.

`--question TEXT` is repeatable and supplements local `.repomap.conf`
questions. Settings are loaded once and passed as a typed value through target
work and serving. Questions share the graph, recalled descriptions and
provider input; each keeps independent source selections and question-keyed
request references. Learn links questions to short source-anchored answers;
supporting reading routes remain collapsed. The flag adds shared retrieval and
a shared final-answer batch after the ordinary atlas; `read` runs them without
it. `--through question` stops after retrieval; `--through answer` changes the
final answer contract while reusing unchanged retrieval. There is no separate
`route` stage; requesting it returns migration guidance.
`question-routes.json` stores the current reading records with every candidate
and its original evidence.

**Answers.** The answer batch reads the complete union of those original
sources once, with each question's own allowed refs and retrieval coverage.
Each accepted row returns its state, an answer, its basis, sources in useful
reading order, and the remaining gap. The supporting guide projects those
ordered sources, with no separate selection or extra open question. Questions
and source records are canonically ordered for exact-request reuse; results
keep the user's question order. Adding a question regenerates its answer
window; there is no per-question answer memo. Actual prepared-input, context,
output or response-envelope refusals split questions first, rebuilding each
child's complete evidence union; only a singleton question whose complete
evidence does not fit partitions its original sources into separate partial
answer parts, keeping their anchors.

Each question validates independently: a malformed, missing or differently
repeated answer leaves it unavailable while accepted neighbours survive; an
identical repeat is one answer. The state is the model's closed decision,
never promoted: an `unanswered` row keeps its state and gap, and any answer,
basis and sources it carries are discarded and journaled as `cell_rejected`.
An `unanswered` or `partial` answer may leave its gap unnamed (`none`), and a
sourced answer its basis. A substantive answer without text or original
sources, a settled answer with a gap, and inapplicability on incomplete
evidence are refused. Answered, partial, unanswered, not-applicable and
unavailable stay distinct. An unparseable response, a failed provider call or
a response with no accepted row on a multi-question window divides the
questions like a resource refusal, down to one question per request, whose
refusal is its unavailable answer. The refused attempt stays in the journal as
superseded; nothing is repaired or retried with the same bytes.

Final answer prose keeps paragraphs and complete qualifications; only short
label cells are whitespace-collapsed or length-trimmed. Source checks keep
owned declarations together, each with its original code link. The final
answer reads the original question and evidence. What a complete, attributed
answer is (completeness at the requested level, documentation-only behavior
attributed in the answer itself, per-call receiver and argument associations,
the background-worker distinction retrieval shares) is stated in
`lines/prompts/answer.md` and `questionbatch/prompt.md`. The basis shows
directly below the prose, with the same display binding and shared source
control; original source checks remain collapsed. Useful deductions, including from
names and signatures, are welcome; they stay recognizable as interpretations
checkable beside the original excerpts. No body retrieval or new semantic graph is added.

Shared question retrieval and final answers opt into provider-supported
reasoning; how each endpoint encodes it, its overrides and its cache identity
are EXECUTION's (owner, 2026-09-08), and other atlas tables keep their fast
mode. Learn proposals, question retrieval and final answers use the shared
128,000-token output allowance; a lower configured provider ceiling still
applies, shared by reasoning and visible output.

**Learn proposals.** The ordinary reading adapts eight base learning intents
after the atlas with the configured client, then answers its proposals through
the same question and answer stages. Preparation starts with the complete
original learning evidence and all eight intents, sized by the actual provider
request envelope, not a 64 KiB fragment budget. Each proposal request encodes
repeated component/area context once behind local refs while keeping every
evidence item's complete original observations; partitions rebuild their own
context catalogue without a parent or sibling window. Actual
context/output/response resource refusals partition complete original evidence
by encoded byte weight; accepted sibling reviews survive, children keep their
partial-context scope, and failed parents supply no semantic review.

Proposal and menu decisions validate per intent; a refused intent stays
unavailable without deleting its neighbours. Reviews may arrive as a bare
array. An intent matches after trimming and case-folding, a state also reading
spaces or hyphens as underscores. A review is read field by field: a string of
refs is a list, a non-text reason is none. Two identical reviews of one intent
are one; two different ones are refused, never joined. In a `questions` review
each proposed question validates alone: one malformed or failing a rule (blank
wording, no or only unadvertised sources) is dropped with a
`question_rejected` journal row, the rest kept; a question without a why keeps
its wording and sources. A review is refused only when none survive. A
`questions` review without a reason takes the first sentence of its first
accepted question's why and records `reason_from: why`. A `not_applicable` or
`unknown` review keeps its state, reason (even empty) and sources as written;
questions it carries are dropped as `question_rejected` and never promote its
state. `not_applicable` still needs complete context and positive sources; a
review without a readable state, or with a state outside the set, is refused.
An intent with no entry in an accepted response is re-asked over the same
evidence — first with the other omitted intents, then alone — before it is
unavailable (`intent_omitted` journal rows, `recovered` when a later window
reviewed it); intents follow the evidence in the request so every re-ask
shares its parent's request prefix. An intent reviewed by any window is not
marked unavailable by windows that skipped it. Failed consolidation keeps
original accepted questions and any accepted comparisons, the plan marked
partial (owner, for user-selected repositories, 2026-09-06). `read --through
learn` stops after the plan. `learning-plan.json` keeps each context review
and every proposal's original intent, reason and sources.

Automatic proposal selection composes the base-intent menus together through
the same executor: each intent has one mandatory set-valued decision over its
closed candidate refs, asked for at most five; all rows share the original
candidate catalogue and read their full curated learning goals. A menu past
that limit keeps every advertised question it chose (`menu_over_limit`), and a
menu without its rationale keeps its choices. Selected and unselected
questions keep their original sources and the menu rationale; an unselected
question is not declared inapplicable or judged unsuitable. Explicit questions
bypass this selection. Context partitions reduce without a question quota;
decisions made before the candidates fit together keep their
partial-comparison scope, including a nonshrinking fixed point, and the report
implies no whole-menu comparison. Stage prompt overrides do not replace the
separate selection and consolidation contracts. The merge step groups the
selected questions by information need in one call per pool (`groups` of
members with a representative; members may be one string of refs); an unplaced
question stays its own group, an empty `groups` list means no repeats, a
refused window keeps every question, and an answer whose groups name no
advertised question is refused. Overlapping automatic questions share one
answer; explicit questions remain visible. Exact duplicate explicit/generated
wording shares a result carrying both origins. Questions expose their
selection reasons and original excerpts. Answers offer existing term
explanations when they selected that exact named declaration, linking to the
term's map memberships. Generated declarations participate. A technically
valid plan is not by itself evidence of a useful introduction;
[CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work) owns ordinary
report and answer acceptance.

Go test declarations enter the same index and question rows from
build-selected go-list test inventories, with exact locations, including
external tests, private packages and test-only directories. They belong to
their module component (or their own executable package), never become
production API or launch seeds, and carry no test call graph, execution result
or assertion semantics.

**Question retrieval.** The shared question cube selects closed source anchors
for every question from one evidence catalogue. Complete evidence precedes the
changing questions; unselected rows need no negative explanation. Every
question is mandatory in a response and keeps per-chunk inspection coverage. A
window asks at most eight questions over its complete rows: density of
decisions per response loses answers. Windows over the same rows share a
request prefix, the first running before its siblings. Only explicit provider
context/output/response resource refusals authorize lossless partition of
complete evidence rows: output refusals split independent questions first,
keeping their complete evidence; input/context refusals split by actual
encoded input weight. A question left out, or named only with missing or null
selections, is asked once more over the same rows with the other omitted
questions (`question_omitted` journal rows, marked recovered when answered);
so is every question of a response that decided none, unless that would repeat
its request. A question omitted twice stays unavailable. Per-question memos
reference the original shared response, with the input metadata to reconstruct
and compare its exact prepared request; replay is revalidated against that
complete window before reuse. Adding or reordering questions reuses existing
decisions; canonical ownership is restored from the current graph.

A retrieval row states each fact once: a unit's `heading_path` lists only its
parent section titles, `anchor_path` appears only when it differs from the
row's path, and a type member that is itself a unit of the same chunk is
referenced by its `a*` ref; the stored evidence restores the complete record.
Selected type anchors keep their native owned declarations and exact member
locations, shown under the answer's source checks. Types beyond the
description-candidate budget remain available for question reading. Every
selected anchor keeps its own subject and original evidence; its row's reason
is a shared relevance hint, not a proof about each declaration. The reader
uses the same sealed in-memory graph the reading input persists, so ordinary
and saved readings bind question results to the same graph hash. Connections
keep their source kind; the route is not an execution trace.

## Question evidence and per-anchor relevance

Decisions are read per (question, row) cell, relevance per original `(row,
anchor)`: distinct anchors in one chunk may be direct or context
independently, and different rationales are kept in stable order. An unknown
entry, row or anchor is discarded; a selection naming no known row is
journaled as discarded. A malformed selection (including a known row named
only in another shape: the bare ref, a list or another field), a positive
selection with no advertised anchor, a relevance other than direct/context, or
one anchor given both relevances refuses only that cell; the prompt still says
such a conflict leaves the question unavailable, so the request bytes stay
unchanged. An entry whose selections are not a list refuses its question in
that window, even beside a readable entry. A refused cell stays unavailable,
never a negative finding, and blocks its row's optional glossary metadata; the
question's other rows and accepted neighbouring questions survive, with cache
and memo reuse. A later run asks a refused cell again in its own smaller
window. Harmless forms are the same answer: relevance in any case or padding,
one anchor as a string, a blank reason (a selection without a hint), a bare
array of entries, or one wrapper object around `questions`. Repeated entries
for one question compare cell by cell: alike is one answer with merged hints;
different, including a selection against an explicit empty list, is refused.

Each selected declaration keeps its native call/API facts in written order (as
the orientation lists them), safe source arguments, receiver/result
expressions and source-qualified ownership. `BoundarySourceContext` and
question call catalogues use the same safe source projection; canonical IDs
stay local. A main-flow or business-effect claim must follow actual
observations or remain an explicitly qualified interpretation. A source path
is not proof of a call, and its reading order is not execution order.

## Entity knowledge

The owner wants descriptions and discoveries to accumulate on internal
entities and serve later model cubes and people. A function's general behavior
is reusable across callers; the purpose and arguments of one call belong to
that call's context. Do not re-read a shared lower-level subtree because
another caller reaches it. Keep source facts and model interpretations
distinct and record which evidence and earlier interpretations each conclusion
depends on, so affected knowledge can be invalidated when its basis changes.
This stays a small record boundary, not a large taxonomy or plugin system.

A question stop's SubjectID is distinct from its context PlaceID, with
original selected evidence kept. Native declarations use their ProgramIndex
object IDs; boundaries and observed entities use graph IDs. Context fields
keep their actual context path, so generated-file attributes cannot silently
describe a configuration anchor. Internal IDs are restored locally, never sent
to the model.

Independent directory, file, symbol and boundary interpretations persist as
`knowledge.json`. Each record binds the internal subject, current owners,
context, exact single-row evidence, cells and dependencies on earlier model
interpretations. The model still receives batches; single-row preparation only
computes identity. Counts, `tables.md` lines, rejections and acceptance follow
row order however rows are prepared and recalled. Only these independent
tables opt into reuse, and their prompts require row independence; comparative
stages keep complete request identity. Provider exchanges keep real transport
accounting; reused entities count as `reused_rows`, not provider cache hits.

Question-only readings use the same independent row builders in recall-only
mode: they recall current descriptions and selections and make no description
or selection requests. Source rows receive explicitly labelled prior model
hypotheses; stops keep used knowledge IDs. This reuses extracted-evidence
descriptions; it is not implementation analysis. Functions still cover
selected declarations by names, signatures and docs, so an uninspected body
change does not invalidate their descriptions. Changed target inventories may
reuse byte-identical row inputs with new entity bindings: exact input reuse,
not fuzzy identity matching, with no old-format readers or cache migration
paths.

## Matching and operation paths

Matching keeps native imports, calls and callback transfers separate from
model-confirmed integration hypotheses. Every peer window is considered, never
a silently selected first subset. Rows share a peer dictionary only when they
have identical eligible counterparts: the same source object,
same-component-only counterparts and incompatible fixtures never appear as
choices in a request that forbids them; shared dictionaries stay exhaustive
over admissible peers. Peer eligibility is partitioned inside each bounded
window to keep unrelated outgoing rows batched. Inferred connections keep
their original joint identity, source/destination subjects and exact anchors,
including distinct calls at one source line, in GroupsIndex, with no legacy
reader. Blind matching compares each peer window's winners again against the
original evidence until one counterpart or none remains per outgoing boundary,
never publishing an independent winner per window. A blind call's counterparts
are only inputs whose handler is known. When a program has a confirmed
integration into a peer program, each row of its accepted tables of inputs is
one peers row against the peer's inputs (K5): the chosen input is kept on the
row (`Operation.Sends`), never drawn, and equal words never link. A peer
boundary whose operation GroupsIndex folded into another (a handler registered
twice under one name, a hand-over joined to its word entry, an option repeated
at a second site) names the operation standing for it. Candidate declarations
keep exact signatures and comments, including streaming result types; protocol
similarity alone is not the same operation. No Cobra-specific detector or
parallel graph exists.

A program's request to what it serves itself is joined by the code, never
asked (reading `self_joints.go`, 2026-09-30): an outgoing request whose
written address is on this machine (a path with no host, a loopback host,
the unix network) and whose method and path, or value, are those of one of
the same target's requests or listening addresses whose handler is known
(the equal-value rule above) is an integration joint of the target with
itself, the only joint one target has with itself. A unix socket joins as
possible, its path known only at run time. A request so joined is offered
to no other program's peer. litestream's subcommands post to
`http://localhost/start` and `/info`, which its own Server serves, and had
stood as an Outside "Litestream" on litestream's own map; a request to
another host with the same path stays another server's. The rule reads
every language's requests and routes alike: the Go fixture's `fetchLevels`
gets `/api/levels`, which `registerLevelRoute` serves; the Python, JS/TS,
Clojure and C fixtures request no route their own program serves (no
adapter work is missing).

The operation map follows native calls and executions from the selected
subject, with cycle protection, and projects the visited subjects to groups.
Imports and supplying a callback or service object are not execution paths;
where an external framework invokes a supplied callback without an observed
call the path stays open, the binding kept in the graph. A model-matched
endpoint is on an operation's path only if its caller is reached; otherwise it
stays visible on the caller's group. Cross-component endpoints link to the
matching operation where its anchor is known. These are possible static paths,
not a runtime trace; unresolved dispatch and model integrations use dashed
lines. The common system map composes these per-input paths across exact
matched input endpoints, with cycle protection; reaching a peer part alone
never starts all its inputs. Each continued path keeps a source witness from
both sides; the integration step stays distinct from a native call. External
communication belongs to an input only when that input's execution relations
reach its original caller subject, not merely its owning part.

Independent joint protocol decisions also use exact row memos keyed by the
complete boundary, eligible peer catalogue and target context; the memo
restores and revalidates its original response row. Equal labels alone do not
establish equivalent inputs or a match.

## Stage concurrency

Stages with no data dependency run concurrently; a stage starts once every
value it reads exists. Scheduling never reaches request bytes, cache and memo
keys, compact IDs or artifacts: each concurrent stage prints, counts and
rejects into its own record, added at the join in step order, so `tables.md`,
requests, compact IDs, knowledge, rejected rows and atlas are those of the
serial walk; only the exchange journal, with the response rejections it
appends to `rejected.jsonl` as responses arrive, interleaves.

In the reading, after the files, the symbols, the outside symbols with the
boundaries their roles make, and the parts run at once and join before the
arrows, which read them all; after the core, the areas, the keys of each part
and the targets with their joints likewise run at once. Targets read their
parts (after their role split) side by side, each on its own record joined in
target order; parts take compact IDs in target order, the only point where a target waits
for the ones before it, and areas theirs once
every target's areas answer is in. The first failure stops the stages beside
it and is the error reported; `--through` stops inside them at its own stage.
A failure is never reported as the cancellation it caused in a sibling;
outside the reading's concurrent stages it is reported in step order.

Facts and claims are built side by side. The report is assembled from the
targets, groups, facts and claims while the orientation is asked; only the
glossary, display translation and publication wait for the orientation, whose
failure is still the reported cause and publishes nothing of the report.

## Orientation

`orientation` is one model-assisted stage over facts, claims and the complete
matched GroupsIndex set, asked twice, each with one embedded prompt and
response shape. The overview returns one repository summary, one role per
target, a run recipe and a closed-ref `main_flow_target`; the flow request
then returns the main flow's title and steps over that target's flow scope. No
target, or an unknown one (refused), asks no flow. The model selects
request-local refs from the exact advertised artifact identities: `t*`
targets, `a*` facts, `h*` claims and target-qualified graph subjects such as
`t1.n22`; groups likewise use qualified IDs such as `t1.g3`. Go allocates no
second numbering scheme before the call.

Validation is a pure function over the response and the advertised catalog.
Set refs filter unknown or incompatible members and deduplicate repeats,
recording ignored refs; one bare-string ref is a one-element list. A cited
member keeps its target-qualified ref (`t1.n22`) in the summary refs and a
role's subject ids, which the report resolves to the member's own source; flow
steps keep their target beside the bare member id. A row with no required
evidence, a recipe step with no manifest or entrypoint evidence, a row naming
an unknown target, a role without a label, a multi-line `cwd`, or a flow step
naming another target's member is rejected with its raw JSON and a reason into
`rejected.jsonl`. Target refs and `cwd` are trimmed before checking. A role
keeps an empty purpose, never filled in: the label is the decision. An invalid
optional recipe note is dropped and recorded; its step stays. Refs go only in
the ref fields (the overview prompt says so, 2026-09-30): a summary or role
label writing an advertised ref (`t7`, `a12`, `h3`, `t1.n22`, `t1.g3`) as a
word is refused, and a purpose or note writing one is dropped and recorded
while its role or step stays; nothing is rewritten, and a ref-shaped word the
request does not advertise is prose. freqtrade's summary had read "main
program (t1)"; on its saved request two of three draws of the former prompt
wrote refs in the summary, none of three with the rule. Summary, roles,
recipe and flow validate separately, so a wrong field type discards no
accepted section. Equivalent target roles, including purposes differing only
in whitespace, combine their evidence; conflicting roles leave only that
target's role unavailable. Complete prose and qualifications survive; only
short labels collapse whitespace. Unparseable responses, and responses whose
every row was refused, yield the legitimate empty orientation and are not
cached; the latter journal each row's reason. Optional terms follow accepted
sections and rows on live and cached responses. Rejection never aborts the run
and no replacement is invented.

The overview carries the complete facts, claims and connections, the groups
with title, summary and `member_count` but no member lists, and each seed's
complete row. The flow request carries the chosen target, its facts anchored
inside the scope (the innermost declaration holding the anchor is a member; a
module holds its file) and one complete row per member. The scope is Launch ∪
every input's Reach ∪, transitively, every repository callable a member hands
over (`passes_callback`) or registers (a registration fact's owner and
object), with what that one runs; only this evidence scope follows a
hand-over, GroupsIndex Reach does not. Members come in reading order: the seed
walk breadth first, the rest of the launch walk, then each input's reach in
operation order; a handed-over callable and what it runs follow the member
handing it over. A member row is that of the declaration place with its
`groupindex.DeclarationKey` (path, line, column, kind, name), never looked up
by the member's own object id (a place merged across programs keeps one
program's). A row lists every call in written order as a lossless tuple
(`name@line -> callee | …`, its non-default
kind/invocation/dispatch/resolution words, then args, receiver, result, values
that say more than the literal arguments, arguments, api, detail and evidence
refs); only the call's and callee's columns and canonical ids stay local, and
no `called_by` is sent. A flow request the provider cannot hold is journaled
under `flow_request` and the overview stands. Nothing is sampled, windowed or
reduced. Fact rows identical but for ref and target are one row with a
`targets` list and the first target's fact id as ref; a role or recipe step
naming one of those targets keeps that target's own fact id. Connections are
one row per from, to and kind with every distinct label and every sentence
differing from its label. The answer allowance is 16,384 tokens (EXECUTION).
The stage caches on its stage identity, prompt version and input digests
through the shared executor.

Original declaration kinds, calls, source arguments and owned fields can
qualify member behavior. A launcher/router, implementation mechanism, callback
and remote destination remain distinct roles. Parent heading context labels
the scope of author claims. No prose explanation supplies a missing source
relation. A launch fact supports an entry point, not a complete invocation:
run recipes follow `internal/orientation/overview-prompt.md` for required
arguments, prerequisites and explicit placeholders, and an unsupported
invocation stays absent rather than losing those requirements.

## Optimization acceptance

Follow [Development: evidence before optimization](DEVELOPMENT.md#evidence-before-optimization). Missing a caption, fewer selected symbols or a smaller request count alone does not prove that reader outcomes survive.
