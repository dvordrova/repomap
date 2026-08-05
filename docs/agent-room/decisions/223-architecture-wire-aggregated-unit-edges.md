# Decision 223: Architecture wire — aggregated unit edges replace raw package-import relations

## Status

ACTIVE (Phase 1 of the overnight program
`hermes-repomap-overnight-goal-v3.txt`; approved by the repository owner's
standing goal authorization 2026-08-05). Completes the Decision 216 wire
promise; provider-free acceptance only. No live provider call is authorized
by this decision.

## Problem proven by Archive 6

Decision 216 (bounded local Architecture units) explicitly promised:
"removes 613 raw package-import edges from the wire in favor of aggregated
unit edges" (216:123-124). Archive 6 proves the promise was not kept:

| run | request bytes | supporting_relations | package_import | share of request |
|---|---|---|---|---|
| etcd | 141,282 | 636 | 613 | 59% |
| restic | 54,291 | 261 | 259 | 64% |
| casdoor | 25,752 | 92 | 90 | 47% |
| telebot | 2,345 | 3 | 3 | 17% |

The model groups `u*` unit refs when a units catalog is present (prompt:
"group unit_refs (u*): copy each unit ref exactly as supplied and never
split one unit across components"). Raw `p*`-level package-import edges are
read-only context the model cannot act on — the response contract accepts
only unit_refs/anchor_refs. Meanwhile `SynthesisUnit.RelationOutCount` is
hardcoded to `0` (units.go:351), so the promised aggregate does not exist.

## Scope (exact changes, one vertical slice)

1. **Drop `package_import` from `supporting_relations` when the request
   carries a unit catalog.** `BuildSynthesisRequest` (synthesis.go:566-577)
   filters `bundle.Relations`: when `len(request.Units) > 0`, only
   `behavior_handoff` relations are serialized. `package_import` edges are
   represented by the unit aggregate instead. When no units catalog exists
   (defensive legacy path), relations serialize unchanged.
2. **Fill `RelationOutCount` deterministically.** `CompileUnitCatalog`
   computes, per unit, the count of outgoing `package_import` relations from
   members of the unit to members outside the unit (exact LocalRelation set,
   stable ordering, no provider input). Wire value replaces the hardcoded 0.
3. **Version gates.** `SynthesisRequestVersion` 11→12 (wire changes),
   `SynthesisPromptVersion` v14→v15 (request JSON shape changes). Old cache
   identities miss closed — no compatibility reader, no migration.
4. No change to: response/proposal contract, validation fail-closed rules,
   expansion, report projection, manifest, localization, Study pipeline.

## Why Candidate A over B/C

- A implements what D216 already promised; the current wire is the deviation.
- B (unit-pair relation summary) is a new wire shape with new validation and
  no demonstrated consumer; D216's `relation_out_count` is the agreed aggregate.
- C (no change) contradicts the accepted D216 contract; the archive shows the
  bytes are measurable waste (59-64% of the request).
- Fresh-context reviewer (deleg_6550c365) verdict: A, low-risk, sound.

## Authority and privacy

- Relations remain backend-owned exact evidence; the filter is deterministic.
- No raw member IDs, canonical IDs, paths, or source enter the wire beyond
  the existing request-local refs.
- Model cannot invent relations: response validation is unchanged.

## Provider-free acceptance

1. `go test ./internal/componentmap -count=1` green (updated + new tests).
2. Request builder tests: package_import absent when units present;
   behavior_handoff retained; no-units legacy path unchanged;
   RelationOutCount deterministic and correct per fixture.
3. Request byte regression: fixture mirror of Archive 6 etcd request drops
   ≥40% (measured 47% on the real saved request).
4. Saved accepted responses (telebot/restic replay fixtures) still replay
   under the new request identity via `RecordSynthesisResponse`/replay seams
   — response contract unchanged, old cache identities miss closed as tested.
5. Rejected runs (etcd/casdoor) remain honestly rejected: the local validator
   still rejects a proposal missing component refs / duplicate member sets —
   no weakening of fail-closed validation.
6. Full gates: gofmt, `go vet ./...`, `go test -count=1 ./...`, `make build`
   → `.bin/repomap`, `node --check` on untouched assets, golden unchanged.

## Non-goals

- No new semantic stage, no provider call, no prompt tuning.
- No change to response/proposal contracts or validation.
- No UI change; no report/manifest/localization change.
- No unit-pair edge list (Candidate B) — out of scope for this decision.
