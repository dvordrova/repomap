# Decision 201: Conceptual membership and Study independence

## Status

Approved explicitly by the repository owner after the Decision 200 Casdoor
run returned a complete Russian Architecture proposal but rejected it solely
because the `service` package appeared in both the provider-lifecycle and
business-service conceptual components. The exact D177 local Canvas remained
valid, while the temporary Architecture-acceptance gate prevented Atlas Study
from running.

## Outcome

Model Architecture expresses conceptual grouping, not ownership. One exact
member may participate in multiple conceptual components. Operational or
navigation ownership is a separate optional local relation and is never
inferred from model membership, component order, confidence or a choose-first
rule.

Atlas Study becomes a consumer of a usable validated local Canvas and exact
reading catalog, not of `ProposalAccepted`. Accepted model grouping enriches
the Canvas; rejected or absent enrichment leaves the complete D177 local
Canvas as the Study input. One physical source locator is represented once,
independent of how many conceptual components refer to it.

## Conceptual membership contract

The provider wire remains the Decision 200 exact request-local typed-ref
proposal. It gains no primary, related or owner field. After exact resolution,
the backend materializes a canonical sorted many-to-many relation:

```text
ConceptualMembership { ComponentID, MemberID }
```

`Component.Members` may remain a materialized projection for compatibility,
but the relation has one canonical source of truth. The original provider
response and proposal SHA remain unchanged.

Fail-closed rules:

- a duplicate member within one component is malformed;
- unknown, wrong-kind, raw-canonical, cross-request or unresolvable refs reject
  the proposal;
- the same exact member in different components is valid;
- coverage and remainder use distinct members, while occurrence and membership
  counts remain separate diagnostics;
- identical exact member-set components remain invalid because their canonical
  component identities collide;
- stage-owned total-membership and per-member bounds are distinct from the
  candidate bound and fail closed when exceeded; and
- reordering provider objects never changes canonical relations or identities.

Decision 200 artifacts and caches are not reinterpreted. The component
contract, prompt/request/cache/record and accepted status semantics advance
without a legacy reader or migration.

## Ownership and consumers

Conceptual membership alone never proves `OwningComponentID`. A producer may
prove an exact source, Surface or anchor relation to a local package, Unit or
D177 component, but that does not become ownership of a model conceptual
component merely because the model grouped the member there. Ownership remains
absent unless an independent exact local proof targets the same typed ownership
domain uniquely. Zero or conflicting proof leaves it absent.

Flows, Surfaces and search consumers may expose a canonical sorted union of
`ParticipatingComponentIDs`. Singular owner fields remain empty when ownership
is absent or ambiguous. Owner-only actions are unavailable. Structural edges
are emitted only for unique exact endpoints; no conceptual cross-product is
created. Exact flow bindings remain inspectable even when no singular component
is assigned.

Bounds distinguish distinct members from conceptual membership edges so a
cross-cutting package cannot consume the entire search or projection budget by
occurrence multiplicity.

## Atlas Study target and gate

Study compilation creates one reading target per exact normalized locator
identity (`path`, line/range, symbol and closed kind), never one target per
conceptual component. A target carries plural exact conceptual
principal/component associations separately from an optional independently
proven local owner. The model sees request-local typed refs only.

A direction is valid only when each selected reading target intersects at least
one selected principal association. Minimum reading cardinality counts distinct
exact locators, not owner-expanded refs. Unknown, wrong-kind, duplicate,
cross-request or owner-outside-catalog refs fail closed without fuzzy repair.

An online Study call is eligible when:

- Atlas and a complete usable validated local Canvas exist;
- the compiled catalog contains at least three distinct exact reading locators;
- provider execution is enabled; and
- ordinary repository authority remains valid.

`ProposalAccepted` controls model Architecture enrichment only. If enrichment
is rejected or unavailable, Study uses the complete D177 local Canvas and never
any partially rejected model proposal. Offline execution still makes zero
provider calls. An insufficient local catalog is a closed provider-free
unavailable state that still publishes the authorized local report and
manifest; it is not a terminal run error.

Atlas Study prompt, model, artifact, manifest and cache identities advance.
Decision 200 Study artifacts are not replayed under the new target contract.

## Acceptance

- the saved Casdoor `service` proposal is accepted as 29 occurrences over 28
  distinct members, preserves both conceptual memberships and omits `service`
  from remainder;
- duplicate-in-component and identity failures still reject the whole proposal;
- total and per-member membership bounds fail closed;
- accepted status permits occurrences greater than distinct and obtains counts
  from the authoritative resolver rather than a contradictory raw parser;
- a shared member yields plural sorted participants, no inferred owner and no
  structural cross-product;
- exact unique producer ownership is preserved, while zero/conflicting proof
  remains absent regardless of ordering;
- one locator associated with two components remains one Study target and
  cannot satisfy the three-target minimum by duplication;
- every accepted route target intersects one selected principal;
- rejected model enrichment plus a valid D177 Canvas makes exactly one Study
  call; an insufficient catalog makes zero calls and publishes a closed local
  product;
- manifest and replay accept only the new exact identities; and
- no raw Orientation, old Study fan-out, primary/related model owner, fuzzy
  repair, choose-first, legacy reader, migration or UI workaround is added.

Provider-free tests and the saved Casdoor response gate precede one new
Casdoor comparison. The repository owner subsequently authorized the root
supervisor explicitly to perform that installed-binary live acceptance with
the existing credentials and to iterate until the product result is useful.
Source and implementation subagents remain prohibited from making live
provider calls, and credentials or Authorization data are never printed.
