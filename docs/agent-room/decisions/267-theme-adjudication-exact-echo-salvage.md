# 267 — Theme Adjudication exact redundant-echo salvage

**Status:** APPROVED IMPLEMENTATION CORRECTIVE (owner-authorized, 2026-08-10)

**Preserves:** Decisions 213, 224, 232, and 241; compact bounded Theme facts;
strict item-local rejection; backend authority over candidate identity and
refs; no semantic retry or extra provider call.

## Observed defect

The fresh repomap Adjudication response returned six themes. `t1` used the
documented response grammar. `t2` through `t6` additionally copied the
backend-owned input fields `theme_kind`, `anchor_refs`, and
`expansion_file_refs`. Their values exactly matched the advertised candidate,
but strict unknown-field decoding rejected all five items before normal local
validation. Exact inspection established that `t2`, `t3`, `t4`, and `t6` were
otherwise valid.

`t5` is different: its refs pass structural checks, but its observation for
`a7` describes evidence belonging elsewhere. Removing redundant fields does
not prove observation-to-anchor semantic binding and this decision does not
claim that `t5` is repaired.

## Exact corrective

Before strict per-item decoding, the backend may inspect `candidate_ref`. Only
when it resolves to a known request-local candidate may it remove these three
input-only fields:

- `theme_kind`;
- `anchor_refs`;
- `expansion_file_refs`.

Each present value must be typed-exact and order-exact to the value actually
advertised for that candidate. An empty `expansion_file_refs` value is not an
echo when `omitempty` kept that field out of the request. Every exact removal
is counted under a closed normalization key for an accepted theme.

A mismatch rejects the item under a field-specific closed issue code that
contains no provider value. Unknown candidates are not normalized. Every
other unknown response field remains in place and therefore reaches the
existing strict `decode_candidate` rejection. No prose, ref, order, support,
or acceptance is repaired.

The prompt repeats the response allowlist after the request bundle, explicitly
forbids input-field echoes, requires each observation to describe its own
anchor evidence, and forbids an `unknowns` entry already answered by supplied
evidence or retained readings.

Adjudication request identity advances 3→4, result/status identity 5→6, the
content-derived prompt identity advances to
`theme-adjudication-prompt-f60d8a346d32`, and accepted-cache contract advances
v2→v3. Existing artifacts miss closed.

## Acceptance

1. A provider-free regression shaped from the exact fresh incident salvages
   `t2`, `t3`, `t4`, and `t6` and counts four removals of each allowed field.
2. Each mismatched allowed field rejects only its item with a closed code and
   no raw-value diagnostic.
3. An unrelated unknown field still rejects after exact echoes; unknown
   candidates and unadvertised empty expansion echoes are not normalized.
4. Prompt tests pin the post-bundle companion and the own-anchor/unknowns
   instructions.
5. Full `internal/themestudy` tests, vet, formatting, and diff checks pass.

No provider call, real run, report, manifest, or UI change belongs to this
corrective.

Approved by:
    Repository owner after the fresh repomap decode audit distinguished four
    structurally valid themes from the separately misbound `t5`, 2026-08-10.
