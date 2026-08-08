# 244 — Exact package-member entry-handoff cube join

**Status:** ACTIVE (owner-authorized through the linked report/product review,
2026-08-09)

**Preserves:** Decision 243's D210 authority, Entrypoints overlay semantics,
true Study mechanism contract, calm report boundary, and exact 0/1/N endpoint
presentation.

**Supersedes:** Decision 243 B4/B5 where absence of an accepted callee symbol
member forced an exact repository-local target to remain off-map even when the
accepted Canvas retained its exact package member. It also supersedes D243's
unique-endpoint overlay rule for an entry and target owned by the same cube:
that exact handoff remains joined and highlighted but is compact side detail,
not a self-loop arrow. D243 remains unchanged for process-entry identity, D210
matching, scenario/provenance, distinct-cube arrows, and true Mechanisms.

## Proven product failure

The fresh Age report retained fourteen exact D210 handoffs for
`cmd/age/age.go:main`, including exact callee declaration paths and locations.
The accepted Canvas retained the exact `filippo.io/age/cmd/age` package member
as a first-class cube but did not retain those helper declarations as symbol
members. Every transition consequently published `component_ids: []`, so the
principal executable produced fourteen off-map rows and zero cube arrows.

The browser cannot repair this without inventing a second authority. The saved
RepositoryGraph already owns the exact package file inventory needed for a
backend join.

## Approved correction

1. The accepted symbol-member + declaration-identity + declaration-location
   join remains first and authoritative. When it returns one or more accepted
   non-remainder components, package evidence cannot add another owner.
2. Only when that direct join returns zero owners, the backend resolves the
   exact D210 callee declaration path by equality with an item in
   `RepositoryGraph.packages[].files`.
3. Every exact matching package contributes only its canonical package path.
   A Canvas component owns that package only when one of its `members` or
   `shared_members` is a typed package candidate carrying an exact declaration
   fact equal to that canonical package path.
4. `LocalRemainderComponentID` is always excluded. No component name, package
   name, symbol name, basename, directory/prefix similarity, model prose,
   structural edge, or sorted-first choice participates.
5. Zero exact accepted owners remains `component_ids: []`; plural ownership
   remains sorted, explicit plural and side-lane-only. A unique target distinct
   from the unique entry owner yields a cube-to-cube arrow. A unique target
   equal to the entry owner remains exact joined evidence and component
   emphasis, but renders as compact side detail with closed reason
   `same_component`, never as a self-loop SVG. Input permutation cannot change
   backend bytes.

## Identity and verification

- `EntrypointHandoffGroup`: v1 → v2 because exact persisted derivation changed;
- `ArchitectureCanvas`: v14 → v15 because target component participation
  changed;
- report format: v40 → v41;
- the cube join and `same_component` projection add no typed UI message ID.
  The combined current candidate advances the UI catalog independently to v14
  under the follow-up hard-review truth-copy corrective;
- run manifest remains v13 because its schema and action authority did not
  change. Its report verifier now decodes `repository_graph` and re-derives
  every v2 group from Canvas + D210 grounding + exact package inventory before
  accepting the report bytes.

Historical v1/Canvas14/report40 artifacts fail closed under the current binary;
there is no compatibility reader or migration.

## Acceptance

1. Provider-free fixtures prove direct-join precedence and package fallback
   with unique, plural, zero, remainder-only, wrong-path, wrong-package, and
   name-lookalike controls.
2. Reordering components, members, packages, files, and behavior anchors does
   not change the projected groups.
3. Manifest verification accepts the exact package-bound group and rejects a
   report whose package file inventory drifts while its persisted group does
   not.
4. Focused report tests, `go vet` for the report package, `git diff --check`,
   an exact candidate build, and fresh hosted Age Entrypoints inspection prove
   46 of 47 handoffs mapped exactly: 23 distinct-cube arrows, 23 compact
   `same_component` rows without self-loop SVGs, and one honest off-map
   `LocalRemainder` target. Plural/zero endpoints retain side-detail behavior.

## Provider-free corpus measurement

The read-only re-derivation over all 36 fresh report40 corpus artifacts (35
Canvas14 reports plus Bench without a Canvas) made no provider call and changed
no saved artifact. Twenty-two repositories contained 41 groups / 191 handoffs.
Old ownership was 165 zero, 26 unique, and zero plural targets. The exact v2
derivation yields one zero, 189 unique, and one plural target: 164 previously
empty transitions gain an exact package-member owner. Presentation classifies
117 distinct unique pairs as arrows, 72 same-component joins as compact side
detail, and two side-detail endpoints (one plural, one Age `LocalRemainder`-only
zero). No aggregate or sorted-first repair was applied.
