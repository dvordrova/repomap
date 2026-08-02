# Decision 203: Architecture coverage calibration

## Status

Approved by the repository owner as one bounded live calibration after the
fresh Decision 202 Casdoor response covered every package and symbol candidate
but omitted every file candidate.

## Outcome

The Architecture request carries an exact typed `required_member_refs`
checklist equal to the complete ordered candidate-ref set. The prompt states
that `parent_ref` is context only and requires an exact distinct-set self-check
by member kind. The existing validator remains authoritative: missing,
unknown, wrong-kind or duplicate coverage is rejected atomically; no member is
added, repaired or selected locally.

This is deliberately one calibration run, not the durable Architecture
contract. The observed 34/34 package, 8/8 symbol and 0/8 file pattern indicates
that the current request mixes conceptual members with structural containment
locators. If the single calibration omits any file again, or includes files
only as mechanical duplicates of their package/symbol context, prompt tuning
stops. The next contract must use producer-owned roles:

- `conceptual_member` refs are the exact set the provider must group;
- `structural_locator` refs are complete read-only containment/source context
  and cannot occur in model-authored conceptual membership; and
- D177 retains every local candidate, relation, locator and source fact
  independently of accepted model enrichment.

Role assignment must be producer-owned and cannot be a global `kind=file`
rule, because a file may be a real semantic unit in other languages and
repository shapes.

## Contract changes

- synthesis request and record advance to v8;
- prompt advances to `architecture-grounding-v11`;
- proposal and Landscape semantics remain unchanged for this calibration;
- cache identity binds the exact new provider body and contract versions; old
  v7/v10 records miss closed; and
- the saved fresh Casdoor 42/50 response remains a rejected fixture while an
  exact 50/50 response is accepted provider-free.

## Acceptance and stop condition

- `required_member_refs` is byte-stable and exactly equals all candidate refs;
- missing, reordered, duplicate or wrong-kind checklist data fails before a
  provider call;
- the saved 42/50 response is rejected with exact diagnostics and unchanged
  D177 facts/relations;
- a synthetic exact 50/50 response is accepted without repair;
- exactly one installed live Casdoor calibration is allowed; and
- after that run, either the result is a genuinely useful complete grouping or
  work moves directly to conceptual-member/structural-locator separation.

