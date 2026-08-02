# Decision 197: Production repository cleanup

## Status

Implemented after the repository owner's tracked-path and caller inventory.
This is a cleanup decision, not a new analysis or presentation layer. Decision
196's accepted Atlas workspace stays installed; Navigator production wiring
remains held for its explicit product-transition decision.

## Problem

The repository contains experiments, experiment-named live product code, and
standalone playground commands whose current authority is unclear. Keeping
them makes the supported product path hard to find and allows stale harnesses
to look like production contracts.

## Decision

Classify each tracked experiment/playground by actual production caller,
maintained check caller, and unique contract. Delete only paths with no such
authority; graduate live contracts out of experiment/playground names; move
acceptance fixtures to stable `testdata`. Do not cosmetically rename a dead
experiment and do not preserve an uncalled tool merely because it might be
useful later.

### Approved outcomes

1. `internal/experiment/surfacediscovery` is live production discovery and
   graduates as a whole to `internal/surfacediscovery`, with all imports
   rewritten. Its exact trigger/coverage/grounding tests remain.
2. `experiments/causal-pipeline` is deleted: it has no product caller and its
   old composition contract is superseded by the report projection.
3. The two product-used `source-episode` `episode.json` fixtures graduate to
   stable product testdata. Composer and auxiliary episode artifacts with no
   caller are deleted only after their necessary pinning/schema assertions are
   retained by product tests.
4. `experiments/semantic-map` may be removed only after its one used
   `beets.python-selection.json` acceptance fixture is moved to product
   testdata and its bridge test is repointed. The fate of the now-orphaned
   internal Pyright adapter is a separate explicit follow-up; this decision
   does not claim it as production merely by retaining it temporarily.
5. Delete standalone component-study/probe/teach/researchtrail, flowproof,
   gopls, pyright, and surface-discovery playground commands where the
   inventory found no maintained product contract. Move a unique assertion to
   a package test before deletion. `investigation` and `symbol` are different:
   their maintained scripts require a stable replacement before their
   playground commands disappear.

Shared Makefile, scripts, AGENTS script list, and current documentation change
once after package/file moves are reconciled. Historical decisions and saved
evaluations are preserved; they are not rewritten to pretend the old paths
never existed.

## Work boundaries

Parallel lanes own only disjoint file trees:

- source-episode/causal cleanup under `experiments/**` plus direct product
  tests;
- surfacediscovery graduation and direct import rewrites;
- deletion of independently unauthorised `cmd/*playground` trees.

The root integrates shared Makefile/scripts/docs/CURRENT changes serially.
No provider, report schema/manifest, Atlas, Navigator, cache, locale, feature
flag, legacy reader, migration, or live-provider change is in scope.

## Completion

- The live Go surface-discovery package was graduated to
  `internal/surfacediscovery` without changing its contents.
- The two source-episode acceptance fixtures moved to product `testdata` with
  SHA-256 and schema/approval coverage; uncalled causal/composer auxiliaries
  were removed.
- The eight inventoried standalone playground commands and their obsolete
  Make/script/current-document entry points were removed. Historical records
  remain untouched.

## Proof

- `rg`/Go import inventory proves no residual old paths;
- every retained fixture has a direct product test caller;
- moved live production package retains focused coverage/grounding tests;
- no stale Make/script/doc entry points remain after root integration; and
- full `./scripts/check.sh` and nearby etcd check pass provider-free.
