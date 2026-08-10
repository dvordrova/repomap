# 257 — One analysis target package

**Status:** APPROVED FOUNDATION (owner-authorized, 2026-08-09)
**Preserves:** Decisions 246 and 256; exact local Atlas, Surface and direct-call
authority; one bounded Architecture and Study interpretation per selected
product; existing provider request limits; and fail-closed local validation.

## Product defect

A repository can contain several executable packages, nested fixture modules
and library packages. The current repository-wide pipeline can therefore let
an auxiliary binary or fixture consume an Architecture, Study or mechanism
budget before the product the user intended to study. Raising a cap does not
fix that defect: it only spends more work on an unselected product.

The selected product must be one package and one identity shared by the whole
analysis. Architecture, Study and mechanism depth must not independently guess
their own roots.

## Approved target contract

One run has one `AnalysisTarget` package:

- an **executable package** is rooted only in the exact build-selected
  top-level `func main()` declarations belonging to that selected package; or
- a **library package** is rooted in the exact exported public API boundary of
  that selected package. A library target never receives a synthetic `main`.

The target identity includes its kind, owning module, canonical package,
repository-relative package directory, root-boundary kind and exact executable
roots when present. It is bound to the repository revision/freshness and Go
build scenario by every consuming run artifact and cache identity.

`--target` is the explicit override. It selects one advertised target package,
not an arbitrary file prefix. A future CLI may accept an exact candidate key,
canonical package path or an unambiguous repository-relative package directory;
the resolver remains authoritative and must reject an unknown or ambiguous
override.

Without an override, deterministic local facts may auto-select only one strong
candidate: for example one uniquely designated primary executable, one unique
executable matching the main module name, one sole plausible executable, or a
sole root library package when no plausible executable competes with it. If
more than one plausible product remains, resolution is `ambiguous`, no target
is selected, and no provider stage starts.

For automatic selection, the main module is the exact module at repository
relative directory `.`. Nested fixture/example modules remain valid explicit
targets, but a separate `go list` invocation marking each nested module as
`Main` cannot make those modules compete with the ordinary root product.

## Scope order

Target admission happens before every semantic or cardinality cap:

1. resolve and bind one target;
2. derive its production package/import scope and exact target-root evidence;
3. build Architecture candidates from that scope;
4. build Study candidates and source readings from that scope; and
5. admit mechanism roots reachable from the selected executable roots, or from
   the selected library API boundary, before applying mechanism card limits.

Architecture and Study are recomputed for the target. A repository-wide model
response is never made target-specific by filtering its finished prose or
grouping. Mechanism compilation independently verifies the same target binding
so an old or mixed Study artifact fails closed.

Omitted target items and rejected off-target items remain separate local
accounting. A limit must never hide that off-target candidates displaced a
selected product item.

## Foundation in this decision

This decision adds a provider-free `internal/analysistarget` model, resolver
and exact Go-facts projection. Snapshot resolves the target immediately after
local Go facts, serializes the selected target, and replaces repository-wide Go
facts with the selected package/import scope before Orient or Atlas can consume
semantic budgets. Orient accepts an internal override and exposes an owned
live target handoff; debug metadata binds the selected identity.

The foundation deliberately does not wire a public CLI flag, run manifest,
caches, Architecture, Study, mechanism runtime, report or templates. Those
consumers must use the same selected snapshot target in their owning changes.
Decision 256 retains ownership of its current presentation work, and
`CURRENT.md` is not changed here.

## Provider-free acceptance

Synthetic Go facts prove:

- a repomap-like root module with several commands selects the package whose
  executable basename uniquely matches the main module, even when many nested
  fixture modules are independently marked `Main`; an explicit override can
  still select another advertised command or nested module;
- a Moby-like multi-command module remains ambiguous without a locally exact
  primary signal, while an explicit override selects `cmd/dockerd`;
- a Telebot-like module with no executable selects its sole root library
  package and records an exact-public-API boundary without inventing roots; and
- multiple plausible products with no strong discriminator remain ambiguous.

Approved by:
    Repository owner after inspecting fresh Telebot and repomap reports and the
    exact mechanism inputs, 2026-08-09: one selected package must scope the
    entire analysis before semantic calls and caps; executable and library
    products share one target contract.
