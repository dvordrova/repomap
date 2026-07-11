# Architecture canvas v2

Status: accepted design, implementation in progress.

## Product question

The same canvas must answer two different questions without conflating them:

1. What major parts does this application contain?
2. How does one concrete operation move through those parts?

The default view is a conceptual architecture landscape. Selecting one flow
projects saved FlowProof evidence over the same stable component positions.
Selecting a component, step, or edge opens the evidence for that object without
running repository analysis or making a provider request.

## Current behavior and ownership

The current report reconstructs components in `internal/report/components.go`
from model `high_level_map` prose and allowlisted paths. Direct package imports
become component relations. `internal/report/templates/script.js` then places
components in role lanes (`entry`, `boundary`, `domain`, and similar), draws
straight center-to-center SVG arrows, and keeps selection in a closure-local
numeric index. Flows remain separate tabs. FlowProof is shown as a ledger rather
than a first-class canvas layer.

The restic baseline demonstrates the concrete failures:

- four broad components are forced into one role-oriented row;
- lines and inline `imports` labels cross node content;
- repeated imports from the overlapping `cmd/restic` package become decorative
  duplicate component edges;
- arrow direction looks like execution even though the witness is a static
  package import;
- the Backup and Init flows are links outside the map;
- an old cached report can still embed FlowProof v1 and the audited false linear
  Scan/cancel/Wait story;
- selection has no stable URL state and the inspector is component-only;
- without model `high_level_map`, the canvas disappears.

Current owners remain deliberately separate:

| Concern | Owner |
| --- | --- |
| local repository/package/entrypoint facts | `internal/snapshot`, `internal/gofacts` |
| typed flow facts | `internal/flowproof` and bounded language adapters |
| conceptual synthesis transport | `internal/deepseek` |
| locally validated conceptual membership | `internal/componentmap` |
| saved canvas projection | `internal/report` |
| layout and interaction | embedded report assets |
| source/editor actions | `internal/reportserver` under run-manifest authority |

## Target interaction model

The navigation hierarchy is:

```text
landscape
  -> selected flow
    -> selected component
      -> selected step or edge
        -> exact saved evidence
```

Landscape mode shows roughly 8–15 short component names inside human-oriented
subsystem groups. Nodes do not contain summaries, counts, paths, and every
badge at once. Selecting a component keeps positions fixed and moves purpose,
members, symbols, packages, tests, evidence, and limitations into the adaptive
inspector.

Flow mode keeps the same component positions, dims unrelated nodes, suppresses
the structural layer, and displays exactly one saved flow. Main work and
asynchronous task branches are distinct. A task start, callback body,
cancellation, join, handler return, and unresolved frontier keep their own
semantics. Selecting a step or transition replaces the inspector contents with
its callsite, declaration, relation, invocation, resolution, certainty,
provenance, scenario, and limitations.

The hash stores stable IDs only:

```text
#flow=<flow-id>&component=<component-id>&step=<anchor-or-transition-id>
```

Ordinary selection and hash restoration never trigger layout, analysis, or a
model call. Layout is recomputed only when graph contents or layout inputs
change.

## Library decision

Retain the embedded DOM/SVG renderer and replace manual role-lane layout and
straight-line routing with ELK.js 0.11.1 layered layout.

The bounded spike used one ten-component restic-like fixture for both options.

| Check | DOM/SVG + ELK | React Flow + ELK |
| --- | --- | --- |
| rich custom nodes | yes | yes |
| subsystem groups | yes; ELK compound graph verified | yes; group nodes verified |
| explicit ports | yes | yes |
| routed edge sections | rendered directly from ELK | requires a custom edge renderer |
| stable positions on selection | yes | yes |
| structural and flow layers | yes | yes |
| async branch and join styles | yes | yes |
| clickable edge and hash restore | yes | library helps, hash remains custom |
| production integration | existing embedded report | new React/npm/esbuild boundary |

Measured spike assets were approximately 1.61 MB for `elk.bundled.js`; the
React/React Flow/ELK bundle was approximately 1.86 MB plus 16 KB CSS. Bundle
size was therefore not the deciding factor. React Flow would own useful
pan/zoom, focus, and selection behavior, but it would not consume ELK edge
sections by itself and would require bridging or porting the existing
inspector, run server, editor links, and source drill-down. The retained
renderer already has those integrations. The DOM spike needed only a small
amount of interaction code after ELK removed custom geometry.

ELK is a layout engine, not an evidence or rendering authority. It receives
only presentation nodes, ports, groups, and edges and returns coordinates and
edge sections. Its output is never saved as an analysis fact and model output
never supplies coordinates.

Rejected alternatives:

- **React Flow + ELK now:** useful if the whole report interaction layer later
  moves to React, but too much migration for this canvas vertical slice.
- **Dagre:** simpler, but does not provide the compound layout and edge routing
  required here.
- **Retain hand-written lane layout:** cannot solve routing, grouping, or flow
  overlays without growing fragile geometry code.
- **Model-generated diagram/layout:** would mix hypothesis with structural
  truth and make output non-deterministic.

## Saved-data contracts

Presentation will consume one versioned canvas projection saved in
`report.json`. It is derived locally from these existing inputs:

- validated conceptual components and exact typed members;
- deterministic package relations and their witnesses;
- FlowProof v2 anchors, transitions, slots, conditions, and locations;
- run-model metadata and validation diagnostics;
- manifest-authorized editor paths.

The projection contains separate arrays for:

- subsystem groups;
- components and exact member IDs;
- quiet structural relations;
- selectable flows;
- flow steps/anchors;
- typed flow transitions;
- task-branch identity;
- unresolved/unassigned frontiers;
- diagnostics and deterministic-fallback state.

Every structural relation carries a stable ID, relation kind, component-specific
source and target member IDs, local witness, certainty, provenance, and
scenario. A package edge with ambiguous component ownership remains a detail
fact and does not become a canvas edge.

Every flow transition refers to its exact FlowProof transition ID. A proof
anchor is assigned to a component only through a unique exact typed member. An
ambiguous or unassigned anchor remains visible as a frontier beside the canvas.
The renderer never uses `RelatedFlowIDs` as proof of participation.

FlowProof/session versions are validated independently of report format.
Obsolete v1 proof is skipped with a visible diagnostic rather than wrapped in a
current report.

Stable conceptual component IDs derive from the sorted exact member IDs plus
the component-contract version. Display name, order, and model wording do not
participate in identity.

## Conceptual synthesis and fallback

A deterministic candidate builder runs before presentation truncation. It
produces bounded opaque IDs for package/file/symbol/entrypoint/flow candidates
and local relations. The deterministic fallback groups those candidates by
module, meaningful package roots, entrypoints, and proven flow participation;
it remains usable without a configured provider.

At most one conceptual-synthesis request is allowed per repository revision.
The provider may return only supplied candidate IDs, subsystem/component names,
membership, split/merge choices, short purposes, and conceptual ordering. It
cannot return relations, proof, paths, coordinates, styles, certainty upgrades,
or removal of unknowns. Local validation drops unknown IDs and empty groups,
retains diagnostics, and falls back deterministically when the proposal is not
usable. The saved result records prompt version, provider profile, request
bytes, latency, validation warnings, and fallback reason.

## Architecture and flow semantics

Architecture mode is conceptual. Its groups and names may be model-assisted
hypotheses. Structural edges are quiet context with exact local witnesses; they
do not imply execution or temporal order.

Flow mode is evidence projection. It uses typed FlowProof v2 relations only.
Task anchors and the relations touching them define the bounded asynchronous
branch. Remaining handler relations form the main branch for presentation.
Source locations may provide stable display ordering inside a branch but never
create an edge. No general happens-before solver is introduced.

For restic Backup the accepted shape is a handler/main branch plus a scanner
task branch. `Scanner.Scan` appears only in the task. Cancel and Wait are sibling
handler operations; the `joins` relation is `Wait -> scanner task`. Guarded
concurrency stays partial, concrete backend writes stay a frontier until proven,
and handler return does not become process termination.

## Remaining evidence limitations

- FlowProof v2 has task identity but no general branch, source-order, or
  happens-before contract.
- Some transitions retain only provider name rather than full provenance and
  scenario; the projection must not manufacture missing metadata.
- Current restic proof does not reach concrete external writes or process exit.
- Model-assisted grouping remains a conceptual hypothesis even when every
  member ID is locally valid.
- Python has no package-import graph equivalent to Go; its deterministic
  fallback may be file/module-oriented and explicitly weaker.
- Layout quality for very large or cyclic conceptual graphs remains a bounded
  product constraint rather than a claim of universal diagramming support.

## Verification strategy

Visual iteration uses a checked-in, provider-free FlowProof v2 fixture and a
small preview server. The primary slice is restic Backup. After step/edge drill-
down works, add one daemon fixture and one branching/plugin/backend fixture.
Tests protect topology, layer membership, identity/hash restoration, local
validation, and absence of invented transitions rather than pixel details.
