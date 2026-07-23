# Decision: Golden Mechanism v0

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product question

Can repomap start from one useful human-selected question, collect a small
amount of missing local evidence, and turn it into one genuinely useful
evidence-backed mechanism instead of a grounded but irrelevant fact?

The experiment uses the saved Caddy run and the human shortlist `File Server
Directory Listing` versus `Caddyfile Syntax Error Reporting`. The exact Caddy
checkout makes directory listing the smaller bounded mechanism, so that is the
golden candidate.

## Approved implementation

1. Record a rubric before implementation. The directory-listing explanation
   is evaluated on entry, item collection, request options, sorting/paging,
   response-format selection, output, material branches, and honest unknowns.
2. Extend the existing Semantic Discovery capability enum with a small closed
   set sufficient for this experiment. One candidate records required,
   available, and missing capabilities plus a resolution. Do not create a
   second capability ontology.
3. Run one local syntax-only targeted probe from 3–6 saved source/evidence
   seeds. It may read enclosing Go functions and expand direct repository-local
   callees for at most 2–3 levels under explicit file, function, byte, and wall
   budgets. It returns partial facts when a budget is reached.
4. The probe emits deterministic semantic facts with local evidence locations.
   It performs no package loading, global source survey, SSA, call graph,
   pointer analysis, runtime-surface discovery, or model call.
5. Use one bounded provider call to edit the original question, rubric, saved
   relevant facts, probe facts, opaque IDs, and known gaps into the existing
   mechanism artifact shape. The provider may not create repository objects or
   evidence.
6. Add local intent-retention validation. The accepted artifact retains the
   original candidate ID and question, declares covered and uncovered answer
   aspects, resolves every support ID, and may be published as a mechanism only
   when the configured key-aspect threshold is met.
7. Save the probe facts and the golden semantic replay separately from the
   existing general Semantic Discovery record. Both use the same semantic
   `Fact`, `Record`, `Artifact`, report, canvas, evidence, and search contracts.
   A missing or rejected golden result leaves the old report and semantic
   artifacts intact.
8. Reuse the current `Explore this repository`, artifact detail, canvas focus,
   evidence inspector, and Super Search. Add no new artifact kind and no new
   renderer.
9. Compare the old and enriched listing artifacts on the pre-recorded 0–4
   rubric. Stop after classifying the first failure if the result scores below
   3; do not add another analyzer or provider experiment.
10. Save ignored findings and a supervisor-facing report. Run only focused
    checks during iteration; do not execute the known long runtime discovery.

## Hard bounds

- one mechanism;
- 3–6 seeds;
- repository-local source only;
- no more than 3 direct expansion levels;
- no more than 4 files, 12 functions, or 48 KiB of source;
- short local timeout and a partial-result stop reason;
- one bounded synthesis call;
- no application response cache changes and no provider configuration changes.

## Acceptance

The experiment succeeds when the enriched artifact scores at least 3/4, has at
least three meaningful supported steps, retains the original intent, contains
no unknown opaque IDs, replays from saved artifacts without repository
analysis, is discoverable through the existing search in English and Russian,
and report generation still succeeds when the model result is absent or
rejected.

The ignored final report must state what the probe added, what remains unknown,
whether the approach generalizes, and exactly one next mechanism to try. It
must not implement that next mechanism.
