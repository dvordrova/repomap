# Repomap Information-Preservation and Salvage Review Charter v5

## Product objective

Repomap optimizes for:

> **maximum useful visible truth with minimum unsupported assertion**

This is not the same as either:

- reject the entire semantic result after one malformed/duplicate item; or
- dump every internal record into the default UI.

The required behavior is:

```text
retain all exact local facts
→ salvage every independently valid interpretation
→ localize conflicts and malformed items
→ expose complete counts and diagnostics
→ use progressive disclosure to keep the default view readable
```

## The monotonic product law

Adding one duplicate, malformed sibling, weak observation, or new exact fact
must never make unrelated previously valid information disappear.

Metamorphic form:

```text
publish(valid_set + bad_item)
must contain
publish(valid_set)
minus only claims whose identity/authority directly conflicts with bad_item
```

A model/projection defect may reduce **authority** or **default prominence**. It
must not reduce unrelated **availability**.

## Three independent ledgers

### 1. Fact ledger

Backend-owned exact local facts:

- units;
- entities;
- observations;
- evidence;
- exact source locations;
- structural relations;
- coverage and omissions.

Rules:

- append/preserve;
- stable-ID idempotence;
- no semantic response may delete or rewrite facts;
- every considered fact is published or accounted for under a closed omission.

### 2. Interpretation ledger

Model-authored or deterministic conceptual claims:

- grouping;
- names/descriptions;
- Study themes;
- proposed memberships;
- editorial ordering.

Rules:

- item-local validation;
- accepted / normalized / conflicted / rejected state per item;
- valid siblings survive;
- conflict is retained as conflict, not silently resolved;
- zero valid items yields local-only product, not loss of the fact ledger.

### 3. Projection ledger

User-facing views:

- Overview perimeter;
- Study shelf;
- Architecture list/canvas;
- focused neighborhood;
- mechanism fragments.

Rules:

- projection may aggregate, cluster, collapse, choose representatives, or show
  counts;
- projection must not mutate canonical identity or acceptance;
- every hidden/collapsed item remains reachable and count-reconciled;
- changing diagram type must not change which facts are accepted.

## Error containment and escalation

Errors are handled at the smallest safe scope.

| Scope | Examples | Required outcome |
|---|---|---|
| Field | overlong prose, invalid optional label | normalize/drop field; keep item |
| Item | unknown ref, malformed membership row | reject item; keep siblings |
| Equivalence class | duplicate/colliding items | coalesce or mark conflict; keep unrelated classes |
| Group | one component has ambiguous membership | quarantine that membership/component; keep other components |
| Semantic stage | zero independently valid semantic items | publish local-only product |
| Run | stale authority, unsafe source, corrupt canonical artifact | terminal failure |

Do not escalate to a wider scope merely because the implementation is simpler.

## Duplicate taxonomy

“Duplicate” is not one error.

### D0 — exact replay duplicate

Same canonical ID and byte-equivalent content.

- idempotently deduplicate;
- increment normalization/duplicate count;
- output must otherwise be byte-equivalent.

### D1 — duplicate inside one semantic item

Same member/ref/reading repeated within one component/theme.

- deduplicate locally;
- preserve strongest evidence/fit;
- retain all distinct exact source locations;
- keep the component/theme.

### D2 — same public source identity with different support

Example: one supporting and one direct reading at the same
`(path,line,symbol)`.

- direct wins the default row;
- alternatives remain in evidence details when materially different;
- theme must not disappear.

### D3 — same member in multiple conceptual components

This can be legitimate cross-cutting participation.

- do not treat it automatically as fatal;
- classify as explicit cross-cutting when the contract supports it;
- otherwise publish an `ambiguous_overlap` / shared-membership diagnostic;
- preserve both components and all unrelated memberships;
- do not choose an arbitrary owner merely to satisfy a tree diagram.

### D4 — two components with equivalent resolved membership

- if semantically equivalent, coalesce deterministically and retain alternate
  labels/descriptions as provenance;
- if names/roles conflict, mark the equivalence class conflicted;
- keep all non-conflicting components;
- default Architecture may show one “conflicted grouping” card plus exact
  alternatives in details.

### D5 — response-local ID reused for incompatible objects

This is genuine ambiguity.

- reject every object using the ambiguous response-local ID;
- retain all other valid objects;
- report exact rejected refs/reason;
- do not reject the whole response unless every useful object depends on that
  ambiguous ID.

### D6 — Boundary and Resource share the same witness

This is not a canonical duplicate; it is two ontology roles over one fact.

- preserve both canonical records;
- project one user-facing interaction row until a distinct resource identity is
  known;
- show ontology roles under evidence details.

### D7 — multiple relations between the same visible nodes

- aggregate into one passive connector;
- retain witness count, kinds and exact sources in the row/details;
- do not draw parallel edge spaghetti;
- do not lose any witness.

### D8 — same label, distinct exact identities

Examples: two HTTP servers or several `Send` callsites.

- never deduplicate by label alone;
- group visually with count and exact children;
- preserve every distinct source identity.

## Publication ladder

Every semantic/product stage should finish in one of:

1. `accepted`
2. `accepted_with_normalization`
3. `accepted_partial`
4. `local_only`
5. `terminal`

`terminal` is reserved for cases where the underlying local authority cannot be
trusted or safely published.

A duplicate conceptual membership, duplicate prose label, invalid sibling, or
dense diagram is not a terminal condition.

## Diagram doctrine

Diagrams adapt to the truth; truth is never rewritten to fit a diagram.

When multiplicity/overlap exists, choose among:

- grouped row with count;
- cross-cutting badge;
- shared-membership disclosure;
- one aggregated connector;
- overlay/focused view;
- diagnostic remainder;
- separate independent mechanism fragments;
- list-first fallback.

Never:

- delete facts because ELK/layout becomes dense;
- force unique ownership to obtain a tree;
- invent an edge to connect islands;
- reject the report because one canvas projection is awkward;
- let a diagram acceptance gate invalidate an otherwise useful list/detail
  product.

## Information-retention accounting

Every stage changed by Hermes must record:

```text
considered
valid before normalization
normalized
conflicted
rejected
published in default
published behind disclosure
omitted under closed reason
candidate-set digest
```

Required reconciliations:

```text
considered
= normalized_unique + conflicted + rejected + omitted

published_accessible
= default_visible + disclosure_visible

all exact source witnesses
= visible representatives + disclosure witnesses
```

No silent loss.

## Required metamorphic tests

Hermes must add or identify tests for:

1. **Permutation invariance**  
   Reordering model items does not change accepted identity/result.

2. **Duplicate injection**  
   Adding an exact duplicate does not remove or alter unrelated output.

3. **Sibling poisoning**  
   Adding one unknown ref/malformed item preserves every valid sibling.

4. **Cross-cutting membership**  
   One member in two conceptual components becomes explicit shared/conflicted
   participation, not whole-stage rejection.

5. **Equivalent component collision**  
   Only the collision class is coalesced/quarantined.

6. **Boundary/resource co-projection**  
   Canonical records remain two; product row is one; witnesses reconcile.

7. **Edge aggregation**  
   N underlying relations become one visible connector with N witnesses.

8. **Projection invariance**  
   List, map and inspector use the same accepted IDs/counts.

9. **Monotonic evidence**  
   Adding exact evidence may strengthen/add claims but cannot remove unrelated
   published facts.

10. **Zero-valid semantic response**  
    Local Atlas/Architecture remains visible as `local_only`.

## Reviewer questions Hermes must answer before PASS

### Information preservation

- What exact facts existed before this change?
- Can any new malformed/duplicate item hide unrelated valid facts?
- Are all rejected/conflicted items counted and inspectable?
- Did any “cleanup” reduce source accessibility?

### Authority

- Which displayed statements became stronger?
- What exact evidence permits that strengthening?
- Is a shared scope being mistaken for runtime dependency or ownership?

### Projection

- Is duplication canonical, semantic, or merely visual?
- Can the UI coalesce without deleting ontology/evidence?
- Does the default view remain calm while every item stays reachable?
- Does changing diagram/list/focus mode preserve accepted identity?

### Cross-shape product value

- What happens on service, library, CLI, monorepo and zero-relation fixtures?
- Does one repository-specific workaround degrade another shape?
- Is diagnostic remainder still available without becoming the product?

## Concrete current Casdoor incident

The current report records a successful provider response that was rejected
because of `proposal.duplicate_component_identity`; 56 distinct members appeared
in 124 membership occurrences. The local Architecture remained available, which
is good, but the semantic response was not salvaged item-by-item.

The same report also contains Boundary and Resource rows with identical
database/net/SDK witnesses. Those must stay distinct canonically but become one
user-facing interaction row.

The next corrective must prove:

- one duplicate/collision class cannot erase unrelated conceptual groups;
- valid semantic siblings can publish as `accepted_partial`;
- exact local facts are always available;
- diagrams coalesce/annotate multiplicity instead of forcing rejection;
- every normalized/conflicted/rejected item is counted.
