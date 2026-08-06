# 232 — Navigator and Theme contract simplification

**Status:** ACTIVE (owner-authorized decision B of the Archive 9 semantic-product program)
**Supersedes:** the Navigator v1 echo contract and the exhaustive Theme
Scout/Adjudication bookkeeping clauses named in the Archive 9
prompts-validators-audit v10 (§4, §5).

## Problem (owner pack, P1)

- **Navigator is used as a checksum**: the model echoes `entity_refs`,
  `trail_refs`, `evidence_refs`, `gap_refs` — every one of those is
  backend-owned and fully restorable from the selected `action_ref` against
  the canonical Atlas. The echo is a whole-response failure surface with
  zero semantic content.
- **Theme Scout** requires 8–12 themes unconditionally (padding pressure),
  and duplicate anchors/files are not normalized.
- **Theme Adjudication** requires every anchor to receive an assessment
  (exhaustive bookkeeping; a missing assessment poisons the theme), asks the
  model to repeat the backend-owned anchor `role`, requires observations even
  for weak/irrelevant anchors, and has no unreviewed/semantic-empty
  publication path.

## Decision

### Navigator v2 (`internal/deepseek/navigator.go`)

Model returns only:

```json
{
  "version": 2,
  "catalog_ref": "exact catalog_ref from the request",
  "action_ref": "exactly one advertised action ref"
}
```

Backend restores from the selected action record, exactly and closed:

- trail (the action's direct trail);
- both endpoint entity refs (source/target);
- every evidence ref attached to that trail;
- the operation;
- the canonical action record for product projection.

Keep exact catalog validation and exact action_ref validation (unknown
action ref → whole-response ReferenceError, closed, with the code in the
counted finding; no fallback silently). Remove the "selected action target_ref
must equal the selected trail source_ref" model-side constraint (backend
re-derives and validates). No model echo of backend-owned evidence.

### Theme Scout (`internal/themestudy`)

Prompt wording:

- "Propose 8–12 themes (valid 1–12)" → **"Aim for 8–12 when distinct
  evidence supports them. Return 1–12; fewer is better than overlap or
  filler."**
- duplicate exact anchor/file refs → normalized and counted locally;
  candidate retained (equivalence preserved for Adjudication/co-projection).
- zero valid semantic themes → **semantic-empty status + complete local
  question browse** published (never fabricated cards, never hidden
  information).

### Theme Adjudication (`internal/themestudy`)

- missing anchor assessment → local `unreviewed`: counted in coverage,
  NOT published as a reading, never poisons the theme;
- anchor `role` → backend-restored from the catalog (removed from the model
  response contract);
- `supported_observation` required only for direct/supporting; weak/
  irrelevant may carry an optional short rejection reason;
- duplicate assessments → normalized (counted);
- reading_order: unknown/duplicate refs drop locally; accepted
  direct/supporting anchors retained; if no order remains → deterministic
  accepted-anchor order labelled **backend default** (model order is
  editorial, never runtime order);
- no direct reading → theme rejected from publication WITH a retained
  diagnostic (siblings never poisoned);
- zero accepted themes → semantic-empty + local browse.

### Fresh review verdicts (all applied)

Product: PASS with 6 bounded defects:
- D1 (problem wording): the current validator never poisons a theme for a
  partial assessment set; the *contract* demanded exhaustive bookkeeping with
  no unreviewed publication path. Fixed in the problem statement and in the
  prompt.
- D2 (missing acceptance tests): semantic duplicate co-projection, Navigator
  catalog-permutation invariance, and duplicate/extra field schema tests were
  unpinned — added (themestudy d232_contract_simplification_test.go,
  navigator product_test.go permutation test, existing reject tables).
- D3 (projection clause un-pinned): Study report projection now carries
  ReviewedAnchors/UnreviewedAnchors with a named bump
  (AtlasStudyReportProjectionVersion 9→10, CurrentFormatVersion 32→33;
  manifest artifact set unchanged). Golden + version tests updated.
- D4 (backend-default surface): the deterministic reading-order fallback is
  the backend default accepted-anchor order, derived in reduce.go and
  labelled in the Study card badge (no model order is runtime order).
- D5 (version drift): ProductVersion 1→2 and navigatorCacheContract →v3.
- D6 (wording): singular/plural action_ref(s) aligned — the wire uses
  `action_refs` (array); the anchor `role` restore source is the catalog
  constant derivation (no catalog role field exists; the archive role is a
  constant public_entry, restored by the backend).

Red-team: PASS with 5 bounded fixes:
- Q1: restore rule pinned — action_ref → catalog key → byKey →
  RecommendationAction (trail = the Atlas relation with the selected
  RelationID; never target-matching). Unknown action_ref stays a
  whole-response ReferenceError (closed), not a new item-scope path.
  Replay of the 5 saved v1 responses extracts catalog_ref+action_refs and
  runs the v2 restore, comparing the backend record byte-for-byte.
- Q2: report projection verified echo-free (navigator_result.v1.json
  Selected is backend-owned; the UI re-derives the trail from relation_id +
  surface + application_operation + evidence_ids).
- Q3/Q6: zero accepted themes is now a semantic-empty result: the failed
  status + empty result publish, the report renders the complete local
  question browse (deriveAtlasStudyFailedBrowse) with a failed state and
  FailureValidation — no hard error, no hidden information, no fabricated
  cards.
- Q4: AdjudicationStatus gains ReviewedAnchors/UnreviewedAnchors, computed
  in ValidateAdjudication and projected into the Study report status.
- Q5: duplicate anchor/file refs (Scout) and duplicate assessments
  (Adjudication) normalize deterministically (keep first) and are counted
  under typed Normalized keys — never reject a valid candidate/theme.

### Version/cache/replay identities

- NavigatorPromptVersionJSON v1 → v2; navigator result/status versions
  advance; navigator cache key + replay identity advance; report
  manifest binding advances. Old v1 records fail closed.
- ScoutPromptVersion + ScoutResultVersion advance; scout cache/replay
  advance.
- AdjudicationPromptVersion + AdjudicationResultVersion advance;
  adjudication cache/replay advance.
- Report projection (Study) advances with the unreviewed/equivalence/
  semantic-empty states.

## Acceptance (provider-free, Archive 9 replay)

Navigator:

- each of the 5 saved Archive 9 navigator responses (casdoor s1→o1/t1/e1,
  etcd s1→o8/t12/e18, restic s1→o3/t2/e3, telebot/chatto navigator
  artifacts) replays under v2 semantics: model action_ref + catalog_ref →
  backend restores EXACTLY the recorded trail/endpoints/evidence; byte-equal
  canonical action record; no echo needed.
- unknown action_ref → item-scope reject with counted finding.

Scout/Adjudication:

- duplicate anchor/file injection changes only normalization counts;
- missing assessment → unreviewed, counted, unpublished, theme survives;
- role removed from model output → backend restores it;
- no-direct theme → rejected with diagnostic, siblings survive;
- zero accepted themes → semantic-empty + local browse;
- duplicate assessments normalize.

Fresh code/contract/product reviews, up to two bounded repair cycles.
Commit implementation and continue automatically.

## Docs

- `PROMPT_VALIDATOR_MATRIX.md` (D231 run dir) rows for Navigator/Scout/
  Adjudication record the audit this decision implements.
