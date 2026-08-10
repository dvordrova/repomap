# 264 — Provider-visible target roots for Study trails

**Status:** APPROVED IMPLEMENTATION SLICE (owner-authorized, 2026-08-10)

**Preserves:** Decisions 243, 246, 256, 260 and 261; refs-only mechanism
responses; exact private target and reading authority; the existing card,
request, response and path bounds; item-local candidate rejection; and legacy
reading-local experiment semantics.

## Product defect

Decision 260 correctly required every targeted mechanism to start at an exact
selected-product root and cross an exact Study reading. The private card
authority and reducer enforced that contract, but the provider-visible card
omitted the target roots and the prompt asked only for a path crossing any
reading. A model could therefore return the locally useful reading suffix it
was explicitly shown how to choose; the reducer then correctly rejected it as
`not_target_rooted`. Current targeted runs could publish zero mechanisms even
though their advertised graph contained a complete valid connector.

This is a provider-contract mismatch, not a graph-depth, edge-limit or reducer
problem. Raising a limit or accepting the suffix would repair the symptom and
drop the selected-product guarantee.

## Exact corrective

A targeted provider card adds one optional `target_root_refs` array. It
contains only exact request-local `n*` refs already present in that card's
bounded node list. The compiler derives the array from the existing private
Decision 260 target-root authority in exact node order. It exposes no target
identity, canonical node ID, package, symbol, source path or source body.

The field is absent, not empty, on every legacy reading-local card. Public
request validation requires every supplied target root to be a unique known
`n*` ref within the existing 32-node card bound. Compilation validation binds
the ordered public array exactly to the private target-root set and the
catalog digest; facts restoration rebuilds the private set from exact target
node IDs and revalidates the public projection. A removed, substituted,
duplicated, reordered, unknown or private root fails closed.

The prompt applies one conditional rule only when `target_root_refs` is
present: the backend-reconstructed path's first edge caller must be one of
those roots, the candidate must include every advertised connector edge from
that root through a supplied reading root, and a local suffix is invalid.
Cards without the field retain the existing reading-local rule.

## Identities and exclusions

The provider request schema advances from version 2 to version 3 and the
content-derived prompt identity becomes
`mechanism-study-prompt-3d2ec6b365d6`. Catalog, request-batch and facts
identities already bind those values and therefore miss closed. The response
schema/result remain version 2 because the provider still returns only card
and edge refs. Compilation version 2, artifact version 1, card/edge limits,
provider-call count, report, manifest and UI identities do not change.

No graph expansion, new model call, semantic retry, path-role heuristic,
reducer relaxation, report field or compatibility reader is added.

## Acceptance

1. A targeted repomap-shaped card exposes its exact selected main only as an
   `n*` ref and leaks no canonical target, module, path or symbol identity.
2. The complete target connector through the reading is accepted while its
   otherwise identical local suffix is rejected as `not_target_rooted`.
3. Unknown, private and duplicate target-root refs fail request validation;
   public/private projection tampering fails compilation validation.
4. Facts round-trip restores byte-identical cards plus exact private root
   authority.
5. A legacy reading-local request omits `target_root_refs` and retains its
   previous candidate semantics.
6. Full `internal/mechanismstudy` tests, vet and diff checks pass.

Approved by:
    Repository owner after the first target-rooted ordinary runs returned zero
    mechanisms and the exact audit showed that the model was never told which
    advertised nodes the already-strict reducer required as path starts,
    2026-08-10.
