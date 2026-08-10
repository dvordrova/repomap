# 250 — Explicit Go target scenario

**Status:** ACTIVE (owner-authorized, 2026-08-09)

**Preserves:** D220's build-selected local surface authority, D243/D246's
single SSA and refs-only provider boundaries, D247 publication truth, D248's
optional framework-free experiment, and D249's producer-owned relation bounds.

## Product failure

An ordinary Moby run on macOS analyzed the `darwin/amd64` build universe.
`cmd/dockerd` was consequently unavailable and test/fixture mains dominated
the optional entry-call input. Running the same exact binary provider-free with
`GOOS=linux GOARCH=amd64` restored `cmd/dockerd`, expanded the build-selected
set from 351 to 378 packages and the SSA function set from 26,912 to 49,247,
but the saved surface artifacts still claimed `darwin/amd64` because the
producer used `runtime.GOOS/runtime.GOARCH` for identity. The local facts and
their recorded authority therefore described different programs.

`linux` is a GOOS constraint, not an ordinary `-tags=linux` feature tag.
Injecting it as a build tag while retaining `GOOS=darwin` is forbidden: it does
not select the Linux standard-library/type universe and can admit mutually
incompatible files.

## Approved contract

- Add one atomic ordinary-command option, `--go-target GOOS/GOARCH`. Separate
  independently optional OS and architecture flags are deliberately absent.
- Without the flag, each nonempty process `GOOS`/`GOARCH` value remains the Go
  command default for that dimension; an absent value falls back to the binary
  host. The resolved pair is validated and frozen once per run.
- The exact same pair configures both deterministic Go loaders: snapshot
  `go list`/Go facts and Surface `packages.Load`/SSA. Neither loader may inherit
  a conflicting ambient value after resolution.
- The saved Surface scenario, semantic repository context/cache identity and
  debug effective options bind that same pair. A mismatch between the two Go
  loaders fails closed; it is never repaired by presentation code.
- Report42 already persists the Surface scenario and Manifest14 already binds
  the exact report bytes. No manifest, report, Canvas, UI, provider schema,
  prompt, or semantic artifact identity changes.
- This decision adds no build-tag flag, platform autodetection heuristic,
  framework adapter, root ranker, provider call, retry, report field, or UI.

## Acceptance

1. A provider-free Linux-target fixture selects Linux-only Go facts and
   Surface syntax, and both saved authorities say `linux/amd64`.
2. Default host behavior remains unchanged; ambient and explicit targets are
   resolved once and conflicting loader environment values are replaced.
3. Invalid target syntax and target drift fail before provider use.
4. A fresh provider-free Moby run with `--go-target linux/amd64` publishes
   `cmd/dockerd` as an available process entry and records no Darwin scenario.

## Owner-risk check

This is the smallest change that makes the analyzed program and its authority
agree. It is needed for platform-specific repositories beyond this week,
fixes the cause rather than a warning or test, and removes a hidden split even
though the target value must be threaded through several existing seams. If it
is skipped, every macOS Moby iteration repeats the same wrong-program audit;
the next iteration cannot recover that lost time automatically.
