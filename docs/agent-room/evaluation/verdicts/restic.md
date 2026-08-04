FIXTURE VERDICT: BLOCKED

## Audited artifact and limits

This audit is of the final-local fallback report
`20260714-210221-restic`, bound to Restic
`987caba4089fc4345bb201e62c5a2ba96b168049` and 357 captured inputs
(`run_manifest.json:3-12`; `report.json:9271-9273`). Fresh source inspection
still resolves that revision and a clean worktree. The fallback exited 0, but it
explicitly used `--offline --discover-surfaces=false`
(`command.txt:1`; `metadata.json:12-22`; `identity.txt:6-7`). It is therefore
not a completed surface-discovery fixture run.

The three default-discovery attempts are **not evidence about Restic product
behavior** and did not contact a provider. They are an evaluation/setup limit:
each reached local Go surface discovery and was cancelled by the command-tool
deadline, including a compiled-binary attempt after 9m40s
(`...-binary/canonical.stderr.log:5-64`; run record `runs/restic.md:130-145`).
They produced no report to assess. Conversely, the fallback's missing model
orientation is an intentional offline limit, not a provider-coverage failure:
model and endpoint are empty and offline is true (`metadata.json:7-15`), stdout
says all LLM calls were skipped (`canonical.stdout.log:2-5`).

## Blocking findings

1. **The authoritative successful artifact disables the capability being
   evaluated.** Its catalog records `packages_inspected: 0`,
   `functions_inspected: 0`, no process entries, generic surfaces, tooling,
   dependencies, seeds, or frontiers (`report.json:1583-1616`). It cannot
   establish primary/secondary/tooling classification, dependency/noise
   precision, generic ownership, exact seed membership, or local coverage.
   Exit status 0 only establishes successful generation of this explicitly
   partial report (`exit.status:1`), not success of the full fixture.

2. **The visible command inventory is incomplete for the recorded Darwin
   source selection while presented as the discovered/application count.** The
   report serializes 28 commands in all three count fields
   (`report.json:13-19,1587-1605`), but source registers `mount` separately
   (`cmd/restic/main.go:107-109`) and `cmd_mount.go` is selected on Darwin
   (oracle `oracles/restic.md:63-68`). `cmd_mount.go` is even manifest-captured
   (`run_manifest.json:620-631`) but absent from report `openable_paths`
   (`report.json:20-270`) and from the 28 triggers. The count is internally
   equal, but not reconciled to build-selected command membership.

3. **The scope claim overstates what was performed.** The report says
   “Build-selected deterministic command registrations and bounded generic
   registration/start analysis” (`report.json:1585-1587`), although the run
   disabled surface discovery and all generic-analysis counters are zero
   (`metadata.json:13-16`; `report.json:1588-1616`). This makes a partial
   fallback look like a full local surface result.

## What is useful and honest

- The 28 ordinary Cobra registrations are useful static entry facts. For
  example, `backup` is correctly owned by `cmd/restic`, classified
  `primary_application`, and traces exact `main` → `newRootCommand` →
  `newBackupCommand` → `runBackup` source evidence
  (`report.json:1619-1775`). This supports recall of the oracle's key `init`,
  `backup`, `restore`, and `check` paths (traces at `report.json:5963,6456,7659,8495`),
  but does not prove repository or backend activity.
- The command traces appropriately distinguish static evidence from execution
  through `reachability: static` (`report.json:1768-1774`). They are only
  registration/handler traces, not saved end-to-end flows. A `complete: true`
  marker on such a trace (for example `report.json:9065-9066`) must remain
  scoped to the serialized command trace, not imply a complete runtime flow.
- The report is honest that no model orientation or flows exist:
  `orientation_confidence` is 0, candidate flows and flows are null
  (`report.json:3-12`), and `flow_count` is 0 (`report.json:9271-9273`). No
  suggestions, focused research windows, saved traces, or evidence bundles are
  available in the run directory. The blank feedback template has no research
  result (`onboarding-feedback.md:1-18`). These absences must not be counted as
  provider or local coverage.
- The package-landscape architecture result is deliberately thin rather than
  fabricated: it selects only `application`, records no behavior grounding,
  anchors, or relationships (`report.json:1570-1581`). It is insufficient to
  validate component ownership/responsibilities against the oracle's command,
  global/backend, repository, archiver, restorer, and checker boundaries, but
  it makes no false detailed responsibility claim.

## Evidence, membership, and coverage accounting

- Source evidence for the serialized command traces is repository-local and
  manifest-bound (for example `cmd/restic/main.go` and command files in
  `run_manifest.json:313-925`). The report correctly binds the repository head
  and report digest (`run_manifest.json:3-12`).
- There are no generic/dependency surface records, so there are no seeds,
  dependency semantic evidence bundles, or generic frontiers whose exact
  membership can be audited. `configured_seeds_matched` is null, not an empty
  successful match (`report.json:1613-1616`). The report must not turn those
  null/zero fields into a “no dependencies/no noise” claim.
- The displayed counts reconcile only mechanically: 28 CLI = 28 application =
  28 total (`report.json:1588-1604`), while both omit the available `mount`
  command. No provider coverage was claimed or obtained; no full local generic
  coverage was obtained either.

## Advisory findings

- The orientation warning exposes an absolute missing-artifact path rather than
  a structured offline state (`report.json:10-12`).
- The report labels its architecture mode `package_landscape` while the source
  facts required to assess architecture responsibilities were available in the
  manifest; without behavior anchors it should be visibly limited to a
  non-authoritative landscape.

## Smallest generic correction hints

1. Mark `--discover-surfaces=false` reports as partial and suppress full
   discovered/application coverage claims; require a completed full run for
   fixture acceptance.
2. Reconcile command inventory with build-selected conditional registrations
   before publishing totals or completeness.
3. Keep null/omitted generic, seed, dependency, research, trace, and evidence
   artifacts visibly distinct from verified zero results.
4. Bound or make resumable local surface discovery so a timed-out analysis
   yields a durable, explicitly partial artifact rather than no report.
