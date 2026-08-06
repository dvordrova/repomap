# MORNING — overnight program report (2026-08-06)

Program: `~/Downloads/hermes-repomap-overnight-goal-v3.txt` (4 phases).
All phases completed and committed locally. Nothing pushed (per program rules).
Working tree clean (only `.hermes/` desktop attachments untracked, never committed).

## Final HEAD

- HEAD: `ef29318` (parent of the final style commit: `fd45a99`)
- Final HEAD (with gofmt fix): `3efc0f9` — see git log below
- Baseline start: `9e81e5447bf7c49286c7f0f1eea6cfe4adc38fdf`
- Commits added (9): decision checkpoints 223-226, implementations D223-D226,
  CURRENT.md registration, gofmt style fix
- Diff baseline→HEAD: 42 files changed, +3061 / −123
- All on `main`, local only (never pushed — program rule)

## Decisions (committed)

| Decision | File | Status |
|---|---|---|
| 223 Architecture wire — aggregated unit edges | decisions/223-architecture-wire-aggregated-unit-edges.md | ACTIVE |
| 224 Study content integrity — editorial normalization (resumes D219) | decisions/224-study-content-integrity-normalization.md | ACTIVE |
| 225 Component↔boundary/resource association from exact local data | decisions/225-component-boundary-resource-association.md | ACTIVE |
| 226 Mechanism Evidence Contract + honest vertical fragment | decisions/226-mechanism-evidence-contract.md | ACTIVE |

CURRENT.md updated (decisions 223-226 registered; D219 remains DEFERRED/history).

## Phase outcomes

### Phase 1 — Archive 6 adjudication + bounded Architecture corrective (D223)
- Evidence audit over Archive 6 (casdoor/etcd/restic/telebot): all goal claims
  verified (etcd request 178KB, 21 units, 53 structural, 636 relations; D216
  unit catalogs present; zero raw conceptual members on the wire; casdoor
  duplicate-component rejection; etcd missing-`ref` rejection — both honest
  fail-closed).
- D216 defect found: raw package-import edges (613/90/259) still on the wire
  despite D216 promising their removal in favor of aggregated unit edges;
  `relation_out_count` was always 0.
- Implemented: package-import relations dropped from the wire when units are
  present; `relation_out_count` filled from exact post-split unit membership;
  SynthesisRequest 12, prompt v15. Request shrink on fixtures: etcd −47%,
  restic −52%, casdoor −36%.
- Fresh-context review: PASS. 74/74.

### Phase 2 — Study content integrity (D224, resumes D219)
- Scout: overlong provisional prose normalized (whole-rune, counted in
  `ScoutStatus.Normalized`) instead of rejected as `prose_too_long` (chatto
  defect: 12 raw → 9 lost).
- Adjudication: split conflated `empty_observation` into
  empty_observation / observation_too_long / unknown_too_long /
  too_many_unknowns; overlong populated observations normalized before
  validation (etcd defect: 250-496-rune observations mislabeled empty).
- Reducer: readings dedup by exact (path,line,symbol) (casdoor webhook
  sendWebhook-twice fixed); badge full only when all direct AND no unknowns;
  deterministic portfolio rank (user-facing kinds first).
- Report: StudyThemeReading carries supported_observation + role; theme cards
  render role badges and bounded observations (EN/RU).
- Versions: ScoutResult/AdjudicationResult 2, projection 9, format 32;
  status records now consistently use result versions.
- Acceptance: saved Archive 5 raw-response replays — casdoor scout 11/12,
  adj 11/11 (4 normalized); etcd adj 12/12 (11 normalized) — previously 3
  rejected as empty. Fresh-context review: PASS.

### Phase 3 — Component↔boundary/resource association (D225)
- Proven join from existing exact data: member fact package paths ↔ Atlas
  unit paths ↔ observations ↔ evidence (path:line:symbol + provenance detail
  = imported path). Archive 6: casdoor 218/218, etcd 164/164, restic 60/60,
  telebot 8/8 — zero silent loss; omissions listed.
- Backend: per-component association rows (kind, imported family, owning
  unit, exact witnesses, observation counts), structural neighbors
  (incoming/outgoing) from canvas edges; drift-validation round trip.
- UI: component card answers the observed-callsites question; connection
  rows as primary controls (click → witnesses expand in place, exact source
  in ≤2 actions); limitations always visible; EN/RU.
- Browser-verified: TLS component shows 8 rows (database/sql, aws-sdk-go,
  go-sms-sender), witnesses expand, limitations visible.
- Fresh-context review: PASS (4 decision-doc wording refinements applied).

### Phase 4 — Mechanism Evidence Contract (D226)
- Closed per-transition claim contract: claim_kind / support_mode / evidence
  refs / scenario / limitations / ordering on every transition.
- One honest vertical fragment from saved local evidence only: process entry
  (main main.go:36) → SSA behavior-handoff transitions (main.go:150 →
  service.Start; ldap/server.go:61 → ldap.getTLSconfig) → observed
  boundary/resource rows → explicit unresolved_continuation (always last,
  support_mode unknown, ordering not_established). No invented edges; a
  downstream handoff (service.Start → …) is NOT chained because local
  evidence doesn't prove the connection from entry.
- UI: compact DFD-like list with contract fields per transition, frontier
  always visible; no BPMN/SIPOC/swimlane/FFBD claims (test-asserted).
- Fresh-context review: NOT PASS → bounded fixes applied (unresolved_
  continuation made an explicit contract transition present in code;
  DOM/asset test added covering kinds/orderings/frontier; entry proof_mode
  wording corrected in the decision; ordering fixed so the continuation is
  last). All suites green after fixes.

## Final gates (exact commands, all at final HEAD)

- `go test -count=1 ./...` → EXIT 0, 74 ok, 0 FAIL
- `go vet ./...` → EXIT 0
- `go build -trimpath -o .bin/repomap ./cmd/repomap` → EXIT 0
- binary SHA-256 (trimpath): `0dc6bb46d2ac9d7e98109e317fe2213907d6dd8df8c0fb05708f9228551a1cef`
- `node --check` script.js / ui_messages.js / architecture_canvas.js → EXIT 0
- gofmt: only pre-existing dirty files remain (none from this program);
  report.go style fix committed
- golden: regenerated and green

## Browser journeys (final binary render)

- EN `#/architecture`: canvas 3 groups; observed relations 2; Mechanism
  Fragment renders entry + 2 direct_static_call + unresolved_continuation
  with all contract fields; frontier box always visible; component list 4.
- EN component inspector (TLS and security boundary): 8 association rows
  (Boundary database/object 3 observations …), witnesses expand in place,
  limitations visible.
- RU `#/architecture`: same structure in Russian (Фрагмент механизма,
  Неразрешённая граница, Наблюдаемые внешние / state-вызовы).
- Study (RU): 11 themes render; role badges/observations present in the DOM
  asset test (this saved run predates D224, so its readings carry no role
  fields — old artifacts stay displayable; new runs render them).

## Artifact SHAs

- Decision files: committed; SHAs via `git rev-parse HEAD:<path>` (see git)
- Binary: SHA-256 above
- Golden: internal/report/testdata/report.golden.html (regenerated, green)

## Known bounds / honest limitations

- No push (program rule); owner reviews.
- Saved fixture runs predate D224/D225/D226 fields; old artifacts display
  compatibly, new runs carry the new projections.
- Live provider A/B and saved-response replay acceptance remain the owner's
  interactive-shell domain (credentials live only there).
- The etcd/casdoor rejected proposals (missing refs / duplicate identity)
  remain correct fail-closed behavior; no retry/fuzzy repair added.
