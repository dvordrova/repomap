# 259 — Entry-call connectivity rejects only the bad selection

**Status:** APPROVED IMPLEMENTATION (owner-authorized, 2026-08-09)

**Preserves:** D248's one bounded refs-only call, exact local restoration,
resource limits, and prohibition on inventing missing connectors or runtime
semantics.

## Product defect

The exact Moby experiment returned sixteen known family refs. Fifteen formed
valid rooted directed selections; one deeper family omitted its advertised
connector. The old reducer rejected the complete response and reported zero
selected families. One item-local model mistake therefore discarded the useful
result of the whole provider call.

The provider wording also said only “connected directed subgraph”, while the
backend required every selected caller to be reachable from the root by
selected caller-to-callee edges. That mismatch made the rejection unnecessarily
likely.

## Contract

- The prompt defines rooted reachability explicitly and tells the model to
  include advertised connector families or omit the deeper family.
- Unknown roots or families, duplicate refs, malformed arrays, identity drift,
  and resource-bound violations still reject the complete response.
- For a structurally valid response, the backend restores every family that is
  reachable from its supplied root. A selected family that remains unreachable
  is rejected item-locally with reason `unreachable_from_root`.
- The backend never inserts the missing connector and never promotes the
  rejected family into the result.
- Result and status artifacts move to v2. A result containing item-local
  rejections has state `accepted_partial`, exact selected/rejected counts, and
  preserves the rejected request-local refs for audit.

This is a validation correction only. It does not change target selection,
graph expansion, provider-call count, report authority, or UI.
