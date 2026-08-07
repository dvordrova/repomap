# 236 — Repository Map as primary product

**Status:** ACTIVE (v11 Decision 2; MAP_READY gate PASSED — see MAP_READY.md)
**Prerequisite:** Decision 235 committed (2eb5da5) + implementation (17db089)
**Deferred after:** nothing — this is the final v11 decision.

## Fresh council verdicts (all applied)

- Product: PASS conditional — 6 bounded defects applied below.
- Red-team: 3 required findings (F1 route state, F2 line contract data,
  F3 entry contradiction) + 4 bounded specs — all applied below.
- Diagram/layout: 3 bounded findings (inspector-aware fit, banded layout,
  shared band style + remainder symbols) — applied below.

## Product navigation

- Ordinary product navigation: **Map → Study**.
- **Map is the default route**: new `map` view end-to-end — state default
  `'map'`, `defaultWorkspaceHash` → `#/map`, `workspaceHashForState` and
  `parseWorkspaceHash` branches, `renderWorkspaceState` dispatch,
  `renderWorkspaceTabs` replaces the Overview tab with Map,
  `viewSectionID` reuses `rm-architecture`. Route `#/overview` becomes
  invalid (canonicalized to `#/map`); `#/architecture?focus=` aliases to
  map + Landscape lens.
- Overview is no longer a competing destination. Its useful information
  becomes the **empty-selection Map inspector** (thesis/brief/atlas shelf,
  entry perimeter, entries, remainder — today's `renderOverviewWorkspace`
  content, render order script.js:7220-7228).
- Canvas-less reports: the architecture-unavailable guard must NOT reset
  view `map` — the empty-selection repository summary renders from report
  data that exists without a canvas.
- Architecture becomes the **Landscape lens** (one of four Map lenses).

## Task Lens (out of v11 scope, preserved contract)

`repomap investigate <repo> --task-file <task.md>` is a SEPARATE command
(decisions 124/125) that produces its own run with `task_investigation` in
the report. It is NOT part of the v11 Map/Study product surface and is not
gated by v11 acceptance: the ordinary report never populates
`task_investigation` (omitempty, report.go:250), so the `#/investigate/…`
route and the `investigate` view are unreachable without an explicit
investigate run. This decision does NOT remove, re-review, or extend Task
Lens; the route branch, `defaultWorkspaceView()` task guard, and
`task_lens_asset_test.go` are preserved as-is so a future change cannot
delete the mechanism by accident.

## Map workspace

Desktop structure (one viewport at 1440×1000):

```text
repository header / thesis / revision / coverage / Start Here
controls | canvas | inspector
```

- The Map workspace is a 2-column grid with the inspector as a column
  (precedent `.rm-workspace.has-source-drawer`), OR fitBounds/
  focusInitialLandscape subtract the open inspector rect — a fitted
  landscape must never sit under the open inspector (D234 no-overlay rule).
- Map lenses: **Landscape, Entrypoints, Integrations, Mechanisms** — all
  projections over the single `architecture_canvas` object (one ELK layout,
  switch by emphasis/dimming — matches "overlay stable component positions"
  and "transform persists"; no relayout API needed). Lens-switch layout
  contract: keep one ELK layout; "Fit exposes every principal node" is
  scoped per lens.

## Default Map composition

```text
entry / consumption perimeter
          ↓
analyzed repository scope
  subsystem enclosures
  principal components
  shared/cross-cutting concerns
          ⋯
observed external/state touchpoints
```

Layout: add a "banded" mode (or post-pass over the board packer) with band
Y offsets — entry band y=0, scope band below, touchpoint band last.
`landscapeBounds()` already covers every group+node, so Fit is automatic.

## Entry categories

First-class category nodes:

- process/server;
- CLI command groups;
- HTTP/API route groups;
- workers/jobs/consumers — **data source:** bounded classification of
  `runtime_activity` triggers by kind (`worker`/`async_task`); the worker
  array today is declared but never populated (script.js:6337,6375-6381);
- public API/constructors/registration/lifecycle — **data source:**
  BehaviorAnchors `registry_write`/`lifecycle_interface`/`lifecycle_start`/
  `command_dispatch` (landscape.go:193-204) on `library_framework` shapes;
- secondary services.

Each shows: exact total, evidence/coverage, 1–3 representatives, unresolved
count, `Show all N` (expands in-band/inspector, never a floating overlay;
category cards get a taller metrics branch in groupMetrics for the
total+coverage+reps+unresolved+ShowAll content).

Shape rules (repository archetype determines which categories are primary):

- CLI: commands are primary, not tooling (D233 already enforces this).
- Service: server + route/handler groups are primary.
- Library: public API/registration/lifecycle is primary.
- Worker: jobs/consumers are primary.
- Monorepo: categories are scoped by app/library (per-app promotion rule,
  script.js:6358-6361).

Value-shaped/unresolved entries remain in a frontier disclosure (D233:
unresolved stays counted + reachable, never hidden).

## Touchpoints

First-class observed touchpoint families — **closed generic
import→family classification** (database/broker/pub-sub/filesystem/
object-storage/cache-lock/config-secrets/process-OS/HTTP-gRPC-SDK; must
never be a per-repository keyword table — DO-NOT-ADD) with a fixture test.
Today raw import paths are shown (familyFromImportPath returns the domain
prefix).

Rules:

- Boundary and Resource sharing evidence co-project into one visible
  interaction (already implemented: architecture_association.go:362-365,
  `Paired`);
- witness precision and association scope are independent;
- root package does not own descendants;
- generic SDK/client is not automatically a named external system;
- unassigned observations live in Shared repository touchpoints.

Canvas-less runs: touchpoints aggregate from `DATA.repository_atlas`
boundary/resource observations directly (Atlas is embedded), entry
perimeter from `discovered_surfaces`, with an explicit degraded-state note.

## Principal components

A principal node must add marginal understanding through:

- distinct bounded exact members;
- distinct entry participation;
- distinct touchpoint scope;
- distinct responsibility;
- explicit shared/cross-cutting role.

Equivalent member sets do not become multiple boxes (D235/266 corpus: 0
duplicate member-set pairs).

Cross-cutting TLS/config/logging/metrics/tooling use a shared band/node/
badge or secondary tray (D234 TLS bias). They do not visually become the
repository core merely because their anchors are easy to detect. The shared
band needs `"shared"/"cross_cutting"` added to semanticCategory + a
`.rm-arch__group.is-shared` style (distinct from the amber diagnostic
remainder).

Diagnostic remainder is outside the principal graph:

```text
Unclassified exact scope · N packages / M symbols
```

(add a symbols count param to the EN/RU remainder messages — today they are
packages-only).

## Line contract

- solid directed + arrowhead: exact local directed relation (flow edges,
  hidden until a flow is selected — shown in flow focus / Mechanism lens);
- thin neutral: product-relevant static structure (structural edges);
- dotted: observed in scope, not runtime dependency — **data source:**
  D225 observed-callsite association rows projected as dotted edges from
  component to a touchpoint node + new CSS class;
- dashed directed: partial/frontier — **data source:** remove the userMode
  zero-gate (architecture_canvas.js:591) and render `ArchitectureFrontier`
  items (FlowID/AnchorID/Slot/Reason/Evidence) as dashed directed edges
  from their anchor's component (reuse flowStepLaneComponent);
- enclosure/badge: conceptual membership (subsystem enclosures +
  category-marker);
- no line: no supported relation.

Edges are passive (no edge click required for selection; D234).

## Canvas interaction (preserved from D234)

- wheel/trackpad over canvas zooms (visible hint, bottom-left);
- drag blank canvas pans;
- page scroll remains available outside canvas;
- `+`, `−`, `Fit/Overview`, Reset;
- Fit exposes every principal node (FIT_MIN_SCALE removed, D234 F1) and
  accounts for the open inspector rect;
- semantic zoom simplifies cards at low scale (data-semantic-scale switch
  exists; new card types need matching overview-scale CSS rules);
- no toolbar covers a node;
- transform persists through selection, source open, lens change, Back
  (lens mode must not add a view write site);
- selected node highlights exact one-hop neighbors, entries, touchpoints;
- unrelated nodes dim without relayout.

## Component deepening (inspector)

Inspector tabs: **Summary | Connections | Read code** (map the current
7-section single scroll onto the tabs; add "one reason each" for sources —
no reason string exists today).

Summary fits one inspector viewport at 1440×1000:

- name; one-line responsibility; authority/coverage; at-a-glance counts;
- up to three entry groups; up to three key interactions;
- one primary Study question; up to three unknowns.

Connections:

- Used by; Uses; Entry categories; Touchpoints;
- shared/cross-cutting participation.

Read code:

- 3–5 ordered exact source starts; one reason each; exact pinned source
  action; Show all; Study this area.

Full provenance/witnesses live under **Evidence and limitations**.

**Entry participation consistency (F3):** build the inspector's "how work
enters" surfaceStarts from the same `canvas.surfaces` join the component
card uses (derive location from surface.Evidence[0] or the related trace
start; keep trigger enrichment when available) — never "no observed entry"
when the card shows owned surface chips. The lowInformationComponent gate
(architecture_canvas.js:3466-3469) is narrowed to the unclassified-remainder
component only; the required Summary always renders name/responsibility/
counts.

## Mechanisms

Mechanisms are a Map lens/overlay, not a long report section.

Every visible transition has explicit: source, target, support, ordering,
evidence, limitation (already in D226 MechanismTransition). Array order is
not path order (ordinals assigned post-sort). Connected process-data/DFD-like
fragments overlay stable component positions. Independent evidence remains a
separate fragment. Unknown frontier is explicit. No BPMN, FFBD, or
whole-repository call graph (DO-NOT-ADD).

Mechanism lens data: `findProcessEntryAnchor` (mechanism_fragment.go:183-195)
currently builds ONE fragment for the first process entry — parameterize per
entry/flow (per-flow entrypoint kind + flow_edges + frontiers) so every
visible transition has a fragment.

## Study/Map coordination

- Study theme → Show on Map;
- component → Study this area;
- Study reading → exact source;
- Back restores selection, filters, scroll, and canvas transform.

Themes remain editorial lenses, not component nodes.

## Source actions

Every visible exact source identity is:

- one coherent actionable symbol + path:line control; or
- explicitly unavailable with reason.

No source-looking inert text. No nested interactive controls (D230; the
association-row structure with noninteractive container + toggle + sibling
witness list is the pattern).

## Identity

Map work touches the frontend + at least one projection. Bump from actual
bases: `CurrentFormatVersion` 36→37 (report.go:35),
`AtlasStudyReportProjectionVersion` 13→14 (report.go:41), UI catalog
`VERSION` 9→10 (ui_messages.js:4). Amend pinned tests:
exact_workspace_search_test.go (format pins), user_workspace_asset_test.go
(invalid-route canonical hash, route hash tests), d233_overview_classification
_test.go / overview_projection_d229_test.go (Overview content moves to the
empty-selection inspector).

## Desktop-first scope

Acceptance viewports: 1280×800, 1440×1000, 1920×1080, 125% zoom, 150% zoom.
Mobile is not a v11 product target; mobile-only defects may not block.

## Tests

- default route (`#/map` is the landing route; `#/overview` canonicalizes;
  canvas-less reports still render the map workspace with the repository
  summary);
- Map/Study navigation;
- empty-selection repository summary (thesis/revision/coverage/Start Here);
- entry categories by repository shape (CLI/service/library/worker/monorepo;
  worker = runtime_activity kind classification; library = anchor-derived);
- touchpoint co-projection (Boundary+Resource → one interaction) + closed
  family classification fixture;
- canvas-less touchpoint aggregation (Atlas observations);
- shared/unclassified placement (shared band style distinct from diagnostic);
- list/canvas/inspector ID/count invariance;
- passive edges;
- component focus (neighborhood highlight, dim without relayout);
- compact inspector bounds at 1440×1000 (fit accounts for the open
  inspector);
- mechanism connectedness (connected fragments only, frontier explicit;
  per-entry fragments);
- source actions (actionable or explicitly unavailable; ≤2 clicks);
- line contract rows (solid flow, neutral structural, dotted association,
  dashed frontier);
- EN/RU catalog parity (incl. remainder symbols count).

## Acceptance

Real desktop browser at the five viewports; first-contact answers within 20
seconds on Map; canvas wheel/drag/Fit match hint; inspector Summary fits one
viewport; source in ≤2 actions; Study core-first with co-projected
duplicates; no console errors; focus/Back restoration; lens-switch keeps
transform; no nested interactive controls (browser DOM walk assertion).

