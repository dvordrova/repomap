# Deeper repository research

Status: product and architecture experiment. This document is a hypothesis,
not a claim that repomap already understands repositories like a senior who
built them.

The first onboarding report answers **where might I start?** Deeper research
must answer a narrower question with stronger evidence, retain what was learned,
and expose why the next step is worth taking. More model calls are useful only
when each call reduces a named unknown; request count is not a product metric.

## Working model of human research

A developer does not read a repository linearly. The smallest useful loop is:

```text
frame the goal
  -> inspect the current landscape
  -> choose one question
  -> form a bounded hypothesis
  -> collect a thin local evidence slice
  -> compare independent evidence
  -> explain claims and unknowns
  -> update the map and choose the next question, or stop
```

The input report in `/Users/dvordrova/Downloads/deep-research-report.md` is a
useful catalog of heuristics, but its fixed reading orders, percentages, file
counts, and tool rankings are not repomap quality evidence. Several citations
are not independently recoverable from the document, and its example shell
request does not actually attach the collected study artifacts. Treat those
items as hypotheses to test.

## What a senior-style explanation should contain

For one selected component or scenario, a useful explanation should make these
ideas navigable:

1. why it exists and who depends on it;
2. its external and internal contracts;
3. one representative trigger-to-effect lifecycle;
4. state ownership and important invariants;
5. failure convergence and observability;
6. design tradeoffs or surprising constraints;
7. exact source and test anchors for independent reading;
8. explicit unknowns that the collected evidence cannot answer.

A package graph alone cannot represent this. It is one input to the
explanation, not the explanation itself.

## Shared research moves and task lenses

The underlying moves should remain small and local:

- inspect a component, direction, file, exact symbol, or relation;
- list exact symbols in one selected file/package;
- expand direct callers, callees, implementations, or references;
- read a bounded source or document slice;
- find tests, examples, configuration, or observable signals;
- compare source, static, test, documentation, and runtime evidence;
- synthesize claims, contradictions, unknowns, and the next bounded move.

Different user tasks rank those moves differently. They do not initially need
separate engines or response schemas.

| Lens | Research path | Useful stopping condition |
| --- | --- | --- |
| Onboarding | purpose -> components -> representative lifecycle -> boundaries -> failures/tests | the user can explain one important scenario and knows where to continue |
| Configuration | schema/default -> file/env/CLI precedence -> validation -> consumer/reload | one knob is traced from source to runtime consumer |
| Extension | interface/registry/factory -> implementations -> construction/lifecycle -> analog/test | exact extension seam and minimal safe change surface are known |
| Testing | behavior -> harness/setup -> fixture -> assertion -> CI/build constraints | what a test proves, omits, and how to run it are explicit |
| Debugging | symptom -> emission/guard -> probable path -> state/dependency -> discriminating experiment | a reproduction or next experiment exists without inventing a root cause |
| Logs | emitter/condition -> fields/context -> callers -> metric/trace/runbook | the signal's meaning and blind spots are understood |
| Integration | boundary/contract -> adapter -> auth/retry/timeout -> lifecycle -> mock/integration test | failure semantics and a testable boundary are explicit |

## Layered research pipeline

The useful boundary is not "one call or many calls." Each layer must reduce one
named unknown and leave a challengeable artifact:

1. **Orientation** proposes a landscape and first questions from tier-0 facts.
2. **Component planner** selects a primary question and a few opaque local
   evidence handles. It proposes where to look; it does not teach yet.
3. **Bounded local probe** resolves the selected exact symbols and collects
   direct static relations, source anchors, call sites, and test references.
   This stage makes no model call.
4. **Readiness gate** reports `connected`, `frontier`, or `blocked`. A frontier
   may expose one new opaque symbol for one more bounded probe round.
5. **Focused teacher** explains only the evidence actually reached, with claims
   and unknowns linked to local IDs. After two probe rounds it teaches a partial
   result instead of crawling further.
6. **Chapter synthesis**, if experiments justify it, compacts several accepted
   steps into a resumable learning trail.

Orientation is the current product path; component planning is now a working
isolated experiment. The planner contract is prompt v3 / Plan v2: it returns
opaque IDs and a `primary_question_id`, while paths, symbols, certainty, and
provenance remain locally owned. Its tolerant parser can repair common JSON
drift, drop unknown IDs with diagnostics, and replay a saved raw response with
zero API calls. The probe and teacher remain experiments, not a reason to grow
an agent framework.

## Evidence ladder

Every explanation item keeps its basis visible:

1. **orientation hypothesis** — model prose grounded only to supplied survey
   facts;
2. **navigation/static** — exact file, symbol, import, reference, or call under
   a named build configuration;
3. **source-supported** — a bounded source slice directly shows the statement;
4. **test-supported** — bounded test source exercises or asserts the statement;
5. **runtime-observed** — a named scenario produced the observation.

Test references are navigation, not test support. Static calls are not runtime
order. Missing evidence becomes an unknown, not a higher confidence score.

## Visual projections

The component canvas remains the landscape. Deeper views should be projections
of the same stored research trail rather than unrelated generated diagrams:

- a trigger-to-effect lifecycle or sequence;
- a component story with contracts, invariants, failures, and anchors;
- a bounded symbol neighborhood;
- a configuration or state-propagation path;
- an extension/interface-to-implementation map;
- a symptom/log-to-emitter investigation path;
- an evidence ladder for one claim;
- a learning trail showing answered questions, unknowns, and a small frontier.

This means repomap visualizes both the system and the process of learning the
system.

The useful GitDiagram lesson is the semantic intermediate graph and progressive
disclosure, not a large model-authored poster. The first bounded adoption is:

- model-proposed component roles create stable landscape lanes and remain
  visibly hypothetical;
- only locally assembled direct package imports become component arrows;
- selecting a component focuses its incoming/outgoing static neighborhood and
  exposes the exact package-edge evidence;
- selecting an exact symbol shows a compact static neighborhood before the
  longer source/call details;
- planner, probe, frontier, and teacher artifacts are adapted into
  `researchtrail.Trail`, while all `file:line` locators and origins stay in a
  separate `researchtrail.LocalIndex`.

The adapter is deliberately presentation-neutral. It knows no HTML, colors,
layout, filesystem, provider, or repository I/O. Its first offline gate is:

```bash
make research-trail-replay
```

The saved Pebble chain currently composes into 79 nodes, 59 typed evidence
edges, four ordered research steps, three stage transitions, and 60 local
locators without a model or gopls call. The trail is bound to the exact
repository state and onboarding report SHA-256, so it cannot silently attach to
an unrelated run. Browser integration comes after this contract can express the
planner hypothesis, the accepted frontier correction, the grounded teaching
claims, and the remaining next dive.

## Local cost policy

Do not optimize the observed 48.5-second Soft Serve survey before measuring
what repeats. The intended tiers are:

```text
tier 0  saved git/module/package/docs/entrypoint survey
tier 1  selected component/package/file candidates
tier 2  first exact-symbol probe with bounded source/test/navigation evidence
tier 3  one accepted frontier hop and a second bounded probe
tier 4  named tests or runtime observations requested for one question
```

Record cold/warm collector latency, entity/relation counts, selected/omitted
bytes, and cache reuse. A selection trace is more valuable now than a new
database or eager repository-wide gopls index.

## First experiment: Soft Serve startup tour

Use the clean saved run
`~/Library/Caches/repomap/runs/20260711-011750-soft-serve` and ask:

> After `soft serve`, how are configuration and backend initialized, which
> long-running services start, how do failures converge, and how is shutdown
> performed?

This is a useful falsifiable case because the current orientation associates
startup with `cmd/soft/serve/serve.go` but omits the central
`cmd/soft/serve/server.go` lifecycle from the candidate directions. Direct
source inspection shows that `NewServer`, `(*Server).Start`, and
`(*Server).Shutdown` live there; the component-planning experiment must discover
that package-local frontier without hard-coding those symbols.

### Step 1: component planner

Input:

- one selected canvas component and question;
- its verified anchor IDs and related direction IDs;
- build-selected files from only the anchor packages;
- bounded document-symbol candidates from those files;
- explicit package/component relations already available locally.

The model selects at most two file IDs and three symbol IDs and returns two to
four questions. It never returns raw paths, symbols, commands, or collector
arguments. Invalid IDs are dropped with parser diagnostics; surviving entries
remain usable.

Initial bounds are experiment ceilings, not optimized targets:

- planner bundle: 12 KiB;
- 16 candidate files;
- 16 candidate symbols, at most four per file;
- two selected files and three selected symbols.

### What the planner experiment taught us

On Soft Serve, the bounded planner selected `serve.go`, `server.go`, and the
`NewServer`, `Start`, and `Shutdown` lifecycle symbols. The improved prompt
covered initialization, concurrent service startup, and shutdown without
hard-coding those names. Exact-symbol inspection then found the synthesized
startup caller and direct outgoing service starts, so this is a plausible
connected teaching case.

On Pebble Batch.Commit, the planner selected `batch.go`, `commit.go`,
`Batch.Commit`, `commitPipeline.Commit`, and `directWrite`. That looked like a
reasonable commit story, but exact-symbol inspection showed that
`Batch.Commit` directly calls `DB.Apply`; it does not directly enter
`commitPipeline.Commit`. The planner therefore found useful territory but not a
connected explanation. This falsification is the reason teacher cannot follow
planner directly: `DB.Apply` must become the single bounded frontier hop.

The historical transport experiment also rejected an artificial small response
budget. Pebble returned empty content with `max_tokens: 1600` and succeeded with
the then-configured 6000-token ceiling. The current global default is 64,000 and
every stage uses its exact configured value instead of imposing a per-cube cap.

### Step 2: bounded local probe

Start from the primary question and selected exact symbols. Collect only direct
call relations, bounded source cards, selected-file/call-site windows, and test
references. If the evidence is disconnected, return a small opaque frontier;
the user or deterministic policy may accept at most one frontier symbol. Run at
most two probe rounds total.

The saved trail should make the correction visible:

```text
planner hypothesis -> local evidence -> gap/frontier -> corrected evidence
```

### Step 3: focused teacher (only after the readiness gate)

Ask for a visualization-ready chapter from the reached evidence only:

```text
mental model
lifecycle steps with evidence IDs
boundaries with evidence IDs
patterns and tradeoffs with evidence IDs
tests and checks
unknowns
next dive
```

`connected` unlocks a normal chapter. Exhausting the two-round budget unlocks a
partial chapter that names the missing link. `blocked` produces no invented
story. Request count remains an implementation detail, not a quality signal.

### What the local probe experiment taught us

The first real probes support a narrow on-demand index instead of an eager
repository-wide one:

- Soft Serve probed only `NewServer`. Exact symbol, source, references, and the
  external caller window took about two seconds. Removing callsite windows
  already covered by the selected function reduced the saved dossier from
  roughly 50 KiB to 36 KiB without losing new evidence.
- Pebble probed three planner-selected symbols in about 14-16 seconds. The
  first round recovered `Batch.Commit -> DB.Apply`, the production caller
  `applyInternal -> commitPipeline.Commit`, and the separate
  `handleIngestAsFlushable -> directWrite` path. The result stayed `frontier`
  instead of turning those disconnected facts into a fluent explanation.
- One accepted opaque frontier (`applyInternal`) produced a SHA-256-bound
  second round in about 3-6 seconds. Its exact source and calls close the static
  bridge `DB.Apply -> applyInternal -> commitPipeline.Commit`, expose validation
  and fatal commit-error handling, and still mark the result as static rather
  than runtime proof.

The two probe rounds make zero provider requests. Round two accepts no raw path
or symbol from a client, is bound to the exact round-one bytes, and cannot be
expanded again. The raw debug dossiers are intentionally richer than a model
request; a separate teacher compactor must deduplicate and select evidence
under its own byte budget.

### What the focused-teacher experiment taught us

The Pebble chain was compacted from about 207 KiB of local probe JSON into a
path-free 27-28 KiB teacher bundle. A separate local index retained every
clickable `file:line` locator. The historical request used JSON mode,
temperature zero, the then-configured 6000-token budget, and one logical
provider call. Current requests use the exact global ceiling (64,000 by
default).

The first teacher version recovered the correct bridge and separated the
ingest `directWrite` lead, but incorrectly claimed that supplied
`commitPipeline.Commit` internals were missing. Two local data bugs caused the
mistake: a small function was split into three indistinguishable source chunks,
and the latest frontier still pointed back to an already probed parent symbol.
After keeping ordinary functions in one bounded slice and filtering every
already-probed frontier, the second response correctly described:

```text
Batch.Commit -> Apply -> applyInternal -> commitPipeline.Commit
                                      -> prepare -> memtable apply -> publish
```

It also retained fatal commit-error handling and test names as navigation only.
One weak-model failure remained: the response used absolute "does not" and
"only used" language from a bounded static caller relation. Repeating the same
request would not strengthen the evidence, so the tolerant local parser now
drops only that closed-world item, emits `claim.closed_world_dropped`, and keeps
the eleven grounded siblings. This is the intended weak-model policy: repair
shape drift, preserve useful evidence-linked items, and reject semantic claims
that exceed their support basis.

The product-relevant model-call profile is therefore layered, not chatty:
orientation for the landscape, component planning for one question, local
probe rounds with no provider, and one focused teacher call for the accepted
evidence. The extra teacher call above was an A/B prompt experiment, not a
required production stage.

### Debug artifacts

Each cube must be challengeable without the browser:

```text
case.json
seed.json
package_scope.json
symbol_catalog.json
selection_trace.json
planner/bundle.json
planner/request.redacted.json
planner/response.raw.txt
planner/plan.json
planner/parse_warnings.json
planner/evaluation.json
probe/bundle.json
probe/selection_trace.json
probe/readiness.json
metrics.json
compare.md
```

Later teacher artifacts live beside, not inside, planner artifacts. Requests
contain no credentials or authorization headers.

### Success criteria

The deeper slice succeeds when:

- it selects evidence leading to construction, concurrent start/failure
  convergence, and shutdown without hard-coded Soft Serve filenames/symbols;
- every selection is an opaque ID present in the bounded bundle;
- invalid model fields degrade to warnings while valid selections survive;
- selection trace explains every included and omitted candidate;
- a disconnected static path becomes an explicit bounded frontier rather than
  a fluent model explanation;
- one frontier hop and two probe rounds are hard ceilings;
- local and external bytes/latency are recorded;
- the same cube can be challenged on a Pebble component without changing its
  schema.

Do not add browser UI, persistent session actions, a collector registry, a
generic playbook framework, or a graph database during this experiment.
