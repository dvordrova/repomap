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

- No Clojure function is proven `unreachable` (the C adapter's per-program
  fact, PROGRAM_INDEX): `resolve`, `requiring-resolve`, `ns-resolve`, a
  symbol or var invoked as a value, multimethods and protocol dispatch reach
  functions no call names. A boundary in a namespace several targets load
  stays with every target, and a part leaves no target's map as code that
  target never runs (READING).
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
the function its expansion calls; that equivalent is missing here.

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
