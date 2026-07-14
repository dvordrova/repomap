# Final-local performance audit

Date: 2026-07-15  
Repomap checkpoint: `f6ae3cf71800a90c3fded480923b9f7ab092f6ca`  
Fixtures: Syncthing, Caddy, Restic

## Comparison validity

These are single local captures, not controlled benchmarks. The fixture revisions
and tracked-file counts are stable within each comparison. A timing improvement is
not accepted when the resulting report is invalid or when the command modes differ.

| Fixture | final-local result | Earlier comparison |
|---|---|---|
| Syncthing | product exited 0, but canonical check failed at 45 records | Earlier accepted/usable report comparison is invalid: final report is blocked by catalog v6 versus reader v5 and canonical expected 36 |
| Caddy | product exited 0, canonical check failed; report projected zero surfaces | Earlier initial report passed; final comparison is invalid because final output is empty and registration oracle fails |
| Restic | full default surface run did not complete; only `--discover-surfaces=false` fallback completed | No valid final-local full-surface comparison; earlier post-edge full run is diagnostic only, not a valid quality comparison because it also had the v6/v5 report warning |

## Snapshot and repository capture

- All fixture worktrees were clean at the recorded revisions. Syncthing had 830
  tracked files, Caddy 612, and Restic 1,344.
- Compact context was bounded at 250 candidates: 89,791 bytes (Syncthing),
  88,481 (Caddy), and 100,345 (Restic). No full file tree or repository contents
  were sent to a provider.
- Final-local snapshot times were 3,550 ms, 1,831 ms, and 2,882 ms respectively.
  Compared with the earlier usable captures, these are descriptive only; they do
  not establish regressions without repeated same-host runs.

## Go/package analysis

No independent package-analysis timer was recorded; package loading is included
in surface discovery. Syncthing retained unavailable-package diagnostics including
`undefined: auto.Assets`. Restic's completed fallback intentionally disabled surface
discovery, so it cannot be used as a Go-analysis or surface benchmark. No evidence
shows duplicate package analysis or a missing cache hit.

## Surface discovery

| Fixture | final-local observation | earlier reference | disposition |
|---|---:|---:|---|
| Syncthing | 45 records / 36,643 ms | 36 expected; earlier post-edge artifact had 21 before report loss | invalid; canonical multiplicity mismatch |
| Caddy | 18 records / 32,658 ms | 33 records in the passing initial report; 18 in post-edge | invalid; registration mismatch and empty final projection |
| Restic | not completed in default mode; fallback disabled discovery | 11 records / 43,609 ms post-edge | invalid comparison; timeout prevents a final full-surface result |

The Syncthing and Caddy counts must not be interpreted as performance wins or
losses: their consumer reports reject the producer's version-6 catalog. Restic's
fallback completed reconciliation but is explicitly not a surface-discovery run.

## Local FlowProof

All final-local product runs were offline and expanded zero directions. No saved
FlowProof traces, direction timings, or local FlowProof cache-hit counters exist.
This is an intentional mode limitation, not evidence of a regression or duplicate
FlowProof work.

## Provider calls, bytes, latency, and cache hits

| Fixture | calls | request / response bytes | latency | cache |
|---|---:|---:|---|---|
| Syncthing | 0 | 0 / 0 | n/a | provider path skipped |
| Caddy | 0 | 0 / 0 | n/a | provider path skipped |
| Restic | 0 | 0 / 0 | n/a | provider path skipped |

There is no provider-latency regression, duplicate provider work, or missing
provider cache hit to report. Checked-in responses are test fixtures, not calls in
these runs. No external provider blocker was encountered.

## Architecture synthesis

No orientation, model synthesis, focused research, or direction expansion ran.
Therefore synthesis latency, architecture quality, and model-cache behavior are
unmeasured. The retained local grounding is not a valid model-synthesis baseline.
The known Restic grounding inaccuracies and the Syncthing/Caddy report loss are
correctness issues, not performance conclusions.

## Report generation

- Syncthing: 157 ms; report generated but is not acceptable because the catalog
  reader rejected v6 and serialized an empty surface inventory.
- Caddy: 83 ms; same v6/v5 incompatibility, with zero projected surfaces.
- Restic fallback: 71 ms; this is a no-surface report and cannot be compared with
  full discovery. Earlier full runs were 53 ms and 74 ms, but those reports also
  carried the compatibility warning.

Short report times do not redeem invalid artifacts. No meaningful report-generation
regression is established.

## Freshness reconciliation

- Syncthing: 402 inputs, 142,933 ms.
- Caddy: 280 inputs, 56,579 ms.
- Restic fallback: 357 inputs, 70,084 ms.

Input counts match the earlier captures for each fixture. No stale-source or
freshness failure was recorded. Restic's fallback reconciliation is not comparable
to a full run as a performance acceptance result, and the long duration is a
candidate for repeated-run investigation rather than a release blocker.

## Browser/report-server startup

Not measured. Every retained product command used `--no-open --no-serve`; hence
there are no browser startup, report-server startup, served-report latency, browser
cache-hit, or console measurements. Any comparison in this category would be
invalid.

## Wall time and disposition

| Fixture | final-local wall / user / system | interpretation |
|---|---|---|
| Syncthing | 186.35 / 114.96 / 59.19 s | not an accepted performance result; freshness and invalid report dominate |
| Caddy | 93.75 / 100.31 / 31.81 s | not an accepted performance result; final report is empty |
| Restic fallback | 74.92 / 39.98 / 29.05 s | no-surface fallback only; not comparable with full discovery |

### Meaningful findings

1. **Correctness blockers:** Syncthing and Caddy lose nonempty discovered data at
   the report boundary because producer catalogs are v6 while the reader accepts
   v5. Caddy additionally fails its registration oracle. These are meaningful
   regressions, but not performance regressions.
2. **Invalid Restic comparison:** the final-local default run was canceled during
   surface discovery after 180 seconds and two 600-second attempts; the completed
   fallback disabled discovery. Do not compare its 74.92-second wall time with a
   full run.
3. **Duplicate work:** retries and canonical/product/playground captures repeat
   local analysis, but there were no duplicate provider calls. The repeats are
   diagnostic artifacts, not production work.
4. **No provider-noise failure:** provider latency and cache behavior were absent by
   design. No external service outage or provider blocker exists.

## External blocker

**Yes, for completing a valid final-local performance comparison:** the execution
harness canceled all full Restic final-local attempts at its timeout, and browser/
server and provider-backed modes were not run. This is an environment/measurement
blocker, not an external provider outage. The report-schema mismatch and Caddy/
Syncthing acceptance failures are repository-side blockers and should be fixed
before another performance wave.
