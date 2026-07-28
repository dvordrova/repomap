# Decision: Mixed Mechanism/Topic Shelf Primary Overview

Status: Active for implementation. Approved by the product supervisor after
review of the provider-free mixed-shelf microexperiment and activated by the
repository owner through the standing delegation to implement supervisor-
approved bounded slices in this session.

## Baseline and preserved decisions

The implementation baseline is commit
`89ae0b3975753d290519ee0abe8f057cc6149dba`. The pre-existing untracked Caddy
semantic-map experiment files are outside this decision and must remain
untouched.

Decision 127 supersedes Decision 126 only where Decision 126 prohibited the
mechanism projection, default Overview, primary navigation, and two-click
mechanism/topic presentation authorized below. Decision 126's operating-path
completeness gate, evidence requirements, final mechanism publication checks,
runnable-path safeguards, diagnostics, and all unrelated scope remain active
and unchanged.

## Product problem

Stable local facts can produce several grounded opportunity candidates while
the complete-mechanism publication gate accepts none. The current product maps
all candidate-level `insufficient_primary_evidence` results to one aggregate
fallback, conflating "not a complete mechanism" with "nothing useful was
discovered."

The provider-free etcd comparison demonstrated a useful distinction:

- an accepted artifact can remain a full source-backed mechanism; and
- a rejected but grounded candidate can remain an honest question with exact
  places to start and an explicit statement of what evidence is missing.

## Product contract

The default Overview workspace is a mixed shelf with two distinct item types:

1. **Full mechanism.** Only an artifact that already passes every existing
   publication gate may show an answer, ordered phases or steps, claimed
   effects, and source-backed explanations. Existing `UserMechanism` data,
   identity, ordering, validation, and source behavior remain unchanged.
2. **Topic · incomplete.** A locally rejected candidate may show only its
   title, question, exact observed starting symbols, and a concise
   plain-language statement of missing evidence. It must never show or imply an
   answer, ordered steps, claimed effect, or proven path.

Topics are projected only when the saved opportunity, selected candidate, and
attempt diagnostics join uniquely by `candidate_id`, and the local eligibility
record retains at least one exact repository-relative path and symbol. A topic
may not invent or recover information from neighbouring files, model prose, or
an unselected candidate.

The primary shelf does not expose internal `input/core/effect` counters or
phrases such as "bounded static analysis." User-facing uncertainty is a
deterministic plain-language rendering of existing local rejection reasons.

## Navigation and presentation contract

- The mixed shelf renders on the default Overview before any Study Map
  fallback can replace it.
- Click 1 opens the existing full-mechanism detail or an incomplete-topic
  detail.
- Click 2 opens an exact mechanism-step source or exact topic starting symbol
  through existing authorized source navigation.
- No button-shaped inert control may ship.
- Architecture remains a secondary overview facade. This decision does not
  redesign its graph, relations, selection, or source behavior.
- Search remains outside primary navigation. This decision does not change its
  data contract, ranking, indexing, or routes.

## Authorized file budget

Decision activation is limited to:

- `docs/agent-room/decisions/127-mixed-mechanism-topic-shelf-primary-overview.md`
- `docs/agent-room/CURRENT.md`

Implementation is limited to:

- `cmd/repomap/fresh_repo_demo.go`
- `cmd/repomap/fresh_repo_demo_test.go`
- `internal/report/report.go`
- `internal/report/html.go`
- `internal/report/report_test.go`

## Acceptance evidence

Fixed provider-free tests use the three etcd candidate artifacts joined
uniquely by `candidate_id` and accepted artifact
`semantic-artifact-003a27952d61f4735635a018`.

They must prove:

- exactly one full mechanism and three incomplete topics;
- the four exact starting symbols `quotaKVServer.Txn`,
  `quotaLeaseServer.LeaseGrant`, `startEtcdOrProxyV2`, and
  `snapshotSender.send`;
- deterministic plain-language uncertainty with no topic answer, steps,
  effect, ordering, or proven-path wording;
- working mechanism-detail, topic-detail, and authorized source targets;
- the mixed shelf precedes Study Map fallback on the default Overview;
- Architecture remains secondary and Search is absent from primary
  navigation; and
- existing accepted mechanisms and publication gates are unchanged.

Required checks:

```sh
go test ./cmd/repomap ./internal/report -count=1
./scripts/check.sh
git diff --check
```

## Explicit non-goals and stop conditions

- No candidate-generation, prompt, provider, replay, adapter, eligibility,
  publication-gate, mechanism-validation, or schema-version change.
- No Python support, Search work, Architecture redesign, Task Lens work,
  provider call, target-repository execution, or broad cleanup.
- No invented topic evidence, partial-mechanism claim, new public universal
  DTO, report-format migration, or dead control.
- Stop before implementation if the two decision files do not activate this
  exact contract.
- Stop on an edit outside the five implementation paths, failure to derive an
  exact source action through existing authority, or any required check
  failure.

## Completion condition

This decision completes only when the bounded projection and default-Overview
rendering pass the required checks and the product supervisor reviews the
implemented result. It authorizes no subsequent repository run or additional
product slice.
