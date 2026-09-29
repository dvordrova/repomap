# Shared native evidence and materialization

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Typed graph

ProgramIndex is the single typed program graph passed from language adapters
to shared stages. The sealed in-memory graph retains
target, object, relation and nested provenance identities. Each target retains
its sealed `program-index-set.json` binding.

ProgramIndex retains:

- exact target scope and seeds;
- the names the repository's build gives the target's executable (ProgramIndex
  21, `target.executables`, sorted and each once; a build fact of its
  adapter, empty when the build names none): C's link-line output
  (`redis-server`; a program built by hand from its main unit has none), a Go
  main package's `go build` name (its import path's last element, the one
  before a major version suffix), a Python console or GUI script
  (`repomap-fixture`), each package.json `bin` command. Clojure names none;
  its deps.edn aliases are not executables. The report joins a started
  program to this repository's program by equal name (REPORT);
- objects and their compact target-local identities;
- adapter-observed package/module directories, independent of source locations;
- exact, alternatives, and unresolved relation authority as distinct states;
- structural relation kinds such as calls, imports, implements, decorates,
  passes-callback, binds-implementation, sources, executes, reads, writes, and
  invokes-external. Containment is the object's `container_id` (and `owner_id`)
  only; there is no parallel `contains` relation. A `reads` relation names the
  declared variable a callable uses, one per source site, never a runtime
  value: Python, JS/TS, Clojure and C emit it (a C function body's file-scope
  variables and tables, C); Go reads fields only, no whole variable (GO). A
  `writes` relation names a field a callable writes, one per site: Python's
  attribute writes on a typed receiver (PYTHON), C's and Go's field writes
  (C, GO). A read or write of a record's field targets the field object,
  contained by its type, and C and Go give it a `field_path`: the field as
  the code reaches it, from the file-scope or package variable the chain
  starts at or, from any other value, the record or struct type holding the
  chain's first field, then each named field with elements left out
  (`server.masterhost`, `server.db.expires`, and `redisDb.expires` for
  `db->expires`; Go's `serverState.db.value`, `Store.dbs` for `s.dbs`). The
  target gathers one field's readers and writers across functions; the path
  is what each wrote. Validation refuses a `field_path` on a relation that is
  no read or write of one field of a type. Python leaves the path out (the
  written expression is its witness's detail); JS/TS and Clojure record no
  field writes (JSTS, CLOJURE);
- complete witnesses and omission counts. A witness that names a
  declaration also carries its identity (`object_id`): the function a store
  put into the field or name an unresolved call reads (C
  `c_function_pointer_store`, Go `interface_field_assignment`, Python
  `function_value_store`). It is identity only: the call stays unresolved,
  the witness is never its target, and validation refuses an `object_id` that
  names no object of the index or a control-context witness that names one;
- every source-distinct neutral relation pattern;
- call/decorator form, selector, invocation, dispatch and exact source location.
  Every adapter uses the same closed words. `invocation` is how a call runs:
  absent for an ordinary call, `deferred`, `goroutine`, `async_task` or
  `construct`. `dispatch` is how the target was found: absent for a static
  target, `interface` (targets are implementations), `interface_method` (the
  declared method of an external interface; implementation unknown) or
  `function_value`. The relation kind already says a call, a callback or a
  binding, so no mechanism prefix is attached;
- one resolution rule for every language: one known target is `exact`, several
  are `alternatives`, none is `unresolved`;
- one owner rule for every language: a call, read or callback belongs to the
  scope in which it runs. What a definition runs once (Python decorator
  arguments, defaults and annotations; TypeScript decorators; Clojure
  metadata and attr-maps) belongs to the defining scope, while the
  `decorates` relation stays the decorated declaration's. Go and C have no
  such expressions. A Clojure `def` value that is not a function is the open
  exception: it runs at load but stays the var's
  ([Clojure](CLOJURE.md#calls-that-run-when-a-namespace-loads));
- signatures as short native text for the reader and model: Go names packages
  by their last element (`model.User`), and a named type's signature is only its
  form (`struct`, `interface` or the underlying type), its members being objects;
- object aliases: a declaration's names in other formats as sorted
  `{format, name}` pairs, such as a field's JSON key;
- a declaration's `end_line`, the last line of its source, when the adapter
  knows it;
- a declaration's `code_lines`: how many lines from its located line to its
  `end_line` hold code, never a blank, comment-only or docstring line, counted
  by the adapter's own lexer or parser (Go `go/scanner`, Python `tokenize`
  with the docstring statements' spans, JS/TS the compiler's tokens without
  comment trivia, Clojure the reader's `;` comments and definition
  docstrings, C a lexer over comments, string and character literals and
  backslash continuations with preprocessor lines as code). A module counts
  its whole file; a Python module or class variable counts its assignment
  statement. Zero is unknown and is never shown as a count; validation
  refuses a negative count, a count without a location or one larger than the
  located range;
- a callable's `unreachable`: the adapter's proof that nothing this program
  runs reaches it, set only by an adapter that sees every way its language
  reaches a callable (C, whose functions run only when running code names
  them). Absent claims nothing, which is every other adapter's case:
  reflection, dynamic attribute lookup, computed property names, `resolve`
  and interface calls the platform makes can reach what no relation names.
  Within one index the proof is whole: an adapter that marks any callable
  of a program has decided every one of them (C marks nothing for a library
  or a program it cannot prove), so there an unmarked callable is one that
  program can reach. The report joins these saved marks across programs by
  declaration identity to name the programs that run what another never
  runs (REPORT); no graph is walked for it.
  Validation refuses it on a declaration that does not run (a type, a
  variable, a module, an external symbol);
- a callable's `macro`: the declaration is a macro, code the compiler expands
  where it is written, so a use of it is no runtime relation and the adapter
  records none (Clojure's `defmacro`, which clj-kondo marks). The places
  graph carries it on the declaration; the role split's helper question
  reads it as a kind of its own (READING). Validation refuses it on a
  declaration that is not callable;
- a callable's `parameters` and `results` in order, each `{name, type,
  type_id}`: the type as short text and, when the value carries a repository
  type (through pointers, slices and arrays in Go; `list[X]`/`Optional[X]` in
  Python; arrays and promises in JS/TS), that type's object. A value has a
  name, a type or both; an adapter that knows no types leaves them out;
- source-anchored enclosing control statements on individual call patterns;
- call-result and receiver identity;
- receiver-origin provenance and its resolution;
- positional and keyword arguments;
- literal, template, dynamic, and object-backed values;
- reconstructed value candidates and their source-object/source-argument
  provenance;
- explicit external origin and repository-path authority where the language
  extractor can prove it.

Every external symbol has an explicit `authority_kind`. `package` means an
ordinary external package that may support dependency categorization and a
cross-target integration boundary. `platform` means standard-library or
runtime authority. Its raw `package_path` is still retained exactly, but it
cannot receive `dependency`, support a dependencies-lane group, or become a
cross-target boundary as a package object. A source call through a standard-library
transport can independently support runtime communication in the atlas review;
the package itself is not a remote participant. Other positively supported
categories remain allowed.
Adapters derive this distinction from language-owned deterministic authority;
shared stages never infer it from a path prefix or dependency-name heuristic.

ProgramIndex is the identity namespace for repository facts. A language adapter's
`SourceRef` exists only while `programindex.New` joins its input; it is not saved.
The sealed target graph assigns deterministic compact IDs: `n*` objects, `e*`
relations and scoped `e*p*`, `e*p*a*`, `e*p*a*v*` descendants. IDs follow
reading order: the launch seeds first (`main` is `n1`), then breadth-first along
every relation but imports (calls, callbacks, bindings, external invocations,
reads) in source order, each
declaration followed by its owner; what no entry reaches follows by file and
line, unlocated objects and external symbols last. Relations are numbered by
their source object, then by site, so `e1` is the first thing `n1` does. The
order is computed by `programindex.New` alone; adapters keep handing over
`SourceRef`s. Provider-facing
stages send these IDs unchanged with a closed request allowlist and restore model
rows directly against that allowlist. Only genuine request-local alternatives
that are not fact entities receive temporary `c*` refs. A model is never asked
to copy a UUID, canonical path, source location or digest.

The complete canonical target plan assigns `t1..tN` before any target graph is
built. The same target ID is reused by selected outcomes, ProgramIndex, facts,
GroupsIndex and report joins; no layer derives a second target hash. A standalone
single-target artifact therefore uses `t1`, while a consumer that deliberately
combines independently built artifacts must first bind that complete target set.
All ordinal IDs use numeric order rather than lexical order. `n*` is deliberately
target-local inside one ProgramIndex. As soon as native objects enter the shared
places/reading scope their existing identities are qualified as `t*.n*`; this is
scope, not another numbering pass. Two targets may both own `n1` without aliasing
unrelated declarations, facts, fields or boundary owners.

GroupsIndex v12 is a semantic overlay, not another fact graph. It persists only
target and ProgramIndex bindings, `g*` groups, `k*` containers, `o*` operations,
`x*` group connections, communication/data interpretations and subject
annotations keyed by the existing `n*`/`e*p*` IDs. Native names, signatures,
locations and structural edges are never serialized there. A reader combines
the overlay with its exact sealed ProgramIndex; the structural view, including
the optional native relation location, is derived in memory from that one fact
authority.

ProgramIndex does not manufacture a second stable identity for a declaration.
Cross-target report edges are explicit `x*` connections whose evidence names
the already scoped `t*.n*` or `t*.e*p*` facts. Equal names, signatures, source
locations or language-specific export tuples do not silently join targets.

## Persistence

Ordinary and standalone persistence have one format: `program-index.json` is
the complete sealed Index. Adapter inputs, parser-owned SourceRefs and a second
`program-facts` directory are not saved. Reading is strict Decode plus seal
validation; it never calls `New`, a parser, the repository or a provider.
Writing is `Encode` (complete validation, seal included) and those exact bytes;
the writer does not decode what it just encoded. The encode/decode round trip
is proven for every adapter's real output by the conformance kit
(`adaptertest.AssertSharedArtifact`), and every reader still decodes and
validates.

ProgramIndex and the GroupsIndex semantic overlay hashing use a local value copy to clear the seal;
JSON serialization reads their nested collections without copying them first.
Validation computes the
target object scope once per invocation and still rechecks every object and
the complete seal. Public snapshots and handoff isolation are unchanged; no
past validation is memoized for these publicly mutable structs. A caller
that already holds a validated value does not validate it again: the report's
ProgramPortfolio validates its indexes where it is built and at the
publication and render boundaries, and its default-entry lookups inside those
only look up.

Places collects each target's declarations, relations, seeds and compact
external-call observations together. Each saved target decodes once. Shared
native parsing is an in-memory producer optimization and never changes the
persisted graph or introduces a reconstruction cache.
Seed locations are resolved against the complete file inventory before depths
are assigned. A declaration place shared by several targets lists, as
`unreached`, the targets whose index proved it `unreachable` (graph v18): it
stays in their map of parts, but what it calls out to, reads or registers is
not theirs (READING); a part of a target's map holding nothing else that runs
leaves that map (atlas v13 `unreached`). A declaration's `object_id` is its
object qualified by the target that indexed it (`t1.n4`), since object IDs
repeat across targets: a GroupsIndex subject `n4` of `t1` finds its place by
that qualified identity. The graph keeps both the seed files (`seeds`) and, where a
launch fact names a declaration that is one of its symbol places, that
declaration (`seed_decls`): a file whose code the map of parts splits between
several parts has no one part, and the entry is then located by the part that
holds its seed declaration. Each declaration carries its adapter's
`code_lines`. Performance changes must preserve sealed graph content independently of any concurrent semantic change.

## Callable identity and observations

Callable observations from different target indexes meet at their existing
compiler-located symbol place. Incoming calls retain that place identity as
well as the target-qualified native object ID; outgoing calls retain callee place IDs for local
retrieval and, for a call through a field or a name, where the code first
stored each callee (`stores`, from the witnesses naming it by identity), the
source order a tie of the map's sentences follows (READING). Outgoing calls also retain their source column locally. An empty
unresolved target view is subsumed only by possible receiver observations at
the same exact call site with otherwise identical call facts; distinct sites,
dispatch details and independent evidence remain. Possible dispatch never
becomes exact. Generated callables retain facts without becoming description or
operation-review candidates. These canonical local keys never enter provider rows. Other atlas tables keep
their compact call projection without native call columns. Question declaration
evidence retains each original call's line, column and native API identity;
known callee/caller IDs are restored locally to source anchors. Short immediate
caller records preserve their native relationship and separate same-line sites
without importing the caller's other calls. Each declaration owns its source
evidence catalogue, carried with its selected original evidence into the answer.

Go callable bindings retain anchored literal assignments to other fields of
the same SSA receiver, independent of framework names. These observations enter
the callable's own review; neighbouring caller registrations omit their field
metadata. No field observation asserts a final runtime value or callback call.
The atlas canonically orders each binding's source-evidence set before its
identity key is built. Reordered or exactly repeated witnesses across target
views do not create duplicate bindings; argument order, distinct source sites,
field values and callback targets remain separate.

- ProgramIndex retains every source-distinct nested pattern without
  local sampling or truncation, including its exact location, neutral
  call-result/receiver provenance, any exact callback source-argument
  provenance, and reconstructed value candidates with their source-object and
  source-argument provenance. Duplicate compiler witnesses do not become pattern omissions.
  Native package/module directories are retained separately from source locations.
  Python namespace packages keep their exact directory without an invented
  `__init__.py` anchor. Relative imports into another explicitly named namespace
  portion retain their original external import authority; an unknown child of
  an ordinary local package stays unresolved.
  Repository scale is neither a cutoff nor a warning. Structural JSON overhead does not reduce semantic evidence authority; valid indexes are not truncated by former local size thresholds.

## Source values and destination reading

ProgramIndex retains source expressions for non-callable arguments,
receivers and locally observed return values. Go, Python, JSTS and C extract these in their existing parse. Parameter/capture owners, call-result anchors, constructors, field
initializers and concatenations remain source observations. `alternatives`
is a value that is one of its parts at run time, none picked: a conditional
expression, a closure's several bindings, a callable's several returns, the
initializer of a field a Python base-class method reads, one store per class
storing it (PYTHON), in
Go a control-flow join's incoming values in edge order (GO.md), and in C
the writes of a local that reach a read (C.md; Python and JSTS read a local
assigned on both branches as `unknown`, a recorded missing equivalent). C
records literals, parameters, call results, `index` (`argv[0]`), `field`
(`c->argv`) and followed locals; the rest is `unknown` with its text. Go
also records, on the value an outside call's argument is given, where the
repository types its static type names are declared (`Types`, source
anchors: `yaml.Unmarshal(buf, &config)` names `Config`), and on a field
object the same for its declared type (`Object.Types`, locations: `DBs
[]*DBConfig` names `DBConfig`); a type declared outside the corpus is
none. Python, JSTS, Clojure and C record neither (missing equivalents: a
Python annotation, a TypeScript type, a Clojure record and a C field's
struct type are not resolved to their declarations here). Neither reaches
a provider body: the compact projection keeps a value's kind and text
only, and the atlas declaration keeps its types beside the ObjectID, never
sent. The existing atlas
calls carry them locally. Compact caption/selection rows retain native API identity through their owning projection; question and boundary evidence retain safe source arguments, receivers, results and native API identity without internal IDs. Destination reading follows those native sites within retained owners,
preserves separate uses and correlated arguments, and stops explicitly at
unknown values or cycles. Flag/environment expressions are not deployed values;
possible initializers never become proven final field values. No hop/caller
quota silently drops a chain. Existing boundaries expose these anchored uses;
imports, request builders and local timers do not become communication locally.

Destination reading traverses those existing native calls and retained owners,
keeping each call site's arguments correlated. Two calls recorded at one
site are one call: a Python construction is the class's call, with its
arguments, and the call of the `__init__` it runs, recorded without them
(PYTHON), so the class's call binds the constructor's formals. One helper may yield several
source-linked destination uses; no hop or caller quota silently truncates them.
Cycles, unresolved factory results and dynamic field values retain a frontier
and its source. Flag/environment expressions identify configuration, not a
known deployed address. A possible field initializer stays labelled as such.
Only the existing boundary review assigns communication meaning. Request
builders, local timers and imports are not locally promoted into integrations.
Boundary uses survive the atlas, GroupsIndex and report. A selected address never hides another original use. [Report: external communication and data](REPORT.md#external-communication-and-data) owns the visible catalogue and source-chain disclosure.

## Declared interfaces

A call of a method declared on an external interface is one `invokes_external` relation whose target is that declared method and whose dispatch is `interface_method`. The implementation that runs there is unresolved unless an adapter observes one; a native view of the same site that found no implementation is not projected as a second, empty `calls` relation. The graph reads such a call as the declared API with `unresolved` resolution. This does not invent an implementation or turn that dispatch into an exact call. An observed repository implementation at the same site stays its own `calls` relation with alternatives.

## Compact artifact encoding

`program-index.json` stores each fact once. Observed counts are not written: every `*_observed` value is the number of retained rows plus the stored `*_omitted` value, and a zero omission, an empty collection or an absent optional value is left out. Coverage stores only non-zero object/relation omissions; everything else is compiled from the rows when the artifact is read. A witness or pattern at its relation's own location writes `"location":{}` (never a real location, which has a path, line and column), so the location is written once, on the relation; one without a location still writes none, and an artifact spelling every location out reads the same. Decoding restores the same in-memory index, including empty collections and those locations, before validating the seal, which is over the index and not over the artifact bytes.

Go may additionally provide exhaustive repository-local method-set matches when
its exact analysis input requests them. ProgramIndex stores these as exact
`implements` relations for both concrete-type/interface-type and directly owned
concrete-method/interface-method pairs. Their witnesses identify value or
pointer method-set authority. They mean language compatibility only; observed
field assignments, constructor arguments and call dispatch remain separate
relations with their own locations and unresolved frontiers.

## Native operation evidence

- Language adapters retain method/path-shaped calls, decorators, arguments,
  reconstructed values, exact targets, alternatives, and unresolved frontiers
  only as neutral ProgramIndex evidence. Protocol meaning arises through the
  atlas tables over that evidence; deterministic stages preserve the neutral
  evidence and its exact provenance.

A call pattern written in an if statement's condition may carry `branch`,
the lines of the statement that condition guards (both included): where the
code a comparison selects is written. The C adapter records it
(`strcasecmp(argv[0], "persist")` guards its block); it is no witness, never
enters a model request, and the report reads a setting's written fields
from it (REPORT). The Go, Python, JS/TS and Clojure adapters record none:
their settings are struct tags, keyword lookups or option declarations, not
compared words.

The Go, Python and JSTS adapters
retain neutral `control_context` witnesses on the existing call pattern. Go
uses the already loaded AST and types, Python annotates each parsed tree once
before its target views, and JSTS walks compiler parent nodes. Loop bodies and
Go select statements retain their original locations; channel ranges and
unconditional conditions remain syntax/type observations. One-time loop input
evaluation is outside the body context, and nested callable bodies start a new
context. These witnesses enter the existing atlas call evidence and shared
source catalogue in symbol selection and operation review. There is no
new graph, reparsing stage, worker detector or local activation promotion.
Calls inside a finite traversal do not thereby become workers. A loop without
retained calls still has no call-context observation. Go/Python/TypeScript
cumulative examples check startup, finite and persistent loops, nested callback
bodies, source locations and provider-row projection; Go additionally checks
channel/select context and JSTS checks nested-package rebasing.

The same native `SourceArgumentID` joins a callback to its exact registration
arguments in the graph and saved reading input. Those anchored literal values
enter the callback's own symbol rows; another registration or the factory's
other calls cannot supply them. An entry is named from its own registration's
words, whatever the protocol: places keeps the call word, the literals as
written and the composed address as the boundary's `words`, the model chooses
among them by closed ref, and code restores the chosen words verbatim, without
translation, whitespace normalization or trimming ([Reading](READING.md#operation-ownership)).
No code tells an HTTP verb or path apart from a command name or a topic, and a
handler with no word keeps its own name rather than an invented URL. The
service check exposed the former loss: `/hello` and `/proxy` were present in
ProgramIndex but absent from callback review, which generated `/hello-world`.
Cumulative Go, Python, JSTS and C tests check the words a route or a command
row offers (`AssertEntryWords`, the kvd preset); unknown refs select nothing.

The 2026-09-10 review also joins native HTTP facts to the original symbol
place before operation review. Observed mounted paths from chi, FastAPI,
Flask, Express and Django retain their exact source spelling and router identity;
an unrelated same-named router supplies no prefix. Mounts on directly annotated
Python parameters retain their prefix: the existing native
pattern carries the written type as a possible receiver origin, resolved in its
defining scope. Function-local imports retain their original router identity;
untyped, locally shadowed or reassigned receivers supply no framework authority
by name, and annotations add no native call edge. This closes the Freqtrade
`configure_app(app: FastAPI)` loss of `/api/v1` before ordinary model acceptance.
Native HTTP facts remain visible independently of a semantic operation label.
Python thread/schedule
registrations retain the receiving call, result identity, time/field observations
and source anchors; a later start on the same result does not become a callback
call edge. Worker/scheduled classification still belongs to model review, with
finite loops and lifecycle setup remaining explicit negative controls.

## Facts and authored documents

Python body docstrings retain both their original quote line and the exact
`def`/`class` declaration line in the existing claims artifact. Nested functions,
methods and async declarations follow the same path. Places attaches that
author quote only to the native declaration at that line; it does not borrow
a neighbouring quote or treat a function's docstring as module documentation.
The original quoted text stays an author claim in boundary and question input,
not a deterministic runtime classification. ProgramIndex objects and relations
are unchanged.

The facts stage runs built-in sqlc and configured external commands through
the same nodes/links contract in [EXTRACTORS](../EXTRACTORS.md).
Extensions supply source observations, not architecture role assignments.
These rows enter the same places graph and question table, with producer
declarations and corpus membership distinguished from compiler call edges.
There are no old-format readers.
Observed entrypoint seeds and manifest values from the existing facts result
also enter this graph, with their exact source and component context. They are
available to both Learn proposals and question retrieval without requiring a
key-symbol interpretation. Native launch identities stay local; no framework
command, call edge or new architecture group is inferred. Corpus-excluded
configuration-file references remain excluded. These observations are appended
after the existing question reservoir so unchanged earlier requests can reuse
their cache entries. Supporting answer excerpts retain the original source.
Markdown documents enter that same graph as source sections, independently
of code-file groups. Question rows include bounded verbatim excerpts with
commands, links and later paragraphs, labelled as author instructions rather
than runtime evidence. Oversized sections are partitioned without omissions;
exact source lines/columns and section identities remain local. Parent heading ancestry accompanies section excerpts; file-role hypotheses retain their labelled origin. Authored instructions remain distinct from observed runtime calls.

The existing facts extractor also records SQL text and source table declarations
(SQL/sqlc and supported SQLAlchemy declarations), with columns, keys, source
scope and anchors. Query table mentions do not establish owned schema, a foreign
key or runtime I/O. Dynamic expressions remain partial; unknown connection
identity stays unknown. These observations enter the same atlas entity graph,
question evidence and compact Data catalogue with exact source links.
Unbound source literals need supported SQL statement structure after literal
concatenation; a leading English verb alone is insufficient. Explicit SQL/sqlc
sources retain their authority. Unsupported or ambiguous strings stay original
source text, never a claim that SQL or database access is absent.

The deterministic facts pass runs after native extraction over the shared corpus, sealed ProgramIndex set, dependency catalogues and manifests. It knows no framework: a `registration` is the shape of a call the repository does not own that hands something over (a callable, a value named by a literal, an address), with its call word, literals, stated verb, the callable handed over, the external symbol behind the call, and the mount prefixes observed for its receiver. A call states an HTTP verb as its word, inside a `VERB host/path` pattern whose method is a capitalised HTTP verb (`GET /health`), or as a literal of its own beside an address it qualifies, given as an argument or a record's field (`NewRequest("GET", url)`, `{Method: "GET", Path: "/users"}`, a C row `{"GET", "/health", health}`); a verb-shaped literal with no address beside it is the name of what is handed over, so a `{"get", getCommand}` row states no method. An address is a path, a URL or such a pattern; prose with a word, a space and a slash (`open /dev/null: %s`) is none, whether it is written at the call or reaches it through a parameter. A call the repository itself declares, on a value the repository itself produced, or on a class that declares the member, is delegation and never a registration. The one exception is a row of a table the repository owns (owner decision D1, 2026-09-26): a record a module-level variable's initializer constructs, storing a callable beside a string literal (C's `{"get", getCommand, 2}`), registers that callable under the literal, with the record type's field as its registrar (`kvd.h.kvCommand.proc`, the file declaring the type, the type and the field the callable is stored in; exact, since the record type is known); the same row built inside a function body stays delegation. One row is one registration: the literals it writes once, at their own source positions, are its identity, so a row that stores two callables (`{"zunion", zunionCommand, ..., zunionInterBlockClientOnSwappedKeys}`) registers the one it writes first, and the other stays a callback the row stores, never a second input under the same name. This is identity the code carries, not a decision about what a field means. Which registration is a request, a consumer, a timer, continuous work, a plugin hook or a client request is decided in the reading stage from the same closed boundary kinds, so a run without a model has candidates and no routes. The stated verb and the address stay native evidence for portals, joints and client addresses; an entry's name never comes from them (Reading). `sql_query` retains a literal handed to a call the repository does not own and the tables it names as a fixed database boundary, only when the literal passes the same SQL statement admission as unbound source literals (`internal/sqltext`): an error message, flag help text or keyword argument that merely starts with an SQL verb (`create %s dir: %w`, `with`) is ordinary text and never a database boundary, while a statement whose table a printf verb fills in (`fmt.Sprintf("DROP TABLE IF EXISTS %s", t)`) stays one and lists no table. A statement is one fact at its call as the fact reads it (its tables and its text with spacing folded): `(str "SELECT 0 AS a" " UNION ALL" " SELECT 0 AS a")` hands one statement twice and records it once. Original template holes remain parameters with possible authority; a mount prefix composed with a router's own prefix is possible, not exact. Dead modules are judged repository-wide against every selected target's seeds and only for files that declare something to run. Dynamic execution remains its own source fact and does not automatically become an incoming operation or remote participant.

Claims retain the original human-written quote, path and available date/age. Only the shallowest README supplies the repository overview claim; nested document evidence retains its heading and file-role context. No credential scanner or withheld-quote classifier is added. Configured extraction and SQL admission remain owned by [EXTRACTORS](../EXTRACTORS.md).

## External package authority

Dependencies are deterministic target-scoped facts, not model-authored
inventories. They bind exact package/import origins and applicable metadata to
the owning ProgramIndex target. Shared atlas stages can use
those facts through the ProgramIndex's external objects and relations, but a
model cannot invent an unobserved dependency.

Platform namespaces are not dependencies. Shared contracts are supporting
code, build and migration scripts are tools, and a runtime script, library, or
tool-only root does not promote itself into an application.

## Test source scope

Exact Go test inventories, authored pytest configuration, JS/TS runner facts
(`node --test` script globs, Vitest and Playwright configurations) and
Clojure test-framework requires populate Target.TestSources; each language
contract names its rules. The overview omits known test-only nodes and edges while
retaining mixed/unknown regions and the complete saved graph, questions and
source checks. Names alone do not authorize exclusion. This is presentation,
not deletion from analysis or a claim about test coverage.

## Ownership and source protection

Architectural membership is an interpretation over native declarations, not
native file ownership. Atlas v8 records the selected declaration IDs explicitly.
Projection resolves those IDs through the original source identities, rejects
unknown or conflicting memberships, and uses only native lexical owners for
inner values and objects. Equal source paths do not join independent
responsibilities. Every native subject and structural relation remains in the
ProgramIndex and is joined to the GroupsIndex overlay by its compact ID;
cross-part connections retain their original relation ID,
declaration endpoints, source locations and resolution.

Public snapshots keep their existing isolation. Shared materialization never reassigns a target, aliases package contexts, invents a callee or turns an alternative into an exact observation. Source bodies do not enter provider requests. [EXTRACTORS](../EXTRACTORS.md) owns configured extractor and SQL admission details.
