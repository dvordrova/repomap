# 273 — One framework-free Go analysis path

**Status:** ACTIVE (owner-authorized, 2026-08-10)

**Preserves:** the target, depth, edge-ceiling and `DirectCallIndex` contracts
from D257/D260/D261; D248/D259's private refs-only entry-call substrate and
item-local validation; D253's exact plain `net/http` handler shape; all current
entry-call provider, report, manifest and target-page contracts; and readers
for persisted runs made by older producers.

**Supersedes:** D248's temporary `--no-frameworks` mode and negative option
plumbing, D253 only where it retained third-party framework handler widening,
and D259 only where it described the retained entry-call contract as a
temporary mode. Their historical evidence and still-preserved contracts are
not rewritten.

## Product failure

The user-facing flag had already been removed, but ordinary execution still
carried a negative `NoFrameworks` option through CLI, snapshot, Go facts,
Surface and debug metadata. The analyzer therefore had two latent modes even
though users could select only one. Deleting the boolean without replacing its
branches positively would have silently re-enabled framework/Cobra producers
and disabled the private entry-call capture.

## Approved ordinary contract

- Go analysis has one ordinary path and no framework-mode option or fresh
  `no_frameworks` metadata.
- Surface loads a positive core catalog consisting of exactly the eleven
  `net/http` and `errgroup` seeds. Embedded framework catalog files do not
  participate in ordinary discovery.
- The generic typed-registration detector accepts only the exact D253 plain
  function shape and named `net/http.Handler` / `net/http.HandlerFunc`.
  Gin, Echo, chi and other third-party context handler types do not widen it.
- Ordinary analysis produces no fresh Cobra surface inventory and Go facts
  produce no fresh Cobra command traces.
- The existing Cobra/framework coverage and command-trace DTO fields, JSON
  names, parsers and report readers remain so older snapshots and reports are
  still readable. Compatibility does not make those producers live again.

## Entry-call and target authority

The authoritative `DirectCallIndex` remains part of the same repository-local
SSA pass and is built regardless of entry-call capture. Target package, exact
roots, depth and edge limits, closure state and SHA binding do not change.

The private entry-call sidecar is retained only when a live consumer is
supplied. In the ordinary pipeline that means `EntryCallSubstrateSink != nil`;
offline/no-consumer runs do not retain it. Capture must not change the
`DirectCallIndex` bytes or SHA. The existing refs-only request, item-local
validation, result/status artifacts, report projection, manifest authorization
and multi-target page contracts remain unchanged.

## Acceptance

1. The positive core catalog contains eleven seeds and cannot be widened by a
   new embedded framework catalog file.
2. Custom exact `net/http`-shaped registrations, standard-library HTTP seeds
   and the `DirectCallIndex` remain available without an option.
3. Cobra-using source and third-party context handlers create no fresh
   framework/Cobra surfaces or command traces.
4. Capturing the entry-call sidecar on and off yields the same authoritative
   `DirectCallIndex` SHA; only the captured sidecar differs.
5. Old JSON containing retired coverage or command-trace fields continues to
   decode through the preserved schemas/readers.

No provider call or real-repository run is authorized by this decision.
