# Decision: Question-aware Study selection guard

Status: Approved by the repository owner through the active overnight
continuation goal after the Study coverage addendum.

## Product outcome

Use the local question coverage signal to prefer Study directions whose saved
anchors and documents visibly cover the question terms, without changing model
prompts, providers, validators, or global limits.

## Approved implementation scope

1. Keep Study directions presentation-first learning plans, not canonical
   Mechanisms and not runtime traces.
2. Add a deterministic local question-fit score to reviewed Study directions.
3. Base the score only on already saved repository objects: direction text,
   retained anchor IDs, retained reviews, anchor statements/symbols/paths,
   document labels/paths/excerpts, area names/responsibilities, and mechanism
   titles/questions.
4. Use the score only inside the existing local reducer when choosing among
   already reviewed candidate directions.
5. Keep the same candidate counts, selected direction bounds, document budgets,
   and provider/model calls.
6. Keep debug/provenance visible in `report.json` and hidden in default user
   UI.

## Truth boundary

Question fit is a local navigation-quality heuristic. It is not semantic proof,
does not verify the whole question, and must not be displayed as a user-facing
verdict.

## Explicit non-goals

- No prompt changes.
- No provider/model changes.
- No retries.
- No limit increases.
- No global call graph, runtime-surface discovery, or repository-wide analysis.
- No weakening of existing Study/Mechanism validators.
- No new renderer or UI exposure of debug diagnostics.

## Focused verification

- Unit tests for question-fit score and reducer preference.
- Replay saved copied runs for restic/repomap where weak coverage was observed.
- Visible UI check that debug strings remain hidden.
- `git diff --check`.
