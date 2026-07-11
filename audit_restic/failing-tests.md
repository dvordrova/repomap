# Required failing tests

These are test contracts, not implemented tests. They must be introduced red
against the current implementation before a fix is accepted. Prefer tiny
restic-shaped fixtures; do not couple unit tests to rendered HTML or a full
restic checkout.

## TestMeaningfulHandlerCalls_ErrgroupCallbackIsNotSynchronousHandlerCall

findings:
- FP-001

fixture:
  a handler containing wg.Go(func() error { return sc.Scan(ctx, targets) })

assertions:
- handler-level calls include wg.Go exactly once
- no synchronous handler to Scanner.Scan transition exists
- Scanner.Scan occurs exactly once as the asynchronous task body

current_failure:
  ast.Inspect descends into the FuncLit and emits both the synchronous and callback forms

## TestLifecycle_BackupPreservesMainAndScannerBranches

findings:
- FP-002

fixture:
  backup-shaped function with optional scanner task followed by synchronous Snapshot

assertions:
- main branch contains Snapshot
- scanner task branch contains Scan
- NoScan branch contains no scanner task
- neither branch is flattened into one source-order call chain

current_failure:
  the proof attaches Scan directly to runBackup and also to Group.Go

## TestLifecycle_CancelAndWaitAreSiblingOperations

findings:
- FP-003

fixture:
  cancel(); err := wg.Wait()

assertions:
- no calls or joins transition runs from cancel to Wait
- cancel and Wait share the handler as owner
- source order, if retained, uses a non-call relation

current_failure:
  lifecycleResult emits cancel to Wait with RelationJoins

## TestLifecycle_WaitJoinsScannerTask

findings:
- FP-004

fixture:
  one errgroup, one conditional scanner task, one Wait

assertions:
- Wait is the join operation
- the joined target is the scanner task registered on the same group
- NoScan branch has an empty joined task set
- the outer Wait does not claim to join Snapshot's internal errgroup

current_failure:
  the current edge ends at the Wait method and starts at cancel; no task identity is represented

## TestLifecycle_SourceOrderIsNotRepresentedAsCallEdge

findings:
- FP-005

fixture:
  Snapshot(); cancel(); werr := wg.Wait(); three conditional return outcomes

assertions:
- source order creates no fabricated calls, joins or returns ownership edge
- snapshot-error, partial-source and werr outcomes remain separate branches
- a precedes relation is permitted only for operations in the same main branch

current_failure:
  current lifecycle emits cancel to Wait and Wait to the lexically last return

## TestIOBoundary_InternalRepositoryCallRemainsPartial

findings:
- FP-006
- FP-008

fixture:
  handler calling an internal repository facade whose target is statically resolved

assertions:
- target resolution changes callable identity only
- I/O boundary remains partial until a backend/read/write/save witness exists
- an internal method name containing Load or Open is insufficient

current_failure:
  name-based I/O selection plus static target resolution promotes the slot to verified

## TestTermination_HandlerReturnDoesNotProveProcessExit

findings:
- FP-007

fixture:
  Cobra handler return followed by ExecuteContext return, exit-code mapping and os.Exit

assertions:
- handler completion and process completion are distinct claims
- handler return does not satisfy process completion
- process completion requires the selected main/Exit chain

current_failure:
  termination is verified from Wait to one runBackup return

## TestSession_PartialTargetCannotBecomeVerifiedOrCompleteByResolutionAlone

findings:
- FP-008

fixture:
  all slots satisfied except one partial core slot with one unresolved transition and an unmet role criterion

assertions:
- resolving the transition target leaves the slot partial
- slot summary and missing criterion remain consistent
- session cannot become 8/8 verified
- stop reason cannot be complete

current_failure:
  refreshResolvedSlots promotes the slot solely from resolution and PlanNext stops complete

## TestCommandTrace_InitIncludesCreateRepository

findings:
- FP-009

fixture:
  runInit-shaped handler calling global.CreateRepository

assertions:
- handler calls contain exactly one CreateRepository call at the source callsite
- call target resolves separately to the declaration
- core-operation discovery does not depend on the word Load, Open, Save or New

current_failure:
  commandCallScore gives CreateRepository zero and drops it

## TestBuildCLI_InitConcurrencyIsNotApplicable

findings:
- FP-010

fixture:
  synchronous init handler with branches and no task/cancel/join lifecycle

assertions:
- concurrency status is not_applicable
- no missing concurrency request is emitted
- not_applicable counts as an honest satisfied slot for this flow scope

current_failure:
  SlotStatus cannot represent not_applicable and BuildCLI emits missing

## TestCobraDispatch_StoresCallsiteAndTargetLocationSeparately

findings:
- FP-011

fixture:
  main -> newRootCommand; AddCommand(newBackupCommand); RunE -> runBackup

assertions:
- root constructor callsite and declaration are separate
- subcommand constructor/registration callsite and declaration are separate
- RunE binding/callsite and handler declaration are separate
- backup expects main.go:188 to main.go:36, main.go:77 to cmd_backup.go:35, and cmd_backup.go:61 to cmd_backup.go:498
- init expects main.go:88 to cmd_init.go:20 and cmd_init.go:38 to cmd_init.go:58

current_failure:
  CommandTraceStep carries only declaration path/line and BuildCLI reuses it as transition evidence

## TestOrientation_ReconcilesLocallyResolvedUnknownFiles

findings:
- FP-014

fixture:
  overview unknown list with one path later resolved by a local proof and one unrelated path

assertions:
- resolved path is removed or reclassified from unknowns
- unrelated unknown remains unchanged
- fabricated/nonexistent diagnostics are not promoted to verified

current_failure:
  proof paths only become openable; orientation unknowns are copied unchanged

## TestComponentRelations_RequireComponentSpecificWitness

findings:
- FP-013

fixture:
  two source components share package P, two target components share package Q, and raw facts contain one P to Q import

assertions:
- no Cartesian four-edge component projection is produced
- each promoted component relation has a component-specific source and target witness
- ambiguous ownership keeps the package edge without inventing component arrows

current_failure:
  nested loops project the package edge to every matching component pair

## TestCommandTrace_DeferredCleanupIsNotSynchronous

findings:
- FP-012

fixture:
  defer resource.DeleteSnapshots()

assertions:
- invocation mode is deferred
- call is associated with handler unwinding and its branch
- it is not placed as an immediate synchronous step

current_failure:
  invocationMode defaults every ordinary call to synchronous

## TestLifecycle_BackupKeepsAllPostWaitOutcomes

findings:
- FP-005
- FP-007

fixture:
  Wait followed by snapshot error, partial-source error, and final task-error returns

assertions:
- all three handler outcomes are retained
- no lexically last return stands for all outcomes
- none of the handler outcomes claims process exit

current_failure:
  lifecycleTail stores only the last ReturnStmt after Wait
