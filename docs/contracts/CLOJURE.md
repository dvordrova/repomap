# Clojure JVM adapter

The ordinary Clojure adapter discovers projects from `deps.edn` and
`project.clj` in the shared corpus. Coexisting manifests describe one project;
`deps.edn` supplies its selector (`clojure:deps.edn` at repository root).
Each project owns its `.clj` and JVM `.cljc` sources up to nested manifest
boundaries, including tests, examples and tools. `.cljs` is inventoried but is
not part of this JVM execution view.

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
Java instance dispatch and dynamic function targets remain unresolved. This
initial adapter does not implement ClojureScript execution views or a runtime
macroexpander; definition/control macro syntax is not promoted into runtime calls.

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
spans); a namespace counts its whole file. Since `defmethod` is no
declaration, the in-file repeat the fixture asserts is a `declare` and its
`defn` (`example.core/shout` in `src/example/core.clj`, 1 and 4 code lines),
which the map of parts reads as one unit.

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
- A table row that stores two callables is one registration in C; Clojure
  has no table-row registration, so the rule has nothing to apply to here.
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
recorded in the 2026-09-29 C field pass, not fabricated.

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
has no Clojure equivalent either. Not recorded yet:

- `-main`'s `& args` carry no argument vector origin;
- `tools.cli` option vectors are vectors, not literals given to a call;
- `case` on an argument;
- `reset!`/`swap!` stores and registries kept in atoms;
- settings a structure names (GO, the tagged-field question): an EDN
  configuration's keys are keywords a function reads (`(:dbs config)`,
  `{:keys [dbs]}`), declared by no structure, so nothing is asked what a
  key is.


## Programs a call starts

`revision` in `src/example/core.clj` calls `(shell/sh "git" "rev-parse"
"HEAD")`: `clojure.java.shell.sh` is asked with the form as written and
gives the words `git`, `rev-parse`, `HEAD`.

## Test sources

A namespace that requires `clojure.test` or `speclj.core` is a test source.
Runner-configured test directories are a missing equivalent of Playwright's
`testDir`: the adapter reads no manifest contents, so Leiningen
`:test-paths` and a `deps.edn` alias that runs `cognitect.test-runner` or
Kaocha are not derived. An alias's `:extra-paths` carries no runner
authority by itself. A helper namespace under `test/` that requires neither
framework stays unclassified.

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
reader branches and stable repeat extraction. The ordinary adapter test checks
the same graph and dependencies through the registered dispatch boundary.

Comparable import/call/source-argument/callback cases already exist in the
cumulative Go, Python and JSTS fixtures and their native adapter tests. Clojure
reader-discard/reader-conditional syntax and clj-kondo's Java metadata serializer
have no identical native equivalent in those languages.
