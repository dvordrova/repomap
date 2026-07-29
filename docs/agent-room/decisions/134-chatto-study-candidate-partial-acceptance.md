# Decision: Chatto Study Candidate Partial Acceptance

Status: Completed implementation. Approved and reviewed by the product
supervisor and repository owner in the current session.

## Baseline and attributable failure

The implementation baseline is commit
`dd460eb323680ea5c2d6d9f6232fdf4226df8d3b`. The six pre-existing untracked
Caddy semantic-map experiment files remain outside this decision and must not
be touched.

Decision 133 ran the verified stable binary exactly once against clean Chatto
revision `e81f585f147eaeafaf8a3b226e28c2599c3bdb2c`. The saved run is:

`/Users/dvordrova/Library/Caches/repomap/runs/20260729-030442-chatto-d18b2df079a9`

The current product fallback behaved coherently:

- Overview exposed three grounded incomplete topics;
- Search was absent;
- one topic opened in one click;
- its authorized exact source opened in the second click; and
- Architecture remained available.

The richer Study map was lost at the provider-response boundary. The accepted
Brief stage was followed by a 113,220-millisecond Study candidate call that
returned 11 ordered candidate directions. Nine candidates satisfied the
existing three-to-five unique-anchor contract. Candidate positions 9 and 11
contained only two anchors each. `DecodeDirectionProposal` rejected the whole
response at the first invalid candidate, so the nine valid siblings were never
reviewed or published. The resulting error used the still-unassigned local
direction ID and therefore reported the unhelpful label `direction ""`.

This is an attributable all-or-nothing collection-validation defect. The
prompt, provider transport, Brief decoder, anchor authority, renderer, and
normal topic fallback are not causal.

## Corrective contract

The provider-facing Study candidate decoder must keep the response envelope
bounded and strict while validating candidates independently:

1. Decode the top-level version and candidate collection with the existing
   total byte budget, strict unknown-field rejection, and existing maximum raw
   candidate count.
2. Strictly decode and validate each count-bounded candidate with the existing
   field, scalar, opaque-ID, three-to-five-anchor, reading-anchor, and
   repository-object rules.
3. Reject an invalid candidate locally with a bounded diagnostic that records
   its original zero-based position and a stable reason. Do not copy arbitrary
   provider prose into the diagnostic.
4. Preserve every valid candidate in original provider order, derive its local
   direction ID exactly as today, and write only accepted candidates to the
   canonical normalized proposal.
5. On a derived-ID collision, retain the first valid candidate and reject the
   later duplicate deterministically.
6. Fail the Study candidate stage only when the envelope is invalid or zero
   valid candidates remain.

The raw provider response remains retained in the attempt artifact. The same
artifact must expose received, accepted, and rejected counts plus bounded
per-item diagnostics. Successful console output may summarize those counts,
but it must not print provider prose.

This decision does not relax any candidate or anchor rule. It changes only the
failure unit from the whole bounded candidate list to one independently
invalid candidate.

## Authorized file budget

Decision activation and implementation are limited to:

- `docs/agent-room/decisions/134-chatto-study-candidate-partial-acceptance.md`
- `docs/agent-room/CURRENT.md`
- `internal/studymap/editing.go`
- `internal/studymap/editing_test.go`
- `cmd/repomap/study_map_v32.go`
- `cmd/repomap/study_map_v32_test.go`
- `internal/report/user_workspace_asset_test.go`

No production report, renderer, navigation, prompt, provider, discovery,
orientation, source-authority, Architecture, or HTTP file is authorized.

## Provider-free acceptance

Tests must prove:

1. the exact retained Chatto `study_direction_candidates_attempt.json`
   response produces nine accepted directions and two position-specific
   rejections;
2. accepted questions, anchors, reading copy, and provider order are unchanged;
3. the two-anchor candidates remain rejected and the three-to-five rule is not
   weakened;
4. an invalid unknown field, model-supplied direction ID, invalid repository
   object ID, invalid scalar, or duplicate derived ID rejects only that item
   when a valid sibling exists;
5. an invalid envelope, excessive raw candidate count, or zero survivors still
   fails closed;
6. canonical saved direction JSON contains only normalized accepted
   candidates and exact locally derived IDs;
7. attempt diagnostics contain bounded positions/reason codes and no provider
   prose; and
8. a report fixture containing the existing three Chatto topics plus retained
   Study directions renders Overview, Study, and Architecture without Search;
   one Study direction reaches an exact source within two clicks, and
   `#/search` canonicalizes to Overview.

No provider or repository analysis is required for acceptance.

Required checks:

```sh
gofmt -w internal/studymap/editing.go internal/studymap/editing_test.go \
  cmd/repomap/study_map_v32.go cmd/repomap/study_map_v32_test.go
go test ./internal/studymap -count=1
go test ./cmd/repomap -run 'StudyMapV32|StudyDirection' -count=1
go test ./internal/report -run 'UserWorkspace|Study|Search' -count=1
./scripts/check.sh
git diff --check
```

## Non-goals and stop conditions

- No provider call, repository run, retry, second repository, or timeout
  change during implementation review.
- No prompt or request-shape change.
- No anchor-cardinality, anchor-authority, review, proof, publication, or
  confidence relaxation.
- No semantic opportunity, guided tour, orientation, paved path,
  reconciliation, renderer, Search, Architecture, schema, framework, or UI
  polish work.
- Stop on unbounded raw collection processing, provider-order drift, accepted
  candidate mutation, canonical artifact drift beyond rejected-item removal,
  zero-survivor acceptance, unrelated dirty-work overlap, or a failed required
  check.

## Completion condition

This decision completes only after the exact saved-response fixture, strict
negative tests, product coexistence fixture, required checks, and concrete
product-supervisor diff review. It authorizes no subsequent provider or
repository run.

## Completed result

The provider-facing decoder now streams at most `MaxCandidates` raw items with
fixed bounded capacity, strict envelope failure, and independent strict item
decoding. Valid candidates retain provider order and their existing locally
derived IDs. Invalid siblings and later derived-ID duplicates are omitted with
bounded zero-based position and stable-code diagnostics. Zero survivors,
invalid envelopes, and excessive raw counts still fail closed. Whitespace-only
model IDs retain the prior absent-ID semantics; nonblank model IDs remain
rejected.

The retained Chatto-shaped regression accepts nine candidates and records
`invalid_anchor_selection` for positions 8 and 10. Negative tests cover
item-local unknown fields, model IDs, duplicates, zero survivors, and the raw
count bound. Attempt artifacts keep the raw response and add only bounded
diagnostics. The product fixture proves that three topics coexist with
routable Study and Architecture, exact source state, and no Search.

All three focused test commands passed. The full `./scripts/check.sh` passed
all Go tests, `go vet`, and six offline quality replays. `git diff --check`
passed. The product supervisor reviewed the exact five-file implementation,
requested one whitespace-ID compatibility correction, then reviewed the
corrected files and returned `VERDICT: COMMIT D134` with no blocker. No
provider or repository run was performed during implementation.
