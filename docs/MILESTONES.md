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

DeepSeek through its OpenAI-compatible API is the default interpretation and
product-quality target. Existing Qwen 1.5B experiments remain useful research,
but local-model work is not a completion gate for the current milestones. A
future `--really-dumb-model` profile may select alternate implementations of the
same typed capabilities without changing their inputs or outputs.

## Status

| Milestone | Status | User-visible outcome |
| --- | --- | --- |
| M1. Source-grounded symbol | **complete** | one selected symbol is explained from exact source evidence and connected to relevant tests |
| M2. Investigation loop | **active** | repository, flow, symbol, source, tests, claims, unknowns, and next actions use one resumable state machine |
| M3. Quality suite | planned | the same golden tasks expose orientation and drill-down regressions on five large Go repositories |
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

## M3 — Quality suite

Maintain small golden tasks for etcd, Grafana k6, Prometheus, NATS Server, and
golangci-lint. Measure useful direction selection, grounded citations, omitted
important evidence, context size, latency, and semantic usefulness separately
from JSON/contract adherence.

## M4 — Fresh local memory

Persist repository facts, derived claims, and investigation sessions separately.
Freshness includes repository identity, HEAD, dirty-file content, Go/gopls and
collector versions, build context, prompt version, and evaluator version.

## M5 — Browser journey

Open the current repository by default, show deterministic analysis progress and
the exact bounded data sent externally, reveal one investigation branch at a
time, retain alternative directions, and open a selected source location in the
editor. The browser consumes saved application state; it does not own analysis.

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
default profile wires DeepSeek implementations. A later dumb-model profile may
use multiple smaller prompts, deterministic reducers, or skip unsupported
capabilities, but it must return the same validated capability output. Do not
put model-name conditionals inside evidence, investigation, or presentation
code.
