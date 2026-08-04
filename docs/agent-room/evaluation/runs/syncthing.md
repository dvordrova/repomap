# Syncthing focused producer-semantics verification

## Fixture and revision

- Fixture: `syncthing` (exactly one fixture)
- Fixture repository: `/Users/dvordrova/git/syncthing`
- Fixture revision: `d4cffd848eb13d65f3caca5ef6da9a3fd25a2d6a`
- Fixture worktree: clean (`## main...origin/main`)
- repomap checkpoint: `21fe3b956134233925fc47cf4ab139444b7d616a`
- Durable isolated run directory: `/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing`
- No production code, tests, or fixture repository files were edited.

## Offline product run

Command:

```text
go run ./cmd/repomap /Users/dvordrova/git/syncthing --offline --no-open --no-serve --debug-dir /Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/product/runs
```

- Exit status: `0` (`product.exit_status`)
- Command record: `product.command.txt`
- Complete output and timings: `product-run.log`
- Stage timings: repository facts `3943 ms`; compact context `89791` bytes across `250` candidates in `8 ms`; Go surface discovery `32204 ms`; reconciliation of `402` inputs `57945 ms`; report generation `94 ms`.
- `/usr/bin/time -p`: wall `102.87s`, user `116.95s`, system `39.36s`.
- Discovered surfaces: `45` (the canonical script expects `36`); the product run is therefore useful but not acceptance-passing.

## Report and semantic verification

- Report run directory: `/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/product/runs/20260714-211633-syncthing`
- Report entrypoint: `/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/product/runs/20260714-211633-syncthing/report.html`
- Machine report: `/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/product/runs/20260714-211633-syncthing/report.json`
- Manifest: `/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/product/runs/20260714-211633-syncthing/run_manifest.json`
- Metadata: `/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/product/runs/20260714-211633-syncthing/metadata.json`
- Surface diagnostics: `/Users/dvordrova/Library/Caches/repomap/evaluation/producer-semantics-syncthing/20260715T011627-21fe3b9-syncthing/product/runs/20260714-211633-syncthing/surface_summary.md`
- Report SHA-256: `90a34e3846a28c4543010a35dce969a46c814802e019e5ef31860e15a18462a3`
- HTML SHA-256: `a47115341851088bbed3ec00fa75f4c1f4a979369f47ec6c03ed10acb07bd80d`
- Manifest binds the report to fixture HEAD `d4cffd848eb13d65f3caca5ef6da9a3fd25a2d6a`.

The primary process ID was checked in **both** `report.json` and
`trigger_catalog.json` (not catalog-only):

- ID: `trigger-1866c3e977616ca13632e2ab`
- Entrypoint: `cmd/syncthing/main.go:212` (`github.com/syncthing/syncthing/cmd/syncthing.main`)
- Role/surface: `primary_application` / `entry_surface`
- Report JSON `trace_readiness`: `partial_trace_ready`
- Reason: `exact process entry can seed a one-anchor partial trace; typed downstream closure is unavailable`

Semantic result: **PASS — primary process retains partial-ready semantics in report JSON.**

## Provider accounting

- Offline mode was enabled (`metadata.json` `effective_options.offline: true`).
- Provider calls: `0`; no live provider call was made.
- Provider request/response bytes: `0` / `0`; no provider cache was read or written.

## Canonical script outcome (separate)

Command:

```text
./scripts/syncthing_surface_check.sh /Users/dvordrova/git/syncthing
```

- Exit status: `1` (`canonical.exit_status`)
- Command record: `canonical.command.txt`
- Output/timings: `canonical-run.log`
- Failure evidence: `Syncthing surface multiplicity mismatch: 45 records`
- Canonical subprocess stage timings: repository facts `5818 ms`; compact context `89791` bytes in `20 ms`; Go surface discovery `47471 ms`; reconciliation `58298 ms`; report `62 ms`; `/usr/bin/time -p` wall `119.26s`, user `134.44s`, system `43.46s`.
- The script's temporary report was removed by its own trap; its failure evidence is retained above. This does not change the durable product report or the semantic result.
