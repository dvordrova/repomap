# Decision: Language-Neutral Discovery Attempt Invariant

Status: Active for implementation. Approved by the product supervisor and
activated by the repository owner through the standing delegation in the
current session.

## Baseline and preserved decisions

The implementation baseline is commit
`a88ddf2c0862c4730c5e746cf9bb4b69cdd37c45`. The pre-existing untracked Caddy
semantic-map experiment files remain outside this decision and must not be
touched.

Decision 128 follows Decision 127. It changes only the eligibility boundary
for making one bounded attempt to discover something worth studying. Decision
127 continues to own the mixed mechanism/topic shelf, topic projection, and
navigation contract. All existing complete-Mechanism proof, publication,
provider, report, source-authority, and safety contracts remain unchanged.

## Product problem

The normal artifact-backed pipeline invokes semantic discovery, but
`prepareFreshRepoMechanism` currently stops before the opportunity scan unless
at least three Go functions were extracted. Both source-function collectors
are Go-specific.

That condition confuses two different questions:

1. whether the repository contains at least one exact place from which the
   product can propose an honest topic to investigate; and
2. whether enough language-specific evidence exists to prove and publish a
   complete Mechanism.

As a result, a repository with one or two exact Go anchors and every Python
repository can complete orientation and targeted research without ever asking
what may be worth studying.

## Discovery invariant

In a normal online artifact-backed run, the opportunity scan runs exactly once
whenever at least one validated exact discovery anchor exists.

The only pre-scan stops are:

- explicit `--offline` operation;
- no artifact-backed report (`--no-debug` or preview operation);
- authorized source-catalog failure, which already makes all optional stages
  view-only; or
- zero validated exact discovery anchors, recorded explicitly as
  `no_exact_discovery_anchors`.

Architecture, guided-tour, or Study Map success must not control discovery.
Language and anchor count above zero must not control discovery. The existing
`fewer_than_three_bounded_source_functions` pre-scan condition is removed.

## Exact discovery-anchor authority

An exact discovery anchor is a small language-neutral local record containing:

- a clean catalog-authorized repository-relative path;
- an opaque nonempty language label;
- a nonempty exact declaration symbol;
- a positive declaration line inside the saved source window;
- one bounded deterministic source statement; and
- matching local evidence provenance for the same path and line.

Anchors may be derived only from:

- already validated `sourcewindowfacts.Function` facts; or
- already saved, authorized, code-bearing local source windows and their exact
  local provenance.

Saved-window anchors preserve saved order and deduplicate exact
`path/language/symbol/line` tuples only. They do not infer calls, effects,
runtime order, package ownership, or relationships.

`ReportData`, `UserTopic`, rendered HTML, model prose, titles, questions, and
presentation-only `UserSources` are not anchor authority. The implementation
must not add repository crawling, a new analyzer process, a new per-language
AST or call-graph parser, a language registry, or a new provider-visible
request shape.

## Go compatibility contract

For currently eligible Go inputs, the implementation preserves:

- opportunity-bundle and request bytes;
- saved and central function ordering;
- existing fact and candidate IDs;
- candidate ordering; and
- the complete-Mechanism planner, probe, validation, and publication path.

Only Go repositories with one or two valid anchors newly cross the removed
pre-scan count gate. They enter the unchanged Go proof path after the
opportunity scan.

## Non-Go topic contract

Non-Go anchors feed the same opportunity scan. A candidate grounded to exact
non-Go anchors is persisted as `insufficient_primary_evidence` with the
deterministic reason `proof_adapter_unavailable`.

This is a discovery result, not a failed Go proof attempt:

- the Go complete-Mechanism planner and probe are not invoked;
- no answer, effect, ordered steps, or proven path is created;
- no `UserMechanism` is created; and
- Decision 127 may project the candidate as `Topic · incomplete` with its
  exact path, symbol, and line.

## Authorized file budget

Decision activation is limited to:

- `docs/agent-room/decisions/128-language-neutral-discovery-attempt-invariant.md`
- `docs/agent-room/CURRENT.md`

Implementation is limited to:

- `cmd/repomap/fresh_repo_demo.go`
- `cmd/repomap/fresh_repo_demo_test.go`
- `cmd/repomap/main_test.go`
- `internal/report/report.go`
- `internal/report/report_test.go`

## Acceptance evidence

Provider-free fixed tests must prove:

1. A tiny Go fixture with exactly two declarations invokes the opportunity
   scan once, never records
   `fewer_than_three_bounded_source_functions`, preserves declaration order,
   and enters the unchanged Go proof path.
2. An existing Go fixture with at least three functions retains byte-identical
   opportunity request bytes and ordering.
3. The existing Python fixture invokes the opportunity scan once, persists one
   grounded `Topic · incomplete` with its literal path, symbol, and line,
   records `proof_adapter_unavailable`, creates zero mechanisms, and invokes
   the Go complete-Mechanism planner and probe zero times.
4. Zero exact anchors performs no opportunity call and records
   `no_exact_discovery_anchors`.

Required checks:

```sh
go test ./cmd/repomap ./internal/report -count=1
./scripts/check.sh
git diff --check
```

## Explicit non-goals and stop conditions

- No report or UI object, model output, title, question, or prose becomes
  anchor authority.
- No repository crawl, new provider call, provider request-shape change,
  language registry, AST/call-graph parser, analyzer process, schema change,
  renderer change, Search work, Architecture work, Study Map work, eligibility
  relaxation, publication-gate change, or proof-adapter change.
- No count-, language-, Architecture-, guided-tour-, Study-Map-, or
  stage-success-dependent discovery branch.
- No non-Go candidate may enter the Go planner or probe.
- Stop on any change to established Go request bytes, ordering, identity, or
  proof behavior; any eighth path; or any failed focused, full, or hygiene
  check.

## Completion condition

This decision completes only when the invariant and both Go/Python fixtures
pass the required checks and the product supervisor reviews the implemented
result. It authorizes no subsequent repository run or unrelated product slice.
