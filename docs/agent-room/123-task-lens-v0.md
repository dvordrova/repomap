# Decision: Task Lens v0 bounded product experiment

Status: Approved by the repository owner through the attached Task Lens v0
implementation and evaluation request.

## Product outcome

Add an opt-in, ephemeral task-conditioned investigation overlay that selects a
small exact repository context, preserves evidence and hypothesis boundaries,
and gives grounded reproduce/observe and verification guidance without running
the generic onboarding editorial pipeline.

The canonical Repository Guide remains unchanged and usable without a task.

## Approved implementation scope

1. Add a first-class experimental command:

   ```text
   repomap investigate <repository> --task-file <path>
   ```

2. Build one bounded local Task Investigation Bundle from tracked repository
   facts, exact declarations/references, bounded source or document windows,
   and locally witnessed relations. Enforce the experiment budgets recorded in
   the owner prompt.
3. Permit zero or one narrow synthesis call in the implemented path. Local
   exact tasks may skip synthesis. Do not enter Orientation, Architecture,
   Guided Tour, Mechanism, Study, or Paved Path stages.
4. Validate every selected anchor, evidence reference, support type, hypothesis
   label, and guidance authority locally. Model prose does not become a local
   fact or observed relation.
5. Persist the four versioned Task Lens artifacts and bind the accepted pack,
   report, and exact repository state with hashes.
6. Add one optional `#/investigate/<task-id>` workspace to the existing report
   renderer. Do not build a second production frontend and do not lead task
   mode with generic Start Here content.
7. Repair benchmark revision identity with real detached worktrees and neutral
   repository basenames. Keep a separate Git-free source export for any future
   source-only condition.
8. Make canonical model-visible repository identity independent of a checkout
   basename, preferring a root module path or normalized remote identity while
   retaining the basename only as display copy.
9. Evaluate the six development episodes, freeze code/prompts/schemas/budgets,
   generate each holdout once in a gold-isolated workspace, seal outputs, and
   only then open supervisor gold.

## Truth boundary

Task text is user-provided evidence, not repository truth. Repository facts,
document claims, task-provided symptoms, model hypotheses, and unresolved
questions remain distinct in saved artifacts and presentation.

A locally observed relation states only the exact static or textual relation
captured by its evidence and carries an explicit non-guarantee. It never implies
runtime execution, ordering, causality, or completeness unless separately
proved by an authorized evidence source.

## Explicit non-goals

- No universal KnowledgeObject framework.
- No issue-tracker integration or autonomous fix implementation.
- No repository-wide SSA, pointer analysis, or global call graph.
- No arbitrary target-repository command execution.
- No provider/model switch or general model-routing framework.
- No repository-specific production branches, aliases, paths, or examples.
- No silent fallback to the generic Repository Guide when bounded evidence is
  insufficient.
- No independent answer-quality A/B claim without separate sealed sessions.

## Focused verification

- Task bundle, proposal reduction, evidence-scope, and secret-boundary tests.
- Task-text versus repository-truth separation tests.
- Cheap-exit classification and bounded retrieval tests.
- Checkout-basename semantic-invariance regression.
- Detached-worktree revision/tree identity regression.
- Task report replay, route, source projection, manifest, and browser smoke.
- Holdout freeze and one-attempt seal checks.
- `git diff --check`, `./scripts/check.sh`, and `./scripts/etcd_check.sh ../etcd`
  when the nearby etcd checkout is available.
