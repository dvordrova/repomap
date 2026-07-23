# Decision: Guided onboarding story experiment

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## User-visible goal

Add one optional `Start here` guided tour inside the existing Architecture
experience. The tour selects and explains one useful repository behavior from
already saved orientation, component, surface, FlowProof, and evidence
artifacts. It highlights existing canvas components and keeps the full map,
saved traces, exact evidence, and unresolved frontiers available.

## Approved scope

1. Build one bounded story bundle only after existing report coherence has
   joined components, surfaces, traces, suggestions, and evidence by exact IDs.
2. Compare two optional editorial strategies over the same bounded local
   bundle: one monolithic story editor, and a fan-out/fan-in strategy. Fan-out
   leaf tasks cover independently bounded components, flows, or mechanisms;
   every leaf is grounded directly in local evidence and returns a structured,
   locally validated artifact containing only supplied opaque IDs. Fan-in sees
   those validated artifacts plus a compact local fact index, not a sequential
   chain of model summaries.
3. Validate and cache the editorial response by repository context, model,
   prompt version, policy version, and canonical bundle hash. Invalid, stale,
   unavailable, or failed model output leaves the existing report usable.
4. Keep the guided mode inside the current architecture canvas and inspector.
   Add highlighting and step navigation; do not add another renderer, graph,
   top-level architecture model, or saved-trace representation.
5. Treat the previous four-call ceiling as a compatibility constraint for old
   analysis runs, not as a product objective. Bound each strategy by aggregate
   request bytes, output size, task count, and cache identity; measure actual
   tokens and wall time. Permit multiple guided-tour calls when the bounded
   fan-out plan requires them, while preserving replay compatibility with old
   saved runs.
6. Reuse an already saved runtime-surface result when generating the experiment
   report. Guided onboarding must not require surface discovery to run again.
7. Diagnose slow surface discovery with saved artifacts, code inspection, and
   phase-specific timing/progress. Do not make the guided experiment depend on
   a performance fix or a fresh long-running fixture analysis.
8. Generate and inspect one report for repomap itself, then leave an ignored,
   uncommitted findings file alongside that run.
9. Run a small same-model comparison between monolithic and fan-out/fan-in on
   the same saved self-run. Persist per-strategy token, latency, cache, and
   locally derived coverage metrics, then record which explanation is more
   complete and useful at a comparable token scale.
10. Tune the experiment for DeepSeek V4 Flash rather than treating it as a
    scarce global-synthesis-only resource. Use thinking/high for independently
    useful leaf semantics and max only for global story planning/coverage.
    Keep a stable common prompt prefix with only a bounded task suffix, record
    provider-reported prompt-cache hit/miss tokens, and do not rely on
    temperature for thinking-mode determinism.
11. Do not ask a leaf to decide whether the complete mechanism is supported.
    Each leaf instead returns bounded, directly supported atomic observations,
    explicitly tentative connections that still need combination, and missing
    evidence. These partial artifacts are cached and visible to fan-in. Run
    fan-in whenever at least one locally valid leaf contains a usable
    observation or meaningful missing-evidence report; tentative connections
    accompanying observations remain visible for combination but never count
    as independent support;
    only fan-in may classify the combined story as `supported`, `mixed`, or
    `insufficient_evidence`. Compare unsupported claims as well as completeness,
    tokens, wall time, and cacheability.

## Constraints

- The model cannot create files, symbols, flows, relations, surfaces,
  transitions, evidence, certainty, or runtime claims.
- Story step order over a saved trace must preserve the saved trace order.
  Other stories are labeled as editorial reading order, never runtime order.
- Every rendered navigation or evidence reference is derived locally from an
  opaque ID present in the canonical story bundle.
- Leaf tasks never create a repository reference in an accepted artifact.
  Any path-like token in raw leaf prose is removed by a deterministic local
  reducer before validation and persistence. Final story prose remains
  path-free. Leaf prose may be used by fan-in only
  alongside the original supplied local IDs/facts, and every leaf artifact is
  independently replayable and cached.
- Each leaf is based directly on its projected local facts, never another
  model-generated summary. Fan-in may use leaf prose editorially, but the
  independent local fact index remains authoritative.
- A leaf observation must cite an exact subset of the leaf's supplied opaque
  support IDs. Candidate connections remain explicitly `needs_combination` and
  cannot independently authorize a final story. A final story may reference
  only beats covered by validated atomic observations; missing-evidence-only
  leaves can still drive an `insufficient_evidence` fan-in result.
- Each final story step names the exact validated leaf observations used for
  that step. Those observation references must belong to the selected
  candidate and cover the step's beat IDs; unrelated leaf prose is never an
  authorization source.
- The fan-in wrapper explanation is diagnostic only and is never rendered or
  used to authorize a beat. Behavioral prose checks apply to the actual story
  proposal; wrapper prose still receives ordinary shape and path validation.
- No repository-wide call graph, package load, SSA build, or source scan is
  added for the story stage.
- Architecture remains the primary context. Historical decision 070's removal
  of a separate `Guided flows` product remains intact; this experiment is a
  small guided mode within Architecture.
- Surface discovery optimization is separate scope. This decision permits
  diagnostics and bounded work avoidance only when independently verified.

## Acceptance

The experiment is complete when both monolithic and fan-out/fan-in stories can
be cached, replayed, and compared on the same bounded self-run; the selected
locally validated story powers `Start here`; related components and exact
evidence remain inspectable; gaps are explicit; Full map restores normal
navigation; partial leaf failure or rejection degrades without breaking the old
report; no new repository analysis feeds the story; surface progress names
concrete phases; repository checks pass; and the comparison, self-report, and
uncommitted product findings are available for inspection.
