# Approved implementation order

The increments run in dependency order without pausing for normal failing
tests, implementation difficulty or incomplete Python features. Each increment
gets focused tests, implementation, Tier 1/Tier 2 verification, diff review and
a coherent commit before the next starts.

## Increment 0 — integration decision record

- commit both audit artifact directories and these integration decisions;
- do not include archive copies or unrelated workspace changes;
- record the pre-existing dirty-tree and test baseline.

## Increment 1 — honest restic semantics

Commit intent:

`fix: correct restic flow semantics with focused regression tests`

Scope:

- add the eight tests in `TEST_MATRIX.md` alongside their fixes;
- stop handler collection at callback boundaries;
- retain `global.CreateRepository`;
- preserve dispatch callsite and target declaration separately;
- represent the concrete scanner task, callback body, cancellation target and
  join target without a fabricated linear call chain;
- add `not_applicable` and remove resolution-only slot promotion;
- keep unresolved external I/O and process completion honest.

## Increment 2 — minimal shared applicability/evidence core

Commit intent:

`refactor: establish shared applicability and evidence rules with Go/Python fixtures`

Scope:

- close semantic enums used at adapter boundaries;
- move final applicability/satisfaction decisions out of Go adapters;
- preserve language-specific mechanism in provenance rather than in shared
  verdict logic;
- migrate versions and deterministic fixtures required by the contract change;
- prove the boundary with one Go and one Python fact fixture, without claiming
  Python production FlowProof parity.

Tier 3 verification runs after this shared contract change.

## Increment 3 — reconcile proven state into the report

Commit intent:

`fix: reconcile proven facts into report state`

Scope:

- require component endpoint witnesses before promoting package imports;
- remove locally resolved files from overview unknowns;
- ensure renderer projections do not reintroduce the audited linear lifecycle;
- fix only reportserver regressions that block these deterministic projections.

## Increment 4 — preserve Python uncertainty

Commit intent:

`fix: preserve Python uncertainty through shared contracts`

Scope:

- structural `dynamic_unknown` for the accepted fixture;
- scenario identity derived from relevant bounded project inputs;
- lossless shared DTO round trip for language, resolution, invocation,
  warnings and scenario;
- explicit experimental status and synchronous concurrency
  `not_applicable`.

No framework registry, Python onboarding parity or generic async abstraction is
approved.

## Increment 5 — documentation and final verification

Commit intent:

`docs: reconcile claims after final verification`

Scope:

- update `CORE_IDEA.md`, `MILESTONES.md`, `NEXT_SESSION.md`, `SYSTEM_MAP.md`,
  `TECHNICAL_DEBT.md` and `PYTHON.md` to match verified behavior;
- record resolved and deferred audit findings;
- run final Tier 3 offline verification and write
  `integration/FINAL_INTEGRATION_REPORT.md`;
- do not implement deferred findings opportunistically.

## Stop conditions

Stop only if the audits materially contradict, a required contract cannot
preserve or explicitly migrate replay artifacts, a forbidden large abstraction
becomes unavoidable, a regression falsifies the root-cause classification, or
unrelated user changes cannot be safely separated for the next commit.

Completion means these approved increments, not every proposal in either
audit.
