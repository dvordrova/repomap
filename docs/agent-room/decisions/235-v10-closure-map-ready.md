# 235 — V10 closure / MAP-READY contracts

**Status:** ACTIVE (v11 Decision 1; owner-authorized standing goal)
**Supersedes:** the v18 prompt's unit_refs branch; the D213-era Study
independence from accepted Architecture; whole-response rejection of
mechanically-defective Architecture proposals.
**Deferred:** Decision 236 (Repository Map primary product) — starts only
after MAP_READY is green.

## Candidate selection (three independent designs)

- **A — prompt-only stricter instructions, keep current validators:** REJECTED.
  v10 evidence shows the model already makes useful semantic choices (27/37
  accepted model Architectures; 266 accepted component records; useful exact
  member sets; 0 duplicate member-set pairs). The mismatch is the
  prompt/backend contract, not model capability. Prompt-only cannot fix the
  three mechanical whole-rejections (Gotify trailing `]}`, Soft Serve missing
  `anchor_refs`, goargs empty sibling) because they are backend decoder/
  validator failures, not prompt failures.
- **C — broad new semantic pipeline/model call:** REJECTED. Every needed fact
  already exists locally (Atlas, grounding, surfaces, accepted canvas). The
  Study rebase defect is a compile-order bug, not a missing semantic stage.
- **B — one member grammar + backend normalization + item-local rejection +
  Study rebase + bounded reliability fixes:** SELECTED. Closes the observed
  contract mismatch and the eight corpus failures with deterministic backend
  work and zero new provider calls.

## 1A. One Architecture response grammar

### Model output (prompt v19)

```json
{
  "records": [
    {"kind":"subsystem","ref":"g1","name":"...","description":"..."},
    {"kind":"component","subsystem_ref":"g1","name":"...","description":"...",
     "member_refs":[{"ref":"p1"}],"anchor_refs":[{"ref":"a1"}]}
  ]
}
```

Binding rules:

- response uses `member_refs` only; `unit_refs` is not part of the response
  grammar (266/266 accepted corpus components already use member_refs);
- the member-only grammar is MODEL-FACING (prompt v19); the backend wire
  decoder retains the `unit_refs` record shape (unit expansion → shared-unit
  participation, D9.7/D231) so provider-free replay of pre-v19 saved bytes
  stays accepted (goargs «Плагин» is a unit_refs record and must publish);
- units remain read-only grouping/context input in the request;
- no legacy response branch is shown to the model;
- model does not author kind beside an opaque ref; no canonical IDs, order
  IDs, coverage, remainder, normalization, ownership, or relation truth.

### Backend normalization

- missing `anchor_refs` → empty array + normalization count
  (Soft Serve: 14 useful components accepted_with_normalization);
- duplicate member/anchor refs inside an item → stable dedupe + count;
- empty component (no members, no units, no anchors) → reject that component
  ONLY (goargs: «Линтеры» dropped item-local, «Плагин» publishes,
  accepted_partial);
- unknown required member → reject dependent component only (already
  item-local, D229);
- unknown optional anchor → drop anchor only;
- valid siblings survive (already D229/D231);
- equivalent resolved member sets remain coalesced/conflicted per v10;
- zero valid semantic components → local-only Architecture (already D231).

### Gotify format recovery — Option 1 (selected)

Deterministic normalization ONLY when exactly one complete JSON object parses
and every trailing byte is whitespace or a bounded sequence of unmatched
closing `]`/`}` delimiters (≤ 8 closing delimiters). Record
`response.trailing_closing_delimiters_normalized`; rerun full semantic
validation. Any other trailing content remains invalid. No broad JSON repair,
no embedded-JSON extraction, no second provider call (Option 2 rejected:
retry costs a provider round-trip for a mechanically deterministic fix).

### Identity

Audit/bump: SynthesisPromptVersion v18→v19; SynthesisRequestVersion 15→16
(response grammar change); SynthesisRecordVersion 12→13; ProposalVersion
11→12; ContractVersion 11→12; ArchitectureSynthesisStatusVersion 10→11;
ArchitectureCanvasVersion 10→11; semantic exchange validation identity; cache
contract. Old identities miss closed; no active migration.

### Provider-free hard cases

- all 27 accepted corpus Architecture responses remain accepted;
- Soft Serve → accepted_with_normalization;
- goargs → accepted_partial («Плагин» published, «Линтеры» item-local);
- Gotify → accepted with trailing-closing-delimiters normalization (or
  unchanged explicit format failure);
- no new duplicate component member-set classes;
- coverage/remainder counts reconcile.

## 1B. Final Architecture rebase into Study

### Scout context compiled after final Architecture resolution

- Architecture validated_model / partial_model / normalized_model (any
  accepted non-fallback model source): Scout context uses the ACCEPTED FINAL
  components — bounded fields: component ref, user-facing name, one-line
  responsibility, exact member/source-role counts, entry participation count,
  touchpoint family count, partial/full/shared/facet state. Never stale
  pre-synthesis names; never all members or source bytes.
  (normalized_model included so Soft Serve's accepted_with_normalization
  canvas feeds Study — MAP_READY "final Architecture used by Study".)
  source bytes.
- Architecture failed / unavailable / local_only: use local Architecture and
  state the authority explicitly.

Implementation: `atlasStudyLocalD177Data` (atlas_study.go:127) currently
ALWAYS rebuilds a local D177 canvas and replaces the accepted model canvas.
Change: when the saved canvas source is validated_model/partial_model and the
canvas is usable, Study input compiles from the ACCEPTED canvas (rebuild only
the surfaces/anchors/flows joins that do not resolve, never replace
component identities). The D177 rebuild remains the failed/unavailable path.

### Span questions

`themeScoutContext` reads `object.Label` (always empty for RefRouteSpan) —
fix to read the backend-owned `object.Question` and `object.SpanKind`.
Populate from actual backend-owned non-empty questions; when a span has no
backend-owned question, omit the field entirely. No placeholder objects.
(712/712 empty today.)

### Theme equivalence (after Adjudication, deterministic)

Classify final themes by: exact reading identity set + accepted Architecture
component refs + theme kind + direct/supporting role +
production/test/tooling source role + central question family. Equivalent
themes → one primary theme, alternate questions/titles, union of exact
readings, complete original ID/accounting record. Evidence is never
discarded. Near-overlap below exact equivalence may influence
ordering/grouping but must not silently merge unrelated questions.
Corpus targets: 17 exact duplicate pairs and 55 Jaccard≥0.5 pairs reduced
without evidence loss.

### Portfolio priority (comparative guidance, no keyword tables)

1. central production journey/responsibility; 2. state/resource lifecycle;
3. async/background mechanism; 4. boundary/integration; 5. cross-cutting
policy; 6. verification/tooling/docs/release.

Fixture expectations: Miniflux feed ingestion/storage/request lifecycle not
hidden behind TLS/HTTP-client; Gotify message/WebSocket/application flow
outranks config/TLS; Task execution graph outranks release/sleepit; Gosec
analysis/rules/findings outrank repeated TLS; Restic backup/restore/prune/
repository outranks release/docs/debug; Casdoor identity/access/users/
policies/lifecycle outrank integration variants; Telebot construct/register/
dispatch/polling-webhook/API lifecycle clear. No third semantic call.

## 1C. Online Go toolchain selection

- Use standard `GOTOOLCHAIN=auto` for target analysis (already landed as the
  REPOMAP_GOTOOLCHAIN knob at 870d98f; keep the knob, translate to
  GOTOOLCHAIN for the packages.Load env only).
- Repomap build toolchain must not cap the target patch version.
- Record the selected target analysis toolchain (provenance) in the run.
- Cold download/selection and warm reuse work.
- Failure is typed `analysis_toolchain_unavailable`; generic
  snapshot/report remains available on failure.
- Regression fixture: Repomap built with go1.26.4, target requires go1.26.5,
  online analysis selects go1.26.5, packages/surfaces succeed.
- Recheck Soft Serve, GitLab CLI, Syn.

## 1D. Local failure containment

The following local failures may not erase already accepted upstream products.

- **Unsafe source window (maddy):** the mandatory secret scan is partitioned
  per expansion file (cmd/repomap/theme_study_runtime.go:343-370) — an unsafe
  file is closed with a typed `closed_reason` (never echoing content), safe
  files and the accepted Scout result survive; the whole-payload scan remains
  as the final net. Contract-D bindings survive because the closed file keeps
  its ref (ExpansionFile.Closed/ClosedReason, themestudy/model.go).
- **Oversized source window (ghz):** already fixed (cde6347); bounded
  clipping preserves valid siblings and records exact omission count/reason.
- **Architecture unavailable (sqlc/syn/bench):** Study runs from the usable
  Atlas/local reading catalog — BuildAtlasStudyInput tolerates a missing
  canvas when Architecture is unavailable (atlas_study.go:43-52,
  atlasStudyInputWithoutCanvas), atlasStudySurfaces falls back to Atlas
  surface entities, compileCatalog accepts a local-source empty Architecture
  block (atlasstudy/compile.go:355-363), and
  errNoCanonicalArchitectureCandidates is publishable
  (cmd/repomap/architecture_synthesis.go:1256-1261) so the run continues to
  a minimal local report.
- **External source (container-registry):** out-of-root locations
  ($GOROOT/stdlib, module cache) are rewritten to the `<external>/<base>`
  marker (surfacediscovery/analyzer.go:4209-4231) and
  collectDiscoveredSurfacePaths skips external/non-repo-relative paths
  (report/surfaces.go:1510-1528) — external source can never become a
  required repository source action; local callsite evidence is retained.
- **Post-Scout failure observability (chatto):** a local failure after an
  accepted Scout writes a typed terminal status (the adjudication stage
  persists it) and the run continues to the report
  (cmd/repomap/theme_study_runtime.go:397-411) — accepted upstream artifacts
  remain inspectable.
- **Conditional same-decision fixes (all implemented, bounded + tested):**
  - Caddy candidate facts: the builder caps facts deterministically at
    MaxFactsPerCandidate (sorted canonical order, typed omission diagnostic
    builder.candidate_facts_capped) — the bundle validator never trips
    whole-bundle (report/architecture_canvas_build.go:874-881, test
    v11_caddy_cap_test.go, provider-free caddy run: 425 candidates pass).
  - Gemnasium duplicate package path: an exact duplicate (same path, same
    owning module) merges deterministically (keep first, no whole-Atlas
    failure); a path owned by two different modules stays a loud typed
    conflict error (repositoryatlas/goadapter/adapter.go:127-139).
  - No canonical candidates: publishable minimal local report
    (sqlc/syn/bench row above).

## MAP_READY gate

`MAP_READY.md` — every applicable row PASS for Casdoor, Telebot, Chatto,
Restic, Miniflux, Gotify, Task, Lazygit, Gosec:

- one Architecture response grammar;
- item-local normalization/salvage;
- no equivalent component multiplication;
- final Architecture used by Study;
- zero blank span questions;
- theme equivalence accounting;
- toolchain provenance;
- source failure containment;
- explicit unclassified remainder;
- version/cache/replay correctness.

Decision 2 (236) starts only after MAP_READY is green.

## Version/cache/replay identities

- SynthesisPromptVersion v18 → v19 (prompt bytes change).
- SynthesisRequestVersion 15 → 16 (member-only response grammar).
- SynthesisRecordVersion 12 → 13; ProposalVersion 11 → 12;
  ContractVersion 11 → 12; ArchitectureSynthesisStatusVersion 10 → 11;
  ArchitectureCanvasVersion 10 → 11 (landscape/status semantics change).
- Scout/Adjudication/StudyThemes versions advance with the rebase and
  equivalence changes (ScoutRequestVersion 1→2, ScoutResultVersion 2→3,
  AdjudicationRequestVersion 1→2, AdjudicationResultVersion 2→3,
  ScoutPromptVersion/AdjudicationPromptVersion v2→v3, StudyThemesVersion
  2→3 with the three literal "v2" sites — themestudy/artifact.go:303,315,
  cmd/repomap/theme_study_runtime.go:433 — flipped to the constant;
  AtlasStudyReportProjectionVersion 12→13, CurrentFormatVersion 35→36).
  The theme-scout-accepted-v1 / theme-adjudication-accepted-v1 cache
  strings need no bump: cache fingerprints embed the RequestSHA256, so a
  rebased context invalidates old entries automatically.
- Replay: old records fail closed on identity mismatch; raw saved response
  bytes replayed through the new code are the deliberate acceptance path.
  Pinned tests amended: atlas_study_test.go (partial rebase semantics),
  workspace_entrypoint_test.go, semantic_discovery_test.go,
  navigator_test.go, workspace_graph_test.go, exact_workspace_search_test.go
  (CurrentFormatVersion 36), architecture_localization_stage_test.go
  (prompt v19 SHA), theme_study_fixture_test.go (themes v3), render_test.go
  golden (report format version).
