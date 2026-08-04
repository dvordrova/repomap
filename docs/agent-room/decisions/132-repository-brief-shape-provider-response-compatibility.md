# Decision: Repository Brief Shape Provider-Response Compatibility

Status: Completed implementation. Approved and reviewed by the product
supervisor and repository owner in the current session.

## Baseline and attributable failure

The implementation baseline is commit
`45afbe0`. The six pre-existing untracked Caddy semantic-map experiment files
remain outside this decision and must not be touched.

The saved Chatto run
`20260728-234147-chatto-3da86b716518` retains the exact rejected
`repository_brief_shape` provider response. The call took 44,830 milliseconds,
used 4,916 output tokens, and returned an otherwise structurally coherent
Brief and Shape proposal.

The prompt required `brief.domain_terms`. The response placed exactly
`domain_terms` at the top level while retaining the required version,
repository type, five Brief statements, Shape area IDs, term fields, and
support IDs. Strict decoding rejected the whole 2,502-byte response with
`json: unknown field "domain_terms"`, so no Study Map survived.

This is an attributable placement-only compatibility failure. A fresh
repository or provider run is not required to reproduce or correct it.

## Corrective contract

The provider-response decoder may accept exactly one alternate field
placement:

- canonical: `brief.domain_terms`; or
- compatibility input: top-level `domain_terms` when the nested key is absent.

For the compatibility input, the complete top-level field is moved into the
Brief before existing structural validation. The returned
`BriefShapeProposal` remains canonical, so any subsequently saved JSON places
domain terms only beneath `brief`.

The decoder must reject:

- simultaneous top-level and nested `domain_terms` keys, including empty
  arrays;
- a non-array or null compatibility value;
- every other unknown top-level, Brief, statement, or term field;
- input outside the existing byte budget;
- malformed or oversized terms;
- invalid or missing support IDs; and
- every existing Brief, Shape, version, and repository-type validation
  failure.

Direction, review, canonical record, report, HTTP, and public JSON contracts
remain unchanged.

## Authorized file budget

Decision activation and implementation are limited to:

- `docs/agent-room/decisions/132-repository-brief-shape-provider-response-compatibility.md`
- `docs/agent-room/CURRENT.md`
- `internal/studymap/editing.go`
- `internal/studymap/editing_test.go`

No command call-site change is required: `DecodeBriefShapeProposal` is the
existing provider-response boundary used by live preparation and accepted
attempt replay.

## Acceptance evidence

Provider-free tests must prove:

1. the exact retained Chatto response shape with top-level `domain_terms`
   decodes successfully;
2. its normalized proposal equals the prompt-conformant nested-form control;
3. marshaling the result emits only canonical nested `brief.domain_terms`;
4. both-key presence fails closed;
5. every other unknown field still fails closed;
6. null, malformed, oversized, and invalid-support compatibility values fail
   closed; and
7. existing prompt-conformant small fixtures remain unchanged.

Required checks:

```sh
gofmt -w internal/studymap/editing.go internal/studymap/editing_test.go
go test ./internal/studymap -count=1
./scripts/check.sh
git diff --check
```

## Non-goals and stop conditions

- No repository or provider run.
- No Search, UI overflow, routing, discovery gate, prompt, direction, review,
  Architecture, adapter, source-authority, report-format, or HTTP change.
- No generalized JSON repair, permissive map decoding, alias registry, or
  schema framework.
- Stop on a fifth path, any unknown-field fail-open, canonical output drift,
  overlap with unrelated dirty work, or failed required check.

## Completion condition

This decision completes only after the exact saved-response replay, strict
negative tests, required checks, and concrete product-supervisor review. It
authorizes no subsequent provider/repository run or unrelated slice.

## Completed result

The provider-response decoder now distinguishes nested and top-level
`domain_terms` presence with raw JSON fields. It accepts the compatibility
location only when the canonical nested key is absent, rejects both-key
presence including empty arrays, requires the selected value to be an array,
and applies the existing bounded strict decoder and structural validation.

The exact retained Chatto payload decodes five ordered terms with `snapshot`
first. The normalized proposal equals its nested-form control, and marshaling
emits only canonical `brief.domain_terms`. Tests fail closed on both populated,
both empty, null, object, unknown term fields, invalid support IDs, trailing
JSON, every other unknown field, and input outside the existing byte budget.

`go test ./internal/studymap -count=1`, the full `./scripts/check.sh`, and
`git diff --check` passed. The product supervisor reviewed the exact patch and
approved the checkpoint. No repository or provider run was performed.
