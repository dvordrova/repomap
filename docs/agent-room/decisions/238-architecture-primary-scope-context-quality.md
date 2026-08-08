# 238 — Architecture primary-scope context and quality

**Status:** ACTIVE (owner-authorized, 2026-08-08)
**Supersedes:** only Decision 237's temporary constraint that the Architecture
model-visible request and prompt remain byte-identical while the
`anchor_refs` presence incident was isolated.
**Preserves:** Decisions 235–237, one bounded Architecture provider call,
request-local opaque refs, local deterministic authority, exact local Canvas
fallback, Study continuation, raw-source/path/edge/privacy boundaries, and
historical saved-response replay.

## Incident evidence

The clean D237 PRE and FIX ghz calls sent byte-identical Architecture requests:
13,358 bytes, SHA-256
`48fee34b0d0f1167dcedc8940abe59e3991aa9c06fc4a71d7715a939c05b4296`.
Both stochastic responses selected all ten supplied symbol refs and zero of
the eighteen supplied package refs. They therefore produced a technically
valid 10/28 `accepted_partial` result centered on TLS, configuration, process
entry symbols, and validation while omitting the CLI/load and web package
architecture.

The bounded request already carried a unit catalog, but its projection lost
the context needed to use it:

- conceptual symbols owned through `symbol -> structural file -> package`
  were placed in `local remainder`, because the compiler recognized only an
  immediate package parent;
- anchors were attached before the final remainder/split units existed;
- `representative_labels` read private opaque member IDs and consequently
  dropped every `member-*` value;
- a host/module prefix became the unhelpful unit label `bojand`;
- candidates did not carry an exact request-local unit association or a
  backend-owned distinction between primary repository scope and supporting
  behavioral evidence.

The live prompt also explicitly allowed representative-only selection and
`unit_refs`, so an all-symbol answer obeyed the instruction. This is a request
projection and quality-contract problem, separate from the corrected D237
`anchor_refs` presence bug.

## 1. Deterministic unit projection

The local unit compiler resolves a conceptual member's nearest conceptual
package owner through the bounded candidate/structural-locator parent graph.
Resolution is cycle-checked, terminates within the finite advertised catalog,
and never reads or publishes additional repository content.

- direct `member -> package` and `member -> file locator -> package` ownership
  produce the same package unit;
- unresolved or cyclic ownership stays explicit and local; it is never guessed;
- process-entry ownership uses the same resolved package relation;
- final units are constructed and split first, then anchors are attached to
  the exact final unit containing each anchor member;
- existing bounded `representative_labels` come from sanitized candidate
  display names, never canonical/opaque IDs;
- module-unit labels are repository-relative semantic labels, never a source
  host, organization, full import path, or per-repository keyword rule;
- role classification remains closed and locally evidenced. D238 does not
  infer test/tooling/production role from provider prose.

The private catalog retains exact member-to-final-unit ownership so the wire
projection can restore it without exposing canonical unit or member identity.

## 2. Bounded candidate context

Every provider-visible conceptual candidate carries only existing or bounded
request-local context:

- `parent_ref`: the nearest conceptual package ref when one is resolved;
- `unit_ref`: exactly one advertised request-local `u*` ref owning the member;
- `coverage_role`: one closed backend-owned value:
  `primary_scope` or `supporting_evidence`.

When conceptual package candidates exist, packages are `primary_scope` and
members resolved beneath such a package are `supporting_evidence`. Unowned
conceptual candidates remain `primary_scope`. When no conceptual package
candidates exist, every conceptual candidate is `primary_scope`; behavior-only
and library-symbol bundles therefore remain usable. This role is request-local
quality context, never ownership, execution proof, or a model-authored claim.

All three fields are read-only and must never be returned. They add no paths,
qualified symbols, canonical IDs, raw relations, source, evidence bodies,
provider prose, or credentials. Request size and identity validation remain
closed and bounded.

The live response prompt permits only `member_refs` plus optional
`anchor_refs`. It no longer advertises `unit_refs` as live output. Historical
flat/nested saved responses containing `unit_refs` remain replayable under
their existing versioned decoder and backend expansion contract.

The prompt states that `primary_scope` forms the conceptual repository surface:
cover defensible primary scope across supplied units before selecting
`supporting_evidence`. Supporting symbols and anchors ground or distinguish
responsibilities but do not substitute for primary-scope coverage. Honest
partial primary coverage remains valid; padding, invention, exhaustive
enumeration, repair, and retry remain forbidden.

Model-visible request/prompt/cache identities advance from their exact new
bytes. No compatibility alias may make old and new provider requests share a
cache key.

## 3. Closed local quality gate

After exact response refs are resolved locally:

1. if the request contains primary scope but the proposal covers zero primary
   refs, the proposal is rejected with
   `proposal.empty_primary_scope_coverage`;
2. if a proposal covers supporting evidence from a unit but covers no primary
   scope from that same unit anywhere in the proposal, it is rejected with
   `proposal.supporting_only_unit_coverage`;
3. package and symbol refs are not required to share a component. TLS/shared
   participation remains valid once the containing unit has defensible primary
   coverage elsewhere in the proposal;
4. no percentage threshold or exhaustive package enumeration is introduced.
   Ambiguous primary refs remain in the exact deterministic local remainder;
5. quality rejection performs no repair, retry, or second semantic call. The
   exact local Canvas is published and Study continues.

Successful/cached synthesis metadata, Architecture status, semantic exchange,
and console retain bounded primary-scope counts: requested primary, covered
primary, uncovered primary, and covered supporting evidence. A user can judge
`accepted_partial` without opening request/response JSON. Failed quality status
retains the stable diagnostic codes and the same bounded counts but never
publishes model membership.

Architecture status advances 12 -> 13 for these fields. Existing v1–v12 status
artifacts remain readable under their historical contracts.

## 4. Verification

Provider-free regressions prove:

- direct and file-mediated package ownership, cycle/unresolved handling, final
  anchor attachment, non-empty representative labels, and safe module labels;
- exact ghz-like projection: eighteen primary package refs, ten supporting
  symbol refs, correct package parents/unit refs, and no symbol-only local unit;
- request absence of canonical IDs, repository paths, qualified source
  identities, raw package-import edges, credentials, and unbounded labels;
- an all-symbol ghz response is rejected with the closed empty-primary and/or
  supporting-only diagnostic and never cached as accepted;
- a response with defensible package scope plus separate supporting/TLS roles
  remains honestly `accepted_partial`;
- a behavior-only bundle can designate its symbols primary and remain accepted;
- historical `unit_refs` replay, D237 omitted/empty/null `anchor_refs`, and
  full 28/28 explicit-empty coverage remain unchanged;
- request/prompt/cache/status identities and diagnostic parity are exact.

Then run focused tests, full `go test -count=1 ./...`, `go vet ./...`, build the
exact clean candidate with `go build -trimpath`, and run it offline on ghz.

Finally, one fresh uncached ghz provider run is owner-authorized. It is a single
acceptance observation, not a retry or prompt-tuning loop. Verify exact build
identity, request SHA/bytes, primary/supporting coverage in console/status,
semantic exchange, synthesis acceptance or honest quality rejection, Atlas,
report JSON/HTML, manifest hashes, and repository freshness. No second D238
provider call is authorized without a new owner instruction.
