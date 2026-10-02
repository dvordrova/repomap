# Clojure JVM adapter

The ordinary Clojure adapter discovers projects from `deps.edn` and
`project.clj` in the shared corpus. Coexisting manifests describe one project;
`deps.edn` supplies its selector (`clojure:deps.edn` at repository root).
Each project owns its `.clj` and JVM `.cljc` sources up to nested manifest
boundaries, including tests, examples and tools. `.cljs` is not part of this
JVM execution view.

Each build of a `shadow-cljs.edn` that names what it starts from (a module's
`:init-fn` or `:entries`, a node script's `:main`) is a ClojureScript program
of its own (`clojure:shadow-cljs.edn:app`, kind `executable`, named by its
build id), restored from its `shadow-cljs.edn` (`ScoutShadow`). Its view is the
`.cljs` sources and the `:cljs` branch of the `.cljc` sources of that
directory, up to a nested `deps.edn`, `project.clj` or `shadow-cljs.edn`, read
by the same clj-kondo analysis; its seeds are the build's `:init-fn`/`:main`
functions and `:entries` namespaces, and `-main` seeds only the JVM view.
ClojureScript's own namespaces (`cljs.core`, `clojure.string`, ...), the
Closure Library (`goog.*`) and the JavaScript globals clj-kondo writes with no
namespace (`js/setInterval`, an external symbol of package `js`) are its
platform. Othello's `:app` build is its browser page beside the desktop
program `deps.edn` describes.

`internal/clojureproject` runs clj-kondo on exactly those files. Namespace and
var identities, declarations, imports, uses, Java static calls and local binding
uses come from its native analysis. The adapter projects them into the existing
ProgramIndex and dependency catalog; all subsequent reading and reporting use
the ordinary pipeline and shared repomap cache. There is no new analysis cache.
Only a real OS argument-size failure partitions a native invocation, and all
rows are merged before projection. Native scheduling and process-local local
binding IDs are normalized before sealing an identical view.

Native namespace/var bindings join repository calls. A same-named local
parameter stays an unresolved callable. Native argument-site symbol uses bind
callback transfers to their original source argument. Clojure reader syntax
preserves complete source expressions, quoted forms, reader-discard forms and
literal strings without executing them. Exact source docstrings enter the
ordinary author-claim layer, with their original quote location.

`-main` declarations provide callable seeds. Test-framework imports
`clojure.test` and `speclj.core` identify test source files. Platform namespaces
come from the Clojure distribution's named namespace set, not the `clojure.*`
prefix; third-party namespaces under that prefix remain packages. Package
catalog identities are native namespace names, not inferred Maven coordinates.
A Java static call is a class usage that names its method
(`(java.util.UUID/randomUUID)`). clj-kondo also reports as a call with no
method an imported class that a syntax-quoted constructor names
(`` (defmacro fresh-list [] `(ArrayList.)) `` in the fixture's `core.clj`) and,
on metabase, classes of an `:import` list; like a plain constructor
(`:call false`) such a row calls nothing and projects no outside symbol. A
native row whose namespace, class or name is no name ProgramIndex accepts
(empty, surrounding space or a control character) names no outside symbol
either: a use keeps its unresolved call, and an import of it is skipped,
rather than an invented or empty symbol failing the whole index. Go, Python,
JS/TS and C name an outside symbol from a native declaration or import that
always carries its name, so none has an equivalent row to guard.
A call of an anonymous function literal's argument (`#(% 1)`, the fixture's
`apply-each`) is a local clj-kondo reports with no name; its function-value
call keeps the callee as the source writes it (`%`, `%1`, `%&`), so its
pattern names what is called. Go, Python, JS/TS and C name every parameter,
so none has such an unnamed local.
Clojure parameters carry no repository type (no `type_id`), so no Clojure
callable `takes` a type in the places graph (READING); the other four
adapters record it and their fixtures check it.
Java instance dispatch and dynamic function targets remain unresolved. The
adapter has no runtime macroexpander; definition/control macro syntax is not
promoted into runtime calls (a `future`'s use is the one exception, below).
A special form (`if`, `do`, `recur`, `fn*`, `try`, `.`) is the language's
syntax, no call: clj-kondo knows an arity for every function of
`clojure.core` and `cljs.core` and none for a special form, so a core usage
with neither `fixed-arities` nor `varargs-min-arity` leaves no relation, as
an `if` or a loop is no call fact in Go, Python, JS/TS or C. clojure.core's
functions (`=`, `str`, `dec`) stay platform calls, read in a flow's "also
calls" line as a C program's libc calls are. A definition's call of itself
written in one arity that its argument count sends to another (othello's
`move` `[board player]` calling `[board player opts]`) keeps its relation
and carries an `arity` witness naming the parameters called, so its reading
says it calls that form, not itself; a call its own arity takes is a
recursion. Only Clojure writes several arities of one definition
(`TestAClojureSpecialFormIsNoCallAndAnotherArityIsNoRecursion`,
`greet-times` in `core.clj`).

Map/vector values returned by functions remain values in native observations;
their constructors are functions, not invented named types. The ordinary
type-concept reading currently cannot expose them as independent entity cards.
Architectural core parts and their callable descriptions still survive. For
example, Othello's board vector and game map have no type declarations; its
Board representation and Game state responsibilities remain in GroupsIndex.
No runtime mutation or entity ownership is inferred from `assoc`/`update` names.

A var is public unless it is `defn-` or `^:private` (clj-kondo's `private`);
the map of parts shows signatures of public vars only. `defmethod`,
`extend-type` and `extend-protocol` are not declarations this adapter
projects, so a method implemented in another namespace than its multimethod
or protocol has no declaration of its own to move with its type; this
equivalent of a Go method declared outside its type's file is recorded as
missing, not fabricated.

Each var carries `code_lines`: the lines of its form holding a character
outside `;` comments and outside the docstring of an `ns`, `defn`, `defn-`,
`defmacro`, `defmulti` or `defprotocol` form (the reader's own docstring
spans); a namespace counts its whole file. A `declare` is a forward
declaration and no var of its own: `example.core/shout` in
`src/example/core.clj` is its `defn` alone (4 code lines); othello's
`(declare negamax)` had stood as a second tile beside the defn.

## A library's exports (missing)

A Clojure package with no `-main` is a library whose public vars (`defn`,
neither `defn-` nor `^:private`, outside test sources) would be its exports
(PROGRAM_INDEX `target.exports`), as Go's exported names are. The cumulative
fixture's one `deps.edn` package has a `-main`, so no fixture holds a library
package: a recorded missing equivalent, and the adapter exports nothing yet.

## Keyword arguments

A call's trailing keyword/value pairs are its keyword arguments, as a
function taking `& {:keys [...]}` receives them, and since Clojure 1.11 a
trailing map literal passes the same arguments:
`(q/sketch :title "Greeter" :draw draw-greeting :key-pressed on-key)` and
`(ws/websocket "ws://localhost:8080/feed" {:on-message receive-greeting
:on-close close-feed})` in `src/example/core.clj`. Each value is an argument
under its keyword (the keyword form itself is no argument), the arguments
before them keep their positions, and a function handed under a keyword is a
callback bound to that keyword. A run of pairs goes back from the last
argument while each pair starts with a plain keyword; a repeated or
auto-resolved (`::k`) keyword leaves every argument positional. The facts pass
reads an outside call handed several repository functions under keywords as
one registration per keyword (PROGRAM_INDEX, the facts pass), the keyword
its first word: othello's `quil.core.sketch.key-pressed` hands `host/on-key`
over at `src/othello/ui/sketch.clj:45` with the words `quil.core/sketch`,
`key-pressed`, `Othello`, the title offered with the keyword it is given
under (`title`, READING's `WordsGiven`), so the entry is named key-pressed
and its key table (n, u, h, 1, 2) stays under it. `TestKeywordArguments`,
`assertKeywordArguments` and `TestCumulativeClojureKeywordHandoffsAndFutures`
check both forms. A Python call's keyword arguments and a Go struct's fields
are keyword arguments natively; a synthetic facts case
(`TestRegistrationsComeFromCallShapesNotFrameworkNames`, a WebSocketApp handed
`on_message` and `on_close`) checks the shared rule, and the Go, Python, JS/TS
and C cumulative fixtures hold no outside call handed two callables under
keywords yet: recorded, not fabricated.

## Handler tables and stored callbacks

These are the Clojure equivalents of the C adapter's command table, its
callbacks stored under a branch and its calls through function-pointer fields.
A call through a local, whether a parameter or a `let` binding, is an
unresolved `function_value` call. `with-shadow` in `src/example/core.clj`
checks this for a parameter that shares a var's name. A `let` binding of a
known var (`(let [handler accept-client] (handler))`) stays unresolved too;
the var is a read at the binding.

Missing equivalents, recorded rather than fabricated:

- Multimethod and protocol dispatch stay unresolved, so GroupsIndex derives
  no dispatch site. A function handed to a higher-order call
  (`(map service/greet names)`) is recorded as an exact call of it beside the
  hand-over, so no declaration of the fixture is handed over without being
  called; the reach's rule that a hand-over is not followed is checked on the
  other adapters' facts and GroupsIndex's unit test.
- No Clojure function is proven `unreachable` (the C adapter's per-program
  fact, PROGRAM_INDEX): `resolve`, `requiring-resolve`, `ns-resolve`, a
  symbol or var invoked as a value, multimethods and protocol dispatch reach
  functions no call names. A boundary in a namespace several targets load
  stays with every target, and a part leaves no target's map as code that
  target never runs (READING). Two targets loading one namespace therefore
  list nothing either never runs, and no declaration is shown "run by" the
  other (REPORT).
- An unresolved `function_value` call names no function a binding could hold,
  so it draws none of the possible arrows the C, Go and Python store
  witnesses draw.
- A call of a function's own parameter (`(defn run [job] (job))`) stays that
  unresolved call: the vars its callers hand there are not joined to it, where
  the Python, Go, JS/TS and C adapters make them its targets (PYTHON, Handler
  tables and stored callbacks).
- A table row that stores two callables is one registration in C; Clojure
  has no table-row registration, so the rule has nothing to apply to here.
- `(subscribe "topic" (partial handle-order store))` hands over `partial`'s
  result: the call hands over no function, where Python's
  `functools.partial(f, ...)` hands `f` (PYTHON, What a call produces).
- A map of handlers (`(def commands {"get" get-command})`) makes each handler
  a native var read of the var that holds the map, without its key. No
  binding names the handler.
- Ordinary Clojure keeps a callback in a map, a record or an atom rather than
  in an assignable field. Handlers kept in an atom
  (`(swap! handlers assoc :on-read handler)`) leave no binding. Invoking a
  looked-up value (`((:on-read @handlers))`, `((commands name) args)`) records
  no call of its own, because the head of that form is a form rather than a
  symbol. The inner `(commands name)` is still an exact call of the map var.

## Calls written through macros

A call written in a macro's argument is an ordinary call at its own place, by
the function that wrote it: `(ensure! (read-limit))` in `ensured-limit`
(`src/example/core.clj`) is an exact call of `read-limit`. Every macro usage is
skipped, so the use of `ensure!` itself leaves no relation, and the
`(fail! ...)` its syntax-quoted body writes is a read of `fail!` by the macro.
The C adapter records a call written through a macro at its use, as a call of
the function its expansion calls; that equivalent is missing here, and a
macro's own use leaves no relation, so the places graph records no use of a
macro (READING). The graph's `uses` hold var reads and hand-overs:
`TestCumulativeClojureMapOfParts` checks that `read-limit` uses
`example.service/source-limit` and `greet-many` hands `example.service/greet`
to `clojure.core/map`. The role split's helper question (READING) counts
these as users. Since no use of a macro is recorded, the adapter marks each
declaration clj-kondo reports as a macro with ProgramIndex's `macro`
(`ensure!` and `fresh-list` in the fixture), and the question asks about a
macro whatever the graph shows of its uses, as it does about a type: no
recorded use is no proof of none (`TestCumulativeClojureMapOfParts` checks
both marks and that `ensure!`, which `ensured-limit` uses, is asked). The
recorded use of a macro stays a missing equivalent, not patched. The private
`exclaim` near the end of
`core.clj`, which only `cheer` calls, is the fixture's helper: the split
check places it with `cheer`, and `cheer`, public and called by nothing, is
not asked.

A Clojure var read is a `reads` relation; a map's key is a keyword a
function reads with a call (`(:dbs config)`), a record's field has no
declaration the index names, and a value is not written in place (an
atom's `swap!` is a call). So Clojure records no field read or write and no
`field_path`, where C records each with its path (C, PROGRAM_INDEX);
recorded in the 2026-09-29 C field pass, not fabricated. Nor does it record
a parameter's origin: the fixture's `deliver!` spits to `destination`,
which `core.clj` passes as "greeting.txt", and the file that call reaches is
a path not established (READING, files a program keeps; recorded in the
2026-09-29 files pass).

## Calls that run when a namespace loads

A call belongs to the scope in which it runs, in every language. The reader
metadata written on a defined name (`def`, `defonce`, `defn`, `defn-`,
`defmacro`, `defmulti`) and a `defn`/`defn-`/`defmacro` attr-map, before the
parameters or after a list of arities, are evaluated once, when the namespace
loads. Their calls and reads belong to the namespace (or to an enclosing
definition), not to the var, the way a Python decorator's arguments and
defaults belong to the defining scope. `:pre`/`:post` conditions and `:or`
defaults run on each call and stay the function's. `routed-by-meta`,
`routed-by-attr-map`, `routed-value`, `routed-once` and `routed-multi` in
`src/example/core.clj` check each form. The owner is read from the definition
form's spans, as for every other Clojure use. clj-kondo reports no use inside
a `defmulti` attr-map, so a call there is a missing equivalent, recorded
rather than fabricated. Open, not aligned: a `def`/`defonce` value that is not
a function (`(def app (wrap routes))`) also runs at load, but its calls still
belong to the var, while Python gives a module-level assignment's calls to
the module.

## Inputs a call's words declare, and what Clojure does not have yet

A call of an outside var given a literal is asked on its own what the
words become (READING, the `atlas_api` per-call question): the fixture asks
`clojure.core/format` (`(format "create %s dir" dir)`),
`clojure.string/replace`, and `clojure.core/=` twice, `shouted?`'s
`(= (first args) "--shout")` and `default-row?`'s `(= (:name row)
"default")`, each on its own (`TestCumulativeClojureInputsAreAskedPerCall`).
The object an input is declared on and J1 have no Clojure equivalent: the
adapter records no call results as origins. A value chosen on either
branch is an `if` expression, not a reassigned local, and the adapter
records no value for it, so the Go join of both launches (`alternatives`)
has no Clojure equivalent either. A `case` form whose tests are strings,
or lists of strings, compares its value with those words: two or more
words in two or more cases are one comparison of the enclosing var
(PROGRAM_INDEX `comparisons`), each case's branch from its test to its
result's last line, its value's origin the form as written; `run-command`'s `(case (first args) "serve" …
("check" "verify") …)` is two cases, while `=` stays a call asked on its
own; serve's case calls `shout`, so run-command handles serve there
(READING).

A call's argument written as its function's own parameter carries that
parameter (clj-kondo's locals, linked to the parameter vectors of the
`defn` as written), and `(:key event)` on one that field of it: deliver!'s
`(spit destination message)` spits to the file -main passes,
`greeting.txt`. A `def` of a map literal whose keys and values are keywords
or strings is a table of words, each entry a row `:n :new-greeting`
(`lines.RowText`). The sketch's key handler `on-key` hands `(:key event)` to
`service/command-for`, which calls `(get key->command key)`: the table's keys
n and u are the keys key-pressed takes (READING, tables a handler looks up
with what it was handed), as othello's key->command. Other values (a let
binding, a destructured parameter, a call's result) stay as written. Not
recorded yet:

- `-main`'s `& args` carry no argument vector origin;
- a function started on its own other than by a `future`: `(Thread. f)`,
  `core.async/go` and `thread` carry no `goroutine` invocation, so they are no
  started registration (GO, goroutines);
- a function literal (`(fn [] ...)`, `#(...)`) is no declaration the adapter
  projects, so one handed to an outside call hands nothing over: othello's web
  build hands its AI search to `(js/setTimeout (fn [] (reset! job (play-ai
  ...))) 20)`, which makes no registration, where Go, Python and JS/TS hand
  over their closure object;
- a handler's comparison of what it was handed is no sub-argument (READING,
  K3): the adapter records no argument's origin, so no argument is known to
  come from a parameter;
- `tools.cli` option vectors are vectors, not literals given to a call;
- `reset!`/`swap!` stores and registries kept in atoms;
- settings a structure names (GO, the tagged-field question): an EDN
  configuration's keys are keywords a function reads (`(:dbs config)`,
  `{:keys [dbs]}`), declared by no structure, so nothing is asked what a
  key is;
- spellings of one value in a `cond` or an `if` chain: only an `or` form is
  read (below).

Spellings of one value are recorded (PROGRAM_INDEX `same_value_as`) for the
operands of one `(or …)` written the same but for the string literals of
their one call, each call of the same form: `replica-options` in
`core.clj` reads `(or (get params "storageClass") (get params
"storage-class"))`, and the later `get` names the first; its region, read
once, and user and password under `and` are none
(`TestEveryLanguageKeepsTheSpellingsOfOneValue`). A form holding a
character outside ASCII joins nothing (the reader's offsets are runes).


## A future starts its body

A `future` runs its body on a thread of its own. Each call written as a form
of its body (`(future (greet-many names))` in `warm-greetings`,
`src/example/core.clj`; othello's `(future (play-ai ...))` in `launch-ai`)
carries the shared invocation word `goroutine`, as Go's `go f()` does, and the
`future`'s own use is a call of the outside `clojure.core/future` given that
call as its `call_result` argument, as `asyncio.create_task` is given a
coroutine. So the facts pass makes the started call a registration whose word
is `clojure.core/future` (PROGRAM_INDEX), and the reading asks the statement
`(future (greet-many names))` on its own what it starts
(`assertFutureStartsItsBody`, `TestCumulativeClojureKeywordHandoffsAndFutures`).
This is the one macro whose use leaves a relation; ClojureScript has no
`future`.

## Programs a call starts

`revision` in `src/example/core.clj` calls `(shell/sh "git" "rev-parse"
"HEAD")`: `clojure.java.shell.sh` is asked with the form as written and
gives the words `git`, `rev-parse`, `HEAD`.

## Test sources

A namespace that requires `clojure.test`, `cljs.test` or `speclj.core` is a
test source. So is every source under a directory the build description runs
as tests, the equivalent of Playwright's `testDir` and pytest's configured
files: a `deps.edn` alias whose `:main-opts` run (`-m`) or whose `:exec-fn`
names a test runner of those frameworks (`speclj.main`,
`cognitect.test-runner`, `kaocha.runner`) names its `:extra-paths`, and a
Leiningen project its `:test-paths`, Leiningen's own `test` when it writes
none. Their helpers are test code too: othello's `:spec` alias makes
`spec/othello/spec_helper.clj` a test source, and the fixture's `:test` alias
`test/example/fixtures.clj`, which requires no framework. An alias's
`:extra-paths` alone carry no runner authority (`:dev` adds tooling), and
neither does an alias running another tool over the same directory (othello's
`:cov`, `:mutate`); a helper under a directory no runner names stays
unclassified. A shadow-cljs build reads the test directories of the
`deps.edn` and `project.clj` beside it.

## Environment and native serialization

Install Clojure CLI and clj-kondo in the normal environment (on macOS:
`brew install clojure/tools/clojure borkdude/brew/clj-kondo`). JVM and Maven
requirements otherwise belong to each repository. All Go, Maven, Clojure and
clj-kondo caches retain their normal locations. Corpus inventory excludes
`.cpcache`, `.clj-kondo/.cache` and generated `.clj-kondo/inline-configs` without
deleting them.

The adapter reads clj-kondo's native EDN output. In 2026.08.04, the CLI JSON
writer cannot serialize internal reader-node ignore metadata attached to some
Java uses. EDN preserves those native rows. The transport decoder consumes the
public analysis fields and skips unconsumed metadata as complete forms, just
as a JSON decoder skips unknown fields. One native process produces the whole
view; no second JVM analysis, custom cache or runtime source evaluation is used.
Other native failures and syntax errors remain errors.

## Verification

`testdata/repositories/clojure` is the cumulative executable repository;
`testdata/contracts/clojure.files.json` binds its exact inventory. Native tests
cover seeds, namespace calls, a shadowed callable, literal and reader arguments,
Java ignore metadata, callback source-argument binding, author quotes, JVM-only
reader branches, keyword arguments, a future, runner-configured test
directories and stable repeat extraction; `TestShadowBuildIsAClojureScriptProgram`
and `TestCumulativeClojureShadowBuild` read the `:app` build
(`src/example/web.cljs` hands `refresh!` to `js/setInterval`, a registration
like any function handed to an outside call), and `TestManifestRowsQuoteAliasesAndBuilds`
the manifest rows. The ordinary adapter tests check the same graph and
dependencies through the registered dispatch boundary, and that the run
restores the JVM project from `deps.edn` and the build from `shadow-cljs.edn`.

Comparable import/call/source-argument/callback cases already exist in the
cumulative Go, Python and JSTS fixtures and their native adapter tests. Clojure
reader-discard/reader-conditional syntax and clj-kondo's Java metadata serializer
have no identical native equivalent in those languages.
