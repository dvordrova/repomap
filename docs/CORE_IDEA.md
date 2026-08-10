# repomap core idea

repomap helps understand unfamiliar local repositories by extracting
deterministic local facts and optionally asking an OpenAI-compatible model to
interpret them as structured orientation reports. Go currently has the deepest
package and symbol evidence. Python repositories share the bounded tracked-file
orientation path; Pyright remains an optional focused analyzer rather than a
default survey dependency. DeepSeek remains the reference provider and
calibration target; company-hosted compatible endpoints use the same bounded
request contract.

Product and research decisions that intentionally remain unresolved are tracked
in [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md). The concrete client package is still
named `deepseek`, but endpoint/model/auth/timeout configuration is provider-neutral.

The proposed shared investigation workflow is documented in
[INVESTIGATION_ENGINE.md](INVESTIGATION_ENGINE.md). Demonstrated implementation
gaps and experiment follow-ups are tracked in
[TECHNICAL_DEBT.md](TECHNICAL_DEBT.md).

For a current/planned module map and independently runnable challenge points,
read [SYSTEM_MAP.md](SYSTEM_MAP.md).

The ordered product outcomes and their observable completion conditions are in
[MILESTONES.md](MILESTONES.md). Exactly one milestone is active at a time.

## Pipeline

### 1. Deterministic local extraction

Run without a model or API key. The tracked-file survey is language-neutral.
When Go modules are present, Go package discovery deliberately respects the
engineer's normal Go environment and may use a configured company proxy:

- `git ls-files` for tracked file inventory
- README (truncated)
- top-level directory stats
- language hints by file extension
- interesting files (entrypoint-ish, storage-ish, messaging-ish, config-ish, background-ish)
- `go.mod` files per discovered module
- `go list -json ./...` per module
- build-selected non-test top-level declaration identities per local package;
  these stay local until the exact target-portfolio projection groups them
  under their owning package without source or comments
- Go package import edges (internal, external top-50)
- entrypoint detection (`go list` build-selected package files plus an exact
  syntax-only top-level `func main()` anchor)
- one framework-free core Go analyzer over build-selected package closures;
  it records exact process entries, standard-library HTTP registrations and
  server starts, `errgroup` task starts, and repository-local registrations
  with the exact `net/http` handler shape
- module summaries (role guess, top internal imports, top external imports)
- orientation candidates (ranked entrypoints plus bounded operational
  candidates derived from source signals, all with repo-relative `open_files`)
- known docs (Documentation/, docs/, architecture .md files)

### 2. Compact LLM bundle

Derived from deterministic facts, bounded by limits:

```json
{
  "repo_name": "...",
  "readme_excerpt": "...",
  "top_level_directory_stats": {...},
  "language_hints": [...],
  "go": {
    "modules_count": 0,
    "packages_count": 0,
    "module_summaries": [...],
    "entrypoints": [...],
    "orientation_candidates": [...],
    "important_edges": [...]
  },
  "known_docs": [...],
  "warnings": [...]
}
```

Must NOT include:
- full file_tree
- full repository contents
- secrets, env files, private keys
- full README
- raw internal_edges beyond limits

### 3. Model orientation

The configured provider receives only the compact facts bundle and returns a
JSON orientation report. Structured verified paths are normalized against the
bundle allowlist, and path-like mentions inside evidence prose cannot name an
unprovided file. If a provider abbreviates or invents a path inside free-form
evidence, that evidence item is removed with a parser warning instead of losing
the whole report. An ungrounded `likely_entrypoint` may fall back to the flow's
first already-allowed `likely_file`; structured file lists remain fail-closed.
Other prose remains an explicit model interpretation.

The report proposes **candidate runtime/event flows**, not folder summaries.
Request-driven and operational candidates remain one ranked list, distinguished
by `flow_type`. Operational candidates must originate in deterministic source
signals; weak static evidence is capped at 0.3 confidence and never presented
as observed execution. Offline runs retain these candidates with an explicit
`(offline hint)` suffix.
Every candidate flow must cite evidence from the bundle.
Confidence must be explicit, warnings for low confidence.
Provider confidence is only a proposal: a local gate caps each candidate using
exact entrypoint/dispatch evidence, unresolved proof slots, unverified targets,
and bounded-retrieval warnings.

### 3.5. Bounded local flow proof

A selected CLI direction can now acquire a separate, replayable `FlowProof`.
The CLI contract has eight slots: trigger, entrypoint, dispatch, application
callable, core operation, I/O boundary, concurrency, and termination. Exact
anchors and transitions carry independent relation, resolution, invocation,
certainty, and `file:line` evidence fields. A complete proof scopes confidence
for that one flow; it does not make the globally truncated repository map
complete and it is never described as an observed runtime trace.

The worklist keeps one deterministic next task. Its identity includes flow,
slot, target, depth, build scenario, and collector version. Duplicate and
no-progress work stops, as do explicit task/depth/symbol/file/model/wall-time
budgets. Current Go executors load only the package enclosing an already chosen
callsite, use type information to resolve the target, and inspect one bounded
goroutine lifecycle. Whole-repository SSA and VTA remain outside this path.

### 4. Atlas-first semantic products

The default run saves a complete canonical Repository Atlas locally. Exact
startup identities remain local Surface/Atlas evidence and enter the Map
through the Entrypoints projection; no provider selects one repository entry.

Architecture and Study are bounded task-shaped questions over this local base.
The Architecture provider returns one complete flat ordered record list:
response-local subsystem refs plus components that cite exact request-local
typed member and anchor refs. Model grouping is conceptual participation, not
ownership, and it cannot replace or mutate the canonical local canvas or its
relations.

Atlas Study receives a compact reading catalog. Each reading target includes
its exact repository-relative path, positive line and optional qualified
symbol as read-only locator context, and `allowed_paths` is exactly the sorted
set of those target paths. It receives no source bytes, full file contents or
raw graph. Identity fields select only short request-local typed refs. A Study
direction may repeat an exact locator only when one of its resolved reading
targets advertised it; that prose is never parsed as authority. Canonical
identities and exact validation remain local. A rejected or absent Architecture
enrichment therefore does not block Study when the validated local canvas and
reading catalog are otherwise usable.

Before target scoping, complete exact package facts remain local authority, but
product targets follow Go's module/executable boundaries rather than mirroring
every compiler package. The sealed catalog contains every exact build-selected
executable and at most one library surface per `go.mod`. A module library has no
synthetic root package: it scopes every exact owning-module non-main package
(including internal context) while its public roots are the package-qualified
exported APIs of completely scanned, externally importable packages. Main and
internal packages never become library API roots; an incomplete or zero-export
module library is omitted without discarding independently exact executables.
One refs-only Go call may choose the useful set and default when more than one
eligible product surface exists. Its request nests exact names-only declaration
labels under their package group, so executables and module APIs can be
distinguished without source or path-purpose guesses. The response can only cite
request-local target refs; the backend restores exact advertised targets and
publishes each selected scope as a sibling page. Exact packages remain local
evidence and drill-down material rather than automatic sibling Study pages.

Named user choice now reaches the first saved local neighborhood. In a served
report, one manifest-authorized component anchor can lazily request bounded Go
function/method candidates, confirm the selected declaration at its exact
`file:line:column`, and read a bounded source/static-call card through the
existing investigation runner. This local drill-down makes no provider call.
The architecture canvas groups exact local members into conceptual components
and subsystems. Names and grouping may be model-assisted hypotheses; membership
is validated locally against opaque candidate IDs. Quiet witnessed structural
relations remain separate from typed FlowProof overlays. Selecting one flow
keeps component positions stable, dims unrelated components, and preserves
main/task/shared branches, cancellation, joins, and unresolved frontiers.
Selecting an exact symbol renders the same evidence distinction again: a
compact incoming → target → outgoing static neighborhood, followed by bounded
source and the omitted frontier.
The accepted exact symbol and source card are checkpointed below that report
run and resume without the short-lived candidate cache. One explicit follow-up
can collect bounded direct `_test.go` references with local gopls only. Those
references are navigation evidence, not proof of coverage or assertions.

### 4.5. Local runtime surfaces (implemented for persisted Go runs)

One typed report catalog normalizes the framework-free core SSA discoveries
under one recorded build scenario: exact process entries, standard-library
`net/http` registrations and server starts, `errgroup` task starts, and
repository-local registrations whose handler is exactly
`func(net/http.ResponseWriter, *net/http.Request)`, `net/http.Handler`, or
`net/http.HandlerFunc`. Wrapper, frontier, and provenance evidence remains
attached to each record. Ordinary analysis has no framework mode, fresh Cobra
inventory, or third-party context-handler widening. `All surfaces`,
architecture ownership, and headline counts are projections of that same
catalog; a saved trace only associates with a record and never creates or
duplicates one.

This shelf is deliberately outside the architecture graph and FlowProof:
static registration does not prove callback execution, runtime order,
cancellation, or lifetime. Non-Go/no-debug/preview runs skip the artifact stage,
and a discovery failure leaves orientation usable with a precise warning.
Its counts are repository-wide across build-selected executables, with primary
application, secondary tooling, test/helper, and unknown roles kept distinct.
Generic-scan latency/counts remain stage metrics rather than product totals.
Worker and non-worker async-task totals are exclusive final classifications; a
selected flow may therefore contain a task that this independently bounded
catalog did not reach.

Persisted schemas retain the former Cobra/framework coverage and command-trace
fields so reports and snapshots from older runs remain readable. Fresh runs do
not populate those retired producers.

### 5. Durable focused investigation (implemented)

The exact-symbol investigation stores deterministic symbol/source/test facts,
model-derived source claims, and reducer/session state separately. Repository
identity, HEAD and dirty contents, Go/gopls/collector/build inputs, and
prompt/parser/evaluator versions are reconciled before a saved action becomes
executable. Unchanged sessions resume without a second model call; changed
facts re-resolve the symbol, while changed claim logic retains local source and
returns to assessment. The browser uses a deliberately smaller branch of this
same state machine: exact local symbol, bounded source, and target-only test
references are durable without creating model claims. Model source assessment
remains an optional later action rather than a prerequisite for saving a useful
leaf function.

## Experimental local evidence layer

The surface catalog above is the first bounded analyzer projection connected to
normal persisted Go runs. Other analyzers are still developed behind a
language-neutral evidence graph before they are connected to the product:

- every relation has a `certainty` (`possible`, `static`, `observed`, ...)
- every relation cites `provenance` (provider, version, operation, location)
- build/runtime conditions are explicit `scenarios`
- language-specific adapters implement the same `analyzer.Provider` port and
  emit the same graph

The former isolated Go/gopls playground and its example-fetch script were
retired because they were not part of the supported product path. The evidence
vocabulary remains available for a future explicitly approved analyzer
integration; no direct analyzer command is currently documented as a product
workflow.

The former focused symbol vertical remains covered by its package and command
tests; it is not a supported ordinary product entrypoint.

The raw local evidence graph is retained for debugging but is never sent.
DeepSeek receives only `symbol_bundle.json`; every report claim must cite its
evidence IDs, and the response is rejected if it invents paths, evidence,
caller/callee identities, observed runtime behavior, or test files.

The completed source-grounded contracts are independently replayable through
their Go fixtures and evaluator tests without a shell harness.

Source lines remain bounded lexical evidence, never a claim that the whole
function body was parsed. For validation-shaped calls, the local source cube can
lexically connect an assigned multiline call to an immediately following
`if err != nil` or returned `err == nil` comparison and exposes only the minimal
supporting source IDs. This uses Go token scanning, not AST semantic inference,
and still leaves callee behavior and runtime reachability unknown. A weak model
that cites only the call anchor is reduced to `ambiguous` with a warning rather
than being allowed to manufacture or inherit the missing proof. Related
`_test.go` locations are `test_reference` navigation evidence with gopls
provenance; they are not `test_supported` until their bounded test source is
assessed.

When a callee name offers no semantic hint, the same bounded scanner can seed
three syntax-only questions: an immediately checked call result, a direct return
of a call, or a standalone call under a locally visible `case`/`if`/`else`
branch. Those predicates deliberately say nothing about callee behavior. Their
locally reconstructed claims require the complete minimal proof (for example,
both the branch line and call anchor), and runtime branch selection remains an
explicit unknown.

## Non-goals for now

- no repository-wide AST semantic analysis; the deterministic survey uses only
  a bounded syntax-only parse to confirm top-level `func main()` declarations
- no gopls in the default repository-wide survey; it remains a lazy focused
  investigation adapter
- no long-lived LSP client yet; the playground uses the experimental gopls CLI
- no embeddings yet
- no repository-wide package dependency dump; the bounded architecture canvas
  uses locally validated conceptual membership, component-specific structural
  witnesses, and one selected saved FlowProof overlay, then expands exact
  evidence lazily
- no automatic huge repo upload
- no autonomous code modification

These are present scope boundaries, not permanent answers to the questions in
[OPEN_QUESTIONS.md](OPEN_QUESTIONS.md).

## Good vs bad output

Good output (runtime/event-oriented flows):
- "client gRPC Put request"
- "etcd server startup"
- "watch stream"
- "lease lifecycle"
- "raft replication/write path"
- "etcdctl command execution"

Bad output (folder-oriented):
- "server module"
- "client folder"
- "pkg package"
