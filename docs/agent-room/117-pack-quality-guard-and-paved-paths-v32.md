# Decision: Repomap v3.2 Pack Quality Guard and Paved Paths

Status: Approved by the repository owner in the current implementation
session.

## Product outcome

Extend the existing Repository Brief and Study Map into a source-backed
reading and operating guide. A new contributor should be able to understand
the repository, choose a small number of useful code-reading questions, and
find exact repository-owned ways to set up, run, observe, or verify it without
repomap inventing commands.

## Approved implementation scope

1. Split the v3.1 monolithic Study Map model stage into independently saved
   Repository Brief + Shape, direction-candidate, and bounded Reading Pack
   review stages.
2. Review every selected reading anchor against its exact bounded source
   window. The review may classify semantic fit, choose a closed presentation
   role, identify a closed overclaim reason, and supply narrower display copy.
   It may reference only supplied opaque IDs and cannot create repository
   facts, paths, symbols, relationships, or runtime order.
3. Validate reviews locally, isolate malformed reviews to their direction,
   remove irrelevant anchors, narrow overbroad presentation copy, and reject a
   direction when fewer than three useful anchors or no production/operational
   anchor remains.
4. Compress the final Study Map to three through six strong, semantically
   distinct directions. Prefer purpose coverage, role-diverse source packs,
   central production areas, concrete navigation, and low cognitive load.
   Reserve one slot for a locally accepted first-contact direction when one
   exists. The repository owner explicitly approved relaxing selection limits
   when measured caps suppress a useful result; three independently reviewed
   chi directions must not be discarded solely by a minimum-count gate.
5. Add a separate bounded operational evidence collector for repository-owned
   documentation, build targets, package scripts, executable scripts,
   configuration, environment declarations, local endpoints, logs, examples,
   and verification commands. Do not reuse the Study Map source selector as
   its operational ranking contract.
6. Build presentation-only Paved Paths from exact operational evidence. Every
   command, target, endpoint, and reference must resolve locally; sensitive
   values are rejected or redacted; ordering basis is explicit; incomplete
   evidence produces exact landmarks rather than a fabricated workflow.
7. Extend the existing report data model, renderer, Search, source drawer, and
   hash routing with an optional Operate workspace. Empty or rejected
   operational output leaves the existing report unchanged.
8. Save each model attempt and local reduction independently so one malformed
   Reading Pack review or Paved Path response cannot destroy the Repository
   Brief, other directions, or the old report.
9. Reuse the five frozen v3.1 repositories and saved artifacts where the new
   work is a local replay. Make fresh provider calls only for the new bounded
   semantic-fit or operational editorial work and record exact provenance.

## Truth boundary

Reading Pack reviews and Paved Paths are presentation/planning artifacts, not
canonical Mechanisms or universal KnowledgeObjects. Documentation expresses
intended procedure; scripts and configuration express executable structure;
neither proves that a command succeeds. The analyzer must not execute target
repository commands or tests. A model can select supplied opaque evidence and
write bounded editorial copy, but local code remains authority for identity,
source, command shape, target existence, safety, and publication eligibility.

## Explicit non-goals

- No change to canonical Mechanism validators or hashes.
- No runtime-surface discovery, SSA, points-to analysis, global call graph, or
  repository-wide flow enumeration.
- No arbitrary target command execution, test execution, semantic retry, or
  repository-specific production rule.
- No universal workflow schema, MCP implementation, agent runtime, second
  frontend, renderer, or model provider.
- No forcing every repository to publish the same operational categories.
- No weakening validation or increasing all token budgets to obtain a pass.

## Focused verification

- changed-package tests for split Study Map contracts, per-anchor review,
  compression, operational evidence collection, Paved Path validation, saved
  replay, and report/Search/navigation projection;
- JavaScript syntax and report smoke checks;
- five frozen repository evaluations using replayed inputs where possible;
- `git diff --check`.

Slow runtime-surface analysis, target-repository tests, broad release checks,
and a new full provider sweep are not required for this MVP iteration.
