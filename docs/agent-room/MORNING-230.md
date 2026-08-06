# MORNING-230 — Archive 8 desktop interaction, source-reference and canvas corrective

**Goal:** `hermes-repomap-archive8-interaction-canvas-goal-v9.txt` (Архив 8.zip + interaction audit v9 + information preservation charter v5 + architecture distinctness salvage v6 + diagram representation v8)
**Date:** 2026-08-06
**Decision:** `docs/agent-room/decisions/230-archive8-interaction-corrective.md`
**HEAD:** `7753f5e` (9 commits, a72cd13..HEAD)
**Status:** COMPLETE — all fresh-review verdicts applied

## Baseline

- main @ a72cd13, clean; next decision: 230.
- Archive 8: 5 reports (casdoor/etcd/telebot/chatto/restic).
- Audit v9 reproduced at local HEAD in a real browser: inert Study previews (23/23 casdoor), nested association buttons, canvas wheel trap, Fit leaving «Запуск» outside + toolbar interception, inspector overscroll auto + close→body, Architecture 12,498px wall, mechanism 47-lane ordinal stack, etcd whole-reject, restic 5×48 identical members, telebot/chatto equivalence classes.

## Decision 230

Candidate B (behavioral corrective using existing data). Rejected A (CSS-only) and C (new analyzer). Two decision critics applied before the decision commit.

## Commits (9)

1. `b0a315e` decision + CURRENT.md
2. `abc9832` PHASE 3-5 source actions + disclosure + canvas
3. `5b0db25` PHASE 6-8 inspector/disclosures/mechanism fragments
4. `a3e7f72` PHASE 9 semantic (etcd/D4/scope/RU)
5. `889bc71` D9.7 shared-unit anchor slice (restic)
6. `48dd3f3` prompt v17 restore (chatto)
7. `092deec` review B1/B2: salvage registry (full-coalescence small bundles)
8. `03e15c2` review B2+S1+S3: prompt version, association nesting regression test, gofmt
9. `7753f5e` review verdicts: symbol-anchor slice resolution, D4 alternates projection, mechanism/relations static actions, focus trap, hover states

## Fresh reviews (5 critics, all applied)

| Review | Verdict | Blockers fixed |
|---|---|---|
| Code/regression | NOT PASS → fixed | D4 full-coalescence state; prompt version v18→v17; +S1 nesting test, S3 gofmt |
| Semantic-contract | BLOCKED → fixed | D9.7 symbol-anchor slice (restic whole-reject); D4 alternates projected + rendered |
| Desktop UX | PASS w/ 1 blocker → fixed | mechanism/relations source refs → static pinned links |
| Diagram/product | PASS w/ 1 blocker → fixed | same source-actionability blocker |
| A11y/keyboard | NOT PASS → fixed | focus trap hidden controls + backdrop tabindex; hover states |

## Key results (live matrix, exact built binary)

| Repo | Before | After |
|---|---|---|
| Casdoor | 12,498px; 8/19 hit-test; «Запуск» occluded; 218 obs via root | 1,260px; 17/17 after Fit; exact scope; 17 components |
| etcd | whole-reject, EN fallback labels | accepted_partial (251 requested = 111+140), RU localized |
| Telebot | 3 components over same u1 | 2 clean components |
| Chatto | 3 components same member set | 6 distinct member sets |
| Restic | 5×48 identical members | distinct anchor slices + remainder; no whole-reject |

## Gates

- `go test ./...` SUITE_EXIT=0 (0 FAIL); `go vet` clean; node --check all 3 JS; quality/localization/surface-check green; golden regenerated; diff --check clean.
- Version identities: ContractVersion 10, ProposalVersion 10, prompt v17; status registry +9 codes.
- Browser-verified: mechanism locations 3/3 static links (0 inert), relation sources 3/3 links, focus trap excludes 18 hidden controls, backdrop tabIndex -1, nested-interactive 0.
