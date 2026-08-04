# Caddy fixture — decision 094 regression

## Final local rerun (production checkpoint `f6ae3cf`)

- Fixture: `/Users/dvordrova/git/caddy`
- Fixture revision: `873fac5fc094fe538d0c477509127bb321d51a32` (clean before and after)
- Repomap revision: `f6ae3cf71800a90c3fded480923b9f7ab092f6ca`
- Mode: offline/saved responses; no provider call authorized or made
- Durable run root: `/Users/dvordrova/Library/Caches/repomap/evaluation/final-local-caddy/caddy-20260715T-final-f6ae3cf-873fac5f`

### Canonical command

`/Users/dvordrova/git/repomap/scripts/caddy_surface_check.sh /Users/dvordrova/git/caddy`

- Exit status: `1`
- Evidence: `canonical.command.txt`, `canonical.stdout.log`, `canonical.stderr.log`, and `canonical.exit_status` in the run root.
- Timing: `real 1:23.49`, `user 107.40s`, `sys 14.65s`.
- Failure: `Caddy registration mismatch`; discovered registrations were only
  `['/debug/pprof/', '/debug/pprof/cmdline', '/debug/pprof/profile',
  '/debug/pprof/symbol', '/debug/pprof/trace', '/debug/vars']`, missing the
  expected `/config/`, `/id/`, and `/stop`. The script had emitted 17 triggers
  before the oracle failed. This is a failed regression, not a relaxed or hidden pass.

### Retained offline product artifacts

The canonical script removes its temporary output on exit. Its two underlying
stages were retained separately under the same isolated run root:

- Playground command: `go run ./cmd/surface-discovery-playground --repo /Users/dvordrova/git/caddy --out "$RUN_DIR/playground"`; exit `0`; `real 50.154s`, `user 80.14s`, `sys 8.15s`; output `playground/`.
- First product capture was interrupted by the harness at 120 seconds while reconciling a captured input; its partial output is retained in `product/`, with diagnostics in `product.stderr.log` and no report entrypoint.
- Completed product command: `go run ./cmd/repomap /Users/dvordrova/git/caddy --offline --no-open --no-serve --debug-dir "$RUN_DIR/product-rerun"`; exit `0`; `real 1:33.75`, `user 100.31s`, `sys 31.81s`.
- Report entrypoint: `/Users/dvordrova/Library/Caches/repomap/evaluation/final-local-caddy/caddy-20260715T-final-f6ae3cf-873fac5f/product-rerun/20260714-204130-caddy/report.html`
- Machine report: same directory, `report.json`; manifest: same directory, `run_manifest.json`.
- Report SHA-256: `af81fa54034c5491ff038b1dcad099d8930ac73640d155b55d518a876d60c975`.
- Machine report SHA-256: `48d865396039f7829530d430ad808ab149f22589748e99717ef39ef28635fab7`.

The successful product process is still not a passing Caddy regression: it
reported 18 total surfaces (6 routes, 3 descriptors, 4 frontiers, 4 servers,
1 process entry), application 18, supporting dependency 0, and captured input
count 280, versus the canonical expected 32-trigger projection and 21/11
application/dependency split. Its stdout records `offline mode: skipping all
LLM calls`; metadata has `offline: true`, empty model/endpoint, and the run
used zero provider calls and zero request/response bytes. Cache status is
offline skip (no live or saved provider response was needed).

### Root issue

At checkpoint `f6ae3cf`, local Caddy surface discovery is incomplete at the
registration boundary: three expected admin registrations are absent, and the
offline product projection consequently emits only 18 surfaces with no
supporting-dependency classification. The canonical decision-094 oracle fails
on that first mismatch.

## Post-edge rerun (production checkpoint `bec3d3e`)

- Fixture: Caddy at `/Users/dvordrova/git/caddy`
- Revision: `873fac5fc094fe538d0c477509127bb321d51a32` (clean; verified before and after)
- Mode: saved/offline only; no live provider call authorized or made
- Durable run directory: `/Users/dvordrova/Library/Caches/repomap/evaluation/post-edge-caddy/caddy-20260714T-post-edge-873fac5f`
- Production checkpoint named by task: `bec3d3e` (no production code, tests, or fixture changes made)

### Canonical check

Command: `/Users/dvordrova/git/repomap/scripts/caddy_surface_check.sh /Users/dvordrova/git/caddy`

Exit status: `1`.

The canonical check failed at its unchanged registration oracle:
`Caddy registration mismatch: got=[]`. It did write `17 trigger(s)` before
failing, so this is a failed canonical run, not a passing run with altered
expectations. Captured evidence:

- stdout: `canonical.stdout.log`
- stderr/diagnostics and timings: `canonical.stderr.log` (`real 22.70s`, `user 47.55s`, `sys 5.01s`)
- exit status: `canonical.exit_status`
- command: `canonical.command.txt`

### Retained offline artifacts

The underlying offline stages were run once for this same fixture and retained
in the durable directory:

- Playground command exit `0`; `real 21.79s`, `user 50.32s`, `sys 5.01s`; output: `playground/`
- Product command exit `0`; `real 45.73s`, `user 59.57s`, `sys 15.18s`; output: `product/20260714-165101-caddy/`
- Product report entrypoint: `/Users/dvordrova/Library/Caches/repomap/evaluation/post-edge-caddy/caddy-20260714T-post-edge-873fac5f/product/20260714-165101-caddy/report.html`
- Machine report: same directory, `report.json`
- Manifest: same directory, `run_manifest.json`
- Report SHA-256: `c493b563ed9ab87b7f9c946fd4992951fe69aad94e21392679763446cb532842`
- Manifest-bound `report.json` SHA-256: `0829777da6d461b0b73f5bc3cec531e75a82d77cfaed140457627722859b9683`

The product command emitted `offline mode: skipping all LLM calls`, with
provider calls `0`, request/response bytes `0`, model and endpoint empty, and
`offline: true` in `metadata.json`. Its generated report is retained but is
partial/invalid for the Caddy oracle: `discovered_surfaces.total_count` and
all projected surface counts are `0`, with `captured_input_count: 280`.
This product-stage weakness is not hidden behind the successful process exit.

- Fixture: Caddy at `/Users/dvordrova/git/caddy`
- Revision: `873fac5fc094fe538d0c477509127bb321d51a32` (clean at setup)
- Mode: saved/offline only; no live provider call authorized or made
- Durable run directory: `/Users/dvordrova/Library/Caches/repomap/evaluation/initial-caddy/caddy-20260714T000000Z-873fac5f`

## Canonical check

Command: `./scripts/caddy_surface_check.sh /Users/dvordrova/git/caddy`

The fixture stages printed both required `OK:` lines. The canonical subprocess completed
successfully (the captured diagnostic output contains no failure). The surrounding first
capture wrapper itself returned nonzero after the subprocess because zsh rejected its
attempt to assign the readonly variable `status`; this wrapper error did not affect the
fixture subprocess. Evidence is retained in:

- `canonical.stdout.log` — `/Users/dvordrova/Library/Caches/repomap/evaluation/initial-caddy/caddy-20260714T000000Z-873fac5f/canonical.stdout.log`
- `canonical.stderr.log` — same run directory
- Canonical timings: `real 71.46s`, `user 105.77s`, `sys 22.54s`

The canonical script's temporary output was intentionally removed by the script's own
trap. Durable equivalent offline artifacts were generated below without changing the
script or production code.

## Durable offline artifacts

Commands:

```sh
go run ./cmd/surface-discovery-playground --repo /Users/dvordrova/git/caddy --out "$RUN_DIR/playground"
go run ./cmd/repomap /Users/dvordrova/git/caddy --offline --no-open --no-serve --debug-dir "$RUN_DIR/product"
```

Stage status: playground `0`; product `0`.

- Playground timing: recorded in `playground.stderr.log` (the Go command itself emitted no timing line; output creation succeeded with 32 triggers).
- Product timings: `real 49.57s`, `user 64.17s`, `sys 16.98s`.
- Report entrypoint: `/Users/dvordrova/Library/Caches/repomap/evaluation/initial-caddy/caddy-20260714T000000Z-873fac5f/product/20260714-162702-caddy/report.html`
- Machine report: same directory, `report.json`
- Run manifest: same directory, `run_manifest.json`
- Evidence: `trigger_catalog.json`, `surface_coverage.json`, `architecture_grounding.json`, `semantic_summaries.json`, `snapshot.json`, `llm_bundle.json`, `metadata.json`, `surface_summary.md`, and `onboarding-feedback.md`

The durable report has surface counts: process entry 1, HTTP routes 19, route
descriptors 5, route frontiers 2, HTTP servers 6; application 22, supporting dependency
11, unassigned 0. The canonical validation also passed the expected 32 trigger,
21-application/11-dependency, and 13-anchor assertions.

## Provider/cache facts and diagnostics

- Provider calls: `0` live calls.
- Provider bytes: `0` request/response bytes.
- Cache status: offline path; all LLM calls skipped. Captured stdout explicitly says
  `offline mode: skipping all LLM calls`.
- Manifest confirms `offline: true`, empty model and endpoint, and repository head above.
- Product report SHA-256: `a9a6467800ee16dd4729c43b3727bae6f0f51796432df442a7ff9e7373533021`.
- Manifest-bound report SHA-256 (`report.json`):
  `a0b0277d64070b0dbdcac65ac91778ffa677d398f126f02eb531df1931e59780`.

No failure or partial product run is hidden; the wrapper diagnostic is preserved above and
the durable product stages both exited `0`.
