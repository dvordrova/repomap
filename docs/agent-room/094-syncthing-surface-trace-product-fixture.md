# Decision: Syncthing surface-to-trace product fixture

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## User-visible goal

Use the saved Syncthing run `20260713-191035-syncthing` as a blind product
fixture and make `repomap <repo>` degrade usefully when part of a repository is
not loadable. Exact entry surfaces must remain distinct from nested runtime
activities, descriptors, dynamic frontiers, suggested investigations, evidence
bundles, and persisted traces.

## Approved scope

1. Diagnose and document the saved Syncthing run without repeating provider
   calls during normal iteration.
2. Isolate Go package-loading failures so one broken package does not erase
   valid surfaces from other packages or executables, while retaining scoped
   diagnostics and never treating unsafe partial type facts as exact.
3. Add generic process-entry surfaces and deterministic executable ownership
   roles for primary applications, secondary services, tooling, tests/helpers,
   and unavailable executables.
4. Persist surface role, quality, ownership, and trace-readiness with specific
   reasons. Keep entry surfaces, runtime activities, descriptors, frontiers,
   and rejected/noisy records distinct.
5. Generalize the smallest bounded local path that can seed an honest partial
   trace from exact entry evidence. Evidence-only flow bundles remain suggested
   investigations and are not saved traces.
6. Reconcile suggestions, surfaces, traces, components, unavailable packages,
   and visible counts through exact local IDs and membership only.
7. Center focused research windows on exact evidence or containing syntax and
   skip provider work when no code-bearing bounded window exists.
8. Improve existing surface and trace presentation without redesigning the
   architecture canvas or inventing runtime order.
9. Add offline Syncthing replay and preserve Restic and Caddy regressions.
10. Perform one fresh bounded Syncthing run after local checks, then personally
    inspect the served report with Playwright and capture product screenshots.

## Constraints

Production code must not contain Syncthing-specific function names, paths, or
package rules. DeepSeek may interpret supplied bounded evidence but may not
create surface authority or missing trace transitions. Missing generated or
build-selected source remains an explicit scoped diagnostic. Exact static
evidence does not prove runtime execution or ordering.

## Acceptance

The decision is complete only when a fresh Syncthing report retains useful
surfaces despite the `auto.Assets` package failure; distinguishes primary,
secondary-service, tooling, activity, descriptor, frontier, and rejected
records; persists at least one evidence-backed partial trace from an exact
entry surface; keeps suggestions and evidence bundles out of saved-trace
counts; maps exact suggestions to components; uses or correctly skips
code-bearing focused research; exposes reconciled quality and diagnostic
counts; preserves usable architecture navigation; passes Syncthing, Restic,
Caddy, and repository checks; and passes the required browser product review.
