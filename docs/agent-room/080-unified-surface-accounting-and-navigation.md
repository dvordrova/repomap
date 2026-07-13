# Decision: Unified surface accounting and actionable architecture metadata

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## User-visible goal

Preserve the accepted modern Architecture and selected-trace experiences while
making every supported discovered surface part of one explainable catalog.
Headline counts, component counts, executable-role groups, and All surfaces must
be projections of the same typed records. Architecture anchors remain distinct
from runtime registration/start surfaces.

## Approved implementation scope

1. Audit Restic surface accounting and document every included or excluded
   record in `docs/design/surface-accounting-audit.md`.
2. Normalize deterministic Cobra command records and generic surface-discovery
   records into one typed report catalog without erasing producer evidence.
3. Complete build-selected Restic Cobra registration discovery for the primary
   executable, including commands without saved traces.
4. Classify executables as primary application, secondary tooling,
   test/helper, or unknown using deterministic repository evidence.
5. Derive headline, All surfaces, group, and component counts from the unified
   catalog; keep detector-stage timing separate from product totals.
6. Make component-card metadata adapt to non-zero surfaces, traces,
   investigations, exact anchors, or exact members.
7. Attach suggested investigations to components only through supplied exact
   IDs, anchors, and locally grounded evidence, with typed action availability.
8. Improve Saved traces summaries while preserving selected-trace semantics.
9. Render a compact zero-surface state with coverage disclosure.
10. Add deterministic Restic/Caddy replay, accounting, ownership, metadata,
    navigation, and responsive browser regressions.

## Preserved behavior

- one modern architecture renderer and current component cards;
- `Saved traces`, complete/partial statuses, and current trace focus/back UX;
- Caddy validated architecture and honest zero-surface coverage;
- `All surfaces` as the complete repository-wide supported catalog;
- exact evidence, manifest authorization, freshness, and editor-opening behavior.

## Non-goals

Do not redesign or migrate the canvas, add a provider call or prompt, add HTTP
framework support or framework seeds, synthesize Caddy surfaces from
architecture anchors, deeply trace every command, change selected-trace
semantics, or revisit freshness/editor performance absent a direct regression.

## Acceptance

The change is accepted when Restic command and generic records share one typed
catalog; command surfaces may exist without traces; tooling tasks remain visible
under Tooling; all visible counts reconcile; Caddy cards use anchors and
suggestions rather than repeated meaningless zeroes; suggestions are actionable
or honestly unavailable; empty All surfaces is compact; the global trace picker
has useful summaries; and the named Restic/Caddy runs pass the required
Playwright and deterministic test matrix without live provider calls.
