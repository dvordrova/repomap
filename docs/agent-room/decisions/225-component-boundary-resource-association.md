# Decision 225: Component↔boundary/resource association from exact local data

## Status

ACTIVE (Phase 3 of the overnight program
`hermes-repomap-overnight-goal-v3.txt`, approved by the repository owner's
standing goal authorization 2026-08-05). Provider-free; no new semantic
stage; no graph redesign; the existing deterministic layout engine and the
existing truth-first structured list remain the defaults.

## Problem

The Architecture report shows components (model-assisted, separately
labeled membership) and a Repository Atlas with locally observed boundary/
resource entities, exact observations and exact callsite evidence — but the
UI does not answer "which of my components actually contains code that
touches external/state surfaces". Reviewers previously had no supported
association; a naive approach would claim runtime dependencies
("depends on PostgreSQL") that local data cannot prove.

## Proven join (from existing exact data, Archive 6, all four runs)

- Component members carry exact package paths: `members[].facts[].value`
  is the canonical package path for package/unit members (e.g.
  `github.com/casdoor/casdoor/service`) and symbol identities for symbol
  members (e.g. `github.com/casdoor/casdoor.main`).
- Atlas units carry the same canonical package path in `name`.
- Boundary/resource observations bind an entity to an exact unit:
  `observations[].unit_id` → `units[].name` (package path), plus
  `observations[].subject` (entity id, kind boundary|resource).
- Exact callsites are the evidence: `evidence[].location.path:line:column`
  + `symbol` + `provenance`.
- Deterministic match: an observed boundary/resource callsite belongs to a
  component's exact member scope when the observation's unit package path
  equals a member package path or lies under it (prefix). The scope set is
  the exact package-path values of package-member `FactDeclaration` facts;
  symbol/file/flow member identities are excluded (they cannot equal or
  prefix a unit package path at a `/` boundary).

Archive 6 match results:
- Casdoor: 218/218 observations in component scopes (17 boundary, 22
  resource entities; 143 evidence).
- etcd: 164/164 (96 entities; 18 structural relations).
- Restic: 60/60 — "Границы безопасности" is the repository-root scope
  (`github.com/restic/restic`) and legitimately covers every observation;
  the card shows counts + omissions, never a flood.
- Telebot: 8/8 in "Телебот", 0 supported component relations — stays
  conceptual/list-first, never a fake runtime diagram.

## Association contract (what may be stated)

- Only: "an observed boundary/resource callsite occurs in an exact member
  scope of this component" (or equivalent).
- Explicitly NOT stated (no local producer proves them): runtime
  dependency, ownership, reachability, read/write semantics, transaction
  semantics, endpoint identity, table/topic/bucket identity, execution
  order.

## Scope (exact changes)

### Backend (deterministic association view-model)
- Compute, per accepted component, the exact set of member package scopes.
- Join Atlas observations to component scopes by exact unit path (equal or
  prefix), deterministically, from the same canonical data the report
  already carries. No model call, no new stage.
- Produce per-component association rows, each row = one boundary/resource
  entity kind + imported API/package family (derived deterministically from
  the exact import path already recorded in `evidence[].provenance.detail` —
  stdlib first segment or external module root) + owning exact unit + exact
  witness list (path:line:symbol, bounded) + observation/omission counts.
- Complete counts/omissions: no first-N silent loss; the card states how
  many observations fall in scope and how many were not associated (none
  for matched; any unmatched are listed as omissions with the honest
  reason).
- Canonical Atlas IDs remain private: association rows carry display-safe
  package paths and evidence locations only.
- Optional same-observation Boundary↔Resource pairing: when a boundary
  observation and a resource observation of the same unit share the same
  evidence ref (`evidence_refs` → same evidence id, hence same callsite
  location), present them as one paired row with a distinct pair class —
  still without semantic claims.

### Architecture interaction (node-centered, truth-first)
- Default remains the structured list plus optional canvas; nothing is
  replaced.
- Selecting a component focuses one-hop supported structural neighbors
  (incoming/outgoing distinguishable) and its exact observation
  associations; unrelated nodes dim; node positions stay stable; second
  hop is explicit; edge geometry stays deterministic through the existing
  layout engine.
- Edges remain passive visual evidence: no edge click requirement, one
  aggregated line per pair, no parallel read/write/invoke lines, no verbs
  on every edge; exact/partial/association meaning uses line style plus a
  visible legend and card/list text — never color alone. Tooltip may show
  type/witness count/source/limitation, but the same information is
  available without hover.
- Connection rows — not edges — are the primary controls: click/tap a row
  to highlight the corresponding node and passive line, expand exact
  witnesses and limitations in place, exact source in no more than two
  actions.

### Selected component card (answers eight questions)
1. what this area is responsible for (existing description);
2. grouping authority, member evidence composition and coverage (member
   counts by kind, provenance, accepted/partial);
3. how work enters when exact surfaces/handoffs exist (existing);
4. supported structural neighbors (incoming/outgoing rows);
5. observed external/state callsites in exact member scope (new rows);
6. relevant source-grounded Study themes (existing theme refs);
7. 3–5 typed places to start reading (exact readings);
8. what remains unknown/unclassified/unresolved (NEW component-card unknowns
   section: association limitations, unassociated observations with reasons,
   and unclassified member state — flow-level unknowns are separate and
   already rendered on study flow cards).

### Boundary/resource card (restricted to what local data proves)
- broad observed class (boundary|resource);
- imported API/package family (derived deterministically from the exact
  import path already recorded in `evidence[].provenance.detail` — stdlib
  first segment or external module root);
- owning exact package/unit;
- exact callsites (path:line, enclosing symbol);
- observation and omission counts;
- exact associated component member scopes;
- explicit limitations: physical target unknown; runtime reachability not
  proven; read/write/order semantics not proven.

## Repository-shape acceptance

- Casdoor: storage/network observations discoverable from component/card
  context without "depends on PostgreSQL" claims.
- etcd: remains bounded; partial coverage visible (fallback local
  architecture keeps its honest label).
- Restic: broad callsites (60 in repository-root scope) do not flood the
  default map/card — counts and omissions render, witnesses expand in
  place.
- Telebot: zero supported component relations stays conceptual/list-first,
  not a fake runtime diagram.
- Library/monorepo semantics are not forced into startup-first wording.

## Provider-free acceptance

1. Unit tests: association join over Archive 6 casdoor/etcd/restic/telebot
   report fixtures → exact match counts (218/218, 164/164, 60/60, 8/8),
   zero silent loss, omissions listed.
2. DOM tests: component card renders association rows with witness list +
   limitations; connection row click highlights node + passive line;
   boundary/resource card shows only allowed fields; exact source in ≤2
   actions; EN/RU parity.
3. Full gates: gofmt, `go vet ./...`, `go test -count=1 ./...`, `make
   build` → `.bin/repomap`, node --check on touched assets, report/manifest
   round trip, golden regen.

## Non-goals

- No runtime dependency/ownership/reachability claims; no read/write/
  transaction/endpoint/table claims; no graph redesign; no edge-click
  requirement; no verbs on edges; no color-only semantics; no new semantic
  stage; no third call; no push.
