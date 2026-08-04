# Decision 212: Provider-free frontier browse over the complete Study question set (D211 considered spans)

## Status

Approved (council red-team PASS).
Approved by: Repository owner via Monster council gate (red-team PASS, council/51-red-team-212-rev2.md) in the current run
This is the decision-council draft for the next numbered
decision after Decision 211 (accepted at HEAD 324e363). It becomes active only
after explicit repository-owner approval replaces the CURRENT.md pointer;
activation and production implementation are not performed in this checkpoint.
This is ONE cohesive user-visible vertical increment (SCOPE-001): a local,
provider-free browse of the complete D211 considered-span question set inside
the Study tab. This decision authorizes NO live semantic acceptance; the full
verification chain is provider-free (ACCEPT-001). A future owner-approved
decision could authorize a live check, and this decision says nothing that
would bind that future decision to any particular provider or run count.
Revision 2 clears the two red-team HOLD blockers (council/50-red-team-212.md)
with two bounded corrections folded in: `Span` gains a manifest-relative
`DirectionID` on accepted rows for the direction-card link (Defect 1), and the
failed-state browse carries the distinct neutral label "Local question" ·
«Локальный вопрос» without the "not shown to the model" suffix (Defect 2).
No contract, artifact, cache, or version change beyond the already-specified
projection v6→v7.

## Notes (CURRENT.md style)

Decision 212 makes the complete D211 considered-span set explorable in the
Study tab through one distinct local "Study questions" surface, provider-free
and derived only from already-validated local artifacts: zero new analysis,
zero provider calls, zero artifact/wire/cache/request/result/status change.
The report projection adds exactly `FrontierBrowse{Total,Shown,Spans}` with
`Span{Ordinal,Title,Question,Stage,Source,Endpoint,DirectionID}`;
`DirectionID` is the public manifest-relative report direction id (matching
the `study_map` direction id used by `openStudyDirection`, script.js:4398),
derived at projection time from the validated `result.Directions` array order
(model rank) and present ONLY on accepted rows — no canonical span ID is
serialized; an accepted row with no matching published direction (should not
occur — fail closed) renders without the link. `Stage` is the four-value
membership (considered/advertised/model_selected/accepted) computed by exact
set arithmetic over the rebuilt input (`BuildAtlasStudyInput` +
`ValidateRequestRecordAgainstInput`, atlas_study.go:1613–1618), the request
catalog's advertised `RefRouteSpan` refs, `result.ModelSelectedSpanRefs`
(artifact.go:80, rejected siblings included) and `result.Directions[].Span`.
The browse is computed ONLY inside `readAtlasStudyReportProduct`
(atlas_study.go:1572): in the accepted/accepted_partial branch after result
decode + `ValidateResultRecordAgainstInput` (after :1657), with the chain
accepted ⊆ model_selected ⊆ advertised ⊆ considered re-verified, every
accepted row's `DirectionID` resolving to exactly one published direction
whose span matches, and per-stage tallies over the FULL pre-truncation row
set equal to the four status counts (68/32/10/10 on casdoor) enforced
fail-closed; a status `failed` run renders a separate neutral local-question
browse (Total from input count) exempt from accepted-stage tallies;
unavailable/prepared/uncalled states carry no browse. The four user-language
stage states, never three (STATE-001), are used ONLY in
accepted/accepted_partial runs: (a) "Model pick" · «Выбор модели» (locally
accepted; links to the numbered direction card via `DirectionID`), (b) "Picked
by the model, rejected by local checks" · «Выбрано моделью, отклонено
локальными проверками» (model_selected ∖ accepted, rendered only when
accepted_partial), (c) "Shown to the model, not picked" · «Показано модели,
не выбрано» (advertised not_selected — neutral wording, never
"reviewed"/"rejected" as a per-span model verdict, AUTH-001), (d) "Local
question — not shown to the model" · «Локальный вопрос — модели не
показывался» (considered ∖ advertised). In the failed state rows carry a
distinct neutral label WITHOUT the "not shown to the model" suffix — "Local
question" · «Локальный вопрос» — because the advertised subset WAS included
in the sent request (the request artifact exists in failed runs;
`readAtlasStudyReportProduct` requires request+status for a failed state,
atlas_study.go:1602–1603); the template keys this neutral label off
`state == failed`, not off the Stage enum value alone, and the
failure-banner-headed distinct surface is preserved. A visible "not a model
ranking" caption states that local order is stage group + canonical span ID
(locale-independent, byte-identical across EN/RU); the raw
`advertised_budget` enum chip becomes a human bilingual sentence, the 12
omission representatives become the first clickable rows of the Local group,
and "Show all N" · «Показать все N» reveals the embedded group client-side
(the report is already a JS app; no reportserver slice). Boundedness:
`MaxAtlasStudyBrowseSpans` = 256 with truthful Total/Shown and deterministic
first-N in canonical span-ID order when considered exceeds the ceiling; the
complete set stays bound by the existing CandidateSHA256 digest. Per-row
`Source`/`Endpoint` are published only for paths in `OpenablePaths` with an
explicit neutral unavailable state for rows whose source cannot open (no dead
buttons; `renderStudySourceAction` silently skips today, script.js:4761).
No per-row Role, no canonical IDs, no package buckets, no raw edges in the
projection (UX-001 + D211 exclusion; the v6 per-role histogram stays the only
role surface). Version delta is `AtlasStudyReportProjectionVersion` 6→7 ONLY
(report.go:30) because the report status JSON contract genuinely gains a
bounded per-span slice while request v7, prompt v13, result/status v8,
accepted-cache v7 (atlas_study_runtime.go:22), RunManifest v11
(manifest.go:32) and CurrentFormatVersion 30 (report.go:28) serialize no
browse bytes and stay unchanged; old v6 projection runs FAIL CLOSED under the
v7 binary (projection gate manifest.go:682–688) and only already-rendered
static HTML remains viewable — no compatibility reader, no migration. Test
updates: pin atlas_study_diagnostics_asset_test.go:141 to projection_version
7, add stage/tally/ceiling/unavailable/failed/DirectionID tests, regenerate
internal/report/testdata/report.golden.html. Verification is fully
provider-free: rebuild request (--repo), mock from the request catalog,
replay the saved accepted fixture (zero provider calls), render + node asset
journey (EN and RU, embedded and stripped, keyboard and pointer), manifest
gate, built binary offline on casdoor and one larger Go repository, and a
byte-identical two-run projection. On Study failure the browse is a distinct
local surface whose heading IS the failure banner, outside/visually disjoined
from the Study diagnostics block, never the default content of a failed Study
tab (FAILURE-001). No focus lane, second semantic call, GoSurvey, Tree-sitter,
deeper SSA/DFS, fuzzy repair, hidden fallback, compatibility reader,
migration, interactive pagination endpoint, or change to
MaxAdvertisedSpans/MaxDirections/wire/cache/request/result/status contracts is
authorized; no live acceptance is authorized by this decision.

## Observed problem

The accepted D211 run publishes a model portfolio of 10 ranked directions over
a 32-span advertised frontier, but the complete locally-owned considered set
contains 68 spans with backend-owned questions, exact symbols, supports,
relations and source navigation. The report renders only a static bounded
aggregate — "Frontier omissions: advertised_budget · Omitted: 36 ·
Representative refs: 12" — in which the 12 representative refs are plain
text (not clickable, not navigable) and the other 24 omitted spans are
unreachable (evidence pack GAP-1). This violates the charter promise "inspect
every claim through exact source evidence in at most two actions": a user
cannot inspect an omission claim that has no source. The raw
`advertised_budget` enum chip is itself a UX-001 violation (an internal
closed-reason enum rendered as user copy). 58 of 68 local questions are
invisible to the user, and nothing tells a user which questions the model saw
versus which it never saw.

The two council proposals each contain a separate defect that the merged
design must avoid: the product proposal's three badges collapse the four D211
stages and mislabel model-selected-but-locally-rejected siblings under
accepted_partial (STATE-001, AUTH-001); the substrate proposal publishes a
per-row `SupportRole` (the first user-visible role surface, UX-001 + D211
exclusion), specifies no user copy (UI-001), and defines an unimplementable
per-role tally (per-role counts are support pairs, not spans).

## User outcome

A person runs `repomap <repo>` on an ordinary service, opens the Study tab,
and sees the ranked model portfolio (unchanged default) plus one distinct
local "Study questions" surface directly below the Study diagnostics panel.
Every question the local analysis can answer — including all 36 spans the
model never saw — is explorable provider-free, and every claim opens its
exact source in at most two actions. Each row carries one of four honest
user-language stage states (model pick / picked-by-model-but-rejected-by-
local-checks / shown-to-model-but-not-picked / local-question-not-shown-to-
model) under a visible "not a model ranking" caption; an accepted "Model
pick" row opens the numbered direction card for that direction. When the
run's Study state is failed, the local questions remain reachable as a
separate surface whose heading is the failure banner, carrying the neutral
label "Local question" · «Локальный вопрос» (no "not shown to the model"
claim), so a provider outage cannot erase local analysis (FAILURE-001).

## Hypothesis

The visibility defect is purely a report-projection gap. The complete
considered set is already rebuilt and validated in memory at report-read time
(BuildAtlasStudyInput + ValidateRequestRecordAgainstInput, atlas_study.go:
1613–1618), all four stage sets share one ID namespace
(`route-span-<digest>`), `result.ModelSelectedSpanRefs` persists the
model-selected stage including rejected siblings, the chain
accepted ⊆ model_selected ⊆ advertised ⊆ considered is already enforced by
artifact validation (artifact.go:243–264), and `result.Directions` is already
the validated ranked portfolio whose public ids drive `openStudyDirection`
(script.js:4398). Stage membership for every considered span is therefore
exact, deterministic set arithmetic over already-validated identity sources,
and the accepted-row → direction-card link is derivable at projection time
from the validated `result.Directions` array order — no new analysis, no
provider call, no artifact bytes, no cache identity. The only contract that
must advance is the report projection (v6→v7) because the projection
serializes the new bounded per-span slice.

## Exact scope (the merged design)

### 1. New report types (internal/report/report.go)

```go
// AtlasStudySpanStage is the highest reached stage of one span, derived by
// exact set arithmetic at projection time. It is never provider-authored.
type AtlasStudySpanStage string // "considered" | "advertised" | "model_selected" | "accepted"

// FrontierBrowse is the bounded provider-free per-span browse of the complete
// considered Study question set. Total/Shown are always truthful; Spans never
// exceed MaxAtlasStudyBrowseSpans.
type FrontierBrowse struct {
    Total int    `json:"total"` // complete considered count (len of rebuilt input.RouteSpans)
    Shown int    `json:"shown"` // len(Spans)
    Spans []Span `json:"spans"`
}

// Span is one browse row. Ordinal is 1..N in canonical span-ID order and is
// manifest-relative; it is NOT a canonical ID. Stage is the four-value
// membership. Source/Endpoint are exact user-code locations published only
// for paths in OpenablePaths; a row whose source cannot open carries the
// neutral unavailable state instead of a dead button. DirectionID is present
// ONLY on accepted rows: it is the public manifest-relative report direction
// id (matching the study_map direction id used by openStudyDirection,
// script.js:4398), derived at projection time from the validated
// result.Directions array order (model rank); no canonical span ID is
// serialized. An accepted row with no matching published direction (should
// not occur — fail closed) renders without the link.
type Span struct {
    Ordinal     int                 `json:"ordinal"`
    Title       string              `json:"title"`    // exact source-card symbol/label; system-path "from → to" endpoints
    Question    string              `json:"question"` // backend-compiled question in the report language
    Stage       AtlasStudySpanStage `json:"stage"`
    Source      UserCodeLocation    `json:"source"`              // only when Source.Path ∈ data.OpenablePaths
    Endpoint    *UserCodeLocation   `json:"endpoint,omitempty"`  // only for system-path spans whose endpoint path ∈ data.OpenablePaths
    DirectionID string              `json:"direction_id,omitempty"` // accepted rows ONLY; public study_map direction id (model rank)
}
```

`AtlasStudyReportStatus` gains one field:

```go
FrontierBrowse *FrontierBrowse `json:"frontier_browse,omitempty"`
```

`MaxAtlasStudyBrowseSpans = 256` is a new report-side constant beside
`AtlasStudyReportProjectionVersion` (report.go:30 area).

### 2. Where it is computed

Inside `readAtlasStudyReportProduct` ONLY (the manifest DeepEqual round-trip
at manifest.go:1030–1037 re-derives the full status and requires byte
equality, so a template-side or second-pass browse is impossible by
construction):

- **accepted / accepted_partial branch** — after `DecodeResultRecord` +
  `ValidateResultRecordAgainstInput` (atlas_study.go:1651–1657) and after
  `projectAtlasStudyMap`. Derive the stage of every considered span from:
  considered = rebuilt `input.RouteSpans` (never from the request catalog,
  which carries only the advertised 32); advertised = request catalog
  `RefRouteSpan` canonical IDs; model_selected = `result.ModelSelectedSpanRefs`
  (rejected siblings included); accepted = `result.Directions[].Span` IDs.
  Re-verify `accepted ⊆ model_selected ⊆ advertised ⊆ considered`; compute
  per-stage tallies over the FULL pre-truncation row set and require they
  equal `ConsideredSpanCount/AdvertisedSpanCount/ModelSelectedSpanCount/
  AcceptedSpanCount` from the status. For every accepted row, derive
  `DirectionID` from the validated `result.Directions` array order (model
  rank) and require it to resolve to exactly one published direction whose
  span matches the row's span; any mismatch is a report projection error
  (fail closed, no partial browse). Only then apply the 256 ceiling in
  canonical span-ID order with truthful Total/Shown.
- **failed state** — a neutral local-question browse: Total from the rebuilt
  input count, exempt from accepted-stage tally checks, rendered as a
  distinct local surface whose heading is the failure banner (see Failure
  behavior). Every row carries the distinct neutral label "Local question" ·
  «Локальный вопрос» — WITHOUT the "not shown to the model" suffix — because
  the advertised subset WAS included in the sent request (the request
  artifact exists in failed runs; `readAtlasStudyReportProduct` requires
  request+status for a failed state, atlas_study.go:1602–1603). The template
  keys this neutral label off `state == failed`, not off the Stage enum value
  alone. Exempt means: the chain re-verification and the `DirectionID`
  resolution are not applicable (no result artifact, no directions) and the
  browse does not claim any model-referencing state.
- **unavailable / prepared / uncalled states** — `FrontierBrowse` stays nil;
  no browse is rendered.

### 3. The four user-language stage states (STATE-001, AUTH-001, UX-001)

Used ONLY in accepted/accepted_partial runs (a failed run uses the distinct
neutral failed-state label — see §2 and Failure behavior):

- (a) **"Model pick" · «Выбор модели»** — Stage accepted; one of the locally
  accepted portfolio directions; the state badge links to the numbered
  direction card via `DirectionID` (the public `study_map` direction id
  opened by `openStudyDirection`, script.js:4398; array order is the model's
  rank). An accepted row whose `DirectionID` does not resolve (should not
  occur — fail closed) renders without the link.
- (b) **"Picked by the model, rejected by local checks" · «Выбрано моделью,
  отклонено локальными проверками»** — Stage model_selected without accepted;
  rendered ONLY when status is accepted_partial (the rejected-sibling case);
  carries a distinct not-accepted marker. Never rendered in a fully accepted
  run (there are no such rows) and never merged into state (c).
- (c) **"Shown to the model, not picked" · «Показано модели, не выбрано»** —
  Stage advertised, normal not_selected. NEUTRAL wording: never "reviewed",
  "rejected" or any per-span evaluative verdict — the provider ranked its
  picks and never adjudicated the other advertised spans individually
  (AUTH-001).
- (d) **"Local question — not shown to the model" · «Локальный вопрос —
  модели не показывался»** — Stage considered only. This four-state label
  (with the suffix) appears only in accepted/accepted_partial runs; the
  failed-state browse uses the distinct neutral label "Local question" ·
  «Локальный вопрос» without the suffix (§2, Failure behavior).

Rows are grouped by the existing user-facing learning stage, then sorted by
canonical span ID within the group (locale-independent, byte-identical across
EN/RU). Each row is a compact list item — not a direction card (CARD-001):
the backend-compiled question as a button, a stage/job tag, and the state
badge. Clicking the question opens the exact source through the existing
two-action journey (question → exact file/line), reusing the
`renderStudySourceAction` / `sourceLocationActionAvailable` machinery and the
pre-rendered raw path/line pattern already used by StudyReadingAnchor
(atlas_study.go:1840–1841); localhost open requests continue to use
manifest-authorized opaque `source_id` only. Clicking a "Model pick" badge
opens the numbered direction card through the existing
`openStudyDirection(DirectionID)` path (script.js:4398).

### 4. Caption, omission-line rework, and disclosure

- Visible caption (EN): "Every question the local analysis can answer for
  this repository, in a fixed local order. This is not a model ranking."
- Caption (RU): «Каждый вопрос, который локальный анализ может поставить
  для этого репозитория, в фиксированном локальном порядке. Это не
  ранжирование моделью.»
- The raw `advertised_budget` enum chip in the diagnostics panel is replaced
  by a human bilingual sentence (EN): "Left out of the model's review to keep
  the request bounded — these are full local questions." (RU): «Не вошли в
  рассмотрение моделью, чтобы запрос оставался ограниченным, — это полные
  локальные вопросы.»
- The line becomes a button "Show all N" · «Показать все N» that reveals the
  Local group (client-side progressive disclosure; the report is already a JS
  app — no reportserver slice, no interactive pagination endpoint).
- The 12 omission representatives are DERIVED rows of the Local group — the
  browse re-derives them from the rebuilt input and the omission aggregates
  (today `projectAtlasStudyOmissions` publishes only counts, report.go:
  391–406); they render as the first clickable rows of the Local group in
  canonical span-ID order. The remaining embedded rows of the group follow;
  "Show all N" reveals the full embedded group.
- Section title (EN): "All study questions" · (RU): «Все вопросы изучения».
- Every new string exists in BOTH catalogs of internal/report/templates/
  ui_messages.js (D211 pattern; EN ~L103–160/L239, RU ~L1383–1440/L1519).

### 5. Source availability gate

Per-row `Source` (and `Endpoint` for system-path spans) is published ONLY
when the path is in `data.OpenablePaths`. A row whose primary source cannot
open — and, for a system-path span, whose endpoint also cannot open — renders
the explicit neutral unavailable state ("source unavailable" in user
language) instead of a button: `renderStudySourceAction` silently returns
null today (script.js:4761), so an unverified row would render as a dead
button. There is no fallback path, no invented source, no package-drawer
substitute.

### 6. Boundedness (GENERALIZE-001, CARD-001)

`MaxAtlasStudyBrowseSpans = 256`. When considered ≤ 256 the complete set is
embedded (casdoor: 68). When considered > 256 the deterministic first-256 in
canonical span-ID order are embedded with truthful `Total`/`Shown`; every
shown row is a real span with exact source (no filler); the complete set
remains bound by the existing `CandidateSHA256` digest in the status
artifact. Progressive disclosure is client-side over the embedded array; a
bounded "showing N of M" aggregate is shown only when the ceiling binds. The
ceiling is not a Casdoor-tuned value: it is a report-byte bound, and the
truthful Total/Shown plus client-side disclosure keep >256-span repositories
honest instead of stranding spans behind a silent hash-truncated aggregate.

## Non-goals

No change to `MaxAdvertisedSpans` (32), `MaxDirections` (10),
`MinPortfolioDirections` (6), `MaxOmissionRepresentatives` (12), or any
wire/cache/request/result/status contract; no new or second provider call; no
focus lane; no GoSurvey; no Tree-sitter; no deeper SSA/DFS; no fuzzy repair;
no hidden fallback; no compatibility reader; no migration; no per-row Role
(`SupportRole` stays internal; the v6 per-role histogram remains the only
role surface); no canonical IDs, package buckets or raw edges in the
projection; no interactive pagination endpoint; no reportserver slice; no
local re-ranking or synthetic "local ranking" (browse order is explicitly
stage group + canonical span ID and labeled non-ranking); no redesign of
direction cards or the model portfolio; no Casdoor-specific checklist
(GENERALIZE-001); no Architecture-failure retry affordance (evidence GAP-2
requires provider-call authorization and cost policy, out of scope); no
live acceptance run.

## Authority contract

Models may not create identity, path, relation, authority, ownership,
membership or acceptance (AUTH-001); this slice creates no new model input of
any kind. Every browse row is derived exclusively from already-validated
local artifacts: the rebuilt input (considered spans, backend-owned
questions, exact producer-owned source locators), the request catalog
(advertised refs), the result (model_selected incl. rejected siblings,
accepted, the ranked `Directions` array), and the status (counts, digest,
omissions). No provider output is parsed to recover symbols, sources, stages
or ordering. Canonical span IDs are used only as an internal deterministic
sort key and are never serialized; the published `Ordinal` is
manifest-relative. `DirectionID` is the public manifest-relative report
direction id already used by the report's own direction-card journey
(`openStudyDirection`, script.js:4398) — not a canonical span ID and not a
new identity — and is published only on accepted rows whose span matches the
published direction. `Title` and `Question` are backend-owned values already
bound by the request identity. Per-row `SupportRole` is never published
(UX-001 + D211 exclusion; the v6 per-role histogram stays the only role
surface). Source availability is decided by the local backend's
`OpenablePaths` authority, never by prose or symbol similarity.

## Version changes (VERSION-001)

- Bump ONLY `AtlasStudyReportProjectionVersion` 6 → 7 (internal/report/
  report.go:30). Exact reasoning: the report status JSON contract genuinely
  gains a bounded per-span slice carrying exact source locators, questions,
  stage membership and the accepted-row `DirectionID`; a v6 reader must not
  misread a v7 projection, and the v7 binary must not accept a v6 projection
  as authoritative.
- Unchanged: request/catalog v7, prompt v13, result/status v8, accepted
  cache contract `atlas-study-accepted-v7` (atlas_study_runtime.go:22),
  RunManifest v11 (manifest.go:32), CurrentFormatVersion 30 (report.go:28).
  The browse is derived at render time and never persisted as an artifact, so
  request bytes and all cache identities are byte-identical — zero cache
  invalidation. Pin tests exact_workspace_search_test.go:448 and
  workspace_entrypoint_test.go:109 keep passing unchanged.
- Old v6 projection runs FAIL CLOSED under the v7 binary: the projection gate
  (manifest.go:682–688) requires `report.AtlasStudy.ProjectionVersion ==
  AtlasStudyReportProjectionVersion`; only the already-rendered static HTML
  remains viewable. No compatibility reader, no active-artifact migration,
  no reinterpretation; historical self-contained HTML reports remain
  untouched.

## Implementation lanes

1. **Projection lane** — internal/report/report.go: `AtlasStudyReportProjectionVersion`
   7, `MaxAtlasStudyBrowseSpans` 256, `AtlasStudySpanStage`/`FrontierBrowse`/
   `Span` types (incl. `DirectionID` on accepted rows), the `FrontierBrowse`
   field on `AtlasStudyReportStatus`; internal/report/atlas_study.go: derive
   the browse inside `readAtlasStudyReportProduct` — accepted/accepted_partial
   branch after atlas_study.go:1657 (stage arithmetic over rebuilt input +
   request catalog + `result.ModelSelectedSpanRefs` + `result.Directions`,
   chain re-verification, per-stage tallies over the full pre-truncation row
   set, accepted-row `DirectionID` derivation from the validated
   `result.Directions` array order with fail-closed resolution, then the 256
   ceiling), failed-state neutral browse (Total from input, exempt from
   accepted tallies, neutral label keyed off `state == failed`),
   OpenablePaths source gate with the neutral unavailable state. No
   internal/atlasstudy/* change.
2. **Template lane** — internal/report/templates/script.js: a sibling of
   `renderAtlasStudyDiagnostics` rendering the "All study questions" surface
   (stage-grouped rows, four user-language states incl. the accepted_partial
   rejected-sibling marker, "Model pick" badges linking to numbered direction
   cards via `DirectionID`/`openStudyDirection`, failed-state surface with
   the distinct neutral label keyed off `state == failed` and headed by the
   failure banner, "not a model ranking" caption, human omission sentence
   replacing the `advertised_budget` chip, "Show all N" progressive
   disclosure); the exact source action reuses `renderStudySourceAction`;
   ui_messages.js EN + RU catalogs.
3. **Test/acceptance lane** — pin atlas_study_diagnostics_asset_test.go:141
   to `projection_version: 7`; new internal/report/atlas_study_browse_test.go
   (stages, tallies, chain, accepted-row `DirectionID` resolution, ceiling,
   unavailable-source, failed-neutral label, browse-absent states); regenerate
   internal/report/testdata/report.golden.html; manifest-gate tests; then the
   provider-free gates below.

## Provider-free gates (ACCEPT-001)

1. `go test ./...` and `go vet ./...` pass; the D210 Unicode entry-handoff
   regression gate stays green.
2. **Rebuild** — atlas_study_request_rebuild.go (`--repo` authority mode)
   reproduces the exact v7 request on the casdoor fixture.
3. **Mock** — atlas_study_response_mock.go derives a response only from the
   request catalog (no network, no credentials).
4. **Replay** — atlas_study_response_replay.go replays the saved accepted
   fixture (checks/casdoor-replay-779db63, provider_calls=0) with zero
   provider calls: refuses the original run dir, exclusive writes, SHA-256
   pins, secretscan.DetectAlways on all bytes.
5. **Render + journey** — node asset journey test (atlas_study_diagnostics_
   asset_test.go pattern): report.html (which embeds the full status) renders
   68 clickable browse rows; each row opens its exact path:line in two
   actions; every accepted row's "Model pick" badge opens exactly one numbered
   direction card whose span matches the row; "Show all N" reveals the Local
   group; the four stage states are distinct and correctly counted (accepted
   run: 10/0/22/36 for states a/b/c/d); EN and RU journeys; embedded and
   stripped static reports; keyboard and pointer.
6. **Manifest gate** — a v6 projection rejects under the v7 binary; a browse
   inconsistent with the four status counts, the chain, or the accepted-row
   `DirectionID` resolution rejects (DeepEqual round-trip,
   manifest.go:1030–1037).
7. **Binary offline gates** — build `go build -trimpath -o PATH
   ./cmd/repomap` and exercise `PATH REPO --offline --no-open --no-serve
   --debug-dir DIR` on casdoor and one larger nearby Go repository: verify
   exit status, manifest, Atlas, report JSON and report HTML directly (a
   wrapper reporting success is not acceptance); provider_calls=0; the larger
   repository exercises the 256 ceiling with truthful Total/Shown (or a
   complete embed when considered ≤ 256).
8. **Byte-identical two-run projection** — two offline runs produce a
   byte-identical browse projection (SHA-verified), proving the deterministic
   stage + ID ordering (including `DirectionID` on the same accepted rows).
9. **EN/RU coverage** — every new message ID exists in both ui_messages.js
   catalogs; `--lang ru` renders Russian copy with exact symbols, questions
   and refs unchanged.

No live semantic acceptance is authorized by this decision.

## Held-out semantic/product comparison

Compare three products on the same fixture (provider-free; ACCEPT-001 applies):

1. **Today's product (held out):** model portfolio 10 + static aggregate; 58
   of 68 spans invisible; a user wanting the entry-handoff or call-boundary
   areas beyond the portfolio must re-run with guesses; the omission claim
   has no source to inspect (charter violation).
2. **Adaptive budget (evidence option b, held out):** more spans reach the
   model, but `MaxDirections` stays 10 and the UI still shows 10 directions +
   a static line — the user outcome is unchanged — while paying request-byte
   growth and VERSION-001 identity churn on a limit that lives in the wire
   contract.
3. **This proposal:** the full local question set is a first-class read-only
   surface with honest four-state provenance; the model portfolio remains the
   ranked default; exact source navigation for every row and a direct
   accepted-row → direction-card link via the public `DirectionID`; zero
   wire/identity risk.

This is a product/UX comparison, not a semantic-usefulness proof: provider-
free gates prove deterministic contracts; a live check would require a
separate owner-approved decision.

## Browser journey (UI-001)

Study tab → "All study questions" section below the Study diagnostics panel →
a "Local question" row (state d) opens the exact file/line in two actions;
the omission line's "Show all N" button reveals the Local group; the four
state badges are distinct; the "not a model ranking" caption is visible; the
"Model pick" rows link to the numbered direction cards via their
`DirectionID` (the public `study_map` direction id opened by
`openStudyDirection`, script.js:4398 — array order is the model's rank); the
"Shown to the model, not picked" rows carry no evaluative verdict; under
accepted_partial a rejected-sibling row visibly shows state (b). In a failed
run the browse is headed by the failure banner and every row carries the
distinct neutral label "Local question" · «Локальный вопрос» (no "not shown
to the model" suffix, no model-referencing badges). Journey passes in EN and
RU, in embedded and stripped static reports, with keyboard and pointer input.

## Failure behavior (FAILURE-001)

When the run's Study status is `failed`, the browse renders as a distinct
local surface whose heading IS the failure banner, outside/visually disjoined
from the Study diagnostics block. The four accepted/accepted_partial stage
states (a)–(d) do NOT appear; every row carries the distinct neutral label
"Local question" · «Локальный вопрос» — WITHOUT the "not shown to the model"
suffix, because the advertised subset WAS included in the sent request (the
request artifact exists in failed runs; `readAtlasStudyReportProduct` requires
request+status for a failed state, atlas_study.go:1602–1603) — keyed off
`state == failed`, not off the Stage enum value alone. No model-referencing
badges, no portfolio, no "reviewed" wording; Total comes from the rebuilt
input count and the surface is exempt from accepted-stage tally checks (there
is no result artifact; chain and `DirectionID` re-verification are not
applicable). It is never the default content of the failed Study tab. The
existing honest unavailable state and the local Architecture remain
untouched; a provider outage cannot erase local analysis. (Architecture-
failure behavior is unchanged and out of scope.)

## Superficial completion traps

- A wrapper reporting success is not acceptance: verify the built binary's
  exit status, manifest, Atlas, report JSON and report HTML directly
  (AGENTS.md).
- Deriving considered spans from the request catalog: the catalog carries
  only the advertised 32; considered must come from the rebuilt input
  (BuildAtlasStudyInput + ValidateRequestRecordAgainstInput).
- Computing the browse outside `readAtlasStudyReportProduct`: the manifest
  DeepEqual round-trip (manifest.go:1030–1037) rejects it by construction.
- Per-role tallies against `candidate_coverage.per_role`: unimplementable —
  those counts are (role, target) SUPPORT PAIRS, not spans; the correct
  fail-closed check is per-stage tallies over the full pre-truncation row
  set (68/32/10/10 on casdoor).
- Three badges instead of four states: an accepted_partial run with a
  rejected sibling must render state (b); mislabeling it "not selected" is a
  STATE-001 failure.
- Publishing per-row Role, canonical IDs or package buckets: direct UX-001 +
  D211 exclusion; canonical IDs may be a sort key only, never serialized.
- Accepting a row without a resolvable `DirectionID`: every accepted row's
  `DirectionID` must resolve to exactly one published direction whose span
  matches; a mismatch is a projection error (fail closed, no partial browse).
- Relying on question-text or source-location matching to link a row to a
  direction card: provably ambiguous — two supports can reference the same
  target symbol/location (atlas_study.go:1084–1092) and produce identical
  question text and source locations for different spans; only the
  projection-time `DirectionID` derived from the validated `result.Directions`
  array order links a row to its direction card.
- Applying the four-state copy (a)–(d) to a failed run: the failed-state
  browse must use the distinct neutral label "Local question" · «Локальный
  вопрос» without the "not shown to the model" suffix (the advertised subset
  WAS in the sent request), keyed off `state == failed`; carrying the
  suffix into failure is an AUTH-001 accuracy failure.
- Emitting rows whose source cannot open: `renderStudySourceAction` silently
  returns null today (script.js:4761); gate on `OpenablePaths` and render the
  neutral unavailable state instead.
- Claiming v6 projections remain renderable under the v7 binary: false — v6
  fails closed at the projection gate; only already-rendered static HTML
  stays viewable.
- EN-only strings: every new message must exist in both ui_messages.js
  catalogs.
- Tuning to casdoor's 68: the 256 ceiling with truthful Total/Shown and
  client-side disclosure keeps >256-span repositories honest; a hard
  truncation without disclosure would re-create GAP-1 at scale.
- Any provider call, live acceptance, wire/limit/cache change, or second
  decision's work smuggled into this slice: out of scope (SCOPE-001).

## Explicitly out of scope (repeated for the implementer)

No new/second provider call; no live acceptance (a future owner-approved
decision may authorize a live check); no focus lane; no GoSurvey; no
Tree-sitter; no deeper SSA/DFS; no fuzzy repair; no hidden fallback; no
compatibility reader; no migration; no per-row Role; no canonical IDs or
package buckets in the projection; no raw edges; no interactive pagination
endpoint; no reportserver slice; no change to MaxAdvertisedSpans (32),
MaxDirections (10), MinPortfolioDirections (6), MaxOmissionRepresentatives
(12), or any wire/cache/request/result/status contract; no local re-ranking
or synthetic local ranking; no redesign of direction cards or the model
portfolio; no Casdoor-specific checklist; no Architecture-failure retry.

---

## Change summary (revision 1 → revision 2, both red-team blockers only)

**Correction 1 (Defect 1 — direction reference on accepted rows):**
- `Span` (§1) gains `DirectionID string json:"direction_id,omitempty"`, present ONLY on accepted rows; matches the public `study_map` direction id used by `openStudyDirection` (script.js:4398), derived at projection time from the validated `result.Directions` array order (model rank); explicit statement that no canonical span ID is serialized and that an accepted row with no matching published direction (should not occur — fail closed) renders without the link.
- §3(a) and Browser journey updated: the "Model pick" badge links to the numbered direction card via this `DirectionID`.
- §2 fail-closed invariants updated: every accepted row's `DirectionID` must resolve to exactly one published direction whose span matches; a mismatch is a projection error (fail closed, no partial browse).
- Superficial completion traps updated: "accepting a row without a resolvable DirectionID" and "relying on question-text or source-location matching to link a row to a direction card" are traps.

**Correction 2 (Defect 2 — failed-state neutral label):**
- The four user-language stage states (a)–(d) are now explicitly used ONLY in accepted/accepted_partial runs (§2, §3 heading, Failure behavior).
- The failed state now carries the distinct neutral label "Local question" · «Локальный вопрос» WITHOUT the "not shown to the model" suffix, with the reason stated (the advertised subset WAS included in the sent request; request artifact exists in failed runs; `readAtlasStudyReportProduct` requires request+status for failed, atlas_study.go:1602–1603); the template keys this label off `state == failed`, not off the Stage enum value alone.
- §2 (failed state), §3(d), Failure behavior, Notes, Browser journey, and traps all updated consistently; the "exempt from accepted-stage tally checks" clause and the failure-banner-headed distinct surface are kept.

Everything else is byte-faithful in substance to revision 1 (title preserved; status now "Proposed (revision 2)").
