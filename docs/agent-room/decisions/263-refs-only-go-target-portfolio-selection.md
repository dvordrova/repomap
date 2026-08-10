# 263 — Refs-only Go target portfolio selection

**Status:** APPROVED IMPLEMENTATION SLICE (owner-authorized, 2026-08-09)

**Preserves:** Decisions 257, 261 and 262; complete local target authority;
explicit `--target`; the configured global provider output ceiling; one exact
Go build scenario; and the existing single-target downstream pipeline.

## Product defect

Exact local facts can enumerate executable and library package targets, but
they do not prove which packages are independent products worth presenting.
Path-based `product`, `fixture`, `generated`, or `auxiliary` roles turn local
conventions into false authority. Selecting sorted-first or applying a local
portfolio balance cap would make the same error deterministically.

The model is useful here for one small editorial question: which exact targets
form the useful repository portfolio, and which one should open first. It must
not create a target, rename a package, explain the repository, or influence any
target's later Architecture or Study evidence.

## Approved semantic contract

`internal/targetportfolio` compiles the complete sealed Decision 262 catalog
into one bounded provider projection. Each target receives a request-local
`t*` ref and exposes only:

- the flat repository-relative display path; and
- the closed structural kind `executable` or `library`.

The request also carries the repository display name and one opaque
`request_ref` bound to the exact private catalog and wire. Canonical module and
package identities, exact target refs and roots, entrypoint guesses, source,
symbols, README, raw files and graph edges remain local.

The response is exactly:

```json
{
  "version": 1,
  "request_ref": "tpq-...",
  "default_ref": "t1",
  "target_refs": ["t1"]
}
```

It contains no prose, score, role, explanation or path. The backend requires a
known unique non-empty ref set, requires the default inside that set, rejects
unknown fields and trailing JSON, and restores exact catalog entries in local
canonical order. Response order has no authority. An old response cannot be
applied to a different private `t*` mapping.

The prompt defines portfolio granularity through its product consequence: each
selected ref becomes a separate top-level report scope in the left navigation,
with its own Architecture canvas and inline Study content; selecting it switches
the whole report scope. It asks for the smallest non-redundant set of independent
products or downstream-consumed library surfaces, explicitly says that this is
not package coverage, and permits a one-target portfolio. Supporting
implementation packages remain inside their owning target's analysis instead
of becoming sibling scopes. This is semantic guidance, not a numeric cap or a
locally asserted package-purpose role.

The complete request and semantic response are each bounded to 64 KiB. The
compiler never prefix-truncates or imposes a semantic target-count cap; an
oversized complete catalog fails before the provider. The exact final
OpenAI-compatible envelope is independently checked against the same request
limit before network access. The configured global
`REPOMAP_LLM_MAX_TOKENS` remains unchanged: a stage-specific token override is
forbidden by the shared provider contract. Official DeepSeek disables thinking
for this bounded classification; compatible endpoints receive no proprietary
extension. Only immutable transport bytes may be retried.

## Runtime boundary

The selector belongs after the complete deterministic Go facts and Decision
262 catalog, but before `ScopeGoFacts`. It does not run inside snapshot
extraction and must not trigger a second snapshot, `go list`, package load or
SSA build.

An explicit `--target` bypasses semantic selection. Offline mode retains the
existing exact local default/ambiguity behavior. A future literal
`--all-targets` bypasses selection and uses the complete catalog; it is not
implemented by this decision. The first runtime integration consumes only the
accepted semantic default and then runs the unchanged single-target pipeline;
the selected portfolio is diagnostic evidence for the later multi-target
container, not yet report authority.

Provider/response failure may fall back only to the already-approved exact
local catalog default. Without one, target selection is terminal and presents
explicit `--target` choices. Cancellation remains terminal. There is no
sorted-first choice, semantic retry, judge or local role repair.

## Acceptance

Provider-free tests prove complete-catalog compilation, 400-target selection,
byte-bound failure without truncation, request/catalog binding, permutation
stability, strict malformed/unknown/duplicate/default rejection, private
identity and credential non-disclosure, exact-envelope preflight, raw invalid
response preservation, configured global token ownership and immutable
transport retry.

The product go/no-go is one fresh repomap call. It must select `cmd/repomap` as
default and must not turn the repository's implementation packages into
separate sibling report pages. A first exact call selected the correct default
and excluded testdata, but returned 78 targets: six development commands plus
every remaining `internal/**` package. That result proved the original word
"portfolio" was underspecified rather than proving that paths were unreadable,
so the page-consequence contract above must be rerun before adding more input.
If it still produces package coverage, the next experiment may add bounded
locally extracted declaration names; it may not add path-purpose roles or a
post-response balancer.

Approved by:
    Repository owner after approving a small LLM target choice instead of
    purpose heuristics and clarifying that finishing the product is more
    important than optimizing one call's hypothetical token cost, 2026-08-09.
