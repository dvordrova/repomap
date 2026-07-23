# Decision: User Report v2

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product outcome

The production report becomes one repository-understanding workspace for a
reader who does not yet know the codebase. Its default presentation answers
how code works and where to read it. Validation state remains available to
replay and an explicit provenance mode, but is not ordinary onboarding copy.

## Approved implementation

1. Keep the existing production report, report data model, embedded assets,
   semantic search, architecture canvas, and source-opening authority. Do not
   add a second frontend application or renderer.
2. Add a report-only user projection for independently replayed canonical
   Mechanisms. It may contain a question, concise answer, supported steps,
   exact code locations, participating files, and existing focus identifiers.
   It must not mutate or replace the canonical Artifact or Mechanism.
3. Publish only a useful supported slice. Unresolved/interpretive statements,
   unknowns, verdicts, confidence, aspect coverage, hashes, internal IDs,
   model/provider state, candidate lineage, and diagnostics remain in the
   underlying report data but are absent from the default user projection.
4. Restructure the existing report shell into persistent repository
   navigation with Overview, Mechanisms, Search, and Architecture views plus
   an optional source drawer. A Mechanism opens in its own detail workspace;
   selecting it never automatically opens the architecture map.
5. Mechanism detail provides its question, source-backed answer, step list,
   current-step explanation, exact source locations, Previous/Next controls,
   and involved files. Evidence navigation opens the source drawer and keeps
   the Mechanism and step selected.
6. Show `Show on map` only when the current step resolves to an existing
   component, flow, flow step, or surface. Preserve a return target so the map
   can return to the same Mechanism and step. Empty focus produces no control
   and no fallback notice.
7. Semantic-search Mechanism results open the same detail workspace. Search
   and default overview must not surface warning/unknown/provenance objects.
8. Retain an explicit debug/provenance mode for raw semantic artifacts,
   verdicts, gaps, hashes, diagnostics, coverage, and run/model information.

## User-facing language

Use `Source-backed path`, `How this code works`, `Trace through the
implementation`, or `Code path`. Do not claim completeness.

Default user mode must not show `Known gaps`, `Unknowns`, `Evidence gap`,
`Unresolved`, `Verified with gaps`, `Mixed`, `Canonical`, `Proposal`,
`Insufficient evidence`, coverage funnels, verdicts, missing capabilities,
provider/model details, hashes, internal IDs, or no-op focus errors.

## Truth boundary

- The user projection is deterministic and derived only from an already
  replayed canonical Mechanism Artifact.
- Only direct or compositional statements may enter the user projection.
- A displayed step must reference at least one retained supported statement
  and at least one exact repository location.
- Existing focus IDs are presentation references, not new architecture facts.
- The canonical object remains authoritative for validation, replay, semantic
  identity, and provenance.

## Focused checks

- default HTML has no forbidden provenance labels in visible user views;
- provenance mode retains raw artifact/verdict/gap information;
- Mechanism card and search result open detail without switching to map;
- Previous/Next preserve Mechanism context and update source locations;
- source drawer exposes concrete `path:line` actions;
- `Show on map` exists only for a resolvable focus and the return action
  restores the same Mechanism step;
- the saved Caddy and chi reports render useful supported paths while their
  Mechanism hashes remain unchanged.

## Hard exclusions

- no model call, repository-wide analysis, package loading, SSA, call graph,
  runtime-surface discovery, provider/cache work, or semantic re-analysis;
- no new universal renderer, frontend application, canonical artifact shape,
  semantic relation, evidence, claim, or architecture object;
- no publication of weak broad semantic proposals as canonical Mechanisms.
