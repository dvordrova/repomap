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
| M3. Quality suite | **active** | the same golden tasks expose orientation and drill-down regressions on five large Go repositories |
| M4. Fresh local memory | planned | saved facts, claims, and sessions survive restart and invalidate safely when the repository changes |
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
unrelated artifacts from passing as one journey. Both existing tasks satisfy
the stronger relation. `scripts/quality_preflight.sh` checks its necessary
precondition before any model call—the symbol path must occur in the bounded
orientation context—and records the clean revision, toolchain, exact model
contexts, requests, hashes, and prompt versions.
The quality loader also rejects obvious credentials in a manifest or hashed
artifact without copying the detected value into its error.

The Prometheus preflight at revision
`af77de9a5fd8b5391eb65ad770a454c9e84346c2` selected `QueryRange` in
`cmd/promtool/query.go`: unlike `Engine.NewInstantQuery`, that source path is
actually present in the current 60-path orientation context. The bounded source
step contains one `maps_error` question and has a direct `TestQueryRange` native
test. This is not yet the third baseline: raw DeepSeek responses, test-evidence
capture, expectations, and artifact hashes are still required.

M3 remains active until equivalent small tasks for Prometheus, NATS Server, and
golangci-lint pass the same offline workflow. Two passing captures establish the
workflow on etcd and k6, not yet cross-repository product quality.

The external-company slice adds one compatibility/calibration run through
`doctor`, request preview, and current repository exploration. Its model output
must be evaluated with the same separate dimensions; a successful HTTP call or
valid JSON is not a quality result.

## M4 — Fresh local memory

Persist repository facts, derived claims, and investigation sessions separately.
Freshness includes repository identity, HEAD, dirty-file content, Go/gopls and
collector versions, build context, prompt version, and evaluator version.

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
