# Implementation and acceptance journal

This is a concise living log, not another ADR. [CURRENT](CURRENT.md) owns the
current decision/status; [topical contracts](../../AGENTS.md#start-here) own
implementation details. Record a changed decision there in place. Append only
useful implementation/acceptance evidence here; archive superseded detailed
runs instead of growing the entry pages again.

## 2026-09-11 — current correction wave, ordinary acceptance pending

- Boundary, file, directory, zone, target and portfolio requests state
  each fact once and offer only real choices (schema review, cold fixes;
  boundaries v7, files v5, directories v4, zones v2, targets v3, portfolio
  preparation 8). Boundaries: the address catalogue no longer lists format
  strings or literals from `fmt`/`errors`/`log`/`time`/`strings`/`strconv`
  calls (Morfeu: `unknown` in 25 of 29 answers, the catalogue being error
  templates) and is sent only when the code does not know the address; the
  out-kind options are the kinds the group index keeps (no `http_server`,
  `config`); `destination` is a closed `d*` choice from the shared known
  systems list annotated with the target's dependencies, with `other: `
  for a system outside it, so the page no longer normalises four spellings
  of one broker; the outgoing `line` is a 120-rune text; the owner's calls
  (line ±3 and client constructors) and source context are sent once per
  window in `context.owners`, rows referencing `owner_ref` (51 % of a
  Morfeu window was that repetition); fixed native facts get their own
  24-line prompt. Files: `box` is asked only when there is more than one
  option (`Missing: "here"`), callers name the declarations that witness
  the file edge. Directories share the parent's line in the window context;
  a zone part may hold one box. Targets define `shared_code`, target kinds
  and boundary labels; portfolio observations carry named fields instead
  of positional values.
- Three small stages ask one decision each (schema review, cold fixes).
  The README classifier asked ten classes over 14 KB of rules while only
  `target_entry` was consumed, and on Morfeu classified exactly the README
  files whose text it was given; it now asks which files the guidance names
  as entries, sends candidate files only (no prose, nothing under
  `.claude`/`.github`/`.vscode`), and answers `{"files":[…]}` in JSON mode
  (system prompt 13.6 → 4.8 KB). `documentation_reduce` drops `claims`
  (read by nothing but the glossary collector, where governance-template
  phrases entered the report) and caps `concepts` at twelve per document.
  The glossary labels each term with a closed `kind`; `identifier` terms
  (54 of Morfeu's 122: env names, file names, `RF01`, skill names) are
  accepted but not published. Contracts of all three stages bumped.
- The learning menu is bounded and questions are merged as groups (learn
  select v4, merge v2). Morfeu with the owner's 524,288-token window split
  Learn into 8 windows, each proposing "without quota"; select kept 11–14
  questions per intent and the row-form merge (each question choosing its
  own representative) merged none of 100, so a run's question count
  depended on how many windows Learn needed (40 vs 100, tokens ×2). Each
  intent's menu now chooses at most five questions (`limit: 5`, an
  over-limit choice refuses that intent's row); the merge is one call per
  pool that partitions the catalogue into groups of one information need
  (`{"groups":[{"representative","members"}]}`): on the saved 100-question
  catalogue the row form merged nothing twice while the group form gave 78
  groups with 16 sensible merges. Unplaced refs stay their own group, a
  refused window keeps every question.
- Question evidence sends each row fact once (question-batch v4, answer
  v9, learn v4). On the Morfeu retrieval window (2.07 MB for 24
  questions) `heading_path` cost 191 KB, its last element repeating the
  unit's own `section_title`/`section_line` and the parents repeating per
  unit; `anchor_path` (98 KB) equalled the row's `path` for 1,807 of 1,834
  units; `owned_declarations` copied type members that were already units
  of the same chunk (274 KB on the Freqtrade code window). A unit now
  carries `heading_path` as its parent titles only, `anchor_path` only
  when it differs from the row, and a member that is a unit of the same
  chunk as `{"ref":"aN"}`; `AnchorEvidence` restores the full record for
  routes, Learn and answers. Morfeu window −10 % (2,074,710 → 1,864,461
  bytes); memos and exact cache of these stages go cold once.
- Six places where the model was asked without a decision to make, or asked
  twice (schema review, warm fixes; request bytes of the remaining rows
  unchanged). A column-less native outbound fact now claims every selected
  call on its line (Morfeu `152759`: 4 of 29 outbound records were the same
  call twice, `bnd:…:sdk` beside `out:…:166:42`); the claim matches every
  native source, since an SDK observation arrives as `external_call`, not
  `fact` (run `171727` still showed the four duplicates while the claim
  matched `fact` alone). Orientation numbers groups
  and members in an order the graph fixes (sorted member ids, then lane,
  title, summary) so one group's changed lane no longer renumbers every
  `g*`/`s*`. An arrow without witnesses (an import-only edge) gets its
  fallback sentence "A uses B." without a model row (Morfeu arrows r7/r10
  had invented sentences). A declaration whose native route is its
  operation is not reviewed by the operations table (its model row was
  dropped by the group index anyway). The glossary comparison round is
  skipped when every group is a singleton whose lowercased names never meet
  (Morfeu: 121 self-assignments, 36 KB). Files under `.claude/`, `.github/`
  and `.vscode/` are never portfolio candidates (Morfeu asked about a hook
  script every run and warned "Target not analyzed").
- Retrieval asks at most eight questions per window and re-asks the ones
  the model omitted. Freqtrade `20260910-144751`, window w1: 64 questions
  over 460 code rows (4,661 anchors, 2.1 MB) came back with four keys and
  `finish=stop`; 60 questions became "missing question" and 27,600 cells
  unavailable with no second request, while the same 64 questions over
  1,241 document rows came back complete. Probes on that saved window:
  eight questions over the same 460 rows → 8/8 in 48 s; with thinking off
  the 64-question window degenerated into counting anchors up to the
  128,000-token cap, so reasoning is not the lever. `plan()` now cuts a
  window's questions into groups of eight over the same rows; the first
  window of each row set runs before its siblings so DeepSeek's prefix
  cache serves the rest (parallel probes hit 1,408 of 516,842 prompt
  tokens); after the first round the questions absent from an accepted
  response are re-asked once over the same rows (`question_omitted`
  journal rows, `recovered` when the second round answers, one console
  line naming the counts). Prompt and contract unchanged; per-question
  memos survive; whole-window cache entries of the old packing go cold.
- A refused answer window divides by questions before giving up. Morfeu
  `20260911-112125`: one empty provider response (`provider_no_content`)
  made all 22 questions of a window unavailable; only resource refusals
  divided a window. A failed call, an unusable envelope or a response with
  no accepted row on a shared window now divides its questions like a
  resource refusal until a request holds one question; the journal keeps
  the refused attempt as superseded and the console says why the window
  continues in smaller requests. Request bytes unchanged.
- A text the translator refuses stays in the source language instead of
  failing the run. The owner's run lost `t38` three times in single-entry
  windows, and by the previous code a terminal single-entry refusal ended
  the whole run after every stage had finished. `Translate` now keeps such
  an entry untranslated, `rejected.jsonl` gets an `entry_untranslated` row
  per text and the console names them once; a failure before any provider
  answer still fails the stage. `ExecuteAdaptiveJSONEachResults` exposes a
  terminal leaf beside the completed cover; the old entry point behaves as
  before.
- Learn reads a review without a reason and drops bad questions singly.
  Freqtrade run 20260910-144751, Learn window w3: eight `questions` reviews
  with 26 questions and `"reason": ""` on every one were all refused
  ("learn: a review needs a reason") and the window was lost as "no intent
  reviews accepted" with eight unavailable intents; the owner's provider
  produced the same message. A questions review with a blank reason and at
  least one accepted question now takes the first sentence of that
  question's `why` as its reason and records `reason_from: why` in
  learning-plan.json and the tables journal; `not_applicable`/`unknown`
  reviews still need their own reason. A proposed question that fails a
  rule (blank wording or why, no or only unadvertised sources) is dropped
  alone with a `question_rejected` journal row naming the intent, the
  question's beginning and the rule; the review keeps its other questions,
  and is refused only when none survive. Prompt, request bytes and cache
  keys unchanged.
- A missing question or Learn intent review says what the response held
  instead of only the gap: the rejection reason lists the field names of
  the entries that named no asked question or intent and the values under
  key-like fields ("3 unmatched entries with fields [question_ref rows],
  named [question_ref=q1]"). The key-synonym tolerance added for a day
  (`question`, `question_id`, intent titles, keyed objects, keyless order)
  is withdrawn: no saved response showed a provider naming keys another
  way, while the Freqtrade run that asked 64 questions in one window got
  q1, q2, q3 and q64 back with the right keys and nothing else. The gap
  is an oversized window, not a naming mismatch. Contracts unchanged.
- The Learn plan is read per intent, not per window. After a context
  refusal the evidence continues in several windows, each asked all eight
  intents; a window that skipped an intent another window reviewed used to
  add an "unavailable" review and make the whole plan "partial", so the
  questions page said "some topics could not be reviewed" although every
  topic had a review somewhere (the owner's run: six windows, 42 per-window
  misses). `consolidateLearningReviews` drops an intent's unavailable
  reviews when any window reviewed it and sets partial only for an intent
  no window reviewed; the journal keeps every per-window rejection. Two
  tests that encoded the per-window shape now assert the per-intent one.
- Editor and tool state directories stay out of the corpus: `.history`
  (VS Code Local History keeps copies of edited files, README included,
  which the owner's run then read as documentation), `.terraform`
  (downloaded providers with their READMEs), `.idea`, `.vs`. Corpus test
  covers `.history/README.md` and `.terraform/providers/x/README.md`.
- Learn selection reasons no longer show request-local keys: a model that
  wrote "q1 and q5 cover the same idea" put "q1", "q5" on the page. The keys
  are spelled out as the questions they stand for (`spellCandidateRefs`);
  keys outside the window's pool stay as written.
- A rejected response's row-level reasons reach the console. "learn: no
  intent reviews accepted" said only that every intent review failed; the
  WARN now lists up to four "rejected: intent: reason" lines from the same
  journal record (`SemanticFailureReceipt.Rejections`), for every stage.
- Selecting a code member no longer sets the part's summary vertically.
  With a member chosen the docked card gained a second column for the
  member panel (`.map-card-has-concepts`, 1fr + 1.35fr), which in the
  384 px side panel left the summary about 160 px wide; the member panel
  now stacks under the summary in the explorer. Verified: summary 367 px,
  member panel 367 px beneath it.
- Call lines keep their type when it tells records apart, and carry the
  note. The member-only line read "Patch, Patch, Patch" on a Kubernetes
  client: a member used by several types within one destination now keeps
  its type ("PodInterface.Patch", "DeploymentInterface.Patch"), a member
  used by one type reads alone ("ExchangeDeclare"), and the generic-name
  list gained Patch, Watch, Apply, Scan, Push, Pull, Insert, Select,
  Subscribe, Emit. With the telegraphic boundaries note the purpose fits
  the line itself: "ExchangeDeclare объявляет topic exchange morfeu.events
  и DLX morfeu.events.dlx · client.go:322"; the disclosure repeats the
  purpose only when it is longer than the note.
- The operation view says what it shows. After the first jump into
  "Операции › criar-filme" the owner liked the picture and asked what he
  was looking at. One muted line under the crumbs now reads "criar-filme:
  the dark box is the operation; below it the part with its handler and
  the parts that handler reaches. A solid arrow is a call in code, a dashed
  one an interpretation." (`.explorer-caption`, hidden in structure mode).
- "Where this service connects" on a real broker/database service (Morfeu:
  Echo + sqlc + PostgreSQL + Redis + RabbitMQ). Destination groups are
  keyed by the known system a destination names: one run called one broker
  "RabbitMQ broker", "AMQP broker (RabbitMQ)", "RabbitMQ broker (queue
  topology)" and four more ways, thirteen groups for three systems; now
  RabbitMQ · 17, PostgreSQL · 9, Redis · 5 (`canonicalDestination`;
  records keep their wording, an unresolved "Cache store" stays apart).
  A call line drops the package the group already implies and keeps the
  type only for a generic member: "Confirm", "ExchangeDeclare", "Pool.Ping",
  "Migrate.Up" instead of "amqp091-go.Channel.Confirm"; the full callable
  stays in the record body. An unresolved interface call takes its calling
  function's name for the line instead of its first sentence of prose. The
  body no longer prints "address not determined" or a one-step chain that
  repeats the record's own anchor. The source location sits on its own
  line under the call; the continuation rows live inside the same list.
  On the first screen the sixth and later rows of a long group open in
  place ("Развернуть · ещё N") instead of sending the reader to the
  component page ("all 13 → flung me upward").
- Boundaries prompt v6: the `line` cell is a telegraphic note of at most
  ten words without a subject ("stores events in PostgreSQL"), no narration
  of the call, an unknown appended as " · not established: …" only when it
  matters. Two probes on Morfeu (same code): a one-sentence wording gave
  average 28 → 11 words, the telegraphic wording 6 words (max 8):
  "publishes catalog events to morfeu.events with confirmation", "applies
  pending schema migrations to PostgreSQL", "checks Redis connectivity at
  startup". With the first wording "declareTopology issues an AMQP QueueDeclare on
  the broker channel to idempotently declare the DLQ … a broker management
  exchange, not a business publish" became "Declares the dead-letter queue
  for failed movie-created events on the broker"; "createDBPool builds the
  PostgreSQL connection pool from the parsed config … actual query
  exchanges happen through its library" became "Creates the PostgreSQL
  connection pool used to store and read application data". Three of 30
  candidate records were not accepted under the new wording; the boundary
  cache for v5 windows is cold.
- The page follows an opened map block only after the reader's own click
  on the map. `revealInWindow` (755383f6) ran on every layout, so a
  component page opened through "All 6 →" or a route link scrolled past
  its inventory to the auto-opened root area: the owner's "everything jumps
  somewhere on click" on the first page. `open()` now marks the map before
  it re-lays out; other layouts leave the page where the link put it.
  Verified: "All 6 →" lands on the inventory (target at 119 px), a node
  click still brings the opened block and the panel into view.
- Interface words a newcomer can read, from the audit's list of seventeen
  unexplained labels: "Входящие запросы" (was "Записи входящих обращений"),
  "Внешние вызовы", "вызов в коде" / "настройка клиента" (was "Вызов
  взаимодействия" / "Настройка взаимодействия"), "Кто обрабатывает входы",
  "Выполняет переданный код (eval/exec)", "Обработчики, для которых маршрут
  не найден в коде", "Адрес не установлен"; an address that passes through
  a frontier reads "Адрес приходит через `v5.Connect`" instead of "не
  определён · v5.Connect". The program index's platform notation stays off
  the page (`platform:javascript.WebSocket` reads WebSocket;
  `displayCallable`). The part search re-lays the map 200 ms after typing
  pauses instead of on every keystroke.
- Long lists read in groups the data already had. Operation lists longer
  than seven rows from more than one file are grouped by source file
  (gop: 17 user actions → six files), each file compacting to five rows on
  its own; the complete key-code list of a large part (`All N →`) carries a
  heading per source file; the data catalog shows tables first and SQL
  texts after, each with its own disclosure; the configuration table is
  sorted by source file so environment reads and manifest keys group
  themselves. `operationsByFile` template function; the first-screen copy
  still takes the first five rows of a group.
- Less noise on the component page. Configuration, manifest and TODO facts
  from vendored trees (`vendor/`, `node_modules/`, `third_party/`,
  `.terraform/`, virtualenvs) and data records from those trees or naming a
  database's own catalog (`pg_*`, `information_schema`) stay out of the
  reader's lists: gop's TODO list was 43 of 43 vendor files and its one "SQL
  text" was a Go error string from vendor/. The data catalog no longer
  prints "Connection not determined" under every row. The legend defines
  area, part and key code in words. Fit, zoom and reset sit in the explorer
  bar instead of a row of their own. Secondary counters and source
  locations in catalog rows are 12 px under 14 px titles instead of larger
  than them. The CSS "model" badge is localized through a `--model-badge`
  variable set from the vocabulary.
- The map opens on its parts, and hovering a node answers "what is this"
  in a readable card. A root view whose only own node was one area showed a
  single box reading "Open parts · N" (gop, meetup, python-dotenv): that
  area is now open from the start, its crumbs still name it. Explorer nodes
  carried their sentence only in the native SVG `<title>` tooltip (delayed,
  vanishing, unselectable); the title is gone and `repomapPreview.bind`
  shows a floating card beside the pointer with the node's kind, name,
  sentence and "click — explore"; the panel still changes only on a click.
  A code cube click reads in the panel instead of also following its link
  (a new tab in static reports, the overview in served ones). Verified at
  1280×900 on meetup: parts visible on load, no `<title>` left, card shown
  on a real hover and hidden on the next pointer move away.
- The map's explanation stands beside the map. The component map explorer
  put its inspector under a stage capped at 58vh, fixed the inspector at
  16rem and scrolled the page back to the map top after every click, so an
  explanation appeared 210–334 px below the clicked part, outside the
  window, and the wheel went to whichever of three nested scrollers was
  under the pointer (the owner's "awful scroller"). Now the workspace is two
  columns above 1100 px: the stage grows with its content (no vertical
  cap) and the inspector is a sticky side panel with auto height, bounded
  by the window; below 1100 px it follows the map with no fixed height.
  `orient()` scrolls only when the map is entirely out of view; an opened
  block low on a tall map is brought into the window by the page
  (`revealInWindow`). The layout width is the stage's, measured one
  microtask after the bundle has run (the first render used the figure's
  full width and produced a 1100 px map inside an 816 px stage). Breadcrumbs
  wrap instead of scrolling horizontally; code cube names clamp to two lines
  instead of scrolling inside the node; viewport history entries are
  written every 250 ms instead of every frame. Verified at 1280×900 on
  meetup and jumping-circle: part click leaves the page in place, the
  explanation is in view beside the map, the stage has no inner scroll.
- The component page leads with what the component is and ends the map
  with what happens: the orientation role and purpose (the overview card's
  sentence, same display refs) sit under the heading with the entrypoints
  on one line, and the main flow and the configuration table follow the map
  instead of hiding inside the "Code, entrypoints and sources" reference
  block. A component without a flow or a start list shows no flow section
  at all ("the model did not include this target" described the tool, not
  the code). Found by the owner's audit: five visible words below the map on
  every one of seven pages, no purpose sentence anywhere on the page.
  `TestComponentPageLeadsWithPurposeAndKeepsFlowBelowTheMap`.
- Data catalog links are live again. Record ids carry a kind prefix with a
  colon (`entity:f-…`, `query:q-…`); inside a fragment href html/template
  read the text before the colon as a URL scheme and replaced the link with
  `#ZgotmplZ` (meetup: 82 dead "SQL texts" links found by the static DOM
  audit). Page ids for data rows now come from `dataRowID`, which swaps the
  colon for a hyphen in both `id` and `href`; regression
  `TestDataRowIDsAreSafeFragmentTargets`.
- "Where this service connects" records are compact lines beneath their
  destination instead of full cards under a "Records · N" disclosure. The
  owner opened the meetup report and expected the destination title, then
  visually nested call blocks, brief, each opening its description on click,
  and an expansion after three. Each line now shows the native method and
  address, else the callable, else the first sentence of the purpose, with
  the source location; its purpose, address, basis, source anchor and
  destination chain open under it. Three lines stay in view, the rest wait
  under "Развернуть · ещё N" (`First`/`Rest` on the group, preview constant
  3). The group-level lead sentence is gone; the destination is named once.
  The first-screen copy clones the group's own children, which also stops a
  record's purpose and "address not determined" from standing in for the
  group's (the copy used `querySelector`, which descended into the first
  record). Counts and locations no longer break onto their own line under
  the catalog's block-level `.meta`. Contract sentence in REPORT.md; tests:
  five records render as three lines and one expansion between the third
  and fourth record, the destination named once.
- A "Where this service connects" section with more than five destinations
  is compacted to five rows and an "All N" disclosure. The compaction removed
  every `ul.operation-catalog` inside the section, which included the record
  list nested inside each destination row, so every "Records · N" disclosure
  opened to nothing (the owner's service: "Kubernetes API server · 156"). Only
  the group's own lists and disclosure move now; a destination row keeps its
  records, and the first-screen copy of the row inherits them. Reproduced on
  the real script with a synthetic six-destination section in the browser
  (six disclosures, zero record lists before; six lists and twelve records
  after). Node regression: `TestCatalogDisclosureKeepsDestinationRecords`
  fails on the previous script. Reports with at most five destinations were
  never affected, which is why the four small-repository reports checked
  earlier today did not show it.
- A symbol row whose `activation` cell is missing or null settles as
  `unassessed` instead of refusing the row: python-dotenv's ordinary run
  lost four rows to `missing "activation" cell` while their `key_symbol`
  and `outbound` were valid. Only a choice whose options already contain a
  no-decision value declares such a default.
- A run stops at the first provider refusal by credentials or balance
  (HTTP 401, 402, 403) with the cause in the console and as the final error,
  instead of walking every remaining stage with a refused window each:
  Freqtrade `20260911-070651` ran into a 402 at translation, and a later
  service run failed every request for its first minutes. Accepted work
  stays cached for the rerun.
- The finder script referenced the removed second search field after the
  Learn/Work switch was taken out (`proxy is not defined` at load), which
  stopped every script bundled after it: reading navigation, glossary and
  question links. The references are gone; a JavaScript smoke check of the
  bundle is part of the report tests now.
- A single-component report gets the same first-screen product card as a
  repository map would give it: name, purpose, counts and the inputs,
  commands and communication entrance, placed before the question list. The
  owner's one-service report showed the summary, then all questions, and
  the three inventory columns only on the component page.
- Glossary reduction windows list at most six sample observations per
  variant beside the real `count`; the catalog entry keeps every source.
  Freqtrade `20260911-053911` sent 137 reduction windows of 1.9–3.1 MB,
  114 million input tokens for 90 thousand output tokens, because every
  anchor of a common term rode into every window; the fourth run paid
  102 million more for the same stage. Reduction state version 6.
- Orientation walks a packing ladder after a size or context refusal, local
  or remote: 40 members per group with 6+6 observations, then 20 with 3+3,
  then 12 without observations (Freqtrade saved input: 1.74 → 1.18 → 0.92 MB). A 524,288-token
  provider with the 128,000 output reservation holds about 1.2 MB, so the
  owner's reports had no written summary; the console said `ready`. It now
  says `unavailable` with the refusal when no rung fits.
- A third context-refusal wording is recognised: "Requested token count
  exceeds the model's maximum context length of L tokens. You requested a
  total of R tokens: I tokens from the input messages and O tokens for the
  completion" (a gateway in front of DeepSeek, `code` "400"). It carries the
  same four counts as DeepSeek's own wording and is parsed with the same
  consistency checks; before this it was a generic failure and the window
  was lost instead of partitioned.
- `REPOMAP_LLM_CONTEXT_TOKENS` / `DEEPSEEK_CONTEXT_TOKENS` declare the
  provider's context window. A request whose estimated prompt tokens (bytes
  ÷ 3, below DeepSeek's measured 3.5) plus the output reservation exceed it
  is refused at preparation with the usual context refusal and partitioned
  by its stage, before any transport attempt; run 20260911-053911 spent
  36 s per remote refusal, 52 times at the answer stage alone. Unset keeps
  the check remote. No request bytes or cache keys change.
- The console now says when a refused request is partitioned: after the
  WARN for a provider resource refusal, the answer, question, learn and
  glossary stages
  print a `partitioned` state with the number of questions or evidence groups
  and the number of smaller requests that follow. The owner could not tell a
  split from a lost window; `tables.md` and `rejected.jsonl` recorded it, the
  console did not.
- A Go module library whose only consumer is one standalone executable is
  folded into that executable. An owner's service with `cmd/app` and
  `internal/app` (plus exported packages that make the module a library
  candidate) showed a "shared code" component holding every handler, worker
  and outbound call, while the product page listed only the routes: file
  ownership goes to the deepest root, and the library's root `.` owned all
  of `internal/`. The executable now claims the module root; the model's
  `shared_code` decision stays journaled with the fold beside it.
- Destination groups on the first screen carry their records disclosure
  (ids stripped from the copy), so a group expands there too; a group of
  several records shows no single record's purpose as its lead. Both were
  reported by the owner on the first grouped report.
- The provider refusal "The input (N tokens) is longer than the model's
  context length (M tokens)" is recognised as a context refusal: an
  OpenAI-compatible server in front of a 524,288-token DeepSeek worded it
  that way, and without recognition the window would be lost instead of
  split.
- The Learn/Work switch is removed from the report toolbar: the owner could
  not tell the two entrances apart, and the switch only hid the intro and
  question list or moved the search field. Summary, questions, map and the
  toolbar search are always present; the per-component entrance keeps its
  question links and the component search button; an old `?mode=` link is
  ignored. Print and JavaScript slice tests re-anchored.
- Go handlers registered as method values or method expressions
  (`mux.HandleFunc("/items", s.handleList)`, `http.HandlerFunc(s.handleCreate)`)
  now resolve to their methods: the callable resolver follows a synthetic
  bound-method wrapper or thunk to the method it delegates to. Before, the
  route fact had no handler and the handler no route, so a page listed
  `GET /test` under registrations and a separately labelled operation for
  the same handler. A wrapper around an interface method keeps resolving to
  nothing static; a middleware's returned closure still resolves to that
  closure.
- A sequence cell citing only refs outside its row's options, or nothing
  at all, is an empty selection and no longer refuses the row: an owner's
  symbol window answered `outbound: "c9 c12"` where those refs were context
  calls, not options, and lost its key-symbol and activation decisions with
  the row. Commas now separate refs like spaces. Exceeding the limit still
  refuses the cell.
- The "Where this service connects" catalogue groups communication records by
  destination: one row per destination text (case-insensitive; native label
  or kind when the model named none) with the record count, the shared kind,
  basis and address, and the first sentence of one purpose; every record keeps
  its own row beneath the group. An owner's Go service showed ten
  "Kubernetes API server" paragraphs and twenty "Postgres" ones as separate
  rows. Rendering only: no saved data changes; the first screen and the
  section count show groups.
- An operation label the model left empty or null takes the first sentence of
  the row's own description (`name_from: description`). An owner's Go service
  on another DeepSeek provider received rows with every schema key present and
  `"name": null` beside a complete description, and each such row was refused
  as `cell "name" is empty`; the official API writes the label. An empty
  description still refuses the row. Decoder rule only: requests and memo
  state are unchanged.
- Orientation is bounded by construction again: at most 40 members per group
  (`member_count` keeps the real size), 6 calls and 6 callers per listed
  member's evidence with omitted counts. After `956e5992` removed the
  12-member cap and added evidence for every listed member, the Freqtrade
  orientation request grew from 1.3 MB (accepted at 330,422 tokens on
  20260910-144751) to 22.9 MB, and the provider's context refusal failed
  publication of a 35-minute run (20260911-045158). Bounded on the same saved
  input: 1.82 MB (2,550 listed members, 258 evidence entries, largest 16.6 KB).
  A provider or preparation refusal by size or context now leaves an empty
  orientation with the refusal in `rejected.jsonl`; the report is published.
- Native boundary scopes stay within the targets holding the file, and the
  target projection omits a boundary whose file the target does not hold.
  After `956e5992` widened native scopes to every observing index view,
  Freqtrade `20260911-040254` (one route fact seen from seven views, file held
  by one product) and an owner's Go service failed at publication with
  `atlas: boundary … names unknown box`. Regression tests in `places`,
  `reading`.
- A complete keyless response is read in asked order: the model drops the
  `key` it was told to copy in small answer windows (seven one-question
  windows of `20260911-040254`, then two- and three-question windows of
  `20260911-045158`, ten of 242 windows), and each such window lost its
  answers as `response row has no string key`. One row per asked row and no
  key anywhere leaves the asked order as the only reading; a partial or
  partly keyed response is still refused row by row.
- A closed choice whose only option is `unknown` accepts any answer as
  `unknown`. Freqtrade `20260911-040254` refused 27 outgoing boundary rows
  (7 whole windows) because the model copied the observed path into
  `address` where `address_options` was `["unknown"]`; their kind, line and
  destination were lost with them. An offered `a*` ref still has to be chosen.
- Restoring an empty selected file set no longer fails a run: a Go repository
  whose only Python file is a test script (air) has a Python catalog but no
  required Python target. Both adapter restorers return nothing for an empty
  selection; a non-empty one still fails closed. Regression test in `run`.
- Glossary generation/reduction output allowance is 32,768 tokens instead of
  the shared 128,000. Measured legitimate windows: 1,276–15,278 output tokens
  (Syn, issue-bot, Watchtower); measured loops: two Watchtower windows at
  128,000/128,003 tokens, 318 s and 519 s, 15 of the run's 21 provider
  minutes. The existing resource-refusal split still halves a refused window.
  Saved-window probe before the change: `scratchpad/glossary-cap-probe`.

- Third ordinary series (binary `664e34c`): Syn 224.602 s, issue-bot 574.736 s,
  Watchtower 306.593 s; 30/20/27 questions, all available. Source/prompt and
  provenance receipts are in external `work/small-repo-audit-20260910/next-*`.
  Native and operation controls passed; answer acceptance still exposed the
  listener/worker distinction, a differing GitLab comment argument and a missing
  required template argument. Their current prompts now state those requirements;
  controlled full-window checks and ordinary follow-up remain pending.
- Glossary generation/reduction now use the existing exact-request resource
  refusal memo; current cached/replayed whole answers keep priority. Exhausted
  provider-local deadlines take optional refusal while actual run cancellation
  remains terminal. Focused terminology/LLM/translation tests and vet pass;
  ordinary warm acceptance remains pending. No timeout, output cap, cache type
  or old-journal migration was added. The observed issue-bot delay was a
  309.809 s, 128k-output repetition followed by complete 31/53-row children.
- The third Syn report exposed a display error: two native registrations plus
  an unmatched proxy interpretation were labelled three routes. Route summaries
  now count native registrations; the combined catalogue identifies its records
  and unmatched handlers separately. Existing rows/anchors are unchanged. Focused
  report tests and vet pass; saved rendering/browser verification follows the
  current frozen-binary series.
- Operation v19 states the positive task-and-activation requirement in the
  existing entry decision. Watchtower's actual v18 bytes already prohibited
  listener and dispatcher promotion, but two model rows still did so. The
  clarification adds no new evidence, judge or local role repair; its effect
  needs the next ordinary run.
- Watchtower listener/route projection: graph 14 / saved input 15 / atlas 7
  retain native target origins and the fixed `listen_address` kind. Exact
  paths/methods/columns no longer collapse into one fact. Shared-source context
  follows the compiler-located subject; each target restores its own FactID and
  ObjectID. Focused sealing, owner-context, method/path, fixed-fact and complete
  reading→GroupsIndex→catalogue tests pass. A local projection of compatible
  saved Watchtower native inputs retained its two route observations plus one
  listener, with both targets' exact route identities (0.50 s, no provider).
  Genuine sealed game adapters through current facts/places produced graph
  SHA-256 `ded06f4a626388e0a80db4b12d788d570057c2197802ef702a8e800d4995d19a`.
  This is local validation; existing model wrapper operations and a fresh
  ordinary Watchtower report remain separate acceptance work.
- Boundary v5 distinguishes `remote_client_instance` from an option for a
  later constructor. Two complete saved windows were prepared through the
  current owner and replayed once each: issue-bot retained NewClient and five
  dispatches, WithBaseURL became none (2.733 s); service retained OTLP New and
  HTTP Do, while the local wrapper became none (2.030 s). Current validation
  accepted 7/7 and 3/3 rows without rejection; full original row contexts were
  equal. This is a contract check, not ordinary report acceptance.
- Watchtower's glossary reducer repeated 87,780 source records in 4.73 MB and
  exceeded provider context twice. All 458 accepted variants survived, but the
  catalogue remained partially compared. Reducer v5 factors exact anchors and
  source sets within each independent request. The same full saved window is
  502,076 bytes; one supported replay passed in 15.503 s with 155,669 input and
  5,502 output tokens. Current validation accepted all 458 assignments into 412
  groups, retaining every original variant/source/origin. Full ordinary acceptance
  of the changed reducer remains separate from this saved-window check.
- Syn's actual operation request omitted native receiver/argument context and
  classified middleware as additional incoming work. Operation v18 retains its
  original own-call observations and separates same-request middleware from an
  independently activated endpoint/consumer. Actual cumulative Go contrasts,
  consuming request/decision tests, full reading/lines tests and scoped vet pass;
  corrected model decisions await the ordinary rerun.
- Operation v17 uses own `self`/`none`; fixed-boundary v4 no longer asks a model
  to decide the existence/kind of a native fact. Exact routes and accepted
  worker rows remain independent of captions/setup callers. Focused
  lines/reading tests and vet pass; ordinary classification needs reruns.
- Question-batch v3 validates relevance per selected anchor. Native calls retain
  receiver/source arguments, API and same-line sites in question and boundary
  evidence. Orientation includes native member evidence. Document ancestry is
  carried with author scope (graph 13 / reading input 14), not used as a hard
  file-role exclusion. Integrated ordinary answer quality remains pending.
- Native declared-interface calls retain their original unresolved dispatch
  observations in graph 13 / input 14. Source-ordered Python parameter types
  can supply declared method alternatives, with reassignment/unknown controls.
  Data query/table links are reversible and operation links require existing
  exact model or callable ownership; embedded literals gain no guessed owner.
  Ordinary source-chain quality remains under acceptance.
- Deferred prose source appendices were removed from provider messages.
  Prepared/existing accepted cache v3 retain compact local context once; current
  warm execution, entity memos, original-window question memos and replay keep
  original provenance. Seven consuming package suites/vet pass; race-enabled
  parallel/warm/replay regressions pass. Receipt:
  `work/small-repo-audit-20260910/local-prose-context-fix.md` in the external
  project work directory.
- Glossary generation/reduction now use the shared 128,000-token allowance.
  The saved Watchtower window that stopped at 8,000 completed with 8,903 output
  tokens in 28.803 s (21,040 input; finish=stop). Model/user evidence and p refs
  were unchanged. Of 216 term rows, 211 have exact occurrences; five unsupported
  optional names are discarded. Structure/closed refs/text are checked; original
  source provenance still requires ordinary acceptance.
- The saved issue-bot response accepts 21/21 question rows under per-anchor
  validation (previously 18/21), without a provider call. This is an offline
  decoder check, not a corrected ordinary report.
- The real Python16 Freqtrade native check retains `/trades` →
  `RPC._rpc_trade_history` → `Trade.get_trades_query` as alternatives, including
  the original Trade owner. This closes that observed native frontier, without
  claiming runtime execution or a completed model/report acceptance.
- Orientation preparation retains complete author claims and all validated group
  members with native evidence; the prepared provider envelope remains the actual
  limit. Fresh ordinary orientation quality remains pending.
- Short AGENTS/CURRENT entry pages now route to topical contracts. Full prior
  text is preserved in the [non-normative archive](../archive/2026-09-10/README.md).
  Required native fixtures, executable expectations, package bounds, ordinary
  runs, browser QA, warm/cache-clear checks and constitution supremacy remain.

Next: complete the combined build/check checkpoint, verify the corrected small
ordinary reports, then complete Freqtrade acceptance before full Airflow. Do not
mark those outcomes complete from probes or local tests.
