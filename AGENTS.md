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
  request-local evidence to the configured model provider. The initial README
  file-role classifier may send the complete names-only tracked-file
  dictionary as a lossless prefix-compressed path tree with compact `f*`
  leaves, complete textual README documents, and sparse local lexical counts;
  it sends no other source-file contents.
- Domain cubes own `State`, bounded input preparation, and semantic
  validation. The shared LLM layer owns exact provider requests, transport,
  retries, provider-envelope/JSON decoding, batch execution, cache, accounting,
  and semantic-journal events.
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
  resolution, and graph restoration remain local. Unknown refs are rejected
  rather than guessed or repaired; absolute host paths are never sent.
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
- Interesting activity entrypoints are selected from an exact symbol graph.
  Framework, protocol, TLS, dependency, and naming heuristics may advertise
  candidates but cannot establish entrypoint authority by themselves.
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

- Build the owner-facing binary with `make build`; it must write
  `.bin/repomap`.
- Product acceptance means running that binary on a real repository through
  the normal online provider path. Offline runs, fixtures, replay commands, and
  helper tools are not acceptance evidence.
- Verify the process exit status and the generated manifest, sealed
  ProgramIndex set, report JSON, and report HTML. For cache changes, also verify
  a real second run and `repomap cache clear`.
- Run focused Go tests for changed contracts and `go vet` for changed
  packages. Never leave known broken tests.
- Debug artifacts must never include API keys or Authorization headers and must
  never be committed.
