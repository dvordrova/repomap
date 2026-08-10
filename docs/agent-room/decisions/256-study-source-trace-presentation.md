# 256 — Study source-trace presentation

**Status:** ACTIVE (owner-authorized presentation corrective, 2026-08-09)
**Preserves:** Decision 246 investigation v1 authority, exact source actions,
the transient Landscape overlay, report42, Canvas15, Manifest14, provider
identity, persisted schema and backend mechanism depth.

## Product defect

The Study detail rendered each exact mechanism as a horizontal train of node
cards and transition cards. Sibling mechanisms repeated their common root,
generic `Path N`, witness counts and synchronous-call labels competed with the
code, and deeper paths forced horizontal scrolling. The saved investigation
already contained enough exact declarations and callsites; the defect was its
presentation, not missing analysis.

## Approved presentation

The browser groups sibling mechanisms only when their first node has the same
exact declaration location. That common root is shown once as a source header:
repository-relative file path plus declaration line and function. Mechanisms
inside the group are ordered by their first exact callsite location solely as
source order; this is not execution order.

Each mechanism is a compact source trace. A row starts with the exact callsite
line, then the exact callee declaration. When the next direct edge exists, its
arrow owns that inner exact callsite and is followed by the next exact callee.
Additional depth continues on subsequent rows rather than widening a card
train. Every line, callee and callsite arrow is an independent existing source
action. Static text remains useful when an action cannot be opened.

Synchronous calls receive no visible chip. Goroutine and deferred invocation
labels appear only on the exact edge where that distinction exists. Witness
counts, `Path N`, branch terminology and node/transition cards are absent. One
subtle Show-on-map action remains for each exact mechanism.

The caption says that the rows are direct calls in source order and are not a
runtime trace. UI catalog identity advances from UI20 to UI21. No report,
manifest, provider, investigation, source-action or map-overlay identity
changes.

## Acceptance

A causal DOM fixture publishes two mechanisms with one exact `runDevUICLI`
root in reverse source order. The rendered detail has one root header, two
mechanisms ordered by first callsite, and a two-edge row with exactly four
independent source actions: line, callee, inner-callsite arrow and next callee.
It contains no witness, synchronous, path-number or card-train presentation.

Approved by:
    Repository owner after direct inspection of the current repomap Study
    mechanism and ten presentation alternatives, 2026-08-09.
