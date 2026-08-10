# 269 — Sibling-page multi-target publication

**Status:** ACTIVE (owner-authorized implementation, 2026-08-10)

**Preserves:** Decisions 257, 261, 262 and 263; one exact Go build scenario;
the refs-only selector's single call and exact selected set; ordinary
single-target/explicit-target fallback; one canonical `ReportData` per target;
repository freshness and source authority; existing provider contracts and
per-target accounting; the current report server and `latest` behavior.

## Product defect

Decision 263 already selects a useful target set and Decision 262 retains the
exact local facts needed to restore it, but the ordinary product publishes only
the selected default. The report therefore presents one apparently global
Architecture and Study even when the repository contains several selected
executables, libraries, modules, or monorepo products. A selector control over
one shared report would falsely imply that those independently derived products
share one semantic and manifest authority.

## Approved publication model

The selector runs exactly once. The chosen default continues through the
existing ordinary target pipeline. Each additional selected ref is restored
only through `TargetRunContainer.ScopedSnapshot(ref)` and runs the same ordinary
target pipeline into its own run directory, including its own local SSA,
semantic provider calls, semantic artifacts, report, repository freshness and
run manifest. No old run is copied, no target shares semantic results, and no
plural `ReportData` schema or new server endpoint is introduced.

The console encloses every ordinary target pipeline with its exact display
path, package path, run ID and default/sibling role. Repeated Architecture,
Study and warning blocks therefore remain attributable without changing their
stage wording or accounting.

After every selected target reaches a terminal state, the backend creates one
canonical `target_page_portfolio.v1.json`. It contains the exact container
self-seal and catalog ref plus one canonical-order outcome per selected target.
A ready target contains only its safe sibling run ID; an unavailable target
contains only the closed code `target_run_failed`. It contains no error prose,
display path, report digest or manifest digest, avoiding both leakage and a
report/manifest hash cycle. The identical canonical container and portfolio
bytes are copied into every ready target directory. Each target's Manifest 17
binds the exact artifact-byte digests in addition to its own exact analysis
target and report.

Every ready page then receives one provider-free authorized final render. Its
render-only target directory is derived from the sealed container and portfolio,
never persisted in `report.json`: module `go.mod` groups use exact `module_dir`,
the default has a green marker, the current target has blue `aria-current`, an
unavailable target is a disabled non-link, and a ready sibling is a normal
relative `../RUN_ID/report.html#/map` link. The current link is `#/map`.
Because these are ordinary anchors with no navigation interception, both
`file://` and the existing local server resolve them and browser Back preserves
the prior page's deep fragment.

Before publication, the backend reopens every ready manifest, verifies its own
container and portfolio bytes, and proves that exactly one manifest authority
exists for every ready target with the matching target ref and run ID. Missing,
tampered, duplicated or cross-bound siblings fail closed.

Finalization reopens existing run directories through the confined
existing-run writer; it never recreates a run directory. A fully verified
portfolio is an idempotent no-op. If all ordinary pages completed but final
portfolio binding was interrupted, `repomap dev finalize-target-pages` accepts
the exact default and sibling run directories, re-confirms saved input
authority, and performs only the provider-free bind/render/verify phase. It
never guesses siblings from nearby cache directories or repeats a provider
call.

## Failure and compatibility boundary

Default-target failure remains terminal. An additional target's ordinary
pipeline failure is isolated as the closed unavailable outcome and its partial
run directory is never a navigable sibling authority. Artifact writing, final
authorized rendering, manifest verification, or cross-sibling integrity failure
is terminal for the whole publication. `latest`, the initially served run and
the initially opened file remain the selected default only.

A one-target selection retains the existing single-page behavior. The literal
`--all-targets` controls inclusion completeness, not default selection. Online
without `--target`, the ordinary portfolio selector still runs exactly once and
its selected default opens first, while the container includes the complete
catalog in canonical order. With explicit `--target`, that exact advertised
package becomes the default and no portfolio selector call is made. Offline
uses the exact catalog default, or requires an explicit `--target` when the
catalog has no strong default. No branch heuristically filters or reorders the
complete catalog.

Identities advance to Report 45, Manifest 17, target navigation v1 and target
page portfolio v1. Existing semantic prompts, responses, caches and artifacts
do not change.

## Acceptance

Provider-free tests prove selector-once online/default handoff, selector bypass
for an explicit default, offline exact-default failure, one ordinary runner invocation
per additional selected ref, exact scoped target authority, canonical terminal
outcomes, safe run IDs, byte-identical artifacts, manifest binding and
tamper/missing-sibling rejection, existing-directory recovery and byte-identical
idempotence, two-target console attribution, current/default source authority, module
grouping, disabled unavailable targets, ordinary file/HTTP sibling links, and
native deep-link/Back behavior. Focused package tests, changed-package vet,
build and diff checks precede any owner-gated provider run.

Approved by:
    Repository owner after requesting a left-side menu of selected defaults and
    explicitly preferring separate ordinary provider runs over copying or
    over-optimizing one shared run, 2026-08-10.
