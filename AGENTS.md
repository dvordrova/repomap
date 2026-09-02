# AGENTS.md

## Product

`repomap` is an online, model-assisted repository orientation tool. Its
supported user-facing surface is deliberately small:

- `repomap [repository] [flags]` runs the ordinary analysis and publishes the
  report artifacts.
- `repomap cache clear [--debug-dir DIR]` clears persistent model-response
  caches.

There is no supported offline, investigate, doctor, dev, replay, experiment, or
`serve` subcommand. Report serving remains part of the ordinary run and is
controlled by `--no-serve` and `--port`. Do not add script entrypoints or
sidecar tools.

## Authority

- Code on the ordinary `repomap` main path is the source of truth. Verify what
  the built binary actually does before documenting or extending it.
- [docs/CONSTITUTION.md](docs/CONSTITUTION.md) is the product constitution: what
  the product is, who it is for, and the invariants every change preserves.
  Where it conflicts with this file or the ADR, the constitution wins and the
  other file is corrected.
- [docs/agent-room/CURRENT.md](docs/agent-room/CURRENT.md) is the single living
  ADR. Change the relevant section in place when the product changes.
- Historical decisions and deleted planning documents remain available in Git
  at the pre-cleanup commit `4e54ab3`; they are not current requirements.
- [docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md) owns only the live
  online transport contract. Prompt and response schemas live with their code.
- The owner has approved the single ProgramIndex-to-GroupsIndex graph pipeline
  recorded in CURRENT.md and a shared LLM-provider executor. Do not add another
  analysis graph, semantic authority, or presentation layer outside that
  pipeline without fresh approval.

## Product contract

- Extract deterministic repository facts locally, then send bounded,
  request-local evidence to the configured model provider. The initial
  repository-guidance file-role classifier may send the complete names-only
  safe-corpus dictionary as a lossless prefix-compressed path tree with
  compact `f*` leaves, the complete closed set of prose-file refs derived from
  that same dictionary, plus complete textual README and AGENTS.md documents;
  it sends no other source-file contents. Before any `f*` identity exists, exclude
  `.npmrc`, every `.env*`, dependency/generated subtrees `node_modules`,
  `dist`, `build`, and `coverage`, and every `*.tsbuildinfo` file from the
  shared corpus, freshness state, model input, debug output, and publication.
- Model-assisted stages own `State`, complete input authority and its provider-sized
  request preparation, and semantic
  validation. The shared LLM layer owns exact provider requests, transport,
  retries, provider-envelope/JSON decoding, batch execution, cache, accounting,
  and semantic-journal events.
- Repository scale is not a local correctness boundary. Former local
  count/text/byte/depth and artifact/report thresholds are warning-only: they
  must never sample, truncate, omit, reject, or partially publish valid
  repository-derived authority. Complete reservoirs are processed through as
  many deterministic disjoint provider batches and convergent closed-ref
  reduction rounds as necessary. Every current stage uses the shared actual
  32 MiB request envelope and 16 MiB decoded-response ceiling and requests up
  to 128,000 output tokens; a lower configured provider token ceiling remains
  authoritative. Composite input is exhaustively repartitioned when its
  prepared request does not fit. A real
  provider response/output envelope failure remains terminal unless the owning
  stage defines a lossless adaptive repartition; it never authorizes truncation
  or partial publication. Only such an actual provider envelope,
  representation overflow, security policy, canonical identity/path/format
  validation, or an explicit user narrowing option may remain terminal.
- Execute independent stage-planned batch items through the shared bounded LLM
  worker pool, with the ordinary product limit set to four. Preserve the
  caller's item index as the only in-memory result slot and replay observer
  events in that order; do not add a random batch identity to semantic or cache
  state. Every provider transport attempt acquires the run-shared adaptive
  gate. An HTTP 429 collapses that gate to one before the provider's existing
  backoff/retry, so already-started attempts finish while that retry and every
  new attempt become serial. Only a terminal item error cancels the batch child
  context and prevents queued items from starting. The owning stage still
  rejects the complete batch on any terminal item failure; accepted sibling
  calls may retain their exact identity-bound cache entries but never become a partial
  semantic result.
- A model-assisted stage returns a fully validated result, a contractually
  legitimate empty result, or an error. Backend orchestration, report
  projection, and browser code must never supply semantic fallback, repair, promotion, or partial
  success for failed or incomplete stage output.
- Keep static prompt prose in readable Markdown beside its owning stage and
  compile it with `go:embed`. Go owns complete dynamic reservoirs and their
  provider-sized request partitions, not long prompt string literals; the
  provider layer does not own domain prompts or schemas.
- Models select only closed request-local short refs. Catalog rows may show
  exact repository-relative paths, file names, symbol names/signatures, and
  dependency names because that context has semantic value. The model is
  never required to copy those values: UUIDs, canonical identities, ref
  resolution, and graph restoration remain local. Unknown refs have no
  authority and are discarded rather than guessed, repaired, or clarified;
  absolute host paths are never sent.
- Set-valued closed-ref responses are filtered to advertised refs and
  deduplicated locally; never require a model to echo each selected ref exactly
  once. Assignment rows keyed by an unknown ref are likewise discarded. A
  known prose ref paired with a valid non-documentation class is an unsupported
  set member and is discarded before its hypotheses or per-file bounds gain
  authority; it is never repaired or promoted into `documentation`. Go
  owns exhaustive batching, completeness, and identity. If filtering leaves a
  mandatory scalar choice or complete assignment unresolved, reject that
  incomplete result without inventing a replacement. Program categorization
  and initial grouping are sparse positive selections: omission is not a
  negative assignment. Categories and group membership are overlapping covers,
  not partitions, and no local `support`, `unassigned`, or other semantic
  complement is manufactured. A grouping merge is consolidation, not a second
  sparse selection: every validated candidate group's complete member set must
  be contained by one returned group in the same lane. One returned group may
  cover many candidates, so the model never has to acknowledge every `g*`.
  Every accepted group member must itself carry a category compatible with
  that lane: `inbound` or `background_activity` for `triggers`, `core` for
  `core`, and `dependency` for `dependencies`. One compatible member never
  promotes an incompatible peer into its lane. Incompatible member refs are
  discarded as unsupported set members while valid siblings survive. Merge
  preserves every already validated same-lane membership and may duplicate a
  member into another group only when that member's own categories support the
  other lane.
  Missing candidate membership rejects the complete merge; Go never inserts or
  promotes the omitted member. Group proposals left with no advertised known
  member are discarded. Only genuinely incompatible assignments for one known
  ref remain an explicit ambiguity; do not apply first-wins repair.
- Provider requests must never contain full repository source contents, raw
  internal edges, canonical internal IDs, credentials, or unadvertised paths.
  A complete names-only tracked-file dictionary is explicitly allowed for the
  README file-role classifier.
- The selected repository is trusted by default. Heuristic credential scanning
  is opt-in through `--scan-secrets`; API keys and Authorization headers must
  still never be persisted.
- Analyze every eligible target by default. `--target` selects an explicit
  target; `--force-platform GOOS/GOARCH` overrides the normal Go platform
  selection.
- Run every active Go and Python target scout plus the JavaScript/TypeScript
  package-target catalog scout over the same repository corpus. Merge their
  exact file candidates and resolvable repository-guidance candidates into one
  repository-wide `TargetPortfolio` request; the presence of one supported
  language must never suppress another. Bind one canonical required file
  representative for every exact native target, deduplicating a shared
  representative and never requiring every alternative file for the same
  target. The portfolio must retain every
  required representative, chooses one retained file as the repository default,
  and may additionally retain positively supported guidance candidates. Restore
  every positive file ref through exactly one language adapter into one typed
  target plan. An exact `--target` bypasses the model portfolio but must still
  resolve unambiguously through that same typed adapter boundary. Target scouts
  may not execute an adapter's page-local ProgramIndex, dependency, or semantic
  path. A compiler projection used to build that page, including the JSTS
  TypeScript Compiler API projection, likewise belongs only to selected typed
  targets, so an unselected language's page prerequisite cannot block an exact
  target owned by another adapter.
- The Go fact and target inventory excludes non-`DepOnly` `go list` root rows
  that have no build-selected `GoFiles` or `CgoFiles`. In particular, a
  directory containing only external `*_test.go` files is not an ordinary
  package when the product loads with `Tests=false`; its raw row may inform
  dependency metadata but must not enter package counts, target identity, or
  the typed ProgramIndex scope. A source-bearing package that fails type
  checking is not filtered and still fails its owning target closed.
- Execute every selected typed target through its own complete page-local path:
  sealed base `ProgramIndex`, target-scoped dependency authority, exact reduced
  documentation, sparse overlapping categorization on that same ProgramIndex,
  one target-local `GroupsIndex`, and a validated report page. A selected
  non-default target is not a structural substitute for that path. Multi-target
  publication seals a language-neutral `ProgramPagePortfolio` keyed by exact
  `ProgramTarget` IDs and child run IDs plus one exhaustive
  `TargetOutcomePortfolio`, then matches the complete GroupsIndex set across
  targets. Preserve every analyzed child run's manifest-bound `report.json`,
  enriched ProgramIndex, reduced documentation, and GroupsIndex, but publish
  exactly one physical `report.html` in the deterministic successful owner run.
  Derive that owner document directly from the verified backing data rather
  than merging child HTML. In served mode, sibling target URLs are virtual
  projections of that backing data and never require sibling HTML files.
  Single-target publication uses the same one-page `ProgramPagePortfolio` and
  one-row exhaustive `TargetOutcomePortfolio`; it has no direct page,
  manifest, report, or browser fallback.
- A multi-target run contains a target-local preparation, analysis, semantic,
  or page-validation failure instead of discarding completed sibling pages.
  Persist one exhaustive, adapter-neutral `TargetOutcomePortfolio` for every
  selected target: a row is either bound to one complete ProgramTarget/page or
  carries only a closed public failure stage and reason. Never persist raw
  errors or adapter-native refs in that authority and never turn a failed
  target into a partial page. The picker keeps failed rows visible,
  red, disabled, and linkless; the repository overview reports analyzed versus
  selected coverage. Materialize each selected JS/TS compiler project at its
  own target boundary so a missing compiler does not preflight-fail unrelated
  languages or packages. A shared selected-target Go workspace is only an
  optimization: if its union cannot be prepared, retry the current exact target
  locally and keep every later Go target isolated; never reuse a target-local
  fallback workspace as sibling authority. Context cancellation and shared
  portfolio, persistence, manifest, repository-overview, or bundle failures
  remain publication-terminal. When at least one target succeeds, the analyzed-page
  portfolio and its neutral bundle remain valid even if that is the only
  successful page; the first successful page owns the one physical HTML while
  the originally selected default remains the logical default in
  TargetOutcomePortfolio. If every selected target
  fails, retain diagnostics but do not invent a targetless or synthetic report.
- The current JavaScript/TypeScript slice owns every eligible `package.json`
  project. Package-source ownership is assigned to the deepest containing
  manifest; every manifest with at least one owned tracked JavaScript/TypeScript
  source contributes one required target representative. A source-less
  manifest is tooling rather than target authority and cannot suppress sibling,
  child, or ancestor package targets. TargetPortfolio retains all exact package
  targets and chooses only their repository default; an explicit
  `jsts:<manifest>` narrows the typed plan to that one owned package before
  TypeScript compiler execution. `package.json#name` is optional: an exact
  top-level npm lockfile name is the secondary package identity, otherwise the
  root uses `root-package` and a nested package uses its repository-relative
  project directory. These fallbacks never authorize an implicit string-form
  `package.json#bin` command. Its corpus-only scout owns the exact
  manifest/source-ownership catalog, target identities, and package candidates
  without invoking Node or the TypeScript compiler; it participates in the same
  portfolio and typed execution plan as Go and Python. Each retained or
  explicitly selected package target receives its own page, which uses an
  owner-prepared, repository-local TypeScript Compiler API to honor
  `tsconfig.json` or `jsconfig.json`, repository-confined solution-style
  project references, aliases, and module resolution. A project reference that
  stays inside the owning package extends that page's complete compiler graph;
  an exact repository-local reference outside the package is a cross-target
  boundary and does not pull the sibling package into the page. Missing
  references and references outside the analyzed repository still fail closed.
  Repomap never installs npm, yarn, pnpm, or other packages. Browser and Node
  server surfaces plus
  canonical safe, package-owned, tracked `package.json#bin` command/path pairs
  are product surfaces inside their owning page-local ProgramTarget. A CLI
  surface never invents a bin-wrapper-to-source relation: its entry refs stay
  empty, while an exact canonical `dev` or `start` script with one
  helper-selected source may independently seed that source only after CLI
  product authority exists.
  Compiler/type-resolved declarations and exact external imports are the only
  call-target authority. Every ProgramIndex external symbol carries its exact
  raw package origin plus an adapter-derived `package` or `platform` authority
  kind. Go derives it from the complete build-selected `go list -deps`
  package-origin universe (including `DepOnly`) and its `Standard` bit; Python
  derives it from the exact `sys.stdlib_module_names` set; JS/TS maps
  TypeScript default-library and Node standard-library origins to `platform`
  and npm origins to `package`. Missing or unknown authority fails closed;
  shared stages never infer it from a package-path prefix. A platform external
  object and an exact external invocation pattern whose complete target set is
  platform-only can never receive `dependency`, support a `dependencies`-lane
  group, or become a matching boundary. They remain valid structural evidence
  and may receive another positively supported category; for example,
  `requestAnimationFrame` may support `background_activity`. Calls and
  constructions retain their distinct exact invocation authority; an
  unresolved property name remains an unresolved frontier and is never matched
  to repository declarations by name alone.
  Compiler authority comes only from `typescript` or an exact npm alias to it
  declared by the selected manifest; a nested package may inherit candidates
  from the repository-root manifest only when it declares none itself. Each
  candidate resolves from the package.json scope that declared it. The
  installed package must identify itself as `typescript`. Distinct compatible
  candidates in the selected API tier fail closed; one stable legacy Compiler
  API candidate is preferred over a native-preview candidate when both are
  deliberately declared.
  Optional signatures and source-expression display text that match the
  always-on persistence guard are omitted locally before the first ProgramIndex
  projection and sealing; the same callsite identity, location, resolution,
  targets, and witness count remain.
  Sensitive required identity or semantic fields still fail closed.
  Shared contracts are supporting code, build/migration scripts remain tools,
  and a runtime script, library, or tool-only root must never promote itself
  into an application.
- Language adapters retain method/path-shaped calls, decorators, arguments,
  reconstructed values, exact targets, alternatives, and unresolved frontiers
  only as neutral ProgramIndex evidence. Protocol meaning and cross-target
  connection authority arise through validated model grouping and matching
  over the complete GroupsIndex set; deterministic stages preserve the neutral
  evidence and its exact provenance.
- Program categorization receives the complete target-local objects and
  relation patterns through deterministic disjoint provider batches, with
  exact incident structure and reduced documentation as request-local evidence.
  At most 32 owned refs per request is semantic-focus partitioning, not a
  repository count limit: complete authority is covered exhaustively, incident
  context is not capped, provider envelopes may split requests further, and no
  subject may be sampled, truncated, or omitted. Every request is sparse only
  in rows and complete for all positively supported ref/category pairs; an
  empty assignment set is legitimate only when none is positively supported.
  Its closed categories are `inbound`, `background_activity`, `dependency`, and
  `core`; one subject may receive several. Framework, protocol, TLS, dependency,
  path, selector, and naming familiarity are evidence only and never establish
  a category locally. A model-selected `dependency` member that contradicts an
  explicit `authority_kind: platform` fact above is an unsupported set member: discard
  and diagnose only that category pair, preserve other valid categories on the
  row, and make the sealed ProgramIndex reject any reintroduced copy.
- Target-local grouping consumes the enriched ProgramIndex and produces one
  sealed `GroupsIndex`. Its closed presentation lanes are `triggers`, `core`,
  and `dependencies`; `triggers` combines inbound and background-activity
  groups visually while their exact categories remain distinct on subjects and
  cards. Every direct group member is locally checked against its lane; evidence
  may cite other advertised context but never becomes membership. An explicit
  platform object, or an exact invocation pattern whose complete targets are
  platform authorities, cannot evidence a `dependencies` group; standard-runtime
  APIs do not become outbound integration evidence. This restriction does not
  apply to connection evidence. Groups are sparse and overlapping. Directed
  local semantic connections use an open snake-case kind and exact restored
  evidence; they need not pretend to be exact runtime calls or deterministic
  call corridors.
- Repository matching consumes only the complete validated GroupsIndex set.
  Every unordered cross-target group pair is considered exactly once to derive
  its complete deterministic candidate set `J(pair)`. Only a pair with a
  non-empty `J(pair)` becomes an indivisible provider item. A zero-candidate
  pair contributes no connection locally and makes no provider, cache, or
  observer call; this is neither a negative semantic fact nor a fallback. Each
  eligible pair's exact dossier advertises the two complete endpoint-group
  member/evidence sets plus separate deterministic `boundary_edge_refs`. Each
  boundary ref names an existing structural `pattern_target`,
  `pattern_receiver`, or `pattern_receiver_origin` edge from a group-owned
  pattern to an exact non-platform external package symbol. The pattern's
  local source object is either an object member or reaches one through a
  finite cycle-safe chain of exact OwnerID/ContainerID facts. Evidence is never
  an ownership root, and calls, paths, frameworks, names, graph adjacency, and
  local semantic connections never enter that closure. An exact qualifying
  edge has exact boundary support. The only non-exact admission is one
  `pattern_receiver_origin` edge with `alternatives` resolution whose endpoint
  group is in the `triggers` lane, whose pattern has `inbound` or
  `background_activity` category, and whose retained origin set contains that
  single external symbol; this remains possible support.
  The dossier retains the deterministic subject-reference closure, complete
  one-hop structural-edge incidence of the endpoint
  member/evidence/boundary-pattern sets, and only target-local connections
  incident to either endpoint with compact neighboring group context; unrelated
  target-local graph facts are not copied into that decision. Before planning
  provider items, Go exhaustively combines the pair's eligible boundary edges with
  arguments owned by each edge's source pattern. It retains every locally
  valid equal direct or reconstructed literal/template value as one closed
  request-local `j*` witness candidate. Each candidate carries its two boundary
  edge refs, source-pattern refs, argument refs, and derived
  `support_resolution`; candidates are evidence only and never create graph
  connections by themselves. Two possible boundary edges cannot form a
  candidate. One possible boundary edge must be paired with an exact edge and
  an exact value match, and that candidate remains possible.
  Matching returns sparse directed cross-target connections. The model authors
  the connection direction, open snake-case kind, label, and summary between
  the two groups in the advertised pair, and selects one or more advertised
  candidates through `witness_joint_refs`. Pair order has no direction
  authority: the `from` endpoint must be the actor described by the semantic
  kind and the grammatical subject of the label/summary, while `to` is the
  acted-on endpoint. When one candidate joins a positive inbound delivery
  pattern in a triggers group to a positive dependency-category exact
  outbound `invokes_external` call, Go advertises that closed
  outbound-to-inbound orientation on the `j*` row and rejects a contradictory
  response; it never flips the model edge. A dual-role subject, arbitrary
  background activity, or bare trigger-lane membership has no direction
  authority. Sparse empty output is legitimate;
  omission of a candidate is not a negative fact, and no candidate is promoted
  without model selection. Go accepts only known `j*` refs, revalidates each
  selected candidate against the same pair authority, automatically restores
  both source-pattern subjects as bilateral connection evidence, and derives
  the connection's strongest surviving `support_resolution` as `exact` or
  `possible`. A row with no surviving selected candidate is discarded.
  The complete eligible batch of candidate-bearing pair items remains atomic:
  any terminal item failure rejects the matching result rather than publishing
  accepted siblings as partial graph authority.
  Possible boundary or value evidence is never promoted to an exact runtime
  call, binding, or occurrence. The response carries no supplementary subject
  evidence. The matcher preserves all target-local groups, structural edges,
  and connections unchanged. It extends that same graph; it never creates a
  protocol-specific or second graph and never asks browser code to infer
  matching from names or paths.
- The ordinary Go direct-call traversal is complete for the selected target:
  `--depth 0` and `--edges-limit 0` are the defaults and mean retain every
  reachable exact call and edge. Positive values are explicit user-requested
  narrowing controls. Graph depth above 10, more than 10,000 exact edges, or
  more than 65,536 exact nodes emits one aggregate warning but never truncates
  the ordinary graph or fails a target. Dynamic and unresolved call frontiers
  remain represented separately. Warning derivation is diagnostic-only and
  can never reject an accepted target or publication.
- ProgramIndex version 11 retains every source-distinct nested pattern without
  local sampling or truncation, including its exact location, neutral
  call-result/receiver provenance, any exact callback source-argument
  provenance, and reconstructed value candidates with their source-object and
  source-argument provenance. Duplicate compiler witnesses do not become pattern omissions.
  The former 64 MiB aggregate-semantic-text and 128 MiB canonical-JSON sizes
  are ordinary scale warnings only. Structural JSON overhead never consumes
  semantic evidence authority, and crossing either size cannot reject or
  truncate the index.
- Everything the report shows is a deterministic fact, a claim quoted from a
  human-written artifact, or a model hypothesis, and the three are always
  labeled. `facts.json` holds the anchored fact layer: entrypoints, HTTP routes
  and client calls with method and path literals, cross-target portals,
  environment keys, the places where the program runs code it was given,
  manifest rows, TODO markers, imports, dead modules, negatives, and
  dependencies. `claims.json` holds quotes with their
  source path, date and age. `orientation.json` holds the model's repository
  summary, roles, run recipe, and main flow; every row cites fact, claim, or
  subject ids, and a row whose refs do not resolve goes to `rejected.jsonl`
  with its raw output and reason instead of being repaired. Validation
  annotates and never aborts the run.
- The report is one static page rendered in Go. Its reader is a newcomer, so
  pipeline vocabulary never reaches the screen: retained, source-bound,
  authority, projection, selector, outcome, target contract, and raw selector
  strings such as `python:backend:guard:main` are banned on screen. The page
  ships only its own small stylesheet and one optional editor-link script; it
  embeds no analysis payload.
- Semantic output and the current HTML report are canonical English. There is
  no `--lang` flag until a separately approved final presentation-localization
  stage actually exists.
- Repository changes during a run do not fail publication. Do not reintroduce a
  freshness gate or strict-snapshot mode.
- `--no-serve` requires resolvable GitHub or GitLab source links and fails in
  preflight with corrective flag guidance otherwise. Served reports may only
  add manifest-authorized local VS Code opening; do not add browser APIs for
  workspace reads, investigation, symbols, source context, or run selection.
- Persistent caches remain part of the ordinary path. Cache hits must be
  identity-bound and fully validated before use; `--no-cache` is the explicit
  live-provider bypass.

## Development and acceptance

- `fixtures/python-tutorial-game` is the acceptance fixture: a tracked copy of
  that repository at revision `78714d34ee` with `expected.json` and the sealed
  artifacts of one real run. Its focused test rebuilds the fact layer and
  asserts every expected row with its anchor.
- Focused discovery and ProgramIndex regressions keep exactly one cumulative
  real repository fixture per active language under
  `testdata/repositories/<language>`. Extend that repository with new scenario
  files instead of creating per-bug repositories. Every tracked fixture file
  must have an exact file-inventory expectation; deterministic language
  adapter stages run for real, while provider-backed stages use exact
  request-bound, fail-closed local presets with no network access. Fixture
  success is focused test evidence only and never replaces ordinary online
  product acceptance.
- Keep the source-bound development inventories under `testdata/contracts`
  exact. Every static prompt Markdown file is classified as active or dormant
  and bound to its owning `go:embed`; every production hard-bound symbol is
  classified, while material anonymous Go/JavaScript boundary forms are
  rejected; every Go test file under the product roots `cmd`/`internal` and
  every repository-owned JavaScript/TypeScript test script is inventoried,
  with non-ordinary compiler, interpreter, browser, subprocess,
  network, environment, external-checkout, or cumulative-fixture requirements
  recorded explicitly. Update the inventory and its contract test together
  when one of those surfaces changes.
- Build the owner-facing binary with `make build`; it must write
  `.bin/repomap`.
- Canonical `make test` and `make vet` use the ambient system Go build and
  module caches, cap package-level parallelism at four, and give each test
  binary at most five minutes. Focused commands must keep the same or tighter
  bounds; ordinary development must not relocate `GOCACHE` or `GOMODCACHE`.
- Product acceptance means running that binary on a real repository through
  the normal online provider path. Offline runs, fixtures, replay commands, and
  helper tools are not acceptance evidence.
- Verify the process exit status and the generated manifest, exact reduced
  documentation, sealed enriched ProgramIndex set, each target-scoped
  `dependency-catalog.json`, every GroupsIndex, the complete matched graph in
  report JSON, and report HTML. For a multi-target run, verify
  every backing manifest/report JSON and exactly one physical report HTML in the
  successful owner run. For cache changes, also verify a real second run and `repomap
  cache clear`.
- Browser QA for a generated standalone report serves the narrow run root that
  contains the owner report and any backing sibling target-run directories from
  a temporary loopback-only `python3 -m http.server`, then opens the corresponding
  `http://127.0.0.1:<port>/<run>/report.html` URL; do not rely on `file://`
  behavior.
  This is a development-only inspection step, not a supported repomap command,
  product server, or checked-in sidecar entrypoint.
- Run focused Go tests for changed contracts and `go vet` for changed
  packages. Never leave known broken tests.
- Debug artifacts must never include API keys or Authorization headers and must
  never be committed.
