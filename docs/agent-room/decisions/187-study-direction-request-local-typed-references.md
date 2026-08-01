# Decision 187: Study direction request-local typed references

## Status

Approved by the repository owner in the current session as the next smallest
Study reliability checkpoint after Decision 186. Historical decisions,
including the broader queued Decision 173, are not rewritten.

## Problem

The Study direction candidate stage asks a model to copy long backend-owned
anchor, document, area, and mechanism IDs. One otherwise valid response lost
bytes from the middle of an anchor ID. The production path also retained a
unique-prefix repair, which made canonical identity depend on model string
copying and a partial-match protocol.

## Decision

Only the `study_direction_candidates` provider seam changes. The backend builds
one deterministic private catalog over the exact ordered candidate-stage
anchor, document, area, and mechanism identities. The model-visible direction
bundle replaces those canonical IDs, including anchor-to-area and
mechanism-to-anchor links, with compact typed ordinal references such as `a1`,
`d1`, `r1`, and `m1`. One bounded top-level `catalog_ref` binds the response to
the exact candidate input, catalog order, prompt contract, response contract,
and validator contract.

The provider returns the exact `catalog_ref` once and returns only typed refs in
typed fields. Local resolution first checks the catalog token atomically, then
resolves each item by exact reference and expected kind. Unknown, wrong-kind,
duplicate, cross-request, substituted canonical-ID, shortened, prefixed,
compacted, and otherwise corrupted refs are rejected. There is no fuzzy,
prefix, edit-distance, semantic, or string repair. The previous production
unique-prefix resolver is removed rather than retained as a legacy path.

An envelope or catalog-token failure rejects the response. A reference or
semantic failure inside one independently proposed candidate rejects only that
candidate with its existing bounded position plus a closed diagnostic code;
valid siblings retain their exact relative order. Diagnostics persist no raw
ref or provider prose. Canonical IDs are restored before the existing
normalizer, validators, local direction-ID derivation, review scheduling, and
publication run.

The saved incomplete-Study projection rebuilds the same exact catalog from its
hash-verified bundle and resolves typed reading-anchor refs before projecting
the existing weak local navigation result. It does not accept the earlier
canonical-ID provider shape or infer missing complete-pack fields.

The candidate prompt advances to v5. Its private catalog identity binds the
exact catalog/order plus prompt, response, and validator contracts, while the
exact typed wire and `catalog_ref` are part of the provider request bytes.
Candidate request/content-addressed identity therefore changes on catalog or
contract drift. Earlier candidate-stage entries are misses; there is no old
response reader, repair, or migration, and `repomap cache clear` remains the
supported invalidation operation.

The downstream per-direction reading-pack review cache stays on its existing
v1 contracts and namespace. It remains keyed by each exact review request,
review bundle, and source fragments. An upstream candidate catalog change does
not invalidate a byte-identical review request, and one changed source fragment
continues to invalidate only its affected review entry.

BriefShape input/output, `shape_area_ids`, reading-pack review prompts and
responses, per-direction splitting, candidate count/composition/order/evidence,
review validation, publication limits/order, canonical direction IDs, canonical
Study DTO/JSON, report, UI, localization, Architecture, Atlas, provider retry,
cache framework, clients, flags, and live-model policy remain unchanged.

## Proof

Provider-free tests establish that:

- catalogs and short typed refs are deterministic for an exact ordered input;
- catalog order or candidate contract drift changes the exact candidate request
  and its content-addressed identity;
- a valid exact round-trip restores byte-identical canonical candidates,
  direction IDs, and Study JSON;
- unknown, wrong-kind, duplicate, canonical-ID substitution, compacted, and
  corrupted refs reject only their candidate while valid siblings remain
  unchanged and ordered;
- a cross-request or mid-token-corrupted `catalog_ref` rejects atomically;
- rejected complete candidates retain the same exact typed incomplete-Study
  starts in provider order;
- the provider bundle contains typed refs rather than canonical IDs;
- the unchanged review cache still reuses unaffected directions when one source
  fragment changes.
