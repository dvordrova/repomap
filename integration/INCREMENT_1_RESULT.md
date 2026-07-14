# Increment 1 result: honest restic semantics

## Focused regressions added

Eight audit-derived semantic regressions protect the accepted Increment 1
scope; lower-level parser/resolver tests cover the new bounded machinery:

1. `TestCommandTraceNestedCallbackIsNotSynchronousHandlerCall`
2. `TestInitTraceIncludesCreateRepository`
3. `TestCobraDispatchKeepsCallsiteAndTargetLocationsSeparate`
4. `TestBackupContainsMainAndScannerBranches`
5. `TestBackupCancelDoesNotJoinWait`
6. `TestBackupWaitJoinsScannerTask`
7. `TestPartialTargetCannotCompleteProof`
8. `TestInitConcurrencyIsNotApplicable`

The `not_applicable` regression also proves that the bounded reason satisfies
the core proof contract. The cancellation regression contains a negative
subcase proving that an unrelated context cancellation cannot verify the
scanner task.

## Behavior corrected

- nested `Scanner.Scan` is no longer collected as an immediate `runBackup`
  call;
- the `!opts.NoScan` guard is preserved on the scanner-task branch instead of
  being flattened into an unconditional lifecycle;
- Cobra dispatch keeps exact `callsite_location` and `target_location`;
- init retains `global.CreateRepository`;
- command-trace v2 rejects legacy v1 replay instead of silently
  reinterpreting it;
- backup has separate handler and scanner-task branches;
- `cancel()` targets `cancelCtx`; `Wait()` joins the scanner task; both remain
  sibling handler operations;
- the scanner task is explicitly associated with the same cancellation target
  before concurrency can be verified;
- adapters emit facts only; core policy owns applicability and slot verdicts;
- resolving source targets cannot verify core/I/O roles or produce
  `stop=complete`;
- synchronous init marks concurrency `not_applicable` with the bounded reason
  `no_concurrent_lifecycle_in_scope`.

## Verification

Focused packages passed:

```text
go test -count=1 ./internal/evidence ./internal/gofacts ./internal/flowproof \
  ./internal/analyzer/golang/gotypes ./internal/flowproof/assemble \
  ./internal/llmbundle ./internal/orient
```

Fresh offline facts were extracted from restic revision
`987caba4089fc4345bb201e62c5a2ba96b168049`. Replaying the saved orientation
with the fresh v2 bundle produced:

- backup: trigger/entrypoint/dispatch/application verified; concurrency stays
  partial until a scenario proves `!opts.NoScan`; core and I/O partial;
  termination missing; `stop=no_task`;
- init: trigger/entrypoint/dispatch/application verified; concurrency
  `not_applicable`; core and I/O partial; termination missing;
  `stop=no_task`.

The sole edge entering `Scanner.Scan` is callback/goroutine from the scanner
task. Legacy command-trace v1 artifacts are skipped with an incompatible-version
warning and receive no local proof.

`go test ./...` also passed against the exact staged tree in a detached clean
worktree before commit review.

## Remaining known failures and assumptions

- No current restic flow is 8/8 complete; this is intentional.
- Core-operation classification still needs a domain-role witness.
- The I/O slot still needs an external resource or persistence-boundary
  witness.
- Handler return and process termination remain unproved.
- No general path-condition solver was added; guarded facts remain partial
  until a selected scenario supplies the missing branch witness.
- Deferred-call semantics remain deferred; this increment does not claim a
  general Go control-flow model.
- The concrete lifecycle recognizer is deliberately bounded to the demonstrated
  inline `errgroup.Go` plus local `context.WithCancel` pattern.
