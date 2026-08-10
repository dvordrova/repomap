# 274 — Exact semantic symbols for Go target portfolios

**Status:** APPROVED IMPLEMENTATION (owner-authorized, 2026-08-10)

**Preserves:** Decisions 257, 262, 263 and 269; the complete exact target
catalog; one refs-only portfolio response; one Go build scenario; local target
restoration; and fail-closed request/catalog binding.

## Product defect

The Decision 263 request exposed only package display paths and the closed
`executable` / `library` kind. On repomap that path-only evidence did not let
the model distinguish the product executable from top-level development and
preview commands, so it selected every `cmd/**` target. A purpose role inferred
from paths would repeat the same mistake locally.

The owner-approved correction is the previously reserved shape
`{ref: package, symbols: [...]}`: give the one editorial selector exact
package-owned declaration labels, not more path heuristics.

## Exact local declaration authority

During the existing safe Go-facts pass, `gofacts` parses only build-selected
package `GoFiles` and `CgoFiles` through the repository-confined reader. A
defensive `_test.go` exclusion remains even if malformed input advertises such
a file. No new `go list`, package load, SSA build or provider stage is added.

The snapshot persists only declaration identity: kind, identifier and the base
receiver identifier for methods. This is the exact local authority later
sealed into the target catalog. Source bytes, bodies, signatures, comments,
literals and file locations do not cross this projection. A package scan is
atomic: a missing, unsafe, oversized or unparsable selected source marks its
declaration inventory unavailable instead of publishing a prefix.

Executable targets expose every top-level function, receiver-qualified method,
type, variable and constant from those non-test files. Library targets expose
only exported top-level declarations and methods whose method and receiver
identifiers are both exported. Blank identifiers are omitted. Canonical kind,
receiver and name ordering plus exact de-duplication make file and fact
permutation irrelevant.

## Provider wire and identities

Every complete target remains present in canonical catalog order. Target
portfolio request v2 groups declaration labels under their owner:

```json
{
  "ref": "t1",
  "display_path": "cmd/repomap",
  "kind": "executable",
  "symbols": [
    {"kind": "func", "names": ["main", "runDevUICLI"]},
    {"kind": "method", "names": ["Server.Start"]}
  ]
}
```

There is no flat symbol collection, source, comment, canonical package ID,
root, edge, score or purpose role. The response remains refs-only. Target
catalog v2 seals the exact filtered symbol inventory. Compilation, request and
result identities advance to v2; the request ref, request SHA, compilation
seal and prompt hash therefore invalidate old cache entries cleanly.

## Bounded cost

The original 64 KiB semantic request cap cannot represent the approved exact
inventory. A provider-free measurement on the current repomap tree produced
137 targets, 5,801 declaration labels and 156,281 canonical request bytes.
No label or candidate may be prefix-truncated, ranked away or split into a
second semantic call.

The dedicated semantic request cap is therefore 256 KiB. Because those bytes
are embedded and escaped inside the final OpenAI-compatible JSON message, the
transport body has a separate exact cap of `2 * 256 KiB + 64 KiB` (576 KiB).
The model sees only the at-most-256-KiB semantic bundle plus bounded prompt
text after JSON decoding. The refs-only response remains capped at 64 KiB and
the configured global output-token ceiling remains unchanged.

This spends more input on the one repository-level editorial call to avoid the
much larger cost and broken product shape of generating independent full pages
for every development package. Oversize at either exact boundary fails before
network access; there is no truncation, retry, judge or local post-selection
balancer.

## Acceptance

Provider-free tests prove build-target and non-test extraction, executable
completeness, exported library filtering, receiver qualification, canonical
permutation stability, symbol/catalog/request tamper rejection, private source
and identity non-disclosure, complete candidate retention, semantic oversize
failure, escaped provider-envelope capacity, pre-network transport rejection,
strict refs-only response restoration and runtime use of the new identities.

Approved by:
    Repository owner after the path-only selector chose repomap development
    commands and after reviewing the exact 137-target / 5,801-symbol /
    156,281-byte provider-free size checkpoint, 2026-08-10.
