# Decision: Chatto Topic Uncertainty and Search Fallback Corrective

Status: Active for implementation. Approved by the product supervisor and
corrected by the repository owner in the current session.

## Baseline and preserved decisions

The implementation baseline is commit
`2b1678466130409db7cc55c1f40ed97271288160`. The pre-existing untracked Caddy
semantic-map experiment files remain outside this decision and must not be
touched.

Decision 130 follows completed Decision 128. It corrects two failures exposed
by the saved normal Chatto run
`20260728-234147-chatto-3da86b716518`:

1. producer-defined incomplete-topic reasons were not understood by the
   Decision 127 report projection; and
2. an empty mixed shelf exposed legacy Search as a normal-report fallback.

Decisions 127 and 128 continue to own the mixed shelf, exact topic authority,
language-neutral discovery attempt, complete-Mechanism gates, and all
unrelated behavior. This decision does not relax any mechanism gate or turn an
incomplete topic into an answer.

## Attributable failure

The Chatto run was produced immediately after the stable PATH binary was
rebuilt from the baseline commit. Its saved artifacts contain the Decision 128
topic projection path and therefore are not attributable to the separate
stale checkout.

The run saved three grounded questions with exact starting symbols:

- how user presences are aggregated for real-time delivery;
- how a new chat message is processed and persisted; and
- how asset uploads are initiated and validated.

All three were retained as `insufficient_primary_evidence`. Their producer
diagnostics included `core_work_fact_missing` and
`unresolved_dynamic_dispatch`, but `userTopicUncertainty` did not recognize
those values. The report projection fail-closed the whole collection, produced
zero topics, and recorded
`topic shelf unavailable: rejected candidate reason is unsupported`.

With both mechanisms and topics empty, the renderer's
`mixedShelfAvailable()` branch exposed a 336-item Search fallback. The
orientation rejection, Study Map `domain_terms` decode failure, and
`CLAUDE.md` symlink warning are independent and outside this corrective.

## Corrective contract

### Supported incomplete-topic reasons

`userTopicUncertainty` additionally maps exactly two producer-defined reasons
to deterministic, honest user copy:

- `core_work_fact_missing`: the exact starting point exists, but exact source
  evidence does not yet establish the core behavior.
- `unresolved_dynamic_dispatch`: exact local evidence exists, but dynamic
  dispatch prevents proving the next target.

The existing mappings remain unchanged. Any unknown reason continues to
fail closed. Topic projection still requires the saved opportunity,
candidate, attempt, eligibility, and exact symbol records to join uniquely.
No topic may expose an answer, steps, claimed effect, runtime order, or proven
path.

### No normal-report Search surface

Every normal report exposes no Search tab, button, empty-shelf fallback, or
direct Search destination, regardless of mixed-shelf availability or the
`no_search` option. An attempted normal `#/search` navigation resolves to
Overview.

The `semantic_search` report JSON field, payload production, format, and
compatibility behavior remain unchanged. This decision removes only the
normal-report presentation and navigation surface. It does not redesign,
rerank, or delete Search data.

Task-investigation and source-episode contracts remain governed by their
existing decisions and tests. No new Search destination is introduced there.

## Authorized file budget

Decision activation is limited to:

- `docs/agent-room/decisions/130-chatto-topic-uncertainty-reason-corrective.md`
- `docs/agent-room/CURRENT.md`

Implementation is limited to:

- `internal/report/report.go`
- `internal/report/templates/script.js`
- `internal/report/report_test.go`
- `internal/report/user_workspace_asset_test.go`
- `internal/report/testdata/report.golden.html`

## Acceptance evidence

Provider-free fixed tests must prove:

1. A three-candidate Chatto-shaped fixture containing
   `core_work_fact_missing`, `unresolved_dynamic_dispatch`, and both together
   projects three topics in saved order.
2. The projected questions, paths, symbols, and lines remain exact; mechanisms
   remain empty; uncertainty is plain language; and no answer, steps, effect,
   runtime order, or proven path is introduced.
3. An unknown rejection reason still fail-closes the projection.
4. A normal report with a nonempty shelf exposes no Search tab, button,
   fallback, or direct destination.
5. A normal report with an empty shelf and populated `semantic_search`
   exposes no Search tab, button, fallback, or direct destination; attempted
   `#/search` navigation resolves to Overview.
6. `semantic_search` remains serialized unchanged for compatibility.

Required checks:

```sh
node --check internal/report/templates/script.js
go test ./internal/report -count=1
./scripts/check.sh
git diff --check
```

If the authorized report golden changes, regenerate it only with:

```sh
go test ./internal/report -run '^TestWriteReportHTML_Golden$' -update -count=1
```

## Explicit non-goals and stop conditions

- No Beets, Chatto, or other repository/provider run.
- No orientation, discovery, candidate-generation, threshold, eligibility,
  mechanism-gate, Study Map, Architecture, provider, prompt, adapter, schema,
  format-version, or source-authority change.
- No removal or mutation of `semantic_search` data or report JSON format.
- No unknown rejection reason may fail open.
- Stop on any eighth path, overlap with unrelated dirty changes, any remaining
  normal-report Search destination, or any failed required check.

## Completion condition

This decision completes only when the bounded projection and normal-report
Search removal pass all required checks and the product supervisor reviews the
implemented result. It authorizes no subsequent repository run or unrelated
product slice.
