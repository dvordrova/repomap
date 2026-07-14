# Component, surface, saved-trace, and evidence coherence

This audit records the state before decision 070 and the joins used by its
implementation. It deliberately distinguishes persistent facts from browser
projections and model suggestions.

## Object map before decision 070

| Product object | Producer | Persistent artifact | Browser DTO | Status model | Component relationship | Exact evidence | Current navigation path before 070 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Architecture component | `internal/report/architecture_canvas_build.go` builds a `componentmap.CandidateBundle`; `componentmap` validates model-proposed membership and builds the landscape | `report.json.architecture_canvas` | `ArchitectureCanvas.Components[]` / `ArchitectureComponent` | architecture source, grounding mode, hypothesis flag, diagnostics | Component owns exact `Candidate` members. Members carry `FlowParticipation`; components therefore derive `ParticipatingFlowIDs`. | Member `LocalFact`s, anchor IDs, behavior anchors, and typed structural witnesses | Architecture card -> canvas-local component inspector. The inspector was inline and canvas-specific. |
| Legacy orientation component | `internal/report/components.go` from the orientation high-level map | `report.json.components` and manifest `components` authority | `ReportData.Components[]` / `Component` | role and role basis | `RelatedFlowIDs` are derived mostly from exact path overlap, with a bounded semantic fallback | anchor groups with exact allowlisted locations | Used only when the architecture canvas was absent; its richer symbol actions were detached from canvas components. |
| Surface | local Go surface discovery, projected by `internal/report/surfaces.go` | `trigger_catalog.json`, `surface_coverage.json`, then `report.json.discovered_surfaces` | `DiscoveredSurfaces.Triggers[]` / `DiscoveredTrigger` | certainty, resolution, status, provisional identity, dynamic frontiers | Before 070 there was no component ID, member ID, executable ownership field, or flow ID. Process entrypoint, registration site, and handler package were retained but not joined. | registration/start locations, process symbol, wrappers, evidence records, and frontiers | Standalone **Discovered surfaces** shelf, independent of architecture and selected flows. |
| Suggested investigation | orientation model plus local confidence/disposition policy | orientation artifacts and `report.json.candidate_directions` | `CandidateDirection` | accepted/rejected disposition, reason, confidence, optional local verification/proof | A direction with a valid `LocalProof` becomes a `componentmap.Flow`; exact anchor bindings can make components participate | model evidence plus optional exact `FlowProof` | Direction cards and flow detail. Suggestions could be confused with expanded/saved flow counts. |
| Saved bounded explanation | focused-flow pipeline, loaded by `parseFlows` | `flows/<id>/flow_bundle.json`, `flow_status.json`, optional `flow_report.json` or `error.txt` | `ReportData.Flows[]` / `FlowData` (`DATA.flows`) | evidence-only, expanded, failed/error; confidence and warnings | Only joined to legacy components by common string flow ID | bounded files, tests, docs, package edges, and model explanation evidence | Top-level **Guided flows** selector opened a detached report tab. |
| Saved trace (`FlowProof`) | bounded local reducer and CLI assembler | `CandidateDirection.LocalProof` session in `report.json`; projected into `architecture_canvas.flows` | `ArchitectureFlow`, steps, branches, slots, flow edges, frontiers | verified/unresolved proof slots, diagnostics, reducer stop state in the source session | Exact member participation and `FlowAnchorBinding` derive participating components | exact anchors, transitions, relation/resolution/invocation/certainty, `file:line` locations | Canvas flow overlay/inspector. It did not navigate to `DATA.flows`, while Guided flows did not synchronize the canvas. |
| Evidence | deterministic survey, Go facts, architecture grounding, surface discovery, FlowProof, and focused investigation collectors | report JSON plus source-specific artifacts | locations, member facts, surface evidence, trace steps/transitions, file cards | certainty and provenance vary by producer; static evidence is not observed runtime | Evidence binds members, anchors, surfaces, and trace steps; path-only legacy joins are secondary | repository-relative path, line/column, symbol/callsite, relation and provenance | Several independent file-reference renderers called one path-based editor endpoint. |

## Existing exact associations

The following associations already existed and should be reused rather than
duplicated:

- `ArchitectureComponent.Members[].Participations[].FlowID` derives
  component-to-trace participation.
- `ArchitectureComponent.ParticipatingFlowIDs` is the validated projection of
  those member participations.
- `FlowAnchorBinding` joins a trace anchor to one exact member and therefore one
  uniquely owning component.
- Surface records retain process-entrypoint and registration/start evidence.
- Repository package facts retain package directories/files and component
  members retain exact package/file facts; unique evidence ownership can derive
  surface ownership without display-name guessing.
- Candidate directions, `DATA.flows`, and architecture traces use the same flow
  ID namespace when all three artifacts exist.
- Run manifests bind the exact report bytes and sorted openable paths to one
  canonical analysis root.

## Where associations were lost

1. The surface projection stopped at evidence locations. It did not carry a
   derived owning component, participating components, executable category, or
   related trace.
2. Architecture and legacy components used different IDs and different
   inspectors. Canvas presence hid the legacy component actions.
3. Candidate directions, saved focused-flow explanations (`DATA.flows`), and
   architecture FlowProof traces were three independent collections joined only
   opportunistically by a string ID.
4. `Guided flows` selected detached top-level pages. Component trace buttons
   selected only a canvas overlay, and neither interaction synchronized the
   other.
5. Suggested investigations and saved artifacts used overlapping words such as
   directions, expanded directions, flow analyses, and guided flows, producing
   counts that looked contradictory.
6. Source clicks sent a repository-relative raw path and caused
   `findAuthorizedRun -> findRun -> loadRuns`, which rescanned and verified every
   saved report before synchronously waiting for `code` or `open` to exit.

## Decision-070 browser model

The browser projection uses one coherent relationship model:

```text
ArchitectureComponent
    owns zero or more derived Surfaces
    participates in zero or more SavedTraces
    contains exact local Evidence

Surface
    retains its deterministic identity and evidence
    has a derived executable/category and unique primary component when proven
    may have participating components and one exact-ID SavedTrace

SavedTrace
    is an existing bounded FlowProof projection
    starts from a matched surface or an explicit investigation seed
    crosses exact participating components
    exposes proof coverage, frontiers, and evidence

SuggestedInvestigation
    remains CandidateDirection unless a bounded trace artifact exists
```

Joins are computed from exact flow IDs, exact member participation, unique
package/file ownership, process symbols, and exact evidence locations. Ambiguous
surface ownership is rendered as **Unassigned**; it is never guessed from names.

## Source-opening path before decision 070

1. A browser file button sent `{run_id, path, line}`.
2. `/api/open` called `loadRuns`, rereading the runs directory, every bounded
   `report.json`, and every usable manifest.
3. It binary-searched the selected manifest's openable paths.
4. It resolved the path beneath the canonical analysis root.
5. It ran `code --goto` or macOS `open -a ... --args --goto` with
   `CombinedOutput` and waited for process exit.
6. Only then did it return HTTP 200.

The editor click itself did not run Git, freshness reconciliation, gopls,
DeepSeek, symbol analysis, or surface discovery. Full repository freshness was
performed while serving a report, not by `/api/open`. The avoidable click
latency was therefore the O(number of saved runs) report reload plus synchronous
launcher waiting. Cold VS Code startup remained an external, separately
measurable cost.

## Decision-070 source-opening authority

On report-cache load, the server builds an immutable per-run index from opaque
source IDs to manifest-authorized repository-relative targets. Browser actions
send only run ID, source ID, and bounded line/column coordinates. The server
does an O(1) run lookup and source lookup, resolves the already-authorized file,
starts the pre-resolved editor launcher without a shell, and responds after
process start. Cache refresh is explicit and process-local; it is never a
persistent authority source.

## Implementation validation

The implementation was replayed against the owner-provided, model-backed saved
runs in the default cache. Neither replay used offline generation:

- Restic `20260712-210947-restic` (`deepseek-v4-flash`, `offline: false`) had a
  failed optional architecture-synthesis artifact but retained two accepted
  FlowProof sessions. The server now projects the deterministic local-anchor
  canvas from those saved facts. Exact saved-trace participation separates the
  `cmd/restic` process entry into **CLI Commands** while unrelated helper
  binaries remain **Tool entrypoints**. The CLI component owns the `backup` and
  `check` start surfaces and participates in both corresponding saved traces.
- Caddy `20260712-101953-caddy` (`deepseek-v4-flash`, `offline: false`) contains
  a bounded `component-landscape-v2` capture. That capture is decoded, upgraded
  only at the proposal-version boundary, and revalidated against the current
  local candidate bundle. Current bounds retain it as normalized model output
  rather than trusting its old cache key or validation metadata.

The saved Caddy synthesis capture does not contain an Admin API component; its
subsystems are Core, Config, CLI, HTTP Module, TLS & PKI, Events, Filesystem,
Standard Modules, and Testing. The older orientation component list does contain
`Admin API server`. The implementation does not invent or duplicate that
component in the validated architecture canvas. A committed deterministic
product fixture verifies the required zero-surface/one-suggestion Admin API
relationship when that component exists, but the exact saved Caddy synthesis
cannot demonstrate that browser selection without a new approved synthesis
artifact.

Playwright checks covered `1600x1000`, `1440x900`, and `1280x800`. They verified
the fixed drawer, component replacement without stacked drawers, backdrop and
Escape-compatible close behavior, Restic surface-to-trace navigation, trace
status and participating components, and return to the prior architecture
selection. Screenshots were retained as local review artifacts and are not
committed.

A follow-up browser regression found that the Saved traces picker was rendered
inside the toolbar's clipped horizontal scroller, so its options existed in the
accessibility tree but were not visible. The picker is now a fixed, bounded menu
outside that clipping context. The same review found that surface and suggested-
investigation indexes were populated inside the saved-trace loop, which made
them unavailable when a canvas had no trace. Those indexes are now independent,
and exact owning surface names are rendered as bounded chips directly on their
architecture component cards.

## Source-open timing result

The model-backed Restic report was served with a fake `code` executable that
recorded arguments and slept for two seconds. One browser click produced exactly
one dispatch:

```text
--goto
/Users/dvordrova/git/restic/cmd/restic/main.go
```

The server reported `resolve_run_ms=0`, `authorize_ms=0`,
`resolve_target_ms=0`, `spawn_ms=2`, and `response_ms=3`. The fake editor exited
about 2.3 seconds later, proving that editor lifetime is outside browser response
latency. No Git, gopls, model, symbol, or whole-repository freshness work ran in
the source-open handler. Separately, loading and freshness-checking the full
saved Restic report measured about 0.48 seconds; that report-view cost is not on
the source-click path.

## Validation commands

- `./scripts/check.sh`
- `go test -race ./internal/reportserver`
- model-backed Restic and Caddy replay tests with their exact cache paths
- model-backed Restic saved-report latency test
- `node --check` for all report JavaScript assets
- `./scripts/etcd_check.sh ../etcd`
