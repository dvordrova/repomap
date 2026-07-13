# Surface accounting audit

This audit uses the owner-provided model-backed Restic run
`20260712-231328-restic`. It distinguishes the bounded model bundle, local
producer stages, the unified product catalog, architecture ownership, and saved
FlowProof traces. A saved trace is an association on a surface; it is never a
surface producer.

## Why “were truncated in the bundle” appears

The orientation request deliberately sends a compact bounded facts bundle. The
normal limits include 60 model-visible candidate files and 60 important import
edges. When a larger local snapshot reaches either limit, deterministic bundle
construction records `truncated important edges` or `truncated
candidate_file_index`. DeepSeek sometimes paraphrases those facts as “were
truncated in the provided bundle.” The wording varies because it is provider
prose.

This warning means only that model-generated orientation and suggestions saw a
bounded subset. It does **not** mean that tracked-file extraction, persisted
command traces, generic surface discovery, the unified surface catalog, or
saved exact evidence were truncated. The report now labels these notices
`Model context limit` and keeps actual local-analysis failures separate.

## Previous accounting

The old numbers described three different collections:

| UI value | Collection | Records |
|---|---|---|
| Discovered surfaces: 4 | `architecture_canvas.surfaces` | two generic records plus two trace-derived presentation records |
| Runtime surfaces stage: 2 | `metadata.surface_discovery_count` | two generic SSA records |
| Architecture-linked command surfaces: 2 | unmatched saved traces projected as starts | backup and check |
| All surfaces: 2 | `discovered_surfaces.triggers` | only the two generic records |

The two `trace-start-*` records were not detector output. Their presence made
the headline depend on model-selected flows while All surfaces read a separate
collection:

| Stable ID | Kind/source | Executable | Component | Saved trace | Headline | All surfaces | Exclusion reason |
|---|---|---|---|---|---:|---:|---|
| `trace-start-backup-command-flow` | saved-trace start | `cmd/restic` | Backup Command | `backup-command-flow` | yes | no | presentation-only trace projection |
| `trace-start-check-repository-flow` | saved-trace start | `cmd/restic` | Check Command | `check-repository-flow` | yes | no | presentation-only trace projection |
| `trigger-88dd322ad1d5016c6eb1ac1f` | worker | `helpers/build-release-binaries` | Build Release Binaries | — | yes | yes | — |
| `trigger-7189f22f0aeb6587edd1b13d` | async task | `helpers/build-release-binaries` | Build Release Binaries | — | yes | yes | — |

## Unified accounting rules

`DiscoveredSurfaces.Triggers` is now the one typed product catalog. It merges
persisted build-selected Cobra registrations with the generic registration/start
artifact. Every record retains producer, discovery basis, exact registration,
constructor and callback evidence when available, executable and role,
component association, trace association, certainty, resolution, status, and
provenance.

- `Discovered surfaces` = `len(discovered_surfaces.triggers)`.
- All surfaces iterates that same slice without producer-specific exclusion.
- `Generic surface scan` remains a stage metric and reports only generic-stage
  duration/output.
- Architecture surface IDs and component counts are projections of catalog IDs.
- Saved traces attach to exact typed command identity or evidence; they never
  add catalog records.
- Application headline counts exclude secondary tooling.
- Diagnostic remainder components do not override a more specific exact owner.

For this run the reconciled total headline is **30**: 28 primary-application
Cobra commands plus two secondary-tooling generic task records. The application
subtotal is 28 and excludes Tooling. All 30 records are included in both the
total headline and All surfaces.

## Reconciled records

| Stable ID | Surface | Kind | Producer | Owning executable | Executable role | Owning component | Related saved trace | Headline | All surfaces | Exclusion reason |
|---|---|---|---|---|---|---|---|---:|---:|---|
| `surface-08a0252a5adf1ea5427da91b` | backup | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Backup Command | `backup-command-flow` | yes | yes | — |
| `surface-c6f45fe2a7fcdb92848af478` | cache | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-6688598c0d6215a5710ccbda` | cat | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-45277d13c85cda9f05b05c56` | check | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Check Command | `check-repository-flow` | yes | yes | — |
| `surface-5f2c7e9c4a7d135890c16002` | copy | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-161b2a6d6543080a1cc42b15` | diff | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-38638d006ced590aeccf9513` | dump | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-d2053df6bc41f3a39753e102` | features | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-41905a13e5758019e73d109c` | find | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-3bbffc08f53532531ea5e6f8` | forget | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-9021a7721e5d43930d4e9be1` | generate | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-aff3b3e6307215006cb03176` | init | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-2aeeefcebc832da9c8e5b8a5` | key | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-3d2b4d6a4bc84d97f478f3cd` | list | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-980e14367cce9b9348e8bcd6` | ls | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | List Command | — | yes | yes | — |
| `surface-7d62c9da22b85a8378cef7f4` | migrate | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-fd788b0946d8fb5b41cb9b9e` | options | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-77facacc7b17865da12b7e39` | prune | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-dce5f8b2b69fa88f8bf2dcec` | rebuild-index | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-cc7273f7b92b077b57e2b0aa` | recover | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-db52882d33161520ccde6e5a` | repair | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-fa780c39d722d0a459da8ca6` | restore | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Restore Command | — | yes | yes | — |
| `surface-883df6b15e718965779a20da` | rewrite | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-c222acc5cbeaa3c3c1b3779c` | snapshots | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-ceef5aa368cd836f0d8b8fd9` | stats | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-2031cbc710a6d3e4757174fb` | tag | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-378e0b63773969d4a5834a35` | unlock | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `surface-705326d6fc140d63c8ff8361` | version | `cli_command` | `deterministic_cobra` | `cmd/restic` | `primary_application` | Main Entry | — | yes | yes | — |
| `trigger-88dd322ad1d5016c6eb1ac1f` | buildTargets task 1 | `worker` | `generic_surface_scan` | `helpers/build-release-binaries` | `secondary_tooling` | Build Release Binaries | — | yes | yes | — |
| `trigger-7189f22f0aeb6587edd1b13d` | buildTargets task 2 | `async_task` | `generic_surface_scan` | `helpers/build-release-binaries` | `secondary_tooling` | Build Release Binaries | — | yes | yes | — |

## Required Restic command evidence

| Command | Constructor | AddCommand site | Callback | Saved trace |
|---|---|---|---|---|
| backup | `newBackupCommand` | `cmd/restic/main.go:77` | `runBackup`, `cmd/restic/cmd_backup.go:498` | partial |
| check | `newCheckCommand` | `cmd/restic/main.go:80` | `runCheck`, `cmd/restic/cmd_check.go:229` | partial |
| init | `newInitCommand` | `cmd/restic/main.go:88` | `runInit`, `cmd/restic/cmd_init.go:58` | none |
| restore | `newRestoreCommand` | `cmd/restic/main.go:98` | `runRestore`, `cmd/restic/cmd_restore.go:99` | none |
| snapshots | `newSnapshotsCommand` | `cmd/restic/main.go:100` | `runSnapshots`, `cmd/restic/cmd_snapshots.go:84` | none |
| list | `newListCommand` | `cmd/restic/main.go:90` | `runList`, `cmd/restic/cmd_list.go:50` | none |
| prune | `newPruneCommand` | `cmd/restic/main.go:94` | `runPrune`, `cmd/restic/cmd_prune.go:164` | none |
| find | `newFindCommand` | `cmd/restic/main.go:85` | `runFind`, `cmd/restic/cmd_find.go:596` | none |

The saved July 12 snapshot predates leading-static-string extraction for
concatenated Cobra `Use` expressions, so its `list` record is replayed from the
exact `newListCommand` constructor with partial identity status. Fresh
deterministic extraction reads the leading `"list ..."` literal and produces a
complete record; the regression fixture covers that distinction.

## Caddy remains distinct

The Caddy run `20260712-231321-caddy` has 13 exact architecture-anchor families
and zero supported catalog surfaces. No anchor is promoted to a fake surface.
Accepted untraced directions map by exact anchor/member IDs to Main Entry,
Admin Handler, and HTTP Middleware. The saved artifact contains no accepted
Config Pipeline direction, so that component truthfully shows its exact anchor
without an invented suggestion.

## Visible metric changes

- `Discovered surfaces 30 · 28 application · 2 tooling` is the unified total
  and role breakdown.
- `Generic surface scan 31 s · 2 found` is explicitly one producer-stage
  measurement, not the product total.
- All surfaces shows `30 total`, `28 CLI commands`, `28 application`, and `2
  tooling`; the two `buildTargets` records are under Tooling.
- Component card counts are derived from the same catalog IDs. Backup Command
  shows one command and one partial trace; Restore Command shows one command and
  no trace.
- Caddy cards prefer suggestions/anchors/members over repeated zero counts, while
  the compact All surfaces row still states the repository-wide zero result.

## Browser validation

Playwright exercised the final static replay at 1600×1000, 1440×900, and
1280×800. Restic showed all 28 Cobra commands after expanding All surfaces,
grouped both `buildTargets` records under Tooling, distinguished Backup from
untraced Restore, opened the Backup saved trace, and returned to the stable
Architecture context. Caddy retained its validated architecture, adaptive card
metadata, exact Admin/HTTP suggestions, compact empty catalog, and coverage
disclosure without surfaces.

Review screenshots are intentionally untracked debug artifacts:

- `decision080-restic-1600x1000.png`
- `decision080-restic-backup-1440x900.png`
- `decision080-restic-traces-1280x800.png`
- `decision080-caddy-1600x1000.png`
- `decision080-caddy-admin-1440x900.png`
- `decision080-caddy-empty-surfaces-1280x800.png`

Static exports cannot execute manifest-authorized source actions; they explain
that limitation rather than rendering a dead button. The normal
`repomap <repo>` served journey exposes `Open starting source` for suggestions
with exact anchors. The current run has no locally executable action that can
build a new trace, so `can_start_trace` remains false and no fake trace action is
shown.
