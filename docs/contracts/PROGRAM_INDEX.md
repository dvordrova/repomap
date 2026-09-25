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
- objects and their compact target-local identities;
- adapter-observed package/module directories, independent of source locations;
- exact, alternatives, and unresolved relation authority as distinct states;
- structural relation kinds such as calls, imports, implements, decorates,
  passes-callback, binds-implementation, sources, executes, reads, writes, and
  invokes-external. Containment is the object's `container_id` (and `owner_id`)
  only; there is no parallel `contains` relation;
- complete witnesses and omission counts;
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
- signatures as short native text for the reader and model: Go names packages
  by their last element (`model.User`), and a named type's signature is only its
  form (`struct`, `interface` or the underlying type), its members being objects;
- object aliases: a declaration's names in other formats as sorted
  `{format, name}` pairs, such as a field's JSON key;
- a declaration's `end_line`, the last line of its source, when the adapter
  knows it;
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
calls, callbacks, bindings and external invocations in source order, each
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
are assigned. Performance changes must preserve sealed graph content independently of any concurrent semantic change.

## Callable identity and observations

Callable observations from different target indexes meet at their existing
compiler-located symbol place. Incoming calls retain that place identity as
well as the target-qualified native object ID; outgoing calls retain callee place IDs for local
retrieval. Outgoing calls also retain their source column locally. An empty
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
receivers and locally observed return values. Go, Python and JSTS extract these in their existing parse. Parameter/capture owners, call-result anchors, constructors, field
initializers and concatenations remain source observations. The existing atlas
calls carry them locally. Compact caption/selection rows retain native API identity through their owning projection; question and boundary evidence retain safe source arguments, receivers, results and native API identity without internal IDs. Destination reading follows those native sites within retained owners,
preserves separate uses and correlated arguments, and stops explicitly at
unknown values or cycles. Flag/environment expressions are not deployed values;
possible initializers never become proven final field values. No hop/caller
quota silently drops a chain. Existing boundaries expose these anchored uses;
imports, request builders and local timers do not become communication locally.

Destination reading traverses those existing native calls and retained owners,
keeping each call site's arguments correlated. One helper may yield several
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

`program-index.json` stores each fact once. Observed counts are not written: every `*_observed` value is the number of retained rows plus the stored `*_omitted` value, and a zero omission, an empty collection or an absent optional value is left out. Coverage stores only non-zero object/relation omissions; everything else is compiled from the rows when the artifact is read. Decoding restores the same in-memory index, including empty collections, before validating the seal.

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
enter the callback's own symbol and operation rows; another registration or
the factory's other calls cannot supply them. The operation table separates HTTP
names from descriptive labels. HTTP rows select a closed p* registration ref
and a method; Go restores the selected literal path without translation,
whitespace normalization or length trimming. Non-HTTP work, or a handler with
no observed literal path, uses a descriptive label and must not invent a URL.
The service check exposed the former loss: `/hello` and `/proxy` were present in
ProgramIndex but absent from callback review, which generated `/hello-world`.
The ordinary repeat selects the original paths. Cumulative Go, Python and JSTS
tests check route/topic registration ownership; the reader test retains a long
Korean path and ignores free-text `name` in an accepted HTTP row. Unknown
registration refs reject their row only. The existing one-declaration operation
review is not a new exhaustive per-mount endpoint inventory.

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

The deterministic facts pass runs after native extraction over the shared corpus, sealed ProgramIndex set, dependency catalogues and manifests. It knows no framework: a `registration` is the shape of a call the repository does not own that hands something over (a callable, a value named by a literal, an address), with its call word, literals, stated verb, the callable handed over, the external symbol behind the call, and the mount prefixes observed for its receiver. A call the repository itself declares, on a value the repository itself produced, or on a class that declares the member, is delegation and never a registration. Which registration is a route, a consumer, a timer, a plugin hook or a client request is decided in the reading stage from the same closed boundary kinds, so a run without a model has candidates and no routes. `sql_query` retains a literal handed to a call the repository does not own and the tables it names as a fixed database boundary, only when the literal passes the same SQL statement admission as unbound source literals (`internal/sqltext`): an error message, flag help text or keyword argument that merely starts with an SQL verb (`create %s dir: %w`, `with`) is ordinary text and never a database boundary, while a statement whose table a printf verb fills in (`fmt.Sprintf("DROP TABLE IF EXISTS %s", t)`) stays one and lists no table. Original template holes remain parameters with possible authority; a mount prefix composed with a router's own prefix is possible, not exact. Dead modules are judged repository-wide against every selected target's seeds and only for files that declare something to run. Dynamic execution remains its own source fact and does not automatically become an incoming operation or remote participant.

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

Exact Go test inventories and authored pytest/Vitest configuration populate
Target.TestSources. The overview omits known test-only nodes and edges while
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
