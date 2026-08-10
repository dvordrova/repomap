# 275 — Nested Architecture symbols retain primary-scope context

**Status:** ACTIVE (owner-authorized, 2026-08-10)

**Preserves:** D238's production-unit primary-scope quality gate, D241's
item-local supporting-only salvage, D254's nested package/symbol request shape,
one bounded Architecture call, request-local refs, exact deterministic local
fallback and remainder authority, and strict response validation without
backend-authored conceptual membership.

## Fresh-run failure

A fresh ordinary repomap Architecture response was valid nested JSON with four
subsystems, eighteen components and 102 member-ref occurrences. All occurrences
were nested `s*` refs; none was a package `p*` primary ref. One 35-ref catch-all
was correctly reduced to 24 refs by the existing response ceiling. The
supporting-only production salvage then moved every remaining occurrence to
the deterministic local remainder.

Two contract defects obscured that honest result:

1. the flat local candidate already carried
   `coverage_role:"supporting_evidence"`, but D254's nested symbol wire omitted
   the field, so the provider could not see the role beside the symbol;
2. when salvage removed every component it also removed every subsystem shell,
   causing `Apply` to report the unrelated fatal
   `proposal.invalid_subsystem_count` instead of the existing recoverable
   `proposal.zero_useful_semantic_components` result.

The prompt also said a symbol could be used "instead of" a package ref. That
wording contradicted D238's production-unit quality gate and encouraged the
observed all-symbol response.

## Approved request and prompt contract

- Every package-owned nested symbol serializes the existing backend-owned
  `coverage_role:"supporting_evidence"` field explicitly.
- Nested symbols distinguish implementations inside a package responsibility.
  They never substitute for a defensible `p*` `primary_scope` ref somewhere in
  the same production unit.
- The primary package ref and supporting symbol ref remain independently legal
  `member_refs`. They need not occur in the same component.
- The backend never inserts, promotes or repairs a missing parent package ref.
  Exact returned refs remain the only model-authored membership.

The provider request identity advances 20 → 21. The prompt identity advances
automatically from the exact system-text SHA, so previous request/cache entries
miss closed. Response, proposal, Landscape and synthesis record schemas remain
unchanged; `SynthesisRecordVersion` remains 16.

## Honest zero-useful classification

When supporting-only salvage removes every component, it retains the already
validated subsystem shells only until exact membership diagnostics run. No
member, anchor or parent ref is restored. The existing
`proposal.zero_useful_semantic_components` diagnostic therefore describes the
zero accepted model membership, while the deterministic local landscape and
complete uncovered-member partition remain authoritative.

Ceiling and salvage diagnostics remain additive. The all-supporting result
therefore carries the exact applicable ceiling diagnostic,
`proposal.supporting_only_unit_coverage_salvaged`, and
`proposal.zero_useful_semantic_components`, with no
`proposal.invalid_subsystem_count`.

## Verification

A provider-free generated regression reproduces the fresh response cardinality
without committing provider artifacts: 64 primary packages, 99 nested
supporting symbols, four subsystems, eighteen components, the exact component
ref-count vector and one 35-ref catch-all. It proves:

1. all 99 nested symbols carry explicit supporting role on the provider wire;
2. the nested response parses and the 35-ref component is bounded 35 → 24;
3. no primary or supporting membership is accepted after item-local salvage;
4. every requested member remains in the exact local uncovered partition and
   deterministic fallback landscape; and
5. diagnostics contain ceiling, supporting salvage and zero-useful semantics,
   never invalid subsystem count.

No provider call, report/UI change, parent auto-promotion, retry or new analysis
stage is authorized by this decision.
