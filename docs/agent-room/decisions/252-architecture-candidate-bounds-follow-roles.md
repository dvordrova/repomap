# 252 — Architecture candidate bounds follow candidate roles

**Status:** ACTIVE (owner-approved, 2026-08-09)

**Preserves:** D216's complete local unit compilation, D235's member-only
response grammar and structural-locator containment, D249's producer-owned
relation bounds, and the existing Architecture request-byte boundary.

**Supersedes:** only the single 512-candidate ceiling introduced before
candidate roles and structural context existed. It changes no provider schema,
prompt, model authority, report projection, or semantic call count.

## Product failure

The exact Linux-target Moby run built 543 Architecture candidates:

- 378 conceptual package members;
- 93 conceptual symbol members;
- 72 read-only file structural locators.

The validator counted all three collections against one old 512-item ceiling
and rejected Architecture before canonical grouping, unit compilation, request
encoding, or provider configuration. The provider therefore saw zero bytes.
The selectable conceptual collection was only 471 items and remained below the
existing runaway ceiling; the read-only containment lane caused the rejection.

The shared 512 constant first appeared with the original flat Landscape
contract, before candidate roles, `structural_context`, Architecture units, and
the current one-MiB request-byte guard. It has no demonstrated authority over
the combined current lanes.

## Contract

- Keep the existing 512 ceiling for `conceptual_member` candidates. They remain
  provider-visible response membership authority under the current member-only
  grammar.
- Bound `structural_locator` candidates independently at 4,096, matching the
  complete persisted Architecture-grounding producer envelope: 256 anchors ×
  16 associated members. Deduplication may reduce this count but never justifies
  retaining a prefix.
- Reject an overrun in either role with a role-specific typed limit before
  provider configuration. Invalid roles remain validation errors and acquire no
  new authority.
- Keep every admitted locator. It is the exact package → file → symbol bridge
  used by local unit ownership and is advertised only as read-only structural
  context; it can never appear in response `member_refs`.
- Existing unit compilation still compresses all conceptual members into at
  most 64 unit summaries. The current member-only prompt also retains the raw
  conceptual candidates on the wire; this decision does not pretend otherwise.
- The existing one-MiB encoded-request and prompt-byte checks remain the final
  provider-visible size protection. No aggregate candidate prefix clipping is
  added.
- Inputs already accepted before this correction produce byte-identical request
  and projection semantics. A capacity correction alone does not justify an
  Architecture schema, prompt, cache, report, or manifest identity change.

## Acceptance

1. A Moby-shaped bundle with 378 packages, 93 symbols, and 72 file locators
   builds one request with 471 conceptual candidates, 72 structural-context
   entries, at most 64 units, and bytes below the existing request limit.
2. 513 conceptual candidates fail with the conceptual candidate limit before
   provider use.
3. 4,097 structural locators fail with the structural-locator limit before
   provider use.
4. The saved report builder still preserves typed candidate-limit errors and
   their Architecture build context.

## Owner-risk check

This is the smallest product correction: one central role-aware validation
boundary plus causal coverage. It is needed next week for any large repository
whose exact grounding adds file containment to an otherwise legal conceptual
catalog. It fixes the rejection cause, not its warning or a test symptom, and
adds no new analysis or presentation path. Code complexity grows by one count
and one typed limit kind. Skipping it guarantees that the next Moby iteration
repeats the same zero-byte Architecture failure; no later provider or report
check can recover a request that was never constructed.
