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
not a competing roadmap: current exploration is calibrated in M3 and becomes
progressive in M5, feature work is M6, and onboarding is M7.

## Status

| Milestone | Status | User-visible outcome |
| --- | --- | --- |
| M1. Source-grounded symbol | **complete** | one selected symbol is explained from exact source evidence and connected to relevant tests |
| M2. Investigation loop | **complete** | repository, flow, symbol, source, tests, claims, unknowns, and next actions use one resumable state machine |
| M3. Quality suite | **complete** | the same golden tasks expose orientation and drill-down regressions on five large Go repositories |
| M4. Fresh local memory | **active** | saved facts, claims, and sessions survive restart and invalidate safely when the repository changes |
| M5. Browser journey | planned | `./repomap` opens a progressive map, shows analysis/API progress, and opens evidence in the editor |
| M6. Ticket playbook | planned | a real ticket produces a bounded change surface, risks, tests, unknowns, and a useful first edit location |
| M7. Shared playbooks | planned | bug, onboarding, and impact analysis reuse the same evidence loop |

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
7. `./scripts/check.sh` and `./scripts/etcd_check.sh ../etcd` pass, and one
   replayable command produces the complete `kvServer.Put` artifact set without
   credentials or Authorization data.

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
local etcd run through `./scripts/investigation_handoff_check.sh`. Selecting and
reading a bounded test body remains M3/M7 work; M2 only promises that the shared
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
not required by `./scripts/check.sh`.

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
the stronger relation. `scripts/quality_preflight.sh` checks its necessary
precondition before any model call—the symbol path must occur in the bounded
orientation context—and records the clean revision, toolchain, exact model
contexts, requests, hashes, and prompt versions.
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

M4 is now active. Its first slice must inventory the existing `internal/index`
contract and prove one safe stale-record rejection before broadening stored
state or wiring persistence into the browser.

## M5 — Browser journey

Open the current repository by default, show deterministic analysis progress and
the exact bounded data sent externally, reveal one investigation branch at a
time, retain alternative directions, and open a selected source location in the
editor. The browser consumes saved application state; it does not own analysis.

An early friend-test baseline now covers the first half of this contract:
`./repomap` targets the current directory, reports compact-context and exact
request bytes, makes one `deepseek-v4-flash` orientation call, retains every
validated direction, and opens a static HTML report. M5 remains planned because
direction selection, progressive evidence steps, session-backed browser state,
and editor opening are not wired.

## M6 — Ticket playbook

Use a real etcd or k6 ticket to identify a change surface, analogous code,
affected flows, relevant tests, risks, unknowns, and the first useful source
location. The ticket is an initial goal and ranking policy, not a separate prompt
pipeline.

## M7 — Shared playbooks

Add bug, onboarding, and impact-analysis policies over the same evidence,
claims, actions, budgets, and stopping rules. Language and editor integrations
remain replaceable adapters; Go stays the implementation and quality target
until the Go journey is complete.

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
