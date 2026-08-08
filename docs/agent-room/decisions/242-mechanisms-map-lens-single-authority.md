# 242 — Per-entry mechanisms and one Map authority

**Status:** ACTIVE (owner-authorized, 2026-08-08)
**Preserves:** Decisions 226, 230, 236, and 241; Map remains the primary
product surface and `report.html` remains calm user documentation.

## Problem

The report owned exact mechanism evidence but published it through two
incompatible UI paths:

- the Mechanisms lens looked only at `architecture_canvas.flow_edges`;
- a separate below-map disclosure rendered singular `mechanism_fragment`.

Ordinary Architecture does not populate saved flows, so the lens said that no
mechanism existed while the same HTML could show one below. The singular
producer had a deeper loss: it returned no fragment whenever the repository had
two or more exact process entries. That left sqlc, Maddy, PocketBase, etcd, and
other multi-entry repositories empty even though local grounding already owned
exact entry identities and first-hop handoffs.

The same hosted-report review found a second competing Map authority:
drawable `architecture_canvas.structural_edges` were visible on the canvas,
while a legacy grounding-relation disclosure below it claimed no component
edge had been accepted.

## Approved correction

### Backend

1. `ArchitectureCanvas` owns bounded `mechanism_fragments`: one deterministic
   fragment for every exact process-entry anchor with at least one supported
   first-hop handoff. Zero-hop entries remain first-class in Entrypoints but
   are not mislabeled as mechanisms. There is no global "first" entry and no
   model selection.
2. Each fragment has a stable backend ID and sorted exact component IDs. Entry
   and handoff targets join only through exact backend member identity;
   diagnostic remainder is never a participating component. An unjoined
   handoff remains visible as exact evidence but never causes a guessed
   component link in JavaScript.
3. Mechanism fragments contain only the exact entry, sibling first-hop direct
   static handoffs, exact callee declaration targets when the accepted Canvas
   or grounding owns one, and unresolved frontier. A missing declaration join
   never becomes a guessed source target. Canonical display order is never
   runtime order and sibling calls are never chained into a path.
   Boundary/resource associations remain owned by the Integrations lens and
   are not duplicated once per entry. Frontier is not double-counted as a
   transition.
4. Shared entry participation aggregates exact component scope; it never picks
   a sorted component as a hidden representative.
5. Projection and validation are provider-free, deterministic, bounded, and
   re-derived from the accepted Canvas and grounding.

### Publication

1. The Mechanisms lens reads only `architecture_canvas.mechanism_fragments`.
   `flow_edges` remain available to saved-flow focus, but are not a second
   Mechanisms authority.
2. Exact fragment `component_ids` own Map emphasis. A fragment with no joinable
   component stays readable without dimming or highlighting a guessed node.
3. The existing evidence renderer lives inside the Mechanisms lens. The old
   below-map mechanism disclosure is removed. With multiple entries, compact
   entry disclosures keep the Map primary and reveal each full fragment on
   demand.
4. The empty state appears only when the backend publishes zero current
   fragments.
5. Drawable singular Canvas structural edges are the Landscape relation
   authority. Legacy grounding anchor relations no longer render as a competing
   component-relation inventory, and no diagnostic/unprojected relation block
   is repeated below the Map.

## Identity and history

- MechanismFragment projection: v2 → v3;
- ArchitectureCanvas: v12 → v13;
- report format: v38 → v39.

The typed UI message catalog advances v11 → v12 for closed EN/RU first-hop
copy. The manifest version, Atlas Study projection, provider prompts, semantic
caches, and request/result schemas do not change. Historical
formats fail closed under the current binary; their source history remains in
git and already-rendered self-contained HTML remains unchanged.

## Boundaries

- no semantic/provider call, retry, or flag;
- no new SSA build, repository traversal, or source selection;
- no client-side path/package/symbol-name inference;
- no whole-repository call graph, BPMN, FFBD, or invented execution order;
- no warning/diagnostic archaeology added to `report.html`.

The linked review's proposed focused retrieval of deeper 1–2-hop mechanisms
for selected Study questions remains a separate future experiment. D242 only
publishes exact evidence repomap already owns.

## Acceptance

1. Backend: multiple process entries produce stable per-entry fragments under
   input permutation; exact entry/target component joins revalidate; drift and
   old versions fail closed.
2. Pure lens: fragments are the sole mechanism input; adversarial
   `flow_edges` cannot change fragment objects or emphasis.
3. Asset/DOM: selecting Mechanisms renders every published entry, handoff, limitation,
   source action, and frontier; no false empty state, no duplicate
   disclosure, no guessed highlight, and no canvas transform write.
4. Hosted real HTML: serve a report generated by the updated binary and verify
   the lens with browser automation on a multi-entry repository plus an honest
   zero-fragment control.
5. Focused tests, current report/manifest gates, `go test -count=1 ./...`,
   `go vet ./...`, `git diff --check`, exact binary build, and fresh ordinary
   repository acceptance pass before commit and push.
