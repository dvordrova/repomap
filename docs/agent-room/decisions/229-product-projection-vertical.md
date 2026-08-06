# Decision 229: useful projection of existing truth (product projection vertical)

## Status

ACTIVE (product projection + information-salvage vertical, goal
`hermes-repomap-product-projection-goal-v5.txt`; hard acceptance contract
`repomap-information-preservation-review-charter-v5.md`; interaction reference
`repomap-product-projection-design-contract-v4.md`).

## Problem (baseline evidence, real Casdoor report 20260806-041149)

The report is technically honest but dumps internally true information at once
and rejects useful semantic results over small defects:

1. Architecture canvas edges are interactive: `<g role="button" tabindex="0">`
   hitboxes; edges are primary controls. Contract: edges are passive visual
   evidence; nodes and connection rows are the primary controls.
2. "Supporting repository evidence" renders as a principal architecture
   component. Contract: unclassified exact scope must be a collapsed
   diagnostic disclosure, never a principal product area.
3. Raw contract enums ("CONFIGURES_SECURITY_BOUNDARY", "STATIC_CALL_…",
   "direct_static_call", "resolved_static") appear as primary user copy.
   Contract: human language in primary UI, raw enums under "Evidence details".
4. Whole-proposal rejection on `duplicate_component_identity` discarded
   valid semantic siblings on the casdoor run `20260806-041149` (56 distinct
   members / 124 occurrences). The shared-membership class is already
   resolved at HEAD by D227 (participation; exact twin only rejects); the
   remaining salvage gaps are the D0/D1/D2 normalization classes and the D4
   exact-twin/equivalent-collision class, which still rejects the whole
   proposal.
5. Boundary and Resource observations sharing one exact witness render as two
   user rows. Contract: one visible "Observed external/state interaction" row;
   canonical records stay distinct.
6. Study themes can read as warnings; evidence badge and scope badge are
   conflated; expanded-by-default details.
7. Mechanism fragment shows one numbered vertical list; disconnected evidence
   must stay separate fragments; raw enums must not be primary copy.
8. Mobile: the inspector can be nested inside a canvas host that is hidden or
   unusable on small viewports; mobile PASS must not mean "no horizontal
   overflow".

## Candidate shapes (PHASE 1 of the goal)

- A. CSS-only polish — REJECTED: the largest failures are projection and
  interaction (edge click contract, remainder as principal node, enum walls,
  two-rows-per-witness), not color.
- B. Deterministic view-model + interaction correction — SELECTED: bounded
  projection changes over existing truth, no new producer.
- C. Broad new analyzer/semantic redesign — REJECTED: no evidence that a new
  producer is indispensable; all acceptance is reachable from existing exact
  facts.

## Scope

Deterministic report view-model + Overview/Study/Architecture UI (the
Mechanism evidence view stays inside the Architecture workspace — exactly
three top-level jobs preserved: Overview, Study, Architecture).
No semantic call, no frontend framework, no new analyzer, no provider stage.

## Decisions

### D1. Passive edges, node/row-centric interaction (Architecture)

- Canvas edges: `pointer-events: none`; remove `role="button"`, `tabindex`,
  edge hitboxes and the edge click contract. Optional `<title>` tooltip may
  describe a line; identical information exists in the selected-node panel.
- Closed line vocabulary: solid directed line + arrowhead = exact local
  handoff/relation; thin neutral solid = static structural relation when
  product-relevant; dotted non-directional = "observed in exact member
  scope", never runtime dependency; dashed directed = unresolved/partial
  continuation; enclosure/badge = model conceptual membership, never a
  relation edge. One visible connector per node pair; no parallel
  read/write/invoke lines; no verb printed on every edge.
- Node click and connection-row click are the primary controls. Selecting a
  connection row expands exact witnesses + limitation in place and highlights
  the corresponding passive connector.
- Selected-node focus: one-hop incoming/outgoing exact neighbors stay
  prominent; unrelated principal nodes dim but remain spatially stable;
  second hop requires an explicit action; clicking another node recenters.
- Selected-node inspector answers the nine questions in fixed order:
  responsibility; grouping authority / evidence composition / coverage as
  independent axes; how work enters; used by; uses; observed external/state
  interactions; relevant Study themes; read first — 3–5 typed exact source
  starts; what remains unknown. A heading does not count as an answer; every
  populated section carries component-specific content or a truthful
  empty/unknown explanation.

### D2. Precision tiers and boundary/resource coalescing (Architecture)

- Closed presentation precision, derived deterministically:
  `exact_symbol_scope | exact_file_scope | narrow_package_scope |
  broad_package_scope | diagnostic_remainder`. Never infer stronger precision
  from prose.
- Default: exact symbol/file/narrow-package rows show as normal rows; broad
  package scope collapses under "Additional observations from broad package
  membership · N"; diagnostic remainder never dominates a principal card.
- Boundary and Resource sharing the same exact witness project as ONE visible
  "Observed external/state interaction" row until a distinct real resource
  identity exists; canonical records remain two and are preserved internally.
- Rows say "observed in exact member scope" or the exact weaker precision;
  never "depends on / calls at runtime / owns resource" unless independently
  proven.

### D3. Diagnostic remainder is not a principal area (Overview + Architecture)

- "Supporting repository evidence" → collapsed diagnostic disclosure
  "Unclassified exact scope · N packages". Never a principal product area,
  never a principal map node. Remainder stays reachable and count-reconciled.
- Repository-perimeter projection (not C4): observed use/entry → analyzed
  repository scope ⋯ observed touchpoints.
- Snapshot freshness and dirty working-tree state are separate axes:
  "Snapshot current" and "local changes present" may coexist; never render a
  dirty checkout as "clean repository".
- Hero is a concise useful summary; capability-list README prose is
  supporting source material, not the whole hero.
- Primary entries pass a deterministic presentation-quality gate: `amount`,
  `payer`, `application_context`, `unresolved value`,
  `result of strings.TrimSpace`, `result of fmt.Sprintf`, callback-locals
  and other value-shaped names never appear as a primary surface title.
- Dynamic route/frontier observations stay behind a bounded disclosure; they
  never compete with exact process/server/command/public API entries.
- Duplicate generic "HTTP server" rows are aggregated or explained by exact
  owner/location; no indistinguishable cards.
- One explicit Start Here action chosen through existing role/evidence rules,
  not sorted-first.
- Source-grounded learning lenses are shown separately from Architecture
  areas and labeled Study/learning lenses, never components.

### D4. Human copy for primary UI, enums under Evidence details (all)

- Mechanism primary copy: "Direct static call", "Resolved in the recorded
  build", "Local callsite order", "Continuation not recovered"; raw enums
  (`direct_static_call`, `resolved_static`, `resolved_path_order`,
  `not_established`) remain under "Evidence details".
- Relations inventory: human group labels; raw relation kinds under details.

### D5. Mechanism fragments never linearize disconnected evidence

- A path is drawn only when source, target and local transition are
  connected. Casdoor: Fragment A (main.go:36 main → main.go:150 service.Start
  → continuation unknown) and Fragment B (ldap/server.go:61 →
  ldap.getTLSconfig) render as SEPARATE fragments. Never
  main → ldap.getTLSconfig → service.Start.
- View-model carries explicit endpoints (source, target, support, ordering,
  exact evidence, limitation) per transition.

### D6. Study calm progressive disclosure

- All published theme titles remain visible; cards collapsed by default; at
  most two reading previews per collapsed card; opening one card does not
  expand siblings; back/forward and focus stable.
- Evidence badge and scope badge are independent axes ("Source-backed" not
  contradicted by "Scope partial"); a narrow-but-exact theme is not a warning.
- Repeated public symbols group visually ("HttpEmailProvider.Send · 2
  callsites") while every exact source location stays in the expansion.
- Exact source is reached in no more than two actions on desktop.

### D7. Semantic salvage contract (Architecture proposal validation)

Implement the charter duplicate taxonomy and smallest-scope fail-closed.
In this section, D0–D8 are the charter-v5 duplicate-taxonomy codes, not the
D1–D9 decisions of this document:

- D0 exact replay duplicate → idempotent dedup, counted.
- D1 duplicate within one component → local dedup, keep strongest, keep
  component.
- D2 same public identity, different support → direct wins default row;
  theme/component survives.
- D3 same member in multiple components → cross-cutting participation
  (already accepted by D227) or explicit shared-membership diagnostic;
  never whole-stage rejection.
- D4 equivalent component collision → coalesce/quarantine the collision class
  only; unrelated components publish. This supersedes the D227 exact-twin
  hard rejection: two components with identical resolved membership and
  semantic content coalesce deterministically (alternate labels/descriptions
  retained as provenance) or quarantine as one conflicted class; components
  sharing a member set under distinct names/descriptions remain D227
  participation and are never coalesced or quarantined.
- D5 response-local ID reused for incompatible objects → reject only objects
  using the ambiguous ID; retain the rest.
- D6 Boundary/Resource same witness → canonical pair, one product row (D2).
- D7 multiple relations between same visible nodes → one aggregated
  connector, all witnesses retained.
- D8 same label distinct identities → never dedup by label; group with count.
- A component referencing an unknown member/anchor ref, or a malformed
  member/type, is rejected item-scope with the exact reason counted (charter
  containment table, Item row); all other components publish as
  `accepted_partial`; whole-stage rejection only when zero independently
  valid items remain. Unknown-ref/type/source validation itself is not
  weakened.

Outcome ladder: accepted / accepted_with_normalization / accepted_partial /
local_only / terminal. `terminal` only when local authority is unsafe/corrupt/
stale. Zero valid semantic items → local_only product, never fact-ledger loss.

### D8. Mobile functional journeys

- 390×844: same five Overview answers reachable; no horizontal overflow;
  Architecture inspector opens as a visible bottom sheet / independent host
  (not nested inside a canvas element hidden on mobile); close/back works;
  Study expand + source in ≤2 actions; Mechanism fragments stack without
  implying extra order.

### D9. Visual system

Calm slate/navy shell; white surfaces; restrained blue nav/selection; teal
touchpoints; amber unknown/frontier; violet conceptual/editorial; limited
shadows, 12–16px radii; dense detail only after disclosure; no rainbow
ontology; no machine-enum walls; no giant equal-height empty cards; stable
alignment and generous whitespace; mono only where useful. No framework
rewrite; existing template/JS/CSS render contracts.

## Non-goals

- No new provider call, analyzer, frontend framework, search, database,
  embeddings, Tree-sitter, language adapter, SSA traversal.
- Do not weaken unknown-ref/type/source validation.
- `--no-secrets` stays the owner-selected acceptance option, unchanged.

## Acceptance (summary; full matrix in the goal)

- All charter metamorphic tests: permutation invariance, duplicate injection,
  sibling poisoning, cross-cutting membership, equivalent collision,
  boundary/resource co-projection, edge aggregation, projection invariance,
  monotonic evidence, zero-valid → local_only.
- Overview 5 answers at 1440×1000 and reachable at 390×844.
- Study: 11 Casdoor theme titles, collapsed cards, ≤2 previews, one-open.
- Architecture: passive edges, arrowheads, remainder outside principal graph,
  precision tiers, B/R coalescing, 9-question inspector, mobile sheet.
- Mechanism: separate fragments, human copy, frontier visible.
- EN/RU parity; no English enum leakage into RU primary UI.
- Gates: focused + full tests, node --check, gofmt, vet, quality-check,
  localization-check, surface-check, exact trimpath binary, real browser
  journeys at 1440×1000 and 390×844, fresh reviews (code/semantic/product/
  a11y), MORNING-229 report.

## Commit policy

Decision checkpoint commit only after decision PASS (update CURRENT.md to
point at 229 in the same decision checkpoint); coherent implementation
commits; no amend/rewrite/push.
