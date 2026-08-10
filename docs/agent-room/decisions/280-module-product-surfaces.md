# 280 — Go module product surfaces

**Status:** ACTIVE (owner-authorized product model, 2026-08-10)

**Supersedes:** Decision 257's package-library product identity, Decision 262's
one-target-per-package catalog, and Decision 279's temporary filtering of that
package catalog to a package at the module root. It preserves exact executable
packages, one resolved Go build scenario, local declaration authority,
fail-closed refs-only restoration, explicit exact target keys and separately
scoped sibling runs.

## Product defect

A Go package is an import/compiler boundary, not necessarily a product. Treating
every package as an AnalysisTarget made a Moby `daemon` subtree, or Telebot's
`layout`, `middleware` and `react` packages, candidates for independent
Architecture and Study runs. Filtering that list to a package located beside
`go.mod` avoided some duplicate pages but erased a module's public library when
the module root was `package main` or contained no Go package at all.

The product boundaries already available without guessing purpose are the
`go.mod` module and exact build-selected `main` packages.

## Approved target authority

`AnalysisTarget` v2 adds `module_library` without fabricating a package at the
module root. Its package fields are empty. Its module identity binds two exact,
canonical inventories:

- `ModulePackages`: every local non-`main` package owned by that exact module,
  including `internal` and other supporting packages; and
- `LibraryPackages`: the externally importable non-`main` subset that has a
  complete package load, a complete declaration scan and at least one exported
  declaration.

The target ref binds both inventories. The first owns analysis scope; the second
owns public API roots. A module-library snapshot therefore cannot silently grow,
drop or exchange a package after selection. Exact executable targets remain one
build-selected `main` package with exact `func main` locators.

`TargetCatalog` v4 contains only:

- every exact executable package; and
- at most one `module_library` target per exact module.

Each module-library entry carries canonical package-qualified `PackageAPIs`.
Declarations are exported, names-only and grouped by exact package identity;
source locations stay in private Go facts. A package whose module-relative path
contains the exact Go `internal` segment remains in module scope but never in
the external API root set. A `package main` never enters either module-library
inventory.

Completeness fails closed at the target item. A truncated package inventory or
an incomplete declaration scan for any externally importable non-main package
omits that module's aggregate library target, while independently exact
executable targets remain usable. An incomplete internal-only declaration scan
does not block the external API. A fully scanned module with zero exported
externally importable declarations has no library product and likewise emits no
module-library target. Existing exact Go-facts warnings retain the local reason;
absence is never presented as a proven empty API.

The exact `go list -e` outcome is persisted as versioned, typed package-local
load completeness. `Incomplete`, package `Error`, or any `DepsErrors` make an
externally importable non-main package incomplete and omit its module-library
target even when its source declaration scan succeeded. Missing or unknown
load-completeness authority also fails closed. The same state on an
`internal`-only package remains bounded analysis context and does not block the
public library product; warning text is never parsed back into authority.

## Selection and scope

Executable and module-library keys have disjoint canonical spellings. An exact
key or self-sealed ref selects exactly one target. Short aliases remain accepted
only when unambiguous: when a root `main` import path equals its module path, the
shared module/package alias is rejected rather than silently preferring either
surface. The ordinary `--target` ambiguity error enumerates every matching exact
key so the user can complete that selection without knowing a private key
spelling in advance.

Scoping a module library retains exactly its sealed owning-module non-main
packages and their internal edges, and removes every main package, process entry,
command trace and executable orientation candidate. Architecture may use the
whole retained module as context; Study and target-rooted call analysis start
from the ordered `LibraryPackages` public API roots. `--all-targets` now means all
product surfaces in catalog v4, not all compiler packages. Exact packages remain
local evidence and explicit drill-down material, not sibling product pages.

Because the selected-target container now embeds Target v2 and a module-level
display path, its identity advances to v2 and its artifact name to
`target_run_container.v2.json`; v1 bytes fail closed rather than being
reinterpreted.

## Provider and privacy boundary

This target/catalog correction is deterministic and provider-free. The existing
portfolio call may see package-qualified names-only API groups under request-local
refs; it never receives source, full files, raw package edges, canonical Atlas
IDs or repository-wide package pages. No provider may add a package or split one
module library into extra targets.

## Acceptance

- a root `main` plus public subpackage yields one executable and one module
  library, with collision-safe explicit selection;
- nested `go.mod` modules receive separate module identities and at most one
  library each;
- internal and main packages never become public API roots, while internal
  non-main packages remain in the sealed module analysis scope;
- incomplete external scans, truncated module inventories and complete
  zero-export modules omit only the unprovable module-library target;
- target/catalog/package-API permutations produce identical canonical bytes,
  and target inventory, API grouping, order or seal tampering fails closed; and
- scoping from a deferred catalog retains exactly the selected module's
  non-main files plus its `go.mod` and shared root documentation.

Approved by:
    Repository owner after observing that `daemon/*` compiler packages should
    be one Moby module-library surface and specifying the product split as
    `go.mod`, then exact `main` executables plus all exported library API at that
    module level, 2026-08-10.
