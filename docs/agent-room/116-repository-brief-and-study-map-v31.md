# Decision: Repomap v3.1 Repository Brief and Study Map

Status: Approved by the repository owner in the current implementation
session.

## Product outcome

Replace the default onboarding gate of “one automatically proven primary
Mechanism or nothing” with a repository-level Brief, compact Repository Shape,
ordered Study Map, and optional canonical Deep Dives.

The primary user should be able to open an unfamiliar repository, understand
what it is for, form a useful mental model, and choose concrete code-backed
directions for studying it even when no end-to-end Mechanism is publishable.

## Approved implementation scope

1. Build a Repository Brief from existing bounded repository facts, README and
   current documentation, architecture areas, and exact local code anchors.
2. Present a compact Repository Shape of three to seven meaningful production
   areas with direct navigation.
3. Generate at most twelve editorial Study Direction candidates from the same
   bounded facts. A candidate may propose meaning and learning order, but every
   referenced repository object must be an existing opaque ID.
4. Validate candidates locally. Require a natural question, a standalone
   learning outcome, relation to repository purpose or a major area, and at
   least one strong production code anchor. Prefer two to five anchors.
5. Select four to seven directions for purpose coverage, diversity, usefulness,
   concrete navigability, progressive learning order, and low cognitive load.
   Reading order is editorial and must not imply runtime execution order.
6. Classify the repository as service/application, library/framework,
   CLI/tool, monorepo, or mixed only to guide coverage; the user does not choose
   a mode and not every suggested category is mandatory.
7. Attach an accepted canonical Mechanism when one already exists. Otherwise
   open a code-first Reading Path with three to five ordered exact anchors,
   bounded source excerpts, and links to Search and Architecture.
8. Extend the existing report data model, renderer, source presentation, hash
   routing, and Search. Do not create a second frontend or renderer.
9. Keep candidate rejection, confidence, evidence gaps, verdicts, model
   metadata, and internal IDs out of the default User View.
10. Preserve role-aware context selection and bounded effect resolution as
    useful deterministic foundations. They are no longer the product gate for
    publishing onboarding guidance.

## Truth boundary

A Study Direction is an evidence-backed editorial recommendation, not a
runtime claim and not a canonical Mechanism. Documentation may support intended
purpose, but does not prove runtime behavior. The model cannot create files,
symbols, components, mechanisms, evidence, or execution order. All visible code
anchors must exist locally, be authorized, and pass local role-aware
validation.

## Evaluation

After implementation, freeze the pipeline and run the same version without
prompt, ranking, or alias changes on five repositories: a library/framework, a
CLI/tool, a service/application, a multi-component repository, and an
additional unfamiliar Go repository. Perform a bounded blind review for each
and retain individual score dimensions rather than one average.

The committed Caddy, chi, and Litestream fixtures remain product regressions:
existing accepted Mechanisms should attach to appropriate directions where the
canonical IDs are available, while the broader Study Map remains useful beyond
those mechanisms.

## Explicit non-goals

- No global SSA, global call graph, pointer analysis, runtime-surface discovery,
  repository-wide flow enumeration, or repository-specific production rule.
- No requirement for a full input/core/effect proof, canonical verdict,
  sequence evidence, accepted Mechanism, or test evidence before publishing a
  useful Study Direction.
- No exposure of all generated candidates or internal diagnostics in User View.
- No second renderer, frontend framework, or parallel report application.
- No prompt, ranking, or alias patching between the five evaluation runs.
- No implementation of the recommended next investment step during this task.

## Focused verification

- changed-package tests for candidate validation, selection, save/replay, and
  report projection;
- fixed Caddy, chi, Litestream, and zero-Mechanism regressions;
- browser and JavaScript smoke for Study Map navigation;
- a frozen five-repository sweep and exact provenance report;
- final diff audit.

Slow runtime-surface analysis and broad release checks are not required for
this MVP iteration.
