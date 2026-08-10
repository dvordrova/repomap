# 249 — Architecture relation bounds follow producer authority

**Status:** ACTIVE (owner-approved, 2026-08-09)

**Preserves:** D223 unit-level package-edge aggregation, D235 complete
candidate input with typed exhaustion rather than prefix clipping, D243 exact
workspace-graph authority, and D247 publication truth.

**Supersedes:** only the single mixed 1,024-relation ceiling introduced before
the current unit projection. It changes no provider schema, model authority,
report projection, or default semantic call count.

## Product failure

The clean Moby run produced 1,056 unique exact package-import relations. The
candidate validator rejected them before Architecture request construction
because package imports and behavior handoffs shared an old 1,024-item guard.
This was not provider protection: with an Architecture unit catalog, raw
package-import relations are excluded from the provider request and only a
bounded per-unit outgoing count is visible.

The overage of 32 is not the reason for this decision. The defect is that the
ceiling described neither producer authority nor provider-visible cost.

## Contract

- Keep the complete exact package-import collection up to the existing
  workspace-graph authority. Do not retain a sorted prefix.
- Bound behavior handoffs independently by the existing persisted
  Architecture-grounding authority.
- Reject an overrun in either category with a category-specific typed limit
  before provider configuration.
- Unit compilation consumes the complete accepted package-import collection
  when deriving relation counts. Raw package imports remain absent from the
  provider request whenever the unit catalog is present.
- Existing inputs at or below the former ceiling retain byte-identical request
  and projection semantics; no identity bump is justified by capacity alone.

## Evidence gate

A provider-free Moby-shaped replay must preserve all 1,056 unique local import
relations, emit zero raw package-import relations in the synthesis request,
retain the exact unit relation aggregate, and remain permutation-stable. The
independent N+1 boundary of each category must still fail closed.
