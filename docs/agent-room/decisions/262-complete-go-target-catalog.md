# 262 — Complete Go target catalog

**Status:** APPROVED FOUNDATION (owner-authorized, 2026-08-09)

**Preserves:** Decisions 250, 257, 260 and 261; one exact Go build scenario;
the existing conservative local default; composite module/package target
identity; and explicit fail-closed target overrides.

## Product defect

Decision 257 resolves and then destructively scopes one package target. That
is sufficient for one report, but it cannot support the owner-approved product
shape in which one repository contains several executable and library pages
and the user switches between them in one report.

Purpose is not a local package fact. Package names such as `internal`,
`fixture`, `generated`, `tools`, or `testdata` do not prove whether a target is
the product, a helper, or an independently useful library. Encoding those
guesses as authoritative catalog roles would contaminate every later semantic
call and hide exact selectable packages.

## Approved catalog contract

`internal/analysistarget` builds one provider-free `TargetCatalog` from the
complete exact Go facts before any target scope is discarded. The catalog is
versioned, canonical, permutation-stable and self-sealed. It contains every
existing exact `Candidate`; it does not add a role, score, priority, quota,
balance rule or provider result.

Each entry carries:

- the existing exact `Candidate`, whose `Target` owns the composite module ID,
  module path, package path, target kind and exact executable roots; and
- one display-only repository-relative package directory.

Display is derived from the exact module directory plus the package's exact
module-relative directory and must equal the target's repository-relative
package directory. It is never used as canonical identity or as a model-owned
join.

`DefaultTargetRef` is exactly the result of the existing Decision 257
`autoSelect` policy over the complete catalog. Building the catalog therefore
does not change the ordinary one-target behavior: the repomap-shaped unique
module-name executable remains `cmd/repomap`, a sole root library remains the
default, and an ambiguous repository remains without a default.

The catalog itself makes no provider call and is not yet a multi-target run,
report, UI selector, cache or manifest contract. A separately approved
refs-only semantic selector may consume the complete catalog, while explicit
`--target` continues to bypass semantic choice and a future literal
`--all-targets` may consume every exact catalog entry.

## Provider-free acceptance

Permanent tests prove:

1. a repomap-shaped catalog retains executables, root/public/internal
   libraries and a nested module while its default exactly matches `Resolve`;
2. root-module, `pkg` and nested-module libraries have distinct composite
   identities and flat repository-relative display paths;
3. ambiguous and unavailable resolutions produce no catalog default;
4. package/module/entrypoint permutation produces identical canonical bytes
   and catalog ref; and
5. target, display, key, order, default and seal tampering fail closed.

Approved by:
    Repository owner after requesting `--all-targets`, one target switcher for
    executables and libraries in a shared report, and explicitly rejecting
    purpose guesses in favor of a small later LLM choice, 2026-08-09.
