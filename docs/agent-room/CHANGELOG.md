# Implementation and acceptance journal

## 2026-09-30 — Options handed to a helper's parameter (skeptic's verdict on freqtrade's flags)

- **Why:** freqtrade's 124 flags are rows of `AVAILABLE_CLI_OPTIONS` that
  `_build_args(optionlist=ARGS_X, parser=P)` adds to the parser it is
  handed; none had `declared_on`, so all stood as tiles beside the
  subcommands, and READING said they "stay the program's".
- **Handed (groupindex `handed.go`, derived):** the calls a declaration
  makes on its own parameter, every call into it with the arguments a call
  made (`call_result` anchors, by parameter; a method through its class
  shifted) and the `keys` read of a list at that call, and the keyed
  tables' rows by key. `options()` gains a third rule (`handedOptions`):
  declared on a parameter, or a row looked up with handed keys, is an
  option of each input whose own call made the object handed; its tile is
  hidden only when every call is exact and followed and, for a row, the
  helper alone reads the table and every list has rows.
- **Fixtures:** Python `tool_cli.py` `add_common` (`--quiet`: init's and
  status's, no tile) and `add_output` (`--json`: init's, stays a tile, the
  program's parser too); `dispatch.py` `build_serve` (OPTIONS rows under
  serve, tiles kept: `build_subcommands` hands parameters). Go
  `addCommon(fs)` from runServe and runCheck: `-quiet` nested under both by
  the branch rule (the Go facts carry the same parameter/`call_result`
  joint). JS, Clojure, C: recorded in READING, nothing fabricated.
- **freqtrade (saved run 234529, rendered):** 164 option rows under 27
  subcommands (trade: `--db-url`, `--dry-run`, `--dry-run-wallet`,
  `--fee`, `--sd-notify`); all 124 row tiles kept (7 subcommand calls hand
  row-less lists, `ARGS_MAIN` the program's parser, two groups via
  `parents=`). Left for the owner: rows through `*X` spreads, elements of
  list-literal arguments, argparse `parents=`.
- **Tests:** `TestASubcommandsOptionsAreNestedUnderIt` (Go, Python),
  `TestAFlagDeclaredOnAHandedParserIsAnOptionOfTheSubcommandMakingIt`,
  `TestARowLookedUpWithHandedKeysIsAnOptionOfTheSubcommandMakingTheParser`;
  fixture expectations in the inputs, table-reads and word-given tests.

## 2026-09-30 — The canvas batch of the second human-eye review: packed programs, trunks, one note for unread targets, folded Outside cards

- **Why:** the reviewer's canvas items 1–7 (crops in the scratchpad's
  `eye2/`), then the column agent's `(inline)` tiles and the data agent's
  kinds and spellings.
- **Packed programs (1, 2; REPORT § whole-map fit):** a component whose
  arrangements all leave its smallest card under 64 px where it is entered
  whole packs its cards in a grid toward the canvas proportion; its arrows
  run orthogonally in the gutters, every arrow at one side of a card meets
  it at one point and runs in that card's one lane there (a lone arrow at a
  side takes the lane it came by), gutters wide enough for lanes 5 px apart
  where it is entered, the frame's 32 padding outside the outer lanes.
  Layered arrangements merge arrows per side (ELK `mergeEdges`).
  freqtrade's program entered (run 221230): smallest card 33×16 → 133×67
  px, occupancy .07 → .38, no arrow over a card; on run 234529 its closed
  areas' titles read at 12–16 px. Cards win clicks in all four programs (0
  of 96 sampled points hit an arrow).
- **Titles (3, 5, 6):** a path breaks only after "/", a segment only when it
  alone is wider than the line, never before its extension; a program's
  title grows only within its header band; a closed area's description
  takes every whole line (counting the card's 8 px gap) and says itself
  whole on hover when cut. A `breakWord` loop that could split a lone "."
  hung litestream's page before it opened.
- **Unread targets (4; page_system_map.go):** one "Not analysed" note
  naming them, sized by its words with half again their room; litestream's
  reads at 7.7/7.1 px at the whole map beside program headings at 8 px,
  where two pale cards had read at 4.4 px.
- **Smaller (7):** inputs list the column's kinds (Scheduled tasks apart
  from Background work; Queue consumers, Extension points, Kind not
  established), so redis-server's Inputs shows its four; `consumer` wears
  the inbox mark (the key had been `queue_consumer`), `extension` the plug,
  an unestablished kind a neutral dot, and the env key mark its paths (its
  download had been a CDN error page). An arrow's card into an Outside frame
  lists destinations under the parts calling them: litestream's 89 rows → 10
  destinations. Input tiles name inline callables "anonymous function in
  X", as the column does. An input's other spellings read "also written
  storage-class" in its reading (`data-spellings`). A loose part's
  magnifier lands with its head in sight again (the drawn-in-the-middle
  offset is gone).
- **Geometry lint (visual/geometry.mjs):** two legs are one trunk, not a
  coincidence, when both routes go on from them along one path to a box
  they share. On the newest saved runs (redis 234508, litestream 234245,
  freqtrade 234529, othello 232515), HEAD 805dac34 → this change: redis
  6 → 4 (the 4 left are Inputs part groups at 4–7 px when redis-server is
  entered), litestream 64 → 1, freqtrade 62 → 0, othello 1 → 1 (the 2 left
  are part declaration rows at the part level).
- **Tests:** `npm test` 131/131, visual suite 55/55 (the dense fixture's
  title floor is scoped: forty-two packed cards open at about 6 px, 4 px
  before, where only an open Inputs group had met the floor), Go report
  tests and vet.

## 2026-09-30 — The data batch of the second human-eye review: held code, test directories, self-launches, unread and template addresses, prose refs, entry names

- **Why:** the reviewer's items 1–8 (crops in the scratchpad's `eye2/`)
  and the column agent's native `X$1 calls Y` labels. Item 6a (spellings
  of one value) is the entry above (0e4536a3).
- **Shared code (1, f467d538; REPORT):** a program holds the files its map
  claims (its parts' declarations, its off-map files); a shared
  declaration's holders, its callers elsewhere and a call side's own path
  count only held declarations. freqtrade's `FreqtradeBot.process` had
  listed `Worker._process_running` under its own part and under five script
  programs sharing the project index. A declaration calling itself is no
  caller elsewhere.
- **Tests on the product map (2, 3ec10290; PYTHON § Test sources):** the
  topmost directory below a resolved pytest table holding a selected test
  module, with no declared module or program launch file under it, in a
  project declaring its packages, is test code whole. freqtrade's test
  sources went 106 → 140; the "Strategy test fixtures" / "Test fixtures"
  parts and the "Tests" area are gone (33 parts, 8 areas). Clojure's test
  alias `:extra-paths` and Playwright's `testDir` are the equivalents.
- **othello (3, 4; 269d3a62, c1b3abcf):** the desktop Inputs read
  `key-pressed`, `mouse-moved`, `mouse-pressed` (378a0679, now run) and the
  `:draw` hand-over `othello.ui.draw/draw-state`, no longer
  `othello.ui.sketch/start!`: an entry with no chosen word is named by its
  handler before the boundary's Caller, which is the enclosing function
  when the handler lives in another file. `move` does call itself (its
  two-argument form calls its three-argument form through the var); no
  declaration is listed among its own callers or callees, and the app
  program's second listing was the shared-code join.
- **Prose refs (5, 94d2130b; READING § Orientation) — this run's one
  question change:** the overview prompt says refs go only in the ref
  fields; the decoder refuses a summary or role label writing an advertised
  ref and drops a purpose or note writing one, no rewrite. freqtrade's
  saved overview request (`bce326d6…`), 3 draws each: the former prompt
  wrote refs in the summary in 2 of 3 ("main program (t1)", "(t2-t6)"),
  the new one in 0 of 3 (every draw 7 roles, 7 steps). The run's summary
  has no ref.
- **Kind titles (6c, ead4fbe1; REPORT):** "Inputs" as a kind was
  `extension` (litestream's `sqlite3vfs.RegisterVFS`, the Python package's
  `Extension(...)`), missing from the column's title map; every kind
  GroupsIndex gives is titled, `entry` reads "Kind not established".
- **Self-launch (7, 28fcda00; REPORT):** litestream's MCP server runs
  `litestream`, its own program (executables `litestream` joined; the rule
  kept a program starting itself outside). Its arrow now goes into the part
  holding the program's seeds; no Outside chip named litestream.
- **Addresses (8; bd60212f, b885d644, 9395235d; READING, GO):** a walk
  ending at an unknown value is `Unread` ("Address not established from
  code": Redis's `(struct sockaddr*)&sa`). A Go function's result leaves
  out returns handing zero values beside a non-nil error (litestream's
  `expand`'s `return "", err` had made the WAL `-wal`). A template writes
  an unresolved part as the code wrote it and is one file: `{db.path}-wal`.
- **Labels (269d3a62; READING):** connections name an inline callable by
  its holder (`FindSQLiteDatabases (inline) calls IsSQLiteDatabase`); 21 of
  litestream's 1340 connections had carried `$N`, now 0.
- **Checks:** `TestACallerIsListedOnlyUnderTheProgramsHoldingIt`,
  `TestATestDirectoryBesideTheDeclaredPackagesIsTestCode`,
  `TestCumulativePytestMetadataPreservesAllIndexedDeclarations` (tests/
  whole), `TestAProseValueWritingARefIsRefusedAtItsCell`,
  `TestAStartedProgramThisRepositoryBuildsIsThatProgram`,
  `TestAWalkEndingAtAnUnreadValueEstablishesNoAddress`,
  `TestAnUnreadAddressIsNotEstablishedFromCode`, `assertGoSourceValues`
  (failing helper), `TestEveryLanguageKeepsTheFilesItsCodeReaches`
  (`{store.path}-journal`), `TestAnUnnamedEntryIsNamedByItsHandlerNotItsRegistrar`,
  `TestAConnectionNamesAnInlineCallableByItsHolder`,
  `TestADeclarationCallingItselfIsNoInternalConnection`; `make test` and
  `make vet` (package parallelism 2) pass.
- **Acceptance** (ordinary binary from 16415332 plus the display agents'
  uncommitted edits, default cache, `--no-serve --no-open`): othello
  `20260929-234241` exit 0, 4 s; litestream `234245` exit 0, 82 s (live:
  atlas_api 1, keys 2, role_assign 1, role_gate 3, symbols 2, glossary 11,
  orientation 2); Redis `234508` exit 0, 21 s (glossary 3, orientation 1);
  freqtrade `234529` exit 0, 247 s (api 1, areas 7, boundaries 2, core 2,
  role_helper 1, glossary 15, orientation 2). litestream: `storageClass`
  carries `storage-class` as its alias, the WAL is `{db.path}-wal`, no
  Outside tile named litestream (the MCP part's arrow goes into the entry
  part). Redis: `b121`'s use is unread and the page says so once.
- **Open:** freqtrade's `--verbose`, `--user-data-dir`, `--no-color` are
  not global: its main parser takes only `--version` (`ARGS_MAIN`); they
  are `ARGS_COMMON`, added to `_common_parser`, which 19 subcommands take
  as `parents=[...]`, and every flag is a row of `AVAILABLE_CLI_OPTIONS`
  read by `_build_args(optionlist=ARGS_X, parser=P)`. Nesting them needs
  two code facts not made yet (a key list handed with a parser to the
  table's reader; a parser handed as a parent to an input's own call). The
  column shows no spelling aliases yet (`Operation.Aliases`), and the
  canvas's input groups (web/cards.mjs, kind-icons.mjs) still lack
  `extension`/`consumer` (its icon key is `queue_consumer`). Python has no
  `same_value_as` (PYTHON).

## 2026-09-30 — Spellings of one value are one input with aliases (reviewer's item 6a)

- **Why:** litestream reads a replica URL's options in two spellings,
  `query.Get("storageClass")` / `"storage-class"` in an if/else-if chain
  (s3/replica_client.go:205, cmd/litestream/main.go:1520) and
  `"forcePathStyle"` / `"force-path-style"` in one `||`
  (s3/replica_client.go:217); each call answered `setting` was its own
  tile.
- **Change:** ProgramIndex 24 records on a call pattern `same_value_as`,
  the earlier call of the same callee (same declaration) written the same
  but for its string literals, as another operand of one `||` (JS/TS `??`,
  Clojure `or`) or the header of another arm of one if/else-if chain whose
  headers and bodies are written alike; `&&` is none. Go
  (`surfacediscovery/same_value_calls.go`), C (`cproject/spellings.go`),
  JS/TS (`helper.mjs`) and Clojure (`or` forms) record it; Python is
  missing (PYTHON). Places carries it on `SymbolCall.SameValueAs` (stripped
  from provider evidence); the reading makes a call-made entry whose call
  reads the same value as another entry of the same kind in the same
  declaration its other spelling (`spellings.go`, `atlas.Boundary.AliasOf`),
  and GroupsIndex folds it into the first spelling's operation
  (`Operation.Aliases`: name, site, call as written). No new question;
  each call keeps its own answer. The python-tutorial-game indexes are
  resealed (version and seal only).
- **Evidence:** a no-model run of cmd/litestream records 36 facts,
  among them `storage-class` (twice), `part-size`, the four `sse-*`
  spellings, `force-path-style`, `LITESTREAM_ACCESS_KEY_ID` and
  `LITESTREAM_SECRET_ACCESS_KEY`, and conditions listing words through
  calls (`strings.HasPrefix(host, "10.") || …`). The fold itself needs an
  online run (the `enters` answers); not run here.
- **Open:** the column shows no alias yet (the data is on
  `Operation.Aliases`); a condition testing two different values with `||`
  (`q.Get("user") == "" || q.Get("password") == ""`) reads as one value if
  both are answered the same kind.
- **Checks:** `TestSameValueAsNamesTheEarlierCallsPattern`,
  `TestEveryLanguageKeepsTheSpellingsOfOneValue` (Go, JS/TS, Clojure, C),
  `TestGoSpellingsOfOneSettingAreOneInput`, the kvd preset's
  `dbfilename`/`dbfile`; `make test` and `make vet` (package parallelism 2)
  pass.

## 2026-09-30 — The column, second human-eye review: whole names, short Outside readings, one kind heading, no run-on start line

- **Why:** the reviewer's column findings: names wrapped mid-identifier
  (`FreqtradeBot.process_open_trade / _positions`); freqtrade's Outside
  reading was 59,592 px of opened per-call records; an area's Connections
  opened on a test fixture; litestream's Inputs said "Incoming requests" once
  per declaring function; redis-cli's start line ran on; "(inline)" and
  `Run$1` (495 times in litestream's page, in address sources) read as jargon.
- **Change:** `rmDotBreaks` breaks only after `.`/`/` and at spaces, a piece
  over 24 characters ends in "…" with the whole name on hover, and every list
  of names in the column goes through it (area keys, catalogues, input paths,
  settings, program labels). An Outside frame lists its destinations and one
  joined "Called from" (`rmMergeReached`); a destination adds its calls under
  one closed "Its calls"; the component page's catalogue is no longer copied
  in. Outbound address steps are named by `subjectDisplay` (inline names);
  the column reads "anonymous function in X", drops the chain's numbering and
  the source count. Parts written only in test sources carry `data-test`;
  their connection ends stand last under a closed "Tests" (`apart` in
  `mountConnections`). The Inputs reading names each kind once, a
  sentence-like input name in prose type. The start list's reaches stand one
  to a line and go when the entry's calls stand open.
- **Shape diff** (`<scratchpad>/pageshape`, HEAD vs this pass, the saved
  redis 203001, litestream 205940, freqtrade 221230 and othello 181127 runs):
  names broken inside a piece 0 everywhere (freqtrade Outside 173, litestream
  212 before); Outside readings freqtrade 102.6 → 1 screen, litestream 610.1 →
  1, their digits 109 and 120 → 0; repeated Inputs headings litestream 20 → 0,
  freqtrade 8 → 0, redis-cli 2 → 0; no page errors.
- **Open:** freqtrade's test fixtures (`tests/strategy/strats/…`) are not in
  its `TestSources`, so their ends do not fold yet (the data agent's check);
  canvas card labels still carry GroupsIndex's native `X$1 calls Y` (8 in
  litestream's page data).
- **Checks:** `TestAColumnNameBreaksOnlyAfterItsDotsAndSlashes`,
  `TestAnInputsReadingNamesEachKindOnce`,
  `TestADestinationsCallersAreJoinedByPart`, `TestAFrameOfTestPartsIsTestOnly`,
  `TestAPartOfTestSourcesIsTestOnly`; `make test`, `make vet` (package
  parallelism 2), `make ui-test` and `make ui-visual-test` (55 passed, 5
  skipped) on 94d2130b plus this pass alone, the report, run and ui tests
  again on ead4fbe1 plus it (whose own reaching-inputs test had failed: it
  now pieces in the kind titles). Code b08daa63.

## 2026-09-30 — Calls sent through one engine are one destination; possible callers; script directories; what a Python program is built from

- **Why:** the open items of ed1a04d9: freqtrade's database named twice
  ("Freqtrade database" 39 rows, "Database" 24, each row its own
  destination asked per window); `func.count(...).label(...)` in `select`
  still a row; the webhook with no callers; build_helpers' two CI scripts
  falling to `freqtrade`; `freqtrade` "Built from" 373 files. The design
  went past a skeptic (read-only agent), whose changes were taken: an
  object's walk stops at a call giving words, takes no config object's base,
  counts only statements handed whole, and moves only the key and the
  destination's ends, never a row's address; closures reading a rebound
  name, test sources' stores and a class using its own same-named field
  handled; a file only a script's root covers keeps its programs.
- **Reading** (READING § One exchange, § One destination, one name):
  `handedOnExchanges` hands a part on through a call with no `talks`
  answer, not through another kind. A row's destination key and ends are
  where its exchange ends (`boundaryState.through`,
  `reading/destination_objects.go`): a call on an exchange's result ends
  where its first call does; a call deciding no argument ends where the
  object its statement is sent through was made (`DestinationReader.Exchange`:
  senders by `sendersOf`, objects by `object`, to `create_engine(db_url)`);
  a field of an object a decided call made goes on through that call
  (`Trade.session.bind`). `DestinationChoices.Talks` carries the talks
  answers. A field's stores are read only when its one receiver path found
  nothing; callers through alternatives are not receiver paths.
- **Python** (PYTHON § Source values of rebound names, class attributes and
  entered objects): a class attribute stored once through its class's name
  (`Trade.session = scoped_session(...)`, `Order.session = Trade.session`)
  reads that call's result; `with X as name` binds an `entered` value
  (a new source value kind, `sourcevalue.Validate`); a straight-line rebinding, or one in an
  if arm, stays known and an if statement joins its arms' values as
  `alternatives`; except/match/def/class rebindings clear a name; a list
  field closed over appended constructions makes a call on its element
  `alternatives` of the classes' methods (dispatch `interface`). The parser
  request marks test sources. Fixtures: `destinations.py`'s ledger,
  `inherited_clients.py`'s `Notifier`/`OpenNotifier`.
- **Reached from** (GroupsIndex `OutboundCaller.Possible`, reading
  `reachedFrom`): a declaration no exact call reaches is reached through the
  alternatives it stands among, every caller past that step possible; the
  reading column marks it (`pageReadingEnd.Possible`).
- **Claims** (DISCOVERY): the deepest root covering a file decides it; a
  file in a script's directory no script imports is no program's when a
  shallower root would take it, unless that program's entry imports it
  (ProgramIndex `ImportedFilesFrom`). **Built from** (REPORT): a Python
  program declaring packages is built from its entry files' imports and
  those packages' files.
- **Checks:** `TestOneExchangeWithASystemIsOneBoundary` (unanswered chain,
  another kind), `TestRowsSentThroughOneEngineReachWhatTheEngineReaches`,
  `TestACallOnAnExchangesResultEndsWhereTheExchangeEnds`,
  `TestCumulativePythonStatementsSentThroughObjectsEndAtTheirEngine`,
  `TestCumulativePythonSelfCallsAndFieldsFollowTheBaseChain` (alternatives,
  dispatch), `TestCumulativePythonSourceValuesRetainSharedHelperAndCapturedOwner`
  (a rebound name reads its replacement: the test had pinned unknown),
  `TestOutboundCallIsReachedFromTheFirstCallersOutsideItsPart`,
  `TestAnOutgoingCallIsNamedWithWhereItsProgramsReachItFrom`,
  `TestAFileInAScriptsDirectoryNoOneImportsIsNoProgramsFile`,
  `TestAScriptProgramIsItsFileAndWhatItImports` (a Python program's Built
  from); `make test`, `make vet` (package parallelism 2) pass.
- **Acceptance** (ordinary binary, default cache, `--no-serve --no-open`):
  freqtrade `20260929-221230` exit 0, 295 s; live calls atlas_api 11,
  areas 9, boundaries 1, core 2, describe 9, keys 21, role_assign 2,
  role_gate 4, symbols 5, systems 1, zones 1, glossary 29, orientation 2
  (the changed graph: Python alternatives and joined values, the dropped CI
  files). The database is one destination of 61 rows, "Database", asked
  once; the webhook's 3 rows "Webhook", reached from 11 FreqtradeBot
  methods (`notify_status`, `startup`, `process`, `_notify_enter`,
  `_notify_exit`, …), each possible, through `RPCManager.send_msg`'s
  `mod.send_msg` (now `alternatives` of Telegram, Discord, Webhook and
  ApiServer); `build_helpers/pre_commit_update.py` and
  `binance_update_lev_tiers.py` are no program's; `freqtrade` is built from
  332 files (its package), not 373. The interim run `220126` (before the if
  join, the not_in and CTE passes and the receiver rule) had kept 4 rows
  alone ("Freqtrade database": `get_trades_query`'s two selects, the
  not_in subquery, the CTE) and read the webhook's field stores, named
  "Discord". Rendered to the scratchpad's `redis-r2/run/latest-freqtrade.html`.
- **Open:** a statement reaching its session only through a loop, try or
  with rebinding stays its own destination; JS/TS reassigned `let`, static
  class attributes and `using` values have no equivalent (PYTHON).

## 2026-09-30 — Room for arrows through the fit, a program frame that hugs its areas, one "where am I", input-kind marks in the column

- **Why:** the lead's decisions on the canvas report of the morning: ship the
  wider gaps, fix the program zoom (an empty band under freqtrade's areas, one
  huge area card beside 5-pixel ones), the same gap rule inside, the column
  header items (a)–(d), and the kind marks beside the canvas's.
- **Change:** the whole map's arrow room is laid out in screen pixels for
  the camera each fit correction plans (three fifths of the interiors'
  spacing, shrinking only as that camera's square root, never below half);
  up to four corrections. A component keeps every arrangement of its areas
  (both directions, with and without unzipping) and grows whole in the one
  nearest its grown box. No closed card of a component is smaller than
  9/20 of its largest area (at most 1.5× its own size); arrows between areas
  are spaced in the unit of the areas, and inside areas 16/28 apart. A
  closed card's title reads 12px where its program is entered; an area and
  the magnifier enter no smaller than their own parts read. Path-like titles
  break at their separators. The column heads a part "Part", its frame link
  flows inline, an empty actions row draws no rule; Inputs and Outside are
  named with their program; the home says "System map" once (the canvas's
  location row is kept only for assistive technology and errors); the
  reading column, the programs list and the key carry the input-kind marks.
- **Check** (saved runs of 2026-09-29, 1054×710): geometry findings before →
  after, Redis 28 → 5, litestream 248 → 11, freqtrade 43 → 12, othello 21 → 3
  (the small-text rule now skips the whole map); no arrow runs along a frame
  or on another arrow any more; what is left is area titles at 7–8px at a
  program entered whole and three clipped descriptions. Nearest arrow to a
  frame it does not join 0.4/1.7/1.2 → 10/7.8/7.8px (Redis, litestream,
  freqtrade); smallest whole-map summary drawn at .94/.61/.55 → .75/.46/.47
  of its size, the cost of that room; freqtrade's program frame empty band
  0.82 → 0.08. Three fixture tests pinning whole-map text scale were relaxed
  to half their reserve, one reveal floor to 10.5px, the location-row wheel
  test deleted.

## 2026-09-30 — A table's response example shows no real row key

- **Why:** F3's open item (03d4542a): windows of near-identical rows came
  back holding only the row whose key the one-row response example showed
  (`b16`, `b22`, `b147`); Redis's `g6` window and freqtrade's `b1655` too.
  Code ac168bec.
- **Change** (EXECUTION § Results and prompt ownership): the example's key
  is the placeholder `<each row's key>` (`table.ExampleKey`), whatever the
  window holds. No row may carry it (a preparation error), and a response
  row under it answers nothing ("response row copied the example's
  placeholder key"). Every table window's bytes change once; row memos
  (their identity has no example) keep their answers.
- **Measured** (saved request bodies, only the example's last line changed,
  the ordinary DeepSeek client, 6 draws unless noted; rows answered per
  draw): litestream 20260929-165546 w40 real key 1/11 in 3 of 3 draws,
  either placeholder 11/11 in 12 of 12; w43 real 1/7, 1/7, 7/7, either
  placeholder 7/7 in 12 of 12; w59 real 1/2 in 3 of 3, either placeholder
  2/2 in 12 of 12; freqtrade 191655 r2-w2 real 2/2 in 1 of 6,
  `<each row's key>` 6 of 6; Redis 201637 r3-w2 3/3 in all 12 draws either
  way. The bare `<row key>` cost an optional cell's "none": litestream's
  publish window (6 rows, two with no holder) came back whole in 3 of 12
  draws, the two rows left out, against 5 of 6 with the real key and 12 of
  12 with `<each row's key>`. One skeleton per row also answered every row
  in 6 of 6 but adds 50 to 100 bytes a row; not taken.
- **Checks:** `TestResponseExampleShowsAPlaceholderKeyNoRowCarries`,
  `TestResponseExampleListsKeyFirstThenColumnsInFillOrder`; `make test` and
  `make vet` (package parallelism 2) pass.
- **Acceptance** (ordinary binary from HEAD plus this change, default
  cache): litestream `20260929-205940` exit 0, 26 s, one live call
  (atlas_publish, the one unmemoized text-model table window; the rest are
  categorizer tables or row memos), 28 rejections, 0 "row was not
  answered" (the newest run before, `203012`: 0 live calls, 28, 0). The
  interim `<row key>` build's run `205505`: one live call, 30 rejections, 2
  "row was not answered" (publish's `pub3`, `pub4`, both "none").

## 2026-09-30 — The canvas: one Outside frame per program, arrows as their own handles, marks for input kinds, one geometry check

- **Why:** the owner on litestream's whole map ("кто придумывает так уродливо
  external вызовы расставлять?"): twelve destination frames in one row of
  every size, empty call tiles, a comb of arrows; then plaques floating
  beside their arrows, a card's heading drawn over its rows, card bodies
  with a zoom cursor, "Inputs / Commands" cut in half, a kind row above
  every input tile. No check had looked at geometry.
- **Change:** a program's destinations are one-size chips (two lines, the
  rest on hover) in one amber Outside frame joined to it by one arrow, the
  records naming none one muted "not established" chip last; no call tile
  is drawn and cross-program display groups are gone (`SystemMap`,
  `overview.mjs outsideChips`, `split-layout.mjs chipInterior`). Plaques are
  gone: an arrow's wide hit path opens its card and a click reads it, boxes
  standing over arrows (`zIndexMode manual`); two frames share one outer
  route; the outer spacing is three fifths of the interiors' at the .44
  camera, not a sixth, and a fit correction places the grown boxes over all
  eight candidates. An input tile has an Octicons kind mark instead of a
  kind row, an input without a handler groups with the part its one arrow
  goes into, a card body reads with the ordinary pointer, the Inputs summary
  is measured as drawn, Inputs/Outside frames are entered at their tiles'
  scale, a call card prints no counts and its headings are rows.
- **Check:** `visual/geometry.spec.mjs` (DEVELOPMENT) on the four 2026-09-29
  renders, findings before → after: Redis 263 → 46, litestream 916 → 290,
  freqtrade 694 → 112, othello 69 → 21; what is left is mostly whole-map text
  under 11px on crowded maps. Whole map at 1054×580 (saved runs): roots
  litestream 29 → 16, freqtrade 32 → 21; outer routes 31 → 10 and 41 → 22;
  nearest arrow to a frame it does not join Redis 2.2 → 2.5px, litestream
  0.9 → 3.2, freqtrade 0.3 → 1.2, summary scale .74/.25/.21 → .72/.22/.24.
  Room kept at screen size through the fit measured 11.2/5.0/2.8px at
  .66/.21/.23 but left the fixture's summaries at .84 and .70 of their
  size: not taken. Tests pinning plaques, display groups, kind rows and
  destination tiles were deleted; `arrow-handle.spec.mjs` and unit tests
  for chips, one route and taken-in groups were added.

## 2026-09-30 — Outside systems: an unnamed launch is no system, one exchange one boundary, a destination is one program's; script programs by their file

- **Why:** the human-eye review of the 2026-09-29 Redis, litestream and
  freqtrade reports (crops in the session scratchpad's `eye/`): a
  "Program not established" frame holding `run`, `CommandContext` and
  `Cmd.Start`, listed twice in Connections; outside systems filled with bare
  call names (`sum`, `filter`, `inspect`, `post`); redis-cli and
  redis-benchmark reading "→ Redis server · → redis-server" and the server's
  own "→ Redis server" (its master) like a self-loop; freqtrade's helper
  scripts titled `build_helpers.create_command_partials`, each "Built from
  373 files" and drawing its siblings' files; lowercase "the …" names
  beside Telegram; a program named "etc". Code 7901c777 (atlas),
  2ef3bd98 (report), ec131c4a (script programs).
- **Unnamed launch** (REPORT, READING § Programs started): a `runs_program`
  call no word names is no catalogue row, frame, tile or connection; it is
  listed under "What is missing" ("Programs it starts that the code does
  not name") and its call in its function's flow says "(starts a program
  the code does not name)" (`page_outbound.go` `unnamedLaunch`,
  `page_flow.go` `Launch`, `32-flow.js`). litestream: `-exec`'s
  `CommandContext` and `Cmd.Start` (two calls: Go records no field's stored
  value, so `c.cmd.Start()` is not tied to its launch) and
  etc/s3_mock.py's `subprocess.run`; freqtrade: `chown_user_directory`'s
  `check_output` and create_command_partials' `subprocess.run([...])`,
  whose program is written inside a list.
- **One exchange, one boundary** (READING): a call on a same-kind call's
  result, or handed whole to a same-kind call, is part of that exchange
  (`sameExchange`, generalizing the launch rule, and `handedOnExchanges`).
  freqtrade db rows 116 → 63; `sum`, `filter`, `label`, `get_table_names`
  are gone; `func.count(...).label(...)` inside `select` stays twice, since
  `func.count.label` has no talks answer.
- **A destination is one program's** (READING § One destination, one name):
  keys, walks and `reached_from` per program, one window per program and
  catalogue naming the program (`context.program`); walks never pass through
  test code (freqtrade's webhook calls, once `tests/` became freqtrade's
  own files with the library fold, walked only into
  `tests/rpc/test_rpc_webhook.py` and the page dropped them).
- **The one question change** (`prompts/destinations.md`, measured online):
  names are proper names, capital first letter, no article, a vendor's
  service by product name, any other system by its role for this program.
  Redis, one draw each: per-program items in one shared window → all
  "Redis Server"; own windows with a `Command server`/`Replication primary`
  example → server "Replication primary", litestream's control socket
  copied the example as "Command server"; final wording without named
  examples → server "Primary", clients "Redis Server" and "DNS Resolver"
  (one draw left two cli rows unanswered, "row was not answered", the open
  one-row response example of F3; the next run answered them). litestream
  "Litestream Daemon", "SFTP", "Litestream Heartbeat Server" (an earlier
  wording drew "Litestream replica" for the heartbeat); freqtrade "Webhook
  Endpoint", "Remote pairlist server", "External Message Producer", and the
  database still two names, "Freqtrade database" (39) and "Database" (24):
  each `select` row walks nothing and is its own destination, named per
  window (open).
- **Own programs** (REPORT): a destination one of whose records has an
  integration connection into another of the report's programs is that
  program: its records read "Reaches this repository's program" and draw
  into its component (`joinOwnPrograms`). redis-cli has no outside frame
  now; redis-benchmark keeps "DNS Resolver".
- **Script programs** (DISCOVERY, REPORT; ProgramTarget `ScriptFile`,
  ProgramIndex `ImportedFiles`): one source file, no name from the build.
  Titled by the file (`build_helpers/create_command_partials.py`,
  `etc/s3_mock.py`, `packages/python/scripts/rename_wheel.py`, its one part
  too); a program the build names by that name where its directory ends
  otherwise (`freqtrade-client`; C programs now read `redis-cli`, not
  `redis-cli (executable)`). "Built from" its file and its import closure
  (create_command_partials 235, extract_config_json_schema 20, was 373
  each). In the atlas a script holds its file and, in its directory, only
  what it imports (`places.TargetInput.Script`): each build_helpers and
  scripts program is one part; the two guardless CI scripts fall to
  `freqtrade` through the folded root distribution. etc/s3_mock.py is a
  guard script CI runs around `go test`, placed `tool`; no test rule covers
  it and no path rule was added.
- **Checks:** `TestOneExchangeWithASystemIsOneBoundary`,
  `TestDestinationKeyIsWhereTheWalksEnd` (a shared row per program),
  `TestDestinationWalksPassNoTestCaller`,
  `TestOutboundProgramsAreOneDestinationOnlyByTheirWord` (unnamed launches
  in the gaps), `TestADestinationThatIsOneOfTheRepositorysProgramsJoinsIt`,
  `TestAScriptProgramIsItsFileAndWhatItImports`, the facet test's claims
  (bench.py its own, cli.py the console script's, rest.py shared); `make
  test` and `make vet` (package parallelism 2) pass.
- **Acceptance** (the ordinary binary, default cache, `--no-serve
  --no-open`): Redis `20260929-203001` exit 0, 11 s; litestream
  `20260929-203012` exit 0, 15 s, no live call; freqtrade `20260929-203027`
  exit 0, 217 s, one live call (atlas_boundaries). The measurement runs
  before them re-asked only atlas_boundaries (the destinations question)
  and the glossary on Redis and litestream; freqtrade's first run
  (`201814`) also re-asked areas, core, describe, keys, zones, joints and
  orientation, from the moved files (the script claims, and the library
  fold 4b51aa1f/d6a1afa9 committed since the reviewed run). Rendered to the
  scratchpad's `redis-r2/run/latest-{redis,litestream,freqtrade}.html`: no
  "Program not established" frame; outside frames Redis "Primary"
  (server), "DNS Resolver" (benchmark), none for redis-cli.
- **Open:** tiles inside an outside system are still one per outside
  symbol (the canvas lane: callers by part, ReachedFrom, is the data);
  freqtrade's webhook has no ReachedFrom, its callers reaching it only
  through alternatives-resolved calls; freqtrade's database named per
  window; `freqtrade`'s own "Built from" still lists every file of the
  project's shared index (373).

## 2026-09-30 — Inline names everywhere, an input's options, a part's namespace said once

- **Why:** the input-naming agent's handover (336d24d2): GroupsIndex now
  names a callable written inline (`ObjectFacts.Inline`: the function it
  only wraps, else "ReplicateCommand.Run (inline)") and nests a
  subcommand's flags as `inputPath.options`, neither yet shown; othello's
  tiles repeated `othello.ui.host/` on every name.
- **Change:** `subjectDisplay` reads `Inline`, so no `$N` is printed in a
  flow, a chain, a caller list or on a tile; the column's own closure naming
  ("in ReplicateCommand.Run", `closureHome`) goes, a new element replacing
  the old; a compiler-numbered callable still stands on no tile and in no
  part's members (`inline`, by its native name). An input's reading lists
  "Its options" and the words its handler checks one to a line, each a link
  to its source. A part whose tiles share one namespace drops it from their
  names (`sharedNamespace`), the whole name kept on the tile's hover
  (`Full`) and in the reading.

## 2026-09-29 — The reading column: five sections, no digits, one name to a line

- **Why:** the owner's critic (round 2): a component reading ran to ten
  screens in nine sections with up to 53 standalone digits, and a function's
  own calls hid in "+ helpers" (processCommand's lookupCommand and
  queueMultiCommand among 18 names); the owner then: an area's reading was
  "мясо сплошной стеной" (127 names several to a line), names one to a line
  everywhere; a reviewer: repeated Main flow names, clipped names, walls of
  "Also runs on its own" chains. UI only, `repomap render` on saved runs.
- **Component reading** (REPORT § External communication and data): summary
  with its entry and input kinds in words; Main flow closed by "Also runs on
  its own"; Files (each path opening to its functions by part, no "Read or
  written by", no "Path not established"); Connections without counts; one
  line of links led by "Component details". Areas and parts are the
  canvas's; Not reachable, TODOs and Analysis coverage moved to the "What is
  missing" page (`component-gaps`, Find keeps their rows' program).
- **Main flow:** two steps in a row naming one declaration are one; methods
  carry their type, same-named functions their module; a closure (`Run$1`)
  reads "in ReplicateCommand.Run" (`closureHome`: the innermost function
  whose lines hold it). Own work names its registering function alone:
  litestream's "main → … → Store.Close → … → Replica.Start registers it" was
  a real exact-call chain but the shortest of many, through the shutdown.
- **Function reading:** a call into the caller's own part is never a helper
  (`page_flow.go`); helpers fold under a nameless "+ helpers" only when more
  than three; calls stand under "Calls"; Reads/Writes under one closed
  "Reads and writes"; the dispatch site named, its counts on hover.
- **Area and part readings:** an area lists its parts with their description
  and keys alone; a part shows its keys (else its ways in) one to a line and
  folds every other declaration, by file; long "Calls into" folds. Names
  break only after `.`/`/` (`rmDotBreaks`), never at a hyphen.
- **Shape diff** (`<scratchpad>/pageshape/shape.mjs`, HEAD vs this pass on
  the saved redis 182554, litestream 182612, freqtrade 182749 runs, fresh
  ordinary runs, all cached but 16 freqtrade calls): component readings
  redis-server 9→5 sections, 14→0 digits, 10.4→5.1 screens; cmd/litestream
  9→5, 40→0, 9.1→4.9; freqtrade 10→5, 43→0, 8.9→5.2; every component ≤5
  sections and 0 digits; areas' crowded lines to 0 (freqtrade Core runtime
  42.2→5 screens, litestream VFS implementation 4.6→1.1); processCommand
  rows blockClientOnSwappedKeys, call → + freeMemoryIfNeeded, resetClient,
  addReplySds, freeClient, lookupCommand, queueMultiCommand, addReply;
  destinations, inputs per kind and sources unchanged; no page errors.
- **Checks:** `TestAComponentsReadingIsAtMostFiveSectionsWithNoDigits`
  (new), the outline test replaced by the home's programs, the flow, files,
  area, part and dispatch tests rewritten to the new shapes; `make test`,
  `make vet` (package parallelism 2), `make ui-test` and `make ui-visual-test`
  (64 passed, 5 skipped) pass on HEAD plus this pass alone.

## 2026-09-29 — An input is named by its word, a subcommand's flags are its options, an inline handler is named by where it is written

- **Why:** litestream's Inputs grid (owner: "не понимаю почему тут столько
  слов разных") read "json output raw JSON", "socket
  /var/run/litestream.sock control socket path", "replica replica URL
  (e.g., s3://bucket/prefix, …)", with `-json`, `-timeout` and `-socket` six
  to ten times each; its routes read `Server.handleInfo`, its MCP tools
  `DatabasesTool$1`.
- **a) Names.** The boundaries window of litestream's 87 entry names came
  back with the words themselves (`socket`, `POST /start`) instead of their
  `w*` refs, and the decoder discarded every one; a handler-less entry then
  took all its literals as its name. A sequence column with `ValuesFrom`
  (the name column's `words`) now takes a member written as exactly one
  option's value, the whole cell first (EXECUTION). With no word chosen, a
  handler-less entry is named by the first nameable word its code wrote
  (`lines.FirstEntryWord`; a handed value's first literal); its default and
  usage stay its registration as written, which its reading shows.
- **b) Options.** GroupsIndex nests a handler-less input under an input of
  its kind (`Reach.Options`, `Launch.Nested`; derived) when it is declared
  on the object that input's call made (argparse's `--force` on
  `add_parser("init")`), unless a handled input shares the object
  (freqtrade's subparsers), or in a case's branch or by code only that
  branch runs (ProgramIndex case and guard `branch`, compiled as
  `Branches`; the launch walk taking no branch call must not reach it). The
  input's page data carries `inputPath.options`; nested members leave its
  catalogue's members and "declares".
- **c) Inline handlers.** `ObjectFacts.Inline` (compiled, never persisted):
  the repository function a closure only wraps, else the function holding
  it, a method with its type, "(inline)": `ReplicateCommand.Run (inline)`,
  `RestoreTool (inline)` (it calls the helper `isReplicaURL` beside outside
  calls), never `$N`. Used for an entry named by its handler and for the
  declarations an input's reading names.
- **Fixtures:** Go `RunSubcommand` runs `runServe`/`runCheck`, each with its
  own flag set and `-verbose`; `StartSweeper`'s goroutine is "StartSweeper
  (inline)"; C kvcli's `bench` branch runs `bench`, whose `--requests` is
  its option; Python's `--force` on init's parser
  (`TestASubcommandsOptionsAreNestedUnderIt`,
  `TestCumulativeGoStartsAreAskedPerStatement`). TypeScript's switch and
  Clojure's case form carry the same branches; their fixtures declare no
  option inside a subcommand's own code yet.
- **Acceptance:** two ordinary litestream runs (default cache), exit 0: the
  first 4 m 31 s (21 live windows, rejected windows re-asked), the second
  57 s warm; rendered to the scratchpad's `redis-r2/run/latest-litestream.html`.
  `cmd/litestream`'s Inputs: 212 → 160 tiles; its 17 commands are the 15
  subcommand words, `help` and `-`, each subcommand reading its flags
  (`databases`: `-config`, `-no-expand-env`, `-json`); no flag word twice
  among the tiles, no `$N`, routes `GET /info` … `POST /unregister`;
  `litestream-test`'s subcommands hold theirs. Saved Redis and freqtrade
  runs render with the same tile counts (128/11/102; 326).
- **Left:** the query keys of a replica URL (`endpoint`, `region`,
  `forcePathStyle`, `skipVerify`, `storageClass`/`storage-class`,
  `concurrency`) are real settings a person writes in that URL, declared on
  `ParseReplicaURLWithQuery(c.URL)`'s result or `NewReplicaClientFromURL`'s
  `query`; tying them to the `url` setting would follow the URL's value, so
  they stay settings. Redis's `acceptHandler` is a request by the entry
  criteria (the accept handler of the listening socket) and wrote no word, so
  it keeps its handler's name. The reading's rendering of `options`, the tile
  kind line and a Clojure part's namespace prefix are the reading and canvas
  pages'.

## 2026-09-29 — One component under two names: a client package's shebang and library fold into its console script

- **Why:** freqtrade's whole map drew three components for `ft_client/`:
  `freqtrade-client` (console script), `freqtrade_client.ft_client` (the
  guardless shebang on the same file, the same two parts drawn twice) and a
  card "ft_client — Packages the freqtrade-client REST client as an
  independently installable library" holding only the package's tests
  (owner: "это вообще чет левое тут"). No existing rule fired: the Go
  sole-consumer fold is Go and `shared_code` only; the shebang has no launch
  callable, so no launch group; the model's `seed_of:t11` for the console
  script was refused (t11 is no advertised seed owner); the library was
  placed standalone and `claimByRoot` left it only `test_client/`.
- **Rule** (skeptic-reviewed on freqtrade, pykrx, litestream, the
  python-tutorial-game and cumulative fixtures; adopted with its changes),
  `foldPythonFacets` after the model's placements, journaled beside its
  decision: a guardless shebang on the file defining one standalone
  program's launch callable is that program's seed (`ScriptFileOf`, not
  offered to the model); a library whose declared packages all lie under the
  root of exactly one standalone program built from them and launched as the
  distribution's command (console/GUI script or `python -m`, itself or as a
  seed) is `folded_into:` it. The program's ProgramTarget carries
  `libraries` (declared top-level import packages), its page intro reads
  "Also installable as library `freqtrade_client`" (copied into the
  component's reading), and the atlas claims the library's root beside the
  program's own directory (`places.TargetInput.AbsorbedRoot`), never instead
  of it, so a guard tool beside the console script's file keeps sharing it;
  the Go fold now claims the same way. A fold moves the repository default to
  its owner (the Go fold had failed validation when the default was folded).
  freqtrade's root distribution folds into `freqtrade` the same way: its
  tests and test strategies become freqtrade's.
- **Equivalents:** JS/TS `bin` commands are executables of the one package
  target; a Clojure project is one target with its `-main` seeds; a C
  directory library holds only units no program links. None folds.
- **Tests:** the cumulative Python fixture gains `client/` (console script
  `fixture-client`, guardless shebang `cli.py`, a `bench.py` guard, its
  tests, the library): `TestClientPackageIsOneProgramWithItsLaunchFormAndLibraryFacet`
  (seed, facet, journal, default, the index's seeds and `libraries`, atlas
  claims with the tool sharing the directory) and
  `TestALibraryIsNoFacetOfAGuardItCouldHaveSeeded`;
  `TestAProgramNamesTheLibraryFacetFoldedIntoIt` (report);
  `TestAnAbsorbedRootIsASecondShallowerRootOfItsProgramOnly` (places).
  Code 4b51aa1f, d6a1afa9.
- **Acceptance:** `go test -p 2 -timeout 5m ./cmd/... ./internal/...` and
  `go vet` green on an export of the committed tree (the shared checkout's
  report, Go-fixture and C tests were red on other lanes' uncommitted work;
  contracttest and jstsproject timed out once at load 100+ and passed
  rerun). Ordinary run `repomap ~/git/freqtrade --no-serve --no-open` built
  from 4b51aa1f, default cache: exit 0, 11 min, but every tool took tests/
  too (`atlasPath("")` is ".", so each target without an absorbed library
  claimed the repository root; fixed in d6a1afa9). Rerun from d6a1afa9:
  exit 0, 11m52s, run `20260929-192959-freqtrade-dfea82fa6fa1`, portfolio
  a cache hit, atlas all cached but 7 calls (glossary always live), rendered
  to the scratchpad's `redis-r2/run/latest-freqtrade.html`. Seven components
  instead of ten: `freqtrade` (472 files, tests included, "Also installable
  as library `freqtrade`", seeds `main()`, the guard and `python -m`),
  `freqtrade-client` (root `ft_client`, seeds `ft_client.py:1` and
  `main()`, "Also installable as library `freqtrade_client`"), three
  build_helpers and two scripts tools holding only their own directories.
  The client's card is titled `ft_client/freqtrade_client`, the existing
  root-path label of a program alone at its root (it had read
  `freqtrade-client` only because the duplicate shared its root).
- **Not done:** the same-root duplicates remain: build_helpers' three guards
  each draw the same five parts, scripts' two the same two (a Python
  target's root is its anchor's directory and ties keep shared files); a
  claim rule of their own.

## 2026-09-29 — F5 othello: shadow-cljs builds are programs, keyword hand-overs and a future's body are registrations, test aliases mark tests, build descriptions are manifest facts

- **Why:** othello (Uncle Bob, Clojure + ClojureScript; ground truth
  `testdata/audit/othello/inventory.json`) came out nearly empty: one
  `deps.edn` target, no registration for Quil's sketch, the recipe
  `clojure -M -m othello.core` without the `:run` alias that brings Quil, and
  a "Spec helpers" part with `spec/othello/spec_helper.clj` in "Built from".
  Code f5c8f148, 378a0679, 35513784 (executable scope); contracts 42ff9ae5
  (CLOJURE, DISCOVERY, PROGRAM_INDEX).
- **a) Registrations.** `(q/sketch … :mouse-pressed host/on-press
  :key-pressed host/on-key …)` was twenty positional arguments; the facts
  name a handler only when a call hands exactly one callable, and the sketch
  hands six, so none. Now a Clojure call's trailing keyword/value pairs, or a
  trailing map (Clojure 1.11), are keyword arguments, and the facts read an
  outside call handed several repository callables under keywords as one
  registration per keyword (`keywordHandoffs`): registrar
  `quil.core.sketch.key-pressed`, the keyword its first word, placed at the
  entry. General: a synthetic WebSocketApp(`on_message=`, `on_close=`) case
  checks the shared rule; the Go, Python, JS/TS and C fixtures hold no such
  call yet (recorded). A `future`'s body calls carry `goroutine`, and the
  future's use is a call of `clojure.core/future` given them, so the started
  call's word is `clojure.core/future` (a started call's word is the call it
  is handed to, else `go`) and the statement is asked as written.
- **b) Discovery.** `ScoutShadow`: each shadow-cljs build naming its
  `:init-fn`/`:main`/`:entries` is a ClojureScript program
  (`clojure:shadow-cljs.edn:app`, executable scope) over the `.cljs` sources
  and the `:cljs` branch of the `.cljc` sources, seeded there; `js/` globals,
  `goog.*` and ClojureScript's own namespaces are its platform.
- **c) Recipe.** deps.edn, project.clj and shadow-cljs.edn rows are
  `manifest` facts (each alias's `main-opts`, `exec-fn`, `extra-paths`,
  `extra-deps` at its line; each build's target, output, entries), held by
  the target whose manifest it is. No model text is patched.
- **d) Tests.** An alias running a test runner (`speclj.main`,
  `cognitect.test-runner`, `kaocha.runner`) names its `:extra-paths` as test
  directories, Leiningen its `:test-paths`; every source there is a test
  source. The no-tests negative reads adapter test sources, and no test
  source is judged a dead module.
- **Acceptance:** one ordinary run, `.bin/repomap ~/git/othello --no-serve
  --no-open` built from the committed tree at cc0f2765 (the shared checkout
  briefly did not compile on another lane's gofacts work), default cache:
  exit 0, 24 s, runs `20260929-181127-othello-a0d4d5e264c6` (othello) and
  `20260929-181129-othello-app-fa6ec6043291` (app), rendered to the
  scratchpad's `redis-r2/run/latest-othello.html`. The page shows two
  programs, `othello (package)` entry `-main()` and `app (executable)` entry
  `init()`, both placed standalone; each with 3 user interactions (mouse
  pressed, mouse moved, key pressed, handlers `host/on-press`, `on-move`,
  `on-key`); "How to run": `clojure -M:run` and `clojure -M:web watch app`
  (open localhost:8080); no spec file in "Built from", no "Spec helpers"
  part, no "no tests" gap. The model answered the sketch's `update`/`draw`
  none or undecided (the criteria's "a hook the program's loop runs on every
  turn") and the future's start none (a one-shot start, like the Go
  fixture's cache load): not patched. That run predates 378a0679: its three
  entries are named after the declaration (`start!`, `update-state`, the
  nearest-declaration caller of the top-level `defsketch`) because no word
  named them, and the six spec files were "not reachable"; offline (a
  no-model run) the facts now give the keyword as a word and no dead
  module, and the preset test checks the naming words. Claim audit
  (`REPOMAP_AUDIT_RUNS`, same run): commands 0 of 14 must (the inventory
  anchors keys at `events.cljc:211` and clicks at `:174`, the inputs sit at
  the registrations), workers 0 of 4, external 0 of 1, data 0 of 2; one
  audit flag, "invented name cljs.edn", is its tokenizer reading
  `shadow-cljs.edn`. `make test` and `make vet` pass on the committed tree.
- **Missing, recorded:** a function literal handed to an outside call
  (`(js/setTimeout (fn [] …) 20)`, the web build's AI search) hands nothing,
  since the adapter projects no closure; `(Thread. f)` and core.async
  `go`/`thread` start nothing; places names a top-level call's caller by the
  nearest declaration above it; the key map `key->command` is no
  sub-argument of `on-key`.

## 2026-09-29 — F3: what outgoing calls reach is named once per destination, not per call

- **Why:** after 84e48166 fed `reached_from` into destination naming,
  litestream's control socket had 10 names and 8 boxes (3 names that
  morning), and populate.go's windows w40 (11 rows) and w43 (7) came back
  "row was not answered", seven boxes titled `sql.DB.Exec`, `sql.Open`, ….
  Owner rule: a decision is asked once, code derives the rest. Design
  reviewed by a skeptic before the code; its seven changes are in. Code
  04b0f429.
- **Cause (a), many names:** `destination` was a cell of every outgoing row,
  asked in one window per owner declaration. litestream's seven
  subcommands each dial `net.DialTimeout("unix", *socketPath, …)` (walk end
  `{--socket}`) and post to `http://localhost/<command>`; `net` and
  `net/http` are `none` in atlas_systems, so 14 windows each drew a free
  `other:` name. `reached_from` only shifted the draws (the DialTimeout rows
  had none: their closure has no exact caller).
- **Cause (b), unanswered windows:** the response held exactly one row, the
  one whose key the response-format example shows (`b16`, `b22`, `b147`).
  Not size: w40's user message was 23.5 KB (19.1 KB that morning, when the
  same 11 rows were answered), `reached_from` one 57-byte entry a row.
  Replayed 3 times through the provider, w40 answered 1 of 11 rows in 3/3
  draws, w43 1 of 7 in 3/3, w59 1 of 2 in 2/3; with `reached_from` stripped,
  w43 answered 7/7 in 3/3 but w40 still 1/11 and w59 1/2: near-identical
  rows under a one-row example with a real key, not one field. Open for the
  table package: a placeholder key (or one skeleton per row) in the
  example, checked first on the saved w40.
- **Change (READING § Outside systems, One destination, one name):** a row
  whose package atlas_systems named takes that name in code (89 of 89
  answered rows with a catalogue package had chosen it). A fact naming no
  external takes the package of the talking call at its exact site (the SQL
  text on `db.Exec` claimed that call), else of the talking call handed
  what its site's call returns (`db.Exec(fmt.Sprintf(…))`), and that call's
  walk. Rows of one targets set whose reaching call's walks end alike are
  one destination: a URL by scheme and host as written, a setting by the
  setting, any other value by the site where the walk stopped; a row with
  no reaching call is alone. One system among a destination's packages
  names all its rows; none asks once (`repomap.atlas.destinations.v1`, table
  `atlas_boundaries`, round 3); several ask each package-less row alone.
  The item: ends with the code where they end, calls as written, callers
  with signature, the calls beside the rows and the callables handed over
  (litestream's `Run$1` by `InfoCommand.Run` to
  `net/http.Transport.DialContext`), `reached_from` by name. The outbound
  boundaries row keeps `line` and `address` only, no `reached_from`; without
  captions a row with no address to choose is not sent.
- **Checks:** `TestADestinationIsNamedOnceForAllItsCalls` (package names,
  one server from two declarations asked once with both calls, callers and
  `reached_from`, a query fact on Exec and a formatted one handed to Exec
  named by `database/sql`, a URL whose packages name two systems asking its
  plain request alone, facts without a reaching call alone),
  `TestDestinationKeyIsWhereTheWalksEnd`,
  `TestDestinationWindowSharesItsCatalogueAndDecodesClosedAndFreeNames`,
  `TestDestinationNameRefusesAnUnlistedOrEmptyNameAlone`; kvd's connect
  asked once as `netConnect in net.c [main repl]`; Echo's driver open named
  by its package, its statement asked. `make test` and `make vet` (package
  parallelism 2) pass on HEAD plus these files.
- **Acceptance (litestream, the ordinary binary, online):** run
  `20260929-175350` (exit 0, 56 s): 15 destinations asked of 146 outgoing
  calls; with formatted SQL handed to its Exec, `20260929-175908` (exit 0,
  35 s): 4 (`{--socket}`, `http://localhost`, ssh's `c.Host`, the heartbeat
  URL); atlas_boundaries 97 live calls before, 1 now; a rerun is fully
  cached (`175647`). The report names the 14 CLI calls "Litestream control
  socket", populate.go's 18 SQLite; the map (headless Chromium, loopback
  server) shows one control-socket box, no page errors.
- **Probe** (`<scratchpad>/f3probe`: saved request bodies replayed byte for
  byte through `deepseek.Client`, 3 draws; text-model answers carry no
  probabilities, so stability is agreement across draws):

  | Destination | Before (per-row windows of `165546`) | After (3 draws) |
  | --- | --- | --- |
  | control socket, 14 calls | 14 questions; 7, 7 and 9 names per draw ("Unix domain socket", "Litestream socket server", "Litestream sync socket", "litestream daemon", …) | 2 questions in one window; both "Litestream control socket daemon", "…daemon", "Litestream control socket": one name per draw |
  | populate.go, 18 calls | w40 1 of 11 answered, 3/3 draws; w43 1 of 7, 3/3 | no question: SQLite from `database/sql` |
  | ssh `c.Host` | "SFTP" | "SFTP", 3/3 (catalogue entry) |
  | heartbeat URL | "heartbeat endpoint" | "heartbeat endpoint", 3/3 |

- **Open:** the socket and the HTTP server it carries stay two destinations
  that share a name only by the model's answer (the client's
  `Transport.DialContext` is not followed); the name's spelling varies
  between draws ("… control socket" / "… control socket daemon").

## 2026-09-29 — F4: a table of another table's keys, or one only tested for membership, is no inputs of its own

- **Why:** the freqtrade acceptance (scratchpad `accept/run-freqtrade.log`,
  `claim-audit/accept/freqtrade.md`) had 272 command extras, up from 11
  after the Python tables (e4de0f0c) and the table question's read lines
  (84e48166). The 28 `ARGS_*` lists of option keys were each answered
  command beside `AVAILABLE_CLI_OPTIONS`' 124 real flags, and
  `NO_CONF_REQURIED` listed the trap `backtest-filter`. The item showed
  `_build_subcommands` handing `ARGS_TRADE` to `_build_args`, never
  `_build_args` looking each element up in `AVAILABLE_CLI_OPTIONS`: the
  code fact was missing, not the model's judgement. Code 5fc884b5; the
  code derives it, no question is added or changed.
- **Facts:** ProgramIndex's shared `membership` and `keys` witnesses on a
  read of a variable (PROGRAM_INDEX), recorded by Python (PYTHON); atlas
  graph 24 carries each read's form (`read_at` `form`, `keys_of`).
- **Reading:** a table every read of which outside tests keys another asked
  table with one-word rows, or only tests a value's membership, is not
  asked and makes nothing; a tables.md line names its readers (READING).
- **Skeptic (acted on):** keys needs X ≠ T (`HELP[name] for name in HELP`
  folded HELP away), a subscript on every pass (a guarded lookup, a try
  with handlers, a continue before it, a comprehension's condition), no
  positional match through a class (`K.build(k, OTHER, FLAGS)` shifted a
  position), every read a keys read, X asked (chains followed, a cycle
  asked) and one-word rows; membership needs one case (`if c in A: … elif
  c in B:` is compared case by case and stays asked); validation of both
  kinds. A validation list before a `getattr` dispatch (`KNOWN`) is none,
  as a list written in its condition is. Each is a fixture case in
  `dispatch.py` (`TestPythonTablesNamingAnotherTablesRowsAreNoInputs`,
  which fails with the reading rule off: the three tables are asked).
- **Missing equivalents:** C tables record neither form (no membership
  operator, no subscript by a string; C.md); Go, JS/TS and Clojure record no
  tables of names.
- **Measured (no-model freqtrade, `python:.:script:freqtrade`):** 60 tables
  outside tests; 28 `ARGS_*` hold keys of `AVAILABLE_CLI_OPTIONS` (167
  rows), `NO_CONF_REQURIED`, `NO_CONF_ALLOWED` and `SUPPORTED_EXCHANGES`
  (answered none before) are only tested (38 rows); 29 tables asked, their
  items byte-identical, so no probe was needed. Against the acceptance
  audit, 167 of the 272 command extras sit on these tables' rows (→ 105;
  the rest: 95 `cli_options.py` flags outside the inventory, 4
  `trade_model.py` settings, 3 other rows), and the trap hit goes (1 → 0);
  every subcommand they duplicated is found at its `add_parser` call, so
  recall is unchanged. Not yet confirmed by an online run.
- **Also:** the `atlas_inputs` tables.md line counted comparisons as tables
  (freqtrade "100 of 60 tables"); each count is now its own question's.

## 2026-09-29 — Acceptance fixes: every stage journaled, each refusal once, no empty frontier, the claim audit's file and target matching

- **Why:** the integrated acceptance (scratchpad `accept/`, `claim-audit/accept/`)
  found F1, F6, F7 and three audit matching gaps. Code 6819ee2e, e9ddfd78,
  d43bfcf4, d18c83c5. Focused tests and vet pass (debugdump, atlas/reading,
  report, audit, run, modeldiag).
- **F1 (6819ee2e):** `atlas_inputs`, `atlas_systems` and `atlas_program` were
  no semantic stage: every run warned `stage=unknown
  code=artifact_write_failed` and journaled none of their exchanges (their
  `rejected.jsonl` rows had no ref). The stages are one set in debugdump.
  The stage owners import debugdump, so no list is derived at compile time;
  `TestEveryDefinedStageIsASemanticStage` reads the `Stage…` constants of
  lines, reading, terminology, reporttranslation and debugdump from source
  (removing a stage from the set fails it by name).
- **F6 (6819ee2e, d18c83c5):** a table's row or cell refusal was journaled
  twice: by the observer with its exchange, and by the reader under the
  window's `response.ref.json` (the same response payload) with the row key
  twice in samples. The reader's rows are `AlreadyJournaled` when the run's
  observer journals, and keep one key; a window with no accepted row still
  journals each row's reason (the observer has only its first). The
  exchange ref alone is kept: it links the request as well.
  `TestTableRowRefusalIsJournaledOnce`; the partly refused run test expects
  one row from the journal.
- **F7 (e9ddfd78):** freqtrade's `getattr(ccxt, name)(config)` is a frontier
  `()`: 41 exchange tiles read "Address passes through ()". A frontier naming
  nothing prints no address line; the chain's steps stand. `repomap render`
  of the saved freqtrade run: 0 such lines. The five ccxt calls of kind
  `client_request` (set_position_mode ×2, un_watch_ohlcv,
  un_watch_ohlcv_for_symbols, publicPostInfo) come from atlas_api's `talks`
  answer (round 2 window 11 answered 3 client_request, 3 sdk, 4 none for
  ccxt.Exchange members; boundaries receive it as `kind_given`). A model
  answer, not patched.
- **Claim audit (d43bfcf4), the same claims, not a widening:** a file record
  matches a data item by its written path (the same literal exactly, not a
  qualifier's words, anywhere in the component; or the write of its field
  storing it at the item's line); a path not established is an `unknown`
  column, not an extra; a target maps to a component by its key or its
  program's path (seeds, a declared component's entry), not its display
  name. Re-run into `claim-audit/accept2`: redis data must found 0 → 3 of 5
  (dump.rdb, appendonly.aof, /tmp/redis-%p.vm; /var/run/redis.pid may),
  extras 7 → 0, unknown 3; freqtrade data extras 158 → 37, unknown 121;
  litestream extras 113 → 33, unknown 79, may found 5 → 6
  (`C:\Litestream\litestream.yml`); othello `t1 othello` → othello-desktop,
  whose only report row is its entrypoint (no Inputs, outside calls or data
  yet), so its counts are unchanged (0 of 21 must). No other inventory moved.
- **Open:** litestream's WAL path is recorded as `-wal` (`db.WALPath()`'s
  unknown prefix dropped); redis-check-dump's mmap'd dump file is a path not
  established at its argv read; a whole-window refusal is still journaled
  twice in kind (the observer's `response_validation`, the reader's
  `window_rejected` with its row count).

## 2026-09-29 — The files a program keeps are its data: path as written, the functions reaching it, no role

- **Why:** owner, DATA includes the files a program owns; skeptic, no role
  label, the path template and the functions that read or write it
  (claim-audit rootcause `external-data.md` F2–F6, F5's role question
  dropped). No new question: `talks: file` and the argument decision
  (84e48166) are the model's part. Code 6ae2c794; `make test`, `make vet`
  (package parallelism 2) and `make ui-test` (133) pass.
- **Derivation (code, `reading.FileReader`):** each call outside tests of a
  `file` symbol, in declarations its program runs, is walked along its
  decided argument; calls are grouped by where the path ends: a literal or
  template (`{--flag}`, `{env:KEY}`, an `initializer:` value included), a
  field the walk cannot follow whose accesses the program records by that
  path (its writes are its values, each walked from what it stores), else a
  path not established, one per function. Data records `w1…` of kind `file`
  (origin `call`, scope the target) list the calls and a field's values;
  GroupsIndex names each call's and write's subject (`call_subject_ids`,
  `value_subject_ids`) and orders `w*` before `y*` by ordinal.
- **ProgramIndex 23:** a C write by plain `=` keeps the value it stores
  (`Relation.Value`, the places graph's `SymbolField.Value`); compound
  assignments, `++`/`--` and array elements store none. No model request
  reads it. The python-tutorial-game indexes are resealed (only version and
  seal differ).
- **Report:** the Data shelf's "Files" lists each path as written (or its
  field's stored paths "from" the field, or `{field}`), one line for the
  paths not established, no functions or places; the component's reading
  (`rmComponentFiles`, after its areas and parts) adds what else sets a
  field (the setting whose branch writes it, else the writing function, each
  once) and "Read or written by": the functions by part box. Paths link to
  their write site; names read their function; no line numbers, no role.
- **Checks:** `TestCFixtureFieldWritesKeepTheValueTheyStore`,
  `TestEveryLanguageKeepsTheFilesItsCodeReaches` (C kvd: `{server.dbfile}`
  = `dump.kv` by main, not established by loadConfig, opened by
  saveSnapshot; `{env:KVD_CONFIG}` by loadConfig; Go `fixture-state.db` and
  LoadServerConfig not established; Python read_settings and Clojure
  deliver! not established; JS/TS names no fs symbol), the kvd preset
  through GroupsIndex (`checkKvdFiles`), and the page's shelf, reading and
  column (`TestAProgramsFilesAreItsDataWithTheFunctionsReachingThemByPart`).
- **Redis preview (no model, redis-server's places with fopen/unlink/open
  argument 1 and rename argument 2 preset; the lead's run decides):** 13
  records: `dump.rdb` from `server.dbfilename` (initServerConfig 1493, and
  loadServerConfig 1755 not established; rdbSave's rename, rdbLoad,
  updateSlavesWaitingBgsave, syncWithMaster's rename), `appendonly.aof`,
  `/var/run/redis.pid`, `/dev/null`, `{server.logfile}`,
  `{server.vm_swap_file}`, and 7 functions whose paths are snprintf
  buffers. Rendered through `repomap render` on the injected records and
  read in headless Chromium: the column's Files section, no page errors.
- **Open:** which call reads and which writes needs a per-call decision (not
  asked); `/tmp/redis-%p.vm` is behind `zstrdup(...)`, whose result the C
  adapter does not follow, so vm_swap_file shows `{server.vm_swap_file}`
  set in initServerConfig; Go writes carry no value; audit.py's data rows do
  not yet take the calls and writes as anchors.

## 2026-09-29 — An outside call's record in the column prints no place; flow items anchored at their function

- **Column (owner rules: no line numbers, no duplication):** an External
  communication tile's record (`rmOutboundRecord`) stands open with no
  printed place. Redis's "gethostbyname anet.c:146", "netdb.h.gethostbyname
  · anet.c:146" and the intro's "anet.c:146" are one name,
  `netdb.h.gethostbyname`, a link to anet.c:146 with the place on hover;
  destination steps are links too. "redis-server connects out from
  syncWithMaster → anetTcpConnect → anetTcpGenericConnect" is gone where
  "Called from" lists the callers; it stays on freqtrade's 25 tiles with
  none (`fetch_balance`: "connects out from get_balances"). The model's note
  is said once, in the intro; the empty actions row goes.
- **Page and column:** a chain whose frontier is the record's own callable,
  one step at its line ("Address passes through ccxt.Exchange.create_order"),
  is no line; equal consecutive steps are one (freqtrade's `start_install_ui`
  and `dl_url` both read "install-ui").
- **Audit errata (2026-09-29):** Main flow steps are anchored at the
  callee's declaration, flow items were at the loop or call site (0/8
  found). Re-anchored at the declaration: Redis aeMain redis.c:9153 → ae.c:375;
  freqtrade Worker.run worker.py:78 → :76, Worker._throttle :186 → :145,
  create_client ws_client.py:221 → :197, standalone uvicorn webserver.py:339
  → uvicorn_threaded.py:36 (UvicornServer.run, its handler); litestream
  Replica.follow replica.go:817 → :791, windowsService.Execute
  main_windows.go:80 → :57. FreqtradeBot.process was already at its `def`
  (freqtradebot.py:257), now asserted. Redis run `20260929-140032`'s Main
  flow names aeMain at ae.c:375.
- **Renders:** the newest saved runs (Redis `20260929-140032`, freqtrade
  `140146`) hold ProgramIndex 21, which the current binary (22) refuses; they
  were rendered by a scratch build overlaying only that constant.
  Screenshots `<scratchpad>/look/ui-ext-{before,after}-*.png`.

## 2026-09-29 — One batch of items and criteria: how a table is read, whose interface, a key's values, talks of handed symbols, `file` and which argument, where a call is reached from

- **Why:** claim-audit rootcause `commands-kinds.md` fixes 2 and 7 and fix
  1's criterion, `external-data.md` T, F1 and A's second half, in one batch
  because each change re-buys cached answers (84e48166).
- **Tables of names (fix 2):** the item gains `file` and `read_by` entries
  with the reader's signature, its reading lines as written (places
  `read_at`, whose reader IDs the graph's ID compaction now maps) and its
  callers outside tests with their calling lines. The file was the missing
  fact: with lines and callers alone redis-cli's `cmdTable` stayed request
  0.73–0.81.
- **Criteria (`entry_options.md`):** request never what this program sends;
  command includes what a person types or passes, words typed into a client
  included (owner 2026-09-29); interaction only from a window or screen this
  program draws, a press another program delivers is a request, a button
  this program sends is none (fix 7); a value a key may take is none.
- **A key's values:** a word call comparing a later literal element of a
  value than the call before it in the same declaration shows that key
  (`compared_after`) and waits; when the key is an entry the call is its
  value and is not asked (Redis's loglevel values drew 0.49/0.46 even with
  the key shown), else it is asked in a second round.
- **T:** a handed symbol the code calls is asked `talks` instead of
  `publishes` (`repomap.atlas.api.v9.handed`); a handed symbol no call names
  (a table row's field) is asked `binds` alone (`.handed.uncalled`), since
  talks drew `db` for `redis.c.redisCommand.proc` with no call to decide.
- **F1:** `talks` offers `file` (no boundary, calls not asked `enters`).
  The per-package list in `destinationArgument` (net/http, requests, httpx,
  aiohttp, axios, ky, got, otlp, os.Getenv, flag.String) is deleted: each
  symbol whose talks is an outgoing kind or `file` is asked which argument
  or receiver names what it reaches (`repomap.atlas.argument.v1`), and a
  symbol whose result a chosen argument carries is asked in a next round
  (`result_given_to`). An option's declaration gives `{--name}` from the
  call's `command` answer, an environment read `{env:KEY}` from its fact.
  File calls' paths are logged as `atlas_files` in `tables.md`, not yet
  projected; a request builder's method is no longer read.
- **A:** each outgoing boundary row carries `reached_from`, its programs'
  first callers outside the call's file (or a seed or input handler), by
  name and signature; the destination prompt names a system by what it is
  to the program.
- **Probe:** 3 draws per row through Jev (`jev-1.13.0`), rows built by the
  ordinary builders (`readAPI`) over the saved places of Redis
  `20260929-140032`, freqtrade `140146` and litestream `143047`, other
  questions answered from the saved answers (`<scratchpad>/b3probe`).
  Margin is the lowest top-minus-runner-up of the three.

  | Row | Before | After (3 draws) | Margin |
  | --- | --- | --- | --- |
  | redis-cli `cmdTable` table | request 0.96 | command 0.88, 0.83, 0.84 | 0.69 |
  | server `cmdTable` (`redis.c.redisCommand.proc`) binds | request | request 0.78, 0.80, 0.78 | 0.65 |
  | DEBUG `strencoding` table | command 0.55 | none 0.88, 0.91, 0.89 | 0.76 |
  | `symsTable` table (control) | none 0.62 | none 0.78, 0.80, 0.81 | 0.57 |
  | loglevel key `argv[0]` (control) | setting | setting 0.89, 0.88, 0.90 | 0.76 |
  | loglevel value `argv[1],"debug"` | setting 0.66–0.81 | not asked (value of loglevel); asked alone 0.49/0.46 | — |
  | `strcasecmp(server.logfile,"stdout")` | setting 0.52 | none 0.63, 0.66, 0.66 | 0.26 |
  | `CallbackQueryHandler` binds | interaction 0.68 | request 0.80, 0.78, 0.80 | 0.63 |
  | `CallbackQueryHandler` talks | (publishes none) | none 0.97–0.98 | 0.96 |
  | `CommandHandler` binds | request 0.93 | request 0.92, 0.92, 0.87 | 0.76 |
  | `InlineKeyboardButton` "Cancel" enters | interaction | none 0.87, 0.86, 0.87 | 0.74 |
  | `InlineKeyboardButton` `force_exit__{…}` enters | interaction | none 0.81, 0.86, 0.85 | 0.66 |
  | litestream `ssh.Dial` talks | never asked (publishes none) | client_request 1.00 ×3 | 1.00 |
  | `ssh.Dial` binds | none | none 0.93 ×3 | 0.88 |
  | Redis `fopen` talks / argument | none / — | file 1.00 / argument 1 1.00 | 1.00 |
  | Redis `connect` talks / argument | client_request / — | client_request 1.00 / argument 2 (`&sa`) 0.95–0.96 | 0.91 |
  | freqtrade `pathlib.Path.open` talks / argument | none / — | file 0.98–0.99 / receiver 0.99 | 0.96 |
  | litestream `os.Create` talks / argument | none / — | file 1.00 / argument 1: name 1.00 | 0.98 |
  | `http.Client.Do` / `NewRequestWithContext` argument | list | argument 1: req 1.00 / argument 3: url 0.99 | 0.98 |

- **Question counts (rows the new builders make over the saved places):**
  talks re-asked for every called symbol (Redis 128, freqtrade 773,
  litestream 601, handed ones included); `publishes` gone (4, 39, 23);
  `argument` new (Redis 3 in the probe, where of the file symbols only
  `fopen` was answered file; freqtrade 88; litestream 75; plus the symbols
  whose results a chosen argument carries); `enters` fewer by the file
  symbols' calls and a key's values (Redis 307 → 282 with the comparisons
  and sub-arguments of the day).
- **Fixtures and tests:** kvcli's table item (file, reads, callers'
  lines), kvd's persist values not asked, the connect's destination asked
  with main and repl; reading tests for the argument rounds, the builder
  walk, key values and reached-from; the Go, Python and JS/TS destination
  chains read with preset decisions.
- **Contracts:** READING, EXECUTION, PROGRAM_INDEX, C.

## 2026-09-29 — Started functions asked per statement (G1); words a handler compares with what it was handed are its sub-arguments (fix 4, K3)

- **Why:** litestream's workers were never asked: each monitor is started by
  a `go` statement on the repository's own function, and facts skipped a
  call to an owned callee as delegation (claim-audit rootcause
  `workers.md`). Redis asked SORT's, DEBUG's and SLAVEOF's `strcasecmp`
  words one by one and got near ties (`commands-kinds.md` fix 4).
- **G1 facts 6 (a88fec67):** a `calls` relation with the shared invocation
  `goroutine` or `async_task` and one exact repository callee is a
  registration handing that callee over (`internal/facts/started.go`), the
  second exception beside D1's table rows. Word `go`, or the call a
  coroutine is handed to (`create_task`); `invocation` on the fact; no
  outside symbol. A closure written in the statement is named and handled
  by the one repository function it calls itself (a call whose result
  another is given is part of it, deferred calls not counted), else keeps
  its own name.
- **G1 reading (a88fec67, places graph 22, now 23 with the comparisons):**
  `repomap.atlas.starts.v1` (`starts`, Jev, memoized per item,
  `prompts/api_start.md`): request, scheduled, continuous, queue_consumer
  or none from `entry_options.md`, never the per-symbol `binds`. Item: the
  statement as written (closure body included; for a coroutine the call it
  is handed to), `in`, `starts` with its signature, `calls` by name,
  literals. An entry answer is handled by the started function and named
  by its declaration (the statement's file may hold only the starter; the
  places caller would have named the enclosing function); a function the
  graph holds no declaration of is not asked.
- **K3 (cfbc8141):** a call `readCalls` would ask, in an established
  entry's handler (a `binds`, kept-callable or `starts` entry, one kind per
  handler), whose argument before its first word is a field or element of
  the handler's own parameter, or whose receiver is one with the word
  first, is not asked: an entry of the handler's kind, handler not
  established, nested by the existing `Launch.Nested` (no GroupsIndex
  code). Kept callables are asked before the calls. The hook sits after
  the talks, handed and other-fact filters, so a `talks` call stays
  outgoing. A whole parameter beside a word (`fmt.Fprintf(w, …)`,
  `res.send(…)`, `req.RequireString("path")`), a word before the value and
  a method receiver stay asked: Express's `res.send("…")` and Go's
  `c.String(…)` made the whole-parameter shape unsafe, so litestream's 13
  MCP `RequireString` rows are still asked (and nested once answered
  request, as before).
- **Counts (no-model runs of the committed code, no provider call):**
  - litestream cmd/litestream: 20 hand-over facts, 19 asked, 1 closure
    calling nothing not asked (`Main.Run`'s signal waiter, main.go:183).
    Handlers: `DirectoryMonitor.run`, `DB.monitor`, `Replica.monitor`,
    `Store.monitorCompactionLevel` ×2 (store.go:199/207, the second through
    `s.SnapshotLevel()`), `Store.monitorL0Retention`,
    `Store.monitorValidation`, `Store.monitorHeartbeats`,
    `ReplicateCommand.runOnce`, `MCPServer.Start`; closures keeping their
    names: `Start$1` ×2, `Run$1` ×2, `Run$2`, `Compact$2`,
    `syncReplicaWithRetry$1`, `snapshotReader$1`, `Restore$4`.
  - cmd/litestream-test: 3 (`LoadCommand.worker`, `LoadCommand.reportStats`,
    `generateLoad$1`); cmd/litestream-vfs: 25 (adds `VFSFile.syncLoop` ×3,
    `VFSFile.monitorReplicaClient` ×2, `VFSFile.runHydration`,
    `VFSFile.monitorCompaction`, `VFSFile.monitorSnapshots`,
    `VFSFile.monitorL0Retention`).
  - Redis redis-server (a scratch probe reading the no-model graph with the
    saved answers of 20260929-140032, every other question neutral):
    `enters` questions 177 → 163; SORT's sub-arguments alpha, asc, by,
    desc, get, limit, store (was limit), DEBUG's loadaof, object, reload,
    segfault, swapout beside its encodings, SLAVEOF's no, one; inputs
    139 → 152.
- **Fixtures:** Go `cmd/worker` → `StartBackground` (direct
  `RunCommitWorker`, a wait-group closure handled by `RunCompactor`, a
  one-shot `loadCache` answered none) and `cmd/worker/status.go` (HEAD and
  X-Verbose nested, `Fprintf(w, …)` asked); Python `create_task` of
  `poll_prices` and `announce_start`, `run_init`'s `fnmatch`; C kvd
  `setCommand`'s `nx`. `TestCumulativeGoStartsAreAskedPerStatement`,
  `TestCumulativePythonStartsAreAskedPerStatement`, the C and Python input
  presets. Missing, recorded: JS/TS (no starting statement; a handler's
  calls name no outside symbol without the dependency's declarations) and
  Clojure (`future`, `core.async/go`; no argument origins).
- **Contracts:** PROGRAM_INDEX, GO, PYTHON, C, JSTS, CLOJURE, READING,
  CURRENT (facts 6).

## 2026-09-29 — Values compared with several words; Python tables of names (fixes 1 and 3)

- **Why:** litestream's 14 subcommands (`switch cmd` in `Main.Run`,
  main.go:138–218) and litestream-test's 5 (`switch fs.Arg(0)`) were never
  asked: a switch or an operator is no call, so `readCalls` had nothing,
  and `cmd`'s origin read `one of: "" | not followed`. Python recorded no
  table of names, so freqtrade's `AVAILABLE_CLI_OPTIONS` (124 `Arg(...)`
  rows) gave no row.
- **Facts (e4de0f0c, ProgramIndex 22):** `Object.Comparisons` on a callable
  or module body: a value compared with two or more different non-empty
  words in two or more cases, each case a closed form (`case`, `equals`),
  its words and its branch lines. A lone `==` and one condition naming
  several words for one branch are none. Go (DirectCallIndex 17, a string
  switch and `==`; source values now follow a slice element, write a
  constant index and name `os.Args`), Python (if/elif `==`/`in`, `match`;
  parallel assignment binds each name to its own value), JS/TS (helper 28,
  result 19: switch, `===`/`==`), C (a switch on character literals) and
  Clojure (`case`). Comparisons written as calls (C `strcmp`, Clojure `=`,
  Go `strings.EqualFold`) stay per-call facts. Python `Object.Rows` for a
  module-level collection of one shape that a function outside tests
  reads; `CONF_SCHEMA` (nested) stays a gap, and `SCHEMA_TRADE_REQUIRED` is
  a table like any other (no name matching credited).
- **Reading (5892d35d, places graph 23):** `SymbolFacts.Comparisons` and a
  table's `ReadAt` (reader place, line, column; callers through the
  reader's `CalledBy`). Each comparison is asked once in `atlas_inputs`
  (`repomap.atlas.dispatch.v1`, `enters`, Jev, memoized, own state
  `api_dispatch.md`; options and criteria unchanged): `compares`, `from`,
  `in`, `cases`. An entry answer makes one input per case (handler not
  established, declared by the comparing function); a setting case reads
  its branch's written fields (report `branchAt`). Not yet: an undecided
  comparison is not a launch-walk unsure call, and K3 (a handler's own
  comparison of what it was handed) does not skip comparisons.
- **Counts (no-model runs, committed binary):**
  - litestream cmd/litestream (9 s): 14 comparisons, 62 cases, 73 words;
    `Main.Run`'s is one question of 17 cases (the 14 subcommands, `wal`,
    the help words, `""`), its origin `one of: "" | element "0" of
    parameter #2 args of Run`. Also asked: `NewReplicaFromConfig`'s
    replica types (8), `InitLog`'s levels and formats, the restore
    integrity check, S3 debug modes, and 7 error-code or host checks.
  - litestream cmd/litestream-test (4 s): 3 comparisons, 13 cases; `Run`'s
    `fs.Arg(0)` is one question of 6 cases (`help` and the 5 subcommands).
  - freqtrade (freqtrade script, 76 s): 46 comparisons outside tests (119
    cases, 140 words; 90 more in test files); 64 tables read by a
    function (582 rows), 60 of them declarations outside tests (555 rows,
    174 reads), `AVAILABLE_CLI_OPTIONS` read by `Arguments._build_args`
    (arguments.py:358). A lone-`==` rule would have asked about 311.
  - Redis 1.3.6: one comparison per program, `stringmatchlen`'s glob
    letters (redis-server) and `cliReadReply`'s reply types (redis-cli);
    both are expected to answer none.
- **Fixtures:** go `RunSubcommand`/`IsDefaultLevel` (tool_cli.go), python
  `dispatch.py`, jsts `src/dispatch.ts`, clojure `run-command`, c kvcli
  `shortOption`; `TestEveryLanguageRecordsAMultiWayDispatchAsOneComparison`,
  `TestPythonTablesOfNamesAreTheOnesAFunctionReads`,
  `TestEveryLanguageAsksAComparisonOnceAndMakesAnInputPerCase`. The
  python-tutorial-game indexes are regenerated through no-model runs of
  the materialized fixture (3 new comparisons and one table).
- **Contracts:** PROGRAM_INDEX, GO, PYTHON, JSTS, CLOJURE, C, READING, the
  testdata README, CURRENT's format numbers.

## 2026-09-29 — Claim audit as a gated Go test

- **What:** `internal/audit` (test-only; DEVELOPMENT "Claim audit") ports the
  scratchpad `audit.py`. `TestClaimAudit`, gated by `REPOMAP_AUDIT_RUNS`,
  reads a run through `report.ReadRunReceipt`, `groupindex.Overlay.Hydrate`
  (its derived `Launch.Nested`) and `atlas.Read`, scores it against
  `testdata/audit/<repo>/inventory.json` (ca90d611, with the Redis aeMain
  erratum) and writes `<repo>.md`/`.json` to `REPOMAP_AUDIT_OUT`. It fails
  only on a BROKEN flow link, an invented name or a missing quote (new:
  AUTO-4, recipe notes). `TestAuditFixture` covers the rules in `make test`.
- **Rules against audit.py:** nested inputs are no rows (fix 0); an outgoing
  call's `ReachedFrom` caller site may match an external item, ranked after
  the row's own location; audit.py's handler, call-chain and table-owner
  anchors (its tiers 5–7) are not ported; `flow` items are scored against the
  Main flow steps (±2 lines), not the workers.
- **Newest runs** (redis `20260929-140032`, litestream `-143047`, freqtrade
  `-140146`; measured at 3408c475, since the tree's uncommitted ProgramIndex
  22 refuses these version-21 runs). Must found per requests / commands /
  workers / external / data; no falsity in any:
  - redis: 95/96, 47/50, 2/2, 3/3, 0/5; trap hits 0 (12 nested); Main flow
    items 0/1.
  - litestream: 16/18, 38/69, 0/13, 10/16, 3/15; extras 13/21/0/38/30
    (13 nested); Main flow items 0/2.
  - freqtrade: 119/122, 44/78, 7/8, 7/12, 6/19; extras 3/11/0/79/20
    (13 nested); Main flow items 0/5.
- **Against audit.py on the same runs:** flow link, names and wording checks
  identical. Fix 0: redis commands trap hits 12 → 0 (7 loglevel/appendfsync
  values, the 4 DEBUG encodings, SORT's `limit`), litestream request extras
  26 → 13 (MCP tool parameters), freqtrade request/command extras 9/18 →
  3/11. ReachedFrom: redis external 0/3 → 3/3 (+2 may), extras 5 → 0.
  Unported anchors: litestream Azure Blob Storage and heartbeat ping,
  freqtrade Discord webhook POST found → missed; litestream external extras
  26 → 38. Flow items leave the workers `may` counts; aeMain (anchored at its
  call in main) and freqtrade's loops are missed: the steps sit at the
  callee's declaration (ae.c:375) or the new freqtrade flow stops at
  `start_trading`.
- **Against the errata baseline (the 10:xx runs):** redis unchanged; the
  others changed with the product (5ecf315d `cmd/litestream-vfs` +4 must,
  data extras 54 → 30; freqtrade +2 requests, +3 workers, +2 external, external
  extras 18 → 79, flow 9 → 7 steps).

## 2026-09-29 — Go programs built with tags; test code reaches no catalogue

- **Why:** litestream's Makefile builds `./cmd/litestream-vfs` with
  `-tags vfs,SQLITE3VFS_LOADABLE_EXT`; the run's own load saw no program
  there, so the report's "litestream-vfs" was the npm wrapper and 22 must
  items (the VFS PRAGMAs and SQL functions, `LITESTREAM_REPLICA_URL`, its
  workers and data) had no page. freqtrade's
  library showed `tests/` rows among its outbound calls and data, and
  litestream's data held `*_test.go` SQL (32 of its 54 extras).
- **Tagged programs (5ecf315d, DISCOVERY "Go programs built with tags"):**
  gofacts reads makefile recipe lines (variables the makefile sets outside
  conditionals expanded) and goreleaser builds for `go build`/`go install`
  with `-tags`, `-C`, a `cd`, `GOOS`/`GOARCH` and package arguments. A
  described package the run's load makes no program is loaded again with
  the line's tags for the line's platform (the run's, then the host's, then
  as written) and becomes a Go executable target with `build_tags`,
  `build_platform` and `build_sources` in its identity, the ordinary key,
  its own prepared workspace, a `go_build_tags` portfolio observation and a
  main-file hypothesis. litestream: Makefile:17 builds for the run's
  `windows/amd64` (platform evidence), where cgo is off and `main.go` is a
  cgo file; Makefile:43 (`GOOS=darwin GOARCH=amd64`) is the host's and makes
  it a program: 2 extra `go list` loads, the other three lines are not
  loaded.
- **cgo (14f93036):** the dynamic handoff index refused the column-less
  `//line` declarations of cgo's `_cgoexp_*` and `_cgo_cmalloc`, which the
  direct call index accepts, so any Go target with cgo in its own packages
  failed ("seal Go dynamic handoff index: invalid function").
- **Test code (4b0e4394, 4445bcd9, READING "Boundaries"):** the reading's one
  test rule, its adapter's `TestSources`, now drops every boundary a test
  file makes before any question and every data object in a test file. An
  extraction in a code file belongs only to the programs holding the file
  (eef3a98e's rule, which a root target's path claim bypassed), so a test no
  load selects (other programs' tests, `vfs`/`chaos`/`soak` tags,
  `tests/integration`) is no program's data.
- **Fixtures:** the Go fixture's Makefile builds `cmd/vfs` with
  `-tags $(VFS_TAGS)`; `cmd/vfs` and `root_vfs.go` build only with
  `fixturevfs` (TestCumulativeGoProgramBuiltWithTagsFromItsMakefile). Each
  language fixture's test file writes `CREATE TABLE test_only_rows`, and
  `root_optional_test.go` (only `repomap_optional_tests`) too
  (TestCumulativeTestCodeReachesNoOutboundOrData; Go data only, Clojure call
  only, C records no testing sources). Mutations fail both tests.
- **Acceptance (binary 994141c4a9bc, commit 5ecf315d; `make test` and
  `make vet` green):**
  - litestream `--no-model --target x` lists `cmd/litestream-vfs
    (executable_package, -tags SQLITE3VFS_LOADABLE_EXT,vfs for
    darwin/amd64; github.com/benbjohnson/litestream@.::…/cmd/litestream-vfs)`.
    Its no-model run: exit 0, 7.6 s, 36 files, 2,203 objects (249 in
    `vfs.go`, which only `vfs` builds; the seven exported `GoLitestream*`),
    eight `LITESTREAM_*` environment reads in `main.go` among 17.
  - litestream library, no-model: data 1,143 → 18, from test files
    1,091 → 0 (the other 34 were `cmd/` programs' and `_examples` code).
  - freqtrade library, no-model: outbound 25 → 20 (`tests/` 5 → 0), data
    71 → 35 (`tests/` 36 → 0).
  - litestream ordinary online run: exit 0, 1m07s, 139 live and 118 cached
    calls (1m52s provider time), 7/9 targets (the two C targets fail as
    before: `Python.h`, the generated `litestream-vfs.h`). Pages 6 → 7:
    `cmd/litestream-vfs` has three parts (Platform internals, VFS core, VFS
    C bindings), the `extension litestream` input
    (`sqlite3vfs.RegisterVFS`), `LITESTREAM_REPLICA_URL` and the other
    settings, and an orientation line naming them. cmd/litestream's data
    40 → 16, from tests 24 → 0; no page has an outbound or data row from a
    test file.
  - Open: the PRAGMA names are the cases of `vfs.go`'s `FileControl`
    switch on `pragmaName`, and the SQL functions are registered in
    `src/litestream-vfs.c`, whose C target fails on the header `go build`
    generates; neither is an input yet. The vfs page has no outbound call or
    data of its own: the shared module folded into cmd/litestream carries
    `db.go`'s statements.
  - Open: `atlas_systems` is no registered semantic stage, so its live
    exchange warns `stage=unknown code=artifact_write_failed`; unrelated to
    this batch.

## 2026-09-29 — Go field reads and writes; an outside field store names the field

- **Why:** Go recorded no field access (GO's gap after the C field pass,
  950d3064), so litestream had no settings "uses" layer and no field walk;
  and `fs.Usage = c.Usage` was projected as a construction of `flag.FlagSet`,
  so the model was asked what a callable handed to `flag.FlagSet` becomes and
  answered `command` for litestream's `X.Usage` rows.
- **Field reads and writes (GO "Field reads and writes"):** every selector
  in a function body naming a field of a package-level struct of the
  target's packages is one exact `reads`/`writes` relation to the field
  object, sited at the field's name, with the C adapter's `field_path` rules:
  the package variable the chain starts from, else the struct type declaring
  the chain's first named field, then each named field, elements,
  dereferences and implicit embedded steps left out. `=`, compound
  assignments, `++`/`--`, a range clause's `=` and an array element there
  write; a struct value a further field is taken from, and an array indexed
  on the way to one, passes through; everything else reads.
  `DirectCallIndex.FieldAccesses` (version 16) records them in the BFS where
  a node's calls are recorded; role and path come from the SSA function's
  typed syntax, since SSA lifts locals (`e.key` came out as
  `serverState.db.key`) and computes `x.f`'s address twice for `x.f += v`.
  The adapter joins each to the core field object. No ProgramIndex format
  change; places keep them as `fields`, so no model request changes.
- **Outside field store (fix 6):** `godynamichandoff.Slot.Assigned`
  (version 10) marks a callable binding whose field a selector names. Such a
  store into an outside value's field projects to the outside field
  (`flag.FlagSet.Usage`, signature `func()`; `net/http.Server.Handler`,
  `http.Handler`), no invocation, a `go_field_store` witness, one relation per
  store, word = the field. The registration fact, the places call API and the
  reading's `outside_symbol`/`declared` follow. A composite literal still
  constructs its type (`testing.InternalTest`, `&cobra.Command{…}`). C still
  names `act.sa_handler = onSignal` by `struct sigaction`; Python and JS/TS
  hand nothing over by such a store (README table).
- **Fixture:** `internal/storefixture/server_state.go` (kvd's server state
  in Go), `tool_cli.go`'s `toolCommand.Run` (`fs.Usage = c.Usage`) and
  `ServeStateStatus` (`srv.Handler = mux`), appended to tool_cli.go and in
  the new file so no pinned line moves; `go.files.json` gains the new file.
  `assertGoFieldAccesses` and `assertGoOutsideFieldStores` in
  `TestCumulativeGoRepositoryDiscoveryAndProgramIndexContract`; the store
  check fails with the assignment test disabled.
- **litestream v24, no model:** cmd/litestream (exit 0, 8 s) 2,843 field
  accesses (2,277 reads, 566 writes); `Store.dbs` written by
  `Store.RegisterDB` (`s.dbs = append(s.dbs, db)`, store.go:313) and
  `Store.UnregisterDB` (store.go:345), read at 16 sites by 13 methods
  (`NewStore`'s `&Store{dbs: dbs}` is a literal, no write);
  `DB.MonitorInterval` written by `NewDBFromConfig` (main.go:751) and
  `ReplicateCommand.Run` (replicate.go:284), read by `DB.Open` and
  `DB.monitor` (twice). cmd/litestream-test (exit 0, 4 s) 240 field
  accesses (179 reads, 61 writes). Registrations of `fs.Usage = c.Usage`
  now name `flag.FlagSet.Usage` (declared `func()`): 14 in cmd/litestream,
  6 in cmd/litestream-test; none names `flag.FlagSet`.
- **Contracts:** GO (new "Field reads and writes", the store paragraph in
  "Receiver fields and bindings", missing equivalents), PROGRAM_INDEX,
  READING, REPORT, PYTHON (cross-reference), CURRENT, testdata README.
- `make test` (package parallelism 2): every package passes but
  `internal/extractors`'
  `TestCumulativeDataPreservesORMOwnershipUnknownScopesAndSQLSources` (go
  and jsts: "SQL declarations=2"), from the test SQL 4b0e4394 added to the
  go and jsts fixtures' test files, outside this change. `make vet`: PASS.

## 2026-09-29 — Where an outgoing call is reached from (claim audit fix A)

- **Why:** an outgoing call written in a shared helper named only its caller
  one hop inside the helper's own file: Redis's `connect` (anet.c:158) and
  `gethostbyname` (anet.c:115/146) read "Called from anetTcpConnect,
  anetTcpNonBlockConnect" with line pairs, never syncWithMaster,
  cliConnect or createClient (root cause: `claim-audit/rootcause/
  external-data.md`, fix A).
- **Change:** GroupsIndex 26 (de051380) saves `OutboundCall.ReachedFrom`, a
  code fact at projection: exact `calls` followed back per program through
  the call's own part; each path ends at the first caller in another part
  with its call site, or where callers run out inside the part only at a
  seed or an input's handler (else dropped); test sources and callers the
  adapter proved never run are skipped; cycles stop, no depth cap; no model
  request changes (READING § Outside systems). The tile's reading
  (dd0087f0) shows "Called from" in place of the one-hop callers: every
  record the tile stands for, its own program first, each part in its box
  with its callers by name, another program's part named with its program,
  no line numbers or code marks, a name reading its function in the column
  and on the canvas, more than twelve folded under the count (REPORT §
  External communication and data).
- **Fixtures (a8f094e1):** C's client gains `repl.c` (without a command,
  each input line over its own connection), so net.c's `netConnect` is
  reached from main (kvcli.c) and repl (repl.c): held end to end by the kvd
  preset reading. Go `DestinationRequest`, Python and TypeScript `dispatch`
  already existed and are held over their real ProgramIndex with the parts
  given by the test. **Missing:** Clojure has no sending helper with two
  callers (`revision`'s git is called by nothing).
- **Skipped:** fix D (declared-on for outgoing calls, "configured at …"). It
  is no one line per destination: freqtrade's "cryptocurrency exchanges"
  spans Exchange's `_init_ccxt` object and exchange_ws's parameter store,
  SQLAlchemy sessions differ by module, and C's object is the `socket()`
  call, which configures nothing.
- **Acceptance (binary of a8f094e1, default cache, ordinary runs):**
  - Redis 20260929-140032: exit 0, 15 s, 0 live calls. t1 connect and
    gethostbyname@146 ← Replication: syncWithMaster (redis.c:7219); t2 ←
    Benchmark client: createClient (redis-benchmark.c:343), @115 ←
    parseOptions (:430); t4 ← Command line client: cliConnect
    (redis-cli.c:179), @115 ← parseOptions (:386). The shared
    gethostbyname@146 tile reads all three under their parts
    (`look/b4a-redis-dns-*.png`). The "TCP endpoint" tile is redis-server's
    master only: redis-cli's and redis-benchmark's connects are joined to
    redis-server's input and drawn into it, so their callers are on that
    arrow (its call's side names cliConnect), not on the tile
    (`look/b4a-redis-tcp-*.png`).
  - freqtrade 20260929-140146: exit 0, 321 s, 0 live calls. t1: 170 of 213
    outgoing calls have callers, 875 in total, at most 38 names on one call
    (Telegram's send_message, reached from 38 command handlers of its own
    part); exchange calls reach 36 functions in 6 parts (fetch_l2_order_book
    23, folded, `look/b4a-freqtrade-orderbook-*.png`). webhook.py's
    `requests.post` has none: RPCManager calls each module's `send_msg`
    over a list, which no exact call resolves.
  - Renders: `redis-r2/run/latest-{redis,freqtrade}.html`.

## 2026-09-29 — Python: inherited self calls and fields, what a call produces, partial

- **Why:** the claim audit's root-cause pass (external-data, workers) found
  freqtrade's ccxt client, Telegram bot and Discord sender lost in the Python
  adapter: 260+ `self.*` calls unresolved, `self._api.*` with no outside
  symbol, `Application.builder().token().build()` stopping after `builder`,
  `partial(self._force_enter, …)` handing nothing to `CommandHandler`, and
  Webhook's address never showing Discord's store.
- **Change (6fd2ee33, PYTHON "Inherited members and fields" and "What a
  call produces"):**
  - `self.helper()` is the method the class declares or inherits along its
    chain of single repository bases (the contract already promised it).
  - A field stored once holds the outside type its store gives it: an
    outside call, a repository factory's declared outside return type
    (`_init_ccxt(...) -> ccxt.Exchange`), a parameter annotated with an
    outside class (`ExchangeWS(…, ccxt_object: ccxt.Exchange)`), or the
    outside call an untyped factory's one return statement returns (P4,
    exact only; PYTHON's "untyped factory returns gain no receiver
    authority" was agent caution from 09-11, rewritten under the owner's
    09-16 anti-hedging rule). A subclass sees its base's field; a store in a
    class deriving from the reader is a second store. `typing.Any`/`Self`,
    unions and coroutine functions give no outside class.
  - A call on a call's result is a member of what that call produces,
    chains included; the same holds for a name bound once to such a call.
  - `functools.partial(f, …)` given as an argument hands `f` over.
  - A base-class method's `self.<field>` source value lists the `__init__`
    store of each class deriving from it as `alternatives`; destination
    reading (`storedValue`) follows each.
  - **Bug:** an `import X` written after another module imported `X` bound
    `X` as a repository name, so `X.attr` resolved only where an earlier
    module had resolved the same attribute. Alone it adds 220 freqtrade
    outside symbols (numpy 41, asyncio 33, torch 30, pandas 16, ccxt 15, …)
    and was what kept `ccxt.Exchange` unresolved in `exchange.py`.
- **Chains counted first (no-model ProgramIndex, outside symbols):**
  freqtrade 777 → 1,219 (+442; 308 invoked outside tests): import fix +220,
  inherited/factory/parameter fields and `self` calls +83, a call on a
  direct call result +96, longer chains +43 (sqlalchemy 13, telegram 12,
  pandas 8, pathlib 4, …). Restricting chains to expression statements
  would have kept 3 of the 43; pandas chains add 8, not hundreds, so chains
  stay unrestricted. python-tutorial-game: 21 → 21, objects and relations
  unchanged (index regenerated for its scenario digest).
- **freqtrade no-model (`python:.:script:freqtrade`, before 176e8535 /
  after 6fd2ee33):**
  - `self.*` calls outside tests: exact 1,327 → 1,634, unresolved 1,319 →
    1,012; one-attribute `self.m()` unresolved 272 → 99 (71 members of an
    outside or unknown base or of a call result, 20 callable fields, 6
    classes with several bases).
  - ccxt: 6 → 110 calls; `ccxt.Exchange.*` 0 → 104 (63 members, 11 files;
    54 outside exchange.py: 8 subclasses, exchange_ws through the annotated
    parameter).
  - telegram.py:249 `.token` and `.build` are
    `telegram.ext.Application.builder.token(.build)`; :2185
    `self._app.bot.send_message` is `…build.bot.send_message`; 11 calls on
    `…build.*` (add_handler ×2, send_message ×2, initialize, start,
    updater.start_polling, …).
  - discord.py:60 `self._send_msg(payload)` → `Webhook._send_msg`
    (webhook.py:114); webhook.py:129/131/133 `requests.post(self._url…)`
    take webhook.py:34 and discord.py:20 as alternatives.
  - `partial` hands over 9 callables (7 outside tests): telegram.py:276/279
    `_force_enter` to `CommandHandler`, arguments.py:481/490
    `start_convert_data` and :585/594 `start_list_markets` to
    `set_defaults`, exchange_ws.py:174 `_continuous_stopped` to
    `add_done_callback`.
- **Fixtures:** python `inherited_clients.py` and `outside_results.py`
  (`TestCumulativePythonSelfCallsAndFieldsFollowTheBaseChain`,
  `TestCumulativePythonBaseReadTakesEachSubclassStore`,
  `TestCumulativePythonCallsOnCallResultsAndPartial`); events.py's
  direct-result and chained `subscribe` expectations now resolve. Go and
  TypeScript already type these through their compilers (workers.go/ts,
  `router.HandleFunc(...).Methods`, `createConsumer().on().on()`); JS
  `bind`, Clojure `partial` and TypeScript's subclass stores are recorded
  missing equivalents (JSTS, CLOJURE); C has no classes.
- **Ordinary run** (6fd2ee33 binary, default cache, `--no-serve
  --no-open`, run 20260929-132755): exit 0 in 972 s, 355 live calls, 618
  cached: atlas_boundaries 115 (one provider call took 10 min), atlas_api
  78, atlas_role_helper 53 (287 cached), glossary 28, atlas_symbols 27,
  atlas_keys 17, atlas_areas 9, atlas_describe 7, atlas_joints 7,
  atlas_core 6, atlas_role_assign 2, orientation 2, atlas_publish,
  atlas_role_gate, atlas_systems and atlas_zones 1 each. Its ProgramIndex
  gives the same numbers as the no-model run; report.json names
  `ccxt.Exchange` 126 times and `…build.bot.send_message` 4 times.
  rejected.jsonl 264 → 402 rows: glossary 28 → 142, atlas_api 0 → 22 (78
  new windows), atlas_role_helper 218 → 222.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make build`.

## 2026-09-29 — Follow-ups of the REPORT and READING trims

- **Dead catalogue click (b200c8f4):** `rmCatalogInputClick` and its capture
  listener read a catalogue input row (`.input-catalog [data-input-item]`);
  no template prints one (the catalogue holds only a link to the Inputs list
  and the outbound groups), so both are deleted with
  `TestACatalogRowReadsItsInputOnAPlainClick` and the `REPOMAP_REAL_RUN`
  journey that clicked such a row (5 real-run journeys remain). The CSS
  rules and the `[data-input-item]` query in 28-map-routing.js for those rows
  are dead too and stay for a later cleanup.
- **No static picture (d3a91ccc):** `buildZoneMap`'s `mapEdges` arrows were
  appended to each target's map as `static` edges, which the system map (the
  page's one figure) and the canvas script both skipped; they reached the
  page only as display-text catalogue entries, each a label joining every
  connection between two areas (one Redis label joins about 250 relations).
  Deleted with the edge router (its other use routed operation-map paths
  the system map then cleared), the dead constants and edge fields, the
  no-script comments and the static half of
  `TestTheMapQuietsWhatGroupsIndexMarksQuiet`; the canvas half stays. The
  zone layout's frames stay (cards link to them).
  - `repomap render` of the saved Redis (20260929-102019) and freqtrade
    (20260929-101431) runs at b79ef0f2 and after: the HTML differs only in
    the deleted script and the text catalogue, which loses only joined
    static labels; the highest ref on the page moves t6076 → t6065 (Redis)
    and t12718 → t12709 (freqtrade). A translated run no longer sends these
    labels; a translated saved run from before no longer rebinds in `render`
    (its catalogue has the extra entries).
- **Translation rules (c643d287):** REPORT's translation request, window,
  refusal and deadline rules (606 words) moved verbatim to EXECUTION
  "Display translation"; REPORT keeps what the reader sees and points there.
- **CURRENT:** Airflow's Freqtrade prerequisite is met; the owner items
  "number chips" (removed, f703fb14) and "area order" (a091e211) are closed.
  The `none` option of `prompts/entry_options.md` lacks READING's "wraps or
  stores it"; the criteria are in every entry question's memo identity
  (`table.questionsDigest`), so the fix would ask every `binds`, `enters` and
  `becomes` question again: recorded as a known drift instead.
- **Checks:** `make test`, `make vet` (package parallelism 2), `make ui-test`
  (133) and `make ui-visual-test` (64 passed, the 5 `REPOMAP_REAL_RUN`
  journeys skipped) pass.

## 2026-09-29 — READING trimmed back to a contract: 18,391 → 13,969 words

- **Why:** READING grew from 9,740 to 18,391 words in 2.5 days with run
  stories, repeated rules, restated prompt criteria and test inventories.
- **Removed:** history and run numbers down to "(owner, date)" tags; 29
  repeated statements merged into their owning section (symbol-selection
  columns, concurrency, the `open` cell, question-only recall, the Jev-only
  tables, parts-answer repeats and conflicts, the role-split task, test and
  never-run parts, question budgets, answer states); option and answer
  criteria that restate the embedded prompt files, now pointers to them (the
  closed choice sets, what each choice leads to, each Jev question's text
  and the decoder rules stay); test names and fixture case lists; mechanics
  the code states.
- **Stale passages removed, the newer rule and the code kept:** symbol
  selection asks `key_symbol` only (no activation, no `unassessed`); the
  operations table and its v19 cells (gone since 2026-09-17); the boundary
  candidate/decision/basis cells (`fixed` is always true); request-local row
  refs where a table has no artifact ID; the glossary's 32,768-token
  allowance; "saved" per-input paths (`Reach` is not persisted); old format
  versions; a program the repository builds itself is joined by equal names
  (`target.executables`), not "not done"; `binds` includes `setting`.
- **Checks:** an independent reviewer compared the draft with the original,
  the pointed-to prompt files and the code; its nine findings are restored
  (the recorded missing fixture equivalents, an entry's line and name failing
  alone, an empty text taking its no-decision value, two type-description
  statements, the one cross-target wait, the request-local ref examples, the
  role-split option sources, `binds` `none` for a symbol that wraps or stores
  the callable, welcome deductions). Headings and anchors are unchanged. The
  cut is 24%, short of the 30% aimed at.

## 2026-09-29 — REPORT trimmed back to a contract: 19,550 → 16,026 words

- **Why:** REPORT grew from 7,920 to 19,550 words in 2.5 days, with run
  stories, repeated rules and layout mechanics. AGENTS.md: contracts hold the
  rules; run evidence is here.
- **Removed:** history ("had", "no longer", Redis/litestream stories, run
  counts) down to "(owner, date)" tags; repeated statements merged (map key,
  declaration reading, input reading, name links and no line numbers, one
  Main flow link, translation windows and failures, source links and
  serving); mechanics the code states (ELK candidates and passes, measured
  dimensions, React Flow internals, the page-data field encoding, CSS sizes,
  colours and timings); the UI iteration text AGENTS.md and DEVELOPMENT.md
  already state; the term-lookup rules TERMINOLOGY states.
- **Newer rule kept where passages disagreed:** a declaration's name is its
  code link and its file stands alone (no "Open code ↗" with file:line); an
  input's reading opens at how a request reaches it, the dispatch-site boxes
  only when no way is known; the page writes no static picture; question
  answers and their evidence are HTML while every column reading is page
  data; the catalogue input row's "To explanation" click is gone (no
  catalogue row is printed).
- **Checks:** an independent reviewer compared all 169 original paragraphs
  with the draft and against the code; its seven findings (one caller per
  call, a card-closing click doing nothing else, reading a name in its own
  part, a test-only part is a fact, the served report's only addition, the
  page-data read-back test, three layout prohibitions) are restored. Headings
  and anchors are unchanged. Short of the 30% aimed at: what remains is
  reader-visible rules.

## 2026-09-29 — Report tests reduced to what they protect (260acd7f)

- **Why:** an independent critic found tests in internal/report pinning
  rendered strings and pixel sizes (`'Programsredis-serverBackend database
  serverEntrymain()…'`, `serverCron` 87 times). Owner rule: delete tests
  that pin incidental layout, keep and reduce the ones guarding a real
  requirement.
- **Change:** 5 tests deleted (the part-code list's CSS box, "possible" in
  the muted colour, the removed-words vocabulary guard, two card heights
  equal to the sizing formula's constants); about 80 assertions reduced or
  deleted in 16 files: concatenated textContent, class lists, markup
  snippets, colours, opacities, stroke/head/dash/corner pixels and card
  positions became names in order, sections in order, counts, "said
  unknown", "no line number", "darker than its own outline", "the same on
  screen at both zooms" and "which side of its frame". 206 lines added,
  218 removed.
- **Checks:** 8 mutations of the page scripts and layout code (a line
  number in a declaration's file line, doubled Reads/Writes, callers before
  members, the dropped "+N" and "not established", the depth-2 fold, a
  description wrapped at 228 px, the card gap's sign) each fail a reduced
  test. `make test`, `make vet` (package parallelism 2), `make ui-test`
  (133, 135 before) and `make ui-visual-test` (64 passed, 6 real-run
  skipped) pass.

## 2026-09-29 — moved from CURRENT: run log and decision details

These passages were moved verbatim from CURRENT when it was trimmed back to decisions and current acceptance state.

### Acceptance and open work

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
| Python constructors (2026-09-29) | A repository class call is `construct` and also calls its own or inherited `__init__`; a name bound to it (a None store aside) types its calls, inherited methods included (PYTHON). freqtrade at 19daee7c, default cache: exit 0 in 304 s, 179 live calls; `worker.run()` is Worker.run and Reach("trade") holds Worker.run (377 callables); FreqtradeBot.process is still outside it, behind `_throttle(func=self._process_running)`. The re-asked parts answer drew 42 parts (22 before). |
| Report batch, "b", compaction and flow (2026-09-29) | Runs at 68d6d4c5 on the default cache, rendered at 656e8a16: Redis exit 0 in 11 s (0 live), 28.96 → 4.27 MB; litestream v24 exit 0 in 74 s, 9.23 → 3.92 MB; freqtrade exit 0 in 319 s (orientation refused by context size), 46.54 → 9.30 MB; self-snap exit 0 in 124 s, 37.34 → 9.72 MB. `make test`, `make vet`, `make ui-test`, `make ui-visual-test` pass; headless walks without page errors (CHANGELOG). |
| Fix B, type takers, field readers/writers, own-executable join (2026-09-29) | Runs with the e04743b1 binary, rendered at 61c9dd00, on the default cache, no `cache clear`: Redis exit 0 in 35 s, litestream v24 exit 0 in 63 s, freqtrade exit 0 in 260 s (orientation refused by context size, as before), self-snap (e04743b1) exit 0 in 116 s. Redis's 4 undecided units (dupClientReplyValue, dupStringObject, lookupKeyRead, convertToRealHash) became rows the parts answer put in Server core state, no follow-up, no lone part; iojob placed by rule C with freeIOJob/queueIOJob in Virtual memory; litestream's 2 and self's 2 joined existing parts. litestream-test's 3 `litestream` launches draw an arrow into cmd/litestream; cmd/litestream's 9 self-launches keep their tile and name it. Pages: Redis 4.31 MB, litestream 3.92 MB, freqtrade 9.41 MB, repomap 9.97 MB. `make test`, `make vet` (package parallelism 2), `make ui-test` (134), `make ui-visual-test` (64 passed, 6 skipped) pass; headless walks without page errors. |
| Joined files' imports, Connections headings (2026-09-29) | At 66902610 on the default cache, rendered at the same commit: Redis exit 0 in 34 s, one live parts request (redis-server, `"c4 -> f13"` its only change): pqsort.c now in Sort command, lzfP.h in Persistence (RDB and AOF), 19 parts, no other part changed. litestream v24 (6/8, the two C targets fail on missing headers as before), freqtrade (10/10) and self-snap (2/2) ran every atlas request from the cache; only orientation went live where its first packing is refused by context size, as before. litestream's Connections: no repeated heading or row naming its heading (13 and 30 before). `make test`, `make vet` (package parallelism 2), `make ui-test` (135), `make ui-visual-test` (64 passed, 6 skipped) pass; headless walks of the four renders without page errors. |
| Freqtrade | Latest larger run stopped after repeated 16k retrieval refusals; no accepted final report for this wave. A corrected ordinary run with worker, destination, question and data checks is required. The 2026-09-28 cold run completed (430 s) with the orientation refused by context size. |

### Current decision (introductory paragraphs)

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

### Registrations (2026-09-17; entries 2026-09-27)

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

### Map of parts (2026-09-25, owner's proxy spec; the owner's open questions 1–7 of that spec still stand)

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

### Map model step 4 (2026-09-28): one reach

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

### Follow-ups of sanity check 3 (2026-09-28)

- **Follow-ups of sanity check 3 (2026-09-28).** The no-users rule applies
  only to a kind whose uses the adapter records (`recordedUses`, per
  language and kind): Go variables and Clojure macros (ProgramIndex
  `macro`) are asked like types. A Go package-level variable is the caller
  of its initializer's calls (direct-call index 15). A call through a stored
  function value is no user for placement, the other half of a hand-over.
  The second pass repeats rounds until no waiting helper qualifies; a
  helper no round could ask is off the map as `blocked`, apart from
  `undecided`.

### Owner decisions of 2026-09-27/28 (map model step 3)

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

### Lead decisions awaiting the owner's review (2026-09-28)

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

### Map reading on the canvas (2026-09-25)

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

### The page needs JavaScript (2026-09-29, owner decision "b")

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

### Decoders (2026-09-26, owner: "валидаторы и строгие декодеры нам уже 30 дней палки в колеса вставляют")

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

### Operations

- **Operations:** v19 asks whether evidence supports this declaration's task and
  its independent activation (`self`/`none`).

### Questions and orientation

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

### Orientation stage 2 (2026-09-29)

- **Orientation stage 2 (2026-09-29):** an overview (facts, claims,
  connections, groups without members, seed rows) answers summary, roles,
  recipe and `main_flow_target`; a second request asks that target's main
  flow over Launch ∪ Reach ∪ hand-overs, every member complete as a lossless
  tuple row in reading order. The member ladder is gone. Accepted on
  freqtrade (flow 760,231 input tokens, into Worker.run and _worker), Redis
  (main → aeMain → processCommand → call, `[/path/to/redis.conf]`) and
  litestream (LITESTREAM env and flags); warm reruns make no live call.
  self-snap's flow request exceeds the window and is journaled under
  `flow_request` while its overview stands.

### Glossary

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

### Where an input takes effect (consilium 2026-09-28, owner answers Q1 and Q2 yes; built in speed mode, tested in the repair pass)

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

### Field readers and writers (2026-09-29)

- **Field readers and writers (2026-09-29):** a record type's reading
  gives each field "Written by" and "Read by", the functions by part; a
  global variable's lists the fields reached through it
  (`server.masterhost`); a function's says "Writes: …" once and
  "Reads: …" once, the fields by their paths and the global variables it
  reads, in place of "Uses variables". No column list prints a line number:
  a caller, callee or variable is its name, once (REPORT). From C field
  paths, Python typed receivers and JS/TS declared properties.

### Benchmark v3 fixes (2026-09-29, REPORT)

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

### Benchmark v4 fixes (2026-09-29, REPORT)

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
  declarations reached from outside it first, counted. A name is its link;
  the page prints no separate code marks. The canvas location row's frames go up a
  level; every reading ends with the home's text-page links.

## 2026-09-29 — report.json names the other targets' program-index.json: freqtrade 354 MB → 14.7 MB

- **Why:** 336 MB of freqtrade's format-92 report.json was the nine other
  targets' ProgramIndexes, each byte for byte the `program-index.json` of its
  own target run directory (left for the owner in the entry below; the
  owner approved naming them).
- **Change (cf5c0681):** report format 93. `files` entries carry a `path`
  from the owner run directory instead of a `name`; another target's
  ProgramIndex is `../<run-id>/program-index.json` for a run ID the owner's
  `program-page-portfolio.json` names whose `program-index-set.json` binds
  that target and seal, with the SHA-256 of the file's bytes. Restoring
  refuses a missing or changed file, and any path other than a file name
  or `../<run-id>/<file>` for a ProgramIndex. REPORT: saved restoration
  reads the files the JSON names in the run's own target directories. Test:
  both targets' indexes read back exactly; one appended byte in the other
  target's file, then its removal, refuse the report.
- **Runs** (`make build` binary at cf5c0681, default cache, `--no-serve
  --no-open`): freqtrade 101431 exit 0 in 256 s, Redis 102019 exit 0 in 9 s,
  0 live calls each; every ProgramIndex is named (0 left inline).
  `repomap render` of each equals its report.html byte for byte (freqtrade
  render 50.1 s / 2.78 GB max RSS, 53.0 s / 3.04 GB before). report.html is
  the same size as the 095735 and 100147 runs'.

  | | freqtrade 095735 → 101431 | Redis 100147 → 102019 |
  | --- | --- | --- |
  | report.json bytes | 353,833,510 → 14,716,376 | 4,230,702 → 3,040,159 |
  | owner run dir (du) | 509,168 KB → 178,052 KB | 25,516 KB → 24,356 KB |
  | with target run dirs | 849,976 KB → 518,860 KB | 26,912 KB → 25,752 KB |

## 2026-09-29 — Orientation stage 2 accepted on freqtrade, Redis and litestream

- **Two prompt sentences (429c9759, 00a8c0db):** the first cold draws kept
  the evidence but not the owner's recipe checks: Redis wrote `./redis-server
  [redis.conf]` though main's seed row carries `Usage: ./redis-server
  [/path/to/redis.conf]`, and litestream's recipe named no LITESTREAM key
  though `LITESTREAM_CONFIG` (DefaultConfigPath) is an overview fact. The
  overview prompt now says to write a usage line's arguments as it writes
  them, and to name the settings a start reads (environment keys, flags) in
  the note. Only the overview requests changed; the flow requests stayed
  cached.
- **Ordinary runs** (`make build` binary, default cache, `--no-serve
  --no-open`, exit 0 each). Actual tokens are the provider's input tokens:

| run | overview chars / tokens | flow chars / tokens | roles / recipe / flow steps | live / cached |
| --- | --- | --- | --- | --- |
| freqtrade 094714 | 1,206,953 / 353,482 | 2,431,311 / 760,231 | 10 / 7 / 9 | 22 / 764 (glossary 21) |
| Redis 094650 | 224,508 / 60,881 | 518,005 / 189,028 | 4 / 4 / 12 | 4 / 153 (glossary 3) |
| litestream 094617 | 380,124 / 106,491 | 839,797 / 269,174 | 6 / 4 / 8 | 7 / 161 (glossary 6) |

  Flow rows tokenize denser than the measure's 3.45 chars/token (2.7–3.2):
  freqtrade's flow was 760,231 tokens against ~711K estimated, within the
  1,032,192 budget. freqtrade cold is 353K + 760K input tokens against one
  712K stage-1 request.
- **freqtrade:** summary "Freqtrade is a Python crypto trading bot whose
  main program (t1) provides CLI commands …"; 10 roles; recipe `python -m
  freqtrade trade` with its `-c/--config` note. Flow "Freqtrade CLI startup
  and command dispatch": __main__.py entrypoint → freqtrade.main → main →
  Arguments.get_parsed_arg → _build_subcommands → the trade registration
  (`set_defaults(func=start_trading)`) → start_trading → Worker.run →
  Worker._worker ("starts the FreqtradeBot, throttles the running
  process"). FreqtradeBot.process is in the request (member #1280, handed
  over) but no step cites it.
- **Redis:** recipe `./redis-server [/path/to/redis.conf]`; flow main →
  initServer → acceptHandler and serverCron registrations → main → aeMain →
  beforeSleep → readQueryFromClient → processInputBuffer → processCommand →
  call (12 steps).
- **litestream:** recipe `go run ./cmd/litestream replicate` with
  LITESTREAM_CONFIG and the LITESTREAM/AWS key pairs in its note; flow main
  → Main.Run → applyLitestreamEnv → ReplicateCommand.ParseFlags →
  ReplicateCommand.Run → NewStore → Store.Open.
- **Checks:** no orientation request carries `calls_omitted`,
  `called_by` or `callee_id`, and no payload of the three warm runs carries
  `callee_id` or `calls_omitted` (767, 156 and 164 payloads). Rendered to
  the scratchpad's `latest-{freqtrade,redis,litestream}.html`; the headless
  smoke walk (4 components, every Inputs kind, 8 parts, 16 declarations
  each) had no page error. Warm reruns: 0 live calls (freqtrade 767 cached,
  Redis 156, litestream 164), exit 0.
- **self-snap (record only, 100211):** overview accepted (320,721 tokens; 2
  roles, 4 recipe steps); the flow request (5,367,195 chars) was refused by
  the provider (`context_tokens`) and journaled under `flow_request` with
  its request bytes; exit 0.

## 2026-09-29 — Orientation stage 2: an overview, then the main flow over one target's entry-forward scope

- **Why:** the 40/20/12 member ladder sampled group members and cut each to
  6 calls and 6 callers (a known violation of "never sample"), and the flow
  cites members (22 of 22 accepted steps) while summary, roles and recipe
  cite facts and claims.
- **Change:** two requests, one prompt and response shape each. The
  overview (`overview-prompt.md`) keeps facts, claims, connections, groups
  without member lists, and each seed's complete row; it answers summary,
  roles, recipe and a closed-ref `main_flow_target`. The flow
  (`flow-prompt.md`) gets that target, its facts anchored inside the scope,
  and one complete tuple row per member of Launch ∪ Reach ∪ hand-overs
  (`scope.go`, `rows.go`). The ladder, `MaxAdvertised*`, `MaxEvidence*`,
  `lines.EvidenceLimits`/`CallableEvidenceWithin` and their tests are gone. A
  refused flow request is journaled under `flow_request`; the overview
  stands. Requests are written without HTML escaping (`->`, not `\u003e`).
  The flow answer is `{"main_flow":{title,steps}}` so the glossary reads its
  slots as before.
- **Hand-over (owner's scope extension):** the scope also takes, transitively,
  every repository callable a member hands over (`passes_callback` edge) or
  registers (registration fact owner → object), placed after the member
  handing it over. Python, Go, JS/TS, C and Clojure all emit
  `passes_callback`; registration owners come from the facts layer, so no
  language lacks the hand-over. Effect:
  freqtrade t1 +190 members (Worker._process_running, FreqtradeBot.process),
  Redis t1 +11 (readQueryFromClient), litestream t1 +32, self t1 +1,882.
- **Tuple row is lossless:** `api` is kept (`[package, receiver, name,
  signature]`): 646 of litestream's 661 api calls carry a signature and 271 a
  package path a call name does not; `values` are left out only when they
  equal the literal arguments' texts. `TestAMemberRowReadsBackEveryCallLosslessly`;
  on saved runs every flow row read back to its calls: Redis 543 rows / 3,657
  calls, litestream 494 / 3,749, freqtrade 1,618 / 10,410, self 4,338 / 27,024.
- **Measured** (real builders on the newest runs; tokens at freqtrade
  stage-1's 3.45 chars/token; budget 1,032,192):

| run | overview chars / ~tok | flow t1 members (handed over) / calls / facts | flow t1 chars / ~tok |
| --- | --- | --- | --- |
| freqtrade 085251 | 1,207,423 / 349,977 | 1,789 (190) / 9,739 / 474 | 2,453,798 / 711,245 |
| Redis 081258 | 224,468 / 65,063 | 421 (11) / 2,887 / 20 | 533,247 / 154,564 |
| litestream 071907 | 381,020 / 110,440 | 447 (32) / 3,299 / 140 | 843,627 / 244,529 |
| self-snap 072058 | 1,267,280 / 367,327 | 3,960 (1,882) / 18,741 / 831 | 5,410,586 / 1,568,285 |

  Every other target's flow is smaller. self t1 does not fit (893,020
  without its hand-overs) and is recorded only. freqtrade t1's order: main #0,
  start_trading #1269 (427 module bodies run at load before any input),
  Worker.run #1272, Worker._worker #1276, Worker._process_running #1278
  (handed over), FreqtradeBot.process #1280.
- **Tests:** `TestCFixtureMainFlowScopeFollowsWhatMainHandsOver` (kvd: every
  launch and reach callable listed, main first, acceptHandler →
  readQueryFromClient → processInputBuffer → processCommand in order, every
  member with all its calls; it fails when the closure stops at the first
  hand-over), kvd's main seed row with all 17 calls, provider bodies of both
  requests without local identities, flow refusal, no flow without a known
  target.

## 2026-09-29 — a cached answer read is marked as used

- Owner: the paid `.llm-cache` answers may never be read again; mark the ones a run reads. `readAcceptedCache` now sets the record and its request/response payloads to the read time (`markCacheUse`, internal/llm/cache.go); a failed touch is ignored. `TestACachedAnswerReadIsMarkedAsUsed` fails without it. After a few days, `find ~/Library/Caches/repomap/runs/.llm-cache -mtime +N` lists answers no run used. Same day: 608 old run dirs (22.5 GB) deleted from the default runs dir at the owner's "да"; .llm-cache (6.5 GB) and today's runs from 07:00 kept.

## 2026-09-29 — Python class calls construct and run __init__; a name bound to one types its calls

- **Why:** orientation stage 2 scopes the main flow by Reach, and freqtrade's
  stopped at `start_trading`: `worker = Worker(args)` (trade_commands.py:24)
  resolved to the type with no `construct` invocation and no `__init__`
  edge, and `worker.run()` (line 25) was unresolved, so Worker.run and
  everything under it were in no set. Go, JS/TS and C already emit
  `construct`.
- **Change (19daee7c):** a call of a repository class is a `construct` call
  of the class (keeping its pattern, result record and the facts stage's
  class reading) plus an exact `construct` call of the `__init__` the class
  declares or inherits along a chain of single repository bases
  (`constructor` witness, no pattern). A name bound to the result types a
  call on it through the same chain, inherited methods included; a store of
  `None` does not count (None has no method), a second other store still
  leaves it unknown. Destination reading takes the two calls at one site as
  one: the class call's arguments bind the constructor's formals.
- **Fixture:** python `workers.py` (own, inherited and absent `__init__`,
  the None store, a name stored twice) with
  `TestCumulativePythonConstructorCallsRunInitAndTypeTheirName`; jsts
  `workers.ts` (`TestCumulativeJSTSConstructorCallsReachTheirConstructor`:
  the compiler already resolves own and inherited constructors and methods;
  `new Plain()` with no constructor stays an unresolved construct call) and
  go `workers.go` (`assertGoConstructedWorker`: NewWorker is an ordinary
  call, the promoted `baseWorker.Run` exact) as equivalents, both already
  handled; Clojure (constructor functions, protocol dispatch unresolved) and
  C have none. Inventories updated; the tutorial-game backend index is
  regenerated through a no-model run (10 class calls gain `construct`, no
  `__init__` edge: its classes are dataclasses).
- **freqtrade no-model (`python:.:script:freqtrade`, 54 s):** line 24 is
  `calls exact construct → Worker` and `calls exact construct →
  Worker.__init__` (worker.py:31); line 25 `worker.run()` is `Worker.run`
  (worker.py:76), line 29 `worker.exit()` `Worker.exit`; in `Worker._init`,
  `FreqtradeBot(self._config)` reaches `FreqtradeBot.__init__`.
- **Ordinary run** (`make build` binary, default cache, `--no-serve
  --no-open`): exit 0 in 304 s, 179 live calls, 661 cached (atlas_describe
  37, atlas_keys 31, atlas_symbols 26, glossary 26, atlas_boundaries 23,
  atlas_areas 17, atlas_core 6, atlas_role_helper 6, atlas_role_assign 2,
  atlas_zones 2, atlas_api 1, atlas_role_gate 1, orientation 1). The
  orientation was accepted (10 roles, 3 run steps, 8 flow steps, 1 row
  rejected). The re-asked parts answer drew freqtrade's map in 42 parts
  (22 before; 19,195 grouped members either way).
- **Stage-2 measurement** (`measure.go`, same ten run dirs): Reach(o32
  "trade") is 377 callables (at most 131 before, below the old top eight)
  and holds start_trading, Worker.__init__, Worker.run, Worker._worker,
  Worker._throttle and FreqtradeBot.__init__. FreqtradeBot.process is still
  in no set: `_worker` hands `self._process_running` to
  `_throttle(func=…)`, which calls `func(*args, **kwargs)`, a callback
  handed over (never followed) and a call through a parameter (unresolved in
  Python). t1 launch+reach 1,117 → 1,365 callables, 6,931 → 8,816 calls,
  callsOnly 3,126,652 → 3,992,405 chars; all ten targets 1,215 → 1,464
  callables, 7,575 → 9,487 calls, callsOnly 3,400,801 → 4,276,530 chars
  (~0.97M → ~1.22M tokens at 3.5). hyperopt's reach 296 → 407, backtesting
  282 → 383.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  build`.

## 2026-09-29 — A call's stores no longer reach provider bodies

- **Why:** `atlas.SymbolCall.Stores` (where the code first stored each
  function a call through a field or a name reaches) is documented as local,
  but `EvidenceCatalog.call()` cleared only `CalleeIDs`. Redis's cached
  orientation requests carried `"callee_id":"sym:redis.c:1398:beforeSleep"`
  (2 of the last 400 payloads: a631ad43, 4744e600), and kvd's Jev
  `atlas_symbols` questions carried `"callee_id":"s17"`.
- **Change:** `call()` clears `Stores` with `CalleeIDs`; every provider row
  built from a call (tables, questions, orientation, Jev) goes through it.
- **Check:** `TestProviderBodiesCarryNoCanonicalIDsOrHostPaths` reads the C
  fixture's kvd and kvcli with presets, runs the orientation, and refuses
  any text-model, Jev or orientation body carrying `"callee_id"`,
  `"callee_ids"`, `"stores"`, `"place_id"`, `"object_id"`, `sym:`, the
  fixture's host path or the checkout's; it fails with the old `call()`.
  The Echo preset reading checks the same over its bodies. A scan of all
  26,895 cached payloads found no other ID key; host paths appear only
  inside repository documents quoted as written (self-analysis READMEs,
  airflow logs), which DISCOVERY allows.

## 2026-09-29 — Run artifacts written compact and once: report.json 1.05 GB → 350 MB

- **Why:** every ordinary freqtrade run wrote a 1,049,791,466-byte
  report.json (one filled the disk). Measured: 608.7 MB of it was
  indentation; 59.5 MB the owner's `program-index.json` and 9.1 MB
  `facts`/`claims`/`glossary`/`orientation`, each byte for byte a file of the
  same run directory; a witness or pattern repeated its relation's location
  (freqtrade 39,368 of 57,992 witnesses, 16,350 of 35,735 patterns; Redis
  7,152 of 8,217 and 3,032 of 3,032, with 150 witnesses located nowhere).
- **Change:** report format 92 is compact and writes each ProgramIndex in its
  artifact encoding; a section equal to a run-directory file is `files`
  (name + SHA-256 of its bytes) and is read from that file, refused if the
  bytes changed. ProgramIndex artifacts write a witness's or pattern's
  location equal to the relation's as `{}` (never a real location; old
  artifacts read the same; seal and version unchanged, so every model
  request and cache key is unchanged). `places.json`, `atlas.json`,
  `knowledge.json` and the glossary files are compact; `reading-input.json`
  (version 20) names `places.json` instead of repeating the graph. The
  unused `reportserver.decodeReportJSON` is gone. Tests: ProgramIndex and
  report round trips reproduce every relation, witness, pattern and section
  exactly (the read-back entry order and the location restore fail when
  reverted); a reading input naming `places.json` reads back the same input.
- **Runs** (default cache, `make build` binary, `--no-serve --no-open`):
  freqtrade exit 0 in 251 s, 0 live calls; Redis exit 0 in 8 s, 0 live.
  `repomap read` of the new freqtrade reading-input.json `--through
  directories` exit 0 (17 live caption windows: the ordinary run keeps
  directory captions off).

  | bytes | freqtrade before → after | Redis before → after |
  | --- | --- | --- |
  | report.json | 1,049,791,466 → 350,364,579 | 24,071,806 → 4,251,973 |
  | places.json | 94,504,222 → 45,561,738 | 8,459,186 → 3,552,267 |
  | reading-input.json | 100,609,250 → 44,942 | 9,049,008 → 662 |
  | knowledge.json | 29,904,303 → 16,201,291 | 2,731,053 → 1,605,447 |
  | program-index.json | 59,518,243 → 55,837,724 | 4,259,431 → 3,857,372 |
  | glossary.json | 7,518,959 → 4,064,739 | 2,306,984 → 903,962 |
  | owner run dir (du) | 1,358,232 KB → 505,720 KB | 63,688 KB → 26,884 KB |
  | with target run dirs | 1,717,676 KB → 843,568 KB | 65,220 KB → 28,280 KB |

  `repomap render` of the new freqtrade run equals its report.html and
  differs from the 07:13 run's render (old binary) only in the format
  version, the report digest and the wall-clock line; render 60.6 s / 5.15 GB
  → 53.0 s / 3.04 GB. Redis likewise (its older run had live glossary and
  orientation calls, so its timing lines differ too). `make test`, `make vet`
  pass.
- **Left for the owner:** 336.1 MB of freqtrade's report.json is the other
  nine targets' ProgramIndexes, each byte for byte the `program-index.json`
  of its own sibling run directory. Naming those too would make restoring
  the saved report read the sibling directories, which REPORT does not
  allow today.

## 2026-09-29 — Orientation stage 1: evidence join, shared facts, collapsed connections, 16,384 allowance

- **Why:** freqtrade's orientation had been empty since 09-28: rung 1 was
  4,198,826 chars, 1,202,242 tokens (facts 2,229,142; connections 764,321;
  claims 551,943; member evidence 476,920; groups 163,617), and rungs 2 and
  3 were refused too. t1..t7 each held the same 1,464 registrations, and a
  merged place keeps one program's object id (`t1.n1781` for t2–t4's
  `build_helpers` module), so only t1's 299 members had evidence.
- **a08da2e6:** members join their place by `groupindex.DeclarationKey`;
  identical fact rows are one row with `targets`, whose ref is the first
  target's fact id and restores each target's own id in a role, recipe or
  flow row; one connection row per from/to/kind keeps every label and every
  sentence that differs from its label; `max_tokens` 16,384 (117 accepted
  exchanges: max 1,260 output tokens, median 848); request version 4,
  prompt v8 with the current fact kinds. The member ladder is unchanged and
  stated in READING, REPORT and EXECUTION as a known violation of "never
  sample". Tests: a merged place owned by t2 and t3 (fails on the old
  join), a shared fact restored per target, lossless connection rows.
- **Runs** (default cache, one at a time; all accepted at rung 1):

  | | chars before → after | input tokens before → after | output / cached / latency |
  | --- | --- | --- | --- |
  | freqtrade | 4,198,826 → 1,991,994 | 1,202,242 refused → 576,861 | 1,282 / 0 / 16.9 s |
  | Redis | 1,774,903 → 1,326,648 | 520,044 → 388,526 | 873 / 1,664 / 13.4 s |
  | litestream | 1,013,404 → 913,719 | 288,307 → 259,760 | 976 / 1,664 / 9.5 s |
  | self-snap | 3,825,079 refused (rung 2 2,436,543) → 3,657,688 | 641,593 at rung 2 → 981,361 | 612 / 1,664 / 25.9 s |

  After, by section: freqtrade facts 402,781 (1,870 rows, 1,622 shared),
  connections 188,620 (380 rows: 3,283 labels, 182 sentences), evidence
  672,112; Redis connections 776,888 → 124,276 (5,124 → 199 rows), evidence
  863,403 → 1,066,246; litestream connections 124,253 → 16,141; self-snap
  connections 246,449 → 74,787 at the same 2,207,853 of evidence.
  Evidence rows per target: freqtrade t1 299, t2–t4 41 each, t5 14, t6 13,
  t7 32, t9 20, t10 22 (t8 has no groups; before only t1); Redis 330 / 101 /
  36 / 66 (was 330 / 15 / 31 / 20); litestream t5 6 (was 0).
- **Answers:** freqtrade: summary citing 5 facts, 10 roles each with a fact,
  7 recipe steps each on an entrypoint, an 8-step flow (6 members, all with
  evidence, then the Worker, Wallets and liquidation prices: the lexical
  member sample shows) and 0 rejected rows. Redis 10 steps (6 members with
  evidence, main's entrypoint and the accept, read and reply
  registrations), 0 rejected;
  litestream 6 of 6 with evidence, 1 ignored ref; self-snap 7 steps (1
  member, 6 facts), 0 rejected. Live glossary requests: freqtrade 18,
  Redis 4, litestream 14, self-snap 34.
- **Exits:** freqtrade's first run filled the disk writing its 1.05 GB
  report.json (exit 1, the macOS swap grew 3 GB); after `go clean -cache`
  its rerun exited 0 in 253 s with the orientation from cache. Redis exit 0
  in 29 s, litestream 68 s, self-snap 103 s. Renders into
  `redis-r2/run/latest-*.html` are byte-identical to report.html; headless
  walks show each summary, every role and recipe command, and the Main flow
  (freqtrade 8 of 8 explanations); smoke walks without page errors.
- **Verified:** `make test`, `make vet` (package parallelism 2).

## 2026-09-29 — Call rows are names: no code marks, library calls on one line

- **Why:** benchmark v5 (R1, R3): with several call sites a flow row's
  "</>" marks were items of the row's three-column grid of their own, so a
  second or third wrapped into the 14 px twist column ("<", "/", ">"
  stacked, 9 × 41 px) and the name column (an empty 224 px bar):
  cliSendCommand's calls opened under redis-cli's Main flow, syncWithMaster's.
  Library calls with many sites (strerror, errno, close ×14) read as runs of
  marks, and as rows they split one part's calls under two boxes
  ("Replication" twice). The owner then asked for no marks at all: a name
  is its purple link with its "()".
- **Change:** every "</>" is gone: flow, Main flow and "What it does" rows,
  "+ helpers", Called by/Calls, Connections rows and headings, and the
  declaration title (its name stays the link to its code, underlined on
  hover). Names render through one helper, `rmDeclName`, whose code slot
  (`rmNameIcon`, a no-op) is where the owner's chosen icon will open the
  declaration's code, or on a "Called by" row the caller's call line. The
  page data keeps only that line (`site` on a "Called by" end: the first
  place in source order, every place in its words); other ends keep no
  place and a flow call's `sites` only the words its name's hover says.
  Renders: Redis 4.24 → 4.17 MB, litestream 3.91 → 3.85 MB, freqtrade
  8.76 → 8.61 MB, repomap 9.75 → 9.47 MB. A step's calls
  into code the report names no declaration for are no rows: one muted line
  ends the step, "also calls: strerror, errno, snprintf, …", each name once
  in source order, so consecutive calls into one part stay under one box.
  A name breaks only after a dot, never inside a word (litestream's
  "sql.Tx.Rollbac k"; a flow row's single word longer than the row ends in
  "…"); a one-sided outgoing Connections row no longer repeats its program
  ("cmd/litestream:" on every row of cmd/litestream).
- **Tests:** the flow test checks no library call is a row, the step's one
  line names them, no part's box repeats back to back and no name carries a
  mark; the no-line-number test checks a caller keeps its first call line
  and no other place is kept; the outgoing-row test checks the program is
  not named again; the mark assertions of the arrow-ends journey and the
  heading-marks test are gone.
- **Dropped (coordinator, after the critic's review), not committed:** a
  type read at the clicked field, the dispatcher wording ("rpush's handler
  itself calls call"; call's "one of 94" under the first alternative's
  part) and the plain card count line. Their implementation is kept as a
  patch in the scratchpad's `v5fix/v5-full-before-scope-change.patch`.
- **Looked at:** Redis (syncWithMaster, cliSendCommand opened under the
  Main flow, processCommand, queueMultiCommand's Called by) and litestream
  (cmd/litestream's Connections to SQLite): no mark, no library row, no
  repeated box, no name past the column (`look/v5fix-clean-*.png`;
  before: `look/v5fix-before-*.png`). Renders into `redis-r2/run/latest-*.html`;
  headless smoke walks of all four: no page error, no nested scroller, no
  line number, no mark, 16 declarations read each.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  ui-test` (135), `make ui-visual-test` (64 passed, 6 skipped).

## 2026-09-29 — A box carries the imports of the files that joined it; a Connections end's name said once

- **Why:** the parts request listed imports between whole-file rows only.
  Redis's lzf_c.c and lzf_d.c join redis.c's Persistence box by rule B,
  and their includes of lzfP.h vanished with them: lzfP.h (f13) was the one
  row of redis-server with no call and no import line. DeepSeek lumped it
  with pqsort.c (f16), sometimes as "Persistence", and two benchmark readers
  repeated that as a wrong claim.
- **Decision (delegated; a skeptic and a provider probe):** "c1". The
  skeptic measured a single-user join rule (a): it would move 44 files in 4
  repositories, 42 of them harmfully, so it was rejected; (b) was rejected
  for reversing the owner's near-tie clause. The probe added `"c4 -> f13"`
  to the saved Redis window, 5 draws: pqsort.c went to Sort command twice
  and "Compression and sorting" three times, never Persistence; lzfP.h to
  Persistence twice; 18–20 parts; zipmap unmoved. A per-row `called_from`
  field was probed too and destabilised the map (9–12 parts): rejected.
  Evidence in the scratchpad's `probe-calledfrom/` (`probe_c1.py`,
  `answers_c1.json`) and `skeptic-filejoin/`.
- **4412b3d6 (c1):** `rowImports` lets a file stand for one row: a whole
  file for its own, a file that is no row of its own for the one row every
  unit of it sits in (exact: all its code is there); a file whose units sit
  in several rows or none stands for none, and an import within one row is
  none. The placement follow-up shows a row the same imports. The prompts
  keep their wording, so requests without such a file keep their bytes.
  `TestAJoinedFileImportsThroughItsBox` fails on the old code; partstest's
  `checkImports` now checks the imports exactly (a mutation dropping one
  import fails the Python, Clojure and C fixtures) and lets an import-only
  arrow touch a role part through a file wholly in it. No language fixture
  has a rule-B join under the check's helper rule (their `role_attached`
  samples are all declarations): recorded missing in READING.
- **66902610 (Connections):** ends of one name are one heading, in the
  column and the canvas's card ("→ checkpointWithExecutor" had stood twice
  under SQLite, one per statement); in the column a row with no call of its
  own that would only name its heading gives the heading its "</>" mark,
  one per place ("→ acquireReadLock </>", not then "acquireReadLock
  </>"). litestream's column: 13 repeated headings and 30 heading-repeating
  rows before, 0 and 0 after (`look/c1-litestream-sqlite-after.png`).
- **Runs at 66902610, default system cache, no `--debug-dir`, no `cache
  clear`:** Redis exit 0 in 34 s: atlas_zones 1 live (redis-server; its
  input differs from the previous run by `"c4 -> f13"` alone, the other
  three programs' byte-identical), and the descriptions, areas, core, keys,
  orientation and glossary that follow. The answer: pqsort.c in Sort
  command, lzfP.h in Persistence (RDB and AOF), 19 parts as before, no
  other part's units or name changed. litestream v24 exit 0 in 45 s, 6/8
  targets (the two C targets fail on missing headers, as before), 0 live
  calls; freqtrade exit 0 in 5m13s, 10/10, every atlas request cached,
  orientation 3 live (refused by context size, as before); self-snap exit 0
  in 70 s, every atlas request cached, orientation 1 live and 1 cached (its
  first packing refused by size, as before). A first litestream and
  self-snap attempt without the Go SDK overlay in `GOFLAGS` lost their Go
  targets to the SDK's broken `slices` and were rerun with it. Renders
  into `redis-r2/run/latest-*.html`: Redis 4.24 MB, litestream 3.91 MB,
  freqtrade 8.76 MB, repomap 9.75 MB; headless smoke walks of all four
  without page errors, nested scrollers or line numbers.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  ui-test` (135), `make ui-visual-test` (64 passed, 6 skipped).

## 2026-09-29 — Visual journeys green, renders byte-identical, Connections rows and helpers by name

- **9fcba2bd (visual journeys):** the two `make ui-visual-test` failures
  since ada1f59e pinned the camera the owner changed that day. "Dense
  internal inventory…" clicked the final area on a whole-map card and
  expected its part opened; the card's area names now read in the column
  with the camera staying, which the journey checks. "A tile points at and
  chooses its own declaration" expected a declaration named out of sight
  centred at its own size; it is now shown with its whole part, framed and
  centred, the tile in sight, which the journey checks. No product change.
- **6cf4388c (render determinism):** glossary lookup ranged a map of
  spellings and kept the first of two equal spans, so freqtrade's Hyperopt
  class (a code name as written) and its "hyperopt" term took turns over
  "Hyperopt reads Config"; the display texts' dedup split or joined and the
  refs after it shifted (t1235/t1236). Equal spans now offer both
  definitions, sorted, as homonyms do. `TestRenderingASavedRunAgainGivesTheSameBytes`
  renders one saved run eight times against its publication and fails
  within a few renders without the fix; the lookup test checks Hyperopt
  offers both. A second render of each of the four saved runs is
  byte-identical to the first.
- **00ab5520 (the column's marks):** a Connections row in the column ends
  in the "</>" code mark linking where the call is written, the place on
  hover, with no printed place or "Open code ↗" (litestream's
  "acquireReadLock db.go:1186 Open code ↗" reads "acquireReadLock </>";
  160 + 50 rows of its two main components, none with a place). A helper's
  hidden marks take no room: "+ helpers: zrealloc(), zmalloc(),
  incrRefCount()"; pointing at or focusing a name shows its mark, Tab
  reaches it. The arrow-ends journey checks the row's mark and title.
- **Renders (no model calls) into `redis-r2/run/latest-*.html`:** Redis
  4.19 MB, litestream 3.91 MB, freqtrade 8.76 MB, repomap 9.75 MB. Headless
  smoke walks of all four: no page error, no nested scroller, no line
  number in a reading, 16 declarations read each. Screenshots
  `look/connrows-before-litestream.png`, `look/connmark-after-litestream.png`,
  `look/helpers-after-{rest,hover}.png`.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  ui-test` (134), `make ui-visual-test` (64 passed, 6 skipped: the
  REPOMAP_REAL_RUN journeys).

## 2026-09-29 — Benchmark v4 fixes: the column's scroller, kinds, work on its own, step links, call sites, dense parts

- **Why:** benchmark v4 on Redis (`results-v4.md`, onboarding R01, R2):
  call's 95 "handlers by input" sat in a scroller of their own (a wheel
  scrolled the page) with plain handler names; "Settings" in the Inputs
  frame opened at the 96 requests; serverCron was not findable from a Main
  flow of client commands; step links opened the registering call's line
  (inside createClient, addReply, initServer); with line numbers gone a
  caller's name opened processCommand 70 lines above the call;
  listAddNodeHead jumped into Core data structures' 91 unreadable tiles.
- **672e98ab and the follow-up:** the dispatched list is part of the
  column's scroll, folded under its count past twelve, each handler (and
  each "reached from" name) a link. A kind in a collection's closed frame
  is a button reading the collection at its section (Background work: the
  first of scheduled/continuous). "Also runs on its own:" after the Main
  flow (`ownWork`, saved scheduled then continuous inputs with their
  registration facts only): Redis "serverCron — main → initServer
  registers it; aeProcessEvents → processTimeEvents runs it" and
  IOThreadEntryPoint; freqtrade UvicornServer.run, Telegram._init,
  ExchangeWS._start_forever, IFreqaiModel._start_scanning; repomap's JS
  sizeWorkspace; litestream none. `runnersOf` counts a function-value call
  resolved to one callable (C `te->timeProc`); a runner chain starts at the
  last runner shown; identical registration chains read once (sizeWorkspace
  had "canvas registers it" ×4); methods carry their type. Step names link
  all of their function's code (`#L2386-L2415`), "registers it" the
  registration line (`#L2456`), no step prints a line, twists always show;
  a scheduled input's reading opens at its line. Reading ends carry
  `sites`: a "</>" mark per call site on callers/callees, flow rows and
  helper names (hover), queueMultiCommand's caller linking `redis.c#L2221`.
  A part too dense to read where it fits (`tooDense`: tile text below
  `staysOpen` of its size) is framed as a part with the chosen tile alone in
  its card. A part lists declarations reached from outside first ("40
  functions · 14 reached from outside"). A tile newly chosen in the part
  being read reads from the top; readings end with the home's text links;
  the canvas location row's frames go up a level.
- **Tests:** `TestWorkARunsOnItsOwnReadsAfterTheMainFlow`,
  `TestAKindChosenOpensItsCollectionAtItsSection`, the C Main flow test
  now checks each step's place is its declaration (fails with the
  registration line), the dispatch-site test checks the fold and links, the
  InputTypes test checks each kind's pick; the no-line-number test keeps
  its read sites out and asserts the call-site marks; the members-order
  test reads the outside-first order.
- **Renders (no model calls) of the four saved runs into `next-*.html` and
  `latest-*.html`:** Redis 4.01 → 4.19 MB, litestream 3.81 → 3.91 MB,
  freqtrade 8.35 → 8.76 MB, repomap 9.11 → 9.75 MB (the call sites).
  Headless walks: no page error, no nested scroller in any reading, every
  kind lands on its section, 16 declarations per report read. Screenshots
  `look/v4fix-{before,after}-{call,settings,component,helper}.png`,
  `look/v4fix-after-{scheduled-input,members,callsite}.png`,
  `look/v4fix-final-{freqtrade,repomap}-own.png`.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  ui-test` (134). `make ui-visual-test`: 62 passed, 2 failed, 6 skipped;
  the two failures ("dense internal inventory…", "a tile points at and
  chooses its own declaration") fail identically at 896eef2a (since
  ada1f59e framed a chosen tile's whole part), not from this change.
- **Open:** freqtrade renders differ between runs in their display-ref
  numbering (t1235/t1236), at 896eef2a as well.

## 2026-09-29 — a part of files in subdirectories reads its members; same-line fields apart

- **0193aa60:** a part reading's declaration names its file by path, as
  the part's file list does, so a part of files in subdirectories reads
  its members file by file (litestream's cmd/litestream parts and most of
  repomap's Go parts listed none: the column compared `main.go` with
  `cmd/litestream/main.go`). A field a struct declares on the same line as
  another is its own declaration by name, so a part's "Calls into" no
  longer names `shared.cone` as czero. `TestAPartsMembersKeepTheirFilesPathAndTheirOwnNames`
  fails with either change reverted.
- **Renders of the same four saved runs:** Redis 4.01 MB, litestream
  3.81 MB, freqtrade 8.35 MB, repomap 9.11 MB; a headless check of every
  part reading (litestream 22, repomap 45, Redis 32) found every member
  under a listed file and no page error. `go test ./internal/report/...`,
  `make ui-test` (134) pass.

## 2026-09-29 — "Reads:" in place of "Uses variables"; no line numbers in the column

- **Why:** the owner's rule for the reading column, no line numbers
  ("человек будет видеть код"); pushGenericCommand's "Uses variables" was a
  wall of fields printed as variables with every line using them ("argv
  :4248 :4250 …"), and named the wrong field where a struct declares
  several on one line (czero for `shared.cone`, nokeyerr for
  `shared.wrongtypeerr`: the reading keys a declaration by its line link).
- **Reads (9ee99c2d):** a declaration says "Reads: redisClient.argv,
  shared.cone, robj.ptr, redisClient.db.dict, …" under "Writes:", from the
  exact reads edges: field paths as the adapter records them (C
  `field_path`; `Type.field` for Python/JS) and module variables read whole,
  each once in the order first used, each name reading the type declaring
  the field or the variable. A field also written stays under Writes; a
  name leading to a longer listed path is said by it (serverCron 32 → 25
  names: `server.db` by `server.db.dict.size`); locals and parameters are
  never listed. Callers, callees, "Used by", a catalogue's callers and the
  launch fold's unsure/closed functions are names only, once; the page
  data keeps no place for them (the flow's hover keeps its call sites).
  Tests pinning `:30`/`server:21` line lists deleted;
  `TestAFunctionsReadingSaysEachReadOnceWithNoLineNumbers` renders Go's
  reading through the column script.
- **Renders (`repomap render`, no model calls) of the four saved runs into
  `redis-r2/run/latest-*.html`:** Redis 4.31 → 4.01 MB (−7.1%), litestream
  3.91 → 3.81 MB (−2.6%), freqtrade 9.41 → 8.37 MB (−11.0%), repomap
  9.88 → 9.13 MB (−7.5%). Headless walks: pushGenericCommand,
  processCommand and serverCron read with no `:N` anywhere, the Settings
  catalogue and "How these were found" likewise; smoke walks of all four
  pages (75 declarations) found no line number and no page error.
  Screenshots `look/reads-{pushGenericCommand,processCommand,serverCron}.png`.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  ui-test` (134).
- **Open:** a part's "Calls into" still lists "Uses variables" per
  neighbour by field name, where same-line fields share one key; a part
  of several files in a subdirectory lists no declarations (the column's
  file-by-file split compares a declaration's base name with the file's
  path; litestream's cmd/litestream parts, most of repomap's Go parts).

## 2026-09-29 — Benchmark v3 fixes: helper names, macros, Main flow, camera, settings, TODOs

- **Why:** benchmark v3 on Redis (`results-v3.md`) and its blind judge:
  the helper fold hid answers of C07, Q6 and C21; `__assert_rtn` and
  `__builtin_expect` read as library calls; the rebuilt Main flow lost
  aeMain, aeProcessEvents and createClient (Q2 0.96 → 0.81) and linked
  readQueryFromClient's beforeSleep registration (redis.c:1414); camera
  jumps; five actions back to Main flow; settings read as variable walls.
- **Flow (055e9c7a, 88768671):** a call into the caller's own part is its
  work and never folds; folded helpers stand as one muted line under their
  step, "+ helpers: createListObject, listAddNodeHead, dictAdd", each name a
  link, "+ helpers" opening them in place. A call a macro's expansion makes
  reads as the macro as written (pattern selector + `macro_expansion`
  witness; `assert`, `redisAssert` opening to `_redisAssert`, `dictHashKey`
  to its one-of-3 hash functions); a `builtin`-package call is no call.
  Fixture: `tools/dump.c`'s `assert(keys.len == 0)`.
- **Main flow (bd46d873, a27de53d):** a step citing a registration, or
  naming a callable a registration hands over, reads as that callable with
  where it is registered (exact calls from the most recent earlier step,
  else the entries; every reached site, never an arbitrary first) and what
  runs it (a dispatch's alternatives or an open call whose stores name it,
  with the entries' chain the first time). Redis: "acceptHandler — main →
  initServer registers it; main → aeMain → aeProcessEvents runs it",
  "readQueryFromClient redis.c:2456 — acceptHandler → createClient
  registers it". Why it changed: c1e72908 (09-28) made kept callables
  registration facts; the orientation re-asked after facts v5 cited them
  (a150, a147, a152) and the page showed `factLabel`, the registrar. The
  run below re-asked orientation again and the model named the subjects
  themselves; both now read the same. One Main flow: the component page's
  copy is hidden as the column's source, the input's link to it removed,
  one "Main flow" link stands above every reading of the component.
  redis-cli (no model flow) opens main's calls (parseOptions, repl, …).
  C test `TestCMainFlowReadsARegistrationAsTheCallableItRegisters` fails
  when the registrar replaces the function.
- **Column and camera (ada1f59e, d599f2f8, 30-map/45-modes):** a name chosen
  in the column shows its tile with the part whole across the canvas (tiles
  drawn), not at the tile's own size; an input's tile and an area named on
  a component's card read without moving the camera; each input of a
  connection row reads that input; the declaration's name is underlined on
  hover with a "</>" mark; "Back to map" restores the page scroll of the
  last click made while the map was read; a reading reached anew opens in
  its default state (folds return only on Back or the same item again).
- **Settings (109463f7, 9af38fb4):** ProgramIndex call patterns may carry
  `branch`, the lines an if condition guards (C adapter; kvd's port,
  dbfilename, persist). A setting reads "Writes: server.masterhost,
  server.masterport, server.replstate" from the declaring function's exact
  field writes on those lines; "also uses … also used by" folds under one
  counted line. litestream's struct-tag settings read as before.
- **Field types (e62211bd):** C fields record `types` (where the repository
  type they name is declared); a field row's `listNode *` reads listNode.
- **TODOs (853da540):** a TODO is a comment's marker in a code file (by the
  language's comment syntax, strings skipped); Redis's ten doc/*.html
  anchors are gone. test-redis.tcl's `# TODO:` is not listed: unanalysed
  files are counted, never read (facts v5).
- **pqsort.c in Persistence (item 17, no change):** DeepSeek's parts answer
  (`repomap.atlas.parts.v2`, exchange 1fef6f6d) grouped `c4` (redis.c
  Persistence box), `f13` (lzfP.h) and `f16` (pqsort.c: swapfunc, med3,
  _pqsort, pqsort) as "Persistence (RDB and AOF)" given `calls:
  ["c13 -> f16 (1)"]` (c13 = Sort command) and no other call or import.
  Rule B did not join the file to Sort command because `_pqsort`'s helper
  answer was a near-tie (Jev helper 0.49, responsibility 0.48, confidence
  0.24). Not patched downstream.
- **Runs (binary 109463f7, default cache, no `cache clear`), rendered at
  05aadb33 into `redis-r2/run/latest-*.html`:** Redis exit 0 in 29 s (atlas
  0 live; orientation 1 and glossary 3 live after the TODO count changed);
  litestream v24 exit 0 in 29 s (orientation 1, glossary 6 live); freqtrade
  exit 0 in 258 s (orientation 3 live, refused by context size at every
  packing, as before); self-snap exit 0 in 79 s (glossary 38, orientation 2
  live). Pages: Redis 4.31 MB, litestream 3.91 MB, freqtrade 9.41 MB,
  repomap 9.88 MB. Headless checks on the Redis page without the toggle:
  pushGenericCommand, expireGenericCommand, rdbLoad and
  handleClientsWaitingListPush show every needed call, no builtin; the Main
  flow names aeMain, aeProcessEvents, createClient and no registrar; no page
  errors; smoke walks of the other three pages without page errors.
  Screenshots `look/v3fix-{before,after,final}-*.png`.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  ui-test` (134).
- **Open:** a joint's `sides` hold only the connect/accept chains; no send or
  read functions are recorded for a joint, so the card lists none (item 7).
  "Uses variables" and "Called by" still show `:line` links (earlier
  decisions). Setting branches are C only (other adapters' settings are
  tags or option declarations).

## 2026-09-29 — Fix B, a type's takers, field readers and writers, the own-executable join

- **Fix B (22f5cce9):** a unit of a split file no box took (a near-tie in
  the assignment or the second pass) is a `c*` row of its own in the parts
  request, named by its declaration as a seed's row is; the parts step
  places it with its calls or the follow-up leaves it `left_out`. Its box
  question stays undecided (`role_undecided`), its helper mark is the
  helper question's; a blocked helper stays off the map.
  `TestAnUndecidedUnitIsARowOfItsOwn`; partstest requires every undecided
  unit to be its own part and checks rule C per unit, only for units that
  are no helpers (the per-file off-map entry had hidden that a helper
  nothing uses is no rule-C case: kvd.c's beforeSleep).
- **A type's takers (cc6996a2):** the places graph records `takes` (exact)
  from a callable to each repository type its parameters carry
  (ProgramIndex parameter `type_id`), and the role split counts it as a
  user of the type. Fixtures: freeClient takes kvClient (C),
  TypedParameter takes TypedQueryClient (Go), update_counter takes
  MutableCounter (Python), recordOrder takes OrderEvent (TS); Clojure has
  no parameter types. No model request changes.
- **Field readers and writers (df8ebe0d, 61c9dd00):** a record type's
  reading gives each field "Written by" / "Read by" by part; a global
  variable's lists the fields reached through it (`server.masterhost`, in
  the source order of each path's first field, from functions reading the
  variable itself); a function's says "Writes: …" once, each name reading
  the type declaring the field, and its field writes leave "Uses
  variables". No line numbers. Stored once per part reading (`fields`,
  `writes`, names by declaration index).
- **Own-executable join (e04743b1):** ProgramIndex 21 `target.executables`
  (C link output, Go main package's `go build` name, Python console/GUI
  script, package.json bin; none for Clojure or a hand-built C program).
  A `runs_program` word equal to one reads "Runs this repository's
  program X" and draws the call's arrow into X's component with no outside
  tile; a self-launch keeps its tile. The python-tutorial-game indexes are
  regenerated through the ordinary no-model run of the materialized fixture
  (only version and seal differ).
- **Runs (e04743b1 binary, default cache, no `cache clear`), rendered at
  61c9dd00 into `redis-r2/run/latest-*.html`:** Redis exit 0 in 35 s:
  dupClientReplyValue, dupStringObject, lookupKeyRead and convertToRealHash
  were each their own row and the parts answer put all four in Server core
  state (no follow-up, no lone part); iojob is placed by rule C
  (`role_placed_by_users`) with freeIOJob and queueIOJob in Virtual memory;
  19 parts. `server` lists 109 fields; `server.masterhost` written by
  initServerConfig, loadServerConfig, slaveofCommand, read by
  slaveofCommand, syncWithMaster, genRedisInfoString; slaveofCommand
  "Writes: server.masterhost, server.replstate, server.masterport".
  litestream v24 exit 0 in 63 s: IsSQLiteDatabase and txidVar joined
  "Configuration parsing"; cmd/litestream-test's 3 `litestream` launches
  draw one arrow into cmd/litestream, the MCP server's 9 keep their tile
  with "Runs this repository's program cmd/litestream". freqtrade exit 0
  in 260 s (orientation refused by context size, as before; no undecided
  unit; `Arguments` lists its 3 fields' writers and readers). self-snap
  (e04743b1) exit 0 in 116 s: Documentation and cloneReadmeRoleLog joined
  existing parts. Pages: Redis 4.31 MB (4.27 before), litestream 3.92 MB,
  freqtrade 9.41 MB (9.30), repomap 9.97 MB (9.72). Headless walks
  (`look/b-*.png`) without page errors.
- **Verified:** `make test`, `make vet` (package parallelism 2), `make
  ui-test` (134), `make ui-visual-test` (64 passed, 6 skipped).
- **Open:** a started program's join is equal names only, so
  `test_startup_time.py`'s `freqtrade` launch (its word not decided) and
  launches named at run time stay outside; the flow's library-call row
  (`exec.CommandContext`) does not name the program it joins; the
  cumulative JS fixture declares no bin (a unit test holds it).

## 2026-09-29 — Report batch: reading fixes, the page needs JavaScript ("b"), compact page data, the flow

- **Why:** the benchmark (`results-v2.md`), the blind judge and the owner's
  screenshots (items 1–16), the owner-approved flow v2 (17), the owner's
  page-size decision "b" of 2026-09-29 (18: the no-script requirement is
  dropped, the page's data holds every answer and the page needs
  JavaScript) and the page-data compaction (19).
- **Reading (1–16):** plaques on the frame border; a declaration's code
  link covers its lines and a relation links its own line, the author's
  comment inline; a declaration two programs hold lists every program's
  callers by program and says "Not called in …" (its tile quiet); a call
  leaving its program is read from each program's own code ("redis-cli:
  cliConnect → anetTcpConnect → anetTcpGenericConnect ⇢ redis-server:
  acceptHandler → anetAccept"), an outside endpoint's "outgoing"; Main
  flow at the top with clickable steps; navigation state (leave the input
  path with Back, remembered folds, Expand all, counts landing on their
  section, one level per pinch, whole framing); a part reads its contents
  first, file by file, a fan-out caller in one line; the component's
  outline and the home's programs table with the files each is built
  from; unanalysed code named with language and lines (`test-redis.tcl`,
  Tcl, 2 083 lines; facts v5); a directive's compared values nested under
  it (redis-server 30 settings); requests before scheduled before
  continuous; the registration as written and a dispatcher's filter; term
  cards on a click or a 600 ms pause; junk commands gone (redis-server 0
  commands); a zoomed part is the looked-at frame and the column follows.
- **"b" (224bc749, 656e8a16):** no printed part cards, connection rows,
  source index, input catalogue rows, state changes or static drawing; a
  glossary term's files are written from the data when opened; one
  `<noscript>` line. CONSTITUTION and REPORT state the rule once.
- **Compaction (ca2f9386):** link its place says → 1, a declaration's code
  link → its last line, a call's ends as declaration indices with its
  words dropped when the names say them, a tile's declaration by index,
  a reading's call kind by default, repeated parts once in `shared`; the
  script reads every value back exactly (tested on GitHub and GitLab).
- **Flow (f76ff625):** a function's calls in written order grouped by part,
  opening in place, helper calls behind one toggle (helper mark and a part
  most parts call, or the caller's own), library calls, declarations no
  part holds kept with their flows; an input opens at how a request
  reaches it (chains Go builds from the dispatch site's outer inputs),
  other ways folded, what the handler does, who sends it.
- **Sizes (the same saved runs rendered before "b" at 9658776e and at
  656e8a16):** Redis 1.3.6 28.96 MB → 4.27 MB (rm-page-data 8.30 → 2.04 MB,
  flows included); freqtrade 46.54 → 9.30 MB (11.41 → 5.89 MB); litestream
  v24 9.23 → 3.92 MB (1.37 → 1.25 MB); repomap (self-snap) 37.34 → 9.72 MB
  (9.49 → 7.24 MB). The earlier baselines before this batch were Redis
  16.62 MB and freqtrade 54.33 MB (older runs).
- **Final ordinary runs (68d6d4c5, default cache, no `--debug-dir`, no
  `cache clear`):** Redis exit 0 in 11 s (0 live calls); litestream v24
  exit 0 in 74 s (orientation and glossary asked again after facts v5);
  freqtrade exit 0 in 319 s (atlas_api 201, boundaries 122 live; the
  orientation refused by context size at every packing, as before);
  self-snap exit 0 in 124 s. Rendered with 656e8a16 into
  `latest-{redis,litestream,freqtrade,repomap}.html`; headless walks show
  no page errors.
- **Verified:** `make test` and `make vet` (package parallelism 2),
  `make ui-test` (134), `make ui-visual-test` (64 passed, 6 real-run specs
  skipped).
- **Open:** `lookupKeyRead` stays in no part (a near-tie placement, the
  owner's pending decision); the flow shows it as a plain name with its own
  flow. JS/TS has no test holding a registration as written (JSTS).

## 2026-09-29 — Python fields stored once from a call (freqtrade's subcommands)

- **Why:** freqtrade builds its subcommands on `self.parser =
  ArgumentParser(...)`; `self.parser.add_subparsers(...)` was untyped, so
  `add_parser` and `set_defaults` on its result were unresolved, never
  asked, and J1 had nothing to join (0 commands in the 2026-09-28 run).
- **Adapter (`parser.py`):** the collector records every store of a class's
  field (`self.name` in a method) and of a class attribute; a field stored
  exactly once by a plain assignment of a call carries that call: a call on
  it resolves to the outside call's member (`argparse.ArgumentParser.
  add_subparsers`) with the field as receiver and the outside symbol as
  receiver origin, and the field's source value is the storing call's
  result, as a local name's is. The call is resolved in the scope that
  makes it, so method order does not matter. Any second store (another
  assignment, augmented, deleted, unpacked, loop or `with` target, any
  class attribute such as a dataclass `field(...)`) leaves it unknown; the
  first cut took a dataclass's `field(default_factory=set)` for the value
  (`dataclasses.field.add` on the tutorial game) and class attributes now
  only disqualify.
- **Fixture:** `tool_cli.py`'s `ServiceCommands` (parser in `__init__`,
  subparsers in `build`, `serve` → `run_serve`) and `RebuiltParser` (stored
  twice, `add_argument("--again")` unresolved).
  `TestCumulativePythonFieldStoredOnceFromACallKeepsItsOrigin` checks the
  targets, receivers, origins and producers; the inputs preset reads `serve`
  as one command with its handler, declared on
  `self.parser.add_subparsers(dest="cmd")`; the word-given counts rise to
  `add_subparsers ×2`, `add_parser ×2`, `set_defaults ×2`. The
  python-tutorial-game backend index is regenerated (objects and relations
  unchanged; its scenario digest follows the parser).
- **freqtrade no-model (`python:.:script:freqtrade`, 60 s):**
  `self.parser.add_subparsers` is exact argparse; all 34 `add_parser` calls
  (`trade`, `create-userdir`, …, `backtesting`, `hyperopt`, `webserver`,
  `recursive-analysis`) are exact and asked per call with their words
  (atlas_api r3), and each one's result receives its `set_defaults(func=…)`,
  which hands its handler over (`trade` → start_trading, `backtesting` →
  start_backtesting, `list-pairs` → start_list_markets through `partial`);
  the add_parser row lists `set_defaults ×33` (the reassigned
  `convert_trade_data_cmd` local keeps no source origin). Registration
  facts hold both calls at the parser of line 380.
- **Equivalents:** Go and TypeScript fields carry the compiler's type;
  Clojure has no fields. Recorded in PYTHON and the fixture README.
- **Verified:** pythonprogramindex, facts, atlas, groupindex, run, report,
  programindex and pythontarget pass; contracttest passes but for
  `TestCumulativeGoMapOfParts` and `TestCumulativeGoExecutableSeedIsItsOwnRow`,
  which fail on the report agent's uncommitted `internal/atlas/reading/helpers.go`
  (they passed at 950d3064 and read no Python).

## 2026-09-29 — C field reads and writes (ProgramIndex 20)

- **Why:** a benchmark reader's top request. The C adapter read a whole
  variable (`reads server`) and folded writes into it, so the report could
  only say "server is used by 116 functions in 16 parts".
- **Facts (`internal/cproject`):** each member expression naming a field of
  a repository record is one exact `reads` or `writes` relation to the field
  object, sited at the field's name (`c_field_read` / `c_field_write`, a
  `macro_expansion` witness for a macro body's field), with the new
  `Relation.FieldPath`: the file-scope root variable, or the record of the
  chain's first field for any other root, then the named fields
  (`server.masterhost`, `server.db.expires`, `redisDb.expires`,
  `redisClient.db.expires`). Written: the destination of `=`, compound
  assignments, `++`/`--`, and an array member's element there. Read:
  everything else, including the pointer a `->` or an indexed pointer
  member is taken from. Passed through: the base of `.` and an array member
  indexed on the way to a field. The whole-variable reads are unchanged.
- **Carried:** ProgramIndex 20 validates a field path only on a read or
  write of one field of a type; GroupsIndex relation-target edges carry it
  (`StructuralEdge.FieldPath`, recompiled on hydrate, overlay unchanged);
  the places graph keeps them as `SymbolFacts.Fields` (type place, field,
  path, kind, site), apart from `uses`, which the role split reads, so no
  model request changes. The report is untouched; its operation writes
  already read `writes` edges to a type's field.
- **Tests:** `TestIndexReadsAndWritesRecordFields` (every role, macro sites,
  sizeof, platform and anonymous records),
  `TestCFixtureReadsAndWritesRecordFields` (kvd's `server.shutdown` one
  writer/one reader, `server.dbfile`, `kvEntry.value` through a local and as
  `server.db.value`, GroupsIndex edges, places fields); the variable-read
  tests skip field reads. The python-tutorial-game program indexes are
  regenerated through the ordinary no-model run of the materialized fixture
  (only version and seal differ; catalogs byte-identical).
- **Other languages:** Python already reads and writes typed receivers'
  fields as relations to the field object; it sets no field path and
  follows no chain (PYTHON). Go records no field accesses: SSA has them,
  but a capture beside the calls, the `*types.Var` → field ID map and the
  paths are not small; recorded (GO). JS/TS reads declared properties and
  writes nothing (JSTS); Clojure has no field declarations (CLOJURE).
- **Redis no-model (redis-server):** 3,088 field accesses of 7,249
  relations, 2.9 s. `server.replstate`: written by initServerConfig,
  loadServerConfig, freeClient, syncWithMaster, slaveofCommand ×2; read by
  serverCron and genRedisInfoString. `server.masterhost`: written by
  initServerConfig, loadServerConfig, slaveofCommand ×2; read by
  genRedisInfoString ×3, slaveofCommand ×4, syncWithMaster.
  `redisDb.expires`: written only by initServer (`server.db.expires`); read
  by deleteIfVolatile, deleteKey, emptyDb, expireIfNeeded, flushdbCommand
  (`redisClient.db.expires`), freeMemoryIfNeeded, genRedisInfoString,
  getExpire, removeExpire, serverCron, setExpire, tryResizeHashTables.
- **Verified:** focused tests of cproject, programindex, groupindex, atlas,
  facts, contracttest, run and report pass; `go vet` on the changed
  packages. Saved ProgramIndex 19 runs no longer render.

## 2026-09-28 — The reading column as the owner chose it (variant A)

- **Owner's choices** (designer's mocks in the scratchpad
  `designer-column/`, variant A for the declaration and the part, with his
  changes): the report renders data Go prepared and sorted (no sorting in
  JS), no canvas labels, the first click reads and zoom is separate, no
  defensive fallbacks; lists are plain names, keys bold, no squares.
- **Colours (d55729d3):** core parts and areas rose (`--core` #913b6d,
  `--core-ink` #62284a, `--core-area` #fef4f9) with the diamond; purple is
  `--link` only; key names bold ink; "returns or takes a type" dotted slate;
  `.flow-input` blue; model text italic `--model`. Green prose and
  navigation links unchanged (not agreed).
- **Page data (8688748c):** `pageGroupReading` on each part's card
  (`data-reading`): members by kind and name, keys, files, author comments,
  every field of a type with its type; callers merged by caller (calls,
  then callbacks), every input registered at the part as one Inputs
  neighbour (Server core state had 95 neighbours `get`, `set`, …); callees
  by part; per declaration its callers and callees grouped by the part each
  end is a member of (a table row's end had taken the input's href), own
  part first, and the variables it uses. `data-collection` on an Inputs
  collection (catalogues and loose inputs by kind and name, requests
  first), `data-entries` on a component, catalogue members by name,
  component titles without "(executable)" unless another would read the
  same. Bug (d): a tile's "… +N" row is appended after the list, so
  redisClient's `bulklen` keeps its type.
- **Column and canvas (b56962a8):** `31-reading-column.js` renders a part
  (Called from → boxed title → italic description → files → "12 functions",
  "4 variables", types → Calls into), a declaration (Called by by part →
  the name as the one code link, file only, "comment" on hover/focus as the
  author's claim → Calls → Uses variables, hover "A global variable of
  {part}"), a component ("Component · C executable", `main()` one link,
  inputs by kind lighting their tiles or the closed collection's row with
  no dimming, Connections, then Main flow / Not reachable / TODOs /
  Analysis coverage opening in place, and "Component details"), an Inputs
  collection (component box first, catalogue lines, names by name) and a
  home without the components list. Every name read in the column is shown
  on the canvas and in the address: `readDeclaration` pushes a visit for a
  declaration in the same part; the camera moves only to what is out of
  sight (`memberInSight`, `frameInSight`). A click on a whole-map card or a
  frame title reads without moving the camera; the magnifier zooms.
  Bug (a): redis-cli's Inputs is read in place; its magnifier frames the
  tiles readably. Bug (c): an arrow end opens its connection alone (a
  remembered open connection had stayed open). Bug (e): the part's
  description is marked by style.
- **Specs:** bug (b) was the spec clicking the first part in page order,
  off the canvas at 1440×900, which landed on its area: it now picks a part
  in sight (3 crumbs). real-report enters the component by its magnifier;
  the catalogue click spec uses the component's page; arrow-ends and
  pointing follow the new camera rule (a tile in sight stays, one out of
  sight is centred). New: `TestPartReadingListsMembersByNameAndCallersByCaller`,
  `TestDeclarationReadingGroupsItsRelationsByPartOwnPartFirst`,
  `TestATilesMoreRowTakesNoFieldsPlace`,
  `TestInputCollectionListsItsInputsByKindAndName`,
  `TestReadingColumnViewsFollowThePreparedData`, and the real spec
  "a caller named in a declaration's reading becomes the canvas's chosen
  tile" (tryResizeHashTables → serverCron: tile chosen, reading subject,
  camera still, Back returns).
- **Verified:** `go test ./internal/report/...`, `go vet`, `make ui-test`
  (131), `make ui-visual-test` (63 passed, 6 real skipped), real specs with
  `REPOMAP_REAL_RUN=~/Library/Caches/repomap/runs/20260928-151426-redis-1-3-6-2fcb7c42f2a5`
  6/6 (the 2026-09-27 redis-r2 run no longer renders: ProgramIndex 19).
  Rendered latest-redis (…151426), latest-litestream (…125752-litestream-v24),
  latest-repomap (…093911-self-snap) and latest-freqtrade (…094118) into
  `redis-r2/run/`; each loads with no page error and reads a part, a
  declaration, a component and a collection. Screenshots `look/col-*.png`.
- **Cost:** the reading data is 11% of the Redis and litestream pages
  (1.8 of 15.8 MB; 1.4 of 12 MB) and 15–17% of repomap's and freqtrade's
  (8.5 and 7.8 of 51 MB). Column width stays 320 px: at
  384 px the first reveal's group titles fell below 12 px.
- **Not done:** the designer's "About the repository" disclosures on home;
  cross-part "returns/takes a type"; the single input's reading; the
  Russian render was not checked in a browser.

## 2026-09-28 — Repair pass after the speed-mode sequence

- **Failures at a2fab47a** (`make vet` green): places `TestFixturePlaces`,
  facts `TestFixturePythonTutorialGame` and
  `TestFixtureAnchorsResolveInTheRepository` (the saved python-tutorial-game
  program indexes were ProgramIndex 18), contracttest
  `TestCumulativeClojureMapOfParts` and `TestCumulativeCMapOfParts` (the
  parts preset took e1's seed row for no role part). The indexes are
  regenerated through the ordinary no-model run of the materialized
  fixture (same objects and relations, dependency catalogs byte-identical);
  the places test's SHA-256 of the whole graph, a pin every change
  rewrote, is deleted for a seeds check; the preset takes every box row of
  the parts request for a role part and checks a seed is never asked,
  never assigned a box and is its own row.
- **Bugs the new tests found and fixed:** the launch walk's "could not look
  inside" read pattern edges no projected GroupsIndex has (only objects
  are retained; Go's unresolved calls carry no pattern), so it never
  showed: GroupsIndex compiles each retained declaration's unresolved
  calls from the bound ProgramIndex (`Index.Unresolved`, in memory) and
  Redis's reading now lists aeProcessEvents's calls through
  rfileProc/wfileProc. Rule C took a seed's row for no row, so a unit only
  the seed used stayed undecided (Go cmd/app's Subscribe). A catalogue
  joint naming a boundary whose operation was folded into another lost its
  peer; folded boundaries now name the operation standing for them.
- **Fixtures** (table in testdata/repositories/README.md): C kvd reads a
  configuration file (`loadConfig`, `splitLine`) and kvcli has `--raw`
  beside a `bgsave` comparison; Go `internal/storefixture/tool_cli.go`
  (per-call EqualFold, two flag sets, `ServerConfig` settings); Python
  `init.add_argument("--force")`; Clojure `shouted?`/`default-row?`. The
  kvd preset and a new inputs preset read the fixtures end to end, answer
  every question (a refused window fails the test) and hold e1, per-call
  words and C argument origins, K1/K2 catalogues, J1, settings, pass 2
  (kept callables with `registered_during`, tables), K5 peers (an
  unmatched row names none, no arrow), u6 outer inputs, the launch, named
  launches and the systems question. Missing equivalents are recorded in
  GO, PYTHON, JSTS, CLOJURE and C; the three-step glossary has no language
  fixture (terminology tests, plus a per-name memo test).
- **Revert checks:** 47 rules, each reverted in place and its test run:
  all 47 fail (scratchpad `repair/revert.log`). One revert (setting left
  out of the entry kinds) only breaks the criteria file at init and is not
  counted.
- **Defensive checks removed:** GroupsIndex outer.go's position lookups
  and launch.go's empty-subject check; the report's launch-chain cap of 64
  (a deeper chain lost its root) and hop hand-over check; places' row
  literal checks (ProgramIndex validates rows); the reading's running-
  target fallbacks and boundary-ID map inits; systems.go's re-sort of
  sorted packages; gatherNames' empty-row check; the projection's joint
  target lookup and its filters of unsure calls, idioms and declared-on
  sites, now validated by atlas.Validate (a site's text is checked where
  the reading writes it). Kept, with reasons in the report: Destinations'
  unnamed-package filter (its contract), the enclosing-callable DeclaredBy
  of handler-bound inputs and the unsure subject fallback (need the
  calling object carried on the atlas boundary), the alias text parse
  (needs structured aliases, a graph format change).
- **Acceptance:** `make test`, `make vet`, `make ui-test`, `make
  ui-visual-test` and `make build` pass; Go ran with `-overlay` restoring
  `~/sdk/go1.27.0/src/slices/slices.go`, into which UI notes had been
  typed at 18:42 (saved in the scratchpad `repair/owner-note-in-go-sdk-
  slices.txt`; the file is left as found). Redis on the default cache exit
  0 in 14.5 s and warm 6.7 s, 0 live calls, report.json identical but for
  `timing`. The C fixture on a scratch `--debug-dir`: cold 91 live in 24 s,
  warm 0 live, `cache clear` exit 0, rerun 79 live. `REPOMAP_REAL_RUN`
  specs on Redis: 2 of 3 fail (UI, open).

## 2026-09-28 — One outside call written once is one tile (owner's decision a; speed mode)

- **Problem:** Redis's DNS resolver stood three times, one frame per program,
  each with one gethostbyname tile, though all three programs compile the
  same call in anet.c.
- **Change (page_system_map.go):** a program's outside tile of one
  destination and symbol is another program's tile when any of its calls is
  at the same saved path and line (`pageOutbound.Anchor`, from the call's
  saved location); it stands once, in the first program's frame, with an
  arrow from each program, and the other programs' records are its
  aliases. No name decides it.
- **Redis (render of 20260928-125746):** one DNS resolver frame, one
  gethostbyname tile (redis-server's anet.c:146; redis-benchmark and
  redis-cli call it at anet.c:115 and :146), arrows from redis-server's
  Event loop and networking, redis-benchmark's Benchmark client and
  redis-cli's Command line client; no display group left. TCP endpoint
  stays redis-server's alone (the clients' connect has a local peer).
- Tests: the destination test covers the shared call site in the Redis
  shape (fails without the change: three frames) and keeps the distinct
  call sites' own frames and display group.

## 2026-09-28 — One plaque per arrow end, no digits (owner's 2a finished; speed mode)

- **Problem:** the owner still saw digits everywhere on Redis: a "1" on
  redis-server → TCP endpoint, stacks like "1 3 6 8 10" on Data type
  commands' border, part badges 2, 7, 6 on the tiles, and two "all" chips
  side by side for one two-headed arrow.
- **Change:** part number badges (and the part card they opened), digit
  chips, the key's numbers line and the unused numbered/arrows switch are
  gone. Each arrow end on the looked-at frame has one plaque: "all" when
  the parts behind it are every part of the frame (of more than one part),
  else a plain handle the size and colour of the former one-digit chip.
  Both directions to one outside frame on a side share the incoming
  direction's plaque, which stands for both (`endPlaques`, layout.mjs); its
  card is the incoming one with the existing "go the other way" link.
  Resting on or opening a plaque outlines the parts behind it and recedes
  nothing. Per-number narrowing of a card is gone with the digits.
- Tests: node `endPlaques` test (one plaque per pair, "all" only for every
  part); layout and emphasis tests lose their numbers; the legend-line Go
  test now checks the line is gone; the badge-card Playwright tests are
  deleted and the chip-text ones expect "all" or an empty handle (edited,
  not run: the owner asked for no browser walks).

## 2026-09-28 — Hover highlights and never dims (speed mode)

- **Problem:** zoomed into Redis's Data type commands, moving the pointer
  across the part tiles flickered the area: each tile receded every other
  part and arrow to 40%, the gap between tiles (the area's own space)
  restored them, the next tile receded them again.
- **Change:** only the reader's own choice recedes (a chosen part, frame or
  declaration, a pinned input path, search results). The pointer and an
  open arrow end keep their highlight (the subject's dark outline, its dark
  arrows, the outlined parts across them, the bold numbers) and bring
  forward only what they highlight; everything else stays as it is with
  nothing pointed at. `recedes` (emphasis.mjs) decides it from the choice
  drawn with nothing pointed at; `routeDrawing` takes that choice instead
  of a dim flag and the pointed frame's parts. Pointing at a declaration
  no longer recedes the others; a chosen one does while chosen.
- **Check (Playwright, 1440x900, Redis 20260928-125746 rendered):** redis-server
  → Data type commands, pointer down Hash → String → Sorted set → Set in
  6 px steps (67 samples). Before: every tile sample put the six other area
  tiles at 0.4, the gaps at 1 (335 samples below their rest value).
  After: the other area tiles 1 at every step, 0 samples below rest,
  receded arrows 42 at every step (the chosen area's own, as at rest).
- Tests: node `recedes` and route tests; pointing.spec's pointed-part test
  now fails if the pointer recedes a part it does not connect; the
  destination-group test chooses instead of pointing; arrow-ends.spec's
  end recedes nothing.

## 2026-09-28 — Settings a structure's tags name: one Jev question per tagged field (speed mode)

- **Problem:** litestream reads its YAML configuration through Go struct tags
  (`yaml:"dbs"`, 96 fields) and none was an input; the C half (Redis's
  `loadServerConfig` directives) came with f6e77401's argument origins.
- **Change (0550b9cb, 2e1ac061):** each field whose tag names a key is asked
  on its own (`repomap.atlas.inputs.v1.field`, Jev, memoized): `setting` or
  `none`, with the field and type, its structure and file, the tag as
  written and `structure_use`: the outside calls given a value of it and the
  fields of other structures typed with it, followed by their own use. The
  Go adapter records the fact from go/types, never a name: where the
  repository types an argument value's static type names are declared
  (`Value.Types`) and the same for a field's declared type
  (`Object.Types` → `Decl.Types`). Name matching had made `Config` "given to
  oss.NewClient(cfg)" (an `*oss.Config`) and left the CLI's JSON results
  without a use, so 13 of their keys became settings. A `setting` answer is
  an entry whose handler is not established, declared by its structure (one
  catalogue each) and on the one call decoding it when there is one.
- **Acceptance (default cache, binary ea762bb7f76d at 2e1ac061):** Redis exit
  0 in 6 s, 0 live calls: redis-server 37 settings (the 30 `loadServerConfig`
  directives, timeout, port, bind, save, dir, loglevel, … vm-max-threads, and
  7 values it compares: debug, verbose, notice, warning, always, everysec,
  no), 96 requests, 5 commands; redis-cli's 6 and redis-benchmark's 11
  options stay commands. litestream exit 0 in 9 s, 4 live atlas_inputs Jev
  requests (390 KB with one atlas_symbols request): 200 tagged fields asked,
  200 decided; all 96 YAML fields are settings, 102 of 104 JSON fields none
  (RestorePlanFile's min_txid and max_txid, reached only through RestorePlan,
  stay setting); cmd/litestream has 110 settings (96 + 2 + 12 per-call words,
  replica URL query keys and `fmt.Sprintf` field paths). Catalogues: Config's
  17 "Declared on yaml.Unmarshal(buf, &config) in Config", ReplicaSettings
  46, DBConfig 17, SocketConfig 3, LoggingConfig 4.
- **Cost:** no DeepSeek call in the acceptance runs; Jev ≈ 0.1 M tokens,
  under $0.01 (the earlier field passes during the work, 200 + 73 rows, the
  same order).
- **Not done:** a value handed through a repository helper typed `any`
  (`writeJSON(w, resp)`) shows no use; `UnmarshalYAML` methods, map keys and
  anonymous structures' fields are not asked; Python, JSTS, Clojure and C
  have no tagged-field equivalent (recorded in their contracts).
- Speed mode: build, vet and the reading, lines, surfacediscovery,
  programindex, goadapter, gocoreobject, groupindex, report and contracttest
  packages pass but the known failures; no full `make test`.

## 2026-09-28 — The glossary in three steps: names, one closed decision per name, explanations (speed mode)

- **Problem:** litestream's glossary grew from 45 to 263 terms with no code
  change. The one open request ("define the unfamiliar terms in these rows")
  chose terms only by leaving others out, so each draw set the size: 3 of 8
  live draws on the same window ran away (263, 288, one loop to the output
  ceiling). About 4 in 5 of the extra names were general words, parts of the
  code in plain words or paraphrases, and report.html grew by 3.3 MB.
- **Change (84f54d94):** DeepSeek returns names only. Code keeps the names the
  term lookup finds, folds case and plural spellings, and attaches every row
  that writes each name. Jev then asks one closed question per name
  (domain_concept / general_vocabulary / code_element, with criteria for
  each). The item is the name plus every text that writes it, with no cap,
  and the context is the orientation summary. The margin rule applies: a
  near-tie is undecided and is remembered per name. DeepSeek explains only
  the decided domain concepts, one closed t ref per name. The same name with
  the same explanation is one definition. The reduce prompt now joins
  same-name groups that mean the same concept, and sources are no longer a
  condition for joining. `glossary_names.json` records every outcome. Tests:
  `TestGlossaryExplainsOnlyDecidedDomainConcepts` (only a decided domain
  concept is explained; a warm run asks neither model),
  `TestGatheredNameFoldsSpellingsAndAttachesEveryRow`,
  `TestTermQuestionCarriesEveryWrittenText`,
  `TestReduceSameNameAndExplanationIsOneDefinition`, and the names and
  explanation form tests.
- **Measurement (saved windows `61235e8f…` / `40bf2f66…`, 5 draws each):**
  - Old shape: 45–248 / 49–270 distinct terms; union 264 / 276; 42 / 34
    terms in all five draws.
  - New shape: 143–181 / 155–271 names; 54–58 / 47–61 accepted; union 62 /
    74; 52 / 39 in all five.
  - Flips between two decision passes: 13 of 207 and 28 of 344. Every flip
    was between a decided option and undecided.
- **Acceptance (default cache, binary 400e81aaf333 = 84f54d94 plus the other
  streams' uncommitted atlas edits):**
  - litestream 20260928-123801, exit 0: 149 names in 99 rows gave 55 entries
    (was 263). Examples: LTX files, shadow WAL, TXID, lock page, storage
    class, salt, generation, WAL segments. report.html went from 13.88 MB to
    11.12 MB (the concepts section from 5.80 MB to 2.58 MB) and
    glossary.json from 9.28 MB to 2.14 MB.
  - redis 20260928-123821, exit 0: 64 names in 51 rows gave 24 entries (was
    20). Examples: AOF, RDB, skiplist, LZF compression, opcodes. The
    concepts section shrank by 168 KB. report.html grew from 13.77 MB to
    14.68 MB because the map's `data-catalogue` grew by 0.89 MB; that is
    the other streams' systems work, not the glossary.
  - Warm reruns 20260928-124108 and -124121: 0 live calls in every stage,
    and glossary.json is byte-identical.
  - Misjudged by Jev: vacuum and lease were marked code_element, Google Cloud
    Storage and SFTP undecided.
- **Cost:** the measurement was DeepSeek 131 k in + 62 k out and Jev about
  1.6 M tokens, ≈ $0.17. The glossary's live calls in the runs were under
  $0.02. The redis cold run re-asked atlas stages that other commits had
  invalidated, ≤ $0.19.
- Speed mode: build and vet pass, as do the terminology, orientation and run
  packages and the reading knowledge tests. No full `make test`.

## 2026-09-28 — Each outside system named once per package; the known-systems list is gone (speed mode)

- **Problem (owner: yes, "промпт сразу поправь на норм"):** the
  destination catalogue was a hand-kept list of known systems
  (`internal/atlas/destinations`), grown one entry at a time (c4a871f3,
  b362538b), and the report folded free text onto it by host and word.
- **Change (7ac6df61):** code groups outgoing rows by the outside package
  of their call; one DeepSeek question per package for the whole run
  (`lines.Systems`, `atlas_systems`, memoized per package): the package,
  its dependency record (module and version) and every symbol of it the
  program calls, one call of each as written; the answer is a short system
  name or `none`. Each outgoing row is offered its own targets' catalogue of
  these names (`lines.Destinations`; `none` gives no entry) plus `other:`.
  The list, its tests and every host/word fold are deleted; the report
  groups by the stored name without a parenthetical. `fixed_boundaries.md`
  rewritten short. Reading input 19.
- **Fix after the first run (73251559):** a query fact took the package of
  the `fmt.Sprintf` formatting its SQL at its site (two litestream-test
  queries named "fmt"); a row now takes the package of its own call only
  (`callsExternal`). Rows through a package answered `none` wrote the
  package or a host (`other: net` ×7, `localhost` ×2); the destination
  instruction now says to name the service or program at the other end, the
  entry listing the row's package first, and that a package, protocol,
  host, URL or key is not a name. Test:
  `TestOneOutsidePackageIsOfferedTheSameDestinationsInEveryWindow` (one
  question per package; two azblob calls in different windows get the same
  catalogue and name; `net/http` answered none gives no entry; the query
  fact carries no package; fails with the package rule reverted).
- **Acceptance, litestream (binary 400e81aaf333, run 20260928-123728,
  exit 0):** 14 packages named: boto3, s3, s3/manager → Amazon S3;
  azblob, azcore/runtime → Azure Blob Storage; cloud.google.com/go/storage
  → Google Cloud Storage; oss → Alibaba Cloud OSS; nats.go, jetstream →
  NATS; pkg/sftp → SFTP; gowebdav → WebDAV; database/sql → SQLite; net,
  net/http → none. Every backend is exactly one destination. Against
  20260928-115417: db.go:992 `sql.DB.Conn` PostgreSQL → SQLite (fixed);
  names now follow the per-package answer ("S3 storage" → "Amazon S3",
  "SFTP server" → "SFTP"). Still free text per row: the seven control-socket
  requests are "litestream control socket" ×5, "litestream daemon control
  socket" ×1 and "localhost" ×1 (a host despite the prompt); shrink.go:252
  once "other: fmt" beside SQLite (its destination chain is the formatting
  call); 5 sql rows unanswered ("row was not answered", 2 windows).
- **Redis (run 20260928-123903, exit 0, cached):** netdb.h and
  sys/socket.h → none, so no entry; destinations unchanged against
  20260928-112659: "DNS resolver" (gethostbyname) and "TCP endpoint"
  (connect) in each of redis-server, redis-cli and redis-benchmark.
- **Cost:** DeepSeek ≈ 1.01 M input (0.24 M cache hits) + 23 k output over
  two litestream runs, about $0.2, of which this stream's own requests are
  the boundaries (2 × 97 windows) and one systems call; the first run also
  re-bought 229 Jev `atlas_api` calls (3.7 M input, ≈ $0.16) and glossary,
  orientation and joints calls whose requests other streams changed.
- Speed mode: build, vet, and the lines, reading, report and run packages
  pass on HEAD plus the first commit alone; no full `make test`.

## 2026-09-28 — One destination list per outgoing row, whatever shares its window (speed mode)

- **Problem (lead):** litestream's azblob `DeleteBlob` at
  abs/replica_client.go:303, alone in its `atlas_boundaries` window, was
  named "S3 storage"; the one at :344, beside its siblings, "other: Azure
  Blob Storage". The saved requests (run 20260928-104647) show both windows
  offered the identical 45-entry list: no window-mate brought an entry. The
  list had no Azure entry and the azblob import annotated nothing, so a row
  asked alone took the nearest listed store; oss (2 rows) and a second
  azblob pager did the same, and GCS clients were "Google".
- **Change (c4a871f3, b362538b):** the known systems list Azure Blob
  Storage, Google Cloud Storage, Alibaba Cloud OSS, SFTP server and WebDAV
  server; a system of several words may take one from a package host
  (`cloud.google.com/go/storage`), a host alone still implies nothing. An
  outgoing row's catalogue is annotated with its own targets' dependencies
  (the owner-less window no longer unions other targets'). The prompt
  explains an entry's `dependencies` and says another system of the same
  sort is `other:`. Test: `TestOneOutsideSystemIsOfferedTheSameDestinationsInEveryWindow`
  (two azblob calls in different windows, one beside an S3 call, get
  byte-identical choices; another target's gateway row keeps its own list
  and its `other:` name). Fails with either half reverted.
- **Acceptance (default cache, binary 3b9ea79efb25, run
  20260928-115417, exit 0, 98 live boundary windows):** :303 and :344 are
  both Azure Blob Storage; every abs/oss/gs/sftp/webdav row now has one
  destination per backend. Against 20260928-112717, 18 of 198 outgoing
  destinations changed: azblob ×2, oss ×2 and GCS ×2 corrected; WebDAV ×9
  renamed "WebDAV server"; "Unix socket" ×2 joined "Unix domain socket";
  "heartbeat endpoint" became "heartbeat service". Still wrong: db.go:992
  `sql.DB.Conn` is PostgreSQL (it was before; SQLite in the intermediate run).
  The intermediate run (c4a871f3, 20260928-115102) showed free text differs
  per window ("SFTP" beside "SFTP server", a capitalised control socket) and
  one window answered 1 of 3 rows. Rendered to the scratchpad
  `redis-r2/run/latest-litestream.html`.
- **Cost:** DeepSeek ≈ 0.66 M input (0.31 M cache hits) + 7 k output over two
  runs, about $0.13; no live Jev calls.
- Speed mode: build, vet and the destinations, lines, reading and report
  packages pass; no full `make test`.

## 2026-09-28 — What a call's words become is asked of each call (speed mode)

- **Problem (lead, owner "пробуй"):** `enters` was decided once per outside
  symbol from one usage example, picked in pattern-ID order. After
  cf32f195 litestream's `strings.HasPrefix` example became
  `strings.HasPrefix(cmd, "-")`, Jev answered `command`, and 31 false
  inputs appeared in cmd/litestream (`http://`, `s3://`, `arn:`, `10.` …).
  One symbol's calls differ: `strcmp(argv[i], "-h")` is an option,
  `strcasecmp(rc->name, "monitor")` and `strings.HasPrefix(url, "s3://")`
  are not.
- **Change:** every call outside tests that gives an outside symbol ≥1
  literal is asked on its own (`lines.APICall`, `repomap.atlas.enters.v1`,
  Jev, round 3 of `atlas_api`), after its symbol's `talks`. Item: symbol,
  declared type, the call as written (`lines.CallText`), the enclosing
  declaration with its signature, every literal, each argument's origin in
  words without a position (`originText`: `"http://"`, `parameter #1 u of
  isValidHeartbeatURL`, `field Bucket of receiver c`, `element … of …`,
  `result of calling Error`, `not followed: argv[i]`), and `talks` when
  decided. Rows are grouped per symbol (their own windows), in source
  order, with request-local `callN` refs; each is memoized by its fields
  only (no line, no program), so a moved call is not re-asked
  (`TestAWordCallMovedByAnUnrelatedEditIsNotAskedAgain`). Not asked: a
  call whose symbol's talks is anything but none or undecided (one is not
  both; `talksStands` and its `cell_rejected` are gone), a call of a
  symbol handed a callable, and a call another fact names at its
  line/column (SQL, config reads such as `getenv`, route and dispatch
  facts); a registration of the symbol itself that hands nothing over is
  asked, and one whose own call gives no literal is not. Removed: the
  `.given` set (`APIGiven`, `GivenOptions`), `per_call` (`APIPerCall`, its
  criteria section, `callLevels`, `called_by`/`level` fields),
  `apiRole.enters`/`perCall`, `atlas.APIRole.Enters` (atlas version 19),
  `undecidedEnters`, `per_call_undecided`. Consumers read the per-call
  answers: `applyAPIRoles` and `bindInterpretedBoundaries` through one
  `readWordCall` (entry, none, undecided → unsure, unnameable → unsure);
  a wordless call of a symbol some call of which is an entry is unsure
  `no_words`; idioms are per symbol and kind, counted against every call
  of the symbol recorded (`TestTheLaunchEvidenceReadsEachCallsOwnAnswer`).
  Word-given symbols join the plain talks round; their `usage` stays the
  first word call.
- **CallText speed:** `lineOffset` and the block-comment lexer copied the
  rest of the file per call (`string(src[i:])`): 190 ms per call on
  redis.c, 6 ms after; `lines.CallFile` lexes a file once for all its
  calls (the reader's `sourceText` caches it per stage).
- **Measured first** (scratchpad `percall/`, exact bodies the reading
  wrote, 3 draws, margin 0.10, ≈ $0.076): every `flag.FlagSet.*` call of
  litestream (74) `command` 3/3 (leads 0.80–1.0); Redis `strcmp(argv[i],
  "-x")` (17) `command` 3/3 (0.32–0.62); `strings.HasPrefix` none 31/33,
  `command` for the two `"-"` checks (Main.Run, ParseFlags; leads
  0.24–0.30); `fmt.Errorf` (151 of 601 sampled) none 3/3; Redis
  `strcasecmp` (66) none 3/3 for 45, `command` 3/3 for 2
  (loadServerConfig `daemonize`, `always`), the rest near-ties among
  loadServerConfig's settings-file keys and DEBUG sub-words; `monitor`,
  `quit`, `exit` none. Without the `talks` field 17 of 66 `strcasecmp`
  calls leaned `command` (≥ 1 of 2 draws) against 4 with it: kept.
- **Re-frozen** (sha256): `entry_options.md` `f404a49b8526508b976404fa242ce0f507c4fcc31b4fe456b1ef9f750b8c694d`
  (only the `per_call` section removed); `api_call.md`
  `1408380f1313973e06647a29d1106e103efc50c25a4c2a744752fd23e352d69c`;
  ask enters "What does the word or words this call is given become on
  our map?" `3de16719…18883`; `api.md` `35e07898…75d08` and ask talks
  `f538b957…6f2f4` unchanged.
- **Tests:** word-given (every language: talks plus each word call asked),
  launches, C preset (`getenv` is a config fact, not asked), Echo preset
  (the SQL-named call is not asked), entries, memo; the fixtures of all
  five languages already hold word calls, no fixture changed.
  `go build ./...`, `go vet` of atlas/..., contracttest, groupindex;
  `go test` of atlas/..., contracttest, groupindex, report, run pass but
  the known places TestFixturePlaces and contracttest
  TestCumulativeClojureMapOfParts / TestCumulativeCMapOfParts (e1 seeds).
  Full `make test`/UI tests not run.
- **Acceptance** (`make build`, ordinary runs `--no-serve --no-open` on the
  default system cache, keys by the sed recipe; renders in
  `redis-r2/run/latest-{redis,litestream}.html`, byte-identical to the
  runs' report.html; headless Chromium: the Inputs folds read
  "string.h.strcasecmp: 6 of 65 word calls declare inputs (command)",
  no page errors):
  - **Redis** `20260928-112613`: exit 0 in 7 s; 314 per-call questions,
    301 decided, 13 undecided (unsure); 29 atlas_api Jev requests; warm
    6 s, 0 live. Inputs: redis-server 98 → 104 (+6 false: loadServerConfig
    `save`, `verbose`, `daemonize`, `always`, `everysec`, `vm-page-size`
    answered command; C records `argv[0]` of the split config line as
    "not followed", so the item cannot say it is not the argument vector),
    redis-cli 100 = 100, redis-benchmark 11 = 11, redis-check-dump 0.
  - **litestream** `20260928-112717`: exit 0 in 14 s; 1,440 per-call
    questions over 78 symbols, 1,425 decided, 15 undecided; 135 Jev
    requests (95 talks rows of former word-given symbols re-asked once);
    warm 9 s, 0 live, atlas equal but budget. cmd/litestream 106 → 77: the
    31 false `strings.HasPrefix` inputs are gone; the two `"-"` checks stay
    (as before); MCP argument names: `mcp.WithBoolean` if_db_not_exists /
    if_replica_exists lost, `RequireString` parallelism and `WithString`
    path gained (near-tie territory, as in K4). Every flag input stays;
    cmd/litestream-test 25 = 25. Unsure: 46 → 19 and 6 → 0.
  - **Cost:** Jev 164 requests, 10.4 MB, 2.79 M input tokens ≈ $0.12;
    DeepSeek none (everything else cached).
- **Owner items:** glossary stays 263 terms (cached, identical to
  104647's; why 45 → 263 after cf32f195 was not cheap to find). azblob
  DeleteBlob at abs/replica_client.go:303 is still "S3 storage": asked
  alone in its boundaries window (r2-w2), the closed destination catalogue
  offered `d23 S3 storage` and no Azure entry, and the model took it; the
  :344 DeleteBlob, asked beside its Azure siblings, answered "other: Azure
  Blob Storage". C argument origins (`argv[0]` of a split line) would stop
  the six settings-file keys.

## 2026-09-28 — A command built on either branch is both launches' (speed mode)

- **Problem (owner):** litestream's whole map had a "Program not established"
  frame holding CombinedOutput although `cmd` in cmd/litestream-test's
  validate.go:110–124 is `exec.CommandContext(ctx, "litestream", …)` on both
  branches of an if/else. The Go adapter recorded that SSA φ as `unknown`
  "conditional value", so the launch fold (a call on a launching call's
  `call_result`) never saw it.
- **Change:** the Go adapter records a φ as `alternatives` of its incoming
  values in edge order (identical values once; an unfollowed edge stays its
  `unknown` part); a join already expanded in the same recorded value stays
  the "conditional value" frontier, so 24 conditional `q += …` appends are
  97 nodes, not 2^24 (the test without that rule times out). The fold takes a
  receiver whose every alternative is a launching call's result
  (`launchedBy`, boxes.go); one other part keeps the call its own boundary.
  `alternatives` already existed in the sourcevalue vocabulary.
- **Fixture:** Go `RevisionOf` (storefixture/destinations.go): the contract
  test checks CombinedOutput's receiver is the alternatives of both launches,
  each giving git; the reading test folds it into both, each named git, and
  keeps `held.Run()` on a one-branch command its own boundary.
- **Other tiles of that frame** (litestream-v24, read only): replicate.go:376
  `exec.CommandContext(ctx, execArgs[0], execArgs[1:]...)` gives no word (both
  arguments are `unknown`: a slice element and a re-slice of
  `shellwords.Parse(c.Config.Exec)`), so it is not asked, as before;
  replicate.go:380 `c.cmd.Start()` reads the field `cmd` of the receiver (the
  store at :376 goes through the pointer and no fact carries it), stays not
  established; etc/s3_mock.py:32 `subprocess.run(cmd, env=env)` with
  `cmd = sys.argv[1:]` gives no word, stays not established. No fact names
  which argument is the program, so none becomes "named at run time".
- **Other languages:** Python (a reassigned binding's read is `unknown` with
  the name) and JSTS (a `let` without initializer or reassigned is `unknown`)
  do not give alternatives: recorded as missing equivalents in PYTHON.md and
  JSTS.md.
- **Verified:** `make build`; `go vet` of surfacediscovery, atlas/reading,
  contracttest; tests of surfacediscovery, atlas/..., programindex/...,
  contracttest pass except the known ProgramIndex-18 fixture tests (places
  TestFixturePlaces, facts TestFixturePythonTutorialGame and
  TestFixtureAnchorsResolveInTheRepository) and the e1 split-file seed tests
  (contracttest TestCumulativeClojureMapOfParts, TestCumulativeCMapOfParts).
  Ordinary litestream run on the default cache
  (`20260928-104647-litestream-v24-655b9ef38d8b`): exit 0 in 1m48s; launches
  not established cmd/litestream 2 = 2, cmd/litestream-test 1 → 0 (4 → 3
  launches, litestream ×3), etc/s3_mock 1 = 1; the frame holds CommandContext,
  Cmd.Start and run (CombinedOutput gone); render byte-identical,
  Playwright 1440×900 without page errors.
- **Side effects for the owner (model re-asks, not reverted):** the new
  origins changed 25 atlas_boundaries windows, 2 atlas_api rows, glossary and
  orientation. Call order inside a declaration follows pattern IDs, so
  Main.Run's two `strings.HasPrefix` calls swapped and the symbol's `usage`
  became `strings.HasPrefix(cmd, "-")` (was `(err.Error(), "signal:")`);
  Jev answered `enters: command`, and cmd/litestream gained 31 false command
  inputs (`http://`, `10.`, `arn:` …; 75 → 106). azblob DeleteBlob is now
  named "S3 storage"; the glossary drew 263 terms (45), report.html
  10.7 → 14.0 MB. nats.Connect's row gained `result_receives: Close ×1` (a φ
  of one value is now that call's result).

## 2026-09-28 — Inputs point into their component: "taken in here" arrows (speed mode)

- **Problem (owner):** "а че у нас инпуты не указывают никуда?" On Redis's
  whole map redis-cli's Inputs (94 cmdTable rows, 6 options) floated beside
  redis-check-dump and redis-benchmark's (11 options) above-right: a
  handler-less input had no arrow, and the undrawn ELK edge of Q1 did not
  place them.
- **Change:** a handler-less input draws the ordinary Inputs arrow into the
  part where its code takes it in (`takenInPlaces`, page_catalogue.go, from
  saved `DeclaredBy` and the derived catalogue readers): "declared in" the
  declaring function's part, "looked up in" each table reader's part; no
  part is chosen among several and a table with no reader draws none. The
  label is data, not drawn; the arrow keeps the input's own style (model
  inputs dashed, as "implemented in" is) and chip rules. It is never an
  "implemented in": `inputOwner`, reach, phases and the part's operations
  are unchanged, and the reading still says "handler not established". The
  arrow card counts such rows as inputs ("100 inputs") and names each row's
  inputs in order ("-a, -h … declared in parseOptions"); a named call's
  label is no longer parsed as "caller verb callee". SystemMap draws a
  collection none of whose inputs reaches a part of its component into the
  component itself (per input, no calls, no label); the undrawn layout edge
  is gone.
- **Verified** (render of `20260928-093656-redis-1-3-6-19b3b69d547d` and
  `20260928-093822-litestream-v24-86f5c09bd44d`, Playwright 1440×900): the
  Redis whole map at rest draws 9 arrows (7 before); redis-cli's and
  redis-benchmark's Inputs stand above their components with an arrow into
  each. Zoomed into redis-cli the arrow enters chip 2, Command line client
  (parseOptions and lookupCommand are both there), and the reading lists
  "-a, -h, -i, -n, -p, -r declared in parseOptions" and the 94 commands
  "looked up in lookupCommand". Every collection of both maps has drawn
  relations into its component (redis-server 97 of 98 inputs by handler).
  `go build`, `go test ./internal/report`, the web node tests and
  `node build.mjs` pass; full suites not run (speed mode).

## 2026-09-28 — Where an input takes effect: catalogues, the launch walk, per-call words, C tables and kept callables, peer inputs, outer inputs (e1, K1–K5, pass 2, u6; speed mode)

- **Speed mode (owner):** every commit builds and vets; focused tests ran where
  fast; known failures and owed tests are listed in the scratchpad
  `impl/cleanup-todo.md` for the cleanup pass (fixture program indexes are
  ProgramIndex 18; cproject's table-row test now also lists kept callables).
- **e1:** `SymbolFacts.Seeds` (graph 20); a split file's seed is its own `c*`
  row, never asked the helper or box question. redis-server's `main` is now a
  one-declaration entry part the grouping named "main".
- **K1/K2:** handler-less inputs form catalogues (GroupsIndex `DeclaredBy`,
  `DeclaredOn`; derived `Catalogues`), keyed by the object they are declared on
  (followed back from the receiver through outside calls that name nothing),
  else by the declaring function. The reading: "Declared on/in F · one of N ·
  called from C :l", "F also uses V" with V's other users folded by part, and
  once "Where these take effect is not established." J1 joins a wordless
  hand-over on a word entry's result of the same kind. An unjoined inputs
  collection sits beside its component through an undrawn ELK edge (Q1).
- **K3:** GroupsIndex derives the launch walk (seeds; Go init and package
  variables; module bodies outside C) with found / unsure / could not look
  inside / nothing per function, in the Inputs reading's fold "How these were
  found" with the idiom lines (model); atlas carries `Unsure` and `Idioms`.
  An input declared only inside an input's handler reach is its
  sub-argument ("Words its handler checks"), never a tile. The overview
  builder now re-derives GroupsIndex after dropping test subjects (its reach
  and catalogue edge positions had named the unfiltered edges; litestream's
  report panicked).
- **K4:** `enters` gains `per_call` (a per-call round, `repomap.atlas.api.v8.call`)
  and every literal is sent (`literals[:6]` lifted). Measured first
  (`impl/k4probe/results.md`, 5 draws, ≈ $0.08): no Redis or litestream symbol
  is answered per_call; FlagSet.Var near-tie → command, NewFlagSet command →
  near-tie/none, 41 of 112 given rows grow and re-ask once.
- **Pass 2 (C):** ProgramIndex 19 `Object.Rows` (a table's rows that store no
  function; string arrays) and `ParameterStores`; facts 4 `Registrar` with
  `registered_during` (no caps; ported from 187d6655). atlas_inputs asks each
  kept callable (per registrar and callable) and each table once (Jev,
  memoized). request's Includes gained "such as the callable the program's
  own event loop runs when its listening socket has a connection to accept"
  (measured: acceptHandler none → request 5/5; binds rows with every literal
  unchanged). CONSTITUTION's registration sentence rewritten for the owner's
  review (084ec957). C argument origin kinds were not added (the per-call item
  shows the text; no symbol is per_call).
- **K5:** a table row names the peer program's input only through the joints
  peers question, and only where the programs are already joined; saved as
  `Operation.Sends` (GroupsIndex 25), read as "Sent to/Sent by … · model match",
  never an arrow. Blind peers now offer only entries with a known handler.
  The launched-program → own-executable link is not built (a redesign).
- **u6:** a dispatch site derives `Outer` (inputs reaching it and not
  dispatched there, plus the registration hop from a handed callable no input
  handles) and `Unexplained`; each outer input is one closed line naming what
  it registers.
- **Acceptance** (ordinary `--no-serve --no-open` runs on the default system
  cache, keys by the sed recipe; no `--debug-dir`, no `cache clear`; binary
  at 66a733a6/dd1cc001; renders at the final templates):
  - **Redis 1.3.6**: exit 0 in 20.8 s with 0 live calls (the development
    runs asked the new Jev and DeepSeek windows), warm 19.6 s with 0 live.
    Entry parts: redis-server "main" (main alone), redis-cli "Command line
    client", redis-benchmark "Benchmark client", redis-check-dump "Dump
    checker"; no "entry not on the map". Inputs: redis-server 98 (95 command
    rows + acceptHandler request, serverCron scheduled, the I/O thread
    continuous), redis-cli 100 (6 options + 94 cmdTable requests),
    redis-benchmark 11, check-dump 0. Catalogues: redis-cli "Declared in
    parseOptions · one of 6 · called from main :513 · parseOptions also uses
    config (5 functions in 1 part)", redis-benchmark parseOptions 11 (main
    :509, config 12), "In cmdTable · one of 94 requests · looked up in
    lookupCommand, called from cliSendCommand :311, main :522". Launch folds:
    redis-cli main → parseOptions (6), main → lookupCommand (94); redis-server
    main → initServer (2), main → loadAppendOnlyFile → lookupCommand (95),
    spawnIOThread (1); strcmp calls with no word unsure (2). Kept callables:
    serverCron scheduled, acceptHandler request, the rest none. K5: 91 of 94
    cmdTable rows name redis-server's input of the same command (0 other
    matches; zmerge, zmergeweighed, rewriteaof none); the redis-cli and
    redis-benchmark arrows to redis-server stay one each. GET: dispatched from
    call, "A request for get arrives at call from serverCron · registers
    sendReplyToClient, sendBulkToSlave, readQueryFromClient" and "… from
    acceptHandler · registers readQueryFromClient, sendReplyToClient", "Other
    ways to call are not established." (the AOF replay at start); from
    loadAppendOnlyFile not established; "Sent by redis-cli: get (cmdTable) ·
    model match". No program launched.
  - **litestream v24**: exit 0 in 29.7 s (22 atlas_api Jev windows live for
    the uncapped literals and new criteria; core 2, joints 1, glossary 1,
    orientation 1 DeepSeek), warm 8.8 s with 0 live. cmd/litestream 115 → 75
    inputs (flag-set names now unsure, MCP argument names gone with
    RequireString a near-tie, -txid and -level back), cmd/litestream-test
    37 → 25. Catalogues per FlagSet ("Declared on
    flag.NewFlagSet("litestream-restore", …) in RestoreCommand.Run · one of 12
    · called from Main.Run :207"), registerConfigFlag (config, no-expand-env;
    called from 6 Run methods). Launched: litestream ×9 and 2 not established
    (cmd/litestream), ×3 and 1 (litestream-test), 1 (s3_mock).
  - **repomap self snapshot** (refreshed to 66a733a6 in place,
    `--target …::…/cmd/repomap`): exit 0 in 104.9 s (partly live: the
    snapshot moved 30 commits). 36 inputs (32 commands, 4 requests) in 5
    FlagSet catalogues; launched git ×4, go ×2, clj-kondo, python3, 2 named
    at run time, 8 not established. fmt.Errorf's words question is a
    near-tie: 1,609 unsure calls, now one folded line.
  - **freqtrade** (cold): exit 0 in 430.4 s; Jev 776 requests (5.87 M input,
    0.35 M output tokens), DeepSeek 202 (0.84 M input, 0.75 M of them cache
    misses, 26 k output): about $0.25 + $0.23. The orientation request was
    refused by context size at every packing (pre-existing). freqtrade main
    158 inputs (133 requests, 21 interactions, 4 continuous) and **no
    subcommand**: `add_parser`/`set_defaults` are unresolved calls (the Python
    adapter does not type `self.parser.add_subparsers(...)`), so neither is an
    outside symbol and J1 has nothing to join. Recorded as a PYTHON gap.
  - **Headless look** (1440×900, loopback 8955, stopped): redis-cli -h, GET,
    redis-cli get, litestream -txid and -config read as above, no page errors
    (`impl/shots/`).
  - **Cost of the sequence** (default cache records since 08:40Z plus direct
    probes): Jev ≈ 9.0 M tokens (≈ $0.37) + probes ≈ 11.8 MB (≈ $0.13);
    DeepSeek ≈ 3.69 M input (3.14 M misses) + 55 k output (≈ $0.9); about
    $1.4 in all, a third of it the cold freqtrade run.

## 2026-09-28 — A seed of a split file is its own grouping row (e1)

- `atlas.SymbolFacts.Seeds` (graph 20) names the targets whose execution
  begins at the declaration. The role split never asks a seed the helper
  question or the box question; a split file gives each seed a `c*` row of
  its own, its box the seed's name, with the helpers only it uses (rule A
  places them there). A whole file keeps its seed in its one row. So an
  executable's `main` is never left off the map by a box near-tie.
- Speed mode (owner): build and vet only; tests for the cleanup pass are
  listed in the scratchpad `impl/cleanup-todo.md`.

## 2026-09-28 — A call that starts another program is an outward boundary named by its word (runs another program)

- **Measured before code** (`runsprog/`: probe.py, analyze-A.txt,
  analyze-final.txt, variants.jsonl, program-probe*.json): the saved
  atlas_api questions of the newest Redis (070208, 50 windows) and
  litestream-v24 (070341, 170) runs and the dry questions of a repomap
  self enumeration (`--no-model --target …cmd/repomap`, 536 symbols: 29
  handed, 77 given, 430 other), jev-1.13.0, margin 0.10: one draw of all
  2,029 questions under the first draft, then variants on the questions
  that moved, then three draws of the 63 symbols that changed, carry an
  entry or were answered runs_program under the final wording, plus five
  control draws of Redis's entries. About 17.0 MB sent (≈ 4.4 M tokens,
  ≈ $0.19).
  - **Harm found and avoided:** the task's direction sentence ("an entry is
    what this program receives; words it gives another program it starts,
    or sends to another program, are not its entries") in `api.md` and in
    `request`'s Not for / `none`'s Includes flipped Redis's command table
    (`redis.c.redisCommand.proc`, 95 requests) from `request` (control
    leads 0.21–0.38) to `none` (0.60 in 5 of 5 draws with the entry-file
    sentences; near-ties with the api.md sentence alone). Not adopted. The
    final wording: `api.md` says entries come in "to this program" and
    outside systems include "the programs it starts"; `command` is not for
    "the command line this program gives another program it starts, or the
    name of a program it looks up or starts". Redis's table stays
    `request` in 13 of 13 draws (leads 0.10–0.32; one at 0.10), `strcmp`
    `command`.
  - **What changes:** `exec.CommandContext`, `exec.Command` and
    `subprocess.run` talks none → `runs_program` (1.00 in every draw);
    `Cmd.Run`/`Output`/`CombinedOutput`/`Start` none → `runs_program`
    (leads 0.14–0.64), folded by code (below); `exec.CommandContext`'s
    enters stays `command` (0.24–0.30) and is refused beside the launch;
    self `exec.LookPath` enters `command` (0.20–0.32) → undecided (no
    `clang`/`node`/`code` inputs). Near-margin talks shifts in litestream
    under every wording: promhttp.Handler none(0.12) → serves (0.10–0.14,
    2 of 3), sftp `clientConn.Close` undecided → client_request, nats
    ObjectStore Get/Put and azblob.NewClient undecided → sdk, sftp
    File.Seek and Server.Shutdown undecided → none. Every other accepted
    entry kept its kind: pthread_create continuous, AddTool/HandleFunc/
    Handle request, the three smithy middlewares, svc.Run extension,
    flag.FlagSet.* command (leads rose), NewTool request,
    setuptools.Extension extension, self's flag and ServeMux entries.
  - **Program question** (10 calls, 3 draws, then 4 with the package-runner
    wording): litestream → `litestream` 1.00, `git` → git, `go list …` →
    go, `system("make -n")` → `make -n`, a path variable → not_named,
    `sh -c "git status"` → `git status` (0.49–0.60), `python -m pip` →
    pip (0.36–0.45), `sudo apt-get` → apt-get, `npx eslint` → eslint.
- **Talks `runs_program`** (`api_talks_options.md`): the call starts
  another program as a separate process, or names the program and the
  arguments of the command a later call on its result starts; not a
  thread, a fork of this program running its own code, code evaluated in
  this process, a lookup of where a program is installed. `none` no longer
  lists processes. `repomap.atlas.api.v8`.
- **Contradiction**: an entry kind beside any talks answer but none
  (serves, runs_program, client_request, db, queues, sdk) is refused alone,
  `cell_rejected`, and the talks answer stands (`talksStands`, was
  `listenerStands`).
- **One boundary per launch**: every call to a runs_program symbol is an
  outgoing `runs_program` boundary unless its receiver is the
  `call_result` of a call to a runs_program symbol (Go's `cmd.Run()` on
  `exec.Command`'s command). Not sent to the boundaries table (no address,
  catalogue destination or line).
- **`atlas_program`** (`repomap.atlas.program.v1`, Jev, `Memoize`): one row
  per launching call — symbol, the call as written, `words` (every
  nameable word, each once, in call order, as w-refs whose option names
  are the words); options the words and `not_named`; every word carries
  the `word` criteria (`table.Column.EachCriteria`, in the memo's
  questions digest). Rows grouped per symbol. A call with no word is not
  asked and stays not established (a program in a list literal is no call
  word). `Boundary.Destination` = the chosen word as written,
  `ProgramNotNamed` for not_named; atlas 17, GroupsIndex 21.
- **Report**: KindLabel "Runs a program"; a program is one destination
  only by its exact word (never canonicalized), not-named ("A program
  named at run time") and undecided ("Program not established") are their
  call's own; a record's line is the callable and every word its call
  writes. Russian UI strings added.
- **Own executables (step 6) not done**: the cross-program joints
  (`atlas_joints`, decision 14) pair an outgoing boundary with an incoming
  boundary of another target; a program's own launch is no boundary
  (seeds are declarations) and targets carry no executable name (Go is
  named by import path, Python by module, JS by package; only C's make
  names are executables). Offering "this launch names target X" needs an
  executable name per adapter and a new joint side plus a joints-prompt
  change: a redesign. Nothing links by equal strings.
- **`literals[:6]`** (reading/api.go): the program question sends each
  call's own words whole. The cap still cuts the symbol-level `literals`
  field of every atlas_api row (all three sets); removing it re-asks every
  symbol with more than six distinct literals and grows rows such as
  `fmt.Errorf` (617 word calls in litestream). Left for a measured change.
  (`places` also dedups a call's literals; lines.sideValue bounds joint
  values by maxValues.)
- **dynamic_execution**: `system`, `popen`, subprocess's `run`/`call`/
  `check_output`/`check_call`, child_process's `execSync`/`execFile`/
  `execFileSync`/`spawn`/`spawnSync` and JavaScript's `exec` are gone
  from `internal/facts/risk.go`, and C files have no dynamic execution.
  Kept: `exec` and `eval` (code evaluated in this process), `new
  Function`, and pickle/yaml/marshal `loads`/`load` (deserialization runs
  code in-process). The CONSTITUTION's facts sentence still lists
  `subprocess` and `os.system` as dynamic execution examples: for the
  owner.
- **Fixtures**: Go `Revision` (`exec.CommandContext(ctx, "git",
  "rev-parse", "HEAD").Output()`) and `RunHook` (`exec.Command(hook,
  args...).Run()`); C `tools/dump.c` `popen("sort -u", "w")` beside kvd's
  `system(hook)`; Clojure `(shell/sh "git" "rev-parse" "HEAD")`; Python
  `subprocess.run(["git", …])` (asked talks; list words are no call words,
  recorded in PYTHON.md); JS `spawn("git", …)` from `node:child_process`
  (no declarations, not asked, recorded in JSTS.md). Go fixture lines
  after the new import moved by one (http_registrations_test).
- **Frozen** (sha256): `entry_options.md` `e2c9adf4…533b3`, `api.md`
  `35e07898…75d08`, `api_talks_options.md` `11c063c7…76c7f`,
  `api_publishes_options.md` `ceee0c0c…ceb63` (unchanged), `program.md`
  `86638d5f…27733`, `program_options.md` `7555b2a8…6f397`; asks talks
  `f538b957…6f2f4`, enters `62b3db34…7af29` (both unchanged), program
  `3ade7198…4d9f2`.
- **Acceptance** (`runsprog/accept/`, ordinary runs `--no-serve --no-open`
  on the default system cache, keys by the sed recipe; no `cache clear`):
  - **Redis 1.3.6**: exit 0 in 17.7 s (11 atlas_api Jev windows live, 266 k
    tokens), warm 15.3 s with 0 live; render byte-identical. Inputs 96 / 11
    / 6 / 0, unchanged; no program launched (fork only).
  - **litestream v24**: exit 0 in 41.7 s (65 atlas_api and 1 atlas_program
    Jev windows; boundaries 4, core 2, joints 11, publish 1, glossary 1,
    orientation 1 DeepSeek), warm 8.3 s with 0 live; render byte-identical.
    The inputs at the `exec.CommandContext` sites: 11 → 0 (mcp.go ×8,
    validate.go ×3). Launched programs: cmd/litestream `litestream` ×9 (its
    MCP tools run its own binary), not established ×2 (replicate.go:376
    `exec.CommandContext(ctx, args[0], …)` and :380 `c.cmd.Start()`, whose
    receiver is a struct field, so it is not folded); cmd/litestream-test
    `litestream` ×3 (`restore -config -o`, `restore -o`, `ltx`), not
    established ×1 (validate.go:124 `CombinedOutput` on a command chosen in
    an if/else, not folded); etc.s3_mock not established ×1
    (`subprocess.run([...])`). No cross-component link (step 6 not done).
    Inputs: cmd/litestream 93 → 115 (`FlagSet.Var` command, lead 0.50: +8
    options; `CallToolRequest.RequireString` request, lead 0.15, a near-tie
    in pass 1: +14 MCP tool argument names, a side effect of the wording
    for the owner), cmd/litestream-test 34 → 37.
  - **repomap self snapshot** (`git clone --depth 1` of 26ec3762, `--target
    …::…/cmd/repomap`): exit 0 in 121 s, cold (the checkout name
    `self-snap` shares no cache with `repomap`: about 8.8 M input tokens,
    Jev role_helper 3.0 M and role_gate 0.5 M, DeepSeek symbols 3.0 M and
    orientation 0.64 M, roughly $1.2). Launched programs: `git` ×4
    (claims, freshness, gitfiles, snapshot), `go` ×2 (`go list -m -json`),
    `clj-kondo` ×1, `python3` ×1 (`python3 -I -S -c <script>`), named at
    run time ×2 (node for the JS helper, python for target discovery: a
    path variable), not established ×8 (clang ×3 and make in cproject, the
    configured extractors, the editor, the browser opener, gitfiles.go:32
    Output on an unfolded receiver). 42 inputs (38 command options and
    subcommands, 4 HTTP handlers), none at a launch; `exec.LookPath`'s
    enters is a near-tie, so no `clang`/`node`/`code` input.
  - **Walk** (headless Chromium 1440×900, loopback 8932, stopped): the
    litestream component catalogue lists `litestream · 9`, `litestream ·
    3` with `CommandContext litestream restore -config -o`, and each
    not-established launch alone; cmd/litestream-test's connections read
    "→ litestream 2 · Validate command" and "→ Program not established";
    the canvas holds one frame per program with a `CommandContext` tile;
    no page errors. A word that cannot stand on one line (the embedded
    Python script) is left out of the record line after the self run.
  - **Cost**: Jev ≈ 17 MB probe (≈ $0.19) + ≈ 6.3 M tokens of runs
    (≈ $0.26); DeepSeek ≈ 4.5 M input tokens (≈ $1), almost all the cold
    self snapshot.
- Tests: `TestAnEntryBesideWhatACallDoesWithOtherProgramsIsRefused`,
  `TestACallThatStartsAProgramIsNamedByTheWordItsCallWrote` (litestream-
  shaped and `sh -c "git status"` through a Jev preset; the fold),
  `TestEveryLanguageAsksItsLaunchingCallsWithTheirWords`,
  `TestOutboundProgramsAreOneDestinationOnlyByTheirWord`. Revert checks
  skipped at the owner's request.
- `go build ./...`, `go vet` on the changed packages and `go test` of
  contracttest, atlas/..., report, facts and groupindex pass. Full `make
  test`, `make ui-test` and `make ui-visual-test` were not run (owner:
  speed over green commits).

## 2026-09-28 — What each language does not have yet, the wording frozen, and pass 1 accepted on Redis and litestream (inputs pass 1, D1)

- **Missing equivalents recorded** in C.md, GO.md, PYTHON.md, JSTS.md and
  CLOJURE.md ("Inputs a call's words declare, and what … does not have
  yet"): per-call argument-vector words, `switch`/`==`, tables of names,
  registries, S1 (C's in pass 2; not enabled elsewhere), and the two gaps
  the fixtures showed: Go's package-level initializer calls no recorded
  outside symbol (`flag.Bool`), and an npm package without its
  declarations names no symbol (commander, express). CURRENT says every
  language has S2a and C's S1/S2b/S3 wait for the owner (pass 2).
- **Frozen** (no edits after this; a held-out check measures them):
  - `prompts/entry_options.md` sha256 `6e10fd335c13c65cf429e57f007220861ad4656699f3dca5652792842ff47fa7`
  - `prompts/api.md` `12f18dbda3e30db7045de59e4b0c19fa02ea65ff44253379237db82887e0d4b5`
  - `prompts/api_talks_options.md` `a88e76d99d599c51b1954aa0380f48b99298fdd88b66d06803462d56f327894f`
  - `prompts/api_publishes_options.md` `ceee0c0cebd56d56796f65afd8262e49ed2f80ea7ea73e75871ad64d3a9ceb63`
  - asks (sha256 of the text): binds `cbfbc988…3192a`, publishes
    `68645ee6…6cd5c`, talks `f538b957…6f2f4`, enters `62b3db34…7af29`
    (`inputs/pass1/freeze.txt`). `inputs.md` does not exist yet (pass 2).
- **Acceptance** (`inputs/pass1/`: base/, after-first/, after/, merge/,
  walk/, cacheclear/, pass1-revert.log). Ordinary runs `.bin/repomap <repo>
  --no-serve --no-open` on the default system cache, keys by the sed
  recipe. Baseline at 1d31976f: Redis 6 s and litestream 8 s, 0 live.
  - **Redis** (first run at c9ca55bd, A3 before the registration-words
    fix: exit 0 in 23 s, 11 atlas_api Jev windows and 3 joint windows live
    once; at 28a57619 18 s and 11 s, 0 live; report.json identical but for
    `timing`; render byte-identical for both). Inputs: redis-server 96 =
    96 (95 requests, 1 continuous); redis-cli 0 → 6 and redis-benchmark
    0 → 11 command options, handler not established; redis-check-dump 0.
    Enters answers: `strcmp` command (lead 0.64); the other 16 word-given
    symbols none. Every one of `strcmp`'s 18 word-given calls compares
    `argv[i]` in a `parseOptions` (the two `-h` sites of redis-cli's are
    one input); `strcasecmp` (67 calls: config directives, `rc->name`
    "monitor", command options) none, `printf` (80) none, `fprintf` none,
    so no comparison or format symbol turns program data into an input.
    GET's reading is byte-identical (`walk/get-before.txt`,
    `get-after.txt`). In-view arrows at rest: redis-server 20 → 15 (its
    own frame and its 2,543 saved connections are unchanged; the drop is
    the neighbours' layout after redis-cli and redis-benchmark gained
    input collections, `walk/rest/*.png`), redis-cli 4 = 4,
    redis-benchmark 5 = 5, redis-check-dump 1 = 1. report.html 12,172,739
    → 12,203,558.
  - **litestream** (first run at c9ca55bd exit 0 in 130 s: 36 atlas_api
    Jev windows, 7 boundaries, 2 core, 6 joints, 1 publish, 4 glossary and
    1 orientation DeepSeek calls live once; at 28a57619 11 s with one
    boundaries window live, the 8 re-worded registrations' names; warm 9 s,
    0 live; report.json identical but for `timing`; render
    byte-identical). Inputs: cmd/litestream 33 → 93 (lost: 14 `X.Usage`
    commands and 3 errgroup goroutines, by the merged criteria; gained 69
    command options and flag-set names and 7 MCP tool requests with their
    handler not established, and `svc.Run`'s Windows service as an
    extension: re-asked with its call as usage it no longer serves, so it
    is an entry instead of a listener); cmd/litestream-test 6 → 34; the
    python library target 0 → 1. Enters answers taken: `flag.FlagSet.*`
    command (leads 0.20–0.68), `flag.NewFlagSet` command (0.46),
    `mcp.NewTool` request (0.71), `os/exec.CommandContext` command (0.53),
    `setuptools.Extension` extension (0.32); `net/http.Handle` and
    `ServeMux.Handle` request (0.96) beside serves, refused (the listener
    stands). Near-ties saved undecided: `RequireString`, `FlagSet.Var`,
    `WithBoolean`, `Client.Post`, `NewRequestWithContext`. Symbols with
    more than 50 word calls: `fmt.Errorf` 617, `fmt.Sprintf` 98,
    `slog.Logger.Debug` 73, `slog.Info` 54, all none. Arrows at rest 16 =
    16 and 3 → 2. report.html 10,720,273 → 12,324,904, mostly the
    glossary drawn again live (47 → 81 terms).
  - **Misses for the owner** (not fixed by wording; the freeze stands):
    `exec.CommandContext` command makes 10 inputs of the `litestream`
    command lines the MCP tools and the test tool launch;
    `flag.NewFlagSet` command makes 20 inputs of flag-set names
    (`litestream-list`, one per subcommand; the criteria call the
    program's own name none); `setuptools.Extension` extension makes one;
    an MCP tool is two inputs (its `NewTool` name and its `AddTool`
    handler), and argparse's `add_parser` + `set_defaults` would be too.
    Eight socket options were named by all their words
    (`socket /var/run/litestream.sock control socket path`): the name
    question returned no word.
  - **Walk** (headless Chromium 1440×900, loopback server on 8931, stopped
    after): `-h` reads "handler not established", "declared in Benchmark
    client", `redis-benchmark.c:428`, Main flow; litestream's `config`,
    `timeout` (7 inputs, one per subcommand) and `litestream_databases`
    read the same way; no input tile of these has an arrow; no page
    errors. Choosing such an input does not frame its tile (it has no
    path); left for the UI.
  - **`repomap cache clear`** on a scratch `--debug-dir` cache over the
    kvd fixture only: cold 48 live (atlas_api 4 windows), warm 0 live
    (46 api rows recalled, 0 windows), clear exit 0 (2.0 MB), then 46
    live again (atlas_api 4 windows). The system cache was not cleared.
  - **Cost**: Jev about 1.23 M input tokens of atlas_api (≈ $0.05) plus
    the merge probe (≈ $0.02); DeepSeek about 0.39 M input and 0.04 M
    output tokens (≈ $0.1–0.15), most of it litestream's orientation and
    glossary.
- `make test`: PASS. `make vet`: PASS (documentation only; no UI change).

## 2026-09-28 — A call given words becomes the entry its symbol makes of them (inputs pass 1, A3, S2a)

- **The third question set** (`repomap.atlas.api.v7.given`): a symbol
  handed nothing whose calls outside tests give it words is asked `talks`
  and `enters` (both `Alone`); `enters` offers the entry kinds but
  `queue_consumer`, and `none`, from the one criteria file (correction 3:
  no outcome in both columns). Its `usage` is its first word-giving call.
  Every row may carry `result_receives`, the calls made on the symbol's
  result, counted (Python and JS receivers are `call_result`s anchored at
  the producing call; Go, C and Clojure record none in the fixtures). An
  entry kind beside `serves` is refused alone (`cell_rejected`): the
  listener answer stands.
- **Entries** (correction 4, renamed by the owner's remark: the handler is
  *not established*, `handler_unknown`, never "no handler"): a word-giving
  call outside tests, to a symbol whose words are an entry, that no fact
  boundary (in or out) names at its line and column, becomes a model
  boundary, direction in, words = the call's literals; a registration the
  code found that hands nothing but words, and a value handed without a
  callable, become the same kind of entry. A registration's own values
  keep only its address when a literal is one, so its entry takes the
  words of the call at its site (`fs.String("socket",
  "/var/run/litestream.sock", …)` had been named by the socket path in the
  first litestream run). No subject, reach or phase;
  keys, the part side, core's entry counts and a holder's address skip
  it. Named by its one word in code, else asked; with no accepted word, by
  all its nameable words as written (never the caller). No nameable word:
  no entry, `entry_unnamed` journaled. GroupsIndex 20 keeps model entries
  (the dead skip at the old l.678-682 is gone), marks `handler_unknown`,
  gives it the declaring part's group and dedupes by (kind, words,
  caller) at the first site; an invalid name refuses that entry alone.
  Atlas 16 (`Boundary.HandlerUnknown`, `APIRole.Enters`).
- **Report**: such an input has no `InputOwner` and no "implemented in"
  arrow, stands loose in its collection, is not listed on its part's card,
  is no route and joins no portal; its reading says "handler not
  established" and "declared in {part}" (en/ru).
- **Fixtures**: Python `src/fixture_app/tool_cli.py` (argparse parser,
  option, subparsers, `add_parser("init")` + `set_defaults(func=run_init)`),
  JS `src/cli.ts` with `commander` in package.json, Go package-level
  `var verbose = flag.Bool(…)` at the end of storefixture/destinations.go;
  Clojure reuses `(format "create %s dir" dir)`. Measured with
  `TestEveryLanguageAsksItsWordGivenCallsTheEntryQuestion` (dry reading of
  each fixture's requests):
  - C: `strcmp(argv[1], "--symbols")` given; pthread_create binds; accept
    talks.
  - Go: `flag.String("price-endpoint", …)` given. **Gap**: the
    package-level `flag.Bool` is not asked — the external call index does
    not record a package-level initializer's outside calls (f1b did it for
    repository calls only). Recorded, not fabricated.
  - Python: ArgumentParser (`add_argument ×1`, `add_subparsers ×1`),
    add_argument, add_subparsers (`add_parser ×1`), add_parser
    (`set_defaults ×1`) all given; `set_defaults(func=run_init)` binds.
    **Result**: a subcommand named by one call and handled through another
    is two questions, and two inputs if both are accepted.
  - JS: platform calls given (`new Worker("./market-worker.js", …)`).
    **Gap**: an npm package without its declarations names no symbol, so
    commander's `.option("-p, --port <n>")` (a registration fact) and every
    express route in the fixture are asked nothing. Recorded.
  - Clojure: `(format "create %s dir" dir)` given.
  - No symbol is asked in two question sets (the helper fails otherwise).
- **Undecided words** (correction 8): model entries are made in the
  reading after the api answers, and a word-giving registration is
  direction out in the graph, so roles.go's `registered` never holds an
  undecided word; the kvd preset asserts no `registered` item carries
  `--symbols`.
- Tests (each fails on revert or mutation, `inputs/pass1-revert.log`):
  `TestNoOutcomeIsOfferedInTwoColumns`,
  `TestAWordGivenCallBecomesAnEntryWhoseHandlerIsNotEstablished`,
  `TestTheEntryWinsOverTalksOnlyAtACallGivenWords`,
  `TestAModelEntryWhoseHandlerIsNotEstablishedIsAnInput` (dedupe of two
  "-h" sites), `TestNoEntryAtACallAFactAlreadyNames` (echo, the skeptic's:
  one db boundary at QueryRowContext, no input in users.sql.go; its
  statement is also unnameable, so the claim itself is pinned by the
  reading test's `SELECT 1`), `TestEveryLanguageAsksItsWordGivenCallsTheEntryQuestion`,
  `TestAnInputWhoseHandlerIsNotEstablishedBindsToNoPart`; the kvd preset
  now answers `enters` and asserts which symbols were asked.
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS (127).
  `make ui-visual-test`: PASS (65 passed, 5 skipped).

## 2026-09-28 — An outside symbol's call as written, and each symbol's answer remembered (inputs pass 1, A2)

- **Usage is the call** (queue item c, trusted inputs): `lines.CallText`
  lexes C, Go, JavaScript/TypeScript, Python and Clojure (comments, strings,
  Go raw strings, template literals, triple quotes, Clojure characters) and
  sends the call at the adapter's position with its receiver chain
  (`KafkaConsumer().subscribe(…)`, `path.split("/")[0].split("/")`,
  `program.option(…).action(run)`), generic arguments and JavaScript's
  `new`, through its closing parenthesis; a table row or Clojure form when
  the position is no call, an assignment's statement
  (`act.sa_handler = onSignal`, `fs.Usage = c.Usage`). Comments dropped,
  whitespace folded, no line number and no length cap (correction 2): the
  call goes whole and a row too large for its request is the request's to
  refuse. Checked against the real anchors of the five fixtures' no-model
  runs (`inputs/pass1/anchors/`): Go anchors at the parenthesis, C at the
  name, Python and JS at the selector, Clojure at the form; every call and
  registration rendered as its call or row. `sourceLine` is deleted.
- **Memo per symbol**: `repomap.atlas.api.v7`, `Memoize` on both rounds; a
  row that is no place takes its subject from `rowSubjects`
  (`api:<symbol>`, no targets, no context) in `knowledge.json`.
- **Uncertain rows are remembered** (correction 1): `table.Result` marks a
  row the categorizer answered under the margin; its memo is saved like an
  answered row's, and a recall returns it found and undecided, so a warm
  reading asks nothing and never draws a near-tie again.
- **The basis pins the questions**: `table.MemoIdentity` adds `questions`
  (a digest of each column's `Ask`, `Item`, options and criteria), only for
  a table with such a column; the symbol selection's basis is unchanged.
- `read --through api` stops a reading after the outside symbols.
- Tests: `TestCallTextIsTheCallAsWritten`,
  `TestCallTextSendsNothingItCannotRead`, `TestCallTextKeepsALongCallWhole`,
  `TestCallTextFollowsIndexesGenericsAndNew`,
  `TestAnOutsideSymbolsUsageIsItsCallNotItsLine`,
  `TestAWarmAtlasAPIRereadMakesNoLiveCall` (with an uncertain row),
  `TestAtlasAPIAsksOnlyTheSymbolANewCallAdds`,
  `TestMemoBasisChangesWithAnOptionsCriteria`,
  `TestAReadingCanStopAfterTheOutsideSymbols`; each fails on revert or on a
  mutation of the rule it pins (`inputs/pass1-revert.log`).
- Cache: every atlas_api row is asked once more (new usage, contract and
  prompt); the pinned Jev request golden is a synthetic table and is
  unchanged.
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Every entry question reads one criteria file (inputs pass 1, A1)

- `internal/atlas/lines/prompts/entry_options.md` replaces
  `api_binds_options.md`: the nine options an entry question may offer
  (the seven entry kinds, `middleware`, `none`) with generic examples.
  `lines.EntryCriteria(names…)` gives any entry question its subset and
  panics on a name the file does not define; `binds` reads it. Merged from
  the binds criteria and the inputs probe's v4 wording: `request` excludes
  reading or writing a connection an earlier entry accepted; `scheduled` is
  a timer the program keeps (a one-shot delay is none); `none` names
  printed text, the program's own name, string comparison and conversion,
  a thread that does one piece of work and ends, and signal, exit and
  failure handlers.
- **Measured before A2** (correction 5; `inputs/pass1/merge/`): the saved
  round-1 `binds` questions of four ordinary runs on the default cache
  (Redis 4, litestream 23, python-tutorial-game 4, pykrx 6 symbols), five
  draws under the merged criteria against five control draws under the
  criteria they were asked with, jev-1.13.0, margin 0.10, about 1.9 MB
  (under $0.03). Every accepted entry kept its kind in every draw
  (pthread_create continuous 0.96, the command table's field request
  0.88–0.93, AddTool and HandleFunc request, svc.Run extension 0.17–0.35,
  fastapi get/post request, requestAnimationFrame scheduled 0.19–0.29, the
  three smithy middlewares 0.64–0.84) except two, each for a stated reason:
  - `flag.FlagSet` (`fs.Usage = c.Usage`, litestream): command at a 0.19–0.31
    lead in the control → none (0.14–0.17) or a near-tie (0.08–0.09). The
    handed callable prints the usage text, which the criteria make none. It
    removes 14 `X.Usage` commands from cmd/litestream and 6 from
    cmd/litestream-test; litestream's subcommands are dispatched by a Go
    `switch`, which is no fact (a recorded missing equivalent), so pass 1
    shows their flags (A3) and not the subcommand names.
  - `errgroup.Group.Go` (litestream): continuous at 0.36–0.52 in the
    control → none (0.10–0.18). Its three sites (store.go:176,
    oss/replica_client.go:453, s3/replica_client.go:1436) open one database
    or fetch one object each and end; the criteria's "a thread that does one
    piece of work and ends" is none. It removes 3 continuous inputs.
  No accepted entry disappeared without a reason, so A1 goes on.
- Tests: `TestEveryEntryQuestionReadsOneCriteria` (every entry column's
  options carry exactly the shared file's criteria) and
  `TestEntryCriteriaNameNoRepositoryItem`. Both fail on revert (build), and
  the first on a mutation giving binds its own `none`
  (`inputs/pass1-revert.log`).
- Cache: the round-1 (`binds`) request bytes change once; `talks` and
  `publishes` are unchanged.
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Moved from REPORT and READING: run measurements and Redis narratives (f4)

- **CURRENT.** The "Owner decisions of 2026-09-28" bullet is split: the
  owner decisions keep what the owner said (the helper question asked
  once, shared helpers in a second pass, helper arrows quiet like init, an
  area purple when any part inside is the domain, types after keys, the
  DNS group stays, split before grouping, no utils part); a new "Lead
  decisions awaiting the owner's review" lists rule B read as "a file of
  helpers", the all-quiet exception, depth layering in input paths (step
  4's "accepted" now says so), the "init" label covering launch and the
  main loop, an off-map seed named in the column, S4-6 deleting chains,
  C4 shipped despite the one pykrx miss, the no-users rule with
  `recordedUses` and u5's depth-1 fold.
- **Contracts.** Redis mentions (case-insensitive lines): REPORT 49 → 0,
  READING 31 → 1. The one left is READING's "The probe's six saved
  grouping answers over units (Redis and pykrx, three draws each,
  `testdata/units-replay`)": it names the checked-in replay files
  (`internal/atlas/reading/testdata/units-replay/redis-d{1,2,3}.json`) that
  `TestSavedUnitsAnswersKeepEveryUnit` requires, not a run. Every sentence
  of the two contracts with "must" or "never" at 170d212f still stands,
  word for word or with only a run's narrative taken out of it (a diff
  read of the 14 that changed: 6 in REPORT, 8 in READING). The
  input-reading paragraph merged with this day's u2, u5 and u7 rules; its
  example "Dispatched from call · one of 94 handlers" became kvd's
  "Dispatched from processCommand · one of 6 handlers" (kvd.c's
  `cmd->proc`, six command handlers). "HEAD" in the quotes below is
  e8b4361c, where they were taken from.
- `make test`: PASS. `make vet`: PASS (documentation only; no UI change).
- `REPORT.md`, § External communication and data (77d821aa): the
  Entrypoints link's history moved; its rule kept.

  > It had landed on the inputs, where a reader looking for the program's start found none.

- `docs/contracts/REPORT.md` and `docs/contracts/READING.md` state rules.
  The run measurements, pixel observations from runs, histories of how the
  page used to look and the narratives of analysed repositories (Redis 1.3.6
  above all; also pykrx, litestream, Freqtrade, microblog, etcd, xk6-dns
  and the repomap self-run) that had gathered in them are kept here
  verbatim, one bullet per passage with its section. The rules they
  illustrated stay in the contracts with the same meaning. Where a Redis
  example was replaced, the new one comes from the C fixture
  (`testdata/repositories/c`: kvd, kvcli, `net.c`, `loop.c`,
  `staticsyms.h`) and was checked against its tests
  (`internal/contracttest/c_*_test.go`, `map_of_parts_test.go`,
  `internal/run/repository_target_c_test.go`), or from the UI vocabulary's
  own placeholders (`internal/report/ui_vocabulary.go`).

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 52–54):
  Redis example replaced by kvcli's part of `loop.c` (the C fixture's client
  links the event loop and never calls it).

  > is not a group, draws no arrow and leaves that program's canvas; redis-cli's
  > "Linked list", whose thirteen adlist functions redis-cli links and never
  > calls, is not drawn.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 126–129):
  location-row example `redis-server` replaced by `kvd`; the history of the
  heading and the window sizes it was measured at moved.

  > row ("Inputs · redis-server (executable)"). Headed with its component's
  > name, Redis's collection had read as a second redis-server beside the
  > programs. It stands attached to its component, in the layer next to it
  > with its arrow straight into it (measured at 1440×900 and 1280×800).

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 139–144):
  Redis's opened-collection history and tile counts moved; the rules on
  loose inputs and tile kinds kept.

  > areas open to their parts. Opened with the collection, Redis's 95 inputs
  > had stood as a wall of 5px tiles under 5px group names. An input with no owner stays loose after the groups, and a collection
  > whose inputs share one part keeps them loose. A tile names its kind only
  > when it is not the collection's most common kind: "Request" on 97 of Redis's
  > 98 tiles repeated the collection's own summary. The groups are display
  > containment, not architectural areas; they add no relation.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 157–162):
  Redis's shared "TCP endpoint" history and the three "DNS resolver" frames
  moved; the rule now says the grouped frames stand side by side.

  > equal destination text proves no identity. One "TCP endpoint" box had taken
  > arrows from all three Redis programs, though for redis-cli that endpoint is
  > redis-server and for redis-server its master. Frames of different
  > components that name the same destination stand together in one display
  > group, an amber frame around them in the outer layout, so the three "DNS
  > resolver" frames stand side by side instead of scattered. The group is no

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 168–169):
  Redis's repeated "DNS resolver" headings (history) moved.

  > zoom marks, one per program, each hit by its own program's arrow. Three
  > "DNS resolver" headings side by side had said one thing three times.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 177–182):
  where Redis's arrows ran and the repeated tile titles (history) moved.

  > tiles when arrows run down and after them when they run right; above the
  > tiles, Redis's three arrows ran through it. Like closed summaries it is
  > laid out at the whole-map camera and zooms with the map; once the tiles
  > open it reads at their open frames' title size and the open tiles stay
  > plain under it, so the destination is named once in every state. Titled
  > one by one, Redis's three open DNS tiles said "DNS resolver" three times.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 187–188):
  the entered-tile history moved.

  > the scale its calls are drawn at: framed alone, an entered tile had read
  > only "gethostbyname" with the heading below the camera. The tiles stay

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 192–196):
  Redis's DNS tile sizes and mark ratios at two window sizes (measurements)
  moved.

  > with it, so it opens with no room of its own below them. Stretched to the
  > mark's proportion and grown to its 50 by 44 pixels, each of Redis's DNS
  > tiles opened half empty below its one call; kept at its calls' own box
  > without growing, its mark was drawn at 0.64 of the size of the TCP
  > endpoint's beside it on a 1440×900 first screen and at 0.41 on 1280×720.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 212–214):
  the earlier cut entry arrow (history) moved.

  > shaft; cut from a 14px square, its shaft ran along the border in the
  > border's colour and only a small head read. The legend draws the same
  > arrow.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 221–229):
  Redis's Networking/Core infrastructure history and the 17-of-29 count
  moved; `redis-server's main` generalised to "a `main` undecided between
  two parts".

  > requests or listen do not make their area the entry: Networking's
  > listen/bind boundary had drawn Redis's "Core infrastructure" as a second
  > entry area beside Server runtime. The same holds for a part: only the
  > part holding the program's launch point has the entry mark; a part that
  > only takes requests or listens has none, and its inputs' blue arrows say
  > where the outside calls in (17 of redis-server's 29 parts had been
  > green). The component reference's "Input responsibilities" list holds
  > only that part. A launch point
  > no part holds (redis-server's `main`, undecided between two parts) makes

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 246–248):
  the grey-veil history moved.

  > one is. No veil or tile fill marks the pointed thing: a grey veil on the
  > pointed area's parts and a grey fill on the pointed declaration had made
  > them look deader than their outlined neighbours.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 279–280):
  the Data type commands history moved.

  > arrows crossing its border darken. Lifted to its area, a part pointed at
  > in Data type commands had lit all of the area's arrows.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 288–290):
  Redis's GET arrow counts (20, 14, 43) moved.

  > ordinary arrows. On Redis GET draws 20 arrows where its single shortest
  > witnesses drew 14; the neighbourhood of every call among its parts, which
  > this rejects, would be 43. GroupsIndex marks an arrow quiet

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 297–298):
  Redis's benchmark, client and dump checker history moved.

  > its reply helpers), and only in a target that serves something: Redis's
  > benchmark, client and dump checker had drawn none of their arrows. One

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 320–323):
  the earlier key glyph colours (history) moved.

  > in the tiles' own purple and grey head. Its glyphs had been painted in
  > the marks' dark colours, so Redis's pale green and purple cards matched
  > nothing in it, and no line was keyed; its tiles' purple dashed links
  > went unexplained. There is no folded prose legend

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 334–341):
  Redis's GET camera narrative moved.

  > outlined in the dark of the path's arrows. Framing its tile had shown GET
  > as one of 98 tiles with no arrow in sight, and centring String commands
  > showed four of its nine dark arrows and none of the parts they reach, drawn
  > like every other part. In Redis the handler's first step, Client
  > connections and replies, stands farther from String commands than the
  > canvas holds at a readable scale: the camera frames String commands,
  > Object and key store, Server configuration and Sorted set commands, six of
  > the nine dark arrows, and leans toward it. An

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 346–347):
  Redis's GET/SET example and counts moved.

  > takes, the group it stands in, never the whole collection: GET among
  > String commands' fourteen inputs, SET beside it, not a wall of 95. A group

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 353–392):
  the GitHub-link histories, Redis's
  get/exec/loadAppendOnlyFile/spawnIOThread narrative and the old
  contradictory lines moved; the dispatch strings now use the C fixture
  (kvd's `processCommand`, one of 6 command functions, reached from no
  input) and the vocabulary's placeholders `{site}`, `{input}`, `{n}`; the
  given text is kvd's `kvd.h.kvCommand.proc get in getCommand`.

  > opens its code (it had opened GitHub above the same name in the path). A
  > registration the model did not explain has no line: its given text
  > ("redis.c.redisCommand.proc get in getCommand") only said the fact again,
  > and the reading and Find name its handler. A chosen input's
  > reading opens at its path (owner's choice 3c, 2026-09-27), drawn in the
  > Inputs blue of its tile and collection, never core's purple: its
  > heading's bar and kind and its links. The path projects GroupsIndex's
  > saved reach and dispatch sites; the page walks no code. It is, first, one
  > box per dispatch site whose alternatives hold the handler, the first open:
  > "Dispatched from call · one of 94", holding the dispatch fact and the
  > statement "How a request for get gets to call is not established.": how
  > a request for the input arrives at the dispatch site from outside;
  > Redis's get is
  > dispatched from call and from loadAppendOnlyFile. No route to the site is
  > drawn: the shortest static chain from the program's entry, Redis's main →
  > aeMain → beforeSleep → call, had been offered to a benchmark reader as
  > GET's path. The inputs whose own code reaches a site (exec, lpush,
  > rpoplpush, rpush and slaveof reach call) are not listed in a dispatched
  > input's reading, where a reader takes them for its route; they are the
  > site's own reading, with the declaration: "call is reached from these
  > inputs:", each input a button to its reading with its calls to the site,
  > then "Which of these, if any, leads to an input dispatched here is not
  > established.", or "No input reaches loadAppendOnlyFile by calls". An
  > input whose own code reaches a site says so in its reading, as its
  > handler's own call back into the site ("exec's handler itself calls call,
  > where 95 inputs are dispatched:") with those calls; the two lines had
  > read "How exec reaches call is not established." beside "Reaches call",
  > which contradicted each other on their face. An input a
  > running declaration of another input's reach hands over reads "Registered
  > by" with those inputs (Redis's IOThreadEntryPoint, handed over by
  > spawnIOThread); the other reads "Registers". Then the parts the input
  > enters, nearest the handler first: each part's name (a link to its
  > reading when the map draws it), every call entering it from a part
  > reached earlier, the first five and the rest folded under "+N", and "{n}
  > other calls into it on this path". A call is its two declarations' names,
  > with no line number, a read or a possible call marked as elsewhere; a
  > name in a drawn part reads that declaration there, as a click on its tile
  > does, and a modifier-click opens its code; a name in a part the map does
  > not draw is only named. getCommand and addReply had opened GitHub for a
  > reader following GET's path. The words "Shared by" and "through" are gone.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 397–400):
  the late-drawn line history (26 px, monitorCommand) moved.

  > never from what the pointer is over: drawn from the canvas's emphasis it
  > came a frame late and again each time the pointer left the canvas for the
  > column, pushing Introspection and debugging's list 26 px down under a
  > reader's click, and monitorCommand was read for pingCommand. Nothing in

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 409–412):
  Redis's component-numbering history moved.

  > does: read by its areas alone, the component's own space was in no frame,
  > and on Redis, where entering redis-server opens four components, its
  > numbers stood only while an area was pointed at and vanished as the
  > pointer crossed that space to reach them. A closed component numbers

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 435–437):
  the Data type commands / Server runtime history moved.

  > frame's, so Data type commands' incoming numbers stood on Server
  > runtime's border, in the gap where the pointer looks at the whole
  > component, and could not be reached. The two directions' labels then

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 444–445):
  the folded-legend history moved ("had room" now reads "has room").

  > the key, where the row had room: folded into a legend at the bottom of a
  > 900 px window, no reader of Redis's map found it.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 460–461):
  the tester's arrowhead history on Redis moved.

  > no card. On Redis the tester's rests on arrowheads opened nothing, and a
  > click there fell through to the frame underneath and moved the camera.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 466–467):
  the card-over-the-head history moved.

  > border: placed outside the frame alone, the card opened over the head the
  > pointer rested on.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 471–472):
  the 291-call card at 10 px beside Redis's Core infrastructure
  (observation) moved.

  > changes: drawn at the parts' scale, a card of 291 calls read at 10 px
  > beside Redis's Core infrastructure. Beside a frame it takes the room

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 479–482):
  the re-targeting history moved (the kept sentence was re-wrapped).

  > chip the pointer has not left at all. Re-targeting on every pointer move had
  > closed the card on the way to it, and with the pointer on it, as soon as
  > it stood outside the frame. A click on a part's number or on a card keeps
  > the card open; a click on an arrow end, its chip or its arrowhead, reads,

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 506–512):
  Redis's `call`/`c->cmd->proc`/94/77 example replaced by kvd's
  `processCommand`, `cmd->proc`, six command functions and `cmdTable`; the
  Server runtime history moved.

  > (Redis's call reaches `c->cmd->proc`, one of 94 command functions), and
  > the calls of a declaration that hands every member of such a set over by
  > another relation (cmdTable passes all 94 as callbacks); each caller's
  > marked calls are one line, "call → one of 94 · 77 here", "cmdTable passes
  > callback the same 94 as call · 77 here", with the callees under it by part,
  > each part opening to their names. Server runtime → Data type commands had
  > listed both 77 times, one row each. Which calls belong to a set, and how

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 519–522):
  the Server runtime → Core infrastructure history moved.

  > that number's parts, and the chip's own edge widens it again: narrowed by
  > every number crossed on the way, Server runtime → Core infrastructure's
  > card only ever showed the calls of whichever number the pointer met
  > last.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 543–551):
  the ELK bend counts, the refused entry-side layout and its 14-window
  measurements moved; the rule (one edge per pair, the pair's first) kept.

  > a pair of ends share one drawn route, so ELK lays out one edge per pair:
  > laid out twice, each such pair made ELK reverse one arrow into a
  > wrap-around, and Server runtime's 21 arrows among six parts had 68 bends.
  > That edge is the pair's first. Laid out instead from the program's entry
  > side (a `triggers` part) toward the other, Redis's Server runtime was
  > measured at 14 window sizes and refused: 84 bends became 70 at four sizes,
  > 54 became 70 or the routes ran 9% longer at eight, and at 1440×900 the
  > area took a one-row layout that entered cut in half. Which of ELK's layouts
  > the area takes, and how it wraps, moved more than the pair's direction.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 556–563):
  Redis's Persistence/Server runtime pixel history moved.

  > fewest detours around the area); then the squarer box. Wrapping had run
  > Persistence's one arrow around the area and drawn Server runtime 1300 px
  > wide in a 1214 px canvas. Its parts keep their own size,
  > the size of the loose parts beside it, so an area is as large as what it
  > holds: laid out with the whole component and shrunk to a peer's width,
  > Redis's Server runtime stood as a staircase of postage stamps under a
  > full-size title. Each area wraps its drawing and heading without reserving a
  > second member-list height. A closed group

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 582–595):
  Redis's Debug symbols pixel measurements, the proxy's window and the
  measured alternatives moved; "This is measured and left." now reads "This
  is left.".

  > theirs when their boxes are larger: Redis's Debug symbols read 10 px beside
  > 14.8 to 17.1 px area titles (about 11 px beside 15.7 px in the proxy's
  > window). This is measured and left. The camera caps every closed heading
  > at a scale the whole map sets, so matching the cap needs the interiors laid
  > out again once the whole map is placed. Growing the box in the one layout
  > until its heading fits at its smallest area's heading scale reads 14.9 px
  > there, but the part fills that box once the areas open: beside two areas
  > of fourteen parts it filled 1036 by 739 px beside 260 by 88 px parts (400
  > by 200 px as its card), and Redis's Debug symbols 244 by 143 px beside 128
  > by 61 px parts (196 by 98 px). Once the areas open it is their parts' peer:
  > the same card at the same scale, filling its box, so its title reads at
  > their parts' size. Kept at the summary scale, Redis's Debug symbols read
  > 41 px beside 17 px parts. A component without areas keeps its direct
  > parts' fitted headings.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 621–623):
  the Data type commands 18px history moved.

  > number stands in room its card already leaves. A rule reserving a row
  > under the title for that number had dropped every description of Redis's
  > Data type commands 18px whenever the pointer crossed the area's border.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 638–655):
  Redis's scattered linked-list tiles, the 190px cut names, the Data
  structures tile counts, the repomap self-run's hidden keys and Redis's
  Persistence history moved.

  > the file is a hint, and Redis's linked-list functions had stood scattered
  > among the dictionary's. They
  > stack in that order within their column and a column too tall spills into
  > the next. A tile is as wide as the longest name among the part's
  > declarations, so no name is cut, and the columns share the card's width;
  > cut to 190px, Redis's names read "_dictStringCopyHTKe…". The tiles are
  > drawn at a quarter of the card's scale, or smaller when a quarter does not
  > hold them all whole, however many the part holds: nothing is counted away
  > and no scale is too small to search, and a large part is a larger
  > drawing to pan (at 1440×900 Data structures had drawn 40 tiles and
  > counted 63 away as "+63"). The
  > part's name stands over them at the same screen size at any such scale.
  > Placed by link column first, the repomap self-run's parts hid 88 of their
  > 256 keys while drawing declarations listed after them. The zoom button
  > enters at the scale the declarations read at, their own 13px: the part
  > whole when it fits there, else its head and first column a margin from
  > the canvas's top left. Fitted to the canvas, Redis's Persistence had
  > opened at its title's scale with no declaration drawn. A drag over the

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 670–672):
  the Redis tile-click history moved.

  > restored visit keeps its camera. Clicked, Redis's tiles had opened GitHub
  > in a new tab or bubbled to the part already selected, and a Find code hit
  > had stopped at the part. The page data gives each tile its file and the

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 678–690):
  the wheel and pinch histories (including Redis's readers) moved.

  > past the inventory's end the wheel stays with it, where the next notch
  > had scrolled the page and the one after moved the map under a still
  > pointer. Anywhere else on the canvas, its location row over the map
  > included, an ordinary wheel moves the map and never scrolls the page;
  > over that row it had scrolled the page while a pixel lower it moved the
  > map. One pinch
  > (ctrl+wheel) crosses at most one level boundary, the zoom where the
  > level changes, and stops short of the next, going on or back; a pause
  > of about a third of a second ends it and the next pinch crosses the next.
  > The levels are the whole map, then one per open hierarchy depth, then a
  > part's tiles in sight; two layers that open at one zoom are one
  > boundary. A pinch of eight ctrl+wheel ticks had carried Redis's readers
  > from the whole map past the areas into a part's tiles. The zoom a tick

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 708–709):
  the warning-red history moved.

  > like theirs, in the muted text colour: in the warning red it read as an
  > error, and the key's dashed stroke says what it is. All original endpoint IDs and sources remain on it;

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 734–739):
  Redis's Server runtime heading sizes and microblog's /explore
  (observations) moved.

  > first part at that scale. Fitted at its layer's floor instead, Redis's
  > Server runtime stood at 8px headings at 1440×900 and 4 to 7px at 1280×800. Entering a frame or an input's path
  > opens what it enters: those frames stay open through the camera move's own
  > zooms, so they arrive open although the camera stands smaller than a
  > closed layer needs to open by itself. Closed on the way, microblog's
  > /explore stood on the closed Web routes summary with its title at 45 px.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 757–759):
  the lost-last-letter history moved.

  > included: measured over the participants alone, the camera stood smaller
  > than the fit, and every heading reserved to the pixel lost its last
  > letter.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 770–773):
  the measured-wider history moved (the kept sentence was re-wrapped).

  > less its zoom button's room beside a part's title: measured wider, a line
  > counted whole broke again in the browser and left its last word alone on
  > a line. A part's description is not broken into lines on the page; the
  > browser wraps it in that column, and its lines counted there only size

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 784–785):
  Redis's 1280×720 first screen (observation) moved.

  > Drawn into the short box, Redis's 1280×720 first screen cut "TCP
  > endpoint" below its frame and "Background" out of its input list.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 830–831):
  the emphasised arrowhead pixel sizes moved.

  > declarations. Drawn in the thicker line's stroke widths, an emphasised
  > head stood 17.5px beside 10.5px and covered the number at the frame.

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 854–855):
  the Command dispatch margin history moved.

  > a screen margin below the top. The margin is screen pixels: taken as world
  > units at a close-up zoom it put Command dispatch under the canvas edge. A

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 871–872):
  the Selecting GET history moved.

  > reached through a matched input after them. Selecting GET had lit every call
  > among fourteen parts and listed them starting with Strings. Call depth is a

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 882–894):
  the Command dispatch, Client connections, List commands and Replication
  histories moved.

  > before outgoing connections: Command dispatch had listed some 5,000 characters of
  > connections before its code, and Client connections and replies the
  > nineteen parts it calls, mostly utilities, before the fourteen that call
  > it. What a part is made of (owner's choice 3a, 2026-09-27) is headed by
  > its declarations counted by the kind its tiles carry ("Made of 18
  > functions", "2 functions, 3 types") with the files they are written in
  > once beside it, and lists every declaration of the part as a link into its
  > code, the model's keys first and in bold as the part's tiles draw them,
  > then the rest by name whatever their case, with no line number: by file
  > and line, List commands' eighteen functions read as a column of line
  > numbers. The tiles keep the page's own order. It had listed only the keys
  > under a heading that said all, and Replication's showed
  > replicationFeedSlaves and not syncWithMaster. An area's reading starts

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 909–917):
  redis-cli's anet.c joint locations, the anetTcpGeneri… history and the
  bordered-box history moved.

  > it lands: redis-cli's joint is written at anet.c:158 and lands at
  > anet.c:256, inside anetAccept declared at 248. A name whose part lists no
  > such declaration is only named. The canvas's own card keeps its links:
  > Redis's "anetTcpGeneri…" in the column had opened GitHub for a reader who
  > meant to read it. An input collection is named Inputs there, as its
  > canvas heading is. Each declaration is one line, as a tile is, a type's fields
  > on a line under it, and a key type the model explained keeps its mark and
  > its fields: a bordered box each had put Client connections' callers some
  > 1,700 px further down the reading. The reading column does not repeat the source index, which

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 931–944):
  `initServer passes acceptHandler as a callback` replaced by kvd's `main
  passes acceptHandler as a callback`; the joint example
  `anetTcpGenericConnect connects to anetAccept` replaced by the vocabulary
  form `{caller} connects to {callee}`; the joint-card and full-sentence
  histories and the 97-row count moved.

  > ("initServer passes acceptHandler as a callback"), never the stored kind
  > ("passes_callback"); a C program's import is said as an include, and a
  > joint between two programs names the declarations at both of its ends
  > ("anetTcpGenericConnect connects to anetAccept", not "integrates with").
  > An arrow's card reads each of its calls as caller, relation and callee and
  > links both names, so an arrow's call is those three words: the phrase's
  > words when they stand between the names (the joint's card had shown only
  > "anet.c:158"), the relation's kind when the phrase wraps the callee
  > ("cmdTable passes callback delCommand"; the full sentence showed neither
  > name). A row the model wrote
  > in its own words keeps them. Rows of one caller and one relation kind in
  > one evidence list fold into one line with their count and callees, each
  > row inside it with its sources: Client connections and replies listed
  > "cmdTable calls …" 97 times. A list with folds has one "Open all" control

- `REPORT.md`, § Projection and first-screen overview (HEAD lines 950–955):
  Redis's Source details history and the 695/610 px measurement moved.

  > Redis's "Source details · 11" under Persistence opened at the column's foot
  > with three of its five lines below it, and Open all put four there.
  > Inputs reaching this part is
  > collapsed with its count. The reading column stands beside the map's
  > controls and key as well as its canvas and takes their height, with the
  > canvas keeping its own: 695 px of a 1440×900 window instead of 610. Only

- `REPORT.md`, § Reader context (HEAD lines 980–982): the Redis command-row
  click history moved.

  > a modifier-click still opens the code. Nothing is matched by name. On
  > Redis's 95 command rows a click on flushdb or flushdbCommand had opened
  > GitHub.

- `REPORT.md`, § Reader context (HEAD lines 1007–1016): the Show whole map
  and zoom-out histories (Redis's readers) moved.

  > and any input path stay, as a reader zooming out to look around expects;
  > clearing them had sent that reader back to the start. The "−" beside it
  > steps out one level, as a zoom mark steps in one, and, as Show whole map
  > does, moves only the camera: from a part's tiles to the frame holding
  > that part, from an open area to its component, from an open component to
  > the whole map. The camera takes the frame as entering it would, and no
  > closer than the zoom at which the level it leaves closes; at the whole
  > map it zooms out by a fifth. Zooming
  > out by a fifth at every level, it had left Redis's readers where they
  > were, and they pressed Show whole map up to nine times in a question. "+"

- `REPORT.md`, § Reader context (HEAD lines 1022–1029): the toolbar example
  `get · redis-server (executable) / Server runtime / Replication ·
  syncCommand` replaced by `get · kvd (executable) / {area} / {part} ·
  getCommand`; the one-link history moved.

  > ("get · redis-server (executable) / Server runtime / Replication ·
  > syncCommand"). Each segment is its own link that goes up to its level,
  > the reading and the camera together: a component or area is read and
  > framed as entering it frames it, the part is read and entered without the
  > declaration, the declaration is read in its part with its tile centred,
  > the input is entered as its path. With nothing read it names the System
  > map. It had been one link that only read the current reading again: Redis's
  > readers clicked "Server runtime" in it and stayed on syncCommand's tiles.

- `REPORT.md`, § Reader context (HEAD lines 1044–1045): the TODOs link
  history moved.

  > the list the heading heads: the TODOs link landed on its heading at the
  > foot of the page with "10 markers in 6 files" still closed under it.

- `REPORT.md`, § Reader context (HEAD line 1079): the listCreate history
  moved.

  > programs compile it: adlist.c's listCreate had been three results. It shows

- `REPORT.md`, § Reader context (HEAD lines 1106–1109): the chain example
  `processInputBuffer → processCommand → call` replaced by kvd's
  `readQueryFromClient → processInputBuffer → processCommand`; the "No
  explanation saved" history moved.

  > same part, so a chain such as processInputBuffer → processCommand → call is
  > followed one call at a time. A declaration without a line says nothing
  > about one: "No explanation saved" had answered a click on every tile but
  > the keys. Equal names with different source locations remain separate.

- `REPORT.md`, § External communication and data (HEAD line 1115): the
  redis-cli/redis-server examples replaced by kvcli (`net.c`'s `netListen`,
  its part of `loop.c`) and kvd (`netConnect` run by kvcli); the "unused
  helpers" and redis-benchmark start histories moved; the last rule now
  reads "Several call sites of one caller calling one callee are one step
  and the next distinct connection takes the freed place.".

  > Their outgoing calls, listener, registrations and settings are not the component's (READING), and this list is where a reader finds them: redis-cli lists `anet.c`'s `anetTcpServer` and `anetAccept`, and neither listens nor accepts on its map, and it lists "adlist.c · Linked list" with its thirteen functions and "adlist.h · Linked list" with its types, a part it no longer draws.
  >
  > Each declaration another program of the same report runs names those programs, "(run by redis-benchmark)", each linking to its component, in page order: redis-server lists `aeStop` run by redis-benchmark and `anetRead`/`anetWrite` run by redis-cli, where a newcomer had read "unused helpers" and skipped redis-cli's whole network I/O.
  >
  > In the connections' stored order, grouped by the part they reach, redis-benchmark's start read "main calls aeMain" before the `aeCreateEventLoop` main calls thirty lines earlier; three call sites of `main` calling `aeMain` are one step and the next distinct connection takes the freed place.

- `REPORT.md`, § Three layers of truth (HEAD lines 1323–1324): Redis's Tcl
  suite narrative moved.

  > files" (Redis's 204-test Tcl suite is no file any adapter recognizes, and
  > a newcomer told readers to skip it). Routes, client requests and the portals between targets are

- `REPORT.md`, § Three layers of truth (HEAD line 1334): Freqtrade's
  orientation request sizes per rung (measurement) moved.

  > rung (Freqtrade: 1.74 → 1.18 → 0.92 MB). A refusal by size or context, local from a

- `READING.md`, § Selection and captions (HEAD lines 32–33): pykrx's
  decoration count (measurement) moved.

  > lifted for context keep only such relations that carry a pattern (pykrx
  > keeps 24 of its 88 exact decorations there). A registration's holder is the value

- `READING.md`, § Selection and captions (HEAD lines 44–45): etcd's
  recursive logger and clock example moved; the Go, Python and TypeScript
  fixtures cover the rule.

  > without end (etcd's `executeTxn` and `node.Repr` pass their logger and
  > clock to themselves). Go, Python and TypeScript fixtures cover it; Clojure

- `READING.md`, § Architectural responsibilities / A file in several boxes
  (the role split) (HEAD lines 322–331): the pre-adoption helper
  measurements on redis-server and pykrx and pykrx's known miss moved; the
  owner's open question and the concurrency rule kept.

  > target. A helper carries the atlas symbol's `helper` mark. Measured before
  > adoption (steps 0b and 0c): redis-server's main, processCommand, call,
  > rdbSave, syncWithMaster, serverCron and its 94 registered handlers are no
  > helpers, symsTable is one at 0.85–0.91 once its item names its reader,
  > and the library files keep their marks; pykrx's library target has 49–50
  > helpers among 188 asked units, 78.9% of leads 0.40 or more. Known miss:
  > pykrx's `get_market_ohlcv`, whose only user is its file's `__main__`
  > demo, comes out helper (0.17–0.35); a library's public API as entries is
  > the owner's open question, not a rule here. The question and the gate
  > run at once.

- `READING.md`, § Architectural responsibilities / A file in several boxes
  (the role split) (HEAD lines 349–362): the gate measurements over
  redis-1.3.6, pykrx, litestream and repomap, the first criteria's history
  and repomap's design.go measurement moved; the known limit kept without
  its example.

  > whole. Measured over every candidate of redis-1.3.6, pykrx, litestream
  > and repomap (377 files, 2 draws, then 5 draws of the 13 nearest the cut):
  > redis.c 0.97–0.98, pykrx's stock_api.py 0.74–0.79, litestream's main.go
  > 0.69–0.76; redis-cli.c 0.15–0.21, redis-check-dump.c 0.22–0.30,
  > redis-benchmark.c 0.38–0.48. 13 files split in every draw; two pykrx
  > query files (`etx/wrap.py`, `bond/core.py`, 0.49–0.60) and repomap's
  > `llm/api.go` (0.46–0.55) still land either side of the cut. The first criteria, which counted groups "with their own vocabulary
  > of names" as several boxes, had shown no flip on the 11 files first
  > measured, but on the whole set split redis-cli.c, redis-benchmark.c and
  > repomap's render.go, table.go and api.go in every draw (redis-cli.c's
  > `main` then became a box of its own) and flipped redis-check-dump.c.
  > Known limit: a Go file's methods on a type declared in another file
  > follow that type and are not in the item, so repomap's design.go shows
  > only its types and helpers and still goes in several boxes (0.86–0.90).

- `READING.md`, § Architectural responsibilities / A file in several boxes
  (the role split) (HEAD line 388): the registration words `redisCommand
  get` replaced by the C fixture's `kvCommand get`.

  > command table row's `redisCommand get`, a route's `GET /users/:id`),

- `READING.md`, § Architectural responsibilities / A file in several boxes
  (the role split) (HEAD lines 407–411): Redis's
  `listDup`/`dupClientReplyValue` example removed; the C fixture's example
  it stood beside kept.

  > stored. Redis's adlist.c `listDup` calls `copy->dup(...)`, which
  > createClient's `listSetDupMethod` stored `dupClientReplyValue` in; the C
  > fixture's loop.c `loopMain` calls the `beforeSleep` kvd.c's `main`
  > stored, and kvd.c's `processCommand` the `preloadKey` its table row
  > holds: each stays with its own file's boxes (`TestCumulativeCMapOfParts`).

- `READING.md`, § Architectural responsibilities / A file in several boxes
  (the role split) (HEAD lines 480–486): the V0WFR measurement and
  Redis/litestream box examples moved; the rule kept.

  > Measured on the saved V0WFR boxes (3 identical draws, 118 Jev calls in all
  > with the gate, $0.067): redis.c 8–11 of 342 units undecided, 1 decided
  > choice flipped; pykrx 2–4 of 87, litestream 1–2 of 46, none flipped. The
  > map's rule that a helper goes in the box it serves most is inert in the
  > assignment: a helper-only box's `holds` names its helper and Jev puts it
  > there (redis "Logging" = redisLog, litestream's value-parsing and flag
  > boxes); no code rule empties such a box.

- `READING.md`, § Architectural responsibilities / A file in several boxes
  (the role split) (HEAD lines 488–500): the registration and task-sentence
  measurements on the owner-proxy's redis.c and pykrx moved; the rule "A
  unit the assignment decides is not asked again." kept.

  > The registrations and the sentence on the box that runs every command were
  > measured before adoption (2026-09-27) on the saved assignment requests of
  > the owner-proxy's redis.c (339 units, 20 boxes) and pykrx's 7 split files
  > (187 units), 3 draws each. Before, 12 redis.c units landed differently
  > between draws, setCommand among them (String commands 0.36 against Set
  > commands 0.34), and getCommand and appendCommand went in Command dispatch
  > (getCommand at 0.94–0.96). After, no unit landed differently; getCommand,
  > setCommand, appendCommand and echoCommand went in String commands and
  > pingCommand in Server administration commands in every draw. The task's
  > sentence alone or the registrations alone left getCommand in Command
  > dispatch. A unit the assignment decides is not asked again:
  > getGenericCommand, called only by string commands, stays in Command
  > dispatch (0.66–0.67 against String commands at 0.16–0.17).

- `READING.md`, § Architectural responsibilities / A file in several boxes
  (the role split) (HEAD lines 502–518): the pingCommand measurements moved;
  the conclusion is kept as a rule: "The task has no wording that keeps a
  command whose work is the connection with the box that runs commands: that
  command's box is the naming's to give.".

  > Where pingCommand goes depends on the boxes the naming gives redis.c
  > (measured 2026-09-27, 3 draws per task on each saved naming). With a box for
  > server administration commands (the owner-proxy's naming) it goes there.
  > Three fresh namings had no box for connection or server commands, and on
  > each this task puts pingCommand in String commands. On one of them, whose
  > Client connection handling box also holds command dispatch, the task
  > without the sentence put it in Client connection handling in 2 of 3 draws
  > (0.42–0.46, the third a near-tie); the sentence moves it to String commands
  > (0.46–0.58, 6 of 6 draws). On another, the task without the sentence put it
  > in String commands too (0.45–0.56). A wording that keeps a command whose work
  > is the connection with the box that runs commands put pingCommand back in
  > Client connection handling there, but left it undecided on the owner-proxy's
  > naming, so it was not adopted: PING's box is the naming's to give. On the
  > naming whose two boxes both claim command dispatch, processCommand is a
  > near-tie between them in 5 of 6 draws (the task without the sentence chose
  > Client connection handling at 0.69–0.76). On litestream's two split files
  > (54 units, no registration in them), no decided unit landed differently.

- `READING.md`, § Architectural responsibilities / One rule for every file
  (HEAD lines 576–578): the Redis `main` example removed.

  > program's entry. A seed no part holds (Redis's `main`, a near-tie of the
  > parts answer) makes no entry part; GroupsIndex keeps it with its off-map
  > reason (`Entries`), and the code never picks a part for it.

- `READING.md`, § Architectural responsibilities / Membership and the
  off-map record (HEAD lines 616–618): redis-cli's adlist.c example replaced
  by kvcli's `loop.c`.

  > map by file with its name (reason `unreachable`, REPORT). redis-cli links
  > `adlist.c` and never calls one of its thirteen functions, so its "Linked
  > list" left its map. A part holding one declaration the program may run

- `READING.md`, § Architectural responsibilities / Descriptions (HEAD lines
  637–639): the history of the first twelve names moved.

  > the part holds, in ID order, never cut to a count (the first twelve were
  > sent until 2026-09-26, while the keys prompt called the list everything the
  > part holds).

- `READING.md`, § Operation ownership (HEAD line 676): the words
  `redisCommand` replaced by the C fixture's `kvCommand`.

  > (`GET` and `/users/:id` of `e.GET("/users/:id", h)`; `redisCommand` and `get`

- `READING.md`, § Operation ownership (HEAD lines 696–700): Redis's
  dispatcher sentence and the declaration-order history moved; the table
  order example is kvd.c's `get`, `set`, `del`.

  > neutral: it favours names that begin early, and Redis's dispatcher read
  > "calls String commands: appendCommand, decrCommand, decrbyCommand", hiding
  > get and set behind `append`. Declaration order alone still hid `get` when the
  > part held `ping` and `echo`, which redis.c defines first; the table's rows are
  > the author's own order of the commands (`get`, `set`, `setnx`). Source order

- `READING.md`, § Operation ownership (HEAD lines 710–712): Redis's
  `findFuncName reads symsTable` example replaced by the C fixture's
  `printSymbols reads symsTable`.

  > of them (`native_reads`): Redis's Introspection and debugging part reads
  > Debug symbols through `findFuncName reads symsTable`, and each command part
  > reads the parts holding `server` and `shared`. A call left unresolved because its field or name was

- `READING.md`, § Operation ownership (HEAD lines 720–723): Redis's Event
  loop fallback sentence example removed.

  > calls, stores and declarations ("Event loop calls Client connection
  > handling: readQueryFromClient, acceptHandler, sendReplyToClient.": the
  > `rfileProc` call comes before the `wfileProc` one, and redis.c stores
  > `readQueryFromClient` before `acceptHandler`),

- `READING.md`, § Boundaries (HEAD lines 769–772): Redis's anet.c example
  replaced by the C fixture's `net.c` (kvd listens with `netListen`; kvcli
  links it and never reaches it).

  > shared `anet.c` gives redis-server the listener `anetTcpServer` and
  > `anetAccept`; redis-cli and redis-benchmark, which link it and never reach
  > them, list them under their component's "Not reachable from the
  > entrypoints" (REPORT), where a reader finds what is not shown. Facts do

- `READING.md`, § External symbols: the `atlas_api` table (HEAD line 789):
  `redis.c.redisCommand.proc` replaced by the C fixture's
  `kvd.h.kvCommand.proc`.

  > field (`redis.c.redisCommand.proc`); its usage is its first row.

- `READING.md`, § External symbols: the `atlas_api` table (HEAD lines
  793–794): the fopen("/dev/null") history moved.

  > nothing names the declaration making the call, and `fopen("/dev/null")` was
  > once asked what a callable becomes and answered a request handler. The

- `READING.md`, § External symbols: the `atlas_api` table (HEAD lines
  840–868): the atlas_api measurements on redis 1.3.6, xk6-dns and
  microblog, Jev's known misses, the Flask observation and the 2026-09-25
  probe moved; the SQLAlchemy sentence kept.

  > Measured 2026-09-27 on the saved requests of redis 1.3.6 (129 symbols),
  > xk6-dns (47) and microblog (117), 3 draws each, as wrong answers / symbols
  > whose answer changed between draws. The v5 text-model table, whose cells
  > were optional notes with no answer for "talks to nothing" and none that
  > fitted `accept`: 7/1, 18/12 and 9/2 (inet_aton `talks sdk` 3 of 3, accept
  > `client_request` 3 of 3; five saved v5 Redis runs had given inet_aton sdk
  > in 3, bind `publishes` in 2 and accept `client_request` in 2). The same
  > options and criteria written into the text model's prompt: 2/1 (`select`
  > serves in 2 of 3; one earlier wording, 3 of 3), 0/0 and 12/0. Jev with
  > them: 0/0, 3/2 and 6/2, and 3 and 3 answers explicitly unanswered.
  > inet_aton, inet_ntoa, accept, listen, bind, connect, gethostbyname, fopen,
  > open, sigaction, pthread_create, k6's `modules.Register`, Flask's `route`,
  > `requests.post`, miekg's `ExchangeContext`, k6's `DialContext` and
  > `LookupHost` were right in every Jev draw of four rounds of wording. What
  > Jev still misses is named by chained Python names (`requests.post.json` as
  > `client_request`, `alembic.op.f` as `db`), k6's `metrics.PushIfNotDone`,
  > which sends a sample on the host's channel (never `none` in 12 Jev draws
  > over four wordings: `sdk` at about 0.4 against `none` at about 0.2 in 8,
  > unanswered in 4; the text table had said `queue_producer` in 2 of 3), so an
  > xk6-dns map can show a "k6 metrics" outside system, and the construction of
  > a `net.Resolver`, whose usage line is a field line of its literal
  > (`client_request` or unanswered). Two task wordings that weigh `declared`
  > first, one adding that a value handed on through a channel stays in the
  > process, left PushIfNotDone `sdk` 3 of 3 or unanswered 3 of 3 and made more
  > microblog answers change between draws (3 draws each, 2026-09-27). Flask's `errorhandler` is now `none` or unanswered
  > where the text model said `request`: `request` and `none` both stay near
  > 0.4. Building a SQLAlchemy `select` is `db`, the program's question to its
  > database. The 2026-09-25 probe that kept this table on the text model asked
  > Jev optional yes-only columns without criteria.

- `READING.md`, § External symbols: the `atlas_api` table (HEAD lines
  921–922): Redis's spawnIOThread example removed.

  > (`HandsOver`/`HandedOverBy`): Redis's `spawnIOThread`, reached by twelve
  > inputs, registers `IOThreadEntryPoint`. A table read registers nothing.

- `READING.md`, § External symbols: the `atlas_api` table (HEAD lines
  951–952): the 82-of-82 alias history moved.

  > `line` alone and the model writes no `alias` (it wrote one on 82 of 82 rows,
  > all discarded, while the prompt demanded two cells).

- `READING.md`, § External symbols: the `atlas_api` table (HEAD line 954):
  Freqtrade's refused-row count and Redis's request count moved.

  > A row whose only address option is `unknown` accepts any address answer as `unknown`: nothing else can be chosen there, and the model tends to copy the observed path into that cell (27 Freqtrade rows were refused for it).
  >
  > A fixed incoming entry additionally requests its `name` among its `words` (Operation ownership), with or without `--captions`, and shows no `method` field; its rows share windows without an owner, since its handler's calls near a registration elsewhere say nothing about its name (one window per handler cost Redis 98 requests for 97 names).

- `READING.md`, § Questions and Learn (HEAD lines 1218–1221): the saved
  Freqtrade window measurements moved; the rule and its reason kept.

  > the saved Freqtrade window that asked 64 questions over 460 code rows (4,661
  > anchors) got four back, the same rows with eight questions got eight, and the
  > same 64 questions over document rows got 64 — density of decisions per response,
  > not key names, loses answers. Windows over the same rows share a request prefix;

- `READING.md`, § Questions and Learn (HEAD lines 1244–1245): the
  canvas.spec.mjs size measurement moved.

  > evidence. The `canvas.spec.mjs` selection row, refused for its size on every
  > run, falls from 550 entries and 70,906 B of calls to 245 entries and 31,046 B.

- `READING.md`, § Orientation (HEAD line 1392): the bare-member-ID history,
  Redis's main flow and its `main` example moved.

  > A group member (`n4` of `t1`) is the declaration place whose object is qualified by its program (`t1.n4`, atlas `ScopedObjectID`); looked up by the bare member ID, no member found its place and every request went without member evidence, and Redis's main flow put `loadServerConfig` before the `initServerConfig` main calls first.
  >
  > A member's calls are listed in the order they are written in it, so a bounded list keeps the first calls written, not the first of the graph's order (redis-server `main`'s `fprintf`, `exit` and `time`).

## 2026-09-28 — An input's reading links to its program's Main flow (u7); the UI fixes u1–u7 walked

- The blind GET check found the Main flow ("A client command from socket
  read to reply") only by chance, from the component's card. An input's
  reading now links to the component's flow section after its dispatch
  boxes: "Main flow" and the flow's title as the model wrote it, kept as
  model text (its `model` class and display ref); a component with no flow
  section gives no link (`rmInputFlow`). On the saved Redis run the title
  is "Server startup and command dispatch", main → … → processCommand →
  call; the link lands on `#t1-flow`.
- **Test.** `TestAnInputsReadingLinksToItsProgramsMainFlow` fails on revert
  (`ui-fixes/revert.log`).
- **Walk** (`repomap render` of `20260928-050038-redis-1-3-6-d8547ea73628`,
  headless Chromium 1440×900 over a loopback server; `ui-fixes/`
  before-*/after-* screenshots of GET's reading and path, the component
  card and the Entrypoints landing). The blind question "what runs for GET
  key", re-asked of the rendered text only (`ui-fixes/blind-recheck.cjs`):
  before, GET's path showed 167 words on arrival with getCommand,
  getGenericCommand, lookupKeyReadOrReply, addReply and addReplyBulk among
  rdbLoadObject, createListObject, convertToRealHash, vmReadObjectFromSwap
  and "other calls into it", no Main flow link; after, 56 words on arrival
  (dispatch, "handled by getCommand", getCommand → getGenericCommand,
  "Reaches 11 more parts deeper", the Main flow link), none of the VM or
  RDB names; one click opens the rest (205 words, all five answer names);
  the link reaches the flow with processCommand and call. The old "How get
  reaches call" key is in no page. Left: the handler calls only
  getGenericCommand, so GET's own work (depth 2) is behind the fold, and
  no input's reading names a route from the network to call (u6, with the
  inputs work).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS (127). `make ui-visual-test`:
  PASS (65 passed, 5 skipped).

## 2026-09-28 — An input's path opens at its handler's own calls; deeper parts fold (u5)

- The blind GET check found GET's Path listing every part GET's code can
  reach, thirteen parts down to VM swap-in's `rdbLoadObject →
  zslInsert`, an empty "String commands" heading, and "getGenericCommand
  → shared · read / 96 other calls into it on this path", read as calls
  getGenericCommand makes.
- **Change.** The part holding the handler (the one part the reach enters
  at depth 0) carries the handler in the page data (`pageInputPart.Handler`)
  and reads "String commands / handled by getCommand". The parts the
  handler calls directly (depth 1) stand open; every part reached deeper
  is folded under one line, "Reaches 11 more parts deeper", which opens
  them as they were, every call and count kept. The fold is by depth
  alone, the handler's own calls against the rest: no route is chosen and
  nothing is dropped. The count of the other calls into a part reads
  "{n} more calls into this part come from other code on this path", in
  the path and in a part's "Why it appears in" alike (Russian likewise).
  A lead decision awaiting the owner (CURRENT).
- **What GET shows now** (Redis run `20260928-050038`): Dispatched from
  call · one of 94 handlers; String commands, handled by getCommand;
  Command dispatch, getCommand → getGenericCommand; "Reaches 11 more parts
  deeper". GET's own work (getGenericCommand → lookupKeyReadOrReply,
  addReply, addReplyBulk, shared) is at depth 2, one click away: the
  handler calls only getGenericCommand.
- **Tests.** `TestAnInputsPathFoldsThePartsPastItsHandlersOwnCallsByDepthAlone`
  (a depth-1 part of seven calls stays open while a depth-2 part of one
  call folds; a depth-1 part listed late still stands before the fold; no
  deeper part, no fold; folded parts keep every call and count), the
  handler and fold assertions in `TestAnInputsPathNamesItsDispatchThenItsOwnSteps`
  and the handler part in `TestAnInputsPathNamesItsDispatchWithoutARoute…`;
  each fails on revert (`ui-fixes/revert.log`).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS (127). `make ui-visual-test`:
  PASS (65 passed, 5 skipped).

## 2026-09-28 — The Entrypoints link lands on the program's entry (u4)

- The blind GET check clicked "Entrypoints 1" under "Code, entrypoints and
  sources" and landed on "Inputs · 96 · External communication · 2" with
  no main in sight: the link went to the header line `#t1-entrypoints`,
  which the scroll left under the toolbar.
- **Change.** The page data (`entryLanding`, from GroupsIndex's entries)
  names the part holding every seed of the program and, for one seed, its
  source key; the link carries them (`data-entry-part`,
  `data-entry-source`). A click reads that part on the map with the seed
  read in it; with the seed in no part (redis-server's main) or seeds in
  two parts, it reads the component, its reading opened at the entry line
  ("The program's entry is not on the map: main redis.c:9124 · In no part
  of its file"). Only a map holding neither leaves the link to its page.
- **Walk** (headless Chromium 1440×900, loopback): Redis, the link from the
  Main flow page lands on `#system-component-t1` with the entry line at
  the column's top, in view; litestream, it lands on `#n-t1-g11`,
  "Command entry point", with `main` read (cmd/litestream/main.go:79). No
  page errors.
- **Tests.** `TestTheEntrypointsLinkLandsOnTheProgramsEntry` (one seed on
  the map, two in one part, two in two parts, one off the map) and
  `TestTheEntrypointsLinkLandsOnThePartOrTheComponentsEntryLine`; each
  fails on revert (`ui-fixes/revert.log`).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS (127). `make ui-visual-test`:
  PASS (65 passed, 5 skipped).

## 2026-09-28 — The entry line names its file (u3)

- The blind GET check read redis-server's reading as "The program's entry
  is not on the map: main :9124 · In no part of its file" and found no
  file in it. The component's heading and reading now write the entry by
  its name and the file:line anchor the page's other source links use:
  "main redis.c:9124 · In no part of its file" (`target.html`); the "Not
  on the map" list keeps its chip rows.
- `TestALaunchPointOffTheMapIsNamedWithItsReason` checks the line's
  `<code>main</code> <span class="anchor">redis.c:9124</span>` and no
  `:9124` chip; it fails on revert (`ui-fixes/revert.log`).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS (127). `make ui-visual-test`:
  PASS (65 passed, 5 skipped).

## 2026-09-28 — Every count on an input's and a site's reading says what it counts (u1, u2)

- Evidence: the blind GET check (`blind-get/answers.md`) met "one of 94",
  "95 inputs dispatched here", "Inputs 96", "← Inputs 94" and "Incoming
  request records · 95" with nothing telling them apart. On the Redis run
  `20260928-050038-redis-1-3-6-d8547ea73628` they count: 94, the
  alternatives of call's `c->cmd->proc`, every one a command handler; 95,
  the inputs dispatched there (sinterCommand handles sinter and smembers);
  96, redis-server's inputs (95 request records and IOThreadEntryPoint's
  background work); "← Inputs 94", the handlers the inputs' arrow enters
  (select's handler, selectCommand, is in no part, and sinter and smembers
  share one row). No number changes.
- **u1.** GET's reading says "How a request for get gets to call is not
  established." (030dbc9e); the old key is in no template, vocabulary or
  rendered report, and every input's box is written by the one
  `rmInputPathSection`. Its test fails when the line is put back to the
  old key (`ui-fixes/revert.log`).
- **Change.** The page data of a dispatch site (`pageDispatched`,
  `pageSiteReading`) carries `handlers`, the alternatives that are an
  input's handler, and `shared`, each handler several of its inputs share
  with those inputs (`siteHandlers`, from GroupsIndex's sites and
  operations). GET's box reads "Dispatched from call · one of 94 handlers"
  and "call → one of 94 handlers", its hover "95 inputs are dispatched
  here / sinterCommand handles 2 of these inputs: sinter, smembers"; call's
  own reading "Dispatch site · one of 94 handlers · 95 inputs dispatched
  here" and "sinterCommand handles 2 of these inputs: sinter, smembers". A
  site some of whose alternatives handle no input reads "one of N
  functions, M of them handlers". The component's Connections line from
  its inputs reads "← Inputs 94 handlers", the arrow's card "94 handlers,
  into 12 of 21", and inputs sharing a handler are one row naming each
  ("sinter, smembers → sinterCommand"): the row key had dropped smembers.
  "Incoming request records · 95" and "Inputs 96" already said records and
  inputs.
- **Tests.** `TestADispatchSiteCountsItsHandlersApartFromItsInputs`, the
  handler assertions in `TestAnInputsPathNamesItsDispatchWithoutARoute…`,
  `TestAnInputsPathNamesItsDispatchThenItsOwnSteps`,
  `TestADispatchSitesReadingListsTheInputsReachingIt` and
  `call-card.test.mjs`'s shared-handler row; each fails on revert
  (`ui-fixes/revert.log`).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS (127). `make ui-visual-test`:
  PASS (65 passed, 5 skipped).

## 2026-09-28 — Follow-ups f1–f3 and the exec wording accepted on Redis and litestream

- Runs (`.bin/repomap <repo> --no-serve --no-open`, default system cache,
  no `--debug-dir`, no `cache clear`), binary `8d8e0b4d…` at 030dbc9e;
  receipts `followups/` (before/, after/, walk/, etcd-probe/, revert.log).
  "Before" is step 4's acceptance at 08f6a3ce on the same cache.
- **Redis**: exit 0 in 27 s with 23 live calls (3 assignment windows
  re-asked because dupClientReplyValue joined redis.c's second-pass window,
  then 9 descriptions, 6 areas, keys, core, zones, glossary and
  orientation), 138 cached; warm rerun exit 0 in 4 s, 0 live, 161 cached.
  The helper question stayed cached (its items did not change).
  redis-server off the map, before → after:
  - 13 helpers never asked because their users were open when the second
    pass ran → 7 asked in round 2 (freeClientMultiState,
    initClientMultiState, setDictType, vmGenericLoadObject,
    vmMarkPagesFree, zslCreateNode, zslFreeNode; 7 placed), 5 placed by
    rule A after it (vmFreePage, vmMarkPageFree, vmReadObjectFromSwap,
    zsetDictType, zslCreate), 1 `blocked` (resetServerSaveParams: main and
    initServerConfig call it, and main is undecided);
  - helpers asked without a decision 4 → 3 (createHashObject and
    redisFunctionSym as before; checkType, placed before, is a near-tie in
    the re-asked window; createZsetObject and oom are placed);
  - units the assignment left open: main, selectCommand, unchanged.
  - dupClientReplyValue: rule A into adlist.c's "Core data structures"
    → asked in round 1 (no placement user) → redis.c's "Server core
    state". createClient, which hands it over, is in "Client connection
    handling": the box is the model's choice, not a code rule.
  - Second pass: round 1 asked 63, placed 60; round 2 asked 7, placed 7.
- **litestream**: exit 0 in 7 s, 0 live, 195 cached. Nothing moved: none
  of its Go targets has a package-level variable whose initializer calls
  repository code; cmd/litestream keeps IsSQLiteDatabase and txidVar
  undecided, one round of 14 asked, 12 placed, nothing blocked. The four
  functions sanity check 3 named (addNewField, GetCluster, filterNoPut,
  filterNoDelete) are etcd's (step 0b's etcd server run), not
  litestream's; the etcd probe (f1b) gives addNewField its user, and the
  other three stay function values with no relation.
- **Walk** (headless Chromium, loopback, after Redis run): exec's path reads
  "How a request for exec gets to call is not established." and "exec's
  handler itself calls call, where 95 inputs are dispatched:" /
  `execCommand → call`; get's reads "How a request for get gets to call is
  not established."; "Used by code in no part" is listed among the
  off-map rows; no page errors. Neither analysed repository changed.
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — exec's reading: the two lines say what each means

- Step 4's open item: exec's reading showed "How exec reaches call is not
  established." beside "Reaches call … execCommand → call", which
  contradicted each other on their face. The first is about how a request
  for the input arrives at the dispatch site from outside; the second is
  the handler's own call back into the site.
- They now read "How a request for exec gets to call is not established."
  and "exec's handler itself calls call, where 95 inputs are dispatched:"
  followed by `execCommand → call`; Russian "Как запрос exec попадает в
  call, не установлено." and "Обработчик exec сам вызывает call, где
  выбираются входы (95):" (`ui_vocabulary.go`).
- `TestAnInputsPathNamesItsDispatchThenItsOwnSteps` checks get's line and
  exec's two lines; either old string fails it (revert log). REPORT
  describes both.
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Follow-up f3: blocked helpers are asked once their users have boxes

- Sanity check 3 (queue f3): the second pass ran once, and a helper with a
  user still open was not asked, so a helper whose user only the second
  pass placed stayed undecided only because of that order (13 of redis.c's
  19 undecided units at 08f6a3ce were such helpers, never asked).
- The second pass now repeats: after each round code settles, rule A places
  a waiting helper whose users now share one row, and a helper whose users
  now stand in two or more rows is asked in a further round. A unit is
  still asked the assignment at most once, so the rounds end. The k-th
  round writes its own windows (`len(targets)·k + round`); `tables.md`
  counts each round's asked and placed.
- A helper never asked because a unit that uses it never got a row is now
  off the map under its own closed reason, `blocked` (`role_blocked`),
  apart from `undecided` (a question asked without a decision). The atlas,
  GroupsIndex and the report accept it; the report reads "Used by code in
  no part" ("Используется кодом, не попавшим ни в одну часть").
- Tests: `TestABlockedHelperIsAskedOnceItsUsersHaveBoxes` (inner waits on
  outer, which round 1 places, and is asked in round 2 in windows `r5`;
  leaf waits on a near-tie and is blocked); the split check and
  `projectSplit` accept `blocked` entries (the Go fixture's
  command_table.go has three). Revert checks: one round only, or blocked
  recorded as undecided, fail the test.
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Follow-up f2: a call through a stored function value is not a user for placement

- Sanity check 3 (queue f2): Redis's adlist.c `listDup` calls
  `copy->dup(node->value)`, resolved exact through its one recorded store
  (createClient's `listSetDupMethod(c->reply, dupClientReplyValue)`), so
  rule A counted listDup as `dupClientReplyValue`'s user and moved a
  redis.c callback into adlist.c's "Core data structures" row.
- A hand-over was already no user; the call through the value it stored is
  the other half of the same hand-over. `unitFacts` now leaves a call with
  ProgramIndex `function_value` dispatch out of `users`/`uses`, which rules
  A, B and C read; it still sets `used` and stays in the helper item's
  `called_by`, so the no-users rule and the helper question's requests are
  unchanged. The split check (`partstest`) computes users the same way.
- The C fixture has the same shape twice: kvd.c's `main` hands
  `beforeSleep` to `loopSetBeforeSleep` and loop.c's `loopMain` calls
  through that field; kvd.c's `processCommand` calls the `preloadKey` its
  table row stores. `TestCumulativeCMapOfParts` checks that both helpers
  are asked once more among kvd.c's boxes instead of following their
  caller (beforeSleep had gone to loop.c's part), and that beforeSleep's
  item still lists `loop.c:loopMain`. Revert check: counting the call as a
  user again fails it.
- gofmt of two files the f1 commits left unaligned (`roles_test.go`,
  `direct_call_index.go`).
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Follow-up f1b: a Go package-level variable is the caller of its initializer's calls

- Sanity check 3 (queue f1, Go): a call written in a package-level `var`
  initializer left no relation, because Go evaluates it in the synthetic
  package initializer, which the direct-call index excludes. etcd's
  `addNewField`, called only in `var schemaChanges = …`, looked unused.
- The direct-call index (version 15) records each exact repository call the
  synthetic initializer makes inside a package-level variable's value as an
  edge whose caller is that variable (`DirectCallVariable`: name, type,
  exported state, whole specification, code lines), at the call's own
  position. The Go adapter projects the variable as a `variable` of its
  package and the call as an ordinary exact `calls` relation; the places
  graph lifts a package-level variable that owns a call as a declaration,
  as it lifts a module body. Calls the synthetic initializer makes outside
  a variable's value (init functions, imported initializers) stay out;
  outside-package calls there stay the package's `synthetic_caller`
  frontier.
- Fixture: `internal/storefixture/command_table.go`'s `defaultCommands =
  namedCommands("get", "set")` (line 164). `TestCumulativeGoMapOfParts`
  checks the call's owner, that `namedCommands` is asked with
  `called_by: defaultCommands` and goes with it as a helper, and that
  `defaultCommands`, a Go variable, is asked.
- Native equivalents, each now checked at its call's owner: Python
  `local_router = LocalRouter()` (module body, `cli.py:44`), TypeScript
  `const root = express()` (module, `route-mounts.ts:3`), Clojure
  `(def default-greeting (service/greet "default"))` (the var, new at the
  end of `core.clj`; `TestNativeCumulativeProject` lists its load-time
  call). C has none: a file-scope initializer holds constant expressions
  only.
- etcd server library, no-model probe (`followups/etcd-probe`):
  `addNewField` ← `calls exact` from the variable `schemaChanges`
  (`schema.go:132`); two package variables own calls (`schemaChanges`,
  `flagsline`). `GetCluster` (`getCluster = srv.GetCluster`) and
  `filterNoPut`/`filterNoDelete` (`append(filters, filterNoPut)` inside
  `FiltersFromRequest`) are function values, not calls, and still have no
  relation: GO's recorded gap, unchanged.
- Revert checks: no initializer call recorded, or places not lifting the
  variable, fail `TestCumulativeGoMapOfParts`.
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Follow-up f1a: the no-users rule asks a kind whose uses are not recorded

- Sanity check 3 (queue f1): "nothing uses it" turned a fact gap into
  "none" for a kind whose uses the adapter never records. `askedHelper`
  took any function or variable with no recorded user out of the helper
  question, so Clojure's `ensure!` (a macro: CLOJURE, its uses leave no
  relation) was "no helper by code".
- `recordedUses` (`helpers.go`) now states, by target language and
  declaration kind, which uses each adapter records, from the contracts: C
  functions (calls, hand-overs) and variables (reads); Go functions and
  methods (calls, hand-overs); Python/TypeScript/JavaScript functions,
  methods, lambdas and variables; Clojure functions and variables. A unit
  of any other kind is asked: types, module bodies, Go variables (no reads)
  and Clojure macros. No name heuristics: a macro is a ProgramIndex fact,
  `macro` (clj-kondo's own mark on the definition), carried to the places
  graph's `Decl.Macro`.
- Effect today: only Clojure macros change (the fixture's `ensure!` and
  `fresh-list` are asked). C, Go, Python and JS/TS units are asked exactly
  as before; Go has no variable units yet.
- Tests: `TestAKindWithNoRecordedUsesIsAsked` (a Go variable and a macro
  asked, a C variable nothing reads not), `TestTheHelperItemCarriesItsUsers`
  runs as C, `TestCumulativeClojureMapOfParts` checks both macro marks and
  that `ensure!` is asked. Revert checks (`followups/revert.log`): the old
  kind switch fails the unit and the Clojure fixture test; an adapter that
  marks no macro fails the fixture test.
- Docs: READING (helper question), CLOJURE (calls written through macros),
  PROGRAM_INDEX (`macro`).
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Map model step 4 accepted: one reach in GroupsIndex, measured on Redis and litestream

- Scope: S4-7 of `map-model/step4-plan.md`: the acceptance of S4-1–S4-6 at
  08f6a3ce (binary `eabdce27…`), with the skeptic's additions and the lead's
  corrections (the reached-from list only in the site's reading; an entry
  off the map named in the reading, no canvas label). Receipts:
  `step4/impl/` (base, accept, walk-redis, shots, measure-*.txt,
  impl-revert.log).
- **Runs** (`.bin/repomap <repo> --no-serve --no-open`, default system
  cache, no `--debug-dir`, no `cache clear`): Redis exit 0 in 6 s with 0
  live calls (the S4-4 run had asked orientation and the glossary once each
  after the lanes changed), warm 5 s, 0 live; litestream exit 0 in 24 s
  with orientation and glossary asked once live, warm 7 s, 0 live. Both
  warm reruns' report.json are identical but for `timing`, and `repomap
  render` of all four runs is byte-identical to report.html.
- **Step 0 baseline** (75da4f82, same cache): `repomap render` was
  byte-identical too (the joint-phase gap is invisible on these runs, as the
  skeptic said; only `TestRenderDerivesWhatTheRunDerived` shows it).
  `Derive`: Redis 13 ms; metabase's 497 MB program index with 3,000
  synthetic high-fan-out inputs 1.35 s, 272 MB allocated, 71 MB retained.
- **Redis measurements** (before → after): report.html 12,555,599 →
  12,268,850 bytes; `data-call-paths` 992,292 → 0; `data-input-path`
  472,245 → 831,348 (median 5,099 → 8,547, max 20,498); `data-dispatch`
  4,084 (two parts); groups-index.json 1,524,786 → 1,520,300. Entry parts:
  redis-server 10 → 0 (main undecided off the map; its reading names it),
  the three small programs 1 each; no area is the entry. Entry summaries
  restating the registration 96 → 0. call is reached from exec, lpush,
  rpoplpush, rpush and slaveof; loadAppendOnlyFile from debug (Go and the
  independent Python recomputation agree). Dark part-pair arrows: GET 12 →
  15 (13 parts), SET 6 → 6, exec 17 → 11 (C1), debug 17 → 13; all inputs
  955 → 1,026. Served-equivalence: 0 pairs missing. Canvas arrows at rest:
  redis-server 19 = 19, redis-cli 4 = 4, redis-benchmark 5 = 5,
  redis-check-dump 1 = 1.
- **litestream** (cmd/litestream, Go): entry parts 5 → 1 (Command entry
  point; its area stays the entry), cmd/litestream-test 6 → 1; 33 entry and
  16 outgoing summaries that restated their facts → 0 (the SQL text stays in
  the Data section); no dispatch site. Arrows at rest 16 = 16 and 2 → 3.
  report.html 10,029,196 → 10,709,603: the live glossary answer added five
  terms (+796,536 bytes of glossary), `data-call-paths` −160,964,
  `data-input-path` +37,649.
- **No-script map**, before and after: the page's one figure, the system
  canvas, shows its 151 (Redis) and 204 (litestream) nodes and none of its
  239 and 198 arrows without scripting, and carries no static arrow; the
  static zone picture's changed quiet flags (B3) reach no rendered figure.
- **Orientation's main flow**: Redis keeps its seven steps (acceptHandler …
  sendReplyToClient) in new words; litestream adds the Windows service
  step. python-tutorial-game (dogfood, 75da4f82 against HEAD, both on the
  default cache): the re-asked flow now runs from the level menu to
  run_level and no longer ends at SimulationField's animation, a different
  model answer after the lanes changed; SimulationField.animate stays an
  input with its reading, and post /api/level/run's reading lists the parts
  it enters with their calls and 12 state changes with their callers.
- **GET before → after** (`get-before.txt`, `get-after.txt`): "Shared by
  95 inputs, through call" / "The path by which an input reaches call is
  not established." / an 18-step witness tree → "Dispatched from call · one
  of 94" / "How get reaches call is not established." / "Dispatched from
  loadAppendOnlyFile · one of 94" / String commands; Command dispatch
  getCommand → getGenericCommand; Client connection handling
  getGenericCommand → addReply, → addReplyBulk, 1 other; Generic key
  commands getGenericCommand → lookupKeyReadOrReply; Server core state
  getGenericCommand → shared · read, 96 others; Core data structures five
  calls, +31, 47 others; … down to Sorted set commands at depth 9.
- **Walk** (headless Chromium, 1440×900, loopback server): Find → get opens
  its reading at the open "Dispatched from call" box with no route and no
  reached-from inputs; call's own reading lists slaveof, exec, lpush,
  rpoplpush, rpush with their calls; a plain click on getCommand reads it
  (0 new tabs), a modifier-click opens its code (1); "Why it appears in
  get" on Command dispatch lists getCommand → getGenericCommand; a part off
  the path says "Outside this input path"; a reload with GET pinned keeps
  the same reading; IOThreadEntryPoint reads "Registered by bgrewriteaof,
  slaveof, sync, bgsave, debug, flushall, save, shutdown, lpush, rpoplpush,
  rpush"; redis-server's reading says "The program's entry is not on the
  map: main:9124 · In no part of its file"; no page errors.
- **Open for the owner**: an input that re-enters its own dispatch site
  (exec calls call) reads both "How exec reaches call is not established."
  and "Reaches call … execCommand → call"; the label "init" covers the
  launch and the main loop (aeMain, serverCron), not only startup; a seed
  left undecided (redis-server's main) leaves the program with no entry part
  on the canvas; a library still shows no entry part (an `export` seed kind
  is the open question).
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Map model step 4, S4-6: chains and operation types deleted

- After S4-1 and S4-5 nothing read `Index.Chains` (only `applyPhases` and
  `drawsInit` had) or `Operation.RequestTypeIDs`/`ResponseTypeIDs` (no
  reader outside GroupsIndex). `chains.go` (`projectChains`, `Chain`,
  `validateChains`, `carriedTypes`, `operationTypes`) is deleted, with the
  fields and their calls in `projectTarget` and `Hydrate`;
  `appendUniqueString` moves to `outbound.go`. Lead decision: YES (the
  constitution's "remove more than you add").
- The Echo preset's chain and type assertions become reach assertions: GET
  /users/:id reaches the handler, the service, the repository's GetByID and
  the sqlc GetUser sending the statement that names the users table; the
  decoded index derives the same reach. It fails when the walk stops short
  (s46a).
- Render-neutral: the S4-4 Redis run renders byte-identical.
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Map model step 4, S4-5: GroupsIndex says which arrows are quiet

- `Connection.Quiet` (derived, never persisted) = initialization or a call
  into a helper, in a program with a handled input (C3's `serves`), with
  C6's exception defined once, over the index's own connections: when every
  one would be quiet, the calls into helpers are drawn and wiring stays
  quiet. C6 had it twice with two scopes (`addMapStructure` over every
  page connection touching the target, `mapEdges` over the target's pairs).
- The report draws `Init: connection.Quiet` on the canvas, and a static
  pair is quiet only when every connection it draws is; `drawsInit`,
  `allQuiet` and the helper-only bookkeeping are deleted. Chains are no
  longer read anywhere but `validateChains` (S4-6).
- Render-neutral on Redis: the S4-4 run rendered with this code is
  byte-identical to its report.html.
- Tests: `TestProgramThatServesNothingDrawsTheArrowsItsMainReaches` deleted
  (GroupsIndex's `TestAProgramThatServesNothingHasNoInitialization`, S4-1,
  holds it); C6's `TestACallIntoAHelperStandsQuiet` becomes GroupsIndex's
  `TestAConnectionIntoAHelperIsQuietUnlessEveryArrowWouldBe` (serving,
  serving nothing, every arrow into helpers, wiring beside helpers) and the
  report's `TestTheMapQuietsWhatGroupsIndexMarksQuiet` (the canvas and the
  static pairs follow `Quiet`); the render/run and flowtest hydrate
  comparisons include `Quiet`. Each fails on revert (s45a–e).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS.
  `make ui-visual-test`: PASS.

## 2026-09-28 — Map model step 4, S4-4: an input's path is its saved reach

- The report walks no code: `buildOperationMap`'s BFS, `served`,
  `operationCallPaths`, `callWitness`, `pageMapNode.CallPaths` (with
  `data-call-paths` and its remaps) and `page_reachability.go` are deleted.
  The trace, the pinned arrows, the matched peers, the outgoing tiles, the
  outbound catalogue's inputs and the state changes read GroupsIndex's
  `Reach`; the dispatch folds keep `dispatchRelations` for the rows a table
  hands over and take the sites from `index.Dispatch` in source order (B2).
- **Pinned arrows:** one per pair of parts where a call (or read) of the
  reach enters a part from a part reached earlier; every such call is
  listed, the others counted, none chosen by length (lead's depth layering;
  the neighbourhood is rejected). Dashed only when none of the pair's calls
  is exact. Redis GET: 12 → 15 dark arrows (13 parts); all inputs: 955 →
  1,026 operation arrows.
- **Readings** (`page_input_path.go`, `29-operation-view.js`): "Dispatched
  from call · one of 94" and "How get reaches call is not established."
  (lead correction: the inputs reaching a site are not listed in a
  dispatched input's reading); "Reaches call, where 95 inputs are
  dispatched" with the input's own calls (exec, lpush, rpoplpush, rpush,
  slaveof; debug reaches loadAppendOnlyFile); "Registered by" / "Registers"
  (IOThreadEntryPoint is registered by the 11 inputs whose running code
  reaches spawnIOThread); then the parts by depth, each with every entering
  call (five, the rest folded), and "{n} other calls into it on this path".
  An identical line (two sites of one call) is written once. The site's own
  reading, with its declaration (`data-dispatch` on its part), lists "call
  is reached from these inputs:" with their calls and "Which of these, if
  any, leads to an input dispatched here is not established.", or "No input
  reaches loadAppendOnlyFile by calls". "Why it appears in get" lists the
  calls entering the part and counts the others; "One shortest static
  path", "Shared by … through" and "The path by which an input reaches …"
  are deleted from the vocabulary. A state change lists its writer's
  callers on the path instead of a call chain; a matched input's parts and
  writes join the root's as a possible integration, with no prefix chain.
- **One arrow, one flag:** the system map collapses the relations between
  two nodes into one arrow and took the first relation's quiet flag;
  `served` had hidden that for pairs on an input's path. The arrow is now
  quiet only when every relation it draws is (as the canvas groups them):
  on Redis one pair changes.
- **Page size (Redis, escaped bytes):** report.html 12,555,599 →
  12,268,850 (−286,749); `data-call-paths` 992,292 → 0; `data-input-path`
  472,245 → 831,348 (median 5,099 → 8,547, max 20,498).
- Tests deleted: `TestCallsOnAnInputsPathAreWorkNotWiring` (`served`). Tests
  rewritten on the saved reach (they call `groupindex.Derive`): the witness
  trace (now a part entered from two earlier parts draws both), the
  dispatch-without-a-route reading (with exec's reach of call, call's own
  reading, one line per identical call), invocations and data reads, entity
  writes' callers, the system joins, the folded tile, the dispatch folds
  (B2: the replay site comes first among the edges and the fold is still
  named by call), the JS reading (no reached-from list in GET's reading, no
  "through"). New: the site's reading (JS) and one quiet flag per arrow.
  Each fails on revert (s44a–h).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS.
  `make ui-visual-test`: PASS (65 passed, 2 workers). `make build`: PASS.

## 2026-09-28 — Map model step 4, S4-2: only the part holding a launch point is the entry (GroupsIndex 19)

- `projectTarget` places every declaration first, then gives each part its
  lane: `triggers` only when it holds a target seed, `dependencies` when its
  box only calls out, `core` otherwise; the subjects' categories follow that
  lane. An area is the entry when one of its parts is, else it stands where
  most of its parts stand; C5's seed scan and "a part that takes requests
  counts as core" collapse into the parts' lanes. The atlas side stays the
  reading's column fact. `groupKey` starts with the lane, so `g*` ordinals
  renumber; GroupsIndex 19 refuses v18 runs. The report reads an area's mark
  with `pageLane` directly; `areaLane` is deleted.
- **A launch point off the map** (lead, owner's "no labels"): GroupsIndex
  derives `Entries`, each seed with its part or its off-map reason. A seed
  no part holds (redis-server's `main`, undecided at 0.54 against 0.46)
  makes no entry part and no canvas label; the component's heading and
  reading say "The program's entry is not on the map: main · In no part of
  its file", and its "Not on the map" row marks main "(the program's
  entry)". Russian strings added.
- Orientation's request carries the group lanes (`internal/orientation/
  request.go`), so the next ordinary run asks orientation once live.
- Tests: `TestOnlyThePartHoldingALaunchPointIsTheEntry` (a listening part
  and a part taking requests are core, main's part and area the entry, a
  calling-out part dependencies, categories follow); the lane assertions of
  `TestProjectAtlasMakesGroupsContainersAndConnections` and
  `outbound_test.go` follow the rule; flowtest checks triggers ⇔ holds a
  seed and area triggers ⇔ holds a triggers part on every fixture (kvd's
  net.c part, which listens, is caught on revert; Echo: only cmd/api's main
  part); `TestALaunchPointOffTheMapIsNamedWithItsReason` (report). Each
  fails on revert (s42a–e).
- `make test`: PASS. `make vet`: PASS. `make ui-test`: PASS.
  `make ui-visual-test`: PASS (65 passed, 2 workers).

## 2026-09-28 — Map model step 4, S4-3: an entry keeps a line only when the model wrote one (B19)

- `atlas.Boundary.Line` now holds only a line the model wrote
  (`boundaryState.written`); a fixed row without captions, which is not
  sent, and a refused line leave it empty. The fact's given text stays the
  joint request's context (`state.line`), so no model request byte
  changes. Operation and outbound summaries are therefore empty unless the
  model explained them: 95 of Redis's 96 summaries restated their
  registration ("redis.c.redisCommand.proc get in getCommand").
- The report's string comparison (`operationSummary`) is deleted; its
  callers read `Operation.Summary`. The reading and Find already name the
  handler; an outgoing row names its call, or its kind. `operationKey`
  includes the summary, so `o*` ordinals may shift on the next run
  (harmless: they are compact identities). Until S4-2's version bump, a
  saved v18 run rendered by this code shows its given lines.
- Tests: `TestInputIsReadByItsHandlerNotItsRegistrationWords` deleted (its
  mechanism is gone); kvd's entries have no summary without captions (C
  preset); a written line is kept and a refused or unasked one leaves none
  (reading); an outgoing row without a line names its call or kind
  (report). Each fails on revert (s43a–c).
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Map model step 4, S4-1: one reach, dispatch sites and phases, derived in GroupsIndex

- Scope: S4-1 of `map-model/step4-plan.md` with the skeptic's corrections
  (step4/skeptic.md): `groupindex.Derive` computes each input's reach, the
  dispatch sites and the phases once, from the program's structural edges
  and the saved overlay. `ProjectAtlas` calls it after the joints are in,
  `Hydrate` and `Build` at their end; nothing new is persisted
  (groups-index.json and report.json keep their bytes). `WithConnections`
  is left alone (no production caller). `Snapshot` shares the derived
  slices. The report is unchanged: its BFS, `served` and `drawsInit` still
  run, now over full-reach phases.
- **One walk.** From the handler: calls, executions and outside
  invocations resolved exactly or as alternatives; reads of a variable or
  type from running code are terminal; imports, hand-overs, decorations,
  writes and unresolved calls are never followed; an alternatives relation
  is not followed into another input's handler (C1), an exact call into one
  is. Every call into a part from a part reached earlier is listed, the
  others counted; a caller off the map stands for the earlier parts reaching
  it through code off the map; a handler off the map enters its first parts
  itself. Hand-overs count only from running code (B1): `spawnIOThread`
  registers `IOThreadEntryPoint`, a table read registers nothing. Dispatch
  sites are in source order; "reached from" is computed only for a site
  that dispatches an input, from a backward pass inside each reach.
- **Phases** (B12): runtime is every input's reach, init what the seeds reach
  and no input does, both in each, none for code neither reaches; a program
  with no handled input has no phases. C5's `ToHelper` is derived beside it
  verbatim.
- **Joint gap fixed.** Joints used to get their phase only on hydrate;
  `Hydrate` also wrote the derived fields into the overlay's connections
  (shared with the projected index by `OverlayFromIndex`), which hid the gap
  in a comparison. It now derives on a copy.
- **Render-neutral on saved runs.** The step 0 Redis and litestream runs
  rendered with this code are byte-identical to their report.html. The
  static zone picture's quiet flags follow the new phases (B3), but the
  page's one figure, the system canvas, carries no static arrow and hides
  every other one without scripting: the no-script map shows 151 parts and
  areas and 0 arrows before and after (Redis; litestream 204 and 0).
- **Measured.** `Derive` on Redis (96 inputs): 13 ms, 5.8 MB allocated,
  15,954 reached declarations, 55,473 followed edges, 3,443 entering calls;
  sites 16, 2 with inputs (`call` reached from exec, lpush, rpoplpush,
  rpush, slaveof; `loadAppendOnlyFile` from debug). litestream (33): 2 ms.
  metabase's 497 MB program index (65,661 objects, 698,746 structural
  edges, 14,490 alternatives) with 3,000 synthetic inputs of the highest
  call fan-out plus its seeds: 1.35 s, 272 MB allocated, 71 MB retained.
- Tests: `internal/groupindex/reach_test.go` (reach and reads, witnesses,
  B12 phases, no phases without inputs, dispatch sites and reached-from with
  parallel routes and the C1 cut, an exact call into another handler, hand-
  overs and a table read, render equals the run with a joint);
  `internal/groupindex/flowtest` checks every fixture's derived values on its
  real facts and their hydrated equality (projectSplit in every language,
  the JS/TS map of parts, the C and Echo presets) and probes declarations as
  inputs (Python `read_level_data`/`traced_level`, JS/TS
  `recordOrder`/`runWorker`, Clojure `read-limit`/`greet-many`). kvd: one
  dispatch site (`processCommand`, kvd.c:176, 6 of 6 inputs, reached from
  none); get's reach holds its handler's work and none of the loop's stored
  callbacks; main, setupSignals, netListen and loopCreate are init,
  loopCreateFileEvent both, getCommand, addReply, statsWorker and reportStats
  runtime. Missing equivalents recorded in C, GO, PYTHON, JSTS and CLOJURE;
  Clojure records a function handed to `map` as called too, so its fixture
  has no declaration reached only by a hand-over. Each new test fails on
  revert (`step4/impl-revert.log`, s41a–s41k).
- `make test`: PASS. `make vet`: PASS.

## 2026-09-28 — Map model step 3 accepted: helpers placed by code, measured on Redis, pykrx and litestream

- Scope: step C8 of `map-model/step3-plan.md`: the acceptance of step 3
  (C1–C6 and the rule B fix), with the owner decisions of 2026-09-28 in
  CURRENT. CURRENT's format line is corrected to the code: atlas 15 and
  GroupsIndex 18 (it said 14 and 17); ProgramIndex 18, places graph 19 and
  reading input 18 were right. The map model's step 6 DNS deletion is void:
  the owner kept one DNS group with three arrows.
- **Runs.** `.bin/repomap <repo> --no-serve --no-open` on the default
  system response cache, never `--debug-dir` and never `cache clear` (the
  owner's cache): the C4 cold runs (`step3/c4c6/<repo>/run-c4-cold.log`,
  `measure-c4-cold.txt`) and the runs after the rule B fix
  (`step3/ruleb/`), whose helper, gate and assignment answers are the saved
  ones. Every run exit 0 with its complete artifact chain; warm reruns make
  0 live calls. `measure.py` reads the artifacts only.
- **Redis 1.3.6** (four programs; final run `20260928-021324`, 25 s):
  - helper question: 654 declarations asked in 46 per-file Jev requests,
    420 helpers; 74 declarations nothing uses are no helpers by code.
    redis-server: 479 asked, 284 helper, 174 responsibility, 21 near-ties;
    leads ≥0.40 400, 0.20–0.40 39, 0.10–0.20 19, <0.10 21.
    processCommand 1.00, serverCron 1.00, syncWithMaster 0.93, rdbSave 0.92
    and call 0.60 are responsibility; lookupCommand is a helper at 0.90
    (redis-cli's at 0.88), symsTable at 0.91, freeMemoryIfNeeded at 0.53,
    setGenericCommand at 0.11; genRedisInfoString is a near-tie (0.07).
    `main` is not asked: nothing uses it.
  - redis.c, the one file of 15 the gate splits: 18 boxes, none empty,
    lone or only helpers. Rule A places 82 helpers with their users; rule B
    joins staticsymbols.h, lzf_c.c and lzf_d.c (pqsort.c stays whole,
    blocked by `_pqsort`); rule C places 1 open unit; the second pass asks
    62 shared helpers and places 58. 19 units stay undecided: 4 second-pass
    near-ties (redisFunctionSym, oom, createHashObject, createZsetObject),
    13 helpers blocked by an open user and never asked, and 2 that are no
    helpers (selectCommand, main).
  - parts: redis-server 21 (0 lone, 0 made only of helpers, 4 areas),
    redis-benchmark 6, redis-check-dump 3, redis-cli 5 (4 drawn). At rest the
    system map draws 7 arrows, the redis-server map 19 (6 two-headed), its
    runtime area 10 (7 two-headed); redis-cli 4. Headless walk without page
    errors.
- **pykrx** (`--target python:.:library:library`, cold 31 s): 188 asked
  in 24 Jev requests, 50 helper, 113 responsibility, 15 none of these, 10
  near-ties; 107 no helpers by code, among them the dispatchers
  get_market_cap, get_index_ohlcv and get_etf_isin; the four shared
  helpers are helpers (get_market_ticker_name 0.78,
  get_nearest_business_day_in_a_week 0.97, market_valid_check 0.98,
  resample_ohlcv 0.92), and datetime2string 1.00. 6 of 24 files split into
  43 boxes; rule A places 5, the second pass asks 7 and places 6; 4 units
  (8 declarations) undecided. 12 parts, 0 lone, 0 made only of helpers;
  the grouping put the boxes of each of the 6 split files back into one
  part (reported, not "fixed", as the plan said).
- **litestream v24** (Go, cold 2m6s; 58 helper and 52 gate Jev requests
  over six targets, 48 and 43 of them for cmd/litestream): cmd/litestream
  293 asked, 181 helper, 102 responsibility, 10 near-ties; main.go splits
  into 3 boxes; rule A places 20, rule B none, the second pass asks 14 and
  places 12; 2 units (IsSQLiteDatabase, txidVar; 4 declarations)
  undecided. 12 parts (11 drawn), 0 lone; `replica_url.go`, 24 helpers used
  from several rows, is a part made only of helpers. The map draws 16
  arrows at rest, 18 in all. cmd/litestream-test's main.go splits into 2
  boxes; that target keeps 2 one-unit parts (main.go's Version command box
  and shrink.go).
- **Known misses and open items.**
  - pykrx `get_market_ohlcv` is a helper (lead 0.23): its only user is
    its file's `__main__` demo. Tied to the owner's open question "a
    library's public API as entries (an `export` seed kind)".
  - redis-server's `main` is undecided in the first pass (Process
    daemonization and crash handling 0.54 against Server lifecycle and cron
    0.46); nothing in redis.c uses it and what it uses spans boxes, so rule
    C leaves it and redis-server has no entry part. Step 4's entry work
    will show an off-map seed explicitly.
  - 13 helpers of redis.c stay undecided because a user was still open
    when the second pass asked: setDictType, zsetDictType,
    resetServerSaveParams, zslCreateNode, zslCreate, zslFreeNode,
    initClientMultiState, freeClientMultiState, vmMarkPageFree,
    vmMarkPagesFree, vmFreePage, vmReadObjectFromSwap, vmGenericLoadObject.
  - `dupClientReplyValue` (redis.c) stands in adlist.c's row, "Core data
    structures", by rule A: its only user is `listDup`, whose call
    `copy->dup(node->value)` (adlist.c:229) is an exact relation, a
    `function_value` dispatch resolved through its one recorded store,
    createClient's `listSetDupMethod` at redis.c:2451 (the same site also
    keeps an unresolved twin). It is not an alternatives relation, so the
    rule counts it as any exact call.
  - pqsort.c stays whole on a near-tie (`_pqsort`, 0.49 against 0.48);
    the grouping still draws it in the Sorting part.
  - Not done in this step: a repomap self-run's extra helper requests and
    the two extra cold draws per repository for margin statistics.

## 2026-09-28 — A whole file joins its users' box only when it is a file of helpers

- Scope: the lead's fix to C4's rule B after the litestream run, before
  C8. Rule B now reads the map model literally: a library file is a file
  of helpers.
- **Change.** A whole file that is neither test nor generated code joins a
  box of a split file only when every unit it declares, its types
  included, is a decided helper and all its users in other files
  (callers, decorated units, readers; never a hand-over; no test or
  generated code) stand in that one box. A type answered responsibility, a
  function nothing uses, `none of these`, a near-tie or an unanswered row
  keeps the file out. The face condition (only the units other files use
  had to be helpers) is deleted: types have no use facts, so a Go file's
  client type never counted. On litestream `cmd/litestream` the abs, gs,
  nats, oss, sftp and webdav `replica_client.go` files, whose only used
  unit is the helper `NewReplicaClient` (0.75–0.84) called from main.go's
  `newXReplicaClientFromConfig`, had joined main.go's "Replica client
  setup" box: a 114-function part over seven files, while "Storage
  backends" kept only file and s3. Rules A and C and the second pass are
  unchanged.
- **Tests.** `TestAFileWithAResponsibilityJoinsNoBox` (the Go shape: a
  file of a responsibility type and a helper constructor Store.Get alone
  calls keeps its own row, no join recorded, the constructor keeps its
  mark) and `TestAFileOfHelpersJoinsTheBoxOfItsUsers` (the C shape: a
  file of two helpers joins Storage; one near-tie among its units, or a
  whole file also using it, keeps it out). Both fail on the face rule. The
  C fixture's `staticsyms.h`, whose type `kvSymbol` the split check takes
  for no helper (a type has no users), now keeps a part of its own; READING
  and C say so.
- **Runs** (`.bin/repomap <repo> --no-serve --no-open`, the default system
  response cache, so the helper, gate and assignment answers are the saved
  ones of the C4–C6 runs; no `cache clear`):
  - Redis 1.3.6, exit 0 in 25 s (one live parts request, redis-server's):
    `staticsymbols.h`, `lzf_c.c` and `lzf_d.c` still join (their one unit
    each is a helper at 0.93–0.95). `pqsort.c` no longer joins: its
    `_pqsort` is a near-tie (helper 0.49, responsibility 0.48); `swapfunc`,
    `med3` and `pqsort` are helpers at 0.95–0.99. It is a row of its own,
    which the grouping puts in the Sorting part beside redis.c's Sorting
    box. redis-server: 21 parts (20 before), 0 lone, 19 units undecided
    (unchanged); `lzfP.h` now stands alone as "Compression support".
  - litestream v24, exit 0 in 65 s: the six backends stay out of main.go's
    box, each kept out by its `ReplicaClient` type (responsibility at
    1.00) and its `init`, which nothing uses. All eight
    `replica_client.go` files and `s3/leaser.go` stand in one "Replica
    clients" part; "Replica client setup" holds main.go's 11 units.
    cmd/litestream: 12 parts (9 before), 0 lone, 2 units (4 declarations)
    undecided. `replica_url.go`, whose 24 units are all helpers used from
    several rows, is a part of its own made only of helpers.
  - pykrx library target, exit 0 in 3 s, all answers cached: no file joined
    before or after; the map is identical.
  - Warm reruns of Redis and litestream: 0 live calls. Headless walk of both
    reports without page errors (`step3/ruleb/shots`).

## 2026-09-28 — A part's tiles stand keys, then types, then the rest

- Scope: step C6 of `map-model/step3-plan.md`, third part (owner decision
  of 2026-09-28: types after keys).
- **Change.** `groupSymbols` lists a part's declarations as the model's
  keys, then its types, then the rest; the canvas places the tiles in the
  page's order within each file (`symbols.mjs` keeps it, sorting nothing
  else), so a part's data stands before the code that works on it. REPORT
  says so; its off-map sentence now names GroupsIndex "since v17".
- **Tests.** `TestPartTilesStandKeysThenTypesThenTheRest` (a key function,
  two types, then a function, whatever their lines); with types among the
  rest it fails (`step3/impl-revert-c4c6.log`).
  `TestPartTilesCarryTheirFileAndSource` no longer pins the incidental
  order of its two tiles.

## 2026-09-28 — An area is purple when any part in it is the domain

- Scope: step C6 of `map-model/step3-plan.md`, second part, with
  correction 8 (the owner's decision of 2026-09-27; skeptic 20).
- **Change.** The report reads an area's mark from its GroupsIndex
  container (C5): `pageLane(container.Lane, container.Core)`. The report's
  own seed scan is deleted. An area holding the program's entry and a
  domain part is now purple, not green: 4a6892a4's "entry mark even when a
  core part stands in it" was an agent's rule and is reversed; the green
  entry mark stays for an entry area with no domain part.
- **Tests.** `TestAreaHoldingTheEntryShowsTheEntryMarkEvenWhenCore`, which
  pinned the reversed rule, becomes `TestAnAreaHoldingADomainPartIsPurple`
  (the entry's core area and another core area purple, a plain area none,
  an entry area without a domain part green, parts keep their own marks).
  The entry mark put over core, and core ignored, each fail it
  (`step3/impl-revert-c4c6.log`).

## 2026-09-28 — Quiet arrows into helpers like wiring, with wiring's exception

- Scope: step C6 of `map-model/step3-plan.md`, first part, with
  correction 7 (the owner accepted "quiet by helpers and init, the same
  mechanism", map-model §4).
- **Change.** A part-to-part arrow whose relations all go into helpers
  (GroupsIndex's derived `ToHelper`) is drawn like initialization: only
  while one of its ends is looked at, and only in a target that serves
  something (`drawsInit`, the exception of 4a6892a4). Unlike wiring it is
  quiet even on an input's path, since every command handler calls its
  reply helpers. A target none of whose arrows would stand at rest draws
  its calls into helpers: quieting them never empties a map (skeptic B6's
  redis-cli case). The canvas's structure arrows and the no-script static
  picture follow one rule. No web rebuild: the flag reuses `init`.
- **Tests.** `TestACallIntoAHelperStandsQuiet`: in a serving program the
  arrow into the reply helper is quiet on an input's path and the other
  stands; a program that serves nothing, and one whose every arrow goes
  into helpers, draw it; the static picture quiets the same arrows. No
  helper quieting, no exception, quieting not gated by serving, helper
  arrows subject to the input-path rule, and a static picture without
  helpers each fail it (`step3/impl-revert-c4c6.log`).

## 2026-09-28 — GroupsIndex carries the helper mark, and an area's marks come from data

- Scope: step C5 of `map-model/step3-plan.md` with correction 4 (skeptic
  B1). GroupsIndex 17 → 18.
- **Change.** A subject's interpretation keeps the atlas symbol's helper
  mark (`helper`, saved). `Connection.ToHelper` (never saved) is derived in
  `applyPhases`, beside `Phase`, from the saved marks of the program's own
  subjects, on projection and on hydrate alike, so a report rendered from a
  saved run marks the same connections as the ordinary run. A container's
  lane is `triggers` only when one of its groups holds a target seed;
  otherwise the majority of its groups' lanes, a part that takes requests
  counting as core. Its `core` stays "any group is core" (owner,
  2026-09-27: purple when any part is the domain). The report does not read
  either yet (C6).
- **Tests.** `TestAConnectionIntoAHelperIsMarkedAfterDecoding` (only the
  relation into the marked subject is marked; the saved JSON carries the
  interpretation and no derived field; the mark survives `Encode` →
  `Decode`) and `TestAnAreaIsTheEntryOnlyByItsSeedAndCoreByAnyPart` (the
  seed's area is triggers and not core; an area whose part takes requests
  without the seed is not triggers, and is core by its one domain part).
  Reverted alone, each of: no helper in the interpretation, the mark
  derived only on projection, any taking part making the area triggers,
  and area core ignoring its parts fails them
  (`step3/impl-revert-c4c6.log`).

## 2026-09-28 — Ask once whether a declaration is a helper, and let code place helpers with their users

- Scope: step C4 of `map-model/step3-plan.md` with corrections 1, 2, 3, 11
  and 12 and skeptic points B2, B3, 4, 5 and 9. The owner approved the
  helper question; steps 0b and 0c measured its baseline criteria with the
  no-users rule (Redis passes; pykrx's library target is NO-GO on
  `get_market_ohlcv` alone, and the lead decided C4 goes).
- **Change.** A new closed table `atlas_role_helper`
  (`repomap.atlas.role_helper.v1`, Jev, `lines.RoleHelper`) asks once per
  unit of every file of the target that is neither test nor generated code,
  one request per file, rows `dN` by the unit's place in its file: helper,
  responsibility or none of these. The task is `role_map.md` plus
  `role_helper.md`, the probe's text with the one `read_by` sentence step 0b
  measured; the options are the probe's baseline criteria
  (`role_helper_options.md`), `none of these` a listed option with its own
  criteria as the probe asked it, not the table's optional cell (a
  deviation from the plan body, which assumed the optional cell; step 0b/0c
  measured the listed one). No `exported` field (correction 1). The item:
  name, kind, file, signature, lines, methods, target-wide `calls` and
  `called_by`, `read_by` and `handed_over_by` as "path:name" without test
  or generated code, `registered`. A function, method, lambda or variable
  nothing uses (calls, decorations, hand-overs and reads, exact or
  alternatives, and registrations) is not asked and is no helper; types and
  module bodies are always asked; `tables.md` names what the rule took out.
  Only a decided `helper` is a helper. The question runs beside the gate.
  A helper is never named or assigned; the naming also drops its name from
  the others' calls and callers. Code then settles to a fixed point: rule A
  puts a helper of a split file whose users (callers, decorated units,
  readers of what does not run; never a hand-over, correction 2) all stand
  in one row into that row, which may be a box of another split file or a
  whole file's row (skeptic 4); rule B joins a whole non-test,
  non-generated file whose non-empty face is all helpers used from one box
  of a split file to that box (skeptic 5); rule C is C2's, its callee
  fallback over units that are no helpers (skeptic B3). A second pass asks
  the assignment once more about the helpers still open whose users stand
  in two or more rows or that nothing uses, in round `len(targets)+round`
  so its windows keep the first pass's (skeptic B2), then code settles
  again (correction 3). A file splits only when the assignment fills two
  boxes with units that are no helpers. Records: `role_attached` (helpers
  by name, joined files by path), `role_second_pass`,
  `role_placed_by_users`/`uses`, `role_undecided`, `role_not_split` for a
  gated file with fewer than two units that are no helpers; `tables.md`
  counts boxes holding only helpers. The atlas symbol carries the MODEL
  `helper` mark (atlas 14 → 15); a parts row may now hold units of other
  files, which bring their files to the part's sources.
- **Known miss (correction 12).** pykrx's `get_market_ohlcv`, a public
  dispatcher whose only user is `stock_api.py`'s `__main__` demo (the
  module body stays a unit of the library target), is asked and comes out
  helper at 0.17–0.35 in 8 of 8 step-0c draws. It is tied to the owner's
  open question "a library's public API as entries (an `export` seed
  kind)": if the owner says yes, "an entry is never a helper" is a code
  fact and fixes it. No launch-code exclusion was added; on data it would
  also take out 8 current helpers used only by demos.
- **Missing equivalents, recorded not patched (correction 11).** Go emits
  no reads, so no Go item carries `read_by`, and a call in a package `var`
  initializer or a function value stored in a var, table or slice leaves
  its function with no user (GO). A Clojure macro's use leaves no relation,
  so a macro nothing else uses is no helper by code instead of being asked
  (the fixture's `ensure!`; CLOJURE).
- **Tests.** `TestHelpersAreNotNamedAndGoWithTheirUsers`,
  `TestASharedHelperIsAskedOnceInASecondPass` (placed by the answer,
  undecided on a near-tie, pass-2 windows in round 3 beside round 1's, no
  unit assigned twice), `TestAHelperFileJoinsTheBoxOfItsUsers` (and not
  when a whole file also uses it), `TestTheHelperItemCarriesItsUsers`
  (read_by, handed_over_by, registrations, a test caller left out, no
  question for what nothing uses), `TestAnUncertainHelperAnswerIsNoHelper`,
  and the cache test's helper counts (warm 0 live; an earlier file 1 live
  group; a call from another file re-asks that file's group). partstest's
  split check adds the helper question on real facts (a helper has users,
  is not exported and is not registered) with its invariants: nothing of
  test or generated code asked, nothing asked twice, no helper named, no
  unit assigned twice, a split-file helper whose users all stand in one
  part is in it. Fixture cases: C `saveSnapshot` with `bgsaveCommand`,
  `staticsyms.h` joining `printSymbols`'s box, `addReplyBulk` and
  `addReplyLong` asked once more; Go `lookupCommand` with
  `DispatchCommand`, which is not asked, and the table's handlers asked
  once more; Python `format_score` keeping `exports.py` whole and the
  levels constants' `read_by`; TypeScript `handledOrderIds` with
  `recordOrder`, `paintColor` (only re-exported) not asked; Clojure's new
  private `exclaim` (appended to `core.clj` with `cheer`, no line moved)
  with `cheer`. The kvd fixture shows the plan's limit: `addReply`, whose
  callers all stood in one box while a caller was still open, is blocked,
  so not asked in the second pass, and stays undecided after it. Each rule
  reverted alone fails its tests (`step3/impl-revert-c4c6.log`, 12
  reverts).

## 2026-09-28 — Split files before grouping: one parts request over units, one rule for every file

- Scope: step C3 of `map-model/step3-plan.md` with corrections 5, 6 and 10
  and skeptic points 6, 7, 8, 12 and 13. The parts request grouped whole
  files and the role split then cut files out of the answer's parts: stale
  part names (pykrx's "Stock market API" holding one ticker.py), a
  `role_not_applied` branch, "a role part's membership is final" and a
  split file with no endpoint needing its own branch in every lookup.
- **Change.** Each target's role split runs first. The parts request
  (`repomap.atlas.parts.v2`) lists units: a whole file under its `f*`, or a
  box of a split file under a request-local `c*` with its name in `box`
  (empty boxes are no row). `calls` count per exact call site each other
  row it reaches, from the unit whose code holds the site; `imports` are
  between whole files only. The decoder reads `units`, or `files` as the
  same list (both given differently refuse that group). One unit sends no
  request. The placement follow-up (`repomap.atlas.placement.v2`) asks per
  left-out or conflicting unit. A part holds its units' declarations; a
  file's own part is the one part holding its placed own units (a whole Go
  file with a method of another part's type keeps it; a methods-only file
  takes its declarations'); a place takes its declaration's part, else the
  module body's, else the file's; the entry is the parts of the seed
  declarations, else the seed file's part. Deleted: `applyRoles`,
  `role_not_applied`, dropped drafts, `designOutcome.split/undecided`,
  `partOfUnit`, `r.splitFiles`, the view's file/unit call maps, `box.rows`,
  the unused `designFiles`. GroupsIndex 16 → 17: an off-map entry of a file
  a part still holds is listed by its subjects under its own reason
  (`left_out`, `conflict` or `undecided`); a file no part holds is listed
  whole. REPORT's data sentence on that record is corrected.
- **Tests.** `TestSplitFilesAreGroupedAsUnits` (rows, per-site calls, no
  import into a split file, answer names and IDs, a part holding two boxes,
  undecided off the map with the file on it, gate candidates, the one-file
  split target sends a request, `role_box_empty`/`role_undecided`),
  `TestOneRuleForEveryFile` (a read inside `Store.Flush`, declared in db.go,
  stands in Storage, not db.go's Database; plus the kept arrow, entry and
  core checks), `TestALeftOutBoxKeepsItsFileOnTheMap`,
  `TestSavedUnitsAnswersKeepEveryUnit` (the probe's six saved grouping
  answers as exact bytes over their `u*` refs: 23/20/17 and 9/9/12 parts,
  every unit placed), `TestPartsUnitsAndFilesAreOneList`, and the GroupsIndex
  off-map test listing a stray method and a left-out box by subject. partstest
  checks the v2 request on every language's real facts, and
  `TestFilesOfMethodsDeclaredElsewhereStayOnTheMap` now also holds h.go to
  its own part while it declares a method of another part's type
  (correction 6). Deleted: `TestTheRoleSplitDrawsAFilesBoxesAsParts`,
  `TestPlacementOnlyAddsWholeFiles`, `TestAPartKeepsItsOtherFiles` (they
  pinned the deleted mechanisms). Each rule reverted alone fails its tests
  (`step3/impl-revert.log`): the old boundary branch, a `files`-only
  decoder, units/files that differ accepted, every file one whole row
  (also every fixture's request check), a file's part over all its
  declarations, and the old GroupsIndex off-map listing.
- **Saved-window check.** The v2 request's shape and output allowance
  (8,192 tokens up to 512 rows) are the probe's: its six complete windows
  (28–69 units) were sent live with this shape and answered in at most
  1,044 bytes; `TestSavedUnitsAnswersKeepEveryUnit` replays them.
- **Acceptance, Redis 1.3.6** (`.bin/repomap ~/git/redis-1.3.6 --no-serve
  --no-open`, default response cache; receipts in
  `map-model/step3/c3-redis/`). Cold run exit 0 in 38 s (27–38 s before),
  129 live exchanges; every artifact present: one common manifest,
  `report.json` and `report.html`, reduced documentation, the ProgramIndex
  set, dependency catalog and GroupsIndex (v17) of each of the four
  programs, places (v19), atlas (v14), `tables.md`. Warm rerun exit 0 in
  6 s with 0 live calls in every stage (133 windows cached); `report.json`
  differs only in `timing`. The gate split redis.c alone (1 of 15
  candidates); its naming gave 25 boxes, none empty, two holding one unit
  (Pattern matching: stringmatchlen; Daemonization: daemonize). The
  assignment left 7 units open; the code rule placed 2 by their users
  (rdbSavedObjectPages into Virtual memory, deleteIfSwapped into String
  commands) and 0 by what they use; 5 stay undecided (saveparam, iojob,
  createZsetObject, dontWaitForSwappedKey, debugCommand). redis-server's
  parts request listed 42 units (17 whole files, 25 boxes; 25,320 bytes)
  and drew 29 parts in 5 areas: each box its own part (no box re-merged by
  file), the libraries grouped by file, and 3 parts of one unit (the two
  lone boxes and staticsymbols.h's Static symbols, which `symsTable`'s read
  does not join to findFuncName until the helper rule). redis-benchmark 6
  parts (10 files), redis-check-dump 3 (4 files), redis-cli 5 (7 files, 1
  never run). No parts answer row was refused and no placement follow-up
  was needed in any program. Headless Chromium walk at 1440×900 of the
  system map and redis-server at rest (`shots/`): no page error; five
  areas and two loose parts, "In no part of its file" listing the
  undecided units.

## 2026-09-28 — An open declaration goes where its file's users are; the neighbours' question is deleted

- Scope: step C2 of `map-model/step3-plan.md` with its corrections 2 and 5.
  The neighbours' question (A13, `atlas_role_neighbours`) asked the
  assignment again with the boxes of a unit's calls and callers: where a
  unit's users are is a code fact, and asking re-decided what the first
  question had decided.
- **Change.** After the assignment, `settleOpen` places each open unit by
  code, to a fixed point: box k when every unit of its file that uses it
  (an exact call, a decoration, an exact read of what does not run; places'
  `uses` from C1) has box k; with no such user, k when everything of its
  file it uses has box k; otherwise it stays undecided. A hand-over is no
  use (cmdTable's 97 handlers, a Go `HandleFunc` or JS `app.get` registrar),
  and neither is a read of a callable, which JS/TS writes where it hands a
  handler over. Each placement is recorded as `role_placed_by_users` or
  `role_placed_by_uses`. Deleted: `lines.RoleNeighbours`, its stage and
  prompt, the debugdump stage, `askNeighbours`, `boxLabels`, `countValues`,
  the assignment item's box annotations.
- **Tests.** `TestAnOpenUnitGoesWhereItsSameFileUsersAre` (helper and a read
  variable go to Storage by their users, main by what it uses, a handler
  its table hands over and its registrar reads as a value stays undecided,
  a unit whose users sit in two boxes stays undecided, each unit asked
  once, records written); `TestAnInputWithAnUndecidedHandlerNamesNoPart`
  now places helper by code; roleGraph's main also calls helper, so the
  tests that pin helper as undecided stay green (skeptic B5). partstest
  drops its asked-again fake and checks on every language's real facts that
  no undecided unit is one the rule places. Reverting the rule fails both
  reading tests and the Go, Python, Clojure and TS fixtures; counting
  hand-overs fails the first (`step3/impl-revert.log`).

## 2026-09-28 — The places graph records what a declaration reads, hands over or is decorated by

- Scope: step C1 of `map-model/step3-plan.md`. `places.json` carried no reads:
  places lifts only pattern-bearing relations besides calls, executes and
  outside invocations, so in the 20:55 Redis run `findFuncName` had no entry
  for `symsTable`, and pykrx kept 24 of its 88 exact decorations.
- **Change.** `SymbolFacts.Uses` (places graph 18 → 19): for each lifted
  declaration, the declarations its program index's exact or alternatives
  `reads`, `passes_callback` and `decorates` relations name, with or without
  a pattern, each once with kind and resolution. Sealing maps them to compact
  `s*` IDs like the callers; a use of a place that is not a declaration is
  refused. Local keys only; no provider request reads them yet.
- **Fixture expectations** (`adaptertest.AssertDeclarationUses`): C
  `printSymbols` reads `symsTable`, `keysCommand` hands `compareKeys` to
  `qsort`, `cmdTable` hands `getCommand` over; Python `read_level_data` reads
  `READ_VALUES` and `READ_LIMIT` across files and the new bare-decorator case
  `traced_level` uses `traced` (appended to models.py); TS `recordOrder` reads
  `handledOrderIds`; Clojure `read-limit` reads `service/source-limit` and
  `greet-many` hands `service/greet` to `map`; Go `commandTable` hands
  `getCommand` over and `registerRouteDefinition` hands `http.HandleFunc` the
  closure `requireRouteToken` returns. Recorded, not patched: Go emits no
  reads, and a call in a package `var` initializer or a function value stored
  in a package variable leaves no relation (GO); a Clojure macro use leaves
  none (CLOJURE). JS/TS also reads a function it names as a value, so a
  registered handler is both read and handed over by its registrar.
- **Revert check** (`step3/impl-revert.log`): with the wiring, remap and
  validation reverted, the five fixture assertions and
  `TestSealedGraphKeepsEachUseOnce` fail.
- CURRENT's format line now names the code's versions: ProgramIndex 18,
  places 19, reading input 18, atlas 14, GroupsIndex 16 (it said 17/16/18/11/13).

## 2026-09-28 — A Python module's own names after its star imports resolve

- Scope: gap (a) of the pykrx investigation (`map-model/step3/pykrx-api.md`,
  Q2): `import_target` answered unknown for every member of a module with
  any `from … import *`, before reading the module's own bindings, so
  `from pykrx.website import krx; krx.datetime2string(...)` stayed
  unresolved although krx/__init__.py defines it after its four stars.
  Gap (b), following a star, is not part of this change.
- **Fix.** The collector records each module-level star import's statement
  position (line, column) instead of one flag, and each export binding's
  position. A binding the module writes once, unconditionally, in a
  statement after its last star resolves as it would without the stars;
  a name only a star binds, one written before a later star, and a child
  module the package does not bind stay unknown (PYTHON, which now states
  the ordering rule).
- **pykrx (fb0d9b3, library target `python:.:library:library`, no model,
  output under the session's `star-fix/`).** The 152 `krx.<name>(...)` calls
  were all unresolved; now 77 are exact (`datetime2string` 74 →
  krx/__init__.py:9, `get_nearest_business_day_in_a_week` 3 → :18) and the
  75 that only the stars bind stay unresolved. Across the whole index
  exactly those 77 relations changed (exact 888 → 965, unresolved 1744 →
  1667, objects 2908 both). pykrx's checkout was clean before and after.
- **Fixture.** `import_facades/star_facade/__init__.py` (a `def shadowed`,
  then `from .rates import *`, then `def to_text`), `star_facade/rates.py`
  and `star_consumer.py`, calling through `from . import star_facade` and
  `import … as facade_alias`. `TestCumulativePythonStarFacadeKeepsItsOwnLaterDeclarations`:
  both `to_text` calls exact, `get_index` and `shadowed` unresolved; it fails
  with the old parser (to_text unresolved) and when any binding of a star
  module resolves whatever its position (shadowed becomes the facade's).
  `StarOnly` stays unresolved in the existing facade test.
- **Equivalents.** TypeScript `export *` plus an own export through
  `import * as` (`src/facade-exports/star-*.ts`,
  `TestCumulativeJSTSStarBarrelKeepsItsOwnExport`) and Clojure `:refer :all`
  plus an own `defn` through an alias (`example.facade`, `example.rates`,
  core's `facade-text`, asserted in `TestClojureFixtureInventoryAndNativeGraph`)
  already resolved: both expectations are new, with no production change,
  and each fails when its fixture drops the `export *` or `:refer :all`. C
  reuses `kvd.h` (includes loop.h and strbuf.h, declares `kvAssertFail`,
  called exactly from kvd.c's `acceptHandler`). Go has no wildcard
  re-export: not applicable. Revert and mutation log: `star-fix/revert.log`.

## 2026-09-28 — "No tests" and "unused" said only what the report knows

- Scope: two of the three wrong claims a blind judge found in a newcomer's
  Redis 1.3.6 onboarding document written only from the report (benchmark
  R01, claims B7 and B85). Checked on the saved Redis run (20260927-205506)
  with `repomap render` (0 provider requests) and on the C fixture's two
  programs read together; logs and revert checks in the session scratchpad,
  `negatives/`.
- **Tests.** The fact said "no recognized test files found in inspected
  paths"; the page led with "No test files found", and the newcomer wrote
  "no test files" over test-redis.tcl's 204 tests and the Makefile's `test:`.
  The page now says "No recognized test files found in the inspected paths."
  (Russian: "В просмотренных путях не распознаны тестовые файлы."), without
  repeating the fact after it. `isTestPath` is unchanged (lead's decision:
  there is no Tcl adapter, and one Redis file is no reason to widen it); its
  only consumer is `addNegatives`, whose negative reaches the orientation
  request and the page. No fact records a Makefile `test` target: the C
  adapter's dry run reads only the default goal (`make -n -B -w -o
  Makefile`), and Makefile rules as run recipes are out of scope (C).
- **Unused.** redis-server's "Not reachable from the entrypoints" listed
  `aeStop`, `anetRead` and `anetWrite` with no note, and the newcomer told
  readers to skip them as unused; redis-benchmark runs `aeStop` and redis-cli
  does all its network I/O through the other two. The list now says, as the
  files' list did, that this does not establish unused code, and names the
  other programs that run each declaration: "aeStop:82 (run by
  redis-benchmark)", "anetResolve:107 (run by redis-benchmark, redis-cli)",
  "anetRead:182 (run by redis-cli)". On the saved run redis-cli's "Linked
  list" part reads "listCreate:41 (run by redis-server, redis-benchmark)",
  and `listRewindTail`, `sdstoupper` and `zipmapRepr`, which no program runs,
  name none.
- **Layer.** Page data joined from the saved ProgramIndex set, not
  ProgramIndex (one target's index must not carry another's facts) and not
  GroupsIndex (a thin per-target overlay; a copied cross-target fact would
  store the fact graph twice and change its format for what the saved marks
  already say). No graph is walked: a program runs a declaration when its
  index holds the same declaration, by the identity GroupsIndex already uses
  across programs (`groupindex.DeclarationKey`, now exported: path, line,
  column, kind and name), does not mark it `unreachable`, and marks some
  other callable `unreachable`. That last condition is the C adapter's
  all-or-nothing proof (a library or an unprovable program marks nothing),
  now written in PROGRAM_INDEX. A program reaching all its callables names
  nothing; a missed name is safe, an invented one is not.
- **Equivalents.** Only C proves `unreachable`; Go, Python, JS/TS and
  Clojure record in their contracts that two programs sharing code list
  nothing either never runs, so nothing is named. Files are already judged
  dead repository-wide.
- **Tests.** `TestNoTestsNegativeSaysOnlyWhatIsRecognized` (English and
  Russian page sentences; fails on revert with "No test files found (…)").
  `TestCRepositoryPageListsWhatAProgramNeverRuns` now reads kvd and kvcli
  together: kvcli's thirteen loop/net/strbuf declarations each "run by kvd",
  kvd's `netConnect` "run by kvcli", both under the note (fails on revert;
  with `markRunBy` a no-op it fails on the missing names).
  `TestAnUnreachedDeclarationNamesTheProgramsThatRunIt` keeps the contrasts:
  a same-named function in another file and a library index that marks
  nothing name no program (fails when an unmarking index counts).

## 2026-09-28 — Navigation bugs from the Redis benchmark: levels, breadcrumb, input path

- Scope (lead): bugs only; the canvas look and camera policy stay frozen.
  Checked on the saved Redis run (20260927-205506) rendered with
  `repomap render`, served on loopback, headless Chromium with real pointer
  moves at 1440×900 and 1280×800 (driver and logs in the session
  scratchpad, `nav-fixes/`).
- **Breadcrumb.** "redis-server (executable) / Server runtime / Replication
  · syncCommand" was one link; clicking "Server runtime" or the component
  left the camera at zoom 11.1 on syncCommand's tiles and the reading on
  Replication. Each segment is now its own link that goes up to its level,
  reading and camera together: "Server runtime" reads the area and frames
  it (zoom 2.0, parts open), the component reads it and frames it whole
  (0.69, areas closed); same at 1280×800 (2.36, 0.64).
- **"−".** It zoomed by 0.8 and stayed on the level: component 0.69 → 0.55,
  area 2.0 → 1.6, both unchanged. It now steps out one level as a zoom mark
  steps in: tiles → their area (11.1 → 2.0), area → component (2.0 → 0.69),
  component → whole map (0.69 → 0.34); "+" still zooms by a quarter.
- **Pinch.** Eight ctrl+wheel ticks (deltaY 40, 60 ms apart) over
  Replication went whole map → components → areas → tiles (levels
  0,1,1,2,2,2,3). One pinch now crosses one level boundary, a pause
  (300 ms) ends it: three pinches go 0→1, 1→2, 2→3, one pinch out 3→2.
  Two layers that open at one zoom (the fixture's small component and its
  areas) are one boundary; the first design, one level either way, froze a
  pinch at the whole map there.
- **Legend.** The tiles' purple dashed links (a function to the type it
  returns, a type to the function taking it) were unexplained; the key
  names them, "returns or takes a type", in the tiles' own purple and grey
  head, when a part draws one.
- **Wheel.** Over the canvas a plain wheel panned the map, as DEVELOPMENT
  requires, except over the 30 px location row, where it scrolled the page
  100 px, and past the end of an overflowing component inventory, where the
  next notch scrolled the page and the one after panned the map under a
  still pointer (fixture). The row now forwards the wheel to the map and an
  inventory contains its overscroll. The wheel's slowness the testers
  reported is timing, left alone.
- **Input path.** The reading offered "One shortest static path: main →
  aeMain → beforeSleep → call" and a box through loadAppendOnlyFile for
  GET, the benchmark's two wrong lures. The entry chain is gone (Go
  `entryGraph`/`chain` and its test assertions deleted); each shared
  dispatch keeps its fact, "call → one of 94", and says "The path by which
  an input reaches call is not established." The handler's own steps stay.
  A later change is to compute the chain from input handlers in GroupsIndex.
- **Catalog rows.** A plain click on "flushdb" or "flushdbCommand" in the
  component reading's 95 rows opened GitHub. When the row's own link names
  one input ("To explanation", `#t1-o1`), a plain click reads the input
  ("Operation · request flushdb", camera on its path); a modifier-click
  opens the code. No new page data was needed.
- **Visual suite.** `make ui-visual-test` failed 18 tests on main, 15 of
  them on PNG baselines last reviewed on 2026-09-15. All 37 baselines and
  the snapshot config are deleted; every test keeps its semantic checks.
  Four semantic failures, decided one by one: edge-size pinned a 2 px
  outline where the frame looked at draws 2.5 px (now: the screen width is
  the frame's own at every zoom, and the painted colour is read from it);
  the chip-width equality of gesture-star ignored each number's hover
  padding (height kept); group-entrance's 10 px floor for every group
  measured the new input groups too (5.7–9.5 px; now the largest title at
  the first reveal reads ≥ 12 px, measured 12.1 and 12.2); layout-work
  compared placed nodes to records without the input groups and then hit
  CSSOM's six-digit rounding at 13443.9 (now: every record placed, only
  `…-inputs~part` added, tolerance from the serialized digits). Six specs
  that used "−" as a fifth-out step now make one ctrl+wheel tick.
- Run measurements moved out of REPORT.md's card-text and whole-map-fit
  rules: "Implements Redis set commands and" measured 225.84 px broke in the
  225 px column; a browser drawing the 1.5 px border 1 px wide left 226 px,
  where pykrx's 225.39 px "Fetches Korean market fundamentals" fit whole;
  measured over the participants alone, the camera framing Redis's "DNS
  resolver" group stood 0.85% smaller than the fit ("DNS resolve", "TCP
  endpoin").
- Tests, each failing with its fix reverted (checked):
  `TestEachBreadcrumbSegmentGoesUpToItsLevel`, `TestKeyNamesTheTilesTypeLinks`,
  `TestACatalogRowReadsItsInputOnAPlainClick`,
  `TestAnInputsPathNamesItsDispatchWithoutARouteAndListsItsOwnSteps` (Go),
  `TestAnInputsPathNamesItsDispatchThenItsOwnSteps` (JS, was
  `TestAnInputsPathIsTheSharedChainThenItsOwnSteps`), `visual/levels.spec.mjs`
  ("−" twice, pinch, location row, inventory end), node tests for
  `detailLevel`, `pinchZoom`, `zoomBelow`; `visual/real-navigation.spec.mjs`
  walks the breadcrumb and a catalog row on `REPOMAP_REAL_RUN` (passes on
  the Redis run).

## 2026-09-28 — Reading-column bugs from the Redis benchmark

- Benchmark participants studied Redis 1.3.6 in the reading column. Scope
  (lead): bugs only, no new features; the input path's shared box, the
  canvas and the camera are left alone. Checked on the saved Redis run
  with `repomap render` and headless Chromium with real pointer moves at
  1440×900 and 1280×800.
- **Late shift.** "Outside this input path" came from the canvas's
  emphasis: a frame late, and again each time the pointer left the canvas
  (hover had turned it off). Reproduced: with GET pinned, a click on
  Introspection and debugging, then the pointer to the column, moved
  pingCommand 567 → 593 px, and a click aimed at it read monitorCommand.
  The line is now drawn with the reading from its own state
  (`projection.outside`); the canvas's `readingOutside` is gone. After:
  593 px throughout, and the aimed click reads pingCommand.
- **Names opening GitHub.** A name in the input's own path steps, its
  "handled by" line and the column's frame Connections rows opened its
  code in a new tab. It now
  reads that declaration in its part, as a tile click does; a
  modifier-click still opens the code, and a path step or connection row
  ends in an explicit "Open code ↗" (a connection row's is where the call
  is written). The page
  data names the declarations at a call's ends (`caller`, `callee`), since
  a call's `from` is where it is written and `to` where it lands (the
  joint's anet.c:256 is inside anetAccept, declared at 248); the Redis page
  grew 12.60 → 13.21 MB. Column rows wrap with names whole to make room
  for the link. The canvas card and the shared "One shortest static path"
  box are unchanged. After: addReply reads Client connections and replies
  with addReply chosen, anetTcpGenericConnect reads redis-cli's Network
  sockets, no tab opens; a modifier-click opens one.
- **TODOs link.** It landed on its heading at the page foot with "10
  markers in 6 files" closed. A heading reached by a link opens the list it
  heads; the list now stands open under it (heading at 390 px of 900).
- **Source details off-screen.** Opening "Source details · 11" under
  Persistence left three of its five lines below the column; Open all left
  four. What the reader opens in the column scrolls into view, no further
  than its summary at the column top: all five lines after opening, four
  (1440×900) or three (1280×800) after Open all, against one.
- Tests, each failing with its fix reverted: `TestAnArrowsCallNamesTheDeclarationsAtItsEnds`,
  `TestAPartOffThePinnedInputSaysSoWithItsReading`,
  `TestWhatAReaderOpensInTheColumnComesIntoView`,
  `TestAHeadingReachedByALinkOpensTheListItHeads`,
  `TestAnInputsHandlerNameReadsItsDeclaration`, the extended
  `TestAnInputsPathIsTheSharedChainThenItsOwnSteps`, a `callCard` node test
  and a fixture spec (a name in the reading's connection reads its
  declaration; its code is an explicit link). Deleted: the two
  `readingOutside` assertions of the emphasis test.
- `make test`, `make vet`, npm test (123), `build --check` pass;
  `make ui-visual-test` fails the same 18 tests as main (names compared),
  42 pass. Left: the component reading's input catalog still links an
  input's name to its registration and its handler to the code (95 rows on
  Redis); those rows are shared with the component reference below the map.

## 2026-09-28 — Arrow ends open their cards for a real pointer on Redis

- A usability tester on the saved Redis run (1440×900, real pointer, 1.1 s
  rests) never opened an arrow card at any level; the fixture spec passed.
  Measured on that run rendered with `repomap render`, headless Chromium:
  - the numbered frame came from `parentArea` alone (canvas.jsx, `chosen`),
    which is empty for a component, with a fallback to the one open
    component; entering redis-server opens four, so choosing it numbered
    nothing, and its border numbers stood only while one of its areas was
    pointed at and vanished as the pointer crossed the component's own
    space to them (0 of 5 chips reached);
  - an arrowhead had no target (`.flow-edge` is `pointer-events:none`), so a
    rest opened nothing and a click fell through to the frame underneath
    (0 of 13 heads at the component level, 0 of 7 on the whole map).
- An open component is now the frame of anything pointed at or chosen in
  its own space; a closed one numbers nothing. Every drawn arrowhead where
  an arrow meets an area's or a component's border is its connection's
  handle, hit-tested on empty canvas from the drawn route (routes now name
  the boxes at their ends), nothing drawn: the incoming connection of the
  frame it points into, or the other end's outgoing one at a destination,
  the inputs or a loose part, or the numbered frame's chip standing there.
  Rest, safe triangle, card and click behave as the chip's; the card clears
  its handle (it had opened over the head), and a move from a head onto its
  own card or chip is not a leave. A closed frame takes the outline for the
  parts behind an end. A skeptic agent refuted the first design (invisible
  hit boxes on every border: overlapping, stealing titles and badges, a
  closed component taking the numbers); the pane-level head test replaced it.
- After: all 7 whole-map heads, 5 chips and 13 component heads open their
  card and keep it with the pointer on it, at 1440×900 and 1280×800; Core
  infrastructure's 3 chips and 2 crossing heads too; a click on a head or a
  chip opens that connection in the column with the camera still.
- Tests: two fixture specs (the component's chips across its space; heads at
  the whole map and inside a component, with the click) and
  `visual/real-report.spec.mjs`, which renders `REPOMAP_REAL_RUN` with the
  built binary and walks every head and chip (skipped without it); a
  placement test and a route-ends test. Each fails with the fix reverted
  (the real-report spec at the first whole-map head). No dumb test needed
  deleting.
- `make test`, `make vet`, npm test (122), `build --check` pass;
  `make ui-visual-test` fails the same 18 tests as main, 41 pass.

## 2026-09-27 — The owner's mockup picks: arrow card, "all", composition, connections, input path

- The owner chose from the designer's Redis mockups: 1b, 2a "but less
  ugly", 3a without line numbers and sorted, 3b, and 3c without line
  numbers and in the Inputs blue. Checked on the saved Redis run with
  `repomap render` (no provider call) and headless Chromium with real
  pointer moves at 1440×900 and 1280×800.
- **1b, arrow card:** headed from-frame → into-frame with one count line
  ("291 calls, 29 reads, from all 8 parts into 8 of 9 · 24 go the other
  way", the last a link to that card), calls under the part they come
  from, then the part they go into, both headings sticky, caller → callee
  in call-site order; kept open, a from → into index of parts with counts
  stands on top. Go marks the calls of a dispatch site with several
  retained alternatives and of a declaration handing a whole such set over
  (page_dispatch.go), so the card says "call → one of 94 · 77 here" and
  "cmdTable passes callback the same 94 as call · 77 here" in one line each
  with the callees by part under it. Cards are drawn at the screen's type
  size (291 calls had read at 10 px), take 300–500 px of the room beside
  their frame, and open on the whole end (crossing the number stack had
  narrowed the card to the last number crossed).
- **2a:** an end joining every numbered part of its frame is one "all" in
  the same chip; numbers stay otherwise. With its card open the parts
  behind the end are outlined in place and the rest recedes. A label now
  stands where its arrow meets its own frame: Data type commands' incoming
  labels had stood on Server runtime's border, unreachable by the pointer.
- **3a:** a part's reading starts "Made of 18 functions · redis.c", then
  every declaration, keys first and bold, the rest by name, no line
  numbers; an area's starts with its parts, each with its counts, files and
  declarations.
- **3b:** a click on an arrow end reads its frame in the column, scrolled
  to its Connections with that connection open, the camera still; a
  frame's Connections list its arrow ends with the card's own rows and
  replace the neighbour buttons.
- **3c:** an input's reading opens at its path: the chain from the entry
  to each dispatch site holding its handler, shared by the inputs it
  handles, folded into a box ("Shared by 95 inputs, through call"; one
  shortest static chain that avoids the handlers the site chooses between,
  or Redis's ran main → loadAppendOnlyFile → execCommand → call), then the
  handler's witness tree, a callee under its caller. Computed in Go
  (page_input_path.go). The reading's heading bar, kind and links take the
  Inputs blue.
- Tests: Go dispatch folds and input path, JS harness tests for the
  composition, the area composition, the arrow-end opening and the path
  section, node tests for the card arrangement and the end emphasis, and
  a Playwright spec for "all" and the arrow-end click; each fails without
  its change. Dumb tests updated: the unavailable-source list by position,
  the Russian legend line. edge-size's declaration-link test hovered during
  the camera move and failed alone on main too; it now waits for the camera.
- `make test`, `make vet`, npm test (120) and `build --check` pass;
  `make ui-visual-test` fails the same 18 tests as main.
- Left open: when no room outside the frame is ≥ 300 px the card still
  covers the parts it outlines; the shared chain is the shortest static
  one, which for Redis's call runs through beforeSleep, not the accept →
  read → processCommand path the designer drew.

## 2026-09-27 — Fixes from the naive-user test (canvas and reading column)

- Five goal-driven naive agents and a designer used the Redis report on a
  laptop with a mouse. Four of five reached their goal in 40–60 steps, by the
  reading column, not the map.
- **Canvas:**
  - Floating cards open on a 120 ms intent and stay reachable along a safe
    triangle. A click keeps a card; ✕, Escape or a click on empty canvas
    closes it.
  - Emphasis recedes what is not involved instead of veiling the subject; a
    pointed part is itself the subject.
  - A drag over tiles pans.
  - A tile click chooses its declaration, and pointing darkens only its
    arrows.
  - Tiles are sized to their longest name, in file order, with no "+N".
  - The inputs collection is headed Inputs and opens to its groups first.
  - Show whole map keeps the reading.
- **Reading column:**
  - A declaration has its own reading: who calls it and what it calls.
  - Callers come before callees, and repeated rows fold with their counts.
  - "To explanation", "To code" and "More details" are gone.
  - Find lists one entry per declaration.
  - One key sits above the map.
  - Relation words come from one vocabulary, and "possible" is muted.
- A second test with a primer, on the canvas alone, reached no goal fully (55
  steps on average) against a build that predates these fixes. The
  designer's mockups for the arrow card, the numbers and the column are the
  next step (owner: 1b, 2a but less ugly, 3a–3c without line numbers, an
  input's reading in the Inputs blue).
- `make test` and `make vet` pass after merging both branches; npm test 114.

## 2026-09-27 — metabase's Clojure target runs without a model

- `repomap --target clojure:deps.edn --no-model` on metabase exited 1 after
  about 8 minutes with "invalid external symbol authority". Three native rows
  broke the sealed graph, one after another:
  1. **Java calls with no method.** clj-kondo reports 178 class usages as
     calls with no method: 152 classes in an `:import` list, 4 constructors of
     an imported class inside a syntax-quote (`` `(ArrayList.) ``) and 22 with
     no position. The adapter made each a static call `Class/` with an empty
     name. Fix: a static call names its method. A native row whose name the
     index would refuse names no outside symbol, and its use stays
     unresolved; an import of it is skipped (`programindex.ValidName`).
  2. **Anonymous-argument calls.** A call of an anonymous function literal's
     argument, `#(% 1)`, is a local clj-kondo gives no name, so its pattern
     had an empty selector. It now keeps `%` as written.
  3. **Duplicate SQL facts.** `(str "SELECT 0 AS A" " UNION ALL" " SELECT 0 AS
     A")` gave two facts that read alike at one call, which the atlas refused
     as a place with two origins in one target. A statement is now one fact
     at its call as the fact reads it (a shared facts rule for all languages).
- The Clojure fixture gains `fresh-list`, `new-id`, `apply-each` and
  `zero-rows`. Each check fails on the old code with the metabase error; the
  facts unit test covers the SQL rule for every language.
- metabase: exit 0 in 6m53s (native analysis 3m21s, projection 1m22s); its
  tree is untouched. `make test` and `make vet` pass.

## 2026-09-27 — Neighbour branches merged: parameter cycles, definition-time calls

- **facts/parameter-cycle.** etcd's server module overflowed the stack in
  `parameterValue`: main exited 2, and this branch exits 0 with 196
  registrations, 44 of them with a holder.
  - Review fix: each search visits a parameter once, and reaching it again
    adds no value. A recursive helper that hands itself the same router
    therefore keeps the holder its outside caller gave it (the neighbour's
    fixtures had pinned "no holder").
  - The search is linear, not per path.
  - C and Clojure have no equivalent.
- **python/annotation-lambdas.** Lambdas in Python definition headers and
  store targets are declared.
- The owner delegated the neighbour's questions. Answer: a call belongs to the
  scope in which it runs, stated once in PROGRAM_INDEX.
  - A TS decorator's evaluated arguments and a parameter decorator now belong
    to the defining class (or the enclosing scope for a class decorator); the
    decoration itself stays the member's. This supersedes the 2026-09-26 entry
    above that kept a TS parameter decorator with its method.
  - Clojure metadata and attr-map calls, `defmulti` included, belong to the
    namespace.
  - Go and C have no equivalent.
  - Open: Clojure `(def x (f))` still gives its load-time call to the var.
- A no-model metabase Clojure run fails on main with "invalid external symbol
  authority"; its fix follows.

## 2026-09-27 — The map model's first steps: dead mechanisms out, C reads in

- Owner: "ты еще прослеживаешь логику того, что мы делаем? или мы уже какие-то
  адхок херни ... придумываем?". An audit wrote the model of the map: facts
  from code, then each model decision asked once, then code derivations,
  then the report only renders. It gave every mechanism a verdict.
- Deleted, since nothing reads them, with the Redis canvas byte- and
  pixel-identical before and after:
  - atlas_layers (A7);
  - the atlas trace (B15; atlas v14);
  - Box.Keys, modelKeys and rankedKeys (B16);
  - the canvas's one-part area folding (C4), which never fired. The visual
    fixtures that built one-part areas now make loose parts.
- C now emits `reads` for a function's use of a file-scope variable or table.
  - Not a read: the destination of `=`, sizeof or _Alignof, parameters and
    locals, platform variables.
  - Redis: findFuncName reads symsTable at 4 sites, so the lone "Debug
    symbols" part can attach to its reader.
  - Go has no reads emitter (recorded); Python, JS/TS and Clojure had
    coverage already.
  - Side effect: input path traces now also reach parts read as data (set: 11
    → 12 parts). Paths are revisited with the GET chain.
- A Redis warm rerun has 0 live calls, except orientation and glossary, whose
  requests carry the new connections. `make test` and `make vet` pass.

## 2026-09-27 — The owner's screenshots of the Redis map, clear fixes

The owner sent ten remarks on the Redis map. Read-only investigators
diagnosed each one; the clear ones are fixed here, and the rest wait for
him.
- **Text jumps (7–8):**
  - Cause: a rule from when a card was only a title added 18px under it
    whenever a number badge appeared. Descriptions were also pre-wrapped at
    228px in a 225px box, which left a lone "and".
  - Fix: the rule is gone. Text wraps at the card's real column, derived
    from its padding, border and zoom-button constants, and the browser
    wraps descriptions itself.
  - Check: 0 of 135 Redis cards move when the pointer enters an area or the
    area is chosen (7 moved before). A real-browser spec pins it.
- **Arrowheads (9):** an emphasised head was 7× its 2.5px line, 17.5px against
  10.5px. It is now the ordinary size on the map and in the deep view, and
  edge-size.spec measures it. That spec had been failing on main, since a
  click stopped entering areas.
- **Entry mark (5):** a 26×18 SVG arrow with a halo in the card colour, so the
  border stops at the mark. The legend draws the same glyph.
- **Deep view (10):** tiles are placed in the page's order, the model's keys
  first, each in its link column; a key is never hidden while a non-key is
  drawn.
  - repomap self-run: hidden keys 68 of 225 → 1.
  - Redis Data structures still shows no struct; putting types after keys
    waits for the owner.
- **The open DNS group (2):**
  - One heading under plain tiles in every state.
  - Tiles only as large as their calls: records 12.7% → 30% of the group.
  - Entering any tile frames the whole group.
- **Layout of mutual pairs from the entry side (1c):** measured at 14 window
  sizes, it helped at 4 and hurt at 8, so it was reverted. REPORT.md
  records the refusal.
- Waiting for the owner:
  - helper arrows (addReply/redisLog/refcount: 8 of Server runtime's 15
    two-headed lines);
  - the number chips (remove, or repair the misplaced reverse-direction chip
    covered by lines);
  - "Core infrastructure", whose nine parts Jev calls support;
  - types after keys in the deep view.
- `make test`, `make vet` and npm test (101) pass. The visual suite has
  main's failures minus the fixed edge-size spec.

## 2026-09-27 — Orientation evidence restored; parts a program never runs

- **Orientation had lost every member's evidence.**
  - Cause: since 59ba934b a place names its object qualified by its program
    (`t1.n4`), while orientation looked evidence up by the bare ID. No
    member found its place, so no real orientation request sent
    `member_evidence` from Sep 17 on (7 saved payloads checked).
  - Symptom: Redis's Main flow was guessed, loadServerConfig before
    initServerConfig.
  - Fix: one `atlas.ScopedObjectID`, and a declaration's calls listed in
    written order (line, then column) for orientation and question evidence;
    orientation prompt v7.
  - redis-server's Main flow now reads main → initServerConfig →
    loadServerConfig → aeMain → aeProcessEvents → readQueryFromClient →
    processCommand → call.
  - The Redis orientation request grows from 0.46 MB to 1.68 MB (about 430k
    input tokens). The packing ladder handles larger repositories.
  - Learn caches miss once.
- **The fallback start list** reads the entrypoint's own calls first, in
  written order.
- **A part whose every callable is proven unreachable in its program** leaves
  that program's map, the way a test-only part does (atlas v13, GroupsIndex
  v16, reason `unreachable`). Its declarations are listed under "Not
  reachable from the entrypoints", and Find opens them as code.
  - redis-cli loses "Linked list": adlist.c/.h, 13 functions and their types.
  - The same part stays on a program that runs it.
  - Only the C adapter proves unreachability; the other languages record the
    missing equivalent.
- **The overview loose-part scale was reverted:** the bigger closed box left
  an empty 1036×739 card once the areas opened. REPORT.md records the
  measurement.
- Redis warm rerun: 6 s, 0 live calls; render is byte-identical. `make test`,
  `make vet`, npm test and `build --check` pass.

## 2026-09-27 — The Redis first screen, polished

- The first screen had three defects:
  - titles were cut ("DNS resolve…", "TCP endpoin", a lone ")");
  - three identical "DNS resolver" tiles stood side by side;
  - redis-server's input box broke its title before ")".
- Map titles now break only between words, and a closing bracket stays with
  its word.
  - A word wider than its line shrinks the heading instead of being broken.
  - The camera fit measures the same boxes it frames, and a summary short of
    its room is drawn whole, scaled down.
- When every frame of a display group spells its destination the same, the
  group says it once, where no arrow enters, over small plain tiles. Each
  tile keeps its own title, reading and program's arrow; an opened tile is
  named again.
  - Differently spelled destinations keep their own headings (constitution:
    "equal destination labels do not prove identity").
- `make test`, `make vet`, npm test (95) and `build --check` pass. The visual
  suite has the 20 failures that main already has.

## 2026-09-27 — Redis journey, round 2: outside roles, C reachability, the report

- **Outside-symbol roles:** the atlas_api questions are closed Jev choices
  through `llm.Categorizer` (contract api.v6).
  - `state.task` says what the map wants; each question holds the symbol row
    as `outside_symbol`; every option, `none` included, carries criteria.
  - Unhanded symbols get one `talks` question: serves, client_request, db,
    queue_producer, queue_consumer, sdk, none.
  - Measured with 3 draws per variant. Wrong answers / flipped symbols:

    | Variant | Redis | xk6-dns | microblog |
    |---|---|---|---|
    | old DeepSeek cells | 7/1 | 18/12 | 9/2 |
    | DeepSeek with the same criteria | 2/1 | 0/0 | 12/0 |
    | Jev | 0/0 | 3/2 | 6/2 |

    Every Jev flip is an answer against an explicit unknown.
  - Redis: accept, bind and listen serve; inet_aton, fopen and sigaction are
    none; gethostbyname stays DNS. The libc and socket outside boxes are gone.
  - A known miss: k6 metrics.PushIfNotDone answers sdk.
- **C reachability per program:** a function is reached only by a direct call
  or through its address, so a linked function that no chain from main or
  from an address-taken function reaches is unreachable in that program.
  - The proof is withheld where a linker script, an asm label or unread code
    could run a function by name.
  - Unreachable declarations' boundaries are not that program's; they are
    listed under "Not reachable from the entrypoints".
  - redis-cli and redis-benchmark no longer listen or accept.
  - Other languages record the missing equivalent.
- **Report:**
  - Outside destinations are never folded by label: each program keeps its
    own tile, and same-kind tiles share a display frame.
  - Only the area holding main gets the entry mark.
  - A focused area fits the canvas, with one route per pair.
  - Choosing an input outlines its path's parts and darkens its edges.
  - A loose part matches its peers when zoomed in.
  - Fallback sentences break ties by source order (getCommand, setCommand,
    setnxCommand).
  - A layout candidate ELK cannot place is left out instead of failing the
    whole map.
- `make test` (53 packages) and `make vet` pass; web unit tests 89/89. Redis
  cold 29.6 s, warm 4.1 s with 0 live calls.
- A third proxy walk found the cli → server and benchmark → server arrows
  drawn solid with an unrelated call as evidence ("Command line client calls
  Dynamic strings"). Two page identities omitted the owning target:
  - another target's group node was `<section>-foreign-<gN>`, so two peers'
    g15 shared one node;
  - a connection was keyed by its target-local `x*` ID, so the benchmark's own
    x17 took the cli link's peer.
  Both now carry the target, and the first screen draws exactly the two
  dashed connect → accept links.

## 2026-09-27 — The Redis reading journey, after an owner-proxy walk

- An owner-proxy walked the Redis report as preparation for the owner's
  reading session. Its verdict: "путь GET — херня: event loop никуда не
  ведёт, SET вообще нет на карте, а Server runtime — лесенка из марок".
  Three reviewed tracks answered it.
- **Arrows:**
  - A call left open with stored-candidate witnesses now reaches each
    witnessed declaration as the existing dashed possible arrow. C, Go and
    Python witnesses carry an object identity.
  - Redis Event loop → Client connections (acceptHandler,
    readQueryFromClient, sendReplyToClient) and → serverCron.
  - Sentences name each callee once, and the handlers stored behind a
    function value instead of the field (`proc`).
- **Inputs and outgoing calls:**
  - One table row is one registration, so zunion and zinter are no longer
    doubled.
  - `http_client` became `client_request`: a socket connect is no longer
    "HTTP", and an unstated method is not shown as GET.
- **SET on the map:** the Jev assignment item carries how the code calls the
  declaration, and an undecided row is asked again with the boxes its
  neighbours landed in. GET, SET and APPEND land in String commands in 3 of 3
  fresh namings. An undecided declaration stays findable in Find, and an
  input whose handler is undecided names no part (GroupsIndex v15).
- **Report:**
  - An input names its handler instead of the raw registrar string (287 → 0)
    and opens where its path starts. Tiles are grouped by the handler's part,
    without a repeated kind word.
  - Areas are laid out from their own arrows, and a focused node is centred
    in the visible canvas.
  - The small programs' part arrows are drawn.
  - The code list comes before connections, with fields inside their type.
  - The entry wins over core for an area's mark.
  - Find no longer lists outside calls as parts.
  - A glossary term lists only the files it is written in.
- The constitution's HTTP-only entry lines now name entries by their written
  words.
- Redis: cold 31 s; warm 3.7 s with 0 live calls. `make test` and `make vet`
  pass.
- The second proxy walk ("Event loop теперь ведёт к acceptHandler …, SET на
  месте — красиво") found:
  - libc and socket drawn as outside systems (inet_aton as sdk, accept as a
    client request);
  - listener, accept and inet_aton calls attached to redis-cli and
    redis-benchmark, which never reach them;
  - loose parts at the wrong scale;
  - outside boxes folded by destination text against the constitution;
  - the GET path still not emphasised when the input is chosen.

## 2026-09-27 — A file in several boxes (the role split)

- Owner, option "в": "what's in one file can have different roles, and one
  role can span different files." Beside the parts request, each placed file
  goes through three steps:
  - a Jev gate: one box or several;
  - DeepSeek naming of the boxes. It sends lines of code, same-file
    `called_by` and `callers_elsewhere`, and the helper rule; the helper probe
    measured judge 2.8 → 3.5, and W+F without the rule was worse;
  - a Jev assignment of each declaration.
  - No size threshold and no box cap. Jev questions name their item (`file`,
    `declaration`) and carry criteria per option. Request-local refs keep each
    file's cache its own.
- New facts and rules:
  - `code_lines` comes from every adapter (graph v17).
  - A name declared twice in one file is one unit.
  - A split file has no endpoint: the entry comes from the part holding the
    seed declaration, and undecided declarations go off the map as
    `undecided` (atlas v12, GroupsIndex v14).
- The gate was measured on 377 files. The review replaced criteria that split
  redis-cli.c, redis-benchmark.c and repomap's render.go; these now stay
  whole. Near the cut stay pykrx etx/wrap.py and bond/core.py and repomap
  llm/api.go.
- redis-server: 9 parts → 28–32 in 5–6 areas, with redis.c split into about
  24 boxes (commands per data type, persistence, replication, virtual memory,
  clients, the command table). 7–9 of its 339 declarations stay undecided.
  - Redis cold run 38.8 s; warm 3.9 s with 0 live calls.
  - The new requests cost about $0.024 per cold run: 34 gate, 3 naming and
    11 assignment calls.
- pykrx: 10 → 46–48 parts.
- The one-time describe goldens (fixture bytes at f49c3304) were deleted
  after merging main: every fixture addition broke them.
- Open for the owner:
  - one-declaration helper boxes (redis "Logging" = redisLog, "Pattern
    matching" = stringmatchlen);
  - the same domain once per code layer in pykrx, with two identical titles;
  - a parts-answer part keeps its title after losing a split file;
  - repomap's design.go still splits, since its methods live on a type
    declared in reading.go;
  - redis-server draws about 200 arrows among about 30 parts.

## 2026-09-27 — C adapter, entries named by their written words, one request in the air

- **C adapter:** repomap reads C (`internal/cproject`, contract
  [C](../contracts/C.md)).
  - clang's JSON AST is streamed.
  - The programs come from a `make -n -B -w` dry run and its linker closure.
  - A function-pointer store is a witness: a call through it is unresolved and
    names the stored candidates.
  - A command-table row `{"get", getCommand, …}` is a registration (owner
    decision D1).
  - Redis 1.3.6 gives four programs: redis-server, redis-benchmark,
    redis-check-dump and redis-cli.
- **Entries:** the owner said an entry must not be HTTP-shaped ("там есть
  GRPC, UDP, TCP, WEBSOCKET … из одного на поверхности должно лепиться при
  помощи моделей").
  - A registration carries the words its call wrote, as written. The model
    names the entry by choosing among them, and code restores the choice
    verbatim.
  - `http_server` became `request` (any protocol), and `continuous` joined
    `binds`.
  - A command row's registrar is its record field (`redis.c.redisCommand.proc`).
  - The "hands a callable" flag had been true for every registration inside a
    declaration since f4878514, so fopen/open were asked what they bind. It is
    now true only when a callable or a repository value is handed. The
    handed-value case keeps xk6-dns's `Register("k6/x/dns", new(DNS))`.
  - redis-server inputs went from 2, both wrong (segvHandler as an
    interaction, IOThreadEntryPoint as a request), to 98: 97 command requests
    named `get`, `set`, … and IOThreadEntryPoint as continuous.
  - Still open for the owner: aeCreateFileEvent(acceptHandler) and
    aeCreateTimeEvent(serverCron) hand callables to Redis's own event loop, and
    a call inside the repository is not a registration.
  - The model classes `vm_preload_proc` as a request too, so zunion and zinter
    appear twice.
- **Stored callbacks in Go and Python follow the C rule:** a store under a
  branch leaves the call unresolved, with the stored candidates as witnesses.
  - Go interface fields: real no-model comparisons found 32 relations in caddy
    and 5 in etcd moving from exact or alternatives to unresolved-with-witnesses,
    including the nil-default `if h.Transport == nil`.
  - Go review fix: 27 caddy calls through a field of an outside interface had
    lost every candidate; they are witnesses again.
  - Python: a branched name, or an attribute of one, names what it may hold, and
    an `if` condition or the first operand of `and`/`or` counts as
    unconditional.
- **Determinism:**
  - Before: redis-server and redis-benchmark both draw zmalloc.c, ae.c, sds.c,
    anet.c and adlist.c, so five byte-identical describe requests went live
    2–3 times at once. Four came back with different sentences, and the cache
    kept whichever landed last.
    - A warm rerun then built different core, keys, areas, orientation and
      glossary requests: 815 changed report.json values.
  - The fix: the executor now asks an exact request that is already in the air
    once, and its twins read that answer.
  - Redis: cold 27–38 s; the warm rerun takes 3 s with 0 live calls in every
    stage, and its report.json differs only in `timing`.
    - Checked over 8 warm reruns at GOMAXPROCS 1, 2, 16 and the default.
  - `cache clear` exits 0.
- `make test` (53 packages) and `make vet` pass.

## 2026-09-26 — Lambdas in Python definition headers

- Closes the gap below: a lambda in a parameter or return annotation
  (`def endpoint(q: Annotated[int, Depends(lambda: 1)])`), in a `def`/`class`
  type-parameter bound or default, or in a lambda's own default
  (`lambda rows, key=lambda row: row: …`) failed the whole target with
  `python program index: <id>`. The declaration pass now visits exactly the
  header expressions the relation pass reads, in the defining scope. A
  42-position probe finds no remaining asymmetry that fails a target.
  Freqtrade `--no-model` is unchanged (exit 0, 39,468 objects, 57,452
  relations); no local repository writes either form.
- Fixtures: `level_limit` and `row_sorter` (models.py) and `Checked` /
  `first_checked` (generic_types.py); removing any of the three declaration
  visits fails an expectation with the target-wide error. Go has no
  equivalent: headers and constraints hold only types and there are no
  default parameters. JS/TS (parameter defaults; TS parameter decorators, so
  the fixture's tsconfig enables `experimentalDecorators`) and Clojure
  (parameter `:or` defaults, `:pre`, an fn's own default) already held and
  gained examples. They evaluate those per call, so the function owns them;
  a TS parameter decorator also stays with its method although it runs once.
- Open: `first, *rest = items` declares no `rest`, and neither pass reads a
  starred store target's index; Clojure metadata/attr-map calls run at load
  time but belong to the var.

## 2026-09-26 — Lambdas inside Python store targets

- `repomap ~/git/freqtrade --no-model --target python:.:script:freqtrade`
  failed the whole target with `python program index: 4635785104`: a
  `KeyError` on `id(node)`. `freqtrade/templates/FreqaiExampleStrategy.py:263`
  writes `df.loc[reduce(lambda x, y: x & y, conditions), "exit_long"] = 1`;
  the relation pass read the target's receiver and index, but the declaration
  pass never visited them, so the lambda had no scope. The declaration pass
  now reads the same parts of `=`, annotated and `for` targets. The same run
  completes (exit 0, 1/1 analyzed, 39,468 objects).
- Fixtures: Python `mark_exit_rows` (models.py) declares and passes three
  lambdas; it fails on the old parser with the Freqtrade error. The native
  equivalents already held and gained regression examples: Go `markExitRows`
  (SSA closures `$1`–`$4`, exact callbacks), JS/TS `markMatchingRows` in
  server.ts and market-worker.js (single tree walk; an inline arrow argument
  stays unresolved as elsewhere, a named callable is passed and read), Clojure
  `mark-handled!` and `handled-or-default` (`set!` target and `:or` default
  relate like an ordinary read).
- Not fixed here: a lambda in a parameter or return annotation
  (`def f(q: Annotated[int, Depends(lambda: 1)])`) still fails the target the
  same way; the declaration pass skips annotations the relation pass visits.

## 2026-09-26 — Table consilium fixes and the alias for non-English names

- A consilium on the table protocol with a 66-call DeepSeek probe (about
  $0.10): the row format is sound (5,099 answers, 79,363 rows, 0 invented
  keys, 0 output-cap hits), while "leave the cell empty" lets the model flip
  a whole window (atlas_api round 2: 228 of 228 rows filled in one run, 0 of
  192 in the next; 227 of those cells were in columns no code read).
- Owner's answers: the api table asks only what boundaries read (binds,
  middleware, publishes, talks; contract v4); the types, symbols, targets and
  joints prompts fill only the cells `fill` advertises (types had the model
  write an unasked alias in 82 of 82 rows); keys and core get the whole
  declaration list (it was cut to 12); a symbol row writes identical calls
  once with their lines (canvas.spec.mjs: 84 KB request → 41 KB; all 5,103
  function rows −14.5%).
- The English alias (for non-English identifiers, e.g. Korean) is asked,
  by code, only of a name with a letter outside the Latin script, with or
  without `--captions`; English names never get the cell. Before, a default
  run asked no alias at all and a captions run asked it of every name.

## 2026-09-26 — Closed decisions through one categorizer, JEV_KEY required

- Owner: "давай пока с JEV_KEY обязательным, но это должен быть типа такой
  интерфейс, что категоризирует". The key declarations, part roles and keys
  (`Definition.Classifier`) go only to `llm.Categorizer`: a Provider with
  `Prompt` (the request for keyed closed questions) and `Verdicts` (the
  answers by key), so the executor still caches, journals and gates it. Jev
  is its only implementation. The DeepSeek fallback is gone; on keys it had
  answered "yes" to 13 of 13.
- `JEV_KEY` is required by a model run (checked after the flags, before any
  artifact, corpus or model call) and by `read`; `--no-model`, `replay`,
  `conf`, `render` and `cache clear` need none. The binary without the key
  exits 1 within 30 ms and creates no runs directory.
- The Jev request bytes and cache keys are unchanged: a full-bytes golden
  taken from main passes on the branch; on the two-target reading fixture
  main and the branch send byte-identical symbol-selection, keys and core
  requests, and the branch reading over main's cache made no Jev or text
  call; saved real `atlas_core` (47,251 bytes) and `atlas_symbols` (84,524
  bytes) requests rebuild byte-identical. Ordinary online acceptance (a warm
  run with no live Jev call, then `cache clear`) is still to be run.

## 2026-09-26 — The owner's short answers, implemented

- Jev choices: the absolute 0.50 floor became a 0.10 lead over the runner-up
  ("none of these" counts as a rival). Chosen from 60 saved role answers
  (leads 0.00, 0.02, then 0.17 and up) and 1,072 eleven-option answers: it
  decides 101 answers the floor refused (0.50 vs 0.34) and leaves uncertain 9
  it took (0.51 vs 0.49). "support" 0.49 vs 0.32 is now taken.
- DeepSeek: an HTTP-200 answer without content, or with finish
  insufficient_system_resource, gets one transport retry; its first answer's
  usage is summed. The console names it "empty answer".
- An unknown or missing default-target answer no longer ends the run: the
  default is recorded unresolved and the report is published; no code picks
  one.
- Glossary lookup: any letter case and a plural -s/-es, whole words, in both
  the occurrence check and the underlines; a native code declaration's name
  matches only as written and an acronym's plural only in lower case (an
  all-folded lookup took a self-run from 132 to 404 code-name underlines, 240
  of them offering several definitions, and let HTTP open inside HTTPS).
- The map legend says what the number chips on parts and frames are; areas
  and a component's area list follow the order of the areas answer (the
  GroupsIndex no longer re-sorts containers by value).

## 2026-09-26 — Refused answers stay in their run, out of the cache

- Owner: refused answers are for the developer to debug, "сохранять, но не
  в кеш". Before, every journal and table ref pointed into
  `.llm-cache/payloads`, so `cache clear` erased the bodies of refused
  answers. A refused exchange (decoder, envelope, provider failure,
  cancellation) now keeps its prompt, input, request and response as the
  run's own content-addressed copy in `<run>/payloads`; so does an accepted
  answer with any part refused or annotated, which is most refusals since the
  decoders refuse at the smallest scope. Its cache record is unchanged; a
  wholly accepted answer links only the store. Refused answers are still never
  cached or reused.
- Real run at df0ecda2 on a checkout named `repomap`: cold exit 0 in 56.2 s,
  warm exit 0 in 144.8 s, `cache clear` (46 MB). After clear every
  `rejected.jsonl` ref of both runs leads to bytes (30 rows; 2 provider
  refusals of the oversized symbols window have no response to keep); on the
  previous build 44 of 46 were broken. The slow warm run: the cold run's
  refused symbol rows are asked again by design, that changed the glossary's
  input, and the live glossary draw looped to its 32,768-token allowance
  (97 s, refused).
- Left: the journal entry of a zones/areas answer the reader annotates after
  acceptance still links the store (its window refs, which the rows name, are
  in the run); `--no-model` and `--no-cache` still write store payloads no
  record owns.

## 2026-09-26 — Decoders refuse only what is wrong; the resample is gone

- Owner: "валидаторы и строгие декодеры нам уже 30 дней палки в колеса
  вставляют". A read-only audit listed 200 refusal rules on model output and
  the refusals in local run logs of the last 30 days; a skeptic checked each
  against the code: 100 relaxed, 18 deleted, 77 kept (they stop an unknown
  ref, an invented value or a missing decision), 5 left to the owner. Five
  worktree chunks, each adversarially reviewed and fixed, then merged.
- The one-time identical-bytes resample is deleted. It never fired (0 second
  draws in 13,704 journals); the refusals it was built for came from rules
  since removed. A parts or areas answer refused whole or cut at the output
  cap is that run's map failure or no areas, asked again only by a new run.
- Measured harm fixed, highest first: one extra field or bad default in the
  target portfolio ended the whole run (now discarded; a refused
  classification batch leaves its candidates standalone or unclassified); a
  newline in a type caption failed atlas validation and ended the run; one bad
  (question, row) threw away the whole question (8 questions, 471 slots),
  now only that cell; 12 answers with a doubled `}` were refused, now the
  closer is deleted when both readings agree; one bad translation entry
  refused its whole window (17 windows); a Learn intent was lost to one
  duplicate review or a missing reason; 128 documentation concepts were cut by
  a 12-per-document cap.
- Also: a table cell refuses only itself (a missing `open` reads as open, so
  descendants are still asked); bare-array and wrapped rows, list and boolean
  cells, choice punctuation and "None." read as the table's forms; an
  identical repeated row, parts group or area is one answer; a string of refs
  is a list; orientation keeps target-qualified subject refs (a two-target
  summary no longer ends the run) and a role with an empty purpose; glossary
  groups refuse alone; a cached answer the current decoder refuses is kept
  for a later decoder, not evicted; invalid transport measurements are
  clamped and the answer kept. The glossary reads table prose through the
  same row reader.
- `make test`, `make vet`, `make build` pass on the merged branch.
- Ordinary run, `.bin/repomap .` at f819f4be on a checkout named `repomap`,
  own cold cache: exit 0 in 108.0 s (atlas ready at 48.8 s; an enumerating
  glossary draw took 48 s of it, 15 s the run before). CLI: 53 parts in 8
  areas, 2 loose; UI: 5 drawn parts and 1 test-only; no map failure, 0 data
  rows. Rejected: 4 `atlas_core` rows under Jev's floor, the oversized
  `atlas_symbols` window, 3 guidance file refs the model invented
  (discarded), 43 glossary terms without an occurrence, 4 glossary code
  names. Warm rerun exit 0 in 25.1 s (only that Jev window live);
  `cache clear` removed 47 MB. Playwright walk without page errors.

## 2026-09-25 — Ordinary acceptance of the map of parts

- `.bin/repomap .` (b0cca08d) on a checkout named `repomap`, own cold cache:
  exit 0, 68.9 s; atlas ready at 42.9 s, glossary 52.3 → 67.3 s. CLI: 61
  parts in 11 areas, 1 loose; UI: 4 drawn parts, 3 test-only parts off the
  canvas. 0 data rows. Rejected: 8 `atlas_core` rows under Jev's 0.50 floor,
  the one oversized `atlas_symbols` window, 2 glossary names, 1 orientation
  flow row.
- Warm rerun: exit 0, 22.2 s; only the refused `atlas_symbols` window went
  live. `cache clear --debug-dir` removed the 46 MB cache.
- The previous run's draw on the same request gave 36 parts in 8 areas.

## 2026-09-25 — Test SQL is no program's data; the map says what its boxes are

- Repomap's own Data section listed 111 tables and 180 SQL texts, and both
  programs showed the same 291 rows: all 295 data places came from `_test.go`
  files (189) and `testdata/` (106). Places gave an extraction no program's
  file holds to every target whose root holds it, and the root `.` holds
  everything. Such an extraction in a code file (`facts.IsSourceFile`) or
  under a tooling directory (`corpus.ToolingPath`, the one list the target
  portfolio already used) now belongs to no program; a schema or migration no
  adapter reads keeps the root rule. The new places test failed without it.
- Canvas: a part's one-sentence description stands under its name (three
  lines at most); a closed area's line fills the whole lines its box leaves;
  a loose part beside areas is as tall as a peer, so its title reads at an
  area's size (it was about 5 px). Looking at an area or a component darkens
  the arrows that cross its border only; before, zooming into repomap's CLI
  selected it and turned all 25 of its area arrows dark.
- The component card named DeepSeek's and TypeSafe's calls "Client.Do" twice;
  a call in a destination's frame reads `DeepSeek · Client.Do`. A component's
  overview lists its areas before its loose parts. A symbol row whose model
  wrote the same answer twice was refused; an identical repeat is one answer.
- An owner-proxy review of the walk (Playwright, 1600x1000) found these; its
  "wait" items are recorded in CURRENT, not done.

## 2026-09-25 — Map of parts review: resample, empty answers, method-only files

- The llm layer's one resample (branch `parts/one-time-resample`, merged into
  this branch) now serves the map of parts: the parts and areas calls opt in
  and the `TODO(parts resample)` markers are gone. A parts answer that decodes
  but draws no part is refused whole in its decoder, so it is resampled and
  then recorded as `window_rejected`, like an answer without groups: every
  ref unknown (paths instead of `f*`), every group without a name, or every
  file in two groups. Before, such an answer gave zero parts, no
  `map_failure` and every file `left_out`. An areas answer whose areas hold
  no listed part is refused the same way; an empty list is still an
  abstention.
- A file that is no row (it declares only methods of types in other files)
  was marked `no_units`, and the card listed it under Not on the map as "No
  declarations of its own" while its methods were drawn. Now `no_units` is
  only a file that declares nothing; a row-less file is on the map through
  its declarations, with the one part they share as its endpoint, and has no
  entry. Declarations off the map in a file a part holds carry that part as
  `box_id` (atlas 11, not yet released, so no version change); GroupsIndex
  and the card no longer list such a file. On the saved no-model
  `cmd/repomap` graph read with every row as its own part: 290 parts hold all
  300 files and nothing is off the map; the 10 row-less files (among them
  `internal/atlas/reading/walk.go`) were `no_units` before.
- Tests: `TestRefusedPartsAnswerIsAskedOnceMore` (spec D4 on the reading
  side, replacing the resample branch's test of the removed proposal code:
  refused then good draws the map, refused twice is a `map_failure`, exactly
  two parts requests, the other target asked once);
  `TestRefusedPartsAnswerIsAnExplicitMapFailure` now expects two draws per
  target; decoder cases for answers that draw nothing (parts and areas);
  `TestFilesOfMethodsDeclaredElsewhereStayOnTheMap` (a methods-only file and
  its boundary stay in the type's part, a method of a type off the map names
  its file's part); the shared fixture checker refuses a file-level off-map
  entry for a file that declares code, and the Go fixture checks that
  `ledger_append.go` is not in GroupsIndex's `off_map`, the card's list. The
  summed metrics of two draws stay covered in `internal/llm/resample_test.go`.
  Each new expectation fails on the code before the fix (checked by
  mutation). No online run was made for this round.

## 2026-09-25 — Map of parts from one file split per target

- Probe evidence behind the change (spec `parts-spec.md`, measured before the
  code): placement judged on 80 units (60 Go, 20 UI) — file split (FILES)
  78/80 and 58/60, directory split 75 and 55, names plus Jev with no
  threshold 67 and 54, the product of the day 51 and 42. FILES beats the
  product 28:1 (p=1.1e-7); against names plus Jev the all-unit 12:1
  (p=0.0034) is the UI alone (7:0), on Go 5:1 (p=0.22, not significant);
  against the directory split 5:2 (p=0.45). Go draw stability, unit ARI over
  10 pairs: median 0.929, mean 0.897, minimum 0.804 (three draws passing the
  old rules: median 0.817). 3 of 5 Go draws passed the old rules (go-1 listed
  f180 twice, go-2 left out f12); all 3 UI draws did. Parts came out at
  package level (30 of go-4's 37 parts one directory). Descriptions from
  members (v1) beat the proposal `about` 23:4 on U+A (p=0.00031).
- Implemented: `atlas_zones` sends one parts request per target over the
  unit-bearing files (sealed `f*`, path, units, type/function/variable names,
  exported signatures, exact file calls once per call site and file pair,
  file imports); validation keeps every good file and sends left-out and
  conflicting files to one `atlas_placement` closed-choice table; each drawn
  part not made only of test code gets an `atlas_describe` request with the
  v1 prompt; `atlas_areas` splits three or more described parts, beside the
  keys, and describes each area from its parts. Windows are directory
  subtrees, only on an actual prepared-size or input/context refusal, through
  the shared split memo. Atlas 11 and GroupsIndex 13 carry the off-map record
  (`left_out`, `conflict`, `no_units`, `map_failure`; `tests` in GroupsIndex)
  and `map_failure`; operations in files off the map have no group; empty
  part/area summaries are the no-description state. Removed: the Jev
  `atlas_zone_parts`/`atlas_zone_areas` tables and prompts, the proposal
  catalogue and its title → purpose decoder, the file inventory boxes, the
  model's `tests` role (core v5). Python visibility now follows a module's
  literal `__all__` (fixture `src/fixture_app/exports.py`).
- Lexical children are found by source range: a declaration inside a
  function or method of its file (adapter positions and end lines). Go
  closures carry no native parent (their container is the package), so the
  range is the one rule for every language.
- Fixtures: Go `internal/localstore/ledger.go` + `ledger_append.go` (a method
  declared outside its type's file); Python `exports.py`. Equivalents: Python
  and TypeScript have no method outside its class; Clojure `defmethod`,
  `extend-type` and `extend-protocol` are not declarations its adapter
  projects (recorded, not fabricated). Replay fixture
  `internal/atlas/reading/testdata/parts-replay` holds the saved go-1, go-2
  and go-4 answers mapped to accept2's sealed refs: go-1 sends only
  internal/repoconfig/config.go (f185 sealed) to the follow-up, go-2 only
  internal/atlas/destinations/destinations.go, go-4 nothing; none refused.
- No-model self-run of `cmd/repomap` (exit 0, 18 s wall): 0 parts, 0 file
  boxes, 300/300 files off the map as `map_failure` (4,962 declarations,
  9 boundaries without a box); the card says "The map of parts is
  unavailable: no model was asked for the parts of this program" and lists
  Not on the map · 300 (five rows, All 300).
- Not run for this change: the spec's gate A (product-bytes parts draws,
  placement re-judge, gallery repositories, a >1,000-file window, description
  and areas judges), B (python-tutorial-game dogfood) and C (ordinary online
  self-run, warm run, cache clear, browser walkthrough), so there are no
  per-stage online timings yet. The one-time resample of a refused parts or
  areas answer was not wired in this entry; merging `parts/one-time-resample`
  wires it: the parts and areas calls opt in.

## 2026-09-25 — One resample of a whole refusal (parts spec §10)

- `llm.Call.Resample` opts a call in. `ExecuteJSON` wraps `executeJSON`, so
  replay, which calls `executeLive` itself, is never resampled. It asks once
  more with the same bytes in three cases. The owner's decoder refused the
  whole answer. The provider's answer was empty, had an undecodable envelope,
  or did not stop (a content filter excepted). Or the answer was cut at the
  output-token cap: a loop, asked again before any adaptive owner could split
  the item into windows (spec §2 windows only on envelope or input/context
  refusals). Transport classes, context or request-size refusals, other
  statuses, cached answers and owner-recovered refusals
  (`SplitRejectedResponse`, `SplitHTTP500`, attempt deadline) keep one call.
- A refused draw is never cached; the accepted second draw is cached under
  the same key with its own measurements; `Outcome.Metrics` sums both draws.
  The journal already told the draws apart: each exchange gets its own
  `instance_ordinal`, so no draw number was added.
- The design parts and areas proposals opt in. A parts answer with no groups
  is now a whole refusal (spec §10 "no groups"), and an empty areas answer
  stays an abstention. A target without units no longer sends a parts request
  (spec §2), which would otherwise draw that refusal twice.
- Tests (fail-closed local providers):
  - The llm layer covers five first-draw refusal classes and two refused
    draws (exactly two provider calls, summed metrics, nothing cached).
    Eleven classes keep one call. An output-token refusal of an item its
    adaptive owner could split is asked again whole, not split (each and
    batch forms).
    A cached answer is served with no call, and a stale one is replaced by
    exactly one call. Replay makes one call.
  - A debugdump test writes the two exchanges with one request SHA-256 and
    distinct `instance_ordinal`s.
  - The reading tests check that a parts answer with no groups followed by a
    good one draws the map after two parts requests. Two empty answers leave
    the declarations loose after two. A target without units sends none.
  - Mutations that always resample or drop the zero-unit skip each fail
    these tests.
  - `make test` and `make vet` pass, and `make build` builds. No online run
    was made for this piece.

## 2026-09-25 — Test sources from the Node runner and Playwright

- The JS/TS helper now derives `TestSources` from three runner facts. The
  first is the globs that package scripts pass to `node --test`; the
  arguments end at a shell operator whether or not it is spaced
  (`node --test t/*.mjs; node build.mjs` no longer marks `build.mjs`). The
  second is a literal Playwright config when the manifest declares
  `@playwright/test`: the config itself, every source under an explicit
  `testDir`, `testMatch` minus `testIgnore` with the default directory,
  reporter modules, and the sources a `webServer` command names unless the
  manifest also names them as a package entry point (module fields,
  `exports`, `bin`, `dev`/`start`). The third is the existing Vitest config,
  which now also counts itself.
- The `webServer` rule is the spec's default for the open question on test
  harness files, with the entry-point exception added so an application
  server the checks also start stays drawn. The owner has not confirmed it
  and no skeptic has reviewed it; JSTS.md says so.
- Measured with a no-model run of `jsts:internal/report/web/package.json`,
  this branch rebased on main 0d78db85 (exit 0): 27 of its 41 JavaScript
  sources are test sources. The saved self-run flagged 0. They are the
  eleven `*.test.mjs` files, the Playwright config and all 15 sources under
  `visual/`, including the `webServer` harness `visual/server.mjs`. The
  production modules and `build.mjs` stay unflagged.
- The cumulative JS/TS fixture gained `packages/canvas-ui`, which mirrors
  that UI and adds a stub API that only `webServer` starts. Its test checks
  the fixture and four contrasts: the manifest without the runner, the
  application server without its `start` script, a non-literal `testDir`,
  and the default directory with `testIgnore`. A table test pins which
  `node --test` arguments are globs, attached operators included. The JS/TS
  inventory test's file-count pin was removed; materializing the fixture
  already checks the exact inventory.
- Equivalents: pytest `conftest.py` under a resolved pytest table is now test
  code (cumulative `tests/conftest.py`). Pytest `testpaths`, unittest
  discovery and Clojure runner test directories are recorded as not derived
  in PYTHON.md and CLOJURE.md. Go `_test.go` was already native.

## 2026-09-25 — Generic declarations keep their type parameters

- Go `typeSignature` began the short form at the first space, which for a
  generic type lies inside its parameters: the saved self-run `places.json`
  showed `llm.Call` as `"any] struct{SplitRejectedResponse bool; …}"`, fields
  and a struct tag included. It now skips the bracketed list by depth:
  `[T any] struct`, `[K comparable, V map[string]int] struct`,
  `[T interface{~int | ~string}] interface`, `[T storefixture.Labeled[int]] []T`.
  With the old function the new cumulative expectation failed on all four
  (`any] struct{Items []T "json:\"items\""; …}`, `interface` without its
  parameters). Generic functions already kept theirs (`func[T any](items []T) T`).
  A no-model `cmd/repomap` run with the fixed binary (exit 0) wrote `Call
  [T any] struct`, `AdaptiveEachResult [Item, Value any] struct` and
  `DecodeValidate [T any] func([]byte) (T, error)`; no type signature in its
  `places.json` holds `struct{`.
- Equivalents, each asserted on the file's places declarations
  (`adaptertest.AssertDeclarationSignatures`): TypeScript `export class
  Box<T extends { id: string }>`, `type Pair<T>` (compiler rendering `Pair<T>`)
  and `firstOf<T>` (`<T>(items: T[]): T`) were already correct and gained
  regression examples in `src/type-members.ts`. Python kept `class
  Box(Generic[T])` but dropped PEP 695 parameters (`class Crate`, `class
  Keyed(Box[V])`, `first(items: list[T]) -> T`); the parser now writes them
  (`class Crate[T]`, `class Keyed[K: str, V: (int, str)](Box[V])`,
  `first[T](items: list[T]) -> T`). The cumulative Python fixture therefore
  needs Python 3.12+ (checked with 3.14.3). Clojure has no type parameters: no
  equivalent. Open: a Python `type Pair[T] = …` statement is not indexed at all.
- New fixture files `go/internal/storefixture/generic_types.go` and
  `python/src/fixture_app/generic_types.py`; both inventories renumbered.
- Review follow-ups at integration: the Go type header is written from go/types
  (`typeHeader` at capture) instead of a bracket scanner over the printed
  type; a no-model `cmd/repomap` run gave the same 1,007 type signatures as the
  scanner. A TypeScript generic alias is
  written from its source parameters, `Keyed<K extends string, V = number>`
  where the compiler rendered `Keyed<K, V>` (the expectation failed without
  it).

## 2026-09-25 — Critical-path work before and after the models

- Render (saved self-run, byte-identical HTML on two saved runs): 6.2 s →
  4.4 s. Group edges and connections indexed once per target instead of per
  card; a display text's source-syntax regexes run once per page, not twice
  per text; structural edges sorted by precomputed keys.
- Symbol selection answered by 29.6 s but handed over at 32.4 s: 4,800 row
  memos were written one after another. They are written by one worker per
  processor (handover 28.8 s in the next cold run).
- Program index reads: `Decode` parsed the 24 MB Go index four times through
  `Index.UnmarshalJSON` (0.68 → 0.43 s per read); places and the group
  projection no longer revalidate what `ReadFile` validated; the TODO facts
  skip files and lines without a marker word; route values encode each
  expression once. No-model `cmd/repomap` snapshot run 18.2 → 16.0 s, every
  artifact equal apart from run IDs and timing.
- Side by side, each on its own view joined in step order: the targets'
  zones (compact IDs still taken in target order; proposals are rounds 2p+1
  and 2p+2), the functions and types selection rounds, the two atlas_api
  rounds, and after the core the keys beside targets → joints. Warm runs of
  old and new binaries over one snapshot cache: atlas, places, knowledge,
  tables.md, groups, orientation and glossary byte-identical, rejected.jsonl
  the same rows with the reading's own in order.
- TypeScript helper memoizes its two path functions: 4.4 → 3.2 s on the
  report UI project, output byte-identical.
- The guidance classifier starts before the 1.0 s Go planning snapshot:
  it needs only the repository name, now `snapshot.RepositoryName`, the
  derivation BuildContext uses; a run refuses a name mismatch.
- Also: the projection's declaration-key lookup reads the targets during the
  atlas (Atlas groups 2.2 → 1.3 s); a TypeScript result is validated with one
  encoding and three fewer times (report UI lane 7.9 → 6.8 s no-model).
- `.bin/repomap .` cold, three consecutive runs: 58.2 / 57.2 / 66.2 s to report
  server ready; the glossary starts at 46–47 s in each (60.0 s before this
  entry, 70 s total then).
- Probes on saved requests (not changed, owner's call): the full16 parts
  request looped to the cap in 3/8 draws; "Propose 8 to 20 parts" stopped
  all 8 (20–29 parts) but raised the report UI's 6–12 to 13–20;
  frequency_penalty 0.3 cut loops to 1/8 (a skeptic found my first count
  missed duplicate titles). The full16 glossary request drew 36–618 terms
  (6–69 s, 4/6 above 240); telling the model the reader is a programmer who
  needs no general programming vocabulary drew 66–106 terms (10–16 s, 6/6),
  against the approved line naming JSON and HTTP as concepts.
- Measured, not changed: the model's enumerating mode dominates the tail.
  Cold self-runs drew a 168-part design proposal (usual 30–38; zone
  assignment then took 492 Jev requests / 11.5 M tokens, hit Jev's 429 and
  25 s), glossary outputs of 6–17k tokens (23–49 s), and an atlas_api
  window that put a cell on every one of 334 rows (3,745 output tokens,
  12.7 s; 130 "validates"). A "list only rows with a cell" instruction did
  not change the output in 18 paired draws (every draw echoed all rows).
  The DeepSeek gate wait was 0.5–3.5 s for the requests that queued.

## 2026-09-25 — Full no-target self-run: 184 s → 70 s cold

- Cold `repomap .` on repomap (two targets after testdata left the plan),
  report server ready: 184 s (first run that completed) → 125 s (render
  index, testdata, classifier fixes) → 74 s (atlas branches, run lanes,
  report assembled during the orientation, served page rendered alongside)
  → 70 s (64 concurrent decision requests instead of 24; no 429).
- Remaining serial tail after the atlas (~44 s): groups 3 s, orientation
  10 s, glossary 11–15 s. Inside the atlas the longest branch ends with the
  type descriptions (~9 s of DeepSeek behind the four-call gate).

## 2026-09-25 — Part roles on Jev

- `atlas_core` opts in to the decision model. On repomap's 50 saved parts Jev
  chose DeepSeek's role for 45; the five others were borderline (two UI
  presentation parts named interface instead of domain). The role is one
  closed choice per part, so the serial tail loses one DeepSeek round.

## 2026-09-25 — Part assignment asks Jev a direct question

- Unit rows carry their file. For Jev, zone_parts and zone_areas ask a direct
  question and leave the text-model table prompt out of the state
  (`ClassifierOmitTask`, `Column.Ask`). Re-asking cmd/repomap's 62 saved
  windows: uncertain units 1,046 → 800 of 4,238, `none` 68 → 8, assigned
  3,124 → 3,430. A blind judge over 60 units the two versions placed in
  different parts preferred the new placement 34 to 21 (5 ties).

## 2026-09-25 — A part's keys are ranked by Jev

- `atlas_keys` opts in to the decision model as a ranking (`Ranked`): each
  candidate's probability of explaining its part is kept, and readKeys shows
  the five highest; a flat ranking (top minus sixth < 0.1) keeps the
  selection's order. DeepSeek had answered yes to 89% of candidates, so file
  order picked its five.
- Same 26 saved parts of `cmd/repomap`: a blind judge over the 125
  declarations where the two differed called 45 of Jev's 65 picks key (69%)
  and 7 of DeepSeek's 60 (12%). The stage drops from ~8 s of DeepSeek calls
  to one Jev round.

## 2026-09-25 — Design proposals have a measured output allowance

- A parts proposal on `cmd/repomap` generated 128,000 tokens for 5m44s (a
  run of 9,761 entries over 163 distinct titles walking `internal/facts`
  names) and failed; the run took 7m27s instead of ~2 min. Resending the
  saved request 6 times answered in 525–922 tokens every time: a rare sample
  (~1 in 10 on this target), not the request. Parts and areas proposals now
  request at most 8,192 output tokens (~8× the largest accepted answer), so a
  loop ends in ~22 s as an ordinary refusal. A proposed "at most 40 parts"
  prompt line was measured and rejected: it pushed 3 of 4 answers to 41–55
  parts. An identical-bytes resample after such a refusal awaits the owner.
## 2026-09-25 — The atlas reads symbols, boundaries and zones at once

- After the files, three chains of the walk read nothing of one another:
  symbols; atlas_api → boundaries (+publish) → layers; zones. They run at
  once on views of the reader with their own tables/rejected/uses sinks,
  joined in step order before arrows → core → keys → targets → joints, which
  stay serial. The boundaries chain works on its own copy of the places;
  knowledge and the response caches sit behind one lock. Symbols now choose
  the described declarations from a per-file inventory (`overviewKeys`)
  instead of projecting every target through `r.target`; a temporary probe
  compared both on every reading, contract and run fixture (79 selections,
  48 non-empty, all equal). The first failure cancels the other chains and
  is the one reported; `--through` stops inside them.
- Also: the places graph is sealed once for places.json and
  reading-input.json (byte-identical); knowledge.json and tables.md are
  rewritten only when they changed; independent rows build their inputs,
  load memos and recall answers on all processors, taken in row order.
- Two fixes the equivalence check needed: a holder with two publishing calls
  took the first address in map order (now boundary order), and
  zone_parts/zone_areas used round 1 for every target, so the second target
  overwrote the first target's window files and repeated its tables.md
  headings (now the target's position, as core).
- Identity check, base 4aed3fd1 against c2892d79. No-model runs on the
  snapshot (cmd/repomap; cmd/repomap + internal/report/web; the Go fixture):
  every artifact equal apart from timing and build info (396, 335 and 32
  files). Online, the same cache after base warm runs converged: every stage
  cached except the one symbols window the provider refuses each time
  (context_tokens), in both; places.json, reading-input.json, knowledge.json,
  atlas.json, report.json and report.html equal; tables.md and the 310 zone
  window files equal once the second target's round 2 is read as round 1;
  rejected.jsonl the same 1,930 rows, the reading's own 962 in order, the
  journaled response rejections interleaved.
- Measured on the ordinary no-target run of the repository snapshot (two
  targets, report server ready), on the same machine with other agents'
  runs beside it. Cold (fresh cache), base 213.7 / 158.5 s, new 93.8 /
  160.4 s; the glossary alone took 102 / 39 s and 14 / 93 s, so compare the
  atlas: reading (directories → Atlas) 55.5 / 64.9 s → 30.3 / 19.5 s, of which
  symbols..zones 53.1 / 49.8 s → 19.0 / 17.6 s; the prelude before the first
  table 2.3 / 2.1 s → 0.9 / 1.0 s. Warm: reading 7.7 → 2.9 s, places → Atlas
  13.5 → 7.9 s, server ready 57.3 → 53.6 s. The critical chain is symbols
  (17.2–18.8 s, 19.6 s serial) with zones close behind (17.4–17.7 s); the
  Jev zone windows and the DeepSeek api/boundaries windows beside it did not
  slow its selection (10.4 / 8.0 s vs 11.0 / 8.7 s serial) or its types
  (8.4 / 9.3 s vs 8.6 / 10.9 s). The serial tail can still cost 11 s when keys
  asks (24 windows, 8 s in new cold 1); keys and targets → joints read
  nothing of each other, the next candidate for a fork.
## 2026-09-25 — Independent run stages side by side (measured)

- Done outside the atlas, from the skeptic-reviewed plan: the documentation
  reduction runs beside the first target's native analysis; claims beside
  facts, with the extractors and tracked-path listing started before the
  target pages; one lane per adapter for target pages (one adapter stays
  serial; outcomes folded in plan order; a stopping failure cancels the other
  lane and the lowest-position non-cancellation cause is reported); the
  report is assembled while the orientation is asked; the served page is
  rendered beside `report.html`. `programindex.Persist` no longer decodes
  what it just encoded, and `report.ProgramPortfolio` lookups no longer
  revalidate every index (validation at construction and at the
  publication/render boundaries only).
- `go test -race` first proved the served render unsafe: `collectOpenablePaths`
  reused the caller's `OpenablePaths` backing array through `PreparePage`'s
  shallow copy (data race, then "openable paths must be uniquely sorted").
  Fixed with a fresh slice before the served render was made concurrent.
- Byte identity: no-model artifact chains (go fixture 1 and 3 targets,
  `cmd/repomap`, `cmd/repomap` + report UI, python-tutorial-game backend +
  front) of every commit equal the parent 4aed3fd1 over a fixed export of
  4aed3fd1, after removing run ids, timing, build identity and the report
  stamp. Online, parent and branch warm runs from one cache have identical
  program indexes, facts, claims, `places.json`, `reading-input.json`,
  first-layer exchanges (classifier, portfolio, documentation) and
  atlas_symbols/api/publish exchanges; later stages diverge from the first
  window re-asked live (refused windows are never cached), as two parent runs
  do.
- Online `repomap` over that export (2 targets: `cmd/repomap`, report UI),
  served, to "report server ready": fresh cache 124.5 s -> 101.6 s; the same
  cache again 100.7 s -> 85.5 s. Fresh-cache segments: documentation + target
  pages 5.1-27.1 -> 5.0-15.1 s; facts + claims 3.1 -> 2.3 s; publication
  before the glossary 2.6 s -> 0; server 4466 ms -> 0 ms. Atlas (48.4 vs
  54.1 s), orientation (13.7 vs 8.3 s) and glossary vary with live answers.
- No-model served run, three alternating runs each: 47.0/48.6/42.7 s ->
  35.2/30.4/34.7 s. CPU profile: ProgramIndex validation 7.61 -> 5.70 s,
  publication 6.43 -> 5.17 s. Peak process-tree RSS (repomap + node) online
  957 -> 1010 MB fresh, 961 -> 1008 MB warm; no-model 1104/1079 ->
  1067/959 MB. The machine was shared with another agent's runs throughout.
- 60 s cold is not reached: the atlas (~50 s), orientation (~10 s) and
  glossary (~10 s) remain serial on the critical path. The atlas fork,
  in-memory indexes, a two-phase glossary and the DeepSeek gate need their
  own change or the owner.

## 2026-09-25 — Each call keeps its own source position

- An all-targets self-run failed with `atlas: boundary "b601" has conflicting
  native origins for target "t17"` (`internal/report/web`). The JS/TS helper
  placed every call at the start of its callee expression, so all calls of a
  chain shared the leftmost receiver's position: `p.split('/')…join('/')` on
  `build.mjs:19` gave split and join one anchor (19:108) and one place, and
  join's holder named its own position. The same start dropped data: in
  `consumer.on(t, first).on(t, second)` the registration once-gate folded
  `second` into `first`.
- A call now sits at the called member's name, the called name, or else the
  `(` of its arguments. The call record, its pattern and the `call_result`
  anchor of its value share that position. Python (attribute name), Go (SSA
  `Lparen`) and Clojure (clj-kondo form) already did this. The boundary key
  also carries the call word and external symbol, so targets merge only when
  they saw the same call. The skeptic-reviewed ordinal pairing of identical
  calls was dropped: facts never reach it, and it would pair calls across
  targets by fact order. Two facts of one target in one place are still
  refused.
- Probe, no model, `--target jsts:internal/report/web/package.json`: the
  HEAD binary exits 1 with the same refusal (`b1`, `t1`); the fixed binary
  exits 0 with split at 19:110, split at 19:131 and join at 19:166 (holder
  19:110) as three places. The online all-targets rerun and browser
  walkthrough are still to do.
- Cumulative fixtures, each read as two targets sharing the file: TypeScript
  `chainedPlatformCalls` and `registerChainedOrderConsumers` (before: one
  split at 77:20 and split/join both at 76:22; `recordOrder` lost), Python
  `subscribe_chained`/`chained_text_calls`, Go `registerStrippedFiles`
  (Handle and two nested StripPrefix calls with one path), Clojure
  `chained-paths`/`nested-paths`. Go's standard library has no fluent
  registration chain; Clojure Java instance chains carry no call pattern.
  Python call-result objects still sit at the call expression's start (two at
  `events.py:64:5`); patterns and anchors are distinct.

## 2026-09-25 — atlas_api stays with the text model (measured)

- Paired probe on `cmd/repomap`: the same 485 atlas_api rows answered by
  DeepSeek (live) and Jev (optional yes-only columns as Nouls). Whole rows
  agreed on 405. Nearly every difference was a Jev yes the row does not
  support (`validates` on `go/types.Identical`, `token.IsExported`;
  `reads_input: body` on `bytes.Split`, `go/parser.ParseFile`), and Jev
  missed the one serving call, `net/http.Server` publishes. The table is not
  opted in to the decision model.

## 2026-09-25 — testdata is test input, not a target

- Owner decision: a `testdata` directory holds the inputs of the tests
  around it and is never a target, like `.claude`, `.github` and `.vscode`.
  Its files leave the guidance classifier's candidate tree and the target
  portfolio. repomap's own no-target run had offered 22 fixture modules as
  targets beside its one program.

## 2026-09-25 — Protected names are found through an index

- Preparing a page's display texts scanned every protected name (paths,
  declarations, addresses) for every text: 38.7 s of a 104 s no-model
  `cmd/repomap` run, twice per run (publication and the report server). A
  name is protected only where it stands whole, so each of its runs of name
  runes is a whole run of the text; names are indexed by their longest run
  once per page. Saved live report re-rendered: 28.8 s → 11.4 s, HTML
  byte-identical.
- A display text's glossary terms were rebuilt and re-sorted for every text;
  they depend only on the question scope and the text's own term and are
  built once per pair. Same report: 11.4 s → 8.1 s, byte-identical.

## 2026-09-25 — Jev by explicit opt-in, and zone catalogues accepted whole

- A table reaches the decision model only when it opts in
  (`Definition.Classifier`): zone_parts, zone_areas and key-symbol
  selection. The row memo basis names the answering provider, and a
  remembered Jev answer is recalled through the Jev decoder.
- Jev sees each part by title and purpose instead of a bare `c*` ref. An
  uncertain answer leaves the row explicitly unanswered instead of reading
  as absent; yes/no questions have an uncertain band around 0.5.
- Key-symbol selection on Jev uses a 0.8 yes cutoff: three blind judges over
  60 rows where Jev and DeepSeek disagreed sided with Jev at 0.8 on 50 and
  with DeepSeek on 32 (Jev at 0.6: 28). Both models over-mark helpers.
- A live DeepSeek parts proposal named 13 of 14 groups `group` instead of
  `title`, and the decoder silently kept the one titled group: 3,789 of
  3,881 units then fell to `none`. Proposals are now a title → purpose
  object read in order and refused whole when an entry is empty or
  repeated; the parts and areas prompts are separate and short.
## 2026-09-25 — SQL admission keeps statements with run-time tables

- Review of the statement-structure change found real SQL it dropped: tables
  filled in by printf verbs (golang-migrate `INSERT INTO %s ...`, `DROP TABLE
  %s`; maddy `UPDATE %s SET %s = $2`; casdoor `CREATE SCHEMA %s;`), 23 sqlc
  end-to-end queries (`SELECT [NOT] EXISTS (...)`, MySQL `UPDATE ... JOIN ...
  SET`, `UPDATE a, b SET`, `DELETE t FROM t JOIN u`), `INSERT IGNORE`,
  `SELECT @@server_id` and `CREATE EXTENSION/MATERIALIZED VIEW/TYPE`, `ALTER
  TABLE ONLY/MODIFY/ENABLE`. `sqltext` now reads an object name pieced from
  identifiers, template holes and printf verbs; such a table is not listed and
  makes an extracted query partial. A `:` after the object name keeps
  `create table %s: %w` a message; SET must start an assignment.
- `sqltext.Tables` skipped three tokens after every IF, so `DROP TABLE IF
  EXISTS jobs CASCADE` listed `CASCADE` and `ALTER TABLE IF EXISTS users ADD`
  listed `ADD`. It now skips `IF [NOT] EXISTS`, `ONLY`, `LATERAL`, clause words
  and `ON DUPLICATE KEY UPDATE`.
- Measured with a throwaway scan of Go string literals (call arguments and
  constants) in go-corpus, casdoor, kubernetes, moby, syncthing, restic, etcd,
  headscale, soft-serve, caddy and repomap: 86 statements regained, all real
  SQL; 8 newly refused, all messages (`create table t: %v`, `select: empty
  target list`, `create table with nil name`). Python literals from django,
  freqtrade and beets: 84 regained, two describe() messages (`Create index %s
  on %s on model %s`) admitted, none lost. repomap's own non-test sources still
  admit none. Unadmitted: `CREATE/ALTER USER`, Cypher/CQL, `SELECT FROM t`.
- The Go and Clojure fixtures hand `DROP TABLE IF EXISTS %s` to their format
  call and expect a `sql_query` fact with no table (both fail with the previous
  admission); Python `%` and a TypeScript template add the partial source
  statement the extractor keeps.

## 2026-09-25 — SQL facts need statement structure; DeepSeek is a destination

- A self-run of `cmd/repomap` had 13 `sql_query` facts and all were false:
  `facts` took any non-owned call's literal starting with an SQL verb
  (`fmt.Errorf("create %s dir: %w")`, `strings.EqualFold(text, "with")`,
  flag help "create a standalone report with GitLab source links"). As fixed
  `db` boundaries they drew outside tiles named fmt, strings, stderr, "local
  filesystem", GitHub and GitLab and gave parts `reaches: db`.
- The SQL lexer, statement admission and table mentions moved from
  `internal/extractors` to `internal/sqltext`; the facts pass now uses that
  admission and `sqltext.Tables` instead of its own verb regex (table names
  keep their written case; WITH names are no longer tables). MERGE INTO and
  REPLACE INTO joined the admission and the extractor's statement prefilter.
  Of the 28 string literals handed to calls in repomap's non-test Go sources
  that the old regex took, the shared admission takes none.
- Each cumulative fixture (Go, Python, TypeScript, Clojure) hands one
  "create %s dir" message to a call outside the repository beside a statement
  that stays a fact; `adaptertest.AssertSQLQueryFacts` requires the exact
  statement set and that the message really reaches such a call. All four
  fail with the old regex. Clojure gained its first positive
  (`(query! "SELECT id FROM direct_rows")`).
- The destination catalogue lists DeepSeek before OpenAI, so the one remote
  call (`api.deepseek.com`, an OpenAI-compatible API) is no longer offered
  only OpenAI; refs after Prometheus shift by one.

## 2026-09-25 — Jev answers closed tables

- `internal/typesafe` is an `llm.Provider` for TypeSafe System One
  (`jev-1.13.0`, `JEV_KEY`). A table whose columns are all unconditional
  closed choices (`table.Closed`) goes to it when the key is set; each row
  and column is one question, the table prompt and window context are the
  state. Optional yes-only columns are Nouls; other optional choices carry
  an explicit `none of these`. A choice is taken when its option's
  probability is above 0.5 (not Jev's `confidence`, which measures the
  whole distribution: a yes at 0.69 has confidence 0.39).
- Packing counts questions (150 per request): 150 rows of seven columns
  exceeded Jev's 64k-token request. 24 requests run at once on their own
  gate, over HTTP/1.1: on one HTTP/2 connection 57 concurrent 100 KB
  requests took 42 s, on separate connections 9 s.
- The semantic journal now records atlas_api, atlas_publish, atlas_layers,
  atlas_core, atlas_keys, atlas_zone_parts and atlas_zone_areas.
- Live `cmd/repomap`, cold: 2m19s against 3m14s DeepSeek-only, no failed
  request. Known and open: zone_parts options are bare `c*` refs and a third
  of units stay unassigned; an uncertain optional answer reads as absent;
  routing is by table shape, not a calibrated allowlist; the row memo
  basis does not include the provider.

## 2026-09-24 — Zones are proposed, then assigned by closed choice

- Units are functions and types (with their methods). `atlas_zones` (design
  v3) only proposes parts and areas; `atlas_zone_parts` and
  `atlas_zone_areas` are ordinary closed-choice tables that assign units to
  parts and parts to areas.
- Live `cmd/repomap`, cold: 3,857 units in 19 part batches (2m04s provider
  time), 15 parts in 8 areas, no refusals, whole run 3m14s. Go analysis and
  the other language adapters now share one part; the single-shot areas
  prompt had split them and grown to 31–34 areas when told not to.

## 2026-09-24 — Zones group packages and types, not every declaration

- `atlas_zones` (design v2) sends units a reader already sees: a package with
  its free declarations and each type with its methods, their first doc
  sentence, declaration names and calls to other units. Outside calls,
  their arguments and every non-README/AGENTS document are gone. The
  model can still split a package along its types or join units across
  directories.
- Live `cmd/repomap`, cold cache: first zones request 4.5 MB → 0.4 MB,
  zones 7 calls / 7m53s provider time (one 5-minute repetition loop) →
  2 calls / 19 s; whole run 9m46s → 3m17s, provider time 14m38s → 6m30s,
  no model request failed. 54 parts in 15 areas.

## 2026-09-24 — Key declarations answer in their own column

- `atlas_keys` named its decision column `key`, the name every table row
  already answers its identity under; the model returned `{"key":"s1"}` and
  all 35 windows of a live `cmd/repomap` run were refused. The column is
  `explains` (`repomap.atlas.keys.v2`) and `table.Request` refuses a column
  named `key`.

## 2026-09-24 — Registration values stop at recursive field reads

- Reading a registration argument's value keyed its cycle guard on the
  expression plus the accumulated field path; a method reaching itself
  through a field of its own receiver grew that path forever. On
  `cmd/repomap` Facts reached ~96 GB and macOS killed the run (exit 137).
  The guard is now the expression alone. Same target, `--no-model`: Facts
  2.2 s, 1,249 facts, peak heap ~350 MB, whole run 5m21s.
- The Go index spent 287 s of that in dynamic handoffs: every interface
  parameter re-sorted all SSA functions (string keys built inside the
  comparator) and scanned every instruction for calls to its function. The
  order is now computed once and static calls are indexed once by callee.
  Index 287 s → 7.3 s; program-index, facts, places and atlas byte-identical
  to the previous binary on the same checkout.

## 2026-09-18 — A handed value is a constructed instance or a module

- `handed` on a registration now means an instance the repository built
  (`new(DNS)`, `&T{}`) or a module was handed over — not any value a
  repository call returned. `c.Set("my_user_model", user)` is no longer an
  entry through a `binds` role on `Context.Set`.

## 2026-09-17 — Routes under groups, one tile per symbol, one operation per handler

- Mount prefixes follow the holder chain: `v1.Group("/articles")` names the
  place a route registered on that group stands under, and a router
  received as a parameter is the value its one repository caller passes
  (`ArticlesRegister(v1.Group("/articles"))`). Gin RealWorld: `GET
  /api/articles/:slug` instead of `GET /:slug`; `POST("", h)` on a group
  answers on the group's path instead of a nameless `POST`.
- A registration records `handed` (a value of the repository's own handed
  over); only such a registration becomes an entry through `binds` without
  a callable, so `Group("/articles")` no longer is one.
- An outside system's tiles fold by symbol (`DB.Close ×2`), and the same
  handler registered twice under one name (`GET("")` and `GET("/")`) is one
  operation. Hovering the component frame lights nothing, like selecting
  the whole component.

## 2026-09-17 — Initialization arrows on the map

- A map arrow every relation of which is initialization (`map-edge-init`)
  is drawn dashed (short dashes, distinct from a possible arrow) and only
  while a box or an area at one of its ends is what the reader looks at —
  hovered, or selected as a part or area. Selecting the whole component
  looks at nothing in particular and draws the runtime arrows alone.
  Gin RealWorld: 5 init arrows out of 124 leave the default view.

## 2026-09-17 — Initialization and runtime phases

- Every subject and connection of the overlay carries a phase, derived and
  never persisted: `init` — reached from the target's seeds by ordinary
  calls, the wiring before anything serves; `runtime` — an operation's
  subject or on one of its chains; `both`. Echo: `main`, `NewUsers`,
  `handler.New` are init, `Handler.GetUser` … `Queries.GetUser` runtime.
  The map can now fold init arrows into one bootstrap and draw runtime
  flows on their own.

## 2026-09-17 — An entry has a named kind

- `binds` offers `http_server`, `queue_consumer`, `scheduled`, `interaction`,
  `extension`, `command` — no `other`: on NestJS and FastAPI the model had
  put every marker decorator (`Injectable`, `Column`, `ApiOperation`,
  `field_validator`, `pytest.fixture`) there and each became an entry. A
  parameter decorator (`@Body() dto`) is a call the method makes, not a
  decoration of the method. NestJS RealWorld: 26 request operations with
  verbs and paths; FastAPI template: 24 requests and the typer command.

## 2026-09-17 — Decorators in JS/TS, annotated receivers in Python

- A TypeScript decorator call (`@Get(':slug')`) is a `decorates` relation
  from the decorated declaration to the decorator's symbol, with a
  `decorator_call` pattern — NestJS, Angular and TypeORM registrations
  become visible. A Python call on a name bound to a value of an outside
  class — `session.exec(...)` on an annotated parameter, `consumer.subscribe`
  on `consumer = KafkaConsumer()` — invokes that class's method as an
  external symbol instead of staying unresolved.

## 2026-09-17 — The reading simplified after a prompt review

- The operations table is gone: an operation is a bound entry (a callable
  handed to a `binds` symbol) or a command; the model no longer judges
  "entry self/none" over a listing of calls. Its 150-line prompt and the
  `operation_candidate` cell went with it.
- `api` is two tables: symbols handed a callable answer `binds`,
  `middleware`, `publishes`; the others `publishes`, `talks`,
  `reads_input`, `writes_output`, `auth`, `config`, `validates`. `talks` is
  only the call that itself crosses: a builder returning its receiver type,
  a header set, a pool tuned talk to nothing. Rows carry `symbol`,
  `declared`, `usage`, `literals`, `hands_callable`; `word`, `sites` and
  `beside` are gone. The prompt is a legend plus two rules; each cell is
  defined once, in its note, without framework names.
- `test` is the code's: places mark a target's test sources on files, and
  a symbol only test files call is not asked about. `access` is the
  code's: the declaration that makes the outgoing call; the model chooses
  among `adapter`, `logic`, `passthrough` from source alone, `before`/`after`
  are gone. A "no"/"none"/"" in an optional cell reads as absence.

## 2026-09-17 — Seven more cells on the api table

- `middleware`, `reads_input` (body/path/query/header), `writes_output`,
  `auth` (verifies/issues/hashes), `config` (reads/loads), `validates`,
  `test` — all optional, all properties of the symbol, recorded on
  `atlas.api` (contract v2). A middleware or testing symbol binds no entry
  and publishes nothing; a testing symbol talks to nothing. On the four real
  repos the model placed them where a reader would: gin's `Context.Param →
  path`, `ShouldBindWith → body + validates`, `RouterGroup.Use → middleware`,
  jwt `ParseWithClaims → verifies`, bcrypt `GenerateFromPassword → hashes`,
  `os.Getenv → reads`; microblog's `load_dotenv → loads`, wtforms
  validators; Express's `expressjwt → middleware + verifies`, jest and
  falso → test. One miss: `String.split → reads_input: header`.

## 2026-09-17 — JS/TS: every declaration the checker resolves names its symbol

- A call the TypeScript checker resolves to a declaration in `node_modules`
  is an external invocation of that symbol whatever the declaration is: a
  class method, an interface method or call signature (`res.json`, jest's
  `test`), a function-typed property, an ambient function, a call signature
  behind a type alias (`NextFunction`). The owner named above it is the
  receiver. Only class methods counted before. The default library stays
  the JavaScript platform. Express RealWorld: unresolved calls 279 → 100,
  external invocations 80 → 259, `Application.listen` now publishes.

## 2026-09-17 — A handed value is one the repository made

- `fmt.Printf("…%v\n", id, err)` was a registration because `err`, a call
  result, counted as a value handed over. A produced value is now a record
  the repository built (`new(DNS)`) or the result of a repository call that
  carries a repository type; what a library returned is the library's.
  The facts validator no longer refuses newlines in a path, a value or a
  symbol text (only NUL and invalid UTF-8), and literals are kept as
  written — the one-line squash is gone. gin-realworld: 41 registrations
  (router verbs and groups) where noise was; xk6-dns: 5.

## 2026-09-17 — Four real repositories through the first two steps

- Registration literals and addresses are one line (`strings.Fields`), so a
  `Printf("…\n")` argument no longer fails the facts validator and the run
  (gin-realworld). The "Repository snapshot" console block (tracked-file
  listing, platform choice, `--force-platform` hints) prints nothing; the
  work stays. Roles observed: gin (34 of 81 symbols: router verbs, `Run`
  publishes, gorm `db`), Express+Prisma (7 of 43: `Router.*`,
  `PrismaClient`), microblog (28 of 100), xk6-dns (4 of 47).

## 2026-09-17 — Glasses for the model: declared signatures, usage lines, source

- An `api` row carries `declared` (the external symbol's type as its package
  declares it — Go, from the type checker; `ExternalCallTarget.Signature`,
  external call index v10) and `usage` (one repository line that calls it).
  An `operations` row carries the declaration's source instead of a listing
  of its calls. Effects on microblog: the model no longer marks the app's
  own model methods reached through `current_user` as database calls; it
  now proposes alembic `upgrade`/`downgrade` as commands (20 rows of noise
  the operations table accepts on source alone).

## 2026-09-17 — Two real repositories: xk6-dns and microblog

- `new(T)`/`&T{}` in Go is a produced value (an empty record), so
  `modules.Register("k6/x/dns", new(DNS))` is a registration; a value handed
  to a `binds` symbol is an entry without a named callable (`extension
  k6/x/dns`). A local variable handed to a call (`fmt.Errorf("…", err)`) is
  no longer "the repository's own value": only module-level values count.
- The symbol behind a method on a value of an outside type carries the call
  word: `flask.Blueprint.route`, not `flask.Blueprint`; an imported
  module-level receiver (`from app.api import bp`) resolves to its
  declaration. Registration values are the composed address alone
  (`/auth/login`, not `/auth/login, /login`). Outgoing calls of kind `other`
  survive without a basis. microblog: 33 request operations with mounted
  paths, three commands; xk6-dns: the extension and its two exports.

## 2026-09-17 — Width: the Python and JS/TS fixtures through the whole reading

- A registration records `owner_id`, the declaration making the call; an
  outgoing boundary without a handed callable belongs to it, so a client
  call (`axios.post` in `runLevel`) is a place a chain can end and a layers
  row can be read. The declaration that starts a listener is not an entry.
- Accepted activations (`interaction`, `scheduled` from the operations
  table) are chain entries beside bound callables; a caption-less operation
  is named by its declaration and needs no summary. JS/TS live: nine
  interaction/scheduled operations, three `http_client` outbound calls,
  layers read `logic`/`access`.

## 2026-09-17 — Publishing sites are listeners everywhere; Python live

- A call to a `publishes` symbol is the listener at every site, registration
  or not (`uvicorn.run("app.app:app", host=…, port=…)` has no address literal
  and no followed holder, so `atlas_publish` asks; DeepSeek picks the
  FastAPI holder and the three routes become request operations). The
  listener's address is the value that reads as one (host:port, :port, URL,
  socket path), never a module path. Every operation kind the boundary
  kinds map onto validates; the api prompt says a transforming decorator
  (`dataclass`, `lru_cache`, `property`) binds nothing.

## 2026-09-17 — Typed signatures in Python and JavaScript/TypeScript

- Python: a function's parameters (without `self`/`cls`) and its return
  carry the annotation as written and the repository class it names once
  `list[X]`/`Optional[X]` are opened; resolved in the relation pass, after
  every class is declared. JS/TS: the checker's type text for each
  parameter and the return, with the declaration it names once an array or
  a promise is opened; a destructured parameter has no name. A value may
  lack type text (an unannotated parameter) but never both name and type.
  Clojure stays untyped.

## 2026-09-17 — The first table that reads code: layers

- New reading stage `atlas_layers`: from every bound entry the reader walks
  the graph's calls to every declaration that makes an outgoing call and
  shows the model each declaration on the way as source (numbered lines
  from its first to its last line, the middle elided beyond 60), with the
  declarations before and after it. One decision per row: `access`,
  `adapter`, `logic` or `passthrough`. The role lives on the atlas symbol
  and the overlay subject. `reading.Options.ReadSource` supplies file
  bytes; the run reads them from the corpus.

## 2026-09-17 — Declarations know where they end; questions are opt-in

- Every adapter records a declaration's `end_line` (Go from the AST node's
  end, Python from `end_lineno`, JS/TS from the node's end position, Clojure
  from clj-kondo's `end-row`), so a declaration's source can be read whole.
- The question cascade (`atlas_learn`, `atlas_question`, `atlas_answer`)
  runs only with `--learn` or an explicit `--question`; `--no-questions` is
  gone. Effective options record `learn`.

## 2026-09-17 — Typed signatures and what an operation exchanges

- ProgramIndex v18: callables carry `parameters` and `results` with the
  repository type each value carries; the Go adapter fills them from the
  core object index (v7, `TypedName` per signature value). Signatures stay
  short text for reading.
- GroupsIndex derives, never persists: per operation the repository types
  its subject takes (`RequestTypeIDs`) and hands to calls outside the
  repository (`ResponseTypeIDs`: the value written into a response, found
  through the call result that produced it); per chain the repository types
  its signatures carry. Echo: `GET /users/:id` responds with `UserResponse`;
  its chain to `users` carries `model.User` and `sqlc.User`.

## 2026-09-17 — Chains from an operation to the system it reaches

- GroupsIndex gains `chains`: from each operation's subject, exact and
  alternative calls are walked in reading order (depth 8) and every path
  ending at a subject with an outbound call is one chain (operation,
  subjects in order, outbound). Chains are derived, recomputed on hydrate,
  never persisted. Echo: `GET /users/:id` → `Handler.GetUser → Service.GetUser
  → Postgres.GetByID → Queries.GetUser` → the `SELECT … FROM users` call.
- Outbound calls carry `data_ids`: the extracted tables their values name.
  Repository-level entities (migrations, schema) now belong to the targets
  whose root holds them, or to every target of the run, so echo's `users`
  table reaches the atlas and the overlay (`data` was null before).

## 2026-09-17 — Construction of an outside type is a registration

- The Go adapter projects a callable bound into a field of a value of an
  outside type (`&cobra.Command{Use: "serve", RunE: run}`) as the
  construction of that type: `invokes_external` with the `construct`
  invocation, the sibling string literals as keyword arguments, the bound
  field as the callback's argument. Facts read it as a registration of the
  callable under the type (`Key` the type name, `Values` the literals;
  keyword literals have no order, so none is the address). New entry kind
  `command` for `binds`; fixture `BuildCommands` with `testing.InternalTest`.

## 2026-09-17 — Symbol roles replace per-site boundary decisions

- New reading stage `atlas_api`: one row per external symbol the repository
  calls, with optional `binds` (entry kind), `publishes` and `talks` cells.
  The roles make the boundaries: bound callables are entries of the role's
  kind, publishing calls are listeners that give their address to every
  entry on the same holder (`atlas_publish` asks which holder when the code
  could not follow the value), every site of a `talks` symbol is an outgoing
  boundary. Deleted: `listen_address` facts and `listenSelectors`, the open
  boundaries table (`decision`/`kind`/`basis`), the symbols' `outbound`
  selection. Registrations carry a `holder` (the producing call, followed
  through `Group`-like calls); a literal handed to a holder of callables
  (`Start(":8080")`) is a registration too.
- Operations carry `address`; the echo preset run yields `GET /users/:id` on
  `:8080` from `Handler.GetUser`. Live DeepSeek on echo: GET → `http_server`,
  Start → `publishes`, `database/sql.*` → `db`.
- Captions are off by default (`--captions` turns them on): prose cells keep
  their fallback and prose-only tables are not sent. A zone of one group is no
  container.

## 2026-09-17 — Object IDs in reading order

- `programindex.New` numbers objects breadth-first from the target seeds
  (`main` is `n1`), neighbours in source order, then owner and container;
  unreached objects follow by file and line, external symbols last. Relations
  are numbered by source object, then by site. Adapters are untouched; the
  python-tutorial-game artifacts and the places fixture hash are regenerated.

## 2026-09-17 — Registrations replace framework-named routes

- `facts` no longer knows Echo, FastAPI, Express, chi or any client library.
  `http_route`, `http_call` and `portal` are gone; one neutral `registration`
  fact records the shape of a call outside the repository that hands over a
  callable, a value named by a literal (`Register("k6/x/dns", new(DNS))`) or an
  address: call word, literals, stated verb, handler, external symbol, mount
  prefixes. Ownership rules exclude lookalikes: a repository callee, a value a
  repository function produced, a class declaring the member, a workspace
  package. Mixed inheritance and unknown receivers stay `possible` candidates.
- `sql_query` is a new fixed fact (statement + tables) and a `db` boundary
  without the model; the `sdkPackages`/`sdkNeverPackages` lists in places are
  deleted. Dead modules are judged repository-wide and only for files that
  declare callables (type-only files are never dead).
- Registrations reach the reading stage as open boundaries: incoming rows
  choose `http_server`, `queue_consumer`, `scheduled`, `interaction`,
  `extension` or `other`; the boundaries prompt gained that section. Accepted
  incoming boundaries become operations of the matching kind; routes, client
  requests and cross-target portals in the report come from the overlay and a
  literal join, no longer from facts. A refused candidate is removed, so a
  `--no-model` run has registrations and no routes.
- Arrow witnesses name the relation (`main supplies GetUser as the
  implementation of …`) instead of calling every relation a call; implementation
  bindings no longer make a declaration an operation candidate. Clojure resolves
  vars named in call arguments (so `(map service/greet names)` is a callback
  transfer with its argument) and writes platform calls as `invokes_external`.
- `TestEchoPresetReadingTurnsRegistrationsIntoOperations` reads the Echo
  service end to end with a preset in place of the model: `GET /users/:id`
  becomes the handler's request operation and the sqlc query a `db` outbound
  with table `users`, without a provider. Facts tests were rewritten as one
  table of call shapes. python-tutorial-game expectations converted. `go vet
  ./...` and `go test ./...` pass with python ≥3.11 and TypeScript on PATH
  (`PATH=$HOME/.nvm/versions/node/v21.6.1/bin:/usr/local/bin:$PATH`).

## 2026-09-16 — Field names in other formats are facts

- Go struct tags no longer ride inside signature text. Each named tag format
  becomes a ProgramIndex object alias (`json`/`count_label`); `,omitempty` and `-`
  are dropped. Places and question evidence show `aliases: json:count_label`
  beside the plain `Count string` signature, and the evidence vocabulary defines
  it. Go core object index v6 keeps the raw tag separately. The cumulative Go
  fixture checks both response JSON names reach question evidence.

## 2026-09-16 — One vocabulary and one resolution rule across languages

- ProgramIndex `invocation` is a closed set shared by every adapter: absent for an
  ordinary call, `deferred`, `goroutine`, `async_task`, `construct`. The new closed
  `dispatch` field says how the target was found: absent, `interface`,
  `interface_method`, `function_value`. Go `synchronous`, Python `direct`/`awaited`,
  JS `call`, and the prefixes `interface_invoke:`, `declared_interface_dispatch:`,
  `callback_transfer:`, `interface_binding:`, `callable_binding:*`,
  `function_value_call:`, `coroutine_result_argument:*`, `generated_cgo_wrapper:`
  are gone; the relation kind already names a callback or binding. Places,
  reading tables, operation activation and the evidence vocabulary use the same
  words; binding tables carry `kind` instead of a mechanism string.
- Python and JavaScript no longer demote one known target to `alternatives`:
  one target is `exact`, several are `alternatives`, none is `unresolved`, in
  every language. Python witnesses lost their `_candidate` suffix.
- Python no longer emits a `reads` relation for a callable reading its own
  parameter or local (293 of the cumulative API target's reads). Reads of fields,
  module values and enclosing scopes remain; the saved places graph was unchanged.
- Go signatures use short package names in the adapter itself, and a named
  type's signature is its form (`struct`, `interface`, underlying type) instead of
  repeating members that are already objects. Places no longer rewrites Go text.
- The JS helper no longer demotes a single `.js`/`.jsx` target either. Its
  compiler-backed tests normally skip without a global npm TypeScript; run with
  `PATH=$HOME/.nvm/versions/node/v21.6.1/bin:$PATH`. Two of them
  (`TestCumulativeJSTSRepositoryCompilerAndProgramIndexContract`,
  `TestCumulativeJSTSHTTPConstructorPathsAndEmptyCallbacks`) had failed before
  this wave: their expectations still used local `n*` IDs where places, boundary
  origins and question anchors carry the contract's target-qualified `t*.n*`.
  The code was correct; the expectations, and the JS `call` invocation /
  `.js` read demotion they also pinned, were updated. The full suite passes
  uncached with the compiler available.
- `go vet ./...` and `go test ./...` pass otherwise; python-tutorial-game artifacts
  were regenerated through the ordinary no-model run.

## 2026-09-16 — Go resolves interface values it actually observed

- A call through a repository interface whose observed value is an external
  type is now `invokes_external` of that type's method with the call's pattern.
  Echo's `sqlc.Queries.GetUser` reads `database/sql.*DB.QueryRowContext` with the
  `SELECT id, name FROM users WHERE id = $1` argument instead of an empty
  unresolved call (external call index v9, dispatch `interface_implementation`).
- The stores of an interface field observed in the program are its values. One
  resolved value is `exact`, several are `alternatives`; the former permanent
  `+1` unknown per field is gone (dynamic handoff index v8). The echo chain
  handler → service → Postgres → sqlc → `*sql.DB` is exact end to end, with no
  unresolved relation and no `targets_omitted`.
- Removed the 318-line reference resolver kept only to compare against the
  production walker, with its equivalence tests. The cumulative Go fixture
  gained `rowQuerier`/`OpenUserRows` and an executable expectation.
  `go vet ./...` and `go test ./...` pass.

## 2026-09-16 — ProgramIndex keeps each fact once

- ProgramIndex v17 removes the `contains` relation kind. Every adapter already
  wrote the identical pair as the object's `container_id`; all five no-model
  fixture runs had byte-identical pair sets. Reachability now follows
  `container_id`, and the python-tutorial-game fixture lost only those rows.
- The artifact no longer writes derivable counts: `*_observed` is retained rows
  plus the stored `*_omitted`, zero omissions and empty collections are absent,
  and coverage keeps only non-zero object/relation omissions. `Index` decoding
  restores the same in-memory values before validation, so consumers are unchanged.
- Go interface values passed into interface-typed parameters are now
  `binds_implementation` (`interface_binding:*`) instead of `passes_callback`;
  real callable values remain callbacks. Reachability, bindings evidence,
  report labels and the evidence vocabulary name the new kind.
- A Go SSA view that found no implementation for a call of a method declared on
  an external interface is no longer projected as a second empty `calls`
  relation beside its `invokes_external` fact. Places reads the declared API
  with `unresolved` resolution directly; `DispatchObservations` and the pair
  merge were removed (places graph v16, reading input v18).
- Fresh no-model runs, old → new `program-index.json`: echo `cmd/api`
  79,803 → 45,845 bytes and 93 → 37 relations; cumulative Go 10,919 → 5,488;
  Python `acme.api` 931,965 → 517,308 (1,304 → 734); JS/TS 469,681 → 269,165
  (631 → 385); Clojure 45,001 → 25,899 (45 → 31). `report.json` for echo
  218,257 → 152,263 bytes. Python/JS places and reading inputs are unchanged.
  `go vet ./...` and `go test ./...` pass.
- Still open in the same format: Python emits
  293 `variable_read_candidate` reads of locals such as `self`; invocation
  words differ per language (`synchronous`/`direct`/`call`); locations repeat
  full paths.

## 2026-09-16 — Provider rows keep artifact IDs

- Removed the atlas table layer's `r1..rN` renumbering. Requests, response
  validation, rejection diagnostics, accepted-row metadata, answer provenance
  and cache-row pointers now use the row's existing artifact ID.
- Removed positional recovery for keyless model rows. Missing, unknown or
  duplicate keys are refused instead of guessed from response order.
- Added stable `q*` identities to question-route artifacts and removed
  `selection:<symbol>` pseudo-places; symbol selection now uses the original
  compact `s*` place ID and remains a separate knowledge stage.
- Operation review likewise uses its source `s*`; atlas arrows receive
  persisted compact `x*` IDs instead of NUL-joined box-pair row keys.
- Independent interpretations of one place are indexed internally by the
  structural tuple `(place, stage, contract)`. The reader no longer encodes
  storage slots as `selection:s*` or `operation:s*` pseudo-identities.
- Exact-input memo fingerprints omit owner ID without minting a replacement,
  preserving shared interpretations for identical evidence while actual
  provider payloads retain real artifact IDs.
- A fresh exact `cmd/api` no-model run completed the whole artifact chain and
  rendered in the browser. Its 15 captured model-input tables use only the
  existing `t*`, `d*`, `f*`, `s*`, `b*` and `x*` artifact IDs: no `r*`,
  prefixed pseudo-ID, NUL pair or long row key remains. `go test ./...` passes
  after the change.

## 2026-09-16 — One graph schema reaches the report

- Removed the unused cross-shard `SymbolLinkIdentity` mechanism from
  ProgramIndex and every language adapter. Its 76-character hash was generated,
  copied into GroupsIndex/report and tested but never consumed by matching.
  ProgramIndex v16, GroupsIndex v12, ProgramPortfolio v4 and report v91 now keep
  only compact native IDs and explicit `x*` cross-target connections over
  target-qualified facts. Tests now assert actual resolution, origin,
  visibility and connection behavior instead of the dead hash transport.
- Ordinary progress no longer prints hashed adapter discovery keys as target
  scope. Target planning names the readable selected target and each target
  page uses its already assigned `t*` identity.
- Qualified compact refs now sort each segment naturally (`t2.g10` precedes
  `t10.g1`). The former helper understood a bare `t10` but treated every
  qualified group ref as unknown, making connection order depend on its prose.
- Report sections no longer mint a second target-name slug. The component keeps
  its existing `t*` identity and its anchors are direct compositions such as
  `t1-g9`, `t1-o1` and `t1-y1`. Questions and Learn concepts likewise use short
  page-local `q*` and `c*` ordinals instead of full text hashes.
- File-backed fallback groups describe what they contain (declarations from
  that source file) instead of exposing the internal "awaiting architecture
  grouping" stage state on the reader's map.
- Dependency catalog v2 replaces persisted hash identities with canonical
  catalog-local `i*` importer refs and `d*` dependency IDs. Target subsets are
  self-contained catalogs and semantic package fields, not copied IDs, own
  joins across them.
- Extraction artifact v2 normalizes accepted producer nodes once to `u*` and
  rewrites their links before facts are built. Exact plugin stdout remains
  replay evidence, but arbitrary producer IDs no longer become saved internal
  identity or reader-facing fact keys.
- The common System map now draws one physical arrow for each directed pair of
  visible nodes. Operation paths, calls, callback transfers and implementation
  facts between that pair are combined on the arrow while every original row
  remains in the two node readings. The Echo API map drops coincident lines
  instead of rendering the same `GetUser` route twice through parallel edges.
- Opposite directions between the same visible pair also share one physical
  route, with an arrowhead at both ends. This removes the second curve created
  when a service calls a repository while that repository implements the
  service's interface; direction remains exact in the retained relation rows.
  Re-rendering the saved Echo analysis made zero provider requests. Browser
  inspection counts 15 physical routes (three bidirectional) carrying all 18
  logical directed connections, down from 28 DOM arrows before display
  folding; the 48 short DOM IDs remain unique.
- A fresh exact `cmd/api` no-model run completed the current pipeline with 67
  objects, 93 relations, 31 facts, 22 claims, 11 groups and 23 connections.
  Its HTML has 48 unique IDs, no duplicates, a maximum ID length of 20, the
  new source-file fallback prose and no provider requests. Its dependency
  artifact contains only `i1..i6` and `d1..d14`; accepted extractor nodes and
  links contain only `u*`. An all-artifact `id`/`ref` audit finds no long graph
  identity (only the operational run ID), and no hash-shaped saved entity ID.
  Browser inspection shows the service file connected to the handler,
  repository, composition root and exact incoming request. Full tests, vet,
  build, and the standalone Echo module's tests and vet pass afterward.

- `places.json` v15 now seals one compact place namespace (`d*`, `f*`, `s*`,
  `b*`, `y*`, `m*`, `a*`) and rewrites parents, calls, edges and seeds in that
  same pass. Source paths remain source data, never entity IDs.
- Reading input v17 and atlas v10 remove hashed architecture identities:
  responsibilities, areas, interpreted boundaries and target joints are born
  as `p*`, `z*`, `b*` and `j*`. The same short refs appear in model windows,
  saved results and the projected GroupsIndex; there is no post-model renumber.
- Native ProgramIndex object ordinals remain target-local. Shared stages now
  qualify them as `t*.n*`, preventing two targets' `n1` declarations from
  colliding without inventing a hash or another ordinal namespace.
- Removed `ProgramView`: `report.json` now embeds the exact selected
  ProgramIndexes instead of copying their objects, relations, witnesses and
  coverage into a second presentation DTO.
- The common multi-target report carries every selected ProgramIndex once.
  Its group graph serializes the same thin GroupsIndex overlays used by the
  standalone artifacts; native subject facts and structural edges are hydrated
  only in memory for page construction.
- The common System map now qualifies target-local group/container DOM refs as
  `n-t*-g*`-shaped map IDs. The two-target browser walkthrough caught and fixed
  both failure modes hidden by the former global hashes: `g1` from `cmd/api`
  could erase `g1` from `cmd/users`, and an equal shared-code source location
  could falsely extend the HTTP request path into the Cobra target. Only exact
  cross-target connections may now create that join.
- The same walkthrough exposed one remaining unqualified renderer lookup: the
  second target's `n*` subjects could overwrite the first target's declarations
  in a group card. Page hydration now keys both subjects and declaration
  locations by target plus local ID. A cumulative two-target regression gives
  both targets the same local subject ID and requires each card to retain its
  own name and source file. Re-rendering the saved two-target report made zero
  provider requests; browser inspection shows both service cards contain the
  same six declarations from `internal/users/service/service.go`, while only
  `cmd/api` retains the incoming HTTP path and `cmd/users` explicitly has none.
- Report format v91 and ProgramPortfolio v4 deliberately have no compatibility
  reader. An executable JSON-shape test rejects a reintroduced `view`, target
  copy or serialized structural edge. Full product verification is recorded
  after the fresh ordinary run below.
- `go test ./...`, `go vet ./...`, `make build`, and the nested module's tests
  and vet pass. A fresh exact two-target no-model run analyzed both `cmd/api`
  (67 objects/93 relations) and `cmd/users` (57 objects/71 relations), then
  published one report with 36 facts, 22 claims, 19 groups and 37 connections.
  Its saved analysis restores through `repomap render` with zero provider
  requests. Browser QA on the same report shape opened `o1` and showed the
  source-backed path from the Echo handler through the generated Goverter
  converter, service and PostgreSQL repository to sqlc.

## 2026-09-15 — Compact fact IDs and direct grouping references

- ProgramIndex v15 assigns deterministic target-local `n*` object and `e*`
  relation IDs, with compact IDs scoped below relations for patterns, arguments
  and reconstructed values. Adapter SourceRefs are builder provenance and no
  no longer serialize into ProgramIndex JSON.
- Architecture grouping now sends selected ProgramIndex object IDs unchanged.
  Only reduction-created parts receive temporary `c*` choice refs; the former
  declaration `r*` remap is gone.
- The constitution and ProgramIndex/execution contracts now bind derived model
  decisions to the repository-index SHA instead of treating entity hashes as
  permanent cross-revision identities.
- No compatibility reader exists for v14. Regenerated the Python and TypeScript
  acceptance ProgramIndexes through their real adapters. `go test ./...`,
  `go vet ./...`, `make build`, and an ordinary Echo `cmd/api` no-model run all
  pass; the run retained 67 objects, 93 relations, 11 groups and 23 connections.
- Target-local node IDs exposed an orientation join that had relied on globally
  unique entity hashes. Evidence lookup now qualifies `n*` by target before
  reading the shared graph; the cumulative tests cover identical local IDs in
  different targets.
- Removed storage-v1 and the `program-facts` sidecar. Ordinary persistence now
  writes the complete sealed ProgramIndex directly; `ReadFile` only decodes and
  validates it and never reconstructs through adapter SourceRefs or `New`.
  Multi-target Python still parses each AST once in memory, while its persisted
  target artifacts are independent and contain no builder input.
- GroupsIndex v12 now persists a thin semantic overlay over the sealed
  ProgramIndex. Its subject rows contain only compact fact refs and semantic
  annotations; native object/pattern facts, target copies and structural edges
  are absent from `groups-index.json` and are joined from ProgramIndex on read.
  Groups, containers and connections use deterministic local `g*`, `k*` and
  `x*` ordinals instead of 64-hex content identities.
- The canonical target plan now assigns `t1..tN` before ProgramIndex creation.
  Selected-target outcomes, facts, atlas, GroupsIndex, page portfolios and the
  report all reuse that same ID; the former selected-target and facts-target
  hashes and `ProgramTargetID` translation table are gone. The obsolete v1
  target-outcome reader was removed rather than preserved.
- Facts v3 and claims v2 assign canonical `a*` and `h*` artifact IDs; GroupsIndex
  operations use `o*`. Orientation sends those IDs directly, with existing
  graph IDs qualified as `tN.gN` and `tN.nN`. Its response resolver checks the
  closed catalogue instead of guessing entity type from a synthetic ref prefix.
  Natural target ordering is shared, so double-digit IDs do not reorder sets.
- Final verification passes with `go test ./...`, `go vet ./...`, `make build`,
  and the nested Echo module's `go test ./...` plus `go vet ./...`. A fresh
  ordinary `cmd/api` no-model run completed in 1.865 s with 67 objects, 93
  relations, 31 facts, 22 claims, 11 groups and 23 group connections. Browser
  QA opened the request operation `o1` and showed its source-backed path through
  handler, generated Goverter converter, service, PostgreSQL repository and
  generated sqlc query.

## 2026-09-15 — Standalone Echo/sqlc example repository

- Added `testdata/echo-sqlc-service` as an independent Go module rather than a
  second scenario inside the cumulative Go contract fixture. It contains one
  Echo handler, interface-separated service and repository layers, a
  PostgreSQL sqlc configuration and checked-in generated query package, and a
  Goverter converter definition with its generated implementation.
- Added exactly two ordered migrations: the first creates `users` with only
  its identifier, and the second adds `name`. The module dependency lock was
  generated and `go test ./...` passed for every package.
- Extended the example with a Cobra `cmd/users` executable. Both it and the
  Echo `cmd/api` executable use the same `internal/app` composition path into
  the service, repository and sqlc query. The nested module passes
  `go test ./...` and `go vet ./...`; an exact two-target no-model run analyzed
  both targets and projected 19 groups with 25 connections.
- Go interface dispatch now follows interface-typed constructor parameters
  back to the concrete values at every static repository call. The cumulative
  fixture proves the assignment source and preserves an unresolved frontier;
  the rebuilt Echo/sqlc artifacts now retain
  `Service.GetUser -> Postgres.GetByID -> sqlc.Queries.GetUser`. Focused
  surface-discovery and contract tests plus `go vet` pass.
- Added the Go cube input `MatchInterfaceImplementations`. When enabled it
  indexes repository method identities, narrows candidates through an inverted
  method index, and confirms every repository-local type/interface pair with
  `go/types.Implements`; it does not require an observed assignment. The
  ordinary Go adapter enables it and projects separate exact `implements`
  relations for matching types and their directly owned methods, while runtime
  dispatch remains a separate observed relation. The cumulative fixture proves
  a compatible implementation that is never assigned to its interface.
- Focused cube, adapter and cumulative contract tests plus `go vet` and the
  canonical build pass. A fresh exact two-target no-model run produced 67
  objects/93 relations for Echo API and 57 objects/71 relations for Cobra CLI;
  the common report contains 19 groups and 37 connections, including all four
  exact type/interface pairs and their method pairs.
## 2026-09-25 — Glossary concepts, not code names

- Self-runs spent most glossary output on `identifier` terms that code then
  discarded (121 of 136 and 100 of 182 generated terms) on the serial 15–30 s
  glossary step. `identifier` is no longer a generated kind; the embedded
  prompt explains domain and concept terms only and names what not to define
  (declarations, packages, files, paths, environment/configuration keys, flags,
  headers, commands, codes). A retired `identifier` answer refuses that term.
- After validation, a term whose whole name exactly equals a code name the
  owner already holds, and is in code spelling, is dropped and journaled by
  name in `rejected.jsonl` (`glossary_code_name_omitted`). Names come from
  every published target's ProgramIndex declarations (including locals,
  parameters, fields and enum members) and packages/modules, `config_read`
  environment keys, and corpus paths with their file names; saved `read` uses
  its graph's declaration names and source paths. No artifact records flag
  names, so flags rely on the prompt. Names never enter the provider request.
  Generation contract is `repomap.glossary.generate.v4`; the prompt change
  alters request bytes, so old cached answers are not reused.
- Review: matching every declared name dropped the domain vocabulary the
  glossary exists for (saved freqtrade: 543 of 1,764 terms, including `candle`,
  `timeframe`, `ROI`, `RPC`; ten saved self-runs: 74 of 586 concept-kind terms,
  including `JSON`, `API`, `SHA256`). Only a code spelling now counts: a
  separator or sigil (`_ . / \ : $`, Clojure `* ! ? < > =`) or a lower-to-upper
  case step. Freqtrade then drops 393 names, all code spellings; the self-runs
  drop 4 concept occurrences (`ProgramIndex`, a struct field) and catch 486 of
  1,173 former `identifier` terms rather than 791. Single-word code names rest
  on the prompt, which now says a word code also uses remains a concept.
- Tests: terminology exact-name drop/journal with domain words that code also
  declares (`candle`, `Exchange`, `JSON`, `ROI`) surviving, and prompt kinds;
  run-package ProgramIndex/facts name selection and a saved-read run where
  `FetchOHLCV` lands in `rejected.jsonl` and the declared acronym `OHLCV`
  stays in `terminology.json`. Freqtrade rates replay saved answers through the
  committed Go rule; self-run rates use an exact mirror of it, because this
  branch's ProgramIndex decoder rejects their newer `end_line` field. No online
  run was made.

## 2026-09-15 — One part, one card

- Removed the drawing-only area wrapper when its sole child is an existing
  part. The saved area, part identity, native relations, operation membership
  and sources remain unchanged. Old area links focus the part, and the reading
  panel retains the area's additional description. Multi-part areas and
  participant/input/external boundaries retain their containment.
- Direct parts use the neighbouring groups' column width and fixed first-reveal
  heading fit, with their measured card height. No extra zoom hint or intermediate
  wrapper appears. The changed native packing also exposed insufficient outer
  text reserve in the short-name fixture; at most one additional correction
  now uses the first correction's actual placement, before display.
- Added identity/source coverage and first-reveal/closer screenshot comparisons
  for a singleton area, including complete text bounds and readable headings.
  Inspected the 71-frame contact sheets and the changed singleton and short-name
  entrances at full size. The existing dense 42-group first-reveal readability
  limitation remains; this change does not claim to resolve it.
- All 65 UI unit tests, focused report tests/vet, embedded asset verification
  and the canonical build passed. All 33 desktop scenarios passed normal
  screenshot comparison against the inspected references, without updating them.
  Saved Python rendering made zero provider
  requests. Its three singleton areas now draw their parts directly; the API
  heading measured 14.2px at component entrance. Original area links, source
  reading, additional descriptions and exact camera/world return passed with
  no browser errors. The resulting real-report frames were inspected.

## 2026-09-15 — Named groups at the first reveal

- Reproduced the first shared reveal with actual Ctrl+wheel gestures: compact
  group titles were hidden by a container query while their hints survived.
  Titles now fit their complete names once to the fixed group rectangle. The
  native frame paints the same thin outline at both levels; the title overlay
  no longer adds its own scaled border. Hover and zoom retain the text layout.
- Inspecting the saved Python report exposed a separate miniature-in-a-corner
  result: the final outer text reserve enlarged participant frames without
  fitting their contents. One uniform transform now fits the prepared cards,
  text and routes to the final frame. Component entrance also keeps the complete
  frame in view instead of panning to a topmost child and hiding its sibling.
  No additional native layout runs during navigation.
- Added first-reveal and slightly-closer screenshot comparisons, with title
  paint, text bounds, screen outline width and unchanged-world assertions. The
  short-name fixture requires 10px headings, measured to 0.1 CSS pixel. Exact
  affine geometry remains checked in unit tests; browser comparisons allow
  CSSOM serialization rounding of large world coordinates.
- Inspected the complete 69-frame gallery as contact sheets, then key entrances,
  role colours and changed camera states at full size. The dense 42-group
  example retains small text at first reveal and still needs further zoom for
  reading; its nonblank-label regression is not a claim of initial readability.
- All 64 UI unit tests, focused report tests/vet, embedded asset verification
  and the canonical build passed. All 32 desktop browser scenarios then passed
  normal comparison against the reviewed references, without updating them.
  Rendering the saved Python report made zero
  provider requests. Its first reveal showed all seven front group names at
  12px with no clipping; the part/source journey and exact camera return passed
  with unchanged world geometry and no page errors.

## 2026-09-14 — Stable detail layers, readable frames and one visible connection

- Detail now switches for every participant or group at the same hierarchy
  depth. Closed groups go directly from their title to actual objects; the
  intermediate member list is removed. Compact group titles share the common
  entrance scale. Pan, hover and selection preserve text layout, and the small
  nested-content hint stays inside its frame.
- Native area interiors are placed first; components wrap their ready child
  rectangles. One bounded outer sizing correction protects measured summary
  minima, including parallel rows and initially readable participants, without
  rebuilding interiors or adding zoom-time layout work.
- Reproduced duplicate exact/possible arrows on the saved Python report:
  `Backend launch` to `Application settings` has five original source relations.
  They now share one visible directed arrow. Original IDs, certainty and source
  reading remain intact. Between-group routes stop at group boundaries at every
  depth; known measured dimensions prevent React Flow from temporarily dropping
  arrows during presentation updates.
- Endpoint numbers sit just inside the native connection and match inner
  badges. Core and entry cards use distinct diamond/arrow symbols and the
  shared legend instead of repeated category captions. Darker boundaries and
  routes retain contrast during neighbour dimming; actual browser colours and
  a grayscale image are checked, without claiming whole-report WCAG conformance.
- Validation: all 64 UI unit tests and 30 desktop browser scenarios passed,
  including normal comparison against the reviewed references. The complete
  gallery contains 65 images across 22 journeys. Focused report tests/vet,
  embedded asset verification and `make build` passed. The canonical binary
  rendered the saved Python report with zero provider requests; its real
  component-to-part/source journey and camera return passed with unchanged world
  geometry and no page errors. A separate real-report check retained all five
  source relations behind the single launch-to-settings arrow. No new online
  analysis was run for these UI changes.

## 2026-09-14 — Reveal the target before its smallest text becomes readable

- Reproduced centre-aimed pinch on both saved Python targets: even at 9.82 zoom,
  the component state was open but every inner card remained offscreen behind
  a persistent summary. The previous star journey aimed at its first area and
  missed this entrance. Two new near-frame-size regressions failed before the
  correction and check actual on-screen inner cards.
- A real approach now reveals the first level at 75% of a canvas dimension,
  retaining it down to 65%; whole-map mode stays summarized. Compact area names
  precede full member lists. Unreadable names and oversized zoom controls do not
  paint over tiny boxes; the location row retains a parent name when its world
  header is too short. Camera and world geometry remain unchanged during zoom.
- Rejected extending the input collection's native column comparison to
  components: it raised an ELK exception on the saved Python report. The
  correction keeps the existing placement and adds no layout work or variant.
- Validation: 52 UI unit tests and all 25 desktop browser scenarios passed
  against the existing references and two reviewed new near-frame PNGs.
  Report tests/vet, embedded asset verification and the canonical build passed.
  Saved rendering made zero provider calls. Centre pinch on the real Python
  report exposed all five backend areas after four small wheel steps and all
  seven frontend areas after two, with no zoom-button click. Its separate
  pan/restore probe retained camera scale and world geometry without page errors.

## 2026-09-14 — Pinch, participant boundaries and overlapping summaries

- Reproduced a scrolling inventory intercepting pinch and a one-target,
  twenty-destination map retaining its summary through repeated pinch gestures.
  Ctrl+wheel now reaches the canvas. Removed interior space reserved for
  cross-participant label titles after those labels moved to the outer boundary;
  the original local boundary routes still guide ELK placement.
- Outer arrows end at participant frames at every detail level. Numbered
  endpoints retain their exact inner parts, directions, possible status and
  complete source relations. The screenshot journey shows an inner number,
  then pans to its matching native outer endpoint without changing scale.
- Compared free outer endpoints with the prepared native ports in the existing
  two orientations and ordinary/unzipped layers. Fitted height uses the actual
  rendered width. A shared 16px overview inset avoids losing a readable fit to
  empty margins. The wider-external-title experiment was rejected because it
  regressed the scenario with input collections; no extra variant was added.
- Screenshot review reproduced the reported “inner border” as an opened
  child's background covering the root summary. A partly visible child now
  removes that fallback even if its heading is offscreen. Every captured zoom
  step checks this exclusion, alongside stable world and pointer anchoring.
- A saved cross-owner connection with a unique known peer no longer draws that
  same peer again as an external system. Original outbound reading and source
  anchors remain; partial and ambiguous matches stay explicit. The private
  repository's reported `run service` cannot be diagnosed from the name alone.
- Validation: all 51 UI unit tests and all 23 desktop browser scenarios passed,
  including the normal comparison against the reviewed PNG references. Focused
  report Go tests and vet, embedded asset verification and `make build` passed.
  The canonical binary rendered the saved Python report with zero provider
  calls; its browser probe confirmed readable interior reveal, restored camera,
  unchanged world geometry and no page errors. No new online analysis was run.

## 2026-09-14 — Optional question cascade

- Added ordinary `--no-questions`. It overrides configured and explicit
  questions and skips Learn, retrieval and answers at the existing reading
  boundary. Other analysis and publication retain their ordinary path.
  Effective run options record the choice; default behavior is unchanged.
- The canonical binary built and focused vet passed. The owner requested an
  immediate push without an online acceptance run; no provider calls were made
  for this change. The unfinished test additions were removed before publication.

## 2026-09-14 — Reveal readable interiors when their complete frame is visible

- Reproduced a fully visible area remaining a summary at readable text scale.
  The new browser regression fails against the previous bundle: panning the
  whole frame into view still leaves its original parts hidden. Complete frames
  now enter detail at the existing 12px readability floor; partly visible frames
  retain the 14px entrance and open frames retain the 12px exit. A pan checks
  visibility only at gesture end. Camera coordinates, geometry and layout work
  are unchanged, including restoration of the before/after cameras.
- Reviewed the two changed threshold PNGs: parts appear one zoom step earlier.
  Normal comparison passed all twenty desktop scenarios; all fifty UI unit
  checks, asset consistency, focused report tests and vet passed. The gallery
  contains forty actual frames across eleven journeys, including the new pan.
- The rebuilt ordinary binary rendered the completed Python report with zero
  provider calls. A browser pass through its backend launch/configuration area
  verified partial-to-whole reveal, restoration and unchanged world geometry,
  with no page errors.
- The reported arrow disappearance was not reproduced. The separate live
  review preserved all original outside relations and arrowheads through hover,
  zoom and All; unrelated routes become faint on hover. No unsupported route
  or layout change was made for that report.

## 2026-09-14 — Separate native layouts and completed desktop journeys

- Replaced repeated whole-graph growth with independently prepared participant
  interiors and a flat outer ELK layout. Native boundary ports join the original
  routes; outside strokes shared by several calls are painted once while each
  original endpoint, possible status and source remains available. Pan, wheel,
  zoom, selection and same-size All perform zero ELK requests. Resize reuses
  interiors and places only the outer rectangles. Tests measure those actual
  Worker requests and preserve every original item and local geometry.
- Independent layout exposed the remaining summary mismatch on the ordinary
  Python report: short names squeezed seven/five area names into narrow columns.
  Preferred width now comes from measured two-line area names. Constant width
  experiments were rejected because they regressed the seventeen-destination
  case. Input collections compare two native column arrangements against their
  own summary shape; eleven frontend inputs use two columns and three backend
  requests retain one. No repository or participant-count exception was added.
- Fixed clipped input-type lists when partly offscreen. SVG strokes, dashes,
  arrowheads and container outlines now retain screen sizes. Raster checks
  cover the close-up frame bug: a 21px border at zoom 10.72 becomes a 2px inset
  outline without changing its world rectangle.
- The nineteen desktop scenarios passed with reviewed updated PNGs. They cover
  the small, dense, nineteen/twenty-one-participant maps, both detail transitions,
  input paths, resize, Worker failure and actual stroke pixels. A new prepared
  1054×580 canvas reproduces the ordinary report's space: its previous bundle
  failed on three-line area names; the corrected complete lists use one/two
  lines. Final normal comparison passed all nineteen scenarios in 1.8 minutes
  without updating references. All fifty UI unit checks and generated-asset
  consistency also passed.
- Focused report tests and vet passed. The canonical binary built with ambient
  Go caches and rendered the existing completed Python run with zero provider
  calls. Ordinary browser acceptance passed: all seven/five areas are visible,
  frontend inputs occupy two columns, Run opens the saved playground.tsx:60
  source, Robot movement retains the original operation path, and Back/All
  restore the exact cameras and unchanged world with zero page errors.
- Independent simplification review confirmed the split architecture. Reusing
  unchanged outer candidates on resize, consolidating summary metrics and
  removing a duplicate local zoom boolean are deferred; they do not expand
  this acceptance or change saved history in this revision.

## 2026-09-14 — Inputs outside components and desktop scope

- The owner requested grouped inputs outside target frames: one compact
  collection per target with the existing catalogue types, opening into its
  named inputs. The outside shows entrances and communications, while parts
  stay inside. This replaces the previous embedded-input presentation and
  avoids an individual root card for every input. Existing input identities, owner
  context and saved implementation relations remain intact; no attachment is
  invented for an unbound input. Ordinary projection now keeps those endpoints
  instead of substituting their implementing part.
- The owner explicitly excluded narrow-window work. Active screenshot
  acceptance is the 1440×900 desktop project; the previous narrow seventeen-
  destination diagnostic is not a release criterion. No whole-map semantics
  were changed to hide destinations outside the viewport.
- Fixed two measured sizing mismatches: external headings may use the full
  width below the zoom control, and two-line names reserve their actual text
  and control height rather than a 52px floor. The dense case went from a
  6.58px shortage to a readable fit in the isolated probe. The intermediate
  zoom-out now closes unreadable interiors before restoring a component
  summary. An independent PNG review confirmed the overlap was removed.
- Input entrance implementation and its screenshot journey are covered by the
  completed desktop and ordinary-report evidence above.

## 2026-09-14 — Seventeen external participants reproduction

- Added prepared input with two components of twenty parts in four areas each,
  seventeen external collections of six actual call cards each, and a caller
  for every call. The ordinary canvas receives 172 records and 140 relations;
  the fixture supplies no layout or additional grouping.
- The previous UI completed its 1440×900 baseline in 87.094 seconds, including
  110 ELK calls taking 85.55 seconds. Root-size reservations repeatedly enlarged
  the same external row; the world reached 917,092 by 613,409 units and fitted
  at 0.00108. The result still split destination names and overlaid zoom marks.
  Thirty whole-map pan events used 112 ms of script time and zero text
  measurements in that probe, so startup and detailed movement need separate
  measurements. These are diagnostic results, not accepted UI performance.
- The native layer-unzipping experiment made the wide nineteen-root default
  readable, but forcing it regressed the original small fixture: 182 native
  calls and 11.34 seconds, compared with 34 calls and 2.06 seconds before the
  experiment. Ordinary and unzipped native candidates are now compared in both
  directions using the existing fitted-text score. Isolated probes retained
  the readable wide geometry in 16.87 seconds (32 calls), while the small
  original fixture completed in 3.19 seconds (32 calls). The narrow nineteen-root
  result was still unreadable and was not accepted at this intermediate stage.
  The owner subsequently excluded narrow-window work; the active desktop
  acceptance and completed fix are recorded above.
- Browser ELK now runs in one reusable embedded Blob Worker. A before/after
  probe of the same sixteen-call placement had byte-identical geometry, routes,
  camera and PNGs. The maximum observed main-thread long task fell from 668 to
  102 ms, while total placement rose from 7.46 to 8.95 seconds. This improves
  responsiveness, not calculation speed. Normal, early-failure and pending-call
  failure probes confirm that worker errors reject rather than leave loading
  pending; no main-thread retry or extra runtime asset is introduced.
- Initial drawing remains concealed until its camera is ready. Parts and
  external calls reveal at an actual 14px heading size and remain down to 12px.
  Visible grandchildren and saved part-name rows count when retaining context
  during zoom, so an offscreen area heading does not cause its parent summary
  to cover a visible part. Tiny secondary header text stays hidden without
  changing world geometry; closed area summaries draw one border.
- The two restored-grandchild browser regressions and 27 UI unit checks passed;
  focused report tests and vet passed. The ordinary binary rendered the saved
  Python run with zero provider requests. A browser journey reached the run
  interaction from its question, opened playground.tsx at line 60, and returned
  to the same question. Full screenshot acceptance is still in progress.

## 2026-09-14 — Dense overview, wheel work and window-size regression

- Reproduced the reported thin-strip failure on a desktop prepared input:
  two systems and five external participants, with forty additional areas per
  system. Fitting complete wrapped inventories repeatedly increased world
  height and decreased their available text width. The resulting world reached
  38.8 million by 1.34 billion units; no root heading was readable.
- Initial placement now measures at least a whole word's width and reserves
  a usable entrance for oversized inventories. Small lists still reserve their
  complete height; every entry in a long list remains scrollable and selectable.
  Tests reach the final area and its actual part and return to the same world.
- Closed overview movement used to rebuild all graph props and remeasure its
  text on every event. Overview labels now subscribe to the camera separately.
  On the same dense fixture, median JS CPU work across three runs dropped from
  645 to 53 ms for thirty wheel-pan events and from 413 to 49 ms for twenty
  zoom events. These are CPU measurements, not FPS. Actual camera displacement
  was checked; the permanent pan regression requires zero remeasurements when
  text width stays unchanged and preserves zoom/world geometry.
- Manual verification of the saved Python report then exposed a separate
  failure after the window narrowed from 1280×720 to 934×1024: a fixed external
  frame became shorter than its screen-sized title and zoom mark. A new resize
  journey fails against the preceding bundle (heading bottom 605.90, frame
  bottom 590.97). Whole-map placement must be measured for the new viewport;
  ordinary zoom and resizing during selected reading keep the current world.
  Long inventories also show a subtle scrolling edge and a thin scrollbar.
- The smaller viewport also exposed a fixed four-pass reservation stopping
  before short lists fitted. Orientation scoring now uses measured text;
  reservation continues to a readable fit, with repeated screen geometry and
  necessary physical-space bounds stopping unproductive growth. That fallback
  retains the complete graph and does not claim its text is readable. Startup
  capture waits for the inner canvas fit; Back honors saved whole-map intent
  before comparing the old geometry identity.
- Visual inspection rejected another existing reference: after a wheel zoom
  crossed the component boundary, only empty borders remained in view. A
  component keeps its title and actual area entrances until a child heading is
  visible. The journey now requires that visible content and uses smaller real
  pinch steps to capture both detail transitions separately. Camera coordinates
  and the world stay fixed through these display changes.
- Acceptance: 24 UI unit checks and the generated-bundle check passed; the final
  normal screenshot comparison passed all 12 scenarios in 2 minutes without
  updating references. Reviewed the dense and resized defaults and both complete
  zoom journeys. Only the component entrance permits three observed glyph-edge
  pixels; geometry, clipping, text visibility and all other image checks remain
  strict. Focused report tests and vet passed; `make build` produced the ordinary
  binary. Rendering the completed `20260914-075635-python-tutorial-game-7ca39618e06e`
  run completed with zero provider requests.
- In the actual Python report at 934×1024, the whole backend API name and all
  seven frontend/five backend area entrances fit. Opened Simulation domain and
  its four parts, opened backend API and all three calls, and used browser Back
  to recover the complete overview. The earlier source journey reached the
  original `backend/app/field.py:10` and returned. Reloaded the HTML screenshot
  gallery and confirmed all six displayed sequences report `passed`.
- CI on `bcd7a589` passed the full Go/native tests, vet, build and eleven visual
  scenarios. The dense desktop journey exhausted its 30-second total while
  capturing the opened final part, before the return action. Its trace showed
  successful default image comparison and part navigation; initial load/fit
  consumed most of the budget. Give this complete multi-action journey the
  same 60-second total as the other zoom journey. Assertion timeouts and image
  tolerances stay unchanged; this does not resolve initial-layout latency.

## 2026-09-14 — Audit question and answer request counts

- `live` counts prepared provider windows, including refused parents replaced
  by adaptive splitting. Network attempts within one window are separate.
  The completed Python run obtained 16 answers with two `atlas_question`
  requests and one `atlas_answer`; a high count is not required per question.
- Question preparation still splits at eight questions before checking the
  provider envelope. Commit `570a3fcd2` introduced that bound after a 64-question
  Freqtrade response omitted most answers. It is an empirical quality workaround,
  not a provider limit. The answer stage has no equivalent eight-question bound.
- The two saved Python retrieval windows repeat identical 51-row evidence
  (182,439 bytes). Combining all 16 questions with all those sources produces
  about 224 KB of prepared input instead of the two windows' combined 445 KB.
  This establishes duplicated input, not the quality of a combined response.
  No batching policy was changed without the saved-window replay comparison.
- The older Airflow run supplies a different reason for high counts: initial
  retrieval/answer requests contained about 24/29 MB, received `context_tokens`
  refusals and were subsequently divided. The reported other-machine counts
  of 17/11 cannot be assigned to either cause without that run's evidence.
  This audit used local code, history and saved requests only; no provider
  calls or additional response caches were created.

## 2026-09-14 — Preserve Python author descriptions at their declarations

- Following the boundary input audit found two existing losses: the claims
  scanner skipped methods and nested functions, and Places searched above a
  Python declaration even though its docstring is inside the body. It could
  consequently attach the preceding method's description to the next one.
- The existing claims path now retains each nested/class/async docstring with
  its exact declaration line while keeping the quote's original source line.
  Places joins that claim to its native declaration and keeps unowned module
  text separate. Strings containing example declarations and later expressions
  do not become docstrings; inline bodies cannot borrow a following quote.
  No ProgramIndex field, call relation or runtime classification was added.
- Extended the cumulative Python destinations example without moving existing
  declarations. The ordinary claims → Python native index → Places → boundary
  request regression failed under the old projection: class/method/nested
  descriptions shifted to their neighbours. It passes with exact attachment,
  including the undocumented negative case. Comparing the complete mixed
  fixture graphs shows only the expected removal of the module description
  from two declarations that previously borrowed it; its file description
  remains. The canonical graph checksum records that correction.
- Go and Clojure already have owning author-quote tests. JS/TS supports JSDoc
  before declarations; JSDoc directly before bare class methods remains a
  separate missing equivalent. This change does not claim to fix it.
- This is an offline input correction. A fresh ordinary online acceptance run
  for this additional change is pending provider balance; the earlier Python
  acceptance predates it.

## 2026-09-14 — Independent prompt, model and validation review

- Prompt, Qwen-compatible transport, DeepSeek response quality and QA reviews
  checked the current code and saved complete requests without provider calls.
  The boundary-specific prompt has 593 words; the full prepared system prompt
  has about 1,309, including the shared evidence vocabulary and response shape.
  Removing that vocabulary was already tested and did not resolve the SQLite
  mistake. The 15-window comparison is a diagnostic regression set after its
  results informed variant selection, not a fresh holdout or a general accuracy
  estimate. No further wording change has demonstrated an improvement.
- Existing wire tests confirm explicit thinking on/off/omission and separation
  of response caches for the compatible endpoint. Qwen inference remains
  untested locally. The response format requests JSON, not provider-enforced
  enum constraints; current owning validation still checks each independent row.
- Extended the existing boundary regression with the reported
  `decision=remote_client_instance` error beside a valid positive answer, plus
  `none` and `unassessed` carrying wrong-typed inactive cells. Only the invalid
  decision is refused; useful neighbours survive and inactive cells have no
  effect. Lines/table tests and vet pass. Production validation is unchanged.
- The older Airflow binary completed atlas analysis and then stopped with HTTP
  402 at orientation (exit 1). All three target analyses are saved, but no final
  report was published. Its 808 diagnostic records include adaptive provider
  refusals and omissions, not 808 failed final answers. In particular, 464
  omitted memberships were called `group_rejected` by that older binary.
- Revalidated all six complete saved Airflow area responses through the current
  ordinary decoder: all 168 proposed groups are accepted, versus 18 accepted
  by the older run, with no provider calls. This counts decisions across windows
  (including successive grouping rounds), not 168 distinct final map areas.
  Eighty-three groups had been lost inside accepted windows and another 67 in
  three wholly refused windows. The temporary owning-package probe was removed;
  its comparison is `/private/tmp/repomap-airflow-design-revalidation-20260914.json`.
- Two known semantic errors remain in the saved DeepSeek diagnostic set: an
  explicit local SQLite constructor labelled external, and an unsupported
  PostgreSQL destination for a configurable SQLAlchemy constructor. They must
  be evaluated against source evidence; schema acceptance cannot establish
  correctness. Further live comparison and Airflow publication need provider
  balance. The previously completed ordinary Python acceptance remains valid
  for the current production changes.
- Source inspection also found a separate input gap: Airflow's nested
  `setup_mock_aws` at `test_log_handlers.py:679` has a docstring and runs inside
  `with mock_aws()`, but request `atlas_boundaries-r4-w1024` supplies an empty
  owner doc and no enclosing mock context for `boto3.client` at line 681. The
  existing quote scanner omitted nested docstrings, and the declaration
  attachment searched on the wrong side of Python headers. The correction
  above keeps the existing claims path; no mock classification was invented.
  A frozen next-comparison manifest selects eight unseen complete windows
  (23 rows) by source categories/order plus five known regression windows:
  `/private/tmp/repomap-boundary-prompt-probe-20260914/next-boundary-evaluation-manifest.json`.
  Their expected distinctions and source sufficiency were recorded without
  reading the unseen responses. No new requests or response caches were made.

## 2026-09-14 — CI prerequisites and screenshot rasterization

- The first remote run exposed an existing CI omission: native Clojure tests
  require `clj-kondo`, but the Ubuntu job never installed it. CI now installs
  the same official 2026.08.04 release used in local acceptance.
- Seven of eight screenshot cases passed on the macOS 15 Intel runner. The
  component entrance differed at one glyph-edge pixel near the bottom of the
  image; actual/expected/diff inspection confirmed matching geometry and text.
  Only that screenshot permits one pixel. Other screenshots, gesture anchors
  and readability checks remain strict; no region is masked or reference
  replaced by the CI output.

## 2026-09-14 — Prepared-input canvas screenshot journeys

- Added pinned Playwright/Chromium screenshot checks at 1440×900 and 1024×768.
  The prepared two-system/five-external input enters the ordinary canvas bundle;
  production card measurement, ELK placement, routes, hover and gestures run for
  real. It makes no model calls and adds no model or Go cache.
- Eight browser cases compare 22 reference PNGs. Each zoom journey also emits
  ten ordered actual images: overview, aim, both detail boundaries, explicit
  pan to the area, readable parts and return. A plain HTML image sequence sits
  beside Playwright's expected/actual/diff report. CI uses macOS 15 Intel and
  never updates references automatically.
- The tests exposed clipped area lists, names broken mid-word and external
  calls still rendered at 10.2px after an entrance. Initial placement now
  reserves measured overview text space, narrow headings clear the zoom icon,
  and external entrances use readable content focus. Closed summaries stay in
  the visible portion of their own frame while approaching the detail boundary.
- An independent screenshot-test review found that aiming was silently clamped
  to the canvas edge and a sliver of a card satisfied the old viewport assertion.
  The corrected scenario aims at real visible text, checks complete titles and
  the selected card, verifies the world point under every zoom gesture, and
  records explicit pan actions. Actual images and camera/aim metadata survive
  a failed check; visual differences remain failures even while later frames
  are collected. World geometry stays unchanged through the journey.
- The narrow-window boundary frame still exposes a UX limitation: after the
  component layer opens, its contents may require a pan. That intermediate
  image is explicitly captioned, followed by the pan frames. Reference matching
  records current behavior; it is not acceptance of the whole zoom experience.
- After visual inspection, normal comparison without snapshot updates passes
  all eight cases. All 23 web tests, bundle verification, report tests/vet and
  `make build` pass. Saved rendering of the completed online Python run makes
  zero provider calls. Browser inspection covers the readable external calls,
  a scenario question, and Back to the same selected communication.

## 2026-09-14 — Compare boundary prompts and preserve free-choice formatting

- Replayed ten prompt/request variants on four complete saved Airflow windows
  through official `deepseek-v4-flash`, with thinking explicitly disabled.
  Additional windows, combined variants and three fresh repeats brought the
  comparison to 106 live replay requests. No source evidence was sampled or
  shortened; all requests and responses use the existing system cache. Qwen
  on the owner's other machine was not tested locally.
- The selected prompt is 70 lines / 593 words instead of 155 / 1,438. It puts
  `decision` and `basis` in separate table columns and distinguishes source
  indexing from communication with another process. Moving selected rows before
  their complete shared context was necessary in this comparison; shortening
  the prompt alone did not fix the SQLite examples. The response schema,
  destination catalogue and reasoning setting are unchanged.
- Comparing the selected ordering on 15 complete windows / 19 rows removed
  eight false external relationships involving SQLite or an in-process HTTP
  transport. Actual HTTP and SMTP relationships survived. One SQLite row and
  one unsupported PostgreSQL destination remain semantic errors in this
  diagnostic set, not schema refusals. This selected set is not an estimate of
  accuracy across repositories. Three fresh repeats of the four seed windows
  preserved the selected variant's decisions.
- One minimal-prompt response wrote `other:Airflow executor`. The old shared
  decoder refused the missing space. Free-choice tags now accept whitespace
  around the colon while preserving the model's written value; unknown tags
  and empty values remain unresolved. All 106 saved responses pass the owning
  decoder after this normalization, including that semantically unsupported
  executor relationship. Syntax acceptance and interpretation quality are
  recorded separately; no decision or destination was inferred by the decoder.
- The free-choice regression failed before the change and passes afterward.
  Focused lines, table and reading tests/vet and `make build` pass. Ordinary
  online Python acceptance at `78714d34ee` completed with exit 0 in 287.7s:
  both targets, 16 answers, one common manifest/report JSON/HTML and the full
  artifact chain. Both live boundary requests were accepted. Browser inspection
  followed Backend API → POST `/api/level/run` → the exact frontend source
  and caller, then Back. The older Airflow process remains in progress and
  predates these changes.

## 2026-09-14 — Audit recent refusals and remove design-only strictness

- Inspected 23 `rejected.jsonl` files from September 13–14 under the existing
  system run root and task run roots, plus their saved responses and stage
  accounting. Their 142 diagnostic records are not 142 failed model calls:
  cached responses recur and omission summaries contain multiple items.
- The removed area minimum produced 62 group refusals across 12 runs. In the
  latest complete Python report it removed 7 of 13 proposed areas; in Othello,
  2 of 8. Revalidating all six complete saved design windows with the current
  decoder accepts all 21 areas, recovering those nine without provider calls.
- Another 13 `group_rejected` records were merely omitted declaration refs,
  allowed by the prompt. Design diagnostics now distinguish `ungrouped_input`,
  `unknown_member` and `group_rejected`; the stage counts affected windows once.
  Removed two further new restrictions: explicit empty groups are accepted in
  every mode, and caption whitespace is normalized like the preexisting table
  decoder. Empty required fields, malformed rows, groups without known members,
  and conflicting known ownership remain refused; independent groups survive.
- Compared production refusal additions against `1ecad4db`. The remaining
  additions bind native Clojure/project inputs, JS/TS value-read refs and source
  locations, structural-edge locations, and unique declaration membership.
  No new boundary-choice validator or boundary prompt was introduced. The
  reported `remote_client_instance` decision failure was not present in these
  local journals; this does not establish what the other machine was sent.
- Existing glossary diagnostics dominate item counts. The last Othello glossary
  response has 684 entries: 639 identifiers, 44 domain terms and one malformed
  row. It omits 482 valid identifiers and refuses 158 terms without an exact
  supported occurrence (157 identifiers and one capitalization mismatch), plus
  the malformed row. There is also one earlier whole glossary output-limit
  refusal. These predate this change; their diagnostics are not evidence of
  hundreds of failed primary-analysis calls. The old documentation reducer
  also drops concepts after twelve per document (79 counted across repeated
  runs); its rule is unchanged here.
- The in-progress Airflow run has one malformed JSON response among 863 symbol
  windows: `r25` lacks its closing quote, losing 30 of 4,090 descriptions.
  All 137 operation windows were accepted. Final Airflow and browser acceptance
  remain pending; the running process predates these design fixes.
- Regressions reproduced empty-response, wrapped-caption and diagnostic-count
  failures before the change. Reading, GroupsIndex and report tests, reading
  vet, saved-response revalidation and `make build` pass. No cache was added or
  cleared, and no model answer was rewritten.

## 2026-09-14 — Remove the new two-part area rejection

- The owner reported `atlas_zones: an area must contain distinct collaborating
  parts` on another computer. The newly added design decoder was imposing a
  two-member minimum, not checking collaboration evidence. Removed only that
  new restriction; title/shape, known-reference and conflicting-membership
  validation remain intact.
- A single known part now survives area decoding and the ordinary reader.
  Repeated refs do not add members, unknown refs are still discarded, and
  singleton areas participate in the same conflict check as larger areas.
  Regression cases reproduced both the refusal and the missing saved area
  before the fix. The prompt leaves useful area boundaries to the model.
- Reading, GroupsIndex and report tests pass; reading vet and the owner binary
  build pass. The Airflow online run remains in progress.

## 2026-09-14 — Airflow exposed annotated underscore parser failure

- The ordinary Apache Airflow run at revision
  `1f5d7691663016c898718768991a2dae522b4475` selected 453 targets. Several
  Python preparations failed before publication: `_remove_kwargs(_: object)`
  in `airflow-core/src/airflow/api_fastapi/core_api/datamodels/trigger.py:27`
  exposed a declaration skipped as `_`, then accessed by its annotation.
- The cumulative Python fixture reproduced the numeric KeyError before the
  fix. Underscore now retains its ordinary Python declaration; annotated
  parameters and call results keep possible methods, while untyped parameters
  remain unresolved. No Airflow-specific filter or invented receiver was added.
- The complete Python parser suite, cumulative Python contract, corresponding
  TypeScript/JavaScript parameter cases, Clojure local callback and Go blank
  identifier checks pass; changed packages pass vet and the binary is rebuilt.
- The interrupted run exited 130 and is not publication acceptance. Its nine
  cache entries were moved into the existing default system response cache;
  the old cache directory is only a link for saved diagnostic references.
  The restarted full run used the default cache and official DeepSeek. It
  parsed Airflow core successfully (105,059 objects and 354,875 relations).
  The owner then explicitly narrowed acceptance to the Airflow CLI/core,
  browser UI and Task SDK library. The full run was stopped with exit 130;
  the ordinary three-target run is in progress, not yet browser acceptance.
- Before the requested push, canonical `make test`, `make vet`, `make ui-test`
  and `git diff --check` pass. Native Clojure `.cpcache` directories are ignored
  alongside other generated build outputs.

## 2026-09-14 — Visible zoom entrances on closed frames

- Replaced the faint expand corners with a small magnifier-plus control on
  every closed component, area and external collection. It follows the actual
  frame's top-right corner, not the limited text column, and retains a 28px
  hit area and 18px symbol while zooming. Open frames and leaves have no icon.
- Area summaries reserve title space beside the icon. Narrow external titles
  wrap instead of losing the destination suffix to an ellipsis.
- Browser inspection confirmed 12px corner insets and a 28px control on all
  three root summaries. The front icon opens its contents; the Application
  shell and navigation icon opens the group's parts and reading panel. Back
  restores the identical component camera. Existing smooth focus is reused.
- Saved Python rendering makes zero provider requests. All 23 web tests,
  generated-asset check, report tests/vet and owner binary build pass.

## 2026-09-14 — Group names in component summaries

- Distant component cards list every saved area name instead of a bare group
  count. Each name opens that exact area through the existing smooth focus and
  history path. Parts and inputs remain a secondary count when space permits.
- Text measurement reserves the group list before optional role, counts and
  purpose; short cards omit prose rather than cutting off a line. Dense lists
  retain all names in a scrollable region. World coordinates stay unchanged.
- Saved Python rendering used zero provider requests. At the normal 934×992
  browser size, all four front and both backend names fit without scrolling;
  one zoom step also retained both complete lists. Canvas rendering opened its
  three parts and reading panel; Back restored the identical camera transform.
- All 23 web tests, generated-asset check, report tests/vet and make build pass.

## 2026-09-14 — Final small-repository navigation audit

- Questions remain one vertical list of open topic headings and all their
  questions. Python question 3 opens directly; Back restores the list. There
  is no duplicate question grid or nested topic chooser.
- A 934 px browser exposed narrow root frames and a clipped first child on
  component entrance. Root summary width is now reserved after the actual
  area-summary heights are placed. Layout selection considers both root
  dimensions; short summaries retain a three-line purpose. Entering front
  focuses its first content at readable scale, with the component title kept
  in the visible horizontal part of its frame. Pan/zoom never rearranges nodes
  or routes. Browser All → front → Back restores identical viewport text.
- Home/All clears the visible Find query after recording the new visit. Empty
  group readings now say no connections to other parts were found. Python's
  remaining isolated Import retry, Sleep utility and Instruction labels have
  no observed external consumers; the retry self-call is internal. We do not
  invent links or infer dead code from an absent mapped relation. Browser
  Sleep utility exposes both the missing connection and missing input path.
- The ordinary online Othello audit completed in 325.546 s. The final owner
  binary completed its ordinary repeat with exit 0 in 5.527 s, reusing exact
  requests: `/private/tmp/repomap-goal-audit-20260914/20260913-223634-othello-2a9cc894b339`.
  Its complete manifest/report, native, places, atlas and GroupsIndex chain
  exists; the graph digest matches the first audit. It has 25 parts (24 core),
  8 inputs and 16 questions. Native declarations include 173 functions and 32
  vars, with no nominal types: board.cljc:27 returns a vector; game.cljc:5
  returns a map. These values are explained by Board model/Game state, not
  synthesized as type declarations. External symbols/imports are not runtime
  communication records, so External communication is absent from its legend.
- Othello browser: Board model is purple, has exact code and incoming sources;
  Handle board click selects its six reached parts. Python's previously
  inspected Run → backend → Robot write paths, GET exclusion and exact external
  POST caller remain unchanged. No native or model topology changed in this
  layout pass. The original audit records seven refusals (documentation,
  single-part areas, an unsupported orientation ref and optional glossary).
- All 23 real UI tests, generated-asset check, report tests/vet and make build
  pass. Python and Othello previews use ordinary saved rendering with zero
  provider requests. The question list remains open for the owner.

## 2026-09-14 — JSX and value uses retain their original connections

- JS/TS now publishes compiler-bound value and JSX tag references as original
  reads. Aliases, reexports, shorthand values and local shadowing retain their
  declaration identity. Pure writes, type-only syntax, unknown bindings and
  unindexed callback bodies do not borrow a runtime reader. JSX does not
  fabricate a synchronous call. Property and receiver uses sharing a source
  column retain separate relations.
- Real native testing exposed an older per-file rank cutoff: module activity
  and values after ten other declarations disappeared, and later types could
  not receive descriptions. Removed that cutoff; ranking only orders eligible
  authored declarations. The saved complete Python graph adds nine original
  declarations and admits seven existing types, losing none. These include
  robot direction values, drawing coordinates, Step, Wall and RunResult.
- Correction to the preceding native-test notes: the default Node 24 lacks a
  TypeScript compiler, so JSTS tests had silently skipped. The actual suite now
  runs with the prepared Node 21 compiler. The cumulative mixed fixture enables
  JavaScript explicitly, and its iteration check correctly expects possible
  JS dispatch. Full JSTS, places, contract, GroupsIndex, reading, report, Python
  and Clojure checks pass; native value-read/shadowing equivalents were also
  run explicitly for Python and Clojure. Focused vet and make build pass.
  Native TypeScript 7 is not prepared; general Go variable reads still have
  no producer equivalent. No fallback relationship is invented for either.
- Final ordinary online run exits 0 in 27s:
  `/private/tmp/repomap-iteration-20260914/20260913-220335-python-tutorial-game-0c7c8ba4ff97`.
  Both target chains are complete: Python 219 objects/738 relations, frontend
  259 objects/730 relations. The common GroupsIndex report has 27 parts, all
  16 questions answered, three original outgoing HTTP observations, one owner
  manifest/report JSON/physical HTML, and the complete saved reading chain.
  Seven design and one glossary refusals remain explicit. Exact accepted
  requests were reused; this was an ordinary run, not replay.
- Browser: Find Editor opens Code editor and its Playground JSX use at
  playground.tsx:110 → editor.tsx:19. The peer visit and Back retain the camera
  and expanded source evidence. Find fieldColor exposes draw.ts:117 →
  constants.ts:1, plus its reaching animation/resize inputs. The final report
  preserves the question → Run → backend/robot-write reading → Back journey.
  Its question menu is eight open topic headings with sixteen visible links;
  no question grid or collapsed topic choices. Updated python.html through
  saved render with zero provider requests and left Questions open.

## 2026-09-14 — Module activity and internal code connections stay readable

- Observed module-body calls, executions, external invocations, reads and
  writes now give the normal architecture request a source-located module
  declaration choice. Import/containment-only modules do not. Selecting that
  body does not assign other declarations in its file; their accepted choices
  and refusals remain independent. The prompt explains the distinction.
- Real cumulative Python module registration, JS/TS module calls, Clojure
  namespace evaluation and Go's callable main equivalent pass through native
  evidence and places. Places, GroupsIndex, reading, contract, JSTS, Clojure
  and report tests pass; focused vet and the owner build pass.
- The ordinary online two-target run finished with exit 0 in 4m5s:
  `/private/tmp/repomap-iteration-20260914/20260913-211836-python-tutorial-game-af4f98e3c6f9`.
  Both targets are analyzed and retain their complete native/GroupsIndex
  chains; the common atlas, report JSON, manifest and one physical HTML exist.
  All 16 questions have answers and all three integrations survive. Sixteen
  design refusals retain thirteen single-part-area decisions and three
  unassigned styled declarations; no local membership is invented for them.
- Browser: Server launch exposes the four original settings reads at
  main.py:17–20 and settings.py:15. Visiting Application configuration and Back
  restores the identical camera and expanded evidence. The normal model puts
  index.tsx and reportWebVitals together in Application bootstrap.
- That last journey exposed a presentation omission: same-part relations were
  kept in GroupsIndex but absent from the source reading. The ordinary group
  card and sidebar now expose those exact relations under Connections within
  this part. No self-arrow or new relation is added. Browser Find reportWebVitals
  now opens the original index.tsx:19 → reportWebVitals.ts:3 call, with both
  revision links. A report test preserves same-line occurrences, resolution,
  original sites and static reading, excluding containment and other members.
  Long source links wrap within the sidebar.
- The owner's question-list request remains one vertical series of open topic
  headings and visible questions. Consolidated its styles in the question
  stylesheet; Python question 7 opens with one click and Back returns to the
  same list. No duplicate question grid or topic disclosure remains there.
- Remaining native gaps: JSX component uses and general JS/TS variable reads
  still do not supply their ordinary dependency relations. Unused helpers and
  refused memberships remain distinct from those missing observations.

## 2026-09-14 — Typed iteration restores the normal robot step

- Python lost the element receiver in `for robot in sorted(self.robots)`, so
  the report showed rollback writes but omitted normal movement. Direct known
  collection annotations now retain a possible element class inside a
  synchronous loop. Calls and field writes preserve their native source sites.
  Unknown/replaced collections and receivers, shadowed sorted, heterogeneous
  tuples, unions, asynchronous iteration and post-loop uses stay unresolved.
- Cumulative Python native tests, real Go range and TS/JS for-of equivalents,
  full contract/JSTS packages and focused vet pass. Clojure's corresponding
  Java instance dispatch has no current native equivalent. The owner binary
  completed the ordinary online two-target Python run with exit 0 in 4m29s:
  `/private/tmp/repomap-iteration-20260914/20260913-205812-python-tutorial-game-5838b558b297`.
  Both native/GroupsIndex chains and the common atlas, manifest/report JSON
  and one physical HTML exist. Three integrations survived. Seven design
  refusals concern single-part areas/missing membership; an identifier-only
  glossary entry was also refused. These do not invent replacement membership.
- Browser: Robot movement lists forward prev_x/prev_y/x/y writes at robot.py
  40–43 for backend POST and front Run. The x path opens as handleClick →
  runLevel → possible HTTP integration → run_level → Field.make_step →
  Robot.make_step, with original links. GET /api/levels reaches HTTP endpoints
  and Level data, with no Robot writes. The exact external POST identifies
  handleClick at playground.tsx:72 as its caller and Run as its reaching input.
  Back restores the identical Run camera and reading.
- That journey exposed a separate focus bug: a hidden child's world bounds
  fitted the overview, so search opened its sidebar without revealing it.
  Camera preservation now requires an actually visible, legible destination.
  Browser Find Robot movement now opens the detailed purple card; all 22 UI
  tests, report tests/vet, bundle and owner build pass. Saved render makes zero
  provider calls.
- Remaining isolation audit: native module-level calls/reads can have no
  selected architectural member (index.tsx → reportWebVitals; main.py →
  settings). JSX component uses and general JS/TS variable reads have additional
  native gaps. These are distinct from unused helpers; no missing edge is
  inferred from a shared path or group name. The broader isolation work remains
  open.

## 2026-09-14 — One question list, header navigation and whole-map entrance

- Questions are one vertical list: saved topic headings and immediately visible
  questions, with a fallback for answers absent from the topic plan. Removed
  the duplicate grid, nested topic choices and input catalogue All disclosures.
- Header: reviewed repository link and house icon on the left, repomap/GitHub
  link on the right, one Find field and a component list. All shares the
  whole-map action. Nested repository links retain their directory and revision.
- Removed changing hover prose above the canvas and the large Zoom in buttons.
  A stable zoom/drag hint and small expand marks expose the action. Input
  context and close-details chevron live in the reading card. Search excerpts
  are short plain text, without model/source popovers; exact code names rank
  before matches in answer bodies.
- Whole-map entrance fits every root frame and refits its available space;
  manual cameras restore exactly. Short-window checking exposed the old .15
  lower zoom clamp: the minimum now permits the actual root bounds. The fixed
  world reserves external-summary space and chooses an orientation that also
  accounts for readable root headers. No layout runs during wheel zoom.
- Outbound reading exposes native callers of the exact sending callable,
  preserving original parts, possible-call status and source pairs. It does
  not attribute every method in a shared client group to the same caller.
- Saved Python browser journeys: grouped questions → question 7 → Field
  simulation → Back restores the answer; front → All → Back restores the exact
  camera; Find config returns Config first, a map visit returns to the same six
  compact results; POST → Playground orchestration → Back restores its source
  reading. Checked ordinary 934×992 and temporary 1280×720 browser layouts.
  Othello also opens its component from the list and exposes all question topics.
- Report tests/vet, 21 frontend tests, owner build and saved rendering pass.
  UI rendering used existing accepted report/translations with zero provider
  calls; the ordinary online variable-read run is recorded below. Broad
  pointer-hover journey acceptance is not claimed from click-only browser QA.

## 2026-09-13 — Connection reading, core colour and source-backed data reads

- Core uses its saved lane for purple cards and overview rows; both legends
  include only present categories. Reading and hover use a single contour.
  Removed the outline of the whole scrolling inspector and renamed Key code
  to Code in this part. Outgoing connections precede incoming connections and
  code; native relations remain inspectable without a model sentence.
- Browser on the saved Python report: Field simulation exposes all four
  Robot movement calls with exact sources. Peer navigation and Back restored
  the identical viewport and expanded evidence. Core fills are purple; node
  outlines/shadows and the inspector outline are absent.
- Python now emits declared-variable reads with lexical shadowing, import and
  receiver controls. Input paths include terminal data reads with their native
  site; reading a value, callback or class declaration does not activate effects.
  Cumulative Python + GroupsIndex checks and a native Clojure var-read
  equivalent pass. General Go/JS variable reads remain unavailable; JS/TS's
  existing type/contract reads are narrower evidence.
- Ordinary online owner-binary run exited 0 with both targets and three
  integrations: `/private/tmp/repomap-variable-reads/20260913-193510-python-tutorial-game-727901bff0c4`.
  Both target chains exist with one common manifest/report JSON/physical HTML.
  Reads bind all three backend requests to levels and POST to the source-size
  constant. Browser: GET /api/levels → Level data exposes get_levels_info →
  possible read levels, its declaration and the original read at app.py:21.
- Removed the introductory model-source popover; citations and attribution
  remain below the canvas through Repository summary sources. Othello's
  expanded legend contains no external category.
- Othello core investigation: its accepted Board representation and Game state
  groups exist, but the type-reading stage describes nominal declarations.
  Functional vectors/maps returned by functions supply no nominal type card.
  Recorded this general limitation; no Othello-specific inferred entity or
  mutation was added.
- Focused report/Python/Clojure/contract tests, scoped vet, 17 frontend tests,
  owner build and saved rendering passed. Multi-component top-level zoom and
  general functional-value entity interpretation remain open.

## 2026-09-13 — Reuse external catalogue groups on the canvas

- The system map now consumes the same outbound display grouping as the
  external catalogue. Each destination has one frame and keeps every original
  selectable call, source and input path. The frame is a display collection,
  not a newly inferred component or target.
- Matched peer links bind by integration identity and the exact caller and
  call location, including column. Duplicate structural/path projections lead
  to the same backend input; matching labels never establish an integration.
- Saved Python browser journey: backend API contains all three HTTP calls;
  POST opens the backend POST input, and Back restores the exact call camera.
  Parameterized GET retains its native method/path. The frame sidebar lists
  every record and its originating inputs; empty code/explanation actions
  were removed.

## 2026-09-13 — Python field writes and input evidence

- Source-ordered Python receiver origins resolve attribute writes to existing
  declared fields as alternatives, retaining exact write locations through
  GroupsIndex. Aliases, declared receivers and constructor results are covered;
  rebound, unknown, nested and dynamic writes stay unresolved.
- Input reading lists reached writes with their entity, field, source site
  and shortest call witness. Entity/part reading links back to those inputs.
  Exact matched peer inputs carry the originating frontend witness and its
  uncertainty; unrelated sibling inputs do not inherit effects.
- Cumulative Python fixture: 14 distinct write observations, nine candidate
  bindings, negative ownership/reassignment controls. No equivalent bound
  field-write emission exists in the current Go, JS/TS or Clojure adapters;
  those remain missing, not inferred from part reachability.
- Ordinary online acceptance, owner binary: `python-tutorial-game` completed
  with exit 0, two analyzed targets and three cross-target integrations.
  Run: `/private/tmp/repomap-entity-writes/20260913-190514-python-tutorial-game-40fdbc785aac`.
  Both native/program/group chains exist, with one common manifest, report JSON
  and physical HTML. All 13 bound writes retain their source locations.
- Browser: backend POST and frontend Run simulation expose the same eight
  reachable writes; Robot reading lists both inputs and only its three writes.
  Expanded frontend witness reaches `robot.py:46` through the HTTP call,
  possible integration and backend call chain. This does not establish that
  every write executes on every run. Functional Clojure value entities remain
  unresolved acceptance work.
- Focused report, Python, GroupsIndex and contract suites, 17 frontend tests,
  scoped vet and the owner binary build passed.

## 2026-09-13 — Align the report shell with the canvas

- Removed the independent centred widths of the sticky toolbar and question
  reading. Header, toolbar, canvas and answers now share the page gutter;
  prose retains its reading measure. Removed the redundant canvas-only width
  override.
- Python browser check at 2250 CSS px: toolbar moved from x=405 and questions
  from x=581 to the canvas gutter x=20; no horizontal document overflow.
  At the ordinary 934 px viewport, questions → answer → Back → Back to map
  kept one mounted canvas and placed the destination below the toolbar.
- Report tests/vet and `make build` passed. Both saved Python and Othello
  reports rendered successfully with zero provider requests.

## 2026-09-13 — Fixed zoom geography and repository reading entrance

- Automatic detail now changes contents inside fixed area rectangles. The
  second overview graph/layout was removed. Original directed routes are
  clipped only at closed area boundaries and continue to their actual parts
  on zooming in. Entry/exit scales differ to avoid boundary flicker.
- Live checks exposed and fixed swapped asymmetric minimum dimensions in ELK's
  vertical layout, initial cameras over empty frame corners, and diagonal
  clipping caused by tiny fractional differences on vertical segments.
- An unselected sidebar now links to questions, run material, terms, missing
  observations and author claims, with component purposes and original sources.
  Long reading stays below the mounted canvas. Closing details restores this
  entrance at its top instead of leaving a mostly empty column.
- Othello: View model → zoom out through automatic summary → retained all world
  coordinates and selected reading; no diagonal routes after the fix. All
  summary content fits. Run topic → desktop answer with 20 source links → Back
  twice restored the exact camera and home reading.
- Python/TS: external POST → Run → Source code validation retained the four
  source steps and their uncertainty. Back twice restored the exact external
  camera and reading. No summary overflow; present-type legend unchanged.
- This is small-repository browser evidence, not acceptance of the remaining
  multi-component top level or value-entity mutation analysis. No new provider
  requests were made. CURRENT was not read or changed.
- Final checks: 17 frontend tests, embedded-asset consistency, report tests,
  vet and owner binary build passed. Both saved renders exited successfully;
  final Othello browser had no console warnings/errors or summary overflow,
  and every home-reading link resolved to its existing report section.

## 2026-09-13 — Cross-component input paths and reverse reading

- The common map now continues an input through exact matched peer inputs,
  keeping original edge authority, cycle protection and source witnesses.
  A reached part never activates its other operations.
- Outbound records list inputs that reach their original caller subject;
  sharing a part, importing or reading the callable is insufficient.
- Part/outbound sidebars show grouped reverse input links. Redundant related
  operation and raw evidence disclosures were removed from these cards;
  the saved call-path explanation stays visible. The toolbar drops the
  type/style switches, and the legend contains only present categories.
- Clojure investigation: saved Othello has 173 functions, 32 variables and no
  type declarations. Board is a vector and Game a map returned by functions.
  The existing native-type concept stage cannot name those value entities.
  Python records unresolved attribute writes without target objects; these
  reports have no resolved write edges. Reaching a part must not claim mutation.
- Saved online Python/TS render: Run reaches 10 participants including the
  backend validation/simulation and its POST communication. The validator's
  four-step witness starts at PlayGround.handleClick, continues through
  runLevel and a possible integration to run_level, then possible validate.
  External POST lists only Run; external → Run → backend → Back twice restores
  exactly translate(-1410.5px,-1214.5px) scale(1) and the external reading.
  Othello shows Parts/Inputs only. Automatic semantic zoom and value-entity
  effects remain unaccepted; this probe does not close the full goal.
- Validation: focused report tests and vet, 12 frontend tests with embedded
  asset consistency, owner binary build, both saved renders and browser console
  checks passed. No new provider requests were made for this report change.

## 2026-09-13 — Named inputs, complete reading and return journeys

- Compact areas now show every named input, grouped once by its saved activation
  type and exact owner. Saved core/entry/dependency labels remain visible.
  Overview and detailed cards share their content and premeasured heights.
- Zoom no longer swaps layouts mid-gesture. Explicit area opening shows the
  area's title; reset keeps the current level. Programmatic camera changes
  commit history after completion. Back's second hash event cannot reopen and
  recenter a restored visit; opening the same part from overview retains a visit
  even with the same URL. Inspector expansions/scroll survive reconstruction,
  separately for the pinned input context. The initial hashless visit now saves
  its completed overview camera; Back clears the later reading and restores
  that initial overview instead of retaining a stale part in the inspector.
- Group readings consolidate repeated connections by exact peer and direction:
  one saved summary per relationship, all original evidence beneath it. Position
  evaluation's 10 evidence rows are three peer groups (2/3/5), with no loss of
  source pairs. Key code is visible immediately, duplicate full-group actions
  and vague All disclosures removed. Input participants and connected inputs
  are navigable; their original call-path explanation remains available.
- The common canvas stays mounted above answers and reference sections. The
  component sidebar links to existing flow, configuration, data, core,
  dependencies, dynamic execution, coverage and TODO material. Removed the
  two introductory labels and the duplicate catalogue inserted below the map.
  All operation and outbound catalogue rows remain visible within their groups.
- Small-repository browser journeys: Othello overview -> zoom/pan -> part ->
  Back/reload retains exact camera; detail -> overview -> same part -> two Backs
  restores both visits. Pinned Track pointer position -> View model -> hover its
  area highlights 11 edges/6 labels; moving into the sidebar restores the empty
  input path without replacing View model's reading. Python/TypeScript Run ->
  HTTP service -> backend POST -> validation -> question/map return preserves
  source-backed explanations and camera. Component -> dynamic execution exposes
  backend/app/field.py:98 in the reading below the same map.
- Focused report tests/vet, 12 frontend tests, generated-asset check and owner
  binary build pass. Saved online reports 20260913-113850-othello-d98e15694e4a
  and 20260913-113853-python-tutorial-game-7e7249a73524 render with zero provider
  requests. These are small-repository UI checks, not acceptance of a new
  multi-component semantic zoom or of the large-repository layout.

## 2026-09-13 — Separate reading, hover connections and zoom overview

- Node hover no longer replaces the selected inspector with a smaller duplicate.
  Clicked details and links stay mounted; connection evidence uses its own section
  and survives moving from its label into the reading column.
- One emphasis reason drives nodes and edges together. Area hover temporarily
  replaces a pinned path and clearly names/frames the area; leaving restores the
  path. An off-path inspected part stays outside that path. Search results do not
  inherit old highlighted edges. Removed the Selected colour legend and purple
  state recolouring; input/external type colours remain stable.
- Added a compact ELK overview using only saved areas, complete part membership,
  input counts, original directed relations and component ownership. Detailed
  card positions remain fixed. Zoom/click moves between display levels in the
  same canvas; history includes the level and both geometry identities. Overview
  keeps readable type and starts at the top/left in a narrow window.
- Browser journeys: input Track pointer position → Test helpers (explicitly outside
  its path) → hover area (4 edges) → right panel (0 path edges restored) → To code
  → Back to map. View model → hover UI presentation logic (6 grouped labels) →
  connection sources (14 source links) → move into inspector → To code → Back.
  Full details remain present throughout hover. Clear selection clears the heading.
  Detailed inventory remains Othello 14 parts/8 inputs and Python+TypeScript
  25 displayed cards/12 inputs/3 outbound records; all 50 detailed routes remain
  orthogonal and do not pass through leaf cards.
- UI vocabulary verification now checks authored frontend sources as well as
  templates; it catches a missing message before runtime. Eleven frontend tests,
  generated asset check, focused report tests, vet and owner binary build pass.
  Both existing saved online runs were rendered with zero provider calls.

## 2026-09-13 — React Flow + ELK system canvas

- Replaced the system SVG viewport and custom boundary route planner with React
  Flow and ELK's compound part-to-part routing. The adapter consumes existing
  report IDs, owners, operation paths and original relations. It adds no analysis
  stage. Legacy small diagrams reuse the same bundled ELK code.
- All bound inputs are named blue cards inside their exact implementing part;
  unbound inputs remain nodes. External observations are amber cards with native
  labels/addresses and their original sources. Component frames show language,
  kind, role and purpose. Neutral structure and purple selection replace the
  indistinguishable green tones. Input inventory: Othello 8, Python/TypeScript 12;
  the latter retains 3 independent external observations. No card text overflow
  was found in those rendered inventories.
- A reserved reading column replaces floating node previews. Numbered connection
  labels group outside identity + direction and every inner endpoint; hover
  reads original sources, number click pans to the outside part. Labels persist
  across their gaps and clear on workspace exit. Lines are noninteractive and
  have no native tooltip. Selection keeps node positions/sizes and camera;
  explicit search/destination navigation moves to the exact card.
- Browser verification found stationary-pointer previews after clicks/navigation,
  initial fit racing saved camera restoration, and stale ELK bends when comparing
  orientations. Fixed these on the ordinary path. Each ELK candidate now receives
  a fresh graph; regression exercises the real mutating library. Final rendered
  geometry: 32 Othello + 18 Python/TypeScript edges, no diagonal segments or paths
  through leaf cards. Input click, grouped destination/Back and question 5 → Run
  → backend validation → code → reload → map → question were walked in the browser.
- Developer-only `make ui-build` compiles checked-in JS/CSS; `make ui-test` runs
  6 tests and verifies generated assets. All pass, as do focused report tests,
  vet and `make build`. A local `go install` with Node absent from PATH completed
  and rendered Othello. Go/npm use their ordinary shared caches. No external
  script assets are requested by either generated report.
- Final UI renders reuse completed online runs 113850/113853 and their saved
  translations, both exit 0 with no provider calls. Preview is the actual generated
  HTML at loopback 8774. Details and input-method limits are in the UI review.



## 2026-09-13 — Boundary grouping and transient hover

- Owner screenshots exposed crowded Domain core labels and annotations that
  survived pointer exit. Previous verification of one convenient area did not
  cover these cases. Boundary groups now use outside identity plus direction,
  displaying every inner endpoint number once beside a single outside name.
  All original calls and source pairs remain in the group's inspection.
- Measured labels, marker hit areas and area titles reserve space; routes avoid
  them and replace the corresponding overview bundle. Extra annotation space is
  part of the initial drawing bounds. SVG edge paint order no longer changes
  which saved relations are highlighted. Selection cannot revive hover labels;
  empty canvas, exit and scroll clear them. Prose width no longer limits the canvas.
- Browser checks covered Othello's dense Domain core (9 groups), UI presentation
  logic (6 groups), frame/part/marker/empty-canvas/outside transitions, scroll,
  grouped source inspection and the Python/TypeScript validation area while a
  Run operation remained selected. A 1600 CSS-pixel window has a 1535-pixel
  canvas and drawing. Final exit leaves no temporary numbers, ports or tooltip.
- Focused report tests (including dense collision and pointer regressions),
  vet and ordinary build passed. Both 113850/113853 saved runs were rendered
  through the ordinary command with zero provider requests. This is a UI-only
  iteration on those previously completed online runs, not another online run.
  Detailed input-method coverage is recorded in repomap-ui-ux-review.md.

## 2026-09-13 — Common canvas and boundary labels

- The ordinary home report now assembles all existing component maps into one
  canvas after translation. Exact destination links join remote copies; saved
  membership, input ownership, native HTTP registrations, outbound records and
  unread components remain represented. Selection changes emphasis and the
  reading below the map, without expanding code lists or relaying out the scene.
- Operations highlight their saved path on the common canvas, including other
  components. A single-part operation stays in that part. Boundary markers
  match local inner-part numbers and name outside participants, retaining the
  earlier directed layout and full-width canvas. Marker click opens
  original relation evidence. Internal arrows remain ordinary arrows.
- Browser checks covered Othello View model → Quil painting boundary marker
  → exact source evidence, plus Track pointer position.
  The owner rejected the intermediate grid/number-strip design with an annotated
  screenshot; that display was removed, and the original directed layout restored. The canonical question 5 → Run simulation on click
  → backend HTTP API endpoints preserved identical node geometry and question
  context. To code → reload → Back to map → Back to question returned to question
  5 with the operation retained. Connection style survives reload too.
- Focused `internal/report` tests and vet, `make build`, and both saved renders
  passed. Ordinary Othello `20260913-113850-othello-d98e15694e4a` exited 0 in
  4.928 s; canonical Python/TypeScript `20260913-113853-python-tutorial-game-7e7249a73524`
  exited 0 in 10.330 s. All model responses reused the shared cache. The complete
  common artifact chain and each target's native/group artifacts were inspected;
  native facts, semantic atlas and common report graph match the prior accepted
  inputs (run-local facts references and cache accounting differ). The two-target
  publication has one manifest, report JSON and physical HTML.
- This records the working display experiment and its verification, not final
  owner approval of the visual design. No analysis cache or provider contract
  changed; CURRENT was not consulted or modified.

This is a concise living log, not another ADR. [CURRENT](CURRENT.md) owns the
current decision/status; [topical contracts](../../AGENTS.md#start-here) own
implementation details. Record a changed decision there in place. Append only
useful implementation/acceptance evidence here; archive superseded detailed
runs instead of growing the entry pages again.

## 2026-09-13 — Responsibilities instead of directory boxes

- Replaced file `here`/sibling/`new:` placement and fixed-count zone naming
  with aggregate declaration membership and areas in the ordinary atlas stage.
  The owner receives declarations, docs, native calls, values and activation
  context. Parts can cross directories and split one file; native relation
  endpoints and lexical ownership survive projection. Atlas v8 records exact
  members. No parallel analysis path, browser semantic validation or new cache
  was added. Conflicting memberships are refused together; independent groups
  survive and missing choices remain explicit inventory.
- Regressions exercise same-file separation, cross-directory collaboration,
  singleton collaborators, conflicting/unknown refs, retained lexical children
  and native cross-part source anchors. The saved Othello complete-window probe
  covered all 205 declarations and six author contexts (121,734 prepared bytes
  before the final behavioral-responsibility prompt clarification).
  Full `make test`, `make vet` and `make build` passed with ordinary Go caches
  and the existing nvm Node 21 TypeScript installation.
- Ordinary online final Othello run
  `20260913-102848-othello-055f2b24716e` exited 0 in 4.279 s, all model requests
  reused. Relative to the accepted 084650 run: 5 → 16 parts and 10 → 169
  cross-part native connections; all 354 subjects and 3,213 structural edges
  and the native index hash are unchanged. The visible product map has 14
  parts and 157 connections; two test-only groups remain in the complete data.
- Canonical ordinary online baseline
  `20260913-094041-python-tutorial-game-152990d369d2` exited 0. Final
  `20260913-102843-python-tutorial-game-ea9674967cc8` exited 0 in 35.989 s.
  Backend: 2 → 9 parts, including source validation and sensor initialization;
  frontend: 6 → 13 parts, including HTTP service, editor and simulation UI.
  Both native hashes, all subjects (219/259) and structural edges (567/679)
  are unchanged. Three native HTTP operations remain. The refreshed model
  selection marks Rootpage and LevelPage `operation_candidate=no` rather than
  separate interactions; both remain key code and participate in the flow.
  Othello adds the interpreted button-click interaction (7 → 8).
- Inspected complete owner/sibling artifact chains and matching published
  GroupsIndex hashes. Each run has one common manifest, report JSON and physical
  HTML; all targets retain their native index and dependency catalogue.
  Browser walkthroughs used loopback reports on 8772/8773: validation question
  → backend validation part → `app.py:90` / `utils.py:18`; Othello click → six
  collaborating parts, retaining eight scenario connections after opening a
  part and reloading. Root areas show their parts and internal arrows directly;
  the layout uses full page width and keeps all folded source relationships.
  A final UI-only correction routes full-code links through reading history:
  question → map → code → map → the original question now retains its named
  return. Focused report tests/vet passed; both final saved reports were
  rendered again with zero provider calls and the journey passed in the browser.

## 2026-09-13 — Clojure JVM and glossary reading

- Added the registered Clojure JVM adapter through native clj-kondo EDN,
  ProgramIndex and ordinary reading/reporting. The cumulative Clojure fixture
  and exact inventory cover imports, direct and shadowed calls, callback
  argument binding, reader syntax, Java uses and exact author docstrings.
  Focused corpus, claims, places, adapter, run, contract and report tests and
  vet passed. Existing native JSTS equivalents passed using the owner's
  TypeScript installation in nvm Node 21; Go and Python cumulative coverage
  passed in the contract suite. ClojureScript execution and Scala are not added.
- Installed the normal Clojure CLI and clj-kondo environment; retained ordinary
  caches and Java 21, and installed Java 25 required by current Metabase.
  Repositories live in `~/git`. Othello's actual specs passed: 143 examples,
  507 assertions, zero failures. Its desktop dependencies and Metabase's run
  dependencies resolved; Metabase's bootstrap namespace loaded on Java 25.
  This is not a complete Metabase application build. The optional complete
  Metabase native probe reached its root's 4,035 files and exceeded the
  five-minute test timeout; full Metabase analysis remains unaccepted.
- Ordinary online Othello run `20260913-083717-othello-56735348db67` exited 0
  in 260.652 s. Final EDN repeat `20260913-084650-othello-82aa91ac38fe`
  exited 0 in 4.427 s through the shared accepted-request cache, with zero
  live provider calls. Inspected the manifest, shared program facts and index
  set, dependencies, reduced documentation, places, atlas, tables, GroupsIndex,
  analyzed target outcome, report JSON and the single HTML. Browser reading
  followed desktop/browser launch instructions, original sources and UI flow.
- Othello exposed glossary context rendered as hundreds of apparently direct
  citations: AI had 834 locations across 28 files; rules had 796 across 28.
  Domain entries now label this saved provenance `Analysis context`, collapsed
  by default; file paths appear once, locations require opening the file.
  Questions are a separate collapsed reading choice. `(via model)` sits in
  the preview heading, outside the definition. Original destinations remain
  in static HTML. Report tests and vet passed; saved rendering updated both
  accepted Othello HTML files with zero provider calls. Browser walkthrough
  verified AI context, README permalinks and a linked question; details are in
  `repomap-ui-ux-review.md`.

## 2026-09-11 — current correction wave, ordinary acceptance pending

- The exchange journal records `cached_input_tokens` (DeepSeek's
  `prompt_cache_hit_tokens`) and `reasoning_tokens` beside the token
  counts, so a run shows whether lead-first windows over shared evidence
  were served from the provider's prefix cache and how much of the output
  was reasoning; the probes on the saved Freqtrade window had to infer both.
- Learn re-asks the intents a response left out and lists the intents
  after the evidence (learn v5). The owner's run (524,288-token provider)
  split Learn into four windows: two answered only the first intent
  (`purpose`) and stopped, two answered nothing, so seven intents were
  "missing intent review" with no other entries in the response and the
  report had two questions. Like retrieval, an intent with no entry in an
  accepted response is now `intent_omitted` and re-asked over the same
  evidence, first together with the other omitted intents, then one per
  window, before it is unavailable; a refused first window goes straight
  to single-intent windows. The intent catalogue moved from the system
  prompt to `"intents":[…]` after `evidence`, so a re-ask shares the
  request prefix with its parent window and the provider's prefix cache
  serves it; the prompt says `sources` are `e*` evidence refs only (the
  owner's model cited `h1 h2 h4` context refs and lost a question).
- Symbol and operation requests carry one decision per cell and define
  every value they render (schema review, cold fixes; symbol-selection v5,
  symbols v8, operations v20). `activation` (seven options consumed as a
  yes/no gate) is now `operation_candidate` yes/no; `call_count`/
  `limit_from` (an unreachable check) are gone; `invocation: synchronous`
  and `resolution: exact` are left out as defaults and exact local callees
  collapse to one `local_calls` line; origin trees (`source_arguments`,
  `receiver_value`, `result_value`) render as `kind`/`text`/`parts` two
  levels deep without anchors or owners (they were 32 % of the Morfeu
  orientation request and 90–95 % of an operations row); `name_kind` is
  asked only when a registered name exists (`[label]` was the single
  option in 16 of 18 rows), null fields are not rendered, the `u*`-era
  paragraph is gone. A shared `evidence-vocabulary.md` defines every
  rendered `invocation`, `resolution`, origin kind, witness kind,
  extractor and control label and is attached to the symbols, operations
  and boundaries prompts; a contract test renders every Go fixture row and
  fails on a value the prompt does not name. The response example puts
  `key` first, then the cells in fill order. `Column.Unasked` names the
  value an unasked conditional cell takes, apart from `Missing`.
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
