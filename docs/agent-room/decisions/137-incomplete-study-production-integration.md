# Decision: Incomplete Study Production Integration

Status: Complete. Approved by the repository owner and product supervisor in
the current session.

## Product outcome

When the bounded Study candidate response contains useful questions with exact
catalog-resolved reading starts but those candidates do not satisfy the
unchanged complete three-to-five-anchor Reading Pack contract, the normal
report must retain those questions as visibly incomplete Study directions.

The existing incomplete topics remain the Overview front door. One Study route
shows the broader retained questions in provider order. One further click opens
a direction with:

- why it matters;
- what can be learned;
- one or more exact saved path, symbol, line, and reading instructions; and
- an explicit statement that the complete reading path is not yet explained.

Search remains absent from the normal report.

## Saved authority and projection boundary

Replay only the already saved bounded artifacts:

- `repository_study_map_bundle.json`; and
- `study_direction_candidates_attempt.json`.

The attempt must be versioned, use the existing Study candidates prompt
version, contain a JSON response, and bind to the exact canonical bundle hash.
The bundle remains the sole authority for anchor IDs and paths.

Process at most the existing twelve candidate items in provider order. An
incomplete direction is eligible only when:

1. its question, relevance, learning outcome, target job, and learning stage
   satisfy existing scalar rules;
2. its model-supplied direction ID is empty;
3. at least one and at most five reading entries are individually valid;
4. every retained reading entry resolves to an exact anchor in the saved
   bundle; and
5. its local ID does not duplicate an already published complete Study
   direction.

Reject invalid candidates independently. Never repair a missing anchor, copy
an unresolved path, infer an instruction, or use the model's broader
`anchor_ids` collection as authority for the incomplete projection.

## Compatibility boundary

- Do not change the provider prompt, request shape, model settings, complete
  candidate decoder, complete Reading Pack review, or complete publication
  gate.
- Do not change existing complete Study JSON fields or IDs.
- The incomplete projection is an additive presentation field and route.
- Preserve existing topic, mechanism, source-open, Architecture, report
  manifest, v3/v4, and HTTP behavior.
- Do not add Search, MCP, a new analysis framework, or another provider call.
- Do not run another repository or provider during implementation.

## Acceptance

Provider-free fixtures and browser checks must prove:

1. the retained Decision 135 response projects twelve incomplete directions in
   provider order;
2. all published starts resolve to the exact saved catalog and no unresolved
   or invented path reaches report JSON;
3. complete candidates remain complete and are not duplicated as incomplete;
4. invalid scalar or reading metadata rejects only its candidate;
5. the three existing topics remain on Overview;
6. Study is one click from Overview and a direction is one further click;
7. incomplete detail contains the exact start and the honest boundary copy;
8. normal complete Study behavior remains unchanged; and
9. Search remains absent.

Run focused tests, `git diff --check`, and `./scripts/check.sh`. Re-render the
saved Decision 135 Chatto run without a provider call and inspect Overview,
Study, direction detail, and exact source-open behavior before checkpointing.

## Stop condition

Stop without production commit if the projection cannot remain catalog-bound,
requires weakening the complete gate, changes existing complete Study wire
fields, or cannot coexist with the three topic Overview without adding a
parallel presentation framework.

## Result

The production integration replays only the hash-bound saved Study bundle and
candidate attempt. The retained Chatto run projected all twelve directions in
provider order, with one exact catalog-resolved reading start per direction,
zero unresolved published starts, zero complete Study directions, and no
change to the complete decoder, review gate, or provider prompt.

An authority-bound provider-free copy of run
`20260729-034513-chatto-1adc0451c0c3` was served against the current Chatto
checkout. Browser verification established:

1. Overview retains the three existing local topics and adds one Study entry;
2. Study is one click away and exposes all twelve scannable questions;
3. a direction opens in one further click and explicitly denies having a
   complete reading path;
4. exact source for `cli/internal/core/core.go:1530` opens in the verified
   drawer with editor, full-function, and copy actions;
5. visible navigation is exactly Overview, Study, and Architecture;
6. a direct `#/search` route canonicalizes to `#/overview`; and
7. the browser reported no warning or error logs.

The product supervisor inspected production screenshots of Overview, Study,
direction detail, and the exact source drawer and returned
`VERDICT: ACCEPT D137` with no blocker. It explicitly accepted the latent,
unused `semantic_search` index in `report.json` as outside this presentation
slice because no Search UI or route is exposed.

Focused Study/report tests passed. The first full check exposed the existing
editor-launcher test's read-during-write race; the isolated test then passed
ten consecutive runs without a source change, and the complete check passed on
the next run, including all Go tests, vet, and six offline quality replays.
