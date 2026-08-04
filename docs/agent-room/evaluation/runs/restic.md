# Restic fixture run

## Identity

- Fixture: `restic`
- Decision: `094-syncthing-surface-trace-product-fixture.md`
- Source: `/Users/dvordrova/git/restic`
- Revision checked: `987caba4089fc4345bb201e62c5a2ba96b168049`
- Source worktree status: clean (`git status --short` was empty).

## Authoritative run

- Isolated durable run directory: `/Users/dvordrova/Library/Caches/repomap/evaluation/initial-restic/20260714T203500-offline`
- Command:
  `go run ./cmd/repomap /Users/dvordrova/git/restic --offline --no-open --no-serve --debug-dir /Users/dvordrova/Library/Caches/repomap/evaluation/initial-restic/20260714T203500-offline/runs`
- Exit status: `0` (recorded in `exit.status`).
- Generated run: `runs/20260714-162756-restic`
- Report entrypoints:
  - HTML: `runs/20260714-162756-restic/report.html`
  - JSON: `runs/20260714-162756-restic/report.json`
- Manifest: `runs/20260714-162756-restic/run_manifest.json`

## Timings and facts

- Wall/user/system: `72.70s / 107.79s / 19.07s` (captured in `canonical.stderr.log`).
- Snapshot: 1,344 tracked files; compact context 100,345 bytes across 250 candidates.
- Go surface discovery: 28 surfaces, 35,854 ms.
- Reconciliation: 357 captured inputs, 34,778 ms.
- Report generation: 53 ms.
- Provider calls: **0**. `--offline` was enabled; stdout records that all LLM calls were skipped.
- Provider bytes/cache: no provider request/response and no provider cache used; local deterministic artifacts were generated. Metadata has empty model and endpoint.
- Report warning: orientation report is absent because offline mode skipped orientation; 0 directions expanded.

## Saved-report availability

The local test metadata requires `REPOMAP_SAVED_RESTIC_RUN` for the owner-provided
saved-report replay (`go test ./internal/report -run 'Test(Rest.*|SavedResticCoherence)' -count=1`).
No such path was present in local metadata/cache, so it was not guessed and no
live provider call was made. The offline report above is the available audit
artifact. The checked-in saved provider response is
`internal/modelresearch/testdata/restic_backup_response.json`, but it is used by
unit coverage rather than providing the owner run directory required by
`TestSavedResticCoherence`.

## Capture caveat

An initial identical run was written to
`.../initial-restic/20260714T203000-offline` and produced a report, but its shell
wrapper failed after `go run` completed because `status` is a read-only zsh
variable. It is retained as failure evidence; the authoritative rerun above
used `rc`, recorded exit status `0`, and is complete.

## Post-checkpoint rerun

- Production checkpoint: `bec3d3e8dcc57bb61792ba9194b7a0623a3c317b5` (the
  repomap checkout was at this revision; unrelated existing worktree changes
  were not modified).
- Isolated durable run directory:
  `/Users/dvordrova/Library/Caches/repomap/evaluation/post-edge-restic`
- Command:
  `go run ./cmd/repomap /Users/dvordrova/git/restic --offline --no-open --no-serve --debug-dir /Users/dvordrova/Library/Caches/repomap/evaluation/post-edge-restic/runs`
- Exit status: `0` (`exit.status`).
- Generated run: `runs/20260714-164750-restic`
- Report entrypoints:
  - HTML: `runs/20260714-164750-restic/report.html`
  - JSON: `runs/20260714-164750-restic/report.json`
  - Manifest: `runs/20260714-164750-restic/run_manifest.json`
- Source identity: `/Users/dvordrova/git/restic` at
  `987caba4089fc4345bb201e62c5a2ba96b168049`; source worktree remained clean.
- Timings: real `86.71s`, user `116.86s`, system `21.55s`; snapshot `1,344`
  files / compact context `100,345` bytes; surface discovery `43,609 ms`
  (11 surfaces); reconciliation `38,645 ms`; report generation `74 ms`.
- Provider calls: **0**; offline mode skipped all LLM calls. Provider request
  and response bytes: `0`; provider cache: unused. No live provider call was
  made.

### Helper ownership result

The false ownership is eliminated for the primary Restic application: generated
surface records identify `cmd/restic` as `primary_application`, while the two
helper executables are separately identified as `tooling`. The evidence also
records that helper entrypoints were not admitted as call targets for
`cmd/restic` (see the unresolved call-target diagnostics in
`surface_coverage.json`). Helpers remain as their own exact static tooling
entries; this is not evidence that they disappeared, and expectations were not
changed to hide them.

### Diagnostics and failure evidence

- `report.json` warns that offline orientation is absent
  (`orientation_report.json` was not created), so there are no model-derived
  directions; this is expected for the offline rerun.
- `report.json` warns that `trigger_catalog.json` is version 6 while the report
  reader requested version 5. The run still exited 0; this compatibility warning
  is retained and is not treated as a warning-free report.
- Full stdout, stderr/timings, and exit evidence are retained as
  `canonical.stdout.log`, `canonical.stderr.log`, and `exit.status` in the
  post-checkpoint run directory.

## Final-local checkpoint run

- Fixture: `restic`; decision: `094-syncthing-surface-trace-product-fixture.md`.
- Repomap checkpoint: `f6ae3cf71800a90c3fded480923b9f7ab092f6ca`.
- Source: `/Users/dvordrova/git/restic` at
  `987caba4089fc4345bb201e62c5a2ba96b168049`; source status was clean.
- Durable output root: `/Users/dvordrova/Library/Caches/repomap/evaluation/final-local-restic`.

### Authoritative available report (partial/local fallback)

- Run directory:
  `/Users/dvordrova/Library/Caches/repomap/evaluation/final-local-restic/20260715T-run-f6ae3cf-987caba4-no-surface`
- Command: compiled local `repomap` binary with
  `/Users/dvordrova/git/restic --offline --discover-surfaces=false --no-open --no-serve`
  and that run's `runs` debug directory.
- Exit status: `0`, in `exit.status`.
- Report entrypoints:
  - HTML: `runs/20260714-210221-restic/report.html`
  - JSON: `runs/20260714-210221-restic/report.json`
  - Manifest: `runs/20260714-210221-restic/run_manifest.json`
- Timings: real `74.92s`, user `39.98s`, system `29.05s`; snapshot `1,344`
  files and compact context `100,345` bytes across `250` candidates;
  reconciliation `70,084 ms`; report generation `71 ms`.
- Provider calls: **0**; offline mode was explicit. Provider request/response
  bytes: `0`; provider cache: unused; metadata has empty model and endpoint.
- Diagnostics: report warning says orientation is absent because offline mode
  skipped it; no provider-derived directions or flows are present. The
  `discover-surfaces=false` setting means this is a partial/local report, not
  a full surface-discovery regression result.

### Full surface-discovery attempts (retained failure evidence)

The exact default command (`--discover-surfaces` enabled) was attempted in
three separate durable directories. Each reached local Go surface discovery,
then exceeded the command-tool timeout and was canceled before report
generation; no exit status was produced by the interrupted shell:

- `.../20260715T-run-f6ae3cf-987caba4` — 180-second timeout;
  `canonical.stderr.log` ends at 2m50s.
- `.../20260715T-run-f6ae3cf-987caba4-retry1` — 600-second timeout;
  ends at 9m50s.
- `.../20260715T-run-f6ae3cf-987caba4-binary` — compiled binary, 600-second
  timeout; ends with `repomap: canceled` at 9m40s.

These directories are intentionally retained and the full run is not reported
as successful. No live provider call was made in any attempt.
