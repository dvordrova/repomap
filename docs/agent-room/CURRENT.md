# Current product decision

Status: active living ADR

Last updated: 2026-08-26

Historical provenance: pre-cleanup commit `4e54ab3`

This file is the only current architectural decision record. When code and
product scope change, edit the affected section here instead of adding another
numbered decision. Older decisions and planning notes are history in Git, not
normative dependencies.

## Product surface

`repomap` has one product path: ordinary online analysis of a repository,
ending in a sealed ProgramIndex set, downstream cube artifacts, manifest,
report JSON, and report HTML.
The report may be opened and served by that same run, or generated as static
HTML with `--no-serve`. `--no-open` independently controls whether repomap
opens the resulting report for the user.

Serving changes only code-link resolution. A served report uses the local
microserver to open manifest-authorized source files in VS Code. A static
report links the same sources to GitHub or GitLab and has no browser API for
workspace reads, investigation control, symbols, source context, or saved-run
selection. If `--no-serve` cannot resolve a GitHub or GitLab repository from
`origin`, and no explicit hosted URL was supplied, the command exits during
preflight with an actionable usage error before repository capture or model
requests. A successful report must not contain deliberately inert code links.

The ordinary console reports measured wall duration at each completed
observable pipeline boundary: language target discovery/projection,
repository-guidance classification, the exact FileRef merge, TargetPortfolio and
typed target-plan restoration when invoked, ProgramIndex and dependency construction,
semantic cubes, repository authority, and report publication. Parallel
first-layer branch durations overlap and are not additive. The TargetPortfolio
cube bypassed by an explicit `--target` is reported as not needed without a
fabricated duration.

The only separate command is:

```text
repomap cache clear [--debug-dir DIR]
```

There are no offline, investigate, doctor, dev, replay, experiment, or
`serve` command paths. There are no product or developer script entrypoints.
Tests may use test-local helpers, but they do not define another product.

## Ordinary CLI

```text
repomap [repository] [flags]
```

Initial target discovery retains both exact native targets and high-recall file
alternatives. By default the target-portfolio cube must retain one canonical
file representative for every exact native target, may positively select
additional repository-guidance candidates, and chooses one retained file as the
default. Alternative or guidance-only hypotheses omitted by the model are
restored locally as `Unclassified`, logged, and dropped. `--target` bypasses the
TargetPortfolio model decision and
narrows the run to exactly one explicit target, but it does not bypass the
parallel README file-role classifier or language target scouts. Page-local
compiler projection, ProgramIndex, dependency, and semantic execution starts
only for the selected typed target set; an unselected language's page
prerequisite cannot block an exact target owned by another adapter. There is no
`--all-targets` mode switch. For every non-explicit merged candidate set,
including a one-file set, a fully
validated live or cached positive selection is required: request-building,
provider, transport, secret-boundary, or response-validation failure ends the
run before report publication. There is no local-default degraded portfolio.

An exact typed Go candidate key (`module@repository-relative-module-dir::surface`)
may narrow initial Go-fact loading to that one discovered module. This is only
a deterministic routing hint: the complete key must still resolve against the
catalog built from that module before it becomes target authority. Human aliases
and opaque target refs do not receive this pre-load narrowing. An unselected
sibling module therefore cannot block a typed explicit target, while a missing,
noncanonical, or mismatched module directory still fails closed.

The supported flags are:

- `--target TARGET`
- `--force-platform GOOS/GOARCH`
- `--depth N`
- `--edges-limit N`
- `--github-url URL` or `--gitlab-url URL`
- `--no-open`, `--no-serve`, and `--port PORT`
- `--debug-dir DIR`
- `--no-cache`
- `--scan-secrets`

`--force-platform` is an explicit build-selection override. Without it,
normal Go environment and host selection applies. `--depth` and
`--edges-limit` remain explicit resource controls. An explicitly supplied
`--port` is valid only for served reports. Combining it with `--no-serve` or
with `--github-url`/`--gitlab-url` (which select static report mode) fails in
preflight with guidance to remove the irrelevant side of the combination.

Removed flags are not compatibility aliases: `--offline`, `--all-targets`,
`--go-target`, `--strict-snapshot`, `--source-episode`, and
`--no-secrets` must be rejected. Until the final presentation-localization
cube exists, `--lang` is also absent rather than promising a mixed-language
report.

## Analysis and publication

The main path is a sequence of domain cubes, not a sequence of provider API
calls. A cube is any operation with an explicit input and output; it does not
need to call a model. The intended product pipeline is:

Every cube has only three honest outcomes: a fully validated result, an empty
result that its own closed contract explicitly permits, or a terminal error.
No caller, downstream cube, report adapter, or browser code may replace failed
or incomplete semantic output with a locally guessed substitute. A missing or
invalid cache entry reruns the same cube through the live provider; cache and
diagnostic-journal availability never changes the cube's semantic contract.

1. Build one run-local `RepositoryCorpus` from the tracked Git index. It owns
   the canonical `f1..fN` to repository-relative-path dictionary and the only
   bounded file reader used by the initial scouts. Listing, executable mode,
   and recorded Git submodule links come from one Git-index read; working-tree
   files are inspected lazily when a cube reads them. Repository-state and
   snapshot construction require this exact already-built corpus authority and
   never run a second tracked-file inventory.
   Before `FileID` assignment, corpus version 2 unconditionally removes every
   `.npmrc`, every `.env*` component, `node_modules`, `dist`, `build`, and
   `coverage` subtree, and every `*.tsbuildinfo` file. Those names and contents
   therefore cannot enter repository guidance, freshness identity, language
   analysis, provider input, debug artifacts, or publication. This boundary is
   independent of the opt-in heuristic secret scan.
   Untracked and ignored files are outside this corpus and are
   not separately inventoried for repository-change authority.
2. Run independent language-specific target scouts over that corpus. The Go
   scout uses exact build-selected main and public-declaration facts; the
   Python adapter independently uses Python packaging, module, launcher,
   main-guard, executable-script, and framework facts. The JS/TS adapter owns
   every eligible `package.json` project with tracked project-owned
   JavaScript/TypeScript source. Ownership goes to the deepest containing
   manifest; each source-owning manifest contributes one mandatory native target
   representative, while a source-less manifest remains tooling and cannot
   suppress another package target. TargetPortfolio retains all exact package
   targets and chooses only the repository default. An explicit
   `jsts:<manifest>` narrows execution to that owned package before TypeScript
   compiler execution. The JSTS scout builds this exact
   manifest/source-ownership catalog directly from the corpus and does not
   invoke Node or the TypeScript Compiler API; the full compiler projection
   belongs to a selected JSTS target page. Every scout activated by exact project
   evidence runs over the same corpus and contributes to the same repository
   candidate stream. Go, Python, and JS/TS package discovery are not mutually
   exclusive: finding one supported language never suppresses another, while
   an incidental source file without that adapter's project authority cannot
   activate it. A language scout may use arbitrary private deterministic
   structures while thinking, but its first-layer handoff is deliberately only
   `[{file_ref,hypotheses:[...]}]`. A candidate is a possible file from which a
   later exact graph can recover the execution or library-usage picture of a
   top-level target; it is not yet an accepted target. Hypothesis strings state
   the evidence and preserve its strength. Private target identities, lines,
   roots, parser records, and language-native refs do not cross this boundary.
   For a conventional Python library package, the projected representative is
   its sealed import-package basis/source ref, normally the packaging manifest,
   and retains both exact packaging evidence and the public-package-surface
   fact. Project module membership never creates extra first-layer
   representatives for that view.
   The ordinary path accepts only a complete language-scout catalog. Syntax,
   dynamic-setup, ambiguity, or resolver omissions are diagnostic facts and
   stop publication instead of silently shrinking the candidate set.
3. In parallel with the language scout, the language-neutral
   repository-guidance file classifier receives the
   complete current contents of every tracked textual README and AGENTS.md,
   the complete names-only corpus
   dictionary encoded as a lossless prefix-compressed path-component tree,
   and the complete closed `prose_file_refs` set derived from that same
   dictionary, in one atomic bounded request. Tree keys are
   exact path components, nested objects are directories, and a string leaf is
   the exact `fN` for the file named by that key; joining keys from root to leaf
   restores every repository-relative path. It preselects neither headings nor
   candidate paths. README and AGENTS.md statements are treated as
   repository-authored claims for later exact code cubes to verify, not as
   verified runtime edges or implementation facts, and instructions inside
   either document are never executed by the classifier. A root README may
   describe the whole repository; a nested README is presumed to own only its
   subtree. A root AGENTS.md applies as evidence for the repository tree and a
   nested AGENTS.md only to its own subtree. Examples,
   fixtures, tests, generated, vendored, dependency, or copied documentation
   do not become independent products without explicit evidence. The result
   is a sparse multi-role catalog keyed by exact `file_ref`; one file may have
   independently supported `target_entry`, `example_entry`, `test_entry`,
   `support_tool_entry`, `configuration`, `database_asset`, `client_entry`,
   `documentation`, `deployment`, or `interface_contract` roles. Every role
   carries short repository-guidance-backed hypotheses. Internal server,
   provider, renderer, orchestrator, transport, or shared-layer implementation
   files do not become independent `target_entry` products from architectural
   ownership language alone. Prompts ask for a 120-byte hypothesis safety
   margin; the strict hard limit remains 160 UTF-8 bytes. Unknown file refs are
   discarded as ungrounded rows; unknown classes are rejected. Every
   advertised prose ref may receive only `documentation`, irrespective
   of what product or component its prose describes. A valid non-documentation
   role assigned to a known prose file is an unsupported set member: the
   reducer discards it before its hypotheses and per-file bounds gain
   authority, retains independently valid rows, and never repairs or promotes
   it into `documentation`. Unknown classes and malformed response structure
   remain terminal errors.
5. Preserve that complete rich role catalog separately. Only
   `target_entry` classifications project into the target-candidate stream as
   `file_ref + hypotheses`. An exact local resolvability gate admits only
   README target files that are themselves an advertised main/root, packaging
   basis/source ref, or another closed file ref with exact resolver authority
   in an active language adapter. Python may resolve an otherwise unadvertised
   module file to one framework-neutral module-execution view derived from its
   sealed project scope; it never widens that file to a neighboring executable
   or library view. The JS/TS adapter resolves each selected `package.json`
   candidate to its exact package-project selector; browser, server, shared,
   and tool surfaces remain facts inside that package target rather than sibling
   runtime targets. Across all active adapters, a README candidate is admitted
   only when exactly one adapter can restore it; unsupported or cross-adapter
   ambiguous target roles remain in diagnostics and cannot poison the
   portfolio. Merge the admitted README projections and every active language
   scout output by exact `file_ref` equality, doing nothing beyond hypothesis
   concatenation and deduplication. There are no target groups, anchors, fuzzy
   path joins, confidence classes, language-precedence rules, or semantic merge
   decisions in this cube. Exact adapter discovery, not incidental file
   extensions, establishes each language's project authority.
6. Run `TargetPortfolio` over the complete merged candidate list. The request
   includes `required_target_file_refs` when deterministic adapters established
   exact native targets. That field contains at most one canonical file
   representative per exact target; a shared representative may cover several
   targets, while alternative files for the same target remain ordinary
   candidates. Every required ref must be retained. Its response is exactly
   `{"default_file_ref":"f1","target_file_refs":["f1"]}` or, when no
   required target exists and no guidance candidate is positively supported,
   `{"default_file_ref":null,"target_file_refs":[]}`: refs only, with no
   target refs, request refs, model-visible versions, scores, ranks, or
   negative list. `target_file_refs` is a set-valued field: unknown array refs
   are discarded before deduplication and reduction, while every advertised
   ref is retained exactly. `default_file_ref` is a mandatory scalar decision;
   a non-null default must be advertised and remain selected after filtering,
   so an unknown or filtered-out default is terminal. Omitting any required ref
   is terminal. An accepted empty result ends the run with precise `--target`
   guidance rather than forcing an unsupported file.
   The one repository-wide request carries shared file evidence, not a
   language-native target catalog or adapter identity. After response
   reduction, each positive file ref is restored through exactly one active
   adapter; an unsupported or cross-adapter ambiguous ref is terminal rather
   than guessed. Adapter resolvers may restore several exact target views from
   one selected file, and repeated views are deduplicated by their
   adapter-qualified native identity. The selected default file must restore
   to exactly one typed target, which becomes the explicit repository default.
   All restored targets and the default are rechecked against their current
   exact adapter authority before they enter the execution plan. The common
   repository plan has no language-specific default policy: every exact native
   target is already retained, and the default controls navigation and order
   rather than coverage. The legacy Go-only selector may retain its stricter
   executable-default contract outside this shared boundary. Candidate order,
   path/name wording, README claims, and model hypotheses cannot add native
   target authority.
   Non-required candidates omitted by the model are restored locally as
   `Unclassified`, logged, and dropped; there is no `unlikely` complement. Several selected
   files may restore to the same exact typed target and are deduplicated
   locally after the model decision. For every non-explicit candidate set this
   cube must complete successfully, including through a revalidated cache hit;
   a sole required native candidate is retained deterministically but still
   needs a validated model choice of repository default. Every typed target in
   the resulting canonical plan receives
   its own complete target-local ProgramIndex, dependency, shared-cube, and
   report-page execution. The repository default controls initial navigation
   and execution order, not which selected targets receive semantic analysis.
   The coordinator never reruns a language portfolio or widens the result back
   to omitted high-recall alternatives.
   An exact `--target` runs the deterministic target scouts and
   README classification needed by downstream cubes but bypasses candidate
   merging and the TargetPortfolio model call. It must match exactly one native
   target across all active adapters; cross-adapter aliases or otherwise
   ambiguous selectors fail with exact per-language choices. No compiler,
   ProgramIndex, dependency catalog, or semantic cube runs for an unselected
   adapter. `--force-platform` is
   applied while preparing Go authority before this shared selection boundary.
   The exact Go `TargetCatalog` itself contains
   no preferred/default target and performs no module-name, sole-executable,
   or sole-library auto-selection. Every fresh Go snapshot exposes that full
   unselected catalog to the ordinary selector; omitting the selector is a
   terminal main-path contract error rather than a hidden deterministic mode.
7. Build each language-specific program index once for its exact analysis
   scenario and selected target scope. Reuse its symbols, objects, typed
   relations, dependency facts, and evidence coverage for every later cube;
   changing the semantic question must not reload packages or rebuild the
   language graph. When the typed plan includes Go targets, the adapter prepares
   their exact package scopes in one live-run-only `packages/types/SSA`
   workspace owner. Targets with one coherent Go module-resolution context
   share a single package/type/SSA universe; incompatible module roots retain
   separate universes inside that owner rather than merging equal import paths
   from independent `packages.Load` calls. Every target page receives a
   target-bound projection from its exact universe.
   Sibling packages are excluded by the projection's admitted package set, and
   a missing or mismatched workspace is terminal rather than a hidden fresh
   load. The Go adapter projects the existing package, type, SSA,
   direct-call, external-call, core-object, and dynamic-handoff lifetime into
   this handoff without starting a second Go analysis. Interface invokes retain
   their exact declared slot; a concrete SSA interface value, closed function
   value flow, or callback argument may add exact/alternative local targets,
   while every other joint remains explicitly unresolved. Partial value flow
   never contributes a candidate set: its known candidates are discarded and
   counted as omitted on the unresolved relation. When direct-call and
   external-call coverage ledgers contribute disjoint counts to the same
   caller/kind/invocation frontier, the adapter merges those counts into one
   unresolved relation instead of emitting duplicate semantic identities.
   At the language-neutral boundary, `alternatives` means one or more locally
   observed possible targets and never implies exact runtime dispatch; a
   dynamic-language adapter may therefore retain one syntactic candidate
   without falsely upgrading it to `exact`. `unresolved` retains no target.
   `programindex.Object.Visibility` is mandatory adapter evidence on every
   object; a genuinely unresolved value is the explicit closed `unknown`
   state, never an omitted field repaired by the shared handoff. When a method
   has an owner, that owner must be an exact type object; a non-type lexical
   owner cannot become receiver authority. Every adapter supplies
   measured object/relation totals and explicit per-relation target/witness
   counts; the ProgramIndex core never derives completeness from the rows that
   happened to survive projection. The Python adapter parses each
   identical selected module inventory once with an isolated standard-library
   AST worker and can publish several target views over that shared parse.
   For each selected package target page, the JS/TS adapter likewise runs one
   prepared-project TypeScript Compiler API graph, binds every selected source
   byte by SHA-256, and reuses the sealed result for ProgramIndex, dependencies,
   semantic cubes, surfaces, and cross-surface paths. Bounded project
   references inside the owning package extend that compiler graph. An exact
   repository-local reference outside the owning package is validated as a
   cross-target boundary and not traversed or folded into the page; missing
   references and references outside the analyzed repository still fail closed.
   The adapter never installs packages and never analyzes
   `node_modules` as repository source.
   Future language adapters may use tree-sitter, import resolution, lexical
   search, or conservative flow recovery, but must project through the same
   sealed handoff rather than making downstream cubes language-specific.
   Every selected view is listed in canonical `program-index-set.json` with
   its distinct ProgramIndex filename and semantic SHA-256; the set also names
   the page-local default target. Each typed repository target owns one such
   complete page-local semantic path; cross-page siblings are not represented
   as `structural_only` entries in the default page. Sharing ends at expensive source parsing: because a
   ProgramIndex seal includes its exact `Target.ID`, every selected view owns
   a separate sealed artifact even when its module inventory is identical to
   another view. Publication readiness requires the sealed set and every
   referenced index to decode and match both target ID and semantic SHA-256. A
   loose `program-index.json` cannot make a run READY. Every target page
   requires its separately persisted ProgramIndex-backed semantic artifacts;
   language-specific artifacts remain additional authority and never replace
   the shared ProgramIndex or semantic-cube chain. The shared target boundary pairs
   every corpus `file_ref` with its exact repository-relative path, retains the adapter's
   exact selector and anchor ref, and binds typed launch seeds to exact indexed
   objects and locations. Executable seeds distinguish callable, declared
   module, selected module-execution, main-guard, script, and bound-object
   launch facts;
   libraries may truthfully have no launch seeds. Python target artifacts keep
   the complete typed `basis` rows as their discovery evidence; they do not
   duplicate that evidence into a derived claim or confidence field with no
   independent downstream authority. A presentation label is not
   target or object identity. When one selected file restores to several exact
   target views in its adapter, every restored view enters the typed target
   plan. The portfolio's default file must instead resolve to exactly one view;
   ambiguity ends the run with corrective exact `--target` guidance rather
   than spending a second model request or applying a local ranking. For Go, the
   exact surface/direct-call/external-call/core-object analysis is likewise a
   required atomic stage: its original error ends the run immediately. It is
   never reduced to a warning followed by a later, less precise missing-index
   error. Snapshot, Go facts, and surface discovery require one already
   resolved valid `GoTarget`; an empty or invalid target is terminal and is
   never reconstructed by a host-side default.
8. Build a progressive target-core hypothesis over those shared facts. Every
   Go, Python, and JavaScript/TypeScript target page compiles this cube from its
   sealed page-local default `ProgramIndex`; changing the question does not
   trigger another language analysis. The first `CoreMap` pass receives the selected target identity and
   only the bounded file-role facts already accepted by
   `ReadmeTargetsScout`: exact file refs, paths, closed classifications, and
   short README-backed hypotheses. It neither rereads README files nor
   receives the complete repository dictionary. It produces a compact
   human-named repository brief grounded only by those accepted refs. When the
   classifier has no accepted role rows, this pass is skipped. The second pass
   receives that accepted
   brief plus the eligible exact core declarations projected from the same
   ProgramIndex: request-local
   refs for callables, types, and public module variables; exact
   kind/file/name/location/package/exported facts; callable incoming/outgoing
   counts; and exact available receiver/signature facts from the adapter's
   object index. ProgramIndex-backed runs additionally pass every exact target
   launch seed as closed kind/object/location context; seeded symbols carry
   their launch kinds directly, while non-symbol seeds remain context-only and
   cannot be invented as selectable refs. Every target page also binds the
   complete preceding `integration-usage.json` result into this refined
   request. Its caller/callsite/dependency facts and selected label/mechanism
   may clarify supporting effects, but retain their adapter-provided authority
   and cannot be returned as repository IDs. Python callsites remain explicitly
   syntactic-unresolved evidence.
   ProgramIndex alternative dispatch, interface implementation, decorator,
   and callback-handoff rows are retained as bounded structural context with
   their original resolution rather than converted into exact calls. The Go
   compiler additionally restores an exact ProgramIndex callable to its
   existing DirectCall node only when name and declaration path/line/column
   agree, and retains the existing Go direct-call and core-object digests for
   adapter-specific exact joins. It does not build a second Go graph. It
   produces a target-core layer grounded by exact symbol refs. Neither the
   number of refs grounding one responsibility nor the number of refined
   responsibilities is forced into a presentation quota: closed request-local
   authority and the existing request, response, and artifact byte envelopes
   are the bounds. Counts are context rather than importance. File-only infrastructure, deployment, metadata, and
   documentation can remain in the broad brief but cannot masquerade as the
   executable or library core. A ProgramIndex-backed CoreMap with no eligible
   exact core declaration is terminal even when README roles exist; it never
   republishes a file-only README hypothesis as refined program authority. The same exact
   file or symbol may support several genuinely different responsibilities;
   identity is local and grouping/name prose is model-owned. Ref arrays in
   baseline, map, and reduce responses are normalized as sets over their exact
   request catalogs: unknown refs and repeats are discarded, while a block
   left without the mandatory advertised evidence is still rejected as
   ungrounded. Exact duplicate block records after that normalization are one
   set member and are canonicalized locally. Different model-authored
   responsibilities remain distinct even when they cite the same exact files
   and symbols: their stable local block IDs bind both the semantic claim and
   restored evidence, rather than treating evidence reuse as an identity
   collision.
   When at least two refined responsibilities survive, one final bounded
   CoreMap call may group them for orientation. It receives only request-local
   block refs, exact representative context, selected effects joined through
   representative callers, exact target seeds, and the complete implicit
   unordered-pair domain of shared representatives and minimum retained exact
   call/execute hops. The wire contract is
   `sparse_positive_complete_v1`: it carries only pairs with positive shared or
   path evidence, while an absent advertised pair losslessly expands to zero
   shared representatives and no retained exact path in either direction. It
   never receives raw ProgramIndex IDs or edges. Because arbitrary repositories
   do not have a universal mutually exclusive ontology, the model returns only
   the memberships supported by its evidence, without a preset presentation
   count and without an exhaustive-echo obligation. A responsibility may
   legitimately appear in several orientation groups. Local code restores
   stable block IDs, discards unknown memberships and groups left with no
   advertised members, canonicalizes repeated membership inside the same
   group, and preserves every known cross-group membership. Exact duplicate
   model group records are one set member and are canonicalized locally;
   different names or purposes remain distinct supported orientation claims
   even when they cite the same membership set, and model-group IDs bind both
   the claim and its restored memberships. When a non-empty
   model grouping omits known blocks, CoreMap appends one deterministic
   `local_unassigned` navigation record containing the exact complement in
   refined-block order. Its fixed name, purpose, authority, counts, and report
   treatment mark it as local omission accounting: it is not a model-owned
   area, inferred membership, fallback, or catch-all responsibility. The total
   non-empty artifact therefore accounts for every refined block without
   assigning omitted blocks a meaning. An explicit empty model group array
   remains the legitimate flat-map result and does not cause a local group to
   be manufactured. Model groups are overlapping orientation facets around
   responsibilities, not new responsibilities, containment ownership,
   deployment units, or execution order.
9. Compose every target around the same semantic skeleton:
   `surface -> operation -> core`, with `operation -> effect` as a supporting
   relation. Target kind changes which surfaces and perspectives are useful,
   not the underlying data model. A service emphasizes routes, commands,
   workers, orchestration, and effects. A library emphasizes public
   construction/invocation, internal execution, central state or data
   structures, inspection, extension points, and intended composition.
   Several perspectives may reuse the same exact refs: capability, runtime
   episode, data structure, and effects are overlapping DAG projections, not
   one forced containment tree. A package such as `net/http` may be an effect
   boundary for a service while being the material of a router library's own
   core abstraction, so dependency identity alone cannot choose the role.
10. For an executable/service target, derive generic, exact
   activity-registration facts through local static reachability over that
   graph, retain only their process-root bindings and bounded evidence, and let
   the activity-surface model cube classify the currently supported closed
   registration shapes as HTTP routes, CLI commands, or scheduled jobs. The
   broader entrypoint cube may additionally select consumers, workers, server
   loops, and other useful activity starts from exact symbols; dynamic
   callback/registration handoffs remain structural `ProgramIndex` relations
   rather than being mislabeled as scheduled jobs. The backend validates refs and restores
   exact paths, symbols, literals, and handlers. Framework names, TLS use,
   dependency names, symbol
   spelling, and other local heuristics may contribute bounded evidence rows;
   no local rule may promote one of them to an activity surface or important
   entrypoint by itself.
   Every typed target page also runs the language-neutral
   `ActivityEntrypoint` cube over the same sealed ProgramIndex. Its generic
   eligibility boundary advertises exact callable launch seeds, callables with
   no fully exact incoming `calls`/`executes` relation, observed direct
   `calls`/`executes` targets of a launch seed, and every local callable at a
   callback, decorator, or implementation joint. Library targets additionally
   advertise every public callable. Self-recursion does not disqualify a graph
   root; alternative or unresolved incoming relations never masquerade as an
   exact caller. The boundary uses no language, framework, path, dependency,
   or symbol-name allowlist and grants no activity semantics locally. It also
   advertises every exact module/package object named by a module, main-guard,
   or script launch seed, as a complete disjoint catalog of at most 32,768
   request-local `aN` refs. Unseeded modules/packages are not candidates. Each
   row carries
   only exact declaration context, target launch-seed kinds, and
   aggregate exact/uncertain topology counts; paths, names, visibility, seeds,
   and counts are evidence rather than local selection rules. Requests contain
   at most 1,024 rows across at most 32 batches, use one absolute selection
   criterion across every batch, and permit at most 1,024 selected activity starts for the complete
   artifact. A resource overflow, provider error, or incomplete batch is
   terminal. `activity_refs` is a set-valued selection: unknown refs are
   discarded before deduplication and the complete-run selection limit, while
   repeated advertised refs are canonicalized locally and cannot create extra
   semantic identity. Unknown refs never trigger retry, clarification, mapping,
   or local promotion. The model may legitimately select an
   empty set, but no host-side candidate is promoted as a replacement. The
   persisted result restores every selected ref to its byte-for-byte exact
   ProgramIndex object, binds the ProgramIndex SHA-256, and retains upstream
   ProgramIndex omissions, locationless candidate counts, and the exact count
   excluded by generic eligibility so an empty result is not misread as proof
   that runtime activity is absent. `candidates_omitted == 0` proves complete
   advertisement only for that declared eligible set. A selected seeded
   module/package is an exact top-level launch anchor for scripts and main
   guards; it is not silently converted into a callable.
   Request-local activity catalogs are atomic: if bounding would omit even one
   candidate or supporting fact, the cube fails with the omission counts
   instead of publishing a plausible but partial activity map. A complete
   empty catalog and a target with no exact executable roots remain honest
   empty/not-applicable results.
11. Derive repository dependencies through language tooling. Dependencies are
   a core language-neutral entity even when their extraction uses gopls,
   tree-sitter, syntax search, or language-specific library knowledge. The
   Python adapter derives them from the already-built ProgramIndex import
   relations and classifies exact direct imports as `workspace`, `stdlib`, or
   `external`; wildcard, dynamic, or otherwise untyped imports make coverage
   partial and stop the ordinary run. The complete result is persisted as
   strict canonical `dependency-catalog.json`.
12. Classify which dependencies may be integration boundaries. The shared
   `IntegrationDependency` cube receives a complete exact dependency catalog,
   assigns fresh run-local `dN` refs to every `stdlib` and `external` row, and
   sends the complete catalog as a disjoint byte-bounded partition with at
   most 16 requests. There is no per-response selection quota: one batch may
   return every advertised known ref that meets the absolute criterion. The
   observed input bound is 4,096 candidates and the declaration input bound is
   16,384 retained packages; input overflow fails before any provider
   call instead of truncating the catalog. Complete-run selected-result bounds
   mirror those already-advertised authorities rather than imposing a second
   semantic quota; any batch failure returns no semantic result. Every selected
   dependency and all of its exact importers are restored locally. On the
   Python path this same cube also receives the exact target-bound declaration
   artifact, advertises its
   retained packages under disjoint `pN` refs in the same byte-bounded batch
   plan, and restores selected declarations into a separate authority. A
   constraint-only package row remains visible context but cannot be selected.
   The two response arrays are mandatory. Within each set-valued array,
   unknown refs are discarded before deduplication and reduction;
   known refs remain bound only to their original dependency or declaration
   catalog. The reducer never retries, clarifies, guesses a mapping, or promotes
   one authority into the other. It is deliberately high-recall:
   selection says only that concrete operations deserve inspection, not that
   an integration exists. Python persists this result as strict canonical
   `integration-dependencies.json` before CoreMap; provider, validation, or
   partial-authority failure has no fallback.
13. Build an exact language-specific external-call index and locate the paths
    and symbols that use the selected integration dependencies. Indexing is
    generic and may cover every observed external call before model selection;
    it does not assign service or resource semantics locally. The Go adapter
    projects this independently complete ledger into `invokes_external`
    relations, including explicit unresolved frontiers. Global ProgramIndex
    relation omissions from unrelated direct-call or dynamic-handoff shapes
    remain visible coverage, but do not invalidate an exact retained Go
    external operation.
14. For each target page, classify individual advertised external-call
   operations as strong integration uses rather than accepting every operation
   merely because another operation in the same caller is useful. Assign
   global `oN` refs first and send a complete disjoint partition of at most
   256 operations per request, with at most 256 selected uses per response.
   Matching the request and selection bounds makes an overfull per-batch model
   response structurally impossible. There is no aggregate operation,
   batch-count, or selected-use ceiling: the cube exhaustively plans as many
   complete request batches of at most 256 operations as the exact input
   requires. Every request retains the 4 MiB request-size, 256 KiB
   response-size, and 8,192 output-token bounds; any request-bound violation,
   provider failure, or incomplete batch outcome set returns no semantic
   result. Group the accepted operations back into their exact repository
   callers locally and persist the strict canonical
   `integration-usage.json`. Every target page passes that exact validated
   result directly into the following refined CoreMap compilation;
   the CoreMap cache and artifact authority bind its canonical digest. It is
   not a presentation-only side artifact and an empty accepted use set remains
   an explicit complete input rather than triggering dependency-name guesses.
15. Build deterministic `ActivityPath` routes without reparsing source or
   asking another model. Every target page consumes the same sealed ProgramIndex,
   exact `ActivityEntrypoint` selection, selected integration dependencies,
   and exact `IntegrationUsage` artifact. It projects retained `calls`,
   `executes`, and callback-handoff candidates once, then chooses one route per
   unique integration caller by the stable tuple: fewest hops, fewest possible
   edges, fewest callback handoffs, activity order, then edge sequence. A route
   is `exact` only when that chosen activity-to-caller prefix contains no
   alternative dispatch or callback edge; the integration operation itself
   remains separately `syntactic_unresolved`. `frontier` reports only global
   ProgramIndex object/relation omissions or an exact non-traversable relation
   whose opposite endpoint is this particular caller. Decorator or unresolved
   evidence reached in one region is never transferred to callers in another
   region; an unresolved relation without a retained target remains explicit
   graph coverage but has no route authority.
   `unconnected` means only that the retained graph has no route from a selected
   activity, never that runtime reachability is impossible. The strict
   `activity-paths.json` artifact binds all four input digests, stores one shared
   route per caller, and stores only the minimal operation tuple needed to join
   each selected use; it does not duplicate dependency, importer, label, or
   callsite rows. A route contains at most 256 steps, the complete artifact at
   most 32,768 route steps, and its canonical encoding at most 16 MiB; crossing
   any bound fails the cube rather than shortening a path or dropping a caller.
16. Bind accepted surfaces and effects to core blocks. The current binder gets
   only request-local surface, effect, core-block, selected core-object, and
   aggregate reachability refs; exact activity roots and integration callsites
   remain local anchors. It does not receive raw graph edges. A later
   bounded-logic pass may request source episodes around selected or ambiguous
   knots to establish dynamic handoffs and distinguish authoring, dispatch,
   state mutation, and effect execution. It must not reread or rebuild the
   whole program graph.
17. Produce one canonical English semantic result.
18. When the typed plan contains more than one target, run every target-local
    page to completion and seal one exact, language-neutral
    `ProgramPagePortfolio`. It binds each complete page's full validated
    `ProgramTarget` identity to its safe child run ID and names the default by
    exact ProgramTarget ID; adapter-native refs never cross this publication
    boundary. Then run one repository-level `RuntimePortfolio` cube. Its
    complete request-local catalog contains every target plus the validated
    responsibilities, activities, integration-use counts, entrypoint evidence,
    and repository-guidance evidence already produced on the target-local
    paths. The model selects only closed `t*` and `e*` refs; canonical
    ProgramTarget IDs and exact source locations are restored locally. The
    result preserves many-to-many role-to-target and
    target-mode mappings, primary/supporting/unknown prominence,
    required/optional/experimental/unknown requiredness, confidence, exact
    evidence, and the exact unclassified-target complement. It has no fixed
    target, role, implementation, or evidence-count limit; byte and token
    bounds remain fail-closed. No target-local cube, orchestration layer, or
    browser code may invent a repository runtime role or repair an incomplete
    result. Runtime role kind remains the closed vocabulary `service`,
    `daemon`, `worker`, `cli`, `supporting_tool`, and `unknown`: `library` is
    not a runnable/deployable role. A genuinely library-only target catalog may
    therefore produce the contractually legitimate empty runtime portfolio;
    no downstream layer turns that absence into a daemon, CLI, or library
    runtime role. RuntimePortfolio semantic cache identity is its exact captured
    revision, canonical run-invariant target and evidence facts, prompt,
    preparation, response-schema, execution, provider, and exact prepared
    request identity. It deliberately excludes the publication-local
    `ProgramPagePortfolio` seal, whose child run IDs change on every publication.
    Both live and cached raw responses are decoded, resolved only through the
    current request's `t*` and `e*` catalogs, semantically validated, and then
    bound locally to the current `ProgramPagePortfolio`. Persist the same
    canonical `runtime-portfolio.json` bytes in every child run, then re-render
    and finalize every page against that shared current-publication authority.
    A one-target run publishes its complete target page directly and creates
    neither page-portfolio nor repository-runtime authority.
    Re-derive a bounded `ProgramView` for every manifest-bound ProgramIndex,
    validate every seed/object/relation join by exact ID, retain canonical and
    projection omission accounting, bind them into one exact
    `ProgramPortfolio`, and publish it with the downstream semantic views in
    report JSON and HTML. The current HTML is an orientation workspace, not a
    browser for those artifacts. Its first vertical slice is
    `Survey -> Choose -> Focus -> Verify`: survey the default target's refined
    CoreMap responsibilities, choose one responsibility, focus it by joining
    exact representative declarations with selected activity entrypoints and
    bounded local ProgramView relations, then verify the claim through exact
    captured-revision source actions. CoreMap projection uses only positively
    selected exact objects and retains global ProgramIndex object/relation
    omissions as visible frontier counts; it never interprets an absent
    relation as proof of absence. Raw ProgramIndex catalogs, per-artifact
    dashboards, producer coverage grids, and a generic graph canvas are not
    product navigation. A bounded system canvas may project only
    `entrypoints -> core responsibilities -> integrations`: entrypoint edges
    require exact representative-object membership, and integration edges
    require a selected use whose exact caller has that membership. Unbound rows
    remain counted frontiers; exact-external-symbol and runtime-unresolved use
    authority stay visually distinct. When several selected uses collapse to
    one core-to-integration edge, any runtime-unresolved use makes that edge
    runtime-unresolved regardless of row order. Model-owned refined CoreMap
    groups render as transparent outer containers around the existing core
    cards; responsibility IDs, routes, exact edge endpoints, and source
    evidence remain unchanged. Node details may expose only
    validated purpose, signature, responsibility membership, bounded selected
    caller/operation/mechanism summaries, authority, and exact source actions.
    Retained display signatures render as whitespace-normalized, explicitly
    ellipsized previews so an inferred type cannot dominate the orientation
    layout; the canonical artifact and exact source action remain unchanged.
    Project each page's validated CoreMap into that same workspace: grouping,
    names, and purposes remain model-owned hypotheses, while every
    representative ID, kind, name, visibility, declaration location, module
    context, and call count is revalidated against the exact page-local
    ProgramIndex. Baseline file refs and paths are revalidated against the
    strict manifest-bound README file-role artifact; a refined file must be
    either one of those exact rows or have an eligible core-object location in
    ProgramIndex, and any overlapping ProgramTarget ref/path pair must agree.
    A missing README role artifact is valid only when CoreMap reports zero
    baseline role files. Only language-neutral CoreMap coverage enters this browser
    contract; legacy Go direct-call coverage is rejected at the boundary.
    A separate manifest-bound `ActivityEntrypointView` publishes only the
    model-selected target-page activity starts, revalidates each exact callable and
    its owner/container/launch-seed context against the same page-local
    ProgramIndex, and retains the complete candidate and upstream-frontier
    ledger. It never substitutes target seeds, names, or declarations omitted
    by the model. A separate, manifest-bound `IntegrationUsageView` publishes
    model-selected observed uses
    revalidated against the exact page-local ProgramIndex and selected dependency
    artifact while preserving the adapter's exact or unresolved authority. On
    Python pages, each callsite row remains `syntactic_unresolved`, with an
    imported candidate but no claim about Python runtime dispatch. The Python
    view also carries the separately selected package-manager candidates and
    their declaration coverage as a declaration-to-code frontier: those rows
    have exact manifest sources and package facts but no import, call, or
    runtime-use claim.
    A separate manifest-bound `ActivityPathView` then joins every such use to
    one caller-shared deterministic route. It republishes neither dependency
    labels nor operation rows: the report projection receives a deduplicated
    dictionary of exact ProgramIndex objects, one route per referenced caller, and the
    minimal operation tuple required to join an existing IntegrationUsage row.
    Connected routes begin at an exact selected ActivityEntrypoint object;
    `exact` means only retained exact call/execute edges, `possible` exposes
    alternative dispatch or callback handoffs, `frontier` exposes an authority
    boundary, and `unconnected` means no retained route rather than runtime
    impossibility. The current orientation slice does not present these
    integration-specific traces as a general repository workflow. It keeps the
    missing end-to-end mechanism capability explicit rather than inventing a
    route from local relations or adding another product screen.
    Unselected dependency and callsite candidates are not browser rows. The
    browser does not infer roles or integrations from names or paths. Its code
    actions use only the existing
    authorized router: local VS Code in served mode or captured-revision
    GitHub/GitLab links in static mode. Missing authority is an error, not a
    disabled-looking successful action. Program pages use a dedicated bounded
    shell and payload; legacy Architecture, Study, Operate, ELK, surface, source
    drawer, and localization assets are not embedded or booted there. Then
    optionally serve and open the report.

The semantic boundary is intentional. Local language adapters may hard-code
syntax and type-system mechanics such as Go calls/imports/interfaces or Python
imports/decorators, plus closed schemas, bounds, and validation. They must not
hard-code conclusions such as `TLS means important`, `this framework means
HTTP`, or `this package means integration`. Those conclusions belong to a
domain model cube operating on exact request-local evidence. A future
framework adapter is allowed only to add independently verifiable structural
evidence or resolve a runtime handoff; its presence cannot itself select or
rank a product concept.

Every typed target page runs the shared ProgramIndex-backed semantic chain and
persists `core-map.json` version 6. Both the broad baseline and
exact-symbol-refined core are retained so evidence improvements and regressions
stay inspectable, and the refined pass binds the page's validated
IntegrationUsage evidence. Language-specific Go, Python, and JS/TS artifacts
remain exact inputs or additional presentation authorities; they do not create
alternate target-selection or cross-page semantic paths. The complete target
object index stays local; only exact objects selected by refined core blocks
enter the browser-safe projection.
Activity/integration output on a library remains diagnostic until it is bound
to core operations; an external call is not presentation authority merely
because it exists. The retired
Architecture, Theme/Study, and call-family EntryCall model chains are not part
of the ordinary main path; local activity-candidate authority remains. The
renderer consumes each target page's sealed ProgramIndex through its
report-owned `ProgramPortfolio`; a multi-target publication additionally
consumes the one repository-level `RuntimePortfolio` bound through the
language-neutral `ProgramPagePortfolio`. The repository route is the initial
orientation surface only when it has at least one exact surface/path or runtime
role/unclassified target to show; an authoritative but empty repository-level
catalog never shadows a non-empty semantic program map. An explicit empty
repository route retains a direct program-map handoff. A populated repository
route shows primary, supporting, and unknown runtime roles, their exact target or
target-mode mappings, source evidence, uncertainty, and every unclassified
target. It exposes exact launch points,
declarations, bounded structural relations, source evidence, and material
uncertainty without copying the complete indexes or repeating backend semantic
validation in JavaScript. Producer diagnostics remain secondary evidence
limits rather than the primary information architecture. Every typed target
page consumes its exact ProgramIndex-bound CoreMap, ActivityEntrypoint,
IntegrationUsage, and ActivityPath projections. A Go page additionally retains
its exact outer `AnalysisTarget`; a Python page retains its Python target and
declaration authority; a JavaScript/TypeScript page retains its sealed project,
surface catalog, and cross-surface path authority. No page may carry another
adapter's language-specific authority. Dependency extraction coverage must be
`complete`; a partial catalog and its closed omission reasons remain adapter
diagnostics and terminate the shared cube path rather than becoming a smaller,
product-looking integration map.

Every `ProgramPortfolio` entry carries one closed semantic capability state.
Each typed repository target is the default of its own page and is publishable
as `program_semantic_available` only with separately material-bound `CoreMapView`,
`ActivityEntrypointView`, `IntegrationUsageView`, and `ActivityPathView`
projections for that exact ProgramTarget and ProgramIndex, plus any required
adapter-specific authority described above. A selected cross-page target may
not be downgraded to `structural_only`; missing semantic authority is a
publication error, not an empty browser fallback.

A final multi-target publication is publishable only when every child carries
byte-identical canonical `program-page-portfolio.json` and
`runtime-portfolio.json` artifacts, both bound by its manifest, and every
report projection is rederived from those authorities. Missing, stale,
noncanonical, unequal, or target-incomplete page or runtime authority is a
publication error. A complete single-target publication legitimately omits
both artifacts and does not invent sibling navigation or repository runtime
authority.

Legacy Architecture, Study, and Operate routes are not product fallbacks. A
ProgramPortfolio report accepts only an empty initial hash, exact Program
target routes, and debug-only provenance. Any other explicit hash remains
visible and produces a fatal route boundary; it is never rewritten to the
default target. A cube
with no accepted answer renders an explicit empty/unknown state; it does not
fall back to a package-name or path-derived story. In particular, TLS,
registry/config naming, a `New*Client` spelling, or a known dependency prefix
must not remain a user-visible semantic decision.

Each target-page render is fail-clean. It invalidates the previous final
manifest, HTML, and JSON names before regeneration, prepares replacement HTML
and JSON in the same run directory, verifies their complete projection and
candidate manifest, installs both files, and installs the manifest last as the
only readiness boundary. Any returned render, manifest, sibling, finalization,
or assessment error removes or quarantines all final product names and never
updates the latest link; raw analysis artifacts remain diagnostic input.
The language-neutral coordinator prepares and authorizes the repository
default page before running every sibling page. It quarantines every attempted
page on a returned failure and does not announce completion before the neutral
page portfolio, RuntimePortfolio, and final re-renders validate. A hard process
termination in that interval can still leave individually ready page manifests
without the later cross-page authority. Closing that crash window requires a
future portfolio-wide staging authority rather than another recovery fallback.

Publication parsing is fail-closed and has one semantic validator:
`ReadRunManifest` strictly restores the report, sealed ProgramIndex set, and
all downstream artifact projections. Publication readiness adds only the HTML
payload and standalone-bundle checks; it does not maintain a second
ProgramIndex or semantic-artifact validator. `snapshot.json` and `metadata.json` are
mandatory, independently strict-decoded authorities; metadata must carry the
same exact repository identity as the snapshot and cannot fill in a missing
one. READY extracts exactly one embedded `rm-report-data` payload from HTML,
strict-decodes it, and compares the complete projection with the validated
`report.json`. Its target navigation must equal the manifest-derived
navigation, and static source authority must retain the manifest-derived host,
revision, and repository-root-to-analysis-root path prefix. Manifest version
34 and report format version 65 own the current publication contract. The
manifest owns canonical `standalone_source {host,repository_url}` and the exact
optional `core_map_sha256`, `dependency_catalog_sha256`,
`python_target_catalog_sha256`, `declared_dependencies_sha256`,
`integration_dependencies_sha256`, `integration_usage_sha256`,
`activity_entrypoints_sha256`, `activity_paths_sha256`,
`js_ts_project_sha256`,
`readme_file_roles_sha256`, `program_page_portfolio_sha256`, and
`runtime_portfolio_sha256` material
bindings. A Python target catalog, its declared-dependency artifact, CoreMap,
activity-entrypoint selection, exact dependency catalog, potential-integration
result, concrete selected uses, and deterministic activity paths must be bound
together; manifest verification strictly decodes
the complete artifact chain, rederives the ActivityPath presentation, and
revalidates it against the exact page-local ProgramIndex. Old
manifest versions and the removed Atlas field are rejected rather than
adapted.

A language-neutral `program_page_portfolio_sha256` is mutually exclusive with
the legacy Go `target_run_container_sha256` and
`target_page_portfolio_sha256` pair. A bound `runtime_portfolio_sha256`
requires exactly one of the neutral or legacy complete page-portfolio
authorities; it cannot float independently. The neutral artifact is
strict-decoded and must contain the current manifest's exact ProgramTarget ID
and current child run ID. A multi-target standalone document is rederived from
every child manifest, report, ProgramIndex, and canonical order and compared byte-for-byte; a
self-consistent replacement seal is not authority. Merely containing an HTML
marker is not publication evidence.
The ordinary language-neutral child HTML may omit the Go-specific outer
`analysis_target`. Standalone preparation restores that authority only from
the manifest-verified canonical `report.json`, rejects any conflicting HTML
copy, and injects it only into the self-contained multi-target payload needed
to bind each selected target to the neutral page projection.

Target navigation is also atomic. The cross-page rail is keyed only by exact
`program_target.id` values restored from each validated page and the sealed
`ProgramPagePortfolio`; its backend-owned repository route enters
`#/repository` in the current document, while target routes enter `#/program`
in the current or an authorized sibling document. The page-local
`ProgramPortfolio` uses the same ProgramTarget identity, and the backend
requires its default to equal the current page binding. Every page must be
ready; an unavailable slot is a publication error, not a disabled link. Route
validation is pure from builder through report server and never mutates an old
`#/map` href into the product route.

A served run is viable only when its manifest-authorized source IDs and the
VS Code `code` CLI are available. Server startup fails explicitly when that
exact launcher cannot be resolved; a generic OS application launcher is not
accepted as evidence that VS Code exists. Each browser action waits for the
launcher result and returns success only after a zero exit status. Losing run,
source, or launcher authority returns a non-success response and a visible
browser error; it never becomes a clickable no-op or a copy-path fallback.
A single-page manifest with no page portfolio is served without inventing
sibling navigation; either neutral or legacy page authority remains mandatory
and fail-closed whenever its manifest digest is present.
Canonical `report.json` remains source-host-neutral in both modes. Static
GitHub/GitLab routing is bound by `standalone_source` in the manifest and is
embedded only into the verified HTML payload; served source IDs are issued
only by the verified local server session and are never persisted.

### Core-map evidence frontier

The progressive split deliberately borrows the useful shape of
[GitDiagram](https://github.com/ahmedkhaleel2004/gitdiagram): first make a
short repository-specific architecture brief from repository documentation,
then plan a bounded graph and validate its paths. Repomap does not adopt its
path-only authority or single-level graph limit. The complete path dictionary,
in its lossless prefix-compressed tree encoding, and README documents are sent
only to the README file-role classifier;
`CoreMap` receives its accepted sparse role rows. The second pass adds exact
target symbols and object facts and restores every selected ref locally;
future passes add only bounded code evidence around selected knots.

Ordinary no-cache runs established the current frontier:

- On `chi`, README plus names recovered the router core, radix tree, request
  context, middleware chain, and optional middleware package. Exact symbol
  facts then separated construction, route registration, matching/dispatch,
  tree storage, context, chaining, 404/405 behavior, grouping, and route
  inspection into nine blocks grounded by 27 representative symbols. The
  earlier unconstrained prompt selected 242 of 255 available symbols and let
  optional middleware dominate; caps alone caused needless model rejection,
  so compactness is prompted strongly while validation keeps a small tolerance.
- On `telebot`, the refined map recovered bot lifecycle, update retrieval and
  dispatch, handler/middleware registration, handler context, outbound
  send/edit operations, and media/file handling. It still describes several
  Telegram capability families separately and does not prove the dynamic
  handler registry, poller implementation, `Sendable` dispatch, or raw HTTP
  transport path.
- On a small HTTP service, the broad pass truthfully retained infrastructure
  and deployment context. Requiring exact target-symbol grounding in the
  refined pass removed that file-only material and recovered construction,
  execution, handler registration, observability, and HTTP utility blocks.
  The selected core-object projection retained 12 exact package-level
  callables and one receiver type. Supplying accepted activity context to the
  integration-operation cube reduced a permissive 34-operation result to four
  representative effects: server run, OTLP exporter construction, stdout
  metric exporter construction, and `http.Client.Do`. The binder attached all
  four to the expected core blocks. Two routes still share several core
  bindings because their handler transfer is dynamic and the direct-call graph
  honestly ends at their common registration/root path.

These results reject three shortcuts. README/tree alone is useful context but
not runtime authority. A flat symbol inventory is not an architecture map.
Types or centrality alone cannot define the core: optional middleware, DTOs,
configuration objects, and grab-bag files can dominate counts. Go now captures
interface invokes, function-value calls, and callback transfers during its
existing SSA walk and projects exact, alternative, or unresolved relations
into `ProgramIndex`; selective bounded logic is reserved for the remaining
unresolved execution joints. Go also builds a target-scoped exact `CoreObjectIndex` during the existing typed
program lifetime and joins receiver/signature evidence into `CoreMap`; Python
and future shell support must provide equivalent language-native object and
relation facts rather than imitate Go SSA.

Repository-guidance classification, target-portfolio, and both `CoreMap` model exchanges
are part of the per-run semantic journal, with the same redaction boundary as
other cubes. If a first-layer exchange fails before the ordinary artifact
writer exists, that failed run materializes only its already-buffered redacted
semantic-exchange journal. It publishes no metadata, semantic artifact, or
report and cannot become partial success.
Re-entering an outer failure path after a child page has already persisted the
same first-layer exchange is an idempotent no-op only when the complete exchange
identity and payload agree; a conflicting duplicate remains a bounded journal
failure rather than overwriting the committed diagnostic.
Every non-empty accepted rich repository-guidance catalog is rebound to the current
run-local corpus namespace and must be written exactly as
`readme-file-roles.json` before downstream cubes run, including every sibling
target page. A write, validation, or redaction mismatch ends the run. Exact
empty role input means exact artifact absence. Roles that cannot enter the
active language adapter remain visible in a non-empty authority instead of
disappearing. Portfolio omissions remain
auditable as the exact difference between the journaled candidate request and
positive response, and are reported locally as `Unclassified`.

The repository-guidance request is atomic: the compiler either includes every
textual README and AGENTS.md, every corpus file mapping, and the exact closed
prose-file ref set derived from those mappings, or makes no
provider call. It enforces a 1.5 MiB reliable atomic byte preflight
and never silently truncates, samples, or drops the tree. The bound is
empirical. On the tracked Airflow inventory, the old flat request was
1,964,003 bytes: 13,760 file mappings alone occupied 949,490 bytes alongside
174 complete READMEs and 6,429 now-removed lexical rows. The lossless object
tree reduced the dictionary to 431,710 bytes. The measured tree has
4,016 directories and maximum depth 17; all 13,760 FileRef/path mappings
round-trip exactly. Those Airflow measurements predate the closed
`prose_file_refs` array; its bytes are covered by the same preflight, but that
exact frontier must be remeasured before claiming Airflow still fits unchanged.
A repository whose complete tree and guidance bodies
still exceed the bound fails before transport with an explicit
resource error. Supporting that larger frontier requires a separately
approved semantic partition or chunked repository-index contract that
preserves the same exact FileRef authority; raising the byte limit is not the
solution.
The classifier and first `CoreMap` pass do not impose a fixed repository-size
quota on accepted role rows or on the exact file/symbol refs grounding one
block. Closed refs, the supplied repository dictionary, and the existing
request, response, and artifact byte envelopes remain the technical bounds;
an arbitrary item count must not turn otherwise valid evidence into a failed
run. The first CoreMap pass does
not share the complete-tree request frontier. When there are no accepted role
rows, it makes no provider request. It never independently rereads repository-guidance
documents or silently substitutes a partial repository dictionary.

### Deferred CoreMap output-budget contract

The Kraken benchmark exposed an unresolved CoreMap map-output budgeting
problem. One refined-map request carried 2,007 complete facts in 781,012 bytes;
the model emitted 122 blocks and reached the former 8,192-token ceiling after
23,822 response bytes, leaving incomplete JSON. The partial response remained
on-schema and repository-grounded until the physical cut: its 1,205 distinct
closed refs were all advertised. Its semantic granularity degraded instead.
After reasonable service and storage responsibilities, it began emitting
individual utilities, configuration types, fixtures, testing helpers, states,
and events despite the prompt explicitly preferring responsibilities over a
file inventory. A conservative lexical audit identifies 55 such names among
the 122 block starts.

A later request with the identical system prompt, the same temperature, and a
nearly identical catalog (74 fewer user-message bytes) was allowed the
temporary 128,000-token ceiling. It stopped normally after 14 blocks and
13,874 bytes, cited 1,534 distinct advertised refs, and was accepted. It did
not consume the available ceiling. This single comparison does not isolate a
cause or establish a threshold, but it shows that neither the large input nor
the raised output allowance by itself determines an enormous response.

Reducing the input byte bound does not control arbitrary model output.
Recursively splitting every length-limited response would also turn a per-call
token guard into a potentially large aggregate token budget, so neither
approach is accepted as the final contract.

Do not choose a new CoreMap output protocol without a separate experiment on a
repository at Linux-kernel scale. First test whether prompt and catalog-shape
changes preserve compact responsibility-level output as input grows. Measure
repeated samples at each scale; one stochastic completion is not a quality
threshold. Only if a reproducible quality-degradation frontier remains, or the
request actually exceeds the provider context, evaluate semantic partitioning
and reduction. Such batching must follow meaningful repository or graph
boundaries rather than an arbitrary byte size, preserve cross-partition
responsibilities and closed-ref authority, and account for aggregate cost,
cache identity, and warm-repeat behavior. Bounded closed assignments and
continuation remain hypotheses to compare, not accepted design decisions.

Until then, the temporary resource envelope is deliberately generous: CoreMap
requests allow up to 128,000 output tokens and 2 MiB of decoded response bytes,
and the default provider ceiling is 128,000 tokens. These are resource guards,
not semantic quotas or a claim that input size controls output size. A
completion that still ends at the provider length boundary remains a terminal,
explicitly journaled resource failure.

### Language-adapter handoff

The ordinary path has two language-neutral orchestration boundaries. The
repository owner discovers all active adapters once, performs one
TargetPortfolio decision, restores one typed target plan, and dispatches every
target to a complete page-local run. Inside each page, `internal/pipeline`
receives the sealed ProgramIndex, dependency catalog,
declaration artifact, accepted README roles, repository corpus, and one shared
provider/executor/journal context; it runs and validates
`ActivityEntrypoint -> IntegrationDependency -> IntegrationUsage ->
ActivityPath -> CoreMap`, persists each canonical artifact, and reports only
neutral progress and result values. Its `ActivityEntrypoint`,
`IntegrationDependency`, and `IntegrationUsage` model calls use distinct
journal stages rather than CubeMap aliases, and every model call emits a
payload-free accounting event with request bytes, live/cache state, transport
attempts, provider metrics, and a stage-local ordinal for ordinary metadata.
Python remains responsible for extracting and persisting its target-bound
declarations and ProgramIndex-derived dependency catalog before that handoff.
These existing handoffs are already reusable across languages:

- `RepositoryCorpus` file refs and first-layer `file_ref + hypotheses` target
  candidates;
- one refs-only repository target-portfolio selection followed by exact
  adapter-local resolvers into a canonical typed plan and the shared sealed
  `programindex.Target` boundary. The shared portfolio cube receives no Go,
  Python, JavaScript/TypeScript, or synthetic language catalog: exact
  `--target` choices remain adapter-owned corrective CLI guidance and are
  attached only after a portfolio error;
- accepted README file roles;
- the dependency catalog for package/import-oriented languages;
- model-facing core, activity-surface, integration, and
  surface/core/effect response schemas.

The sealed language-neutral `programindex.Index` handoff now contains four
kinds of data:

1. exact symbols and objects with repository locations and display signatures;
2. typed relations such as calls, contains, imports, implements, decorates,
   passes-as-callback, sources, executes, reads, writes, and invokes-external;
3. a closed evidence strength for every relation (`exact`, `alternatives`, or
   `unresolved`) plus bounded witnesses. A witness may carry a typed bounded
   source expression separately from human-oriented detail, so downstream
   cubes never parse presentation text to recover call authority; and
4. scenario, source identity, coverage, omissions, and stable local IDs that
   never need to be copied by a model.

Language adapters may retain richer private indexes. New downstream domain
cubes consume only bounded projections of this handoff and request-local refs.
Every selected Go, Python, and JavaScript/TypeScript typed target now feeds its
sealed page-local default ProgramIndex and dependency authority into the same
shared semantic cube chain. The Go adapter still also retains
`gocoreobject.Index`, `DirectCallNodeID`, SSA-flavored external-call fields,
and Go target scope for Go-specific exact projections; those additional
adapter facts are not the permanent cross-cube contract. A shell adapter
can model scripts and functions as objects and
`source`, direct command execution, subprocess, environment, file, and network
operations as typed relations. Dynamic `eval`, computed command names, and
runtime-generated files remain explicit alternatives or unresolved joints;
grep may advertise witnesses but cannot silently upgrade them to exact edges.
The current dependency catalog is package/import-shaped and cannot pretend an
external executable is a package; shell support must either generalize its
dependency-unit fields or derive dependency cards from exact `sources` and
`executes` relations through a compatible domain projection.
Adding such an adapter does not by itself make shell a supported product path:
target selection, graph restoration, dependency extraction, validation, and a
real ordinary online run must all consume its output first.

Language adapters are capability based. The Go adapter is backed by the
repository's real build selection, package loading, gopls/SSA, and syntax
facts, and adapts those already-built facts without repeating analysis. Its
dynamic-handoff overlay accepts only concrete SSA value flow: it never turns
interface compatibility or matching signatures into runtime callees. A
structural handoff that cannot enter the closed projection increments measured
handoff omission coverage; a partial candidate flow enters only as unresolved
and counts every known discarded candidate in relation-level omission. An
exact repository-local representative source is retained for every admitted
package while its typed AST is alive; library packages remain projectable even
when their public API contains only exported constants or variables. An
interface without a working adapter is not evidence that another language is
supported.

The Python adapter is an active ordinary-path slice of this approved
migration, not historical orphan code. Its target scout deterministically
inspects PEP 621, Poetry, setup.cfg, literal-only setup.py, Hatch package lists,
requirements/Pipfile signals, `src`/flat layouts, package `__main__.py`, main
guards, executable scripts, and decorators without importing repository
modules or executing build backends. It binds modules, project scopes, roots,
basis facts, anchor files, and source files to the shared `RepositoryCorpus`,
then projects its public first-layer answer to only `file_ref + hypotheses`.
Native discovery advertises only packaging, declared scripts/modules, main
guards, and executable-script authority; it does not emit one hypothesis per
module. The exact Python catalog separately seals one cryptographic module
scope per project. When README or another cube selects a closed module file
ref that has no more precise native root, the resolver derives one
catalog-owned `module_execution` target from that scope. Its exact selector is
`python:module-execution:<scope-digest>:<file-ref>`; paths, module names, object
constructors, annotations, dependency names, and framework allowlists are not
target authority. Explicit native roots keep precedence, so a main guard,
declared callable, or executable script is never replaced by the generic
view. Catalog snapshotting and canonical encode/decode retain the scope digest
and reproduce the byte-identical derived target. Typed discovery omissions do
not erase independently sealed targets: the ordinary selector may continue
with exact native or resolver-owned targets while persisting and reporting the
complete partial catalog. An omitted dynamic setup declaration, unrelated
syntax error, or unsupported launcher is never converted into a target and
never hidden as complete coverage. If no exact or resolver-owned target can be
selected, the run still fails; after selection, incomplete ProgramIndex,
dependency, or cube authority remains terminal. This is omission-preserving
selection, not a semantic fallback. Its isolated `python3 -I -S` AST
worker reads source only through stdin, imports and executes no repository
code, and
   projects modules, declarations, local and external imports, calls,
   containment, decorators, callback handoffs, source locations, and explicit
   unresolved joints into `programindex.Index`. A locally resolved mutable
   call, decorator, base, or callback binding is retained by exact ObjectID as
   an `alternatives` relation even when it is the only observed candidate;
   this preserves graph evidence without claiming immutable Python dispatch.
   Source contents for an identical
module inventory are read once and parsed into AST once; target-local package
and alias semantics are projected as independent views inside that same
isolated parser batch. Thus executable and library views neither contaminate
each other nor force a second source parse. Every module, basis, source, and
anchor ref/path pair is checked in both directions against the current
`RepositoryCorpus` before it can enter the sealed target. Project-relative
layout aliases
such as `src.config` resolve back to the same local source object, while
decorator witnesses retain only structural callee names and never persist
literal route, command, token, header, or default-value arguments. Function,
lambda, and class defaults, annotations, bases, and metaclass expressions are
visited in their defining scope so their calls do not disappear. Identical
module inventories share one parse but remain separately selected target
views, each with its own Target-ID-bound ProgramIndex artifact. The current
Python semantic product path preserves dynamic-language uncertainty rather
than becoming a structural-only fallback. Each selected Python target's page derives and
persists a strict language-neutral dependency catalog, runs the shared
potential-integration dependency cube, classifies concrete integration
operations from exact callsites whose possible external target ObjectID is
retained by the ProgramIndex, and runs both
CoreMap passes from the exact ProgramIndex. The refined pass consumes every
exact target launch seed and the complete selected integration-operation
artifact rather than recomputing either fact or leaving integration output for
presentation alone; incomplete ProgramIndex,
direct-import, dependency-classifier, or integration-operation coverage ends
the run. Its ordinary report consumes the truthful language-neutral
ProgramView, a separately validated human-named CoreMap, and only the
model-selected integration operations with their exact callsites and
explicitly non-exact runtime authority.
The report rechecks every selected CoreMap member, activity start, integration
operation, and activity-to-caller path against the same sealed ProgramIndex.
Manifest version 34 and report format version 65 material-bind the exact Python
target catalog, declared-dependency artifact, CoreMap, activity-entrypoint
selection, dependency catalog, selected integration dependencies, concrete
integration-usage artifact, and deterministic ActivityPath artifact as one
Python semantic authority; the result is not relabeled as a CubeMap. The
Python page renders the exact selected activity starts and integration
operations as separate authorities, enriches each operation with the shared
route to its caller without claiming runtime reachability, and renders selected
declaration candidates as a visibly separate declaration-to-code frontier.

The JavaScript/TypeScript adapter is an active multi-package ordinary path.
Package-source ownership goes to the deepest containing tracked `package.json`.
Every manifest with owned JavaScript/TypeScript source contributes one required
native target and receives its own complete page-local execution; source-less
manifests remain tooling and do not suppress other packages. TargetPortfolio
retains all exact package targets and chooses only the repository default. An
explicit `jsts:<manifest>` narrows the typed plan to one owned package before
compiler execution. `package.json#name` is optional. The exact
top-level npm lockfile name is the secondary package name/path; without either
declaration, the root uses `root-package` and a nested package uses its full
repository-relative project directory, never the absolute checkout basename.
Only raw `package.json#name` may derive the command for npm's string-form
`package.json#bin`; a display/identity fallback grants no CLI authority. The
adapter participates in the same repository-wide TargetPortfolio and typed
execution plan as Go and Python. The owner prepares dependencies with the
repository's normal package manager; Repomap requires a repository-local
TypeScript compiler for the Compiler API but never runs an install. The
versioned helper receives only the
closed corpus path/ref catalog and configuration identity, runs with an empty
environment, and emits deterministic JSON with repository-relative locations.
Helper contract version 5 derives compiler candidates only from `typescript`
or exact npm aliases to it declared by the selected package manifest. A nested
selected package consults the repository-root manifest only when it declares no
candidate itself, and each candidate resolves from the manifest scope that
declared it. The installed candidate must report `package.json#name` as
`typescript`; duplicate resolutions are one candidate, one compatible legacy
Compiler API candidate is preferred over native-preview candidates, and
multiple distinct candidates in the preferred tier fail closed with their
declared names. This recognizes deliberate owner-prepared aliases such as
`typescript-api` without scanning arbitrary installed or transitive packages.
Its caller bounds helper output and reduces stderr failures to a closed
diagnostic before an error can reach journals; environment values and absolute
host paths are never emitted. It honors
`tsconfig.json`/`jsconfig.json` include/exclude, bounded recursively expanded
in-package solution-style project references, `baseUrl`, path aliases, and
module resolution. An existing repository-local reference outside the selected
package is a cross-target boundary rather than an unresolved config or an
instruction to absorb sibling sources. Each traversed in-package config is
compiled with its own parsed options;
overlapping tracked source has one deterministic canonical owning config rather
than incompatible options being flattened together. JavaScript facts use the same syntax and resolver path but
checker-derived call targets are retained only as alternatives rather than
TypeScript-exact authority. Helper contract version 5 retains version 4's
removal of project-wide property-name matching: only compiler/type-resolved
declarations or exact external imports establish a call target, while an
unresolved property call is retained as a targetless unresolved frontier.
Compiler-default-library calls and constructions use one version- and
installation-independent `platform:javascript` external-symbol authority; it
is not a package dependency and never enters integration selection. Calls and
`new` expressions retain the
closed invocation `call` or `construct`; an explicit local constructor is exact,
while a synthetic or otherwise unbound construction remains unresolved rather
than targeting a class by name. Version 4 of the sealed project artifact owns safe
package, lockfile, script-kind, tool-config, source-root and entry facts plus
canonical `package.json#bin` command/path pairs bound to exact tracked,
selected-package-owned FileRefs; raw dependency locators and script commands
are not persisted. Before the first ProgramIndex projection and sealing, the
adapter locally omits only optional signature and source-expression display
text matched by the always-on persistence guard, including the matching
product-path label. Call identity, location, resolution, target authority, and
witness accounting remain exact; sensitive required identity or semantic
fields still fail closed. Encode and decode reapply the same guard to the
complete artifact bytes.

The adapter projects modules, qualified declarations, imports, exports,
re-exports, direct calls, unresolved joints, and external symbols into the
shared ProgramIndex and language-neutral dependency pipeline. A class method
becomes a ProgramIndex method only when the compiler projection retains its
exact owning type; a callable property in an object literal remains a nested
function and never receives an invented type receiver. An exact
browser/server surface or an exact retained package binary makes that
page-local ProgramTarget an `application`; a library or tool-only selected
package remains a `library` and is never promoted by a script. Each retained binary pair owns
one command-line product surface at the binary file with empty entry refs; it
does not claim that a separate development source implements the bin wrapper.
Only inside an already proven CLI package, an exact canonical `dev` or `start`
script with exactly one helper-selected source ref may independently add a
`SeedScript` launch root. Separate deterministic surface facts classify browser
application, Node server, and command-line application as product surfaces,
shared contracts as supporting code, and selected build/migration entry scripts
and integration/test servers as tools.
ProgramIndex version 7 keeps two independent byte authorities: at most 64 MiB
of aggregate semantic strings and at most 128 MiB for the complete canonical
JSON envelope. The larger envelope accounts only for repeated field names,
punctuation, numeric metadata, and escaping; it does not expand semantic text
or permit truncation.
Framework parsing is limited to exact import/type-backed structural evidence
for Express/Router registration, Wouter routes and links, React root, Node
listen, TanStack/fetch/API-request sites, Zod/Drizzle contracts, environment
declarations, cron registration, and external resource boundaries; local
lookalikes and framework presence never select a product surface or semantic
responsibility.

A small version-1 cross-surface artifact joins every statically compatible
client HTTP use and server route with an explicit `http_method_path_match`
relation rather than a call edge. Its ordered steps retain the closed authority
`exact_static`, `resolved_indirect`, `possible`, or `unresolved_frontier` and
cite only exact producer refs and source locations. It may connect a browser
route/component, mutation, API wrapper, server registration, middleware,
handler factory/returned handler, shared contract, storage call, and resource
boundary; absent relations stay visible as frontiers and are never supplied by
the model. Server registrations must be reachable through retained program
calls from a product Node surface, so an integration-test server with the same
method/path cannot replace the production handler. Dependency-injected storage
dispatch remains possible rather than exact. Report format 65 derives the
surface catalog and cross-surface view
from the sealed project plus the exact ProgramIndex, starts JS/TS reports at
`#/repository` only when that view has an exact surface or path, otherwise
starts at the semantic program map, and owns fail-closed surface/path deep links. Manifest version
34 material-binds and rederives both views before publication.

The dependency handoff is a versioned language-neutral catalog. Each direct
dependency has a deterministic local ID, the closed kind `workspace`,
`stdlib`, or `external`, exact available module/package/name metadata, safe
repository-relative path metadata for workspace packages, and stable refs to
its exact importing repository packages. IDs and importer refs remain local;
later model cubes must advertise request-local short refs instead.

An unresolved variable-driven `importlib.import_module(...)` relation is a
ProgramIndex dynamic-handoff frontier, not a direct dependency with the fake
package name `importlib.import_module`. The Python dependency adapter excludes
only that exactly witnessed unresolved mechanism from package-coverage
omissions; the ProgramIndex relation remains available to activity, core, and
future mechanism cubes. Exact literal dynamic imports still resolve to their
local module when possible, and every other unresolved or ambiguous direct
import remains an explicit partial-coverage omission.

The potential-integration dependency cube receives every `stdlib` and
`external` row, including ordinary-looking packages; it does not use a local
allowlist of databases, brokers, network stacks, cloud SDKs, TLS packages, or
other supposedly important dependencies. The dependency kind, exact package
identity, and importer locations are evidence for the model. `workspace` rows
remain internal graph facts and are not external-boundary candidates. Every
request creates new short refs; refs from README, target, or earlier semantic
requests never cross this cube boundary. A complete catalog may be empty and
then produces an authoritative empty result without a model call. Candidate
rows are never omitted inside the global artifact bound; exceeding that bound
is a pre-provider error. Per-row importer context is capped with an explicit
count, while every selected dependency is restored with its complete exact
importer set locally.

Declared package-manager dependencies are a separate live authority rather
than synthetic rows in the observed-import catalog. The Python ordinary path
always builds and persists target-scoped `declared-dependencies.json`, bound to
the exact corpus, selected Python target catalog, and page-local ProgramIndex. It
parses PEP 621 and Poetry dependency tables, string-valued dependency groups,
build-system requirements, and in-scope `requirements*.txt`/`requirements/`
files with corpus-confined requirement and constraint includes. It preserves
exact source refs and content hashes, package-manager names, normalized
distribution identities, roles, groups, conditional-marker presence,
constraints, safe locator kinds, includes, and a complete positive/frontier
ledger. `setup.cfg`,
`setup.py`, and `Pipfile` are currently read and recorded as explicit source
frontiers rather than parsed into package rows. Dynamic or unsupported
pyproject shapes, unsupported requirement statements/options, and unresolved
includes likewise remain explicit boundaries; they do not disappear or become
guessed dependencies. Malformed TOML, non-text requirement input, unreadable
or truncated inputs, unresolved target scope, and bounds overflow are terminal
rather than frontier or partial-success results.

The declaration-aware `IntegrationDependency` request sends those exact
package rows as `p*` candidates beside, but never merged with, observed `d*`
imports. It may select a non-constraint declaration as a high-recall
integration candidate, but the result restores `d*` and `p*` selections into
separate artifact sections and coverage ledgers. A constraint-only row is not
selectable. Normalized distribution identity never proves an import-name
mapping (`PyYAML` is not locally rewritten to `yaml`), and declared-but-unused
packages never acquire importers or callsites. `IntegrationUsage` continues to
require exact ProgramIndex callsite candidates and does not consume `p*` rows.

The next missing handoff is a `DeclaredPackageResolution` cube. Its job is to
connect selected declaration candidates to exact language-native code evidence
or retain an explicit unresolved frontier; it must not guess a distribution to
import-name mapping. Until that cube exists, selected declaration candidates
stop at the report's declaration-to-code frontier and never enter CoreMap,
IntegrationUsage, or ActivityPath as observed code use.

### Deferred integration drill-down

An accepted integration should later be explorable in two clicks from its
card: first to the exact client/adapter operations and callsites, then to the
repository types that cross the boundary. The same drill-down should state the
observed or best-supported interaction mechanism, such as HTTP, gRPC, TCP,
WebSocket, a broker protocol, or `unknown`, without deriving it from a package
allowlist.

The intended data path is:

1. The existing exact external-call index supplies caller, dependency package,
   receiver, operation, invocation mode, witness count, and callsites.
2. A future language-specific `BoundaryContractFacts` cube runs during the
   same program-facts lifetime and attaches positional receiver, argument, and
   result type refs; generic value-origin and data-flow refs; exact neighboring
   call identities; bounded repository-local type shapes; and explicit
   coverage frontiers. It extracts type-system and data-flow facts only; it
   does not decide which value is a request, response, client, constructor,
   endpoint, configuration value, or serialized payload, and it does not name
   a service or choose a protocol. It must retain exact `interface_invoke` witnesses as
   interface type + method + declared argument/result types + caller/callsite.
   A concrete implementation may be attached only when the receiver's closed
   SSA value flow establishes it; otherwise it remains unresolved. This is essential for
   generated gRPC/Connect-style clients and is not permission to guess a
   dynamic callee. Where an external signature exposes only an interface such
   as `proto.Message` or `io.Reader`, a bounded backward projection through
   exact repository callsites may advertise concrete local value-type
   alternatives with an explicit ambiguity frontier.
3. A future `IntegrationProfile` model cube receives only selected integration
   uses and their bounded contract facts. It groups operations that belong to
   one logical external service/resource; classifies semantic roles such as
   request, response, client/constructor, endpoint/config, and serialization;
   chooses a closed interaction kind or `unknown`; and selects the exact
   operation/type/value-origin evidence refs supporting that result. Several plausible mechanisms remain explicit
   alternatives rather than being collapsed by a local rule.
4. A later presentation projection renders integration -> operations -> types
   navigation. It may link to another repository only when an exact configured
   or discovered hosted-repository authority exists; dependency naming alone
   cannot invent a source destination.

This drill-down is recorded product direction, not part of the current HTML
contract. The current implementation should preserve the exact facts needed by
it without delaying the first activity/integration cube-map prototype.

The Go adapter extends the existing build-targeted package load to one
`go list -deps -e -json ./...` call per discovered module. `DepOnly` objects
provide dependency metadata but never enter workspace package counts, target
selection, entrypoints, or internal-edge roots. `Standard` is the only stdlib
authority, and non-standard external dependencies require exact `Module.Path`
authority, including safe replacement metadata. The adapter does not classify
dependencies from dots or other import-path spelling and does not execute a
second dependency command. Imports and importer refs are deduplicated and
canonically ordered; target scoping removes importers outside the exact
retained package scope without changing dependency IDs. The adapter catalog
records closed `complete`/`partial` coverage: every observed direct import is
either a retained typed dependency use or a deterministic omission tied to its
importer and import path with a closed reason. The ordinary cube pipeline
rejects `partial`; the state exists to explain the failure, not as a degraded
publication mode. A missing or broken package object must never look like
evidence that the dependency is absent.

Every semantic cube and the current report run in canonical English. A future
presentation-localization cube may translate a final allowlisted projection
and resolve fixed UI labels after semantic analysis. That deferred cube must
not change target, graph, dependency, integration, or other semantic
identities; this refactor does not redesign the existing HTML renderer for it.

The provider never receives the whole repository source, raw internal edges,
canonical local IDs, credentials, or paths outside the advertised request.
Repository-wide path visibility is limited to the repository-guidance file classifier: it
sends a lossless prefix-compressed tree of the complete names-only tracked-file
dictionary addressed by run-local `f*` refs plus the complete tracked textual
README and AGENTS.md bodies, never the corresponding other source bodies. The
first `CoreMap` pass receives only accepted sparse repository-guidance role
rows. The refined pass receives exact target symbol rows plus available
receiver/signature facts and the accepted baseline. The binder receives only
selected object rows and aggregate reachability categories. Source bodies and
raw graph edges remain local throughout.

The repository is trusted input by default, so heuristic credential detection
is off. `--scan-secrets` enables the cautious scan when the caller wants it.
This does not relax the unconditional rule that API keys and Authorization
headers are never written to artifacts.

Repository contents may change while analysis is running. The ordinary path
does not recapture the repository, compare a later state, emit a freshness
classification, or gate publication on drift. The manifest retains the one
captured repository identity/revision plus hashes of material inputs for source
authorization; there is no `freshness` field or strict freshness mode.

Invalid invocation and missing prerequisites are checked before analysis when
possible. A fast nonzero exit that identifies the bad flag or gives the exact
corrective flag is a usage failure, not a failed analysis. Once provider-backed
analysis begins, repository drift is not grounds for withholding its report.

## LLM provider boundary and cache

Domain cubes expose a high-level operation and own only their domain state,
input preparation, and semantic result validation. A cube must not implement
HTTP, provider envelopes, retries, cache files, token accounting, or semantic
journal persistence.

Static system/user prompt instructions live as readable Markdown files beside
their owning cube and are compiled into the binary with `go:embed`. Go code
may assemble dynamic bounded catalogs and exact JSON request values, but it
must not hide long prompt prose in string literals. Prompt files and response
schemas are versioned cube state; the provider layer does not own them.

The shared LLM executor owns:

- provider configuration and a non-secret stable provider state;
- construction of the exact immutable provider request;
- HTTP transport, bounded retries, response-envelope decoding, and accounting;
- stable ordered execution of cube-planned batches;
- JSON decoding into the response type requested by the cube;
- accepted-only persistent caching; and
- per-run observation events used by the semantic journal.

Domain identities are never delegated to model spelling. Before a cube call,
the backend maps UUIDs, canonical symbol identities, dependency identities,
and repository-relative paths to a closed request-local catalog of compact
refs such as `t1`, `s3`, or `d7`. Each catalog row may expose the exact bounded
context that gives the model useful meaning: the repository-relative path,
file name, symbol name or signature, dependency name, and a small local fact
projection. The response schema accepts only the short refs, and the cube
restores full identities locally; the model is never asked to reproduce a
path, symbol, UUID, or canonical ID exactly. In explicitly set-valued closed-ref
fields, unknown elements are discarded before deduplication and limits while
advertised refs are retained; they never trigger retry or clarification. An
assignment row keyed by an unknown ref is discarded for the same reason: it
has no object to assign. Unknown scalar refs and missing mandatory refs remain
errors when filtering leaves the required choice unresolved. Neither the
executor nor a cube guesses, fuzzy-matches, or promotes a replacement. Repeated
selection refs are canonicalized locally, so a model does not have to prove
uniqueness by echo discipline. Exhaustive
batching, coverage, identity, and set reduction remain local responsibilities.
Assignment-valued conflicts are different: when one ref is assigned two
genuinely incompatible owners or meanings, the cube retains the ambiguity as
an error; it never silently chooses the first row. Group membership is not
presumed incompatible: model-owned orientation groups may overlap because
arbitrary repositories need not admit one universal partition. They are not
required to exhaustively echo the catalog. If a non-empty grouping leaves
known responsibilities unplaced, one provenance-marked local ledger records
the exact complement without asserting an orientation membership; together
the model groups and local ledger account for the complete refined set. The
shared ref-catalog primitive may be reused by cubes, but ref meaning remains
domain-owned rather than part of provider transport. Absolute host paths are
not semantic repository evidence and never enter provider requests.
The shared JSON intake may unwrap one unambiguous complete object/array from a
Markdown fence or harmless leading prose. That is syntax normalization only:
it never repairs malformed JSON, missing or unknown fields, unresolved
mandatory scalar refs, schemas, or values. A domain reducer may discard
unknown set elements and unknown-keyed assignment rows only as described
above. Ambiguous, truncated, or otherwise semantically invalid output
is a terminal cube error, and orchestration, report projection, and browser code
have no semantic fallback path for it.
For the README file classifier, the closed catalog is the complete names-only
corpus dictionary in lossless prefix-compressed tree form plus its complete
closed prose-file ref set, rather than a locally selected candidate subset. The model
owns README-claim interpretation, role classification, and matching those
claims to advertised FileRefs; the backend owns exact ref authority, closed
classes, incompatible prose-role filtering without promotion, canonicalization, and the
projection of only `target_entry` into target discovery.

Only the cube knows safe semantic batch boundaries, so it prepares a batch
plan; the executor performs that plan. The cube's current validator is applied
to cache hits and live responses before either can become an accepted result.
Provider configuration, cube `State`, and the exact prepared input form the
cache identity. `State` includes prompt/preparation and response-validation
contract versions, not mutable runtime counters. Cache keys use SHA-256.

Ordinary runs reuse validated persistent model responses. Cache identity must
include the effective provider/request contract and the bounded evidence that
can change the result. A cache hit is decoded and validated exactly like a live
response. A domain's publication-only run IDs, output paths, timestamps, and
artifact seals must not enter semantic identity when the same raw response can
be re-resolved and strictly rebound to the current publication. The domain must
retain a separate local compilation seal over its full current authority so
that excluding publication metadata from executor state never weakens artifact
or manifest binding.

`--no-cache` bypasses persistent cache reads and writes for a live-provider
check. It does not disable the per-run semantic journal. A bad cache hit is
removed and followed by at most one live cube call; failed live calls are never
cached. `repomap cache clear` removes only repomap's known persistent cache
directories under the selected debug root; it does not remove saved report
runs.

## Change and acceptance rule

Code comes first. Before retaining a feature or documenting a contract, prove
that the ordinary built `repomap` binary exercises it. Dead branches,
historical schema adapters, experiment fixtures, corpus/replay machinery, and
helper commands should be deleted when they are not required by this path or by
its focused tests.

Focused discovery and ProgramIndex regressions keep exactly one cumulative
real repository fixture for each active language under
`testdata/repositories/<language>`. A new regression extends that repository
with the smallest scenario file and updates its exact tracked-file inventory;
it does not create a per-bug repository. Deterministic discovery and language
adapter stages execute for real. Model-backed batching, closed-ref
normalization, cache validation, and grouping stay in smaller cube or executor
tests unless the invariant genuinely depends on repository shape. If an
approved repository regression must cross a provider boundary, it uses an
exact request-bound, fail-closed local preset with no network access. Fixture
success is focused test evidence only and never replaces ordinary online
acceptance.
Creating or extending one of these repositories is not automatic. Before any
such change, including a possible mixed-language fixture, ask the owner for
approval, identify the exact repository-dependent invariant the fixture would
prove, and establish that an ordinary unit or contract test is insufficient.

Three exact, source-bound development inventories live under
`testdata/contracts`. The prompt inventory covers every Markdown `go:embed`
under `cmd` and `internal`, classifies it as active or dormant, and binds its
owner, stage, role, source, pattern, and variable. The production-limit
inventory classifies every named hard-bound symbol and rejects material
anonymous `LimitReader`, `MaxBytesReader`, `WithTimeout`, timeout-field, and
JavaScript prefix-slice bounds until they are named; explicit name-pattern
false positives carry a reason. The test inventory covers every Go test file
under the product roots `cmd` and `internal` by directory and package, plus
every repository-owned JavaScript/TypeScript test script outside excluded
artifact, fixture, generated, and dependency trees. It gives ordinary tests a
no-network/no-real-provider default and records all exceptional runtime,
process, platform, environment, network, checkout, and cumulative-fixture
requirements. Their contract tests fail on unclassified
additions, removals, or changed source bindings; the inventories do not create
a second runner or analysis path.

That cleanup rule does not authorize deleting a newly written adapter slice
that belongs to the currently approved pipeline merely because its next
consumer is still being built. Active work must have an explicit next handoff
and focused executable checks; it is deleted only with owner approval or after
an equivalent replacement is connected and verified. An orphan audit may flag
such a slice, but may not decide its product status.

A change is accepted only after:

1. `make build` produces `.bin/repomap`;
2. the binary completes a normal online run against a real repository;
3. exit status, manifest, the sealed ProgramIndex set and every referenced
   index, downstream cube artifacts, report JSON, and report HTML are checked
   directly;
4. focused tests and vet for changed packages pass.

Canonical `make test` and `make vet` retain the ambient system `GOCACHE` and
`GOMODCACHE`, cap Go package fan-out at four, and apply a five-minute timeout
to each test binary. Focused developer commands use the same or tighter
bounds; they do not create private replacement module/build caches.

Offline or replay success is never a substitute for this check.

For bounded inspection of the ordinary online path, `STOP_AFTER=ActivityEntrypoints`
ends a successful run immediately after the shared pipeline has validated and
persisted `activity-entrypoints.json`. No later cube, report generation, or
server startup runs. Any other non-empty `STOP_AFTER` value is rejected before
repository analysis; this checkpoint is not a second command or an alternate
analysis implementation.
