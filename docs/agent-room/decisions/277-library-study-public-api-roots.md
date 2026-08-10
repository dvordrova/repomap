# 277 — Library Study starts from exact executable public API

**Status:** ACTIVE (owner-authorized Go library corrective, 2026-08-10)

**Preserves:** Decisions 257–258, 260–264, 269, 273–276; one selected
analysis target per page; the existing Theme Scout and Adjudication stages;
the private DirectCallIndex/TargetRoots authority; names-only target-portfolio
wire; and strict saved-report/manifest replay without a provider call.

## Failure

Selected library packages such as Telebot and etcd library subpackages have no
process `main` and may have no discovered runtime surface, flow, handoff or
Architecture Component. Study treated those executable-oriented signals as a
mandatory admission gate and returned `no observed support roles`, even though
the exact build-selected package exposed useful Go callables such as `NewBot`,
`Raw`, `Download` and `File`. Assigning the package to LocalRemainder or
inventing a component owner would misstate authority.

## Exact public-API shelf

For an exact selected Go library target, Study uses a separate deterministic
`public_api_root` support contract:

- the principal is the selected package's literal Atlas package Unit;
- that Unit's canonical package path and module parent must equal the sealed
  selected `AnalysisTarget`;
- every reading is an exported top-level function or exported method with a Go
  body, taken from the exact build-selected `GoFiles`/`CgoFiles` declaration
  inventory and carrying its canonical repository path, line and column;
- no Component, LocalRemainder, surface, path-prefix, symbol-name or model
  heuristic is allowed to supply an owner;
- an exported method remains externally selectable even when its receiver type
  is unexported; this clause alone supersedes D274's stricter receiver clause;
- bodyless/assembly declarations remain names-only D274 selector evidence but
  do not become source readings or mechanism roots.

The snapshot-private declaration inventory persists only kind, label,
receiver, exact locator and executable-body presence. Source, comments,
signatures and bodies remain absent. The target-portfolio provider wire remains
names-only.

Before any Theme provider call, the live `TargetRoots` authority must match the
advertised public roots by exact selected target and exact `path:line`, with
complete omission accounting. Mismatch fails closed without fuzzy symbol
fallback. A type/constant/bodyless-only library legitimately has zero live and
zero advertised roots; it publishes typed `insufficient_catalog` with zero
semantic calls instead of terminating the report.

## Bounded provider frontier

The complete considered public callable set remains coverage authority. At
most 32 roots are advertised to Theme Scout in this Go-only request-budget
order:

1. constructors named `New`, or `New` followed by an uppercase letter or
   digit;
2. other exported top-level functions;
3. methods round-robin by receiver family.

Source order is exact within each group and the result is permutation-stable.
This is a request-budget hypothesis, not a semantic importance, ownership or
runtime claim. The same selected order owns `a*` seed allocation and source
budget consumption, so `NewBot` and receiver-family breadth cannot be lost to
catalog hashes, path sorting or a second 64-anchor order. Saved replay verifies
every retained seed's exact locator, public-API role and canonical focused span,
plus the complete `seed_budget` omission partition.

Source evidence remains atomic. If any seed object or whole seed pack exceeds
its source-byte bound, that seed is omitted under the same exact `seed_budget`
accounting and a later bounded root may survive; code bytes are never truncated
and an oversized first constructor may not overflow the Scout artifact.

## Replay and privacy

The compiled Atlas Study private catalog binds the full selected library
`AnalysisTarget` snapshot and its exact package Unit. Provider wire omits both.
The saved workspace snapshot is the bounded replay authority for exact private
Go declarations; report reads and manifest verification rehydrate it before
both artifact and no-artifact projection. Public root files join the captured
openable-source authority. Ordinary artifact reconstruction may derive those
paths from the manifest-bound snapshot, while persisted report/manifest replay
requires the saved openable set to contain them already and never repairs it.
Arbitrary package Units, non-callable roots,
out-of-package declaration paths, stale target catalogs and selected-target
identity echoes in provider prose all fail closed.

Identity advances are Atlas Study request v8, prompt v14, result v9, target
catalog v3 and Atlas Study report projection v16. Report format 45,
run manifest v18 binds the exact `snapshot.json` bytes consumed by replay;
target-portfolio request/result identities and provider-visible target symbols
remain unchanged.

## Verification

Provider-free tests cover a Telebot-shaped zero-surface package with more than
32 callable roots (`NewBot` plus multiple receiver families), a small
etcd-shaped library, bodyless/types-only unavailability, exact declaration and
selected-file tampering, arbitrary Unit principals, hidden-receiver exported
methods, source authorization, catalog/replay privacy, permutation stability,
tight Scout source budgets, canonical-span swaps, omission shrinkage and saved
no-artifact report replay. Existing executable shelf bytes and system-path
semantics remain unchanged.

Approved by:
    Repository owner after fresh Telebot and etcd target pages showed exact
    library public APIs but Study incorrectly reported no observed roles, and
    after choosing exact Unit-rooted public callables over fabricated Component
    ownership, 2026-08-10.
