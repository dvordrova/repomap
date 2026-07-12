# Config-driven Go runtime-surface discovery

Status: bounded analyzer experiment with a production presentation projection
for persisted Go runs. The catalog remains separate from recommendation and
FlowProof completion claims.

## Current pipeline

`cmd/repomap` asks `internal/orient` to build a deterministic `snapshot`. The
snapshot uses tracked files plus `go list` facts to retain build-selected Go
packages, imports, exact `main` declarations, entrypoints, and bounded Cobra
syntax traces. `llmbundle` then selects a smaller allowlisted orientation
bundle. The configured model returns candidate runtime flows. Local validation
normalizes paths, attaches any matching Cobra `FlowProof`, caps confidence, and
writes report/debug artifacts. A selected direction gets a bounded local
file/test/package/import neighborhood; the browser can later authorize one
exact-symbol investigation through the versioned run manifest.

Model-proposed flow existence currently enters at the orientation response's
`candidate_flows`. `orient` converts those proposals to
`flowexplain.CandidateFlow`, and `report` converts them again to candidate
directions. Deterministic evidence constrains the proposals, but except for the
Cobra reader it does not first enumerate the runtime registration surface.

The existing deterministic runtime facts are:

- build-selected `package main` entrypoints and exact top-level `func main`;
- direct Go package import relations;
- Cobra root construction, `AddCommand`, `Run`/`RunE`, ranked handler calls,
  and directly visible handler concurrency;
- exact-symbol static callers/callees and bounded source/test evidence, only
  after a user selects a target.

## Reusable contracts and present losses

The new work can reuse `evidence.Location`, `evidence.Provenance`, certainty,
resolution, invocation, conditions, repository/build scenario identity, the
debug writer, stable report run directories, `flowexplain.FlowSeed`, and the
existing `FlowProof` slots/worklist. The gopls adapter remains a focused
exact-symbol provider and is not needed for the default discovery experiment.

Today `CandidateFlow` prose has confidence and paths but no typed registration
operation, receiver/dispatcher identity, wrapper call chain, invocation mode,
or terminal semantic seed. Conversion to `FlowSeed` further reduces evidence
to strings and files. Cobra's typed trace preserves more relation and callsite
information, but its current `FlowProof` assembler is CLI-specific. Build
scenario is represented in the proof seed, not in every candidate direction.
Model grouping in the component canvas is explicitly hypothetical; locally
assembled package imports are the only component edges.

## Experiment boundary

The first executable is `cmd/surface-discovery-playground`; its analyzer and
fixtures are isolated from orientation, reports, the browser, and `FlowProof`.
It loads build-selected Go syntax/type information with `go/packages`, builds
SSA, and performs bounded entrypoint-driven traversal. A small versioned
catalog declares exact terminal `net/http` registration and server-root
symbols plus argument/receiver projections. Go code—not catalog data—resolves
calls, substitutes abstract values through wrappers, evaluates the small set of
expressions required by fixtures, and records explicit frontiers.

The prototype emits deterministic `trigger_catalog.json`,
`surface_coverage.json`, `semantic_summaries.json`, and a Markdown summary. It
does not call a model or launch a browser. The default bound is intentionally
small and records every exhausted task/depth/target frontier.

The evaluated stack is deliberately incremental:

1. `go/packages` supplies build selection, syntax, imports, types, and type
   information.
2. `go/ssa` supplies resolved direct calls, receiver/argument values, closures,
   returns, fields, and simple expression structure.
3. class-hierarchy call targets are consulted only for interface invokes and
   remain ambiguous when more than one implementation is reachable.
4. repository-wide pointer analysis, VTA/RTA, `go/analysis` facts, and gopls are
   excluded until a fixture demonstrates that the smaller stack cannot retain
   a useful bounded result.

Cross-package summaries are ordinary revision/scenario-specific SSA summaries;
the experiment does not make `go/analysis` facts central without evidence that
separate compilation or cache reuse needs them.

## Contracts

The catalog owns only terminal meaning, exact symbol identity, projections,
optional constants/options, applicability, and fixture/reference notes. It
never contains wrapper names or traversal rules. Origins remain distinct:
`catalog_static`, `wrapper_static`, and the reserved future
`user_declared_semantics`.

A semantic summary names one function, its terminal effect, value projections,
wrapper path, scenario, provenance, source dependency hashes, and bounded
frontier. A `TriggerRecord` contains a stable hash-derived local ID, kind,
identity, transport, entrypoint, dispatcher, registration and server-start
sites, handler, middleware, wrapper chain, terminal seed, basis, certainty,
resolution, scenario, evidence, provenance, status, and dynamic frontier.
Stable IDs exclude display names and output order.

`SurfaceCoverage` describes only registrations found through configured seeds
and bounded propagation under its recorded scenario. It records inspected
packages/functions, matched seeds, entrypoints, dispatch roots, direct versus
wrapper-derived results, unresolved values, skipped inputs, budgets, latency,
and cache reuse. It never claims complete runtime enumeration.

## Incremental product connection

The migration keeps three separate collections:

1. discovered surfaces (`TriggerCatalog`), determined locally;
2. recommended flows, selected locally and optionally named/grouped by a model;
3. expanded flows, selected by the user and investigated with `FlowProof`.

Phase A runs only the playground. Phase B writes the catalog and coverage next
to an otherwise normal report run. Phase C exposes a compact summary while
leaving existing recommendations unchanged. Phase D gives the model only
bounded trigger data and opaque IDs; its response may name, group, and rank
those IDs but cannot create triggers or edges. Phase E adapts one selected HTTP
record into the existing flow seed with only trigger, entrypoint/dispatch,
registration, handler, and evidence populated. Core operation, I/O,
concurrency, termination, and confidence stay unknown for the existing
worklist to investigate. Model-created flow existence remains a fallback until
compatibility fixtures justify retirement.

Catalog version, analyzer version, build scenario, repository revision/dirty
digest, and source dependency hashes participate in freshness. Saved model
replays remain separate from deterministic surface artifacts and must reference
only supplied trigger IDs.

## Non-goals and deferred work

- no full-program pointer analysis, unbounded call graph, universal abstract
  interpreter, graph database, or default gopls survey;
- no claim that registration proves handler execution or middleware order at
  runtime;
- no forced unique result for interfaces, function maps, reflection, generated
  registrations, or configuration-driven loops;
- no generic custom-router state-correlation detector in the first integration;
- no browser/canvas redesign and no eager deep investigation of every trigger;
- no simultaneous CLI, gRPC, worker, cron, controller, or Python support;
- no repository-local semantic hints until unavailable private-library source
  demonstrates the need;
- no removal of model-created candidate flows in this work.

Advanced string evaluation, reflection, plugins, generic `ServeHTTP` registry
inference, complete cross-revision matching, and additional framework seeds are
tracked as explicit frontiers rather than approximated with invented precision.

## Experiment verdict (2026-07-12)

The core hypothesis passed the bounded fixtures. One exact terminal catalog
entry is sufficient to derive both a library convenience wrapper and an
application wrapper: the Gin fixture resolves
`registerJSON → (*RouterGroup).GET → (*RouterGroup).Handle` and emits a concrete
`GET /runs` trigger. The propagation engine contains zero Gin/Echo/Chi symbol
branches. Gin-specific production input is one 15-line JSON file containing one
seed. The same engine resolves direct `net/http`, one/multiple repository
wrappers, cross-package wrappers, returned muxes, path concatenation, method
values, simple middleware wrapping, one interface implementation, and explicit
multi-implementation ambiguity.

Fixture results are:

| Fixture | Result |
| --- | --- |
| direct `net/http` | 1 direct trigger, exact path/handler/dispatcher/start |
| nested repository wrappers | 1 wrapper-derived trigger, three wrapper frames |
| cross-package wrapper | 1 wrapper-derived trigger with source hash provenance |
| value projection | `/v1` + `/runs` reconstructed; handler and middleware separate |
| interface dispatch | one implementation exact; multiple implementations ambiguous |
| dynamic loop/map | mechanism retained; path/handler remain dynamic frontiers |
| recursive wrapper | useful route retained; cycle frontier stops recursion |
| negative controls | 0 triggers |
| Gin terminal seed | 1 wrapper-derived `GET /runs`; `GET` and `registerJSON` inferred |
| custom registry stretch | 0 triggers; state write/read/callback correlation unsupported |

No fixture produced an obvious false positive. Dynamic registration produces a
provisional stable ID and never invents a route or handler. Registration remains
distinct from callback execution, and middleware remains a separate field.

The selected stack is `go/packages` + `go/types` + `go/ssa`. Class-hierarchy
targets are consulted only at unresolved interface invokes and are bounded;
repository-wide pointer analysis, VTA/RTA, `go/analysis` facts, and gopls were
not required. Direct fixture runs measured 5.28 s and 5.88 s in two consecutive
standalone processes on the development machine. The second was not faster, so
coverage honestly records `cache_reuse: false` and no warm-latency claim. The
focused package test suite is slower because every fixture performs a fresh
build-selected package load; production cache design remains deferred.

The grouping experiment made zero live calls. Its 703-byte deterministic
bundle and 497-byte saved response produced one group and one recommendation;
local validation rejected a response containing an invented trigger ID. The
deterministic fallback produced the same counts. This proves the no-invention
contract and replay shape, not comparative model quality.

Product integration stops at the intended safe bridge. A normal persisted Go
run writes `trigger_catalog.json`,
`surface_coverage.json`, `semantic_summaries.json`, and `surface_summary.md`
beside the existing report artifacts without changing candidate
recommendations. `--discover-surfaces=false` remains an explicit opt-out while
latency is measured. The report renders a separate bounded surface shelf;
surface facts do not enter the architecture graph. `surfacebridge.FlowSeed`
maps only grounded trigger,
entrypoint, registration, handler, dispatcher, files, and evidence; it has no
way to populate core operation, I/O, concurrency, termination, or confidence.
The existing model-created path remains available.

Remaining limitations are variadic handler-slice recovery beyond the exercised
single-handler wrapper, nontrivial struct-field/return evaluation, closures with
captured mutable state, reflection, configuration-derived identities, generic
custom-registry inference, cache reuse, live model quality comparison, and
trigger-to-HTTP-`FlowProof` execution. Because SSA uses the Go type checker
compiled into repomap, a target module requiring a newer Go language version is
currently skipped with a bounded warning before package loading. The next
smallest semantic seed family is Cobra `Command.AddCommand`: repomap already
owns a deterministic Cobra trace and CLI `FlowProof`, making it the lowest-risk test of
whether the generic catalog can replace framework-specific existence logic
without losing the current command evidence.

## Loop and worker signal increment

The first worker-oriented extension keeps loops subordinate to a configured
terminal start. The built-in catalog now contains the exact
`golang.org/x/sync/errgroup.(*Group).Go(func() error)` operation, verified
against the locally installed `x/sync` v0.19.0 source. The catalog projects the
callback and owning group; it does not declare loop traversal or worker names.

SSA natural-loop detection records three bounded signal shapes:

- `registration_loop` when a configured registration sink occurs inside a
  control-flow cycle, indicating potentially dynamic cardinality;
- `channel_receive_loop` when an async callback loop contains a channel receive;
- `select_event_loop` when an async callback loop contains a `select`.

A loop alone never creates a trigger. `errgroup.Go(oneShot)` is retained as an
`async_task`; `errgroup.Go(runWorker)` becomes a `worker` only when `runWorker`
is resolved and has static loop evidence. The status
`confirmed_worker_registration` means the async callback registration and its
static event-loop shape are confirmed under the build scenario. It does not
claim that the callback executed, that the loop is infinite, that a select arm
ran, or that cancellation is correct.

The worker fixture produces one worker and one finite async task from the same
repository wrapper. The existing dynamic HTTP fixture now also emits one
`registration_loop` without inventing additional routes. The real opt-in CLI
path was replayed over the worker fixture and persisted the worker,
`channel_receive_loop`, finite async task, and coverage counts beside the normal
report artifacts.

Next worker increments should cover direct Go statements, `sync.WaitGroup.Go`,
and explicit cancellation/termination evidence. These should be separate
language/library start seeds; ordinary computational loops must remain
non-triggering noise.
