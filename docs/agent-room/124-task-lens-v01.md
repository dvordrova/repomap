# Decision: Task Lens v0.1 decisive-anchor completion

Status: Approved by the repository owner through the attached Task Lens v0.1
implementation and development-regression request.

## Product question

Can the bounded retriever complete the decisive source relation and find one
grounded verification anchor before asking the model to narrate the task?

## Approved implementation scope

1. Preserve the exact Task Lens v0 freeze, manifests, packs, scorecard, and
   hashes as a read-only baseline under `tmp/task-lens-v01/baseline/`.
2. Add explicit retained-source scope metadata. Prefer complete Go functions,
   complete document sections, and complete operational/config files up to
   64 KiB. Partial scopes must expose truncation and prohibit absence claims.
3. Derive a small generic task-role contract before synthesis. A result cannot
   be sufficient while a key role is missing.
4. Run at most two bounded exact-reference completion expansions. Reserve
   anchor capacity for missing key roles rather than adjacent helpers.
5. Treat verification as a bounded retrieval frontier with exact tests,
   fixtures, examples, and repository-owned documented effects ordered by
   authority.
6. Save typed decisive relations with exact evidence and explicit
   non-guarantees. Syntax is not silently upgraded to runtime causality.
7. Add a deterministic zero-call path only when all local sufficiency gates
   pass. Otherwise permit at most one compact synthesis call and no retry.
8. Save retrieval traces and the v0.1 contract projections as replayable
   artifacts. Keep the four Task Lens artifacts unchanged as the primary
   quartet.
9. Evaluate the six former holdout episodes only as a known development set,
   plus the predeclared configuration cheap-exit case. Do not claim fresh
   generalization or product integration.
10. Keep the existing opt-in `repomap investigate` surface and report
    projection. Do not change default `repomap` behavior.

## Explicit non-goals

- No global call graph, pointer analysis, repository-wide SSA, embeddings,
  issue tracker, runtime-surface discovery, or autonomous patching.
- No Fuego-specific production names, paths, aliases, or prompt examples.
- No target-repository command or test execution.
- No broad cross-repository benchmark and no product-success claim from the
  known regression set.
- No semantic retry, budget inflation across every stage, or weakened claim
  validation.

## Completion condition

The iteration ends with a development-only supervisor decision and exactly one
recommended next experiment. Passing targets authorize only a new untouched
holdout; they never authorize product integration.
