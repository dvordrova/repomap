# Decision 213: Atlas-backed source-grounded Study themes

## Status

Approved (council red-team PASS).
Approved by: Repository owner via Monster council gate (red-team PASS,
run 20260804T122640Z-afe3e93, council/31-red-team-213.md; fresh-context
read-only red-team returned PASS with no blockers and no required revision).
This is the decision-council draft for the next numbered decision after
Decision 212 (accepted at HEAD afe3e93). It becomes active only after explicit
repository-owner approval replaces the CURRENT.md pointer; activation,
implementation and final acceptance are performed in the same run. This is ONE
cohesive user-visible vertical increment (SCOPE-001): the default Study shelf
becomes an editorial, multi-anchor, source-backed "Study themes" surface
produced by exactly two bounded semantic stages, on top of the accepted
Atlas-first base. One decision per Monster run: after D213 PASS, write
MORNING.md and stop; no further numbered decision in this run (directive
"Monster execution constraint", stop condition).

## Notes (CURRENT.md style)

Decision 213 restores the editorial multi-anchor Study theme layer on top of
the accepted Atlas-first base. One new deterministic local package
`internal/themestudy` produces a flat names-only `f*` file vocabulary, bounded
exact `a*` seed-anchor source packs, and two new versioned semantic stages —
**Theme Scout** then **Source Review / Theme Adjudication** — separated by
local source expansion, followed by a deterministic local reducer into clean
Study theme cards plus the existing exact-source drawer. The single-stage
atlas-study **provider call is retired** (its local compile remains the exact
seed producer), so the Study pipeline has exactly the two semantic stages the
directive mandates; Navigator is untouched and remains its own subsystem
(atlas_study_runtime.go:61: "Navigator is neither an input nor a
prerequisite"). The report projection advances `AtlasStudyReportProjectionVersion`
7→8 and `CurrentFormatVersion` 30→31; `RunManifest` v11→v12 binds eight new
theme artifacts by SHA-256. D212's four-stage local browse is **kept** (never
three states) and re-based onto the D213 pipeline exactly as the product
critic's B1 correction prescribes: considered / seed-advertised /
scout-anchored / published, with the distinct failed-run neutral label "Local
question" · «Локальный вопрос» preserved. Old v7 projections fail closed under
the v8 binary; historical self-contained reports remain untouched. Acceptance
is provider-free first, then **exactly one** fresh Casdoor semantic A/B run on
the same revision and dirty state, judged by the M1–M9 material-improvement
criteria; it is acceptance, never a tuning loop, with no second live
calibration.

## 1. Problem and evidence

### 1.1 Verified regression (from actual compact artifacts)

Same Casdoor revision `2b9a05239da8ea70b8f401ca94a3808ebfd755cf` (both runs;
OLD dirty_sha256 `b68cb013…`):

| measure | OLD source-backed 20260801-152602 | CURRENT Atlas-first 20260804-102436 |
|---|---|---|
| themes / cards | 8 themes | 10 cards (considered 68, advertised 32, accepted 10) |
| unique anchors | 21 | 10 (one span per card) |
| distinct files | 17 | 2 (main.go + certificate/dns.go) |
| root reuse | — | `casdoor.main` at main.go:36 in 9 of 10 |
| card questions | editorial multi-anchor | 8 of 10 are direct-call evidence shapes |

### 1.2 Root cause

The Atlas-first pipeline replaced the old multi-stage source-backed Study with
ONE semantic stage over the compact span catalog: the model ranks/selects spans
and authors one card per selected span. Cards therefore track exact spans (one
anchor each) and repeat the entry root (`main`), because the considered set is
dominated by entry-handoff spans. The editorial multi-anchor theme layer and
its per-anchor source review are gone; 15 of the 17 files the old run proved
thematically useful are invisible from the current shelf.

### 1.3 Hypothesis and the preserved authority transition

Restoring the editorial layer requires exactly the directive's two bounded
semantic stages (Theme Scout; Source Review / Theme Adjudication) separated by
backend-executed local source expansion and followed by a deterministic local
reducer. A theme is a source-backed editorial question over one or more exact
anchors; it is not required to be a proven runtime path. The model may create
title/question/explanation/reading order; it may not create source locations,
paths, symbols, facts, relations, membership, ownership, runtime order,
canonical IDs, successful checks or acceptance. Paths, symbols, identities,
membership, relations, evidence, coverage and publication remain local and
exact; request-local short refs remain mandatory; local Architecture remains
available on semantic failure; raw Orientation must not return as authority.
D210/D211/D212 spans and exact relations remain valuable support seeds — never
the final Study ontology or mandatory card titles.
## 2. Scope (exact changes, ONE vertical slice)

1. New deterministic package `internal/themestudy` (model.go, compile.go,
   artifact.go, response.go, mock_response.go, replay.go, vocab.go, sources.go,
   expand.go, scout.go, adjudicate.go, reduce.go).
2. Two new semantic stages with versioned artifacts:
   `theme_scout_request/result/status.v1.json`,
   `theme_source_expansion.v1.json` (provider-free, persisted),
   `theme_adjudication_request/result/status.v1.json`, `study_themes.v1.json`.
3. Theme shelf projection: `AtlasStudyReportStatus` gains
   `Themes *AtlasStudyThemesProjection` alongside `frontier_browse`, under
   projection bump 7→8; card DTO/HTML carry **zero source bytes**.
4. D212's four-stage local browse **kept and re-based** onto the D213 pipeline
   (B1 correction) in the same v8 bump: considered / seed-advertised /
   scout-anchored / published, with the distinct failed-run neutral label
   "Local question" · «Локальный вопрос» preserved.
5. Retire the atlas-study provider call; keep `BuildAtlasStudyInput` +
   `atlasstudy.Compile` as the exact local seed producer. Study pipeline =
   exactly two semantic calls.
6. Navigator, Architecture, Orientation, snapshot, sourcewindowfacts, and
   atlasstudy wire contracts (request v7, result/status v8, catalog/wire v7)
   stay frozen and untouched.

## 3. Pipeline (exactly two semantic stages)

```text
compact Atlas / Architecture / exact spans
+ flat names-only file vocabulary (contract A)
+ exact source-backed seed anchors (contract B)
                ↓
Theme Scout semantic call (contract C)
                ↓
local source expansion (contract D; backend executes, never the model)
                ↓
Source Review / Theme Adjudication semantic call (contract E; shard-ready)
                ↓
local exact validation and reducer (contract F rules)
                ↓
clean Study cards + exact-source drawer
```

Requests follow docs/DEEPSEEK_API_NOTES.md exactly: JSON mode with
`response_format:{"type":"json_object"}`, the word "json" plus an example shape
in the prompt, and explicit `"thinking":{"type":"disabled"}` on the official
DeepSeek endpoint because both stages are bounded classification. Transport
policy unchanged: global ceiling only, transport-only retry.

### Authority contract

Models may create: editorial title, question, why-it-matters, expected
learning, theme-kind tag, candidate anchor groupings, fit assessments
(direct/supporting/weak/irrelevant), supported observations, reading order,
unknowns, theme narrowing/rejection — all editorial prose or hypothesis, never
authority or identity. Models may NOT create: source locations, paths, symbols,
facts, relations, membership, ownership, runtime order, canonical IDs,
successful checks, or acceptance. `f*` refs are leads only (a filename is never
evidence and may only request local expansion); `relation_claim` at Scout is
exactly `editorial_only`; runtime-order claims appear only when an exact
supplied relation proves them, decided locally by the reducer, never by model
prose. A theme is not required to be a proven runtime path.

## 4. Contracts (bounded)

- **Contract A — flat names-only `f*` vocabulary.** Deduplicated eligible
  tracked code + documentation paths; item `{ref,path,language,role}` with
  `role` closed 3-value enum (production_source | test | documentation);
  complete-when-fits else explicit considered/advertised counts + candidate
  digest + coverage-aware omissions; never raw hierarchical tree; no signals,
  reasons, weights, source bytes or canonical IDs in the layer. `f*` never in
  `anchor_refs`.
- **Contract B — exact source-backed `a*` seed anchors.** From compiled local
  substrate only (D211 span reading targets, Architecture behavior anchors when
  usable — tolerant of failed/absent Architecture; Surfaces; accepted
  Documents). Per-seed bounded source pack: path/line/symbol/provenance; full
  body when ≤200 lines & ≤32 KiB via frozen `sourcewindowfacts.ExtractGoFunction`;
  else signature + bounded regions + explicit omitted ranges;
  caller/callsite/callee separated for system-path seeds; bounded related
  tests/documents. Source bytes are provider evidence only, never card content.
- **Contract C — Theme Scout (first semantic call).** Proposes meaningfully
  distinct themes; response `{themes:[{title,question,theme_kind,anchor_refs,
  expansion_file_refs,why_it_matters,expected_learning,relation_claim}]}`.
  Desired 8–12 candidates, valid 1–12; desired 2–5 anchors/theme; one-anchor
  `focused` permitted but must not dominate; `theme_kind` closed enum
  (incl. `shared_domain_responsibility`); `relation_claim` = `editorial_only`
  only. Item-local rejection; zero valid = semantic failure, never fabricated.
- **Contract D — local source expansion (backend executes).** For each accepted
  `f*`: small file → bounded whole source; large file → top-level declaration
  index via one bounded `go/parser` pass then exact selected bodies via
  `sourcewindowfacts`, every non-included range an explicit omitted range.
  Relevance is never inferred from a filename alone. Persisted as
  `theme_source_expansion.v1.json` for rebuild/replay.
- **Contract E — Source Review / Theme Adjudication (second semantic call,
  shard-ready).** Per-candidate sections; classify every anchor
  direct/supporting/weak/irrelevant; write one bounded supported observation;
  may narrow/rewrite title/question, remove weak anchors, reorder reading path,
  or reject the theme; no padding. Response `{themes:[{candidate_ref,
  final_title,final_question,anchor_assessments,reading_order,unknowns}]}`.
  Only direct/supporting publish; weak/irrelevant never appear as readings;
  ≥ 1 direct per accepted theme. Item-local rejection; zero accepted =
  semantic failure.
- **Contract F — local reducer (deterministic).** Publish filter (direct +
  supporting only, ≥ 1 direct); canonical identity
  `theme_id = SHA-256(JSON{sorted accepted anchor canonical refs, theme_kind})`
  (never model prose); dedupe on normalized (question | anchor set | learning
  outcome); catalog-relative balance cap ("no root in >half" only when the
  accepted catalog has enough distinct alternatives; anchor-first removal keeps
  ≥ 1 direct + answerable, else theme dropped, honest count); reading order
  clamped to accepted anchors preserving relative order; exact-relation badge
  upgrade via set membership over compiled ProducerRelations only; zero source
  bytes in card DTO/HTML; no false Architecture-area association.

## 5. State and failure model

- prepared / uncalled / offline: no theme cards; local Atlas + local
  Architecture + neutral local question surface visible.
- Scout failure: no theme shelf; honest "Study themes unavailable" · «Темы
  недоступны»; distinct failed-run neutral browse; local surfaces survive.
- Adjudication failure after Scout success: honest partial; only exactly-
  adjudicated, exactly-validated themes publish; zero accepted → failed, never
  fabricated; four-state browse with published = ∅.
- Reducer-outcome partial: 1–3 valid themes publish with the shelf-level
  partial badge and honest count; 0 → honest unavailable state.
- Invariants: local Atlas/Architecture are never downstream of the theme stages
  and survive every semantic failure; no synthetic/direct-call fallback Study;
  no raw Orientation fallback; no fuzzy ref repair; no semantic retry loop; no
  silent filename-only evidence; historical self-contained reports untouched.

## 6. Report / manifest / version changes

- `AtlasStudyReportProjectionVersion` 7 → 8 (report.go:30).
- `CurrentFormatVersion` 30 → 31 (report.go:28).
- `RunManifest` v11 → v12 (manifest.go:32): `MaterialInputs` gains 8 theme
  artifact SHA-256 fields; `VerifyThemesArtifacts` mirrors
  `VerifyAtlasStudyArtifacts` (manifest.go:984–1039).
- New artifacts: theme_scout_request/result/status.v1.json,
  theme_source_expansion.v1.json, theme_adjudication_request/result/status.v1.json,
  study_themes.v1.json.
- Prompt versions `theme-scout-prompt-v1`, `theme-adjudication-prompt-v1`;
  stage-cache contracts `theme-scout-accepted-v1`,
  `theme-adjudication-accepted-v1` via existing modelresearch machinery.
- `AtlasStudyReportStatus` gains `Themes *AtlasStudyThemesProjection` beside
  `frontier_browse`. Projection gate (manifest.go:682–688) now requires
  ProjectionVersion == 8 — old v7 projections FAIL CLOSED under the v8 binary
  (static HTML only; no compatibility reader, no migration). DeepEqual round-trip
  (manifest.go:1030–1037) requires byte-identical re-derivation of `Themes` and
  the browse from the SHA-bound artifacts.

## 7. Acceptance

Provider-free gates first (contract H order): `go test ./...` + `go vet ./...`;
typed-ref gates (a*/f*/t* resolve exactly, every item-local rejection path);
names-only vocabulary coverage/digest; source-pack bounds + omitted ranges +
full-body vs partial; caller/callsite/callee separation; Scout + Adjudication
saved-response replay (provider_calls=0, exclusive writes, SHA-256 pins, refuses
original run dir, secretscan on all bytes); reducer gates (dedupe, balance cap,
canonical identity stability, ≥ 1 direct, zero source bytes in card DTO/HTML);
failure gates (forced Scout/Adjudication failure → local Atlas + Architecture
survive, no synthetic shelf, no retry); binary offline on casdoor + one large Go
service + one library-shaped Go fixture (exit status, manifest, Atlas, report
JSON/HTML verified directly; provider_calls=0); manifest gate (v7 projection
rejects under v8, SHA mismatch rejects, DeepEqual round-trip); EN + RU (every
message ID in both ui_messages.js catalogs; `--lang ru` correct). Then EXACTLY
ONE fresh Casdoor semantic A/B run on the same revision `2b9a0523…` and same
dirty state as both baselines, comparing OLD / CURRENT / CANDIDATE, judged by
M1–M9 (multi-anchor themes baseline-relative; no root in >half catalog-relative;
no duplicate normalized question/outcome; anchors answer question together;
representative callsite in direct-call packs; zero source bytes on cards; every
reading opens exact source ≤ 2 actions; no false Architecture-area association;
fresh reviewer preference). Acceptance, not prompt tuning; no second live run.

## 8. Non-goals (binding)

No raw Orientation restoration; no model-owned paths/facts/relations/membership/
ownership/runtime order/IDs/checks/acceptance; no full repository source upload;
no raw hierarchical file tree or raw internal graph on any wire; no Tree-sitter
or second language; no embeddings / Meaning lens; no generic workflow/agent
framework; no new Boundary/Resource producer (D212 outputs consumed as exact
seeds only, never widened); no source-code blocks on Study cards; no user-facing
internal support-role flags or fit-class badges; no compatibility reader or
migration (old static HTML stays viewable; v7 projection JSON fails closed); no
third portfolio critic (deferred); no semantic retry loop; no fuzzy ref repair;
no synthetic/direct-call fallback Study; no Casdoor-specific checklist; no
second live A/B run.

