# Decision 217: Report UI/UX acceptance repair across application, CLI, service, and library reports

## Status

ACTIVE — bounded presentation/view-model corrective authorized by the owner
goal "Repomap report UI/UX acceptance repair across application, CLI, service,
and library reports" (2026-08-05). Provider-free implementation iteration over
the five owner-supplied saved reports.

## Scope

Turn the generated `report.html` from a cleanly styled dump of report
internals into a trustworthy repository-orientation workspace: a new
developer understands what the repository is, where execution/consumption
begins, what the main areas are, what to study next, and how strong the
supporting evidence is, in no more than two clicks.

Pure UI/presentation + deterministic view-model goal. No provider calls, no
upstream LLM prompt/selection/retrieval/architecture-synthesis changes, no new
evidence ledger, no frontend framework, no persistent state, no analytics, no
search platform, no IDE integration.

## Binding constraints (owner)

- Preserve `repomap <repo>` primary UX; no extra flags, no pipeline exposure.
- Preserve the three top-level jobs (Overview, Study, Architecture) and
  navigation <= 2 clicks.
- Local-first, evidence-backed; not a README viewer or documentation CMS.
- Weak signals allowed and useful, but must look different from exact
  evidence. Never hide uncertainty by hiding it or inventing architecture.
- No truncation of the theme shelf or evidence ("no truncating, more facts is
  better"); bounded `Total/Shown` only where a hard render bound is needed.
- Prefer deterministic report view-model helpers over ad hoc template
  conditions. Minimal report-contract addition allowed only when computed from
  already-available local facts and necessary for truthful presentation.
- Preserve exact revision-pinned source navigation and dirty-path behavior.
- Preserve the typed EN/RU catalog and fail-closed parameter parity; no DOM
  string rewriting or scattered hard-coded copy.
- Preserve current work (D215 uncommitted changes stay in the tree); do not
  reset/checkout/clean/stash/rebase/force-push/push. No commit without owner
  authorization.
- Do not add a production Playwright/MCP dependency. Use existing browser
  tooling (Node VM asset tests + a lightweight /tmp CDP screenshot helper).

## Baseline verified against the five owner-supplied runs

All confirmed by direct inspection of report.json + rendered HTML:

- All format-31 conceptual components are `hypothesis: true`.
- Casdoor (20260804-225729): 14 visible components, 4 exact symbol sources,
  10 package-only; 2 grounding relationships; Study accepted_partial (8 of 11
  themes shown); 7 exact members remain outside the grouping.
- Telebot (20260805-065044): 4 components, 2 exact / 2 package-only
  (Middleware + Interface look as solid as Start + Webhook); 0 relationships;
  Study partial; raw English README quote is the RU hero; quote/Markdown
  residue.
- Restic (20260805-065118): 9 components, 0 exact (all package-only);
  27 exact members outside; 3 relationships; unmapped-evidence wall dominates
  the Architecture page.
- Chatto (20260805-065121): 8 components, 0 exact; 43 exact members outside;
  0 relationships; page presents a confident map anyway.
- All runs: `flow_count: 0`, `orientation_confidence: 0` — treat carefully,
  do not promise an absent code-path mode.
- 390 px Casdoor Study: `scrollWidth=748 > 390` — real horizontal overflow,
  caused by `rm-study-theme-card__reading rm-source-action-link` long symbol
  strings (reproduced by CDP measurement).
- Permanent localization-status strip occupies primary chrome; duplicate
  repository naming; long provenance prose; Units ontology precedes user
  value; component cards are equal-height empty towers; `PARTIAL` English
  residue in Study detail; no evidence tier distinction; no entry
  classification; no recommended-next-reading.

## Implementation plan (bounded slices)

1. Global shell: move the localization-status strip into a compact
   "About this report" disclosure; reduce duplicate naming/provenance prose;
   keep revision/freshness/dirty/source-link semantics in one compact
   provenance row; safely normalize source Markdown; RU orientation uses the
   localized repository guide, original README shown as labeled source
   material.
2. Overview: four questions above the fold (what / entry / main areas /
   open-first) from existing grounded data; demote Units; classify entry
   surfaces (primary product, secondary service, tooling, library API) when
   data supports it; content-sized compact component cards with
   responsibility, evidence tier, source/package counts, one representative
   exact anchor, map/detail action; one recommended next reading.
3. Evidence honesty: deterministic non-numeric presentation tier derived from
   existing fields — exact source / package-backed grouping / exploratory
   hypothesis / unmapped exact evidence. No fake confidence percentages; no
   hiding package-only or partial components.
4. Study: source-backed theme cards as an ordered learning plan with title,
   question, why-it-matters, coverage state, bounded reading preview, detail
   route, bounded default with "show all"; user-facing "supported /
   incomplete" explanations replacing opaque PARTIAL; raw span/model/frontier
   diagnostics + full machine question inventory move into a collapsed
   "Coverage and provenance" section; safe overlap handling.
5. Architecture: no nonexistent mode promises; labeled relations or an
   explicit zero-relation statement; compact expandable unmapped-evidence
   warning preserving every item; desktop map where it adds value + a
   structured list alternative; mobile grouped expandable lists; component
   drawer with responsibility/evidence tier/scope/starting points/limitation/
   relations; fix clipped long identifiers and drawer accessibility.
6. Responsive + accessible: no horizontal overflow at 390 px on any route;
   keyboard-accessible architecture alternative; dialog drawer semantics
   (label, focus trap, Escape, focus return); visible focus, logical tab
   order, route changes keep focus; descriptive accessible names for repeated
   controls; reduced motion respected; safe `_blank` rel.

## Acceptance gates

- Node VM asset tests (real script.js + ui_messages.js) for the new view
  model, EN+RU.
- Catalog parity (no orphan/duplicate keys) + golden HTML regeneration.
- `go test ./...`, `go vet ./...`, `make build`, `node --check` on touched
  assets.
- Browser acceptance at 1440x1000 and 390x844 for Casdoor, Telebot, Restic,
  Chatto: Overview, Study, Architecture, one detail/drawer state;
  `scrollWidth <= innerWidth` at 390 px everywhere; architecture has a
  non-drag keyboard alternative; drawer is a real dialog; repeated controls
  have distinct accessible names.
- Final PASS report with evidence-honesty matrix and before/after behavior.

## Implementation record (2026-08-05)

Implemented and verified end to end on the saved-report matrix:

- **Global shell**: localization-status strip moved out of primary chrome into
  a collapsed "About this report" disclosure (server-rendered, catalog-driven);
  compact provenance row (revision / repository state / language); README
  prose normalized by a deterministic markdown normalizer and labeled
  "Источник: README репозитория"; localized repository-guide `system_story`
  becomes the primary hero when present.
- **Overview**: "At a glance" answers what / entry / main areas / open-first
  from existing grounded fields; entry surfaces grouped by classification
  (primary product / service / tooling / library); compact content-sized
  component cards with evidence tier, package counts, one representative exact
  anchor, map + source actions; Atlas unit ontology demoted below user-facing
  anatomy.
- **Evidence honesty**: deterministic four-tier presentation — exact source /
  package-backed grouping / exploratory hypothesis / unmapped — with distinct
  badges; package-only components are visibly weaker than exact-backed ones.
- **Study**: theme cards with coverage badges (source-backed / partial),
  bounded default shelf with "show all", bounded reading previews, detail
  route with user-facing coverage explanation ("N of M anchors passed source
  review" no longer reads as failed evidence); diagnostics + frontier browse
  moved into a collapsed "Coverage and provenance" section.
- **Architecture**: labeled relations list when relationships exist; explicit
  zero-relation honesty notice otherwise; the unassigned-evidence wall became
  a collapsed disclosure (count in summary, every item preserved, both in the
  canvas and on mobile); structured component list alternative with tiers;
  mobile hides the canvas and defaults to the list; canvas disclosure hidden
  on desktop when the canvas renders its own (no duplication).
- **Drawer a11y**: real dialog (role=dialog, aria-modal, descriptive label),
  focus trap, Escape closes, focus returns to the trigger; long identifiers
  wrap (`overflow-wrap: anywhere`) instead of clipping; `_blank` links keep
  safe `rel`.
- **Responsive**: 390 px overflow eliminated on all 15 route×repo
  combinations (baseline: Casdoor Study document width 748 px); reduced-motion
  and focus-visible support added.
- Tests: Node VM asset journeys extended with the drawer-dialog assertion;
  overview-order and representative-anchor assertions updated to the D217
  contract; full suite, vet, build, quality and localization gates green.
