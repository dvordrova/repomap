# Decision: Coherent architecture navigation and fast editor opening

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## User-visible goal

`repomap <repo>` keeps Architecture as the primary context while connecting
architecture components, deterministic surfaces, saved traces, and exact local
evidence. Opening an already-authorized source location in Visual Studio Code
must use a cached, non-blocking fast path rather than reloading all reports or
waiting for the editor process.

The product relationship is:

```text
Architecture component -> surface -> saved trace -> exact evidence
```

Suggested investigations remain proposals and are not surfaces or saved traces.

## Approved implementation scope

1. Audit and document the existing component, surface, trace, evidence, and
   source-opening data paths.
2. Derive component/surface/trace joins from exact existing IDs, membership, and
   evidence. Do not add duplicate manually maintained relationships.
3. Use the visible labels Architecture, Surfaces, Saved traces, Suggested
   investigations, and Evidence. Remove the normal `Guided flows` interaction
   and contradictory counts.
4. Keep the architecture board primary. Add a fixed right-side inspector for
   components, surfaces, traces, trace steps, and evidence.
5. Preserve the accepted card-based saved-trace presentation, but render it as
   Trace focus inside the Architecture experience. Returning restores the prior
   landscape viewport and highlights participating components.
6. Retain the complete surface catalog as secondary navigation, labeled `All
   surfaces`, with honest application/tooling/test/unassigned/unresolved
   distinctions when the saved facts support them.
7. Connect a surface to its existing saved trace. A surface without a trace may
   invoke only the existing bounded local investigation path, or display a typed
   unavailable reason when that path cannot accept the exact surface seed.
8. Measure the complete exact-source click path and add bounded debug-stage
   timing for run resolution, authorization, target resolution, process start,
   and response.
9. Build a process-local, concurrency-safe authorization index when saved
   reports are loaded. Source opening uses only opaque run and source IDs, with
   manifest-bound authorization.
10. Resolve the VS Code launcher once per server startup, start it without a
    shell, do not wait for its lifetime, and retain bounded launch diagnostics.
11. Permit opening the current authorized file when its captured input changed,
    with an honest stale-source warning and no full freshness reconciliation on
    click.
12. Add fake-editor, authorization, one-launch, non-blocking, unavailable-editor,
    stale-source, Restic, Caddy, responsive drawer, and viewport-restoration
    regressions. Automated tests require neither a provider nor real VS Code.

## Performance and security invariants

- A source-open request is O(1) over an already loaded report.
- It does not rescan saved reports, run Git, reconcile repository freshness,
  invoke DeepSeek, discover surfaces, run architecture analysis, start gopls, or
  perform symbol lookup.
- It accepts no raw browser-supplied path. The browser supplies a run opaque ID
  and source opaque ID; the server maps both through the loaded manifest index.
- Editor arguments are passed as an argument array without a shell.
- HTTP success follows successful process initiation, not process exit.
- Normal logs contain no sensitive or absolute paths. Debug diagnostics may
  contain authorized repository-relative paths and never secrets.
- Warm server authorization and response target p95 is below 100 ms. Cold VS
  Code startup is measured and reported separately.

## Required fixtures

- Restic saved reports: component -> command surface -> saved trace -> evidence,
  trace focus/back behavior, viewport restoration, and one exact-source launch.
- Caddy saved reports: Admin API anchors with honest zero configured-catalog
  surfaces when applicable, and suggested investigations distinct from traces.
- Fake editor: records argument arrays, sleeps for two seconds, and proves that
  HTTP response and server shutdown do not wait for editor exit.

## Non-goals

Do not add DeepSeek calls or prompts, another architecture model, repository
analysis frameworks, framework-specific seeds, surface kinds, a database, a
canvas renderer migration, a saved-trace internal redesign, or a general IDE
integration framework. Do not weaken source authorization or claim runtime
execution from static evidence.

## Acceptance

The change is accepted when Architecture remains visible and primary; components
expose compact surface/trace counts in a fixed overlay inspector; surfaces lead
to an existing trace or an honest bounded action; saved trace focus explains
trigger, purpose, participating components, grounding, frontier, and evidence;
normal terminology and counts are consistent; the complete All surfaces catalog
remains available; source open is cached, opaque-ID authorized, non-blocking,
instrumented, and covered by a two-second fake editor; and Restic/Caddy browser
checks pass at 1600x1000, 1440x900, and 1280x800.
