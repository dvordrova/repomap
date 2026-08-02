# Fixture impact — checkpoint `7e901b7`

## Basis

- Active decision: `094-syncthing-surface-trace-product-fixture.md`.
- Production checkpoint: `7e901b7` (`fix: resolve unique local callback origins`).
- The production diff from `f6ae3cf` is confined to
  `internal/experiment/surfacediscovery/analyzer.go`: bounded unique store,
  parameter, return, and callback-origin resolution. It changes which dynamic
  callback edges can become authoritative surface/wrapper evidence.
- The uncommitted `.opencode/`, `opencode.json`, workflow directory, and related
  evaluation files are workflow configuration/material and are ignored for
  impact selection. No production or test changes are part of this assessment.
- Reuse the pinned, clean source oracles and retained fresh artifacts for
  Restic, Caddy, and Syncthing. Do not make an oracle/provider call merely to
  establish fixture identity.

## Smallest conservative rerun set

### Selected

1. **Synthetic surfacediscovery fixtures** — `captured_closure`,
   `separate_mains`, `interface_single`, `interface_multiple`, `wrappers`, and
   `caddy_admin`. These are the smallest existing controls for the changed
   origin resolver: positive closure/store propagation, executable isolation,
   interface ambiguity, wrapper preservation, and exact route-provider
   behavior. The current suite has `TestAnalyzeBindsCapturedClosureFreeVar`,
   `TestAnalyzeDoesNotCrossExecutableRoots`, `TestAnalyzeInterfaceTargets`,
   `TestAnalyzeRepositoryWrappersAndValues`, and
   `TestAnalyzeCaddyAdminRouteProviders`; keep their negative/frontier
   assertions rather than substituting aggregate counts.
2. **Restic** at oracle revision `987caba4089fc4345bb201e62c5a2ba96b168049`.
   This is the same-signature/deferred-callback regression owner. Re-run the
   offline surface artifact and inspect `retry.(*Backend).Stat` for removal of
   the false Azure and `x/net/trace` static wrapper paths, while retaining an
   unresolved/possible dynamic frontier and helper-tool ownership. Do not
   freeze totals; the retained full-surface artifact is incomplete, so a
   completed current artifact is needed for a release verdict.
3. **Caddy** at oracle revision `873fac5fc094fe538d0c477509127bb321d51a32`.
   This is the dynamic callback and captured-value positive control. Re-run the
   offline analyzer/product path and inspect both the false
   `closeConnections -> caddy.run$2` chain and the exact admin registrations.
   The existing canonical script is useful as a direct check but remains a
   known failing oracle until `/config/`, `/id/`, and `/stop` wrapper recall is
   repaired; its historical aggregate totals must not be used as a pass/fail
   substitute for semantic inspection.
4. **Syncthing** at oracle revision
   `d4cffd848eb13d65f3caca5ef6da9a3fd25a2d6a`. The shared analyzer change can
   alter process-entry, route, server, frontier, and ownership evidence, so
   the decision fixture cannot be excluded. Re-run offline first and verify
   the exact primary process seed, scoped `auto.Assets` diagnostics, secondary
   services, tooling/helper records, and no fabricated callback paths. Reuse
   the saved orientation response for deterministic replay; do not treat the
   stale fixed 36-record script as authoritative until membership semantics
   are corrected.

## Direct checks

Run the focused checks before any model call:

```text
go test ./internal/experiment/surfacediscovery -run 'TestAnalyze(DirectRoute|RepositoryWrappersAndValues|BindsCapturedClosureFreeVar|CrossPackageWrapper|DoesNotCrossExecutableRoots|InterfaceTargets|CaddyAdminRouteProviders|DynamicAndNegativeControls|ImportReachableDetachedHTTPComposition)' -count=1
go test ./internal/report -run 'Test(ParseDiscoveredSurfaces|ProjectDiscoveredSurfaces|.*Surface.*|.*Coherence.*)' -count=1
go test ./internal/orient -run TestReplaySavedSyncthingOrientationSeedsPartialTracesWithoutProvider -count=1
go vet ./internal/surfacediscovery/... ./internal/report/... ./internal/orient/...
```

The Caddy and Syncthing fixtures should record their semantic failure honestly
if their unchanged fixed-count/wrapper expectations still fail; do not relax
them in this assessment. For Restic, use the retained offline command/artifact
path and a semantic assertion over the changed wrapper chains; the prior
`--discover-surfaces=false` fallback is not sufficient evidence of a full
rerun.

## Skipped fixtures

- **Core persisted canvas fixtures (Colima, Restic backup, Soft-Serve daemon):**
  skipped as separate external reruns. `7e901b7` changes analyzer admission,
  not FlowProof, report projection, canvas schema, or renderer code; existing
  local report/canvas tests cover the unchanged shared
  contract. Restic remains selected above for its external surface behavior.
- **Additional HTTP/Echo synthetic fixtures:** skipped. No HTTP projection or
  Echo production code changed; Caddy plus the existing wrapper/closure fixtures
  cover the affected callback-origin contract.
- **Other external repositories and language analyzers:** skipped. No affected
  package or shared contract changed outside Go surface discovery.
- **Performance-only reruns:** skipped. The checkpoint changes correctness of
  admission, not a measured performance contract; existing timing artifacts
  are not a valid new baseline.

## Provider and browser requirements

- **Fresh provider calls:** not required for the direct checks or the retained
  Restic/Caddy regressions; use existing fresh source oracles and saved/offline
  responses. Decision 094 still requires **one fresh bounded Syncthing provider
  run** after deterministic checks pass, because the current offline artifacts
  cannot establish suggestions, focused research, or a saved partial trace.
- **Browser review:** required for that fresh Syncthing run. Inspect the exact
  primary ID through report, trace, suggestion/component membership, evidence
  bundle versus saved-trace counts, diagnostics, and rendered surface/readiness
  counts; capture screenshots and console output. No browser run is required
  for unchanged offline Caddy/Restic artifacts in this rerun wave.
