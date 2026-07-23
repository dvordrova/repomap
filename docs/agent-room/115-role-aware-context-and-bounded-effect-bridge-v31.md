# Decision: Repomap v3.1 Role-aware Context and Bounded Effect Bridge

Status: Approved by the repository owner in the current implementation
session.

## Product outcome

Use the failed v3.0 Litestream checkpoint as the fixed product regression.
Improve deterministic context selection and bounded cross-package effect
resolution without changing the report, provider, prompts, validators, token
or fact budgets, candidate limits, or one-call synthesis contract.

## Approved implementation scope

1. Classify repository artifacts as primary production entry, production core,
   effect/integration boundary, public API, example, test, fixture, generated,
   playground/preview/evaluator, experimental, current documentation, or
   historical/decision documentation.
2. Use those roles inside existing limits when selecting module summaries,
   entrypoints, important edges, documentation, source signals, semantic facts,
   and candidate anchors.
3. Prevent fixture/tooling artifacts from consuming production slots while
   relevant production artifacts exist. Rank executable/public entrypoints and
   current operational documentation above previews, evaluators, fixtures, and
   historical decisions.
4. Preserve exact static connectivity from selected primary entries into
   production core when the existing local import graph contains it. Tests stay
   optional evidence and are not required for Mechanism publication.
5. Extend the existing candidate-specific deterministic planner only. Starting
   from an exact locally proven call, resolve repository-local exact targets,
   follow at most two additional frontiers, inspect at most four additional
   files and ten functions, and stop at a typed effect or integration boundary.
6. Allowed terminal boundaries are file write, database mutation, network send,
   backend/client interface handoff, persisted artifact, and public output. An
   interface call proves only the handoff, not a concrete remote implementation.
7. Preserve exact source, opaque facts, replayability, local verdict authority,
   and all v3.0 publication checks.

## Fixed regression

Rerun the complete v3.0 product gate on frozen Litestream revision
`d26cb54ec43b5937a0c8b3bd875696c1375d8cb3` with the same provider/model,
validators, total budgets, maximum candidates, one synthesis call per eligible
candidate, no semantic retry, and unchanged User Report behavior.

Success requires a fresh natural central question, an accepted input/core/effect
Mechanism with at least three meaningful source-backed phases, Start Here
publication, natural behavior Search routing, and no unsupported visible claim.
On failure, preserve the exact bounded reason and classify it as unresolved
dynamic dispatch, insufficient cross-package connectivity, missing effect
classification, context-role selection failure, or bounded static-analysis
limit.

## Explicit non-goals

- No global call graph, points-to analysis, repository-wide SSA, runtime-surface
  discovery, or repository-specific rule.
- No new provider, prompt expansion, token/fact/candidate increase, semantic
  retry, weakened validation, response editing, or UI change.
- No second repository or attractive manual story to compensate for a failed
  fixed regression.

## Focused verification

- role classification and selection tests;
- cross-package/effect resolution tests;
- Caddy and chi offline Mechanism regressions;
- fixed Litestream product replay and cold gate;
- report/Search smoke;
- final diff audit.

Full repository test coverage and slow runtime-surface analysis are not required
for this MVP iteration.

## Stop condition

If the unchanged fixed gate still fails after the two bounded deterministic
improvements, stop and report the exact capability class. Do not compensate
through prose, UI, longer prompts, weaker validation, more model calls, or a
different repository.
