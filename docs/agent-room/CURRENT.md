# Current product decision

Status: active living ADR

Last updated: 2026-09-02

Historical provenance: pre-cleanup commit `4e54ab3`

This is the only current architectural decision record. Change the affected
section here when the ordinary product path changes. Older decisions and
planning notes are history in Git, not current requirements.

## Product surface

`repomap` has one supported product path:

```text
repository
  -> safe repository corpus and guidance documents
  -> repository-wide target selection
  -> one typed language-adapter path per selected target
  -> one sealed ProgramIndex per target
  -> one reduced documentation handoff
  -> sparse overlapping categorization of the same ProgramIndex
  -> one GroupsIndex per target
  -> cross-target group matching
  -> deterministic facts and claims
  -> model orientation over both
  -> report.json + one static report.html
```

The supported commands are deliberately small:

- `repomap [repository] [flags]` runs the ordinary online analysis and
  publishes the report;
- `repomap cache clear [--debug-dir DIR]` clears persistent model-response
  caches.

Report serving is part of the ordinary run and is controlled by `--no-serve`
and `--port`; `--no-open` controls only automatic opening. There is no second
analysis command, replay product, developer server, or sidecar entrypoint.

The graph has one semantic data path. Language adapters provide deterministic
program facts. Repository documentation is reduced once and supplies context
to categorization. Categorization enriches the same ProgramIndex type rather
than creating another element inventory. Grouping produces the target-local
group graph. Matching adds validated connections across complete target-local
graphs. A deterministic stage then derives the anchored fact layer and the
quoted claim layer, and one model stage writes the orientation over both. Go
renders the finished page; the browser receives no analysis payload and
discovers nothing of its own.

Every target reads in one order:

```text
Inbound -> Entrypoints -> Core -> External calls and dependencies
```

`Triggers` is one presentation lane containing both `inbound` and
`background_activity`. The original category remains attached to every
subject, so an HTTP handler, CLI command, scheduler, consumer, worker, startup
hook, watcher, polling loop, or controller is not flattened into an
indistinguishable generic entry.

## Repository corpus and file identity

Every later stage derives authority from one immutable repository corpus.
Corpus collection owns repository-relative paths, tracked-file bytes,
Git-host metadata, and stable short file refs. Later stages select from that
corpus; they do not reopen arbitrary workspace paths.

Before any short file identity exists, collection excludes `.npmrc`, every
`.env*`, dependency/generated subtrees `node_modules`, `dist`, `build`, and
`coverage`, and every `*.tsbuildinfo`. Excluded files never enter freshness
state, model input, debug output, or publication.

The selected repository is trusted by default. Heuristic credential scanning
is enabled only by `--scan-secrets`. API keys and Authorization headers are
never persisted. Provider requests never contain absolute host paths,
credentials, full repository source files, raw internal graph IDs, or
unadvertised repository paths.

Repository scale is retained completely. Former local file-count, text-byte,
graph-depth, collection-size, and artifact-size thresholds are warning-only.
They cannot sample, truncate, omit, reject, or partially publish otherwise
valid repository authority. Only representation overflow, security or
canonical identity/path/format validation, an actual provider envelope, or an
explicit user narrowing option may remain a correctness boundary.

Repository changes during a run do not fail publication. There is no
freshness gate or strict-snapshot mode.

## Repository guidance and documentation reduction

The initial guidance classifier may send the complete names-only safe-corpus
dictionary as a lossless prefix-compressed path tree with compact `f*` leaves,
the complete closed set of prose-file refs derived from that dictionary, and
complete textual README and AGENTS.md documents. It sends no other source-file
contents.

The classifier restores accepted rows only to advertised file refs. Its
repository-guidance result supplies exact documentation inputs and optional
target evidence. It does not create program objects, program edges, or target
identity.

`documentation_reduce` consumes the complete validated guidance snapshot and
produces one sealed `reduced-documentation.json` handoff:

- an optional repository overview;
- source-bound claims and concepts;
- the exact guidance digest;
- the exact reduction digest.

Sparse reduced documentation is legitimate, including the canonical empty
result when there is no guidance authority. Every retained source must restore
to the exact guidance snapshot. The reduction is repository context for every
selected target's categorization request; it is not copied into adapter facts
or treated as source-code authority.

## Target discovery and selection

Every active Go and Python target scout and the JavaScript/TypeScript package
scout runs over the same corpus. Their exact native candidates and resolvable
guidance candidates are merged into one repository-wide `TargetPortfolio`
request. Discovering one supported language never suppresses another.

Each exact native target contributes one canonical required file
representative. A shared representative is deduplicated; alternatives for the
same target are not all made mandatory. The portfolio retains every required
representative, chooses one retained file as the repository default, and may
also retain positively supported guidance candidates.

Every positive file ref restores through exactly one language adapter into a
typed target plan. An exact `--target` bypasses model portfolio selection but
still resolves unambiguously through that same typed adapter boundary.
`--target` narrows selection; `--force-platform GOOS/GOARCH` overrides the
normal Go platform choice.

Scouts are corpus-only. They do not execute a selected target's compiler,
ProgramIndex, dependency, categorization, grouping, or matching path. A
compiler projection belongs only to the selected typed target, so an
unselected language prerequisite cannot block another language's target.

## Language-adapter boundary

Each language adapter owns syntax, compiler/type-system mechanics, native
target interpretation, and exact local fact restoration. It returns one
atomic, language-neutral `programindex.Input` for its selected target. The
shared `programindex.New` boundary validates and seals that snapshot.

The adapter input contains:

- the exact `ProgramTarget`, its source refs, anchor, selector, and structural
  execution seeds;
- program objects with kind, display name, visibility, ownership,
  containment, exact source location, and optional signature;
- external symbols with their exact raw package origins and required,
  language-neutral `package` or `platform` authority kind;
- language-neutral relations with resolution, targets, witnesses, neutral
  patterns, and complete observed/omitted coverage;
- target-scoped deterministic dependency authority.

Adapter-private indexes may retain richer language facts, but shared semantic
and report code depends only on the sealed ProgramIndex and target-scoped
dependency authority. Adapters do not contain framework, protocol, or product
role allowlists.

Every eligible selected target runs its real adapter. Fixture or preset paths
may replace only provider responses in focused tests; they never replace
deterministic adapter execution.

## ProgramIndex

ProgramIndex is the single typed program graph passed from language adapters
to shared stages. There is one schema and one sealed artifact per selected
target, plus a sealed `program-index-set.json` that binds the complete selected
target inventory.

ProgramIndex version 11 retains:

- exact target scope and seeds;
- objects and their stable local identities;
- exact, alternatives, and unresolved relation authority as distinct states;
- structural relation kinds such as calls, contains, imports, implements,
  decorates, passes-callback, sources, executes, reads, writes, and
  invokes-external;
- complete witnesses and coverage counts;
- every source-distinct neutral relation pattern;
- call/decorator form, selector, invocation text, and exact source location;
- call-result and receiver identity;
- receiver-origin provenance and its resolution;
- positional and keyword arguments;
- literal, template, dynamic, and object-backed values;
- reconstructed value candidates and their source-object/source-argument
  provenance;
- exact symbol-link identities suitable for deterministic cross-shard joins.

Every external symbol has an explicit `authority_kind`. `package` means an
ordinary external package that may support dependency categorization and a
cross-target integration boundary. `platform` means standard-library or
runtime authority. Its raw `package_path` is still retained exactly, but it
cannot receive `dependency`, support a dependencies-lane group, or become a
cross-target boundary. Other positively supported categories remain allowed.
Adapters derive this distinction from language-owned deterministic authority;
shared stages never infer it from a path prefix or dependency-name heuristic.

ProgramIndex IDs are canonical local identities. Provider-facing stages assign
deterministic request-local refs and restore accepted rows locally. A model is
never asked to copy a UUID, canonical path, canonical ID, or source location.

Categorization is an optional sealed section of the same ProgramIndex type.
It binds its sparse assignments to the base ProgramIndex digest and the
reduced-documentation digest, then reseals the index. Enrichment cannot change
target, objects, relations, patterns, witnesses, coverage, or value
provenance. The base structural index is recoverable and independently
validatable.

ProgramIndex aggregate semantic-text and canonical-JSON size thresholds are
warnings only. Structural JSON overhead does not consume evidence authority,
and crossing a warning threshold cannot truncate or reject the index.

## Language-specific deterministic authority

### Go

The Go fact inventory excludes non-`DepOnly` `go list` roots with no
build-selected `GoFiles` or `CgoFiles`. A directory containing only external
`*_test.go` files is not an ordinary package when `Tests=false`; its raw row
may inform dependency metadata but cannot enter package counts, target
identity, or ProgramIndex scope. A source-bearing package that fails type
checking remains selected and fails its owning target closed.

The direct-call traversal is complete by default. `--depth 0` and
`--edges-limit 0` mean retain all reachable exact calls and edges; positive
values are explicit user narrowing. Large depth, node, or edge counts emit
aggregate warnings without truncation. Dynamic and unresolved frontiers remain
separate from exact edges.

The Go fact inventory also retains the complete build-selected package-origin
universe from every `go list -deps` row, including `DepOnly` rows. The Go
tool's exact `Standard` bit maps standard packages to `platform`; every other
known external package maps to `package`, and generated cgo `C` authority maps
to `platform`. An external target absent from that universe fails the adapter
closed rather than being guessed from its import path.

### Python

The Python adapter owns package/module scope, import restoration, call and
registration facts, decorators, arguments, target seeds, external origins,
and complete observed/omitted coverage. Dynamic dispatch remains alternatives
or unresolved authority; shared stages never repair it by matching names.
It maps an exact top-level import root in `sys.stdlib_module_names` to
`platform` and every other external root to `package`; an invalid or missing
authority kind fails the adapter boundary.

### JavaScript and TypeScript

Every eligible `package.json` project is considered. Source ownership belongs
to the deepest containing manifest. Every manifest with owned tracked
JavaScript/TypeScript source contributes one required target representative;
a source-less manifest is tooling and cannot suppress another target.

`package.json#name` is optional. An exact top-level lockfile name is the
secondary identity; otherwise the root uses `root-package` and a nested
project uses its repository-relative directory. These fallbacks never
authorize an implicit string-form `package.json#bin` command.

An explicit `jsts:<manifest>` narrows the typed plan before compiler
execution. Each retained package target receives its own compiler projection.
The owner-prepared repository-local TypeScript Compiler API honors
`tsconfig.json` or `jsconfig.json`, repository-confined solution project
references, aliases, and module resolution. An in-package project reference
extends the page graph; an exact reference into another selected package is a
cross-target boundary. Missing references and references outside the analyzed
repository fail closed.

Repomap never installs npm, yarn, pnpm, or other packages. Compiler authority
comes only from `typescript` or an exact npm alias declared by the selected
manifest. A nested package inherits root candidates only when it declares none.
Each candidate resolves from its declaring package scope and must identify
itself as TypeScript. Distinct compatible candidates in the selected API tier
fail closed; one stable legacy Compiler API candidate is preferred when a
native-preview candidate is also deliberately declared.

Browser, Node server, and canonical safe package-owned `package.json#bin`
command/path pairs are product surfaces inside the owning target. A CLI entry
never invents a wrapper-to-source relation. An exact canonical `dev` or
`start` script with one helper-selected source may seed that source only after
CLI product authority exists.

Compiler/type-resolved declarations and exact external imports are the only
call-target authority. TypeScript default-library declarations and exact Node
standard-library origins map to `authority_kind: platform`; npm origins map to
`authority_kind: package`. Their raw origins, including the closed
`platform:javascript` namespace, remain unchanged. The shared platform policy
then excludes only dependency and integration-boundary authority while
retaining structural evidence and any other positively supported category.
Calls and constructions remain distinct. An unresolved property name stays
unresolved and is never joined to a repository declaration by name alone.

Method/path-shaped calls retain their selector, receiver and external origin,
positional arguments, callback refs, reconstructed values, and unresolved
frontiers only as neutral ProgramIndex patterns. The adapter does not build a
parallel route catalog, HTTP-use catalog, resource catalog, product path, or
deterministic HTTP join. Target-local meaning comes from categorization and
grouping; cross-target connections come only from repository matching over the
complete GroupsIndex set.

Optional signatures and source-expression display text matching the
always-on persistence guard are removed before initial ProgramIndex sealing;
identity, source location, resolution, targets, and witness counts remain.
Sensitive required identity or semantic fields still fail closed.

## Dependency authority

Dependencies are deterministic target-scoped facts, not model-authored
inventories. They bind exact package/import origins and applicable metadata to
the owning ProgramIndex target. Shared categorization and grouping can use
those facts through the ProgramIndex's external objects and relations, but a
model cannot invent an unobserved dependency.

Platform namespaces are not dependencies. Shared contracts are supporting
code, build and migration scripts are tools, and a runtime script, library, or
tool-only root does not promote itself into an application.

## Sparse overlapping categorization

`program_categorization` receives one base ProgramIndex and the exact reduced
documentation handoff. It advertises request-local refs for ProgramIndex
objects and relation patterns and returns zero or more assignments from this
closed vocabulary:

- `inbound` — externally delivered work such as HTTP/RPC requests, event or
  message subscriptions, webhooks, and equivalent request surfaces;
- `background_activity` — scheduled, startup, consumer, worker, watcher,
  polling, reconciliation, CLI invocation, and other independently initiated
  activity;
- `core` — domain responsibilities and coordinating application behavior;
- `dependency` — external systems, storage, services, packages, and boundary
  use.

The result is a sparse cover, not a partition. Each disjoint request is sparse
in rows but complete for its positively supported assignments: it inspects
every owned ref, is neither top-k nor a sample, and may return empty only when
none of those refs has a positively supported category. This does not require
an acknowledgement row for every ref. One subject may carry several
categories; an uncategorized subject remains in ProgramIndex without a
synthetic fallback. Unknown refs, malformed rows, empty category sets, and
invalid categories are discarded and diagnosed locally. A model-selected
`dependency` category that contradicts explicit `authority_kind: platform` is an
unsupported set member: only that category pair is discarded and diagnosed,
other valid categories on the same row remain, and the sealed ProgramIndex
rejects any reintroduced copy. Known accepted rows are canonicalized and
applied directly to the same ProgramIndex, which is resealed with both input
digests.

Categorization plans a deterministic disjoint cover of at most 32 owned
subjects per request and repeats the complete reduced documentation while it
fits. This is a semantic-focus partition, not a count limit: every subject is
owned by exactly one request for each required documentation shard, incident
graph context remains closed, and accepted positive rows are unioned without
sampling or truncation. If complete documentation cannot fit beside every
indivisible subject, only the documentation reservoir is losslessly sharded
and the exhaustive subject cover is repeated for every shard.

Categorization does not require every object or pattern to receive a class,
does not create groups, and does not construct the presentation layout.

## GroupsIndex

`program_grouping` consumes one complete enriched ProgramIndex. GroupsIndex
version 4 carries the required external-symbol authority kind and exact
per-member lane compatibility through the
self-contained downstream graph. Grouping proposes a
sparse set of developer-facing responsibilities and directed semantic
connections within that target. Go restores every request-local member and
evidence ref before `groupindex.Build` assigns stable identities and seals
`groups-index.json`.

Every group has:

- stable ID, title, and short summary;
- exactly one lane: `triggers`, `core`, or `dependencies`;
- one or more exact ProgramIndex object/pattern members;
- zero or more exact evidence subjects.

Lane selection is locally constrained for every direct member by its accepted
subject categories: `inbound` and `background_activity` support `triggers`,
`core` supports `core`, and `dependency` supports `dependencies`. One
compatible member cannot promote an incompatible peer into its lane. The two
trigger categories share the `triggers` lane. Membership is sparse and
overlapping: a multiply categorized exact subject may support several useful
groups, and uncategorized or ungrouped subjects are not deleted. Evidence may
cite advertised context, but it never becomes group membership. For a
`dependencies` group, explicit platform objects and exact invocation patterns
whose complete targets are platform authorities are unsupported evidence and
are discarded individually; standard-runtime APIs cannot substantiate an
outbound dependency group. Local callers and other advertised context remain
valid evidence, and connection evidence is unaffected by this group-lane rule.

GroupsIndex is self-contained for matching and report projection. It retains:

- the exact target and enriched ProgramIndex digest;
- every ProgramIndex object and relation pattern as a typed subject, including
  subjects with no group membership;
- accepted categories on each subject;
- object, pattern, argument, receiver, result, external-origin, value, and
  source-location facts needed downstream;
- deterministic structural edges derived from exact ProgramIndex structure;
- model-authored semantic group connections with exact evidence endpoints;
- its canonical seal.

Grouping and matching return canonical diagnostics for rejected proposals
alongside the graph; diagnostics are not graph nodes or connection authority.

Structural edges and semantic group connections remain distinguishable. The
former restate adapter authority; the latter are validated, evidence-bound
explanatory relations. A report projection cannot turn either kind into a
stronger claim.

Grouping may use provider-sized disjoint batches and a deterministic merge
round. Initial shards remain sparse positive selections. A merge is lossless
consolidation of their validated membership: for every candidate group, one
returned group in the same lane must contain that candidate's complete member
set. A single returned group can cover arbitrarily many candidates; no
candidate `g*` acknowledgement rows are required. The check runs after closed
ref filtering and canonicalization. Every candidate member has already been
validated individually against its lane; merge cannot use another member to
promote an incompatible ref. A member may additionally appear in another group
only when its own categories support that lane, and that duplication does not
replace the candidate's same-lane containment. An incomplete merge rejects the target's
complete grouping result instead of locally restoring or promoting omitted
members. Every accepted batch is restored against the same complete input.
Unknown members, unselectable categories, unknown evidence, conflicting group
keys, lane-invalid groups, or invalid connections are discarded rather than
guessed. A terminal batch failure rejects the target's complete grouping
result.

## Cross-target group matching

`group_matching` consumes the complete validated GroupsIndex set and no
separate program or documentation authority. It assigns request-local refs to
targets, groups, subjects, arguments, reconstructed values, structural edges,
and existing target-local connections.

Every unordered pair of groups belonging to different targets is considered
exactly once to derive its complete deterministic witness set `J(pair)`. For a
candidate-bearing pair, the request is an exact pair-local dossier rather than
a repeated copy of both complete target graphs:
the two endpoint groups retain their complete model-selected member/evidence
sets and separate deterministic `boundary_edge_refs`. Every boundary ref names
an existing GroupsIndex structural edge with role `pattern_target`,
`pattern_receiver`, or `pattern_receiver_origin`; its source is a pattern and
its destination is an exact non-platform external package symbol. The pattern
is group-owned only when its local source object is itself an object member or
reaches an object member through a finite, cycle-safe chain of exact
OwnerID/ContainerID facts. Evidence subjects are never roots; calls, structural
adjacency, local semantic connections, paths, frameworks, names, and selectors
never enter the ownership closure. This allows an outer service function to
remain the readable card member while a boundary in its nested callback or
call-result object is available to matching without being promoted into
presentation membership.

An eligible boundary edge normally has exact structural resolution. One
narrow language-neutral exception represents dynamic inbound or background
registration without a framework allowlist: an `alternatives`
`pattern_receiver_origin` edge is admitted only when its endpoint group is in
the `triggers` lane, the pattern itself has an accepted `inbound` or
`background_activity` category, and its retained receiver-origin set contains
exactly that one external symbol. This is possible boundary support, not an
exact framework identity, binding, runtime call, or request occurrence.

Subjects retain the deterministic closure of every member/evidence ref and
every advertised boundary edge's pattern and external endpoint, plus incident
local-connection evidence. Structural context retains the advertised boundary
edges and every one-hop edge incident to the endpoint
member/evidence/boundary-pattern subjects, together with the counterpart and
nested provenance owners required to close those refs. Target-local semantic
context retains only connections incident to an endpoint, with compact
neighboring-group facts.

Before any provider dispatch, Go exhaustively combines every eligible left
boundary edge and source-pattern-owned argument with every eligible right
boundary edge and source-pattern-owned argument. It keeps exactly the pairs
whose direct or locally reconstructed literal/template values are equal and
whose boundary/value resolutions can support a connection. Each surviving
pair becomes one canonical request-local witness candidate with a closed `j*`
ref. The `witness_candidates` catalog exposes the candidate's two boundary
edge refs, source-pattern refs, argument refs, and locally derived
`support_resolution`; the model never has to reproduce paths, values, or the
expanded witness tuple. Group evidence can explain a boundary but cannot lend
its boundary authority to an endpoint or become an ownership root.

Two possible boundary edges cannot form a candidate. One possible
semantic-trigger edge must be opposite an exact edge and the shared argument
value must be exact; its candidate remains possible. Two exact boundary edges
may still produce possible support when their equal value has only possible
reconstruction authority. Candidate construction is exhaustive and
deterministic, with no count cap or model-free semantic promotion: a `j*` row
is only admissible evidence for a possible model-authored connection, not a
connection in the graph.

Only a group pair with a non-empty `J(pair)` becomes an indivisible provider
item. A zero-candidate pair contributes no connection locally and makes no
provider request, cache lookup or write, or observer call. This absence is not
a negative semantic fact, a model answer, or a semantic fallback; it means
only that the closed local authority contains no admissible witness that a
model could select for that pair.

The model may return a directed semantic connection only between the advertised
endpoint groups, and only when those groups themselves retain opposite sides of
one direct integration boundary. For each proposal it authors the direction,
open snake-case semantic kind, label, and summary, then selects one or more
advertised refs through `witness_joint_refs`. It cannot invent an expanded
witness, supplementary subject evidence, a third endpoint, or a stronger
resolution. Pair ordering has no direction authority: `from` must be the actor
named by the semantic kind and the grammatical subject of the label/summary,
while `to` is the acted-on endpoint. A backend listed on the left does not
become the caller of a frontend merely because of request order. In the narrow
case where one `j*` joins a positive inbound delivery pattern in a
triggers group to a positive dependency-category exact outbound
`invokes_external` call, Go includes closed `required_from_group_ref` and
`required_to_group_ref` fields on that candidate. Normalization rederives this
orientation and discards a contradictory row; it never flips or rewrites the
model edge. A dual-role subject, arbitrary background activity, or bare
trigger-lane membership has no direction authority. Sparse empty output is a valid result: omission of a candidate is
not a negative fact, and no candidate is promoted without model selection.

Go resolves every selected `j*` ref inside that exact pair, revalidates its
boundary ownership, argument ownership, equal-value authority, and resolution
against the same GroupsIndex facts, and discards unknown or invalid refs. It
automatically restores both source-pattern subjects of every selected
candidate as bilateral connection evidence. Compatible selected candidates
are merged, and the strongest surviving resolution is persisted on the
connection as `support_resolution: exact|possible`. A row with no surviving
selected candidate is discarded before compatible accepted rows are merged.
Possible support is never promoted to an exact runtime call, binding, framework
identity, or occurrence. The underlying value candidates and their resolutions
remain in the automatically restored pattern evidence.

This closed selection contract prevents separate incomplete rows, local
call-chain adjacency, a handler's upstream domain logic, downstream UI
consumers, similar model/type names, and other indirect relationships from
becoming cross-target edges. Unknown pairs, invalid endpoints, malformed rows,
unknown witness refs, and legacy expanded-witness rows are discarded with
diagnostics. Candidate-bearing items execute through the shared bounded worker
pool. Their complete eligible batch remains atomic: a terminal item or
indivisible request/response envelope failure rejects the complete matching
result instead of publishing accepted siblings as partial graph authority.
Matching never creates new targets, subjects, groups, structural program
edges, arguments, reconstructed values, or locally unsupported value equality.

The matched GroupsIndex set is the full graph consumed by publication. There
is no second graph, report-only matching pass, or browser-side semantic
reconstruction.

## Facts, claims, and orientation

`docs/CONSTITUTION.md` is the product constitution and defines three labeled
layers of truth. The group graph is the model layer; it sits on top of the
deterministic fact layer and never replaces it.

`facts` is a deterministic stage over the repository corpus, the sealed
ProgramIndex set, the dependency catalogs, and the manifests. It runs after
matching and writes one repository-wide `facts.json` into every successful
backing run. Its closed row kinds are entrypoints from adapter seeds, HTTP
server routes and client calls with their method and path literals, cross-target
portals, environment-key config reads, risk calls, manifest rows, TODO markers,
file-level imports, dead modules, negatives, and dependencies. Every row carries
an exact anchor and a stable id derived from its root, kind, anchored path, the
anchored line content, and its principal literal. A template path keeps its
holes as parameters and is `possible`, never exact. A portal exists only when
one call matches exactly one route of another target; zero or several
candidates produce a diagnostic instead of a fact. Environment files are never
read: only the tracked path may be recorded.

`claims` quotes human-written text with its source path and, when known, its
date and age: README paragraphs and headings, docstrings and doc comments,
marker comments, and recent commit subjects. Only the shallowest README speaks
for the repository on the overview. A quote whose text matches a credential
shape is withheld and counted.

`orientation` is one model-assisted stage over facts, claims, and the complete
matched GroupsIndex set. It returns one repository summary, one role per
target, a run recipe, and one main flow. The model selects request-local refs
that Go restores to exact fact, claim, and subject ids. Validation is a pure
function over the response and the advertised catalog: a row with an unknown or
wrongly typed ref, a recipe step with no manifest or entrypoint evidence, or a
flow step naming another target's member is rejected with its raw JSON and a
reason into `rejected.jsonl`. Rejection never aborts the run, nothing is
repaired or promoted, and an all-rejected response yields the legitimate empty
result. The stage caches on its stage identity, prompt version, and input
digests through the shared executor.

## Multi-target orchestration and failure isolation

Every selected typed target runs its complete target-local path: preparation,
adapter projection, base ProgramIndex validation, categorization, enriched
ProgramIndex validation, GroupsIndex construction, and page validation. A
non-default target is not a structural substitute.
The page-local `ProgramPortfolio` is only the deterministic target plus
ProgramIndex presentation projection. Its entries carry no semantic
availability or `structural_only` discriminator: the enriched ProgramIndex and
GroupsIndex are mandatory, separately sealed authorities for every successful
page.

A selected target may fail at preparation, program analysis, dependency
analysis, semantic analysis, or page validation without erasing completed
sibling pages. `target-outcome-portfolio.json` contains one exhaustive,
adapter-neutral row for every selected target. A row either binds one complete
validated ProgramTarget/page/run or carries only a closed public failure stage
and reason. It never persists raw errors or adapter-native refs and never turns
a failed target into a partial page.

`program-page-portfolio.json` is the complete language-neutral binding from
every successfully analyzed ProgramTarget to its safe child run. It has an
explicit logical default and does not infer one from slice order. Both
portfolios are sealed and persisted identically in every successful backing
run.
The same artifacts are mandatory when the repository selects only one target:
that run contains a one-page ProgramPagePortfolio and one exhaustive outcome
row, with no direct-page or browser-synthesized fallback.

JavaScript/TypeScript compiler materialization occurs inside each selected
package boundary so a missing compiler cannot preflight-fail unrelated
targets. A shared Go workspace is only an optimization: if union preparation
fails, the current exact target is retried locally and later Go targets remain
isolated. A target-local fallback workspace never becomes sibling authority.

Context cancellation and complete-portfolio, persistence, manifest,
repository-overview, graph-set, or publication failures remain terminal. If at
least one target succeeds, the successful page portfolio remains valid even
when it has one page. The first deterministic successful page owns the one
physical HTML; the originally selected default remains the logical default in
the outcome portfolio. If every selected target fails, diagnostics are
retained but no targetless report is invented.

## Persistence and publication

Manifest version 39 binds the exact material inputs of the current path. A
successful target run persists, as applicable:

- repository corpus and repository-guidance authority;
- `reduced-documentation.json`;
- the enriched ProgramIndex artifact and `program-index-set.json`;
- the target-scoped `dependency-catalog.json` artifact;
- the matched `groups-index.json` artifact;
- `program-page-portfolio.json`;
- `target-outcome-portfolio.json`;
- `facts.json`, `claims.json`, `orientation.json`, and `rejected.jsonl`;
- manifest-bound `report.json`;
- the single owner `report.html`.

Multi-target publication preserves every successful child run's backing
manifest, ProgramIndex, GroupsIndex, and report JSON. Exactly one physical
HTML is published in the deterministic owner run. Its contents are derived
from verified backing data, never by merging child HTML. In served mode,
sibling target URLs are virtual projections of those backing artifacts.

The report is one static page rendered in Go from the run's own verified
report data. Its overview answers what the repository is, which targets exist,
how they talk to each other, what is missing, and how to run it; each target
then reads inbound routes, entrypoints, core responsibilities, external calls
and dependencies, the main flow, risks, configuration, dead code, and TODOs.
Provenance is visible: facts are plain, model sentences carry a model badge,
and claims are quoted with their source and age. Anchors are `path:line` links
to the captured revision on GitHub or GitLab, or to the local editor opener in
served mode. The page embeds no analysis payload; it is stamped with the digest
of the report.json bytes it was rendered from. The Canvas renderer, the browser
payload projection, and the per-target chunk transport were removed.

The target picker keeps failed rows visible, red, disabled, and linkless. The
repository overview reports analyzed versus selected coverage.

Semantic output and the current HTML report are canonical English. There is no
`--lang` flag until a separately approved final presentation-localization
stage exists.

## Source links and report server

`--no-serve` requires resolvable GitHub or GitLab source links and fails in
preflight with corrective flag guidance otherwise. A served report may add
only manifest-authorized local VS Code opening. It does not expose browser APIs
for workspace reads, symbol lookup, source context, analysis control, or run
selection.

The local server validates manifest identity and every requested source path.
Static reports use hosted source URLs derived from the same exact locations.
No successful report contains deliberately inert source links.

## Model execution contract

Each model-assisted stage owns its state, complete input authority,
provider-sized request preparation, prompt, response schema, restoration, and
semantic validation. Static prompt prose lives in readable Markdown beside
the owning stage and is compiled with `go:embed`. Go owns dynamic reservoirs
and deterministic partitions; the provider layer does not own domain prompts
or schemas.

The shared LLM executor owns exact provider requests, transport, retries,
provider-envelope and JSON decoding, adaptive batch execution, cache,
accounting, and journal events. Current stages share a 32 MiB request envelope,
a 16 MiB decoded-response ceiling, and request up to 128,000 output tokens;
the configured provider ceiling remains authoritative when lower.

Complete reservoirs are processed through as many deterministic disjoint
batches and convergent closed-ref merge rounds as needed. Composite input is
exhaustively repartitioned when a prepared request does not fit. An indivisible
oversize item or real provider response/output envelope failure is terminal
unless the owning stage defines a lossless adaptive split. It never authorizes
truncation or partial publication.

Independent planned items execute through a bounded worker pool of four. The
caller's item index is the only in-memory result slot and observer events replay
in that order. Every provider attempt acquires the run-shared adaptive gate. An
HTTP 429 collapses the gate to one before existing backoff/retry, so started
attempts finish while that retry and all new attempts become serial. A terminal
item error cancels queued siblings; the owning stage rejects the complete
batch. Already accepted sibling calls may retain exact identity-bound cache
entries but never become a partial semantic result.

A stage returns a fully validated result, a contractually legitimate empty
result, or an error. Orchestration, reporting, and the browser cannot provide
semantic fallback, repair, promotion, or partial success.

## Request-local refs and validation

Models select only closed request-local short refs. Provider catalogs may show
exact repository-relative paths, file names, symbol names and signatures,
dependency names, neutral pattern shapes, and reconstructed values when that
context has semantic value. The model never supplies canonical identities.

Unknown refs have no authority and are discarded rather than guessed,
repaired, or clarified. Set-valued outputs are filtered to advertised refs and
deduplicated locally; a model is not required to echo each selected ref exactly
once. Unknown keyed rows are likewise discarded. If filtering leaves a
mandatory scalar or complete required assignment unresolved, the stage rejects
the result instead of inventing a replacement.

Persistent caches remain on the ordinary path. Every hit is bound to exact
stage state, prompt, request, schema, provider configuration, and relevant
input digests, then decoded and validated again before use. `--no-cache` is the
explicit live-provider bypass. Debug artifacts never contain API keys or
Authorization headers.

## Development contracts

Focused discovery and ProgramIndex regressions use exactly one cumulative real
repository fixture per active language under `testdata/repositories/<language>`.
New scenarios extend that fixture and its exact tracked-file inventory.
Deterministic adapter stages run for real; provider stages use exact
request-bound, fail-closed local presets with no network. Fixture success is
focused evidence, not product acceptance.

The acceptance fixture `fixtures/python-tutorial-game` is a tracked copy of
that repository at revision `78714d34ee` with its `expected.json` and the
sealed ProgramIndex and dependency artifacts of one real run. A focused test
rebuilds the fact layer from those inputs and asserts every expected row with
its anchor. It is the constitution's acceptance list; a real online run and a
browser read of the published page remain the product acceptance.

`testdata/contracts` inventories every static prompt and its owning
`go:embed`, every production hard-bound symbol, and every product-root Go or
repository-owned JavaScript/TypeScript test with non-ordinary requirements.
The inventory and its contract test change together. These inventories do not
create another product path.

Canonical `make test` and `make vet` use ambient Go build and module caches,
cap package-level parallelism at four, and limit each test binary to five
minutes. Focused commands keep the same or tighter bounds. Owner-facing builds
use `make build` and produce `.bin/repomap`.

## Acceptance

A product change is accepted only when:

1. `make build` produces `.bin/repomap`;
2. that binary completes a normal online provider run on a real repository;
3. the process exit status is verified;
4. repository guidance, `reduced-documentation.json`, every enriched
   ProgramIndex, `program-index-set.json`, every matched `groups-index.json`,
   both target/page portfolios, every backing manifest and report JSON, and
   the owner report HTML are inspected directly;
5. a multi-target run proves that every successful backing page validates and
   exactly one physical report HTML exists;
6. focused tests and vet for changed packages pass.

For cache changes, acceptance also verifies a real second run and
`repomap cache clear`. Offline runs, fixtures, model-response replay, and
helper probes do not replace ordinary online acceptance.

Contributor-only browser QA serves the narrow owner run root and its backing
sibling directories through a temporary loopback-only HTTP server, then opens
the owner report URL. This is an inspection procedure, not a product command
or checked-in sidecar.
