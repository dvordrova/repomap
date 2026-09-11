# Working on repomap

`repomap` is an online, model-assisted repository orientation tool. Its report
helps a newcomer find a component's requests, commands, workers, external
communication and data, then understand a scenario beside its original sources.

## Start here

1. Read [CONSTITUTION](docs/CONSTITUTION.md) for product invariants. It takes
   precedence over these instructions, CURRENT and topical contracts.
2. Read [CURRENT](docs/agent-room/CURRENT.md) for the current decision and what
   has actually been accepted. It is the single living ADR.
3. Read only the relevant contract sections below, then the owning ordinary
   code and tests. Do not read every contract or the historical archive by default.

| Work | Authoritative implementation detail |
| --- | --- |
| Commands, settings, targets completing/failing, ordinary run | [Surface and orchestration](docs/contracts/SURFACE.md) |
| Corpus, README/documentation, target scouting and ownership | [Discovery](docs/contracts/DISCOVERY.md) |
| Shared graph, source values, materialization or memory | [ProgramIndex](docs/contracts/PROGRAM_INDEX.md) |
| Go extraction, dynamic calls or registration evidence | [Go](docs/contracts/GO.md) |
| Python extraction, package views or import facades | [Python](docs/contracts/PYTHON.md) |
| JavaScript/TypeScript extraction or compiler prerequisites | [JSTS](docs/contracts/JSTS.md) |
| Selection/captions, operations/boundaries, Learn/questions or orientation | The relevant section of [Reading](docs/contracts/READING.md) |
| Prompts, response validation, limits, cache/memo/replay or retries | [Execution](docs/contracts/EXECUTION.md); endpoint details in [DeepSeek notes](docs/DEEPSEEK_API_NOTES.md) |
| Optional definitions or glossary provenance | [Terminology](docs/contracts/TERMINOLOGY.md) |
| Report, navigation, source links, localization or saved rendering | [Report](docs/contracts/REPORT.md) |
| SQL, sqlc or configured external fact commands | [EXTRACTORS](docs/EXTRACTORS.md) |
| Tests, fixtures, builds and product acceptance | [Development](docs/contracts/DEVELOPMENT.md) |

For a change spanning areas, read those areas' contracts. Update the owning
contract in place; CURRENT summarizes a changed decision or acceptance state,
not every test and attempt. Record concise implementation/probe evidence in
[CHANGELOG](docs/agent-room/CHANGELOG.md). Old runs and abandoned designs belong
in the explicitly [non-normative archive](docs/archive/2026-09-10/README.md).
The code on the ordinary main path is implementation truth: verify it before
claiming behavior in documentation.

## Boundaries that apply to every change

- Keep one pipeline: native language evidence → ProgramIndex → places/atlas
  reading → GroupsIndex → ordinary report. Do not add a parallel graph,
  semantic authority, sidecar analysis, script entrypoint or browser analysis.
- Preserve deterministic facts, attributed author claims and model
  interpretations as distinct layers. A path, import, helper name or source
  anchor does not prove a runtime role or effect. Unknown remains explicit.
- The owner requires defining the needed reader outcome and its evidence
  before choosing an optimization. Share parsing, identical native views and
  exact accepted requests without erasing owners, uses or source locations.
- Every eligible selected target keeps its complete ordinary analysis, including
  tools, examples and shared code. A failed target is an explicit outcome, not
  a partial or invented page. Successful siblings survive within that contract.
- Models choose closed request-local refs. Unknown members are discarded;
  unresolved mandatory choices and malformed known rows are refused. Independent
  accepted neighbours survive. Orchestration, projection and the browser do not
  repair, invent, promote or semantically complete a failed model result.
- Request preparation owns complete evidence and provider-sized partitioning.
  Repository scale is neither a correctness boundary nor a warning. Only an
  actual provider envelope, representation overflow, canonical identity/path/
  format validation or explicit user narrowing can justify the corresponding
  limit; never silently sample or truncate evidence.
- Static prompts live in embedded Markdown beside their owning stage. The owner
  supplies one response shape. Optional glossary work is separate; its local
  source context must not be appended to provider messages.
- The selected repository is trusted; this is not a security boundary. Do not
  add scanning, redaction, freshness gates or strict-snapshot mode. API keys and
  Authorization headers never enter request bodies, caches or debug artifacts.
  Keep full source files, raw internal edges, canonical IDs and host-absolute
  paths out of provider bodies. The allowed names-only guidance dictionary and
  complete README/AGENTS documents are specified in the discovery contract.

## Supported paths and verification

The supported commands are `repomap [repository]`, `conf`, `read`, `render`,
`replay` and `cache clear`, with their existing flags. Serving belongs to the
ordinary run; there is no `serve` command. `--no-model` requires an explicit
`--target`. Do not introduce old-format readers or substitute fixture paths for
the ordinary implementation.

Language-cube changes require an understandable source example and executable
expectation in the one cumulative real fixture under
`testdata/repositories/<language>`. Keep `testdata/contracts` file inventories
exact. Check native equivalents in every other supported language immediately;
record a missing equivalent instead of fabricating it. Existing exact coverage
may be reused. Native stages run for real; provider tests use request-bound,
fail-closed local presets. Unrelated green tests or a prose note are insufficient.
Do not recreate the removed prompt/numeric-limit/test-file inventories.

Run focused tests and vet for changed contracts; never leave known broken tests.
Canonical `make test` / `make vet` use ambient Go caches, package parallelism at
most four, and at most five minutes per test binary. Focused commands use the
same or tighter bounds; do not relocate `GOCACHE` or `GOMODCACHE`. `make build`
produces the owner-facing `.bin/repomap`.

Product acceptance requires that binary to complete an ordinary online run on a
real repository, an inspected exit status, its complete artifact chain and a
browser walkthrough. Multi-target publication has one common manifest/report
JSON and one physical HTML. Cache changes also require a real second run and
`repomap cache clear`. Fixture, offline, replay and helper probes do not replace
ordinary acceptance. Before a full expensive run, check a limit/packing change
on one saved complete window. Full verification details remain mandatory in
[Development](docs/contracts/DEVELOPMENT.md#mandatory-fixtures-and-checks).

For UI changes, edit ordinary templates and use `repomap render` on the same
saved report/translations. It makes no provider calls and rejects incompatible
saved inputs. Temporary worktrees are permitted; integrate accepted commits,
preserve uncommitted work, and remove a worktree/branch after integration or
explicit discard. Current layout acceptance is desktop mouse/keyboard. Browser
QA uses a narrow temporary loopback HTTP server, not `file://`; it is not a new
product entrypoint. Follow real questions and return-after-a-break journeys,
not only DOM existence checks.
