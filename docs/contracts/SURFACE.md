# Commands and ordinary run

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Supported commands

`repomap` is an online, model-assisted repository orientation tool. Its
supported user-facing surface is deliberately small:

- `repomap [repository] [flags]` runs the ordinary analysis and publishes the
  report artifacts.
- `repomap conf [repository]` creates the local `.repomap.conf` if absent and
  opens it using its editor setting. This command performs no analysis.
- `repomap replay --file REQUEST.json [--debug-dir DIR]` resends exact saved
  provider bytes through the configured client and refreshes its cached answer.
- `repomap cache clear [--debug-dir DIR]` clears persistent model-response
  caches.
- `repomap render RUN_DIR --output FILE.html` applies the current ordinary
  report templates to a saved common report, manifest and saved translations.
  It performs no analysis, reads no model configuration or response cache, and
  never invokes a provider. Missing or incompatible saved inputs fail without
  regeneration. Only the requested HTML is replaced, after rendering succeeds.
  A changed traversal order reuses saved translations only through a complete
  exact match of the saved/current display catalogues by role, text and protected
  spans. New, changed or missing entries are rejected; no translation is guessed.

`repomap read READING_INPUT.json [--through STAGE] [flags]` runs the same
atlas reading stages from the current saved input format for development.
It can override one stage's prompt and context budgets, saves inputs and
normalized results, and never renders HTML. No old-format adapters or
parallel analysis implementations are supported. There is no separate
`serve` subcommand. Report serving remains part of the ordinary run and is
controlled by `--no-serve` and `--port`. `--no-model` walks the atlas without
a provider, every cell on its fallback line and no orientation, and needs
`--target` because no target is selected without the model. Do not add
script entrypoints or sidecar tools.

## One computation and publication path

- Execute every selected typed target through its own complete page-local path:
  sealed base `ProgramIndex`, target-scoped dependency authority, exact reduced
  documentation, the atlas tables over that ProgramIndex, one target-local
  `GroupsIndex` projected from the atlas, and a validated report page. A selected
  non-default target is not a structural substitute for that path. Multi-target
  publication seals a language-neutral `ProgramPagePortfolio` keyed by exact
  `ProgramTarget` IDs and child run IDs plus one exhaustive
  `TargetOutcomePortfolio`; joints between targets come from matched boundary
  values confirmed by the model. Retain target-local ProgramIndex, dependency
  catalogue and GroupsIndex artifacts. Publish repository-wide facts, claims,
  orientation, atlas, portfolios, manifest, `report.json` and `report.html` once
  in the successful owner run. Stages pass typed values in memory; persistence
  is separate from computation. Identical project/parser views share their
  complete common ProgramIndex input; target-local `program-index.json` stores
  only the target, seeds and a relative binding to `program-facts/<digest>.json`
  in the initial run directory. The existing common builder restores the exact
  sealed target index without parsing code. Distinct package/import contexts
  remain separate facts even when they share ASTs. The owner approved this
  shared storage on 2026-09-09, independently of tools/examples' full analyses.
  Release completed child indexes after persistence; facts, places and group
  projection load one target at a time, without memoizing all children. Places
  loads each target once, retaining only its boundary observations for the
  later source-documentation pass. Its sequential file reader reuses one
  decoded shared input until the project binding changes and releases it
  after graph construction; it never stores the complete child indexes. Other
  handoffs reuse a value in memory when available. The report server consumes the generated result directly,
  or restores one common report and manifest in another process. Every target
  is a section of that common page.
  Single-target publication uses the same one-page `ProgramPagePortfolio` and
  one-row exhaustive `TargetOutcomePortfolio`; it has no direct page,
  manifest, report, or browser fallback.

## Target failure isolation

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

## Helper and publication errors

The embedded JS/TS helper reserves an exit status for compiler-load failures;
stderr remains diagnostic text and cannot choose the public failure reason.
Missing Node retains the same prerequisite cause. Both Python parsers retain
the process exit error even with empty stderr, and enforce their existing
stderr bound through Write without an inherited bytes.Buffer.ReadFrom bypass.
Each helper accepts one complete JSON response; trailing values are refused.
Context cancellation retains its original cause. These transport-error fixes
do not change successful native results, their versions or model cache keys.

The final publication-failure block preserves the successfully analyzed target
count and shows the original wrapped failure reason. It links the owner run's
existing `rejected.jsonl` and semantic exchange directory when available.
These are console diagnostics, not raw-error fields in TargetOutcomePortfolio;
the original error still propagates unchanged.

After a failed model exchange is committed, the same journal recorder supplies
direct absolute request, raw-response and journal paths to the run console.
An unavailable body is identified explicitly rather than linking its marker as
raw content. Buffered first-layer failures notify only after their journal is
flushed; accepted answers with discarded optional terminology do not emit a
failure notice. Payloads are not copied or reformatted for the console.
The provider carries the last transport attempt's HTTP status and selected
diagnostic response headers through the shared executor into this journal and
notice. Request/trace/correlation IDs, retry/rate-limit headers, date and server
are retained; request headers, authorization, cookies and arbitrary token/key
headers are not collected. Total attempts and elapsed time still describe the
whole completion. HTTP metadata is diagnostic only and changes neither request
identity nor accepted-cache records; a warm hit makes no claim about a new HTTP
response. Raw HTTP error bodies remain exact, including non-JSON 500 responses.

## Settings and questions

`--question TEXT` is repeatable and supplements `.repomap.conf` questions. Settings are loaded once and passed as typed values through target work and serving. The ordinary atlas, questions and report use that same configured provider. See [reading](READING.md#questions-and-learn) for question semantics and [report](REPORT.md#source-links-and-serving) for serving.
