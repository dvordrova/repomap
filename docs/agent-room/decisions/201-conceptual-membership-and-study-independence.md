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
proven local owner. The model selects and returns request-local typed refs
only; the current request also carries the exact locator context described
below.

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

## Owner-authorized current wire clarification

This clarification records the implemented wire after the original Decision
201 approval; it does not reinterpret historical Decision 200 artifacts.

The Architecture response is exactly one complete JSON object whose only root
field is one flat ordered `records` array. A subsystem record declares a unique
response-local `gN` ref, name and description. A component record points to one
of those refs with `subsystem_ref` and carries exact request-local typed
`member_refs` and `anchor_refs`, plus its name, description and hypothesis
flag. Records remain in conceptual display order. Nested records, extra root
fields, unknown or duplicate refs, duplicate keys, partial JSON and embedded
or fenced repair are invalid. Reusing one member ref in different components
expresses conceptual participation, never local ownership, and cannot change
the canonical D177 facts or relations.

The Atlas Study request exposes each reading target's exact normalized
repository-relative `path`, positive `line` and optional qualified `symbol` as
producer-owned read-only context. `allowed_paths` is exactly the sorted,
deduplicated set of all model-visible reading-target paths. No source bytes,
source body, snippet, raw graph, raw canonical ID, catalog/version/hash token
or backend-owned identity is model-visible. Identity fields return only the
advertised short request-local typed refs. After exact ref resolution,
direction prose may repeat only a locator advertised by one of that
direction's selected reading targets, and per-reading guidance may repeat only
its own target locator. Supported Brief statement text and domain-term meanings
may repeat only locators advertised by reading targets resolved from their
exact support refs; domain-term names cannot. Echoed locator text is decorative
and is never parsed back into identity, evidence, ownership or relations.
Canonical target identity and exact ref restoration remain local and are bound
through request, cache and artifact identity.

## Owner-authorized product continuation

The Go adapter may attach one optional producer-owned package-declaration
Evidence locator to a package Unit. The producer selects one deterministic
build-selected, tracked regular Go file, parses only its exact package clause,
and emits no evidence when that proof is unavailable or conflicts. Persisted
recognition is bound to the exact package Unit, stable Evidence identity,
location, package symbol and complete provenance contract. Unrecognized or
missing package evidence disables only that package source action; it never
fails an ordinary run or changes another Atlas fact or relation.

The existing saved-source projection may materialize those exact locators for
the package drawer. A package-only excerpt does not become an Atlas Study
reading target. When the same path and line is independently selected by an
Entry surface, Navigator recommendation or Architecture component, that exact
semantic target remains Study-eligible. This distinction is reconstructed from
persisted report and Atlas data and is not inferred from display order, a role
string, basename or package prefix.

For Atlas-first reports, the exact snippet-backed Overview anatomy is the first
visible product when it exists: Entry surfaces, Architecture components,
truthful integration absence and accepted Study routes. The Atlas shelf follows
as a compact inventory. Identically named repository, module and application
Units share one display card with role tags while retaining every canonical
Unit ID and relation. Packages remain collapsed by default, factor a common
module prefix only on exact boundaries, and open the source drawer only through
the exact adapter-owned package Evidence and its bound saved excerpt. Empty
Navigator and startup-relation states are collected in one compact diagnostic
after useful content instead of occupying the first screen.

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
