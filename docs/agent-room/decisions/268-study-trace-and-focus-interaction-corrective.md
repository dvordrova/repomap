# 268 — Study trace and focus interaction corrective

**Status:** ACTIVE (owner-authorized white-box UI corrective, 2026-08-10)
**Preserves:** Decision 256 source-trace authority, exact source actions,
Decision 229 selected-component focus semantics, UI23 target workspace,
report44, Canvas15, Manifest16, persisted schemas and every provider contract.

## Product defects

The Decision 256 renderer consumed mechanism edges in pairs. The second edge
was represented only by a clickable arrow, so its callsite line disappeared;
an odd-length path happened to expose the final edge while an even-length path
did not. The saved mechanism remained complete, but the visible source trace
was not.

Selected-component focus also left the generic component-card hover elevation
active on dimmed unrelated components. Moving the pointer across the canvas
could therefore flash a border and shadow on content that focus mode had
deliberately de-emphasized.

Finally, the two-column Study table of contents remained two columns below the
640 px workspace breakpoint, competing with the persistent target rail and
risking horizontal overflow.

## Approved corrective

The Study source trace renders exactly one row for every persisted mechanism
edge. Each row exposes the edge's visible exact callsite line and the exact
callee declaration as independent existing source actions, with the invocation
kind attached only to that edge. The shared root declaration plus one callee
label per edge forms one linear source trace. No edge is paired with or hidden
inside the following edge's arrow.

Component hover elevation applies only to non-dimmed components and the
selected component. A dimmed unrelated component does not gain a hover border
or shadow. Keyboard `:focus-visible` remains independently visible so the
corrective does not remove accessible focus feedback.

At the existing 640 px breakpoint, the Study table of contents becomes one
column in the same DOM and tab order and is width-bounded by its container.

This adds no copy, route, source mode, provider call, report data, identity or
compatibility reader. UI catalog identity remains UI23.

## Acceptance

- A four-edge mechanism renders four visible callsite-line actions and five
  function labels including its shared root.
- Style contracts cover non-dimmed hover, selected hover, dimmed non-hover and
  keyboard focus-visible as separate states.
- The 640 px rule sets the existing Study contents list to one column without
  reordering or duplicating navigation actions.

Approved by:
    Repository owner during white-box review of the fresh Casdoor report,
    including the reported flashing border while moving across components,
    2026-08-10.
