# Implementation and acceptance journal

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
