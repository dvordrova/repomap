# 279 — Ordinary target portfolio candidate surface

**Status:** ACTIVE (owner-authorized product-scope corrective, 2026-08-10)

**Preserves:** Decisions 262, 269, 274, 276 and 277 except that this decision
supersedes D274's complete-package ordinary request surface; the complete exact local
`TargetCatalog`; explicit `--target`; exhaustive `--all-targets` inclusion;
one exact Go build scenario; names-only declaration evidence; and refs-only
provider restoration.

## Product defect

The complete package catalog was also used as the ordinary portfolio selector
surface. In a library such as Telebot this advertised the module-root API plus
the public `layout`, `middleware` and `react` implementation packages. A model
could therefore create a sibling report page and a separate Architecture and
Study run for every public subpackage even though ordinary navigation needs
product surfaces rather than package coverage.

Package paths do not prove product purpose, so a local `internal`, `pkg`,
`example` or other name classifier remains forbidden. The exact module
boundary and exact executable shape are already deterministic facts.

## Approved ordinary surface

The Decision 262 `TargetCatalog` remains complete, sealed and unchanged. The
ordinary target-portfolio compiler advertises, in canonical catalog order,
only:

- every exact executable package; and
- an exact library package whose `Target.PackageDir` equals its
  `Target.ModuleDir`.

The second condition is the literal package-at-module-root relationship. It is
not a path-purpose role or a provider judgment. Public non-root libraries stay
in the private catalog and remain available to explicit `--target` and
exhaustive `--all-targets`; they do not become ordinary sibling pages.

Request-local refs are dense over only this advertised surface. The compilation
keeps the complete private catalog ref in its request binding but its restoration
table contains only advertised entries, so a response cannot revive a hidden
subpackage. There is no target or symbol truncation within the surface. Decision
276 still makes a module-root library with an empty exported declaration
inventory ineligible for selection even when it appears as `symbols:[]` in the
wire.

Ordinary fallback applies the same advertised-surface rule plus Decision 276's
non-empty-library rule. When exactly one eligible advertised target exists, the
runtime selects it locally and configures no portfolio provider; thus Telebot's
four-package catalog produces one root-library page and zero selector calls.
When several eligible candidates exist, the existing one-call refs-only
selector may choose among only those product candidates. `--all-targets` still
includes every exact catalog entry; an explicit `--target` remains authoritative.

Compilation and request identities advance v2→v3 and the prompt hash advances
with its exact prefiltered-surface wording. Result v3 remains valid because the
v3 request identity and private request ref already bind the new mapping; prior
request bytes and refs miss closed. No report, manifest, target-navigation,
Architecture or Study identity changes.

## Acceptance

- Telebot-shaped root + `layout` + `middleware` + `react` facts retain four
  complete catalog entries but advertise and select only the root library, with
  zero provider attempts.
- etcd-shaped client-module root + server `main` + public subpackages retains
  the complete catalog but advertises only the client root and server main.
- request-local refs are dense over that surface and responses citing an
  unadvertised ref fail closed.
- explicit exhaustive inclusion still returns every exact catalog target.

Approved by:
    Repository owner after observing that a library repository could otherwise
    publish and run Study for every public package, 2026-08-10.
