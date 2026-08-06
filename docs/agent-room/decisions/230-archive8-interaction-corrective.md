# Decision 230: desktop interaction, source-reference and canvas corrective (Archive 8)

## Status

ACTIVE (interaction/diagram corrective, goal
`hermes-repomap-archive8-interaction-canvas-goal-v9.txt`; hard acceptance contracts
`repomap-information-preservation-review-charter-v5 (2).md`,
`repomap-architecture-distinctness-salvage-contract-v6.md`,
`repomap-diagram-representation-contract-v8.md`).

## Problem (baseline evidence, Archive 8 real-browser audit, all reproduced on local HEAD)

The desktop interaction layer is not accepted. Every quoted measurement was
re-verified in a real Chromium session at 1440×1000 on the Archive 8 reports:

1. **Source-looking text is inconsistent and often inert.** Study preview
   symbols/paths are inert in all five reports (casdoor 23/23, etcd 23/23,
   telebot 12/12, chatto 17/17, restic 22/22); Study detail symbol is a link
   but the adjacent path:line is plain text; Architecture relation paths
   (`ldap/server.go:61`, `main.go:150`) and mechanism paths are plain text;
   the Overview "Open first" button has no hover state and its visible
   `main.go:36` line is inert.
2. **Canvas geometry is broken.** Casdoor primary node «Основное приложение»
   (x=395, y=233, 340×132) has its center OUTSIDE the canvas viewport
   (y=466); `elementFromPoint(center)` returns
   `rm-architecture-truth-strip__item` (intercepted), and `.rm-arch__flows`
   (y=353, h=45) covers it; only 8 of 19 casdoor component centers pass the
   initial hit-test. "Fit" applies scale 0.65 and moves the primary node
   farther outside (y→162). Do not label a control Fit when it leaves
   principal nodes outside.
3. **Wheel trap.** Ordinary wheel inside the canvas changes scale
   (0.65→0.543) while page scroll stays 0. Contract: ordinary wheel scrolls
   the page; Ctrl/Cmd+wheel zooms; +/- zoom; drag blank canvas pans; drag
   starting on a node/control does not pan.
4. **Inspector leaks scroll and focus.** Inspector is a fixed overlay with
   `overscroll-behavior: auto` (page behind moves at the bottom); close
   button drops focus to `<body>`; tab continues through covered map nodes.
   Contract: docked nonmodal inspector (preferred) or overlay drawer with
   dialog semantics, `overscroll-behavior: contain`, focus enter/return.
5. **Invalid nested interaction.** `<button class="rm-arch__association-row">`
   contains nested witness `<button>`s (23 rows × up to 4). Witness click
   opens the source AND collapses the parent row, focus falls to `<body>`.
   Contract: valid disclosure structure; child source click never toggles the
   parent; expanded state, selected component, map transform and page scroll
   are preserved.
6. **Architecture is still a 9k–12.6k evidence wall**
   (casdoor 12,664px, etcd 10,353px, restic 9,480px in the Archive 8 audit;
   re-verified at local HEAD as 12,498px / 10,291px / 9,314px) because
   relation lists, a 34–48 item mechanism lane stack and the full component
   list are appended below the map. Contract: bounded workspace with explicit
   sections `[Map] [Relations] [Mechanism fragments] [Component list]` and
   compact default counts.
7. **Mechanism is an ordinal list, not connected fragments.** 47 casdoor
   transitions render as one vertical story; array order is visually dominant;
   boundary/resource observations read as "next steps". Contract: connected
   fragments with explicit source/target endpoints; adjacency only when
   `target(previous) == source(next)`; disconnected evidence becomes separate
   fragments or side touchpoint groups.
8. **Remaining data defects damage the diagrams:** etcd whole-response
   rejection under `proposal.unknown_anchor_id` with English fallback labels
   in RU; equivalent resolved member sets in telebot/chatto/restic publish as
   independent components; casdoor «Запуск» inherits 218 observations through
   broad/root association scope. Salvage-contract v6 and charter v5 govern.

## Decision

One coherent product rule:

> every visible technical object has a truthful, discoverable and stable
> interaction; the canvas is navigable without trapping or occluding the user.

Candidate A (CSS-only hover polish) is rejected: geometry, nested actions,
wheel trapping and source navigation are behavioral. Candidate C (new
analyzer/semantic redesign) is rejected: existing data is sufficient and the
DO-NOT-ADD list forbids new providers/stages.

Scope (presentation/view-model corrective using existing data; no new
provider, no new semantic stage):

### D1. Unified source-reference contract (PHASE 3)

One deterministic presentation contract for every exact source reference:
symbol/label + path + line/column + authority + action availability +
destination or unavailable reason.

- Static report: one `<a>` with pinned revision, exact path/line,
  `target="_blank"`, `rel="noopener noreferrer"`.
- Served report: one button/link through the existing source action/drawer
  contract.
- Unavailable: neutral non-link styling with visible reason, no pointer
  cursor.
- Applied consistently to Overview Open First, Overview object cards, Study
  collapsed previews, Study detail readings, Study frontier browse,
  Architecture relation list, inspector read-first rows, association
  witnesses, mechanism fragments, component list, package rows, surface
  catalog.
- Study collapsed card: card is not a global click target; title/header has
  explicit "Open theme" affordance; each preview symbol+path is ONE
  independent source action; at most two previews; source click does not open
  theme detail; title click does not open source.
- Study detail: symbol and path:line form one source action.
- Overview Open First: one coherent action row/card with action label AND
  path; no primary button plus inert duplicate source text.
- Relation/mechanism: every exact path:line actionable; raw enum under
  Evidence Details.
- Acceptance: for every source-looking element in all five reports, a
  clickable target exists and is correct OR an unavailable state/reason
  exists. No unexplained inert path/symbol. DOM tests enumerate
  source-looking projection nodes and verify the invariant.

### D2. Interaction signifier system (PHASE 4)

Closed patterns: whole-card action (one anchor/button, hover/focus state,
visible label, no interactive descendants), multi-action card (noninteractive
container, no whole-card hover, per-action signaling, whitespace has no
action), disclosure (`<details>/<summary>` or sibling toggle; count + chevron;
truthful aria-expanded; child source actions siblings of content, not nested
inside a button; child click does not toggle parent; expanded state remains;
focus meaningful), static card (no pointer cursor, no hover elevation), and
buttons/links with visible hover AND focus states.

Replace the invalid association DOM (button containing witness buttons) with a
valid disclosure structure. After a witness source action: association remains
expanded, selected component remains selected, map transform unchanged, page
scroll unchanged, focus does not fall to body. Regression test: `button
button`, `button a`, `a button`, `a a` must be zero in every expanded tested
state.

### D3. Canvas geometry, fit and scroll (PHASE 5)

- Initial layout: no principal node outside the canvas clip while appearing
  part of the initial map; no toolbar/overlay covers a component center;
  center hit-test returns that component; deterministic ELK layout preserved.
- Fit contract: show every principal node/group inside the viewport with
  semantic zoom (low scale: title/count only; readable scale: description/
  meta). Alternative: two explicitly named controls "Overview" (all principal
  areas) and "Readable scale" (larger cards with panning). A control labeled
  Fit must not leave principal nodes outside. Diagnostic/unclassified nodes
  may stay outside the principal fit only when a clear disclosure exists;
  they are never silently clipped.
- Edges remain passive: no role/tabindex/hitbox/click handler is
  reintroduced on Architecture edges (audit §11 must-not-regress; goal
  PHASE 11 "passive edges" test).
- Wheel: ordinary wheel scrolls the page; Ctrl/Cmd+wheel zooms; +/- zooms;
  Fit/Overview resets; drag blank canvas pans; drag starting on node/control
  does not pan.
- Toolbar outside the canvas clip; no redundant single-option button when no
  alternative mode exists; clear hover/focus/pressed feedback.
- Large maps (casdoor/etcd/restic) usable at 125% and 150% zoom: bounded
  SVG/DOM, stable positions, no layout thrash on selection, no hidden
  unreachable principal node, relation aggregation.
- Browser hit-test assertions after initial render, Fit/Overview, zoom, pan.

### D4. Inspector and scroll context (PHASE 6)

Docked nonmodal inspector preferred when viewport width permits:
`area list | map | inspector`; map resized rather than covered; logical tab
order; inspector scroll contained; selected node remains visible. Overlay
drawer only when necessary: dialog semantics, focus enters close/first
control, Escape closes, focus returns to triggering component, background
pointer/keyboard policy explicit, `overscroll-behavior: contain`, page behind
does not move at inspector scroll boundary. Never combine a visually modal
overlay with keyboard focus continuing through hidden map nodes. Opening/
closing preserves page scroll, map transform, selected node, expanded
association state.

### D5. Architecture page information architecture (PHASE 7)

Bounded workspace: `[Map] [Relations] [Mechanism fragments] [Component
list]` or equivalent explicit disclosures. Default: map + selected inspector +
compact relation count + compact mechanism-fragment count + diagnostic
remainder count. Relations: human labels in primary UI, source-reference
action, raw kind under details, list and map counts reconcile. Component list:
useful non-canvas alternative; selection focuses the same canonical
component; does not duplicate every inspector detail by default. All
information preserved behind explicit sections/disclosures; evidence is never
deleted merely to shorten the page.

### D6. Connected mechanism fragments (PHASE 8)

Every transition has explicit source endpoint, target endpoint, claim kind,
support mode, ordering, evidence, scenario, limitation. Array order is never
path order. Render process-data/DFD-like fragments: entry, operation,
resolved resource/data store only when identity exists, external/client
touchpoint, exact arrow, dashed unknown frontier, separate independent
fragment. Adjacent nodes require `target(previous) == source(next)` or an
independent exact join. Boundary/resource observations without a path join
become side touchpoint groups, not next steps. Casdoor must not render 47
observations as one vertical story. At minimum preserve `main.go:36 main →
main.go:150 service.Start ⇢ continuation not recovered` and the independent
fragment `ldap/server.go:61 → ldap.getTLSconfig`. Every node/path is
source-actionable. View-model carries explicit endpoints (bump
MechanismFragmentVersion when the serialized shape changes; per-fragment
ordinals assigned post-sort).

### D7. Remaining semantic diagram correctness (PHASE 9)

- etcd: an unknown/opaque anchor ref (the `proposal.unknown_anchor_id`
  class) drops the anchor field and keeps the valid unit-backed component; a
  known anchor ID with redundant wrong kind is canonicalized with count
  normalization; unknown required unit refs reject only the dependent
  component; independently valid components survive (salvage contract v6:
  "wrong redundant anchor kind: canonicalize and count normalization";
  "unknown optional anchor: drop field, keep valid unit-backed component";
  "unknown required unit: reject dependent component only").
- etcd: local fallback labels/descriptions are localized for the RU product
  (no English fallback prose in RU reports; audit §10).
- Equivalent components (telebot/chatto/restic): shared/cross-cutting scope,
  equivalence coalescing, conflict/alternative disclosure, valid sibling
  preservation. Never "first component wins".
- Zero independently valid components publish local-only Architecture
  (salvage contract v6: "zero independently valid components: local-only
  Architecture").
- Association scope: exact package equality; root package does not own
  descendants; witness precision separate from association scope; broad/
  shared scope collapsed; Boundary+Resource same-witness co-projected into
  one visible interaction, with both canonical roles and every witness
  retained in details (salvage contract v6: "Every witness and canonical
  role remains available in details"). Casdoor «Запуск» must not inherit all
  218 observations through root-prefix scope.
- Do not weaken authority to improve diagrams.
- Version/cache/replay identities advance whenever acceptance semantics
  change.

## Acceptance

Provider-free first (PHASE 10-11): exact trimpath build, real-browser pointer
journeys on all five re-rendered reports at 1280×800, 1440×1000, 1920×1080,
zoom; hit-test every principal node; wheel/zoom/pan contract; inspector
overscroll containment and focus enter/return; association child click
preserves context; connected mechanism adjacency; passive edges;
Architecture section routing; zero unexplained inert source refs; EN/RU
catalog parity; raw enums absent from primary RU UI; `go test -count=1 ./...`,
`go vet ./...`, `make quality-check`, `make localization-check`,
`make surface-check`; node --check on touched JS; gofmt; git diff --check;
fresh reviewers
(code/regression, authority/semantic, desktop interaction/UX, diagram/
product, accessibility/keyboard) with up to two bounded repair cycles per
blocker. If semantic acceptance/unit identity changes, saved-response replay
first, then at most one final live five-repository matrix after every
provider-free gate is green.

## Out of scope

No frontend framework; no new provider/semantic stage; no embeddings; no
Tree-sitter; no new language adapter; no repository database; no BPMN/C4/FFBD
generation; no full call graph; no fuzzy path/ref repair; no production
repository-specific keyword rules; no provider tuning loop; no redesign of
`--no-secrets`. Mobile-only issues do not block; existing responsive CSS is
not intentionally regressed. No push without the owner's explicit signal.
