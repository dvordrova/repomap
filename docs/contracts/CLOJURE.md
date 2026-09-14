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
