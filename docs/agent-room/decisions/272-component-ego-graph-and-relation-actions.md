# 272 — Component ego-graph and relation actions

**Status:** ACTIVE (owner-authorized white-box UI corrective, 2026-08-10)

**Preserves:** Decision 222 exact structural relation authority, Decision 229
selected-component focus, Decision 268 hover behavior, Decision 269 target
pages, Report45, Manifest17, Canvas15, UI24, every persisted schema and every
provider/backend contract.

## Product defects

Selecting one component left every structural line on the Landscape at low
opacity. In a relation-dense target such as etcd, unrelated global lines still
crossed the focused neighborhood and the selected component's incoming and
outgoing direction was not visible on the lines themselves.

The component inspector rendered `Used by` and `Uses` neighbors as bordered
cards, but the ordinary user-mode mount supplied no external component callback.
The cards therefore looked actionable while offering no navigation. Large
neighbor and operation sets also formed one unbounded wall. An operation with
no exact source action used the same card affordance as an actionable exact
source row.

## Approved corrective

The neutral Landscape still renders the complete saved structural graph. Once
one component is selected, only structural edges incident to that exact
component remain visible; every unrelated edge is suppressed without deleting,
reordering, rerouting or reinterpreting saved data. Structural edges carry an
arrow at their exact `to_component_id`. Unrelated component cards remain in
their original positions, and their opaque card plane masks a local line that
passes behind them while their content remains subdued.

Every known neighbor in `Used by` and `Uses` is one real button. It calls the
canvas's own exact component navigation, updates selection, local edges and the
inspector, keeps the Connections tab active for continued traversal, and
focuses the already-laid-out neighbor without relayout. No name, path or sorted
fallback restores identity.

Each relation or operation section renders the first four backend-ordered rows
and retains every remaining exact row in one native disclosure with a truthful
remaining count. This is presentation bounding only: no item is ranked,
dropped, merged or hidden from the DOM. Exact operation locations remain source
buttons. An operation without an available exact source action is plain
non-card text, never a dead control.

This adds no copy, route, report field, data projection, provider call or
compatibility reader. UI catalog identity remains UI24.

## Acceptance

- Selecting a component in a 12-edge graph shows its ten exact incident edges
  and hides the two unrelated edges; selecting a neighbor recomputes the local
  graph to that neighbor's two incident edges.
- Every rendered structural edge has a visible direction marker.
- Ten exact incoming/outgoing neighbor rows remain present as ten buttons,
  bounded by two truthful disclosures, and clicking one changes the selected
  component while retaining the Connections tab.
- Six operations with exact locations are source buttons; one operation without
  an exact action is plain text; all seven remain present behind the same
  four-plus-disclosure rule.

Approved by:
    Repository owner during direct review of the fresh multi-target etcd
    server report, including the selected `Storage and WAL` component and its
    Connections inspector, 2026-08-10.
