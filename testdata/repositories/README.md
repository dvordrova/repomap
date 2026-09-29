# Cumulative language repositories

This directory contains exactly one small real repository for each language
covered by repository-discovery and ProgramIndex regression tests. A future
repository-dependent regression extends the existing repository for that
language only after owner approval; it does not create one repository per bug.
Every tracked file has an exact inventory entry under `testdata/contracts`.

Every language-cube behavior change must add or extend a source example here
and its executable expected result. Exercise the real extractor and adapter
through the boundary whose behavior changed; a nearby test that does not
exercise the change is insufficient. Reuse an existing example when it covers
that exact behavior. Keep contrasting cases for ownership and resolution so
an invented owner, call or relationship fails the check.

A case discovered in one language triggers the same check for its equivalents
in every other supported language. Add or extend comparable examples and
expectations immediately; no separate request is needed. Respect native
semantics instead of copying syntax mechanically. When an adapter already
handles the case, add the regression example without changing that adapter.
If a language has no equivalent, record that explicitly.

The fixtures deliberately contain no nested `.git` directory, generated
binary, product command, helper script, network requirement, or third-party
runtime dependency. Test harnesses may copy a fixture to a temporary directory
and initialize source-control metadata there when tracked-file behavior is
part of the contract.

These fixtures cover deterministic indexing and preparation of the original
declaration evidence for questions. Model-backed behavior such
as batching, closed-ref normalization, cache validation, and grouping belongs
in smaller cube or executor tests unless a separately approved regression
really depends on repository shape. If a future repository test must cross a
provider boundary, it uses an exact request-bound, fail-closed local preset
with no network access.

These repositories are regression evidence, not product acceptance. Ordinary
online runs against real repositories remain the acceptance path.

Current language repositories:

- `go/` proves the ordinary Go orientation handoff through an imported local
  package. Its unused private receiver method remains a ProgramIndex object but
  never gains a DirectCall node that was not observed.
- `python/` proves the complete PEP 621 `src`-layout script chain from target
  discovery through a validated, round-tripped ProgramIndex with one exact
  `main` script seed.
- `jsts/` proves the selected-package TypeScript compiler path for local and
  JavaScript default-library invocations. DOM canvas calls, `Math`, `console`,
  `Date`, `Promise`, and `Image` retain exact platform authority; a class
  construction retains its exact local constructor, while a repository-local
  value merely typed as a platform constructor remains an unresolved frontier.
- `c/` is a small key-value server, `kvd`, and its client, `kvcli`, read
  through clang with the flags `make -n -B` prints. Each link line of the
  [Makefile](c/Makefile) is a program; both link `strbuf.c`, `net.c` and
  `loop.c`, each parsed once. [tools/dump.c](c/tools/dump.c), which no link line builds, is a program
  through its own `main` and links what the linker would take. `loop.c`
  includes the poll backend the build asks for; the epoll backend is outside
  this build on every host and is never parsed. Inside repomap's own
  repository the Makefile is not at the root, so no flags reach clang:
  each `main` is a program of its own, `loop.c` takes epoll on Linux and
  poll elsewhere, and every program still parses.

C function pointers and macros in [kvd.c](c/kvd.c) and [loop.c](c/loop.c):

| Source | Expected result |
| --- | --- |
| `cmdTable` rows `{"get", getCommand, 2}` | each row hands its function over under its own name (`name = "get"`) |
| `cmd->proc(c)` in `processCommand` | alternatives: the six functions the table stores, each with the row that stored it (the source order a tie of the map's sentences follows) |
| `fe->rfileProc = proc` under `if (mask & LOOP_READABLE)` | the call through `rfileProc` is unresolved and names every handler passed to `loopCreateFileEvent`; a branch never makes alternatives |
| `l->beforeSleep = proc`, stored once | the call through `beforeSleep` is exact |
| [staticsyms.h](c/staticsyms.h) `(unsigned long)getCommand` | an address used as an integer: no call and no callback |
| `act.sa_handler = onSignal` | `onSignal` is handed to `struct sigaction` under the field as written, not the platform's internal union |
| `kvAssert(setNonBlocking(cfd) == 0)` | `setNonBlocking` is called at its own column; the `kvAssertFail` call the macro body writes is at `kvAssert` |
| `static void oom` in both `loop.c` and `strbuf.c` | two functions, one per file |
| [net.c](c/net.c)'s `netListen` and `netConnect`, linked into both programs like Redis's `anet.c` | kvd never runs `netConnect`, kvcli never runs `netListen` or `strbuf.c`'s `sbConsume`, and the dump tool never runs `sbConsume`: each is `unreachable` in that program's index. kvd's listener, and the `KVD_BACKLOG` that `netListen` reads, are kvd's alone; `connect` is kvcli's alone; kvcli's page lists `netListen` and `sbConsume` under "Not reachable from the entrypoints" |
| kvcli links [loop.c](c/loop.c), the server's event loop, like redis-cli links Redis's `adlist.c`, and calls none of it | every function of `loop.c` and of the `loop_poll.c` it includes is `unreachable` in kvcli; drawn one part per file, those two parts leave kvcli's map and are listed by their declarations under "Not reachable from the entrypoints", and stay on kvd's map; `loop.h`, whose types run nothing of their own, keeps its part. Only C proves `unreachable`, so no other fixture has an equivalent (below) |
| `static inline size_t sbAvail` in [strbuf.h](c/strbuf.h) | one function, whichever units include it |

Go, Python and JS/TS have no macros, so a call a macro writes has no
equivalent there. Clojure has macros, but its adapter does not expand them
([Clojure](../../docs/contracts/CLOJURE.md)), so a call a Clojure macro writes
is not a call in its index. Only C turns a function into a number with a
cast. Go, Python and Clojure get a function's identity only from a call that
receives the function (`reflect.ValueOf(f).Pointer()`, `id(f)`,
`System/identityHashCode`), which is a different construct, and TypeScript
has none. Both cases are recorded as missing rather than imitated.

A C function runs only when running code names it, so the C adapter proves
what a program never runs. No other adapter can: Go reflection, interfaces
the standard library calls and `go:linkname`; Python `getattr`, `importlib`
and special methods; JS/TS computed property names and dynamic `import()`;
Clojure `resolve` and vars invoked as values all reach functions no call
names. Their fixtures have no `unreachable` declaration, and their contracts
record the missing equivalent rather than an unsound one.

Comparable response-field examples:

| Language | Source example | Declaration |
| --- | --- | --- |
| TypeScript | [src/type-members.ts](jsts/src/type-members.ts) | `IGetLevelsResponse` owns `count: number`. |
| Python | [src/fixture_app/models.py](python/src/fixture_app/models.py) | `GetLevelsInfoResponse` owns `count: int`. |
| Go | [internal/storefixture/level_responses.go](go/internal/storefixture/level_responses.go) | `GetLevelsInfoResponse` owns `Count int` with the JSON name `count`. |

The checks follow these declarations through the real language adapter,
ProgramIndex, atlas type members and question evidence. They preserve each
field's signature and exact source location under its own type; a same-named
field in another type cannot replace it. These checks make no model request
and do not assert that a future answer will select or correctly explain the
field.

Go interface methods are covered in
[internal/storefixture/fixtures.go](go/internal/storefixture/fixtures.go#L136):
`TicketContract[T]` declares `Cancel(id T) error` and `Status(id T) string`.
The cumulative Go check retains their original owner and signatures, rejects
new method ownership on `EmbeddedTicket`/`TicketAlias`, and verifies that a
declaration without a body acquires no execution node or runtime relations.

SQL statement facts need statement structure. Each language hands one SQL
statement and one ordinary message that starts with an SQL verb to calls
outside the repository; only the statement becomes a `sql_query` fact:

| Language | Statement | Ordinary text |
| --- | --- | --- |
| Go | [handoff_flow.go](go/internal/storefixture/handoff_flow.go) `QueryRowContext` | [cmd/app/sql_literals.go](go/cmd/app/sql_literals.go) `fmt.Errorf("create %s dir: %w", ...)` |
| Python | [data_sources.py](python/src/fixture_app/data_sources.py) `connection.execute` | [sql_literals.py](python/src/fixture_app/sql_literals.py) `logging.error("create %s dir", path)` |
| TypeScript | [data-sources.ts](jsts/src/data-sources.ts) `connection.execute` | [sql-literals.ts](jsts/src/sql-literals.ts) `console.error("create %s dir", path)` |
| Clojure | [core.clj](clojure/src/example/core.clj) `(query! "SELECT ...")` | [core.clj](clojure/src/example/core.clj) `(format "create %s dir" dir)` |

A statement whose table the source fills in stays a statement with no listed
table. Go (`fmt.Sprintf("DROP TABLE IF EXISTS %s", table)`) and Clojure
(`(format "DROP TABLE IF EXISTS %s" table)`) hand it to a call outside the
repository, so it is also a `sql_query` fact. Python's `%` operator and a
TypeScript template literal are not call arguments, so their equivalents in
`sql_literals.py` and `sql-literals.ts` are partial source SQL only. C has no
equivalent: its standard library has no database call, and the C fixture
links no database library.

Calls chained or nested on one line keep their own positions. Each language
writes different calls with the same value, and the same call twice, on one
line; every call is its own registration fact at its own column and its own
boundary place, observed once by each of two targets sharing the file:

| Language | Source example | A call's position |
| --- | --- | --- |
| TypeScript | [platform.ts](jsts/src/platform.ts) `chainedPlatformCalls`, [server.ts](jsts/src/server.ts) `registerChainedOrderConsumers` | the called member's name |
| Python | [events.py](python/src/fixture_app/events.py) `subscribe_chained`, `chained_text_calls` | the attribute name |
| Go | [http_registrations.go](go/internal/storefixture/http_registrations.go) `registerStrippedFiles` | the opening parenthesis |
| Clojure | [core.clj](clojure/src/example/core.clj) `chained-paths`, `nested-paths` | the form's opening parenthesis |
| C | [kvcli.c](c/kvcli.c) `withoutScheme` | the called function's name |

Go's and C's standard libraries have no fluent registration chain, so nesting
stands in for it. A Clojure Java instance chain (`(.. s (replace "/" "-"))`) carries no
call pattern and becomes no fact. Python knows no type for an untyped
parameter or for what `subscribe` returns, so those calls name no external
symbol; they are still separate facts.

A Clojure class usage that names no method is no call:
[core.clj](clojure/src/example/core.clj) `fresh-list` constructs an imported
`ArrayList` inside a syntax-quote, which clj-kondo reports as a call with no
method, and it projects no outside symbol, while `new-id`'s
`java.util.UUID/randomUUID` is a static call. Go, Python, JS/TS and C name an
outside callee from a declaration that always carries its name, so they have
no such row. `apply-each` calls an anonymous function literal's argument
(`#(% 1)`), a local clj-kondo gives no name; the call keeps `%` as written.
Every other language names its parameters.

A worker is built and run the way freqtrade's trade command runs its
`Worker`: constructed, then a method called on the name holding it, one it
inherits included:

| Language | Source example | Construction | Call on the result |
| --- | --- | --- | --- |
| Python | [workers.py](python/src/fixture_app/workers.py) | `Worker(name, 2)` calls the class (`construct`) and its `__init__`; `QuietWorker(name)` runs the inherited `BaseWorker.__init__`; `Plain()` the class alone | `worker.run()` is `BaseWorker.run` after `worker = None`; `chosen`, stored twice, stays unresolved |
| TypeScript | [workers.ts](jsts/src/workers.ts) | `new Worker(name, 2)` calls `Worker.constructor`, `new QuietWorker(name)` `BaseWorker.constructor`; `new Plain()`, with no constructor, stays unresolved | the declared type's method, `BaseWorker.run` inherited |
| Go | [workers.go](go/internal/storefixture/workers.go) | none runs: `NewWorker` is an ordinary call and the struct literal no call | `worker.Run()` is the promoted `baseWorker.Run` |

Clojure has no equivalent: a record's constructor is an ordinary function
and a protocol call on it stays unresolved. C has no constructor; its
`construct` is a record a table row or a field store builds.

A client is built once and inherited, and a call's result is used on the
spot, the way freqtrade's Exchange keeps its ccxt client, its Webhook its
address and its Telegram bot its application:

| Language | Inherited call and field | A subclass's store | A call's result, and a callable handed through partial application |
| --- | --- | --- | --- |
| Python | [inherited_clients.py](python/src/fixture_app/inherited_clients.py): `self.ask` in `FuturesPrices` is `Prices.ask`; `self.client.get` is `httpx.Client.get` from the factory's declared type and from the annotated parameter; `Quotes.last` (a subclass stores its own client) and `MaybePrices.ask` (a union) stay unresolved | `Webhook.send`'s `self.url` is Webhook's or Discord's store, both listed | [outside_results.py](python/src/fixture_app/outside_results.py): `Path(name).open()`, `Worker(name, 1).run()`, the `Application.builder()` chain, `self._app` from an untyped factory's one return; `partial(self._force_enter, …)` hands `_force_enter` to `CommandHandler` |
| TypeScript | the compiler's declared types (`workers.ts`) | missing: only the enclosing class's constructor record (JSTS) | `createConsumer().on(...).on(...)` in [server.ts](jsts/src/server.ts) is `Consumer.on`; `bind` hands over its result: missing |
| Go | promoted methods and declared field types (`workers.go`) | no base method runs for another type | `router.HandleFunc(...).Methods("GET")` in [main.go](go/cmd/app/main.go); no partial application |

Clojure keeps no fields, types no call result and hands over `partial`'s
result (CLOJURE); C has no classes or member calls, so neither has an
equivalent.

A registration on a router parameter is held by what the function's callers
pass. One caller hands the same router to a helper that passes it on to the
leaf, to a branch helper that also hands it to itself, and to a spare helper
that hands itself a router of its own making. The leaf is held by the
construction call two parameters back, and so is the branch, since handing
its own router to itself adds no value; the spare's route has no holder,
since two routers reach it:

| Language | Source example | The leaf's and branch's holder |
| --- | --- | --- |
| Go | [http_registrations.go](go/internal/storefixture/http_registrations.go) `RegisterRouteTree` | `http.NewServeMux()` |
| Python | [http_registrations.py](python/src/fixture_app/http_registrations.py) `install_route_tree` | `APIRouter()` |
| TypeScript | [http-registrations.ts](jsts/src/http-registrations.ts) `installRouteTree` | `express()` |

Clojure has no equivalent: its adapter records no parameter values, so no
registration is followed through a parameter. C has none either: a C call
has no receiver, and a holder handed as an argument is never followed
through a parameter in any language.

Test code comes from runner facts, never from a file name alone:

| Language | Test code | Stays production or unclassified |
| --- | --- | --- |
| JS/TS | `node --test *.test.mjs` matches, the Playwright config, its `testDir`, reporter and the stub API only its `webServer` starts in [packages/canvas-ui](jsts/packages/canvas-ui); the Vitest config and its matches | the application server that `start` runs and `webServer` starts too, a draft test the script glob does not select, [src/excluded/retained.test.ts](jsts/src/excluded/retained.test.ts) |
| Python | pytest `python_files` matches and [tests/conftest.py](python/tests/conftest.py) | `tests/__init__.py` |
| Go | build-selected `_test.go` files | [internal/testhelper/helper.go](go/internal/testhelper/helper.go) |
| Clojure | namespaces that require `clojure.test` | none in the fixture |
| C | none: C has no standard test runner, and the Makefile's `test` rule is a recipe | every unit, including [tools/dump.c](c/tools/dump.c) |

A test's SQL reaches no program's outbound calls or data
([test_code_catalogs_test.go](../../internal/contracttest/test_code_catalogs_test.go)):
each fixture's test file writes `CREATE TABLE test_only_rows`.

| Language | Test file | What the facts see |
| --- | --- | --- |
| Go | [root_test.go](go/root_test.go), and [root_optional_test.go](go/root_optional_test.go), which no load selects | data only: test sources are parsed declarations without calls |
| Python | [tests/test_facade.py](python/tests/test_facade.py) | a `sql_query` call and data |
| JS/TS | [src/market.test.ts](jsts/src/market.test.ts) | a `sql_query` call and data |
| Clojure | [test/example/service_test.clj](clojure/test/example/service_test.clj) | a `sql_query` call only: the database extractor reads no Clojure |
| C | none: the C adapter records no testing sources | |

A Go program that builds only with build tags
([go_build_tags_test.go](../../internal/contracttest/go_build_tags_test.go)):
[cmd/vfs](go/cmd/vfs/main.go) and [root_vfs.go](go/root_vfs.go) build only
with `fixturevfs`, which the fixture's [Makefile](go/Makefile) passes to
`go build ./cmd/vfs` through `VFS_TAGS`. Only Go has build constraints: C's
equivalent, the flags of a makefile's link lines, is read from its dry run;
Python, JS/TS and Clojure have none.

Runner-configured test directories have no derived equivalent in Python
(`testpaths` often names the production package) or Clojure (no manifest is
read). The Python and Clojure contracts record both gaps. C has no runner
facts at all.

A record's field is read and written where functions name it, and one
field's readers and writers gather by the field whatever each reaches it
from (ProgramIndex `field_path`):

| Language | A field's writers and readers |
| --- | --- |
| C | [kvd.c](c/kvd.c) `server.shutdown`: one writer, `onSignal`, and one reader, `beforeSleep`; `server.dbfile` written by `main` and `loadConfig`, read by `bgsaveCommand`; `kvEntry.value` written through `setCommand`'s local `e` and read as `server.db.value` by `saveSnapshot` |
| Python | [models.py](python/src/fixture_app/models.py) `MutableCounter.count`: each write to the field object; no `field_path` (the written expression is the witness) and no chains |
| Go | [server_state.go](go/internal/storefixture/server_state.go) `serverState.shutdown`: one writer, `onStateSignal`, and one reader, `stateBeforeSleep`; `serverState.dbfile` written by `StartStateServer` and `loadStateConfig`, read by `saveStateSnapshot`; `stateEntry.value` written through `setState`'s local `e` and read as `serverState.db.value` by `saveStateSnapshot` |
| TypeScript | missing: a property read is a declared value reference; no writes (JSTS) |
| Clojure | missing: a map's keys are keywords and a record's fields no declarations (CLOJURE) |

A callable stored into the field of an outside value the code holds is
handed to that field, named with its declared type:

| Language | Store into an outside value's field |
| --- | --- |
| Go | [tool_cli.go](go/internal/storefixture/tool_cli.go) `fs.Usage = c.Usage` hands `toolCommand.Usage` to `flag.FlagSet.Usage` (`func()`); [server_state.go](go/internal/storefixture/server_state.go) `srv.Handler = mux` hands `stateStatus` to `net/http.Server.Handler` (`http.Handler`); a composite literal still constructs its type (`testing.InternalTest`) |
| C | [kvd.c](c/kvd.c) `act.sa_handler = onSignal` is handed to `struct sigaction`, the record rather than its field: a recorded difference (GO) |
| Python | missing: an attribute write binds only a repository class's field (PYTHON), so a callable stored on an outside object's attribute hands nothing over |
| TypeScript | missing: a property assignment is a destination and no relation (JSTS) |
| Clojure | missing: a map's keys are keywords and a record's fields no declarations (CLOJURE) |

A C command table, a callback stored under a branch and a call through a
function-pointer field have these equivalents:

| Language | Table of named handlers | Callback stored under a branch | Call through a stored function value |
| --- | --- | --- | --- |
| Go | [command_table.go](go/internal/storefixture/command_table.go) `commandTable`: one exact binding per row, with that row's `Name` and `Arity` | `eventLoop.register`, `RunChosenHandler`: the calls through the fields stay unresolved; with interface-typed fields (`readyLoop.register`, `RunChosenReady`) they are unresolved with each stored handler as a witness, as in C | `RunSingleHandler` is exact; a looked-up row (`DispatchCommand`) is unresolved, where C gives the table's handlers as alternatives |
| Python | missing | [stored_callbacks.py](python/src/fixture_app/stored_callbacks.py) `EventLoop.register`: the calls through the attributes stay unresolved; `run_chosen_handler`: the call through a name reassigned under a branch stays unresolved and names both stored functions | `run_single_handler` (a local name) is exact; through an attribute, missing |
| TypeScript | missing | [stored-callbacks.ts](jsts/src/stored-callbacks.ts) `EventLoop.register`: the calls through the properties stay unresolved | missing |
| Clojure | missing | missing | [core.clj](clojure/src/example/core.clj) `with-shadow` is unresolved |

Each handler keeps its exact callback at the call that registers it. The
language contracts record every missing equivalent, including one that gives a
store under a branch a wrong answer: Go's interface field stored by a helper
that a branch calls (false alternatives). A Python name reassigned under a
branch leaves its call unresolved and names each function stored in it, as
the C adapter does.

A C call written through a macro and a C function address cast to an integer
have no equivalent in Go, Python or TypeScript, which have neither macros nor
such casts. Clojure has macros: a call written in a macro's argument keeps its
own place and caller (`ensured-limit` in
[core.clj](clojure/src/example/core.clj)), while the use of the macro itself
leaves no call, which the Clojure contract records.

The inputs a program's code declares (the 2026-09-28 inputs sequence) are
read end to end with a preset that answers each closed question from the
item it shows (`internal/contracttest`: the kvd preset for C, the inputs
preset for the others):

| Shape | C | Go | Python | TypeScript | Clojure |
| --- | --- | --- | --- | --- | --- |
| A split file's seed is its own row (e1) | [kvd.c](c/kvd.c) `main` with `setupSignals` | [cmd/app/main.go](go/cmd/app/main.go) `main` with `Subscribe` | not read: the map test reads the library, which has no seed | any seed of a split file (partstest checks it on every split read) | [core.clj](clojure/src/example/core.clj) `-main` |
| Each word call asked on its own | `strcasecmp(argv[1], "--raw")` beside `strcasecmp(cmd->name, "bgsave")` in [kvcli.c](c/kvcli.c) | `strings.EqualFold(os.Args[1], "check")` beside `strings.EqualFold(level, "default")` in [tool_cli.go](go/internal/storefixture/tool_cli.go) | missing: a comparison is an operator | missing: `process.argv` names no symbol without Node's declarations | `(= (first args) "--shout")` beside `(= (:name row) "default")` |
| Where a compared argument comes from | element "0" of `splitLine`'s result through a local, element "1" of `argv`, field `name` of `lookupCommand`'s result | the call as written | the call as written | missing | the call as written |
| Catalogue of a declaring function (K1) | `main`'s `--symbols`, `loadConfig`'s directives | `DestinationApplication`'s `price-endpoint` | by object (below) | missing: commander's declarations are not installed | missing: `tools.cli` vectors are no call literals |
| Catalogue of an object (K2) and J1 | missing: no object is made for an option | two `flag.NewFlagSet` objects in `ToolCommand`; J1 missing | `argparse.ArgumentParser("tool")`, init's parser; `add_parser` + `set_defaults` one input, also through fields stored once (`ServiceCommands`'s `serve`), not through a field stored twice (`RebuiltParser`) | missing (commander) | missing: no call-result origins |
| Setting | `loadConfig`'s `port`, `dbfilename`, `persist` | `ServerConfig`'s tagged fields decoded by `json.Unmarshal` | missing: no key alias recorded | missing | missing: EDN keys are keywords |
| A directive's values nested under it (K3) | `loadConfig`'s `persist` with `never` and `always`, compared with element "1" of the same `splitLine` result | missing: no configuration line split into words | missing: a comparison is an operator | missing | missing: no call-result origins |
| Registration as written | `{"get", getCommand, 2, preloadKey}` | `strings.EqualFold(os.Args[1], "check")` | `add_argument("--force"…)`, `add_parser("init")` | missing: no inputs fixture test | `(= (first args) "--shout")` |
| Kept callable and table of names (pass 2) | `acceptHandler` request; kvcli's `cmdTable` rows | missing: S1 not enabled; no table of names (Go records no whole-variable read) | tables: [dispatch.py](python/src/fixture_app/dispatch.py) `OPTIONS` (`Opt(...)` rows) and `REQUIRED`, both read by a function; `FORMATS`, read by nothing, is none | missing | missing |
| A value compared with several words, asked once (one input per case) | kvcli's `shortOption`: `switch (arg[1])`, `'h'`/`'?'` one case, `'V'` | [tool_cli.go](go/internal/storefixture/tool_cli.go) `RunSubcommand`: `switch cmd` and its default's `cmd == "help" \|\| cmd == "-h"`, cmd from `args[0]`; `IsDefaultLevel`'s lone `==` is none | [dispatch.py](python/src/fixture_app/dispatch.py) `dispatch`'s if/elif chain (`command` from `argv[0]`), `describe`'s `match`; `is_default` is none | [dispatch.ts](jsts/src/dispatch.ts) `switch (process.argv[2])` with stacked `check`/`verify`; `isDefault` is none | `run-command`'s `(case (first args) …)`; `=` stays a call |
| A row's peer input (K5) | kvcli's rows name kvd's inputs; `del`, left unmatched, names none | no table of inputs | none | none | none |
| Outer inputs of a dispatch site (u6) | `processCommand` from `acceptHandler` through `readQueryFromClient` | no dispatch site | no dispatch site | no dispatch site | no dispatch site |
| A launch named by its word | missing in a preset read (`tools/dump.c`'s `popen("sort -u")` is asked up to atlas_api) | `Revision` names git | missing: a list's strings are no call words | missing (Node) | `revision` names git |
| Outside package asked its system once | kvcli's socket calls | `net/http` with every symbol called | `httpx` (`requests` is the fixture's own module) | missing (`fetch` names no package) | no outgoing call |

The three-step glossary reads accepted prose, not a language's code: its
tests are the terminology package's, with no fixture here.
