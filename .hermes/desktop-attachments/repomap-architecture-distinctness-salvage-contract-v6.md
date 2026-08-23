# Repomap Architecture Distinctness, Salvage and Projection Contract v6

## Root product law

> Maximum useful visible truth with minimum unsupported assertion.

## Architecture acceptance is two-dimensional

A proposal item must satisfy both:

1. **Reference validity**
   - every required opaque ref exists;
   - backend canonicalizes redundant kind metadata;
   - unknown required membership refs reject only the dependent item.

2. **Information gain**
   - the component contributes exclusive exact members/anchors; or
   - it declares explicit shared/cross-cutting participation; or
   - it is a bounded conflict/alternative projection.

Syntactic validity alone is not acceptance.

## Unit compiler

- split broad units adaptively by stable repository path segment, exact anchor
  family and source package;
- never return to raw-member enumeration;
- retain stable identity and deterministic ordering;
- keep a bounded unit ceiling;
- publish a shared/unclassified remainder with complete counts/digest;
- expose representative labels, member-kind counts, role, relation count and
  anchor summary;
- no broad root/internal unit should be the only choice for several unrelated
  responsibilities when a bounded deterministic split is possible.

## Unit use

- primary unit ownership is exclusive by default;
- repeated use of one broad unit across components is not ordinary ownership;
- repeated units become shared/conflicted scope or are removed from individual
  ownership while anchor-specific slices survive;
- no “first component wins” resolution;
- output is permutation-invariant.

## Semantic salvage

Handle errors at the smallest safe scope:

- wrong redundant anchor kind with known opaque anchor ID:
  canonicalize and count normalization;
- unknown optional anchor:
  drop field, keep valid unit-backed component;
- unknown required unit:
  reject dependent component only;
- invalid component:
  keep siblings;
- equivalent resolved member sets:
  coalesce/quarantine equivalence class;
- zero independently valid components:
  local-only Architecture;
- unsafe/corrupt/stale local authority:
  terminal.

## Publication states

- accepted
- accepted_with_normalization
- accepted_partial
- local_only
- terminal

Persist:

- items considered;
- accepted;
- normalized;
- conflicted;
- rejected;
- exact members covered;
- shared;
- uncovered;
- candidate and result digests.

## Association scope

Track separately:

- witness precision;
- association scope.

Allowed association scopes:

- exact_symbol_scope;
- exact_file_scope;
- exact_package_scope;
- explicit_unit_scope;
- explicit_module_or_service_scope;
- shared_or_broad_scope;
- diagnostic_remainder.

Package paths match by exact package identity. Prefix matching is not package
ownership. Root package does not own descendants.

## Canonical vs product co-projection

Boundary and Resource may remain separate canonical roles over one evidence
set. One user-facing interaction row may represent both roles. Every witness and
canonical role remains available in details.

## Mechanism graph

Every transition has explicit source and target. Ordering is not inferred from
array position. A path exists only through supported endpoint joins.
Disconnected evidence becomes separate fragments or touchpoints.

## Required metamorphic tests

1. permutation invariance;
2. exact duplicate injection;
3. wrong anchor-kind canonicalization;
4. unknown optional anchor;
5. unknown required unit sibling poisoning;
6. repeated broad unit across components;
7. equivalent resolved member-set collision;
8. explicit shared/cross-cutting participation;
9. root-package non-inheritance;
10. Boundary/Resource co-projection;
11. list/map/inspector identity invariance;
12. connected-mechanism adjacency;
13. adding unrelated evidence cannot remove valid output or create ownership.
