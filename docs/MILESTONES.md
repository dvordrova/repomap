# Product milestones

This is the execution order for repomap. Exactly one milestone is active at a
time. A milestone is complete only when its user-visible outcome, replayable
artifacts, and normal verification pass together; completing an isolated
collector, prompt, or contract score is not sufficient.

repomap is a local-first guide through an unfamiliar Go repository. It first
offers useful runtime or event-oriented directions, then helps the user follow
one bounded, evidence-backed path through symbols, source, and tests. It keeps
navigation hypotheses, source-supported claims, test support, runtime
observations, and unknowns distinct.

DeepSeek through its OpenAI-compatible API is the reference interpretation and
product-quality target. The main CLI can also use one explicitly configured
OpenAI-compatible company endpoint; generic and legacy configuration namespaces
are never mixed. Existing Qwen 1.5B experiments remain useful research, but
local-model work is not a completion gate for the current milestones. A future
`--really-dumb-model` profile may select alternate implementations of the same
typed capabilities without changing their inputs or outputs.

[ENGINEER_TRIAL.md](ENGINEER_TRIAL.md) is an acceptance track across this order,
not a competing roadmap: M3 calibrated quality, M4 made one investigation
durable, M5 is the handoff-ready onboarding trial, M6 makes repository
exploration freely navigable, and feature work follows in M7.

## Status

| Milestone | Status | User-visible outcome |
| --- | --- | --- |
| M1. Source-grounded symbol | **complete** | one selected symbol is explained from exact source evidence and connected to relevant tests |
| M2. Investigation loop | **complete** | repository, flow, symbol, source, tests, claims, unknowns, and next actions use one resumable state machine |
| M3. Quality suite | **complete** | the same golden tasks expose orientation and drill-down regressions on five large Go repositories |
| M4. Fresh local memory | **complete** | saved facts, claims, and sessions survive restart and invalidate safely when repository/tool/prompt inputs change |
| M5. Friend onboarding trial | **active** | a company engineer runs one command on a known Go project, evaluates its map, and chooses one direction for a first drill-down |
| M6. Free repository exploration | planned | the user can deepen, branch, backtrack, and resume anywhere interesting without losing provenance or crawling the whole repository |
| M7. Ticket playbook | planned | a real ticket produces a bounded change surface, risks, tests, unknowns, and a useful first edit location |
| M8. Shared follow-on playbooks | planned | bug and impact analysis reuse the same evidence loop after onboarding, exploration, and ticket work |

## M1 — Source-grounded symbol

The reference journey is `etcd`'s `kvServer.Put`:

```text
exact symbol
  -> bounded static neighborhood
  -> bounded line-addressable target source
  -> DeepSeek source-supported claims
  -> related test evidence
  -> explicit unknowns and next action
```

Done when all of the following are observable:

1. A Go source collector reads only the resolved repository-relative target,
   enforces repository containment and byte/line limits, rejects symlink escape,
   and emits deterministic evidence IDs for exact source lines.
2. The source card is a versioned contract independent of gopls and DeepSeek.
   It explicitly says when the captured window is truncated or only a lexical
   boundary rather than a parsed function body.
3. DeepSeek receives only the bounded card plus required structural context.
   Every normalized behavioral claim cites existing source evidence IDs.
4. Parsing recovers safe weak-format drift with warnings, while local validation
   rejects unknown evidence, unsupported paths, invalid support levels, and
   invented executable actions.
5. Related tests are collected as evidence rather than invented by the model.
   The output distinguishes source-supported from test-supported claims.
6. A deliberately misleading-name fixture prevents names or static calls from
   being promoted to source truth without supporting lines.
7. Focused Go tests and a direct built-binary etcd run pass, and one replayable
   fixture produces the complete `kvServer.Put` artifact set without credentials
   or Authorization data.

Completed on 2026-07-10. The DeepSeek v2 replay assessed four questions with
zero parser warnings and a 100/100 contract score from a 4,286-byte source
bundle. The local follow-up found nine `_test.go` references with gopls
provenance and kept them explicitly below `test_supported`.

## M2 — Investigation loop

Move the M1 journey through one pure reducer:

```text
goal -> focus -> questions -> evidence -> claims -> support assessment
     -> next action -> new evidence -> reassess -> stop or user choice
```

Done when repository orientation can hand one selected flow or symbol into the
same saved session, unsupported claims request bounded evidence, repository
changes and user redirection are explicit events, and no collector or model
client is embedded in the reducer.

The first implementation must replay the completed `kvServer.Put` path with
data-only state/events/actions. It composes the existing symbol, source-card,
source-assessment, and test-reference cubes; it does not rewrite them or add a
generic plugin registry.

Completed on 2026-07-10. An orientation report can hand a selected candidate
flow plus a user-confirmed exact symbol into the same session without promoting
entrypoint prose to a symbol fact. The session pins the exact report hash,
canonical repository root, and accepted revision. Passive resume, explicit
continue, finish, same-revision symbol redirect, and repository-change
invalidation are wired at the CLI boundary; the reducer remains pure and owns no
collector, context, filesystem, model client, or presentation call.

Replay coverage includes unit fixtures for every handoff/resume choice and a
local etcd integration fixture. Selecting and
reading a bounded test body remains M3/M8 work; M2 only promises that the shared
loop reaches an explicit user choice without silently expanding it.

## M3 — Quality suite

Maintain small golden tasks for etcd, Grafana k6, Prometheus, NATS Server, and
golangci-lint. Measure useful direction selection, grounded citations, omitted
important evidence, context size, latency, and semantic usefulness separately
from JSON/contract adherence.

Start with a versioned task manifest and replay evaluator. Each task must name a
user goal, repository revision/scenario, expected useful directions or symbols,
forbidden overclaims, and size/latency observations. Saved model responses are
evaluated without another API call; live DeepSeek runs refresh baselines but are
not required by ordinary product acceptance.

The first etcd task is replayable offline. It combines the saved orientation
response, the `kvServer.Put` source assessment, and sanitized gopls test-reference
evidence behind exact artifact hashes. The evaluator reports five independent
direction checks, 21 grounded structured paths, four source predicates, two
useful test paths, contract adherence, bytes, and unknown legacy latencies. It
does not turn these dimensions into a single semantic score, and it leaves 17
free-form evidence strings explicitly unscored. The historical normalized
orientation report is valid for semantic replay but cannot prove adherence to
the original model-output contract, so that check remains `not measured`.

The second task replays a current Grafana k6 capture from revision
`dfa0d07cee2b535fef57077cca261ea5e155f423`. It covers three runtime-oriented
directions, 11 grounded structured paths, and a drill-down from the metrics REST
API direction into `Client.Metrics` and a related gopls test reference. Its raw
orientation response and source response both have measured clean contracts;
exact compact provider-request byte counts and hashes were recorded from local
ignored capture artifacts. Those request bodies are not committed, so the
offline loader reports but cannot recompute that metadata. Latencies were not
instrumented and remain explicitly `null`. The live run also exposed and fixed
a parser false positive: extensionless `/v1/metrics` is an HTTP route, not a
repository file path, while actual structured file paths remain
allowlist-checked.

Evaluator v3 additionally requires the drill-down source path to occur in the
selected orientation candidate. This prevents two independently useful but
unrelated artifacts from passing as one journey. All five current tasks satisfy
the stronger relation. Its provider-free preflight contract checks before any
model call that the symbol path occurs in bounded model context and records the
clean revision, toolchain, exact contexts, requests, hashes, and prompt versions.
The quality loader also rejects obvious credentials in a manifest or hashed
artifact without copying the detected value into its error.

The third task replays a current Prometheus capture from revision
`af77de9a5fd8b5391eb65ad770a454c9e84346c2`. The orientation selects
the model-proposed `TSDB Write Path (Ingestion to WAL)` and links it to
`Labels.IsValid` in
`model/labels/labels_common.go`; the drill-down retains one locally grounded
call-result observation under the name-seeded `validates_input` question and a
compatible reference in
`model/labels/labels_test.go`. Both raw `deepseek-v4-flash` wire contracts are
clean; 35 mixed evidence-prose items remain explicitly unscored.
The orientation request was 39,979 bytes and took 27,247 ms; the source request
was 6,228 bytes and took 9,216 ms.

This capture exposed a source-context bug before it became a fixture. The old
fixed `anchor + 2 lines` candidate window omitted `return err == nil` after a
multiline callback, so a correct-looking model verdict could not be grounded.
The source cube now tokenizes only a bounded lexical window and selects the
unique call anchor plus immediate returned nil comparison. An incomplete model
citation becomes ambiguous with an explicit score-reducing warning. Captured
prompt v3 asks the model to cite both lines; the committed response does so
without repair. Current prompt v5 also defines the syntax-only checked-result,
direct-return, and branch-call predicates explicitly.

The NATS and golangci-lint preflights exposed a deterministic context-selection
failure before any model call. NATS spent 56 of 60 paths on tests, while
golangci-lint spent most paths on auxiliary main packages and documentation and
omitted `pkg/commands/run.go`. The selector now boosts production files in
packages imported directly by user-facing root/cmd/primary/CLI entrypoints,
reserves bounded source/test/doc diversity, and leaves six slots to global
score. At the 60-path product limit it retains the existing etcd `key.go`, k6
`metrics.go`, and Prometheus `query.go` bridges while adding NATS
`server/client.go`/`server/parser.go` and golangci-lint `pkg/commands/run.go`.
Only real documentation formats enter `known_docs`, and every model-visible
document, source signal, entrypoint file, and orientation-candidate file is
filtered to `allowed_paths` after selection. All previewed requests still use
`deepseek-v4-flash`.

Exact prompt-only runs for NATS `client.processInboundMsg` and golangci-lint
`runCommand.runAnalysis` now reach the selected source path and seed bounded
syntax-only questions without using callee names as behavior. NATS exposes four
locally visible case-branch calls; golangci-lint exposes three immediately
checked call results and one direct returned call. The NATS capture is now
committed, and the golangci-lint capture now closes the five-repository matrix.

The first NATS live orientation was correctly rejected because the model placed
the real but unprovided `server/server.go` in a structured `likely_files` list,
even while warning that it was outside `allowed_paths`. Rather than weakening
path validation or retrying until lucky, file ranking now gives a small generic
boost to a source file named after its directory. The next NATS preflight keeps
`server/server.go`, `server/client.go`, and `server/parser.go` inside the same
60-path bound; no NATS-specific path is hard-coded.

The fourth task replays NATS Server revision
`1be499156d9bc757ea08bd148608b622e38b7514`. Three useful directions cover
server startup, client message publishing, and JetStream storage. The client
direction links `server/client.go` to `client.processInboundMsg`; four
`calls_from_branch` claims cite each exact `case` and call pair, while runtime
branch selection and callee behavior remain unknown. One compatible
`server/jetstream_cluster_2_test.go` reference remains navigation evidence only.
Both raw `deepseek-v4-flash` contracts are clean and the source response scores
100/100 without repair. The orientation request was 27,644 bytes / 21,877 ms;
the source request was 7,495 bytes / 6,995 ms.

The first golangci-lint live orientation was rejected because it copied package
directories such as `pkg/commands/internal/migrate/versionone` into
`likely_files` and emitted trailing-slash package paths under
`unverified_paths`. Orientation prompt v3 now requires an exact membership
check for every verified file field and states that directory/import/package
paths are never files. This keeps the strict local path validator unchanged.
The next response obeyed the file shape but named the real
`pkg/lint/linter/linter.go`, which the entrypoint-only ranking had omitted even
though `pkg/commands/run.go` imports it. A lower-priority, source-only
`entrypoint-second-hop` signal now admits bounded files from exactly that next
import layer while direct entrypoint dependencies retain higher priority. The
latest preflight contains both the linked target and that linter anchor.

The fifth task replays golangci-lint revision
`9b5e24cba6e9964465bc892ab9377fae5a60cb97`. Three useful directions cover the
CLI lint run, configuration migration, and internal caching. The CLI direction
links `pkg/commands/run.go` to `runCommand.runAnalysis`; three
`checks_call_result` observations cite the call and immediate error guard, and
one `returns_call_result` observation cites the direct `runner.Run` return. One
compatible `pkg/lint/lintersdb/manager_test.go` reference remains navigation
evidence only. Both raw contracts are clean and source scores 100/100. The
orientation request was 45,001 bytes / 22,691 ms; the source request was 7,984
bytes / 8,748 ms.

The five-repository golden matrix now passes offline. The requested
company-style `REPOMAP_LLM_*` compatibility/calibration run is recorded below
and is judged by the same separate quality dimensions.

Completed on 2026-07-10. A sixth task calibrates the atomic generic provider
namespace against the requested `deepseek-v4-flash` reference endpoint/model.
The run passed `doctor --check`, exact request preview, NATS orientation, linked
source assessment, and test-reference collection. Offline replay covers three
directions, 16 grounded structured paths, 11 important-path checks, one linked
branch-call predicate, one compatible test path, clean raw contracts, and a
100/100 source contract. The orientation request was 28,355 bytes / 24,851 ms;
the source request was 7,495 bytes / 5,100 ms. This proves the generic
configuration and OpenAI-compatible transport path against the reference model;
it does not prove quality on an arbitrary company-hosted model, which remains an
explicit research question.

## M4 — Fresh local memory

Persist repository facts, derived claims, and investigation sessions separately.
Freshness includes repository identity, HEAD, dirty-file content, Go/gopls and
collector versions, build context, prompt version, and evaluator version.

Completed on 2026-07-10. `freshness.RepositoryState` stabilizes two consecutive
observations of the canonical repository identity, HEAD, non-ignored dirty
contents, and ignored Go build inputs without reading unrelated ignored secrets.
`FactContext` additionally pins Go/gopls, collector schema, build context,
GOFLAGS/GOWORK/CGO, and analyzer/collector options. `ClaimContext` pins the fact
document, provider/model provenance, prompt, parser, and evaluator versions.

`internal/index` v2 now rejects a schema-valid snapshot before exposing any
symbol when its current fact context differs. The investigation path writes
content-addressed `facts/` and `claims/` documents plus a small session
checkpoint, verifies hashes through a confined filesystem root, and reconciles
repository, fact, and claim changes through separate reducer events. Repository
or analyzer changes re-resolve the symbol; prompt/evaluator changes retain local
source facts and return to `assess_source`.

The completed proof includes unit coverage for same-HEAD/different-dirty bytes,
tool/options and prompt/evaluator changes, tampered and symlinked artifacts,
atomic concurrent index saves, and strict restart. The etcd handoff resumes from
a copied memory directory, while a live `deepseek-v4-flash` run retained the
exact same fact and claim hashes after a second no-network resume. The current
session slice conservatively drops all focused facts for any repository-content
change; dependency-selective reuse is a measured future optimization, not part
of M4.

## M5 — Friend onboarding trial

Make the existing browser journey handoff-ready for one engineer evaluating a
Go project they already know. The friend gets a binary plus a short setup,
configures either the DeepSeek reference or one OpenAI-compatible company
endpoint, runs one command in the repository, and sees deterministic progress,
the exact bounded external payload, a useful system overview, entrypoints,
subsystems, and alternative directions. They can choose one named direction and
reach a first evidence-backed drill-down without knowing an exact gopls symbol.
The browser consumes saved application state; it does not own analysis.

The first runnable friend baseline now targets the current directory, makes one
`deepseek-v4-flash` orientation call, preserves the complete onboarding shape,
shows context/request bytes and latency, and prepares a bounded local evidence
bundle for every direction without another provider call. Direction cards open
those saved neighborhoods, focused Go fixtures build and replay the handoff
fixture, and each run creates a non-overwriting correct/missing/misleading
feedback note. The current etcd proof produced three clickable directions from
a 42,159-byte compact context and a 49,509-byte external request in 30,983 ms,
stored explicit `local_only` status for every direction, and capped each
neighborhood at 20 file/test/doc items.

The first hands-on review exposed useful strengthening work before external
handoff. The report chrome is now compact: debug paths live under `Run details`,
direction tabs no longer repeat the internal `Local evidence` state, local
rankings are labelled as suggested files rather than a proven read sequence,
and raw retrieval reasons are translated for the product view. Flow retrieval
no longer promotes a generic `main.go` basename or derives package edges from
discarded lexical candidates. The repository survey also retains a bounded
third-hop Raft integration file instead of allowing one nearer package to own
the complete source budget. Remaining package-first, direction-diversity,
external-boundary, and unverified-path work is recorded in TD-013 through
TD-016 rather than hidden behind more etcd-specific weights.

The handoff now has an optional local presentation adapter: the default command
serves saved reports on a random loopback port, the header can switch runs, and
grounded file references open through a validated `code --goto` action. The
analysis artifacts and static HTML remain independent of the server, and
`repomap serve` can reopen the latest run without another provider call.

Evidence locations now cross the model boundary through a canonical local
contract. Model variants such as `file.go line 42` and `at line 42` are checked
against deterministic `source_signals` and persisted as `file.go:42`; an
unverified line is removed instead of becoming an editor link. The browser only
interprets this canonical form. The direction/location-to-symbol bridge is now
implemented for served reports. A versioned run manifest binds the exact report
authority to repository HEAD and dirty contents. The browser sends only
component/anchor IDs; the loopback server checks freshness before and after
gopls, returns at most eight callable candidates, and accepts the next action
only through opaque server-owned IDs. The selected declaration is confirmed at
the exact source position and enters the existing investigation runner for a
bounded source card plus five direct static callers/callees. Discovery and
inspection make no provider call.

The architecture canvas is now the primary exploration surface rather than a
row of passive cards. ELK lays out locally validated conceptual subsystems and
routes quiet witnessed structural edges around node content. One selected
FlowProof stays on that same canvas: unrelated components dim, main/task/shared
branches remain distinct, and exact steps, transitions, conditions, joins, and
frontiers open in the adaptive inspector. Model synthesis may name and group
only supplied opaque member IDs; it cannot create relations or proof. The
detailed proof ledger remains available as secondary trust/debug detail.
Exact-symbol drill-down still uses the manifest-authorized component/anchor
identity and starts with a compact static neighborhood before the longer
bounded source and call lists.

Persisted Go runs now add a separate `Discovered surfaces` shelf below that
canvas. It shows bounded local HTTP registrations, async starts, worker-loop
evidence, and analysis frontiers without turning them into model
recommendations, component edges, or completed FlowProof. Discovery is
default-on for this artifact path, remains explicitly disableable, skips
non-Go/no-debug/preview runs, and degrades to a saved warning instead of
discarding orientation.

The friend run also confirmed that strict structured fields need a tolerant
repair boundary before validation. A provider returning a package or directory
such as `internal/compact` inside `likely_files` no longer aborts the complete
onboarding pass: that one entry is removed with a warning, exact remaining
files or evidence anchors repair the flow, and only a flow with no grounded
file at all is discarded. Directories are never expanded into guessed files.

The live Pebble proof followed `Batch Operations -> batch.go ->
(*Batch).Commit:1571`, returned its five-line lexical source window, one direct
callee (`Apply`), and five bounded callers with an explicit static-not-runtime
warning. The selected symbol/source now persists in the run-local investigation
store, resumes after a server restart without the candidate cache, and can find
up to five direct `_test.go` references through an explicit local-only action.

The isolated deeper-research proof now continues that same component without
turning the default survey into an eager crawl. One component-planning call
selects an opaque primary question and exact-symbol candidates. Two local probe
rounds then established the bounded static path `Batch.Commit -> Apply ->
applyInternal -> commitPipeline.Commit`, rejected `directWrite` as a disconnected
ingest lead, and bound the accepted frontier to the exact prior artifact hash.
A path-free 27 KiB teacher bundle produced eleven surviving grounded explanation
items from one logical model call; every item resolves through a separate local
`file:line` index. The tolerant parser preserved valid siblings while dropping
one model-authored closed-world claim that exceeded bounded static evidence.
The planner, both probe rounds, and the teacher output now also compose offline
through a presentation-neutral, SHA-bound `researchtrail` adapter. Local
locators remain separate from claims, so the browser can project the same trail
without importing planner/probe/teacher schemas directly.

A restic calibration exposed a more fundamental onboarding failure: a compact
bundle could contain `cmd/restic/main.go` only as a filename, omit the backup
command body behind package truncation, and still present provider-authored high
confidence. The first correction is now local and replayable. A package-local
command-framework seam has a Cobra reader that reconstructs the exact bounded
prefix `main -> root constructor -> AddCommand constructor -> Run/RunE handler`
and typed handler call sites. The restic bundle now contains
`main:161 -> newRootCommand:36 -> newBackupCommand:35 -> runBackup:498`, plus
exact call sites for repository open/index loading, scanner creation,
`archiver.New`, and `Snapshot`. Unresolved selector targets remain explicitly
unresolved. A deterministic post-model gate caps confidence and records
verified/missing evidence; the browser no longer has to imply that provider
confidence is evidence confidence.

The bounded restic proof increment is now complete. `FlowProof` stores the CLI
slot contract, exact anchors/transitions, budgets, statistics, pending work, and
stop reason as JSON. A unique worklist resolved the live `Backup Command Flow`
through `main:161 -> newRootCommand:36 -> newBackupCommand:35 -> runBackup:498`,
then exact callsites for `openWithAppendLock:542`, `LoadIndex:577`,
`NewScanner:643`, `Scan`/`errgroup.Go:652`, `archiver.New:655`, and
`Snapshot:698`. The lifecycle cube linked `cancel:701 -> Wait:704 -> return:718`.
All eight CLI slots completed in six local tasks over four evidence files with
zero additional model calls. The ordinary live run used one provider request;
the same proof replays through `cmd/flowproof-playground` without credentials.

Resolution and invocation are now separate evidence axes, so a statically
selected target is not confused with a runtime observation. Targeted
`go/packages`/types was sufficient for this slice; repository-wide SSA, VTA,
SQLite, and SCC processing remain deferred until an unresolved focused task
demonstrates the need.
M5 remains active only until the knowledgeable friend evaluates this pass on a
project they know. Editor opening and model assessment do not block that
external calibration.

## M6 — Free repository exploration

Turn the guided onboarding map into a user-directed workspace over the same
saved evidence and investigation actions. A user can start from a subsystem,
direction, file, or exact symbol; go deeper into callers, callees, source, and
tests; branch to a sibling question; return through breadcrumbs; and resume the
trail after restart. Depth and context budgets remain explicit, and the tool
must never simulate freedom by eagerly crawling the whole repository.

The first proof is one browser trail with `deeper`, `back`, and `branch` actions
whose saved nodes retain repository/fact/claim provenance. It reuses M4 memory
and the M5 direction-to-symbol bridge rather than creating a second exploration
engine.

## M7 — Ticket playbook

Use a real etcd or k6 ticket to identify a change surface, analogous code,
affected flows, relevant tests, risks, unknowns, and the first useful source
location. The ticket is an initial goal and ranking policy, not a separate prompt
pipeline.

## M8 — Shared follow-on playbooks

Add bug and impact-analysis policies over the same evidence, claims, actions,
budgets, and stopping rules after the onboarding and ticket trials. Language
and editor integrations remain replaceable adapters; Go stays the
implementation and quality target until the Go journey is complete.

## Focus guard

Before accepting work, ask which active-milestone outcome it unlocks. Defer work
that only improves a technology in isolation, including further 0.5B/3B model
tuning, a universal provider framework, Rust analysis, embeddings, a database,
MCP, or an editor plugin. A small consumer-owned provider seam and the current
language-neutral evidence contracts remain; speculative frameworks do not.

Model-dependent capabilities are composed at the application boundary. The
reference profile wires the current DeepSeek-named implementations through an
explicit OpenAI-compatible endpoint. A later dumb-model profile may
use multiple smaller prompts, deterministic reducers, or skip unsupported
capabilities, but it must return the same validated capability output. Do not
put model-name conditionals inside evidence, investigation, or presentation
code.
