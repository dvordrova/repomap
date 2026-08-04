# Decision 204: Conceptual members and structural locators

## Status

Approved by the repository owner after the single Decision 203 calibration
proved that an exhaustive checklist cannot make structural containment nodes a
stable conceptual grouping contract. The live response returned all eight file
refs mechanically beside their symbols, yet still omitted the exact
`controllers` package and was rejected at 49/50.

## Outcome

Architecture candidates have one producer-owned role:

- `conceptual_member` is eligible for model-authored many-to-many conceptual
  membership and belongs to the exact required response coverage set; or
- `structural_locator` is complete read-only local containment/source context
  and can never occur in model-authored membership.

Role assignment is not inferred by the provider, basename, ordering or a
global member-kind rule. In the current Go producer, the intermediate file
nodes created solely to connect an exact package/file/symbol chain are
structural locators. A semantically meaningful file in another producer or
repository shape may remain a conceptual member.

The provider request contains only conceptual candidates in `candidates` and
`required_member_refs`. A separate complete `structural_context` projects each
locator through request-local typed refs plus its exact conceptual parent and
child refs. It is context only. A locator ref returned in `member_refs` is a
wrong-kind identity failure; missing conceptual coverage rejects enrichment
atomically.

D177 remains authoritative. Every local candidate, fact, relation, anchor and
containment link survives independently of model acceptance. Accepted
enrichment adds only conceptual many-to-many memberships. Structural locators
are retained in one explicit local projection and mapped to zero or more
participating conceptual components through exact containment links. They do
not acquire an owner, primary component or remainder component.

## Status and identity

Architecture status distinguishes:

- complete local candidate count;
- requested conceptual-member count;
- structural-locator count;
- response membership occurrences and distinct conceptual coverage.

Success requires exact distinct coverage of the requested conceptual set, not
the total local-candidate count. Cache, record, proposal, Landscape and status
identities advance with the new role semantics and exact provider body. Older
records miss closed and are never migrated or reinterpreted.

## Acceptance

- the saved Casdoor shape is 50 local = 42 conceptual + 8 structural;
- a complete package/symbol response is accepted without returning files;
- omitting `controllers` or any other requested conceptual ref is rejected;
- returning any structural locator ref is rejected as wrong-kind;
- valid cross-component conceptual membership remains many-to-many;
- every structural locator and exact containment/source link remains locally
  navigable after accepted or rejected enrichment;
- component participation recovered through containment is plural and sorted,
  never choose-first ownership; and
- a semantic-file fixture proves role assignment is producer-owned rather than
  a global `kind=file` exclusion.

No raw Orientation, source bytes, model-owned path, fuzzy repair, structural
cross-product, legacy reader, migration or additional prompt-only retry is
introduced.
