# Decision: Targeted Round Selection Diversity

Status: Complete. Local checkpoint authorized by the repository owner and
product supervisor in the current session.

## Product problem

The current targeted-research planner ranks eligible questions only by impact
and new-evidence count. In the retained Beets run this spent both available
rounds on overlapping CLI/import paths:

- `cli-dispatch`: score 8;
- `import-pipeline-start`: score 8; and
- `plugin-api`: score 4, skipped as `targeted_round_limit`.

The resulting product exposed three narrow topics from one command area and
missed the distinct plugin area. Lowering the Study publication threshold does
not fix that upstream breadth loss.

## Accepted microexperiment

One provider-free test-only fixture reproduced the exact 8/8/4 plans. Using
only each already-built `PlannedRound.Bundle.ProviderAllowedPaths`, it kept the
highest-scoring CLI round and selected the plugin round for the second slot by
minimum average shared directory-prefix depth. The focused test passed without
changing production code.

The product supervisor accepted the result with no blocker.

## Authorized corrective

Change only second-slot selection inside `PlanTargetedRounds`:

1. preserve all existing eligibility gates and evidence assembly;
2. preserve the highest-scoring eligible first round;
3. when another slot is available, prefer the eligible round whose bounded
   provider-visible paths have the least average shared directory prefix with
   already selected rounds;
4. use score and the existing stable question-ID order only as tie-breakers;
5. preserve `MaxTargetedRounds`, skipped reasons, provider request bounds, and
   no-new-evidence handling.

Do not add semantic clustering, a new score model, a new provider call, a new
analysis layer, or a presentation change.

## Acceptance

- An exact Beets-shaped regression fixture must retain scores 8/8/4 and select
  `cli-dispatch` plus `plugin-api`.
- Existing one-round and ordinary small fixtures must remain unchanged.
- The hard two-round cap and `targeted_round_limit` reason must remain intact.
- Run focused planner tests, `git diff --check`, and `./scripts/check.sh`.
- Record durations, but do not treat elapsed time as a stop condition at this
  product stage.

## Stop condition

Stop without a production commit if diversity requires changing eligibility,
collecting more repository data, increasing the round budget, changing a
provider-visible schema or prompt, or weakening any evidence/security gate.

## Verification result

The permanent Beets-shaped regression retained scores 8/8/4, selected
`cli-dispatch` plus `plugin-api`, and left `import-pipeline-start` skipped as
`targeted_round_limit`. The existing focused exact-evidence, no-new-evidence,
and hard-cap tests passed in the same focused command.

Recorded commands and durations:

- focused four-test planner command: package completed in 0.527 seconds;
- `go test ./internal/modelresearch -count=1`: 0.66 seconds wall time;
- `./scripts/check.sh`: passed in 46.16 seconds wall time, including all Go
  tests, `go vet`, and six offline quality replays; and
- `./scripts/etcd_check.sh ../etcd`: passed in 17.33 seconds wall time.

`git diff --check` passed. No provider or product repository run was made, no
timeout-based stop was added, and the repository owner's untracked Caddy
experiment files were not modified.

The product supervisor accepted D139 with no blocker. The expected product
effect is broader candidate coverage on a fresh Beets run, not a guarantee of
Study publication or useful plugin evidence. The authorized next action is
exactly one local checkpoint followed by rebuilding the stable PATH binary; no
push or repository/provider run is part of this decision.
