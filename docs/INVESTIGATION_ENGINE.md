# Investigation engine

repomap product modes should share one progressive evidence loop. Explore,
symbol, ticket, bug, onboarding, and impact analysis differ in their starting
focus, ranking policy, allowed actions, and stopping condition; they should not
be implemented as unrelated prompt pipelines.

## State loop

```text
goal captured
  -> repository indexed
  -> focus selected
  -> questions planned
  -> missing evidence selected
  -> evidence collected
  -> claims synthesized
  -> claims assessed
  -> result / user choice / another evidence iteration
```

Repository changes, budget exhaustion, cancellation, contradiction, and user
redirection are explicit events. A completed investigation retains its evidence,
claims, unknowns, and chosen path so later exploration does not start over.

## Core session data

```go
type Session struct {
	Goal      Goal
	Revision  string
	Focus     Focus
	Questions []Question
	Evidence  []Evidence
	Claims    []Claim
	Unknowns  []Question
	Next      []Action
	Budget    Budget
	State     State
}
```

These are domain records, not LLM response shapes. Every claim references stored
evidence; every action explains which question or uncertainty it should reduce.

## Local index and repository memory

The local index should be the center of the system; an LLM is one consumer of a
selected evidence slice. Keep three stores logically separate:

1. **Fact index** — deterministic files, modules, packages, symbols, relations,
   tests, docs, scenarios, certainty, and provenance.
2. **Claim index** — model- or rule-derived interpretations keyed by their
   evidence, model, prompt version, and evaluator version.
3. **Session index** — the user's goal, path through the repository, verified and
   rejected claims, unknowns, frontier, and next actions.

Index progressively rather than computing every function relationship eagerly:

```text
tier 0: git inventory, modules, docs, entrypoints, source signals
tier 1: symbols, imports, interfaces, selected references and call edges
tier 2: source facts and tests for the current focus
tier 3: named test/runtime observations for a concrete investigation
```

Freshness must include repository content, dirty files, Go/gopls versions, build
context, and index schema. Changing one file invalidates dependent evidence and
claims, not the whole repository.

The first persistence format may be versioned JSON shards under ignored local
storage. Do not add a database until real index size and query patterns justify
one.

## Adaptive context assembly

Do not send the complete index or conversation. Build each model request from:

```text
goal
short ancestor/repository capsule
current focus evidence
verified claims and concrete unknowns
allowed next actions
output contract
```

Use lossless reduction first: canonical entity IDs, shared scenario/provenance
headers, deduplication, and evidence IDs instead of repeated paths. Lossy
summaries are allowed only when they retain links to underlying evidence and can
be expanded or invalidated.

Graph depth alone is not a sufficient budget because one hub may have hundreds
of edges. Context policy should combine:

```go
type ExpansionBudget struct {
	MaxDepth       int
	BeamWidth      int
	MaxEntities    int
	MaxRelations   int
	MaxSourceBytes int
	MaxModelTokens int
}
```

Rank frontier nodes by goal relevance, uncertainty reduction, architecture
boundary, state/I/O behavior, test availability, and evidence for an important
claim. Penalize generated/external code, duplicates, and expansion cost. If a
compact request leaves an important claim unsupported, expand one branch and
try another bounded round instead of falling back immediately to the whole repo.

Prior art to reuse rather than recreate:

- Aider's graph-ranked repository map and active token budget;
- OpenCode's iterative tool/LSP loop and session compaction;
- Sourcegraph/SCIP's precise persistent navigation index;
- content-hash-based incremental indexing used by code search products.

repomap's intended specialization is Go-aware evidence selection for weak/local
models and human investigation, not a new universal parser or coding agent.

## Actions

Initial action vocabulary:

- index repository;
- resolve focus;
- expand static calls;
- read a selected function;
- find related tests and documentation;
- inspect a dependency boundary;
- ask the model to interpret bounded evidence;
- run a named test or collect a named runtime observation;
- ask the user to choose a direction;
- open a file or finish the investigation.

Read-only deterministic actions may run automatically within budget. Source
execution or materially broader actions require their own policy and provenance.

## Claims and evidence levels

Claims remain distinct from facts:

1. navigation hypothesis — names, paths, signatures, graph neighborhood;
2. source-supported — bounded source facts corroborate the statement;
3. test-supported — a relevant test exercises or asserts it;
4. runtime-observed — a named scenario produced an observation;
5. verified for a scenario — appropriate evidence agrees for the stated build
   and runtime conditions.

Static does not mean runtime. The model may propose claims and next queries but
cannot create evidence or promote its own inference to a higher evidence level.

## Function understanding capability

Function understanding is a reusable expansion step, not a whole-repository
precomputation. Its result should contain:

```go
type SymbolUnderstanding struct {
	Target          EntityRef
	StructuralFacts []Evidence
	SourceFacts     []Evidence
	Claims          []Claim
	Unknowns        []Question
	RelatedTests    []EntityRef
	NextActions     []Action
}
```

The planner requests source evidence when an important claim depends only on a
name/static edge, when a goal needs behavioral detail, or when contradictory
evidence must be resolved. It should not expand trivial, generated, external, or
unrelated functions by default.

## Playbooks

| Playbook | Initial focus | Stop condition |
| --- | --- | --- |
| Explore | repository | components, entrypoints, and candidate flows are navigable |
| Symbol | symbol | important claims are supported or explicitly unknown |
| Ticket | issue text | change surface, analogs, risks, and test plan are identified |
| Bug | symptom, test, stack, or log | reproduction or discriminating experiment exists |
| Onboarding | standard question set | evidence-backed curriculum is available |
| Impact | diff or symbol | affected dependants, flows, and tests are identified |

A playbook supplies initial questions, action weights, collectors, budgets, and
stop criteria. It does not own a separate evidence model.

## Minimal Go architecture

Start with one small `internal/investigation` package:

```text
session.go   domain records
event.go     facts delivered to the state machine
action.go    requested side effects
reducer.go   pure state transitions
policy.go    ranking, budget, and stop conditions
claim.go     support assessment
```

The central operation should be a pure, table-testable reducer:

```go
func Reduce(session Session, event Event) (Session, []Action, error)
```

Action runners consume small interfaces defined by the investigation package.
Existing concrete packages remain adapters:

```text
snapshot/gofacts/sourcesignals -> repository indexing
analyzer/gopls                -> focus resolution and static graph
evidence                      -> fact/provenance representation
symbol                        -> bounded symbol evidence and report normalization
deepseek                      -> current model adapter
report                        -> presentation
orient                        -> current workflow to migrate incrementally
```

Do not introduce a DI framework, collector registry, or interface per package
before the first current symbol flow is running through this model.

## First vertical slice

```text
goal: understand kvServer.Put
  -> resolve symbol
  -> collect depth-one graph
  -> synthesize initial claims
  -> detect unsupported “validates request” claim
  -> request bounded source evidence
  -> reassess claim
  -> rank server.Put and relevant tests as next actions
  -> stop or wait for user
```

This slice should preserve current bundle limits and validators. Ticket and bug
playbooks come after the loop is proven with the existing symbol workflow.
