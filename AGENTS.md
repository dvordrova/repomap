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
- [docs/agent-room/CURRENT.md](docs/agent-room/CURRENT.md) is the single living
  ADR. Change the relevant section in place when the product changes.
- Historical decisions and deleted planning documents remain available in Git
  at the pre-cleanup commit `4e54ab3`; they are not current requirements.
- [docs/DEEPSEEK_API_NOTES.md](docs/DEEPSEEK_API_NOTES.md) owns only the live
  online transport contract. Prompt and response schemas live with their code.
- The owner has approved the domain-cube pipeline recorded in CURRENT.md and a
  shared LLM-provider executor. Do not add another analysis or presentation
  layer outside that pipeline without fresh approval.

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
- Domain cubes own `State`, bounded input preparation, and semantic
  validation. The shared LLM layer owns exact provider requests, transport,
  retries, provider-envelope/JSON decoding, batch execution, cache, accounting,
  and semantic-journal events.
- Execute independent cube-planned batch items through the shared bounded LLM
  worker pool, with the ordinary product limit set to four. Preserve the
  caller's item index as the only in-memory result slot and replay observer
  events in that order; do not add a random batch identity to semantic or cache
  state. Every provider transport attempt acquires the run-shared adaptive
  gate. An HTTP 429 collapses that gate to one before the provider's existing
  backoff/retry, so already-started attempts finish while that retry and every
  new attempt become serial. Only a terminal item error cancels the batch child
  context and prevents queued items from starting. The cube still rejects the
  complete batch on any terminal item failure; accepted sibling calls may
  retain their exact identity-bound cache entries but never become a partial
  semantic result.
- A cube returns a fully validated result, a contractually legitimate empty
  result, or an error. Backend orchestration, report projection, and browser
  code must never supply semantic fallback, repair, promotion, or partial
  success for failed or incomplete cube output.
- Keep static prompt prose in readable Markdown beside its domain cube and
  compile it with `go:embed`. Go owns dynamic bounded catalogs, not long prompt
  string literals; the provider layer does not own domain prompts or schemas.
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
  incomplete result without inventing a replacement. Orientation groups are
  a complete cover, not a partition: arbitrary repositories may place one
  known responsibility in several legitimate groups, and every such
  membership must be preserved. Only genuinely incompatible assignments for
  one known ref remain an explicit ambiguity; do not apply first-wins repair.
  A refined CoreMap proposal left with no advertised exact symbol after this
  filtering is an unsupported set row and is discarded; CoreMap remains
  terminal when no validated target-core block survives across all shards.
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
  sealed `ProgramIndex`, target-scoped dependency authority, the shared semantic
  cubes, and a validated report page. A selected non-default target is not a
  `structural_only` substitute for that path. Multi-target publication seals a
  language-neutral `ProgramPagePortfolio` keyed by exact `ProgramTarget` IDs and
  child run IDs, then binds the repository-level `RuntimePortfolio` to it. In
  manifest version 35 this neutral page authority is mutually exclusive with
  the legacy Go `TargetRunContainer`/`TargetPagePortfolio` authority. Preserve
  every analyzed child run's manifest-bound `report.json`, ProgramIndex, and semantic
  artifacts, but publish exactly one physical `report.html` in the deterministic
  successful owner run.
  Derive that owner document directly from the verified backing data rather
  than merging child HTML. In served mode, sibling target URLs are virtual
  projections of that backing data and never require sibling HTML files.
- A multi-target run contains a target-local preparation, analysis, semantic,
  or page-validation failure instead of discarding completed sibling pages.
  Persist one exhaustive, adapter-neutral `TargetOutcomePortfolio` for every
  selected target: a row is either bound to one complete ProgramTarget/page or
  carries only a closed public failure stage and reason. Never persist raw
  errors or adapter-native refs in that authority, never turn a failed target
  into a partial page, and never mix it with RuntimePortfolio's distinct
  analyzed-but-unclassified complement. The picker keeps failed rows visible,
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
  `tsconfig.json` or `jsconfig.json`, bounded solution-style
  project references, aliases, and module resolution. A project reference that
  stays inside the owning package extends that page's bounded compiler graph;
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
  call-target authority. TypeScript default-library declarations use the closed,
  non-package `platform:javascript` authority and never enter dependency or
  integration selection. Calls and constructions retain their distinct exact
  invocation authority; an unresolved property name remains an unresolved
  frontier and is never matched to repository declarations by name alone.
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
- A deterministic JS/TS cross-surface path may join a client HTTP use to a
  server route only through an explicit method/path-match relation and retained
  program reachability from a product Node surface; integration/test servers
  remain tools even when they repeat production paths. It must
  retain exact-static, resolved-indirect, possible, and unresolved-frontier
  authority separately and may never invent a missing call, handler, contract,
  storage, or resource step through a model or report projection.
- Interesting activity entrypoints are selected from an exact symbol graph.
  Framework, protocol, TLS, dependency, and naming heuristics may advertise
  candidates but cannot establish entrypoint authority by themselves.
- ProgramIndex version 7 bounds aggregate semantic text at 64 MiB and its
  complete canonical JSON envelope at 128 MiB. Structural JSON overhead never
  consumes semantic evidence authority; neither bound authorizes truncation.
- Semantic output and the current HTML report are canonical English. There is
  no `--lang` flag until a separately approved final presentation-localization
  cube actually exists.
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
- Verify the process exit status and the generated manifest, sealed
  ProgramIndex set, report JSON, and report HTML. For a multi-target run, verify
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
