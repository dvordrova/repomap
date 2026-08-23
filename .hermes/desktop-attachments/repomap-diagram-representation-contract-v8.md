# Repomap Diagram and Representation Contract v8 — desktop-first

## Why this exists

Repomap has several different kinds of truth. One generic graph cannot display
them without making one of two mistakes:

- treating conceptual grouping as runtime behavior; or
- hiding useful exact facts because they do not fit the chosen diagram.

The product therefore uses a **small family of coordinated representations**.
The representation adapts to the available evidence. Canonical facts and
acceptance never change merely to make a diagram prettier.

## Representation map

| Product question | Primary representation | Explicitly not |
|---|---|---|
| What is inside this checkout and how is it touched? | Repository perimeter | C4 System Context |
| What are the main conceptual/code areas? | Component landscape / concept-map hybrid | Runtime flowchart |
| What surrounds one selected area? | One-hop neighborhood map | Full repository call graph |
| How does one supported behavior fragment connect? | Process-data / DFD-like fragment | BPMN or FFBD |
| Who participates, when both ownership and order are supported? | Optional local swimlane | Whole-repo swimlane |
| What are inputs/outputs/stakeholders of a theme? | Optional SIPOC-like summary table | Main architecture graph |
| What exists across repositories/deployments? | Future system-landscape mode | Claim from one repository |

---

# 1. Overview: repository perimeter

## Question answered

> What exact repository scope was analyzed, how can it be entered or consumed,
> and what external/state touchpoints were observed?

This is **not** C4 System Context. A repository is not automatically a software
system, a deployable container, or a business boundary.

## Visual structure

Use a calm three-zone layout:

```text
Observed entry / consumption
            ↓
┌──────────────────────────────┐
│   Analyzed repository scope  │
│                              │
│ apps / libraries / modules   │
│ principal conceptual areas   │
└──────────────────────────────┘
            ⋯
Observed external/state touchpoints
```

Desktop may render this horizontally:

```text
entry/consumer cards  →  repository scope  ⋯  touchpoint cards
```

### Incoming examples

- process entry;
- CLI command;
- exact HTTP registration;
- worker/job registration;
- public library API;
- plugin/callback registration;
- unresolved dynamic frontier, clearly weaker and collapsed.

### Outgoing examples

- database/client API callsites;
- message broker publish/consume callsites;
- HTTP/gRPC/SDK clients;
- filesystem/object-storage callsites;
- cache/lock/config boundaries.

## Visual rules

- Repository scope is one explicit enclosing frame.
- Incoming exact surfaces use solid ingress connectors.
- Dynamic/partial frontiers use dashed ingress.
- Touchpoints use dotted connectors because they are observations, not proven
  runtime dependencies.
- Human actors and named external systems appear only when independently
  resolved.
- Imported SDK/package names are touchpoints, not automatically systems.
- No dense package graph in Overview.
- One “Start here” source action remains visually dominant.
- Study lenses may appear nearby, but are labeled learning lenses, not
  architecture nodes.

## Empty/weak states

- Library: show consumer/public API/lifecycle perimeter.
- CLI: command/invocation → repository → filesystem/network perimeter.
- Monorepo: show multiple app/library scopes inside the repository frame.
- No exact outgoing touchpoint: show no outgoing connector, not an invented
  external-system box.

---

# 2. Architecture: component landscape / concept-map hybrid

## Question answered

> Which conceptual/code areas are useful for understanding this repository, how
> are they structurally related, and how trustworthy is each grouping?

This view is not an execution diagram.

## Default visual structure

```text
Architecture workspace
┌──────────────────┬─────────────────────────────┬──────────────────────┐
│ Area list        │ Stable component landscape  │ Selected inspector   │
│                  │                             │                      │
│ principal areas  │ subsystem enclosures        │ role / entry / uses  │
│ shared areas     │ component cards             │ touchpoints / source │
│ remainder        │ passive relation lines      │ unknowns             │
└──────────────────┴─────────────────────────────┴──────────────────────┘
```

## Node semantics

### Principal component

A bounded responsibility with distinct exact membership/anchors or an explicit
shared/cross-cutting role.

Card contents by default:

- user-facing name;
- one-line responsibility;
- grouping authority badge;
- compact evidence/coverage count;
- at most two representative surfaces or anchors.

Packages, files, functions and raw Atlas entities are not default canvas nodes.

### Subsystem/group enclosure

Used for conceptual membership. It is an area boundary, not a runtime relation.

### Shared/cross-cutting participation

Never duplicate an identical component merely to satisfy multiple groups.

Use one of:

- one shared node between enclosures;
- “shared by N areas” badge;
- shared-membership tray;
- inspector disclosure showing all participating areas.

### Diagnostic/unclassified scope

Render outside the principal graph as:

```text
Unclassified exact scope · N packages / M symbols
```

collapsed by default. It must remain browsable, but must not become a fake
central architecture component.

## Edge language

Edges are passive SVG evidence, not buttons.

| Style | Meaning |
|---|---|
| Solid directed, stronger stroke, arrowhead | Exact local handoff / supported directed relation |
| Thin neutral directed/undirected | Static structural relation such as imports, only when product-relevant |
| Dotted connector | Observed in exact member/unit scope; no runtime dependency |
| Dashed directed connector | Partial or unresolved continuation/frontier |
| Enclosure / badge | Conceptual membership |
| No line | No supported relation |

Rules:

- one visible connector per node pair;
- multiple underlying witnesses aggregate into one line;
- no default verb label on every edge;
- optional tooltip/title may state type/count/limitation;
- the same details are accessible in the inspector;
- no `role=button`, `tabindex`, hitbox or edge click handler;
- arrowheads are visible for directed exact relations;
- line color never carries meaning alone;
- relation density has a display budget; overflow becomes counts/disclosure,
  never silent deletion.

## Layout rules

- Use deterministic ELK layout; the model never chooses coordinates.
- Preserve positions while focus changes.
- Favor left-to-right when entry→core→touchpoint direction is meaningful.
- Favor grouped landscape when relation direction is sparse or mostly
  conceptual.
- Do not force all repositories into one orientation.
- Avoid long crossing edges by aggregating relations and using enclosure ports.
- A zero-relation repository is list-first with optional grouped canvas, not an
  empty “runtime graph”.

---

# 3. Selected component: one-hop neighborhood

## Question answered

> Who uses this area, what does it use, what was merely observed inside its
> scope, and what should I read next?

## Interaction

Selecting a node:

- keeps node positions stable;
- highlights exact one-hop incoming and outgoing neighbors;
- dims unrelated principal nodes;
- shows second hop only after an explicit action;
- opens the inspector;
- does not require clicking an edge.

Inspector sections, in order:

1. Responsibility.
2. Grouping authority / evidence / coverage.
3. How work enters.
4. Used by.
5. Uses.
6. Observed external/state interactions.
7. Relevant Study themes.
8. Read first.
9. Unknowns and limitations.

Connection rows are the primary relation controls:

```text
row click
→ expand witnesses and limitation
→ highlight neighbor / passive connector
→ exact source in one additional action
```

## Association precision

Display witness precision and association scope independently.

A broad package-scope observation must never look like a component dependency.

Default tiers:

- exact symbol/file/package scope: normal row;
- explicit unit/module scope: normal row with scope label;
- shared/broad scope: collapsed group;
- diagnostic remainder: available only under diagnostics.

Boundary and Resource with the same evidence co-project into one visible
interaction row; canonical roles remain in details.

---

# 4. Mechanism: process-data / DFD-like evidence fragments

## Question answered

> Which connected behavior fragment is actually supported, which data/external
> touchpoint does it reach, and where does proof stop?

## Why this representation

Process-data/DFD-like fragments require fewer unsupported semantics than BPMN or
FFBD. They can show:

- entry/external entity;
- operation/process;
- resource/data store;
- external touchpoint;
- exact transition;
- unknown frontier.

They do not require full gateways, compensation, message-flow semantics or
global execution order.

## Visual grammar

### Entry / external entity

Rounded rectangle or compact ingress card.

### Operation

Plain process rectangle.

### Resource/data store

Distinct data-store shape or double-line card only when a real resource
identity exists. A generic `database/sql` callsite remains an interaction
touchpoint, not a named database.

### External touchpoint

Peripheral pill/card with dotted connector unless a stronger transition exists.

### Exact transition

Solid directed arrow with arrowhead.

### Partial/unknown continuation

Dashed arrow ending in an explicit frontier node:

```text
Continuation not recovered
```

### Independent evidence

Separate fragment card or branch. Never append it to the previous fragment by
ordinal.

## Connectedness rule

Two transitions may be adjacent only when:

```text
target(previous) == source(next)
```

or a separate exact supported join exists.

Array order is never path order.

Example:

```text
main.go:36 main
    → exact callsite
service.Start
    ⇢ continuation unknown
```

Independent:

```text
ldap/server.go:61
    → direct static call
ldap.getTLSconfig
```

Forbidden:

```text
main → ldap.getTLSconfig → service.Start
```

unless that exact sequence is supported.

## Primary copy

Use human language:

- Direct static call
- Resolved in recorded build
- Local callsite order
- Continuation not recovered

Raw enums are under “Evidence details”.

---

# 5. Optional swimlane

Swimlane is allowed only inside one mechanism detail when both are supported:

- participant/ownership boundary;
- meaningful local or partial order.

Possible lanes:

```text
HTTP layer | application service | storage/client boundary
```

Rules:

- no lane for guessed team/business ownership;
- no whole-repository swimlane;
- a missing participant or order falls back to process-data fragments;
- cross-goroutine/distributed order remains partial unless observed.

---

# 6. Optional SIPOC-like summary

SIPOC is a compact summary table for a Study theme or mechanism, not the main
diagram.

Use only when supported:

```text
Suppliers | Inputs | Process | Outputs | Consumers
```

Unknown cells stay unknown. Do not infer business actors from package names.

Useful for:

- login/authentication;
- backup/restore;
- webhook delivery;
- plugin registration;
- request processing.

Poor fit for purely internal utility/library areas; omit there.

---

# 7. Representations explicitly rejected by default

## C4 System Context

Reserved for future multi-repository/deployment/service-catalog mode or
explicitly supplied system boundary. One checkout cannot normally prove users,
software systems and business relationships.

## BPMN 2.0

Do not emit gateways, pools, message events, compensation or sequence/message
flow unless their exact semantics are independently supported.

## FFBD

Do not emit a functional time sequence when only static declarations/callsites
are known.

## Full call graph

Not a product architecture view. Bounded source investigation may expose a
local call neighborhood, but the default report never renders a repository-wide
call graph.

---

# 8. Repository-shape adaptation

## Service/application

```text
entry surfaces → conceptual components → observed touchpoints
```

## Library/framework

```text
consumer/public API → registration/lifecycle → internal operations
                                     ⋯ external API/resources
```

Do not center the view on process startup when none exists.

## CLI

```text
commands → operation areas → filesystem/network/storage
```

## Monorepo

```text
repository scope
├── application/service A
├── worker B
├── library C
└── shared infrastructure
```

- each app/library is a first-class scope;
- shared infrastructure is visibly shared;
- no flattening into one component landscape;
- cross-scope relations are aggregated and weaker unless exact.

---

# 9. Diagram acceptance tests

## Identity and information preservation

- list, canvas and inspector use the same accepted component IDs/counts;
- changing representation does not change acceptance;
- duplicate/equivalent components are coalesced or shown as conflict/shared
  class, not repeated as useful nodes;
- all underlying witnesses reconcile to visible representative + disclosure.

## Architecture screenshots

At 1280×800, 1440×1000 and 1920×1080:

- principal areas are understandable without opening provenance;
- diagnostic remainder is not a principal node;
- exact directed edges have arrowheads;
- association connectors are visibly weaker;
- no edge looks clickable;
- selected one-hop neighborhood is legible;
- inspector remains readable without hiding the map unintentionally;
- 125% and 150% zoom remain usable.

## Mechanism screenshots

- connected fragments are visually connected;
- independent fragments are separate;
- unknown frontier is obvious;
- no ordinal list implies unsupported sequence;
- touchpoints do not masquerade as next steps;
- raw enum values are absent from primary copy.

## Performance

- layout remains interactive on large etcd/restic canvases;
- no animation/layout thrash while selecting nodes;
- relation aggregation limits SVG element count;
- source drawer and inspector do not create nested-scroll traps.

---

# 10. Failure examples

The diagram implementation fails even if all tests are green when:

- one broad unit is cloned into several apparently distinct components;
- diagnostic remainder becomes a main subsystem;
- a package import looks like runtime execution;
- a root package visually owns all descendant touchpoints;
- Boundary and Resource duplicate one witness as two visible nodes/rows;
- an edge is a primary clickable target;
- disconnected transitions look sequential;
- a zero-relation library shows an empty runtime-style graph;
- the same diagram is forced onto service, library, CLI and monorepo shapes;
- information is deleted merely to improve layout.
