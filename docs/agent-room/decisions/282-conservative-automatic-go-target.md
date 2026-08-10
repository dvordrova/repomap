# 282 — Conservative automatic Go target selection

**Status:** ACTIVE (owner-authorized product corrective, 2026-08-10)

**Preserves:** D250's one atomic Go build scenario and explicit override;
D251's bounded tracked-file evidence, exclusions, unique-leader threshold and
single inventory pass; D269's sibling-page scenario equality; all provider,
cache, report and manifest authority. This decision supersedes only D251's
statement that a qualifying advisory never changes the target.

## Product defect

A default macOS Moby run already has strong exact D251 evidence for Linux, but
still builds the Darwin package universe. `cmd/dockerd` then fails because its
daemon closure intentionally has Linux/Windows configuration fields and no
Darwin implementation. The later warning cannot recover omitted Go facts or
the already-built target catalog.

## Approved contract

- When neither `--go-target` nor either existing nonempty `GOOS`/`GOARCH`
  environment input is present, the ordinary command may apply the existing
  D251 advisory.
- Selection is allowed only for D251's unique strong production GOOS leader;
  weak, tied, custom-tag, excluded-path or over-budget evidence leaves the
  resolved host target unchanged.
- Selection happens after the one safe tracked-file inventory and bounded
  prefix preflight, but before Go facts, target catalog, SSA, cache identity or
  any provider call. Go facts and the catalog are built once, for the final
  scenario. There is no failed-scenario retry.
- The selected GOOS retains the resolved host GOARCH. Explicit `--go-target`
  and existing environment target inputs always win and never enter this lane.
- Exact live provenance is copied through target-run projections. Metadata
  persists the final `go_target`, source `auto`, and exact baseline target; the
  console derives `auto: linux/amd64 (host darwin)` from those typed fields.
  Every sibling and recovery run must agree on all three fields.
- Provider inputs, caches, RepositoryContext, Surface scenario and later
  semantic stages see only the final selected scenario. Advisory evidence
  paths remain local and are never added to a provider request.
- This adds no per-page platform mixing, build-script execution, container,
  model call, semantic retry, synthetic source overlay, report field or UI.

## Acceptance

1. A Darwin-baseline Moby-shaped fixture with a unique Linux advisory builds
   only Linux facts/catalog, exposes `cmd/dockerd`, and reaches the first
   provider seam once with only that final catalog.
2. Explicit Darwin and either Go environment target input disable automatic
   selection and retain fail-closed Darwin behavior.
3. Weak and tied evidence retain the baseline target without a provider retry.
4. Offline runs add no provider accounting; metadata and console retain exact
   automatic provenance.
5. Sibling publication and recovery reject target/provenance drift.

Approved by:
    Repository owner after the Moby `cmd/dockerd` failure review and selection
    of the bounded preflight fix, 2026-08-10.
