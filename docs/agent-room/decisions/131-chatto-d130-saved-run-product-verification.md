# Decision: Chatto D130 Saved-Run Product Verification

Status: Completed verification. Approved and reviewed by the product
supervisor and repository owner in the current session.

## Baseline and purpose

The verification baseline is commit
`45bd588c31ff15d421529aa7c6d84c9ac6add3d0`. The stable PATH binary must have
SHA-256
`64d7f8312a68ce3fe20a097cf8c55d30f1ee89ad15588183fc10adfb5812792a`.

Decision 131 verifies the completed Decision 130 projection and normal-report
UI against the saved Chatto run
`20260728-234147-chatto-3da86b716518`. It does not validate fresh discovery or
provider behavior and does not authorize a new repository analysis.

The original saved run predates Decision 130, so its existing `report.html`
necessarily contains the old renderer. Serving that original file is not a
valid check of the Decision 130 result. A local renderer replay over a complete
copy of its saved evidence is valid because it exercises the current
`report.ReadRunDir`, topic projection, routing, and renderer without repeating
analysis or contacting a provider.

## Authorized mutation boundary

The original run is read-only:

`/Users/dvordrova/Library/Caches/repomap/runs/20260728-234147-chatto-3da86b716518`

Copy the complete run to the confirmed-ignored directory:

`tmp/chatto-d130-replay/20260728-234147-chatto-3da86b716518`

Only that copy may receive report JSON, HTML, feedback-template, manifest,
serving, or screenshot-related writes. The original run, Chatto checkout,
provider, repository artifacts outside the ignored copy, and caches outside
the copy remain untouched.

Render the copied run with the stable checkpoint binary:

```sh
repomap dev render-report \
  tmp/chatto-d130-replay/20260728-234147-chatto-3da86b716518
```

The copied report may then be served locally for browser verification. No
repository analyzer or provider command is permitted.

## Product pass criteria

The generated `report.json` must contain:

1. exactly three `user_topics`, in saved presence, message, then upload order;
2. each saved question and exact path, symbol, and line unchanged;
3. nonempty reason-appropriate uncertainty;
4. no answer, steps, observable-effect claim, or runtime-order claim; and
5. zero `user_mechanisms`.

The rendered normal report must:

1. show all three incomplete topics on Overview;
2. expose no Search tab, button, fallback, keyboard/modal entry, or direct
   Search destination;
3. canonicalize `#/search` to Overview; and
4. open one topic detail preserving its exact starting symbols within two
   clicks.

Capture one Overview screenshot and one topic-detail screenshot for the
product supervisor. Record concrete JSON counts, questions, uncertainty, and
starting-symbol values rather than giving a generic pass.

## Authorized file budget

Decision activation is limited to:

- `docs/agent-room/decisions/131-chatto-d130-saved-run-product-verification.md`
- `docs/agent-room/CURRENT.md`

The verification itself may create only ignored files beneath
`tmp/chatto-d130-replay/`. It authorizes no production or test source change.

## Stop conditions

Stop without changing source code if:

- the stable binary hash differs;
- the original run would need to be mutated;
- any repository analysis or provider call would be required;
- a topic is missing, reordered, or gains unsupported answer semantics;
- any normal-report Search entry remains;
- routing does not canonicalize `#/search` to Overview;
- a write occurs outside the copied ignored run; or
- the two documentation paths overlap unrelated dirty work.

Any product defect found by this replay requires a separate reviewed decision
before implementation.

## Completion condition

This decision completes only after the copied replay, JSON inspection, browser
inspection, screenshot capture, and concrete supervisor review. It authorizes
no subsequent repository/provider run or implementation slice.

## Completed verification result

The checkpoint binary hash matched. Re-rendering the complete ignored copy
took 5 seconds and made no repository-analysis or provider call. The generated
report contained zero `user_mechanisms` and exactly three `user_topics` in
saved presence, message, then upload order. Questions, paths, symbols, and
lines remained exact; uncertainty was nonempty and reason-appropriate; no
answer, steps, effect, or runtime-order fields were introduced.

The Overview screenshot showed all three incomplete topics with no Search
surface. The presence topic detail opened in one click and preserved
`cli/internal/connectapi/realtime_projection.go`,
`API.BuildRealtimeProjectionPresences`, and line 85. A fresh direct
`#/search` load canonicalized to `#/overview` and rendered Overview.

Two observations are recorded outside this decision's completion boundary:

- forcing `#/search` as a same-document hash change from an already-open topic
  detail canonicalizes the URL but leaves the detail body until a fresh load;
  no normal-report UI can trigger this stale state; and
- at the in-app browser's normal 1280-pixel viewport, the existing layout
  overflows horizontally and clips primary content.

Plain `Generate` removed the copied stale authority manifest, and
`repomap serve` correctly failed closed with HTTP 409. Browser inspection used
a view-only static server and made no source-authority or source-open claim.
The original saved run remained unchanged.

The product supervisor accepted the projection, fresh-direct Search
canonicalization, view-only authority boundary, and Decision 131 completion.
The same-document stale route and 1280-pixel overflow remain separately
attributable product debt.
