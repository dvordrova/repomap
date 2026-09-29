# Implementation and acceptance journal

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
