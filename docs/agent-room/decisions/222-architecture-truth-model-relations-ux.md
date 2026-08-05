# Decision 222 — Architecture truth model, relation storytelling, and list-first UX

## Status

ACTIVE (goal 04 of the Archive 5 roadmap, approved by the repository owner
2026-08-05; implementation landed in the same session). Provider-free
report/view-model goal; no provider calls, no new analysis stage.

## Scope

Turn the Architecture view into a truthful, human-readable answer to six
questions:

1. what the major conceptual areas are;
2. who owns each grouping claim;
3. what exact evidence supports its members;
4. which relationships are locally observed;
5. what remains unclassified;
6. where to open source next.

## Truth model: three independent axes

### Grouping authority (who owns the claim)

Derived exclusively from the closed canvas source and synthesis state:

- `validated_model` -> validated model hypothesis;
- `partial_model` -> partial validated model hypothesis;
- `local_anchors` / `local_*` / `package_fallback` -> local deterministic
  architecture.

Rendered as a compact truth strip above the relations area. Authority is
never derived from component membership.

### Member evidence (what supports the members)

Per-component composition classified deterministically:

- exact sources only;
- mixed exact + package;
- package/structure only.

Shown as a badge on every component list entry. A local deterministic
component with exact sources is still a local grouping; a validated
component with package-only members is still a validated hypothesis about
structure. Authority and evidence are never conflated.

### Coverage (how much is covered)

- complete;
- partial with covered/requested (e.g. 62 of 88);
- local structure only (no accepted synthesis).

## Relations storytelling

- human-facing summary before the raw inventory: total count, static
  structural support vs runtime transition evidence, and the kind breakdown;
- grouped by relation kind;
- exact source action (embedded-snippet only) to open the witness; a
  location outside the persisted evidence stays text-only, never a dead
  button;
- zero-relation state stays explicit (unprojected member relations or the
  conceptual/package grouping note);
- the raw inventory is grouped and labeled; no machine enum wall before the
  map/list.

## D215 fallback (provider rejected / output-limited)

- exact bounded reason in user language;
- partial provider output is never presented as used;
- local deterministic Architecture is shown;
- never "synthesis was not performed" when a provider call occurred;
- no raw provider error in the primary UI.

## Components and remainder

- responsibility (existing description);
- authority (whole-canvas strip);
- evidence composition badge;
- member/unit scope count;
- observed relations above the map;
- representative entry/boundary/resource sources (source action);
- local unclassified remainder preserved behind a bounded disclosure
  ("Coverage outside the component map — N additional exact local items").

## Desktop/mobile

Desktop: structured list is first-class beside the canvas; relation summary
close to the affected components. Mobile was explicitly deprioritized by the
owner for this decision ("мне похер на mobile"); no new mobile-specific
layout work, existing 390px overflow hygiene remains a gate.

## Acceptance (per fixture)

- etcd: fallback understandable without reading diagnostics (provider
  responded, proposal rejected, local Architecture shown; authority = local;
  coverage = local structure only);
- Telebot/Chatto: zero-relations state does not look like a runtime diagram
  (unprojected note / conceptual grouping note);
- Restic/Casdoor: partial coverage visible (62 of 88; 33 of 56) with
  authority = partial validated model hypothesis;
- authority and member evidence never conflated;
- unclassified evidence preserved but not dominant (collapsed disclosure);
- 1440 screenshots; keyboard drawer journey; no horizontal overflow/page
  error; full tests/vet/build/report/manifest gates.

## Non-goals

- no new architecture analysis;
- no D216 algorithm changes;
- no new semantic stage;
- no broad Overview/Study redesign;
- no framework;
- no provider calls.

## Implementation record (2026-08-05)

### Delivered (working tree, not committed)

- `internal/report/templates/script.js`:
  - `architectureGroupingAuthority()` / `architectureAuthorityMessageID()` —
    closed authority axis from canvas source;
  - `architectureCoverageState()` — complete / partial X-of-Y / local;
  - `componentEvidenceComposition()` — exact / mixed / package;
  - `renderArchitectureAuthorityStrip()` — compact truth strip above the
    relations area;
  - `architectureRelationSummary()` — human summary (total, static vs
    runtime, kind breakdown) before the raw inventory;
  - `renderArchitectureRelations()` — grouped-by-kind inventory with exact
    source actions (embedded snippet only; location-only stays text);
  - component list entries carry evidence-tier badge + composition badge.
- `internal/report/templates/ui_messages.js` (EN + RU): authority,
  coverage, evidence-composition, and relations-summary catalog entries with
  fail-closed parameter parity.
- `internal/report/templates/style.css`: truth-strip chips, composition
  badges, relation kind chips, relation groups.

### Verified per fixture (provider-free rebuilds of the Archive 5 runs)

- etcd: fallback note + authority=local + coverage=local structure only;
  26 relations summarized (26 static, 0 runtime), kind chips, grouped
  inventory with exact source actions for snippet-covered witnesses.
- restic: partial 62/88; authority=partial; remainder disclosure 26 items.
- telebot: validated/complete; zero relations rendered as unprojected note,
  never a runtime diagram.
- casdoor: partial 33/56; authority=partial; 2 relations.
- 1440px screenshots verified visually; no overflow.

### Gate results

- `go test ./... -count=1`: full suite (74+ packages) — see final run;
- `go vet ./...`, gofmt on touched files: clean;
- `make build`, `make quality-check`, `make localization-check`: pass;
- provider-free: all gates run without credentials.
