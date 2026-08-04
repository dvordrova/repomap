# Decision: User Report v2.2 — stable Mechanism reading

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product outcome

A reader can open one source-backed Mechanism, understand each step beside
compact real code, navigate between steps, reload, use browser history, share a
deep link, inspect related source, and return without losing reading context.

## Approved implementation

1. Extend the existing User Report workspace and renderer. Do not create a
   second frontend or change the architecture canvas.
2. Make Overview, Mechanisms, one Mechanism and step, Search, and Architecture
   stable hash routes. Invalid Mechanism IDs return safely to Overview. Browser
   Back, Forward, and reload restore the route-selected state.
3. Keep source-drawer state subordinate to the current Mechanism and step.
   Closing the drawer preserves that context; browser history may close the
   drawer before leaving the reading state.
4. Project a short human-facing title from the repository name and the accepted
   question. Keep model titles, artifact IDs, canonical Mechanism content, and
   semantic hashes unchanged.
5. Remove the duplicated `Source-backed path` prose from the user projection.
   The short answer comes from an already accepted step statement, and the step
   navigator remains the sole ordered overview.
6. Show compact bounded source fragments around exact evidence, with omitted
   regions marked. A secondary action may reveal the saved full function. This
   is a presentation projection and does not alter evidence bindings.
7. Add presentation-only `What to notice` callouts derived solely from accepted
   statements and their exact supporting ranges. A callout is omitted when that
   local binding cannot be established safely.
8. Keep Previous and Next controls before and after source on desktop. On
   mobile, keep a compact step header available and expose all steps without
   relying on horizontal scrolling.
9. When at least one canonical source-backed Mechanism is published, Overview
   leads with those Mechanisms and keeps raw saved excerpts secondary. Without
   a Mechanism, code-bearing saved excerpts remain the fallback and metadata-only
   cards remain hidden.
10. Use English UI chrome throughout the production report while preserving
    multilingual queries, aliases, and matching behavior.

## Truth boundary

- No model call, analyzer, probe, repository-wide analysis, fact, claim,
  validator, provider, ranking, semantic pipeline, or architecture graph change.
- No Mechanism v1 identity, LogicalID, canonical content, evidence binding, or
  semantic hash change.
- Every visible Mechanism step must retain real saved source. Presentation-only
  titles, excerpts, callouts, and route state do not become semantic authority.
- Exact-path source actions remain code-first and retain existing manifest and
  repository authorization boundaries.

## Focused checks

- Caddy and chi use the approved human titles and compact evidence-centered
  source for every visible step.
- Mechanism and step routes survive reload and browser history navigation.
- Related source opens with code and returns to the same Mechanism step.
- Overview does not lead with raw excerpts when a Mechanism exists.
- The no-Mechanism fixture retains code-bearing fallback excerpts.
- Mobile navigation remains available without page-level horizontal overflow.
- Canonical Mechanism IDs, content, and hashes remain unchanged.

## Relationship to prior decisions

This decision extends decision 108's code-first source presentation. It does
not delete, rewrite, narrow, or invalidate decision 108 or any earlier approved
decision. All truth, validation, replay, source-authority, and safety boundaries
from decision 108 remain in force.
